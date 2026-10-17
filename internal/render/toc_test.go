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

// renderDoc renders md as doc.md and returns the page HTML and shared assets.
func renderDoc(t *testing.T, md string) (string, []render.SharedAsset) {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "doc.md"), []byte(md), 0o600))

	r := render.NewMarkdownRenderer()
	sources, assets, err := r.Plan([]string{"doc.md"}, rootFS(t, dir), "flash/1/testid")
	require.NoError(t, err)
	return openSource(t, sources, "doc.html"), assets
}

// tocSection returns the wide-screen TOC nav, failing if it is absent.
func tocSection(t *testing.T, html string) string {
	t.Helper()
	start := strings.Index(html, `<nav class="page-toc"`)
	require.NotEqual(t, -1, start, "page must render a TOC nav")
	end := strings.Index(html[start:], "</nav>")
	require.NotEqual(t, -1, end)
	return html[start : start+end]
}

// TestTOC_ListsH2ToH4 verifies the TOC links every h2–h4 heading to its id, in
// document order, with its depth, and leaves out the page title and h5+.
func TestTOC_ListsH2ToH4(t *testing.T) {
	md := "# Title\n\n## Install\n\n### From source\n\n#### Prerequisites\n\n##### Too deep\n\n## Usage\n"
	html, _ := renderDoc(t, md)

	toc := tocSection(t, html)
	install := strings.Index(toc, `<a class="page-toc-link" href="#install" data-depth="2">Install</a>`)
	source := strings.Index(toc, `<a class="page-toc-link" href="#from-source" data-depth="3">From source</a>`)
	prereq := strings.Index(toc, `<a class="page-toc-link" href="#prerequisites" data-depth="4">Prerequisites</a>`)
	usage := strings.Index(toc, `<a class="page-toc-link" href="#usage" data-depth="2">Usage</a>`)
	require.NotEqual(t, -1, install, toc)
	require.NotEqual(t, -1, source, toc)
	require.NotEqual(t, -1, prereq, toc)
	require.NotEqual(t, -1, usage, toc)
	assert.Less(t, install, source)
	assert.Less(t, source, prereq)
	assert.Less(t, prereq, usage)

	assert.NotContains(t, toc, "Title")
	assert.NotContains(t, toc, "Too deep")
	assert.Contains(t, toc, `aria-label="On this page"`)
}

// TestTOC_InlineCopyForNarrowScreens verifies the same links are rendered in a
// collapsible block above the content for viewports too narrow for the side
// column.
func TestTOC_InlineCopyForNarrowScreens(t *testing.T) {
	html, _ := renderDoc(t, "## One\n\n## Two\n")

	start := strings.Index(html, `<details class="page-toc-inline">`)
	require.NotEqual(t, -1, start, "page must render the inline TOC")
	inline := html[start : start+strings.Index(html[start:], "</details>")]
	assert.Contains(t, inline, "<summary>On this page</summary>")
	assert.Contains(t, inline, `href="#one"`)
	assert.Contains(t, inline, `href="#two"`)
	assert.Less(t, start, strings.Index(html, `<div class="markdown-body">`), "inline TOC must precede the content")
}

// TestTOC_OmittedBelowTwoHeadings verifies a page with fewer than two h2–h4
// headings renders neither TOC nor its script.
func TestTOC_OmittedBelowTwoHeadings(t *testing.T) {
	html, _ := renderDoc(t, "# Title\n\n## Only one\n\nText\n")

	assert.NotContains(t, html, "page-toc")
	assert.NotContains(t, html, "dollop-toc.js")
}

// TestTOC_HeadingText verifies TOC entries hold the heading's plain text: code
// spans and emphasis keep their text, the anchor link glyph is dropped, and
// markup characters are escaped.
func TestTOC_HeadingText(t *testing.T) {
	html, _ := renderDoc(t, "## Use `<b>` tags\n\n## A *very* & bold one\n")

	toc := tocSection(t, html)
	assert.Contains(t, toc, `data-depth="2">Use &lt;b&gt; tags</a>`)
	assert.Contains(t, toc, `data-depth="2">A very &amp; bold one</a>`)
}

func TestTOC_HeadingTextResolvesEntitiesAndEscapes(t *testing.T) {
	html, _ := renderDoc(t, "## Fish &amp; Chips &#35;1\n\n## \\*not emphasis\\*\n")

	toc := tocSection(t, html)
	assert.Contains(t, toc, `data-depth="2">Fish &amp; Chips #1</a>`)
	assert.Contains(t, toc, `data-depth="2">*not emphasis*</a>`)
}

// TestTOC_ScriptAsset verifies the TOC script ships as a shared asset and is
// loaded deferred, by its versioned reference, only on pages with a TOC.
func TestTOC_ScriptAsset(t *testing.T) {
	html, assets := renderDoc(t, "## One\n\n## Two\n")

	script := assetContent(t, assets, "dollop-toc.js")
	assert.Contains(t, script, ".page-toc-track")
	for _, a := range assets {
		if a.Name == "dollop-toc.js" {
			assert.Equal(t, "text/javascript; charset=utf-8", a.ContentType)
		}
	}
	assert.Contains(t, html, `<script src=".dollop/dollop-toc.js?v=`)
	assert.Contains(t, html, `" defer></script>`)
}
