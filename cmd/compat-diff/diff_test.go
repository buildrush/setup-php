package main

import (
	"path/filepath"
	"testing"
)

func loadProbe(t *testing.T, name string) *probe {
	t.Helper()
	p, err := readProbe(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("readProbe %s: %v", name, err)
	}
	return &p
}

func TestDiffExactMatch(t *testing.T) {
	a := loadProbe(t, "probe-bare.json")
	b := loadProbe(t, "probe-bare.json")
	res := diffProbes(a, b, allowlist{}, "bare")
	if len(res) != 0 {
		t.Fatalf("expected 0 diffs, got %d: %+v", len(res), res)
	}
}

func TestDiffIniValueChange(t *testing.T) {
	a := loadProbe(t, "probe-bare.json")
	b := loadProbe(t, "probe-bare-ini-shift.json")
	res := diffProbes(a, b, allowlist{}, "bare")
	if len(res) != 1 {
		t.Fatalf("expected 1 diff, got %d: %+v", len(res), res)
	}
	if res[0].Path != "ini.memory_limit" {
		t.Errorf("Path = %q, want ini.memory_limit", res[0].Path)
	}
	if res[0].Ours != "128M" || res[0].Theirs != "256M" {
		t.Errorf("values = (%q,%q), want (128M,256M)", res[0].Ours, res[0].Theirs)
	}
}

func TestDiffIgnoreKindDropsKey(t *testing.T) {
	a := loadProbe(t, "probe-bare.json")
	b := loadProbe(t, "probe-bare-ini-shift.json")
	al := allowlist{Deviations: []deviation{
		{Path: "ini.memory_limit", Kind: "ignore", Reason: "x", Fixtures: []string{"*"}},
	}}
	res := diffProbes(a, b, al, "bare")
	if len(res) != 0 {
		t.Fatalf("expected ignore to drop diff, got %+v", res)
	}
}

func TestDiffAllowKindBothNonEmpty(t *testing.T) {
	a := loadProbe(t, "probe-bare.json")
	b := loadProbe(t, "probe-bare-ini-shift.json")
	al := allowlist{Deviations: []deviation{
		{Path: "ini.memory_limit", Kind: "allow", Reason: "x", Fixtures: []string{"bare"}},
	}}
	res := diffProbes(a, b, al, "bare")
	if len(res) != 0 {
		t.Fatalf("expected allow to pass (both non-empty), got %+v", res)
	}
}

func TestDiffAllowKindEmptyFails(t *testing.T) {
	a := loadProbe(t, "probe-bare.json")
	b := loadProbe(t, "probe-bare-ext-empty.json")
	al := allowlist{Deviations: []deviation{
		{Path: "extensions", Kind: "allow", Reason: "x", Fixtures: []string{"bare"}},
	}}
	res := diffProbes(a, b, al, "bare")
	if len(res) != 1 {
		t.Fatalf("expected allow to fail (one side empty), got %d diffs: %+v", len(res), res)
	}
}

func TestDiffFixtureGlobFilter(t *testing.T) {
	a := loadProbe(t, "probe-bare.json")
	b := loadProbe(t, "probe-bare-ini-shift.json")
	al := allowlist{Deviations: []deviation{
		{Path: "ini.memory_limit", Kind: "ignore", Reason: "x", Fixtures: []string{"other-fixture"}},
	}}
	res := diffProbes(a, b, al, "bare")
	if len(res) != 1 {
		t.Fatalf("expected fixture filter to block deviation, got %d diffs", len(res))
	}
}
