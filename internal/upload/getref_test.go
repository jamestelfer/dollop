package upload_test

import (
	"testing"

	"github.com/jamestelfer/dollop/internal/upload"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveGetRef_Accepts(t *testing.T) {
	tests := []struct {
		name, base, input string
		want              upload.GetRef
	}{
		{"bare flash", "", "flash/7/abc", upload.GetRef{Prefix: "flash/7/abc", Path: "flash/7/abc"}},
		{"any day count", "", "flash/30/abc", upload.GetRef{Prefix: "flash/30/abc", Path: "flash/30/abc"}},
		{"trailing slash", "", "flash/7/abc/", upload.GetRef{Prefix: "flash/7/abc", Path: "flash/7/abc"}},
		{"leading slash", "", "/keep/happy-cat", upload.GetRef{Prefix: "keep/happy-cat", Path: "keep/happy-cat"}},
		{"keep sub path", "", "keep/happy-cat.aB3.x9Z.q2M/docs/a.md", upload.GetRef{Prefix: "keep/happy-cat.aB3.x9Z.q2M", Path: "keep/happy-cat.aB3.x9Z.q2M/docs/a.md"}},
		{"full url", "https://drop.example.com", "https://drop.example.com/flash/1/xyz/index.html", upload.GetRef{Prefix: "flash/1/xyz", Path: "flash/1/xyz/index.html"}},
		{"base url sub path", "https://cdn.example.com/files", "https://cdn.example.com/files/keep/n/a.txt", upload.GetRef{Prefix: "keep/n", Path: "keep/n/a.txt"}},
		{"encoded space", "", "flash/7/abc/my%20file.txt", upload.GetRef{Prefix: "flash/7/abc", Path: "flash/7/abc/my file.txt"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := upload.ResolveGetRef(tt.base, tt.input)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestResolveGetRef_Rejects(t *testing.T) {
	inputs := []string{
		"",
		"/",
		"flash",
		"flash/7",
		"flash/7/",
		"flash/x/abc",
		"keep",
		"keep/",
		"deps/mermaid/11.0.0",
		"other/thing",
		"https://drop.example.com/",
		"flash/7/abc/../def",
		"flash/7/../7/abc",
		"keep/../flash/7/abc",
		"flash/7/abc/./a.txt",
		"flash/7/abc//a.txt",
		"flash/7/abc/%2e%2e/def",
		"flash/7/abc%2F..%2Fdef",
		"https://drop.example.com/flash/7/abc/%2e%2e/x",
		"flash/7/abc/a\\b",
		"flash/7/abc/a%00b",
		"//flash/7/abc",
		"flash/7/abc%zz",
	}
	for _, in := range inputs {
		t.Run(in, func(t *testing.T) {
			_, err := upload.ResolveGetRef("https://drop.example.com", in)
			assert.Error(t, err)
		})
	}
}

func TestGetRef_Whole(t *testing.T) {
	assert.True(t, upload.GetRef{Prefix: "keep/a", Path: "keep/a"}.Whole())
	assert.False(t, upload.GetRef{Prefix: "keep/a", Path: "keep/a/b"}.Whole())
}
