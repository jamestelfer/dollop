package upload

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateIndexHTML_PairsMarkdownSource(t *testing.T) {
	html, err := generateIndexHTML([]string{"notes.md", "notes.html", "data.json"}, nil)
	require.NoError(t, err)
	s := string(html)

	// the rendered HTML is a top-level entry
	assert.Contains(t, s, `href="notes.html"`)
	// the markdown is linked only as the source alongside it, never as its own item
	assert.Equal(t, 1, strings.Count(s, `href="notes.md"`), "markdown should appear exactly once, as the source link")
	assert.Contains(t, s, `class="source"`)
	// unrelated files remain plain entries
	assert.Contains(t, s, `href="data.json"`)
}

func TestGenerateIndexHTML_HTMLWithoutSourceHasNoSourceLink(t *testing.T) {
	html, err := generateIndexHTML([]string{"page.html"}, nil)
	require.NoError(t, err)
	s := string(html)

	assert.Contains(t, s, `href="page.html"`)
	assert.NotContains(t, s, `class="source"`, "an HTML file with no markdown source must not get a source link")
}

func TestGenerateIndexHTML_MarkdownWithoutHTMLStaysTopLevel(t *testing.T) {
	html, err := generateIndexHTML([]string{"readme.md"}, nil)
	require.NoError(t, err)
	s := string(html)

	// when there is no rendered HTML, the .md is the entry itself
	assert.Contains(t, s, `href="readme.md"`)
	assert.NotContains(t, s, `class="source"`)
}

func TestGenerateIndexHTML_SupportingSection(t *testing.T) {
	html, err := generateIndexHTML([]string{"a.html"}, []string{"github-markdown.css", "highlight-github.css"})
	require.NoError(t, err)
	s := string(html)

	assert.Contains(t, s, "Supporting files")
	assert.Contains(t, s, `href="github-markdown.css"`)
	assert.Contains(t, s, `href="highlight-github.css"`)
}

func TestGenerateIndexHTML_NoSupportingSectionWhenEmpty(t *testing.T) {
	html, err := generateIndexHTML([]string{"a.txt"}, nil)
	require.NoError(t, err)
	assert.NotContains(t, string(html), "Supporting files")
}
