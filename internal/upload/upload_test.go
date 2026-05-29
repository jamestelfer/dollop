package upload_test

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/jamestelfer/dollop/internal/render"
	"github.com/jamestelfer/dollop/internal/upload"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeUploader records calls to PutObject.
type fakeUploader struct {
	calls []putCall
	err   error
}

type putCall struct {
	bucket      string
	key         string
	contentType string
	body        []byte
}

func (f *fakeUploader) PutObject(_ context.Context, bucket, key, contentType string, body io.Reader, _ ...upload.PutOption) error {
	if f.err != nil {
		return f.err
	}
	data, err := io.ReadAll(body)
	if err != nil {
		return err
	}
	f.calls = append(f.calls, putCall{bucket, key, contentType, data})
	return nil
}

func TestUploadFiles_SingleFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "hello.txt")
	require.NoError(t, os.WriteFile(path, []byte("hello"), 0600))

	up := &fakeUploader{}
	var stderr bytes.Buffer
	files, err := upload.UploadFiles(context.Background(), up, "my-bucket", "dollop/1/abc", path, false, &stderr)
	require.NoError(t, err)

	require.Len(t, up.calls, 1)
	assert.Equal(t, "my-bucket", up.calls[0].bucket)
	assert.Equal(t, "dollop/1/abc/hello.txt", up.calls[0].key)
	assert.Contains(t, up.calls[0].contentType, "text/plain")
	assert.Equal(t, []byte("hello"), up.calls[0].body)
	assert.Contains(t, stderr.String(), "uploading [hello.txt]")
	assert.Contains(t, stderr.String(), "...done")
	assert.Equal(t, []string{"hello.txt"}, files)
}

func TestUploadFiles_Directory(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "sub"), 0700))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "index.html"), []byte("<h1>hi</h1>"), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "sub", "data.json"), []byte("{}"), 0600))

	up := &fakeUploader{}
	var stderr bytes.Buffer
	files, err := upload.UploadFiles(context.Background(), up, "bucket", "keep/friendly-cat", dir, false, &stderr)
	require.NoError(t, err)

	require.Len(t, up.calls, 2)

	keys := make([]string, len(up.calls))
	for i, c := range up.calls {
		keys[i] = c.key
	}
	assert.ElementsMatch(t, []string{
		"keep/friendly-cat/index.html",
		"keep/friendly-cat/sub/data.json",
	}, keys)
	assert.ElementsMatch(t, []string{"index.html", "sub/data.json"}, files)
}

func TestUploadFiles_UploaderError(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "file.bin"), []byte("x"), 0600))

	up := &fakeUploader{err: assert.AnError}
	var stderr bytes.Buffer
	_, err := upload.UploadFiles(context.Background(), up, "b", "p", filepath.Join(dir, "file.bin"), false, &stderr)
	assert.Error(t, err)
	assert.Contains(t, stderr.String(), "uploading [file.bin]")
	assert.Contains(t, stderr.String(), "...failed")
}

func TestUploadFiles_Index_GeneratesAndUploads(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "report.pdf"), []byte("pdf"), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("notes"), 0600))

	up := &fakeUploader{}
	var stderr bytes.Buffer
	files, err := upload.UploadFiles(context.Background(), up, "bucket", "flash/1/abc", dir, true, &stderr)
	require.NoError(t, err)

	keys := make([]string, len(up.calls))
	for i, c := range up.calls {
		keys[i] = c.key
	}
	assert.Contains(t, keys, "flash/1/abc/index.html")

	// index.html is uploaded with the right content type
	var indexCall *putCall
	for i := range up.calls {
		if up.calls[i].key == "flash/1/abc/index.html" {
			indexCall = &up.calls[i]
			break
		}
	}
	require.NotNil(t, indexCall)
	assert.Contains(t, indexCall.contentType, "text/html")
	assert.Contains(t, string(indexCall.body), "notes.txt")
	assert.Contains(t, string(indexCall.body), "report.pdf")

	assert.NotContains(t, stderr.String(), "warning")

	// generated index.html is not in the returned file list
	assert.ElementsMatch(t, []string{"notes.txt", "report.pdf"}, files)
}

func TestUploadFiles_Index_SkipsIfExists(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "index.html"), []byte("<h1>mine</h1>"), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "data.csv"), []byte("a,b"), 0600))

	up := &fakeUploader{}
	var stderr bytes.Buffer
	files, err := upload.UploadFiles(context.Background(), up, "bucket", "flash/1/xyz", dir, true, &stderr)
	require.NoError(t, err)

	// only the two real files, no generated index
	require.Len(t, up.calls, 2)
	assert.Contains(t, stderr.String(), "warning")
	assert.Contains(t, stderr.String(), "index.html")
	assert.ElementsMatch(t, []string{"index.html", "data.csv"}, files)
}

func TestUploadFiles_Index_SingleFile_IsIndex(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "index.html")
	require.NoError(t, os.WriteFile(path, []byte("<h1>hi</h1>"), 0600))

	up := &fakeUploader{}
	var stderr bytes.Buffer
	files, err := upload.UploadFiles(context.Background(), up, "bucket", "flash/1/xyz", path, true, &stderr)
	require.NoError(t, err)

	require.Len(t, up.calls, 1) // only the file itself, no generated index
	assert.Contains(t, stderr.String(), "warning")
	assert.Equal(t, []string{"index.html"}, files)
}

func TestUploadFiles_Render_MermaidBatchUploadsJSOnce(t *testing.T) {
	dir := t.TempDir()
	mermaidMD := "```mermaid\ngraph TD\n    A --> B\n```\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.md"), []byte(mermaidMD), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "b.md"), []byte("# Plain"), 0o600))

	up := &fakeUploader{}
	var stderr bytes.Buffer
	_, err := upload.UploadFiles(context.Background(), up, "bucket", "flash/1/abc", dir, false, &stderr, upload.WithRenderer(render.NewMarkdownRenderer()))
	require.NoError(t, err)

	mermaidCount := 0
	for _, c := range up.calls {
		if c.key == "flash/1/abc/mermaid.min.js" {
			mermaidCount++
		}
	}
	assert.Equal(t, 1, mermaidCount, "mermaid.min.js should be uploaded exactly once")
}

func TestUploadFiles_Render_SharedAssetsUploadedOnce(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.md"), []byte("# A"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "b.md"), []byte("# B"), 0o600))

	up := &fakeUploader{}
	var stderr bytes.Buffer
	files, err := upload.UploadFiles(context.Background(), up, "bucket", "flash/1/abc", dir, false, &stderr, upload.WithRenderer(render.NewMarkdownRenderer()))
	require.NoError(t, err)

	// count CSS uploads
	cssCount := 0
	for _, c := range up.calls {
		if c.key == "flash/1/abc/github-markdown.css" {
			cssCount++
		}
	}
	assert.Equal(t, 1, cssCount, "shared CSS should be uploaded exactly once")

	// shared assets must not appear in returned relPaths
	assert.NotContains(t, files, "github-markdown.css")
	assert.NotContains(t, files, "highlight-github.css")
}

func TestUploadFiles_Render_IndexMdSetsHasIndex(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "index.md"), []byte("# Home"), 0600))

	up := &fakeUploader{}
	var stderr bytes.Buffer
	// generateIndex=true so we can observe the hasIndex collision warning
	files, err := upload.UploadFiles(context.Background(), up, "bucket", "flash/1/abc", dir, true, &stderr, upload.WithRenderer(render.NewMarkdownRenderer()))
	require.NoError(t, err)

	// rendered index.html triggers the "already present" warning, not a generated one
	assert.Contains(t, stderr.String(), "warning")
	assert.Contains(t, stderr.String(), "index.html")

	// both index.md and index.html are in the returned paths
	assert.Contains(t, files, "index.md")
	assert.Contains(t, files, "index.html")
}

func TestUploadFiles_Render_MarkdownProducesHTML(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "notes.md"), []byte("# Hello"), 0600))

	up := &fakeUploader{}
	var stderr bytes.Buffer
	files, err := upload.UploadFiles(context.Background(), up, "bucket", "flash/1/abc", dir, false, &stderr, upload.WithRenderer(render.NewMarkdownRenderer()))
	require.NoError(t, err)

	keys := make([]string, len(up.calls))
	for i, c := range up.calls {
		keys[i] = c.key
	}
	assert.Contains(t, keys, "flash/1/abc/notes.md")
	assert.Contains(t, keys, "flash/1/abc/notes.html")
	assert.ElementsMatch(t, []string{"notes.md", "notes.html"}, files)
}

func TestUploadFiles_Render_NoRender_SkipsHTML(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "notes.md"), []byte("# Hello"), 0600))

	up := &fakeUploader{}
	var stderr bytes.Buffer
	files, err := upload.UploadFiles(context.Background(), up, "bucket", "flash/1/abc", dir, false, &stderr)
	require.NoError(t, err)

	keys := make([]string, len(up.calls))
	for i, c := range up.calls {
		keys[i] = c.key
	}
	assert.Contains(t, keys, "flash/1/abc/notes.md")
	assert.NotContains(t, keys, "flash/1/abc/notes.html")
	assert.Equal(t, []string{"notes.md"}, files)
}

func TestUploadFiles_NilUploader_ReturnsError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "f.txt")
	require.NoError(t, os.WriteFile(path, []byte("x"), 0600))

	var stderr bytes.Buffer
	// A nil Uploader must produce an error, not a nil pointer panic.
	_, err := upload.UploadFiles(context.Background(), nil, "b", "p", path, false, &stderr)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "uploader")
}

func TestHumanSize(t *testing.T) {
	tests := []struct {
		n    int64
		want string
	}{
		{0, "0 B"},
		{512, "512 B"},
		{1023, "1023 B"},
		{1024, "1.0 KB"},
		{1536, "1.5 KB"},
		{1024 * 1024, "1.0 MB"},
		{int64(1.5 * 1024 * 1024), "1.5 MB"},
		{1024 * 1024 * 1024, "1.0 GB"},
		{2_469_606_195, "2.3 GB"},
	}
	for _, tc := range tests {
		assert.Equal(t, tc.want, upload.HumanSize(tc.n), "n=%d", tc.n)
	}
}
