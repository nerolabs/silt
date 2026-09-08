package node

import (
	"errors"
	"testing"

	"github.com/nerolabs/silt/adapters/identity"
	"github.com/nerolabs/silt/adapters/memstore"
	"github.com/nerolabs/silt/adapters/simclock"
	"github.com/nerolabs/silt/adapters/simnet"
	"github.com/nerolabs/silt/core/chain"
	"github.com/nerolabs/silt/ports"
)

// TestM380_2_NewViewRefusesEmptyRoundChangeSet is M-380-2 of the research
// certification CONSENSUS-380-quorum-floor-direction-1-PREDICATE-AND-CERTIFICATION-2026-09-08.md
// (§2, the rounds.go newViewFor row): with the mature-epoch count floor at 0
// (#380 regime (b)), SupportMeetsQuorum(designated, [], h) reduces to the
// weight rule over the proposer alone, so a new-view certificate carrying
// ZERO round-change envelopes would validate whenever the round's designee
// holds > 2/3 of the frozen weight — and newViewFor would return forced ==
// nil, freeing that designee from any carried lock. A genuine view change
// always has >= 1 sender, so refusing the empty set costs nothing. This is
// node-side (liveness), not a validity rule; the same short-circuit class as
// the acceptRoundCert Round == 0 guard (T-SHORTCIRCUIT).
//
// THE FIXTURE is the smallest world that reaches the state: one bonded
// identity (the whale) is the whole frozen epoch set, so it is the round-1
// designee AND holds 100% of the frozen weight; RequiredQuorum() is 0.
// RED-FIRST (predicate landed, guard absent): newViewFor(1, 1, nil) returns
// (nil, nil). GREEN with the guard: errEmptyNewView, by name.
func TestM380_2_NewViewRefusesEmptyRoundChangeSet(t *testing.T) {
	sched := simclock.New()
	net := simnet.New(sched, 1, simnet.DefaultConfig())
	whale := identity.FromSeed(8800)
	cfg := chain.Config{Quorum: 1, MinBond: 1 << 20, ByzantineQuorum: true, MatureValidators: 0, EpochBlocks: 4}

	g := &chain.Block{Version: 1, Height: 0, Entries: []ports.Entry{mkEntry("g-m380-2")}}
	pub := whale.NodeID()
	g.BondRegs = []chain.BondReg{chain.NewBondReg(whale.Signer(), ports.HashBytes(pub[:]), 10<<20, []byte("stub"), ports.Hash{}, 1)}
	chain.Sign(g, whale.Signer())

	nd := New(pub, DefaultConfig(), sched, net.Endpoint(pub), memstore.New())
	ch := chain.New(cfg, func(ports.NodeID) int64 { return 0 })
	ch.SetBondVerifier(mcStubVerify)
	if err := ch.AppendGenesis(*g); err != nil {
		t.Fatalf("genesis: %v", err)
	}
	nd.EnableChain(ch, whale.Signer())

	// Premises — each one is what makes the empty set pass every OTHER check,
	// so a RED below is attributable to the missing guard alone.
	const height, round = uint64(1), uint64(1)
	if got := ch.RequiredQuorum(); got != 0 {
		t.Fatalf("premise: RequiredQuorum() must be 0 in the mature regime (#380 regime (b)), got %d", got)
	}
	if got := nd.designatedProposer(height, round); got != pub {
		t.Fatalf("premise: the whale must be the round-%d designee at height %d", round, height)
	}
	if !ch.SupportMeetsQuorum(pub, nil, height) {
		t.Fatal("premise: the whale alone must clear SupportMeetsQuorum with NO attesters (floor 0, > 2/3 weight) — " +
			"otherwise this test cannot reach the guard")
	}

	forced, err := nd.newViewFor(height, round, nil)
	if err == nil {
		t.Fatalf("M-380-2 REPRODUCED: newViewFor(h=%d, r=%d, ZERO envelopes) validated (forced=%v) — a zero-envelope "+
			"new-view certificate frees a > 2/3-weight designee from its lock; newViewFor must refuse an empty "+
			"round-change set", height, round, forced != nil)
	}
	if !errors.Is(err, errEmptyNewView) {
		t.Fatalf("M-380-2: refused for the WRONG reason (%v), want errEmptyNewView by name", err)
	}
	t.Logf("M-380-2: empty new-view certificate refused by name: %v", err)
}
