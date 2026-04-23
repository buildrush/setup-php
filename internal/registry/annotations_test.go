package registry

import (
	"bytes"
	"context"
	"path/filepath"
	"testing"
)

func TestAnnotations_AsMap_OmitsEmpty(t *testing.T) {
	if got := (Annotations{}).asMap(); len(got) != 0 {
		t.Errorf("empty Annotations produced non-empty map: %v", got)
	}
	got := Annotations{BundleName: "x", SpecHash: "sha256:abc"}.asMap()
	if got[annotationBundleName] != "x" || got[annotationSpecHash] != "sha256:abc" {
		t.Errorf("asMap = %v", got)
	}
	// BundleName only
	got2 := Annotations{BundleName: "x"}.asMap()
	if _, ok := got2[annotationSpecHash]; ok {
		t.Errorf("asMap should omit empty SpecHash: %v", got2)
	}
	// SpecHash only
	got3 := Annotations{SpecHash: "sha256:abc"}.asMap()
	if _, ok := got3[annotationBundleName]; ok {
		t.Errorf("asMap should omit empty BundleName: %v", got3)
	}
}

// pushForTest pushes a bundle with the given (name, spec-hash) pair via
// the new Annotations API. It is intentionally minimal so the cache-probe
// tests below read as declarative fixtures rather than re-exercising the
// full Push surface.
func pushForTest(t *testing.T, s *layoutStore, name, specHash string, payload []byte) {
	t.Helper()
	err := s.Push(context.Background(), Ref{Name: name}, bytes.NewReader(payload), nil,
		Annotations{BundleName: name, SpecHash: specHash})
	if err != nil {
		t.Fatalf("Push: %v", err)
	}
}

func TestLayoutStore_LookupBySpec_Hit(t *testing.T) {
	s, err := openLayout(filepath.Join(t.TempDir(), "layout"))
	if err != nil {
		t.Fatalf("openLayout: %v", err)
	}
	pushForTest(t, s, "php-core", "sha256:abc", []byte("bundle"))
	ref, hit, err := s.LookupBySpec(context.Background(), "php-core", "sha256:abc")
	if err != nil {
		t.Fatalf("LookupBySpec: %v", err)
	}
	if !hit {
		t.Fatal("want hit, got miss")
	}
	if ref.Name != "php-core" || ref.Digest == "" {
		t.Fatalf("ref = %+v", ref)
	}
}

func TestLayoutStore_LookupBySpec_Miss_EmptyLayout(t *testing.T) {
	s, err := openLayout(filepath.Join(t.TempDir(), "layout"))
	if err != nil {
		t.Fatalf("openLayout: %v", err)
	}
	ref, hit, err := s.LookupBySpec(context.Background(), "php-core", "sha256:abc")
	if err != nil {
		t.Fatalf("LookupBySpec on empty layout: err = %v, want nil", err)
	}
	if hit {
		t.Fatal("want miss, got hit")
	}
	if ref.Name != "" || ref.Digest != "" {
		t.Fatalf("ref should be zero: %+v", ref)
	}
}

func TestLayoutStore_LookupBySpec_WrongNameIsMiss(t *testing.T) {
	s, err := openLayout(filepath.Join(t.TempDir(), "layout"))
	if err != nil {
		t.Fatalf("openLayout: %v", err)
	}
	pushForTest(t, s, "php-core", "sha256:abc", []byte("x"))
	_, hit, err := s.LookupBySpec(context.Background(), "php-ext-redis", "sha256:abc")
	if err != nil {
		t.Fatalf("LookupBySpec: %v", err)
	}
	if hit {
		t.Fatal("lookup with wrong name should miss")
	}
}

func TestLayoutStore_LookupBySpec_WrongHashIsMiss(t *testing.T) {
	s, err := openLayout(filepath.Join(t.TempDir(), "layout"))
	if err != nil {
		t.Fatalf("openLayout: %v", err)
	}
	pushForTest(t, s, "php-core", "sha256:abc", []byte("x"))
	_, hit, err := s.LookupBySpec(context.Background(), "php-core", "sha256:xyz")
	if err != nil {
		t.Fatalf("LookupBySpec: %v", err)
	}
	if hit {
		t.Fatal("lookup with wrong hash should miss")
	}
}

func TestRemoteStore_LookupBySpec_AlwaysMiss(t *testing.T) {
	host := startTestRegistry(t)
	s, err := Open(host + "/buildrush")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	_, hit, err := s.LookupBySpec(context.Background(), "php-core", "sha256:abc")
	if err != nil {
		t.Fatalf("LookupBySpec: %v", err)
	}
	if hit {
		t.Fatal("remote LookupBySpec should always miss in PR 1/2 scope")
	}
}
