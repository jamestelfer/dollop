package upload_test

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

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

func (f *fakeUploader) PutObject(_ context.Context, bucket, key, contentType string, body io.Reader) error {
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
	err := upload.UploadFiles(context.Background(), up, "my-bucket", "dollop/1/abc", path, &stderr)
	require.NoError(t, err)

	require.Len(t, up.calls, 1)
	assert.Equal(t, "my-bucket", up.calls[0].bucket)
	assert.Equal(t, "dollop/1/abc/hello.txt", up.calls[0].key)
	assert.Contains(t, up.calls[0].contentType, "text/plain")
	assert.Equal(t, []byte("hello"), up.calls[0].body)
	assert.Contains(t, stderr.String(), "dollop/1/abc/hello.txt")
}

func TestUploadFiles_Directory(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "sub"), 0700))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "index.html"), []byte("<h1>hi</h1>"), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "sub", "data.json"), []byte("{}"), 0600))

	up := &fakeUploader{}
	var stderr bytes.Buffer
	err := upload.UploadFiles(context.Background(), up, "bucket", "keep/friendly-cat", dir, &stderr)
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
}

func TestUploadFiles_UploaderError(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "file.bin"), []byte("x"), 0600))

	up := &fakeUploader{err: assert.AnError}
	var stderr bytes.Buffer
	err := upload.UploadFiles(context.Background(), up, "b", "p", filepath.Join(dir, "file.bin"), &stderr)
	assert.Error(t, err)
}
