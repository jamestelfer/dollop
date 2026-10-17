package render

import (
	"strings"

	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/util"
	"go.abhg.dev/goldmark/anchor"
)

// tocMinEntries is the fewest headings worth a table of contents; below it
// the page renders without one.
const tocMinEntries = 2

// tocEntry is one "On this page" link: a heading's id, its plain text, and its
// level (2–4).
type tocEntry struct {
	ID    string
	Text  string
	Depth int
}

// tableOfContents returns the h2–h4 headings of doc in document order, or nil
// when there are fewer than tocMinEntries. The h1 is the page title, and h5+
// are too fine-grained to navigate by.
func tableOfContents(doc ast.Node, src []byte) []tocEntry {
	var entries []tocEntry
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		h, ok := n.(*ast.Heading)
		if !ok {
			return ast.WalkContinue, nil
		}
		id, _ := h.AttributeString("id")
		idBytes, _ := id.([]byte)
		if h.Level >= 2 && h.Level <= 4 && len(idBytes) > 0 {
			entries = append(entries, tocEntry{ID: string(idBytes), Text: headingText(h, src), Depth: h.Level})
		}
		return ast.WalkSkipChildren, nil
	})
	if len(entries) < tocMinEntries {
		return nil
	}
	return entries
}

// headingText returns the plain text of a heading's inline content, as a
// reader sees it (escapes and entities resolved), leaving out the anchor link
// the anchor extension appends.
func headingText(h *ast.Heading, src []byte) string {
	var sb strings.Builder
	_ = ast.Walk(h, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch n := n.(type) {
		case *anchor.Node:
			return ast.WalkSkipChildren, nil
		case *ast.Text:
			v := n.Segment.Value(src)
			if !n.IsRaw() {
				v = util.ResolveEntityNames(util.ResolveNumericReferences(util.UnescapePunctuations(v)))
			}
			sb.Write(v)
			if n.SoftLineBreak() || n.HardLineBreak() {
				sb.WriteByte(' ')
			}
		case *ast.String:
			sb.Write(n.Value)
		}
		return ast.WalkContinue, nil
	})
	return strings.TrimSpace(sb.String())
}
