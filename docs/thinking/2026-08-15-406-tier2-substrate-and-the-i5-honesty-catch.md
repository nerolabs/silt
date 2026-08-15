# 2026-08-15 — #406 tier 2: shipped the held-delivery substrate; the I5 oracle failed its own failing-first check (and why that's the right outcome)

**Context / trigger:** Building #406 tier 2 (the model-check over the REAL node loop via a
simnet held-delivery mode), aiming for the I2-restart and I5-honest-never-slashed (#397)
oracles that the chain-level tier can't reach.

## What shipped (solid, verify-first)

1. **`adapters/simnet` held-delivery mode** — `EnableHeldDelivery()` + `Pending()`/`Deliver(id)`/
   `DropPending(id)`. `Send` parks the delivery closure instead of `sched.AfterFunc`-ing it, so a
   driver fires messages in an order IT chooses. Off by default (existing sim/e2e untouched);
   conformance-tested (`helddelivery_test.go`: parks, fires in chosen order, drops, and re-checks
   death at deliver time). This is literally the "adversarial delivery scheduler" the design's
   option A specifies.
2. **Tier-2 substrate proof** (`core/node/modelcheck_tier2_test.go`): a REAL proposer gathers a
   REAL quorum and commits + broadcasts to every replica **entirely over driver-controlled
   delivery** — no clock advance, no auto-timers (request timeouts sit on simclock, which the
   driver never advances, so a gather completes purely by delivered replies). This proves the
   held-delivery layer drives the real node loop — the precondition for any adversarial tier-2
   oracle.

## What did NOT ship, and why (the honest catch)

I built an **I5 honest-never-slashed oracle** (two honest anchors race one height; assert no honest
node is ever slashed — the #397 catch). It passed. Then the failing-first check **failed**: with the
#397 propose-time watermark reverted (the pre-fix state), the oracle **still passed** — first under
arbitrary orders (fifo/reverse), then even under a *targeted* cross-attest-first order designed to
force the race. So the oracle was green for a weak reason — the #303 trap — in my own work.

**Root cause (attributed, not guessed):** #397's honest-slash requires **both** forks to COMMIT (so
`slashEquivocators` compares two chains and finds the cross-signer). But **#402's I1 structurally
prevents two blocks committing at one height**: two 3-of-4 anchor sets share ≥2 anchors, and each
attester records its attestation (the attest-side watermark), so a second block can never gather the
strict anchor majority. The losing proposal never commits, is never synced, and its double-sign
evidence never reaches a comparison. So in the *fixed* codebase, the tier-2 #397 slash cannot be
reproduced by the both-commit fork — **the fix I wanted to test is shielded by a different fix (#402).**
A test that cannot be shown to fail on the bug cannot claim to catch it.

**Decision:** ship the substrate (proven, honest); **do not ship the I5 oracle** (I cannot demonstrate
it failing-first here, so its "#397 catch" claim would be false). This is the session's own lesson
applied to my own work — a green oracle over a masked bug is worse than none (#303) — and the
failing-first discipline is exactly what surfaced it.

## The genuine I5 tier-2 oracle — design for the next increment

To make an honest I5/#397 tier-2 catch, one of:
- **Pre-#402 baseline:** run the racing-proposers scenario against a chain WITHOUT the derived anchor
  majority (e.g. `AnchorQuorum=1` and a lower quorum), so two forks CAN both commit; then the #397
  watermark is the sole thing preventing the honest slash, and reverting it goes RED. (Cleanest — it
  isolates #397 from #402.)
- **Sync the losing fork:** drive a topology/schedule where the losing proposal is committed on a
  replica and served on sync, so `slashEquivocators` sees the cross-signer even without a global
  fork (the seam-7 shape, but adversarially scheduled).
- **I2 across a real restart** is separate and cleanly testable via held-delivery: sign at h → Kill →
  Restart (reload the persisted mark) → deliver a competitor at h → must refuse. (`signmark_restart_test`
  covers one scenario; the model-check adds systematic restart-timing coverage.) This is the higher-
  confidence next tier-2 oracle.

## Status

Held-delivery mode + conformance + the round-commits substrate: DONE, green, ready to ship. I5/#397
tier-2 oracle: deferred with the design above (its failing-first story needs a pre-#402 baseline).
I2-restart tier-2 oracle: the recommended next build.
