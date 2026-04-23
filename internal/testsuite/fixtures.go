// Package testsuite parses and filters test/compat/fixtures.yaml for use by
// the phpup test subcommand (local+CI test runner).
//
// A Fixture mirrors one entry in that YAML. Filter selects the subset of
// fixtures that apply to a given (os, arch, php) cell, applying tolerant
// aliasing on the runner OS (jammy/noble <-> ubuntu-22.04/24.04) and arch
// (amd64/x86_64 and arm64/aarch64 treated as equivalent).
package testsuite

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Fixture mirrors a single entry in test/compat/fixtures.yaml. Field names
// match shivammathur/setup-php@v2 inputs (php-version, extensions, ini-values,
// coverage) so the same YAML feeds both the legacy compat harness and the
// new phpup test subcommand.
type Fixture struct {
	Name       string `yaml:"name"`
	PHPVersion string `yaml:"php-version"`
	// Extensions is a comma-separated list. Tokens may be "none" (reset),
	// or prefixed with ":" to exclude (e.g. ":opcache"). Empty means no
	// extensions beyond the core bundle.
	Extensions string `yaml:"extensions"`
	// INIValues is a comma-separated list of key=value pairs.
	INIValues string `yaml:"ini-values"`
	// Coverage is "none", "pcov", or "xdebug".
	Coverage string `yaml:"coverage"`
	// INIFile is optional: "production" or "development".
	INIFile string `yaml:"ini-file,omitempty"`
	// Arch is optional; "aarch64" or "x86_64". Empty means x86_64.
	Arch string `yaml:"arch,omitempty"`
	// RunnerOS is optional; e.g. "ubuntu-22.04". Empty means any OS.
	RunnerOS string `yaml:"runner_os,omitempty"`
}

// FixtureSet is the top-level document shape of test/compat/fixtures.yaml.
type FixtureSet struct {
	Fixtures []Fixture `yaml:"fixtures"`
}

// Load reads the YAML file at path and returns the parsed FixtureSet.
func Load(path string) (*FixtureSet, error) {
	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return nil, fmt.Errorf("testsuite.Load: read %s: %w", path, err)
	}
	var s FixtureSet
	if err := yaml.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("testsuite.Load: parse %s: %w", path, err)
	}
	return &s, nil
}

// Filter returns fixtures matching the given (os, arch, php) tuple.
//
// Matching rules:
//   - php: exact match on Fixture.PHPVersion.
//   - arch: Fixture.Arch (default "x86_64" if empty) must equal arch input.
//     Both canonical ("x86_64"/"aarch64") and docker aliases
//     ("amd64"/"arm64") are accepted on the input side and normalized
//     before comparison.
//   - os: if Fixture.RunnerOS == "", the fixture is eligible for any OS.
//     Otherwise Fixture.RunnerOS must equal the os input, with tolerant
//     aliasing: "jammy" <-> "ubuntu-22.04", "noble" <-> "ubuntu-24.04".
//
// The returned slice is never nil; an empty match yields an empty slice.
func (s *FixtureSet) Filter(osName, arch, php string) []Fixture {
	wantArch := normalizeArch(arch)
	wantOS := normalizeOS(osName)
	out := make([]Fixture, 0)
	for i := range s.Fixtures {
		f := &s.Fixtures[i]
		if f.PHPVersion != php {
			continue
		}
		fArch := f.Arch
		if fArch == "" {
			fArch = "x86_64"
		}
		if normalizeArch(fArch) != wantArch {
			continue
		}
		if f.RunnerOS != "" && normalizeOS(f.RunnerOS) != wantOS {
			continue
		}
		out = append(out, *f)
	}
	return out
}

// normalizeArch maps the docker aliases (amd64, arm64) to the canonical
// forms (x86_64, aarch64) used in fixtures.yaml. Unknown inputs are
// returned unchanged so Filter silently drops them instead of panicking.
func normalizeArch(a string) string {
	switch a {
	case "amd64", "x86_64":
		return "x86_64"
	case "arm64", "aarch64":
		return "aarch64"
	}
	return a
}

// normalizeOS maps the short codenames (jammy, noble) to the long
// ubuntu-XX.YY form used in fixtures.yaml so either spelling works on
// input. Unknown inputs are returned unchanged.
func normalizeOS(o string) string {
	switch o {
	case "jammy":
		return "ubuntu-22.04"
	case "noble":
		return "ubuntu-24.04"
	}
	return o
}
