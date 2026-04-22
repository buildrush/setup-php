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
	"maps"
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
	commit := flag.Bool("commit", false, "commit + push the updated lockfile to HEAD (CI use)")
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

	builderOS, err := readBuilderOS(filepath.Join("builders", "common", "builder-os.env"))
	if err != nil {
		log.Fatalf("read builder-os.env: %v", err)
	}

	// Hash builder scripts and schema version file
	builderHashPHP, err := planner.HashFile(filepath.Join("builders", "linux", "build-php.sh"))
	if err != nil {
		log.Fatalf("hash php builder: %v", err)
	}
	builderHashExt, err := planner.HashFile(filepath.Join("builders", "linux", "build-ext.sh"))
	if err != nil {
		log.Fatalf("hash ext builder: %v", err)
	}
	schemaEnvHash, err := planner.HashFile(filepath.Join("builders", "common", "bundle-schema-version.env"))
	if err != nil {
		log.Fatalf("hash schema env: %v", err)
	}
	builderHashPHP = builderHashPHP + ":" + schemaEnvHash
	builderHashExt = builderHashExt + ":" + schemaEnvHash

	var resolved []resolvedEntry

	// PHP core cells.
	phpCells := planner.ExpandPHPMatrix(cat.PHP)
	for i := range phpCells {
		c := &phpCells[i]
		yamlBytes, err := planner.PerVersionYAML(cat.PHP, c.Version)
		if err != nil {
			log.Fatalf("per-version yaml %s: %v", c.Version, err)
		}
		c.SpecHash = planner.ComputeSpecHash(c, yamlBytes, builderHashPHP, builderOS)

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
			c.SpecHash = planner.ComputeSpecHash(c, extYAML, builderHashExt, builderOS)

			tag := fmt.Sprintf("%s-%s-%s-%s", c.ExtVer, c.PHPAbi, c.OS, c.Arch)
			ref := fmt.Sprintf("%s/php-ext-%s:%s", *registry, c.Extension, tag)
			digest, err := client.ResolveDigest(ctx, ref)
			if err != nil {
				log.Printf("WARNING: skip %s %s: %v", c.Extension, tag, err)
				continue
			}
			phpMinor, _, ok := splitAbi(c.PHPAbi)
			if !ok {
				log.Printf("WARNING: skip %s: malformed php_abi %q", c.Extension, c.PHPAbi)
				continue
			}
			key := lockfile.ExtBundleKey(c.Extension, c.ExtVer, phpMinor, c.OS, c.Arch, c.TS)
			resolved = append(resolved, resolvedEntry{Key: key, Digest: digest, SpecHash: c.SpecHash})
		}
	}

	lf := buildLockfile(resolved)
	preserveGeneratedAtIfUnchanged(lf, *lockfilePath)

	if err := lf.Write(*lockfilePath); err != nil {
		log.Fatalf("write lockfile: %v", err)
	}
	fmt.Printf("wrote %s with %d entries\n", *lockfilePath, len(lf.Bundles))

	if *commit {
		branch := firstNonEmpty(os.Getenv("GITHUB_HEAD_REF"), os.Getenv("GITHUB_REF_NAME"))
		if branch == "" {
			log.Fatalf("--commit requires GITHUB_HEAD_REF or GITHUB_REF_NAME to be set")
		}
		runID := os.Getenv("GITHUB_RUN_ID")
		if runID == "" {
			runID = "manual"
		}
		if err := CommitLockfile(CommitOpts{
			LockfilePath: *lockfilePath,
			BranchRef:    branch,
			Message:      fmt.Sprintf("chore(lock): update bundles.lock from pipeline %s", runID),
			ActorName:    "github-actions[bot]",
			ActorEmail:   "41898282+github-actions[bot]@users.noreply.github.com",
		}); err != nil {
			log.Fatalf("commit lockfile: %v", err)
		}
	}
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
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

// preserveGeneratedAtIfUnchanged reads the existing lockfile at path, and if
// its bundle map equals lf's, sets lf.GeneratedAt to the existing timestamp.
// Without this, every no-op planner run produces a timestamp-only byte diff
// that the --commit mode dutifully commits and pushes — driving an endless
// lockfile-update loop on bundle-affecting PRs once the first real rebuild
// has landed. A missing or unreadable existing file is not an error here; we
// simply keep the freshly-stamped GeneratedAt that buildLockfile produced.
func preserveGeneratedAtIfUnchanged(lf *lockfile.Lockfile, path string) {
	existing, err := lockfile.ParseFile(path)
	if err != nil {
		return
	}
	if maps.Equal(existing.Bundles, lf.Bundles) {
		lf.GeneratedAt = existing.GeneratedAt
	}
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
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "BUILDER_OS=") {
			return strings.TrimPrefix(line, "BUILDER_OS="), nil
		}
	}
	return "", fmt.Errorf("%s: BUILDER_OS not found", path)
}

// splitAbi parses "8.4-nts" → ("8.4", "nts", true). Splits on the last hyphen
// so patch-level versions like "8.4.6-nts" also work. Rejects inputs with no
// dash or a leading dash (empty minor) so a malformed ABIMatrix.PHP entry
// cannot silently yield a key like "ext:redis:6.2.0::linux:...".
func splitAbi(abi string) (phpMinor, ts string, ok bool) {
	idx := strings.LastIndex(abi, "-")
	if idx <= 0 {
		return abi, "", false
	}
	return abi[:idx], abi[idx+1:], true
}
