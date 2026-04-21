package compose

import (
	"os"
	"path/filepath"
	"reflect"
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

// Regression: PHP 8.5 cores install opcache statically (not shared), so the
// extracted bundle has no usr/local/lib/php/extensions/ directory. Composing
// a PECL extension against such a core must still succeed — SymlinkExtension
// creates the target directory on demand rather than failing on ENOENT.
func TestSymlinkExtensionCreatesMissingTargetDir(t *testing.T) {
	dir := t.TempDir()
	extDir := filepath.Join(dir, "usr", "local", "lib", "php", "extensions")
	// deliberately do NOT mkdir extDir — simulates an 8.5 core bundle

	soDir := filepath.Join(dir, "bundle")
	os.MkdirAll(soDir, 0o755)
	soPath := filepath.Join(soDir, "pcov.so")
	os.WriteFile(soPath, []byte("fake so"), 0o644)

	if err := SymlinkExtension(soPath, extDir, "pcov"); err != nil {
		t.Fatalf("SymlinkExtension() error = %v", err)
	}

	link := filepath.Join(extDir, "pcov.so")
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

func TestSelectBaseIniFile_Production(t *testing.T) {
	root := t.TempDir()
	tmpl := filepath.Join(root, "share", "php", "ini")
	lib := filepath.Join(root, "lib")
	if err := os.MkdirAll(tmpl, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(lib, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmpl, "php.ini-production"), []byte("; production\ndisplay_errors=Off\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	layout := &Layout{
		IniTemplateDir: tmpl,
		IniFile:        filepath.Join(lib, "php.ini"),
	}
	if err := SelectBaseIniFile(layout, "php.ini-production"); err != nil {
		t.Fatalf("SelectBaseIniFile: %v", err)
	}
	got, err := os.ReadFile(layout.IniFile)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "; production\ndisplay_errors=Off\n" {
		t.Errorf("effective php.ini = %q", string(got))
	}
}

func TestSelectBaseIniFile_Development(t *testing.T) {
	root := t.TempDir()
	tmpl := filepath.Join(root, "share", "php", "ini")
	lib := filepath.Join(root, "lib")
	if err := os.MkdirAll(tmpl, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(lib, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmpl, "php.ini-development"), []byte("; development\ndisplay_errors=On\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	layout := &Layout{
		IniTemplateDir: tmpl,
		IniFile:        filepath.Join(lib, "php.ini"),
	}
	if err := SelectBaseIniFile(layout, "php.ini-development"); err != nil {
		t.Fatalf("SelectBaseIniFile: %v", err)
	}
	got, err := os.ReadFile(layout.IniFile)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "; development\ndisplay_errors=On\n" {
		t.Errorf("effective php.ini = %q", string(got))
	}
}

func TestSelectBaseIniFile_None(t *testing.T) {
	root := t.TempDir()
	lib := filepath.Join(root, "lib")
	if err := os.MkdirAll(lib, 0o755); err != nil {
		t.Fatal(err)
	}
	layout := &Layout{
		IniTemplateDir: filepath.Join(root, "share", "php", "ini"), // does not exist; must not matter
		IniFile:        filepath.Join(lib, "php.ini"),
	}
	if err := SelectBaseIniFile(layout, ""); err != nil {
		t.Fatalf("SelectBaseIniFile(\"\"): %v", err)
	}
	info, err := os.Stat(layout.IniFile)
	if err != nil {
		t.Fatalf("effective php.ini missing after `none`: %v", err)
	}
	if info.Size() != 0 {
		t.Errorf("effective php.ini not empty: size=%d", info.Size())
	}
}

func TestSelectBaseIniFile_MissingSourceFails(t *testing.T) {
	root := t.TempDir()
	tmpl := filepath.Join(root, "share", "php", "ini")
	lib := filepath.Join(root, "lib")
	if err := os.MkdirAll(tmpl, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(lib, 0o755); err != nil {
		t.Fatal(err)
	}
	layout := &Layout{
		IniTemplateDir: tmpl,
		IniFile:        filepath.Join(lib, "php.ini"),
	}
	err := SelectBaseIniFile(layout, "php.ini-production")
	if err == nil {
		t.Fatal("expected error for missing source, got nil")
	}
	if !strings.Contains(err.Error(), "php.ini-production") {
		t.Errorf("error does not mention filename: %v", err)
	}
}

func TestMergeCompatLayers_OrderAndPrecedence(t *testing.T) {
	defaults := map[string]string{"memory_limit": "-1", "opcache.enable": "1"}
	xdebug := map[string]string{"xdebug.mode": "coverage"}
	extra := map[string]string{"pcov.enabled": "1"}

	// extra beats xdebug beats defaults, so test a conflict:
	conflicting := map[string]string{"memory_limit": "512M"} // would overwrite default
	got := MergeCompatLayers(defaults, nil, conflicting)
	if got["memory_limit"] != "512M" {
		t.Errorf("extra should override defaults: got %q", got["memory_limit"])
	}

	got = MergeCompatLayers(defaults, xdebug, extra)
	want := map[string]string{
		"memory_limit":   "-1",
		"opcache.enable": "1",
		"xdebug.mode":    "coverage",
		"pcov.enabled":   "1",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("MergeCompatLayers = %v, want %v", got, want)
	}

	// nil layers treated as empty
	got = MergeCompatLayers(defaults, nil, nil)
	if !reflect.DeepEqual(got, defaults) {
		t.Errorf("with nil layers should equal defaults: got %v", got)
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

func TestAssertBundleSchema_BlocksBelowMinimum(t *testing.T) {
	err := AssertBundleSchema("php-core", 0) // synthetic bundle below min
	if err == nil {
		t.Fatal("expected error when schema below minimum, got nil")
	}
	if !strings.Contains(err.Error(), "schema_version") {
		t.Errorf("error must mention schema_version, got: %v", err)
	}
	if !strings.Contains(err.Error(), "php-core") {
		t.Errorf("error must mention the kind (php-core), got: %v", err)
	}
}

func TestAssertBundleSchema_AllowsAtOrAboveMinimum(t *testing.T) {
	if err := AssertBundleSchema("php-core", 2); err != nil {
		t.Errorf("expected nil at exact min, got %v", err)
	}
	if err := AssertBundleSchema("php-core", 99); err != nil {
		t.Errorf("expected nil above min, got %v", err)
	}
}

func TestAssertBundleSchema_UnknownKind_Allows(t *testing.T) {
	// Unknown kind → min is 0 → any schema passes. The caller is expected
	// to have validated the kind elsewhere.
	if err := AssertBundleSchema("php-tool", 7); err != nil {
		t.Errorf("unknown kind must not error, got %v", err)
	}
}
