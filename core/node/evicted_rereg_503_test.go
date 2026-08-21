package node

// #503 — the evicted-identity re-registration storm (research-certified Q1,
// research-outcome/503-bond-renewal-storm-RESEARCH-CERTIFICATION-2026-08-21.md).
// An F2 slash deletes bonded[id], which makes BondRenewalDue's first clause
// (bonded < MinBond) true forever; the daemon re-submits its ~1.5 MB reg every
// sweep, no layer consults slashed, and honest proposers commit the banned
// identity's registration as a fresh block every ~30 s, unbounded — the island
// OOM's dominant driver (build-immutable #8). Certified fix, zero block-validity
// change: (c) the client never submits once it observes its own slash; (a) an
// honest receiver refuses the reg at arrival and an honest proposer never folds
// it. The structural close (a validity-rule rate bound, Q3) is a separate,
// version-gated issue.
//
// FAILING-FIRST (all three born RED on the pre-fix tree).

import (
	"crypto/ed25519"
	"testing"

	"github.com/nerolabs/silt/adapters/identity"
	"github.com/nerolabs/silt/adapters/memstore"
	"github.com/nerolabs/silt/core/chain"
	"github.com/nerolabs/silt/ports"
)

// slashOnChain commits a valid slash block for culprit at height 1 on every
// given chain: the culprit's own double-sign (two conflicting height-1 blocks)
// is self-verifying proof, proposed by anchor ids[0] and attested by enough
// anchors to clear the launch gate. Returns the slash block.
func slashOnChain(t *testing.T, g *chain.Block, culprit *identity.Identity, ids []*identity.Identity, chains ...*chain.Chain) *chain.Block {
	t.Helper()
	mkFork := func(tag byte) chain.Block {
		b := chain.Block{Version: 1, Height: 1, Prev: g.Hash(), Entries: []ports.Entry{mkEntry("fork-" + string(rune(tag)))}}
		chain.Sign(&b, culprit.Signer())
		return b
	}
	proof := chain.Equivocation{
		Culprit: append([]byte(nil), culprit.Signer().Public().(ed25519.PublicKey)...),
		A:       mkFork('a'), B: mkFork('b'),
	}
	if !chain.VerifyEquivocation(&proof) {
		t.Fatal("setup: the equivocation proof must self-verify")
	}
	sb := &chain.Block{Version: 1, Height: 1, Prev: g.Hash(), Slashes: []chain.Equivocation{proof}}
	chain.Sign(sb, ids[0].Signer())
	for _, att := range ids[1:] {
		sb.Atts = append(sb.Atts, chain.Attest(sb, att.Signer()))
	}
	for _, c := range chains {
		if err := c.Append(*sb); err != nil {
			t.Fatalf("setup: the slash block must commit: %v", err)
		}
		if !c.IsSlashed(culprit.NodeID()) {
			t.Fatal("setup: the culprit must be slashed after the commit")
		}
	}
	return sb
}

// regFrom builds a decode/signature/size-valid reg for who over the chain's
// current head — the exact payload an evicted daemon re-broadcasts each sweep.
func regFrom(ch *chain.Chain, who *identity.Identity) chain.BondReg {
	head, _ := ch.Head()
	var root ports.Hash
	nid := who.NodeID()
	copy(root[:], nid[:16])
	return chain.NewBondReg(who.Signer(), root, 2<<20, []byte("stub"), head, 0)
}

// TestSlashedSelfNeverSubmitsRenewal503 — Q1(c): a validator that observes its
// OWN F2 eviction on the committed chain must stop submitting renewals, even
// though BondRenewalDue stays true forever (bonded[id] was deleted — the storm
// signal this fix deliberately does NOT change). RED pre-fix: the evicted node
// broadcasts its ~1.5 MB reg every sweep.
func TestSlashedSelfNeverSubmitsRenewal503(t *testing.T) {
	nodes, ids, net, g, cfg := tier2AnchorNet(t, 4)

	evicted := identity.FromSeed(9501)
	ch := chain.New(cfg, func(ports.NodeID) int64 { return 0 })
	ch.SetBondVerifier(mcStubVerify)
	if err := ch.AppendGenesis(*g); err != nil {
		t.Fatal(err)
	}
	slashOnChain(t, g, evicted, ids, ch, nodes[0].chain)

	nd := New(evicted.NodeID(), DefaultConfig(), nodes[0].clock, net.Endpoint(evicted.NodeID()), memstore.New())
	nd.EnableBond(evicted.Signer(), 2<<20)
	nd.EnableChain(ch, evicted.Signer())

	if !nd.chain.IsSlashed(nd.ID()) {
		t.Fatal("setup: the node must see its own slash on the committed chain")
	}
	if !nd.chain.BondRenewalDue(nd.ID()) {
		t.Fatal("setup: BondRenewalDue must still read true for the evicted id — the fix gates the SUBMIT, not the signal")
	}

	nd.SubmitBondRenewal([]ports.NodeID{ids[0].NodeID()})
	for _, held := range net.Pending() {
		if held.Kind == ports.MsgSubmitBondReg {
			t.Fatal("#503 Q1(c): a permanently evicted identity submitted a bond renewal — the storm's client half (must back off forever, with a log naming the eviction)")
		}
	}
}

// TestSlashedIdentityRegRefusedAtSubmit503 — Q1(a), arrival: an honest receiver
// refuses a slashed identity's submitted reg BEFORE the expensive space-time
// verify (a map lookup, same placement discipline as the Phase 1.2 CPU gate)
// and never queues it. A non-slashed sender still passes (control). RED
// pre-fix: the reg reaches the verify and the queue.
func TestSlashedIdentityRegRefusedAtSubmit503(t *testing.T) {
	nodes, ids, _, g, _ := tier2AnchorNet(t, 4)
	r := nodes[0]

	evicted := identity.FromSeed(9502)
	slashOnChain(t, g, evicted, ids, r.chain)

	verifies := 0
	r.chain.SetBondVerifier(func([]byte, ports.Hash, int64, uint64, []byte) bool {
		verifies++
		return true
	})

	r.handleChain(evicted.NodeID(), ports.Message{Kind: ports.MsgSubmitBondReg, Data: bondRegEncode(regFrom(r.chain, evicted))})
	if verifies != 0 {
		t.Fatalf("#503 Q1(a): a slashed identity's reg reached the expensive verify (%d) — must be refused at arrival by a map lookup", verifies)
	}
	if len(r.pendingBondRegs) != 0 {
		t.Fatal("#503 Q1(a): a slashed identity's reg was queued for the next block")
	}

	// Control: an honest (non-slashed) submitter still reaches validation.
	honest := identity.FromSeed(9503)
	r.handleChain(honest.NodeID(), ports.Message{Kind: ports.MsgSubmitBondReg, Data: bondRegEncode(regFrom(r.chain, honest))})
	if verifies != 1 {
		t.Fatalf("an honest submitter must still reach the verify (got %d) — the refusal must be slashed-only", verifies)
	}
}

// TestSlashedRegNeverFolded503 — Q1(a), fold: a slashed identity's reg that
// RACED into the pending queue before the slash committed must not be folded
// into the proposer's next block (the queue is re-filtered each fold; this adds
// the slashed check to that filter). The committed block stays reg-free — the
// storm's block-bloat half. No validity change: an attester still ACCEPTS a
// block carrying such a reg (mixed-version safety); only the honest proposer
// declines to build one. RED pre-fix: the block commits carrying the reg.
func TestSlashedRegNeverFolded503(t *testing.T) {
	nodes, ids, net, g, _ := tier2AnchorNet(t, 4)
	proposer := nodes[0]

	evicted := identity.FromSeed(9504)
	chains := make([]*chain.Chain, len(nodes))
	for i := range nodes {
		chains[i] = nodes[i].chain
	}
	slashOnChain(t, g, evicted, ids, chains...)

	// The race: the reg was queued while the slash was still in flight.
	proposer.queuePendingBondReg(regFrom(proposer.chain, evicted))

	prev, h := proposer.chain.Head()
	b := &chain.Block{Version: chain.BlockVersion, Height: h, Prev: prev, Entries: []ports.Entry{mkEntry("post-slash")}}
	attesters := []ports.NodeID{ids[1].NodeID(), ids[2].NodeID(), ids[3].NodeID()}
	all := []ports.NodeID{ids[0].NodeID(), ids[1].NodeID(), ids[2].NodeID(), ids[3].NodeID()}
	var done bool
	var perr error
	proposer.proposeBlock(b, attesters, all, 0, func(err error) { done, perr = true, err })
	drainHeld(t, net, fifo)
	if !done || perr != nil {
		t.Fatalf("the post-slash block must commit: done=%v err=%v", done, perr)
	}

	head, _ := proposer.chain.Head()
	committed := proposer.chain.Blocks(0)
	last := committed[len(committed)-1]
	if last.Hash() != head {
		t.Fatalf("setup: expected the committed head, got height %d", last.Height)
	}
	for _, reg := range last.BondRegs {
		if reg.ValidatorID() == evicted.NodeID() {
			t.Fatal("#503 Q1(a): the proposer folded a slashed identity's reg into its block — the storm's commit path")
		}
	}
}
