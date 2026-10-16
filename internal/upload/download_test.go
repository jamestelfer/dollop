package upload_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jamestelfer/dollop/internal/upload"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// seedStore returns a DirUploader populated with the given key → body objects.
func seedStore(t *testing.T, objects map[string]string) *upload.DirUploader {
	t.Helper()
	store := &upload.DirUploader{Root: t.TempDir()}
	for key, body := range objects {
		require.NoError(t, store.PutObject(context.Background(), "b", key, "text/plain", strings.NewReader(body)))
	}
	return store
}

func mustRef(t *testing.T, input string) upload.GetRef {
	t.Helper()
	ref, err := upload.ResolveGetRef("", input)
	require.NoError(t, err)
	return ref
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	require.NoError(t, err)
	return string(b)
}

var sampleObjects = map[string]string{
	"flash/7/abc/index.html":     "idx",
	"flash/7/abc/docs/a.md":      "a",
	"flash/7/abc/docs/a.html":    "a-html",
	"flash/7/abc/docs-old/z.txt": "z",
	"flash/7/abcdef/other.txt":   "other",
}

func TestDownload_WholeUpload(t *testing.T) {
	store := seedStore(t, sampleObjects)
	target := filepath.Join(t.TempDir(), "out")
	var stderr bytes.Buffer

	out, err := upload.Download(context.Background(), store, "b", mustRef(t, "flash/7/abc"), target, false, &stderr)
	require.NoError(t, err)
	assert.Equal(t, target, out)

	assert.Equal(t, "idx", readFile(t, filepath.Join(target, "index.html")))
	assert.Equal(t, "a", readFile(t, filepath.Join(target, "docs", "a.md")))
	assert.Equal(t, "a-html", readFile(t, filepath.Join(target, "docs", "a.html")))
	assert.Equal(t, "z", readFile(t, filepath.Join(target, "docs-old", "z.txt")))
	assert.NoFileExists(t, filepath.Join(target, "other.txt"), "sibling upload sharing a name stem is excluded")
	assert.Contains(t, stderr.String(), "downloading [docs/a.md] 1 B...done")
}

func TestDownload_Subdirectory_FullNameMatch(t *testing.T) {
	store := seedStore(t, sampleObjects)
	target := t.TempDir()

	out, err := upload.Download(context.Background(), store, "b", mustRef(t, "flash/7/abc/docs"), target, false, io.Discard)
	require.NoError(t, err)
	assert.Equal(t, target, out)

	entries, err := os.ReadDir(target)
	require.NoError(t, err)
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	assert.ElementsMatch(t, []string{"a.md", "a.html"}, names, "docs-old is not matched by docs")
}

func TestDownload_SingleFile(t *testing.T) {
	store := seedStore(t, sampleObjects)
	target := t.TempDir()

	out, err := upload.Download(context.Background(), store, "b", mustRef(t, "flash/7/abc/docs/a.md"), target, false, io.Discard)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(target, "a.md"), out)
	assert.Equal(t, "a", readFile(t, out))
}

func TestDownload_NothingFound(t *testing.T) {
	store := seedStore(t, sampleObjects)
	_, err := upload.Download(context.Background(), store, "b", mustRef(t, "flash/7/missing"), t.TempDir(), false, io.Discard)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "nothing found")
}

func TestDownload_ExistingFileFailsWithoutForce(t *testing.T) {
	store := seedStore(t, sampleObjects)
	target := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(target, "docs"), 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(target, "docs", "a.md"), []byte("local"), 0o600))

	_, err := upload.Download(context.Background(), store, "b", mustRef(t, "flash/7/abc"), target, false, io.Discard)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "docs/a.md")
	assert.Contains(t, err.Error(), "--force")

	assert.Equal(t, "local", readFile(t, filepath.Join(target, "docs", "a.md")), "existing file untouched")
	assert.NoFileExists(t, filepath.Join(target, "index.html"), "nothing downloaded after a failed pre-flight")
}

func TestDownload_ForceOverwrites(t *testing.T) {
	store := seedStore(t, sampleObjects)
	target := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(target, "index.html"), []byte("local"), 0o600))

	_, err := upload.Download(context.Background(), store, "b", mustRef(t, "flash/7/abc"), target, true, io.Discard)
	require.NoError(t, err)
	assert.Equal(t, "idx", readFile(t, filepath.Join(target, "index.html")))
}

func TestDownload_TypeMismatchFailsEvenWithForce(t *testing.T) {
	store := seedStore(t, sampleObjects)

	t.Run("directory where file goes", func(t *testing.T) {
		target := t.TempDir()
		require.NoError(t, os.MkdirAll(filepath.Join(target, "index.html"), 0o750))
		_, err := upload.Download(context.Background(), store, "b", mustRef(t, "flash/7/abc"), target, true, io.Discard)
		assert.Error(t, err)
	})
	t.Run("file where directory goes", func(t *testing.T) {
		target := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(target, "docs"), []byte("x"), 0o600))
		_, err := upload.Download(context.Background(), store, "b", mustRef(t, "flash/7/abc"), target, true, io.Discard)
		assert.Error(t, err)
	})
	t.Run("target is a file", func(t *testing.T) {
		target := filepath.Join(t.TempDir(), "f")
		require.NoError(t, os.WriteFile(target, []byte("x"), 0o600))
		_, err := upload.Download(context.Background(), store, "b", mustRef(t, "flash/7/abc"), target, true, io.Discard)
		assert.Error(t, err)
	})
}

// fakeReader serves fixed keys from memory, letting tests return keys a real
// listing should never produce.
type fakeReader struct {
	objects map[string]string
	keys    []string
}

func (f *fakeReader) ListObjects(_ context.Context, _, _ string) ([]string, error) {
	return f.keys, nil
}

func (f *fakeReader) GetObject(_ context.Context, _, key string) (io.ReadCloser, int64, error) {
	body, ok := f.objects[key]
	if !ok {
		return nil, 0, fmt.Errorf("%s: %w", key, upload.ErrObjectNotFound)
	}
	return io.NopCloser(strings.NewReader(body)), int64(len(body)), nil
}

func TestDownload_RejectsUnsafeListedKeys(t *testing.T) {
	for _, key := range []string{
		"flash/7/abc/../escape.txt",
		"flash/7/abc/a//b",
		"flash/7/abcdef/x",
		"keep/other/x",
		"flash/7/abc/a\\..\\b",
	} {
		t.Run(key, func(t *testing.T) {
			parent := t.TempDir()
			target := filepath.Join(parent, "out")
			r := &fakeReader{objects: map[string]string{key: "x"}, keys: []string{key}}
			_, err := upload.Download(context.Background(), r, "b", mustRef(t, "flash/7/abc"), target, true, io.Discard)
			require.Error(t, err)
			assert.NoFileExists(t, filepath.Join(parent, "escape.txt"))
		})
	}
}

func TestDownload_RejectsFileDirectoryCollisionInUpload(t *testing.T) {
	r := &fakeReader{
		objects: map[string]string{"keep/n/a": "1", "keep/n/a/b": "2"},
		keys:    []string{"keep/n/a", "keep/n/a/b"},
	}
	_, err := upload.Download(context.Background(), r, "b", mustRef(t, "keep/n"), t.TempDir(), false, io.Discard)
	assert.Error(t, err)
}

func TestDownload_DoesNotFollowSymlinkOutOfTarget(t *testing.T) {
	store := seedStore(t, sampleObjects)
	outside := t.TempDir()
	target := t.TempDir()
	require.NoError(t, os.Symlink(outside, filepath.Join(target, "docs")))

	_, err := upload.Download(context.Background(), store, "b", mustRef(t, "flash/7/abc"), target, true, io.Discard)
	require.Error(t, err)
	assert.NoFileExists(t, filepath.Join(outside, "a.md"))
}
