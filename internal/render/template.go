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
	Title            string
	CSSPath          string
	HighlightCSSPath string
	MermaidPath      string // empty when no mermaid fence in the document
	LogoLightPath    string
	LogoDarkPath     string
	FaviconPath      string
	Body             template.HTML
	SourcePath       string
}

func renderTemplate(data pageData) ([]byte, error) {
	var buf bytes.Buffer
	if err := htmlTmpl.Execute(&buf, data); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
