package render

import (
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	extast "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/renderer/html"
	"github.com/yuin/goldmark/util"
)

// tableExtension wraps every GFM table in a <div class="table-scroll">. Tables
// are styled full width, so the wrapper is what lets a wide one scroll
// sideways on a narrow screen instead of overflowing the page. It also emits
// column alignment as an align attribute: the default style attribute is
// removed by the sanitiser, which silently dropped alignment.
type tableExtension struct{}

func (e *tableExtension) Extend(m goldmark.Markdown) {
	m.Renderer().AddOptions(
		extension.WithTableCellAlignMethod(extension.TableCellAlignAttribute),
		// Lower priority than the GFM table renderer (500), so this one
		// registers last and takes over the table node.
		renderer.WithNodeRenderers(util.Prioritized(&tableWrapRenderer{}, 100)),
	)
}

type tableWrapRenderer struct{}

func (r *tableWrapRenderer) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	reg.Register(extast.KindTable, r.render)
}

func (r *tableWrapRenderer) render(w util.BufWriter, _ []byte, n ast.Node, entering bool) (ast.WalkStatus, error) {
	if entering {
		_, _ = w.WriteString("<div class=\"table-scroll\">\n<table")
		if n.Attributes() != nil {
			html.RenderAttributes(w, n, extension.TableAttributeFilter)
		}
		_, _ = w.WriteString(">\n")
	} else {
		_, _ = w.WriteString("</table>\n</div>\n")
	}
	return ast.WalkContinue, nil
}
