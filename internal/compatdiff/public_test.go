package compatdiff

import (
	"path/filepath"
	"testing"
)

// TestDiffFiles_NoDeviations verifies DiffFiles returns an empty slice
// and nil error when ours == theirs byte-for-byte and the allowlist
// contains no matching rules.
func TestDiffFiles_NoDeviations(t *testing.T) {
	bare := filepath.Join("testdata", "probe-bare.json")
	empty := filepath.Join("testdata", "compat-matrix-empty.md")
	devs, err := DiffFiles(bare, bare, empty, "bare")
	if err != nil {
		t.Fatalf("DiffFiles: %v", err)
	}
	if len(devs) != 0 {
		t.Fatalf("DiffFiles: want 0 deviations, got %d: %+v", len(devs), devs)
	}
}

// TestDiffFiles_ReportsDeviations checks that divergent probes produce
// a non-empty []Deviation whose fields match the internal diffEntry shape.
func TestDiffFiles_ReportsDeviations(t *testing.T) {
	ours := filepath.Join("testdata", "probe-bare.json")
	theirs := filepath.Join("testdata", "probe-bare-ini-shift.json")
	empty := filepath.Join("testdata", "compat-matrix-empty.md")
	devs, err := DiffFiles(ours, theirs, empty, "bare")
	if err != nil {
		t.Fatalf("DiffFiles: %v", err)
	}
	if len(devs) == 0 {
		t.Fatalf("DiffFiles: want at least 1 deviation, got 0")
	}
	for _, d := range devs {
		if d.Path == "" {
			t.Errorf("Deviation.Path empty: %+v", d)
		}
	}
}

// TestDiffFiles_InvalidProbeSurfacesError confirms a broken probe path
// (missing file) returns a non-nil error (not a silent empty diff).
func TestDiffFiles_InvalidProbeSurfacesError(t *testing.T) {
	empty := filepath.Join("testdata", "compat-matrix-empty.md")
	_, err := DiffFiles("/nonexistent/ours.json", "/nonexistent/theirs.json", empty, "bare")
	if err == nil {
		t.Fatalf("DiffFiles: want error for missing probe files, got nil")
	}
}
