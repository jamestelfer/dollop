package render

import (
	"strings"

	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
)

// linkRewriter is a goldmark AST transformer that rewrites internal .md links
// to .html, leaving external URLs and non-batch files unchanged.
type linkRewriter struct {
	batch map[string]bool // set of relPaths in the current render batch
}

func (lr *linkRewriter) Transform(doc *ast.Document, _ text.Reader, _ parser.Context) {
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch node := n.(type) {
		case *ast.Link:
			rewritten := lr.rewrite(string(node.Destination))
			if rewritten != "" {
				node.Destination = []byte(rewritten)
			}
		case *ast.Image:
			rewritten := lr.rewrite(string(node.Destination))
			if rewritten != "" {
				node.Destination = []byte(rewritten)
			}
		}
		return ast.WalkContinue, nil
	})
}

func (lr *linkRewriter) rewrite(dest string) string {
	// do not touch external URLs
	if strings.Contains(dest, "://") {
		return ""
	}
	// split off fragment
	path, fragment, hasFragment := strings.Cut(dest, "#")
	if !isMarkdownExt(path) {
		return ""
	}
	// only rewrite if the target is in the batch
	if !lr.batch[path] {
		return ""
	}
	htmlPath := strings.TrimSuffix(path, extOf(path)) + ".html"
	if hasFragment {
		return htmlPath + "#" + fragment
	}
	return htmlPath
}

func isMarkdownExt(path string) bool {
	lower := strings.ToLower(path)
	return strings.HasSuffix(lower, ".md") || strings.HasSuffix(lower, ".markdown")
}

func extOf(path string) string {
	idx := strings.LastIndex(path, ".")
	if idx < 0 {
		return ""
	}
	return path[idx:]
}
