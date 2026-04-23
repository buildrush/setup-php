package registry

import (
	"context"
	"errors"
	"io"
)

// remoteStore is the HTTPS-registry backed Store.
//
// The real implementation (go-containerregistry remote.Image, auth, retries)
// lands in Task 3; this file provides the constructor and Kind() so Open can
// dispatch, plus Push gated on ErrUnsupported since published remotes are
// read-only from the action's perspective.
type remoteStore struct {
	root string
}

func openRemote(uri string) (*remoteStore, error) {
	return &remoteStore{root: uri}, nil
}

func (s *remoteStore) Kind() string { return "remote" }

func (s *remoteStore) Fetch(_ context.Context, _ Ref) (io.ReadCloser, *Meta, error) {
	return nil, nil, errors.New("remote.Fetch: implemented in Task 3")
}

func (s *remoteStore) Push(_ context.Context, _ Ref, _ io.Reader, _ *Meta) error {
	return ErrUnsupported
}

func (s *remoteStore) Has(_ context.Context, _ Ref) (bool, error) {
	return false, errors.New("remote.Has: implemented in Task 3")
}

func (s *remoteStore) ResolveDigest(_ context.Context, _ string) (string, error) {
	return "", errors.New("remote.ResolveDigest: implemented in Task 3")
}
