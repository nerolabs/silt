package credit

// R2.13 — R-COMPACT-ORPHAN. Gates G-CO-2 and G-CO-3 (PE ruling
// RULING-ledger-durability-family-FP2-R2.13-R2.10-2026-09-03.md §6, Tester assignment).
// G-CO-1 lives at adapters/guardstore/r213_compact_orphan_test.go — this file is the
// ledger-tier half.
//
// G-CO-3 AS LITERALLY STATED IS ALREADY GREEN ON MAIN, AND STAYS THAT WAY, NOT A NEW
// GATE HERE. The ruling's simplest contract — "a store whose Append returns a non-nil
// error" ⇒ RedeemDeliveryCreditReason returns ReasonGuardStore, 0 paid — is already
// covered by TestRTC3_GuardEntryIsDurableBeforeTheCreditMoves
// (core/credit/rt_r04b_c3_credit_test.go:116-134, via failingStore, which fails
// Append unconditionally). Re-verified green as part of this work: `go test -run
// TestRTC3_GuardEntryIsDurableBeforeTheCreditMoves` PASS. It is unconditional on
// Compact's own outcome — delivery.go's addPaidSerial already refuses on any Append
// error, regardless of why the store is in that state — so it does not exercise
// R-COMPACT-ORPHAN specifically and is not repeated here.
//
// What IS missing, and is the actual R2.13 ledger-side gap: sweepExpiredSerials
// discards Compact's error UNCONDITIONALLY (delivery.go:558-560, `_ =
// l.paidStore.Compact(...)`), for every class of failure, benign or broken. So today:
//
//   - G-CO-2 (a BENIGN, pre-rename Compact failure must not refuse payouts) holds only
//     because ALL Compact errors are ignored — not because the ledger tells the two
//     classes apart. Written first, per the ruling ("write it before the ledger-side
//     change"), so the anti-over-correction property is pinned before anyone teaches
//     the ledger to read Compact's error at all.
//   - G-CO-3, redirected per the Tester's task brief (the literal form above is
//     already green): a BROKEN store — one left in the orphaned state R-COMPACT-ORPHAN
//     describes, where Append keeps returning nil while its records become
//     unrecoverable — is NOT observable by the ledger today. The redeem that follows
//     reports ReasonPaid while the guard entry it believes it just wrote can never be
//     restored. This is the gap the ledger-side error-class split (ruling §1, "stop
//     discarding the error... only refuse payouts when the store reports itself
//     broken") exists to close.

import (
	"errors"
	"testing"

	"github.com/nerolabs/silt/ports"
)

// benignCompactFailure models a PRE-rename Compact failure (ruling §1: temp-file
// create/write/sync/rename failing before any durable state changes — e.g. a full
// disk). Load and Append are untouched and keep working; only Compact fails, every
// time it is called, so the test can confirm the failing path was actually reached
// (a silently-vacuous gate is worse than no gate — instrumented with `calls`).
type benignCompactFailure struct {
	mem   *memStore
	calls int
}

func (b *benignCompactFailure) Load() ([]ports.PaidSerial, error) { return b.mem.Load() }
func (b *benignCompactFailure) Append(p ports.PaidSerial) error   { return b.mem.Append(p) }
func (b *benignCompactFailure) Compact([]ports.PaidSerial) error {
	b.calls++
	// The store is left COMPLETELY UNCHANGED: this is the "superset log, only ever
	// over-refuses" case (ruling §1's table) that fail-closed-on-any-Compact-error was
	// REFUSED for conflating with a broken store.
	return errors.New("injected: pre-rename compaction failure (disk full)")
}

// TestG_CO2_BenignCompactionFailureDoesNotRefusePayouts is G-CO-2, the
// anti-over-correction gate. It must hold BEFORE the ledger-side error-class split
// lands (today it holds vacuously, because every Compact error is discarded) and
// AFTER (it must hold for the right reason: the store reports itself healthy).
func TestG_CO2_BenignCompactionFailureDoesNotRefusePayouts(t *testing.T) {
	const fee = 50_000
	skim := int64(fee) * SkimNum / SkimDen
	wantPay := int64(fee) - skim

	store := &benignCompactFailure{mem: &memStore{}}
	l := New(fee, 500_000)
	l.SetPaidSerialStore(store)
	if err := l.LoadPaidSerials(); err != nil {
		t.Fatal(err)
	}
	srv, fetcher, obj := id(1), id(2), id(7)
	l.Register(srv)
	l.Register(fetcher)

	// An epoch-0 serial so the later band advance has something to sweep, which is
	// what drives sweepExpiredSerials into calling Compact.
	if paid := l.RedeemDeliveryCredit(srv, fetcher, obj, testSerial(1), 0, 0); paid != wantPay {
		t.Fatalf("setup: epoch-0 redeem must pay %d, got %d", wantPay, paid)
	}

	// The band advances past the window: sweepExpiredSerials removes the epoch-0
	// entry and calls Compact, which this store fails BENIGNLY. The redeem driving
	// that advance must still pay.
	paid, reason := l.RedeemDeliveryCreditReason(srv, fetcher, obj, testSerial(2),
		paidSerialWindow+1, paidSerialWindow+1)
	if paid != wantPay || reason != ReasonPaid {
		t.Fatalf("a BENIGN (pre-rename) compaction failure must not refuse payouts: "+
			"paid=%d reason=%q, want %d/%q. Fail-closed-on-any-Compact-error was REFUSED "+
			"by the ruling precisely for this case: a pre-rename failure leaves a superset "+
			"log that only ever over-refuses, and turning it into a payout refusal is a "+
			"self-inflicted liveness break at exactly the load where compaction fails.",
			paid, reason, wantPay, ReasonPaid)
	}
	if store.calls == 0 {
		t.Fatalf("vacuous gate: the band advance never called Compact, so this test never " +
			"exercised the failing path at all")
	}
}

// brokenAfterCompact models R-COMPACT-ORPHAN itself (ruling §1 / P7) at the ledger's
// integration boundary, not just the adapter's. Compact's rename succeeds — durable
// already holds the correct post-sweep set — but the handle swap after it fails, so
// every Append AFTER that point can no longer reach `durable`.
//
// WHAT "REPORTS ITSELF BROKEN" MEANS, AS SHIPPED (Builder, R2.13). The port contract's
// handle clause (ports.PaidSerialStore.Compact) gives a store exactly two legal
// states after a failed Compact: still appendable-and-reachable, or failing every
// Append. A store in the orphaned state therefore FAILS Append — that is
// adapters/guardstore.ErrStoreBroken, the sticky backstop — and the ledger observes
// brokenness through the one signal it already refuses on (delivery.go, the
// addPaidSerial call: an Append error is ReasonGuardStore, 0 paid). This double
// mirrors that contract: the pre-fix double returned nil from Append while dropping
// the record into `lost`, which is a store VIOLATING the clause; no ledger can be
// sound against a store that lies about durability, and the redirected gate below
// was RED against it precisely because the ledger had no class signal. Under the
// shipped contract the gate is GREEN through the existing ReasonGuardStore path, with
// no new ledger machinery. `lost` is kept so the gate can still assert that nothing
// was silently written past the break.
type brokenAfterCompact struct {
	durable  []ports.PaidSerial // what a fresh Load() would see
	lost     []ports.PaidSerial // what an Append would have written past the break (must stay empty)
	orphaned bool
}

func (b *brokenAfterCompact) Load() ([]ports.PaidSerial, error) {
	return append([]ports.PaidSerial(nil), b.durable...), nil
}

var errStoreBroken = errors.New("injected: store is broken (sticky; the handle clause's second leg)")

func (b *brokenAfterCompact) Append(p ports.PaidSerial) error {
	if b.orphaned {
		b.lost = append(b.lost, p) // recorded only so the gate can prove the ledger never paid on it
		return errStoreBroken
	}
	b.durable = append(b.durable, p)
	return nil
}

func (b *brokenAfterCompact) Compact(live []ports.PaidSerial) error {
	b.durable = append([]ports.PaidSerial(nil), live...) // the RENAME did succeed
	b.orphaned = true                                    // ...the re-open after it did not
	return errors.New("injected: post-rename re-open failed (ruling P7)")
}

// TestG_CO3_BrokenStoreMustBeObservableByTheLedger is the redirected G-CO-3 (see file
// header: the literal form is already green via TestRTC3_GuardEntryIsDurableBefore-
// TheCreditMoves). It drives the real R-COMPACT-ORPHAN sequence through the ledger —
// a band advance that sweeps and compacts, where the compact fails in the orphaning
// way — and asserts the invariant the missing port clause exists to state (ruling §1):
// a redeem the ledger reports as PAID must have a guard entry a fresh Load can
// recover. It was RED on main against a double whose Append returned nil past the
// break (the ledger had no signal beyond Append's own return value). Under the shipped
// contract the double fails Append past the break and the gate closes through
// ReasonGuardStore — see the brokenAfterCompact comment. Ablation recorded in the
// Builder memory: with the double's Append restored to return nil, the gate is RED
// again, which is the port clause's own statement that a lying store is unsound.
func TestG_CO3_BrokenStoreMustBeObservableByTheLedger(t *testing.T) {
	const fee = 50_000

	store := &brokenAfterCompact{}
	l := New(fee, 500_000)
	l.SetPaidSerialStore(store)
	if err := l.LoadPaidSerials(); err != nil {
		t.Fatal(err)
	}
	srv, fetcher, obj := id(1), id(2), id(7)
	l.Register(srv)
	l.Register(fetcher)

	// An epoch-0 serial so the band advance below has something to sweep, driving
	// Compact — which orphans the store's append handle (durable already holds this
	// entry's replacement snapshot post-sweep, i.e. none — it expires).
	if paid := l.RedeemDeliveryCredit(srv, fetcher, obj, testSerial(1), 0, 0); paid == 0 {
		t.Fatalf("setup: epoch-0 redeem did not pay")
	}

	// The band advance: sweeps the epoch-0 entry, calls Compact (orphans the store),
	// and itself redeems a FRESH serial at the new epoch. Its own guard append lands
	// on the now-orphaned handle.
	current := paidSerialWindow + 1
	paid, reason := l.RedeemDeliveryCreditReason(srv, fetcher, obj, testSerial(2), current, current)
	if !store.orphaned {
		t.Fatalf("vacuous gate: the band advance never reached Compact, so the store was "+
			"never orphaned (store.durable=%d)", len(store.durable))
	}

	// The invariant (ruling §1's missing port clause, restated at the ledger): a
	// redeem the LEDGER reports as PAID must have left a guard entry a FRESH Load can
	// recover. If the store paid nothing (refused), that's a different, acceptable
	// outcome (an under-pay) and this check does not apply — but the refusal must be
	// the store's own class, and nothing may have been paid on a record past the break.
	if reason != ReasonPaid {
		if reason != ReasonGuardStore || paid != 0 {
			t.Fatalf("a broken store must refuse as ReasonGuardStore with 0 paid, got paid=%d reason=%q",
				paid, reason)
		}
		if len(store.lost) != 1 {
			t.Fatalf("the redeem must have tried exactly one Append past the break (that is how the "+
				"store reports itself broken), got %d", len(store.lost))
		}
		if l.CompactFailures() != 1 || l.LastCompactError() == nil {
			t.Fatalf("the sweep's Compact error must be recorded, not discarded: failures=%d lastErr=%v",
				l.CompactFailures(), l.LastCompactError())
		}
	}
	if paid > 0 && reason == ReasonPaid {
		fresh, err := store.Load()
		if err != nil {
			t.Fatal(err)
		}
		visible := false
		for _, e := range fresh {
			if string(e.Serial) == string(testSerial(2)) {
				visible = true
			}
		}
		if !visible {
			t.Fatalf("R-COMPACT-ORPHAN at the ledger tier: RedeemDeliveryCreditReason reported "+
				"ReasonPaid (paid=%d) for serial %x, but a fresh Load of the durable store "+
				"never sees that guard entry — the store was silently broken by the prior "+
				"Compact's post-rename re-open failure, and the ledger had no way to know. "+
				"A restart now un-guards this serial: the same wire receipt can be redeemed "+
				"again. The store must report itself broken (e.g. Append fails after an "+
				"orphaning Compact) so the existing ReasonGuardStore path "+
				"(TestRTC3_GuardEntryIsDurableBeforeTheCreditMoves) catches this too.",
				paid, testSerial(2))
		}
	}
}
