package render

import (
	"html/template"
	"strings"
)

// FontPrefix is the bucket key prefix of the optional shared body font. Like
// the mermaid engine it lives outside the expiring flash/ prefixes, but it is
// never fetched, embedded, or committed: the font is proprietary, so its files
// are supplied locally and uploaded with `dollop deps fonts <dir>`.
const FontPrefix = "deps/fonts/pp-mori"

// fontFamily is the family name the @font-face rules declare. It leads the
// font stack in dollop-markdown.css, ahead of Inter.
const fontFamily = "PP Mori"

// fontFace maps one font file to the CSS weights and style it serves.
type fontFace struct {
	file   string
	weight string
	style  string
}

// fontFaces covers the weights the stylesheet asks for: 400 (body), 500
// (links), 600 (strong) and 700 (headings). Headings resolve to Semibold; Black
// reads too heavy at heading sizes and is kept for explicit 800–900 only.
var fontFaces = []fontFace{
	{"PPMori-Regular.woff2", "400 500", "normal"},
	{"PPMori-Italic.woff2", "400 500", "italic"},
	{"PPMori-Semibold.woff2", "600 700", "normal"},
	{"PPMori-SemiboldItalic.woff2", "600 700", "italic"},
	{"PPMori-Black.woff2", "800 900", "normal"},
	{"PPMori-BlackItalic.woff2", "800 900", "italic"},
}

// FontFiles returns the file names that make up the shared body font.
func FontFiles() []string {
	files := make([]string, len(fontFaces))
	for i, f := range fontFaces {
		files[i] = f.file
	}
	return files
}

// fontFaceCSS builds the @font-face rules that load the shared body font from
// depsRoot (the relative climb to the bucket root, e.g. "../../../"). A browser
// only requests the faces a page uses, and when the files are not published the
// requests fail and the font stack falls back to Inter. depsRoot and the file
// names are server-generated (no user content), so the result is safe to emit
// verbatim as template.CSS.
func fontFaceCSS(depsRoot string) template.CSS {
	var b strings.Builder
	for _, f := range fontFaces {
		b.WriteString(`@font-face { font-family: "` + fontFamily + `"; src: url("` + depsRoot + FontPrefix + "/" + f.file +
			`") format("woff2"); font-weight: ` + f.weight + `; font-style: ` + f.style + "; font-display: swap; }\n")
	}
	return template.CSS(b.String()) //nolint:gosec
}
