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
	if c.registry != "ghcr.io/buildrush" {
		t.Errorf("registry = %q, want ghcr.io/buildrush", c.registry)
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
