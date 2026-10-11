package render

import (
	"bytes"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

// Only this fixed, application-owned SVG is inserted after sanitization. Raw
// SVG from Markdown remains disallowed; no user attributes enter the SVG.
const markdownIconPlaceholder = `<span class="dollop-md-icon-placeholder"></span>`
const markdownIconHTML = `<svg class="md-icon" xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" role="img" aria-label="Markdown source"><use href="#dollop-md-icon" /></svg>`

var markdownIconKind = ast.NewNodeKind("MarkdownIcon")

type markdownIconNode struct{ ast.BaseInline }

func (n *markdownIconNode) Kind() ast.NodeKind { return markdownIconKind }
func (n *markdownIconNode) Dump(source []byte, level int) {
	ast.DumpHelper(n, source, level, nil, nil)
}

type markdownIconExtension struct{}

func (e *markdownIconExtension) Extend(m goldmark.Markdown) {
	m.Parser().AddOptions(parser.WithInlineParsers(util.Prioritized(&markdownIconParser{}, 150)))
	m.Renderer().AddOptions(renderer.WithNodeRenderers(util.Prioritized(&markdownIconRenderer{}, 100)))
}

type markdownIconParser struct{}

func (p *markdownIconParser) Trigger() []byte { return []byte{':'} }
func (p *markdownIconParser) Parse(_ ast.Node, reader text.Reader, _ parser.Context) ast.Node {
	const marker = "::md-icon::"
	line, _ := reader.PeekLine()
	if !bytes.HasPrefix(line, []byte(marker)) {
		return nil
	}
	reader.Advance(len(marker))
	return &markdownIconNode{}
}

type markdownIconRenderer struct{}

func (r *markdownIconRenderer) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	reg.Register(markdownIconKind, r.render)
}
func (r *markdownIconRenderer) render(w util.BufWriter, _ []byte, _ ast.Node, entering bool) (ast.WalkStatus, error) {
	if entering {
		_, err := w.WriteString(markdownIconPlaceholder)
		return ast.WalkContinue, err
	}
	return ast.WalkContinue, nil
}

func sanitizeMarkdownHTML(body string) string {
	return strings.ReplaceAll(sanitizeHTML(body), markdownIconPlaceholder, markdownIconHTML)
}

// Icon-labelled links deliberately point to Markdown source, not its rendered
// counterpart. Other Markdown links retain the usual .md → .html rewriting.
func isMarkdownIconLink(link *ast.Link) bool {
	return link.FirstChild() != nil && link.FirstChild().Kind() == markdownIconKind && link.FirstChild().NextSibling() == nil
}
