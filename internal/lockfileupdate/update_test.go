package lockfileupdate

import (
	"path/filepath"
	"testing"
	"time"

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

func TestPreserveGeneratedAtIfUnchanged_SameBundles(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bundles.lock")

	old := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	existing := &lockfile.Lockfile{
		SchemaVersion: 2,
		GeneratedAt:   old,
		Bundles: map[lockfile.BundleKey]lockfile.Entry{
			"php:8.4:linux:x86_64:nts": {Digest: "sha256:aaa", SpecHash: "sha256:hhh"},
		},
	}
	if err := existing.Write(path); err != nil {
		t.Fatalf("write existing: %v", err)
	}

	newer := &lockfile.Lockfile{
		SchemaVersion: 2,
		GeneratedAt:   time.Now().UTC(),
		Bundles: map[lockfile.BundleKey]lockfile.Entry{
			"php:8.4:linux:x86_64:nts": {Digest: "sha256:aaa", SpecHash: "sha256:hhh"},
		},
	}
	preserveGeneratedAtIfUnchanged(newer, path)
	if !newer.GeneratedAt.Equal(old) {
		t.Errorf("GeneratedAt = %v, want preserved %v", newer.GeneratedAt, old)
	}
}

func TestPreserveGeneratedAtIfUnchanged_DifferentBundles(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bundles.lock")

	old := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	existing := &lockfile.Lockfile{
		SchemaVersion: 2,
		GeneratedAt:   old,
		Bundles: map[lockfile.BundleKey]lockfile.Entry{
			"php:8.4:linux:x86_64:nts": {Digest: "sha256:aaa", SpecHash: "sha256:hhh"},
		},
	}
	if err := existing.Write(path); err != nil {
		t.Fatalf("write existing: %v", err)
	}

	fresh := time.Now().UTC()
	newer := &lockfile.Lockfile{
		SchemaVersion: 2,
		GeneratedAt:   fresh,
		Bundles: map[lockfile.BundleKey]lockfile.Entry{
			"php:8.4:linux:x86_64:nts": {Digest: "sha256:NEW", SpecHash: "sha256:hhh"},
		},
	}
	preserveGeneratedAtIfUnchanged(newer, path)
	if !newer.GeneratedAt.Equal(fresh) {
		t.Errorf("GeneratedAt = %v, want fresh %v (bundles differ, must re-stamp)", newer.GeneratedAt, fresh)
	}
}

func TestPreserveGeneratedAtIfUnchanged_MissingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "does-not-exist.lock")

	fresh := time.Now().UTC()
	newer := &lockfile.Lockfile{
		SchemaVersion: 2,
		GeneratedAt:   fresh,
		Bundles:       map[lockfile.BundleKey]lockfile.Entry{},
	}
	preserveGeneratedAtIfUnchanged(newer, path)
	if !newer.GeneratedAt.Equal(fresh) {
		t.Errorf("GeneratedAt = %v, want fresh %v (no existing file, keep what we built)", newer.GeneratedAt, fresh)
	}
}
