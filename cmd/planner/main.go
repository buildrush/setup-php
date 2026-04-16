package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"

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

	// Expand PHP matrix
	phpCells := planner.ExpandPHPMatrix(cat.PHP)
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

func filterExisting(ctx context.Context, cells []planner.MatrixCell, lf *lockfile.Lockfile, client *oci.Client) []planner.MatrixCell {
	var filtered []planner.MatrixCell
	for i := range cells {
		cell := &cells[i]
		// Build the lockfile key for this cell
		var key string
		if cell.Extension != "" {
			key = lockfile.ExtBundleKey(cell.Extension, cell.ExtVer, cell.PHPAbi, cell.OS, cell.Arch, cell.TS)
		} else {
			key = lockfile.PHPBundleKey(cell.Version, cell.OS, cell.Arch, cell.TS)
		}

		digest, ok := lf.Lookup(key)
		if !ok {
			filtered = append(filtered, *cell) // not in lockfile, needs building
			continue
		}

		// Check if digest exists on registry
		var ref string
		if cell.Extension != "" {
			ref = fmt.Sprintf("ghcr.io/buildrush/php-ext-%s@%s", cell.Extension, digest)
		} else {
			ref = fmt.Sprintf("ghcr.io/buildrush/php-core@%s", digest)
		}
		exists, _ := client.Exists(ctx, ref, digest)
		if !exists {
			filtered = append(filtered, *cell) // not on registry, needs building
		}
		// else: skip, already built and published
	}
	return filtered
}
