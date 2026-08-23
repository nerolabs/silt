package chain

import (
	"crypto/ed25519"
	"errors"
	"testing"

	"github.com/nerolabs/silt/ports"
)

// #535 fix (4), research-certified 2026-08-23: a frozen-epoch member re-proving
// a Root it ALREADY OWNS is exempt from the #506 R interval — a returning
// participant restoring standing it already held, not the reg-flood of fresh
// identities R defends against. This is the layer that removes the field
// wedge's non-recovery (a returning member was R-refused, `re-registering 1
// block after its last reg, R=10`, and so could not re-bond to heal the stalled
// boundary). Safe by construction: it can only restore weight the honest set
// already trusted for this epoch, never admit new weight (unlike shrinking the
// quorum denominator — the certification's rejected fix (1), which cheapened
// capture 344→216 MiB).
func TestRegGate_535_RestoreExemptionForReturningFrozenMember(t *testing.T) {
	const bond = int64(64) << 20
	// 4 frozen-epoch members (mature at genesis via MatureValidators=0), gate
	// active from h>1, TTL 32 ⇒ R = 10.
	k := make([]ed25519.PrivateKey, 4)
	for i := range k {
		k[i] = key(int64(53900 + i))
	}
	cfg := Config{Quorum: 2, MinBond: 1 << 20, ByzantineQuorum: true,
		MatureValidators: 0, EpochBlocks: 8, BondTTLBlocks: 32, RegGateActivationHeight: 1}
	c := New(cfg, func(ports.NodeID) int64 { return 0 })
	c.SetBondVerifier(objectiveVerify)

	g := &Block{Version: 1, Height: 0, Entries: []ports.Entry{entry(0)}}
	for i, kk := range k {
		g.BondRegs = append(g.BondRegs, bondRegDom(kk, bond, ports.Hash{}, uint64(i+1)))
	}
	Sign(g, k[0])
	if err := c.AppendGenesis(*g); err != nil {
		t.Fatalf("genesis: %v", err)
	}
	if _, ok := c.epochSet[idOf(k[1])]; !ok {
		t.Fatal("setup: member must be seated in the frozen genesis epoch")
	}
	member := idOf(k[1])
	root := ports.HashBytes(k[1].Public().(ed25519.PublicKey))
	if c.bondRootOwner[root] != member {
		t.Fatal("setup: member must own its genesis root")
	}

	// h1 (pre-gate): a plain attested block, so h2 is past the activation height.
	prev, _ := c.Head()
	b1 := &Block{Version: 1, Height: 1, Prev: prev, Entries: []ports.Entry{entry(1)}}
	Sign(b1, k[0])
	b1.Atts = []Attestation{Attest(b1, k[1]), Attest(b1, k[2]), Attest(b1, k[3])}
	if err := c.Append(*b1); err != nil {
		t.Fatalf("h1: %v", err)
	}
	if !c.regGateActive(2) {
		t.Fatal("setup: the R-gate must be active at h2")
	}

	// Model the LAPSE: the member's standing drops below MinBond (its bond
	// lapsed while it was offline through the boundary) while it stays seated in
	// the frozen epoch (attesterQualified is frozen membership) and still owns
	// its root. This is the state a returning member must re-register out of —
	// and the exact state #506's R would refuse without fix (4).
	if c.bonded[member] < c.cfg.MinBond {
		t.Fatal("setup: member should be bonded before we model its lapse")
	}
	delete(c.bonded, member) // lapsed: below MinBond, but frozen-epoch-seated + owns its root

	// h2 (gate active): member k[1] re-registers its OWN root, 2 blocks after its
	// genesis reg — inside R=10. Without fix (4) this is ErrRegGate; the
	// certification exempts it because it merely RESTORES held (now lapsed) standing.
	h1hash, _ := c.Head()
	reReg := bondRegDom(k[1], bond, h1hash, 2) // same Root (HashBytes(pub)), fresh nonce bound to prev
	if !c.restoresHeldStanding(member, reReg.Root) {
		t.Fatal("restoresHeldStanding must be true for a LAPSED frozen member re-proving its owned root")
	}
	b2 := &Block{Version: 1, Height: 2, Prev: h1hash, BondRegs: []BondReg{reReg}}
	Sign(b2, k[0])
	b2.Atts = []Attestation{Attest(b2, k[1]), Attest(b2, k[2]), Attest(b2, k[3])}
	if err := c.Append(*b2); err != nil {
		t.Fatalf("#535 fix (4): a returning frozen member re-proving its owned root within R must be EXEMPT, got: %v", err)
	}

	// NARROWNESS — R still bites everything that is NOT a restore:
	// (a) a non-owned root (a different identity's plot) is not a restore.
	if c.restoresHeldStanding(member, ports.HashBytes([]byte("some-other-plot-root"))) {
		t.Fatal("restore exemption must require the identity to OWN the root — a foreign root is not a restore")
	}
	// (b) an identity NOT in the frozen epoch (a fresh joiner) is not restoring.
	fresh := key(53990)
	freshRoot := ports.HashBytes(fresh.Public().(ed25519.PublicKey))
	c.bondRootOwner[freshRoot] = idOf(fresh) // it owns a root, but is not epoch-seated
	if c.restoresHeldStanding(idOf(fresh), freshRoot) {
		t.Fatal("restore exemption must require CURRENT frozen-epoch membership — a fresh joiner is not restoring")
	}
	// (c) a member that STILL holds live standing is NOT restoring — its re-reg
	// is the #506 storm, and R must still bite it (the narrowness that keeps
	// #506's flood protection intact for bonded members).
	c.bonded[member] = bond // re-bond it (live standing)
	if c.restoresHeldStanding(member, reReg.Root) {
		t.Fatal("a bonded member re-proving its root is the #506 storm, NOT a restore — must not be exempt")
	}
	delete(c.bonded, member) // back to lapsed for the slash check
	// (d) a slashed member never restores (evicted).
	c.slashed[member] = true
	if c.ValidateBondRegErr(reReg) == nil {
		t.Fatal("a slashed identity's reg must still be refused, restore exemption notwithstanding")
	}
	c.slashed[member] = false
	_ = errors.Is
}
