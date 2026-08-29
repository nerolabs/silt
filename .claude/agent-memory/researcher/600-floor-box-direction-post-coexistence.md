---
name: 600-floor-box-direction-post-coexistence
description: #600 RECOMMENDATION — adopt witness path as PRIMARY floor-box validation; hold-tree measured-not-viable; ≥1-honest-provider liveness residual PROMOTED optional→load-bearing; tenet 557 conditional on open witness-serving.
metadata:
  type: project
---

**#600 floor-box direction — recommendation filed 2026-08-28** (composes the coexistence measurement + the C-7 soundness cert).

**Recommendation:** adopt the WITNESS path as the floor box's PRIMARY validation; do NOT ship hold-the-tree as the floor-box default.

**Why:** hold-tree is measured NOT VIABLE on the 2 GB floor box — `integration/cloudtest/coexist-20260827T212244-citev/`: 1060 MB balloon + 1M-key bbolt build on 2 GB no-swap = THRASH-TO-UNUSABLE (NOT OOM; kernel evicts mmap'd pages, 1-vCPU re-faults faster than pd-SSD serves; network+SSH dead ~40 min; the #601 305 MB no-pressure floor never reached). Witness path is certified sound ([[c7-witness-floor-box-validation]]). Witness = sound-AND-fits; hold-tree = sound-but-doesn't-fit. No third certified option.

**Measurement scoping guard (do not over-read):** the run measured the 1M-key BUILD phase under coexistence. It refutes "floor box self-holds the tree," NOT "bbolt is unusable in general." Cite it as the floor-box-cannot-self-hold finding only.

**THE KEY RE-PRICING (Q2):** the ≥1-reachable-honest-witness-provider liveness assumption is PROMOTED from optional (C-7 priced it as parked, because hold-tree was still a self-sufficient fallback) to LOAD-BEARING (the only floor box that fits the box now depends on the tier above to make progress). This is a change in KIND not degree. SAFETY is unaffected — floor box still stalls-not-accepts, unconditional on provider honesty. Only LIVENESS/progress depends on the tier.

**New seam — its OWN residual, cross-ref #183, NOT folded in.** #183 = cold-start/maturity-before-capture seam (see [[C1-maturity-before-capture]]). Witness-liveness = post-maturity "can a tree-less floor box keep validating if the witness tier degrades." Same SHAPE (liveness-on-tier-above, safe-degradation), different TRIGGER. Record separately in `docs/design/owned-residuals.md`.

**Tenet bound (Q3):** governing tenet is `docs/TENETS.md:557` — decentralization may be centralized for convenience but NEVER load-bearing; decentralized path must always exist. Witness path is tenet-compliant CONDITIONALLY: witness-serving must be OPEN + multi-provider (any archival/pruning node, un-permissioned). Trustless VERIFICATION does not rescue a permissioned AVAILABILITY choke — that would be the banned load-bearing-centralized dependency. Floor box becomes a "semi-stateless full validator": same security, narrower self-sufficiency. Honest VISION story = validates soundly against the root, not holds-the-whole-tree (VISION:154-155 already framed it this way).

**Owner's call for Andrew:** ratify the posture (semi-stateless witness validator, load-bearing-but-DECENTRALIZED liveness dependency); hold the open-witness-serving condition as a hard requirement on the era-3 delivery design.

**Filed:** `/Users/andrewedmond/Claude/claude/silt-reviews/research/research-outcome/600-floor-box-direction-post-coexistence-RESEARCH-NOTE-2026-08-28.md`
