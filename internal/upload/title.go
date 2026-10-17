package upload

import (
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

// localTitles finds the titles of the HTML pages among relPaths that exist in
// fsys, keyed by relative path. Pages without a title are left out.
func localTitles(fsys fs.FS, relPaths []string) map[string]string {
	titles := make(map[string]string)
	for _, p := range relPaths {
		if !isHTMLExt(p) || render.InAssetDir(p) {
			continue
		}
		f, err := fsys.Open(p)
		if err != nil {
			continue
		}
		title := htmlTitle(f)
		f.Close() //nolint:errcheck
		if title != "" {
			titles[p] = title
		}
	}
	return titles
}
