package credit

// R2.14 — the relay-lane prepayment anchor: the LEDGER-tier RED-first gates
// (Tester, 2026-09-04). Binding spec:
// silt-reviews/research/research-outcome/R2.14-relay-prepayment-anchor-CONSTRUCTION-RESEARCH-CERTIFICATION-2026-09-04.md
// §2 (conservation re-derived: Δ Σ_L = settled − Σ face ≤ 0, equality iff fully
// consumed — C-1 withdraws the 2026-09-03 "unchanged" corollary), §2.4 (the six
// doors), §5 (guard window == keyset window is THE security property), §9 (T-1,
// T-2, T-3, T-4, T-5, T-8, T-12). Build shape: advisory §5 step 3
// (SpendRelayAnchors(anchors []RelayAnchor{Epoch, Serial}, current) (face, reason):
// verify-none, guard-check all, reserve k, durable append all, record all, return
// k × l.fee; RedeemRelayCredit(relay, ephID, chainValue, budget) pays
// min(chainValue, budget) to acct(relay) only, never acct(ephID)).
//
// RULES (cert §9): none of these call fund(); T-2/T-4/T-5 burn through the REAL
// ChargePublish path. The buyer is the fetcher's DURABLE identity holding the
// shipped faucet grant (500,000, cmd/silt/daemon.go) — the only honest source.
// The ledger-total oracle is sumConserved (money_pump_test.go's shape), never a
// pair total (R-RELAY-ORACLE, closed by T-2).
//
// ABLATIONS that must redden (cert §9): remove the all-or-nothing check (T-3
// all_or_nothing / T-10); restore budget := S × inc (T-4); touch acct(ephID) (T-1).

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"encoding/binary"
	"errors"
	"fmt"
	"testing"

	"github.com/nerolabs/silt/core/blindtoken"
	"github.com/nerolabs/silt/core/demand"
	"github.com/nerolabs/silt/ports"
)

const (
	r214Fee   = int64(50_000)  // the shipped --fee; face = Fee() (cert §2.1)
	r214Grant = int64(500_000) // the shipped faucet grant (cmd/silt/daemon.go) — the durable buyer's honest source
	// r214SMax is relaypay.MaxChainLength × RelayIncrementCredit: the value of a
	// FULLY paid maximum-length session (262,144). A literal so this package does
	// not import core/relaypay for one number.
	r214SMax = int64(262_144)
)

// anchorSerial is a deterministic 32-byte serial (blindtoken.SerialSize) for anchor i.
func anchorSerial(i int) []byte {
	s := make([]byte, blindtoken.SerialSize)
	copy(s, "r214-anchor-serial")
	binary.BigEndian.PutUint64(s[24:], uint64(i))
	return s
}

// anchorsAt builds k (epoch, serial) anchors with serials from..from+k-1.
func anchorsAt(epoch uint64, from, k int) []RelayAnchor {
	out := make([]RelayAnchor, 0, k)
	for i := 0; i < k; i++ {
		out = append(out, RelayAnchor{Epoch: epoch, Serial: anchorSerial(from + i)})
	}
	return out
}

// buyAnchors is the issuance burn: k refusable ChargePublish debits on the PAYING
// ledger against the durable buyer (the exact path answerDemandTokenRequest →
// tokenChargeFor → ChargePublish takes on the wire, cert §2.1). It returns the k
// anchors the relay would have blind-signed for those fees.
func buyAnchors(t *testing.T, l *Ledger, buyer ports.NodeID, epoch uint64, from, k int) []RelayAnchor {
	t.Helper()
	for i := 0; i < k; i++ {
		if err := l.ChargePublish(buyer); err != nil {
			t.Fatalf("issuance burn %d/%d: ChargePublish(buyer) = %v (buyer balance %d, fee %d)", i+1, k, err, l.Balance(buyer), l.Fee())
		}
	}
	return anchorsAt(epoch, from, k)
}

func minI64(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}

// TestRelaySettlementRefusesUnanchoredSession is T-1 (cert §9; GREEN by the R0.7
// interim, kept as the ablation guard for "touch acct(ephID)"): shipped grant, no
// anchors — budget 0 because Σ face of zero spent anchors is 0 — ⇒ paid == 0, the
// relay's balance is unchanged, the ephemeral is never registered (white-box
// l.accounts), and the ledger total does not move.
func TestRelaySettlementRefusesUnanchoredSession(t *testing.T) {
	l := New(r214Fee, r214Grant)
	relay, eph := id(1), id(2)
	l.Register(relay)
	before := sumConserved(l)
	relayBefore := l.Balance(relay)

	paid := l.RedeemRelayCredit(relay, eph, r214SMax, 0)
	if paid != 0 {
		t.Fatalf("an unanchored session (budget = Σ face = 0) settled %d, want 0 — RT-RELAY-1's per-session mint", paid)
	}
	if got := l.Balance(relay); got != relayBefore {
		t.Fatalf("relay balance moved %d → %d on an unanchored settlement", relayBefore, got)
	}
	if _, ok := l.accounts[eph]; ok {
		t.Fatalf("the fresh ephemeral was registered by an unanchored settlement — a phantom faucet balance was conjured (the acct(ephID) fiction)")
	}
	if got := sumConserved(l); got != before {
		t.Fatalf("ledger total moved %d → %d on an unanchored settlement", before, got)
	}
}

// TestRelayLaneConservesTotalSupplyOnOnePerNodeLedger is T-2 (cert §9, CORRECTED
// per C-1): on ONE per-node ledger, through the REAL ChargePublish burn × k, then
// open (SpendRelayAnchors) → pay c → settle:
//
//	after ≤ before
//	after − before == settled − k·fee            (exactly)
//	settled ≤ k·fee                              (INV-RELAY-CONS)
//	settled == min(c, k·fee)                     (the pay rule)
//	after == before  ⇔  min(c, k·fee) == k·fee   (equality iff fully consumed)
//
// and the ephemeral is never registered. The unanchored variant leaves the total
// unchanged. The three pair-total tests in relay_test.go are REWRITTEN to this
// oracle, not extended (R-RELAY-ORACLE). RED on main: the interim pays 0, so the
// fully-consumed case has after == before − k·fee.
func TestRelayLaneConservesTotalSupplyOnOnePerNodeLedger(t *testing.T) {
	for _, k := range []int{1, 3, 6} {
		kFee := int64(k) * r214Fee
		for _, c := range []int64{0, 1, kFee - 1, kFee, kFee + 1, r214SMax} {
			t.Run(fmt.Sprintf("k=%d/c=%d", k, c), func(t *testing.T) {
				l := New(r214Fee, r214Grant)
				relay, buyer, eph := id(1), id(2), id(3)
				l.Register(relay)
				l.Register(buyer)
				before := sumConserved(l)

				anchors := buyAnchors(t, l, buyer, 0, 0, k)
				if got := sumConserved(l); got != before-kFee {
					t.Fatalf("after the issuance burn the total is %d, want before − k·fee = %d", got, before-kFee)
				}
				face, reason := l.SpendRelayAnchors(anchors, 0)
				if reason != "" || face != kFee {
					t.Fatalf("SpendRelayAnchors(k=%d) = (face %d, reason %q), want (k·fee = %d, \"\") — face is an identity with the burn (cert §2.1)", k, face, reason, kFee)
				}

				settled := l.RedeemRelayCredit(relay, eph, c, face)
				after := sumConserved(l)
				want := minI64(c, kFee)

				if settled != want {
					t.Fatalf("settled %d for chainValue %d against budget %d, want min(c, k·fee) = %d", settled, c, face, want)
				}
				if settled > kFee {
					t.Fatalf("settled %d > Σ face %d — INV-RELAY-CONS violated (a mint)", settled, kFee)
				}
				if after > before {
					t.Fatalf("ledger total ROSE %d → %d across acquire→open→pay→settle — the banned dual, Δ Σ_L > 0", before, after)
				}
				if after-before != settled-kFee {
					t.Fatalf("Δ Σ_L = %d, want settled − k·fee = %d − %d = %d (cert §2.1, C-1)", after-before, settled, kFee, settled-kFee)
				}
				if (after == before) != (want == kFee) {
					t.Fatalf("after == before is %v but full consumption is %v — equality must hold iff min(c, k·fee) == k·fee (C-1)", after == before, want == kFee)
				}
				if _, ok := l.accounts[eph]; ok {
					t.Fatalf("the ephemeral was registered by the settlement — acct(ephID) was touched")
				}
			})
		}
	}

	t.Run("unanchored total unchanged", func(t *testing.T) {
		l := New(r214Fee, r214Grant)
		relay, eph := id(1), id(2)
		l.Register(relay)
		before := sumConserved(l)
		if settled := l.RedeemRelayCredit(relay, eph, r214SMax, 0); settled != 0 {
			t.Fatalf("unanchored settle paid %d, want 0", settled)
		}
		if got := sumConserved(l); got != before {
			t.Fatalf("unanchored settle moved the total %d → %d", before, got)
		}
	})
}

// TestRelayCredentialIsSpentOncePerLedger is T-3 (cert §9), ledger tier: a second
// spend of the same (epoch, serial) is refused; a batch containing one spent anchor
// is refused ALL-OR-NOTHING with nothing recorded (T-10's ledger half); and the
// guard at its cap of LIVE entries REFUSES, never evicts (G-A2 — the R0.4b lesson:
// eviction of a live entry is a second payout). The two-relay variant (relay B
// refuses relay A's anchor, G-A5) is a node-tier property and lives in core/node.
func TestRelayCredentialIsSpentOncePerLedger(t *testing.T) {
	t.Run("second_spend_refused", func(t *testing.T) {
		l := New(r214Fee, 0)
		a := anchorsAt(0, 0, 1)
		if face, reason := l.SpendRelayAnchors(a, 0); face != r214Fee || reason != "" {
			t.Fatalf("first spend: (face %d, reason %q), want (%d, \"\")", face, reason, r214Fee)
		}
		if face, reason := l.SpendRelayAnchors(a, 0); face != 0 || reason == "" {
			t.Fatalf("SECOND spend of the same anchor: (face %d, reason %q) — the credential was spent twice on one ledger", face, reason)
		}
		// Under a fresh ephemeral/session on a later epoch inside the window, still spent.
		if face, _ := l.SpendRelayAnchors(a, 2); face != 0 {
			t.Fatalf("the same anchor spent again at epoch 2 (face %d) — the guard forgot a live entry", face)
		}
	})

	t.Run("all_or_nothing", func(t *testing.T) {
		l := New(r214Fee, 0)
		a1, a2 := anchorsAt(0, 1, 1)[0], anchorsAt(0, 2, 1)[0]
		if face, _ := l.SpendRelayAnchors([]RelayAnchor{a2}, 0); face != r214Fee {
			t.Fatalf("setup: spending a2 alone gave face %d, want %d", face, r214Fee)
		}
		if face, reason := l.SpendRelayAnchors([]RelayAnchor{a1, a2}, 0); face != 0 || reason == "" {
			t.Fatalf("a batch with an already-spent anchor was accepted (face %d, reason %q) — must refuse all-or-nothing", face, reason)
		}
		// a1 was NOT recorded by the refused batch: it is still spendable.
		if face, reason := l.SpendRelayAnchors([]RelayAnchor{a1}, 0); face != r214Fee {
			t.Fatalf("a1 is no longer spendable after a REFUSED batch (face %d, reason %q) — the refused open recorded an anchor; the fetcher lost anchor 1 because anchor 2 was spent (cert §2.2, T-10)", face, reason)
		}
	})

	t.Run("cap_refuses_never_evicts", func(t *testing.T) {
		l := New(r214Fee, 0)
		var accepted []RelayAnchor
		limit := 2 * maxPaidSerial
		refused := false
		for i := 0; i < limit; i += 6 {
			batch := anchorsAt(0, i, 6)
			if face, _ := l.SpendRelayAnchors(batch, 0); face == 0 {
				refused = true
				break
			}
			accepted = append(accepted, batch...)
		}
		if len(accepted) == 0 {
			t.Fatalf("no anchor batch was ever accepted — SpendRelayAnchors is not built")
		}
		if !refused {
			t.Fatalf("%d live same-epoch anchors were recorded without a refusal — the relay guard is unbounded (the paidSerial cap %d is not applied)", len(accepted), maxPaidSerial)
		}
		for _, idx := range []int{0, len(accepted) / 2, len(accepted) - 1} {
			if face, _ := l.SpendRelayAnchors([]RelayAnchor{accepted[idx]}, 0); face != 0 {
				t.Fatalf("at cap, previously spent anchor #%d became spendable again (face %d) — the guard EVICTED a live entry to make room (the R0.4b eviction pump)", idx, face)
			}
		}
		// A k=6 batch is refused as soon as FEWER than 6 slots remain (all-or-nothing
		// on a k-slot reserve), so the table may still hold up to 5 free slots here
		// (Builder correction, 2026-09-04: 65,536 mod 6 = 4). Fill them one anchor at
		// a time — each k=1 spend must succeed while a slot is free — then the table
		// is EXACTLY full and one more fresh anchor must be refused.
		filled := 0
		for ; filled < 6; filled++ {
			if face, _ := l.SpendRelayAnchors(anchorsAt(0, limit+10+filled, 1), 0); face == 0 {
				break
			}
		}
		if filled >= 6 {
			t.Fatalf("6 single anchors were accepted after a k=6 batch was refused — the batch refusal was not a cap refusal")
		}
		if face, _ := l.SpendRelayAnchors(anchorsAt(0, limit+20, 1), 0); face != 0 {
			t.Fatalf("a fresh anchor was accepted at cap (face %d) — a slot was freed by eviction, not expiry", face)
		}
		if face, _ := l.SpendRelayAnchors(anchorsAt(1, limit+30, 1), 1); face != 0 {
			t.Fatalf("an epoch advance INSIDE the window (0 → 1, W = %d) freed a slot (face %d) — live entries were evicted, not expired", paidSerialWindow, face)
		}
	})
}

// TestRelaySettlementIgnoresForwardedBytesIsBoundedByAnchor is T-4 (cert §9),
// ledger tier: Count() = S_max (the WHOLE chain revealed), forwarded = 0 (the
// ledger cannot see forwarding — that is the whole reason the bound must come
// from the anchor), one anchor ⇒ settled ≤ face, and the ledger total moves at
// settle by exactly settled. RED on main (pays 0); reddens again under the
// ablation "restore budget := S × inc" (settled would be S_max, not face).
func TestRelaySettlementIgnoresForwardedBytesIsBoundedByAnchor(t *testing.T) {
	l := New(r214Fee, r214Grant)
	relay, buyer, eph := id(1), id(2), id(3)
	l.Register(relay)
	l.Register(buyer)
	anchors := buyAnchors(t, l, buyer, 0, 0, 1)
	face, reason := l.SpendRelayAnchors(anchors, 0)
	if face != r214Fee || reason != "" {
		t.Fatalf("SpendRelayAnchors(k=1) = (%d, %q), want (%d, \"\")", face, reason, r214Fee)
	}
	afterSpend := sumConserved(l)

	settled := l.RedeemRelayCredit(relay, eph, r214SMax, face)
	if settled > face {
		t.Fatalf("settled %d > face %d for a fully revealed S_max chain — settlement is bounded by S, not by the anchor (R2.9 G-3 re-opened)", settled, face)
	}
	if settled != face {
		t.Fatalf("settled %d, want min(S_max = %d, face = %d) = %d", settled, r214SMax, face, face)
	}
	if got := sumConserved(l) - afterSpend; got != settled {
		t.Fatalf("the settle moved the ledger total by %d, want exactly settled = %d", got, settled)
	}
	if _, ok := l.accounts[eph]; ok {
		t.Fatalf("the ephemeral was registered by the settlement")
	}
}

// TestRelaySettlementNeverLeavesAnAccountNegative is T-5 (cert §9): a zero-budget
// settle pays 0; a zero-balance buyer cannot burn below zero (ChargePublish refuses,
// the burn is REFUSABLE — INV-RELAY-CONS (iii)); and after the full flow no account
// on L is below 0, the settle debits NOBODY (the burn already happened at
// issuance), and the ephemeral is never registered.
func TestRelaySettlementNeverLeavesAnAccountNegative(t *testing.T) {
	nonNegative := func(t *testing.T, l *Ledger, when string) {
		t.Helper()
		for n, a := range l.accounts {
			if a.balance < 0 {
				t.Fatalf("%s: account %x has balance %d < 0", when, n[:4], a.balance)
			}
		}
	}

	t.Run("zero_budget_pays_zero", func(t *testing.T) {
		l := New(r214Fee, 0)
		relay, eph := id(1), id(2)
		l.Register(relay)
		if paid := l.RedeemRelayCredit(relay, eph, 1<<40, 0); paid != 0 {
			t.Fatalf("zero-budget settle paid %d, want 0", paid)
		}
		nonNegative(t, l, "after a zero-budget settle")
		if _, ok := l.accounts[eph]; ok {
			t.Fatal("the ephemeral was registered by a zero-budget settle")
		}
	})

	t.Run("zero_balance_buyer_cannot_burn_below_zero", func(t *testing.T) {
		l := New(r214Fee, 0)
		buyer := id(2)
		l.Register(buyer)
		if err := l.ChargePublish(buyer); !errors.Is(err, ports.ErrInsufficientCredit) {
			t.Fatalf("ChargePublish on a zero-balance buyer = %v, want ErrInsufficientCredit — the issuance burn must be refusable", err)
		}
		if got := l.Balance(buyer); got != 0 {
			t.Fatalf("buyer balance %d after a refused burn, want 0", got)
		}
		nonNegative(t, l, "after a refused burn")
	})

	t.Run("full_flow_debits_nobody_at_settle", func(t *testing.T) {
		l := New(r214Fee, r214Grant)
		relay, buyer, eph := id(1), id(2), id(3)
		l.Register(relay)
		l.Register(buyer)
		const k = 6
		anchors := buyAnchors(t, l, buyer, 0, 0, k)
		buyerAfterBurn := l.Balance(buyer)
		if buyerAfterBurn != r214Grant-k*r214Fee {
			t.Fatalf("buyer balance after 6 burns is %d, want %d", buyerAfterBurn, r214Grant-k*r214Fee)
		}
		face, _ := l.SpendRelayAnchors(anchors, 0)
		relayBefore := l.Balance(relay)
		settled := l.RedeemRelayCredit(relay, eph, 1<<40, face)
		if settled != k*r214Fee {
			t.Fatalf("settled %d against budget %d for an over-long chain, want the budget %d", settled, face, k*r214Fee)
		}
		nonNegative(t, l, "after the settle")
		if got := l.Balance(buyer); got != buyerAfterBurn {
			t.Fatalf("the settle moved the BUYER's balance %d → %d — settlement must debit nobody; the burn already happened at issuance", buyerAfterBurn, got)
		}
		if got := l.Balance(relay); got != relayBefore+settled {
			t.Fatalf("relay balance %d, want %d + settled %d", got, relayBefore, settled)
		}
		if _, ok := l.accounts[eph]; ok {
			t.Fatal("the ephemeral was registered by the settlement")
		}
	})
}

// TestRelayAnchorGuardSurvivesRestart is T-8 (cert §9; red-team F2, restart is not
// an eviction; the F-4 creditSpent lesson): a spent anchor is in the durable
// PaidSerialStore BEFORE SpendRelayAnchors returns; a new ledger loaded from the
// same store refuses the re-spend; an attached-but-unloaded store refuses EVERY
// spend (the ledger does not yet know what it already accepted); a store whose
// Append fails refuses the spend (an under-pay, never an over-pay).
func TestRelayAnchorGuardSurvivesRestart(t *testing.T) {
	store := &memStore{}
	l1 := New(r214Fee, r214Grant)
	l1.SetPaidSerialStore(store)
	if err := l1.LoadPaidSerials(); err != nil {
		t.Fatal(err)
	}
	a := anchorsAt(0, 0, 1)
	if face, reason := l1.SpendRelayAnchors(a, 0); face != r214Fee {
		t.Fatalf("first spend: (face %d, reason %q), want (%d, \"\")", face, reason, r214Fee)
	}
	found := false
	for _, e := range store.entries {
		if bytes.Equal(e.Serial, a[0].Serial) && e.Epoch == 0 {
			found = true
		}
	}
	if !found {
		t.Fatalf("the spent anchor is not in the durable store after SpendRelayAnchors returned (store holds %d entries) — a crash here re-opens it for a second spend", len(store.entries))
	}

	// Restart: a new ledger from the same store.
	l2 := New(r214Fee, r214Grant)
	l2.SetPaidSerialStore(store)
	if err := l2.LoadPaidSerials(); err != nil {
		t.Fatal(err)
	}
	if face, reason := l2.SpendRelayAnchors(a, 0); face != 0 || reason == "" {
		t.Fatalf("after a restart the same anchor was spent AGAIN (face %d, reason %q) — restart is an eviction (red-team F2); one fee, two sessions", face, reason)
	}

	// Attached but NOT loaded: refuse everything until the guard is known.
	l3 := New(r214Fee, r214Grant)
	l3.SetPaidSerialStore(store)
	b := anchorsAt(0, 1, 1)
	if face, reason := l3.SpendRelayAnchors(b, 0); face != 0 || reason == "" {
		t.Fatalf("a spend on an attached-but-unloaded guard was accepted (face %d, reason %q) — the ledger cannot know what it already accepted", face, reason)
	}
	if err := l3.LoadPaidSerials(); err != nil {
		t.Fatal(err)
	}
	if face, _ := l3.SpendRelayAnchors(b, 0); face != r214Fee {
		t.Fatalf("after the load a fresh anchor was still refused (face %d) — refuse-until-loaded must clear on load", face)
	}
	if face, _ := l3.SpendRelayAnchors(a, 0); face != 0 {
		t.Fatalf("after the load the ORIGINAL anchor was spendable (face %d) — the load did not restore the guard", face)
	}

	// A store that cannot append: the spend is refused (under-pay direction).
	l4 := New(r214Fee, r214Grant)
	l4.SetPaidSerialStore(&failingStore{mem: &memStore{}})
	if err := l4.LoadPaidSerials(); err != nil {
		t.Fatal(err)
	}
	if face, reason := l4.SpendRelayAnchors(anchorsAt(0, 2, 1), 0); face != 0 || reason == "" {
		t.Fatalf("a spend whose guard entry could not be persisted was accepted (face %d, reason %q)", face, reason)
	}
}

// TestRelayAnchorGuardWindowMatchesKeysetWindow is T-12 (cert §9), the
// TestGuardLifetimeMatchesDemandKeysetLifetime twin on the anchor lane: at every
// epoch, "the keyset still verifies the anchor" and "the guard still remembers
// it" must be the SAME predicate. accepts && !remembers is the eviction pump (a
// second session re-spends); !accepts && remembers only wastes a slot but proves
// the windows are not one W. The guard's "remembers" is observed as a refusal
// labelled ReasonAlreadyPaid (the shipped label for "this serial already paid on
// this ledger"); any other refusal past the window is an expiry, which is fine
// only if the keyset also refuses.
//
// Also: an anchor dated in the FUTURE (epoch > current) is refused — the ledger
// must never record an entry it can never sweep (the delivery lane's
// ReasonBackdated shape).
func TestRelayAnchorGuardWindowMatchesKeysetWindow(t *testing.T) {
	t.Run("future_epoch_refused", func(t *testing.T) {
		l := New(r214Fee, 0)
		if face, reason := l.SpendRelayAnchors(anchorsAt(10, 0, 1), 0); face != 0 || reason == "" {
			t.Fatalf("an anchor from epoch 10 was spent at current epoch 0 (face %d, reason %q) — a future-dated entry can never be swept", face, reason)
		}
	})

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	pub := &key.PublicKey
	serial, _ := blindtoken.NewSerial(rand.Reader)
	blinded, secret, err := blindtoken.BlindRelayAnchor(rand.Reader, pub, 0, serial)
	if err != nil {
		t.Fatalf("BlindRelayAnchor: %v", err)
	}
	blindSig, err := blindtoken.SignBlinded(rand.Reader, key, blinded)
	if err != nil {
		t.Fatal(err)
	}
	sig, err := blindtoken.UnblindRelayAnchor(pub, 0, serial, blindSig, secret)
	if err != nil {
		t.Fatalf("UnblindRelayAnchor: %v", err)
	}
	tok := demand.Token{Serial: serial, Sig: sig}

	l := New(r214Fee, 0)
	a0 := []RelayAnchor{{Epoch: 0, Serial: serial}}
	if face, reason := l.SpendRelayAnchors(a0, 0); face != r214Fee {
		t.Fatalf("setup: the first spend must succeed, got (face %d, reason %q)", face, reason)
	}

	ks := demand.NewKeyset(demand.DefaultWindow)
	for cur := uint64(0); cur <= 2*demand.DefaultWindow+2; cur++ {
		for e := uint64(0); e <= cur; e++ {
			ks.Put(e, pub)
		}
		ks.Prune(cur)
		_, upstreamAccepts := ks.VerifyAnchorInWindow(cur, tok)
		if cur == 0 && !upstreamAccepts {
			t.Fatal("liveness: a fresh anchor does not verify at its own issue epoch through Keyset.VerifyAnchorInWindow")
		}

		l.sweptEpoch = 0 // force the expiry sweep to run at cur (the twin's idiom)
		face, reason := l.SpendRelayAnchors(a0, cur)
		remembers := face == 0 && reason == ReasonAlreadyPaid

		if upstreamAccepts && !remembers {
			t.Fatalf("epoch %d: the keyset still ACCEPTS the anchor but the guard does not remember it (face %d, reason %q) — the eviction pump: a second session re-spends the same anchor", cur, face, reason)
		}
		if !upstreamAccepts && remembers {
			t.Fatalf("epoch %d: the keyset refuses the anchor but the guard still holds it as paid — the guard window is not the keyset window W = %d", cur, demand.DefaultWindow)
		}
		if !upstreamAccepts {
			return // expired on both sides at the same boundary
		}
	}
	t.Fatal("the anchor never expired within 2W+2 epochs")
}
