package upload_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/jamestelfer/dollop/internal/upload"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDirUploader_WritesBodyAtKey(t *testing.T) {
	root := t.TempDir()
	up := &upload.DirUploader{Root: root}

	err := up.PutObject(context.Background(), "ignored-bucket", "flash/1/abc/file.txt", "text/plain", bytes.NewReader([]byte("hello")))
	require.NoError(t, err)

	got, err := os.ReadFile(filepath.Join(root, "flash", "1", "abc", "file.txt"))
	require.NoError(t, err)
	assert.Equal(t, "hello", string(got))
}

func TestDirUploader_CreatesIntermediateDirectories(t *testing.T) {
	root := t.TempDir()
	up := &upload.DirUploader{Root: root}

	err := up.PutObject(context.Background(), "b", "a/b/c/d/e.bin", "application/octet-stream", bytes.NewReader([]byte{1, 2, 3}))
	require.NoError(t, err)

	info, err := os.Stat(filepath.Join(root, "a", "b", "c", "d", "e.bin"))
	require.NoError(t, err)
	assert.False(t, info.IsDir())
}

func TestDirUploader_MultipleObjects(t *testing.T) {
	root := t.TempDir()
	up := &upload.DirUploader{Root: root}
	ctx := context.Background()

	require.NoError(t, up.PutObject(ctx, "b", "prefix/a.txt", "text/plain", bytes.NewReader([]byte("aaa"))))
	require.NoError(t, up.PutObject(ctx, "b", "prefix/b.txt", "text/plain", bytes.NewReader([]byte("bbb"))))

	a, err := os.ReadFile(filepath.Join(root, "prefix", "a.txt"))
	require.NoError(t, err)
	assert.Equal(t, "aaa", string(a))

	b, err := os.ReadFile(filepath.Join(root, "prefix", "b.txt"))
	require.NoError(t, err)
	assert.Equal(t, "bbb", string(b))
}

func TestDirUploader_ListObjects_MultiFilePrefix(t *testing.T) {
	root := t.TempDir()
	up := &upload.DirUploader{Root: root}
	ctx := context.Background()

	// Populate a prefix with files in nested directories, plus a sibling prefix
	// and a name-stem sibling that must NOT be returned.
	put := func(key string) {
		require.NoError(t, up.PutObject(ctx, "b", key, "text/plain", bytes.NewReader([]byte("x"))))
	}
	put("flash/7/abc/index.html")
	put("flash/7/abc/style.css")
	put("flash/7/abc/sub/page.html")
	put("flash/7/abcdef/other.txt") // name-stem sibling, must be excluded
	put("keep/happy-cat/notes.md")  // unrelated prefix

	keys, err := up.ListObjects(ctx, "b", "flash/7/abc")
	require.NoError(t, err)

	assert.Equal(t, []string{
		"flash/7/abc/index.html",
		"flash/7/abc/style.css",
		"flash/7/abc/sub/page.html",
	}, keys, "keys are full and lexically ordered; siblings excluded")
}

func TestDirUploader_ListObjects_TrailingSlashPrefix(t *testing.T) {
	root := t.TempDir()
	up := &upload.DirUploader{Root: root}
	ctx := context.Background()
	require.NoError(t, up.PutObject(ctx, "b", "flash/1/xyz/a.txt", "text/plain", bytes.NewReader([]byte("a"))))

	// A trailing slash on the prefix argument is tolerated.
	keys, err := up.ListObjects(ctx, "b", "flash/1/xyz/")
	require.NoError(t, err)
	assert.Equal(t, []string{"flash/1/xyz/a.txt"}, keys)
}

func TestDirUploader_ListObjects_MissingPrefixEmpty(t *testing.T) {
	root := t.TempDir()
	up := &upload.DirUploader{Root: root}

	keys, err := up.ListObjects(context.Background(), "b", "flash/7/nonexistent")
	require.NoError(t, err)
	assert.Empty(t, keys, "a missing prefix yields no keys and no error")
}

func TestDirUploader_OverwritesExistingFile(t *testing.T) {
	root := t.TempDir()
	up := &upload.DirUploader{Root: root}
	ctx := context.Background()

	require.NoError(t, up.PutObject(ctx, "b", "k/f.txt", "text/plain", bytes.NewReader([]byte("old"))))
	require.NoError(t, up.PutObject(ctx, "b", "k/f.txt", "text/plain", bytes.NewReader([]byte("new"))))

	got, err := os.ReadFile(filepath.Join(root, "k", "f.txt"))
	require.NoError(t, err)
	assert.Equal(t, "new", string(got))
}
