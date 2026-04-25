package testsuite

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/buildrush/setup-php/internal/compatdiff"
)

// InstallFunc runs `phpup install` for a single fixture with the given env
// overlay. The default is realInstall which execs os.Args[0] as a child
// process with the env composed into os.Environ(). Tests override via
// SetInstaller.
type InstallFunc func(ctx context.Context, env map[string]string, stdout, stderr io.Writer) error

// installerMu protects currInstaller during SetInstaller swaps. Mirrors the
// pattern in internal/build.SetRunner: tests using SetInstaller MUST NOT run
// in parallel since the installer is a package-level global.
var (
	installerMu   sync.Mutex
	currInstaller InstallFunc = realInstall
)

// SetInstaller swaps the package-level InstallFunc and returns a restore
// function that callers MUST defer to revert. Same pattern as
// internal/build.SetRunner — tests substitute a fake to exercise fixture
// orchestration without invoking the real install path.
func SetInstaller(fn InstallFunc) (restore func()) {
	installerMu.Lock()
	prev := currInstaller
	currInstaller = fn
	installerMu.Unlock()
	return func() {
		installerMu.Lock()
		currInstaller = prev
		installerMu.Unlock()
	}
}

// getInstaller returns the currently-installed InstallFunc under the mutex.
func getInstaller() InstallFunc {
	installerMu.Lock()
	defer installerMu.Unlock()
	return currInstaller
}

// realInstall spawns a child process invoking the current binary's
// default subcommand (phpup install). The child inherits os.Environ()
// plus the env overlay passed in.
func realInstall(ctx context.Context, env map[string]string, stdout, stderr io.Writer) error {
	self, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve self: %w", err)
	}
	//nolint:gosec // G204 false positive: self is os.Executable() — the path of the
	// currently-running phpup binary — and argv is empty (no user-controlled args),
	// so `phpup install` is dispatched as the default subcommand.
	cmd := exec.CommandContext(ctx, self)
	cmd.Env = composeEnv(os.Environ(), env)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	return cmd.Run()
}

// composeEnv merges overlay into base, with overlay winning on key conflicts.
// Output is deterministic: keys from base retain their original relative
// order; overlay keys are appended alphabetically. Tests rely on the
// alphabetical overlay ordering.
func composeEnv(base []string, overlay map[string]string) []string {
	keys := map[string]struct{}{}
	for k := range overlay {
		keys[k] = struct{}{}
	}
	out := make([]string, 0, len(base)+len(overlay))
	for _, kv := range base {
		eq := strings.IndexByte(kv, '=')
		if eq < 0 {
			out = append(out, kv)
			continue
		}
		k := kv[:eq]
		if _, ok := keys[k]; !ok {
			out = append(out, kv)
		}
	}
	names := make([]string, 0, len(overlay))
	for k := range overlay {
		names = append(names, k)
	}
	sort.Strings(names)
	for _, k := range names {
		out = append(out, k+"="+overlay[k])
	}
	return out
}

// composeProbeEnv parses GITHUB_ENV (KEY=value per line) and GITHUB_PATH
// (one entry per line) written by phpup install during its GHA-runner
// emulation, then applies them on top of os.Environ() to produce the env
// for the probe.sh invocation. This mirrors what GitHub Actions does
// between steps: post-step processing reads these files and rolls them
// into the next step's env.
//
// Without this, phpup install succeeds but the path additions it emits
// (notably, the directory containing the freshly-installed php binary)
// stay trapped in the two files and probe.sh fails with
// "php: command not found".
// parseGitHubEnv reads a GITHUB_ENV file written by the GHA-runner-style
// writer in internal/env (which emits heredoc for every SetEnv, including
// scalars: `KEY<<DELIM\nVALUE\nDELIM\n`). Also tolerates the simpler
// `KEY=value` form GHA's own docs describe. Unknown lines are ignored.
func parseGitHubEnv(envFile string) map[string]string {
	overlay := map[string]string{}
	data, err := os.ReadFile(filepath.Clean(envFile))
	if err != nil {
		return overlay
	}
	lines := strings.Split(string(data), "\n")
	i := 0
	for i < len(lines) {
		line := lines[i]
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			i++
			continue
		}
		// Heredoc: KEY<<DELIM
		if idx := strings.Index(line, "<<"); idx > 0 && !strings.ContainsRune(line[:idx], '=') {
			key := strings.TrimSpace(line[:idx])
			delim := strings.TrimSpace(line[idx+2:])
			i++
			var value []string
			for i < len(lines) && lines[i] != delim {
				value = append(value, lines[i])
				i++
			}
			if i < len(lines) {
				i++ // consume closing delim
			}
			overlay[key] = strings.Join(value, "\n")
			continue
		}
		// Simple KEY=VALUE line.
		if eq := strings.IndexByte(line, '='); eq > 0 {
			overlay[strings.TrimSpace(line[:eq])] = line[eq+1:]
		}
		i++
	}
	return overlay
}

func composeProbeEnv(envFile, pathFile string) ([]string, error) {
	base := os.Environ()

	// Parse GITHUB_ENV. Supports both KEY=value single-line form AND the
	// heredoc form GHA uses: `KEY<<DELIM\nVALUE\nDELIM`. phpup install's
	// internal/env exporter emits heredoc even for simple scalar values
	// so we must handle both.
	overlay := parseGitHubEnv(envFile)

	// Parse GITHUB_PATH: one entry per line; prepend to PATH.
	var prefix []string
	if data, err := os.ReadFile(filepath.Clean(pathFile)); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			line = strings.TrimSpace(line)
			if line != "" {
				prefix = append(prefix, line)
			}
		}
	}
	if len(prefix) > 0 {
		// Prepend to the current PATH, preserving existing overlay value if set.
		currentPath := overlay["PATH"]
		if currentPath == "" {
			currentPath = os.Getenv("PATH")
		}
		overlay["PATH"] = strings.Join(append(prefix, currentPath), ":")
	}

	return composeEnv(base, overlay), nil
}

// testCellOpts is the parsed flag set for `phpup internal test-cell`.
// The Compat* paths are container-side absolute paths populated by the
// outer runner (runCell in runner.go) when the compat gate should be
// active. Non-canonical cells and the standalone `phpup install` path
// leave them empty — runFixture's shouldRunCompat gate short-circuits
// on any empty value, so the compat-diff step becomes a no-op without
// requiring the CLI user to know about it.
type testCellOpts struct {
	OS, Arch, PHP  string
	FixturesPath   string
	ProbePath      string
	RegistryURI    string
	CompatMatrix   string // path to compat-matrix.md; "" = compat off
	GoldenDir      string // directory containing <fixture>.json goldens; "" = compat off
	DeviationsPath string // path to append deviation artifacts to; "" = compat off
}

// parseTestCellFlags parses the flag tail for `phpup internal test-cell`.
// Uses ContinueOnError so callers get an error back instead of a process
// exit — keeps the surface testable without os.Exit acrobatics.
func parseTestCellFlags(args []string) (*testCellOpts, error) {
	fs := flag.NewFlagSet("phpup internal test-cell", flag.ContinueOnError)
	osFlag := fs.String("os", "", "Ubuntu flavour (jammy|noble); REQUIRED")
	archFlag := fs.String("arch", "", "Target arch (x86_64|aarch64); REQUIRED")
	phpFlag := fs.String("php", "", "PHP minor version (e.g. 8.4); REQUIRED")
	fixturesFlag := fs.String("fixtures", "/test-compat/fixtures.yaml", "Path to fixtures YAML")
	probeFlag := fs.String("probe", "/test-compat/probe.sh", "Path to probe.sh")
	registryFlag := fs.String("registry", "", "Registry URI for phpup install (propagated via PHPUP_REGISTRY env)")
	compatMatrixFlag := fs.String("compat-matrix", "", "Path to compat-matrix.md; empty disables compat-diff")
	goldenDirFlag := fs.String("golden-dir", "", "Directory containing v2 golden probe JSONs; empty disables compat-diff")
	deviationsFlag := fs.String("deviations", "", "Path to append deviation artifact JSON; empty disables artifact writes")
	if err := fs.Parse(args); err != nil {
		return nil, err
	}
	if *osFlag == "" || *archFlag == "" || *phpFlag == "" {
		return nil, errors.New("phpup internal test-cell: --os, --arch, and --php are required")
	}
	return &testCellOpts{
		OS: *osFlag, Arch: *archFlag, PHP: *phpFlag,
		FixturesPath:   *fixturesFlag,
		ProbePath:      *probeFlag,
		RegistryURI:    *registryFlag,
		CompatMatrix:   *compatMatrixFlag,
		GoldenDir:      *goldenDirFlag,
		DeviationsPath: *deviationsFlag,
	}, nil
}

// fixtureOutcome captures the result of running a single fixture. Err is
// nil on full success, or set to the first failure encountered (install,
// probe, parse, or invariant assertion).
type fixtureOutcome struct {
	Name     string
	Err      error
	ProbeOut map[string]any
}

// RunTestCell is the entry point for `phpup internal test-cell`. Invokes
// phpup install + probe.sh for each matching fixture, asserts invariants,
// aggregates outcomes, and returns a non-nil error if any fixture failed.
func RunTestCell(ctx context.Context, args []string) error {
	opts, err := parseTestCellFlags(args)
	if err != nil {
		return err
	}
	set, err := Load(opts.FixturesPath)
	if err != nil {
		return fmt.Errorf("phpup internal test-cell: load fixtures: %w", err)
	}
	fixtures := set.Filter(opts.OS, opts.Arch, opts.PHP)
	if len(fixtures) == 0 {
		fmt.Printf("phpup internal test-cell: no matching fixtures for os=%s arch=%s php=%s\n", opts.OS, opts.Arch, opts.PHP)
		return nil
	}

	if _, err := os.Stat(opts.ProbePath); err != nil {
		return fmt.Errorf("phpup internal test-cell: probe.sh unavailable: %w", err)
	}

	var outcomes []fixtureOutcome
	for i := range fixtures {
		outcomes = append(outcomes, runFixture(ctx, opts, &fixtures[i]))
	}

	printFixtureSummary(os.Stdout, outcomes)
	var failed int
	for _, o := range outcomes {
		if o.Err != nil {
			failed++
		}
	}
	if failed > 0 {
		return fmt.Errorf("phpup internal test-cell: %d of %d fixture(s) failed", failed, len(outcomes))
	}
	return nil
}

// runFixture composes the env for the fixture, runs phpup install, then
// probe.sh, parses the probe output, and asserts invariants. Returns a
// fixtureOutcome with Err set on the first failure encountered.
func runFixture(ctx context.Context, opts *testCellOpts, f *Fixture) fixtureOutcome {
	out := fixtureOutcome{Name: f.Name}
	fmt.Printf("phpup internal test-cell: [run] fixture=%s\n", f.Name)

	// 1. Snapshot env + PATH for probe.sh's delta computation.
	workDir, err := os.MkdirTemp("", "phpup-test-cell-"+f.Name+"-*")
	if err != nil {
		out.Err = fmt.Errorf("mktemp: %w", err)
		return out
	}
	// Leave workDir around for debug; caller decides to clean up. In a
	// container the filesystem is discarded on exit anyway.

	envBefore := filepath.Join(workDir, "env-before")
	pathBefore := filepath.Join(workDir, "path-before")
	iniKeys := filepath.Join(workDir, "ini-keys.txt")
	probeOut := filepath.Join(workDir, "probe-out.json")
	stdoutLog := filepath.Join(workDir, "install-stdout.log")
	stderrLog := filepath.Join(workDir, "install-stderr.log")

	if err := writeEnvSnapshot(envBefore); err != nil {
		out.Err = fmt.Errorf("snapshot env: %w", err)
		return out
	}
	if err := os.WriteFile(pathBefore, []byte(os.Getenv("PATH")), 0o600); err != nil {
		out.Err = fmt.Errorf("snapshot PATH: %w", err)
		return out
	}
	if err := writeIniKeysFromFixture(iniKeys, f); err != nil {
		out.Err = fmt.Errorf("write ini-keys: %w", err)
		return out
	}

	// 2. Run phpup install.
	stdoutFile, err := os.Create(stdoutLog)
	if err != nil {
		out.Err = err
		return out
	}
	defer func() { _ = stdoutFile.Close() }()
	stderrFile, err := os.Create(stderrLog)
	if err != nil {
		out.Err = err
		return out
	}
	defer func() { _ = stderrFile.Close() }()

	// Create GITHUB_ENV / GITHUB_PATH files phpup install writes to during
	// the simulated GHA runner dance. Parse them afterwards so probe.sh
	// inherits the exported env/PATH — otherwise php's location isn't on
	// PATH and probe.sh fails with "php: command not found".
	gitEnvFile := filepath.Join(workDir, "github-env")
	gitPathFile := filepath.Join(workDir, "github-path")
	if err := os.WriteFile(gitEnvFile, nil, 0o600); err != nil {
		out.Err = fmt.Errorf("create github-env: %w", err)
		return out
	}
	if err := os.WriteFile(gitPathFile, nil, 0o600); err != nil {
		out.Err = fmt.Errorf("create github-path: %w", err)
		return out
	}

	installEnv := composeInstallEnv(opts, f)
	installEnv["GITHUB_ENV"] = gitEnvFile
	installEnv["GITHUB_PATH"] = gitPathFile
	if err := getInstaller()(ctx, installEnv, stdoutFile, stderrFile); err != nil {
		stderrBytes, _ := os.ReadFile(stderrLog)
		out.Err = fmt.Errorf("phpup install failed: %w; stderr tail:\n%s", err, tailBytes(stderrBytes, 20))
		return out
	}

	// Build probe env: phpup install's GITHUB_ENV + GITHUB_PATH exports
	// must reach probe.sh. Parse the files into env vars / a PATH prefix.
	probeEnv, err := composeProbeEnv(gitEnvFile, gitPathFile)
	if err != nil {
		out.Err = fmt.Errorf("compose probe env: %w", err)
		return out
	}

	// 3. Invoke probe.sh via bash so the script doesn't need an exec bit
	// (mount boundaries sometimes strip it) and so gosec's taint analysis
	// has a fixed executable to reason about.
	//nolint:gosec // G204 false positive: bash is a fixed compile-time argv[0];
	// opts.ProbePath is set by the --probe flag (trusted operator input,
	// defaulted to the container-mounted path); remaining argv is a fixed
	// set of filepath.Join-derived temp paths.
	cmd := exec.CommandContext(ctx, "bash", opts.ProbePath,
		probeOut, envBefore, pathBefore, iniKeys)
	cmd.Env = probeEnv
	probeStdout, err := cmd.CombinedOutput()
	if err != nil {
		out.Err = fmt.Errorf("probe.sh failed: %w; output:\n%s", err, string(probeStdout))
		return out
	}

	// 4. Parse probe output.
	pdata, err := os.ReadFile(probeOut)
	if err != nil {
		out.Err = fmt.Errorf("read probe output: %w", err)
		return out
	}
	var parsed map[string]any
	if err := json.Unmarshal(pdata, &parsed); err != nil {
		out.Err = fmt.Errorf("parse probe JSON: %w; raw:\n%s", err, string(pdata))
		return out
	}
	out.ProbeOut = parsed

	// 5. Assert invariants.
	if err := assertFixtureInvariants(f, parsed); err != nil {
		out.Err = err
		return out
	}

	// 6. Compat-diff gate (canonical cell + fixture.Compat + golden exists).
	if shouldRunCompat(opts, f) {
		goldenPath := filepath.Join(opts.GoldenDir, f.Name+".json")
		if _, err := os.Stat(goldenPath); err != nil {
			// Missing golden on a fixture that opted in is a hard error: it
			// means the refresh workflow has never captured this fixture, or
			// a golden was deleted. Better to fail loudly than silently skip.
			out.Err = fmt.Errorf("compat: golden missing for fixture %q at %s: run `gh workflow run compat-golden-refresh.yml` to capture it", f.Name, goldenPath)
			return out
		}
		devs, err := compatdiff.DiffFiles(probeOut, goldenPath, opts.CompatMatrix, f.Name)
		if err != nil {
			out.Err = fmt.Errorf("compat: diff failed for fixture %q: %w", f.Name, err)
			return out
		}
		if len(devs) > 0 {
			if opts.DeviationsPath != "" {
				cell := opts.OS + "-" + opts.Arch + "-" + opts.PHP
				if werr := AppendDeviations(opts.DeviationsPath, cell, f.Name, devs); werr != nil {
					// Artifact write failure is diagnostic, not compat-diff
					// failure; log but don't overwrite the deviation error.
					fmt.Fprintf(os.Stderr, "compat: append deviations artifact: %v\n", werr)
				}
			}
			out.Err = fmt.Errorf("compat: fixture %q has %d unexplained deviation(s) vs v2 golden", f.Name, len(devs))
			return out
		}
	}
	return out
}

// shouldRunCompat returns true iff the current cell is the canonical
// compat cell (noble/x86_64/8.4), the fixture opted in via Compat: true,
// and the outer runner supplied both compat paths. Non-empty paths
// indicate the outer runner intends compat to run in this cell; the
// on-disk presence of the per-fixture golden file is re-verified
// inside step 6 of runFixture (the `os.Stat(goldenPath)` check).
func shouldRunCompat(opts *testCellOpts, f *Fixture) bool {
	if !f.Compat {
		return false
	}
	// Defense-in-depth: the CI-driven path (runner.go:runCell) always
	// passes both flags with constant non-empty values, so this branch
	// is only reachable by test-direct callers that build testCellOpts
	// by hand. Guard kept so a future bug in flag threading fails
	// closed (compat off) rather than open (nil-deref in DiffFiles).
	if opts.CompatMatrix == "" || opts.GoldenDir == "" {
		return false
	}
	return opts.OS == "noble" && opts.Arch == "x86_64" && opts.PHP == "8.4"
}

// writeEnvSnapshot captures os.Environ() into path, one KEY=value per line,
// sorted alphabetically. Used by probe.sh to diff against the post-install
// env for the env_delta computation.
func writeEnvSnapshot(path string) error {
	env := os.Environ()
	sort.Strings(env)
	data := strings.Join(env, "\n") + "\n"
	return os.WriteFile(path, []byte(data), 0o600)
}

// writeIniKeysFromFixture emits the ini keys probe.sh should query.
// Combines a curated default list with any keys present in the fixture's
// INIValues. Newline-separated; duplicates are de-duped.
func writeIniKeysFromFixture(path string, f *Fixture) error {
	defaults := []string{
		"memory_limit", "date.timezone", "error_reporting",
		"display_errors", "post_max_size", "upload_max_filesize",
		"max_execution_time",
	}
	keys := map[string]struct{}{}
	for _, d := range defaults {
		keys[d] = struct{}{}
	}
	for _, kv := range strings.Split(f.INIValues, ",") {
		kv = strings.TrimSpace(kv)
		if kv == "" {
			continue
		}
		if eq := strings.IndexByte(kv, '='); eq > 0 {
			keys[strings.TrimSpace(kv[:eq])] = struct{}{}
		}
	}
	names := make([]string, 0, len(keys))
	for k := range keys {
		names = append(names, k)
	}
	sort.Strings(names)
	data := strings.Join(names, "\n") + "\n"
	return os.WriteFile(path, []byte(data), 0o600)
}

// composeInstallEnv builds the env overlay phpup install expects for this
// fixture: INPUT_PHP-VERSION, INPUT_EXTENSIONS, INPUT_INI-VALUES,
// INPUT_COVERAGE, INPUT_INI-FILE. PHPUP_REGISTRY inherited from the
// cell opts (which got it from --registry).
//
// plan.FromEnv reads RUNNER_OS and RUNNER_ARCH (GHA convention) to build
// the lockfile key `php:<ver>:<os>:<arch>:<ts>`. Inside our bare-ubuntu
// test container there are no GHA variables set, so without these the
// key becomes malformed (e.g. `php:8.4://nts`) and resolve fails. Inject
// them explicitly: OS is always Linux in the container; ARCH is mapped
// from the cell's canonical arch back to GHA's X64/ARM64 spelling so
// plan.normalizeArch round-trips cleanly.
func composeInstallEnv(opts *testCellOpts, f *Fixture) map[string]string {
	env := map[string]string{
		"INPUT_PHP-VERSION": f.PHPVersion,
		"INPUT_EXTENSIONS":  f.Extensions,
		"INPUT_INI-VALUES":  f.INIValues,
		"INPUT_COVERAGE":    f.Coverage,
		"RUNNER_OS":         "Linux",
		"RUNNER_ARCH":       runnerArchFromCellArch(opts.Arch),
	}
	if f.INIFile != "" {
		env["INPUT_INI-FILE"] = f.INIFile
	}
	if opts.RegistryURI != "" {
		env["PHPUP_REGISTRY"] = opts.RegistryURI
	}
	return env
}

// runnerArchFromCellArch maps the test-cell's canonical arch to GHA's
// RUNNER_ARCH values: x86_64 -> X64, aarch64 -> ARM64. Unknown arches
// are passed through unchanged so downstream normalizeArch can still
// make a best-effort match.
func runnerArchFromCellArch(arch string) string {
	switch arch {
	case "x86_64":
		return "X64"
	case "aarch64":
		return "ARM64"
	default:
		return arch
	}
}

// assertFixtureInvariants checks the probe output against what the fixture
// requested:
//   - php_version non-empty and starts with fixture.PHPVersion.
//   - every extension in fixture.Extensions (minus exclusions) is present
//     in probe.extensions (case-insensitive, ignoring "none" reset marker).
//   - every ini key/value in fixture.INIValues matches probe.ini[key].
func assertFixtureInvariants(f *Fixture, probe map[string]any) error {
	// php_version
	pv, _ := probe["php_version"].(string)
	if pv == "" {
		return fmt.Errorf("probe has no php_version")
	}
	if !strings.HasPrefix(pv, f.PHPVersion+".") && pv != f.PHPVersion {
		return fmt.Errorf("php_version = %q, want prefix %q", pv, f.PHPVersion+".")
	}

	// extensions
	wantedExts, excluded := parseExtensionsList(f.Extensions)
	loaded := map[string]struct{}{}
	if extArr, ok := probe["extensions"].([]any); ok {
		for _, e := range extArr {
			if s, ok := e.(string); ok {
				loaded[strings.ToLower(s)] = struct{}{}
			}
		}
	}
	for _, want := range wantedExts {
		w := strings.ToLower(want)
		if _, ok := loaded[w]; !ok {
			return fmt.Errorf("extension %q not loaded (got %s)", want, setKeys(loaded))
		}
	}
	for _, nope := range excluded {
		n := strings.ToLower(nope)
		if _, ok := loaded[n]; ok {
			return fmt.Errorf("extension %q was excluded but still loaded", nope)
		}
	}

	// ini-values
	iniMap, _ := probe["ini"].(map[string]any)
	for _, kv := range strings.Split(f.INIValues, ",") {
		kv = strings.TrimSpace(kv)
		if kv == "" {
			continue
		}
		eq := strings.IndexByte(kv, '=')
		if eq < 0 {
			continue
		}
		key := strings.TrimSpace(kv[:eq])
		want := strings.TrimSpace(kv[eq+1:])
		got, _ := iniMap[key].(string)
		if got != want {
			return fmt.Errorf("ini[%s] = %q, want %q", key, got, want)
		}
	}
	return nil
}

// parseExtensionsList splits fixture.Extensions into (wanted, excluded).
// Tokens starting with ':' are exclusions; "none" is an ignored reset marker.
func parseExtensionsList(s string) (wanted, excluded []string) {
	for _, p := range strings.Split(s, ",") {
		p = strings.TrimSpace(p)
		switch {
		case p == "" || p == "none":
			continue
		case strings.HasPrefix(p, ":"):
			excluded = append(excluded, strings.TrimPrefix(p, ":"))
		default:
			wanted = append(wanted, p)
		}
	}
	return wanted, excluded
}

// setKeys returns a sorted, bracketed rendering of m's keys. Used only for
// error diagnostics (so the operator can see what WAS loaded when an
// expected extension is missing).
func setKeys(m map[string]struct{}) string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return "[" + strings.Join(keys, ",") + "]"
}

// printFixtureSummary writes a per-fixture summary table to w. Errors
// rendered inline: "OK" or "FAIL: <reason>". Fire-and-forget Fprintf —
// summary is informational; the real exit status is computed in RunTestCell.
func printFixtureSummary(w io.Writer, outcomes []fixtureOutcome) {
	_, _ = fmt.Fprintln(w, "\n=== test-cell fixture summary ===")
	for _, o := range outcomes {
		status := "OK"
		if o.Err != nil {
			status = "FAIL: " + o.Err.Error()
		}
		_, _ = fmt.Fprintf(w, "  %-40s %s\n", o.Name, status)
	}
}

// tailBytes returns the last `lines` lines of b as a newline-joined string.
// Used for compact install-stderr tails in error diagnostics.
func tailBytes(b []byte, lines int) string {
	s := bufio.NewScanner(strings.NewReader(string(b)))
	var rows []string
	for s.Scan() {
		rows = append(rows, s.Text())
	}
	if len(rows) > lines {
		rows = rows[len(rows)-lines:]
	}
	return strings.Join(rows, "\n")
}
