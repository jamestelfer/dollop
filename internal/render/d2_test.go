package render_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jamestelfer/dollop/internal/d2render"
	"github.com/jamestelfer/dollop/internal/render"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeD2 counts its calls, fails for any source containing BROKEN, and
// otherwise returns a fixed-size diagram wrapping the source.
func fakeD2(calls *int) render.D2RenderFunc {
	return func(_ context.Context, src []byte) (d2render.Diagram, error) {
		*calls++
		if bytes.Contains(src, []byte("BROKEN")) {
			return d2render.Diagram{}, errors.New("1:1: bad thing\n2:1: worse thing")
		}
		return d2render.Diagram{SVG: []byte("<svg>" + string(src) + "</svg>"), Width: 640, Height: 412}, nil
	}
}

func d2Name(src string) string {
	sum := sha256.Sum256([]byte(src))
	return "d2/" + hex.EncodeToString(sum[:])[:12] + ".svg"
}

func d2Assets(assets []render.SharedAsset) []render.SharedAsset {
	var out []render.SharedAsset
	for _, a := range assets {
		if strings.HasPrefix(a.Name, "d2/") {
			out = append(out, a)
		}
	}
	return out
}

func writeFile(t *testing.T, dir, rel, content string) {
	t.Helper()
	p := filepath.Join(dir, filepath.FromSlash(rel))
	require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o750))
	require.NoError(t, os.WriteFile(p, []byte(content), 0o600))
}

func TestD2_FenceRendersLinkedImage(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "doc.md", "# Doc\n\n```d2\na -> b\n```\n")

	calls := 0
	r := render.NewMarkdownRendererWithD2(&bytes.Buffer{}, fakeD2(&calls))
	sources, assets, err := r.Plan([]string{"doc.md"}, rootFS(t, dir), "flash/1/testid")
	require.NoError(t, err)

	name := d2Name("a -> b\n")
	html := openSource(t, sources, "doc.html")
	assert.Contains(t, html, `<a class="d2" href="`+name+`">`)
	assert.Contains(t, html, `src="`+name+`"`)
	assert.Contains(t, html, `width="640"`)
	assert.Contains(t, html, `height="412"`)
	assert.Contains(t, html, `alt="d2 diagram"`)
	assert.Contains(t, html, `loading="lazy"`)
	assert.NotContains(t, html, "a -&gt; b", "the fence is no longer shown as code")

	got := d2Assets(assets)
	require.Len(t, got, 1)
	assert.Equal(t, name, got[0].Name)
	assert.Equal(t, "image/svg+xml; charset=utf-8", got[0].ContentType)
	assert.Equal(t, "<svg>a -> b\n</svg>", string(got[0].Content))
	assert.Equal(t, 1, calls)
}

func TestD2_SameDiagramInTwoFilesListedOnce(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "one.md", "```d2\na -> b\n```\n")
	writeFile(t, dir, "two.md", "```d2\na -> b\n```\n")

	calls := 0
	r := render.NewMarkdownRendererWithD2(&bytes.Buffer{}, fakeD2(&calls))
	_, assets, err := r.Plan([]string{"one.md", "two.md"}, rootFS(t, dir), "flash/1/testid")
	require.NoError(t, err)

	assert.Len(t, d2Assets(assets), 1)
}

func TestD2_FailingFenceFallsBackAndWarns(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "doc.md", "```d2\ngood -> one\n```\n\n```d2\nBROKEN\n```\n\n```d2\ngood -> two\n```\n")

	calls := 0
	var stderr bytes.Buffer
	r := render.NewMarkdownRendererWithD2(&stderr, fakeD2(&calls))
	sources, assets, err := r.Plan([]string{"doc.md"}, rootFS(t, dir), "flash/1/testid")
	require.NoError(t, err)

	html := openSource(t, sources, "doc.html")
	assert.Contains(t, html, `src="`+d2Name("good -> one\n")+`"`)
	assert.Contains(t, html, `src="`+d2Name("good -> two\n")+`"`)
	assert.Contains(t, html, "<pre", "the failed fence is shown as a code block")
	assert.Contains(t, html, "BROKEN")

	assert.Len(t, d2Assets(assets), 2)
	assert.Equal(t, "warning: doc.md: d2 diagram 2: 1:1: bad thing; 2:1: worse thing\n", stderr.String())
}

func TestD2_OpenDoesNotRenderAgain(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "doc.md", "```d2\na -> b\n```\n")

	calls := 0
	r := render.NewMarkdownRendererWithD2(&bytes.Buffer{}, fakeD2(&calls))
	sources, _, err := r.Plan([]string{"doc.md"}, rootFS(t, dir), "flash/1/testid")
	require.NoError(t, err)

	openSource(t, sources, "doc.html")
	openSource(t, sources, "doc.html")
	assert.Equal(t, 1, calls)
}

func TestD2_SanitiserKeepsMarkupIntact(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "doc.md", "```d2\na -> b\n```\n")

	calls := 0
	r := render.NewMarkdownRendererWithD2(&bytes.Buffer{}, fakeD2(&calls))
	sources, _, err := r.Plan([]string{"doc.md"}, rootFS(t, dir), "flash/1/testid")
	require.NoError(t, err)

	name := d2Name("a -> b\n")
	assert.Contains(t, openSource(t, sources, "doc.html"),
		`<a class="d2" href="`+name+`"><img src="`+name+`" width="640" height="412" alt="d2 diagram" loading="lazy"></a>`)
}

func TestD2_TildeFence(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "doc.md", "~~~d2\na -> b\n~~~\n")

	calls := 0
	r := render.NewMarkdownRendererWithD2(&bytes.Buffer{}, fakeD2(&calls))
	sources, _, err := r.Plan([]string{"doc.md"}, rootFS(t, dir), "flash/1/testid")
	require.NoError(t, err)

	assert.Contains(t, openSource(t, sources, "doc.html"), `src="`+d2Name("a -> b\n")+`"`)
}

func TestD2_NestedPageClimbsToPrefixRoot(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "sub/page.md", "```d2\na -> b\n```\n")

	calls := 0
	r := render.NewMarkdownRendererWithD2(&bytes.Buffer{}, fakeD2(&calls))
	sources, assets, err := r.Plan([]string{"sub/page.md"}, rootFS(t, dir), "flash/1/testid")
	require.NoError(t, err)

	name := d2Name("a -> b\n")
	html := openSource(t, sources, "sub/page.html")
	assert.Contains(t, html, `href="../`+name+`"`)
	assert.Contains(t, html, `src="../`+name+`"`)
	got := d2Assets(assets)
	require.Len(t, got, 1)
	assert.Equal(t, name, got[0].Name, "the asset is named relative to the prefix root")
}

func TestD2_EmptyFence(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "doc.md", "```d2\n```\n")

	calls := 0
	r := render.NewMarkdownRendererWithD2(&bytes.Buffer{}, fakeD2(&calls))
	sources, assets, err := r.Plan([]string{"doc.md"}, rootFS(t, dir), "flash/1/testid")
	require.NoError(t, err)

	assert.Contains(t, openSource(t, sources, "doc.html"), `src="`+d2Name("")+`"`)
	assert.Len(t, d2Assets(assets), 1)
}

func TestD2_FenceInsideListItem(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "doc.md", "- item one\n\n  ```d2\n  a -> b\n  ```\n\n- item two\n")

	calls := 0
	r := render.NewMarkdownRendererWithD2(&bytes.Buffer{}, fakeD2(&calls))
	sources, _, err := r.Plan([]string{"doc.md"}, rootFS(t, dir), "flash/1/testid")
	require.NoError(t, err)

	html := openSource(t, sources, "doc.html")
	li := strings.Index(html, "item one")
	img := strings.Index(html, `src="`+d2Name("a -> b\n")+`"`)
	end := strings.Index(html, "item two")
	require.NotEqual(t, -1, img, "diagram image missing")
	assert.True(t, li < img && img < end, "diagram must stay inside the first list item")
}

func TestD2_NoFenceMeansNoRenderAndNoAssets(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "doc.md", "# Doc\n\n```go\nfmt.Println(\"d2\")\n```\n")

	calls := 0
	r := render.NewMarkdownRendererWithD2(&bytes.Buffer{}, fakeD2(&calls))
	_, assets, err := r.Plan([]string{"doc.md"}, rootFS(t, dir), "flash/1/testid")
	require.NoError(t, err)

	assert.Empty(t, d2Assets(assets))
	assert.Zero(t, calls)
}

func TestD2_SkippedOnHTMLCollision(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "doc.md", "```d2\na -> b\n```\n")
	writeFile(t, dir, "doc.html", "<p>existing</p>")

	calls := 0
	r := render.NewMarkdownRendererWithD2(&bytes.Buffer{}, fakeD2(&calls))
	_, assets, err := r.Plan([]string{"doc.md", "doc.html"}, rootFS(t, dir), "flash/1/testid")
	require.NoError(t, err)

	assert.Zero(t, calls)
	assert.Empty(t, d2Assets(assets))
}

func TestD2_DefaultRendererUsesRealD2(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "doc.md", "```d2\na -> b\n```\n")

	r := render.NewMarkdownRenderer()
	_, assets, err := r.Plan([]string{"doc.md"}, rootFS(t, dir), "flash/1/testid")
	require.NoError(t, err)

	got := d2Assets(assets)
	require.Len(t, got, 1)
	assert.Contains(t, string(got[0].Content), "<svg")
}

func fixedSVG(svg string) render.D2RenderFunc {
	return func(_ context.Context, _ []byte) (d2render.Diagram, error) {
		return d2render.Diagram{SVG: []byte(svg), Width: 1, Height: 1}, nil
	}
}

func TestD2_AllowedLinksKeepDiagram(t *testing.T) {
	hrefs := []string{
		"other.html", "../up/page.html", "/root.html", "#frag", "",
		"https://example.com/x", "HTTP://example.com", "//example.com/x",
	}
	for _, href := range hrefs {
		t.Run(href, func(t *testing.T) {
			dir := t.TempDir()
			writeFile(t, dir, "doc.md", "```d2\na -> b\n```\n")

			svg := `<svg xmlns:xlink="http://www.w3.org/1999/xlink"><a href="` + href + `" xlink:href="` + href + `"></a><image href="` + href + `"/></svg>`
			var stderr bytes.Buffer
			r := render.NewMarkdownRendererWithD2(&stderr, fixedSVG(svg))
			_, assets, err := r.Plan([]string{"doc.md"}, rootFS(t, dir), "flash/1/testid")
			require.NoError(t, err)

			assert.Len(t, d2Assets(assets), 1)
			assert.Empty(t, stderr.String())
		})
	}
}

func TestD2_ImageDataURIKeepsDiagram(t *testing.T) {
	for _, href := range []string{"data:image/png;base64,iVBORw0KGgo=", "DATA:Image/svg+xml,&lt;svg/&gt;"} {
		t.Run(href, func(t *testing.T) {
			dir := t.TempDir()
			writeFile(t, dir, "doc.md", "```d2\na -> b\n```\n")

			var stderr bytes.Buffer
			r := render.NewMarkdownRendererWithD2(&stderr, fixedSVG(`<svg><image href="`+href+`"/></svg>`))
			_, assets, err := r.Plan([]string{"doc.md"}, rootFS(t, dir), "flash/1/testid")
			require.NoError(t, err)

			assert.Len(t, d2Assets(assets), 1)
			assert.Empty(t, stderr.String())
		})
	}
}

func TestD2_DisallowedLinksFallBackToCode(t *testing.T) {
	svgs := map[string]string{
		"javascript href":       `<svg><a href="javascript:alert(1)"></a></svg>`,
		"uppercase scheme":      `<svg><a href="JavaScript:alert(1)"></a></svg>`,
		"entity-encoded scheme": `<svg><a href="&#106;avascript:alert(1)"></a></svg>`,
		"tab in scheme":         `<svg><a href="java&#9;script:alert(1)"></a></svg>`,
		"leading space":         `<svg><a href=" javascript:alert(1)"></a></svg>`,
		"xlink href only":       `<svg xmlns:xlink="http://www.w3.org/1999/xlink"><a xlink:href="javascript:alert(1)"></a></svg>`,
		"data uri on link":      `<svg><a href="data:image/svg+xml,&lt;svg/&gt;"></a></svg>`,
		"non-image data uri":    `<svg><image href="data:text/html,&lt;script&gt;alert(1)&lt;/script&gt;"/></svg>`,
		"vbscript":              `<svg><a href="vbscript:msgbox"></a></svg>`,
		"malformed svg":         `<svg><a href="x"></svg>`,
	}
	for name, svg := range svgs {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			writeFile(t, dir, "doc.md", "```d2\na -> b\n```\n")

			var stderr bytes.Buffer
			r := render.NewMarkdownRendererWithD2(&stderr, fixedSVG(svg))
			sources, assets, err := r.Plan([]string{"doc.md"}, rootFS(t, dir), "flash/1/testid")
			require.NoError(t, err)

			assert.Empty(t, d2Assets(assets))
			assert.Contains(t, openSource(t, sources, "doc.html"), "<pre")
			assert.Contains(t, stderr.String(), "warning: doc.md: d2 diagram 1: ")
		})
	}
}

func TestD2_RealD2Links(t *testing.T) {
	cases := map[string]struct {
		src     string
		allowed bool
	}{
		"https link and icon": {"a: {link: https://example.com; icon: https://icons.terrastruct.com/essentials/087-display.svg}\n", true},
		"relative link":       {"a: {link: ./other.html}\n", true},
		"javascript link":     {"a: {link: \"javascript:alert(1)\"}\n", false},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			writeFile(t, dir, "doc.md", "```d2\n"+tc.src+"```\n")

			var stderr bytes.Buffer
			r := render.NewMarkdownRendererWithD2(&stderr, d2render.Render)
			_, assets, err := r.Plan([]string{"doc.md"}, rootFS(t, dir), "flash/1/testid")
			require.NoError(t, err)

			if tc.allowed {
				assert.Len(t, d2Assets(assets), 1)
				assert.Empty(t, stderr.String())
			} else {
				assert.Empty(t, d2Assets(assets))
				assert.Contains(t, stderr.String(), "only relative, http(s) and image data URLs are allowed")
			}
		})
	}
}
