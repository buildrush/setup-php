package planner

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHashFiles_Deterministic_AcrossCalls(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a.txt")
	b := filepath.Join(dir, "b.txt")
	if err := os.WriteFile(a, []byte("alpha"), 0o600); err != nil {
		t.Fatalf("write a: %v", err)
	}
	if err := os.WriteFile(b, []byte("beta"), 0o600); err != nil {
		t.Fatalf("write b: %v", err)
	}

	h1, err := HashFiles([]string{a, b})
	if err != nil {
		t.Fatalf("HashFiles 1: %v", err)
	}
	h2, err := HashFiles([]string{a, b})
	if err != nil {
		t.Fatalf("HashFiles 2: %v", err)
	}
	if h1 != h2 {
		t.Errorf("HashFiles not deterministic:\n%s\n---\n%s", h1, h2)
	}
}

// The planner relied historically on colon-joined per-file sha256 strings for
// the builder hash. HashFiles must reproduce that exact shape or spec_hash
// values would silently change across the refactor.
func TestHashFiles_MatchesColonJoinedHashFile(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a.txt")
	b := filepath.Join(dir, "b.txt")
	c := filepath.Join(dir, "c.txt")
	if err := os.WriteFile(a, []byte("alpha"), 0o600); err != nil {
		t.Fatalf("write a: %v", err)
	}
	if err := os.WriteFile(b, []byte("beta"), 0o600); err != nil {
		t.Fatalf("write b: %v", err)
	}
	if err := os.WriteFile(c, []byte("gamma"), 0o600); err != nil {
		t.Fatalf("write c: %v", err)
	}

	ha, _ := HashFile(a)
	hb, _ := HashFile(b)
	hc, _ := HashFile(c)
	want := ha + ":" + hb + ":" + hc

	got, err := HashFiles([]string{a, b, c})
	if err != nil {
		t.Fatalf("HashFiles: %v", err)
	}
	if got != want {
		t.Errorf("HashFiles = %q, want %q", got, want)
	}
}

func TestHashFiles_ChangesWhenFileChanges(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "f.txt")
	if err := os.WriteFile(p, []byte("v1"), 0o600); err != nil {
		t.Fatalf("write v1: %v", err)
	}
	h1, err := HashFiles([]string{p})
	if err != nil {
		t.Fatalf("HashFiles v1: %v", err)
	}
	if err := os.WriteFile(p, []byte("v2"), 0o600); err != nil {
		t.Fatalf("write v2: %v", err)
	}
	h2, err := HashFiles([]string{p})
	if err != nil {
		t.Fatalf("HashFiles v2: %v", err)
	}
	if h1 == h2 {
		t.Errorf("HashFiles should differ after mutation, both = %q", h1)
	}
}

func TestHashFiles_OrderSensitive(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a.txt")
	b := filepath.Join(dir, "b.txt")
	if err := os.WriteFile(a, []byte("alpha"), 0o600); err != nil {
		t.Fatalf("write a: %v", err)
	}
	if err := os.WriteFile(b, []byte("beta"), 0o600); err != nil {
		t.Fatalf("write b: %v", err)
	}
	hab, _ := HashFiles([]string{a, b})
	hba, _ := HashFiles([]string{b, a})
	if hab == hba {
		t.Errorf("HashFiles should be order-sensitive, both = %q", hab)
	}
}

func TestHashFiles_MissingFile_Errors(t *testing.T) {
	dir := t.TempDir()
	if _, err := HashFiles([]string{filepath.Join(dir, "nope")}); err == nil {
		t.Error("HashFiles should error on missing file")
	}
}

func TestPerVersionYAMLFromFile_ExtractsVersion(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "php.yaml")
	content := `name: php
versions:
  "8.3":
    bundled_extensions: [core]
    sources:
      url: https://example/php-8.3.tar.xz
    abi_matrix:
      os: [linux]
      arch: [x86_64]
      ts: [nts]
  "8.4":
    bundled_extensions: [core, opcache]
    sources:
      url: https://example/php-8.4.tar.xz
    abi_matrix:
      os: [linux]
      arch: [x86_64]
      ts: [nts]
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	got, err := PerVersionYAMLFromFile(path, "8.4")
	if err != nil {
		t.Fatalf("PerVersionYAMLFromFile: %v", err)
	}
	s := string(got)
	// Per-version YAML contains only the one version's subtree — it should
	// mention the url for 8.4 but not 8.3's url, since 8.3 is a sibling key
	// at the parent that gets filtered out by the per-version marshaling.
	if !strings.Contains(s, "php-8.4") {
		t.Errorf("expected 8.4 url in output:\n%s", s)
	}
	if strings.Contains(s, "php-8.3") {
		t.Errorf("per-version yaml must not leak sibling version:\n%s", s)
	}
}

func TestPerVersionYAMLFromFile_MissingVersion_Errors(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "php.yaml")
	content := `name: php
versions:
  "8.4":
    bundled_extensions: [core]
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := PerVersionYAMLFromFile(path, "9.9"); err == nil {
		t.Error("missing version must error")
	}
}

func TestPerVersionYAMLFromFile_MissingFile_Errors(t *testing.T) {
	dir := t.TempDir()
	if _, err := PerVersionYAMLFromFile(filepath.Join(dir, "nope.yaml"), "8.4"); err == nil {
		t.Error("missing file must error")
	}
}

func TestExtensionYAMLFromFile_ReadsRawBytes(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "redis.yaml")
	content := `name: redis
kind: pecl
source:
  pecl_package: redis
versions:
  - "6.2.0"
abi_matrix:
  php: ["8.4"]
  os: ["linux"]
  arch: ["x86_64"]
  ts: ["nts"]
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	got, err := ExtensionYAMLFromFile(path)
	if err != nil {
		t.Fatalf("ExtensionYAMLFromFile: %v", err)
	}
	s := string(got)
	if !strings.Contains(s, "redis") {
		t.Errorf("expected redis in marshaled yaml:\n%s", s)
	}
	if !strings.Contains(s, "6.2.0") {
		t.Errorf("expected version in marshaled yaml:\n%s", s)
	}
}

func TestExtensionYAMLFromFile_MissingFile_Errors(t *testing.T) {
	dir := t.TempDir()
	if _, err := ExtensionYAMLFromFile(filepath.Join(dir, "nope.yaml")); err == nil {
		t.Error("missing file must error")
	}
}

func TestParseEnvValue_FindsKey(t *testing.T) {
	data := []byte("BUILDER_OS=ubuntu-22.04\n")
	if got := ParseEnvValue(data, "BUILDER_OS"); got != "ubuntu-22.04" {
		t.Errorf("ParseEnvValue = %q, want ubuntu-22.04", got)
	}
}

func TestParseEnvValue_MissingKey_ReturnsEmpty(t *testing.T) {
	data := []byte("OTHER=value\n")
	if got := ParseEnvValue(data, "BUILDER_OS"); got != "" {
		t.Errorf("ParseEnvValue for missing key = %q, want empty", got)
	}
}

func TestParseEnvValue_IgnoresComments(t *testing.T) {
	data := []byte("# BUILDER_OS=ignored\nBUILDER_OS=ubuntu-24.04\n")
	if got := ParseEnvValue(data, "BUILDER_OS"); got != "ubuntu-24.04" {
		t.Errorf("ParseEnvValue = %q, want ubuntu-24.04", got)
	}
}

func TestParseEnvValue_IgnoresBlankLines(t *testing.T) {
	data := []byte("\n\nBUILDER_OS=ubuntu-22.04\n\n")
	if got := ParseEnvValue(data, "BUILDER_OS"); got != "ubuntu-22.04" {
		t.Errorf("ParseEnvValue = %q, want ubuntu-22.04", got)
	}
}

func TestParseEnvValue_FirstMatchWins(t *testing.T) {
	data := []byte("BUILDER_OS=first\nBUILDER_OS=second\n")
	if got := ParseEnvValue(data, "BUILDER_OS"); got != "first" {
		t.Errorf("ParseEnvValue = %q, want first (first match wins)", got)
	}
}

func TestParseEnvValue_TrimsWhitespace(t *testing.T) {
	// The inline planner parser used strings.TrimSpace before prefix-matching,
	// so leading/trailing whitespace around the whole line should be tolerated.
	data := []byte("   BUILDER_OS=ubuntu-22.04   \n")
	if got := ParseEnvValue(data, "BUILDER_OS"); got != "ubuntu-22.04" {
		t.Errorf("ParseEnvValue = %q, want ubuntu-22.04", got)
	}
}
