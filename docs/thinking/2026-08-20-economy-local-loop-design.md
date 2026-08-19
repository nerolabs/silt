# 2026-08-20 — The local economy-loop e2e: design deliberation

**Goal (the one scoped goal, ROADMAP "Immediate next work" #1):** a LOCAL multi-daemon
e2e of the full S7 economy loop — publish erasure-coded → `swarm holders` → kill 3
columns → caretaker reconstructs from parity → bounty pays (`paid > 0`). Green locally
is the gate before the confirming `ECONOMY=1` cloud run (build-immutable #7;
[[silt-local-proof-before-billable]]).

## What the code-read established (evidence, before design)

Read before designing: `core/node/repair.go`, `core/node/repairclaim.go`,
`core/credit/escrow.go`, `cmd/silt/daemon.go`, `integration/cloudtest/scenarios.sh`
(`flow_economy_repair`), plus the three existing pieces (`e2e/economy_test.go`,
`e2e/holders_test.go`, `sim/repair_bounty_test.go`).

1. **The paramedic never judges its own claim.** `emitRepairClaim`
   (`core/node/repairclaim.go:291`) skips `t == n.id` and `t == holder` when it
   fans the claim out to the careKey quorum. The bounty is released by a
   **different** caretaker-judge, on **that judge's own ledger**
   (`settleRepairVerdict` → `n.ledger.PayBounty`). Credit is per-node-local.
2. **Therefore the loop needs ≥ 2 economy caretakers**, and `paid > 0` appears on
   the **judge's** `/api/status`, drawn from an escrow funded **on the judge's
   ledger**. Since which caretaker sweeps first (and so becomes the paramedic) is
   timing, both must be funded and both polled.
3. **The starter grant is 500,000 credits** (`cmd/silt/daemon.go:567`), and
   `FundEscrow` refuses an amount above the funder's balance
   (`core/credit/escrow.go:105`, `ErrInsufficientCredit`).
4. **`PayBounty` pays `min(amount, escrow balance)`** (`escrow.go:158`) — an
   underfunded reserve still yields `paid > 0`; that is the finite-but-renewable
   horizon by design. And `RepairQuorumTau = 1` (per-node-local ledgers), so a
   single judge's own retrievability vote releases.
5. **No node declares `-domain` in the cloud harness** → `domainID = 0` everywhere
   → `selfHoldEligible` is false → the payee is a **remote fresh holder**, not the
   self-holding paramedic. A local test with no `-domain` flags exercises the same
   payee path as the cloud.
6. **Repair fires when a stripe's missing shards exceed `RepairSlack` (2)** on a
   sweep every `RepairInterval` (60 s, previously not flag-tunable).
   `swarm add -replication 1` is the documented lever ("makes shard loss … 
   reproducible on a small swarm", `cmd/silt/swarm.go:206`).

## Three latent defects in the cloud scenario (found by this work, would GAP the re-run)

The 2323b09-20931 GAP was the setup publish (#489 fixed that). But even with the
publish landing, `flow_economy_repair` as written cannot pass:

- **It arms ONE caretaker** (`store-2`) and polls it for `paid>0`. With a single
  caretaker there is no judge (fact 1), so no `PayBounty` fires anywhere — and even
  with a second caretaker, the paid counter lands on the judge, which the scenario
  never polls.
- **It funds `amount=2000000`** against a 500,000 starter grant (fact 3) —
  `/api/fund` would refuse and the scenario would GAP at step 3.
- **Its relaunched caretaker had no `-registry`** — and `-care` with `reg == nil`
  silently skipped the entire care loop (`cmd/silt/daemon.go` care wiring), so the
  armed caretaker was a healthy-looking no-op. The #235 silent-skip shape, one
  layer up. Fixed at the product layer: `-care` without a registry now **refuses
  to start** (guard + failing-first e2e `TestCareWithoutRegistryRefusesToStart`).

This is exactly the class of integration gap the local-proof rule exists to catch:
every piece passes alone; the composition was never run. All three fixes ship in
this PR (two caretakers — the judge is the relay node, which is outside the
killable role set; fund 400k on both; poll both; `-registry` on the relaunch).

## Design decisions (options weighed)

**Consensus shape — lone trusted validator (`-quorum 0 -min-rep 0`), not the bonded
quorum.** Option (a): mirror the cloud's 4-anchor objective topology. Option (b): the
one-box trusted path `TestPublishCommitFetchOverTCP` and `TestEconomyEndToEndOnLiveDaemon`
already use. Chose (b): the integration under test is the economy loop
(repair → claim → judge → pay over real TCP); consensus-on-the-wire has its own e2e
tests, and the cloud run itself confirms the composed topology. (a) would add minutes
of standing-warmup and a second way to flake for no added coverage of the seam that
GAPed.

**Erasure geometry — one stripe at the cloud's chunk size.** File = k·chunkBytes =
10 × 256 KiB = 2.5 MiB → exactly one stripe, 16 columns (10 data + 6 parity).
256 KiB mirrors the §0.1-mandated economy-run geometry; one stripe keeps the
kill-selection reasoning exact. Repair RAM ≈ k·shard ≈ 2.6 MiB — trivial locally.

**Placement — `-replication 1`, 12 storage nodes.** Replication 1 makes each column's
holder set a single node, so killing 3 columns kills ~3 daemons and the collateral
bound (total columns lost ≤ n−k = 6) is easy to hold. The kill selector still
computes the union kill set and verifies 3 ≤ lost ≤ 6 before acting, exactly like
the cloud selector, so a placement surprise fails loudly instead of wedging the
stripe past recoverability.

**Caretakers — started AFTER the publish with `-registry <ref> -care <carelink>
-economy -ui`.** The care link only exists once `swarm add` prints it; starting the
two caretakers afterwards avoids the cloud's relaunch_with dance entirely and is the
natural operator shape ("I was handed a care link").

**Sweep cadence — new `-repair-interval` flag (default 60 s, unchanged).** Without
it the local proof waits multiple 60 s sweeps (minutes of dead time per run of a
test that gates every future economy change — V5 wants regressions caught in
seconds). Precedent: `-bond-audit` is the same knob for the audit loop and every
consensus e2e sets it to 1 s. Default stays 60 s, so no shipped behavior changes.
This is a liveness/test-tunable, not a security parameter (no proof depends on the
sweep period; the bounty legs are structural), so it is not research-gated under
build-immutable #3/#6.

**Assertions (outcome, not mechanism):**
1. `paid > 0` for the root on at least one caretaker's `/api/status` — the exit-gate
   signal, same as the cloud verdict.
2. Escrow arithmetic honest on the payer: `paid ≤ funded`, `repairs ≥ 1`.
3. **Fetch-back bit-perfect after the kill+repair** from a fresh client — the S2
   half: the bounty paid for a reconstruction that actually restored availability.
4. Standing neutrality (Invariant A) is NOT re-asserted here: it is pinned at the
   unit tier (`core/credit/invariant_a_test.go`) and the sim tier
   (`sim/repair_bounty_test.go`); the e2e adds no new ledger surface for it.

**Known non-goals:** domains/dispersion audit (domain 0 locally and in the cloud),
the serve auto-skim path (sim-covered), fork (c) split-pay (evidence-gated, owned in
the payee-fork ruling).

## Outcome (2026-08-20)

RED on the first run (no payout in 180 s; killed 3 holders, **6** columns lost —
exactly n−k, leaving exactly k survivors), GREEN after two changes: `-log info` on
the caretakers (observation only) and the kill selector preferring the **fewest**
lost columns ≥3 (3 in the green runs). 3/3 repeat runs green at ~55 s each.
**Attribution honesty (#7):** the RED run's mechanism was not captured (the info
narration wasn't wired up yet), so the margin story — at exactly-k survivors both
the paramedic's rebuild-gather and the judge's verify-gather need a perfect
k-for-k fetch round — is *plausible, not proven*. The selector change is justified
on its own terms (the test asserts the loop, not max-loss margin); the residual
below is the open question it points at.

**Product residual worth an issue — the repair claim is a one-shot volley.**
`emitRepairClaim` is fire-and-forget; a judge that denies *transiently* (its
survivor-gather raced dead provider records, the holder PoR timed out) never
re-hears the claim: next sweep the shards are all present, so no repair fires, no
claim re-emits, and the reconstruction stays permanently unpaid. Repair itself is
unaffected (S2 holds); the *payment* liveness is what can be missed — the exact
underfunded-reconstruction concern the fork (c) split-pay ruling parked. On a
240 s cloud window a one-shot volley is a real GAP risk. Evidence-gated next step,
not built now: if the ECONOMY=1 run (or a lost=n−k local variant) shows denied
volleys, a claim-retry (re-emit while `paid == 0` and the escrow is live) is the
minimal mechanism.

## Risks accepted

- **careKey quorum discovery**: both caretakers announce under `careKey(root)`; with
  a ~15-node network the announce target set (K closest) spans essentially every
  node, so the records survive the 3-node kill. If discovery still proves flaky the
  fix is instrumentation on `resolveProviders`, not a sleep (#7).
- **Both caretakers may both repair** (both sweeps fire): benign — each emits a
  claim the other judges; either `paid>0` satisfies the gate.
