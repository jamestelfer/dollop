package upload_test

import (
	"testing"

	"github.com/jamestelfer/dollop/internal/upload"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEphemeralPrefix(t *testing.T) {
	assert.Equal(t, "flash/1/abc123", upload.EphemeralPrefix(1, "abc123"))
	assert.Equal(t, "flash/7/xyz", upload.EphemeralPrefix(7, "xyz"))
	assert.Equal(t, "flash/14/id99", upload.EphemeralPrefix(14, "id99"))
}

func TestPermanentPrefix(t *testing.T) {
	assert.Equal(t, "keep/happy-dog", upload.PermanentPrefix("happy-dog"))
}

func TestPublicURL(t *testing.T) {
	tests := []struct {
		baseURL string
		prefix  string
		suffix  string
		want    string
	}{
		{"https://drop.example.com", "dollop/7/abc123", "", "https://drop.example.com/dollop/7/abc123/"},
		{"https://drop.example.com/", "keep/happy-dog", "", "https://drop.example.com/keep/happy-dog/"},
		{"drop.example.com", "dollop/1/xyz", "", "https://drop.example.com/dollop/1/xyz/"},
		{"HTTPS://drop.example.com", "dollop/1/xyz", "", "HTTPS://drop.example.com/dollop/1/xyz/"},
		{"https://drop.example.com", "flash/1/abc", "photo.jpg", "https://drop.example.com/flash/1/abc/photo.jpg"},
		{"https://drop.example.com", "flash/7/xyz", "sub/file.txt", "https://drop.example.com/flash/7/xyz/sub/file.txt"},
	}
	for _, tc := range tests {
		assert.Equal(t, tc.want, upload.PublicURL(tc.baseURL, tc.prefix, tc.suffix))
	}
}

func TestResolvePrefix(t *testing.T) {
	const base = "https://drop.example.com"
	tests := []struct {
		name    string
		baseURL string
		input   string
		want    string
	}{
		// A full URL and the bare prefix it was built from resolve identically.
		{"full url ephemeral root", base, "https://drop.example.com/flash/7/abc123/", "flash/7/abc123"},
		{"bare ephemeral prefix", base, "flash/7/abc123", "flash/7/abc123"},
		{"bare ephemeral prefix trailing slash", base, "flash/7/abc123/", "flash/7/abc123"},

		// Trailing filename / index.html / nested path are stripped.
		{"full url with filename", base, "https://drop.example.com/flash/1/xyz/photo.jpg", "flash/1/xyz"},
		{"full url with index.html", base, "https://drop.example.com/flash/14/id99/index.html", "flash/14/id99"},
		{"full url with nested file", base, "https://drop.example.com/flash/7/abc/sub/file.txt", "flash/7/abc"},
		{"bare prefix with filename", base, "flash/1/xyz/photo.jpg", "flash/1/xyz"},

		// keep prefixes have two segments.
		{"full url keep root", base, "https://drop.example.com/keep/happy-cat/", "keep/happy-cat"},
		{"bare keep prefix", base, "keep/happy-cat", "keep/happy-cat"},
		{"full url keep with file", base, "https://drop.example.com/keep/happy-cat/notes.html", "keep/happy-cat"},

		// base_url variations: no scheme, trailing slash, sub-path.
		{"base without scheme", "drop.example.com", "https://drop.example.com/flash/7/abc/", "flash/7/abc"},
		{"base with trailing slash", "https://drop.example.com/", "https://drop.example.com/keep/cool-fox/", "keep/cool-fox"},
		{"base with subpath, full url", "https://cdn.example.com/files", "https://cdn.example.com/files/flash/7/abc/x.png", "flash/7/abc"},
		{"base with subpath, bare prefix", "https://cdn.example.com/files", "flash/7/abc", "flash/7/abc"},

		// Empty base still resolves a full URL via its path.
		{"empty base, full url", "", "https://anything.example/flash/7/abc/x.png", "flash/7/abc"},
		{"empty base, bare prefix", "", "keep/happy-cat", "keep/happy-cat"},

		// Surrounding whitespace is tolerated (copy/paste).
		{"whitespace trimmed", base, "  flash/7/abc123/  ", "flash/7/abc123"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := upload.ResolvePrefix(tc.baseURL, tc.input)
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestResolvePrefix_Errors(t *testing.T) {
	const base = "https://drop.example.com"
	tests := []struct {
		name    string
		baseURL string
		input   string
	}{
		{"empty input", base, ""},
		{"whitespace only", base, "   "},
		{"unknown prefix root", base, "https://drop.example.com/bogus/thing/"},
		{"bare unknown prefix", base, "random/path"},
		{"incomplete flash prefix", base, "flash/7"},
		{"incomplete keep prefix", base, "keep"},
		{"base only, no prefix", base, "https://drop.example.com/"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := upload.ResolvePrefix(tc.baseURL, tc.input)
			assert.Error(t, err)
		})
	}
}

func TestResolvePrefix_RoundTripsPublicURL(t *testing.T) {
	// ResolvePrefix is the inverse of PublicURL: a URL built from a prefix must
	// resolve back to that prefix.
	base := "https://drop.example.com"
	for _, prefix := range []string{"flash/1/abc", "flash/7/xyz123", "flash/14/id", "keep/happy-cat"} {
		url := upload.PublicURL(base, prefix, "photo.jpg")
		got, err := upload.ResolvePrefix(base, url)
		require.NoError(t, err)
		assert.Equal(t, prefix, got)
	}
}

func TestURLSuffix(t *testing.T) {
	tests := []struct {
		name      string
		indexFlag bool
		files     []string
		want      string
	}{
		{"index flag, no files", true, nil, ""},
		{"index flag, single file", true, []string{"photo.jpg"}, ""},
		{"index flag, multiple files", true, []string{"a.txt", "b.txt"}, ""},
		{"index.html in files", false, []string{"index.html"}, ""},
		{"index.html among multiple files", false, []string{"index.html", "photo.jpg"}, ""},
		{"single file", false, []string{"photo.jpg"}, "photo.jpg"},
		{"single nested file", false, []string{"sub/file.txt"}, "sub/file.txt"},
		{"multiple files, picks alphabetical first", false, []string{"z.txt", "a.txt", "m.txt"}, "a.txt"},
		{"multiple files already sorted", false, []string{"apple.txt", "banana.txt"}, "apple.txt"},
		{"no files", false, nil, ""},
		{"empty files slice", false, []string{}, ""},
		{"html preferred over md same stem", false, []string{"notes.md", "notes.html"}, "notes.html"},
		{"html preferred over md multiple files", false, []string{"z.md", "z.html", "a.txt"}, "a.txt"},
		{"md alone no html", false, []string{"readme.md"}, "readme.md"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, upload.URLSuffix(tc.indexFlag, tc.files))
		})
	}
}
