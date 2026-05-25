package createcmd_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jamestelfer/dollop/internal/cli/createcmd"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v3"
)

// fakeUploader records PutObject calls; returns err if set.
type fakeUploader struct {
	calls []string // keys uploaded
	err   error
}

func (f *fakeUploader) PutObject(_ context.Context, _, key, _ string, _ io.Reader) error {
	if f.err != nil {
		return f.err
	}
	f.calls = append(f.calls, key)
	return nil
}

func runCreate(t *testing.T, up *fakeUploader, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	var outBuf, errBuf bytes.Buffer
	cmd := createcmd.New(
		up,
		"test-bucket",
		"", // no base_url in unit tests; URL will be just the prefix path
		func() (string, error) { return "testid", nil },
		func() string { return "happy-cat" },
	)
	app := &cli.Command{
		Name:           "dollop",
		Writer:         &outBuf,
		ErrWriter:      &errBuf,
		Commands:       []*cli.Command{&cmd},
		ExitErrHandler: func(_ context.Context, _ *cli.Command, _ error) {},
	}
	err := app.Run(context.Background(), append([]string{"dollop"}, args...))
	if err != nil {
		var ec cli.ExitCoder
		if errors.As(err, &ec) {
			return outBuf.String(), errBuf.String(), ec.ExitCode()
		}
		return outBuf.String(), errBuf.String(), 1
	}
	return outBuf.String(), errBuf.String(), 0
}

func TestCreate_SingleFile_EphemeralDefault(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "notes.txt")
	require.NoError(t, os.WriteFile(path, []byte("hi"), 0600))

	up := &fakeUploader{}
	stdout, _, code := runCreate(t, up, "create", path)
	require.Equal(t, 0, code)

	// prefix: flash/1/testid — days defaults to 1, id fixed to "testid"
	assert.Equal(t, "flash/1/testid/notes.txt", up.calls[0])
	assert.Contains(t, stdout, "flash/1/testid/")
}

func TestCreate_EphemeralDays7(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "f.bin")
	require.NoError(t, os.WriteFile(path, []byte("x"), 0600))

	up := &fakeUploader{}
	stdout, _, code := runCreate(t, up, "create", "--days", "7", path)
	require.Equal(t, 0, code)
	assert.Equal(t, "flash/7/testid/f.bin", up.calls[0])
	assert.Contains(t, stdout, "flash/7/testid/")
}

func TestCreate_Keep(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "site")
	require.NoError(t, os.MkdirAll(path, 0700))
	require.NoError(t, os.WriteFile(filepath.Join(path, "index.html"), []byte("<html>"), 0600))

	up := &fakeUploader{}
	stdout, _, code := runCreate(t, up, "create", "--keep", path)
	require.Equal(t, 0, code)
	assert.Equal(t, "keep/happy-cat/index.html", up.calls[0])
	assert.Contains(t, stdout, "keep/happy-cat/")
}

func TestCreate_KeepAndDaysMutuallyExclusive(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "f.txt")
	require.NoError(t, os.WriteFile(path, []byte("x"), 0600))

	_, _, code := runCreate(t, &fakeUploader{}, "create", "--keep", "--days", "7", path)
	assert.NotEqual(t, 0, code, "--keep and --days together should be rejected")
}

func TestCreate_InvalidDays(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "f.txt")
	require.NoError(t, os.WriteFile(path, []byte("x"), 0600))

	_, _, code := runCreate(t, &fakeUploader{}, "create", "--days", "3", path)
	assert.NotEqual(t, 0, code)
}

func TestCreate_MissingArg(t *testing.T) {
	_, _, code := runCreate(t, &fakeUploader{}, "create")
	assert.NotEqual(t, 0, code)
}

func TestCreate_UploaderError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "f.txt")
	require.NoError(t, os.WriteFile(path, []byte("x"), 0600))

	up := &fakeUploader{err: errors.New("network error")}
	_, stderr, code := runCreate(t, up, "create", path)
	assert.NotEqual(t, 0, code)
	assert.True(t, strings.Contains(stderr, "network error") || strings.Contains(stderr, "upload"))
}

func TestCreate_URLToStdout_OnlyURL(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "f.txt")
	require.NoError(t, os.WriteFile(path, []byte("x"), 0600))

	up := &fakeUploader{}
	stdout, _, code := runCreate(t, up, "create", path)
	require.Equal(t, 0, code)

	lines := strings.Split(strings.TrimSpace(stdout), "\n")
	assert.Len(t, lines, 1, "stdout should contain only the URL")
	assert.True(t, strings.HasPrefix(lines[0], "http") || strings.HasPrefix(lines[0], "/"),
		"stdout should be a URL, got: %q", lines[0])
}
