# Progress: `update` subcommand

> Plan: `docs/update-command.plan.md`
> Source PRD: `docs/prd.md` §4 and "Update Command Reconciliation"

A phase is checked off only when **every** acceptance criterion beneath it is
checked. Tick the phase last, once its criteria are all complete.

- [x] **Phase 1 — Spike: throwaway copier/touch prototype** _(complete — PASS; results in `docs/update-command.spike-results.md`)_
  - [x] Spike implemented as `cmd/spike/main.go`, reusing the `config` package
        wiring; builds and `just verify` passes. Probes `COPY`, `REPLACE`, and
        `MERGE` directives (plus `+meta` variants) in one run so the gate is
        informed even if `COPY` is rejected (as it is on AWS S3 for a no-op
        self-copy).
  - [x] Spike runs against real R2 using keys sourced via the `config` package.
        Ran 2026-05-30 against the configured bucket; output captured in
        `docs/update-command.spike-results.md`.
  - [x] Self-copy advances `Last-Modified` (T1 > T0) for at least one directive.
        All five variants advance it when measured against a fresh per-probe
        baseline; bare `COPY` works. (R2 `Last-Modified` is 1-second resolution.)
  - [x] Stop-and-go gate: **PASS**. Decision recorded in `docs/prd.md` ("Spike
        gate"): touch ships as a bare no-op `COPY` self-copy; no reconciliation
        change needed. Phase 2 unblocked.
  - [x] Spike code (`cmd/spike/`) removed.

- [x] **Phase 2 — Minimal `update`: resolve prefix + overwrite** _(complete)_
  - [x] `update <full-url> <source>` and `update <bare-prefix> <source>` resolve
        to the same prefix (unit-tested, both inputs). `upload.ResolvePrefix`
        added as the pure inverse of `PublicURL`; covered in
        `internal/upload/prefix_test.go` (incl. a `PublicURL` round-trip) and
        `internal/cli/updatecmd/command_test.go`.
  - [x] Resolution strips a trailing filename / `index.html` / slash correctly.
        Recovers the canonical prefix by segment count (`flash/<days>/<id>`,
        `keep/<name>`), so any trailing filename, nested path, or slash is
        discarded; base URLs with a sub-path are handled.
  - [x] Source uploaded into the existing prefix, overwriting matching keys —
        `update` reuses `upload.UploadFiles`, so matching keys are overwritten.
  - [x] Rendering, shared assets, and `--index`/`--no-render` behave as in
        `create` (same pipeline + flags; verified by unit tests).
  - [x] Final public URL to stdout; progress to stderr. URL formatting now lives
        in the shared `internal/cli/urlout` package (deduplicated from
        `createcmd`).
  - [x] Any upload failure prints to stderr and exits non-zero.
  - [x] Verifiable end-to-end with `--copy-dir` (ran against a temp dir: full URL
        resolved to `flash/7/abc123`, markdown rendered, shared assets uploaded).

- [x] **Phase 3 — List-by-prefix capability** _(complete)_
  - [x] `ObjectLister` interface added alongside `Uploader` / `BucketLister`
        (`internal/upload/uploader.go`); compile-time assertions in `s3_test.go`
        confirm both `S3Uploader` and `DirUploader` satisfy it.
  - [x] `S3Uploader.ListObjects` paginates beyond a single `ListObjectsV2` page
        via `s3.NewListObjectsV2Paginator`, scoping the prefix to a path segment
        (trailing slash) and returning full keys.
  - [x] `DirUploader.ListObjects` walks the prefix subtree, returning the same
        full keys in lexical order (missing prefix → no keys, no error).
  - [x] Unit-tested via `DirUploader` with a multi-file prefix, including
        nested files, a name-stem sibling that must be excluded, a trailing-slash
        prefix, and a missing prefix.

- [x] **Phase 4 — Self-copy capability** _(complete — bare `COPY` confirmed by Phase 1; success = `CopyObject` 200, not a `Last-Modified` diff)_
  - [x] `ObjectCopier` interface added alongside `Uploader` / `ObjectLister`
        (`internal/upload/uploader.go`); compile-time assertions in `s3_test.go`
        confirm both `S3Uploader` and `DirUploader` satisfy it. Not yet wired
        into `update` (lands in Phase 5), per the existing capability pattern.
  - [x] `S3Uploader.CopyObject` builds a URL-encoded `CopySource` via the pure
        `copySource` helper (`bucket/<url-encoded-key>`, slashes preserved) and
        sets `MetadataDirective: COPY`. Encoding white-box-tested in
        `s3_internal_test.go` (plain key, plus spaces/`#` escaped while slashes stay
        literal).
  - [x] Success determined from the `CopyObject` response: no follow-up
        `HeadObject`/`Last-Modified` diff (1-second resolution; see Phase 1).
  - [x] `DirUploader.CopyObject` provides the equivalent touch — advances the
        file's mod time without altering content; a missing key errors (mirrors
        R2 404).
  - [x] Unit-tested via `DirUploader`: advances mod time while preserving content,
        and errors on a missing key (`diruploader_test.go`).

- [ ] **Phase 5 — Wire touch into `update`**
  - [ ] Upload pipeline returns the complete set of keys written this run.
  - [ ] Pre-existing objects not referenced by the source are self-copied.
  - [ ] Freshly written objects (sources, assets, index) are not touched.
  - [ ] `keep/` and single-file updates perform no touch pass.
  - [ ] Each touched key printed to stderr; any touch failure exits non-zero.
  - [ ] Verifiable with `--copy-dir`: an unreferenced pre-existing object is touched.

- [ ] **Phase 6 — Integration test** _(sleep ≥1 s before asserting `Last-Modified` advanced — R2 is 1-second resolution)_
  - [ ] Test publishes via `create`, mutates via `update`, asserts overwrite.
  - [ ] Test asserts an unreferenced pre-existing object is touched (the
        `CopyObject` succeeds; `Last-Modified` advances after a ≥1 s gap).
  - [ ] Skipped cleanly when no R2/minio endpoint is configured.

- [ ] **Phase 7 — Documentation & help text**
  - [ ] `update` help text documents both positional args, `--no-render`, `--index`.
  - [ ] README covers the `update` use case and the `flash/`-only touch behaviour.
  - [ ] `CLAUDE.md` project layout and conventions mention `update`.
  - [ ] `docs/progress.md` updated.
