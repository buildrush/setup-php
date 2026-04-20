package plan

import (
	"os"
	"path/filepath"
	"slices"
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
	if len(p.ExtensionsExclude) != 0 {
		t.Errorf("len(ExtensionsExclude) = %d, want 0", len(p.ExtensionsExclude))
	}
	if p.ExtensionsReset {
		t.Errorf("ExtensionsReset = true, want false for plain-list input")
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
		name        string
		input       string
		wantInclude []string
		wantExclude []string
		wantReset   bool
	}{
		{"empty", "", nil, nil, false},
		{"plain list", "mbstring, redis, curl", []string{"curl", "mbstring", "redis"}, nil, false},
		{"case and whitespace", " Redis , MBSTRING , curl ", []string{"curl", "mbstring", "redis"}, nil, false},
		{"dedupe", "redis, redis, curl", []string{"curl", "redis"}, nil, false},
		{"single exclusion", ":opcache", nil, []string{"opcache"}, false},
		{"exclusion with includes", "redis, :opcache, curl", []string{"curl", "redis"}, []string{"opcache"}, false},
		{"none alone", "none", nil, nil, true},
		{"none then include", "none, redis, curl", []string{"curl", "redis"}, nil, true},
		{"none then include and exclude", "none, redis, :opcache", []string{"redis"}, []string{"opcache"}, true},
		{"none anywhere resets before it", "redis, none, curl", []string{"curl"}, nil, true},
		{"exclude literal 'none' with :none", ":none", nil, []string{"none"}, false},
		{"bare colon is dropped", ":", nil, nil, false},
		{"uppercase NONE is reset", "NONE, redis", []string{"redis"}, nil, true},
		{"same name in both sets is preserved", "redis, :redis", []string{"redis"}, []string{"redis"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			incl, excl, reset := ParseExtensions(tt.input)
			if !slices.Equal(incl, tt.wantInclude) {
				t.Errorf("include = %v, want %v", incl, tt.wantInclude)
			}
			if !slices.Equal(excl, tt.wantExclude) {
				t.Errorf("exclude = %v, want %v", excl, tt.wantExclude)
			}
			if reset != tt.wantReset {
				t.Errorf("reset = %v, want %v", reset, tt.wantReset)
			}
		})
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
		name        string
		coverage    CoverageDriver
		initial     []string
		wantExts    []string
		wantExclude []string
	}{
		// coverage: none → exclude both drivers, leave extensions untouched
		{"none excludes both drivers", CoverageNone, []string{"intl"}, []string{"intl"}, []string{"pcov", "xdebug"}},
		// unrecognised/empty coverage → no change to either slice
		{"empty is no-op", CoverageDriver(""), []string{"intl"}, []string{"intl"}, nil},
		// coverage: xdebug → adds xdebug, excludes pcov
		{"xdebug adds driver and excludes pcov", CoverageXdebug, []string{"intl"}, []string{"intl", "xdebug"}, []string{"pcov"}},
		// coverage: pcov → adds pcov, excludes xdebug
		{"pcov adds driver and excludes xdebug", CoveragePCOV, []string{"intl"}, []string{"intl", "pcov"}, []string{"xdebug"}},
		{"xdebug keeps sort order", CoverageXdebug, []string{"redis"}, []string{"redis", "xdebug"}, []string{"pcov"}},
		{"pcov sorted before redis", CoveragePCOV, []string{"redis"}, []string{"pcov", "redis"}, []string{"xdebug"}},
		// driver already present: no duplicate added
		{"already present: no duplicate", CoverageXdebug, []string{"xdebug"}, []string{"xdebug"}, []string{"pcov"}},
		{"empty extensions + xdebug", CoverageXdebug, nil, []string{"xdebug"}, []string{"pcov"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &Plan{Coverage: tt.coverage, Extensions: append([]string(nil), tt.initial...)}
			p.ApplyCoverage()
			if !slices.Equal(p.Extensions, tt.wantExts) {
				t.Errorf("Extensions = %v, want %v", p.Extensions, tt.wantExts)
			}
			if !slices.Equal(p.ExtensionsExclude, tt.wantExclude) {
				t.Errorf("ExtensionsExclude = %v, want %v", p.ExtensionsExclude, tt.wantExclude)
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

func TestFromEnvPHPTSDefaults(t *testing.T) {
	t.Setenv("INPUT_PHPTS", "")
	t.Setenv("RUNNER_OS", "Linux")
	t.Setenv("RUNNER_ARCH", "X64")
	p, err := FromEnv()
	if err != nil {
		t.Fatalf("FromEnv: %v", err)
	}
	if p.ThreadSafety != "nts" {
		t.Errorf("ThreadSafety = %q, want nts", p.ThreadSafety)
	}
}

func TestFromEnvPHPTSZTS(t *testing.T) {
	t.Setenv("INPUT_PHPTS", "zts")
	t.Setenv("RUNNER_OS", "Linux")
	t.Setenv("RUNNER_ARCH", "X64")
	p, err := FromEnv()
	if err != nil {
		t.Fatalf("FromEnv: %v", err)
	}
	if p.ThreadSafety != "zts" {
		t.Errorf("ThreadSafety = %q, want zts", p.ThreadSafety)
	}
}

func TestFromEnvUpdate(t *testing.T) {
	t.Setenv("INPUT_UPDATE", "true")
	t.Setenv("RUNNER_OS", "Linux")
	t.Setenv("RUNNER_ARCH", "X64")
	p, err := FromEnv()
	if err != nil {
		t.Fatalf("FromEnv: %v", err)
	}
	if !p.Update {
		t.Errorf("Update = false, want true")
	}
}

func TestFromEnvUpdateDefault(t *testing.T) {
	t.Setenv("INPUT_UPDATE", "")
	t.Setenv("RUNNER_OS", "Linux")
	t.Setenv("RUNNER_ARCH", "X64")
	p, err := FromEnv()
	if err != nil {
		t.Fatalf("FromEnv: %v", err)
	}
	if p.Update {
		t.Errorf("Update default = true, want false")
	}
}

func TestFromEnvFailFast(t *testing.T) {
	t.Setenv("INPUT_FAIL-FAST", "true")
	t.Setenv("RUNNER_OS", "Linux")
	t.Setenv("RUNNER_ARCH", "X64")
	p, err := FromEnv()
	if err != nil {
		t.Fatalf("FromEnv: %v", err)
	}
	if !p.FailFast {
		t.Errorf("FailFast = false, want true")
	}
}

func TestFromEnvIniFileDefault(t *testing.T) {
	t.Setenv("INPUT_INI-FILE", "")
	t.Setenv("RUNNER_OS", "Linux")
	t.Setenv("RUNNER_ARCH", "X64")
	p, err := FromEnv()
	if err != nil {
		t.Fatalf("FromEnv: %v", err)
	}
	if p.IniFile != "production" {
		t.Errorf("IniFile default = %q, want production", p.IniFile)
	}
}

func TestFromEnvIniFileDevelopment(t *testing.T) {
	t.Setenv("INPUT_INI-FILE", "development")
	t.Setenv("RUNNER_OS", "Linux")
	t.Setenv("RUNNER_ARCH", "X64")
	p, err := FromEnv()
	if err != nil {
		t.Fatalf("FromEnv: %v", err)
	}
	if p.IniFile != "development" {
		t.Errorf("IniFile = %q, want development", p.IniFile)
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

func TestApplyCoverage_XdebugAddsAndDisablesPCOV(t *testing.T) {
	p := &Plan{Coverage: CoverageXdebug}
	p.ApplyCoverage()
	if !slices.Contains(p.Extensions, "xdebug") {
		t.Errorf("xdebug not added: %v", p.Extensions)
	}
	if !slices.Contains(p.ExtensionsExclude, "pcov") {
		t.Errorf("pcov not excluded: %v", p.ExtensionsExclude)
	}
	if _, ok := p.ExtraIni["pcov.enabled"]; ok {
		t.Errorf("pcov.enabled should not be set for coverage: xdebug: %v", p.ExtraIni)
	}
}

func TestApplyCoverage_PCOVAddsDisablesXdebugAndSetsIni(t *testing.T) {
	p := &Plan{Coverage: CoveragePCOV}
	p.ApplyCoverage()
	if !slices.Contains(p.Extensions, "pcov") {
		t.Errorf("pcov not added: %v", p.Extensions)
	}
	if !slices.Contains(p.ExtensionsExclude, "xdebug") {
		t.Errorf("xdebug not excluded: %v", p.ExtensionsExclude)
	}
	if p.ExtraIni["pcov.enabled"] != "1" {
		t.Errorf("ExtraIni[pcov.enabled] = %q, want 1", p.ExtraIni["pcov.enabled"])
	}
}

func TestApplyCoverage_NoneDisablesBoth(t *testing.T) {
	p := &Plan{Coverage: CoverageNone}
	p.ApplyCoverage()
	for _, drv := range []string{"xdebug", "pcov"} {
		if !slices.Contains(p.ExtensionsExclude, drv) {
			t.Errorf("%s not excluded under coverage: none: %v", drv, p.ExtensionsExclude)
		}
	}
	if slices.Contains(p.Extensions, "xdebug") || slices.Contains(p.Extensions, "pcov") {
		t.Errorf("drivers must not be added under coverage: none: %v", p.Extensions)
	}
}

func TestApplyCoverage_CoverageDisableWinsOverExplicitInclude(t *testing.T) {
	// extensions: xdebug + coverage: pcov — pcov should be added, and xdebug
	// should be forced into ExtensionsExclude even though the user listed it
	// explicitly. v2 runs disable_extension last, so the user's explicit
	// include loses; we match that behavior.
	p := &Plan{
		Coverage:   CoveragePCOV,
		Extensions: []string{"xdebug"},
	}
	p.ApplyCoverage()
	if !slices.Contains(p.ExtensionsExclude, "xdebug") {
		t.Errorf("xdebug not in ExtensionsExclude under coverage: pcov: %v", p.ExtensionsExclude)
	}
	if !slices.Contains(p.Extensions, "pcov") {
		t.Errorf("pcov not added: %v", p.Extensions)
	}
}

func TestHashIncludesIniFile(t *testing.T) {
	p1 := &Plan{PHPVersion: "8.4", IniFile: "production"}
	p2 := &Plan{PHPVersion: "8.4", IniFile: "development"}
	if p1.Hash() == p2.Hash() {
		t.Errorf("Hash() must differ for different IniFile values; got equal: %q", p1.Hash())
	}
}

func TestApplyCoverage_IdempotentOnRepeatedCalls(t *testing.T) {
	p := &Plan{Coverage: CoveragePCOV}
	p.ApplyCoverage()
	before := append([]string(nil), p.Extensions...)
	excl := append([]string(nil), p.ExtensionsExclude...)
	p.ApplyCoverage()
	if !slices.Equal(before, p.Extensions) {
		t.Errorf("Extensions mutated on second call: before=%v after=%v", before, p.Extensions)
	}
	if !slices.Equal(excl, p.ExtensionsExclude) {
		t.Errorf("ExtensionsExclude mutated on second call: before=%v after=%v", excl, p.ExtensionsExclude)
	}
}
