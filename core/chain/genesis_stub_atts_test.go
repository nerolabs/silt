package chain

import (
	"reflect"
	"testing"

	"github.com/nerolabs/silt/ports"
)

// MG-C / R-CARRIER-GENESIS-DISPOSAL (delta cert
// LASTCOMMIT-CARRIER-26977a4-DELTA-CERTIFICATION-2026-09-03 §6; ROADMAP).
//
// Only the HASH-COVERED half is gated here: a genesis carrying a LastCommit carrier is
// authored, signed content and is REFUSED (ErrGenesisLastCommit). The UNSIGNED slot
// (Atts, outside the Hash() preimage — see hash_literal_pin_test.go) is deliberately NOT
// gated: stripping it broke the anchor bootstrap (the launch anchors' genesis
// attestations are what seat them into validatorsSeen on core/node), so its disposal
// (strip-all vs seat-only-verified) is RESEARCH-GATED and AppendGenesis keeps main's
// pre-carrier behaviour for it. Do not add an Atts gate here without that verdict.

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

// the strip-not-fatal probe — gate (a): the direct AppendGenesis path.
//
// 2026-09-04: the three Atts-STRIP gates that lived here were WITHDRAWN with the strip
// itself — stripping genesis Atts breaks the anchor bootstrap (four core/node tests).
// The Atts half of MG-C is research-gated; the tests are parked in the Researcher's
// question (`genesis-atts-seating`), not deleted from history. Only the LastCommit
// refusal (the hash-covered half, still ratified) remains here.

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

// the reload-survives probe — gate (c): the own-disk path. Persist
// (EncodeBlocks, the bytes chainstore writes) a genesis that acquired a stub, reload it
// into a fresh chain: no error, and the stub is stripped on the way in.
