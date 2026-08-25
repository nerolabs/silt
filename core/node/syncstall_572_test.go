package node

import (
	"fmt"
	"testing"

	"github.com/nerolabs/silt/adapters/simclock"
	"github.com/nerolabs/silt/adapters/simnet"
	"github.com/nerolabs/silt/core/chain"
	"github.com/nerolabs/silt/ports"
)

// #572 — the 027c354-deep val-c catch-up stall, asked deterministically.
//
// FIELD SHAPE (corrected on the issue — NOT a fork): val-c was severed by the
// partition drill, healed, and restarted with its chain store BEHIND
// (restored h0–h23, caught up h24) while its atomically-persisted sign mark
// was AHEAD (mark_height=33 mark_round=1 — it had signed up to h33 before the
// restart; chain.cbor lagged the markstore). Its h24 was the SAME block the
// majority committed (checkpoint 24:b04e6fb7 on val-b/maturer-4/sybil-2),
// the majority stood 12 heights ahead on the same chain, sweeps fired every
// ~30 s — and it never caught up for 100+ minutes. Every failure branch of
// that walk logs at debug or not at all, so the field capture cannot name
// the branch; this repro asks the question in-process on the exact topology
// (12-seat governed mature epoch, epoch rotation inside the gap, mark inside
// the gap past the rotation).
//
// The oracle property: a healed, behind seat on the SAME chain catches up to
// the majority head within one sweep — chain-behind-mark must not block
// ADOPTION (the I2 mark gates SIGNING, never sync). If the control is GREEN
// and the mark shape is RED, the mark→sync coupling is the defect. If both
// are GREEN, the field wedge lives in a dimension this schedule lacks and
// the shipped sync observability (same PR) captures it on the next run.

// world12BehindSeat builds the latched 12-seat world, then advances the
// MAJORITY (11 seats) `ahead` heights past the behind seat by direct
// chain-append of fully-certified era-2 blocks (prepare-QC + precommit from
// every majority seat — well over the frozen-weight threshold), crossing the
// h16 epoch rotation. The behind seat's chain is exactly the majority's
// prefix — the field's severed-then-healed shape, with no live gather needed.
func world12BehindSeat(t *testing.T, ahead uint64) (behind *Node, majority []*Node, majorityIDs []ports.NodeID, net *simnet.Network, sched *simclock.Scheduler) {
	t.Helper()
	nodes, ids, snet, ssched, _ := matureWorld12(t)
	behind = nodes[2] // an anchor seat — the field's val-c

	majority = make([]*Node, 0, len(nodes)-1)
	majorityIDs = make([]ports.NodeID, 0, len(nodes)-1)
	majoritySigners := make([]int, 0, len(nodes)-1)
	for i, nd := range nodes {
		if nd != behind {
			majority = append(majority, nd)
			majorityIDs = append(majorityIDs, nd.id)
			majoritySigners = append(majoritySigners, i)
		}
	}

	_, next := nodes[0].chain.Head()
	for h := next; h < next+ahead; h++ {
		prev, hh := majority[0].chain.Head()
		if hh != h {
			t.Fatalf("drive: head next=%d want %d", hh, h)
		}
		b := &chain.Block{Version: chain.BlockVersion, Height: h, Prev: prev, Entries: []ports.Entry{mkEntry(fmt.Sprintf("572-%d", h))}}
		chain.Sign(b, ids[0].Signer())
		for _, i := range majoritySigners {
			b.PrepareQC = append(b.PrepareQC, chain.AttestAt(b, ids[i].Signer(), 0, chain.PhasePrepare))
		}
		for _, i := range majoritySigners {
			b.Atts = append(b.Atts, chain.AttestAt(b, ids[i].Signer(), 0, chain.PhasePrecommit))
		}
		for mi, nd := range majority {
			if err := nd.chain.Append(*b); err != nil {
				t.Fatalf("drive h%d on majority[%d]: %v", h, mi, err)
			}
		}
	}
	return behind, majority, majorityIDs, snet, ssched
}

func assertCaughtUp(t *testing.T, behind *Node, ref *Node, tag string) {
	t.Helper()
	wantHash, wantNext := ref.chain.Head()
	gotHash, gotNext := behind.chain.Head()
	if gotNext != wantNext || gotHash != wantHash {
		t.Fatalf("#572 REPRODUCED (%s): the healed behind seat did not catch up — head next=%d hash=%x, majority next=%d hash=%x. The field ran this exact shape for 100+ min of sweeps without adopting a block.",
			tag, gotNext, gotHash[:6], wantNext, wantHash[:6])
	}
}

// Control: no mark manipulation — a healed behind seat must catch up in one sweep.
func TestSyncStall_572_Control_BehindSeatCatchesUp(t *testing.T) {
	behind, majority, majorityIDs, net, sched := world12BehindSeat(t, 12)
	net.DisableHeldDelivery()
	done := false
	behind.SyncChain(majorityIDs, func(int, error) { done = true })
	sched.Run()
	if !done {
		t.Fatal("SyncChain did not complete")
	}
	assertCaughtUp(t, behind, majority[0], "control")
}

// The field shape: the seat's sign mark sits INSIDE the gap it must sync
// across (chain h8, mark h17 r1 — the 027c354-deep 24-vs-33 shape). The I2
// mark gates signing only; adoption of committed blocks must be unaffected.
func TestSyncStall_572_BehindChainAheadMark(t *testing.T) {
	behind, majority, majorityIDs, net, sched := world12BehindSeat(t, 12)
	if !behind.recordSign(17, 1, chain.PhasePrecommit, ports.HashBytes([]byte("572-mark"))) {
		t.Fatal("premise: could not set the ahead mark")
	}
	net.DisableHeldDelivery()
	done := false
	behind.SyncChain(majorityIDs, func(int, error) { done = true })
	sched.Run()
	if !done {
		t.Fatal("SyncChain did not complete")
	}
	assertCaughtUp(t, behind, majority[0], "ahead-mark")
	// The mark survives adoption untouched at or above its slot: catching up
	// must not have re-signed anything at or below h17 (I2 stays intact).
	if !behind.signAllowedAt(18, 0, chain.PhasePrepare, ports.HashBytes([]byte("above"))) {
		t.Fatal("mark moved above its slot during sync — adoption must not sign")
	}
	if behind.signAllowedAt(16, 0, chain.PhasePrepare, ports.HashBytes([]byte("below"))) {
		t.Fatal("mark lowered during sync — I2 broken by catch-up")
	}
}

// The observability half: a sweep that ends with ZERO adopted blocks while
// behind must emit the #572 warn naming what every branch did — the exact
// line whose absence made the field stall unattributable. Shape: all peers
// dead, every probe fails, nothing adopted.
func TestSyncStall_572_NoProgressSweepWarns(t *testing.T) {
	behind, _, majorityIDs, net, sched := world12BehindSeat(t, 12)
	lg := &captureLog{clock: sched}
	behind.SetLogger(lg)
	for _, id := range majorityIDs {
		net.Kill(id)
	}
	net.DisableHeldDelivery()
	done := false
	behind.SyncChain(majorityIDs, func(int, error) { done = true })
	sched.Run()
	if !done {
		t.Fatal("SyncChain did not complete")
	}
	for _, ev := range lg.events {
		if ev == "chain sync sweep made NO progress while behind (#572)" {
			return
		}
	}
	t.Fatalf("#572: a no-progress sweep while behind completed SILENTLY — the diagnostic warn is missing (events: %v)", lg.events)
}
