package testsuite

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/buildrush/setup-php/internal/compatdiff"
)

// DeviationsFile is the on-disk shape of the per-cell deviation artifact
// uploaded by ci.yml::pipeline when a fixture's compat-diff fails.
// One file per cell (noble-x86_64-8.4 is the only compat cell today),
// aggregating all failing fixtures from that cell.
type DeviationsFile struct {
	Cell     string              `json:"cell"`
	Fixtures []FixtureDeviations `json:"fixtures"`
}

// FixtureDeviations collects the non-empty deviations from one fixture's
// compat-diff invocation.
type FixtureDeviations struct {
	Name       string                 `json:"name"`
	Deviations []compatdiff.Deviation `json:"deviations"`
}

// AppendDeviations merges (fixture, deviations) into the JSON file at
// path. If the file doesn't exist, it is created with cell as the top-
// level Cell field. If it exists with a different Cell, an error is
// returned — the writer refuses to cross-contaminate per-cell artifacts.
//
// Callers pass the cell tag in the form "noble-x86_64-8.4" (os-arch-php)
// so the CI `compat-report` job can render a clear per-cell heading.
//
// Not safe for concurrent callers on the same path: the helper performs
// read-modify-write without locking and expects the serial fixture loop
// in testcell.go (runFixture is called sequentially from RunTestCell) to
// be the only caller. If fixture execution is ever parallelized, add a
// per-path mutex or switch to an append-only output shape.
func AppendDeviations(path, cell, fixture string, devs []compatdiff.Deviation) error {
	if len(devs) == 0 {
		return fmt.Errorf("AppendDeviations: refuse to record fixture %q with no deviations (artifact is for failing fixtures only)", fixture)
	}
	var doc DeviationsFile
	data, err := os.ReadFile(filepath.Clean(path))
	switch {
	case err == nil:
		if err := json.Unmarshal(data, &doc); err != nil {
			return fmt.Errorf("AppendDeviations: parse existing %s: %w", path, err)
		}
		if doc.Cell != "" && doc.Cell != cell {
			return fmt.Errorf("AppendDeviations: cell mismatch: file=%q call=%q", doc.Cell, cell)
		}
	case errors.Is(err, os.ErrNotExist):
		// Fresh file — fall through.
	default:
		return fmt.Errorf("AppendDeviations: read existing %s: %w", path, err)
	}
	if doc.Cell == "" {
		doc.Cell = cell
	}
	doc.Fixtures = append(doc.Fixtures, FixtureDeviations{Name: fixture, Deviations: devs})
	out, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return fmt.Errorf("AppendDeviations: marshal: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return fmt.Errorf("AppendDeviations: mkdir: %w", err)
	}
	if err := os.WriteFile(path, out, 0o644); err != nil { //nolint:gosec // G306: artifact is non-sensitive CI output uploaded by upload-artifact; 0644 matches prior art (writeLayoutLockfileOverride).
		return fmt.Errorf("AppendDeviations: write: %w", err)
	}
	return nil
}
