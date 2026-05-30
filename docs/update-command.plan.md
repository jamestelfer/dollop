# Plan: `update` subcommand

> Source PRD: `docs/prd.md` §4 (Update), with §5 lifecycle context.

The `update` command was specified in the original PRD against a `create` that did
a plain 1:1 file→object copy. `create` has since grown a rendering pipeline
(markdown→HTML, shared assets, generated index). The §4 reconciliation is recorded
in `docs/prd.md` ("Update Command Reconciliation"); this plan implements `update`
as a series of thin vertical slices against that reconciliation.

## Architectural decisions

Durable decisions that apply across all phases:

- **Command**: `dollop update <url-or-path> <source>` — first positional is an
  existing upload (full public URL **or** bare prefix); second is the local file
  or directory to publish into it.
- **Prefix shapes** (unchanged): ephemeral `flash/<days>/<id>`, permanent
  `keep/<name>`.
- **Prefix resolution**: the inverse of `upload.PublicURL`. Strip `base_url`,
  then strip any trailing filename / `index.html` / slash to recover the bare
  prefix. Accept a bare prefix verbatim. Pure, unit-tested function.
- **Render parity with `create`** (decision): `update` reuses the full upload
  pipeline — renders markdown, uploads shared assets, supports `--no-render` and
  `--index`, render on by default. The PRD's "all source files" (§4.2) is read as
  "all planned `Source`s + shared assets + optional generated index".
- **Touch scope** (decision): the self-copy/touch pass exists only to reset the
  lifecycle expiry clock, which exists only on `flash/`. `update` runs the touch
  pass for **directory** updates on **`flash/`** prefixes only. `keep/` and
  single-file updates skip it. This deviates from the letter of §4.3 ("every
  pre-existing object") but matches its stated intent.
- **Touch exclusion set**: "objects not part of the current upload set" =
  everything listed under the prefix **minus every key written this run**
  (rendered HTML, shared assets, generated index, and copied source files). The
  upload pipeline must therefore report its full written-key set.
- **Capabilities** (interfaces, injected per existing pattern): add
  `ObjectLister` (list all keys under a prefix, paginated) and `ObjectCopier`
  (`CopyObject` self-copy, `MetadataDirective: COPY`) alongside the existing
  `Uploader` / `BucketLister`. Implemented by `S3Uploader` and `DirUploader`.
- **Known wart** (per PRD out-of-scope: no diffing/deletion): source files
  deleted between `create` and `update` leave stale objects under the prefix.
  Those stale objects are **not** deleted and, being "not in the upload set",
  would be touched. Documented, not fixed.
- **Touch directive** (resolved by Phase 1 spike, 2026-05-30): the touch ships as
  a bare `CopyObject` self-copy with `MetadataDirective: COPY`. R2 accepts a no-op
  self-copy and advances `Last-Modified` — unlike AWS S3, no metadata change is
  required, so no `+meta` / `REPLACE` / `MERGE` workaround is needed.
- **Touch success signal** (resolved by Phase 1 spike): R2's `Last-Modified` has
  **1-second resolution**. The touch (and any test of it) must treat a successful
  `CopyObject` response as the authoritative signal that the object was rewritten —
  it must **not** re-read and diff `Last-Modified`, because two touches within the
  same wall-clock second are indistinguishable.

---

> **Reconciliation and spike gate are resolved.** Phase 1 ran against real R2 and
> **passed** — see `docs/prd.md` "Spike gate" and `docs/update-command.spike-results.md`.
> The reconciliation stands unchanged; Phases 2–7 below are cleared to implement.

## Phase 1: Spike — throwaway copier/touch prototype ✅ DONE (PASS)

> **Result (2026-05-30):** R2 accepts a bare `MetadataDirective: COPY` no-op
> self-copy and advances `Last-Modified`; all five probed variants passed. The
> touch ships as a bare `COPY`. One constraint for downstream phases: R2's
> `Last-Modified` is 1-second resolution — rely on the `CopyObject` 200, not a
> timestamp diff. Spike code removed. Full results:
> `docs/update-command.spike-results.md`.

**User stories**: de-risks §4.3, §4.4 (no requirement delivered).

### What to build

A throwaway standalone `main` (e.g. `cmd/spike/main.go`) that empirically proves
R2's self-copy resets `Last-Modified`. No interfaces, no core changes — inline,
hacked, deleted once the result is recorded.

- Use the existing `config` package to load config and read R2 keys from the
  keyring / fallback store (same wiring as `cmd/dollop/main.go`).
- Build an `s3.Client` inline pointed at R2.
- `PutObject` a small fixed object under a scratch key.
- `HeadObject` to capture `Last-Modified` (T0).
- Sleep long enough for a measurable timestamp delta.
- `CopyObject` self-copy onto the same key with `MetadataDirective: COPY`.
- `HeadObject` again to capture `Last-Modified` (T1).
- Print T0 and T1 and whether the timestamp advanced.

### Acceptance criteria

- [x] Spike runs against real R2 using keys sourced via the `config` package.
- [x] Self-copy with `MetadataDirective: COPY` is observed to advance
      `Last-Modified` (T1 > T0).
- [x] **Stop-and-go gate**: PASS. Outcome recorded in `docs/prd.md` ("Spike
      gate"); reconciliation unchanged. Phase 2 unblocked.
- [x] Spike code removed before merge.

---

## Phase 2: Minimal `update` — resolve prefix + overwrite

**User stories**: §4.1, §4.2, §4.6, §4.7.

### What to build

The `update` subcommand end-to-end, minus the touch pass. Resolve the prefix from
either a full URL or a bare path, then reuse the existing upload pipeline to
publish the source into that prefix, overwriting matching keys. Mirror `create`'s
`--no-render` and `--index` flags. Print the public URL to stdout, progress to
stderr. Demoable via the hidden `--copy-dir` flag.

### Acceptance criteria

- [ ] `update <full-url> <source>` and `update <bare-prefix> <source>` both
      resolve to the same prefix (unit-tested both inputs).
- [ ] Resolution strips a trailing filename / `index.html` / slash correctly.
- [ ] Source is uploaded into the existing prefix, overwriting matching keys.
- [ ] Rendering, shared assets, and `--index`/`--no-render` behave as in `create`.
- [ ] Final public URL printed to stdout; progress to stderr.
- [ ] Any upload failure prints to stderr and exits non-zero.
- [ ] Verifiable end-to-end with `--copy-dir`.

---

## Phase 3: List-by-prefix capability

**User stories**: foundation for §4.3.

### What to build

An `ObjectLister` capability returning every object key under a prefix.
Implemented on `S3Uploader` (paginated `ListObjectsV2`) and `DirUploader` (walk
the prefix subtree). Not yet wired into `update`; lands behind tests.

### Acceptance criteria

- [ ] Interface added alongside `Uploader` / `BucketLister`, injected per the
      existing pattern.
- [ ] `S3Uploader` paginates beyond a single `ListObjectsV2` page.
- [ ] `DirUploader` lists keys consistently for integration use.
- [ ] Unit-tested via `DirUploader` with a multi-file prefix.

---

## Phase 4: Self-copy capability

**User stories**: foundation for §4.3, §4.4.

### What to build

An `ObjectCopier` capability performing a `CopyObject` self-copy on a single key
with `MetadataDirective: COPY` (the bare no-op directive confirmed working in the
Phase 1 spike). Implemented on `S3Uploader` and `DirUploader`. Behind tests; not
yet wired.

The spike built `CopySource` as `"<bucket>/<key>"`. For keys with reserved/special
characters this must be URL-encoded; use the SDK's expected `CopySource` encoding
rather than raw concatenation. Treat the `CopyObject` 200 response as success — do
not re-`HeadObject` to confirm via `Last-Modified` (1-second resolution; see
architectural decisions).

### Acceptance criteria

- [ ] Interface added and injected per the existing pattern.
- [ ] `S3Uploader` builds a correct, URL-encoded `CopySource` for R2 and sets
      `MetadataDirective: COPY`.
- [ ] Success is determined from the `CopyObject` response, not a `Last-Modified` diff.
- [ ] `DirUploader` provides an equivalent touch for integration use.
- [ ] Unit-tested via `DirUploader`.

---

## Phase 5: Wire touch into `update`

**User stories**: §4.3, §4.4, §4.5.

### What to build

Make the upload pipeline report its full written-key set (sources + shared assets
+ generated index). In `update`, for **directory** updates on **`flash/`**
prefixes: list existing objects under the prefix, compute
`untouched = existing − written`, and self-copy each, printing its key to stderr.
Single-file and `keep/` updates skip the pass.

### Acceptance criteria

- [ ] Upload pipeline returns the complete set of keys written this run.
- [ ] Pre-existing objects not referenced by the source are self-copied.
- [ ] Freshly written objects (sources, assets, index) are **not** touched.
- [ ] `keep/` and single-file updates perform no touch pass.
- [ ] Each touched key printed to stderr; any touch failure exits non-zero.
- [ ] Verifiable with `--copy-dir`: an unreferenced pre-existing object is touched.

---

## Phase 6: Integration test

**User stories**: Testing Decisions (optional integration coverage).

### What to build

An optional round-trip test against minio or real R2 covering `update`: overwrite
of an existing object plus the `flash/` touch pass resetting `Last-Modified`.
Gated so it does not run without credentials/stub.

Because R2's `Last-Modified` is 1-second resolution (Phase 1 finding), any
assertion that the timestamp advanced must sleep **≥1 s** between the baseline
read and the touch, and assert `after > baseline` at second granularity (not a
sub-second delta). The touch's primary success assertion is that `CopyObject`
returns without error.

### Acceptance criteria

- [ ] Test publishes via `create`, mutates via `update`, asserts overwrite.
- [ ] Test asserts an unreferenced pre-existing object is touched (the `CopyObject`
      succeeds; `Last-Modified` advances after a ≥1 s gap).
- [ ] Skipped cleanly when no R2/minio endpoint is configured.

---

## Phase 7: Documentation & help text

**User stories**: cross-cutting.

### What to build

User-facing docs for `update`: subcommand help/usage text, README, the
`docs/progress.md` tracker, and the `CLAUDE.md` project-layout and subcommand
list (which currently omits `update`).

### Acceptance criteria

- [ ] `update` help text documents both positional args, `--no-render`, `--index`.
- [ ] README covers the `update` use case and the `flash/`-only touch behaviour.
- [ ] `CLAUDE.md` project layout and conventions mention `update`.
- [ ] `docs/progress.md` updated.
