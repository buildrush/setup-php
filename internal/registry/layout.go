package registry

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/layout"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/static"
	"github.com/google/go-containerregistry/pkg/v1/types"
)

// annotationBundleName is the OCI manifest annotation key used to tag each
// pushed bundle with its logical Ref.Name, so Has/Fetch can walk the index
// and find the right manifest without relying on external state.
const annotationBundleName = "io.buildrush.bundle.name"

// layoutStore is a filesystem-backed Store implemented over an on-disk OCI
// image layout (pkg/v1/layout). Bundles are stored as two-layer OCI images:
// layer 0 carries the bundle bytes, layer 1 (when present) carries the
// serialised Meta sidecar. Legacy bundles without a meta layer Fetch back
// with a default Meta{SchemaVersion:1}, matching the runtime's tolerance in
// internal/oci/client.go.
type layoutStore struct {
	root string
}

func openLayout(path string) (*layoutStore, error) {
	if path == "" {
		return nil, errors.New("registry: oci-layout URI requires a path")
	}
	return &layoutStore{root: path}, nil
}

func (s *layoutStore) Kind() string { return "layout" }

// openOrInit returns a writable layout.Path, creating an empty layout at
// s.root on first use.
func (s *layoutStore) openOrInit() (layout.Path, error) {
	if p, err := layout.FromPath(s.root); err == nil {
		return p, nil
	}
	p, err := layout.Write(s.root, empty.Index)
	if err != nil {
		return layout.Path(""), fmt.Errorf("layout: init %q: %w", s.root, err)
	}
	return p, nil
}

// open returns a read-only handle to the layout at s.root. A missing layout
// surfaces as the underlying error from layout.FromPath.
func (s *layoutStore) open() (layout.Path, error) {
	return layout.FromPath(s.root)
}

func (s *layoutStore) Push(_ context.Context, ref Ref, body io.Reader, meta *Meta, ann Annotations) error {
	if ref.Name == "" {
		return errors.New("layout.Push: ref.Name required")
	}
	bundleBytes, err := io.ReadAll(body)
	if err != nil {
		return fmt.Errorf("layout.Push: read bundle: %w", err)
	}

	layers := []v1.Layer{static.NewLayer(bundleBytes, types.OCILayer)}
	if meta != nil {
		metaBytes, err := json.Marshal(meta)
		if err != nil {
			return fmt.Errorf("layout.Push: marshal meta: %w", err)
		}
		layers = append(layers, static.NewLayer(metaBytes, types.OCILayer))
	}

	img, err := mutate.AppendLayers(empty.Image, layers...)
	if err != nil {
		return fmt.Errorf("layout.Push: append layers: %w", err)
	}
	// Annotate both the image manifest and the index descriptor. The index
	// descriptor annotation is what Has/Fetch walk over (go-containerregistry's
	// partial.Descriptor does not propagate manifest-level annotations into the
	// index), while the manifest-level annotation keeps the round-trip
	// self-describing for tools that inspect the OCI image directly.
	//
	// Callers supply the desired annotation set via Annotations. For backward
	// compatibility with PR 1 semantics, if the caller didn't set BundleName
	// (or any annotation at all) we fall back to deriving it from ref.Name so
	// Has/Fetch still find the manifest.
	annotations := ann.asMap()
	if annotations[annotationBundleName] == "" {
		annotations[annotationBundleName] = ref.Name
	}
	annotated, ok := mutate.Annotations(img, annotations).(v1.Image)
	if !ok {
		return errors.New("layout.Push: mutate.Annotations did not return v1.Image")
	}

	p, err := s.openOrInit()
	if err != nil {
		return err
	}
	if err := p.AppendImage(annotated, layout.WithAnnotations(annotations)); err != nil {
		return fmt.Errorf("layout.Push: append image: %w", err)
	}
	return nil
}

// LookupBySpec walks the index for a manifest whose annotations match BOTH
// the given bundle name and spec-hash. Returns (Ref, true, nil) on hit.
// An absent layout is a valid miss (not an error) so callers can probe
// empty caches without a pre-check. Any other open failure propagates.
func (s *layoutStore) LookupBySpec(_ context.Context, name, specHash string) (Ref, bool, error) {
	p, err := s.open()
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return Ref{}, false, nil
		}
		return Ref{}, false, fmt.Errorf("layout.LookupBySpec: open %q: %w", s.root, err)
	}
	idx, err := p.ImageIndex()
	if err != nil {
		return Ref{}, false, fmt.Errorf("layout.LookupBySpec: index: %w", err)
	}
	manifest, err := idx.IndexManifest()
	if err != nil {
		return Ref{}, false, fmt.Errorf("layout.LookupBySpec: index manifest: %w", err)
	}
	for i := range manifest.Manifests {
		m := &manifest.Manifests[i]
		if m.Annotations[annotationBundleName] != name {
			continue
		}
		if m.Annotations[annotationSpecHash] != specHash {
			continue
		}
		return Ref{Name: name, Digest: m.Digest.String()}, true, nil
	}
	return Ref{}, false, nil
}

func (s *layoutStore) Has(_ context.Context, ref Ref) (bool, error) {
	p, err := layout.FromPath(s.root)
	if err != nil {
		// Absent layout is not an error — the ref simply isn't present yet.
		if errors.Is(err, fs.ErrNotExist) {
			return false, nil
		}
		return false, fmt.Errorf("layout.Has: open %q: %w", s.root, err)
	}
	if ref.Digest == "" {
		// Without a digest we can't match a specific manifest. Treat as "not
		// present" rather than erroring so callers can probe empty stores.
		return false, nil
	}
	idx, err := p.ImageIndex()
	if err != nil {
		return false, fmt.Errorf("layout.Has: load index: %w", err)
	}
	manifest, err := idx.IndexManifest()
	if err != nil {
		return false, fmt.Errorf("layout.Has: parse index: %w", err)
	}
	fallback := false
	for i := range manifest.Manifests {
		m := &manifest.Manifests[i]
		if m.Digest.String() != ref.Digest {
			continue
		}
		ann, hasAnn := m.Annotations[annotationBundleName]
		if hasAnn && ann == ref.Name {
			return true, nil
		}
		// Only treat a digest-only match as a fallback candidate when the
		// manifest has NO bundle-name annotation at all (e.g. it was
		// populated by `oras copy` without our annotation). A manifest
		// that carries the annotation but names a *different* bundle is
		// an affirmative negative — we must not cross-link it.
		if !hasAnn {
			fallback = true
		}
	}
	return fallback, nil
}

func (s *layoutStore) Fetch(_ context.Context, ref Ref) (io.ReadCloser, *Meta, error) {
	p, err := s.open()
	if err != nil {
		return nil, nil, fmt.Errorf("layout.Fetch: open %q: %w", s.root, err)
	}
	idx, err := p.ImageIndex()
	if err != nil {
		return nil, nil, fmt.Errorf("layout.Fetch: load index: %w", err)
	}
	manifest, err := idx.IndexManifest()
	if err != nil {
		return nil, nil, fmt.Errorf("layout.Fetch: parse index: %w", err)
	}

	var (
		chosen        v1.Hash
		found         bool
		fallback      v1.Hash
		fallbackFound bool
	)
	for i := range manifest.Manifests {
		m := &manifest.Manifests[i]
		if ref.Digest != "" && m.Digest.String() != ref.Digest {
			continue
		}
		ann, hasAnn := m.Annotations[annotationBundleName]
		if hasAnn && ann == ref.Name {
			chosen = m.Digest
			found = true
			break
		}
		// See Has: we only accept a digest-only fallback when the manifest
		// carries no bundle-name annotation. An annotation that names a
		// different bundle is an affirmative negative.
		if ref.Digest != "" && !hasAnn {
			fallback = m.Digest
			fallbackFound = true
		}
	}
	if !found && fallbackFound {
		chosen = fallback
		found = true
	}
	if !found {
		return nil, nil, fmt.Errorf("layout.Fetch: %s not found in %q", ref, s.root)
	}

	img, err := idx.Image(chosen)
	if err != nil {
		return nil, nil, fmt.Errorf("layout.Fetch: load image %s: %w", chosen, err)
	}
	layers, err := img.Layers()
	if err != nil {
		return nil, nil, fmt.Errorf("layout.Fetch: list layers: %w", err)
	}
	if len(layers) == 0 {
		return nil, nil, fmt.Errorf("layout.Fetch: image %s has no layers", chosen)
	}

	bundle, err := layers[0].Compressed()
	if err != nil {
		return nil, nil, fmt.Errorf("layout.Fetch: open bundle layer: %w", err)
	}

	meta := &Meta{SchemaVersion: 1}
	if len(layers) >= 2 {
		mrc, err := layers[1].Compressed()
		if err != nil {
			_ = bundle.Close()
			return nil, nil, fmt.Errorf("layout.Fetch: open meta layer: %w", err)
		}
		metaBytes, err := io.ReadAll(mrc)
		_ = mrc.Close()
		if err != nil {
			_ = bundle.Close()
			return nil, nil, fmt.Errorf("layout.Fetch: read meta layer: %w", err)
		}
		parsed := &Meta{}
		if err := json.Unmarshal(metaBytes, parsed); err != nil {
			_ = bundle.Close()
			return nil, nil, fmt.Errorf("layout.Fetch: parse meta: %w", err)
		}
		if parsed.SchemaVersion == 0 {
			parsed.SchemaVersion = 1
		}
		meta = parsed
	}
	return bundle, meta, nil
}

func (s *layoutStore) ResolveDigest(_ context.Context, reference string) (string, error) {
	return "", fmt.Errorf("layout.ResolveDigest: not supported on layout backend (reference=%q)", reference)
}

// list walks the index and returns one Ref per manifest. Intended for tests
// only — callers in production should track their own refs or use a remote
// backend that can advertise them.
func (s *layoutStore) list(_ context.Context) ([]Ref, error) {
	p, err := layout.FromPath(s.root)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("layout.list: open %q: %w", s.root, err)
	}
	idx, err := p.ImageIndex()
	if err != nil {
		return nil, fmt.Errorf("layout.list: load index: %w", err)
	}
	manifest, err := idx.IndexManifest()
	if err != nil {
		return nil, fmt.Errorf("layout.list: parse index: %w", err)
	}
	if len(manifest.Manifests) == 0 {
		return nil, nil
	}
	refs := make([]Ref, 0, len(manifest.Manifests))
	for i := range manifest.Manifests {
		m := &manifest.Manifests[i]
		refs = append(refs, Ref{
			Name:   m.Annotations[annotationBundleName],
			Digest: m.Digest.String(),
		})
	}
	return refs, nil
}

var _ Store = (*layoutStore)(nil)
