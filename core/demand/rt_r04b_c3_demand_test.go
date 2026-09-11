package demand

// R0.4b C3 re-break — demand/crypto-tier regression gates. Inversions of the red-team
// probes core/demand/rt_c3b_demand_test.go (RT-C3B-6 … RT-C3B-10), archived at
// /Users/andrewedmond/.claude/silt-agent-memory/red-team/reviews/probes/R0.4b-C3-re-break-2026-09-03/.

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
	blinded, secret, err := Withdraw(rand.Reader, &k.PublicKey, testChainID, epoch, serial)
	if err != nil {
		t.Fatal(err)
	}
	tok, uerr := Unblind(&k.PublicKey, testChainID, epoch, serial, SignWithdrawal(rand.Reader, k, testChainID, blinded), secret)
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
		e, ok := ks.VerifyInWindow(testChainID, cur, tok)
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
			if blindtoken.VerifyDemand(&k.PublicKey, testChainID, e2, tok.Serial, tok.Sig) {
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
			if blindtoken.VerifyDemand(tc.key, testChainID, 3, []byte("not-a-serial"), []byte{0x00}) {
				t.Fatalf("VerifyDemand accepted a forged (serial, sig) pair under a degenerate key")
			}
			if _, _, err := blindtoken.BlindDemand(rand.Reader, tc.key, testChainID, 3, []byte("s")); err == nil {
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
	if _, ok := ks.VerifyInWindow(testChainID, 0, rtMint(t, good, 0, serial)); !ok {
		t.Fatalf("a well-formed 2048-bit key no longer verifies its own token")
	}
}

// ---------------------------------------------------------------------------
// RT-C3B-9 / F3 RE-HOMED (C1, 2026-09-08). The bank's own spent set retired with the
// v2 flat lane: under R2.9 an anchor is spent at session OPEN, into the credit ledger's
// SHARED paid-serial guard, so there is exactly one double-spend set left and it is the
// one the probes attacked. The two properties this file used to assert here are pinned
// on that guard, unchanged in substance:
//
//   - bounded, refuse-never-evict, expiry-only eviction (RT-C3B-9) — core/credit
//     TestSerialGuard_SetIsBounded, TestSerialGuard_ExpiryFreesTheCap,
//     TestSerialGuard_EvictThenReRedeemMintsZero and
//     TestSerialGuard_EvictionPumpIsNotSelfFinancing (the triple no FIFO-alone design
//     can satisfy).
//   - keyed by the TOKEN, not the serial, so an entry expires on its OWN issue epoch
//     (F3) — core/credit TestRTC3_GuardEntryExpiresOnItsOwnIssueEpoch.
//
// ---------------------------------------------------------------------------
// RT-C3B-10 CLOSED, on the session wire. Neither guard bounded the serial's SIZE, and
// the payload rides a 132 MiB frame. The probe pinned a 1 MiB serial as a map key. The
// bound now sits at the session decode (UnmarshalSessionOpen / boundAnchors) and again
// at the ledger, which refuses a mis-sized anchor serial with ReasonAnchorMalformed
// before it can become a guard key (core/credit spendAnchors).
// ---------------------------------------------------------------------------
func TestRTC3_OversizedSerialIsRefusedAtTheSessionWire(t *testing.T) {
	k := rtKey(t)
	fpriv := ed25519.NewKeyFromSeed(make([]byte, 32))
	server := ports.HashBytes([]byte{9})

	const oversized = 1 << 20 // 1 MiB; the wire frame allows ~132x this
	serial := make([]byte, oversized)
	serial[0] = 0xAB
	tok := rtMint(t, k, 0, serial)

	// (1) The open decode refuses it, before anything can store, count or hash it.
	blob, err := SignSessionOpen(fpriv, server, []Token{tok}).Marshal()
	if err != nil {
		t.Fatal(err)
	}
	if _, derr := UnmarshalSessionOpen(blob); !errors.Is(derr, ErrSessionBounds) {
		t.Fatalf("BREAK RT-C3B-10 REOPENED: the session decode accepted a %d-byte anchor serial (err=%v)",
			oversized, derr)
	}
	// (2) So does the top-up decode: a session admitted on honest anchors must not be
	// the door for an oversized one.
	fblob, err := SignSessionFund(fpriv, server, 7, []Token{tok}).Marshal()
	if err != nil {
		t.Fatal(err)
	}
	if _, derr := UnmarshalSessionFund(fblob); !errors.Is(derr, ErrSessionBounds) {
		t.Fatalf("the fund decode accepted a %d-byte anchor serial (err=%v)", oversized, derr)
	}
	// (3) An oversized SIGNATURE is refused on the same path — the other attacker-chosen
	// variable-length field on an anchor.
	fat := Token{Serial: make([]byte, blindtoken.SerialSize), Sig: make([]byte, maxTokenSigBytes+1)}
	fatBlob, err := SignSessionOpen(fpriv, server, []Token{fat}).Marshal()
	if err != nil {
		t.Fatal(err)
	}
	if _, derr := UnmarshalSessionOpen(fatBlob); !errors.Is(derr, ErrSessionBounds) {
		t.Fatalf("the session decode accepted a %d-byte anchor signature (err=%v)", len(fat.Sig), derr)
	}
	// An honest anchor still round-trips: the bound is not a denial of the real lane.
	honest, _ := blindtoken.NewSerial(rand.Reader)
	hblob, err := SignSessionOpen(fpriv, server, []Token{rtMint(t, k, 0, honest)}).Marshal()
	if err != nil {
		t.Fatal(err)
	}
	if o, derr := UnmarshalSessionOpen(hblob); derr != nil || len(o.Anchors) != 1 {
		t.Fatalf("an honest open was refused by the size bounds: %v", derr)
	}
}
