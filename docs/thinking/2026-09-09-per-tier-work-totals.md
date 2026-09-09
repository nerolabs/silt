# 2026-09-09 — Per-tier work totals: giving the edge-majority tenet a source

**Context / trigger:** the Economist advisory
`/Users/andrewedmond/Claude/claude/silt-reviews/economist/ADVISORY-c3-concentration-gate-thresholds-redderived-2026-09-09.md`
withdrew both C3 concentration thresholds and named ONE build item as the thing that re-founds them
(§3a, Builder item 1). It is not deferred by `D-WORK-VISIBILITY` (ratified 2026-09-09), precisely
because the harness sets the counters directly, so a per-tier serve total is exactly what a harness
gate needs.

---

## The mechanism, stated before the fix

**The failure is that T-AR has no source, because the tenet is a TIER SHARE and every published
concentration figure is a PER-NODE dispersion statistic.** `docs/TENETS.md` Part IX: "the edge tier
that does the **majority of the work** must remain a net-positive place to do it". On main before
this change, `node.EconomySample` carried `Mix` (a count per tier), `ServeGini`, `ServeSampleSize`
and `ServeWorkTotal` (the sample-wide SUM). No per-tier work quantity, on the struct or on either
gossip-estimated route. So the tenet's own number could not be computed by anything, and the two
Ginis beside it answer a different question — on a tier design that spans a 500:1 capacity
dispersion, an elevated Gini is not a violation of anything silt has ratified.

**The evidence that this is a real hole and not a tidiness argument** is that the two statistics were
measured DISAGREEING on one sample, in both directions:

| fixture | serve Gini verdict | pony share of served bytes | which is the tenet |
|---|---|---|---|
| disk-weighted ceiling, 4,096 nodes | `G_adj` 0.1726, CONCENTRATED | 0.8184 | the share — far above the floor |
| A2 concentrated, 1,011 nodes | `G_adj` 0.7941, CONCENTRATED | 0.1998 | the share — far below the floor |

**And the trap is already pinned.** `TestGateC3_1b_TierMixShareIsNodeCountNotServedBytes` measured
`mix[pony].share = 0.9891` against a true byte share of `0.1998` on that second fixture. `share` is a
share of NODE COUNT and under the ratified 10000:100:1 vision ratio it is ~0.99 by construction, so
anything wiring it to the tenet floor passes total serve capture.

**The change addresses it by** accumulating per-tier serve/repair/pledged totals in the SAME `add`
closure that already has `(tier, srv, rep)` in hand — past the M-2 exclusion return, so the totals are
over exactly the population the two Ginis are over — and publishing SHARES derived from them.

---

## Options weighed

- **(A) A third gossip field carrying a self-declared tier label.** REFUTED, and not by me: R2.2 row 10
  forbids it and the reason stands — all three numbers are self-reported, so a self-declared label adds
  attack surface and buys nothing a band over the pledged bytes does not already give. The class stays
  DERIVED from `CapTotal`.
- **(B) Publish absolute per-tier byte totals.** REFUTED by the advisory and by the shape of the
  reconstruction break already closed in `r22_gini_reconstruction_test.go`: a per-tier total plus n−1
  sybil-supplied terms recovers the n-th in ONE SUBTRACTION, which is strictly easier than the Gini
  inversion. Shares are no safer in kind (one equation, one unknown), which is exactly why they ride the
  EXISTING `gossipWithheld` marker with no new marker and no new clause — the same covered set one
  derivation removed.
- **(C) Publish the shares AND a threshold on the panel.** Rejected on `D-WORK-VISIBILITY`: no production
  concentration alarm is claimed, because none can fire on a default fleet. The route publishes the
  MEASUREMENT; the harness holds the floor. This also keeps an economic decision out of the Builder seat.
- **(D) Shares with the `known` discipline, floors in the gate. CHOSEN.**

---

## The three things this build had to get right, and how each is gated

### 1. The exclusion hole, one level down

Under the certified M-2 rule (research certification
`C3-GOSSIP-DISCLOSURE-vs-D-UI-PRIVACY-FLAG-2026-09-09`) a peer that reported NEITHER counter is excluded
from the work series rather than counted as a zero. Push that down to a per-tier share and a new failure
appears: **a tier ALL of whose peers are silent contributes 0 to the numerator while OTHER tiers hold the
denominator up, so its share computes to a perfectly well-formed 0.0.** That number says "this tier does
none of the work". The truth is "no peer of this tier told me anything". On the shipped `-privacy`
default that is every tier, every time.

So `tierShareOf` checks REPORTERS before it checks the denominator, and the order is the rule rather than
defensive plumbing — with the denominator checked first, the false 0.0 is emitted on every network where
some other tier reported. `ReportersByTier` is the field that tells a named absence from a measured zero,
and both cases are driven:

- a tier with no reporter is ABSENT from all four maps and renders `known:false` with its reason;
- a capable tier that reported and has never repaired IS present with a `0`, and that zero publishes.

### 2. A claim about a boundary is a measurement — and the advisory's boundary was wrong

The advisory retains the 0.50 floor with a validity condition:
`ponyServeShare >= min(0.50, 0.8 × expectedPonyShare(sampled mix, disk weighting))`, and says "at every
sample of ≥ ~560 classifiable nodes this reduces to the flat 0.50 tenet floor".

**Run it and it does not.** `min(0.50, 0.8·E) = 0.50` requires `E ≥ 0.625`, not `E ≥ 0.50`. On the
vision-ratio family (100k ponies : k horses : 1 archival, disk-weighted 0.1/1/50 TB),
`E(k) = 10k/(11k+50)`:

| k | n = 101k+1 | mix | E (null) | (4/5)·E | floor = min(0.50, (4/5)E) |
|---|---|---|---|---|---|
| 4 | 405 | 400:4:1 | 0.425532 | 0.340426 | 0.340426 |
| 5 | 506 | 500:5:1 | 0.476190 | 0.380952 | 0.380952 |
| 6 | 607 | 600:6:1 | 0.517241 | 0.413793 | 0.413793 |
| 9 | 910 | 900:9:1 | 0.604027 | 0.483221 | 0.483221 |
| **10** | **1011** | 1000:10:1 | **0.625000** | **0.500000** | **0.500000** |
| 11 | 1112 | 1100:11:1 | 0.643275 | 0.514620 | 0.500000 |

**Two different boundaries for two different claims.** `n ≈ 562` (where `E` crosses 0.50) is the point
below which a BARE 0.50 floor false-fires on an honest network. `n = 1,011` is the point at which the
CONDITIONED floor becomes the flat 0.50. The advisory named the first and used it for the second.

And the fixed point is real: `0.625 × 4/5 = 2.5/5 = 0.5` exactly. That is why the margin ships as the
ratio `4/5` and not the literal `0.80` — 0.8 is not representable in binary, so with the literal, the
k = 10 row of that table is decided by a rounding mode rather than by the economics. A fixed point has no
mutation to ablate, so the gate is the table of literal values, checked at the endpoints and one step
either side, and never `want := theFunctionUnderTest(...)`.

The disagreement is also driven through the real routes: an honest 554-node disk-weighted sample
publishes `0.499089` — below the bare floor, above the conditioned floor `0.3993`.

### 3. Which direction the figure errs, derived rather than asserted

The Gini's coverage correction is a two-sided identity, `G_adj = (1−c) + c·G_pub`. **A tier share has no
such identity**, and this is a correction to the framing rather than to a number: the true share is
`(P + P_s)/(P + P_s + O + O_s)`, and `O_s` — work done by silent NON-pony peers — is unbounded above, so
the observed share is neither an upper nor a lower bound on the truth in general.

What IS derivable is the one-tier case: if only ponies go silent, `(P−d)/(P+O−d) < P/(P+O)` for `d, O > 0`,
so **edge silence depresses the edge's own share** — a false alarm that names itself in `coverage`, never a
false clean bill. Measured: 100/194 = 0.5155 with the edge fully reporting, 50/144 = 0.3472 with half of it
silent, on the same underlying network. The gate reports that second arm INDETERMINATE, not a violation.

Because the identity does not exist, the coverage clause stands in for the bound. It REUSES
`c3ServeReportingMin` (= 1 − `c3ServeGiniMax` = 0.85) and introduces no new parameter: it is the same serve
series over the same population, so it inherits that series' own indeterminacy boundary.

---

## What was withdrawn, and what is now re-foundable

| withdrawn | re-founded by | status |
|---|---|---|
| `ponyShareOfServedBytes ≥ 0.50` (RETAINED but unbuildable) | `concentration.ponyShareOfServedBytes`, gated by `TestGateC3_3_EdgeMajorityOfServeWorkIsTheTenetNotTheNodeShare` | **BUILT.** The primary T-AR gate. |
| `serveGini ≤ 0.15` as a field alarm | mix-conditioned null (`ptExpectedPonyShareDiskWeighted`) | the null is built; the Gini's mix-conditioned alarm is the Tester's to point |
| `repairGini ≤ 0.40` as a field alarm | `repairShare` vs `pledgedShare`, gated by `TestGateC3_3d_RepairShareIsRelativeToHoldingsWhichIsWhatTheWithdrawnConstantCouldNotBe` | **BUILT** and it separates: 0.0000 on the holdings-proportional honest shape (where the published Gini is 0.7740 and the withdrawn constant fires), +0.8649 on capture |
| the cross-document repair coverage join | `concentration.capableSize` | **FIXED**, gated by `TestGateC3_3e_CapableSizeClosesTheCrossDocumentRepairCoverageJoin` |
| `caretakerSetSize ≥ 3` | — | withdrawn from this lane by the advisory; not built, and must not be published on a gossip route (Don't #3) |

The `0.20` repair margin stays the advisory's `[ASSUMPTION]`, labelled as such in the gate and owed a
re-derivation on the first field data. It is not this seat's number.

---

## The residual, stated plainly

**On the shipped default every figure in this change is a NAMED ABSENCE.** `cmd/silt/daemon.go` welds
`PublishWorkCounters` to `!privacyOn`, so no node gossips its counters, the exclusion drops every peer,
and the per-tier block reads `not reported` for every tier. That is expected, ratified under
`D-WORK-VISIBILITY`, and said out loud on the panel rather than rendered as a row of zeroes. What this
change buys is that the harness — where the operator sets the counters — now has the tenet's own number
to grade, and that when the committed-ledger route is opened before the cut, the surface that consumes it
already exists.
