# Research consult — honest-proposer cross-attestation: the signing ledger, post-guard fork resolution, and slash recoverability (#397)

**From:** build (2026-08-14)
**To:** research team
**Re:** issue #397 — field run `b88245d-3496` wedged at a 2-2 fork with BOTH racing anchors slashed as equivocators
**Status:** consult, **RC-blocking**. The mechanism is fully attributed (captured journals + code + a deterministic failing-first repro, below); nothing is built. Per build-immutable #6 rule 5 this touches the equivocation-slash safety argument and the fork-choice liveness story, so no fix ships before your certification.
**Provenance:** found by the first evidence-instrumented P1 field run on the capture-hardened harness (#396). The run that found it is the P1 gate run for the external red team (#183) — this consult sits directly on the M0 critical path.

---

## The event, from captured evidence

Run `b88245d-3496` (3-region WAN, 4 anchors + 8 opt-in sybils, product binary identical to the locally-gated main@841fa1b):

- Blocks 1–5 committed identically on all four validators (~30 s cadence, 2 attestations each).
- **09:26:55 — two different empty block-6s commit simultaneously**: `98e587…` (val-a + val-d) and `d2e3307…` (val-b + val-c).
- **09:27:22 — every node slashes BOTH val-a and val-b**: `chain: slashed equivocator … (double-signed at height 6)` — 27 s ≈ the next sync sweep, i.e. the moment each side saw the other branch. The slash line then **re-fires every ~2 s indefinitely**.
- Aftermath: with 2 of 4 anchors slashed under anchor-quorum (wheels engaged), no proposal can commit. Every later publish fails (`ft_publish FAILED after 120s`, validators **4/4 reachable** — it is not egress), `5-convergence` records the fork verbatim, and the C2 flow's restore-the-anchors clincher cannot resume — the slash is committed state, restarting processes cannot undo it.
- The **prior day's run on the same product binary converged cleanly to tip 13** — this is a race, armed when two proposers hit one height inside the propagation window.

Journals: `integration/cloudtest/flow-evidence-b88245d-3496.log` (per-verdict captures of all four validators + sybil-1).

## The mechanism (attributed, not hypothesized)

1. **Two honest proposers raced one height.** val-a and val-b bonded at genesis together, so their `BondRenewalDue` clocks are aligned — permanently. At height 6 both had a renewal due. The drain path's designated-proposer rule (`core/node/chainrole.go:705-721`) picked one designated node, but a designated proposer with nothing pending stays quiet, so after 3 idle sweeps the takeover fallback re-opened proposing to **every** eligible proposer simultaneously; both took over in the same window.

2. **Each proposer attested the other's competing block.** The attest path refuses a second signature at a height it has attested (`chainrole.go:168-174`, the `n.attested` ledger), and the drain path refuses to *propose* at an attested height (`chainrole.go:696-703` — its comment cites exactly this wedge from a failing-first repro, crediting Tendermint locking). But **`proposeBlock` signs the proposal without writing `n.attested[b.Height]`** (`chainrole.go:407`). Each racing proposer therefore found an empty ledger at height 6 when the competitor's gather request arrived, and honestly attested it — signing two different blocks at one height.

3. **The slash rule fired on its own premise.** The cross-fork scan treats a double-sign as proven malice — sound only if an honest validator can never double-sign. The missing ledger write makes that premise false: the protocol itself manufactured the equivocation, then permanently slashed two honest anchors (F2: no re-earning), on both branches, wedging the chain.

**Deterministic repro (failing-first, RED on main):** `core/node/equivocation_proposer_test.go` → `TestProposerRefusesToAttestCompetingBlock`, branch `honest-proposer-cross-attest`. Target proposes at height 1 (gather sent to a dead attester — signed, uncommitted), then receives a rival block at height 1 → it attests (FAIL). The attest-after-attest twin (`TestValidatorRefusesToEquivocate`) stays green; attest-after-**propose** is the uncovered mirror.

---

## Q1 — The signing-ledger fix: is "record own proposal at sign time" the sound and sufficient closure?

Proposed minimal fix: in `proposeBlock`, record `n.attested[b.Height] = b.Hash()` at `chain.Sign` time — a proposer's signature enters the same never-sign-twice ledger as an attestation, **whether or not the proposal commits** (a signature at a height is final; that finality is what makes a double-sign proof evidence of malice). Symmetrically the existing propose-guard (`if _, signed := n.attested[height]`) then also covers "already proposed here."

Sub-questions:
- **Q1a.** Is signature-finality-per-height the right rule for the *proposer* side too, including the failure path — a proposal that gathers no quorum still permanently consumes that node's signature for the height until the head moves? (Tendermint answers yes via locking; our height clears when a commit advances the head — `attested` entries for cleared heights are... in fact never pruned today, see Q1b.)
- **Q1b.** The `attested` map is never pruned and is in-memory only. (i) Unbounded growth is minor (one hash per height), but (ii) **a restart wipes it** — a validator that signs at height h, crashes, restarts, and receives a competitor at h will attest it. Does the ledger need to be persisted (crash-safe never-sign-twice, as Tendermint persists its last-signed state in `priv_validator_state`), or is the restart window an acceptable owned residual at M0 (the field event did not involve a restart)?

## Q2 — Post-guard, the same race yields a clean 2-2 fork *without* equivocation. What resolves it?

With Q1 in place, replay the event: val-a and val-b each propose at height 6, each **refuses** to attest the other, and each races for the two remaining attesters. The observed outcome shape becomes: A commits with (a, d), B commits with (b, c) — two quorum-2 committed blocks at height 6 with disjoint signer pairs, **no equivocator, both branches honest**. The slash correctly stays silent — but the 2-2 equal-weight fork remains.

- **Q2a.** Does the certified objective fork-choice (model B, #357 §1-§3: anchor bootstrap weight, stable anchor-pinned quorum, super-quorum finality) already resolve an equal-weight same-height fork **deterministically** (e.g. a hash tie-break), and if so does every replica converge without a committed-block reorg (D-1 prefer-stall-to-reorg allows a stall here — but a *permanent* 2-2 stall is the wedge again, minus the slash)?
- **Q2b.** If fork-choice alone does not close it, the race itself must be closed. Candidate: **stagger the takeover fallback by proposer rank** (designated waits 0 extra sweeps, next-ranked +1, etc.), so the window only readmits one proposer per sweep; and/or **renewal-due validators SUBMIT their BondReg to the designated proposer (`MsgSubmitBondReg`, the existing attest-only path) instead of proposing themselves** — the aligned-renewal-clock collision driver disappears because `ownDue` stops being a reason to propose at all. Are either/both sound, and which is the M0-minimal set?
- **Q2c.** The genesis-aligned renewal clocks mean this collision recurs **every TTL period** at near-deterministic heights. Even with staggered takeover, should renewal scheduling be de-synchronized (jitter derived from NodeID), or does the submit-don't-propose change (Q2b) moot it?

## Q3 — Slash recoverability: keep permanent, or is honest-slash a recoverable class?

F2 makes a slashed equivocator permanently unable to re-earn standing. The field event slashed two *honest* anchors. Our instinct: **keep the slash permanent and harsh** — the fix is to make honest double-signs impossible (Q1), not to soften the penalty for real ones; a recoverable slash weakens the deterrent the C1/C2 argument leans on, and a socially-recoverable path for a provably-manufactured slash (a protocol bug, fixed) is a governance/relaunch event, not an in-protocol rule. Confirm, or is there a principled in-protocol recovery for "the double-sign proof is real but the signing rule that produced it was defective" that doesn't open the door an adversary can walk through?

## Q4 — The slash-detection hot loop (plain bug, sanity-check only)

The slash re-fires every ~2 s forever on every node (thousands of journal lines/hour). Two candidate causes we'll fix as ordinary bugs unless you flag consensus semantics: (i) the cross-fork scan re-detects and re-applies the same equivocation on every reconcile without an idempotency latch (`IsSlashed` is consulted when *recording on-chain* but apparently not before re-applying/logging the local ledger slash); (ii) relatedly, `proposeBlock`'s pending-slash requeue (`chainrole.go:397-406`) builds `still` and never appends to it, silently dropping pending on-chain slash records after one attempt — masked today by (i)'s re-detection. Confirm both are consensus-neutral cleanups: the ledger slash stays idempotent-once, the on-chain record requeues until committed.

## Q5 — Interaction check with the shipped B2 weight-quorum (#389) and the MATURING handoff

The wedge occurred in the launch phase (wheels engaged). Post-handoff, quorum is >⅔ frozen epoch **weight** (#389). Same race there: two mature proposers race a height, Q1 guard holds, disjoint coalitions try to commit — but >⅔ weight quorums **must intersect**, so a 2-2 double-commit is impossible in the mature phase; the loser stalls and resyncs. Confirm this reading — i.e. the fork-manufacturing race is a **launch-phase-only** exposure (quorum-2-of-4 count floor, non-intersecting), which bounds Q2's blast radius and argues the launch-phase fix can be liveness-shaped (close the race) rather than quorum-shaped (raise launch quorum to intersecting, which would harm young-network liveness — immutable #4 tension).

---

## What we will NOT do without your certification

Ship any of: the Q1 ledger write, a Q2 race-closure/fork-choice change, a Q3 recoverability change, or a launch-quorum change. The failing-first repro stays red on its branch until then. The two Q4 items we treat as consensus-neutral bug fixes unless you say otherwise.

## What this blocks

The P1 gate run → MATURING field-cert (#391/#389) → external red team (#183). A red team pointed at a chain that can slash its own honest anchors on a timing race would — correctly — walk straight through this seam.
