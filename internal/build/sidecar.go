package build

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/static"
	"github.com/google/go-containerregistry/pkg/v1/types"

	"github.com/buildrush/setup-php/internal/registry"
)

// Sidecar is an ephemeral distribution:3 OCI registry running on an
// isolated docker network. Used by phpup build ext to stage the
// prerequisite php-core artifact so the in-container fetch-core.sh
// pulls from it without any script change.
type Sidecar struct {
	// Name is the docker container name assigned at start time.
	Name string
	// Network is the docker network the sidecar is attached to. Build
	// containers MUST join this network to reach the sidecar by name.
	Network string
	// InNetworkHost is the hostname:port other containers on Network
	// should dial to reach the sidecar (e.g. "phpup-sidecar-…:5000").
	// Not reachable from the host.
	InNetworkHost string
	// HostHost is the hostname:port reachable from the host process,
	// used by go-containerregistry remote.Write to seed the sidecar.
	// Typically "127.0.0.1:<ephemeral-port>".
	HostHost string
}

// SidecarLifecycle is the package-level factory used by BuildExt. Tests
// override via SetSidecarLifecycle to inject a fake that doesn't touch
// real docker. Start returns the running sidecar and a stop function the
// caller MUST defer to tear down both the container and the network.
type SidecarLifecycle interface {
	Start(ctx context.Context) (*Sidecar, func(context.Context) error, error)
	SeedCore(ctx context.Context, sc *Sidecar, source registry.Store, ref registry.Ref, owner, tag string) error
}

// sidecarLifecycleMu protects sidecarLifecycle during SetSidecarLifecycle
// swaps. Swaps are rare (test setup/teardown only) so the mutex cost is
// negligible, but it rules out a data race when two tests happen to swap
// concurrently. Tests that call SetSidecarLifecycle MUST NOT use
// t.Parallel() because the lifecycle is a package-level global.
var sidecarLifecycleMu sync.Mutex

// sidecarLifecycle is the SidecarLifecycle BuildExt dispatches through
// when no test has overridden it via SetSidecarLifecycle.
var sidecarLifecycle SidecarLifecycle = defaultSidecarLifecycle{}

// SetSidecarLifecycle swaps the package-level SidecarLifecycle and
// returns a restore function that callers MUST defer to revert.
// The typical test pattern is:
//
//	restore := SetSidecarLifecycle(myFake)
//	defer restore()
//
// The package-level state means tests that call SetSidecarLifecycle must
// not run in parallel — they share one global lifecycle.
func SetSidecarLifecycle(l SidecarLifecycle) func() {
	sidecarLifecycleMu.Lock()
	prev := sidecarLifecycle
	sidecarLifecycle = l
	sidecarLifecycleMu.Unlock()
	return func() {
		sidecarLifecycleMu.Lock()
		sidecarLifecycle = prev
		sidecarLifecycleMu.Unlock()
	}
}

// currentSidecarLifecycle returns the installed SidecarLifecycle under
// lock so callers observe a consistent value even if a swap is in
// progress.
func currentSidecarLifecycle() SidecarLifecycle {
	sidecarLifecycleMu.Lock()
	defer sidecarLifecycleMu.Unlock()
	return sidecarLifecycle
}

// defaultSidecarLifecycle is the production SidecarLifecycle. Its Start
// method shells out to `docker` to run distribution:3 on a fresh
// network; SeedCore pushes the prerequisite bundle via remote.Write.
type defaultSidecarLifecycle struct{}

// sidecarLabel marks every container and network this lifecycle
// creates so sweepStaleSidecars can reliably clean up zombies from
// prior runs without touching unrelated docker state.
const sidecarLabel = "buildrush.phpup.sidecar=1"

// sweepStaleSidecars removes any containers or networks from prior runs
// that didn't clean up after themselves (e.g. outer timeout killed the
// process before the defer). Scoped by label so unrelated docker state
// is untouched. Errors are ignored — if docker can't list or remove the
// resources, the subsequent Start will either succeed (the zombies didn't
// collide) or fail with its own clear error.
func sweepStaleSidecars(ctx context.Context) {
	// Best-effort; failures are not actionable from the caller's
	// perspective and would just add noise on first-ever-run (no
	// prior label to match).

	// Containers first (they hold the network in use, so must go before the network).
	if out, err := execDocker(ctx, "ps", "-aq", "--filter", "label="+sidecarLabel); err == nil {
		for _, id := range strings.Fields(string(out)) {
			_, _ = execDocker(ctx, "rm", "-f", id)
		}
	}
	// Then networks.
	if out, err := execDocker(ctx, "network", "ls", "-q", "--filter", "label="+sidecarLabel); err == nil {
		for _, id := range strings.Fields(string(out)) {
			_, _ = execDocker(ctx, "network", "rm", id)
		}
	}
}

// Start spins up a distribution:3 container on a fresh network and
// waits for its /v2/ endpoint to become reachable. Returns the
// *Sidecar and a stop function the caller MUST defer to tear down
// both the container and the network.
func (defaultSidecarLifecycle) Start(ctx context.Context) (*Sidecar, func(context.Context) error, error) {
	// Opportunistic: clean up any zombie sidecars from prior runs that
	// aborted before their deferred stop (panic, outer timeout,
	// SIGKILL). Scoped by label so unrelated docker state is untouched.
	sweepStaleSidecars(ctx)

	// Ephemeral unique names to avoid collision across concurrent
	// runs. Timestamp in UTC so the name is deterministic at the
	// nanosecond level; strip the "." from the fractional-second
	// separator so the result fits docker's name constraints.
	tag := strings.ReplaceAll(time.Now().UTC().Format("20060102T150405.000000000"), ".", "")
	network := "phpup-build-" + tag
	containerName := "phpup-sidecar-" + tag

	if err := dockerCmdCombined(ctx, "network", "create", "--label", sidecarLabel, network); err != nil {
		return nil, nil, fmt.Errorf("sidecar: create network: %w", err)
	}

	// --publish 127.0.0.1::5000 asks docker to pick a free host port
	// bound to the loopback — the sidecar is intentionally not exposed
	// on the public interface; it's only reachable by the host process
	// and by build containers on the same docker network.
	runOut, err := dockerCmdOutput(ctx,
		"run", "-d", "--rm",
		"--name", containerName,
		"--network", network,
		"--label", sidecarLabel,
		"--publish", "127.0.0.1::5000",
		"distribution/distribution:3",
	)
	if err != nil {
		_ = dockerCmdCombined(context.Background(), "network", "rm", network)
		return nil, nil, fmt.Errorf("sidecar: run registry: %w (output: %s)", err, runOut)
	}
	port, err := dockerPublishedPort(ctx, containerName, 5000)
	if err != nil {
		_ = dockerCmdCombined(context.Background(), "rm", "-f", containerName)
		_ = dockerCmdCombined(context.Background(), "network", "rm", network)
		return nil, nil, fmt.Errorf("sidecar: inspect port: %w", err)
	}

	sc := &Sidecar{
		Name:          containerName,
		Network:       network,
		InNetworkHost: containerName + ":5000",
		HostHost:      "127.0.0.1:" + port,
	}

	if err := waitForRegistry(ctx, sc.HostHost); err != nil {
		_ = dockerCmdCombined(context.Background(), "rm", "-f", containerName)
		_ = dockerCmdCombined(context.Background(), "network", "rm", network)
		return nil, nil, fmt.Errorf("sidecar: wait: %w", err)
	}

	stop := func(stopCtx context.Context) error {
		var errs []error
		if err := dockerCmdCombined(stopCtx, "rm", "-f", containerName); err != nil {
			errs = append(errs, err)
		}
		if err := dockerCmdCombined(stopCtx, "network", "rm", network); err != nil {
			errs = append(errs, err)
		}
		if len(errs) > 0 {
			return fmt.Errorf("sidecar: stop: %v", errs)
		}
		return nil
	}
	return sc, stop, nil
}

// SeedCore copies a php-core bundle from source into the sidecar at
// "<HostHost>/<owner>/php-core:<tag>". Reads the source bundle via
// source.Fetch, builds a two-layer OCI image (matching layoutStore.Push's
// shape so oras-pulling the seeded image from the sidecar produces bytes
// identical to what the source store returns), and pushes via
// remote.Write with name.Insecure since distribution:3 serves HTTP by
// default.
func (defaultSidecarLifecycle) SeedCore(ctx context.Context, sc *Sidecar, source registry.Store, ref registry.Ref, owner, tag string) error {
	rc, meta, err := source.Fetch(ctx, ref)
	if err != nil {
		return fmt.Errorf("sidecar.SeedCore: fetch source: %w", err)
	}
	defer func() { _ = rc.Close() }()
	bundle, err := io.ReadAll(rc)
	if err != nil {
		return fmt.Errorf("sidecar.SeedCore: read source bundle: %w", err)
	}

	img, err := buildTwoLayerImage(bundle, meta)
	if err != nil {
		return err
	}

	// distribution:3 speaks HTTP by default, so name.Insecure is required;
	// remote.Write falls back to HTTP when the target ref is parsed with
	// name.Insecure.
	target, err := name.ParseReference(sc.HostHost+"/"+owner+"/php-core:"+tag, name.Insecure)
	if err != nil {
		return fmt.Errorf("sidecar.SeedCore: parse ref: %w", err)
	}
	if err := remote.Write(target, img, remote.WithContext(ctx)); err != nil {
		return fmt.Errorf("sidecar.SeedCore: push: %w", err)
	}
	return nil
}

// buildTwoLayerImage assembles the OCI image shape that layoutStore.Push
// uses for bundles: layer 0 carries the raw bundle bytes; layer 1
// (when meta is non-nil) carries the serialised meta sidecar. Keeping
// the shape in sync lets fetch-core.sh in the build container pull
// from the sidecar with oras and get byte-identical output to what
// registry.Store.Fetch would return directly.
func buildTwoLayerImage(bundle []byte, meta *registry.Meta) (v1.Image, error) {
	img := emptyImage()
	bundleLayer := static.NewLayer(bundle, types.OCILayer)
	var err error
	img, err = mutate.AppendLayers(img, bundleLayer)
	if err != nil {
		return nil, fmt.Errorf("sidecar: append bundle layer: %w", err)
	}
	if meta != nil {
		metaBytes, err := marshalMeta(meta)
		if err != nil {
			return nil, fmt.Errorf("sidecar: marshal meta: %w", err)
		}
		metaLayer := static.NewLayer(metaBytes, types.OCILayer)
		img, err = mutate.AppendLayers(img, metaLayer)
		if err != nil {
			return nil, fmt.Errorf("sidecar: append meta layer: %w", err)
		}
	}
	return img, nil
}

// execDocker runs docker with the given args and returns the combined
// stdout/stderr. G204 is a genuine false positive: exec.CommandContext
// passes argv directly to execve(2) (no shell), and all args come from
// typed sources within this package (static strings + lifecycle-state
// fields) — the wrapper's purpose is precisely to spawn docker with
// dynamic argv, so a fixed argv is impossible by design.
func execDocker(ctx context.Context, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, "docker", args...).CombinedOutput() //nolint:gosec // G204 false positive: exec.CommandContext passes argv directly to execve(2) (no shell), and all args come from typed sources in this package.
}

// dockerCmdCombined runs a docker command and returns its combined
// output folded into the error. Used for commands whose stdout is
// diagnostic rather than load-bearing.
func dockerCmdCombined(ctx context.Context, args ...string) error {
	out, err := execDocker(ctx, args...)
	if err != nil {
		return fmt.Errorf("docker %s: %w (%s)", strings.Join(args, " "), err, string(out))
	}
	return nil
}

// dockerCmdOutput runs a docker command and returns its trimmed
// combined output. Used where the caller wants the stdout (e.g. the
// container id returned by `docker run -d`).
func dockerCmdOutput(ctx context.Context, args ...string) (string, error) {
	out, err := execDocker(ctx, args...)
	return strings.TrimSpace(string(out)), err
}

// dockerPublishedPort inspects the container and returns the published
// host port that maps to containerPort. Uses docker's go-template
// formatter so we don't need to parse JSON.
func dockerPublishedPort(ctx context.Context, container string, containerPort int) (string, error) {
	fmtFlag := fmt.Sprintf(`{{(index (index .NetworkSettings.Ports "%d/tcp") 0).HostPort}}`, containerPort)
	out, err := execDocker(ctx, "inspect", "--format", fmtFlag, container)
	if err != nil {
		return "", fmt.Errorf("inspect: %w (%s)", err, string(out))
	}
	return strings.TrimSpace(string(out)), nil
}

// waitForRegistry polls the sidecar's /v2/ endpoint until it returns a
// non-5xx response or the 30-second deadline elapses. distribution:3
// returns 200 for anonymous /v2/ as soon as it's listening; a 4xx
// would also mean "up and serving", so we only treat 5xx as "still
// starting".
func waitForRegistry(ctx context.Context, host string) error {
	url := "http://" + host + "/v2/"
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodHead, url, http.NoBody)
		if err != nil {
			return fmt.Errorf("build request: %w", err)
		}
		resp, err := http.DefaultClient.Do(req)
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode < 500 {
				return nil
			}
		}
		time.Sleep(200 * time.Millisecond)
	}
	return fmt.Errorf("registry at %s did not become healthy within 30s", host)
}

// emptyImage returns a fresh v1.Image to append layers to. Kept as a
// function (not a constant) so tests can swap if needed and so the
// import of pkg/v1/empty lives at exactly one site.
func emptyImage() v1.Image { return empty.Image }

// marshalMeta serialises a registry.Meta for embedding as the second
// layer of a seeded image. Kept separate from buildTwoLayerImage so
// the error path is localised and the marshal can be mocked if needed.
func marshalMeta(m *registry.Meta) ([]byte, error) {
	return json.Marshal(m)
}
