package render

import (
	"bytes"
	"html/template"
)

const htmlTmplSrc = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>{{.Title}}</title>
<link rel="stylesheet" href="{{.CSSPath}}">
<link rel="stylesheet" href="{{.HighlightCSSPath}}">
</head>
<body>
<div class="markdown-body">
{{.Body}}
</div>
<footer>
<a href="{{.SourcePath}}">View source (.md)</a>
</footer>
</body>
</html>`

var htmlTmpl = template.Must(template.New("page").Parse(htmlTmplSrc))

type pageData struct {
	Title            string
	CSSPath          string
	HighlightCSSPath string
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
