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
	envBefore := filepath.Join(repoRoot, "test", "compat", "testdata", "probe", "env-before")
	pathBefore := filepath.Join(repoRoot, "test", "compat", "testdata", "probe", "path-before")
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

	outPath := filepath.Join(tmp, "out.json")
	cmd := exec.Command("bash", probe, outPath, envBefore, pathBefore, iniKeys)
	cmd.Env = append(os.Environ(), "PATH="+binDir+":/usr/bin:/bin")
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
