package main

import (
	"github.com/nerolabs/silt/core/credit"
	"github.com/nerolabs/silt/core/node"
)

// nodeEpochSource adapts the node's chain epoch to ports.EpochSource. It is a live
// read on every call (DemandEpoch is chainEpoch(): head height / EpochBlocks, 0
// with no chain or with epochs disabled), never a captured value.
type nodeEpochSource struct{ nd *node.Node }

func (s nodeEpochSource) Epoch() uint64 { return s.nd.DemandEpoch() }

// wireLedgerEpochSource is the daemon's R2.10 / F8 seam: the credit ledger reads
// its consensus epoch from THIS node's chain — the same chainEpoch() that prunes
// the demand keyset, drives the receipt bank and verifies relay anchors — so the
// paid-serial guard's expiry window and the keyset's validity window are two
// predicates on one clock (R-F8-SOURCE). Never a constant, never a wall clock,
// never the finalized head (refuted: permanently 0 without BFT finality, and one
// epoch behind the keyset at every boundary block). cmdDaemon calls it exactly
// once, right after EnableChain; the cmd-tier gates pin the call and the runtime
// property (TestF8_DaemonWiresTheLedgerEpochSourceToTheNode,
// TestF8_LedgerEpochFollowsTheChainThroughTheDaemonSeam).
func wireLedgerEpochSource(ledger *credit.Ledger, nd *node.Node) {
	ledger.SetEpochSource(nodeEpochSource{nd: nd})
}
