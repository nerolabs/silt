package node

// D-BB-BUILD-TAG residual R-BB-BOND-STAMP-TUPLE — this file carries NO build tag,
// because the fact it pins is true in both builds and the enforcement has to live in the
// ordinary (untagged) test job.
//
// WHAT WENT WRONG. Four sites justified leaving account.firstSeenTick's surviving writer
// alone by saying RecordBondChallenge is "stamped from the bond auditor's own request
// counter rather than from a wall clock, and never for a fetcher". A blind review
// measured the opposite on two real bonded validators. The tick is
// uint64(n.clock.Now())+1, and the daemon's node clock is the walltime adapter, so the
// stamp is time.Now().UnixNano()+1 — a wall-clock nanosecond, written for this node's own
// id and for every bonded peer that answers a challenge. An identity that is both a
// bonded peer and a fetcher therefore carries the whole
// (identity, cumulative fetched bytes, first-seen wall-clock nanosecond) tuple in a
// DEFAULT build.
//
// A comment saying so would rot the same way the wrong one did. This measures it.

import (
	"testing"

	"github.com/nerolabs/silt/adapters/identity"
	"github.com/nerolabs/silt/adapters/memstore"
	"github.com/nerolabs/silt/adapters/simclock"
	"github.com/nerolabs/silt/adapters/simnet"
	"github.com/nerolabs/silt/ports"
)

// bondStampClock is a ports.Clock parked at a chosen instant. The test sets it to a real
// Unix-nanosecond reading, because the whole question is whether the stamp carries that
// magnitude or a small counter's.
type bondStampClock struct{ now ports.Time }

func (c *bondStampClock) Now() ports.Time                         { return c.now }
func (c *bondStampClock) AfterFunc(ports.Duration, func()) func() { return func() {} }

// bondTickRecorder captures every tick the bond auditor passes to the ledger. It embeds
// the port interface rather than implementing it, so any method this fixture forgot
// panics loudly instead of silently doing nothing.
type bondTickRecorder struct {
	ports.CreditLedger
	ticks []uint64
}

func (r *bondTickRecorder) RecordBondChallenge(_ ports.NodeID, _ ports.Hash, _ int64, _ bool, tick uint64) {
	r.ticks = append(r.ticks, tick)
}
func (r *bondTickRecorder) Reputation(ports.NodeID) int64 { return 0 }

// TestR29aBondAuditStampsAWallClockNanosecondNotACounter drives the real self-record
// sweep twice, with the clock moved by an hour in between, and reads the ticks the
// auditor handed the ledger.
//
// The two arms are the discriminator. A request counter increments by 1 per sweep. A
// clock increments by the elapsed time. Only the second arm can tell them apart — the
// magnitude alone would pass if someone seeded a counter high.
//
// credit.Ledger.RecordBondChallenge stores the FIRST such tick verbatim into
// account.firstSeenTick (core/credit/credit.go; gated by
// TestR29aBondChallengeStillStampsFirstSeenTick), so what this test measures is the value
// that becomes the `when` in the surviving tuple.
func TestR29aBondAuditStampsAWallClockNanosecondNotACounter(t *testing.T) {
	const t0 = ports.Time(1_788_599_138_518_548_000) // a real walltime.Now() reading
	const hour = int64(3600 * 1e9)

	sched := simclock.New()
	net := simnet.New(sched, 1, simnet.DefaultConfig())
	ident := identity.FromSeed(29)
	clk := &bondStampClock{now: t0}
	rec := &bondTickRecorder{}

	cfg := DefaultConfig()
	cfg.MinBondBytes = 4 << 20
	nd := New(ident.NodeID(), cfg, clk, net.Endpoint(ident.NodeID()), memstore.New())
	nd.SetLedger(rec)
	nd.EnableBond(ident.Signer(), 8<<20)

	nd.AuditBondsOnce()
	clk.now += ports.Time(hour)
	nd.AuditBondsOnce()

	if len(rec.ticks) != 2 {
		t.Fatalf("the auditor recorded %d bond challenges over two sweeps, want 2 — the fixture is not driving the self-record and every assertion below would be vacuous", len(rec.ticks))
	}
	if want := uint64(t0) + 1; rec.ticks[0] != want {
		t.Fatalf("bond-audit tick = %d, want %d (uint64(clock.Now())+1). The stamp is DERIVED FROM THE CLOCK; four D-BB-BUILD-TAG texts once called it the auditor's request counter and were wrong (residual R-BB-BOND-STAMP-TUPLE)", rec.ticks[0], want)
	}
	if rec.ticks[0] < 1_500_000_000_000_000_000 {
		t.Fatalf("bond-audit tick = %d, below Unix-nanosecond magnitude on a wall-clock-valued clock — the daemon hands the node a walltime clock, so this stamp is a wall-clock instant and must read like one", rec.ticks[0])
	}
	if got := int64(rec.ticks[1] - rec.ticks[0]); got != hour {
		t.Fatalf("two sweeps an hour apart produced ticks %d apart, want %d. A request COUNTER would move by 1; this moves by elapsed time, which is what makes the stamp a wall-clock reading and the surviving tuple a (identity, bytes, WHEN)", got, hour)
	}
}
