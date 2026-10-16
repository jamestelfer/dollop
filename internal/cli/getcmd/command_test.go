package getcmd_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jamestelfer/dollop/internal/cli/getcmd"
	"github.com/jamestelfer/dollop/internal/upload"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v3"
)

func runGet(t *testing.T, r upload.ObjectReader, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	var outBuf, errBuf bytes.Buffer
	cmd := getcmd.New(r, "test-bucket", "https://drop.example.com")
	app := &cli.Command{
		Name:           "dollop",
		Writer:         &outBuf,
		ErrWriter:      &errBuf,
		Commands:       []*cli.Command{&cmd},
		ExitErrHandler: func(_ context.Context, _ *cli.Command, _ error) {},
	}
	err := app.Run(context.Background(), append([]string{"dollop", "get"}, args...))
	if err != nil {
		if ec, ok := errors.AsType[cli.ExitCoder](err); ok {
			return outBuf.String(), errBuf.String(), ec.ExitCode()
		}
		return outBuf.String(), errBuf.String(), 1
	}
	return outBuf.String(), errBuf.String(), 0
}

func seed(t *testing.T) *upload.DirUploader {
	t.Helper()
	store := &upload.DirUploader{Root: t.TempDir()}
	for key, body := range map[string]string{
		"flash/7/abc/index.html": "idx",
		"flash/7/abc/docs/a.md":  "a",
	} {
		require.NoError(t, store.PutObject(context.Background(), "b", key, "text/plain", strings.NewReader(body)))
	}
	return store
}

func TestGet_UsageErrors(t *testing.T) {
	_, _, code := runGet(t, seed(t))
	assert.NotEqual(t, 0, code)
	_, _, code = runGet(t, seed(t), "flash/7/abc", "a", "b")
	assert.NotEqual(t, 0, code)
}

func TestGet_NoCredentials(t *testing.T) {
	_, _, code := runGet(t, nil, "flash/7/abc", t.TempDir())
	assert.NotEqual(t, 0, code)
}

func TestGet_RejectsNonUploadReference(t *testing.T) {
	for _, ref := range []string{"deps/mermaid/11.0.0", "flash/7", "flash/7/abc/../x", "/"} {
		_, _, code := runGet(t, seed(t), ref, t.TempDir())
		assert.NotEqual(t, 0, code, ref)
	}
}

func TestGet_WholeUploadFromURL(t *testing.T) {
	target := filepath.Join(t.TempDir(), "out")
	stdout, stderr, code := runGet(t, seed(t), "https://drop.example.com/flash/7/abc/", target)
	require.Equal(t, 0, code, stderr)
	assert.Equal(t, target+"\n", stdout)
	assert.Contains(t, stderr, "downloading [docs/a.md]")
	assert.FileExists(t, filepath.Join(target, "index.html"))
	assert.FileExists(t, filepath.Join(target, "docs", "a.md"))
}

func TestGet_SingleFile(t *testing.T) {
	target := t.TempDir()
	stdout, stderr, code := runGet(t, seed(t), "flash/7/abc/docs/a.md", target)
	require.Equal(t, 0, code, stderr)
	assert.Equal(t, filepath.Join(target, "a.md")+"\n", stdout)
}

func TestGet_ForceFlag(t *testing.T) {
	target := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(target, "a.md"), []byte("local"), 0o600))

	_, stderr, code := runGet(t, seed(t), "flash/7/abc/docs/a.md", target)
	assert.NotEqual(t, 0, code)
	assert.Contains(t, stderr, "--force")

	_, stderr, code = runGet(t, seed(t), "--force", "flash/7/abc/docs/a.md", target)
	require.Equal(t, 0, code, stderr)
	b, err := os.ReadFile(filepath.Join(target, "a.md"))
	require.NoError(t, err)
	assert.Equal(t, "a", string(b))
}

func TestGet_CopyDir(t *testing.T) {
	store := seed(t)
	target := t.TempDir()
	_, stderr, code := runGet(t, nil, "--copy-dir", store.Root, "flash/7/abc", target)
	require.Equal(t, 0, code, stderr)
	assert.FileExists(t, filepath.Join(target, "index.html"))
}
