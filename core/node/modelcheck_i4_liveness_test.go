package node

import (
	"testing"

	"github.com/nerolabs/silt/adapters/simnet"
	"github.com/nerolabs/silt/core/chain"
	"github.com/nerolabs/silt/ports"
)

// Consensus model-check — the I4 LIVENESS oracle (issue #432).
//
// consensus-invariants.md I4's assert-note names the property: "a connected
// network never suffers a *permanent* non-final stall." This is the
// deterministic repro of the field wedge in runs 9c3777d-73949 ("chain STARVED
// at tip ~6") and 8ae8326-34086 (stall at h6: zero commits for 30+ min).
//
// THE MECHANISM (pre-fix): the #397 never-sign-twice watermark was HEIGHT-ONLY
// and FINAL — once a validator signed any block at height h, it refused every
// other h-block forever; the mark cleared only on a commit the marks
// themselves forbade. One crossed proposer race splitting the anchor
// signatures 2-2 left NO h-block able to reach the #402 strict anchor majority
// (3-of-4): a permanent stall of a connected, all-honest, 0-fault network.
// This oracle was born RED on branch oracle/i4-liveness-wedge (commit 3617b1d,
// failing-first) and turns GREEN with the certified #432 fix on this branch:
// rounds WITH locking (research certification 432-rounds-locking-liveness,
// 2026-08-15) — the (height, ROUND, phase)-scoped watermark plus the
// deterministic sweep-count round advance give the wedged height a next round
// where fresh signatures are honest, and the lock rule keeps the escape from
// re-opening I1 (asserted by the S1/S2 oracles in modelcheck_s1s2_test.go).
//
// Literature (B8): Tendermint's priv_validator_state signs (height, ROUND,
// step); a failed height advances the round so validators may sign again at
// (h, r+1) without equivocation. silt had imported the watermark without the
// round dimension.
func TestModelCheck_I4_WedgedHeightMustRecover(t *testing.T) {
	nodes, ids, net, g, refill := s1s2World(t)
	id := func(i int) ports.NodeID { return ids[i].NodeID() }
	all := []ports.NodeID{id(0), id(1), id(2), id(3)}

	// I5 guard over the whole recovery: the escape must not manufacture
	// slashable evidence — every anchor signs TWO different blocks at h1
	// across this test (its r0 side and the r1 recovery block), which is
	// honest cross-round signing in era 2, never equivocation.
	var honestSlashed bool
	for _, nd := range nodes {
		nd.OnSlash(func(ports.NodeID, uint64) { honestSlashed = true })
	}

	// THE CROSSED RACE at h1 — two proposal paths, disjoint attester targets,
	// so the four anchors' signatures split 2-2: n0 proposes A gathering only
	// n2; n1 proposes B gathering only n3. Neither side can reach the #402
	// strict anchor majority (3 of 4); every anchor's (1, r0, prepare) slot is
	// now marked.
	blkA := &chain.Block{Version: 1, Height: 1, Prev: g.Hash(), Entries: []ports.Entry{mkEntry("publish-A")}}
	blkB := &chain.Block{Version: 1, Height: 1, Prev: g.Hash(), Entries: []ports.Entry{mkEntry("drain-B")}}
	var errA, errB error
	nodes[0].proposeBlock(blkA, []ports.NodeID{id(2)}, all, 0, func(err error) { errA = err })
	drainHeld(t, net, fifo)
	nodes[1].proposeBlock(blkB, []ports.NodeID{id(3)}, all, 0, func(err error) { errB = err })
	drainHeld(t, net, fifo)

	// Wedge precondition: NEITHER side committed (2 sigs < the 3-of-4 anchor
	// majority). If either committed here, the anchor gate is not engaged and
	// this oracle is not testing what it claims (anti-#303).
	for i, nd := range nodes {
		if _, h := nd.Chain().Head(); h != 1 {
			t.Fatalf("setup: node %d committed at the 2-2 race (head=%d) — the anchor majority gate is not engaged; the wedge premise is broken (errA=%v errB=%v)", i, h, errA, errB)
		}
	}

	// The pre-#432 dead ends, kept as evidence the wedge shape is real:
	// (a) same-hash re-proposals re-gather only the side that already signed;
	for _, re := range []struct {
		n    int
		b    *chain.Block
		atts []ports.NodeID
	}{{0, blkA, []ports.NodeID{id(1), id(2), id(3)}}, {1, blkB, []ports.NodeID{id(0), id(2), id(3)}}} {
		nodes[re.n].proposeBlock(re.b, re.atts, all, 0, func(error) {})
		drainHeld(t, net, fifo)
	}
	// (b) fresh proposals at (h1, r0) die at each proposer's own watermark.
	freshRefused := 0
	for i := range nodes {
		c := &chain.Block{Version: 1, Height: 1, Prev: g.Hash(), Entries: []ports.Entry{mkEntry("fresh-" + string(rune('0'+i)))}}
		var ferr error
		nodes[i].proposeBlock(c, all[:0:0], all, 0, func(err error) { ferr = err })
		drainHeld(t, net, fifo)
		if ferr != nil {
			freshRefused++
		}
	}
	if freshRefused != 4 {
		t.Fatalf("setup: expected all 4 fresh (h1, r0) proposals refused at their proposers' own watermarks, got %d/4 — the wedge premise is broken", freshRefused)
	}
	for i, nd := range nodes {
		if _, h := nd.Chain().Head(); h != 1 {
			t.Fatalf("setup: node %d advanced during the dead-end repertoire (head=%d) — premise broken", i, h)
		}
	}

	// THE #432 ESCAPE — the production sweep: pending work + no progress for
	// roundAdvanceSweeps sweeps → deterministic round-changes to r1 → the
	// designated proposer assembles the new-view certificate (no locks were
	// formed at r0 — no side reached a prepare-QC) and proposes a FRESH drain
	// block at round 1, where every anchor's (1, r1, prepare) slot is signable.
	refill()
	sweepRounds(t, net, nil2, nodes)
	refill()
	sweepRounds(t, net, nil2, nodes)
	for _, nd := range nodes {
		nd.SyncChain(all, func(int, error) {})
	}
	drainHeld(t, net, fifo)

	// THE ORACLE (I4 liveness): the wedged height MUST commit — on every
	// replica, the same block, at round 1 (the escape did it, not some
	// accidental r0 completion), with no honest slash.
	var want ports.Hash
	for i, nd := range nodes {
		_, h := nd.Chain().Head()
		if h < 2 {
			t.Fatalf("I4 LIVENESS VIOLATION (the field wedge, runs 9c3777d/8ae8326): node %d is still wedged at h1 (head=%d) — the round escape did not recover the height", i, h)
		}
		blk := nd.Chain().Blocks(1)
		if len(blk) == 0 {
			t.Fatalf("node %d reports head %d but holds no h1 block", i, h)
		}
		if i == 0 {
			want = blk[0].Hash()
		} else if blk[0].Hash() != want {
			t.Fatalf("I1 VIOLATION during recovery: node %d committed %x at h1, node 0 committed %x", i, blk[0].Hash(), want)
		}
		if blk[0].CommitRound != 1 {
			t.Fatalf("VACUOUS: node %d holds h1 committed at round %d, want 1 — recovery did not go through the view-change", i, blk[0].CommitRound)
		}
	}
	if honestSlashed {
		t.Fatal("I5 VIOLATION: the liveness escape manufactured a slash — cross-round different-hash signing must be honest in era 2")
	}
}

// nil2 is the no-hold predicate for sweepRounds.
func nil2(simnet.HeldMsg) bool { return false }
