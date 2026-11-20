package node

// PROPOSING WHEN THERE IS WORK, NOT ONLY WHEN THE SWEEP COMES ROUND.
//
// The designee proposes on its sync sweep, so a publish waits up to a full
// ChainSyncInterval for the next block, and the chain commits at most one entry
// budget per interval: about 1.7 three-signature publishes a second at the shipped
// 30 s, measured on a simulated WAN. With ProposeDelay set, a publish entering the
// mempool, or a commit that leaves entries waiting, offers the designee a proposal
// that long afterwards. The same rules decide the offer as decide the sweep's
// proposal (proposeDrain), and only the round's designee may act on it, so the
// designation, the takeover walk and the sign-slot watermark are untouched. On the
// same WAN the chain then followed the offered load to 40 publishes a second with a
// p95 latency near 5 s.
//
// One offer is outstanding at a time; arrivals inside the delay ride the same one,
// which is what batches them into a block.

// offerProposal arms one early proposal ProposeDelay from now, if none is armed and
// there is entry work to carry.
func (n *Node) offerProposal() {
	if n.cfg.ProposeDelay <= 0 || n.proposeArmed || n.chain == nil || len(n.pendingEntries) == 0 {
		return
	}
	n.proposeArmed = true
	n.clock.AfterFunc(n.cfg.ProposeDelay, func() {
		n.proposeArmed = false
		n.proposeDrain(true)
	})
}
