# Progress: `update` subcommand

> Plan: `docs/update-command.plan.md`
> Source PRD: `docs/prd.md` §4 and "Update Command Reconciliation"

A phase is checked off only when **every** acceptance criterion beneath it is
checked. Tick the phase last, once its criteria are all complete.

- [x] **Phase 1 — Spike: throwaway copier/touch prototype** _(complete — PASS; outcome recorded in `docs/prd.md` "Spike gate")_
  - [x] Spike implemented as `cmd/spike/main.go`, reusing the `config` package
        wiring; builds and `just verify` passes. Probes `COPY`, `REPLACE`, and
        `MERGE` directives (plus `+meta` variants) in one run so the gate is
        informed even if `COPY` is rejected (as it is on AWS S3 for a no-op
        self-copy).
  - [x] Spike runs against real R2 using keys sourced via the `config` package.
        Ran 2026-05-30 against the configured bucket; outcome recorded in
        `docs/prd.md` ("Spike gate").
  - [x] Self-copy advances `Last-Modified` (T1 > T0) for at least one directive.
        All five variants advance it when measured against a fresh per-probe
        baseline; bare `COPY` works. (R2 `Last-Modified` is 1-second resolution.)
  - [x] Stop-and-go gate: **PASS**. Decision recorded in `docs/prd.md` ("Spike
        gate"): touch ships as a bare no-op `COPY` self-copy; no reconciliation
        change needed. Phase 2 unblocked.
  - [x] Spike code (`cmd/spike/`) and throwaway spike docs (instructions +
        results) removed.

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

- [x] **Phase 5 — Wire touch into `update`** _(complete)_
  - [x] `UploadFiles` now returns `*UploadResult` carrying `SourceRelPaths` (for
        the URL suffix) and `WrittenKeys` — the complete key set written this run
        (shared assets + optional generated index + sources). Asset/index/source
        upload extracted into `uploadPlan`; `WrittenKeys == PutObject keys` is
        locked by a test.
  - [x] `upload.TouchUntouched` lists the prefix, subtracts `WrittenKeys`, and
        self-copies each remaining (pre-existing, unreferenced) key. Unit-tested
        in `touch_test.go`.
  - [x] Freshly written objects (sources, assets, index) are excluded via the
        written-key set; covered by the touch unit test and the command test.
  - [x] `keep/` and single-file updates perform no touch pass — the command gates
        the pass on a **directory** source under a **`flash/`** prefix
        (`command_test.go`).
  - [x] Each touched key printed to stderr (`touching [key]...done`); any touch
        failure exits non-zero (`TestUpdate_Flash_TouchFailure_NonZeroExit`).
  - [x] Verified with `--copy-dir`: an unreferenced pre-existing `old.pdf` was
        touched (mtime advanced) while the freshly-written `page.html` was not.

- [ ] **Phase 6 — Integration test** _(sleep ≥1 s before asserting `Last-Modified` advanced — R2 is 1-second resolution)_
  - [ ] Test publishes via `create`, mutates via `update`, asserts overwrite.
  - [ ] Test asserts an unreferenced pre-existing object is touched (the
        `CopyObject` succeeds; `Last-Modified` advances after a ≥1 s gap).
  - [ ] Skipped cleanly when no R2/minio endpoint is configured.
  - Manual verification (2026-05-30, built CLI against real R2; not yet an
        automated test):
    - `just build` and `./dist/dollop doctor` passed.
    - Created a `flash/1/...` directory upload containing `page.md` and
          `zzz-old.txt`; fetched the resulting public URL with `curl` and
          confirmed rendered `page.html` contained the initial markdown content
          while `zzz-old.txt` remained directly downloadable.
    - Ran `./dist/dollop update <full-create-url> <dir>` with a changed
          `page.md`; fetched `page.html` with `curl` and confirmed the overwrite,
          while stderr showed `touching [flash/.../zzz-old.txt]...done` and did
          not touch freshly-written `page.html`.
    - Read response headers with `curl -D - -o /dev/null` and confirmed
          `zzz-old.txt` `Last-Modified` advanced from
          `Sat, 30 May 2026 09:02:43 GMT` to `Sat, 30 May 2026 09:02:47 GMT`.
    - Ran `./dist/dollop update <bare-prefix> <dir>` with another changed
          `page.md`; fetched `page.html` and confirmed the second overwrite,
          with stderr again showing `touching [flash/.../zzz-old.txt]...done`.
    - Confirmed `zzz-old.txt` `Last-Modified` advanced again to
          `Sat, 30 May 2026 09:02:51 GMT`.

- [ ] **Phase 7 — Documentation & help text**
  - [ ] `update` help text documents both positional args, `--no-render`, `--index`.
  - [ ] README covers the `update` use case and the `flash/`-only touch behaviour.
  - [ ] `CLAUDE.md` project layout and conventions mention `update`.
  - [ ] `docs/progress.md` updated.
