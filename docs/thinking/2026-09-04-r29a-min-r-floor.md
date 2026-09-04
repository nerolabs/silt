# R2.9a — the minimum-requester suppression floor for `B_bootstrap`

**Date:** 2026-09-04 · **Seat:** BUILDER (deliberation → build) · **Status:** BUILT 2026-09-04 on branch `builder/r2.9a-min-r-floor` (PR pending); §6 records the build as it landed

**Binding input.** `silt-reviews/research/research-outcome/R2.9a-Bbootstrap-DELTA-contamination-privacy-floor-clock-RESEARCH-CERTIFICATION-2026-09-04.md`
— §2.1–§2.4 (the floor and its reasoning), §1.1–§1.3 (the census mixture and the refuted
discriminator), §5 gate **G-BB-11**, §6 Tester gates **BB-15, BB-16, BB-18, BB-19**. Overriding:
`docs/TENETS.md` Part 0, Part VI (Don't #3) and Part IX; `docs/build-process.md`;
`internal/depcheck` (`core/` may not import adapters or effects).

**Scope, stated as a refusal.** Five items were certified with derived numbers and no owner input;
they are what this branch builds. Everything else on the DELTA certification depends on `W`, on the
population `P`, or on an owner ruling, and **none of it is built here**: G-BB-9 (pin `P`), G-BB-10
(a handoff item), G-BB-12 (a deployment instruction), G-BB-13 (an owner veto gate on a bright
line), G-BB-14, G-BB-15, G-BB-16, BB-17. `W`, `q`, `P` and `R_min` above the derived floor are **not
pinned here**. The histogram shape, the bins, the edges, the default-off posture, the `Register`
stamp site and the two-clock cross-check are **untouched**.

## 1. The mechanism, in one paragraph

The published `B_bootstrap` block is attributable at a small census **because** `requesters` is the
anonymity-set size, and silt already published the aggregates that a set size makes readable:
`stats.bytesServed` (`cmd/silt/ui.go`, unconditional) and `durability.objects[].funded`
(`core/credit/escrow.go`, moved by the skim on each served object) both **predate the instrument and
are not behind its flag**, so at one requester their deltas already say "the single requester
fetched X bytes of object Y". The instrument's contribution is the number that tells an observer the
set is degenerate. This change addresses that by withholding **every census-derived quantity** —
`cells`, `aged`, `requesters`, `unstamped`, `maxOccupiedAgeEdgeNanos` — whenever the census holds
fewer than `R_min` requesters, and publishing `suppressed: true` in their place. Suppressing the
grid while still publishing the count would close nothing, which is the whole reason the rule is
written over the census rather than over the cells.

## 2. The two objects a reader must not fuse

The prior certification **refused** low-count cell suppression, and that refusal stands. The two
rules are about different objects and the code says so at the site:

| | What is suppressed | Why |
|---|---|---|
| **Refused** (prior cert §5.2) | an individual **low-count cell** | suppression eats exactly the tail the quantile fit reads, and the bias is not correctable afterwards |
| **Built** (DELTA §2.3, G-BB-11) | the **whole block**, below `R_min` | the census is not a population; below `R_min` there is no tail to eat |

## 3. `R_min` is derived, not chosen — and the derivation is what ships

Estimating a `q`-quantile needs at least `⌈1/(1−q)⌉` observations **in the read cell**. The read
cell is one of eight age buckets, so the census must carry far more than that. For any `q ≥ 0.90`
the requirement is `≥ 10` in one cell, hence far more than 10 across the census, so a **census-wide**
floor of 10 is strictly dominated by the fit's own requirement and **costs the fit nothing**.

`q` is the owner's (G-BB-1) and is **not pinned here**. What is encoded is the certified edge of the
range the derivation holds over, and the ceiling itself, as constant arithmetic:

- `bbDerivedQuantileFloorPercent = 90` — **not `q`**. The lower edge of the range the certification
  covers. At `q` below it the derivation must be re-run, and the constant says so.
- `bbQuantileObservationFloor = ceil(100 / (100 − q%))`, written as the integer ceiling
  `(100 + (100−q%) − 1) / (100−q%)`, which is 10 at the edge and re-derives if the edge moves.
- A **compile-time guard**, `const _ = uint(BBootstrapMinRequesters - 10)`, so a future edit that
  drives the derived floor below the certified 10 fails to build rather than fails a review.

`TestR29aBB15FloorIsDerivedNotChosen` recomputes `⌈1/(1−q)⌉` by counting up rather than by the same
ceiling expression, so the constant and its stated derivation cannot drift apart silently.

## 4. Where the floor is applied — the options, and the call

The floor is about **publication, not recording**. §2.4 is explicit: the serving operator's own
process already holds `fetchedBytes` keyed by `NodeID` and every `ChunkID` it answered, so a
node-local read gives the operator nothing it does not have; the leak exists only against a reader
who is not the operator.

| Option | Cost | Verdict |
|---|---|---|
| **A — inside `BBootstrapSnapshot`** | the operator's own census becomes lossy, and `BBootstrapRunPrecondition` loses the empty-census arm it needs to refuse a void run | rejected |
| **B — in `cmd/silt/ui.go` only** | correct today, but the rule lives at the renderer, so a second publisher inherits nothing | rejected |
| **C — at `core/node`'s `Node.BBootstrap` (BUILT)** | one extra call; the rule and its derivation live in `core/credit` as `WithMinRequesterFloor`, applied at the one seam the histogram leaves the ledger by | **chosen** |

C is chosen because it makes the claim structural rather than conventional: the ledger reference
inside `Node` is unexported, so `Node.BBootstrap` is the only route out, and a source gate pins that
neither `cmd/silt/ui.go` nor `cmd/silt/daemon.go` calls the raw reader. `core/credit`'s snapshot
stays the honest operator-local census.

**Absent, not zero.** Below the floor the three count keys and `maxOccupiedAgeEdgeNanos` are
**omitted from the JSON** rather than published as `0`. A published `requesters: 0` under a census
of nine is a false total, and a reader that sums the block takes it as measured; a missing key
cannot be misread. The wire fields are pointers with `omitempty` precisely so a **legitimate** zero
— the instrument on, the node idle — still publishes, which is the same absent-vs-empty distinction
the whole block already draws against `-bbootstrap` being unset.

**What survives suppression, and why.** The clock self-reports, both uptimes, the signed skew and
every corruption flag (`clockStepBack`, `clockSuspect`, `ageExceedsUptime`). They describe the
**instrument**, not the population, they carry no count, and an operator needs them exactly when the
census is too small to publish. `maxOccupiedAgeEdgeNanos` does **not** survive: it is census-derived,
and at a degenerate anonymity set it is the singleton's own age.

**Not built: a separate endpoint.** §2.4 rules separation out. An observer polls two endpoints as
easily as one, so splitting the document buys nothing and adds surface.

## 5. What the floor does NOT close

**`R-BB-DELTA-TRAJECTORY`, open.** A polled series of cell deltas yields single-identity bin
trajectories at **any** R: one identity crossing a bin edge between two polls appears as `−1` at one
bin and `+1` at another. R governs **attribution**, not **extraction**. The bound is the poll rate
and the number of bin crossings per interval, neither of which this instrument controls. The floor
withholds the grid at a degenerate anonymity set, which is a different and smaller claim, and the
code comment, the CHANGELOG and this record all state it as an open residual rather than a close.

**The census mixture stays open too** (`R-BB-CENSUS-MIXTURE`). BB-18 pins that a repair-path fetch
lands in the census indistinguishably from a viewer's. That is asserted as a **fact**, not a bug:
whether repairing peers belong in the estimand's population is G-BB-9, the owner's, and it trades M0
against D-S7.

## 6. As built

- `core/credit/bbootstrap.go` — the derivation constants and the compile-time guard; the
  `Suppressed` field; `WithMinRequesterFloor`; a `BBootstrapRunPrecondition` arm that names the
  floor instead of reporting an empty census.
- `core/node/bbootstrap.go` — the seam applies the floor.
- `cmd/silt/ui.go` — `suppressed` on the wire; the four census fields become pointers, absent below
  the floor.
- Gates: `core/credit/r29a_minr_floor_test.go`, `core/node/r29a_minr_floor_test.go`,
  `cmd/silt/r29a_minr_floor_test.go`.
- Five merged fixtures were below the floor and went RED on the change (two, ten and one requesters
  at the node seam and on the wire); each was raised to a census of ten or more, which is the
  honest fixture for an instrument that refuses to publish a smaller one.

### The ablations (each RED, each reverted)

| # | Controlled revert | Went RED |
|---|---|---|
| A | drop `.WithMinRequesterFloor()` at the seam | BB-15 (node, wire, source gate), BB-16 |
| B | hand-pick `R_min = 12` | `TestR29aBB15FloorIsDerivedNotChosen` |
| B2 | lower the certified `q` edge to 0.50 | **compile error** (`constant -8 overflows uint`) |
| C | suppress the cells but keep the counts (the "closes nothing" defect) | BB-15 core + node |
| C2 | render the counts despite `suppressed` | BB-15 wire |
| D | the serve path stops booking coded-shard fetches | BB-18 (census 1, want 2) |
| E | `RecordServe` also credits the requester's `servedBytes` | BB-19 (both arms) |

BB-16 carries its positive control **inside the test**: the same trajectory oracle run over the
unfloored local snapshots must find the trajectory, measured at 5 single-identity transitions across
bins `80 → 84 → 86 → 88 → 90 → 92` — the same class of read the review measured as
`80 → 84 → 86 → 88 → 89 → 90`. Without that arm, a green result would be indistinguishable from an
oracle that can never fire.

## 7. One disagreement with the certification, recorded

§2.3 words G-BB-11 as "no `cells`, no `aged`, and no `requesters`". Taken literally that leaves
`unstamped` and `maxOccupiedAgeEdgeNanos` on the wire, and both are census-derived: `unstamped` is a
requester total whenever the accounts predate the clock injection, and `maxOccupiedAgeEdgeNanos` at
one requester is that identity's age bucket. Publishing either below the floor would leave the
gate's own claim — no requester total reaches the wire — false. The build therefore withholds all
five, which is the rule the certification's **mechanism** implies rather than a widening of it. This
is a strictly larger suppression than the words specify; it costs the fit nothing, because below the
floor there is no fit.
