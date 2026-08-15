# Fix for factor (ii) of the MATURING drain wall: bond-reg head-window (A) + durable pending queue (B)

**Date:** 2026-08-15
**Follows:** `2026-08-15-maturing-wall-is-coordination-cadence-not-cpu.md` (the wall is cadence, not CPU) and the PE cadence ruling (`silt-reviews/.../maturing-cadence-ruling-PE-2026-08-15.md`).

## The defect (factor ii), root-caused

A bond registration is signed over `BondRegNonce(prev)` — bound to ONE specific head — and `validateBondRegs` / `ValidateBondReg` accepted it only against the **current** head. So the instant the head advances (a heartbeat or any block commits), a reg in flight goes **stale** and is refused; the submitter must resign over the new head and race the moving head again. Over a real WAN — where a proposer proposes on head-advance **before** the WAN-delayed resubmission arrives — this starves the drain: blocks commit **empty below the #286 byte cap** (measured: 13 blocks, only 4 with `entries=1`), maturity never reaches bar-2 in the window (~1 reg/105s, ~3× slower than the byte cap alone). It never showed in single-host sims because there is no propagation delay there.

The instrumented field run (7134711-18163) is what surfaced it: the goroutine was ≤7% busy, so it was never CPU — the drain was coordination-limited by this staleness race.

## The fix

**(A) Widen bond-reg validity to the last K committed heads.** `validateBondRegs(b)` and `ValidateBondReg(r)` now accept a reg if its proof validates against `BondRegNonce(h)` for any h in the last `BondRegHeadWindow` committed heads (walked deterministically via `recentBondRegNonces`/`blockByHash`). Default K=8 (`DefaultBondRegHeadWindow`). This removes the one-head brittleness so a reg survives WAN propagation + one proposer rotation.

**(B) Durable pending queue.** `proposeBlock` (chainrole.go) now KEEPS the valid regs that didn't fit the byte cap (dropping only the embedded/committed and no-longer-valid ones) instead of wiping the whole pending map and relying on a re-broadcast to refill it. Composes with (A): together the drain runs at ~1 reg/block (the cap rate) instead of racing the head.

## Invariant analysis (I1–I5) — verified green by the #406 model-check tier

- **I1 (quorum intersection):** unaffected. Bond regs don't change the finality-quorum rule, only which validators are bonded. The window admits the SAME honest regs a few heads later; it does not admit invalid or duplicate regs (`validateBondReg` still checks size/floor/signature/space-time proof against each candidate nonce; commit dedups via `bondRegHeight`). The bonded set is identical, just drained reliably.
- **I2 (never sign twice):** unrelated (no attestation/signing change).
- **I3 (set changes at finalized boundary by weight):** the weight rule and the `EpochBlocks` boundary snapshot are unchanged; the fix affects only WHEN a reg commits, not what weight it carries or which epoch counts it. It strengthens I3's *premise* (maturity becomes reachable) without touching the rule.
- **I4 / I5:** unrelated to bond-reg admission.

The full launch + handoff model-check tier (I1/I2/I3/I5 + #357/#397/#402 replays) stays green under the change.

## The C1 security parameter (K) — bounded, and research-gated

Widening admits a reg whose space-time proof was computed up to K heads ago, so it weakens proof-freshness by K×block-time. This is **safe and bounded** because:
1. **K ≪ BondTTLBlocks** — standing already lapses without a fresh proof within the TTL, so a K-head window (8) is a small fraction of the freshness the TTL already enforces.
2. **Continuous bond-audit backstops it** — a released bond fails the next audit challenge (a fresh unpredictable nonce, every `BondAuditInterval`) and decays out, so a released-and-replayed old reg cannot persist.
3. **The window is bounded, not "accept forever"** — pinned by `TestBondRegRejectedBeyondHeadWindow_factorII`: a reg stale beyond K is rejected.

The exact K vs the anti-release/reseal window is the one thing I've flagged for a research security-check (build-immutable #3/#4: never fuse a security signal with a perf knob — so K is a *conservative* default, not tuned for drain speed). The mechanism doesn't rely on K being large; even K=2 removes the one-head brittleness.

## Verification

- **Failing-first:** `TestBondRegStaleAfterOneHead_factorII` encodes the desired behavior (a one-head-stale reg still validates and commits) — RED on the old single-head rule, GREEN with (A).
- **Security bound:** `TestBondRegRejectedBeyondHeadWindow_factorII` — a reg beyond K is rejected.
- Full `go test -short ./...` + the #406 model-check tier green; no regression in the #338 idle-drain or #286 genesis-spread tests.

## What this does NOT close (still open, per the PE ruling)

- **The re-run** that confirms maturity is reachable-within-the-computed-bound over a real WAN, and measures the fresh-publisher completion (strand a: bounded vs unbounded). This fix should let maturity land; the re-run proves it and computes the principled harness bound (blocks-to-quorum-weight × block-time) that replaces the arbitrary 420s.
- **#299** (succinct proof) remains the high-priority M1 speedup that shrinks the ~1.5 MB proof so factor (i)'s byte cap admits many regs/block — orthogonal to this fix.
- The **K research-check** (above).
