package upload

import (
	"bytes"
	_ "embed"
	"html/template"
)

//go:embed index.gohtml
var indexTmplSrc string

var indexTmpl = template.Must(template.New("index").Parse(indexTmplSrc))

type indexData struct {
	Files []string
}

func generateIndexHTML(files []string) ([]byte, error) {
	var buf bytes.Buffer
	if err := indexTmpl.Execute(&buf, indexData{Files: files}); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
