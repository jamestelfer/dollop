package upload

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"

	"github.com/jamestelfer/dollop/internal/render"
)

// FileRenderer converts source files (e.g. .md → .html) within a directory.
// It is the same interface as render.Renderer and is satisfied by the types in
// that package.
type FileRenderer = render.Renderer

type uploadOptions struct {
	renderer FileRenderer
}

// UploadOption configures optional behaviour for UploadFiles.
type UploadOption func(*uploadOptions)

// WithRenderer sets a FileRenderer that is invoked after path collection and
// before uploading. When no renderer is supplied, files are uploaded as-is.
func WithRenderer(r FileRenderer) UploadOption {
	return func(o *uploadOptions) {
		o.renderer = r
	}
}

// NewMarkdownFileRenderer returns a FileRenderer that converts .md files to .html.
func NewMarkdownFileRenderer() FileRenderer {
	return render.NewMarkdownRenderer()
}

// UploadFiles uploads localPath (file or directory) to bucket under prefix.
// Each completed key is written to stderr. Files are uploaded sequentially.
// If generateIndex is true and no index.html is present in the upload, a
// generated index page is uploaded first; if index.html already exists a
// warning is written to stderr and generation is skipped.
// Returns the relative paths of all on-disk files uploaded (not including any
// generated index.html).
func UploadFiles(ctx context.Context, up Uploader, bucket, prefix, localPath string, generateIndex bool, stderr io.Writer, opts ...UploadOption) ([]string, error) {
	var o uploadOptions
	for _, opt := range opts {
		opt(&o)
	}

	info, err := os.Stat(localPath)
	if err != nil {
		return nil, err
	}

	sourceDir := localPath
	if !info.IsDir() {
		sourceDir = filepath.Dir(localPath)
	}

	relPaths, hasIndex, err := collectRelativePaths(localPath, info.IsDir())
	if err != nil {
		return nil, fmt.Errorf("scan files: %w", err)
	}

	if o.renderer != nil {
		relPaths, err = o.renderer.Render(relPaths, sourceDir)
		if err != nil {
			return nil, fmt.Errorf("render: %w", err)
		}
		// after rendering, index.md may have produced index.html
		if slices.Contains(relPaths, "index.html") {
			hasIndex = true
		}
		for _, asset := range o.renderer.SharedAssets() {
			key := prefix + "/" + asset.Name
			fmt.Fprintf(stderr, "uploading [%s] %s...", asset.Name, HumanSize(int64(len(asset.Content)))) //nolint:errcheck
			if err := up.PutObject(ctx, bucket, key, asset.ContentType, bytes.NewReader(asset.Content), WithCacheControl("max-age=604800")); err != nil {
				fmt.Fprintln(stderr, "failed") //nolint:errcheck
				return nil, fmt.Errorf("upload shared asset %s: %w", asset.Name, err)
			}
			fmt.Fprintln(stderr, "done") //nolint:errcheck
		}
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

	for _, p := range relPaths {
		key := prefix + "/" + p
		absPath := filepath.Join(sourceDir, filepath.FromSlash(p))
		if err := uploadFile(ctx, up, bucket, key, absPath, stderr); err != nil {
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
