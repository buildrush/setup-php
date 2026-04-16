package extract

import (
	"archive/tar"
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/klauspost/compress/zstd"
)

type Options struct {
	TargetDir       string
	StripComponents int
}

type ExtractItem struct {
	Data []byte
	Opts Options
}

func Extract(data []byte, opts Options) error {
	dec, err := zstd.NewReader(bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("zstd decompress: %w", err)
	}
	defer dec.Close()

	tr := tar.NewReader(dec)
	cleanTarget := filepath.Clean(opts.TargetDir) + string(os.PathSeparator)
	const maxFileSize = 256 << 20 // 256 MB

	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("tar read: %w", err)
		}

		target := filepath.Join(opts.TargetDir, hdr.Name)
		if !strings.HasPrefix(filepath.Clean(target)+string(os.PathSeparator), cleanTarget) &&
			filepath.Clean(target) != filepath.Clean(opts.TargetDir) {
			return fmt.Errorf("tar entry %q escapes target directory", hdr.Name)
		}

		mode := os.FileMode(hdr.Mode) & 0o7777

		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, mode); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o750); err != nil {
				return err
			}
			f, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
			if err != nil {
				return err
			}
			if _, err := io.Copy(f, io.LimitReader(tr, maxFileSize)); err != nil {
				_ = f.Close()
				return err
			}
			if err := f.Close(); err != nil {
				return err
			}
		case tar.TypeSymlink:
			if err := os.MkdirAll(filepath.Dir(target), 0o750); err != nil {
				return err
			}
			if err := os.Remove(target); err != nil && !os.IsNotExist(err) {
				return err
			}
			if err := os.Symlink(hdr.Linkname, target); err != nil {
				return err
			}
		}
	}
	return nil
}

func ExtractParallel(items []ExtractItem) error {
	var wg sync.WaitGroup
	errs := make([]error, len(items))

	for i, item := range items {
		wg.Add(1)
		go func(idx int, it ExtractItem) {
			defer wg.Done()
			errs[idx] = Extract(it.Data, it.Opts)
		}(i, item)
	}
	wg.Wait()

	for _, err := range errs {
		if err != nil {
			return err
		}
	}
	return nil
}
