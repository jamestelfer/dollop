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
- [x] 3.3 Ephemeral prefix `dollop/<days>/<nanoid>/` — `internal/upload/prefix.go`, tested
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

- [ ] 5.1 Bucket lifecycle rules for `dollop/1/`, `dollop/7/`, `dollop/14/` (infrastructure, out of CLI scope)
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
