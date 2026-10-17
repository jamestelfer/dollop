package upload_test

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
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
	res, err := upload.UploadFiles(context.Background(), up, "my-bucket", "dollop/1/abc", path, upload.IndexNone, &stderr)
	files := res.SourceRelPaths
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
	res, err := upload.UploadFiles(context.Background(), up, "bucket", "keep/friendly-cat", dir, upload.IndexNone, &stderr)
	files := res.SourceRelPaths
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

	// nested files show their path relative to the upload root, not just the basename
	assert.Contains(t, stderr.String(), "uploading [sub/data.json]")
}

func TestUploadFiles_UploaderError(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "file.bin"), []byte("x"), 0600))

	up := &fakeUploader{err: assert.AnError}
	var stderr bytes.Buffer
	_, err := upload.UploadFiles(context.Background(), up, "b", "p", filepath.Join(dir, "file.bin"), upload.IndexNone, &stderr)
	assert.Error(t, err)
	assert.Contains(t, stderr.String(), "uploading [file.bin]")
	assert.Contains(t, stderr.String(), "...failed")
}

func TestUploadFiles_Index_NoRender_RedirectsToGeneratedMarkdown(t *testing.T) {
	up, res, stderr := uploadTree(t, map[string]string{"report.pdf": "pdf", "notes.txt": "notes"}, upload.IndexGenerate)

	// with no renderer the listing is published as raw markdown...
	md := bodyOf(t, up, "flash/1/abc/index.md")
	assert.Contains(t, md, "[notes.txt](notes.txt)")
	assert.Contains(t, md, "[report.pdf](report.pdf)")

	// ...and index.html sends the browser to it
	assert.Contains(t, bodyOf(t, up, "flash/1/abc/index.html"), `location.replace("index.md")`)

	assert.NotContains(t, stderr, "warning")
	assert.Empty(t, res.Entry)
	// generated pages are not in the returned file list
	assert.ElementsMatch(t, []string{"notes.txt", "report.pdf"}, res.SourceRelPaths)
}

func TestUploadFiles_Index_SupportingFilesCollapsed(t *testing.T) {
	up, _, _ := uploadTree(t, map[string]string{"notes.md": "# Hello"}, upload.IndexGenerate, upload.WithRenderer(render.NewMarkdownRenderer()))

	s := bodyOf(t, up, "flash/1/abc/index.html")
	// the markdown is folded into its rendered page as the source link
	assert.Equal(t, 1, strings.Count(s, `<a href="notes.md">`), "the markdown must only appear as the source link, not as its own item")

	// shared assets are listed in a collapsed section, not the main list
	assert.Regexp(t, `(?s)<details>\s*<summary>Supporting files</summary>.*href=".dollop/github-markdown.css".*</details>`, s)
	assert.NotContains(t, s, "<details open")
}

func TestUploadFiles_Index_SkipsIfExists(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "index.html"), []byte("<h1>mine</h1>"), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "data.csv"), []byte("a,b"), 0600))

	up := &fakeUploader{}
	var stderr bytes.Buffer
	res, err := upload.UploadFiles(context.Background(), up, "bucket", "flash/1/xyz", dir, upload.IndexGenerate, &stderr)
	files := res.SourceRelPaths
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
	res, err := upload.UploadFiles(context.Background(), up, "bucket", "flash/1/xyz", path, upload.IndexGenerate, &stderr)
	files := res.SourceRelPaths
	require.NoError(t, err)

	require.Len(t, up.calls, 1) // only the file itself, no generated index
	assert.Contains(t, stderr.String(), "warning")
	assert.Equal(t, []string{"index.html"}, files)
}

func TestUploadFiles_Render_MermaidEngineNotUploadedPerPrefix(t *testing.T) {
	dir := t.TempDir()
	mermaidMD := "```mermaid\ngraph TD\n    A --> B\n```\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.md"), []byte(mermaidMD), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "b.md"), []byte("# Plain"), 0o600))

	up := &fakeUploader{}
	var stderr bytes.Buffer
	_, err := upload.UploadFiles(context.Background(), up, "bucket", "flash/1/abc", dir, upload.IndexNone, &stderr, upload.WithRenderer(render.NewMarkdownRenderer()))
	require.NoError(t, err)

	// The mermaid engine is now shared under deps/mermaid/<v>/ and referenced by
	// relative path; it must never be uploaded into the publish prefix.
	for _, c := range up.calls {
		assert.NotContains(t, c.key, "mermaid", "no mermaid engine should be written under the prefix")
	}
}

func TestUploadFiles_Render_SharedAssetsUploadedOnce(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.md"), []byte("# A"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "b.md"), []byte("# B"), 0o600))

	up := &fakeUploader{}
	var stderr bytes.Buffer
	res, err := upload.UploadFiles(context.Background(), up, "bucket", "flash/1/abc", dir, upload.IndexNone, &stderr, upload.WithRenderer(render.NewMarkdownRenderer()))
	files := res.SourceRelPaths
	require.NoError(t, err)

	// count CSS uploads
	cssCount := 0
	for _, c := range up.calls {
		if c.key == "flash/1/abc/.dollop/github-markdown.css" {
			cssCount++
		}
	}
	assert.Equal(t, 1, cssCount, "shared CSS should be uploaded exactly once")

	// shared assets must not appear in returned relPaths
	assert.NotContains(t, files, "github-markdown.css")
	assert.NotContains(t, files, "highlight-github.css")
}

func TestUploadFiles_WrittenKeys_CoverSourcesAssetsAndIndex(t *testing.T) {
	dir := t.TempDir()
	// A markdown source produces a rendered .html plus shared CSS assets; --index
	// adds a generated index.html. WrittenKeys must report every key written.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "notes.md"), []byte("# Hi"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "data.csv"), []byte("a,b"), 0o600))

	up := &fakeUploader{}
	var stderr bytes.Buffer
	res, err := upload.UploadFiles(context.Background(), up, "bucket", "flash/7/abc", dir, upload.IndexGenerate, &stderr, upload.WithRenderer(render.NewMarkdownRenderer()))
	require.NoError(t, err)

	// Every PutObject key — sources, rendered HTML, shared assets, generated
	// index — must be reported, with no extras and no omissions.
	putKeys := make([]string, len(up.calls))
	for i, c := range up.calls {
		putKeys[i] = c.key
	}
	assert.ElementsMatch(t, putKeys, res.WrittenKeys,
		"WrittenKeys must be exactly the set of object keys written this run")
	assert.Contains(t, res.WrittenKeys, "flash/7/abc/index.html", "generated index is reported")
	assert.Contains(t, res.WrittenKeys, "flash/7/abc/notes.html", "rendered source is reported")
	assert.Contains(t, res.WrittenKeys, "flash/7/abc/.dollop/github-markdown.css", "shared asset is reported")
}

func TestUploadFiles_Render_IndexMdSetsHasIndex(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "index.md"), []byte("# Home"), 0600))

	up := &fakeUploader{}
	var stderr bytes.Buffer
	// generateIndex=true so we can observe the hasIndex collision warning
	res, err := upload.UploadFiles(context.Background(), up, "bucket", "flash/1/abc", dir, upload.IndexGenerate, &stderr, upload.WithRenderer(render.NewMarkdownRenderer()))
	files := res.SourceRelPaths
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
	res, err := upload.UploadFiles(context.Background(), up, "bucket", "flash/1/abc", dir, upload.IndexNone, &stderr, upload.WithRenderer(render.NewMarkdownRenderer()))
	files := res.SourceRelPaths
	require.NoError(t, err)

	keys := make([]string, len(up.calls))
	for i, c := range up.calls {
		keys[i] = c.key
	}
	assert.Contains(t, keys, "flash/1/abc/notes.md")
	assert.Contains(t, keys, "flash/1/abc/notes.html")
	assert.ElementsMatch(t, []string{"notes.md", "notes.html"}, files)

	// the rendered file's size is unknown up front, so it is reported after the
	// ellipsis once the body is produced (e.g. "rendering [notes.html]...1.2 KB")
	assert.Regexp(t, `rendering \[notes\.html\]\.\.\.[0-9.]+ ?[KMG]?B`, stderr.String())
	assert.NotContains(t, stderr.String(), "rendering [notes.html]...done")
}

func TestUploadFiles_Render_NoRender_SkipsHTML(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "notes.md"), []byte("# Hello"), 0600))

	up := &fakeUploader{}
	var stderr bytes.Buffer
	res, err := upload.UploadFiles(context.Background(), up, "bucket", "flash/1/abc", dir, upload.IndexNone, &stderr)
	files := res.SourceRelPaths
	require.NoError(t, err)

	keys := make([]string, len(up.calls))
	for i, c := range up.calls {
		keys[i] = c.key
	}
	assert.Contains(t, keys, "flash/1/abc/notes.md")
	assert.NotContains(t, keys, "flash/1/abc/notes.html")
	assert.Equal(t, []string{"notes.md"}, files)
}

// symlinkFixture builds an upload directory beside a sibling "outside"
// directory holding a secret, so links can point out of the upload root.
func symlinkFixture(t *testing.T) (dir, outside string) {
	t.Helper()
	base := t.TempDir()
	dir = filepath.Join(base, "upload")
	outside = filepath.Join(base, "outside")
	require.NoError(t, os.MkdirAll(dir, 0o700))
	require.NoError(t, os.MkdirAll(outside, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("private"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(outside, "secret.md"), []byte("# Private"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "keep.txt"), []byte("public"), 0o600))
	return dir, outside
}

func uploadedKeys(up *fakeUploader) []string {
	keys := make([]string, len(up.calls))
	for i, c := range up.calls {
		keys[i] = c.key
	}
	return keys
}

func TestUploadFiles_Directory_SkipsSymlinkOutsideRoot(t *testing.T) {
	dir, outside := symlinkFixture(t)
	require.NoError(t, os.Symlink(filepath.Join(outside, "secret.txt"), filepath.Join(dir, "notes.txt")))

	up := &fakeUploader{}
	var stderr bytes.Buffer
	res, err := upload.UploadFiles(context.Background(), up, "bucket", "flash/1/abc", dir, upload.IndexNone, &stderr)
	require.NoError(t, err)

	assert.Equal(t, []string{"flash/1/abc/keep.txt"}, uploadedKeys(up))
	assert.Equal(t, []string{"keep.txt"}, res.SourceRelPaths)
	assert.Contains(t, stderr.String(), "warning: skipping notes.txt")
	for _, c := range up.calls {
		assert.NotContains(t, string(c.body), "private")
	}
}

func TestUploadFiles_Directory_FollowsSymlinkInsideRoot(t *testing.T) {
	dir, _ := symlinkFixture(t)
	require.NoError(t, os.Symlink("keep.txt", filepath.Join(dir, "alias.txt")))

	up := &fakeUploader{}
	var stderr bytes.Buffer
	_, err := upload.UploadFiles(context.Background(), up, "bucket", "flash/1/abc", dir, upload.IndexNone, &stderr)
	require.NoError(t, err)

	bodies := map[string]string{}
	for _, c := range up.calls {
		bodies[c.key] = string(c.body)
	}
	assert.Equal(t, map[string]string{
		"flash/1/abc/keep.txt":  "public",
		"flash/1/abc/alias.txt": "public",
	}, bodies)
	assert.NotContains(t, stderr.String(), "warning")
}

func TestUploadFiles_Directory_OmitsSymlinkToDirectory(t *testing.T) {
	dir, outside := symlinkFixture(t)
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "sub"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "sub", "data.json"), []byte("{}"), 0o600))
	require.NoError(t, os.Symlink("sub", filepath.Join(dir, "sublink")))
	require.NoError(t, os.Symlink(outside, filepath.Join(dir, "outlink")))

	up := &fakeUploader{}
	var stderr bytes.Buffer
	res, err := upload.UploadFiles(context.Background(), up, "bucket", "flash/1/abc", dir, upload.IndexNone, &stderr)
	require.NoError(t, err)

	assert.ElementsMatch(t, []string{"keep.txt", "sub/data.json"}, res.SourceRelPaths)
}

func TestUploadFiles_Render_SkipsMarkdownSymlinkOutsideRoot(t *testing.T) {
	dir, outside := symlinkFixture(t)
	require.NoError(t, os.Symlink(filepath.Join(outside, "secret.md"), filepath.Join(dir, "notes.md")))

	up := &fakeUploader{}
	var stderr bytes.Buffer
	_, err := upload.UploadFiles(context.Background(), up, "bucket", "flash/1/abc", dir, upload.IndexNone, &stderr, upload.WithRenderer(render.NewMarkdownRenderer()))
	require.NoError(t, err)

	keys := uploadedKeys(up)
	assert.NotContains(t, keys, "flash/1/abc/notes.md")
	assert.NotContains(t, keys, "flash/1/abc/notes.html")
	assert.Contains(t, keys, "flash/1/abc/keep.txt")
	assert.Contains(t, stderr.String(), "warning: skipping notes.md")
}

func TestUploadFiles_SingleFile_FollowsSymlink(t *testing.T) {
	dir, outside := symlinkFixture(t)
	link := filepath.Join(dir, "some-link.md")
	require.NoError(t, os.Symlink(filepath.Join(outside, "secret.md"), link))

	up := &fakeUploader{}
	var stderr bytes.Buffer
	res, err := upload.UploadFiles(context.Background(), up, "bucket", "flash/1/abc", link, upload.IndexNone, &stderr, upload.WithRenderer(render.NewMarkdownRenderer()))
	require.NoError(t, err)

	assert.ElementsMatch(t, []string{"some-link.md", "some-link.html"}, res.SourceRelPaths)
	for _, c := range up.calls {
		if c.key == "flash/1/abc/some-link.md" {
			assert.Equal(t, "# Private", string(c.body))
		}
	}
}

func TestUploadFiles_NilUploader_ReturnsError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "f.txt")
	require.NoError(t, os.WriteFile(path, []byte("x"), 0600))

	var stderr bytes.Buffer
	// A nil Uploader must produce an error, not a nil pointer panic.
	_, err := upload.UploadFiles(context.Background(), nil, "b", "p", path, upload.IndexNone, &stderr)
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

// uploadTree writes files (slash-separated relative path → content) to a temp
// directory, uploads it to flash/1/abc, and returns the uploader, the result
// and what was written to stderr.
func uploadTree(t *testing.T, files map[string]string, index upload.IndexMode, opts ...upload.UploadOption) (*fakeUploader, *upload.UploadResult, string) {
	t.Helper()
	dir := t.TempDir()
	for rel, content := range files {
		path := filepath.Join(dir, filepath.FromSlash(rel))
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
		require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	}
	up := &fakeUploader{}
	var stderr bytes.Buffer
	res, err := upload.UploadFiles(context.Background(), up, "bucket", "flash/1/abc", dir, index, &stderr, opts...)
	require.NoError(t, err)
	return up, res, stderr.String()
}

// keysOf returns the keys uploaded, in order.
func keysOf(up *fakeUploader) []string {
	keys := make([]string, len(up.calls))
	for i, c := range up.calls {
		keys[i] = c.key
	}
	return keys
}

// bodyOf returns the uploaded body for key, failing the test when key was not
// uploaded.
func bodyOf(t *testing.T, up *fakeUploader, key string) string {
	t.Helper()
	for _, c := range up.calls {
		if c.key == key {
			return string(c.body)
		}
	}
	require.Failf(t, "key not uploaded", "%s was not uploaded", key)
	return ""
}

func TestUploadFiles_Readme_RedirectsIndexToRenderedReadme(t *testing.T) {
	up, res, _ := uploadTree(t, map[string]string{"README.md": "# Project", "notes.txt": "notes"}, upload.IndexNone, upload.WithRenderer(render.NewMarkdownRenderer()))

	s := bodyOf(t, up, "flash/1/abc/index.html")
	assert.Contains(t, s, `location.replace("README.html")`)
	assert.Contains(t, s, `<noscript><meta http-equiv="refresh" content="0; url=README.html"></noscript>`)

	// the printed URL goes straight to the rendered README, not via the redirect
	assert.Equal(t, "README.html", res.Entry)
	// the generated redirect is not one of the uploaded sources
	assert.ElementsMatch(t, []string{"README.md", "README.html", "notes.txt"}, res.SourceRelPaths)
	assert.Contains(t, res.WrittenKeys, "flash/1/abc/index.html")
}

func TestUploadFiles_Readme_NoRender_RedirectsToRawMarkdown(t *testing.T) {
	up, res, _ := uploadTree(t, map[string]string{"readme.md": "# Project"}, upload.IndexNone)

	assert.Contains(t, bodyOf(t, up, "flash/1/abc/index.html"), `location.replace("readme.md")`)
	assert.Equal(t, "readme.md", res.Entry)
}

func TestUploadFiles_Readme_SingleFileGetsRedirect(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "Readme.markdown")
	require.NoError(t, os.WriteFile(path, []byte("# Project"), 0o600))

	up := &fakeUploader{}
	var stderr bytes.Buffer
	res, err := upload.UploadFiles(context.Background(), up, "bucket", "flash/1/abc", path, upload.IndexNone, &stderr, upload.WithRenderer(render.NewMarkdownRenderer()))
	require.NoError(t, err)

	assert.Contains(t, bodyOf(t, up, "flash/1/abc/index.html"), `location.replace("Readme.html")`)
	assert.Equal(t, "Readme.html", res.Entry)
}

func TestUploadFiles_Readme_WinsOverIndexFlag(t *testing.T) {
	up, res, stderr := uploadTree(t, map[string]string{"README.md": "# Project", "data.csv": "a,b"}, upload.IndexGenerate, upload.WithRenderer(render.NewMarkdownRenderer()))

	s := bodyOf(t, up, "flash/1/abc/index.html")
	assert.Contains(t, s, `location.replace("README.html")`)
	assert.NotContains(t, s, "data.csv", "the index is the redirect, not a listing")
	assert.Contains(t, stderr, "warning: README.md present")
	assert.Equal(t, "README.html", res.Entry)
}

func TestUploadFiles_Readme_ExistingIndexIsKept(t *testing.T) {
	up, res, _ := uploadTree(t, map[string]string{"README.md": "# Project", "index.html": "<h1>mine</h1>"}, upload.IndexNone, upload.WithRenderer(render.NewMarkdownRenderer()))

	assert.Equal(t, "<h1>mine</h1>", bodyOf(t, up, "flash/1/abc/index.html"))
	assert.Empty(t, res.Entry)
}

func TestUploadFiles_Readme_NestedReadmeIgnored(t *testing.T) {
	up, res, _ := uploadTree(t, map[string]string{"docs/README.md": "# Docs"}, upload.IndexNone, upload.WithRenderer(render.NewMarkdownRenderer()))

	assert.NotContains(t, keysOf(up), "flash/1/abc/index.html", "only a README at the upload root gets a redirect")
	assert.Equal(t, "docs/README.html", res.Entry)
}

func TestUploadFiles_Index_RendersGeneratedMarkdownWithTheme(t *testing.T) {
	up, res, stderr := uploadTree(t, map[string]string{"notes.md": "# Hello", "data.csv": "a,b"}, upload.IndexGenerate, upload.WithRenderer(render.NewMarkdownRenderer()))

	md := bodyOf(t, up, "flash/1/abc/index.md")
	assert.Contains(t, md, `<p class="index-meta">2 files · 1 page · expires <time datetime="`)
	assert.Contains(t, md, "[notes.html](notes.html)")
	assert.Contains(t, md, "[::md-icon::](notes.md)")
	assert.Contains(t, md, "[data.csv](data.csv)")

	html := bodyOf(t, up, "flash/1/abc/index.html")
	assert.Contains(t, html, "dollop-theme-tide.css", "the index is a themed rendered page")
	assert.Contains(t, html, `href="notes.html"`)
	assert.Contains(t, html, `href="notes.md"`, "source links are not rewritten to .html")
	assert.Contains(t, html, `<use href="#dollop-md-icon"`)
	assert.Equal(t, 1, strings.Count(html, `<symbol id="dollop-md-icon"`))

	assert.Empty(t, res.Entry, "the URL points at the generated index")
	assert.NotContains(t, res.SourceRelPaths, "index.md", "generated pages are not reported as sources")
	assert.NotContains(t, stderr, "warning")
}

func TestUploadFiles_Index_TitledByUploadDirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "q3_review")
	require.NoError(t, os.MkdirAll(dir, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.txt"), []byte("a"), 0o600))

	up := &fakeUploader{}
	var stderr bytes.Buffer
	_, err := upload.UploadFiles(context.Background(), up, "bucket", "keep/emu.abc.def.ghi", dir, upload.IndexGenerate, &stderr, upload.WithRenderer(render.NewMarkdownRenderer()))
	require.NoError(t, err)

	html := bodyOf(t, up, "keep/emu.abc.def.ghi/index.html")
	assert.Contains(t, html, "<title>q3_review</title>", "the page title comes from the frontmatter, unescaped")
	assert.Contains(t, html, `<p class="index-meta">1 file · permanent</p>`)
}

func TestUploadFiles_Index_RendersDirectoryTree(t *testing.T) {
	up, _, _ := uploadTree(t, map[string]string{
		"top.txt":           "t",
		"docs/guide.md":     "# Guide",
		"docs/img/logo.png": "png",
	}, upload.IndexGenerate, upload.WithRenderer(render.NewMarkdownRenderer()))

	html := bodyOf(t, up, "flash/1/abc/index.html")
	// each directory is a <details> inside its own list item, holding the
	// nested list as rendered markdown rather than raw text.
	assert.Regexp(t, `(?s)<div class="file-index">\s*<ul>\s*<li>\s*<p><a href="top.txt">top.txt</a></p>\s*</li>\s*`+
		`<li>\s*<details open="">\s*<summary>docs/ <small>2 files</small></summary>\s*<ul>\s*`+
		`<li>\s*<p><a href="docs/guide.html">guide.html</a> <a href="docs/guide.md"><svg[^>]*>.*?</svg></a></p>\s*</li>\s*`+
		`<li>\s*<details>\s*<summary>img/ <small>1 file</small></summary>\s*<ul>\s*`+
		`<li><a href="docs/img/logo.png">logo.png</a></li>\s*</ul>\s*</details>\s*</li>\s*`+
		`</ul>\s*</details>\s*</li>\s*</ul>\s*</div>`, html)
}

func TestUploadFiles_Index_NoRender_ExistingIndexMarkdownSkipsGeneration(t *testing.T) {
	up, _, stderr := uploadTree(t, map[string]string{"index.md": "# Mine", "README.md": "# Project"}, upload.IndexGenerate)

	assert.Equal(t, "# Mine", bodyOf(t, up, "flash/1/abc/index.md"), "the user's index.md is not replaced")
	assert.NotContains(t, keysOf(up), "flash/1/abc/index.html", "neither a listing nor a README redirect is generated")
	assert.Contains(t, stderr, "warning: index.md already present")
}

func TestUploadFiles_NoRender_ExistingIndexMarkdownIsTheEntry(t *testing.T) {
	_, res, _ := uploadTree(t, map[string]string{"index.md": "# Mine", "a.txt": "a"}, upload.IndexNone)

	assert.Equal(t, "index.md", res.Entry, "the user's index page is the entry, not the first file alphabetically")
}

func TestUploadFiles_NoIndex_EntryIsNotInAssetDir(t *testing.T) {
	_, res, _ := uploadTree(t, map[string]string{".dollop/dollop-dark.svg": "<svg/>", "notes.html": "<p>hi</p>"}, upload.IndexNone)

	assert.Equal(t, "notes.html", res.Entry)
}

func TestUploadFiles_Index_UploadsAssetsTheIndexPageNeeds(t *testing.T) {
	up, res, _ := uploadTree(t, map[string]string{"data.csv": "a,b"}, upload.IndexGenerate, upload.WithRenderer(render.NewMarkdownRenderer()))

	// the rendered index needs the page stylesheets even though the upload
	// itself has no markdown
	assert.Contains(t, res.WrittenKeys, "flash/1/abc/.dollop/github-markdown.css")
	assert.Contains(t, res.WrittenKeys, "flash/1/abc/.dollop/dollop-theme-tide.css")
	assert.Equal(t, 1, strings.Count(strings.Join(res.WrittenKeys, "\n"), "github-markdown.css"), "each asset is uploaded once")

	// they serve the index page, so they are not listed as the upload's files
	assert.NotContains(t, bodyOf(t, up, "flash/1/abc/index.md"), "Supporting files")
}

func TestUploadFiles_Index_SharedAssetsUploadedOnce(t *testing.T) {
	_, res, _ := uploadTree(t, map[string]string{"notes.md": "# Hi"}, upload.IndexGenerate, upload.WithRenderer(render.NewMarkdownRenderer()))

	seen := map[string]int{}
	for _, k := range res.WrittenKeys {
		seen[k]++
	}
	for k, n := range seen {
		assert.Equal(t, 1, n, "%s written more than once", k)
	}
}

func TestUploadFiles_NoRender_ExistingIndexMarkdownExtensionIsTheEntry(t *testing.T) {
	up, res, stderr := uploadTree(t, map[string]string{"index.markdown": "# Mine", "a.txt": "a"}, upload.IndexGenerate)

	assert.NotContains(t, keysOf(up), "flash/1/abc/index.html", "no listing is generated over the user's index page")
	assert.NotContains(t, keysOf(up), "flash/1/abc/index.md")
	assert.Equal(t, "index.markdown", res.Entry)
	assert.Contains(t, stderr, "warning: index.markdown already present")
}

func TestUploadFiles_Published_IndexBlocksReadmeRedirect(t *testing.T) {
	up, res, stderr := uploadTree(t, map[string]string{"README.md": "# Project"}, upload.IndexNone,
		upload.WithRenderer(render.NewMarkdownRenderer()), upload.WithPublished([]string{"index.html", "old.txt"}))

	assert.NotContains(t, keysOf(up), "flash/1/abc/index.html", "the published index.html is not replaced")
	assert.Contains(t, stderr, "warning: index.html already published, skipping README redirect")
	assert.Empty(t, res.Entry, "the published index.html serves the root")
}

func TestUploadFiles_Published_IndexBlocksIndexGeneration(t *testing.T) {
	up, res, stderr := uploadTree(t, map[string]string{"a.txt": "a"}, upload.IndexGenerate,
		upload.WithRenderer(render.NewMarkdownRenderer()), upload.WithPublished([]string{"index.md"}))

	assert.NotContains(t, keysOf(up), "flash/1/abc/index.md", "the published index.md is not replaced")
	assert.NotContains(t, keysOf(up), "flash/1/abc/index.html")
	assert.Contains(t, stderr, "warning: index.md already published, skipping index generation (use --index-overwrite to replace it)")
	assert.Equal(t, "index.md", res.Entry)
}

func TestUploadFiles_Published_LocalIndexStillUploaded(t *testing.T) {
	up, res, stderr := uploadTree(t, map[string]string{"index.html": "<h1>new</h1>"}, upload.IndexNone, upload.WithPublished([]string{"index.html"}))

	assert.Equal(t, "<h1>new</h1>", bodyOf(t, up, "flash/1/abc/index.html"), "an index page in the upload itself replaces the published one")
	assert.NotContains(t, stderr, "warning")
	assert.Empty(t, res.Entry)
}

func TestUploadFiles_Published_ListingCoversPublishedAndLocalFiles(t *testing.T) {
	up, _, _ := uploadTree(t, map[string]string{"new.txt": "n"}, upload.IndexGenerate,
		upload.WithPublished([]string{"old.txt", "docs/guide.md"}))

	md := bodyOf(t, up, "flash/1/abc/index.md")
	assert.Contains(t, md, "[new.txt](new.txt)")
	assert.Contains(t, md, "[old.txt](old.txt)", "files published by an earlier run are listed")
	assert.Contains(t, md, "[guide.md](docs/guide.md)")
}

func TestUploadFiles_Published_AssetsListedAsSupportingFiles(t *testing.T) {
	up, _, _ := uploadTree(t, map[string]string{"new.txt": "n"}, upload.IndexGenerate,
		upload.WithPublished([]string{"old.md", "old.html", "github-markdown.css", "dollop-theme-retired.css", "d2/0123456789ab.svg"}))

	md := bodyOf(t, up, "flash/1/abc/index.md")
	main, supporting, ok := strings.Cut(md, "Supporting files")
	require.True(t, ok, "published assets are listed as supporting files")
	assert.Contains(t, main, "[old.html](old.html)")
	for _, asset := range []string{"github-markdown.css", "dollop-theme-retired.css", "d2/0123456789ab.svg"} {
		assert.NotContains(t, main, asset, "%s is not a primary entry", asset)
		assert.Contains(t, supporting, "("+asset+")")
	}
}

func TestUploadFiles_Published_ReadmeGetsRedirect(t *testing.T) {
	up, res, _ := uploadTree(t, map[string]string{"a.txt": "a"}, upload.IndexNone,
		upload.WithRenderer(render.NewMarkdownRenderer()), upload.WithPublished([]string{"README.md", "README.html"}))

	assert.Contains(t, bodyOf(t, up, "flash/1/abc/index.html"), `location.replace("README.html")`)
	assert.Equal(t, "README.html", res.Entry)
}

func TestUploadFiles_Overwrite_ReplacesPublishedIndex(t *testing.T) {
	up, res, stderr := uploadTree(t, map[string]string{"new.txt": "n"}, upload.IndexOverwrite,
		upload.WithRenderer(render.NewMarkdownRenderer()), upload.WithPublished([]string{"index.md", "index.html", "old.txt"}))

	md := bodyOf(t, up, "flash/1/abc/index.md")
	assert.Contains(t, md, "[new.txt](new.txt)")
	assert.Contains(t, md, "[old.txt](old.txt)")
	assert.NotContains(t, md, "(index.", "the index being replaced is not listed")
	assert.Contains(t, bodyOf(t, up, "flash/1/abc/index.html"), "old.txt", "the rendered listing replaces the published index.html")
	assert.Empty(t, res.Entry)
	assert.NotContains(t, stderr, "warning")
}

func TestUploadFiles_Overwrite_PublishedReadmeWinsOverListing(t *testing.T) {
	up, res, stderr := uploadTree(t, map[string]string{"new.txt": "n"}, upload.IndexOverwrite,
		upload.WithRenderer(render.NewMarkdownRenderer()), upload.WithPublished([]string{"index.html", "README.md", "README.html"}))

	assert.Contains(t, bodyOf(t, up, "flash/1/abc/index.html"), `location.replace("README.html")`)
	assert.NotContains(t, keysOf(up), "flash/1/abc/index.md", "no listing is generated")
	assert.Contains(t, stderr, "warning: README.md present")
	assert.Equal(t, "README.html", res.Entry)
}

func TestUploadFiles_Overwrite_LocalIndexWins(t *testing.T) {
	up, res, stderr := uploadTree(t, map[string]string{"index.html": "<h1>mine</h1>", "a.txt": "a"}, upload.IndexOverwrite,
		upload.WithPublished([]string{"index.html"}))

	assert.Equal(t, "<h1>mine</h1>", bodyOf(t, up, "flash/1/abc/index.html"))
	assert.NotContains(t, keysOf(up), "flash/1/abc/index.md")
	assert.Contains(t, stderr, "warning: index.html already present, skipping index generation")
	assert.Empty(t, res.Entry)
}

func TestUploadFiles_Published_ListingIsSorted(t *testing.T) {
	up, _, _ := uploadTree(t, map[string]string{"b.txt": "b"}, upload.IndexGenerate,
		upload.WithPublished([]string{"c.txt", "a.txt"}))

	md := bodyOf(t, up, "flash/1/abc/index.md")
	assert.Regexp(t, `(?s)\(a\.txt\).*\(b\.txt\).*\(c\.txt\)`, md, "local and published files are listed in one sorted order")
}

// markerIn returns the dollop provenance marker in the first 512 bytes of
// html, or "" when there is none.
func markerIn(html string) string {
	head := html[:min(len(html), 512)]
	_, rest, ok := strings.Cut(head, `<meta name="dollop-generated" content="`)
	if !ok {
		return ""
	}
	kind, _, ok := strings.Cut(rest, `">`)
	if !ok {
		return ""
	}
	return kind
}

func TestUploadFiles_GeneratedIndexIsMarked(t *testing.T) {
	longTitle := strings.Repeat("very-long-directory-name-", 9) // near the 255-byte name limit
	for name, tc := range map[string]struct {
		files map[string]string
		opts  []upload.UploadOption
		want  string
	}{
		"rendered listing":  {map[string]string{"a.txt": "a"}, []upload.UploadOption{upload.WithRenderer(render.NewMarkdownRenderer())}, "listing"},
		"no-render listing": {map[string]string{"a.txt": "a"}, nil, "listing"},
		"readme redirect":   {map[string]string{"README.md": "# R"}, []upload.UploadOption{upload.WithRenderer(render.NewMarkdownRenderer())}, "readme-redirect"},
	} {
		t.Run(name, func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), longTitle)
			require.NoError(t, os.MkdirAll(dir, 0o700))
			for rel, content := range tc.files {
				require.NoError(t, os.WriteFile(filepath.Join(dir, rel), []byte(content), 0o600))
			}
			up := &fakeUploader{}
			_, err := upload.UploadFiles(context.Background(), up, "bucket", "flash/1/abc", dir, upload.IndexGenerate, io.Discard, tc.opts...)
			require.NoError(t, err)

			html := bodyOf(t, up, "flash/1/abc/index.html")
			assert.Equal(t, tc.want, markerIn(html), "the marker is within the first 512 bytes")
			assert.Less(t, strings.Index(html, "dollop-generated"), strings.Index(html, "<title>"), "the marker precedes the title")
		})
	}
}

func TestUploadFiles_UserIndexIsNotMarked(t *testing.T) {
	up, _, _ := uploadTree(t, map[string]string{"index.md": "---\ntitle: Mine\n---\n# Mine"}, upload.IndexNone,
		upload.WithRenderer(render.NewMarkdownRenderer()))

	assert.NotContains(t, bodyOf(t, up, "flash/1/abc/index.html"), "dollop-generated")
}

func TestUploadFiles_GeneratedListing_RefreshedWithoutFlag(t *testing.T) {
	up, res, stderr := uploadTree(t, map[string]string{"new.txt": "n"}, upload.IndexNone,
		upload.WithRenderer(render.NewMarkdownRenderer()),
		upload.WithPublished([]string{"index.html", "index.md", "old.txt"}),
		upload.WithPublishedIndex(upload.PublishedIndex{Kind: upload.IndexKindListing, Title: "Original Title"}))

	md := bodyOf(t, up, "flash/1/abc/index.md")
	assert.Contains(t, md, "[new.txt](new.txt)")
	assert.Contains(t, md, "[old.txt](old.txt)")
	assert.NotContains(t, md, "(index.", "the listing being replaced is not listed")
	assert.Contains(t, md, "# Original Title", "the published listing's title is kept")
	assert.Equal(t, "listing", markerIn(bodyOf(t, up, "flash/1/abc/index.html")))
	assert.Empty(t, res.Entry)
	assert.NotContains(t, stderr, "warning")
}

func TestUploadFiles_GeneratedReadmeRedirect_Rewritten(t *testing.T) {
	up, res, stderr := uploadTree(t, map[string]string{"new.txt": "n"}, upload.IndexNone,
		upload.WithRenderer(render.NewMarkdownRenderer()),
		upload.WithPublished([]string{"index.html", "README.md", "README.html"}),
		upload.WithPublishedIndex(upload.PublishedIndex{Kind: upload.IndexKindReadmeRedirect}))

	html := bodyOf(t, up, "flash/1/abc/index.html")
	assert.Contains(t, html, `location.replace("README.html")`)
	assert.Equal(t, "readme-redirect", markerIn(html))
	assert.Equal(t, "README.html", res.Entry)
	assert.NotContains(t, stderr, "warning")
}

func TestUploadFiles_GeneratedReadmeRedirect_IndexFlagWithoutReadmeLists(t *testing.T) {
	// the redirect's README is gone from the overlay (e.g. renamed locally and
	// the old object expired), so --index produces a listing.
	up, _, _ := uploadTree(t, map[string]string{"new.txt": "n"}, upload.IndexGenerate,
		upload.WithPublished([]string{"index.html", "old.txt"}),
		upload.WithPublishedIndex(upload.PublishedIndex{Kind: upload.IndexKindReadmeRedirect}))

	assert.Contains(t, bodyOf(t, up, "flash/1/abc/index.md"), "[old.txt](old.txt)")
	assert.Equal(t, "listing", markerIn(bodyOf(t, up, "flash/1/abc/index.html")))
}

func TestUploadFiles_GeneratedListing_LocalReadmeTakesOver(t *testing.T) {
	up, res, _ := uploadTree(t, map[string]string{"README.md": "# R"}, upload.IndexNone,
		upload.WithRenderer(render.NewMarkdownRenderer()),
		upload.WithPublished([]string{"index.html", "index.md", "old.txt"}),
		upload.WithPublishedIndex(upload.PublishedIndex{Kind: upload.IndexKindListing, Title: "T"}))

	assert.Contains(t, bodyOf(t, up, "flash/1/abc/index.html"), `location.replace("README.html")`)
	assert.Equal(t, "README.html", res.Entry)
}

func TestUploadFiles_GeneratedListing_MissingTitleFallsBack(t *testing.T) {
	up, _, stderr := uploadTree(t, map[string]string{"new.txt": "n"}, upload.IndexNone,
		upload.WithPublished([]string{"index.html", "index.md"}),
		upload.WithPublishedIndex(upload.PublishedIndex{Kind: upload.IndexKindListing}))

	assert.Contains(t, bodyOf(t, up, "flash/1/abc/index.md"), "[new.txt](new.txt)")
	assert.Contains(t, stderr, "warning: published index title not found")
}

func TestUploadFiles_Published_DollopDirListedAsSupportingFiles(t *testing.T) {
	up, _, _ := uploadTree(t, map[string]string{"new.txt": "n"}, upload.IndexGenerate,
		upload.WithPublished([]string{"old.md", ".dollop/github-markdown.css", ".dollop/d2/0123456789ab.svg"}))

	md := bodyOf(t, up, "flash/1/abc/index.md")
	main, supporting, ok := strings.Cut(md, "Supporting files")
	require.True(t, ok, "published assets are listed as supporting files")
	assert.NotContains(t, main, ".dollop")
	assert.Contains(t, supporting, "(.dollop/github-markdown.css)")
	assert.Contains(t, supporting, "(.dollop/d2/0123456789ab.svg)")
}

func TestUploadFiles_Render_LocalDollopDirOverwrittenByAssets(t *testing.T) {
	// a local .dollop directory (e.g. from a downloaded upload) must not keep
	// stale copies of the generated assets: the upload overwrites them.
	up, res, _ := uploadTree(t, map[string]string{
		"notes.md":                    "# Hi",
		".dollop/github-markdown.css": "stale",
		".dollop/extra.txt":           "mine",
	}, upload.IndexGenerate, upload.WithRenderer(render.NewMarkdownRenderer()))

	assert.Equal(t, 1, strings.Count(strings.Join(keysOf(up), "\n"), ".dollop/github-markdown.css"), "the asset is written once")
	assert.NotEqual(t, "stale", bodyOf(t, up, "flash/1/abc/.dollop/github-markdown.css"), "the generated asset wins")
	assert.Equal(t, "mine", bodyOf(t, up, "flash/1/abc/.dollop/extra.txt"), "other local files there are still uploaded")
	assert.ElementsMatch(t, keysOf(up), res.WrittenKeys)

	md := bodyOf(t, up, "flash/1/abc/index.md")
	main, supporting, ok := strings.Cut(md, "Supporting files")
	require.True(t, ok)
	assert.NotContains(t, main, ".dollop", "the asset directory is not a primary entry")
	assert.Contains(t, supporting, "(.dollop/extra.txt)")
	assert.Equal(t, 1, strings.Count(supporting, "(.dollop/github-markdown.css)"))
}

func TestUploadFiles_Published_AssetsOfIndexOnlyNotListed(t *testing.T) {
	// with no markdown under the prefix, the published shared assets only
	// served the previous generated index, as in a fresh --index upload.
	up, _, _ := uploadTree(t, map[string]string{"new.txt": "n"}, upload.IndexGenerate,
		upload.WithPublished([]string{"old.txt", "github-markdown.css", "dollop-theme-tide.css"}))

	md := bodyOf(t, up, "flash/1/abc/index.md")
	assert.NotContains(t, md, "Supporting files")
	assert.NotContains(t, md, "github-markdown.css")
}
