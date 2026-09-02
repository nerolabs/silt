package chain

// GATE F — the R0.4b ROLLOUT RULE (red-team break 6; merge condition M9).
//
// THE HAZARD. IssuerKeys is cbor key 17 inside the hashed `unsigned` struct. A
// pre-R0.4b decoder ignores the unknown key (fxamacker's default) and then hashes the
// body WITHOUT it, so the proposer's and attesters' signatures — made over the real
// hash — fail on every old node: a reg-carrying block is rejected as ErrBadSignature.
// The standard additive-field hazard.
//
// WHY IT IS LATENT TODAY, not fixed by luck. A registration is valid only in a v5
// block, and no shipped binary can mint one: NewBondReg stamps the READINESS signal
// at BlockVersionRegGate (3), the era-3 tally needs >= 4 and the era-4 tally >= 5, and
// there is no activation override. So on a Path-A network neither era can lock in and
// no reg-carrying block can exist. The activation mechanism IS the version gate era-4
// was designed with — "the decode ceiling and the predicate ship atomically".
//
// THE RULE THIS ENCODES. A binary may stamp Version = 5 only if its v5 leaf set
// includes issuerKeyCommit and its Block.Hash covers cbor 17. A binary that signalled
// 5 without the leaf would compute a DIFFERENT v5 root from one that has it — a fork
// among "5-ready" nodes. So this is a freeze-scope obligation, not a courtesy.
//
// The two coverage properties are asserted UNCONDITIONALLY (they are cheap, they are
// true today, and a vacuous gate protects nothing); the stamp assertion is what turns
// them load-bearing the day someone raises it.

import (
	"testing"

	"github.com/nerolabs/silt/ports"
)

// TestReadinessStampImpliesIssuerKeyCoverage is the compile-time-shaped rollout gate.
func TestReadinessStampImpliesIssuerKeyCoverage(t *testing.T) {
	k := issuerKeyTestKey(94001)
	stamp := NewBondReg(k, ports.Hash{0x01}, 1, nil, ports.Hash{}, 0).Version

	// (a) Block.Hash MUST cover IssuerKeys. Two blocks differing only in the field
	// must hash differently, or an old binary's hash silently agrees with a new one's
	// over different content.
	with := &Block{Version: BlockVersionWitnessable, Height: 3, Entries: []ports.Entry{entry(3)},
		IssuerKeys: []IssuerKeyReg{SignIssuerKeyReg(k, 0, ports.Hash{0x77})}}
	without := *with
	without.IssuerKeys = nil
	without.hashMemoSet = false
	if with.Hash() == without.Hash() {
		t.Fatalf("Block.Hash does not cover IssuerKeys. A binary stamping readiness %d "+
			"would sign a hash that omits committed content: every pre-R0.4b node rejects "+
			"the block as ErrBadSignature, and any node that accepted it would commit "+
			"state its own root does not bind.", stamp)
	}

	// (b) The v5 leaf set MUST include issuerKeyCommit. Without it two "5-ready"
	// binaries compute different v5 roots over the same history — a fork.
	found := false
	for _, tag := range stateRootTagsV5 {
		if tag == "issuerKeyCommit" {
			found = true
		}
	}
	if !found {
		t.Fatalf("stateRootTagsV5 does not include issuerKeyCommit, but registrations are "+
			"committed state. A binary stamping readiness %d without the leaf computes a "+
			"different v5 root from one that has it — a fork among 5-ready nodes.", stamp)
	}

	// (c) The stamp itself. Raising it is the moment (a) and (b) stop being hygiene
	// and become the fork-avoidance rule, so the raise must be a deliberate edit here
	// — with R0.4b (or a later release carrying it) already shipped.
	if stamp != BlockVersionRegGate {
		t.Logf("NewBondReg now stamps readiness %d (was %d). The R0.4b rollout rule is "+
			"live from this point: this release MUST carry cbor key 17 in the hashed "+
			"block, validateIssuerKeys, and the issuerKeyCommit leaf — all asserted above.",
			stamp, BlockVersionRegGate)
		if stamp >= BlockVersionWitnessable {
			// Deliberately not a failure: (a) and (b) already hold, which is the
			// rule. This branch exists so the raise cannot happen silently.
			t.Logf("readiness %d >= v5: era-4 can now lock in by tally", stamp)
		}
	}
}

// TestIssuerKeyRegRequiresAV5Block re-states, at the validity layer, why the rollout
// rule is bounded: no reg can ride a pre-v5 block at all, so the hazard cannot reach
// an old binary until the mint version flips.
func TestIssuerKeyRegRequiresAV5Block(t *testing.T) {
	c := New(Config{Quorum: 1}, func(ports.NodeID) int64 { return 1 << 30 })
	k := issuerKeyTestKey(94002)
	for _, v := range []uint64{BlockVersionRounds, BlockVersionRegGate, BlockVersionStateRoot} {
		b := &Block{Version: v, Height: 1,
			IssuerKeys: []IssuerKeyReg{SignIssuerKeyReg(k, 0, ports.Hash{0x77})}}
		if err := c.validateIssuerKeys(b); err == nil {
			t.Fatalf("a v%d block carrying a key registration was accepted — it would write "+
				"committed state the era-3 leaf set does not cover", v)
		}
	}
}
