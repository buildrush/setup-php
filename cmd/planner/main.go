package main

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
	"github.com/buildrush/setup-php/internal/planner"
)

func main() {
	catalogDir := flag.String("catalog", "./catalog", "path to catalog directory")
	lockfilePath := flag.String("lockfile", "./bundles.lock", "path to bundles.lock")
	registry := flag.String("registry", "ghcr.io/buildrush", "OCI registry prefix")
	outputDir := flag.String("output-matrix", "/tmp/matrix", "output directory for matrix JSON files")
	force := flag.Bool("force", false, "force rebuild even if digests match")
	flag.Parse()

	ctx := context.Background()

	cat, err := catalog.LoadCatalog(*catalogDir)
	if err != nil {
		log.Fatalf("load catalog: %v", err)
	}
	if err := cat.PHP.Validate(); err != nil {
		log.Fatalf("validate PHP spec: %v", err)
	}

	lf, err := lockfile.ParseFile(*lockfilePath)
	if err != nil {
		log.Fatalf("parse lockfile: %v", err)
	}

	token := os.Getenv("GHCR_TOKEN")
	client, err := oci.NewClient(*registry, token)
	if err != nil {
		log.Fatalf("create OCI client: %v", err)
	}

	result := &planner.Result{}

	builderOS, err := readBuilderOS(filepath.Join("builders", "common", "builder-os.env"))
	if err != nil {
		log.Fatalf("read builder-os.env: %v", err)
	}

	// Hash builder scripts and shared support files the builders source. Changes
	// to any of these change the bundle contents; fold them into builderHash so
	// spec_hash invalidates and bundles rebuild. Ordering mirrors the historical
	// inline concatenation (build-<kind>.sh + schema env + capture + pack) so
	// spec_hash values stay byte-identical across this refactor.
	common := []string{
		filepath.Join("builders", "common", "bundle-schema-version.env"),
		filepath.Join("builders", "common", "capture-hermetic-libs.sh"),
		filepath.Join("builders", "common", "pack-bundle.sh"),
	}
	builderHashPHP, err := planner.HashFiles(append(
		[]string{filepath.Join("builders", "linux", "build-php.sh")},
		common...,
	))
	if err != nil {
		log.Fatalf("hash php builder: %v", err)
	}
	builderHashExt, err := planner.HashFiles(append(
		[]string{filepath.Join("builders", "linux", "build-ext.sh")},
		common...,
	))
	if err != nil {
		log.Fatalf("hash ext builder: %v", err)
	}

	// Expand PHP matrix
	phpCells := planner.ExpandPHPMatrix(cat.PHP)
	for i := range phpCells {
		yamlBytes, err := planner.PerVersionYAML(cat.PHP, phpCells[i].Version)
		if err != nil {
			log.Fatalf("per-version yaml for %s: %v", phpCells[i].Version, err)
		}
		phpCells[i].SpecHash = planner.ComputeSpecHash(&phpCells[i], yamlBytes, builderHashPHP, builderOS)
	}

	if !*force {
		phpCells = filterExisting(ctx, phpCells, lf, client)
	}
	result.PHP = planner.Matrix{Include: phpCells}

	// Expand extension matrices
	var extCells []planner.MatrixCell
	for _, ext := range cat.Extensions {
		if ext.Kind == catalog.ExtensionKindBundled {
			continue
		}
		cells := planner.ExpandExtMatrix(ext)
		extYAML, err := planner.ExtensionYAML(ext)
		if err != nil {
			log.Fatalf("ext yaml for %s: %v", ext.Name, err)
		}
		for i := range cells {
			cells[i].SpecHash = planner.ComputeSpecHash(&cells[i], extYAML, builderHashExt, builderOS)
		}
		if !*force {
			cells = filterExisting(ctx, cells, lf, client)
		}
		extCells = append(extCells, cells...)
	}
	result.Ext = planner.Matrix{Include: extCells}

	// Tools: empty for Phase 1
	result.Tool = planner.Matrix{Include: []planner.MatrixCell{}}

	if err := planner.WriteMatrices(result, *outputDir); err != nil {
		log.Fatalf("write matrices: %v", err)
	}

	fmt.Printf("Plan complete: %d PHP cores, %d extensions, %d tools\n",
		len(result.PHP.Include), len(result.Ext.Include), len(result.Tool.Include))
}

// readBuilderOS parses builders/common/builder-os.env and returns the value of
// BUILDER_OS. Missing file or missing key is a fatal — the planner's spec_hash
// depends on this being load-bearing, and silent fallback would produce a
// lockfile where every entry looks up-to-date but reflects the wrong runner.
func readBuilderOS(path string) (string, error) {
	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return "", fmt.Errorf("read %s: %w", path, err)
	}
	v := planner.ParseEnvValue(data, "BUILDER_OS")
	if v == "" {
		return "", fmt.Errorf("%s: BUILDER_OS not found", path)
	}
	return v, nil
}

func filterExisting(ctx context.Context, cells []planner.MatrixCell, lf *lockfile.Lockfile, client *oci.Client) []planner.MatrixCell {
	var filtered []planner.MatrixCell
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
			filtered = append(filtered, *cell) // not in lockfile, must build
			continue
		}
		if entry.SpecHash != "" && entry.SpecHash != cell.SpecHash {
			filtered = append(filtered, *cell) // inputs drifted, must rebuild
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
		// Tag exists and resolves to the lockfile digest — skip.
	}
	return filtered
}
