package planner

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/buildrush/setup-php/internal/catalog"
	"github.com/buildrush/setup-php/internal/lockfile"
	"github.com/buildrush/setup-php/internal/oci"
)

// Main is the entry point for `phpup plan`. args is everything after the
// "plan" subcommand token. Output is byte-identical to the previous
// cmd/planner binary for the same inputs (same matrix JSON, same stdout
// progress line) — callers that parsed its output can switch transparently.
func Main(args []string) error {
	fs := flag.NewFlagSet("phpup plan", flag.ContinueOnError)
	catalogDir := fs.String("catalog", "./catalog", "path to catalog directory")
	lockfilePath := fs.String("lockfile", "./bundles.lock", "path to bundles.lock")
	registry := fs.String("registry", "ghcr.io/buildrush", "OCI registry prefix")
	outputDir := fs.String("output-matrix", "/tmp/matrix", "output directory for matrix JSON files")
	force := fs.Bool("force", false, "force rebuild even if digests match")
	if err := fs.Parse(args); err != nil {
		return err
	}

	ctx := context.Background()

	cat, err := catalog.LoadCatalog(*catalogDir)
	if err != nil {
		return fmt.Errorf("load catalog: %w", err)
	}
	if err := cat.PHP.Validate(); err != nil {
		return fmt.Errorf("validate PHP spec: %w", err)
	}

	lf, err := lockfile.ParseFile(*lockfilePath)
	if err != nil {
		return fmt.Errorf("parse lockfile: %w", err)
	}

	token := os.Getenv("GHCR_TOKEN")
	client, err := oci.NewClient(*registry, token)
	if err != nil {
		return fmt.Errorf("create OCI client: %w", err)
	}

	result := &Result{}

	builderOS, err := readBuilderOS(filepath.Join("builders", "common", "builder-os.env"))
	if err != nil {
		return fmt.Errorf("read builder-os.env: %w", err)
	}

	// Hash builder scripts and shared support files the builders source.
	// Changes to any of these change the bundle contents; fold them into
	// builderHash so spec_hash invalidates and bundles rebuild. Ordering
	// mirrors the historical inline concatenation (build-<kind>.sh + schema
	// env + capture + pack) so spec_hash values stay byte-identical across
	// this refactor.
	common := []string{
		filepath.Join("builders", "common", "bundle-schema-version.env"),
		filepath.Join("builders", "common", "capture-hermetic-libs.sh"),
		filepath.Join("builders", "common", "pack-bundle.sh"),
	}
	builderHashPHP, err := HashFiles(append(
		[]string{filepath.Join("builders", "linux", "build-php.sh")},
		common...,
	))
	if err != nil {
		return fmt.Errorf("hash php builder: %w", err)
	}
	builderHashExt, err := HashFiles(append(
		[]string{filepath.Join("builders", "linux", "build-ext.sh")},
		common...,
	))
	if err != nil {
		return fmt.Errorf("hash ext builder: %w", err)
	}

	phpCells := ExpandPHPMatrix(cat.PHP)
	for i := range phpCells {
		yamlBytes, err := PerVersionYAML(cat.PHP, phpCells[i].Version)
		if err != nil {
			return fmt.Errorf("per-version yaml for %s: %w", phpCells[i].Version, err)
		}
		phpCells[i].SpecHash = ComputeSpecHash(&phpCells[i], yamlBytes, builderHashPHP, builderOS)
	}

	if !*force {
		phpCells = filterExisting(ctx, phpCells, lf, client)
	}
	result.PHP = Matrix{Include: phpCells}

	// Build a map of already-published php-core digests keyed by
	// lockfile.PHPBundleKey (matches the key format ExpandExtMatrix builds
	// internally). Used to populate ext cells' CoreDigest field so the ext
	// builder can pin the core by digest. Cells whose core is being rebuilt
	// in the same run get CoreDigest plumbed from the within-run
	// build-php → build-ext ordering (see ci.yml::pipeline); legacy
	// planner-driven workflows kept this as a lockfile lookup only.
	coreDigestByKey := make(map[string]string, len(lf.Bundles))
	for key, entry := range lf.Bundles {
		if strings.HasPrefix(key, "php:") {
			coreDigestByKey[key] = entry.Digest
		}
	}

	var extCells []MatrixCell
	for _, ext := range cat.Extensions {
		if ext.Kind == catalog.ExtensionKindBundled {
			continue
		}
		cells := ExpandExtMatrix(ext, coreDigestByKey)
		extYAML, err := ExtensionYAML(ext)
		if err != nil {
			return fmt.Errorf("ext yaml for %s: %w", ext.Name, err)
		}
		for i := range cells {
			cells[i].SpecHash = ComputeSpecHash(&cells[i], extYAML, builderHashExt, builderOS)
		}
		if !*force {
			cells = filterExisting(ctx, cells, lf, client)
		}
		extCells = append(extCells, cells...)
	}
	result.Ext = Matrix{Include: extCells}

	// Tools: empty for Phase 1
	result.Tool = Matrix{Include: []MatrixCell{}}

	if err := WriteMatrices(result, *outputDir); err != nil {
		return fmt.Errorf("write matrices: %w", err)
	}

	fmt.Printf("Plan complete: %d PHP cores, %d extensions, %d tools\n",
		len(result.PHP.Include), len(result.Ext.Include), len(result.Tool.Include))
	return nil
}

// readBuilderOS parses builders/common/builder-os.env and returns the value
// of BUILDER_OS. Missing file or missing key is a fatal — the planner's
// spec_hash depends on this being load-bearing, and silent fallback would
// produce a lockfile where every entry looks up-to-date but reflects the
// wrong runner.
func readBuilderOS(path string) (string, error) {
	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return "", fmt.Errorf("read %s: %w", path, err)
	}
	v := ParseEnvValue(data, "BUILDER_OS")
	if v == "" {
		return "", fmt.Errorf("%s: BUILDER_OS not found", path)
	}
	return v, nil
}

func filterExisting(ctx context.Context, cells []MatrixCell, lf *lockfile.Lockfile, client *oci.Client) []MatrixCell {
	var filtered []MatrixCell
	for i := range cells {
		cell := &cells[i]

		var key string
		if cell.Extension != "" {
			// Lockfile stores php minor (e.g. "8.4") — not the combined ABI
			// string "8.4-nts". lockfile-update already splits on the last
			// hyphen to produce the same key format; mirror that here so
			// LookupEntry can find the entry. (Previously masked by a
			// digest-HEAD existence check that always failed for ext
			// artifacts, so the !ok branch was never reached.)
			phpMinor := cell.PHPAbi
			if idx := strings.LastIndex(cell.PHPAbi, "-"); idx > 0 {
				phpMinor = cell.PHPAbi[:idx]
			}
			key = lockfile.ExtBundleKey(cell.Extension, cell.ExtVer, phpMinor, cell.OS, cell.Arch, cell.TS)
		} else {
			key = lockfile.PHPBundleKey(cell.Version, cell.OS, cell.Arch, cell.TS)
		}

		entry, ok := lf.LookupEntry(key)
		if !ok {
			filtered = append(filtered, *cell)
			continue
		}
		if entry.SpecHash != "" && entry.SpecHash != cell.SpecHash {
			filtered = append(filtered, *cell)
			continue
		}

		// Resolve the tagged ref and compare its current digest against the
		// lockfile entry. Tag-form refs go through the same auth / API path
		// that build jobs successfully use when pulling (`oras pull :tag`),
		// whereas digest-form HEAD requests have been observed to fail with
		// 403 for some OCI artifacts on GHCR even when the package is public
		// and the repo has read access. Switching to tag+compare keeps the
		// verification semantics (skip only when the registry actually has
		// the lockfile digest) while working around that inconsistency.
		var ref string
		if cell.Extension != "" {
			tag := fmt.Sprintf("%s-%s-%s-%s", cell.ExtVer, cell.PHPAbi, cell.OS, cell.Arch)
			ref = fmt.Sprintf("ghcr.io/buildrush/php-ext-%s:%s", cell.Extension, tag)
		} else {
			tag := fmt.Sprintf("%s-%s-%s-%s", cell.Version, cell.OS, cell.Arch, cell.TS)
			ref = fmt.Sprintf("ghcr.io/buildrush/php-core:%s", tag)
		}
		currentDigest, err := client.ResolveDigest(ctx, ref)
		if err != nil {
			log.Printf("WARNING: resolve %s: %v; will rebuild", ref, err)
			filtered = append(filtered, *cell)
			continue
		}
		if currentDigest != entry.Digest {
			log.Printf("INFO: %s tag digest %s differs from lockfile %s; rebuild", ref, currentDigest, entry.Digest)
			filtered = append(filtered, *cell)
			continue
		}
	}
	return filtered
}
