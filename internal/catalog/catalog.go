package catalog

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

type ExtensionKind string

const (
	ExtensionKindPECL    ExtensionKind = "pecl"
	ExtensionKindBundled ExtensionKind = "bundled"
	ExtensionKindGit     ExtensionKind = "git"
)

// PHPSpec describes the PHP catalog entry. It holds per-version metadata
// under Versions; only versions with Sources set are built by the pipeline,
// the rest encode compat intent (e.g. bundled extension lists) used for
// cross-validation against internal/compat.
type PHPSpec struct {
	Name     string                     `yaml:"name"`
	Versions map[string]*PHPVersionSpec `yaml:"versions"`
	Smoke    []string                   `yaml:"smoke,omitempty"`
}

// PHPVersionSpec is the per-version subtree of PHPSpec. Sources, ABIMatrix,
// and ConfigureFlags are only required for versions that should actually be
// built by the pipeline; compat-only versions may omit them.
type PHPVersionSpec struct {
	BundledExtensions []string       `yaml:"bundled_extensions"`
	Sources           *PHPSource     `yaml:"sources,omitempty"`
	ABIMatrix         ABIMatrix      `yaml:"abi_matrix,omitempty"`
	ConfigureFlags    ConfigureFlags `yaml:"configure_flags,omitempty"`
	Smoke             []string       `yaml:"smoke,omitempty"`
	HermeticLibs      []string       `yaml:"hermetic_libs,omitempty"`
}

// BuildTarget pairs a PHP version with its version-specific spec. Only
// versions with Sources set produce BuildTargets.
type BuildTarget struct {
	Version string
	Spec    *PHPVersionSpec
}

// BuildTargets returns the subset of Versions that have Sources set, sorted
// by version string for deterministic ordering. Callers iterating the build
// matrix should use this rather than Versions directly.
func (s *PHPSpec) BuildTargets() []BuildTarget {
	var out []BuildTarget
	for v, vs := range s.Versions {
		if vs != nil && vs.Sources != nil {
			out = append(out, BuildTarget{Version: v, Spec: vs})
		}
	}
	// TODO(8.10): lexicographic string compare — "8.10" < "8.2" is true. Revisit
	// when PHP 8.10 arrives (estimated post-2028) and switch to semver-aware sort.
	sort.Slice(out, func(i, j int) bool { return out[i].Version < out[j].Version })
	return out
}

type PHPSource struct {
	URL string `yaml:"url"`
	Sig string `yaml:"sig"`
}

type ABIMatrix struct {
	PHP  []string `yaml:"php,omitempty"`
	OS   []string `yaml:"os"`
	Arch []string `yaml:"arch"`
	TS   []string `yaml:"ts"`
}

type ConfigureFlags struct {
	Common string `yaml:"common"`
	Linux  string `yaml:"linux,omitempty"`
	MacOS  string `yaml:"macos,omitempty"`
}

type ExtensionSpec struct {
	Name         string              `yaml:"name"`
	Kind         ExtensionKind       `yaml:"kind"`
	Note         string              `yaml:"note,omitempty"`
	Source       ExtensionSource     `yaml:"source,omitempty"`
	Versions     []string            `yaml:"versions,omitempty"`
	ABIMatrix    ABIMatrix           `yaml:"abi_matrix,omitempty"`
	Exclude      []ExcludeRule       `yaml:"exclude,omitempty"`
	BuildDeps    map[string][]string `yaml:"build_deps,omitempty"`
	RuntimeDeps  map[string][]string `yaml:"runtime_deps,omitempty"`
	Ini          []string            `yaml:"ini,omitempty"`
	Smoke        []string            `yaml:"smoke,omitempty"`
	HermeticLibs []string            `yaml:"hermetic_libs,omitempty"`
}

type ExtensionSource struct {
	PECLPackage string `yaml:"pecl_package,omitempty"`
	GitURL      string `yaml:"git_url,omitempty"`
}

type ExcludeRule struct {
	OS   string `yaml:"os,omitempty"`
	Arch string `yaml:"arch,omitempty"`
	PHP  string `yaml:"php,omitempty"`
}

type Catalog struct {
	PHP        *PHPSpec
	Extensions map[string]*ExtensionSpec
}

func LoadPHPSpec(path string) (*PHPSpec, error) {
	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return nil, fmt.Errorf("read PHP spec %s: %w", path, err)
	}
	var spec PHPSpec
	if err := yaml.Unmarshal(data, &spec); err != nil {
		return nil, fmt.Errorf("parse PHP spec %s: %w", path, err)
	}
	return &spec, nil
}

func LoadExtensionSpec(path string) (*ExtensionSpec, error) {
	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return nil, fmt.Errorf("read extension spec %s: %w", path, err)
	}
	var spec ExtensionSpec
	if err := yaml.Unmarshal(data, &spec); err != nil {
		return nil, fmt.Errorf("parse extension spec %s: %w", path, err)
	}
	return &spec, nil
}

func LoadCatalog(dir string) (*Catalog, error) {
	phpPath := filepath.Join(dir, "php.yaml")
	php, err := LoadPHPSpec(phpPath)
	if err != nil {
		return nil, fmt.Errorf("load PHP spec: %w", err)
	}

	extensions := make(map[string]*ExtensionSpec)
	extDir := filepath.Join(dir, "extensions")
	entries, err := os.ReadDir(extDir)
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("read extensions dir: %w", err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".yaml") {
			continue
		}
		spec, err := LoadExtensionSpec(filepath.Join(extDir, entry.Name()))
		if err != nil {
			return nil, err
		}
		extensions[spec.Name] = spec
	}

	return &Catalog{PHP: php, Extensions: extensions}, nil
}

// IsBundled reports whether extName is bundled with the given PHP version in
// this catalog. Returns false if the version is unknown or the catalog has no
// entry for it. Matching is exact against the per-version
// bundled_extensions list — callers should pass the normalized (lowercase)
// identifier.
func (c *Catalog) IsBundled(phpVersion, extName string) bool {
	if c.PHP == nil {
		return false
	}
	vs, ok := c.PHP.Versions[phpVersion]
	if !ok || vs == nil {
		return false
	}
	return slices.Contains(vs.BundledExtensions, extName)
}

func (c *Catalog) RequiresSeparateBundle(extName string) bool {
	spec, ok := c.Extensions[extName]
	if !ok {
		return false
	}
	return spec.Kind != ExtensionKindBundled
}

func (s *PHPSpec) Validate() error {
	if s.Name == "" {
		return fmt.Errorf("PHP spec: name is required")
	}
	if len(s.Versions) == 0 {
		return fmt.Errorf("PHP spec: at least one version is required")
	}
	hasBuild := false
	for v, vs := range s.Versions {
		if vs == nil {
			return fmt.Errorf("PHP spec %s: version entry is nil", v)
		}
		if len(vs.BundledExtensions) == 0 {
			return fmt.Errorf("PHP spec %s: bundled_extensions is required", v)
		}
		if vs.Sources != nil {
			hasBuild = true
			if vs.Sources.URL == "" {
				return fmt.Errorf("PHP spec %s: sources.url is required when sources is set", v)
			}
			if len(vs.ABIMatrix.OS) == 0 || len(vs.ABIMatrix.Arch) == 0 || len(vs.ABIMatrix.TS) == 0 {
				return fmt.Errorf("PHP spec %s: abi_matrix must have at least one value in each dimension", v)
			}
		}
		if err := ValidateHermeticLibs(vs.HermeticLibs); err != nil {
			return fmt.Errorf("PHP spec %s: %w", v, err)
		}
	}
	if !hasBuild {
		return fmt.Errorf("PHP spec: at least one version must have sources set for the builder to run")
	}
	return nil
}

func (s *ExtensionSpec) Validate() error {
	if s.Name == "" {
		return fmt.Errorf("extension spec: name is required")
	}
	if s.Kind == "" {
		return fmt.Errorf("extension spec %s: kind is required", s.Name)
	}
	if s.Kind == ExtensionKindPECL {
		if s.Source.PECLPackage == "" {
			return fmt.Errorf("extension spec %s: source.pecl_package is required for PECL extensions", s.Name)
		}
		if len(s.Versions) == 0 {
			return fmt.Errorf("extension spec %s: at least one version is required", s.Name)
		}
	}
	if err := ValidateHermeticLibs(s.HermeticLibs); err != nil {
		return fmt.Errorf("extension spec %s: %w", s.Name, err)
	}
	return nil
}

// ValidateHermeticLibs validates a list of hermetic-lib globs. Globs must look
// like shared-library filenames (starting with "lib", ending with ".so" or
// ".so.<soversion>"), must not contain path separators, and must either contain
// a wildcard or end with a concrete SOVERSION. Duplicates are rejected so
// catalog authors don't silently list the same glob twice.
//
// Pattern: lib<chars>.so or lib<chars>.so.<soversion-or-*>
// The grammar is intentionally narrow; extend only with a test-driven reason.
func ValidateHermeticLibs(globs []string) error {
	pattern := regexp.MustCompile(`^lib[A-Za-z0-9._+-]+\.so(\.[A-Za-z0-9.*]+)?$`)
	seen := make(map[string]struct{}, len(globs))
	for _, g := range globs {
		if strings.Contains(g, "/") {
			return fmt.Errorf("hermetic_libs glob %q must not contain '/'", g)
		}
		if !pattern.MatchString(g) {
			return fmt.Errorf("hermetic_libs glob %q must match lib<name>.so[.<soversion>]", g)
		}
		// Require either a wildcard or a concrete SOVERSION past ".so".
		if !strings.Contains(g, "*") && !strings.Contains(g, ".so.") {
			return fmt.Errorf("hermetic_libs glob %q must contain '*' or a SOVERSION (e.g. libfoo.so.*)", g)
		}
		if _, dup := seen[g]; dup {
			return fmt.Errorf("hermetic_libs contains duplicate entry %q", g)
		}
		seen[g] = struct{}{}
	}
	return nil
}
