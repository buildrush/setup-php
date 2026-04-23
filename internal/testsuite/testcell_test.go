package testsuite

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// fakeProbeScript writes a bash script at <dir>/probe.sh that, when invoked,
// writes a synthetic probe JSON to its $1 argument. The JSON mimics what
// the real probe.sh would emit for a fixed PHP environment (version,
// extensions, ini-values). Tests use this to exercise RunTestCell's
// orchestration path without needing a real PHP binary.
func fakeProbeScript(t *testing.T, dir, php string, exts []string, iniKV map[string]string) string {
	t.Helper()
	extQuoted := make([]string, 0, len(exts))
	for _, e := range exts {
		extQuoted = append(extQuoted, `"`+e+`"`)
	}
	iniPairs := make([]string, 0, len(iniKV))
	for k, v := range iniKV {
		iniPairs = append(iniPairs, `"`+k+`":"`+v+`"`)
	}
	script := `#!/usr/bin/env bash
set -euo pipefail
cat > "$1" <<EOF
{
  "php_version": "` + php + `",
  "sapi": "cli",
  "zts": false,
  "extensions": [` + strings.Join(extQuoted, ",") + `],
  "ini": {` + strings.Join(iniPairs, ",") + `},
  "env_delta": [],
  "path_additions": []
}
EOF
`
	probePath := filepath.Join(dir, "probe.sh")
	// 0o600 is fine because testcell.go invokes probe.sh via `bash <path> …`,
	// so no exec bit is required on the script itself.
	if err := os.WriteFile(probePath, []byte(script), 0o600); err != nil {
		t.Fatal(err)
	}
	return probePath
}

// fakeInstallOK does nothing and returns nil. Probe output is synthetic
// in tests, so no real install work needs to happen.
func fakeInstallOK(_ context.Context, _ map[string]string, _, _ io.Writer) error {
	return nil
}

// fakeInstallFail simulates an install failure: writes a synthetic error
// to stderr and returns a non-nil error. Used to assert that install
// failures propagate into the fixtureOutcome.
func fakeInstallFail(_ context.Context, _ map[string]string, _, stderr io.Writer) error {
	_, _ = stderr.Write([]byte("synthetic install failure\n"))
	return errors.New("install boom")
}

// writeTestCellFixtures serializes a slice of Fixture into a synthetic
// fixtures.yaml at <dir>/fixtures.yaml and returns the path. Used by the
// RunTestCell tests to control the fixture set per test case.
func writeTestCellFixtures(t *testing.T, dir string, entries []Fixture) string {
	t.Helper()
	path := filepath.Join(dir, "fixtures.yaml")
	fs := FixtureSet{Fixtures: entries}
	out := "fixtures:\n"
	for i := range fs.Fixtures {
		f := &fs.Fixtures[i]
		out += "  - name: " + f.Name + "\n"
		out += `    php-version: "` + f.PHPVersion + `"` + "\n"
		out += `    extensions: "` + f.Extensions + `"` + "\n"
		out += `    ini-values: "` + f.INIValues + `"` + "\n"
		out += `    coverage: ` + f.Coverage + "\n"
		if f.INIFile != "" {
			out += `    ini-file: ` + f.INIFile + "\n"
		}
		if f.Arch != "" {
			out += `    arch: ` + f.Arch + "\n"
		}
	}
	if err := os.WriteFile(path, []byte(out), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestRunTestCell_AllPass(t *testing.T) {
	dir := t.TempDir()
	fx := writeTestCellFixtures(t, dir, []Fixture{
		{Name: "bare", PHPVersion: "8.4", Coverage: "none"},
		{Name: "with-redis", PHPVersion: "8.4", Extensions: "redis", Coverage: "none"},
	})
	probe := fakeProbeScript(t, dir, "8.4.20", []string{"core", "redis"}, map[string]string{"memory_limit": "-1"})

	restore := SetInstaller(fakeInstallOK)
	defer restore()

	err := RunTestCell(context.Background(), []string{
		"--os", "jammy", "--arch", "x86_64", "--php", "8.4",
		"--fixtures", fx, "--probe", probe,
	})
	if err != nil {
		t.Fatalf("RunTestCell: %v", err)
	}
}

func TestRunTestCell_FixtureInvariantFails_ExtensionMissing(t *testing.T) {
	dir := t.TempDir()
	fx := writeTestCellFixtures(t, dir, []Fixture{
		{Name: "expects-redis", PHPVersion: "8.4", Extensions: "redis", Coverage: "none"},
	})
	// Probe returns only "core" — redis missing → fixture should fail.
	probe := fakeProbeScript(t, dir, "8.4.20", []string{"core"}, nil)

	restore := SetInstaller(fakeInstallOK)
	defer restore()

	err := RunTestCell(context.Background(), []string{
		"--os", "jammy", "--arch", "x86_64", "--php", "8.4",
		"--fixtures", fx, "--probe", probe,
	})
	if err == nil || !strings.Contains(err.Error(), "fixture(s) failed") {
		t.Fatalf("RunTestCell err = %v, want mention fixture(s) failed", err)
	}
}

func TestRunTestCell_INIValueMismatch_Fails(t *testing.T) {
	dir := t.TempDir()
	fx := writeTestCellFixtures(t, dir, []Fixture{
		{Name: "memory", PHPVersion: "8.4", INIValues: "memory_limit=256M", Coverage: "none"},
	})
	probe := fakeProbeScript(t, dir, "8.4.20", []string{"core"}, map[string]string{"memory_limit": "-1"})

	restore := SetInstaller(fakeInstallOK)
	defer restore()

	err := RunTestCell(context.Background(), []string{
		"--os", "jammy", "--arch", "x86_64", "--php", "8.4",
		"--fixtures", fx, "--probe", probe,
	})
	if err == nil || !strings.Contains(err.Error(), "fixture(s) failed") {
		t.Fatalf("RunTestCell err = %v, want failure", err)
	}
}

func TestRunTestCell_InstallFails_Propagates(t *testing.T) {
	dir := t.TempDir()
	fx := writeTestCellFixtures(t, dir, []Fixture{
		{Name: "bare", PHPVersion: "8.4", Coverage: "none"},
	})
	probe := fakeProbeScript(t, dir, "8.4.20", []string{"core"}, nil)

	restore := SetInstaller(fakeInstallFail)
	defer restore()

	err := RunTestCell(context.Background(), []string{
		"--os", "jammy", "--arch", "x86_64", "--php", "8.4",
		"--fixtures", fx, "--probe", probe,
	})
	if err == nil || !strings.Contains(err.Error(), "1 of 1 fixture(s) failed") {
		t.Fatalf("RunTestCell err = %v, want 1 of 1 failure", err)
	}
}

func TestRunTestCell_NoFixturesForCell_NoError(t *testing.T) {
	dir := t.TempDir()
	fx := writeTestCellFixtures(t, dir, []Fixture{
		{Name: "other", PHPVersion: "8.5", Coverage: "none"},
	})
	probe := fakeProbeScript(t, dir, "8.4.20", []string{"core"}, nil)

	restore := SetInstaller(fakeInstallOK)
	defer restore()

	err := RunTestCell(context.Background(), []string{
		"--os", "jammy", "--arch", "x86_64", "--php", "8.4",
		"--fixtures", fx, "--probe", probe,
	})
	if err != nil {
		t.Errorf("RunTestCell no-match = %v, want nil", err)
	}
}

func TestRunTestCell_MissingFlags_Errors(t *testing.T) {
	for _, args := range [][]string{
		{"--arch", "x86_64", "--php", "8.4"},
		{"--os", "jammy", "--php", "8.4"},
		{"--os", "jammy", "--arch", "x86_64"},
	} {
		err := RunTestCell(context.Background(), args)
		if err == nil {
			t.Errorf("RunTestCell(%v) want error", args)
		}
	}
}

func TestComposeEnv_OverlayReplaces(t *testing.T) {
	base := []string{"FOO=bar", "BAZ=qux"}
	overlay := map[string]string{"FOO": "new", "NEW": "val"}
	got := composeEnv(base, overlay)
	want := []string{"BAZ=qux", "FOO=new", "NEW=val"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("composeEnv = %v, want %v", got, want)
	}
}

func TestParseExtensionsList(t *testing.T) {
	cases := []struct {
		in, wantW, wantE string
	}{
		{"", "", ""},
		{"none", "", ""},
		{"redis,grpc", "redis,grpc", ""},
		{"redis,:opcache", "redis", "opcache"},
		{"none,redis,:xdebug,grpc", "redis,grpc", "xdebug"},
	}
	for _, c := range cases {
		w, e := parseExtensionsList(c.in)
		if strings.Join(w, ",") != c.wantW || strings.Join(e, ",") != c.wantE {
			t.Errorf("parseExtensionsList(%q) = (%v, %v), want (%q, %q)", c.in, w, e, c.wantW, c.wantE)
		}
	}
}

func TestAssertFixtureInvariants_PHPVersionMismatch(t *testing.T) {
	probe := map[string]any{"php_version": "8.2.10", "extensions": []any{"core"}, "ini": map[string]any{}}
	err := assertFixtureInvariants(&Fixture{PHPVersion: "8.4"}, probe)
	if err == nil || !strings.Contains(err.Error(), "php_version") {
		t.Errorf("err = %v, want php_version mismatch", err)
	}
}

func TestAssertFixtureInvariants_ExcludedExtStillLoaded(t *testing.T) {
	probe := map[string]any{"php_version": "8.4.0", "extensions": []any{"core", "opcache"}, "ini": map[string]any{}}
	err := assertFixtureInvariants(&Fixture{PHPVersion: "8.4", Extensions: ":opcache"}, probe)
	if err == nil || !strings.Contains(err.Error(), "excluded") {
		t.Errorf("err = %v, want excluded-but-loaded", err)
	}
}

func TestWriteEnvSnapshot_DeterministicOrder(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "env")
	if err := writeEnvSnapshot(path); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	sorted := append([]string(nil), lines...)
	for i := 1; i < len(sorted); i++ {
		if sorted[i] < sorted[i-1] {
			t.Errorf("line %d not sorted: %q < %q", i, sorted[i], sorted[i-1])
		}
	}
}

func TestProbeOutputRoundTrip_JSONParses(t *testing.T) {
	// Sanity: the synthetic probe output shape parses as map[string]any.
	sample := `{"php_version":"8.4.0","sapi":"cli","zts":false,"extensions":["core"],"ini":{},"env_delta":[],"path_additions":[]}`
	var got map[string]any
	if err := json.Unmarshal([]byte(sample), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got["php_version"] != "8.4.0" {
		t.Error("parse mismatch")
	}
}

func TestComposeInstallEnv_SetsRunnerEnv(t *testing.T) {
	opts := &testCellOpts{Arch: "x86_64", RegistryURI: "oci-layout:/reg"}
	env := composeInstallEnv(opts, &Fixture{PHPVersion: "8.4", Extensions: "", Coverage: "none"})
	if env["RUNNER_OS"] != "Linux" {
		t.Errorf("RUNNER_OS = %q, want Linux", env["RUNNER_OS"])
	}
	if env["RUNNER_ARCH"] != "X64" {
		t.Errorf("RUNNER_ARCH = %q, want X64", env["RUNNER_ARCH"])
	}
	// Aarch64 mapping.
	opts.Arch = "aarch64"
	env = composeInstallEnv(opts, &Fixture{PHPVersion: "8.4"})
	if env["RUNNER_ARCH"] != "ARM64" {
		t.Errorf("RUNNER_ARCH aarch64 = %q, want ARM64", env["RUNNER_ARCH"])
	}
}

func TestPrintFixtureSummary_IncludesAllRows(t *testing.T) {
	var buf bytes.Buffer
	printFixtureSummary(&buf, []fixtureOutcome{
		{Name: "alpha"},
		{Name: "beta", Err: errors.New("nope")},
	})
	out := buf.String()
	for _, want := range []string{"alpha", "OK", "beta", "FAIL: nope"} {
		if !strings.Contains(out, want) {
			t.Errorf("summary missing %q in:\n%s", want, out)
		}
	}
}
