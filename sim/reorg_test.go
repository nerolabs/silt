package sim

import (
	"testing"

	"github.com/nerolabs/silt/adapters/identity"
	"github.com/nerolabs/silt/adapters/memstore"
	"github.com/nerolabs/silt/adapters/simclock"
	"github.com/nerolabs/silt/adapters/simnet"
	"github.com/nerolabs/silt/core/chain"
	"github.com/nerolabs/silt/core/credit"
	"github.com/nerolabs/silt/core/genesis"
	"github.com/nerolabs/silt/core/node"
	"github.com/nerolabs/silt/ports"
)

// TestPartitionHealsToHeavierFork is the D2 integration test: a network splits,
// each side commits its OWN block at the same height (a fork the old chain
// could never resolve — "first valid block wins, no reorg"), then the partition
// heals and the side holding the SHORTER history reorgs onto the LONGER one over
// the wire, while the longer side does not budge. Consensus reconverges on one
// history.
//
// CORRECTED 2026-09-12, AND THE FIXTURE'S POSTURE IS LOAD-BEARING. This header, and
// the setup comment below, used to explain the outcome by cumulative attestation
// WEIGHT. There is no weight term: `heavier` ranks on Height then head hash and
// reads nothing else (TestO3T_HeavierReadsOnlyHeightAndHeadHash). Group B wins here
// because it is TALLER (height 3 vs 2), not because it is better attested — height
// and weight are confounded in this fixture, so the test could never have told the
// two rules apart (research certification README-BOND-FORKCHOICE-literal-claim-and-
// equivalence-RESEARCH-CERTIFICATION-2026-09-12, residual R-5).
//
// The REORG this test observes is real, and that is a property of the posture, not
// of fork choice: the fixture is chain.DefaultConfig(), which leaves MinBond at 0,
// so the chain is NON-OBJECTIVE and finalityQuorumActive() is false. With the
// finality gate ON, Reconcile admits only forks CONTAINING the committed head, a
// sub-quorum side commits nothing at all, and convergence is by CATCH-UP with
// nothing dropped. Do not read this test as evidence about an objective chain.
//
// The test NAME still says "HeavierFork". It is cited from docs/test-topologies.md
// (:22, :95), so renaming it is a cited-test surface and is left as a residual
// rather than bundled into a text repair.
func TestPartitionHealsToHeavierFork(t *testing.T) {
	const (
		seed = int64(42)
		N    = 10
	)
	ledger := credit.New(50_000, 0)
	repFn := func(n ports.NodeID) int64 { return ledger.Reputation(n) }
	cfg := chain.DefaultConfig() // quorum 3, rep 100
	nodeCfg := node.DefaultConfig()

	sched := simclock.New()
	net := simnet.New(sched, seed, simnet.DefaultConfig())

	var nodes []*node.Node
	var ids []ports.NodeID
	for i := 0; i < N; i++ {
		ident := identity.FromSeed(seed*1000 + int64(i))
		id := ident.NodeID()
		st := memstore.New()
		nd := node.New(id, nodeCfg, sched, net.Endpoint(id), st)
		nd.SetLedger(ledger)
		ch := chain.New(cfg, repFn)
		if gb, _, _, gerr := genesis.Build(st, nil); gerr == nil { // identical genesis on every node
			ch.AppendGenesis(gb)
		}
		nd.EnableChain(ch, ident.Signer())
		// Earn standing via the BOND press (a distinct root per node) so each is a
		// qualified validator — PoR audits no longer mint standing (H1).
		ledger.RecordBondChallenge(id, ports.HashBytes([]byte{byte(i), 0xb0}), 64<<20, true, 1)
		nodes = append(nodes, nd)
		ids = append(ids, id)
	}
	for i := 1; i < N; i++ {
		nodes[i].Bootstrap([]ports.NodeID{ids[0]}, func() {})
	}
	sched.Run()

	// Split: group A = 4 nodes, group B = 6 nodes; the two sides cannot talk.
	groupA, groupB := ids[0:4], ids[4:10]
	net.Partition(groupB...)

	// Group A commits ONE block, group B commits TWO, so B's history is TALLER:
	// height 3 against A's 2. That height gap is what fork choice ranks on. Each
	// block happens to carry exactly `quorum` attestations, but the attestation
	// count is not an input to the ranking and never was — see the header.
	if err := propose(nodes[0], "forkA", groupA[1:4], groupA, cfg.Quorum, sched); err != nil {
		t.Fatalf("group A commit: %v", err)
	}
	if err := propose(nodes[4], "forkB1", groupB[1:4], groupB, cfg.Quorum, sched); err != nil {
		t.Fatalf("group B block 1: %v", err)
	}
	if err := propose(nodes[4], "forkB2", groupB[1:4], groupB, cfg.Quorum, sched); err != nil {
		t.Fatalf("group B block 2: %v", err)
	}

	headA, _ := nodes[0].Chain().Head()
	headB, _ := nodes[4].Chain().Head()
	if nodes[0].Chain().Len() != 2 || nodes[4].Chain().Len() != 3 {
		t.Fatalf("setup: A should hold genesis+1, B genesis+2 (A=%d B=%d)", nodes[0].Chain().Len(), nodes[4].Chain().Len())
	}
	if headA == headB {
		t.Fatal("setup: the partition should have produced two DIFFERENT histories")
	}

	// Heal, and let the SHORTER side reconcile from a peer on the taller history.
	net.ClearPartition()
	if err := runSync(nodes[0], ids[4], sched); err != nil {
		t.Fatalf("A syncing from B: %v", err)
	}

	// Group A reorged onto B's taller history.
	if newHeadA, _ := nodes[0].Chain().Head(); newHeadA != headB {
		t.Fatal("the shorter partition must reorg onto the taller history after healing (ranking is Height then head hash — there is no weight term; this reorg is reachable only because the fixture is legacy posture)")
	}
	if nodes[0].Chain().Len() != 3 {
		t.Fatalf("group A should now hold B's full 2-block history (len=%d)", nodes[0].Chain().Len())
	}
	for _, name := range []string{"forkB1", "forkB2"} {
		if _, ok := nodes[0].Chain().LookupRoot(ports.HashBytes([]byte(name))); !ok {
			t.Fatalf("group A should now hold fork B's entry %q", name)
		}
	}
	if _, ok := nodes[0].Chain().LookupRoot(ports.HashBytes([]byte("forkA"))); ok {
		t.Fatal("group A's abandoned entry must be gone after the reorg")
	}

	// The taller side must NOT switch to the shorter fork.
	if err := runSync(nodes[4], ids[0], sched); err != nil {
		t.Fatalf("B syncing from A: %v", err)
	}
	if headB2, _ := nodes[4].Chain().Head(); headB2 != headB {
		t.Fatal("the taller partition must not adopt the shorter fork")
	}
}

// runSync drives one SyncChain against a single peer to completion.
func runSync(n *node.Node, peer ports.NodeID, sched *simclock.Scheduler) error {
	var serr error
	done := false
	n.SyncChain([]ports.NodeID{peer}, func(_ int, err error) { serr = err; done = true })
	sched.Run()
	if !done {
		return errString("sync never completed")
	}
	return serr
}
