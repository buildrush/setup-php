package build

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/buildrush/setup-php/internal/registry"
)

// writeRepoFixture creates a minimal on-disk repo skeleton under dir so
// ComputeSpecHash can find the files it needs during the test. The files
// are stand-ins — the builder scripts are single-line no-ops, but their
// contents still participate in the spec-hash so a realistic structure is
// required for cache-hit probes to line up with cache-miss builds.
func writeRepoFixture(t *testing.T, dir string) {
	t.Helper()
	mustWrite := func(rel, content string) {
		p := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", rel, err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", rel, err)
		}
	}
	mustWrite("builders/linux/build-php.sh", "#!/bin/bash\nexit 0\n")
	mustWrite("builders/linux/build-ext.sh", "#!/bin/bash\nexit 0\n")
	mustWrite("builders/common/bundle-schema-version.env", "SCHEMA_VERSION=3\n")
	mustWrite("builders/common/capture-hermetic-libs.sh", "#!/bin/bash\n")
	mustWrite("builders/common/pack-bundle.sh", "#!/bin/bash\n")
	mustWrite("builders/common/fetch-core.sh", "#!/bin/bash\n")
	mustWrite("builders/common/builder-os.env", "BUILDER_OS=ubuntu-22.04\n")
	mustWrite("catalog/php.yaml", "versions:\n  \"8.4\":\n    sources:\n      url: https://example.com/php-8.4.0.tar.xz\n")
	// Extension catalog with a real build_deps.linux list so
	// loadExtBuildDeps has something to parse. The shape matches
	// catalog/extensions/amqp.yaml's production form (see
	// .github/workflows/build-extension.yml's yq invocation).
	mustWrite("catalog/extensions/redis.yaml", "name: redis\nversions:\n  - \"6.2.0\"\nbuild_deps:\n  linux:\n    - libssl-dev\n")
}

// seedLayout pushes a manifest into an oci-layout so BuildPHP's cache
// probe returns a hit. Returns the registry URI pointing at the layout.
func seedLayout(t *testing.T, dir, bundleName, specHash string) string {
	t.Helper()
	layoutURI := "oci-layout:" + dir
	s, err := registry.Open(layoutURI)
	if err != nil {
		t.Fatalf("open layout: %v", err)
	}
	err = s.Push(context.Background(), registry.Ref{Name: bundleName},
		bytes.NewReader([]byte("fake")), nil,
		registry.Annotations{BundleName: bundleName, SpecHash: specHash})
	if err != nil {
		t.Fatalf("seed layout: %v", err)
	}
	return layoutURI
}

// fakeRunner is a RunnerFunc that writes a valid bundle + meta.json into
// the output mount so BuildPHP's read-push step can proceed without a
// real docker invocation. Looks for the mount at /tmp (matching the
// production mount contract — see BuildPHP's Mounts comment for why).
// Defaults to kind=php-core; use fakeRunnerKind when the test needs a
// different meta.json kind (e.g. "php-ext" for BuildExt).
func fakeRunner(bundleBytes []byte) RunnerFunc {
	return fakeRunnerKind(bundleBytes, "php-core")
}

// fakeRunnerKind is fakeRunner parameterised on the meta.json kind field
// so BuildExt tests can produce a synthetic php-ext bundle without
// tripping BuildPHP's schema-version assumption.
func fakeRunnerKind(bundleBytes []byte, kind string) RunnerFunc {
	return func(_ context.Context, opts *DockerRunOpts) error {
		var outHost string
		for _, m := range opts.Mounts {
			if m.Container == "/tmp" {
				outHost = m.Host
				break
			}
		}
		if outHost == "" {
			return errors.New("fakeRunner: no /tmp mount")
		}
		if err := os.WriteFile(filepath.Join(outHost, "bundle.tar.zst"), bundleBytes, 0o644); err != nil {
			return err
		}
		meta := map[string]any{"schema_version": 3, "kind": kind}
		mjson, err := json.Marshal(meta)
		if err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(outHost, "meta.json"), mjson, 0o644)
	}
}

// fakeSidecar is a SidecarLifecycle that doesn't touch docker. Start
// returns a placeholder *Sidecar and a no-op stop; SeedCore records
// the call for assertions. All counts are atomic so tests can safely
// inspect them after concurrent dispatch (even though BuildExt itself
// is single-goroutine).
type fakeSidecar struct {
	startCalls  atomic.Int32
	seedCalls   atomic.Int32
	stopCalls   atomic.Int32
	recordedRef registry.Ref
	recordedTag string
	recordedOwn string
}

// Start implements SidecarLifecycle; returns a synthetic sidecar with
// placeholder Network/hostnames that pass through DockerRunOpts without
// ever being dialled.
func (f *fakeSidecar) Start(_ context.Context) (*Sidecar, func(context.Context) error, error) {
	f.startCalls.Add(1)
	sc := &Sidecar{
		Name:          "fake-sidecar",
		Network:       "fake-net",
		InNetworkHost: "fake-sidecar:5000",
		HostHost:      "127.0.0.1:0",
	}
	return sc, func(context.Context) error {
		f.stopCalls.Add(1)
		return nil
	}, nil
}

// SeedCore implements SidecarLifecycle; records the ref/tag so tests
// can assert the right core was requested and swallows all I/O.
func (f *fakeSidecar) SeedCore(_ context.Context, _ *Sidecar, _ registry.Store, ref registry.Ref, owner, tag string) error {
	f.seedCalls.Add(1)
	f.recordedRef = ref
	f.recordedTag = tag
	f.recordedOwn = owner
	return nil
}

func TestBuildPHP_CacheHit_ShortCircuitsWithoutRunning(t *testing.T) {
	repo := t.TempDir()
	writeRepoFixture(t, repo)
	layoutDir := filepath.Join(t.TempDir(), "layout")

	hash, err := ComputeSpecHash(&SpecHashInputs{
		Kind: "php", Version: "8.4", OS: "linux", Arch: "x86_64", TS: "nts", Repo: repo,
	})
	if err != nil {
		t.Fatalf("ComputeSpecHash: %v", err)
	}
	layoutURI := seedLayout(t, layoutDir, "php-core", hash)

	var called bool
	restore := SetRunner(func(_ context.Context, _ *DockerRunOpts) error {
		called = true
		return errors.New("runner should not be called on cache hit")
	})
	defer restore()

	out := captureStdout(t, func() {
		err = BuildPHP(context.Background(), []string{
			"--php", "8.4",
			"--registry", layoutURI,
			"--repo", repo,
			"--out-dir", t.TempDir(),
		})
	})
	if err != nil {
		t.Fatalf("BuildPHP: %v", err)
	}
	if called {
		t.Fatal("runner was called on cache hit")
	}
	if !strings.Contains(out, "cache hit") {
		t.Errorf("stdout = %q, want contains \"cache hit\"", out)
	}
}

func TestBuildPHP_CacheMiss_InvokesRunnerThenPushes(t *testing.T) {
	repo := t.TempDir()
	writeRepoFixture(t, repo)
	layoutDir := filepath.Join(t.TempDir(), "layout")
	layoutURI := "oci-layout:" + layoutDir

	restore := SetRunner(fakeRunner([]byte("synthetic-bundle")))
	defer restore()

	err := BuildPHP(context.Background(), []string{
		"--php", "8.4",
		"--registry", layoutURI,
		"--repo", repo,
		"--out-dir", t.TempDir(),
	})
	if err != nil {
		t.Fatalf("BuildPHP: %v", err)
	}

	// Verify the layout now contains a manifest annotated with
	// php-core + spec-hash so a subsequent run lands on the cache-hit
	// path.
	s, _ := registry.Open(layoutURI)
	hash, _ := ComputeSpecHash(&SpecHashInputs{
		Kind: "php", Version: "8.4", OS: "linux", Arch: "x86_64", TS: "nts", Repo: repo,
	})
	ref, hit, err := s.LookupBySpec(context.Background(), "php-core", hash)
	if err != nil || !hit {
		t.Fatalf("LookupBySpec after build: hit=%v err=%v", hit, err)
	}
	if ref.Digest == "" {
		t.Error("pushed ref has empty digest")
	}
}

func TestBuildPHP_RunnerError_Propagates(t *testing.T) {
	repo := t.TempDir()
	writeRepoFixture(t, repo)
	layoutURI := "oci-layout:" + filepath.Join(t.TempDir(), "layout")

	restore := SetRunner(func(_ context.Context, _ *DockerRunOpts) error {
		return errors.New("boom")
	})
	defer restore()

	err := BuildPHP(context.Background(), []string{
		"--php", "8.4",
		"--registry", layoutURI,
		"--repo", repo,
		"--out-dir", t.TempDir(),
	})
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Errorf("BuildPHP err = %v, want containing \"boom\"", err)
	}
}

func TestBuildPHP_MissingVersionFlag_Errors(t *testing.T) {
	err := BuildPHP(context.Background(), []string{"--registry", "oci-layout:/tmp/x"})
	if err == nil || !strings.Contains(err.Error(), "--php") {
		t.Errorf("BuildPHP err = %v, want --php required", err)
	}
}

func TestBuildPHP_UnknownOS_Errors(t *testing.T) {
	repo := t.TempDir()
	writeRepoFixture(t, repo)
	layoutURI := "oci-layout:" + filepath.Join(t.TempDir(), "layout")

	err := BuildPHP(context.Background(), []string{
		"--php", "8.4", "--os", "bogus",
		"--registry", layoutURI, "--repo", repo,
		"--out-dir", t.TempDir(),
	})
	if err == nil || !strings.Contains(err.Error(), "unknown os") {
		t.Errorf("BuildPHP err = %v, want unknown os", err)
	}
}

func TestBuildPHP_UnknownArch_Errors(t *testing.T) {
	repo := t.TempDir()
	writeRepoFixture(t, repo)
	layoutURI := "oci-layout:" + filepath.Join(t.TempDir(), "layout")

	err := BuildPHP(context.Background(), []string{
		"--php", "8.4", "--arch", "bogus",
		"--registry", layoutURI, "--repo", repo,
		"--out-dir", t.TempDir(),
	})
	if err == nil || !strings.Contains(err.Error(), "unknown arch") {
		t.Errorf("BuildPHP err = %v, want unknown arch", err)
	}
}

// TestBuildPHP_ZTSNotSupported_Errors: --ts zts differs the spec-hash (so
// a future ZTS builder gets its own cache key) but today's build-php.sh
// has no --enable-zts path. Accepting zts would silently cache an NTS
// artifact under a ZTS key — reject until builder support lands.
func TestBuildPHP_ZTSNotSupported_Errors(t *testing.T) {
	repo := t.TempDir()
	writeRepoFixture(t, repo)
	err := BuildPHP(context.Background(), []string{
		"--php", "8.4", "--ts", "zts",
		"--registry", "oci-layout:" + filepath.Join(t.TempDir(), "layout"),
		"--repo", repo,
		"--out-dir", t.TempDir(),
	})
	if err == nil || !strings.Contains(err.Error(), "zts") {
		t.Errorf("BuildPHP err = %v, want zts rejection", err)
	}
}

// TestBuildPHP_AmdAliasNormalizes verifies that --arch amd64 produces the
// same spec-hash as --arch x86_64 — otherwise callers using the docker
// spelling would get a distinct cache entry for the same build.
func TestBuildPHP_AmdAliasNormalizes(t *testing.T) {
	repo := t.TempDir()
	writeRepoFixture(t, repo)
	// Reference hash computed from the canonical spelling.
	h1, err := ComputeSpecHash(&SpecHashInputs{
		Kind: "php", Version: "8.4", OS: "linux", Arch: "x86_64", TS: "nts", Repo: repo,
	})
	if err != nil {
		t.Fatalf("ComputeSpecHash: %v", err)
	}
	// Route through parsePHPFlags to get the normalized value.
	opts, err := parsePHPFlags([]string{"--php", "8.4", "--arch", "amd64", "--repo", repo})
	if err != nil {
		t.Fatalf("parsePHPFlags: %v", err)
	}
	if opts.Arch != "x86_64" {
		t.Fatalf("normalizeArch failure: Arch = %q, want x86_64", opts.Arch)
	}
	h2, err := ComputeSpecHash(&SpecHashInputs{
		Kind: "php", Version: opts.Version, OS: "linux", Arch: opts.Arch, TS: opts.TS, Repo: repo,
	})
	if err != nil {
		t.Fatalf("ComputeSpecHash (amd64): %v", err)
	}
	if h1 != h2 {
		t.Fatalf("spec-hash differs: %q vs %q", h1, h2)
	}
}

// TestBuildPHP_RealDockerSmoke exercises the real docker mount contract
// end to end — unit tests that use fakeRunner write into the output host
// dir directly, bypassing the container's filesystem. This test swaps
// build-php.sh for a 3-line synthetic that emulates pack-bundle.sh's
// output (writes /tmp/bundle.tar.zst + /tmp/meta.json) so we can verify
// the mount actually captures those files without paying a 10-minute
// PHP compile. Catches mount-path bugs (e.g. mounting /tmp/out instead
// of /tmp) that unit tests cannot.
func TestBuildPHP_RealDockerSmoke(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping real docker smoke under -short")
	}
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skipf("docker not found in PATH: %v", err)
	}
	repo := t.TempDir()
	writeRepoFixture(t, repo)
	// Overwrite build-php.sh with a synthetic that emulates what real
	// build-php.sh does: produce /tmp/bundle.tar.zst + /tmp/meta.json.
	fakeBuilder := "#!/usr/bin/env bash\n" +
		"set -euo pipefail\n" +
		"mkdir -p /tmp\n" +
		"echo 'synthetic bundle' > /tmp/bundle.tar.zst\n" +
		`echo '{"schema_version":3,"kind":"php-core"}' > /tmp/meta.json` + "\n"
	builderPath := filepath.Join(repo, "builders", "linux", "build-php.sh")
	if err := os.WriteFile(builderPath, []byte(fakeBuilder), 0o755); err != nil {
		t.Fatalf("write fake builder: %v", err)
	}
	// os.WriteFile preserves the mode of the pre-existing fixture file
	// (0o644 from writeRepoFixture). Force +x so the container can execv.
	if err := os.Chmod(builderPath, 0o755); err != nil {
		t.Fatalf("chmod fake builder: %v", err)
	}

	layoutURI := "oci-layout:" + filepath.Join(t.TempDir(), "layout")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	err := BuildPHP(ctx, []string{
		"--php", "8.4",
		"--registry", layoutURI,
		"--repo", repo,
		"--out-dir", t.TempDir(),
	})
	if err != nil {
		t.Fatalf("BuildPHP: %v", err)
	}

	// Verify the layout has the manifest at the expected spec-hash.
	hash, err := ComputeSpecHash(&SpecHashInputs{
		Kind: "php", Version: "8.4", OS: "linux", Arch: "x86_64", TS: "nts", Repo: repo,
	})
	if err != nil {
		t.Fatalf("ComputeSpecHash: %v", err)
	}
	s, err := registry.Open(layoutURI)
	if err != nil {
		t.Fatalf("registry.Open: %v", err)
	}
	ref, hit, err := s.LookupBySpec(ctx, "php-core", hash)
	if err != nil || !hit {
		t.Fatalf("LookupBySpec after real-docker build: hit=%v err=%v", hit, err)
	}
	if ref.Digest == "" {
		t.Fatal("pushed ref has empty digest after real-docker build")
	}
}

// TestResolveOutDir_DefaultShape asserts the derived default path when
// --out-dir is not supplied: <repo>/build/php/<version>-<os>-<arch>-<ts>/.
// Shape is load-bearing because it participates in the gitignore contract
// (build/ is ignored) and the Task 5 ext mirror will reuse <repo>/build/.
// Use a temp dir as the repo root — keeps the test OS-agnostic (so it
// works on Windows CI too where "/abs/repo" isn't absolute) and side-
// steps gocritic's "filepath.Join on a string that already contains a
// separator" complaint.
func TestResolveOutDir_DefaultShape(t *testing.T) {
	repo := t.TempDir()
	opts := &phpOpts{
		Version: "8.4", OS: "jammy", Arch: "x86_64", TS: "nts",
		Repo: repo,
	}
	got := resolveOutDir(opts)
	want := filepath.Join(repo, "build", "php", "8.4-jammy-x86_64-nts")
	if got != want {
		t.Errorf("resolveOutDir = %q, want %q", got, want)
	}
}

// TestResolveOutDir_ExplicitOverride asserts --out-dir is honored verbatim
// (pass-through, no derivation), which is how tests keep the worktree
// clean by pointing at t.TempDir().
func TestResolveOutDir_ExplicitOverride(t *testing.T) {
	opts := &phpOpts{OutDir: "/custom/path"}
	if got := resolveOutDir(opts); got != "/custom/path" {
		t.Errorf("resolveOutDir with --out-dir = %q, want /custom/path", got)
	}
}

func TestMain_UnknownKind_Errors(t *testing.T) {
	err := Main([]string{"tool"})
	if err == nil || !strings.Contains(err.Error(), "unknown kind") {
		t.Errorf("Main err = %v, want unknown kind", err)
	}
}

func TestMain_EmptyArgs_Errors(t *testing.T) {
	err := Main(nil)
	if err == nil {
		t.Fatal("Main(nil) want error, got nil")
	}
}

// TestMain_ExtDispatch_RoutesToBuildExt verifies that `phpup build ext`
// routes through to BuildExt's flag parsing — calling with no flags
// trips the --ext required error, which is BuildExt's first validation
// step. Confirms the Main switch does not fall through to an unrelated
// branch.
func TestMain_ExtDispatch_RoutesToBuildExt(t *testing.T) {
	err := Main([]string{"ext"})
	if err == nil || !strings.Contains(err.Error(), "--ext is required") {
		t.Errorf("Main([]string{\"ext\"}) err = %v, want containing \"--ext is required\"", err)
	}
}

// TestBuildExt_CacheHit_ShortCircuits seeds the layout with a
// php-ext-redis manifest annotated with the expected spec-hash and
// verifies BuildExt returns "cache hit" without starting the sidecar
// or invoking the runner.
func TestBuildExt_CacheHit_ShortCircuits(t *testing.T) {
	repo := t.TempDir()
	writeRepoFixture(t, repo)
	layoutDir := filepath.Join(t.TempDir(), "layout")

	hash, err := ComputeSpecHash(&SpecHashInputs{
		Kind: "ext", Name: "redis", Version: "6.2.0",
		OS: "linux", Arch: "x86_64", PHPABI: "8.4-nts", TS: "nts",
		Repo: repo,
	})
	if err != nil {
		t.Fatalf("ComputeSpecHash: %v", err)
	}
	layoutURI := seedLayout(t, layoutDir, "php-ext-redis", hash)

	var runnerCalled bool
	restoreRunner := SetRunner(func(_ context.Context, _ *DockerRunOpts) error {
		runnerCalled = true
		return errors.New("runner should not be called on cache hit")
	})
	defer restoreRunner()

	fs := &fakeSidecar{}
	restoreSidecar := SetSidecarLifecycle(fs)
	defer restoreSidecar()

	out := captureStdout(t, func() {
		err = BuildExt(context.Background(), []string{
			"--ext", "redis",
			"--ext-version", "6.2.0",
			"--php-abi", "8.4-nts",
			"--php-core-digest", "sha256:" + strings.Repeat("0", 64),
			"--registry", layoutURI,
			"--repo", repo,
			"--out-dir", t.TempDir(),
		})
	})
	if err != nil {
		t.Fatalf("BuildExt: %v", err)
	}
	if runnerCalled {
		t.Error("runner was called on cache hit")
	}
	if fs.startCalls.Load() != 0 {
		t.Errorf("sidecar.Start called %d times on cache hit, want 0", fs.startCalls.Load())
	}
	if !strings.Contains(out, "cache hit") {
		t.Errorf("stdout = %q, want contains \"cache hit\"", out)
	}
}

// TestBuildExt_CacheMiss_StartsSidecarSeedsCorePushes is the integration
// cousin of TestBuildPHP_CacheMiss_InvokesRunnerThenPushes: verifies
// that on an empty layout, BuildExt (a) starts the sidecar, (b) seeds
// the prerequisite core with the expected digest+tag, (c) runs the
// build, (d) pushes the resulting bundle. All via fakes — no real
// docker, no real registry.
func TestBuildExt_CacheMiss_StartsSidecarSeedsCorePushes(t *testing.T) {
	repo := t.TempDir()
	writeRepoFixture(t, repo)
	layoutDir := filepath.Join(t.TempDir(), "layout")
	layoutURI := "oci-layout:" + layoutDir

	restoreRunner := SetRunner(fakeRunnerKind([]byte("synthetic-ext-bundle"), "php-ext"))
	defer restoreRunner()

	fs := &fakeSidecar{}
	restoreSidecar := SetSidecarLifecycle(fs)
	defer restoreSidecar()

	coreDigest := "sha256:" + strings.Repeat("a", 64)
	err := BuildExt(context.Background(), []string{
		"--ext", "redis",
		"--ext-version", "6.2.0",
		"--php-abi", "8.4-nts",
		"--php-core-digest", coreDigest,
		"--registry", layoutURI,
		"--repo", repo,
		"--out-dir", t.TempDir(),
	})
	if err != nil {
		t.Fatalf("BuildExt: %v", err)
	}

	// Sidecar was started, seeded once, and stopped.
	if fs.startCalls.Load() != 1 {
		t.Errorf("startCalls = %d, want 1", fs.startCalls.Load())
	}
	if fs.seedCalls.Load() != 1 {
		t.Errorf("seedCalls = %d, want 1", fs.seedCalls.Load())
	}
	if fs.stopCalls.Load() != 1 {
		t.Errorf("stopCalls = %d, want 1", fs.stopCalls.Load())
	}
	// SeedCore received the right ref and tag.
	if fs.recordedRef.Name != "php-core" || fs.recordedRef.Digest != coreDigest {
		t.Errorf("recordedRef = %+v, want {Name:php-core Digest:%s}", fs.recordedRef, coreDigest)
	}
	wantTag := "8.4-linux-x86_64-nts"
	if fs.recordedTag != wantTag {
		t.Errorf("recordedTag = %q, want %q", fs.recordedTag, wantTag)
	}
	if fs.recordedOwn != "buildrush" {
		t.Errorf("recordedOwn = %q, want buildrush", fs.recordedOwn)
	}

	// Pushed bundle is discoverable via LookupBySpec.
	hash, err := ComputeSpecHash(&SpecHashInputs{
		Kind: "ext", Name: "redis", Version: "6.2.0",
		OS: "linux", Arch: "x86_64", PHPABI: "8.4-nts", TS: "nts",
		Repo: repo,
	})
	if err != nil {
		t.Fatalf("ComputeSpecHash: %v", err)
	}
	s, _ := registry.Open(layoutURI)
	ref, hit, err := s.LookupBySpec(context.Background(), "php-ext-redis", hash)
	if err != nil || !hit {
		t.Fatalf("LookupBySpec after build: hit=%v err=%v", hit, err)
	}
	if ref.Digest == "" {
		t.Error("pushed ref has empty digest")
	}
}

// TestBuildExt_RunnerError_Propagates verifies the build-container
// exit code surfaces as an error with the expected prefix. Also
// doubles as a teardown probe: fakeSidecar.stopCalls MUST still be 1
// after the error path exits, confirming the defer fires even on
// failure.
func TestBuildExt_RunnerError_Propagates(t *testing.T) {
	repo := t.TempDir()
	writeRepoFixture(t, repo)
	layoutURI := "oci-layout:" + filepath.Join(t.TempDir(), "layout")

	restoreRunner := SetRunner(func(_ context.Context, _ *DockerRunOpts) error {
		return errors.New("boom")
	})
	defer restoreRunner()

	fs := &fakeSidecar{}
	restoreSidecar := SetSidecarLifecycle(fs)
	defer restoreSidecar()

	err := BuildExt(context.Background(), []string{
		"--ext", "redis",
		"--ext-version", "6.2.0",
		"--php-abi", "8.4-nts",
		"--php-core-digest", "sha256:" + strings.Repeat("b", 64),
		"--registry", layoutURI,
		"--repo", repo,
		"--out-dir", t.TempDir(),
	})
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Errorf("BuildExt err = %v, want containing \"boom\"", err)
	}
	if fs.stopCalls.Load() != 1 {
		t.Errorf("stopCalls after error = %d, want 1 (teardown defer must run)", fs.stopCalls.Load())
	}
}

// TestBuildExt_MissingFlags_Errors sweeps through each required flag
// and verifies parseExtFlags rejects the omission with a clear message.
func TestBuildExt_MissingFlags_Errors(t *testing.T) {
	cases := []struct {
		name    string
		args    []string
		wantMsg string
	}{
		{"no ext", []string{"--ext-version", "1", "--php-abi", "8.4-nts", "--php-core-digest", "sha256:x"}, "--ext is required"},
		{"no ext-version", []string{"--ext", "redis", "--php-abi", "8.4-nts", "--php-core-digest", "sha256:x"}, "--ext-version is required"},
		{"no php-abi", []string{"--ext", "redis", "--ext-version", "1", "--php-core-digest", "sha256:x"}, "--php-abi is required"},
		{"no php-core-digest", []string{"--ext", "redis", "--ext-version", "1", "--php-abi", "8.4-nts"}, "--php-core-digest is required"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := BuildExt(context.Background(), tc.args)
			if err == nil || !strings.Contains(err.Error(), tc.wantMsg) {
				t.Errorf("BuildExt err = %v, want containing %q", err, tc.wantMsg)
			}
		})
	}
}

// TestBuildExt_UnknownArch_Errors ensures the arch normalisation
// rejects bogus inputs before we ever touch the sidecar or runner.
func TestBuildExt_UnknownArch_Errors(t *testing.T) {
	repo := t.TempDir()
	writeRepoFixture(t, repo)
	err := BuildExt(context.Background(), []string{
		"--ext", "redis", "--ext-version", "6.2.0",
		"--php-abi", "8.4-nts", "--php-core-digest", "sha256:x",
		"--arch", "bogus",
		"--registry", "oci-layout:" + filepath.Join(t.TempDir(), "layout"),
		"--repo", repo,
		"--out-dir", t.TempDir(),
	})
	if err == nil || !strings.Contains(err.Error(), "unknown arch") {
		t.Errorf("BuildExt err = %v, want unknown arch", err)
	}
}

// TestResolveExtOutDir_DefaultShape asserts the derived default path
// when --out-dir is not supplied. Shape is load-bearing because it
// participates in the gitignore contract (build/ is ignored), mirroring
// resolveOutDir (php).
func TestResolveExtOutDir_DefaultShape(t *testing.T) {
	repo := t.TempDir()
	opts := &extOpts{
		Name: "redis", Version: "6.2.0", PHPABI: "8.4-nts",
		OS: "jammy", Arch: "x86_64",
		Repo: repo,
	}
	got := resolveExtOutDir(opts)
	want := filepath.Join(repo, "build", "ext", "redis-6.2.0-8.4-nts-jammy-x86_64")
	if got != want {
		t.Errorf("resolveExtOutDir = %q, want %q", got, want)
	}
}

// TestResolveExtOutDir_ExplicitOverride asserts --out-dir is honored
// verbatim (pass-through, no derivation).
func TestResolveExtOutDir_ExplicitOverride(t *testing.T) {
	opts := &extOpts{OutDir: "/custom/path"}
	if got := resolveExtOutDir(opts); got != "/custom/path" {
		t.Errorf("resolveExtOutDir with --out-dir = %q, want /custom/path", got)
	}
}

// TestCoreTagForFetch_MatchesFetchCoreShellLogic pins the tag shape
// phpup emits against the shell logic in builders/common/fetch-core.sh
// (which constructs TAG="${PHP_VER}-${OS}-${ARCH}-${PHP_TS}"). If this
// diverges, the build container's oras pull will miss the seeded image.
func TestCoreTagForFetch_MatchesFetchCoreShellLogic(t *testing.T) {
	cases := []struct {
		phpABI, arch, want string
	}{
		{"8.4-nts", "x86_64", "8.4-linux-x86_64-nts"},
		{"8.3-nts", "aarch64", "8.3-linux-aarch64-nts"},
		{"8.2-nts", "x86_64", "8.2-linux-x86_64-nts"},
	}
	for _, tc := range cases {
		if got := coreTagForFetch(tc.phpABI, tc.arch); got != tc.want {
			t.Errorf("coreTagForFetch(%q, %q) = %q, want %q", tc.phpABI, tc.arch, got, tc.want)
		}
	}
}

// TestLoadExtBuildDeps_JoinsLinuxList verifies loadExtBuildDeps returns
// the space-joined list that build-ext.sh's BUILD_DEPS env expects —
// matching the yq invocation in build-extension.yml.
func TestLoadExtBuildDeps_JoinsLinuxList(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "amqp.yaml")
	const body = "name: amqp\nbuild_deps:\n  linux:\n    - libfoo-dev\n    - libbar-dev\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write yaml: %v", err)
	}
	got, err := loadExtBuildDeps(path)
	if err != nil {
		t.Fatalf("loadExtBuildDeps: %v", err)
	}
	if got != "libfoo-dev libbar-dev" {
		t.Errorf("got %q, want %q", got, "libfoo-dev libbar-dev")
	}
}

// TestLoadExtBuildDeps_AbsentSection_ReturnsEmpty exercises the
// no-build-deps path (e.g. catalog/extensions/redis.yaml originally).
// Must return "" (not error) so build-ext.sh treats it as a no-op.
func TestLoadExtBuildDeps_AbsentSection_ReturnsEmpty(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "x.yaml")
	if err := os.WriteFile(path, []byte("name: x\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	got, err := loadExtBuildDeps(path)
	if err != nil {
		t.Fatalf("loadExtBuildDeps: %v", err)
	}
	if got != "" {
		t.Errorf("got %q, want empty", got)
	}
}

// captureStdout redirects os.Stdout for the duration of fn and returns
// what was written. Simple helper; doesn't need to be fancy. Tests that
// use this helper must not run with t.Parallel() because os.Stdout is a
// process-level global.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	os.Stdout = w
	fn()
	_ = w.Close()
	os.Stdout = old
	out, _ := io.ReadAll(r)
	return string(out)
}
