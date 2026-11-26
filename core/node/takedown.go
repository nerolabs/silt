package node

import (
	"encoding/base64"

	"github.com/nerolabs/silt/core/chain"
	"github.com/nerolabs/silt/ports"
)

// TakedownRecord is one removal this operator honoured, with the receipt that proves
// it was the network's committed takedown and not this operator's own invention.
type TakedownRecord struct {
	Root    ports.Hash
	Curator ports.NodeID            // who listed it; zero for a takedown from before curators
	Purged  int                     // held chunks dropped
	Receipt chain.RevocationReceipt // zero when Err is set
	Err     string                  // why no receipt could be produced or verified
}

// Takedowns lists, in the order honoured, every on-chain takedown this node has
// honoured since it started.
func (n *Node) Takedowns() []TakedownRecord { return append([]TakedownRecord(nil), n.takedowns...) }

// honourTakedowns runs after the chain grows, by a commit or by catching up. When
// this operator subscribes to on-chain takedowns it drops, at once, the held bytes of
// every root the new blocks revoke, and records each removal with its receipt.
// Each receipt carries the proof that the log extends this node's previous receipt,
// so the operator's removals form one chain an auditor can check from the first.
//
// It costs one comparison when no revocation was committed: the revocation log's
// size against the size this node last honoured.
func (n *Node) honourTakedowns() {
	if (!n.honorChainRevocations && len(n.honorCurators) == 0) || n.chain == nil {
		return
	}
	size := uint64(n.chain.RevocationLogSize())
	if size == n.takedownLogSeen {
		return
	}
	if size < n.takedownLogSeen {
		// A reorg replaced blocks that carried takedowns. Bytes already dropped stay
		// dropped; the receipts issued for them name blocks a reader can no longer
		// find on this chain, and the log says so.
		n.logf(ports.LogWarn, "takedown: the revocation log shrank under a reorg; removals already honoured stand",
			"was", n.takedownLogSeen, "now", size)
		n.takedownLogSeen = size
		return
	}
	for _, l := range n.chain.RevocationsSince(n.takedownLogSeen) {
		if !n.honours(l) || !n.chain.Listed(l) {
			continue // a curator this operator does not follow, or lifted later in the same catch-up
		}
		root := l.Root
		purged := n.purgeRoot(root)
		rec := TakedownRecord{Root: root, Curator: l.Curator, Purged: purged}
		r, err := n.chain.RevocationReceiptFor(l)
		if err == nil {
			err = n.chain.ExtendFrom(&r, n.takedownReceiptSize)
		}
		if err == nil {
			err = chain.VerifyRevocationReceipt(r, n.chain.BlockAt(r.At))
		}
		if err != nil {
			rec.Err = err.Error()
			n.logf(ports.LogWarn, "takedown: honoured WITHOUT a verifiable receipt", "root", root, "purged", purged, "err", err)
		} else {
			rec.Receipt = r
			n.takedownReceiptSize = r.Size
			n.logf(ports.LogInfo, "takedown: honoured", "root", root, "curator", l.Curator, "purged", purged,
				"revoked_at", r.Height, "log_index", r.Index, "log_size", r.Size, "anchor", r.At,
				"receipt", base64.RawURLEncoding.EncodeToString(chain.EncodeReceipt(r)))
		}
		n.takedowns = append(n.takedowns, rec)
	}
	n.takedownLogSeen = size
}

// purgeRoot drops every held chunk of one root: bytes, proof and resident metadata.
func (n *Node) purgeRoot(root ports.Hash) int {
	purged := 0
	for id, m := range n.proofMeta {
		if m.Root == root {
			n.dropHosted(id)
			purged++
		}
	}
	return purged
}
