package tcpnet

// R2.2 rows 8-9 — the two work-gossip fields must survive real CBOR.
//
// The #65 class: a ports.Message field with no wireMsg mapping is SILENTLY DROPPED over
// TCP and the feature works only in the in-process sim. Here that failure is especially
// quiet, because a dropped work figure decodes as 0 and 0 is a legal value — every
// remote peer would look like a node that has served nothing, the serve-Gini would read
// 0 (perfect equality) over a captured network, and nothing would error.

import (
	"testing"

	"github.com/fxamacker/cbor/v2"

	"github.com/nerolabs/silt/ports"
)

func TestR22WorkGossipSurvivesWire(t *testing.T) {
	want := ports.Message{Kind: ports.MsgFindNode, RID: 4,
		CapUsed: 1 << 20, CapTotal: 64 << 30, ServedBytes: 123_456_789, RepairsDone: 42}
	b, err := encMode.Marshal(envelope{Msg: toWire(want)})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var env envelope
	if err := cbor.Unmarshal(b, &env); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	got := fromWire(env.Msg)
	if got.ServedBytes != want.ServedBytes || got.RepairsDone != want.RepairsDone {
		t.Fatalf("work gossip lost over the wire: got served %d repairs %d, want %d and %d. A dropped field decodes as 0, which is a LEGAL value here — every peer would read as having done no work and the Ginis would report perfect equality over a captured network",
			got.ServedBytes, got.RepairsDone, want.ServedBytes, want.RepairsDone)
	}
	if got.CapTotal != want.CapTotal {
		t.Fatalf("CapTotal = %d, want %d — the work fields must ride BESIDE the pledge, not instead of it", got.CapTotal, want.CapTotal)
	}
}

// TestR22WorkGossipIsAdditiveForAnOldPeer: the two new slots are omitempty and a decoder
// that does not know keys 29/30 skips them, so a node that has served nothing adds zero
// bytes to every packet and an un-upgraded peer keeps parsing. This is DHT gossip, not a
// frozen consensus format, so additive is the correct posture — but it has to be true.
// TestR22WorkGossipIsAdditiveForAnOldPeer is ALSO the wire half of the research
// certification's condition M-1 (C3-GOSSIP-DISCLOSURE-vs-D-UI-PRIVACY-FLAG-2026-09-09):
// "with privacy on, an exact scan of the raw CBOR map finds neither key 29 nor key 30 on a
// node with non-zero work."
//
// THE COMPOSITION, because it takes two gates and neither is complete alone. HERE: an exact
// scan of the raw CBOR map proves that a ports.Message with both work fields at zero emits
// NEITHER key 29 nor key 30, so its frame is byte-identical to a pre-R2.2 node's. THERE:
// core/node's TestR22M1WorkCountersAreGossipedOnlyWhenTheOperatorPublishesThem proves that
// Node.send, on the compiled -privacy=on posture, stamps EXACTLY ZERO into both fields even
// when the ledger holds 987,654,321 served bytes. Composed: withholding posture => zero
// stamped => no key on the wire. The two halves live apart because toWire/encMode are
// package-private here and the node is package-private there; splitting is the honest cost
// of not exporting wire internals for a test.
//
// The scan is EXACT-KEY and not a frame-length compare on purpose. The first version of this
// assertion compared len(quiet) < len(busy), and it stayed GREEN under the controlled revert
// that deletes omitempty — a non-omitempty key grows both frames equally and the inequality
// survives. The certification names that vacuity by reference.
func TestR22WorkGossipIsAdditiveForAnOldPeer(t *testing.T) {
	// EXACTLY what Node.send produces on the withholding posture: the capacity pledge
	// stamped, both work fields left at zero.
	quiet, err := encMode.Marshal(envelope{Msg: toWire(ports.Message{Kind: ports.MsgFindNode, CapTotal: 1 << 30})})
	if err != nil {
		t.Fatal(err)
	}
	busy, err := encMode.Marshal(envelope{Msg: toWire(ports.Message{Kind: ports.MsgFindNode, CapTotal: 1 << 30, ServedBytes: 5, RepairsDone: 1})})
	if err != nil {
		t.Fatal(err)
	}
	// EXACT: the two keys must be ABSENT from a frame whose sender has done no work.
	// A length comparison does NOT test this — a non-omitempty key 29 grows BOTH frames
	// equally and the inequality still holds, which is how the first version of this
	// assertion passed under the controlled revert that deletes omitempty.
	for name, frame := range map[string][]byte{"quiet": quiet, "busy": busy} {
		var top map[int]cbor.RawMessage
		if err := cbor.Unmarshal(frame, &top); err != nil {
			t.Fatal(err)
		}
		var msg map[int]cbor.RawMessage
		if err := cbor.Unmarshal(top[4], &msg); err != nil {
			t.Fatal(err)
		}
		_, has29 := msg[29]
		_, has30 := msg[30]
		if name == "quiet" && (has29 || has30) {
			t.Fatalf("a sender that has done no work still emits keys 29/%v and 30/%v. Not omitempty: every packet on the network pays for a field most senders have nothing to say in", has29, has30)
		}
		if name == "busy" && (!has29 || !has30) {
			t.Fatalf("a sender WITH work emitted keys 29/%v 30/%v — the check above is vacuous unless a real figure actually rides", has29, has30)
		}
	}
	// An "old" decoder: a struct that predates keys 29/30 entirely.
	var old struct {
		Msg struct {
			Kind     uint8 `cbor:"1,keyasint"`
			CapTotal int64 `cbor:"14,keyasint,omitempty"`
		} `cbor:"4,keyasint"`
	}
	if err := cbor.Unmarshal(busy, &old); err != nil {
		t.Fatalf("a decoder that does not know the new keys FAILED on a new frame: %v — that is a network split, not an upgrade", err)
	}
	if old.Msg.CapTotal != 1<<30 {
		t.Fatalf("the old decoder read CapTotal %d, want %d", old.Msg.CapTotal, int64(1)<<30)
	}
}
