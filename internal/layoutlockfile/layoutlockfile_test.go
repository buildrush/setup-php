package layoutlockfile_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/buildrush/setup-php/internal/layoutlockfile"
	"github.com/buildrush/setup-php/internal/registry"
)

func TestWriteSynthesized(t *testing.T) {
	ctx := context.Background()
	layoutDir := filepath.Join(t.TempDir(), "layout")
	layoutURI := "oci-layout:" + layoutDir

	// Open a fresh layout store and push two annotated manifests.
	store, err := registry.Open(layoutURI)
	if err != nil {
		t.Fatalf("registry.Open: %v", err)
	}

	bundles := []registry.Annotations{
		{
			BundleKey:  "php:8.4:linux:x86_64:nts",
			BundleName: "php-core-8.4-nts",
			SpecHash:   "sha256:aabbccdd",
		},
		{
			BundleKey:  "ext:redis:6.2.0:8.4-nts:linux:x86_64",
			BundleName: "php-ext-redis",
			SpecHash:   "",
		},
	}

	for i, ann := range bundles {
		ref := registry.Ref{Name: ann.BundleName}
		body := bytes.NewReader([]byte{byte(i + 1)})
		if err := store.Push(ctx, ref, body, nil, ann); err != nil {
			t.Fatalf("Push[%d]: %v", i, err)
		}
	}

	// Obtain expected digests from ListKeyed for later assertion.
	type lister interface {
		ListKeyed(ctx context.Context) ([]registry.KeyedRef, error)
	}
	ls, ok := store.(lister)
	if !ok {
		t.Fatal("store does not implement ListKeyed")
	}
	refs, err := ls.ListKeyed(ctx)
	if err != nil {
		t.Fatalf("ListKeyed: %v", err)
	}
	expectedDigest := make(map[string]string, len(refs))
	expectedSpecHash := make(map[string]string, len(refs))
	for _, r := range refs {
		expectedDigest[r.Key] = r.Digest
		expectedSpecHash[r.Key] = r.SpecHash
	}

	// Call WriteSynthesized.
	outPath := filepath.Join(t.TempDir(), "sub", "bundles-override.lock")
	if err := layoutlockfile.WriteSynthesized(layoutURI, outPath); err != nil {
		t.Fatalf("WriteSynthesized: %v", err)
	}

	// Read and parse the output JSON.
	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	var doc struct {
		SchemaVersion int                          `json:"schema_version"`
		GeneratedAt   string                       `json:"generated_at"`
		Bundles       map[string]map[string]string `json:"bundles"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	// Assert schema_version == 2.
	if doc.SchemaVersion != 2 {
		t.Errorf("schema_version = %d; want 2", doc.SchemaVersion)
	}

	// Assert generated_at is non-empty.
	if doc.GeneratedAt == "" {
		t.Error("generated_at is empty")
	}

	// Assert both bundle keys are present with correct digests.
	for _, ann := range bundles {
		key := ann.BundleKey
		entry, ok := doc.Bundles[key]
		if !ok {
			t.Errorf("bundles[%q] missing", key)
			continue
		}
		if got, want := entry["digest"], expectedDigest[key]; got != want {
			t.Errorf("bundles[%q].digest = %q; want %q", key, got, want)
		}
		if ann.SpecHash != "" {
			if got, want := entry["spec_hash"], expectedSpecHash[key]; got != want {
				t.Errorf("bundles[%q].spec_hash = %q; want %q", key, got, want)
			}
		} else {
			// spec_hash must be absent when empty.
			if _, present := entry["spec_hash"]; present {
				t.Errorf("bundles[%q].spec_hash present but should be omitted", key)
			}
		}
	}
}

func TestMainCLI_MissingFlags(t *testing.T) {
	// Both --layout and --out are required; missing either should return an error.
	if err := layoutlockfile.MainCLI([]string{}); err == nil {
		t.Error("expected error for missing flags, got nil")
	}
}

func TestMainCLI_OK(t *testing.T) {
	ctx := context.Background()
	layoutDir := filepath.Join(t.TempDir(), "layout")
	layoutURI := "oci-layout:" + layoutDir

	store, err := registry.Open(layoutURI)
	if err != nil {
		t.Fatalf("registry.Open: %v", err)
	}
	ann := registry.Annotations{
		BundleKey:  "php:8.3:linux:aarch64:nts",
		BundleName: "php-core-8.3-nts",
	}
	if err := store.Push(ctx, registry.Ref{Name: ann.BundleName},
		bytes.NewReader([]byte("y")), nil, ann); err != nil {
		t.Fatalf("Push: %v", err)
	}

	outPath := filepath.Join(t.TempDir(), "out.lock")
	if err := layoutlockfile.MainCLI([]string{
		"--layout", layoutURI,
		"--out", outPath,
	}); err != nil {
		t.Fatalf("MainCLI: %v", err)
	}

	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if sv, _ := doc["schema_version"].(float64); int(sv) != 2 {
		t.Errorf("schema_version = %v; want 2", doc["schema_version"])
	}
}
