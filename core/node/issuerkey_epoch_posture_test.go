package node

// R-E2E-ERA4-FIXTURE, the POSTURE half (freeze manifest item 17).
//
// WHAT WAS OWED, AND WHY. The item's FORMAT half discharged as a side effect of the
// -era4-activation-height default; the POSTURE half — objective + bonded +
// epoch-enabled — did not. NO fixture that drove pinDemandIssuerKey's POSITIVE arm
// resolved against a chain in that posture. Read one at a time, which is the only way
// to see it, because each fixture is missing a DIFFERENT leg:
//
//   - demandkeys_test.go newIssuerKeyFixture, sim demandbinding_test.go
//     issuerKeyGenesis and e2e relay_paid_test.go anchorChainFor all build
//     chain.Config{Quorum: 1} — no MinBond, no ByzantineQuorum, no EpochBlocks — and
//     place the IssuerKeyReg in GENESIS. So the binding goes through
//     validateGenesisIssuerKeys, NOT validateIssuerKeys; they are different functions
//     with different rules, and the normal door's epoch window never runs. With
//     cfg.EpochBlocks == 0, blockEpoch and chainEpoch are identically 0, so every
//     registration and every pin lives at epoch 0.
//   - r04b_c3_gates_test.go c3Chain DOES set EpochBlocks (8) and mints v5 past
//     genesis, but it is "in legacy mode so block production stays plain" — its own
//     words. cfg.MinBond is 0 and no bond verifier is wired, so chain.objective() is
//     false, chain.epochsEnabled() (EpochBlocks > 0 AND objective()) is false, and
//     validateIssuerKeys' bonded clause is INERT by its own terms
//     ("c.cfg.MinBond > 0 && c.bonded[...] <= 0" — issuerkey.go).
//
// So the bonded clause has never gated a pin that later succeeded, and no positive pin
// has ever resolved a binding committed at a non-zero epoch on an objective chain.
//
// THIS TEST IS THE POSTURE. The chain is objective (MinBond > 0 + a wired bond
// verifier), bonded (the issuer's BondReg is COMMITTED before its registration is
// admissible), epoch-enabled (EpochBlocks > 0, and the binding commits at a NON-ZERO
// epoch, past a real epoch boundary) and era-4 (the carrying block mints v5). The
// registration rides through the real proposer fold and the real validateIssuerKeys.
//
// AND IT DRIVES BOTH ARMS ON THAT ONE FIXTURE. A green refusal with no demonstrated
// accept is decoration, so the discrimination is what is asserted:
//
//	ACCEPT   — the committed binding matches the served key   -> pinned == 1
//	REFUSE-A — ABSENT: an issuer that committed no binding    -> pinned == 0
//	REFUSE-M — MISMATCH: a served key off the commitment      -> pinned == 0
//
// In EVERY arm the resolving fetcher HAS A CHAIN, asserted explicitly. That is the
// point of the gate. On the OS-process e2e fixtures the refusal is NOT attributable:
// `silt swarm receipt` resolves on the chain-less ephemeral node joinSwarm builds
// (cmd/silt/daemon.go; the only EnableChain call outside tests in cmd/ is the
// daemon's), so pinDemandIssuerKey returns false at its `n.chain == nil` guard before
// the committed-binding check ever runs. MEASURED 2026-09-11: deleting the commitment
// check from pinDemandIssuerKey AND from DemandIssuerKeyset's Retain leaves both
// TestPaidDeliveryLaneRefusesWithoutACommittedKeyBinding and
// TestPaidDeliveryLaneArmsInTheHarnessPosture GREEN, exit 0.

import (
	"crypto/rand"
	"crypto/rsa"
	"testing"

	"github.com/nerolabs/silt/adapters/identity"
	"github.com/nerolabs/silt/adapters/markstore"
	"github.com/nerolabs/silt/adapters/memstore"
	"github.com/nerolabs/silt/adapters/simclock"
	"github.com/nerolabs/silt/adapters/simnet"
	"github.com/nerolabs/silt/core/blindtoken"
	"github.com/nerolabs/silt/core/chain"
	"github.com/nerolabs/silt/core/credit"
	"github.com/nerolabs/silt/core/demand"
	"github.com/nerolabs/silt/ports"
)

// era4EpochNet is era4AnchorNet with an EPOCH CLOCK. epochBlocks == 0 reproduces
// era4AnchorNet exactly (epochs disabled), which is what era4AnchorNet calls.
func era4EpochNet(t *testing.T, nAnchors int, epochBlocks uint64) ([]*Node, []*identity.Identity, *simnet.Network, *chain.Block, chain.Config) {
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
		MatureValidators: 99, Era3ActivationHeight: 3, Era4ActivationHeight: 3,
		EpochBlocks: epochBlocks}

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

// commitBindingInThePosture drives the fixture to the state under test and returns the
// issuer, its key and the epoch its binding was committed for. Every step is asserted:
// a fixture that quietly fails to reach the posture makes the discrimination vacuous.
func commitBindingInThePosture(t *testing.T, nodes []*Node, ids []*identity.Identity,
	net *simnet.Network, all []ports.NodeID, epochBlocks uint64) (*Node, *rsa.PrivateKey, uint64) {
	t.Helper()
	saturateValidatorsSeen(t, nodes, net, all)
	issuer := nodes[0]

	// BONDED. The registration is inadmissible until the issuer's own bond is
	// COMMITTED — the clause that is inert on every MinBond == 0 fixture.
	if issuer.chain.IssuerKeyRegAdmissible(issuer.id) {
		t.Fatal("posture: an unbonded issuer must not be admissible under MinBond > 0")
	}
	issuer.EnableBond(ids[0].Signer(), 2<<20)
	if err := proposeOnce(t, issuer, net, all, "bond"); err != nil {
		t.Fatalf("the bond-registering proposal must commit: %v", err)
	}
	if !issuer.chain.IsBonded(issuer.id) {
		t.Fatal("posture: the issuer's bond must be committed")
	}

	// EPOCH-ENABLED. Carry the chain past a real epoch boundary so the registration,
	// the commitment and the pin all live at a NON-ZERO epoch.
	for i := 0; issuer.DemandEpoch() == 0; i++ {
		if i > 16 {
			t.Fatalf("the chain never crossed an epoch boundary with EpochBlocks = %d", epochBlocks)
		}
		if err := proposeOnce(t, issuer, net, all, "epoch"+string(rune('a'+i))); err != nil {
			t.Fatalf("the epoch-advancing proposal must commit: %v", err)
		}
	}
	epoch := issuer.DemandEpoch()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("issuer key: %v", err)
	}
	issuer.SetDemandIssuerKey(rand.Reader, epoch, key)
	if len(issuer.pendingIssuerKeys) != 1 {
		t.Fatalf("setup: the registration for epoch %d must be staged, got %d", epoch, len(issuer.pendingIssuerKeys))
	}

	const bound = 6
	for i := 0; i < bound; i++ {
		if _, ok := issuer.chain.IssuerKeyCommitment(issuer.id, epoch); ok {
			break
		}
		if err := proposeOnce(t, issuer, net, all, "reg"+string(rune('0'+i))); err != nil {
			t.Fatalf("proposal %d must commit: %v", i, err)
		}
	}
	fp, ok := issuer.chain.IssuerKeyCommitment(issuer.id, epoch)
	if !ok {
		t.Fatalf("the registration for epoch %d never committed within %d blocks", epoch, bound)
	}
	if fp != demand.KeyFingerprint(&key.PublicKey) {
		t.Fatal("the committed fingerprint is not this key's")
	}

	// Find the block that carried it, and assert the POSTURE it committed under: a
	// NON-ZERO block epoch (so validateIssuerKeys' epoch window was live, not the
	// degenerate cur == 0 every prior fixture ran at) and an era-4/v5 block.
	var at uint64
	var version uint64
	found := false
	for h := uint64(0); ; h++ {
		bs := issuer.chain.Blocks(h)
		if len(bs) == 0 {
			break
		}
		for _, b := range bs {
			for _, r := range b.IssuerKeys {
				if r.IssuerID() == issuer.id && r.Epoch == epoch {
					at, version, found = b.Height, b.Version, true
				}
			}
		}
	}
	if !found {
		t.Fatal("the committed registration is in no block — the commitment map and the blocks disagree")
	}
	if at == 0 {
		t.Fatal("posture: the registration rode in GENESIS, so it went through validateGenesisIssuerKeys, not validateIssuerKeys")
	}
	if got := issuer.chain.BlockEpoch(at); got == 0 {
		t.Fatalf("posture: the carrying block at height %d is in epoch 0 — the epoch window is degenerate", at)
	}
	if version < chain.BlockVersionWitnessable {
		t.Fatalf("posture: the carrying block at height %d is v%d, want era-4 (v%d)", at, version, chain.BlockVersionWitnessable)
	}
	if !issuer.chain.Objective() {
		t.Fatal("posture: the chain must run objective fork-choice")
	}
	if issuer.chain.EpochBlocks() == 0 {
		t.Fatal("posture: the chain must be epoch-enabled")
	}
	t.Logf("posture reached: objective + bonded + epoch-enabled; binding for epoch %d committed in a v%d block at height %d (block epoch %d)",
		epoch, version, at, issuer.chain.BlockEpoch(at))
	return issuer, key, epoch
}

// pinFrom drives the real FetchDemandIssuerKeys call over the wire and returns how
// many keys the fetcher PINNED. It asserts the fetcher has a chain first: without
// that, every count below is 0 for a reason that has nothing to do with the binding.
func pinFrom(t *testing.T, fetcher *Node, net *simnet.Network, issuer ports.NodeID) (int, error) {
	t.Helper()
	if fetcher.chain == nil {
		t.Fatal("the resolving fetcher has NO CHAIN — pinDemandIssuerKey refuses at its nil guard and this measures nothing")
	}
	var pinned int
	var perr error
	var done bool
	fetcher.FetchDemandIssuerKeys(issuer, func(n int, err error) { pinned, perr, done = n, err, true })
	drainHeld(t, net, fifo)
	if !done {
		t.Fatal("FetchDemandIssuerKeys never completed")
	}
	return pinned, perr
}

// TestIssuerKeyBindingResolvesInTheObjectiveBondedEpochPosture is the R-E2E-ERA4-FIXTURE
// posture gate. See the file header for what it closes and what it measured.
func TestIssuerKeyBindingResolvesInTheObjectiveBondedEpochPosture(t *testing.T) {
	const epochBlocks = 4
	nodes, ids, net, _, _ := era4EpochNet(t, 4, epochBlocks)
	all := make([]ports.NodeID, len(ids))
	for i, id := range ids {
		all[i] = id.NodeID()
	}
	issuer, key, epoch := commitBindingInThePosture(t, nodes, ids, net, all, epochBlocks)

	// ---- ACCEPT. The served key resolves against the committed binding.
	fetcher := nodes[1]
	pinned, err := pinFrom(t, fetcher, net, issuer.id)
	if err != nil || pinned != 1 {
		t.Fatalf("ACCEPT arm: pinned %d err %v — the committed binding for epoch %d must resolve", pinned, err, epoch)
	}
	ks := fetcher.DemandIssuerKeyset(issuer.id)
	if ks == nil || ks.Key(epoch) == nil {
		t.Fatalf("ACCEPT arm: the pinned keyset holds no key at epoch %d", epoch)
	}
	if demand.KeyFingerprint(ks.Key(epoch)) != demand.KeyFingerprint(&key.PublicKey) {
		t.Fatal("ACCEPT arm: the held key is not the committed one")
	}
	if pub, e, ok := fetcher.ResolvedDemandIssuerKey(issuer.id); !ok || e != epoch ||
		demand.KeyFingerprint(pub) != demand.KeyFingerprint(&key.PublicKey) {
		t.Fatalf("ACCEPT arm: ResolvedDemandIssuerKey gave (epoch %d, ok %v), want the committed key at epoch %d", e, ok, epoch)
	}

	// ---- REFUSE-A, ABSENT. A second issuer serves a key it never committed. Same
	// fixture, same chain-bearing fetcher, same wire call.
	absent := nodes[2]
	absentKey, kerr := rsa.GenerateKey(rand.Reader, 2048)
	if kerr != nil {
		t.Fatalf("absent issuer key: %v", kerr)
	}
	absent.SetDemandIssuerKey(rand.Reader, epoch, absentKey)
	if _, ok := absent.chain.IssuerKeyCommitment(absent.id, epoch); ok {
		t.Fatal("REFUSE-A setup: this issuer's binding must NOT be committed")
	}
	pinnedA, errA := pinFrom(t, fetcher, net, absent.id)
	if errA != nil {
		t.Fatalf("REFUSE-A arm: the issuer must ANSWER and be refused on the binding, not fail transport: %v", errA)
	}
	if pinnedA != 0 {
		t.Fatalf("REFUSE-A arm: pinned %d against an issuer with NO committed binding, want 0", pinnedA)
	}

	// ---- REFUSE-M, MISMATCH. The bound issuer rotates its key for the SAME epoch.
	// applyIssuerKeys is first-write-wins, so the commitment still names the OLD
	// fingerprint and the served key is off-commitment — the targeting shape.
	rotated, rerr := rsa.GenerateKey(rand.Reader, 2048)
	if rerr != nil {
		t.Fatalf("rotated key: %v", rerr)
	}
	issuer.demandIssuers[epoch] = blindtoken.NewIssuer(rand.Reader, rotated)
	if fp, ok := issuer.chain.IssuerKeyCommitment(issuer.id, epoch); !ok ||
		fp == demand.KeyFingerprint(&rotated.PublicKey) {
		t.Fatal("REFUSE-M setup: the commitment must still name the ORIGINAL key")
	}
	pinnedM, errM := pinFrom(t, nodes[3], net, issuer.id)
	if errM != nil {
		t.Fatalf("REFUSE-M arm: the issuer must ANSWER and be refused on the fingerprint: %v", errM)
	}
	if pinnedM != 0 {
		t.Fatalf("REFUSE-M arm: pinned %d against an OFF-COMMITMENT key, want 0", pinnedM)
	}

	// The discrimination IS the assertion: one fixture, three arms, a chain under the
	// fetcher in all three.
	if pinned == pinnedA || pinned == pinnedM {
		t.Fatalf("no discrimination: accept %d, absent %d, mismatch %d", pinned, pinnedA, pinnedM)
	}
}
