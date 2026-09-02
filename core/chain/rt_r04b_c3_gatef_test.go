package chain

// R0.4b C3 re-break — F9 regression gates. Inversion of the red-team probe
// rt_c3b_gatef_test.go (RT-C3B-19 / 19b).
//
// The finding: gate F's clause (c) — the readiness stamp — was a t.Logf, so raising the mint
// stamp failed nothing and, under a plain `go test` (no -v), printed nothing either. "The
// raise cannot happen silently" did not hold in CI. And the readiness TALLY is not the only
// route to v5: Config.Era4ActivationHeight activates era-4 with no readiness signalling at
// all, so the config route bypassed the rule the gate was written against.
//
// The close is in core/chain/issuerkey_rollout_gate_test.go: clause (c) is now a hard failure
// via gateFStampPin, and TestGateF_ConfigActivationRouteCarriesTheSameCoverage drives the
// config route end to end. These two gates keep the close honest — one proves the tripwire has
// teeth, the other pins the wire-level fact the rule exists for.

import (
	"testing"

	"github.com/fxamacker/cbor/v2"
	"github.com/nerolabs/silt/ports"
)

// TestGateF_StampRaiseIsAHardFailure is the teeth-proof. Clause (c) cannot be exercised by
// raising the real constant (that is the edit the gate exists to force), so it asserts the
// factored predicate directly: any stamp other than the pinned one must produce a failure
// message. Reverting clause (c) to a log reddens this.
func TestGateF_StampRaiseIsAHardFailure(t *testing.T) {
	if msg := gateFStampPin(BlockVersionRegGate); msg != "" {
		t.Fatalf("the pinned stamp must pass clause (c), got %q", msg)
	}
	for _, raised := range []uint8{BlockVersionStateRoot, BlockVersionWitnessable} {
		if gateFStampPin(raised) == "" {
			t.Fatalf("clause (c) does not fail on a raised stamp (%d). A one-line constant "+
				"change would turn the R0.4b rollout rule live with a fully GREEN tree — the "+
				"exact silence red-team F9 measured.", raised)
		}
	}
}

// TestGateF_WireCarriesIssuerKeysAndTheHashCoversThem is the wire-level fact behind the rule,
// pinned against real CBOR bytes rather than by inspection: an R0.4b proposer really does put
// key 17 on the wire, and a decoder that drops it (no strict-unknown-field mode is configured
// anywhere in the tree) re-derives a DIFFERENT hash. That divergence is the mixed-binary fork
// the rollout rule exists to prevent, and it is also the proof that the hash covers the field.
func TestGateF_WireCarriesIssuerKeysAndTheHashCoversThem(t *testing.T) {
	k := key(94100)
	b := &Block{Version: BlockVersionWitnessable, Height: 3, Entries: []ports.Entry{entry(3)},
		IssuerKeys: []IssuerKeyReg{SignIssuerKeyReg(k, 0, ports.Hash{0x77})}}
	Sign(b, k)
	newHash := b.Hash()

	wire, err := cbor.Marshal(b)
	if err != nil {
		t.Fatal(err)
	}
	var raw map[uint64]cbor.RawMessage
	if err := cbor.Unmarshal(wire, &raw); err != nil {
		t.Fatal(err)
	}
	if _, ok := raw[17]; !ok {
		t.Fatalf("the wire block does not carry cbor key 17 — a registration would ride " +
			"uncommitted, outside the signed hash")
	}
	var decoded Block
	if err := cbor.Unmarshal(wire, &decoded); err != nil {
		t.Fatalf("decode: %v", err)
	}
	rebuilt := decoded
	rebuilt.IssuerKeys = nil
	rebuilt.hashMemoSet = false
	if rebuilt.Hash() == newHash {
		t.Fatalf("a decoder that drops IssuerKeys derives the SAME hash (%x) — Block.Hash "+
			"does not cover key 17, so a registration is committed content no signature binds",
			newHash[:8])
	}
}
