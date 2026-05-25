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
	info, err := os.Stat(localPath)
	if err != nil {
		return err
	}
	fmt.Fprintf(stderr, "uploading [%s] %s...", filepath.Base(localPath), HumanSize(info.Size())) //nolint:errcheck

	f, err := os.Open(localPath) //nolint:gosec
	if err != nil {
		fmt.Fprintln(stderr, "failed") //nolint:errcheck
		return err
	}
	defer f.Close() //nolint:errcheck

	ct := ContentType(localPath)
	if err := up.PutObject(ctx, bucket, key, ct, f); err != nil {
		fmt.Fprintln(stderr, "failed") //nolint:errcheck
		return fmt.Errorf("upload %s: %w", key, err)
	}
	fmt.Fprintln(stderr, "done") //nolint:errcheck
	return nil
}

func HumanSize(n int64) string {
	const (
		kb = 1024
		mb = 1024 * kb
		gb = 1024 * mb
	)
	switch {
	case n >= gb:
		return fmt.Sprintf("%.1f GB", float64(n)/gb)
	case n >= mb:
		return fmt.Sprintf("%.1f MB", float64(n)/mb)
	case n >= kb:
		return fmt.Sprintf("%.1f KB", float64(n)/kb)
	default:
		return fmt.Sprintf("%d B", n)
	}
}
