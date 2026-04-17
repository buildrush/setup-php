package resolve

import (
	"strings"
	"testing"

	"github.com/buildrush/setup-php/internal/catalog"
	"github.com/buildrush/setup-php/internal/lockfile"
	"github.com/buildrush/setup-php/internal/plan"
)

func testCatalog() *catalog.Catalog {
	return &catalog.Catalog{
		PHP: &catalog.PHPSpec{
			Versions: map[string]*catalog.PHPVersionSpec{
				"8.4.6": {BundledExtensions: []string{"mbstring", "curl", "intl", "zip"}},
			},
		},
		Extensions: map[string]*catalog.ExtensionSpec{
			"redis":    {Name: "redis", Kind: catalog.ExtensionKindPECL},
			"mbstring": {Name: "mbstring", Kind: catalog.ExtensionKindBundled},
		},
	}
}

func testLockfile() *lockfile.Lockfile {
	return &lockfile.Lockfile{
		Bundles: map[lockfile.BundleKey]lockfile.Entry{
			"php:8.4.6:linux:x86_64:nts":           {Digest: "sha256:phpdigest"},
			"ext:redis:6.2.0:8.4:linux:x86_64:nts": {Digest: "sha256:redisdigest"},
		},
	}
}

func TestResolveHappyPath(t *testing.T) {
	p := &plan.Plan{
		PHPVersion:   "8.4.6",
		Extensions:   []string{"mbstring", "redis"},
		OS:           "linux",
		Arch:         "x86_64",
		ThreadSafety: "nts",
	}

	res, err := Resolve(p, testLockfile(), testCatalog())
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if res.PHPCore.Digest != "sha256:phpdigest" {
		t.Errorf("PHPCore.Digest = %q, want sha256:phpdigest", res.PHPCore.Digest)
	}
	if len(res.Extensions) != 1 {
		t.Fatalf("len(Extensions) = %d, want 1 (redis only)", len(res.Extensions))
	}
	if res.Extensions[0].Name != "redis" {
		t.Errorf("Extensions[0].Name = %q, want redis", res.Extensions[0].Name)
	}
}

func TestResolveMissingPHP(t *testing.T) {
	p := &plan.Plan{
		PHPVersion:   "9.0.0",
		OS:           "linux",
		Arch:         "x86_64",
		ThreadSafety: "nts",
	}

	_, err := Resolve(p, testLockfile(), testCatalog())
	if err == nil {
		t.Fatal("Resolve() should return error for missing PHP version")
	}
}

func TestResolveMissingExtension(t *testing.T) {
	p := &plan.Plan{
		PHPVersion:   "8.4.6",
		Extensions:   []string{"unknown_ext"},
		OS:           "linux",
		Arch:         "x86_64",
		ThreadSafety: "nts",
	}

	_, err := Resolve(p, testLockfile(), testCatalog())
	if err == nil {
		t.Fatal("Resolve() should return error for missing extension in lockfile")
	}
}

func TestResolveAllBundled(t *testing.T) {
	p := &plan.Plan{
		PHPVersion:   "8.4.6",
		Extensions:   []string{"mbstring", "curl", "intl"},
		OS:           "linux",
		Arch:         "x86_64",
		ThreadSafety: "nts",
	}

	res, err := Resolve(p, testLockfile(), testCatalog())
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if len(res.Extensions) != 0 {
		t.Errorf("len(Extensions) = %d, want 0 (all bundled)", len(res.Extensions))
	}
}

func TestResolveToolsWarning(t *testing.T) {
	p := &plan.Plan{
		PHPVersion:   "8.4.6",
		Tools:        []string{"composer"},
		OS:           "linux",
		Arch:         "x86_64",
		ThreadSafety: "nts",
	}

	res, err := Resolve(p, testLockfile(), testCatalog())
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if len(res.Warnings) == 0 {
		t.Error("Resolve() should emit a warning for tools in Phase 1")
	}
}

func TestResolveZTSFallback(t *testing.T) {
	// Lockfile has NTS but not ZTS.
	lf := testLockfile()
	// testLockfile's "redis" and ZTS entries aren't relevant here; just verify the
	// core fallback semantics.
	cat := testCatalog()

	p := &plan.Plan{
		PHPVersion:   "8.4.6",
		OS:           "linux",
		Arch:         "x86_64",
		ThreadSafety: "zts",
		FailFast:     false,
	}
	res, err := Resolve(p, lf, cat)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if res.PHPCore.Digest != "sha256:phpdigest" {
		t.Errorf("ZTS should fall back to NTS; got digest %q", res.PHPCore.Digest)
	}
	found := false
	for _, w := range res.Warnings {
		if strings.Contains(w, "ZTS not available") && strings.Contains(w, "falling back to NTS") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected ZTS fallback warning, got %v", res.Warnings)
	}
}

func TestResolveZTSFallbackFailFast(t *testing.T) {
	lf := testLockfile()
	cat := testCatalog()
	p := &plan.Plan{
		PHPVersion:   "8.4.6",
		OS:           "linux",
		Arch:         "x86_64",
		ThreadSafety: "zts",
		FailFast:     true,
	}
	_, err := Resolve(p, lf, cat)
	if err == nil {
		t.Fatal("expected hard error under fail-fast: true, got nil")
	}
	if !strings.Contains(err.Error(), "ZTS") {
		t.Errorf("error should mention ZTS; got %v", err)
	}
}

func TestResolveZTSUnresolvable(t *testing.T) {
	// Neither ZTS nor NTS present for the requested version → hard error regardless of FailFast.
	lf := &lockfile.Lockfile{Bundles: map[lockfile.BundleKey]lockfile.Entry{}}
	cat := testCatalog()
	p := &plan.Plan{
		PHPVersion:   "8.4.6",
		OS:           "linux",
		Arch:         "x86_64",
		ThreadSafety: "zts",
		FailFast:     false,
	}
	_, err := Resolve(p, lf, cat)
	if err == nil {
		t.Fatal("expected hard error when neither ZTS nor NTS available")
	}
}

func TestResolveToolsWarningFailFast(t *testing.T) {
	p := &plan.Plan{
		PHPVersion:   "8.4.6",
		OS:           "linux",
		Arch:         "x86_64",
		ThreadSafety: "nts",
		Tools:        []string{"composer"},
		FailFast:     true,
	}
	_, err := Resolve(p, testLockfile(), testCatalog())
	if err == nil {
		t.Fatal("expected hard error under fail-fast: true with tools input")
	}
	if !strings.Contains(err.Error(), "composer") {
		t.Errorf("error should mention the requested tool; got %v", err)
	}
}
