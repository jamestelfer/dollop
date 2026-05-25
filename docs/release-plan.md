# Release Pipeline Plan

## Overview

Two-phase implementation of an automated release pipeline:

- **Phase 1** — goreleaser builds and publishes binaries to GitHub Releases and a Homebrew tap on every `v*` tag
- **Phase 2** — release-please automates tag creation, CHANGELOG generation, and version bumping from Conventional Commit PR titles

A third phase (Nix `vendorHash` automation) is planned separately; see bottom of this document.

The end-to-end flow once both phases are live:

1. Merge feature PRs to `main` using squash merge with Conventional Commit PR titles (`feat:`, `fix:`, `chore:`, etc.)
2. release-please creates or updates a release PR showing the pending version bump and accumulated CHANGELOG
3. Merge the release PR when ready to cut a release
4. release-please creates the tag (e.g. `v0.2.0`) on `main`
5. goreleaser fires on the tag, builds multi-platform binaries, publishes to GitHub Releases and the Homebrew tap

---

## Phase 1 — goreleaser release workflow

### Manual prerequisites

- [ ] **Create `release` environment** in GitHub: Settings → Environments → New environment → name: `release`
- [ ] **Create Homebrew tap repo** `jamestelfer/homebrew-tap` if it does not exist; ensure it contains a `Casks/` directory
- [ ] **Generate a PAT** with write access to `jamestelfer/homebrew-tap` (fine-grained: Contents read/write on that repo)
- [ ] **Add `HOMEBREW_GITHUB_TOKEN` secret** to the `release` environment with the PAT value above

### Files

| File | Action |
|---|---|
| `mise.toml` | Add `goreleaser` tool with pinned version |
| `.goreleaser.yaml` | New — multi-platform build, archives, checksums, Homebrew cask |
| `.github/workflows/release.yml` | New — triggers on `v*` tags, runs in `release` environment |

### Workflow details

- Trigger: `on: push: tags: v*`
- Runs in `release` environment (gates access to `HOMEBREW_GITHUB_TOKEN`)
- Permissions: `contents: write`, `id-token: write`
- Reads goreleaser version from `mise.toml` via `mise current goreleaser`
- No npm publish step (dollop is Go-only)

### goreleaser config details

- Binary: `dollop` from `./cmd/dollop`
- Platforms: linux, darwin, windows × amd64, arm64
- Archives: `tar.gz` (zip on Windows)
- Checksums file
- Changelog: `github-native`
- Homebrew cask: published to `jamestelfer/homebrew-tap` in `Casks/`, with macOS quarantine removal hook

### Manual verification after Phase 1

- [ ] Push a test tag (e.g. `v0.0.1-test`) to confirm the workflow runs and goreleaser succeeds
- [ ] Confirm binary appears in GitHub Releases
- [ ] Confirm Homebrew cask is updated in `jamestelfer/homebrew-tap`
- [ ] Delete test tag and release afterwards

---

## Phase 2 — release-please automation

### Manual prerequisites

- [ ] **Set Actions workflow permissions**: Settings → Actions → General → Workflow permissions → select "Read and write permissions" (required so the GITHUB_TOKEN can create PRs and tags)
- [ ] **Configure squash-only merges**: Settings → General → Pull Requests section:
  - Uncheck "Allow merge commits"
  - Uncheck "Allow rebase merging"
  - Keep "Allow squash merging" checked
  - Set default commit message to "Pull request title" (this becomes the Conventional Commit that release-please reads)
- [ ] **Adopt Conventional Commit PR titles** from this point forward — release-please reads the squash commit message (= PR title) to determine version bumps and CHANGELOG entries:
  - `feat: <description>` → minor bump
  - `fix: <description>` → patch bump
  - `feat!:` or `BREAKING CHANGE:` in body → major bump
  - `chore:`, `docs:`, `refactor:`, `test:` → no release bump (still appear in CHANGELOG)

### Files

| File | Action |
|---|---|
| `.github/workflows/release-please.yml` | New — runs on push to `main`, drives release-please then (future) Nix hash update |
| `release-please-config.json` | New — package type `simple`, `flake.nix` in extra-files |
| `.release-please-manifest.json` | New — bootstraps at `0.0.0` so first release proposes `0.1.0` |
| `flake.nix` | New — adapted from imds-broker; `version` line marked with `# x-release-please-version` |
| `flake.lock` | New — copied from imds-broker (pins nixpkgs and flake-utils) |

### release-please config details

- Release type: `simple` (correct for a Go CLI — `go` type is for published libraries)
- `extra-files`: `["flake.nix"]` — release-please updates the line marked `# x-release-please-version`
- Starting manifest: `"0.0.0"` — release-please will propose `0.1.0` as the first release given a `feat:` commit in history

### Workflow details

- Trigger: `on: push: branches: [main]`
- Permissions: `contents: write`, `pull-requests: write`
- Uses `google-github-actions/release-please-action@v4`
- Outputs: `release_created` (bool), `tag_name`, `upload_url` — used by Phase 3 to conditionally run Nix hash update steps

### Manual verification after Phase 2

- [ ] Merge a `feat:` PR to `main` and confirm release-please creates a release PR
- [ ] Confirm the release PR contains correct CHANGELOG entry and `flake.nix` version bump
- [ ] Merge the release PR and confirm:
  - Tag `v0.1.0` is created on `main`
  - goreleaser fires and publishes the release (Phase 1 workflow)

---

## Phase 3 — Nix vendorHash automation

The `vendorHash` in `flake.nix` must match the Go module dependency state of the released version exactly. It is semantically tied to the release, not to individual feature PRs, so it belongs in the release preparation step alongside the version bump and CHANGELOG.

### How it fits into the workflow

The release-please action runs first and creates or updates the release PR branch. Phase 3 adds conditional steps immediately after in the same `release-please.yml` job. When `prs_created == 'true'`, the steps check out the release PR branch, compute the correct hash, and push a commit back to it. The release PR then contains version bump, CHANGELOG, and the correct `vendorHash` — all before you merge it.

**No loop risk**: the workflow triggers only on `push: branches: [main]`. The vendorHash commit pushes to the release PR branch (not `main`), so it cannot retrigger the workflow.

### Conditional execution

The release-please action outputs:
- `prs_created` — `'true'` if a release PR was created or updated
- `pr` — JSON object; `headBranchName` field contains the release PR branch name

Steps run under `if: steps.release-please.outputs.prs_created == 'true'`. The branch is accessed via `fromJson(steps.release-please.outputs.pr).headBranchName`.

### vendorHash computation approach

Nix reports the correct hash when a build fails due to a hash mismatch. The steps exploit this:

1. Patch `flake.nix`: replace the existing `vendorHash` value with a known-wrong SRI hash (`sha256-AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=`)
2. Run `nix build .#dollop`; capture stderr
3. Parse the correct hash from the `got:` line in the error output
4. If the correct hash differs from what was in the file before step 1, patch `flake.nix` with the correct value and commit; otherwise no-op

The commit is attributed to `github-actions[bot]` and the message is a `chore:` so it does not affect the release version.

### Files

| File | Action |
|---|---|
| `.github/workflows/release-please.yml` | Add conditional Nix steps after release-please action |

No new files. The additional steps extend the existing workflow job.

### New workflow steps (outline)

```
- checkout release PR branch (ref: headBranchName from pr output)
- DeterminateSystems/nix-installer-action
- shell: capture original hash; set fake hash; nix build; parse got: line;
          if changed: write correct hash, git commit, git push
```

The checkout step uses `persist-credentials: true` (the default) so the push succeeds with `GITHUB_TOKEN`. No additional secrets are required — `contents: write` is already on the job from Phase 2.

### Manual steps

- [ ] **Set initial vendorHash in `flake.nix`** before merging Phase 2: run `nix build` locally with a fake hash and copy the value from Nix's error output — OR — push a draft with `sha256-AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=` and let Phase 3 automation correct it on the first release PR

### Manual verification after Phase 3

- [ ] Merge a `feat:` PR to `main` and confirm the release PR branch receives a second commit from `github-actions[bot]` updating `vendorHash`
- [ ] Confirm `nix build` succeeds locally (or in CI) against the release PR branch

---

## Notes

- `flake.lock` will be pinned to the nixpkgs revision from imds-broker initially; run `nix flake update` in dollop to refresh it when needed
- Individual feature PRs do not touch `flake.nix` — all flake updates (version and hash) are automated at release time
