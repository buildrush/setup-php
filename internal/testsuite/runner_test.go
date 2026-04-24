package testsuite

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/buildrush/setup-php/internal/build"
)

// writeTestsuiteFixture creates a minimal repo layout under dir with:
//   - test/compat/fixtures.yaml (a small synthetic FixtureSet)
//   - test/compat/probe.sh (empty but exists)
//   - docs/compat-matrix.md (compat matrix stub, required by buildCellMounts)
//   - out/oci-layout/ (empty dir to pass stat check)
//   - a fake "phpup" binary file (content doesn't matter — only presence is checked)
//
// Returns the absolute path of the fake binary. All paths written under
// dir so t.TempDir cleanup removes everything on test completion.
func writeTestsuiteFixture(t *testing.T, dir string) string {
	t.Helper()
	mustWrite := func(rel, content string, mode os.FileMode) {
		p := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
			t.Fatalf("mkdir %s: %v", rel, err)
		}
		if err := os.WriteFile(p, []byte(content), mode); err != nil {
			t.Fatalf("write %s: %v", rel, err)
		}
	}
	mustWrite("test/compat/fixtures.yaml", `fixtures:
  - name: bare-84
    php-version: "8.4"
    extensions: ""
    coverage: none
  - name: bare-82
    php-version: "8.2"
    extensions: ""
    coverage: none
`, 0o644)
	mustWrite("test/compat/probe.sh", "#!/bin/bash\n", 0o755)
	mustWrite("docs/compat-matrix.md", "# compat matrix stub\n", 0o644)
	if err := os.MkdirAll(filepath.Join(dir, "out", "oci-layout"), 0o750); err != nil {
		t.Fatal(err)
	}
	fakeBinary := filepath.Join(dir, "phpup-fake")
	mustWrite(filepath.Base(fakeBinary), "", 0o755)
	return fakeBinary
}

// testOptsWith builds a testOpts pointing at the synthetic fixture dir
// that writeTestsuiteFixture produced. Registry URI points at the
// layout-like empty dir so buildCellMounts' stat check succeeds.
func testOptsWith(t *testing.T, dir, fakeBinary string, oses, arches, phps []string) *testOpts {
	t.Helper()
	absRepo, err := filepath.Abs(dir)
	if err != nil {
		t.Fatalf("abs(%s): %v", dir, err)
	}
	return &testOpts{
		RegistryURI: "oci-layout:" + filepath.Join(absRepo, "out", "oci-layout"),
		OSes:        oses,
		Arches:      arches,
		PHPVersions: phps,
		Fixtures:    "test/compat/fixtures.yaml",
		Repo:        dir,
		Parallel:    1,
		AbsFixtures: filepath.Join(absRepo, "test", "compat", "fixtures.yaml"),
		AbsRepo:     absRepo,
		SelfBinary:  fakeBinary,
	}
}

func TestMain_UnknownArgs_Errors(t *testing.T) {
	err := Main([]string{})
	if err == nil || !strings.Contains(err.Error(), "--php") {
		t.Errorf("Main(empty) err = %v, want --php required", err)
	}
}

func TestMain_FixturesMissing_Errors(t *testing.T) {
	// Point at a nonexistent fixtures path; Main should error when Load
	// fails inside runAllCells (os.ReadFile on a missing file).
	err := Main([]string{"--php", "8.4", "--fixtures", "/tmp/phpup-testsuite-nonexistent.yaml"})
	if err == nil {
		t.Fatal("Main with missing fixtures: want error, got nil")
	}
}

func TestRunAllCells_AllPass(t *testing.T) {
	dir := t.TempDir()
	fakeBinary := writeTestsuiteFixture(t, dir)
	opts := testOptsWith(t, dir, fakeBinary, []string{"jammy"}, []string{"x86_64"}, []string{"8.4"})

	var runnerCalls int
	restore := build.SetRunner(func(_ context.Context, ro *build.DockerRunOpts) error {
		runnerCalls++
		// Sanity: the mounted /registry exists, the cmd is correct.
		// Cmd is wrapped as bash -c "<preamble> /usr/local/bin/phpup internal test-cell …"
		// so probe.sh has PHP's runtime libs at invocation time.
		if len(ro.Cmd) != 3 || ro.Cmd[0] != "bash" || ro.Cmd[1] != "-c" ||
			!strings.Contains(ro.Cmd[2], "/usr/local/bin/phpup internal test-cell") {
			return errors.New("unexpected Cmd: " + strings.Join(ro.Cmd, " "))
		}
		return nil
	})
	defer restore()

	if err := runAllCells(context.Background(), opts); err != nil {
		t.Fatalf("runAllCells: %v", err)
	}
	if runnerCalls != 1 {
		t.Errorf("runnerCalls = %d, want 1", runnerCalls)
	}
}

func TestRunAllCells_CellFailure_Aggregates(t *testing.T) {
	dir := t.TempDir()
	fakeBinary := writeTestsuiteFixture(t, dir)
	opts := testOptsWith(t, dir, fakeBinary, []string{"jammy"}, []string{"x86_64"}, []string{"8.4", "8.2"})

	restore := build.SetRunner(func(_ context.Context, ro *build.DockerRunOpts) error {
		if strings.Contains(strings.Join(ro.Cmd, " "), "--php 8.2") {
			return errors.New("synthetic failure for 8.2")
		}
		return nil
	})
	defer restore()

	err := runAllCells(context.Background(), opts)
	if err == nil || !strings.Contains(err.Error(), "1 cell(s) failed") {
		t.Errorf("runAllCells err = %v, want 1 cell(s) failed", err)
	}
}

func TestRunAllCells_NoFixturesForCell_SkipsNotFails(t *testing.T) {
	dir := t.TempDir()
	fakeBinary := writeTestsuiteFixture(t, dir)
	// PHP 7.4 has no matching fixture → skip, don't fail.
	opts := testOptsWith(t, dir, fakeBinary, []string{"jammy"}, []string{"x86_64"}, []string{"7.4"})

	var runnerCalls int
	restore := build.SetRunner(func(_ context.Context, _ *build.DockerRunOpts) error {
		runnerCalls++
		return nil
	})
	defer restore()

	err := runAllCells(context.Background(), opts)
	if err != nil {
		t.Fatalf("runAllCells with no matches should not error: %v", err)
	}
	if runnerCalls != 0 {
		t.Errorf("runnerCalls = %d, want 0 (no matching fixtures)", runnerCalls)
	}
}

func TestBuildCellMounts_RemoteRegistry_Errors(t *testing.T) {
	opts := &testOpts{
		RegistryURI: "ghcr.io/buildrush",
		AbsRepo:     "/tmp",
		SelfBinary:  "/tmp/phpup",
	}
	_, _, err := buildCellMounts(opts, "jammy", "x86_64", "8.4")
	if err == nil || !strings.Contains(err.Error(), "oci-layout:") {
		t.Errorf("buildCellMounts err = %v, want oci-layout: requirement", err)
	}
}

func TestParseTestFlags_SelfBinaryOverride(t *testing.T) {
	dir := t.TempDir()
	fake := filepath.Join(dir, "phpup-fake")
	if err := os.WriteFile(fake, []byte(""), 0o755); err != nil {
		t.Fatal(err)
	}
	opts, err := parseTestFlags([]string{
		"--php", "8.4",
		"--self-binary", fake,
	})
	if err != nil {
		t.Fatalf("parseTestFlags: %v", err)
	}
	absFake, err := filepath.Abs(fake)
	if err != nil {
		t.Fatalf("filepath.Abs(%s): %v", fake, err)
	}
	// filepath.Abs may normalize (e.g. resolve /var → /private/var on macOS).
	// Accept either the exact input or its absolutized form.
	if opts.SelfBinary != fake && opts.SelfBinary != absFake {
		t.Errorf("SelfBinary = %q, want %q (or abs %q)", opts.SelfBinary, fake, absFake)
	}
}

func TestParseTestFlags_SelfBinaryMissing_Errors(t *testing.T) {
	_, err := parseTestFlags([]string{
		"--php", "8.4",
		"--self-binary", "/tmp/definitely-not-here-XYZ",
	})
	if err == nil || !strings.Contains(err.Error(), "--self-binary") {
		t.Errorf("parseTestFlags err = %v, want mentions --self-binary", err)
	}
}

func TestCellSummary_FormatsOK(t *testing.T) {
	var buf bytes.Buffer
	printCellSummary(&buf, []cellResult{
		{OS: "jammy", Arch: "x86_64", PHP: "8.4", FixtureCount: 3},
		{OS: "noble", Arch: "aarch64", PHP: "8.2", FixtureCount: 0},
		{OS: "jammy", Arch: "x86_64", PHP: "8.1", FixtureCount: 2, Err: errors.New("boom")},
	})
	out := buf.String()
	for _, want := range []string{"jammy/x86_64 php=8.4 fixtures=3 — OK", "SKIP (no fixtures)", "FAIL: boom"} {
		if !strings.Contains(out, want) {
			t.Errorf("summary missing %q\nfull output:\n%s", want, out)
		}
	}
}

// TestBuildCellMounts_MountsCompatMatrix asserts that the compat-matrix
// markdown doc and the goldens directory (when it exists) are mounted
// read-only into the test-cell container so compatdiff.DiffFiles can
// read them via their container-side paths. The matrix file is a hard
// requirement (error if missing); the goldens dir is optional (absent
// = degraded compat-skip mode in runFixture).
func TestBuildCellMounts_MountsCompatMatrix(t *testing.T) {
	dir := t.TempDir()
	fakeBinary := writeTestsuiteFixture(t, dir)

	// writeTestsuiteFixture already wrote docs/compat-matrix.md.
	matrixPath := filepath.Join(dir, "docs", "compat-matrix.md")
	goldenDir := filepath.Join(dir, "test", "compat", "testdata", "golden")
	if err := os.MkdirAll(goldenDir, 0o755); err != nil {
		t.Fatalf("mkdir golden: %v", err)
	}

	opts := testOptsWith(t, dir, fakeBinary, []string{"noble"}, []string{"x86_64"}, []string{"8.4"})
	mounts, _, err := buildCellMounts(opts, "noble", "x86_64", "8.4")
	if err != nil {
		t.Fatalf("buildCellMounts: %v", err)
	}
	var sawMatrix, sawGoldens bool
	for _, m := range mounts {
		if m.Host == matrixPath && m.Container == "/compat-matrix.md" && m.ReadOnly {
			sawMatrix = true
		}
		if m.Host == goldenDir && m.Container == "/golden" && m.ReadOnly {
			sawGoldens = true
		}
	}
	if !sawMatrix {
		t.Errorf("buildCellMounts did not mount docs/compat-matrix.md -> /compat-matrix.md (ro); mounts=%+v", mounts)
	}
	if !sawGoldens {
		t.Errorf("buildCellMounts did not mount test/compat/testdata/golden -> /golden (ro); mounts=%+v", mounts)
	}
}

// TestBuildCellMounts_MissingCompatMatrix asserts that buildCellMounts
// returns an error (not silently skips) when docs/compat-matrix.md is
// absent from the repo. This is a repo-integrity signal: any checkout
// in which that file is missing should fail loudly.
func TestBuildCellMounts_MissingCompatMatrix(t *testing.T) {
	dir := t.TempDir()
	fakeBinary := writeTestsuiteFixture(t, dir)
	// Remove docs/compat-matrix.md that writeTestsuiteFixture created.
	matrixPath := filepath.Join(dir, "docs", "compat-matrix.md")
	if err := os.Remove(matrixPath); err != nil {
		t.Fatalf("remove compat-matrix.md: %v", err)
	}

	opts := testOptsWith(t, dir, fakeBinary, []string{"noble"}, []string{"x86_64"}, []string{"8.4"})
	_, _, err := buildCellMounts(opts, "noble", "x86_64", "8.4")
	if err == nil {
		t.Fatalf("buildCellMounts: want error for missing compat-matrix.md, got nil")
	}
	if !strings.Contains(err.Error(), "compat-matrix.md") {
		t.Errorf("error message should mention compat-matrix.md; got %v", err)
	}
}

// TestBuildCellMounts_GoldensDirOptional asserts that an absent
// goldens directory is NOT an error from buildCellMounts — it's the
// degraded mode that a fresh checkout (pre-refresh-workflow) sits in.
// Note: on a canonical-cell run, opted-in fixtures whose per-fixture
// golden file is missing are a HARD error at the runFixture level
// (step 6), not a silent skip. This test only covers the mount-
// construction layer; the per-fixture error is covered in
// testcell_compat_test.go.
func TestBuildCellMounts_GoldensDirOptional(t *testing.T) {
	dir := t.TempDir()
	fakeBinary := writeTestsuiteFixture(t, dir)

	// writeTestsuiteFixture already wrote docs/compat-matrix.md.
	// Deliberately do NOT create test/compat/testdata/golden.

	opts := testOptsWith(t, dir, fakeBinary, []string{"noble"}, []string{"x86_64"}, []string{"8.4"})
	mounts, _, err := buildCellMounts(opts, "noble", "x86_64", "8.4")
	if err != nil {
		t.Fatalf("buildCellMounts (no goldens dir): %v", err)
	}
	for _, m := range mounts {
		if m.Container == "/golden" {
			t.Errorf("no /golden mount expected when goldens dir is absent; got %+v", m)
		}
	}
}
