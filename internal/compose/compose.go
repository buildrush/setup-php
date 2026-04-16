package compose

import (
	"fmt"
	"os"
	"path/filepath"
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

func WriteIniValues(confDir string, values []plan.IniValue) error {
	if len(values) == 0 {
		return nil
	}
	var lines []string
	for _, v := range values {
		lines = append(lines, fmt.Sprintf("%s=%s", v.Key, v.Value))
	}
	path := filepath.Join(confDir, "99-user.ini")
	content := strings.Join(lines, "\n") + "\n"
	return os.WriteFile(path, []byte(content), 0o600)
}
