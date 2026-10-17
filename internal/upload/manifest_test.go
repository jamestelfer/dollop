package upload_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
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

// readsRecorder records the keys read through it.
type readsRecorder struct {
	upload.ObjectHeadReader
	keys []string
	err  error
}

func (r *readsRecorder) ReadObjectHead(ctx context.Context, bucket, key string, n int64) ([]byte, error) {
	r.keys = append(r.keys, key)
	if r.err != nil {
		return nil, r.err
	}
	return r.ObjectHeadReader.ReadObjectHead(ctx, bucket, key, n)
}

func readManifest(t *testing.T, files map[string]string) (upload.Manifest, []string, string) {
	t.Helper()
	up, published := publishTo(t, "keep/site", files)
	r := &readsRecorder{ObjectHeadReader: up}
	var stderr bytes.Buffer
	m := upload.ReadPublishedManifest(context.Background(), r, "b", "keep/site", published, &stderr)
	return m, r.keys, stderr.String()
}

func TestReadPublishedManifest_Valid(t *testing.T) {
	m, keys, stderr := readManifest(t, map[string]string{
		".dollop/manifest.json": `{"version":1,"title":"alpha","index":"listing","extra":true,"files":{"a.html":{"title":"Alpha","more":1},"b.txt":{}}}`,
	})

	assert.Equal(t, upload.Manifest{Version: 1, Title: "alpha", Index: upload.IndexKindListing, Files: map[string]upload.ManifestFile{
		"a.html": {Title: "Alpha"}, "b.txt": {},
	}}, m, "unknown fields are ignored")
	assert.Equal(t, []string{"keep/site/.dollop/manifest.json"}, keys)
	assert.Empty(t, stderr)
}

func TestReadPublishedManifest_NotPublished(t *testing.T) {
	m, keys, stderr := readManifest(t, map[string]string{"a.txt": "a"})

	assert.Zero(t, m)
	assert.Empty(t, keys, "nothing is read when no manifest is listed")
	assert.Empty(t, stderr, "a prefix from before manifests is not warned about")
}

func TestReadPublishedManifest_TreatedAsAbsent(t *testing.T) {
	padded := `{"version":1,"files":{"a.txt":{"title":"A"}}}` + strings.Repeat(" ", 4<<20)
	for name, body := range map[string]string{
		"invalid JSON":    `{"version":1,`,
		"unknown version": `{"version":2,"files":{"a.txt":{"title":"A"}}}`,
		"no version":      `{"files":{"a.txt":{"title":"A"}}}`,
		"past the cap":    padded,
	} {
		t.Run(name, func(t *testing.T) {
			m, _, stderr := readManifest(t, map[string]string{".dollop/manifest.json": body})
			assert.Zero(t, m)
			assert.Equal(t, 1, strings.Count(stderr, "warning: "), stderr)
		})
	}
}

func TestReadPublishedManifest_Unreadable(t *testing.T) {
	up, published := publishTo(t, "keep/site", map[string]string{".dollop/manifest.json": `{"version":1,"files":{}}`})
	r := &readsRecorder{ObjectHeadReader: up, err: errors.New("access denied")}
	var stderr bytes.Buffer

	assert.Zero(t, upload.ReadPublishedManifest(context.Background(), r, "b", "keep/site", published, &stderr))
	assert.Contains(t, stderr.String(), "warning: ")
	assert.Contains(t, stderr.String(), "access denied")
}

func TestReadPublishedManifest_VanishedIsAbsent(t *testing.T) {
	up := &upload.DirUploader{Root: t.TempDir()}
	var stderr bytes.Buffer

	assert.Zero(t, upload.ReadPublishedManifest(context.Background(), up, "b", "keep/site", []string{".dollop/manifest.json"}, &stderr))
	assert.Empty(t, stderr.String())
}

func TestUploadFiles_PublishedManifest_TitlesPublishedFiles(t *testing.T) {
	up, _, _ := uploadTree(t, map[string]string{"b.html": "<h1>Beta</h1>"}, upload.IndexGenerate,
		upload.WithPublished([]string{"old.html", "extra.txt", ".dollop/manifest.json"}),
		upload.WithPublishedManifest(upload.Manifest{Version: 1, Files: map[string]upload.ManifestFile{
			"old.html":  {Title: "Old page"},
			"gone.html": {Title: "Gone"},
		}}))

	index := bodyOf(t, up, "flash/1/abc/index.md")
	assert.Contains(t, index, "- [Beta](b.html) <small>b.html</small>\n")
	assert.Contains(t, index, "- [Old page](old.html) <small>old.html</small>\n")
	assert.Contains(t, index, "- [extra.txt](extra.txt)\n")
	assert.NotContains(t, index, "gone", "manifest entries that are not published are dropped")

	assert.Equal(t, map[string]upload.ManifestFile{
		"b.html":    {Title: "Beta"},
		"old.html":  {Title: "Old page"},
		"extra.txt": {},
	}, manifestOf(t, up).Files)
}

func TestUploadFiles_PublishedManifest_LocalEntryWins(t *testing.T) {
	up, _, _ := uploadTree(t, map[string]string{"p.html": "<p>no heading now</p>"}, upload.IndexGenerate,
		upload.WithPublished([]string{"p.html"}),
		upload.WithPublishedManifest(upload.Manifest{Version: 1, Files: map[string]upload.ManifestFile{"p.html": {Title: "Old"}}}))

	assert.Contains(t, bodyOf(t, up, "flash/1/abc/index.md"), "- [p.html](p.html)\n")
	assert.Equal(t, map[string]upload.ManifestFile{"p.html": {}}, manifestOf(t, up).Files)
}

func TestUploadFiles_PublishedManifest_LeavesOutDollopFiles(t *testing.T) {
	up, _, _ := uploadTree(t, map[string]string{"a.txt": "a"}, upload.IndexNone,
		upload.WithPublished([]string{"index.html", "index.md", "x.txt", "github-markdown.css", ".dollop/dollop-toc.js", ".dollop/manifest.json"}),
		upload.WithPublishedIndex(upload.PublishedIndex{Kind: upload.IndexKindListing, Title: "site"}))

	assert.Equal(t, map[string]upload.ManifestFile{"a.txt": {}, "x.txt": {}}, manifestOf(t, up).Files,
		"generated index pages and assets are not recorded")
}

func TestUploadFiles_PublishedManifest_KeepsUserIndexPage(t *testing.T) {
	up, _, _ := uploadTree(t, map[string]string{"a.txt": "a"}, upload.IndexNone,
		upload.WithPublished([]string{"index.html"}),
		upload.WithPublishedManifest(upload.Manifest{Version: 1, Files: map[string]upload.ManifestFile{"index.html": {Title: "Home"}}}))

	assert.Equal(t, map[string]upload.ManifestFile{"a.txt": {}, "index.html": {Title: "Home"}}, manifestOf(t, up).Files)
}

func TestUploadFiles_PublishedManifest_KeepsUserMarkdownIndexUnderReadmeRedirect(t *testing.T) {
	up, _, _ := uploadTree(t, map[string]string{"a.txt": "a"}, upload.IndexNone,
		upload.WithPublished([]string{"index.html", "index.md", "README.md"}),
		upload.WithPublishedIndex(upload.PublishedIndex{Kind: upload.IndexKindReadmeRedirect}),
		upload.WithPublishedManifest(upload.Manifest{Version: 1, Files: map[string]upload.ManifestFile{"index.md": {Title: "Mine"}}}))

	assert.Equal(t, map[string]upload.ManifestFile{"a.txt": {}, "index.md": {Title: "Mine"}, "README.md": {}}, manifestOf(t, up).Files,
		"only the redirect is dollop's; the index.md beside it is the user's")
}
