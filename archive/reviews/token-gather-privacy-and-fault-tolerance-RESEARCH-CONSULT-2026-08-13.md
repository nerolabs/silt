# Research consult — parallel token-gather privacy stamp (§1) + fault-tolerance attribution (§2)

**From:** build (2026-08-13), routed by the principal engineer's ruling
**To:** research team
**Re:** `m0-candidate-remaining-issues-PE-RULING-2026-08-13.md`, which routes two items to you
**Status:** consult. **Item A** is a fast privacy stamp gating an M0-candidate latency fix. **Item B** is an attribution I have already reduced to a deterministic in-process repro — I need you to confirm the reading (or flag a mature-phase angle I missed) before I close it as "no consensus change."
**Provenance:** the PE certified §1 as a product defect in M0 scope, gave the build spec, and gated the merge on your privacy stamp. §2's branch is now named by a repro (below); the PE's conditional rulings said "build only after research certifies whichever consensus change applies" — the repro says *none* applies, which is itself a call I want you to confirm.

---

## Item A — Privacy stamp: parallelizing the publish-token gather (§1)

### The change (latency only, no protocol redesign)

The flagship privacy user story — a fresh ephemeral client acquiring a `-token-quorum k` blind-signed publish token — is failing/timing out over real WAN under load (two SYBILS=8 cloud runs: `ft_publish FAILED after 120s (token-quorum=2)`, and sometimes issuer-set discovery itself doesn't complete in 180s). It is latency + round-trip-count bound, not a reachability bug (validators 4/4 reachable). The PE-certified M0-minimal fix is to **parallelize the transport** of the k `MsgTokenRequest` round-trips (plus overlap/cache the issuer-key fetches and discovery), with adaptive size-scaled deadlines (build-immutable #5). No change to *what* is signed or *who* signs.

### The privacy argument we need you to stamp

The anonymity property of the publish token rests on **signer-set selection being publisher-independent**, because the token **reveals its signers** (`core/publishtoken/publishtoken.go` — per-signer sigs verified against per-issuer keys, so *which* validators signed is observable to anyone holding the token). The code already selects signers by a **network-canonical ranking** — "ranked by committed bond… the SAME for every publisher, so the signer subset can't narrow the publisher's anonymity set (R-3)" (`cmd/silt/swarm.go`).

**Our claim to stamp:**

> Parallelizing the *transport* of the token requests to the **fixed canonical k signers** (concurrent round-trips, then wait for that exact set) is **privacy-neutral**: selection remains latency-independent and identical for every publisher, so the revealed signer set is unchanged; blindness is per-message (the issuer sees only a blinded serial either way); and a shorter observation window is if anything privacy-positive. The one forbidden variant is **first-k-of-N-to-reply**, which makes the revealed signer set a function of the publisher's network position (nearest validators answer first) — a positional fingerprint stamped into every token, re-opening exactly the leak R-3 closed. We will fire concurrently but wait on the **fixed** canonical set, never first-k-to-reply.

**Question A1:** Is that argument sound as stated — is "parallel transport to a latency-independent fixed signer set" privacy-neutral, and is first-k-to-reply the only variant that breaks R-3, or is there a second-order channel (e.g. timing-correlation across the concurrent requests to the same issuer set, or the issuer observing *k concurrent* vs *k sequential* requests as a distinguisher)?

**Question A2 (idempotency, PE-flagged):** the sign request gets one bounded backoff retry. To avoid double-spend ambiguity we intend to **re-blind a fresh serial on retry**. Is that necessary, or are the blind-sign re-request semantics already idempotent at the issuer (i.e. re-presenting the same blinded serial to the same issuer is safe)? We want to retry without minting a spend-ambiguous second token.

**What we will NOT do without your yes:** ship the parallel gather. It is built-ready per the PE spec; the stamp is the only gate.

---

## Item B — Fault-tolerance attribution (§2): the repro names the branch, and it is NOT a consensus change

### The symptom and the PE's correction

`6-fault-tolerance` (commit with one honest validator down) **GAPs in both SYBILS=8 runs** but **passes in the no-sybil run**. My consult hypothesis was "banked sybil bonds inflate `bftThreshold` into the honest quorum." The PE correctly noted this **doesn't match the code**: `validatorSetSize()` returns `len(Anchors)` in the objective launch phase pre-handoff, so sybil bonds shouldn't touch the quorum — and demanded a repro naming the actual branch before anyone changes a quorum rule (#6).

### The repro (deterministic, in-process) and its verdict

`core/chain/TestFaultToleranceBranch_SybilBondsDoNotInflateLaunchQuorum` builds the exact committed state — 4 anchors + 8 banked single-domain sybil bonds, all attesting, pre-maturity, objective, epochs on — and measures:

```
validatorSetSize=4  RequiredQuorum=2  |  qualifiedCount=12  anchors=4
Mature=false  handedOff=false  everMature=false
→ a commit proposed by a1, attested by a2+a3 (a4 "down") COMMITS in-process.
```

**Reading → BRANCH (b), and more specifically: not even an arithmetic bug — a latency effect.**
- The launch branch fires: `validatorSetSize=4`, **not** the `qualifiedCount=12` fall-through. Sybil bonds do **not** inflate the launch quorum. (Rules out the PE's branch (a) and my original hypothesis.)
- `RequiredQuorum = bftThreshold(4) = 2` — note this **corrects the PE's branch-(b) arithmetic estimate** (the ruling supposed `bftThreshold(4)=3` needing all four; it is 2, `f=⌊3/3⌋=1, q=4−1−1=2`). So one fault is already tolerated by the sizing.
- A live 3-of-4 anchor commit (proposer + 2 attesters, one down) **passes in-process**. The arithmetic tolerates the fault.

**Therefore the cloud GAP is a gather-LATENCY effect under the 8-sybil load** — with val-d down, the proposer needs both remaining attesters (val-b, val-c) to return signatures within the commit window, and the sybil-load latency (same root as Item A) makes that unreliable. **No consensus rule change is indicated.** It folds into the §1 / M1 load-reduction work.

**Question B1:** Do you concur that the repro closes §2 as "quorum sizing correct; GAP is gather latency under load, no consensus change"? Or do you see a reason the in-process pass would not generalize (e.g. a partition-shaped interaction between the drain traffic and the gather that only manifests on the wire)?

**Question B2 (the residual the PE asked us to record regardless):** the PE directed us to write into `owned-residuals.md` a **mature-phase** residual: *a cohort honestly banking ≥⅓ of bonded weight can stall finality (not capture) — held-in-tension, priced by C1 (⅓ of real, decaying, sealed disk, recurring), bounded by C2's concentration alarm, safety preserved by D-1 (stall not reorg).* We have added it (draft in this PR). **Confirm the framing is correct** — specifically that this residual is **mature-phase only**, and that the launch phase provably has neither capture nor stall from un-matured bonds (the launch quorum is the fixed anchor set; un-matured sybil bonds neither vote nor count in the fault budget — consistent with the repro above). Is there any regime between "young" and "handed off" where an un-matured bond could acquire *stall* power? We believe not (validatorSetSize is the anchor set until the finalized handoff), but this is the seam the external red team will probe (brief seam #8), so we want your read.

---

## What each answer unblocks

- **A1 + A2 → yes:** we ship the parallel token-gather (M0-candidate scope) and ~6 field flows unblock; the next field run *instruments* the legs so remaining latency is named, not guessed.
- **B1 → concur:** §2 closes with no consensus change; the fault-tolerance corner is expected to recover once the §1/load work lands (re-verified on the next P1 run), and we do not touch a quorum rule.
- **B2 → confirm:** the mature-phase residual is recorded honestly and scoped correctly for the external red team; the launch phase stands as "neither capture nor stall from un-matured bonds."

*Net: Item A is a fast privacy stamp on a latency fix the PE already certified and spec'd — the one gate before we ship it. Item B is already reduced to a deterministic repro that says "no consensus change, it's latency"; we want your concurrence before we close it, plus a confirm on the mature-phase liveness residual's framing. Nothing here proposes a new mechanism; the two claim-adjacent lines (token-gather privacy, the fault-tolerance quorum reading) are exactly the ones we won't move without you.*
