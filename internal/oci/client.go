package oci

import (
	"context"
	"fmt"
	"io"
	"os"
	"sync"

	"github.com/buildrush/setup-php/internal/registry"
)

// Client is a thin facade over an internal/registry.Store. Its public surface
// is preserved for backwards compatibility with the pre-refactor call sites
// in cmd/phpup, cmd/planner, and cmd/lockfile-update. The facade itself is
// slated for deletion by end of PR 3 of the local+CI unification rollout.
type Client struct {
	registryURI string
	token       string
	store       registry.Store
}

// FetchResult is returned by Fetch/FetchAll. Data is the raw (compressed)
// bundle layer bytes; callers decompress as needed.
type FetchResult struct {
	Key      string
	Digest   string
	Data     []byte
	Metadata Metadata
}

// Metadata mirrors registry.Meta; kept here for backwards compatibility with
// callers that import oci.Metadata directly. Will be removed when the facade
// is deleted (scheduled for end of PR 3 of the local+CI unification rollout).
type Metadata struct {
	SchemaVersion int    `json:"schema_version"`
	Kind          string `json:"kind"`
}

// ResolvedBundle identifies a bundle to fetch. Kind selects the OCI name
// prefix ("php-core", "php-ext-<name>", "php-tool-<name>").
type ResolvedBundle struct {
	Key     string
	Digest  string
	Name    string
	Version string
	Kind    string
}

// NewClient preserves the legacy constructor shape. The registryURI argument
// may be any URI accepted by registry.Open — including the new
// "oci-layout:<path>" form added in PR 1 — so callers that switch to an
// oci-layout source do not need a different entry point.
//
// Token is honoured by the remote backend via env lookup
// (INPUT_GITHUB-TOKEN / GITHUB_TOKEN). If the caller passes an explicit
// token and the env is empty, we thread it through by setting
// INPUT_GITHUB-TOKEN for this process. This matches the behaviour of the
// pre-refactor code, which built an authn.Basic directly from the token
// arg. No-op for the layout backend.
func NewClient(registryURI, token string) (*Client, error) {
	if token != "" {
		setIfUnset("INPUT_GITHUB-TOKEN", token)
	}
	s, err := registry.Open(registryURI)
	if err != nil {
		return nil, fmt.Errorf("oci.NewClient: %w", err)
	}
	return &Client{registryURI: registryURI, token: token, store: s}, nil
}

func setIfUnset(key, value string) {
	if os.Getenv(key) == "" {
		_ = os.Setenv(key, value)
	}
}

// FetchAll concurrently fetches every bundle in the slice. On the first error
// from any goroutine it returns a wrapped error tagged with the failing
// bundle's Key; successful sibling fetches are discarded.
func (c *Client) FetchAll(ctx context.Context, bundles []ResolvedBundle) ([]FetchResult, error) {
	if len(bundles) == 0 {
		return nil, nil
	}
	results := make([]FetchResult, len(bundles))
	errs := make([]error, len(bundles))
	var wg sync.WaitGroup
	for i := range bundles {
		wg.Add(1)
		go func(idx int, b *ResolvedBundle) {
			defer wg.Done()
			r, err := c.Fetch(ctx, b)
			if err != nil {
				errs[idx] = err
				return
			}
			results[idx] = *r
		}(i, &bundles[i])
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			return nil, fmt.Errorf("fetch %s: %w", bundles[i].Key, err)
		}
	}
	return results, nil
}

// Fetch delegates to the underlying Store, mapping the ResolvedBundle to a
// registry.Ref by kind-prefix, and normalises the returned Meta into a
// package-local Metadata (defaulting SchemaVersion to 1 when absent, matching
// the pre-refactor permissive behaviour for legacy bundles).
func (c *Client) Fetch(ctx context.Context, b *ResolvedBundle) (*FetchResult, error) {
	ref := registry.Ref{Name: ociName(b), Digest: b.Digest}
	rc, meta, err := c.store.Fetch(ctx, ref)
	if err != nil {
		return nil, fmt.Errorf("fetch %s: %w", b.Key, err)
	}
	defer func() { _ = rc.Close() }()
	data, err := io.ReadAll(rc)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", b.Key, err)
	}
	out := &FetchResult{
		Key:    b.Key,
		Digest: b.Digest,
		Data:   data,
	}
	if meta != nil {
		out.Metadata = Metadata{SchemaVersion: meta.SchemaVersion, Kind: meta.Kind}
	}
	if out.Metadata.SchemaVersion == 0 {
		out.Metadata.SchemaVersion = 1
	}
	return out, nil
}

// Exists reports whether the given ref (optionally fully-qualified with the
// registry host prefix) and digest are present in the backing Store.
func (c *Client) Exists(ctx context.Context, ref, digest string) (bool, error) {
	return c.store.Has(ctx, registry.Ref{Name: stripHost(ref, c.registryURI), Digest: digest})
}

// ResolveDigest looks up the OCI manifest digest for the given tagged or
// digest-form reference. Returns "sha256:..." on success. Layout-backed
// clients surface an ErrUnsupported-style error from the store.
func (c *Client) ResolveDigest(ctx context.Context, ref string) (string, error) {
	return c.store.ResolveDigest(ctx, ref)
}

// ociName maps a ResolvedBundle to the OCI name within the Store.
// Mirrors the prior bundleRef() behaviour:
//
//   - "php"   → "php-core"
//   - "ext"   → "php-ext-<b.Name>"
//   - "tool"  → "php-tool-<b.Name>"
//   - other   → b.Name (defensive passthrough)
func ociName(b *ResolvedBundle) string {
	switch b.Kind {
	case "php":
		return "php-core"
	case "ext":
		return "php-ext-" + b.Name
	case "tool":
		return "php-tool-" + b.Name
	default:
		return b.Name
	}
}

// stripHost removes a leading "<registryURI>/" from ref so Exists() can
// accept either a fully-qualified reference or a bare name. Mirrors the
// prior client's tolerance for both forms.
func stripHost(ref, root string) string {
	if root == "" {
		return ref
	}
	if len(ref) > len(root)+1 && ref[:len(root)] == root && ref[len(root)] == '/' {
		return ref[len(root)+1:]
	}
	return ref
}
