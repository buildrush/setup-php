package testsuite

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// GoldenCaptureOpts is the parsed flag set for `phpup internal
// golden-capture`. Callers may construct one directly (tests) or via
// parseGoldenCaptureFlags (CLI).
type GoldenCaptureOpts struct {
	FixturesPath string
	ProbePath    string
	OutDir       string
}

// RunGoldenCapture iterates every Compat: true fixture in the loaded
// fixtures file, runs probe.sh (assuming v2 has already been set up
// by the outer workflow), and writes the probe output JSON to
// <OutDir>/<fixture-name>.json. Caller is responsible for sequencing
// the v2 setup step before invoking this; this function does not
// install or configure PHP.
//
// Running this on a developer laptop without first installing v2
// captures that laptop's PHP — the resulting JSON is not a valid v2
// golden. See docs/superpowers/specs/2026-04-24-pr-gated-v2-compat-design.md
// and the Makefile compat-refresh-goldens target for the canonical
// invocation path (weekly refresh workflow + pinned v2 install).
func RunGoldenCapture(opts GoldenCaptureOpts) error {
	set, err := Load(opts.FixturesPath)
	if err != nil {
		return fmt.Errorf("golden-capture: %w", err)
	}
	if err := os.MkdirAll(opts.OutDir, 0o750); err != nil {
		return fmt.Errorf("golden-capture: mkdir out-dir: %w", err)
	}
	var failed []string
	for i := range set.Fixtures {
		f := &set.Fixtures[i]
		if !f.Compat {
			continue
		}
		outPath := filepath.Join(opts.OutDir, f.Name+".json")
		if err := captureOne(opts.ProbePath, f, outPath); err != nil {
			fmt.Fprintf(os.Stderr, "golden-capture: fixture=%s FAIL: %v\n", f.Name, err)
			failed = append(failed, f.Name)
			continue
		}
		fmt.Printf("golden-capture: fixture=%s wrote %s\n", f.Name, outPath)
	}
	if len(failed) > 0 {
		return fmt.Errorf("golden-capture: %d fixture(s) failed: %v", len(failed), failed)
	}
	return nil
}

// captureOne is the per-fixture probe invocation. It writes tmpfiles
// for probe.sh's env-before / path-before / ini-keys inputs, then
// execs bash probe.sh with the given outPath.
func captureOne(probePath string, f *Fixture, outPath string) (err error) {
	dir, mktempErr := os.MkdirTemp("", "phpup-golden-capture-"+f.Name+"-*")
	if mktempErr != nil {
		return fmt.Errorf("mktemp: %w", mktempErr)
	}
	// Clean up the per-fixture tempdir on success; leave it behind on
	// error so a maintainer can inspect env-before / path-before /
	// ini-keys.txt when debugging a failed probe.
	defer func() {
		if err == nil {
			_ = os.RemoveAll(dir)
		}
	}()
	envBefore := filepath.Join(dir, "env-before")
	pathBefore := filepath.Join(dir, "path-before")
	iniKeys := filepath.Join(dir, "ini-keys.txt")
	if err := writeEnvSnapshot(envBefore); err != nil {
		return fmt.Errorf("env-before: %w", err)
	}
	if err := os.WriteFile(pathBefore, []byte(os.Getenv("PATH")), 0o600); err != nil {
		return fmt.Errorf("path-before: %w", err)
	}
	if err := writeIniKeysFromFixture(iniKeys, f); err != nil {
		return fmt.Errorf("ini-keys: %w", err)
	}
	//nolint:gosec // G204: probePath is --probe flag value (trusted operator input);
	// tmp-file args are filepath.Join-derived. No user CLI flows into argv.
	cmd := exec.Command("bash", probePath, outPath, envBefore, pathBefore, iniKeys)
	out, runErr := cmd.CombinedOutput()
	if runErr != nil {
		return fmt.Errorf("probe.sh: %w (is PHP on PATH? this subcommand assumes the caller has installed v2 first, e.g. `shivammathur/setup-php@<pinned-sha>` in the refresh workflow); output tail:\n%s", runErr, tailBytes(out, 20))
	}
	return nil
}

// parseGoldenCaptureFlags parses the flag tail for
// `phpup internal golden-capture`. All flags have sensible defaults
// for invocation from the CI refresh workflow (where the working
// directory is the repo root).
func parseGoldenCaptureFlags(args []string) (*GoldenCaptureOpts, error) {
	fs := flag.NewFlagSet("phpup internal golden-capture", flag.ContinueOnError)
	fixtures := fs.String("fixtures", "test/compat/fixtures.yaml", "Path to fixtures YAML")
	probe := fs.String("probe", "test/compat/probe.sh", "Path to probe.sh")
	outDir := fs.String("out-dir", "test/compat/testdata/golden/v2", "Where to write <fixture>.json")
	if err := fs.Parse(args); err != nil {
		return nil, err
	}
	if *fixtures == "" || *probe == "" || *outDir == "" {
		return nil, errors.New("golden-capture: --fixtures, --probe, --out-dir are required")
	}
	return &GoldenCaptureOpts{
		FixturesPath: *fixtures,
		ProbePath:    *probe,
		OutDir:       *outDir,
	}, nil
}

// MainGoldenCapture is the entry point invoked by `phpup internal
// golden-capture`. It parses flags, then calls RunGoldenCapture.
func MainGoldenCapture(args []string) error {
	opts, err := parseGoldenCaptureFlags(args)
	if err != nil {
		return err
	}
	return RunGoldenCapture(*opts)
}
