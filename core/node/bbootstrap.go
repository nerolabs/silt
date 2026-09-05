//go:build bbootstrap

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
//
// THIS FILE COMPILES ONLY UNDER THE `bbootstrap` BUILD TAG (D-BB-BUILD-TAG,
// 2026-09-05). There is no untagged stub, because nothing untagged calls it: the only
// caller is cmd/silt's status renderer, which is tagged the same way. A default silt
// binary has no Node.BBootstrap method at all.
//
// Whether it is RECORDED and whether it is PUBLISHED are now the SAME default-off
// decision, made in cmd/silt: -bbootstrap gates the clock injection, so on a node that
// was not asked for the instrument this method has nothing to read — no account is
// stamped and the block reports ClockSource "none" with a dead age axis.

import "github.com/nerolabs/silt/core/credit"

// BBootstrap snapshots the histogram from this node's ledger. The bool is false with no
// ledger wired, or with a ledger that does not implement the export (a test double) —
// the same optional-interface pattern EconomySelf uses, so ports.CreditLedger stays the
// consensus-relevant surface. Loop-owned (it reads the ledger); call it on the event
// loop. Reading moves nothing.
//
// THE FLOOR IS NOT APPLIED HERE ANY MORE, AND THAT IS THE FIX. The reviewed build called
// the RAW exported snapshot here and floored it on this line, which made "no unfloored
// census reaches a consumer" a property of one line in one file. A reviewer ablated past
// the source gate that guarded it by adding a second unfloored export elsewhere in five
// lines. Widening the gate to walk the tree would not have closed it either: this
// assertion is DUCK-TYPED on a method name, so a name-based gate cannot see a second
// exported reader added inside core/credit.
//
// So the raw snapshot is now UNEXPORTED and credit.BBootstrapPublish — which floors — is
// the only route out of that package. This seam can no longer obtain an unfloored census
// even if it wanted to, and neither can any future publisher, in any file, under any
// method name. The rule, its derivation and the exact scope of the compiler's guarantee
// live in core/credit; BB-20 (cmd/silt/r29a_bb20_equivalence_test.go) runs the resulting
// property at the wire.
func (n *Node) BBootstrap() (credit.BBootstrapHistogram, bool) {
	if n.ledger == nil {
		return credit.BBootstrapHistogram{}, false
	}
	r, ok := n.ledger.(interface {
		BBootstrapPublish() credit.BBootstrapHistogram
	})
	if !ok {
		return credit.BBootstrapHistogram{}, false
	}
	return r.BBootstrapPublish(), true
}
