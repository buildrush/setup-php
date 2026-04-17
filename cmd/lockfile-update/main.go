// Command lockfile-update regenerates bundles.lock from the current catalog
// and the state of GHCR. Replaces the previous bash + Python implementation.
//
// Usage:
//
//	lockfile-update [-catalog ./catalog] [-lockfile ./bundles.lock] [-registry ghcr.io/buildrush]
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/buildrush/setup-php/internal/catalog"
	"github.com/buildrush/setup-php/internal/lockfile"
	"github.com/buildrush/setup-php/internal/oci"
	"github.com/buildrush/setup-php/internal/planner"
)

type resolvedEntry struct {
	Key      string
	Digest   string
	SpecHash string
}

func main() {
	catalogDir := flag.String("catalog", "./catalog", "path to catalog directory")
	lockfilePath := flag.String("lockfile", "./bundles.lock", "path to bundles.lock")
	registry := flag.String("registry", "ghcr.io/buildrush", "OCI registry prefix")
	flag.Parse()

	ctx := context.Background()

	cat, err := catalog.LoadCatalog(*catalogDir)
	if err != nil {
		log.Fatalf("load catalog: %v", err)
	}
	if err := cat.PHP.Validate(); err != nil {
		log.Fatalf("validate catalog: %v", err)
	}

	token := os.Getenv("GHCR_TOKEN")
	client, err := oci.NewClient(*registry, token)
	if err != nil {
		log.Fatalf("create OCI client: %v", err)
	}

	builderHashPHP, err := planner.HashFile(filepath.Join("builders", "linux", "build-php.sh"))
	if err != nil {
		log.Fatalf("hash php builder: %v", err)
	}
	builderHashExt, err := planner.HashFile(filepath.Join("builders", "linux", "build-ext.sh"))
	if err != nil {
		log.Fatalf("hash ext builder: %v", err)
	}

	var resolved []resolvedEntry

	// PHP core cells.
	phpCells := planner.ExpandPHPMatrix(cat.PHP)
	for i := range phpCells {
		c := &phpCells[i]
		yamlBytes, err := planner.PerVersionYAML(cat.PHP, c.Version)
		if err != nil {
			log.Fatalf("per-version yaml %s: %v", c.Version, err)
		}
		c.SpecHash = planner.ComputeSpecHash(c, yamlBytes, builderHashPHP)

		tag := fmt.Sprintf("%s-%s-%s-%s", c.Version, c.OS, c.Arch, c.TS)
		ref := fmt.Sprintf("%s/php-core:%s", *registry, tag)
		digest, err := client.ResolveDigest(ctx, ref)
		if err != nil {
			log.Printf("WARNING: skip php-core %s: %v", tag, err)
			continue
		}
		key := lockfile.PHPBundleKey(c.Version, c.OS, c.Arch, c.TS)
		resolved = append(resolved, resolvedEntry{Key: key, Digest: digest, SpecHash: c.SpecHash})
	}

	// Extensions.
	for _, ext := range cat.Extensions {
		if ext.Kind == catalog.ExtensionKindBundled {
			continue
		}
		extYAML, err := planner.ExtensionYAML(ext)
		if err != nil {
			log.Fatalf("ext yaml %s: %v", ext.Name, err)
		}
		cells := planner.ExpandExtMatrix(ext)
		for i := range cells {
			c := &cells[i]
			c.SpecHash = planner.ComputeSpecHash(c, extYAML, builderHashExt)

			tag := fmt.Sprintf("%s-%s-%s-%s", c.ExtVer, c.PHPAbi, c.OS, c.Arch)
			ref := fmt.Sprintf("%s/php-ext-%s:%s", *registry, c.Extension, tag)
			digest, err := client.ResolveDigest(ctx, ref)
			if err != nil {
				log.Printf("WARNING: skip %s %s: %v", c.Extension, tag, err)
				continue
			}
			phpMinor, _, _ := splitAbi(c.PHPAbi)
			key := lockfile.ExtBundleKey(c.Extension, c.ExtVer, phpMinor, c.OS, c.Arch, c.TS)
			resolved = append(resolved, resolvedEntry{Key: key, Digest: digest, SpecHash: c.SpecHash})
		}
	}

	lf := buildLockfile(resolved)
	if err := lf.Write(*lockfilePath); err != nil {
		log.Fatalf("write lockfile: %v", err)
	}
	fmt.Printf("wrote %s with %d entries\n", *lockfilePath, len(lf.Bundles))
}

// buildLockfile turns a list of resolvedEntry into a canonical Lockfile.
// Isolated from main() so it is unit-testable.
func buildLockfile(entries []resolvedEntry) *lockfile.Lockfile {
	sort.Slice(entries, func(i, j int) bool { return entries[i].Key < entries[j].Key })
	lf := &lockfile.Lockfile{
		SchemaVersion: 2,
		GeneratedAt:   time.Now().UTC(),
		Bundles:       make(map[lockfile.BundleKey]lockfile.Entry, len(entries)),
	}
	for _, e := range entries {
		lf.Bundles[e.Key] = lockfile.Entry{Digest: e.Digest, SpecHash: e.SpecHash}
	}
	return lf
}

// splitAbi parses "8.4-nts" → ("8.4", "nts", true). Splits on the last hyphen
// so patch-level versions like "8.4.6-nts" also work.
func splitAbi(abi string) (phpMinor, ts string, ok bool) {
	idx := strings.LastIndex(abi, "-")
	if idx < 0 {
		return abi, "", false
	}
	return abi[:idx], abi[idx+1:], true
}
