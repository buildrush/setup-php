package planner

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/buildrush/setup-php/internal/catalog"
)

func TestExpandPHPMatrix(t *testing.T) {
	spec := &catalog.PHPSpec{
		Versions: map[string]*catalog.PHPVersionSpec{
			"8.4": {
				BundledExtensions: []string{"mbstring"},
				Sources:           &catalog.PHPSource{URL: "https://example/php-{version}.tar.xz"},
				ABIMatrix: catalog.ABIMatrix{
					OS:   []string{"linux"},
					Arch: []string{"x86_64"},
					TS:   []string{"nts"},
				},
			},
		},
	}

	cells := ExpandPHPMatrix(spec)
	if len(cells) != 1 {
		t.Fatalf("len(cells) = %d, want 1", len(cells))
	}
	if cells[0].Version != "8.4" {
		t.Errorf("Version = %q, want 8.4", cells[0].Version)
	}
	if cells[0].OS != "linux" || cells[0].Arch != "x86_64" || cells[0].TS != "nts" {
		t.Errorf("cell = %+v", cells[0])
	}
}

func TestExpandPHPMatrixMulti(t *testing.T) {
	spec := &catalog.PHPSpec{
		Versions: map[string]*catalog.PHPVersionSpec{
			"8.3": {
				BundledExtensions: []string{"mbstring"},
				Sources:           &catalog.PHPSource{URL: "https://example/php-{version}.tar.xz"},
				ABIMatrix: catalog.ABIMatrix{
					OS:   []string{"linux"},
					Arch: []string{"x86_64", "aarch64"},
					TS:   []string{"nts"},
				},
			},
			"8.4": {
				BundledExtensions: []string{"mbstring"},
				Sources:           &catalog.PHPSource{URL: "https://example/php-{version}.tar.xz"},
				ABIMatrix: catalog.ABIMatrix{
					OS:   []string{"linux"},
					Arch: []string{"x86_64", "aarch64"},
					TS:   []string{"nts"},
				},
			},
		},
	}
	// 2 versions × 1 OS × 2 arch × 1 TS = 4 cells
	cells := ExpandPHPMatrix(spec)
	if len(cells) != 4 {
		t.Fatalf("len(cells) = %d, want 4", len(cells))
	}
}

func TestExpandPHPMatrixSkipsCompatOnlyVersions(t *testing.T) {
	spec := &catalog.PHPSpec{
		Versions: map[string]*catalog.PHPVersionSpec{
			"8.1": {BundledExtensions: []string{"mbstring"}}, // no sources → compat only
			"8.4": {
				BundledExtensions: []string{"mbstring"},
				Sources:           &catalog.PHPSource{URL: "https://example/php-{version}.tar.xz"},
				ABIMatrix: catalog.ABIMatrix{
					OS:   []string{"linux"},
					Arch: []string{"x86_64"},
					TS:   []string{"nts"},
				},
			},
		},
	}

	cells := ExpandPHPMatrix(spec)
	if len(cells) != 1 {
		t.Fatalf("len(cells) = %d, want 1 (compat-only versions must be skipped)", len(cells))
	}
	if cells[0].Version != "8.4" {
		t.Errorf("expected only 8.4 cell, got %+v", cells[0])
	}
}

func TestExpandExtMatrixWithExclude(t *testing.T) {
	spec := &catalog.ExtensionSpec{
		Name:     "redis",
		Kind:     catalog.ExtensionKindPECL,
		Versions: []string{"6.2.0"},
		ABIMatrix: catalog.ABIMatrix{
			PHP:  []string{"8.4"},
			OS:   []string{"linux", "windows"},
			Arch: []string{"x86_64", "aarch64"},
			TS:   []string{"nts"},
		},
		Exclude: []catalog.ExcludeRule{
			{OS: "windows", Arch: "aarch64"},
		},
	}

	// Without exclude: 1×2×2×1 = 4 per version = 4
	// With exclude: -1 (windows+aarch64) = 3
	cells := ExpandExtMatrix(spec)
	if len(cells) != 3 {
		t.Fatalf("len(cells) = %d, want 3", len(cells))
	}

	for _, c := range cells {
		if c.OS == "windows" && c.Arch == "aarch64" {
			t.Error("excluded combination (windows, aarch64) should not appear")
		}
	}
}

func TestComputeSpecHash(t *testing.T) {
	cell := MatrixCell{Version: "8.4", OS: "linux", Arch: "x86_64", TS: "nts"}
	h1 := ComputeSpecHash(&cell, []byte("catalog data"), "builder-hash-1")
	h2 := ComputeSpecHash(&cell, []byte("catalog data"), "builder-hash-1")
	if h1 != h2 {
		t.Error("ComputeSpecHash should be deterministic")
	}
	h3 := ComputeSpecHash(&cell, []byte("different catalog"), "builder-hash-1")
	if h1 == h3 {
		t.Error("ComputeSpecHash should differ for different inputs")
	}
}

func TestWriteMatrices(t *testing.T) {
	dir := t.TempDir()
	result := &Result{
		PHP: Matrix{Include: []MatrixCell{
			{Version: "8.4", OS: "linux", Arch: "x86_64", TS: "nts", Digest: "sha256:abc"},
		}},
		Ext:  Matrix{Include: []MatrixCell{}},
		Tool: Matrix{Include: []MatrixCell{}},
	}

	err := WriteMatrices(result, dir)
	if err != nil {
		t.Fatalf("WriteMatrices() error = %v", err)
	}

	data, _ := os.ReadFile(filepath.Join(dir, "php.json"))
	var m Matrix
	json.Unmarshal(data, &m)
	if len(m.Include) != 1 {
		t.Errorf("php.json should have 1 entry, got %d", len(m.Include))
	}

	// Empty matrix should produce valid JSON
	extData, _ := os.ReadFile(filepath.Join(dir, "ext.json"))
	json.Unmarshal(extData, &m)
	if m.Include == nil {
		t.Error("empty matrix should have non-nil Include")
	}
}

func TestHashFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "data.txt")
	if err := os.WriteFile(path, []byte("hello"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	h, err := HashFile(path)
	if err != nil {
		t.Fatalf("HashFile: %v", err)
	}
	// sha256("hello") = 2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824
	want := "sha256:2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824"
	if h != want {
		t.Errorf("HashFile = %q, want %q", h, want)
	}

	// Missing file is a hard error.
	if _, err := HashFile(filepath.Join(dir, "nope")); err == nil {
		t.Error("HashFile should error on missing file")
	}
}

func TestPerVersionYAMLDeterminism(t *testing.T) {
	spec := &catalog.PHPSpec{
		Name: "php",
		Versions: map[string]*catalog.PHPVersionSpec{
			"8.4": {
				BundledExtensions: []string{"curl", "opcache"},
				Sources:           &catalog.PHPSource{URL: "u", Sig: "s"},
				ABIMatrix:         catalog.ABIMatrix{OS: []string{"linux"}, Arch: []string{"x86_64"}, TS: []string{"nts"}},
			},
			"8.5": {BundledExtensions: []string{"curl", "opcache", "uri"}},
		},
	}
	y1, err := PerVersionYAML(spec, "8.4")
	if err != nil {
		t.Fatalf("PerVersionYAML: %v", err)
	}
	y2, _ := PerVersionYAML(spec, "8.4")
	if !bytes.Equal(y1, y2) {
		t.Errorf("non-deterministic:\n%s\n---\n%s", y1, y2)
	}
	// 8.5 must produce different bytes from 8.4.
	y3, _ := PerVersionYAML(spec, "8.5")
	if bytes.Equal(y1, y3) {
		t.Errorf("8.4 and 8.5 produced identical bytes; hash would not differ")
	}
	// Unknown version is an error.
	if _, err := PerVersionYAML(spec, "7.0"); err == nil {
		t.Error("unknown version should error")
	}
}

func TestExtensionYAMLDeterminism(t *testing.T) {
	spec := &catalog.ExtensionSpec{
		Name:     "redis",
		Kind:     catalog.ExtensionKindPECL,
		Versions: []string{"6.2.0"},
	}
	y1, err := ExtensionYAML(spec)
	if err != nil {
		t.Fatalf("ExtensionYAML: %v", err)
	}
	y2, _ := ExtensionYAML(spec)
	if !bytes.Equal(y1, y2) {
		t.Error("non-deterministic")
	}
	if !strings.Contains(string(y1), "redis") {
		t.Errorf("expected redis in output; got:\n%s", y1)
	}
}

func TestComputeSpecHashDeltas(t *testing.T) {
	cell := MatrixCell{Version: "8.4", OS: "linux", Arch: "x86_64", TS: "nts"}
	base := ComputeSpecHash(&cell, []byte("catalog"), "builder-a")

	if got := ComputeSpecHash(&cell, []byte("catalog"), "builder-a"); got != base {
		t.Error("same inputs must produce same hash")
	}
	if got := ComputeSpecHash(&cell, []byte("catalog-v2"), "builder-a"); got == base {
		t.Error("changing catalog must change hash")
	}
	if got := ComputeSpecHash(&cell, []byte("catalog"), "builder-b"); got == base {
		t.Error("changing builder must change hash")
	}
	cell2 := cell
	cell2.Version = "8.5"
	if got := ComputeSpecHash(&cell2, []byte("catalog"), "builder-a"); got == base {
		t.Error("changing cell must change hash")
	}
}
