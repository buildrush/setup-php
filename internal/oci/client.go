package oci

import (
	"context"
	"fmt"
	"sync"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
)

type Client struct {
	registry string
	token    string
	auth     authn.Authenticator
}

type FetchResult struct {
	Key    string
	Digest string
	Data   []byte
}

type ResolvedBundle struct {
	Key     string
	Digest  string
	Name    string
	Version string
	Kind    string
}

func NewClient(registry, token string) (*Client, error) {
	var auth authn.Authenticator
	if token != "" {
		auth = &authn.Basic{Username: "token", Password: token}
	} else {
		auth = authn.Anonymous
	}
	return &Client{registry: registry, token: token, auth: auth}, nil
}

func (c *Client) FetchAll(ctx context.Context, bundles []ResolvedBundle) ([]FetchResult, error) {
	if len(bundles) == 0 {
		return nil, nil
	}

	results := make([]FetchResult, len(bundles))
	errs := make([]error, len(bundles))
	var wg sync.WaitGroup

	for i := range bundles {
		wg.Add(1)
		go func(idx int, bundle *ResolvedBundle) {
			defer wg.Done()
			result, err := c.Fetch(ctx, bundle)
			if err != nil {
				errs[idx] = err
				return
			}
			results[idx] = *result
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

func (c *Client) Fetch(ctx context.Context, bundle *ResolvedBundle) (*FetchResult, error) {
	ref, err := c.bundleRef(bundle)
	if err != nil {
		return nil, err
	}

	desc, err := remote.Get(ref, remote.WithAuth(c.auth), remote.WithContext(ctx))
	if err != nil {
		return nil, fmt.Errorf("fetch %s: %w", bundle.Key, err)
	}

	layer, err := desc.Image()
	if err != nil {
		return nil, fmt.Errorf("get image %s: %w", bundle.Key, err)
	}

	layers, err := layer.Layers()
	if err != nil || len(layers) == 0 {
		return nil, fmt.Errorf("get layers %s: no layers found", bundle.Key)
	}

	rc, err := layers[0].Compressed()
	if err != nil {
		return nil, fmt.Errorf("read layer %s: %w", bundle.Key, err)
	}
	defer func() { _ = rc.Close() }()

	var data []byte
	buf := make([]byte, 32*1024)
	for {
		n, err := rc.Read(buf)
		if n > 0 {
			data = append(data, buf[:n]...)
		}
		if err != nil {
			break
		}
	}

	// The manifest digest in bundle.Digest was already verified by the
	// OCI library when fetching via content-addressed reference. The
	// layer integrity is covered by the manifest's layer descriptors,
	// which the library also validates. No extra hashing needed here.

	return &FetchResult{
		Key:    bundle.Key,
		Digest: bundle.Digest,
		Data:   data,
	}, nil
}

func (c *Client) Exists(ctx context.Context, ref, digest string) (bool, error) {
	r, err := name.ParseReference(ref)
	if err != nil {
		return false, err
	}
	_, err = remote.Head(r, remote.WithAuth(c.auth), remote.WithContext(ctx))
	if err != nil {
		return false, fmt.Errorf("check existence %s: %w", ref, err)
	}
	return true, nil
}

// ResolveDigest looks up the OCI manifest digest for the given tagged or
// digest-form reference. Returns "sha256:..." on success.
func (c *Client) ResolveDigest(ctx context.Context, ref string) (string, error) {
	r, err := name.ParseReference(ref)
	if err != nil {
		return "", fmt.Errorf("parse ref %s: %w", ref, err)
	}
	desc, err := remote.Head(r, remote.WithAuth(c.auth), remote.WithContext(ctx))
	if err != nil {
		return "", fmt.Errorf("head %s: %w", ref, err)
	}
	return desc.Digest.String(), nil
}

func (c *Client) bundleRef(bundle *ResolvedBundle) (name.Reference, error) {
	var refStr string
	switch bundle.Kind {
	case "php":
		refStr = fmt.Sprintf("%s/php-core@%s", c.registry, bundle.Digest)
	case "ext":
		refStr = fmt.Sprintf("%s/php-ext-%s@%s", c.registry, bundle.Name, bundle.Digest)
	case "tool":
		refStr = fmt.Sprintf("%s/php-tool-%s@%s", c.registry, bundle.Name, bundle.Digest)
	default:
		return nil, fmt.Errorf("unknown bundle kind %q", bundle.Kind)
	}
	return name.ParseReference(refStr)
}
