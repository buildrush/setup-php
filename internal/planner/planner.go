package planner

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"

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

	// CoreDigest is the OCI manifest digest of the prerequisite php-core
	// bundle for this ext cell (e.g., "sha256:abc..."). Populated ONLY for
	// ext cells; zero for php/tool cells. Surfaced in the emitted matrix
	// JSON as `core_digest` so downstream callers can pass it to
	// `phpup build ext --php-core-digest`. omitempty keeps the JSON
	// backward-compatible — php/tool cells don't gain a noisy empty field.
	CoreDigest string `json:"core_digest,omitempty"`
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
//
// coreDigestByKey maps a canonical PHP bundle key (matching
// lockfile.PHPBundleKey — "php:<ver>:<os>:<arch>:<ts>") to the resolved OCI
// digest of the prerequisite php-core bundle. The resolved digest (if any) is
// stored on each ext cell's CoreDigest field. Pass nil if no digest context
// is available; cells will have empty CoreDigest and a warning is logged per
// unresolved cell. The zero-value behavior intentionally matches the existing
// "missing ABI row" case (no silent skip) — so Task 5's consumer must treat
// empty CoreDigest as a hard error, not as "fall back to tag-form".
func ExpandExtMatrix(spec *catalog.ExtensionSpec, coreDigestByKey map[string]string) []MatrixCell {
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
						coreKey := fmt.Sprintf("php:%s:%s:%s:%s", php, osName, arch, ts)
						digest := coreDigestByKey[coreKey]
						if digest == "" && coreDigestByKey != nil {
							log.Printf("WARN: ExpandExtMatrix: no core digest for ext=%s ext_ver=%s php=%s os=%s arch=%s ts=%s (key=%s); cell will have empty CoreDigest",
								spec.Name, ver, php, osName, arch, ts, coreKey)
						}
						cells = append(cells, MatrixCell{
							Extension:  spec.Name,
							ExtVer:     ver,
							PHPAbi:     fmt.Sprintf("%s-%s", php, ts),
							OS:         osName,
							Arch:       arch,
							TS:         ts,
							CoreDigest: digest,
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

// ComputeSpecHash computes a deterministic hash for a matrix cell. builderOS
// is the runner OS the build will execute on — changing it forces a full
// rebuild because the resulting bundle's hermetic-lib set differs.
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
