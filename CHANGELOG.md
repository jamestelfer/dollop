# Changelog

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
