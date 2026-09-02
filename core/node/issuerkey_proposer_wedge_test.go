package node

// C2 — the proposer self-wedge on a current-era objective network.
//
// MECHANISM. `chain.validateIssuerKeys` reads the bond ledger PRE-apply
// (`c.bonded[r.IssuerID()] <= 0` ⇒ ErrIssuerKeyUnbonded), while `proposeBlock` folds
// the proposer's own first `BondReg` AND every still-uncommitted `pendingIssuerKeys`
// entry into the SAME block, then runs `n.chain.ValidateProposal(b)` as a local
// pre-check. The pre-check therefore fails against the PRE-state, `done(err)` aborts
// the proposal, and — because a staged registration rides and stays queued — the same
// registration is re-folded into every later proposal, which fails identically. A
// fresh validator that installs a demand-issuer key at startup (which is what the
// daemon does for `-accept-delivery-receipts`) can never propose again.
//
// FIX. Proposer POLICY: defer a registration whose issuer is not bonded in the
// PRE-state (`chain.IssuerKeyRegAdmissible`), keeping it staged. No validity rule
// changes, so a mixed swarm cannot fork on it — the same shape as the IsSlashed
// filter on pending bond regs (#503 Q1(a)).

import (
	"crypto/rand"
	"crypto/rsa"
	"errors"
	"testing"

	"github.com/nerolabs/silt/adapters/identity"
	"github.com/nerolabs/silt/adapters/markstore"
	"github.com/nerolabs/silt/adapters/memstore"
	"github.com/nerolabs/silt/adapters/simclock"
	"github.com/nerolabs/silt/adapters/simnet"
	"github.com/nerolabs/silt/core/chain"
	"github.com/nerolabs/silt/core/credit"
	"github.com/nerolabs/silt/ports"
)

// era4AnchorNet is tier2AnchorNet with the CURRENT ERA active from height 3: from
// there every proposed block mints v5, which is the only regime where `IssuerKeys`
// may ride at all. Genesis and height 1 stay pre-era, so the boundary behaves as on a
// real network, which activates through LOCK-IN some distance after launch
// (chain.era4Active), never at genesis.
//
// The pre-era warm-up blocks are not decoration. `apply` folds `b.Atts` into
// `validatorsSeen`, which is a committed state-root leaf, but a proposer stamps its
// roots BEFORE it gathers attestations — so a block whose attestations introduce a
// validator not already in `validatorsSeen` is rejected by the proposer's OWN replica
// on a root mismatch. Callers must therefore SATURATE `validatorsSeen` under a
// pre-era version (where no root predicate runs) before driving current-era
// proposals. See `saturateValidatorsSeen`.
func era4AnchorNet(t *testing.T, nAnchors int) ([]*Node, []*identity.Identity, *simnet.Network, *chain.Block, chain.Config) {
	t.Helper()
	sched := simclock.New()
	net := simnet.New(sched, 1, simnet.DefaultConfig())
	net.EnableHeldDelivery()

	ids := make([]*identity.Identity, nAnchors)
	anchors := map[ports.NodeID]bool{}
	for i := range ids {
		ids[i] = identity.FromSeed(int64(7700 + i))
		anchors[ids[i].NodeID()] = true
	}
	g := &chain.Block{Version: 1, Height: 0, Entries: []ports.Entry{mkEntry("g-era4")}}
	chain.Sign(g, ids[0].Signer())
	cfg := chain.Config{Quorum: 1, MinBond: 1 << 20, ByzantineQuorum: true, Anchors: anchors,
		MatureValidators: 99, Era3ActivationHeight: 3, Era4ActivationHeight: 3}

	nodes := make([]*Node, nAnchors)
	for i, id := range ids {
		nd := New(id.NodeID(), DefaultConfig(), sched, net.Endpoint(id.NodeID()), memstore.New())
		nd.SetLedger(credit.New(50_000, 0))
		ch := chain.New(cfg, func(ports.NodeID) int64 { return 0 })
		ch.SetBondVerifier(mcStubVerify)
		if err := ch.AppendGenesis(*g); err != nil {
			t.Fatalf("genesis: %v", err)
		}
		nd.EnableChain(ch, id.Signer())
		if err := nd.SetSignMarkStore(markstore.NewMem()); err != nil {
			t.Fatalf("sign-mark store: %v", err)
		}
		nodes[i] = nd
	}
	return nodes, ids, net, g, cfg
}

// proposeOnce drives one real proposal from `proposer` over held delivery and returns
// the proposal error (nil ⇒ committed). Every peer except the proposer attests.
func proposeOnce(t *testing.T, proposer *Node, net *simnet.Network, all []ports.NodeID, tag string) error {
	t.Helper()
	attesters := make([]ports.NodeID, 0, len(all)-1)
	for _, id := range all {
		if id != proposer.id {
			attesters = append(attesters, id)
		}
	}
	prev, h := proposer.chain.Head()
	b := &chain.Block{Version: chain.BlockVersion, Height: h, Prev: prev, Entries: []ports.Entry{mkEntry(tag)}}
	var done bool
	var perr error
	proposer.proposeBlock(b, attesters, all, 0, func(err error) { done, perr = true, err })
	drainHeld(t, net, fifo)
	if !done {
		t.Fatalf("proposal %q never completed", tag)
	}
	return perr
}

// saturateValidatorsSeen commits the pre-era blocks that put every member of `all`
// into `validatorsSeen`, which the current-era root predicate needs stable across a
// proposal (see era4AnchorNet). Each block is proposed by a different node, so every
// node attests at least once. It leaves the head at the era boundary.
func saturateValidatorsSeen(t *testing.T, nodes []*Node, net *simnet.Network, all []ports.NodeID) {
	t.Helper()
	for i := 1; i >= 0; i-- {
		if err := proposeOnce(t, nodes[i], net, all, "pre-era-"+string(rune('0'+i))); err != nil {
			t.Fatalf("the pre-era bootstrap block from node %d must commit: %v", i, err)
		}
	}
	if nodes[0].chain.MintVersion(headHeight(nodes[0])) < chain.BlockVersionWitnessable {
		t.Fatal("setup: the next block must mint current-era (v5), or IssuerKeys never ride")
	}
}

// headHeight is the height the NEXT block will occupy.
func headHeight(n *Node) uint64 {
	_, h := n.chain.Head()
	return h
}

// TestIssuerKeyRegDoesNotWedgeTheProposer is the C2 gate. A fresh validator installs
// its demand-issuer key at startup (staging the registration) on a network that is
// already current-era, with its own bond not yet committed, and must still propose —
// then land the registration once the bond commits, within a small bound.
//
// RED pre-fix: EVERY proposal from that node fails with ErrIssuerKeyUnbonded, forever.
func TestIssuerKeyRegDoesNotWedgeTheProposer(t *testing.T) {
	nodes, ids, net, _, _ := era4AnchorNet(t, 4)
	all := make([]ports.NodeID, len(ids))
	for i, id := range ids {
		all[i] = id.NodeID()
	}
	// The pre-era blocks carry the network past the era boundary, so the node under
	// test arrives with neither its bond nor its key registration committed — the
	// shape of a fresh validator on a network that is already current-era.
	saturateValidatorsSeen(t, nodes, net, all)
	fresh := nodes[0]

	// The daemon's startup shape for -accept-delivery-receipts: bond enabled, demand
	// issuer key installed for the live epoch. Neither is committed.
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("issuer key: %v", err)
	}
	fresh.EnableBond(ids[0].Signer(), 2<<20)
	fresh.SetDemandIssuerKey(fresh.DemandEpoch(), key)
	if len(fresh.pendingIssuerKeys) != 1 {
		t.Fatalf("setup: the key registration must be staged, got %d", len(fresh.pendingIssuerKeys))
	}
	if fresh.chain.IsBonded(fresh.id) {
		t.Fatal("setup: the bond must NOT be committed yet — that is the wedge precondition")
	}

	// The wedge, if present, is immediate and PERMANENT: the proposer folds its own
	// first BondReg and the staged registration into one block, and its own pre-check
	// rejects that block against the PRE-apply bond ledger.
	if err := proposeOnce(t, fresh, net, all, "first"); err != nil {
		if errors.Is(err, chain.ErrIssuerKeyUnbonded) {
			t.Fatalf("C2 self-wedge: the proposer folded a registration its own pre-check rejects: %v", err)
		}
		t.Fatalf("the first proposal must commit: %v", err)
	}

	// Within a small bound the bond commits and the registration follows it. The
	// bound is what makes this a LIVENESS gate: "eventually" would pass on a wedge
	// that merely takes longer.
	const bound = 5
	epoch := fresh.DemandEpoch()
	for i := 0; i < bound; i++ {
		if _, ok := fresh.chain.IssuerKeyCommitment(fresh.id, epoch); ok {
			break
		}
		if err := proposeOnce(t, fresh, net, all, "b"+string(rune('0'+i))); err != nil {
			t.Fatalf("proposal %d must commit, got %v", i, err)
		}
	}
	if _, ok := fresh.chain.IssuerKeyCommitment(fresh.id, epoch); !ok {
		t.Fatalf("the staged registration never committed within %d blocks", bound)
	}
	if !fresh.chain.IsBonded(fresh.id) {
		t.Fatal("the bond must be committed by the time the registration lands")
	}

	// The queue drains on the NEXT fold, not at commit: a registration RIDES AND
	// STAYS QUEUED until the chain confirms it (the #397-Q4-ii discipline), so the
	// deferral must not have turned that into permanent residency either.
	if err := proposeOnce(t, fresh, net, all, "drain"); err != nil {
		t.Fatalf("the drain proposal must commit: %v", err)
	}
	if len(fresh.pendingIssuerKeys) != 0 {
		t.Fatalf("a committed registration must leave the queue, %d still staged", len(fresh.pendingIssuerKeys))
	}
}

// TestIssuerKeyRegAdmissibleMirrorsTheValidityClause pins the POLICY hook to the RULE
// it mirrors. If the validity clause ever moves, this drifts and reddens rather than
// silently letting the proposer build blocks it will reject (or defer forever).
func TestIssuerKeyRegAdmissibleMirrorsTheValidityClause(t *testing.T) {
	nodes, ids, net, _, _ := era4AnchorNet(t, 4)
	proposer := nodes[0]
	all := make([]ports.NodeID, len(ids))
	for i, id := range ids {
		all[i] = id.NodeID()
	}
	saturateValidatorsSeen(t, nodes, net, all)
	if proposer.chain.IssuerKeyRegAdmissible(proposer.id) {
		t.Fatal("an unbonded issuer must not be admissible under MinBond > 0")
	}
	proposer.EnableBond(ids[0].Signer(), 2<<20)
	if err := proposeOnce(t, proposer, net, all, "bond"); err != nil {
		t.Fatalf("the bond-registering proposal must commit: %v", err)
	}
	if !proposer.chain.IssuerKeyRegAdmissible(proposer.id) {
		t.Fatal("a bonded issuer must be admissible once its bond is committed")
	}

	// Legacy mode (MinBond == 0) makes the validity clause inert, so the policy must
	// be inert too — otherwise a legacy network defers every registration forever.
	legacy := chain.New(chain.Config{Quorum: 1}, func(ports.NodeID) int64 { return 1 << 30 })
	if !legacy.IssuerKeyRegAdmissible(proposer.id) {
		t.Fatal("with MinBond == 0 the bonded gate is inert, so every issuer is admissible")
	}
}
