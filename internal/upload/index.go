package upload

import (
	"bytes"
	_ "embed"
	"html/template"
	"strings"
)

//go:embed index.gohtml
var indexTmplSrc string

var indexTmpl = template.Must(template.New("index").Parse(indexTmplSrc))

// indexEntry is a single primary file shown in the index. SourcePath is the
// markdown file a rendered HTML entry was generated from, or empty when the
// entry has no separate source.
type indexEntry struct {
	Name       string
	SourcePath string
}

type indexData struct {
	Files      []indexEntry
	Supporting []string
}

// buildIndexEntries pairs each rendered HTML file with the markdown source it
// was generated from. The markdown is folded into the HTML entry as its source
// link rather than appearing as its own top-level item. Files with no such
// relationship remain plain entries. Order is preserved.
func buildIndexEntries(files []string) []indexEntry {
	present := make(map[string]bool, len(files))
	for _, f := range files {
		present[f] = true
	}

	// map each rendered HTML to its markdown source, and record which markdown
	// files have been folded into an HTML entry.
	sourceOf := make(map[string]string)
	folded := make(map[string]bool)
	for _, f := range files {
		if !isHTMLExt(f) {
			continue
		}
		stem := strings.TrimSuffix(f, extOf(f))
		for _, ext := range []string{".md", ".markdown"} {
			if cand := stem + ext; present[cand] {
				sourceOf[f] = cand
				folded[cand] = true
				break
			}
		}
	}

	entries := make([]indexEntry, 0, len(files))
	for _, f := range files {
		if folded[f] {
			continue
		}
		entries = append(entries, indexEntry{Name: f, SourcePath: sourceOf[f]})
	}
	return entries
}

func generateIndexHTML(files []string, supporting []string) ([]byte, error) {
	data := indexData{
		Files:      buildIndexEntries(files),
		Supporting: supporting,
	}
	var buf bytes.Buffer
	if err := indexTmpl.Execute(&buf, data); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func isHTMLExt(path string) bool {
	return strings.EqualFold(extOf(path), ".html")
}

func extOf(path string) string {
	idx := strings.LastIndex(path, ".")
	if idx < 0 {
		return ""
	}
	return path[idx:]
}
