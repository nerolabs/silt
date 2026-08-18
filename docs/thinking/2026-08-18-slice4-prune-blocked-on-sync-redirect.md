# Slice 4 (the actual prune) is BLOCKED on a sync-protocol decision — evidence + the fork

**Date:** 2026-08-18 · **Author:** builder · **Status:** DELIBERATION / STOP-and-decide (a
#6-gated fork surfaced during code-grounding; no slice-4 code yet). **Basis:** slices 1–3
landed (`76fc7ef`); the H2 design doc `2026-08-18-serve-retain-from-checkpoint-oom-fix.md`.

## What slice 4 was supposed to be (design doc)

Drop `BondReg.Answer` from `c.blocks` below `RetentionHorizon()` (in-place) + serve the
light chain. Persistence and serve get pruning **for free** (evidence): the daemon persists
via `chainstore.Save(chainPath, ch.Blocks(0))` (whole-chain encode, `daemon.go:768/1099/1252`),
and serves via `chain.EncodeBlocks(n.chain.Blocks(msg.Height))` (`chainrole.go:400/410`) —
both read `c.blocks`, so pruning it in place prunes both. `Block.Prune()` already deep-copies
(slice 2), so `c.blocks[i] = c.blocks[i].Prune()` is a clean in-place shed.

## ★ THE BLOCKER — pruning breaks mesh catch-up for ANY behind peer

`SyncChain` (`chainrole.go:1065`) is, verbatim from the code:

1. A cheap **head probe** (#382): peer head hash == mine ⇒ identical committed history ⇒
   **skip**. Caught-up peers do zero work → no pruning conflict. ✓
2. On **any** head difference (peer ahead, divergent fork, or too old to probe) →
   **`fetchFull`**: request `MsgGetChain{Height:0}` (the WHOLE chain from genesis) and
   `Reconcile(full)` — replaying every block in a `tmp` replica from genesis. The comment is
   explicit: *"A genuine genesis-to-head block DIFF within Reconcile is a further follow-up"*
   — **there is no suffix-diff sync.** It is all-or-nothing full-genesis re-validation.

The slice-3 Q2 gate rejects a pruned block presented at/above the RECEIVER's own floor. So if
a server pruned below its floor `Fs`, a receiver whose floor is `Fr < Fs` (i.e. it is behind)
replays the server's chain and hits pruned blocks in the gap `[Fr, Fs)` — **rejected**
(`ErrPrunedAboveHorizon`), Reconcile returns `rerr`, "peer chain not adopted". This breaks:

- a **restarted** validator catching up, a **latecomer** joining, a **partitioned** validator
  healing — every non-trivial sync is a full-genesis Reconcile, and every one of them hits a
  pruned peer's gap the instant pruning is enabled.

Caught-up peers (head-identical) are fine; *any* behind peer is not. So the prune is **not a
memory tweak** — enabling it is a change to how nodes sync, and it cannot ship alone.

## Why this is #6-gated (don't guess)

The unblock is the design doc's **(A) redirect**: "the rolling horizon IS a rolling
WS-checkpoint." Concretely, Reconcile must stop re-verifying from genesis and instead **trust
a pruned prefix** anchored by hash-linkage from a super-quorum-**final** head it CAN verify
(each block's `Hash()` commits its `Prev`, so a verified finalized head authenticates the
whole chain back through the pruned prefix). That changes Reconcile's **trust model** — from
"re-verify every block" to "verify the finalized suffix, trust the hash-linked pruned prefix."
That is a consensus-rule / published-claim change (build-immutable #6 + the consensus-
correctness discipline): **consult PE/research before building**, with a failing-first oracle
and the I1–I5 statement, exactly as slice 3 was gated.

## The safety guard the prune ALSO needs (independent of sync)

`RetentionHorizon()` returns 0 without finality (safe). But with finality active **and
`BondTTLBlocks == 0`** (common in tests/legacy), `safetyDepth = 2·0 = 0` and the horizon
collapses to `finalizedHead` — pruning right up to the tip, stranding the `BondRegHeadWindow`
(~8-head) re-verify window the research cert relies on. The prune must **retain at least
`max(safetyDepth, BondRegHeadWindow + margin)`** below the finalized head — i.e. never prune
when `2·BondTTL` is degenerate. A cheap guard, but load-bearing; note it for whatever slice
lands the prune.

## The fork (owner/PE decision — this is the STOP)

- **P1 — Prune mechanism DORMANT now; sync-redirect is the (research-gated) enablement.**
  Build `chain.pruneBelowHorizon()` (in-place `Answer` shed below the horizon, with the
  safety guard) + failing-first unit tests, but DO NOT call it from the commit path — dormant,
  exactly like slices 1–3. Route the (A) checkpoint-redirect Reconcile change to PE/research as
  the gate that enables it. *Pro:* real, tested progress on the shed logic; no guess on the
  security-relevant sync change; matches the dormant-substrate pattern. *Con:* the OOM does not
  actually close until the redirect lands (interim field air stays e2-medium + GOMEMLIMIT +
  #466 buffer, per PE).
- **P2 — Route ALL of slice 4 to PE/research first.** The redirect is the crux and the prune
  is trivial once it's decided; write the consult, build nothing until it returns. *Pro:*
  one clean pass. *Con:* no code progress this session; the shed logic is uncontroversial and
  could be banked.
- **P3 — Prune + a suffix-request unblock (NOT recommended).** Make the requester ask from a
  recent finalized point so a behind peer never pulls pruned blocks. *Con:* reopens the
  suffix-Reconcile the PE explicitly DROPPED (Finding A), and `SyncChain` deliberately compares
  whole chains to catch equal-length forks — this is a closed decision with its own safety
  questions. Do not reopen without PE.

## Recommendation

**P1.** Bank the prune mechanism dormant + tested (the shed + the `BondTTL`-degenerate guard
are uncontroversial and #6-clean as long as nothing calls them), and send the **(A)
rolling-checkpoint Reconcile trust-model change** to PE/research as the enablement gate — it is
the one genuinely consensus-relevant piece and must not be guessed. This keeps the OOM program
moving without spending a wrong guess on the sync protocol.

**Consult to draft (if P1/P2):** *"Rolling-horizon-as-rolling-WS-checkpoint: may Reconcile
trust a pruned prefix authenticated by hash-linkage from a super-quorum-final head, instead of
re-verifying from genesis? What is the failing-first oracle and the I1/I3/I4 statement?"*
