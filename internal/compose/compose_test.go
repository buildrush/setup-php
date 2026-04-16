package compose

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/buildrush/setup-php/internal/plan"
)

func TestSymlinkExtension(t *testing.T) {
	dir := t.TempDir()
	extDir := filepath.Join(dir, "ext")
	os.MkdirAll(extDir, 0o755)

	soDir := filepath.Join(dir, "bundle")
	os.MkdirAll(soDir, 0o755)
	soPath := filepath.Join(soDir, "redis.so")
	os.WriteFile(soPath, []byte("fake so"), 0o644)

	err := SymlinkExtension(soPath, extDir, "redis")
	if err != nil {
		t.Fatalf("SymlinkExtension() error = %v", err)
	}

	link := filepath.Join(extDir, "redis.so")
	target, err := os.Readlink(link)
	if err != nil {
		t.Fatalf("Readlink() error = %v", err)
	}
	if target != soPath {
		t.Errorf("symlink target = %q, want %q", target, soPath)
	}
}

func TestWriteIniFragment(t *testing.T) {
	dir := t.TempDir()
	err := WriteIniFragment(dir, "redis", []string{"extension=redis"})
	if err != nil {
		t.Fatalf("WriteIniFragment() error = %v", err)
	}

	data, _ := os.ReadFile(filepath.Join(dir, "redis.ini"))
	if !strings.Contains(string(data), "extension=redis") {
		t.Errorf("ini content = %q, should contain extension=redis", string(data))
	}
}

func TestWriteIniValues(t *testing.T) {
	dir := t.TempDir()
	vals := []plan.IniValue{
		{Key: "memory_limit", Value: "256M"},
		{Key: "display_errors", Value: "On"},
	}
	err := WriteIniValues(dir, vals)
	if err != nil {
		t.Fatalf("WriteIniValues() error = %v", err)
	}

	data, _ := os.ReadFile(filepath.Join(dir, "99-user.ini"))
	content := string(data)
	if !strings.Contains(content, "memory_limit=256M") {
		t.Errorf("should contain memory_limit=256M, got %q", content)
	}
	if !strings.Contains(content, "display_errors=On") {
		t.Errorf("should contain display_errors=On, got %q", content)
	}
}

func TestCompose(t *testing.T) {
	dir := t.TempDir()
	extDir := filepath.Join(dir, "ext")
	confDir := filepath.Join(dir, "conf.d")
	os.MkdirAll(extDir, 0o755)
	os.MkdirAll(confDir, 0o755)

	soDir := filepath.Join(t.TempDir(), "bundle")
	os.MkdirAll(soDir, 0o755)
	soPath := filepath.Join(soDir, "redis.so")
	os.WriteFile(soPath, []byte("fake"), 0o644)

	layout := &Layout{
		RootDir:      dir,
		BinDir:       filepath.Join(dir, "bin"),
		ExtensionDir: extDir,
		ConfDir:      confDir,
	}
	exts := []ExtensionComposition{
		{Name: "redis", SOPath: soPath, IniLines: []string{"extension=redis"}},
	}

	err := Compose(layout, exts)
	if err != nil {
		t.Fatalf("Compose() error = %v", err)
	}

	if _, err := os.Lstat(filepath.Join(extDir, "redis.so")); err != nil {
		t.Error("redis.so symlink should exist")
	}
	if _, err := os.Stat(filepath.Join(confDir, "redis.ini")); err != nil {
		t.Error("redis.ini should exist")
	}
}
