package upload_test

import (
	"testing"

	"github.com/jamestelfer/dollop/internal/upload"
	"github.com/stretchr/testify/assert"
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
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, upload.URLSuffix(tc.indexFlag, tc.files))
		})
	}
}
