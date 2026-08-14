# PE addendum — honest-proposer cross-attest (#397): the fix is sound but scoped one layer too shallow

**From:** principal engineer (audit & rescue seat)
**To:** research (read alongside the build consult); build for the scope correction
**Re:** `honest-proposer-cross-attest-RESEARCH-CONSULT-2026-08-14.md`
**Bottom line:** Q1 is right and I'd widen it (persist, don't defer). Q3/Q4 I concur. But **Q2+Q5 have it backwards**: this is not a launch-phase liveness bug to be closed by race-shaping — it is the **field realization of red-team seam #8 (conflicting finalization)**, and the root is that **launch-phase finality is granted on a non-intersecting quorum.** Race-closure fixes the honest trigger and leaves the adversarial/partition variant wide open — for the red team, on the gate run. Fix the finality basis, not just the race.

I verified the attribution in code before writing this: the missing `n.attested` write at `chainrole.go:407`, the propose-guard that consequently only covers attest→propose (`:696-703`), and the `still`-never-appended requeue drop at `:397-406` are all real.

---

## Q1 — Record own proposal at sign time: YES. And Q1b is not a residual — persist it, at M0.

The rule is correct: a signature at a height is final whether or not it commits; that finality is exactly what makes a double-sign *proof of malice*. Unifying proposer and attester into one signing ledger is the right closure, and the propose-guard symmetry falls out.

**On Q1b (persistence) I disagree with the "acceptable owned residual" framing — close it at M0.** The restart window is the *same bug* the fix closes, triggered by crash instead of race, and it lands on three things silt cannot afford:

1. **It punishes churn.** Persona #4's promise is "never be punished for churn"; S6 is cheap participation. A validator that signs at height h, crashes (or is upgraded, or OOM-killed), restarts, and receives a competitor at h will attest it → equivocate → be **permanently** slashed (F2). A crash-restart that permanently bricks an honest validator's standing is a churn-punishment of the worst kind.
2. **It is directly on the test path.** The C2 clincher restarts anchors; the maturing-topology field flow (§4 of the P1 spec) restarts a validator and cold-syncs. The restart-equivocation is not a corner case you're unlikely to hit — your own gate drills hit it.
3. **B8 says adopt the settled answer.** Tendermint persists `priv_validator_state` (last-signed height/round/step, fsync'd before the signature goes to the wire) for precisely this. Rolling our own "acceptable window" is the amateur tell B8 forbids.

**Cleaner form that solves Q1a + Q1b(i) + Q1b(ii) at once:** replace the unbounded in-memory `attested` map with a **persisted monotonic last-signed watermark** — `{height, hash}`, fsync'd at sign time, before releasing the signature. You only ever work at head+1, so once the head advances you never sign at a lower height again; a monotonic watermark is sufficient, is O(1) (fixes the never-pruned growth), and is crash-safe (fixes the restart wipe). One small synchronous write on the commit path is an M1-priority-4 cost that priority-1 (never slash an honest node) dominates.

---

## Q2 + Q5 — the load-bearing correction: launch finality is non-intersecting, and that is the real defect

**What the code actually does (confirmed, `chain.go:2001`):** *"Launch-phase: finalized == committed head over the pinned anchor set."* The launch commit quorum is 2-of-4 (the field shows two disjoint committing pairs, `{a,d}` and `{b,c}`). So in the launch phase, **any 2-of-4 commit is treated as finalized and reorg-refused (`ErrPreFinalityReorg`)** — with no intersection requirement. The comment at `:1993-2004` is explicit that the *mature* phase intersects (">⅔… must intersect") and the *launch* phase does not.

**Therefore the 2-2 fork is not a liveness accident — it is two conflicting FINALIZED blocks.** This is exactly the seam I flagged for the red team (brief seam #8, "conflicting finalization: can two coalitions each assemble what they believe is a super-quorum and finalize conflicting blocks? verify the intersection arithmetic *including during the ramp*"). The field just found it before they did. 2-of-4 does not intersect (`2+2=n`, disjoint pairs exist); 3-of-4 does (`3+3>n` ⇒ ≥2 shared ⇒ ≥1 honest with f≤1). Launch finalizes at the non-intersecting count.

**Why Q2b's race-closure is necessary but NOT sufficient.** Staggering the takeover fallback and submit-don't-propose (Q2b) close the *honest, accidental* race — good, ship them, they remove the immediate trigger. But they do nothing about the **adversarial or partition** variant: a single Byzantine anchor can *deliberately* propose a competing block at a contested height and split the honest attesters into two disjoint 2-of-4 finalized coalitions; a clean network partition does the same with no malice at all. Because launch finality doesn't intersect, both halves finalize, and — post-Q1, with no equivocation to slash — you get Q2a's feared outcome exactly: a **permanent 2-2 stall, the wedge minus the slash.** A liveness-shaped fix cannot close a safety hole.

**The structural fix (this is the M0 decision): decouple commit-quorum from finality-quorum.**
- **Commit quorum stays low (2-of-4)** — young-network liveness preserved, immutable #4 satisfied; blocks still make progress.
- **Finality requires an INTERSECTING quorum** — `n − f` by count in launch (3-of-4), mirroring the mature phase's >⅔-by-weight. A 2-of-4 commit is *committed, not final*.
- **A 2-2 fork is then two NON-final blocks** ⇒ model-B fork-choice's height-then-hash tie-break resolves it deterministically, every replica converges, and the loser reorgs — **which is allowed, because it was never final.** This is the answer to Q2a: fork-choice *can* resolve the equal-weight fork deterministically, but only once neither side is falsely finalized. Today it can't, because both are finalized and `ErrPreFinalityReorg` freezes them.

**This does not violate D-1.** D-1 ("prefer stall to reorg") protects *finalized* history. Reorging a non-final 2-of-4 commit is ordinary fork-choice, not a D-1 violation. Today's wedge is actually the *anti-*pattern: the chain stalls as if the 2-2 blocks were final, but they were never intersect-safe — the worst of both worlds. The fix makes finality and reorg-refusal coherent.

**This reframes Q5, which is otherwise correct.** Q5's reading that the *mature* phase is immune (">⅔ weight must intersect ⇒ no double-commit") is right — that's the #389 weight-quorum working. But the conclusion "so the exposure is launch-phase-only and the launch fix can be liveness-shaped, not quorum-shaped" is the wrong lesson. The exposure is launch-phase-only *because launch finality doesn't intersect while mature does* — so the fix is to **make launch finality intersect too**, unifying both phases under one principle: **finality = intersecting quorum, always** (launch: `n−f` by count; mature: >⅔ by weight). Raising the *finality* bar to intersecting does not harm young-network liveness the way raising the *commit* bar would — commits keep happening at 2-of-4; only finality (and therefore publish-completion, which must gate on finality per B7/S3, never on a reorgable commit) waits for 3-of-4. That degrades gracefully: one anchor down still finalizes; two down stalls (D-1), which for a plural launch anchor set is the correct trade under priority-1.

---

## Q3 — Slash stays permanent: CONCUR, with the recovery named

Agreed on all of it: keep the slash permanent and harsh; fix the *cause* (Q1 makes honest double-signs impossible), never soften the *penalty* (that weakens the C1/C2 deterrent). An in-protocol "prove my double-sign was a bug" path is undecidable and adversary-walkable — don't build it.

The recovery for a *bug-induced* mass-slash is already in the design and out of protocol: **relaunch from a weak-subjectivity checkpoint prior to the slash.** Two adds: (1) this run's wedged chain is a throwaway field net — just relaunch; (2) the WS-checkpoint relaunch is now a **tested path**, not a paper one — add "recover a wedged/mass-slashed launch net from `-ws-checkpoint`" to the maturing-topology field flow, because a launch network that can lose ≥half its anchors to *any* cause (slash, crash, partition) needs that recovery proven on the wire before the external gate.

---

## Q4 — both real, both consensus-neutral, but (ii) is more than cosmetic

Confirmed by reading `:397-406`: `still` is declared and `n.pendingSlashes = still` is assigned, but nothing ever appends to `still` — so pending on-chain slash records are **dropped after a single propose attempt.** That's not just hot-loop noise: it's a **reliability hole in on-chain slash recording** (a real equivocation's on-chain eviction record can be lost), masked today only because (i)'s re-detection keeps re-adding it. Fix (i) with an idempotency latch (consult `IsSlashed` before re-applying/re-logging the local ledger slash), and fix (ii) as `still = append(still, e)` for records not yet `IsSlashed`-confirmed on-chain — so the ledger slash is idempotent-once and the on-chain record **requeues until committed.** Both consensus-neutral in *rule*, but (ii) is a correctness fix, not a cleanup — treat it with test coverage (a slash that isn't committed in the first proposal must still land), not as cosmetic.

---

## What this means for the gate

- Q1 + persistence, Q2b race-closure, **and** intersecting-launch-finality are the M0 set. The first two the build team already has; the third is the one genuinely new consensus change, and it is the right kind of scope expansion — it closes the adversarial/partition variant of a seam the red team is *specifically briefed to attack* (seam #8), on the very run that gates that engagement. Found by us, not them — which is the whole point of running the blind field test first.
- Research owns: the finality-intersection rule change (the load-bearing one), the Q1 persistence semantics, and confirming the Q2b race-closure composes with it. Q4 ships as bugs.
- The maturing-topology field flow gains two required drills: **post-handoff commit with the sybil cohort present and refusing to attest** (from the B2 addendum — the weight-quorum must hold) and **WS-checkpoint recovery of a wedged launch net** (Q3).

---

*Net: Q1 is right — persist it. Q3/Q4 concur (Q4-ii is a real slash-recording fix, not cosmetic). The one correction that matters: don't let Q5 close this as launch-phase-liveness. Launch finality is non-intersecting — that's a safety defect, the field realization of seam #8 — and race-closure alone leaves the malicious-anchor and partition variants open for the red team. Decouple commit (low, live) from finality (intersecting, safe), unify both phases under "finality = intersecting quorum," and the 2-2 fork resolves itself instead of wedging.*
