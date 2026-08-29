---
name: scar-one-defect-four-costumes
description: SCAR — "one defect in four costumes": #357, B2, #397, #402 — four RC-blocking field runs, one root cause (I1 finality quorum non-intersection). Base rate: every multi-region field run found a consensus bug.
metadata:
  type: project
---

# Scar: "One defect in four costumes" — consensus quorum non-intersection

**Failure class:** A finality quorum that does not intersect over its phase's real
validator set. Four incidents; one root cause.

**Source citations:**
- `docs/build-process.md` — "the consensus-correctness discipline (canon, 2026-08-14)":
  "#357, B2, #397, and #402 were one defect in four costumes — a finality quorum that did
  not intersect over its phase's real validator set."
- `docs/decisions.md` D-CONSENSUS section: "the four RC-blocking consensus bugs (#357,
  B2-handoff, #397, #402) were one defect — a finality quorum that did not intersect over
  its phase's real validator set — discovered one billable field run at a time because the
  invariant set was never written down."
- `docs/design/consensus-invariants.md` — I1 section: "Over 2026-08-13/14 the build hit
  #357, the B2 handoff issue, #397, and #402. They present as four different bugs. They are
  one bug in four costumes."

**The four incidents (each is I1 at a different doorway):**

| Incident | I1 doorway | Symptom |
|---|---|---|
| #357 | Quorum sized against live, shifting `qualifiedCount` | "0 of 2 gathered" — no consistent quorum ever formed |
| B2 handoff | Quorum counted by head-count, not weight | 8 min-bond sybils reach "quorum" with no real resource |
| #397 | Non-intersecting 2-of-4 launch finality | Two honest coalitions finalized conflicting blocks |
| #402 | `AnchorQuorum=1` free anchor; quorum sized over 4 anchors but fillable from 12 bonded | Competing block commits; permanent partition on 2-2 split |

Code sites (verified in `consensus-invariants.md` I1 section):
- `core/chain/chain.go` — `RequiredQuorum` (:721), `validatorSetSize` (:742),
  `bftThreshold` (:703); finality gate (~:2001–2024); `ValidateCommit` anchor gate
  (~:1472); epoch weight sum (~:1827).

**Base rate to remember:** Four consecutive multi-region field runs each found a new
RC-blocking consensus bug. The cause was the same each time; the doorway was different.
Inspection did not find these — only stressed field runs did.

**Resolution (canon, 2026-08-14):**
- `docs/design/consensus-invariants.md` (I1–I5) adopted as the closed invariant set.
- Every consensus-touching PR states which invariants it touches.
- The model-check tier is the first consensus gate (before any field run).
- The `⌊A/2⌋+1` strict anchor majority rule for launch finality (the `⌈A/2⌉` off-by-one
  was itself a bug: admits a 2-2 anchor split that the finality gate then cements into a
  permanent conflicting-finalization partition).

**What to watch:**
- Any new quorum site: does it finalize? What set is N? Are attesters drawn ONLY from that
  set? Does arithmetic intersect? Are non-members excluded? Weight or head-count?
- The `⌈A/2⌉` vs `⌊A/2⌋+1` seam: for even A these differ by exactly one. At A=4, that is
  the difference between tolerating and cementing a 2-2 fork.

**Links:** [[scar-depth-war-lineage]], [[class-gate-o-depth-review]]
