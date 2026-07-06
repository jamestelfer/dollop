package render

import (
	"bytes"
	"fmt"
	"html/template"
	"io"
	"os"
	"path"
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

func (m *markdownRenderer) Plan(relPaths []string, sourceDir string, prefix string) ([]Source, []SharedAsset, error) {
	// batch map is used both for collision detection and by the link rewriter
	// to know which .md files are being rendered.
	batch := make(map[string]bool, len(relPaths))
	for _, p := range relPaths {
		batch[p] = true
	}

	hasMarkdown := false
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

		mdPath := p
		sources = append(sources, Source{
			RelPath:     htmlRel,
			ContentType: "text/html; charset=utf-8",
			Size:        -1,
			Open: func() (io.ReadSeekCloser, error) {
				html, err := renderMarkdownFile(mdPath, sourceDir, prefix, batch)
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

	// The mermaid engine is no longer shipped per-prefix: rendered pages
	// reference the shared, version-pinned copy under deps/mermaid/<v>/ (published
	// once via `dollop deps publish`). Only the CSS and logo assets remain
	// per-prefix.
	assets := []SharedAsset{
		{Name: "github-markdown.css", ContentType: "text/css; charset=utf-8", Content: githubMarkdownCSS},
		{Name: "highlight-github.css", ContentType: "text/css; charset=utf-8", Content: highlightGithubCSS},
		{Name: "dollop-light.svg", ContentType: "image/svg+xml; charset=utf-8", Content: dollopLightSVG},
		{Name: "dollop-dark.svg", ContentType: "image/svg+xml; charset=utf-8", Content: dollopDarkSVG},
		{Name: "dollop-favicon.svg", ContentType: "image/svg+xml; charset=utf-8", Content: dollopFaviconSVG},
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

// mermaidDepsPath returns the relative reference from a rendered page at
// prefix/relPath to the shared mermaid ESM entrypoint at
// deps/mermaid/<MermaidVersion>/mermaid.esm.min.mjs. It climbs out of the
// publish prefix entirely (one "../" per directory segment in prefix/relPath),
// then descends into the bucket-rooted deps namespace, so the reference is
// origin-independent and same-origin (no CORS).
func mermaidDepsPath(prefix, relPath string) string {
	full := path.Join(prefix, filepath.ToSlash(relPath))
	climb := strings.Count(full, "/")
	return strings.Repeat("../", climb) + "deps/mermaid/" + MermaidVersion + "/mermaid.esm.min.mjs"
}

// mermaidModuleScript builds the ES module loader that imports the shared
// mermaid engine from depsPath and initialises it. depsPath is server-generated
// from the pinned version and a relative climb (no user content), so the result
// is safe to emit verbatim as template.HTML. Loading as a module means only the
// diagram-type chunks a page actually uses are fetched on demand.
func mermaidModuleScript(depsPath string) template.HTML {
	return template.HTML(`<script type="module">import mermaid from '` + depsPath + //nolint:gosec
		`';mermaid.initialize({startOnLoad:true});</script>`)
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

// UsesMermaid reports whether publishing localPath (a markdown file or a
// directory of files) would render any mermaid diagram. It parses each markdown
// file's AST and reuses hasMermaidFence, so it agrees exactly with the renderer
// (a substring scan would miss ~~~ fences and trip on literal sample code).
// Non-markdown files are ignored. It is used by create/update to warn when the
// shared mermaid engine is not yet published.
func UsesMermaid(localPath string) (bool, error) {
	info, err := os.Stat(localPath)
	if err != nil {
		return false, fmt.Errorf("stat %s: %w", localPath, err)
	}
	if !info.IsDir() {
		return fileUsesMermaid(localPath)
	}
	found := false
	err = filepath.WalkDir(localPath, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || found || !isMarkdown(p) {
			return err
		}
		uses, ferr := fileUsesMermaid(p)
		if ferr != nil {
			return ferr
		}
		if uses {
			found = true
		}
		return nil
	})
	if err != nil {
		return false, fmt.Errorf("scan %s for mermaid: %w", localPath, err)
	}
	return found, nil
}

// fileUsesMermaid parses a single markdown file and reports whether it contains
// a mermaid fence.
func fileUsesMermaid(absPath string) (bool, error) {
	src, err := os.ReadFile(absPath) //nolint:gosec
	if err != nil {
		return false, fmt.Errorf("read %s: %w", absPath, err)
	}
	return hasMermaidFence(mdParser.Parser().Parse(text.NewReader(src)), src), nil
}

// renderMarkdownFile parses and renders the given .md file to HTML bytes.
// It does not write any files to disk.
func renderMarkdownFile(relPath, sourceDir, prefix string, batch map[string]bool) ([]byte, error) {
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
	depthPrefix := cssDepthPrefix(relPath)

	var mermaidScript template.HTML
	if mermaid {
		mermaidScript = mermaidModuleScript(mermaidDepsPath(prefix, relPath))
	}

	data := pageData{
		Title:            title,
		CSSPath:          depthPrefix + "github-markdown.css",
		HighlightCSSPath: depthPrefix + "highlight-github.css",
		MermaidScript:    mermaidScript,
		LogoLightPath:    depthPrefix + "dollop-light.svg",
		LogoDarkPath:     depthPrefix + "dollop-dark.svg",
		FaviconPath:      depthPrefix + "dollop-favicon.svg",
		Body:             template.HTML(sanitizeHTML(bodyBuf.String())), //nolint:gosec
		SourcePath:       filepath.Base(relPath),
	}

	out, err := renderTemplate(data)
	if err != nil {
		return nil, fmt.Errorf("template %s: %w", relPath, err)
	}
	return out, nil
}
