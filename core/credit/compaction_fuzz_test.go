package credit

// Tester seat — compaction/tombstone adversarial fuzz for Boulder-0 (2026-09-01).
//
// Drives a long serve/redeem/evict sequence with heavy redeem-then-reserve
// (tombstone churn), pushing provOrder past its 2*maxProvisional compaction
// threshold repeatedly.
//
// Three invariants asserted at every step (or every 1000 steps for (a)):
//   (a) conservation: Σbalances + Σescrow == expected ledger sum
//   (b) provOrder bounded: len(provOrder) <= 2*maxProvisional
//   (c) provIndex integrity: every live provisional map key maps to a correct
//       provOrder position; no tombstone reversal ever debits a LIVE re-served
//       lane; compaction preserves the index
//
// Seeds are fixed so failures are deterministic and citable. A failure reports
// the seed and step so the repro is always available.
//
// Run with -short for a fast integrity check (fewer ops, suitable for -race);
// run without -short for the full adversarial stress.

import (
	"fmt"
	"math/rand"
	"testing"

	"github.com/nerolabs/silt/ports"
)

// TestCompactionTombstoneFuzz drives three seeded scenarios against the
// provOrder compaction logic. Scenario A stresses pure tombstone churn
// (poolSize < maxProvisional so eviction never fires). Scenario B stresses
// the eviction-dominant path (poolSize > maxProvisional). Scenario C places
// the pool exactly at the cap (boundary).
func TestCompactionTombstoneFuzz(t *testing.T) {
	// Full op counts for the stress run; short counts for -race.
	opsA, opsB, opsC := 200_000, 100_000, 150_000
	if testing.Short() {
		opsA, opsB, opsC = 20_000, 10_000, 15_000
	}

	type scenario struct {
		name     string
		seed     int64
		ops      int
		poolSize int
	}
	scenarios := []scenario{
		// A: small pool, all ops are tombstone churn — compaction fires
		// repeatedly without the eviction loop.
		{"tombstone-churn-only", 0xDEAD_BEEF_0001, opsA, maxProvisional / 4},
		// B: large pool, eviction fires on most serves (mixed path).
		{"eviction-dominant", 0xDEAD_BEEF_0002, opsB, maxProvisional * 2},
		// C: pool size exactly at cap — the boundary.
		{"pool-at-cap", 0xDEAD_BEEF_0003, opsC, maxProvisional},
	}

	for _, sc := range scenarios {
		sc := sc
		t.Run(sc.name, func(t *testing.T) {
			runCompactionFuzz(t, sc.seed, sc.ops, sc.poolSize)
		})
	}
}

// runCompactionFuzz is the seeded deterministic fuzz body.
func runCompactionFuzz(t *testing.T, seed int64, ops, poolSize int) {
	t.Helper()
	const (
		fee       = 50_000
		serveSize = 256 // bytes per serve
	)

	rng := rand.New(rand.NewSource(seed))

	// Build a pool of (requester, object) lane keys.
	type laneKey struct {
		req ports.NodeID
		obj ports.Hash
	}
	pool := make([]laneKey, poolSize)
	for i := range pool {
		pool[i] = laneKey{
			req: ports.NodeID(ports.HashBytes([]byte(fmt.Sprintf("fuzz-req-%d", i)))),
			obj: ports.HashBytes([]byte(fmt.Sprintf("fuzz-obj-%d", i))),
		}
	}

	server := ports.NodeID(ports.HashBytes([]byte("fuzz-server")))
	chunk := ports.HashBytes([]byte("fuzz-chunk"))

	l := New(fee, 0 /*no auto-grant*/)

	// expectedTotal tracks the conservation quantity: Σbalances + Σescrow.
	// Updated by explicit delta accounting at each operation.
	expectedTotal := int64(0)

	// liveNet tracks per pool index the net+skim currently in the provisional map.
	type mintEntry struct {
		net  int64
		skim int64
	}
	liveMints := make(map[int]*mintEntry)

	sumLedger := func() int64 {
		var total int64
		for _, a := range l.accounts {
			total += a.balance
		}
		for _, e := range l.escrow {
			total += e.balance
		}
		return total
	}

	// verifyProvIndexIntegrity checks structural integrity of provIndex ↔ provOrder.
	// Runs at every step — O(provIndex + provOrder) per call.
	verifyProvIndexIntegrity := func(step int) {
		t.Helper()
		for k, i := range l.provIndex {
			if i < 0 || i >= len(l.provOrder) {
				t.Fatalf("seed=%#x step=%d: provIndex[%v]=%d out of provOrder bounds [0,%d)",
					seed, step, k, i, len(l.provOrder))
			}
			if l.provOrder[i] == nil {
				t.Fatalf("seed=%#x step=%d: provIndex[%v]=%d points to tombstone nil — compaction left stale index",
					seed, step, k, i)
			}
			if *l.provOrder[i] != k {
				t.Fatalf("seed=%#x step=%d: provIndex[%v]=%d points to wrong key %v — provIndex/provOrder desync",
					seed, step, k, i, *l.provOrder[i])
			}
			if _, ok := l.provisional[k]; !ok {
				t.Fatalf("seed=%#x step=%d: provIndex has key %v but provisional map does not — ghost index entry",
					seed, step, k)
			}
		}
		for i, kp := range l.provOrder {
			if kp == nil {
				continue
			}
			idx, ok := l.provIndex[*kp]
			if !ok {
				t.Fatalf("seed=%#x step=%d: provOrder[%d]=%v not in provIndex — provIndex/provOrder desync",
					seed, step, i, *kp)
			}
			if idx != i {
				t.Fatalf("seed=%#x step=%d: provOrder[%d]=%v but provIndex says position %d — provIndex stale after compaction",
					seed, step, i, *kp, idx)
			}
		}
	}

	for step := 0; step < ops; step++ {
		idx := rng.Intn(poolSize)
		ln := pool[idx]
		k := provKey{requester: ln.req, root: ln.obj}

		// Three possible actions weighted: 50% serve, 30% redeem, 20% force-serve.
		action := rng.Intn(10)

		switch {
		case action < 5:
			// SERVE.
			skim := int64(serveSize) * SkimNum / SkimDen
			net := int64(serveSize) - skim

			// Determine the lane that will be FIFO-evicted (if cap is hit).
			var evictedPoolIdx *int
			if _, alreadyLive := l.provisional[k]; !alreadyLive && len(l.provisional) >= maxProvisional {
				for _, kp := range l.provOrder {
					if kp == nil {
						continue
					}
					for pi, pl := range pool {
						ek := provKey{requester: pl.req, root: pl.obj}
						if ek == *kp {
							piCopy := pi
							evictedPoolIdx = &piCopy
							break
						}
					}
					break
				}
			}

			if evictedPoolIdx != nil {
				if em := liveMints[*evictedPoolIdx]; em != nil {
					expectedTotal -= em.net + em.skim
					delete(liveMints, *evictedPoolIdx)
				}
			}

			if em, alreadyLive := liveMints[idx]; alreadyLive {
				em.net += net
				em.skim += skim
			} else {
				liveMints[idx] = &mintEntry{net: net, skim: skim}
			}
			expectedTotal += int64(serveSize)

			l.RecordServeToObject(server, ln.req, ln.obj, chunk, serveSize)

		case action < 8:
			// REDEEM (only if the lane is live).
			if _, ok := l.provisional[k]; !ok {
				// Not live; serve instead. This serve can ALSO force a FIFO
				// eviction when the map is at cap — the same eviction the
				// action<5 / default serve branches predict. Predict it here too
				// so expectedTotal subtracts the evicted lane's reversed
				// self-mint; omitting it under-counts by the reversed mint (the
				// eviction-dominant conservation gap the desync fix uncovered).
				skim := int64(serveSize) * SkimNum / SkimDen
				net := int64(serveSize) - skim
				var evictedPoolIdx *int
				if _, alreadyLive := l.provisional[k]; !alreadyLive && len(l.provisional) >= maxProvisional {
					for _, kp := range l.provOrder {
						if kp == nil {
							continue
						}
						for pi, pl := range pool {
							ek := provKey{requester: pl.req, root: pl.obj}
							if ek == *kp {
								piCopy := pi
								evictedPoolIdx = &piCopy
								break
							}
						}
						break
					}
				}
				if evictedPoolIdx != nil {
					if em := liveMints[*evictedPoolIdx]; em != nil {
						expectedTotal -= em.net + em.skim
						delete(liveMints, *evictedPoolIdx)
					}
				}
				if em, alreadyLive := liveMints[idx]; alreadyLive {
					em.net += net
					em.skim += skim
				} else {
					liveMints[idx] = &mintEntry{net: net, skim: skim}
				}
				expectedTotal += int64(serveSize)
				l.RecordServeToObject(server, ln.req, ln.obj, chunk, serveSize)
				break
			}
			if em := liveMints[idx]; em != nil {
				// Fund the fetcher for ChargePublish so conservation holds.
				l.acct(ln.req).balance += fee
				expectedTotal += fee
				if err := l.ChargePublish(ln.req); err != nil {
					t.Fatalf("seed=%#x step=%d: ChargePublish: %v", seed, step, err)
				}
				expectedTotal -= fee
				_ = l.RedeemDeliveryCredit(server, ln.req, ln.obj)
				expectedTotal += int64(fee) - (em.net + em.skim)
				delete(liveMints, idx)
			}

		default:
			// FORCE-SERVE (always serve this lane regardless of state).
			skim := int64(serveSize) * SkimNum / SkimDen
			net := int64(serveSize) - skim
			var evictedPoolIdx *int
			if _, alreadyLive := l.provisional[k]; !alreadyLive && len(l.provisional) >= maxProvisional {
				for _, kp := range l.provOrder {
					if kp == nil {
						continue
					}
					for pi, pl := range pool {
						ek := provKey{requester: pl.req, root: pl.obj}
						if ek == *kp {
							piCopy := pi
							evictedPoolIdx = &piCopy
							break
						}
					}
					break
				}
			}
			if evictedPoolIdx != nil {
				if em := liveMints[*evictedPoolIdx]; em != nil {
					expectedTotal -= em.net + em.skim
					delete(liveMints, *evictedPoolIdx)
				}
			}
			if em, alreadyLive := liveMints[idx]; alreadyLive {
				em.net += net
				em.skim += skim
			} else {
				liveMints[idx] = &mintEntry{net: net, skim: skim}
			}
			expectedTotal += int64(serveSize)
			l.RecordServeToObject(server, ln.req, ln.obj, chunk, serveSize)
		}

		// Assert (b): provOrder bounded.
		if len(l.provOrder) > 2*maxProvisional {
			t.Fatalf("seed=%#x step=%d: provOrder len=%d > 2*maxProvisional=%d — compaction did not cap the slice",
				seed, step, len(l.provOrder), 2*maxProvisional)
		}

		// Assert (c): provIndex integrity at every step.
		verifyProvIndexIntegrity(step)

		// Assert (a): conservation every 1000 steps.
		if step%1000 == 0 {
			got := sumLedger()
			if got != expectedTotal {
				t.Fatalf("seed=%#x step=%d: conservation VIOLATED: Σ=%d want=%d delta=%+d "+
					"(provOrder=%d provisional=%d provIndex=%d)",
					seed, step, got, expectedTotal, got-expectedTotal,
					len(l.provOrder), len(l.provisional), len(l.provIndex))
			}
		}
	}

	// Final full conservation check.
	got := sumLedger()
	if got != expectedTotal {
		t.Fatalf("seed=%#x final: conservation VIOLATED: Σ=%d want=%d delta=%+d "+
			"(provOrder=%d provisional=%d provIndex=%d)",
			seed, got, expectedTotal, got-expectedTotal,
			len(l.provOrder), len(l.provisional), len(l.provIndex))
	}
}
