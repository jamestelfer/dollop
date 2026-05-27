# Goldmark rendering research

Research into using [goldmark](https://github.com/yuin/goldmark) to render
markdown files uploaded via dollop into self-contained HTML, with GitHub-quality
fidelity. This document captures the design decisions and open questions surfaced
during planning.

---

## Goal

When a `.md` file is uploaded, render it to `.html` and upload both side-by-side.
The rendered HTML should look like GitHub's markdown renderer, support all the
common GFM extensions, and be self-contained enough to serve correctly from an R2
public bucket with no server-side logic.

---

## Extensions

### GFM core

[`github.com/yuin/goldmark/extension`](https://github.com/yuin/goldmark) ships a
`GFM` convenience bundle covering tables, strikethrough, task lists, and autolinks
— the four extensions defined in the [GitHub Flavored Markdown
spec](https://github.github.com/gfm/). These should be enabled as the baseline.

Footnotes (added to GitHub's renderer around 2022) are in the same package as
`extension.Footnote`.

### Syntax highlighting

[`github.com/yuin/goldmark-highlighting/v2`](https://github.com/yuin/goldmark-highlighting)
integrates [Chroma](https://github.com/alecthomas/chroma) for fenced code blocks.
Chroma includes a `github` theme that closely matches GitHub's colouring. The
extension can emit either inline styles or CSS classes; CSS classes are preferred
since the stylesheet becomes a shared upload alongside the CSS (see below), keeping
per-file HTML smaller.

### Heading anchors

[`github.com/abhinav/goldmark-anchor`](https://github.com/abhinav/goldmark-anchor)
adds the hover-link anchors on headings that GitHub renders.

### Emoji

[`github.com/yuin/goldmark-emoji`](https://github.com/yuin/goldmark-emoji) handles
`:smile:` shortcode syntax.

### Alerts / callouts

GitHub's alert syntax (`> [!NOTE]`, `> [!WARNING]`, `> [!TIP]`, `> [!IMPORTANT]`,
`> [!CAUTION]`) is not in the GFM spec and has no official goldmark extension.
Options are a community package such as
[`github.com/stefanfritsch/goldmark-alerts`](https://github.com/stefanfritsch/goldmark-alerts)
(worth verifying maintenance status) or a small custom goldmark AST transformer.
The custom approach is around 60–80 lines: blockquotes whose first paragraph
matches `[!TYPE]` are transformed into `<div class="callout callout-note">` (or
equivalent). That gives full control over the output HTML and has no external
dependency risk.

### Mermaid diagrams

There is no pure-Go Mermaid renderer. The `mermaid.js` library must execute in the
browser. The approach: detect ```` ```mermaid ```` fences in the parsed AST and,
only when present, emit a `<script src="mermaid.min.js">` tag pointing to the
shared asset uploaded alongside the CSS (see below). Browsers without JS see the
raw code block. `mermaid.min.js` is uploaded once per prefix, not once per
rendered file.

### Math

LaTeX math (`$...$` / `$$...$$`) is excluded from the initial implementation.
KaTeX and MathJax both require JS bundles, and the cost-to-reach ratio is low for
the typical dollop use case.

---

## Styling: GitHub Markdown CSS

[`github-markdown-css`](https://github.com/sindresorhus/github-markdown-css)
provides the actual CSS GitHub uses to render markdown. It is embedded in the
dollop binary at build time using `//go:embed` and uploaded once per prefix as a
shared file (e.g., `github-markdown.css`).

Rendered HTML wraps content in `<div class="markdown-body">` as the package
requires.

### CSS path depth

The CSS lives at `{prefix}/github-markdown.css`. HTML files in subdirectories
need a depth-relative path: `github-markdown.css` for a file at the root of the
prefix, `../github-markdown.css` one level down, `../../github-markdown.css` two
levels down, and so on. The rendering step computes this from the HTML file's
relative path within the prefix.

The same depth logic applies to the Chroma syntax-highlight stylesheet and
`mermaid.min.js`.

---

## Caching

### R2 Cache-Control support

R2 supports `Cache-Control` on objects via the S3-compatible `PutObject` call's
`CacheControl` field. The header maps to `httpMetadata.cacheControl` and is
returned as-is in HTTP responses to browsers. This was confirmed in the [R2 S3
extensions reference](https://developers.cloudflare.com/r2/api/s3/extensions/).

### Strategy

Shared assets (CSS, syntax stylesheet, `mermaid.min.js`) are uploaded with
`Cache-Control: max-age=604800` (one week). The current `Uploader` interface has
no `CacheControl` parameter, so either the interface needs extending (adding the
field, which touches all callers) or functional options (`...PutOption`) are used.
The shared-asset upload is the only path that needs a non-default value.

R2 auto-generates ETags (MD5 of the upload content) for all objects. This means
browsers that have a stale cached copy revalidate with `If-None-Match` after the
week expires and get a `304 Not Modified` if the asset is unchanged, with no
re-download. No explicit ETag management is required.

### Cloudflare CDN edge

When a custom domain is configured for an R2 bucket, Cloudflare's edge only caches
certain file types by default. Caching everything requires a "Cache Everything"
Cache Rule on the zone. Without it, `Cache-Control` still reaches browsers and they
cache locally; Cloudflare's edge just doesn't cache the asset, so every request
hits R2. At the traffic scale dollop is designed for this is not a concern. See the
[R2 public buckets documentation](https://developers.cloudflare.com/r2/buckets/public-buckets/)
for details on custom domain caching.

---

## HTML sanitization

goldmark passes raw HTML through to output when `html.WithUnsafe()` is set. Without
it, raw HTML in markdown is stripped entirely — more aggressive than GitHub, which
uses an allowlist (permitting `<details>`, `<summary>`, `<kbd>`, `<sub>`, `<sup>`,
and others).

The correct pattern: render with `WithUnsafe()`, then pass the output through
[`github.com/microcosm-cc/bluemonday`](https://github.com/microcosm-cc/bluemonday)
with a custom policy that matches GitHub's allowlist. `UGCPolicy()` is too
restrictive (it blocks `<details>` and `<summary>`). A custom policy needs to be
crafted explicitly. This is the same approach used in the `relic` project.

---

## Design decisions

### Side-by-side model

Both the original `.md` (served as `text/plain`) and the rendered `.html` (served
as `text/html`) are uploaded. The markdown source is accessible directly by URL;
the rendered HTML is the default landing target. If a `.html` file with the same
stem already exists in the source directory, a per-file warning is printed
(`guide.html already present, skipping rendering of guide.md`) and rendering is
skipped for that file — the same pattern as the existing `--index` collision
warning.

### On by default

Rendering is on by default. An explicit `--no-render` flag opts out. If the user
wants to share raw markdown they link directly to the `.md` URL; the HTML is an
addition, not a replacement.

### Link to source

Every rendered HTML file includes a link to its `.md` source — something like a
small fixed-position element or footer pointing to `./filename.md`. This is part of
the HTML template, not the rendered markdown content.

### Optional: content-negotiation Worker

A Cloudflare Worker in front of the bucket could inspect the `Accept` header and
redirect `Accept: text/markdown` requests to the `.md` file. This is cleanly
separable — it requires no changes to dollop and can be added to the same R2 bucket
later. The only prerequisite (that both files exist at predictable relative paths)
is satisfied by the side-by-side model.

---

## Interlinking and link rewriting

When a markdown file links to another markdown file (`[guide](guide.md)`), the
rendered HTML must point to the rendered counterpart (`guide.html`). A goldmark AST
transformer walks all `ast.Link` and `ast.Image` nodes after parsing and rewrites
destinations that end in `.md` or `.markdown` to `.html`, preserving any fragment
(`guide.md#section` → `guide.html#section`).

The transformer must receive the full set of `.md` files being uploaded so it can
distinguish internal links (rewrite) from external ones like
`https://github.com/foo/bar/blob/main/README.md` (leave alone).

---

## Pipeline architecture

### Placement

A pre-upload rendering step sits between `collectRelativePaths` and the upload
calls. It receives the list of relative paths and the local source directory,
renders each `.md` file to an in-memory or temp-directory `.html` counterpart, and
returns the augmented path list.

### Multiple outputs per `.md` file

Rendering one `.md` file can produce several uploads:

- `guide.html` — the rendered page
- `github-markdown.css` — shared CSS (once per prefix)
- `highlight.css` — Chroma syntax stylesheet (once per prefix)
- `mermaid.min.js` — only if any file in the batch contains a mermaid fence (once
  per prefix)

Shared assets must be deduplicated across all `.md` files in a batch — they are
uploaded once per prefix regardless of how many markdown files are rendered. The
returned `relPaths` slice should not include shared assets; they are infrastructure,
not user content, and must not influence `URLSuffix` selection.

### `URLSuffix` behaviour

`URLSuffix` currently picks the landing URL from the uploaded file list. After
rendering, both `README.md` and `README.html` are present. `URLSuffix` must prefer
`.html` over `.md` for the same stem when determining the landing URL. With
`--no-render`, only `.md` is present and the existing behaviour is unchanged.

### `--index` collision

If rendering produces `index.html` (from `index.md`), `hasIndex` is true when
`uploadGeneratedIndex` is reached. The existing warning fires and the `--index`
generated page is skipped. The precedence chain is: `index.html` on disk >
`index.md` rendered > `--index` generated.

### Frontmatter

`goldmark-meta` (or equivalent) strips YAML frontmatter from the rendered body and
exposes it as a map. The `title` field, if present, populates the HTML `<title>`
tag. Fallback: first H1 in the document, then the filename.

### Mermaid conditional inclusion

The `<script src="mermaid.min.js">` tag is only injected into HTML files that
contain at least one mermaid code fence. The AST is scanned for
`ast.FencedCodeBlock` nodes with language `mermaid` after parsing and before
rendering. `mermaid.min.js` is only added to the shared-assets upload list if at
least one file in the batch triggers this.

### Raw HTML

Render with `html.WithUnsafe()`, then sanitize the full HTML output with bluemonday
before uploading.
