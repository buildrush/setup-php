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
	"time"

	"github.com/buildrush/setup-php/internal/catalog"
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
// the "build" subcommand token (so args[0] is "php", "ext", or "cell").
// Returning a nil error means the requested build (or cache hit)
// succeeded; a non-nil error is safe to pass straight to log.Fatalf.
func Main(args []string) error {
	if len(args) == 0 {
		return errors.New("phpup build: usage: phpup build (php|ext|cell) [flags]")
	}
	ctx := context.Background()
	switch args[0] {
	case "php":
		return BuildPHP(ctx, args[1:])
	case "ext":
		return BuildExt(ctx, args[1:])
	case "cell":
		return BuildCell(ctx, args[1:])
	default:
		return fmt.Errorf("phpup build: unknown kind %q (want php, ext, or cell)", args[0])
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

	// 3. Prepare output mount dir. Default is <repo>/build/php/<slug>/ so
	// artifacts persist next to the source tree for easy inspection;
	// --out-dir overrides verbatim. We wipe before writing so stale files
	// from an aborted previous run don't masquerade as fresh output.
	// Docker requires absolute host paths for bind mounts, so absolutize
	// whatever we end up with. No defer-cleanup — artifacts are meant to
	// persist; the next run's cache-hit path reads from registry.Store.
	outDir := resolveOutDir(opts)
	if err := prepareOutDir(outDir); err != nil {
		return fmt.Errorf("phpup build php: %w", err)
	}
	absOutDir, err := filepath.Abs(outDir)
	if err != nil {
		return fmt.Errorf("phpup build php: resolve out dir: %w", err)
	}

	// 4. Invoke builder in docker.
	image, err := UbuntuImage(opts.OS)
	if err != nil {
		return fmt.Errorf("phpup build php: %w", err)
	}
	platform, err := DockerPlatform(opts.Arch)
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
			{Host: absOutDir, Container: "/tmp", ReadOnly: false},
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
	// internally from resolveOutDir (repo-derived or explicit --out-dir),
	// not arbitrary user input; no filepath.Clean needed (and the linter's
	// G304 is excluded project-wide).
	bundlePath := filepath.Join(absOutDir, "bundle.tar.zst")
	metaPath := filepath.Join(absOutDir, "meta.json")
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

// BuildExt runs the php-ext build end to end. args is everything after
// "build ext" (flags). Returns nil on success, or an error with the
// "phpup build ext: <reason>" prefix the CLI dispatcher expects.
//
// Unlike BuildPHP, the ext build needs a prerequisite php-core bundle —
// the extension is dynamically linked against a specific PHP. Per spec,
// builders/** stays unchanged, so instead of modifying fetch-core.sh we
// spin up an ephemeral distribution:3 sidecar registry, copy the
// prerequisite php-core@sha256 into it, and override REGISTRY for the
// build container so fetch-core.sh pulls from the sidecar transparently.
func BuildExt(ctx context.Context, args []string) error {
	opts, err := parseExtFlags(args)
	if err != nil {
		return err
	}

	// 1. Spec-hash.
	specHash, err := ComputeSpecHash(&SpecHashInputs{
		Kind:    "ext",
		Name:    opts.Name,
		Version: opts.Version,
		OS:      "linux",
		Arch:    opts.Arch,
		PHPABI:  opts.PHPABI,
		TS:      tsFromPHPABI(opts.PHPABI),
		Repo:    opts.Repo,
	})
	if err != nil {
		return fmt.Errorf("phpup build ext: %w", err)
	}

	// 2. Open target store + cache-probe.
	store, err := registry.Open(opts.Registry)
	if err != nil {
		return fmt.Errorf("phpup build ext: open registry: %w", err)
	}
	bundleName := "php-ext-" + opts.Name
	// Remote backends return ErrUnsupported for LookupBySpec; treat that
	// as a soft miss so callers without an oci-layout cache fall through
	// to building. Hard errors from a layout backend still propagate.
	ref, hit, err := store.LookupBySpec(ctx, bundleName, specHash)
	if errors.Is(err, registry.ErrUnsupported) {
		hit, err = false, nil
	}
	if err != nil {
		return fmt.Errorf("phpup build ext: lookup by spec: %w", err)
	}
	if hit {
		fmt.Printf("phpup build ext: cache hit %s (spec-hash %s)\n", ref.Digest, specHash)
		return nil
	}

	// 3. Start sidecar + seed prerequisite core. The sidecar is tied to
	// this invocation's lifetime; teardown is unconditional (including
	// on error paths below) via defer.
	lifecycle := currentSidecarLifecycle()
	sc, stopSidecar, err := lifecycle.Start(ctx)
	if err != nil {
		return fmt.Errorf("phpup build ext: start sidecar: %w", err)
	}
	defer func() {
		// Fresh context with a generous timeout so teardown still runs
		// if the caller's ctx has already been cancelled (which is the
		// common case when the build itself failed). 30s is
		// comfortably more than `docker rm -f` + `docker network rm`
		// need under load.
		stopCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_ = stopSidecar(stopCtx)
	}()

	// Core tag matches what fetch-core.sh constructs inside the build
	// container: "<php-ver>-<os>-<arch>-<ts>". This MUST line up or the
	// oras pull will miss the seeded image.
	coreTag := coreTagForFetch(opts.PHPABI, opts.Arch)
	coreRef := registry.Ref{Name: "php-core", Digest: opts.CoreDigest}
	if err := lifecycle.SeedCore(ctx, sc, store, coreRef, "buildrush", coreTag); err != nil {
		return fmt.Errorf("phpup build ext: seed core: %w", err)
	}

	// 4. Prepare output dir (default: <repo>/build/ext/<slug>/ ;
	// --out-dir overrides verbatim).
	outDir := resolveExtOutDir(opts)
	if err := prepareOutDir(outDir); err != nil {
		return fmt.Errorf("phpup build ext: %w", err)
	}
	absOutDir, err := filepath.Abs(outDir)
	if err != nil {
		return fmt.Errorf("phpup build ext: resolve out dir: %w", err)
	}

	// 5. Load build_deps.linux from catalog (build-ext.sh reads
	// BUILD_DEPS env). Absent build_deps = empty = no-op in the
	// builder script.
	buildDeps, err := loadExtBuildDeps(filepath.Join(opts.Repo, "catalog", "extensions", opts.Name+".yaml"))
	if err != nil {
		return fmt.Errorf("phpup build ext: load build_deps: %w", err)
	}

	// 6. Run build container on the sidecar's network so
	// fetch-core.sh can reach the sidecar by in-network hostname.
	image, err := UbuntuImage(opts.OS)
	if err != nil {
		return fmt.Errorf("phpup build ext: %w", err)
	}
	platform, err := DockerPlatform(opts.Arch)
	if err != nil {
		return fmt.Errorf("phpup build ext: %w", err)
	}
	runOpts := &DockerRunOpts{
		Image:    image,
		Platform: platform,
		Network:  sc.Network,
		Mounts: []Mount{
			{Host: opts.Repo, Container: "/workspace", ReadOnly: true},
			// Mount at /tmp (not /tmp/out) because builders/linux/build-ext.sh
			// calls pack-bundle.sh with the hardcoded output path
			// /tmp/bundle.tar.zst — mirroring the php-core build. See
			// BuildPHP for the full rationale.
			{Host: absOutDir, Container: "/tmp", ReadOnly: false},
		},
		Env: map[string]string{
			"EXT_NAME":    opts.Name,
			"EXT_VERSION": opts.Version,
			"PHP_ABI":     opts.PHPABI,
			"ARCH":        opts.Arch,
			"WORKSPACE":   "/workspace",
			"OUTPUT_DIR":  "/tmp/ext-out",
			// REGISTRY override: fetch-core.sh defaults to
			// ghcr.io/buildrush but honours REGISTRY if set. We point
			// it at the sidecar so the builder pulls the seeded
			// php-core without any script change.
			"REGISTRY":   sc.InNetworkHost + "/buildrush",
			"BUILD_DEPS": buildDeps,
		},
		Cmd: []string{"bash", "-c", linuxAptPreamble + "/workspace/builders/linux/build-ext.sh"},
	}
	if err := DockerRun(ctx, runOpts); err != nil {
		return fmt.Errorf("phpup build ext: docker: %w", err)
	}

	// 7. Read the bundle + meta from the mount dir.
	bundlePath := filepath.Join(absOutDir, "bundle.tar.zst")
	metaPath := filepath.Join(absOutDir, "meta.json")
	bundle, err := os.Open(bundlePath)
	if err != nil {
		return fmt.Errorf("phpup build ext: open bundle: %w", err)
	}
	defer func() { _ = bundle.Close() }()
	meta, err := parseMetaJSONFile(metaPath)
	if err != nil {
		return fmt.Errorf("phpup build ext: parse meta.json: %w", err)
	}

	// 8. Push to the store.
	pushRef := registry.Ref{Name: bundleName}
	ann := registry.Annotations{BundleName: bundleName, SpecHash: specHash}
	if err := store.Push(ctx, pushRef, bundle, meta, ann); err != nil {
		return fmt.Errorf("phpup build ext: push bundle: %w", err)
	}

	fmt.Printf("phpup build ext: built and pushed %s (spec-hash %s) to %s\n", bundleName, specHash, opts.Registry)
	return nil
}

// BuildCell is the entry point for `phpup build cell …`. It is a
// one-shot convenience that wraps BuildPHP + a loop of BuildExt over
// every catalog extension compatible with the cell's PHP ABI, so a
// single invocation produces a populated oci-layout for one
// (php, os, arch, ts) tuple. Intended for the local-CI pipeline:
//
//	phpup build cell --php 8.4 --os jammy --arch x86_64
//
// Produces (approximately, subject to the catalog's abi_matrix):
//
//	oci-layout:./out/oci-layout
//	  ├── php-core (spec-hash for php 8.4/jammy/x86_64/nts)
//	  ├── php-ext-redis
//	  ├── php-ext-xdebug
//	  └── …
//
// On cache hit in the target registry, per-step BuildPHP/BuildExt
// invocations short-circuit without touching docker — so repeat runs of
// `phpup build cell` against a warm layout are cheap. The function
// returns the first error encountered (no partial-success graph walk);
// callers that want per-step resilience should invoke phpup build php
// and phpup build ext directly from a retry wrapper.
func BuildCell(ctx context.Context, args []string) error {
	opts, err := parseCellFlags(args)
	if err != nil {
		return err
	}

	// Step 1: discover extensions for the cell up front so progress
	// banners can show "[1/N] build php" with an accurate N instead of
	// a placeholder. Uses the same abi_matrix + exclude gate as the
	// planner so the local build graph is byte-identical to what CI
	// would have enqueued for this cell.
	exts, err := discoverExtensionsForCell(opts.Repo, opts.Version, "linux", opts.Arch, opts.TS)
	if err != nil {
		return fmt.Errorf("phpup build cell: discover extensions: %w", err)
	}
	total := len(exts) + 1 // +1 for the prerequisite php-core step
	fmt.Printf("phpup build cell: %d extensions compatible with PHP %s linux/%s\n", len(exts), opts.Version, opts.Arch)

	// Step 2: build php-core. Same flags the user would have passed to
	// `phpup build php` directly, forwarded verbatim (no shell-level
	// string concatenation — each flag is a single argv element so
	// spaces in repo paths never trip the downstream FlagSet).
	phpArgs := []string{
		"--php", opts.Version,
		"--os", opts.OS,
		"--arch", opts.Arch,
		"--ts", opts.TS,
		"--registry", opts.Registry,
		"--repo", opts.Repo,
	}
	if opts.OutDir != "" {
		phpArgs = append(phpArgs, "--out-dir", opts.OutDir)
	}
	fmt.Printf("phpup build cell: [1/%d] build php %s\n", total, opts.Version)
	if err := BuildPHP(ctx, phpArgs); err != nil {
		return fmt.Errorf("phpup build cell: build php: %w", err)
	}

	// Step 3: resolve the just-built php-core digest for the ext builds.
	// LookupBySpec is the only affordance the Store interface offers for
	// "find the bundle I just pushed"; a remote backend returns
	// ErrUnsupported here, which cells are not expected to target (the
	// primary use case is a local oci-layout).
	specHash, err := ComputeSpecHash(&SpecHashInputs{
		Kind: "php", Version: opts.Version,
		OS: "linux", Arch: opts.Arch, TS: opts.TS,
		Repo: opts.Repo,
	})
	if err != nil {
		return fmt.Errorf("phpup build cell: recompute core spec-hash: %w", err)
	}
	store, err := registry.Open(opts.Registry)
	if err != nil {
		return fmt.Errorf("phpup build cell: open registry: %w", err)
	}
	coreRef, hit, err := store.LookupBySpec(ctx, "php-core", specHash)
	if err != nil {
		return fmt.Errorf("phpup build cell: lookup core: %w", err)
	}
	if !hit || coreRef.Digest == "" {
		return fmt.Errorf("phpup build cell: php-core not in layout after build (expected spec-hash=%s)", specHash)
	}

	// Step 4: build each extension.
	phpAbi := opts.Version + "-" + opts.TS
	for i, ext := range exts {
		extArgs := []string{
			"--ext", ext.Name,
			"--ext-version", ext.Version,
			"--php-abi", phpAbi,
			"--os", opts.OS,
			"--arch", opts.Arch,
			"--php-core-digest", coreRef.Digest,
			"--registry", opts.Registry,
			"--repo", opts.Repo,
		}
		if opts.OutDir != "" {
			// When --out-dir was passed at cell level, slot each
			// extension into its own subdir so the php-core artifacts
			// and per-extension artifacts don't fight over the same
			// /tmp mount.
			extArgs = append(extArgs, "--out-dir", filepath.Join(opts.OutDir, "ext", ext.Name+"-"+ext.Version))
		}
		fmt.Printf("phpup build cell: [%d/%d] build ext %s %s\n", i+2, total, ext.Name, ext.Version)
		if err := BuildExt(ctx, extArgs); err != nil {
			return fmt.Errorf("phpup build cell: build ext %s: %w", ext.Name, err)
		}
	}

	fmt.Printf("phpup build cell: completed php-core + %d extensions for %s/%s PHP %s\n", len(exts), opts.OS, opts.Arch, opts.Version)
	return nil
}

// cellOpts is the parsed flag set for `phpup build cell`. Shape
// deliberately mirrors phpOpts so resolveOutDir-style derivation is
// straightforward — Repo is resolved to an absolute path during parsing
// so downstream code can use it directly.
type cellOpts struct {
	Version  string // "8.4"
	OS       string // "jammy" or "noble"
	Arch     string // "x86_64" or "aarch64" (after normalisation)
	TS       string // "nts" — zts not yet supported
	Registry string // "oci-layout:./out/oci-layout" or "ghcr.io/..."
	Repo     string // absolute path to setup-php repo root
	OutDir   string // --out-dir override; empty = let BuildPHP/BuildExt derive
}

// parseCellFlags parses the flag tail for `phpup build cell`. The shape
// matches phpOpts + a cell-specific --out-dir meaning: when set, each
// sub-build lands in a subdir of the supplied directory (see BuildCell).
// ZTS is rejected for the same reason as parsePHPFlags — accepting it
// would silently cache an NTS artifact under a ZTS key.
func parseCellFlags(args []string) (*cellOpts, error) {
	fs := flag.NewFlagSet("phpup build cell", flag.ContinueOnError)
	version := fs.String("php", "", "PHP version, e.g. 8.4 (required)")
	osFlag := fs.String("os", "jammy", "Ubuntu flavour: jammy or noble")
	arch := fs.String("arch", "x86_64", "Target arch: x86_64 or aarch64 (amd64/arm64 aliases accepted)")
	ts := fs.String("ts", "nts", "Thread safety: nts (zts not yet supported)")
	registryFlag := fs.String("registry", "oci-layout:./out/oci-layout",
		"Target registry URI (oci-layout:<path> or ghcr.io/<owner>)")
	repo := fs.String("repo", ".", "Path to setup-php repo root")
	outDir := fs.String("out-dir", "",
		"Shared docker output directory root (defaults to <repo>/build/ derivation for each build subcommand)")
	if err := fs.Parse(args); err != nil {
		return nil, err
	}
	if *version == "" {
		return nil, errors.New("phpup build cell: --php is required")
	}
	if *ts != "nts" {
		return nil, fmt.Errorf("phpup build cell: --ts %q not yet supported (only nts)", *ts)
	}
	archNormalized, err := normalizeArch(*arch)
	if err != nil {
		return nil, fmt.Errorf("phpup build cell: %w", err)
	}
	absRepo, err := filepath.Abs(*repo)
	if err != nil {
		return nil, fmt.Errorf("phpup build cell: resolve repo path: %w", err)
	}
	return &cellOpts{
		Version:  *version,
		OS:       *osFlag,
		Arch:     archNormalized,
		TS:       *ts,
		Registry: *registryFlag,
		Repo:     absRepo,
		OutDir:   *outDir,
	}, nil
}

// extOpts is the parsed flag set for `phpup build ext`. Repo is
// resolved to an absolute path during parsing so downstream code
// (spec-hash, docker bind mount) can use it directly. CoreDigest
// addresses the prerequisite php-core bundle that gets seeded into
// the sidecar — the caller is responsible for resolving it (typically
// via the lockfile or a prior BuildPHP invocation).
type extOpts struct {
	Name       string // "redis"
	Version    string // "6.2.0"
	PHPABI     string // "8.4-nts"
	OS         string // "jammy" or "noble" (after normalisation)
	Arch       string // "x86_64" or "aarch64" (after normalisation)
	Registry   string
	Repo       string
	OutDir     string
	CoreDigest string // "sha256:..." — required; resolved by caller
}

// parseExtFlags parses the flag tail for `phpup build ext`. The FlagSet
// uses ContinueOnError so callers get back an error instead of a process
// exit — makes the surface testable without os.Exit acrobatics.
func parseExtFlags(args []string) (*extOpts, error) {
	fs := flag.NewFlagSet("phpup build ext", flag.ContinueOnError)
	extName := fs.String("ext", "", "Extension name, e.g. redis (required)")
	extVer := fs.String("ext-version", "", "Extension version (required)")
	phpAbi := fs.String("php-abi", "", "PHP ABI, e.g. 8.4-nts (required)")
	osFlag := fs.String("os", "jammy", "Ubuntu flavour: jammy (22.04) or noble (24.04)")
	arch := fs.String("arch", "x86_64", "Target arch: x86_64 or aarch64 (amd64/arm64 aliases accepted)")
	registryFlag := fs.String("registry", "oci-layout:./out/oci-layout",
		"Target registry URI (oci-layout:<path> or ghcr.io/<owner>)")
	repo := fs.String("repo", ".", "Path to setup-php repo root")
	outDir := fs.String("out-dir", "",
		"Docker output directory (defaults to <repo>/build/ext/<name>-<version>-<php_abi>-<os>-<arch>/)")
	coreDigest := fs.String("php-core-digest", "",
		"Digest of the prerequisite php-core bundle (sha256:...). Required.")
	if err := fs.Parse(args); err != nil {
		return nil, err
	}
	if *extName == "" {
		return nil, errors.New("phpup build ext: --ext is required")
	}
	if *extVer == "" {
		return nil, errors.New("phpup build ext: --ext-version is required")
	}
	if *phpAbi == "" {
		return nil, errors.New("phpup build ext: --php-abi is required")
	}
	if *coreDigest == "" {
		return nil, errors.New("phpup build ext: --php-core-digest is required")
	}

	archNormalized, err := normalizeArch(*arch)
	if err != nil {
		return nil, fmt.Errorf("phpup build ext: %w", err)
	}
	absRepo, err := filepath.Abs(*repo)
	if err != nil {
		return nil, fmt.Errorf("phpup build ext: resolve repo path: %w", err)
	}

	return &extOpts{
		Name:       *extName,
		Version:    *extVer,
		PHPABI:     *phpAbi,
		OS:         *osFlag,
		Arch:       archNormalized,
		Registry:   *registryFlag,
		Repo:       absRepo,
		OutDir:     *outDir,
		CoreDigest: *coreDigest,
	}, nil
}

// resolveExtOutDir derives the project-relative build output directory
// from the input tuple when the caller didn't pass --out-dir
// explicitly. Mirrors Task 4's resolveOutDir shape but slots under
// <repo>/build/ext/ instead of <repo>/build/php/:
//
//	<repo>/build/ext/<name>-<version>-<php_abi>-<os>-<arch>/
//
// Keying on the same tuple as the spec-hash makes the path
// deterministic: repeated runs with the same inputs overwrite the
// same dir instead of piling up random tempdirs.
func resolveExtOutDir(opts *extOpts) string {
	if opts.OutDir != "" {
		return opts.OutDir
	}
	slug := opts.Name + "-" + opts.Version + "-" + opts.PHPABI + "-" + opts.OS + "-" + opts.Arch
	return filepath.Join(opts.Repo, "build", "ext", slug)
}

// coreTagForFetch assembles the OCI tag fetch-core.sh constructs for
// the prerequisite php-core. The format — "<php_ver>-<os>-<arch>-<ts>" —
// is baked into builders/common/fetch-core.sh and must match exactly
// or the sidecar pull inside the build container will miss. Example:
// PHPABI="8.4-nts", arch="x86_64" → "8.4-linux-x86_64-nts".
func coreTagForFetch(phpABI, arch string) string {
	ts := tsFromPHPABI(phpABI)
	ver := strings.TrimSuffix(phpABI, "-"+ts)
	return ver + "-linux-" + arch + "-" + ts
}

// tsFromPHPABI extracts the thread-safety suffix from a PHPABI string
// of the form "<version>-<ts>". Returns "nts" as a safe default if the
// input is malformed — the spec-hash then diverges from a correct
// invocation, guaranteeing a cache miss rather than silent cross-use.
func tsFromPHPABI(phpABI string) string {
	i := strings.LastIndex(phpABI, "-")
	if i < 0 {
		return "nts"
	}
	return phpABI[i+1:]
}

// loadExtBuildDeps reads catalog/extensions/<name>.yaml and returns the
// .build_deps.linux list joined by single spaces (the BUILD_DEPS env
// shape build-ext.sh expects). Mirrors the yq invocation in
// build-extension.yml:
//
//	yq eval '.build_deps.linux // [] | join(" ")' catalog/extensions/<name>.yaml
//
// Absent or empty returns "" — the builder treats that as a no-op.
// Uses the typed catalog API so the extension-schema shape lives in one
// place (internal/catalog) instead of drifting across ad-hoc parsers.
func loadExtBuildDeps(path string) (string, error) {
	spec, err := catalog.LoadExtensionSpec(path)
	if err != nil {
		return "", fmt.Errorf("load extension catalog: %w", err)
	}
	return strings.Join(spec.BuildDeps["linux"], " "), nil
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
	// OutDir is the docker output mount path. Empty = derive the default
	// under <repo>/build/php/<version>-<os>-<arch>-<ts>/; non-empty =
	// use verbatim (may be absolute or relative — absolutized in BuildPHP).
	OutDir string
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
	outDir := fs.String("out-dir", "",
		"Docker output directory (defaults to <repo>/build/php/<version>-<os>-<arch>-<ts>/)")
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
		OutDir:   *outDir,
	}, nil
}

// resolveOutDir derives the project-relative build output directory from
// the input tuple when the caller didn't pass --out-dir explicitly. The
// directory lives under <repo>/build/ so it's project-local (gitignored
// via the repo's .gitignore). Path shape:
//
//	<repo>/build/php/<version>-<os>-<arch>-<ts>/
//
// Keying on the same tuple as the spec-hash makes the path deterministic:
// repeated runs with the same inputs overwrite the same dir instead of
// piling up random tempdirs. Task 5 (BuildExt) will mirror this pattern
// under <repo>/build/ext/<name>-<version>-<php_abi>-<os>-<arch>/.
func resolveOutDir(opts *phpOpts) string {
	if opts.OutDir != "" {
		return opts.OutDir
	}
	slug := opts.Version + "-" + opts.OS + "-" + opts.Arch + "-" + opts.TS
	return filepath.Join(opts.Repo, "build", "php", slug)
}

// prepareOutDir wipes any previous content at path (so stale files from
// an aborted earlier run don't masquerade as fresh output) and creates
// a clean directory with mode 1777. The 1777 mode matches traditional
// /tmp semantics — necessary because this directory is bind-mounted
// into the build container at /tmp, and container-internal unprivileged
// users (notably apt-key, which drops to _apt) need write access.
// Without this, apt-key cannot create its temporary config files and
// `apt-get update` fails with "Couldn't create temporary file" on
// hosts where Docker doesn't transparently map uids (e.g., GitHub
// Actions linux runners, non-Docker-Desktop setups).
func prepareOutDir(path string) error {
	if err := os.RemoveAll(path); err != nil {
		return fmt.Errorf("clean out dir: %w", err)
	}
	// MkdirAll at a conservative 0o750 first (keeps gosec G301 quiet for
	// the creation step), then Chmod up to 1777 below. MkdirAll also
	// respects umask and never sets sticky/setuid/setgid bits, so a
	// separate Chmod is required regardless.
	if err := os.MkdirAll(path, 0o750); err != nil {
		return fmt.Errorf("create out dir: %w", err)
	}
	// 1777 is a genuine requirement of the docker bind-mount use case
	// (see comment above); gosec G301 flags world-writable as suspicious
	// but the /tmp-style bind mount is exactly the intended use — this
	// directory IS a /tmp replacement for a container.
	//nolint:gosec // G301: world-writable is required for /tmp-style docker bind mount; see func comment.
	if err := os.Chmod(path, 0o1777); err != nil {
		return fmt.Errorf("chmod out dir to 1777: %w", err)
	}
	return nil
}

// UbuntuImage maps a short OS flavour name onto the concrete docker image
// tag that builders/linux/build-php.sh expects. Accepts both the short
// ("jammy"/"noble") and long ("ubuntu-22.04"/"ubuntu-24.04") spellings
// because the planner emits the long form and humans tend to type the
// short form; either is unambiguous. Exported so the testsuite package
// can reuse the same OS-to-image mapping without duplicating it.
func UbuntuImage(osFlag string) (string, error) {
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

// DockerPlatform maps the canonical arch name (as produced by
// normalizeArch) onto the docker --platform value. Exported so the
// testsuite package can reuse the same arch-to-platform mapping without
// duplicating it.
func DockerPlatform(arch string) (string, error) {
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
