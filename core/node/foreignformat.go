package node

import (
	"errors"
	"fmt"

	"github.com/nerolabs/silt/core/chain"
	"github.com/nerolabs/silt/ports"
)

// ForeignFormat is what this node has seen of blocks in a format it cannot
// validate: how many, and the most recent one.
//
// Such a block is refused, never adopted, so a node that keeps meeting them stops
// advancing. That stall is the safe direction. An un-upgraded node waits at the era
// boundary rather than accept a format it cannot check. It keeps serving everything
// else meanwhile. What the stall must not be is silent, because an operator who sees
// a stuck head and no reason cannot tell "upgrade the binary" from "the network is
// down". Nothing here acts on the refusal. No node replaces its own binary, and an
// upgrade is always its operator's step.
type ForeignFormat struct {
	Count   int    `json:"count"`   // foreign-format blocks refused since start
	Version uint64 `json:"version"` // the most recent one's format version
	Height  uint64 `json:"height"`  // the height it claims
	From    string `json:"from"`    // the peer that sent it, hex
	Via     string `json:"via"`     // the path it arrived on
	Newer   bool   `json:"newer"`   // from an era after this build's
	Max     uint64 `json:"max"`     // the highest format this build validates
}

// ForeignFormat reports the foreign-format blocks this node has refused.
func (n *Node) ForeignFormat() ForeignFormat {
	f := n.foreignFormat
	f.Max = chain.BlockVersionWitnessable
	return f
}

// noteUndecodable records a block a peer sent that failed to decode. Only a format
// this build does not validate is recorded; any other decode failure is the caller's
// to handle. It logs at warn on the 1st, 2nd, 4th, 8th ... refusal, so a peer that
// keeps re-sending the block cannot flood the log, while the line keeps returning
// for as long as the node stays stalled.
func (n *Node) noteUndecodable(from ports.NodeID, via string, err error) {
	var ve *chain.VersionError
	if !errors.As(err, &ve) {
		return
	}
	n.Stats.ForeignFormatBlocks++
	n.foreignFormat = ForeignFormat{
		Count: n.Stats.ForeignFormatBlocks, Version: ve.Got, Height: ve.Height,
		From: fmt.Sprintf("%x", from[:]), Via: via, Newer: ve.Newer(),
	}
	c := n.Stats.ForeignFormatBlocks
	if c&(c-1) != 0 {
		return
	}
	action := "the sender runs a release older than the versioned block formats; nothing on this node needs to change"
	if ve.Newer() {
		action = "the network may have activated a newer era: this node will not advance past it until its operator upgrades the binary"
	}
	n.logf(ports.LogWarn, "foreign-format: refused a block in a format this build cannot validate",
		"format", ve.Got, "validates", fmt.Sprintf("v1..v%d", chain.BlockVersionWitnessable),
		"height", ve.Height, "from", from, "via", via, "refused", c, "action", action)
}
