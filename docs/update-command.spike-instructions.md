# Phase 1 spike — execution instructions (hand-off)

> **You are the executing agent.** The spike code is already written and pushed
> (`cmd/spike/main.go`). Your job is to run it against a **real, configured R2
> bucket**, capture the output, decide the stop-and-go gate, and record the
> result. The authoring environment had no R2 credentials, so the empirical run
> could not be done there.
>
> Plan: `docs/update-command.plan.md` (Phase 1)
> Gate to resolve: `docs/prd.md` → "Update Command Reconciliation" → "Spike gate"
> Branch: `plan/update-command`

## What this spike answers

The `update` touch pass (PRD §4.3/§4.4) resets the lifecycle expiry clock across
a prefix by issuing a `CopyObject` self-copy (same source and destination key)
with `MetadataDirective: COPY`. This run de-risks two assumptions:

1. **Does R2 permit** a self-copy with `MetadataDirective: COPY`?
2. **Does it advance `Last-Modified`** (T1 > T0)?

This is genuinely uncertain because **standard AWS S3 rejects a no-op self-copy**:

> This copy request is illegal because it is trying to copy an object to itself
> without changing the object's metadata, storage class, website redirect
> location or encryption attributes.

On S3 you must use `MetadataDirective: REPLACE` (or change storage class /
encryption / tags). R2's behaviour for this exact case is **undocumented** — the
[R2 S3 extensions](https://developers.cloudflare.com/r2/api/s3/extensions/) and
[object lifecycle](https://developers.cloudflare.com/r2/buckets/object-lifecycles/)
docs are silent on self-copy and on what resets the expiry clock. R2 does add a
third `MERGE` directive value.

The spike probes **five variants** in a single run, so the gate is decidable even
if `COPY` is rejected the way it is on S3:

| Variant | Directive | Metadata change | Notes |
|---|---|---|---|
| `COPY` | `COPY` | none (no-op) | the PRD's literal choice; S3 rejects this |
| `REPLACE` | `REPLACE` | none, Content-Type re-sent | S3 rejects (resulting metadata identical) |
| `MERGE` | `MERGE` (R2 ext) | none (no-op) | preserves source metadata |
| `REPLACE+meta` | `REPLACE` | `x-amz-meta-dollop-touched-at` (changes each run), Content-Type re-sent | the realistic, shippable touch |
| `MERGE+meta` | `MERGE` | `x-amz-meta-dollop-touched-at` (changes each run) | shippable touch, preserves other metadata |

The `+meta` variants carry a **benign user-metadata key** with a value that
differs every call. It is invisible to browsers/`curl` (it only returns as an
`x-amz-meta-*` response header) but it makes the self-copy legal under S3
semantics. These are the variants the real `update` touch would use; the bare
directives establish whether R2 even needs the workaround.

## Prerequisites

- A machine/environment with **R2 credentials configured for dollop**:
  - `dollop config set bucket <bucket>`
  - `dollop config set account-id <cloudflare-account-id>`
  - `dollop config auth r2-key <r2-access-key-id>`
  - `dollop config auth r2-secret <r2-secret-access-key>`
  - (the keyring **or** the plaintext fallback file both work — the spike uses
    the same `config.NewFallbackStore` wiring as `cmd/dollop`)
- The bucket should be one where writing/deleting a throwaway key
  (`spike/touch-probe`) is acceptable. The spike cleans up after itself unless
  `-keep` is passed.
- Go 1.26 via `mise` (use `mise exec -- go ...` or `just`).

## Steps

1. Check out `plan/update-command` and confirm it builds:
   ```
   mise exec -- go build ./cmd/spike
   ```
2. Run the spike (do at least one default run; a `-keep` run is useful for manual
   inspection in the R2 dashboard):
   ```
   mise exec -- go run ./cmd/spike
   mise exec -- go run ./cmd/spike -sleep 5s    # optional: larger T0→T1 gap
   ```
   Flags: `-key <key>` (scratch key, default `spike/touch-probe`),
   `-sleep <dur>` (gap before the copy, default `3s`),
   `-keep` (don't delete the scratch object).

## What to capture and document

Record the following in **`docs/update-command.spike-results.md`** (create it),
and copy the verdict into the locations listed under "Gate" below:

1. **Raw output** — paste the full stderr+stdout of at least one run, e.g.:
   ```
   bucket=<bucket> key=spike/touch-probe sleep=3s
   T0 Last-Modified: <RFC3339Nano>
     COPY          <accepted|REJECTED> ...
     REPLACE       <accepted|REJECTED> ...
     MERGE         <accepted|REJECTED> ...
     REPLACE+meta  <accepted|REJECTED> ...
     MERGE+meta    <accepted|REJECTED> ...
   === SPIKE RESULT ===
   <PASS|FAIL>: ...
   ```
2. **Per-variant table** — for each of the five variants
   (`COPY`, `REPLACE`, `MERGE`, `REPLACE+meta`, `MERGE+meta`): accepted vs
   rejected; if rejected, the R2 error message; if accepted, whether
   `Last-Modified` advanced and by roughly how much. Call out explicitly whether
   any **bare** (no-op) directive works — if so, the touch needs no metadata
   workaround; if not, identify the cheapest **+meta** variant that does.
3. **Environment notes** — R2 region/jurisdiction if non-default, account/bucket
   used (no secrets), and any retries or anomalies.
4. **Clock-reset interpretation** — does an advanced `Last-Modified` actually move
   the object's lifecycle expiry? `Last-Modified` advancing is the spike's proxy;
   if you can also observe the `x-amz-expiration` header (returned on `PutObject`/
   `HeadObject` when a lifecycle rule matches) before vs after the copy, note
   whether it shifts. This is a bonus signal, not required to pass the gate.

## Gate — decide and record

Resolve the stop-and-go gate based on the run:

- **PASS** (at least one variant advances `Last-Modified`):
  - In `docs/prd.md`, under the "Spike gate" note, record **which variant(s)**
    work and that the touch approach is validated.
  - If a **bare** directive works, the touch is a plain no-op self-copy — record
    that and prefer it (no metadata churn).
  - If only the **+meta** variants work, update PRD **§4.4**: the touch is a
    self-copy carrying a changing `x-amz-meta-dollop-touched-at`. Prefer
    `MERGE+meta` (preserves all other metadata); fall back to `REPLACE+meta`,
    which must re-send `Content-Type` (and `Cache-Control` where set) because
    `REPLACE` drops unspecified system metadata.
  - Phase 2 may then begin.
- **FAIL** (no variant advances `Last-Modified`):
  - Do **not** start Phase 2. Revise the PRD reconciliation. Candidate
    alternatives to document: re-`PutObject` the object bytes (read-modify-write
    per object), or drop the touch pass and accept per-object expiry variance.

Then update the checklist in **`docs/update-command.progress.md`** (Phase 1) and
**delete `cmd/spike/`** before merge (it is throwaway by design).
