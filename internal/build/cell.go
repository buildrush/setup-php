package build

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/buildrush/setup-php/internal/catalog"
)

// extensionBuildEntry describes one ext to build for a cell.
type extensionBuildEntry struct {
	Name    string
	Version string
}

// discoverExtensionsForCell walks catalog/extensions/*.yaml under repo and
// returns one entry per extension compatible with the supplied cell. The
// compatibility check mirrors planner.ExpandExtMatrix exactly — an
// extension is compatible iff its kind is not "bundled", its abi_matrix
// contains the requested (php, os, arch, ts) quadruple, and no exclude
// rule matches.
//
// Only PECL/git extensions contribute entries — bundled extensions come
// with php-core and have no separate bundle. For each compatible
// extension the chosen Version is taken verbatim from spec.Versions
// (the catalog only ships one pinned version per extension at present;
// when multi-version slices land this function will need a "pick
// highest-precedence" policy rather than a single-value copy).
//
// Results are sorted by extension name for deterministic ordering so
// repeated BuildCell invocations produce reproducible build graphs.
func discoverExtensionsForCell(repo, php, osName, arch, ts string) ([]extensionBuildEntry, error) {
	extDir := filepath.Join(repo, "catalog", "extensions")
	entries, err := os.ReadDir(extDir)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", extDir, err)
	}

	var out []extensionBuildEntry
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".yaml") {
			continue
		}
		path := filepath.Join(extDir, e.Name())
		spec, err := catalog.LoadExtensionSpec(path)
		if err != nil {
			return nil, fmt.Errorf("load %s: %w", path, err)
		}
		if !extensionSupportsCell(spec, php, osName, arch, ts) {
			continue
		}
		// Catalog currently ships one pinned version per extension; take
		// the first entry. When multi-version slices land this will need
		// to be revisited — see doc comment above.
		if len(spec.Versions) == 0 {
			continue
		}
		out = append(out, extensionBuildEntry{Name: spec.Name, Version: spec.Versions[0]})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// extensionSupportsCell reports whether spec's abi_matrix covers the
// requested (php, os, arch, ts) tuple AND the tuple is not excluded.
// Mirrors planner.ExpandExtMatrix's iteration + isExcluded check, but
// short-circuits on the first hit instead of enumerating the full
// cross-product.
//
// Bundled extensions have no separate bundle, so they are reported as
// NOT compatible here (matching planner.ExpandExtMatrix's early return).
func extensionSupportsCell(spec *catalog.ExtensionSpec, php, osName, arch, ts string) bool {
	if spec.Kind == catalog.ExtensionKindBundled {
		return false
	}
	if !containsString(spec.ABIMatrix.PHP, php) {
		return false
	}
	if !containsString(spec.ABIMatrix.OS, osName) {
		return false
	}
	if !containsString(spec.ABIMatrix.Arch, arch) {
		return false
	}
	if !containsString(spec.ABIMatrix.TS, ts) {
		return false
	}
	// Use the same exclude check shape the planner uses; kept local so
	// we don't depend on an unexported planner helper.
	for _, r := range spec.Exclude {
		if excludeMatches(r, osName, arch, php) {
			return false
		}
	}
	return true
}

// containsString reports whether needle is in haystack. Local helper so
// the compatibility predicate can stay single-file.
func containsString(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}

// excludeMatches mirrors planner.isExcluded (unexported there) so
// discoverExtensionsForCell honours catalog exclude rules byte-for-byte
// with the CI planner.
func excludeMatches(r catalog.ExcludeRule, osName, arch, php string) bool {
	if r.OS != "" && r.OS != osName {
		return false
	}
	if r.Arch != "" && r.Arch != arch {
		return false
	}
	if r.PHP != "" && r.PHP != php {
		return false
	}
	// All three either empty (match-any) or exact-match.
	return true
}
