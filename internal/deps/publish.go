package deps

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha512"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/jamestelfer/dollop/internal/upload"
)

const (
	// jsContentType is set explicitly on every uploaded module; mime type
	// detection for .mjs is unreliable across platforms.
	jsContentType = "text/javascript; charset=utf-8"
	// immutableCacheControl marks deps objects as permanently cacheable. Each
	// version lives at its own immutable path, so it never changes.
	immutableCacheControl = "public, max-age=31536000, immutable"

	// tarballDistPrefix is where the tarball places the dist tree.
	tarballDistPrefix = "package/dist/"
	// chunksDir is the ESM-min chunk directory, relative to the dist tree.
	chunksDir = "chunks/mermaid.esm.min/"
)

// FetchFunc downloads the tarball at url and returns its body. Inject
// http.DefaultClient.Do for production; tests supply a fixture.
type FetchFunc func(ctx context.Context, url string) (io.ReadCloser, error)

// TarballURL returns the npm registry tarball URL for a mermaid version.
func TarballURL(version string) string {
	return "https://registry.npmjs.org/mermaid/-/mermaid-" + version + ".tgz"
}

// Publish fetches the pinned mermaid tarball, verifies it against integrity,
// extracts the minimal ESM-min tree, and uploads every entry under
// deps/mermaid/<version>/ with an immutable cache header and explicit content
// types. It is idempotent: when the version is already present it uploads
// nothing and returns uploaded=false, unless force is set. A tarball whose
// sha512 does not match integrity aborts the publish with an error and uploads
// nothing.
func Publish(ctx context.Context, up upload.Uploader, lister upload.ObjectLister, bucket, version, integrity string, fetch FetchFunc, force bool, stderr io.Writer) (uploaded bool, err error) {
	if !force {
		present, perr := Present(ctx, lister, bucket, version)
		if perr != nil {
			return false, perr
		}
		if present {
			fmt.Fprintf(stderr, "mermaid %s already published, skipping\n", version) //nolint:errcheck
			return false, nil
		}
	}

	tgz, err := download(ctx, fetch, version)
	if err != nil {
		return false, err
	}
	if err := verify(tgz, integrity); err != nil {
		return false, err
	}

	entries, err := extractESMTree(tgz)
	if err != nil {
		return false, err
	}
	if len(entries) == 0 {
		return false, fmt.Errorf("mermaid tarball contained no ESM-min tree (expected %s%s)", tarballDistPrefix, entrypoint)
	}

	for _, e := range entries {
		key := VersionPrefix(version) + "/" + e.relPath
		fmt.Fprintf(stderr, "uploading [%s] %s...", e.relPath, upload.HumanSize(int64(len(e.content)))) //nolint:errcheck
		if err := up.PutObject(ctx, bucket, key, jsContentType, bytes.NewReader(e.content), upload.WithCacheControl(immutableCacheControl)); err != nil {
			fmt.Fprintln(stderr, "failed") //nolint:errcheck
			return false, fmt.Errorf("upload %s: %w", key, err)
		}
		fmt.Fprintln(stderr, "done") //nolint:errcheck
	}
	return true, nil
}

// download reads the whole tarball into memory so it can be hashed before any
// upload. The compressed tarball is well under the size that makes buffering a
// concern.
func download(ctx context.Context, fetch FetchFunc, version string) ([]byte, error) {
	rc, err := fetch(ctx, TarballURL(version))
	if err != nil {
		return nil, fmt.Errorf("fetch mermaid %s: %w", version, err)
	}
	defer rc.Close() //nolint:errcheck
	data, err := io.ReadAll(rc)
	if err != nil {
		return nil, fmt.Errorf("read mermaid %s tarball: %w", version, err)
	}
	return data, nil
}

// verify checks the tarball's sha512 against the npm integrity string
// (sha512-<base64>). It uses a constant-time comparison and returns an error on
// any mismatch or malformed integrity value.
func verify(tgz []byte, integrity string) error {
	b64, ok := strings.CutPrefix(integrity, "sha512-")
	if !ok {
		return fmt.Errorf("unsupported integrity %q: expected sha512- prefix", integrity)
	}
	want, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return fmt.Errorf("decode integrity: %w", err)
	}
	sum := sha512.Sum512(tgz)
	if subtle.ConstantTimeCompare(sum[:], want) != 1 {
		return errors.New("mermaid tarball sha512 does not match pinned integrity; aborting publish")
	}
	return nil
}

// esmEntry is one file from the minimal ESM-min tree, keyed by its path
// relative to the dist directory (e.g. mermaid.esm.min.mjs or
// chunks/mermaid.esm.min/chunk-A.mjs).
type esmEntry struct {
	relPath string
	content []byte
}

// extractESMTree reads the gzipped tarball and returns only the minimal ESM-min
// tree: the entrypoint and the .mjs chunks under chunks/mermaid.esm.min/. Source
// maps, unminified builds, other chunk variants, and unrelated package files are
// skipped.
func extractESMTree(tgz []byte) ([]esmEntry, error) {
	gr, err := gzip.NewReader(bytes.NewReader(tgz))
	if err != nil {
		return nil, fmt.Errorf("gunzip mermaid tarball: %w", err)
	}
	defer gr.Close() //nolint:errcheck

	tr := tar.NewReader(gr)
	var entries []esmEntry
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read mermaid tarball: %w", err)
		}
		if hdr.Typeflag != tar.TypeReg {
			continue
		}
		rel, ok := esmRelPath(hdr.Name)
		if !ok {
			continue
		}
		content, err := io.ReadAll(tr)
		if err != nil {
			return nil, fmt.Errorf("read %s from tarball: %w", hdr.Name, err)
		}
		entries = append(entries, esmEntry{relPath: rel, content: content})
	}
	return entries, nil
}

// esmRelPath reports whether a tar entry name belongs to the minimal ESM-min
// tree and, if so, returns its path relative to the dist directory. It matches
// the entrypoint mermaid.esm.min.mjs and .mjs chunks under
// chunks/mermaid.esm.min/, excluding source maps and every other file.
func esmRelPath(name string) (string, bool) {
	rel, ok := strings.CutPrefix(name, tarballDistPrefix)
	if !ok {
		return "", false
	}
	if !strings.HasSuffix(rel, ".mjs") {
		return "", false
	}
	if rel == entrypoint {
		return rel, true
	}
	if strings.HasPrefix(rel, chunksDir) && !strings.ContainsRune(strings.TrimPrefix(rel, chunksDir), '/') {
		return rel, true
	}
	return "", false
}
