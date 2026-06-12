package render

import (
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

// mermaidKind is the unique NodeKind for mermaid diagram nodes.
var mermaidKind = ast.NewNodeKind("Mermaid")

// mermaidNode holds the verbatim source of a ```mermaid fenced code block.
type mermaidNode struct {
	ast.BaseBlock
	Source []byte
}

func (n *mermaidNode) Kind() ast.NodeKind { return mermaidKind }

func (n *mermaidNode) Dump(source []byte, level int) {
	ast.DumpHelper(n, source, level, nil, nil)
}

// mermaidExtension wires mermaidTransformer and mermaidNodeRenderer into a
// goldmark instance so that mermaid fences render as <pre class="mermaid">
// elements that mermaid.js can discover, instead of highlighted code blocks.
type mermaidExtension struct{}

func (e *mermaidExtension) Extend(m goldmark.Markdown) {
	m.Parser().AddOptions(
		parser.WithASTTransformers(util.Prioritized(&mermaidTransformer{}, 100)),
	)
	m.Renderer().AddOptions(
		renderer.WithNodeRenderers(util.Prioritized(&mermaidNodeRenderer{}, 100)),
	)
}

// mermaidTransformer replaces fenced code blocks with language "mermaid" by a
// mermaidNode carrying the raw diagram source. Running after parsing, it leaves
// every other fenced code block to the highlighting renderer untouched.
type mermaidTransformer struct{}

func (t *mermaidTransformer) Transform(doc *ast.Document, reader text.Reader, _ parser.Context) {
	src := reader.Source()

	type replacement struct{ old, new ast.Node }
	var replacements []replacement

	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		cb, ok := n.(*ast.FencedCodeBlock)
		if !ok || string(cb.Language(src)) != "mermaid" {
			return ast.WalkContinue, nil
		}
		replacements = append(replacements, replacement{cb, &mermaidNode{Source: codeBlockText(cb, src)}})
		return ast.WalkSkipChildren, nil
	})

	for _, r := range replacements {
		r.old.Parent().ReplaceChild(r.old.Parent(), r.old, r.new)
	}
}

// codeBlockText returns the verbatim text inside a fenced code block.
func codeBlockText(cb *ast.FencedCodeBlock, src []byte) []byte {
	lines := cb.Lines()
	var out []byte
	for i := 0; i < lines.Len(); i++ {
		seg := lines.At(i)
		out = append(out, seg.Value(src)...)
	}
	return out
}

// mermaidNodeRenderer renders mermaidNode as a <pre class="mermaid"> block with
// the diagram source escaped as text content, which is what mermaid.js parses.
type mermaidNodeRenderer struct{}

func (r *mermaidNodeRenderer) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	reg.Register(mermaidKind, r.render)
}

func (r *mermaidNodeRenderer) render(w util.BufWriter, _ []byte, n ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkContinue, nil
	}
	node := n.(*mermaidNode)
	_, _ = w.WriteString(`<pre class="mermaid">`)
	_, _ = w.Write(util.EscapeHTML(node.Source))
	_, _ = w.WriteString("</pre>\n")
	return ast.WalkSkipChildren, nil
}
