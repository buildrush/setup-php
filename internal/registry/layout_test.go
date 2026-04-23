package registry

import (
	"bytes"
	"context"
	"io"
	"path/filepath"
	"testing"
)

func newTestLayoutStore(t *testing.T) *layoutStore {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "layout")
	s, err := openLayout(dir)
	if err != nil {
		t.Fatalf("openLayout: %v", err)
	}
	return s
}

func TestLayoutStore_RoundTrip(t *testing.T) {
	ctx := context.Background()
	s := newTestLayoutStore(t)

	payload := []byte("fake bundle bytes — in reality this would be bundle.tar.zst")
	meta := &Meta{SchemaVersion: 2, Kind: "php-core"}

	// Before Push, Has must return false.
	ref := Ref{Name: "php-core"} // digest filled in by Push
	has, err := s.Has(ctx, ref)
	if err != nil {
		t.Fatalf("Has before Push: %v", err)
	}
	if has {
		t.Fatal("Has returned true on empty layout")
	}

	if err := s.Push(ctx, ref, bytes.NewReader(payload), meta); err != nil {
		t.Fatalf("Push: %v", err)
	}

	// Walk the index to find the just-pushed digest, then assert Has and Fetch.
	got, err := s.list(ctx)
	if err != nil {
		t.Fatalf("list after Push: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("list after Push: got %d entries, want 1", len(got))
	}
	stored := got[0]
	if stored.Name != "php-core" {
		t.Fatalf("stored.Name = %q, want %q", stored.Name, "php-core")
	}
	if stored.Digest == "" {
		t.Fatal("stored.Digest empty")
	}

	has, err = s.Has(ctx, stored)
	if err != nil {
		t.Fatalf("Has after Push: %v", err)
	}
	if !has {
		t.Fatal("Has returned false after Push")
	}

	rc, metaOut, err := s.Fetch(ctx, stored)
	if err != nil {
		t.Fatalf("Fetch after Push: %v", err)
	}
	defer rc.Close()
	gotBytes, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("read bundle bytes: %v", err)
	}
	if !bytes.Equal(gotBytes, payload) {
		t.Fatalf("bundle bytes mismatch: got %q, want %q", gotBytes, payload)
	}
	if metaOut == nil || metaOut.SchemaVersion != 2 || metaOut.Kind != "php-core" {
		t.Fatalf("meta mismatch: got %+v, want {SchemaVersion:2 Kind:php-core}", metaOut)
	}
}

func TestLayoutStore_FetchMissingRef_Errors(t *testing.T) {
	ctx := context.Background()
	s := newTestLayoutStore(t)
	_, _, err := s.Fetch(ctx, Ref{Name: "php-core", Digest: "sha256:deadbeef"})
	if err == nil {
		t.Fatal("Fetch on empty layout: want error, got nil")
	}
}

func TestLayoutStore_TolerateMissingMeta(t *testing.T) {
	// A bundle pushed without a meta sidecar must Fetch back with a
	// default Meta{SchemaVersion:1} — matching the legacy behaviour the
	// runtime already tolerates (see internal/oci/client.go Fetch path).
	ctx := context.Background()
	s := newTestLayoutStore(t)
	payload := []byte("legacy-bundle")
	if err := s.Push(ctx, Ref{Name: "php-core"}, bytes.NewReader(payload), nil); err != nil {
		t.Fatalf("Push nil meta: %v", err)
	}
	got, err := s.list(ctx)
	if err != nil || len(got) != 1 {
		t.Fatalf("list: %v / %d", err, len(got))
	}
	_, meta, err := s.Fetch(ctx, got[0])
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if meta == nil || meta.SchemaVersion != 1 {
		t.Fatalf("meta = %+v, want {SchemaVersion:1}", meta)
	}
}
