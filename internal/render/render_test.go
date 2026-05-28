package render_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jamestelfer/dollop/internal/render"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNoopRenderer verifies that the no-op renderer returns its input unchanged
// and never errors.
func TestNoopRenderer_PassesThrough(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "notes.md"), []byte("# Hello"), 0600))

	r := render.NewNoopRenderer()
	got, err := r.Render([]string{"notes.md"}, dir)
	require.NoError(t, err)
	assert.Equal(t, []string{"notes.md"}, got)
}

// TestNoopRenderer_EmptySlice verifies no-op handles an empty input.
func TestNoopRenderer_EmptySlice(t *testing.T) {
	r := render.NewNoopRenderer()
	got, err := r.Render([]string{}, t.TempDir())
	require.NoError(t, err)
	assert.Equal(t, []string{}, got)
}

// TestMarkdownRenderer_RendersHTML verifies that a .md file produces a
// corresponding .html file on disk and that both paths are in the result.
func TestMarkdownRenderer_RendersHTML(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "notes.md"), []byte("# Hello\n\nWorld"), 0600))

	r := render.NewMarkdownRenderer()
	got, err := r.Render([]string{"notes.md"}, dir)
	require.NoError(t, err)

	assert.Contains(t, got, "notes.md")
	assert.Contains(t, got, "notes.html")

	// .html file must exist on disk
	htmlPath := filepath.Join(dir, "notes.html")
	_, statErr := os.Stat(htmlPath)
	require.NoError(t, statErr, "notes.html should have been written to disk")

	content, err := os.ReadFile(htmlPath)
	require.NoError(t, err)
	assert.Contains(t, string(content), "<h1")
}

// TestMarkdownRenderer_NonMarkdownPassedThrough verifies non-.md files are
// included in the result without any additional paths.
func TestMarkdownRenderer_NonMarkdownPassedThrough(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "photo.jpg"), []byte("data"), 0600))

	r := render.NewMarkdownRenderer()
	got, err := r.Render([]string{"photo.jpg"}, dir)
	require.NoError(t, err)
	assert.Equal(t, []string{"photo.jpg"}, got)
}

// TestMarkdownRenderer_TitleFromFrontmatter verifies that a YAML frontmatter
// title field is used as the HTML <title>.
func TestMarkdownRenderer_TitleFromFrontmatter(t *testing.T) {
	dir := t.TempDir()
	md := "---\ntitle: My Custom Title\n---\n\n# Other Heading\n\nBody text."
	require.NoError(t, os.WriteFile(filepath.Join(dir, "doc.md"), []byte(md), 0o600))

	r := render.NewMarkdownRenderer()
	_, err := r.Render([]string{"doc.md"}, dir)
	require.NoError(t, err)

	content, err := os.ReadFile(filepath.Join(dir, "doc.html"))
	require.NoError(t, err)
	assert.Contains(t, string(content), "<title>My Custom Title</title>")
	// frontmatter block must not appear in the rendered body
	assert.NotContains(t, string(content), "title: My Custom Title")
}

// TestMarkdownRenderer_TitleFromH1 verifies that when there is no frontmatter
// title, the first H1 text is used.
func TestMarkdownRenderer_TitleFromH1(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "doc.md"), []byte("# My H1 Title\n\nBody."), 0o600))

	r := render.NewMarkdownRenderer()
	_, err := r.Render([]string{"doc.md"}, dir)
	require.NoError(t, err)

	content, err := os.ReadFile(filepath.Join(dir, "doc.html"))
	require.NoError(t, err)
	assert.Contains(t, string(content), "<title>My H1 Title</title>")
}

// TestMarkdownRenderer_TitleFallbackToFilename verifies that when there is
// neither frontmatter nor H1, the stem of the filename is used.
func TestMarkdownRenderer_TitleFallbackToFilename(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "my-doc.md"), []byte("Just a paragraph."), 0o600))

	r := render.NewMarkdownRenderer()
	_, err := r.Render([]string{"my-doc.md"}, dir)
	require.NoError(t, err)

	content, err := os.ReadFile(filepath.Join(dir, "my-doc.html"))
	require.NoError(t, err)
	assert.Contains(t, string(content), "<title>my-doc</title>")
}

// TestMarkdownRenderer_OutputIsFullHTMLDocument verifies the rendered file is a
// complete HTML5 document with required structural tags and markdown-body div.
func TestMarkdownRenderer_OutputIsFullHTMLDocument(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "doc.md"), []byte("# Title\n\nParagraph."), 0o600))

	r := render.NewMarkdownRenderer()
	_, err := r.Render([]string{"doc.md"}, dir)
	require.NoError(t, err)

	content, err := os.ReadFile(filepath.Join(dir, "doc.html"))
	require.NoError(t, err)
	html := string(content)

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
	_, err := r.Render([]string{"notes.md"}, dir)
	require.NoError(t, err)

	content, err := os.ReadFile(filepath.Join(dir, "notes.html"))
	require.NoError(t, err)
	html := string(content)

	assert.Contains(t, html, `href="notes.md"`)
	// the link must appear outside the markdown-body div (after its closing tag)
	mdBodyClose := strings.Index(html, "</div>")
	footerLink := strings.Index(html, `href="notes.md"`)
	assert.Greater(t, footerLink, mdBodyClose, "source link should appear after markdown-body closing tag")
}

// TestMarkdownRenderer_Emoji renders :smile: shortcode as the emoji character.
func TestMarkdownRenderer_Emoji(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "doc.md"), []byte("Hello :smile:"), 0o600))

	r := render.NewMarkdownRenderer()
	_, err := r.Render([]string{"doc.md"}, dir)
	require.NoError(t, err)

	content, err := os.ReadFile(filepath.Join(dir, "doc.html"))
	require.NoError(t, err)
	// :smile: maps to 😄 (U+1F604); goldmark emits the HTML entity form
	assert.True(t,
		strings.Contains(string(content), "😄") || strings.Contains(string(content), "&#x1f604;"),
		"expected emoji character or HTML entity for :smile:",
	)
}

// TestMarkdownRenderer_HeadingAnchor verifies headings get an anchor attribute.
func TestMarkdownRenderer_HeadingAnchor(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "doc.md"), []byte("# My Heading"), 0o600))

	r := render.NewMarkdownRenderer()
	_, err := r.Render([]string{"doc.md"}, dir)
	require.NoError(t, err)

	content, err := os.ReadFile(filepath.Join(dir, "doc.html"))
	require.NoError(t, err)
	// goldmark-anchor adds id attributes to headings
	assert.Contains(t, string(content), `id="`)
}

// TestMarkdownRenderer_ScriptTagStripped verifies that a raw <script> tag in
// markdown input is removed from the rendered output.
func TestMarkdownRenderer_ScriptTagStripped(t *testing.T) {
	dir := t.TempDir()
	md := "Hello\n\n<script>alert('xss')</script>\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "doc.md"), []byte(md), 0o600))

	r := render.NewMarkdownRenderer()
	_, err := r.Render([]string{"doc.md"}, dir)
	require.NoError(t, err)

	content, err := os.ReadFile(filepath.Join(dir, "doc.html"))
	require.NoError(t, err)
	assert.NotContains(t, string(content), "<script>")
	assert.NotContains(t, string(content), "alert")
}

// TestMarkdownRenderer_DetailsPermitted verifies that <details>/<summary>
// survive the sanitization pass.
func TestMarkdownRenderer_DetailsPermitted(t *testing.T) {
	dir := t.TempDir()
	md := "<details><summary>Click</summary>Body</details>\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "doc.md"), []byte(md), 0o600))

	r := render.NewMarkdownRenderer()
	_, err := r.Render([]string{"doc.md"}, dir)
	require.NoError(t, err)

	content, err := os.ReadFile(filepath.Join(dir, "doc.html"))
	require.NoError(t, err)
	assert.Contains(t, string(content), "<details>")
	assert.Contains(t, string(content), "<summary>")
}

// TestMarkdownRenderer_TaskList renders GFM task list items as checkboxes.
func TestMarkdownRenderer_TaskList(t *testing.T) {
	dir := t.TempDir()
	md := "- [x] done\n- [ ] todo\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "doc.md"), []byte(md), 0o600))

	r := render.NewMarkdownRenderer()
	_, err := r.Render([]string{"doc.md"}, dir)
	require.NoError(t, err)

	content, err := os.ReadFile(filepath.Join(dir, "doc.html"))
	require.NoError(t, err)
	assert.Contains(t, string(content), `type="checkbox"`)
}

// TestMarkdownRenderer_Strikethrough renders ~~text~~ as <del>.
func TestMarkdownRenderer_Strikethrough(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "doc.md"), []byte("~~gone~~"), 0o600))

	r := render.NewMarkdownRenderer()
	_, err := r.Render([]string{"doc.md"}, dir)
	require.NoError(t, err)

	content, err := os.ReadFile(filepath.Join(dir, "doc.html"))
	require.NoError(t, err)
	assert.Contains(t, string(content), "<del>")
}

// TestMarkdownRenderer_Footnote renders a footnote reference and definition.
func TestMarkdownRenderer_Footnote(t *testing.T) {
	dir := t.TempDir()
	md := "Text[^1].\n\n[^1]: Note.\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "doc.md"), []byte(md), 0o600))

	r := render.NewMarkdownRenderer()
	_, err := r.Render([]string{"doc.md"}, dir)
	require.NoError(t, err)

	content, err := os.ReadFile(filepath.Join(dir, "doc.html"))
	require.NoError(t, err)
	assert.Contains(t, string(content), "footnote")
}

// TestMarkdownRenderer_MermaidFenceInjectsScript verifies that a file with a
// mermaid fenced code block gets a mermaid script tag in the rendered HTML.
func TestMarkdownRenderer_MermaidFenceInjectsScript(t *testing.T) {
	dir := t.TempDir()
	md := "```mermaid\ngraph TD\n    A --> B\n```\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "doc.md"), []byte(md), 0o600))

	r := render.NewMarkdownRenderer()
	_, err := r.Render([]string{"doc.md"}, dir)
	require.NoError(t, err)

	content, err := os.ReadFile(filepath.Join(dir, "doc.html"))
	require.NoError(t, err)
	assert.Contains(t, string(content), "mermaid.min.js")
}

// TestMarkdownRenderer_NoMermaidFenceNoScript verifies that a file without a
// mermaid fence does not get a mermaid script tag.
func TestMarkdownRenderer_NoMermaidFenceNoScript(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "doc.md"), []byte("# Hello"), 0o600))

	r := render.NewMarkdownRenderer()
	_, err := r.Render([]string{"doc.md"}, dir)
	require.NoError(t, err)

	content, err := os.ReadFile(filepath.Join(dir, "doc.html"))
	require.NoError(t, err)
	assert.NotContains(t, string(content), "mermaid")
}

// TestMarkdownRenderer_AlertInsideFencedCodeNotConverted verifies that alert
// syntax inside a fenced code block is treated as literal code, not converted.
// The regex pre-processor fails this because it runs on raw bytes before parsing.
func TestMarkdownRenderer_AlertInsideFencedCodeNotConverted(t *testing.T) {
	dir := t.TempDir()
	md := "Example:\n\n```\n> [!NOTE]\n> body text\n```\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "doc.md"), []byte(md), 0o600))

	r := render.NewMarkdownRenderer()
	_, err := r.Render([]string{"doc.md"}, dir)
	require.NoError(t, err)

	content, err := os.ReadFile(filepath.Join(dir, "doc.html"))
	require.NoError(t, err)
	assert.NotContains(t, string(content), `class="markdown-alert"`, "alert syntax inside code fence must not be converted")
	assert.Contains(t, string(content), "[!NOTE]", "literal text must survive inside code fence")
}

// TestMarkdownRenderer_AlertNotOnFirstLineNotConverted verifies that a
// blockquote where [!TYPE] appears after the first line is NOT converted.
// The regex pre-processor fails this: its leading (> ...\n)* prefix allows
// it to match mid-blockquote markers.
func TestMarkdownRenderer_AlertNotOnFirstLineNotConverted(t *testing.T) {
	dir := t.TempDir()
	md := "> Regular blockquote text.\n> [!NOTE]\n> This should not be an alert.\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "doc.md"), []byte(md), 0o600))

	r := render.NewMarkdownRenderer()
	_, err := r.Render([]string{"doc.md"}, dir)
	require.NoError(t, err)

	content, err := os.ReadFile(filepath.Join(dir, "doc.html"))
	require.NoError(t, err)
	assert.NotContains(t, string(content), `class="markdown-alert"`, "[!NOTE] not on first line must not become an alert")
	assert.Contains(t, string(content), "<blockquote>", "original blockquote must be preserved")
}

// TestMarkdownRenderer_AlertNote verifies that > [!NOTE] blockquotes are
// transformed into GitHub-style alert divs.
func TestMarkdownRenderer_AlertNote(t *testing.T) {
	dir := t.TempDir()
	md := "> [!NOTE]\n> This is a note.\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "doc.md"), []byte(md), 0o600))

	r := render.NewMarkdownRenderer()
	_, err := r.Render([]string{"doc.md"}, dir)
	require.NoError(t, err)

	content, err := os.ReadFile(filepath.Join(dir, "doc.html"))
	require.NoError(t, err)
	html := string(content)

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
			_, err := r.Render([]string{"doc.md"}, dir)
			require.NoError(t, err)

			content, err := os.ReadFile(filepath.Join(dir, "doc.html"))
			require.NoError(t, err)
			lower := strings.ToLower(alertType)
			assert.Contains(t, string(content), `class="markdown-alert markdown-alert-`+lower+`"`)
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
	_, err := r.Render([]string{"doc.md"}, dir)
	require.NoError(t, err)

	content, err := os.ReadFile(filepath.Join(dir, "doc.html"))
	require.NoError(t, err)
	html := string(content)

	// Chroma CSS-class mode emits class="chroma" container
	assert.Contains(t, html, `class="chroma"`)
	// must not use inline style attributes for syntax colours
	assert.NotContains(t, html, `style="color`)
}

// TestMarkdownRenderer_GFMTable renders a markdown table as an HTML table.
func TestMarkdownRenderer_GFMTable(t *testing.T) {
	dir := t.TempDir()
	md := "| Col1 | Col2 |\n|------|------|\n| A    | B    |\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "doc.md"), []byte(md), 0o600))

	r := render.NewMarkdownRenderer()
	_, err := r.Render([]string{"doc.md"}, dir)
	require.NoError(t, err)

	content, err := os.ReadFile(filepath.Join(dir, "doc.html"))
	require.NoError(t, err)
	assert.Contains(t, string(content), "<table>")
}

// TestMarkdownRenderer_InternalLinkRewritten verifies that a link to an .md
// file present in the batch is rewritten to .html in the rendered output.
func TestMarkdownRenderer_InternalLinkRewritten(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "index.md"), []byte("[see guide](guide.md)"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "guide.md"), []byte("# Guide"), 0o600))

	r := render.NewMarkdownRenderer()
	_, err := r.Render([]string{"index.md", "guide.md"}, dir)
	require.NoError(t, err)

	content, err := os.ReadFile(filepath.Join(dir, "index.html"))
	require.NoError(t, err)
	assert.Contains(t, string(content), `href="guide.html"`)
	assert.NotContains(t, string(content), `href="guide.md"`)
}

// TestMarkdownRenderer_ExternalLinkNotRewritten verifies that an external URL
// containing .md is not rewritten.
func TestMarkdownRenderer_ExternalLinkNotRewritten(t *testing.T) {
	dir := t.TempDir()
	md := "[ext](https://github.com/foo/bar/blob/main/README.md)"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "doc.md"), []byte(md), 0o600))

	r := render.NewMarkdownRenderer()
	_, err := r.Render([]string{"doc.md"}, dir)
	require.NoError(t, err)

	content, err := os.ReadFile(filepath.Join(dir, "doc.html"))
	require.NoError(t, err)
	assert.Contains(t, string(content), "README.md")
}

// TestMarkdownRenderer_FragmentPreservedOnRewrite verifies that fragment
// identifiers are kept when an internal link is rewritten.
func TestMarkdownRenderer_FragmentPreservedOnRewrite(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "doc.md"), []byte("[sec](other.md#section)"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "other.md"), []byte("# other"), 0o600))

	r := render.NewMarkdownRenderer()
	_, err := r.Render([]string{"doc.md", "other.md"}, dir)
	require.NoError(t, err)

	content, err := os.ReadFile(filepath.Join(dir, "doc.html"))
	require.NoError(t, err)
	assert.Contains(t, string(content), `href="other.html#section"`)
}

// TestMarkdownRenderer_NonBatchLinkNotRewritten verifies that a link to an .md
// file NOT in the batch is left unchanged.
func TestMarkdownRenderer_NonBatchLinkNotRewritten(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "doc.md"), []byte("[other](missing.md)"), 0o600))

	r := render.NewMarkdownRenderer()
	_, err := r.Render([]string{"doc.md"}, dir)
	require.NoError(t, err)

	content, err := os.ReadFile(filepath.Join(dir, "doc.html"))
	require.NoError(t, err)
	assert.Contains(t, string(content), `href="missing.md"`)
}

// TestMarkdownRenderer_CSSPathRootLevel verifies that a root-level file links
// to the CSS with no path prefix.
func TestMarkdownRenderer_CSSPathRootLevel(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "notes.md"), []byte("Hello"), 0o600))

	r := render.NewMarkdownRenderer()
	_, err := r.Render([]string{"notes.md"}, dir)
	require.NoError(t, err)

	content, err := os.ReadFile(filepath.Join(dir, "notes.html"))
	require.NoError(t, err)
	assert.Contains(t, string(content), `href="github-markdown.css"`)
}

// TestMarkdownRenderer_CSSPathOneLevelDeep verifies that a file one directory
// deep links to the CSS with a ../ prefix.
func TestMarkdownRenderer_CSSPathOneLevelDeep(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "sub"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "sub", "page.md"), []byte("Hello"), 0o600))

	r := render.NewMarkdownRenderer()
	_, err := r.Render([]string{"sub/page.md"}, dir)
	require.NoError(t, err)

	content, err := os.ReadFile(filepath.Join(dir, "sub", "page.html"))
	require.NoError(t, err)
	assert.Contains(t, string(content), `href="../github-markdown.css"`)
}

// TestMarkdownRenderer_CollisionSkipsAndWarns verifies that when a .html file
// with the same stem already exists on disk, the .md file is not rendered and
// a warning is written to stderr.
func TestMarkdownRenderer_CollisionSkipsAndWarns(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "notes.md"), []byte("# Hello"), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "notes.html"), []byte("<h1>existing</h1>"), 0600))

	var stderr bytes.Buffer
	r := render.NewMarkdownRendererWithStderr(&stderr)
	got, err := r.Render([]string{"notes.md", "notes.html"}, dir)
	require.NoError(t, err)

	// both paths still appear (existing html is kept)
	assert.Contains(t, got, "notes.md")
	assert.Contains(t, got, "notes.html")
	// but it was not re-rendered — the existing content is intact
	htmlContent, err := os.ReadFile(filepath.Join(dir, "notes.html"))
	require.NoError(t, err)
	assert.Equal(t, "<h1>existing</h1>", string(htmlContent))

	assert.Contains(t, stderr.String(), "notes.html already present, skipping rendering of notes.md")
}

func TestMarkdownRenderer_CollisionStillUploadsCSS(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "notes.md"), []byte("# Hello"), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "notes.html"), []byte("<h1>existing</h1>"), 0600))

	r := render.NewMarkdownRenderer()
	_, err := r.Render([]string{"notes.md", "notes.html"}, dir)
	require.NoError(t, err)

	// CSS must be returned even though rendering was skipped — the existing HTML
	// references the stylesheet and it needs to be uploaded alongside it.
	assets := r.SharedAssets()
	names := make([]string, len(assets))
	for i, a := range assets {
		names[i] = a.Name
	}
	assert.Contains(t, names, "github-markdown.css")
	assert.Contains(t, names, "highlight-github.css")
}
