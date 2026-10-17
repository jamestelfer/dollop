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

// IndexMode selects whether UploadFiles generates an index listing.
type IndexMode int

const (
	// IndexNone generates no listing; a README redirect may still apply.
	IndexNone IndexMode = iota
	// IndexGenerate generates a listing unless an index page is present
	// locally or already published.
	IndexGenerate
	// IndexOverwrite generates a listing (or README redirect) even over an
	// index page already published; a local index page still wins.
	IndexOverwrite
)

// entryPlan is the outcome of deciding how the upload is entered: the
// generated pages to publish alongside the sources, any shared assets those
// pages need, and the relative path the public URL should point at ("" for the
// prefix root).
// publishedPrefix is what is already published under the prefix: the
// relative paths there, and what generated its index page.
type publishedPrefix struct {
	paths    []string
	index    PublishedIndex
	manifest Manifest
}

// titles returns the manifest's titles for the paths still published.
func (pub publishedPrefix) titles() map[string]string {
	titles := make(map[string]string)
	for _, p := range pub.paths {
		if title := pub.manifest.Files[p].Title; title != "" {
			titles[p] = title
		}
	}
	return titles
}

// userPaths returns the published paths less the index pages dollop generated.
func (pub publishedPrefix) userPaths() []string {
	if pub.index.Kind == "" {
		return pub.paths
	}
	return slices.DeleteFunc(slices.Clone(pub.paths), isIndexPage)
}

type entryPlan struct {
	generated []render.Source
	assets    []render.SharedAsset
	entry     string
	index     IndexKind
}

// planEntry decides which entry page, if any, to generate for sources:
//
//  1. an index.html or markdown index in the upload is left alone, and is the
//     entry;
//  2. an index page already published under the prefix (pub holds the
//     relative paths there) is not replaced, unless mode is IndexOverwrite or
//     dollop generated it: nothing is generated and it is the entry. A
//     generated listing is refreshed (mode is at least IndexGenerate) under
//     its published title;
//  3. a README at the upload root, local or published, gets an index.html redirecting to it, and the
//     URL points at the README directly;
//  4. with IndexNone, the URL points at the first local file;
//  5. otherwise an index.md listing the local and published files (pages
//     under their titles) under heading (with shared assets as supporting files) is generated and
//     planned through renderer with fsys underneath it. When renderer does not turn it into index.html, an
//     index.html redirecting to index.md is added.
func planEntry(renderer render.Renderer, fsys fs.FS, prefix string, sources []render.Source, assets []render.SharedAsset, pub publishedPrefix, mode IndexMode, heading indexHeading, titles map[string]string, stderr io.Writer) (entryPlan, error) {
	relPaths, localAssets := splitAssetDir(relPathsOf(sources))

	if existing, entry, ok := findIndexPage(relPaths); ok {
		if mode != IndexNone {
			fmt.Fprintf(stderr, "warning: %s already present, skipping index generation\n", existing) //nolint:errcheck
		}
		return entryPlan{entry: entry}, nil
	}

	published, mode, heading := pub.resolve(mode, heading, stderr)
	generateIndex := mode != IndexNone

	readme, target, hasReadme := findReadme(overlayPaths(relPaths, published))

	if existing, entry, ok := findIndexPage(published); ok {
		switch {
		case hasReadme:
			fmt.Fprintf(stderr, "warning: %s already published, skipping README redirect\n", existing) //nolint:errcheck
		case generateIndex:
			fmt.Fprintf(stderr, "warning: %s already published, skipping index generation (use --index-overwrite to replace it)\n", existing) //nolint:errcheck
		}
		return entryPlan{entry: entry}, nil
	}

	if hasReadme {
		if generateIndex {
			fmt.Fprintf(stderr, "warning: %s present, index.html redirects to %s instead of listing files\n", readme, target) //nolint:errcheck
		}
		redirect, err := redirectSource(target, IndexKindReadmeRedirect)
		if err != nil {
			return entryPlan{}, err
		}
		return entryPlan{generated: []render.Source{redirect}, entry: target, index: IndexKindReadmeRedirect}, nil
	}

	if !generateIndex {
		return entryPlan{entry: firstFile(relPaths)}, nil
	}

	files, supporting := listingPaths(relPaths, localAssets, published, assets)
	return planListing(renderer, fsys, prefix, heading, files, supporting, titles)
}

// resolve applies what generated the published index page to an upload in
// mode. It returns the published paths entry pages are planned over (without
// the index page when it is being replaced), the effective mode (a generated
// listing is always refreshed), and heading under the listing's own title.
func (pub publishedPrefix) resolve(mode IndexMode, heading indexHeading, stderr io.Writer) ([]string, IndexMode, indexHeading) {
	published := pub.paths
	if pub.index.Kind == IndexKindListing {
		mode = max(mode, IndexGenerate) // IndexOverwrite stays as it is
		if pub.index.Title == "" {
			fmt.Fprintf(stderr, "warning: published index title not found, using %q\n", heading.Title) //nolint:errcheck
		}
		heading.Title = pub.listingTitle(heading.Title)
	}
	if mode == IndexOverwrite || pub.index.Kind != "" {
		// the published index page is being replaced, so it is neither in the
		// way nor one of the files to list.
		published = slices.DeleteFunc(slices.Clone(published), isIndexPage)
	}
	return published, mode, heading
}

// listingTitle returns the title of the published generated listing, or
// fallback when there is none.
func (pub publishedPrefix) listingTitle(fallback string) string {
	if pub.index.Kind == IndexKindListing && pub.index.Title != "" {
		return pub.index.Title
	}
	return fallback
}

// planListing generates an index.md listing files (and supporting files, with
// the titles of pages) under heading and plans it through renderer with fsys underneath it. The
// index.html is marked as a generated listing: the rendered page, or when the
// renderer does not produce one, a redirect to index.md.
func planListing(renderer render.Renderer, fsys fs.FS, prefix string, heading indexHeading, files, supporting []string, titles map[string]string) (entryPlan, error) {
	listing, err := generateIndexMarkdown(heading, files, supporting, titles)
	if err != nil {
		return entryPlan{}, err
	}
	index := overlayFS{base: fsys, name: "index.md", data: listing}
	generated, genAssets, err := renderer.Plan([]string{index.name}, index, prefix)
	if err != nil {
		return entryPlan{}, fmt.Errorf("plan index: %w", err)
	}
	if i := slices.Index(relPathsOf(generated), "index.html"); i >= 0 {
		if generated[i], err = markSource(generated[i], IndexKindListing); err != nil {
			return entryPlan{}, err
		}
	} else {
		redirect, err := redirectSource(index.name, IndexKindListing)
		if err != nil {
			return entryPlan{}, err
		}
		generated = append(generated, redirect)
	}
	return entryPlan{generated: generated, assets: genAssets, index: IndexKindListing}, nil
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

// isIndexPage reports whether relPath is a root index page: index.html or a
// markdown index.
func isIndexPage(relPath string) bool {
	_, _, ok := findIndexPage([]string{relPath})
	return ok
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

// redirectSource returns a generated index.html, marked as kind, that sends
// the browser to target: location.replace, with a meta refresh for when
// scripts are off.
func redirectSource(target string, kind IndexKind) (render.Source, error) {
	var buf bytes.Buffer
	if err := redirectTmpl.Execute(&buf, target); err != nil {
		return render.Source{}, fmt.Errorf("generate redirect: %w", err)
	}
	return memSource("index.html", "text/html; charset=utf-8", markGenerated(buf.Bytes(), kind)), nil
}

// listingPaths splits the prefix's paths after this upload into listed and supporting files.
// Existing assets are supporting only alongside markdown; otherwise they served just the index.
func listingPaths(local, localAssets, published []string, assets []render.SharedAsset) (files, supporting []string) {
	supporting = make([]string, len(assets))
	for i, a := range assets {
		supporting[i] = a.RelPath()
	}
	existingAssets := slices.Clone(localAssets)
	var publishedFiles []string
	for _, p := range published {
		switch {
		case isManifest(p):
		case render.IsAssetName(p):
			existingAssets = append(existingAssets, p)
		default:
			publishedFiles = append(publishedFiles, p)
		}
	}
	files = overlayPaths(local, publishedFiles)
	slices.Sort(files)
	if slices.ContainsFunc(files, isMarkdownPath) {
		supporting = overlayPaths(supporting, existingAssets)
	}
	return files, supporting
}

func splitAssetDir(relPaths []string) (files, assets []string) {
	for _, p := range relPaths {
		if render.InAssetDir(p) {
			assets = append(assets, p)
		} else {
			files = append(files, p)
		}
	}
	return files, assets
}

// isMarkdownPath reports whether relPath has a markdown extension, in any case.
func isMarkdownPath(relPath string) bool {
	return slices.Contains(markdownExts, strings.ToLower(extOf(relPath)))
}

// overlayPaths returns local followed by the published paths not already in
// it: what the prefix will hold once this upload lands.
func overlayPaths(local, published []string) []string {
	out := slices.Clone(local)
	seen := make(map[string]bool, len(local)+len(published))
	for _, p := range local {
		seen[p] = true
	}
	for _, p := range published {
		if !seen[p] {
			seen[p] = true
			out = append(out, p)
		}
	}
	return out
}

func relPathsOf(sources []render.Source) []string {
	relPaths := make([]string, len(sources))
	for i, src := range sources {
		relPaths[i] = src.RelPath
	}
	return relPaths
}
