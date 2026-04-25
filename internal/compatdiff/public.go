package compatdiff

// Deviation is a single path-level divergence between two probes after
// allowlist processing. Exported counterpart of the internal diffEntry
// type; used by internal/testsuite.runFixture to record structured
// failure data for the PR-time deviation artifact.
type Deviation struct {
	Path   string
	Ours   string
	Theirs string
}

// DiffFiles loads two probe JSON files and an allowlist markdown file,
// runs the same comparison as the `phpup compat-diff` CLI, and returns
// any deviations that are neither identical nor allowlist-tolerated.
//
// Errors:
//   - reading or parsing either probe -> non-nil error
//   - reading or parsing the allowlist -> non-nil error
//   - invalid allowlist entries (bad kind/empty path) -> non-nil error
//
// A nil error with an empty returned slice means "no deviations".
// A nil error with a non-empty slice means "deviations found" — the
// caller decides how to surface them.
func DiffFiles(oursPath, theirsPath, allowlistPath, fixture string) ([]Deviation, error) {
	ours, err := readProbe(oursPath)
	if err != nil {
		return nil, err
	}
	theirs, err := readProbe(theirsPath)
	if err != nil {
		return nil, err
	}
	al, err := loadAllowlist(allowlistPath)
	if err != nil {
		return nil, err
	}
	entries := diffProbes(&ours, &theirs, al, fixture)
	devs := make([]Deviation, len(entries))
	for i, e := range entries {
		devs[i] = Deviation(e)
	}
	return devs, nil
}
