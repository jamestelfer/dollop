package deps_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/jamestelfer/dollop/internal/deps"
	"github.com/jamestelfer/dollop/internal/upload"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var testFonts = deps.FontSet{
	Prefix: "deps/fonts/test",
	Files:  []string{"Face-Regular.woff2", "Face-Bold.woff2"},
}

// writeFile creates a file (and its parent directories) under dir.
func writeFile(t *testing.T, dir, rel, content string) {
	t.Helper()
	p := filepath.Join(dir, filepath.FromSlash(rel))
	require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o750))
	require.NoError(t, os.WriteFile(p, []byte(content), 0o600))
}

func TestPublishFonts_UploadsKnownFilesFromNestedDir(t *testing.T) {
	src, dst := t.TempDir(), t.TempDir()
	writeFile(t, src, "pack/woff2/Face-Regular.woff2", "REGULAR")
	writeFile(t, src, "pack/woff2/Face-Bold.woff2", "BOLD")
	writeFile(t, src, "pack/woff2/Face-Thin.woff2", "THIN")
	writeFile(t, src, "pack/EULA.pdf", "EULA")

	up := &upload.DirUploader{Root: dst}
	uploaded, err := deps.PublishFonts(context.Background(), up, "bucket", testFonts, src, &bytes.Buffer{})
	require.NoError(t, err)
	assert.Equal(t, []string{"Face-Regular.woff2", "Face-Bold.woff2"}, uploaded)

	got, err := os.ReadFile(filepath.Join(dst, "deps", "fonts", "test", "Face-Regular.woff2"))
	require.NoError(t, err)
	assert.Equal(t, "REGULAR", string(got))

	// files outside the font set must not be uploaded
	_, err = os.Stat(filepath.Join(dst, "deps", "fonts", "test", "Face-Thin.woff2"))
	assert.True(t, os.IsNotExist(err))
	_, err = os.Stat(filepath.Join(dst, "deps", "fonts", "test", "EULA.pdf"))
	assert.True(t, os.IsNotExist(err))
}

func TestPublishFonts_PartialSetWarns(t *testing.T) {
	src, dst := t.TempDir(), t.TempDir()
	writeFile(t, src, "Face-Regular.woff2", "REGULAR")

	var stderr bytes.Buffer
	uploaded, err := deps.PublishFonts(context.Background(), &upload.DirUploader{Root: dst}, "bucket", testFonts, src, &stderr)
	require.NoError(t, err)
	assert.Equal(t, []string{"Face-Regular.woff2"}, uploaded)
	assert.Contains(t, stderr.String(), "warning: Face-Bold.woff2 not found")
}

func TestPublishFonts_NoFontsIsError(t *testing.T) {
	src, dst := t.TempDir(), t.TempDir()
	writeFile(t, src, "notes.txt", "x")

	_, err := deps.PublishFonts(context.Background(), &upload.DirUploader{Root: dst}, "bucket", testFonts, src, &bytes.Buffer{})
	require.Error(t, err)
	_, statErr := os.Stat(filepath.Join(dst, "deps"))
	assert.True(t, os.IsNotExist(statErr))
}

func TestFontsPresent_ListsPublishedFiles(t *testing.T) {
	src, dst := t.TempDir(), t.TempDir()
	writeFile(t, src, "Face-Regular.woff2", "REGULAR")
	dir := &upload.DirUploader{Root: dst}

	present, err := deps.FontsPresent(context.Background(), dir, "bucket", testFonts)
	require.NoError(t, err)
	assert.Empty(t, present)

	_, err = deps.PublishFonts(context.Background(), dir, "bucket", testFonts, src, &bytes.Buffer{})
	require.NoError(t, err)

	present, err = deps.FontsPresent(context.Background(), dir, "bucket", testFonts)
	require.NoError(t, err)
	assert.Equal(t, []string{"Face-Regular.woff2"}, present)
}

func TestPublishFonts_FollowsSymlinkedRoot(t *testing.T) {
	src, dst := t.TempDir(), t.TempDir()
	writeFile(t, src, "real/Face-Regular.woff2", "REGULAR")
	link := filepath.Join(src, "link")
	require.NoError(t, os.Symlink(filepath.Join(src, "real"), link))

	uploaded, err := deps.PublishFonts(context.Background(), &upload.DirUploader{Root: dst}, "bucket", testFonts, link, &bytes.Buffer{})
	require.NoError(t, err)
	assert.Equal(t, []string{"Face-Regular.woff2"}, uploaded)
}

func TestPublishFonts_RefusesSymlinkOutsideDir(t *testing.T) {
	outside, src, dst := t.TempDir(), t.TempDir(), t.TempDir()
	writeFile(t, outside, "secret.woff2", "SECRET")
	require.NoError(t, os.Symlink(filepath.Join(outside, "secret.woff2"), filepath.Join(src, "Face-Regular.woff2")))

	_, err := deps.PublishFonts(context.Background(), &upload.DirUploader{Root: dst}, "bucket", testFonts, src, &bytes.Buffer{})
	require.Error(t, err)
	_, statErr := os.Stat(filepath.Join(dst, "deps"))
	assert.True(t, os.IsNotExist(statErr), "nothing outside the directory may be uploaded")
}

func TestPublishFonts_DuplicateNameUsesFirstAndWarns(t *testing.T) {
	src, dst := t.TempDir(), t.TempDir()
	writeFile(t, src, "desktop/Face-Regular.woff2", "DESKTOP")
	writeFile(t, src, "web/Face-Regular.woff2", "WEB")

	var stderr bytes.Buffer
	_, err := deps.PublishFonts(context.Background(), &upload.DirUploader{Root: dst}, "bucket", testFonts, src, &stderr)
	require.NoError(t, err)

	got, err := os.ReadFile(filepath.Join(dst, "deps", "fonts", "test", "Face-Regular.woff2"))
	require.NoError(t, err)
	assert.Equal(t, "DESKTOP", string(got))
	assert.Contains(t, stderr.String(), "warning: ignoring web/Face-Regular.woff2")
}
