package demand

// R0.4b C3 re-break — demand/crypto-tier regression gates. Inversions of the red-team
// probes core/demand/rt_c3b_demand_test.go (RT-C3B-6 … RT-C3B-10), archived at
// /Users/andrewedmond/Claude/claude/silt-reviews/red-team/probes/R0.4b-C3-re-break-2026-09-03/.

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/rsa"
	"errors"
	"math/big"
	"testing"

	"github.com/nerolabs/silt/core/blindtoken"
	"github.com/nerolabs/silt/ports"
)

func rtKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	k, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	return k
}

func rtMint(t *testing.T, k *rsa.PrivateKey, epoch uint64, serial []byte) Token {
	t.Helper()
	blinded, secret, err := Withdraw(rand.Reader, &k.PublicKey, epoch, serial)
	if err != nil {
		t.Fatal(err)
	}
	tok, uerr := Unblind(&k.PublicKey, epoch, serial, SignWithdrawal(rand.Reader, k, blinded), secret)
	if uerr != nil {
		t.Fatal(uerr)
	}
	return tok
}

// ---------------------------------------------------------------------------
// RT-C3B-6 (CONTROL, held under attack and must keep holding). A signature made for
// epoch E verifies under exactly (key_E, E) and no other epoch, even under one shared
// key. This is the (b1) close the whole R0.4b expiry argument rests on.
// ---------------------------------------------------------------------------
func TestRTC3_EpochBindingHoldsUnderOneKey(t *testing.T) {
	k := rtKey(t)
	ks := NewKeyset(DefaultWindow)
	for e := uint64(0); e <= 8; e++ {
		ks.Put(e, &k.PublicKey)
	}
	serial, _ := blindtoken.NewSerial(rand.Reader)
	tok := rtMint(t, k, 4, serial)
	for cur := uint64(4); cur <= 8; cur++ {
		ks.Prune(cur)
		e, ok := ks.VerifyInWindow(cur, tok)
		switch {
		case cur-4 <= DefaultWindow && (!ok || e != 4):
			t.Fatalf("cur=%d: in-window token must verify at epoch 4, got ok=%v e=%d", cur, ok, e)
		case cur-4 > DefaultWindow && ok:
			t.Fatalf("cur=%d: out-of-window token verified at epoch %d", cur, e)
		}
		for e2 := range ks.keys {
			if e2 == 4 {
				continue
			}
			if blindtoken.VerifyDemand(&k.PublicKey, e2, tok.Serial, tok.Sig) {
				t.Fatalf("CROSS-EPOCH REPLAY: epoch-4 token verified at epoch %d under the same key", e2)
			}
		}
	}
}

// ---------------------------------------------------------------------------
// RT-C3B-7 / 7b / 8 CLOSED. A consensus commitment attests 32 BYTES — which bytes the
// issuer serves — and never that those bytes are an unforgeable signature scheme.
// Nothing between the wire and the modexp validated the key, so:
//
//	N = 1 ⇒ the FDH image is 0 and s^e mod 1 == 0 ⇒ EVERY (serial, sig) pair verified.
//	E = 1 ⇒ sig := FDH(msg), computable by anyone holding the public key.
//	N = 0 ⇒ big.Int.Mod divided by zero ⇒ the verifier PANICKED.
//
// blindtoken.ValidatePub is now the single definition of well-formedness, enforced at
// ParsePub (the wire), at Keyset.Put (the door), and before every modexp.
// ---------------------------------------------------------------------------
func TestRTC3_DegenerateKeysAreRefusedEverywhere(t *testing.T) {
	good := rtKey(t)
	for _, tc := range []struct {
		name string
		key  *rsa.PublicKey
	}{
		{"zero-modulus", &rsa.PublicKey{N: big.NewInt(0), E: 65537}},
		{"unit-modulus", &rsa.PublicKey{N: big.NewInt(1), E: 65537}},
		{"even-modulus", &rsa.PublicKey{N: new(big.Int).Lsh(big.NewInt(1), 2048), E: 65537}},
		{"short-modulus", &rsa.PublicKey{N: big.NewInt(1023), E: 65537}},
		{"exponent-one", &rsa.PublicKey{N: good.N, E: 1}},
		{"even-exponent", &rsa.PublicKey{N: good.N, E: 65536}},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			// (1) The wire boundary refuses it, legibly.
			if _, err := blindtoken.ParsePub(blindtoken.MarshalPub(tc.key)); !errors.Is(err, blindtoken.ErrBadPubKey) {
				t.Fatalf("ParsePub accepted a degenerate key (err=%v)", err)
			}
			// (2) The keyset door refuses to HOLD it, so nothing downstream can reach it.
			ks := NewKeyset(DefaultWindow)
			ks.Put(3, tc.key)
			if ks.Key(3) != nil {
				t.Fatalf("Keyset.Put held a degenerate key — a held N=1 verifies an arbitrary " +
					"(serial, sig) pair, which is a universal forgery behind a valid pin")
			}
			// (3) And the primitives themselves refuse: never a panic, never a true.
			if blindtoken.VerifyDemand(tc.key, 3, []byte("not-a-serial"), []byte{0x00}) {
				t.Fatalf("VerifyDemand accepted a forged (serial, sig) pair under a degenerate key")
			}
			if _, _, err := blindtoken.BlindDemand(rand.Reader, tc.key, 3, []byte("s")); err == nil {
				t.Fatalf("BlindDemand ran the modexp against a degenerate modulus")
			}
			if _, uerr := blindtoken.Unblind(tc.key, []byte("s"), []byte{1}, []byte{1}); uerr == nil {
				t.Fatalf("Unblind ran the modexp against a degenerate modulus")
			}
		})
	}
	// The honest key still works end to end — the validation is not a denial of service
	// on the real lane.
	ks := NewKeyset(DefaultWindow)
	ks.Put(0, &good.PublicKey)
	serial, _ := blindtoken.NewSerial(rand.Reader)
	if _, ok := ks.VerifyInWindow(0, rtMint(t, good, 0, serial)); !ok {
		t.Fatalf("a well-formed 2048-bit key no longer verifies its own token")
	}
}

// ---------------------------------------------------------------------------
// RT-C3B-9 CLOSED. Bank.spent is BOUNDED (build-immutable #8) and its eviction is
// expiry-only. The probe measured "3000 entries after the window advanced 1000 epochs
// past every token — no cap, no sweep and no eviction path".
// ---------------------------------------------------------------------------
func TestRTC3_BankSpentSetIsBoundedAndExpirySwept(t *testing.T) {
	if maxSpentTokens < 1 {
		t.Fatalf("maxSpentTokens must be positive")
	}
	k := rtKey(t)
	ks := NewKeyset(DefaultWindow)
	ks.Put(0, &k.PublicKey)
	b := NewBank()

	// Fill past the cap directly — minting 65k real blind signatures would take
	// minutes and the property under test is the guard's bookkeeping, not the crypto.
	for i := 0; i < maxSpentTokens; i++ {
		b.spent[spentKey(0, []byte{byte(i), byte(i >> 8), byte(i >> 16)})] = 0
	}
	if len(b.spent) != maxSpentTokens {
		t.Fatalf("fill: %d entries, want %d", len(b.spent), maxSpentTokens)
	}
	// At the cap with nothing expired, the bank REFUSES rather than evicting a live
	// entry (the refuted FIFO design is self-financing for the flooder).
	if b.reserveSpent(0, DefaultWindow) {
		t.Fatalf("the guard admitted an entry past its cap with nothing expired")
	}
	if len(b.spent) > maxSpentTokens {
		t.Fatalf("BREAK RT-C3B-9 REOPENED: b.spent grew to %d, past the %d cap",
			len(b.spent), maxSpentTokens)
	}
	// Once the window has moved past every entry, the sweep frees them — and only
	// then. Eviction is expiry-only.
	if !b.reserveSpent(DefaultWindow+1, DefaultWindow) {
		t.Fatalf("the sweep did not free the expired entries")
	}
	if len(b.spent) != 0 {
		t.Fatalf("the sweep left %d expired entries", len(b.spent))
	}

	// And a live entry is never swept: an in-window token stays guarded.
	serial, _ := blindtoken.NewSerial(rand.Reader)
	tok := rtMint(t, k, 0, serial)
	fpriv := ed25519.NewKeyFromSeed(make([]byte, 32))
	r := DeliveryReceipt{Serial: serial, Object: ports.HashBytes([]byte{7}),
		Server: ports.HashBytes([]byte{9}), Fetcher: fpriv.Public().(ed25519.PublicKey)}
	r.Sig = ed25519.Sign(fpriv, r.receiptMsg())
	if ok, _, why := b.Redeem(ks, 0, tok, r); !ok {
		t.Fatalf("honest receipt: %s", why)
	}
	b.sweepExpiredSpent(DefaultWindow, DefaultWindow)
	if _, live := b.spent[spentKey(0, serial)]; !live {
		t.Fatalf("the sweep evicted a token still inside its window — evicted ⇒ expired is false")
	}
	b.sweepExpiredSpent(DefaultWindow+1, DefaultWindow)
	if _, live := b.spent[spentKey(0, serial)]; live {
		t.Fatalf("the sweep kept a token past its window")
	}
}

// TestRTC3_SpentIsKeyedByTheTokenNotTheSerial: the demand layer's own double-spend set
// carries the same per-token expiry key the credit guard does, so its eviction is
// sound for the same reason (F3, at this tier).
func TestRTC3_SpentIsKeyedByTheTokenNotTheSerial(t *testing.T) {
	k := rtKey(t)
	ks := NewKeyset(DefaultWindow)
	ks.Put(0, &k.PublicKey)
	ks.Put(4, &k.PublicKey)
	fpriv := ed25519.NewKeyFromSeed(make([]byte, 32))
	fpub := fpriv.Public().(ed25519.PublicKey)
	obj, server := ports.HashBytes([]byte{7}), ports.HashBytes([]byte{9})
	serial, _ := blindtoken.NewSerial(rand.Reader)

	b := NewBank()
	for _, e := range []uint64{0, 4} {
		tok := rtMint(t, k, e, serial)
		r := DeliveryReceipt{Serial: serial, Object: obj, Server: server, Fetcher: fpub}
		r.Sig = ed25519.Sign(fpriv, r.receiptMsg())
		ok, got, why := b.Redeem(ks, 4, tok, r)
		if !ok || got != e {
			t.Fatalf("token at epoch %d: ok=%v issuedEpoch=%d (%s)", e, ok, got, why)
		}
		// The SAME token twice is always a double-spend.
		if ok, _, _ := b.Redeem(ks, 4, tok, r); ok {
			t.Fatalf("the same token was redeemed twice at epoch %d", e)
		}
	}
	b.sweepExpiredSpent(5, DefaultWindow)
	if _, live := b.spent[spentKey(4, serial)]; !live {
		t.Fatalf("the epoch-4 entry was swept at epoch 5 while still in window — the set is " +
			"keyed by the serial again, so its expiry is the MINIMUM epoch over the tokens " +
			"sharing it")
	}
}

// ---------------------------------------------------------------------------
// RT-C3B-10 CLOSED. Neither guard bounded the serial's SIZE, and a SubmittedReceipt
// rides a 132 MiB frame. The probe pinned a 1 MiB serial as a map key in Bank.spent.
// ---------------------------------------------------------------------------
func TestRTC3_OversizedSerialIsRefusedAtTheWireAndAtTheBank(t *testing.T) {
	k := rtKey(t)
	ks := NewKeyset(DefaultWindow)
	ks.Put(0, &k.PublicKey)
	fpriv := ed25519.NewKeyFromSeed(make([]byte, 32))
	fpub := fpriv.Public().(ed25519.PublicKey)
	obj, server := ports.HashBytes([]byte{7}), ports.HashBytes([]byte{9})

	const oversized = 1 << 20 // 1 MiB; the wire frame allows ~132x this
	serial := make([]byte, oversized)
	serial[0] = 0xAB
	tok := rtMint(t, k, 0, serial)
	r := DeliveryReceipt{Serial: serial, Object: obj, Server: server, Fetcher: fpub}
	r.Sig = ed25519.Sign(fpriv, r.receiptMsg())

	// (1) The wire decode refuses it, before anything can store, count or hash it.
	blob, err := SubmittedReceipt{Token: tok, Receipt: r}.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	if _, derr := UnmarshalSubmittedReceipt(blob); !errors.Is(derr, ErrOversizedReceipt) {
		t.Fatalf("BREAK RT-C3B-10 REOPENED: the wire decode accepted a %d-byte serial (err=%v)",
			oversized, derr)
	}
	// (2) And the bank refuses it too, so a non-wire caller cannot pin the bytes either.
	b := NewBank()
	if ok, _, why := b.Redeem(ks, 0, tok, r); ok {
		t.Fatalf("the bank accepted a %d-byte serial (%s)", oversized, why)
	}
	if len(b.spent) != 0 {
		t.Fatalf("a refused oversized serial was still pinned as a map key")
	}
	// An honest serial still round-trips.
	honest, _ := blindtoken.NewSerial(rand.Reader)
	ht := rtMint(t, k, 0, honest)
	hr := DeliveryReceipt{Serial: honest, Object: obj, Server: server, Fetcher: fpub}
	hr.Sig = ed25519.Sign(fpriv, hr.receiptMsg())
	hb, err := SubmittedReceipt{Token: ht, Receipt: hr}.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	if _, derr := UnmarshalSubmittedReceipt(hb); derr != nil {
		t.Fatalf("an honest receipt was refused by the size bounds: %v", derr)
	}
}
