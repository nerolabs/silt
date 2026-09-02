package credit

// THE CROSS-LAYER WINDOW PIN — the gate core/credit's own comment cited for weeks
// while it did not exist (Tester finding, 2026-09-02: repo-wide grep for
// TestPaidSerialWindowMatchesDemandWindow found only the comment claiming it).
//
// W is written down THREE times: demand.DefaultWindow (the demand keyset's validity
// window), chain.issuerKeyPrePublish (how far ahead a key registration may name), and
// paidSerialWindow here. Two of the three were pinned together; this file pins the
// third, on both the value axis and the behavioural axis.
//
// WHY THE DRIFT IS EXPLOITABLE, not cosmetic (measured with a control): at
// paidSerialWindow = 2 against demand.DefaultWindow = 4 and current epoch 3, a
// cap-filling redeem sweeps the epoch-0 guard entries out while the demand layer
// still holds key_0 and still verifies epoch-0 tokens — so a second colluding server
// re-redeems an evicted serial and mints a full payout. The self-financing eviction
// pump, re-opened by a constant. At paidSerialWindow = 4 the target stays guarded and
// the re-redeem mints 0.
//
// core/demand is imported in TEST CODE ONLY. There is no production dependency from
// core/credit to core/demand and this file does not create one; it exists precisely
// because the constant is DUPLICATED across that boundary and duplication without a
// gate is drift waiting to happen.

import (
	"crypto/rand"
	"crypto/rsa"
	"testing"

	"github.com/nerolabs/silt/core/blindtoken"
	"github.com/nerolabs/silt/core/demand"
)

// TestPaidSerialWindowMatchesDemandWindow is the value pin. Ablation: set
// paidSerialWindow to anything but demand.DefaultWindow → RED.
func TestPaidSerialWindowMatchesDemandWindow(t *testing.T) {
	if paidSerialWindow != demand.DefaultWindow {
		t.Fatalf("paidSerialWindow = %d but demand.DefaultWindow = %d. These are the same W. "+
			"If the credit guard's window is SMALLER it sweeps serials the demand layer still "+
			"verifies, and a second server re-collects an evicted serial for a full payout — the "+
			"self-financing eviction pump. If it is LARGER the guard holds dead entries and the "+
			"cap fills with serials nothing can redeem.", paidSerialWindow, demand.DefaultWindow)
	}
}

// TestGuardLifetimeMatchesDemandKeysetLifetime is the BEHAVIOURAL half: it walks the
// epoch clock and asserts, at every epoch, that "the demand layer still verifies this
// token" and "the credit guard still remembers this serial" are the SAME predicate.
// A constant-equality test can be satisfied by changing both constants together; this
// one measures the seam itself, so it stays honest even then.
func TestGuardLifetimeMatchesDemandKeysetLifetime(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	// One token withdrawn at epoch 0, one guard entry recorded for it at epoch 0.
	serial, _ := blindtoken.NewSerial(rand.Reader)
	blinded, secret, _ := demand.Withdraw(rand.Reader, &key.PublicKey, 0, serial)
	tok := demand.Unblind(&key.PublicKey, serial, demand.SignWithdrawal(key, blinded), secret)

	l := New(50_000, 0)
	srvA, srvB, fetcher := id(1), id(2), id(3)
	obj := id(7)
	l.Register(fetcher)
	l.accounts[fetcher].balance = 1 << 40
	if paid := l.RedeemDeliveryCredit(srvA, fetcher, obj, tok.Serial, 0, 0); paid == 0 {
		t.Fatal("setup: the first redeem must pay")
	}

	// The issuer keeps its key_E schedule honest: one key, re-registered every epoch
	// (the schedule the chain permits and a restart produces). Under (b1) that is
	// sound, so this walk also proves the shared-key case.
	ks := demand.NewKeyset(demand.DefaultWindow)
	for cur := uint64(0); cur <= 2*demand.DefaultWindow+2; cur++ {
		for e := uint64(0); e <= cur; e++ {
			ks.Put(e, &key.PublicKey)
		}
		ks.Prune(cur)
		_, upstreamAccepts := ks.VerifyInWindow(cur, tok)

		// Force the guard to run its sweep at this epoch, then ask whether it still
		// remembers the serial. reservePaidSerial only sweeps at the cap, so drive
		// the sweep directly — the question is which entries it drops, not when.
		l.sweptEpoch = 0
		l.reserveSweepForTest(cur)
		_, guardRemembers := l.paidSerial[paidKey(0, tok.Serial)]

		if upstreamAccepts != guardRemembers {
			t.Fatalf("epoch %d: the demand layer accepts=%v but the guard remembers=%v. "+
				"These must be the same set. accepts && !remembers is the eviction pump "+
				"(a second server re-collects); !accepts && remembers only wastes a slot.",
				cur, upstreamAccepts, guardRemembers)
		}
		if !guardRemembers {
			// Past the boundary: a second server presenting the same token must be
			// refused UPSTREAM, before it ever reaches the ledger.
			if _, ok := ks.VerifyInWindow(cur, tok); ok {
				t.Fatalf("epoch %d: the token still verifies after the guard forgot it", cur)
			}
			_ = srvB
			return
		}
	}
	t.Fatal("the token never expired within 2W+2 epochs")
}

// reserveSweepForTest runs the expiry sweep at current without needing the cap to be
// full. It is the same call reservePaidSerial makes.
func (l *Ledger) reserveSweepForTest(current uint64) { l.sweepExpiredSerials(current) }
