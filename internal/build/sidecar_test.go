package build

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"

	"github.com/buildrush/setup-php/internal/registry"
)

// TestSetSidecarLifecycle_SwapAndRestore verifies the package-level
// swap primitive used by tests. Two calls stack LIFO: the inner
// restore returns the lifecycle to whatever was set before the inner
// SetSidecarLifecycle call, not all the way back to the original.
func TestSetSidecarLifecycle_SwapAndRestore(t *testing.T) {
	original := currentSidecarLifecycle()
	first := &fakeSidecar{}
	second := &fakeSidecar{}

	restoreFirst := SetSidecarLifecycle(first)
	if got := currentSidecarLifecycle(); got != first {
		t.Errorf("after first swap, currentSidecarLifecycle = %T, want fakeSidecar (first)", got)
	}
	restoreSecond := SetSidecarLifecycle(second)
	if got := currentSidecarLifecycle(); got != second {
		t.Errorf("after second swap, currentSidecarLifecycle = %T, want fakeSidecar (second)", got)
	}
	restoreSecond()
	if got := currentSidecarLifecycle(); got != first {
		t.Errorf("after restoreSecond, currentSidecarLifecycle = %T, want first fake", got)
	}
	restoreFirst()
	if got := currentSidecarLifecycle(); got != original {
		t.Errorf("after restoreFirst, currentSidecarLifecycle = %T, want original", got)
	}
}

// TestDockerCmdCombined_BadBinary_ReturnsError pokes the thin
// error-wrapping path by invoking `docker` with an obviously-bogus
// subcommand. Works whether or not docker is installed because we
// just need the exec to fail — either "docker: executable not found"
// or "docker: 'bogus-subcommand' is not a docker command" both
// satisfy the assertion.
func TestDockerCmdCombined_BadBinary_ReturnsError(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	err := dockerCmdCombined(ctx, "totally-bogus-subcommand-"+strings.Repeat("x", 8))
	if err == nil {
		t.Fatal("dockerCmdCombined on bogus args returned nil, want error")
	}
	if !strings.Contains(err.Error(), "docker") {
		t.Errorf("err = %v, want containing \"docker\"", err)
	}
}

// TestWaitForRegistry_UnreachableHost_ExpiresPromptlyUnderCancel
// asserts that context cancellation short-circuits the 30-second
// polling window. Without cancellation this would block for 30s; we
// give the poll 500ms of head-start, then cancel and require the
// function to return within a second.
func TestWaitForRegistry_UnreachableHost_ExpiresPromptlyUnderCancel(t *testing.T) {
	// A port that's never listening on loopback. 127.0.0.1:1 is the
	// standard "you will not get a connection here" choice.
	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() { errCh <- waitForRegistry(ctx, "127.0.0.1:1") }()
	time.Sleep(300 * time.Millisecond)
	cancel()
	select {
	case err := <-errCh:
		if err == nil {
			t.Error("waitForRegistry returned nil after cancel, want error")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("waitForRegistry did not return within 2s of cancel")
	}
}

// TestBuildTwoLayerImage_IncludesMetaWhenSet asserts the two-layer
// shape buildTwoLayerImage produces: layer 0 is the bundle bytes,
// layer 1 is the marshalled meta. This matches layoutStore.Push's
// shape (see internal/registry/layout.go) so oras-pulling from a
// sidecar yields identical bytes to a direct source.Fetch.
func TestBuildTwoLayerImage_IncludesMetaWhenSet(t *testing.T) {
	meta := &registry.Meta{SchemaVersion: 3, Kind: "php-core"}
	img, err := buildTwoLayerImage([]byte("bundle-bytes"), meta)
	if err != nil {
		t.Fatalf("buildTwoLayerImage: %v", err)
	}
	layers, err := img.Layers()
	if err != nil {
		t.Fatalf("Layers: %v", err)
	}
	if len(layers) != 2 {
		t.Fatalf("len(layers) = %d, want 2", len(layers))
	}
	rc0, err := layers[0].Compressed()
	if err != nil {
		t.Fatalf("Compressed[0]: %v", err)
	}
	defer func() { _ = rc0.Close() }()
	got0, _ := io.ReadAll(rc0)
	if !bytes.Equal(got0, []byte("bundle-bytes")) {
		t.Errorf("layer[0] = %q, want %q", got0, "bundle-bytes")
	}
	rc1, err := layers[1].Compressed()
	if err != nil {
		t.Fatalf("Compressed[1]: %v", err)
	}
	defer func() { _ = rc1.Close() }()
	got1, _ := io.ReadAll(rc1)
	parsed := &registry.Meta{}
	if err := json.Unmarshal(got1, parsed); err != nil {
		t.Fatalf("parse meta layer: %v", err)
	}
	if parsed.SchemaVersion != 3 || parsed.Kind != "php-core" {
		t.Errorf("meta = %+v, want {3 php-core}", parsed)
	}
}

// TestBuildTwoLayerImage_NilMeta_OneLayer verifies that omitting meta
// (nil) produces a single-layer image. Matches layoutStore.Push's
// legacy-bundle shape so seeding from a store that returns nil Meta
// still round-trips.
func TestBuildTwoLayerImage_NilMeta_OneLayer(t *testing.T) {
	img, err := buildTwoLayerImage([]byte("only-bundle"), nil)
	if err != nil {
		t.Fatalf("buildTwoLayerImage: %v", err)
	}
	layers, err := img.Layers()
	if err != nil {
		t.Fatalf("Layers: %v", err)
	}
	if len(layers) != 1 {
		t.Fatalf("len(layers) = %d, want 1", len(layers))
	}
}

// TestSidecar_LifecycleAndSeed_Real is the only test in this file that
// exercises real docker + a real distribution:3 container. Skipped
// under -short and when docker is absent, so CI without docker is
// unaffected. Pulls distribution:3 on first run (~50MB); subsequent
// runs hit the local image cache.
func TestSidecar_LifecycleAndSeed_Real(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping real sidecar smoke under -short")
	}
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skipf("docker not found in PATH: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	sc, stop, err := defaultSidecarLifecycle{}.Start(ctx)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() {
		stopCtx, scancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer scancel()
		_ = stop(stopCtx)
	}()

	// Shape check on the sidecar fields.
	if sc.Name == "" || sc.Network == "" || sc.InNetworkHost == "" || sc.HostHost == "" {
		t.Fatalf("sidecar has empty fields: %+v", sc)
	}
	if !strings.HasPrefix(sc.HostHost, "127.0.0.1:") {
		t.Errorf("HostHost = %q, want 127.0.0.1:<port>", sc.HostHost)
	}

	// Seed a fake bundle from a temp layout.
	sourceDir := filepath.Join(t.TempDir(), "layout")
	source, err := registry.Open("oci-layout:" + sourceDir)
	if err != nil {
		t.Fatalf("open source layout: %v", err)
	}
	bundlePayload := []byte("fake-core-bundle")
	if err := source.Push(ctx, registry.Ref{Name: "php-core"},
		bytes.NewReader(bundlePayload),
		&registry.Meta{SchemaVersion: 3, Kind: "php-core"},
		registry.Annotations{BundleName: "php-core", SpecHash: "sha256:integration-test"}); err != nil {
		t.Fatalf("seed source: %v", err)
	}
	// Resolve the source digest so SeedCore can Fetch it back.
	fetchedRef, hit, err := source.LookupBySpec(ctx, "php-core", "sha256:integration-test")
	if err != nil || !hit {
		t.Fatalf("LookupBySpec: hit=%v err=%v", hit, err)
	}

	if err := (defaultSidecarLifecycle{}).SeedCore(ctx, sc, source, fetchedRef, "buildrush", "test-tag"); err != nil {
		t.Fatalf("SeedCore: %v", err)
	}

	// Verify the seeded image is reachable on the sidecar's host port
	// by pulling it back with go-containerregistry. name.Insecure
	// because distribution:3 serves HTTP.
	target, err := name.ParseReference(sc.HostHost+"/buildrush/php-core:test-tag", name.Insecure)
	if err != nil {
		t.Fatalf("parse ref: %v", err)
	}
	desc, err := remote.Get(target, remote.WithContext(ctx))
	if err != nil {
		t.Fatalf("remote.Get: %v", err)
	}
	img, err := desc.Image()
	if err != nil {
		t.Fatalf("desc.Image: %v", err)
	}
	layers, err := img.Layers()
	if err != nil {
		t.Fatalf("Layers: %v", err)
	}
	if len(layers) < 1 {
		t.Fatal("seeded image has no layers")
	}
	rc, err := layers[0].Compressed()
	if err != nil {
		t.Fatalf("Compressed: %v", err)
	}
	defer func() { _ = rc.Close() }()
	got, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if !bytes.Equal(got, bundlePayload) {
		t.Errorf("pulled bundle = %q, want %q", got, bundlePayload)
	}
}
