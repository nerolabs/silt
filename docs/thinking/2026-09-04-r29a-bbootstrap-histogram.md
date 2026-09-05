# 2026-09-04 — R2.9a: replacing the `B_bootstrap` row export with a full-census histogram

**Seat:** Builder. **Steered:** the R2.9a rebuild on `builder/r2.9a-bbootstrap-histogram`.

**Input of record:**
`silt-reviews/research/research-outcome/R2.9a-Bbootstrap-instrument-sufficiency-RESEARCH-CERTIFICATION-2026-09-04.md`
(verdict GATED; eight gates G-BB-1…G-BB-8 in §4.2, fourteen Tester gates BB-1…BB-14 in §7),
itself routed on the blind PE ruling
`silt-reviews/principle-engineer/RULING-R2.9a-bbootstrap-export-b142a65-2026-09-04.md`.

This is a **full replacement of the export's shape and clock**, not a patch. The previous
build (`builder/r2.9a-bbootstrap-export`, PR #728) is superseded, not fixed.

## The mechanism, stated before the code

The failure is that the shipped export **cannot answer the question it exists to answer**,
for two independent reasons.

1. *The sampling rule selects on the response variable.* Retaining the top 4,096 rows **by
   bytes** removes, from every age cell, exactly the identities below an unpublished
   threshold `b*`. Since bytes grow with age, the **young** cells empty first and hardest —
   and the young cells are where the `grant/r` fit reads. The payload reported only a
   `truncated` bool, never `b*`, so the bias was not correctable after the fact. Past
   R = 4,096 the age-conditional quantile for any q < 1 is unrecoverable.
2. *The age axis was the consensus epoch, which is identically 0 on a non-validator.* The
   node with real users (the flixz content node) has no chain: `EnableChain` sits inside
   `if *validator {`, so `l.Epoch()` returns the watermark, which never leaves 0. The
   export would have published a constant on exactly the machine it was built for.

This change addresses both by **replacing the shape** (a full-census 2-D count histogram
over age × log-bytes: no cap, no sampling, no rows, no salt) and **replacing the clock**
(the boot-relative elapsed tick from the injected `ports.Clock`, stamped at first touch).
The PE's two blockers — `crypto/rand` inside `core/`, and the unsalted embedder publishing
`truncated: true` with an empty series — do not need fixing: they **disappear**, because
the new shape needs neither randomness nor a salt.

## Options considered

| # | Option | Cost | Benefit | Verdict |
|---|---|---|---|---|
| A | Patch the cap: publish `b*` alongside `truncated` | one field | makes the bias *correctable* in principle | **rejected.** The estimand is conditional on age; publishing the threshold does not restore the emptied young cells, and it publishes a per-row byte threshold — a sharper join key, not a duller one. |
| B | Reservoir-sample k rows uniformly | randomness inside `core/` (the exact `internal/depcheck` rule the first build tripped), per-row publication back on the wire, sampling error ~1/√k | unbiased at every conditional quantile | **rejected.** Correct but dominated: it re-introduces the join key AND the longitudinal row-matching the PE demonstrated by polling, and pays sampling error a census does not. |
| C | Keep rows, drop the cap | payload and cost unbounded in R (the PE measured 124 ms / 114 MiB at R = 500,000, on the event loop, per unauthenticated GET); accounts are never evicted, so R only grows | no selection bias | **rejected on build-immutable #8.** An unbounded cost on the floor box is unsafe, not merely slow. |
| **D** | **Full-census fixed 2-D count histogram** | gives up exact quantiles *inside* a cell and the exact mean — neither is the estimand | exact at bin resolution over the whole census; payload and work constant in R; no salt, no randomness, no sort, no per-row allocation; no per-identity datum exists to join on | **TAKEN.** It is strictly better on every axis this round raised. |

### The byte-axis resolution: log2 or quarter-log2?

The decision `grant/r` faces spans ~1,000× (480 KiB per grant under the R2.9 provisional
numbers, against the Economist advisory's *guessed* 512 MiB). Plain log2 resolves `B` to
2×, comfortably inside that span, and would cost 41 × 8 = 328 counters.

The reason to spend more is that **2× is not free at the margin**: too high a `grant/r`
cheapens Sybil bootstrap (M0); too low raises the floor of honest participation, which
build-immutable #4 calls a regression against silt's reason to exist. Both directions of
the residual land on immutables, so **there is no safe side to round to**. *(CORRECTED
2026-09-05, G-BB-22: the too-high direction does NOT land on M0's Sybil corner — the grant
mints balance, standing is bond-only under Invariant A. It lands on Don't #7 / T-AR /
build-immutable #8. The "no safe side" conclusion stands on the corrected landings.)* The certified
ruling is to *shrink* the trade rather than resolve it. Quarter-log2 cuts the residual to
19% for 1,312 counters — measured at **10,496 bytes** of fixed array and **one allocation
per snapshot** — against 114 MiB for the row export. Taken.

### The age axis: whose clock?

C1 (consensus epoch) is refuted above. C4 (registration ordinal) needs this node's own
arrival rate to convert to a duration, and that rate is least stable exactly in the bursty
regime the answer matters in. C3 — boot-relative elapsed from the injected `ports.Clock` —
is the only candidate that is *live where the users are* and *denominated in the unit W is
denominated in*.

**One thing the certification names as a pattern but that does not exist yet, and I had to
build.** §2.3 reason 4 cites `firstSeenTick` (`credit.go:52`, written at `:381-383` from
`bondaudit.go:157`) as the shipped first-touch stamp. It is not one for this population:
its only write is inside `RecordBondChallenge`, which fires for **validators whose bonds
this node challenges**. A pure fetcher never gets one, so on the flixz node every requester
would carry `firstSeenTick == 0` — the *same* defect class as the epoch, a constant on the
machine that will run this. The certification's own §1.2/§1.3 pin the window's open at
**first touch on this ledger**, and §1.2's table names `acct` → `Register` as that instant.
So the stamp is written in `Register` — the one place an account is constructed, which makes
"written once at first touch" structural rather than a guarded assignment at N call sites.
`RecordBondChallenge`'s existing `if firstSeenTick == 0` write is left exactly as it is, so
a ledger with no injected clock behaves byte-for-byte as it ships today.

That mixing of two tick sources is a real hazard, and it is one of the two ways the G-BB-4
assertion can fire. *(The other — a forward wall-clock step — is what round 2 below adds;
at the time this paragraph was written the assertion had only the foreign-tick arm, and
that arm alone cannot fire on the production path.)*

### Injection: gated on the publish flag, or unconditional?

Unconditional, in the daemon, right after `credit.New`. Gating injection on `-bbootstrap`
would make one flag control **two** things — stamping and publication — and would leave an
operator who flips the flag on at restart staring at a ledger full of unstamped accounts.
Unconditional injection means the flag controls exactly one thing: publication. The stamp
is inert without it (no standing calculation reads `firstSeenTick`, and that is still true).

### Default on or off?

**Off.** The certification leaves the call to the owner and recommends off; I ship off. The
reason that survives the shape change: `GET /api/status` needs no token (reads are exempt),
and the Host allow-list stops a rebinding browser, not `curl` — so anything published there
is world-readable wherever `-ui` is bound off loopback. Reversing a default is cheap now and
expensive after adoption, and the measurement needs exactly one deployment.

## What is deliberately NOT decided here

- **`W` and `q` are NOT pinned** (G-BB-1, the owner's call). Without W the series has no
  reading rule, and a pure fetcher has no income on the serving ledger, so "before it has
  income" does not define a window at all (R-FETCHER-INCOME). No reading rule is hard-coded
  anywhere in the code or on the wire. The age edges are a named table with a comment saying
  G-BB-1 may require adding one, and `BBootstrapRunPrecondition` takes W as a **required
  argument with no default**.
- **The RUN is not made.** G-BB-1 blocks the run, not the build. Nothing here has been
  pointed at real traffic.

## Honest costs, recorded rather than hidden

- **Right-censoring at uptime is permanent.** Accounts are in-memory and nothing evicts
  them, so a restart destroys the whole census. No W longer than the longest clean uptime is
  measurable, ever — not a gap awaiting FP-2. The payload carries the ledger's uptime so the
  bound is in the artifact.
- **The age clock is a wall clock, and a second source is injected to police it.**
  `adapters/walltime` returns `time.Now().UnixNano()`, which discards Go's monotonic
  reading, so an NTP step is a real risk. *(Corrected 2026-09-04, second round: the original
  text here read "It is absorbed by clamping and **reported**, never silently reshaped",
  and that was measured FALSE by the blind review — clamping only fires when a subtraction
  crosses zero. See "Round 2" at the end of this record for the defect, the mechanism and
  the fix.)* A step is now detected by comparing the wall clock against an injected
  `ports.MonotonicNanos`, and the divergence is published as `clockSkewNanos` with a
  `clockSuspect` flag.
- **A count-1 cell is not zero information** (R-BB-SINGLETON-CELL, open, bounded). No
  suppression: suppressing low-count cells would destroy exactly the tail the fit reads.
- **BB-5 is encoded as a property, not literally.** "Byte-identical in length at R = 10 and
  R = 500,000" cannot hold for a JSON count histogram — counts are decimal numbers. The test
  asserts the structure is identical and the length stays under a fixed ceiling. Measured:
  3,114 bytes at R = 10 and 3,183 bytes at R = 20,000.
- **BB-6 asserts allocation, not wall time.** A wall-clock ceiling calibrated on this dev box
  has already reddened this repo's CI once on slower hardware (the R0.4b C3 `ValidatePub`
  budget). The gate is "exactly one allocation, of a fixed size, whatever R is" — the property
  that made the row export unsafe — and the elapsed time is logged rather than asserted.

---

## Round 2 — the cross-check compared the wall clock against itself

Filed after the blind PRINCIPAL-ENGINEER review of `f0234be`
(`RULING-R2.9a-bbootstrap-histogram-f0234be-2026-09-04.md`), which returned MERGE WITH
CONDITIONS and one blocker, F-1: G-BB-4 / BB-13 was **claimed as met and was not**. The
shape, the arithmetic, the byte and age axes and the `Register` re-attribution all held.
This section records only what changed and why.

### The defect, and why the original check was vacuous

`UptimeNanos` is `obsClock.Now() − obsStartNanos`. Every age is `obsClock.Now() −
(firstSeenTick − 1)`. Both are differences taken from **one reading of one clock**, and
`adapters/walltime` returns `time.Now().UnixNano()`, which discards Go's monotonic reading.
So when that clock steps, the step lands in the minuend of the uptime *and* in the minuend
of every age, and **cancels out of any comparison between them**. The assertion "largest
occupied age edge ≤ uptime" was therefore invariant under exactly the event it existed to
detect, and the code comment saying it "cannot be violated by construction" was not the
proof — it was the defect, stated out loud.

The reviewer measured two failures in the build:

| Input | What the reviewed build reported |
|---|---|
| 30-second-old identity, 60-second-old process, then an **8-day forward step** | age bucket 7 (">7 days"), uptime 8 days, no flag, and `BBootstrapRunPrecondition` **accepted a 7-day window on a 60-second-old process** |
| 3-hour-old identity, then a **2 h 50 m backward step** | age bucket 4 → 3, uptime 3 h → 10 min, no flag |

The second one is the sharper of the two. `ClockStepBack` fires only when a subtraction
would go negative, so a backward step *smaller than the ages it moves* reshapes every
bucket and trips nothing. That is the "silent age reshaping" BB-13's certified wording
forbids, and it also voids G-BB-3: a censoring bound produced by a clock that stepped is
not a bound.

### Options

1. **Strike the claims.** Remove every G-BB-4 / BB-13 claim from the CHANGELOG, the ROADMAP
   and this record, and file `R-BB-CLOCK-STEP-UNDETECTED` as open and run-blocking. Cheapest,
   and honest. Rejected because the run happens on a VPS over days, which is precisely when
   an NTP step happens, and the age axis is the whole instrument.
2. **Detect the step in `cmd/silt` only**, comparing `h.UptimeNanos` against
   `time.Since(s.started)` and raising a flag on the wire (the reviewer's suggestion).
   Fifteen lines, no core change. Rejected: `BBootstrapRunPrecondition` is the executable
   handoff gate and it takes `credit.BBootstrapHistogram` values. A flag that exists only in
   the UI layer cannot make the precondition refuse, and the precondition accepting a
   60-second-old process for a 7-day window is the failure that actually costs the run.
3. **Inject a second, independent source into the ledger.** Taken.

### The decision

`SetObservabilityClock` now takes **two** sources and stamps both origins in one call:

- `c ports.Clock` — the age clock, unchanged. Wall time in production, steppable.
- `mono ports.MonotonicNanos` — a `func() int64` of elapsed nanoseconds on a source nothing
  can step. In `cmd/silt` it is a closure over `time.Since(obsStart)`, and `time.Time`
  carries Go's monotonic reading, so an NTP correction, an operator and a container clock
  all move `c` without moving it. `core` and `ports` may not import `time`
  (`internal/depcheck`), so the reading has to arrive injected — the same shape the ledger
  already uses for `SetEpochSource`.

Nothing is measured on the monotone source. Its only job is to make a step in the wall clock
**visible as a divergence** instead of letting it cancel. Three consequences:

- `MonotonicUptimeNanos` is the real **censoring bound**, and `CensoringBoundNanos()` is what
  the G-BB-4 assertion and the precondition's W arm read. `AgeExceedsUptime` now fires on the
  **production path**: a forward step ages identities past a bound that did not move. That is
  what turns BB-9 from decorative into a live gate.
- `ClockSkewNanos` = wall elapsed − monotone elapsed, **signed** and taken from the raw wall
  delta before the clamp, so the two directions stay distinguishable and a step past the
  ledger start still reports its full size instead of reporting zero.
- `ClockSuspect` = `|skew| ≥ 60 s`. That threshold is **derived, not picked**: one minute is
  the width of the narrowest positive-width age bucket, so below it a divergence cannot
  displace an identity by a whole bucket and at or above it, it can. It is deliberately
  strict — a long run that accumulates a minute of ordinary slew is flagged, which costs a
  re-run, while the other error costs a wrong `grant/r`, and `grant/r` lands on M0 *(CORRECTED
  2026-09-05, G-BB-22: on build-immutable #4 from below and Don't #7 / T-AR / #8 from above,
  never M0)*. The raw number is published either way, so an operator can read past the threshold.

**One call, not two setters.** Both origins are stamped inside `SetObservabilityClock`, so
"the two sources start at the same instant" is structural. Two setters would make it a
call-ordering hope, and any gap between them would read forever after as skew — the same
class of defect (a check resting on an unenforced ordering) the change exists to close.

**Nil is refusal, not a silent downgrade.** With no monotone source the snapshot reports
`monotonicSource: "none"`, `CensoringBoundNanos()` degenerates back into the self-comparison,
and `BBootstrapRunPrecondition` refuses the whole configuration on that ground. The
degeneracy is stated in the code rather than hidden behind a fallback.

### The other gates the ruling named

- **BB-14's degeneracy arm was dead** (F-5): it declared the axis degenerate only when every
  identity sat in age bucket 0, which is an age of *exactly* 0 ns — a sim-clock artifact the
  wall clock essentially never produces, and the one case it would catch (a frozen clock) is
  already caught by the "clock not advancing" arm. The live degeneracy under an injected wall
  clock is **every counted identity in ONE bucket**, whichever bucket that is, because the
  estimand is a quantile *conditioned on age* and one occupied bucket conditions on nothing.
  That is what the arm now checks. The precondition also refuses when the **monotone** uptime
  is below W, which is the arm that the 8-day forward step defeated.
- **BB-9 has teeth now**, and they are production-path teeth: arm 2 of
  `TestR29aOccupiedAgeBucketsNeverExceedUptime` steps nothing but the wall clock, through the
  public API, and the assertion goes true. The foreign-tick arm is kept because it is a
  *different* hazard — a stamp in the wrong unit, not a clock that moved — and the test now
  also asserts that a foreign stamp does **not** raise `clockSuspect`, so the two stay
  distinguishable.
- **BB-8's source gate was split** (F-4 / C-5). One test read two literals under a doc comment
  naming a single runtime cover, and that cover observes the **flag** and nothing about the
  clock; `scripts/check_source_gates.py` passed because it requires a *named* cover, not a
  *matching* one. The flag assertion keeps its real cover. The clock assertion moved to its own
  gate, annotated `UNGATED:` with the honest residual — no test in this repo boots the real
  daemon, so nothing observes at runtime that a live daemon reports `clockSource: "injected"`
  and `unstamped == 0`. That gate gained the thing a source gate *can* honestly check: the
  **order**. It asserts the injection appears before `nd.SetLedger`, because the ledger cannot
  receive a `RecordServe` — the only thing that creates a requester account — until it is
  attached. (`node.New` is the wrong anchor: the daemon constructs the node at `daemon.go:416`,
  long before it constructs the ledger.)
- **F-6, the false payload-contract comment**, is corrected: `Aged + Unstamped == Requesters`
  holds *whenever the age axis is live*, and only then.
- **F-7 / C-6, the missing BB-6 points.** Recorded as the reviewer's own measurement rather
  than restated as ours: R = 5 × 10⁵ costs **32.3 ms and 1 allocation**, against 124 ms and
  114 MiB for the refuted row export. The gate still asserts allocation rather than wall time,
  for the reason already recorded above.

### Ablations run, each reverted

| # | Reverted | Went RED |
|---|---|---|
| A1 | `CensoringBoundNanos()` → always the wall uptime | BB-9's forward arm, BB-13's forward arm, the wire arm |
| A2 | `ClockSuspect` never set | BB-13's forward **and** backward arms, the wire arm |
| A3 | precondition's W arm → wall uptime | BB-13's "uptime below W" assertion |
| A4 | degeneracy → bucket-0-only | BB-14's one-bucket arm |
| A5 | the two-source injection literal removed from `daemon.go` | the BB-8 clock source gate |
| A6 | injection moved *after* `nd.SetLedger` | the BB-8 clock source gate's order assertion |

### Residual, stated rather than hidden

**The backward step rests on one detector.** A forward step trips three independent arms
(the censoring assertion, the skew flag, and the precondition's W arm). A backward step
trips only the skew flag, because deflated ages violate no bound — they are consistent with
a younger population. So the 60-second tolerance is load-bearing for that direction alone. A
backward step *smaller* than 60 seconds reshapes the young cells and is not flagged; it is
bounded by the same 60 seconds, which is under one bucket width, and `clockSkewNanos` carries
the number for an analyst who wants to check.
