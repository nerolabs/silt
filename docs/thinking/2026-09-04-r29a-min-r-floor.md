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
against D-S7 *(CORRECTED 2026-09-05, G-BB-22: NOT an M0 trade — the grant mints balance, standing is
bond-only, free bytes build no bond; the trade is build-immutable #4 against Don't #7 / T-AR / #8. And
the mixture is now a RUN precondition, not a harmless residual — see
`R2.9a-BB-RESIDUALS-tier-asymmetry-W-gate-tension-census-mixture-RESEARCH-CERTIFICATION-2026-09-05.md`)*.

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

---

# ROUND 2 — 2026-09-05: my five-field list was the third enumeration to miss a field

The blind review of `3337e8b` and the re-certification that adjudicated it upheld two merge
blockers and extended each by one. §7 above is the record of me catching the certification's
three-field list and widening it to five. **The five missed two more.** That is the finding, and it
is not a finding about which fields — it is a finding about the FORM.

Three enumerations, one pull request, three misses:

1. the certification's three fields (missed `unstamped`, `maxOccupiedAgeEdgeNanos`),
2. my five (missed `ageExceedsUptime`, one arm of `clockStepBack`),
3. the source gate's two literal file paths, which claimed a whole-tree property a reviewer
   ablated past in five lines.

I do not get to argue with that. §7's confidence — "the rule the certification's mechanism
implies" — was correct about the mechanism and wrong about the method. A list cannot cover a field
nobody foresaw, and I wrote a list.

## 1. The property, and the one design decision in this round

G-BB-11′ partitions the block: an INSTRUMENT field is a function of the injected clock sources,
their injection instants and the compiled axis constants alone; a CENSUS field depends on
`l.accounts` or `l.order`. Below `R_min` the block must be a function of the instrument fields
alone, sole exemption `suppressed`.

The whole design question is **how a rule like that gets enforced on a field written next year by
someone who never reads this document.** Options considered:

| Option | What it does | Why not / why |
|---|---|---|
| Widen the list to seven | Fixes today's two fields | This is what failed three times. Refused. |
| Split the struct into an embedded `Instrument` and `Census` sub-struct | The compiler forces every new field into a class; the floor is a one-line copy | Genuinely the strongest form, and I nearly built it. It touches the reflection field-set audit, the wire renderer and every fixture. **The blast radius buys nothing BB-20 does not already cover**, because the leak only matters once a field reaches the wire, and BB-20 is at the wire. Deferred, not refused. |
| **CONSTRUCT the suppressed block from the instrument class** | The floor returns a fresh `BBootstrapHistogram` carrying only instrument fields; everything else takes its ZERO VALUE | **Chosen.** ~15 lines, no refactor. |

The reason construction is the answer and clearing is not: a zero value is a compile-time constant,
and a constant is trivially a function of the instrument alone. So an unforeseen field satisfies the
property *by default*. **The default flips from PUBLISHED to WITHHELD**, and a new field is
published below the floor only by a deliberate edit to one literal — which is a privacy decision
made in the one place the reasoning lives.

The cost is real and I state it in the code rather than hiding it: a new INSTRUMENT field is also
withheld until someone adds it there. That is a loss of operator signal, never a loss of privacy,
and it is the right way round.

**Measured, and this is the evidence that the form is fixed rather than the fields.** Ablation D
adds a census field nobody had foreseen (`PeUnforeseenCensus`, set from `Requesters`), publishes it
unconditionally on the wire, and does not touch the floor at all:

- shipped construction → **BB-20 GREEN**, the field is withheld,
- floor reverted to the clear-a-list form the review saw → **BB-20 RED**, `peUnforeseenCensus: 7`
  on the wire below the floor.

## 2. The two defects

`ageExceedsUptime` is `MaxOccupiedAgeEdgeNanos > CensoringBoundNanos()` — a **threshold on the very
field the floor withholds**. At a census of one, joined to the surviving `monotonicUptimeNanos`, it
is a lower bound on that identity's age, published below the floor. Withheld. The operator loses
nothing: `clockSuspect` and the raw signed `clockSkewNanos` report the same corruption and read no
account.

`clockStepBack` had two arms and the certification permits split or withhold. **Split**, because it
keeps the operator's whole signal:

- the ledger-start arm (the wall delta went negative) touches no account → INSTRUMENT, keeps the
  name, survives suppression;
- the per-account clamp (a stamp reads later than the clock) → CENSUS, becomes `ageClampedToZero`,
  withheld below the floor, and it gained its own arm in `BBootstrapRunPrecondition` so the split
  does not quietly drop a run-voiding signal.

The split is pinned by its **discriminator**, not just by its own assertion: arm 1 of
`TestR29aBackwardClockStepClampsAndSaysSo` now asserts `ageClampedToZero` fires AND `clockStepBack`
stays DOWN. Fusing them again reddens it, and reddens BB-20's backward-step group too (ablation C).

## 3. M-2 — why the reviewer's fix is insufficient, and what I built instead

The reviewer proposed walking the tree instead of two literals. The re-certification refutes that on
the artifact and I agree with the refutation after checking it myself:

- `core/node/bbootstrap.go` obtained the export through an **anonymous interface assertion on a
  method name**, so the seam is duck-typed and no name pins it;
- a tree walk has to EXCLUDE `core/credit` — that is where the method lives — which is exactly where
  a second exported reader would sit, invisible.

So the close is the type system. `bBootstrapSnapshot` is unexported; `BBootstrapPublish`, which
floors, is the only route out of the package. A duck-typed impostor can still satisfy `core/node`'s
interface, but it can only supply its own data — it cannot reach this ledger's raw census.

**The residual the compiler cannot reach, and what I did about it.** A second exported reader added
*inside* `core/credit` is invisible to the compiler and to any gate in another package. I closed it
with a gate that is package-scope and structural rather than literal:
`TestR29aBBootstrapHasOneExportedRoute` reads the directory, parses **every** non-test `.go` file
with `go/ast`, and requires that the only exported functions returning a `BBootstrapHistogram` are
`BBootstrapPublish` and `WithMinRequesterFloor` (which can only ever floor). Its failure text names
exactly the scope it checks — one package, every file in it — which is the discipline the deleted
gate broke. It carries its own teeth test over a synthetic bypass (a value return and a pointer
return must flag; an unexported method and an unrelated exported method must not), and it goes RED
on the reviewer's exact ablation planted in a THIRD file inside the package, with the build clean.

**The residual I could NOT close, stated plainly rather than papered over.** This is a rule about the
histogram OBJECT, not about census data in general. `FetchedBytes` and `ServedBytes` are exported
per-identity readers that predate the instrument. A future exported method returning census-derived
**scalars of some other type** defeats both the compiler and this gate, and would be caught by BB-20
only once it reached `/api/status`. Filed as **R-BB-EXPORT-SCALAR-BYPASS**, open, bounded by review.
I would rather name it than ship a gate that reads as protection and is not.

**The old source gate is DELETED, not widened.** A gate that re-checks a compiler-enforced fact is
decoration, and decoration is what got ablated.

## 4. BB-20 — RED first, and it found both defects on its own

Built before the fix, run on `3337e8b`:

```
--- FAIL: .../forward-step
    ageExceedsUptime differs: empty=false, one=true      <- M-1
--- FAIL: .../backward-step-past-a-stamp
    clockStepBack differs: empty=false, one=true         <- the certification's extension
```

Two groups red, one per defect, without either field being named anywhere in the gate. That is the
whole argument for a property over a list, demonstrated rather than asserted.

Three fixture rules, all from the reviewer's F-3, which is correct:

- each group replays ONE clock script across every census arm, so clock state is equal **by
  construction**. BB-16's byte-identity arm holds only because its fixture pins `clk.now`; that is
  an accident and must not be cited as a production property.
- `r29aServer` sets `uiServer.peerCount`, a nil func in the `economyServer` fixture.
- the gate ships a **positive control** that drives the same scripts ABOVE the floor and requires the
  blocks to DIFFER. Without it, byte-identity below the floor could be a property of a fixture that
  varies nothing.

## 5. BB-16's control had to move, and it got better

The control was "the same oracle over the operator's unfloored local snapshots". After M-2 the raw
census does not leave `core/credit`, so that arm is gone. The replacement is stronger and is on the
wire: the SAME six fetches by the SAME single identity, on a node whose census has been **padded
over the floor by nine identities**. The trajectory is then readable straight off the PUBLISHED
series.

That is the review's own measured refutation encoded as a permanent gate: nine keypairs and one
fetch each is the entire price of lifting the floor (**R-BB-CENSUS-SYBIL-PAD**), and the fixture
does in eight lines what an adversary does in seconds. The same pad lifts BB-18 and BB-19 over the
floor, which also removes the reviewer's F-3 objection to those two — they now read through the
SHIPPED `BBootstrapPublish` path instead of a raw reader that no longer exists.

## 6. The claim, corrected in four texts

`core/credit/bbootstrap.go`, `cmd/silt/ui.go`, `CHANGELOG.md`, `ROADMAP.md`. Each now says the floor
bounds the census **count**, not the anonymity **set**; that an observer which can fetch buys the
difference for nine keypairs; that the floor is a **fit precondition** and a defence against a reader
that **cannot** fetch; and that `suppressed: true` is itself a disclosure of an upper bound of
`R_min − 1`. The CHANGELOG's *"every corruption flag survives suppression — they describe the
instrument rather than the population"* was flatly false for `ageExceedsUptime` and is gone.

## 7. Round-2 ablations (each RED, each reverted)

| # | Controlled revert | Went RED |
|---|---|---|
| A | the floor copies `AgeExceedsUptime` through again (the reviewer's M-1) | BB-20 `forward-step` |
| B | the floor copies `AgeClampedToZero` through again | BB-20 `backward-step-past-a-stamp` |
| C | un-split the arms: the per-account clamp writes `ClockStepBack` again | BB-20 `backward-step-past-a-stamp`, and arm 1 of the BB-13 clock test |
| D1 | an UNFORESEEN census field, published unconditionally, floor untouched | **GREEN — withheld by default. This is the point of the round.** |
| D2 | same field, floor reverted to the clear-a-list form | BB-20 `ages-and-bytes` + `forward-step`, `peUnforeseenCensus: 7` on the wire |
| E | the reviewer's bypass export planted in a THIRD file inside `core/credit` | `TestR29aBBootstrapHasOneExportedRoute` — with `go build ./...` clean, which is the case a tree walk could never see |

## 8. What I did NOT build, and why

- **G-BB-12′ / G-BB-13′ Part A** — the `-ui` routable refusal or a token gate. That is the owner's
  configuration call and Part B behind it is a VETO GATE on Don't #3. Not mine.
- **`W`, `q`, `P`, `R_min` above the derived floor** — unpinned, the owner's (G-BB-1, G-BB-9).
- **G-BB-17 … G-BB-19** — they bind the `grant/r` pin and the run, not this branch.
- **BB-7's suppressed-block arm (F-1)** — BB-20 subsumes it: BB-7 pins the key set at `R = 40`, and
  the below-floor key set is now pinned by byte-identity across varying censuses. Adding a second
  gate on the same axis is decoration. If a reviewer disagrees, the arm is four lines.
- **The embedded two-struct class split** — see §1. Strongest form, larger blast radius, and BB-20
  covers the path that matters. Deferred with the reasoning recorded, not silently dropped.

## 9. Where I think the certification is wrong, or at least imprecise

Two small things, neither of which changes what I built.

1. **§5.4's "the type system is the close" is not sufficient on its own, and the certification says
   so only implicitly.** Unexporting the raw snapshot closes every OTHER package; it does nothing
   about a second exported reader inside `core/credit`, which is the very hole §5.4 raises two
   paragraphs earlier. The certification then recommends the type system without noting that it
   leaves that hole open. I built the package-scope gate to cover it and I am flagging the gap in the
   ruling rather than letting the phrase "enforced by the compiler, not by a lint" stand unqualified
   — because the same phrase, taken at face value, is how the last overstated gate got written.
2. **§5.3's table classes `suppressed` as "census (1 bit)" and then exempts it.** It is cleaner to
   say it is neither: its value is a function of the census only through the single predicate
   `Requesters < R_min`, which is the floor's own decision variable. The distinction matters for a
   future reader deciding whether some other one-bit census predicate qualifies for the same
   exemption — it does not, and calling `suppressed` "a census field we allow" invites exactly that
   reading. The code comment says it the narrow way.

Neither is a defect in the verdict. I record them because a certification is read later as law.
