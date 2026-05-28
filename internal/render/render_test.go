package render_test

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jamestelfer/dollop/internal/render"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// sourceRelPaths extracts the RelPath from each Source.
func sourceRelPaths(sources []render.Source) []string {
	paths := make([]string, len(sources))
	for i, s := range sources {
		paths[i] = s.RelPath
	}
	return paths
}

// openSource opens the Source with the given RelPath and returns its full content.
func openSource(t *testing.T, sources []render.Source, relPath string) string {
	t.Helper()
	for _, src := range sources {
		if src.RelPath == relPath {
			rc, err := src.Open()
			require.NoError(t, err)
			defer rc.Close() //nolint:errcheck
			content, err := io.ReadAll(rc)
			require.NoError(t, err)
			return string(content)
		}
	}
	t.Fatalf("source %q not found in plan results (got %v)", relPath, sourceRelPaths(sources))
	return ""
}

// TestDiskRenderer_PassesThrough verifies that the disk renderer returns its
// input unchanged with no shared assets.
func TestDiskRenderer_PassesThrough(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "notes.md"), []byte("# Hello"), 0600))

	r := render.NewDiskRenderer()
	sources, assets, err := r.Plan([]string{"notes.md"}, dir)
	require.NoError(t, err)

	assert.Equal(t, []string{"notes.md"}, sourceRelPaths(sources))
	assert.Nil(t, assets)
}

// TestDiskRenderer_EmptySlice verifies the disk renderer handles empty input.
func TestDiskRenderer_EmptySlice(t *testing.T) {
	r := render.NewDiskRenderer()
	sources, _, err := r.Plan([]string{}, t.TempDir())
	require.NoError(t, err)
	assert.Empty(t, sources)
}

// TestDiskRenderer_OpenReadsFile verifies that opening a disk Source returns
// the file's content.
func TestDiskRenderer_OpenReadsFile(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "hello.txt"), []byte("hello world"), 0600))

	r := render.NewDiskRenderer()
	sources, _, err := r.Plan([]string{"hello.txt"}, dir)
	require.NoError(t, err)
	require.Len(t, sources, 1)

	content := openSource(t, sources, "hello.txt")
	assert.Equal(t, "hello world", content)
}

// TestDiskRenderer_OpenIsSeekable verifies that disk sources return seekable
// bodies. The S3 SDK requires Seek to determine Content-Length automatically.
func TestDiskRenderer_OpenIsSeekable(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "data.txt"), []byte("hello world"), 0600))

	r := render.NewDiskRenderer()
	sources, _, err := r.Plan([]string{"data.txt"}, dir)
	require.NoError(t, err)
	require.Len(t, sources, 1)

	rc, err := sources[0].Open()
	require.NoError(t, err)
	defer rc.Close() //nolint:errcheck

	first, err := io.ReadAll(rc)
	require.NoError(t, err)

	_, err = rc.Seek(0, io.SeekStart)
	require.NoError(t, err, "disk source must be seekable")

	second, err := io.ReadAll(rc)
	require.NoError(t, err)
	assert.Equal(t, first, second, "seek-and-reread must return identical content")
}

// TestMarkdownRenderer_RenderedHTMLIsSeekable verifies that rendered HTML
// sources return seekable bodies. The S3 SDK requires Seek to determine
// Content-Length automatically; wrapping bytes.Reader in io.NopCloser would
// hide Seek and cause 411 MissingContentLength errors from R2.
func TestMarkdownRenderer_RenderedHTMLIsSeekable(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "doc.md"), []byte("# Hello"), 0o600))

	r := render.NewMarkdownRenderer()
	sources, _, err := r.Plan([]string{"doc.md"}, dir)
	require.NoError(t, err)

	var htmlSrc render.Source
	for _, src := range sources {
		if src.RelPath == "doc.html" {
			htmlSrc = src
			break
		}
	}
	require.NotEmpty(t, htmlSrc.RelPath, "expected doc.html source")

	rc, err := htmlSrc.Open()
	require.NoError(t, err)
	defer rc.Close() //nolint:errcheck

	first, err := io.ReadAll(rc)
	require.NoError(t, err)

	_, err = rc.Seek(0, io.SeekStart)
	require.NoError(t, err, "rendered HTML source must be seekable")

	second, err := io.ReadAll(rc)
	require.NoError(t, err)
	assert.Equal(t, first, second, "seek-and-reread must return identical content")
}

// TestMarkdownRenderer_RendersHTML verifies that a .md file produces a
// corresponding .html source and that both paths are in the result.
// Rendered HTML must not be written to disk.
func TestMarkdownRenderer_RendersHTML(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "notes.md"), []byte("# Hello\n\nWorld"), 0600))

	r := render.NewMarkdownRenderer()
	sources, _, err := r.Plan([]string{"notes.md"}, dir)
	require.NoError(t, err)

	got := sourceRelPaths(sources)
	assert.Contains(t, got, "notes.md")
	assert.Contains(t, got, "notes.html")

	html := openSource(t, sources, "notes.html")
	assert.Contains(t, html, "<h1")

	// rendered HTML must not be written to disk
	_, statErr := os.Stat(filepath.Join(dir, "notes.html"))
	assert.True(t, os.IsNotExist(statErr), "notes.html must not be written to disk")
}

// TestMarkdownRenderer_DarkModeSupport verifies the rendered HTML includes the
// color-scheme meta tag and a body style block that mirrors the dark/light
// backgrounds from github-markdown.css so the page chrome matches the content.
func TestMarkdownRenderer_DarkModeSupport(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "doc.md"), []byte("Hello"), 0o600))

	r := render.NewMarkdownRenderer()
	sources, _, err := r.Plan([]string{"doc.md"}, dir)
	require.NoError(t, err)

	html := openSource(t, sources, "doc.html")
	assert.Contains(t, html, `name="color-scheme" content="light dark"`)
	assert.Contains(t, html, `prefers-color-scheme: dark`)
	assert.Contains(t, html, `#0d1117`) // dark bg matching --bgColor-default
}

// TestMarkdownRenderer_NonMarkdownPassedThrough verifies non-.md files are
// included in the result without any additional paths.
func TestMarkdownRenderer_NonMarkdownPassedThrough(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "photo.jpg"), []byte("data"), 0600))

	r := render.NewMarkdownRenderer()
	sources, _, err := r.Plan([]string{"photo.jpg"}, dir)
	require.NoError(t, err)
	assert.Equal(t, []string{"photo.jpg"}, sourceRelPaths(sources))
}

// TestMarkdownRenderer_TitleFromFrontmatter verifies that a YAML frontmatter
// title field is used as the HTML <title>.
func TestMarkdownRenderer_TitleFromFrontmatter(t *testing.T) {
	dir := t.TempDir()
	md := "---\ntitle: My Custom Title\n---\n\n# Other Heading\n\nBody text."
	require.NoError(t, os.WriteFile(filepath.Join(dir, "doc.md"), []byte(md), 0o600))

	r := render.NewMarkdownRenderer()
	sources, _, err := r.Plan([]string{"doc.md"}, dir)
	require.NoError(t, err)

	html := openSource(t, sources, "doc.html")
	assert.Contains(t, html, "<title>My Custom Title</title>")
	assert.NotContains(t, html, "title: My Custom Title")
}

// TestMarkdownRenderer_TitleFromH1 verifies that when there is no frontmatter
// title, the first H1 text is used.
func TestMarkdownRenderer_TitleFromH1(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "doc.md"), []byte("# My H1 Title\n\nBody."), 0o600))

	r := render.NewMarkdownRenderer()
	sources, _, err := r.Plan([]string{"doc.md"}, dir)
	require.NoError(t, err)

	html := openSource(t, sources, "doc.html")
	assert.Contains(t, html, "<title>My H1 Title</title>")
}

// TestMarkdownRenderer_TitleFallbackToFilename verifies that when there is
// neither frontmatter nor H1, the stem of the filename is used.
func TestMarkdownRenderer_TitleFallbackToFilename(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "my-doc.md"), []byte("Just a paragraph."), 0o600))

	r := render.NewMarkdownRenderer()
	sources, _, err := r.Plan([]string{"my-doc.md"}, dir)
	require.NoError(t, err)

	html := openSource(t, sources, "my-doc.html")
	assert.Contains(t, html, "<title>my-doc</title>")
}

// TestMarkdownRenderer_OutputIsFullHTMLDocument verifies the rendered source
// is a complete HTML5 document with required structural tags and markdown-body div.
func TestMarkdownRenderer_OutputIsFullHTMLDocument(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "doc.md"), []byte("# Title\n\nParagraph."), 0o600))

	r := render.NewMarkdownRenderer()
	sources, _, err := r.Plan([]string{"doc.md"}, dir)
	require.NoError(t, err)

	html := openSource(t, sources, "doc.html")
	assert.Contains(t, html, "<!DOCTYPE html>")
	assert.Contains(t, html, "<html")
	assert.Contains(t, html, "<head>")
	assert.Contains(t, html, "<body>")
	assert.Contains(t, html, `class="markdown-body"`)
}

// TestMarkdownRenderer_SourceFooterLink verifies the rendered HTML includes a
// link back to the .md source file outside the markdown-body div.
func TestMarkdownRenderer_SourceFooterLink(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "notes.md"), []byte("Hello"), 0o600))

	r := render.NewMarkdownRenderer()
	sources, _, err := r.Plan([]string{"notes.md"}, dir)
	require.NoError(t, err)

	html := openSource(t, sources, "notes.html")
	assert.Contains(t, html, `href="notes.md"`)
	mdBodyClose := strings.Index(html, "</div>")
	footerLink := strings.Index(html, `href="notes.md"`)
	assert.Greater(t, footerLink, mdBodyClose, "source link should appear after markdown-body closing tag")
}

// TestMarkdownRenderer_Emoji renders :smile: shortcode as the emoji character.
func TestMarkdownRenderer_Emoji(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "doc.md"), []byte("Hello :smile:"), 0o600))

	r := render.NewMarkdownRenderer()
	sources, _, err := r.Plan([]string{"doc.md"}, dir)
	require.NoError(t, err)

	html := openSource(t, sources, "doc.html")
	assert.True(t,
		strings.Contains(html, "😄") || strings.Contains(html, "&#x1f604;"),
		"expected emoji character or HTML entity for :smile:",
	)
}

// TestMarkdownRenderer_HeadingAnchor verifies headings get an anchor attribute.
func TestMarkdownRenderer_HeadingAnchor(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "doc.md"), []byte("# My Heading"), 0o600))

	r := render.NewMarkdownRenderer()
	sources, _, err := r.Plan([]string{"doc.md"}, dir)
	require.NoError(t, err)

	html := openSource(t, sources, "doc.html")
	assert.Contains(t, html, `id="`)
}

// TestMarkdownRenderer_ScriptTagStripped verifies that a raw <script> tag in
// markdown input is removed from the rendered output.
func TestMarkdownRenderer_ScriptTagStripped(t *testing.T) {
	dir := t.TempDir()
	md := "Hello\n\n<script>alert('xss')</script>\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "doc.md"), []byte(md), 0o600))

	r := render.NewMarkdownRenderer()
	sources, _, err := r.Plan([]string{"doc.md"}, dir)
	require.NoError(t, err)

	html := openSource(t, sources, "doc.html")
	assert.NotContains(t, html, "<script>")
	assert.NotContains(t, html, "alert")
}

// TestMarkdownRenderer_DetailsPermitted verifies that <details>/<summary>
// survive the sanitization pass.
func TestMarkdownRenderer_DetailsPermitted(t *testing.T) {
	dir := t.TempDir()
	md := "<details><summary>Click</summary>Body</details>\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "doc.md"), []byte(md), 0o600))

	r := render.NewMarkdownRenderer()
	sources, _, err := r.Plan([]string{"doc.md"}, dir)
	require.NoError(t, err)

	html := openSource(t, sources, "doc.html")
	assert.Contains(t, html, "<details>")
	assert.Contains(t, html, "<summary>")
}

// TestMarkdownRenderer_TaskList renders GFM task list items as checkboxes.
func TestMarkdownRenderer_TaskList(t *testing.T) {
	dir := t.TempDir()
	md := "- [x] done\n- [ ] todo\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "doc.md"), []byte(md), 0o600))

	r := render.NewMarkdownRenderer()
	sources, _, err := r.Plan([]string{"doc.md"}, dir)
	require.NoError(t, err)

	html := openSource(t, sources, "doc.html")
	assert.Contains(t, html, `type="checkbox"`)
}

// TestMarkdownRenderer_Strikethrough renders ~~text~~ as <del>.
func TestMarkdownRenderer_Strikethrough(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "doc.md"), []byte("~~gone~~"), 0o600))

	r := render.NewMarkdownRenderer()
	sources, _, err := r.Plan([]string{"doc.md"}, dir)
	require.NoError(t, err)

	html := openSource(t, sources, "doc.html")
	assert.Contains(t, html, "<del>")
}

// TestMarkdownRenderer_Footnote renders a footnote reference and definition.
func TestMarkdownRenderer_Footnote(t *testing.T) {
	dir := t.TempDir()
	md := "Text[^1].\n\n[^1]: Note.\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "doc.md"), []byte(md), 0o600))

	r := render.NewMarkdownRenderer()
	sources, _, err := r.Plan([]string{"doc.md"}, dir)
	require.NoError(t, err)

	html := openSource(t, sources, "doc.html")
	assert.Contains(t, html, "footnote")
}

// TestMarkdownRenderer_MermaidFenceInjectsScript verifies that a file with a
// mermaid fenced code block gets a mermaid script tag in the rendered HTML and
// mermaid.min.js in the shared assets.
func TestMarkdownRenderer_MermaidFenceInjectsScript(t *testing.T) {
	dir := t.TempDir()
	md := "```mermaid\ngraph TD\n    A --> B\n```\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "doc.md"), []byte(md), 0o600))

	r := render.NewMarkdownRenderer()
	sources, assets, err := r.Plan([]string{"doc.md"}, dir)
	require.NoError(t, err)

	html := openSource(t, sources, "doc.html")
	assert.Contains(t, html, "mermaid.min.js")

	assetNames := make([]string, len(assets))
	for i, a := range assets {
		assetNames[i] = a.Name
	}
	assert.Contains(t, assetNames, "mermaid.min.js")
}

// TestMarkdownRenderer_NoMermaidFenceNoScript verifies that a file without a
// mermaid fence does not get a mermaid script tag.
func TestMarkdownRenderer_NoMermaidFenceNoScript(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "doc.md"), []byte("# Hello"), 0o600))

	r := render.NewMarkdownRenderer()
	sources, _, err := r.Plan([]string{"doc.md"}, dir)
	require.NoError(t, err)

	html := openSource(t, sources, "doc.html")
	assert.NotContains(t, html, "mermaid")
}

// TestMarkdownRenderer_AlertInsideFencedCodeNotConverted verifies that alert
// syntax inside a fenced code block is treated as literal code, not converted.
func TestMarkdownRenderer_AlertInsideFencedCodeNotConverted(t *testing.T) {
	dir := t.TempDir()
	md := "Example:\n\n```\n> [!NOTE]\n> body text\n```\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "doc.md"), []byte(md), 0o600))

	r := render.NewMarkdownRenderer()
	sources, _, err := r.Plan([]string{"doc.md"}, dir)
	require.NoError(t, err)

	html := openSource(t, sources, "doc.html")
	assert.NotContains(t, html, `class="markdown-alert"`, "alert syntax inside code fence must not be converted")
	assert.Contains(t, html, "[!NOTE]", "literal text must survive inside code fence")
}

// TestMarkdownRenderer_AlertNotOnFirstLineNotConverted verifies that a
// blockquote where [!TYPE] appears after the first line is NOT converted.
func TestMarkdownRenderer_AlertNotOnFirstLineNotConverted(t *testing.T) {
	dir := t.TempDir()
	md := "> Regular blockquote text.\n> [!NOTE]\n> This should not be an alert.\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "doc.md"), []byte(md), 0o600))

	r := render.NewMarkdownRenderer()
	sources, _, err := r.Plan([]string{"doc.md"}, dir)
	require.NoError(t, err)

	html := openSource(t, sources, "doc.html")
	assert.NotContains(t, html, `class="markdown-alert"`, "[!NOTE] not on first line must not become an alert")
	assert.Contains(t, html, "<blockquote>", "original blockquote must be preserved")
}

// TestMarkdownRenderer_AlertNote verifies that > [!NOTE] blockquotes are
// transformed into GitHub-style alert divs.
func TestMarkdownRenderer_AlertNote(t *testing.T) {
	dir := t.TempDir()
	md := "> [!NOTE]\n> This is a note.\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "doc.md"), []byte(md), 0o600))

	r := render.NewMarkdownRenderer()
	sources, _, err := r.Plan([]string{"doc.md"}, dir)
	require.NoError(t, err)

	html := openSource(t, sources, "doc.html")
	assert.Contains(t, html, `class="markdown-alert markdown-alert-note"`)
	assert.Contains(t, html, "This is a note.")
}

// TestMarkdownRenderer_AlertAllTypes verifies all five alert types produce the
// correct class name.
func TestMarkdownRenderer_AlertAllTypes(t *testing.T) {
	types := []string{"NOTE", "WARNING", "TIP", "IMPORTANT", "CAUTION"}
	for _, alertType := range types {
		t.Run(alertType, func(t *testing.T) {
			dir := t.TempDir()
			md := "> [!" + alertType + "]\n> body\n"
			require.NoError(t, os.WriteFile(filepath.Join(dir, "doc.md"), []byte(md), 0o600))

			r := render.NewMarkdownRenderer()
			sources, _, err := r.Plan([]string{"doc.md"}, dir)
			require.NoError(t, err)

			html := openSource(t, sources, "doc.html")
			lower := strings.ToLower(alertType)
			assert.Contains(t, html, `class="markdown-alert markdown-alert-`+lower+`"`)
		})
	}
}

// TestMarkdownRenderer_SyntaxHighlightingCSSClasses verifies that fenced code
// blocks produce CSS class-based highlighting (not inline styles).
func TestMarkdownRenderer_SyntaxHighlightingCSSClasses(t *testing.T) {
	dir := t.TempDir()
	md := "```go\nfmt.Println(\"hello\")\n```\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "doc.md"), []byte(md), 0o600))

	r := render.NewMarkdownRenderer()
	sources, _, err := r.Plan([]string{"doc.md"}, dir)
	require.NoError(t, err)

	html := openSource(t, sources, "doc.html")
	assert.Contains(t, html, `class="chroma"`)
	assert.NotContains(t, html, `style="color`)
}

// TestMarkdownRenderer_GFMTable renders a markdown table as an HTML table.
func TestMarkdownRenderer_GFMTable(t *testing.T) {
	dir := t.TempDir()
	md := "| Col1 | Col2 |\n|------|------|\n| A    | B    |\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "doc.md"), []byte(md), 0o600))

	r := render.NewMarkdownRenderer()
	sources, _, err := r.Plan([]string{"doc.md"}, dir)
	require.NoError(t, err)

	html := openSource(t, sources, "doc.html")
	assert.Contains(t, html, "<table>")
}

// TestMarkdownRenderer_InternalLinkRewritten verifies that a link to an .md
// file present in the batch is rewritten to .html in the rendered output.
func TestMarkdownRenderer_InternalLinkRewritten(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "index.md"), []byte("[see guide](guide.md)"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "guide.md"), []byte("# Guide"), 0o600))

	r := render.NewMarkdownRenderer()
	sources, _, err := r.Plan([]string{"index.md", "guide.md"}, dir)
	require.NoError(t, err)

	html := openSource(t, sources, "index.html")
	assert.Contains(t, html, `href="guide.html"`)
	assert.NotContains(t, html, `href="guide.md"`)
}

// TestMarkdownRenderer_ExternalLinkNotRewritten verifies that an external URL
// containing .md is not rewritten.
func TestMarkdownRenderer_ExternalLinkNotRewritten(t *testing.T) {
	dir := t.TempDir()
	md := "[ext](https://github.com/foo/bar/blob/main/README.md)"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "doc.md"), []byte(md), 0o600))

	r := render.NewMarkdownRenderer()
	sources, _, err := r.Plan([]string{"doc.md"}, dir)
	require.NoError(t, err)

	html := openSource(t, sources, "doc.html")
	assert.Contains(t, html, "README.md")
}

// TestMarkdownRenderer_FragmentPreservedOnRewrite verifies that fragment
// identifiers are kept when an internal link is rewritten.
func TestMarkdownRenderer_FragmentPreservedOnRewrite(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "doc.md"), []byte("[sec](other.md#section)"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "other.md"), []byte("# other"), 0o600))

	r := render.NewMarkdownRenderer()
	sources, _, err := r.Plan([]string{"doc.md", "other.md"}, dir)
	require.NoError(t, err)

	html := openSource(t, sources, "doc.html")
	assert.Contains(t, html, `href="other.html#section"`)
}

// TestMarkdownRenderer_NonBatchLinkNotRewritten verifies that a link to an .md
// file NOT in the batch is left unchanged.
func TestMarkdownRenderer_NonBatchLinkNotRewritten(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "doc.md"), []byte("[other](missing.md)"), 0o600))

	r := render.NewMarkdownRenderer()
	sources, _, err := r.Plan([]string{"doc.md"}, dir)
	require.NoError(t, err)

	html := openSource(t, sources, "doc.html")
	assert.Contains(t, html, `href="missing.md"`)
}

// TestMarkdownRenderer_CSSPathRootLevel verifies that a root-level file links
// to the CSS with no path prefix.
func TestMarkdownRenderer_CSSPathRootLevel(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "notes.md"), []byte("Hello"), 0o600))

	r := render.NewMarkdownRenderer()
	sources, _, err := r.Plan([]string{"notes.md"}, dir)
	require.NoError(t, err)

	html := openSource(t, sources, "notes.html")
	assert.Contains(t, html, `href="github-markdown.css"`)
}

// TestMarkdownRenderer_CSSPathOneLevelDeep verifies that a file one directory
// deep links to the CSS with a ../ prefix.
func TestMarkdownRenderer_CSSPathOneLevelDeep(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "sub"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "sub", "page.md"), []byte("Hello"), 0o600))

	r := render.NewMarkdownRenderer()
	sources, _, err := r.Plan([]string{"sub/page.md"}, dir)
	require.NoError(t, err)

	html := openSource(t, sources, "sub/page.html")
	assert.Contains(t, html, `href="../github-markdown.css"`)
}

// TestMarkdownRenderer_CollisionSkipsAndWarns verifies that when a .html file
// with the same stem already exists in the batch, the .md file is not rendered
// and a warning is written to stderr.
func TestMarkdownRenderer_CollisionSkipsAndWarns(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "notes.md"), []byte("# Hello"), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "notes.html"), []byte("<h1>existing</h1>"), 0600))

	var stderr bytes.Buffer
	r := render.NewMarkdownRendererWithStderr(&stderr)
	sources, _, err := r.Plan([]string{"notes.md", "notes.html"}, dir)
	require.NoError(t, err)

	relPaths := sourceRelPaths(sources)
	assert.Contains(t, relPaths, "notes.md")
	assert.Contains(t, relPaths, "notes.html")

	// existing notes.html source must be readable and unchanged
	html := openSource(t, sources, "notes.html")
	assert.Equal(t, "<h1>existing</h1>", html)

	assert.Contains(t, stderr.String(), "notes.html already present, skipping rendering of notes.md")
}

// TestMarkdownRenderer_CollisionStillUploadsCSS verifies that CSS shared
// assets are returned even when rendering was skipped due to a collision.
func TestMarkdownRenderer_CollisionStillUploadsCSS(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "notes.md"), []byte("# Hello"), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "notes.html"), []byte("<h1>existing</h1>"), 0600))

	r := render.NewMarkdownRenderer()
	_, assets, err := r.Plan([]string{"notes.md", "notes.html"}, dir)
	require.NoError(t, err)

	names := make([]string, len(assets))
	for i, a := range assets {
		names[i] = a.Name
	}
	assert.Contains(t, names, "github-markdown.css")
	assert.Contains(t, names, "highlight-github.css")
}
