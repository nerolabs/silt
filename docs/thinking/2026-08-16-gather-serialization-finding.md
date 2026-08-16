# 2026-08-16 — run 1eded27-71360: the synchronizer WORKS; the stall moves one layer down — sequential gathers through dead peers

18 pass / 4 gap / 1 fail. Third consecutive full-cohort latch (h26); **handoff PASS
under the corrected bound** (h57 = target); 10c F-1 held. The FAIL is 10a again —
430s, no honest-coalition commit — but this run carries the `via=` telemetry, and it
exonerates the view synchronizer:

## What the telemetry shows

- `via=catch-up` fired 151 times run-wide; at the drill height h58 the members
  converged co-round across r1–r5 (catch-up + new-view jumps), new-views assembled
  at r3/r4. **View synchronization worked as certified.**
- The height still did not commit: every assembled proposal's GATHER came up short.
  Rounds burned 5 in 430s (vs 14 pre-synchronizer — the durations work too).

## The mechanism one layer down (code + arithmetic)

The two-phase gather asks attesters **sequentially** (`ask(i+1)` only after attester
i resolves), under the patient per-peer retry discipline (network-durability §2 —
correct for routing). Field daemons: `-request-timeout 8s -request-retries 3` +
backoff ⇒ **one dead peer costs ~34s of ask-chain time**. The 10a drill stops 4 of
12 members; #455's ID-sorted ask order makes their position deterministic. Four dead
members ≈ ~134s per phase, ~270s for prepare+precommit — a 430s window fits ~1.5
proposals. The same tax explains 10b's slow resume (restarted members mid-rejoin),
flow-6's variance (one down designee), and the publish-window misses ('accepted but
not committed within 3m0s': the designee's gather crawling).

The deterministic fixture could never see it: simnet's blocked peers cost ZERO (a
drop is instant). The model-check needs a per-ask COST dimension for unreachable
peers — the same method lesson as the skew dimension, one layer down.

## Why this is a consult, not a build

The standard BFT shape is a CONCURRENT gather (broadcast the proposal, collect
replies until quorum) — silt's sequential ask chain is the anomaly. But the gather
is the certified #432 machinery's execution engine; concurrency changes interaction
with early-stop counting, count-neutrality, the byte-identical #402 arithmetic, and
B2's single-loop discipline (the gather already runs on callbacks — bounded
concurrent asks are compatible in principle). Also in scope: skip-or-deprioritize
known-unreachable attesters (the reachability signal exists; must not conflate with
standing — #288/#3). Research consult filed:
/Users/andrewedmond/Claude/claude/silt-reviews/research/456-gather-serialization-CONSULT.md

PUBLISH_RETRY_S also still assumes the old commit-wait leg (missed in #454's
re-derivation) — folded into the same follow-up.
