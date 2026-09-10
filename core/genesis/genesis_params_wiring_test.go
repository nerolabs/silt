package genesis_test

import (
	"encoding/hex"
	"reflect"
	"testing"

	"github.com/nerolabs/silt/adapters/memstore"
	"github.com/nerolabs/silt/core/chain"
	"github.com/nerolabs/silt/core/genesis"
	"github.com/nerolabs/silt/ports"
)

// THE WIRING PIN — owner call F. core/genesis had NO pin at all, and that is exactly how the
// defect this closes shipped: chain.ConsensusParams declared 17 fields at cbor key 20, five
// G-CFGBIND gates went green over it, and NOTHING populated it on any production path. Every one
// of those five gates hand-constructed `chain.Block{... Params: &p}` in its own fixture.
//
// A GATE THAT CONSTRUCTS THE EXACT STATE WHOSE PRODUCTION ABSENCE IS THE DEFECT CANNOT DETECT
// THAT DEFECT. So these gates never build a Block literal. They call genesis.Build — the function
// the daemon calls — and read what comes back.
//
// The instructive contrast is inside the SAME commit that shipped the inert half: (d-3) was wired
// end to end (setD3Digests -> PopulateEra4Roots -> the two chainrole call sites) while the
// genesis-config bind was not, and no gate distinguished the live half from the dead one.
//
// G-CFGBIND-6 is the runtime half (below). The wiring on the DAEMON path — that Build is called
// with real params rather than nil, and that CheckConsensusParams has its production caller — is
// a source claim and lives with the source it reads: G-CFGBIND-7/8 in cmd/silt.

// G-CFGBIND-6 — genesis.Build CARRIES the params it is given, and the genesis HASH COVERS them.
//
// The three assertions are deliberately separate claims:
//
//	(a) Build carries params through to Block.Params at all;
//	(b) the block hash MOVES with the params, so a divergently-configured node computes a
//	    different height-0 identity and Reconcile refuses it — the whole mechanism;
//	(c) the params round-trip by VALUE, so the committed content is the config, not a stub.
//
// (a) alone is what a reader-only pin would check, and it would stay green if Params were carried
// into a field the hash preimage does not fold in. (b) is the one that would go red on that.
func TestG_CFGBIND_6_BuildCarriesAndHashCoversTheParams(t *testing.T) {
	p := representativeParams()

	withParams, _, _, err := genesis.Build(memstore.New(), &p)
	if err != nil {
		t.Fatal(err)
	}
	paramless, _, _, err := genesis.Build(memstore.New(), nil)
	if err != nil {
		t.Fatal(err)
	}

	// (a) carried at all.
	if withParams.Params == nil {
		t.Fatal("WIRING BROKEN: genesis.Build was given consensus params and returned a block with Params == nil. " +
			"A network minted by this binary commits NO configuration at height 0, so every consensus-critical flag " +
			"is once again a local knob two honest operators can differ on (canon rule 8).")
	}

	// (b) the hash MOVES with them. Without this the commitment is decoration: a joining node
	// compares genesis hashes, and if the params sit outside the preimage every configuration
	// produces the same hash and every divergent node joins happily.
	if withParams.Hash() == paramless.Hash() {
		t.Fatal("THE COMMITMENT IS DECORATION: a genesis carrying consensus params hashes IDENTICALLY to one " +
			"carrying none, so height-0 identity does not depend on the config. Reconcile compares genesis " +
			"hashes, so a node with a divergent -min-bond/-quorum/-bond-label-k would join anyway and reach " +
			"different validity verdicts on the same block (I1). Params must be folded into Block.bodyHash().")
	}

	// (c) by value, not a stub.
	if !reflect.DeepEqual(*withParams.Params, p) {
		t.Fatalf("genesis.Build altered the committed params.\n  got:  %+v\n  want: %+v\n"+
			"The refusal is DIAGNOSABLE only because values are committed rather than a digest; a mutated "+
			"projection makes the diff lie about which flag differs.", *withParams.Params, p)
	}

	// And every field must actually be non-zero in the fixture, or (b) and (c) are being carried
	// by a handful of fields while the rest ride along untested. This is the value-equality
	// blind spot: a wire scan is blind to any field its fixture leaves at ZERO.
	v := reflect.ValueOf(p)
	for i := 0; i < v.NumField(); i++ {
		if v.Field(i).IsZero() {
			t.Fatalf("the fixture leaves ConsensusParams.%s at its ZERO value, so this pin says nothing about it. "+
				"Give it a distinguishing value in representativeParams().", v.Type().Field(i).Name)
		}
	}
}

// G-CFGBIND-6b — THE PARAMS-CARRYING GENESIS HASH IS PINNED, one literal per claim, the same
// discipline TestGenesisBlockHashIsPinned uses for the manifesto's geometry.
//
// WHAT IT ADDS OVER 6(b). 6(b) says the hash MOVES with the params. It cannot say the ENCODING is
// stable, because it compares two hashes computed by the same binary. A cbor key renumbering, a
// field reorder, or an omitempty added to a ConsensusParams field would move every network's
// height-0 identity while leaving 6(b) perfectly green — the two hashes would still differ from
// each other. This literal is what makes that change loud.
//
// The paramless literal is UNCHANGED from TestGenesisBlockHashIsPinned and is repeated here on
// purpose: it is the proof that adding the field cost no committed fixture its hash. Params is a
// POINTER with omitempty, so nil omits cbor key 20 entirely and a pre-bind genesis is byte-
// identical to one written before the field existed.
//
// ABLATION: renumber ConsensusParams.Quorum from cbor key 1 to key 18 -> RED here, GREEN on
// G-CFGBIND-6.
func TestG_CFGBIND_6b_TheParamsCarryingGenesisHashIsPinned(t *testing.T) {
	const (
		wantParamless  = "e44344eafa258c64904d88337e72ad7a904bd3c16a3058abb8d58557740272c0"
		wantWithParams = "4a305b96db986bb73ff571e93f1059bbafd47ce618da6ce0519869e05c8b9c46"
	)
	p := representativeParams()
	withParams, _, _, err := genesis.Build(memstore.New(), &p)
	if err != nil {
		t.Fatal(err)
	}
	paramless, _, _, err := genesis.Build(memstore.New(), nil)
	if err != nil {
		t.Fatal(err)
	}
	ph := paramless.Hash()
	if got := hex.EncodeToString(ph[:]); got != wantParamless {
		t.Fatalf("the PARAMLESS genesis hash is %s, want %s — adding Block.Params was supposed to cost no "+
			"pre-bind genesis its identity (nil pointer + omitempty omits cbor key 20 entirely). It moved.", got, wantParamless)
	}
	wh := withParams.Hash()
	if got := hex.EncodeToString(wh[:]); got != wantWithParams {
		t.Fatalf("the PARAMS-CARRYING genesis hash is %s, want %s — the ConsensusParams ENCODING moved "+
			"(a cbor key, a field order, an omitempty), so every network minted by this binary has a different "+
			"height-0 identity from one minted by the last. Nodes across the change cannot join each other. "+
			"Move this literal only as an explicit, recorded act.", got, wantWithParams)
	}
}

// representativeParams is the fixture: EVERY field distinguishably non-zero, checked by
// G-CFGBIND-6. The values are not a real network's — they exist to make each field observable.
func representativeParams() chain.ConsensusParams {
	return chain.ConsensusParams{
		Quorum:                  3,
		ByzantineQuorum:         true,
		Anchors:                 chain.SortedAnchors(map[ports.NodeID]bool{ports.HashBytes([]byte("anchor-a")): true, ports.HashBytes([]byte("anchor-b")): true}),
		AnchorQuorum:            2,
		MatureValidators:        7,
		OperatorMargin:          1,
		MinBond:                 1 << 20,
		MinBondBytes:            2 << 20,
		BondTTLBlocks:           64,
		BondRegHeadWindow:       8,
		EpochBlocks:             32,
		RegGateActivationHeight: 100,
		Era3ActivationHeight:    200,
		Era4ActivationHeight:    300,
		AllowPublisher:          true,
		BondLabelSamples:        64,
		BondVDFDelay:            1000,
	}
}
