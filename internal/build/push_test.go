package build

import (
	"bytes"
	"context"
	"io"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-containerregistry/pkg/name"
	gcrregistry "github.com/google/go-containerregistry/pkg/registry"
	"github.com/google/go-containerregistry/pkg/v1/remote"

	"github.com/buildrush/setup-php/internal/registry"
)

// startPushTestRegistry spins up an in-process OCI registry and returns
// its host:port. Mirrors the helper in internal/registry/remote_test.go
// but reproduced here so internal/build can exercise the full
// PushAll → remoteStore path without a cross-package import cycle.
func startPushTestRegistry(t *testing.T) string {
	t.Helper()
	srv := httptest.NewServer(gcrregistry.New())
	t.Cleanup(srv.Close)
	u, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatalf("parse server URL: %v", err)
	}
	return u.Host
}

// seedLayoutForPush writes a bundle into a fresh oci-layout at
// <dir>/layout, then returns the URI. Used to populate a source layout
// for PushAll round-trip tests.
func seedLayoutForPush(t *testing.T, dir, bundleName string, body []byte, meta *registry.Meta) string {
	t.Helper()
	layoutDir := filepath.Join(dir, "layout")
	s, err := registry.Open("oci-layout:" + layoutDir)
	if err != nil {
		t.Fatalf("open layout: %v", err)
	}
	if err := s.Push(context.Background(),
		registry.Ref{Name: bundleName},
		bytes.NewReader(body), meta,
		registry.Annotations{BundleName: bundleName}); err != nil {
		t.Fatalf("seed layout: %v", err)
	}
	return "oci-layout:" + layoutDir
}

// TestPushAll_RoundTrip_LayoutToInProcessRegistry is the happy-path
// end-to-end: seed an oci-layout with two manifests (php-core + one
// ext), run PushAll against an in-process registry as destination, and
// verify both manifests are fetchable on the destination under the
// digest-derived tag.
func TestPushAll_RoundTrip_LayoutToInProcessRegistry(t *testing.T) {
	ctx := context.Background()
	tmp := t.TempDir()
	phpCoreBytes := []byte("fake php-core bundle")
	phpExtBytes := []byte("fake php-ext-redis bundle")

	// Seed source layout with two bundles.
	layoutURI := seedLayoutForPush(t, tmp, "php-core", phpCoreBytes, &registry.Meta{SchemaVersion: 3, Kind: "php-core"})
	// Second push lands in the same layout path.
	layoutDir := filepath.Join(tmp, "layout")
	s, _ := registry.Open("oci-layout:" + layoutDir)
	if err := s.Push(ctx, registry.Ref{Name: "php-ext-redis"},
		bytes.NewReader(phpExtBytes), &registry.Meta{SchemaVersion: 3, Kind: "php-ext"},
		registry.Annotations{BundleName: "php-ext-redis"}); err != nil {
		t.Fatalf("push ext: %v", err)
	}

	host := startPushTestRegistry(t)
	destURI := host + "/buildrush"

	if err := PushAll(ctx, layoutURI, destURI); err != nil {
		t.Fatalf("PushAll: %v", err)
	}

	// Re-open source to enumerate digest-derived tags the push would
	// have used, so we can fetch back from the destination under the
	// same tag.
	lister := s.(interface {
		List(ctx context.Context) ([]registry.Ref, error)
	})
	refs, err := lister.List(ctx)
	if err != nil {
		t.Fatalf("source List: %v", err)
	}
	if len(refs) != 2 {
		t.Fatalf("source layout has %d manifests, want 2", len(refs))
	}

	// For each seeded bundle, verify the destination has a manifest at
	// the derived tag with the same layer 0 bytes.
	byName := map[string][]byte{
		"php-core":      phpCoreBytes,
		"php-ext-redis": phpExtBytes,
	}
	for _, r := range refs {
		tag := deriveTagForPromotion(r)
		if tag == "" {
			t.Fatalf("empty derived tag for %s", r)
		}
		want, ok := byName[r.Name]
		if !ok {
			t.Fatalf("unexpected source ref %s", r)
		}
		dest, err := name.ParseReference(destURI + "/" + r.Name + ":" + tag)
		if err != nil {
			t.Fatalf("parse dest ref: %v", err)
		}
		desc, err := remote.Get(dest)
		if err != nil {
			t.Fatalf("remote.Get %s: %v", dest, err)
		}
		img, err := desc.Image()
		if err != nil {
			t.Fatalf("image: %v", err)
		}
		layers, err := img.Layers()
		if err != nil {
			t.Fatalf("layers: %v", err)
		}
		if len(layers) < 1 {
			t.Fatalf("manifest has %d layers, want >=1", len(layers))
		}
		rc, err := layers[0].Compressed()
		if err != nil {
			t.Fatalf("layer 0: %v", err)
		}
		got, _ := io.ReadAll(rc)
		_ = rc.Close()
		if !bytes.Equal(got, want) {
			t.Errorf("bundle %s: got %q, want %q", r.Name, got, want)
		}
	}
}

// TestPushAll_RemoteSourceRejected ensures a remote source (which can't
// be walked by anonymous index enumeration) trips the explicit guard
// message. The dest is permissive — failure must come from the source
// type assertion, not from network I/O.
func TestPushAll_RemoteSourceRejected(t *testing.T) {
	host := startPushTestRegistry(t)
	// Source "looks like" remote URI (has dots in the head) so Open
	// returns remoteStore rather than layoutStore.
	err := PushAll(context.Background(), host+"/source", host+"/dest")
	if err == nil {
		t.Fatal("PushAll with remote source: want error, got nil")
	}
	if !strings.Contains(err.Error(), "doesn't support index walk") {
		t.Errorf("err = %v, want containing \"doesn't support index walk\"", err)
	}
}

// TestPushMain_MissingFlags_Errors: both --from and --to are required;
// each omission must surface a clear error before any I/O.
func TestPushMain_MissingFlags_Errors(t *testing.T) {
	cases := []struct {
		name string
		args []string
	}{
		{"no flags", nil},
		{"only --from", []string{"--from", "oci-layout:/tmp/x"}},
		{"only --to", []string{"--to", "ghcr.io/buildrush"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := PushMain(tc.args)
			if err == nil || !strings.Contains(err.Error(), "--from and --to are required") {
				t.Errorf("PushMain err = %v, want required-flags error", err)
			}
		})
	}
}

// TestPushMain_EmptyLayoutSucceeds: a source layout that exists but
// holds zero manifests is a legal no-op (symmetry with
// List's ErrNotExist handling). Covers the case where PushAll is
// invoked against a fresh build directory before any bundles have been
// pushed. Must print 0-manifests banner and return nil.
func TestPushMain_EmptyLayoutSucceeds(t *testing.T) {
	// An empty directory makes layout.FromPath fail with ErrNotExist on
	// the first walk — List returns (nil, nil) for that case, so
	// PushAll should successfully iterate over zero refs.
	host := startPushTestRegistry(t)
	tmp := t.TempDir()
	err := PushMain([]string{
		"--from", "oci-layout:" + filepath.Join(tmp, "never-created"),
		"--to", host + "/buildrush",
	})
	if err != nil {
		t.Errorf("PushMain on absent layout: err = %v, want nil (empty source is a no-op)", err)
	}
}

// TestDeriveTagForPromotion_DigestShortPrefix pins the fallback tag
// format PushAll emits when the source manifest has no surface-level
// tag info. Shape: "sha256-<first-12-hex>" — matches the comment on
// deriveTagForPromotion. If a future PR threads the original tag via a
// manifest annotation, this test will start failing and should be
// updated to the new contract.
func TestDeriveTagForPromotion_DigestShortPrefix(t *testing.T) {
	cases := []struct {
		name   string
		digest string
		want   string
	}{
		{"standard sha256", "sha256:abcdef012345678901234567890abcdefabcdefabcdefabcdefabcdefabcdef", "sha256-abcdef012345"},
		{"short digest (pathological)", "sha256:abc", "sha256-abc"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := deriveTagForPromotion(registry.Ref{Digest: tc.digest})
			if got != tc.want {
				t.Errorf("deriveTagForPromotion(%q) = %q, want %q", tc.digest, got, tc.want)
			}
		})
	}
}

// TestDeriveTagForPromotion_EmptyDigest ensures the empty-digest input
// produces an empty string (caller treats that as "can't derive a tag"
// and errors at the promoteOne boundary).
func TestDeriveTagForPromotion_EmptyDigest(t *testing.T) {
	if got := deriveTagForPromotion(registry.Ref{}); got != "" {
		t.Errorf("deriveTagForPromotion(empty) = %q, want empty", got)
	}
}
