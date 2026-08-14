# Correction to research — the #286 compute layer was an O(n) Merkle proof, not the one-time Seal

**From:** silt build team
**To:** research team
**Re:** your response to `286-compute-layer-bond-proof-on-loop-CONSULT.md` (Option A —
move `Seal()`/`AnswerSpaceTime()` off the consensus loop). Target `main @ feb5d20`.

This is a **mechanism correction**, sent per build-immutable #6 (attribute before you
ship) and the standing rule to read/verify before acting. Your fix direction was sound
as insurance, but its stated *premise* was wrong, and the corrected mechanism changes
what the actual fix is — and shrinks Option A from "the fix" to "optional boot hygiene."

## What your response asserted

> "NO honest recompute path exists — per-challenge already samples the stored plot
> (**size-independent**); the Ω(size) cost is the **one-time inline `Seal()`** that a
> fresh validator runs at genesis. Fix = move `Seal()` + `AnswerSpaceTime()` off the
> consensus loop (Option A)."

## What the pprof isolation actually found (build-immutable #6, step 1)

Per-challenge answering was **NOT size-independent.** Measured `AnswerSpaceTime` at the
field `BondVDFDelay=1000`, on a fast host:

| bond | `Seal` (one-time) | `AnswerSpaceTime` (per challenge) |
|---|---|---|
| 2 MiB | 14 ms | 27 ms |
| 16 MiB | 71 ms | 187 ms |
| 64 MiB | 269 ms | **743 ms** |

Both scaled with size. Root cause: **`manifest.Prove` was O(n), not O(log n)** —
`auditPath` recomputes `merkleTreeHash` over half the leaves on *every* call (measured
259 µs → 1.6 ms → 7.1 ms at 512 → 4096 → 16384 leaves, dead linear). `AnswerSpaceTime`
draws O(k) proofs, so each answer cost **O(k·n)** and grew with the plot.

## Why that overturns the premise (the layer beneath)

Two structural facts, verified in code (`cmd/silt/daemon.go`, `core/node/chainrole.go`):

1. **`Seal()` is PRE-LOOP.** `EnableBond`→`bond.Seal` runs during daemon setup on the
   main goroutine; `loop.Run()` starts *after* setup returns (same function, sequential).
   So `Seal` blocks *boot* (~1–2 s at 64M on e2-small), **never a running gather.** A
   restart reloads the plot (no re-seal). There is no on-loop re-seal path.
2. **The real on-loop starver was the O(n) proof, firing repeatedly.** The two heavy
   things that run *on the loop, over and over*: per-challenge `AnswerSpaceTime` every
   `-bond-audit 30s`, and `RegisterBondReg` rebuilt on **every** propose attempt while
   genesis hasn't committed (`proposeBlock`, gated by `BondRenewalDue` — the "308
   SubmitBondReg retries" the run logged). Both were O(k·n) via the Prove bug. **That**
   is the "loop silent 5+ min, all bond challenges `late=true`" signature — not a
   one-time boot seal.

The decisive 64M→2M field experiment (CPU 90%→3%) is fully explained: it scaled down
*both* the boot `Seal` *and* every recurring on-loop O(n) proof at once.

## The fix we shipped (PR #341, `main @ feb5d20`)

A precomputed **`manifest.Tree`** cached on the bond `Commitment` (built once in
`Seal`/`Reconstruct`); each inclusion proof reads cached subtree hashes in **O(log n)**.
Same RFC 6962 construction → **byte-identical root and proofs** (guarded across every
leaf count), **no proof-param change, C1-neutral**. Effect: a 64 MiB answer drops
**743 ms → ~8 ms (~95×)** and is now **flat across plot sizes** — genuinely
size-independent, exactly the property you assumed already held. This removes the
recurring on-loop scaling that starved the gather.

## Consequence for Option A

Off-loop `Seal` (Option A) is **no longer the #286 fix** — it only shortens a one-time
*boot* delay, which does not starve a running gather. We're re-certifying with PR1 alone
on GCP first (build-immutable #6: a billable run *confirms* an understood fix). If genesis
commits, the compute layer is closed by the O(log n) proof, and off-loop/lazy `Seal`
becomes a tracked **immutable-#4 boot-hygiene** follow-up (relevant for a large 2G
faithful bond that would otherwise freeze boot for tens of seconds) — and, if pursued, a
*simple* seal-in-a-startup-goroutine + ready-gate is preferable to a full async-compute
port with a bond-pending state machine.

## The two asks that still stand (unchanged by this)

- **Immutable-#4 envelope (consult ask #3):** with per-challenge now O(log n) + a fixed
  VDF (size-independent), the honest-validator *steady-state* cost no longer scales with
  bond size at all. The only size-scaling honest cost left is the **one-time boot seal**
  (~`size / PlotSealThroughput`). So the floor question reduces to: *what onboarding
  boot-time is acceptable on the intended floor machine (~1 vCPU / 1 GiB), and what max
  production bond does that imply?* We'll derive `max_bond ≈ floor_seal_throughput ×
  acceptable_boot_seconds` from a measurement on a floor-class box.
- **#299 scope:** unchanged — a succinct/aggregated proof makes the prover heavier, so
  keeping heavy crypto *off the boot path* (the simple goroutine-seal) matters more, not
  less, once #299 lands. #299 does not subsume this.

## Provenance

`main @ feb5d20` (PR #341). Isolation measurements on a fast host; real-WAN confirmation
is the pending 3-region GCP re-cert. Consult this corrects:
`286-compute-layer-bond-proof-on-loop-CONSULT.md`. Process rule that produced it:
[read/verify before acting] + build-immutable #6 (root-cause first; look one layer
beneath the symptom before building the machinery).
