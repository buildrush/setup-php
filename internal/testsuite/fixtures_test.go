package testsuite

import (
	"os"
	"path/filepath"
	"testing"
)

// TestLoad_RealFixturesFile parses the checked-in test/compat/fixtures.yaml
// (reached from this package's directory via ../../test/compat/fixtures.yaml)
// and asserts the file has at least the expected number of entries plus one
// known-shape entry. This guards against accidental schema drift between the
// Go structs here and the YAML the compat harness consumes.
func TestLoad_RealFixturesFile(t *testing.T) {
	path := filepath.Join("..", "..", "test", "compat", "fixtures.yaml")
	set, err := Load(path)
	if err != nil {
		t.Fatalf("Load(%q) returned error: %v", path, err)
	}
	if got := len(set.Fixtures); got < 40 {
		t.Fatalf("expected >= 40 fixtures in %s, got %d", path, got)
	}

	// Spot-check the "bare" fixture: first entry in the file, PHP 8.4,
	// empty extensions/ini-values, coverage=none, no arch/runner_os.
	var bare *Fixture
	for i := range set.Fixtures {
		if set.Fixtures[i].Name == "bare" {
			bare = &set.Fixtures[i]
			break
		}
	}
	if bare == nil {
		t.Fatalf("expected fixture named %q in %s", "bare", path)
	}
	if bare.PHPVersion != "8.4" {
		t.Errorf("bare.PHPVersion = %q, want %q", bare.PHPVersion, "8.4")
	}
	if bare.Extensions != "" {
		t.Errorf("bare.Extensions = %q, want empty", bare.Extensions)
	}
	if bare.INIValues != "" {
		t.Errorf("bare.INIValues = %q, want empty", bare.INIValues)
	}
	if bare.Coverage != "none" {
		t.Errorf("bare.Coverage = %q, want %q", bare.Coverage, "none")
	}
	if bare.Arch != "" {
		t.Errorf("bare.Arch = %q, want empty (implicit x86_64)", bare.Arch)
	}
	if bare.RunnerOS != "" {
		t.Errorf("bare.RunnerOS = %q, want empty (any OS)", bare.RunnerOS)
	}
}

func TestLoad_MissingFile_Errors(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "does-not-exist.yaml")
	if _, err := Load(missing); err == nil {
		t.Fatalf("Load(%q) returned nil error for missing file", missing)
	}
}

func TestLoad_MalformedYAML_Errors(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.yaml")
	// Not valid YAML: unterminated flow sequence.
	if err := os.WriteFile(path, []byte("fixtures: [ this is: not valid"), 0o600); err != nil {
		t.Fatalf("seed malformed YAML: %v", err)
	}
	if _, err := Load(path); err == nil {
		t.Fatalf("Load(%q) returned nil error for malformed YAML", path)
	}
}

func TestFilter_PHPVersionMatch(t *testing.T) {
	set := &FixtureSet{
		Fixtures: []Fixture{
			{Name: "a", PHPVersion: "8.4"},
			{Name: "b", PHPVersion: "8.5"},
			{Name: "c", PHPVersion: "8.4"},
		},
	}
	got := set.Filter("jammy", "x86_64", "8.4")
	if len(got) != 2 {
		t.Fatalf("expected 2 matches for PHP 8.4, got %d: %+v", len(got), got)
	}
	for _, f := range got {
		if f.PHPVersion != "8.4" {
			t.Errorf("Filter returned wrong version: %+v", f)
		}
		if f.Name != "a" && f.Name != "c" {
			t.Errorf("Filter returned wrong fixture: %+v", f)
		}
	}
}

func TestFilter_ArchMatch_DefaultX86(t *testing.T) {
	set := &FixtureSet{
		Fixtures: []Fixture{
			{Name: "bare", PHPVersion: "8.4"}, // empty Arch => x86_64
		},
	}
	if got := set.Filter("jammy", "x86_64", "8.4"); len(got) != 1 {
		t.Errorf("arch=x86_64 should match default-x86_64 fixture; got %d", len(got))
	}
	if got := set.Filter("jammy", "amd64", "8.4"); len(got) != 1 {
		t.Errorf("arch=amd64 should alias to x86_64 and match; got %d", len(got))
	}
	if got := set.Filter("jammy", "aarch64", "8.4"); len(got) != 0 {
		t.Errorf("arch=aarch64 should NOT match default-x86_64 fixture; got %d", len(got))
	}
}

func TestFilter_ArchMatch_ExplicitArm64(t *testing.T) {
	set := &FixtureSet{
		Fixtures: []Fixture{
			{Name: "bare-arm", PHPVersion: "8.4", Arch: "aarch64"},
		},
	}
	if got := set.Filter("jammy", "aarch64", "8.4"); len(got) != 1 {
		t.Errorf("arch=aarch64 should match explicit-aarch64 fixture; got %d", len(got))
	}
	if got := set.Filter("jammy", "arm64", "8.4"); len(got) != 1 {
		t.Errorf("arch=arm64 should alias to aarch64 and match; got %d", len(got))
	}
	if got := set.Filter("jammy", "x86_64", "8.4"); len(got) != 0 {
		t.Errorf("arch=x86_64 should NOT match explicit-aarch64 fixture; got %d", len(got))
	}
}

func TestFilter_RunnerOSUnset_MatchesAny(t *testing.T) {
	set := &FixtureSet{
		Fixtures: []Fixture{
			{Name: "bare", PHPVersion: "8.4"}, // empty RunnerOS => any OS
		},
	}
	if got := set.Filter("jammy", "x86_64", "8.4"); len(got) != 1 {
		t.Errorf("RunnerOS-unset fixture should match jammy; got %d", len(got))
	}
	if got := set.Filter("noble", "x86_64", "8.4"); len(got) != 1 {
		t.Errorf("RunnerOS-unset fixture should match noble; got %d", len(got))
	}
	if got := set.Filter("ubuntu-22.04", "x86_64", "8.4"); len(got) != 1 {
		t.Errorf("RunnerOS-unset fixture should match ubuntu-22.04; got %d", len(got))
	}
}

func TestFilter_RunnerOSExplicit_MatchesOnly(t *testing.T) {
	set := &FixtureSet{
		Fixtures: []Fixture{
			{Name: "bare-jammy", PHPVersion: "8.4", RunnerOS: "ubuntu-22.04"},
		},
	}
	if got := set.Filter("jammy", "x86_64", "8.4"); len(got) != 1 {
		t.Errorf("ubuntu-22.04 fixture should match os=jammy alias; got %d", len(got))
	}
	if got := set.Filter("ubuntu-22.04", "x86_64", "8.4"); len(got) != 1 {
		t.Errorf("ubuntu-22.04 fixture should match os=ubuntu-22.04; got %d", len(got))
	}
	if got := set.Filter("noble", "x86_64", "8.4"); len(got) != 0 {
		t.Errorf("ubuntu-22.04 fixture should NOT match os=noble; got %d", len(got))
	}
	if got := set.Filter("ubuntu-24.04", "x86_64", "8.4"); len(got) != 0 {
		t.Errorf("ubuntu-22.04 fixture should NOT match os=ubuntu-24.04; got %d", len(got))
	}
}

func TestFilter_EmptyResult_NoMatch(t *testing.T) {
	set := &FixtureSet{
		Fixtures: []Fixture{
			{Name: "a", PHPVersion: "8.4"},
		},
	}
	got := set.Filter("jammy", "x86_64", "7.4")
	if got == nil {
		t.Fatalf("Filter returned nil for no-match; want empty slice")
	}
	if len(got) != 0 {
		t.Fatalf("Filter returned %d fixtures for no-match PHP version; want 0", len(got))
	}
}
