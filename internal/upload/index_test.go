package upload

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGenerateIndexMarkdown_PairsMarkdownSource(t *testing.T) {
	s := string(generateIndexMarkdown([]string{"notes.md", "notes.html", "data.json"}, nil))

	// the rendered HTML is a top-level entry with the markdown linked beside it
	assert.Contains(t, s, "- [notes.html](notes.html) · [source](notes.md)\n")
	assert.Equal(t, 1, strings.Count(s, "(notes.md)"), "markdown should appear exactly once, as the source link")
	// unrelated files remain plain entries
	assert.Contains(t, s, "- [data.json](data.json)\n")
}

func TestGenerateIndexMarkdown_HTMLWithoutSourceHasNoSourceLink(t *testing.T) {
	s := string(generateIndexMarkdown([]string{"page.html"}, nil))

	assert.Contains(t, s, "- [page.html](page.html)\n")
	assert.NotContains(t, s, "[source]", "an HTML file with no markdown source must not get a source link")
}

func TestGenerateIndexMarkdown_MarkdownWithoutHTMLStaysTopLevel(t *testing.T) {
	s := string(generateIndexMarkdown([]string{"readme.md"}, nil))

	// when there is no rendered HTML, the .md is the entry itself
	assert.Contains(t, s, "- [readme.md](readme.md)\n")
	assert.NotContains(t, s, "[source]")
}

func TestGenerateIndexMarkdown_SupportingFilesCollapsed(t *testing.T) {
	s := string(generateIndexMarkdown([]string{"a.html"}, []string{"github-markdown.css", "highlight-github.css"}))

	assert.Contains(t, s, "<details>\n<summary>Supporting files</summary>\n\n"+
		"- [github-markdown.css](github-markdown.css)\n"+
		"- [highlight-github.css](highlight-github.css)\n\n</details>\n")
	assert.NotContains(t, s, "<details open", "supporting files are collapsed by default")
}

func TestGenerateIndexMarkdown_NoSupportingSectionWhenEmpty(t *testing.T) {
	s := string(generateIndexMarkdown([]string{"a.txt"}, nil))
	assert.NotContains(t, s, "Supporting files")
	assert.NotContains(t, s, "<details>")
}

func TestGenerateIndexMarkdown_EscapesNamesAndPaths(t *testing.T) {
	s := string(generateIndexMarkdown([]string{"my [draft] (v2)_*.txt", "sub dir/a:b.txt", "c:d.txt"}, nil))

	assert.Contains(t, s, `- [my \[draft\] \(v2\)\_\*.txt](my%20%5Bdraft%5D%20%28v2%29_%2A.txt)`)
	assert.Contains(t, s, "- [sub dir/a:b.txt](sub%20dir/a:b.txt)")
	// a leading segment with a colon must not read as a URL scheme
	assert.Contains(t, s, "- [c:d.txt](./c:d.txt)")
}

func TestFirstFile(t *testing.T) {
	tests := []struct {
		name  string
		files []string
		want  string
	}{
		{"single file", []string{"photo.jpg"}, "photo.jpg"},
		{"single nested file", []string{"sub/file.txt"}, "sub/file.txt"},
		{"multiple files, picks alphabetical first", []string{"z.txt", "a.txt", "m.txt"}, "a.txt"},
		{"multiple files already sorted", []string{"apple.txt", "banana.txt"}, "apple.txt"},
		{"no files", nil, ""},
		{"empty files slice", []string{}, ""},
		{"html preferred over md same stem", []string{"notes.md", "notes.html"}, "notes.html"},
		{"html preferred over md multiple files", []string{"z.md", "z.html", "a.txt"}, "a.txt"},
		{"md alone no html", []string{"readme.md"}, "readme.md"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, firstFile(tc.files))
		})
	}
}
