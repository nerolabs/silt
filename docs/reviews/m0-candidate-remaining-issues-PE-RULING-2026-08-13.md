# PE ruling — remaining issues on the M0 candidate (answers the 2026-08-13 consult)

**From:** principal engineer (audit & rescue seat)
**To:** build; §2 and the §1 privacy paragraph routed to research
**Re:** `m0-candidate-remaining-issues-PE-CONSULT-2026-08-13.md`
**Shape:** one ruling per section — RULING / WHY / BUILD / GATED-ON. Sequencing at the end. The consult itself is good work: real attributions, and you stopped at exactly the three claim-touching lines you should have.

---

## §1 Token-quorum publish over WAN — RULING: product defect, M0-candidate scope. Do NOT certify at quorum-1.

**The call on your question 1:** this is a **product defect**, not a harness artifact. The ephemeral publisher acquiring a privacy token over a real WAN is not an edge case — it is the *flagship user story* (T2: "a link is enough"; the M0 privacy corner *is* this path). If a fresh client can't gather 2 blind signatures in 120s on a live network, real users hit that wall on day one. **Dropping the field default to `TOKEN_QUORUM=1` to unblock six flows is the exact grade-around-it pattern we banned** — quorum-1 is a different (weaker) mechanism, and certifying it would be certifying a claim the product doesn't make. The field test certifies the real default. *(A quorum-1 run as a __diagnostic__ — to isolate where the time goes — is fine and encouraged: that's instrumentation, not certification. Label it so.)*

**The call on your question 2 (the privacy question — why you rightly stopped):** the gather **can be parallelized, with one hard condition.** I checked the primitives:

- The token **reveals its signers** (`tok.Sigs`, `core/publishtoken/publishtoken.go:40` — per-signer sigs, verified against per-issuer keys). So *which* validators signed is observable to anyone who sees the token.
- The anonymity property therefore rests on **signer selection being publisher-independent** — which the code already does by design: the signer set is network-canonical, "ranked by committed bond… the SAME for every publisher, so the signer subset can't narrow the publisher's anonymity set (R-3)" (`cmd/silt/swarm.go:113`).

**The condition:** parallelize the *round-trips*, never the *selection*. Fire the k requests to the **same canonical k signers concurrently** and wait for that fixed set — do **not** switch to first-k-of-N-to-reply. First-k-to-reply makes the revealed signer set a function of the publisher's network position (nearest validators reply first) — a positional fingerprint stamped into every token, re-opening exactly the leak R-3 closed. Latency-independent selection + concurrent transport is privacy-neutral (blindness is per-message; the issuer sees a blinded serial either way) and arguably privacy-positive (a shorter observation window).

**BUILD (M0-minimal — latency work, no protocol redesign):**
1. **Parallelize** the k `MsgTokenRequest` round-trips to the canonical signer set (fixed membership, concurrent transport).
2. **Overlap + cache within the session:** issuer-key fetches run concurrently with each other and with dial warm-up; discovery results and issuer keys cached for the client's lifetime (one publish — trivially safe).
3. **Bounded retry on the idempotent steps** (discovery, key fetch) per the #334 pattern; the sign request itself gets one bounded, backoff retry (re-blinding a fresh serial on retry so no double-spend ambiguity — confirm with research if re-request semantics are already idempotent).
4. **Adaptive, size-scaled deadlines** on each leg per build-immutable #5 — no new flat constants (wanguard will hold you to this).
5. **Instrument the legs** (discovery / key-fetch / per-sign RTT) so the next field run *names* where remaining time goes instead of guessing.

**GATED-ON:** a one-paragraph privacy argument — "selection is latency-independent; only transport is concurrent; the revealed signer set is unchanged for every publisher" — sent to research for a quick stamp **before merge** (it touches the privacy claim's implementation, not its semantics; this should be a fast yes).

---

## §2 Byzantine quorum vs. the bonded hostile cohort — RULING: attribution incomplete. Repro must name the branch before any rule changes. Conditional rulings below so you're not blocked on a round-trip.

**Why I won't rule on your either/or yet:** your hypothesis ("bftThreshold legitimately counts the bonded cohort") **doesn't match the code as written.** `validatorSetSize()` (`core/chain/chain.go:742`) already returns `len(c.cfg.Anchors)` in the objective launch phase pre-handoff, and the frozen `epochSet` post-handoff; the live `qualifiedCount()` is only the *fall-through*. If the anchors branch were active on the failing nodes, quorum would be `bftThreshold(4)` regardless of 8 banked sybil bonds. Yet `6-fault-tolerance` **passes** in the no-sybil run (`c815091-2633`) and fails in both sybil runs — so *something sybil-correlated* is real, but the mechanism is not yet named. Per #6: **stop, instrument, repro** — a deterministic in-process test (4 anchors + 8 banked bonds, one anchor down) that logs, on the failing node, which `validatorSetSize` branch fired, and the actual N / `RequiredQuorum` / gathered-attestation count at the failed propose.

**Three candidate mechanisms — with the ruling for each, so the fix starts the moment the repro picks one:**

- **(a) The fall-through is being hit** (sybil bonds inflate live `qualifiedCount` into quorum pre-handoff — find *why* the anchors branch didn't apply: config not set on that node? `objective()` false? an ordering bug?). **Ruling if (a):** the fix is **phase-boundary voting-set discipline**, and it is stronger than "don't count sybils": *consensus membership may change only at a finalized phase boundary* (launch set fixed at genesis → expands at the finalized handoff → then epoch snapshots). An un-matured bond **neither votes nor counts in the fault budget** — it banks standing while onboarding, nothing more. This is not an invention; it is the settled BFT reconfiguration pattern (validator-set changes at epoch boundaries only — Tendermint/HotStuff lineage; B8 says adopt it). Note the symmetric safety half: you also cannot size quorum against 4 while letting 12 *attest* into weight — quorum intersection must be computed over the set that can actually vote, or two conflicting commits can each gather a "quorum" from disjoint mixes. One phase, one voting set, membership changes only at finality.
- **(b) It's the non-proposer-attestation arithmetic** (n live validators yield n−1 attestations; `bftThreshold(4)=3` then needs *all four* live — zero fault tolerance even with no sybils in the count; the sybil load merely slows val-a enough to expose it). **Ruling if (b):** the proposer's own signature must count toward its block's quorum (standard practice), so n−1 honest attesters + proposer meet the threshold with f faults. Small change, but it *is* a quorum rule — research certifies.
- **(c) Harness/config mismatch** (some node missing the anchors config in the sybil topology). **Ruling if (c):** harness fix, plus a daemon-side guard: an objective validator with no anchors and no handoff should *refuse or loudly warn*, not silently size quorum off the open bonded set (this is Invariant-B territory).

**On the design question you called sharpest — recorded regardless of branch:** in the **mature** phase, a cohort that honestly banks ≥⅓ of bonded weight *can* stall finality. That is not a silt defect; it is the BFT bound every weakly-subjective system lives with, and it is **priced** (⅓ of real, sealed, decaying disk — C1 makes the griefing cost real and *recurring*, since bonds decay without re-proof) and **bounded** (C2 alarms on the concentration; D-1 stalls rather than reorgs, so safety holds). **Write it into `owned-residuals.md`** as the liveness dual of the honest-whale entry: *bonded-minority liveness-denial — held-in-tension, priced by C1, surfaced by C2, safety preserved by D-1.* The red team should attack the price, and my updated brief (seam #8, stall-griefing) already sends them there. What the launch phase must guarantee — and what the (a)-ruling delivers — is that this residual **does not exist pre-maturity**: un-matured bonds must buy *neither* capture *nor* stall.

**GATED-ON:** the repro naming the branch; then research certifies whichever consensus change applies. Do not build ahead of the repro.

---

## §3 M1 ordering — RULING: #299 is NOT in M0-candidate scope. It is M1's first item.

**Why:** the succinct bond proof is the right structural target (it collapses the size-aware deadline, the byte budget, and the O(N) drain in one move) — and that is exactly why it doesn't belong in the M0 gate: it redesigns a **security proof object**, which means a research consult, a new adversary analysis, and a fresh regression surface, all while the current machinery *demonstrably works* (the network is live at tip 9 under 8-sybil load). Starting a crypto-payload redesign days before the external gate is how a nearly-closed milestone reopens. The M0 candidate ships the 1.5 MB proof with the deadlines and drain that carry it.

**M1 order when it opens:**
1. **#299 succinct bond proof** (research-gated; the structural win).
2. **Residual token-gather cost** — only what §1's M0-minimal fix leaves on the table.
3. **Wire the CPU-per-audit + dials-per-fetch gauges** — cheap, and do it *during* the P1 run regardless (the P1 spec already requires capturing these as the M1 baseline; you can't order M1 work you haven't measured).
4. **Drain batching** (keep the determinism/never-sign-twice guards).
5. **Reconcile genesis-to-head diff** (the #382 follow-up).

Item 3 is allowed to run early precisely because it's measurement, not change.

---

## §4 Mature-phase field coverage — RULING: option (a). Build the maturing topology. It gates the external red team.

**Why, three reasons in priority order:** (1) the red team's sharpest new target is the maturity handoff and post-shed regime (seam #8 in the brief: ramp-weight gaming across `everMature`, conflicting finalization *during* the set transition) — handing them a phase that has **never run outside process memory** invites cheap findings and devalues the engagement; (2) R1 requires the trust plane's core promises field-proven, and the mature regime *is* the product — the launch phase is scaffolding; certifying only the launch phase is certifying the training wheels and never riding the bicycle; (3) the handoff (Condition B) is the single most delicate consensus moment silt has, and it has zero wire coverage.

**BUILD (one flow, minimal):** a topology that actually matures — unequal bonds / ≥ the maturity bar of bond-distinct operators across distinct declared domains — then on WAN: warm → **`everMature` latches** (real `chain-status` field) → **anchors shed** → post-shed commits continue (bond-weighted regime live) → restart one validator and **cold-sync against the WS checkpoint** → confirm the latch holds after restart. That is the whole flow; resist decorating it. Add it to the P1 entry checklist alongside the sybil-banking and takedown items.

---

## §5 #378 equivocation-drill wedge — RULING: promoted. It is a red-team entry criterion.

The deterministic netem adversarial gate is the certification tier the whole P2 refactor rests on; a bimodally-red gate is a backstop you can't trust, and "the slash still fires but the drill wedges" is precisely the GAP-shaped noise we just banned from security drills. Fix the resumable placement before the external engagement, and add the entry criterion: **the local adversarial suite must be deterministic-green N consecutive runs (suggest N=10) before the red team starts.** Harness-only, small — but it is now *sequenced*, not opportunistic.

---

## §6 Sequencing (confirmed with modifications)

1. **§1 build now** (M0 scope, privacy paragraph to research in parallel — quick stamp). Biggest unblock: ~6 flows.
2. **§2 repro now, in parallel** — it's in-process and cheap. Route the named branch + my conditional ruling to research; build only after their certification.
3. **§4 maturing topology** — after §1/§2 land (it needs a working publish path and correct quorum to be gradable).
4. **§5 #378** — before the red team; can interleave.
5. **M0 gate:** the P1 all-corners run per `P1-WAN-PROOF.md` (entry checklist now: sybil banking ✅ per #338 — re-verify on the next run, takedown store-2, maturing-topology flow, adversarial-gate determinism).
6. **M1 opens** with #299. Not before the gate.

**The three research routings out of this ruling:** the §1 privacy paragraph (fast), the §2 branch ruling (real certification), and — when M1 opens — #299. Nothing else touches a claim.

---

*Net: your instinct on §1 was right and it's buildable today with one privacy condition (parallel transport, never parallel selection). §2's attribution isn't done — the code already guards the thing you hypothesized broke, so name the branch before anyone touches a quorum rule; the conditional rulings are pre-written so the repro's answer starts the fix same-day. #299 waits for M1. The mature phase gets field coverage before anyone outside attacks it. And the local adversarial gate becomes trustworthy before it has to vouch for you.*
