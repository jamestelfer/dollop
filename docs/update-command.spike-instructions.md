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
third `MERGE` directive value. The spike therefore probes **all three** directives
(`COPY`, `REPLACE`, `MERGE`) in a single run, so the gate is decidable even if
`COPY` is rejected the way it is on S3.

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
     directive COPY    <accepted|REJECTED> ...
     directive REPLACE <accepted|REJECTED> ...
     directive MERGE   <accepted|REJECTED> ...
   === SPIKE RESULT ===
   <PASS|FAIL>: ...
   ```
2. **Per-directive table** — for each of `COPY`, `REPLACE`, `MERGE`:
   accepted vs rejected; if rejected, the R2 error message; if accepted, whether
   `Last-Modified` advanced and by roughly how much.
3. **Environment notes** — R2 region/jurisdiction if non-default, account/bucket
   used (no secrets), and any retries or anomalies.
4. **Clock-reset interpretation** — does an advanced `Last-Modified` actually move
   the object's lifecycle expiry? `Last-Modified` advancing is the spike's proxy;
   if you can also observe the `x-amz-expiration` header (returned on `PutObject`/
   `HeadObject` when a lifecycle rule matches) before vs after the copy, note
   whether it shifts. This is a bonus signal, not required to pass the gate.

## Gate — decide and record

Resolve the stop-and-go gate based on the run:

- **PASS** (at least one directive advances `Last-Modified`):
  - In `docs/prd.md`, under the "Spike gate" note, record **which directive(s)**
    work and that the touch approach is validated.
  - If `COPY` is **rejected** but `REPLACE`/`MERGE` works, update PRD **§4.4** to
    use the accepted directive. Note that `REPLACE` drops unspecified system
    metadata, so the implementation must re-send `Content-Type` (the spike does
    this); `MERGE` preserves source metadata and is the closest analogue to the
    original `COPY` intent — prefer `MERGE` if it works.
  - Phase 2 may then begin.
- **FAIL** (no directive advances `Last-Modified`):
  - Do **not** start Phase 2. Revise the PRD reconciliation. Candidate
    alternatives to document: re-`PutObject` the object bytes (read-modify-write
    per object), or drop the touch pass and accept per-object expiry variance.

Then update the checklist in **`docs/update-command.progress.md`** (Phase 1) and
**delete `cmd/spike/`** before merge (it is throwaway by design).
