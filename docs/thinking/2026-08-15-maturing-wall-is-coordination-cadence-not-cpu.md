# The MATURING "wall" is coordination cadence, not CPU — measured, and it reframes #382

**Date:** 2026-08-15
**Run:** `7134711-18163` (instrumented MATURING re-run, LOG_LEVEL=info LOOP_BUDGET=1). Torn down clean (33 destroyed).
**Instrument:** the event-loop latency instrumentation (Andrew's timing-for-evidence idea, PR #423).

## The question this run answered

Two prior hypotheses for the MATURING throughput "wall" (handoff never matures, chain crawls,
publishes starve): the PE's lead was VDF-eval saturation of the single goroutine; my refined H1
was single-goroutine saturation more broadly. Both are CPU/throughput models. The instrument
lets us *measure* the goroutine instead of arguing about it.

## What the instrument measured (live, during the starvation)

Pulled from an anchor's `debug.log` while the net was under the 12-participating-validator load:

- **The event-loop goroutine is ≤7% busy** — every 30s window summed to 0.8% / 3.9% / 7.1% / 2.6%
  of the goroutine's time across handlers, even at peak (chain advancing, publishes landing,
  fault/restart flows running). It is **~95% idle.**
- **No task starves** — `wait_max_ms` is tens of ms; the always-on queue-wait/slow/hang thresholds
  (2s/250ms/15s) never fired.
- The busiest handler was ChainReply (chain-sync serving) at ~3% of a window. BondReply/
  BondChallenge/SubmitBondReg were ~200–380ms/window — trivial. **The VDF is a non-event, exactly
  as the microbench predicted** (`BenchmarkAnswerSpaceTimeFieldConfig`: 3.4ms/eval × ~11 honest
  evals/window = 0.1–0.6% of a core).

**Conclusion: not CPU-bound. The single-goroutine-saturation model — the PE's and mine — is
refuted by direct measurement.**

## What the wall actually is

Three strands, none of them throughput, all consistent with Andrew's tenet (*we are durable to
network lossiness; the latency we care about is our own compute*):

1. **Publishes: marginal WAN-coordination timing vs test wall-clock.** The `-request-timeout`
   (8s here, 5s default) is an across-network per-attempt deadline, but exceeding it **retries**
   (`-request-retries`=3, backoff) — durable by design, *not* a gate. The real failing deadline is
   the **harness's** 120s (ft_publish) / 180s (fresh-publisher warm-up) wall-clock windows. Over a
   lossy/high-latency WAN the fresh-publisher issuer-discovery+gather sometimes exceeds them → the
   harness grades GAP/FAIL, but the product would keep retrying. Run-to-run variance (this run's
   val-a publishes landed; fetch-1 warm-up still missed 180s) confirms marginal timing, not a break.
2. **Maturity: the bond-reg drain cadence.** The handoff GAPped even on this healthy run because
   the **bar-2 maturity metric never accumulated enough committed bonded weight in 420s.** The chain
   committed 13 blocks but only **4 with entries=1, 9 empty** — the bond-reg drain is serialized
   (~1 reg per block, most blocks empty) while `SubmitBondReg` traffic keeps re-submitting
   uncommitted regs. With 12 validators needing their bonds on-chain and the drain committing ~1 per
   block, maturity can't reach bar-2 inside the window. **Idle goroutine → not CPU.** It's how fast
   our protocol drives bond regs to *commitment* over WAN consensus — coordination cadence, in our
   control, an M1 efficiency question (batch more regs per block / accelerate the drain), not a
   correctness defect (bonds *do* commit).
3. **#382 was mis-framed.** Its "throughput / SyncChain / drain serialization" framing assumed CPU.
   The instrument shows idle CPU. The residual is coordination cadence + test-deadline design, not
   throughput. #382's own dominant cause (SyncChain refetch) already shipped fixed (#384); what
   remains is not a faster-goroutine problem.

## The discipline that got here

Three hypotheses, each killed by a cheap measurement instead of a fix built on a guess:
bond-audit VDF (microbench) → single-goroutine saturation (instrument) → network-latency
fragility (the product retries; it's durable). Each time, measure-first paid off. The PE and I had
converged on a CPU model; Andrew's tenet-based question (*why are we gating on across-network time
we're durable to?*) plus the instrument reading ≤7% together broke it. Owning that plainly: we
were optimizing the wrong dimension.

## What changes

- **#424 (bond-challenge rate-limit) still stands** — a *flood* (~8800 evals/window to saturate)
  is a real remote CPU-DoS; the rate-limit caps it. It just wasn't the MATURING cause.
- **The fix scope moves from "throughput" to two M1/coordination items, both in our control:**
  (a) the bond-reg drain cadence (batch/accelerate commitment so maturity reaches bar-2 in a
  reasonable wall-clock); (b) the fresh-publisher gather/issuer-discovery wall-clock (the R-3
  "wait out slow issuers, never race past" privacy choice × per-attempt-8s×3 compounds on a bad
  path — an efficiency-vs-privacy M1 tension, not a bug).
- **Grade the field test on what we control** — does our compute stay bounded (yes, ≤7%) and does
  it *eventually* complete — rather than a tight wall-clock window WAN latency legitimately blows.
- **The instrument is permanent infrastructure** now (on main), and `-loop-budget` makes the
  decomposition capturable at info without the debug-firehose confound. It will keep turning
  "which function is slow / hung" from a guess into a measurement.

Route to the PE: this reframes the H1/throughput scope entirely, and the red team disposition
(the MATURING regime's field-confirm is blocked by drain cadence + test-deadline design, not a
safety property — safety held on every drivable flow this run).
