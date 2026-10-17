package upload

import (
	"bytes"
	"fmt"
	"html"
	"io/fs"
	"net/url"
	"path"
	"slices"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// indexEntry is a single primary file shown in the index. SourcePath is the
// markdown file a rendered HTML entry was generated from, or empty when the
// entry has no separate source.
type indexEntry struct {
	Name       string
	SourcePath string
	Title      string
}

// buildIndexEntries pairs each rendered HTML file with the markdown source it
// was generated from. The markdown is folded into the HTML entry as its source
// link rather than appearing as its own top-level item. Files with no such
// relationship remain plain entries. Order is preserved. Each entry takes its
// title from titles.
func buildIndexEntries(files []string, titles map[string]string) []indexEntry {
	// markdown files keyed by stem plus lowercased extension: the renderer
	// accepts any case of extension and always writes a lowercase .html.
	markdownByKey := make(map[string]string)
	for _, f := range files {
		ext := extOf(f)
		if lower := strings.ToLower(ext); slices.Contains(markdownExts, lower) {
			markdownByKey[strings.TrimSuffix(f, ext)+lower] = f
		}
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
			if cand, ok := markdownByKey[stem+ext]; ok {
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
		entries = append(entries, indexEntry{Name: f, SourcePath: sourceOf[f], Title: titles[f]})
	}
	return entries
}

// hasDistinctTitle reports whether e has a title that says more than its file
// name or stem.
func (e indexEntry) hasDistinctTitle() bool {
	name := path.Base(e.Name)
	return e.Title != "" && !strings.EqualFold(e.Title, name) && !strings.EqualFold(e.Title, strings.TrimSuffix(name, extOf(name)))
}

// indexHeading describes the upload for the top of the index page.
type indexHeading struct {
	// Title names the upload: the uploaded directory's or file's name.
	Title string
	// Lifetime says how long the upload lasts (see lifetimeOf), or is empty
	// when that is unknown.
	Lifetime string
}

// generateIndexMarkdown returns the markdown for an index page: the upload's
// title and a summary line, then its files as a directory tree. Each
// directory is a disclosure, open when it leads to a page. Each rendered
// page's markdown source is linked beside it, and the supporting files are
// tucked into a collapsed section. A page with a title in titles is listed
// under it.
func generateIndexMarkdown(heading indexHeading, files, supporting []string, titles map[string]string) ([]byte, error) {
	entries := buildIndexEntries(files, titles)
	root := buildIndexTree(entries)

	// frontmatter carries the title verbatim; the heading below is escaped
	// markdown, which the renderer would not read back as plain text.
	front, err := yaml.Marshal(map[string]string{"title": heading.Title})
	if err != nil {
		return nil, fmt.Errorf("index frontmatter: %w", err)
	}

	var b bytes.Buffer
	b.WriteString("---\n")
	b.Write(front)
	b.WriteString("---\n\n")
	b.WriteString("# " + mdEscaper.Replace(heading.Title) + "\n\n")
	b.WriteString(`<p class="index-meta">` + indexSummary(root, heading.Lifetime) + "</p>\n\n")

	// blank lines around the tree let it render as markdown inside the raw
	// <div>; the class hooks the tree styles in dollop-markdown.css.
	b.WriteString("<div class=\"file-index\">\n\n")
	writeIndexTree(&b, root, "")
	b.WriteString("\n</div>\n")

	if len(supporting) > 0 {
		// blank lines around the list let it render as markdown inside the
		// raw <details> block.
		b.WriteString("\n<details>\n<summary>Supporting files</summary>\n\n")
		for _, s := range supporting {
			b.WriteString("- " + mdLink(s, s) + "\n")
		}
		b.WriteString("\n</details>\n")
	}
	return b.Bytes(), nil
}

// indexSummary returns the HTML summary line under the index title: how many
// files and pages are listed, and the upload's lifetime when known.
func indexSummary(root *indexDir, lifetime string) string {
	parts := []string{plural(root.files, "file")}
	if root.pages > 0 {
		parts = append(parts, plural(root.pages, "page"))
	}
	if lifetime != "" {
		parts = append(parts, lifetime)
	}
	return strings.Join(parts, " · ")
}

// indexDir is a directory in the index tree. entries are the files directly
// inside it, dirs its subdirectories in name order; files and pages count
// everything below it.
type indexDir struct {
	name    string
	entries []indexEntry
	dirs    []*indexDir
	byName  map[string]*indexDir
	files   int
	pages   int
}

// buildIndexTree arranges entries, whose names are slash-separated relative
// paths, into a directory tree. Entry order is preserved within a directory.
func buildIndexTree(entries []indexEntry) *indexDir {
	root := &indexDir{}
	for _, e := range entries {
		d := root
		segments := strings.Split(e.Name, "/")
		for _, seg := range segments[:len(segments)-1] {
			sub, ok := d.byName[seg]
			if !ok {
				sub = &indexDir{name: seg}
				if d.byName == nil {
					d.byName = make(map[string]*indexDir)
				}
				d.byName[seg] = sub
				d.dirs = append(d.dirs, sub)
			}
			d = sub
		}
		d.entries = append(d.entries, e)
	}
	root.tally()
	return root
}

// tally sorts the subdirectories of d by name and counts the files and pages
// below it, recursively.
func (d *indexDir) tally() {
	slices.SortFunc(d.dirs, func(a, b *indexDir) int { return strings.Compare(a.name, b.name) })
	d.files = len(d.entries)
	d.pages = 0
	for _, e := range d.entries {
		if isPageExt(e.Name) {
			d.pages++
		}
	}
	for _, sub := range d.dirs {
		sub.tally()
		d.files += sub.files
		d.pages += sub.pages
	}
}

// writeIndexTree writes d as a markdown list indented by indent: its files
// first, then each subdirectory as a list item holding a <details> block. The
// blank line after <summary> ends the raw HTML so the nested list is parsed as
// markdown again.
func writeIndexTree(b *bytes.Buffer, d *indexDir, indent string) {
	for _, e := range d.entries {
		name := path.Base(e.Name)
		label := name
		if e.hasDistinctTitle() {
			label = e.Title
		}
		b.WriteString(indent + "- " + mdLink(label, e.Name))
		if e.SourcePath != "" {
			// The fixed marker must remain unescaped so the renderer can parse it.
			b.WriteString(" [::md-icon::](" + mdHref(e.SourcePath) + ")")
		}
		if e.hasDistinctTitle() {
			// Last on the line, so a row too narrow for it can drop it.
			b.WriteString(" <small>" + mdEscaper.Replace(name) + "</small>")
		}
		b.WriteString("\n")
	}
	for _, sub := range d.dirs {
		open := ""
		if sub.pages > 0 {
			open = " open"
		}
		b.WriteString(indent + "- <details" + open + ">\n")
		b.WriteString(indent + "  <summary>" + html.EscapeString(sub.name) + "/ <small>" + plural(sub.files, "file") + "</small></summary>\n\n")
		writeIndexTree(b, sub, indent+"  ")
		b.WriteString("\n" + indent + "  </details>\n")
	}
}

func plural(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return strconv.Itoa(n) + " " + noun + "s"
}

// lifetimeOf describes how long an upload under prefix lasts, as of now:
// "expires <date>" for an ephemeral flash/<days>/ prefix, "permanent" for a
// keep/ prefix, and "" for anything else. The date is when the lifecycle rule
// becomes due; R2 may remove the objects some hours after that.
func lifetimeOf(prefix string, now time.Time) string {
	segments := strings.Split(prefix, "/")
	switch segments[0] {
	case "keep":
		return "permanent"
	case "flash":
		if len(segments) < 2 {
			return ""
		}
		days, err := strconv.Atoi(segments[1])
		if err != nil || days < 1 {
			return ""
		}
		expires := now.AddDate(0, 0, days)
		return `expires <time datetime="` + expires.Format(time.DateOnly) + `">` + expires.Format("Mon 2 Jan 2006") + "</time>"
	default:
		return ""
	}
}

// mdLink returns a markdown link to the relative path dest, escaping the text
// and percent-encoding the destination.
func mdLink(text, dest string) string {
	return "[" + mdEscaper.Replace(text) + "](" + mdHref(dest) + ")"
}

func mdHref(dest string) string {
	href := (&url.URL{Path: dest}).EscapedPath()
	// a colon in the first segment would read as a URL scheme
	if first, _, _ := strings.Cut(href, "/"); strings.Contains(first, ":") {
		href = "./" + href
	}
	return href
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

// isPageExt reports whether path is a page a reader opens: rendered HTML, or
// markdown left unrendered.
func isPageExt(path string) bool {
	return isHTMLExt(path) || slices.ContainsFunc(markdownExts, func(ext string) bool {
		return strings.EqualFold(extOf(path), ext)
	})
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
