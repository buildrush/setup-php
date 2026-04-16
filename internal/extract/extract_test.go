package extract

import (
	"archive/tar"
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/klauspost/compress/zstd"
)

func createTestBundle(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var tarBuf bytes.Buffer
	tw := tar.NewWriter(&tarBuf)
	for name, content := range files {
		hdr := &tar.Header{
			Name: name,
			Mode: 0o644,
			Size: int64(len(content)),
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	tw.Close()

	var zstdBuf bytes.Buffer
	enc, _ := zstd.NewWriter(&zstdBuf)
	enc.Write(tarBuf.Bytes())
	enc.Close()

	return zstdBuf.Bytes()
}

func TestExtract(t *testing.T) {
	data := createTestBundle(t, map[string]string{
		"bin/php":       "#!/bin/php",
		"lib/libphp.so": "library content",
	})

	dir := t.TempDir()
	err := Extract(data, Options{TargetDir: dir})
	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}

	content, err := os.ReadFile(filepath.Join(dir, "bin", "php"))
	if err != nil {
		t.Fatalf("ReadFile(bin/php) error = %v", err)
	}
	if string(content) != "#!/bin/php" {
		t.Errorf("bin/php content = %q, want %q", string(content), "#!/bin/php")
	}
}

func TestExtractCorrupted(t *testing.T) {
	err := Extract([]byte("not a valid archive"), Options{TargetDir: t.TempDir()})
	if err == nil {
		t.Error("Extract() should return error for corrupted data")
	}
}

func TestExtractParallel(t *testing.T) {
	data1 := createTestBundle(t, map[string]string{"file1.txt": "content1"})
	data2 := createTestBundle(t, map[string]string{"file2.txt": "content2"})

	dir1 := t.TempDir()
	dir2 := t.TempDir()

	err := ExtractParallel([]ExtractItem{
		{Data: data1, Opts: Options{TargetDir: dir1}},
		{Data: data2, Opts: Options{TargetDir: dir2}},
	})
	if err != nil {
		t.Fatalf("ExtractParallel() error = %v", err)
	}

	if _, err := os.Stat(filepath.Join(dir1, "file1.txt")); err != nil {
		t.Error("file1.txt should exist in dir1")
	}
	if _, err := os.Stat(filepath.Join(dir2, "file2.txt")); err != nil {
		t.Error("file2.txt should exist in dir2")
	}
}
