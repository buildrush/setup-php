// cmd/compat-diff/main_test.go
package main

import (
	"os/exec"
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
