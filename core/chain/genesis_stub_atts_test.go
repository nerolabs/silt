package chain

import (
	"reflect"
	"testing"

	"github.com/nerolabs/silt/ports"
)

// MG-C / R-CARRIER-GENESIS-DISPOSAL (delta cert
// LASTCOMMIT-CARRIER-26977a4-DELTA-CERTIFICATION-2026-09-03 §6; ROADMAP).
//
// Block.Atts is OUTSIDE the Hash() preimage (see hash_literal_pin_test.go). A serving
// peer can therefore append an unsigned stub attestation to a genesis it relays: the
// hash is byte-identical, the proposer signature still verifies, and the
// genesis-divergence check passes. Whatever AppendGenesis does with that stub is done
// at zero cost to the attacker, on two live paths — Reload (own disk, chain.go:3160)
// and Reconcile (peer-supplied fork, chain.go:4012).
//
// The rule the cert certifies: STRIP an unsigned slot (Atts), REFUSE a hash-covered one
// (LastCommit). "Strip" means nil b.Atts BEFORE c.apply(b): the sub-v5 seating loop
// (chain.go:3471) runs over b.Atts and would otherwise pre-seat validatorsSeen from a
// signature nobody made.
//
// On main (b328268) AppendGenesis neither refuses nor strips: the stub reaches apply()
// and pre-seats. On the carrier branch it refuses (ErrGenesisAtts) — the free denial
// lever. Both are RED here; the Builder's split fix makes them GREEN.

// stubAttFor is the attacker's stub: the REAL public key of a QUALIFIED validator that
// signs nothing else in the test, with a 64-zero-byte signature nobody produced. A
// qualified key makes pre-seating observable (validatorsSeen[id] is written iff
// attesterQualified(id)); a key that signs nothing else means any seating of it is the
// stub's doing, on every path (the world's own vals legitimately attest fork blocks).
func stubAttFor(w *world) (Attestation, ports.NodeID) {
	k := key(31000)
	w.reps[idOf(k)] = 1000 // qualified by reputation, exactly like w.vals
	return Attestation{PubKey: pubOf(k), Sig: make([]byte, 64)}, idOf(k)
}

// signedGenesisWithStub builds a proposer-signed genesis, then appends the stub AFTER
// signing (the in-transit mutation), asserting the premise that the hash is unchanged.
func signedGenesisWithStub(t *testing.T, w *world) (Block, ports.NodeID) {
	t.Helper()
	g := Block{Version: 1, Height: 0, Entries: []ports.Entry{entry(0)}}
	Sign(&g, w.prop)
	pre := g.Hash()
	stub, id := stubAttFor(w)
	g.Atts = append(g.Atts, stub)
	g.hashMemoSet = false
	if g.Hash() != pre {
		t.Fatal("premise: Atts must be outside the Hash() preimage for this attack to exist — if that changed, this gate's premise is gone (re-derive from hash_literal_pin_test.go)")
	}
	return g, id
}

func assertGenesisStripped(t *testing.T, c *Chain, victim ports.NodeID, path string) {
	t.Helper()
	if c.Len() == 0 {
		t.Fatalf("%s: no genesis committed", path)
	}
	if n := len(c.blocks[0].Atts); n != 0 {
		t.Errorf("%s: committed genesis carries %d Atts — the unsigned stub was NOT STRIPPED before apply (MG-C: strip, do not refuse, do not keep)", path, n)
	}
	if c.validatorsSeen[victim] {
		t.Errorf("%s: validatorsSeen was PRE-SEATED from an unsigned stub attestation — the maturity metric counted a signature nobody made", path)
	}
}

// TestGenesisStubAttsAreStrippedNotFatal — gate (a): the direct AppendGenesis path.
func TestGenesisStubAttsAreStrippedNotFatal(t *testing.T) {
	w := newWorld(DefaultConfig())
	g, victim := signedGenesisWithStub(t, w)
	if err := w.c.AppendGenesis(g); err != nil {
		t.Fatalf("AppendGenesis REFUSED a genesis whose only defect is an unsigned stub Att appended after signing: %v — REFUSE on a non-hash-covered slot is a free denial lever (MG-C)", err)
	}
	assertGenesisStripped(t, w.c, victim, "AppendGenesis")
}

// TestGenesisLastCommitIsRefused — gate (b): the hash-covered slot MUST be refused.
// Written to compile on main, where Block has no LastCommit field: it skips with a
// named reason until the carrier lands, then asserts refusal.
func TestGenesisLastCommitIsRefused(t *testing.T) {
	f, ok := reflect.TypeOf(Block{}).FieldByName("LastCommit")
	if !ok {
		t.Skip("Block has no LastCommit field on this tree (pre-carrier); this gate arms when the carrier lands")
	}
	w := newWorld(DefaultConfig())
	g := Block{Version: 1, Height: 0, Entries: []ports.Entry{entry(0)}}
	lc := reflect.MakeSlice(f.Type, 1, 1)
	stub, _ := stubAttFor(w)
	lc.Index(0).Set(reflect.ValueOf(stub))
	reflect.ValueOf(&g).Elem().FieldByName("LastCommit").Set(lc)
	Sign(&g, w.prop) // LastCommit is hash-covered: the proposer AUTHORED it
	if err := w.c.AppendGenesis(g); err == nil {
		t.Fatal("a genesis carrying LastCommit was ACCEPTED — the hash-covered slot must be REFUSED (a genesis has no parent to carry a commit for)")
	}
	if w.c.Len() != 0 {
		t.Fatal("a refused genesis must leave the chain empty")
	}
	// And the same LastCommit appended AFTER signing must fail the signature, because
	// it is inside the preimage — the property that makes REFUSE the right disposal.
	g2 := Block{Version: 1, Height: 0, Entries: []ports.Entry{entry(0)}}
	Sign(&g2, w.prop)
	reflect.ValueOf(&g2).Elem().FieldByName("LastCommit").Set(lc)
	g2.hashMemoSet = false
	if err := w.c.AppendGenesis(g2); err == nil {
		t.Fatal("LastCommit appended after signing was ACCEPTED — it is not hash-covered on this tree")
	}
}

// TestReloadSurvivesAGenesisWithAStubAtt — gate (c): the own-disk path. Persist
// (EncodeBlocks, the bytes chainstore writes) a genesis that acquired a stub, reload it
// into a fresh chain: no error, and the stub is stripped on the way in.
func TestReloadSurvivesAGenesisWithAStubAtt(t *testing.T) {
	w := newWorld(DefaultConfig())
	g, victim := signedGenesisWithStub(t, w)
	blocks, err := DecodeBlocks(EncodeBlocks([]Block{g}))
	if err != nil {
		t.Fatal(err)
	}
	if len(blocks[0].Atts) != 1 {
		t.Fatal("premise: the stub must survive the persisted encoding (it is on the wire)")
	}
	fresh := New(DefaultConfig(), func(n ports.NodeID) int64 { return w.reps[n] })
	n, err := fresh.Reload(blocks)
	if err != nil {
		t.Fatalf("Reload WEDGED on a persisted genesis carrying one unsigned stub Att (applied %d): %v — a node whose disk genesis ever acquired a stub never restarts (MG-C)", n, err)
	}
	if n != 1 {
		t.Fatalf("Reload applied %d blocks, want 1", n)
	}
	assertGenesisStripped(t, fresh, victim, "Reload")
}

// TestGenesisStubAttSurvivesForkAdopt — the second live path the cert names: Reconcile
// calls tmp.AppendGenesis(fork[0]) on a PEER-SUPPLIED fork. A heavier valid fork whose
// relayed genesis carries a stub must still be adopted, with the stub stripped.
func TestGenesisStubAttSurvivesForkAdopt(t *testing.T) {
	w := newWorld(DefaultConfig())
	g := w.genesis() // our own clean genesis, committed
	light := w.forkBlock(g.Hash(), entry(1), 3)
	if err := w.c.Append(*light); err != nil {
		t.Fatal(err)
	}
	relayed := *g
	stub, victim := stubAttFor(w)
	relayed.Atts = []Attestation{stub}
	relayed.hashMemoSet = false
	if relayed.Hash() != g.Hash() {
		t.Fatal("premise: the stub must not move the genesis hash")
	}
	heavy := w.forkBlock(g.Hash(), entry(2), 4)
	adopted, err := w.c.Reconcile([]Block{relayed, *heavy})
	if err != nil {
		t.Fatalf("Reconcile REFUSED a heavier valid fork because its relayed genesis carries one unsigned stub Att: %v — fork-adopt is deniable by any serving peer at zero cost (MG-C)", err)
	}
	if !adopted {
		t.Fatal("a strictly heavier fork must be adopted")
	}
	assertGenesisStripped(t, w.c, victim, "Reconcile")
	if _, ok := w.c.LookupRoot(entry(2).Root); !ok {
		t.Error("the adopted fork's entry must be present")
	}
}
