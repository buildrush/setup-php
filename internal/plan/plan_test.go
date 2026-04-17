package plan

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFromEnvFull(t *testing.T) {
	t.Setenv("INPUT_PHP-VERSION", "8.4")
	t.Setenv("INPUT_EXTENSIONS", "mbstring, redis, curl")
	t.Setenv("INPUT_INI-VALUES", "memory_limit=256M, display_errors=On")
	t.Setenv("INPUT_COVERAGE", "xdebug")
	t.Setenv("INPUT_TOOLS", "composer, phpunit")
	t.Setenv("RUNNER_OS", "Linux")
	t.Setenv("RUNNER_ARCH", "X64")

	p, err := FromEnv()
	if err != nil {
		t.Fatalf("FromEnv() error = %v", err)
	}
	if p.PHPVersion != "8.4" {
		t.Errorf("PHPVersion = %q, want 8.4", p.PHPVersion)
	}
	if len(p.Extensions) != 3 {
		t.Errorf("len(Extensions) = %d, want 3", len(p.Extensions))
	}
	if p.Coverage != CoverageXdebug {
		t.Errorf("Coverage = %q, want xdebug", p.Coverage)
	}
	if p.OS != "linux" {
		t.Errorf("OS = %q, want linux", p.OS)
	}
	if p.Arch != "x86_64" {
		t.Errorf("Arch = %q, want x86_64", p.Arch)
	}
}

func TestFromEnvDefaults(t *testing.T) {
	for _, key := range []string{"INPUT_PHP-VERSION", "INPUT_EXTENSIONS", "INPUT_INI-VALUES", "INPUT_COVERAGE", "INPUT_TOOLS"} {
		t.Setenv(key, "")
	}
	t.Setenv("RUNNER_OS", "Linux")
	t.Setenv("RUNNER_ARCH", "X64")

	p, err := FromEnv()
	if err != nil {
		t.Fatalf("FromEnv() error = %v", err)
	}
	if p.PHPVersion != "8.4" {
		t.Errorf("PHPVersion = %q, want 8.4 (default)", p.PHPVersion)
	}
	if p.Coverage != CoverageNone {
		t.Errorf("Coverage = %q, want none (default)", p.Coverage)
	}
	if p.ThreadSafety != "nts" {
		t.Errorf("ThreadSafety = %q, want nts (default)", p.ThreadSafety)
	}
}

func TestParseExtensions(t *testing.T) {
	tests := []struct {
		input string
		want  []string
	}{
		{"mbstring, redis, curl", []string{"curl", "mbstring", "redis"}},
		{"Redis, MBSTRING, curl", []string{"curl", "mbstring", "redis"}},
		{" redis , redis , curl ", []string{"curl", "redis"}},
		{"", nil},
		{"none", nil},
	}
	for _, tt := range tests {
		got := ParseExtensions(tt.input)
		if len(got) != len(tt.want) {
			t.Errorf("ParseExtensions(%q) = %v, want %v", tt.input, got, tt.want)
			continue
		}
		for i := range got {
			if got[i] != tt.want[i] {
				t.Errorf("ParseExtensions(%q)[%d] = %q, want %q", tt.input, i, got[i], tt.want[i])
			}
		}
	}
}

func TestParseIniValues(t *testing.T) {
	vals, err := ParseIniValues("memory_limit=256M, display_errors=On")
	if err != nil {
		t.Fatalf("ParseIniValues() error = %v", err)
	}
	if len(vals) != 2 {
		t.Fatalf("len = %d, want 2", len(vals))
	}
	if vals[0].Key != "memory_limit" || vals[0].Value != "256M" {
		t.Errorf("vals[0] = %+v", vals[0])
	}
}

func TestParsePHPVersionFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".php-version")
	os.WriteFile(path, []byte("8.4\n"), 0o644)

	v, err := ParsePHPVersionFile(path)
	if err != nil {
		t.Fatalf("ParsePHPVersionFile() error = %v", err)
	}
	if v != "8.4" {
		t.Errorf("version = %q, want 8.4", v)
	}
}

func TestApplyCoverage(t *testing.T) {
	tests := []struct {
		name     string
		coverage CoverageDriver
		initial  []string
		want     []string
	}{
		{"none is no-op", CoverageNone, []string{"intl"}, []string{"intl"}},
		{"empty is no-op", CoverageDriver(""), []string{"intl"}, []string{"intl"}},
		{"xdebug adds driver", CoverageXdebug, []string{"intl"}, []string{"intl", "xdebug"}},
		{"pcov adds driver", CoveragePCOV, []string{"intl"}, []string{"intl", "pcov"}},
		{"xdebug keeps sort order", CoverageXdebug, []string{"redis"}, []string{"redis", "xdebug"}},
		{"pcov sorted before redis", CoveragePCOV, []string{"redis"}, []string{"pcov", "redis"}},
		{"already present: no duplicate", CoverageXdebug, []string{"xdebug"}, []string{"xdebug"}},
		{"empty extensions + xdebug", CoverageXdebug, nil, []string{"xdebug"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &Plan{Coverage: tt.coverage, Extensions: append([]string(nil), tt.initial...)}
			p.ApplyCoverage()
			if len(p.Extensions) != len(tt.want) {
				t.Fatalf("Extensions = %v, want %v", p.Extensions, tt.want)
			}
			for i := range p.Extensions {
				if p.Extensions[i] != tt.want[i] {
					t.Errorf("Extensions[%d] = %q, want %q", i, p.Extensions[i], tt.want[i])
				}
			}
		})
	}
}

func TestHashDeterminism(t *testing.T) {
	p := &Plan{
		PHPVersion:   "8.4",
		Extensions:   []string{"curl", "mbstring", "redis"},
		OS:           "linux",
		Arch:         "x86_64",
		ThreadSafety: "nts",
	}
	h1 := p.Hash()
	h2 := p.Hash()
	if h1 != h2 {
		t.Errorf("Hash() not deterministic: %q != %q", h1, h2)
	}
	if h1 == "" {
		t.Error("Hash() should not be empty")
	}
}

func TestNormalizeArch(t *testing.T) {
	tests := map[string]string{
		"X64":     "x86_64",
		"x64":     "x86_64",
		"AMD64":   "x86_64",
		"amd64":   "x86_64",
		"x86_64":  "x86_64",
		"ARM64":   "aarch64",
		"arm64":   "aarch64",
		"aarch64": "aarch64",
	}
	for input, want := range tests {
		got := normalizeArch(input)
		if got != want {
			t.Errorf("normalizeArch(%q) = %q, want %q", input, got, want)
		}
	}
}
