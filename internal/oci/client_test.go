package oci

import (
	"context"
	"testing"
)

func TestNewClient(t *testing.T) {
	c, err := NewClient("ghcr.io/buildrush", "fake-token")
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	if c.registryURI != "ghcr.io/buildrush" {
		t.Errorf("registryURI = %q, want ghcr.io/buildrush", c.registryURI)
	}
}

func TestFetchAllEmpty(t *testing.T) {
	c, _ := NewClient("ghcr.io/buildrush", "fake-token")
	results, err := c.FetchAll(context.Background(), nil)
	if err != nil {
		t.Fatalf("FetchAll(nil) error = %v", err)
	}
	if len(results) != 0 {
		t.Errorf("FetchAll(nil) should return empty slice, got %d items", len(results))
	}
}

func TestParseMetaJSON_HappyPath(t *testing.T) {
	raw := []byte(`{"schema_version":2,"kind":"php-core","build_timestamp":"2026-04-20T00:00:00Z","digest":"sha256:aaa","builder_versions":{"gcc":"x","autoconf":"y"}}`)
	m, err := parseMetaJSON(raw)
	if err != nil {
		t.Fatalf("parseMetaJSON: %v", err)
	}
	if m.SchemaVersion != 2 {
		t.Fatalf("SchemaVersion = %d, want 2", m.SchemaVersion)
	}
	if m.Kind != "php-core" {
		t.Fatalf("Kind = %q, want %q", m.Kind, "php-core")
	}
}

func TestParseMetaJSON_MissingSchemaVersion_DefaultsToOne(t *testing.T) {
	// Legacy sidecar without schema_version — must be accepted as version 1
	// so pre-slice bundles referenced by released lockfiles stay valid.
	raw := []byte(`{"build_timestamp":"2026-04-16T00:00:00Z","digest":"sha256:bbb"}`)
	m, err := parseMetaJSON(raw)
	if err != nil {
		t.Fatalf("parseMetaJSON: %v", err)
	}
	if m.SchemaVersion != 1 {
		t.Fatalf("SchemaVersion = %d, want 1 (permissive default)", m.SchemaVersion)
	}
}

func TestParseMetaJSON_MalformedJSON_ReturnsError(t *testing.T) {
	if _, err := parseMetaJSON([]byte("{not json")); err == nil {
		t.Fatal("expected error on malformed JSON, got nil")
	}
}

func TestNewClient_AcceptsOciLayoutURI(t *testing.T) {
	c, err := NewClient("oci-layout:/tmp/nonexistent-oci-layout", "")
	if err != nil {
		t.Fatalf("NewClient(oci-layout:) err = %v, want nil", err)
	}
	if c == nil {
		t.Fatal("NewClient returned nil")
	}
	// FetchAll(nil) should return (nil, nil) on any backend — verifies the
	// delegation path is wired up without needing a real bundle on disk.
	results, err := c.FetchAll(context.Background(), nil)
	if err != nil {
		t.Fatalf("FetchAll(nil) on layout-backed client: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("FetchAll(nil) returned %d results, want 0", len(results))
	}
}
