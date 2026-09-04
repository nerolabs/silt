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
//
// REAL SERIALS (PE ruling @ 271ab81 §4, correction 1, 2026-09-03). Every redeem used
// to pass a NIL serial, and `RedeemDeliveryCreditReason` short-circuits its whole
// R0.4b guard on `if len(serial) > 0`. So this fuzz — cited as evidence for the
// durable paid-serial guard — never touched `paidSerial`, the epoch watermark, or the
// expiry sweep at all. It now drives a UNIQUE 32-byte serial per redeem on a moving
// epoch clock, which puts the guard's admission, watermark advance and per-epoch sweep
// under the same adversarial churn as provOrder, and adds invariant (d).
//
// The epoch clock advances every fuzzEpochEvery steps so the sweep retires expired
// entries: with W = paidSerialWindow the live set is bounded by the redeems of the
// last W+1 epochs, far under maxPaidSerial, so an HONEST unique in-window serial must
// always pay. That is asserted, not assumed — a refusal here would mean the guard is
// declining honest customers, and it would also silently break the conservation
// arithmetic below.

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
		// fuzzEpochEvery: steps per demand epoch. Chosen so the guard's live set —
		// the redeems of the last paidSerialWindow+1 epochs — stays two orders of
		// magnitude under maxPaidSerial (65,536), which keeps every honest serial
		// admissible while still exercising the watermark advance and the sweep
		// hundreds of times per scenario.
		fuzzEpochEvery = 1_000
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

	// R2.10 (PE ruling RULING-R2.10-F8-build-178ff3b F1): the ledger's clock is an
	// injected source, not a call parameter. Before R2.10 this fuzz drove the watermark
	// through RedeemDeliveryCreditReason's currentEpoch argument; the migration left the
	// moving `epoch` bound only to issuedEpoch, so the watermark stayed 0, the band-advance
	// sweep never ran, and invariant (d) went RED past 32,768 live serials — invisible under
	// -short. The source below is the fuzz's clock; every point that moves `epoch` moves it.
	clock := &mockEpochSource{}
	l.SetEpochSource(clock)

	// expectedTotal tracks the conservation quantity: Σbalances + Σescrow.
	// Updated by explicit delta accounting at each operation.
	expectedTotal := int64(0)

	// liveNet tracks per pool index the net+skim currently in the provisional map.
	type mintEntry struct {
		net  int64
		skim int64
	}
	liveMints := make(map[int]*mintEntry)

	// The last serial this run actually paid on, for the replay assertion after the
	// loop. Kept out of the loop's accounting: ReasonAlreadyPaid returns ABOVE the
	// supersede, so a replay moves no value and cannot perturb conservation.
	type paidRecord struct {
		serial []byte
		epoch  uint64
		req    ports.NodeID
		obj    ports.Hash
	}
	var lastPaid paidRecord
	var havePaid bool

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
		k := provKey{server: server, requester: ln.req, root: ln.obj}

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
						ek := provKey{server: server, requester: pl.req, root: pl.obj}
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
							ek := provKey{server: server, requester: pl.req, root: pl.obj}
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
				// A UNIQUE, in-window serial: the honest case. It must ALWAYS pay —
				// the guard exists to refuse a re-presented serial, never a new one.
				epoch := uint64(step / fuzzEpochEvery)
				clock.e = epoch // the ledger's clock advances with the scenario (F1)
				serial := mintFuzzSerial(rng)
				paid, reason := l.RedeemDeliveryCreditReason(server, ln.req, ln.obj, serial, epoch)
				if paid <= 0 {
					t.Fatalf("seed=%#x step=%d: an HONEST unique in-window serial was REFUSED "+
						"(paid=%d reason=%q, epoch=%d, guard holds %d of %d). The paid-serial "+
						"guard must bound a REPLAY, never an honest customer",
						seed, step, paid, reason, epoch, len(l.paidSerial), maxPaidSerial)
				}
				lastPaid = paidRecord{serial: serial, epoch: epoch, req: ln.req, obj: ln.obj}
				havePaid = true
				expectedTotal += int64(fee) - (em.net + em.skim)
				delete(liveMints, idx)

				// Assert (d): the guard stays bounded. It is swept on the epoch
				// advance, so it must never approach its cap on this workload — if it
				// does, the sweep has stopped running and the next honest redeem is
				// one step from a paid-serial-guard-full refusal.
				if len(l.paidSerial) > maxPaidSerial/2 {
					t.Fatalf("seed=%#x step=%d: the paid-serial guard holds %d entries "+
						"(cap %d) at epoch %d — the per-epoch expiry sweep is not retiring "+
						"expired serials, and guard-full refusals are imminent",
						seed, step, len(l.paidSerial), maxPaidSerial, epoch)
				}
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
						ek := provKey{server: server, requester: pl.req, root: pl.obj}
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

	// TRIPWIRE (PE ruling F1, the coupling): this fuzz's header claims it exercises the
	// watermark advance and the per-epoch sweep. A migration that silently unbinds the
	// clock makes that claim false while every assertion still passes under -short, which
	// is how F1 shipped. Assert the sweep actually ran whenever the scenario spans more
	// than the guard's window, so the claim is checked at the tier that runs by default.
	if epochsSpanned := uint64(ops / fuzzEpochEvery); epochsSpanned > paidSerialWindow {
		if l.SerialSweeps() == 0 {
			t.Fatalf("seed=%#x: the scenario spanned %d epochs (window %d) but the expiry sweep NEVER ran — the ledger's epoch source is not wired to the scenario's clock", seed, epochsSpanned, paidSerialWindow)
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

	// Assert (d), second half: the guard REFUSES the serial it just paid on, and the
	// refusal costs nothing. ReasonAlreadyPaid returns above the supersede, so the
	// re-presentation must move no value at all — re-checking conservation is what
	// proves that, rather than trusting the reason string.
	if !havePaid {
		t.Fatalf("seed=%#x: no redeem ever paid — the guard assertions below are vacuous", seed)
	}
	clock.e = uint64(ops / fuzzEpochEvery) // the final re-presentation runs at the last epoch (F1)
	paid, reason := l.RedeemDeliveryCreditReason(server, lastPaid.req, lastPaid.obj,
		lastPaid.serial, lastPaid.epoch)

	if paid != 0 || reason != ReasonAlreadyPaid {
		t.Fatalf("seed=%#x: re-presenting an ALREADY-PAID serial paid %d with reason %q, "+
			"want 0 / %q — one token, one conserved payout",
			seed, paid, reason, ReasonAlreadyPaid)
	}
	if again := sumLedger(); again != got {
		t.Fatalf("seed=%#x: refusing a replayed serial moved the ledger by %+d — "+
			"ReasonAlreadyPaid returns ABOVE the supersede precisely so that it cannot",
			seed, again-got)
	}
}

// mintFuzzSerial draws a fresh 32-byte serial from the SEEDED rng, so a failure is
// reproducible from the seed alone — the property the rest of this fuzz is built on.
func mintFuzzSerial(rng *rand.Rand) []byte {
	s := make([]byte, 32)
	for i := range s {
		s[i] = byte(rng.Intn(256))
	}
	return s
}
