# Silt Roadmap

> **Source of truth.** *What M0 asserts and why* lives in
> [`docs/design/m0.md`](docs/design/m0.md) (the composition spec). *What the owner
> has decided* lives in [`docs/decisions.md`](docs/decisions.md). *The destination*
> is [`docs/TENETS.md`](docs/TENETS.md). This file is the **narrative path**: where
> we are, what the forward tracks are, and why they're ordered the way they are. It
> is not a tracker — live state is the GitHub **`V1` milestone** and its issues.
>
> The earlier **Gate 0→6 spine** (with Gate 4 as "the M0 mechanism to build") is
> **retired**: that mechanism is built and the mission was reframed (below). The
> `v0.1.x`/`0.2.x` tags are **experimental / learning releases**, not steps on the
> march to V1; that history lives in `docs/buildlog/`.

## Tenets are the destination; this roadmap is the path

**V1 is defined by the tenets, satisfied and field-proven — not by a feature
list.** Every track below advances one or more tenets; if a step serves no tenet,
it doesn't belong here. The relationship is one-way: **tenets guide the roadmap.**
A tenet gates V1 as a *principle*, never a *mechanism* — with one deliberate
exception, **M0** (the mission itself), whose *real* mechanism is in V1 by
definition. **Release is gated by proof (R1):** a tenet is "met" only when
field-proven multi-machine, not sim- or single-host-only.

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

## The ordered path (the V1 roadmap — ratified 2026-08-19, D-M1-PIVOT)

**Why this section exists.** "Tenets are the roadmap" proved too loose: it let a month
of effort pool on one axis (consensus correctness / memory survival — necessary, and
now complete and verified) while the other axis (the S7 economy, bandwidth pricing, the
operational floor) received none. The 2026-08-19 fresh-eyes audit
([`docs/thinking/2026-08-19-fresh-eyes-audit-and-the-m1-pivot.md`](docs/thinking/2026-08-19-fresh-eyes-audit-and-the-m1-pivot.md))
found the trust plane verifying end-to-end but the durability economy **built,
test-proven, and switched off in every shipped node** — a network on track to earn a
security certificate without a demonstrated reason to exist at equilibrium. So the
tenets stay the *destination*; **this ordered path is the track**, and work is expected
to follow it top-down. The prior rule "M1 opens only after the M0 gate" is superseded
([`docs/decisions.md`](docs/decisions.md) D-M1-PIVOT): the M0 tail is small and
enumerated, and both the deep field confirmation and a valid #183 depend on M1 being
real.

1. **Phase 1 — Close the M0 tail (small, enumerated). ✅ COMPLETE (2026-08-19).**
   *(1.1 inbound-cap: resolved — v2b shelved to owned-residual E5 on the drain measurement
   (cap/drain ≈ 0.21s at 1227 MB/s real drain, well under the 2s bound); the publish-flood 502 was a
   separate Care/NetGet loop self-deadlock, fixed (#474). 1.2 CPU gate: shipped (#476). 1.3 evidence
   hygiene: RSS/heap telemetry shipped + field-validated over two runs (#478). 1.4 deep >h64 run:
   deferred to Phase 3's exit gate by owner ruling — heights too expensive to accrue is Phase 3's
   justification.)*
   1. Inbound-cap hardening — **resolved-for-now 2026-08-19, sequenced behind 1.2 (two
      PE rulings).** Per-peer fairness: shipped (v2a). The consensus-priority lane: a
      timed drill proved the starvation lives in the loop's FIFO **drain**, not gate
      admission (an admission reserve alone is insufficient), and its severe regime is
      the bond-reg/VDF slow-drain — Phase 1.2's domain. Filed as owned residual **E5**
      with the reach-recipe and the go/no-go (measure the real drain rate as a rider on
      1.2, re-run the parked drill `drill/v2b-gate-starvation`; only a still-RED re-run
      builds the two-class drain). The `-inbound-cap` two-axis sizing note shipped in
      the flag help. *(Also corrected: the field publish-flood 502 was a Care/NetGet
      event-loop self-deadlock — cap-independent, fixed separately; see
      [docs/thinking/2026-08-19-publish-502-attribution-care-self-deadlock.md](docs/thinking/2026-08-19-publish-502-attribution-care-self-deadlock.md)
      and the drill record
      [docs/thinking/2026-08-19-v2b-gate-starvation-drill-design.md](docs/thinking/2026-08-19-v2b-gate-starvation-drill-design.md).)*
   2. The `MsgSubmitBondReg` CPU gate (pre-#183 DoS floor) — **now also carries the E5
      rider:** measure the real saturation drain rate at the shipped 256M cap during
      this item's validation, and re-run the parked v2b drill re-parameterized to it
      (the go/no-go for the two-class drain).
   3. **Evidence hygiene:** commit the field-run artifacts; add RSS/heap telemetry to
      `integration/cloudtest` so memory claims carry a citable, in-repo artifact
      (build-immutable #7 applied to our own headlines).
   4. Attempt the deep confirming run (longer soak; retention prune engaged past h64 at
      production parameters). If heights starve the run's budget, that measurement is
      Phase 3's justification — record it, don't force it.
2. **Phase 2 — Economy-ON (the S7 keystone; enablement, not construction). ✅ COMPLETE
   (2026-08-21): Slices 1–4 DONE — the exit gate CLOSED on the wire by the clean sheet
   `f35a0f9-18198` (24 pass / 0 gap / 0 fail; liveness precondition HELD).**
   - **Slice 1 (enable) DONE:** `-economy` flag (opt-in, default OFF); the bounty base is a protocol
     price `credit.RepairBountyBase = c·(k·shardBytes)`, **`c=1` research-certified**; payee =
     **(a-domain-fresh)** — the paramedic keeps the shard it rebuilt iff its own failure domain is
     unused by the stripe (funds the reconstructor without reducing S2 dispersal); Invariant A held
     (bounty pays balance, never standing), failing-first guards green.
   - **Slice 2 (telemetry) DONE:** `/api/status` durability block (balance, bountyOn, per-object
     reserve/funded/paid/repairs/horizon). **Slice 3 (endowment) DONE:** `POST /api/fund`.
   - **§0.1 (repair RAM) MEASURED locally** (`core/erasure/reconstruct_mem_test.go`): a prod 64 MiB
     stripe reconstructs **1.0 GiB resident** → OOMs a 2 GB box, so the economy grade uses 256 KiB
     chunks (repair ≈ 2.5 MiB). *A prod-chunk field-confirm on a bigger box is a later option.*
   - **Slice 4 (economy on the wire) = THE EXIT GATE, CLOSED (2026-08-21).** The gate row first
     went green on `fa501cc-56689` (paid=432770; ledger identity prepay+skim=paid, reserve→0) but
     that sheet was PROVISIONAL on its island OOM liveness FAIL (#503). The #503 bond-renewal storm
     was research-certified and fixed same-day (PR #508), and the clean re-run **`f35a0f9-18198`
     banked the sheet: 24 pass / 0 gap / 0 fail, liveness precondition HELD, islands flat at
     0.34–0.38 GiB peak** (was 1.5–1.6 GiB + OOM×3). Economy on that run: paid=531080 over 1
     repair, identity closed, post-payout skim already refilling the reserve. The g-series now has
     three samples (432770 cloud / 629390 LOCAL / 531080 cloud). Exit gate met: prepay→(skim)→bounty
     closes on a real network; standing coin-free.
3. **Phase 3 — Cheap heights (the M1 lever with the M0 dividend). ✅ COMPLETE
   (2026-08-26): the exit gate CLOSED on the wire by the deep sheet `fe2376a-deep`
   (30 pass / 1 gap / 0 fail; the gap is a harness observer-arming question, #586).**
   The gate row: `12-deep-heights` drove h78→**h132** (target 128) at **~48 s/height**
   (was ~390 s at the arc's start), the #549 Q4 stabilization barrier cleared in 215 s,
   **the retention prune ENGAGED on every validator at depth** (12b: 59 payload-stripped
   blocks each, horizon h64) and the pruned chain **converged** (12c, h134+, shared
   head). Worst RSS 0.65 GiB; zero OOM; every safety/adversarial row green; the economy
   closed on the wire for the third consecutive sheet. The month of depth findings that
   gated this — #528 knee, #535 boundary recovery, #549 view-sync, #555 hash-work,
   #558 era-replay, #561 escape-walk, #563 memory, #572 restore under-latch — is
   entirely fixed, each with a failing-first local RED home, and the last three runs
   field-confirmed the stack (evidence PRs #564/#575/#579/#584/#585). *Deliberately
   NOT built:* the #299 near-tier proof compression — the gate was met on prune +
   the fixed depth costs alone, so proof-size work stays demand-driven (revisit if a
   future regime pushes per-height cost back up; the parked sealing re-architecture
   stays research-gated). *Gate clause owed:* the original gate also asked for the
   360 s publish bound re-derived **downward** — not yet done (heights got cheap enough
   without it; at ~48 s/height the computed windows in `scenarios.sh` now have real
   slack). The re-derivation is a small, evidence-rich follow-up: derive from the
   fe2376a-deep measured cadence, don't guess (#549-Q3 discipline).
4. **Phase 4 — Proof-of-Delivery (price bandwidth).** Storage is priced; bandwidth is
   not, so an open relay/gateway is a free-rider choke (the recentralization failure
   mode). Spec + research consult **first**; the hard prerequisite is closing the
   receipt-forgeability residual (a demand receipt is currently mintable with zero
   object bytes — inert only while demand has no consumer). Wash re-priced per
   D-DEMAND. **Firewall immutable:** delivery credits fund durability and relay
   compensation, never consensus standing (γ→1/N, #182).
5. **Phase 5 — The operational floor (a node a person can run).** Per-platform service
   packaging (launchd / systemd / Windows service) + signed installers, and
   operator-consented self-update per R4 (signed manifests, never silent — a working
   downstream macOS signed+notarized+launchd reference exists and generalizes into
   silt). Incremental O(delta) proof maturation (kill the O(store) restart scan);
   reprovide dirty-tracking (kill the O(held) per-interval re-sign). Exit gate: a
   non-developer installs a node that survives reboot and returns to serving in
   seconds, and steady-state cost no longer scales with the whole held set (S6).
6. **Phase 6 — External red team (#183) → V1 RC.** The engagement runs against the
   **economy-ON** configuration — the network people will actually run. Then R1: a
   fully green multi-region grade on the RC config, and V1.

### Immediate next work (updated 2026-08-26 — post-Phase-3 priority order)

Phases 1–3 are banked. The depth war is over: `fe2376a-deep` met the Phase 3 gate
(h132, prune engaged, 0 fail) and closed the whole stall lineage (#549/#560/#561/
#572/#573). #183's close condition is MET and the issue carries the evidence — the
close itself is the owner's call and is deliberately HELD. The current order:

1. **Phase 4 opening move — ✅ DONE (2026-08-26): the PoD spec shipped
   ([design/pod.md](docs/design/pod.md)) and research CERTIFIED it same day**
   (`silt-reviews/research/research-outcome/PoD-neutral-lane-B3-close-RESEARCH-CERTIFICATION-2026-08-26.md`):
   the conservation close is sound (the only structural wash defense), with three
   folded amendments — the supersede rule over the `RecordServe` self-mint is
   load-bearing, the PoR leg is dropped in the neutral lane, and strong-form
   Camenisch–Shoup is not adoptable (quorum-TTP/VSS is the route if ever). **The
   state-root keystone consult is ALSO CERTIFIED**
   (`silt-reviews/research/research-outcome/D-TIERING-state-root-keystone-RESEARCH-CERTIFICATION-2026-08-26.md`):
   compact SMT over the set-valued state PLUS a separate append-only root for the
   transparency log (refined 2026-08-27 by the #597 certification), era-3 rides the #506 version-gate as
   tenant #2, rebuild-at-boot, self-checkpoint closes #559's crash-reboot case —
   with three load-bearing obligations (snapshot-boot-equivalence oracle proves
   field completeness; incremental-cost oracle; era-2→3 Reload test ships ahead).
2. **The PoD neutral-lane BUILD** (per the certified [design/pod.md](docs/design/pod.md)
   §7): firewall failing-first test → supersede rule + no-PoR receipt →
   `EnableDemandBank` + the conserved balance-lane consumer → the D-TIERING mode
   flags (`--serve-content`/`--archive`). Owner knobs held: skim burn-vs-escrow
   (escrow leaned), the relay dispute-TTP question (deferred with relay).
3. **The state-root keystone BUILD track** (larger; sequence against 2): SMT
   library call (`pokt-network/smt` closest-proof shape vs JMT port), the three
   certified oracles as failing-first RED homes, era-3 gate as #506 tenant #2.
   Owner/builder calls held: lifetime-owner vs TTL-lapse for `bondRootOwner`
   (both C2-sound), rebuild-vs-persist (a build-immutable-#8 measurement).
4. **The publish-bound re-derivation** (the owed Phase 3 gate clause — derive downward
   from fe2376a-deep's measured ~48 s/height cadence).
5. **#586** — the economy sheet's one open row (skim-observer arming under per-node
   ledgers; harness-vs-product question filed on the issue).
6. **Standing tail:** #559 (common crash-reboot case now closed by the keystone
   cert's Q7 self-checkpoint; true-loss residual stays operator-anchored), #583
   (e2e flake watch, 3rd occurrence = journal attribution), #574 thread 2 (drill
   quarantine design), #530 (Docker e2e pre-genesis stall), Phase 5 scoping.

The pre-gate order below is retained for context:

1. **Quota gate (external, check FIRST each session):** the us-west1 `IN_USE_ADDRESSES`
   8→16 preference. When it lands, the **SYBILS=8 + MATURING=1 coverage run** becomes
   possible — the two standing structural SKIPs on every sheet (`5-sybil-no-capture`,
   `10-maturing-handoff`), and the handoff/post-shed regime is the external red team's
   sharpest seam-#8 target. That run is the next new field coverage, not a re-run.
2. **The repair-sweep family with three runs' field data — #501 (the sweep is unbounded
   under dead holders; now also CI-measured via the e2e flake history), #500
   (fetch-retained copies never announce), #502 (restart orphans the repair working
   set).** #501 first: it is the mechanism behind the widened (measured) e2e window and
   the #509 bound miss; bounding the sweep re-tightens both.
3. **#466 chain-serve pagination — the PE approach review, now evidence-backed** (the
   #503 retainer attribution: 98 MB of retained `EncodeBlocks` output on the
   dead-peer-serving node; ~310 MB per full-chain encode at cloud heights). The serve
   working set is the remaining island memory term.
4. **#506 — the Q3 reg-inclusion rate bound (validity rule), version-gated.** Required
   per the #503 certification (the structural close against an adversarial
   re-registrant); needs the coordinated-upgrade/version-gate story first.
5. **Harness hardening, remaining from the 2026-08-19 audit:** (a) chaos-crash GAPs (not
   FAILs) on a non-landed publish; (b) dedicated non-anchor registry (repoint REGREF);
   (c) persistent VPC across runs (~7 min/run); (d) parallelize read-only scenarios.
   Plus the small filed items: #509 (compute the down-designee bound from the #451
   round arithmetic), #507 (the `TestMeasure_StoreChunkDrainRate` test race).
6. **Phase 3 proper — cheap heights** (#299 near tiers: Merkle multiproof compression,
   batch verification, reg/entry batching). Exit gate: a deep green sheet (h ≥ 128)
   with the prune field-exercised at production parameters — which also delivers the
   deferred Phase 1.4 deep run.
7. **Fork (c) split-pay stays evidence-gated** — two clean economy grades have not yet
   shown the (a-domain-fresh) gap biting; it stays parked until one does.

**Standing parallel lane (blocks nothing):** the #183 procurement search (longest lead
time, zero code dependency, still ownerless); evidence hygiene; stale-branch pruning
(`drill/v2b-gate-starvation`, `evidence/soak-9453325-7258`, `oracle/451-locked-value-stall`).

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
  Verdict: **M1 is the binding constraint.** The ordered path above is the response
  (D-M1-PIVOT); full findings in
  [`docs/thinking/2026-08-19-fresh-eyes-audit-and-the-m1-pivot.md`](docs/thinking/2026-08-19-fresh-eyes-audit-and-the-m1-pivot.md).

## The forward tracks (what replaces the gate spine)

Three kinds of work remain. **Build** makes the decided directions real; **verify**
is the gate to declaring M0 *held* (not merely built); **research frontier** is the
handful of items that need a new result, not a decision.

### Build tracks

- **H8 — metadata-layer privacy (next).** The D-PRIV build track: mixnet transport +
  **private DHT lookup** (server-held-DB PIR, Peer2PIR model — routing/provider
  records only; blobs ride the mixnet) + **D3 issuance-mixing** (route token
  issuance over the content-blind relay from an ephemeral identity, epoch-batched)
  to close the publisher IP+timing link. Bounded by the anonymity trilemma; a stated
  product tradeoff, not a blob-layer absolute.
- **H9 — pluralistic takedown, provably non-global.** Signed subscribable label
  layer + a **CT-style append-only transparency log** (every honored revocation
  committed, with inclusion/consistency proofs) + a narrow opt-in denylist + the
  **non-globality metric** (survivor Nakamoto-coefficient published as a certified
  lower bound `≥ t` via a ZK threshold predicate). Low urgency.
- **D-DEMAND — the blind demand receipt. ✅ P0–P3 BUILT** (#181): issue → PoR-bound
  delivery-ack → bank → redeem, blind withdrawal, fee-burn + bonded-fetcher
  cost-to-wash levers, self-dealing red-team at both tiers. Demand stays a **neutral
  observable** (never standing). Remaining: the P2 dispute-*resolution* half (gated
  on a verifiable-escrow primitive with no adoptable pure-Go impl) and
  fetcher-unlinkability's timing leg (H8 epoch-batching).
- **C2 metric wiring. ✅ BUILT** (#185): `chain.C2Metric()` over the committed bond
  ledger (never gossip), consumed by the shed; `OperatorMargin` discounts for the
  clustering unknowns. Still future: the H8 committee-certification consumer +
  Byzantine-robust sampling.
- **Registry economics (Gate-5 lineage). ✅ core shipped** — registry-only mode +
  read-cost bounding (#206). Post-launch: liveness-pruning + federation (#207).
- **M1 — efficiency + the economy. ⚠ Sequencing SUPERSEDED (2026-08-19, D-M1-PIVOT):**
  the earlier rule "M1 opens only after the M0 gate" is retired — M1 now interleaves
  with the M0 tail on the ordered path above (Phases 2–5), because the deep field
  confirmation and a valid #183 both depend on it. The track's content stands:
  #299's near tiers (Merkle multiproof, batch-verify) and reg/entry batching are
  Phase 3; the full #299 sealing re-architecture stays parked (research-gated);
  CPU/dial gauges remain allowed-early measurement. **The trust harness never softens
  for M1 — cost budgets overlay the same runs** (unchanged).

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
