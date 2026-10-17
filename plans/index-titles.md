# Plan: Page titles in generated listings, via a published manifest

> Source: design discussion in session (no written PRD). Builds on PR #106
> (`upload-supporting-files-dollop-subdir`), which moves rendering's supporting
> files into the dollop-owned `<prefix>/.dollop/` directory. This branch is
> fast-forwarded onto that PR.

## Architectural decisions

Durable decisions that apply across all phases:

- **Manifest location**: `<prefix>/.dollop/manifest.json`, inside the
  dollop-owned `render.AssetDir`. Content type `application/json`,
  `Cache-Control: no-cache` because it is rewritten on every run (unlike the
  week-long cache on the other `.dollop/` assets).
- **Manifest schema (v1)**:
  ```json
  {
    "version": 1,
    "title": "<listing/upload title>",
    "index": "listing",
    "files": { "a/b.html": { "title": "B's heading" }, "data.csv": {} }
  }
  ```
  - `files` is keyed by relative path under the prefix. It covers every
    published file except dollop's own: no `.dollop/` paths, no legacy root
    assets (`render.IsAssetName`), no generated index pages.
  - `title` on a file is omitted when none was found.
  - `index` holds the `IndexKind` of the generated `index.html` (`listing`,
    `readme-redirect`), or is omitted.
  - Readers ignore unknown fields. A manifest with `version` other than 1 is
    treated as absent.
- **Source of truth for what exists**: `ListObjects` (`PublishedPaths`) stays
  authoritative. The manifest supplies **titles only**. Reconciling the two:
  drop manifest entries whose path is not listed; listed files that aren't in
  the manifest get no title. There are **no per-file ranged reads** of
  published objects.
- **Provenance stays with the in-content marker**: whether a published
  `index.html` is dollop-generated, and so may be replaced, is still decided
  only by the `<meta name="dollop-generated">` marker. The manifest's `index`
  field is informational. It is never used to authorise overwriting, because
  someone can replace `index.html` outside dollop without the manifest
  changing.
- **Write ordering**: the manifest is the last object written in a run, after
  sources, shared assets and entry pages. If a run fails partway, the
  previous manifest survives, and reconciliation corrects any drift on the
  next run.
- **Title extraction** (local files only):
  - Streaming read, capped at **128 KiB**, stopping as soon as a title is
    found.
  - **HTML**: text of the first `<h1>` (nested tags dropped, entities
    decoded), using the `golang.org/x/net/html` tokenizer (promoted to a direct
    dependency). If no `<h1>` closes within the cap, use `<title>`. Otherwise
    there is no title.
  - **HTML rendered from markdown this run**: the title comes from the
    markdown source using the renderer's rule (frontmatter `title`, then the
    first level-1 heading), but with no basename fallback. The render package
    owns this rule, so the rendered page's `<title>` and the listing can't
    drift apart.
  - Normalised: whitespace collapsed and trimmed, truncated to 120 runes with
    `…`.
- **Listing display**: the link text is the title, with the path's base name
  after it in `<small>`. If there's no title, or the title equals the base
  name or its stem (case-insensitive), the entry looks exactly as it does
  today.
- **Dependency injection**: listing code receives titles as data (a map or
  lookup function), not by reading S3 or the filesystem itself, so tests need
  no fakes beyond what already exists.

## Normalization notes

- "use the first h1 element in an HTML file to give it a title when generating the index" -> `R1`, `R4`, `R5`
- "Put a cap ... don't read the entire file" / "as long as it's a streaming read 128kb is OK" -> `R2`
- (design) rendered pages' h1 sits past any small cap, so take titles from the markdown source -> `R3`
- "publish writes an index.json as well that keeps a path/title mapping" + "upload to .dollop/manifest.json" + "shape looks good" -> `R6`, `R7`, `R8`, `R9`
- "we can use this on update to (a) get the files to be updated" -> `R11` (normalised: the manifest is reconciled against the object listing rather than replacing it; see the open question below)
- "(b) get the titles without getting bits of all the files" -> `R10`, `R11`, `R13`
- (design) recover the listing title from the manifest -> `R14`
- (design) safety -> `R12`, `R15`
- (resolved) unrendered markdown should get titles from its heading too -> `R16`

## Requirements (EARS)

- `R1`: When generating an index listing, the system shall show each HTML page with a known title as that title, followed by the page's file name.
- `R2`: When finding the title of a local file, the system shall read the file as a stream, stop at the first title found, and read no more than 128 KiB.
- `R3`: When an HTML page is rendered from a local markdown file in the same run, the system shall take its title from the markdown source (frontmatter `title`, else the first level-1 heading).
- `R4`: If no title is found for a page, or the title matches the file name or its stem, then the system shall list the page by its file name only, as today.
- `R5`: The system shall collapse whitespace in titles, truncate them to 120 runes, and escape them for markdown.
- `R6`: When `create` or `update` completes uploading, the system shall write `.dollop/manifest.json` (schema v1) describing every file published under the prefix and the title of each one that has a known title.
- `R7`: The system shall write the manifest last, as `application/json` with `Cache-Control: no-cache`, and include its key in the run's written keys.
- `R8`: The system shall exclude `.dollop/manifest.json` from a listing's files and supporting files.
- `R9`: If the uploaded path contains `.dollop/manifest.json`, then the system shall not upload that file and shall write the generated manifest in its place.
- `R10`: When updating, the system shall read the published manifest and use its titles for published files in the listing.
- `R11`: When updating, the system shall ignore manifest entries for paths absent from the object listing, list files absent from the manifest without a title, and not read published files to find titles.
- `R12`: If the published manifest is missing, unreadable, invalid JSON, or an unknown version, then the system shall continue as if it had no titles (warning on stderr except when it is missing).
- `R13`: When updating, the system shall write a manifest that merges the reconciled published entries with this run's local entries, with local entries winning.
- `R14`: When refreshing a published generated listing, the system shall take its title from the manifest, falling back to the published `index.md` frontmatter.
- `R15`: The system shall continue to decide whether a published `index.html` is dollop-generated only from its in-content marker.
- `R16`: When a markdown file is published without being rendered (e.g. `--no-render`), the system shall list it under its title, found with the same markdown rule as R3 and within the same 128 KiB limit.

## P0 baseline and standard quality gate

- [ ] Standard command: `just verify` (fmt + build + lint + test).
- [ ] Run as the P0 baseline on this branch (PR #106 head) before Phase 1; it must pass.
- [ ] If P0 fails, fix it (or rebase onto an updated #106) before starting the phases.
- [ ] Re-run before completing each phase; it must pass.

Observable checks use `--copy-dir` (a hidden flag on `create`/`update` that
writes to a local directory through `DirUploader`), so they need no R2 access:

```
go run ./cmd/dollop create --index --copy-dir /tmp/dollop-out <dir>
go run ./cmd/dollop update --copy-dir /tmp/dollop-out <prefix> <dir>
```

---

## Phase 1: Titles for local HTML pages in the listing

**EARS requirements**: R1, R2, R4, R5

### Why this phase exists

The thinnest useful slice: a directory of hand-written or exported HTML,
published with `--index`, gets a listing that shows what each page is
instead of only file names.

### Locked decisions (non-negotiable)

- Streaming tokenizer over `io.LimitReader(f, 128<<10)`. Stop at the first
  closed `<h1>`, with `<title>` as the fallback.
- If an `<h1>` opens but doesn't close within the cap, it counts as not found
  (it is never shown cut off).
- Display format as described in the architectural decisions.
- Titles are passed into listing generation as data. `generateIndexMarkdown`
  does no I/O.

### Flex zone (implementation choice allowed)

- Package placement of the HTML title extractor (`upload` or a small new
  internal package).
- Whether titles travel as `map[string]string` or a func.
- The exact markup for the file name (`<small>` vs a class), as long as it
  renders inside the existing `.file-index` styles.

### End-to-end behaviour to implement

`UploadFiles` finds titles for local `.html` sources that the renderer passes
through unchanged, reading `fsys`. It hands them to the listing planner, and
`writeIndexTree` shows a title wherever there is one.

### Acceptance criteria

- [ ] `[observable]` `create --index --copy-dir` on a dir with `a.html` (`<h1>Alpha <code>x</code></h1>`) shows `Alpha x` with `a.html` beside it in the generated `index.md`/`index.html`.
- [ ] `[observable]` A page with a large inline `<style>` (more than 128 KiB) before its `<h1>` falls back to its `<title>`. With neither, it is listed by file name as today.
- [ ] `[observable]` A title equal to the file stem (`<h1>notes</h1>` in `notes.html`) is listed by file name only.
- [ ] `[observable]` Markdown-special and HTML characters in titles (`*`, `[`, `<script>`) show literally and never become markup.
- [ ] `[structural]` The extractor reads through a 128 KiB limit and returns on the first `<h1>` end tag. A unit test with an `io.Reader` that fails after N bytes proves it stops early.

### Verification

Unit tests for the extractor (nested tags, entities, whitespace, missing
close, oversized head, `<title>` fallback, no title). Listing tests for
display and escaping. A manual `--copy-dir` run on a sample dir, checking the
rendered `index.html` in a browser.

### Replan triggers

- The `x/net/html` tokenizer can't stop cleanly mid-stream, or misreports a
  truncated `<h1>`.
- The listing markup with titles clashes with the existing tree CSS badly
  enough to need a CSS change in `render`.

---

## Phase 2: Titles for pages rendered from markdown

**EARS requirements**: R3, R16 (re-verifies R1, R4, R5)

**Carry-forward**: re-run the Phase 1 observable checks.

### Why this phase exists

Most dollop uploads are markdown. Their rendered pages put the `<h1>` behind
the stylesheet links, font CSS, scripts and the TOC, past any useful cap. The
markdown source has the title near the top.

### Locked decisions (non-negotiable)

- `render` exposes the title rule (frontmatter `title`, then the first
  level-1 heading) as a function over a bounded (128 KiB) reader that returns
  `""` when there's no title. The renderer's own `extractTitle` uses the same
  rule and keeps its basename fallback.
- A title is found for the rendered `.html` from its markdown source in
  `fsys`, never by opening the rendered `Source` (that would render the page
  twice).
- With `--no-render`, a `.md` file is a page in its own right and gets its
  title from the same rule (R16). When a `.md` is folded into its rendered
  `.html` entry, only the `.html` entry carries the title.

### Flex zone (implementation choice allowed)

- How the `.html` → markdown source pairing is shared with
  `buildIndexEntries` (reuse it, or a common helper).
- Whether the markdown rule parses the bounded buffer with goldmark or scans
  frontmatter and the first `# ` line directly, as long as the results match
  the renderer for ATX and setext headings and frontmatter.

### End-to-end behaviour to implement

`create --index` on a markdown dir lists each rendered page under the same
title shown in that page's browser tab.

### Acceptance criteria

- [ ] `[observable]` `notes.md` with frontmatter `title: Release Notes` and `# Something else` is listed as `Release Notes`.
- [ ] `[observable]` `guide.md` with no frontmatter and `# Getting started` is listed as `Getting started`.
- [ ] `[observable]` A markdown file with no h1 and no frontmatter title is listed by file name only (no basename-as-title).
- [ ] `[observable]` For each sample file, the listing title matches the `<title>` of the rendered page.
- [ ] `[observable]` `create --index --no-render` on the same dir lists each `.md` under its title.
- [ ] `[structural]` A single rule in `render` is used by both page rendering and listing titles.

### Verification

Shared table-driven tests that run the same inputs through the renderer
(`<title>`) and through the listing title function. A manual `--copy-dir`
run on the repo's `plans/` dir.

### Regression watchpoints

- Rendered page `<title>`s are unchanged (golden/render tests).
- README redirect and folding the `.md` source link beside its `.html` entry.

### Replan triggers

- Goldmark can't parse a truncated buffer safely (for example, a cut in the
  middle of the frontmatter gives a misleading title).

---

## Phase 3: Publish `.dollop/manifest.json`

**EARS requirements**: R6, R7, R8, R9

**Carry-forward**: re-run the Phase 1–2 observable checks.

### Why this phase exists

Record titles where a later `update` can recover them cheaply. This phase
writes the manifest for local files only. Merging with what's already
published comes in Phase 4.

### Locked decisions (non-negotiable)

- Schema v1 as in the architectural decisions. It is written on every
  `create` and `update`, whether or not a listing is generated (including a
  user-supplied index page or a README redirect).
- The top-level `title` is the listing heading's title (`uploadTitle` or the
  recovered published title). `index` is the kind of entry page generated
  this run, if any.
- Written last, `no-cache`, `application/json`, and included in
  `WrittenKeys`, so the touch pass doesn't re-copy it.
- Excluded from listing files and supporting files, even though it sits under
  `.dollop/`. A local `.dollop/manifest.json` (for example from `dollop get`)
  is dropped in favour of the generated one.

### Flex zone (implementation choice allowed)

- Go types and where manifest code lives (`upload/manifest.go` suggested).
- Whether JSON output is indented. It must be deterministic (sorted keys).

### End-to-end behaviour to implement

After any `create`/`update`, `<prefix>/.dollop/manifest.json` lists the
uploaded files with titles where they're known.

### Acceptance criteria

- [ ] `[observable]` `create --copy-dir` on a mixed dir (html, md, pdf) writes `.dollop/manifest.json` with every published non-dollop path and titles for the pages only.
- [ ] `[observable]` The manifest is written even without `--index`, and with a local `index.html` or `README.md`.
- [ ] `[observable]` The manifest appears in neither the listing's files nor its supporting files.
- [ ] `[observable]` After `get` followed by `update` of the fetched dir, the uploaded manifest is the generated one, not the fetched copy.
- [ ] `[structural]` The manifest put is the last `PutObject` call, with `Cache-Control: no-cache` (asserted with a recording uploader).

### Verification

Upload tests with a recording uploader (order, headers, body). A manual
`--copy-dir` run and a `jq` check of the output.

### Regression watchpoints

- `TouchUntouched` skips the manifest (it is in `WrittenKeys`).
- PR #106's rule that generated `.dollop/` assets override local copies.
- `get` downloads the manifest like any other object (acceptable). Confirm it
  doesn't break `get` of a single file.

### Replan triggers

- Some `Uploader` can't set `no-cache` (for example, `DirUploader` semantics
  cause test friction).
- Deciding the top-level `title` needs Phase 4's published data earlier than
  planned.

---

## Phase 4: `update` uses the published manifest for titles

**EARS requirements**: R10, R11, R12, R13

**Carry-forward**: re-run the Phase 1–3 observable checks, plus a `create`
then `update` smoke run.

### Why this phase exists

The payoff. A listing refreshed on `update` keeps the titles of files
published by earlier runs, without fetching parts of each file.

### Locked decisions (non-negotiable)

- `update` reads `.dollop/manifest.json` once, alongside `PublishedPaths`,
  via the existing `ObjectHeadReader` with a generous cap (4 MiB). A
  truncated or invalid manifest is treated as absent with a warning.
- Reconciliation as described in the architectural decisions. Local entries
  replace published ones for the same path, even when the local title is
  empty.
- `SyncUploader` doesn't change. The manifest arrives at `UploadFiles` as an
  option (alongside `WithPublished`/`WithPublishedIndex`).

### Flex zone (implementation choice allowed)

- Option naming, and whether the manifest folds into `publishedPrefix`.
- Warning wording.

### End-to-end behaviour to implement

`create --index` of dir A, then `update` with dir B (different files): the
refreshed listing shows titles for files from both A and B, and the new
manifest holds both sets.

### Acceptance criteria

- [ ] `[observable]` After `create --index A` then `update B`, the listing shows titles for A's pages (from the manifest) and B's pages (from local files).
- [ ] `[observable]` If a file from A is deleted from the prefix by hand before `update B`, it is gone from both the listing and the new manifest.
- [ ] `[observable]` If a file is added to the prefix outside dollop, it is listed by file name and recorded in the new manifest without a title.
- [ ] `[observable]` `update` against a prefix published before this feature (no manifest) behaves as today and warns about nothing. A corrupt manifest produces one warning and the update still succeeds.
- [ ] `[structural]` `update` makes no read calls for published files other than `index.html`, `index.md` and the manifest (asserted with a recording reader).

### Verification

Updatecmd tests driving `DirUploader` through create→mutate→update, plus
unit tests for reconciliation and merging. A manual `--copy-dir` run of the
same scenario.

### Regression watchpoints

- The `--index` / `--index-overwrite` / generated-listing refresh rules from
  `plans/index-refresh.md`.
- The touch pass on `flash/` prefixes.

### Replan triggers

- A 4 MiB cap turns out to be too small for realistic prefixes. That would
  mean a streaming `GetObject` read, which requires widening `SyncUploader`.

---

## Phase 5: Listing title from the manifest

**EARS requirements**: R14, R15

**Carry-forward**: re-run the Phase 4 create→update scenario.

### Why this phase exists

Recover the listing's own title in the same read as the file titles. The
separate `index.md` frontmatter read is only needed for prefixes published
before the manifest existed.

### Locked decisions (non-negotiable)

- Title precedence when refreshing a generated listing: manifest `title`,
  then `index.md` frontmatter, then the upload name (with the existing
  warning).
- `index.md` is read only when the manifest has no title.
- Provenance is still decided by the marker read alone (R15). A manifest that
  says `index: listing` never causes an unmarked `index.html` to be replaced.

### Flex zone (implementation choice allowed)

- Whether this reshapes `InspectPublishedIndex` or composes around it.

### End-to-end behaviour to implement

Refreshing a listing on `update` keeps its original title, read from the
manifest.

### Acceptance criteria

- [ ] `[observable]` `create --index` of dir `alpha`, then `update` with dir `beta`: the listing is still titled `alpha`.
- [ ] `[observable]` With the manifest's `title` removed by hand, the title is still recovered from `index.md` (the legacy path).
- [ ] `[observable]` If a hand-written, unmarked `index.html` replaces the generated one while the manifest still says `index: listing`, then `update --index` warns and keeps it.
- [ ] `[structural]` With a manifest title present, no `index.md` read happens (recording reader).

### Verification

Updatecmd tests for the three scenarios, plus the existing provenance tests
unchanged.

### Regression watchpoints

- `provenance_test.go` and the `index-refresh` behaviours.

### Replan triggers

- You want the manifest to become the authority on provenance (that reverses
  a locked decision).

---

## Resolved questions

1. **What (a) "get the files to be updated" means**: `ListObjects` stays
   authoritative and the manifest supplies titles only (R11, as planned).
2. **Unrendered markdown**: `.md` files listed as pages get titles from the
   markdown rule (R16, Phase 2).
3. **Phase size**: five phases, as drafted.

## Requirements coverage matrix

| Requirement ID | Phase(s) | Notes |
|---|---|---|
| R1 | 1, 2 | HTML in P1, rendered markdown in P2 |
| R2 | 1, 2 | 128 KiB streaming cap |
| R3 | 2 | |
| R4 | 1 | |
| R5 | 1 | |
| R6 | 3 | local entries; merged in P4 |
| R7 | 3 | |
| R8 | 3 | |
| R9 | 3 | |
| R10 | 4 | |
| R11 | 4 | resolved: listing stays authoritative |
| R12 | 4 | |
| R13 | 4 | |
| R14 | 5 | |
| R15 | 5 | invariant; also watched in P4 |
| R16 | 2 | unrendered markdown titles |
