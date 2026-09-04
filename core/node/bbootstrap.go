package node

// R2.9a — the node-tier read of the B_bootstrap histogram. core/credit/bbootstrap.go is
// the source and carries the reasoning; this file only reaches the ledger.
//
// The previous shape narrowed the ledger snapshot here, dropping a salted requester
// label before publication. Under the histogram there is nothing left to narrow: the
// core object already carries no per-identity datum — no id, no label, no exact age, no
// row — so this is a pass-through, and that is the point.
//
// Instrumentation only: nothing here moves credit, escrow or standing, and it is NOT
// gated on -economy (the counters accrue on every node; only disbursement is gated).
// Whether it is PUBLISHED is a separate, default-off decision made in cmd/silt.

import "github.com/nerolabs/silt/core/credit"

// BBootstrap snapshots the histogram from this node's ledger. The bool is false with no
// ledger wired, or with a ledger that does not implement the export (a test double) —
// the same optional-interface pattern EconomySelf uses, so ports.CreditLedger stays the
// consensus-relevant surface. Loop-owned (it reads the ledger); call it on the event
// loop. Reading moves nothing.
//
// THIS IS THE SEAM THE MINIMUM-REQUESTER FLOOR IS APPLIED AT (G-BB-11). It is the one
// route the histogram takes out of the ledger to any consumer — the ledger reference is
// unexported and nothing outside core/credit calls BBootstrapSnapshot — so applying the
// floor here makes "no census count reaches a consumer below the floor" structural
// rather than a convention every future publisher has to remember. The rule and its
// derivation live in core/credit; this line is the enforcement point, and
// TestR29aBB15TheFloorIsAppliedAtTheOnlySeam pins it.
func (n *Node) BBootstrap() (credit.BBootstrapHistogram, bool) {
	if n.ledger == nil {
		return credit.BBootstrapHistogram{}, false
	}
	r, ok := n.ledger.(interface {
		BBootstrapSnapshot() credit.BBootstrapHistogram
	})
	if !ok {
		return credit.BBootstrapHistogram{}, false
	}
	return r.BBootstrapSnapshot().WithMinRequesterFloor(), true
}
