# Phase 1 spike — results

Empirical run of `cmd/spike` against a real, configured R2 bucket on 2026-05-30,
resolving the Phase 1 stop-and-go gate (`docs/update-command.plan.md`, `docs/prd.md`
→ "Update Command Reconciliation" → "Spike gate").

**Verdict: PASS.** R2 accepts a self-copy under every directive tested —
including a bare `MetadataDirective: COPY` no-op — and every variant advances
`Last-Modified`. The PRD's literal choice (bare `COPY`) works; no metadata
workaround is needed.

## Headline findings

1. **R2 accepts bare no-op self-copy.** Unlike standard AWS S3 — which rejects a
   self-copy that changes nothing as "illegal" — R2 accepted all five variants
   with no error, across every run.
2. **Self-copy advances `Last-Modified`.** Every accepted variant moved
   `Last-Modified` forward to the copy time when measured against a baseline read
   immediately before its own copy.
3. **`Last-Modified` has 1-second resolution on R2.** This is the one trap. Two
   copies of the same object inside the same wall-clock second produce an
   identical `Last-Modified`, so a naive back-to-back before/after comparison can
   report "no change" for the later copies. It is a measurement-resolution
   artifact, not a failure to reset the clock (see "Resolution caveat" below).

## Per-variant results

All five variants were **accepted** by R2 (zero rejections). With each probe
measured against a baseline read immediately before its own copy (a distinct
clock second), all five **advanced** `Last-Modified`:

| Variant | Directive | Metadata change | Accepted | Advanced `Last-Modified` |
|---|---|---|---|---|
| `COPY` | `COPY` | none (no-op) | yes | yes (2–3 s delta) |
| `REPLACE` | `REPLACE` | none, Content-Type re-sent | yes | yes (2 s delta) |
| `MERGE` | `MERGE` (R2 ext) | none (no-op) | yes | yes (2 s delta) |
| `REPLACE+meta` | `REPLACE` | changing `x-amz-meta-dollop-touched-at` | yes | yes (2 s delta) |
| `MERGE+meta` | `MERGE` | changing `x-amz-meta-dollop-touched-at` | yes | yes (2–3 s delta) |

**Bare `COPY` works.** The touch needs no metadata workaround — it is a plain
no-op self-copy preserving all existing metadata. The `+meta` variants (the S3
workaround) are unnecessary on R2, as are the `REPLACE`/`MERGE` contingencies.

## Raw output

Per-probe run (the spike sleeps before each probe and compares against a fresh
baseline, giving one clean measurement per variant). Reproduced twice with
identical all-`advanced=true` results; bucket name redacted:

```
bucket=<bucket> key=spike/touch-probe sleep=2s
T0 Last-Modified: 2026-05-30T05:51:02Z
  COPY          accepted; before=2026-05-30T05:51:02Z after=2026-05-30T05:51:05Z advanced=true delta=3s
  REPLACE       accepted; before=2026-05-30T05:51:05Z after=2026-05-30T05:51:07Z advanced=true delta=2s
  MERGE         accepted; before=2026-05-30T05:51:07Z after=2026-05-30T05:51:09Z advanced=true delta=2s
  REPLACE+meta  accepted; before=2026-05-30T05:51:09Z after=2026-05-30T05:51:11Z advanced=true delta=2s
  MERGE+meta    accepted; before=2026-05-30T05:51:11Z after=2026-05-30T05:51:14Z advanced=true delta=3s
=== SPIKE RESULT ===
PASS: at least one self-copy variant advanced Last-Modified on R2.
```

## Resolution caveat (why a back-to-back run looks mixed)

The spike originally ran the five probes back-to-back, each compared against the
previous probe's result. Because the copies after the first all landed in the
**same wall-clock second**, and R2's `Last-Modified` is second-precision, only the
probes that happened to straddle a second boundary reported `advanced=true`:

```
bucket=<bucket> key=spike/touch-probe sleep=3s
T0 Last-Modified: 2026-05-30T05:47:07Z
  COPY          accepted; Last-Modified=2026-05-30T05:47:10Z advanced=true
  REPLACE       accepted; Last-Modified=2026-05-30T05:47:10Z advanced=false
  MERGE         accepted; Last-Modified=2026-05-30T05:47:10Z advanced=false
  REPLACE+meta  accepted; Last-Modified=2026-05-30T05:47:10Z advanced=false
  MERGE+meta    accepted; Last-Modified=2026-05-30T05:47:11Z advanced=true
=== SPIKE RESULT ===
PASS: at least one self-copy variant advanced Last-Modified on R2.
```

Here `COPY` advanced only because its baseline was `t0` (several seconds back);
`REPLACE`/`MERGE`/`REPLACE+meta` all wrote within `05:47:10` and so showed no
change; `MERGE+meta` tipped into `05:47:11`. Every copy still succeeded and
rewrote the object — the `advanced=false` rows are the resolution artifact, which
the per-probe rework above removes. The `-sleep 5s` and `-sleep 2s` back-to-back
runs showed the same shape.

**Implication for Phase 2:** the `update` touch must not depend on sub-second
`Last-Modified` deltas to confirm a touch succeeded. A successful `CopyObject`
response (HTTP 200) is the authoritative signal that the object was rewritten; do
not verify by re-reading `Last-Modified` and diffing, because two touches within
the same second are indistinguishable.

## Environment notes

- **Endpoint:** `https://<account-id>.r2.cloudflarestorage.com`, `Region: auto`,
  path-style addressing — default jurisdiction (no `eu`/`fedramp` prefix).
- **Bucket/account:** the operator's configured dollop bucket and account
  (redacted here; not recorded in the repo).
- **Scratch key:** `spike/touch-probe`, written and deleted each run (the spike
  cleans up unless `-keep` is passed).
- **Runs:** several back-to-back at `-sleep 2s/3s/5s` plus two per-probe runs;
  results were fully reproducible. No retries or anomalies.

## Clock-reset interpretation

`Last-Modified` advancing is the spike's proxy for the lifecycle expiry clock
resetting. R2's lifecycle rules measure object age from the upload/modification
time, and a self-copy rewrites the object in place with a new modification time,
so an advanced `Last-Modified` is the expected and sufficient signal that the
expiry window restarts. The `x-amz-expiration` header was not separately captured
(it is a bonus signal, not required to pass the gate); the `Last-Modified`
advance is conclusive for the gate.
