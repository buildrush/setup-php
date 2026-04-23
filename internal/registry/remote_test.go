package registry

import (
	"bytes"
	"context"
	"io"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/registry"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/static"
	"github.com/google/go-containerregistry/pkg/v1/types"
)

// startTestRegistry spins up an in-process OCI registry and returns its
// host:port. It's torn down by t.Cleanup.
func startTestRegistry(t *testing.T) string {
	t.Helper()
	srv := httptest.NewServer(registry.New())
	t.Cleanup(srv.Close)
	u, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatalf("parse server URL: %v", err)
	}
	return u.Host
}

// seedTestRegistry pushes a two-layer image (bundle + meta.json) to the
// registry at path "<host>/buildrush/php-core:seed". Returns the manifest digest.
func seedTestRegistry(t *testing.T, host string, bundle, metaJSON []byte) string {
	t.Helper()
	ref, err := name.ParseReference(host + "/buildrush/php-core:seed")
	if err != nil {
		t.Fatalf("parse ref: %v", err)
	}
	bundleLayer := static.NewLayer(bundle, types.OCILayer)
	img := empty.Image
	img, err = mutate.AppendLayers(img, bundleLayer)
	if err != nil {
		t.Fatalf("append bundle: %v", err)
	}
	if metaJSON != nil {
		metaLayer := static.NewLayer(metaJSON, types.OCILayer)
		img, err = mutate.AppendLayers(img, metaLayer)
		if err != nil {
			t.Fatalf("append meta: %v", err)
		}
	}
	if err := remote.Write(ref, img); err != nil {
		t.Fatalf("seed registry: %v", err)
	}
	d, err := img.Digest()
	if err != nil {
		t.Fatalf("digest: %v", err)
	}
	return d.String()
}

func TestRemoteStore_FetchSeededImage(t *testing.T) {
	ctx := context.Background()
	host := startTestRegistry(t)
	payload := []byte("remote bundle payload")
	metaJSON := []byte(`{"schema_version":2,"kind":"php-core"}`)
	digest := seedTestRegistry(t, host, payload, metaJSON)

	s, err := Open(host + "/buildrush")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if s.Kind() != "remote" {
		t.Fatalf("Kind = %q, want remote", s.Kind())
	}

	rc, meta, err := s.Fetch(ctx, Ref{Name: "php-core", Digest: digest})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	defer rc.Close()
	got, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatalf("payload = %q, want %q", got, payload)
	}
	if meta == nil || meta.SchemaVersion != 2 || meta.Kind != "php-core" {
		t.Fatalf("meta = %+v", meta)
	}
}

func TestRemoteStore_FetchWithoutMetaLayer(t *testing.T) {
	ctx := context.Background()
	host := startTestRegistry(t)
	payload := []byte("legacy-bundle-no-meta")
	digest := seedTestRegistry(t, host, payload, nil) // no meta layer

	s, _ := Open(host + "/buildrush")
	rc, meta, err := s.Fetch(ctx, Ref{Name: "php-core", Digest: digest})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	defer rc.Close()
	got, _ := io.ReadAll(rc)
	if !bytes.Equal(got, payload) {
		t.Fatalf("payload mismatch")
	}
	if meta == nil || meta.SchemaVersion != 1 {
		t.Fatalf("meta = %+v, want SchemaVersion:1 (legacy default)", meta)
	}
}

func TestRemoteStore_HasSeededImage(t *testing.T) {
	ctx := context.Background()
	host := startTestRegistry(t)
	digest := seedTestRegistry(t, host, []byte("x"), nil)
	s, _ := Open(host + "/buildrush")
	has, err := s.Has(ctx, Ref{Name: "php-core", Digest: digest})
	if err != nil {
		t.Fatalf("Has: %v", err)
	}
	if !has {
		t.Fatal("Has returned false for seeded ref")
	}
}

func TestRemoteStore_HasMissingRef_FalseNoError(t *testing.T) {
	ctx := context.Background()
	host := startTestRegistry(t)
	s, _ := Open(host + "/buildrush")
	missing := Ref{Name: "php-core", Digest: "sha256:" + strings.Repeat("0", 64)}
	has, err := s.Has(ctx, missing)
	if err != nil {
		t.Fatalf("Has on missing ref: err = %v, want nil (404 should be \"not present\", not an error)", err)
	}
	if has {
		t.Fatal("Has returned true on empty registry")
	}
}

// TestRemoteStore_Push_RoundTrip exercises the full Push→ResolveDigest→Fetch
// path against the in-process test registry. It asserts the bundle bytes and
// the Meta sidecar survive a round-trip, proving remote.Push emits an OCI
// image shape that the existing Fetch path can consume.
func TestRemoteStore_Push_RoundTrip(t *testing.T) {
	ctx := context.Background()
	host := startTestRegistry(t)
	s, err := Open(host + "/buildrush")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	payload := []byte("roundtrip-payload")
	meta := &Meta{SchemaVersion: 2, Kind: "php-core"}
	if err := s.Push(ctx,
		Ref{Name: "php-core", Tag: "roundtrip-tag"},
		bytes.NewReader(payload), meta,
		Annotations{BundleName: "php-core", SpecHash: "sha256:abc"}); err != nil {
		t.Fatalf("Push: %v", err)
	}

	digest, err := s.ResolveDigest(ctx, host+"/buildrush/php-core:roundtrip-tag")
	if err != nil {
		t.Fatalf("ResolveDigest: %v", err)
	}

	rc, gotMeta, err := s.Fetch(ctx, Ref{Name: "php-core", Digest: digest})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	defer rc.Close()
	gotBytes, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !bytes.Equal(gotBytes, payload) {
		t.Errorf("payload = %q, want %q", gotBytes, payload)
	}
	if gotMeta == nil || gotMeta.SchemaVersion != 2 || gotMeta.Kind != "php-core" {
		t.Errorf("meta = %+v, want SchemaVersion:2 Kind:php-core", gotMeta)
	}
}

// TestRemoteStore_Push_RequiresTag guards the design invariant: remote
// registries are tag-addressed for writes, so Push must refuse a digest-only
// (or empty) Ref with an error mentioning Tag.
func TestRemoteStore_Push_RequiresTag(t *testing.T) {
	ctx := context.Background()
	host := startTestRegistry(t)
	s, _ := Open(host + "/buildrush")

	err := s.Push(ctx, Ref{Name: "php-core"}, // intentionally no Tag
		bytes.NewReader([]byte("x")), nil, Annotations{BundleName: "php-core"})
	if err == nil {
		t.Fatal("Push with empty Tag err = nil, want error")
	}
	if !strings.Contains(err.Error(), "Tag") {
		t.Errorf("Push err = %q, want mention of Tag", err)
	}
}

// TestRemoteStore_Push_ArtifactTypeAnnotation_PhpExt verifies the manifest
// annotation matches what `oras push --artifact-type application/vnd.buildrush.php-ext.v1`
// emits for php-ext-* bundles (see .github/workflows/build-extension.yml).
func TestRemoteStore_Push_ArtifactTypeAnnotation_PhpExt(t *testing.T) {
	ctx := context.Background()
	host := startTestRegistry(t)
	s, _ := Open(host + "/buildrush")

	ref := Ref{Name: "php-ext-redis", Tag: "6.2.0-8.4-nts-linux-x86_64"}
	if err := s.Push(ctx, ref,
		bytes.NewReader([]byte("x")), nil,
		Annotations{BundleName: "php-ext-redis"}); err != nil {
		t.Fatalf("Push: %v", err)
	}

	target, err := name.ParseReference(host + "/buildrush/" + ref.Name + ":" + ref.Tag)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	desc, err := remote.Get(target)
	if err != nil {
		t.Fatalf("remote.Get: %v", err)
	}
	img, err := desc.Image()
	if err != nil {
		t.Fatalf("image: %v", err)
	}
	mf, err := img.Manifest()
	if err != nil {
		t.Fatalf("manifest: %v", err)
	}
	if got := mf.Annotations["org.opencontainers.artifact.type"]; got != "application/vnd.buildrush.php-ext.v1" {
		t.Errorf("artifact.type = %q, want %q", got, "application/vnd.buildrush.php-ext.v1")
	}
}

// TestRemoteStore_Push_ArtifactTypeAnnotation_PhpCore mirrors the php-ext
// assertion for the php-core artifact type (build-php-core.yml).
func TestRemoteStore_Push_ArtifactTypeAnnotation_PhpCore(t *testing.T) {
	ctx := context.Background()
	host := startTestRegistry(t)
	s, _ := Open(host + "/buildrush")

	ref := Ref{Name: "php-core", Tag: "8.4-linux-x86_64-nts"}
	if err := s.Push(ctx, ref,
		bytes.NewReader([]byte("x")), nil,
		Annotations{BundleName: "php-core"}); err != nil {
		t.Fatalf("Push: %v", err)
	}

	target, err := name.ParseReference(host + "/buildrush/" + ref.Name + ":" + ref.Tag)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	desc, err := remote.Get(target)
	if err != nil {
		t.Fatalf("remote.Get: %v", err)
	}
	img, err := desc.Image()
	if err != nil {
		t.Fatalf("image: %v", err)
	}
	mf, err := img.Manifest()
	if err != nil {
		t.Fatalf("manifest: %v", err)
	}
	if got := mf.Annotations["org.opencontainers.artifact.type"]; got != "application/vnd.buildrush.php-core.v1" {
		t.Errorf("artifact.type = %q, want %q", got, "application/vnd.buildrush.php-core.v1")
	}
}

// TestRemoteStore_Push_LayerMediaTypes verifies the bundle and meta layer
// media types are byte-identical to what `oras push` writes: the bundle at
// application/vnd.oci.image.layer.v1.tar+zstd and the meta sidecar at
// application/vnd.buildrush.meta.v1+json.
func TestRemoteStore_Push_LayerMediaTypes(t *testing.T) {
	ctx := context.Background()
	host := startTestRegistry(t)
	s, _ := Open(host + "/buildrush")

	ref := Ref{Name: "php-core", Tag: "media-types"}
	if err := s.Push(ctx, ref,
		bytes.NewReader([]byte("bundle-bytes")), &Meta{SchemaVersion: 2, Kind: "php-core"},
		Annotations{BundleName: "php-core"}); err != nil {
		t.Fatalf("Push: %v", err)
	}

	target, err := name.ParseReference(host + "/buildrush/" + ref.Name + ":" + ref.Tag)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	desc, err := remote.Get(target)
	if err != nil {
		t.Fatalf("remote.Get: %v", err)
	}
	img, err := desc.Image()
	if err != nil {
		t.Fatalf("image: %v", err)
	}
	mf, err := img.Manifest()
	if err != nil {
		t.Fatalf("manifest: %v", err)
	}
	if len(mf.Layers) != 2 {
		t.Fatalf("len(layers) = %d, want 2", len(mf.Layers))
	}
	if got := mf.Layers[0].MediaType; got != "application/vnd.oci.image.layer.v1.tar+zstd" {
		t.Errorf("layers[0].MediaType = %q, want %q", got, "application/vnd.oci.image.layer.v1.tar+zstd")
	}
	if got := mf.Layers[1].MediaType; got != "application/vnd.buildrush.meta.v1+json" {
		t.Errorf("layers[1].MediaType = %q, want %q", got, "application/vnd.buildrush.meta.v1+json")
	}
}

func TestRemoteStore_HasEmptyDigest_FalseNoError(t *testing.T) {
	// Symmetry with layoutStore.Has: a probe with empty digest is a legal
	// "not present" query across both backends (see Store.Has docstring).
	ctx := context.Background()
	host := startTestRegistry(t)
	s, _ := Open(host + "/buildrush")
	has, err := s.Has(ctx, Ref{Name: "php-core"}) // Digest intentionally empty
	if err != nil {
		t.Fatalf("Has empty-digest: err = %v, want nil (symmetry with layout backend)", err)
	}
	if has {
		t.Fatal("Has empty-digest returned true on empty registry")
	}
}

func TestRemoteStore_ResolveDigestReturnsSeededDigest(t *testing.T) {
	ctx := context.Background()
	host := startTestRegistry(t)
	digest := seedTestRegistry(t, host, []byte("x"), nil)
	s, _ := Open(host + "/buildrush")
	got, err := s.ResolveDigest(ctx, host+"/buildrush/php-core:seed")
	if err != nil {
		t.Fatalf("ResolveDigest: %v", err)
	}
	if got != digest {
		t.Fatalf("ResolveDigest = %q, want %q", got, digest)
	}
}
