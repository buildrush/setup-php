package build

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/buildrush/setup-php/internal/planner"
)

// fakeRepo lays down a minimal tree that mimics the setup-php repo layout
// well enough for ComputeSpecHash to find every file it needs. Contents are
// arbitrary but stable; tests that want to see the hash change mutate the
// relevant file under this tempdir.
func fakeRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()

	mk := func(rel, content string) {
		t.Helper()
		abs := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(abs), 0o750); err != nil {
			t.Fatalf("mkdir %s: %v", rel, err)
		}
		if err := os.WriteFile(abs, []byte(content), 0o600); err != nil {
			t.Fatalf("write %s: %v", rel, err)
		}
	}

	mk("builders/linux/build-php.sh", "#!/bin/bash\necho build-php v1\n")
	mk("builders/linux/build-ext.sh", "#!/bin/bash\necho build-ext v1\n")
	mk("builders/common/bundle-schema-version.env", "BUNDLE_SCHEMA_VERSION=1\n")
	mk("builders/common/capture-hermetic-libs.sh", "#!/bin/bash\necho capture v1\n")
	mk("builders/common/pack-bundle.sh", "#!/bin/bash\necho pack v1\n")
	mk("builders/common/fetch-core.sh", "#!/bin/bash\necho fetch v1\n")
	mk("builders/common/builder-os.env", "BUILDER_OS=ubuntu-22.04\n")

	mk("catalog/php.yaml", `name: php
versions:
  "8.4":
    bundled_extensions: [core]
    sources:
      url: https://example/php-8.4.tar.xz
    abi_matrix:
      os: [linux]
      arch: [x86_64]
      ts: [nts]
`)

	mk("catalog/extensions/redis.yaml", `name: redis
kind: pecl
source:
  pecl_package: redis
versions:
  - "6.2.0"
abi_matrix:
  php: ["8.4"]
  os: ["linux"]
  arch: ["x86_64"]
  ts: ["nts"]
`)

	return root
}

func phpIn(repo string) *SpecHashInputs {
	return &SpecHashInputs{
		Kind:    "php",
		Version: "8.4",
		OS:      "linux",
		Arch:    "x86_64",
		TS:      "nts",
		Repo:    repo,
	}
}

func extIn(repo string) *SpecHashInputs {
	return &SpecHashInputs{
		Kind:    "ext",
		Name:    "redis",
		Version: "6.2.0",
		OS:      "linux",
		Arch:    "x86_64",
		PHPABI:  "8.4-nts",
		TS:      "nts",
		Repo:    repo,
	}
}

func TestComputeSpecHash_PHP_StableAcrossCalls(t *testing.T) {
	repo := fakeRepo(t)
	h1, err := ComputeSpecHash(phpIn(repo))
	if err != nil {
		t.Fatalf("first call: %v", err)
	}
	h2, err := ComputeSpecHash(phpIn(repo))
	if err != nil {
		t.Fatalf("second call: %v", err)
	}
	if h1 != h2 {
		t.Errorf("stable call returned different hashes:\n1: %s\n2: %s", h1, h2)
	}
}

func TestComputeSpecHash_PHP_ChangesWhenCatalogChanges(t *testing.T) {
	repo := fakeRepo(t)
	h1, err := ComputeSpecHash(phpIn(repo))
	if err != nil {
		t.Fatalf("h1: %v", err)
	}

	// Mutate the per-version subtree. Adding a new field to the 8.4 entry
	// must flow through PerVersionYAMLFromFile into the hash.
	newPHP := `name: php
versions:
  "8.4":
    bundled_extensions: [core, opcache]
    sources:
      url: https://example/php-8.4.tar.xz
    abi_matrix:
      os: [linux]
      arch: [x86_64]
      ts: [nts]
`
	if err := os.WriteFile(filepath.Join(repo, "catalog", "php.yaml"), []byte(newPHP), 0o600); err != nil {
		t.Fatalf("rewrite php.yaml: %v", err)
	}

	h2, err := ComputeSpecHash(phpIn(repo))
	if err != nil {
		t.Fatalf("h2: %v", err)
	}
	if h1 == h2 {
		t.Errorf("catalog change did not alter spec_hash, both = %s", h1)
	}
}

func TestComputeSpecHash_PHP_ChangesWhenBuilderChanges(t *testing.T) {
	repo := fakeRepo(t)
	h1, err := ComputeSpecHash(phpIn(repo))
	if err != nil {
		t.Fatalf("h1: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repo, "builders", "linux", "build-php.sh"),
		[]byte("#!/bin/bash\necho build-php v2\n"), 0o600); err != nil {
		t.Fatalf("rewrite builder: %v", err)
	}
	h2, err := ComputeSpecHash(phpIn(repo))
	if err != nil {
		t.Fatalf("h2: %v", err)
	}
	if h1 == h2 {
		t.Errorf("builder change did not alter spec_hash, both = %s", h1)
	}
}

func TestComputeSpecHash_PHP_ChangesWhenCommonBuilderChanges(t *testing.T) {
	// The common files (capture-hermetic-libs, pack-bundle, schema env) are
	// folded into the builder hash alongside build-php.sh. Mutating any of
	// them must bust spec_hash too — otherwise a pack-script fix would ship
	// silently and the lockfile would never invalidate.
	repo := fakeRepo(t)
	h1, err := ComputeSpecHash(phpIn(repo))
	if err != nil {
		t.Fatalf("h1: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repo, "builders", "common", "pack-bundle.sh"),
		[]byte("#!/bin/bash\necho pack v2\n"), 0o600); err != nil {
		t.Fatalf("rewrite pack-bundle: %v", err)
	}
	h2, err := ComputeSpecHash(phpIn(repo))
	if err != nil {
		t.Fatalf("h2: %v", err)
	}
	if h1 == h2 {
		t.Errorf("common-builder change did not alter spec_hash, both = %s", h1)
	}
}

func TestComputeSpecHash_PHP_ChangesWhenBuilderOSChanges(t *testing.T) {
	repo := fakeRepo(t)
	h1, err := ComputeSpecHash(phpIn(repo))
	if err != nil {
		t.Fatalf("h1: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repo, "builders", "common", "builder-os.env"),
		[]byte("BUILDER_OS=ubuntu-24.04\n"), 0o600); err != nil {
		t.Fatalf("rewrite builder-os.env: %v", err)
	}
	h2, err := ComputeSpecHash(phpIn(repo))
	if err != nil {
		t.Fatalf("h2: %v", err)
	}
	if h1 == h2 {
		t.Errorf("builder-os change did not alter spec_hash, both = %s", h1)
	}
}

func TestComputeSpecHash_Ext_StableAcrossCalls(t *testing.T) {
	repo := fakeRepo(t)
	h1, err := ComputeSpecHash(extIn(repo))
	if err != nil {
		t.Fatalf("h1: %v", err)
	}
	h2, err := ComputeSpecHash(extIn(repo))
	if err != nil {
		t.Fatalf("h2: %v", err)
	}
	if h1 != h2 {
		t.Errorf("stable call returned different hashes:\n1: %s\n2: %s", h1, h2)
	}
}

func TestComputeSpecHash_Ext_ChangesWhenExtensionCatalogChanges(t *testing.T) {
	repo := fakeRepo(t)
	h1, err := ComputeSpecHash(extIn(repo))
	if err != nil {
		t.Fatalf("h1: %v", err)
	}

	newExt := `name: redis
kind: pecl
source:
  pecl_package: redis
versions:
  - "6.2.0"
  - "6.3.0"
abi_matrix:
  php: ["8.4"]
  os: ["linux"]
  arch: ["x86_64"]
  ts: ["nts"]
`
	if err := os.WriteFile(filepath.Join(repo, "catalog", "extensions", "redis.yaml"), []byte(newExt), 0o600); err != nil {
		t.Fatalf("rewrite redis.yaml: %v", err)
	}
	h2, err := ComputeSpecHash(extIn(repo))
	if err != nil {
		t.Fatalf("h2: %v", err)
	}
	if h1 == h2 {
		t.Errorf("ext catalog change did not alter spec_hash, both = %s", h1)
	}
}

// The PHP and ext kinds have distinct builder-file sets (build-php.sh vs
// build-ext.sh), plus different catalog bytes. They must never collide even
// if the other axes are zeroed.
func TestComputeSpecHash_PHP_And_Ext_Differ(t *testing.T) {
	repo := fakeRepo(t)
	hp, err := ComputeSpecHash(phpIn(repo))
	if err != nil {
		t.Fatalf("php: %v", err)
	}
	he, err := ComputeSpecHash(extIn(repo))
	if err != nil {
		t.Fatalf("ext: %v", err)
	}
	if hp == he {
		t.Errorf("php and ext produced the same spec_hash: %s", hp)
	}
}

// ComputeSpecHash must mirror planner.ComputeSpecHash exactly for equivalent
// inputs. This test composes the inputs by hand the way the planner does it
// and verifies the wrapper returns the same string.
func TestComputeSpecHash_PHP_MatchesPlannerOutput(t *testing.T) {
	repo := fakeRepo(t)
	got, err := ComputeSpecHash(phpIn(repo))
	if err != nil {
		t.Fatalf("wrapper: %v", err)
	}

	builderHash, err := planner.HashFiles([]string{
		filepath.Join(repo, "builders", "linux", "build-php.sh"),
		filepath.Join(repo, "builders", "common", "bundle-schema-version.env"),
		filepath.Join(repo, "builders", "common", "capture-hermetic-libs.sh"),
		filepath.Join(repo, "builders", "common", "pack-bundle.sh"),
	})
	if err != nil {
		t.Fatalf("hash builders: %v", err)
	}
	catalogBytes, err := planner.PerVersionYAMLFromFile(filepath.Join(repo, "catalog", "php.yaml"), "8.4")
	if err != nil {
		t.Fatalf("catalog bytes: %v", err)
	}
	cell := &planner.MatrixCell{Version: "8.4", OS: "linux", Arch: "x86_64", TS: "nts"}
	want := planner.ComputeSpecHash(cell, catalogBytes, builderHash, "ubuntu-22.04")
	if got != want {
		t.Errorf("wrapper diverges from planner:\n wrapper = %s\n planner = %s", got, want)
	}
}

func TestComputeSpecHash_Ext_MatchesPlannerOutput(t *testing.T) {
	repo := fakeRepo(t)
	got, err := ComputeSpecHash(extIn(repo))
	if err != nil {
		t.Fatalf("wrapper: %v", err)
	}

	builderHash, err := planner.HashFiles([]string{
		filepath.Join(repo, "builders", "linux", "build-ext.sh"),
		filepath.Join(repo, "builders", "common", "bundle-schema-version.env"),
		filepath.Join(repo, "builders", "common", "capture-hermetic-libs.sh"),
		filepath.Join(repo, "builders", "common", "pack-bundle.sh"),
	})
	if err != nil {
		t.Fatalf("hash builders: %v", err)
	}
	catalogBytes, err := planner.ExtensionYAMLFromFile(filepath.Join(repo, "catalog", "extensions", "redis.yaml"))
	if err != nil {
		t.Fatalf("catalog bytes: %v", err)
	}
	// Note: ext cells use ExtVer/PHPAbi/Extension — the planner leaves
	// Version empty for ext entries. Matching that avoids silently changing
	// the hash.
	cell := &planner.MatrixCell{
		Extension: "redis",
		ExtVer:    "6.2.0",
		PHPAbi:    "8.4-nts",
		OS:        "linux",
		Arch:      "x86_64",
		TS:        "nts",
	}
	want := planner.ComputeSpecHash(cell, catalogBytes, builderHash, "ubuntu-22.04")
	if got != want {
		t.Errorf("wrapper diverges from planner:\n wrapper = %s\n planner = %s", got, want)
	}
}

func TestComputeSpecHash_UnknownKind_Errors(t *testing.T) {
	repo := fakeRepo(t)
	in := &SpecHashInputs{Kind: "tool", Name: "composer", Version: "2.7", Repo: repo}
	if _, err := ComputeSpecHash(in); err == nil {
		t.Error("unknown kind must error")
	}
}

func TestComputeSpecHash_MissingCatalog_Errors(t *testing.T) {
	repo := fakeRepo(t)
	// Delete the per-version catalog so load fails.
	if err := os.Remove(filepath.Join(repo, "catalog", "php.yaml")); err != nil {
		t.Fatalf("remove php.yaml: %v", err)
	}
	if _, err := ComputeSpecHash(phpIn(repo)); err == nil {
		t.Error("missing catalog must error")
	}
}

func TestComputeSpecHash_MissingExtCatalog_Errors(t *testing.T) {
	repo := fakeRepo(t)
	if err := os.Remove(filepath.Join(repo, "catalog", "extensions", "redis.yaml")); err != nil {
		t.Fatalf("remove redis.yaml: %v", err)
	}
	if _, err := ComputeSpecHash(extIn(repo)); err == nil {
		t.Error("missing ext catalog must error")
	}
}

func TestComputeSpecHash_MissingBuilderOS_Errors(t *testing.T) {
	repo := fakeRepo(t)
	if err := os.Remove(filepath.Join(repo, "builders", "common", "builder-os.env")); err != nil {
		t.Fatalf("remove builder-os.env: %v", err)
	}
	if _, err := ComputeSpecHash(phpIn(repo)); err == nil {
		t.Error("missing builder-os.env must error")
	}
}

func TestComputeSpecHash_EmptyBuilderOS_Errors(t *testing.T) {
	// Silent-fallback guard: if BUILDER_OS isn't found in the env file we
	// must fail loudly, because emitting "" into the hash would make every
	// lockfile entry look up-to-date but reflect the wrong runner.
	repo := fakeRepo(t)
	if err := os.WriteFile(filepath.Join(repo, "builders", "common", "builder-os.env"),
		[]byte("# no BUILDER_OS here\n"), 0o600); err != nil {
		t.Fatalf("rewrite builder-os.env: %v", err)
	}
	if _, err := ComputeSpecHash(phpIn(repo)); err == nil {
		t.Error("missing BUILDER_OS key must error")
	}
}

func TestComputeSpecHash_MissingBuilderScript_Errors(t *testing.T) {
	repo := fakeRepo(t)
	if err := os.Remove(filepath.Join(repo, "builders", "linux", "build-php.sh")); err != nil {
		t.Fatalf("remove build-php.sh: %v", err)
	}
	if _, err := ComputeSpecHash(phpIn(repo)); err == nil {
		t.Error("missing builder script must error")
	}
}
