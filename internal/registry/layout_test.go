package registry

import (
	"bytes"
	"context"
	"io"
	"path/filepath"
	"strings"
	"testing"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/layout"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/static"
	"github.com/google/go-containerregistry/pkg/v1/types"
)

func newTestLayoutStore(t *testing.T) *layoutStore {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "layout")
	s, err := openLayout(dir)
	if err != nil {
		t.Fatalf("openLayout: %v", err)
	}
	return s
}

func TestLayoutStore_RoundTrip(t *testing.T) {
	ctx := context.Background()
	s := newTestLayoutStore(t)

	payload := []byte("fake bundle bytes — in reality this would be bundle.tar.zst")
	meta := &Meta{SchemaVersion: 2, Kind: "php-core"}

	// Before Push, Has must return false.
	ref := Ref{Name: "php-core"} // digest filled in by Push
	has, err := s.Has(ctx, ref)
	if err != nil {
		t.Fatalf("Has before Push: %v", err)
	}
	if has {
		t.Fatal("Has returned true on empty layout")
	}

	if err := s.Push(ctx, ref, bytes.NewReader(payload), meta); err != nil {
		t.Fatalf("Push: %v", err)
	}

	// Walk the index to find the just-pushed digest, then assert Has and Fetch.
	got, err := s.list(ctx)
	if err != nil {
		t.Fatalf("list after Push: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("list after Push: got %d entries, want 1", len(got))
	}
	stored := got[0]
	if stored.Name != "php-core" {
		t.Fatalf("stored.Name = %q, want %q", stored.Name, "php-core")
	}
	if stored.Digest == "" {
		t.Fatal("stored.Digest empty")
	}

	has, err = s.Has(ctx, stored)
	if err != nil {
		t.Fatalf("Has after Push: %v", err)
	}
	if !has {
		t.Fatal("Has returned false after Push")
	}

	rc, metaOut, err := s.Fetch(ctx, stored)
	if err != nil {
		t.Fatalf("Fetch after Push: %v", err)
	}
	defer rc.Close()
	gotBytes, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("read bundle bytes: %v", err)
	}
	if !bytes.Equal(gotBytes, payload) {
		t.Fatalf("bundle bytes mismatch: got %q, want %q", gotBytes, payload)
	}
	if metaOut == nil || metaOut.SchemaVersion != 2 || metaOut.Kind != "php-core" {
		t.Fatalf("meta mismatch: got %+v, want {SchemaVersion:2 Kind:php-core}", metaOut)
	}
}

func TestLayoutStore_FetchMissingRef_Errors(t *testing.T) {
	ctx := context.Background()
	s := newTestLayoutStore(t)
	// Use a valid-form digest so future refactors that start parsing digests
	// via v1.NewHash don't regress this test silently.
	emptyLayoutRef := Ref{Name: "php-core", Digest: "sha256:" + strings.Repeat("0", 64)}
	if _, _, err := s.Fetch(ctx, emptyLayoutRef); err == nil {
		t.Fatal("Fetch on empty layout: want error, got nil")
	}

	// Populated layout, wrong digest — must still error (exercises the
	// "not found in index" branch, not just the open() failure).
	if err := s.Push(ctx, Ref{Name: "php-core"}, bytes.NewReader([]byte("x")), nil); err != nil {
		t.Fatalf("Push: %v", err)
	}
	missing := Ref{Name: "php-core", Digest: "sha256:" + strings.Repeat("0", 64)}
	if _, _, err := s.Fetch(ctx, missing); err == nil {
		t.Fatal("Fetch with wrong digest in populated layout: want error, got nil")
	}
}

// pushImageWithoutAnnotation simulates an `oras copy`-style append: it writes
// a single-layer OCI image into the layout WITHOUT the io.buildrush.bundle.name
// annotation, so Has/Fetch must rely on the digest-only fallback path.
func pushImageWithoutAnnotation(t *testing.T, s *layoutStore, payload []byte) v1.Hash {
	t.Helper()
	img, err := mutate.AppendLayers(empty.Image, static.NewLayer(payload, types.OCILayer))
	if err != nil {
		t.Fatalf("append layer: %v", err)
	}
	p, err := s.openOrInit()
	if err != nil {
		t.Fatalf("openOrInit: %v", err)
	}
	if err := p.AppendImage(img); err != nil {
		t.Fatalf("AppendImage: %v", err)
	}
	d, err := img.Digest()
	if err != nil {
		t.Fatalf("Digest: %v", err)
	}
	return d
}

// indexEntryForName returns the digest of the manifest that Push tagged with
// the given Ref.Name annotation. Fails the test if no such entry exists.
func indexEntryForName(t *testing.T, s *layoutStore, name string) string {
	t.Helper()
	p, err := layout.FromPath(s.root)
	if err != nil {
		t.Fatalf("FromPath: %v", err)
	}
	idx, err := p.ImageIndex()
	if err != nil {
		t.Fatalf("ImageIndex: %v", err)
	}
	m, err := idx.IndexManifest()
	if err != nil {
		t.Fatalf("IndexManifest: %v", err)
	}
	for i := range m.Manifests {
		if m.Manifests[i].Annotations[annotationBundleName] == name {
			return m.Manifests[i].Digest.String()
		}
	}
	t.Fatalf("no manifest tagged %q in index", name)
	return ""
}

func TestLayoutStore_DigestOnlyFallback_PrefersExactAnnotationMatch(t *testing.T) {
	ctx := context.Background()
	s := newTestLayoutStore(t)

	// Two pushes with distinct payloads → distinct digests.
	if err := s.Push(ctx, Ref{Name: "php-core"}, bytes.NewReader([]byte("core-bytes")), nil); err != nil {
		t.Fatalf("Push core: %v", err)
	}
	if err := s.Push(ctx, Ref{Name: "php-ext-redis"}, bytes.NewReader([]byte("redis-bytes")), nil); err != nil {
		t.Fatalf("Push redis: %v", err)
	}

	coreDigest := indexEntryForName(t, s, "php-core")
	redisDigest := indexEntryForName(t, s, "php-ext-redis")
	if coreDigest == redisDigest {
		t.Fatalf("expected distinct digests, got %q", coreDigest)
	}

	// Correct Name+Digest → must fetch the right bundle.
	rc, _, err := s.Fetch(ctx, Ref{Name: "php-core", Digest: coreDigest})
	if err != nil {
		t.Fatalf("Fetch(core,coreDigest): %v", err)
	}
	got, _ := io.ReadAll(rc)
	_ = rc.Close()
	if !bytes.Equal(got, []byte("core-bytes")) {
		t.Fatalf("bundle bytes = %q, want %q", got, "core-bytes")
	}

	// Wrong Name + correct-Digest-of-another-manifest → affirmative negative.
	has, err := s.Has(ctx, Ref{Name: "php-core", Digest: redisDigest})
	if err != nil {
		t.Fatalf("Has(core,redisDigest): %v", err)
	}
	if has {
		t.Fatal("Has(core,redisDigest) = true; want false (annotation mismatch)")
	}
	if _, _, err := s.Fetch(ctx, Ref{Name: "php-core", Digest: redisDigest}); err == nil {
		t.Fatal("Fetch(core,redisDigest): want error, got nil")
	}
}

func TestLayoutStore_DigestOnlyFallback_AcceptsManifestWithoutAnnotation(t *testing.T) {
	ctx := context.Background()
	s := newTestLayoutStore(t)

	payload := []byte("oras-copied-bytes")
	d := pushImageWithoutAnnotation(t, s, payload)

	// Probe with any Name — should succeed because there is no affirmative
	// "wrong name" signal on this manifest (annotation absent).
	ref := Ref{Name: "whatever", Digest: d.String()}
	has, err := s.Has(ctx, ref)
	if err != nil {
		t.Fatalf("Has: %v", err)
	}
	if !has {
		t.Fatal("Has on un-annotated manifest = false; want true (digest-only fallback)")
	}
	rc, _, err := s.Fetch(ctx, ref)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	got, _ := io.ReadAll(rc)
	_ = rc.Close()
	if !bytes.Equal(got, payload) {
		t.Fatalf("bundle bytes = %q, want %q", got, payload)
	}
}

func TestLayoutStore_DigestOnlyFallback_RejectsManifestWithWrongAnnotation(t *testing.T) {
	ctx := context.Background()
	s := newTestLayoutStore(t)

	if err := s.Push(ctx, Ref{Name: "php-ext-redis"}, bytes.NewReader([]byte("redis-bytes")), nil); err != nil {
		t.Fatalf("Push: %v", err)
	}
	redisDigest := indexEntryForName(t, s, "php-ext-redis")

	// Probe with a different Name but the redis manifest's digest. The
	// manifest's annotation affirmatively says it's redis, so we must refuse.
	probe := Ref{Name: "php-core", Digest: redisDigest}
	has, err := s.Has(ctx, probe)
	if err != nil {
		t.Fatalf("Has: %v", err)
	}
	if has {
		t.Fatal("Has on wrong-annotation manifest = true; want false")
	}
	if _, _, err := s.Fetch(ctx, probe); err == nil {
		t.Fatal("Fetch on wrong-annotation manifest: want error, got nil")
	}
}

func TestLayoutStore_TolerateMissingMeta(t *testing.T) {
	// A bundle pushed without a meta sidecar must Fetch back with a
	// default Meta{SchemaVersion:1} — matching the legacy behaviour the
	// runtime already tolerates (see internal/oci/client.go Fetch path).
	ctx := context.Background()
	s := newTestLayoutStore(t)
	payload := []byte("legacy-bundle")
	if err := s.Push(ctx, Ref{Name: "php-core"}, bytes.NewReader(payload), nil); err != nil {
		t.Fatalf("Push nil meta: %v", err)
	}
	got, err := s.list(ctx)
	if err != nil || len(got) != 1 {
		t.Fatalf("list: %v / %d", err, len(got))
	}
	_, meta, err := s.Fetch(ctx, got[0])
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if meta == nil || meta.SchemaVersion != 1 {
		t.Fatalf("meta = %+v, want {SchemaVersion:1}", meta)
	}
}
