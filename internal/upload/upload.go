package upload

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// UploadFiles uploads localPath (file or directory) to bucket under prefix.
// Each completed key is written to stderr. Files are uploaded sequentially.
func UploadFiles(ctx context.Context, up Uploader, bucket, prefix, localPath string, stderr io.Writer) error {
	info, err := os.Stat(localPath)
	if err != nil {
		return err
	}
	if info.IsDir() {
		return uploadDir(ctx, up, bucket, prefix, localPath, stderr)
	}
	return uploadFile(ctx, up, bucket, prefix+"/"+filepath.Base(localPath), localPath, stderr)
}

func uploadDir(ctx context.Context, up Uploader, bucket, prefix, dir string, stderr io.Writer) error {
	return filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		key := prefix + "/" + filepath.ToSlash(rel)
		return uploadFile(ctx, up, bucket, key, path, stderr)
	})
}

func uploadFile(ctx context.Context, up Uploader, bucket, key, localPath string, stderr io.Writer) error {
	f, err := os.Open(localPath)
	if err != nil {
		return err
	}
	defer f.Close()

	ct := ContentType(localPath)
	if err := up.PutObject(ctx, bucket, key, ct, f); err != nil {
		return fmt.Errorf("upload %s: %w", key, err)
	}
	fmt.Fprintln(stderr, key)
	return nil
}
