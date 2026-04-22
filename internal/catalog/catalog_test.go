package catalog

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestLoadPHPSpecVersionedLayout(t *testing.T) {
	phpYAML := `
name: php
versions:
  "8.1":
    bundled_extensions: [mbstring, curl]
  "8.4":
    bundled_extensions: [mbstring, curl, intl, zip]
    sources:
      url: "https://www.php.net/distributions/php-{version}.tar.xz"
      sig: "https://www.php.net/distributions/php-{version}.tar.xz.asc"
    abi_matrix:
      os: ["linux"]
      arch: ["x86_64"]
      ts: ["nts"]
    configure_flags:
      common: "--enable-mbstring --with-curl"
      linux: "--with-pdo-pgsql"
smoke:
  - 'php -v'
`
	dir := t.TempDir()
	path := filepath.Join(dir, "php.yaml")
	if err := os.WriteFile(path, []byte(phpYAML), 0o600); err != nil {
		t.Fatalf("write tmp yaml: %v", err)
	}

	spec, err := LoadPHPSpec(path)
	if err != nil {
		t.Fatalf("LoadPHPSpec() error = %v", err)
	}
	if spec.Name != "php" {
		t.Errorf("Name = %q, want %q", spec.Name, "php")
	}
	if len(spec.Versions) != 2 {
		t.Fatalf("Versions len = %d, want 2", len(spec.Versions))
	}
	v81, ok := spec.Versions["8.1"]
	if !ok || v81 == nil {
		t.Fatalf("Versions[8.1] missing")
	}
	if v81.Sources != nil {
		t.Errorf("Versions[8.1].Sources should be nil (compat-only); got %+v", v81.Sources)
	}
	if len(v81.BundledExtensions) != 2 {
		t.Errorf("Versions[8.1].BundledExtensions len = %d, want 2", len(v81.BundledExtensions))
	}
	v84, ok := spec.Versions["8.4"]
	if !ok || v84 == nil {
		t.Fatalf("Versions[8.4] missing")
	}
	if v84.Sources == nil {
		t.Fatal("Versions[8.4].Sources should be non-nil")
	}
	if v84.Sources.URL == "" {
		t.Error("Versions[8.4].Sources.URL should be set")
	}
	if len(v84.BundledExtensions) != 4 {
		t.Errorf("Versions[8.4].BundledExtensions len = %d, want 4", len(v84.BundledExtensions))
	}
	if len(v84.ABIMatrix.OS) != 1 || v84.ABIMatrix.OS[0] != "linux" {
		t.Errorf("Versions[8.4].ABIMatrix.OS = %v, want [linux]", v84.ABIMatrix.OS)
	}
	if v84.ConfigureFlags.Linux == "" {
		t.Error("Versions[8.4].ConfigureFlags.Linux should be set")
	}
}

func TestBuildTargets(t *testing.T) {
	spec := &PHPSpec{
		Name: "php",
		Versions: map[string]*PHPVersionSpec{
			"8.1": {BundledExtensions: []string{"mbstring"}},
			"8.3": {BundledExtensions: []string{"mbstring"}},
			"8.4": {
				BundledExtensions: []string{"mbstring"},
				Sources:           &PHPSource{URL: "u"},
			},
			"8.5": {
				BundledExtensions: []string{"mbstring"},
				Sources:           &PHPSource{URL: "u"},
			},
		},
	}
	targets := spec.BuildTargets()
	if len(targets) != 2 {
		t.Fatalf("len(targets) = %d, want 2", len(targets))
	}
	if targets[0].Version != "8.4" || targets[1].Version != "8.5" {
		t.Errorf("targets = %v, want [8.4, 8.5] sorted", []string{targets[0].Version, targets[1].Version})
	}
}

func TestLoadExtensionSpecPECL(t *testing.T) {
	redisYAML := `
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
	os.WriteFile(path, []byte(redisYAML), 0o644)

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
	mbstringYAML := `
name: mbstring
kind: bundled
note: "Built into PHP core via --enable-mbstring."
`
	dir := t.TempDir()
	path := filepath.Join(dir, "mbstring.yaml")
	os.WriteFile(path, []byte(mbstringYAML), 0o644)

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
			Versions: map[string]*PHPVersionSpec{
				"8.4": {BundledExtensions: []string{"mbstring", "curl", "intl", "zip"}},
			},
		},
		Extensions: map[string]*ExtensionSpec{},
	}

	if !cat.IsBundled("8.4", "mbstring") {
		t.Error("IsBundled(8.4, mbstring) should be true")
	}
	if cat.IsBundled("8.4", "redis") {
		t.Error("IsBundled(8.4, redis) should be false")
	}
	if cat.IsBundled("9.9", "mbstring") {
		t.Error("IsBundled(unknown version) should be false")
	}
}

func TestLoadCatalog(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "extensions"), 0o755)

	phpYAML := `
name: php
versions:
  "8.4":
    bundled_extensions: ["mbstring"]
    sources:
      url: "https://php.net/php-{version}.tar.xz"
      sig: "https://php.net/php-{version}.tar.xz.asc"
    abi_matrix:
      os: ["linux"]
      arch: ["x86_64"]
      ts: ["nts"]
    configure_flags:
      common: "--enable-mbstring"
smoke: ["php -v"]
`
	if err := os.WriteFile(filepath.Join(dir, "php.yaml"), []byte(phpYAML), 0o600); err != nil {
		t.Fatalf("write php.yaml: %v", err)
	}

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

func TestValidatePHPSpec(t *testing.T) {
	buildable := func() *PHPVersionSpec {
		return &PHPVersionSpec{
			BundledExtensions: []string{"mbstring"},
			Sources:           &PHPSource{URL: "u"},
			ABIMatrix: ABIMatrix{
				OS:   []string{"linux"},
				Arch: []string{"x86_64"},
				TS:   []string{"nts"},
			},
		}
	}

	tests := []struct {
		name    string
		spec    *PHPSpec
		wantErr string // substring to match
	}{
		{
			name:    "missing name",
			spec:    &PHPSpec{Versions: map[string]*PHPVersionSpec{"8.4": buildable()}},
			wantErr: "name is required",
		},
		{
			name: "nil version entry",
			spec: &PHPSpec{
				Name:     "php",
				Versions: map[string]*PHPVersionSpec{"8.4": nil},
			},
			wantErr: "version entry is nil",
		},
		{
			name: "missing bundled_extensions",
			spec: &PHPSpec{
				Name: "php",
				Versions: map[string]*PHPVersionSpec{
					"8.4": {Sources: &PHPSource{URL: "u"}},
				},
			},
			wantErr: "bundled_extensions is required",
		},
		{
			name: "sources without URL",
			spec: &PHPSpec{
				Name: "php",
				Versions: map[string]*PHPVersionSpec{
					"8.4": {
						BundledExtensions: []string{"mbstring"},
						Sources:           &PHPSource{},
						ABIMatrix: ABIMatrix{
							OS:   []string{"linux"},
							Arch: []string{"x86_64"},
							TS:   []string{"nts"},
						},
					},
				},
			},
			wantErr: "sources.url is required",
		},
		{
			name: "sources with empty abi_matrix dimension",
			spec: &PHPSpec{
				Name: "php",
				Versions: map[string]*PHPVersionSpec{
					"8.4": {
						BundledExtensions: []string{"mbstring"},
						Sources:           &PHPSource{URL: "u"},
						ABIMatrix: ABIMatrix{
							OS:   []string{"linux"},
							Arch: []string{},
							TS:   []string{"nts"},
						},
					},
				},
			},
			wantErr: "abi_matrix must have at least one value",
		},
		{
			name: "no version has sources",
			spec: &PHPSpec{
				Name: "php",
				Versions: map[string]*PHPVersionSpec{
					"8.1": {BundledExtensions: []string{"mbstring"}},
				},
			},
			wantErr: "at least one version must have sources set",
		},
		{
			name: "valid: mix of compat-only and buildable",
			spec: &PHPSpec{
				Name: "php",
				Versions: map[string]*PHPVersionSpec{
					"8.1": {BundledExtensions: []string{"mbstring"}},
					"8.4": buildable(),
				},
			},
			wantErr: "",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.spec.Validate()
			if tc.wantErr == "" {
				if err != nil {
					t.Errorf("Validate() = %v, want nil", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("Validate() = nil, want error containing %q", tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("Validate() error %q does not contain %q", err.Error(), tc.wantErr)
			}
		})
	}
}

func TestValidateExtSpecMissingSource(t *testing.T) {
	spec := &ExtensionSpec{Name: "foo", Kind: ExtensionKindPECL}
	if err := spec.Validate(); err == nil {
		t.Error("Validate() should fail when PECL source is missing")
	}
}

func TestValidateExtSpec(t *testing.T) {
	tests := []struct {
		name    string
		spec    *ExtensionSpec
		wantErr bool
	}{
		{"missing name", &ExtensionSpec{Kind: ExtensionKindBundled}, true},
		{"missing kind", &ExtensionSpec{Name: "foo"}, true},
		{"pecl missing versions", &ExtensionSpec{
			Name: "foo", Kind: ExtensionKindPECL,
			Source: ExtensionSource{PECLPackage: "foo"},
		}, true},
		{"pecl valid", &ExtensionSpec{
			Name: "foo", Kind: ExtensionKindPECL,
			Source:   ExtensionSource{PECLPackage: "foo"},
			Versions: []string{"1.0.0"},
		}, false},
		{"bundled valid without source", &ExtensionSpec{
			Name: "mbstring", Kind: ExtensionKindBundled,
		}, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.spec.Validate()
			if (err != nil) != tc.wantErr {
				t.Errorf("Validate() err=%v, wantErr=%v", err, tc.wantErr)
			}
		})
	}
}

func TestRequiresSeparateBundle(t *testing.T) {
	cat := &Catalog{
		Extensions: map[string]*ExtensionSpec{
			"redis":    {Name: "redis", Kind: ExtensionKindPECL},
			"mbstring": {Name: "mbstring", Kind: ExtensionKindBundled},
		},
	}
	if !cat.RequiresSeparateBundle("redis") {
		t.Error("redis (pecl) should require separate bundle")
	}
	if cat.RequiresSeparateBundle("mbstring") {
		t.Error("mbstring (bundled) should not require separate bundle")
	}
	if cat.RequiresSeparateBundle("unknown") {
		t.Error("unknown extension should not require separate bundle")
	}
}

func TestHermeticLibsRoundTripPHPVersion(t *testing.T) {
	yamlIn := []byte(`
name: php
versions:
  "8.4":
    bundled_extensions: [mbstring]
    sources:
      url: "https://example/php-{version}.tar.xz"
    abi_matrix:
      os: ["linux"]
      arch: ["x86_64"]
      ts: ["nts"]
    hermetic_libs:
      - "libicui18n.so.*"
      - "libicuuc.so.*"
`)
	var spec PHPSpec
	if err := yaml.Unmarshal(yamlIn, &spec); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	got := spec.Versions["8.4"].HermeticLibs
	want := []string{"libicui18n.so.*", "libicuuc.so.*"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("HermeticLibs = %v, want %v", got, want)
	}
}

func TestHermeticLibsRoundTripExtension(t *testing.T) {
	yamlIn := []byte(`
name: imagick
kind: pecl
source:
  pecl_package: imagick
versions: ["3.8.1"]
abi_matrix:
  php: ["8.4"]
  os: ["linux"]
  arch: ["x86_64"]
  ts: ["nts"]
hermetic_libs:
  - "libMagickWand-6.Q16.so.*"
  - "libMagickCore-6.Q16.so.*"
`)
	var spec ExtensionSpec
	if err := yaml.Unmarshal(yamlIn, &spec); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	want := []string{"libMagickWand-6.Q16.so.*", "libMagickCore-6.Q16.so.*"}
	if !reflect.DeepEqual(spec.HermeticLibs, want) {
		t.Errorf("HermeticLibs = %v, want %v", spec.HermeticLibs, want)
	}
}

func TestValidateHermeticLibs(t *testing.T) {
	cases := []struct {
		name    string
		globs   []string
		wantErr string // substring; empty means expect no error
	}{
		{"empty", nil, ""},
		{"valid wildcard", []string{"libicui18n.so.*"}, ""},
		{"valid concrete SOVERSION", []string{"libMagickWand.so.6"}, ""},
		{"rejects path separator", []string{"lib/foo.so.1"}, "must not contain '/'"},
		{"rejects invalid prefix", []string{"icui18n.so.70"}, "must match"},
		{"rejects bare name no SOVERSION no wildcard", []string{"libfoo.so"}, "must contain '*' or a SOVERSION"},
		{"rejects duplicate", []string{"libicui18n.so.*", "libicui18n.so.*"}, "duplicate"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateHermeticLibs(tc.globs)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("err = %v, want substring %q", err, tc.wantErr)
			}
		})
	}
}

func TestExtensionSpec_BuildDeps(t *testing.T) {
	cases := []struct {
		name string
		yaml string
		want map[string][]string
	}{
		{
			name: "present with linux packages",
			yaml: `name: foo
kind: pecl
source:
  pecl_package: foo
versions: ["1.0.0"]
abi_matrix:
  php: ["8.4"]
  os: ["linux"]
  arch: ["x86_64"]
  ts: ["nts"]
build_deps:
  linux: ["libfoo-dev", "libbar-dev"]
`,
			want: map[string][]string{"linux": {"libfoo-dev", "libbar-dev"}},
		},
		{
			name: "absent",
			yaml: `name: foo
kind: pecl
source:
  pecl_package: foo
versions: ["1.0.0"]
abi_matrix:
  php: ["8.4"]
  os: ["linux"]
  arch: ["x86_64"]
  ts: ["nts"]
`,
			want: nil,
		},
		{
			name: "present but empty",
			yaml: `name: foo
kind: pecl
source:
  pecl_package: foo
versions: ["1.0.0"]
abi_matrix:
  php: ["8.4"]
  os: ["linux"]
  arch: ["x86_64"]
  ts: ["nts"]
build_deps:
  linux: []
`,
			want: map[string][]string{"linux": {}},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var spec ExtensionSpec
			if err := yaml.Unmarshal([]byte(tc.yaml), &spec); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if !reflect.DeepEqual(spec.BuildDeps, tc.want) {
				t.Errorf("BuildDeps = %v, want %v", spec.BuildDeps, tc.want)
			}
		})
	}
}
