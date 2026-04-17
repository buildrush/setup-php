package catalog

import (
	"sort"
	"testing"

	"github.com/buildrush/setup-php/internal/compat"
)

// TestCatalogBundledMatchesCompat guards against drift between
// catalog/php.yaml and internal/compat.BundledExtensions. Both encode the
// same per-version "what does `php -m` report for a default Ondrej PPA
// build?" truth; if they disagree, one of the two is out of date and must be
// updated. See docs/compat-matrix.md for provenance.
func TestCatalogBundledMatchesCompat(t *testing.T) {
	spec, err := LoadPHPSpec("../../catalog/php.yaml")
	if err != nil {
		t.Fatalf("LoadPHPSpec: %v", err)
	}
	for v, vs := range spec.Versions {
		if vs == nil {
			t.Errorf("php.yaml version %s: spec is nil", v)
			continue
		}
		want := append([]string(nil), compat.BundledExtensions(v)...)
		got := append([]string(nil), vs.BundledExtensions...)
		if len(want) == 0 {
			t.Errorf("compat has no bundled extensions for version %s (unknown to internal/compat?)", v)
			continue
		}
		sort.Strings(want)
		sort.Strings(got)
		if len(want) != len(got) {
			t.Errorf("php.yaml %s bundled_extensions len=%d, compat len=%d\n got=%v\nwant=%v",
				v, len(got), len(want), got, want)
			continue
		}
		for i := range want {
			if got[i] != want[i] {
				t.Errorf("php.yaml %s bundled_extensions[%d]=%q, compat=%q", v, i, got[i], want[i])
			}
		}
	}
}
