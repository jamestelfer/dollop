package render_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jamestelfer/dollop/internal/render"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func renderIconMarkdown(t *testing.T, markdown string) string {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "page.md"), []byte(markdown), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "notes.md"), []byte("# Notes"), 0600))
	sources, _, err := render.NewMarkdownRenderer().Plan([]string{"page.md", "notes.md"}, rootFS(t, dir), "flash/1/testid")
	require.NoError(t, err)
	return openSource(t, sources, "page.html")
}

func TestMarkdownIcon_Marker(t *testing.T) {
	html := renderIconMarkdown(t, "# Files\n\nIcon ::md-icon:: twice ::md-icon::\n")
	assert.Equal(t, 1, strings.Count(html, `<symbol id="dollop-md-icon"`))
	assert.Equal(t, 3, strings.Count(html, `<use href="#dollop-md-icon"`), "header and two inline icons")
	assert.Contains(t, html, `stroke="currentColor"`)
	assert.Contains(t, html, `role="img" aria-label="Markdown source"`)
	assert.NotContains(t, html, "dollop-md-icon-placeholder")
}

func TestMarkdownIcon_Standalone(t *testing.T) {
	html := renderIconMarkdown(t, "::md-icon::\n")
	assert.Equal(t, 2, strings.Count(html, `<use href="#dollop-md-icon"`))
}

func TestMarkdownIcon_SourceLinks(t *testing.T) {
	html := renderIconMarkdown(t, "[Page](notes.md) [::md-icon::](notes.md)\n\n[::md-icon::][source]\n\n[source]: notes.md\n")
	assert.Contains(t, html, `<a href="notes.html">Page</a>`)
	assert.Equal(t, 2, strings.Count(html, `<a href="notes.md"><svg`), "inline and reference source links stay raw")
}

func TestMarkdownIcon_HTMLMarkersAreUnsupported(t *testing.T) {
	html := renderIconMarkdown(t, "Inline <md-icon/> and <md-icon />\n\n<md-icon/>\n\n<md-icon />\n")
	assert.Equal(t, 1, strings.Count(html, `<use href="#dollop-md-icon"`), "only the header icon")
}

func TestMarkdownIcon_LiteralsAndSanitization(t *testing.T) {
	html := renderIconMarkdown(t, "`::md-icon::` `<md-icon/>`\n\n```text\n::md-icon::\n<md-icon/>\n```\n\n\\::md-icon::\n\n<md-icon onclick=\"alert(1)\"/>\n\n<svg onload=\"alert(1)\"><use href=\"https://evil.example/icon.svg#icon\" /></svg>\n\n<script>alert(1)</script>\n")
	assert.Equal(t, 1, strings.Count(html, `<use href="#dollop-md-icon"`), "only the header icon, not code or escaped markers")
	assert.Contains(t, html, "<code>::md-icon::</code>")
	assert.Contains(t, html, "&lt;md-icon/&gt;")
	assert.NotContains(t, html, "onclick")
	assert.NotContains(t, html, "onload")
	assert.NotContains(t, html, "evil.example")
	assert.NotContains(t, html, "<script>alert(1)</script>")
}
