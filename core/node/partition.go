package node

import (
	"errors"

	"github.com/nerolabs/silt/ports"
)

// errPartitioned is the send error when a message is dropped because its peer is on
// the far side of a simulated partition (SetBlockedPeers). It never escapes to a
// caller as a real failure — request() surfaces it like any unreachable peer, and the
// periodic loops retry, exactly as they would across a genuinely down link.
var errPartitioned = errors.New("node: peer partitioned (link simulated down)")

// SetBlockedPeers installs a simulated NETWORK PARTITION: this node drops every
// message to or from any peer in ids, as if the link were severed. It is how the
// daemon's -block-peers exercises the M0 consensus denial over the real wire —
// partition a validator, let each side make what progress its own rules allow,
// then HEAL (an empty set, or a restart without the flag) and watch the set
// converge on one history (#184 partition→heal). Passing an empty/nil set clears
// the partition.
//
// CORRECTED 2026-09-12. This comment used to say the healed set watches "the
// lighter fork reorg onto the heavier one". There is no such rule. Fork choice
// ranks on Height then head hash and reads nothing else (core/chain `heavier`,
// pinned by TestO3T_HeavierReadsOnlyHeightAndHeadHash), so no side wins on bond
// or weight; and in any objective posture Reconcile's finality gate admits only
// forks that CONTAIN the committed head, so a sub-quorum side commits nothing,
// stalls, and CATCHES UP on heal rather than being reorged past. A reorg is
// reachable only in a legacy (non-objective) posture, where the gate is off.
//
// This models a transport fault, not Byzantine behaviour: the node stays perfectly
// honest, it simply can't reach the far side. A real deployment never sets it.
func (n *Node) SetBlockedPeers(ids []ports.NodeID) {
	if len(ids) == 0 {
		n.blockedPeers = nil
		return
	}
	blocked := make(map[ports.NodeID]bool, len(ids))
	for _, id := range ids {
		blocked[id] = true
	}
	n.blockedPeers = blocked
}
