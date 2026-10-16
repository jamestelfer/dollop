package upload

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// download is one object to fetch: its bucket key and its slash-separated path
// relative to the download target. body and size are set when the object was
// already opened while resolving the reference (the single-file case).
type download struct {
	key  string
	rel  string
	body io.ReadCloser
	size int64
}

// Download fetches what ref names into targetDir: a single file when ref.Path is
// an exact object key (written as <targetDir>/<basename>), otherwise every
// object under ref.Path, laid out relative to it. Objects are written exactly
// as stored.
//
// targetDir is created when missing. Every target is checked before anything
// is downloaded: an existing file is a conflict unless force is set, and a
// file/directory mismatch is always an error. All writes go through an
// os.Root, so no object can be written outside targetDir.
//
// Progress is written to stderr. The returned path is the written file for a
// single-file download, or targetDir otherwise.
func Download(ctx context.Context, r ObjectReader, bucket string, ref GetRef, targetDir string, force bool, stderr io.Writer) (string, error) {
	items, err := planDownload(ctx, r, bucket, ref)
	if err != nil {
		return "", err
	}
	defer func() {
		for _, it := range items {
			if it.body != nil {
				_ = it.body.Close()
			}
		}
	}()

	if err := checkPlanConflicts(items); err != nil {
		return "", err
	}

	root, err := openTarget(targetDir)
	if err != nil {
		return "", err
	}
	defer root.Close() //nolint:errcheck

	if err := preflight(root, items, force); err != nil {
		return "", err
	}

	for i := range items {
		if err := fetchOne(ctx, r, bucket, root, &items[i], force, stderr); err != nil {
			return "", err
		}
	}

	if !ref.Whole() && len(items) == 1 && items[0].key == ref.Path {
		return filepath.Join(targetDir, filepath.FromSlash(items[0].rel)), nil
	}
	return targetDir, nil
}

// planDownload resolves ref to the objects to fetch. A sub-path is first tried
// as an exact key; failing that it is listed as a directory. The lister scopes
// the listing to "<path>/", so only full directory names match.
func planDownload(ctx context.Context, r ObjectReader, bucket string, ref GetRef) ([]download, error) {
	if !ref.Whole() {
		body, size, err := r.GetObject(ctx, bucket, ref.Path)
		switch {
		case err == nil:
			return []download{{key: ref.Path, rel: path.Base(ref.Path), body: body, size: size}}, nil
		case !errors.Is(err, ErrObjectNotFound):
			return nil, err
		}
	}

	keys, err := r.ListObjects(ctx, bucket, ref.Path)
	if err != nil {
		return nil, err
	}
	if len(keys) == 0 {
		return nil, fmt.Errorf("nothing found at %s", ref.Path)
	}

	base := ref.Path + "/"
	items := make([]download, 0, len(keys))
	for _, key := range keys {
		rel, ok := strings.CutPrefix(key, base)
		if !ok {
			return nil, fmt.Errorf("listed key %q is outside %s", key, base)
		}
		if err := checkSegments(strings.Split(rel, "/")); err != nil {
			return nil, fmt.Errorf("unsafe key %q: %w", key, err)
		}
		if !filepath.IsLocal(filepath.FromSlash(rel)) {
			return nil, fmt.Errorf("unsafe key %q: not a local path", key)
		}
		items = append(items, download{key: key, rel: rel})
	}
	return items, nil
}

// checkPlanConflicts rejects a plan where one object's path is a directory of
// another (e.g. both "a" and "a/b"), which cannot both be written.
func checkPlanConflicts(items []download) error {
	files := make(map[string]struct{}, len(items))
	for _, it := range items {
		files[it.rel] = struct{}{}
	}
	for _, it := range items {
		for dir := path.Dir(it.rel); dir != "."; dir = path.Dir(dir) {
			if _, ok := files[dir]; ok {
				return fmt.Errorf("object %q conflicts with directory needed for %q", dir, it.rel)
			}
		}
	}
	return nil
}

// openTarget creates targetDir when missing and opens it as a root that confines
// every write beneath it.
func openTarget(targetDir string) (*os.Root, error) {
	info, err := os.Stat(targetDir)
	switch {
	case err == nil && !info.IsDir():
		return nil, fmt.Errorf("target %s exists and is not a directory", targetDir)
	case errors.Is(err, fs.ErrNotExist):
		if err := os.MkdirAll(targetDir, 0o750); err != nil {
			return nil, fmt.Errorf("create target %s: %w", targetDir, err)
		}
	case err != nil:
		return nil, fmt.Errorf("stat target %s: %w", targetDir, err)
	}
	root, err := os.OpenRoot(targetDir)
	if err != nil {
		return nil, fmt.Errorf("open target %s: %w", targetDir, err)
	}
	return root, nil
}

// preflight checks every target path before any download starts. Parent
// directories must be absent or real directories, and a target must be absent
// or (with force) a regular file. Existing files without force are collected
// and reported together.
func preflight(root *os.Root, items []download, force bool) error {
	var conflicts []string
	for _, it := range items {
		for dir := path.Dir(it.rel); dir != "."; dir = path.Dir(dir) {
			info, err := root.Lstat(filepath.FromSlash(dir))
			if err == nil && !info.IsDir() {
				return fmt.Errorf("cannot write %s: %s exists and is not a directory", it.rel, dir)
			}
			if err != nil && !errors.Is(err, fs.ErrNotExist) {
				return fmt.Errorf("check %s: %w", dir, err)
			}
		}

		info, err := root.Lstat(filepath.FromSlash(it.rel))
		switch {
		case errors.Is(err, fs.ErrNotExist):
			continue
		case err != nil:
			return fmt.Errorf("check %s: %w", it.rel, err)
		case !info.Mode().IsRegular():
			return fmt.Errorf("cannot write %s: it exists and is not a regular file", it.rel)
		case !force:
			conflicts = append(conflicts, it.rel)
		}
	}
	if len(conflicts) > 0 {
		return fmt.Errorf("%d file(s) already exist (use --force to overwrite): %s",
			len(conflicts), strings.Join(conflicts, ", "))
	}
	return nil
}

// fetchOne downloads a single object into root, removing any partial file on
// failure. Without force the file is created exclusively, so a file appearing
// after preflight is never overwritten.
func fetchOne(ctx context.Context, r ObjectGetter, bucket string, root *os.Root, it *download, force bool, stderr io.Writer) error {
	if it.body == nil {
		body, size, err := r.GetObject(ctx, bucket, it.key)
		if err != nil {
			return fmt.Errorf("download %s: %w", it.key, err)
		}
		it.body, it.size = body, size
	}
	defer func() {
		_ = it.body.Close()
		it.body = nil
	}()

	fmt.Fprintf(stderr, "downloading [%s] %s...", it.rel, HumanSize(it.size)) //nolint:errcheck

	local := filepath.FromSlash(it.rel)
	if dir := filepath.Dir(local); dir != "." {
		if err := root.MkdirAll(dir, 0o750); err != nil {
			fmt.Fprintln(stderr, "failed") //nolint:errcheck
			return fmt.Errorf("create directory %s: %w", dir, err)
		}
	}

	flags := os.O_WRONLY | os.O_CREATE | os.O_TRUNC
	if !force {
		flags |= os.O_EXCL
	}
	f, err := root.OpenFile(local, flags, 0o644) //nolint:gosec // downloaded files are ordinary user files
	if err != nil {
		fmt.Fprintln(stderr, "failed") //nolint:errcheck
		return fmt.Errorf("create %s: %w", it.rel, err)
	}

	_, copyErr := io.Copy(f, it.body)
	closeErr := f.Close()
	if err := errors.Join(copyErr, closeErr); err != nil {
		_ = root.Remove(local)
		fmt.Fprintln(stderr, "failed") //nolint:errcheck
		return fmt.Errorf("download %s: %w", it.key, err)
	}

	fmt.Fprintln(stderr, "done") //nolint:errcheck
	return nil
}
