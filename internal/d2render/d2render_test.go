package d2render

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const sample = "web -> api -> db\napi -> cache\n"

func rootElement(t *testing.T, doc []byte) string {
	t.Helper()
	dec := xml.NewDecoder(bytes.NewReader(doc))
	for {
		tok, err := dec.Token()
		require.NoError(t, err)
		if start, ok := tok.(xml.StartElement); ok {
			return start.Name.Local
		}
	}
}

func TestRender_ProducesThemedSVG(t *testing.T) {
	d, err := Render(context.Background(), []byte(sample))
	require.NoError(t, err)

	assert.Equal(t, "svg", rootElement(t, d.SVG))
	assert.Positive(t, d.Width)
	assert.Positive(t, d.Height)
	assert.Contains(t, string(d.SVG), fmt.Sprintf(`viewBox="0 0 %d %d"`, d.Width, d.Height))
	assert.Contains(t, string(d.SVG), "prefers-color-scheme:dark")
	// the background rect makes the standalone SVG legible in both colour schemes
	assert.Contains(t, string(d.SVG), `class=" fill-N7"`)
}

func TestRender_UnknownLayoutEngine(t *testing.T) {
	src := "vars: {\n  d2-config: {\n    layout-engine: bogus\n  }\n}\na -> b\n"
	_, err := Render(context.Background(), []byte(src))
	require.ErrorContains(t, err, `unsupported layout engine "bogus"`)
}

func TestRender_SourceSelectsLayoutEngine(t *testing.T) {
	for _, engine := range []string{"elk", "dagre"} {
		t.Run(engine, func(t *testing.T) {
			src := "vars: {\n  d2-config: {\n    layout-engine: " + engine + "\n  }\n}\na -> b\n"
			d, err := Render(context.Background(), []byte(src))
			require.NoError(t, err)
			assert.Equal(t, "svg", rootElement(t, d.SVG))
		})
	}
}

func TestRender_IsDeterministic(t *testing.T) {
	first, err := Render(context.Background(), []byte(sample))
	require.NoError(t, err)
	second, err := Render(context.Background(), []byte(sample))
	require.NoError(t, err)
	assert.Equal(t, first.SVG, second.SVG)
}

func TestRender_SyntaxError(t *testing.T) {
	_, err := Render(context.Background(), []byte("a -> -> {"))
	require.Error(t, err)
}

func TestRender_EmptySource(t *testing.T) {
	d, err := Render(context.Background(), nil)
	require.NoError(t, err)
	assert.Equal(t, "svg", rootElement(t, d.SVG))
	assert.Positive(t, d.Width)
	assert.Positive(t, d.Height)
}

func TestRender_MultiBoardRendersRoot(t *testing.T) {
	src := "a -> b\nlayers: {\n  deep: {\n    c -> d\n  }\n}\n"
	d, err := Render(context.Background(), []byte(src))
	require.NoError(t, err)
	assert.Equal(t, "svg", rootElement(t, d.SVG))
}

// No filesystem is provided, so imports cannot resolve.
func TestRender_ImportFails(t *testing.T) {
	_, err := Render(context.Background(), []byte("...@other\na -> b\n"))
	require.ErrorContains(t, err, "failed to import")
}

// Publishing must make no network request, so remote icons stay references.
func TestRender_RemoteIconStaysReference(t *testing.T) {
	const url = "https://icons.example.invalid/picture.svg"
	src := "a: {\n  icon: " + url + "\n}\na -> b\n"
	d, err := Render(context.Background(), []byte(src))
	require.NoError(t, err)
	assert.Contains(t, string(d.SVG), url)
}

// The SVG opens as a standalone document on the bucket origin, so script must
// never reach it.
func TestRender_ScriptInLabelRejected(t *testing.T) {
	src := "a -> b: |md **hi** <script>alert(1)</script> |\n"
	_, err := Render(context.Background(), []byte(src))
	require.ErrorContains(t, err, "<script>")
}
