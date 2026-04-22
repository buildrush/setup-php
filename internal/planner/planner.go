package planner

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/buildrush/setup-php/internal/catalog"
)

// MatrixCell represents one concrete build job.
type MatrixCell struct {
	Version   string `json:"version,omitempty"`
	OS        string `json:"os"`
	Arch      string `json:"arch"`
	TS        string `json:"ts"`
	Digest    string `json:"digest,omitempty"`
	SpecHash  string `json:"spec_hash,omitempty"`
	Extension string `json:"extension,omitempty"`
	ExtVer    string `json:"ext_version,omitempty"`
	PHPAbi    string `json:"php_abi,omitempty"`
}

// Matrix is the GitHub Actions matrix JSON format.
type Matrix struct {
	Include []MatrixCell `json:"include"`
}

// Result contains the three output matrices.
type Result struct {
	PHP  Matrix
	Ext  Matrix
	Tool Matrix
}

// ExpandPHPMatrix expands php.yaml's per-version abi_matrix into concrete
// build cells. Only versions with `sources:` set contribute cells — the rest
// encode compat intent and are intentionally skipped here.
func ExpandPHPMatrix(spec *catalog.PHPSpec) []MatrixCell {
	var cells []MatrixCell
	for _, target := range spec.BuildTargets() {
		for _, osName := range target.Spec.ABIMatrix.OS {
			for _, arch := range target.Spec.ABIMatrix.Arch {
				for _, ts := range target.Spec.ABIMatrix.TS {
					cells = append(cells, MatrixCell{
						Version: target.Version,
						OS:      osName,
						Arch:    arch,
						TS:      ts,
					})
				}
			}
		}
	}
	return cells
}

// ExpandExtMatrix expands an extension's abi_matrix, applying excludes.
func ExpandExtMatrix(spec *catalog.ExtensionSpec) []MatrixCell {
	if spec.Kind == catalog.ExtensionKindBundled {
		return nil
	}

	var cells []MatrixCell
	for _, ver := range spec.Versions {
		for _, php := range spec.ABIMatrix.PHP {
			for _, osName := range spec.ABIMatrix.OS {
				for _, arch := range spec.ABIMatrix.Arch {
					for _, ts := range spec.ABIMatrix.TS {
						if isExcluded(spec.Exclude, osName, arch, php) {
							continue
						}
						cells = append(cells, MatrixCell{
							Extension: spec.Name,
							ExtVer:    ver,
							PHPAbi:    fmt.Sprintf("%s-%s", php, ts),
							OS:        osName,
							Arch:      arch,
							TS:        ts,
						})
					}
				}
			}
		}
	}
	return cells
}

func isExcluded(rules []catalog.ExcludeRule, osName, arch, php string) bool {
	for _, r := range rules {
		match := true
		if r.OS != "" && r.OS != osName {
			match = false
		}
		if r.Arch != "" && r.Arch != arch {
			match = false
		}
		if r.PHP != "" && r.PHP != php {
			match = false
		}
		if match {
			return true
		}
	}
	return false
}

// ComputeSpecHash computes a deterministic hash for a matrix cell.
// builderOS is the pinned builder runner OS (e.g. "ubuntu-22.04"), read from
// builders/common/builder-os.env by the caller. Including it here means a
// runner-OS change invalidates every cell's spec_hash and forces the pipeline
// to rebuild every bundle on the new OS, guaranteeing the lockfile and the
// physical bundles stay in sync.
func ComputeSpecHash(cell *MatrixCell, catalogData []byte, builderHash, builderOS string) string {
	h := sha256.New()
	h.Write(catalogData)
	h.Write([]byte(builderHash))
	h.Write([]byte(builderOS))
	_, _ = fmt.Fprintf(h, "%s:%s:%s:%s:%s:%s",
		cell.Version, cell.Extension, cell.OS, cell.Arch, cell.TS, cell.PHPAbi)
	return fmt.Sprintf("sha256:%x", h.Sum(nil))
}

// HashFile returns "sha256:<hex>" of file contents. Intentionally bails on
// missing files so callers don't silently produce an empty spec_hash and skip
// rebuilds when a builder script has been deleted/moved.
func HashFile(path string) (string, error) {
	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return "", fmt.Errorf("hash file %s: %w", path, err)
	}
	sum := sha256.Sum256(data)
	return fmt.Sprintf("sha256:%x", sum[:]), nil
}

// ReadBuilderOS reads the pinned BUILDER_OS value from
// builders/common/builder-os.env. The file format is one or more VAR=value
// lines; only the BUILDER_OS key is consulted. Missing file or missing key
// produces a clear error so callers can bail early with a recognisable
// diagnostic (matches the HashFile missing-file contract).
func ReadBuilderOS(path string) (string, error) {
	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return "", fmt.Errorf("read builder-os env %s: %w", path, err)
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		if strings.TrimSpace(key) == "BUILDER_OS" {
			return strings.TrimSpace(value), nil
		}
	}
	return "", fmt.Errorf("BUILDER_OS not set in %s", path)
}

// PerVersionYAML marshals a single version entry from a PHP spec into
// canonical YAML, suitable for hashing. Only the named version is included;
// unrelated versions and the top-level Name/Smoke fields are omitted so that
// editing one version does not bust the hash of others.
func PerVersionYAML(spec *catalog.PHPSpec, version string) ([]byte, error) {
	vs, ok := spec.Versions[version]
	if !ok {
		return nil, fmt.Errorf("version %q not in spec", version)
	}
	return yaml.Marshal(vs)
}

// ExtensionYAML marshals an extension spec into canonical YAML for hashing.
func ExtensionYAML(spec *catalog.ExtensionSpec) ([]byte, error) {
	return yaml.Marshal(spec)
}

// WriteMatrices writes php.json, ext.json, tool.json to the output directory.
func WriteMatrices(result *Result, outputDir string) error {
	if err := os.MkdirAll(outputDir, 0o750); err != nil {
		return err
	}
	// Ensure empty Include slices are non-nil for valid JSON
	if result.PHP.Include == nil {
		result.PHP.Include = []MatrixCell{}
	}
	if result.Ext.Include == nil {
		result.Ext.Include = []MatrixCell{}
	}
	if result.Tool.Include == nil {
		result.Tool.Include = []MatrixCell{}
	}

	for name, m := range map[string]Matrix{
		"php.json":  result.PHP,
		"ext.json":  result.Ext,
		"tool.json": result.Tool,
	} {
		data, err := json.Marshal(m)
		if err != nil {
			return fmt.Errorf("marshal %s: %w", name, err)
		}
		if err := os.WriteFile(filepath.Join(outputDir, name), data, 0o600); err != nil {
			return fmt.Errorf("write %s: %w", name, err)
		}
	}
	return nil
}
