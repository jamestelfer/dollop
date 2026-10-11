package upload

import (
	"bytes"
	_ "embed"
	"fmt"
	"html/template"
	"io"
	"path"
	"slices"
	"strings"

	"github.com/jamestelfer/dollop/internal/render"
)

//go:embed redirect.gohtml
var redirectTmplSrc string

var redirectTmpl = template.Must(template.New("redirect").Parse(redirectTmplSrc))

// entryPlan is the outcome of deciding how the upload is entered: the
// generated pages to publish alongside the sources and the relative path the
// public URL should point at ("" for the prefix root).
type entryPlan struct {
	generated []render.Source
	entry     string
}

// planEntry decides which entry page, if any, to generate for sources:
//
//  1. an index.html already present is left alone;
//  2. a README at the upload root gets an index.html redirecting to it, and the
//     URL points at the README directly;
//  3. otherwise, when generateIndex is set, an index page listing the sources
//     (and the shared assets as supporting files) is generated.
func planEntry(sources []render.Source, assets []render.SharedAsset, generateIndex bool, stderr io.Writer) (entryPlan, error) {
	relPaths := relPathsOf(sources)

	if slices.Contains(relPaths, "index.html") {
		if generateIndex {
			fmt.Fprintln(stderr, "warning: index.html already present, skipping index generation") //nolint:errcheck
		}
		return entryPlan{entry: URLSuffix(relPaths)}, nil
	}

	if readme, target, ok := findReadme(relPaths); ok {
		if generateIndex {
			fmt.Fprintf(stderr, "warning: %s present, index.html redirects to %s instead of listing files\n", readme, target) //nolint:errcheck
		}
		redirect, err := redirectSource(target)
		if err != nil {
			return entryPlan{}, err
		}
		return entryPlan{generated: []render.Source{redirect}, entry: target}, nil
	}

	if !generateIndex {
		return entryPlan{entry: URLSuffix(relPaths)}, nil
	}

	supporting := make([]string, len(assets))
	for i, a := range assets {
		supporting[i] = a.Name
	}
	html, err := generateIndexHTML(relPaths, supporting)
	if err != nil {
		return entryPlan{}, fmt.Errorf("generate index: %w", err)
	}
	return entryPlan{generated: []render.Source{htmlSource("index.html", html)}}, nil
}

// findReadme returns the README at the upload root and the page it is viewed
// through: its rendered HTML when present, otherwise the markdown itself.
// README.md and README.markdown match in any case, with .md preferred.
func findReadme(relPaths []string) (readme, target string, ok bool) {
	present := make(map[string]bool, len(relPaths))
	for _, p := range relPaths {
		present[p] = true
	}
	for _, ext := range []string{".md", ".markdown"} {
		for _, p := range relPaths {
			if strings.Contains(p, "/") || !strings.EqualFold(p, "readme"+ext) {
				continue
			}
			if html := strings.TrimSuffix(p, path.Ext(p)) + ".html"; present[html] {
				return p, html, true
			}
			return p, p, true
		}
	}
	return "", "", false
}

// redirectSource returns a generated index.html that sends the browser to
// target: location.replace, with a meta refresh for when scripts are off.
func redirectSource(target string) (render.Source, error) {
	var buf bytes.Buffer
	if err := redirectTmpl.Execute(&buf, target); err != nil {
		return render.Source{}, fmt.Errorf("generate redirect: %w", err)
	}
	return htmlSource("index.html", buf.Bytes()), nil
}

func htmlSource(relPath string, html []byte) render.Source {
	return render.Source{
		RelPath:     relPath,
		ContentType: "text/html; charset=utf-8",
		Size:        int64(len(html)),
		Open: func() (io.ReadSeekCloser, error) {
			return nopSeekCloser{bytes.NewReader(html)}, nil
		},
	}
}

// nopSeekCloser wraps *bytes.Reader to satisfy io.ReadSeekCloser with a no-op Close.
type nopSeekCloser struct{ *bytes.Reader }

func (nopSeekCloser) Close() error { return nil }

func relPathsOf(sources []render.Source) []string {
	relPaths := make([]string, len(sources))
	for i, src := range sources {
		relPaths[i] = src.RelPath
	}
	return relPaths
}
