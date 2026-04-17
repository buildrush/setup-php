package compose

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/buildrush/setup-php/internal/plan"
)

type Layout struct {
	RootDir      string
	BinDir       string
	ExtensionDir string
	ConfDir      string
	IncludeDir   string
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
