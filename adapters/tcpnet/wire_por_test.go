package tcpnet

import (
	"bytes"
	"testing"

	"github.com/fxamacker/cbor/v2"

	"github.com/nerolabs/silt/ports"
)

// TestPorFieldsSurviveWire is the-class guard: the wire codec is
// hand-rolled (toWire/fromWire map field-by-field), so a new field is
// SILENTLY DROPPED over real TCP until it is added there — a unit test on
// the port struct alone would never notice. This round-trips a message
// carrying the full PoR surface (challenge + inclusion proof + the opened
// leaves and their paths) through encode → CBOR → decode and asserts every field
// arrives intact, so an audit that depends on them can't fail invisibly in
// the field.
func TestPorFieldsSurviveWire(t *testing.T) {
	// A challenge (auditor → prover).
	challenge := ports.Message{
		Kind:     ports.MsgChallenge,
		RID:      7,
		ChunkID:  ports.ChunkID{0xab, 0xcd},
		PorSeed:  bytes.Repeat([]byte{0x5a}, 32),
		PorCount: 8,
	}
	// A reply (prover → auditor): the shard's inclusion proof with the leaf
	// width it was committed at, plus the opened leaves and their paths.
	reply := ports.Message{
		Kind:  ports.MsgChallengeReply,
		RID:   7,
		Found: true,
		Proof: &ports.StorageProof{
			Root:      ports.Hash{0x11},
			Index:     2,
			Total:     6,
			Path:      []ports.Hash{{0x22}, {0x33}},
			Column:    3,
			LeafBytes: 128,
		},
		PorOpen:   [][]byte{bytes.Repeat([]byte{3}, 128), bytes.Repeat([]byte{4}, 128)},
		PorPaths:  [][]byte{bytes.Repeat([]byte{5}, 64), bytes.Repeat([]byte{6}, 64)},
		PorBlocks: 2,
	}

	for _, want := range []ports.Message{challenge, reply} {
		b, err := encMode.Marshal(envelope{Msg: toWire(want)})
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		var env envelope
		if err := cbor.Unmarshal(b, &env); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		got := fromWire(env.Msg)

		if !bytes.Equal(got.PorSeed, want.PorSeed) {
			t.Errorf("PorSeed lost over the wire: got %x want %x", got.PorSeed, want.PorSeed)
		}
		if got.PorCount != want.PorCount {
			t.Errorf("PorCount lost: got %d want %d", got.PorCount, want.PorCount)
		}
		if got.PorBlocks != want.PorBlocks {
			t.Errorf("PorBlocks lost: got %d want %d", got.PorBlocks, want.PorBlocks)
		}
		if !equalChunks(got.PorOpen, want.PorOpen) {
			t.Errorf("PorOpen lost: got %v want %v", got.PorOpen, want.PorOpen)
		}
		if !equalChunks(got.PorPaths, want.PorPaths) {
			t.Errorf("PorPaths lost: got %v want %v", got.PorPaths, want.PorPaths)
		}
		if want.Proof != nil {
			if got.Proof == nil {
				t.Fatal("Proof dropped entirely over the wire")
			}
			if got.Proof.LeafBytes != want.Proof.LeafBytes {
				t.Errorf("Proof.LeafBytes lost: got %d want %d", got.Proof.LeafBytes, want.Proof.LeafBytes)
			}
		}
	}
}

// TestCreditSurvivesWire is the same-class guard for the prepaid publish credit
// (F4 / D3 fee decoupling): the Credit riding a MsgTokenRequest was a port-struct field
// with no wire mapping, so it was SILENTLY DROPPED over real TCP — the fee-decoupling
// and any ephemeral/credit-paid withdrawal only worked in the in-process sim. This
// round-trips a credit-bearing request and asserts it arrives intact.
func TestCreditSurvivesWire(t *testing.T) {
	want := ports.Message{
		Kind:   ports.MsgTokenRequest,
		RID:    9,
		Data:   bytes.Repeat([]byte{0xaa}, 16), // the blinded serial
		Credit: &ports.PublishCredit{Serial: bytes.Repeat([]byte{0xbb}, 24), Sig: bytes.Repeat([]byte{0xcc}, 32)},
	}
	b, err := encMode.Marshal(envelope{Msg: toWire(want)})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var env envelope
	if err := cbor.Unmarshal(b, &env); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	got := fromWire(env.Msg)
	if got.Credit == nil {
		t.Fatal("Credit dropped entirely over the wire")
	}
	if !bytes.Equal(got.Credit.Serial, want.Credit.Serial) {
		t.Errorf("Credit.Serial lost: got %x want %x", got.Credit.Serial, want.Credit.Serial)
	}
	if !bytes.Equal(got.Credit.Sig, want.Credit.Sig) {
		t.Errorf("Credit.Sig lost: got %x want %x", got.Credit.Sig, want.Credit.Sig)
	}
}

func equalChunks(a, b [][]byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !bytes.Equal(a[i], b[i]) {
			return false
		}
	}
	return true
}
