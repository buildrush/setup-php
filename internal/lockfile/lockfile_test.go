package lockfile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestParse(t *testing.T) {
	data := []byte(`{
		"schema_version": 1,
		"generated_at": "2026-04-16T03:00:00Z",
		"bundles": {
			"php:8.4.6:linux:x86_64:nts": "sha256:abc123",
			"ext:redis:6.2.0:8.4:linux:x86_64:nts": "sha256:def456"
		}
	}`)

	lf, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if lf.SchemaVersion != currentSchemaVersion {
		t.Errorf("SchemaVersion = %d, want %d", lf.SchemaVersion, currentSchemaVersion)
	}
	if len(lf.Bundles) != 2 {
		t.Errorf("len(Bundles) = %d, want 2", len(lf.Bundles))
	}
}

func TestParseInvalid(t *testing.T) {
	_, err := Parse([]byte(`not json`))
	if err == nil {
		t.Fatal("Parse() should return error for invalid JSON")
	}
}

func TestParseUnsupportedSchema(t *testing.T) {
	data := []byte(`{"schema_version": 99, "generated_at": "2026-01-01T00:00:00Z", "bundles": {}}`)
	_, err := Parse(data)
	if err == nil {
		t.Fatal("Parse() should return error for unsupported schema version")
	}
}

func TestLookup(t *testing.T) {
	lf := &Lockfile{
		Bundles: map[BundleKey]Entry{
			"php:8.4.6:linux:x86_64:nts": {Digest: "sha256:abc123"},
		},
	}

	d, ok := lf.Lookup("php:8.4.6:linux:x86_64:nts")
	if !ok || d != "sha256:abc123" {
		t.Errorf("Lookup() = (%q, %v), want (sha256:abc123, true)", d, ok)
	}

	_, ok = lf.Lookup("php:8.3.0:linux:x86_64:nts")
	if ok {
		t.Error("Lookup() should return false for missing key")
	}
}

func TestPHPBundleKey(t *testing.T) {
	key := PHPBundleKey("8.4.6", "linux", "x86_64", "nts")
	want := BundleKey("php:8.4.6:linux:x86_64:nts")
	if key != want {
		t.Errorf("PHPBundleKey() = %q, want %q", key, want)
	}
}

func TestExtBundleKey(t *testing.T) {
	key := ExtBundleKey("redis", "6.2.0", "8.4", "linux", "x86_64", "nts")
	want := BundleKey("ext:redis:6.2.0:8.4:linux:x86_64:nts")
	if key != want {
		t.Errorf("ExtBundleKey() = %q, want %q", key, want)
	}
}

func TestWriteAndParseRoundTrip(t *testing.T) {
	lf := &Lockfile{
		SchemaVersion: currentSchemaVersion,
		GeneratedAt:   time.Date(2026, 4, 16, 3, 0, 0, 0, time.UTC),
		Bundles: map[BundleKey]Entry{
			"php:8.4.6:linux:x86_64:nts": {Digest: "sha256:abc123"},
		},
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "bundles.lock")

	if err := lf.Write(path); err != nil {
		t.Fatalf("Write() error = %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}

	lf2, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	d, ok := lf2.Lookup("php:8.4.6:linux:x86_64:nts")
	if !ok || d != "sha256:abc123" {
		t.Errorf("round-trip failed: got (%q, %v)", d, ok)
	}
}

func TestParseV1Upgrades(t *testing.T) {
	v1 := []byte(`{
		"schema_version": 1,
		"generated_at": "2026-04-17T08:53:34Z",
		"bundles": {
			"php:8.4:linux:x86_64:nts": "sha256:abc"
		}
	}`)
	lf, err := Parse(v1)
	if err != nil {
		t.Fatalf("Parse v1: %v", err)
	}
	entry, ok := lf.LookupEntry("php:8.4:linux:x86_64:nts")
	if !ok {
		t.Fatal("entry missing")
	}
	if entry.Digest != "sha256:abc" {
		t.Errorf("digest = %q, want sha256:abc", entry.Digest)
	}
	if entry.SpecHash != "" {
		t.Errorf("spec_hash = %q, want empty (grandfathered)", entry.SpecHash)
	}
}

func TestParseV2RoundTrip(t *testing.T) {
	v2 := []byte(`{
		"schema_version": 2,
		"generated_at": "2026-04-17T08:53:34Z",
		"bundles": {
			"php:8.4:linux:x86_64:nts": {"digest": "sha256:abc", "spec_hash": "sha256:def"}
		}
	}`)
	lf, err := Parse(v2)
	if err != nil {
		t.Fatalf("Parse v2: %v", err)
	}
	entry, _ := lf.LookupEntry("php:8.4:linux:x86_64:nts")
	if entry.Digest != "sha256:abc" || entry.SpecHash != "sha256:def" {
		t.Errorf("entry = %+v", entry)
	}
}

func TestWriteEmitsV2(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bundles.lock")
	lf := &Lockfile{
		SchemaVersion: currentSchemaVersion,
		Bundles:       map[BundleKey]Entry{"k": {Digest: "sha256:abc", SpecHash: "sha256:def"}},
	}
	if err := lf.Write(path); err != nil {
		t.Fatalf("Write: %v", err)
	}
	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), `"schema_version": 2`) {
		t.Errorf("write did not emit v2; got:\n%s", data)
	}
	if !strings.Contains(string(data), `"spec_hash"`) {
		t.Errorf("write did not emit spec_hash field; got:\n%s", data)
	}
}

func TestParseSchemaVersionTooNew(t *testing.T) {
	_, err := Parse([]byte(`{"schema_version": 99, "bundles": {}}`))
	if err == nil {
		t.Fatal("expected error on schema v99")
	}
	if !strings.Contains(err.Error(), "schema version") {
		t.Errorf("error should mention schema version; got %v", err)
	}
}

func TestLookupStringCompat(t *testing.T) {
	lf := &Lockfile{
		SchemaVersion: currentSchemaVersion,
		Bundles:       map[BundleKey]Entry{"k": {Digest: "sha256:abc", SpecHash: "sha256:def"}},
	}
	d, ok := lf.Lookup("k")
	if !ok || d != "sha256:abc" {
		t.Errorf("Lookup(k) = %q,%v; want sha256:abc,true", d, ok)
	}
}
