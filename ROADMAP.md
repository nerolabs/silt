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
decisions are folded in (R0.2, R1.7, R2.3, R2.5, R2.6, R3.4).

**Two whole-board sequencing constraints:**
1. **Nothing turns the economy on over a live mint.** Boulder 0 (A4) precedes Boulder 2.
   Boulder 0 is now DONE + MERGED (PR #686); the R0.4 conservation re-cert cleared before any
   economy default flip.
2. **The accept-flip does not close until its whole witness-soundness spine is green and its
   external pass clears.** The flip (R1.8) is a consensus-rule change (I1); the old "single
   remaining step" framing hid 7 predecessors — see the flag under *Superseded Rocks*.

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

**Decisions to make next session (Boulder 0 residuals):**
- **RT-DELIV-3 · delivery-credit `provKey` omits the server.** A latent conservation break
  reachable only in the shared-ledger *sim* (all serves share one ledger), NOT in per-node
  prod (each node's ledger has a single server identity). **Decide:** fix now — add the server
  to `provKey`, which changes the conserved-lane key shape and therefore **re-opens the R0.4
  conservation cert** — vs track it as a sim-only residual.
- **R0.4b · receipt-expiry (the full (b)-prunable form) — PARKED, evidence-gated.** Re-open
  when the unwitnessed-bilateral under-pay tail bites: **>8192 un-redeemed lanes on one node**.
  Two owner calls gate the build first (per the R0.4b scoping cert,
  `R0.4-receipt-expiry-scoping-RESEARCH-CERTIFICATION-2026-09-01.md`): (1) the
  **privacy-safe shape** — epoch-granular / serial-indexed, NEVER a wall-clock `NotAfter`
  (which adds a timing quasi-identifier on the D3/H8 channel); and (2) a **TTL-bounds
  measurement** (the honest-pony redemption-latency tail at the >8192-lane regime is
  unmeasured — no value may be pinned at desk).

#### Boulder 1 — Make the accept-flip safe (floor-box witness-soundness spine) · was Rock 1
Root cause (verified, all seats): classes P/A/B take witness values/screens as `NewValue`s or
branch predicates without `Resolve`-ing against `prevStateRoot`; the attacker controls the
committed root too, so `postRoot==StateRoot` holds by construction. Sound classes route witness
values as VerifyProof'd `OldValue`s — the fix mirrors them. Design:
[`docs/thinking/2026-09-01-floorbox-witness-soundness-fix-design.md`](docs/thinking/2026-09-01-floorbox-witness-soundness-fix-design.md).

**Status (2026-09-01): R1.0 / R1.1 / R1.2 DONE; R1.3 REFUTED; R1.4 CERTIFIED. The
accept-flip (R1.8) is NOT done** — it now has four additional named preconditions (below the
increment list). The recompute-soundness increment merged as a standalone **never-Accept**
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
- **R1.5 · Accept-flip model-check exercising the NEW Resolve path** · Tester · model-check.
  Absorbs old Rock 4 (#406). NOT done.
- **R1.6 · Oracle coverage: 23 predicate fields each get an adversarial Resolve-path probe;
  extend probeUncovered to name A1/A2/A3** · Tester. Also a freeze gate (Boulder 3). NOT done.
- **R1.7 · External red-team pass (B8) — DECISION RATIFIED: the external pass is a HARD
  precondition of the flip.** Owner milestone-call + external seat. The recompute's OUTPUT is
  C2's INPUT; internal cert cannot close "no adversary forges a witness the recompute accepts."
  Attack the FIXED artifact (R1.1–R1.4 have landed). Model-check the Resolve-every-value class
  under adversarial scheduling first, to convert the external pass from discovery to bounded
  confirmation.
- **R1.8 · The flip: wire `WitnessValidateV5` → Accept-iff-all-predicates-pass** · Builder · S.
  **NOT done.** Trivial code; a consensus-rule change (I1). Beyond R1.5/R1.6 green, the flip
  **additionally requires** (per the R1.4 cert §R1.4-FLIP):
  - **R-membership** — OPEN: a set-size bound on the qualified / `validatorsSeen` sets;
  - the **EXTERNAL B8 red-team pass** (R1.7) — owner-ratified HARD precondition;
  - the **recovery-boundary decision** (cold-auditor directive-trust boundary) — repro/residual
    in [`docs/thinking/2026-09-01-residual-defect-repro-recipes.md`](docs/thinking/2026-09-01-residual-defect-repro-recipes.md)
    (formerly #535);
  - the **legacy-mode invariant** (the pre-v5 path stays sound under the flip).
  **Decoupled from the era-4/v5 freeze** (R3.4): the flip proceeds pre-freeze;
  `WitnessValidateV5` changes no committed format field, so the freeze re-confirms
  byte-identity later, at RC.

**Carried residuals (worth a line, owed before the flip):**
- **R-CARRIER-REFLECTION** — 7 fold-input carriers are verified by hand; a **reflection pin**
  on those carriers is owed before R1.8 (so a future added carrier cannot slip the table
  silently).
- **R-ROTATE-EPOCH-LAST — pin `rotateEpoch`-is-last-in-`apply` as load-bearing for `epochSet`
  order-independence (#621).** Distinct from R-CARRIER-REFLECTION. `epochSet` order-independence
  (proven in #620) holds only because `rotateEpoch` runs LAST in `apply` (`core/chain/chain.go`),
  reading only the final post-block `bonded`/`slashed` state, so `epochSet =
  liveQualifiedSet(bonded, slashed)` is order-invariant by construction. A refactor that moves
  `rotateEpoch` before slash/bond application, or makes `liveQualifiedSet` read history rather than
  the two final maps, would silently break the SMT history-independence premise for `epochSet` and
  #620 would no longer hold. Owed before the era-4 format freeze (Boulder 3): a drift-guard
  assertion or test that fails if the freeze can observe pre-final state. PE ruling:
  `RULING-620-mature-epoch-order-independence-2026-08-28.md` ("Couplings the consult should carry
  forward").
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
- **R3.4 · era-4/v5 format freeze — DECISION RATIFIED: deferred to the RELEASE CANDIDATE (not
  end-of-PoD), and DECOUPLED from the flip.** `WitnessValidateV5` changes no committed format
  field, so the flip (R1.8) proceeds pre-freeze; the freeze then re-confirms byte-identity of
  the recompute at RC. Owner-call, after the field set settles (R1.4) + domain-sep owned
  (R3.1). The prior "freeze at end-of-PoD" framing is superseded.

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
  close gate.** This is the RC-defining gate. A fresh, no-memory EXTERNAL red team (self-graded
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
