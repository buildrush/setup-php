package main

import (
	"fmt"
	"path/filepath"
	"runtime"
	"slices"
	"sort"
	"testing"

	"github.com/buildrush/setup-php/internal/catalog"
)

// TestRuntimeExtensionSpecsMatchYAML guards against drift between the inline
// extension specs in runtimeExtensionSpecs() and the authoritative YAML files
// at catalog/extensions/<name>.yaml. Mirrors the pattern used by
// TestCatalogBundledMatchesCompat in internal/catalog/catalog_compat_test.go.
//
// Only version pins and kind are checked; RuntimeDeps and Ini intentionally
// differ in shape between the runtime catalog and the YAML.
func TestRuntimeExtensionSpecsMatchYAML(t *testing.T) {
	// Resolve the catalog/extensions directory relative to this test file.
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	extDir := filepath.Join(filepath.Dir(thisFile), "..", "..", "catalog", "extensions")

	specs := runtimeExtensionSpecs()

	// Sort keys for deterministic output.
	names := make([]string, 0, len(specs))
	for name := range specs {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		inline := specs[name]
		yamlPath := filepath.Join(extDir, fmt.Sprintf("%s.yaml", name))
		yamlSpec, err := catalog.LoadExtensionSpec(yamlPath)
		if err != nil {
			t.Errorf("extension %s: LoadExtensionSpec(%s): %v", name, yamlPath, err)
			continue
		}

		// Check kind.
		if inline.Kind != yamlSpec.Kind {
			t.Errorf("extension %s: kind mismatch: inline=%q yaml=%q", name, inline.Kind, yamlSpec.Kind)
		}

		// Check versions.
		inlineVersions := append([]string(nil), inline.Versions...)
		yamlVersions := append([]string(nil), yamlSpec.Versions...)
		sort.Strings(inlineVersions)
		sort.Strings(yamlVersions)
		if !slices.Equal(inlineVersions, yamlVersions) {
			t.Errorf("extension %s: versions mismatch: inline=%v yaml=%v", name, inlineVersions, yamlVersions)
		}
	}
}
