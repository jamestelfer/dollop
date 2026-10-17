package upload

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"slices"

	"github.com/jamestelfer/dollop/internal/render"
	"gopkg.in/yaml.v3"
)

// IndexKind identifies an index.html dollop generated: a file listing or a
// redirect to the README. The zero value means the page is not known to be
// generated (absent, user-authored, or predating the marker).
type IndexKind string

const (
	// IndexKindListing is a generated file listing, rendered or (with no
	// renderer) a redirect to the generated index.md.
	IndexKindListing IndexKind = "listing"
	// IndexKindReadmeRedirect is a generated redirect to the root README.
	IndexKindReadmeRedirect IndexKind = "readme-redirect"
)

// pages returns the relative paths of the pages dollop generates for k.
func (k IndexKind) pages() []string {
	switch k {
	case IndexKindListing:
		return []string{"index.html", "index.md"}
	case IndexKindReadmeRedirect:
		return []string{"index.html"}
	}
	return nil
}

// markerHeadSize is how much of a published index.html is read to find the
// marker; markGenerated keeps the marker well inside it.
const markerHeadSize = 512

// markerPrefix starts the provenance <meta>; the kind and `">` follow.
const markerPrefix = `<meta name="dollop-generated" content="`

// markGenerated inserts the provenance marker for kind into html straight
// after its <meta charset> tag, ahead of the title and anything else of
// unbounded length, so it falls within the first markerHeadSize bytes. Without
// a charset tag it goes after <head>, or at the start.
func markGenerated(html []byte, kind IndexKind) []byte {
	at := 0
	lower := bytes.ToLower(html)
	if i := bytes.Index(lower, []byte("<meta charset")); i >= 0 {
		if end := bytes.IndexByte(lower[i:], '>'); end >= 0 {
			at = i + end + 1
		}
	} else if i := bytes.Index(lower, []byte("<head>")); i >= 0 {
		at = i + len("<head>")
	}
	tag := "\n" + markerPrefix + string(kind) + `">`
	out := make([]byte, 0, len(html)+len(tag))
	out = append(out, html[:at]...)
	out = append(out, tag...)
	return append(out, html[at:]...)
}

// markSource returns src with the provenance marker for kind added. The page
// is small, so it is read now and held in memory.
func markSource(src render.Source, kind IndexKind) (render.Source, error) {
	f, err := src.Open()
	if err != nil {
		return render.Source{}, fmt.Errorf("open %s: %w", src.RelPath, err)
	}
	html, err := io.ReadAll(f)
	f.Close() //nolint:errcheck
	if err != nil {
		return render.Source{}, fmt.Errorf("read %s: %w", src.RelPath, err)
	}
	return memSource(src.RelPath, src.ContentType, markGenerated(html, kind)), nil
}

// memSource returns a Source serving data from memory.
func memSource(relPath, contentType string, data []byte) render.Source {
	return render.Source{
		RelPath:     relPath,
		ContentType: contentType,
		Size:        int64(len(data)),
		Open: func() (io.ReadSeekCloser, error) {
			return &memFile{Reader: bytes.NewReader(data), name: relPath}, nil
		},
	}
}

// titleHeadSize bounds the read of a published index.md when recovering its
// title from the frontmatter at the top.
const titleHeadSize = 64 << 10

// PublishedIndex describes the index page already published under a prefix:
// whether dollop generated it, and for a listing, the title it was given. The
// zero value means none was published or it is not known to be generated.
type PublishedIndex struct {
	Kind  IndexKind
	Title string
}

// InspectPublishedIndex reports what generated the index.html among published
// (relative paths under prefix), from the provenance marker in its first
// markerHeadSize bytes; manifest (the published one) never decides that. For a
// listing it takes the title from manifest, or else recovers it from the
// frontmatter of the published index.md; Title is empty when both fail. An
// unmarked or vanished index.html is not known to be generated; any other
// read error is returned.
func InspectPublishedIndex(ctx context.Context, r ObjectHeadReader, bucket, prefix string, published []string, manifest Manifest) (PublishedIndex, error) {
	if !slices.Contains(published, "index.html") {
		return PublishedIndex{}, nil
	}
	head, err := r.ReadObjectHead(ctx, bucket, prefix+"/index.html", markerHeadSize)
	if errors.Is(err, ErrObjectNotFound) {
		return PublishedIndex{}, nil
	}
	if err != nil {
		return PublishedIndex{}, fmt.Errorf("inspect published index: %w", err)
	}
	kind := markerKind(head)
	if kind != IndexKindListing {
		return PublishedIndex{Kind: kind}, nil
	}
	if manifest.Title != "" {
		return PublishedIndex{Kind: kind, Title: manifest.Title}, nil
	}
	md, err := r.ReadObjectHead(ctx, bucket, prefix+"/index.md", titleHeadSize)
	if err != nil {
		return PublishedIndex{Kind: kind}, nil //nolint:nilerr // the title is best effort; the caller falls back
	}
	return PublishedIndex{Kind: kind, Title: frontmatterTitle(md)}, nil
}

// markerKind returns the generated-page kind recorded by the marker in head,
// or "" when there is no marker naming a known kind.
func markerKind(head []byte) IndexKind {
	_, rest, ok := bytes.Cut(head[:min(len(head), markerHeadSize)], []byte(markerPrefix))
	if !ok {
		return ""
	}
	kind, _, ok := bytes.Cut(rest, []byte(`">`))
	if !ok {
		return ""
	}
	switch k := IndexKind(kind); k {
	case IndexKindListing, IndexKindReadmeRedirect:
		return k
	default:
		return ""
	}
}

// frontmatterTitle returns the title from the YAML frontmatter opening md, or
// "" when there is none.
func frontmatterTitle(md []byte) string {
	rest, ok := bytes.CutPrefix(md, []byte("---\n"))
	if !ok {
		return ""
	}
	front, _, ok := bytes.Cut(rest, []byte("\n---\n"))
	if !ok {
		return ""
	}
	var fm struct {
		Title string `yaml:"title"`
	}
	if err := yaml.Unmarshal(front, &fm); err != nil {
		return ""
	}
	return fm.Title
}
