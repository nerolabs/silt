package chain

// Rolling retention horizon (the read-only half of the H2 payload-selective-prune
// OOM fix). A validator's chain grows O(all history) in the ~1.5 MB space-time bond
// proof (BondReg.Answer) carried by every registration block, which OOMs a small box
// (the flixz-corroborated MATURING crash). The fix bounds the RESIDENT heavy payload
// to a recent finalized window: blocks strictly below the retention horizon are
// prune-eligible — their heavy Answer may be dropped while the header + consensus
// signatures (the light chain, ~KB/block) are kept to genesis. This file computes the
// horizon; the prune/serve/Reconcile-tolerance land in the consensus-adjacent slice
// behind the Q2 gate (a pruned block is trusted ONLY strictly below the node's OWN
// finalized anchor). Design: docs/thinking/2026-08-18-serve-retain-from-checkpoint-oom-fix.md.
//
// safetyDepth = 2·BondTTLBlocks (research-certified,
// research-outcome/safetyDepth-retention-horizon-RESEARCH-CERTIFICATION-2026-08-18.md):
// one BondTTL for the bond-standing lifecycle (already ~4× the true ~8-head
// BondRegHeadWindow re-verify need) + one BondTTL margin (slash-processing depth,
// finality-lag, safety headroom). The slashing window does NOT drop out of this
// derivation the way the original note claimed ("slashing is served by the retained
// headers+sigs, never the heavy Answer"): under R0.6 (F2-EVIDENCE-RECOMPUTE) a pruned
// block is never equivocation evidence, so slashability is BOUNDED BY THE PRUNE FLOOR.
// The derivation survives because honest cross-fork detection only pairs heights at or
// above the node's own finalized head, which is above pruneFloor (every honest evidence
// block is still full); what is lost is the penalty for a late reveal below the floor,
// where ErrPreFinalityReorg already forbids adoption (R-LATE-REVEAL, held in tension —
// docs/decisions.md D-F2-EVIDENCE-RECOMPUTE).

// RetentionHorizon reports the lowest height whose heavy BondReg.Answer this node must
// still RETAIN in full: blocks with height < RetentionHorizon() are prune-eligible.
// It is anchored at the node's own finalized head, so it only advances as finality
// advances — under a >⅓ Byzantine stall finality (and thus the horizon) stalls, so no
// unsafe prune happens (self-correcting). Returns 0 (prune nothing) unless the chain
// has genuine BFT finality: without a super-quorum there is no finalized anchor to
// trust below.
func (c *Chain) RetentionHorizon() uint64 {
	if !c.finalityQuorumActive() {
		return 0 // no BFT finality → no finalized anchor → never prune
	}
	// In objective mode every committed block met the super-quorum, so the committed
	// head IS finalized (the same property the ErrPreFinalityReorg gate rests on).
	_, finalizedHeight := c.Head()
	return retentionHorizonAt(finalizedHeight, 2*c.cfg.BondTTLBlocks, c.cfg.EpochBlocks)
}

// pruneGuardMargin is the headroom, above the bond-reg re-verify window, that the prune
// floor always retains even when 2·BondTTL is smaller than that window (a degenerate/legacy
// config). It covers finality-lag + slash-processing depth. In production 2·BondTTL (=64 at
// BondTTL 32) dwarfs BondRegHeadWindow(8)+margin, so this guard is inert there and only
// bites the small-BondTTL configs — where it stops the horizon collapsing onto the tip and
// stranding the re-verify window (PE ack, slice4-sync-redirect-ruling-2026-08-18 Q4).
const pruneGuardMargin = 4

// pruneFloorAt is the pure prune-floor arithmetic: the height strictly below which a
// finalized block's heavy BondReg.Answer is prune-eligible. It retains AT LEAST
// max(2·bondTTL, headWindow+margin) full-proof blocks below the finalized head, then
// epoch-aligns (validator-set snapshot, #357 Cond A). Returns 0 when bondTTL is degenerate
// (0 ⇒ retention-decay disabled, safetyDepth would be 0 — never prune) or there isn't yet
// a full retain window of history. The retain window is >= 2·bondTTL, so the prune floor is
// always <= RetentionHorizon() <= trustFloor() — a pruned block is therefore always below
// the Q2 gate's trust floor (we prune more conservatively than we trust). Err toward
// retaining: the floor only ever LOWERS on epoch-align.
func pruneFloorAt(finalized, bondTTL, headWindow, epoch uint64) uint64 {
	if bondTTL == 0 {
		return 0 // degenerate retention config: never prune
	}
	retain := 2 * bondTTL
	if guard := headWindow + pruneGuardMargin; guard > retain {
		retain = guard
	}
	if finalized <= retain {
		return 0
	}
	raw := finalized - retain
	if epoch > 0 {
		raw = (raw / epoch) * epoch
	}
	return raw
}

// pruneFloor is the live prune floor: 0 without BFT finality (no finalized anchor to prune
// below), else pruneFloorAt over the node's finalized head, BondTTL, bond-reg head window,
// and epoch. Distinct from (and always <= ) RetentionHorizon()/trustFloor(): the prune is
// the more conservative of the two so it never sheds a block the Q2 gate would then reject.
func (c *Chain) pruneFloor() uint64 {
	if !c.finalityQuorumActive() {
		return 0
	}
	_, finalized := c.Head()
	k := c.cfg.BondRegHeadWindow
	if k <= 0 {
		k = DefaultBondRegHeadWindow
	}
	return pruneFloorAt(finalized, c.cfg.BondTTLBlocks, uint64(k), c.cfg.EpochBlocks)
}

// PruneBelowHorizon payload-selectively prunes every stored block strictly below the prune
// floor that still carries a heavy BondReg.Answer — dropping the ~1.5 MB proof while keeping
// the header + consensus sigs (Block.Prune, slice 2). Because the durable store and the
// serve path both read c.blocks (chainstore.Save(Blocks(0)); chainrole serve Blocks(from)),
// this one in-place shed bounds resident, on-disk, AND served heavy payload to a recent
// finalized window. Returns the number of blocks newly pruned. Idempotent (skips
// already-pruned and entry-only blocks). ENABLED (slice 5b): the node calls this after each
// commit; a behind peer safely catches up via suffix-sync from its own finalized head, and a
// deep-cold node beyond the WS window is told to use a checkpoint/archive (PE ruling
// slice4/5-sync-redirect-2026-08-18). Safe to call on every commit — pruneFloor is 0 (no-op)
// without BFT finality or with a degenerate BondTTL.
//
// The ARCHIVAL tier (Config.Archive, `-archive`) opts out entirely and keeps every heavy
// proof to genesis, so it can serve the deep history a pruning swarm has shed. That is a
// retention choice only — trustFloor and the horizon are untouched, so an archival node
// validates identically to a pruning one.
func (c *Chain) PruneBelowHorizon() int {
	if c.cfg.Archive {
		return 0 // archival tier: retain every heavy proof to genesis (D-TIERING §3)
	}
	floor := c.pruneFloor()
	if floor == 0 {
		return 0
	}
	n := 0
	for i := range c.blocks {
		if c.blocks[i].Height >= floor {
			break // height-ordered; nothing at/above the floor is prune-eligible
		}
		if c.blocks[i].IsPruned() || !c.blocks[i].hasHeavyBondProof() {
			continue // already pruned, or nothing heavy to shed (entry-only block)
		}
		c.blocks[i] = c.blocks[i].Prune()
		n++
	}
	return n
}

// trustFloor is the height at/above which this node re-verifies bond space-time
// proofs in full, and strictly below which it TRUSTS a payload-pruned (Answer-less)
// block. It is the node's OWN anchor: the higher of its out-of-band weak-subjectivity
// checkpoint and its rolling retention horizon (both already trusted-finalized). During
// a Reconcile replay the receiver pins tmp's floor to its own via trustFloorOverride,
// so a peer's fork can never raise the height at which pruned blocks are trusted — the
// C1 gate. Returns 0 (trust no pruned block) on a fresh node with neither anchor, which
// is the safe default: nothing is trusted-pruned until finality or a checkpoint exists.
func (c *Chain) trustFloor() uint64 {
	if c.trustFloorOverride != nil {
		return *c.trustFloorOverride
	}
	h := c.RetentionHorizon()
	if cp := c.cfg.WSCheckpoint.Height; cp > h {
		return cp
	}
	return h
}

// retentionHorizonAt is the pure horizon arithmetic (exhaustively unit-tested):
// finalizedHeight − safetyDepth, floored to an epoch boundary so the horizon lands on
// a validator-set snapshot (#357 Condition A) and retains AT LEAST safetyDepth (the
// floor only ever LOWERS the horizon → retains more; err long, per the cert). Returns
// 0 when there is not yet a full safetyDepth of history below the finalized head.
func retentionHorizonAt(finalizedHeight, safetyDepth, epochBlocks uint64) uint64 {
	if finalizedHeight <= safetyDepth {
		return 0
	}
	raw := finalizedHeight - safetyDepth
	if epochBlocks == 0 {
		return raw
	}
	return (raw / epochBlocks) * epochBlocks // floor to the epoch boundary
}
