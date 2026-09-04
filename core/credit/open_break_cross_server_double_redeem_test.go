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
// # CLOSED — the gate is FLIPPED (R0.4b, 2026-09-02)
//
// This test asserted the (K-1)·fee mint while the break was live. The fix has
// landed and the assertion is now the CONSERVATION assertion the comment above
// always named as the flip condition: Σ(balances)+Σ(escrow) is UNCHANGED by the
// attack, at every K.
//
// The close is NOT a gate keyed on (fetcher, object) — that would have denied a
// legitimate second delivery of the same object to the same fetcher. It is keyed on
// the TOKEN SERIAL: one demand token (one blind withdrawal, one serial, one fee)
// funds exactly ONE conserved payout, so the K colluding servers' K receipts —
// which all name the SAME serial, because they share one token — collapse to one
// payout. The distinguisher is completed server-distinct redeems off one serial,
// not "was the token reused", so honest abort-retry (a NEW completion, still the
// first redeem of that serial) is untouched.
//
// The serial guard is bounded, and its EVICTION is expiry-only — see
// delivery_serial_evict_regression_test.go for why a FIFO-bounded guard re-opened
// this same pump in a self-financing form.
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
			// THE SHARED SERIAL is what makes this the cross-server attack: the K
			// colluders hold ONE token, so all K receipts name the same serial.
			sharedSerial := []byte("one-token-K-colluding-servers")
			paid := make([]int64, tc.k)
			for i, srv := range servers {
				paid[i] = l.RedeemDeliveryCredit(id(srv), fetcher, obj, sharedSerial, 0)
			}

			gotTotal := sumLedger()

			// CONSERVATION assertion (the flip): one fee in, one fee distributed,
			// net zero. Nothing is minted at any K.
			skim := int64(fee) * SkimNum / SkimDen
			wantPayout := int64(fee) - skim
			wantTotal := initial
			brokenTotal := initial + int64(tc.k-1)*int64(fee)

			if gotTotal != wantTotal {
				t.Fatalf("CROSS-SERVER DOUBLE-REDEEM PUMP OPEN at K=%d:\n"+
					"  sum(balances+escrow)=%d, want %d (conserved), minted %+d\n"+
					"  the pre-fix broken value was %d ((K-1)*fee)\n"+
					"  paid per server: %v\n"+
					"  K servers sharing ONE token must collapse to ONE conserved payout.",
					tc.k, gotTotal, wantTotal, gotTotal-wantTotal, brokenTotal, paid)
			}

			// Exactly ONE server collected, and it collected exactly fee-skim. The
			// aggregate check above cannot see a re-distribution among the K servers
			// that happens to sum right, so pin the per-call shape too.
			var payers int
			for i, p := range paid {
				switch p {
				case 0:
					// refused: this serial already funded its one payout
				case wantPayout:
					payers++
				default:
					t.Fatalf("K=%d: server %d paid %d - want either 0 (refused) or fee-skim=%d",
						tc.k, i, p, wantPayout)
				}
			}
			if payers != 1 {
				t.Fatalf("K=%d: %d servers collected a payout off ONE token, want exactly 1 (paid: %v)",
					tc.k, payers, paid)
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
