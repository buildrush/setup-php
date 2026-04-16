package cache

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCheckHit(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("RUNNER_TOOL_CACHE", dir)

	s, err := NewStore()
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}

	cacheDir := filepath.Join(s.CacheDir(), "abc123")
	os.MkdirAll(cacheDir, 0o755)
	os.WriteFile(filepath.Join(cacheDir, ".complete"), []byte("1"), 0o644)

	result := s.Check("abc123")
	if !result.Hit {
		t.Error("Check() should return hit when cache exists")
	}
	if result.Path != cacheDir {
		t.Errorf("Path = %q, want %q", result.Path, cacheDir)
	}
}

func TestCheckMiss(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("RUNNER_TOOL_CACHE", dir)

	s, _ := NewStore()
	result := s.Check("nonexistent")
	if result.Hit {
		t.Error("Check() should return miss for nonexistent hash")
	}
}

func TestSeed(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("RUNNER_TOOL_CACHE", dir)

	s, _ := NewStore()

	srcDir := filepath.Join(t.TempDir(), "source")
	os.MkdirAll(filepath.Join(srcDir, "bin"), 0o755)
	os.WriteFile(filepath.Join(srcDir, "bin", "php"), []byte("#!/bin/php"), 0o755)

	if err := s.Seed("hash123", srcDir); err != nil {
		t.Fatalf("Seed() error = %v", err)
	}

	result := s.Check("hash123")
	if !result.Hit {
		t.Error("Check() should return hit after Seed()")
	}
}

func TestNewStoreNoEnv(t *testing.T) {
	t.Setenv("RUNNER_TOOL_CACHE", "")

	s, err := NewStore()
	if err != nil {
		t.Fatalf("NewStore() should not error without RUNNER_TOOL_CACHE, got %v", err)
	}
	if s.CacheDir() == "" {
		t.Error("CacheDir() should not be empty even without RUNNER_TOOL_CACHE")
	}
}
