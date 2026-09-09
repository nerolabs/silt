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

So `tierShareOf` has a REPORTERS test, and its EXISTENCE is the rule. **The first draft of this
paragraph said the ORDER was the rule, and running the ablation refuted that in one command:** swapping
the reporters test behind the empty-denominator test came back GREEN, because with a positive denominator
the swapped code still reaches the reporters test and still refuses. Deleting the reporters test is what
publishes the false `known:true, value 0.0000`, and that is the ablation the gate is written against.
The ORDER is observable on exactly one input — a wholly silent network, where both refusals are live —
and there it decides which reason an operator reads. That input is the shipped `-privacy` default's own
shape, so it has its own arm (G-PT-2b). `ReportersByTier` is the field that tells a named absence from a
measured zero, and both cases are driven:

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

The fixed point is real: `0.625 × 4/5 = 2.5/5 = 0.5` exactly. **The reason I first gave for spelling the
margin as `4/5` rather than the literal `0.80` was FALSE, and it is the fourth false sentence on this
branch** — see the through-line below. The struck claim was that with the literal, the k = 10 row would be
"decided by a rounding mode". Measured, by the Economist and the blind PE independently and then by me:
`fl(0.8) = 0x3fe999999999999a`, the exact product `0.625 · fl(0.8)` is `0.5 + 2⁻⁵⁵` — a *quarter* ulp,
where half an ulp is `2⁻⁵³` — so it rounds to exactly 0.5 under round-to-nearest, which Go mandates and
exposes no mode for. Both forms give bits `0x3fe0000000000000` at k = 10 and substituting the literal
leaves every arm in the file green.

The true reason to keep `4/5` is a preference, not a hazard: `E*4` is exact (a power-of-two scaling) and
the single division that follows is correctly rounded, so `E*4/5` **is** the correctly-rounded `4E/5`
while `E*0.8` carries two roundings. They do differ, by one ulp, *off* the fixed point — measured over
240,000 mixes of this family, 83,512 of them (34.8 %), including the k = 4 row of the table above, where
`min()` differs too. Nothing depends on it: the `min()` clamp absorbs an upward ulp at 0.50 and a downward
ulp only loosens the floor, which is the false-PASS direction and never a false RED.

A fixed point has no mutation to ablate, so the gate is the table of literal values, checked at the
endpoints and one step either side, and never `want := theFunctionUnderTest(...)`. The exactness
assertion is re-pointed at what it *does* pin: that k = 10 is a true fixed point rather than a near-miss
inside the table's tolerance — and it reddens when the tenet floor moves (0.50 → 0.51 reddens it and the
k = 11 row).

The disagreement is also driven through the real routes: an honest 554-node disk-weighted sample
publishes `0.499089` — below the bare floor, above the conditioned floor `0.3993`.

**Six sentences on this branch were false, every one found by EXECUTING the claim, five of them mine.**
The advisory's `n ≈ 560`; my ordering claim in §1; my three revert descriptions naming a reddening point
they had not been run at; my `4/5` rounding-mode rationale; and the universal in §4a — *"cannot buy a
pass"* — which one decoy node defeats. Every gate was green through every draft, which is the signal
rather than the exception: **a gate checks the code and nothing checks the sentence.**

**And the pattern is now fully general, which it was not after four.** Every one of the six was a claim
about what a mechanism *defeats* or *covers*, and every one fell to someone RUNNING THE ADVERSARY rather
than reading the argument. So:

> A bound is a theorem and can be proved on paper. **"This defeats X" is a MEASUREMENT against X, and you
> do not have it until you have run X.**

The `4/5` and the `cannot buy a pass` sentences are the two worth dwelling on, because in both cases the
underlying mathematics was sound and independently re-derived by two other seats — the interval bound
holds, the fixed point is exact — and the false part was the sentence about what that mathematics *bought*.
Correct code and a correct derivation are no protection at all against an overclaimed consequence. The
defence is the habit: if a sentence names a number, a boundary, a reddening point, or an adversary, run it
before publishing it. And if an ablation comes back GREEN, the sentence is wrong, not the ablation.

### 3. Which direction the figure errs, derived rather than asserted

The Gini's coverage correction is a two-sided identity, `G_adj = (1−c) + c·G_pub`. **A tier share has no
such identity**, and this is a correction to the framing rather than to a number: the true share is
`(P + P_s)/(P + P_s + O + O_s)`, and `O_s` — work done by silent NON-pony peers — is unbounded above, so
the observed share is neither an upper nor a lower bound on the truth in general.

What IS derivable is the one-tier case: if only ponies go silent, `(P−d)/(P+O−d) < P/(P+O)` for `d, O > 0`,
so **edge silence depresses the edge's own share** — a false alarm that names itself in `coverage`, never a
false clean bill. Measured: 100/194 = 0.5155 with the edge fully reporting, 50/144 = 0.3472 with half of it
silent, on the same underlying network. The gate reports that second arm INDETERMINATE, not a violation.

Because the identity does not exist, a coverage clause has to stand in for the bound. **The first form of
that clause was the sample-wide reporting fraction against 0.85, and it is structurally unable to do the
job** — the Economist priced it on this file's own concentrated fixture and it is the sharpest finding of
the round. See §4.

---

## 4. Two silence floors, both derived, and neither implies the other

### 4a. Per-tier COVERAGE — the concentrating tier must buy decoys to buy a pass

`[MEASURED]` on this file's own `ptConcentratedPeers()` fixture (1000 : 10 : 1, top-5 horses serve 80 %):

| arm | silenced | sample-wide fraction | edge share | verdict |
|---|---|---|---|---|
| all report | — | 1.0000 | 0.1998 | EDGE-MINORITY (correct) |
| top-5 horses withhold | **5 nodes, 0.49 % of the sample** | 0.9951 | **0.9940** | **PASS** |

The attack costs nothing: withholding is the compiled default. And the sample-wide clause is
**structurally** unable to reach it, not merely mis-tuned — its floor needs 15 % of the sample silent, and
under the ratified 10000:100:1 target the non-edge tiers *are* the sample's one percent (1.088 % at
1000:10:1, 0.9999 % at 10000:100:1). **The target ratio the gate defends is what makes the tiers whose
silence matters invisible to a count-weighted coverage measure.**

The fix makes the gate an **interval**. Under a named assumption — within a tier, silence is uncorrelated
with work rate, the analogue of the Gini's "silence means idle" — with `φ` the least coverage among the
tiers present:

```
R_t ≤ R_t/c_t ≤ R_t/φ   for every tier
⇒   φ · s_obs  ≤  s_true  ≤  s_obs / φ
```

PASS iff the lower end clears the floor, EDGE-MINORITY iff the upper end is below it, INDETERMINATE iff it
straddles. Three cases, exhaustive, no fourth. **And the coverage floor is a theorem, not a parameter:**
`s_obs ≤ 1`, so a PASS requires `φ ≥ φ·s_obs ≥ F` — the minimum per-tier coverage is the tenet floor
itself, exactly as the Gini's `1 − T` boundary is a theorem of `T`. Endpoints run at 4 / 5 / 6 of ten
reporting horses, where the boundary sits at `φ = F = 0.50` exactly.

The sample-wide clause is **removed rather than kept as a belt**: it is dominated, and where the two
disagree it is wrong — every tier at coverage 0.6 with `s_obs = 0.95` gives a lower bound of 0.57, a sound
PASS, which the 0.85 fraction refuses.

#### And the interval PRICES this attack; it does not close it. The price is one node.

The heading of this section used to say *cannot buy a pass*, and that was a universal proved on one
fixture. The blind PE ran the same attack with the concentrating operator also running decoy nodes **in
its own band** — nothing else changed, same silenced horses, same true edge share. `[MEASURED]`, and this
table is now an asserted arm of the gate rather than a sentence:

| decoys | horses (reporting) | φ | interval | floor | verdict | TRUE edge share |
|---|---|---|---|---|---|---|
| 0 | 10 (5) | 0.5000 | [0.4970, 1.9881] | 0.5000 | **INDETERMINATE** | 0.1998 |
| 1 | 11 (6) | 0.5455 | [0.5417, 1.8206] | 0.4969 | **PASS** | 0.1997 |
| 2 | 12 (7) | 0.5833 | [0.5787, 1.7007] | 0.4938 | **PASS** | 0.1997 |
| 5 | 15 (10) | 0.6667 | [0.6594, 1.4837] | 0.4848 | **PASS** | 0.1996 |

**One node.** The `d = 0` arm refuses by 0.0030, and a single decoy raises `φ` past the floor while *also*
enlarging the tier, which drags the mix-conditioned floor down. The attack is helped twice.

**This is not a defect in the derivation** — the PE re-derived both ends and the `φ ≥ F` theorem
independently and they hold. The defect was in what I claimed the mechanism *defeats*. The bound's
purchase price is the named assumption — within a tier, silence is uncorrelated with work rate — and this
attack is *defined* by silencing the highest-work members of a tier, so it violates the assumption by
construction. Under that violation `φ·s_obs` is not a lower bound at all. **The interval defends against
INNOCENT under-reporting, which is the common case, and against a deliberate silencer it costs one node
per silenced band.**

That price is real and not zero: the adversary must now run and keep reporting plausibly from nodes in the
band it is hiding in. But it is a price, not a closure, and the gate is named and asserted for what it
proves. This is the same shape as the reporters floor's *"parity, not closure"* and the four limits on
`ptWorstRepairExcess` — it is the one place I had not applied it.

### 4b. Per-tier REPORTERS — a share over one reporter is that peer's counter

A different quantity, found by the blind PE, and neither floor implies the other: a tier of one node fully
reporting has coverage 1.0 and one reporter; a tier of 1,000 with 999 silent has one reporter too.

`[MEASURED]` at `Size: 100, ServeSampleSize: 2` — one honest pony at 300 GiB, one Sybil horse at 1000:

```
serveGini published?     false      <- suppressed by ITS OWN floor, on the same document
ponyShareOfServedBytes:  known=true value=0.230769231 reporting=1
INVERSION: 1000/(1−share) = 1300  ->  the single honest pony served exactly 300
```

One division. Strictly easier than the two-term Gini inversion `minGossipSample` exists to stop, reaching
the same reader that floor still defends. My own comment asserted the opposite of what the code did — "a
tier share resolves to no individual peer's counter" — while the field itself published `reporting: 1`.

**The floor is `minGossipSample` unchanged.** Its derivation is about how many terms a reader does not
already know: at n = 1 the aggregate *is* the value, at n = 2 it resolves to two named peers, 3 is the
smallest where neither holds. Applied to a tier's *reporter count* the same three cases give the same
answer — the same constant reaching a population it had not been applied to, not a second parameter.

**It buys parity, not closure.** At three reporters an adversary holding two sybils in that band still
recovers the third, exactly as four sybils recover the Gini's secret. Reconstruction is closed by
`gossipWithheld`. What this closes is the *asymmetry* of one document suppressing `serveGini` at two
reporters while publishing a figure that inverts to one.

**A consequence worth stating rather than hiding:** at the vision ratio the archival tier is one node, so
it never clears a three-reporter floor inside `maxPeerInfo = 4,096`. **The repair alarm is therefore
structurally dark on a vision-ratio sample.** It grades a harness topology, which is what
`D-WORK-VISIBILITY` asks of it.

### The rule both are instances of

The Economist states it once so it is not rediscovered a third time, and it is worth carrying:

> Every concentration statistic carries a coverage refusal at its **own granularity**, and is never
> aggregated above the granularity at which capture can occur.

---

## What was withdrawn, and what is now re-foundable

| withdrawn | re-founded by | status |
|---|---|---|
| `ponyShareOfServedBytes ≥ 0.50` (RETAINED but unbuildable) | `concentration.ponyShareOfServedBytes`, gated by `TestGateC3_3_EdgeMajorityOfServeWorkIsTheTenetNotTheNodeShare` | **BUILT.** The primary T-AR gate. |
| `serveGini ≤ 0.15` as a field alarm | mix-conditioned null (`ptExpectedPonyShareDiskWeighted`) | the null is built; the Gini's mix-conditioned alarm is the Tester's to point |
| `repairGini ≤ 0.40` as a field alarm | `repairShare` vs `pledgedShare`, gated by `TestGateC3_3d_RepairShareIsRelativeToHoldingsWhichIsWhatTheWithdrawnConstantCouldNotBe` | **BUILT** and it separates on two hand fixtures — 0.0000 on the holdings-proportional honest shape (where the published Gini is 0.7197 and the withdrawn constant fires), +0.9505 on capture — **but see §6: it is sound as shipped and vacuous as generalised, and the Economist has since withdrawn both the 0.20 and the claim that it answers the recentralization question** |
| the cross-document repair coverage join | `concentration.capableSize` | **FIXED**, gated by `TestGateC3_3e_CapableSizeClosesTheCrossDocumentRepairCoverageJoin` |
| `caretakerSetSize ≥ 3` | — | withdrawn from this lane by the advisory; not built, and must not be published on a gossip route (Don't #3) |

The `0.20` is no longer an assumption to be re-derived — it is **WITHDRAWN outright**, and what replaces
it is not a number. See §6.

---

## 6. What the repair statistic does not catch, and what is OWED rather than done

The Economist re-priced this after the blind PE bounded it, and I am recording the outcome rather than
building it: **the coding job on this branch was the two PE blockers and the edge-share coverage refusal.**

**Withdrawn here, and it is the Economist's own substitution being withdrawn:**

| Withdrawn | Why |
|---|---|
| `margin = 0.20` on `observed − expected`, **outright, not re-priced downward** | an additive excess is tier-incomparable. The ceiling is `1 − expected(tier)`, so on one 1000:10:1 sample the ceilings are 0.8649 (horse) and **0.1351** (archival) — a 6.40× spread. **No value works**, and any value low enough to catch archival capture is inside the horse tier's unmeasured honest excursion. |
| "one tunable margin, no sample-size dependence" | false: the excess is **identically 0.0000** on a sample with exactly one capable tier, for every possible repair distribution — ~59.5 % of honest 4,096-node draws, since a uniform 4,096-of-10,101 draw contains the single archival node only 40.5 % of the time |
| that the excess answers the **recentralization** question | it conditions on holdings, so recentralization living **in the holdings** is invisible to it by construction. On this file's own honest fixture one tier holds 0.9505 of the repair-capable bytes before a single repair is routed, and nothing in this lane gates that. The raw Gini asked *"is repair piling onto the few?"*; the excess asks *"is repair allocated other than by holdings?"* — the second was adopted for having a convenient null of 0. |

**Measured on this branch, and driven rather than asserted:**

- the ceiling on this file's fixture is **+0.0495** for the archival tier — total capture, undetectable, any distribution;
- routing the whole horse tier's repairs onto **one** horse reads **+0.0000 at the tier**, at coverage 1.0, so no coverage refusal can see it. A tier statistic replacing a node statistic loses within-tier concentration.

**What ships now** is the doc block on `ptWorstRepairExcess` recording all four limits, and the constant
renamed `ptWithdrawnMargin` so its name refuses the promotion the Economist warns about: a later seat
reads a passing test and adds an "archival capture" arm to it.

**Owed elsewhere, filed here so it is not lost:** a graded harness run must seat **≥ 3 archival nodes**,
or the repair alarm refuses the topology (§4b). That is a fixture contract discovered by this gate, and it
belongs where the harness is configured rather than only in a test comment — the blind PE files it as a
residual row (N-9). Two further non-blocking PE items are fixed in this branch: the `minGossipSample`
derivation paragraph now argues parity rather than restating the Gini's cases as though they carried over
(N-7), and `ulp(0.5)` is labelled correctly (N-8).

**OWED, filed, not built here:**

1. **The normalisation** `N = (observed − expected)/(1 − expected)`, which reads exactly 1.0 for total
   capture in *every* tier and 0.0 for exactly-holdings, so **any margin below 1.0 is non-vacuous** — and
   that needs no data. `known:false` when `expected = 1` (the one-capable-tier regime).
2. **Node granularity in the harness** — `max over capable nodes of Nᵢ`. The **field keeps the tier form**,
   deliberately: a per-node work figure joined to a node identity on a gossip surface is the
   `(peer × object)`-adjacent access record the Economist has refused twice. Node-level is available
   precisely and only where publication is not happening.
3. **Capable-set pledged concentration published beside the excess** — the condition next to the
   conditional. `pledgedShare` is already on the wire from this build, so it needs no new field. **No
   threshold**, the Economist having no honest null for a pledged distribution.
4. **The discharge for any future margin:** measure the honest `N` distribution across a *family* —
   repair drivers (uniform / holdings-proportional / heat-skewed cold-tail), mixes sweeping `k` and the
   archival count 0/1/2 because the honest distribution is bimodal, seeded churn — and set the margin to
   the **widest honest excursion plus the sampling spread, never a midpoint**, which is exactly how the
   0.40 died. None of it needs a live network; repair is loss- and placement-driven and the harness
   controls both. Interim, parameter-free: gate on total capture only, `N ≥ 1 − ε`, and record the rest.

---

## The residual, stated plainly

**On the shipped default every figure in this change is a NAMED ABSENCE.** `cmd/silt/daemon.go` welds
`PublishWorkCounters` to `!privacyOn`, so no node gossips its counters, the exclusion drops every peer,
and the per-tier block reads `not reported` for every tier. That is expected, ratified under
`D-WORK-VISIBILITY`, and said out loud on the panel rather than rendered as a row of zeroes. What this
change buys is that the harness — where the operator sets the counters — now has the tenet's own number
to grade, and that when the committed-ledger route is opened before the cut, the surface that consumes it
already exists.
