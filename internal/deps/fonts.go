package deps

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"

	"github.com/jamestelfer/dollop/internal/upload"
)

const (
	// fontContentType is set explicitly; mime type detection for .otf is
	// unreliable across platforms.
	fontContentType = "font/otf"
	// fontCacheControl is long but not immutable: the font path carries no
	// version, so a replaced file must eventually be re-fetched.
	fontCacheControl = "public, max-age=604800"
)

// FontSet identifies the optional shared body font: the bucket key prefix it is
// published under and the file names rendered pages reference. The files are
// proprietary and supplied locally; nothing here fetches or embeds them.
type FontSet struct {
	Prefix string
	Files  []string
}

// FontsPresent returns the font set's file names that are published in the
// bucket, in the set's order.
func FontsPresent(ctx context.Context, lister upload.ObjectLister, bucket string, fonts FontSet) ([]string, error) {
	keys, err := lister.ListObjects(ctx, bucket, fonts.Prefix)
	if err != nil {
		return nil, fmt.Errorf("list fonts: %w", err)
	}
	var present []string
	for _, f := range fonts.Files {
		if slices.Contains(keys, fonts.Prefix+"/"+f) {
			present = append(present, f)
		}
	}
	return present, nil
}

// PublishFonts uploads the font set's files found anywhere under dir to the
// font prefix and returns the uploaded file names in the set's order. Files are
// matched by base name, so the directory an archive unpacks to can be passed
// as-is; every other file is ignored. When a name appears more than once the
// first in walk order is used and the rest are reported. A missing file produces a warning (pages
// fall back for the weights it serves); finding none at all is an error.
func PublishFonts(ctx context.Context, up upload.Uploader, bucket string, fonts FontSet, dir string, stderr io.Writer) ([]string, error) {
	// WalkDir does not descend into a symlinked root, so resolve it first.
	root, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return nil, fmt.Errorf("scan %s: %w", dir, err)
	}

	found := map[string]string{}
	err = filepath.WalkDir(root, func(p string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !slices.Contains(fonts.Files, entry.Name()) {
			return nil
		}
		if first, dup := found[entry.Name()]; dup {
			fmt.Fprintf(stderr, "warning: ignoring %s; using %s\n", p, first) //nolint:errcheck
			return nil
		}
		found[entry.Name()] = p
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("scan %s: %w", dir, err)
	}
	if len(found) == 0 {
		return nil, fmt.Errorf("no font files found under %s (expected %v)", dir, fonts.Files)
	}

	var uploaded []string
	for _, name := range fonts.Files {
		p, ok := found[name]
		if !ok {
			fmt.Fprintf(stderr, "warning: %s not found under %s\n", name, dir) //nolint:errcheck
			continue
		}
		if err := putFont(ctx, up, bucket, fonts.Prefix+"/"+name, p, stderr); err != nil {
			return uploaded, err
		}
		uploaded = append(uploaded, name)
	}
	return uploaded, nil
}

func putFont(ctx context.Context, up upload.Uploader, bucket, key, path string, stderr io.Writer) error {
	f, err := os.Open(path) //nolint:gosec
	if err != nil {
		return fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close() //nolint:errcheck

	info, err := f.Stat()
	if err != nil {
		return fmt.Errorf("stat %s: %w", path, err)
	}
	return putDep(ctx, up, bucket, depObject{
		key:          key,
		name:         filepath.Base(path),
		contentType:  fontContentType,
		cacheControl: fontCacheControl,
		size:         info.Size(),
		body:         f,
	}, stderr)
}
