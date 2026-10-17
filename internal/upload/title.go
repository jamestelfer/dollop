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
// <title> when no <h1> closes within titleReadLimit, or "" for neither. Text
// inside <svg> (icon titles and labels) is not part of either.
func htmlTitle(r io.Reader) string {
	z := html.NewTokenizer(io.LimitReader(r, titleReadLimit))
	var s titleScanner
	for {
		switch z.Next() {
		case html.ErrorToken:
			if s.inTitle {
				return "" // cut off by the cap
			}
			return normalizeTitle(s.title.String())
		case html.TextToken:
			s.text(z.Text())
		case html.StartTagToken:
			name, _ := z.TagName()
			s.start(string(name))
		case html.EndTagToken:
			name, _ := z.TagName()
			if t, ok := s.end(string(name)); ok {
				return t
			}
		}
	}
}

// titleScanner collects the text of the first <h1> and <title> outside <svg>.
type titleScanner struct {
	h1, title     strings.Builder
	inH1, inTitle bool
	svgDepth      int
}

func (s *titleScanner) text(b []byte) {
	switch {
	case s.svgDepth > 0:
	case s.inH1:
		s.h1.Write(b)
	case s.inTitle:
		s.title.Write(b)
	}
}

func (s *titleScanner) start(tag string) {
	switch tag {
	case "svg":
		s.svgDepth++
	case "h1":
		s.inH1 = true
		s.h1.Reset()
	case "title":
		s.inTitle = s.svgDepth == 0 && s.title.Len() == 0
	}
}

// end closes tag, returning the <h1> title once one with text closes.
func (s *titleScanner) end(tag string) (string, bool) {
	switch tag {
	case "svg":
		s.svgDepth = max(s.svgDepth-1, 0)
	case "h1":
		if t := normalizeTitle(s.h1.String()); t != "" {
			return t, true
		}
		s.inH1 = false
	case "title":
		s.inTitle = false
	}
	return "", false
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

// localTitles finds the titles of the pages among sources, keyed by relative
// path; pages without one are left out. A page rendered this run carries its
// title, an HTML page in fsys is read for its <h1>, and markdown left
// unrendered is titled by the rule rendered pages use.
func localTitles(fsys fs.FS, sources []render.Source) map[string]string {
	titles := make(map[string]string)
	for _, src := range sources {
		if title := normalizeTitle(src.Title); title != "" {
			titles[src.RelPath] = title
		}
	}
	for _, e := range buildIndexEntries(relPathsOf(sources), nil) {
		var title string
		switch {
		case titles[e.Name] != "", render.InAssetDir(e.Name):
			continue
		case isHTMLExt(e.Name):
			title = htmlFileTitle(fsys, e.Name)
		case isMarkdownPath(e.Name):
			title = markdownFileTitle(fsys, e.Name)
		}
		if title != "" {
			titles[e.Name] = title
		}
	}
	return titles
}

// htmlFileTitle returns the title of the HTML page at relPath in fsys, or ""
// when fsys does not hold it (the page is rendered this run).
func htmlFileTitle(fsys fs.FS, relPath string) string {
	f, err := fsys.Open(relPath)
	if err != nil {
		return ""
	}
	defer f.Close() //nolint:errcheck
	return htmlTitle(f)
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
