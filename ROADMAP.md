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
> **critical path to the Release Candidate** and the **Boulder status ledger** below — is the SINGLE
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

### The Release Candidate — what it is, and the critical path to it (trued up 2026-09-07)

**The RC is ONE release: the era-4/v5 stamp-raising release, with the consensus format FROZEN,
the floor box still never-Accept, and the economy DEFAULT-OFF with every paid lane built (`D-RC-SCOPE-S1`).** The external B8 pass (R1.7 = R4.4, one engagement) attacks that
frozen artifact; the accept-flip (R1.8) lands after it; `1.0.0` follows a green multi-machine field
grade of the same artifact. Every item below is one of: a **precondition of the freeze**, a
**deliverable of the same stamp-raising release**, or an **RC gate that runs against it**. Ratified
sequencing that still governs: nothing turns the economy on over a live mint (Boulder 0 is DONE);
internal flip preconditions → freeze at the RC → B8 on the frozen artifact → the flip (2026-09-03);
a graded field run is gated on the model-check tier covering its regime — a field run confirms, it
never discovers (`docs/build-process.md`).

**How to read the tables.** `Status` is trued to main `e963034` (2026-09-07). `What is left` is the
one sentence a builder acts on without a design session; **OWNER** marks a sentence the owner
ratifies; a seat name marks the artifact that seat owes. Rows are ordered within a lane; a row
depends on the rows above it unless it says otherwise. The per-Rock argument, every source path and
every owner call as it was worded live verbatim in
[`/archive/roadmap-boulders-detail-2026-09-07.md`](archive/roadmap-boulders-detail-2026-09-07.md);
this file carries what is LEFT. The definition program that produced the 2026-09-07 rows is
[`docs/thinking/2026-09-07-roadmap-true-up-and-definition-program.md`](docs/thinking/2026-09-07-roadmap-true-up-and-definition-program.md).

#### Lane A — consensus liveness (opened 2026-09-07; blocks every further graded field run)

| # | Item | Status | What is left | Gate / seat |
|---|---|---|---|---|
| A1 | `R-H43-ROUND-LADDER-DESYNC` — under f=1 down, per-validator #432 round ladders drifted ~77 s apart and block 43 took 17 min 20 s (run `c450985-deep`; evidence `integration/cloudtest/h43-stall-evidence-c450985-deep/`) | field-discovered; **REPRODUCED DETERMINISTICALLY 2026-09-07 — G-H43-1 is RED on main** (`core/node/modelcheck_h43_arming_test.go`, branch `tester/h43-round-ladder-desync-modelcheck` @ `8b1e1ef`, unmerged, test-only): 3 of 12 seats armed, 8 quiescent, one heavy seat killed and returned mid-ladder, real timed delivery — no commit within the certified f=1 bound of 190 s; every seat sits at round 2 at the deadline and no proposal reaches the wire. The first attempt was GREEN because the fixture's own constructor calls `refill()` and arms every seat (fixture-blindness, a THIRD layer under the two the certification named); G-H43-6 (the fixture-blindness pin) is GREEN and the Tester's third-time rule FIRED on the fixture-green-on-wrong-arm family (count 3) | **ATTRIBUTED 2026-09-07** (`/Users/andrewedmond/Claude/claude/silt-reviews/research/research-outcome/CONSENSUS-LIVENESS-h43-round-ladder-desync-441-380-RESEARCH-CERTIFICATION-2026-09-07.md`): timer-skew smear (the #451 shape) REFUTED — the root is that `core/node/rounds.go:306-310` arms the round clock on LOCAL mempool content, so the round number is a function of unreplicated private state (3 of 13 seats ran the pacemaker; 10 sat at r0 for ten minutes, `sybil-3.log:32`, `maturer-2.log:31`); M1b the quiescent branch ZEROES `rs.Sweeps` (why val-b stopped after r3); M2 (GATED) `proposeBlock` re-derives the round from `rs.Round` (`chainrole.go:1040-1047`), so the (43, r1) designee assembled `Changes[3]` and bailed; M3 `Changes[r]` is a point record, so the catch-up target is structurally the LOWEST quorum-bearing round. The model-check was GREEN because `refill()` arms every seat uniformly (fixture-blind; the #560 GREEN is the same blindness). **Fix direction CERTIFIED:** (A) arm on a REPLICATED condition (pending work OR any consensus message for the working height — PBFT arming, Tendermint L21); (B) suffix-semantics round-changes + a relayable round certificate (DiemBFT TC); (C) the designee proposes at the certificate's round; I1 holds via `slotCompare`'s round term, never via the arming rule. Published bound: **≤ f+1 rounds = 190 s ≈ 4.3 × `T_b` at f=1** (observed 996 s = 22.6 ×). The harness's 1445 s cap is REFUTED (never measured — `fp0` empty, `scenarios.sh:526-541` — and it priced the defect). What is left: merge the Tester's branch (G-H43-1 RED, G-H43-6 GREEN) as the first commit of the fix PR; Tester encodes G-H43-2…5, 7, 8 RED-first; Builder lands (A)+(B)+(C) in ONE PR that turns G-H43-1 GREEN; **RATIFIED 2026-09-07 (`D-CONSENSUS-ARMING`)**: the arming rule, the certificate, the pass-through, the published bound, the re-priced FT tiers (190 / 380 s), harness honesty + `-log debug` on graded runs, and no graded run until G-H43-1 is GREEN on the fix | **BUILT 2026-09-07 on `builder/h43-consensus-arming` (PR pending):** (A)(B)(C) plus (D), the closer the model-check exposed once (A)(B)(C) existed — `R-H43-WORKLESS-DESIGNEE`: a LIVE designee holding none of the height's work wastes its round exactly like a down one (the empty-block validity rule); certified by the blind delta cert (`/Users/andrewedmond/Claude/claude/silt-reviews/research/research-outcome/CONSENSUS-LIVENESS-h43-ABC-ASBUILT-workless-designee-RESEARCH-CERTIFICATION-2026-09-07.md`): entries are forwarded to the round's designee (capped at 4; registrations REFUTED — owner-submitted only), the #338 takeover is re-keyed to the round's designee (the designee has PRIORITY, never exclusivity — corrects the #441 claim), and three cost gates on the certificate (designee-directed send, per-sender budget + envelope cap, one attempt per (h, r)). **Owner calls 21–23 below.** Gates G-H43-1…6, 9, 10, 10a, 11–15 GREEN on the fix (each RED-first); blind PE ruled NOT MERGEABLE at `c2a476a` on the evidence layer (`/Users/andrewedmond/Claude/claude/silt-reviews/principle-engineer/RULING-h43-consensus-arming-c2a476a-2026-09-07.md`) — every code finding fixed, G-H43-1 re-shaped to pin (A); the composed re-cert (`/Users/andrewedmond/Claude/claude/silt-reviews/research/research-outcome/CONSENSUS-LIVENESS-h43-COMPOSED-DIFF-c2a476a-RESEARCH-CERTIFICATION-2026-09-07.md`) certified (A)(B)(C)(D1)(D3) and I1–I5, its one blocker (`R-H43-CERT-ROUND-ZERO-UNVERIFIED`) fixed. Owner calls 21–23 RATIFIED 2026-09-07 (`D-H43-WORKLESS-DESIGNEE`); merged as PR #772. Left for A3: the graded run on the owner's go. `R-H43-NULL-PROPOSAL` (PBFT's null request) routed to era 5 |
| A2 | `#441` mature-regime PUBLISH starvation (I4, entry path) and `#380` objective-mode `Config.Quorum` floor divergence (I1) — the two tracked consensus-touching residuals | CERTIFIED 2026-09-07 (same certification): **#441 SHARES A1's root** — `rounds.go:306` IS the shipped #441 arming clause, one closer (A); the old #441 row is retired (it contradicted `consensus-invariants.md:113-132`). **#380 is SEPARATE but composes**: `RequiredQuorum()` returns `cfg.Quorum` verbatim in the mature regime and `SupportMeetsQuorum` gates `newViewFor`, so a divergent `-quorum` is a LIVENESS defect (a permanently dead designee), not only the `Reconcile` stranding | #441: closed by A1's fix, gate G-H43-3/-4; #380: direction (1) — ignore the local floor in `ValidateCommit`, keep `Quorum` as a proposer-side gather target — a separate GATED item with gate G-H43-8; RATIFIED 2026-09-07 (`D-CONSENSUS-ARMING`) | Researcher (done); Tester G-H43-8; Builder |
| A3 | The next graded GCP deep run (the RC field-test gate needs a green pass on the frozen artifact) | **DONE 2026-09-07 — `2633a11-deep`: 30 pass / 2 gap / 0 fail / 3 skip** (`integration/cloudtest/report-2633a11-deep.md`). The fix is FIELD-CONFIRMED: 39 s/height on the deep drive (44 before), the entry forward + designee propose observed at h29 (1.3 s), h30→h40 with val-d down at ~50 s/height. `6-fault-tolerance` GAPped on a harness artifact (the wait read the boot node's commit banner, which catch-up never prints) — fixed in #774; `184-low-bond` is the known #350 gap | the next graded run (after Lane D's freeze, or sooner on the owner's go) grades `6-fault-tolerance` at 190 / 380 s with the head-reading wait | Tester; owner's go (billable) |

#### Lane B — Boulder 1: the floor box (the accept-flip's preconditions)

| # | Item | Status | What is left | Gate / seat |
|---|---|---|---|---|
| B1 | `R-STRUCTURE-REDERIVATION` — ONE accept composition `ValidateCommitV5(view, block)` over a three-valued `StateView`, all box doors unexported, `box.Accept ⇒ node.Accept` | **Round 1A MERGED 2026-09-08** (PR pending merge at write time): ONE composition `ValidateCommitV5(view, block)` = `ValidateProposalV5` + C1…C5, dispatched from BOTH `ValidateProposal` and `ValidateCommit` on version (chain.go = two dispatch hunks + one comment); `StateView` sealed, `HeadRef` five fields, the zero `Budget` stalls; 8 doors unexported; the stage-cover gate (arms D/A/B/C) + G-D13 per-row node-body digests + the nine-regime v4/v5 parity oracle; honest twins on all 26 gates; blind PE (245-case differential, 13 ablations) MERGEABLE-WITH-CHANGES → closed; Researcher CERTIFIED with five conditions → closed. Prior status: RATIFIED 2026-09-03; build plan CERTIFIED (`FLOORBOX-STRUCTURE-BUILD-PLAN-CERTIFICATION-2026-09-03.md`, BG-1…BG-4). **Readiness ruled 2026-09-07 (blind PE, `RULING-structure-rederivation-build-readiness-e963034-2026-09-07.md`): NOT buildable as written in one session.** The round is largely BUILT on the unmerged branch `builder/floorbox-structure` (`2ca01d9`, 81 files) which conflicts with main in 19 files (`chain.go` content, `carrier.go` add/add) — re-apply file-by-file, never rebase; the composition's stage table DRIFTED (main's `ValidateProposal` runs `validateIssuerKeys`, v5-only, and P5 gained an `IssuerKeys` clause — neither in the plan); the plan never addresses `R-LOGROOT-FORMAT-SCOPE` (`HeadRef` has no `LogRoot` field); item 8 (MG-C strip) is superseded by #723; `BoxQuorumSupport` no longer exists; the `IngestBlockWitnesses` zero-caller fact has no gate | **Round 1B** — the box-entry-dependent closers (`R-DRIVER-ASSERTED-BOXSTATE` N2/N3/N6/N7, `R-GATE-PINS-BYPASS`, `R-PIN-*`, `R-BOX-EXACTNESS-CITED`) after `builder/floorbox-box-entry` is re-applied file-by-file (never rebased); `R-VIEW-FAITHFULNESS` to the red-team before the flip; `R-WITNESS-RESIDENT-HEAP` before a witness server ships. Prior: **(i) DONE 2026-09-07** — the stage table is RE-CERTIFIED on e963034 (`/Users/andrewedmond/Claude/claude/silt-reviews/research/research-outcome/FLOORBOX-STRUCTURE-P-TABLE-DRIFT-DELTA-CERTIFICATION-e963034-2026-09-07.md`): 24 stages (was 22) — P8b `validateIssuerKeys` (v5-only; reads `cfg.EpochBlocks`, `cfg.MinBond`, `bonded[IssuerID]`), P5's sixth clause, and **P13b the `LogRoot` conjunct the 2026-09-03 plan missed entirely** (its absence is a wrong-accept on the cheapest mutation: the box checks only the state root); `HeadRef` = `{Hash, NextHeight, ProposerID, *StateRoot, *LogRoot, Empty}`, zero format change; the `LogRoot` predicate is plain equality at k = 0 and STALLS at k ≥ 1 — and a witness-supplied `m` is a WRONG-ACCEPT (`translog.go:215,222-234`), so `tagRevLogSize` is a SAFETY leaf (agrees with the freeze manifest); a four-arm stage-cover gate (v5 non-vacuity first; AST call-cover vs a `nodeStages` table; body-hash change detection; reverse cover — 14 of 24 derived, 10 change-detected); MG-C do not build; the `chain.go:736` claim REFUTED (every parent `Att` is a valid child carrier entry — a two-sided gate owed). **The 1A/1B split is CERTIFIED as sequencing, but §6.4's identity claim is NOT preserved as certified** until M-1 (the legacy `rep` leg — the S2 fence is box-only and legacy mode ships, `daemon.go:799`) and M-2 (`ValidateProposal` must dispatch to the composition too, or it stays a second v5 implementation) are closed; **(ii) Round 1A** — the main-only spine per the PE's 12-step brief (an AST-derived stage-cover gate FIRST; a v5 fixture for every equivalence gate; BG-3 as a byte budget the box OWNS, never a constructor parameter); **(iii) Round 1B** — the box-entry-dependent items after the HELD round-A branch merges (owner call 16 **DECIDED 2026-09-07**: main-only first, `D-TRUE-UP-CALLS-2026-09-07`). Round 1A closes by construction `R-CARRIER-PARENT-BINDING`, `R-CARRIER-PARENTPROPOSER`, `R-LOGROOT-FORMAT-SCOPE`, `R-BOXENTRY-RESIDUALS` (N1–N8), `R-SUPPORT-SLASHED-SCREEN`, `R-MALFORMED-DIVERGENCE`, `R-PIN-LABEL-ESCAPE`, `R-PIN-REANCHOR-POSITION`, `R-PIN-BUDGET-ESCAPE`, `R-FENCE-TABLE-DRIFT`, `R-BOUNDARY-PREDICATE-COVERAGE`, `R-INVENTORY-HAND-LIST`, and R3.2 (the class-A probes re-pointed to `LastCommit` and naming A1/A2/A3) | Builder; blind PE on the diff; Tester ablations (every gate RED first) |
| B2 | `R-membership` — retire `slashedRoot` and `validatorsSeenRoot` from the v5 digest set (D-V5-WHOLESET-ROOTS five → three) instead of capping identities; G-1 the explicit `objective()` guard | GATED (7 gates), certified 2026-09-03; the freeze manifest (2026-09-07) finds G-1 NOT satisfied on main (the box-entry assert covers `verifyBond` only, not `MinBond`) | owner call 1 **RATIFIED 2026-09-07** (`D-TRUE-UP-CALLS-2026-09-07`); one PR: the `objective()` guard that also covers `MinBond` + the digest-set change (a hard fork at activation, free while era-4 is dark) — PRE-FREEZE | Researcher gates G-1…G-7; Tester RED-first |
| B3 | Recovery boundary (formerly #535) — the floor box is a COLD AUDITOR | CERTIFIED (a′), 4 conditions | owner call 2 **RATIFIED 2026-09-07** (`D-TRUE-UP-CALLS-2026-09-07`); one PR: unconditional loud stall, delete `RecoveryDirective.Heights` + `LiveFollower`, refuse pruned blocks, `trustFloor` off the contract surface, the `-ws-checkpoint` re-anchor documented as irrecoverable-if-unreachable | Builder; PE |
| B4 | `R-CARRIER-BYTES` — the box witness/frame byte ceiling as a v5 validity rule | principle + formula CERTIFIED; VALUE gated on a pony-class measurement (none exists); BG-3 stalls the box above its own budget meanwhile | Tester: the measurement as SPECIFIED in the freeze manifest (item 8; the honest FLOOR ≈ 878 KiB is the binding constraint, not the box); **OWNER** ratifies the value; the rule lands at the stamp raise (Lane D) | Tester measurement → owner → Builder |
| B5 | `R-CARRIER-GENESIS-DISPOSAL` O-2 — a "pruned" genesis served to a fresh-sync victim with our hash and an attacker-chosen body (`AppendGenesis` never checks `IsPruned()`) | both halves SHIPPED (#720, #723); the O-2 probe is OWED | red-team probe carrying the PE's composition (attacker-declared `BondRegs` qualifying its own keys under a kept `Pruned`); if it lands, Tester encodes it, Researcher certifies the refusal | red-team |
| B6 | Small owed items: `R-CARRIER-DOUBLESIGN-SLOT` (LOW; the evidence producer lifts a carried precommit onto the parent's `Atts`, with the SILENT-GREEN and FALSE-SLASH traps as gates) · `R-CARRIER-ORDER-ORACLE` (Tester rewrites the call-site gate on the behavioural oracle in the pre-maturity branch) · `R-SWARM-NOTBANKED-DEAD` (reachability argument or removal) · `R-CARRIER-CREDIT-DENIAL` (owner call 3 **DECLINED 2026-09-07**, `D-TRUE-UP-CALLS-2026-09-07`: no inclusion window in v1; the doc fix is all that is left) | OPEN, all LOW | one small PR each, any order | Builder / Tester |
| B8 | `#558` torn `chain.cbor` → silent genesis fallback (a SIGKILL mid-persist tears the chain store; replay silently discards finalized history back to genesis) | **DONE 2026-09-07 (RC-blocking by scope call S3, `D-RC-SCOPE-S2-S4`)**: `chainstore.Save` fsync'd + atomic; `chainstore.Recover` refuses to start on a torn or corrupt tail unless `-accept-chain-loss`, which preserves the original as `chain.cbor.rejected-<unix>`; five gates RED under ablation (three library, two daemon-level e2e); blind PE ruling `/Users/andrewedmond/Claude/claude/silt-reviews/principle-engineer/RULING-b8-558-chainstore-refuse-to-start-2026-09-07.md` (MERGEABLE-WITH-CHANGES, all three blockers closed). **Owner note:** the rule refuses on any structural-verification failure, a larger surface than S3's "torn tail" — kept per the PE; ratify or narrow | — (merged; the `7-restart-survival` and WS cold-sync flows on the next graded run confirm the boot path) | Builder; blind PE |
| B7 | `R1.8` the accept-flip — wire `WitnessValidateV5` → Accept-iff-all-predicates-pass | NOT done; a consensus-rule change (I1) | after Lane D (the freeze) and Lane E4 (B8 on the frozen artifact); requires B1–B4, the legacy-mode invariant (CONVERGED 2026-09-02: v5-only, BG-1), FP-1 inert under `D-FP2-SCOPE`, and the cold-box tier as the tier every recompute gate runs in | Builder; Researcher re-certifies the composed flip predicate; OWNER ratifies (I1) |

#### Lane C — Boulder 2: the economy (turn it on, prove it under adversary)

| # | Item | Status | What is left | Gate / seat |
|---|---|---|---|---|
| C1 | R2.9 tail — the attended PR deleting the `core/demand` v2 primitive and the ledger's flat leg (`RedeemDeliveryCreditReason`, ~25 tests re-homed) | B-9 retired the flat path at the node (#764); the primitive + flat leg remain | one attended PR (defined in `docs/thinking/2026-09-07-b9-flat-path-retirement.md`); `R-GUARD-RESTORE-LANE-UNKNOWN` (persist the lane byte in the guard record, a store-format bump) rides with it | Builder; blind PE |
| C2 | R2.9 owner calls: the delivery idle window (REFUSE-UNTIL-SET; `T_b` = 44 s/height MEASURED 2026-09-07 ⇒ one epoch ≈ 5.9 min; 2–3 epochs ≈ 12–18 min is the arithmetic) · `R-SESSION-WALLCLOCK-STEP` (a forward wall-clock step reaps every live session) · `R-CREDITSPENT-UNBOUNDED` (the cap as an operator-managed ceiling, PE recommends accept) · `R-ANCHOR-STALL` (≤ 300,000 credits per 1 GiB relay session, disclosed v1 residual) · the cloud sheet's row 13b SKIP-vs-GAP (PE recommended SKIP, built as SKIP) | **all five DECIDED 2026-09-07** (`D-TRUE-UP-CALLS-2026-09-07` (4)–(8)): the idle window stays REFUSE-UNTIL-SET until A1's fix lands and the ≤ f+1 bound is field-confirmed, then the default is set ABOVE that bound; `R-SESSION-WALLCLOCK-STEP` and `R-ANCHOR-STALL` are disclosed v1 residuals; the 65,536 `creditspent` cap is an operator-managed ceiling until the epoch-bind; 13b is SKIP | one small doc PR carrying the three disclosed-residual lines (`R-SESSION-WALLCLOCK-STEP`, `R-ANCHOR-STALL`, the operator-managed `creditspent` ceiling with its rotate-and-clear rule); the idle-window default is a one-line builder PR AFTER Lane A3's green run (depends on A1, not on the rows above) | Builder |
| C3 | R2.2 full observability set (serve-work AND repair-work Gini, per-tier margin, live `g`, funded-horizon, wash detection; network panels via the DHT crowd-estimator with knowability tiers) | DEFINED (`docs/thinking/2026-09-01-economy-observability-design.md` §3–§4: endpoints, tiers, the two gossip fields) | one builder session on the Economist's 17-row build list (`/Users/andrewedmond/Claude/claude/silt-reviews/economist/ADVISORY-boulder2-telemetry-spec-R2.4-checklist-and-RC-scope-2026-09-07.md` §2): `/api/economy/flows`, `/g`, `/concentration`, `/network`, the two gossip fields (`servedBytes`, `repairsDone`; tier class DERIVED from gossiped `CapTotal`), the four dashboard panels (no page consumes `/api/economy/self` today); the design doc's network repair-Gini gate is VACUOUS (0.9902 by construction under D-TIERING) — the Tester asserts instead `serveGini ≤ 0.15` ∧ pony serve-share ≥ 0.50, `repairGini ≤ 0.40` within the capable sample, caretaker set ≥ 3, each ablation RED first | Builder + Tester (the testable telemetry gate) |
| C4 | R2.7 blocking telemetry — `servedBytesWitnessed` / `servedBytesUnwitnessed` (the A2 detector) and `bountyPaidToEscrowFunder` (the A4 detector) | SPECIFIED 2026-09-07 (`/Users/andrewedmond/Claude/claude/silt-reviews/economist/ADVISORY-boulder2-telemetry-spec-R2.4-checklist-and-RC-scope-2026-09-07.md` §1): seven new counters + one split — A2: `serveBytesObjectAware` (`escrow.go` RecordServeToObject), `serveBytesWitnessed` (`deliveryanchor.go` SettleDelivery), `serveBytesLaneEvicted`, `serveBytesSupersededFlat` (dies with C1); `servedBytesUnwitnessed` is DERIVED; **`bountyPaidToEscrowFunder` is DEGENERATE** (every escrow funder on a ledger is `n.id`) and is replaced by the prepay/skim split of `funded`, `Stats.BountyPaidToSelf` (`repairclaim.go`; `claim.Holder` is attacker-declared), and `bountyPaidToPriorFetcherCredits`; the third detector is `spendRefusedInsufficientCredit` / `spendRefusersDistinct` (the build-immutable-#4 affordability floor; abort-only). All ride existing token-gated, privacy-withheld blocks; no `(fetcher × object)` join anywhere (Don't #3) | one small builder PR with a unit test per detector (fixture shapes in the advisory) | Economist specifies → Builder |
| C5 | Pre-flip closers (each before R2.4): `R-BOUNTY-TRUNCATION` (Researcher: accumulator vs truncation warning at the geometry) · `R-SHORT-FINAL-STRIPE` (parity at the shard's true length; a second content-addressing break, so it lands inside 4′'s window or before economy-on) · `R-ANCHOR-BEARER-TRANSFER` (red-team pass on the bearer-anchor transfer surface) · `R-DEMAND-PRICE-LEVEL` (**OWNER** at the flip; the bonded-fetcher credential is the lever) · `R-RELAY-WASH-ZERO-LOSS` (**OWNER**: a relay skim before R2.4, re-opened by R2.12) · `R-LAMBDA-DUST′` (Researcher corrects G-R212-7 §6.2's dust bound to the measured 24.00 GiB) · `R-RELAY-ANON-SET′` (Researcher: the two-sided statement) · `R-REFUSE-AND-SELF-SPEND` (Researcher direction) · `R-PARITY-AMPLIFICATION` (Researcher: discharge, or accept the worst case under the 64 GiB pin) | **CERTIFIED 2026-09-07** (`/Users/andrewedmond/Claude/claude/silt-reviews/research/research-outcome/BOULDER2-residual-closures-bounty-truncation-lambda-dust-relay-anon-refuse-self-spend-parity-estimand-RESEARCH-CERTIFICATION-2026-09-07.md`): `R-BOUNTY-TRUNCATION` GATED — bounded at the shipped geometry (the truncated fraction stays in escrow, D-S7 threshold 35.998) but the worst case was mis-filed (`-chunk-size 52412` under-pays 50 % silently), so G-BT-1 (a truncation arm on the publish warning) + G-BT-2 (divide AFTER the rarest-shard multiplier, free and dominant) lift it; the accumulator REFUTED on #8 · `R-LAMBDA-DUST′` CLOSED (24.00 GiB is the escrow leg and exact, the server leg 3.43 GiB, never summed; ≤ 3.3 % of a grant; D-S7's 36 is a rate with no remainder term) · `R-RELAY-ANON-SET` stays OPEN, held: the property moved from enforced-by-construction to elected-and-budgeted at one face per rotation · `R-REFUSE-AND-SELF-SPEND` GATED-INERT on the relay lane, ARMED at R2.4 on the delivery lane (attacker gain nil; "serial bound to root" REFUTED — it destroys buy-ahead blindness; the sound direction is a showing binding over the existing commitment `M`) · `R-PARITY-AMPLIFICATION` DISCHARGED (`N/K` is a fetcher-bandwidth bound; the per-server floor corrects DOWN to 27.94 GiB, so 64 GiB gains margin 1.43× → 2.29×; `R-CORRUPT-PROVIDER-BANDWIDTH` filed) · G-BB-21 DISCHARGED (the handoff paragraph is §7, one-sided falsifier: only a measurement ABOVE 64 GiB is decisive) · G-R212-8 DISCHARGED as restated (the surviving form is the concurrency arithmetic already gated in `cmd/silt/numeraire.go`) | Builder: G-BT-1 + G-BT-2 (one small PR) and the `swarm.go` stale burn comment; red-team: `R-ANCHOR-BEARER-TRANSFER`; `R-DEMAND-PRICE-LEVEL` + `R-RELAY-WASH-ZERO-LOSS` **DECIDED 2026-09-07** (`D-TRUE-UP-CALLS-2026-09-07` (13): `(U, p)` unmoved, `RequireBondedFetchers = false`, `RelaySkim = 0/1` — re-confirmed at R2.4 only if C7 contradicts it; the relay-wash line is a disclosed residual); `R-SHORT-FINAL-STRIPE` Builder | Researcher (done) / red-team / owner / Builder |
| C6 | R2.4 the economy-ON default flip — phased: correctness gates (incl. FP-1 inert) → economy-OFF baselines incl. the pre-flip Gini → canary → default | DEFINED as a CHECKLIST 2026-09-07 (`/Users/andrewedmond/Claude/claude/silt-reviews/economist/ADVISORY-boulder2-telemetry-spec-R2.4-checklist-and-RC-scope-2026-09-07.md` §3): correctness gates → economy-OFF baselines over ≥ 3 epochs → canary (window = 1 epoch ≈ 5.9 min at the measured `T_b`, hysteresis 3 windows; HARD aborts: any `guardFullRefusals`, caretakers < 3, `BountyPaidToSelf` > 0, any rise in `spendRefusersDistinct`; soft aborts: coverage < 0.75, pony serve-share < 0.50 or −20 pp, `serveGini` > baseline + 0.10, `repairGini` > 0.40, funded horizon < 600 s, evicted/object-aware > 0.01) → default | after C1–C5; the flip is the FIRST of the three `D-FP2-SCOPE` re-arm triggers (FP-2, FP-1, `R-F8-RESTART-REWIND` re-open with it) | Builder; Economist; OUTSIDE the RC by `D-RC-SCOPE-S1` — a `0.9.x` release after C7's clean verdict, before `1.0.0` |
| C7 | R2.7 economy-ON adversarial-solvency verdict (five inequalities × seven attacks; A2 supersede-suppression and A5 cold-start capture unpriced) | SCOPED; after C3 + C4 + C6 | Researcher NEW-CERT + red-team pass; feeds the R4.4 brief | Researcher; red-team |
| C8 | R2.8 cold-repair funding path (reserve-aware scheduling + early cliff disclosure to ANY funder + R2.9's expiring remainder; no network pool, never a mint) | DEFINED 2026-09-03; after C3 | one builder session; five inputs stay ASSUMPTION until live data | Builder |
| C9 | Measurements owed (external or Tester): `B_bootstrap` + the honest arrival rate on REAL traffic (the flixz export; G-BB-21 the estimand restated as the CUMULATIVE per-server draw — `R-BB-ESTIMAND-MISSPECIFIED`) · `M_seen` pony-class value (the class-M streaming verifier's TIME ceiling) · G-λ-11 the receipt-lane operator cost before `PF` may move | OPEN | the flixz handoff (T-1 first, G-BB-5 answered first); two Tester measurements on the floor box | Tester / external |
| C10 | Small owed PRs: `R-COMPACT-ORPHAN` WARN line (surface `CompactFailures` / `LastCompactError` on the banked/status path) · `R-PRIVACY-OPERATOR-TAB-TOKEN` (a persistent operator-token route before the `-privacy` default reaches real operators) · R2.14b `MsgRelayFund` (top-up with fresh anchors on an admitted session) | OPEN, LOW | one small PR each | Builder |

#### Lane D — Boulder 3: the freeze (the stamp-raising release IS the RC)

| # | Item | Status | What is left | Gate / seat |
|---|---|---|---|---|
| D1 | **The era-4/v5 freeze manifest** — the ONE ordered list of everything the stamp-raising release contains or decides: `tagRevLogSize` (≤ 1 leaf, liveness-only) · the `Block.IssuerKeys` per-block cap (value) · the (d-3) two-level block hash (`AnswerDigest`; the only close for `R-LATE-REVEAL` and the `Slashes` cap's completeness face) · `R-AAXIS-TAG-RESERVE` · `R-GENESIS-HASH-FREEZE-SURFACE` + the #237 on-disk migration policy · `R-ISSUERKEY-POP` slot · the R0.4b-FDH versioned change · `R-CREDITSPENT-UNBOUNDED` epoch-bind · the `Serial` validity rule (the SMT keyspace-injectivity oracle) · `R-CARRIER-BYTES` as a validity rule (B4) · R-membership five → three (B2) · `R-CARRIER-ROLLOUT-SIGNAL` named-upgrade check · the `proof.Unmarshal` gob-length allocation bound | **CERTIFIED 2026-09-07 as a 22-item manifest in four classes with THREE deadlines** (`/Users/andrewedmond/Claude/claude/silt-reviews/research/research-outcome/ERA4-V5-FREEZE-MANIFEST-RESEARCH-CERTIFICATION-2026-09-07.md`): T-FREEZE-SURFACE refutes the ROADMAP's "a size rule on hash-covered content cannot land in-era" (`D-F2-EVIDENCE-RECOMPUTE` landed no-era-gate five days after the era-3 freeze), so FORMAT items freeze at the freeze, VALIDITY items land at the stamp raise, and one TEST item may land at the flip. **FORMAT (a miss = era-5):** `tagRevLogSize` CERTIFIED-BUY (a wrong `m` is a WRONG-ACCEPT — `VerifyConsistency` degenerates at m = 1 — so it cannot ride the head record); D-V5-WHOLESET-ROOTS five → three GATED (G-1 is NOT satisfied on main: the box-entry assert covers `verifyBond` only, not `MinBond`); (d-3) `AnswerDigest` SPECIFIED, **OWNER** call, recommend BUY (closes `R-LATE-REVEAL`, the `Slashes` cap's completeness face and the pruned-hash scope at once); the PoP slot RESERVE + length bound; `R-AAXIS-TAG-RESERVE` REFUTED (`statehash.Key` is injective for distinct NUL-free tags — the Researcher corrects its own R4.2 §4b); the genesis hash is OUTSIDE the era surface (network identity, not format). **VALIDITY (stamp raise):** `IssuerKeys` cap CERTIFIED = COUNT 4,096 and INADMISSIBLE ALONE (needs a proposer packing budget + an `(issuer, epoch)` distinctness clause); `R-CARRIER-BYTES` value still GATED, the measurement SPECIFIED, the honest FLOOR (≈ 878 KiB) is the binding constraint; `Serial` rule CERTIFIED = `SerialSize` 32; the R0.4b-FDH change DECLINED in favour of a prefix-freeness gate; `R-CREDITSPENT-UNBOUNDED` is NOT a consensus format (stays Lane C2); blind-sampling not this release. **Activation:** no mechanism beyond the readiness tally — the release is ONE constant (`chain.go:1840` stamps `BlockVersionRegGate`; both tallies fire at one boundary, no v4 block is ever minted); the tally sits behind `everMature` (`R-ERA4-NEEDS-MATURITY`, disclosed). | Builder lands the FORMAT items in one stamp-raising PR train against the manifest's gates; the nine §8 sentences are **RATIFIED 2026-09-07** (owner call 9, `D-TRUE-UP-CALLS-2026-09-07` — `tagRevLogSize` BOUGHT, (d-3) `AnswerDigest` BOUGHT, the PoP slot RESERVED, `IssuerKeys` 4,096 with packing budget + distinctness, `SerialSize` 32, #237 refuse-to-start, genesis = network identity, tally behind `everMature`, no activation mechanism); the B8 green list is §6 (two groups RED or absent today) | Researcher (done); OWNER; Builder |
| D2 | Stamp-raise test deliverables (same release, not the freeze): `R-CARRIER-PRUNED-HASH` (prove the seen-fold never depends on a pruned body; end-to-end that the first non-pruned descendant's root recompute catches a rewritten ancestor) · `R-CARRIER-MODELCHECK` (the seating AGREEMENT property) · `R-E2E-ERA4-FIXTURE` (objective + bonded + epoch-enabled fixture; the e2e cost ACCEPTED 2026-09-07, owner call 14) · `R-CLOUD-ERA-PROBE` (expose the block era on a CLI/status surface so cloud row 13b can tell "era-4 dark" from "keys off-commitment") | certified as manifest items 14–19 (`/Users/andrewedmond/Claude/claude/silt-reviews/research/research-outcome/ERA4-V5-FREEZE-MANIFEST-RESEARCH-CERTIFICATION-2026-09-07.md`): the carrier model-check; the h43 reproduction (item 15 — NOT on the old carry-list and the item most likely to slip the RC, Lane A1); the pruned-hash proof (scope depends on whether (d-3) is bought); the e2e era-4 fixture; the `proof.Unmarshal` gob-allocation bound (`R-R3-GOB-ALLOC-AMPLIFICATION`, deadline = THE FLIP); the era OBSERVABLE (one deliverable fusing `R-CLOUD-ERA-PROBE` with what survives of `R-CARRIER-ROLLOUT-SIGNAL`, whose code half is vacuous) | Tester + Builder inside the stamp-raising PR train; #237 = REFUSE TO START across a format boundary | Tester |
| D3 | R3.4 the freeze decision itself — the owner freezes the format at the RC on D1's manifest, and the readiness stamp goes 3 → 5 (no release ever stamps 4; era-3 retired unrun, the #632 note) | ratified as a principle 2026-09-03; the nine manifest sentences RATIFIED 2026-09-07 (owner call 9); the ACT is owed | **OWNER** freezes at the RC, after D1 + D2 + B1–B4 + B8 — and commissions the external seat then (owner call 12) | owner |

#### Lane E — Boulder 4: the RC gates

| # | Item | Status | What is left | Gate / seat |
|---|---|---|---|---|
| E1 | R4.3b-pre-on — the eight certified preconditions for `-dht-address-cap=on` (de-herd + PE O-1/O-2; the pony's own direct dial (v) with G-14; `cap_relay ∈ [2, K−R]`, `R ≥ K/2`; the v6 width flag with the `/32` floor; the exempt gauge + warning; PE O-3; one ≥3-relay cloudtest shadow run + one at `NAT_MODE=symmetric`; gates G-14…G-19 + the red-team's six) | DEFINED 2026-09-04 (certified); shadow mode MERGED (#725) | one builder session for (1)–(6) + (8); the shadow run rides the next graded run (Lane A3); then **OWNER** ratifies R and `cap_relay` with the printed floor and flips `on` (owner call 10) | Builder + Tester → owner |
| E2 | R4.2 the A-axis — re-scope to measure / publish / hand to B8 as-is; do NOT wire A3 | measure/publish SHIPPED (#715); the re-scope **RATIFIED 2026-09-07** (owner call 11, `D-TRUE-UP-CALLS-2026-09-07`) — A3 is NOT wired | put the bonded-adversary domain-collision finding in the R4.4 brief (one doc edit) | Builder (doc) |
| E3 | R4.3 continuous internal red-team hunt on the not-yet-run backlog (class-P compound ordering; bondreg full path; DHT/eclipse; long-range/WS checkpoint; relay/PayWord; churn/restart `everMature`) | standing | runs opportunistically between builds; every confirmed break → Tester gate | red-team |
| E4 | **R4.4 = R1.7 — the external B8 pass** against the FROZEN, never-Accept artifact: C1 (no discount), C2 (no quiet capture), the seven `m0.md` §7 seams, plus the brief items (the `SlashesBytesCap` second face; the bonded-adversary domain collision; the R2.7 verdict) | the M0 close gate; brief `docs/reviews/m0-redteam-brief-2026-08.md` | after D3; **OWNER** commissions the external seat (owner call 12); findings triaged to the build-immutable bar and re-attacked until clean | external |
| E5 | The RC field grade — a green multi-machine GCP deep run of the frozen artifact (the standing release gate) | last graded: `2633a11-deep` 30 pass / 2 gap / 0 fail / 3 skip (2026-09-07; the gaps are the #350 low-bond premise and a harness artifact fixed in #774) | after A1 and D3 | Tester |
| E6 | R1.8 the flip (B7) → `1.0.0` | — | after E4 + E5, and the two S6 scaling kills (Boulder 5's `1.0.0` gates, scope call S2) | owner |

**Owner calls (2026-09-07) — each is one sentence to ratify; the recommendation is the seat's, the call is the owner's.** Calls 1–9, 11, 13–17 were decided 2026-09-07 (`D-TRUE-UP-CALLS-2026-09-07`); 18–20 the same day (`D-CONSENSUS-ARMING`). Calls 21–23 (raised by the h43 build's delta certification) were ratified 2026-09-07 (`D-H43-WORKLESS-DESIGNEE`). **Still owed: 10 (after the R4.3b shadow run) and 12 (at D3).**
1. **R-membership (B2):** close it by retiring `slashedRoot` and `validatorsSeenRoot` from the v5 committed digest set (five → three) rather than by capping seated identities; a hard fork at activation; not certified until the `objective()` guard lands. **✅ RATIFIED 2026-09-07 (`D-TRUE-UP-CALLS-2026-09-07` (1)).**
2. **Recovery boundary (B3):** the floor box is a COLD AUDITOR — unconditional loud stall, the two directive knobs deleted, pruned blocks refused, `trustFloor` off the contract surface, recovery by a fresh `-ws-checkpoint`-class anchor at H+1, irrecoverable if unreachable. **✅ RATIFIED 2026-09-07 (`D-TRUE-UP-CALLS-2026-09-07` (2)).**
3. **`R-CARRIER-CREDIT-DENIAL` (B6):** decline the multi-block inclusion window for v1 (the carrier already narrows the pre-existing denial to one proposer; the window is a format change). **✅ DECLINED 2026-09-07 — doc fix only (`D-TRUE-UP-CALLS-2026-09-07` (3)).**
4. **The delivery idle window (C2):** the reaper runs on the WALL clock (`core/node/deliverysession.go:29-31`), so a chain stall reaps live sessions — a 2-epoch window (≈ 12 min) would have reaped every session during the 17-minute h43 stall. *Recommended: do not set the default until Lane A1 publishes the f=1 liveness bound; then set it above that bound (the arithmetic today: ≥ 4 epochs ≈ 24 min); refuse-until-set stays meanwhile.* **✅ DECIDED 2026-09-07 as recommended (`D-TRUE-UP-CALLS-2026-09-07` (4)): refuse-until-set stays; the default is set only after Lane A1's fix lands and the ≤ f+1 bound is field-confirmed, and then above that bound.**
5. **`R-SESSION-WALLCLOCK-STEP` (C2):** accept as a disclosed v1 residual (a forward wall-clock step reaps every live session and the deposit returns at anchor expiry, so nothing is lost but latency). **✅ ACCEPTED 2026-09-07 (`D-TRUE-UP-CALLS-2026-09-07` (5)).**
6. **`R-CREDITSPENT-UNBOUNDED` (C2):** accept the 65,536 cap as an OPERATOR-MANAGED ceiling (rotate the publish key AND clear `creditspent.log` together) until the epoch-bind lands with the freeze. **✅ ACCEPTED 2026-09-07 (`D-TRUE-UP-CALLS-2026-09-07` (6)).**
7. **`R-ANCHOR-STALL` (C2):** accept ≤ 300,000 credits per 1 GiB relay session as a disclosed v1 residual; R2.14b `MsgRelayFund` is the follow-on. **✅ ACCEPTED 2026-09-07 (`D-TRUE-UP-CALLS-2026-09-07` (7)).**
8. **Cloud row 13b (C2):** SKIP (not GAP) while era-4 is dark, behind the era probe. **✅ DECIDED 2026-09-07: SKIP (`D-TRUE-UP-CALLS-2026-09-07` (8)).**
9. **R3.4 the freeze (D3):** the nine sentences of the freeze manifest §8 — buy `tagRevLogSize` (a safety leaf, not liveness-only); buy (d-3) `AnswerDigest`; reserve the PoP slot; the `IssuerKeys` cap at 4,096 with its packing budget and distinctness clause; `SerialSize` 32; refuse-to-start across a format boundary (#237); the genesis hash is network identity; the tally stays behind `everMature`; no activation mechanism beyond the tally. **✅ RATIFIED 2026-09-07, all nine (`D-TRUE-UP-CALLS-2026-09-07` (9)); the freeze ACT itself (D3) is still owed at the RC.**
10. **R4.3b `on` (E1):** ratify R = 4, `cap_relay` = 4 (K − R) with the printed floor, after the shadow run reports series A < 5 % and series B < 20 %. *Owed after the run.*
11. **R4.2 (E2):** re-scope the A-axis to measure / publish / hand to B8 as-is; do not wire A3. **✅ RATIFIED 2026-09-07 (`D-TRUE-UP-CALLS-2026-09-07` (11)).**
12. **R4.4 (E4):** commission the external B8 seat at the RC. *Owed at D3.*
13. **`R-DEMAND-PRICE-LEVEL` and `R-RELAY-WASH-ZERO-LOSS` (C5):** decided AT the R2.4 flip, not before. *Economist's recommendation: do not move `(U, p)` and keep `RequireBondedFetchers = false`; no relay skim in v1 (`RelaySkim = 0/1`), a disclosed residual.* **✅ ACCEPTED 2026-09-07 (`D-TRUE-UP-CALLS-2026-09-07` (13)): the Economist's position is the standing one carried INTO the flip — `(U, p)` unmoved, `RequireBondedFetchers = false`, `RelaySkim = 0/1` — re-confirmed at R2.4 only if C7's verdict contradicts it.**
14. **`R-ISSUERKEY-POP` and `R-E2E-ERA4-FIXTURE`:** reserve the PoP slot inert at the stamp raise; accept the e2e cost. *Folded into D1/D2.* **✅ ACCEPTED 2026-09-07 (`D-TRUE-UP-CALLS-2026-09-07` (14)): reserve-only; the e2e cost is accepted.**
15. **The `-grant-capacity` help note:** leave as is. **✅ DECIDED 2026-09-07: leave (`D-TRUE-UP-CALLS-2026-09-07` (15)).**
18. **The consensus arming rule (A1):** arm the round clock on a REPLICATED condition (pending work or any consensus message for the working height), add a relayable round certificate with suffix-semantics round-changes, and make the designee propose at the certificate's round — a consensus-rule change (I4, with I1 preserved by `slotCompare`). **✅ RATIFIED 2026-09-07 (`D-CONSENSUS-ARMING`).**
19. **The published liveness bound and the harness (A1/A3):** publish ≤ f+1 rounds (190 s at f=1) as the bound; re-price the `6-fault-tolerance` tiers to 190 / 380 s; graded runs log `-log debug` on validators; no graded run until G-H43-1 (RED on main today) is GREEN on the fix. **✅ RATIFIED 2026-09-07 (`D-CONSENSUS-ARMING`).**
20. **#380 (A2):** objective mode ignores the local `Quorum` floor in `ValidateCommit` and keeps it as a proposer-side gather target only. **✅ RATIFIED 2026-09-07 (`D-CONSENSUS-ARMING`).**
17. **`R-PS-LOCAL-ROT-NOT-HEALED` (Lane TAIL):** evict a verified-rotten shard the node still hosts and announces, so the fetch path replaces it — a trade of one operator's hosting count and D-S7 revenue for truthful durability. *Recommended: take it (PE); research-gated if the fix reaches bounty or escrow.* **✅ ACCEPTED 2026-09-07 (`D-TRUE-UP-CALLS-2026-09-07` (17)): take it.**
16. **Structure Round 1B (B1):** scope the structure round MAIN-ONLY (Round 1A) and take the five box-entry-dependent closers in Round 1B after the HELD box-entry round-A branch merges, rather than merging round A first. **✅ DECIDED 2026-09-07: main-only first (`D-TRUE-UP-CALLS-2026-09-07` (16)).**
21. **The restated liveness bound (A1, amends call 19):** publish **f′+1 rounds after GST, where f′ counts governing-set seats that are DOWN or hold none of the height's work at the round they are designated** — with the entry forward landing f′ = f and the number stays 190 s at f = 1; a lost forward is bounded by the re-keyed takeover at ≤ (N+2)·30 s + G = 430 s at N = 12. **✅ RATIFIED 2026-09-07 (`D-H43-WORKLESS-DESIGNEE` (21)).**
22. **The entry-forward cap (A1, a security parameter):** the entries one work-holder forwards to a round's designee on round entry are capped by their OWN constant, value 4 — never the client's `entrySubmitBurst` = 32 (N−1 forwarders at one seat). **✅ RATIFIED 2026-09-07: 4 (`D-H43-WORKLESS-DESIGNEE` (22)).**
23. **`R-H43-NULL-PROPOSAL` (scope):** PBFT's null request — the literature's answer to a workless leader — is a verifier-posture change inside the frozen era surface; route it to era 5, outside the RC. **✅ RATIFIED 2026-09-07 (`D-H43-WORKLESS-DESIGNEE` (23)).**

**Scope calls — these move the RC date more than any build. All four are DECIDED (S1 `D-RC-SCOPE-S1`; S2–S4 `D-RC-SCOPE-S2-S4`).**
- **S1 — ✅ DECIDED 2026-09-07 (`D-RC-SCOPE-S1`): NO — the RC ships economy default-OFF with every lane built; B8 attacks economy-ON in the harness; the flip is a `0.9.x` release before `1.0.0`.** Original question kept: is the economy-ON default flip (C6) inside the RC? Today the roadmap implies yes ("an economy-off HEAD certifies a network nobody runs"; R2.7 feeds the B8 brief) but no sentence says so. *Recommendation: the RC (`0.9.0`, the frozen stamp-raising release) ships with the economy DEFAULT-OFF and every paid lane BUILT, gated and dark; the B8 pass attacks the frozen consensus + the economy under `-economy` ON in the harness; the default flip is a `0.9.x` release inside the RC line once C7's verdict is clean, before `1.0.0`. Reason: the flip re-arms FP-2/FP-1 and is the one economic mechanism with no field data behind it; freezing the format does not depend on it. The Economist independently recommends (b) (`/Users/andrewedmond/Claude/claude/silt-reviews/economist/ADVISORY-boulder2-telemetry-spec-R2.4-checklist-and-RC-scope-2026-09-07.md` §4): under (a) a pony eats silent restart loss on an ephemeral ledger and loses the T-AR baseline forever; under (c) it carries durability's cost for a whole major version with the reward half dark — a T-AR violation; under (b) it defers ~33 % of revenue for one release.*
- **S2 — ✅ DECIDED 2026-09-07 (`D-RC-SCOPE-S2-S4`): the operational floor is POST-RC as Boulder 5; the two S6 scaling kills are `1.0.0` gates.** Original question kept: Is the operational floor (packaging, signed installers, R4 self-update, the S6 O(delta) maturation and reprovide dirty-tracking) inside the RC?** The archived Phase 5 says "RC-path, needs scoping"; the flixz handoff says post-RC (Boulder 5). *Recommendation: post-RC for packaging/self-update; the two S6 scaling kills are `1.0.0` gates because they price out the honest operator (build-immutable #4/#8). Define Boulder 5 accordingly.*
- **S3 — ✅ DECIDED 2026-09-07 (`D-RC-SCOPE-S2-S4`): RC-BLOCKING, one builder PR, no research gate — Lane B8.** Original question kept: `#558` torn `chain.cbor` → silent genesis fallback. A real silent-loss residual against the no-silent-loss floor. *Recommendation: RC-blocking; the fix direction is in the repro doc (atomic chain-store write + refuse-to-start on a torn tail); one builder PR, no research gate.*
- **S4 — ✅ DECIDED 2026-09-07 (`D-RC-SCOPE-S2-S4`): POST-RC (`1.x`); the crypto choice is research-gated then.** Original question kept: `#437` transport authentication (TLS/Noise). Certified NOT a safety break; residual = censorship over a controlled link. *Recommendation: post-RC (`1.x`), research-gate the crypto choice then.*

### The Boulders — status ledger (trued up 2026-09-07)

The five Boulders are unchanged as the organizing spine (Boulder 5, the post-RC operational floor, was defined 2026-09-07 by scope call S2); each Rock is one line here. DONE means
merged on main with its PR. Everything OPEN appears in the critical-path lanes above, by row id.

#### Boulder 0 — Stop the live bleed (A4 money-pump) · ✅ DONE + MERGED
R0.1–R0.5 (#686, cert `A4-provisional-eviction-conservation-RESEARCH-CERTIFICATION-2026-09-01.md`); RT-DELIV-3 (#699, gate #700); R0.4b receipt-expiry (#711; its four owner calls ratified 2026-09-03); R0.6 the I5 cross-height `Pruned` slash forgery (#714; `SlashesBytesCap` 16 MiB ratified); R0.7 relay-lane mint (interim #718 → closed by R2.14 #721).

#### Boulder 1 — Make the accept-flip safe (floor-box witness-soundness spine)
- **DONE:** R1.0 · R1.1 · R1.2 · R1.3 (refuted, withdrawn) · R1.4 (certified, re-closed by #704) · R1.5 (#702) · R1.6 (#701) · `R-FOLD-LIVE-STATE-READS` Direction A (#706) · `R-CARRIER-REFLECTION` (#705) · `R-ROTATE-EPOCH-LAST` (#703) · `R-COLD-BOX-HARNESS` (permanent tier) · `R-VERIFYBOND-WIRING` · `R-STATEVIEW-ENUMERATION` (closed; freeze scope ZERO leaves for safety) · the `LastCommit` carrier O1/O2 (#719 merge gates + MG-C, #720) · `R-CARRIER-BOXSPLIT` / `R-CARRIER-SIG-COMPOSITION` · `R-HASH-LITERAL-PIN` + `R-V5-TAGSET-EQUALITY` (#719) · `R-CARRIER-GENESIS-DISPOSAL` both halves (#720, #723) · O3 Direction T `R-FORKCHOICE-WEIGHT` with `R-558-VERIFIER-INVENTORY`, `R-INTERLOCK-GATE`, `R-FORKCHOICE-RAMP-GUARD`, `R-I5-TEXT-AND-CLAIMS-LEDGER`, `R-O4-CANON-HASH-COVERAGE` (widened I5) (#722) · `R-AST-PIN-GLOB` + `R-S5-STRING-REGISTRY` (#716) · FP-2 closed by scope with G-FP2-0 (#729, #732).
- **OPEN (Lane B):** B1 `R-STRUCTURE-REDERIVATION` (+ `R-BOXENTRY-RESIDUALS`, the `HeadRef` trio, `R-INVENTORY-HAND-LIST`, R3.2) · B2 `R-membership` · B3 recovery boundary · B4 `R-CARRIER-BYTES` · B5 O-2 probe · B6 the four small items · B7 R1.8 the flip. B8 `#558` is DONE (2026-09-07). Stamp-raise deliverables `R-CARRIER-PRUNED-HASH`, `R-CARRIER-MODELCHECK`, `R-CARRIER-ROLLOUT-SIGNAL`, `R-E2E-ERA4-FIXTURE`, `R-ISSUERKEY-POP` sit in Lane D. Lane A (the h43 stall) is consensus liveness and is homed here.
- **HELD-IN-TENSION / declined:** `R-CARRIER-PREFIX-ONLY` · `R-STATEROOT-EQUIVALENCE-SEAM` (the state-root predicate stays an equivalence-with-gates, never "one implementation everywhere") · `R-CARRIER-VECTOR-CAP` (declined unless the M0 claim re-opens) · `R-DOUBLESIGN-TIP-BLIND` (LOW) · FP-1 (inert under `D-FP2-SCOPE`) · option M (re-basing maturity on `bonded`; a published-claim change, declined by default).
- Design record: [`docs/thinking/2026-09-01-floorbox-witness-soundness-fix-design.md`](docs/thinking/2026-09-01-floorbox-witness-soundness-fix-design.md); the root-cause paragraph and every source are in the archive file.

#### Boulder 2 — Turn the economy on, prove it under adversary
- **DONE:** R2.1 slice 6a (#689) · R2.3 (ratified: A4 fix first) · R2.5 (1024 MiB resident at production chunk; ONE prod-chunk repair fits a 2 GB pony) · R2.6 (hold; G2-gated) · R2.9 direction ratified 2026-09-04, ledger half (#759), node half (#760), the deposit-at-anchor-expiry G-6 (#763), B-9 flat-path retirement at the node (#764), the 256 KiB default + true-length manifest framing 4′ with the NEW genesis `f428d0a8…0951` (#765), the cloud delivery-lane flow (#766) · R2.9a the `B_bootstrap` instrument, all seven owner calls (#734–#745; `grant/r` = 64 GiB; `P` = all honest fetchers; 1 bin/doubling; `-privacy` withheld by default; G-BB-12′ reader-is-operator; the build tag) · R2.10 F8 chain-anchored epoch (#727) · R2.11 issuer-key peer-submit (#747; S6 measured #748) · R2.12 faucet rate limit (#746), the empty-bucket advance (#754), the relay re-price G-R212-2 (#755), the numéraire λ and all five G-R212-7 calls (#758), the bounty geometry correction (#761) · R2.13 (#717) · R2.13b (#724) · R2.14 relay prepayment anchor (#721) · the per-stripe parity fetch (#751) · `R-DEFAULT-CHUNK-BOUNTY-ZERO` + `R-MANIFEST-PADDING` (#765).
- **OPEN (Lane C):** C1 the v2 primitive deletion · C2 the disclosed-residual doc PR + the idle-window default after A3 · C3 R2.2 · C4 the R2.7 telemetry · C5 the pre-flip closers · C6 R2.4 · C7 R2.7 · C8 R2.8 · C9 measurements · C10 small PRs.
- **HELD-IN-TENSION / closed by bound:** see the register — `R-STOCK-RENEWABLE-OCCUPANCY`, `R-EDGE-PREMIUM-REMOVED`, `R-SKIM-OBJECT-ATTRIBUTION`, `R-DELIVERY-PIN-GROUND`, `R-NUMERAIRE-SANDWICH`, `R-AUDIT-REWARD-DOMINATES`, `R-LAMBDA-WASH-MINT`, `R-FAUCET-BUCKET-PROBEABLE`, the `R-BB-*` trades, and the FP-2 carry-list.
- Design records: [`docs/thinking/2026-09-01-economy-observability-design.md`](docs/thinking/2026-09-01-economy-observability-design.md) and the dated R2.x deliberations in `docs/thinking/`.

#### Boulder 3 — Freeze prerequisites (era-4/v5) · the RC release
- **DONE:** R3.1 the SMT domain-separation residual (#731, #749; G-R31-5 the empty-leaf refusal #754; closed as an owned residual, `docs/design/state-root-domain-separation.md`) · R3.3 (doc-only; re-derive only if #299 moves).
- **OPEN (Lane D):** D1 the freeze manifest · D2 the stamp-raise test deliverables · D3 the freeze act. R3.2 (the `probeUncovered` debt) is folded into B1's gate list.

#### Boulder 4 — Standing gates + M0 endgame
- **DONE:** R4.3a the DHT domain-0 exemption stripped + the C2 metric print (#715) · R4.3b shadow mode with de-herd relay selection (#725).
- **OPEN (Lane E):** E1 R4.3b-pre-on → `on` · E2 R4.2 ratification · E3 the standing hunt · E4 R4.4 = R1.7 the external B8 pass · E5 the RC field grade · E6 the flip → `1.0.0`. R4.1 is a review gate that fires per PoD increment, not scheduled work.

#### Boulder 5 — the operational floor · POST-RC (scope call S2, 2026-09-07)
- **Post-RC (`1.x`):** packaging, signed installers, R4 self-update (the flixz handoff's gaps). `#437` transport authentication (TLS/Noise) is post-RC by scope call S4, research-gated on the crypto choice.
- **`1.0.0` gates (on the E6 path, not the RC):** the two S6 scaling kills — O(delta) maturation and reprovide dirty-tracking — because O(store) cold-start and O(held) reprovide price out the honest operator (build-immutables #4/#8).

**Watch-items / standing gates (not scheduled):** bond-floor vs pony-disk ratio (a dashboard row); owned-residual doc lines (SHA-256 pinned, store-free verify, R3 16 KiB cap); PayWord re-derivation (R3.3). The old Rock 2 (third-operator committed settlement, DEFINITION only, #658) attaches its demand→standing bright-line to R4.1 and is post-RC.

> *The 2026-08-19→08-31 six-Rock overlay ("Superseded Rocks"), the retired Phases 1–6 and the
> 2026-08-26 priority snapshot live in
> [`/archive/roadmap-history-2026-09-01.md`](archive/roadmap-history-2026-09-01.md); the
> 2026-09-01→07 per-Rock detail lives in
> [`/archive/roadmap-boulders-detail-2026-09-07.md`](archive/roadmap-boulders-detail-2026-09-07.md).*

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

- **The 2026-09-01→07 build run closed the live breaks and built the paid economy dark.** Sixty-seven
  PRs (#697–#767): the A4 money-pump (Boulder 0), the I5 pruned-slash forgery, the relay-lane mint, the
  floor-box witness-soundness spine through R1.6, the `LastCommit` carrier, O3 Direction T, the whole R2.9
  paid-delivery economy on PayWord under the numéraire (ledger + node halves, deposit-at-expiry, B-9), the
  `B_bootstrap` instrument behind a build tag, F8, the faucet, R2.14, R4.3b shadow, R3.1 — every one
  blind-reviewed and certified, every gate ablation-proven RED first. The graded run `c450985-deep` closed
  the economy on the wire (29 pass / 2 gap / 0 fail) and DISCOVERED the h43 liveness stall (Lane A). The
  2026-09-07 true-up then rewrote this file around the critical path to the RC.
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

> **Residual filing rule (2026-09-07).** The backlog grew from 7 named residuals on 2026-09-01 to 127 on
> 2026-09-06 with no bucket and no closer on most of them, so disclosed trades and bounded numbers sat in the
> same list as work. A residual name is any token of the form `R-<UPPER>[-<UPPER|DIGIT>…]` (a trailing `′`
> marks a re-priced restatement and is the same name). A new residual name may be introduced anywhere in this
> file ONLY if the same PR adds one row for it to the **Residual register** below with all five columns filled:
> `Name` · `Bucket` (exactly one of `ACTIONABLE`, `HELD-IN-TENSION`, `CLOSED-BY-BOUND`, `UNCLEAR`) · `Closer`
> (ACTIONABLE: the PR / measurement / owner call and the Boulder or Rock it belongs to; other buckets: the
> sentence or source that states the classification) · `Source` (a `ROADMAP.md` line anchor `L123` at filing
> time, or a `silt-reviews/…` path) · `Duplicate-of` (`—` or the canonical name). An `UNCLEAR` row carries
> `since:YYYY-MM-DD` and may stay so for at most one merged PR. A closed or merged residual KEEPS its row
> (bucket `CLOSED-BY-BOUND`, `Duplicate-of` filled); rows are never deleted, so the register is also the audit
> trail. As-built rules are not residuals. Enforced by `scripts/check_residual_register.py`
> (`scar:residual-backlog-unbucketed-2026-09-06`). Only the ACTIONABLE bucket is work.
>
> True-up of record (2026-09-07, 125 names): every ACTIONABLE row now opens with its critical-path lane row (`Lane B1`, `Lane C5`, …) or `Lane TAIL`; the four UNCLEAR rows are disposed (see the PE ruling cited on each); `archive:L123` in a Source or Closer is a line of [`/archive/roadmap-boulders-detail-2026-09-07.md`](archive/roadmap-boulders-detail-2026-09-07.md), where the pre-true-up prose lives verbatim.
>
> Triage of record (2026-09-06, 115 names): ACTIONABLE 45 · HELD-IN-TENSION 14 · CLOSED-BY-BOUND 51 · UNCLEAR 4;
> nine duplicates merged. ACTIONABLE items by closer: the structure-build PR (13, incl. the three `HeadRef`
> items), pre-freeze owner ratifications (5), the stamp-raise release (6), the G-6 refund certification + build
> (3), the B-9 flat-path retirement PR (1), the `T_b` measurement (2), Researcher-owed (5), economy owner calls
> (4), small LOW PRs (7), backlog (1). Line anchors in `Source` are the lines at filing time and drift with edits;
> the name is the key.

### Residual register

| Name | Bucket | Closer | Source | Duplicate-of |
|---|---|---|---|---|
| `R-GUARD-RESTORE-LANE-UNKNOWN` | ACTIONABLE | Lane C1 · pre-existing (found by the blind PE on the deposit build): `LoadPaidSerials` rebuilds every guard entry as lane DELIVERY because `ports.PaidSerial` carries no lane, so the per-lane live counts (`LivePaidSerialsByLane`) and `RestoredGuardEntries` conflate the two populations after a restart; closer: persist the lane byte in the durable record (a store-format bump, Boulder 2 / B-9-adjacent LOW PR) | silt-reviews/principle-engineer/RULING-R2.9-deposit-at-anchor-expiry-4d4a90c-2026-09-07.md | — |
| `R-STOCK-RENEWABLE-OCCUPANCY` | HELD-IN-TENSION | under the deposit a fixed credit stock can fill the paid-serial guard EVERY window (under the burn, once); bounded by Σ credits ever granted / f per window — the honest price of a renewable honest budget; the R2.12 flow assertion stays the coherence gate (refund cert §6.3) | silt-reviews/research/research-outcome/R2.9-session-remainder-refund-and-live-anchor-cap-RESEARCH-CERTIFICATION-2026-09-06.md | — |
| `R-ANCHOR-BEARER-TRANSFER` | ACTIONABLE | Lane C5 · a demand anchor is a bearer instrument: its presenter need not be its payer (the refund therefore pays only an existing account — M1); owed a red-team pass on the transfer surface (Boulder 2, before R2.4) | silt-reviews/research/research-outcome/R2.9-session-remainder-refund-and-live-anchor-cap-RESEARCH-CERTIFICATION-2026-09-06.md | — |
| `R-REFUND-NEEDS-AN-ACCOUNT` | CLOSED-BY-BOUND | a fetcher with no account on the server's ledger at release forfeits its remainder (burned, counted `RefundsBurnedNoAccount`); ≤ f per session; the honest fetcher always has one (it bought the anchor here); closable later by registering at OPEN without a grant | silt-reviews/research/research-outcome/R2.9-session-remainder-refund-and-live-anchor-cap-RESEARCH-CERTIFICATION-2026-09-06.md | — |
| `R-BOUNTY-TRUNCATION` | ACTIONABLE | Lane C5 · GATED 2026-09-07: bounded at the shipped 256 KiB (0.006 %; the truncated fraction stays in escrow, so D-S7 holds at 35.998) but `-chunk-size 52412` under-pays 50 % silently; closer: one small Builder PR — G-BT-1 a truncation arm on the existing publish warning + G-BT-2 divide AFTER the rarest-shard multiplier (free, strictly dominant); the accumulator is REFUTED on build-immutable #8 | silt-reviews/research/research-outcome/BOULDER2-residual-closures-bounty-truncation-lambda-dust-relay-anon-refuse-self-spend-parity-estimand-RESEARCH-CERTIFICATION-2026-09-07.md | — |
| `R-MANIFEST-PADDING` | CLOSED-BY-BOUND | CLOSED 2026-09-07 (4′ BUILT and MERGED, #765, under the NEW genesis the owner accepted): `pipeline.ManifestFrameSize` frames a manifest that fits in one chunk at its own length + 8; larger manifests keep the chunk size — the bound: true-length framing applies to a SINGLE-chunk manifest only (`core/pipeline/pipeline.go:326-332`); a multi-chunk manifest still pads, now to 262,144 B. `chunk.Split` pads every frame to the chunk size and manifests carry no parity (87.6 % of the flixz store is 1.4 KB manifests padded to 65,536 B); closes with the same PR as the 262,144 B default — one content-addressing break (D-R2.9-NODE-HALF-CALLS 4′, Boulder 2) | L497, silt-reviews/economist/ADVISORY-default-chunk-size-256KiB-2026-09-06.md | — |
| `R-BOX-ATTESTS` | CLOSED-BY-BOUND | archive:L608: "O1 and O2 are OWNER-RATIFIED; O4 is RATIFIED; O3 is RATIFIED (Direction T, built in PR #722)"; the carrier is merged (archive:L610). The one live fragment, "R-BOX-ATTESTS invariant II / G-F" (an S7 clause), is carried inside row 9. [note: Umbrella; its fragments are rows 6–8, 23–35.] | archive:L31, archive:L608–683, silt-reviews/research/research-outcome/R-BOX-ATTESTS-scoping-CONVERGED-RESEARCH-VERDICT-2026-09-02.md | — |
| `R-FOLD-LIVE-STATE-READS` | CLOSED-BY-BOUND | archive:L498: "Direction A landed (PR #706)"; archive:L501–544: the remaining obligation is a standing discipline (every new recompute gate runs in the cold-box tier), which row 3 encodes. [note: Generalized into row 3.] | archive:L491–544, silt-reviews/research/research-outcome/floorbox-R-FOLD-LIVE-STATE-READS-RESEARCH-CERTIFICATION-2026-09-02.md | — |
| `R-COLD-BOX-HARNESS` | CLOSED-BY-BOUND | archive:L536: "CLOSED as a permanent tier (`core/chain/floorbox_recompute_coldbox_v5_test.go`)". | archive:L536–582 | — |
| `R-STRUCTURE-REDERIVATION` | ACTIONABLE | ROUND 1A MERGED 2026-09-08 (`builder/floorbox-structure-1a`; cert §8.1): the composition, both dispatches, the sealed `StateView`, the box-owned byte budget, 8 doors unexported, the stage-cover gate + G-D13 digests + the v4/v5 parity oracle, honest twins on all 26 gates; blind PE MERGEABLE-WITH-CHANGES (all closed) + Researcher CERTIFIED. Round 1B (the box-entry closers) remains, blocked on `builder/floorbox-box-entry`. Lane B1 · archive:L561: "RATIFIED 2026-09-03; BUILD OPEN, after R-STATEVIEW-ENUMERATION." Closer: the structure build PR (Boulder 1, own session). | archive:L506, archive:L561–611, PE/RULING-floorbox-predicate-rederivation-structure-2026-09-03.md | — |
| `R-STATEVIEW-ENUMERATION` | CLOSED-BY-BOUND | archive:L570: "CLOSED 2026-09-03 (closure GATED G-1…G-5; freeze scope CERTIFIED: ZERO leaves for safety)." Note: the archive:L67 owner sentence (freeze scope ≤ one leaf `tagRevLogSize`) carries no ✅; it now lives on the R3.4 carry-list (archive:L1253, "owner ratifies"). | archive:L67, archive:L507, archive:L570–634, silt-reviews/research/research-outcome/R-STATEVIEW-ENUMERATION-closure-and-freeze-scope-RESEARCH-CERTIFICATION-2026-09-03.md | — |
| `R-CARRIER-PARENT-BINDING` | ACTIONABLE | Lane B1 · archive:L642: "DEFINED (CERTIFIED direction; flip-gated; ZERO format change)"; archive:L650: "No `HeadRef` symbol exists on any tree yet." Closer: `HeadRef` lands in the structure build (row 4). Boulder 1 flip precondition. [note: Same closer as rows 7, 8.] | archive:L509, archive:L642–695 | — |
| `R-CARRIER-PARENTPROPOSER` | CLOSED-BY-BOUND | ROUND 1A MERGED 2026-09-08 (`builder/floorbox-structure-1a`; cert §8.1): CLOSED for both witness directions — the slot no longer exists; the id is `HeadRef.ProposerID`. The 1B pin question (which parent the box holds) is folded into `R-DRIVER-ASSERTED-BOXSTATE` (PE: the ADD direction RELOCATED to `NewBox(parent)`, inert under the R1.8 downgrade). Lane B1 · archive:L654: "RE-PRICED: NOT a leaf, NOT pre-freeze … Remains a flip precondition." Closer: row 4's `HeadRef`. | archive:L511, archive:L654–701 | `R-DRIVER-ASSERTED-BOXSTATE` |
| `R-LOGROOT-FORMAT-SCOPE` | ACTIONABLE | ROUND 1A MERGED 2026-09-08 (`builder/floorbox-structure-1a`; cert §8.1): k = 0 CLOSED with zero format change (P13b, G-D7); k ≥ 1 STALLS by name (`ErrRevLogSizeUnauthenticated`, G-D8 + the G-D9 control) until `tagRevLogSize` lands at R3.4; the stall masks P13a on revocation-bearing blocks — coverage owed when the leaf lands. Lane B1 · archive:L760: "a flip-gated SAFETY item; closes with ZERO format change" — verify-not-recompute with `parentLogRoot` in the head record. Closer: row 4; liveness half needs `tagRevLogSize` (R3.4, archive:L1253). | archive:L646, archive:L760–809 | — |
| `R-BOXENTRY-RESIDUALS` | ACTIONABLE | 1B — blocked on `builder/floorbox-box-entry` (not "row 4"; cert §8.1, 2026-09-08). Lane B1 · archive:L593: "OPEN (box-entry round A, HELD unmerged; the HOLD is RATIFIED)"; "Live findings owed with it". Closer: the structure build (part B). [note: Contains rows 10–16 and the G-F clause.] | archive:L512, archive:L593–648, silt-reviews/research/research-outcome/floorbox-box-entry-round-A-fee43ba-DELTA-CERTIFICATION-2026-09-03.md | — |
| `R-SUPPORT-SLASHED-SCREEN` | CLOSED-BY-BOUND | ROUND 1A MERGED 2026-09-08 (`builder/floorbox-structure-1a`; cert §8.1): CLOSED — P4 precedes Q3 in the composition and the standalone recompute screens the author on the anchored `slashed` leaf (N1), driven gate. Lane B1 · Closer: structure build — the build-plan cert deletes `BoxQuorumSupport` (`RO/FLOORBOX-STRUCTURE-BUILD-PLAN-CERTIFICATION-2026-09-03.md` residual table). | archive:L601 | — |
| `R-MALFORMED-DIVERGENCE` | CLOSED-BY-BOUND | ROUND 1A MERGED 2026-09-08 (`builder/floorbox-structure-1a`; cert §8.1): CLOSED for the box-vs-node axis (`BoxQuorumSupport` gone; the composition calls the shared constructor); the mirror-vs-node axis folds into `R-PTABLE-DRIFT`. Lane B1 · Build-plan cert: "CLOSED — `BoxQuorumSupport` is deleted, not fenced" — in the PLAN; ROADMAP archive:L601 still lists it open. Closer: row 4. | archive:L601 | — |
| `R-PIN-LABEL-ESCAPE` | ACTIONABLE | 1B — blocked on `builder/floorbox-box-entry` (not "row 4"; cert §8.1, 2026-09-08). Lane B1 · Closer: row 4 (driver-supplied inputs become box-owned). | archive:L602 | — |
| `R-PIN-REANCHOR-POSITION` | ACTIONABLE | 1B — blocked on `builder/floorbox-box-entry` (not "row 4"; cert §8.1, 2026-09-08). Lane B1 · Closer: row 4. | archive:L602 | — |
| `R-PIN-BUDGET-ESCAPE` | ACTIONABLE | 1B — blocked on `builder/floorbox-box-entry` (not "row 4"; cert §8.1, 2026-09-08). Lane B1 · Build-plan cert: "RE-PRICED — now buildable (`s.budget` is box-owned). Not owed this round." Closer: row 4. | archive:L602 | — |
| `R-FENCE-TABLE-DRIFT` | CLOSED-BY-BOUND | ROUND 1A MERGED 2026-09-08 (`builder/floorbox-structure-1a`; cert §8.1): CLOSED — the hand list is gone; G-6/G-6b derive the door inventory by AST (two `*Chain` doors; the package surface allow-listed with reasons). Lane B1 · Build-plan cert: "CLOSED by NG-5 + the one-door surface" (plan). Closer: row 4. [note: Instance of row 17 (archive:L869: "mirrored by R-FENCE-TABLE-DRIFT").] | archive:L602, archive:L869 | — |
| `R-BOUNDARY-PREDICATE-COVERAGE` | ACTIONABLE | Lane B1 · Build-plan cert: "CLOSED — the composition names Q1–Q4 explicitly" (plan). Closer: row 4. | archive:L603, silt-reviews/research/research-outcome/floorbox-predicate-rederivation-STRUCTURE-RESEARCH-VIEW-2026-09-03.md | — |
| `R-INVENTORY-HAND-LIST` | ACTIONABLE | Lane B1 · archive:L866: "OPEN (Tester)". Closer: derive the surface from code — lands with row 4's one-door surface; Tester owns the gate rewrite. [note: Canonical for row 15.] | archive:L866–913 | — |
| `R-membership` | ACTIONABLE | Lane B2 · archive:L456: "GATED, 7 gates; owner ratification owed; PRE-FREEZE"; archive:L468: owner sentence (retire `slashedRoot`/`validatorsSeenRoot`, D-V5-WHOLESET-ROOTS five → three; G-1 blocks until the `objective()` guard lands). Owner call 1 RATIFIED 2026-09-07 (`D-TRUE-UP-CALLS-2026-09-07`). Closer: the pre-freeze digest-set PR with the `objective()` guard (Lane B2). | archive:L86, archive:L456–515, silt-reviews/research/research-outcome/R-membership-unbounded-sets-and-recovery-boundary-DIRECTION-RESEARCH-CERTIFICATION-2026-09-03.md | — |
| `R-A-membership-source` | CLOSED-BY-BOUND | Built rule, cited at archive:L743 as semantics ("in a mature epoch the class-A screen reads the frozen `epochSet`"). Register as a RULE, not a residual. | archive:L743, silt-reviews/research/research-outcome/floorbox-recompute-classA-classP-wholeset-RESEARCH-CERTIFICATION-2026-08-31.md | — |
| `R-CARRIER-REFLECTION` | CLOSED-BY-BOUND | archive:L528: "DONE (PR #705, 2026-09-02, test-only)". | archive:L528–577 | — |
| `R-VERIFYBOND-WIRING` | CLOSED-BY-BOUND | archive:L541: "CLOSED." | archive:L541–585 | — |
| `R-ROTATE-EPOCH-LAST` | CLOSED-BY-BOUND | archive:L544: "DONE (PR #703, test-only)". | archive:L544–597 | — |
| `R-CARRIER-BYTES` | ACTIONABLE | RE-PRICED 2026-09-08: the box pays `validateCarrier` TWICE on the door path (P12 + the recompute's (0a)), so the ceiling prices 2× until the (0a) call goes. Lane B4 · archive:L660: "principle + formula CERTIFIED, VALUE GATED on a pony measurement; owner ratifies"; archive:L669: "A pony-class measurement does not exist yet." Closer: Tester pony-class measurement → owner ratifies the value → validity-rule PR pre-freeze (R3.4). | archive:L244, archive:L330–383, archive:L511 | — |
| `R-CARRIER-GENESIS-DISPOSAL` | ACTIONABLE | Lane B5 · archive:L681: "✅ BOTH HALVES SHIPPED". Still owed (archive:L690): "**O-2** … red-team probe owed" (a "pruned" genesis with an attacker-chosen body). Closer: the red-team O-2 probe with the PE's composition (Boulder 1). | archive:L681–734 | — |
| `R-CARRIER-CREDIT-DENIAL` | ACTIONABLE | Lane B6 · archive:L693: "GATED (window); minimum REFUTED; vector REFUTED"; archive:L700: "Only dated item: a doc fix." Owner call 3 DECLINED 2026-09-07 (`D-TRUE-UP-CALLS-2026-09-07`): no inclusion window in v1. Closer: the doc fix only (Lane B6). [note: Spawns row 26.] | archive:L73, archive:L693–743 | — |
| `R-CARRIER-VECTOR-CAP` | CLOSED-BY-BOUND | Declined by default — archive:L73: "decline the vector cap unless the M0 claim is re-opened." Re-opens only with an M0 re-certification. | archive:L699 | — |
| `R-CARRIER-DOUBLESIGN-SLOT` | ACTIONABLE | Lane B6 · archive:L702: "NOT a consensus-rule change; severity LOW"; build-spec given, couples to row 23; Tester traps named. Build-plan cert: "OPEN, correctly Rocks". Closer: a small evidence-producer PR + the two Tester traps (Boulder 1, LOW). | archive:L702–754 | — |
| `R-DOUBLESIGN-TIP-BLIND` | CLOSED-BY-BOUND | Source: "open, LOW", bounded; no action named. | archive:L712 | — |
| `R-CARRIER-BOXSPLIT` | CLOSED-BY-BOUND | archive:L713: "CLOSED 2026-09-03" — one shared `validateCarrier`, three callers; RED gates on warm and cold tiers. | archive:L713–764 | — |
| `R-CARRIER-SIG-COMPOSITION` | CLOSED-BY-BOUND | merged 2026-09-07 into `R-CARRIER-BOXSPLIT` (triage §3: same fact, one closer) | archive:L721–770 | `R-CARRIER-BOXSPLIT` |
| `R-CARRIER-PREFIX-ONLY` | HELD-IN-TENSION | archive:L729: "held-in-tension, no action owed (cert §5)"; archive:L735: "Comments corrected; no build owed." | archive:L729–777 | — |
| `R-CARRIER-ORDER-ORACLE` | ACTIONABLE | Lane B6 · archive:L736: "OPEN, LOW (gate quality)." Closer: Tester rewrites the gate on the behavioural oracle in the PRE-MATURITY branch (Boulder 1 gate quality). | archive:L736–790 | — |
| `R-CARRIER-ROLLOUT-SIGNAL` | ACTIONABLE | Lane D2 · RE-PRICED 2026-09-07: the code half (a startup refusal on a pre-carrier binary) is VACUOUS by construction; what survives is the era OBSERVABLE, fused with `R-CLOUD-ERA-PROBE` into one stamp-raise deliverable (freeze manifest item 19), plus the release-runbook line | silt-reviews/research/research-outcome/ERA4-V5-FREEZE-MANIFEST-RESEARCH-CERTIFICATION-2026-09-07.md | — |
| `R-CARRIER-MODELCHECK` | ACTIONABLE | Lane D2 · archive:L757: "GATED, owed BEFORE the stamp raise." Closer: model-check PR before the stamp raise (Boulder 3 carry-list). | archive:L757–801, archive:L1253 | — |
| `R-CARRIER-PRUNED-HASH` | ACTIONABLE | ROUND 1A MERGED 2026-09-08 (`builder/floorbox-structure-1a`; cert §8.1): G-D11 (two-sided) shipped; the `Hash()` comment corrected; bounded not eliminated — the descendant catch is the only defence. Deadline: the stamp raise. Lane D2 · archive:L672: "OPEN, owed BEFORE the stamp raise (not a flip gate)." Closer: the two proofs/tests before the stamp raise (Boulder 3 carry-list). | archive:L672–722, archive:L1253 | — |
| `R-HASH-LITERAL-PIN` | CLOSED-BY-BOUND | archive:L768: "✅ ENCODED ON MAIN 2026-09-04". | archive:L768–818 | — |
| `R-V5-TAGSET-EQUALITY` | CLOSED-BY-BOUND | archive:L768: same sentence. | archive:L768–818 | — |
| `R-FORKCHOICE-WEIGHT` | CLOSED-BY-BOUND | archive:L779: "✅ MERGED-pending-PR 2026-09-04" (PR #722 per archive:L608). | archive:L255, archive:L779–845 | — |
| `R-558-VERIFIER-INVENTORY` | CLOSED-BY-BOUND | archive:L804: "✅ DONE 2026-09-04". | archive:L804–856 | — |
| `R-INTERLOCK-GATE` | CLOSED-BY-BOUND | archive:L815: "✅ DONE 2026-09-04". | archive:L815–866 | — |
| `R-FORKCHOICE-RAMP-GUARD` | CLOSED-BY-BOUND | archive:L825: "✅ DONE 2026-09-04 (both sites)". | archive:L825–875 | — |
| `R-I5-TEXT-AND-CLAIMS-LEDGER` | CLOSED-BY-BOUND | archive:L834: "✅ DONE 2026-09-04". | archive:L834–887 | — |
| `R-O4-CANON-HASH-COVERAGE` | CLOSED-BY-BOUND | archive:L846: "✅ DONE; NUMBERING RATIFIED 2026-09-04". Leftover text at archive:L853: "The #632 *frozen-and-retired-unrun* note is NOT landed" — a doc note; attach to row 33's stamp-raise runbook. | archive:L846–901 | — |
| `R-AST-PIN-GLOB` | CLOSED-BY-BOUND | archive:L862: "✅ DONE 2026-09-03". | archive:L862–907 | — |
| `R-S5-STRING-REGISTRY` | CLOSED-BY-BOUND | archive:L872: "✅ DONE 2026-09-03 … the third-time rule FIRED". | archive:L797, archive:L872–928 | — |
| `R-SWARM-NOTBANKED-DEAD` | ACTIONABLE | Lane B6 · archive:L887: "OPEN, LOW." Closer: a small Builder PR — reachability argument or removal. | archive:L887–933 | — |
| `R-E2E-ERA4-FIXTURE` | ACTIONABLE | Lane D2 · archive:L892: "NOT independently schedulable; a deliverable OF the stamp raise"; archive:L897: "Owner: accept the e2e cost increase at the stamp raise" (archive:L85, un-ticked). Owner call 14 ACCEPTED 2026-09-07 (the e2e cost). Closer: the fixture upgrade in the stamp-raising release (R3.4). | archive:L85, archive:L892–941, archive:L1253 | — |
| `R-ISSUERKEY-POP` | ACTIONABLE | Lane D1 · archive:L900: "RE-DIRECTED … reserve the format slot at the stamp raise"; archive:L906: "Owner: build the slot now vs reserve-only. Until then, correct the two comments." Owner call 14 DECIDED 2026-09-07: reserve-only (`D-TRUE-UP-CALLS-2026-09-07`). Closer: the inert slot reserved at R3.4; the research-gated D-DEMAND change is post-RC. [note: Canonical for the L359 entry "R0.4b-PoP" (same DSKS fact, older statement).] | archive:L83, archive:L900–950, archive:L1253 | — |
| `FP-1` | CLOSED-BY-BOUND | archive:L909: "OPEN and INERT under `D-FP2-SCOPE`; re-armed with FP-2 by the same three triggers. Still a flip precondition." Scope-closed; re-arm pinned by G-FP2-0 (shipped, PR #732). [note: FP-2 carry-list.] | archive:L513, archive:L909–961 | — |
| `FP-2` | CLOSED-BY-BOUND | archive:L920: "✅ CLOSED BY SCOPE 2026-09-04 (owner: 'scope close it is')"; re-armed by the FIRST of three named triggers; G-FP2-0 pins it. [note: Canonical for the carry-list (rows 49, 51, 52, 80, 99, 108, 109, 105-inert half).] | archive:L78–122, archive:L920–990 | — |
| `R-F8-RESTART-REWIND` | CLOSED-BY-BOUND | archive:L1062: "open-inert … on FP-2's carry-list with its close R-F8-RESTORE." | archive:L79, archive:L932, archive:L1061–1106 | — |
| `R-F8-RESTORE` | CLOSED-BY-BOUND | merged 2026-09-07 into `R-F8-RESTART-REWIND` (triage §3: same fact, one closer) | archive:L943, archive:L1064 | `R-F8-RESTART-REWIND` |
| `R-F8-SOURCE` | CLOSED-BY-BOUND | Not a residual — an as-built rule name inside R2.10 (CLOSED). Register as RULE or drop the `R-` prefix. | archive:L1042–1091 | — |
| `R-F8-LATCH` | CLOSED-BY-BOUND | Same as row 53. | archive:L941, archive:L1047–1092 | — |
| `R-F8-DISABLED` | CLOSED-BY-BOUND | Same as row 53. | archive:L1051–1098 | — |
| `R-LATE-REVEAL` | ACTIONABLE | Lane D1 · closes with (d-3) `AnswerDigest` if the owner buys it (freeze manifest item 3, recommend BUY); otherwise stays held as the cap's completeness face | silt-reviews/research/research-outcome/ERA4-V5-FREEZE-MANIFEST-RESEARCH-CERTIFICATION-2026-09-07.md | — |
| `R-AAXIS-TAG-RESERVE` | CLOSED-BY-BOUND | REFUTED 2026-09-07 by the freeze manifest (the Researcher corrects its own R4.2 §4b): `statehash.Key` is injective for any distinct NUL-free tags, so a future partition leaf is never a prefix collision; nothing to reserve | silt-reviews/research/research-outcome/ERA4-V5-FREEZE-MANIFEST-RESEARCH-CERTIFICATION-2026-09-07.md | — |
| `R-FETCHER-INCOME` | HELD-IN-TENSION | archive:L125: "`R-FETCHER-INCOME` is PERMANENT under per-node ledgers, not a bootstrap-phase residual". Structural; it is why the estimand is the lifetime draw. | archive:L125–168, archive:L1026, silt-reviews/research/research-outcome/R2.9a-Bbootstrap-instrument-sufficiency-…-2026-09-04.md | — |
| `R-BB-ESTIMAND-MISSPECIFIED` | CLOSED-BY-BOUND | DISCHARGED 2026-09-07 (G-BB-21): the commissionable handoff paragraph is certification §7 — cumulative per-server draw over one server uptime, `P` = all honest fetchers, `q` unpinned and swept, `C_max` from placement records never a census join, read the top occupied age bucket, one-sided falsifier (only a measurement ABOVE 64 GiB is decisive); Lane C9 commissions the flixz export from it | silt-reviews/research/research-outcome/BOULDER2-residual-closures-bounty-truncation-lambda-dust-relay-anon-refuse-self-spend-parity-estimand-RESEARCH-CERTIFICATION-2026-09-07.md | — |
| `R-BB-W-GATE-TENSION` | CLOSED-BY-BOUND | archive:L130: "CLOSED by restatement" (G-BB-1′ pins `q` only). | archive:L130–175 | — |
| `R-BB-TIER-ASYMMETRY` | HELD-IN-TENSION | archive:L133: "held in tension, discharges into G-BB-19"; archive:L136: it "does NOT license 'read the high end'". | archive:L133–185 | — |
| `R-BB-CENSUS-MIXTURE` | CLOSED-BY-BOUND | archive:L150: "DISSOLVED 2026-09-05 by the `P` = all-honest-fetchers ratification (`C = 0`)". The handoff still carries `C_max` (G-BB-10′). | archive:L151–201 | — |
| `R-BB-SINGLETON-CELL` | HELD-IN-TENSION | archive:L165–208: "the bin count is the ONLY lever … Don't #3 is on one side of the trade, so it is the owner's" — lever pulled (1 bin/doubling RATIFIED, PR #742); archive:L1034: "REDUCED, not closed … 1.39× at R = 10". Residual exposure accepted by the ratification. | archive:L165, archive:L1034 | — |
| `R-PRIVACY-OPERATOR-TAB-TOKEN` | ACTIONABLE | Lane C10 · archive:L172: "a persistent token route is the UX follow-on before the default lands on real operators." Closer: Builder UX PR before `-privacy` default reaches real operators (R2.9a / D-UI-PRIVACY-FLAG). | archive:L170–214 | — |
| `R-BB-SIBLING-AGGREGATES` | CLOSED-BY-BOUND | archive:L168: "CLOSED BY DEFAULT 2026-09-05 · behind the `-privacy` flag … withheld from unauthenticated readers in every build; published only under `-privacy=off`, labelled." archive:L1221's "Still open and NAMED, the owner's trade" is the earlier text; archive:L168 supersedes it. | archive:L173–222, archive:L1221 | — |
| `R-BB-BOND-STAMP-TUPLE` | CLOSED-BY-BOUND | archive:L216: "CLOSED (G-BB-28, 2026-09-05: the bond-path first-seen stamp is deleted; nothing read it)." | archive:L216, archive:L1040 | — |
| `R-BB-DELTA-TRAJECTORY` | CLOSED-BY-BOUND | archive:L1221: G-BB-26 snapshot cache bounds it ("caps the amplification at 1 per interval"); `T` = 5 s RATIFIED (archive:L213, `D-STATUS-SNAPSHOT-INTERVAL`); archive:L1034: improves with the bin count. | archive:L1028, archive:L1034, archive:L1221 | — |
| `R-BB-CENSUS-SYBIL-PAD` | HELD-IN-TENSION | archive:L1032: "under the tag with the flag on, the census is still attacker-mintable for $0"; disclosed to the ratification via G-BB-18/19 (the run reports an adversarial-pad screen). | archive:L1030 | — |
| `R-BB-ANONYMITY-SET-SIZE` | CLOSED-BY-BOUND | archive:L1036: "closed as the RE-CERT §3.3 wrote it ('Closed by G-BB-12′') for every reader that is not the operator." | archive:L1028, archive:L1030, archive:L1036 | — |
| `R-BB-SUPPRESSED-IS-A-DISCLOSURE` | HELD-IN-TENSION | Source: "held-in-tension, structurally unremovable" — cannot be withheld without fusing "below the floor" with "no clock". [note: Sibling of row 74.] | archive:L1030 | — |
| `R-BB-EXPORT-SCALAR-BYPASS` | HELD-IN-TENSION | DISPOSED 2026-09-07 (blind PE): no census-derived value of any type leaves `core/credit` (54 exported `Ledger` methods enumerated; `firstFetchTick` has four in-package readers), bounded by `//go:build bbootstrap`; the CLASS is unprevented, so held. `core/credit/bbootstrap.go:537` overstates BB-20 (it compares only `top["bBootstrap"]`) — a comment fix | silt-reviews/principle-engineer/RULING-residual-register-true-up-e963034-2026-09-07.md | — |
| `R-BB-ESTIMAND-STEERABLE` | CLOSED-BY-BOUND | Proposed: superseded — `grant/r` is now pinned by an adversary-independent STRUCTURAL derivation (archive:L103: "METHOD CERTIFIED, G-BB-17 LIFTED"; archive:L102: 64 GiB RATIFIED); the census is falsification input only (archive:L109–152). No source states this closure; the Researcher should. | archive:L1030 | — |
| `R-BB-STAMP-BY-ANY-PATH` | CLOSED-BY-BOUND | archive:L1221 (b): "G-BB-24 … It moved to the one place `fetchedBytes` is written" — BUILT. | archive:L1221 | — |
| `R-BB-WITHHELD-IS-A-DISCLOSURE` | HELD-IN-TENSION | archive:L1036: "reopens G-BB-13′ Part B only if the loopback refusal is ever relaxed" — disclosed, revisit trigger named. [note: Sibling of row 70.] | archive:L1036 | — |
| `R-BB-TOKEN-MODE-STARTUP-ONLY` | HELD-IN-TENSION | DISPOSED 2026-09-07 (blind PE): operator posture. Widening a `0600` file owned by the daemon's euid needs that uid or root, and both already hold the token, so the post-start window admits no non-operator; a periodic re-check is the wrong trade | silt-reviews/principle-engineer/RULING-residual-register-true-up-e963034-2026-09-07.md | — |
| `R-DELIVERY-BURN-PRICES-THE-GUARD` | HELD-IN-TENSION | RE-PRICED 2026-09-07, not closed: the deposit locked for the anchor's window prices a guard slot at f for exactly its occupancy (stock/f), the same bound the burn gave; what remains is R-STOCK-RENEWABLE-OCCUPANCY. `docs/decisions.md` D-R2.9-NODE-HALF-CALLS (1): REFUND ratified with the ⌊g/f⌋ cap; "The MECHANISM owes its own certification before it is built; the code BURNS until then." Closer: the G-6 refund certification + build at `CloseDeliverySession` (R2.9). | archive:L1006, silt-reviews/research/research-outcome/R2.9-G-R212-8-…-2026-09-06.md | — |
| `R-DELIVERY-SKIM-COLLAPSE` | CLOSED-BY-BOUND | merged 2026-09-07 into `R-DEMAND-PRICE-LEVEL` (triage §3: same fact, one closer) | archive:L1006 | `R-DEMAND-PRICE-LEVEL` |
| `R-DEMAND-PRICE-LEVEL` | ACTIONABLE | Lane C5 · `docs/decisions.md` D-R2.9-NODE-HALF-CALLS (2): "a separate item for the economy-on decision; the bonded-fetcher credential is its lever." Owner call 13 DECIDED 2026-09-07 (`D-TRUE-UP-CALLS-2026-09-07`): `(U, p)` unmoved, `RequireBondedFetchers = false`. Closer: re-confirmed at R2.4 only if C7's verdict contradicts it (Lane C5). [note: Canonical for row 77.] | archive:L1006 | — |
| `R-DELIVERY-PIN-GROUND` | HELD-IN-TENSION | Source: "held-in-tension. Pin NOT re-opened; enforced by `G-λ-3` today". | archive:L1006 | — |
| `R-DELIVERY-SESSION-EPHEMERAL` | CLOSED-BY-BOUND | ESCALATED 2026-09-07 (refund cert §3.4): under the deposit a restart turns a refund OWED into a LOSS — live sessions' unsettled faces and pending deposits vanish while their guard entries survive durably; bounded by ≤ f per session, counted by `RestoredGuardEntries` (an upper bound, printed at boot and on /api/status); closes with FP-2 or a durable session/deposit store. Source: "open, bounded not eliminated … Closes with FP-2 or with §5.1" — inert under D-FP2-SCOPE. [note: FP-2 carry-list (row 50).] | archive:L1006 | — |
| `R-FACE-BURN-GRIEF` | CLOSED-BY-BOUND | CLOSED 2026-09-07: an abandoned session returns its whole face at anchor expiry (G-6R-5), so grief against a fetcher's own face is not grief. Source: "open, priced, gated. G-λ-8-10"; archive:L1006: "8-10 (OPEN, owner call)". Closer: the G-λ-8-10 owner call (zero-settle session spends no face) built with the G-6 refund (R2.9). | archive:L1006 | — |
| `R-EDGE-PREMIUM-REMOVED` | HELD-IN-TENSION | Source: "adopted, held-in-tension … Don't #7-correct, ratio-negative; ratify knowingly" — the R2.9 direction is ratified (archive:L1006). | archive:L1006 | — |
| `R-SETTLEMENT-SKIM-DELTA` | CLOSED-BY-BOUND | archive:L1006: "REMEDY CERTIFIED AND BUILT the same day … gates G-SKIM-1…6 RED under the per-settlement floor". | archive:L1006 | — |
| `R-SKIM-OBJECT-ATTRIBUTION` | HELD-IN-TENSION | Source: "held-in-tension, bounded, FORCED"; archive:L1006: "FORCED by Don't #3". | archive:L1006 | — |
| `R-SKIM-SESSION-FRACTION` | CLOSED-BY-BOUND | Source: "closed by bound. Worth `< 262,144 B` of fetch price". | archive:L1006 | — |
| `R-V2-V3-DEMAND-DILUTION` | CLOSED-BY-BOUND | CLOSED 2026-09-07 by B-9: the flat receipt is refused at the node, so one face has no second surface to buy a v2 demand unit on. archive:L1006: "closed by B-9's retirement PR, where `SubmitDeliveryReceipt` is DELETED (#764)". Closer: the B-9 flat-path retirement PR (R2.9). | archive:L1006 | — |
| `R-SESSION-WALLCLOCK-STEP` | CLOSED-BY-BOUND | ACCEPTED as a disclosed v1 residual 2026-09-07 (owner call 5, `D-TRUE-UP-CALLS-2026-09-07`): a forward wall-clock step or a chain stall reaps live sessions; the deposit returns at anchor expiry, so only latency is lost. Doc line rides the Lane C2 disclosed-residual PR. Lane C2 · EXTENDED 2026-09-07 (blind PE): field-observed, not hypothetical — a chain STALL (h43, 17 min), not only an NTP step, reaps live sessions on the wall-clock reaper; decide with the idle window (owner call 4) and `R-REAPER-FORFEIT`. Original: `docs/decisions.md` D-R2.9-NODE-HALF-CALLS (5) | silt-reviews/principle-engineer/RULING-residual-register-true-up-e963034-2026-09-07.md | — |
| `R-BOUNTY-BASE-DENOMINATION` | CLOSED-BY-BOUND | Built — archive:L1076: "`RepairBountyBase = c·k·shardBytes/(U/p)`" (PR #758, ratified call 3). L357's "BLOCKING" is stale text; true it up. | L357 | — |
| `R-NUMERAIRE-SANDWICH` | HELD-IN-TENSION | L357: "not separable; held in tension." | L357 | — |
| `R-AUDIT-REWARD-DOMINATES` | HELD-IN-TENSION | `docs/decisions.md` :2011–2013: "`AuditReward`/`AuditSlash` LEFT at 1,000/25,000 with the disclosure" — ratified at option 1. L357: "not a Sybil surface". | L357 | — |
| `R-LAMBDA-WASH-MINT` | HELD-IN-TENSION | L357 states the structural fact; the wash is bounded by the skim and the wash self-check panel (R2.1), not by `λ`. | L357 | — |
| `R-DEFAULT-CHUNK-BOUNTY-ZERO` | CLOSED-BY-BOUND | CLOSED 2026-09-07: `pipeline.DefaultChunkSize` = 262,144 B MERGED (#765, decision 4′); the geometry premise corrected (#761: a shard is a whole ciphertext chunk, base 2 not 0); what remains is `R-BOUNTY-TRUNCATION` | L356 | — |
| `R-LAMBDA-DUST′` | CLOSED-BY-BOUND | CLOSED 2026-09-07: the certification's 3.0 GiB priced a single accumulator; the two-floor escrow leg is 24.00 GiB and exact, the server leg 3.43 GiB, never summed; credits ≤ 16,384 = 3.3 % of a grant; D-S7's threshold 36 is a rate with no remainder term, so solvency holds unchanged (correction filed: `G-R212-7-lambda-redenomination-CORRECTION-lambda-dust-2026-09-07.md`) | silt-reviews/research/research-outcome/BOULDER2-residual-closures-bounty-truncation-lambda-dust-relay-anon-refuse-self-spend-parity-estimand-RESEARCH-CERTIFICATION-2026-09-07.md | — |
| `R-RELAY-ANON-SET` | HELD-IN-TENSION | the canonical row (the ′ restatement folds in here): the relay re-price moved this residual BOTH ways — `k` dissolves at the OPEN only (purchase-side `k` untouched), forced ephemeral rotation is 24.4× coarser, and the elected ceiling stays ⌊g/f⌋ = 10 — so unlinkability moved from enforced-by-construction to elected-and-budgeted at one face per rotation; disclosed to the B8 brief (correction filed: `G-R212-2-relay-lane-reprice-CORRECTION-relay-anon-set-2026-09-07.md`) | silt-reviews/research/research-outcome/BOULDER2-residual-closures-bounty-truncation-lambda-dust-relay-anon-refuse-self-spend-parity-estimand-RESEARCH-CERTIFICATION-2026-09-07.md | — |
| `R-ANCHOR-STALL` | CLOSED-BY-BOUND | ACCEPTED as a disclosed v1 residual 2026-09-07 (owner call 7, `D-TRUE-UP-CALLS-2026-09-07`): ≤ 300,000 credits per 1 GiB relay session; R2.14b `MsgRelayFund` (Lane C10) is the follow-on. Doc line rides the Lane C2 disclosed-residual PR. Lane C2 · archive:L64: "Two owner calls carried, BOTH OPEN … (1) R-ANCHOR-STALL for v1, proposed as a disclosed residual"; archive:L1213: "Owner sentence still unsaid". Closer: the owner sentence at the re-priced numbers + follow-on R2.14b `MsgRelayFund` (R2.14). [note: Canonical for row 97.] | archive:L64–107, archive:L1168–1212, archive:L1213, silt-reviews/research/research-outcome/G-R212-2-relay-lane-reprice-…-2026-09-06.md | — |
| `R-ANCHOR-GRANULARITY` | CLOSED-BY-BOUND | merged 2026-09-07 into `R-ANCHOR-STALL` (triage §3: same fact, one closer) | archive:L1168 | `R-ANCHOR-STALL` |
| `R-RELAY-WASH-ZERO-LOSS` | ACTIONABLE | Lane C5 · archive:L1172: "DECIDED 2026-09-04 with R2.9 sentence 6: NO relay skim in v1; re-opens with R2.12"; R2.12 is BUILT (archive:L1076), so the re-open trigger fired; archive:L66: "(2) a relay skim before R2.4 — OPEN." Owner call 13 DECIDED 2026-09-07 (`D-TRUE-UP-CALLS-2026-09-07`): no relay skim in v1 (`RelaySkim = 0/1`), a disclosed residual; re-confirmed at R2.4 only if C7 contradicts it. Closer: the disclosed-residual doc line (Lane C5). | archive:L1006, archive:L1172 | — |
| `R-FEE-CONSTANCY` | CLOSED-BY-BOUND | archive:L1136: "inert; a NOTE on the FP-2 carry-list, not freeze-timed; no inert fee slot in `IssuerKeyReg`". [note: FP-2 carry-list (row 50).] | archive:L1135–1178, archive:L1174 | — |
| `R-ANCHOR-REPRESENT-LINK` | CLOSED-BY-BOUND | Source: "bounded (a refused open carried no traffic)"; carried unchanged in the as-built delta cert :161. | archive:L1174 | — |
| `R-DARK-UNTIL-ERA4` | CLOSED-BY-BOUND | Generalized into the stamp raise (R3.4); no separate action. [note: R3.4.] | archive:L66, archive:L1174 | — |
| `R-RELAY-MINT` | CLOSED-BY-BOUND | archive:L1201: "Researcher delta cert CERTIFIED (closes R-RELAY-MINT)". | archive:L1201 | — |
| `R-GUARD-SHARED-FILL` | CLOSED-BY-BOUND | archive:L1205: "closes with R2.12"; R2.12 BUILT (archive:L1076: start-up assertion `capacity × (grant/fee) × (W+1) ≤ MaxPaidSerial/4`); archive:L1006: the guard cap derivation re-based and "held by the R2.12 start-up assertion". | archive:L1205 | — |
| `R-REAPER-FORFEIT` | ACTIONABLE | Lane C2 · RE-BUCKETED 2026-09-07 (blind PE): its closer names owed Tester work, `T_b` is the input not the answer. The delivery idle reaper runs on the WALL clock and never the chain (`core/node/deliverysession.go:29-31`), so the 44 s/height that sizes the window and the 17-minute h43 stall that would reap every live session came from the same run — size the window against the STALL case (Lane A1's liveness bound), not the steady state; the relay lane keeps the burn and its settle-inside-one-epoch measurement is owed before `-accept-relay-payments` is enabled | silt-reviews/principle-engineer/RULING-residual-register-true-up-e963034-2026-09-07.md | — |
| `R-REFUSE-AND-SELF-SPEND` | HELD-IN-TENSION | GATED-INERT on the relay lane (Δ Σ_L = 0 and the attacker's gain is nil under per-node ledgers; the content is the fetcher's loss of `f` per open, which permanent refusal achieves anyway); ARMED at R2.4 on the delivery lane, where the certified direction is a SHOWING binding over the existing commitment `M` ("serial bound to the session root" REFUTED: it destroys guard (i)'s buy-ahead blindness) | silt-reviews/research/research-outcome/BOULDER2-residual-closures-bounty-truncation-lambda-dust-relay-anon-refuse-self-spend-parity-estimand-RESEARCH-CERTIFICATION-2026-09-07.md | — |
| `R-CREDITSPENT-UNBOUNDED` | ACTIONABLE | Lane C2 · NOT stamp-raise-class (freeze manifest item 12: a credit-format change is not a consensus format). Owner call 6 ACCEPTED 2026-09-07 (`D-TRUE-UP-CALLS-2026-09-07`); the operator rotate-and-clear rule is a doc line in the Lane C2 disclosed-residual PR; the epoch-bind stays the closer: accept the 65,536 cap as an OPERATOR-MANAGED ceiling (rotate the publish key AND clear `creditspent.log` together) until the epoch-bind lands (research-gated; partitions the D3 anonymity set) | silt-reviews/research/research-outcome/ERA4-V5-FREEZE-MANIFEST-RESEARCH-CERTIFICATION-2026-09-07.md | — |
| `R-COMPACT-ORPHAN` | ACTIONABLE | Lane C10 · archive:L1086: "✅ MERGED 2026-09-03 (PR #717)". OWED-AFTER (archive:L1105–1150): "the BENIGN compaction-failure class has no daemon WARN line — surface `CompactFailures`/`LastCompactError` on the banked/status path". Closer: small Builder PR. | archive:L1086–1152 | — |
| `R-FAUCET-ACCOUNT-MAP-UNBOUNDED` | CLOSED-BY-BOUND | merged 2026-09-07 into `FP-2` (triage §3: same fact, one closer) | archive:L1076, docs/thinking/2026-09-05-r2.12-faucet-rate-limit.md | `FP-2` |
| `R-FAUCET-RESTART-REGRANT-HERD` | CLOSED-BY-BOUND | Inert under D-FP2-SCOPE. [note: FP-2 carry-list.] | archive:L1076 | — |
| `R-FAUCET-BUCKET-PROBEABLE` | HELD-IN-TENSION | Same structural fact as row 68 (identities are free); disclosed, no closer named by the source. [note: Coupled to row 68.] | archive:L1076 | — |
| `R-PARITY-AMPLIFICATION` | CLOSED-BY-BOUND | DISCHARGED 2026-09-07: `N/K` is a FETCHER-total-bandwidth bound, not the per-server draw the pin names; under the deficit walk the honest server serves `K` shards for every provider behaviour, so the floor corrects from 44.7 GiB DOWN to `S_max/F_min` = 27.94 GiB — the ratified 64 GiB gains margin (1.43× → 2.29×) and is not reopened; the bandwidth face is `R-CORRUPT-PROVIDER-BANDWIDTH` | silt-reviews/research/research-outcome/BOULDER2-residual-closures-bounty-truncation-lambda-dust-relay-anon-refuse-self-spend-parity-estimand-RESEARCH-CERTIFICATION-2026-09-07.md | — |
| `R-SPARSE-COLUMN-PROVIDER` | ACTIONABLE | Lane TAIL · DISPOSED 2026-09-07 (blind PE): cap the per-provider shard probe in `confirmColumnHolders` (`core/node/file.go:1041-1064`) — a live 1-of-T holder costs up to T round-trips and the corpse gate trips only on proven-dead; NOT new with #751 and does NOT fool the durability audit (`probeShard`). Closer: a small Builder PR | silt-reviews/principle-engineer/RULING-residual-register-true-up-e963034-2026-09-07.md | — |
| `R-PS-PRESENCE-COST` | ACTIONABLE | Lane TAIL · Closer named by the source: the pay-on-failure redesign (Builder; residual backlog, no Boulder gate). | L550–1873 | — |
| `R-PS-LOCAL-ROT-NOT-HEALED` | ACTIONABLE | Lane TAIL · DISPOSED 2026-09-07 (blind PE): evict on verified-absent-but-stat-present so the fetch path replaces the rotten shard; the "#277/#500" home was wrong (no scrub exists; the sweep is stat-gated too, `core/node/repair.go:85,457,678`). Owner call 17 ACCEPTED 2026-09-07 (`D-TRUE-UP-CALLS-2026-09-07`): evict the verified-rotten shard the node hosts and announces (one operator's hosting count and D-S7 revenue traded for truthful durability). Closer: one Builder PR with a Tester gate RED first; research-gated if it reaches bounty or escrow | silt-reviews/principle-engineer/RULING-residual-register-true-up-e963034-2026-09-07.md | — |
| `R-H43-WORKLESS-DESIGNEE` | ACTIONABLE | Lane A1 · CERTIFIED 2026-09-07 (delta cert): a live designee holding none of the height's work wastes its round (the empty-block validity rule); present at f = 0; masked by M1 until (A) unmasked it. Closer: BUILT on `builder/h43-consensus-arming` — entries forwarded to the round's designee (cap 4, owner call 22), the takeover re-keyed to the round's designee, the bound restated (owner call 21, RATIFIED); gate G-H43-9 — merged in PR #772 | silt-reviews/research/research-outcome/CONSENSUS-LIVENESS-h43-ABC-ASBUILT-workless-designee-RESEARCH-CERTIFICATION-2026-09-07.md | — |
| `R-H43-NULL-PROPOSAL` | HELD-IN-TENSION | PBFT §4.4's null request (a leader with nothing to propose proposes a no-op) is the literature's closer for `R-H43-WORKLESS-DESIGNEE`; silt refuses an empty block as a validity rule, so adopting it is a verifier-posture change inside the frozen era surface. Owner call 23 RATIFIED 2026-09-07: era 5, outside the RC; own certification, own activation height | silt-reviews/research/research-outcome/CONSENSUS-LIVENESS-h43-ABC-ASBUILT-workless-designee-RESEARCH-CERTIFICATION-2026-09-07.md §3.5 | — |
| `R-H43-CERT-ROUND-ZERO-UNVERIFIED` | CLOSED-BY-BOUND | FOUND by the composed-diff re-cert (the merge blocker) and FIXED in the same branch (`ec13bd3`): `newViewFor` verifies nothing at round 0, so a `MsgRoundCert` at round 0 let any peer write unverified envelopes into `rs.Changes`; now refused at the receiver, at `checkRoundQuorum`, and at `verifyRoundChange` (C-1/C-2/C-3). Gate G-H43-14 (RED at `c2a476a`, GREEN at `ec13bd3`). #424 class, fourth recurrence | silt-reviews/research/research-outcome/CONSENSUS-LIVENESS-h43-COMPOSED-DIFF-c2a476a-RESEARCH-CERTIFICATION-2026-09-07.md §1.3 | — |
| `R-PTABLE-DRIFT` | HELD-IN-TENSION | Lane B1 · RE-SCOPED 2026-09-08 (cert §3.1/§8): the composition MIRRORS 22 of 24 node stages (no `*Chain` receiver), so drift inside a mirrored node body is seen by neither stage-cover arm; BOUNDED by G-D13 (a per-row sha256 of every mirrored node body, 28 digests, same-commit rule) + the v4/v5 parity oracle. Never fully closed; the bound is the gate pair | silt-reviews/research/research-outcome/FLOORBOX-STRUCTURE-ROUND-1A-COMPOSED-DIFF-869399e-RESEARCH-CERTIFICATION-2026-09-08.md | — |
| `R-MIRROR-COVERAGE-REGIME` | CLOSED-BY-BOUND | CLOSED 2026-09-08 by M-1A-3: `TestM1A3_V4V5ParityOracle` drives the eight previously undriven mirrors across nine regimes (mature epoch, launch window, reg gate, recovery boundary, de-maturation, legacy, era-3, era-4) with verdict + sentinel parity | silt-reviews/research/research-outcome/FLOORBOX-STRUCTURE-ROUND-1A-COMPOSED-DIFF-869399e-RESEARCH-CERTIFICATION-2026-09-08.md | — |
| `R-WITNESS-RESIDENT-HEAP` | ACTIONABLE | Lane B7 (flip precondition) · the BG-3 byte budget bounds WORK, not the resident-witness heap: `Box.Validate` takes the witness by value, so the measured 2.67 GiB of `AttScreens` is allocated before the check. Closer: a pull/streaming delivery seam for the resident bundle (precedent `WitnessSource.Leaf`, `SeenSetStreamWitness`) before a witness server ships | silt-reviews/research/research-outcome/FLOORBOX-STRUCTURE-ROUND-1A-COMPOSED-DIFF-869399e-RESEARCH-CERTIFICATION-2026-09-08.md §3.5 | — |
| `R-QUALIFIED-ACCELERATOR-SAFETY` | HELD-IN-TENSION | `len(qualified)` is an input to `RequiredQuorum` for a v5 block — a SAFETY quantity where it was a state-root leaf. Already gated by `core/chain/modelcheck_era4_maintenance_test.go` (the maintenance oracle); that gate must not be weakened. Owed (PE re-ruling): pin `qualifiedCount` / `liveQualifiedSet` in G-D13 — nothing today holds `qualifiedCount() == len(liveQualifiedSet())` while `len(Qualified())` feeds `RequiredQuorum` | silt-reviews/research/research-outcome/FLOORBOX-STRUCTURE-ROUND-1A-COMPOSED-DIFF-869399e-RESEARCH-CERTIFICATION-2026-09-08.md §2.7 | — |
| `R-BOX-STALLS-ON-TAKEDOWN` | ACTIONABLE | Lane D (R3.4) · a revocation-bearing v5 block STALLS on `provenView` by name (`ErrRevLogSizeUnauthenticated`, terminal not transient) until `tagRevLogSize` lands as the safety leaf; driven by G-D8. Closer: the leaf at the stamp raise | silt-reviews/research/research-outcome/FLOORBOX-STRUCTURE-ROUND-1A-COMPOSED-DIFF-869399e-RESEARCH-CERTIFICATION-2026-09-08.md | — |
| `R-COMMITTEDROOTS-EQUIVALENCE-SEAM` | HELD-IN-TENSION | Renamed from `R-STATEROOT-EQUIVALENCE-SEAM` 2026-09-08: the committed-roots predicate is an equivalence over TWO equalities (StateRoot, LogRoot) with gates, never "one implementation everywhere" (`stateview_v5.go` says so). By design, never closed | silt-reviews/research/research-outcome/FLOORBOX-STRUCTURE-ROUND-1A-COMPOSED-DIFF-869399e-RESEARCH-CERTIFICATION-2026-09-08.md §8 | — |
| `R-VIEW-FAITHFULNESS` | ACTIONABLE | Lane E3 · the `provenView` accessors are enumerable and the shape to hunt is named in `stateview_proven_v5.go`; route `resolveLeaf`, `members`, `presenceOf`, `weightedSet` to the red-team before the flip (this round's named red-team target) | silt-reviews/research/research-outcome/FLOORBOX-STRUCTURE-ROUND-1A-COMPOSED-DIFF-869399e-RESEARCH-CERTIFICATION-2026-09-08.md §8 | — |
| `R-SECOND-DOOR-COMMENT` | CLOSED-BY-BOUND | CLOSED 2026-09-08 (M-1A-2): `floorbox_v5.go`'s comment names `(*Box).Validate` as the door and forbids the parameter-taking recompute shape | silt-reviews/research/research-outcome/FLOORBOX-STRUCTURE-ROUND-1A-COMPOSED-DIFF-869399e-RESEARCH-CERTIFICATION-2026-09-08.md | — |
| `R-COMPOSITION-LEGACY-LEG` | CLOSED-BY-BOUND | CLOSED 2026-09-08: M-1 shipped — `Params` carries `MinProposerRep`/`MinAttesterRep`, `StateView.Rep` on the view, `liveView` Present / `provenView` NoWitness; `TestGD6_LegacyRepLegOracle` | silt-reviews/research/research-outcome/FLOORBOX-STRUCTURE-ROUND-1A-COMPOSED-DIFF-869399e-RESEARCH-CERTIFICATION-2026-09-08.md | — |
| `R-DRIVER-ASSERTED-BOXSTATE` | ACTIONABLE | Lane B1 Round 1B · N2/N3/N6/N7 plus the parent-pin question folded from `R-CARRIER-PARENTPROPOSER`: `AdoptPin`, `PinAdoptionInput`, `HasHeldHead` do not exist on main. Blocked on `builder/floorbox-box-entry` (re-applied file-by-file, never rebased) | silt-reviews/research/research-outcome/FLOORBOX-STRUCTURE-ROUND-1A-COMPOSED-DIFF-869399e-RESEARCH-CERTIFICATION-2026-09-08.md §8.1 | — |
| `R-GATE-PINS-BYPASS` | ACTIONABLE | Lane B1 Round 1B · blocked on `builder/floorbox-box-entry` | silt-reviews/research/research-outcome/FLOORBOX-STRUCTURE-ROUND-1A-COMPOSED-DIFF-869399e-RESEARCH-CERTIFICATION-2026-09-08.md §8.1 | — |
| `R-BOX-EXACTNESS-CITED` | ACTIONABLE | Lane B1 Round 1B · the "EXACT equality" comment lives on the box-entry branch, never on main; re-scoped to 1B | silt-reviews/research/research-outcome/FLOORBOX-STRUCTURE-ROUND-1A-COMPOSED-DIFF-869399e-RESEARCH-CERTIFICATION-2026-09-08.md §8.1 | — |
| `R-FOLD-PIN-COMPOSITE-LITERAL` | ACTIONABLE | Lane B1 Round 1B · the fold-pin's type-reachability walk does not resolve a COMPOSITE LITERAL: `liveView{s.c}.Params()` (`floorbox_box_v5.go`) yields no liveRead, so a future `liveView{s.c}.EpochSet()` in a box file is invisible (PE re-ruling 2026-09-08). Closer: resolve composite literals whose fields carry a `*Chain` in the pin's classifier, with the teeth test re-injecting that shape | silt-reviews/principle-engineer/RULING-floorbox-structure-round-1a-869399e-2026-09-08.md | — |
| `R-LANEOFF-ROTATION-RUNTIME` | ACTIONABLE | Lane TAIL · L451: "Low: the fix is structural … but the property is UNGATED at runtime". The source describes the runtime test (unwritable issuer dir + epoch crossing). Closer: Tester runtime observer (test debt, low). | L446–1775 | — |
| `R-SHORT-FINAL-STRIPE` | ACTIONABLE | Lane C5 · Builder: compute a single-chunk object's parity at the shard's TRUE length (Economist advisory rider item 2, "lower priority, more invasive") — worth the +300 % floor on sub-chunk objects (a 1 KB object stores 2,097,264 B at the 256 KiB default; erasure floor K × chunkSize = 2.5 MiB). A second content-addressing break, so it lands before economy-on or inside 4′'s window; Boulder 2 | silt-reviews/economist/ADVISORY-default-chunk-size-256KiB-2026-09-06.md | — |
| `R-GENESIS-HASH-FREEZE-SURFACE` | CLOSED-BY-BOUND | DISPOSED 2026-09-07 by the freeze manifest: the height-0 hash is OUTSIDE the era surface — it is NETWORK identity, not a consensus format; the migration rule at a format boundary is REFUSE TO START (#237, owner call 9); `TestGenesisBlockHashIsPinned` keeps every move explicit | silt-reviews/research/research-outcome/ERA4-V5-FREEZE-MANIFEST-RESEARCH-CERTIFICATION-2026-09-07.md | — |
| `R-CLOUD-ERA-PROBE` | ACTIONABLE | Lane D2 · Builder, at the R3.4 stamp raise: expose the chain's block era on a CLI/status surface (the `swarm receipt` refusal points at a `silt status` that does not exist; `chainstatus.go` prints no era) so the cloud sheet's `13b-delivery-settlement` can tell "no binding can commit (era-4 dark)" from "keys off-commitment" — today both read as the same client sentence and the row SKIPs on either (blind PE re-review `silt-reviews/principle-engineer/RULING-cloudtest-delivery-lane-flow-aab3626-2026-09-07.md`). Boulder 3 / pre-freeze carry-list | silt-reviews/principle-engineer/RULING-cloudtest-delivery-lane-flow-aab3626-2026-09-07.md | — |
| `R-H43-ROUND-LADDER-DESYNC` | ACTIONABLE | Lane A1. DISCOVERED IN THE FIELD 2026-09-07 (run `c450985-deep`, evidence `integration/cloudtest/h43-stall-evidence-c450985-deep/`): with val-d stopped, block 43 took 17 min 20 s to commit — the #432 view-change round ladders ran ~77 s apart across validators (exponential timeouts, no re-alignment) and only re-converged eventually; prior deep runs passed the same drill inside 650 s. Closer: the Tester encodes the desynchronized-ladder shape in the model-check tier (`modelcheck_i2_rounds` family) and it must go RED before any fix; the fix is a consensus-rule change (round-change catch-up / timeout re-sync) → Researcher-certified, owner-ratified. No further graded cloud run until the model-check reproduces it. Boulder 1 | L498 (evidence: integration/cloudtest/h43-stall-evidence-c450985-deep/README.md) | — |
| `R-STATEROOT-EQUIVALENCE-SEAM` | CLOSED-BY-BOUND | RENAMED 2026-09-08 → `R-COMMITTEDROOTS-EQUIVALENCE-SEAM` (two equalities, StateRoot and LogRoot). the state-root predicate stays an EQUIVALENCE WITH GATES between the node's `postApplyRoots` dry-run and the box's certified witness recompute — never one shared function; the build-plan certification forbids publishing the structure round as "one implementation everywhere" (`FLOORBOX-STRUCTURE-BUILD-PLAN-CERTIFICATION-2026-09-03.md`, "The one thing that is NOT this round") | silt-reviews/research/research-outcome/FLOORBOX-STRUCTURE-BUILD-PLAN-CERTIFICATION-2026-09-03.md | `R-COMMITTEDROOTS-EQUIVALENCE-SEAM` |
| `R-POR-SAMPLE-REGIME` | HELD-IN-TENSION | filed 2026-09-07 (the PE coined it on 4′ and had filed no row): PoR audits are exhaustive only up to 128 blocks ≈ 496 KiB (`core/node/por.go:52`, `core/por/por.go:68`); the shipped 256 KiB shard is 67 blocks so p = 1.0 today; the only guard is the `DefaultChunkSize` literal pin, which never states the PoR reason — add the reason to the pin's failure text | silt-reviews/principle-engineer/RULING-residual-register-true-up-e963034-2026-09-07.md | — |
| `R-OBJECT-UNCHECKED` | HELD-IN-TENSION | filed 2026-09-07: `SettleDelivery` reverses whatever `root` it is handed (`core/credit/deliveryanchor.go:116`); named in four certifications, no row until now; bounded by the session's own face (a wrong root mis-attributes skim within one session, never mints) — Don't #3 forbids the fetcher × object join that would check it | silt-reviews/principle-engineer/RULING-residual-register-true-up-e963034-2026-09-07.md | — |
| `R-SHARED-RULE-BLINDSPOT` | HELD-IN-TENSION | filed 2026-09-07, BEFORE the structure build starts: once a validity rule is one shared function with three callers (the `validateCarrier` shape), a defect in it is invisible to every node-vs-box equivalence gate by construction; the honest-twin (agree-direction) controls of BG-4 are the only cover — grows with `R-STRUCTURE-REDERIVATION` (Lane B1) | silt-reviews/principle-engineer/RULING-residual-register-true-up-e963034-2026-09-07.md | — |
| `R-R3-GOB-ALLOC-AMPLIFICATION` | ACTIONABLE | Lane D1 · the R3.1 PE residual, now named: a 5-byte hand-crafted gob length prefix forces a ~10 MB allocation in `proof.Unmarshal` — `SProofMax` bounds ENCODED bytes, not parse memory (`core/statehash/witness_bound.go:78`); closer: bound parse memory (a decoder limit) in the freeze train, Boulder 3 | silt-reviews/principle-engineer/RULING-residual-register-true-up-e963034-2026-09-07.md | — |
| `R-R31-PREFIX-PANIC` | HELD-IN-TENSION | filed 2026-09-07: the 33-byte-proof `checkPrefix` panic is guarded at both call sites (G-R31-1/-2, #749) but the SMT library still panics on that input and no `recover()` exists in non-test `core/`; held until the library pin (go.mod v1.0.0) is bumped past a fixed release | silt-reviews/principle-engineer/RULING-residual-register-true-up-e963034-2026-09-07.md | — |
| `R-R31-SI2-WIDTH-NOTE` | CLOSED-BY-BOUND | closed by restatement in `docs/design/state-root-domain-separation.md` (the prefix-free leading byte is NOT fixed width — the disjoint-preimage argument does not need it); its lesson is that the register lint never saw an `R-` name in `docs/`, so `check_residual_register.py` now scans `docs/design/` too (see the filing rule) | silt-reviews/principle-engineer/RULING-residual-register-true-up-e963034-2026-09-07.md | — |
| `R-ERA4-NEEDS-MATURITY` | HELD-IN-TENSION | disclosed 2026-09-07 by the freeze manifest: the era-4 readiness tally sits behind `everMature`, so a network that never latches maturity never activates era-4 — the correct safety-first shape (no activation override exists in any binary, ratified); disclosed to the B8 brief | silt-reviews/research/research-outcome/ERA4-V5-FREEZE-MANIFEST-RESEARCH-CERTIFICATION-2026-09-07.md | — |
| `R-CORRUPT-PROVIDER-BANDWIDTH` | HELD-IN-TENSION | filed 2026-09-07 in place of the discharged parity-amplification worst case: a CORRUPTING provider still forces the fetcher's total download to `S·N/K` because `fetchFrom` transfers before it verifies — a fetcher-bandwidth cost, not a per-server draw, so it does not touch the `grant/r` pin; bounded by the provider count and closable by verify-before-transfer streaming | silt-reviews/research/research-outcome/BOULDER2-residual-closures-bounty-truncation-lambda-dust-relay-anon-refuse-self-spend-parity-estimand-RESEARCH-CERTIFICATION-2026-09-07.md | — |


These are the off-critical-path residuals migrated from the (retiring) GitHub issue tracker.
They do NOT gate the Boulder spine; they are the honest tail. Each carries its provenance
issue number as an anchor only. Repro recipes for the field defects (#558/#535/#530/#574/#586/#277)
live in [`docs/thinking/2026-09-01-residual-defect-repro-recipes.md`](docs/thinking/2026-09-01-residual-defect-repro-recipes.md).

**Security / data-safety residuals:**
- **`R-DEFAULT-CHUNK-BOUNTY-ZERO` + `R-MANIFEST-PADDING` — CLOSED, MERGED 2026-09-07 (#765, decision 4′).** The publish default is 262,144 B and a manifest that fits one chunk is framed at its own length + 8 (one content-addressing break, crossed once; the NEW genesis `f428d0a8…0951` accepted by the owner — no live network). The geometry premise was corrected first (#761: a shard is a whole ciphertext chunk, so the old default paid a base of 2, not 0). What remains from this family: **`R-BOUNTY-TRUNCATION`** (the bounty floor has no accumulator; `-chunk-size 65536` under-pays 20 % silently — Researcher, Lane C5), **`R-SHORT-FINAL-STRIPE`** (parity at the shard's true length; +300 % floor on sub-chunk objects — Builder, Lane C5), and `R-LAMBDA-DUST′` (Lane C5). Sources: `/Users/andrewedmond/Claude/claude/silt-reviews/economist/ADVISORY-default-chunk-size-256KiB-2026-09-06.md`, `/Users/andrewedmond/Claude/claude/silt-reviews/principle-engineer/RULING-default-chunk-256k-manifest-framing-b365f10-2026-09-07.md`, `docs/thinking/2026-09-07-default-chunk-256k-manifest-framing.md`.
- **G-R212-7 residuals (Researcher, 2026-09-06).** `R-NUMERAIRE-SANDWICH` — `Dλ` is squeezed between parity (below) and Don't #7 against the relay price (above); one knob, two opposite-side constraints, not separable; held in tension. `R-BOUNTY-BASE-DENOMINATION` — BUILT (#758): `RepairBountyBase = c·k·shardBytes/(U/p)` moves with `λ` (had it not, the D-S7 threshold would have gone 24 → 12.6 M). `R-AUDIT-REWARD-DOMINATES` — at `Dλ = 393,216` one passed audit (1,000 credits) equals 375 MiB of gross unwitnessed serving; inverts the cheapest credit source; not a Sybil surface (`por.go` skips self). `R-LAMBDA-WASH-MINT` — no `λ` closes the loopback wash; the advantage ratio is `loopback_BW/uplink_BW`, only the absolute rate falls (393,216×). Source: `/Users/andrewedmond/Claude/claude/silt-reviews/research/research-outcome/G-R212-7-lambda-redenomination-RESEARCH-CERTIFICATION-2026-09-06.md`.
- **`R-RELAY-ANON-SET′` — the relay re-price moves the relay anonymity residual BOTH ways (blind PE N-6, 2026-09-06; research-gated).** `k_max = 1` dissolves the by-`k` partition of a relay's buyers, but guard (ii) rotates one ephemeral per session and the session ceiling rose 24.4× (1 GiB → 24.4 GiB), so ephemeral rotation per relayed byte falls 24.4×. The G-R212-2 certification's "IMPROVED" reading (§1.3/§6) is one-sided; the Researcher owes the two-sided statement before anything cites the residual as closed. Source: `/Users/andrewedmond/Claude/claude/silt-reviews/principle-engineer/RULING-G-R212-2-relay-reprice-code-2026-09-06.md`.
- **Demand issuer-key proof-of-possession (DSKS) — R0.4b-PoP.** `validateIssuerKeys`
  (`core/chain/issuerkey.go`) requires a verifying ed25519 self-signature, an in-range epoch and
  a bond, but **no proof that the registrant holds the RSA private key** whose fingerprint it
  registers. Public keys are served publicly, so bonded issuer B can register issuer A's
  fingerprint for epoch E; a redeemer resolving against B then pins A's key, and a token A
  signed verifies under B's keyset — Duplicate-Signature Key Selection (Blake-Wilson & Menezes
  1999). **Latent, not live:** `handleDeliveryReceipt` resolves ONE configured issuer and ledgers
  are per-node, so the multi-issuer surface is not exercised today. The close is either a PoP in
  the registration, or the faithful RFC 9578 binding `keyFingerprint(32) ‖ epoch(8) ‖ serial` in
  `demandMsg`. Both are **validity-rule changes: research-gate + owner ratification, BEFORE the
  stamp raise.** Source: crypto-specialist advisory C-4,
  `/Users/andrewedmond/Claude/claude/silt-reviews/crypto-specialist/ADVISORY-R0.4b-C3-blind-RSA-epoch-binding-2026-09-03.md`.
- **FDH domain-separation-tag length prefix + 128-bit reduction slack — R0.4b-FDH.** The three
  FDH domains are not length-prefixed and `fullDomainHashD` expands to only `nLen + 8` bytes
  (64 spare bits against RFC 9380 §5.2's `k = 128`). Both are sound as built — the crypto seat
  verified the three domain constants differ at byte index 10, so no message produces a
  cross-domain collision — but sound *by accident of the constants*. **Not fixed in R0.4b C3
  because either change alters the FDH output, and the publish and credit domains are BYTE-FROZEN
  against chain replay:** committed publish tokens re-verify on every replay, so changing their
  signed bytes invalidates history. Fixing only the demand domain would leave two conventions in
  one file. Do it as one versioned change, with a chain-era gate. Source: advisory C-8.
- **Blinding-factor sampling: mod-reduction, not rejection sampling — R0.4b-BLIND-SAMPLING.**
  `blindtoken.randInt` draws the blinding factor `r` (and the issuer-side blind `u`) by reducing
  `(bitlen(N) + 64)` random bits mod `N`. RFC 9474 §4.2 states a **MUST**: *"The blinding factor r
  MUST be randomly chosen from a uniform distribution. This is typically done via rejection
  sampling."* silt meets it only **statistically** — the distribution is within `2^-64` of uniform.
  **Declared, not fixed, in R0.4b C3:** it is a conformance gap rather than a break (no use of a
  `2^-64` bias is known), and it changes no committed byte, so it neither blocks the close nor
  should slip in unannounced. Work: rejection-sample into `[1, N)` and **bound the retry** — the
  two loops calling `randInt` (`blindD`, `SignBlinded`) currently spin forever on a reader that
  yields zeros. Declared at `core/blindtoken/blindtoken.go` (`randInt`) and in
  `docs/thinking/2026-09-02-r0.4b-c3-close-design.md` §12. Source: crypto advisory R4,
  `/Users/andrewedmond/Claude/claude/silt-reviews/crypto-specialist/ADVISORY-R0.4b-C3-crypto-items-as-built-01bf8e9-2026-09-03.md`.
- **Transport authentication (TLS or Noise) — #437.** **POST-RC (`1.x`) by scope call S4, 2026-09-07 (`D-RC-SCOPE-S2-S4`).** silt's wire is unauthenticated CBOR
  (`adapters/tcpnet/wire.go`), so an on-path MITM can strip certificate signatures in transit.
  Certified NOT a safety break and NOT a wedge (stripped blocks fail `ValidateCommit`, are inert,
  never drive round-advance; only verifying sigs count; ≥⌊A/2⌋+1 full-certificate holders re-serve
  over any honest path; residual = censorship over a controlled link, already tolerated by the
  liveness model). Work: wire authentication/integrity at the transport layer — a pre-existing
  transport property, orthogonal to consensus. Consult `docs/network-durability.md` before design
  (build-immutable #5); research-gate the crypto choice. Cert:
  `432-proposer-prepare-required-RESEARCH-CERTIFICATION-2026-08-16.md` §5.2.
- **Crash-safety: torn `chain.cbor` → silent genesis fallback — #558.** **CLOSED 2026-09-07 (Lane B8, scope call S3): fsync'd atomic write + refuse-to-start unless `-accept-chain-loss` (which preserves the original).** Attribution corrected by the PE: the field event was an INTACT file an era-2 replay bug rejected (`core/chain/reload_era2_558_test.go`), not a torn write. History as first written: a SIGKILL mid-persist
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
- **Silent-behavior observability — #235 — (1) and (2) DONE on main (trued up 2026-09-06); (3) OPEN with #237.**
  (1) every healthy sweep now logs `repair sweep complete` at info with shard/reachable counts and a `repair pass
  complete` summary (`core/node/repair.go`, `finishSweep`); (2) `-revoke <root>` prints `revoke: target … waiting until it
  is committed on-chain …`, then `… is committed — gathering a takedown quorum`, then `takedown: proposed …`
  (`cmd/silt/daemon.go`, the `-revoke` block, which cites #235); (3) rolling upgrade across a format boundary still
  degrades with a one-line stderr notice — carried under #237.

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
- **e2e paid-delivery-lane fixture is `-objective=false` — R-E2E-ERA4-FIXTURE.** The e2e
  delivery-receipt daemon runs `-objective=false`, so `chain.objective()` is false, so
  `epochsEnabled()` is false, so `apply()` never calls `rotateEpoch` — and the era-4 readiness
  tally lives inside it. **The tally can therefore never latch on that fixture, at any readiness
  stamp, on any binary**, so the paid lane's POSITIVE arm has no e2e coverage:
  `TestPaidDeliveryLaneRefusesWithoutACommittedKeyBinding` asserts the certified refusal
  instead, and `sim TestPaidDeliveryLaneThreeCallComposition` + `core/node
  TestRTC3_RestartDoesNotRePayTheSameWireReceipt` carry the positive arm below e2e. **This is a
  PREREQUISITE of the stamp-raising release, not of the R0.4b merge:** restore the e2e positive
  arm by UPGRADING THE FIXTURE to objective + bonded + epoch-enabled (a topology that reaches
  the `everMature` latch), **never** by exposing `Config.Era4ActivationHeight` to a harness in
  any form — that is the one branch that skips every readiness predicate the tally embodies.
  The trace is pinned by `core/chain TestGateF_NonObjectiveTopologyCanNeverLatchEra4`. Source:
  G-8 convergence,
  `/Users/andrewedmond/Claude/claude/silt-reviews/research/research-outcome/R0.4b-C3-G8-dark-lane-CONVERGENCE-2026-09-03.md`.
- **LANE-OFF rotation disarm has no runtime observer — R-LANEOFF-ROTATION-RUNTIME.** That no
  demand-key rotation goroutine RUNS after a failed boot install is pinned only by a source-ORDER
  gate (`cmd/silt TestDaemonArmsTheRotatorOnlyAfterABootInstall`). Reaching the branch in a real
  daemon needs an unwritable issuer directory whose publish-token key already exists; OBSERVING
  the difference additionally needs the chain to cross an epoch boundary, which on a lone
  validator needs a driven publish. Low: the fix is structural (the single assignment sits below
  the single failure exit), so there is no branch left to regress into — but the property is
  UNGATED at runtime and says so in the gate's own failure text. Source: PE ruling H-2,
  `/Users/andrewedmond/Claude/claude/silt-reviews/principle-engineer/RULING-R0.4b-C3-close-271ab81-final-2026-09-03.md`.
- **A bare `go test ./...` has no margin against the default package timeout — ✅ DONE 2026-09-06 (branch
  `docs/full-suite-timeout-margin`): `go test -timeout 40m ./...` is the documented full-suite command (README,
  CONTRIBUTING, `docs/v1-test.md`) and `release.yml`'s sanity step carries the same `-timeout`; the measurement is
  untouched; the blind PE re-measured the package at 601 s on 2026-09-06 (killed at Go's 10-minute default).**
  `core/chain TestMeasureRecomputeMatureNowStreamingWin` is a long MEASUREMENT test, and it
  puts the whole `core/chain` package close to Go's 10-minute default. Measured 2026-09-03 on
  this branch's tree: the single test **309 s**, the package **530 s** with an explicit
  `-timeout 40m`; a bare `go test ./...` on `origin/main` `2247235` was reported timing out in
  `core/chain` (~7.5 min for the same test on that run). Nothing is broken — the test has an
  always-on fast structural twin (`..._Structural`) and is skipped under `-short`, which is what
  CI runs — but the documented full-suite command has no margin, and load decides whether it
  passes. **Work:** document `go test -timeout 40m ./...` as the full-suite command, or move the
  measurement behind an opt-in build tag. Do NOT simply shorten the measurement — the number is
  the artifact. Untouched here: it is not this branch's test, and editing another branch's
  measurement inside a receipt-expiry commit is how a regression hides.
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
- **Per-stripe parity fetch — MERGED 2026-09-06 (#751).** NetGet's parity fallback is a
  DEFICIT walk (per stripe, real data shards − present; one parity column consulted at a time; early exit) instead of
  every parity column of the whole object; an honest or withholding provider can no longer force the object-size
  term of `R-PARITY-AMPLIFICATION`. **NOT claimed discharged** (research-gated; a corrupting provider still forces
  `S·N/K` because `fetchFrom` transfers before it verifies — the 64 GiB pin's worst case stands). Blind PE design
  ruling `RULING-parity-fetch-per-stripe-design-2026-09-06.md` chose (A′) over the record's (A) (6× over-fetch, 1.5×
  adversarial ceiling). **New residual `R-SPARSE-COLUMN-PROVIDER`:** `NetGetRetain` retainers become 1-of-T holders of
  parity columns; `probeShard` walks providers sequentially and corpse gating does not skip live nodes; #500 unchanged.
  Gates G-PS-1…6 and G-PS-8 (there is no uncoded gate — `K == 0` is unreachable from any publish path); two counters
  `stats.ParityColumnLookups` / `stats.ParityShardsPulled`. **Two residuals from the blind PE code review:**
  `R-PS-PRESENCE-COST` — the verified presence check (Get + Verify per data shard, measured 46 ms per 64 MiB chunk
  vs 2.5 µs for a stat; ~21 s at 30 GB) is taken for correctness; the pay-on-failure redesign (trust the stat, verify
  only when the pipeline fails) is the cheaper shape, unbuilt. `R-PS-LOCAL-ROT-NOT-HEALED` — a bit-rotten local shard is
  routed around every retrieval but never replaced (`fetchFrom`/`FetchChunk` stat-gate; the working-set snapshot
  counts it as held), so the parity detour recurs on every NetGet of that object.
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

**Consensus-touching residuals — RE-HOMED 2026-09-07 to Lane A (the consensus-liveness certification):**
- **Objective-mode `Config.Quorum` floor divergence — #380 · Lane A2.** In objective mode `RequiredQuorum()` returns the LOCAL `cfg.Quorum` verbatim in the mature regime, so two honest replicas with different `-quorum` compute different validity for the same committed block (`Reconcile` strands the stricter one at genesis — the SYBILS=8 field GAP, #338) AND, found 2026-09-07, `SupportMeetsQuorum` gates `newViewFor`, so a divergent floor is a permanently dead designee — a liveness defect. Workaround: a uniform `-quorum` across the swarm (`TestDivergentQuorumFloorStrandsSyncingNode338`). Direction (1) — ignore the local floor in `ValidateCommit`, keep `Quorum` as a proposer-side gather target — is GATED on G-H43-8 and the owner's ratification (owner call 20). A consensus-rule change (I1); do NOT assert resolved.
- **Mature-regime PUBLISH starvation — #441 · RETIRED as a separate row 2026-09-07; it is Lane A1's root.** `core/node/rounds.go:306` IS the shipped #441 arming clause (the round clock arms on local mempool content); the certified fix (A) closes both. Attribution: `docs/thinking/2026-08-16-run3-mature-handoff-passed-publish-starvation-found.md`; the certification: `/Users/andrewedmond/Claude/claude/silt-reviews/research/research-outcome/CONSENSUS-LIVENESS-h43-round-ladder-desync-441-380-RESEARCH-CERTIFICATION-2026-09-07.md`.

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
