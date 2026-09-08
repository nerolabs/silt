package credit

// G-4, ON THE ANCHORED SESSION LANE: a server that is NOT PAID must never end better
// off than one that is. Research certification
// R0.4b-C3-composed-close-bc062d0-RESEARCH-CERTIFICATION-2026-09-03, item 4.
//
// THE LEVER THESE GATES CLOSE. RecordServeToObject self-credits — the unfunded
// self-mint Boulder 0's conservation rule bans as a standalone subsidy — and only a
// settlement reverses it. If not-being-paid can be worth more than being paid, the
// whole witnessed lane is optional for the server, and the server can trigger the
// no-payment branch itself (fill your own guard with junk anchors; the red team
// measured the grief cost at exactly the skim).
//
// C1 (2026-09-08) — WHAT MOVED, AND WHAT DID NOT. On the retired FLAT leg the property
// was stated as an ORDERING inside one call: every guard refusal below the supersede,
// so a refused receipt "gives up" its self-mint. That ordering does not exist on the
// session lane and must not be re-created: the anchor is spent at OPEN, before any
// service, so a refused OPEN means no session was ever admitted and the server is on
// the unwitnessed bilateral fallback — where keeping the mint is the CERTIFIED
// behaviour (R2.9 cert §2.5, gates B-3/B-9). The economic question is unchanged and is
// asserted here in the form that is true on the lane:
//
//	net(server | not paid)  <  net(server | paid),   at every object size.
//
// Both no-payment routes collapse to the same outcome — keep the mint, receive nothing
// — so one comparison covers a refused open AND a suppressed settlement. The companion
// gate on the paying side is TestDeliveryAcceptStrictlyDominatesSuppressionAtEverySize
// (B-1 / G-1), which pins the same +75 margin at 64 MiB from the other direction.
//
// The root cause of the original break was the FLAT fee against a byte-proportional
// mint (residual R-FLAT-FEE). The lane's payment is byte-proportional too
// (min(count·p, budget)), which is why the lever is closed structurally now and not by
// an ordering rule.
//
// ABLATION (run RED 2026-09-08): re-price the self-mint to 1 credit per byte — the
// pre-G-R212-7 shape the break was measured on — by setting ServeMintBytesPerCredit to
// 1 in numeraire.go. The mint then dwarfs the settlement at every size and
// TestG4_NotBeingPaidIsNeverBetterThanBeingPaid goes RED at the first size with
// "NOT being paid nets the server 7 and being paid nets 1".

import (
	"testing"

	"github.com/nerolabs/silt/ports"
)

// chunkOf is a distinct chunk id; RecordServeToObject takes one per serve.
func chunkOf(b byte) ports.ChunkID { return ports.ChunkID(id(b)) }

// fullGuardLedger returns a ledger whose paid-serial guard is full of STILL-LIVE
// anchors, so the next session open is refused with ReasonGuardFull.
func fullGuardLedger(t *testing.T, fee int64) *Ledger {
	t.Helper()
	l := New(fee, 0)
	srv, fetcher, obj := id(1), id(2), id(7)
	for i := 0; i < maxPaidSerial; i++ {
		paidOnLane(l, srv, fetcher, obj, testSerial(i), 0)
	}
	return l
}

// serveAndSettle serves B bytes on a fresh lane and then either settles it or does
// not, and returns what the server netted against its PRE-SERVE balance plus the Σ
// drift. settle == false is the no-payment case; whether the server was refused at the
// open or chose to suppress the receipt, the outcome is identical, so this one arm
// covers both.
func serveAndSettle(t *testing.T, l *Ledger, srv, fetcher ports.NodeID, obj ports.Hash, chunk ports.ChunkID, serial []byte, b int64, settle bool) (net, sigmaDrift, mint int64) {
	t.Helper()
	l.Register(srv)
	l.Register(fetcher)
	before := l.Balance(srv)
	sigmaBefore := sumConserved(l)
	if settle {
		// The open must come BEFORE the serve, as it does in production: a server
		// serves on a session it has already admitted.
		if face, why := l.SpendDeliveryAnchors(srv, []RelayAnchor{{Epoch: 0, Serial: serial}}); face == 0 {
			t.Fatalf("B=%d: setup open refused (%s)", b, why)
		}
	}
	l.RecordServeToObject(srv, fetcher, obj, chunk, b)
	mint = l.Balance(srv) - before
	if settle {
		budget := l.Fee()
		if _, _, why := l.SettleDelivery(srv, fetcher, obj, ceilDiv(b, DeliveryIncrementBytes), budget, 0); why != ReasonPaid {
			t.Fatalf("B=%d: the settlement returned %q, want %q", b, why, ReasonPaid)
		}
	}
	return l.Balance(srv) - before, sumConserved(l) - sigmaBefore, mint
}

// TestG4_NotBeingPaidIsNeverBetterThanBeingPaid is gate (i)+(iv) in its lane form, at
// every object size including 64 MiB (the tree's stated minimum production chunk),
// where the old flat behaviour was worth +58,676,506 over being paid.
func TestG4_NotBeingPaidIsNeverBetterThanBeingPaid(t *testing.T) {
	const fee = 50_000
	for _, b := range []int64{mintUnit, 8 * mintUnit, 64 << 20} { // sizes that mint (G-R212-7: below 8·Dλ/7 the lane mints nothing yet)
		srv, fetcher, obj := id(11), id(12), id(13)

		// (a) NOT PAID. The guard is full of still-live anchors, so the open is refused
		// and no session exists — the server serves anyway and keeps its self-mint.
		refusedLedger := fullGuardLedger(t, fee)
		refusedLedger.Register(srv)
		refusedLedger.Register(fetcher)
		if paid, why := settleOnLane(refusedLedger, srv, fetcher, obj, testSerial(9_000_001), 0); paid != 0 || why != ReasonGuardFull {
			t.Fatalf("B=%d: setup expected a guard-full refusal, got paid=%d reason=%q", b, paid, why)
		}
		notPaid, refusedDrift, mint := serveAndSettle(t, refusedLedger, srv, fetcher, obj, chunkOf(1), testSerial(9_000_001), b, false)
		if mint <= 0 {
			t.Fatalf("B=%d: setup produced no self-mint", b)
		}
		if notPaid != mint {
			t.Fatalf("B=%d: the unpaid server netted %d, want its self-mint %d", b, notPaid, mint)
		}

		// (b) PAID. A fresh ledger, the same serve, settled out of an anchor budget.
		paidLedger := New(fee, 0)
		paid, paidDrift, _ := serveAndSettle(t, paidLedger, srv, fetcher, obj, chunkOf(1), testSerial(1), b, true)

		if notPaid >= paid {
			t.Fatalf("B=%d: NOT being paid nets the server %d and being paid nets %d — "+
				"a server that is refused or that suppresses the receipt must never end "+
				"better off than one that settles. That is a profitable, "+
				"operator-triggerable opt-out of Boulder 0's conservation rule.",
				b, notPaid, paid)
		}
		// Neither route MINTS beyond what the serve itself already did: the refused
		// route moves Σ by exactly the serve's mint+skim, and the paid route by that
		// plus the credits it settled out of the anchor face the fetcher burned in.
		// (The face itself leaves the system at ChargePublish, which these fixtures do
		// not run — the conservation walk over the whole cycle is B-1 / G-1's.)
		if refusedDrift != mint+objSkim(b) {
			t.Fatalf("B=%d: the refused route moved Σ by %+d, want the serve's own mint+skim %+d", b, refusedDrift, mint+objSkim(b))
		}
		if want := ceilDiv(b, DeliveryIncrementBytes) * DeliveryIncrementCredit; paidDrift != want {
			t.Fatalf("B=%d: the paid route moved Σ by %+d, want exactly the credits it settled out of the anchor budget (%+d)", b, paidDrift, want)
		}
	}
}

// TestG4_TheEconomistsNumber pins the exact arithmetic the lever turned on, so a
// re-pricing of either leg cannot silently re-open it. At the 64 MiB production chunk
// the unpaid server must net 149 and the paid one 224 — a +75 margin for settling,
// not the +58,676,506 the flat leg gave for refusing.
func TestG4_TheEconomistsNumber(t *testing.T) {
	const fee = 50_000
	const b = int64(64 << 20)
	const wantMint = 149  // ⌊7·b/(8·Dλ)⌋ at Dλ = 393,216 (G-R212-7; 58,720,256 at λ = 1)
	const wantPaid = 224  // ⌈b/U⌉ increments settled, gross less the 1/8 skim
	const wantMargin = 75 // the certified accept margin (B-1 / G-1 pins the same number)

	if wantMint != objNet(b) || wantPaid-wantMint != wantMargin {
		t.Fatalf("the pricing moved: mint=%d (objNet %d) paid=%d margin=%d. Re-derive the "+
			"G-4 argument against the new numbers before touching this gate.",
			wantMint, objNet(b), wantPaid, wantPaid-wantMint)
	}

	srv, fetcher, obj := id(21), id(22), id(23)
	refused := fullGuardLedger(t, fee)
	refused.Register(srv)
	refused.Register(fetcher)
	if paid, why := settleOnLane(refused, srv, fetcher, obj, testSerial(9_000_002), 0); paid != 0 || why != ReasonGuardFull {
		t.Fatalf("expected a guard-full refusal, got paid=%d reason=%q", paid, why)
	}
	notPaid, _, mint := serveAndSettle(t, refused, srv, fetcher, obj, chunkOf(2), testSerial(9_000_002), b, false)
	if mint != wantMint || notPaid != wantMint {
		t.Fatalf("the 64 MiB self-mint is %d and the unpaid server nets %d, want %d for both", mint, notPaid, wantMint)
	}
	got, _, _ := serveAndSettle(t, New(fee, 0), srv, fetcher, obj, chunkOf(2), testSerial(2), b, true)
	if got != wantPaid {
		t.Fatalf("at B=64 MiB the paid server nets %+d, want %d (margin %+d over the unpaid %d)",
			got, wantPaid, got-notPaid, notPaid)
	}
}

// TestG4_OneSettlementReversesExactlyOnce is gate (ii) in its lane form. The reversal
// is subtractive, so a double reversal is an under-pay rather than a mint — but it is
// still wrong, and what bounds it is that a FULLY acknowledged settlement DELETES the
// lane and a repeat acknowledgement of the same cumulative count carries a delta of
// zero. Both are asserted: the second settlement finds no lane, and a zero-delta
// settlement moves nothing even while a lane is live.
func TestG4_OneSettlementReversesExactlyOnce(t *testing.T) {
	const fee = 50_000
	l := New(fee, 0)
	srv, fetcher, obj := id(31), id(32), id(33)
	l.Register(srv)
	l.Register(fetcher)

	const b = int64(64 << 20)
	sigmaBefore := sumConserved(l)
	budget, why := l.SpendDeliveryAnchors(srv, []RelayAnchor{{Epoch: 0, Serial: testSerial(3)}})
	if budget == 0 {
		t.Fatalf("setup open refused: %s", why)
	}
	l.RecordServeToObject(srv, fetcher, obj, chunkOf(3), b)
	count := b/DeliveryIncrementBytes + 1

	settled, _, r := l.SettleDelivery(srv, fetcher, obj, count, budget, 0)
	if r != ReasonPaid || settled == 0 {
		t.Fatalf("first settlement: settled=%d reason=%q", settled, r)
	}
	afterFirst := l.Balance(srv)
	if _, live := l.provisional[provKey{server: srv, requester: fetcher, root: obj}]; live {
		t.Fatal("a fully acknowledged lane survived its settlement — the deletion is what bounds the reversal")
	}

	// Re-settling the SAME cumulative count is a delta of zero, three times over.
	for i := 0; i < 3; i++ {
		if _, _, rr := l.SettleDelivery(srv, fetcher, obj, 0, budget-settled, settled); rr != ReasonNoIncrement {
			t.Fatalf("zero-delta re-settlement %d returned %q, want %q", i, rr, ReasonNoIncrement)
		}
	}
	if got := l.Balance(srv); got != afterFirst {
		t.Fatalf("re-settling one receipt moved the balance %+d more. A settlement "+
			"reverses the self-mint EXACTLY ONCE — the lane deletion and the monotone "+
			"count are what bound it.", got-afterFirst)
	}
	// And a FRESH serve on the same lane, re-settled at the same cumulative count, is
	// still a zero delta: it must not reverse the new mint.
	beforeReserve := l.Balance(srv)
	l.RecordServeToObject(srv, fetcher, obj, chunkOf(3), b)
	freshMint := l.Balance(srv) - beforeReserve
	if _, _, rr := l.SettleDelivery(srv, fetcher, obj, 0, budget-settled, settled); rr != ReasonNoIncrement {
		t.Fatalf("zero-delta settlement over a re-served lane returned %q", rr)
	}
	if got := l.Balance(srv) - beforeReserve; got != freshMint {
		t.Fatalf("a zero-delta settlement reversed the re-served lane's fresh self-mint "+
			"(balance moved %+d, the fresh mint was %+d)", got, freshMint)
	}
	if want := sigmaBefore + settled + freshMint + objSkim(b); sumConserved(l) != want {
		t.Fatalf("Σ moved %+d across the whole walk, want the settled credits plus the "+
			"fresh serve's own mint+skim (%+d)", sumConserved(l)-sigmaBefore, want-sigmaBefore)
	}
}

// TestG4_RecordedTokenDoesNotReverseAgain is the ordering constraint the certification
// REFUTES a naive fix on: a token already recorded in the guard must be refused BEFORE
// anything is reversed. The provisional lane key is (server, requester, root) — a lane,
// not a delivery — so a receipt cannot say which serve it acknowledges. If a recorded
// anchor could reach a settlement it would reverse a RE-SERVED lane's fresh self-mint,
// for free, on every re-presentation. On the session lane the refusal is at the OPEN
// (ReasonAlreadyPaid), which is strictly earlier than the flat leg's screen.
func TestG4_RecordedTokenDoesNotReverseAgain(t *testing.T) {
	const fee = 50_000
	l := New(fee, 0)
	srv, fetcher, obj := id(41), id(42), id(43)
	l.Register(srv)
	l.Register(fetcher)
	serial := testSerial(7)

	// First delivery: serve, then redeem. Paid, and the lane's mint is reversed.
	l.RecordServeToObject(srv, fetcher, obj, chunkOf(4), 64<<20)
	if paid, why := settleOnLane(l, srv, fetcher, obj, serial, 0); why != ReasonPaid || paid == 0 {
		t.Fatalf("setup: the first redeem must pay, got paid=%d reason=%q", paid, why)
	}

	// A SECOND, fresh serve on the same lane, then the SAME receipt again.
	beforeReserve := l.Balance(srv)
	l.RecordServeToObject(srv, fetcher, obj, chunkOf(4), 64<<20)
	freshMint := l.Balance(srv) - beforeReserve
	if paid, why := settleOnLane(l, srv, fetcher, obj, serial, 0); paid != 0 || why != ReasonAlreadyPaid {
		t.Fatalf("re-presentation: paid=%d reason=%q, want 0 / %q", paid, why, ReasonAlreadyPaid)
	}
	if got := l.Balance(srv) - beforeReserve; got != freshMint {
		t.Fatalf("a re-presented RECORDED token reversed the re-served lane's fresh "+
			"self-mint (balance moved %+d, the fresh mint was %+d). ReasonAlreadyPaid "+
			"must return BEFORE the supersede — the guard's own record is what bounds "+
			"double-reversal.", got, freshMint)
	}
}
