package upload

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// DirUploader implements Uploader by writing objects to a local directory
// tree rooted at Root. The bucket and content-type are ignored; the object
// key is used as the relative path under Root. Intended for integration
// testing without real R2 credentials.
type DirUploader struct {
	Root string
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
