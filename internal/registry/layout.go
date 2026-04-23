package registry

import (
	"context"
	"errors"
	"io"
)

// layoutStore is the filesystem OCI-layout backed Store.
//
// The real implementation (readdir, blob lookup, manifest walk) lands in
// Task 2; this file only provides the constructor and Kind() so that Open
// can dispatch and callers can begin wiring to the interface.
type layoutStore struct {
	root string
}

func openLayout(path string) (*layoutStore, error) {
	if path == "" {
		return nil, errors.New("registry: oci-layout URI requires a path")
	}
	return &layoutStore{root: path}, nil
}

func (s *layoutStore) Kind() string { return "layout" }

func (s *layoutStore) Fetch(_ context.Context, _ Ref) (io.ReadCloser, *Meta, error) {
	return nil, nil, errors.New("layout.Fetch: implemented in Task 2")
}

func (s *layoutStore) Push(_ context.Context, _ Ref, _ io.Reader, _ *Meta) error {
	return errors.New("layout.Push: implemented in Task 2")
}

func (s *layoutStore) Has(_ context.Context, _ Ref) (bool, error) {
	return false, errors.New("layout.Has: implemented in Task 2")
}

func (s *layoutStore) ResolveDigest(_ context.Context, _ string) (string, error) {
	return "", errors.New("layout.ResolveDigest: implemented in Task 2")
}
