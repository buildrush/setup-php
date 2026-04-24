package testsuite

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/buildrush/setup-php/internal/compatdiff"
)

// TestAppendDeviations_CreatesFileOnFirstCall asserts that writing to a
// non-existent path creates the file with a well-formed JSON document
// containing the single fixture's entry.
func TestAppendDeviations_CreatesFileOnFirstCall(t *testing.T) {
	path := filepath.Join(t.TempDir(), "deviations.json")
	devs := []compatdiff.Deviation{{Path: "ini.memory_limit", Ours: "256M", Theirs: "-1"}}
	if err := AppendDeviations(path, "noble-x86_64-8.4", "bare", devs); err != nil {
		t.Fatalf("AppendDeviations: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	var parsed DeviationsFile
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("Unmarshal: %v\nraw: %s", err, data)
	}
	if parsed.Cell != "noble-x86_64-8.4" {
		t.Errorf("Cell: got %q, want %q", parsed.Cell, "noble-x86_64-8.4")
	}
	if len(parsed.Fixtures) != 1 || parsed.Fixtures[0].Name != "bare" {
		t.Errorf("Fixtures: got %+v, want 1 entry for 'bare'", parsed.Fixtures)
	}
	if len(parsed.Fixtures[0].Deviations) != 1 || parsed.Fixtures[0].Deviations[0].Path != "ini.memory_limit" {
		t.Errorf("Deviations: got %+v, want 1 entry for ini.memory_limit", parsed.Fixtures[0].Deviations)
	}
}

// TestAppendDeviations_AppendsToExistingFile asserts that a second call
// to AppendDeviations merges into the same JSON file without clobbering
// the first entry.
func TestAppendDeviations_AppendsToExistingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "deviations.json")
	if err := AppendDeviations(path, "noble-x86_64-8.4", "bare",
		[]compatdiff.Deviation{{Path: "ini.memory_limit", Ours: "256M", Theirs: "-1"}}); err != nil {
		t.Fatalf("first AppendDeviations: %v", err)
	}
	if err := AppendDeviations(path, "noble-x86_64-8.4", "multi-ext",
		[]compatdiff.Deviation{{Path: "extensions", Ours: "[a,b]", Theirs: "[a]"}}); err != nil {
		t.Fatalf("second AppendDeviations: %v", err)
	}
	data, _ := os.ReadFile(path)
	var parsed DeviationsFile
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("Unmarshal: %v\nraw: %s", err, data)
	}
	if len(parsed.Fixtures) != 2 {
		t.Fatalf("Fixtures: got %d, want 2", len(parsed.Fixtures))
	}
	names := map[string]bool{parsed.Fixtures[0].Name: true, parsed.Fixtures[1].Name: true}
	if !names["bare"] || !names["multi-ext"] {
		t.Errorf("expected both 'bare' and 'multi-ext' in %+v", parsed.Fixtures)
	}
}

// TestAppendDeviations_RejectsCellMismatch protects against a caller
// mixing two different cells' outputs into the same artifact file —
// that would break the per-cell artifact naming scheme in CI.
func TestAppendDeviations_RejectsCellMismatch(t *testing.T) {
	path := filepath.Join(t.TempDir(), "deviations.json")
	_ = AppendDeviations(path, "noble-x86_64-8.4", "bare",
		[]compatdiff.Deviation{{Path: "p", Ours: "a", Theirs: "b"}})
	err := AppendDeviations(path, "noble-aarch64-8.4", "multi-ext",
		[]compatdiff.Deviation{{Path: "p", Ours: "a", Theirs: "b"}})
	if err == nil {
		t.Fatalf("AppendDeviations: expected cell-mismatch error, got nil")
	}
}

// TestAppendDeviations_RejectsEmptyDeviations pins the contract that the
// artifact records failing fixtures only. A caller that passes an empty
// (or nil) deviations slice has a bug — reject loudly rather than
// silently writing `"deviations": null`.
func TestAppendDeviations_RejectsEmptyDeviations(t *testing.T) {
	path := filepath.Join(t.TempDir(), "deviations.json")
	err := AppendDeviations(path, "noble-x86_64-8.4", "bare", nil)
	if err == nil {
		t.Errorf("AppendDeviations(nil devs): want error, got nil")
	}
	err = AppendDeviations(path, "noble-x86_64-8.4", "bare", []compatdiff.Deviation{})
	if err == nil {
		t.Errorf("AppendDeviations(empty devs): want error, got nil")
	}
}
