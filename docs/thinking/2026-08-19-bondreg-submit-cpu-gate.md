# Phase 1.2 — the `MsgSubmitBondReg` CPU gate (deliberation before code)

**Date:** 2026-08-19 · **Roadmap:** the ordered path Phase 1.2 (pre-#183 DoS floor),
now also carrying the E5 measurement rider (PE drain ruling).

## The evidence (gathered first, #7)

1. **The submit path has NO rate limit.** `chainrole.go` `MsgSubmitBondReg`:
   decode → `ValidateBondRegErr` (per-window: up to K× ed25519 at ~50 µs, then at most
   one `VerifySpaceTime`) → queue. Nothing bounds submits per sender; the #424 gate
   covers bond *challenges* only.
2. **Measured verify cost** (`core/bond/verifycost_bench_test.go`, Apple M4 single
   core, field config delay=1000/k=64): **valid ~2.0–2.8 ms; garbage-until-reject
   ~0.44–0.59 ms** (dies at `vdf.Verify`). Structurally-hollow garbage (empty answer)
   dies in ns at the shape checks. The hobbyist floor box (~1 vCPU, build-immutable
   #8) is several × slower.
3. **Correction to E5's drain folklore:** the "VDF at 100+ ms" figure was the
   *prover* (sequential squarings); the *verify* side is ms-scale by design
   (Wesolowski). The E5 drain-rate measurement is therefore MORE necessary, and this
   benchmark is its first datapoint.
4. **The replay hole:** queue dedup (`queuePendingBondReg`, one slot per validator,
   replace-in-place) sits AFTER the verify — a third party replaying a captured
   *valid* ~1.5 MB reg re-pays full verify per message. And the honest path always
   self-submits (`SubmitBondRenewal` sends the node's OWN reg directly), so
   `from == reg.ValidatorID()` is an invariant of legitimate traffic.
5. **Honest cadence:** one submit per sender per `ChainSyncInterval` (30 s) sweep,
   only while `BondRenewalDue`, re-sent until committed.

## The threat, quantified

Per malicious submit the loop pays: frame decode (size-dependent; a full answer is
~1.5 MB) + up to K cheap sig checks + ≤1 expensive verify (~0.5–3 ms M4; more on the
floor box). No single message is catastrophic — the amplification is **unbounded
rate**: one authenticated identity (or a minted set) holding a pipe keeps the single
loop at a permanent duty cycle for free. This is the #424 shape exactly, one message
kind over.

## Options (PACE)

- **A — per-sender window budget, #424 idiom (CHOSEN).** `allowBondSubmit(from)`
  charged BEFORE decode (the sender is known pre-decode; a refusal costs a map
  lookup — zero amplification), window = `ChainSyncInterval` (the honest cadence
  clock), burst = 8 (honest 1/window + retries, wide headroom — mirrors
  `bondChallengeBurst`), table bounded at 4096 with stale-window sweep (the
  `maxBondChallengers` idiom). Precedented, ~30 lines, no new concepts.
- **B — sender-binding check** (`from == reg.ValidatorID()`, post-decode, pre-verify):
  closes the third-party-replay hole for µs. Composes with A; both ship.
- **C — negative-cache of refused payload hashes.** Rejected for now (KISS): A already
  bounds per-sender work; a cache adds state and eviction policy for a marginal win.
  Revisit only if the E5 drain measurement shows refused-replay dominating.
- **D — global concurrent-verify budget.** Rejected: a global budget lets a flooder
  starve honest submitters (the exact failure #424's per-challenger choice avoided).

## Consensus-invariants statement (I1–I5)

Untouched. The gate changes when a submit is *examined*, never what is *valid*: a
refused honest submit is indistinguishable from a dropped frame and heals by the
existing next-sweep resubmit (the same recovery path as WAN skew refusals, run
09fbe60-84613). Quorum math, finality, signing, and set membership are unchanged.

## Tests (V5, failing-first)

1. **Unit (the flood):** N submits from one sender in one window → at most burst
   reach validation (hook: count `ValidateBondRegErr` calls via a test seam or
   observe queue/log); budget resets across windows; a second sender proceeds while
   the flooder is capped.
2. **Unit (sender binding):** a valid reg submitted by a DIFFERENT transport identity
   is refused before verify.
3. **Unit (honest cadence unharmed):** the due→submit→commit renewal flow at sweep
   cadence never trips the gate (regression for the existing H2 renewal e2e/sim).
4. **Bench:** `verifycost_bench_test.go` (committed) — the sizing evidence.

## The E5 rider (after the gate lands)

Measure the real saturation drain rate at the shipped 256M cap under a bond-reg
submit flood (post-gate) on a laptop: a harness that floods submits at the gate's
refusal rate + admitted-budget verifies, and measures loop drain MB/s. Re-run the
parked v2b drill (`drill/v2b-gate-starvation`) re-parameterized to that number —
the go/no-go the drain ruling requires.

## The E5 rider — DONE (2026-08-19)

Measured (`core/node/draindrate_measure_test.go`): the real single-loop drain for
the cheapest bulk a flood rides — MsgStoreChunk, real hash-verify + store handler
over a real TLS transport — is **~1227 MB/s on an M4 core** (24.5k × 256 KiB
chunks acked in 5 s). At the shipped 256M cap that is `cap/drain ≈ 0.21 s`, well
under the 2 s saturation bound: the parked v2b drill re-parameterized to this
drain goes GREEN (the latency ≈ cap/drain relation held to 1% in the original
run, so the analytic re-parameterization is decisive). **Verdict: SHELVE v2b**
(owned-residual E5), with a floor-box drain measurement as the one owed caveat —
SHA-256 is hardware-accelerated on cheap ARM too, so the floor box is *expected*
above the ~128 MB/s go-line, but that is expectation, not measurement. Verdict
reported to the PE: `silt-reviews/principle-engineer/`
`v2b-drain-measurement-VERDICT-shelve-2026-08-19.md`.
