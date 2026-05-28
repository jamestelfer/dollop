# Implementation Progress

## Tooling & Environment

- [x] `mise.toml` with `go@latest` (go 1.26.3)
- [x] `justfile` with `verify`, `fmt`, `build`, `test` recipes (all via `mise exec --`)
- [x] `.claude/hooks/session-start.sh` + `.claude/settings.json` for remote session setup
- [x] `go.mod` initialised as `github.com/jamestelfer/dollop`

## §1 General

- [x] 1.1 CLI named `dollop` (`cmd/dollop/main.go`)
- [x] 1.2 Subcommands: `config` and `create` registered; `update` not yet started
- [x] 1.3 All progress/diagnostic output to stderr — `internal/upload/upload.go`, `createcmd`
- [x] 1.4 Only final URL to stdout — `internal/cli/createcmd/command.go`, tested
- [x] 1.5 Content-Type inferred from extension via `mime.TypeByExtension` — `internal/upload/mime.go`
- [x] 1.6 Fallback to `application/octet-stream` — `internal/upload/mime.go`, tested
- [x] 1.7 Sequential file uploads — `internal/upload/upload.go` (sequential WalkDir)
- [x] 1.8 TLS for R2 S3-compatible API — `internal/upload/s3.go` (https endpoint)

## §2 Configuration

- [x] 2.1 Config stored at `$XDG_CONFIG_HOME/dollop/config.yaml` (with `~/.config` fallback) — `internal/config/config.go`, tested
- [x] 2.2 R2 credentials stored in OS keyring via `github.com/zalando/go-keyring` — `internal/config/keyring.go`
- [x] 2.3 Config file keys: `bucket`, `account_id`, `base_url`
- [x] 2.4 `config set <key> <value>` — implemented and tested
- [x] 2.5 `config get <key>` — implemented and tested
- [x] 2.6 `config list` — implemented and tested
- [x] 2.7 `config auth <key> <value>` — implemented and tested
- [x] 2.8 Missing required key → non-zero exit with descriptive error (`ErrMissingKey`, `Config.Require`) — unit tested; wired into commands not yet needed
- [x] 2.9 Keyring unavailable → non-zero exit with error to stderr — tested via `fakeKeyring.err`

## §3 Create

- [x] 3.1 `--days` flag (values: 1, 7, or 14; default: 1) — `internal/cli/createcmd/command.go`
- [x] 3.2 `--keep` flag — `internal/cli/createcmd/command.go`
- [x] 3.3 Ephemeral prefix `flash/<days>/<nanoid>/` — `internal/upload/prefix.go`, tested
- [x] 3.4 Permanent prefix `keep/<petname>/` — `internal/upload/prefix.go`, tested
- [x] 3.5 Single file upload under prefix with original filename — `internal/upload/upload.go`, tested
- [x] 3.6 Directory upload with recursive walk, preserving relative paths — `internal/upload/upload.go`, tested
- [x] 3.7 `Content-Type` header set per file on upload — `internal/upload/mime.go`, tested
- [x] 3.8 Object key printed to stderr after each upload — `internal/upload/upload.go`, tested
- [x] 3.9 Final URL printed to stdout: `<base_url>/<prefix>/` — `internal/upload/prefix.go`, tested
- [x] 3.10 Upload failure → error to stderr, non-zero exit — `internal/cli/createcmd/command.go`, tested

## §4 Update

- [ ] 4.1 Positional args: URL or prefix path + file/directory
- [ ] 4.2 Upload all source files to existing prefix (overwrite on key match)
- [ ] 4.3 `CopyObject` self-copy on pre-existing objects not in current upload set
- [ ] 4.4 `MetadataDirective: COPY` on self-copies
- [ ] 4.5 Object key printed to stderr after each touch
- [ ] 4.6 Final URL printed to stdout on completion
- [ ] 4.7 Upload/touch failure → error to stderr, non-zero exit

## §5 R2 Lifecycle Rules

- [ ] 5.1 Bucket lifecycle rules for `flash/1/`, `flash/7/`, `flash/14/` (infrastructure, out of CLI scope)
- [ ] 5.2 No lifecycle rule on `keep/` prefix (infrastructure, out of CLI scope)

## Dependencies Added

| Package | Purpose |
|---|---|
| `github.com/urfave/cli/v3` | CLI framework |
| `gopkg.in/yaml.v3` | Config file serialisation |
| `github.com/zalando/go-keyring` | OS keyring for R2 credentials |

## Dependencies Added (continued)

| Package | Purpose |
|---|---|
| `github.com/aws/aws-sdk-go-v2/service/s3` | R2 S3-compatible uploads |
| `github.com/matoous/go-nanoid/v2` | Ephemeral upload IDs |
| `github.com/dustinkirkland/golang-petname` | Permanent upload names |
| `github.com/stretchr/testify` | Test assertions |

---

## Markdown Rendering

> Source PRD: docs/prd-markdown-rendering.md  
> Source plan: docs/plan-markdown-rendering.md  
> Branch: `claude/markdown-rendering-docs-edJdh`  
> Process: TDD red-green-refactor, one test at a time. Each phase committed separately after `just verify` passes.

### P0 Baseline

- [x] Run `just verify` on the branch before starting Phase 1 — all checks pass

### Phase 1: Tracer bullet — bare render, pipeline wiring, URL suffix

> Requirements: R27, R28, R29, R30, R31  
> Commit: `feat: tracer bullet markdown render pipeline`

- [x] `internal/render` package exists with `Renderer` interface + real + no-op implementations
- [x] `UploadFiles` calls renderer between `collectRelativePaths` and upload loop
- [x] Rendered `.html` uploaded as `text/html; charset=utf-8`; original `.md` still uploaded
- [x] `URLSuffix` returns `.html` over `.md` for the same stem (R29)
- [x] `--no-render` flag on `create` command disables rendering (R31)
- [x] Collision warning fires to stderr when `{stem}.html` already exists on disk (R28)
- [x] `hasIndex` is true when `index.md` is in the batch and rendering is active (R30)
- [x] `just verify` passes

### Phase 2: HTML document template, title, source footer

> Requirements: R11, R12, R13, R14, R16  
> Commit: `feat: html document template, title resolution, source footer`

- [x] Output is a complete HTML5 document (`<!DOCTYPE html>`, `<html>`, `<head>`, `<body>`)
- [x] `<div class="markdown-body">` wraps rendered content (R10)
- [x] `<title>` from frontmatter `title` field (R12)
- [x] `<title>` falls back to first H1 text when no frontmatter title (R13)
- [x] `<title>` falls back to filename without extension when no H1 (R14)
- [x] Stylesheet `<link>` tags emitted in `<head>` (placeholder paths; CSS not yet uploaded) (R11)
- [x] Source footer link points to `./{filename}.md`, outside `markdown-body` (R16)
- [x] `just verify` passes

### Phase 3: Embedded CSS assets + depth-relative paths

> Requirements: R19, R20, R21, R23, R24 (stub), R25 (stub), R26  
> Commit: `feat: embed css assets, depth-relative paths, shared asset upload`

- [x] `internal/render/assets/` created with `github-markdown.css` and `highlight-github.css` embedded via `//go:embed assets/*`
- [x] Asset versions pinned and documented in a comment near the embed directive
- [x] Shared assets uploaded once per prefix regardless of `.md` file count (R23)
- [x] Depth-relative `<link>` hrefs: root → `github-markdown.css`; one level deep → `../github-markdown.css` (R19)
- [x] Shared assets not present in `relPaths` returned by `UploadFiles` (R26)
- [ ] `Cache-Control: max-age=604800` stubbed or hardcoded on shared asset uploads (R25 stub) — deferred to Phase 9
- [x] `just verify` passes

### Phase 4: Link rewriting

> Requirements: R17, R18  
> Commit: `feat: rewrite internal .md links to .html in rendered output`

- [x] goldmark AST transformer rewrites `.md` / `.markdown` link destinations to `.html`
- [x] Only links whose paths are present in the upload batch are rewritten (R17)
- [x] External URLs (any destination with a scheme) are never rewritten (R18)
- [x] Fragment identifiers preserved (`guide.md#section` → `guide.html#section`)
- [x] `just verify` passes

### Phase 5: GFM extensions + frontmatter stripping

> Requirements: R1, R2, R3, R4, R5, R15  
> Commit: `feat: gfm extensions, goldmark-anchor, emoji, footnotes, frontmatter stripping`

- [x] `extension.GFM` bundle enabled (tables, strikethrough, task lists, autolinks) (R1)
- [x] `extension.Footnote` enabled (R2)
- [ ] `goldmark-highlighting/v2` with `github` Chroma theme, CSS classes (R3) — deferred to Phase 7
- [x] `goldmark-anchor` for heading hover-links (R4)
- [x] `goldmark-emoji` for `:shortcode:` syntax (R5)
- [x] YAML frontmatter stripped from rendered body (R15)
- [x] `just verify` passes

### Phase 6: Bluemonday sanitization

> Requirements: R9  
> Commit: `feat: sanitize rendered html with bluemonday github-compatible policy`

- [x] Render with `html.WithUnsafe()`, then sanitize output with bluemonday
- [x] Custom policy from `NewPolicy()`, not `UGCPolicy()`
- [x] Policy permits `<details>`, `<summary>`, `<kbd>`, `<sub>`, `<sup>` and other GitHub-allowed tags
- [x] Sanitization runs on rendered body fragment before HTML template wrapping
- [x] Raw `<script>` in markdown is stripped; `<details>` survives
- [x] `just verify` passes

### Phase 7: Syntax highlighting (Chroma fully wired)

> Requirements: R3 (fully verified)  
> Commit: `feat: verify chroma syntax highlighting css classes and stylesheet link`

- [x] CSS classes emitted (not inline styles), `github` Chroma theme
- [x] Stylesheet linked via depth-relative path from Phase 3
- [x] Fenced code blocks render with token-level colouring in the browser
- [x] `just verify` passes

### Phase 8: Alerts + Mermaid

> Requirements: R6, R7, R8, R22, R24  
> Commit: `feat: github alert callouts and mermaid diagram support`

- [x] Custom goldmark AST transformer implements GitHub alert syntax (R6) — implemented as pre-processing step
- [x] Alert output: `<div class="markdown-alert markdown-alert-{type}">` with title span
- [x] All five types supported: NOTE, WARNING, TIP, IMPORTANT, CAUTION
- [x] Mermaid fences detected; `<script src="…/mermaid.min.js">` injected only when present (R7, R8)
- [x] `mermaid.min.js` embedded and added to shared-asset uploads only when a fence is found (R22, R24)
- [x] Batch with one mermaid file + one non-mermaid file: exactly one `mermaid.min.js` upload
- [x] `just verify` passes

### Phase 9: `PutOption` / Cache-Control on shared assets

> Requirements: R32, R33, R25 (properly implemented)  
> Commit: `feat: putOption pattern and cache-control on shared asset uploads`

- [x] `type PutOption func(*PutOptions)` added to `upload` package (PutOptions exported for implementors)
- [x] `Uploader.PutObject` signature gains `opts ...PutOption` — test fakes updated with variadic, existing callers pass none (R32)
- [x] `WithCacheControl(value string) PutOption` implemented
- [x] `S3Uploader` passes `CacheControl` in `PutObjectInput` when option supplied (R33)
- [x] Shared assets uploaded with `Cache-Control: max-age=604800` (R25)
- [x] All existing `PutObject` call sites compile without changes
- [x] `just verify` passes
