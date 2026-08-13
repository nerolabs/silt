# PE consult — remaining issues on the M0 candidate (post-#357/#338/#382)

**From:** build (autonomous session, 2026-08-13)
**To:** principal engineer (audit & rescue seat) / research where a call is theirs
**Status:** consult — attribution + a proposed sequencing for your ruling. Each issue below has the evidence I have, my current attribution, and the specific decision I need. Per build-immutable #6 I have NOT built fixes for the consensus/claim-touching items; this asks which to build and in what order.

---

## 0. Where we are (so the consult has a frame)

The M0-candidate consensus/trust surface is in good shape and mostly cloud-validated:

- **#357 fork-choice oscillation: CLOSED.** Two-phase bond-weighted BFT (launch anchor-weight → mature bond-weight) + a quorum-finality gate (§3) + the mature-phase **epoch snapshot (Condition A)** and finalized handoff (Condition B). Research-certified; merged.
- **#338 C2 drivability: FIXED.** The sybils now sync + bank committed standing (reactive bond-reg drain + the static/persistent-peer tier as a chain-sync target + a uniform quorum floor). The C2 capture drill DRIVES for the first time.
- **#383: the C2 "capture" the field flagged was a HARNESS false-positive** (a lagging sybil catching up read as an advance). Chain-level proof the property holds in this topology: `core/chain/TestC2SingleDomainSybilsDoNotMature` — 8 equal single-domain bonds cap NakamotoBonds at 3 < MatureValidators 4 **regardless of domain/margin**, so the network can't mature, the anchor gate can't shed, and a no-anchor sybil quorum is refused `ErrAnchorRequired`. Detector hardened (true-tip ceiling + fresh-commit requirement).
- **#382 (first M1 efficiency): MERGED.** Chain-sync used to re-fetch + re-validate every peer's WHOLE chain every 30s sweep (O(chain×peers)); a cheap head probe now elides that when heads match. Trust-neutral (equivocation slash still fires, verified).

**Two SYBILS=8 cloud runs bracket the story.** Run A (pre-#382): the network **wedged** under the participating-8-sybil load — val-a publishes timed out (`context deadline exceeded`), sybils lagged, cascading FAILs. Run B (post-#382, this consult's run): the network is **LIVE** — durable convergence at tip 9, care-link publishes commit, val-a is responsive (forged-block / low-bond / priv-unlinkability all PASS where they failed in Run A). So #382 measurably restored liveness. **But Run B still has a cluster of GAP/FAIL, and it now has ONE dominant root cause (below), plus a consensus-liveness question the sybil topology surfaces.**

*(C2 verdict for Run B: TO FINALIZE — folding in when the sybil flow completes.)*

---

## 1. THE #1 REMAINING BLOCKER — the token-quorum (publisher-privacy) publish path over WAN (#344/#351)

**Symptom (Run B, evidence-cited):** the `-token-quorum 2` publish — a fresh non-validator (`fetch-1`) that must (a) discover the canonical issuer set + issuer keys, then (b) gather 2 blind-signature publish tokens from validators over WAN — fails, and it CASCADES:

- `2-publish-fetch` **FAIL**: "publish never produced a silt: link within 120s"; underneath: `publisher fetch-1 warm after 13s` then `ft_publish FAILED after 120s on fetch-1 (token-quorum=2)`. So it discovered the issuer set fast, then could not gather 2 signatures in 120s.
- Later flows show the *other* failure mode: `WARN: publisher fetch-1 did not warm within 180s — publish subsystem degraded` (issuer-set discovery itself didn't complete).
- Downstream GAPs, all "setup publish did not land a link": `7-restart-content`, `8-takedown`, `durability-turnover`, `chaos-crash`; and `9-cross-nat` FAIL (natted node couldn't publish to move a file).

**What is NOT the cause:** general network health (convergence durable, care-link + warm-up publishes commit, validators 4/4 reachable) and the chain-sync load (#382 fixed that). This is specifically the **publisher-privacy token path**.

**My attribution (hypothesis, needs your read):** the token-quorum publish makes an *ephemeral* CLI client do a multi-step, latency-serial dance over WAN — canonical-issuer discovery (`FetchCanonicalIssuersFromAny`), issuer-key fetch per signer, then N blind-sign round-trips (`MsgTokenRequest`→`MsgTokenReply`) — each a fresh dial from a keeps-nothing client, under the same WAN + 8-sybil load. The two failure modes (didn't warm / warmed-but-didn't-gather) suggest it is **latency + round-trip-count bound**, not a reachability bug (reachability is 4/4). It is inconsistent run-to-run, which reads as a race against the 120s/180s deadlines rather than a hard break.

**The decisions I need:**
1. **Is this a product defect or a harness-timeout artifact?** The harness note itself says "retry with `TOKEN_QUORUM=1`." Lowering the field-test default to 1 would unblock ~6 flows immediately — but that dodges the question of whether token-quorum≥2 acquisition is genuinely too slow/fragile over WAN for a real ephemeral publisher. Which is it, and should the field test certify token-quorum≥2 at all (vs. certifying it in-process and running the field with quorum-1)?
2. **If it's a real latency-cost issue, is it the next M1 target** (reduce the round-trip count / parallelize the gather / cache issuer keys across the client's lifetime), and does that touch the privacy claim (the signer set is canonical/deterministic for anonymity — can the gather be parallelized without narrowing the anonymity set)? That last clause is why I stopped short of just building it.

---

## 2. Byzantine-quorum sizing vs. a bonded hostile cohort (the 8-sybil : 4-validator ratio) — #380

**Symptom (both runs):** `6-fault-tolerance` GAPs — with one honest validator (val-d) down, the surviving validators do not commit. The harness note says "likely quorum/byzantine-quorum sizing."

**Attribution:** once the 8 sybils bank committed bonds, the qualified set N includes them, so `bftThreshold(N)` (the Byzantine super-majority) rises. The honest set is only 4 validators; with one down, 3 remain, which cannot meet a quorum sized against ~12 bonded participants. This is the same mechanism as **#380** (the objective-mode quorum-floor footgun) but arriving from the *other* side: not a mis-set local floor, but the **objective `bftThreshold` legitimately counting a hostile bonded cohort into the fault budget the honest set must overcome.**

**The real question (yours/research — it touches the C2/consensus model):** in the M0 threat model, a single-domain sybil cohort is *prevented from capturing* (it can't mature; anchor gate holds — proven). **But can that same cohort, merely by bonding, inflate the Byzantine quorum requirement enough to DENY LIVENESS to the honest validators?** That is a liveness-DoS vector distinct from capture. Options I can see:
- It's a **test-topology artifact** (a real launch would not have 2× hostile:honest bonded ratio while still "young"), and the fix is the harness (fewer sybils, or accept the GAP as unrealistic) — *or*
- It's a **real residual** that wants a mechanism: e.g. the young-phase quorum should be sized against the **anchor set** (which validatorSetSize already does pre-handoff) and NOT admit un-matured sybil bonds into the fault budget until maturity — which may already be the intent of Condition A's epoch snapshot but isn't holding here because the network never matures (so it stays in a regime where sybil bonds count for quorum but not for maturity). I need your ruling on whether that asymmetry (sybil bonds count for `bftThreshold` but not for `Mature()`) is correct or is itself the bug.

This is the sharpest *design* question in the consult.

---

## 3. The M1 cost/efficiency frontier — #382 done, what's next and in what order

Per the aligned M1 approach (trust stays green; budget the **local single-node compute turnaround**, isolated from network jitter). #382 was the first cut (chain-sync no-op-sweep cost) and it measurably helped. The remaining cost centers I can see, for you to prioritize:

1. **Token-gather latency (§1)** — if §1 is a cost issue, it's M1's next target.
2. **The ~1.5 MB space-time bond proof (#299)** — still the payload behind the size-aware deadline, the per-block byte budget, AND the O(N)-block drain serialization. A succinct proof collapses all three. Likely the highest-leverage structural M1 item.
3. **Bond-audit CPU on e2-small** — 8 sybils answering/issuing 64 MB bond challenges; steady-state should be size-independent after #341, but I have not *measured* it under the participating-sybil load (the M1 CPU-per-audit gauge isn't wired yet).
4. **Drain cadence** — the #338 drain is correct but O(N) blocks to onboard N validators (~30s/validator). Batching without losing the determinism/never-sign-twice guards.
5. **Genesis-to-head chain DIFF inside Reconcile** — #382 elides the no-op sweep; a real catch-up still fetches the whole chain. The follow-up.

**Decision I need:** the ordering, and specifically whether **#299 (succinct proof)** should jump the queue as the structural fix that dissolves several centers at once — and whether it's in M0-candidate scope or an M1 track item.

---

## 4. The mature phase (Condition A) is built but never field-EXERCISED

Condition A (epoch-snapshot the mature validator set) + Condition B (finalized handoff) are built and unit-tested, and research-certified as the design. **But no cloud run has ever reached maturity** — because the young-network topologies use equal bonds (8 equal sybils can't push NakamotoBonds past 3 < 4; 4 equal validators similarly). So the mature-phase consensus (bond-weighted fork-choice, epoch rotation, the handoff) has **only in-process coverage**, never field coverage. An external red team attacking consensus would go straight at the mature phase.

**Decision I need:** before the external red team (#183), do we (a) construct a field topology that *actually matures* (unequal bonds / enough distinct-domain validators to clear the maturity bar) and certify the mature phase + handoff on WAN, or (b) accept in-process + netem coverage for the mature phase and scope the field test to the launch phase? This determines whether there's a build item before the external gate.

---

## 5. Minor / tracked — the #378 equivocation-drill wedge (harness)

`TestEquivocatorSlashedOverTCP` is bimodally red under netem (and on unmodified main — verified) because the adversary's placement wedges after a partial placement; the **slash still fires** (property holds; #382 doesn't regress it). It makes the local adversarial gate flaky. Filed #378 with a fix direction (resumable placement). Low priority (harness, not product), but it's the reason the netem `SUITE=all` gate can't be relied on for a clean signal. Flagging in case you want it prioritized so the local adversarial gate is trustworthy before the external red team.

---

## 6. My proposed sequencing (for you to confirm or redirect)

1. **§1 token-quorum publish** — the biggest unblock (clears ~6 field flows). First need your call: product-fix vs. field-test-at-quorum-1. If product, it's likely M1-latency work with a privacy check.
2. **§2 Byzantine-quorum-vs-sybil-cohort** — the design ruling; may be "harness artifact, no code," may be a real young-phase quorum-sizing refinement. Blocks a clean fault-tolerance corner.
3. **§4 mature-phase field coverage** — construct a maturing topology OR scope the field test; needed before the external red team.
4. **§3 M1 ordering** — #299 likely first if it's in scope.
5. **§5 #378** — harness, opportunistic.

**What I will NOT do without your ruling:** touch the token gather (privacy claim), change any quorum-sizing rule (consensus), or decide the mature-phase field-coverage scope. Those are the three that touch a claim or a consensus rule (#6).

---

*Net: #357/#338/#382 landed and the network is live under load. The remaining cluster reduces to (1) the token-quorum publish path over WAN — one root cause behind most of the field GAPs — and (2) a genuine design question about whether a bonded single-domain cohort can deny honest liveness by inflating the Byzantine quorum, even though it provably cannot capture. Plus the mature-phase field-coverage gap before the external red team. The rest is M1 cost ordering.*
