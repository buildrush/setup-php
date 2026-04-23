package build

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/buildrush/setup-php/internal/registry"
)

// linuxAptPreamble installs the minimal host-side packages needed for
// the Ubuntu build containers to run the builder scripts. Shared
// between build-php and build-ext; keep in sync with Makefile's
// bundle-php / bundle-ext targets. Output is NOT silenced so apt
// diagnostics (mirror outages, DNS, missing packages) stream through
// to the user's stderr via DockerRun's default Stdout/Stderr wiring.
const linuxAptPreamble = "apt-get update && " +
	"apt-get install -y --no-install-recommends curl xz-utils ca-certificates && "

// Main is the entry point for `phpup build …`. args is the tail after
// the "build" subcommand token (so args[0] is "php" or "ext"). Returning
// a nil error means the requested build (or cache hit) succeeded; a
// non-nil error is safe to pass straight to log.Fatalf.
func Main(args []string) error {
	if len(args) == 0 {
		return errors.New("phpup build: usage: phpup build (php|ext) [flags]")
	}
	ctx := context.Background()
	switch args[0] {
	case "php":
		return BuildPHP(ctx, args[1:])
	case "ext":
		return BuildExt(ctx, args[1:])
	default:
		return fmt.Errorf("phpup build: unknown kind %q (want php or ext)", args[0])
	}
}

// BuildPHP runs the php-core build end to end. args is everything after
// "build php" (flags). Returns nil on success, or an error with the
// "phpup build php: <reason>" prefix the CLI dispatcher expects.
func BuildPHP(ctx context.Context, args []string) error {
	opts, err := parsePHPFlags(args)
	if err != nil {
		return err
	}

	// 1. Spec-hash.
	specHash, err := ComputeSpecHash(&SpecHashInputs{
		Kind:    "php",
		Version: opts.Version,
		OS:      "linux",
		Arch:    opts.Arch,
		TS:      opts.TS,
		Repo:    opts.Repo,
	})
	if err != nil {
		return fmt.Errorf("phpup build php: %w", err)
	}

	// 2. Open target store + cache-probe.
	store, err := registry.Open(opts.Registry)
	if err != nil {
		return fmt.Errorf("phpup build php: open registry: %w", err)
	}
	// Remote backends return ErrUnsupported for LookupBySpec; treat that
	// as a soft miss so callers without an oci-layout cache fall through
	// to building. Hard errors from a layout backend still propagate.
	ref, hit, err := store.LookupBySpec(ctx, "php-core", specHash)
	if errors.Is(err, registry.ErrUnsupported) {
		hit, err = false, nil
	}
	if err != nil {
		return fmt.Errorf("phpup build php: lookup by spec: %w", err)
	}
	if hit {
		fmt.Printf("phpup build php: cache hit %s (spec-hash %s)\n", ref.Digest, specHash)
		return nil
	}

	// 3. Prepare output mount dir.
	outDir, err := os.MkdirTemp("", "phpup-build-php-*")
	if err != nil {
		return fmt.Errorf("phpup build php: mktemp: %w", err)
	}
	defer func() { _ = os.RemoveAll(outDir) }()

	// 4. Invoke builder in docker.
	image, err := ubuntuImage(opts.OS)
	if err != nil {
		return fmt.Errorf("phpup build php: %w", err)
	}
	platform, err := dockerPlatform(opts.Arch)
	if err != nil {
		return fmt.Errorf("phpup build php: %w", err)
	}
	runOpts := &DockerRunOpts{
		Image:    image,
		Platform: platform,
		Mounts: []Mount{
			{Host: opts.Repo, Container: "/workspace", ReadOnly: true},
			// Mount at /tmp (not /tmp/out) because builders/linux/build-php.sh
			// invokes pack-bundle.sh with the hardcoded output path
			// /tmp/bundle.tar.zst, and pack-bundle.sh writes meta.json as a
			// sibling of the output tar. Mounting /tmp/out would leave both
			// files in the container's ephemeral /tmp and lose them on exit.
			// OUTPUT_DIR=/tmp/out still lives INSIDE the mount so the
			// builder's INSTALL_ROOT staging tree is preserved unchanged.
			{Host: outDir, Container: "/tmp", ReadOnly: false},
		},
		Env: map[string]string{
			"PHP_VERSION": opts.Version,
			"ARCH":        opts.Arch,
			"OUTPUT_DIR":  "/tmp/out",
			"WORKSPACE":   "/workspace",
		},
		Cmd: []string{"bash", "-c", linuxAptPreamble + "/workspace/builders/linux/build-php.sh"},
	}
	if err := DockerRun(ctx, runOpts); err != nil {
		return fmt.Errorf("phpup build php: docker: %w", err)
	}

	// 5. Read the bundle + meta from the mount dir. Paths are constructed
	// internally from os.MkdirTemp output, not user input; no filepath.Clean
	// needed (and the linter's G304 is excluded project-wide).
	bundlePath := filepath.Join(outDir, "bundle.tar.zst")
	metaPath := filepath.Join(outDir, "meta.json")
	bundle, err := os.Open(bundlePath)
	if err != nil {
		return fmt.Errorf("phpup build php: open bundle: %w", err)
	}
	defer func() { _ = bundle.Close() }()
	meta, err := parseMetaJSONFile(metaPath)
	if err != nil {
		return fmt.Errorf("phpup build php: parse meta.json: %w", err)
	}

	// 6. Push to the store.
	pushRef := registry.Ref{Name: "php-core"}
	ann := registry.Annotations{BundleName: "php-core", SpecHash: specHash}
	if err := store.Push(ctx, pushRef, bundle, meta, ann); err != nil {
		return fmt.Errorf("phpup build php: push bundle: %w", err)
	}

	fmt.Printf("phpup build php: built and pushed php-core (spec-hash %s) to %s\n", specHash, opts.Registry)
	return nil
}

// BuildExt is declared but not implemented yet; it lands in Task 5. The
// stub returns a recognisable error so Main's dispatch compiles and
// callers see a clear "not yet" rather than an obscure panic.
func BuildExt(_ context.Context, _ []string) error {
	return errors.New("phpup build ext: not yet supported in this build; will land in a subsequent release")
}

// phpOpts is the parsed flag set for `phpup build php`. Repo is resolved
// to an absolute path during parsing so downstream code (spec-hash, docker
// bind mount) can use it directly without re-resolving.
type phpOpts struct {
	Version  string // "8.4"
	OS       string // "jammy" or "noble" → maps to ubuntu:22.04 / ubuntu:24.04
	Arch     string // "x86_64" or "aarch64" → maps to linux/amd64 / linux/arm64
	TS       string // "nts" or "zts"
	Registry string // "oci-layout:./out/oci-layout" or "ghcr.io/..."
	Repo     string // absolute path to setup-php repo root
}

// parsePHPFlags parses the flag tail for `phpup build php`. The FlagSet
// uses ContinueOnError so callers get back an error instead of a process
// exit — makes the surface testable without os.Exit acrobatics.
func parsePHPFlags(args []string) (*phpOpts, error) {
	fs := flag.NewFlagSet("phpup build php", flag.ContinueOnError)
	version := fs.String("php", "", "PHP version, e.g. 8.4 (required)")
	osFlag := fs.String("os", "jammy", "Ubuntu flavour: jammy (22.04) or noble (24.04)")
	arch := fs.String("arch", "x86_64", "Target arch: x86_64 or aarch64 (amd64/arm64 aliases accepted)")
	ts := fs.String("ts", "nts", "Thread safety: nts (zts not yet supported)")
	registryFlag := fs.String("registry", "oci-layout:./out/oci-layout",
		"Target registry URI (oci-layout:<path> or ghcr.io/<owner>)")
	repo := fs.String("repo", ".", "Path to setup-php repo root")
	if err := fs.Parse(args); err != nil {
		return nil, err
	}
	if *version == "" {
		return nil, errors.New("phpup build php: --php is required")
	}
	// ZTS differs the spec-hash (so a future ZTS builder gets its own
	// cache key), but the current build-php.sh has no --enable-zts
	// conditional — accepting zts here would silently cache an NTS
	// artifact under a ZTS key. Reject until builder support lands.
	if *ts != "nts" {
		return nil, fmt.Errorf("phpup build php: --ts %q not yet supported (only nts)", *ts)
	}
	archNormalized, err := normalizeArch(*arch)
	if err != nil {
		return nil, fmt.Errorf("phpup build php: %w", err)
	}
	absRepo, err := filepath.Abs(*repo)
	if err != nil {
		return nil, fmt.Errorf("phpup build php: resolve repo path: %w", err)
	}
	return &phpOpts{
		Version:  *version,
		OS:       *osFlag,
		Arch:     archNormalized,
		TS:       *ts,
		Registry: *registryFlag,
		Repo:     absRepo,
	}, nil
}

// ubuntuImage maps a short OS flavour name onto the concrete docker image
// tag that builders/linux/build-php.sh expects. Accepts both the short
// ("jammy"/"noble") and long ("ubuntu-22.04"/"ubuntu-24.04") spellings
// because the planner emits the long form and humans tend to type the
// short form; either is unambiguous.
func ubuntuImage(osFlag string) (string, error) {
	switch strings.ToLower(osFlag) {
	case "jammy", "ubuntu-22.04":
		return "ubuntu:22.04", nil
	case "noble", "ubuntu-24.04":
		return "ubuntu:24.04", nil
	default:
		return "", fmt.Errorf("unknown os %q (want jammy|noble)", osFlag)
	}
}

// normalizeArch canonicalizes arch aliases to the forms used in the
// planner/lockfile ("x86_64" / "aarch64"), so spec-hashes are stable
// regardless of whether the caller used the docker-style ("amd64"/"arm64")
// or the uname-style ("x86_64"/"aarch64") spelling.
func normalizeArch(arch string) (string, error) {
	switch arch {
	case "x86_64", "amd64":
		return "x86_64", nil
	case "aarch64", "arm64":
		return "aarch64", nil
	default:
		return "", fmt.Errorf("unknown arch %q (want x86_64 or aarch64)", arch)
	}
}

// dockerPlatform maps the canonical arch name (as produced by
// normalizeArch) onto the docker --platform value.
func dockerPlatform(arch string) (string, error) {
	switch arch {
	case "x86_64":
		return "linux/amd64", nil
	case "aarch64":
		return "linux/arm64", nil
	default:
		return "", fmt.Errorf("unknown arch %q (want x86_64|aarch64)", arch)
	}
}

// parseMetaJSONFile reads the builder's meta.json sidecar into a
// registry.Meta. The builder is the source of truth for schema_version
// (pack-bundle.sh always writes the current SCHEMA_VERSION from
// builders/common/bundle-schema-version.env); we do not fill a default
// here. Callers that need legacy tolerance (e.g. remote Fetch reading an
// older, pre-schema_version bundle from a registry) handle it there.
// Path is constructed internally from os.MkdirTemp; no filepath.Clean
// needed (G304 is excluded project-wide).
func parseMetaJSONFile(path string) (*registry.Meta, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var m registry.Meta
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	return &m, nil
}
