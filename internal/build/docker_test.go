package build

import (
	"context"
	"errors"
	"os/exec"
	"reflect"
	"strings"
	"testing"
	"time"
)

// recorder is a RunnerFunc test double that captures the last DockerRunOpts
// it received. Tests install it via SetRunner to assert the argv phpup would
// build without invoking real docker. The captured value is stored by
// dereference rather than by pointer so assertions remain immune to any
// post-return mutation of the caller's opts struct.
type recorder struct {
	got DockerRunOpts
	err error
}

// asRunner returns a RunnerFunc closure that captures into r and returns r.err.
func (r *recorder) asRunner() RunnerFunc {
	return func(_ context.Context, opts *DockerRunOpts) error {
		r.got = *opts
		return r.err
	}
}

// TestDockerRun_PropagatesOptsToRunner verifies that DockerRun passes every
// field of the opts struct to the installed runner unchanged. This is the
// foundational guarantee Task 4/5 rely on when asserting the argv phpup
// would construct for a given build.
func TestDockerRun_PropagatesOptsToRunner(t *testing.T) {
	r := &recorder{}
	restore := SetRunner(r.asRunner())
	defer restore()

	opts := &DockerRunOpts{
		Image:    "ubuntu:22.04",
		Platform: "linux/arm64",
		Network:  "test-net",
		Mounts: []Mount{
			{Host: "/a", Container: "/mnt/a", ReadOnly: true},
			{Host: "/b", Container: "/mnt/b"},
		},
		Env: map[string]string{"FOO": "bar", "BAZ": "qux"},
		Cmd: []string{"bash", "-c", "echo hi"},
	}
	if err := DockerRun(context.Background(), opts); err != nil {
		t.Fatalf("DockerRun: %v", err)
	}
	if !reflect.DeepEqual(r.got, *opts) {
		t.Errorf("recorder.got = %+v, want %+v", r.got, *opts)
	}
}

// TestDockerRun_PropagatesError verifies that an error returned by the
// installed runner flows back through DockerRun unchanged. Task 4/5 rely on
// this so build failures surface to the CLI caller.
func TestDockerRun_PropagatesError(t *testing.T) {
	r := &recorder{err: errors.New("boom")}
	restore := SetRunner(r.asRunner())
	defer restore()

	err := DockerRun(context.Background(), &DockerRunOpts{Image: "alpine:3"})
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("err = %v, want containing \"boom\"", err)
	}
}

// TestDockerRun_ContextCancellation verifies that DockerRun returns promptly
// once the caller cancels the context, even when the underlying runner would
// otherwise block indefinitely. The real runner relies on
// exec.CommandContext's SIGKILL behaviour; here we simulate that with a
// runner that blocks on ctx.Done and returns ctx.Err.
func TestDockerRun_ContextCancellation(t *testing.T) {
	slow := func(ctx context.Context, _ *DockerRunOpts) error {
		<-ctx.Done()
		return ctx.Err()
	}
	restore := SetRunner(slow)
	defer restore()

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() { errCh <- DockerRun(ctx, &DockerRunOpts{Image: "alpine:3"}) }()
	cancel()
	select {
	case err := <-errCh:
		if !errors.Is(err, context.Canceled) {
			t.Errorf("err = %v, want context.Canceled", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("DockerRun did not return after cancel within 2s")
	}
}

// TestRealDockerRun_BuildsExpectedArgs asserts the exact argv phpup's real
// runner would pass to `docker`. argvFor is extracted precisely so this
// test can exercise argv construction without invoking exec. The expected
// ordering — run/--rm first, flags in field order, env sorted, image, cmd —
// is the contract Task 4/5 depend on.
func TestRealDockerRun_BuildsExpectedArgs(t *testing.T) {
	args := argvFor(&DockerRunOpts{
		Image:    "ubuntu:22.04",
		Platform: "linux/amd64",
		Network:  "build-net",
		Mounts: []Mount{
			{Host: "/src", Container: "/workspace", ReadOnly: true},
			{Host: "/out", Container: "/tmp"},
		},
		Env: map[string]string{"Z": "last", "A": "first"},
		Cmd: []string{"bash", "-c", "echo hi"},
	})
	want := []string{
		"run", "--rm",
		"--platform", "linux/amd64",
		"--network", "build-net",
		"-v", "/src:/workspace:ro",
		"-v", "/out:/tmp",
		"-e", "A=first", // env keys sorted alphabetically
		"-e", "Z=last",
		"ubuntu:22.04",
		"bash", "-c", "echo hi",
	}
	if !reflect.DeepEqual(args, want) {
		t.Errorf("argv mismatch\n got = %v\nwant = %v", args, want)
	}
}

// TestRealDockerRun_BuildsMinimalArgs asserts that optional flags (platform,
// network, mounts, env, cmd) are omitted when their corresponding
// DockerRunOpts fields are zero. Verifies argvFor doesn't synthesise an
// empty --platform flag value or similar garbage.
func TestRealDockerRun_BuildsMinimalArgs(t *testing.T) {
	args := argvFor(&DockerRunOpts{Image: "alpine:3"})
	want := []string{"run", "--rm", "alpine:3"}
	if !reflect.DeepEqual(args, want) {
		t.Errorf("argv mismatch\n got = %v\nwant = %v", args, want)
	}
}

// TestRealDockerRun_ImageRequired asserts the guardrail on the real runner:
// an empty Image is a programmer error, not a docker error to pipe through.
func TestRealDockerRun_ImageRequired(t *testing.T) {
	err := realDockerRun(context.Background(), &DockerRunOpts{})
	if err == nil || !strings.Contains(err.Error(), "Image is required") {
		t.Errorf("err = %v, want containing \"Image is required\"", err)
	}
}

// TestRealDockerRun_SmokeIntegration exercises the real docker binary end to
// end with a tiny `alpine:3 echo hello` invocation. Skipped under -short and
// when docker isn't on PATH, so CI without docker is unaffected.
func TestRealDockerRun_SmokeIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping real docker smoke under -short")
	}
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skipf("docker not found in PATH: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	out := &strings.Builder{}
	err := DockerRun(ctx, &DockerRunOpts{
		Image:  "alpine:3",
		Cmd:    []string{"echo", "hello"},
		Stdout: out,
	})
	if err != nil {
		t.Fatalf("DockerRun: %v", err)
	}
	if !strings.Contains(out.String(), "hello") {
		t.Errorf("stdout = %q, want contains \"hello\"", out.String())
	}
}
