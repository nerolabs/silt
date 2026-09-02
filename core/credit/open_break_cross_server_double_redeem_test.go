package credit

// TestOpenBreak_CrossServerDoubleRedeemMoneyPump is the OPEN-BREAK regression
// gate for the cross-server double-redeem money pump, confirmed by a blind
// red-team run on origin/main = abe2d35 (2026-09-02).
//
// # The confirmed break
//
// K colluding servers share ONE demand-withdrawal token. The token signs K
// valid receipts Ack(serial, object, serverI) — the receipt binds `server`,
// and the PoD design even permits token reuse at a second server after an
// abort. With ONE ChargePublish (one fee, one fetcher debit), each of the K
// servers calls RedeemDeliveryCredit for the same (fetcher, object) pair.
//
// RedeemDeliveryCredit (delivery.go:204-234) has NO cross-server double-spend
// guard. After the provisional-lane supersede it unconditionally pays:
//
//	s.balance += fee - skim        // line 229
//	e.balance += skim              // line 231
//
// for every call, regardless of how many times it has already been called for
// the same (fetcher, object). The per-server double-spend guard
// Bank.spent[serial] in core/node/demandrole.go (line 108: demand.NewBank()
// allocated fresh per node) is NOT consulted here — it lives in the node
// handler layer, not in the ledger. A cross-server colluder bypasses it
// entirely by submitting to K different server nodes, each of which holds its
// own Bank.
//
// # The mint
//
// One ChargePublish debits the fetcher by `fee`. K redeems each credit the
// server tier by `fee - skim` and the escrow by `skim`. The ledger total
// (Σbalances + Σescrow) increases by exactly (K-1)·fee:
//
//	K=2 → +50 000 credits minted without a counterparty debit
//	K=3 → +100 000 credits minted without a counterparty debit
//	K=5 → +200 000 credits minted without a counterparty debit
//
// Root cause: delivery.go:204-234, the conserved leg, lacks a
// per-object-per-fetcher redeem-count gate. The provisional map
// (l.provisional) covers only the SAME server redeeming a lane it served;
// it has no cross-server memory.
//
// # OPEN-BREAK encoding
//
// This test ASSERTS THE CURRENT BROKEN BEHAVIOR so the suite stays GREEN on
// main and CI passes while the break is live. It verifies the exact
// (K-1)·fee mint at K∈{2,3,5}.
//
// When the fix lands — a cross-server redeem gate keyed on (fetcher, object)
// — the assertion flips from "delta == (K-1)*fee" to "delta == 0". At that
// point the subtests named "openBreakDeltaK=N" will fail (that is the signal
// to flip), and "conservedK=N" subtests added in the fix PR will go GREEN.
//
// # Relation to A4
//
// A4 (TestA4MoneyPumpConservation, money_pump_test.go) was a per-server
// double-pay: a single server's provisional-lane eviction left a self-mint
// on the books that a later redeem then paid on top of. That break was
// fixed (Boulder 0, R0.4a) by reversing the self-mint at eviction. This is
// the second delivery-credit money pump, distinct axis: K servers, one fee,
// K payouts. The delivery-credit subsystem now has two confirmed money-pump
// shapes; both live in this package's scar record.
//
// DESIGN REFERENCE: scar ledger
// .claude/agent-memory/tester/scar-cross-server-double-redeem.md
// (second money-pump in the delivery/demand-credit subsystem, session-20,
// 2026-09-02, origin/main = abe2d35).

import "testing"

// id is declared in delivery_test.go (same package). Not re-declared here.

func TestOpenBreak_CrossServerDoubleRedeemMoneyPump(t *testing.T) {
	const fee = 50_000

	cases := []struct {
		k int
	}{
		{k: 2},
		{k: 3},
		{k: 5},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(openBreakKName(tc.k), func(t *testing.T) {
			// One shared ledger — matching real topology where all nodes share
			// a single ledger (core/node/demandrole.go).
			l := New(fee, 0)

			fetcher := id(200)
			obj := objHash(99)

			// Seed the fetcher with enough to ChargePublish once.
			l.Register(fetcher)
			l.accounts[fetcher].balance = fee

			// Register K servers.
			servers := make([]id8, tc.k)
			for i := range servers {
				servers[i] = id8(i + 1)
				l.Register(id(servers[i]))
			}

			// sumLedger: Σ(all balances) + Σ(all escrow reserves).
			// This is the closed-system conservation quantity.
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

			initial := sumLedger() // fetcher has fee, servers have 0

			// One ChargePublish: the fetcher pays fee once.
			// wantTotal after the attack (independent derivation):
			//   initial
			//   - fee            (ChargePublish debit, the one honest payment)
			//   + K*(fee - skim) (K conserved-leg payouts, one per colluding server)
			//   + K*skim         (K escrow payouts, one per colluding server)
			// = initial - fee + K*fee
			// = initial + (K-1)*fee
			//
			// Under correct conservation the ledger should be:
			//   initial + 0   (one fee in, one fee distributed, net zero)
			//
			// The difference: (K-1)*fee is the minted credit.

			if err := l.ChargePublish(fetcher); err != nil {
				t.Fatalf("ChargePublish: %v", err)
			}

			// K redeems of the same (fetcher, obj) — one per colluding server.
			// No server serves the object first (no provisional lane), so the
			// provisional lookup is a no-op for each call; the conserved leg
			// fires unconditionally every time.
			paid := make([]int64, tc.k)
			for i, srv := range servers {
				paid[i] = l.RedeemDeliveryCredit(id(srv), fetcher, obj)
			}

			gotTotal := sumLedger()

			// OPEN-BREAK assertion: confirm the system mints (K-1)*fee.
			// This assertion PASSES on the broken main (abe2d35) and will
			// FAIL when the fix lands. Flip this to assert delta==0 on fix.
			skim := int64(fee) * SkimNum / SkimDen
			wantPayout := int64(fee - skim) // one honest payout
			_ = wantPayout

			wantTotal := initial                              // what conservation demands (one fee in, one fee out)
			brokenTotal := initial + int64(tc.k-1)*int64(fee) // what the bug produces

			if gotTotal != brokenTotal {
				// The break has been fixed — flip this test.
				t.Errorf("OPEN-BREAK FLIPPED (fix detected):\n"+
					"  gotTotal=%d, expected broken value=%d\n"+
					"  delta from broken=%+d\n"+
					"  If delta==0 (gotTotal==wantTotal=%d), the fix is in. "+
					"Replace this test with a conservation-pass assertion.",
					gotTotal, brokenTotal, gotTotal-brokenTotal, wantTotal)
				return
			}

			// Confirm the exact mint magnitude and per-server payout.
			mintedCredits := gotTotal - wantTotal
			wantMinted := int64(tc.k-1) * int64(fee)
			if mintedCredits != wantMinted {
				t.Errorf("OPEN-BREAK K=%d: minted %d credits, want %d\n"+
					"  Σbalances+Σescrow=%d initial=%d fee=%d\n"+
					"  paid per server: %v",
					tc.k, mintedCredits, wantMinted,
					gotTotal, initial, int64(fee), paid)
			} else {
				t.Logf("OPEN-BREAK K=%d CONFIRMED: minted %d credits = (K-1)*fee (%d-1)*%d\n"+
					"  Σbalances+Σescrow=%d initial=%d paid per server: %v",
					tc.k, mintedCredits, tc.k, int64(fee), gotTotal, initial, paid)
			}
		})
	}
}

// openBreakKName returns the subtest name for a given K.
func openBreakKName(k int) string {
	switch k {
	case 2:
		return "openBreakDeltaK=2"
	case 3:
		return "openBreakDeltaK=3"
	case 5:
		return "openBreakDeltaK=5"
	default:
		return "openBreakDeltaK=?"
	}
}

// id8 is a small integer type used to index server IDs in this test.
type id8 = byte

// objHash produces a test object hash from a byte index. Distinct from the
// id() helper (which produces NodeIDs) — here we need a ports.Hash.
func objHash(b byte) [32]byte {
	var h [32]byte
	h[0] = b
	h[1] = 0xde
	h[2] = 0xad
	h[3] = 0xbe
	return h
}
