package catalog

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadPHPSpec(t *testing.T) {
	yaml := `
name: php
versions:
  - "8.4"
source:
  url: "https://www.php.net/distributions/php-{version}.tar.xz"
  sig: "https://www.php.net/distributions/php-{version}.tar.xz.asc"
abi_matrix:
  os: ["linux"]
  arch: ["x86_64"]
  ts: ["nts"]
configure_flags:
  common: "--enable-mbstring --with-curl"
  linux: "--with-pdo-pgsql"
bundled_extensions:
  - mbstring
  - curl
  - intl
  - zip
smoke:
  - 'php -v'
`
	dir := t.TempDir()
	path := filepath.Join(dir, "php.yaml")
	os.WriteFile(path, []byte(yaml), 0o644)

	spec, err := LoadPHPSpec(path)
	if err != nil {
		t.Fatalf("LoadPHPSpec() error = %v", err)
	}
	if spec.Name != "php" {
		t.Errorf("Name = %q, want %q", spec.Name, "php")
	}
	if len(spec.Versions) != 1 || spec.Versions[0] != "8.4" {
		t.Errorf("Versions = %v, want [8.4]", spec.Versions)
	}
	if len(spec.BundledExtensions) != 4 {
		t.Errorf("BundledExtensions len = %d, want 4", len(spec.BundledExtensions))
	}
}

func TestLoadExtensionSpecPECL(t *testing.T) {
	yaml := `
name: redis
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
runtime_deps:
  linux: []
ini:
  - "extension=redis"
smoke:
  - 'php -r "assert(extension_loaded(\"redis\"));"'
`
	dir := t.TempDir()
	path := filepath.Join(dir, "redis.yaml")
	os.WriteFile(path, []byte(yaml), 0o644)

	spec, err := LoadExtensionSpec(path)
	if err != nil {
		t.Fatalf("LoadExtensionSpec() error = %v", err)
	}
	if spec.Kind != ExtensionKindPECL {
		t.Errorf("Kind = %q, want %q", spec.Kind, ExtensionKindPECL)
	}
	if spec.Source.PECLPackage != "redis" {
		t.Errorf("PECLPackage = %q, want redis", spec.Source.PECLPackage)
	}
}

func TestLoadExtensionSpecBundled(t *testing.T) {
	yaml := `
name: mbstring
kind: bundled
note: "Built into PHP core via --enable-mbstring."
`
	dir := t.TempDir()
	path := filepath.Join(dir, "mbstring.yaml")
	os.WriteFile(path, []byte(yaml), 0o644)

	spec, err := LoadExtensionSpec(path)
	if err != nil {
		t.Fatalf("LoadExtensionSpec() error = %v", err)
	}
	if spec.Kind != ExtensionKindBundled {
		t.Errorf("Kind = %q, want %q", spec.Kind, ExtensionKindBundled)
	}
}

func TestCatalogIsBundled(t *testing.T) {
	cat := &Catalog{
		PHP: &PHPSpec{
			BundledExtensions: []string{"mbstring", "curl", "intl", "zip"},
		},
		Extensions: map[string]*ExtensionSpec{},
	}

	if !cat.IsBundled("mbstring") {
		t.Error("IsBundled(mbstring) should be true")
	}
	if cat.IsBundled("redis") {
		t.Error("IsBundled(redis) should be false")
	}
}

func TestLoadCatalog(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "extensions"), 0o755)

	phpYAML := `
name: php
versions: ["8.4"]
source:
  url: "https://php.net/php-{version}.tar.xz"
  sig: "https://php.net/php-{version}.tar.xz.asc"
abi_matrix:
  os: ["linux"]
  arch: ["x86_64"]
  ts: ["nts"]
configure_flags:
  common: "--enable-mbstring"
bundled_extensions: ["mbstring"]
smoke: ["php -v"]
`
	os.WriteFile(filepath.Join(dir, "php.yaml"), []byte(phpYAML), 0o644)

	redisYAML := `
name: redis
kind: pecl
source:
  pecl_package: redis
versions: ["6.2.0"]
abi_matrix:
  php: ["8.4"]
  os: ["linux"]
  arch: ["x86_64"]
  ts: ["nts"]
ini: ["extension=redis"]
smoke: ['php -r "assert(extension_loaded(\"redis\"));"']
`
	os.WriteFile(filepath.Join(dir, "extensions", "redis.yaml"), []byte(redisYAML), 0o644)

	cat, err := LoadCatalog(dir)
	if err != nil {
		t.Fatalf("LoadCatalog() error = %v", err)
	}
	if cat.PHP == nil {
		t.Fatal("PHP spec should not be nil")
	}
	if _, ok := cat.Extensions["redis"]; !ok {
		t.Error("redis extension should be loaded")
	}
}

func TestValidatePHPSpecMissingVersions(t *testing.T) {
	spec := &PHPSpec{Name: "php"}
	if err := spec.Validate(); err == nil {
		t.Error("Validate() should fail when versions is empty")
	}
}

func TestValidateExtSpecMissingSource(t *testing.T) {
	spec := &ExtensionSpec{Name: "foo", Kind: ExtensionKindPECL}
	if err := spec.Validate(); err == nil {
		t.Error("Validate() should fail when PECL source is missing")
	}
}
