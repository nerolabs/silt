# Research consult — objective fork-choice oscillates under real multi-region WAN (#357)

**From:** silt build team
**To:** research team
**Re:** #357 — the 2026-08-12 **blind** field test found the objective bond-weighted chain
**reorging its own committed blocks back to height 0** on real GCP multi-region VMs, so no
publish can land and the flagship publish→fetch property can't be certified. This is a
**consensus-rule + published-claim** matter, so per build-immutable #6 we are characterizing
it and routing to you **before** touching the fork-choice weight or quorum. No fix proposed here.

---

## Ground truth (from the blind run, commit `9ed88ef`, `fast` bond, us-west1/us-east1/europe-west1)

The chain commits one or two blocks (each with the full 2 attestations), then reorgs onto a
"heavier fork" whose head is height 0, dropping the blocks it just committed — twice, ~10 min
apart, on both the client and val-a publish paths:

```
17:56:35  chain: committed block 1 (1 entries, 2 attestations)
17:56:46  chain: committed block 2 (1 entries, 2 attestations)
17:56:49  chain: reorged onto a heavier fork (dropped 2 block(s), new head height 0)
18:06:31  chain: committed block 1 …
18:06:49  chain: reorged onto a heavier fork (dropped 1 block(s), new head height 0)
```

A publish then fails `propose height 1: insufficient valid attestations: 0 of 2 gathered`
— from the ephemeral client **and** from val-a itself (bonds/standing all healthy,
`reputation=1024`, bond challenges `passed=true`). The **local `consensus` sim PASSES**
(objective fork-choice reorgs to the heavier chain under a *simulated* partition,
deterministically, on one host) — this is a real-WAN-only defect, exactly what the two-substrate
immutable exists to catch. Distinct from #286 (that was a genesis wedge that never committed;
this **commits then reorgs the commit away**).

---

## Code-level mechanism (what we found; where we're unsure)

**Fork-choice weight is summed on-chain bond of the attesters, per fork.**
`Chain.Weight()` sums `blockWeight(b)` over the chain's blocks; `blockWeight` (objective mode,
`chain.go:1570`) adds, for each distinct verified non-proposer attester that is
`attesterQualified`, **`c.bonded[id]`** — the attester's committed on-chain bond *as recorded in
THIS chain's state*. `heavier(a,b)` (`chain.go:1651`) adopts `a` iff `a.Weight() > b.Weight()`
(lower-head-hash tiebreak). `Reconcile` rebuilds the candidate fork in a fresh replica `tmp` and
compares `tmp.Weight()` vs `c.Weight()` — so **the same block's weight is computed against each
fork's own `bonded` map.**

**The founding set are anchors with ZERO committed bond at genesis.** Per the #286-L2b work,
genesis bootstraps quorum from anchor *eligibility* (`launchAnchor`), and the founding validators'
~1.5 MB bond registrations **drain across the next blocks** (the `MaxBondRegBytesPerBlock` byte
budget), not into genesis. So:
- `attesterQualified` is true for a genesis anchor via `launchAnchor(id)` — but `blockWeight`
  adds `c.bonded[id]`, which is **0** for a not-yet-registered anchor. So a **genesis-only fork
  has Weight ≈ 0**, and a chain's weight only becomes non-zero as bond registrations commit.
- `qualifiedCount()` (the N that byzantine-quorum is sized against) counts only
  `bonded[id] ≥ MinBond` — it is **0 at genesis** and grows as registrations land, so
  `RequiredQuorum()` (`max(Quorum, bftThreshold(N))`) **changes underfoot** during the ramp.

**Our leading hypothesis (needs your ruling).** During the first few blocks, a fork's weight is
dominated by *which bond registrations have committed in that fork*. Under real multi-region
latency/reordering, competing forks contain **different subsets** of the drained registrations,
so `Weight()` disagrees across forks and can rank a lighter-height fork heavier — and because
`blockWeight` re-derives each block's weight from the *candidate* fork's `bonded` map, a fork that
happens to carry an earlier/denser registration set can out-weigh a taller fork that doesn't.
Combined with the honest **refuse-to-cross-attest** rule (a validator won't sign a different block
at a height it already attested — `chainrole.go`), validators that diverge onto different forks
can't gather each other's quorum, so no fork reaches `RequiredQuorum` and the head oscillates.
The reviewer's independent lead points the same way: **`-byzantine-quorum` defaults ON and raises
the effective commit threshold above the plain `-quorum` floor**, and the `6-fault-tolerance`
no-commit (3 of 4 validators up, no commit) is "likely the same knot."

**What we did NOT establish** (and won't guess): whether the reorg-to-0 is (a) a *weight*
inversion from the per-fork `bonded`/registration-timing asymmetry above, (b) a *liveness* stall
from quorum-sizing vs. refuse-to-cross-attest under divergence, or (c) both — and whether the
"new head height 0" is a genuine adoption of a `[genesis]` fork or a transient rollback in
`adopt`. Pinning that needs a targeted `netem`/multi-region repro with per-fork weight + quorum
logging, which we can build once you point us at the axis.

---

## Why this is your call, not a build fix

1. **Fork-choice weight is a consensus rule and a published claim.** M0's C2 (no quiet capture)
   and the shed metric read off "weight = summed committed bond over distinct operators." Changing
   how weight is computed, or how quorum is sized during the ramp, touches the soundness argument,
   not just liveness.
2. **The knobs entangle.** `ByzantineQuorum` (safety-as-the-set-grows), the anchor-zero-bond
   bootstrap, the bond-registration drain (#286-L2b), and the refuse-to-cross-attest rule all meet
   here. A change to any one to restore liveness could weaken quorum-intersection safety or the
   Sybil-weight meaning. That trade is yours to adjudicate.
3. **Real-WAN-only + touches a claim ⇒ research-gated** (build-immutable #6): a billable multi-region
   run confirms an already-understood fix; it does not discover the cause.

---

## Specific asks

1. **Weight during the ramp:** is it *sound* for fork weight to depend on which drained bond
   registrations a fork has committed (so weight is ~0 at genesis and unstable for the first blocks
   under WAN)? If not, what is the intended weight of an anchor-attested block before its bond
   commits — count anchor eligibility, use a stable per-height weight, or finalize genesis+N before
   opening fork-choice?
2. **Quorum sizing vs. divergence:** with `ByzantineQuorum` raising `RequiredQuorum` as
   `qualifiedCount` grows mid-ramp, and honest validators refusing to cross-attest across forks, is
   the observed no-progress a liveness bug in the sizing, or is a small-set (n=4) config expected to
   pin `-quorum` explicitly? What should a fresh objective network's quorum be *during* the anchor→
   bonded transition?
3. **Fork-choice stability:** should reorg be gated on a stability/finality condition (e.g. don't
   reorg away a committed block below a rolling checkpoint, like the WS-checkpoint but N-deep) so a
   momentary weight inversion under WAN can't drop committed history — or is that masking the real
   weight bug?
4. **Repro we should build:** which minimal deterministic harness would let us reproduce this
   locally — a `netem` multi-region delay/reorder over the real chain, or a sim that models
   independent per-validator registration-commit ordering — so the fix is confirmed cheaply before
   a billable run?

---

## Provenance
Blind field test 2026-08-12 (`silt-reviews/fieldtest-august-12a/reports/cloud/…publish-fetch…`),
GCP run at `9ed88ef`, torn down (0 residual). Code refs: `core/chain/chain.go`
(`Weight`/`blockWeight`/`heavier`/`Reconcile`/`attesterQualified`/`qualifiedCount`/`RequiredQuorum`/
`bftThreshold`), `core/node/chainrole.go` (refuse-to-cross-attest, proposeBlock gather).
Standing rule that produced this: read/verify before acting + build-immutable #6 (root-cause first;
consensus/claim changes are research-gated). A harness `5-convergence` double-sample now catches
the instability directly (PR #356, H5).
