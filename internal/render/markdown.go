package render

import (
	"bytes"
	"fmt"
	"html/template"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/yuin/goldmark"
	meta "github.com/yuin/goldmark-meta"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
)

// NewMarkdownRenderer returns a Renderer that converts .md files to .html,
// discarding any warnings.
func NewMarkdownRenderer() Renderer {
	return &markdownRenderer{stderr: io.Discard}
}

// NewMarkdownRendererWithStderr returns a Renderer that writes collision
// warnings to stderr.
func NewMarkdownRendererWithStderr(stderr io.Writer) Renderer {
	return &markdownRenderer{stderr: stderr}
}

type markdownRenderer struct {
	stderr    io.Writer
	didRender bool // set to true after at least one .md is rendered
}

func (m *markdownRenderer) Render(relPaths []string, sourceDir string) ([]string, error) {
	// build a set of paths already in the list so we can detect collisions
	existing := make(map[string]bool, len(relPaths))
	for _, p := range relPaths {
		existing[p] = true
	}

	result := make([]string, 0, len(relPaths))
	for _, p := range relPaths {
		result = append(result, p)
		if !isMarkdown(p) {
			continue
		}
		stem := strings.TrimSuffix(p, filepath.Ext(p))
		htmlRel := stem + ".html"

		if existing[htmlRel] {
			fmt.Fprintf(m.stderr, "%s already present, skipping rendering of %s\n", htmlRel, p) //nolint:errcheck
			continue
		}

		generated, err := renderMarkdownFile(p, sourceDir)
		if err != nil {
			return nil, err
		}
		result = append(result, generated)
		m.didRender = true
	}
	return result, nil
}

func (m *markdownRenderer) SharedAssets() []SharedAsset {
	if !m.didRender {
		return nil
	}
	return []SharedAsset{
		{Name: "github-markdown.css", ContentType: "text/css; charset=utf-8", Content: githubMarkdownCSS},
		{Name: "highlight-github.css", ContentType: "text/css; charset=utf-8", Content: highlightGithubCSS},
	}
}

func extractTitle(metadata map[string]any, src []byte, doc ast.Node, fallback string) string {
	if metadata != nil {
		if v, ok := metadata["title"]; ok {
			if s, ok := v.(string); ok && s != "" {
				return s
			}
		}
	}
	// walk AST for the first H1
	var h1 string
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		if heading, ok := n.(*ast.Heading); ok && heading.Level == 1 {
			// collect all text segments from the heading's children
			var sb strings.Builder
			for child := heading.FirstChild(); child != nil; child = child.NextSibling() {
				if t, ok := child.(*ast.Text); ok {
					sb.Write(t.Segment.Value(src))
				}
			}
			h1 = sb.String()
			return ast.WalkStop, nil
		}
		return ast.WalkContinue, nil
	})
	if h1 != "" {
		return h1
	}
	return fallback
}

// cssDepthPrefix returns the relative path prefix (e.g. "../../") needed to
// reach the prefix root from the file's location within the prefix.
func cssDepthPrefix(relPath string) string {
	depth := strings.Count(filepath.ToSlash(relPath), "/")
	return strings.Repeat("../", depth)
}

func isMarkdown(p string) bool {
	ext := strings.ToLower(filepath.Ext(p))
	return ext == ".md" || ext == ".markdown"
}

var mdParser = goldmark.New(
	goldmark.WithExtensions(meta.Meta),
)

func renderMarkdownFile(relPath, sourceDir string) (string, error) {
	src, err := os.ReadFile(filepath.Join(sourceDir, relPath)) //nolint:gosec
	if err != nil {
		return "", fmt.Errorf("read %s: %w", relPath, err)
	}

	reader := text.NewReader(src)
	pctx := parser.NewContext()
	doc := mdParser.Parser().Parse(reader, parser.WithContext(pctx))

	var bodyBuf bytes.Buffer
	if err := mdParser.Renderer().Render(&bodyBuf, src, doc); err != nil {
		return "", fmt.Errorf("render %s: %w", relPath, err)
	}

	stem := strings.TrimSuffix(relPath, filepath.Ext(relPath))
	basename := filepath.Base(stem)
	htmlRel := stem + ".html"

	title := extractTitle(meta.Get(pctx), src, doc, basename)
	prefix := cssDepthPrefix(relPath)

	data := pageData{
		Title:            title,
		CSSPath:          prefix + "github-markdown.css",
		HighlightCSSPath: prefix + "highlight-github.css",
		Body:             template.HTML(bodyBuf.String()), //nolint:gosec
		SourcePath:       filepath.Base(relPath),
	}

	out, err := renderTemplate(data)
	if err != nil {
		return "", fmt.Errorf("template %s: %w", relPath, err)
	}

	if err := os.WriteFile(filepath.Join(sourceDir, htmlRel), out, 0o600); err != nil {
		return "", fmt.Errorf("write %s: %w", htmlRel, err)
	}
	return htmlRel, nil
}
