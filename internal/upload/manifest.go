package upload

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"

	"github.com/jamestelfer/dollop/internal/render"
)

// manifestPath is where the manifest is published, relative to the prefix.
const manifestPath = render.AssetDir + "/manifest.json"

const manifestVersion = 1

// manifestReadLimit bounds the read of a published manifest; a larger one is
// ignored rather than misread.
const manifestReadLimit = 4 << 20

// Manifest describes what an upload published under its prefix: the listing's
// title, the generated index page and the title of each page. Files is keyed
// by relative path and leaves out dollop's own files.
type Manifest struct {
	Version int                     `json:"version"`
	Title   string                  `json:"title,omitempty"`
	Index   IndexKind               `json:"index,omitempty"`
	Files   map[string]ManifestFile `json:"files"`
}

// ManifestFile is a published file in a Manifest.
type ManifestFile struct {
	Title string `json:"title,omitempty"`
}

// newManifest returns the manifest for relPaths, titled as given, skipping
// dollop's own files.
func newManifest(title string, index IndexKind, relPaths []string, titles map[string]string) Manifest {
	m := Manifest{Version: manifestVersion, Title: title, Index: index, Files: make(map[string]ManifestFile)}
	for _, p := range relPaths {
		if !render.IsAssetName(p) {
			m.Files[p] = ManifestFile{Title: titles[p]}
		}
	}
	return m
}

// uploadManifest publishes m uncached, since every run rewrites it.
func uploadManifest(ctx context.Context, up Uploader, bucket, prefix string, m Manifest, stderr io.Writer) (string, error) {
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return "", fmt.Errorf("encode manifest: %w", err)
	}
	key := prefix + "/" + manifestPath
	fmt.Fprintf(stderr, "uploading [%s] %s...", manifestPath, HumanSize(int64(len(data)))) //nolint:errcheck
	if err := up.PutObject(ctx, bucket, key, "application/json", bytes.NewReader(data), WithCacheControl("no-cache")); err != nil {
		fmt.Fprintln(stderr, "failed") //nolint:errcheck
		return "", fmt.Errorf("upload manifest: %w", err)
	}
	fmt.Fprintln(stderr, "done") //nolint:errcheck
	return key, nil
}

// ReadPublishedManifest returns the manifest published under prefix when
// published (the relative paths there) lists one. A missing manifest is the
// zero Manifest; so is one that cannot be read, is not valid JSON, is too large
// or has an unknown version, with a warning on stderr.
func ReadPublishedManifest(ctx context.Context, r ObjectHeadReader, bucket, prefix string, published []string, stderr io.Writer) Manifest {
	if !slices.Contains(published, manifestPath) {
		return Manifest{}
	}
	data, err := r.ReadObjectHead(ctx, bucket, prefix+"/"+manifestPath, manifestReadLimit)
	if errors.Is(err, ErrObjectNotFound) {
		return Manifest{}
	}
	if err == nil && len(data) >= manifestReadLimit {
		err = fmt.Errorf("larger than %s", HumanSize(manifestReadLimit))
	}
	var m Manifest
	if err == nil {
		err = json.Unmarshal(data, &m)
	}
	if err == nil && m.Version != manifestVersion {
		err = fmt.Errorf("unknown version %d", m.Version)
	}
	if err != nil {
		fmt.Fprintf(stderr, "warning: ignoring published manifest: %v\n", err) //nolint:errcheck
		return Manifest{}
	}
	return m
}

func isManifest(relPath string) bool {
	return relPath == manifestPath
}
