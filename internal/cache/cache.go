package cache

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

type Store struct {
	baseDir string
}

type CacheResult struct {
	Hit  bool
	Path string
}

func NewStore() (*Store, error) {
	base := os.Getenv("RUNNER_TOOL_CACHE")
	if base == "" {
		home, _ := os.UserHomeDir()
		base = filepath.Join(home, ".buildrush", "cache")
	} else {
		base = filepath.Join(base, "buildrush")
	}
	if err := os.MkdirAll(base, 0o750); err != nil {
		return nil, fmt.Errorf("create cache dir: %w", err)
	}
	return &Store{baseDir: base}, nil
}

func (s *Store) CacheDir() string {
	return s.baseDir
}

func (s *Store) Check(planHash string) CacheResult {
	dir := filepath.Join(s.baseDir, planHash)
	marker := filepath.Join(dir, ".complete")
	if _, err := os.Stat(marker); err == nil {
		return CacheResult{Hit: true, Path: dir}
	}
	return CacheResult{Hit: false}
}

func (s *Store) Seed(planHash, sourceDir string) error {
	destDir := filepath.Join(s.baseDir, planHash)
	if err := os.MkdirAll(destDir, 0o750); err != nil {
		return err
	}

	cleanSource := filepath.Clean(sourceDir) + string(os.PathSeparator)
	err := filepath.WalkDir(sourceDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		// Skip symlinks to avoid TOCTOU races
		if d.Type()&fs.ModeSymlink != 0 {
			return nil
		}
		rel, _ := filepath.Rel(sourceDir, path)
		dest := filepath.Join(destDir, rel)
		// Verify path stays within source directory
		if filepath.Clean(path) != filepath.Clean(sourceDir) &&
			!strings.HasPrefix(filepath.Clean(path)+string(os.PathSeparator), cleanSource) {
			return fmt.Errorf("path %q escapes source directory", path)
		}

		if d.IsDir() {
			return os.MkdirAll(dest, 0o750)
		}

		data, err := os.ReadFile(filepath.Clean(path))
		if err != nil {
			return err
		}
		info, _ := d.Info()
		return os.WriteFile(filepath.Clean(dest), data, info.Mode())
	})
	if err != nil {
		return err
	}

	return os.WriteFile(filepath.Join(destDir, ".complete"), []byte("1"), 0o600)
}
