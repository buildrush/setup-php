package registry

import (
	"strings"
	"testing"
)

func TestOpen_RemoteSchemeReturnsRemoteStore(t *testing.T) {
	s, err := Open("ghcr.io/buildrush")
	if err != nil {
		t.Fatalf("Open(ghcr.io/buildrush) err = %v, want nil", err)
	}
	if s == nil {
		t.Fatal("Open returned nil store")
	}
	if got := s.Kind(); got != "remote" {
		t.Errorf("Kind() = %q, want %q", got, "remote")
	}
}

func TestOpen_LayoutSchemeReturnsLayoutStore(t *testing.T) {
	s, err := Open("oci-layout:/tmp/nonexistent")
	if err != nil {
		t.Fatalf("Open(oci-layout:...) err = %v, want nil", err)
	}
	if got := s.Kind(); got != "layout" {
		t.Errorf("Kind() = %q, want %q", got, "layout")
	}
}

func TestOpen_EmptyURIIsError(t *testing.T) {
	if _, err := Open(""); err == nil {
		t.Fatal("Open(\"\") want error, got nil")
	}
}

func TestOpen_UnknownSchemeIsError(t *testing.T) {
	_, err := Open("fluffy-clouds:whatever")
	if err == nil {
		t.Fatal("want error for unknown scheme, got nil")
	}
	if !strings.Contains(err.Error(), "scheme") {
		t.Errorf("error = %q, want it to mention \"scheme\"", err)
	}
}

func TestRefString_Remote(t *testing.T) {
	r := Ref{Name: "php-core", Digest: "sha256:abc"}
	if got := r.String(); got != "php-core@sha256:abc" {
		t.Errorf("Ref.String() = %q, want %q", got, "php-core@sha256:abc")
	}
}

func TestRefString_EmptyDigest(t *testing.T) {
	r := Ref{Name: "php-core"}
	if got := r.String(); got != "php-core" {
		t.Errorf("Ref.String() = %q, want %q", got, "php-core")
	}
}
