package oci

import (
	"context"
	"crypto/sha256"
	"fmt"
	"testing"
)

func TestDigestVerification(t *testing.T) {
	data := []byte("test bundle content")
	h := sha256.Sum256(data)
	digest := fmt.Sprintf("sha256:%x", h)

	if err := verifyDigest(data, digest); err != nil {
		t.Fatalf("verifyDigest() should pass for matching digest, got %v", err)
	}
}

func TestDigestVerificationMismatch(t *testing.T) {
	data := []byte("test bundle content")
	if err := verifyDigest(data, "sha256:0000000000000000"); err == nil {
		t.Fatal("verifyDigest() should fail for mismatching digest")
	}
}

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
