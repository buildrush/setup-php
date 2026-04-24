package gcbundles

import "time"

// Candidates returns the versions eligible for pruning: older than minAge AND
// not in the referenced set. The returned slice preserves input order so the
// dry-run report is stable across runs.
func Candidates(versions []GHCRVersion, referenced map[string]struct{}, minAge time.Duration, now time.Time) []GHCRVersion {
	cutoff := now.Add(-minAge)
	var out []GHCRVersion
	for _, v := range versions {
		if v.CreatedAt.After(cutoff) {
			continue
		}
		if _, refed := referenced[v.Name]; refed {
			continue
		}
		out = append(out, v)
	}
	return out
}
