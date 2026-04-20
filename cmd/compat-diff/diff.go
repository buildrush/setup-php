package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
)

type probe struct {
	PHPVersion    string            `json:"php_version"`
	SAPI          string            `json:"sapi"`
	ZTS           bool              `json:"zts"`
	Extensions    []string          `json:"extensions"`
	INI           map[string]string `json:"ini"`
	EnvDelta      []string          `json:"env_delta"`
	PathAdditions []string          `json:"path_additions"`
}

type diffEntry struct {
	Path   string
	Ours   string
	Theirs string
}

func readProbe(path string) (probe, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return probe{}, fmt.Errorf("read probe %s: %w", path, err)
	}
	var p probe
	if err := json.Unmarshal(raw, &p); err != nil {
		return probe{}, fmt.Errorf("parse probe %s: %w", path, err)
	}
	return p, nil
}

func flatten(p *probe) map[string]string {
	out := map[string]string{
		"php_version":    p.PHPVersion,
		"sapi":           p.SAPI,
		"zts":            fmt.Sprintf("%t", p.ZTS),
		"extensions":     joinSorted(p.Extensions),
		"env_delta":      joinSorted(p.EnvDelta),
		"path_additions": joinSorted(p.PathAdditions),
	}
	for k, v := range p.INI {
		out["ini."+k] = v
	}
	return out
}

func joinSorted(xs []string) string {
	cp := make([]string, len(xs))
	copy(cp, xs)
	sort.Strings(cp)
	b, err := json.Marshal(cp)
	if err != nil {
		// []string cannot fail to marshal; this branch is unreachable.
		panic(fmt.Sprintf("joinSorted: unexpected marshal error: %v", err))
	}
	return string(b)
}

// fixtureMatches returns true if fixtures contains "*" or the exact fixture name.
func fixtureMatches(fixtures []string, fixture string) bool {
	for _, f := range fixtures {
		if f == "*" || f == fixture {
			return true
		}
	}
	return false
}

// applyAllowlist mutates the flattened probes per the allowlist, returning
// the set of paths flagged as allow (caller treats any non-empty/non-empty
// pair on those paths as equal).
func applyAllowlist(ours, theirs map[string]string, al allowlist, fixture string) map[string]bool {
	allow := map[string]bool{}
	for _, d := range al.Deviations {
		if !fixtureMatches(d.Fixtures, fixture) {
			continue
		}
		switch d.Kind {
		case "ignore":
			delete(ours, d.Path)
			delete(theirs, d.Path)
		case "allow":
			allow[d.Path] = true
		}
	}
	return allow
}

func diffProbes(ours, theirs *probe, al allowlist, fixture string) []diffEntry {
	lo := flatten(ours)
	lt := flatten(theirs)
	allow := applyAllowlist(lo, lt, al, fixture)

	keys := map[string]struct{}{}
	for k := range lo {
		keys[k] = struct{}{}
	}
	for k := range lt {
		keys[k] = struct{}{}
	}
	sorted := make([]string, 0, len(keys))
	for k := range keys {
		sorted = append(sorted, k)
	}
	sort.Strings(sorted)

	var out []diffEntry
	for _, k := range sorted {
		vo, vt := lo[k], lt[k]
		if vo == vt {
			continue
		}
		if allow[k] {
			// allow kind: both must be non-empty (including "not the empty JSON array")
			if vo != "" && vo != "[]" && vt != "" && vt != "[]" {
				continue
			}
		}
		out = append(out, diffEntry{Path: k, Ours: vo, Theirs: vt})
	}
	return out
}
