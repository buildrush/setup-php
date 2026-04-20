package compat_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"
)

func TestProbeBasic(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("probe.sh is bash-only")
	}
	tmp := t.TempDir()
	repoRoot, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	probe := filepath.Join(repoRoot, "test", "compat", "probe.sh")
	stubSrc := filepath.Join(repoRoot, "test", "compat", "testdata", "probe", "stub-php.sh")
	iniKeys := filepath.Join(repoRoot, "test", "compat", "ini-keys.txt")
	goldenPath := filepath.Join(repoRoot, "test", "compat", "testdata", "probe", "golden-basic.json")

	binDir := filepath.Join(tmp, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	phpBin := filepath.Join(binDir, "php")
	if err := copyFile(stubSrc, phpBin, 0o755); err != nil {
		t.Fatal(err)
	}

	envBefore := filepath.Join(tmp, "env-before")
	pathBefore := filepath.Join(tmp, "path-before")
	if err := os.WriteFile(envBefore, []byte("HOME=/home/runner\nEXISTING_VAR=one\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(pathBefore, []byte(binDir+":/usr/bin:/bin"), 0o644); err != nil {
		t.Fatal(err)
	}

	outPath := filepath.Join(tmp, "out.json")
	cmd := exec.Command("bash", probe, outPath, envBefore, pathBefore, iniKeys)
	cmd.Env = []string{
		"PATH=" + binDir + ":/usr/bin:/bin",
		"HOME=/home/runner",
		"EXISTING_VAR=one",
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("probe: %v\n%s", err, out)
	}
	got := readJSON(t, outPath)
	want := readJSON(t, goldenPath)
	if !reflect.DeepEqual(got, want) {
		gotBytes, _ := json.MarshalIndent(got, "", "  ")
		wantBytes, _ := json.MarshalIndent(want, "", "  ")
		t.Fatalf("probe JSON mismatch\n--- got ---\n%s\n--- want ---\n%s", gotBytes, wantBytes)
	}
}

func copyFile(src, dst string, mode os.FileMode) error {
	b, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, b, mode)
}

func readJSON(t *testing.T, path string) map[string]any {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var v map[string]any
	if err := json.Unmarshal(b, &v); err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	return v
}

func TestProbeEscapesControlChars(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("probe.sh is bash-only")
	}
	tmp := t.TempDir()
	repoRoot, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	probe := filepath.Join(repoRoot, "test", "compat", "probe.sh")
	stubSrc := filepath.Join(repoRoot, "test", "compat", "testdata", "probe", "stub-php-newlines.sh")
	envBefore := filepath.Join(repoRoot, "test", "compat", "testdata", "probe", "env-before")
	pathBefore := filepath.Join(repoRoot, "test", "compat", "testdata", "probe", "path-before")
	iniKeys := filepath.Join(repoRoot, "test", "compat", "ini-keys.txt")

	binDir := filepath.Join(tmp, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	phpBin := filepath.Join(binDir, "php")
	if err := copyFile(stubSrc, phpBin, 0o755); err != nil {
		t.Fatal(err)
	}

	outPath := filepath.Join(tmp, "out.json")
	cmd := exec.Command("bash", probe, outPath, envBefore, pathBefore, iniKeys)
	cmd.Env = append(os.Environ(), "PATH="+binDir+":/usr/bin:/bin")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("probe: %v\n%s", err, out)
	}
	got := readJSON(t, outPath)
	ini, ok := got["ini"].(map[string]any)
	if !ok {
		t.Fatalf("ini field missing or wrong type: %v", got["ini"])
	}
	disabled, ok := ini["disable_functions"].(string)
	if !ok {
		t.Fatalf("disable_functions missing or wrong type: %v", ini["disable_functions"])
	}
	want := "eval\nexec"
	if disabled != want {
		t.Fatalf("disable_functions: got %q, want %q", disabled, want)
	}
}

func TestProbeEnvAndPathDeltas(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("probe.sh is bash-only")
	}
	tmp := t.TempDir()
	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	probe := filepath.Join(repoRoot, "test", "compat", "probe.sh")
	stubSrc := filepath.Join(repoRoot, "test", "compat", "testdata", "probe", "stub-php.sh")
	iniKeys := filepath.Join(repoRoot, "test", "compat", "ini-keys.txt")

	binDir := filepath.Join(tmp, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	phpBin := filepath.Join(binDir, "php")
	if err := copyFile(stubSrc, phpBin, 0o755); err != nil {
		t.Fatal(err)
	}

	// Synthesize "before" snapshots where the probe's current env/PATH differs.
	envBefore := filepath.Join(tmp, "env-before")
	pathBefore := filepath.Join(tmp, "path-before")
	// binDir must be in the "before" PATH, so it is NOT counted as an addition.
	if err := os.WriteFile(pathBefore, []byte(binDir+":/usr/bin:/bin"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(envBefore, []byte("HOME=/x\nEXISTING_VAR=one\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	outPath := filepath.Join(tmp, "out.json")
	cmd := exec.Command("bash", probe, outPath, envBefore, pathBefore, iniKeys)
	cmd.Env = []string{
		"PATH=" + binDir + ":/opt/hostedtoolcache/PHP/8.4.5/x64:/usr/bin:/bin",
		"HOME=/x",
		"EXISTING_VAR=one",
		"PHP_INI_SCAN_DIR=/etc/php/conf.d",
		"SETUP_PHP_TOOL_CACHE_DIR=/tmp",
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("probe: %v\n%s", err, out)
	}
	got := readJSON(t, outPath)

	wantEnvDelta := []any{"PHP_INI_SCAN_DIR", "SETUP_PHP_TOOL_CACHE_DIR"}
	if !reflect.DeepEqual(got["env_delta"], wantEnvDelta) {
		t.Errorf("env_delta: got %v, want %v", got["env_delta"], wantEnvDelta)
	}

	wantPathAdditions := []any{"<PHP_ROOT>/x64"}
	if !reflect.DeepEqual(got["path_additions"], wantPathAdditions) {
		t.Errorf("path_additions: got %v, want %v", got["path_additions"], wantPathAdditions)
	}
}
