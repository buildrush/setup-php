package compatdiff

import (
	"path/filepath"
	"testing"
)

func TestLoadAllowlist(t *testing.T) {
	tests := []struct {
		name    string
		file    string
		wantErr bool
		wantLen int
	}{
		{"empty", "compat-matrix-empty.md", false, 0},
		{"entries", "compat-matrix-entries.md", false, 2},
		{"no markers", "compat-matrix-no-markers.md", true, 0},
		{"malformed yaml", "compat-matrix-malformed.md", true, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			a, err := loadAllowlist(filepath.Join("testdata", tc.file))
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got allowlist with %d entries", len(a.Deviations))
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(a.Deviations) != tc.wantLen {
				t.Errorf("len(Deviations) = %d, want %d", len(a.Deviations), tc.wantLen)
			}
		})
	}
}

func TestLoadAllowlistEntryFields(t *testing.T) {
	a, err := loadAllowlist(filepath.Join("testdata", "compat-matrix-entries.md"))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(a.Deviations) != 2 {
		t.Fatalf("got %d entries, want 2", len(a.Deviations))
	}
	d := a.Deviations[0]
	if d.Path != "ini.opcache.jit_buffer_size" {
		t.Errorf("path = %q, want ini.opcache.jit_buffer_size", d.Path)
	}
	if d.Kind != "ignore" {
		t.Errorf("kind = %q, want ignore", d.Kind)
	}
	if len(d.Fixtures) != 1 || d.Fixtures[0] != "*" {
		t.Errorf("fixtures = %v, want [*]", d.Fixtures)
	}
}
