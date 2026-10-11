package upload

import (
	"bytes"
	"io/fs"
	"net/url"
	"path"
	"strings"
	"time"
)

// indexEntry is a single primary file shown in the index. SourcePath is the
// markdown file a rendered HTML entry was generated from, or empty when the
// entry has no separate source.
type indexEntry struct {
	Name       string
	SourcePath string
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
		for _, ext := range markdownExts {
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

// generateIndexMarkdown returns the markdown for an index page listing files,
// with each rendered page's markdown source linked beside it, and the
// supporting files tucked into a collapsed section.
func generateIndexMarkdown(files []string, supporting []string) []byte {
	var b bytes.Buffer
	b.WriteString("# Files\n\n")
	for _, e := range buildIndexEntries(files) {
		b.WriteString("- " + mdLink(e.Name, e.Name))
		if e.SourcePath != "" {
			b.WriteString(" · " + mdLink("source", e.SourcePath))
		}
		b.WriteString("\n")
	}
	if len(supporting) > 0 {
		// blank lines around the list let it render as markdown inside the
		// raw <details> block.
		b.WriteString("\n<details>\n<summary>Supporting files</summary>\n\n")
		for _, s := range supporting {
			b.WriteString("- " + mdLink(s, s) + "\n")
		}
		b.WriteString("\n</details>\n")
	}
	return b.Bytes()
}

// mdLink returns a markdown link to the relative path dest, escaping the text
// and percent-encoding the destination.
func mdLink(text, dest string) string {
	href := (&url.URL{Path: dest}).EscapedPath()
	// a colon in the first segment would read as a URL scheme
	if first, _, _ := strings.Cut(href, "/"); strings.Contains(first, ":") {
		href = "./" + href
	}
	return "[" + mdEscaper.Replace(text) + "](" + href + ")"
}

var mdEscaper = func() *strings.Replacer {
	const special = "\\`*_{}[]()<>#+!|~&"
	pairs := make([]string, 0, 2*len(special))
	for _, c := range special {
		pairs = append(pairs, string(c), "\\"+string(c))
	}
	return strings.NewReplacer(pairs...)
}()

// markdownExts are the extensions of markdown files, in order of preference.
var markdownExts = []string{".md", ".markdown"}

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

// overlayFS serves one generated file, name, over base, so a generated page
// can be planned by a renderer alongside the files it links to.
type overlayFS struct {
	base fs.FS
	name string
	data []byte
}

func (o overlayFS) Open(name string) (fs.File, error) {
	if name != o.name {
		return o.base.Open(name)
	}
	return &memFile{Reader: bytes.NewReader(o.data), name: path.Base(name)}, nil
}

// memFile is an in-memory fs.File that is also seekable, as upload sources
// must be.
type memFile struct {
	*bytes.Reader
	name string
}

func (f *memFile) Stat() (fs.FileInfo, error) { return memFileInfo{f.name, f.Size()}, nil }
func (f *memFile) Close() error               { return nil }

type memFileInfo struct {
	name string
	size int64
}

func (i memFileInfo) Name() string       { return i.name }
func (i memFileInfo) Size() int64        { return i.size }
func (i memFileInfo) Mode() fs.FileMode  { return 0o444 }
func (i memFileInfo) ModTime() time.Time { return time.Time{} }
func (i memFileInfo) IsDir() bool        { return false }
func (i memFileInfo) Sys() any           { return nil }
