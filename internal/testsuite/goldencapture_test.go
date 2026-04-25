package testsuite

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// TestRunGoldenCapture_WritesAllEligibleFixtures verifies the capture
// command reads test/compat/fixtures.yaml, filters to entries with
// Compat: true, invokes probe.sh (stubbed), and writes one JSON per
// fixture to --out-dir.
func TestRunGoldenCapture_WritesAllEligibleFixtures(t *testing.T) {
	workDir := t.TempDir()

	// Minimal fixtures file: 2 compat fixtures + 1 non-compat (skipped).
	fixturesPath := filepath.Join(workDir, "fixtures.yaml")
	if err := os.WriteFile(fixturesPath, []byte(`fixtures:
  - name: bare
    php-version: "8.4"
    extensions: ""
    ini-values: ""
    coverage: "none"
    compat: true
  - name: multi-ext
    php-version: "8.4"
    extensions: "redis, xdebug"
    ini-values: ""
    coverage: "none"
    compat: true
  - name: other
    php-version: "8.4"
    extensions: ""
    ini-values: ""
    coverage: "none"
`), 0o644); err != nil {
		t.Fatal(err)
	}

	// Stub probe.sh: emits an always-ok JSON (probe contents are irrelevant
	// to this test; we're verifying orchestration, not probe fidelity).
	probePath := filepath.Join(workDir, "probe.sh")
	if err := os.WriteFile(probePath, []byte(`#!/usr/bin/env bash
set -euo pipefail
out="$1"
cat > "$out" <<'JSON'
{"php_version":"8.4.0","sapi":"cli","zts":false,"extensions":[],"ini":{},"env_delta":[],"path_additions":[]}
JSON
`), 0o755); err != nil {
		t.Fatal(err)
	}

	outDir := filepath.Join(workDir, "golden")
	err := RunGoldenCapture(GoldenCaptureOpts{
		FixturesPath: fixturesPath,
		ProbePath:    probePath,
		OutDir:       outDir,
	})
	if err != nil {
		t.Fatalf("RunGoldenCapture: %v", err)
	}

	// Check presence of the two compat goldens and absence of the third.
	for _, name := range []string{"bare", "multi-ext"} {
		path := filepath.Join(outDir, name+".json")
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("expected %s; got err: %v", path, err)
		}
		var doc map[string]any
		if err := json.Unmarshal(data, &doc); err != nil {
			t.Errorf("%s is not valid JSON: %v", path, err)
		}
	}
	if _, err := os.Stat(filepath.Join(outDir, "other.json")); !os.IsNotExist(err) {
		t.Errorf("non-compat fixture 'other' should not have a golden; err=%v", err)
	}
}

// TestRunGoldenCapture_NoCompatFixturesIsNoOp pins the contract that
// a fixtures file containing zero Compat: true entries returns nil
// (no work, no error). Guards against a future refactor that would
// error on "no eligible fixtures found" — the refresh workflow must
// tolerate the empty case.
func TestRunGoldenCapture_NoCompatFixturesIsNoOp(t *testing.T) {
	workDir := t.TempDir()

	fixturesPath := filepath.Join(workDir, "fixtures.yaml")
	if err := os.WriteFile(fixturesPath, []byte(`fixtures:
  - name: a
    php-version: "8.4"
    extensions: ""
    ini-values: ""
    coverage: "none"
  - name: b
    php-version: "8.4"
    extensions: ""
    ini-values: ""
    coverage: "none"
`), 0o644); err != nil {
		t.Fatal(err)
	}

	probePath := filepath.Join(workDir, "probe.sh")
	if err := os.WriteFile(probePath, []byte(`#!/usr/bin/env bash
exit 0
`), 0o755); err != nil {
		t.Fatal(err)
	}

	outDir := filepath.Join(workDir, "golden")
	err := RunGoldenCapture(GoldenCaptureOpts{
		FixturesPath: fixturesPath,
		ProbePath:    probePath,
		OutDir:       outDir,
	})
	if err != nil {
		t.Errorf("RunGoldenCapture on all-non-compat fixtures: want nil, got %v", err)
	}
	entries, err := os.ReadDir(outDir)
	if err != nil {
		t.Fatalf("ReadDir(%s): %v", outDir, err)
	}
	if len(entries) != 0 {
		t.Errorf("out-dir should be empty when no fixtures are eligible; got %d entries", len(entries))
	}
}
