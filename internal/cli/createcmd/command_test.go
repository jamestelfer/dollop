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
	"github.com/jamestelfer/dollop/internal/upload"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v3"
)

// fakeUploader records PutObject calls; returns err if set.
type fakeUploader struct {
	calls []string // keys uploaded
	err   error
}

func (f *fakeUploader) PutObject(_ context.Context, _, key, _ string, _ io.Reader, _ ...upload.PutOption) error {
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

func TestCreate_NilUploader_FriendlyError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "notes.txt")
	require.NoError(t, os.WriteFile(path, []byte("hi"), 0600))

	var outBuf, errBuf bytes.Buffer
	// nil uploader simulates missing R2 credentials/account-id at startup.
	cmd := createcmd.New(
		nil,
		"test-bucket",
		"",
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
	err := app.Run(context.Background(), []string{"dollop", "create", path})

	// Must fail cleanly with a non-zero exit, not panic.
	require.Error(t, err)
	var ec cli.ExitCoder
	require.ErrorAs(t, err, &ec)
	assert.NotEqual(t, 0, ec.ExitCode())
	assert.Contains(t, err.Error(), "credentials")
	assert.Empty(t, outBuf.String(), "no URL should be printed when uploader is unconfigured")
}

func TestCreate_NilUploader_CopyDirStillWorks(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(src, "data.txt"), []byte("content"), 0o600))

	var outBuf, errBuf bytes.Buffer
	// Even with no uploader configured, --copy-dir must work (it uses DirUploader).
	cmd := createcmd.New(
		nil,
		"test-bucket",
		"",
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
	require.NoError(t, app.Run(context.Background(), []string{"dollop", "create", "--copy-dir", dst, src}))

	got, err := os.ReadFile(filepath.Join(dst, "flash", "1", "testid", "data.txt"))
	require.NoError(t, err)
	assert.Equal(t, "content", string(got))
}

func TestCreate_NonTTY_OutputIsPlainURL(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "notes.txt")
	require.NoError(t, os.WriteFile(path, []byte("hi"), 0600))

	up := &fakeUploader{}
	stdout, _, code := runCreate(t, up, "create", path)
	require.Equal(t, 0, code)

	// bytes.Buffer is not a TTY — stdout must contain no ANSI escape sequences
	assert.NotContains(t, stdout, "\x1b", "non-TTY output must not contain escape sequences")
	assert.Contains(t, stdout, "flash/1/testid")
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
	assert.Contains(t, stdout, "flash/1/testid/notes.txt")
}

func TestCreate_EphemeralDays7(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "f.bin")
	require.NoError(t, os.WriteFile(path, []byte("x"), 0600))

	up := &fakeUploader{}
	stdout, _, code := runCreate(t, up, "create", "--days", "7", path)
	require.Equal(t, 0, code)
	assert.Equal(t, "flash/7/testid/f.bin", up.calls[0])
	assert.Contains(t, stdout, "flash/7/testid/f.bin")
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
	// index.html in the upload → no filename suffix on URL
	assert.Contains(t, stdout, "keep/happy-cat/")
	assert.NotContains(t, strings.TrimSpace(stdout), "index.html")
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

func TestCreate_Index_GeneratesIndex(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "report.pdf"), []byte("pdf"), 0600))

	up := &fakeUploader{}
	stdout, stderr, code := runCreate(t, up, "create", "--index", dir)
	require.Equal(t, 0, code)

	keys := make([]string, len(up.calls))
	copy(keys, up.calls)
	assert.Contains(t, keys, "flash/1/testid/index.html")
	assert.NotContains(t, stderr, "warning")
	// --index flag → no filename suffix
	assert.Contains(t, stdout, "flash/1/testid/")
	assert.NotContains(t, strings.TrimSpace(stdout), "report.pdf")
}

func TestCreate_Index_SkipsWithWarningIfIndexExists(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "index.html"), []byte("<h1>mine</h1>"), 0600))

	up := &fakeUploader{}
	stdout, stderr, code := runCreate(t, up, "create", "--index", dir)
	require.Equal(t, 0, code)

	// no generated index — only the real index.html
	assert.Len(t, up.calls, 1)
	assert.Equal(t, "flash/1/testid/index.html", up.calls[0])
	assert.Contains(t, stderr, "warning")
	// --index flag → no filename suffix
	assert.Contains(t, stdout, "flash/1/testid/")
	assert.NotContains(t, strings.TrimSpace(stdout), "index.html")
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

func TestCreate_URL_MultipleFiles_FirstAlphabetical(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "zebra.txt"), []byte("z"), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "alpha.txt"), []byte("a"), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "mango.txt"), []byte("m"), 0600))

	up := &fakeUploader{}
	stdout, _, code := runCreate(t, up, "create", dir)
	require.Equal(t, 0, code)

	assert.Contains(t, stdout, "flash/1/testid/alpha.txt")
}

func TestCreate_Render_MarkdownProducesHTMLByDefault(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "notes.md"), []byte("# Hello"), 0600))

	up := &fakeUploader{}
	stdout, _, code := runCreate(t, up, "create", dir)
	require.Equal(t, 0, code)

	assert.Contains(t, up.calls, "flash/1/testid/notes.md")
	assert.Contains(t, up.calls, "flash/1/testid/notes.html")
	// URL suffix should point to .html
	assert.Contains(t, stdout, "notes.html")
}

func TestCreate_NoRender_SkipsRendering(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "notes.md"), []byte("# Hello"), 0600))

	up := &fakeUploader{}
	stdout, _, code := runCreate(t, up, "create", "--no-render", dir)
	require.Equal(t, 0, code)

	assert.Contains(t, up.calls, "flash/1/testid/notes.md")
	assert.NotContains(t, up.calls, "flash/1/testid/notes.html")
	assert.Contains(t, stdout, "notes.md")
}

func TestCreate_CopyDir_WritesFilesToDisk(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(src, "data.txt"), []byte("content"), 0o600))

	up := &fakeUploader{}
	_, stderr, code := runCreate(t, up, "create", "--copy-dir", dst, src)
	require.Equal(t, 0, code)

	// fakeUploader receives no calls; DirUploader handles the write
	assert.Empty(t, up.calls)
	assert.Contains(t, stderr, "local directory")

	got, err := os.ReadFile(filepath.Join(dst, "flash", "1", "testid", "data.txt"))
	require.NoError(t, err)
	assert.Equal(t, "content", string(got))
}

func TestCreate_URL_IndexHtml_NoSuffix(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "index.html"), []byte("<html>"), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "style.css"), []byte("body{}"), 0600))

	up := &fakeUploader{}
	stdout, _, code := runCreate(t, up, "create", dir)
	require.Equal(t, 0, code)

	// index.html in upload → no suffix, URL ends with /
	assert.Contains(t, stdout, "flash/1/testid/")
	assert.NotContains(t, strings.TrimSpace(stdout), "index.html")
	assert.NotContains(t, strings.TrimSpace(stdout), "style.css")
}
