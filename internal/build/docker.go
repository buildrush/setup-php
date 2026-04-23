package build

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sort"
	"sync"
)

// DockerRunOpts describes a single `docker run --rm` invocation. Fields map
// one-for-one onto docker CLI flags so the argv phpup builds is easy to audit
// in test assertions.
type DockerRunOpts struct {
	// Image is the container image reference (required). Example: "ubuntu:22.04".
	Image string
	// Platform is the target platform override. Example: "linux/amd64".
	// Empty string skips the --platform flag and lets docker pick the host default.
	Platform string
	// Network is a named docker network the container joins. Empty string
	// skips the --network flag and uses docker's default bridge network.
	Network string
	// Mounts is the ordered list of volume binds. Caller order is preserved
	// in argv so readability matches the caller's intent.
	Mounts []Mount
	// Env is the set of environment variables to export inside the container.
	// Keys are sorted alphabetically when turned into argv so the -e flags
	// appear in a deterministic order — useful for test assertions; docker
	// itself doesn't care.
	Env map[string]string
	// Cmd is the argv passed after the image. Empty leaves the image's
	// default entrypoint/cmd intact.
	Cmd []string
	// Stdout is where container stdout is streamed. Nil defaults to os.Stdout.
	Stdout io.Writer
	// Stderr is where container stderr is streamed. Nil defaults to os.Stderr.
	Stderr io.Writer
}

// Mount describes a single bind mount. Both Host and Container MUST be
// absolute paths; the caller is responsible for resolving relative inputs.
type Mount struct {
	// Host is the absolute host-side path (required).
	Host string
	// Container is the absolute container-side path (required).
	Container string
	// ReadOnly appends ":ro" to the bind spec when true.
	ReadOnly bool
}

// RunnerFunc runs a single docker invocation. Tests substitute a fake via
// SetRunner to assert the argv phpup would construct without needing real
// docker in the test environment. Takes a pointer so the ~136-byte
// DockerRunOpts isn't copied on every call.
type RunnerFunc func(ctx context.Context, opts *DockerRunOpts) error

// runnerMu protects defaultRunner during SetRunner swaps. Swaps are rare
// (test setup/teardown only) so the mutex cost is negligible, but it rules
// out a data race when two tests happen to swap concurrently. That said,
// tests that call SetRunner MUST NOT use t.Parallel() because the runner
// is a package-level global: a swap in one test would leak into another.
var runnerMu sync.Mutex

// defaultRunner is the RunnerFunc DockerRun dispatches through when no test
// has overridden it via SetRunner.
var defaultRunner RunnerFunc = realDockerRun

// DockerRun dispatches the invocation through the currently-installed
// runner. Production callers go through the default realDockerRun; tests
// call SetRunner first to install a fake.
func DockerRun(ctx context.Context, opts *DockerRunOpts) error {
	runnerMu.Lock()
	r := defaultRunner
	runnerMu.Unlock()
	return r(ctx, opts)
}

// SetRunner swaps the package-level RunnerFunc and returns a restore
// function that callers MUST defer to revert. The typical test pattern is:
//
//	restore := SetRunner(myFake)
//	defer restore()
//
// The package-level state means tests that call SetRunner must not run in
// parallel — they share one global runner.
func SetRunner(r RunnerFunc) func() {
	runnerMu.Lock()
	prev := defaultRunner
	defaultRunner = r
	runnerMu.Unlock()
	return func() {
		runnerMu.Lock()
		defaultRunner = prev
		runnerMu.Unlock()
	}
}

// realDockerRun shells out to the `docker` binary via exec.CommandContext
// and streams stdio to the caller-provided writers (defaulting to
// os.Stdout/Stderr). Context cancellation relies on exec.CommandContext's
// built-in SIGKILL behaviour.
func realDockerRun(ctx context.Context, opts *DockerRunOpts) error {
	if opts.Image == "" {
		return fmt.Errorf("DockerRun: Image is required")
	}
	args := argvFor(opts)
	cmd := exec.CommandContext(ctx, "docker", args...) //nolint:gosec // G204 false positive: exec.CommandContext passes argv directly to execve(2) (no shell), and all args come from typed DockerRunOpts fields assembled by internal callers — the wrapper's purpose is precisely to spawn docker with dynamic argv, so a fixed argv is impossible by design.
	cmd.Stdout = coalesceWriter(opts.Stdout, os.Stdout)
	cmd.Stderr = coalesceWriter(opts.Stderr, os.Stderr)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("DockerRun %s: %w", opts.Image, err)
	}
	return nil
}

// argvFor returns the exact argv (minus the leading "docker") that
// realDockerRun would pass to exec.CommandContext. Extracted as a pure
// function so unit tests can assert on it without invoking exec.
// Takes a pointer so callers can assert on large opts without copy cost;
// the function reads the struct without mutating it.
func argvFor(opts *DockerRunOpts) []string {
	// Preallocate with a conservative estimate: "run" + "--rm" + two per
	// platform/network, two per mount, two per env, image, plus Cmd.
	args := make([]string, 0, 2+4+2*len(opts.Mounts)+2*len(opts.Env)+1+len(opts.Cmd))
	args = append(args, "run", "--rm")
	if opts.Platform != "" {
		args = append(args, "--platform", opts.Platform)
	}
	if opts.Network != "" {
		args = append(args, "--network", opts.Network)
	}
	for _, m := range opts.Mounts {
		spec := m.Host + ":" + m.Container
		if m.ReadOnly {
			spec += ":ro"
		}
		args = append(args, "-v", spec)
	}
	for _, k := range sortedKeys(opts.Env) {
		args = append(args, "-e", k+"="+opts.Env[k])
	}
	args = append(args, opts.Image)
	args = append(args, opts.Cmd...)
	return args
}

// sortedKeys returns the map's keys in alphabetical order. Empty and nil
// maps both produce a nil slice, which append handles fine.
func sortedKeys(m map[string]string) []string {
	if len(m) == 0 {
		return nil
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// coalesceWriter returns w if non-nil, else fallback. Used to let callers
// opt into a custom destination while defaulting to the process's stdio.
func coalesceWriter(w, fallback io.Writer) io.Writer {
	if w != nil {
		return w
	}
	return fallback
}
