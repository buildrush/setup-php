package compose

import (
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/buildrush/setup-php/internal/plan"
	"github.com/buildrush/setup-php/internal/version"
)

type Layout struct {
	RootDir        string
	BinDir         string
	ExtensionDir   string
	ConfDir        string
	IncludeDir     string
	IniTemplateDir string // <bundle>/usr/local/share/php/ini
	IniFile        string // <bundle>/usr/local/lib/php.ini (written by SelectBaseIniFile)
}

type ExtensionComposition struct {
	Name     string
	SOPath   string
	IniLines []string
}

func Compose(layout *Layout, extensions []ExtensionComposition) error {
	// PHP was built with --prefix=/usr/local so its compile-time extension_dir
	// points to /usr/local/lib/... Override it to the actual bundle location.
	if err := writeExtensionDirIni(layout); err != nil {
		return fmt.Errorf("write extension_dir ini: %w", err)
	}
	for _, ext := range extensions {
		if err := SymlinkExtension(ext.SOPath, layout.ExtensionDir, ext.Name); err != nil {
			return fmt.Errorf("symlink extension %s: %w", ext.Name, err)
		}
		if len(ext.IniLines) > 0 {
			if err := WriteIniFragment(layout.ConfDir, ext.Name, ext.IniLines); err != nil {
				return fmt.Errorf("write ini for %s: %w", ext.Name, err)
			}
		}
	}
	return nil
}

func writeExtensionDirIni(layout *Layout) error {
	// PHP's default extension_dir (compiled at build time with --prefix=/usr/local)
	// does not match where our bundle is extracted. Override it to the actual
	// directory where SymlinkExtension places the .so files.
	path := filepath.Join(layout.ConfDir, "00-extension-dir.ini")
	content := fmt.Sprintf("extension_dir=%s\n", layout.ExtensionDir)
	return os.WriteFile(path, []byte(content), 0o600)
}

func SymlinkExtension(soPath, extensionDir, name string) error {
	// PHP 8.5 cores install opcache statically, so the extracted bundle has
	// no extensions/ subdirectory. Earlier PHP cores happened to have one
	// (shared opcache created it). Create on demand so the compose layer is
	// independent of the core's shared-vs-static module choice.
	if err := os.MkdirAll(extensionDir, 0o750); err != nil {
		return fmt.Errorf("create extension dir: %w", err)
	}
	link := filepath.Join(extensionDir, name+".so")
	if err := os.Remove(link); err != nil && !os.IsNotExist(err) {
		return err
	}
	return os.Symlink(soPath, link)
}

func WriteIniFragment(confDir, extName string, lines []string) error {
	path := filepath.Join(confDir, extName+".ini")
	content := strings.Join(lines, "\n") + "\n"
	return os.WriteFile(path, []byte(content), 0o600)
}

// WriteIniValuesWithDefaults writes compat default ini values first, then user
// values on top — later writes win because PHP processes the file top-to-bottom.
// Default keys are written in deterministic (sorted) order. User values are
// written in their supplied order, matching how shivammathur/setup-php@v2
// appends user-specified ini-values on top of the base ini.
//
// If both maps are empty, no file is written and nil is returned.
func WriteIniValuesWithDefaults(confDir string, defaults map[string]string, user []plan.IniValue) error {
	if len(defaults) == 0 && len(user) == 0 {
		return nil
	}

	defKeys := make([]string, 0, len(defaults))
	for k := range defaults {
		defKeys = append(defKeys, k)
	}
	sort.Strings(defKeys)

	var lines []string
	for _, k := range defKeys {
		lines = append(lines, fmt.Sprintf("%s=%s", k, defaults[k]))
	}
	for _, v := range user {
		lines = append(lines, fmt.Sprintf("%s=%s", v.Key, v.Value))
	}

	path := filepath.Join(confDir, "99-user.ini")
	return os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o600)
}

// SelectBaseIniFile copies the bundle's upstream php.ini-{production,development}
// template to the effective php.ini location named by layout.IniFile. An empty
// filename means `ini-file: none` — the target is written empty.
//
// Returns an error if the named source file is not found in layout.IniTemplateDir
// (which would indicate a builder regression since Task 5 guarantees both files
// are present in every bundle).
func SelectBaseIniFile(layout *Layout, filename string) error {
	if filename == "" {
		return os.WriteFile(layout.IniFile, nil, 0o600)
	}
	src := filepath.Join(layout.IniTemplateDir, filename)
	data, err := os.ReadFile(src) //nolint:gosec // src is composed from layout.IniTemplateDir (runtime-owned, not user input) + a value from compat.BaseIniFileName (closed set)
	if err != nil {
		return fmt.Errorf("read base ini template %s: %w", filename, err)
	}
	return os.WriteFile(layout.IniFile, data, 0o600)
}

// MergeCompatLayers returns a single ini-key map composed of the given layers
// in increasing precedence (last write wins). Any layer may be nil and is
// treated as empty.
//
// Callers pass the result as the `defaults` argument to
// WriteIniValuesWithDefaults, which then layers user ini-values over the top.
//
// The returned map is always non-nil (may be empty for a zero-layer call).
func MergeCompatLayers(layers ...map[string]string) map[string]string {
	merged := make(map[string]string)
	for _, layer := range layers {
		maps.Copy(merged, layer)
	}
	return merged
}

// WriteDisableExtension writes a conf.d fragment that documents and enforces
// non-loading of the named extension. The "00-" filename prefix makes it sort
// before extension-loading fragments (this codebase writes them as plain
// "<name>.ini"; see WriteIniFragment).
//
// Because our composed PHP only loads what conf.d explicitly declares, the
// primary mechanism for disabling is simply the absence of the load directive;
// this file provides a durable audit trail and defence-in-depth.
func WriteDisableExtension(confDir, extName string) error {
	path := filepath.Join(confDir, fmt.Sprintf("00-disable-%s.ini", extName))
	content := fmt.Sprintf("; disabled by extensions: :%s\n; extension=%s\n", extName, extName)
	return os.WriteFile(path, []byte(content), 0o600)
}

// AssertBundleSchema ensures the bundle's sidecar schema_version meets
// the runtime's minimum for this kind. A mismatch indicates the bundle
// was built before a runtime feature that now asserts on its contents
// (e.g. the php-core share/php/ini/ requirement introduced in the
// Phase-2 compat closeout, PR #28). Returns a clear error identifying
// both the kind and the required/actual versions so CI and local
// diagnostics are unambiguous. Unknown kinds have minimum 0 and always
// pass.
func AssertBundleSchema(kind string, actual int) error {
	minReq := version.MinBundleSchema(kind)
	if actual >= minReq {
		return nil
	}
	return fmt.Errorf("bundle %s has schema_version=%d; runtime requires >= %d (see docs/bundle-schema-changelog.md)",
		kind, actual, minReq)
}
