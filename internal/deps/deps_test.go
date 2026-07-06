package deps_test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha512"
	"encoding/base64"
	"errors"
	"io"
	"sort"
	"testing"

	"github.com/jamestelfer/dollop/internal/deps"
	"github.com/jamestelfer/dollop/internal/upload"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// tarEntry is a single file to place in a fixture tarball.
type tarEntry struct {
	name    string
	content string
}

// buildTarball returns a gzipped tar containing entries and the npm-style
// integrity string (sha512-<base64>) computed over the gzipped bytes.
func buildTarball(t *testing.T, entries []tarEntry) (tgz []byte, integrity string) {
	t.Helper()
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)
	for _, e := range entries {
		hdr := &tar.Header{Name: e.name, Mode: 0o644, Size: int64(len(e.content)), Typeflag: tar.TypeReg}
		require.NoError(t, tw.WriteHeader(hdr))
		_, err := tw.Write([]byte(e.content))
		require.NoError(t, err)
	}
	require.NoError(t, tw.Close())
	require.NoError(t, gw.Close())

	tgz = buf.Bytes()
	sum := sha512.Sum512(tgz)
	integrity = "sha512-" + base64.StdEncoding.EncodeToString(sum[:])
	return tgz, integrity
}

// mermaidFixture returns a tarball that mimics the mermaid npm package layout:
// the ESM-min entrypoint, two chunks, plus files that must NOT be uploaded
// (source maps, other dist builds, and unrelated package files).
func mermaidFixture(t *testing.T) (tgz []byte, integrity string) {
	t.Helper()
	return buildTarball(t, []tarEntry{
		{"package/dist/mermaid.esm.min.mjs", "import './chunks/mermaid.esm.min/chunk-A.mjs';\n"},
		{"package/dist/mermaid.esm.min.mjs.map", "ENTRYPOINT MAP"},
		{"package/dist/chunks/mermaid.esm.min/chunk-A.mjs", "export const a = 1;\n"},
		{"package/dist/chunks/mermaid.esm.min/chunk-A.mjs.map", "CHUNK MAP"},
		{"package/dist/chunks/mermaid.esm.min/flowchart-XYZ.mjs", "export const flow = 2;\n"},
		{"package/dist/mermaid.min.js", "UMD BUILD"},
		{"package/dist/chunks/mermaid.core/chunk-A.mjs", "CORE VARIANT"},
		{"package/README.md", "readme"},
	})
}

// fetchFrom returns a FetchFunc that serves the given tarball bytes and records
// the requested URL.
func fetchFrom(tgz []byte, urlSeen *string) deps.FetchFunc {
	return func(_ context.Context, url string) (io.ReadCloser, error) {
		if urlSeen != nil {
			*urlSeen = url
		}
		return io.NopCloser(bytes.NewReader(tgz)), nil
	}
}

// storedObject captures the metadata a store recorded for one PutObject call.
type storedObject struct {
	contentType  string
	cacheControl string
	body         []byte
}

// fakeStore is an in-memory Uploader + ObjectLister recording every object and
// its content type / cache-control header.
type fakeStore struct {
	objects map[string]storedObject
}

func newFakeStore() *fakeStore { return &fakeStore{objects: map[string]storedObject{}} }

func (s *fakeStore) PutObject(_ context.Context, _, key, contentType string, body io.Reader, opts ...upload.PutOption) error {
	var o upload.PutOptions
	for _, opt := range opts {
		opt(&o)
	}
	b, err := io.ReadAll(body)
	if err != nil {
		return err
	}
	s.objects[key] = storedObject{contentType: contentType, cacheControl: o.CacheControl, body: b}
	return nil
}

func (s *fakeStore) ListObjects(_ context.Context, _, prefix string) ([]string, error) {
	want := prefix + "/"
	var keys []string
	for k := range s.objects {
		if len(k) >= len(want) && k[:len(want)] == want {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	return keys, nil
}

var (
	_ upload.Uploader     = (*fakeStore)(nil)
	_ upload.ObjectLister = (*fakeStore)(nil)
)

func (s *fakeStore) keys() []string {
	keys := make([]string, 0, len(s.objects))
	for k := range s.objects {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func TestPresent_FalseWhenAbsent(t *testing.T) {
	up := &upload.DirUploader{Root: t.TempDir()}

	present, err := deps.Present(context.Background(), up, "bucket", "11.16.0")
	require.NoError(t, err)
	assert.False(t, present)
}

func TestPublish_FetchesVerifiesAndUploadsESMTree(t *testing.T) {
	tgz, integrity := mermaidFixture(t)
	store := newFakeStore()
	var urlSeen string

	uploaded, err := deps.Publish(context.Background(), store, store, "bucket", "11.16.0", integrity, fetchFrom(tgz, &urlSeen), false, io.Discard)
	require.NoError(t, err)
	assert.True(t, uploaded)

	assert.Equal(t, "https://registry.npmjs.org/mermaid/-/mermaid-11.16.0.tgz", urlSeen)

	// only the ESM-min .mjs tree is uploaded, directory structure preserved
	assert.Equal(t, []string{
		"deps/mermaid/11.16.0/chunks/mermaid.esm.min/chunk-A.mjs",
		"deps/mermaid/11.16.0/chunks/mermaid.esm.min/flowchart-XYZ.mjs",
		"deps/mermaid/11.16.0/mermaid.esm.min.mjs",
	}, store.keys())

	entry := store.objects["deps/mermaid/11.16.0/mermaid.esm.min.mjs"]
	assert.Equal(t, "import './chunks/mermaid.esm.min/chunk-A.mjs';\n", string(entry.body))
	assert.Equal(t, "text/javascript; charset=utf-8", entry.contentType)
	assert.Equal(t, "public, max-age=31536000, immutable", entry.cacheControl)
}

func TestPublish_IntegrityMismatchAbortsWithNoUpload(t *testing.T) {
	tgz, _ := mermaidFixture(t)
	store := newFakeStore()

	badIntegrity := "sha512-" + base64.StdEncoding.EncodeToString(make([]byte, 64))
	uploaded, err := deps.Publish(context.Background(), store, store, "bucket", "11.16.0", badIntegrity, fetchFrom(tgz, nil), false, io.Discard)
	require.Error(t, err)
	assert.False(t, uploaded)
	assert.Empty(t, store.keys(), "a mismatched sha512 must upload nothing")
}

func TestPublish_SkipsWhenAlreadyPresent(t *testing.T) {
	tgz, integrity := mermaidFixture(t)
	store := newFakeStore()

	// first publish populates the store
	_, err := deps.Publish(context.Background(), store, store, "bucket", "11.16.0", integrity, fetchFrom(tgz, nil), false, io.Discard)
	require.NoError(t, err)
	before := store.keys()

	fetchCalled := false
	fetch := func(_ context.Context, _ string) (io.ReadCloser, error) {
		fetchCalled = true
		return nil, errors.New("fetch must not be called on skip")
	}

	uploaded, err := deps.Publish(context.Background(), store, store, "bucket", "11.16.0", integrity, fetch, false, io.Discard)
	require.NoError(t, err)
	assert.False(t, uploaded)
	assert.False(t, fetchCalled, "present version must not be re-fetched")
	assert.Equal(t, before, store.keys())
}

func TestPublish_ForceReuploadsWhenPresent(t *testing.T) {
	tgz, integrity := mermaidFixture(t)
	store := newFakeStore()

	_, err := deps.Publish(context.Background(), store, store, "bucket", "11.16.0", integrity, fetchFrom(tgz, nil), false, io.Discard)
	require.NoError(t, err)

	fetchCalled := false
	fetch := func(ctx context.Context, url string) (io.ReadCloser, error) {
		fetchCalled = true
		return fetchFrom(tgz, nil)(ctx, url)
	}

	uploaded, err := deps.Publish(context.Background(), store, store, "bucket", "11.16.0", integrity, fetch, true, io.Discard)
	require.NoError(t, err)
	assert.True(t, uploaded, "--force must re-upload even when present")
	assert.True(t, fetchCalled)
}

func TestPresent_TrueAfterPublish(t *testing.T) {
	tgz, integrity := mermaidFixture(t)
	store := newFakeStore()

	_, err := deps.Publish(context.Background(), store, store, "bucket", "11.16.0", integrity, fetchFrom(tgz, nil), false, io.Discard)
	require.NoError(t, err)

	present, err := deps.Present(context.Background(), store, "bucket", "11.16.0")
	require.NoError(t, err)
	assert.True(t, present)
}
