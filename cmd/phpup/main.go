package main

import (
	"context"
	_ "embed"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/buildrush/setup-php/internal/cache"
	"github.com/buildrush/setup-php/internal/catalog"
	"github.com/buildrush/setup-php/internal/compose"
	"github.com/buildrush/setup-php/internal/env"
	"github.com/buildrush/setup-php/internal/extract"
	"github.com/buildrush/setup-php/internal/lockfile"
	"github.com/buildrush/setup-php/internal/oci"
	"github.com/buildrush/setup-php/internal/plan"
	"github.com/buildrush/setup-php/internal/resolve"
	"github.com/buildrush/setup-php/internal/version"
)

//go:embed bundles.lock
var embeddedLockfile []byte

var bundledExtensions = []string{
	"mbstring", "curl", "intl", "zip", "json", "pdo", "pdo_mysql",
	"pdo_sqlite", "sqlite3", "tokenizer", "xml", "dom", "simplexml",
	"xmlreader", "xmlwriter", "ctype", "filter", "hash", "iconv",
	"session", "bcmath", "calendar", "exif", "ftp", "soap", "sockets",
	"sodium", "gd", "readline", "openssl", "zlib", "opcache",
	"pgsql", "pdo_pgsql",
}

func main() {
	if len(os.Args) > 1 && os.Args[1] == "--version" {
		fmt.Printf("phpup %s (%s) built %s\n", version.Version, version.Commit, version.BuildDate)
		return
	}

	ctx := context.Background()

	// 1. Parse inputs
	p, err := plan.FromEnv()
	if err != nil {
		log.Fatalf("parse inputs: %v", err)
	}

	if p.Verbose {
		log.Printf("Plan: PHP %s, extensions=%v, os=%s, arch=%s, ts=%s",
			p.PHPVersion, p.Extensions, p.OS, p.Arch, p.ThreadSafety)
	}

	// 2. Load embedded lockfile
	lf, err := lockfile.Parse(embeddedLockfile)
	if err != nil {
		log.Fatalf("parse embedded lockfile: %v", err)
	}

	// 3. Build minimal catalog for resolution
	cat := &catalog.Catalog{
		PHP: &catalog.PHPSpec{BundledExtensions: bundledExtensions},
		Extensions: map[string]*catalog.ExtensionSpec{
			"redis": {Name: "redis", Kind: catalog.ExtensionKindPECL, Versions: []string{"6.2.0"}},
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

	// 5. Check cache
	store, _ := cache.NewStore()
	planHash := p.Hash()
	hit := store.Check(planHash)

	if hit.Hit {
		if p.Verbose {
			log.Printf("Cache hit at %s", hit.Path)
		}
		layout := layoutFromDir(hit.Path)
		if err := exportEnv(layout, p.PHPVersion); err != nil {
			log.Fatalf("export env: %v", err)
		}
		fmt.Printf("PHP %s ready (cached)\n", p.PHPVersion)
		return
	}

	// 6. Fetch from GHCR
	token := os.Getenv("INPUT_GITHUB-TOKEN")
	if token == "" {
		token = os.Getenv("GITHUB_TOKEN")
	}
	registry := "ghcr.io/buildrush"
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
		ini := []string{fmt.Sprintf("extension=%s", ext.Name)}
		if spec, ok := cat.Extensions[ext.Name]; ok && len(spec.Ini) > 0 {
			ini = spec.Ini
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

	// 9. Write user ini values
	if err := compose.WriteIniValues(layout.ConfDir, p.IniValues); err != nil {
		log.Fatalf("write ini values: %v", err)
	}

	// 10. Export env
	if err := exportEnv(layout, p.PHPVersion); err != nil {
		log.Fatalf("export env: %v", err)
	}

	// 11. Seed cache
	if err := store.Seed(planHash, baseDir); err != nil {
		log.Printf("WARNING: failed to seed cache: %v", err)
	}

	fmt.Printf("PHP %s ready\n", p.PHPVersion)
}

func layoutFromDir(dir string) *compose.Layout {
	return &compose.Layout{
		RootDir:      dir,
		BinDir:       filepath.Join(dir, "core", "usr", "local", "bin"),
		ExtensionDir: filepath.Join(dir, "core", "usr", "local", "lib", "php", "extensions"),
		ConfDir:      filepath.Join(dir, "core", "usr", "local", "etc", "php", "conf.d"),
	}
}

func detectLayout(coreDir string) *compose.Layout {
	return &compose.Layout{
		RootDir:      filepath.Dir(coreDir),
		BinDir:       filepath.Join(coreDir, "usr", "local", "bin"),
		ExtensionDir: filepath.Join(coreDir, "usr", "local", "lib", "php", "extensions"),
		ConfDir:      filepath.Join(coreDir, "usr", "local", "etc", "php", "conf.d"),
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
