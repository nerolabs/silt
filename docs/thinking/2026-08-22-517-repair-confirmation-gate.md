# 2026-08-22 — #517: the repair confirmation gate (and #514's full attribution)

**Status: attribution complete (captured journals), fix decided and shipped in
the same PR. This entry is the capture story + the design call.**

## How #514 was captured

The flake (~2/10 under load, window-independent): `TestRepairBountyPaysOnTheWire`
kills the sole holders of 3 columns, yet the caretakers report `missing ≤
slack` forever — no repair, no payout. Capture kit (local, reverted after): all
16 daemons at `-log debug`, per-daemon `debug.log` copied out on failure
(t.Cleanup registered AFTER every t.TempDir so LIFO runs the copy first — the
first attempt got this backwards), the kill decision (killSet + `swarm holders`
snapshot) and the publish client's placement narration saved, plus a new
permanent `shard confirmed by=<node>` debug line in `probeShard` so a
"reachable" verdict names its confirmer. Two lessons paid for: Go's test cache
silently replayed identical-env runs (six byte-identical "passes" — add
`-count=1` or vary the env), and the failure reproduced fastest under
concurrent load (two parallel loops; first attempt under mutual load hit it).

## The attributed chain (capture attempt-33, all from journals)

1. Publish at 09:33:01, replication 1: S2 receives 4 shards spanning columns
   4/12/14.
2. 09:33:25 — caretaker C1's FIRST sweep after arming (its warm start's
   manifest-heal took 21.8s; its record vantage was fresh): `reachable=18`,
   `stripe repaired missing=3` — it read S2's shards as missing because their
   records had not converged to its walk, "rebuilt" them from parity, and
   placed the rebuilds at the daemon's replication 3 onto six other nodes,
   correctly keyed and recorded. The bytes were never lost.
3. The kill-selector's later `swarm holders` snapshot still listed only S2 for
   those columns (its own propagation raggedness) → the test killed S2
   believing 3 columns die — while every "doomed" shard had three live,
   record-backed copies.
4. Both caretakers then CORRECTLY watched `missing=2 ≤ slack` forever. No
   repair claim was emitted in this run (escrow unfunded at the false-repair
   moment); whether a funded escrow would pay for a false repair is noted on
   #517 as an open question (the judge's retrievability-shortfall deny is the
   expected defense).

So the flake's root is the PRODUCT defect filed as #517: **the repair trigger
fires on a single probe sample**, and a just-armed caretaker's sample is
maximally noisy. Every caretaker arming on an object silently mints
replication-N duplicates of whichever columns' records converge last — a third
source of the #497 extra-copies census (persistent AND record-backed), after
the two closed by #500/#502.

## The fix (network-durability §3, applied to the repair trigger)

§3's rule is verbatim the discipline: *minimum-filter a noisy signal — never
trust one sample.* The repair (and dispersion re-spread) trigger now requires
the over-slack observation to persist across **two consecutive sweeps**; a
clean sweep resets the counter; once fired, the counter stays satisfied while
the condition persists, so a failed repair (below k) retries every sweep
without re-confirming what the failed attempt just verified. Cost: one repair
interval of added latency on a true loss (2s in the harness, 60s at the
default) — cheap against false rebuilds amplified by replication N. State is
per (root, stripe), bounded by cared roots, deleted on clean observations.

Alternatives considered: an arm-up grace period (suppress repair for N sweeps
after Care) — rejected as a special case that misses mid-life propagation
glitches (reprovide races, churn flaps) which the consecutive-observation rule
covers uniformly; and probe-side record-freshness heuristics — rejected as a
new signal with its own noise, where §3 already names the sound shape.

## Downstream effects owned

- The #501 measurement re-baselines: first post-kill sweep 45.3s (walls only,
  repair pending), second sweep 72.8s (repair fires, caches warm). The e2e
  bounty cycle grows by one 2s sweep (~64s package, well inside 180s).
- The #502 crash rig drives an observation sweep before its crash-window
  sweep.
- The e2e gains a premise fast-fail: if no caretaker observes an over-slack
  loss within 60s of the kill, the test fails LOUD naming the premise defeat
  (holders-view vs bytes divergence) instead of burning a 180s window — the
  #514 issue's ask.
- Rig reality noted for future sim tests: a sole holder's column records live
  only on itself (receivers self-register; publish plants nothing on near
  nodes), so a walk-gated sole holder is undiscoverable until cooldown lapse
  or its own re-announce — real daemons re-announce at boot (#69), sim nodes
  must advance past the cooldown.
