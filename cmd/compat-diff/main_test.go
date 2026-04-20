// cmd/compat-diff/main_test.go
package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func buildBinary(t *testing.T) string {
	t.Helper()
	bin := t.TempDir() + "/compat-diff"
	cmd := exec.Command("go", "build", "-o", bin, ".")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("build: %v: %s", err, out)
	}
	return bin
}

func TestMissingFlagsExits2(t *testing.T) {
	bin := buildBinary(t)
	cmd := exec.Command(bin)
	out, err := cmd.CombinedOutput()
	ee, ok := err.(*exec.ExitError)
	if !ok {
		t.Fatalf("expected ExitError, got %v (%s)", err, out)
	}
	if ee.ExitCode() != 2 {
		t.Fatalf("exit code = %d, want 2 (output: %s)", ee.ExitCode(), out)
	}
	if !strings.Contains(string(out), "--ours") {
		t.Errorf("usage output missing --ours flag (got %s)", out)
	}
}

func runCLI(t *testing.T, bin string, args ...string) (output string, exitCode int) {
	t.Helper()
	cmd := exec.Command(bin, args...)
	out, err := cmd.CombinedOutput()
	if err == nil {
		return string(out), 0
	}
	ee, ok := err.(*exec.ExitError)
	if !ok {
		t.Fatalf("run %v: %v", args, err)
	}
	return string(out), ee.ExitCode()
}

func TestRunExactMatchExits0(t *testing.T) {
	bin := buildBinary(t)
	tdata, err := filepath.Abs("testdata")
	if err != nil {
		t.Fatal(err)
	}
	out, code := runCLI(t, bin,
		"--ours", filepath.Join(tdata, "probe-bare.json"),
		"--theirs", filepath.Join(tdata, "probe-bare.json"),
		"--allowlist", filepath.Join(tdata, "compat-matrix-empty.md"),
		"--fixture", "bare",
	)
	if code != 0 {
		t.Fatalf("exit = %d, want 0 (out: %s)", code, out)
	}
}

func TestRunDiffExits1WithAnnotation(t *testing.T) {
	bin := buildBinary(t)
	tdata, err := filepath.Abs("testdata")
	if err != nil {
		t.Fatal(err)
	}
	out, code := runCLI(t, bin,
		"--ours", filepath.Join(tdata, "probe-bare.json"),
		"--theirs", filepath.Join(tdata, "probe-bare-ini-shift.json"),
		"--allowlist", filepath.Join(tdata, "compat-matrix-empty.md"),
		"--fixture", "bare",
	)
	if code != 1 {
		t.Fatalf("exit = %d, want 1 (out: %s)", code, out)
	}
	if !strings.Contains(out, "::error::") {
		t.Errorf("expected ::error:: annotation, got: %s", out)
	}
	if !strings.Contains(out, "ini.memory_limit") {
		t.Errorf("expected path in output, got: %s", out)
	}
}

func TestRunMalformedAllowlistExits2(t *testing.T) {
	bin := buildBinary(t)
	tdata, err := filepath.Abs("testdata")
	if err != nil {
		t.Fatal(err)
	}
	out, code := runCLI(t, bin,
		"--ours", filepath.Join(tdata, "probe-bare.json"),
		"--theirs", filepath.Join(tdata, "probe-bare.json"),
		"--allowlist", filepath.Join(tdata, "compat-matrix-malformed.md"),
		"--fixture", "bare",
	)
	if code != 2 {
		t.Fatalf("exit = %d, want 2 (out: %s)", code, out)
	}
	_ = out
}

func TestRunMissingProbeExits2(t *testing.T) {
	bin := buildBinary(t)
	tdata, err := filepath.Abs("testdata")
	if err != nil {
		t.Fatal(err)
	}
	missing := filepath.Join(t.TempDir(), "does-not-exist.json")
	out, code := runCLI(t, bin,
		"--ours", missing,
		"--theirs", filepath.Join(tdata, "probe-bare.json"),
		"--allowlist", filepath.Join(tdata, "compat-matrix-empty.md"),
		"--fixture", "bare",
	)
	if code != 2 {
		t.Fatalf("exit = %d, want 2 (out: %s)", code, out)
	}
}

// Unit tests for parseFlags and run() to achieve package-level coverage >= 80%.

func TestParseFlagsAllPresent(t *testing.T) {
	args := cliArgs{
		ours:      "a.json",
		theirs:    "b.json",
		allowlist: "al.md",
		fixture:   "bare",
	}
	got, code := parseFlags([]string{
		"--ours", args.ours,
		"--theirs", args.theirs,
		"--allowlist", args.allowlist,
		"--fixture", args.fixture,
	}, os.Stderr)
	if code != 0 {
		t.Fatalf("parseFlags returned code %d, want 0", code)
	}
	if got != args {
		t.Errorf("parseFlags = %+v, want %+v", got, args)
	}
}

func TestParseFlagsMissingField(t *testing.T) {
	_, code := parseFlags([]string{"--ours", "a.json"}, os.Stderr)
	if code != exitMalformed {
		t.Fatalf("parseFlags code = %d, want %d", code, exitMalformed)
	}
}

func TestParseFlagsUnknownFlag(t *testing.T) {
	_, code := parseFlags([]string{"--bogus"}, os.Stderr)
	if code != exitMalformed {
		t.Fatalf("parseFlags code = %d, want %d", code, exitMalformed)
	}
}

func TestRunOursNotFound(t *testing.T) {
	tdata, err := filepath.Abs("testdata")
	if err != nil {
		t.Fatal(err)
	}
	missing := filepath.Join(t.TempDir(), "does-not-exist.json")
	args := cliArgs{
		ours:      missing,
		theirs:    filepath.Join(tdata, "probe-bare.json"),
		allowlist: filepath.Join(tdata, "compat-matrix-empty.md"),
		fixture:   "bare",
	}
	code := run(args, os.Stdout, os.Stderr)
	if code != exitMalformed {
		t.Fatalf("run code = %d, want %d", code, exitMalformed)
	}
}

func TestRunTheirsNotFound(t *testing.T) {
	tdata, err := filepath.Abs("testdata")
	if err != nil {
		t.Fatal(err)
	}
	missing := filepath.Join(t.TempDir(), "does-not-exist.json")
	args := cliArgs{
		ours:      filepath.Join(tdata, "probe-bare.json"),
		theirs:    missing,
		allowlist: filepath.Join(tdata, "compat-matrix-empty.md"),
		fixture:   "bare",
	}
	code := run(args, os.Stdout, os.Stderr)
	if code != exitMalformed {
		t.Fatalf("run code = %d, want %d", code, exitMalformed)
	}
}

func TestRunAllowlistNotFound(t *testing.T) {
	tdata, err := filepath.Abs("testdata")
	if err != nil {
		t.Fatal(err)
	}
	missing := filepath.Join(t.TempDir(), "does-not-exist.md")
	args := cliArgs{
		ours:      filepath.Join(tdata, "probe-bare.json"),
		theirs:    filepath.Join(tdata, "probe-bare.json"),
		allowlist: missing,
		fixture:   "bare",
	}
	code := run(args, os.Stdout, os.Stderr)
	if code != exitMalformed {
		t.Fatalf("run code = %d, want %d", code, exitMalformed)
	}
}

func TestRunExactMatchUnit(t *testing.T) {
	tdata, err := filepath.Abs("testdata")
	if err != nil {
		t.Fatal(err)
	}
	args := cliArgs{
		ours:      filepath.Join(tdata, "probe-bare.json"),
		theirs:    filepath.Join(tdata, "probe-bare.json"),
		allowlist: filepath.Join(tdata, "compat-matrix-empty.md"),
		fixture:   "bare",
	}
	code := run(args, os.Stdout, os.Stderr)
	if code != exitMatch {
		t.Fatalf("run code = %d, want %d", code, exitMatch)
	}
}

func TestRunDiffUnit(t *testing.T) {
	tdata, err := filepath.Abs("testdata")
	if err != nil {
		t.Fatal(err)
	}
	args := cliArgs{
		ours:      filepath.Join(tdata, "probe-bare.json"),
		theirs:    filepath.Join(tdata, "probe-bare-ini-shift.json"),
		allowlist: filepath.Join(tdata, "compat-matrix-empty.md"),
		fixture:   "bare",
	}
	code := run(args, os.Stdout, os.Stderr)
	if code != exitDiff {
		t.Fatalf("run code = %d, want %d", code, exitDiff)
	}
}

func TestRunDiffAnnotationsToStderr(t *testing.T) {
	tdata, err := filepath.Abs("testdata")
	if err != nil {
		t.Fatal(err)
	}
	tmpdir := t.TempDir()
	stdoutFile, err := os.Create(filepath.Join(tmpdir, "stdout"))
	if err != nil {
		t.Fatal(err)
	}
	defer stdoutFile.Close()
	stderrFile, err := os.Create(filepath.Join(tmpdir, "stderr"))
	if err != nil {
		t.Fatal(err)
	}
	defer stderrFile.Close()

	args := cliArgs{
		ours:      filepath.Join(tdata, "probe-bare.json"),
		theirs:    filepath.Join(tdata, "probe-bare-ini-shift.json"),
		allowlist: filepath.Join(tdata, "compat-matrix-empty.md"),
		fixture:   "bare",
	}
	code := run(args, stdoutFile, stderrFile)
	if code != exitDiff {
		t.Fatalf("run code = %d, want %d", code, exitDiff)
	}

	// Read back the files
	stdoutBytes, err := os.ReadFile(stdoutFile.Name())
	if err != nil {
		t.Fatal(err)
	}
	stderrBytes, err := os.ReadFile(stderrFile.Name())
	if err != nil {
		t.Fatal(err)
	}

	stdout := string(stdoutBytes)
	stderr := string(stderrBytes)

	if !strings.Contains(stderr, "::error::") {
		t.Errorf("expected ::error:: annotation in stderr, got: %s", stderr)
	}
	if strings.Contains(stdout, "::error::") {
		t.Errorf("unexpected ::error:: annotation in stdout, got: %s", stdout)
	}
	if !strings.Contains(stdout, "compat-diff: fixture=bare FAIL") {
		t.Errorf("expected summary line in stdout, got: %s", stdout)
	}
	if !strings.Contains(stderr, "fixture=bare") {
		t.Errorf("expected fixture name in stderr error annotation, got: %s", stderr)
	}
}
