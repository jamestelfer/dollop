package upload

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/jamestelfer/dollop/internal/render"
)

// FileRenderer converts source files (e.g. .md → .html) within a directory.
// It is the same interface as render.Renderer and is satisfied by the types in
// that package.
type FileRenderer = render.Renderer

type uploadOptions struct {
	renderer  FileRenderer
	published publishedPrefix
}

// UploadOption configures optional behaviour for UploadFiles.
type UploadOption func(*uploadOptions)

// UploadResult reports what UploadFiles wrote.
type UploadResult struct {
	// SourceRelPaths holds the relative paths of the uploaded source files (not
	// generated entry pages, not shared assets).
	SourceRelPaths []string
	// Entry is the relative path the public URL should point at, or "" for the
	// prefix root (an index.html is published there).
	Entry string
	// WrittenKeys is the complete set of object keys written this run: every
	// source, every shared asset, and any generated index. The update touch pass
	// uses it to tell freshly-written objects from untouched pre-existing ones.
	WrittenKeys []string
}

// WithRenderer sets a FileRenderer that is invoked after path collection and
// before uploading. When no renderer is supplied, files are uploaded as-is.
func WithRenderer(r FileRenderer) UploadOption {
	return func(o *uploadOptions) {
		o.renderer = r
	}
}

// WithPublished reports the relative paths already published under the
// prefix. Entry pages are planned over them as well as the local files: a
// README there gets a redirect, a listing includes them, and an index page
// there is replaced only with IndexOverwrite. update supplies it; a fresh
// prefix has nothing published.
func WithPublished(relPaths []string) UploadOption {
	return func(o *uploadOptions) {
		o.published.paths = relPaths
	}
}

// WithPublishedIndex reports what generated the index page already published
// under the prefix (see InspectPublishedIndex). A dollop-generated index page
// is treated as absent and regenerated over the overlay; a generated listing
// is refreshed even without an index mode, keeping its title.
func WithPublishedIndex(index PublishedIndex) UploadOption {
	return func(o *uploadOptions) {
		o.published.index = index
	}
}

// WithPublishedManifest supplies the manifest already published under the
// prefix (see ReadPublishedManifest). Its titles are used for the published
// files that are not uploaded again, and carried into the new manifest.
func WithPublishedManifest(m Manifest) UploadOption {
	return func(o *uploadOptions) {
		o.published.manifest = m
	}
}

// UploadFiles uploads localPath (file or directory) to bucket under prefix.
// Each completed key is written to stderr. Files are uploaded sequentially.
// After the sources, an entry page may be generated (see planEntry): none when
// the upload has its own index page or one is already published (see
// WithPublished) and mode is not IndexOverwrite, an index.html redirecting to
// a root README, or, when mode asks for one, an index.md listing the local and
// published files, planned through the renderer. A skipped --index is warned
// about on stderr.
// A directory is read through an os.Root, so symlinks resolving outside it are
// skipped with a warning; a single file is read wherever its symlink points.
// Returns an UploadResult reporting the relative paths of the uploaded sources
// (not including any generated index.html), the entry the public URL should
// point at, and the complete set of keys written.
func UploadFiles(ctx context.Context, up Uploader, bucket, prefix, localPath string, mode IndexMode, stderr io.Writer, opts ...UploadOption) (*UploadResult, error) {
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

	var fsys fs.FS
	var relPaths []string
	if info.IsDir() {
		root, err := os.OpenRoot(localPath)
		if err != nil {
			return nil, fmt.Errorf("open %s: %w", localPath, err)
		}
		// Sources open lazily during uploadPlan, so the root must outlive it.
		defer root.Close() //nolint:errcheck
		fsys = root.FS()
		relPaths, err = render.ListFiles(fsys, stderr)
		if err != nil {
			return nil, fmt.Errorf("scan files: %w", err)
		}
	} else {
		fsys = os.DirFS(filepath.Dir(localPath))
		relPaths = []string{filepath.Base(localPath)}
	}

	sources, sharedAssets, err := renderer.Plan(relPaths, fsys, prefix)
	if err != nil {
		return nil, fmt.Errorf("plan: %w", err)
	}

	// dollop writes the manifest itself, after everything else
	sources = slices.DeleteFunc(sources, func(src render.Source) bool { return isManifest(src.RelPath) })

	heading := indexHeading{Title: o.published.listingTitle(uploadTitle(localPath)), Lifetime: lifetimeOf(prefix, time.Now())}
	titles := o.published.titles()
	for _, p := range relPathsOf(sources) {
		delete(titles, p)
	}
	maps.Copy(titles, localTitles(fsys, sources))
	plan, err := planEntry(renderer, fsys, prefix, sources, sharedAssets, o.published, mode, heading, titles, stderr)
	if err != nil {
		return nil, err
	}

	assets := mergeAssets(sharedAssets, plan.assets)
	sources = withoutAssets(sources, assets)
	writtenKeys, err := uploadPlan(ctx, up, bucket, prefix, append(slices.Clip(sources), plan.generated...), assets, stderr)
	if err != nil {
		return nil, err
	}

	generated := relPathsOf(plan.generated)
	manifestPaths := slices.DeleteFunc(overlayPaths(relPathsOf(sources), o.published.userPaths()), func(p string) bool {
		return slices.Contains(generated, p)
	})
	manifest := newManifest(heading.Title, plan.index, manifestPaths, titles)
	manifestKey, err := uploadManifest(ctx, up, bucket, prefix, manifest, stderr)
	if err != nil {
		return nil, err
	}
	writtenKeys = append(writtenKeys, manifestKey)

	return &UploadResult{SourceRelPaths: relPathsOf(sources), Entry: plan.entry, WrittenKeys: writtenKeys}, nil
}

// uploadTitle names the upload at localPath for its index page: the base name
// of the directory or file, resolved first so "." names the directory itself.
func uploadTitle(localPath string) string {
	if abs, err := filepath.Abs(localPath); err == nil {
		localPath = abs
	}
	return filepath.Base(localPath)
}

// mergeAssets returns a followed by the assets of b not already in a, by name.
func mergeAssets(a, b []render.SharedAsset) []render.SharedAsset {
	seen := make(map[string]bool, len(a))
	for _, asset := range a {
		seen[asset.Name] = true
	}
	merged := slices.Clip(a)
	for _, asset := range b {
		if !seen[asset.Name] {
			seen[asset.Name] = true
			merged = append(merged, asset)
		}
	}
	return merged
}

// withoutAssets drops sources at an asset's path: dollop owns AssetDir, so the generated asset wins.
func withoutAssets(sources []render.Source, assets []render.SharedAsset) []render.Source {
	if len(assets) == 0 {
		return sources
	}
	assetPaths := make(map[string]bool, len(assets))
	for _, asset := range assets {
		assetPaths[asset.RelPath()] = true
	}
	return slices.DeleteFunc(slices.Clone(sources), func(src render.Source) bool {
		return assetPaths[src.RelPath]
	})
}

// uploadPlan uploads the shared assets and then the sources, returning the
// full set of object keys written (in that order).
func uploadPlan(ctx context.Context, up Uploader, bucket, prefix string, sources []render.Source, sharedAssets []render.SharedAsset, stderr io.Writer) ([]string, error) {
	var writtenKeys []string

	if err := uploadSharedAssets(ctx, up, bucket, prefix, sharedAssets, stderr); err != nil {
		return nil, err
	}
	for _, asset := range sharedAssets {
		writtenKeys = append(writtenKeys, prefix+"/"+asset.RelPath())
	}

	for _, src := range sources {
		if err := uploadSource(ctx, up, bucket, prefix, src, stderr); err != nil {
			return nil, err
		}
		writtenKeys = append(writtenKeys, prefix+"/"+src.RelPath)
	}

	return writtenKeys, nil
}

func uploadSharedAssets(ctx context.Context, up Uploader, bucket, prefix string, assets []render.SharedAsset, stderr io.Writer) error {
	for _, asset := range assets {
		key := prefix + "/" + asset.RelPath()
		fmt.Fprintf(stderr, "uploading [%s] %s...", asset.RelPath(), HumanSize(int64(len(asset.Content)))) //nolint:errcheck
		if err := up.PutObject(ctx, bucket, key, asset.ContentType, bytes.NewReader(asset.Content), WithCacheControl("max-age=604800")); err != nil {
			fmt.Fprintln(stderr, "failed") //nolint:errcheck
			return fmt.Errorf("upload shared asset %s: %w", asset.RelPath(), err)
		}
		fmt.Fprintln(stderr, "done") //nolint:errcheck
	}
	return nil
}

func uploadSource(ctx context.Context, up Uploader, bucket, prefix string, src render.Source, stderr io.Writer) error {
	ct := src.ContentType
	if ct == "" {
		ct = ContentType(src.RelPath)
	}
	key := prefix + "/" + src.RelPath
	name := src.RelPath

	known := src.Size >= 0
	if known {
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

	// For rendered sources the size is unknown until the body is produced, so
	// report it after the ellipsis. body is seekable (the S3 SDK relies on it),
	// so seeking to the end yields the number of bytes uploaded.
	if !known {
		if size, serr := body.Seek(0, io.SeekEnd); serr == nil {
			fmt.Fprintln(stderr, HumanSize(size)) //nolint:errcheck
			return nil
		}
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
