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

// seedKeyedLayoutForPush writes a bundle into a fresh oci-layout at
// <dir>/layout with full annotations (name + key + spec_hash) so the
// PushAll path can recover the canonical tag from the key. Mirrors what
// `phpup build php|ext` writes for produced bundles.
func seedKeyedLayoutForPush(t *testing.T, dir, bundleName, bundleKey, specHash string, body []byte, meta *registry.Meta) string {
	t.Helper()
	layoutDir := filepath.Join(dir, "layout")
	s, err := registry.Open("oci-layout:" + layoutDir)
	if err != nil {
		t.Fatalf("open layout: %v", err)
	}
	if err := s.Push(context.Background(),
		registry.Ref{Name: bundleName},
		bytes.NewReader(body), meta,
		registry.Annotations{BundleName: bundleName, BundleKey: bundleKey, SpecHash: specHash}); err != nil {
		t.Fatalf("seed layout: %v", err)
	}
	return "oci-layout:" + layoutDir
}

// TestPushAll_RoundTrip_LayoutToInProcessRegistry is the happy-path
// end-to-end: seed an oci-layout with two annotated manifests (php-core +
// one ext), run PushAll against an in-process registry as destination,
// and verify both manifests are fetchable on the destination under the
// canonical publication tag derived from the bundle key. Also verifies
// that BundleKey and SpecHash annotations are propagated.
func TestPushAll_RoundTrip_LayoutToInProcessRegistry(t *testing.T) {
	ctx := context.Background()
	tmp := t.TempDir()
	phpCoreBytes := []byte("fake php-core bundle")
	phpExtBytes := []byte("fake php-ext-redis bundle")

	const phpKey = "php:8.5:linux:x86_64:nts"
	const phpHash = "sha256:phpspecdigesthex0000000000000000000000000000000000000000000000"
	const extKey = "ext:redis:6.3.0:8.5:linux:x86_64:nts"
	const extHash = "sha256:extspecdigesthex0000000000000000000000000000000000000000000000"

	// Seed source layout with two keyed bundles.
	layoutURI := seedKeyedLayoutForPush(t, tmp, "php-core", phpKey, phpHash, phpCoreBytes,
		&registry.Meta{SchemaVersion: 3, Kind: "php-core"})
	layoutDir := filepath.Join(tmp, "layout")
	s, _ := registry.Open("oci-layout:" + layoutDir)
	if err := s.Push(ctx, registry.Ref{Name: "php-ext-redis"},
		bytes.NewReader(phpExtBytes),
		&registry.Meta{SchemaVersion: 3, Kind: "php-ext"},
		registry.Annotations{BundleName: "php-ext-redis", BundleKey: extKey, SpecHash: extHash}); err != nil {
		t.Fatalf("push ext: %v", err)
	}

	host := startPushTestRegistry(t)
	destURI := host + "/buildrush"

	if err := PushAll(ctx, layoutURI, destURI); err != nil {
		t.Fatalf("PushAll: %v", err)
	}

	// Each seeded bundle should land at the canonical tag derived from
	// its bundle key — the same tag shape `lockfile-update` queries.
	cases := []struct {
		key      string
		wantName string
		wantTag  string
		wantBody []byte
	}{
		{phpKey, "php-core", "8.5-linux-x86_64-nts", phpCoreBytes},
		{extKey, "php-ext-redis", "6.3.0-8.5-nts-linux-x86_64", phpExtBytes},
	}
	for _, tc := range cases {
		t.Run(tc.wantName, func(t *testing.T) {
			dest, err := name.ParseReference(destURI + "/" + tc.wantName + ":" + tc.wantTag)
			if err != nil {
				t.Fatalf("parse dest ref: %v", err)
			}
			desc, err := remote.Get(dest)
			if err != nil {
				t.Fatalf("remote.Get %s: %v (canonical tag missing — phpup push regression)", dest, err)
			}
			img, err := desc.Image()
			if err != nil {
				t.Fatalf("image: %v", err)
			}
			// Verify layer 0 bytes match the seeded body.
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
			if !bytes.Equal(got, tc.wantBody) {
				t.Errorf("bundle %s: got %q, want %q", tc.wantName, got, tc.wantBody)
			}
			// Verify destination annotations propagated.
			mf, err := img.Manifest()
			if err != nil {
				t.Fatalf("manifest: %v", err)
			}
			if got := mf.Annotations["io.buildrush.bundle.key"]; got != tc.key {
				t.Errorf("dest bundle.key = %q, want %q", got, tc.key)
			}
			if got := mf.Annotations["io.buildrush.bundle.name"]; got != tc.wantName {
				t.Errorf("dest bundle.name = %q, want %q", got, tc.wantName)
			}
			if got := mf.Annotations["io.buildrush.bundle.spec-hash"]; got == "" {
				t.Errorf("dest bundle.spec-hash missing (want propagated from source)")
			}
		})
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
	if !strings.Contains(err.Error(), "doesn't support ListKeyed") {
		t.Errorf("err = %v, want containing \"doesn't support ListKeyed\"", err)
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
// holds zero manifests is a legal no-op (symmetry with ListKeyed's
// ErrNotExist handling). Covers the case where PushAll is invoked
// against a fresh build directory before any bundles have been pushed.
// Must print 0-manifests banner and return nil.
func TestPushMain_EmptyLayoutSucceeds(t *testing.T) {
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

// TestCanonicalRefFromBundleKey covers every key shape the helper accepts
// plus the rejection branch. The expected tag formats here are the same
// strings `lockfile-update` queries against ghcr.io — keep this test in
// lockstep with `internal/lockfileupdate.update.go`'s tag construction.
func TestCanonicalRefFromBundleKey(t *testing.T) {
	cases := []struct {
		name       string
		key        string
		wantName   string
		wantTag    string
		wantErrSub string
	}{
		{
			name:     "php core 8.5 linux x86_64 nts",
			key:      "php:8.5:linux:x86_64:nts",
			wantName: "php-core",
			wantTag:  "8.5-linux-x86_64-nts",
		},
		{
			name:     "php core 8.4 linux aarch64 nts",
			key:      "php:8.4:linux:aarch64:nts",
			wantName: "php-core",
			wantTag:  "8.4-linux-aarch64-nts",
		},
		{
			name:     "ext redis 6.3.0",
			key:      "ext:redis:6.3.0:8.5:linux:x86_64:nts",
			wantName: "php-ext-redis",
			wantTag:  "6.3.0-8.5-nts-linux-x86_64",
		},
		{
			name:     "ext xdebug 3.5.1",
			key:      "ext:xdebug:3.5.1:8.4:linux:aarch64:nts",
			wantName: "php-ext-xdebug",
			wantTag:  "3.5.1-8.4-nts-linux-aarch64",
		},
		{
			name:       "tool key not supported",
			key:        "tool:composer:2.7.0:any:any",
			wantErrSub: "unrecognized bundle key shape",
		},
		{
			name:       "wrong prefix",
			key:        "container:redis:7.0",
			wantErrSub: "unrecognized bundle key shape",
		},
		{
			name:       "ext with too few parts",
			key:        "ext:redis:6.3.0:8.5:linux",
			wantErrSub: "unrecognized bundle key shape",
		},
		{
			name:       "empty",
			key:        "",
			wantErrSub: "unrecognized bundle key shape",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotName, gotTag, err := canonicalRefFromBundleKey(tc.key)
			if tc.wantErrSub != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErrSub) {
					t.Errorf("err = %v, want containing %q", err, tc.wantErrSub)
				}
				return
			}
			if err != nil {
				t.Fatalf("err = %v, want nil", err)
			}
			if gotName != tc.wantName {
				t.Errorf("name = %q, want %q", gotName, tc.wantName)
			}
			if gotTag != tc.wantTag {
				t.Errorf("tag = %q, want %q", gotTag, tc.wantTag)
			}
		})
	}
}
