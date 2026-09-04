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
the residual land on immutables, so **there is no safe side to round to**. The certified
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

That mixing of two tick sources is a real hazard, and it is what gives the G-BB-4 assertion
its teeth: an age can only exceed uptime if a **foreign** tick reached `firstSeenTick`.

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
- **The clock is a wall clock.** `adapters/walltime` returns `time.Now().UnixNano()`, which
  discards Go's monotonic reading. Steady-state slew (~500 ppm) is negligible against buckets
  measured in minutes-to-days; a **boot-time NTP step** is the real risk and lands at the
  start of the observation window. It is absorbed by clamping and **reported**, never
  silently reshaped.
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
