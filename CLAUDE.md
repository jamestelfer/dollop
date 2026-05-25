# dollop

CLI tool that uploads files and directories to unique, expiring paths in a Cloudflare R2 bucket via the S3-compatible API.

## Go version

This project uses **Go 1.26**, which was released after the AI knowledge cutoff. Do not rely on training data for Go stdlib or dependency APIs — always fetch current documentation via Context7 before using unfamiliar APIs.

## Build and test

```
just verify    # fmt + build + lint + test (run before committing)
just build     # produces dist/dollop
just test      # go test ./...
just fmt       # gofmt -w .
```

## Project layout

```
cmd/dollop/main.go              entry point; wires config, keyring, uploader into commands
internal/config/                YAML config file (~/.config/dollop/config.yaml, XDG-aware)
                                + keyring access (r2-key, r2-secret via OS keyring)
internal/cli/configcmd/         config subcommand tree: set, get, list, auth
internal/cli/createcmd/         create subcommand: generates prefix, calls upload.UploadFiles
internal/upload/                uploader interface, S3 client, MIME detection, prefix logic
```

## Key conventions

- Dependencies are injected into commands (uploader, keyring, path functions) — keep it that way for testability.
- Credentials never go in the config file; only in the OS keyring (`config auth <key> <value>`).
- Prefix shapes: ephemeral `flash/<days>/<nanoid>/`, permanent `keep/<petname>/`.
- `cli.Exit(msg, code)` for user-facing errors; `fmt.Errorf("context: %w", err)` for propagated errors.
- Check errors from `fmt.Fprintln`/`fmt.Fprintf` when the write is the command's primary output; writes to stderr for progress/diagnostics do not need error checks.

## Major dependencies

Use Context7 for up-to-date documentation on all of these — do not guess at APIs.

| Library | Context7 ID | Notes |
|---|---|---|
| `github.com/urfave/cli/v3` | `/urfave/cli` | CLI framework; commands, flags, `cli.Exit` |
| `github.com/aws/aws-sdk-go-v2` | `/websites/aws_amazon_sdk-for-go_v2_developer-guide` | S3 client for Cloudflare R2 |
| `github.com/zalando/go-keyring` | `/zalando/go-keyring` | OS keyring (Set/Get/Delete) |
| `gopkg.in/yaml.v3` | — | Config serialisation; use `yaml` struct tags |
| `github.com/stretchr/testify` | — | `require` (fatal) / `assert` (non-fatal) in tests |
