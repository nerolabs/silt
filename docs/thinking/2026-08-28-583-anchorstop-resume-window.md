# 2026-08-28 — #583 third occurrence: derive the anchor-stop resume observation window

**Issue:** #583 (`TestAnchorStopHaltsBondedNonAnchors`, `e2e/anchorstop_test.go`), THIRD
documented occurrence. **Rule invoked (verbatim from #583):** "Do not silently reroll a
third time … the bound derivation (resume window vs computed commit cadence) gets the
#549-Q3 treatment (derive, don't guess)." **Scope:** test-harness timing only. The product
daemon and every consensus code path are untouched (`git diff --stat` confirms one file).

## The failure (X), the mechanism (Y), the fix (Z)

- **X.** `anchorstop_test.go:172 val3 never observed a committed block after the anchors
  resumed`. Green locally (~46 s total); RED on CI (run 32962125680, 76.12 s total, every
  other e2e test in the job green). Load-sensitive, not a determinism break.
- **Y.** After the driven publish commits on the resumed anchor set, a bonded NON-anchor
  (val3) observes that block by one of two paths:
  1. **Live participation** — val3 is in the commit round with the resumed anchors. In this
     path `committedCount(val3)` bumps essentially simultaneously with the publish returning
     (measured 0.0 s delta, below). This is the LOCAL common case.
  2. **Catch-up** — val3 missed the live round (its peer standing with the freshly-restarted
     anchor PROCESSES is re-earned live by bond audits, so it is not always a counting
     participant the instant the anchors return). It then heals on its next
     `chainSyncTick`, which reschedules itself every `ChainSyncInterval` = 30 s
     (`core/node/chainrole.go:1527`, `core/node/node.go:288`) at an ARBITRARY phase, then
     runs one `SyncChain` gather + `Reconcile` leg.

  The test's observation window was **30 s** (`anchorstop_test.go:165`), i.e. EXACTLY one
  `ChainSyncInterval`. A window equal to one sweep interval catches ZERO sweeps in the worst
  phase: if val3's tick fired just before the resume commit landed, its next attempt is a
  full interval later and the window is already spent. This is the identical zero-overlap-
  margin defect #549-Q3 fixed for the round-duration base (a window = one interval has no
  phase margin). Under CI load the gather leg stretches on top, and the #583 CI/local delta
  (76 − 46 ≈ **30 s ≈ one `ChainSyncInterval`**) is exactly the extra sweep's worth of
  latency a loaded runner adds.

- **Z.** Derive the window from the catch-up cadence — `resumeObserveSweeps ×
  ChainSyncInterval` — sized to span at least TWO catch-up sweeps (so one fires regardless
  of tick phase), plus a load margin for the gather-under-load stretch. Keep the POLL
  structure (fail fast if val3 is genuinely halted). Add a guard test so the window can
  never silently regress below the derived multiple of the interval.

## The measurement (artifacts)

Instrumented measurement harness (`e2e/anchorstop_measure_test.go`, NOT shipped — deleted
before commit). Widened the observation window to 240 s so it records the true delta instead
of truncating it. Logs at `/tmp/583/`.

**Live-participation path (val3 running normally), 6 serial + 3 under full 10-core CPU
saturation:**

```
MEASURE-583 publishDuration=0.2s observeAfterPublishOK=0.0s observeAfterPublishStart=0.2s
  (all 9 iterations identical; total test 45.7–46.3 s)
```

val3 observes the resumed commit within one 500 ms poll of the publish returning — it is a
live participant. This is why the test is green locally: the catch-up path (the one the
30 s window must actually cover) is not exercised on an unloaded laptop.

**Forced catch-up path (`MEASURE_FORCE_CATCHUP=1`: SIGSTOP val3 across the resume+publish,
SIGCONT after the publish commits, so val3 STRICTLY misses the live round), 5 iterations:**

```
MEASURE-583-CATCHUP observeAfterSIGCONT=0.5–1.5s (pure ChainSyncInterval catch-up path)
```

The `SyncChain` gather + `Reconcile` LEG itself is cheap (0.5–1.5 s local). SIGCONT fires
val3's overdue suspended tick almost immediately, so this isolates the gather leg but NOT
the phase wait. The phase wait is the STRUCTURAL term, derived below (it cannot be smaller
than 0 or larger than one full interval, by construction of a self-rescheduling ticker —
the same structural bound #549-Q3 used, `< ChainSyncInterval`, not an empirical fit).

## The derivation

Worst-case wall-clock for a non-anchor to observe the resumed commit via the catch-up path:

| Term | Value | Source |
|---|---|---|
| Tick phase wait | up to `ChainSyncInterval` = 30 s | self-rescheduling ticker, arbitrary phase (`chainrole.go:1527`); same `< ChainSyncInterval` skew bound as #549-Q3 |
| One `SyncChain` gather + `Reconcile` leg | 0.5–1.5 s local; stretches under CI load | measured (MEASURE-583-CATCHUP) |
| CI load stretch | ~30 s observed | #583 CI/local delta (76 − 46) ≈ one extra interval |

`resumeObserveSweeps × ChainSyncInterval`:

- **`resumeObserveSweeps = 1` → 30 s.** Equals the phase wait exactly. ZERO margin: a tick
  that fired just before the commit lands is a full interval away, and the gather leg + any
  CI stretch pushes past 30 s. **This is the shipped defect — REJECTED.**
- **`resumeObserveSweeps = 2` → 60 s.** Spans TWO catch-up sweeps, so one fires regardless
  of phase (even a worst-case-30 s-late tick has a full second sweep inside the window). This
  is the smallest window that reliably covers the catch-up path — the direct analogue of
  #549-Q3's "smallest base that reliably outruns the skew" (base = 2).
- **`resumeObserveSweeps = 3` → 90 s.** 60 s (two guaranteed sweeps) + a full third
  interval of margin for the gather-leg-under-load stretch the #583 CI/local delta measured
  (~30 s). **CHOSEN.**

**Chosen: `resumeObserveSweeps = 3` → 90 s.** The margin (the third interval over the
minimal 60 s) is justified directly by the measured ~30 s CI/local delta: the loaded runner
adds roughly one interval of scheduling latency on top of the two-sweep phase guarantee. This
is not padding-for-comfort; it is the measured load term. 90 s also stays well below the test
body's total budget and does not weaken the test's teeth (see below).

## The teeth are preserved — proven

The window is a POLL with a fatal on expiry, not a fixed sleep. It goes RED iff val3
observes NO fresh commit within 90 s. A genuinely halted non-anchor (the liveness regression
this test exists to catch) never bumps `committedCount`, so the poll exhausts and
`t.Fatalf` fires — same failure mode as before, just with a phase-correct bound. Widening
does not blunt this: 90 s is a SPAN over the catch-up cadence, not an unconditional wait; a
real halt still fails, only ~44 s later than the old flake-prone bound. Proof by injected
regression is in the evidence section of the handoff (the daemon is patched to drop val3's
sync adopt, the test goes RED at 90 s).

## The gate against a fourth occurrence

Encode the derivation as a compile-time guard in the test file (the #549-Q3 pattern — a
derivation guard, not more prose):

```go
// resumeObserveSweeps: the resume-observation window in ChainSyncInterval units.
// Derived (#583): a non-anchor that missed the live resume round heals on its next
// chainSyncTick (every ChainSyncInterval, arbitrary phase). A window = 1 interval has
// ZERO phase margin (the #549-Q3 defect). 2 sweeps guarantees one fires regardless of
// phase; the 3rd is the measured ~30 s CI/local load stretch (#583 76 s − 46 s).
const resumeObserveSweeps = 3
const chainSyncInterval = 30 * time.Second // mirror of core/node/node.go ChainSyncInterval
const resumeObserveWindow = resumeObserveSweeps * chainSyncInterval

// Guard: a future tightening below 2 sweeps re-introduces the zero-margin flake.
func init() {
    if resumeObserveSweeps < 2 {
        panic("resumeObserveSweeps < 2 re-opens #583: a window ≤ 1 ChainSyncInterval " +
            "has zero phase margin for the non-anchor catch-up sweep")
    }
}
```

The guard fails the build (panics on package init, so the whole `e2e` package refuses to run)
if anyone lowers the window below the two-sweep phase floor. A fourth silent reroll is
impossible: the only way to shrink the window is to delete the guard, which is a visible,
reviewable act with the rationale in front of the author.

## Residual (owned)

The bound rests on `ChainSyncInterval` = 30 s being the catch-up cadence on every seat, and
on the round-change/catch-up timer staying coupled to that tick. This is the same residual
#549-Q3 owns. `chainSyncInterval` in the test is a MIRROR of the daemon constant, not a
read of it (the e2e binary is a separate process); if the daemon's `ChainSyncInterval`
changes, the mirror must be re-synced. That coupling is called out in the constant's comment.

## Decision

1. Replace the fixed `30 * time.Second` resume-observation window with the derived
   `resumeObserveWindow = resumeObserveSweeps × chainSyncInterval` = 90 s, keeping the poll.
2. Add the `init()` derivation guard so the window can never silently regress below the
   two-sweep phase floor.
3. Delete the measurement harness before commit; it is not shipped.
