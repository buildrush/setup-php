package catalog

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"
)

type ExtensionKind string

const (
	ExtensionKindPECL    ExtensionKind = "pecl"
	ExtensionKindBundled ExtensionKind = "bundled"
	ExtensionKindGit     ExtensionKind = "git"
)

type PHPSpec struct {
	Name              string         `yaml:"name"`
	Versions          []string       `yaml:"versions"`
	Source            PHPSource      `yaml:"source"`
	ABIMatrix         ABIMatrix      `yaml:"abi_matrix"`
	ConfigureFlags    ConfigureFlags `yaml:"configure_flags"`
	BundledExtensions []string       `yaml:"bundled_extensions"`
	Smoke             []string       `yaml:"smoke"`
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
	Name        string              `yaml:"name"`
	Kind        ExtensionKind       `yaml:"kind"`
	Note        string              `yaml:"note,omitempty"`
	Source      ExtensionSource     `yaml:"source,omitempty"`
	Versions    []string            `yaml:"versions,omitempty"`
	ABIMatrix   ABIMatrix           `yaml:"abi_matrix,omitempty"`
	Exclude     []ExcludeRule       `yaml:"exclude,omitempty"`
	RuntimeDeps map[string][]string `yaml:"runtime_deps,omitempty"`
	Ini         []string            `yaml:"ini,omitempty"`
	Smoke       []string            `yaml:"smoke,omitempty"`
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

func (c *Catalog) IsBundled(extName string) bool {
	return slices.Contains(c.PHP.BundledExtensions, extName)
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
	if s.Source.URL == "" {
		return fmt.Errorf("PHP spec: source.url is required")
	}
	if len(s.ABIMatrix.OS) == 0 || len(s.ABIMatrix.Arch) == 0 || len(s.ABIMatrix.TS) == 0 {
		return fmt.Errorf("PHP spec: abi_matrix must have at least one value in each dimension")
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
	return nil
}
