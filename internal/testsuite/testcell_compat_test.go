package testsuite

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestRunFixture_CompatSkippedOnNonCanonicalCell asserts that a fixture
// with Compat: true still passes on non-canonical cells (e.g., 8.1,
// arm64) because compat-diff is gated to noble/x86_64/8.4. The in-cell
// invariants still run; only the compat step is skipped.
func TestRunFixture_CompatSkippedOnNonCanonicalCell(t *testing.T) {
	restore := SetInstaller(stubInstallerOK)
	defer restore()

	workDir := t.TempDir()
	opts := &testCellOpts{
		OS: "jammy", Arch: "aarch64", PHP: "8.1",
		FixturesPath:   filepath.Join(workDir, "fixtures.yaml"),
		ProbePath:      filepath.Join(workDir, "probe.sh"),
		CompatMatrix:   filepath.Join(workDir, "compat-matrix.md"),
		GoldenDir:      filepath.Join(workDir, "golden"),
		DeviationsPath: filepath.Join(workDir, "deviations", "out.json"),
	}
	writeStubProbe(t, opts.ProbePath, `{"php_version":"8.1.0","sapi":"cli","zts":false,"extensions":[],"ini":{},"env_delta":[],"path_additions":[]}`)
	f := &Fixture{Name: "bare", PHPVersion: "8.1", Compat: true}

	out := runFixture(context.Background(), opts, f)
	if out.Err != nil {
		t.Fatalf("runFixture err (want nil on non-canonical cell): %v", out.Err)
	}
	if _, err := os.Stat(opts.DeviationsPath); !os.IsNotExist(err) {
		t.Errorf("deviations file should NOT exist on non-canonical cell; got err=%v", err)
	}
}

// TestRunFixture_CompatPassesOnCanonicalCellWithMatchingGolden asserts
// the compat-diff path runs when (os, arch, php) == canonical AND
// fixture.Compat && golden exists, and the probe matches.
func TestRunFixture_CompatPassesOnCanonicalCellWithMatchingGolden(t *testing.T) {
	restore := SetInstaller(stubInstallerOK)
	defer restore()

	workDir := t.TempDir()
	opts := &testCellOpts{
		OS: "noble", Arch: "x86_64", PHP: "8.4",
		FixturesPath:   filepath.Join(workDir, "fixtures.yaml"),
		ProbePath:      filepath.Join(workDir, "probe.sh"),
		CompatMatrix:   filepath.Join(workDir, "compat-matrix.md"),
		GoldenDir:      filepath.Join(workDir, "golden"),
		DeviationsPath: filepath.Join(workDir, "deviations", "out.json"),
	}
	probeJSON := `{"php_version":"8.4.0","sapi":"cli","zts":false,"extensions":[],"ini":{},"env_delta":[],"path_additions":[]}`
	writeStubProbe(t, opts.ProbePath, probeJSON)
	if err := os.MkdirAll(opts.GoldenDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(opts.GoldenDir, "bare.json"), []byte(probeJSON), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(opts.CompatMatrix, []byte(emptyAllowlistMD()), 0o644); err != nil {
		t.Fatal(err)
	}
	f := &Fixture{Name: "bare", PHPVersion: "8.4", Compat: true}

	out := runFixture(context.Background(), opts, f)
	if out.Err != nil {
		t.Fatalf("runFixture err: %v", out.Err)
	}
	if _, err := os.Stat(opts.DeviationsPath); !os.IsNotExist(err) {
		t.Errorf("deviations file should NOT exist on a passing compat run; got err=%v", err)
	}
}

// TestRunFixture_CompatFailsAndWritesDeviationsArtifact asserts that a
// probe that diverges from the golden fails the fixture AND appends to
// the deviations artifact.
func TestRunFixture_CompatFailsAndWritesDeviationsArtifact(t *testing.T) {
	restore := SetInstaller(stubInstallerOK)
	defer restore()

	workDir := t.TempDir()
	opts := &testCellOpts{
		OS: "noble", Arch: "x86_64", PHP: "8.4",
		FixturesPath:   filepath.Join(workDir, "fixtures.yaml"),
		ProbePath:      filepath.Join(workDir, "probe.sh"),
		CompatMatrix:   filepath.Join(workDir, "compat-matrix.md"),
		GoldenDir:      filepath.Join(workDir, "golden"),
		DeviationsPath: filepath.Join(workDir, "deviations", "out.json"),
	}
	oursProbe := `{"php_version":"8.4.0","sapi":"cli","zts":false,"extensions":[],"ini":{"memory_limit":"256M"},"env_delta":[],"path_additions":[]}`
	theirsGolden := `{"php_version":"8.4.0","sapi":"cli","zts":false,"extensions":[],"ini":{"memory_limit":"-1"},"env_delta":[],"path_additions":[]}`
	writeStubProbe(t, opts.ProbePath, oursProbe)
	if err := os.MkdirAll(opts.GoldenDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(opts.GoldenDir, "bare.json"), []byte(theirsGolden), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(opts.CompatMatrix, []byte(emptyAllowlistMD()), 0o644); err != nil {
		t.Fatal(err)
	}
	f := &Fixture{Name: "bare", PHPVersion: "8.4", Compat: true}

	out := runFixture(context.Background(), opts, f)
	if out.Err == nil {
		t.Fatalf("runFixture: want err for compat deviation, got nil")
	}
	if !strings.Contains(out.Err.Error(), "compat") {
		t.Errorf("runFixture err should mention compat; got: %v", out.Err)
	}
	data, err := os.ReadFile(opts.DeviationsPath)
	if err != nil {
		t.Fatalf("deviations artifact not written: %v", err)
	}
	var df DeviationsFile
	if err := json.Unmarshal(data, &df); err != nil {
		t.Fatalf("parse deviations artifact: %v\nraw: %s", err, data)
	}
	if df.Cell != "noble-x86_64-8.4" {
		t.Errorf("artifact cell: got %q, want noble-x86_64-8.4", df.Cell)
	}
	if len(df.Fixtures) != 1 || df.Fixtures[0].Name != "bare" {
		t.Errorf("artifact fixtures: got %+v, want single 'bare' entry", df.Fixtures)
	}
}

// TestRunFixture_CompatDevationsPathEmpty asserts that when compat
// deviates but the caller passed an empty DeviationsPath, the fixture
// STILL fails with the compat-deviation error — the artifact-write
// step is skipped silently, not elevated into a separate failure.
// Covers the non-transitive gate: shouldRunCompat returns true even
// when DeviationsPath is empty; the artifact write is guarded
// separately inside step 6 of runFixture.
func TestRunFixture_CompatDevationsPathEmpty(t *testing.T) {
	restore := SetInstaller(stubInstallerOK)
	defer restore()

	workDir := t.TempDir()
	opts := &testCellOpts{
		OS: "noble", Arch: "x86_64", PHP: "8.4",
		FixturesPath: filepath.Join(workDir, "fixtures.yaml"),
		ProbePath:    filepath.Join(workDir, "probe.sh"),
		CompatMatrix: filepath.Join(workDir, "compat-matrix.md"),
		GoldenDir:    filepath.Join(workDir, "golden"),
		// DeviationsPath deliberately left "" — artifact write must skip
		// without elevating to a failure mode.
	}
	oursProbe := `{"php_version":"8.4.0","sapi":"cli","zts":false,"extensions":[],"ini":{"memory_limit":"256M"},"env_delta":[],"path_additions":[]}`
	theirsGolden := `{"php_version":"8.4.0","sapi":"cli","zts":false,"extensions":[],"ini":{"memory_limit":"-1"},"env_delta":[],"path_additions":[]}`
	writeStubProbe(t, opts.ProbePath, oursProbe)
	if err := os.MkdirAll(opts.GoldenDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(opts.GoldenDir, "bare.json"), []byte(theirsGolden), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(opts.CompatMatrix, []byte(emptyAllowlistMD()), 0o644); err != nil {
		t.Fatal(err)
	}
	f := &Fixture{Name: "bare", PHPVersion: "8.4", Compat: true}

	out := runFixture(context.Background(), opts, f)
	if out.Err == nil {
		t.Fatalf("runFixture: want compat-deviation error, got nil")
	}
	if !strings.Contains(out.Err.Error(), "compat") {
		t.Errorf("err should mention compat; got: %v", out.Err)
	}
}

// TestRunFixture_CompatWriterErrorIsDiagnostic asserts that when
// AppendDeviations fails (unwritable path), the fixture still fails
// with the compat-deviation error — the writer error is surfaced on
// stderr but does NOT overwrite the fixture's out.Err. CI's
// compat-report job relies on the deviation error being surfaced, not
// a file-write error.
func TestRunFixture_CompatWriterErrorIsDiagnostic(t *testing.T) {
	restore := SetInstaller(stubInstallerOK)
	defer restore()

	workDir := t.TempDir()
	// Point DeviationsPath at a read-only parent so AppendDeviations'
	// MkdirAll fails. Creating the parent as 0o500 (read+execute, no
	// write) achieves this without needing superuser.
	readOnlyParent := filepath.Join(workDir, "ro-parent")
	if err := os.MkdirAll(readOnlyParent, 0o500); err != nil {
		t.Fatal(err)
	}
	// Child dir inside the read-only parent; MkdirAll will fail when
	// trying to create it.
	deviationsPath := filepath.Join(readOnlyParent, "child", "deviations.json")

	opts := &testCellOpts{
		OS: "noble", Arch: "x86_64", PHP: "8.4",
		FixturesPath:   filepath.Join(workDir, "fixtures.yaml"),
		ProbePath:      filepath.Join(workDir, "probe.sh"),
		CompatMatrix:   filepath.Join(workDir, "compat-matrix.md"),
		GoldenDir:      filepath.Join(workDir, "golden"),
		DeviationsPath: deviationsPath,
	}
	oursProbe := `{"php_version":"8.4.0","sapi":"cli","zts":false,"extensions":[],"ini":{"memory_limit":"256M"},"env_delta":[],"path_additions":[]}`
	theirsGolden := `{"php_version":"8.4.0","sapi":"cli","zts":false,"extensions":[],"ini":{"memory_limit":"-1"},"env_delta":[],"path_additions":[]}`
	writeStubProbe(t, opts.ProbePath, oursProbe)
	if err := os.MkdirAll(opts.GoldenDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(opts.GoldenDir, "bare.json"), []byte(theirsGolden), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(opts.CompatMatrix, []byte(emptyAllowlistMD()), 0o644); err != nil {
		t.Fatal(err)
	}
	f := &Fixture{Name: "bare", PHPVersion: "8.4", Compat: true}

	out := runFixture(context.Background(), opts, f)
	if out.Err == nil {
		t.Fatalf("runFixture: want compat-deviation error, got nil")
	}
	if !strings.Contains(out.Err.Error(), "compat") || !strings.Contains(out.Err.Error(), "unexplained deviation") {
		t.Errorf("err should still be the compat-deviation error even when writer fails; got: %v", out.Err)
	}
	// The writer failure should NOT have replaced the compat error. We
	// don't assert on stderr content here because capturing it would
	// require more plumbing than is worth for this contract; the
	// assertion that out.Err is the compat-deviation error (not a
	// write-error with "mkdir" or "write" in its message) suffices.
	if strings.Contains(out.Err.Error(), "mkdir") || strings.Contains(out.Err.Error(), "write") {
		t.Errorf("out.Err should not carry the AppendDeviations error; got: %v", out.Err)
	}
}

// stubInstallerOK is a no-op InstallFunc for tests; it writes the GHA-
// runner emulation files so downstream composeProbeEnv doesn't fail.
func stubInstallerOK(_ context.Context, env map[string]string, _, _ io.Writer) error {
	// runFixture pre-creates GITHUB_ENV / GITHUB_PATH files and passes their
	// paths via env. A real installer writes to them; this stub does not.
	// composeProbeEnv tolerates empty files (parseGitHubEnv returns empty
	// overlay on read error or empty body).
	_ = env
	return nil
}

// writeStubProbe writes a bash script that emits the given JSON to its
// first argument. Mirrors the shape of test/compat/probe.sh just enough
// for runFixture's parsing to succeed. probe.sh is invoked by runFixture
// with four args: out, env-before, path-before, ini-keys. This stub only
// reads the first and writes the literal JSON to it.
//
// The heredoc delimiter is single-quoted (<<'JSON') so bash does not
// perform variable expansion, command substitution, or backslash
// handling on jsonBody — tests can safely embed JSON containing $, `,
// or \ without corrupting the written output.
func writeStubProbe(t *testing.T, path, jsonBody string) {
	t.Helper()
	script := `#!/usr/bin/env bash
set -euo pipefail
out="$1"
cat > "$out" <<'JSON'
` + jsonBody + `
JSON
`
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
}

// emptyAllowlistMD returns a minimal compat-matrix.md body with the
// required delimiters and an empty deviations list. Just enough for
// internal/compatdiff.loadAllowlist to parse without error.
func emptyAllowlistMD() string {
	return `# Stub

<!-- compat-harness:deviations:start -->
` + "```yaml" + `
deviations: []
` + "```" + `
<!-- compat-harness:deviations:end -->
`
}
