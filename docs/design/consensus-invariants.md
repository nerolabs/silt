# silt — Consensus Invariants (the map)

**Status: ADOPTED — canon reference (owner ratified 2026-08-14).** Authored from the principal-engineer seat; the research team **independently converged on the same rule** the same day (`silt-reviews/research/research-outcome/INTERSECTING-QUORUM-INVARIANT-note.md` — one invariant, the same four scars, plus a six-question checklist to apply at every quorum site). That convergence is the ratification basis. Consensus-rule changes remain research-gated (build-immutable #6); this document does not change a rule — it *names the closed set of rules the implementation must already satisfy*, so we stop discovering them one field run at a time.

**Binding working rules (canon, 2026-08-14):**
1. **Every consensus-touching PR states, in its body, which of I1–I5 it touches and how it preserves each.** No statement, no merge.
2. **At every quorum/gate/threshold site, the research checklist applies** (does it finalize? what set is N — and are attesters drawn *only* from that set? does the arithmetic intersect, stated in the code comment? are non-members excluded from the finality count? weight or head-count, matched to the threat? does the phase boundary preserve it?). The #402 trap to remember: **sizing a quorum over one set while filling it from a larger one.**
3. **The model-check (`consensus-model-check.md`) is the enforcement**, and its tier gates field runs (see that doc's Gating section).

**Why this exists.** Over 2026-08-13/14 the build hit #357, the B2 handoff issue, #397, and #402. They present as four different bugs. **They are one bug in four costumes:** a quorum that does not *intersect*, or does not *hold still*, admits a conflicting commit or a fork. That is the oldest safety property in Byzantine consensus, met at four different doorways. The pain has been that consensus correctness is *emergent* from a pile of individually-correct-looking rules, added reactively, with no written statement of the closed invariant set the whole surface must satisfy — so every multi-region field run is an expensive, slow, non-deterministic fuzzer for a spec that was never written down.

**The reframe (B8).** The novel part of silt is M0 — the token-less, work-backed, unlinkable Sybil composition. **BFT consensus is not novel** and must not be: Tendermint/CometBFT, HotStuff, and Casper/Gasper settled this exact cluster a decade ago. Every consensus bug so far is a place silt re-derived a settled corner instead of importing it. Spend the novelty budget on M0; make the consensus layer as boring and literature-faithful as possible. This document is that import.

**How to use it.**
1. Every consensus-touching PR states, in its body, **which invariants it touches and how it preserves each.**
2. The deterministic consensus model-check (`consensus-model-check.md`) asserts all five under adversarial scheduling — that is the tool that finds these on a laptop *before* a field run.
3. The field run's job returns to **confirming** on real WAN what the model already proved (build-immutable #6, applied to the consensus layer as a whole).

The set is closed and small. Everything hit so far is a corollary of I1 + I3 + I4.

---

## I1 — Quorum intersection at every gate, phase, and transition

**Statement.** For any two blocks that could each be treated as *final* at the same height, their supporting quorums must share at least one **honest** participant. A finalizing quorum must therefore exceed the size at which two such sets can be disjoint — **by weight** in the mature phase (> ⅔ of frozen epoch weight), **by an intersecting count** in the launch phase (of the pinned anchor set) — and this must hold **during the handoff transition**, not only within a phase.

**Class.** Safety. It is *the* BFT safety property: with intersection and ≤ f faults, at least one honest validator is in both quorums, so it cannot vote for two conflicting blocks → conflicting finalization is impossible.

**Scars (each is I1 at a different doorway):**
- **#357** — quorum sized against the live, shifting `qualifiedCount` as bond regs drained → no *consistent* quorum ever formed ("0 of 2 gathered").
- **B2 (handoff)** — quorum counted by **members, not weight**, so 8 minimum-bond sybils ride into the epoch set and reach a "quorum" that reflects no real resource.
- **#397** — launch finality granted on a **non-intersecting** 2-of-4 → two honest coalitions finalized conflicting blocks.
- **#402** — `AnchorQuorum=1` leaves a free anchor whose lone signature satisfies the gate → a competing block commits. **Deeper root (research cert):** the launch general quorum was *sized over the 4 anchors but fillable from all 12 bonded validators* — size-set ≠ membership-set, so disjoint support existed before the anchor gate even applied. **Certified fix (2026-08-14):** launch finality requires a **strict anchor majority `⌊A/2⌋+1`** (=3 for A=4) counting the proposer-if-anchor, **sybils excluded from the launch finality count**; the consult's `⌈A/2⌉` was off by one for even A (a both-sybil-proposed 2-2 anchor split passes `AnchorQuorum=2` and the finality gate then *cements* a permanent conflicting-finalization partition). Chosen encoding: **(B) anchor-only launch proposing** (sybils drain via `MsgSubmitBondReg` submit-don't-propose, per #397) — removes the sybil-proposed fork at the source. Fault tolerance unchanged: 3-of-4 anchors up, 1-fault-tolerant.

**Governs (code):** `core/chain/chain.go` — `RequiredQuorum` (:721), `validatorSetSize` (:742), `bftThreshold` (:703); the finality gate (~:2001–2024, `ErrPreFinalityReorg` :412); `ValidateCommit` anchor gate (~:1472); epoch weight sum (~:1827).

**Assert (test):** a property test — under *any* partition/proposer schedule, **no two distinct blocks at one height both satisfy the finality predicate.** This single property subsumes #357/B2/#397/#402. The threshold must be verified for true *set-intersection*, not "majority up": for A anchors, two finalizing anchor sets must be unable to be disjoint (mind the `⌈A/2⌉` vs `⌊A/2⌋+1` off-by-one — it is exactly where the seam reopens).

**Literature (B8):** Tendermint/CometBFT (> ⅔ voting power), Casper FFG (⅔ accountable finality), HotStuff (n−f quorums). All **weight/stake**-based; none count heads.

---

## I2 — Never sign twice at a height, persisted across restart

**Statement.** A validator's signature at a height — **as proposer or attester** — is final and unique. It never signs a different block at a height it has already signed, and **that memory survives process restart.**

**Class.** Safety + accountability. It is what makes a double-sign *proof of malice* rather than an accident; if an honest validator can be induced to double-sign, the slash rule fires on a false premise and slashes the honest (see I5).

**Scars:**
- **#397** — `proposeBlock` signed a block without writing the never-sign-twice ledger (`chainrole.go:407`), so a racing proposer honestly attested a competitor → mutual slash → wedge.
- **#397 (restart variant)** — the ledger is in-memory only; a sign→crash→restart→attest-competitor sequence equivocates an honest node and permanently slashes it (F2) — a churn-punishment (persona #4, S6).

**Governs (code):** `core/node/chainrole.go` — proposer `chain.Sign` site (:407, the missing write), attest ledger (:168–175), propose guard (:696–703).

**Assert (test):** unit + **restart injection** — sign at h, kill the process, restart, present a competitor at h → must refuse. The in-memory version passes *without* restart; only the persisted version satisfies the invariant. Prefer a **persisted monotonic last-signed watermark** `{height, hash}` (fsync before the signature goes to the wire) — O(1), crash-safe, and it also fixes the never-pruned map.

**Literature (B8):** Tendermint `priv_validator_state` (persisted last-signed height/round/step, fsync'd before broadcast).

---

## I3 — Validator-set changes only at a finalized boundary; admission is by weight, not head-count

**Statement.** The set a quorum is computed over changes **only at a finalized checkpoint** (epoch boundary / handoff), never mid-flight from live churn. Admission to that set must not let a cheap identity acquire vote or veto weight it did not pay for: quorum weight is **real bonded weight**, not membership count.

**Class.** Safety (quorum stability is a precondition for I1) + Sybil-economics (the M0 tie-in).

**Scars:**
- **#357** — sizing against the live `qualifiedCount` shifted `RequiredQuorum` block-to-block as registrations drained → no fixed set → no intersection.
- **B2 (handoff)** — un-matured, minimum-bond sybils entered the first epoch snapshot as **full members**, so `bftThreshold(memberCount)` handed a MinBond-per-head cohort stall power that weight-counting denies. (Fixed by #389 — weight-quorum. This invariant records *why*.)

**Governs (code):** `core/chain/chain.go` — `validatorSetSize` (:742: anchors pre-handoff, frozen `epochSet` post-handoff — the correct pattern), `epochSet` (:518, :1765), the handoff (Condition B); weight sum (:1827).

**Assert (test):** membership is **constant within an epoch**; a bond that banks mid-epoch gains **zero** quorum weight until the next finalized boundary; quorum is computed over **weight**, not count.

**Literature (B8):** Tendermint/Casper apply validator-set changes at a committed epoch/height boundary, weighted by stake.

---

## I4 — Commit ≠ Final (finality needs an intersecting quorum; commit may be lower for liveness)

**Statement.** "**Committed**" (optimistic progress, at a lower quorum so a young network stays live) is distinct from "**final**" (irreversible, at an intersecting quorum). **Only final blocks are reorg-refused** (D-1). A non-final commit is reorgable by deterministic fork-choice. A user-visible durability signal (a publish link) is returned **only on final**, never on a reorgable commit (B7/S3).

**Class.** Safety + liveness reconciliation. This is what lets a young network commit at a low quorum (immutable #4) *without* the non-intersecting quorum being able to finalize a fork.

**Scars:**
- **#397** — launch treated `committed == finalized` at a non-intersecting 2-of-4 (`chain.go:2001` "Launch-phase: finalized == committed head"), so a clean 2-2 fork became **two finalized blocks that can never reorg → permanent wedge.** Decoupling commit (2, live) from final (intersecting, safe) lets fork-choice resolve the fork instead of wedging.
- **#432 (the LIVENESS half — 2026-08-15; mechanism SHIPPED 2026-08-16).** The height-only
  #397 watermark permanently wedged a height whose gather fails: a crossed publish-vs-drain
  proposer race splitting the anchor signatures 2-2 left every anchor able to sign only its
  own block at that height, no block could reach the strict anchor majority, fresh proposals
  died at their proposers' own watermarks, and the mark cleared only on a commit the marks
  forbade — a **permanent stall of a connected, all-honest, 0-fault network**, violating this
  invariant's assert-note verbatim (both MATURING field starves at tip ~6; deterministic
  repro `core/node/modelcheck_i4_liveness_test.go`, born RED). Fork-choice cannot resolve
  what never committed — the safety rulings above covered I4's *safety* face and missed this
  one. **Shipped mechanism (research-certified, plain T1): the era-2 two-phase gather** —
  `(height, round, phase)`-scoped signatures, a two-certificate commit (prepare-QC +
  precommit, each at the full commit threshold in both regimes — the POL threshold IS the
  commit threshold), lock-on-prepare-QC (durable, restart-rehydrated), deterministic
  sweep-count round advance (never wall-clock — B2/#3), a view-change that carries the
  highest lock forward, round-scoped equivocation (I5), and the author's required
  round-scoped self-prepare (the structural ProposerSig's era-2 analogue — keeps a
  double-proposal attributable, count-neutral in every quorum). Merge gate held: the S1/S2
  oracles (`core/node/modelcheck_s1s2_test.go`, mature faces in
  `core/chain/modelcheck_s1s2_mature_test.go`) are RED against a recorded lock-free revert,
  GREEN with the prepare phase. Certification:
  `silt-reviews/research/research-outcome/432-rounds-locking-liveness-RESEARCH-CERTIFICATION-2026-08-15.md`.

**Ruling — I4 is a *permission*, not a build mandate (owner, 2026-08-14).** The invariant is satisfied whenever nothing non-intersecting can finalize; it does **not** require silt to build a commit/final decoupling. Research has twice ruled the minimal path instead: the #397 certification found launch finality already intersecting once the ledger write landed, and the #402 certification's M0 set is the strict-anchor-majority rule alone ("no fork-choice change, no D-1 change") — with an intersecting launch finality quorum, `commit == final` satisfies I4 trivially. The supporting research rule to remember: **the finality gate enforces, it does not create** — `ErrPreFinalityReorg` stops a node reverting its *own* head, never two groups finalizing conflicting blocks; leaning on the gate to fix a non-intersecting quorum *cements* the fork. A decoupling is built only if the model-check produces a schedule that violates I4 as stated — that evidence, not this scar note, reopens the question (build-immutable #7).

**Governs (code):** `core/chain/chain.go` — finality gate (~:2001), `ErrPreFinalityReorg` (:412), `heavier`/`Reconcile` (:1651 / ~:2001).

- **#441 (the OPERATION-liveness face — 2026-08-16; mechanism SHIPPED same day).** Chain
  liveness is not the property the product promise needs: after the #432 escape restored
  "the chain always commits *something*", the first mature-regime field run
  (a56ac10-42834) committed **zero publish entries post-latch across 33+ heights** while
  drain blocks committed every height — and the launch soak (9453325-7258) stalled a
  publish-contended height 361 s past the computed escape bound. One root: the round
  machinery's new-view seat AND its escape arming belonged exclusively to the drain
  path, so an entry proposal could win no round of any height. **I4's full statement is
  therefore *operation-liveness*: no legitimately submitted operation is permanently
  starved** — a property asserted at the *product* layer, not only the chain layer (the
  same recurring lesson as the intersecting-quorum note: chain-liveness passed while
  entry-liveness went unasserted). **Shipped mechanism (research-certified, direction
  A): entries are MEMPOOL CONTENT** — submitted via `MsgSubmitEntry` (the
  `MsgSubmitBondReg` mirror, validate-on-arrival, FIFO, dedup-by-root), folded into the
  single `(h, r)` designee's block under a byte budget SEPARATE from the reg budget
  (neither stream can starve the other), arming the escape alongside regs; the client
  publishes by submit-then-poll-for-finality (B7/S3 unchanged). Entries are content,
  never a competing value: locks/POL, `requireProposerPrepare`, and #402
  count-neutrality untouched. Certification:
  `silt-reviews/research/research-outcome/441-publish-starvation-RESEARCH-CERTIFICATION-2026-08-16.md`.

**Assert (test):** a 2-2 non-intersecting fork is **resolved by fork-choice** (loser reorgs — allowed, it was never final), never wedges; a connected network never suffers a *permanent* non-final stall (**the chain-liveness half — asserted by `TestModelCheck_I4_WedgedHeightMustRecover`, GREEN with the #432 rounds+locking mechanism**); **no legitimately submitted operation is permanently starved** (**the operation-liveness half — asserted by `TestModelCheck_441_PublishStarvedAcrossRounds` + the §6 siblings in `core/node/modelcheck_441_siblings_test.go`, all RED under the recorded fold+arming revert, GREEN with the entry mempool**); a publish link is issued only after finality.

**Literature (B8):** Gasper — LMD-GHOST advances the head optimistically, Casper FFG finalizes at ⅔ behind it. The commit/final separation is the norm, not a silt invention. The entry mempool is the leader-carries-the-mempool shape of every BFT SMR — a leader's one block carries the transaction pool; there is no separate per-transaction proposal that can lose a race.

---

## I5 — Deterministic fork-choice among finalized descendants; every safety violation is attributable (an honest node is never slashed)

**Statement.** Fork-choice is a **deterministic total order** (weight → height → hash), evaluated **only over descendants of the latest finalized block.** And the system has **accountable safety**: if two conflicting blocks ever finalize, it is always attributable to a slashable ≥ ⅓ — an **honest** validator is *never* slashed.

**Class.** Safety (determinism → all honest replicas pick the same head) + the accountability guarantee C1/C2's deterrent leans on.

**Scars:**
- **#357** — among zero-weight ramp forks, `heavier` fell to a **height-blind hash tiebreak** → committed blocks dropped by hash luck (non-deterministic-looking, and reorg-of-final).
- **#397** — honest anchors were **slashed** because the double-sign-proof premise was falsified by I2's gap. Accountable safety was violated: the penalty hit the honest. Any harness/model that asserts "no honest schedule produces a slash" catches #397 immediately.

**Governs (code):** `core/chain/chain.go` — `heavier` (:1651), `Reconcile` (~:2001); slash path `core/chain/equivocation.go`, `core/credit/credit.go` `SlashEquivocation`.

**Assert (test):** fork-choice is a **pure function** (replay determinism — same inputs → same head on every replica); and **no honest schedule ever produces a slash** (the accountable-safety oracle — this is the direct catch for #397).

**Literature (B8):** Casper FFG accountable safety / slashing conditions; LMD-GHOST determinism.

---

## The closure claim

Every consensus defect silt has hit is a corollary of the above:

| Scar | Primary invariant(s) violated |
|---|---|
| #357 fork-choice oscillation | I3 (shifting set) → I1 (no intersection) + I5 (height-blind tiebreak) |
| B2 handoff cheap-member quorum | I3 (count not weight) → I1 |
| #397 honest-proposer cross-attest | I2 (unpersisted, unwritten ledger) + I4 (commit==final) + I1 (non-intersecting launch finality) + I5 (honest slashed) |
| #402 one-free-anchor fork | I1 (non-intersecting anchor gate) |
| #432 wedged-height permanent stall | I4 (liveness half: height-only watermark, no rounds) — the safety rulings covered I4's safety face only |

If a fifth consensus surprise appears that is **not** a corollary of I1–I5, that is real signal the set is incomplete — add it here, with its scar and code site. Absent that, **the set is closed**, and the way to stop the tail-chasing is to assert all five under adversarial scheduling (`consensus-model-check.md`) *before* spending a field run — not to keep discovering them one region at a time.

*The perimeter of BFT correctness is finite and published. This is silt's copy of it, annotated with the doorways we already walked through the hard way.*
