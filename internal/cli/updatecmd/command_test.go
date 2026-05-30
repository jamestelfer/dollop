package updatecmd_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jamestelfer/dollop/internal/cli/updatecmd"
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

// fakeFullUploader implements Uploader, ObjectLister, and ObjectCopier so the
// flash/ touch pass can be exercised. existing seeds objects already under the
// prefix; ListObjects reports those plus everything PutObject wrote this run.
type fakeFullUploader struct {
	existing []string // pre-existing keys under the prefix
	put      []string // keys written this run
	copied   []string // keys touched (self-copied)
	copyErr  error
}

func (f *fakeFullUploader) PutObject(_ context.Context, _, key, _ string, _ io.Reader, _ ...upload.PutOption) error {
	f.put = append(f.put, key)
	return nil
}

func (f *fakeFullUploader) ListObjects(_ context.Context, _, _ string) ([]string, error) {
	seen := map[string]struct{}{}
	var keys []string
	for _, k := range append(append([]string{}, f.existing...), f.put...) {
		if _, ok := seen[k]; ok {
			continue
		}
		seen[k] = struct{}{}
		keys = append(keys, k)
	}
	return keys, nil
}

func (f *fakeFullUploader) CopyObject(_ context.Context, _, key string) error {
	if f.copyErr != nil {
		return f.copyErr
	}
	f.copied = append(f.copied, key)
	return nil
}

func runUpdate(t *testing.T, up upload.Uploader, baseURL string, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	var outBuf, errBuf bytes.Buffer
	cmd := updatecmd.New(up, "test-bucket", baseURL)
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

func TestUpdate_MissingArgs(t *testing.T) {
	_, _, code := runUpdate(t, &fakeUploader{}, "", "update")
	assert.NotEqual(t, 0, code)

	_, _, code = runUpdate(t, &fakeUploader{}, "", "update", "flash/7/abc")
	assert.NotEqual(t, 0, code, "a single positional arg should be rejected")
}

func TestUpdate_FullURL_And_BarePrefix_ResolveSame(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "report.pdf")
	require.NoError(t, os.WriteFile(path, []byte("pdf"), 0600))

	base := "https://drop.example.com"

	upURL := &fakeUploader{}
	stdoutURL, _, codeURL := runUpdate(t, upURL, base, "update", "https://drop.example.com/flash/7/abc123/old.txt", path)
	require.Equal(t, 0, codeURL)

	upBare := &fakeUploader{}
	stdoutBare, _, codeBare := runUpdate(t, upBare, base, "update", "flash/7/abc123", path)
	require.Equal(t, 0, codeBare)

	// Both inputs upload into the same prefix...
	require.Equal(t, []string{"flash/7/abc123/report.pdf"}, upURL.calls)
	assert.Equal(t, upURL.calls, upBare.calls)
	// ...and print the same public URL.
	assert.Equal(t, stdoutURL, stdoutBare)
	assert.Contains(t, stdoutURL, "https://drop.example.com/flash/7/abc123/report.pdf")
}

func TestUpdate_StripsTrailingFilenameAndIndex(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "data.bin")
	require.NoError(t, os.WriteFile(path, []byte("x"), 0600))

	base := "https://drop.example.com"
	cases := []string{
		"https://drop.example.com/flash/1/xyz/index.html",
		"https://drop.example.com/flash/1/xyz/nested/file.txt",
		"flash/1/xyz/",
	}
	for _, ref := range cases {
		t.Run(ref, func(t *testing.T) {
			up := &fakeUploader{}
			_, _, code := runUpdate(t, up, base, "update", ref, path)
			require.Equal(t, 0, code)
			assert.Equal(t, []string{"flash/1/xyz/data.bin"}, up.calls)
		})
	}
}

func TestUpdate_OverwritesMatchingKeys(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "page.html"), []byte("<h1>new</h1>"), 0600))

	up := &fakeUploader{}
	_, _, code := runUpdate(t, up, "https://drop.example.com", "update", "keep/happy-cat", dir)
	require.Equal(t, 0, code)
	// The key matches an existing object's key, so the upload overwrites it.
	assert.Equal(t, []string{"keep/happy-cat/page.html"}, up.calls)
}

func TestUpdate_InvalidReference(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "f.txt")
	require.NoError(t, os.WriteFile(path, []byte("x"), 0600))

	up := &fakeUploader{}
	_, _, code := runUpdate(t, up, "https://drop.example.com", "update", "not-a-known-prefix", path)
	assert.NotEqual(t, 0, code)
	assert.Empty(t, up.calls, "nothing should be uploaded when the reference cannot be resolved")
}

func TestUpdate_NilUploader_FriendlyError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "f.txt")
	require.NoError(t, os.WriteFile(path, []byte("x"), 0600))

	stdout, _, code := runUpdate(t, nil, "https://drop.example.com", "update", "flash/7/abc", path)
	assert.NotEqual(t, 0, code)
	assert.Empty(t, stdout, "no URL should be printed when uploader is unconfigured")
}

func TestUpdate_UploaderError_NonZeroExit(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "f.txt")
	require.NoError(t, os.WriteFile(path, []byte("x"), 0600))

	up := &fakeUploader{err: errors.New("network error")}
	_, stderr, code := runUpdate(t, up, "https://drop.example.com", "update", "flash/7/abc", path)
	assert.NotEqual(t, 0, code)
	assert.True(t, strings.Contains(stderr, "network error") || strings.Contains(stderr, "upload"))
}

func TestUpdate_Render_MarkdownProducesHTMLByDefault(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "notes.md"), []byte("# Hello"), 0600))

	up := &fakeUploader{}
	stdout, _, code := runUpdate(t, up, "https://drop.example.com", "update", "flash/7/abc", dir)
	require.Equal(t, 0, code)

	assert.Contains(t, up.calls, "flash/7/abc/notes.md")
	assert.Contains(t, up.calls, "flash/7/abc/notes.html")
	assert.Contains(t, stdout, "notes.html")
}

func TestUpdate_NoRender_SkipsRendering(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "notes.md"), []byte("# Hello"), 0600))

	up := &fakeUploader{}
	stdout, _, code := runUpdate(t, up, "https://drop.example.com", "update", "--no-render", "flash/7/abc", dir)
	require.Equal(t, 0, code)

	assert.Contains(t, up.calls, "flash/7/abc/notes.md")
	assert.NotContains(t, up.calls, "flash/7/abc/notes.html")
	assert.Contains(t, stdout, "notes.md")
}

func TestUpdate_Index_GeneratesIndex(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "report.pdf"), []byte("pdf"), 0600))

	up := &fakeUploader{}
	stdout, stderr, code := runUpdate(t, up, "https://drop.example.com", "update", "--index", "flash/7/abc", dir)
	require.Equal(t, 0, code)

	assert.Contains(t, up.calls, "flash/7/abc/index.html")
	assert.NotContains(t, stderr, "warning")
	// --index flag → no filename suffix on the URL
	assert.Contains(t, stdout, "flash/7/abc/")
	assert.NotContains(t, strings.TrimSpace(stdout), "report.pdf")
}

func TestUpdate_URLToStdout_OnlyURL(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "f.txt")
	require.NoError(t, os.WriteFile(path, []byte("x"), 0600))

	up := &fakeUploader{}
	stdout, _, code := runUpdate(t, up, "https://drop.example.com", "update", "flash/7/abc", path)
	require.Equal(t, 0, code)

	lines := strings.Split(strings.TrimSpace(stdout), "\n")
	assert.Len(t, lines, 1, "stdout should contain only the URL")
	assert.True(t, strings.HasPrefix(lines[0], "http"), "stdout should be a URL, got: %q", lines[0])
}

func TestUpdate_Flash_Directory_TouchesUnreferencedPreExistingObjects(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "page.html"), []byte("<h1>new</h1>"), 0o600))

	up := &fakeFullUploader{existing: []string{"flash/7/abc/old.pdf"}}
	_, stderr, code := runUpdate(t, up, "https://drop.example.com", "update", "flash/7/abc", dir)
	require.Equal(t, 0, code)

	// The unreferenced pre-existing object is touched; the freshly-written one is not.
	assert.Equal(t, []string{"flash/7/abc/old.pdf"}, up.copied)
	assert.NotContains(t, up.copied, "flash/7/abc/page.html")
	assert.Contains(t, stderr, "flash/7/abc/old.pdf")
}

func TestUpdate_Keep_Directory_SkipsTouchPass(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "page.html"), []byte("<h1>new</h1>"), 0o600))

	up := &fakeFullUploader{existing: []string{"keep/happy-cat/old.pdf"}}
	_, _, code := runUpdate(t, up, "https://drop.example.com", "update", "keep/happy-cat", dir)
	require.Equal(t, 0, code)

	assert.Empty(t, up.copied, "keep/ prefixes have no expiry clock, so no touch pass runs")
}

func TestUpdate_Flash_SingleFile_SkipsTouchPass(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "report.pdf")
	require.NoError(t, os.WriteFile(path, []byte("pdf"), 0o600))

	up := &fakeFullUploader{existing: []string{"flash/7/abc/old.pdf"}}
	_, _, code := runUpdate(t, up, "https://drop.example.com", "update", "flash/7/abc", path)
	require.Equal(t, 0, code)

	assert.Empty(t, up.copied, "single-file updates do not run the touch pass")
}

func TestUpdate_Flash_TouchFailure_NonZeroExit(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "page.html"), []byte("x"), 0o600))

	up := &fakeFullUploader{existing: []string{"flash/7/abc/old.pdf"}, copyErr: errors.New("touch boom")}
	_, _, code := runUpdate(t, up, "https://drop.example.com", "update", "flash/7/abc", dir)
	assert.NotEqual(t, 0, code, "a touch failure exits non-zero")
}

func TestUpdate_CopyDir_WritesFilesToDisk(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(src, "data.txt"), []byte("content"), 0o600))

	// --copy-dir routes the upload through DirUploader; even a nil R2 uploader works.
	up := &fakeUploader{}
	_, stderr, code := runUpdate(t, up, "https://drop.example.com", "update", "--copy-dir", dst, "flash/7/abc", src)
	require.Equal(t, 0, code)

	assert.Empty(t, up.calls, "DirUploader handles the write, not the R2 uploader")
	assert.Contains(t, stderr, "local directory")

	got, err := os.ReadFile(filepath.Join(dst, "flash", "7", "abc", "data.txt"))
	require.NoError(t, err)
	assert.Equal(t, "content", string(got))
}
