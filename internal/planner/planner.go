package planner

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/buildrush/setup-php/internal/catalog"
)

// MatrixCell represents one concrete build job.
type MatrixCell struct {
	Version   string `json:"version,omitempty"`
	OS        string `json:"os"`
	Arch      string `json:"arch"`
	TS        string `json:"ts"`
	Digest    string `json:"digest,omitempty"`
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

// ExpandPHPMatrix expands php.yaml's abi_matrix into concrete cells.
func ExpandPHPMatrix(spec *catalog.PHPSpec) []MatrixCell {
	cells := make([]MatrixCell, 0, len(spec.Versions)*len(spec.ABIMatrix.OS)*len(spec.ABIMatrix.Arch)*len(spec.ABIMatrix.TS))
	for _, ver := range spec.Versions {
		for _, osName := range spec.ABIMatrix.OS {
			for _, arch := range spec.ABIMatrix.Arch {
				for _, ts := range spec.ABIMatrix.TS {
					cells = append(cells, MatrixCell{
						Version: ver,
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
func ComputeSpecHash(cell *MatrixCell, catalogData []byte, builderHash string) string {
	h := sha256.New()
	h.Write(catalogData)
	h.Write([]byte(builderHash))
	_, _ = fmt.Fprintf(h, "%s:%s:%s:%s:%s:%s",
		cell.Version, cell.Extension, cell.OS, cell.Arch, cell.TS, cell.PHPAbi)
	return fmt.Sprintf("sha256:%x", h.Sum(nil))
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
