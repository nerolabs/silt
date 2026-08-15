> **⚠️ ATTRIBUTION SUPERSEDED (2026-08-15).** The `#382` attribution below is WRONG — a
> symptom-match to a cause that already shipped fixed (#384), the exact #6 trap. A code
> investigation (prompted by the PE ruling) overturned it AND the PE's follow-on
> issuer-withholding hypothesis: the publisher's `-peers` is the 4 anchors only (sybils never
> asked), and val-a's *no-egress* self-warm failed in 600s — an **anchor-side throughput wall**,
> not #382 and not issuer-withholding. See
> `2026-08-15-maturing-attribution-corrected-anchor-throughput.md`. The run *outcome* (9/10/0/1,
> handoff untested, 0 FAIL, safety held) and the A/B/C scope framing below remain accurate; only
> the root-cause attribution changed. Kept unedited as the learning trail.

# MATURING field run — blocked by #382 (participating-sybil load), not a safety defect

**Date:** 2026-08-15
**Run:** `9c3777d-73949` (MATURING=1 SYBILS=8, 21 nodes, 3-region WAN)
**Grade:** REVIEW — 9 pass / 10 gap / 0 fail / 1 skip. Torn down clean (33 destroyed).
**Author:** builder (autonomous), for PE/Research/Andrew review.

## What we set out to confirm

The young→mature handoff on a real WAN: `everMature` latch trips → anchors shed →
then the post-shed drills 10a-stall (D-1 prefer-stall), 10b-capture (B2 weight-priced
quorum: cheap MinBond members alone can't advance the mature chain), 10c-ws-cold-sync
(#399). This is the last M0 milestone before red team #183.

## What happened — the evidence chain (no guessing)

1. **The handoff never fired.** `10-maturing-handoff` GAP: the `everMature` latch never
   tripped within 420s — the bar-2 maturity precondition (bonded weight committed on-chain
   to clear Nakamoto ≥ `-mature-validators 2`) was never met. So 10a/10b/10c are all
   UNTESTED (they require a matured net).
2. **Why maturity never came: the chain starved.** `5-convergence` PASSED but only at
   **tip ~6** (durable, all 4 validators identical head — safety intact). The chain barely
   advanced the whole run.
3. **Why it starved: publish-token gather couldn't complete.** publish-diag (all 5+
   attempts): `ft_publish FAILED after 120s (token-quorum=2)` with
   **`publisher->validator reachability: 4/4 of the -peers set reachable`**. The publisher
   *reaches* every validator (TCP) but cannot assemble a 2-of-4 publish-token signature
   quorum inside the window. `last silt error: <none captured>` — it just times out.
   Meanwhile bond-challenges pass fine (standing=1024 for the 64M anchors, =16 for the 1M
   sybils) — the nodes are up and answering audits; they just can't turn the token round-trip
   in time under load.
4. **No node crashed** (no failed-nodes log). Not preemption, not a crash.
5. **Everything downstream cascades from (3).** 10 GAPs, but they collapse to one cause:
   nothing could publish/commit → 2-publish, 7-restart-content, 8-takedown, 9-cross-nat,
   chaos-crash, durability-turnover (all need a landed publish); 184-equivocation (needs the
   adversary's bond committed on-chain — same starvation); 184-partition (idle chain, no
   fork formed). 6-fault-tolerance GAP is the one possibly-separate note (quorum sizing with
   val-d down) — worth a second look but not the headline.

## Attribution: this is #382, already filed, already M1

#382 (OPEN): *"M1/liveness: participating 8-Sybil load slows the network (publishes time
out, Sybils lag ~11 blocks) — likely full-chain SyncChain O(chain×peers)/sweep + drain
serialization."* The signature matches line-for-line: token-quorum publishes time out;
downstream failures all trace to "nothing could publish/commit"; base convergence still
PASSES; it's a throughput/latency degradation, **not a safety break**.

The delta from the clean P1 run is confirmed in `topology.py`: under `MATURING=1` the 8
sybils run `-validator -objective` with `syb_bond = min_bond` and `-domain sybilnet` —
**participating in consensus**. That participation is precisely the load #382 documents.
P1 (launch regime) field-confirmed clean because its sybils weren't loading the consensus
path the same way.

**The MATURING run did not discover a new bug. It confirmed that the known #382 liveness
wall blocks the MATURING *field* drill.** The handoff/B2 *logic* itself is already certified
deterministically: I3 mature weight-quorum oracle (PR #414) + sim maturequorum + f1_latch,
all green. Field is meant to be confirmatory here, not the source of truth — but the field
substrate can't reach the state the drill needs.

## The decision this forces (needs PE/Research + Andrew — a milestone-scope call)

The core question is milestone scope, not a code bug: **Is field-confirming the mature/
post-shed regime an M0 gate, given a *known M1 efficiency* issue (#382) blocks that field
drill?**

- **Position A — #382 is an M0 blocker.** M0's Sybil corner includes the mature-regime B2
  capture-resistance as a headline claim. If we can't field-run the B2 drill because the net
  starves under the very sybil load that makes it meaningful, M0 isn't field-proven. Fix #382
  first.
- **Position B (my recommendation) — the handoff is M0-certified by the deterministic tier;
  #382 is M1.** The #406 tier was built exactly so field runs are confirmatory. The handoff/
  B2 safety is green in model-check + sim + netem. #382 is textbook M1 efficiency (SyncChain
  O(chain×peers) + drain serialization), and Andrew has consistently scoped M1 = efficiency.
  Point the red team at what's field-confirmed (launch/young regime, P1 green) **plus** the
  deterministic mature-regime certification, and be transparent that the mature-regime
  evidence is deterministic-tier + sim, not WAN-field. Pull #382 forward as the **top M1
  item** because it now demonstrably blocks a field drill we want.
- **Position C (cheap near-term unblock, compatible with B) — decouple drill from scale.**
  Re-run MATURING with a *reduced* participating-sybil count (enough for a meaningful B2
  capture majority — `n_syb//2+1`, so ~SYBILS=4–5 still forms a cheap-member majority
  attempting capture — but below the ~8 that trips #382), or a longer maturity window. This
  field-confirms the handoff *safety* now while #382-at-scale stays M1. Risk: too few sybils
  and the capture drill proves less; the count is a design choice worth a quick PE check.

**Why I'm flagging rather than just proceeding:** this is the exact "need a consult with
PE/Research" case Andrew named. It's not an execution blocker (I know how to do any of the
three); it's a scope + red-team-timing decision that is Andrew's + PE's to make. Re-running
the identical MATURING topology would just reproduce this GAP — no new information — so I'm
not burning another identical run. I've prepared Position C so execution is ready whichever
way it's ruled.

## What is NOT in question

Zero FAILs. Safety held everywhere testable: convergence durable + identical head, forged-
signature and under-bonded proposals rejected, privacy refuse-to-surveil held, web-UI guard
held. The #402 consensus fix is intact. This is a *liveness/throughput* wall, not a
correctness regression.
