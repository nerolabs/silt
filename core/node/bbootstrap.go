package node

// R2.9a — the node-tier B_bootstrap snapshot.
//
// B_bootstrap is per-requester fetched bytes vs identity age on REAL traffic: the
// measurement D-R2.9-DIRECTION sentence 4 makes a precondition of pinning the
// affordability ratio grant/r. cloudtest measures its own synthetic fetch plan and
// therefore cannot produce it, so the series has to come off a deployment carrying
// real users. This is the node-side read; core/credit/bbootstrap.go is the source and
// carries the privacy reasoning.
//
// The node narrows the ledger snapshot further: it drops the salted requester label
// and publishes (age, bytes) pairs only. What reaches an operator is a scatter of
// "an identity this old had fetched this much" — enough to fit grant/r, and not a
// per-identity series at all.
//
// Instrumentation only: nothing here moves credit, escrow or standing, and it is NOT
// gated on -economy (the counters accrue on every node; only disbursement is gated).

import (
	"sort"

	"github.com/nerolabs/silt/core/credit"
)

// BBootstrapRow is one point of the series: how old an identity was, and how many
// bytes it had fetched by then. Two numbers, deliberately — no identity, no object,
// no clock finer than the epoch (immutable #4).
type BBootstrapRow struct {
	AgeEpochs    uint64 // ledger epoch now − the epoch this identity first touched the ledger
	FetchedBytes int64  // that identity's lifetime fetched bytes
}

// BBootstrapSeries is the whole snapshot: the ledger epoch the ages are measured
// against, how many requesters the ledger knows, whether the row cap dropped a tail,
// and the rows themselves (sorted by age, then bytes).
type BBootstrapSeries struct {
	Epoch      uint64
	Requesters int
	Truncated  bool
	Series     []BBootstrapRow
}

// BBootstrap snapshots the B_bootstrap series from this node's ledger. Empty with no
// ledger wired, or with a ledger that does not implement the export (a test double) —
// the same optional-interface pattern EconomySelf uses, so the port stays the
// consensus-relevant surface. Loop-owned (it reads the ledger); call it on the event
// loop. Reading moves nothing.
func (n *Node) BBootstrap() BBootstrapSeries {
	out := BBootstrapSeries{Series: []BBootstrapRow{}}
	if n.ledger == nil {
		return out
	}
	r, ok := n.ledger.(interface {
		FetchedBytesByRequester() []credit.RequesterFetch
		FetchedRequesters() (int, uint64)
	})
	if !ok {
		return out
	}
	rows := r.FetchedBytesByRequester()
	out.Requesters, out.Epoch = r.FetchedRequesters()
	out.Truncated = out.Requesters > len(rows)
	out.Series = make([]BBootstrapRow, 0, len(rows))
	for _, row := range rows {
		age := uint64(0)
		if row.FirstSeenEpoch < out.Epoch {
			// The ledger epoch is monotone and firstSeenEpoch was stamped from it, so
			// firstSeenEpoch > epoch is unreachable; clamp anyway rather than wrap a
			// uint64 subtraction into a nonsense age.
			age = out.Epoch - row.FirstSeenEpoch
		}
		out.Series = append(out.Series, BBootstrapRow{AgeEpochs: age, FetchedBytes: row.FetchedBytes})
	}
	sort.Slice(out.Series, func(i, j int) bool {
		if out.Series[i].AgeEpochs != out.Series[j].AgeEpochs {
			return out.Series[i].AgeEpochs < out.Series[j].AgeEpochs
		}
		return out.Series[i].FetchedBytes < out.Series[j].FetchedBytes
	})
	return out
}
