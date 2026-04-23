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

func TestRefString_WithDigest(t *testing.T) {
	r := Ref{Name: "php-core", Digest: "sha256:abc"}
	if got := r.String(); got != "php-core@sha256:abc" {
		t.Errorf("Ref.String() = %q, want %q", got, "php-core@sha256:abc")
	}
}

func TestOpen_URIFormDispatch(t *testing.T) {
	cases := []struct {
		name             string
		uri              string
		wantKind         string // empty when an error is expected
		wantErrSubstring string // empty when success is expected
	}{
		{
			name:             "dotless host is not remote",
			uri:              "localhost/foo",
			wantErrSubstring: "scheme",
		},
		{
			name:     "host only succeeds as remote",
			uri:      "ghcr.io",
			wantKind: "remote",
		},
		{
			name:     "uppercase host succeeds as remote",
			uri:      "GHCR.IO/foo",
			wantKind: "remote",
		},
		{
			name:             "leading slash has empty head",
			uri:              "/leading-slash",
			wantErrSubstring: "scheme",
		},
		{
			name:     "multi-segment path succeeds as remote",
			uri:      "ghcr.io/buildrush/extra/path/segments",
			wantKind: "remote",
		},
		{
			name:     "underscore in path does not affect head",
			uri:      "ghcr.io/weird_path",
			wantKind: "remote",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s, err := Open(tc.uri)
			if tc.wantErrSubstring != "" {
				if err == nil {
					t.Fatalf("Open(%q) err = nil, want error containing %q", tc.uri, tc.wantErrSubstring)
				}
				if !strings.Contains(err.Error(), tc.wantErrSubstring) {
					t.Errorf("Open(%q) err = %q, want substring %q", tc.uri, err, tc.wantErrSubstring)
				}
				return
			}
			if err != nil {
				t.Fatalf("Open(%q) err = %v, want nil", tc.uri, err)
			}
			if s == nil {
				t.Fatalf("Open(%q) returned nil store", tc.uri)
			}
			if got := s.Kind(); got != tc.wantKind {
				t.Errorf("Open(%q).Kind() = %q, want %q", tc.uri, got, tc.wantKind)
			}
		})
	}
}

func TestRefString_EmptyDigest(t *testing.T) {
	r := Ref{Name: "php-core"}
	if got := r.String(); got != "php-core" {
		t.Errorf("Ref.String() = %q, want %q", got, "php-core")
	}
}
