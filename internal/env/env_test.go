package env

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAddPath(t *testing.T) {
	dir := t.TempDir()
	pathFile := filepath.Join(dir, "GITHUB_PATH")
	os.WriteFile(pathFile, nil, 0o644)
	t.Setenv("GITHUB_PATH", pathFile)

	e, err := NewExporter()
	if err != nil {
		t.Fatalf("NewExporter() error = %v", err)
	}
	if err := e.AddPath("/opt/php/bin"); err != nil {
		t.Fatalf("AddPath() error = %v", err)
	}
	data, _ := os.ReadFile(pathFile)
	if !strings.Contains(string(data), "/opt/php/bin") {
		t.Errorf("GITHUB_PATH should contain /opt/php/bin, got %q", string(data))
	}
}

func TestSetEnv(t *testing.T) {
	dir := t.TempDir()
	envFile := filepath.Join(dir, "GITHUB_ENV")
	os.WriteFile(envFile, nil, 0o644)
	t.Setenv("GITHUB_ENV", envFile)
	t.Setenv("GITHUB_OUTPUT", filepath.Join(dir, "GITHUB_OUTPUT"))
	t.Setenv("GITHUB_PATH", filepath.Join(dir, "GITHUB_PATH"))
	os.WriteFile(filepath.Join(dir, "GITHUB_OUTPUT"), nil, 0o644)
	os.WriteFile(filepath.Join(dir, "GITHUB_PATH"), nil, 0o644)

	e, _ := NewExporter()
	e.SetEnv("PHP_VERSION", "8.4.6")
	data, _ := os.ReadFile(envFile)
	content := string(data)
	if !strings.Contains(content, "PHP_VERSION") || !strings.Contains(content, "8.4.6") {
		t.Errorf("GITHUB_ENV should contain PHP_VERSION=8.4.6, got %q", content)
	}
}

func TestSetOutput(t *testing.T) {
	dir := t.TempDir()
	outputFile := filepath.Join(dir, "GITHUB_OUTPUT")
	os.WriteFile(outputFile, nil, 0o644)
	t.Setenv("GITHUB_ENV", filepath.Join(dir, "GITHUB_ENV"))
	t.Setenv("GITHUB_OUTPUT", outputFile)
	t.Setenv("GITHUB_PATH", filepath.Join(dir, "GITHUB_PATH"))
	os.WriteFile(filepath.Join(dir, "GITHUB_ENV"), nil, 0o644)
	os.WriteFile(filepath.Join(dir, "GITHUB_PATH"), nil, 0o644)

	e, _ := NewExporter()
	e.SetOutput("php-version", "8.4.6")
	data, _ := os.ReadFile(outputFile)
	if !strings.Contains(string(data), "php-version") {
		t.Errorf("GITHUB_OUTPUT should contain php-version, got %q", string(data))
	}
}

func TestNewExporterMissingVars(t *testing.T) {
	t.Setenv("GITHUB_ENV", "")
	t.Setenv("GITHUB_OUTPUT", "")
	t.Setenv("GITHUB_PATH", "")

	e, err := NewExporter()
	if err != nil {
		t.Fatalf("NewExporter() error = %v", err)
	}
	if err := e.AddPath("/test"); err != nil {
		t.Fatalf("AddPath() should succeed in local mode, error = %v", err)
	}
}
