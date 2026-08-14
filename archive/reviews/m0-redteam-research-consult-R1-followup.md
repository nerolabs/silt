# R-1 follow-up — measured knee data + a serial-vs-parallel refinement that touches ε*

**From:** build team **To:** research team
**Re:** your R-1 answer (ship Option B, ε*≈0.20, knee-calibrated timed window).
**Why:** you offered to build the labeling-open cost probe to pin ε* from data;
Andrew asked us to do it in-house. We did — on silt's real `core/bond` v3 graph.
The numbers confirm your knee estimate **and** surface one refinement that changes
which ε* the enforcement can actually hold, which (because it changes the published
C1 number) is your call, not ours.

## What we measured (this hardware; reproduces the red team's tables)

Probe = the red team's `redteam_recompute_probe_test.go` run against live `core/bond`
(indegree-4 DRSample + chain edge, `plotSeedN`/`parentIndices`/`labelBlock`, the exact
`VerifySpaceTime` the wire calls). Two quantities per ε: **work** (total label
recomputations a *serial* prover pays) and **depth** (sequential critical path a
*parallel* prover pays). n = 65,536 (256 MiB).

| ε (disk dropped) | avg **work** (labels) | avg **depth** | recompute wall-clock (serial) |
|---|---|---|---|
| 0.02 | ~1 | ~1 | 0.17 ms |
| 0.05 | ~1 | ~1 | 0.39 ms |
| 0.10 | ~1.3 | ~1.3 | 0.98 ms |
| 0.15 | — | — | 2.5 ms |
| 0.20 | ~394 (answer) | small | 7.1 ms |
| 0.25 | ~6,030 (answer) | ~30 | ~100 ms |
| 0.30 | ~6.2M | ~32 (max 183) | seconds (serial) |
| 0.50 | ~7.6e16 | ~1,800 (max 3,709) | astronomical (serial) |

The break reproduces exactly: a partial prover at ε=0.25 **passes** the full
space-time audit (`VerifySpaceTime` PASS, answer built in ~100 ms of recompute on top
of the ~920 ms VDF baseline). Your ε*≈0.20 sits just below the **work** knee (~0.25).

## The refinement (the part that's your call)

A timed window that is **secure against a parallel adversary** — the property Fisch's
tight construction proves — bounds the prover's **sequential depth**, not its total
work (a parallel prover with W cores pays ≈ depth, not work). Our data splits the two:

- **Work** explodes at the knee ε≈0.25 (→ millions of labels). A *serial/rational*
  attacker (one who just deleted disk to save money, not a well-resourced parallel
  farm) is work-bound, so a knee-calibrated timed window **does** fail it past ε≈0.20–0.25.
  This is the regime your answer targets ("failing-past-it bound against the rational
  attack point"), and it holds.
- **Depth**, which is what a *parallel*-secure timed window can actually bound, stays
  **small at the knee** (~30 at ε=0.25) and only becomes large (~1,800) near ε≈0.50.
  So against a parallel adversary the timed window only bites around **ε≈0.4–0.5**, not
  0.20–0.25.

**Consequence for the claim:** the *enforced* ε* depends on the attacker model —
~0.20–0.25 vs a serial/rational attacker, ~0.4–0.5 vs a parallel one. The **disclosed**
ε* in the C1 restatement can still be 0.20 (the rational attack point, and the floor of
what the graph concedes), but we should be precise about what "enforced" means. Two
honest framings, and picking is a claim-wording call:

- **(B-serial)** Restate C1 as `≥(1−0.20)·q·C_honest`; the knee-calibrated timed window
  **enforces** this against the serial/rational attacker (the realistic disk-saver);
  own — in the same C1 residual row — that a **parallel** adversary can push toward the
  depth knee (~0.5) and that closing *that* gap is Option A (stacked tight-PoS + SNARK,
  H-track). This is our lean: it matches the realistic threat, keeps ε*=0.20, and is
  fully honest about the parallel residual.
- **(B-parallel)** If the composition must hold ε* against a parallel adversary in M0,
  ε* has to be set to the **depth** knee (~0.4–0.5) — a much larger, worse disclosed
  discount — because that's the most a timed window can enforce without Option A.

We do **not** think (B-parallel) is right for M0 (a ~50% disclosed discount is worse
than the honest ~20% one, and the parallel attacker is the less realistic disk-saver),
but the "is ε*=0.20 *enforced* or *disclosed-with-a-parallel-caveat*" distinction is a
claim-level statement in the composition proof, so we're surfacing it rather than
choosing.

## What we'll build once you confirm the framing (default = B-serial unless you say otherwise)

1. **Restate C1** as `≥(1−ε*)·q·C_honest`, ε*=0.20, in `m0.md` + a new `owned-residuals.md`
   C1 row (class research-frontier; Option A = the tight close; the parallel-adversary
   residual named explicitly per B-serial) + `m0-sybil-rebind §8.1` xref + CHANGELOG.
2. **Enforcement:** a knee-calibrated reply-latency bound on the live bond challenge
   (`core/node/bondaudit.go` times challenge→reply; a reply implying recompute past the
   knee is rejected, standing not minted) calibrated so honest (recompute≈0) passes with
   margin and a >ε* **serial** prover fails. Config knob + narration.
3. **Regression:** adapt the red team's `c1_recompute_regression_test.go` to the timing
   enforcement — green condition "a >ε* serial prover's answer-build exceeds the budget",
   **not** "any ε>0 fails" (unachievable on a single-layer graph; there is **no
   deterministic content check** for partial recompute — recomputed bytes are
   content-identical to stored, so `VerifySpaceTime` as a content predicate can never
   catch it; enforcement is inherently the timing/sequential-depth leg).

**One crisp ask:** confirm **B-serial** (ε*=0.20 enforced vs the rational attacker,
parallel residual owned → Option A) — or tell us to pin ε* to the parallel depth knee.
We proceed on B-serial as the default if we don't hear otherwise; meanwhile we're
building the rest of the red-team backlog (the other decided items) so this doesn't
block the marathon.

## Note
No `VerifySpaceTime` content check can distinguish stored from correctly-recomputed
bytes — confirmed against the code. Any enforcement is the timing/sequential-depth leg;
the tight (parallel-secure, small-ε*) version is Option A. This follow-up only pins
*which ε* the M0 timed leg can enforce*, from measured depth-vs-work data.
