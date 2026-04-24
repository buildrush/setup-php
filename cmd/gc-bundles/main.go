// Command gc-bundles prunes unreferenced GHCR bundles.
//
// Runs in two modes:
//
//	--dry-run (default): list candidate versions without deleting.
//	--confirm: actually delete the candidates. Guarded behind a workflow
//	           dispatch input; local dev should stick to --dry-run.
//
// A version is a candidate for pruning when it is:
//   - a container manifest under ghcr.io/<org>/php-core or
//     ghcr.io/<org>/php-ext-<name>;
//   - older than --min-age-days (default 30);
//   - not referenced by bundles.lock on any remote branch or any released
//     tag's embedded bundles.lock.
//
// Released-tag lockfile references are load-bearing for product-vision §16
// reproducibility and must never be pruned.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"
)

func main() {
	org := flag.String("org", "buildrush", "GHCR organization to scan")
	minAgeDays := flag.Int("min-age-days", 30, "minimum age before a version is prunable")
	confirm := flag.Bool("confirm", false, "actually delete (default is dry-run)")
	flag.Parse()

	if err := run(*org, *minAgeDays, *confirm, os.Stdout); err != nil {
		log.Fatalf("gc-bundles: %v", err)
	}
}

// run is the testable core. It writes a human-readable report to w —
// invoke on-demand (e.g. `go run ./cmd/gc-bundles --org buildrush
// --min-age-days 30`) when GHCR quota pressure warrants a sweep.
func run(org string, minAgeDays int, confirm bool, w *os.File) error {
	runner := NewGHRunner()

	// 1. Collect referenced digests from all branches + released tags.
	releaseTags, err := listReleases(runner)
	if err != nil {
		return fmt.Errorf("list releases: %w", err)
	}
	referenced, err := EnumerateReferencedDigests(".", releaseTags)
	if err != nil {
		return fmt.Errorf("enumerate refs: %w", err)
	}
	if _, err := fmt.Fprintf(w, "referenced: %d digest(s) across branches + %d release tag(s)\n",
		len(referenced), len(releaseTags)); err != nil {
		return err
	}

	// 2. Enumerate our packages. We only touch php-core and php-ext-*.
	packages, err := listSetupPhpPackages(runner, org)
	if err != nil {
		return fmt.Errorf("list packages: %w", err)
	}

	// 3. For each package, list versions + filter to candidates.
	minAge := time.Duration(minAgeDays) * 24 * time.Hour
	now := time.Now().UTC()
	totalCandidates := 0
	for _, pkg := range packages {
		versions, err := ListPackageVersions(runner, org, pkg)
		if err != nil {
			if _, werr := fmt.Fprintf(w, "WARNING: list %s: %v\n", pkg, err); werr != nil {
				return werr
			}
			continue
		}
		cands := Candidates(versions, referenced, minAge, now)
		for _, c := range cands {
			if _, werr := fmt.Fprintf(w, "candidate: %s %s (id=%d, created=%s)\n",
				pkg, c.Name, c.ID, c.CreatedAt.Format(time.RFC3339)); werr != nil {
				return werr
			}
			if confirm {
				if err := DeletePackageVersion(runner, org, pkg, c.ID); err != nil {
					if _, werr := fmt.Fprintf(w, "  DELETE FAILED: %v\n", err); werr != nil {
						return werr
					}
				} else {
					if _, werr := fmt.Fprintf(w, "  deleted\n"); werr != nil {
						return werr
					}
				}
			}
		}
		totalCandidates += len(cands)
	}

	mode := "dry-run"
	if confirm {
		mode = "DELETE"
	}
	if _, err := fmt.Fprintf(w, "%s complete: %d candidate(s) across %d package(s)\n",
		mode, totalCandidates, len(packages)); err != nil {
		return err
	}
	return nil
}

// listReleases calls `gh release list` and returns tag names.
func listReleases(r Runner) ([]string, error) {
	out, err := r.Run("release", "list", "--json", "tagName", "--limit", "500")
	if err != nil {
		return nil, err
	}
	var releases []struct {
		TagName string `json:"tagName"`
	}
	if err := json.Unmarshal(out, &releases); err != nil {
		return nil, fmt.Errorf("parse releases: %w", err)
	}
	tags := make([]string, 0, len(releases))
	for _, rel := range releases {
		tags = append(tags, rel.TagName)
	}
	return tags, nil
}

// listSetupPhpPackages returns only the package names we manage (php-core,
// php-ext-*). Other org packages are out of scope for this GC.
func listSetupPhpPackages(r Runner, org string) ([]string, error) {
	out, err := r.Run("api", "--paginate",
		fmt.Sprintf("/orgs/%s/packages?package_type=container&per_page=100", org))
	if err != nil {
		return nil, err
	}
	var pkgs []struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(out, &pkgs); err != nil {
		return nil, fmt.Errorf("parse packages: %w", err)
	}
	var filtered []string
	for _, p := range pkgs {
		if p.Name == "php-core" || strings.HasPrefix(p.Name, "php-ext-") {
			filtered = append(filtered, p.Name)
		}
	}
	return filtered, nil
}
