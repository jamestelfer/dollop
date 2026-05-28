package render

import (
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

// alertKind is the unique NodeKind for alert callout nodes.
var alertKind = ast.NewNodeKind("Alert")

// alertNode represents a GitHub-style alert callout (> [!NOTE], etc.).
type alertNode struct {
	ast.BaseBlock
	AlertType string // lower-cased: "note", "warning", "tip", "important", "caution"
}

func (n *alertNode) Kind() ast.NodeKind { return alertKind }

func (n *alertNode) Dump(source []byte, level int) {
	ast.DumpHelper(n, source, level, map[string]string{"AlertType": n.AlertType}, nil)
}

var validAlertTypes = map[string]bool{
	"note":      true,
	"warning":   true,
	"tip":       true,
	"important": true,
	"caution":   true,
}

// alertExtension wires alertTransformer and alertNodeRenderer into a goldmark instance.
type alertExtension struct{}

func (e *alertExtension) Extend(m goldmark.Markdown) {
	m.Parser().AddOptions(
		parser.WithASTTransformers(util.Prioritized(&alertTransformer{}, 100)),
	)
	m.Renderer().AddOptions(
		renderer.WithNodeRenderers(util.Prioritized(&alertNodeRenderer{}, 100)),
	)
}

// alertTransformer converts GitHub alert blockquotes to alertNode elements in the
// goldmark AST. Because it runs after parsing, content inside fenced code blocks
// is never affected.
type alertTransformer struct{}

func (t *alertTransformer) Transform(doc *ast.Document, reader text.Reader, _ parser.Context) {
	src := reader.Source()

	type replacement struct{ old, new ast.Node }
	var replacements []replacement

	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		bq, ok := n.(*ast.Blockquote)
		if !ok {
			return ast.WalkContinue, nil
		}
		alertType, ok := detectAlertMarker(bq, src)
		if !ok {
			return ast.WalkContinue, nil
		}
		replacements = append(replacements, replacement{bq, buildAlertNode(bq, alertType)})
		return ast.WalkSkipChildren, nil
	})

	for _, r := range replacements {
		r.old.Parent().ReplaceChild(r.old.Parent(), r.old, r.new)
	}
}

// detectAlertMarker returns the lower-cased alert type when bq's first child
// paragraph starts with [!TYPE] on its very first line. Returns ("", false)
// otherwise. Goldmark splits [!NOTE] across multiple Text nodes ([, !NOTE, ]),
// so the first line is reconstructed by concatenating Text nodes up to the
// first SoftLineBreak.
func detectAlertMarker(bq *ast.Blockquote, src []byte) (string, bool) {
	para, ok := bq.FirstChild().(*ast.Paragraph)
	if !ok {
		return "", false
	}

	var firstLine strings.Builder
	for child := para.FirstChild(); child != nil; child = child.NextSibling() {
		txt, ok := child.(*ast.Text)
		if !ok {
			// Non-text inline (link, image, etc.) on the first line: not an alert marker.
			return "", false
		}
		firstLine.Write(txt.Segment.Value(src))
		if txt.SoftLineBreak() {
			break
		}
	}

	val := strings.TrimSpace(firstLine.String())
	if !strings.HasPrefix(val, "[!") || !strings.HasSuffix(val, "]") {
		return "", false
	}
	keyword := strings.ToLower(val[2 : len(val)-1])
	if !validAlertTypes[keyword] {
		return "", false
	}
	return keyword, true
}

// buildAlertNode moves blockquote children into a new alertNode, stripping the
// first line (the [!TYPE] marker) from the first paragraph's inline nodes.
func buildAlertNode(bq *ast.Blockquote, alertType string) *alertNode {
	alert := &alertNode{AlertType: alertType}

	para := bq.FirstChild().(*ast.Paragraph)

	// Remove inline nodes of the first line (up to and including the node that
	// carries SoftLineBreak=true, which marks the end of that line).
	child := para.FirstChild()
	for child != nil {
		next := child.NextSibling()
		isSoftBreak := func(n ast.Node) bool {
			t, ok := n.(*ast.Text)
			return ok && t.SoftLineBreak()
		}
		atLineEnd := isSoftBreak(child)
		para.RemoveChild(para, child)
		child = next
		if atLineEnd {
			break
		}
	}

	if para.FirstChild() == nil {
		// Paragraph held only the marker line — discard it, move remaining bq children.
		child := para.NextSibling()
		for child != nil {
			next := child.NextSibling()
			bq.RemoveChild(bq, child)
			alert.AppendChild(alert, child)
			child = next
		}
	} else {
		// Paragraph retains body content — move all bq children to the alert.
		child := bq.FirstChild()
		for child != nil {
			next := child.NextSibling()
			bq.RemoveChild(bq, child)
			alert.AppendChild(alert, child)
			child = next
		}
	}

	return alert
}

// alertNodeRenderer renders alertNode as a GitHub-style alert div.
type alertNodeRenderer struct{}

func (r *alertNodeRenderer) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	reg.Register(alertKind, r.render)
}

func (r *alertNodeRenderer) render(w util.BufWriter, _ []byte, n ast.Node, entering bool) (ast.WalkStatus, error) {
	alert := n.(*alertNode)
	if entering {
		title := strings.ToUpper(alert.AlertType[:1]) + alert.AlertType[1:]
		_, _ = w.WriteString("<div class=\"markdown-alert markdown-alert-" + alert.AlertType + "\">\n")
		_, _ = w.WriteString("<p class=\"markdown-alert-title\">" + title + "</p>\n")
	} else {
		_, _ = w.WriteString("</div>\n")
	}
	return ast.WalkContinue, nil
}
