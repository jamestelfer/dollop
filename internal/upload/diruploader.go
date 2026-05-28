package upload

import (
	"context"
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
		return err
	}
	f, err := os.Create(dest) //nolint:gosec
	if err != nil {
		return err
	}
	defer f.Close() //nolint:errcheck
	_, err = io.Copy(f, body)
	return err
}
