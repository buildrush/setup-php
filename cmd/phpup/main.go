package main

import (
	"context"
	_ "embed"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"sort"
	"strings"

	"github.com/buildrush/setup-php/internal/build"
	"github.com/buildrush/setup-php/internal/cache"
	"github.com/buildrush/setup-php/internal/catalog"
	"github.com/buildrush/setup-php/internal/compat"
	"github.com/buildrush/setup-php/internal/compose"
	"github.com/buildrush/setup-php/internal/env"
	"github.com/buildrush/setup-php/internal/extract"
	"github.com/buildrush/setup-php/internal/lockfile"
	"github.com/buildrush/setup-php/internal/oci"
	"github.com/buildrush/setup-php/internal/plan"
	"github.com/buildrush/setup-php/internal/resolve"
	"github.com/buildrush/setup-php/internal/testsuite"
	"github.com/buildrush/setup-php/internal/version"
)

//go:embed bundles.lock
var embeddedLockfile []byte

// resolveRegistry picks the registry URI with precedence:
//  1. --registry flag (cliFlag, passed in)
//  2. INPUT_REGISTRY env var (GitHub Actions input convention)
//  3. PHPUP_REGISTRY env var (local/CLI convention)
//  4. default "ghcr.io/buildrush"
func resolveRegistry(cliFlag string) string {
	if cliFlag != "" {
		return cliFlag
	}
	if v := os.Getenv("INPUT_REGISTRY"); v != "" {
		return v
	}
	if v := os.Getenv("PHPUP_REGISTRY"); v != "" {
		return v
	}
	return "ghcr.io/buildrush"
}

func main() {
	if len(os.Args) > 1 && os.Args[1] == "--version" {
		fmt.Printf("phpup %s (%s) built %s\n", version.Version, version.Commit, version.BuildDate)
		return
	}

	// `phpup build …` is dispatched before the setup-flow flag.Parse so the
	// two argv universes never collide. build.Main uses its own FlagSet.
	if len(os.Args) > 1 && os.Args[1] == "build" {
		if err := build.Main(os.Args[2:]); err != nil {
			log.Fatalf("%v", err)
		}
		return
	}

	// `phpup test …` is dispatched the same way as `phpup build …`: its
	// own FlagSet, its own argv universe. Used by maintainers to run the
	// compat-harness fixtures locally against a registry (oci-layout or
	// remote). See internal/testsuite.Main for the flag surface.
	if len(os.Args) > 1 && os.Args[1] == "test" {
		if err := testsuite.Main(os.Args[2:]); err != nil {
			log.Fatalf("%v", err)
		}
		return
	}

	// `phpup internal <subcmd> …` is the maintainer-only umbrella for
	// commands that are driven by the outer tooling rather than by end
	// users. Currently only "test-cell" (the inner, container-side part
	// of `phpup test`). Kept separate so the user-facing help surface
	// stays lean.
	if len(os.Args) > 1 && os.Args[1] == "internal" {
		if err := testsuite.InternalMain(os.Args[2:]); err != nil {
			log.Fatalf("%v", err)
		}
		return
	}

	registryFlag := flag.String("registry", "",
		"OCI artifact store (e.g., ghcr.io/buildrush or oci-layout:./out/oci-layout). Overrides INPUT_REGISTRY / PHPUP_REGISTRY; defaults to ghcr.io/buildrush.")
	flag.Parse()

	ctx := context.Background()

	// 1. Parse inputs
	p, err := plan.FromEnv()
	if err != nil {
		log.Fatalf("parse inputs: %v", err)
	}
	p.ApplyCoverage()

	// Emit warnings for inputs that cannot be implemented given our architecture.
	if p.Update {
		fmt.Fprintln(os.Stderr, compat.UnimplementedInputWarning("update", "true"))
	}
	if p.Verbose {
		log.Printf("Plan: PHP %s, extensions=%v, os=%s, arch=%s, ts=%s, coverage=%s",
			p.PHPVersion, p.Extensions, p.OS, p.Arch, p.ThreadSafety, p.Coverage)
	}

	// 2. Load embedded lockfile
	lf, err := lockfile.Parse(embeddedLockfile)
	if err != nil {
		log.Fatalf("parse embedded lockfile: %v", err)
	}

	// 3. Build minimal catalog for resolution. The runtime keys the Versions
	// map by the exact PHPVersion that resolve.Resolve will look up, so the
	// per-version bundled list is always found for IsBundled. The list is
	// the UNION of v2's baseline (compat.BundledExtensions) and our bundle's
	// extras (compat.OurBuildBundledExtras) — the latter reflects configure
	// flags our current 8.4 core has beyond v2's slim set.
	bundled := append(compat.BundledExtensions(p.PHPVersion), compat.OurBuildBundledExtras(p.PHPVersion)...)
	cat := &catalog.Catalog{
		PHP: &catalog.PHPSpec{
			Versions: map[string]*catalog.PHPVersionSpec{
				p.PHPVersion: {BundledExtensions: bundled},
			},
		},
		Extensions: map[string]*catalog.ExtensionSpec{
			"redis":     {Name: "redis", Kind: catalog.ExtensionKindPECL, Versions: []string{"6.2.0"}},
			"xdebug":    {Name: "xdebug", Kind: catalog.ExtensionKindPECL, Versions: []string{"3.5.1"}, Ini: []string{"zend_extension=xdebug"}},
			"pcov":      {Name: "pcov", Kind: catalog.ExtensionKindPECL, Versions: []string{"1.0.12"}, Ini: []string{"extension=pcov"}},
			"apcu":      {Name: "apcu", Kind: catalog.ExtensionKindPECL, Versions: []string{"5.1.28"}},
			"igbinary":  {Name: "igbinary", Kind: catalog.ExtensionKindPECL, Versions: []string{"3.2.16"}},
			"msgpack":   {Name: "msgpack", Kind: catalog.ExtensionKindPECL, Versions: []string{"3.0.0"}},
			"uuid":      {Name: "uuid", Kind: catalog.ExtensionKindPECL, Versions: []string{"1.3.0"}, RuntimeDeps: map[string][]string{"linux": {"libuuid1"}}},
			"ssh2":      {Name: "ssh2", Kind: catalog.ExtensionKindPECL, Versions: []string{"1.5.0"}, RuntimeDeps: map[string][]string{"linux": {"libssh2-1"}}},
			"yaml":      {Name: "yaml", Kind: catalog.ExtensionKindPECL, Versions: []string{"2.3.0"}, RuntimeDeps: map[string][]string{"linux": {"libyaml-0-2"}}},
			"memcached": {Name: "memcached", Kind: catalog.ExtensionKindPECL, Versions: []string{"3.4.0"}, RuntimeDeps: map[string][]string{"linux": {"libmemcached11", "libsasl2-2"}}},
			"amqp":      {Name: "amqp", Kind: catalog.ExtensionKindPECL, Versions: []string{"2.2.0"}, RuntimeDeps: map[string][]string{"linux": {"librabbitmq4"}}},
			"event":     {Name: "event", Kind: catalog.ExtensionKindPECL, Versions: []string{"3.1.4"}, RuntimeDeps: map[string][]string{"linux": {"libevent-2.1-7", "libevent-openssl-2.1-7"}}},
			"rdkafka":   {Name: "rdkafka", Kind: catalog.ExtensionKindPECL, Versions: []string{"6.0.5"}, RuntimeDeps: map[string][]string{"linux": {"librdkafka1"}}},
			"protobuf":  {Name: "protobuf", Kind: catalog.ExtensionKindPECL, Versions: []string{"5.34.1"}},
			"imagick":   {Name: "imagick", Kind: catalog.ExtensionKindPECL, Versions: []string{"3.8.1"}, RuntimeDeps: map[string][]string{"linux": {"libfontconfig1", "libx11-6", "libxext6", "liblcms2-2", "liblqr-1-0", "libfftw3-double3", "libbz2-1.0"}}},
			"mongodb":   {Name: "mongodb", Kind: catalog.ExtensionKindPECL, Versions: []string{"2.2.1"}, RuntimeDeps: map[string][]string{"linux": {"libssl3", "libsasl2-2"}}},
			"swoole":    {Name: "swoole", Kind: catalog.ExtensionKindPECL, Versions: []string{"6.2.0"}, RuntimeDeps: map[string][]string{"linux": {"libssl3", "libcurl4"}}},
			"grpc":      {Name: "grpc", Kind: catalog.ExtensionKindPECL, Versions: []string{"1.80.0"}, RuntimeDeps: map[string][]string{"linux": {"libssl3"}}},
		},
	}

	// 4. Resolve plan against lockfile
	res, err := resolve.Resolve(p, lf, cat)
	if err != nil {
		log.Fatalf("resolve: %v", err)
	}

	for _, w := range res.Warnings {
		log.Printf("WARNING: %s", w)
	}

	if p.Verbose {
		log.Printf("Resolved: core=%s, %d extensions", res.PHPCore.Digest, len(res.Extensions))
	}

	// 4b. Install runtime_deps for PECL bundles (v2 parity on Linux).
	if err := installRuntimeDeps(runtime.GOOS, res.Extensions, cat, aptInstaller); err != nil {
		log.Fatalf("install runtime deps: %v", err)
	}

	// 5. Check cache
	store, _ := cache.NewStore()
	planHash := p.Hash()
	hit := store.Check(planHash)

	if hit.Hit {
		if p.Verbose {
			log.Printf("Cache hit at %s", hit.Path)
		}
		layout := layoutFromDir(hit.Path)
		resolved := resolvePHPVersion(layout, p.PHPVersion)
		if err := exportEnv(layout, resolved); err != nil {
			log.Fatalf("export env: %v", err)
		}
		fmt.Printf("PHP %s ready (cached)\n", resolved)
		return
	}

	// 6. Fetch from GHCR
	token := os.Getenv("INPUT_GITHUB-TOKEN")
	if token == "" {
		token = os.Getenv("GITHUB_TOKEN")
	}
	registry := resolveRegistry(*registryFlag)
	client, err := oci.NewClient(registry, token)
	if err != nil {
		log.Fatalf("create OCI client: %v", err)
	}

	var bundles []oci.ResolvedBundle
	bundles = append(bundles, oci.ResolvedBundle{
		Key: res.PHPCore.Key, Digest: res.PHPCore.Digest,
		Name: res.PHPCore.Name, Version: res.PHPCore.Version, Kind: res.PHPCore.Kind,
	})
	for _, ext := range res.Extensions {
		bundles = append(bundles, oci.ResolvedBundle{
			Key: ext.Key, Digest: ext.Digest,
			Name: ext.Name, Version: ext.Version, Kind: ext.Kind,
		})
	}

	if p.Verbose {
		log.Printf("Fetching %d bundles from %s", len(bundles), registry)
	}

	results, err := client.FetchAll(ctx, bundles)
	if err != nil {
		log.Fatalf("fetch bundles: %v", err)
	}

	// Assert each bundle's sidecar schema_version meets the runtime's
	// per-kind minimum. Fails fast with a clear diagnostic before any
	// extract/compose work starts.
	for i := range results {
		kind := results[i].Metadata.Kind
		actual := results[i].Metadata.SchemaVersion
		if err := compose.AssertBundleSchema(kind, actual); err != nil {
			log.Fatalf("bundle %s: %v", results[i].Key, err)
		}
	}

	// 7. Extract
	baseDir := "/opt/buildrush"
	if dir := os.Getenv("BUILDRUSH_DIR"); dir != "" {
		baseDir = dir
	}
	coreDir := filepath.Join(baseDir, "core")
	bundlesDir := filepath.Join(baseDir, "bundles")
	if err := os.MkdirAll(coreDir, 0o750); err != nil {
		log.Fatalf("create core dir: %v", err)
	}
	if err := os.MkdirAll(bundlesDir, 0o750); err != nil {
		log.Fatalf("create bundles dir: %v", err)
	}

	var items []extract.ExtractItem
	items = append(items, extract.ExtractItem{
		Data: results[0].Data,
		Opts: extract.Options{TargetDir: coreDir},
	})
	for i, ext := range res.Extensions {
		extDir := filepath.Join(bundlesDir, ext.Name)
		if err := os.MkdirAll(extDir, 0o750); err != nil {
			log.Fatalf("create extension dir %s: %v", ext.Name, err)
		}
		items = append(items, extract.ExtractItem{
			Data: results[i+1].Data,
			Opts: extract.Options{TargetDir: extDir},
		})
	}

	if err := extract.ExtractParallel(items); err != nil {
		log.Fatalf("extract: %v", err)
	}

	// 8. Compose
	layout := detectLayout(coreDir)

	var exts []compose.ExtensionComposition
	for _, ext := range res.Extensions {
		extDir := filepath.Join(bundlesDir, ext.Name)
		soPath := findSO(extDir, ext.Name)
		// Use the absolute .so path in the load directive rather than the
		// extension's name. The name form makes PHP open <extension_dir>/<name>.so
		// which is a symlink we create in compose.SymlinkExtension. glibc's
		// ld.so resolves $ORIGIN in the .so's DT_RUNPATH relative to the
		// SYMLINK'S directory, not the target's — so $ORIGIN/hermetic points
		// at an empty dir inside core/ and transitive hermetic libs fail to
		// dlopen. Absolute path bypasses the symlink entirely: ld.so evaluates
		// $ORIGIN against the real file's directory, where hermetic/ lives.
		ini := []string{fmt.Sprintf("extension=%s", soPath)}
		if spec, ok := cat.Extensions[ext.Name]; ok && len(spec.Ini) > 0 {
			ini = rewriteCatalogIniToAbsolute(spec.Ini, ext.Name, soPath)
		}
		exts = append(exts, compose.ExtensionComposition{
			Name: ext.Name, SOPath: soPath, IniLines: ini,
		})
	}

	if err := os.MkdirAll(layout.ConfDir, 0o750); err != nil {
		log.Fatalf("create conf dir: %v", err)
	}
	if err := compose.Compose(layout, exts); err != nil {
		log.Fatalf("compose: %v", err)
	}

	// 8b. Auto-load opcache when it's in the core's bundled set AND the user
	// didn't exclude it. `--enable-opcache` builds opcache as a shared module
	// (.so in a PHP-API-dated subdir of extension_dir). Without an explicit
	// zend_extension directive, opcache.* ini keys are unaddressable.
	// Respect :opcache exclusions and `none`-reset semantics by skipping the
	// loader when the user disabled opcache.
	opcacheExcluded := slices.Contains(p.ExtensionsExclude, "opcache") ||
		(p.ExtensionsReset && !slices.Contains(p.Extensions, "opcache"))
	if slices.Contains(bundled, "opcache") && !opcacheExcluded {
		if opcacheSO := findSO(layout.ExtensionDir, "opcache"); opcacheSO != "" {
			if err := compose.SymlinkExtension(opcacheSO, layout.ExtensionDir, "opcache"); err != nil {
				log.Fatalf("symlink opcache: %v", err)
			}
			if err := compose.WriteIniFragment(layout.ConfDir, "10-opcache", []string{"zend_extension=opcache.so"}); err != nil {
				log.Fatalf("write opcache loader: %v", err)
			}
		}
	}

	// 9a. Select base ini file (production/development/none) from the bundle.
	baseIni, baseWarn := compat.BaseIniFileName(p.IniFile)
	if baseWarn != "" {
		fmt.Fprintln(os.Stderr, baseWarn)
	}
	if err := os.MkdirAll(filepath.Dir(layout.IniFile), 0o750); err != nil {
		log.Fatalf("create php.ini dir: %v", err)
	}
	if err := compose.SelectBaseIniFile(layout, baseIni); err != nil {
		log.Fatalf("select base ini file: %v", err)
	}

	// 9b. Compose compat ini layers: defaults → xdebug fragment (only when
	// coverage: xdebug drives the install — matches v2, which applies
	// xdebug.ini only in the coverage flow, NOT when xdebug is loaded via
	// extensions:) → ExtraIni (e.g. pcov.enabled=1) → user ini-values.
	var xdebugFrag map[string]string
	if p.Coverage == plan.CoverageXdebug {
		xdebugFrag = compat.XdebugIniFragment(p.PHPVersion)
	}
	layered := compose.MergeCompatLayers(
		compat.DefaultIniValues(p.PHPVersion, p.Arch),
		xdebugFrag,
		p.ExtraIni,
	)
	if err := compose.WriteIniValuesWithDefaults(layout.ConfDir, layered, p.IniValues); err != nil {
		log.Fatalf("write ini values: %v", err)
	}

	// 10. Write disable fragments for excluded / reset-implied extensions.
	for _, name := range computeDisabledExtensions(p, bundled) {
		if err := compose.WriteDisableExtension(layout.ConfDir, name); err != nil {
			log.Fatalf("write disable fragment for %s: %v", name, err)
		}
	}

	// 11. Export env
	resolved := resolvePHPVersion(layout, p.PHPVersion)
	if err := exportEnv(layout, resolved); err != nil {
		log.Fatalf("export env: %v", err)
	}

	// 12. Seed cache
	if err := store.Seed(planHash, baseDir); err != nil {
		log.Printf("WARNING: failed to seed cache: %v", err)
	}

	fmt.Printf("PHP %s ready\n", resolved)
}

// computeDisabledExtensions returns the sorted list of extensions that should
// be disabled via conf.d fragments given the user's plan and the bundled
// extension set for the requested PHP version. The returned set is the union
// of:
//   - explicit ":ext" exclusions, and
//   - if the user wrote "none" in extensions (ExtensionsReset), every bundled
//     extension not in their explicit include list.
//
// Include wins: a name that appears in p.Extensions is never added to the
// disabled set — not from ExtensionsExclude (e.g. `extensions: redis, :redis`
// is a contradiction that resolves to "redis stays enabled") nor from the
// reset-minus-include logic.
//
// Sorted output ensures deterministic filesystem ordering across runs.
func computeDisabledExtensions(p *plan.Plan, bundled []string) []string {
	included := map[string]bool{}
	for _, name := range p.Extensions {
		included[name] = true
	}

	disabled := map[string]bool{}
	for _, name := range p.ExtensionsExclude {
		if included[name] {
			continue // include wins
		}
		disabled[name] = true
	}
	if p.ExtensionsReset {
		for _, name := range bundled {
			if included[name] {
				continue
			}
			disabled[name] = true
		}
	}
	if len(disabled) == 0 {
		return nil
	}
	out := make([]string, 0, len(disabled))
	for name := range disabled {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// resolvePHPVersion runs `php -v` against the composed binary and parses the
// full X.Y.Z version from the first line. Falls back to the requested version
// if execution fails (non-zero exit, unexpected output, etc.) — better to
// surface a less-specific version than to abort a successful install.
//
// The absolute path is constructed from layout.BinDir (runtime-owned, not
// user input). An earlier PATH-prepend attempt was dropped because duplicate
// PATH entries in cmd.Env caused glibc to resolve "php" to the system binary
// instead of the composed one.
func resolvePHPVersion(layout *compose.Layout, requested string) string {
	phpBin := filepath.Join(layout.BinDir, "php")
	out, err := exec.Command(phpBin, "-v").Output() //nolint:gosec // G204 false positive: phpBin is filepath.Join(layout.BinDir, "php"); layout.BinDir is runtime-composed, not user input
	if err != nil {
		log.Printf("WARNING: failed to run %s -v: %v; using requested %q as output", phpBin, err, requested)
		return requested
	}
	line := strings.SplitN(string(out), "\n", 2)[0]
	// Expected: "PHP 8.4.5 (cli) (built: ...)"
	fields := strings.Fields(line)
	if len(fields) >= 2 && fields[0] == "PHP" {
		return fields[1]
	}
	return requested
}

func layoutFromDir(dir string) *compose.Layout {
	core := filepath.Join(dir, "core")
	return &compose.Layout{
		RootDir:        dir,
		BinDir:         filepath.Join(core, "usr", "local", "bin"),
		ExtensionDir:   filepath.Join(core, "usr", "local", "lib", "php", "extensions"),
		ConfDir:        filepath.Join(core, "usr", "local", "etc", "php", "conf.d"),
		IniTemplateDir: filepath.Join(core, "usr", "local", "share", "php", "ini"),
		IniFile:        filepath.Join(core, "usr", "local", "lib", "php.ini"),
	}
}

func detectLayout(coreDir string) *compose.Layout {
	return &compose.Layout{
		RootDir:        filepath.Dir(coreDir),
		BinDir:         filepath.Join(coreDir, "usr", "local", "bin"),
		ExtensionDir:   filepath.Join(coreDir, "usr", "local", "lib", "php", "extensions"),
		ConfDir:        filepath.Join(coreDir, "usr", "local", "etc", "php", "conf.d"),
		IniTemplateDir: filepath.Join(coreDir, "usr", "local", "share", "php", "ini"),
		IniFile:        filepath.Join(coreDir, "usr", "local", "lib", "php.ini"),
	}
}

func exportEnv(layout *compose.Layout, phpVersion string) error {
	exporter, err := env.NewExporter()
	if err != nil {
		return err
	}
	if err := exporter.AddPath(layout.BinDir); err != nil {
		return err
	}
	// PHPRC tells PHP which base php.ini to load. Our PHP is built with
	// --prefix=/usr/local, so its compiled-in search path points at the
	// install prefix (not the bundle's extracted location). PHPRC redirects
	// it at the file SelectBaseIniFile wrote.
	if err := exporter.SetEnv("PHPRC", layout.IniFile); err != nil {
		return err
	}
	if err := exporter.SetEnv("PHP_INI_SCAN_DIR", layout.ConfDir); err != nil {
		return err
	}
	if err := exporter.SetOutput("php-version", phpVersion); err != nil {
		return err
	}
	return nil
}

func findSO(dir, name string) string {
	var result string
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.Name() == name+".so" {
			result = path
			return filepath.SkipAll
		}
		return nil
	})
	if err != nil && result == "" {
		log.Printf("WARNING: walk %s: %v", filepath.Clean(dir), err)
	}
	return result
}

// rewriteCatalogIniToAbsolute replaces catalog-declared "extension=<name>" and
// "zend_extension=<name>" directives with absolute-path variants pointing at
// soPath. See the comment at the call site for why: ld.so's $ORIGIN resolution
// uses the symlink's directory, which breaks hermetic-lib discovery when PHP
// opens an extension via its symlink in extension_dir.
func rewriteCatalogIniToAbsolute(lines []string, extName, soPath string) []string {
	targets := map[string]string{
		"extension=" + extName:      "extension=" + soPath,
		"zend_extension=" + extName: "zend_extension=" + soPath,
	}
	out := make([]string, len(lines))
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if replaced, ok := targets[trimmed]; ok {
			out[i] = replaced
			continue
		}
		out[i] = line
	}
	return out
}

// installRuntimeDeps installs the union of runtime_deps.linux apt packages
// declared by the resolved extensions. Linux-only; non-linux is a no-op.
// Packages are deduplicated across extensions and sorted for deterministic
// invocation (makes the apt-get command line stable for logging and tests).
// An empty package set is a no-op (no apt-get call).
func installRuntimeDeps(goos string, resolved []resolve.ResolvedBundle, cat *catalog.Catalog, installer func([]string) error) error {
	if goos != "linux" {
		return nil
	}
	set := map[string]struct{}{}
	for _, r := range resolved {
		spec, ok := cat.Extensions[r.Name]
		if !ok {
			continue
		}
		for _, pkg := range spec.RuntimeDeps["linux"] {
			set[pkg] = struct{}{}
		}
	}
	if len(set) == 0 {
		return nil
	}
	pkgs := make([]string, 0, len(set))
	for pkg := range set {
		pkgs = append(pkgs, pkg)
	}
	sort.Strings(pkgs)
	return installer(pkgs)
}

// aptInstaller is the production installer used when not testing.
// Runs `sudo apt-get update -qq` then `sudo apt-get install -y -qq --no-install-recommends <pkgs...>`.
// Update failure is a ::warning:: (transient mirror issues); install failure is a hard error.
func aptInstaller(pkgs []string) error {
	log.Printf("installRuntimeDeps: apt-get install %s", strings.Join(pkgs, " "))
	upd := exec.Command("sudo", "apt-get", "update", "-qq")
	upd.Stdout = os.Stdout
	upd.Stderr = os.Stderr
	if err := upd.Run(); err != nil {
		log.Printf("::warning::apt-get update failed (%v); proceeding to install", err)
	}
	args := append([]string{"apt-get", "install", "-y", "-qq", "--no-install-recommends"}, pkgs...)
	inst := exec.Command("sudo", args...) //nolint:gosec // G204: args built from catalog-provided package names (compile-time constant, non-user-controlled)
	inst.Stdout = os.Stdout
	inst.Stderr = os.Stderr
	if err := inst.Run(); err != nil {
		return fmt.Errorf("apt-get install runtime_deps %v: %w", pkgs, err)
	}
	return nil
}
