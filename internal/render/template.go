package render

import (
	"bytes"
	_ "embed"
	"html/template"
)

//go:embed page.gohtml
var htmlTmplSrc string

var htmlTmpl = template.Must(template.New("page").Parse(htmlTmplSrc))

type pageData struct {
	Title   string
	CSSPath string
	// ThemeCSSPath is the selected theme's token stylesheet; LayoutCSSPath is
	// the layout layer that reads those tokens.
	ThemeCSSPath     string
	LayoutCSSPath    string
	HighlightCSSPath string
	// MermaidScript is the full <script type="module"> element that loads the
	// shared mermaid engine, or empty when the document has no mermaid fence. It
	// is built server-side from the pinned version and a relative climb path (no
	// user-controlled content), so it is emitted verbatim; interpolating the path
	// into a JS string context would mangle its slashes.
	MermaidScript template.HTML
	// FontFaceCSS declares the optional shared body font, and FontFamily names
	// it for the fallback loader; both are empty when the theme does not use
	// it. Like MermaidScript it is built server-side from constants and a
	// relative climb path.
	FontFaceCSS template.CSS
	FontFamily  string
	// FontStylesheet is the theme's web-font stylesheet URL, if any.
	FontStylesheet string
	LogoLightPath  string
	LogoDarkPath   string
	FaviconPath    string
	Body           template.HTML
	SourcePath     string
}

func renderTemplate(data pageData) ([]byte, error) {
	var buf bytes.Buffer
	if err := htmlTmpl.Execute(&buf, data); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
