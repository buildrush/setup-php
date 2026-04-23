// Package build docker-wraps builders/linux/*.sh to produce OCI bundles
// and writes them to a registry.Store. Exposed via `phpup build php|ext`
// subcommands.
package build

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/buildrush/setup-php/internal/planner"
)

// SpecHashInputs groups the inputs needed to compute a bundle's spec-hash.
// All fields are string for CLI-friendliness; the caller is responsible for
// normalising (e.g. "linux" instead of "Linux"). The mapping onto
// planner.MatrixCell depends on Kind:
//
//   - For Kind == "php": Version is the PHP minor (e.g. "8.4"); Name and
//     PHPABI are ignored.
//   - For Kind == "ext": Name is the extension name (e.g. "redis"); Version
//     is the extension version (e.g. "6.2.0") which maps onto
//     MatrixCell.ExtVer; PHPABI is the combined "<php>-<ts>" (e.g. "8.4-nts").
type SpecHashInputs struct {
	Kind    string // "php" or "ext"
	Name    string // empty for PHP; extension name for ext (e.g. "redis")
	Version string // PHP version (e.g. "8.4") or extension version (e.g. "6.2.0")
	OS      string // "linux"
	Arch    string // "x86_64" or "aarch64"
	PHPABI  string // e.g. "8.4-nts" (for ext only)
	TS      string // "nts" or "zts" (for php only)
	Repo    string // absolute path to the setup-php repo root
}

// ComputeSpecHash produces the canonical spec-hash string for the inputs.
// It reads the same builder + catalog files the planner reads and defers to
// planner.ComputeSpecHash for the actual hashing, so local `phpup build`
// invocations and CI planner invocations produce identical hashes for the
// same inputs. The pointer receiver avoids copying the SpecHashInputs struct
// on every call (it's ~128 bytes).
func ComputeSpecHash(in *SpecHashInputs) (string, error) {
	builderFiles, err := builderFilesFor(in.Kind, in.Repo)
	if err != nil {
		return "", err
	}
	builderHash, err := planner.HashFiles(builderFiles)
	if err != nil {
		return "", fmt.Errorf("spechash: hash builders: %w", err)
	}

	builderOS, err := readBuilderOS(filepath.Join(in.Repo, "builders", "common", "builder-os.env"))
	if err != nil {
		return "", fmt.Errorf("spechash: read builder-os.env: %w", err)
	}

	var catalogBytes []byte
	switch in.Kind {
	case "php":
		catalogBytes, err = planner.PerVersionYAMLFromFile(
			filepath.Join(in.Repo, "catalog", "php.yaml"), in.Version)
	case "ext":
		catalogBytes, err = planner.ExtensionYAMLFromFile(
			filepath.Join(in.Repo, "catalog", "extensions", in.Name+".yaml"))
	default:
		return "", fmt.Errorf("spechash: unknown kind %q", in.Kind)
	}
	if err != nil {
		return "", fmt.Errorf("spechash: load catalog: %w", err)
	}

	cell := cellFor(in)
	return planner.ComputeSpecHash(cell, catalogBytes, builderHash, builderOS), nil
}

// cellFor builds the planner.MatrixCell shape the planner would have produced
// for these inputs. For ext entries the planner leaves Version empty and puts
// the extension version in ExtVer; we mirror that exactly because
// planner.ComputeSpecHash's wire format hashes cell.Version (empty for ext)
// and the ext version is instead encoded via the catalog YAML bytes.
func cellFor(in *SpecHashInputs) *planner.MatrixCell {
	switch in.Kind {
	case "php":
		return &planner.MatrixCell{
			Version: in.Version,
			OS:      in.OS,
			Arch:    in.Arch,
			TS:      in.TS,
		}
	case "ext":
		return &planner.MatrixCell{
			Extension: in.Name,
			ExtVer:    in.Version,
			PHPAbi:    in.PHPABI,
			OS:        in.OS,
			Arch:      in.Arch,
			TS:        in.TS,
		}
	}
	return &planner.MatrixCell{}
}

// builderFilesFor returns the ordered list of builder files whose hash
// contributes to this kind's builder hash. Ordering must match the planner's
// historical per-kind concatenation: "build-<kind>.sh" first, then the shared
// common files. For ext builds the planner did NOT historically include
// fetch-core.sh in the hash (it's only sourced at bundle-assembly time, not
// at spec-hash time), so we preserve that.
func builderFilesFor(kind, repo string) ([]string, error) {
	common := []string{
		filepath.Join(repo, "builders", "common", "bundle-schema-version.env"),
		filepath.Join(repo, "builders", "common", "capture-hermetic-libs.sh"),
		filepath.Join(repo, "builders", "common", "pack-bundle.sh"),
	}
	switch kind {
	case "php":
		return append([]string{
			filepath.Join(repo, "builders", "linux", "build-php.sh"),
		}, common...), nil
	case "ext":
		return append([]string{
			filepath.Join(repo, "builders", "linux", "build-ext.sh"),
		}, common...), nil
	}
	return nil, fmt.Errorf("spechash: unknown kind %q", kind)
}

func readBuilderOS(path string) (string, error) {
	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return "", err
	}
	v := planner.ParseEnvValue(data, "BUILDER_OS")
	if v == "" {
		return "", fmt.Errorf("%s: BUILDER_OS not found", path)
	}
	return v, nil
}
