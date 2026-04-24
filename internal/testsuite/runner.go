package testsuite

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/buildrush/setup-php/internal/build"
	"github.com/buildrush/setup-php/internal/registry"
)

// Main is the entry point for `phpup test …`. Parses flags, expands the
// (os × arch × php) cartesian product, and runs each cell in a bare-ubuntu
// container via internal/build.DockerRun. Aggregates outcomes; returns
// non-nil error if any cell failed.
func Main(args []string) error {
	opts, err := parseTestFlags(args)
	if err != nil {
		return err
	}
	ctx := context.Background()
	return runAllCells(ctx, opts)
}

// testOpts is the parsed flag set for `phpup test`. Fields are populated
// from the flag parser and then augmented with absolute paths (AbsRepo,
// AbsFixtures, SelfBinary) so downstream code can mount them into the
// per-cell container without re-resolving.
type testOpts struct {
	RegistryURI string
	OSes        []string // normalized to "jammy" / "noble"
	Arches      []string // normalized to "x86_64" / "aarch64"
	PHPVersions []string
	Fixtures    string
	Repo        string
	Parallel    int
	Cache       string
	// Resolved once at parse time.
	AbsFixtures string
	AbsRepo     string
	SelfBinary  string
}

// parseTestFlags parses the flag tail for `phpup test`. Uses ContinueOnError
// so callers get an error back instead of a process exit — keeps the surface
// testable without os.Exit acrobatics.
func parseTestFlags(args []string) (*testOpts, error) {
	fs := flag.NewFlagSet("phpup test", flag.ContinueOnError)
	registryFlag := fs.String("registry", "oci-layout:./out/oci-layout",
		"OCI artifact store (oci-layout:<path> or ghcr.io/<owner>)")
	osFlag := fs.String("os", "jammy,noble",
		"Comma-separated Ubuntu flavours: jammy (22.04), noble (24.04)")
	archFlag := fs.String("arch", "x86_64,aarch64",
		"Comma-separated target archs: x86_64, aarch64 (amd64/arm64 aliases accepted)")
	phpFlag := fs.String("php", "",
		"Comma-separated PHP minor versions (e.g. 8.1,8.4); REQUIRED")
	fixturesFlag := fs.String("fixtures", "test/compat/fixtures.yaml",
		"Path to fixtures YAML (relative to --repo or absolute)")
	repoFlag := fs.String("repo", ".",
		"Path to setup-php repo root")
	parallelFlag := fs.Int("parallel", 1,
		"Number of cells to run concurrently (default 1)")
	cacheFlag := fs.String("cache", "./.cache/phpup-test",
		"Cache directory (reserved for future use)")
	selfBinaryFlag := fs.String("self-binary", "",
		"Absolute path to a linux/<cell-arch> phpup binary to mount as "+
			"/usr/local/bin/phpup inside the bare-ubuntu container. "+
			"When empty, os.Executable() is used — only correct when the "+
			"host arch matches the cell arch (e.g. linux CI runners). On "+
			"macOS hosts or cross-arch runs, pass a cross-compiled binary.")

	if err := fs.Parse(args); err != nil {
		return nil, err
	}
	if *phpFlag == "" {
		return nil, errors.New("phpup test: --php is required")
	}

	absRepo, err := filepath.Abs(*repoFlag)
	if err != nil {
		return nil, fmt.Errorf("phpup test: resolve repo path: %w", err)
	}
	fixturesPath := *fixturesFlag
	if !filepath.IsAbs(fixturesPath) {
		fixturesPath = filepath.Join(absRepo, fixturesPath)
	}

	selfAbs, err := resolveSelfBinary(*selfBinaryFlag)
	if err != nil {
		return nil, err
	}

	return &testOpts{
		RegistryURI: *registryFlag,
		OSes:        splitNormalizedOS(*osFlag),
		Arches:      splitNormalizedArch(*archFlag),
		PHPVersions: splitCSV(*phpFlag),
		Fixtures:    *fixturesFlag,
		Repo:        *repoFlag,
		Parallel:    *parallelFlag,
		Cache:       *cacheFlag,
		AbsFixtures: fixturesPath,
		AbsRepo:     absRepo,
		SelfBinary:  selfAbs,
	}, nil
}

// resolveSelfBinary picks the absolute path to the phpup binary that should
// be mounted at /usr/local/bin/phpup inside the bare-ubuntu cell container.
// When override is non-empty it is absolutized and checked for existence —
// this is the path CI/Makefile use to supply a cross-compiled
// linux/<cell-arch> binary when the host arch or OS doesn't match the cell.
// When override is empty the caller is trusted (host is a linux/<cell-arch>
// runner) and os.Executable() is used. An empty override on a mismatched
// host will surface as `exec format error` when the container tries to run
// the darwin/amd64/wrong-arch binary, which is why CI threads --self-binary.
func resolveSelfBinary(override string) (string, error) {
	if override != "" {
		abs, err := filepath.Abs(override)
		if err != nil {
			return "", fmt.Errorf("phpup test: resolve --self-binary: %w", err)
		}
		if _, err := os.Stat(abs); err != nil {
			return "", fmt.Errorf("phpup test: --self-binary missing: %w", err)
		}
		return abs, nil
	}
	self, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("phpup test: resolve self binary: %w", err)
	}
	abs, err := filepath.Abs(self)
	if err != nil {
		return "", fmt.Errorf("phpup test: absolutize self binary: %w", err)
	}
	return abs, nil
}

// splitCSV splits s on commas, trims whitespace, and drops empty tokens.
// Returns an empty slice for empty input rather than a nil one-element
// slice containing the empty string.
func splitCSV(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// splitNormalizedOS splits s on commas and canonicalizes each entry to the
// short codename form used by Filter ("jammy"/"noble"). The ubuntu-XX.YY
// long form is accepted on input. Deduplicates while preserving order.
func splitNormalizedOS(s string) []string {
	raw := splitCSV(s)
	seen := map[string]struct{}{}
	var out []string
	for _, o := range raw {
		// Canonicalize to jammy/noble; the reverse of normalizeOS's direction.
		// normalizeOS takes short codename → long form; here we go long → short
		// so cell labels (and docker image lookup via UbuntuImage which takes
		// both) stay consistent.
		switch o {
		case "ubuntu-22.04":
			o = "jammy"
		case "ubuntu-24.04":
			o = "noble"
		}
		if _, ok := seen[o]; !ok {
			seen[o] = struct{}{}
			out = append(out, o)
		}
	}
	return out
}

// splitNormalizedArch splits s on commas and canonicalizes each entry to
// the uname form ("x86_64"/"aarch64") via normalizeArch. Silently drops
// entries that normalizeArch can't canonicalize (returns empty). Dedupes
// while preserving first-seen order.
func splitNormalizedArch(s string) []string {
	raw := splitCSV(s)
	seen := map[string]struct{}{}
	var out []string
	for _, a := range raw {
		a = normalizeArch(a)
		if a == "" {
			continue
		}
		if _, ok := seen[a]; !ok {
			seen[a] = struct{}{}
			out = append(out, a)
		}
	}
	return out
}

// cellResult captures the outcome of running one (os, arch, php) cell.
// A cell that matched zero fixtures records FixtureCount=0 and Err=nil
// (skip), which printCellSummary renders as "SKIP (no fixtures)". A cell
// that ran with matching fixtures but the docker invocation failed
// records the error and the count; the cell is counted as failed.
type cellResult struct {
	OS, Arch, PHP string
	FixtureCount  int
	Err           error
}

// runAllCells loads the fixtures file once, then walks the cartesian
// product of opts.OSes × opts.Arches × opts.PHPVersions. Each cell is run
// serially; the --parallel flag is reserved but unused in PR 3 (Task 2).
// A cell with no matching fixtures is not an error. The returned error
// is non-nil iff at least one cell's DockerRun failed.
func runAllCells(ctx context.Context, opts *testOpts) error {
	set, err := Load(opts.AbsFixtures)
	if err != nil {
		return fmt.Errorf("phpup test: load fixtures: %w", err)
	}

	var results []cellResult
	for _, o := range opts.OSes {
		for _, a := range opts.Arches {
			for _, p := range opts.PHPVersions {
				res := runCell(ctx, opts, set, o, a, p)
				results = append(results, res)
			}
		}
	}

	var failed []cellResult
	for _, r := range results {
		if r.Err != nil {
			failed = append(failed, r)
		}
	}
	printCellSummary(os.Stdout, results)
	if len(failed) > 0 {
		return fmt.Errorf("phpup test: %d cell(s) failed", len(failed))
	}
	return nil
}

// runCell executes one (os, arch, php) cell. Returns a cellResult whose
// Err is nil on success, nil on "no fixtures" (a skip), or the docker
// error otherwise. Never panics; all failure modes funnel through the
// cellResult.Err field so the caller can aggregate without special cases.
func runCell(ctx context.Context, opts *testOpts, set *FixtureSet, cellOS, cellArch, cellPHP string) cellResult {
	res := cellResult{OS: cellOS, Arch: cellArch, PHP: cellPHP}
	fixtures := set.Filter(cellOS, cellArch, cellPHP)
	res.FixtureCount = len(fixtures)
	if len(fixtures) == 0 {
		fmt.Printf("phpup test: [skip] cell os=%s arch=%s php=%s: no fixtures\n", cellOS, cellArch, cellPHP)
		return res
	}

	image, err := build.UbuntuImage(cellOS)
	if err != nil {
		res.Err = err
		return res
	}
	platform, err := build.DockerPlatform(cellArch)
	if err != nil {
		res.Err = err
		return res
	}

	mounts, env, err := buildCellMounts(opts, cellOS, cellArch, cellPHP)
	if err != nil {
		res.Err = err
		return res
	}

	// The test container is bare ubuntu:22.04; PHP's runtime libs aren't
	// preinstalled. Wrap the test-cell invocation with the same apt
	// preamble that build-ext uses so probe.sh's `php -v` has its
	// shared libs available (libreadline, libpq, libssl, libxml2, etc.).
	cellCmd := strings.Join([]string{"/usr/local/bin/phpup", "internal", "test-cell",
		"--os", cellOS, "--arch", cellArch, "--php", cellPHP,
		"--fixtures", "/test-compat/fixtures.yaml",
		"--probe", "/test-compat/probe.sh",
		"--registry", "oci-layout:/registry",
	}, " ")
	runOpts := &build.DockerRunOpts{
		Image:    image,
		Platform: platform,
		Mounts:   mounts,
		Env:      env,
		Cmd:      []string{"bash", "-c", build.LinuxExtAptPreamble + cellCmd},
	}

	fmt.Printf("phpup test: [run] cell os=%s arch=%s php=%s (%d fixtures)\n", cellOS, cellArch, cellPHP, len(fixtures))
	if err := build.DockerRun(ctx, runOpts); err != nil {
		res.Err = fmt.Errorf("cell os=%s arch=%s php=%s: %w", cellOS, cellArch, cellPHP, err)
	}
	return res
}

// buildCellMounts builds the docker mount list and container env for a
// cell. Expects opts.RegistryURI to be oci-layout:<path>; if it's a
// remote URI, returns an error because mounting a remote registry inside
// the test container isn't wired up yet (PR 4 scope).
//
// Three mounts are produced:
//   - oci-layout directory → /registry (read-only)
//   - phpup self-binary   → /usr/local/bin/phpup (read-only)
//   - repo's test/compat  → /test-compat (read-only)
//
// Env is minimal: PHPUP_REGISTRY=oci-layout:/registry so the inner
// phpup invocation inherits the right registry without needing to pass
// --registry through again. The --registry flag IS still passed on the
// inner Cmd (see runCell) so the inner binary works even if env-inherit
// semantics change.
func buildCellMounts(opts *testOpts, _, _, _ string) ([]build.Mount, map[string]string, error) {
	const ociLayoutPrefix = "oci-layout:"
	if !strings.HasPrefix(opts.RegistryURI, ociLayoutPrefix) {
		return nil, nil, fmt.Errorf("phpup test: --registry must be oci-layout:<path> (got %q); remote registries inside the test container are PR 4 scope", opts.RegistryURI)
	}
	layoutPath := strings.TrimPrefix(opts.RegistryURI, ociLayoutPrefix)
	absLayout, err := filepath.Abs(layoutPath)
	if err != nil {
		return nil, nil, fmt.Errorf("resolve layout: %w", err)
	}

	testCompat := filepath.Join(opts.AbsRepo, "test", "compat")
	if _, err := os.Stat(testCompat); err != nil {
		return nil, nil, fmt.Errorf("test/compat dir missing under repo: %w", err)
	}
	if _, err := os.Stat(opts.SelfBinary); err != nil {
		return nil, nil, fmt.Errorf("self binary missing: %w", err)
	}

	// Synthesize a lockfile override from the layout's annotated manifests.
	// `phpup install` inside the container reads PHPUP_LOCKFILE (env) as an
	// alternative to its embedded bundles.lock. This lets test fixtures
	// resolve bundle refs against the layout's actual digests (which differ
	// from GHCR-published digests because meta.json carries a build-time
	// timestamp that isn't reproducible byte-for-byte).
	overridePath, err := writeLayoutLockfileOverride(absLayout, opts.AbsRepo)
	if err != nil {
		return nil, nil, fmt.Errorf("synthesize lockfile override: %w", err)
	}

	mounts := []build.Mount{
		{Host: absLayout, Container: "/registry", ReadOnly: true},
		{Host: opts.SelfBinary, Container: "/usr/local/bin/phpup", ReadOnly: true},
		{Host: testCompat, Container: "/test-compat", ReadOnly: true},
		{Host: overridePath, Container: "/tmp/bundles-override.lock", ReadOnly: true},
	}
	env := map[string]string{
		"PHPUP_REGISTRY": "oci-layout:/registry",
		"PHPUP_LOCKFILE": "/tmp/bundles-override.lock",
	}
	return mounts, env, nil
}

// writeLayoutLockfileOverride walks the oci-layout's annotated manifests and
// emits a lockfile JSON file mapping each manifest's io.buildrush.bundle.key
// annotation to its actual digest. Written under <absRepo>/.cache/phpup-test/
// so it's inside the gitignored cache tree; returns the absolute path for
// mounting into the test container.
func writeLayoutLockfileOverride(absLayout, absRepo string) (string, error) {
	store, err := registry.Open("oci-layout:" + absLayout)
	if err != nil {
		return "", fmt.Errorf("open layout: %w", err)
	}
	type lister interface {
		ListKeyed(ctx context.Context) ([]registry.KeyedRef, error)
	}
	listerStore, ok := store.(lister)
	if !ok {
		return "", fmt.Errorf("layout store does not implement ListKeyed")
	}
	refs, err := listerStore.ListKeyed(context.Background())
	if err != nil {
		return "", fmt.Errorf("list keyed refs: %w", err)
	}
	bundles := make(map[string]map[string]string, len(refs))
	for _, r := range refs {
		bundles[r.Key] = map[string]string{"digest": r.Digest}
		if r.SpecHash != "" {
			bundles[r.Key]["spec_hash"] = r.SpecHash
		}
	}
	doc := map[string]any{
		"schema_version": 2,
		"generated_at":   time.Now().UTC().Format(time.RFC3339),
		"bundles":        bundles,
	}
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal lockfile: %w", err)
	}
	outDir := filepath.Join(absRepo, ".cache", "phpup-test")
	if err := os.MkdirAll(outDir, 0o750); err != nil {
		return "", fmt.Errorf("mkdir cache: %w", err)
	}
	outPath := filepath.Join(outDir, "bundles-override.lock")
	if err := os.WriteFile(outPath, data, 0o644); err != nil { //nolint:gosec // non-sensitive override file; mounted read-only into ephemeral test container.
		return "", fmt.Errorf("write lockfile: %w", err)
	}
	return outPath, nil
}

// printCellSummary writes a per-cell summary table to w. Entries are
// emitted in iteration order (matches the cartesian product walk in
// runAllCells). Each row is one of:
//
//	<os>/<arch> php=<v> fixtures=<n> — OK
//	<os>/<arch> php=<v> fixtures=0 — SKIP (no fixtures)
//	<os>/<arch> php=<v> fixtures=<n> — FAIL: <reason>
//
// The Fprint* writes are intentionally fire-and-forget: the summary is
// purely informational and its write errors (e.g. broken pipe when the
// operator pipes into `head`) shouldn't clobber the real exit code
// that runAllCells computes from the aggregated cell results.
func printCellSummary(w io.Writer, results []cellResult) {
	_, _ = fmt.Fprintln(w, "\n=== phpup test summary ===")
	for _, r := range results {
		status := "OK"
		switch {
		case r.Err != nil:
			status = "FAIL: " + r.Err.Error()
		case r.FixtureCount == 0:
			status = "SKIP (no fixtures)"
		}
		_, _ = fmt.Fprintf(w, "  %s/%s php=%s fixtures=%d — %s\n", r.OS, r.Arch, r.PHP, r.FixtureCount, status)
	}
}
