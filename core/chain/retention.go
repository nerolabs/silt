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
// finality-lag, safety headroom). The slashing window drops out — slashing is served
// by the retained headers+sigs, never the heavy Answer.

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
