package upload

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// UploadFiles uploads localPath (file or directory) to bucket under prefix.
// Each completed key is written to stderr. Files are uploaded sequentially.
// If generateIndex is true and no index.html is present in the upload, a
// generated index page is uploaded first; if index.html already exists a
// warning is written to stderr and generation is skipped.
// Returns the relative paths of all on-disk files uploaded (not including any
// generated index.html).
func UploadFiles(ctx context.Context, up Uploader, bucket, prefix, localPath string, generateIndex bool, stderr io.Writer) ([]string, error) {
	info, err := os.Stat(localPath)
	if err != nil {
		return nil, err
	}

	relPaths, hasIndex, err := collectRelativePaths(localPath, info.IsDir())
	if err != nil {
		return nil, fmt.Errorf("scan files: %w", err)
	}

	if generateIndex {
		if hasIndex {
			fmt.Fprintln(stderr, "warning: index.html already present, skipping index generation") //nolint:errcheck
		} else {
			if err := uploadGeneratedIndex(ctx, up, bucket, prefix, relPaths, stderr); err != nil {
				return nil, err
			}
		}
	}

	if info.IsDir() {
		if err := uploadDir(ctx, up, bucket, prefix, localPath, stderr); err != nil {
			return nil, err
		}
	} else {
		if err := uploadFile(ctx, up, bucket, prefix+"/"+filepath.Base(localPath), localPath, stderr); err != nil {
			return nil, err
		}
	}
	return relPaths, nil
}

func collectRelativePaths(localPath string, isDir bool) ([]string, bool, error) {
	if !isDir {
		name := filepath.Base(localPath)
		return []string{name}, name == "index.html", nil
	}
	var paths []string
	hasIndex := false
	err := filepath.WalkDir(localPath, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, err := filepath.Rel(localPath, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		paths = append(paths, rel)
		if rel == "index.html" {
			hasIndex = true
		}
		return nil
	})
	return paths, hasIndex, err
}

func uploadGeneratedIndex(ctx context.Context, up Uploader, bucket, prefix string, files []string, stderr io.Writer) error {
	html, err := generateIndexHTML(files)
	if err != nil {
		return fmt.Errorf("generate index: %w", err)
	}
	key := prefix + "/index.html"
	fmt.Fprintf(stderr, "uploading [index.html] %s...", HumanSize(int64(len(html)))) //nolint:errcheck
	if err := up.PutObject(ctx, bucket, key, "text/html; charset=utf-8", bytes.NewReader(html)); err != nil {
		fmt.Fprintln(stderr, "failed") //nolint:errcheck
		return fmt.Errorf("upload %s: %w", key, err)
	}
	fmt.Fprintln(stderr, "done") //nolint:errcheck
	return nil
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
