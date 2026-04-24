package planner

import (
	"strings"

	"github.com/buildrush/setup-php/internal/catalog"
)

// HashFiles computes a deterministic, order-sensitive hash string across the
// given files. The output is the colon-joined concatenation of each file's
// individual HashFile result, i.e. for paths=[a, b, c] it returns
// "sha256:<a>:sha256:<b>:sha256:<c>". This is the same shape the planner has
// historically produced by manually concatenating HashFile calls, so using
// HashFiles in place of that inline code keeps spec_hash values byte-identical.
//
// Missing or unreadable files bail via HashFile's error semantics so callers
// cannot silently produce an empty prefix and skip rebuilds when a builder
// script has been deleted or moved.
func HashFiles(paths []string) (string, error) {
	parts := make([]string, 0, len(paths))
	for _, p := range paths {
		h, err := HashFile(p)
		if err != nil {
			return "", err
		}
		parts = append(parts, h)
	}
	return strings.Join(parts, ":"), nil
}

// PerVersionYAMLFromFile loads the PHP catalog YAML at path and returns the
// marshaled YAML for a single version, suitable for hashing. It wraps
// catalog.LoadPHPSpec + PerVersionYAML so callers that have only a file path
// (e.g. phpup build) get the same byte-level output as the planner. Unknown
// versions are an error; see PerVersionYAML for the marshaling contract.
func PerVersionYAMLFromFile(path, version string) ([]byte, error) {
	spec, err := catalog.LoadPHPSpec(path)
	if err != nil {
		return nil, err
	}
	return PerVersionYAML(spec, version)
}

// ExtensionYAMLFromFile loads an extension catalog YAML at path and returns
// the marshaled YAML for hashing. It wraps catalog.LoadExtensionSpec +
// ExtensionYAML so callers that have only a file path get the same byte-level
// output as the planner.
func ExtensionYAMLFromFile(path string) ([]byte, error) {
	spec, err := catalog.LoadExtensionSpec(path)
	if err != nil {
		return nil, err
	}
	return ExtensionYAML(spec)
}

// ParseEnvValue scans a minimal .env file byte slice and returns the value for
// the given key, or "" if the key is absent. Lines are split on "\n"; the
// first line matching "<key>=<value>" (after trimming) wins. Blank lines and
// lines starting with "#" are ignored. Values are not unquoted — callers that
// need quote handling must do it themselves. This parser is deliberately
// minimal (it only needs to extract BUILDER_OS for the planner's spec-hash);
// extend only with a test-driven reason.
func ParseEnvValue(data []byte, key string) string {
	prefix := key + "="
	for _, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if strings.HasPrefix(trimmed, prefix) {
			return strings.TrimPrefix(trimmed, prefix)
		}
	}
	return ""
}
