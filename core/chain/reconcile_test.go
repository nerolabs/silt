package chain

import (
	"testing"

	"github.com/nerolabs/silt/ports"
)

// genesisBlock builds and appends a declared genesis, returning it so forks
// can branch from it.
func (w *world) genesis() *Block {
	g := &Block{Version: 1, Height: 0, Entries: []ports.Entry{entry(0)}}
	Sign(g, w.prop)
	if err := w.c.AppendGenesis(*g); err != nil {
		panic(err)
	}
	return g
}

// blockAt builds a block at height h on top of prev with the given entry,
// attested by the first `nAtt` validators. The attestations are VALIDITY inputs
// only: fork-choice is height → head-hash (O3 Direction T) and reads no
// certificate, so nAtt never ranks a fork.
func (w *world) blockAt(prev ports.Hash, h uint64, e ports.Entry, nAtt int) *Block {
	b := &Block{Version: 1, Height: h, Prev: prev, Entries: []ports.Entry{e}}
	Sign(b, w.prop)
	for _, v := range w.vals[:nAtt] {
		b.Atts = append(b.Atts, Attest(b, v))
	}
	return b
}

// forkBlock builds a height-1 block on top of prev (see blockAt).
func (w *world) forkBlock(prev ports.Hash, e ports.Entry, nAtt int) *Block {
	return w.blockAt(prev, 1, e, nAtt)
}

// The heal: a replica committed to a shorter fork adopts a TALLER valid one that
// shares its genesis, and rolls its state off the abandoned fork. (Was
// TestReconcileAdoptsHeavierFork, which ranked the forks by attester count at
// equal height; under O3 Direction T fork-choice is height → head-hash, and the
// re-grounded fixture makes the adopted fork strictly taller — never an
// equal-height head-hash coin flip.)
func TestReconcileAdoptsTallerFork(t *testing.T) {
	w := newWorld(DefaultConfig())
	g := w.genesis()

	short := w.forkBlock(g.Hash(), entry(1), 4)      // height 1, 4 attesters
	tall1 := w.forkBlock(g.Hash(), entry(2), 3)      // height 1, 3 attesters
	tall2 := w.blockAt(tall1.Hash(), 2, entry(3), 3) // height 2 — taller

	if err := w.c.Append(*short); err != nil {
		t.Fatalf("append short fork: %v", err)
	}
	if _, ok := w.c.LookupRoot(entry(1).Root); !ok {
		t.Fatal("setup: short fork's entry should be present")
	}

	adopted, err := w.c.Reconcile([]Block{*g, *tall1, *tall2})
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if !adopted {
		t.Fatal("a strictly taller fork must be adopted (height is the primary fork-choice term)")
	}
	if _, ok := w.c.LookupRoot(entry(3).Root); !ok {
		t.Fatal("taller fork's entry should now be present")
	}
	if _, ok := w.c.LookupRoot(entry(1).Root); ok {
		t.Fatal("the abandoned fork's entry must be gone after the reorg")
	}
	if _, h := w.c.Head(); h != 3 {
		t.Fatalf("head after reorg: Head()==%d, want 3 (block-height 2)", h)
	}
}

// A SHORTER fork is NOT adopted, however many attesters stand behind it — we
// don't thrash off a taller committed history. (Was TestReconcileRejectsLighterFork.)
func TestReconcileRejectsShorterFork(t *testing.T) {
	w := newWorld(DefaultConfig())
	g := w.genesis()

	tall1 := w.forkBlock(g.Hash(), entry(1), 3)
	tall2 := w.blockAt(tall1.Hash(), 2, entry(2), 3) // committed: height 2
	short := w.forkBlock(g.Hash(), entry(3), 4)      // height 1, MORE attesters

	for _, b := range []*Block{tall1, tall2} {
		if err := w.c.Append(*b); err != nil {
			t.Fatalf("append committed block %d: %v", b.Height, err)
		}
	}
	adopted, err := w.c.Reconcile([]Block{*g, *short})
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if adopted {
		t.Fatal("a shorter fork must not displace a taller committed history (attester count is not a fork-choice input)")
	}
	if _, ok := w.c.LookupRoot(entry(2).Root); !ok {
		t.Fatal("the taller committed entry must remain")
	}
}

// Fork-choice must be deterministic on a height tie: every honest replica
// breaks the tie the same way (lower head hash), so the network converges
// rather than splitting on which equal fork to keep.
func TestReconcileTieBreakDeterministic(t *testing.T) {
	// Two equal-height forks; whichever has the lower head hash must win from
	// BOTH starting points.
	mkForks := func() (g, a, b *Block) {
		w := newWorld(DefaultConfig())
		g = w.genesis()
		a = w.forkBlock(g.Hash(), entry(1), 4)
		b = w.forkBlock(g.Hash(), entry(2), 4)
		return
	}
	_, a, b := mkForks()
	ah, bh := a.Hash(), b.Hash()
	lo, hi := a, b
	if bytesLess(bh[:], ah[:]) {
		lo, hi = b, a
	}

	// A replica on the HIGHER-hash fork must adopt the lower-hash one.
	w1 := newWorld(DefaultConfig())
	g1 := w1.genesis()
	if err := w1.c.Append(*hi); err != nil {
		t.Fatalf("append hi: %v", err)
	}
	if adopted, err := w1.c.Reconcile([]Block{*g1, *lo}); err != nil || !adopted {
		t.Fatalf("equal-height lower-hash fork should win the tiebreak (adopted=%v err=%v)", adopted, err)
	}

	// A replica already on the LOWER-hash fork must NOT switch.
	w2 := newWorld(DefaultConfig())
	g2 := w2.genesis()
	if err := w2.c.Append(*lo); err != nil {
		t.Fatalf("append lo: %v", err)
	}
	if adopted, _ := w2.c.Reconcile([]Block{*g2, *hi}); adopted {
		t.Fatal("must not switch off the lower-hash fork to an equal higher-hash one")
	}
}

// A fork that does not share our genesis is refused outright — a peer cannot
// swap in a heavier FOREIGN history.
func TestReconcileRefusesForeignGenesis(t *testing.T) {
	w := newWorld(DefaultConfig())
	g := w.genesis()
	if err := w.c.Append(*w.forkBlock(g.Hash(), entry(1), 4)); err != nil {
		t.Fatalf("append: %v", err)
	}

	// A different genesis (different entry) + a heavy block on it.
	other := newWorld(DefaultConfig())
	og := &Block{Version: 1, Height: 0, Entries: []ports.Entry{entry(9)}}
	Sign(og, other.prop)
	oheavy := other.forkBlock(og.Hash(), entry(8), 4)

	adopted, err := w.c.Reconcile([]Block{*og, *oheavy})
	if err != ErrForeignGenesis {
		t.Fatalf("expected ErrForeignGenesis, got adopted=%v err=%v", adopted, err)
	}
}

// A heavier-LOOKING but INVALID fork (a block without a real quorum) is
// re-validated end to end and rejected — a lying peer cannot feed us garbage.
func TestReconcileRejectsInvalidFork(t *testing.T) {
	w := newWorld(DefaultConfig())
	g := w.genesis()
	if err := w.c.Append(*w.forkBlock(g.Hash(), entry(1), 3)); err != nil {
		t.Fatalf("append: %v", err)
	}

	// A fork block at height 1 with only ONE attestation — below Quorum=3.
	bad := w.forkBlock(g.Hash(), entry(2), 1)

	adopted, err := w.c.Reconcile([]Block{*g, *bad})
	if adopted || err == nil {
		t.Fatalf("an under-quorum fork must be rejected, got adopted=%v err=%v", adopted, err)
	}
	if _, ok := w.c.LookupRoot(entry(1).Root); !ok {
		t.Fatal("our valid chain must survive a rejected invalid fork")
	}
}
