# Silt Roadmap

> **Source of truth.** *The finished system we are building toward* is
> [`docs/VISION.md`](docs/VISION.md) (the north star). *What M0 asserts and why*
> lives in [`docs/design/m0.md`](docs/design/m0.md) (the composition spec). *The
> principles that keep it honest* are [`docs/TENETS.md`](docs/TENETS.md). *What the
> owner has decided* lives in [`docs/decisions.md`](docs/decisions.md). This file is
> the **single source of truth for tasks** and the **narrative path**: where we are,
> what the Boulders are, and why they're ordered the way they are. The GitHub issue
> tracker is being retired as a task driver (see the SSOT note below).
>
> The earlier **Gate 0→6 spine** (with Gate 4 as "the M0 mechanism to build") is
> **retired**: that mechanism is built and the mission was reframed (below). The
> `v0.1.x`/`0.2.x` tags are **experimental / learning releases**, not steps on the
> march to V1; that history lives in `docs/buildlog/`.

> **★ Single source of truth for tasks (2026-09-01).** This file — specifically the
> **Boulder/Rock spine** below ("The Boulders (current big-step tracker)") — is the SINGLE
> SOURCE OF TRUTH for live work. The path to the Release Candidate (V1) is the Boulders,
> executed in order under their stated dependencies. The GitHub issue tracker is being
> **retired** as a task driver; all live work lives here. A handful of umbrella/frontier
> issues (#183, #182, #180, #179, #94, #52) remain as evidence anchors the Boulders point
> at — they are not a second task list. Residual/off-critical-path items are enumerated in
> the **Residual backlog** section at the end of this file; repro recipes for named field
> defects live in the cited `docs/design/` and `docs/thinking/` docs.

## Tenets are the destination; the Boulders are the track

**V1 is defined by the tenets, satisfied and field-proven — not by a feature
list.** Every Boulder below advances one or more tenets; if a step serves no tenet,
it doesn't belong here. The relationship, stated once: **the tenets are the
destination; the Boulder/Rock spine is the current track that operationalizes them.**
(The earlier framings — "tenets are the roadmap" and, later, "the ordered path is the
track" — are retired; both are preserved in
[`/archive/roadmap-history-2026-09-01.md`](archive/roadmap-history-2026-09-01.md). The
D-M1-PIVOT *decision* they carried stands — see `docs/decisions.md`.) A tenet gates
V1 as a *principle*, never a *mechanism* — with one deliberate exception, **M0** (the
mission itself), whose *real* mechanism is in V1 by definition. **Release is gated by
proof (R1):** a tenet is "met" only when field-proven multi-machine, not sim- or
single-host-only.

## The launch stance — harden-first

The first public appearance must be **credible and spectacular from day one**. A
half-baked drop on a project this ambitious burns the one first impression we get
with the exact technical audience we need. So the tenet **floors** (integrity,
no-silent-loss, don't-crash, honest observability) *and* the **mission** (M0,
field-proven) are done before any launch. Feedback is sought — on something that
already stands up, not as a substitute for hardening.

**The build principle (B8):** best-in-class *components*, a novel *composition*.
We do not reinvent primitives (crypto, transport, codec); we adopt the strongest
proven ones and reserve novelty for the composition and incentives — where M0
lives — proven by spec + an **external** red-team, never self-graded.

> *The retired "The ordered path" preamble + numbered Phases 1–6 lived here (2026-08-19,
> D-M1-PIVOT framing). Extracted 2026-09-01 to
> [`/archive/roadmap-history-2026-09-01.md`](archive/roadmap-history-2026-09-01.md) — the
> D-M1-PIVOT decision it carried (M1 interleaves with the M0 tail; storage-economy-first;
> firewall unchanged; harness never softens) stands and lives in `docs/decisions.md`; the
> phase packaging is superseded by the Boulders below.*

### The Boulders (current big-step tracker — Rock/Boulder synthesis, 2026-09-01)

**This is the current track.** It replaces the earlier six-Rock overlay (preserved below as
*Superseded Rocks*), reorganizing the same board into **five sequenced Boulders** after a
7-seat audit ([`docs/thinking/2026-09-01-*-design.md`](docs/thinking/), the four PACE
deliberations). **Boulders** are the major arcs; **Rocks** are the ordered deliverables inside
each. This spine is the SSOT for live work; the Residual backlog (end of file) holds the
off-critical-path tail. Items are
marked owner-DECISION, cert-gated, or RED-first as they apply. The overnight owner-ratified
decisions are folded in (R0.2, R1.7, R2.3, R2.5, R2.6, R3.4), and the **2026-09-03 ratifications**
below (the flip/freeze/B8 order; R-BOX-ATTESTS O1/O2/O4).

**Two whole-board sequencing constraints:**
1. **Nothing turns the economy on over a live mint.** Boulder 0 (A4) precedes Boulder 2.
   Boulder 0 is now DONE + MERGED (PR #686); the R0.4 conservation re-cert cleared before any
   economy default flip.
2. **The accept-flip does not close until its whole witness-soundness spine is green, the era-4
   format is FROZEN, and the external pass clears on the frozen artifact.** The flip (R1.8) is a
   consensus-rule change (I1); the old "single remaining step" framing hid 7 predecessors — see the
   flag under *Superseded Rocks*. **RATIFIED 2026-09-03 — the order is now:** internal flip
   preconditions (R1.0–R1.6 + the named residuals) → **era-4/v5 format freeze at the RELEASE
   CANDIDATE, which is the stamp-raising release (R3.4)** → **external B8 pass (R1.7 / R4.4) against
   the FROZEN, still never-Accept artifact** → **R1.8, the flip**. This **reverses** R3.4's earlier
   "the flip proceeds pre-freeze; the freeze re-confirms byte-identity later" clause: a B8 pass
   bought against an unfrozen format would have to be re-bought at the freeze.

**★ Decisions owed to the owner (as of 2026-09-03).** Read these first; each names its source.
- **Floor-box STRUCTURE (Boulder 1, pre-freeze).** Build option (E)/(D): ONE accept composition
  `ValidateCommitV5(view, block)` over a three-valued `StateView`
  (`Present`/`ProvenAbsent`/`NoWitness`, where `NoWitness` STALLS), called by both the node and the
  box, with all 11 box doors **unexported** behind a single `WitnessValidateV5` door; the contract
  form becomes one-sided `box.Accept ⇒ node.Accept` (exact box-vs-node equality is REFUTED as a
  certified form). Both seats also recommend **HOLDING** the box-entry round-A exported surface
  (merge the arithmetic, keep the doors unexported — zero production callers today) and name two
  **PRE-FREEZE** items: `tagLastProposer` and the carrier byte ceiling. Sources:
  `/Users/andrewedmond/Claude/claude/silt-reviews/principle-engineer/RULING-floorbox-predicate-rederivation-structure-2026-09-03.md`,
  `/Users/andrewedmond/Claude/claude/silt-reviews/research/research-outcome/floorbox-predicate-rederivation-STRUCTURE-RESEARCH-VIEW-2026-09-03.md`.
- **O3 — the fork-choice weight term.** Both seats independently recommend **Direction T (retire
  the term; state `heavier` as height → head-hash)** over Direction R (repair). **Consensus
  recommendation T; owner decision PENDING.** Sources:
  `/Users/andrewedmond/Claude/claude/silt-reviews/principle-engineer/RULING-O3-fork-choice-weight-R-vs-T-2026-09-03.md`,
  `/Users/andrewedmond/Claude/claude/silt-reviews/research/research-outcome/O3-fork-choice-weight-R-vs-T-RESEARCH-RECOMMENDATION-2026-09-03.md`.
- **R0.4b C3 (expiry) — four owner calls.** (1) **Break-1 ratification:** the payload-driven
  `issuerKeyCommit` prune is a **consensus rule** and is research-CERTIFIED
  (`/Users/andrewedmond/Claude/claude/silt-reviews/research/research-outcome/R0.4b-C3-composed-close-bc062d0-RESEARCH-CERTIFICATION-2026-09-03.md`
  §1). (2) **The `IssuerKeys` per-block count cap** — a v5 **validity rule** of the class `RegCap`
  already answers for `BondRegs`; measured ~33 s of ed25519 per validator per block at the 128 MiB
  frame ceiling; free now while v5 is dark, a fork after era-4 opens
  (`/Users/andrewedmond/Claude/claude/silt-reviews/principle-engineer/RULING-R0.4b-C3-close-271ab81-final-2026-09-03.md`
  §3, §8). (3) **The production `grant` + faucet rate limit** — keep `grant = 500_000` and
  rate-limit the faucet (structure fixed, rate is an Evolving-tier knob; do NOT key the limiter on
  the ledger watermark — blocked on F8) and (4) **`RequireBondedFetchers` default OFF**
  (`/Users/andrewedmond/Claude/claude/silt-reviews/economist/ADVISORY-R0.4b-cap-griefing-grant-and-bonded-fetchers-2026-09-03.md`,
  certified in the composed cert §6). **G-8 no longer needs an owner call:** both seats converged on
  **(iii) re-scope** the e2e gate to the certified era-4 refusal — never an activation override, in
  any form, in any binary
  (`/Users/andrewedmond/Claude/claude/silt-reviews/research/research-outcome/R0.4b-C3-G8-dark-lane-CONVERGENCE-2026-09-03.md`).
- **`R-CARRIER-BYTES` — the validity bound, with the cost now MEASURED.** 1.3 M carrier entries fit
  a single 132 MiB frame and cost **42.2 s single-core** to validate, **130.18 MiB** on the wire,
  **1.13 GiB** max RSS — and are re-paid on every disk reload, permanently
  (`/Users/andrewedmond/Claude/claude/silt/.claude/agent-memory/tester/pr-lastcommit-carrier-verification-2026-09-03.md`).
  Red-team round 2 adds the box-side face: a **VALID junk carrier** costs the box **2.67 GiB** of
  `AttScreen` witness for one canonical NO-OP block
  (`/Users/andrewedmond/Claude/claude/silt-reviews/red-team/RED-TEAM-lastcommit-carrier-26977a4-RE-BREAK-2026-09-03.md`
  RT2-CARRIER-17/17b). The two legs differ in reachability and that matters: the node's 42.2 s leg
  requires a **qualified proposer**; the box's does not
  (`/Users/andrewedmond/Claude/claude/silt-reviews/research/research-outcome/LASTCOMMIT-CARRIER-26977a4-DELTA-CERTIFICATION-2026-09-03.md`
  §2.4). A size rule on hash-covered content is a v5 validity rule: research-gated, owner-ratified,
  and it must land **before** the era-4 freeze.

#### Boulder 0 — Stop the live bleed (A4 money-pump) · ✅ DONE + MERGED (PR #686, 2026-09-01)
The only break exploitable on `main` (behind `-accept-delivery-receipts`; `-economy`
default false). Bond-gated so it bought no consensus standing, but it broke the certified B3
conservation close and could fund spam/publish at scale. **CLOSED:** the live money-pump is
dead. **R0.4 CERTIFIED** — conservation (`Σ balances + Σ escrow == grant + legitimate transfers`)
holds across all three terminal lane states (never-redeemed, in-window-redeemed,
evicted-then-redeemed); both firewalls (γ→1/N and standing) untouched. Shipped **(b)-minimal**
(the eviction claw-back), NOT full (b)-prunable — receipt-expiry is parked as R0.4b (below).
Alongside the ledger fix, `provOrder`/`provIndex` integrity landed: a blind red-team pass and a
compaction fuzz each found and closed a FIFO-order desync.

- **R0.1 · A4 conservation regression gate (RED-first)** · ✅ DONE. Asserts
  `Σ balances + Σ escrow == initial grant` across flood→evict→redeem
  (`money_pump_test.go`, `TestA4MoneyPumpConservation`); RED on main → GREEN on the fix.
- **R0.2 · redeem-without-provisional-record semantics — DECISION: (b)-minimal SHIPPED;
  (b)-prunable is the ratified longer-term direction, parked as R0.4b.** (b)-minimal
  reverses the eager self-mint at eviction (one delivery, one payment) and is a
  conservation-correct close on its own. The full (b)-prunable form — couple provisional
  lifetime to an ENFORCED receipt-expiry — is a separate, evidence-gated certification unit
  (R0.4b), not a soundness dependency of the shipped fix.
- **R0.3 · A4 fix (claw back the eager self-mint on eviction)** · ✅ DONE. Shared
  `reverseProvisional` at both eviction and redeem (identical escrow floor at each);
  `provisionalServe` now stores the server identity so eviction reverses the exact credited
  account.
- **R0.4 · Economic re-cert of the B3 conservation close** · ✅ CERTIFIED (2026-09-01)
  (`A4-provisional-eviction-conservation-RESEARCH-CERTIFICATION-2026-09-01.md`). The prior B3
  cert did not model bounded-map eviction; this cert closes exactly that residual and confirms
  no new money pump and no firewall re-open.
- **R0.5 · A4 node-path integration conservation gate** · ✅ DONE. Proves the fix is wired at
  the node path (`node.go` + `demandrole.go`), not just in the ledger.

**Boulder 0 residuals (status 2026-09-03):**
- **RT-DELIV-3 · delivery-credit `provKey` omits the server** · ✅ **DONE + MERGED (PR #699), with
  its open-break gate (PR #700).** DECIDED: fix now. The server identity is in `provKey`, which
  additionally closes a conservation break in the shared-ledger MULTI-SERVER case the prior A4/R0.4
  cert did not model; both firewalls (γ→1/N and standing) stay intact. Research cert (the change
  re-opened and re-closed R0.4):
  `RT-DELIV-3-provkey-server-identity-RESEARCH-CERTIFICATION-2026-09-02.md`.
- **R0.4b · receipt-expiry (the full (b)-prunable form) — NO LONGER PARKED: BUILT, NOT MERGED,
  GATED.** The two owner calls that gated the build were answered (per-epoch / serial-indexed keys,
  never a wall-clock `NotAfter`) and the C3 close is built on `builder/r0.4b-c3-close-fix`
  (`271ab81`). It ships the (b1) FDH epoch binding, the payload-driven `issuerKeyCommit` prune, the
  token-keyed guards, and a persisted paid-serial store. **Merge is blocked**, not by design: the
  required `e2e` CI job is RED and the `IssuerKeys` count cap (H-1) is owed in the same commit.
  Verdicts: research `R0.4b-C3-composed-close-bc062d0-RESEARCH-CERTIFICATION-2026-09-03.md` (GATED;
  G-3/G-4/G-D since CLOSED per
  `R0.4b-C3-271ab81-G3-G4-GD-DELTA-CERTIFICATION-2026-09-03.md`), PE
  `RULING-R0.4b-C3-close-271ab81-final-2026-09-03.md` (NOT merge-ready: red e2e + H-1), G-8
  `R0.4b-C3-G8-dark-lane-CONVERGENCE-2026-09-03.md` (take option (iii)). The four owner calls are in
  the *Decisions owed* block above; the named residuals are Rocks under Boulder 1 and Boulder 2.

#### Boulder 1 — Make the accept-flip safe (floor-box witness-soundness spine) · was Rock 1
Root cause (verified, all seats): classes P/A/B take witness values/screens as `NewValue`s or
branch predicates without `Resolve`-ing against `prevStateRoot`; the attacker controls the
committed root too, so `postRoot==StateRoot` holds by construction. Sound classes route witness
values as VerifyProof'd `OldValue`s — the fix mirrors them. Design:
[`docs/thinking/2026-09-01-floorbox-witness-soundness-fix-design.md`](docs/thinking/2026-09-01-floorbox-witness-soundness-fix-design.md).

**Status (2026-09-03): R1.0 / R1.1 / R1.2 / R1.5 / R1.6 DONE; R1.3 REFUTED; R1.4 CERTIFIED then
RE-OPENED and re-closed by the class-P scalar-anchoring fix (PR #704). The accept-flip (R1.8) is NOT
done** — its precondition list has GROWN this session (the fold-live-state class, the box-entry
round-A findings N1–N8, and the `LastCommit` carrier's parent-anchoring class), and the flip now
lands **after** the era-4 freeze and the external B8 pass (see the sequencing constraint above). The recompute-soundness increment merged as a standalone **never-Accept**
change: the box gained the Resolve-anchoring but still emits `indeterminate-trustlessly`, never
Accept. Certification:
[`floorbox-R1.3-refutation-R1.4-witness-soundness-RESEARCH-CERTIFICATION-2026-09-01.md`](../silt-reviews/research/research-outcome/floorbox-R1.3-refutation-R1.4-witness-soundness-RESEARCH-CERTIFICATION-2026-09-01.md).

- **R1.0 · Pin the held invariants BEFORE the refactor** · ✅ DONE. The shared quorum
  arithmetic (`requireQuorumStack`) does not fork between the live path and the box (the #402
  lesson); class-M is A2-poisoned (inherits the forged `validatorsSeenRoot`).
- **R1.1 · Adversarial-committed-root regression gates for every P/A/B break (RED-first)** ·
  ✅ DONE. Proved **all 11 P/A/B witness fields forgeable**: each gate forges the committed
  root from the forged ops, then asserts non-nil while forgedRoot≠honestRoot. Settled the open
  tension by evidence — class-P `Weight` IS forgeable (`epochSetRoot` is membership-only).
- **R1.2 · Re-anchor P/A/B witness values as Resolved fold OldValues** · ✅ DONE. Every
  untrusted P/A/B value/predicate and the class-A screen read is now `Resolve`d against
  `prevStateRoot` (the one root the attacker does not control); **NoWitness ⇒ stall** (never
  falls through to a false/absent read). The whole-set digest pre-sets are fold-anchored.
- **R1.3 · RE-OPEN + REFUTE the class-A/P/B directional certs** · ✅ REFUTED (2026-09-01).
  The 2026-08-31 class-A/P/B directional certs are **WITHDRAWN**: their "fold-equality
  (`postRoot == StateRoot`) is a universal backstop" premise is FALSIFIED (an attacker who
  forges the block controls the committed root too, so equality holds by construction). Correct
  direction named: **Resolve-anchoring against `prevStateRoot`**.
- **R1.4 · Witness-soundness recompute cert (23-field carrier table, membership-vs-value ×
  source)** · ✅ CERTIFIED (2026-09-01), bounded strictly to **recompute soundness as a
  NEVER-ACCEPT increment**. The 23-field carrier table is complete by reflection with teeth;
  each of the 10 per-field anchors plus the class-M poisoning path has a driven
  adversarial-committed-root gate that wrong-accepts if its anchor is dropped. This is NOT the
  flip's certification — it is its precondition.
- **R1.5 · Accept-flip model-check exercising the NEW Resolve path** · ✅ **DONE (PR #702).**
  A scheduling oracle over the Resolve path: honest-baseline agreement, I1 (disjoint boxes never
  conflict-Accept), I5 (an honest node is never slashed), multi-block Resolve stability under
  reorder, and no forged-witness poisoning of the next `prevStateRoot`; two OPEN-BREAK gates carry
  distinct forged roots, so neither is vacuous. Absorbs old Rock 4 (#406).
- **R1.6 · Oracle coverage: per-field adversarial Resolve-path probes** · ✅ **DONE (PR #701).**
  Surfaced and gated a class-P activation-lock wrong-accept; the three OPEN-BREAK gates were each
  confirmed non-vacuous (forged root ≠ honest committed root). The class-P fix landed separately
  (PR #704, Direction A + B), which is why R1.4 was re-opened and re-closed.
  **Residual:** `probeUncovered` still does not name A1/A2/A3 — that half is carried as R3.2, and
  the class-A probes must be re-pointed if the `LastCommit` carrier lands (they read `b.Atts` today;
  `R-BOX-ATTESTS-scoping-CONVERGED-RESEARCH-VERDICT-2026-09-02.md` §9 stale-list).
- **R1.7 · External red-team pass (B8) — DECISION RATIFIED (re-ratified 2026-09-03): the external
  pass is a HARD precondition of the flip, and it runs at the RELEASE CANDIDATE, AFTER the era-4
  freeze.** Owner milestone-call + external seat; it is the same gate as **R4.4**, not a second one.
  The recompute's OUTPUT is C2's INPUT; internal cert cannot close "no adversary forges a witness the
  recompute accepts." **The artifact it attacks is the FROZEN, still never-Accept box** — attacking
  an unfrozen format would buy a pass that has to be re-bought at the freeze. R1.5 (model-check) and
  R1.6 (per-field probes) are DONE, which is what converts the external pass from discovery to
  bounded confirmation.
- **R1.8 · The flip: wire `WitnessValidateV5` → Accept-iff-all-predicates-pass** · Builder · S.
  **NOT done.** Trivial code; a consensus-rule change (I1). Beyond R1.5/R1.6 green, the flip
  **additionally requires** (per the R1.4 cert §R1.4-FLIP):
  - **R-membership** — OPEN: a set-size bound on the qualified / `validatorsSeen` sets;
  - the **EXTERNAL B8 red-team pass** (R1.7) — owner-ratified HARD precondition;
  - the **recovery-boundary decision** (cold-auditor directive-trust boundary) — repro/residual
    in [`docs/thinking/2026-09-01-residual-defect-repro-recipes.md`](docs/thinking/2026-09-01-residual-defect-repro-recipes.md)
    (formerly #535);
  - the **legacy-mode invariant** (the pre-v5 path stays sound under the flip);
  - **R-FOLD-LIVE-STATE-READS — the fold reads NO live box state** (research cert
    `floorbox-R-FOLD-LIVE-STATE-READS-RESEARCH-CERTIFICATION-2026-09-02.md`, GATED). The class-A
    screen selected its qualification branch from `c.matureEpoch` and its anchor eligibility from
    `c.launchAnchor` → `c.handedOff()` — box-own fields written only by `apply→rotateEpoch` and
    `adopt`. The deployment target replays no `apply()`, so a COLD box never set them and screened
    every mature-epoch block under the pre-maturity rule: wrong-accept of a mid-epoch joiner against
    an attacker's root, and false stall on an honest one, with every witness proof passing.
    **Direction A landed (PR #706)** (the branch selector is the Resolved `tagMatureEpoch`
    pre-value anchored against `prevStateRoot`; `launchAnchorGiven` is the one shared predicate)
    together with the
    **R-COLD-BOX-HARNESS** tier and the fold-file live-state allowlist pin. The remaining flip
    obligation is that the cold-box tier stays the tier every new recompute gate runs in.

  **Added to the precondition list 2026-09-02/03** (each is a Rock in the *New Rocks* block below,
  with its source):
  - **R-STRUCTURE-REDERIVATION** — the one-accept-composition build (owner decision pending), and
    its **R-STATEVIEW-ENUMERATION** precursor, which is a FREEZE precondition earlier in the order
    than the flip itself;
  - **R-CARRIER-PARENT-BINDING** — a flip ENTRY blocker: the box reproduces `validateCarrier`
    without the ancestry precondition its node-side caller establishes;
  - **R-CARRIER-PARENTPROPOSER** (`tagLastProposer`) and **R-CARRIER-BYTES**, both also pre-freeze;
  - **R-BOXENTRY-RESIDUALS** — box-entry round-A findings N1–N8 and the §8 residual set;
  - **FP-1** — `Bank.spent` persistence, re-armed the moment witnessed demand confers value.

  **ORDERING — RATIFIED 2026-09-03 (supersedes the earlier "decoupled; the flip proceeds
  pre-freeze" clause):** the flip lands **AFTER** the era-4/v5 format freeze (R3.4) and **AFTER** the
  external B8 pass on the frozen artifact. `WitnessValidateV5` still changes no committed format
  field — the reason for the new order is the B8 pass, not the box: a pass bought against an unfrozen
  format has to be re-bought at the freeze. Two further couplings this session make the order
  load-bearing rather than merely tidy: the state-view enumeration the STRUCTURE build needs is
  itself a **freeze precondition** (it decides which committed leaves the box requires, and a leaf
  discovered after the freeze is a new era), and `R-CARRIER-BYTES` is a v5 **validity rule** that
  cannot land in-era. PE ruling
  `/Users/andrewedmond/Claude/claude/silt-reviews/principle-engineer/RULING-floorbox-predicate-rederivation-structure-2026-09-03.md`
  §7.

**Carried residuals (worth a line, owed before the flip):**
- **R-CARRIER-REFLECTION — DONE (PR #705, 2026-09-02, test-only).** The fold-input carriers were verified
  by hand; they are now pinned by reflection. `TestFoldInputCarrierCoverageIsComplete`
  (`core/chain/floorbox_recompute_carrier_reflection_v5_test.go`) walks the transitive struct
  closure of the state-root fold's witness bundle (13 carrier types / 74 fields) and requires exact
  equality with the declared coverage (`r12CoverageTable` ∪ `foldInputCoverageTable`), so an added
  carrier type, an added field, or a stale row goes RED. Teeth demonstrated by injection.
  The R-FOLD-LIVE-STATE-READS fix added the `StateRootMaturityWitness.MatureEpoch` carrier field;
  it ships with its own `r12CoverageTable` row, so the pin stays green.
- **R-COLD-BOX-HARNESS — CLOSED as a permanent tier** (`core/chain/floorbox_recompute_coldbox_v5_test.go`).
  Every recompute gate had run on the chain that APPLIED the history, so the test tier shared the
  producer's blind spot — third occurrence in this spine (R1.3 fold-caught premise, class-P
  suppression, live-state reads). The tier drives the real entry on a `New(cfg)` box that never
  applied a block. New recompute gates run in it.
- **R-VERIFYBOND-WIRING — CLOSED.** The injected `verifyBond` is asserted once at the box entry
  (`ErrRecomputeBoxWiring`), so the #572 replay shape fails loud and named instead of as a fold
  mismatch three classes later.
- **R-ROTATE-EPOCH-LAST — DONE (PR #703, test-only): `rotateEpoch`-is-last-in-`apply` is now
  pinned by a drift guard** as load-bearing for `epochSet` order-independence (#621).
  Distinct from R-CARRIER-REFLECTION. `epochSet` order-independence
  (proven in #620) holds only because `rotateEpoch` runs LAST in `apply` (`core/chain/chain.go`),
  reading only the final post-block `bonded`/`slashed` state, so `epochSet =
  liveQualifiedSet(bonded, slashed)` is order-invariant by construction. A refactor that moves
  `rotateEpoch` before slash/bond application, or makes `liveQualifiedSet` read history rather than
  the two final maps, would silently break the SMT history-independence premise for `epochSet` and
  #620 would no longer hold. Owed before the era-4 format freeze (Boulder 3): a drift-guard
  assertion or test that fails if the freeze can observe pre-final state — **shipped**. PE ruling:
  `RULING-620-mature-epoch-order-independence-2026-08-28.md` ("Couplings the consult should carry
  forward").

**New Rocks opened 2026-09-02/03 (Boulder 1 — flip preconditions and pre-freeze items).** Each
carries its source; none is decided by a seat.

*The structure (the shape the box is built in):*
- **R-STRUCTURE-REDERIVATION — OPEN, owner decision pending.** Build ONE accept composition
  `ValidateCommitV5(view, block)` over a three-valued `StateView`, both callers, all box doors
  unexported behind `WitnessValidateV5`; contract form becomes `box.Accept ⇒ node.Accept`. Five of
  five box defeats this cycle were **composition** errors (a tail reproduced without the
  precondition its caller established), not missed screens, so a shared predicate *set* does not
  close them. `/Users/andrewedmond/Claude/claude/silt-reviews/principle-engineer/RULING-floorbox-predicate-rederivation-structure-2026-09-03.md`
  (option E), `/Users/andrewedmond/Claude/claude/silt-reviews/research/research-outcome/floorbox-predicate-rederivation-STRUCTURE-RESEARCH-VIEW-2026-09-03.md`
  (option D).
- **R-STATEVIEW-ENUMERATION — OPEN, a FREEZE precondition and the earliest item in the order.**
  The certified inventory of every state read on the accept chain, classified as committed leaf /
  box-owned config / block-local. The PE's one-pass enumeration closes with exactly one non-leaf
  fact (`parent.ProposerID()` → `tagLastProposer`); **its whole job is to find what that pass
  missed**, because a second non-leaf fact discovered after the freeze is a new era. Source: the PE
  structure ruling §3, §7.
- **R-BOXENTRY-RESIDUALS — OPEN (box-entry round A, HELD unmerged).** The exported-door surface is
  recommended HELD (zero production callers today; unexporting is free now and migration work once
  part B starts). Live findings owed with it: **N1** unqualified author credited by the frozen
  weight quorum, **N2** the O2 precedence bypass via the driver-supplied `HasHeldHead`, **N3** the
  backward/off-chain pin walk, **N4** the de-mature super-quorum crediting a mid-epoch joiner,
  **N5** the missed fatal out-of-scope replay leg, **N6** the over-broad O2 refusal, **N7** the zero
  `PinRecord` resetting the forward-progress guard, **N8** the fail-path lazy-fold DoS. Plus the
  §8 residual table's open set (R-SUPPORT-SLASHED-SCREEN, R-MALFORMED-DIVERGENCE,
  R-PIN-LABEL-ESCAPE, R-PIN-REANCHOR-POSITION, R-PIN-BUDGET-ESCAPE, R-FENCE-TABLE-DRIFT,
  R-BOUNDARY-PREDICATE-COVERAGE, R-BOX-ATTESTS invariant II / G-F). Sources:
  `/Users/andrewedmond/Claude/claude/silt-reviews/red-team/RED-TEAM-floorbox-box-entry-roundA-4a394fd-RE-BREAK-2026-09-03.md`,
  `/Users/andrewedmond/Claude/claude/silt-reviews/research/research-outcome/floorbox-box-entry-round-A-fee43ba-DELTA-CERTIFICATION-2026-09-03.md`
  §8.

*The `LastCommit` carrier — R-BOX-ATTESTS. **O1, O2 and O4 are OWNER-RATIFIED (2026-09-03);
O3 is pending** (source for all four:
`/Users/andrewedmond/Claude/claude/silt-reviews/research/research-outcome/R-BOX-ATTESTS-scoping-CONVERGED-RESEARCH-VERDICT-2026-09-02.md`
§10). Built on `builder/lastcommit-carrier`, NOT merged.*
- **O1 — RATIFIED: the `LastCommit` carrier is an additive format field of the OPEN era**
  (`LastCommit []Attestation`, additive cbor key, `omitempty`, **folded into `Hash()`**). Validity:
  every entry verifies over `b.Prev`'s hash at `PhasePrecommit` at any single round (`CommitRound` is
  uncovered, so the rule must not bind to it), distinct ids, a pre-v5 block carrying the field is
  invalid, height 1's carrier is empty by rule, genesis carriers refused. Transition: seat each
  carried signer with `id != parent.ProposerID()` and `attesterQualified` evaluated against the
  child's pre-state; the child's own `Atts` write nothing; the frozen era-3 rule is left byte-for-byte.
  The carrier fold runs **before** this block's bond regs / TTL / slashes, pinned the way rotate-LAST
  is pinned. Disclosed: the seat lands one block late; a proposer can DELAY a seating but never FORGE
  one. The smaller same-block `SeatAdds` alternative is explicitly not recommended.
- **O2 — RATIFIED: the rollout rule, both paths.** The readiness stamp goes **3 → 5 directly; no
  release ever stamps 4** — so era-3's frozen format is retired **without ever running**, recorded in
  #632 as *frozen-and-retired-unrun* (a doc note; it edits no format). And: **no mainnet era
  activation, by tally OR by pre-latch genesis override, on a binary without the carrier** — the
  override bypasses `regVersion` entirely, so the stamp rule alone does not cover it. Note the
  enforcement limit found this session: a "refuse activation on a pre-carrier binary" check is
  **vacuous by construction** (any binary carrying the check carries the carrier); the predicate that
  actually binds is *the full v5 carry-list has landed*, which no binary can evaluate about itself
  except as the hard-coded stamp. The mixed-fleet exposure is safety-preserving (a pre-carrier node
  drops cbor key 18, computes a different `Hash()`, and **stalls loudly**; it does not commit
  divergent state) — the residual is `R-CARRIER-ROLLOUT-SIGNAL` below. Sources: §10 (O2) and
  `/Users/andrewedmond/Claude/claude/silt-reviews/research/research-outcome/R0.4b-C3-G8-dark-lane-CONVERGENCE-2026-09-03.md`
  §2, §4.
- **NOT re-opened (recorded so it is not silently reopened):** option **M** — re-basing the maturity
  metric on `bonded` instead of `validatorsSeen`. It would remove the "attested a committed block"
  anti-declaration property, which makes it a **published-claim** change (Sybil metric semantics),
  not a consensus fix. Declined by default; reopening it is a separate certification. Source: §10.
- **R-CARRIER-PARENT-BINDING — OPEN, a flip ENTRY blocker.** The box runs the shared
  `validateCarrier`, but not the ancestry precondition its node-side caller establishes: on the full
  node `ValidateProposal` refuses `b.Height != height || b.Prev != prev` FIRST, so "a genuine
  precommit over `b.Prev`" means "over THE PARENT"; in the box it means "over a hash the ATTACKER
  chose". Replaying a stale certificate re-opens the `everMature`/C2 maturity forge **with zero
  forged signatures**. Two candidate directions, neither built nor certified: give the box the
  parent hash+height as inputs, or commit the pair as v5 scalar leaves Resolved against
  `prevStateRoot`. The HEIGHT axis (the height-1 empty-carrier rule) is label-enforced in the box
  and has the same root cause. Source: `.../red-team/RED-TEAM-lastcommit-carrier-26977a4-RE-BREAK-2026-09-03.md`
  RT2-CARRIER-13 / 13b / 13c.
- **R-CARRIER-PARENTPROPOSER / `tagLastProposer` — OPEN, flip AND freeze precondition.** The class-A
  carrier fold excludes `id == parent.ProposerID()`, which is **not a committed leaf**; the witnessed
  `(pub, sig)` anchor is one-sided — DROP is bounded, **ADD is not** (a freshly minted keypair
  verifies, matches no entry, and the parent's true proposer self-seats). Certified fix direction: a
  `tagLastProposer` committed scalar leaf, Resolved like every other class-A input. **Additive
  committed-format change on the open era — it must land BEFORE the era-4 freeze.** Source:
  `/Users/andrewedmond/Claude/claude/silt-reviews/research/research-outcome/LASTCOMMIT-CARRIER-round-A-5d3fda0-RESEARCH-CERTIFICATION-2026-09-03.md`
  §6.2 (FP-1).
- **R-CARRIER-BYTES — OPEN, PROMOTED to a flip precondition, and a v5 VALIDITY rule (pre-freeze).**
  See the *Decisions owed* block for the measured numbers. A bound needs its own certification plus
  owner ratification; do not add one inline. Sources:
  `/Users/andrewedmond/Claude/claude/silt-reviews/research/research-outcome/LASTCOMMIT-CARRIER-round-A-5d3fda0-RESEARCH-CERTIFICATION-2026-09-03.md`
  §10.1,
  `/Users/andrewedmond/Claude/claude/silt-reviews/research/research-outcome/LASTCOMMIT-CARRIER-26977a4-DELTA-CERTIFICATION-2026-09-03.md`
  §2.4,
  `/Users/andrewedmond/Claude/claude/silt-reviews/red-team/RED-TEAM-lastcommit-carrier-26977a4-RE-BREAK-2026-09-03.md`
  RT2-CARRIER-15 / 17.
- **R-CARRIER-PRUNED-HASH — OPEN, owed BEFORE the stamp raise (not a flip gate).** `Hash()`
  short-circuits to `b.Pruned`, and `Prune()` keeps `LastCommit` and `StateRoot`, so on a pruned
  block neither is signature-covered: `Pruned` is a linkage token, not a content commitment.
  Pre-existing to the carrier, which is in fact its best-protected member. **Owed:** prove the
  seen-fold never depends on a pruned body, and prove end-to-end that the first non-pruned
  descendant's root recompute catches a rewritten ancestor. Do **not** make `Hash()` cover pruned
  bodies — that defeats pruning (build-immutable #8). Source: the carrier delta cert
  (`/Users/andrewedmond/Claude/claude/silt-reviews/research/research-outcome/LASTCOMMIT-CARRIER-26977a4-DELTA-CERTIFICATION-2026-09-03.md`)
  §5.
- **R-CARRIER-GENESIS-DISPOSAL — OPEN, MERGE-BLOCKING on the carrier branch (MG-C).** The rule
  splits: genesis `Atts` must be **STRIPPED**, genesis `LastCommit` must be **REFUSED**. This
  corrects the round-A certification's own §2. Source: the delta cert §6.
- **R-CARRIER-CREDIT-DENIAL — OPEN, live; owner + Researcher.** The carrier separates the seating
  slot from the quorum slot, and it has **no minimum**: an under-carrying block still COMMITS, so
  the denial lands on the canonical chain with no rule, no minimum and no attribution — available
  actively (omit) and passively (be fast), both indistinguishable from honest behaviour. A carrier
  minimum is a v5 validity-rule change: research-gated, owner-ratified; do not add one inline.
  Source: the delta cert §8.1 (RT-CARRIER-4).
- **R-CARRIER-DOUBLESIGN-SLOT — OPEN, RESEARCH-GATED (slashing-rule change).** The carrier is a
  new, hash-covered, permanently committed slot for consensus signatures that **neither half of the
  equivocation machinery reads** — with the same signatures in `LastCommit` rather than `b.Atts`,
  `FindEquivocations` convicts nobody and the equivocator is seated on both forks. Widening
  slashing evidence to a new signature slot is an I1/F2 rule change: Researcher certifies, owner
  ratifies. Source: the delta cert §8.1 (RT-CARRIER-7).
- **R-CARRIER-ROLLOUT-SIGNAL — OPEN, live at activation; = the O2 enforcement gap (OWNER call).**
  Nothing on the wire distinguishes a pre-carrier from a post-carrier binary: both mint v5 and both
  stamp readiness 3, and unknown cbor key 18 is silently dropped, so a partially upgraded fleet does
  not fork — it degrades silently and asymmetrically back to the frozen-seating state the carrier
  exists to fix. The tally path is protected (only carrier binaries stamp 5); the **genesis pre-latch
  override path is not**. Source: the delta cert §8.1 (RT-CARRIER-3).
- **R-CARRIER-MODELCHECK — GATED, owed BEFORE the stamp raise.** The model-check tier must state and
  drive the seating **AGREEMENT** property under the carrier (every replica seats the same set from
  the same hash-covered carrier), not merely the carrier's own validity. Source: the delta cert §7.

*Fork-choice, canon and inventory (O3 / #558 family):*
- **R-FORKCHOICE-WEIGHT (O3) — consensus recommendation T, OWNER DECISION PENDING.** See the
  *Decisions owed* block for the recommendation and its two sources. Direction T means: remove the
  weight comparison from `heavier`, delete `Weight()` / `blockWeight()` / `anchorWeight()` /
  `Config.AnchorWeight` (no operator-visible contract — no CLI flag exists), **preserve and promote
  the §1b height preference**, and re-ground the three legacy `Reconcile` fixtures on height →
  hash. Do not leave the term inert: a dead consensus quantity that survives a retirement is exactly
  how the third bare-hash verify site happened. **The next four Rocks ship in the SAME commit as
  whichever direction the owner picks.** The reopening condition is narrow and named: evidence of a
  shipping posture in which `FinalizedHeight()` lags `Head()`.
- **R-558-VERIFIER-INVENTORY — OPEN (Tester).** A standing grep-shaped gate that **no attestation is
  verified outside `verifyAtt`**. `#558`'s repair swept two of three bare-hash verify sites;
  `blockWeight` is the third and was missed. Under Direction T the gate is satisfiable by
  construction because the third site is deleted, so it ships **with the T commit**. Source:
  `/Users/andrewedmond/Claude/claude/silt-reviews/research/research-outcome/O3-fork-choice-weight-R-vs-T-RESEARCH-RECOMMENDATION-2026-09-03.md`
  O3-R7.
- **R-INTERLOCK-GATE — OPEN.** The binding interlock ("the `blockWeight` signature verify is never
  repaired alone") has **zero test enforcement**: the PE applied exactly the forbidden one-line
  change and `./core/... ./sim/...` was fully GREEN, EXIT=0. A gate is owed in the same commit as
  whichever O3 direction lands. Source:
  `/Users/andrewedmond/Claude/claude/silt-reviews/principle-engineer/RULING-O3-fork-choice-weight-R-vs-T-2026-09-03.md`
  §5.
- **R-FORKCHOICE-RAMP-GUARD — OPEN (Tester), small.** Delete the `Weight() > 0` assertion at
  `core/chain/forkchoice_ramp357_test.go:66-69`; do **not** repair it. It asserts a property
  production does not have and is green only because the fixture hand-builds `Version: 1` blocks
  with era-1 attestations inside an objective config — a guard that passes only under an attestation
  era no node mints is not a guard. Invariant D in the same file is the part that guards #357 and it
  stands. Source: the O3 research recommendation §3.5 item 3 / O3-R3.
- **R-I5-TEXT-AND-CLAIMS-LEDGER — OPEN, RESEARCH-GATED.** Two canon/ledger edits ride the O3
  decision: the I5 restatement in `docs/design/consensus-invariants.md` (editing an invariant's
  stated rule is inside the research gate — the PE explicitly does not assert that certification),
  and `docs/design/claims-ledger.md:47` ("objective fork-choice heals a partition to the
  heavier-standing chain"), which has been **false since 2026-08-16** and is backed by an era-1 unit
  test no production path can produce. Re-word it to name the real mechanism: quorum finality plus
  height. Sources: the O3 PE ruling §6 and the O3 research recommendation §3, O3-R2.
- **R-O4-CANON-HASH-COVERAGE — RATIFIED in substance, NUMBERING PENDING.** Amend canon with the
  hash-coverage rule covering **both** the transition and the fork-choice weight, with both scars and
  both code sites; plus the #632 *frozen-and-retired-unrun* note. Whether it widens **I5** or mints
  **I6** is the owner's; both seats agree the split is non-substantive. Source:
  `/Users/andrewedmond/Claude/claude/silt-reviews/research/research-outcome/R-BOX-ATTESTS-scoping-CONVERGED-RESEARCH-VERDICT-2026-09-02.md`
  §10 (O4).

*Gate quality, coverage debt and small items:*
- **R-AST-PIN-GLOB — OPEN, small.** The R-FOLD-LIVE-STATE-READS AST pin's glob does **not** cover
  the file where three of five box defeats live: `floorbox_recompute_v5.go` and four others are
  outside it. Direct evidence that AST gates fail by scope. Widen the glob and re-run. Source: the
  PE structure ruling §8 item 2.
- **R-INVENTORY-HAND-LIST — OPEN (Tester).** A gate that claims to cover "every X" but enumerates X
  as a hand-written literal is green because the list is shorter than X, and worse than no gate
  because the claim is now published in a test name. Demonstrated on the eleven-door legacy fence
  (which misses `RecomputeMatureNowStreaming`) and mirrored by R-FENCE-TABLE-DRIFT. Derive the
  surface from the code, not from a list. Source:
  `/Users/andrewedmond/Claude/claude/silt/.claude/agent-memory/tester/scar-inventory-gate-is-a-hand-list.md`.
- **R-S5-STRING-REGISTRY — OPEN (Builder + Tester); scar at count 2, THIRD-TIME RULE ONE AWAY.**
  An announced operator string is a contract (S5), and an upstream early-return breaks it while every
  unit test stays green (instance 1: `freeload: ON`; instance 2: the R0.4b `NOT banked` refusal). The
  proposed gate is a registry of contracted operator strings (27 seed entries verified present) plus
  a unit-tier presence test with a teeth test, keeping the e2e assertion for every registered marker
  — the registry proves the string is still in the SOURCE, never that it is still REACHABLE. Full
  buildable spec:
  `/Users/andrewedmond/Claude/claude/silt/.claude/agent-memory/tester/scar-observable-log-contract.md`.
- **R-SWARM-NOTBANKED-DEAD — OPEN, LOW.** After the lane-off legibility fix and PE H-2, the
  `NOT banked` branch in `cmd/silt/swarm.go` may be genuinely unreachable on the shipped daemon. It
  needs either a reachability argument or removal — a dead branch must not carry an S5 contract.
  Source: `/Users/andrewedmond/Claude/claude/silt-reviews/research/research-outcome/R0.4b-C3-G8-dark-lane-CONVERGENCE-2026-09-03.md`
  §5.
- **R-E2E-ERA4-FIXTURE — OPEN, a prerequisite of the STAMP-RAISING release (not of any PR).** The
  e2e delivery-receipt fixture runs `-objective=false`, so `epochsEnabled()` is false, `rotateEpoch`
  never runs, and the era-4 readiness tally **can never latch on it at any stamp value, on any
  binary, forever**. The fixture must become objective + bonded + epoch-enabled before it can ever
  green by tally — **never by an activation override, in any form, in any binary**. Carries the
  coverage debt from G-8 option (iii): the e2e paid-lane positive arm is uncovered until then, and
  the two genuinely-uncovered arms (the three-call happy-path composition; the second genuine
  delivery on one lane) are owed as a node/sim-tier composition gate now. Source: the G-8
  convergence §3, §4, §7.
- **R-ISSUERKEY-POP — OPEN, crypto residual (latent).** `validateIssuerKeys` requires a verifying
  ed25519 self-signature, an in-range epoch and a bond, but **no proof-of-possession of the RSA
  key** — a bonded issuer can register another issuer's public-key fingerprint, a DSKS surface. Not
  a live break today (one configured issuer, per-node ledgers), and the faithful RFC 9578 import
  forecloses it by construction: sign `issuerID(32) ‖ epoch(8, BE) ‖ serial` (or the key
  fingerprint) instead of the bare epoch. Until then, correct the two code comments claiming RFC 9578
  fidelity. Source:
  `/Users/andrewedmond/Claude/claude/silt-reviews/crypto-specialist/ADVISORY-R0.4b-C3-blind-RSA-epoch-binding-2026-09-03.md`
  C-4 / Q3.
- **FP-1 · `Bank.spent` persistence — OPEN, a flip precondition.** The in-memory spent guard has
  less durability than the thing it guards; narrowed, not waived, and re-armed the moment witnessed
  demand confers any value. Source: the composed cert Residuals; the FP-1/FP-2 text is CERTIFIED
  faithful in `R0.4b-C3-271ab81-G3-G4-GD-DELTA-CERTIFICATION-2026-09-03.md` §4.
  *(The mirror crash window — the guard append lands, the payment is lost — is RULED SAFE and owes no
  gate: it is a pure under-pay that self-heals when the window advances, delta cert §3.)*

- **SMT app-layer keyspace-injectivity oracle** — a **freeze-blocker (A2)**. It is currently a
  decoration; it needs a defect-injected gate (inject a keyspace collision, watch it go RED)
  before it counts as coverage.

#### Boulder 2 — Turn the economy on, prove it under adversary · depends on Boulder 0
All solvency claims are sim-only today (economy default-off, no live enable path). An
economy-off HEAD certifies a network nobody runs. Design:
[`docs/thinking/2026-09-01-economy-observability-design.md`](docs/thinking/2026-09-01-economy-observability-design.md).

- **R2.1 · Economy observability MVP + node-local APIs** · Builder · L (sliceable) ·
  **Slice 6a DONE** (shipped in #689 / commit 94c5c04). 4 local-exact panels (my solvency /
  am-I-profitable / durability self-funding / wash self-check), extending the existing
  `/api/status` durability block; ships cert-free, economy-off. The per-node `repairsDone`
  counter landed with it (`core/credit/credit.go:71`, incremented at
  `core/credit/escrow.go:177`), unblocking the repair-work Gini (R2.2).
- **R2.2 · Full observability set + testable telemetry gate** · Builder + Tester. Serve-work
  Gini AND repair-work Gini (separate), per-tier margin, live `g`, funded-horizon-to-expiry,
  wash-detection; network panels via the DHT crowd-estimator (knowability tiers).
- **R2.3 · A4-fix + economy-ON packaging — DECISION RATIFIED: separate; A4 fix first.** The
  conservation re-cert (R0.4) lands before the economy default flips regardless of packaging.
  ★ The flag that arms A4 is `-accept-delivery-receipts`, not `-economy` — sequence it behind
  the re-cert; do not let it ride silently in an economy-on PR.
- **R2.4 · Economy-ON default flip** · Builder. After Boulder 0 + R0.4 cert.
- **R2.5 · C-5 G2 RAM measurement at production chunk — DONE (2026-09-01): 1024 MiB resident.**
  Measured locally at production chunk (16 × 64 MiB), +~512 MiB reclaimable (1536 MiB
  allocation-inclusive peak). Consequence: on a 2 GB pony ONE repair fits, TWO concurrent
  prod-chunk repairs OOM. Build-day: confirm a repair-concurrency limiter in
  `core/node/repair.go` (PE found none); fold into the owed node-store coexistence test.
- **R2.6 · Repair-payee model — DECISION RATIFIED: HOLD the ratified design; convert to a
  G2-gate.** The `selfHold` conditional payee ALREADY pays the reconstructor where safe
  (2026-08-19 ruling); the real surface is only the domain-collision case, and the binding
  constraint is COST (the G2 RAM spike), not incentive. Any re-open needs a cert grounded on
  the measured G2 (**cert-gated**, re-opens D-S7). Real seam: who re-endows cold-escrow + the
  missing per-tier repair-work-Gini telemetry.
- **R2.7 · Economy-ON adversarial-solvency verdict + attack pass** · Researcher (NEW-CERT,
  **cert-gated**) + red-team. After R0.3 (mint fixed) + R2.6 (payee decided) + R2.2 (live
  telemetry). Feeds the #183 external brief.
- **R2.8 · Cold-repair funding path (who re-endows the long tail)** · Builder/economist. After
  R2.2 quantifies the heat/tier/repair correlation silt cannot measure today.

**New Rocks opened 2026-09-03 (Boulder 2 — economy):**
- **R2.9 · D-POD-KNOBS re-pricing is now LOAD-BEARING (was a deferred knob)** · Researcher
  (NEW-CERT, **cert-gated**) + economist. `R-FLAT-FEE` — a **flat** conserved fee (50,000) against a
  **byte-proportional** unfunded self-mint (`bytes − bytes/8`) — is the ROOT CAUSE of G-4: past a
  break-even of 50,000 accumulated lane bytes, **refusing** a witnessed receipt paid the server more
  than accepting it, and the operator could trigger the refusal itself. G-4 is CLOSED at `271ab81`
  by ordering every refusal below the supersede, so the lever is gone — but that fix makes the
  standing consequence permanent: **every refusal now costs an honest server the entire accumulated
  lane self-mint for that object**, and every new refusal path must go below the supersede by
  default. `B` is the **accumulated lane** (the whole object), not one chunk, and the shipped default
  chunk is 64 KiB — so the lever was live at the default, not only at the frame ceiling. Re-pricing
  is a D-POD-KNOBS certification of its own. Sources:
  `/Users/andrewedmond/Claude/claude/silt-reviews/research/research-outcome/R0.4b-C3-composed-close-bc062d0-RESEARCH-CERTIFICATION-2026-09-03.md`
  §4, `/Users/andrewedmond/Claude/claude/silt-reviews/research/research-outcome/R0.4b-C3-271ab81-G3-G4-GD-DELTA-CERTIFICATION-2026-09-03.md`
  (Residuals), `/Users/andrewedmond/Claude/claude/silt-reviews/economist/ADVISORY-R0.4b-cap-griefing-grant-and-bonded-fetchers-2026-09-03.md`.
- **R2.10 · F8 / FP-2 — the ledger must own a CHAIN-ANCHORED epoch; a clamp is not a close** ·
  Builder + Researcher. Gates any shared-ledger, third-operator-settlement or **persisted**-ledger
  deployment, and it **blocks the faucet rate limiter** (the limiter must not key on the ledger
  watermark). Unreachable on the shipped topology today (one production caller, passing the node's
  own `chainEpoch()`). Sources:
  `/Users/andrewedmond/Claude/claude/silt-reviews/research/research-outcome/R0.4b-C3-composed-close-bc062d0-RESEARCH-CERTIFICATION-2026-09-03.md`
  §5;
  `/Users/andrewedmond/Claude/claude/silt-reviews/research/research-outcome/R0.4b-C3-271ab81-G3-G4-GD-DELTA-CERTIFICATION-2026-09-03.md`
  §4.
- **R2.11 · R0.4b-11 — no peer-submit path for an issuer-key registration.** An attest-only
  validator's key is never committed. Fail-closed, liveness only. Source:
  `/Users/andrewedmond/Claude/claude/silt-reviews/research/research-outcome/R0.4b-C3-composed-close-bc062d0-RESEARCH-CERTIFICATION-2026-09-03.md`
  (Residuals → Open).
- **R2.12 · Grant + faucet rate limit; `RequireBondedFetchers` default — OWNER CALLS, pending.**
  See the *Decisions owed* block. The limiter must gate the **grant**, not `Ledger.Register` (which
  is reached implicitly from `acct()`, including for the node's own id and for `PayBounty`).
  Sources:
  `/Users/andrewedmond/Claude/claude/silt-reviews/economist/ADVISORY-R0.4b-cap-griefing-grant-and-bonded-fetchers-2026-09-03.md`;
  `/Users/andrewedmond/Claude/claude/silt-reviews/research/research-outcome/R0.4b-C3-composed-close-bc062d0-RESEARCH-CERTIFICATION-2026-09-03.md`
  §6 (the "grant is the only non-self balance" claim is CORRECTED there: `PayBounty` credits a
  remote payee, but it is earned work, not a faucet).
- **M_seen — the pony-class value is still OWED (carried).** The class-M streaming verifier (PR
  #709) removed RSS as the binding ceiling; the remaining ceiling is **TIME**, the O(N·log N)
  compute floor streaming does not remove. The cap value must be derived from a pony-class
  measurement, and the transport budget must be stated with it. Source: `CHANGELOG.md` (Unreleased,
  the class-M streaming verifier entry) and
  `/Users/andrewedmond/Claude/claude/silt/.claude/agent-memory/tester/class-m-maturity-fold-cost-measurement-2026-09-02.md`.

#### Boulder 3 — Freeze prerequisites (era-4/v5) · parallel · freeze-gated · was Rock 3
- **R3.1 · Close/own the SMT second-preimage / domain-separation residual** · Builder (scope to
  hashing first) + crypto/Researcher confirm. Load-bearing: a leaf/internal second-preimage
  would defeat the fold-OldValue soundness Boulder 1 relies on. Design (NO hash change; the
  library domain-separates by construction for silt's fixed-width leaves):
  [`docs/thinking/2026-09-01-smt-domain-separation-close-design.md`](docs/thinking/2026-09-01-smt-domain-separation-close-design.md).
- **R3.2 · Close the oracle probeUncovered debt fully (from R1.6)** · Tester. Freeze gate.
- **R3.3 · Record PayWord/RegCap re-derivation dependencies** · doc-only; re-derive only if
  the bond-proof size / N² cost residual moves (measured numbers in the **Residual backlog**
  below, "Bond-proof reply size / N² cost"; formerly #299).
- **R3.4 · era-4/v5 format freeze — DECISION RATIFIED, REVISED 2026-09-03: still at the RELEASE
  CANDIDATE (not end-of-PoD), and it is now the **stamp-raising** release that the flip and the
  external B8 pass come AFTER, not before.** The earlier "DECOUPLED from the flip; the flip proceeds
  pre-freeze" clause is **superseded**: R1.8 lands after the freeze and after B8 on the frozen
  artifact (see the sequencing constraint at the top of the Boulders). Owner-call, after the field
  set settles (R1.4) + domain-sep owned (R3.1). The prior "freeze at end-of-PoD" framing remains
  superseded.
  **Pre-freeze carry-list (each is unfixable in-era once the format freezes):** `tagLastProposer`
  (R-CARRIER-PARENTPROPOSER) · the `R-CARRIER-BYTES` byte ceiling · the `Block.IssuerKeys` per-block
  count cap (PE H-1) · **R-STATEVIEW-ENUMERATION** (it decides which committed leaves the box needs;
  the PE's one-pass sketch found exactly one, and its job is to find a second) · the O3 fork-choice
  decision · R-membership. **Owed before the stamp raise, but not before the freeze:**
  R-CARRIER-PRUNED-HASH's end-to-end test, R-CARRIER-MODELCHECK, the `R-E2E-ERA4-FIXTURE` upgrade.
  Sources: `/Users/andrewedmond/Claude/claude/silt-reviews/principle-engineer/RULING-floorbox-predicate-rederivation-structure-2026-09-03.md`
  §7, `/Users/andrewedmond/Claude/claude/silt-reviews/research/research-outcome/R-BOX-ATTESTS-scoping-CONVERGED-RESEARCH-VERDICT-2026-09-02.md`
  §9–§10, `/Users/andrewedmond/Claude/claude/silt-reviews/principle-engineer/RULING-R0.4b-C3-close-271ab81-final-2026-09-03.md`
  §3.

#### Boulder 4 — Standing gates + M0 endgame (post-PoD hardening) · was Rock 6
- **R4.1 · PoD demand→standing bright-line gate (fires per PoD increment)** · Researcher cert
  on trigger (**cert-gated**). Any increment wiring served demand toward standing re-opens the
  C1 discount (γ→1/N, #182). A review gate, not scheduled work.
- **R4.2 · Wire the A-axis (operator/domain) into standing** · Builder + Researcher (NEEDS-CERT,
  **cert-gated**) · L · depends on PoD. Highest-value M0 hardening after the flip; today
  `C_honest ≈ D`, A-axis self-declared (reputation-Sybil seam). The self-declared A-axis is an
  explicit SECOND B8 external target.
- **R4.3 · Continuous internal red-team hunt on the not-yet-run backlog** · red-team → feeds the
  external seat. Backlog: class-P compound-block ordering; bondreg full path; DHT/eclipse/A-axis
  layer; long-range/weak-subjectivity checkpoint; relay/PayWord economy; churn/restart
  everMature under the R1.2 refactor.
- **R4.4 · External red-team vs the C1 + C2 composition and the seven §7 seams (#183) — THE M0
  close gate.** This is the RC-defining gate, and **it is the same pass as R1.7** — one external B8
  engagement, not two. **RATIFIED 2026-09-03: it runs at the RELEASE CANDIDATE, AFTER the era-4/v5
  format freeze (R3.4), against the frozen, still never-Accept artifact; R1.8 (the accept-flip) lands
  after it.** A fresh, no-memory EXTERNAL red team (self-graded
  does not count — B8 requires the certifying adversary to be external) attacks the BUILT system
  and must find no strategy that (a) earns quorum-controlling standing for less than `q · C_honest`
  (**C1**), (b) concentrates bonded weight past capture under adversarially-skewed measurement
  (**C2**), or (c) breaks one of the seven `docs/design/m0.md` §7 composition seams (re-pricing/wealth
  residue; cold-start scaffolding-capture; real-demand > wash-demand; privacy↔attribution linkage;
  operator-clustering heuristic; time-axis gaming; new liveness/griefing surfaces). The unit of test
  is the SYSTEM, not a primitive — a primitive failing a standalone Sybil-proof test is Douceur
  (expected). A seam held in tension (bounded cost, documented residual) is a PASS; a seam silently
  assumed closed is the failure mode. Findings are triaged/fixed to the build-immutable bar (unit +
  integration + e2e + inverted PoC) and re-attacked until a clean verdict. Brief:
  `docs/reviews/m0-redteam-brief-2026-08.md`. Pairs with the multi-machine field test (#52) to render the
  verdict. This is the gate to declaring M0 *held* (not merely built) and defines the R1 field
  grade → V1 endgame. #183's close condition is MET and the issue carries the evidence — the close
  is the owner's call, deliberately held.
- The self-declared A-axis (R4.2) is an explicit SECOND external B8 target once wired.

**Watch-items / standing gates (not scheduled):** bond-floor vs pony-disk ratio (a dashboard
row); owned-residual doc lines (SHA-256 pinned, store-free verify, R3 16 KiB cap); PayWord
re-derivation. **A4 fix (Boulder 0) is Third-operator settlement's predecessor** — the old Rock 2
(third-operator committed settlement, DEFINITION, #658) is unchanged and attaches its
demand→standing bright-line to R4.1.

<details>
<summary><b>Superseded Rocks (2026-08-19→08-31 overlay — kept as history)</b></summary>

> **⚠ Reconciliation flag (2026-09-01).** The old Rock 1 read *"the accept-flip is the single
> remaining step."* The 7-seat audit found that FALSE: the flip has a witness-soundness spine
> (re-anchor + refute + cert table + gates + external pass — Boulder 1's R1.0–R1.7) it never
> named, and classes P/A/B currently accept forged witness values because they are not
> Resolve-anchored. The flip is now R1.8, gated on that spine. Old Rock 4 (#406) folds in as
> R1.5. This flag preserves the earlier framing and records the correction.

1. **Trustless floor box** (D-TIERING keystone / "lane 1"). **IN PROGRESS — the recompute
   is built and merged; the accept-flip is the single remaining step.** The witness read-set
   producer (Part A, #656) and the additive floor-box v5 validation-mode scaffold (Part B1,
   #657) are merged; the box still holds `indeterminate-trustlessly` (never wrongly accepts).
   **R-boundary DECIDED (owner-ratified 2026-08-31, decisions.md):** the **heavy / fully-trustless**
   posture — the box reproduces every validity predicate, it does not re-derive the state root and
   trust finality. Whole-set committed reads are backed by MTH digest-root leaves; **≥4** carry a
   recompute reader (`bondedRoot`, `epochSetRoot`, `qualifiedRoot`, `validatorsSeenRoot`), and the
   committed v5 format emits all five (adding `slashedRoot`, F1). **The `apply()` transition set is
   reproduced and merged:** the O(payload) HYBRID recompute (payload/`dueBucket`-derived write-set +
   the R-fold over changed paths) covers the E/R spine and classes S/B/T/A/P plus the class-M
   maturity latch — O(payload)+O(registry), via a changed-digest write-set primitive. A
   **write-obligation ledger** (an emission-keyed differential guard) confirms **the committed-leaf
   diff a real `apply()` produces equals the key-set the recompute folds (28/28 committed leaf kinds)**,
   and self-detects a future added or renamed leaf. **Cost:** the witness bundle is measured **flat in
   the total state** — O(payload)+O(log N), not O(whole-state) — so the recompute fits the 1-CPU/2-GB
   "pony" budget. **Remaining: the accept-flip** — wire `WitnessValidateV5` to the merged recompute and
   return Accept-iff-all-predicates-pass. **Owner-ratified (pre-launch)**; gated on the #406
   model-check cert and the accept-flip gates. One step to a validating pony.
   Maps to: **Boulder 1** (the flip is R1.8, not a single step).
2. **Third-operator committed settlement** (the next PoD economic frontier). **DEFINITION.**
   Cross-operator settlement in committed state, replacing today's bilateral in-memory
   ledgers. Design-space strawman filed (#658, DEFINITION only); a **gated economic
   mechanism** (γ→1/N firewall #182 + conservation) under blind Research + PE evaluation.
   Couples to the v5 format. Maps to: the PoD frontier (Boulder 2 economy + Boulder 3 freeze);
   unchanged; attaches to R4.1.
3. **era-4 / v5 format freeze** (the closing act of Proof-of-Delivery; a second practiced
   era freeze). **DEFERRED BY DESIGN.** era-4/v5 is kept OPEN-ENDED — no live chain, and
   PoD may still reshape witnessable state (owner-ratified 2026-08-30). The R-boundary digest
   leaves are ratified and merged (F1, five roots); the hard gate is the COMPLETE exhaustive
   digest-root set — the mechanically-enumerated whole-set reads — plus any third-operator leaves,
   certified before the freeze (decisions.md). Maps to: **Boulder 3** (freeze re-scoped to RC).
4. **#406 consensus model-check (I1–I5).** **IN PROGRESS.** The deterministic adversarial
   property harness that HARD-GATES every graded field run; each invariant ablation-proven.
   Maps to: **Boulder 1 R1.5** (folded into the flip's model-check).
5. **Phase 5 — operational floor** (a node a person can run). **NOT STARTED (needs scoping).**
   Per-platform packaging + signed installers + operator-consented R4 self-update, plus the
   S6 scaling kills (incremental O(delta) proof maturation; reprovide dirty-tracking).
   Maps to: the operational-floor residual (Residual backlog; archived Phase 5).
6. **External red team (#183) → R1 field grade → V1.** **GATED (endgame).** Runs against
   the economy-ON config, then a green multi-region R1 grade, then V1. #183's close condition
   is MET and the issue carries the evidence; the close is the owner's call, deliberately held.
   Maps to: **Boulder 4**.

</details>

> *The retired numbered **Phases 1–6** block (Phases 1–3 COMPLETE; 4–6 the forward plan
> the Boulders replaced) and the dated **"Immediate next work (2026-08-26)"** priority
> snapshot lived here. Extracted 2026-09-01 to
> [`/archive/roadmap-history-2026-09-01.md`](archive/roadmap-history-2026-09-01.md). The
> Boulder spine above is the current packaging of the same board; still-live residuals
> they named (#559, #583, #574, #530, #586, #501/#500/#502, #506, #299) are carried in the
> Boulders and the **Residual backlog** at the end of this file.*

## Where we are now (the honest status)

- **Storage plane — sim-proven at scale, field-proven cross-network at small scale.** Cross-network publish/fetch,
  erasure-coded durability with failure-domain-aware placement + dispersion audit,
  capacity pledging/spill, mutual-TLS pinned identity, encrypted manifests +
  care-links, a quorum chain, web UI/observatory, desktop client. The silent-loss
  floors and the reprovide/config-drift gaps are fixed; scale (bit-perfect retrieval
  under churn) is proven in the deterministic in-process simulation, and
  cross-network hole-punching is proven through cone NAT in an automated Docker
  harness in CI. A warm multi-region cloud run has not yet graded a full suite
  end-to-end.
- **Trust plane — the M0 mechanism is BUILT and internally hardened.** The genuine
  composition shipped (PRs #117–#127): a verify-without-fetch proof-of-retrieval, a
  proof-of-**space-time** bond (an identity-bound sealed plot × a Wesolowski VDF,
  persisted, so N Sybils cost N real disks), standing as the time-integral of bond +
  audit, fork-choice reconciliation so partitions heal, and provable equivocation
  that slashes double-signers. The **H1–H6 systemic hardening pass is complete** —
  every standing/consensus/DHT/privacy surface has a shipped mechanism + inverted-PoC
  regression + the Invariant-A/B guardrails.
- **Durability is funded and verifiable — H7 is BUILT (#95).** The S7 spine ships: a
  per-object credit escrow, a **serve auto-skim** (popular data self-funds its repair),
  a rarest-shard bounty, and a **verified proof-of-correct-repair** — a bounty pays the
  new holder of a rebuilt shard only when correctness (Merkle recompute against the
  manifest-anchored survivors) *and* an identity-bound Shacham–Waters retrievability
  proof both verify, with an attributable false claim bond-slashed. It ships as an
  explicit **finite-but-renewable** contract with the funded horizon and **instrument
  `g`** measured, and the self-dealing red-team (garbage claim → slash, don't-store →
  deny, double-count → deny) is a permanent regression. *The plaintext-blind
  correctness commitment was found a GF(2⁸) dead end in pure Go, so M0 ships the
  Merkle-recompute floor and the bandwidth-blind upgrade is a documented fast-follow.*
- **The mission was reframed — the composition reset.** M0's Sybil corner is a
  **systemic claim held in tension**, not a Sybil-proof primitive (impossible by
  Douceur). It is stated as **C1 (no discount)** — forging a fraction *q* of standing
  costs ≈ *q*·`C_honest`, where `C_honest = disk × address-diversity × time ×
  served-demand` (non-substitutable) — **plus C2 (no quiet capture)** — the
  concentration metric keeps the minimum colluding *operator* set above *k*.
  Durability (S7) is **fused into the same budget** (one ledger). Full spec:
  [`docs/design/m0.md`](docs/design/m0.md).
- **The research commission has been answered.** The two constructions we'd routed
  to research both **exist**: center-less **proof-of-repair** (a composition of
  proven parts — unblocks durability) and a **non-globality metric** (ZK threshold
  predicate). Durability is decided **finite-but-renewable**, not "perpetual." The
  genuinely-hard residue collapsed to **one** named open problem — the shared-content
  sealing boundary (see the research frontier below). Decisions:
  [`docs/decisions.md`](docs/decisions.md).
- **The M0 build backlog is complete and externally re-attacked (2026-08-08→09).**
  All decided directions shipped: D-DEMAND P0–P3 (blind receipt + fee-burn +
  bonded-fetcher), H8 slices 1+2 (ephemeral-identity + relay-routed issuance), the
  CT-style transparency log (#180), the C2 metric wired from the committed ledger
  (#185), registry-only mode (#206), and the real-wire adversarial suite (#204). A
  second blind red-team found 2 shipped-default P0 gaps (not broken crypto); the
  certified remediation (F-1 `everMature` latch bundle, ε* discount close,
  SurvivorNakamoto, canonical signer set, refuse-to-start) is merged.
- **The consensus-correctness arc is closed as a class (2026-08-12→14, canon
  [`D-CONSENSUS`](docs/decisions.md)).** Four multi-region field runs each found an
  RC-blocking consensus bug — #357 (fork-choice oscillation), the B2 handoff
  head-count quorum, #397 (honest-proposer cross-attest), #402 (one-free-anchor
  fork). PE + research converged independently: **all four were one defect** — a
  finality quorum that did not intersect over its phase's real validator set. The
  closed invariant set is now canon
  ([`consensus-invariants.md`](docs/design/consensus-invariants.md), I1–I5), the
  deterministic **consensus model-check** (#406,
  [`consensus-model-check.md`](docs/design/consensus-model-check.md)) becomes the
  *first* consensus gate, and **every graded field run is gated on the model-check
  tier covering its regime** — a field run confirms; it never discovers an
  invariant. Fixes #357/B2/#397 are merged + field-confirmed; the certified #402
  fix (strict anchor majority `⌊A/2⌋+1`, anchor-only launch proposing) is the next
  build item.

- **The fresh-eyes audit (2026-08-19) verified the trust plane and found the economy
  dark.** Every claimed M0 mechanism verifies against code and tests (model-check
  genuine; all four adversarial wire drills pass over real TCP), and the M0 tail to
  #183 is small and enumerated. But the S7 repair economy — built and adversarially
  tested — is **default-off in every shipped node with no enable path**, `g` has never
  been measured live, bandwidth is unpriced, and the operational floor (packaging,
  O(store) cold-start, O(held) reprovide) prices out the honest operator in practice.
  Verdict: **M1 is the binding constraint.** The Boulder spine above is the response
  (D-M1-PIVOT — M1 interleaves with the M0 tail; the Boulders are the current packaging
  of that ordered board); full findings in
  [`docs/thinking/2026-08-19-fresh-eyes-audit-and-the-m1-pivot.md`](docs/thinking/2026-08-19-fresh-eyes-audit-and-the-m1-pivot.md).

## Verifying M0 is held (the Boulder 4 endgame frontier)

Two kinds of work carry the endgame: **verify** is the gate to declaring M0 *held* (not
merely built); **research frontier** is the handful of items that need a new result, not a
decision. Both feed Boulder 4 and the external red team (#183).

> *The retired **Build tracks** list (H8/H9 privacy+takedown, D-DEMAND/C2/registry status —
> a third top-level organizing scheme) was extracted 2026-09-01 to
> [`/archive/roadmap-history-2026-09-01.md`](archive/roadmap-history-2026-09-01.md). Its
> still-live items (H8 #179, H9 #180, post-launch registry #207) are carried in the
> **Residual backlog** below.*

### Verify tracks — the gate to "M0 held"

M0 is *held* only when an **external** party attacks the built composition and it
survives at declared parameters. Self-graded does not count.

**The certified sequence to the gate (D-CONSENSUS, 2026-08-14):** build the
certified #402 fix → **consensus model-check launch tier green** (#406, with the
#357/#397/#402 failing-first replays) → the P1 all-corners field run → model-check
handoff tier + the #399 WS-recovery drill → the MATURING=1 run (field-cert of the
#389 weight-quorum handoff) → model-check full budget + the red-team entry
criteria ([`release-checklist.md`](docs/release-checklist.md)) → **external red
team (#183)**.

- **Consensus model-check (#406).** The deterministic I1–I5 property harness — the
  first consensus gate, and the gate on every graded field run (tier order:
  unit → model-check → sim → netem → field).
- **Multi-machine field test (R1, #52).** Bonds, tokens, and consensus across real
  machines and real NAT — the trust plane earning the rigor the storage plane has.
  Confirms on real WAN what the model-check proved; grades liveness, which only
  the field can.
- **External red-team vs C1/C2.** A fresh, no-memory adversary attacks the
  *systemic* claim and the seven composition **seams** ([`m0.md`](docs/design/m0.md)
  §7), not isolated primitives — a primitive failing a standalone "Sybil-proof" test
  is Douceur, expected, not an M0 failure. A seam *held in tension* (bounded cost,
  documented residual) is a pass; a seam silently assumed closed is the failure mode.

### Research frontier — needs a new result, not a decision

- **⭐ The shared-content sealing boundary — the one surviving economy of scale.**
  Plain PoR over *shared* erasure-coded shards lets one physical copy answer for N
  pledges (γ→1/N); closed only by **identity-keyed PoRep sealing of arbitrary useful
  shared data**, which is not yet publicly-verifiable + timing-free +
  trusted-setup-free. **silt is not exposed today** — standing comes from a dedicated
  identity-keyed bond plot, not the shared shards — but *fusing* served content into
  standing without leaking γ→1/N is the highest-leverage open question. An
  academic-collaborator task (`m0.md` §10).
- **MSR / regenerating-code proof-of-repair.** The A1 composition is airtight for
  plain-RS reconstruction; no published construction specializes it to MSR/Clay. Off
  today's critical path (silt ships plain-RS).
- **Byzantine size-estimation under adversarial NodeID placement.** The C2 sampling
  tolerance is proven for *random* Byzantine placement; a stake-splitter chooses its
  NodeIDs, degrading it by an amount the literature does not quantify.

## What "M0 held" means

M0's Sybil corner is not a primitive to be proven Sybil-proof — that is impossible.
It is **held** when, at the network's declared parameters, no strategy earns
consensus-controlling standing for less than `q · C_honest` (**C1**), and the
concentration metric keeps the minimum colluding operator set above *k* (**C2**),
with the §7 seams either closed or *held in tension with a documented, bounded
residual*. C1 is a **theorem *under* the B5 hypotheses H1–H3** (a direct-product
bound) — *conditional*, not unconditional: the per-identity Alwen–Blocki lift is
unproven, the shared-content H3 gap (γ→1/N, #182) is open, and in shipped code only
the bond (D) axis gates standing so far (§ "where we are"). C2 is a **measurement
bounded by an impossibility result** (Kwon) — held, not closed, by design. The
verdict is rendered by the external red-team + the field test, together.

## Residual backlog (tracked here, not on the Boulder critical path)

These are the off-critical-path residuals migrated from the (retiring) GitHub issue tracker.
They do NOT gate the Boulder spine; they are the honest tail. Each carries its provenance
issue number as an anchor only. Repro recipes for the field defects (#558/#535/#530/#574/#586/#277)
live in [`docs/thinking/2026-09-01-residual-defect-repro-recipes.md`](docs/thinking/2026-09-01-residual-defect-repro-recipes.md).

**Security / data-safety residuals:**
- **Transport authentication (TLS or Noise) — #437.** silt's wire is unauthenticated CBOR
  (`adapters/tcpnet/wire.go`), so an on-path MITM can strip certificate signatures in transit.
  Certified NOT a safety break and NOT a wedge (stripped blocks fail `ValidateCommit`, are inert,
  never drive round-advance; only verifying sigs count; ≥⌊A/2⌋+1 full-certificate holders re-serve
  over any honest path; residual = censorship over a controlled link, already tolerated by the
  liveness model). Work: wire authentication/integrity at the transport layer — a pre-existing
  transport property, orthogonal to consensus. Consult `docs/network-durability.md` before design
  (build-immutable #5); research-gate the crypto choice. Cert:
  `432-proposer-prepare-required-RESEARCH-CERTIFICATION-2026-08-16.md` §5.2.
- **Crash-safety: torn `chain.cbor` → silent genesis fallback — #558.** A SIGKILL mid-persist
  can tear the `chain.cbor` write; replay hits the damaged region and the daemon silently discards
  finalized history back to genesis (the markstore is atomic; the chain store is not). Silent loss
  on a common failure (OOM/power). Fix direction + RED home in the repro doc. **Real silent-loss
  residual** (build-immutable: no-silent-loss floor).
- **On-disk format migration policy — #237.** Upgrading a daemon onto a pre-format store is
  silently destructive: the chain is rejected (`unsupported block version`) and silently reseeds
  with only a one-line stderr notice; content is stranded. **Needs a POLICY call before any release
  that crosses a format boundary:** (a) a migration path (read old store, upgrade proofs/chain), or
  (b) refuse to start with a clear message (safe default). Relates to #70 (proof migration) / #98
  (chain format).
- **Silent-behavior observability — #235.** Three silent behaviors reduce field assertability:
  (1) a healthy repair sweep emits no log line at any level ("is the caretaker working?" is
  unanswerable until something breaks) — add `repair: sweep complete, all stripes healthy` and/or
  per-repair counts; (2) `-revoke <bogus-root>` leaves the daemon silently inert — add `revoke:
  root not yet committed, waiting` or an explicit refusal; (3) rolling upgrade across a format
  boundary degrades with only a one-line stderr notice (ties to #237). Partly overlaps #237.

**Test / harness debt:**
- **Cited-test lint + the OWED ledger it opened — PR #708 (lint) and PR #707 (first two payments).**
  `scripts/check_cited_tests.py` fails the build when a `TestXxx` cited in a Go comment, `CHANGELOG.md`,
  `ROADMAP.md` or `docs/**` resolves to no `func TestX(` in the repo — the
  `scar:cited-test-does-not-exist-2026-09-02` class, where a phantom laundered from a production
  comment into a research certification. In-repo citations are STRICT; external review trees are
  ADVISORY (they may legitimately cite a test on an unmerged branch), and `.claude/` is excluded —
  load-bearing, because those worktrees are copies of OTHER branches and scanning them would let a
  phantom resolve. PR #707 wrote the first two cited-but-missing consensus guards (the v5 root
  coverage guard and the era-4 write-path guard) and closed the same worktree-walk unsoundness in
  `scripts/check_claims.py` (measured: 121 test names resolvable only in `.claude/worktrees/`).
  **Open:** the rest of the OWED ledger.
- **Test-honesty audit — #303.** 27 adversarially-verified test-honesty issues + 9 product
  findings from a per-harness audit (65 agents) of all 18 integration harnesses against the 5
  field-test immutables — each a way a harness could go GREEN on a broken product (e.g. the
  redteam SCENARIO 2/3 assertions key off a non-specific `!resp.OK`, so a dead/blanket-rejecting
  H3 goes green; fix: add an H3 positive control). Open QA debt; overlaps but is not fully covered
  by the Boulder-1 decoration-oracle gates.
- **Harness reachability + flow-overlap (#574), Docker pre-genesis stall (#530), skim-observer
  arming (#586).** Standing-tail harness defects; repro recipes + fix directions in the repro doc.
  #530 first step is instrumentation (`-log debug`, capture one full client transcript), not a fix
  (build-immutable #7).
- **CPU-time O(depth) regression CI gate — #616.** The shipped O(depth) gate (#613) measures
  baseline-subtracted `runtime.MemStats.HeapObjects`, so it catches allocation-shaped depth
  blow-ups (the #555 `AllEntries` shape) but NOT CPU-time-shaped ones (the #528 per-height CPU
  burn allocates little, so it slips the memory gate). Needed: a companion wall-time/CPU-time
  slope-vs-depth gate analogous to the memory gate's two-stage doubling test. The blocker is
  noise — wall-time is far noisier than a seeded byte-deterministic `HeapObjects` count, so this
  needs its own **noise study first**: measure run-to-run variance of the per-height time cost,
  derive a bound from that measurement (not a guessed constant), and prove it goes RED on an
  injected CPU-time O(depth) defect before it can assert. Companions the Boulder-1/Boulder-3 test
  gates. PE ruling: `RULING-613-odepth-ci-gate-2026-08-28.md` §B; deliberation
  `docs/thinking/2026-08-27-o-depth-ci-gate.md` (lines 183–188).
- **cloudtest infra-liveness-FAIL journal-capture gap — #504.** A distinct evidence-capture
  defect. The flow-level capture-on-fail exists (`flow-evidence-*.log`), but the `infra-node-liveness`
  row's FAIL path has NO capture step: on run fa501cc-56689 it named `island-c×3` crashes then let
  the EXIT-trap teardown destroy them, losing crash-type attribution (OOM vs Go fatal), crash times,
  and the pre-crash log tail — the exact build-immutable #7 canonical loss that ratified
  capture-the-evidence-first (PR #394). Fix (third-time rule — a gate, not prose): when
  `infra-node-liveness` records a FAIL, pull each named node's `journalctl -u silt` (all boots) +
  `dmesg | tail` into `failed-nodes-<run>.log` BEFORE returning, the same capture path the flow
  failures use. Fires only on the FAIL branch (cheap).

**Field-test harness residuals (folded from the retired `integration/FIELD-TEST-ROADMAP.md`, 2026-09-01):**
The RC field-test gate is MET (RC run `585c82a-58990` graded 28 pass / 0 gap / 0 fail /
2 skip-by-design, #532 `eb57d50`; deep lineage `fe2376a`-deep 30P/1G/0F). What remains is
harness truthfulness/coverage/parity hardening — none gates the Boulder spine. The full
list and per-item fix directions live in
[`archive/FIELD-TEST-ROADMAP-2026-09-01.md`](archive/FIELD-TEST-ROADMAP-2026-09-01.md);
the load-bearing still-live items:
- **Harness truthfulness hardening — tracked under #303** (the test-honesty audit). Each
  is a way a green harness could hide a broken property: the consensus P0 negative control
  needs a real quorum + a positive control; refusal reasons should be read from the daemon
  log, not client stdout; `soak` memory-growth and `churn` seeded-placement gates need
  falsifiable oracles; `bond` C1 must assert reputation ∝ bond (two bond sizes, ratio
  roughly linear); `nat` hole-punch should assert the direct path bypassed the relay;
  `redteam` should cross-check the honest target's head height is unchanged. `upgrade`
  chain-reload (CHAIN_OK positive height) is DONE.
- **Demand field test (#264, above).** `integration/demand` becomes real only once the
  demand P2/P3 seam is wired into the daemon fetch path — the same #264 residual listed
  under Durability/repair/demand below.
- **`chaos` WAVE-2 redundant-bootstrap survival — root-cause open.** Does a redundant
  (≥2 seed) bootstrap survive one crashing? Pin it, then fix + assert or document the
  single-bootstrap topology limit.
- **#281 empty-routing-table self-heal wire-certification.** Fixed in-product
  (`Node.StartBootstrapRetry`) but no cloud flow disables the startup TCP-wait to exercise
  the real `re-bootstrapped: recovered from an empty routing table` path over the wire.
- **GCP substrate operability — RC gate MET; quota/preflight hardening still worthwhile.**
  The full 13-node run is completed and graded (the RC sheet above). Separately: a
  full-topology run is still blocked by two ENVIRONMENTAL constraints (not product bugs) —
  a `us-central1-a` E2 capacity shortage and the default `IN_USE_ADDRESSES` = 8/region
  quota (~11 external IPs needed). Worth doing: a pre-flight that checks IP headroom + zone
  capacity before `apply`; shrink the public-IP footprint (IAP-only/bastion) so a
  single-zone full run fits the default quota; make `nuke` sweep the leaked
  VPC/subnets/firewall/routes by label and stop swallowing `terraform destroy` stderr.
- **Per-substrate parity + GCP-only scenarios (parity).** Factor the shared
  `exec-on-node`/`assert-on-log` node abstraction so one scenario targets either substrate,
  then add scale-out churn (50+ nodes), a real firewall partition, `tc` link-shaping, and
  long-haul soak. An **AWS variant + two-cloud** field test is the far end (a fallback
  substrate when GCP capacity/quota blocks, then a GCP+AWS split for real inter-provider WAN).

**Durability / repair / demand residuals:**
- **Repair dial-storm to dead holders — #277.** The DHT walk re-dials dead holders every sweep
  (the `deadUntil` negative cache is consulted on the fetch/repair decision path but not on the
  walk's dials), so under heavy permanent loss a sweep can't finish though ≥k shards survive.
  Scale-independent behavior; part of the pre-gate repair-sweep family (#501 unbounded sweep / #500
  fetch-retained copies never announce / #502 restart orphans the working set). Repro in the doc.
- **Wire demand P2/P3 into the daemon fetch path — #264.** `core/demand` P2 fair-exchange floor
  and P3 cost-to-wash (fee-burn + bonded-fetcher credential) are real and unit-tested but have no
  live daemon-wire seam (no `silt sim run demand` scenario, no CLI flag, no serve/fetch-path
  enforcement), so a real field test can't cynically exercise them. Wire one/both (a sim scenario
  and/or daemon-level demand enforcement), then add `integration/demand` asserting the outcome:
  a freeloader can't fetch without paying; faking demand costs one bonded identity per unit + a real
  fee. Phase-4 (PoD) detail; the firewall (delivery credits never fund standing, γ→1/N #182) is
  immutable.
- **Bond-proof reply size / N² cost — #299.** The encoded bond-challenge answer is ~1.5 MB,
  near-flat in bond size (dominated by 64 label opens), so proofs are loss-sensitive over lossy TCP
  and cost N²×1.5 MB per audit interval as the validator set grows. The sound fixes are structural
  (SNARK-wrapped succinct proof → H-track; fewer samples = a soundness tradeoff, Evolving-tier,
  research-gated; FEC needs QUIC first). An **owned, named residual**, not a stopgap. ROADMAP R3.3
  keys the PayWord/RegCap re-derivation to "only if #299 moves" — this is where #299's measured
  numbers live. Possible cheap interim (unverified): de-duplicate shared DRSample parent blocks in
  the encoding (no soundness change) — measure before any k-reduction.

**Polish & latent wins (low-priority — folded from the retired `BACKLOG.md`, 2026-09-01):**
These are small, still-open captured ideas — polish and opportunistic wins, not RC-critical.
They carry no provenance issue (they never merited one); they gate nothing on the Boulder
lattice. When one matures, promote it to the section it belongs in.

- **Demand-responsive dispersion — pull half (storage placement).** The push half ships (a hot
  holder leases cache copies away from its own failure domain — the dispersion re-spread in
  `core/node/repair.go`). Still open: let a node that had to *fetch* a chunk under load
  opportunistically cache and announce it, decaying when unused — so hot copies also gravitate
  *toward* readers, not just away from hot holders. Kin to the repair-sweep residual #500
  (fetch-retained copies never announce) above; a shared fix could subsume both.
- **Domain-aware placement gaps (storage placement).** Query a candidate's failure domain when
  gossip hasn't reached it yet (today placement spreads only across *learned* peer domains);
  domain-aware capacity spill. Column placement will subsume the per-stripe anti-affinity repair
  path later.
- **Direct IPv6 dial before assuming a relay (networking latent win).** Try a direct IPv6 dial
  before falling back to relayed transport — a cheap latent win before the relay fallback.
- **Relay selection + failover (networking latent win).** A NATed node that discovers relays by
  gossip currently adopts the lowest-ID one and commits to it; if the chosen relay won't
  register, it retries that one forever instead of failing over. Fine while a swarm has one dev
  relay; wants selection + failover once community relays are plural.
- **`docs/` staleness enforcement (observability polish).** The `Docs ship with code` CI job
  (`.github/workflows/ci.yml`) already fails a PR that touches `cmd/`/`core/`/`adapters/`
  without a `CHANGELOG.md` update. Extending the same staleness enforcement to `docs/` is a
  possible later tightening.
- **e2e relay-in-the-middle variant (test/harness polish).** A relay-in-the-middle variant of
  the multi-process e2e suite. (The kill-a-node erasure-resilience variant it was captured
  alongside has since shipped as `e2e/economy_repair_test.go`.)
- **Capacity/scaling shape test — deferred (test/harness polish).** The 3 GB shape test —
  30×100 MB vs 300×10 MB — to characterize manifest/DHT overhead vs chunk-count. Deferred while
  the dev box is RAM-bound.

**Consensus-touching residuals (tracked, NOT resolved — need the research gate when worked):**
- **Objective-mode `Config.Quorum` floor divergence — #380 · tracked, NOT resolved —
  consensus-touching (I1).** In objective mode the effective `ValidateCommit` requirement is
  `max(Quorum, bftThreshold(validatorSetSize))` (`core/chain/chain.go` `RequiredQuorum`). `bftThreshold`
  is a pure function of committed chain state (replica-identical), but the `Quorum` floor is LOCAL
  config, so two honest replicas with different `-quorum` compute different validity for the SAME
  committed block; a node whose floor exceeds a committed block's attestation count rejects it in
  `Reconcile` and is stranded at genesis. Confirmed root cause of the SYBILS=8 field GAP (#338).
  **Workaround shipped:** a uniform `-quorum` across the objective swarm (documented behavior,
  `TestDivergentQuorumFloorStrandsSyncingNode338` passes today). **The product question is a
  consensus-rule change (I1) and needs the research/owner gate before any build** (build-immutable
  #6): should objective mode ignore the local `Quorum` floor in `ValidateCommit` and defer to
  `bftThreshold`, keeping `Quorum` as a proposer-side gather target only? Options: (1) ignore the
  floor in `ValidateCommit`; (2) a cheap non-gated startup warning when `-quorum` is set on an
  objective+byzantine node; (3) leave as-is, documented. Do NOT assert resolved.
- **Mature-regime PUBLISH starvation — #441 · tracked, NOT resolved — consensus-touching
  (I4 liveness).** Field-observed (run a56ac10-42834): entry-carrying blocks committed only
  pre-latch (h4–h23, latch at h26); NONE of the 52 post-latch blocks carries a publish entry,
  though the chain stays live via renewal drains. Consensus SAFETY unaffected (S1/S2/I1–I5 held);
  this is I4's *liveness* face for the ENTRY path. Leading (code-cited, NOT proven) mechanism: the
  #432 view-change machinery is drain-only — `recordRoundChange` (`core/node/rounds.go`) fires the
  drain proposal, the publish path (`httpregistry publish` → `proposeBlock`) has no round-aware
  retry or new-view seat, so at mature steady state pending renewals own every height's rounds and
  the publish loses every round. **A fix touches the certified #432 round machinery, so it needs a
  research/PE consult BEFORE any build** (no unilateral change). Deterministic home: a repro
  schedule in `matureWorld` (PR #440). Attribution: `docs/thinking/2026-08-16-run3-mature-handoff-passed-publish-starvation-found.md`.
  Do NOT assert resolved.

**Operational floor (RC-path, needs scoping — the archived Phase 5):**
- **A node a person can run.** Per-platform service packaging (launchd / systemd / Windows
  service) + signed installers, and operator-consented self-update per R4 (signed manifests,
  never silent). Plus the S6 scaling kills: incremental O(delta) proof maturation (kill the
  O(store) restart scan) and reprovide dirty-tracking (kill the O(held) per-interval re-sign).
  Exit gate: a non-developer installs a node that survives reboot and returns to serving in
  seconds, and steady-state cost no longer scales with the whole held set. NOT STARTED.

**Post-launch (explicitly not V1):**
- **Registry liveness-pruning + federation — #207.** The read-cost-bounding lever of registry
  economics shipped for M0 (per-IP rate limit + timeouts, #206). The remaining cheapness levers —
  liveness-pruning of dead entries (needs a provider-liveness probe; inapplicable to the chain-backed
  append-only registry) and federation/sharding of a large public registry — are post-launch scaling.
  These do NOT gate M0.

**Umbrella/frontier issues kept as evidence anchors (not a second task list):** #183 (external
red team → **Boulder 4 R4.4, the M0 close gate**; close condition MET, owner deliberately holds), #182 (shared-content
sealing frontier → R4.1 / research frontier above), #179 (H8 metadata privacy), #180 (H9 pluralistic
takedown), #94 / #52 (the forward-tracks / R1 verify epics — now the Boulder structure itself),
#406 (consensus model-check = Boulder 1 R1.5).

## The resolver layer ("Aslan" — separate product)

Meaning lives above the infrastructure, in a separate codebase: name/description/
tags → (root, manifest key). Silt ships zero Aslan code, ever. See
`docs/aslan-boundary.md`.

## Release engineering — the march to V1

The `0.1.x` / `0.2.x` tags were **experimental / learning releases**, not steps
toward V1 — treat them as archaeology. The real cadence has three stages:

- **Learning phase (past).** Everything through the experimental 0.x tags:
  proving the architecture. Detail lives in `docs/buildlog/`.
- **Feature-complete → `0.9.0`.** When every V1 build track's *mechanism* is built
  (the floors plus the M0 composition, durability, privacy, takedown) we cut
  `0.9.0` as the release-candidate line and harden it in the field.
- **`1.0.0` = V1.** Cut only once the tenets are field-proven multi-machine (R1)
  **and** the external red-team verdict holds — a *true* release candidate,
  signed/notarized/checksummed. This is the first release we stand behind publicly.

Mechanics when ready: move CHANGELOG "Unreleased" into the version, tag, and the
release workflow builds + publishes binaries; add code-signing/notarization
(macOS) + a checksums file first. See `docs/release-checklist.md`; website/DNS in
`DEPLOYMENT.md`.
