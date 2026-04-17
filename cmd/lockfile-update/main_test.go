package main

import (
	"testing"

	"github.com/buildrush/setup-php/internal/lockfile"
)

func TestBuildLockfileFromResolvedEntries(t *testing.T) {
	resolved := []resolvedEntry{
		{Key: "php:8.4:linux:x86_64:nts", Digest: "sha256:aaa", SpecHash: "sha256:hhh"},
		{Key: "ext:redis:6.2.0:8.4:linux:x86_64:nts", Digest: "sha256:bbb", SpecHash: "sha256:iii"},
	}
	lf := buildLockfile(resolved)
	if lf.SchemaVersion != 2 {
		t.Errorf("schema_version = %d, want 2", lf.SchemaVersion)
	}
	if len(lf.Bundles) != 2 {
		t.Fatalf("bundles len = %d, want 2", len(lf.Bundles))
	}
	got, ok := lf.LookupEntry("php:8.4:linux:x86_64:nts")
	if !ok {
		t.Fatal("php entry missing")
	}
	if got.Digest != "sha256:aaa" || got.SpecHash != "sha256:hhh" {
		t.Errorf("entry = %+v", got)
	}
}

func TestBuildLockfileEmpty(t *testing.T) {
	lf := buildLockfile(nil)
	if lf.SchemaVersion != 2 {
		t.Errorf("schema_version = %d, want 2", lf.SchemaVersion)
	}
	if len(lf.Bundles) != 0 {
		t.Errorf("expected empty bundles, got %d", len(lf.Bundles))
	}
	_ = lockfile.Entry{}
}

func TestSplitAbi(t *testing.T) {
	tests := []struct {
		in                string
		wantMinor, wantTS string
		wantOK            bool
	}{
		{"8.4-nts", "8.4", "nts", true},
		{"8.5-zts", "8.5", "zts", true},
		{"8.4.6-nts", "8.4.6", "nts", true},
		{"8.4", "8.4", "", false},
		{"", "", "", false},
		{"-nts", "-nts", "", false},
		{"nts", "nts", "", false},
	}
	for _, tt := range tests {
		m, ts, ok := splitAbi(tt.in)
		if m != tt.wantMinor || ts != tt.wantTS || ok != tt.wantOK {
			t.Errorf("splitAbi(%q) = (%q, %q, %v), want (%q, %q, %v)",
				tt.in, m, ts, ok, tt.wantMinor, tt.wantTS, tt.wantOK)
		}
	}
}
