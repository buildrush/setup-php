// Package registry provides a backend-agnostic abstraction for fetching and
// pushing OCI-style bundles used by setup-php.
//
// Two concrete backends are supported, selected by URI scheme via Open:
//
//   - Remote HTTPS registries (e.g. "ghcr.io/buildrush"): implemented on top
//     of go-containerregistry and used for published bundles.
//   - Local filesystem OCI layouts (e.g. "oci-layout:/path/to/layout"): used
//     for the local-CI smoke pipeline and tests.
//
// The public surface lives in this file (Ref, Meta, Store, Open). Concrete
// store implementations live in layout.go and remote.go.
package registry

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
)

// ErrUnsupported is returned by Store implementations when the caller invokes
// an operation the backend cannot serve (for example, pushing to a read-only
// remote).
var ErrUnsupported = errors.New("registry: operation not supported by this backend")

// Ref identifies a bundle within a Store by its logical Name and, optionally,
// its content-addressed Digest (in the usual "sha256:..." form) or its
// human-readable Tag.
//
// Digest is used by Fetch/Has/LookupBySpec on every backend — those operations
// are content-addressed regardless of publication channel. Tag is a
// publication-time concern: the remote backend needs it for Push because OCI
// registries are tag-addressed for writes (the registry computes the digest
// from the manifest; callers cannot supply one). The layout backend ignores
// Tag because its Push writes by index annotation, not by tag.
type Ref struct {
	Name   string
	Digest string
	Tag    string
}

// String renders the Ref as "name@digest" when a Digest is present,
// "name:tag" when only a Tag is present, or just "name" otherwise. It is
// suitable for logs and error messages; it is not a canonical OCI reference.
func (r Ref) String() string {
	switch {
	case r.Digest != "":
		return r.Name + "@" + r.Digest
	case r.Tag != "":
		return r.Name + ":" + r.Tag
	default:
		return r.Name
	}
}

// Meta describes bundle metadata persisted alongside the payload. Fields are
// kept minimal on purpose; callers that need richer structure layer it on top.
type Meta struct {
	SchemaVersion int    `json:"schema_version"`
	Kind          string `json:"kind"`
}

// Store is the backend-agnostic interface for fetching and pushing bundles.
//
// Kind returns a short identifier for the backend ("remote", "layout") and is
// intended for logs and test assertions only.
//
// Fetch may return a nil *Meta when the backend has no sidecar metadata for
// the ref; callers must tolerate that. Push accepts a nil meta to write a
// bundle without a meta sidecar.
//
// Fetch returns the raw layer blob (compressed on the wire; callers
// decompress as needed). This matches the behavior of internal/oci/client.go
// so that a layout-backed store returns bytes byte-identical to what a
// remote-backed store would return for the same manifest.
type Store interface {
	Kind() string
	Fetch(ctx context.Context, ref Ref) (io.ReadCloser, *Meta, error)
	// Push writes a bundle to the store under the given ref, attaching
	// the supplied Annotations to the manifest. Remote backends that
	// don't support anonymous writes return ErrUnsupported.
	Push(ctx context.Context, ref Ref, body io.Reader, meta *Meta, ann Annotations) error
	Has(ctx context.Context, ref Ref) (bool, error)
	// LookupBySpec finds a manifest whose annotations include both
	// BundleName==name AND SpecHash==specHash. Returns (Ref, true, nil)
	// on hit, (zero, false, nil) when absent, (zero, false, err) on
	// backend error. Used by the build subcommand's cache probe to
	// short-circuit redundant rebuilds.
	//
	// Callers with an `Annotations` value already in scope should pass
	// `ann.BundleName` and `ann.SpecHash` positionally.
	LookupBySpec(ctx context.Context, name, specHash string) (Ref, bool, error)
	ResolveDigest(ctx context.Context, name string) (string, error)
}

// Open dispatches on the given URI and returns a Store.
//
// The accepted forms are:
//
//   - "oci-layout:<path>" — a local filesystem OCI image layout.
//   - a bare host[/path] that "looks like" a remote (the head segment before
//     the first "/" must contain a dot and only [a-zA-Z0-9.-] characters),
//     for example "ghcr.io/buildrush".
//
// Any unrecognised URI form is rejected with an error. The empty string yields
// "registry: empty URI"; other unrecognised forms yield an error whose message
// contains "scheme".
func Open(uri string) (Store, error) {
	if uri == "" {
		return nil, errors.New("registry: empty URI")
	}
	if strings.HasPrefix(uri, "oci-layout:") {
		return openLayout(strings.TrimPrefix(uri, "oci-layout:"))
	}
	if looksLikeRemote(uri) {
		return openRemote(uri)
	}
	return nil, fmt.Errorf("registry: unrecognised scheme in %q", uri)
}

// looksLikeRemote returns true when the URI's head segment (up to the first
// "/") is a plausible registry host: it must contain at least one "." and be
// composed exclusively of ASCII letters, digits, dots, hyphens, and colons
// (colons appear in host[:port] forms like "127.0.0.1:5000" used by the
// in-process test registry).
func looksLikeRemote(uri string) bool {
	head := uri
	if i := strings.IndexByte(uri, '/'); i >= 0 {
		head = uri[:i]
	}
	if head == "" || !strings.ContainsRune(head, '.') {
		return false
	}
	for _, r := range head {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9':
		case r == '.' || r == '-' || r == ':':
		default:
			return false
		}
	}
	return true
}
