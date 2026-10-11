package upload

import (
	"bytes"
	_ "embed"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"slices"
	"strings"

	"github.com/jamestelfer/dollop/internal/render"
)

//go:embed redirect.gohtml
var redirectTmplSrc string

var redirectTmpl = template.Must(template.New("redirect").Parse(redirectTmplSrc))

// entryPlan is the outcome of deciding how the upload is entered: the
// generated pages to publish alongside the sources, any shared assets those
// pages need, and the relative path the public URL should point at ("" for the
// prefix root).
type entryPlan struct {
	generated []render.Source
	assets    []render.SharedAsset
	entry     string
}

// planEntry decides which entry page, if any, to generate for sources:
//
//  1. an index.html or markdown index in the upload is left alone, and is the
//     entry;
//  2. an index page already published under the prefix (published holds the
//     relative paths there) is never replaced: nothing is generated and it is
//     the entry;
//  3. a README at the upload root gets an index.html redirecting to it, and the
//     URL points at the README directly;
//  4. with no generateIndex, the URL points at the first file;
//  5. otherwise an index.md listing the sources (and the shared assets as
//     supporting files) is generated and planned through renderer with fsys
//     underneath it. When renderer does not turn it into index.html, an
//     index.html redirecting to index.md is added.
func planEntry(renderer render.Renderer, fsys fs.FS, prefix string, sources []render.Source, assets []render.SharedAsset, published []string, generateIndex bool, stderr io.Writer) (entryPlan, error) {
	relPaths := relPathsOf(sources)

	if existing, entry, ok := findIndexPage(relPaths); ok {
		if generateIndex {
			fmt.Fprintf(stderr, "warning: %s already present, skipping index generation\n", existing) //nolint:errcheck
		}
		return entryPlan{entry: entry}, nil
	}

	readme, target, hasReadme := findReadme(relPaths)

	if existing, entry, ok := findIndexPage(published); ok {
		switch {
		case hasReadme:
			fmt.Fprintf(stderr, "warning: %s already published, skipping README redirect\n", existing) //nolint:errcheck
		case generateIndex:
			fmt.Fprintf(stderr, "warning: %s already published, skipping index generation\n", existing) //nolint:errcheck
		}
		return entryPlan{entry: entry}, nil
	}

	if hasReadme {
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
		return entryPlan{entry: firstFile(relPaths)}, nil
	}

	supporting := make([]string, len(assets))
	for i, a := range assets {
		supporting[i] = a.Name
	}
	index := overlayFS{base: fsys, name: "index.md", data: generateIndexMarkdown(relPaths, supporting)}
	generated, genAssets, err := renderer.Plan([]string{index.name}, index, prefix)
	if err != nil {
		return entryPlan{}, fmt.Errorf("plan index: %w", err)
	}
	if !slices.Contains(relPathsOf(generated), "index.html") {
		redirect, err := redirectSource(index.name)
		if err != nil {
			return entryPlan{}, err
		}
		generated = append(generated, redirect)
	}
	return entryPlan{generated: generated, assets: genAssets}, nil
}

// findIndexPage returns the index page among relPaths and the URL entry for
// it: an index.html is served at the prefix root (""), while an unrendered
// markdown index is linked directly.
func findIndexPage(relPaths []string) (page, entry string, ok bool) {
	if slices.Contains(relPaths, "index.html") {
		return "index.html", "", true
	}
	for _, ext := range markdownExts {
		if page := "index" + ext; slices.Contains(relPaths, page) {
			return page, page, true
		}
	}
	return "", "", false
}

// findReadme returns the README at the upload root and the page it is viewed
// through: its rendered HTML when present, otherwise the markdown itself.
// README.md and README.markdown match in any case, with .md preferred.
func findReadme(relPaths []string) (readme, target string, ok bool) {
	present := make(map[string]bool, len(relPaths))
	for _, p := range relPaths {
		present[p] = true
	}
	for _, ext := range markdownExts {
		for _, p := range relPaths {
			if strings.Contains(p, "/") || !strings.EqualFold(p, "readme"+ext) {
				continue
			}
			if html := strings.TrimSuffix(p, extOf(p)) + ".html"; present[html] {
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
	html := buf.Bytes()
	return render.Source{
		RelPath:     "index.html",
		ContentType: "text/html; charset=utf-8",
		Size:        int64(len(html)),
		Open: func() (io.ReadSeekCloser, error) {
			return &memFile{Reader: bytes.NewReader(html), name: "index.html"}, nil
		},
	}, nil
}

func relPathsOf(sources []render.Source) []string {
	relPaths := make([]string, len(sources))
	for i, src := range sources {
		relPaths[i] = src.RelPath
	}
	return relPaths
}
