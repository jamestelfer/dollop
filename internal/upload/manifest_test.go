package upload_test

import (
	"encoding/json"
	"testing"

	"github.com/jamestelfer/dollop/internal/render"
	"github.com/jamestelfer/dollop/internal/upload"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const manifestKey = "flash/1/abc/.dollop/manifest.json"

// manifestOf decodes the manifest uploaded to up.
func manifestOf(t *testing.T, up *fakeUploader) upload.Manifest {
	t.Helper()
	var m upload.Manifest
	require.NoError(t, json.Unmarshal([]byte(bodyOf(t, up, manifestKey)), &m))
	return m
}

func TestUploadFiles_Manifest_WrittenLastWithNoCache(t *testing.T) {
	up, res, _ := uploadTree(t, map[string]string{"a.html": "<h1>Alpha</h1>", "b.txt": "b"}, upload.IndexGenerate,
		upload.WithRenderer(render.NewMarkdownRenderer()))

	last := up.calls[len(up.calls)-1]
	assert.Equal(t, manifestKey, last.key, "the manifest is the last object written")
	assert.Equal(t, "application/json", last.contentType)
	assert.Equal(t, "no-cache", last.options.CacheControl)
	assert.Contains(t, res.WrittenKeys, manifestKey)
	assert.Equal(t, 1, countKey(up, manifestKey))
}

func TestUploadFiles_Manifest_ListsPublishedFilesWithPageTitles(t *testing.T) {
	up, _, _ := uploadTree(t, map[string]string{
		"a.html":    "<h1>Alpha</h1>",
		"notes.md":  "# Release Notes\n",
		"doc.pdf":   "%PDF",
		"sub/x.txt": "x",
	}, upload.IndexGenerate, upload.WithRenderer(render.NewMarkdownRenderer()))

	m := manifestOf(t, up)
	assert.Equal(t, 1, m.Version)
	assert.NotEmpty(t, m.Title)
	assert.Equal(t, upload.IndexKindListing, m.Index)
	assert.Equal(t, map[string]upload.ManifestFile{
		"a.html":     {Title: "Alpha"},
		"notes.html": {Title: "Release Notes"},
		"notes.md":   {},
		"doc.pdf":    {},
		"sub/x.txt":  {},
	}, m.Files, "dollop's own files and the generated index are left out")
}

func TestUploadFiles_Manifest_OmitsMissingTitlesInJSON(t *testing.T) {
	up, _, _ := uploadTree(t, map[string]string{"b.txt": "b"}, upload.IndexNone)

	var raw map[string]any
	require.NoError(t, json.Unmarshal([]byte(bodyOf(t, up, manifestKey)), &raw))
	assert.Equal(t, map[string]any{"b.txt": map[string]any{}}, raw["files"])
	assert.NotContains(t, raw, "index", "no entry page was generated")
}

func TestUploadFiles_Manifest_WrittenWithoutIndex(t *testing.T) {
	up, _, _ := uploadTree(t, map[string]string{"a.html": "<h1>Alpha</h1>"}, upload.IndexNone)
	assert.Equal(t, map[string]upload.ManifestFile{"a.html": {Title: "Alpha"}}, manifestOf(t, up).Files)
}

func TestUploadFiles_Manifest_LocalIndexPageIsAFile(t *testing.T) {
	up, _, _ := uploadTree(t, map[string]string{"index.html": "<h1>Home</h1>"}, upload.IndexGenerate)

	m := manifestOf(t, up)
	assert.Empty(t, m.Index)
	assert.Equal(t, map[string]upload.ManifestFile{"index.html": {Title: "Home"}}, m.Files)
}

func TestUploadFiles_Manifest_ReadmeRedirect(t *testing.T) {
	up, _, _ := uploadTree(t, map[string]string{"README.md": "# Project\n"}, upload.IndexNone)

	m := manifestOf(t, up)
	assert.Equal(t, upload.IndexKindReadmeRedirect, m.Index)
	assert.Equal(t, map[string]upload.ManifestFile{"README.md": {Title: "Project"}}, m.Files)
}

func TestUploadFiles_Manifest_TitledByPublishedListing(t *testing.T) {
	up, _, _ := uploadTree(t, map[string]string{"b.txt": "b"}, upload.IndexNone,
		upload.WithPublished([]string{"index.html", "index.md"}),
		upload.WithPublishedIndex(upload.PublishedIndex{Kind: upload.IndexKindListing, Title: "alpha"}))

	assert.Equal(t, "alpha", manifestOf(t, up).Title)
}

func TestUploadFiles_Manifest_LocalCopyReplacedByGenerated(t *testing.T) {
	up, res, _ := uploadTree(t, map[string]string{
		".dollop/manifest.json": `{"version":1,"title":"stale","files":{"gone.txt":{}}}`,
		".dollop/keep.txt":      "kept",
		"b.md":                  "b",
	}, upload.IndexGenerate)

	assert.Equal(t, 1, countKey(up, manifestKey), "the local copy is not uploaded")
	m := manifestOf(t, up)
	assert.NotEqual(t, "stale", m.Title)
	assert.Equal(t, map[string]upload.ManifestFile{"b.md": {}}, m.Files)
	assert.NotContains(t, res.SourceRelPaths, ".dollop/manifest.json")

	index := bodyOf(t, up, "flash/1/abc/index.md")
	assert.Contains(t, index, "(.dollop/keep.txt)")
	assert.NotContains(t, index, "manifest.json", "the manifest is neither a file nor a supporting file")
}

func TestUploadFiles_Manifest_PublishedCopyNotListed(t *testing.T) {
	up, _, _ := uploadTree(t, map[string]string{"notes.md": "# Notes\n"}, upload.IndexGenerate,
		upload.WithPublished([]string{".dollop/manifest.json", ".dollop/github-markdown.css", "old.txt"}))

	index := bodyOf(t, up, "flash/1/abc/index.md")
	assert.Contains(t, index, "(old.txt)")
	assert.NotContains(t, index, "manifest.json")
}

func countKey(up *fakeUploader, key string) int {
	n := 0
	for _, c := range up.calls {
		if c.key == key {
			n++
		}
	}
	return n
}
