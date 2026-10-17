package render_test

import (
	"html"
	"regexp"
	"testing"
	"testing/fstest"

	"github.com/jamestelfer/dollop/internal/render"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var titleTagRe = regexp.MustCompile(`<title>(.*?)</title>`)

// markdownTitleCases are run through both the renderer and MarkdownTitle so the
// listing title and the rendered page's <title> cannot drift apart.
var markdownTitleCases = []struct {
	name string
	src  string
	want string
}{
	{"frontmatter title wins", "---\ntitle: Release Notes\n---\n\n# Something else\n", "Release Notes"},
	{"atx heading", "intro\n\n# Getting started\n\nbody\n", "Getting started"},
	{"setext heading", "Getting started\n===============\n\nbody\n", "Getting started"},
	{"empty frontmatter title falls to heading", "---\ntitle: \"\"\n---\n# Heading\n", "Heading"},
	{"frontmatter without title", "---\nauthor: me\n---\n# Heading\n", "Heading"},
	{"entities decoded", "# Fish &amp; Chips &#35;1\n", "Fish & Chips #1"},
	{"inline markup flattened", "# Hello *world* and `code` [link](x.html)\n", "Hello world and code link"},
	{"backslash escapes resolved", "# \\*not emphasis\\*\n", "*not emphasis*"},
	{"multi-line setext heading", "Line one\nline two\n===\n", "Line one line two"},
	{"only lower headings", "## Section\n\ntext\n", ""},
	{"no heading", "just text\n", ""},
}

func TestMarkdownTitle(t *testing.T) {
	for _, tc := range markdownTitleCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, render.MarkdownTitle([]byte(tc.src)))
		})
	}
}

func TestMarkdownTitle_MatchesRenderedPageTitle(t *testing.T) {
	for _, tc := range markdownTitleCases {
		t.Run(tc.name, func(t *testing.T) {
			fsys := fstest.MapFS{"doc.md": {Data: []byte(tc.src)}}
			sources, _, err := render.NewMarkdownRenderer().Plan([]string{"doc.md"}, fsys, "flash/1/abc")
			require.NoError(t, err)

			m := titleTagRe.FindStringSubmatch(openSource(t, sources, "doc.html"))
			require.NotNil(t, m)
			want := tc.want
			if want == "" {
				want = "doc" // the renderer falls back to the file's base name
			}
			assert.Equal(t, want, html.UnescapeString(m[1]))
		})
	}
}
