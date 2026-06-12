package render

import (
	"bytes"
	"fmt"
	"html/template"
	"io"
	"os"
	"path/filepath"
	"strings"

	chromahtml "github.com/alecthomas/chroma/v2/formatters/html"
	"github.com/alecthomas/chroma/v2/styles"
	"github.com/yuin/goldmark"
	emoji "github.com/yuin/goldmark-emoji"
	highlighting "github.com/yuin/goldmark-highlighting/v2"
	meta "github.com/yuin/goldmark-meta"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer/html"
	"github.com/yuin/goldmark/text"
	"go.abhg.dev/goldmark/anchor"
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
	stderr io.Writer
}

func (m *markdownRenderer) Plan(relPaths []string, sourceDir string) ([]Source, []SharedAsset, error) {
	// batch map is used both for collision detection and by the link rewriter
	// to know which .md files are being rendered.
	batch := make(map[string]bool, len(relPaths))
	for _, p := range relPaths {
		batch[p] = true
	}

	hasMarkdown := false
	needsMermaid := false
	sources := make([]Source, 0, len(relPaths)+len(relPaths)/2)

	for _, p := range relPaths {
		if !isMarkdown(p) {
			sources = append(sources, diskSource(p, sourceDir))
			continue
		}

		hasMarkdown = true

		// include the source .md file as a plain disk source
		sources = append(sources, diskSource(p, sourceDir))

		stem := strings.TrimSuffix(p, filepath.Ext(p))
		htmlRel := stem + ".html"

		if batch[htmlRel] {
			fmt.Fprintf(m.stderr, "%s already present, skipping rendering of %s\n", htmlRel, p) //nolint:errcheck
			continue
		}

		// pre-scan for mermaid with a fast string search (no retained memory)
		srcBytes, err := os.ReadFile(filepath.Join(sourceDir, filepath.FromSlash(p))) //nolint:gosec
		if err != nil {
			return nil, nil, fmt.Errorf("read %s: %w", p, err)
		}
		if bytes.Contains(srcBytes, []byte("```mermaid")) {
			needsMermaid = true
		}

		mdPath := p
		sources = append(sources, Source{
			RelPath:     htmlRel,
			ContentType: "text/html; charset=utf-8",
			Size:        -1,
			Open: func() (io.ReadSeekCloser, error) {
				html, err := renderMarkdownFile(mdPath, sourceDir, batch)
				if err != nil {
					return nil, err
				}
				return nopSeekCloser{bytes.NewReader(html)}, nil
			},
		})
	}

	if !hasMarkdown {
		return sources, nil, nil
	}

	assets := []SharedAsset{
		{Name: "github-markdown.css", ContentType: "text/css; charset=utf-8", Content: githubMarkdownCSS},
		{Name: "highlight-github.css", ContentType: "text/css; charset=utf-8", Content: highlightGithubCSS},
		{Name: "dollop-light.svg", ContentType: "image/svg+xml; charset=utf-8", Content: dollopLightSVG},
		{Name: "dollop-dark.svg", ContentType: "image/svg+xml; charset=utf-8", Content: dollopDarkSVG},
		{Name: "dollop-favicon.svg", ContentType: "image/svg+xml; charset=utf-8", Content: dollopFaviconSVG},
	}

	if needsMermaid {
		assets = append(assets, SharedAsset{Name: "mermaid.min.js", ContentType: "application/javascript", Content: mermaidMinJS})
	}

	return sources, assets, nil
}

// diskSource creates a Source that reads the given relative path from sourceDir.
func diskSource(relPath, sourceDir string) Source {
	absPath := filepath.Join(sourceDir, filepath.FromSlash(relPath))
	info, err := os.Stat(absPath)
	var sz int64 = -1
	if err == nil {
		sz = info.Size()
	}
	return Source{
		RelPath: relPath,
		Size:    sz,
		Open:    func() (io.ReadSeekCloser, error) { return os.Open(absPath) }, //nolint:gosec
	}
}

// nopSeekCloser wraps *bytes.Reader to satisfy io.ReadSeekCloser with a no-op Close.
type nopSeekCloser struct{ *bytes.Reader }

func (nopSeekCloser) Close() error { return nil }

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
	goldmark.WithRendererOptions(html.WithUnsafe()),
	goldmark.WithParserOptions(parser.WithAutoHeadingID()),
	goldmark.WithExtensions(
		meta.Meta,
		extension.GFM,
		extension.Footnote,
		emoji.Emoji,
		&anchor.Extender{},
		highlighting.NewHighlighting(
			highlighting.WithStyle("github"),
			highlighting.WithFormatOptions(
				chromahtml.WithClasses(true),
				chromahtml.WithLineNumbers(false),
			),
			highlighting.WithCustomStyle(styles.Get("github")),
		),
		&alertExtension{},
		&mermaidExtension{},
	),
)

// hasMermaidFence reports whether the document contains a mermaid diagram.
// It looks for mermaidNode, which mermaidTransformer has already substituted for
// any ```mermaid fenced code block by the time rendering runs.
func hasMermaidFence(doc ast.Node, _ []byte) bool {
	found := false
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		if n.Kind() == mermaidKind {
			found = true
			return ast.WalkStop, nil
		}
		return ast.WalkContinue, nil
	})
	return found
}

// renderMarkdownFile parses and renders the given .md file to HTML bytes.
// It does not write any files to disk.
func renderMarkdownFile(relPath, sourceDir string, batch map[string]bool) ([]byte, error) {
	src, err := os.ReadFile(filepath.Join(sourceDir, filepath.FromSlash(relPath))) //nolint:gosec
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", relPath, err)
	}

	reader := text.NewReader(src)
	pctx := parser.NewContext()
	doc := mdParser.Parser().Parse(reader, parser.WithContext(pctx))

	// rewrite internal .md links before rendering
	lr := &linkRewriter{batch: batch}
	lr.Transform(doc.(*ast.Document), reader, pctx)

	mermaid := hasMermaidFence(doc, src)

	var bodyBuf bytes.Buffer
	if err := mdParser.Renderer().Render(&bodyBuf, src, doc); err != nil {
		return nil, fmt.Errorf("render %s: %w", relPath, err)
	}

	stem := strings.TrimSuffix(relPath, filepath.Ext(relPath))
	basename := filepath.Base(stem)

	title := extractTitle(meta.Get(pctx), src, doc, basename)
	prefix := cssDepthPrefix(relPath)

	var mermaidPath string
	if mermaid {
		mermaidPath = prefix + "mermaid.min.js"
	}

	data := pageData{
		Title:            title,
		CSSPath:          prefix + "github-markdown.css",
		HighlightCSSPath: prefix + "highlight-github.css",
		MermaidPath:      mermaidPath,
		LogoLightPath:    prefix + "dollop-light.svg",
		LogoDarkPath:     prefix + "dollop-dark.svg",
		FaviconPath:      prefix + "dollop-favicon.svg",
		Body:             template.HTML(sanitizeHTML(bodyBuf.String())), //nolint:gosec
		SourcePath:       filepath.Base(relPath),
	}

	out, err := renderTemplate(data)
	if err != nil {
		return nil, fmt.Errorf("template %s: %w", relPath, err)
	}
	return out, nil
}
