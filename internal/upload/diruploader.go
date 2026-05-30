package upload

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// DirUploader implements Uploader by writing objects to a local directory
// tree rooted at Root. The bucket and content-type are ignored; the object
// key is used as the relative path under Root. Intended for integration
// testing without real R2 credentials.
type DirUploader struct {
	Root string
}

// ListObjects walks the prefix subtree under Root and returns the full keys of
// every file found, in lexical order. A missing prefix directory yields no
// keys (mirroring an empty prefix in R2). The bucket is ignored.
func (d *DirUploader) ListObjects(_ context.Context, _, prefix string) ([]string, error) {
	rel := strings.TrimRight(filepath.ToSlash(prefix), "/")
	dir := filepath.Join(d.Root, filepath.FromSlash(rel))
	info, err := os.Stat(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("stat %s: %w", dir, err)
	}
	if !info.IsDir() {
		return nil, nil
	}

	var keys []string
	err = filepath.WalkDir(dir, func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		relPath, err := filepath.Rel(d.Root, path)
		if err != nil {
			return err
		}
		keys = append(keys, filepath.ToSlash(relPath))
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk %s: %w", dir, err)
	}
	return keys, nil
}

// CopyObject is the DirUploader equivalent of an R2 self-copy touch: it advances
// the modification time of the file at key without altering its contents, so an
// integration round-trip can observe the lifecycle clock reset. A missing key is
// an error, mirroring R2 returning 404 for a copy of a nonexistent object.
func (d *DirUploader) CopyObject(_ context.Context, _, key string) error {
	dest := filepath.Join(d.Root, filepath.FromSlash(key))
	info, err := os.Stat(dest)
	if err != nil {
		return fmt.Errorf("stat %s: %w", dest, err)
	}
	if info.IsDir() {
		return fmt.Errorf("cannot touch directory %s", dest)
	}
	now := time.Now()
	if err := os.Chtimes(dest, now, now); err != nil {
		return fmt.Errorf("touch %s: %w", dest, err)
	}
	return nil
}

func (d *DirUploader) PutObject(_ context.Context, _, key, _ string, body io.Reader, _ ...PutOption) error {
	dest := filepath.Join(d.Root, filepath.FromSlash(key))
	if err := os.MkdirAll(filepath.Dir(dest), 0o750); err != nil {
		return fmt.Errorf("mkdir %s: %w", filepath.Dir(dest), err)
	}
	f, err := os.Create(dest) //nolint:gosec
	if err != nil {
		return fmt.Errorf("create %s: %w", dest, err)
	}
	defer f.Close() //nolint:errcheck
	if _, err := io.Copy(f, body); err != nil {
		return fmt.Errorf("copy to %s: %w", dest, err)
	}
	return nil
}
