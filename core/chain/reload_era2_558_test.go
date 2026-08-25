package chain

import (
	"testing"

	"github.com/nerolabs/silt/ports"
)

// #558 — the DEEP a434494-deep genesis-fallback, made deterministic.
//
// FIELD OBSERVATION: val-d, OOM-killed at h83 and restarted, logged
// `chain replay: chain: bad signature: attester d4f5ec0d…` and silently fell
// back to genesis — while its persisted chain.cbor was intact (chain-status
// read it at h83). Behind the swarm's prune horizon, peer catch-up could no
// longer mask the fallback, and the node was stranded (#559) with its frozen
// seat still counted (#560).
//
// ROOT CAUSE (not a torn write — chainstore.Save is atomic): validateStructural,
// the Reload path, verified attester signatures over the BARE block hash — the
// era-1 form. Era-2 (#432) attestations sign the domain-separated
// consensusSigBytes(phase, round, hash), so replay of ANY era-2 chain has
// always failed at its first non-genesis block. The defect was invisible
// before the retention prune: a restarted node silently re-fetched its whole
// chain from peers (an expensive full Reconcile per restart — a hidden #555
// load source); once peers pruned below the horizon, the mask fell away.
//
// THE ORACLE: a committed era-2 history — including a payload-pruned block,
// the at-depth shape — must Reload into a fresh replica through the exact
// wire/disk representation (EncodeBlocks/DecodeBlocks, what chainstore
// persists). RED on the pre-fix code at block 1 with ErrBadSignature; GREEN
// with validateStructural using the shared era-aware verifyAtt.
func TestReload_Era2AndPrunedBlocksReplay_558(t *testing.T) {
	c, keys, g := roundsWorld(t)

	// Height 1: a committed era-2 block carrying a bond reg (so the pruned
	// variant genuinely drops a heavy Answer, not a no-op copy).
	b1 := &Block{Version: BlockVersionRounds, Height: 1, Prev: g.Hash(), Entries: []ports.Entry{entry(9)}}
	b1.BondRegs = append(b1.BondRegs, bondReg(keys[1], twoMiB, g.Hash()))
	Sign(b1, keys[0])
	b1.PrepareQC = append(b1.PrepareQC, AttestAt(b1, keys[0], 0, PhasePrepare))
	for _, k := range keys[1:] {
		b1.PrepareQC = append(b1.PrepareQC, AttestAt(b1, k, 0, PhasePrepare))
	}
	b1.Atts = append(b1.Atts, AttestAt(b1, keys[0], 0, PhasePrecommit))
	for _, k := range keys[1:] {
		b1.Atts = append(b1.Atts, AttestAt(b1, k, 0, PhasePrecommit))
	}
	if err := c.Append(*b1); err != nil {
		t.Fatalf("live commit of the era-2 block: %v", err)
	}

	// The persisted representation, byte-faithful to chainstore: encode, decode.
	persisted, err := DecodeBlocks(EncodeBlocks(c.Blocks(0)))
	if err != nil {
		t.Fatalf("roundtrip: %v", err)
	}

	// A fresh replica with the same config but an EMPTY history (the restart).
	freshWorld := func() *Chain {
		anchors := map[ports.NodeID]bool{}
		for _, k := range keys {
			anchors[idOf(k)] = true
		}
		fc := New(Config{Quorum: 1, MinBond: 1 << 20, ByzantineQuorum: true,
			Anchors: anchors, AnchorQuorum: 1, MatureValidators: 99},
			func(ports.NodeID) int64 { return 0 })
		fc.SetBondVerifier(objectiveVerify)
		return fc
	}

	fresh := freshWorld()
	n, err := fresh.Reload(persisted)
	if err != nil {
		t.Fatalf("#558 REPRODUCED: Reload of a committed era-2 history failed at block %d: %v — validateStructural is verifying era-2 attestations with the era-1 bare-hash arithmetic, so every restart silently falls to a %d-block prefix and (behind the prune horizon) strands the validator. Use the shared era-aware verifyAtt.", n, err, n)
	}
	if n != len(persisted) {
		t.Fatalf("Reload restored %d of %d blocks", n, len(persisted))
	}
	if _, h := fresh.Head(); h != 2 {
		t.Fatalf("restored head: next=%d, want 2", h)
	}

	// The at-depth shape: the reg block payload-pruned (Answer dropped, Pruned
	// hash carried) — exactly what a below-horizon block looks like on disk.
	prunedSet := append([]Block(nil), persisted...)
	prunedSet[1] = prunedSet[1].Prune()
	prunedSet, err = DecodeBlocks(EncodeBlocks(prunedSet))
	if err != nil {
		t.Fatalf("pruned roundtrip: %v", err)
	}
	fresh2 := freshWorld()
	n, err = fresh2.Reload(prunedSet)
	if err != nil || n != len(prunedSet) {
		t.Fatalf("Reload of the PRUNED era-2 history: restored %d of %d, err=%v — a validator must be able to replay its own payload-pruned store (#558)", n, len(prunedSet), err)
	}
}
