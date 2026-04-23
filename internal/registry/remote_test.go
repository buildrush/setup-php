package registry

import (
	"bytes"
	"context"
	"errors"
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

func TestRemoteStore_PushReturnsUnsupported(t *testing.T) {
	ctx := context.Background()
	host := startTestRegistry(t)
	s, _ := Open(host + "/buildrush")
	err := s.Push(ctx, Ref{Name: "php-core"}, bytes.NewReader([]byte("x")), nil, Annotations{BundleName: "php-core"})
	if !errors.Is(err, ErrUnsupported) {
		t.Fatalf("Push err = %v, want ErrUnsupported", err)
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
