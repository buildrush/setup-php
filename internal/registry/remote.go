package registry

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/remote/transport"
)

// remoteStore is the HTTPS-registry backed Store. It wraps
// go-containerregistry/pkg/v1/remote for Fetch / Has / ResolveDigest; Push
// returns ErrUnsupported because remote pushes land with the `phpup build`
// subcommand in a later PR.
type remoteStore struct {
	base string
	auth authn.Authenticator
}

// openRemote resolves auth from the environment (INPUT_GITHUB-TOKEN first,
// then GITHUB_TOKEN; anonymous if both are empty) and returns the store.
func openRemote(uri string) (*remoteStore, error) {
	token := os.Getenv("INPUT_GITHUB-TOKEN")
	if token == "" {
		token = os.Getenv("GITHUB_TOKEN")
	}
	var auth authn.Authenticator
	if token != "" {
		auth = &authn.Basic{Username: "token", Password: token}
	} else {
		auth = authn.Anonymous
	}
	return &remoteStore{base: uri, auth: auth}, nil
}

func (s *remoteStore) Kind() string { return "remote" }

// refFor constructs a name.Reference of the form "<base>/<name>@<digest>".
// Both the ref name and digest are required; a missing value is an error
// because the remote backend is strictly digest-addressed.
func (s *remoteStore) refFor(r Ref) (name.Reference, error) {
	if r.Name == "" {
		return nil, errors.New("remote: ref.Name required")
	}
	if r.Digest == "" {
		return nil, errors.New("remote: ref.Digest required")
	}
	return name.ParseReference(fmt.Sprintf("%s/%s@%s", s.base, r.Name, r.Digest))
}

func (s *remoteStore) Has(ctx context.Context, ref Ref) (bool, error) {
	r, err := s.refFor(ref)
	if err != nil {
		return false, fmt.Errorf("remote.Has %s: %w", ref, err)
	}
	if _, err := remote.Head(r, remote.WithAuth(s.auth), remote.WithContext(ctx)); err != nil {
		var terr *transport.Error
		if errors.As(err, &terr) && terr.StatusCode == 404 {
			return false, nil
		}
		return false, fmt.Errorf("remote.Has %s: %w", ref, err)
	}
	return true, nil
}

func (s *remoteStore) Fetch(ctx context.Context, ref Ref) (io.ReadCloser, *Meta, error) {
	r, err := s.refFor(ref)
	if err != nil {
		return nil, nil, fmt.Errorf("remote.Fetch %s: %w", ref, err)
	}
	desc, err := remote.Get(r, remote.WithAuth(s.auth), remote.WithContext(ctx))
	if err != nil {
		return nil, nil, fmt.Errorf("remote.Fetch %s: %w", ref, err)
	}
	img, err := desc.Image()
	if err != nil {
		return nil, nil, fmt.Errorf("remote.Fetch %s: load image: %w", ref, err)
	}
	layers, err := img.Layers()
	if err != nil {
		return nil, nil, fmt.Errorf("remote.Fetch %s: list layers: %w", ref, err)
	}
	if len(layers) == 0 {
		return nil, nil, fmt.Errorf("remote.Fetch %s: image has no layers", ref)
	}

	// Use Compressed() so the returned bytes match what the layout backend
	// returns for the same manifest — both surfaces are "raw layer blob".
	bundle, err := layers[0].Compressed()
	if err != nil {
		return nil, nil, fmt.Errorf("remote.Fetch %s: open bundle layer: %w", ref, err)
	}

	meta := &Meta{SchemaVersion: 1}
	if len(layers) >= 2 {
		mrc, err := layers[1].Compressed()
		if err != nil {
			_ = bundle.Close()
			return nil, nil, fmt.Errorf("remote.Fetch %s: open meta layer: %w", ref, err)
		}
		metaBytes, err := io.ReadAll(mrc)
		_ = mrc.Close()
		if err != nil {
			_ = bundle.Close()
			return nil, nil, fmt.Errorf("remote.Fetch %s: read meta layer: %w", ref, err)
		}
		parsed := &Meta{}
		if err := json.Unmarshal(metaBytes, parsed); err != nil {
			_ = bundle.Close()
			return nil, nil, fmt.Errorf("remote.Fetch %s: parse meta: %w", ref, err)
		}
		if parsed.SchemaVersion == 0 {
			parsed.SchemaVersion = 1
		}
		meta = parsed
	}
	return bundle, meta, nil
}

// Push is deliberately unsupported in PR 1; remote publication lands with the
// `phpup build` subcommand in a follow-up PR.
func (s *remoteStore) Push(_ context.Context, _ Ref, _ io.Reader, _ *Meta) error {
	return ErrUnsupported
}

func (s *remoteStore) ResolveDigest(ctx context.Context, reference string) (string, error) {
	ref, err := name.ParseReference(reference)
	if err != nil {
		return "", fmt.Errorf("remote.ResolveDigest %q: %w", reference, err)
	}
	desc, err := remote.Head(ref, remote.WithAuth(s.auth), remote.WithContext(ctx))
	if err != nil {
		return "", fmt.Errorf("remote.ResolveDigest %q: %w", reference, err)
	}
	return desc.Digest.String(), nil
}

var _ Store = (*remoteStore)(nil)
