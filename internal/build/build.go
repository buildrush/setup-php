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
			{Host: outDir, Container: "/tmp/out", ReadOnly: false},
		},
		Env: map[string]string{
			"PHP_VERSION": opts.Version,
			"ARCH":        opts.Arch,
			"OUTPUT_DIR":  "/tmp/out",
			"WORKSPACE":   "/workspace",
		},
		Cmd: []string{"bash", "-c",
			"apt-get update >/dev/null 2>&1 && " +
				"apt-get install -y --no-install-recommends curl xz-utils ca-certificates >/dev/null 2>&1 && " +
				"/workspace/builders/linux/build-php.sh"},
	}
	if err := DockerRun(ctx, runOpts); err != nil {
		return fmt.Errorf("phpup build php: docker: %w", err)
	}

	// 5. Read the bundle + meta from the mount dir.
	bundlePath := filepath.Join(outDir, "bundle.tar.zst")
	metaPath := filepath.Join(outDir, "meta.json")
	bundle, err := os.Open(filepath.Clean(bundlePath))
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
	return errors.New("phpup build ext: implemented in PR2 Task 5")
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
	arch := fs.String("arch", "x86_64", "Target arch: x86_64 or aarch64")
	ts := fs.String("ts", "nts", "Thread safety: nts or zts")
	registryFlag := fs.String("registry", "oci-layout:./out/oci-layout",
		"Target registry URI (oci-layout:<path> or ghcr.io/<owner>)")
	repo := fs.String("repo", ".", "Path to setup-php repo root")
	if err := fs.Parse(args); err != nil {
		return nil, err
	}
	if *version == "" {
		return nil, errors.New("phpup build php: --php is required")
	}
	absRepo, err := filepath.Abs(*repo)
	if err != nil {
		return nil, fmt.Errorf("phpup build php: resolve repo path: %w", err)
	}
	return &phpOpts{
		Version:  *version,
		OS:       *osFlag,
		Arch:     *arch,
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

// dockerPlatform maps the caller-facing arch name onto the docker
// --platform value. The caller-facing names match the builder script's
// ARCH env contract ("x86_64"/"aarch64"); the docker aliases ("amd64"/
// "arm64") are accepted too for ergonomics.
func dockerPlatform(arch string) (string, error) {
	switch arch {
	case "x86_64", "amd64":
		return "linux/amd64", nil
	case "aarch64", "arm64":
		return "linux/arm64", nil
	default:
		return "", fmt.Errorf("unknown arch %q (want x86_64|aarch64)", arch)
	}
}

// parseMetaJSONFile reads the builder's meta.json sidecar into a
// registry.Meta. Missing schema_version defaults to 1 to match
// internal/registry/layout.go's legacy-bundle tolerance; callers should
// not rely on this default — the builder writes the real version.
func parseMetaJSONFile(path string) (*registry.Meta, error) {
	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return nil, err
	}
	var m registry.Meta
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	if m.SchemaVersion == 0 {
		m.SchemaVersion = 1
	}
	return &m, nil
}
