package main

import (
	"testing"

	"github.com/buildrush/setup-php/internal/plan"
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
