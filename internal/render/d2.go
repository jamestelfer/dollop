package render

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"

	"github.com/jamestelfer/dollop/internal/d2render"
)

// D2RenderFunc renders d2 diagram source to SVG.
type D2RenderFunc func(ctx context.Context, src []byte) (d2render.Diagram, error)

// d2Kind is the unique NodeKind for rendered d2 diagram nodes.
var d2Kind = ast.NewNodeKind("D2")

// d2Node replaces a ```d2 fenced code block that rendered successfully.
type d2Node struct {
	ast.BaseBlock
	Src           string // page-relative URL of the diagram's SVG
	Width, Height int
}

func (n *d2Node) Kind() ast.NodeKind { return d2Kind }

func (n *d2Node) Dump(source []byte, level int) {
	ast.DumpHelper(n, source, level, nil, nil)
}

// d2Extension registers only the node renderer. Fences are replaced by
// d2Rewriter, which carries per-file state and so cannot live on mdParser.
type d2Extension struct{}

func (e *d2Extension) Extend(m goldmark.Markdown) {
	m.Renderer().AddOptions(
		renderer.WithNodeRenderers(util.Prioritized(&d2NodeRenderer{}, 100)),
	)
}

// d2NodeRenderer renders d2Node as an image linked to its own SVG, so the
// diagram can be opened standalone and explored with the browser's zoom.
type d2NodeRenderer struct{}

func (r *d2NodeRenderer) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	reg.Register(d2Kind, r.render)
}

func (r *d2NodeRenderer) render(w util.BufWriter, _ []byte, n ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkContinue, nil
	}
	node := n.(*d2Node)
	src := util.EscapeHTML([]byte(node.Src))
	_, _ = fmt.Fprintf(w, `<p><a class="d2" href="%s"><img src="%s" width="%d" height="%d" alt="d2 diagram" loading="lazy"></a></p>`+"\n",
		src, src, node.Width, node.Height)
	return ast.WalkSkipChildren, nil
}

// d2Rewriter renders the d2 fences of one markdown file in document order,
// replacing each with a d2Node and collecting its SVG as an asset. A fence that
// fails to render is left in place, so it shows as an ordinary code block.
type d2Rewriter struct {
	render      D2RenderFunc
	stderr      io.Writer
	relPath     string
	depthPrefix string // climbs from the page to the prefix root
	assets      []SharedAsset
}

func (dr *d2Rewriter) Transform(doc *ast.Document, reader text.Reader, _ parser.Context) {
	src := reader.Source()

	type replacement struct{ old, new ast.Node }
	var replacements []replacement
	index := 0

	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		cb, ok := n.(*ast.FencedCodeBlock)
		if !ok || string(cb.Language(src)) != "d2" {
			return ast.WalkContinue, nil
		}
		index++

		code := codeBlockText(cb, src)
		diagram, err := dr.render(context.Background(), code)
		if err != nil {
			// d2 reports one problem per line; keep the warning to one line
			msg := strings.ReplaceAll(strings.TrimSpace(err.Error()), "\n", "; ")
			fmt.Fprintf(dr.stderr, "warning: %s: d2 diagram %d: %s\n", dr.relPath, index, msg) //nolint:errcheck
			return ast.WalkSkipChildren, nil
		}

		name := d2AssetName(code)
		dr.assets = append(dr.assets, SharedAsset{
			Name:        name,
			ContentType: "image/svg+xml; charset=utf-8",
			Content:     diagram.SVG,
		})
		replacements = append(replacements, replacement{cb, &d2Node{
			Src:    dr.depthPrefix + name,
			Width:  diagram.Width,
			Height: diagram.Height,
		}})
		return ast.WalkSkipChildren, nil
	})

	for _, r := range replacements {
		r.old.Parent().ReplaceChild(r.old.Parent(), r.old, r.new)
	}
}

// d2AssetName derives the prefix-relative SVG name from the diagram source, so
// an unchanged diagram keeps its key across updates and a diagram repeated
// across pages resolves to one object.
func d2AssetName(src []byte) string {
	sum := sha256.Sum256(src)
	return "d2/" + hex.EncodeToString(sum[:])[:12] + ".svg"
}
