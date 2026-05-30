# dollop

## Problem Statement

Sharing files and directories temporarily or permanently from the command line requires either a third-party service (WeTransfer, Gist) or manual S3/R2 management. There is no simple personal CLI that uploads to self-hosted Cloudflare R2, generates a shareable URL, and handles expiry automatically.

## Solution

`dollop` is a Go CLI that uploads files and directories to a Cloudflare R2 bucket served via a public custom domain. Uploads are either ephemeral (under a `flash/<days>/<nanoid>/` prefix with R2 lifecycle-based expiry) or permanent (under `keep/<petname>/` with no expiry). The tool outputs a single URL to stdout so it composes cleanly with other shell tools.

## Requirements

### 1. General

1.1. The CLI shall be named `dollop`.
1.2. The CLI shall provide the subcommands `create`, `update`, and `config`.
1.3. The CLI shall write all progress and diagnostic output to stderr.
1.4. The CLI shall write only the final public URL to stdout.
1.5. The CLI shall infer the `Content-Type` of each file from its extension using the MIME type registry.
1.6. If a file extension has no known MIME type, then the CLI shall use `application/octet-stream`.
1.7. The CLI shall upload files sequentially, not concurrently.
1.8. The CLI shall use TLS for all connections to the Cloudflare R2 S3-compatible API.

### 2. Configuration

2.1. The CLI shall store configuration in a YAML file at `$XDG_CONFIG_HOME/dollop/config.yaml`, defaulting to `~/.config/dollop/config.yaml` when `$XDG_CONFIG_HOME` is not set.
2.2. The CLI shall store the R2 access key ID and R2 secret access key in the OS keyring under the service name `dollop`.
2.3. The configuration file shall support the following keys: `bucket`, `account_id`, `base_url`.
2.4. The `config set <key> <value>` subcommand shall write a key-value pair to the configuration file.
2.5. The `config get <key>` subcommand shall print the value of a configuration key to stdout.
2.6. The `config list` subcommand shall print all configuration file keys and values to stdout.
2.7. The `config auth <key> <value>` subcommand shall write the given key-value pair to the OS keyring under the service name `dollop`.
2.8. If a required configuration value is missing at command runtime, then the CLI shall exit with a non-zero status and print a descriptive error to stderr identifying the missing key.
2.9. If the OS keyring is unavailable when `config auth` is run, then the CLI shall exit with a non-zero status and print an error to stderr.

### 3. Create

3.1. The `create` subcommand shall accept a `--days` flag with allowed values of `1`, `7`, or `14`, defaulting to `1`.
3.2. The `create` subcommand shall accept a `--keep` flag that designates the upload as permanent.
3.3. When `--keep` is not set, the `create` subcommand shall generate a nanoid and place the upload under the key prefix `flash/<days>/<nanoid>/`.
3.4. When `--keep` is set, the `create` subcommand shall generate a petname and place the upload under the key prefix `keep/<petname>/`.
3.5. When the target is a single file, the CLI shall upload that file under the prefix with its original filename.
3.6. When the target is a directory, the CLI shall recursively walk the directory tree and upload every file, preserving relative paths under the prefix.
3.7. When uploading each file, the CLI shall set the `Content-Type` header to the inferred MIME type.
3.8. When each file upload completes, the CLI shall print the object key to stderr.
3.9. When all uploads complete, the CLI shall print the full public URL of the upload root to stdout, constructed as `<base_url>/<prefix>/`.
3.10. If any file upload fails, then the CLI shall print the error to stderr and exit with a non-zero status.

### 4. Update

4.1. The `update` subcommand shall accept a URL or nanoid/petname path as its first positional argument and a file or directory as its second positional argument.
4.2. When `update` is run, the CLI shall upload all files from the source (file or directory, recursively) to the existing prefix, overwriting any objects with matching keys.
4.3. When `update` is run on a directory, after uploading all source files, the CLI shall issue a `CopyObject` self-copy on every pre-existing object under the prefix that was not part of the current upload set.
4.4. The `CopyObject` self-copy shall use `MetadataDirective: COPY` to preserve existing user-defined metadata.
4.5. When each touch (self-copy) completes, the CLI shall print the object key to stderr.
4.6. When `update` completes, the CLI shall print the full public URL of the upload root to stdout.
4.7. If any upload or touch operation fails, then the CLI shall print the error to stderr and exit with a non-zero status.

### 5. R2 Lifecycle Rules (Infrastructure, not enforced by CLI)

5.1. The R2 bucket shall have three prefix-scoped lifecycle rules: delete objects under `flash/1/` after 1 day, `flash/7/` after 7 days, and `flash/14/` after 14 days.
5.2. The R2 bucket shall have no lifecycle rule on the `keep/` prefix.

## Implementation Decisions

- **S3 client**: `aws-sdk-go-v2` pointed at `https://<account_id>.r2.cloudflarestorage.com` with region set to `auto`.
- **Nanoid**: `github.com/matoous/go-nanoid/v2` for ephemeral upload IDs.
- **Petname**: `github.com/dustinkirkland/golang-petname` (2-word, hyphen-separated) for permanent upload names.
- **Keyring**: `github.com/zalando/go-keyring` for R2 credentials. Service name: `dollop`. Usernames: `r2-key`, `r2-secret`.
- **YAML**: `gopkg.in/yaml.v3` for config file serialisation.
- **MIME inference**: Go standard library `mime.TypeByExtension`. Falls back to `application/octet-stream`.
- **XDG config path**: resolved via a helper function that reads `$XDG_CONFIG_HOME`, not a library dependency.
- **Content-Type on CopyObject self-copy**: preserved via `MetadataDirective: COPY`; `Last-Modified` is reset by R2 automatically, which is the intended behaviour for resetting the lifecycle clock.
- **URL construction**: `<base_url>/<prefix>/` where `base_url` is the user-configured public custom domain attached to the R2 bucket (e.g. `https://drop.example.com`).
- **Update prefix resolution**: the `update` subcommand accepts either a full URL (from which the prefix is extracted by stripping `base_url`) or a bare prefix path (e.g. `flash/7/<nanoid>`).
- **Static site support**: correct `Content-Type` on each object (e.g. `text/html`, `text/css`) means R2 serves the files correctly for browser consumption without a Worker.

## Testing Decisions

- Unit test the XDG config path resolution function, covering presence and absence of `$XDG_CONFIG_HOME`.
- Unit test MIME type inference, including the `application/octet-stream` fallback.
- Unit test prefix construction for both ephemeral and keep paths.
- Unit test URL construction from base URL and prefix.
- Unit test `update` prefix extraction from both full URL and bare path inputs.
- Integration tests for `create` and `update` should be run against a real R2 bucket (or a local S3-compatible stub such as `minio`) and are optional for initial implementation.
- The keyring and S3 client should be injected as interfaces to allow unit testing of command logic without live credentials or network.

## Out of Scope

- `dollop delete` subcommand (reserved for future implementation).
- Concurrent multi-file uploads.
- Diffing source and destination on `update` (all source files are always uploaded).
- Any read-path Worker or expiry enforcement on access (expiry is best-effort via lifecycle rules).
- Authentication beyond static R2 API token (Wrangler OAuth, workload identity federation).
- Multiple account or bucket profiles.
- Progress bars or transfer rate display.
- Compression of uploads.

## Update Command Reconciliation

Section 4 was specified when `create` performed a plain 1:1 file→object copy.
`create` has since gained a rendering pipeline: markdown files render to HTML,
shared assets (`github-markdown.css`, `highlight-github.css`, `mermaid.min.js`)
upload once at the prefix root with a `Cache-Control` header, and `--index`
generates an `index.html`. This section reconciles §4 with that pipeline. It
amends the interpretation of §4; the numbered requirements stand.

- **§4.2 source set**: "all files from the source" means the full upload set the
  pipeline produces — planned source objects, rendered HTML, shared assets, and
  any generated index — not the raw source bytes. `update` reuses the `create`
  pipeline and honours `--no-render` and `--index`. Rendering is on by default.
- **§4.3 touch scope**: the self-copy pass resets the lifecycle expiry clock,
  which exists only on `flash/`. `update` runs the touch pass for directory
  updates on `flash/` prefixes only. `keep/` prefixes (no lifecycle) and
  single-file updates skip it. This narrows the literal "every pre-existing
  object" to preserve the requirement's intent without redundant `CopyObject`
  calls.
- **§4.3 exclusion set**: "objects not part of the current upload set" is every
  key listed under the prefix minus every key written during the run (sources,
  rendered HTML, shared assets, generated index). The upload pipeline reports its
  full written-key set so `update` can compute the difference.
- **§4.1 prefix resolution**: the bare prefix is recovered as the inverse of
  public URL construction — strip `base_url`, then strip any trailing filename,
  `index.html`, or slash. A bare prefix argument is accepted verbatim.
- **New capabilities required**: §4.3 and §4.4 need two S3 operations the tool
  does not yet have — listing every key under a prefix (paginated) and a
  single-key `CopyObject` self-copy with `MetadataDirective: COPY`. Both are added
  as injected interfaces alongside the existing uploader.
- **Stale objects** (consistent with Out of Scope: no diffing/deletion): source
  files removed between `create` and `update` leave orphaned objects under the
  prefix. `update` neither deletes them nor — because the touch pass runs on the
  whole prefix — exempts them from the expiry-clock reset. This is accepted.

### Spike gate

The touch behaviour depends on R2 advancing an object's `Last-Modified` when it is
self-copied with `MetadataDirective: COPY`. This is validated empirically by a
throwaway spike before interface or core work begins. The spike ends with a
stop-and-go assessment: if the self-copy does not reset `Last-Modified`, this
reconciliation and the touch approach are revised before implementation proceeds.

**Result (2026-05-30): PASS.** The spike ran against a real R2 bucket. R2 accepts
a self-copy under every directive tested (`COPY`, `REPLACE`, `MERGE`, and both
`+meta` variants) with no rejection, and every variant advances `Last-Modified`.
Bare `MetadataDirective: COPY` — the PRD's literal choice — works; unlike standard
S3, R2 does not require a metadata change to permit a self-copy, so no `+meta`
workaround or `REPLACE`/`MERGE` contingency is needed. One constraint surfaced:
R2's `Last-Modified` has **1-second resolution**, so the touch must treat a
successful `CopyObject` response as the authoritative success signal rather than
re-reading and diffing `Last-Modified`. Phase 2 may begin. Full results:
`docs/update-command.spike-results.md`.

## Further Notes

- The URL composability requirement (stdout-only URL) enables patterns like `open "$(dollop create file.zip)"` and `curl "$(dollop create --days 7 dir/)"`.
- The `flash/n/nanoid` path structure encodes expiry in the prefix, making lifecycle rules trivial and avoiding any per-object metadata dependency for expiry.
- `keep` paths use petnames rather than nanoids to be memorable and speakable — suitable for links shared verbally or in documentation.
- The `update` touch behaviour (self-copy on untouched objects) ensures the entire prefix has a consistent `Last-Modified` date after an update, avoiding variance in expiry across a directory upload.
- Static site hosting is a first-class use case: a directory containing `index.html`, CSS, and assets can be uploaded and served directly from R2 via the public custom domain without a Worker, provided `Content-Type` is correctly set on each object.
