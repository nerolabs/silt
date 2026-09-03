package credit

// G-4: a REFUSED witnessed receipt must not leave the server holding the eager
// self-mint (research certification
// R0.4b-C3-composed-close-bc062d0-RESEARCH-CERTIFICATION-2026-09-03, item 4).
//
// THE LEVER THESE GATES CLOSE. RecordServeToObject self-credits 1 credit/byte — the
// unfunded self-mint Boulder 0's conservation rule bans as a standalone subsidy — and
// only the supersede reverses it. Every guard refusal used to return BEFORE the
// supersede, so a refusing server kept 0.875·B while a PAID one kept only fee−skim =
// 43,750. The conserved leg is FLAT and the self-mint is BYTE-PROPORTIONAL, so above
// B = 50,000 bytes refusing beats being paid, and at the tree's stated MINIMUM
// production chunk of 64 MiB it
// beats it by 1,342×. Cap-full is operator-reachable (fill your own guard with junk
// serials; the red team measured the grief cost at exactly the skim), so this was a
// PROFITABLE, OPERATOR-TRIGGERABLE supersede-disable.
//
// The root cause is the flat fee against a byte-proportional mint (residual
// R-FLAT-FEE, a D-POD-KNOBS re-pricing needing its own certification). These gates
// close the LEVER, not the root cause, and they pin the server-side arithmetic so the
// number cannot go stale.

import (
	"testing"

	"github.com/nerolabs/silt/ports"
)

// chunkOf is a distinct chunk id; RecordServeToObject takes one per serve.
func chunkOf(b byte) ports.ChunkID { return ports.ChunkID(id(b)) }

// fullGuardLedger returns a ledger whose paid-serial guard is full of STILL-LIVE
// serials, so the next guarded redeem is refused with ReasonGuardFull.
func fullGuardLedger(t *testing.T, fee int64) *Ledger {
	t.Helper()
	l := New(fee, 0)
	srv, fetcher, obj := id(1), id(2), id(7)
	for i := 0; i < maxPaidSerial; i++ {
		l.RedeemDeliveryCredit(srv, fetcher, obj, testSerial(i), 100, 100)
	}
	return l
}

// TestG4_RefusedReceiptDoesNotKeepTheSelfMint is gate (i)+(iv): a guard-full refusal
// leaves the server at its PRE-SERVE balance — the self-mint reversed, nothing paid —
// at every object size, including 64 MiB (the tree's stated minimum production chunk)
// where the old behaviour
// was worth +58,676,506 over being paid.
func TestG4_RefusedReceiptDoesNotKeepTheSelfMint(t *testing.T) {
	const fee = 50_000
	const paidLeg = fee - fee*SkimNum/SkimDen // 43,750, the conserved payout
	for _, bytesServed := range []int64{1_000, 64 << 10, 64 << 20} {
		l := fullGuardLedger(t, fee)
		srv, fetcher, obj := id(11), id(12), id(13)
		l.Register(srv)
		l.Register(fetcher)

		beforeServe := l.Balance(srv)
		sigmaBefore := sumConserved(l)
		l.RecordServeToObject(srv, fetcher, obj, chunkOf(1), bytesServed)
		selfMint := l.Balance(srv) - beforeServe
		if selfMint <= 0 {
			t.Fatalf("bytes=%d: setup produced no self-mint", bytesServed)
		}

		paid, why := l.RedeemDeliveryCreditReason(srv, fetcher, obj, testSerial(9_000_001), 100, 100)
		if paid != 0 || why != ReasonGuardFull {
			t.Fatalf("bytes=%d: setup expected a guard-full refusal, got paid=%d reason=%q",
				bytesServed, paid, why)
		}

		if gain := l.Balance(srv) - beforeServe; gain != 0 {
			t.Fatalf("bytes=%d: a REFUSED witnessed receipt left the server +%d "+
				"(self-mint %d, conserved leg would have paid %d, so refusing beats "+
				"being paid by %+d). A refusal must reverse the unfunded self-mint, "+
				"not keep it — that is a profitable, operator-triggerable "+
				"supersede-disable on Boulder 0's conservation rule.",
				bytesServed, gain, selfMint, paidLeg, selfMint-paidLeg)
		}
		if sigmaAfter := sumConserved(l); sigmaAfter != sigmaBefore {
			t.Fatalf("bytes=%d: Sigma moved %+d over serve+refused-redeem; the reversal must "+
				"return the skim to the pre-serve state too", bytesServed, sigmaAfter-sigmaBefore)
		}
		// The composed property the certification asks to pin, stated as an
		// inequality so it survives a re-pricing of either leg.
		if refusalGain := l.Balance(srv) - beforeServe; refusalGain > paidLeg {
			t.Fatalf("bytes=%d: a refused receipt nets the server %d, a paid one %d — "+
				"a refused witnessed receipt must never leave the server better off "+
				"than a paid one", bytesServed, refusalGain, paidLeg)
		}
	}
}

// TestG4_TheEconomistsNumber pins the exact arithmetic the lever turned on, so a
// re-pricing of the fee or the skim cannot silently re-open it. At the 64 MiB
// production chunk the refusal path must net the server 0, not +58,676,506.
func TestG4_TheEconomistsNumber(t *testing.T) {
	const fee = 50_000
	const b = int64(64 << 20)
	const wantMint = b - b/8                 // 58,720,256
	const wantPaid = fee - fee/8             // 43,750
	const wantOldLever = wantMint - wantPaid // 58,676,506

	if wantMint != 58_720_256 || wantPaid != 43_750 || wantOldLever != 58_676_506 {
		t.Fatalf("the pricing moved: mint=%d paid=%d lever=%d. Re-derive the G-4 "+
			"argument against the new numbers before touching this gate.",
			wantMint, wantPaid, wantOldLever)
	}

	l := fullGuardLedger(t, fee)
	srv, fetcher, obj := id(21), id(22), id(23)
	l.Register(srv)
	l.Register(fetcher)
	before := l.Balance(srv)
	l.RecordServeToObject(srv, fetcher, obj, chunkOf(2), b)
	if mint := l.Balance(srv) - before; mint != wantMint {
		t.Fatalf("the 64 MiB self-mint is %d, want %d", mint, wantMint)
	}
	if paid, why := l.RedeemDeliveryCreditReason(srv, fetcher, obj, testSerial(9_000_002), 100, 100); paid != 0 || why != ReasonGuardFull {
		t.Fatalf("expected a guard-full refusal, got paid=%d reason=%q", paid, why)
	}
	if got := l.Balance(srv) - before; got != 0 {
		t.Fatalf("at B=64 MiB the refusal path nets the server %+d, want 0 "+
			"(the pre-fix lever was %+d, 1342x the conserved leg)", got, wantOldLever)
	}
}

// TestG4_OneReceiptReversesExactlyOnce is gate (ii). The reversal is subtractive, so
// a double reversal is an under-pay rather than a mint — but it is still wrong, and
// what bounds it is that the supersede DELETES the lane. Presenting the same refused
// receipt again finds no lane and moves nothing.
func TestG4_OneReceiptReversesExactlyOnce(t *testing.T) {
	const fee = 50_000
	l := fullGuardLedger(t, fee)
	srv, fetcher, obj := id(31), id(32), id(33)
	l.Register(srv)
	l.Register(fetcher)

	before := l.Balance(srv)
	sigmaBefore := sumConserved(l)
	l.RecordServeToObject(srv, fetcher, obj, chunkOf(3), 64<<20)

	serial := testSerial(9_000_003)
	if paid, why := l.RedeemDeliveryCreditReason(srv, fetcher, obj, serial, 100, 100); paid != 0 || why != ReasonGuardFull {
		t.Fatalf("first presentation: paid=%d reason=%q", paid, why)
	}
	afterFirst := l.Balance(srv)
	if afterFirst != before {
		t.Fatalf("after one reversal the server is %+d off its pre-serve balance",
			afterFirst-before)
	}
	for i := 0; i < 3; i++ {
		if paid, why := l.RedeemDeliveryCreditReason(srv, fetcher, obj, serial, 100, 100); paid != 0 || why != ReasonGuardFull {
			t.Fatalf("re-presentation %d: paid=%d reason=%q", i, paid, why)
		}
	}
	if got := l.Balance(srv); got != afterFirst {
		t.Fatalf("re-presenting the same refused receipt moved the balance %+d more. "+
			"A witnessed receipt reverses the self-mint EXACTLY ONCE — the lane deletion "+
			"is what bounds it.", got-afterFirst)
	}
	if got := sumConserved(l); got != sigmaBefore {
		t.Fatalf("Sigma moved %+d across four presentations of one receipt", got-sigmaBefore)
	}
}

// TestG4_RecordedTokenDoesNotReverseAgain is the ordering constraint the certification
// REFUTES a naive fix on: ReasonAlreadyPaid must stay ABOVE the supersede. The lane
// key is (server, requester, root) — a lane, not a delivery — so a receipt cannot say
// which serve it acknowledges. If a recorded token could reach the supersede it would
// reverse a RE-SERVED lane's fresh self-mint, for free, on every re-presentation.
func TestG4_RecordedTokenDoesNotReverseAgain(t *testing.T) {
	const fee = 50_000
	l := New(fee, 0)
	srv, fetcher, obj := id(41), id(42), id(43)
	l.Register(srv)
	l.Register(fetcher)
	serial := testSerial(7)

	// First delivery: serve, then redeem. Paid, and the lane's mint is reversed.
	l.RecordServeToObject(srv, fetcher, obj, chunkOf(4), 64<<20)
	if paid, why := l.RedeemDeliveryCreditReason(srv, fetcher, obj, serial, 0, 0); why != ReasonPaid || paid == 0 {
		t.Fatalf("setup: the first redeem must pay, got paid=%d reason=%q", paid, why)
	}

	// A SECOND, fresh serve on the same lane, then the SAME receipt again.
	beforeReserve := l.Balance(srv)
	l.RecordServeToObject(srv, fetcher, obj, chunkOf(4), 64<<20)
	freshMint := l.Balance(srv) - beforeReserve
	if paid, why := l.RedeemDeliveryCreditReason(srv, fetcher, obj, serial, 0, 0); paid != 0 || why != ReasonAlreadyPaid {
		t.Fatalf("re-presentation: paid=%d reason=%q, want 0 / %q", paid, why, ReasonAlreadyPaid)
	}
	if got := l.Balance(srv) - beforeReserve; got != freshMint {
		t.Fatalf("a re-presented RECORDED token reversed the re-served lane's fresh "+
			"self-mint (balance moved %+d, the fresh mint was %+d). ReasonAlreadyPaid "+
			"must return BEFORE the supersede — the guard's own record is what bounds "+
			"double-reversal.", got, freshMint)
	}
}
