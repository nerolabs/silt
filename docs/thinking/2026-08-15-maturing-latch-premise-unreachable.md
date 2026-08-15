# 2026-08-15 — The MATURING drill's latch premise is unreachable by construction (found before the re-run, on a laptop)

**Context.** The session plan (post-#427) was: compute principled harness bounds, then ONE
instrumented MATURING re-run to confirm (a) maturity reachable-within-bound and (b)
fresh-publisher bounded. Deriving the maturity bound requires knowing how many committed
regs trip the `everMature` latch — and walking that arithmetic against the shipped code
showed the answer is **no number of them ever does, in this topology**.

## The mechanism paragraph (build-immutable #6)

The failure is *the 10-maturing-handoff latch never trips* **because** the bar-2 maturity
predicate is evaluated over a set the MATURING topology leaves structurally empty of
qualifying decentralization: `matureNow()` gates on
`min(NakamotoOperators, NakamotoDomains) ≥ MatureValidators(=2)` computed by `C2Metric`,
which (1) **excludes anchors** (`core/chain/chain.go:1314`) — and all 4 validators in the
topology ARE the anchors (`topology.py:189`: anchors = every validator) — and (2)
aggregates the only non-anchor cohort, the 8 Sybils all declaring `-domain sybilnet`
(`topology.py:323`), into **one** address-diversity group → `NakamotoDomains = 1 < 2`,
**by design** (that discount is the C2 defense `TestC2SingleDomainSybilsDoNotMature`
pins). A third, independent blocker: `validatorsSeen` fills only from **attesters** of
committed blocks (`chain.go:1957-1961`), which the single-anchor-gathered launch phase
limits further. **The drill asks the metric to be tripped by exactly the cohort the metric
exists to refuse.**

**Evidence:** `core/chain/maturing_topology_premise_test.go`
(`TestMaturingFieldTopologyLatchUnreachable`) constructs the exact MATURING
parameterization at **full drain** — every bond banked, every participant generously
granted `validatorsSeen` — and measures
`NakamotoBonds=3 NakamotoOperators=3 NakamotoDomains=1 → Mature()=false`. Deterministic,
0.4s, on a laptop.

**Corollary:** the two field GAPs ("latch never tripped in 420s", runs 9c3777d-73949 and
7134711-18163) had **two stacked causes**: the drain staleness race (real, root-caused,
fixed in #427) *and* this premise defect (the latch was unreachable even at full drain).
The planned re-run would have burned a billable run to re-discover this GAP. The
model-check discipline ("no graded field run until the deterministic tier covering its
regime is green") is exactly what caught it — the maturity-bound derivation forced the
arithmetic that the 420s window had been hiding.

**What this is not:** a core defect. The anchor exclusion is correct (counting the
scaffolding's own bonds to shed the scaffolding would be circular — immutable #3), and the
single-domain discount is the certified C2 defense. The defect is in the drill's
**topology parameterization**: `topology.py`'s comment expects the latch to trip on "the
coefficient the 4 distinct-operator validators actually reach (2)" — a model of the metric
that forgot the anchor exclusion.

## The intended maturation shape already has a deterministic certification

`modelcheck_i3_test.go` (`matureWeightedEpoch`) matures a chain the way the design
intends: **honest non-anchor validators, distinct domains, real weight, attesting
committed blocks** (rotated so every one lands in `validatorsSeen`), alongside a
single-domain cheap cohort that adds head-count but no decentralization. The field drill
should be the on-the-wire confirmation of that certified shape — today it deploys only the
cheap half.

## Options

**(A) Re-split the 8 Sybil slots: 4 honest maturers + 4 single-domain Sybils.**
Maturers = non-anchor validators, real bond (64M), distinct domains (or unset — each its
own group), persistent-peers to the anchors. No new VMs/IPs (the 3 regions are exactly at
the 8-IP quota; the 8 slots already exist). No core change: drain proposers gather from
`syncTargets()` filtered by `AttesterEligible` (`chainrole.go:869-875`), so a maturer's
attestations are solicited once its bond commits → `validatorsSeen` → the metric.
Arithmetic at full drain: non-anchor set = 4×64M distinct + 4×1M one-domain → total 260M,
threshold 86.7M, NakamotoOperators=2, NakamotoDomains=2 → `min=2 ≥ bar 2` ✓ — and the
Sybil cohort alone still cannot mature it (their 4M single group is nowhere near
threshold). Mirrors the I3 oracle exactly.
*Cost:* the drill's cheap-cohort pricing changes from "~9 MiB of cheap heads vs ~256 MiB"
to ~4 MiB vs ~512 MiB, and the "≥8 equal single-domain bonds trip the bond-atomization
note" premise scopes down to 4 — a disclosed re-parameterization of a reviewed drill, so
it gets a PE concurrence note rather than a silent edit.

**(B) Add 4 maturer VMs, keep 8 Sybils.** Cleanest drill economics, but needs 4 more
static IPs — every region is at its 8-IP `IN_USE_ADDRESSES` quota (`topology.py:117-121`),
so this waits on a quota bump or a 4th region. Money is not the constraint; quota latency
and topology churn are.

**(C) Give the Sybils distinct domains.** Rejected: 3 distinct-domain 1M bonds would trip
bar-2 — a drill in which *cheap identities mature the network* is the opposite of what the
drill certifies (it would field-demonstrate ¬C2).

**(D) Co-locate maturer daemons on existing storage/fetch VMs.** No new IPs, but two
consensus participants sharing a VM muddies the drill's failure-domain claims and risks
CPU cross-talk perturbing the very cadence measurements the run exists to make. Rejected
while (A) is available.

## Decision

**(A)**, routed through a PE concurrence note
(`silt-reviews/principle-engineer/maturing-topology-premise-builder-2026-08-15.md`)
because the drill parameterization was part of a reviewed ruling — the re-run stays
blocked until the topology fix lands. The premise repro test ships now regardless (it pins
correct core behavior and documents the trap). The principled-bounds work (PE cadence
ruling §4) proceeds: the publish bound is topology-independent, and the maturity bound
becomes computable under (A) — 2 maturer regs (~1.5 MB each, ~1/block under the 2 MiB cap,
one drain block per 30s `ChainSyncInterval` sweep) put the latch ~2 reg-blocks after drain
start, with the full 12-reg drain bounded at ~12 × 30s = ~360s + submission-retry margin.

## The lesson

The premise of a drill is part of what the deterministic tier must cover. The I3 oracle
verified *its own* setup ("a green oracle over a broken setup is worse than none") — but
nothing asserted the **field topology's** setup reaches the state its flows grade. The
new premise test is that assertion for 10-maturing-handoff. When a field flow GAPs twice
with the same "precondition unmet" line, check the precondition's *reachability* on a
laptop before attributing the miss to anything on the wire.
