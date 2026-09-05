//go:build bbootstrap

package credit

// R2.9a G-BB-24 / Tester gate BB-22 — R-BB-STAMP-BY-ANY-PATH.
//
// TAGGED, BECAUSE THIS IS WHERE THE DEFECT CAN LIVE. The stamp only exists under the
// `bbootstrap` build tag (D-BB-BUILD-TAG): untagged, stampFirstFetch is an empty body
// and "no non-fetch path stamps" is true of a mechanism that is not in the binary. The
// default build's own claim — that nothing writes the age axis at all — is the untagged
// TestR29aDefaultBuildStampsNoFirstTouchOnRegister.
//
// THE DEFECT THIS PINS. The B_bootstrap age axis is specified as time since first
// FETCH. The stamp used to be written in Register, and Register is reached through
// acct() by every ledger path, so it actually recorded first ledger touch by ANY path:
// bond audit (core/node/bondaudit.go:234 -> RecordBondChallenge), PoR grading
// (core/node/por.go:348 -> RecordAudit), bounty payment (PayBounty) and the false-repair
// slash. An identity that is also a DHT participant therefore published an age
// over-stated by however long it had been a peer before it first fetched, unbounded
// above by the ledger's uptime, on the input to a security parameter. Certified in
// R2.9a-instrument-necessity-geometry-bound-and-tail-merging-RESEARCH-CERTIFICATION-2026-09-05
// section 4.

import (
	"testing"

	"github.com/nerolabs/silt/ports"
)

// TestR29aBB22NonFetchLedgerTouchesLeaveNoStamp is the gate. Every non-fetch path is
// driven against a fresh identity, and none of them may stamp the age axis.
//
// It is deliberately driven through the EXPORTED methods rather than through acct()
// directly: acct() is one line and a test against it would pass on a build where the
// stamp had crept back into any single caller.
func TestR29aBB22NonFetchLedgerTouchesLeaveNoStamp(t *testing.T) {
	root := ports.Hash{0xB0}
	cases := []struct {
		name string
		who  ports.NodeID
		do   func(l *Ledger, n ports.NodeID)
	}{
		{"Register", ports.NodeID{0x01}, func(l *Ledger, n ports.NodeID) { l.Register(n) }},
		{"RecordBondChallenge", ports.NodeID{0x02}, func(l *Ledger, n ports.NodeID) {
			l.RecordBondChallenge(n, ports.Hash{0xAA}, 1<<20, true, 7)
		}},
		{"RecordAudit", ports.NodeID{0x03}, func(l *Ledger, n ports.NodeID) {
			l.RecordAudit(n, ports.ChunkID{0x1}, true)
		}},
		{"PayBounty", ports.NodeID{0x04}, func(l *Ledger, n ports.NodeID) {
			funder := ports.NodeID{0xFE}
			l.RecordServe(funder, ports.NodeID{0xFF}, ports.ChunkID{0xFF}, 10_000)
			if err := l.FundEscrow(root, funder, 5_000); err != nil {
				t.Fatalf("FundEscrow: %v", err)
			}
			l.PayBounty(root, n, 500)
		}},
		{"SlashFalseRepair", ports.NodeID{0x05}, func(l *Ledger, n ports.NodeID) { l.SlashFalseRepair(n) }},
		{"Audits", ports.NodeID{0x06}, func(l *Ledger, n ports.NodeID) { l.Audits(n) }},
		{"FetchedBytes", ports.NodeID{0x07}, func(l *Ledger, n ports.NodeID) { l.FetchedBytes(n) }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			clk := &bbClock{now: 1_000}
			l := New(50_000, 0)
			l.SetObservabilityClock(clk, clk.monotonic)
			tc.do(l, tc.who)
			a := l.accounts[tc.who]
			if a == nil {
				t.Fatalf("%s did not even create the account — the fixture is not exercising the path", tc.name)
			}
			if a.firstFetchTick != 0 {
				t.Fatalf("%s stamped the age axis (firstFetchTick=%d). The axis measures time since first FETCH; a bond audit, a PoR grade, a bounty or a slash is not a fetch, and stamping there over-states the age of every identity that is also a DHT participant", tc.name, a.firstFetchTick)
			}
			if a.fetchedBytes != 0 {
				t.Fatalf("%s wrote fetchedBytes=%d — the fixture is driving a fetch path and the case is mislabelled", tc.name, a.fetchedBytes)
			}
		})
	}
}

// TestR29aBB22AgeIsMeasuredFromTheFetchNotTheBondChallenge is the composed gate, and it
// is the one that goes RED on the shipped defect rather than on a rename.
//
// A peer is bond-challenged at t=0 (the node audits its routing table long before that
// peer ever asks for a chunk), then fetches one chunk a whole day later. Its published
// age must be measured from the FETCH — under an hour — not from the challenge.
//
// THE TWO WRITERS RECORD DIFFERENT EVENTS, which is why one shared field could not have
// been fixed by re-pointing the guard. Both are wall-clock readings off the same daemon
// clock (corrected 2026-09-05; an earlier version of this comment called the auditor's
// tick a request counter, and it is not). Tick 7 below is therefore an instant near this
// ledger's start on the SAME axis the census reads — the peer is challenged at boot and
// fetches a day later. Under one shared field guarded on "unset" the fetch stamp never
// fires, the census reads 7, and the peer's published age is the whole day.
func TestR29aBB22AgeIsMeasuredFromTheFetchNotTheBondChallenge(t *testing.T) {
	const (
		hour = int64(3600) * 1e9
		day  = 24 * hour
	)
	clk := &bbClock{now: 1_000}
	l := New(50_000, 0)
	l.SetObservabilityClock(clk, clk.monotonic)

	peer := ports.NodeID{0xD1}
	l.RecordBondChallenge(peer, ports.Hash{0xAA}, 1<<20, true, 7) // t=0: a routing-table audit

	// Nine more requesters so the census clears the minimum-requester floor; they all
	// arrive with the fetcher so the fixture has one cohort, not two.
	clk.now = ports.Time(day)
	for i := 0; i < BBootstrapMinRequesters-1; i++ {
		l.RecordServe(ports.NodeID{0x50}, ports.NodeID{byte(i), 0x29}, ports.ChunkID{}, 4096)
	}
	l.RecordServe(ports.NodeID{0x50}, peer, ports.ChunkID{}, 4096) // the peer's FIRST fetch

	clk.now = ports.Time(day + 30*60*1e9) // half an hour after the fetch
	h := l.bBootstrapSnapshot()
	if h.Requesters != BBootstrapMinRequesters || h.Aged != BBootstrapMinRequesters {
		t.Fatalf("census = %d requesters / %d aged, want %d each — the fixture is not producing the census the gate reads", h.Requesters, h.Aged, BBootstrapMinRequesters)
	}
	// bbAgeEdgeNanos[4] is the one-hour edge. Every identity fetched half an hour ago,
	// so the whole census must sit strictly below it.
	if h.MaxOccupiedAgeEdgeNanos >= bbAgeEdgeNanos[4] {
		t.Fatalf("highest occupied age bucket has lower edge %d ns; every identity fetched %d ns ago, so nothing may reach the one-hour edge %d. The age axis is measuring first ledger TOUCH (the bond challenge a day earlier), not first FETCH — R-BB-STAMP-BY-ANY-PATH", h.MaxOccupiedAgeEdgeNanos, 30*60*int64(1e9), bbAgeEdgeNanos[4])
	}
	if h.AgeExceedsUptime {
		t.Fatalf("ageExceedsUptime set on a clean fixture — a foreign tick unit reached the age axis")
	}
	// And the bond auditor's own field is untouched by any of this: lastBondTick still
	// carries the auditor's tick, which is what retention (DecayStale) reads. It is the
	// ONLY tick a bond challenge writes since G-BB-28 deleted the first-seen stamp.
	if got := l.accounts[peer].lastBondTick; got != 7 {
		t.Fatalf("lastBondTick = %d, want the bond auditor's tick 7 — the R2.9a fetch stamp must not disturb the retention tick", got)
	}
}
