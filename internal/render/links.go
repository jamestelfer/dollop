package render

import (
	"net/url"
	"path"
	"strings"

	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
)

// linkRewriter is a goldmark AST transformer that rewrites internal .md links
// to .html, leaving external URLs and non-batch files unchanged. Link
// destinations are resolved relative to the directory of the page being
// rendered before they are matched against the batch.
type linkRewriter struct {
	batch map[string]bool // set of relPaths in the current render batch
	dir   string          // slash-separated directory of the page being rendered
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

// rewrite returns dest with its .md extension replaced by .html when dest is a
// relative reference to a markdown file in the batch. It returns "" when dest
// must be left unchanged.
func (lr *linkRewriter) rewrite(dest string) string {
	u, err := url.Parse(dest)
	// leave external URLs (any scheme or host) and root-relative paths alone:
	// the latter resolve against the bucket root, not the upload.
	if err != nil || u.Scheme != "" || u.Host != "" || u.Path == "" || strings.HasPrefix(u.Path, "/") {
		return ""
	}

	// rawPath is dest up to any query or fragment, in its original encoding
	end := strings.IndexAny(dest, "?#")
	if end < 0 {
		end = len(dest)
	}
	rawPath, suffix := dest[:end], dest[end:]
	if !isMarkdownExt(rawPath) {
		return ""
	}

	// only rewrite if the target, resolved against the page's directory, is in
	// the batch; links that climb out of the upload never match.
	target := path.Join(lr.dir, u.Path)
	if !lr.batch[target] {
		return ""
	}
	return strings.TrimSuffix(rawPath, extOf(rawPath)) + ".html" + suffix
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
