# Progress: `update` subcommand

> Plan: `docs/update-command.plan.md`
> Source PRD: `docs/prd.md` §4 and "Update Command Reconciliation"

A phase is checked off only when **every** acceptance criterion beneath it is
checked. Tick the phase last, once its criteria are all complete.

- [ ] **Phase 1 — Spike: throwaway copier/touch prototype** _(code built & pushed; empirical run handed off — see `docs/update-command.spike-instructions.md`)_
  - [x] Spike implemented as `cmd/spike/main.go`, reusing the `config` package
        wiring; builds and `just verify` passes. Probes `COPY`, `REPLACE`, and
        `MERGE` directives in one run so the gate is informed even if `COPY` is
        rejected (as it is on AWS S3 for a no-op self-copy).
  - [ ] Spike runs against real R2 using keys sourced via the `config` package.
        **Handed off**: the authoring environment has no R2 credentials; an
        executing agent runs it per `docs/update-command.spike-instructions.md`
        and records output in `docs/update-command.spike-results.md`.
  - [ ] Self-copy advances `Last-Modified` (T1 > T0) for at least one directive.
  - [ ] Stop-and-go gate: result assessed; decision on whether the PRD
        reconciliation or touch approach needs revising recorded in `docs/prd.md`
        (revised there if no self-copy directive resets `Last-Modified`). Phase 2
        does not start until this gate passes.
  - [ ] Spike code (`cmd/spike/`) removed before merge.

- [ ] **Phase 2 — Minimal `update`: resolve prefix + overwrite**
  - [ ] `update <full-url> <source>` and `update <bare-prefix> <source>` resolve
        to the same prefix (unit-tested, both inputs).
  - [ ] Resolution strips a trailing filename / `index.html` / slash correctly.
  - [ ] Source uploaded into the existing prefix, overwriting matching keys.
  - [ ] Rendering, shared assets, and `--index`/`--no-render` behave as in `create`.
  - [ ] Final public URL to stdout; progress to stderr.
  - [ ] Any upload failure prints to stderr and exits non-zero.
  - [ ] Verifiable end-to-end with `--copy-dir`.

- [ ] **Phase 3 — List-by-prefix capability**
  - [ ] `ObjectLister` interface added alongside `Uploader` / `BucketLister`,
        injected per the existing pattern.
  - [ ] `S3Uploader` paginates beyond a single `ListObjectsV2` page.
  - [ ] `DirUploader` lists keys consistently for integration use.
  - [ ] Unit-tested via `DirUploader` with a multi-file prefix.

- [ ] **Phase 4 — Self-copy capability**
  - [ ] `ObjectCopier` interface added and injected per the existing pattern.
  - [ ] `S3Uploader` builds a correct `CopySource` for R2 and sets
        `MetadataDirective: COPY`.
  - [ ] `DirUploader` provides an equivalent touch for integration use.
  - [ ] Unit-tested via `DirUploader`.

- [ ] **Phase 5 — Wire touch into `update`**
  - [ ] Upload pipeline returns the complete set of keys written this run.
  - [ ] Pre-existing objects not referenced by the source are self-copied.
  - [ ] Freshly written objects (sources, assets, index) are not touched.
  - [ ] `keep/` and single-file updates perform no touch pass.
  - [ ] Each touched key printed to stderr; any touch failure exits non-zero.
  - [ ] Verifiable with `--copy-dir`: an unreferenced pre-existing object is touched.

- [ ] **Phase 6 — Integration test**
  - [ ] Test publishes via `create`, mutates via `update`, asserts overwrite.
  - [ ] Test asserts an unreferenced pre-existing object's `Last-Modified` advances.
  - [ ] Skipped cleanly when no R2/minio endpoint is configured.

- [ ] **Phase 7 — Documentation & help text**
  - [ ] `update` help text documents both positional args, `--no-render`, `--index`.
  - [ ] README covers the `update` use case and the `flash/`-only touch behaviour.
  - [ ] `CLAUDE.md` project layout and conventions mention `update`.
  - [ ] `docs/progress.md` updated.
