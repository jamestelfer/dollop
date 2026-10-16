# Plan: Refresh generated index pages on update

> Source: feasibility analysis in session (no written PRD). Bug: once an
> `index.html` is published under a prefix, `dollop update` never replaces it,
> even with `--index`, and even when dollop generated it.

## Architectural decisions

Durable decisions that apply across all phases:

- **Overlay**: entry-page planning during `update` works over the *overlay*:
  published relative paths (`upload.PublishedPaths`, already listed by
  `update`) unioned with this run's local sources. `create` passes no published
  paths, so its overlay is just the local sources and its behaviour does not change.
- **Overlay filtering**: when building a listing, the overlay drops:
  - dollop's own index files that are being replaced (`index.html`, `index.md`
    or other markdown index variants);
  - shared/render assets (the fixed shared CSS/SVG names, any
    `dollop-theme-*.css`, d2 diagram SVGs), which go under "Supporting files"
    instead of being listed as primary entries.

  The render package owns the predicate that recognises its asset names, so
  upload never hardcodes them.
- **Precedence is `create`'s, unchanged**: local index page > README redirect >
  listing (when requested) > first file. Phases only change *which* inputs feed
  these rules (the overlay) and *when* a published index blocks them.
- **CLI flags (`update` only)**:
  - `--index` (existing bool) and new `--index-overwrite` (bool) are declared
    in one `cli.MutuallyExclusiveFlags` group on the `update` command, not in
    `Flags`. Verified on urfave/cli v3.14.0: passing both gives
    `Incorrect Usage: option index cannot be set along with option index-overwrite`
    plus help, and a non-nil error from `Run`. Both flags remain listed in help.
  - `--index-overwrite` implies index generation; it is not combined with `--index`.
  - `create` does **not** get `--index-overwrite` (a fresh prefix has nothing
    to overwrite).
  - A custom tri-state `--index=overwrite` value was prototyped and rejected:
    the space form `--index overwrite` cannot work with bool-style flags in
    urfave v3, and a separate flag is easier to see in scripts.
- **Upload API**: `UploadFiles`' `generateIndex bool` becomes an index mode
  (none / generate / overwrite), and both commands map their flags onto it.
  `WithPublished` remains the way published paths are passed in.
- **Ownership default**: any published index page dollop cannot prove it
  generated is user-owned and is only replaced by `--index-overwrite` or by a
  local index page.
- **Provenance (Phase 2)**: an in-content marker in generated `index.html`
  (not S3 object metadata, not a sidecar file). The marker survives the
  `CopyObject` touch pass, needs no metadata support in `DirUploader`, and
  never shows up in prefix listings.
- **Marker placement**: the whole marker sits within the first 512 bytes of
  every generated `index.html`. It is the `<meta>` immediately after
  `<meta charset>`, before `<title>` and everything else of unbounded length.
  Provenance is decided from a ranged read of bytes 0-511 only. The marker
  carries the kind only (no title or other variable-length data), so its
  offset is fixed.
- **No legacy detection**: generated pages that predate the marker are
  user-owned, with no heuristic recognition (e.g. by CSS class). Users refresh
  them once with `--index-overwrite`.

## Requirements (EARS)

- `R1`: When `update --index` runs and no index page exists locally or
  published, dollop shall generate a listing of the overlay, not just this
  run's files.
- `R2`: While building a listing, dollop shall exclude its own index files
  and list shared/render assets only as supporting files.
- `R3`: When planning an entry page during `update`, dollop shall detect a
  README in the overlay, not only in local files.
- `R4`: Where `--index-overwrite` is supplied to `update`, dollop shall
  replace any published index page with the entry page `create`'s precedence
  produces from the overlay (README redirect, otherwise listing).
- `R5`: If `--index` and `--index-overwrite` are both supplied, then dollop
  shall reject the command with a usage error before uploading anything, using
  urfave's `MutuallyExclusiveFlags`.
- `R6`: If a local index page is present, then dollop shall upload it as-is,
  generate no entry page, and warn when `--index` or `--index-overwrite` was given.
- `R7`: If `--index` is supplied and a published index page is not known to be
  generated, then dollop shall leave it unchanged and warn, naming
  `--index-overwrite`.
- `R8`: dollop shall mark every generated `index.html` (rendered listing,
  `--no-render` listing redirect, README redirect) with a machine-readable
  provenance marker that records its kind (`listing` or `readme-redirect`).
- `R9`: When `update` runs with no local index page and the published
  `index.html` carries a `listing` marker, dollop shall regenerate the entry
  page from the overlay as though `--index` were supplied (sticky).
- `R10`: When the published `index.html` carries any dollop marker, dollop
  shall treat it as absent and apply `create`'s precedence over the overlay.
- `R11`: When regenerating over a published generated listing, dollop shall
  reuse that listing's title (from the published `index.md` frontmatter)
  instead of the local directory name.
- `R12`: If the published `index.html` carries no marker, then dollop shall
  treat it as user-owned (`R7` applies).
- `R13`: dollop shall check provenance with a single ranged read of the first
  512 bytes of the published `index.html`, only when one is published, with
  the same behaviour against R2 and `--copy-dir`.
- `R14`: dollop shall place the provenance marker entirely within the first
  512 bytes of every generated `index.html`, before `<title>`.

## Normalization notes

- "should rules be based on an overlay of the published directory too?" ->
  `R1`, `R2`, `R3`
- "should the existing index be always overridden if it's a generated index
  (and --index is supplied)" -> `R9`, `R10` (generated) / `R7`, `R12` (not provably generated)
- "should a generated index be updated if one exists but no --index" -> `R9`
- "metadata file or rules?" -> provenance marker decision, `R8`, `R13`
- "`--index overwrite` / `--index-overwrite`", "mut excl using urfave" -> `R4`, `R5`

## P0 baseline and standard quality gate

- [x] Standard command: `just verify` (fmt + build + lint + test)
- [x] P0 baseline run: `just verify` passed on `3fa0cbf` before any changes
- [x] Re-run `just verify` before marking each phase complete; it must pass
- [x] Conventional Commits: Phase 1 `feat: ...`, Phase 2 `feat: ...`

---

## Phase 1: Overlay-aware entry pages and explicit `--index-overwrite`

**EARS requirements**: R1, R2, R3, R4, R5, R6, R7

### Why this phase exists

Users get a working, explicit way to refresh a stale index today. Making
listings overlay-aware fixes a latent bug too: without it, any regenerated
listing on a partial update would list only the files from that run.

### Locked decisions (non-negotiable)

- `--index` and `--index-overwrite` sit in one `MutuallyExclusiveFlags` group
  on `update`. Passing both is a usage error, and nothing is listed or uploaded.
- `--index-overwrite` never replaces a *local* index page (R6 wins).
- With `--index-overwrite`, a README in the overlay still wins over a listing,
  as in `create`.
- With plain `--index` and a published index page, the result is unchanged
  from today (skip + warn), but the warning names `--index-overwrite`.
- `create`'s observable behaviour is unchanged.

### Flex zone (implementation choice allowed)

- Shape of the index-mode type and how `planEntry` receives the overlay.
- Name and location of the render-side "is a dollop asset" predicate.
- Whether the overlay is computed in `UploadFiles` or `planEntry`.
- Warning wording, as long as it names `--index-overwrite`.

### Open questions / risk burn-down

- Published d2 SVGs from earlier runs: confirm the asset predicate recognises
  `d2AssetName` output so they land in supporting files, not primary entries.
- Published paths include `.md` sources alongside their `.html`; confirm that
  `buildIndexEntries` folding still pairs them when the two come from
  different runs.
- Title on `--index-overwrite` comes from the local directory name in this
  phase (accepted; fixed by R11 in Phase 2).

### End-to-end behaviour to implement

`dollop update <prefix> <dir> --index-overwrite` replaces the published index
with a listing of everything under the prefix (or a README redirect if a
README is anywhere in the overlay). `--index` on a prefix with no index
generates a full-prefix listing. Passing both flags fails before any I/O.

### Acceptance criteria

- [x] `[observable]` `create --index` with dir A, then `update --index-overwrite`
  with a disjoint dir B (via `--copy-dir`): the new `index.html` lists files from
  both A and B, and no index or shared-asset file appears as a primary entry.
- [x] `[observable]` `update --index --index-overwrite …` exits non-zero with
  the urfave mutual-exclusion usage error and writes nothing to the
  `--copy-dir` target.
- [x] `[observable]` `update --index` against a prefix with a published
  `index.html` leaves it byte-identical and warns, mentioning `--index-overwrite`.
- [x] `[observable]` `update --index-overwrite` where the overlay has a
  published README (none locally) writes a README redirect, not a listing.
- [x] `[observable]` `update --index-overwrite` with a local `index.html`
  uploads the local file unchanged and warns that generation was skipped.
- [x] `[structural]` `create` has no `--index-overwrite` flag, and existing
  `create` entry-page tests pass without changes to their expectations.

### Verification

Table-driven `planEntry` tests over (local index, published index, README
location, mode). Command-level tests for flag parsing and the mutex error. A
manual `--copy-dir` run of create-then-update, inspecting the written
`index.md`/`index.html`. `just verify` green.

### Regression watchpoints

- README-redirect precedence and `--no-render` listing redirect (`index.md`
  target) in `create`.
- `TouchUntouched`: the regenerated index must be in `WrittenKeys`.
- Supporting-files section on listings (no duplicates when an asset is both
  published and re-emitted this run; reuse `mergeAssets`-style dedupe).

### Replan triggers

- The overlay can't distinguish dollop assets from user files with the same
  name (e.g. a user's own `github-markdown.css`).
- urfave changes mutex-group semantics or help placement in a way that hides
  either flag.
- Listing size or ordering becomes unreadable on large prefixes.

---

## Phase 2: Provenance marker and sticky refresh of generated indexes

**EARS requirements**: R8, R9, R10, R11, R12, R13, R14

**Carry-forward**: Before starting, re-run the Phase 1 `--copy-dir`
create-then-update scenarios and the mutex check. Fix any regressions first.

### Why this phase exists

The default should just work: a dollop-generated index stays current on every
`update` without the user remembering a flag, and user-authored pages still
can't be overwritten by accident.

### Locked decisions (non-negotiable)

- The marker is embedded in the generated `index.html` content: a `<meta>`
  naming dollop and the kind (`listing`, `readme-redirect`), placed straight
  after `<meta charset>` in both `page.gohtml` and `redirect.gohtml`, and
  ending before byte 512. The `--no-render` listing redirect is kind `listing`.
- Provenance is read with one ranged read (`bytes=0-511`) of `index.html`,
  only when it is published. This needs a new read capability on the uploader
  interfaces, implemented by both `S3Uploader` (S3 `Range`) and `DirUploader`
  (read at most 512 bytes). A marker that is absent or truncated within the
  512 bytes means unmarked.
- The title is recovered from the published `index.md` frontmatter (always
  published with a generated listing). It is read only when the kind is
  `listing`. If the title is missing or unreadable, fall back to the local
  directory name with a warning.
- Unmarked or unreadable published `index.html` is treated as user-owned. A
  read error other than "not found" fails the update rather than guessing.
- Sticky rule: effective mode = flag mode, upgraded to "generate" when the
  published index is a `listing`. A marked index is removed from the overlay,
  then `create` precedence applies.
- No legacy detection: pages that predate the marker are user-owned. Users
  refresh them once with `--index-overwrite`, which then writes a marked page.

### Flex zone (implementation choice allowed)

- Exact meta name/content format and how the listing's frontmatter reaches
  the page template.
- Interface name and signature for the ranged read (e.g. a generic
  offset/length read or a fixed head read), as long as both uploaders honour
  the 512-byte bound.
- How the page template knows to emit the marker (template field vs. a
  generator-only render option), as long as user frontmatter cannot set it.

### Open questions / risk burn-down

- Confirm R2 honours `Range` on GetObject for this key shape (expected; it is
  S3-compatible). If not, replan the read. Verified against an httptest S3
  endpoint only; the read caps the body at n bytes, so a server ignoring
  Range costs bandwidth, not correctness. Still needs a live R2 check.
- User-authored `index.md` rendered to `index.html` must **not** carry the
  marker. Confirm the marker is driven only by the index generator, not by
  arbitrary frontmatter a user could write.
- A user re-uploading a generated page byte-for-byte keeps the marker
  (accepted).

### End-to-end behaviour to implement

`dollop update <prefix> <dir>` with no index flags, on a prefix created with
`--index`, rewrites the listing to cover the whole overlay and keeps the
original title. On a prefix with a hand-written `index.html`, nothing changes
without `--index-overwrite`.

### Acceptance criteria

- [x] `[observable]` Rendered listing, `--no-render` listing redirect, and README
  redirect `index.html` each contain the provenance marker with the right
  kind, and the marker ends before byte 512 even with a very long title. A
  user `index.md` rendered to `index.html` does not carry it.
- [x] `[observable]` `create --index` on dir A, then plain `update` (no flags) with
  dir B: `index.html` is rewritten, lists A ∪ B, and keeps A's title.
- [x] `[observable]` `create` on a dir with a README, then `update --index` with a
  dir without one: the redirect is rewritten, not swapped for a listing
  (README in overlay wins).
- [x] `[observable]` Published unmarked `index.html` plus `update --index`:
  unchanged and warns (R7). Plus `--index-overwrite`: replaced with a marked page.
- [x] `[observable]` Behaviour is identical against `--copy-dir` and the S3
  uploader (S3 covered by the existing fake/stub test harness).
- [x] `[structural]` Provenance uses one ranged read of at most 512 bytes,
  skipped when no `index.html` is published. The title read happens only for
  kind `listing`.

### Verification

Unit tests for marker emission per kind and marker parsing, including
malformed/absent cases. A `planEntry` table extended with provenance as a
dimension. `--copy-dir` create-then-update runs for each acceptance criterion.
Update `update` help text, the `--index` usage string and README.md to
describe sticky refresh and `--index-overwrite`. `just verify` green.

### Regression watchpoints

- Touch pass: the marker survives `CopyObject` (content unchanged).
- Mermaid/fonts warnings and URL output unchanged.
- Title escaping (frontmatter YAML vs. HTML attribute) for unusual titles.

### Replan triggers

- The marker can't be injected into rendered listings without leaking into user
  pages (would force object metadata instead).
- The R2/S3 read path needs permissions the configured key lacks.
- Users report surprise at a sticky refresh, which would argue for an opt-out flag.

---

## Requirements coverage matrix

| Requirement ID | Phase(s) | Notes |
|---|---|---|
| R1 | Phase 1 | |
| R2 | Phase 1 | Asset predicate owned by render |
| R3 | Phase 1 | |
| R4 | Phase 1 | |
| R5 | Phase 1 | `cli.MutuallyExclusiveFlags`, verified on v3.14.0 |
| R6 | Phase 1 | Existing behaviour, re-asserted |
| R7 | Phase 1, Phase 2 | P1: any published index. P2: narrowed to unmarked |
| R8 | Phase 2 | |
| R9 | Phase 2 | |
| R10 | Phase 2 | |
| R11 | Phase 2 | P1 accepts local-dir title |
| R12 | Phase 2 | |
| R13 | Phase 2 | 512-byte ranged read |
| R14 | Phase 2 | Marker placement before `<title>` |
