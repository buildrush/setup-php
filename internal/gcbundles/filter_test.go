package gcbundles

import (
	"testing"
	"time"
)

func TestCandidates_FiltersByAgeAndReferenced(t *testing.T) {
	now := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	minAge := 30 * 24 * time.Hour

	versions := []GHCRVersion{
		// Too young (14 days): skipped regardless of referenced-ness.
		{ID: 1, Name: "sha256:young", CreatedAt: now.Add(-14 * 24 * time.Hour)},
		// Old + unreferenced: CANDIDATE.
		{ID: 2, Name: "sha256:orphan", CreatedAt: now.Add(-90 * 24 * time.Hour)},
		// Old but referenced by a released lockfile: MUST NOT be a candidate.
		{ID: 3, Name: "sha256:released", CreatedAt: now.Add(-365 * 24 * time.Hour)},
		// Old but referenced by an open branch: MUST NOT be a candidate.
		{ID: 4, Name: "sha256:active", CreatedAt: now.Add(-60 * 24 * time.Hour)},
	}
	referenced := map[string]struct{}{
		"sha256:released": {},
		"sha256:active":   {},
	}

	got := Candidates(versions, referenced, minAge, now)

	if len(got) != 1 {
		t.Fatalf("len = %d, want 1; got %+v", len(got), got)
	}
	if got[0].ID != 2 {
		t.Errorf("candidate ID = %d, want 2 (orphan)", got[0].ID)
	}
}
