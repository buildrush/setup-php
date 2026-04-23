package registry

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/remote/transport"
	"github.com/google/go-containerregistry/pkg/v1/static"
	"github.com/google/go-containerregistry/pkg/v1/types"
)

// remoteStore is the HTTPS-registry backed Store. It wraps
// go-containerregistry/pkg/v1/remote for Fetch / Has / ResolveDigest and
// Push; the only unsupported operation is LookupBySpec (see its comment).
type remoteStore struct {
	base string
	auth authn.Authenticator
}

// openRemote resolves auth from the environment (INPUT_GITHUB-TOKEN first,
// then GITHUB_TOKEN; anonymous if both are empty) and returns the store.
func openRemote(uri string) (*remoteStore, error) {
	token := os.Getenv("INPUT_GITHUB-TOKEN")
	if token == "" {
		token = os.Getenv("GITHUB_TOKEN")
	}
	var auth authn.Authenticator
	if token != "" {
		auth = &authn.Basic{Username: "token", Password: token}
	} else {
		auth = authn.Anonymous
	}
	return &remoteStore{base: uri, auth: auth}, nil
}

func (s *remoteStore) Kind() string { return "remote" }

// refFor constructs a name.Reference of the form "<base>/<name>@<digest>".
// Both the ref name and digest are required; a missing value is an error
// because the remote backend is strictly digest-addressed.
func (s *remoteStore) refFor(r Ref) (name.Reference, error) {
	if r.Name == "" {
		return nil, errors.New("remote: ref.Name required")
	}
	if r.Digest == "" {
		return nil, errors.New("remote: ref.Digest required")
	}
	return name.ParseReference(fmt.Sprintf("%s/%s@%s", s.base, r.Name, r.Digest))
}

func (s *remoteStore) Has(ctx context.Context, ref Ref) (bool, error) {
	// Symmetry with layoutStore.Has: an empty-digest probe is a legal
	// "not present" query. Fetch still rejects empty Digest as an error
	// because you can't fetch "nothing".
	if ref.Digest == "" {
		return false, nil
	}
	r, err := s.refFor(ref)
	if err != nil {
		return false, fmt.Errorf("remote.Has %s: %w", ref, err)
	}
	if _, err := remote.Head(r, remote.WithAuth(s.auth), remote.WithContext(ctx)); err != nil {
		// NOTE: Some registries (notably GHCR) return 401 Unauthorized for
		// blobs in private repos when the caller is anonymous, to avoid leaking
		// existence of private content. That will propagate as an error here.
		// If/when private-repo probes become a real use case (Task 6/7), consider
		// treating (terr.StatusCode == 401 && auth == anonymous) as "not present".
		var terr *transport.Error
		if errors.As(err, &terr) && terr.StatusCode == 404 {
			return false, nil
		}
		return false, fmt.Errorf("remote.Has %s: %w", ref, err)
	}
	return true, nil
}

func (s *remoteStore) Fetch(ctx context.Context, ref Ref) (io.ReadCloser, *Meta, error) {
	r, err := s.refFor(ref)
	if err != nil {
		return nil, nil, fmt.Errorf("remote.Fetch %s: %w", ref, err)
	}
	desc, err := remote.Get(r, remote.WithAuth(s.auth), remote.WithContext(ctx))
	if err != nil {
		return nil, nil, fmt.Errorf("remote.Fetch %s: %w", ref, err)
	}
	img, err := desc.Image()
	if err != nil {
		return nil, nil, fmt.Errorf("remote.Fetch %s: load image: %w", ref, err)
	}
	layers, err := img.Layers()
	if err != nil {
		return nil, nil, fmt.Errorf("remote.Fetch %s: list layers: %w", ref, err)
	}
	if len(layers) == 0 {
		return nil, nil, fmt.Errorf("remote.Fetch %s: image has no layers", ref)
	}

	// Use Compressed() so the returned bytes match what the layout backend
	// returns for the same manifest — both surfaces are "raw layer blob".
	bundle, err := layers[0].Compressed()
	if err != nil {
		return nil, nil, fmt.Errorf("remote.Fetch %s: open bundle layer: %w", ref, err)
	}

	meta := &Meta{SchemaVersion: 1}
	if len(layers) >= 2 {
		mrc, err := layers[1].Compressed()
		if err != nil {
			_ = bundle.Close()
			return nil, nil, fmt.Errorf("remote.Fetch %s: open meta layer: %w", ref, err)
		}
		metaBytes, err := io.ReadAll(mrc)
		_ = mrc.Close()
		if err != nil {
			_ = bundle.Close()
			return nil, nil, fmt.Errorf("remote.Fetch %s: read meta layer: %w", ref, err)
		}
		parsed := &Meta{}
		if err := json.Unmarshal(metaBytes, parsed); err != nil {
			_ = bundle.Close()
			return nil, nil, fmt.Errorf("remote.Fetch %s: parse meta: %w", ref, err)
		}
		if parsed.SchemaVersion == 0 {
			parsed.SchemaVersion = 1
		}
		meta = parsed
	}
	return bundle, meta, nil
}

// Push writes a bundle as a two-layer OCI image to "<base>/<ref.Name>:<ref.Tag>".
//
// Why Tag (and not Digest): OCI registries are tag-addressed for writes —
// the registry computes the manifest digest from the uploaded bytes; callers
// cannot supply one ahead of time. remote.Write therefore requires a tag.
// Digest-only Refs are a Fetch/Has concept; for Push the caller must provide
// a Tag.
//
// Media-type choices mirror the existing `oras push` command in
// .github/workflows/build-php-core.yml + build-extension.yml: layer 0 is the
// bundle at application/vnd.oci.image.layer.v1.tar+zstd, layer 1 (when meta
// is non-nil) is the meta sidecar at application/vnd.buildrush.meta.v1+json,
// and the manifest carries the OCI artifact-type annotation
// (org.opencontainers.artifact.type) that matches --artifact-type. Keeping
// this byte-identical with the CI path lets cosign + downstream OCI tooling
// treat remoteStore-pushed bundles exactly like oras-pushed ones.
func (s *remoteStore) Push(ctx context.Context, ref Ref, body io.Reader, meta *Meta, ann Annotations) error {
	if ref.Name == "" {
		return errors.New("remote.Push: ref.Name required")
	}
	if ref.Tag == "" {
		return errors.New("remote.Push: ref.Tag required (remote registries are tag-addressed for writes)")
	}
	bundleBytes, err := io.ReadAll(body)
	if err != nil {
		return fmt.Errorf("remote.Push: read bundle: %w", err)
	}

	layers := []v1.Layer{static.NewLayer(bundleBytes, types.MediaType(mediaTypeBundleLayer))}
	if meta != nil {
		metaBytes, err := json.Marshal(meta)
		if err != nil {
			return fmt.Errorf("remote.Push: marshal meta: %w", err)
		}
		layers = append(layers, static.NewLayer(metaBytes, types.MediaType(mediaTypeMetaSidecar)))
	}

	img, err := mutate.AppendLayers(empty.Image, layers...)
	if err != nil {
		return fmt.Errorf("remote.Push: append layers: %w", err)
	}

	// Mirror the layout backend's back-compat fallback: callers supply
	// Annotations, but if they didn't set BundleName we fill it from
	// ref.Name so round-trip Fetch via annotation-walk still works.
	annotations := ann.asMap()
	if annotations[annotationBundleName] == "" {
		annotations[annotationBundleName] = ref.Name
	}
	// Replay what `oras push --artifact-type <X>` writes on the manifest.
	annotations[annotationArtifactType] = artifactTypeForBundle(ref.Name)
	annotated, ok := mutate.Annotations(img, annotations).(v1.Image)
	if !ok {
		return errors.New("remote.Push: mutate.Annotations did not return v1.Image")
	}

	target, err := name.ParseReference(fmt.Sprintf("%s/%s:%s", s.base, ref.Name, ref.Tag))
	if err != nil {
		return fmt.Errorf("remote.Push: parse target %q: %w", ref, err)
	}
	if err := remote.Write(target, annotated, remote.WithAuth(s.auth), remote.WithContext(ctx)); err != nil {
		return fmt.Errorf("remote.Push %s: %w", ref, err)
	}
	return nil
}

// LookupBySpec is not supported on the remote backend: anonymous OCI
// registries don't expose an index walk for annotation-keyed probes.
// Callers should use an oci-layout registry as their cache store and
// publish to the remote only after a fresh build succeeds. Build
// callers that want "no cache => build anyway" semantics should
// treat ErrUnsupported as a soft miss:
//
//	if errors.Is(err, ErrUnsupported) {
//	    hit, err = false, nil // no cache available on this backend
//	}
func (s *remoteStore) LookupBySpec(_ context.Context, _, _ string) (Ref, bool, error) {
	return Ref{}, false, ErrUnsupported
}

func (s *remoteStore) ResolveDigest(ctx context.Context, reference string) (string, error) {
	ref, err := name.ParseReference(reference)
	if err != nil {
		return "", fmt.Errorf("remote.ResolveDigest %q: %w", reference, err)
	}
	desc, err := remote.Head(ref, remote.WithAuth(s.auth), remote.WithContext(ctx))
	if err != nil {
		return "", fmt.Errorf("remote.ResolveDigest %q: %w", reference, err)
	}
	return desc.Digest.String(), nil
}

var _ Store = (*remoteStore)(nil)
