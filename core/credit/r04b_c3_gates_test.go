package credit

// R0.4b C3 close — the credit-layer gates for the red-team's confirmed breaks
// (2026-09-02) and for the Tester's measured residuals. Each names its ablation.
//
//   Gate G  TestSharedKeyRotationDoesNotReopenThePump    (break 1, probe G)
//   Gate D  TestGuardHealsUnderASharedKey                (probe D)
//   Gate E  TestSweepRunsAtMostOncePerEpoch              (RT-E, measured 1.32 ms/redeem)
//   —       TestRefusedRedeemLeavesOnlyTheBilateralFallback  (Tester side measurement)
//   —       TestCapFullRefusalIsObservable               (Tester FINDING: not observable)

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/rsa"
	"testing"

	"github.com/nerolabs/silt/core/blindtoken"
	"github.com/nerolabs/silt/core/demand"
	"github.com/nerolabs/silt/ports"
)

// sharedKeyScene is the adversary's best case: ONE persisted RSA key, registered for
// EVERY epoch. Nothing in consensus forbids it and an ordinary restart produces it,
// so soundness has to hold here or it holds nowhere.
type sharedKeyScene struct {
	key *rsa.PrivateKey
	ks  *demand.Keyset
}

func newSharedKeyScene(t *testing.T) *sharedKeyScene {
	t.Helper()
	k, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	return &sharedKeyScene{key: k, ks: demand.NewKeyset(demand.DefaultWindow)}
}

// rotate re-registers the one key for every epoch up to cur — the "rotation" a
// persisted key admits — and prunes to the window.
func (s *sharedKeyScene) rotate(cur uint64) {
	for e := uint64(0); e <= cur; e++ {
		s.ks.Put(e, &s.key.PublicKey)
	}
	s.ks.Prune(cur)
}

// withdraw mints a real blind-withdrawn token for issue epoch e.
func (s *sharedKeyScene) withdraw(t *testing.T, e uint64) demand.Token {
	t.Helper()
	serial, err := blindtoken.NewSerial(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	blinded, secret, err := demand.Withdraw(rand.Reader, &s.key.PublicKey, e, serial)
	if err != nil {
		t.Fatal(err)
	}
	return demand.Unblind(&s.key.PublicKey, serial, demand.SignWithdrawal(s.key, blinded), secret)
}

// TestSharedKeyRotationDoesNotReopenThePump is GATE G (red-team break 1, probe G).
//
// THE ATTACK: pay real tokens on server A at epoch 0; wait for the guard entries to
// expire at epoch W+1; re-present the SAME tokens to server B, whose own spent-set is
// empty. Before (b1) every one of those tokens still verified — re-dated to the
// newest epoch the shared key was held for — so B collected a second full payout per
// serial and Σ(balances+escrow) rose by fee-minus-skim per serial with nothing charged
// behind it.
//
// THE GATE: at epoch W+1 the epoch-0 tokens must be refused UPSTREAM, at the demand
// window, before any credit path — and Σ must be exactly unchanged.
//
// ABLATION: drop the epoch from the demand FDH input (core/blindtoken demandMsg) →
// RED on the "still verifies" line, and Σ rises.
func TestSharedKeyRotationDoesNotReopenThePump(t *testing.T) {
	const fee = 50_000
	s := newSharedKeyScene(t)
	l := New(fee, 0)
	srvA, srvB, fetcher := id(1), id(2), id(3)
	obj := id(7)
	l.Register(fetcher)
	l.accounts[fetcher].balance = 1 << 40

	sum := func() int64 {
		var total int64
		for _, a := range l.accounts {
			total += a.balance
		}
		for _, e := range l.escrow {
			total += e.balance
		}
		return total
	}

	const n = 32
	s.rotate(0)
	toks := make([]demand.Token, n)
	for i := range toks {
		toks[i] = s.withdraw(t, 0)
		ep, ok := s.ks.VerifyInWindow(0, toks[i])
		if !ok || ep != 0 {
			t.Fatalf("epoch-0 token: ok=%v epoch=%d", ok, ep)
		}
		if err := l.ChargePublish(fetcher); err != nil {
			t.Fatal(err)
		}
		if paid := l.RedeemDeliveryCredit(srvA, fetcher, obj, toks[i].Serial, ep, 0); paid == 0 {
			t.Fatalf("token %d: the honest first redeem must pay", i)
		}
	}
	injected := sum()

	// The window moves past epoch 0. The guard sweeps its epoch-0 entries.
	cur := demand.DefaultWindow + 1
	s.rotate(cur)
	l.sweepExpiredSerials(cur)
	if _, still := l.paidSerial[paidKey(0, toks[0].Serial)]; still {
		t.Fatal("setup: the guard did not sweep the expired epoch-0 entries, so this " +
			"test would pass for the wrong reason (the guard, not expiry, refusing)")
	}

	// Server B re-presents every one of them through its OWN bank — a fresh
	// spent-set, which is exactly what makes this the cross-server pump and not a
	// double-spend. The bank is the gate: the credit layer must never be reached.
	bankB := demand.NewBank()
	fetcherKey := ed25519.NewKeyFromSeed(make([]byte, ed25519.SeedSize))
	var reMinted int64
	for i, tok := range toks {
		rcpt := demand.Ack(fetcherKey, tok, [32]byte(obj), srvB)
		credited, ep, why := bankB.Redeem(s.ks, cur, tok, rcpt)
		if credited {
			t.Fatalf("token %d withdrawn at epoch 0 was BANKED at epoch %d (reported "+
				"issuedEpoch %d) under a key bound to every epoch. The issue epoch is not "+
				"bound into the signed message, so guard entries expire while tokens do not "+
				"— the cross-server double-redeem pump is open.", i, cur, ep)
		}
		if why != "token expired or not issued" {
			t.Fatalf("token %d refused for the wrong reason %q — this gate must measure "+
				"EXPIRY, not some other rejection", i, why)
		}
		// The honest server settles only on a credited receipt, so the ledger is
		// never reached. Assert that by settling exactly as core/node does.
		if credited {
			reMinted += l.RedeemDeliveryCredit(srvB, fetcher, [32]byte(obj), tok.Serial, ep, cur)
		}
	}
	if delta := sum() - injected; delta != 0 || reMinted != 0 {
		t.Fatalf("server B re-collected %d credits off epoch-0 serials; Σ moved by %+d, want 0",
			reMinted, delta)
	}
}

// TestGuardHealsUnderASharedKey is GATE D (probe D). Under the SAME key for every
// epoch, guard entries must still be tagged with each token's OWN issue epoch, so
// they age out and the cap heals as the window advances. Before (b1) every entry was
// tagged `current` (the newest epoch that verified), nothing ever expired, and an
// honest server that filled the cap hit a permanent refuse-to-pay wall.
//
// ABLATION: report `current` as issuedEpoch (or drop the FDH epoch binding) → the
// guard never empties → RED.
func TestGuardHealsUnderASharedKey(t *testing.T) {
	s := newSharedKeyScene(t)
	l := New(50_000, 0)
	srv, fetcher := id(1), id(2)
	obj := id(7)
	l.Register(fetcher)
	l.accounts[fetcher].balance = 1 << 40

	// Honest traffic across several epochs, each token withdrawn in its own epoch.
	const perEpoch = 8
	for cur := uint64(0); cur < demand.DefaultWindow+2; cur++ {
		s.rotate(cur)
		for i := 0; i < perEpoch; i++ {
			tok := s.withdraw(t, cur)
			ep, ok := s.ks.VerifyInWindow(cur, tok)
			if !ok {
				t.Fatalf("epoch %d: a token withdrawn this epoch does not verify", cur)
			}
			if ep != cur {
				t.Fatalf("epoch %d: token reported issuedEpoch %d — the guard would tag it "+
					"with the wrong expiry and never free the slot", cur, ep)
			}
			if paid := l.RedeemDeliveryCredit(srv, fetcher, obj, tok.Serial, ep, cur); paid == 0 {
				t.Fatalf("epoch %d: honest redeem paid nothing", cur)
			}
		}
	}
	// The window has moved past the first epochs; the sweep must free them.
	cur := 2*demand.DefaultWindow + 4
	before := len(l.paidSerial)
	l.sweepExpiredSerials(cur)
	if len(l.paidSerial) != 0 {
		t.Fatalf("after the window advanced to %d the guard still holds %d of %d entries — "+
			"expiry is a no-op under a shared key and the cap is a permanent wall",
			cur, len(l.paidSerial), before)
	}
}

// TestSweepRunsAtMostOncePerEpoch is GATE E (RT-E). At a full cap of STILL-LIVE
// serials every refused redeem used to run a full map scan — the red-team measured
// 1.32 ms per refused receipt at 65,536 entries, a free amplifier for a griefer. The
// gate counts SWEEPS, not time, so it cannot go green on a fast machine.
func TestSweepRunsAtMostOncePerEpoch(t *testing.T) {
	l := New(50_000, 0)
	srv, fetcher := id(1), id(2)
	obj := id(7)
	const epoch = uint64(100)
	for i := 0; i < maxPaidSerial; i++ {
		l.RedeemDeliveryCredit(srv, fetcher, obj, testSerial(i), epoch, epoch)
	}
	if len(l.paidSerial) != maxPaidSerial {
		t.Fatalf("setup: guard holds %d, want the cap %d", len(l.paidSerial), maxPaidSerial)
	}
	base := l.SerialSweeps()
	const refusals = 500
	for i := 0; i < refusals; i++ {
		if paid := l.RedeemDeliveryCredit(srv, fetcher, obj, testSerial(1_000_000+i), epoch, epoch); paid != 0 {
			t.Fatal("a redeem paid at a full LIVE cap — the guard must refuse, never evict a live entry")
		}
	}
	if got := l.SerialSweeps() - base; got > 1 {
		t.Fatalf("%d refused redeems ran %d full sweeps of a %d-entry guard, want at most 1 "+
			"(nothing can expire twice within one epoch)", refusals, got, maxPaidSerial)
	}
	if l.GuardFullRefusals() != refusals {
		t.Fatalf("GuardFullRefusals = %d, want %d — the operator-visible counter must count "+
			"every cap refusal", l.GuardFullRefusals(), refusals)
	}
	// A new epoch buys exactly one more sweep, not none: the guard must still heal.
	base = l.SerialSweeps()
	l.RedeemDeliveryCredit(srv, fetcher, obj, testSerial(2_000_000), epoch+1, epoch+1)
	if got := l.SerialSweeps() - base; got != 1 {
		t.Fatalf("a new epoch ran %d sweeps, want exactly 1 — the once-per-epoch latch must "+
			"not stop the guard healing", got)
	}
}

// TestCapFullRefusalIsObservable pins the Tester's FINDING that a cap-full refusal
// returned a bare 0, indistinguishable from self-delivery, an already-paid serial, or
// a zero fee — and surfaced at the node inside a log line reading "delivery receipt
// banked". Each non-paying path must now name itself.
func TestCapFullRefusalIsObservable(t *testing.T) {
	l := New(50_000, 0)
	srv, fetcher := id(1), id(2)
	obj := id(7)
	if _, why := l.RedeemDeliveryCreditReason(srv, srv, obj, testSerial(1), 0, 0); why != ReasonSelfDelivery {
		t.Fatalf("self-delivery reason = %q", why)
	}
	if paid, why := l.RedeemDeliveryCreditReason(srv, fetcher, obj, testSerial(2), 0, 0); paid == 0 || why != ReasonPaid {
		t.Fatalf("an honest redeem: paid=%d reason=%q", paid, why)
	}
	if _, why := l.RedeemDeliveryCreditReason(srv, fetcher, obj, testSerial(2), 0, 0); why != ReasonAlreadyPaid {
		t.Fatalf("re-redeem reason = %q", why)
	}
	if _, why := l.RedeemDeliveryCreditReason(srv, fetcher, obj, testSerial(3), 0, 2*demand.DefaultWindow+2); why != ReasonBackdated {
		t.Fatalf("backdated reason = %q", why)
	}
	if _, why := New(0, 0).RedeemDeliveryCreditReason(srv, fetcher, obj, testSerial(4), 0, 0); why != ReasonNoFee {
		t.Fatalf("zero-fee reason = %q", why)
	}

	full := New(50_000, 0)
	for i := 0; i < maxPaidSerial; i++ {
		full.RedeemDeliveryCredit(srv, fetcher, obj, testSerial(i), 100, 100)
	}
	paid, why := full.RedeemDeliveryCreditReason(srv, fetcher, obj, testSerial(9_000_001), 100, 100)
	if paid != 0 || why != ReasonGuardFull {
		t.Fatalf("cap-full refusal: paid=%d reason=%q, want 0 / %q — an operator cannot tell "+
			"'the serve rate exceeded the modeled bound' from an ordinary no-pay otherwise",
			paid, why, ReasonGuardFull)
	}
	if full.GuardFullRefusals() != 1 {
		t.Fatalf("GuardFullRefusals = %d, want 1", full.GuardFullRefusals())
	}
}

// TestRefusedRedeemLeavesOnlyTheBilateralFallback bounds the combination the Tester
// measured and no shipped gate exercised: a PROVISIONAL serve lane plus a REFUSED
// redeem. The refusal returns before the supersede block, so the serving node keeps
// its unreversed 1-credit/byte self-mint and Σ is up by exactly `bytes`.
//
// THE BOUND (why this is disclosed and gated rather than fixed): that residual EQUALS
// the no-receipt baseline — a server gets the same +bytes by simply never submitting
// a receipt — so the refusal creates no new incentive. It is the pre-existing
// unwitnessed bilateral fallback (RecordServeToObject's self-mint), capped by
// maxProvisional lanes, and eviction reverses it. What must never happen is Σ rising
// by MORE than the serve's own self-mint, i.e. the refusal must not also pay.
func TestRefusedRedeemLeavesOnlyTheBilateralFallback(t *testing.T) {
	const fee = 50_000
	for _, bytesServed := range []int64{1_000, 50_000, 500_000} {
		l := New(fee, 0)
		srvA, srvB, fetcher := id(1), id(2), id(3)
		obj := id(7)
		l.Register(fetcher)
		l.accounts[fetcher].balance = 1 << 40
		sum := func() int64 {
			var total int64
			for _, a := range l.accounts {
				total += a.balance
			}
			for _, e := range l.escrow {
				total += e.balance
			}
			return total
		}
		serial := testSerial(1)
		if err := l.ChargePublish(fetcher); err != nil {
			t.Fatal(err)
		}
		if paid := l.RedeemDeliveryCredit(srvA, fetcher, obj, serial, 0, 0); paid == 0 {
			t.Fatal("the honest first redeem must pay")
		}

		// The colluding second server serves the same object to the same fetcher
		// (recording its provisional self-mint) and then presents the SAME serial.
		before := sum()
		l.RecordServeToObject(srvB, fetcher, obj, ports.ChunkID(id(11)), bytesServed)
		afterServe := sum()
		paid, why := l.RedeemDeliveryCreditReason(srvB, fetcher, obj, serial, 0, 0)
		if paid != 0 || why != ReasonAlreadyPaid {
			t.Fatalf("bytes=%d: the second server was paid %d (%q) off one serial", bytesServed, paid, why)
		}
		selfMint := afterServe - before
		if delta := sum() - before; delta != selfMint {
			t.Fatalf("bytes=%d: Σ moved %+d over the serve+refused-redeem pair but the serve's "+
				"own self-mint was %+d. The refusal must add nothing on top of the "+
				"pre-existing unwitnessed bilateral fallback.", bytesServed, delta, selfMint)
		}
		// And the bound: the residual is the serve's self-mint, never a payout.
		if selfMint > bytesServed {
			t.Fatalf("bytes=%d: the provisional self-mint was %+d, above the bytes served",
				bytesServed, selfMint)
		}
	}
}
