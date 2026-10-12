package upload

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// genIndex returns the generated index markdown for files and supporting under
// a fixed heading.
func genIndex(t *testing.T, files, supporting []string) string {
	t.Helper()
	b, err := generateIndexMarkdown(indexHeading{Title: "upload"}, files, supporting)
	require.NoError(t, err)
	return string(b)
}

func TestGenerateIndexMarkdown_PairsMarkdownSource(t *testing.T) {
	s := genIndex(t, []string{"notes.md", "notes.html", "data.json"}, nil)

	// the rendered HTML is a top-level entry with the markdown linked beside it
	assert.Contains(t, s, "- [notes.html](notes.html) [::md-icon::](notes.md)\n")
	assert.Equal(t, 1, strings.Count(s, "(notes.md)"), "markdown should appear exactly once, as the source link")
	// unrelated files remain plain entries
	assert.Contains(t, s, "- [data.json](data.json)\n")
}

func TestGenerateIndexMarkdown_PairsMarkdownSourceAnyCase(t *testing.T) {
	// the renderer writes NOTES.MD to NOTES.html
	s := genIndex(t, []string{"NOTES.MD", "NOTES.html", "b.Markdown", "b.html"}, nil)

	assert.Contains(t, s, "- [NOTES.html](NOTES.html) [::md-icon::](NOTES.MD)\n")
	assert.Contains(t, s, "- [b.html](b.html) [::md-icon::](b.Markdown)\n")
	assert.Contains(t, s, `<p class="index-meta">2 files · 2 pages</p>`)
}

func TestGenerateIndexMarkdown_HTMLWithoutSourceHasNoSourceLink(t *testing.T) {
	s := genIndex(t, []string{"page.html"}, nil)

	assert.Contains(t, s, "- [page.html](page.html)\n")
	assert.NotContains(t, s, "::md-icon::", "an HTML file with no markdown source must not get a source link")
}

func TestGenerateIndexMarkdown_MarkdownWithoutHTMLStaysTopLevel(t *testing.T) {
	s := genIndex(t, []string{"readme.md"}, nil)

	// when there is no rendered HTML, the .md is the entry itself
	assert.Contains(t, s, "- [readme.md](readme.md)\n")
	assert.NotContains(t, s, "::md-icon::")
}

func TestGenerateIndexMarkdown_SupportingFilesCollapsed(t *testing.T) {
	s := genIndex(t, []string{"a.html"}, []string{"github-markdown.css", "highlight-github.css"})

	assert.Contains(t, s, "<details>\n<summary>Supporting files</summary>\n\n"+
		"- [github-markdown.css](github-markdown.css)\n"+
		"- [highlight-github.css](highlight-github.css)\n\n</details>\n")
	assert.NotContains(t, s, "<details open", "supporting files are collapsed by default")
}

func TestGenerateIndexMarkdown_NoSupportingSectionWhenEmpty(t *testing.T) {
	s := genIndex(t, []string{"a.txt"}, nil)
	assert.NotContains(t, s, "Supporting files")
	assert.NotContains(t, s, "<details>")
}

func TestGenerateIndexMarkdown_EscapesNamesAndPaths(t *testing.T) {
	s := genIndex(t, []string{"my [draft] (v2)_*.txt", "sub dir/a:b.txt", "c:d.txt"}, nil)

	assert.Contains(t, s, `- [my \[draft\] \(v2\)\_\*.txt](my%20%5Bdraft%5D%20%28v2%29_%2A.txt)`)
	assert.Contains(t, s, "  - [a:b.txt](sub%20dir/a:b.txt)")
	// a leading segment with a colon must not read as a URL scheme
	assert.Contains(t, s, "- [c:d.txt](./c:d.txt)")
}

func TestGenerateIndexMarkdown_SourceIconEscapesPaths(t *testing.T) {
	s := genIndex(t, []string{"c:my [draft].md", "c:my [draft].html"}, nil)
	assert.Contains(t, s, `[::md-icon::](./c:my%20%5Bdraft%5D.md)`)
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

func TestGenerateIndexMarkdown_NestsDirectories(t *testing.T) {
	s := genIndex(t, []string{
		"top.txt",
		"zeta/z.txt",
		"alpha/a.txt",
		"alpha/deep/d.txt",
		"zz.txt",
	}, nil)

	// files come first at each level, in input order, then directories by
	// name; nested files are listed by base name under their directory.
	assert.Contains(t, s, "<div class=\"file-index\">\n\n"+
		"- [top.txt](top.txt)\n"+
		"- [zz.txt](zz.txt)\n"+
		"- <details>\n"+
		"  <summary>alpha/ <small>2 files</small></summary>\n\n"+
		"  - [a.txt](alpha/a.txt)\n"+
		"  - <details>\n"+
		"    <summary>deep/ <small>1 file</small></summary>\n\n"+
		"    - [d.txt](alpha/deep/d.txt)\n\n"+
		"    </details>\n\n"+
		"  </details>\n"+
		"- <details>\n"+
		"  <summary>zeta/ <small>1 file</small></summary>\n\n"+
		"  - [z.txt](zeta/z.txt)\n\n"+
		"  </details>\n\n"+
		"</div>\n")
}

func TestGenerateIndexMarkdown_OpensDirectoriesLeadingToPages(t *testing.T) {
	s := genIndex(t, []string{
		"docs/guide/intro.md", "docs/guide/intro.html",
		"docs/img/logo.png",
		"raw/notes.md",
		"data/x.json",
	}, nil)

	assert.Contains(t, s, "<details open>\n  <summary>docs/ ")
	assert.Contains(t, s, "<details open>\n    <summary>guide/ ")
	assert.Contains(t, s, "<details>\n    <summary>img/ ", "a directory with no pages below it starts collapsed")
	assert.Contains(t, s, "<details open>\n  <summary>raw/ ", "unrendered markdown counts as a page")
	assert.Contains(t, s, "<details>\n  <summary>data/ ")
}

func TestGenerateIndexMarkdown_EscapesDirectoryNames(t *testing.T) {
	s := genIndex(t, []string{"a<b>&c/f.txt"}, nil)
	assert.Contains(t, s, "<summary>a&lt;b&gt;&amp;c/ <small>")
	assert.Contains(t, s, "(a%3Cb%3E&c/f.txt)")
}

func TestGenerateIndexMarkdown_Heading(t *testing.T) {
	b, err := generateIndexMarkdown(
		indexHeading{Title: "q3_review: [draft]", Lifetime: "permanent"},
		[]string{"a.md", "a.html", "b.txt", "sub/c.html"}, nil)
	require.NoError(t, err)
	s := string(b)

	// the title is carried verbatim in frontmatter, and escaped in the heading
	assert.True(t, strings.HasPrefix(s, "---\ntitle: 'q3_review: [draft]'\n---\n\n"), s)
	assert.Contains(t, s, "\n# q3\\_review: \\[draft\\]\n")
	// the folded markdown source is not counted as a separate file
	assert.Contains(t, s, `<p class="index-meta">3 files · 2 pages · permanent</p>`)
	assert.NotContains(t, s, "# Files")
}

func TestGenerateIndexMarkdown_SummaryOmitsUnknowns(t *testing.T) {
	s := genIndex(t, []string{"a.txt"}, nil)
	assert.Contains(t, s, `<p class="index-meta">1 file</p>`)
}

func TestLifetimeOf(t *testing.T) {
	now := time.Date(2026, 10, 12, 23, 30, 0, 0, time.UTC)
	tests := []struct {
		prefix string
		want   string
	}{
		{"flash/1/abc", `expires <time datetime="2026-10-13">Tue 13 Oct 2026</time>`},
		{"flash/14/abc", `expires <time datetime="2026-10-26">Mon 26 Oct 2026</time>`},
		{"keep/hardy-emu.T9y.QbT.TDJ", "permanent"},
		{"flash/x/abc", ""},
		{"flash", ""},
		{"dollop/1/abc", ""},
		{"", ""},
	}
	for _, tc := range tests {
		t.Run(tc.prefix, func(t *testing.T) {
			assert.Equal(t, tc.want, lifetimeOf(tc.prefix, now))
		})
	}
}

func TestUploadTitle(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "q3-review")
	require.NoError(t, os.Mkdir(dir, 0o700))
	t.Chdir(dir)

	assert.Equal(t, "q3-review", uploadTitle("."), `"." names the directory itself`)
	assert.Equal(t, "q3-review", uploadTitle(dir+"/"))
	assert.Equal(t, "notes.md", uploadTitle("sub/notes.md"))
}
