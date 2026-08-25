# #555 deep-drive crawl: the measured attribution (2026-08-25)

**Verdict: the crawl was not gather latency. It was event-loop saturation from
redundant `Block.Hash` work on the chain-sync path, and it grows with depth.
The round base stays 2. The fix is a hash memo, not a timing constant.**

## What the certification held open

The #555 research certification (2026-08-25) certified direction (a) — size the
round base to the measured happy-path gather latency `G` — but HELD the constant
on its own condition: *"no timing constant without the measurement; the sim
hides WAN cost."* It inferred `G ≈ 90–150 s` from the commit-round distribution
(252× r1, 118× r2). The open question flagged at consult time: per-reg verify is
~ms and WAN RTT ~160 ms — neither explains 82 s (maturing) → 390 s (deep) per
height, so the measurement had to pin the driver before anything moved.

## The measurement (flow-evidence-95d39e8-deep.log, 12-deep-heights, 22:00–22:30Z)

Three independent readings from the same window agree:

1. **Intrinsic G ≈ 10 s, not 90–150 s.** h74: `new-view proposal round=1` at
   22:16:50.876 → `block committed height=74 round=1 via=proposal` at
   22:17:00.514 — **9.6 s** proposal-to-commit on the real 3-region WAN with a
   reg-carrying block. The 60 s r0 fits it six times over. The h73 "158 s
   gather" at r2 (22:12:52.9 → 22:15:30.5) ran while the fleet was saturated
   (next point) — it measures queueing, not gathering.

2. **The event loop was the bottleneck, and its cost grows with depth.** The
   watchdog counted 324 `eventloop task slow kind=ChainReply` (total 5,211 s,
   max **86 s**) plus 131 `eventloop task HANG kind=ChainReply` (total 2,129 s,
   p50 16 s). Average blocked-time per ChainReply grew 2.4 s → 10.8 s → 35.8 s
   → 41.9 s across the window — the same growth curve as the per-height time
   (82 s maturing → 390 s deep). While blocked, everything starved: the sweep
   **timer itself** fired late (waited p50 18 s, p90 **146 s**), RoundChange
   messages waited p50 32 s, GetChainHead p50 20 s. The round ladder wasn't
   escaping because gathering was slow; it was escaping because every timer and
   every reply sat behind a wedged thread.

3. **The HANG stacks name the work.** `Reconcile → Append → ValidateCommit →
   ValidateProposal → validateBondRegs → recentBondRegNonces → blockByHash →
   Block.Hash → sha256.Sum256`, plus the same `Block.Hash` under `Head()` and
   `blockWeight`, plus `bond.verifyLabels` (real PoST work, the minority).

## The mechanism

`Block.Hash()` re-marshaled the entire unsigned body — including each BondReg's
~1.5 MB `Answer` proof — and re-hashed it on **every call**. `blockByHash` is a
tip-back scan that called `.Hash()` per step; `recentBondRegNonces` does up to
K=8 such lookups **per validated block**. A full-fetch `Reconcile` re-validates
the whole fork, so at depth n it paid O(n × K × scan) full-body marshal+hash on
the single node thread — tens of GB of SHA-256 at h70+ with 5–7 reg blocks.
Even `Head()` re-hashed the multi-MB tip per probe.

The loop that sustained it: saturation delays head-probe replies past the 8 s
request timeout → `SyncChain` falls back to the full fetch → the reply's
Reconcile blocks the thread for 16–86 s → more probes time out. Depth growth is
intrinsic: every unit of chain length adds hash work to every subsequent full
fetch — no fixed round base can outrun a per-height cost that grows with
height, which is why direction (a) would not have cured the crawl.

## The fix (this change)

Memoize `Block.Hash` (unexported memo fields; cbor-skips them; zero on decode;
`Sign` invalidates before signing since it mutates hashed content; the pruned
branch keeps priority). One hash per block per lifetime: the scan steps, head
probes, and repeat validations all become memo reads. No consensus rule, no
wire change, no timing constant — build-immutable #6 is not engaged.

Deterministic RED home: `chain.TestReconcileHashWorkIsLinear_555` counts actual
hash computations during a cold-sync Reconcile of a 24-block reg-carrying chain
— pre-fix 798 (~33/block), post-fix 25 (~1/block), budget 8/block; plus
`Head()` must add zero. RED-proven by neutering the memo check.

## What stands, what changed

- **Round base:** stays `roundAdvanceSweeps = 2`. The #549 Q3 skew derivation
  remains the binding lower bound; the certification's held measurement came
  back `G ≈ 10 s < 60 s`. `TestRoundBaseOutrunsSkew` unchanged (comment notes
  the resolution).
- **Fix (b) (renewal phase-jitter, PR #556):** stands and compounds — lighter
  blocks shrink both the remaining per-validation PoST cost and every future
  full-fetch's byte volume.
- **Residual, accepted for now:** a genuine cold-sync full Reconcile still
  re-verifies every unpruned reg's PoST labels (O(n × regs) real crypto). With
  jitter (~1 reg/block), pruning below the horizon, and full fetches rare once
  probes answer promptly, this is bounded; revisit only if the deep re-run
  shows it.
- **Deep re-run gate:** the prior "do not re-run DEEP before fix (a)" now
  reads: re-run after THIS fix lands — expect per-height ≈ sweep-phase wait +
  ~10 s gather, r0 commits, zero view-changes per the cert's Q2 bar.
