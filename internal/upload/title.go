package upload

import (
	"bytes"
	"io"
	"io/fs"
	"strings"

	"golang.org/x/net/html"

	"github.com/jamestelfer/dollop/internal/render"
)

// titleReadLimit bounds how much of a local file is read to find its title.
const titleReadLimit = 128 << 10

const maxTitleRunes = 120

// htmlTitle returns the text of the first <h1> in the HTML read from r, or the
// <title> when no <h1> closes within titleReadLimit, or "" for neither.
func htmlTitle(r io.Reader) string {
	z := html.NewTokenizer(io.LimitReader(r, titleReadLimit))
	var h1, title strings.Builder
	inH1, inTitle := false, false
	for {
		switch z.Next() {
		case html.ErrorToken:
			return normalizeTitle(title.String())
		case html.TextToken:
			switch {
			case inH1:
				h1.Write(z.Text())
			case inTitle:
				title.Write(z.Text())
			}
		case html.StartTagToken:
			switch name, _ := z.TagName(); string(name) {
			case "h1":
				inH1 = true
				h1.Reset()
			case "title":
				inTitle = title.Len() == 0
			}
		case html.EndTagToken:
			switch name, _ := z.TagName(); string(name) {
			case "h1":
				if t := normalizeTitle(h1.String()); t != "" {
					return t
				}
				inH1 = false
			case "title":
				inTitle = false
			}
		}
	}
}

// normalizeTitle collapses runs of whitespace in s and truncates it to
// maxTitleRunes, ending a truncated title with an ellipsis.
func normalizeTitle(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if runes := []rune(s); len(runes) > maxTitleRunes {
		s = string(runes[:maxTitleRunes-1]) + "…"
	}
	return s
}

// localTitles finds the titles of the pages among relPaths, keyed by relative
// path; pages without one are left out. An HTML page in fsys is read for its
// <h1>; one rendered this run takes the title of its markdown source, and
// markdown left unrendered is titled by the same rule.
func localTitles(fsys fs.FS, relPaths []string) map[string]string {
	titles := make(map[string]string)
	for _, e := range buildIndexEntries(relPaths, nil) {
		var title string
		switch {
		case render.InAssetDir(e.Name):
			continue
		case isHTMLExt(e.Name):
			var ok bool
			if title, ok = htmlFileTitle(fsys, e.Name); !ok && e.SourcePath != "" {
				title = markdownFileTitle(fsys, e.SourcePath)
			}
		case isMarkdownPath(e.Name):
			title = markdownFileTitle(fsys, e.Name)
		}
		if title != "" {
			titles[e.Name] = title
		}
	}
	return titles
}

// htmlFileTitle returns the title of the HTML page at relPath in fsys, and
// false when fsys does not hold it (the page is rendered this run).
func htmlFileTitle(fsys fs.FS, relPath string) (string, bool) {
	f, err := fsys.Open(relPath)
	if err != nil {
		return "", false
	}
	defer f.Close() //nolint:errcheck
	return htmlTitle(f), true
}

// markdownFileTitle returns the title of the markdown file at relPath in fsys
// from its first titleReadLimit bytes, or "" when it has none.
func markdownFileTitle(fsys fs.FS, relPath string) string {
	f, err := fsys.Open(relPath)
	if err != nil {
		return ""
	}
	defer f.Close() //nolint:errcheck
	src, err := io.ReadAll(io.LimitReader(f, titleReadLimit))
	if err != nil {
		return ""
	}
	if len(src) == titleReadLimit {
		// a line cut by the cap could read as a shorter heading
		src = src[:bytes.LastIndexByte(src, '\n')+1]
	}
	return normalizeTitle(render.MarkdownTitle(src))
}
