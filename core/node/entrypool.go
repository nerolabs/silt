// The #441 entry mempool (research-certified 2026-08-16, direction A): publish
// entries are MEMPOOL CONTENT the single (h, r) designee's block folds in —
// the leader-carries-the-mempool shape of every mature BFT SMR — never a
// second proposal stream. The pre-fix ProposeEntry client raced the drain
// designee for the same (h, r0) prepare slots and could win no round of any
// height: the round machinery's new-view seat re-proposed only drain blocks
// and its escape armed only on drain work, so on a matured network publishes
// starved outright (run a56ac10-42834: zero entry-blocks post-latch) and on a
// launch network a crossed publish race had no escape driver (soak run
// 9453325-7258: a 361s height stall vs the 160s computed bound). The mirror
// of the MsgSubmitBondReg path: submit → validate-on-arrival → FIFO queue →
// designee folds under a SEPARATE byte budget → client polls for finality.
package node

import (
	"fmt"

	"github.com/nerolabs/silt/core/chain"
	"github.com/nerolabs/silt/ports"
)

// pendingEntry is one mempool slot: the entry plus the local chain height at
// which it was first queued (FIFO order is arrival order; the height is
// observability — which submissions are aging — not an ordering key beyond it).
type pendingEntry struct {
	E  ports.Entry
	At uint64
}

// entryEncode / entryDecode ride the block-CBOR wrapper exactly as bond-reg
// submissions do (bondRegEncode), so a submitted entry travels the wire
// without a new codec and version-checks like any block payload.
func entryEncode(e ports.Entry) []byte {
	b := chain.Block{Version: chain.BlockVersion, Entries: []ports.Entry{e}}
	return chain.Encode(&b)
}

func entryDecode(raw []byte) (ports.Entry, error) {
	b, err := chain.Decode(raw)
	if err != nil || len(b.Entries) != 1 {
		return ports.Entry{}, fmt.Errorf("bad entry payload")
	}
	return b.Entries[0], nil
}

// maxMempool bounds each proposer mempool (pendingEntries by distinct root,
// pendingBondRegs by validator). The entry pool dedups by ROOT, so a remote party
// with valid publish tokens could submit distinct roots faster than the designee
// drains them and grow it without bound — a resident-memory DoS (boundedness audit
// A2). Past the cap a new arrival is REJECTED rather than admitted, which preserves
// the FIFO seniority the #441 certification pinned ("waiting cannot lose seniority")
// — dropping the oldest would let a flood evict entries that were first in line. Safe
// to reject: the #441 client path is fire-and-forget + poll-for-finality, so a
// rejected submission is re-sent by the client's retry loop (and the pool has usually
// drained by then). Far above any honest in-flight publish count. The bond pool is
// already validator-bounded (one slot per validator, replace-in-place); the cap is
// defense-in-depth against a forged-ID path.
const maxMempool = 8192

// queuePendingEntry adds a validated entry to the FIFO mempool. Dedup by root:
// a root already queued keeps its original position (resubmission cannot
// queue-jump — Addition 2's fairness depends on it). Returns whether the
// entry newly entered the queue.
func (n *Node) queuePendingEntry(e ports.Entry) bool {
	for i := range n.pendingEntries {
		if n.pendingEntries[i].E.Root == e.Root {
			return false
		}
	}
	if len(n.pendingEntries) >= maxMempool {
		// Full: reject the new arrival (preserve FIFO seniority; client re-sends).
		n.logf(ports.LogDebug, "entry mempool full: rejecting submission", "root", e.Root, "cap", maxMempool)
		return false
	}
	_, height := n.chain.Head()
	n.pendingEntries = append(n.pendingEntries, pendingEntry{E: e, At: height})
	return true
}

// SubmitEntry broadcasts a publish entry to peers so whichever eligible
// proposer makes the next block folds it in — the certified #441 client path
// (submit, never propose; poll for finality). Fire-and-forget, like
// SubmitBondRenewal: a dropped submission is re-sent by the client's retry
// loop, and inclusion is read from the chain. The entry ALSO enters this
// node's own mempool when it is itself proposer-eligible (the registry
// validator publishing through its own replica — the common daemon path).
func (n *Node) SubmitEntry(e ports.Entry, peers []ports.NodeID) {
	if n.chain == nil {
		return
	}
	if n.chain.ProposerEligible(n.id) {
		n.queuePendingEntry(e)
	}
	raw := entryEncode(e)
	for _, p := range peers {
		if p == n.id {
			continue
		}
		n.request(p, ports.Message{Kind: ports.MsgSubmitEntry, Data: raw}, func(ports.Message, error) {})
	}
}

// foldPendingEntries appends mempool entries to a block under construction —
// FIFO, re-validated against the CURRENT head (a root committed or a token
// serial spent since submit is dropped, exactly the reg queue's discipline;
// one poisoned entry never kills the block), under the SEPARATE entry byte
// budget (never the reg budget — Addition 1; at least one entry always folds
// so an oversized entry cannot stall the queue). The queue keeps only the
// still-valid entries that did not fit — the durable-queue rule (#427 B).
// Dedup within the block by root AND token serial: two mempool entries can
// each be valid against the chain yet conflict inside one block.
func (n *Node) foldPendingEntries(b *chain.Block) {
	if len(n.pendingEntries) == 0 {
		return
	}
	budget := n.cfg.MaxEntryBytesPerBlock
	used := int64(0)
	roots := make(map[ports.Hash]bool, len(b.Entries))
	serials := map[string]bool{}
	for _, e := range b.Entries {
		roots[e.Root] = true
		if e.Token != nil {
			serials[string(e.Token.Serial)] = true
		}
	}
	kept := n.pendingEntries[:0]
	for _, pe := range n.pendingEntries {
		if roots[pe.E.Root] {
			continue // already in this block (or a duplicate) — drop from the queue
		}
		if pe.E.Token != nil && serials[string(pe.E.Token.Serial)] {
			continue // its serial is spent by an earlier entry in this very block
		}
		if err := n.chain.ValidateEntry(pe.E); err != nil {
			// Stale against the current head (committed root, spent serial, …):
			// dead weight either way — drop, and say so (B5: never silently).
			n.logf(ports.LogDebug, "entry mempool: dropped at fold", "root", pe.E.Root, "err", err)
			continue
		}
		sz := int64(len(entryEncode(pe.E)))
		if budget > 0 && len(b.Entries) > 0 && used+sz > budget {
			kept = append(kept, pe) // still valid, out of budget — next block's work
			continue
		}
		b.Entries = append(b.Entries, pe.E)
		roots[pe.E.Root] = true
		if pe.E.Token != nil {
			serials[string(pe.E.Token.Serial)] = true
		}
		used += sz
	}
	n.pendingEntries = append([]pendingEntry(nil), kept...)
}

// pendingBondReg is one reg-queue slot. The queue is FIFO BY ARRIVAL — the
// same no-priority rule the #441 certification pinned for entries (Addition
// 2): with the byte budget admitting ~one plot-sized reg per block, any
// priority order (the old validator-ID sort) starves whoever sorts last for
// as long as higher-priority traffic flows (confirm run 54003f7-91159: a
// first-time maturer reg queued 22 minutes behind lower-ID renewals).
type pendingBondReg struct {
	R chain.BondReg
}

// queuePendingBondReg records a peer-submitted (or refill-modeled) bond
// registration for the next proposal to fold — one slot per validator. A
// resubmission from an already-queued validator REPLACES its bytes IN PLACE
// (the fresh nonce wins) but keeps its original FIFO position: renewing
// cannot queue-jump, and waiting cannot lose seniority.
func (n *Node) queuePendingBondReg(reg chain.BondReg) {
	vid := reg.ValidatorID()
	for i := range n.pendingBondRegs {
		if n.pendingBondRegs[i].R.ValidatorID() == vid {
			n.pendingBondRegs[i].R = reg
			return
		}
	}
	if len(n.pendingBondRegs) >= maxMempool {
		// Defense-in-depth: the pool is validator-bounded (one slot each), but a
		// forged/unbonded-ID path must not grow it without bound. Reject when full.
		n.logf(ports.LogDebug, "bond-reg mempool full: rejecting submission", "validator", vid, "cap", maxMempool)
		return
	}
	n.pendingBondRegs = append(n.pendingBondRegs, pendingBondReg{R: reg})
}
