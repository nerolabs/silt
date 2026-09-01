# Retired ROADMAP organizing schemes — superseded process history (extracted 2026-09-01)

> ⚠ **HISTORICAL — superseded process framing. FROZEN, NOT THE PLAN.** This file
> preserves three retired organizing schemes that used to live in `ROADMAP.md` in the
> present tense, stacked on top of one another. They are **superseded by the Boulder/Rock
> spine** (`ROADMAP.md`, "The Boulders (current big-step tracker)"). Read them for
> provenance only — how the plan was framed before the 2026-09-01 seven-seat audit
> reorganized the same board into five sequenced Boulders → Release Candidate (V1).
>
> **Why extracted.** `ROADMAP.md` carried three live-tense frameworks at once — Boulder/Rock
> (current), "the ordered path" + numbered Phases 1–6 (this file), and an "Immediate next
> work" priority snapshot dated 2026-08-26 (this file). A fresh reader could not tell which
> governed. The Boulder spine is now the single source of truth; the schemes below are the
> record of what it replaced. Nothing here is a task list. All still-live content named
> below was re-situated under the correct Boulder in `ROADMAP.md`; only the retired *framing*
> and the completed-phase detail were moved here.

---

## Scheme 1 — "The ordered path" preamble + numbered Phases 1–6

Introduced as *"The ordered path (the V1 roadmap — ratified 2026-08-19, D-M1-PIVOT)."* The
D-M1-PIVOT **decision** (M1 interleaves with the M0 tail; storage-economy-first sequencing;
the γ→1/N firewall unchanged; the trust harness never softens) remains load-bearing and lives
in `docs/decisions.md`. What is retired is only the *packaging label* — the "phase" framing and
the claim that "this ordered path is the track." The Boulder synthesis (2026-09-01) is the
current packaging of the same ordered board.

### The ordered-path preamble (retired framing)

**Why this section existed.** "Tenets are the roadmap" proved too loose: it let a month
of effort pool on one axis (consensus correctness / memory survival — necessary, and
now complete and verified) while the other axis (the S7 economy, bandwidth pricing, the
operational floor) received none. The 2026-08-19 fresh-eyes audit
([`docs/thinking/2026-08-19-fresh-eyes-audit-and-the-m1-pivot.md`](../docs/thinking/2026-08-19-fresh-eyes-audit-and-the-m1-pivot.md))
found the trust plane verifying end-to-end but the durability economy **built,
test-proven, and switched off in every shipped node** — a network on track to earn a
security certificate without a demonstrated reason to exist at equilibrium. So the
tenets stay the *destination*; **this ordered path is the track**, and work was expected
to follow it top-down. The prior rule "M1 opens only after the M0 gate" is superseded
([`docs/decisions.md`](../docs/decisions.md) D-M1-PIVOT): the M0 tail is small and
enumerated, and both the deep field confirmation and a valid #183 depend on M1 being
real.

*(Correction folded into the Boulder spine: the Boulders are NOT strictly top-down. They
encode explicit dependencies and parallelism — Boulder 2 depends on Boulder 0, not
Boulder 1; Boulder 3 runs parallel; R2.1 is independent, start now. The "follow it
top-down" expectation was a phase-era statement.)*

### The numbered Phases 1–6 (retired; Phases 1–3 COMPLETE, historical record)

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
      [docs/thinking/2026-08-19-publish-502-attribution-care-self-deadlock.md](../docs/thinking/2026-08-19-publish-502-attribution-care-self-deadlock.md)
      and the drill record
      [docs/thinking/2026-08-19-v2b-gate-starvation-drill-design.md](../docs/thinking/2026-08-19-v2b-gate-starvation-drill-design.md).)*
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
   stays research-gated). *Gate clause DISCHARGED (2026-08-27):* the publish bound is
   re-derived **downward** 360 → 300 s from fe2376a-deep's measured ~48 s/height cadence
   (results-fe2376a-deep.jsonl:29). The finding: only the gather-leg term is cadence-free
   arithmetic; the commit-wait leg is the #451 synchronizer 2-round escape FLOOR (150 s,
   counted in fixed 30 s sweeps — a consensus-liveness parameter, left untouched). The
   60 s shed is historical escape-rounding + stale slow-height padding the cheap cadence
   retires; 300 s keeps the full escape window inside the bound (6.25× the measured
   cadence). Derivation: `docs/thinking/2026-08-27-publish-bound-rederivation.md`
   (#549-Q3 discipline: derived, not guessed).
4. **Phase 4 — Proof-of-Delivery (price bandwidth).** Storage is priced; bandwidth is
   not, so an open relay/gateway is a free-rider choke (the recentralization failure
   mode). Spec + research consult **first**; the hard prerequisite is closing the
   receipt-forgeability residual (a demand receipt is currently mintable with zero
   object bytes — inert only while demand has no consumer). Wash re-priced per
   D-DEMAND. **Firewall immutable:** delivery credits fund durability and relay
   compensation, never consensus standing (γ→1/N, #182).
   *(Status: the PoD spec shipped + certified; §7.1/§7.2/§7.3 shipped. Third-operator
   committed settlement is the remaining heavier PoD lane. Now carried in the Boulder
   spine and the residual backlog.)*
5. **Phase 5 — The operational floor (a node a person can run).** Per-platform service
   packaging (launchd / systemd / Windows service) + signed installers, and
   operator-consented self-update per R4 (signed manifests, never silent — a working
   downstream macOS signed+notarized+launchd reference exists and generalizes into
   silt). Incremental O(delta) proof maturation (kill the O(store) restart scan);
   reprovide dirty-tracking (kill the O(held) per-interval re-sign). Exit gate: a
   non-developer installs a node that survives reboot and returns to serving in
   seconds, and steady-state cost no longer scales with the whole held set (S6).
   *(Status: NOT STARTED — needs scoping. Now carried in the residual backlog.)*
6. **Phase 6 — External red team (#183) → V1 RC.** The engagement runs against the
   **economy-ON** configuration — the network people will actually run. Then R1: a
   fully green multi-region grade on the RC config, and V1.
   *(Status: now the Boulder 4 endgame.)*

---

## Scheme 2 — "Immediate next work (updated 2026-08-26 — post-Phase-3 priority order)"

A dated priority snapshot, five days before the 2026-09-01 Boulder synthesis. It said "The
current order" and enumerated work the Boulders now organize differently. Retired as a
superseded dated snapshot; its still-live residuals (#559, #583, #574, #530, #586) are carried
in the Boulder spine and the residual backlog.

Phases 1–3 are banked. The depth war is over: `fe2376a-deep` met the Phase 3 gate
(h132, prune engaged, 0 fail) and closed the whole stall lineage (#549/#560/#561/
#572/#573). #183's close condition is MET and the issue carries the evidence — the
close itself is the owner's call and is deliberately HELD. The current order:

1. **Phase 4 opening move — ✅ DONE (2026-08-26): the PoD spec shipped
   ([design/pod.md](../docs/design/pod.md)) and research CERTIFIED it same day**
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
2. **The PoD neutral-lane BUILD** (per the certified [design/pod.md](../docs/design/pod.md)
   §7). **§7.1 SHIPPED (#590):** the conserved balance-lane consumer
   (`RedeemDeliveryCredit`, `core/credit/delivery.go`), the supersede rule, the
   no-PoR neutral receipt, and the firewall failing-first test
   (`TestDeliveryCreditNeverTouchesStanding`). **§7.2 SHIPPED (#593/#594):** the
   D-TIERING mode flags `--serve-content`/`--archive` (`cmd/silt/daemon.go`), the
   daemon wiring, and e2e (`-accept-delivery-receipts`). The three owner knobs are
   decided (**D-POD-KNOBS**, #592). The **B3 receipt-forgeability residual is CLOSED**
   by the conservation design and regression-locked (`TestWashLoopIsAStrictLoss`,
   `TestPaidBountyIsNotRecoverableBySupersede`); it is neutralized by the firewall
   today and must close before any demand→standing fusion
   (`docs/design/owned-residuals.md`). **§7.3 relay compensation SHIPPED
   (#646/#647/#649/#650):** the certified sender-funded PayWord micropayment (the relay
   leg is self-enforcing, no TTP — **D-POD-KNOBS** item 2), delivered across four merged
   increments — PayWord machinery (#646), transport Batch 1 (#647; S-clamp #644 +
   epoch-tied seen eviction #645), Batch 2 (#649; wire protocol + paid forwarding pump +
   session reaper), and Batch 3 (#650; daemon binding, paid relay goes LIVE, closes #648).
   The Batch-3 free-vs-paid routing policy was ratified as **Option B**
   (**D-POD-RELAY-COEXIST** — paid relay is additive to free relay under shared caps).
   Tracked
   follow-ons remain (not blockers): #651 (resolver stopped-loop hardening, deferred to
   graceful-shutdown work). A separate **GATED** residual — paid-reserved headroom —
   re-opens a research-gated parameter and is not scheduled. **Third-operator committed
   settlement is the remaining heavier PoD lane** (couples to the v5 keystone; own
   certification when specced) — the next PoD frontier, not yet started.
3. **The state-root keystone track** — **era-3 format FROZEN 2026-08-29 (#632, build
   `3af40bc`); era-4 witnessable-transitions spine BUILT + merged 2026-08-29.** The era-3
   committed state-root format (`BlockVersion = 4`) is now in the Immutable tier
   (`docs/TENETS.md` Part IX): the block commits a state SMT root plus an append-only
   transparency-log root over the completeness-proven 18-field set, and changing it requires
   a new era, not an edit. The trustless-floor-box path then continued as the **era-4
   witnessable-transitions track (`BlockVersion = 5`)**, whose build spine landed across four
   merged increments: 4a (#637 — mint v5 + reserve the three v5 field tags, inert), 4b (#639
   — the maintenance spine: `qualified` + due-bucket + `epochStart`, v5-gated), 4c (#640 —
   the v5 validity predicate + `RegCap` + version-widen), 4d (#641 — height-gated activation +
   mint-flip). `RegCap` is the per-block TOTAL BondReg count cap — fresh AND renewal, after
   the same-id fold — value **256** (`core/chain/chain.go:404`); the earlier "fresh-only"
   reading was REFUTED and corrected. This is the **chain-side** witnessable-transitions spine
   only; the trustless floor-box (witness) validator itself remains the open C-7 / #600
   follow-on, not yet shipped. **Governance stance (owner-ratified 2026-08-30): era-4/v5 is
   deliberately kept OPEN-ENDED — there is no live blockchain, and PoD may still reshape
   witnessable state — so the v5 freeze is DEFERRED to the END of Proof-of-Delivery, to be run
   as a second practiced era freeze** (the era-3 freeze being the first). *(Superseded detail:
   the freeze is now re-scoped to the Release Candidate and decoupled from the accept-flip —
   Boulder 3 R3.4.)*
4. **The publish-bound re-derivation** — ✅ DONE (2026-08-27): 360 → 300 s, derived
   downward from fe2376a-deep's measured ~48 s/height cadence; the #451 escape floor
   (a consensus-liveness parameter) left untouched.
   Derivation: `docs/thinking/2026-08-27-publish-bound-rederivation.md`.
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

---

## Scheme 3 — "The forward tracks (what replaces the gate spine)": the Build-tracks list

The forward-tracks section was a third top-level organizing scheme (Build / Verify /
Research-frontier tracks). The **Verify tracks** and **Research frontier** content is still
live and load-bearing for "M0 held" — it was re-situated under the Boulder 4 endgame in
`ROADMAP.md`, not archived. Only the phase-framed **Build tracks** list (below) is retired
here; its still-live items (H8 #179, H9 #180, post-launch registry #207) are carried in the
`ROADMAP.md` residual backlog.

### Build tracks (retired framing — status snapshot)

- **H8 — metadata-layer privacy (next).** The D-PRIV build track: mixnet transport +
  **private DHT lookup** (server-held-DB PIR, Peer2PIR model — routing/provider
  records only; blobs ride the mixnet) + **D3 issuance-mixing** (route token
  issuance over the content-blind relay from an ephemeral identity, epoch-batched)
  to close the publisher IP+timing link. Bounded by the anonymity trilemma; a stated
  product tradeoff, not a blob-layer absolute. *(#179 — carried in the residual backlog.)*
- **H9 — pluralistic takedown, provably non-global.** Signed subscribable label
  layer + a **CT-style append-only transparency log** (every honored revocation
  committed, with inclusion/consistency proofs) + a narrow opt-in denylist + the
  **non-globality metric** (survivor Nakamoto-coefficient published as a certified
  lower bound `≥ t` via a ZK threshold predicate). Low urgency. *(#180 — carried in the
  residual backlog.)*
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
