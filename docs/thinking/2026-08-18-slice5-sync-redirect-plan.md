# Slice 5 — the sync-redirect enablement (turn pruning ON, safely): plan before code

**Date:** 2026-08-18 · **Author:** builder · **Status:** DELIBERATION (plan; no code — the
most consensus-critical slice, PE said build carefully not to a field date). **Basis:** the PE
ruling `principle-engineer/slice4-sync-redirect-ruling-PE-2026-08-18.md` (Opt A rejected as a
C1/long-range break; safe unblock = suffix-sync from the node's OWN finalized head +
WS-checkpoint/archive for cold nodes). Slices 1–4 landed (`a36cbd4`).

## The mechanism paragraph (build-immutable #6)

*The failure* (why pruning is currently blocked): mesh catch-up is a full-chain-from-genesis
`Reconcile` (`fetchFull` requests `{Height:0}`, `chainrole.go:1099`); once nodes prune, any
**behind** peer replays the pruned chain and the slice-3 Q2 gate rejects the pruned blocks in
its lag gap. *The fix* (Z): a behind node requests the suffix **from its OWN finalized head**
(`{Height: Fh}`), which the peer still serves un-pruned for `d < safetyDepth`; the node anchors
the suffix on its own already-verified finalized block and re-verifies only the (full) suffix.
Trust is anchored on the node's OWN state, never a peer-served head — so the Q1 forgery
(garbage-`Answer` bond + forged super-quorum, served pruned) has no purchase.

## Why a TRUE suffix-Reconcile is hard, and the minimal alternative

`Reconcile` (chain.go:2487) replays the fork **from genesis** in a `tmp` replica and
`heavier(tmp,c)` (2568/2578) compares `Weight()` — the **sum of accumulated bonded weight over
all history**. A suffix replayed alone has no bonded state from `[0,Fh)`, so its weight (and
fork-choice) is wrong. That is exactly the "state snapshot at the anchor doesn't exist" the PE
named in Finding A. Two ways to supply it:

- **M1 — PREPEND-and-reuse (recommended).** The node already HOLDS its own verified prefix
  `[0,Fh)` (in `c.blocks`, pruned/light below its floor). Reconstruct
  `full = c.blocks[0:Fh] ++ suffix` and call the **existing** `Reconcile(full)`. `tmp` replays
  from genesis accumulating correct weight; the node's own pruned prefix is **below its own
  trustFloor**, so the slice-3 Q2 gate ACCEPTS it (this is the piece slices 2–3 built for exactly
  this moment); the full `[Nf,Fh)` window + the peer suffix are re-verified. Reuses the audited
  Reconcile + finality gate + Q2 gate verbatim — **no new consensus code**, only a request-height
  change + a prepend in `fetchFull`. Cost: re-verifies the node's own recent full window each sync
  (bounded ≈ 2·BondTTL blocks; the head-probe already elides the no-op-sweep case).
- **M2 — true `ReconcileSuffix(anchorState, suffix)`.** Seed `tmp` with the node's state as of
  `Fh` (reconstructed by replaying `c.blocks[0:Fh]`, or forked from `c`), then replay only the
  suffix. Less re-verify CPU, but a NEW consensus-path with its own state-seeding + weight
  bootstrapping — more surface, more risk, the thing the PE's "consensus is boring" pushes against.

**Recommendation: M1.** It realizes the PE's semantics ("anchor on the node's own finalized
head; re-verify the suffix; adopt only a fork containing the finalized head") by *reusing* the
exact path already certified for I1/I4/I5, and it turns the slice-3 pruned-tolerance gate into
the load-bearing enabler rather than adding a parallel trust path. Flag for a one-line PE ack that
M1 (prepend own verified prefix, reuse genesis-rooted Reconcile) is an acceptable realization of
the blessed suffix-request approach — it is *more* conservative than a true suffix-Reconcile, not
less. (This is the mechanism decision to confirm before coding.)

## The request height — `Fh`, not the floor

Request `{Height: Fh}` where `Fh = len(c.blocks)-1` (the node's finalized/committed tip in
objective mode — committed == final, the same anchor the Reconcile finality gate uses). NOT the
node's floor `Nf`: requesting below `Fh` risks pulling the peer's pruned `[Nf,Pf)` blocks, which
are ≥ the node's floor → the Q2 gate would (correctly) reject them. `Fh` is the highest point at
which the node is fully caught up, so for `d < safetyDepth` the peer's `[Fh, peerHead]` is all
above the peer's floor → un-pruned → re-verifiable. The one-block overlap at `Fh` is enough for
the equivocation scan (forks below `Fh` are impossible under finality; the scan lives above it).

## How each case resolves (the PE's `d` split, mechanized)

- **`d < safetyDepth` (restart / brief partition / latecomer-in-window — the FIELD case):** peer
  serves full `[Fh, peerHead]`; M1 reconstructs + Reconciles; adopts if heavier. **Works.**
- **Deep-cold WITH a WS-checkpoint:** `trustFloor = max(WSCheckpoint, Nf)` already ≥ checkpoint, so
  the peer's pruned prefix below the checkpoint is Q2-accepted; request from the checkpoint height.
  **Works** (the static `-ws-checkpoint` path, now the anchor).
- **Deep-cold WITHOUT a checkpoint (`d > safetyDepth`):** the peer's `[Fh, Pf)` is pruned and ≥ the
  node's floor → Q2 rejects → Reconcile fails. This is the weak-subjectivity limit. **Must
  stall-and-SIGNAL "need a WS-checkpoint / archive node", never silently fail** (PE, I4). A new
  observable: on `ErrPrunedAboveHorizon` from a sync fetch, surface a distinct
  `need-checkpoint` signal/log (S5 honest observability), not a silent "not adopted".

## Enablement (the actual prune turns on here)

Once M1 sync is in and tested, wire `pruneBelowHorizon()` (slice 4, dormant) into the node's
post-commit path (next to `onCommit`, `chainrole.go:394/844`) so a validator sheds below its floor
as finality advances. This is the line that closes the field OOM. Gate it so it only runs in
objective/final mode (pruneFloor already returns 0 otherwise).

## Consensus-invariants (I1–I5) — the binding PR-body statement

- **I4 (liveness) — the headline.** A node behind by `d < safetyDepth` MUST catch up (suffix-sync
  from its own finalized head); a node beyond the window is *correctly* unable to sync from a
  pruned peer and MUST stall-and-signal-need-checkpoint (never silently adopt). Asserted by the
  catch-up test + the deep-cold-signal test.
- **I3 (set integrity).** The trusted validator set below the horizon is read from the node's OWN
  finalized snapshot (its own `c.blocks[0:Fh]`), never re-derived from pruned proofs and never
  taken from a peer's claim. M1 guarantees this structurally (the node supplies its own prefix).
- **I1 (safety).** Preserved — no quorum re-sizing; adoption still runs the existing finality gate
  (fork must contain the node's committed head) + `heavier` over the full accumulated weight.
- **I5 / accountable-safety.** The equivocation scan still runs on the fetched suffix (forks only
  exist above the finalized head); slashing unaffected.

## ★ The merge-gate SAFETY oracle (PE-specified — build FIRST, failing-first)

**A node NEVER adopts a chain it cannot anchor to its own finalized head / WS-checkpoint.** The
Q1 attack as a RED-first test: a peer serves a chain whose below-`Fh` history contains a
**garbage-`Answer` bond** "signing" a **forged super-quorum**, served **pruned**, NOT matching the
receiver's own finalized block at `Fh` → the node must **REJECT** (never adopt, never forge
standing). RED if M1 trusted the peer's prefix; GREEN because M1 uses the node's OWN prefix and
the finality gate rejects a non-matching `Fh`. Ship it with the change (the PE's load-bearing gate).

## Failing-first test list (sub-sliced)

**5a — suffix-sync (no prune enabled yet, so it's testable in isolation):**
1. `TestSuffixSync_CatchUpWithinWindow` — a behind node (d < safetyDepth) suffix-syncs from a
   peer whose recent history is pruned; adopts, ends at peerHead. (RED on `{Height:0}` full fetch
   against a pruned peer; GREEN with `{Height:Fh}` + prepend.)
2. `TestSuffixSync_Q1ForgeryRejected` — the merge-gate oracle above.
3. `TestSuffixSync_DeepColdStallsAndSignals` — d > safetyDepth, no checkpoint → not adopted +
   `need-checkpoint` signalled (not silent).
4. `TestSuffixSync_CompetingFinalizedForkRejected` — a peer suffix that diverges at/below `Fh` →
   `ErrPreFinalityReorg` (finality gate still governs).
5. `TestSuffixSync_EquivocationStillCaughtInSuffix` — a cross-fork double-sign above `Fh` is
   slashed from the suffix (I5).

**5b — enable the prune:**
6. `TestPruneEnabled_MeshStillConverges` — a multi-node objective net with pruning ON: all nodes
   commit, prune below their floors, and a lagging node catches up via 5a. (The integration proof
   the OOM fix doesn't break the mesh.)
7. Node-level: after N commits with pruning wired, resident heavy payload is bounded (the OOM
   assertion, node/sim tier).

## PE ACK (2026-08-18) — M1, and the override is the merge-gate invariant

`slice5-suffix-mechanism-ruling-PE-2026-08-18.md`. **ACK M1** (reject M2). Key confirmations:
- **The substrate is already wired:** `Reconcile` already sets `tmp.trustFloorOverride = c.trustFloor()`
  (slice 3), which is *precisely* what M1 needs — during the genesis replay `tmp`'s running floor would
  reject the node's own pruned prefix, but the pinned override (the node's OWN fixed floor) accepts it.
  So M1 = request-height change + prepend, on the already-certified path. Confirm the override is set in
  the path M1 exercises (it is — Reconcile, unconditionally).
- **★ The override safety invariant (a merge-gate assertion):** `trustFloorOverride` must ALWAYS be
  `c.trustFloor()` — the node's OWN anchor — and NEVER derived from `full`/the peer/the fork. A
  fork-inflated floor would get pruned forgeries accepted → the Q1 C1 break. **Add a test:** a taller
  peer fork (whose own history implies a higher floor) containing a pruned block ABOVE the receiver's
  floor is REJECTED — the receiver's fixed floor governs, not the fork's.
- **`ErrNeedCheckpoint`:** endorsed; the operator message must name the remedy (obtain a recent
  `-ws-checkpoint` out-of-band, or point at an archive node).
- **`FinalizedHeight()`:** endorsed = `len(c.blocks)-1` when finality active (committed==final in BOTH
  regimes — no committed≠final case to fear). When finality is NOT active (trusted `Quorum=1`/demo),
  fall back to **full-genesis sync + no prune** (consistent with `RetentionHorizon()`/`pruneFloor()`
  already returning 0 without finality). So `FinalizedHeight()` returns 0 without finality ⇒ `{Height:0}`
  ⇒ unchanged behavior.

## SHIPPED (2026-08-18, 5a+5b together, ablation-verified)

M1 as planned, and it was as small as predicted. Implemented:
- **5a:** `FinalizedHeight()` (finality-gated, 0-without-finality genesis fallback); `fetchFull` requests
  `{Height: FinalizedHeight()}`; `reconstructFork` prepends our own prefix below the SERVED start height
  (keys off `served[0].Height`, robust to suffix-honoring peers AND genesis-rooted servers/adversaries);
  `ErrNeedCheckpoint` + `ChainSyncNeedCheckpoint` stat for deep-cold. The Q1 merge-gate is the chain-level
  `TestQ2_ReconcileFloorIsReceiversNotForks` (a taller fork can't inflate the receiver's floor).
- **5b:** exported `PruneBelowHorizon`, wired `pruneOnCommit` into both commit sites. Integration oracles
  (`prune_suffix_sync_test.go`): catch-up-around-a-pruned-peer (within window), deep-cold-signals-need-checkpoint,
  reconstructFork-prepend. **Ablation-proven load-bearing:** forcing the old `{Height:0}` makes the catch-up
  RED (n2 stuck at 13, wants 18).

**One behavior change, PE-sanctioned (note, not a new residual):** equivocation-on-detection over sync now
covers heights ≥ our finalized head (the suffix), not the full chain. Sub-finalized forks can't exist under
finality (the PE's own reasoning: "finality forbids forks below the finalized head"), so a sub-finalized
double-sign is either already-slashed at commit or a >f attack out of the safety model. The #184 wire drill
(recent-tip baiting, at/above the finalized head) is preserved — proven by the suite. In-code note at the
`slashEquivocators` call.

**A real bug the suite caught (evidence > assumption):** the first `reconstructFork` assumed the served run
starts at `reqHeight` and broke the #184 drill (the adversary serves a genesis-rooted fork). Fix: key off
`served[0].Height`. Full core suite green, vet clean.

## Open questions to confirm before coding

1. **M1 vs M2** — one-line PE ack that prepend-and-reuse is an acceptable (more-conservative)
   realization of the blessed suffix-request. (My strong lean: M1.)
2. **The deep-cold signal shape** — a `Stats`/log `need-checkpoint`, an `onNeedCheckpoint`
   callback, or an error variant surfaced by `SyncChain`? Lean: a distinct sentinel
   (`ErrNeedCheckpoint`) surfaced + logged, mirroring the existing "peer chain not adopted" debug.
3. **Request height source** — confirm `Fh = len(c.blocks)-1` is the right finalized anchor in
   both launch (committed==final over anchors) and mature-epoch regimes; add a `FinalizedHeight()`
   accessor rather than open-coding the off-by-one.
4. **Scope this session** — 5a (sync) alone, or 5a+5b (enable) together? Lean: 5a first (testable
   without turning pruning on; keeps the risky enablement behind its own green gate), then 5b.
