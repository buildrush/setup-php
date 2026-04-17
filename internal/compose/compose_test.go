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

func TestWriteIniValuesWithDefaults(t *testing.T) {
	dir := t.TempDir()
	defaults := map[string]string{
		"memory_limit":  "-1",
		"date.timezone": "UTC",
	}
	user := []plan.IniValue{
		{Key: "memory_limit", Value: "512M"}, // overrides default
		{Key: "display_errors", Value: "On"},
	}

	if err := WriteIniValuesWithDefaults(dir, defaults, user); err != nil {
		t.Fatalf("WriteIniValuesWithDefaults: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "99-user.ini"))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	s := string(data)

	if !strings.Contains(s, "memory_limit=512M\n") {
		t.Errorf("user override not present; got:\n%s", s)
	}
	// Verify the user override line comes AFTER the default line
	defaultIdx := strings.Index(s, "memory_limit=-1\n")
	userIdx := strings.Index(s, "memory_limit=512M\n")
	if defaultIdx == -1 {
		t.Errorf("default memory_limit not present; got:\n%s", s)
	} else if userIdx <= defaultIdx {
		t.Errorf("user override at idx %d should come after default at idx %d; got:\n%s",
			userIdx, defaultIdx, s)
	}
	if !strings.Contains(s, "date.timezone=UTC\n") {
		t.Errorf("untouched default missing; got:\n%s", s)
	}
	if !strings.Contains(s, "display_errors=On\n") {
		t.Errorf("user-only value missing; got:\n%s", s)
	}
}

func TestWriteIniValuesWithDefaultsSorted(t *testing.T) {
	dir := t.TempDir()
	defaults := map[string]string{
		"z_key": "1",
		"a_key": "2",
		"m_key": "3",
	}
	if err := WriteIniValuesWithDefaults(dir, defaults, nil); err != nil {
		t.Fatalf("WriteIniValuesWithDefaults: %v", err)
	}
	data, _ := os.ReadFile(filepath.Join(dir, "99-user.ini"))
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 3 {
		t.Fatalf("got %d lines, want 3: %v", len(lines), lines)
	}
	// Defaults must appear in sorted key order.
	if !strings.HasPrefix(lines[0], "a_key=") ||
		!strings.HasPrefix(lines[1], "m_key=") ||
		!strings.HasPrefix(lines[2], "z_key=") {
		t.Errorf("defaults not sorted; got:\n%v", lines)
	}
}

func TestWriteIniValuesWithDefaultsEmpty(t *testing.T) {
	dir := t.TempDir()
	if err := WriteIniValuesWithDefaults(dir, nil, nil); err != nil {
		t.Fatalf("WriteIniValuesWithDefaults: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "99-user.ini")); !os.IsNotExist(err) {
		t.Errorf("expected no file when both defaults and user are empty, err=%v", err)
	}
}

func TestWriteDisableExtension(t *testing.T) {
	dir := t.TempDir()
	if err := WriteDisableExtension(dir, "opcache"); err != nil {
		t.Fatalf("WriteDisableExtension: %v", err)
	}

	matches, err := filepath.Glob(filepath.Join(dir, "*.ini"))
	if err != nil || len(matches) != 1 {
		t.Fatalf("expected 1 ini file, got %v (err=%v)", matches, err)
	}
	name := filepath.Base(matches[0])
	if !strings.HasPrefix(name, "00-") {
		t.Errorf("disable fragment must have 00- prefix to sort early; got %s", name)
	}
	if !strings.Contains(name, "opcache") {
		t.Errorf("disable fragment filename should mention opcache; got %s", name)
	}

	data, _ := os.ReadFile(matches[0])
	if !strings.Contains(string(data), "opcache") {
		t.Errorf("disable fragment body should mention opcache; got:\n%s", data)
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
