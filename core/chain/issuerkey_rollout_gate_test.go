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
	"fmt"
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

	// (c) The stamp itself, a HARD TRIPWIRE (red-team re-break F9, 2026-09-03). This
	// clause used to be a t.Logf, which `go test` without -v suppresses: a one-line
	// constant change could raise the stamp with a fully green tree, which is exactly
	// the "cannot happen silently" property the gate claimed to provide and did not.
	// It is now a failure. Raising the stamp is a deliberate act, so raising it is a
	// deliberate EDIT HERE — and the edit is where a human re-reads (a) and (b) and
	// confirms the release carrying the raise also carries cbor key 17 in the hashed
	// block, validateIssuerKeys, and the issuerKeyCommit leaf.
	if msg := gateFStampPin(stamp); msg != "" {
		t.Fatal(msg)
	}
}

// gateFStampPin is clause (c)'s predicate, factored out so its TEETH are themselves
// testable: TestGateF_StampRaiseIsAHardFailure asserts that a raised stamp produces a
// non-empty message, which is the property "the raise cannot happen silently" — the property
// the t.Logf version claimed and did not have (red-team re-break F9).
func gateFStampPin(stamp uint8) string {
	if stamp == BlockVersionRegGate {
		return ""
	}
	return fmt.Sprintf("NewBondReg stamps readiness %d, but this gate is pinned to %d. "+
		"Raising the mint stamp turns the R0.4b rollout rule LIVE: every node that accepts a "+
		"v%d block must carry cbor key 17 inside the hashed block, validateIssuerKeys, and "+
		"the issuerKeyCommit leaf (asserted above). Confirm the release carries all three, "+
		"then update this pin in the same commit.", stamp, BlockVersionRegGate, stamp)
}

// TestGateF_ConfigActivationRouteCarriesTheSameCoverage closes the OTHER half of the F9
// finding: the readiness TALLY is not the only route to v5. Config.Era4ActivationHeight
// activates era-4 "with no readiness signalling at all" (chain.go:265), so a network can mint
// v5 blocks — and therefore key registrations — while every NewBondReg still stamps 3 and the
// tally clause of the rollout rule never fires.
//
// The rule the tally route enforces is a property of the BINARY, not of the activation route,
// so this drives the config route end to end and asserts the same two coverage properties on
// a real minted block: the hash covers the registrations, and the post-apply committed root
// carries the issuerKeyCommit leaf. If a future edit ever makes the leaf conditional on the
// tally, this reddens where the stamp pin cannot.
func TestGateF_ConfigActivationRouteCarriesTheSameCoverage(t *testing.T) {
	cfg := Config{Quorum: 1, MinBond: era4MinBond, ByzantineQuorum: true,
		EpochBlocks: 2, MatureValidators: 0, BondTTLBlocks: 4096, Era4ActivationHeight: 1}
	c := New(cfg, func(ports.NodeID) int64 { return 0 })
	c.SetBondVerifier(objectiveVerify)
	prop := key(94200)
	g := &Block{Version: BlockVersionWitnessable, Height: 0}
	g.BondRegs = append(g.BondRegs,
		bondRegFull(prop, ports.HashBytes(pubOf(prop)), 8<<20, ports.Hash{}, 5, 1))
	Sign(g, prop)
	c.apply(*g)

	// The bypass, measured: era-4 is admissible at height 1 with NO readiness lock-in and a
	// mint stamp still at BlockVersionRegGate.
	if !c.era4Active(1) {
		t.Fatalf("fixture: Era4ActivationHeight=1 must make v5 admissible at height 1")
	}
	if c.era4LockedIn {
		t.Fatalf("fixture: the config route must NOT need a readiness lock-in")
	}
	if stamp := NewBondReg(key(94201), ports.Hash{0x01}, 1, nil, ports.Hash{}, 0).Version; uint64(stamp) >= uint64(BlockVersionWitnessable) {
		t.Fatalf("fixture drifted: the mint stamp is %d, so this is no longer the bypass shape", stamp)
	}

	prev, h := c.Head()
	b := Block{Version: BlockVersionWitnessable, Height: h, Prev: prev,
		Entries:    []ports.Entry{entry(9)},
		IssuerKeys: []IssuerKeyReg{SignIssuerKeyReg(prop, c.blockEpoch(h), ports.Hash{0x77})}}
	Sign(&b, prop)

	// (a) on the config route: the hash covers the registrations.
	withoutRegs := b
	withoutRegs.IssuerKeys = nil
	withoutRegs.hashMemoSet = false
	if b.Hash() == withoutRegs.Hash() {
		t.Fatalf("on the Era4ActivationHeight route, Block.Hash does not cover IssuerKeys")
	}

	// (b) on the config route: the applied block commits an issuerKeyCommit leaf.
	if err := c.validateIssuerKeys(&b); err != nil {
		t.Fatalf("the registration must be valid on a config-activated v5 chain: %v", err)
	}
	c.apply(b)
	found := false
	for _, lf := range c.stateRootLeavesV5() {
		if tagOfKey(string(lf.Key)) == "issuerKeyCommit" {
			found = true
		}
	}
	if !found {
		t.Fatalf("a config-activated v5 chain applied a registration but committed no " +
			"issuerKeyCommit leaf — the committed root does not bind the state the block wrote")
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
