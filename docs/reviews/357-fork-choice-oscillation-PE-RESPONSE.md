# PE response to #357 — objective fork-choice oscillation on real WAN

**From:** principal engineer (audit & rescue seat) — advisory to the owner
**To:** research team
**Re:** the #357 consult (`357-fork-choice-oscillation-CONSULT.md`)
**Status:** opinion + repro spec for your ruling. Per build-immutable #6 the consensus-rule / C2-claim change is **yours to certify**; this note is input, not a decision. One trade in it is the owner's, and he has made it (below).

**Owner's priority order for this decision (governs the trade-offs):**
**immutable chain/trust → durability → scale → efficiency (cheap to run).** Where they conflict, trust wins.

---

## 0. First, the process worked

The blind field test found a real defect the local `consensus` sim passes clean, exactly because the sim models a partition of *already-bonded* validators and not the **bond-registration drain window** where this lives. That is the two-substrate immutable (V1) doing its job. The fix belongs at the tier the bug lives — the drain window under divergence — not at the sim that can't see it.

---

## 1. The sharpest thing in the consult that isn't named yet: the "heavier fork" is not heavier

The consult treats "reorged onto a heavier fork, new head height 0" as a weight *inversion*. I read it as a **degenerate zero-weight tiebreak**, and I think that distinction changes the fix.

Trace the weight path during the drain window:

- `blockWeight` (objective, `chain.go:1570`) sums `c.bonded[id]` over qualified non-proposer attesters.
- During the ramp the attesters are **anchors whose bond has not committed yet** (`launchAnchor` makes them `attesterQualified`, but their registration is still draining per `MaxBondRegBytesPerBlock`), so `c.bonded[id] = 0`.
- ⇒ committed blocks 1 and 2 each contribute **weight ≈ 0**; the 2-block chain has `Weight() ≈ 0`.
- A lagging validator still on `[genesis]` also has `Weight() ≈ 0`.
- `heavier(a,b)` (`chain.go:1651`) is `a.Weight() > b.Weight()` → `0 > 0` is **false** → it falls through to the **lower-head-hash tiebreak**. Genesis's head hash is fixed and low, so the genesis fork can **win the tiebreak** and be adopted — dropping the committed blocks.

So during the pre-bond window fork-choice silently degrades to **"lowest head hash wins,"** which has zero correlation with height or commitment. Committed history is dropped by **hash luck**; the WAN's only role is to supply the competing genesis fork by delaying a validator. **This is not fundamentally a WAN bug** — WAN is just how the second fork gets introduced.

**Prediction to falsify cheaply (do this before any netem/cloud run):** a pure unit test on `heavier`/`Reconcile` with two anchor-attested forks whose attesters have `bonded=0` — a taller committed fork vs. a shorter/genesis fork — reproduces the height-0 reorg deterministically, on a laptop, no network. If it does, the root cause is fork-choice degeneracy, and the WAN repro is confirmation, not discovery (#6).

---

## 2. Two entangled failures, one structural cause

1. **Safety / immutability degeneracy** (§1): zero-weight tiebreak drops quorum-committed blocks. This is the one that violates the #1 priority — a chain reorging its own committed history is the negation of "immutable."
2. **Liveness stall** (as the consult describes, correctly): split forks + honest **refuse-to-cross-attest** (`chainrole.go`) + a `RequiredQuorum` that grows as `qualifiedCount` climbs mid-ramp ⇒ no fork gathers quorum ⇒ `insufficient valid attestations: 0 of 2`. The `6-fault-tolerance` no-commit is very likely the same knot.

Both dissolve if, **during the launch window**, (a) anchor attestation carries real, stable weight so a quorum-attested chain always outweighs a bare-genesis fork, (b) quorum is sized against the fixed anchor set rather than the drifting `qualifiedCount`, and (c) a finality gate makes quorum-committed blocks non-reorgable.

---

## 3. Recommendation, shaped by the priority order

**Adopt an explicit two-phase consensus with a finality gate.** It collapses the four knobs the consult correctly says entangle (anchor-zero-bond bootstrap, the #286-L2b registration drain, `ByzantineQuorum`, refuse-to-cross-attest) into a single clean phase boundary — and it is the *same* young→mature, time-boxed-scaffolding, one-way pattern the canon already ratifies (`everMature`, immutable #3), just applied to fork-choice weight and quorum sizing where it was quietly missing.

1. **Launch phase (genesis → founding bonds committed).** Anchor *eligibility* carries a fixed, equal fork-choice weight; quorum is sized against the **fixed plural anchor set** (a threshold, e.g. ≥⅔ of the M anchors), not against a growing `qualifiedCount`. The drain window is anchor-finalized and immutable. Objective bond-weighted fork-choice does **not** run while weight is meaningless.
2. **Handoff at drain-complete.** Transition to objective bond-weighted fork-choice + byzantine-quorum over committed bond. This is where C2/shed lives, and by now `weight = committed bond over operators` is meaningful and stable.
3. **Finality gate, both phases.** A super-quorum-attested block is irreversible; fork-choice chooses only among descendants of the latest finalized block.

**Why this ordering serves the priorities:**
- **Trust (#1):** the finality gate protects committed history *even if the weight metric is still imperfect* — so it lands **first**, as the smaller, more certain change, and stops the bleeding today.
- **Durability (#2):** committed roots / bond regs / care-links stop vanishing. See §5 — this is plausibly the root of the `durability-turnover` FAIL, so it may be one bug, not two.
- **Scale (#3):** post-handoff it is your normal bond-weighted BFT; the special-casing is time-boxed, not an n=4 assumption baked in.
- **Efficiency (#4):** the gadget is incremental on machinery you already have (quorum attestations, byzantine-quorum, equivocation slashing, WS-checkpoint). It is not a new protocol.

**One consequence to make explicit (owner's call, already made):** real finality is **safety-over-availability under partition** — a minority-partitioned validator **stalls** (cannot finalize) rather than reorg. Given trust > durability > scale > efficiency, that is the correct trade and the owner has decided it: a stalled partition is an availability hit that heals; a reorged commit is a trust violation that does not. Please treat "prefer stall to reorg" as a fixed constraint on the design, not a knob.

**Finality must be quorum-based, not bare depth.** A naive "don't reorg below depth N" lets two partitions finalize conflicting blocks — worse than a reorg. Final = a real super-quorum of the relevant weight (anchor threshold in launch, ≥⅔ committed-bond after), so conflicting finalization is impossible. This also composes with the existing `-ws-checkpoint` (WS handles cold-boot/long-range; the finality gate handles live reorg depth).

---

## 4. Your four asks, answered directly

1. **Is fork weight depending on which drained registrations a fork committed *sound*?** No — that is the root. During launch, weight must come from **anchor attestation** (stable, height-monotonic), not the transient registration subset. Do not run bond-weighted fork-choice before there are bonds. *(This is the leg that touches C2 — see §6, your ruling.)*
2. **Quorum sizing vs divergence?** Pin it to the **fixed anchor set** during launch; stop deriving `RequiredQuorum` from a `qualifiedCount` that is itself a function of the unstable drain. It is a real liveness bug in the sizing, not "small-set configs should pin `-quorum` by hand."
3. **Gate reorg on a finality condition?** **Yes, unconditionally** — and it is not masking, provided you *also* fix the launch-weight metric (§4.1). The weight fix is the liveness/consistency leg; the finality gate is the safety leg; neither substitutes for the other. Tie it to super-quorum, not bare depth.
4. **Which repro?** Two, cheapest first: **(a)** the **zero-weight-tiebreak unit test** of §1 (no network — nails the height-0 degeneracy); **(b)** a **sim that models independent per-validator registration-commit ordering** across the drain window, with per-fork weight + quorum logging — the element the passing `consensus` sim lacks. Netem alone may not deterministically hit the reorder that matters; model the drain divergence directly.

---

## 5. A connection worth checking

The #357 oscillation is a strong candidate for the **root of the `durability-turnover` FAIL** in the same run family ("publish never produced a link"): no stable quorum ⇒ no commit ⇒ no link. If §1 is confirmed, verify whether fixing #357 closes that flow too — they may be one defect reported at two surfaces, which would also unblock the P0 publish-over-WAN work rather than being a separate item.

---

## 6. The line between your ruling and the owner's decision

- **Yours to certify:** the launch-weight change (§4.1) alters what fork-choice weight *means* during the anchor window — anchor attestation, not summed committed bond. The C2 "no quiet capture" argument reads off "weight = committed bond over distinct operators." My view is C2 is untouched, because it was always a *mature-regime* claim and the launch window is anchor-trust by design (immutable #3) — but that transition-of-meaning across the maturity boundary is exactly the soundness call that is yours, not the build team's.
- **Owner's, and decided:** the CAP trade in §3 — safety/finality over availability under partition.
- **Build team's, once you rule:** implement the phase boundary + finality gate, land the finality-depth safety gate first, and confirm on the two repros above before any billable run.

---

*Net: I believe the height-0 reorg is a zero-weight tiebreak degeneracy (falsifiable in a unit test today), the liveness stall is its twin, and both close under an explicit anchor→bond phase boundary with a quorum-based finality gate — landing the finality leg first because trust is the priority that is actually broken.*
