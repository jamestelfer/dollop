# Changelog

## [1.9.0-alpha.0](https://github.com/jamestelfer/dollop/compare/v1.8.0...v1.9.0-alpha.0) (2026-06-13)


### Miscellaneous Chores

* migrate to shared chinmina release pipeline ([#43](https://github.com/jamestelfer/dollop/issues/43)) ([60f33ca](https://github.com/jamestelfer/dollop/commit/60f33cad1880b0c49884ab394d38610afe476421))

## [1.8.0](https://github.com/jamestelfer/dollop/compare/v1.7.0...v1.8.0) (2026-06-03)


### Features

* add favicon to rendered markdown pages ([#40](https://github.com/jamestelfer/dollop/issues/40)) ([09b0237](https://github.com/jamestelfer/dollop/commit/09b0237da61b827f5469e599526390b6bcea5eda))

## [1.7.0](https://github.com/jamestelfer/dollop/compare/v1.6.0...v1.7.0) (2026-05-30)


### Features

* add update subcommand to publish and overwrite content ([#36](https://github.com/jamestelfer/dollop/issues/36)) ([#38](https://github.com/jamestelfer/dollop/issues/38)) ([7106a80](https://github.com/jamestelfer/dollop/commit/7106a801ba8719b33bd5bd0fcec60c7e348bcd06))

## [1.6.0](https://github.com/jamestelfer/dollop/compare/v1.5.0...v1.6.0) (2026-05-29)


### Features

* fold markdown source links into rendered index entries ([#31](https://github.com/jamestelfer/dollop/issues/31)) ([c624ec2](https://github.com/jamestelfer/dollop/commit/c624ec295a86c89833a60d63e0edcb3960432b2d))
* page header with logo and source link, embedded template ([#35](https://github.com/jamestelfer/dollop/issues/35)) ([22118c9](https://github.com/jamestelfer/dollop/commit/22118c97254b7d935aa2c81cc4a09f0921f7f171))
* show relative path and rendered size in upload progress ([#33](https://github.com/jamestelfer/dollop/issues/33)) ([0a5cfa3](https://github.com/jamestelfer/dollop/commit/0a5cfa325a10f5764b37a4dff21b271db2144c2f))


### Bug Fixes

* fold markdown source links into rendered index entries ([c624ec2](https://github.com/jamestelfer/dollop/commit/c624ec295a86c89833a60d63e0edcb3960432b2d))

## [1.5.0](https://github.com/jamestelfer/dollop/compare/v1.4.0...v1.5.0) (2026-05-29)


### Features

* enhance doctor command with grouped output and storage info ([#30](https://github.com/jamestelfer/dollop/issues/30)) ([05abda5](https://github.com/jamestelfer/dollop/commit/05abda5832635a1cfa5ff601d1526a4a3f7d8718))
* group doctor checks into config, auth, and roundtrip ([05abda5](https://github.com/jamestelfer/dollop/commit/05abda5832635a1cfa5ff601d1526a4a3f7d8718))


### Bug Fixes

* prevent nil pointer crash when R2 credentials are unconfigured ([#28](https://github.com/jamestelfer/dollop/issues/28)) ([524426f](https://github.com/jamestelfer/dollop/commit/524426f6444cc869f10e2c3c9014677fe60aa0a6))

## [1.4.0](https://github.com/jamestelfer/dollop/compare/v1.3.0...v1.4.0) (2026-05-28)


### Features

* add --version flag ([#26](https://github.com/jamestelfer/dollop/issues/26)) ([51b2898](https://github.com/jamestelfer/dollop/commit/51b2898b191be2ab478042ee9ceb88b75cfe7c29))

## [1.3.0](https://github.com/jamestelfer/dollop/compare/v1.2.0...v1.3.0) (2026-05-28)


### Features

* render markdown files to HTML on upload ([#24](https://github.com/jamestelfer/dollop/issues/24)) ([21cec1d](https://github.com/jamestelfer/dollop/commit/21cec1d102f5b5873030c57d6ab6d3e3e6c0edfd))

## [1.2.0](https://github.com/jamestelfer/dollop/compare/v1.1.0...v1.2.0) (2026-05-26)


### Features

* add doctor command for end-to-end configuration and connectivity checks ([#21](https://github.com/jamestelfer/dollop/issues/21)) ([b6b3b16](https://github.com/jamestelfer/dollop/commit/b6b3b1668f6405c448487a9d6b88468b77dd0cd8))
* add plain-text auth fallback for headless Linux environments ([#22](https://github.com/jamestelfer/dollop/issues/22)) ([10a5b4e](https://github.com/jamestelfer/dollop/commit/10a5b4e253e59cb2ebfabc36f44dc1143d320d3c))

## [1.1.0](https://github.com/jamestelfer/dollop/compare/v1.0.1...v1.1.0) (2026-05-26)


### Features

* add --index flag to create command to generate a file listing ([#18](https://github.com/jamestelfer/dollop/issues/18)) ([d9644bd](https://github.com/jamestelfer/dollop/commit/d9644bde762f7a0bff45345e62d8ae6d3cd4e6bf))
* append filename suffix to public URLs for single files ([#20](https://github.com/jamestelfer/dollop/issues/20)) ([1fb8c42](https://github.com/jamestelfer/dollop/commit/1fb8c420c6b87867e4977fc746bc61e3ca1fc523))
* include filename in stdout URL based on upload contents ([1fb8c42](https://github.com/jamestelfer/dollop/commit/1fb8c420c6b87867e4977fc746bc61e3ca1fc523))

## [1.0.1](https://github.com/jamestelfer/dollop/compare/v1.0.0...v1.0.1) (2026-05-25)


### Bug Fixes

* serve markdown files as text/plain for browser rendering ([#16](https://github.com/jamestelfer/dollop/issues/16)) ([f86ab2b](https://github.com/jamestelfer/dollop/commit/f86ab2be4d27a6d6a7c214ec100f42493d973cdb))

## 1.0.0 (2026-05-25)

Initial release of `dollop` — a CLI for uploading files and directories to a
self-hosted Cloudflare R2 bucket and getting back a shareable public URL.


### Features

* **`create` command** — upload a file or directory to R2 and print the public
  URL to stdout. Progress and per-file keys are written to stderr so the URL
  composes cleanly with other shell tools (`open "$(dollop create file.zip)"`).

* **Ephemeral uploads (`--days`)** — place uploads under a
  `flash/<days>/<nanoid>/` prefix that maps directly to R2 lifecycle rules.
  Allowed values are `1`, `7`, and `14` days; defaults to `1`.

* **Permanent uploads (`--keep`)** — place uploads under a `keep/<petname>/`
  prefix with no expiry. Petnames (e.g. `keep/happy-otter/`) are memorable and
  speakable, suitable for links shared verbally or in documentation.
  `--keep` and `--days` are mutually exclusive.

* **Directory uploads** — recursively walk a directory tree and upload every
  file, preserving relative paths under the prefix. Enables static-site hosting
  directly from R2: upload a directory containing `index.html`, CSS, and assets
  and serve them via the public custom domain without a Worker.

* **Content-Type inference** — infer `Content-Type` from each file's extension
  via the standard-library MIME registry. Falls back to
  `application/octet-stream` for unknown extensions.

* **`config` subcommand tree** — manage tool configuration without touching
  credentials files:
  * `config set <key> <value>` — write a config key (`bucket`, `account_id`,
    `base_url`) to the YAML file.
  * `config get <key>` — print a single config value to stdout.
  * `config list` — print all config keys and values.
  * `config auth <key> <value>` — store R2 credentials (`r2-key`, `r2-secret`)
    in the OS keyring; credentials never touch the config file.

* **XDG-aware configuration** — config file lives at
  `$XDG_CONFIG_HOME/dollop/config.yaml`, defaulting to
  `~/.config/dollop/config.yaml` when `$XDG_CONFIG_HOME` is unset.

* **OS keyring credential storage** — R2 access key and secret are stored and
  retrieved via the OS keyring (`github.com/zalando/go-keyring`), keeping them
  out of plain-text config files and shell history.

* **CI pipeline** — GitHub Actions workflow runs `fmt + build + lint + test` on
  every pull request using `golangci-lint` and a conventional-PR-title check.
  Action references are pinned to commit SHAs with `persist-credentials: false`
  on all checkout steps.

* **Release pipeline** — `release-please` automates versioning and CHANGELOG
  generation from Conventional Commit messages.


### Developer tooling

* `justfile` with `verify` (fmt + build + lint + test), `build`, `test`, and
  `fmt` recipes via `mise exec`.
* `mise.toml` pins the Go version for reproducible local and CI builds.
* Claude Code session-start hook pre-builds the binary and installs tools so
  remote sessions are ready to use immediately.
