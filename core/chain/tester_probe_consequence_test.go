// TESTER PROBE — NOT FOR MERGE. The CONSEQUENCE half: accepted-as-evidence is not
// the same claim as validator-is-slashed. This drives the mixed-form pair all the way
// through Append -> validateSlashes -> apply() and reads the resulting standing.
package chain

import (
	"crypto/ed25519"
	"testing"

	"github.com/nerolabs/silt/ports"
)

// probeCrossFormSlashHarness runs one full commit of a block carrying `ev`, on a chain
// whose ChainID is its own genesis hash, and reports the victim's standing after.
func probeCrossFormSlashHarness(t *testing.T, mkEvidence func(cid ports.Hash, victim ed25519.PrivateKey, foreign ed25519.PrivateKey) Equivocation) {
	t.Helper()
	prop, v1, v2, v3, victim, foreignProp := key(51), key(52), key(53), key(54), key(55), key(96099)
	cfg := Config{Quorum: 1, MinBond: 1 << 20, ByzantineQuorum: true, EpochBlocks: 8}
	c := New(cfg, func(ports.NodeID) int64 { return 0 })
	c.SetBondVerifier(objectiveVerify)

	g := &Block{Version: 1, Height: 0, Entries: []ports.Entry{entry(0)}}
	g.BondRegs = append(g.BondRegs,
		bondReg(prop, twoMiB, ports.Hash{}),
		bondReg(v1, twoMiB, ports.Hash{}),
		bondReg(v2, twoMiB, ports.Hash{}),
		bondReg(v3, twoMiB, ports.Hash{}),
		bondReg(victim, twoMiB, ports.Hash{}))
	Sign(g, prop)
	if err := c.AppendGenesis(*g); err != nil {
		t.Fatalf("append genesis: %v", err)
	}
	cid := c.ChainID()
	if cid != g.Hash() {
		t.Fatalf("harness: ChainID must be the genesis hash; got %x want %x", cid, g.Hash())
	}
	if !c.attesterQualified(idOf(victim)) {
		t.Fatal("harness: the victim must START qualified, or the probe proves nothing")
	}
	if c.IsSlashed(idOf(victim)) {
		t.Fatal("harness: the victim must START unslashed")
	}

	ev := mkEvidence(cid, victim, foreignProp)
	b1 := &Block{Version: 1, Height: 1, Prev: g.Hash(), Entries: []ports.Entry{entry(1)},
		Slashes: []Equivocation{ev}}
	Sign(b1, prop)
	b1.Atts = []Attestation{Attest(b1, v1), Attest(b1, v2), Attest(b1, v3)}

	err := c.Append(*b1)
	t.Logf("CONSEQUENCE Append(block carrying the slash) -> err = %v", err)
	t.Logf("CONSEQUENCE IsSlashed(victim)        = %v", c.IsSlashed(idOf(victim)))
	t.Logf("CONSEQUENCE attesterQualified(victim)= %v", c.attesterQualified(idOf(victim)))
	t.Logf("CONSEQUENCE BondedSize(victim)       = %d (was %d before)", c.BondedSize(idOf(victim)), twoMiB)
	if err == nil && c.IsSlashed(idOf(victim)) {
		t.Log("CONSEQUENCE VERDICT: the honest validator IS PERMANENTLY EVICTED — I5 broken, not merely 'evidence accepted'")
	}
	if err != nil {
		t.Logf("CONSEQUENCE VERDICT: the commit was REFUSED; something upstream of apply() fired first: %v", err)
	}
}

// probeMkBlock builds one evidence leg: a block at height 9 with the victim's honest
// precommit at round 1, in the form dictated by `version`, bound to `cid`.
func probeMkBlock(version uint64, e byte, prop, victim ed25519.PrivateKey, cid ports.Hash) Block {
	b := Block{Version: version, Height: 9, Prev: ports.HashBytes([]byte("shared-prev")),
		Entries: []ports.Entry{entry(e)}}
	if version >= BlockVersionWitnessable {
		b.StateRoot, b.LogRoot = &ports.Hash{}, &ports.Hash{}
		setD3Digests(&b)
	}
	Sign(&b, prop)
	b.Atts = []Attestation{AttestAt(&b, victim, 1, PhasePrecommit, cid)}
	return b
}

// (v2 leg from a FOREIGN chain, v5 leg from THIS chain) — the certification's §5.3 case.
func TestProbe_Consequence_MixedFormV2V5(t *testing.T) {
	probeCrossFormSlashHarness(t, func(cid ports.Hash, victim, foreign ed25519.PrivateKey) Equivocation {
		foreignCID := ports.HashBytes([]byte("some other silt network genesis"))
		a := probeMkBlock(BlockVersionStateRoot, 1, foreign, victim, foreignCID) // v2 form: chain-BLIND
		b := probeMkBlock(BlockVersionWitnessable, 2, foreign, victim, cid)      // v5 form: bound to THIS chain
		if a.Atts[0].Phase != PhasePrecommit || b.Atts[0].Phase != PhasePrecommitV5 {
			t.Fatalf("FIXTURE BROKEN: forms are %d / %d, want %d / %d",
				a.Atts[0].Phase, b.Atts[0].Phase, PhasePrecommit, PhasePrecommitV5)
		}
		return Equivocation{Culprit: append([]byte(nil), victim.Public().(ed25519.PublicKey)...), A: a, B: b}
	})
}

// (v2 leg, v2 leg) from two FOREIGN chains — the pre-existing face.
func TestProbe_Consequence_BothV2(t *testing.T) {
	probeCrossFormSlashHarness(t, func(cid ports.Hash, victim, foreign ed25519.PrivateKey) Equivocation {
		cidX := ports.HashBytes([]byte("foreign silt network X genesis"))
		cidY := ports.HashBytes([]byte("foreign silt network Y genesis"))
		a := probeMkBlock(BlockVersionStateRoot, 1, foreign, victim, cidX)
		b := probeMkBlock(BlockVersionStateRoot, 2, foreign, victim, cidY)
		if a.Atts[0].Phase != PhasePrecommit || b.Atts[0].Phase != PhasePrecommit {
			t.Fatalf("FIXTURE BROKEN: forms are %d / %d, want both %d", a.Atts[0].Phase, b.Atts[0].Phase, PhasePrecommit)
		}
		return Equivocation{Culprit: append([]byte(nil), victim.Public().(ed25519.PublicKey)...), A: a, B: b}
	})
}

// CONTROL: (v5, v5) from two FOREIGN chains — the case G-PRE-6 covers. Must NOT slash.
func TestProbe_Consequence_BothV5Foreign(t *testing.T) {
	probeCrossFormSlashHarness(t, func(cid ports.Hash, victim, foreign ed25519.PrivateKey) Equivocation {
		cidX := ports.HashBytes([]byte("foreign silt network X genesis"))
		cidY := ports.HashBytes([]byte("foreign silt network Y genesis"))
		a := probeMkBlock(BlockVersionWitnessable, 1, foreign, victim, cidX)
		b := probeMkBlock(BlockVersionWitnessable, 2, foreign, victim, cidY)
		return Equivocation{Culprit: append([]byte(nil), victim.Public().(ed25519.PublicKey)...), A: a, B: b}
	})
}
