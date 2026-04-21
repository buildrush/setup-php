package main

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/buildrush/setup-php/internal/catalog"
	"github.com/buildrush/setup-php/internal/plan"
	"github.com/buildrush/setup-php/internal/resolve"
)

func TestComputeDisabledExtensions(t *testing.T) {
	tests := []struct {
		name    string
		p       plan.Plan
		bundled []string
		want    []string
	}{
		{
			name:    "plain excludes",
			p:       plan.Plan{ExtensionsExclude: []string{"opcache", "xml"}},
			bundled: []string{"curl", "opcache", "xml"},
			want:    []string{"opcache", "xml"},
		},
		{
			name:    "reset disables all bundled not in include",
			p:       plan.Plan{Extensions: []string{"curl"}, ExtensionsReset: true},
			bundled: []string{"curl", "opcache", "xml"},
			want:    []string{"opcache", "xml"},
		},
		{
			name:    "reset with explicit exclude union",
			p:       plan.Plan{Extensions: []string{"curl"}, ExtensionsExclude: []string{"foo"}, ExtensionsReset: true},
			bundled: []string{"curl", "opcache"},
			want:    []string{"foo", "opcache"},
		},
		{
			name:    "no reset, no excludes gives empty",
			p:       plan.Plan{Extensions: []string{"redis"}},
			bundled: []string{"curl", "opcache"},
			want:    nil,
		},
		{
			name:    "include wins over explicit exclude (redis, :redis)",
			p:       plan.Plan{Extensions: []string{"redis"}, ExtensionsExclude: []string{"redis"}},
			bundled: []string{"curl", "opcache"},
			want:    nil,
		},
		{
			name:    "reset with include overlap does not disable included",
			p:       plan.Plan{Extensions: []string{"curl", "opcache"}, ExtensionsReset: true},
			bundled: []string{"curl", "opcache", "mbstring"},
			want:    []string{"mbstring"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := computeDisabledExtensions(&tt.p, tt.bundled)
			if len(got) != len(tt.want) {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
			for i := range tt.want {
				if got[i] != tt.want[i] {
					t.Errorf("[%d] got %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestInstallRuntimeDeps(t *testing.T) {
	ext := func(name string, deps []string) *catalog.ExtensionSpec {
		return &catalog.ExtensionSpec{
			Name:        name,
			Kind:        catalog.ExtensionKindPECL,
			RuntimeDeps: map[string][]string{"linux": deps},
		}
	}
	cat := &catalog.Catalog{
		Extensions: map[string]*catalog.ExtensionSpec{
			"a": ext("a", []string{"libfoo1", "libbar1"}),
			"b": ext("b", []string{"libbar1", "libbaz1"}),
			"c": ext("c", nil),
		},
	}
	tests := []struct {
		name     string
		os       string
		resolved []string
		want     []string
	}{
		{"empty resolved list is no-op", "linux", nil, nil},
		{"single extension with deps", "linux", []string{"a"}, []string{"libbar1", "libfoo1"}},
		{"dedupes across extensions", "linux", []string{"a", "b"}, []string{"libbar1", "libbaz1", "libfoo1"}},
		{"extension without deps contributes nothing", "linux", []string{"c"}, nil},
		{"non-linux os skips", "darwin", []string{"a", "b"}, nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var got []string
			installer := func(pkgs []string) error {
				got = pkgs
				return nil
			}
			var resolved []resolve.ResolvedBundle
			for _, n := range tc.resolved {
				resolved = append(resolved, resolve.ResolvedBundle{Name: n})
			}
			if err := installRuntimeDeps(tc.os, resolved, cat, installer); err != nil {
				t.Fatalf("installRuntimeDeps: %v", err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("installer called with %v, want %v", got, tc.want)
			}
		})
	}
}

func TestInstallRuntimeDeps_PropagatesError(t *testing.T) {
	cat := &catalog.Catalog{
		Extensions: map[string]*catalog.ExtensionSpec{
			"a": {Name: "a", Kind: catalog.ExtensionKindPECL, RuntimeDeps: map[string][]string{"linux": {"libfoo1"}}},
		},
	}
	installer := func(_ []string) error {
		return errors.New("apt failed")
	}
	err := installRuntimeDeps("linux", []resolve.ResolvedBundle{{Name: "a"}}, cat, installer)
	if err == nil || !strings.Contains(err.Error(), "apt failed") {
		t.Errorf("expected apt failed error, got %v", err)
	}
}
