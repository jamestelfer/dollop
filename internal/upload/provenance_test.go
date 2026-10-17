package upload_test

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jamestelfer/dollop/internal/upload"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// publishTo writes files under prefix in a DirUploader and returns it with the
// published relative paths.
func publishTo(t *testing.T, prefix string, files map[string]string) (*upload.DirUploader, []string) {
	t.Helper()
	up := &upload.DirUploader{Root: t.TempDir()}
	rel := make([]string, 0, len(files))
	for name, body := range files {
		require.NoError(t, up.PutObject(context.Background(), "b", prefix+"/"+name, "", bytes.NewReader([]byte(body))))
		rel = append(rel, name)
	}
	return up, rel
}

const markedListing = "<!DOCTYPE html>\n<html>\n<head>\n<meta charset=\"utf-8\">\n<meta name=\"dollop-generated\" content=\"listing\">\n<title>x</title>"

func TestInspectPublishedIndex(t *testing.T) {
	for name, tc := range map[string]struct {
		files map[string]string
		want  upload.PublishedIndex
	}{
		"no index":   {map[string]string{"a.txt": "a"}, upload.PublishedIndex{}},
		"user index": {map[string]string{"index.html": "<h1>mine</h1>"}, upload.PublishedIndex{}},
		"marker past 512 bytes": {map[string]string{
			"index.html": "<html><head>" + strings.Repeat(" ", 512) + `<meta name="dollop-generated" content="listing">`,
		}, upload.PublishedIndex{}},
		"unknown kind": {map[string]string{
			"index.html": `<meta charset="utf-8"><meta name="dollop-generated" content="other">`,
		}, upload.PublishedIndex{}},
		"readme redirect": {map[string]string{
			"index.html": `<meta charset="UTF-8"><meta name="dollop-generated" content="readme-redirect">`,
		}, upload.PublishedIndex{Kind: upload.IndexKindReadmeRedirect}},
		"listing with title": {map[string]string{
			"index.html": markedListing,
			"index.md":   "---\ntitle: Q3 Review — ünïcode\n---\n\n# Q3 Review\n",
		}, upload.PublishedIndex{Kind: upload.IndexKindListing, Title: "Q3 Review — ünïcode"}},
		"listing without index.md": {map[string]string{"index.html": markedListing}, upload.PublishedIndex{Kind: upload.IndexKindListing}},
	} {
		t.Run(name, func(t *testing.T) {
			up, published := publishTo(t, "keep/x", tc.files)
			got, err := upload.InspectPublishedIndex(context.Background(), up, "b", "keep/x", published, "")
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

type failingReader struct{}

func (failingReader) ReadObjectHead(context.Context, string, string, int64) ([]byte, error) {
	return nil, errors.New("access denied")
}

func TestInspectPublishedIndex_ReadErrorFails(t *testing.T) {
	_, err := upload.InspectPublishedIndex(context.Background(), failingReader{}, "b", "keep/x", []string{"index.html"}, "")
	assert.ErrorContains(t, err, "access denied")
}

func TestInspectPublishedIndex_ListingTitle(t *testing.T) {
	tests := []struct {
		name          string
		manifestTitle string
		want          string
		reads         []string
	}{
		{"from the manifest, without reading index.md", "alpha", "alpha", []string{"keep/x/index.html"}},
		{"from index.md without a manifest title", "", "From index.md", []string{"keep/x/index.html", "keep/x/index.md"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			up, published := publishTo(t, "keep/x", map[string]string{
				"index.html": markedListing,
				"index.md":   "---\ntitle: From index.md\n---\n",
			})
			r := &readsRecorder{ObjectHeadReader: up}

			got, err := upload.InspectPublishedIndex(context.Background(), r, "b", "keep/x", published, tt.manifestTitle)
			require.NoError(t, err)
			assert.Equal(t, upload.PublishedIndex{Kind: upload.IndexKindListing, Title: tt.want}, got)
			assert.Equal(t, tt.reads, r.keys)
		})
	}
}

func TestInspectPublishedIndex_ManifestDoesNotDecideProvenance(t *testing.T) {
	up, published := publishTo(t, "keep/x", map[string]string{"index.html": "<h1>hand-written</h1>"})

	got, err := upload.InspectPublishedIndex(context.Background(), up, "b", "keep/x", published, "alpha")
	require.NoError(t, err)
	assert.Equal(t, upload.PublishedIndex{}, got, "only the marker says the page is generated")
}
