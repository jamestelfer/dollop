package upload

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

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

// UploadFiles uploads localPath (file or directory) to bucket under prefix.
// Each completed key is written to stderr. Files are uploaded sequentially.
// If generateIndex is true and no index.html is present in the upload, a
// generated index page is uploaded first; if index.html already exists a
// warning is written to stderr and generation is skipped.
// Returns the relative paths of all files uploaded (not including any
// generated index.html).
func UploadFiles(ctx context.Context, up Uploader, bucket, prefix, localPath string, generateIndex bool, stderr io.Writer, opts ...UploadOption) ([]string, error) {
	if up == nil {
		return nil, fmt.Errorf("uploader is not configured")
	}

	var o uploadOptions
	for _, opt := range opts {
		opt(&o)
	}

	renderer := o.renderer
	if renderer == nil {
		renderer = render.NewDiskRenderer()
	}

	info, err := os.Stat(localPath)
	if err != nil {
		return nil, fmt.Errorf("stat %s: %w", localPath, err)
	}

	sourceDir := localPath
	if !info.IsDir() {
		sourceDir = filepath.Dir(localPath)
	}

	relPaths, err := collectRelativePaths(localPath, info.IsDir())
	if err != nil {
		return nil, fmt.Errorf("scan files: %w", err)
	}

	sources, sharedAssets, err := renderer.Plan(relPaths, sourceDir)
	if err != nil {
		return nil, fmt.Errorf("plan: %w", err)
	}

	if err := uploadSharedAssets(ctx, up, bucket, prefix, sharedAssets, stderr); err != nil {
		return nil, err
	}

	if generateIndex {
		if err := maybeUploadGeneratedIndex(ctx, up, bucket, prefix, sources, stderr); err != nil {
			return nil, err
		}
	}

	for _, src := range sources {
		if err := uploadSource(ctx, up, bucket, prefix, src, stderr); err != nil {
			return nil, err
		}
	}

	result := make([]string, len(sources))
	for i, src := range sources {
		result[i] = src.RelPath
	}
	return result, nil
}

// maybeUploadGeneratedIndex uploads a generated index.html for sources unless
// one is already present, in which case it warns and skips generation.
func maybeUploadGeneratedIndex(ctx context.Context, up Uploader, bucket, prefix string, sources []render.Source, stderr io.Writer) error {
	if containsIndex(sources) {
		fmt.Fprintln(stderr, "warning: index.html already present, skipping index generation") //nolint:errcheck
		return nil
	}
	srcRelPaths := make([]string, len(sources))
	for i, src := range sources {
		srcRelPaths[i] = src.RelPath
	}
	return uploadGeneratedIndex(ctx, up, bucket, prefix, srcRelPaths, stderr)
}

func containsIndex(sources []render.Source) bool {
	for _, src := range sources {
		if src.RelPath == "index.html" {
			return true
		}
	}
	return false
}

func uploadSharedAssets(ctx context.Context, up Uploader, bucket, prefix string, assets []render.SharedAsset, stderr io.Writer) error {
	for _, asset := range assets {
		key := prefix + "/" + asset.Name
		fmt.Fprintf(stderr, "uploading [%s] %s...", asset.Name, HumanSize(int64(len(asset.Content)))) //nolint:errcheck
		if err := up.PutObject(ctx, bucket, key, asset.ContentType, bytes.NewReader(asset.Content), WithCacheControl("max-age=604800")); err != nil {
			fmt.Fprintln(stderr, "failed") //nolint:errcheck
			return fmt.Errorf("upload shared asset %s: %w", asset.Name, err)
		}
		fmt.Fprintln(stderr, "done") //nolint:errcheck
	}
	return nil
}

func collectRelativePaths(localPath string, isDir bool) ([]string, error) {
	if !isDir {
		return []string{filepath.Base(localPath)}, nil
	}
	var paths []string
	err := filepath.WalkDir(localPath, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, err := filepath.Rel(localPath, path)
		if err != nil {
			return err
		}
		paths = append(paths, filepath.ToSlash(rel))
		return nil
	})
	return paths, err
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

func uploadSource(ctx context.Context, up Uploader, bucket, prefix string, src render.Source, stderr io.Writer) error {
	ct := src.ContentType
	if ct == "" {
		ct = ContentType(src.RelPath)
	}
	key := prefix + "/" + src.RelPath
	name := filepath.Base(src.RelPath)

	if src.Size >= 0 {
		fmt.Fprintf(stderr, "uploading [%s] %s...", name, HumanSize(src.Size)) //nolint:errcheck
	} else {
		fmt.Fprintf(stderr, "rendering [%s]...", name) //nolint:errcheck
	}

	body, err := src.Open()
	if err != nil {
		fmt.Fprintln(stderr, "failed") //nolint:errcheck
		return fmt.Errorf("open %s: %w", src.RelPath, err)
	}
	defer body.Close() //nolint:errcheck

	if err := up.PutObject(ctx, bucket, key, ct, body); err != nil {
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
