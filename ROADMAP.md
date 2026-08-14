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
- **M1 — efficiency (opens only after the M0 gate, in this order per the PE
  ruling):** (1) #299 succinct bond proof (research-gated; collapses the size-aware
  deadline + byte budget + O(N) drain in one move); (2) residual token-gather cost;
  (3) CPU-per-audit + dials-per-fetch gauges — measurement, allowed early, captured
  during P1 as the M1 baseline; (4) drain batching (determinism guards intact);
  (5) genesis-to-head diff sync (#382 follow-up). The trust harness never softens
  for M1 — cost budgets overlay the same runs.

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
