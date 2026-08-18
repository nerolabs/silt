# Serve/retain from a checkpoint — the OOM/box fix: deliberation before code

**Date:** 2026-08-18 · **Author:** builder · **Status:** DELIBERATION (no code yet — a
design fork needs an owner/PE call first). **Basis:** PE decision note
`principle-engineer/299-research-outcome-PE-decision-note-2026-08-18.md` ("fix the OOM by
pruning + serve-from-WS-checkpoint — no crypto"); PE take
`principle-engineer/299-succinct-proofs-PE-take-2026-08-18.md` §3; the exhaustive code map
(this session). #299-full is parked; this is the blessed, no-crypto OOM/box fix.

## The two drivers this must kill (from the wire heap profiles)

1. **Chain-serve (currently DOMINANT, ~144 MB live):** a syncing peer sends
   `MsgGetChain{Height:0}`; the server `EncodeBlocks(Blocks(0))` marshals the WHOLE
   bond-reg-laden chain into one buffer (`chainrole.go:400,410`). Every ~1.5 MB `BondReg.Answer`
   (the space-time proof, `chain.go:330-352`) piles into that buffer.
2. **Chain-retention (~186 MB resident):** every node holds every block — all history, all
   1.5 MB `Answer` payloads — forever (append-only; **no block pruning exists today**,
   verified). `bonded`-standing decays on TTL (`chain.go:624`) but the block bodies never leave.

Both are the *same artifact* (the 1.5 MB proof) seen from two angles: encoded-on-serve, and
resident-in-store. A recent-window retention bounds both.

## What the code already supports (evidence — cheap wins hiding here)

- **The server already honors `msg.Height`:** `blocks := n.chain.Blocks(msg.Height)`
  (`chainrole.go:400`). `Blocks(from)` returns a correct arbitrary suffix (`chain.go:1575`).
  So serving a suffix is a **requester-side flip**, not a server rewrite — the requester just
  always asks `Height:0` (`chainrole.go:1099`).
- **Equivocation-slash survives a suffix:** `slashEquivocators`→`FindEquivocations`
  (`equivocation.go:120`) is purely intra-height (same height, different hash, same signer). It
  needs NO pre-checkpoint blocks. ✓ Safe on a suffix.

## The blockers (evidence — why it isn't a one-liner)

- **`Reconcile` REQUIRES genesis** (`chain.go:2388`): `if fork[0].Height != 0 || fork[0].Hash()
  != c.blocks[0].Hash() { return ErrForeignGenesis }`. It will reject a checkpoint-anchored
  suffix. This is the consensus-critical crux (#6-gated: model-check discipline applies).
- **`reorgDropped` assumes index==height from genesis** (`chainrole.go:526`, "genesis (i=0) is
  shared by construction"). On a suffix, index 1 is height cp+1 → miscount. Needs a height-based fix.
- **The block hash covers `BondReg.Answer`** (it's a hashed field). So you **cannot strip the
  proof payload from a retained block without changing its `Hash()`** — which would break parent
  linkage, the WS-checkpoint hash match, and every signature over the block. "Prune the proof"
  therefore means *a node never HOLDS the pre-horizon body in the first place* (fresh-sync case),
  or *a genesis-old node drops bodies below the horizon and serves "start higher" for older
  asks* — NOT mutating a block in place.

## ★ THE LOAD-BEARING FINDING — the static WSCheckpoint does NOT fix the FIELD OOM

`WSCheckpoint` is a **static CLI flag** (`-ws-checkpoint HEIGHT:HASH`, `daemon.go:117,607`) set
once at boot **for a fresh node JOINING an already-mature network**. It is **never advanced** by
a running node (no rolling-checkpoint code exists — verified). The field's MATURING nodes launch
fresh with `-anchors`/`-mature-validators`, **not** `-ws-checkpoint`, so their
`WSCheckpoint.Height == 0`. **"Serve/retain from the checkpoint" is a no-op for exactly the
nodes that OOM in the field** — they are genesis-old with no checkpoint, holding full history.

So the literal blessed phrasing helps a *fresh bootstrapping* node (cheap checkpoint-sync) but
does **not** bound a *long-lived genesis-old* node's serve/retention — which is the observed
field driver. To fix the field OOM, the horizon a node serves/retains from must be **derived and
rolling**, not the static bootstrap pin.

## The design fork — this is the decision to make BEFORE coding

**H1 — Static WSCheckpoint only.** Requester asks from `cfg.WSCheckpoint.Height` (0 ⇒ genesis,
unchanged); Reconcile accepts a fork anchored at the checkpoint. *Scope:* small, contained,
low-risk (consistent with the trust the node already places in its checkpoint). *Effect:* speeds
fresh checkpoint-sync and bounds a *fresh* node's retention. **Does NOT fix the field OOM**
(genesis-old no-checkpoint nodes unaffected). Honest verdict: a real but *different* win —
bootstrap cost, not the box fix.

**H2 — Rolling finality-derived retention horizon (the actual box fix).** Introduce a local
`retentionHorizon = finalizedHead − safetyDepth`, where `safetyDepth ≥ BondTTLBlocks + reorg/
slashing depth` (the weak-subjectivity period the doc already names, `chain.go:195`). The node
(a) serves from `max(reqHeight, horizon)`, (b) requests from its own horizon, (c) drops block
bodies below the horizon from its store, (d) Reconcile accepts a fork anchored at/after the
horizon. *Effect:* bounds serve AND retention to a recent window on **every** node → returns the
validator to a 2 GB box → fixes the field OOM. *Cost/risk:* a NEW consensus-adjacent mechanism —
**#6-gated**: it must never prune anything a node could still need to (i) defend finality against
a legal reorg, (ii) slash a cross-fork double-sign, or (iii) serve to a lagging honest peer that
is behind the horizon. That last one is the sharp edge: **if I prune below my horizon and a slow
peer is further back, I can no longer serve it** — it must checkpoint-sync from the horizon
instead (which is *safe* — the horizon is finalized — but is a protocol behavior change, not just
a memory tweak). Needs failing-first tests + an oracle + PE/research review of `safetyDepth`.

**H1.5 — H1 now, H2 as the sequel.** Land the contained checkpoint-anchored serve/Reconcile
(H1) first (it's the reusable substrate — suffix-Reconcile, height-based reorgDropped, requester
Height plumbing), THEN build the rolling horizon (H2) on top once `safetyDepth` and the
serve-to-lagging-peer behavior are PE-blessed. H1 ships value (bootstrap) and de-risks the hard
part; H2 is where the field OOM actually closes.

## Reconciliation with PR #466 (pagination)

Under H2 the served window is already recent/bounded, so `EncodeBlocksUpTo` pagination becomes a
**secondary belt-and-suspenders** — it caps the buffer *if the horizon..head window itself still
exceeds `maxChainReplyBytes`* (a launch-genesis burst could). That confirms the PE ranking:
horizon/pruning is primary, pagination secondary. #466's 3 open questions soften — the "atomic
rollout" concern is smaller for an additive suffix bound. Recommend: **fold #466 into this track**
as the inner buffer bound, not a parallel PR.

## Recommendation (for the owner/PE call)

1. **H2 is the fix; H1 alone is not** — the field OOM is on genesis-old no-checkpoint nodes, so
   only a rolling finality-derived horizon bounds them. Do not ship H1 and call the OOM fixed.
2. **Sequence H1.5:** build the checkpoint-anchored suffix substrate (Reconcile-suffix +
   height-based reorgDropped + requester-Height), which is contained and testable in isolation,
   THEN the rolling horizon + body-pruning + serve-to-lagging-peer on top.
3. **PE/research owns two knobs before H2 code:** `safetyDepth` (must clear the weak-subjectivity
   period so pruning never strands finality/slashing evidence) and the **serve-to-lagging-peer**
   contract (a peer behind my horizon gets a checkpoint-sync, not a genesis serve). These are
   consensus-adjacent (#6) — failing-first + oracle required.
4. **Interim, if flixz/field needs air now:** the lower-risk stopgaps already on the table
   (e2-medium box + GOMEMLIMIT; #466 buffer cap) hold the line while H2 is built correctly. Don't
   rush a consensus-adjacent prune.

**Open questions for the call:** (a) is a rolling finality-horizon acceptable as an M0
mechanism, or does it wait behind #183? (b) `safetyDepth` value/derivation. (c) serve-to-lagging
-peer: checkpoint-sync redirect vs. keep-genesis-serve-on-demand. (d) fold #466 in, or keep separate?

## PE RULING (2026-08-18) — resolved, H1 green-lit

`principle-engineer/rolling-horizon-oom-ruling-PE-2026-08-18.md`.
- **Q1 IN-SCOPE, not a consensus-RULE change, NOT a #183 blocker.** The horizon reuses the
  *existing immutable finalized head* as the prune anchor (finality already un-reorgable:
  `chain.go:529/909/2407`), so it changes only *what a node retains/serves* — not fork-choice/
  finality/quorum/slash. Same class as the inbound-queue boundedness floor. It's the **PRIMARY
  no-crypto structural return-to-2GB fix** (more direct than #299/aggregation — bounds retained
  COUNT, not per-proof wire size). Build careful, not to a field date; the grade runs on the
  interim box.
- **Q2 safetyDepth → research** (consult sent `silt-retention-horizon-safetydepth-CONSULT-2026-08-18.md`).
  PE narrowed: reorg term DROPS OUT (anchor below finality subsumes it); floor =
  `max(BondTTLBlocks, slashingWindow) + margin` below finality; three non-prune guarantees must be
  PROVEN. Crux research must settle: is slashing evidence bounded (⇒ prunable) or indefinite.
- **Q3 lagging-peer = (A) primary + (C) complementary; REJECT (B).** (A) redirect a lagging peer to
  checkpoint-sync from my finalized horizon = the rolling horizon IS a rolling WS-checkpoint (one
  anchor serves retention + lagging-serve; generalizes the static `-ws-checkpoint`). (C) optional
  archive role retains full history for deep-cold/forensic. WS-window caveat: (A) serves a peer
  lagging < ~BondTTL; longer needs out-of-band checkpoint or (C). No new trust assumption.

## H1 BUILD PLAN (green-lit — contained, unit-tested in isolation, failing-first)

The substrate H2 builds on. Anchor is the node's OWN trusted checkpoint (`cfg.WSCheckpoint`),
never attacker-supplied — that's what keeps H1 safe. Pieces:

1. **`chain.ReconcileFrom` (or extend `Reconcile`) — accept a fork anchored at the node's trusted
   checkpoint, not genesis.** Today `Reconcile` (`chain.go:2381`) hard-requires `fork[0].Height==0`
   + genesis-hash (`:2388` `ErrForeignGenesis`). New path: if the node has a trusted anchor
   (`cfg.WSCheckpoint.Height>0`), accept `fork[0].Height == anchor.Height && fork[0].Hash()==anchor.Hash`
   and replay from there (seed `tmp` chain at the anchor instead of `AppendGenesis`). **THE GUARD to
   prove: a fork anchored at ANY height/hash other than the node's trusted anchor (or genesis when
   no checkpoint) is still rejected** — an attacker cannot supply a fake anchor to skip verification.
   Preserve the WS-checkpoint guard (`:2398`) + finality gate (`:2437`) unchanged.
2. **`reorgDropped` → height-based** (`chainrole.go:526`). Today assumes index==height from genesis.
   Compare by height, not slice index, so a suffix (`old`/`now` both anchored at cp) counts drops
   correctly.
3. **Requester asks from the anchor** (`chainrole.go:1099` `Height:0` → `Height: n.chain.WSCheckpointHeight()`;
   0 when unset ⇒ genesis, fully backward-compatible). `old := n.chain.Blocks(0)` snapshots
   (`:1104/1119`) → snapshot from the anchor consistently so the equivocation scan + reorgDropped
   line up on the same base.
4. **Fold #466** `EncodeBlocksUpTo` as the inner buffer belt on the serve path (`chainrole.go:410`) —
   caps `anchor..head` only if it still exceeds `maxChainReplyBytes`.

**Failing-first tests (write RED first):**
- `TestReconcileFromCheckpointAnchor` — a fork anchored at the trusted checkpoint reconciles; a fork
  anchored at a DIFFERENT (attacker) height/hash is rejected (the guard); genesis path unchanged when
  no checkpoint.
- `TestReorgDroppedByHeightOnSuffix` — suffix inputs count drops correctly (the index==height bug
  would miscount).
- `TestServeSuffixHonorsCheckpointHeight` — server serves `Blocks(anchorHeight)`, requester round-trips.
- Windowed-reassembly test for #466's `EncodeBlocksUpTo` (from the branch).

**Explicitly NOT in H1** (waits on research safetyDepth): the rolling horizon derivation, body
pruning, and the (A) lagging-peer redirect. H1 only makes the machinery *accept a non-genesis
trusted anchor*; H2 makes that anchor roll with finality and prunes below it.

## RESEARCH CERTIFICATION (2026-08-18) — safetyDepth resolved; H2 fully specced

`research-outcome/safetyDepth-retention-horizon-RESEARCH-CERTIFICATION-2026-08-18.md`. The
`max(BondTTL, slashingWindow)` framing was the wrong frame; the clean answer is a **retention-class
split** that dissolves the slashing crux:

| Class | What | Size | Retention |
|---|---|---|---|
| STATE | `slashed[id]`/`bonded[id]`/validator set (`chain.go:2226,2512`) | O(validators), tiny | **always** (never pruned; carried via `adopt()`) |
| Light metadata | block headers + consensus sigs (PrepareQC/Atts) | ~KB/block | **deep/cheap** (even indefinite) — serves slashing evidence |
| Heavy payload | `BondReg.Answer` (~1.5 MB proof) | MB/reg — **the OOM** | **prune below `finalizedHead − safetyDepth`** |

- **`safetyDepth = 2·BondTTL` (= 64 @ BondTTL 32), epoch-aligned** — heavy `Answer` ONLY.
  `slashingWindow` DROPS OUT (slashing never touches the heavy payload). Huge headroom over the true
  ~8-head `BondRegHeadWindow` need.
- **★ PAYLOAD-SELECTIVE PRUNE (soundness-relevant):** drop the `BondReg.Answer` field(s), **KEEP the
  block header + consensus sigs**. Whole-block pruning would strand late-reveal slashing evidence and
  force a bounded slashing window. This resolves my earlier "hash covers Answer" worry: the header
  (and thus the hash committing to the original Answer) + sigs stay; only the heavy `Answer` bytes
  leave storage. Re-verification below the horizon is neither possible nor needed (finalized + past
  the re-verify window).
- **3 guarantees PROVEN** (§5): (i) reorg subsumed by anchoring below finality; (ii) slashing not
  stranded (STATE bar + light sigs + revealer-supplied fork block); (iii) WS-window =
  `BondTTL + slashing-depth`, served by checkpoint-sync redirect — no extra safetyDepth constraint.
- **Byzantine-stall self-corrects:** pruning is finality-anchored, so if slash-commit stalls, finality
  stalls, the horizon stalls, no pruning happens. Margin need only cover steady-state ~1–2 block
  slash-processing.
- **Impl caveats to honor:** anchor at `finalizedHead` (NOT `head`); payload-selective; epoch-align.

## H2 BUILD PLAN (now unblocked — build AFTER H1, on the careful track)

1. **Rolling horizon** `retentionHorizon = finalizedHead − 2·BondTTL`, rounded up to the next epoch
   boundary (align with the validator-set snapshot, #357 Cond A). Derived from the finalized head
   (the existing immutable anchor) — not a new consensus concept.
2. **Payload-selective prune** below the horizon: strip `BondReg.Answer` from stored blocks, keep
   header + consensus sigs + STATE. A pruned block still hash-links and still yields slashing sigs.
3. **Serve/request from the horizon** (built on H1's suffix-Reconcile), and **(A) lagging-peer
   redirect**: a peer within the WS window checkpoint-syncs from my finalized horizon (the rolling
   horizon IS a rolling WS-checkpoint); beyond it → out-of-band checkpoint or (C) archive role.
4. **Failing-first + oracle:** the three non-prune guarantees as tests (no unfinalized prune; a
   pruned-payload block still produces a valid late-reveal slash; a lagging peer syncs across the
   horizon). Payload-selective prune preserves `Hash()` linkage — assert it.

**PE note (minor):** the payload-selective refinement borders the PE's in-scope mechanism call — the
cert flags it as a recommended refinement with a soundness consequence. It's consistent with (and
sharper than) the PE's own "hash covers Answer → can't mutate in place" framing; worth a one-line PE
ack at H2 PR time, not a re-consult.

## PE RULING (2026-08-18) — Opt 1, and the whole soundness is ONE gate

`principle-engineer/pruned-block-representation-ruling-PE-2026-08-18.md`. **Opt 1** (header +
stored-hash + sigs) — minimal, no chain-format change, preserves unbounded late-reveal slashing.
Reject Opt 3 (loses slashing for trivial memory). Opt 2 converges with #299 later. **Finding A acked**
(drop suffix-Reconcile; keep genesis-rooted, with a pruned-*tolerance* change). Q3 stored-hash sound
(full block always recomputes; pruned hash authenticated by Prev-linkage to the trust anchor).
**THE MERGE GATE = Q2:** a pruned (Answer-less) block is trusted ONLY strictly below the node's OWN
finalized/checkpoint anchor, never a peer's claim — else an attacker strips the Answer to skip
space-time verification and forge standing (a C1 break). Build it failing-first.

## SLICE STATUS + SLICE 3 BUILD SPEC (the consensus-critical core — build carefully)

- ✅ **Slice 1** — `RetentionHorizon()` (retention.go), pure + read-only, green. Committed 0d03ae1.
- ✅ **Slice 2** — Opt 1 pruned repr (`Block.Prune`/`IsPruned`, `Pruned` cbor 14, `Hash()` returns
  stored value), green, backward-compat verified. Committed 0d03ae1. **Dormant** (nothing prunes/accepts yet).
- ⏭️ **Slice 3 — Reconcile/Reload pruned-tolerance + the Q2 gate** (#6-gated; the merge-gate oracle):
  1. **`Chain.trustFloor() uint64`** = `max(cfg.WSCheckpoint.Height, RetentionHorizon())` — the height
     below which THIS node treats history as finalized/trusted (its out-of-band checkpoint OR its own
     rolling horizon; the PE's "the node's OWN anchor, never a peer's claim").
  2. **Replay tolerance** in BOTH re-validation paths — `Reconcile` (peer fork; `tmp.Append` loop,
     chain.go:2451) AND `Reload` (own disk restart, chain.go:2074). The bond re-verify is
     `validateBondReg → verifyBond(r.Answer)` (chain.go:1146); `validateBondRegs` (chain.go:1119) has the
     block `b` (Height + IsPruned) in scope — gate there. Thread the receiving node's floor into the
     replay (set a field on `tmp`, mirroring `tmp.verifyBond = c.verifyBond` at 2447).
  3. **THE Q2 GATE** (in `validateStructural`/`validateBondRegs` on the replay):
     - `b.IsPruned() && b.Height >= trustFloor` → **REJECT** (`ErrPrunedAboveHorizon`) — the C1 gate.
     - `b.IsPruned() && b.Height <  trustFloor` → accept, **SKIP** the space-time re-verify (trust);
       structural + proposer-sig checks still run (sig verifies vs `Hash()`==stored, proven slice 2).
     - `!b.IsPruned()` → full verification, any height (unchanged).
  4. **Decode invariant** (belt): reject a block with `Pruned` set AND any `BondReg.Answer` present
     (a full block can't smuggle a forged stored-hash). Add to `DecodeBlocks`/Reconcile entry.
  5. **Note:** the node pruning its OWN chain (slice 4) is a storage mutation on already-committed
     `c.blocks` — no re-validation. Reconcile's finality gate (`ErrPreFinalityReorg`, 2437) already stops
     a peer replacing the node's finalized (below-horizon) history, reinforcing Q2.
  - **FAILING-FIRST ORACLES** (merge gate): (1) **Q2 security oracle** — a fork with a pruned block at/
    above `trustFloor` is REJECTED (RED without the gate: attacker forges standing); below it, accepted +
    Prev-linkage-authenticated. (2) Round-trip: a pruned block serves/reconciles identically to full for
    every purpose except the (legitimately skipped) space-time re-verify. (3) The three non-prune guarantees.
- **Slice 4** — the actual prune (drop Answer below horizon in `c.blocks` + the durable store) + serve
  the light chain (genesis-rooted; below-horizon blocks Answer-less). **Slice 5** — fold in #466
  `EncodeBlocksUpTo` as the recent-heavy-window buffer belt.
- **Model each oracle on existing Reconcile/mature fixtures** (`matureWorld`/`matureWorld12`); a real
  objective chain with finality + bond verify is needed to exercise the floor. Interim field air stays
  e2-medium + GOMEMLIMIT + #466 buffer — don't rush this to a field date (PE).
