---
name: orchestra-leverage-and-loop-health
description: Where the orchestra's leverage actually is on silt (tester-depth + research-parallelism, NOT builder throughput) + the three loop-health signals the Planner monitors. From the founding-PE feedback, 2026-08-27.
metadata:
  type: feedback
---

Feedback from the single-lane PE that helped build this orchestration model (2026-08-27), Andrew-endorsed.

## Aim the orchestra at silt's BOTTLENECK — not the builder
The builder is already fast (built the foundation in 5 weeks); orchestrating it faster buys ~nothing.
Silt's pacing constraint is **research-gate latency + the un-enumerable deep-run bug tail** — neither
speeds up with build throughput. **Rebalance the orchestra's weight there:**
- **Run the TESTER continuously against DEEP RUNS.** The #528/#549/#555/#558 "cost grows with depth"
  family surfaces one bug per cycle and only a hammered deep run flushes it — you can't find them by
  inspection. A tester always running deep runs and feeding the tail back is the single highest-leverage
  use of the orchestra on silt. (This is the evergreen Tester's PRIMARY job — see [[billable-run-orchestration-playbook]] — with billable-run standby as one mode of it.)
- **Parallelize the independent RESEARCH gates.** Fan concurrently, not serially: keystone-witness
  sharded-registry omission proof (§11.2), sharded-registry soundness, PoD relay-crypto, and the
  Phase-4 verifiable-escrow crypto DESK STUDY (pure literature, off critical path — the roadmap's only
  UNSCOPED risk; the orchestra scopes it without stalling the build).
- **Nuance (Planner):** builder orchestration is not zero — the fixes tester/research SURFACE still need
  a builder. This is a rebalance, not an abandonment.
- **Self-correction:** session-8 over-indexed on builder parallelism and ran ZERO deep runs. Don't repeat.

## Make the TESTER earn its keep — two silt-specific moves
- **Seed the scar ledger; don't start empty.** Load silt's paid-for scar history into the Tester's
  memory day one: #357/#397/#402/#432 ("one defect in four costumes" — a non-intersecting finality
  quorum), the OOM saga (#503 island, retain-from-checkpoint), the depth-war lineage
  (#528/#535/#549/#555/#558/#560/#561/#562/#563/#572), and the base rate "four field runs each found a
  new consensus bug." Sources: `docs/build-process.md`, `docs/decisions.md`, `docs/design/consensus-invariants.md`.
- **Convert the O(depth) tail into a CLASS-LEVEL GATE — instantiate NOW.** The third-time threshold is
  long crossed (the family is 10+). Standing check: **any hot-path function touching the chain gets an
  O(depth) review before merge** (does per-height cost grow with chain length?). Turns whack-a-mole into
  a defense. Owned by Tester (scar) + PE (review standing).

## The three LOOP-HEALTH signals the Planner monitors (and reports)
These erode quietly under a smooth loop; on silt they're load-bearing. Watch the cadence, report it.
1. **A GATED research verdict is HEALTH, not failure.** Reward the refusal. **Suspicion trigger:** a
   researcher that certifies easily / never GATES on a gated surface (M0/C1/C2, I1–I5, the economy
   firewall). Session-8 evidence it's working: C-5 and C-1 came back GATED.
2. **The PE must READ THE CODE, not re-attribute the builder.** Every ruling must carry an independent
   "I verified X myself" at a file:line the builder didn't hand it. **Suspicion trigger (rubber-stamp
   tell):** a PE ruling that echoes the builder's cited file:line with no independent verification. If
   the PE stops opening files, it's decoration. Session-8 evidence: the PE caught #607's dial-storm and
   the #604 ⅔-weight overclaim by opening files.
3. **Veto-gate CADENCE is a health metric.** Andrew stays the M0/immutable owner; the orchestra cannot
   make those calls. **Suspicion trigger:** the loop stops producing "your call" moments → either it's
   making immutable-trades silently (dangerous) or not hitting real decisions. A healthy silt loop keeps
   handing Andrew trades. Session-8: #600 scope, the C-1/C-5/C-7 ratifications, the A-vs-B merge call.

Related: [[validated-verify-before-merge-and-measure-first]], [[keystone-era3-freeze-sequencing]].
