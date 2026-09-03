package blindtoken

// Crypto-specialist advisory gates, 2026-09-03.
// Source: /Users/andrewedmond/Claude/claude/silt-reviews/crypto-specialist/ADVISORY-R0.4b-C3-blind-RSA-epoch-binding-2026-09-03.md
//
// One test per finding, each named for the finding and each driving the primitive
// rather than a proxy. The advisory's own four degenerate moduli are reproduced
// verbatim as the C-3 fixture, because those are the shapes its exploratory spike
// showed the pre-hardening bound set ACCEPTED.

import (
	"crypto/rand"
	"crypto/rsa"
	"math/big"
	mrand "math/rand"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// C-1 — RFC 9474 §4.4 Finalize. Unblind MUST verify before returning.
// ---------------------------------------------------------------------------

func TestC1_UnblindRefusesASignatureThatDoesNotVerify(t *testing.T) {
	priv := testKey(t)
	pub := &priv.PublicKey
	serial, _ := NewSerial(rand.Reader)
	blinded, secret, err := Blind(rand.Reader, pub, serial)
	if err != nil {
		t.Fatal(err)
	}
	good := mustSign(t, priv, blinded)

	// A DUD from a malicious issuer: a well-formed representative that is simply not
	// the signature. Perturbing the real one keeps it in range and canonical, so the
	// ONLY thing that can catch it is the Finalize verification.
	dud := new(big.Int).Add(new(big.Int).SetBytes(good), bigOne)
	dud.Mod(dud, pub.N)
	if _, uerr := Unblind(pub, serial, dud.Bytes(), secret); uerr == nil {
		t.Fatal("BREAK C-1: Unblind returned a token for a signature that does not verify. " +
			"RFC 9474 §4.4 requires Finalize to verify and stop. A malicious issuer could " +
			"charge the withdrawal fee and hand back a dud whose failure only surfaces at " +
			"redemption — and a receipt that fails Bank.Redeem never reaches the ledger, so " +
			"the serve's eager self-mint is never reversed.")
	}
	// The honest leg — a positive control, so the refusal above is not vacuous.
	sig, uerr := Unblind(pub, serial, good, secret)
	if uerr != nil || !Verify(pub, serial, sig) {
		t.Fatalf("the honest withdrawal must still finalize: %v", uerr)
	}
}

func TestC1_UnblindDemandRefusesTheWrongEpochAndTheWrongKey(t *testing.T) {
	priv := testKey(t)
	other := testKey(t)
	pub := &priv.PublicKey
	serial, _ := NewSerial(rand.Reader)
	blinded, secret, err := BlindDemand(rand.Reader, pub, 7, serial)
	if err != nil {
		t.Fatal(err)
	}
	sig := mustSign(t, priv, blinded)

	if _, uerr := UnblindDemand(pub, 7, serial, sig, secret); uerr != nil {
		t.Fatalf("the honest (key, epoch) pair must finalize: %v", uerr)
	}
	if _, uerr := UnblindDemand(pub, 8, serial, sig, secret); uerr == nil {
		t.Fatal("BREAK C-1: a signature made for epoch 7 finalized at epoch 8")
	}
	if _, uerr := UnblindDemand(&other.PublicKey, 7, serial, sig, secret); uerr == nil {
		t.Fatal("BREAK C-1: a signature finalized under a key that did not make it")
	}
}

// ---------------------------------------------------------------------------
// C-2 — the issuer's private-key operation: canonical input, blinded modexp,
// verify-after-sign. The wire format must be UNCHANGED.
// ---------------------------------------------------------------------------

func TestC2_SignBlindedRoundTripsAndKeepsTheWireFormat(t *testing.T) {
	priv := testKey(t)
	pub := &priv.PublicKey
	for i := 0; i < 16; i++ {
		serial, _ := NewSerial(rand.Reader)
		blinded, secret, err := Blind(rand.Reader, pub, serial)
		if err != nil {
			t.Fatal(err)
		}
		sig := mustSign(t, priv, blinded)
		// The blinding factor cancels: the signature is the deterministic RSA value,
		// byte for byte, whatever randomness the issuer used. This is what makes the
		// blinding invisible on the wire AND keeps the retry-dedup cache honest
		// (a retry must return the IDENTICAL blind signature).
		want := new(big.Int).Exp(new(big.Int).SetBytes(blinded), priv.D, priv.N).Bytes()
		if string(sig) != string(want) {
			t.Fatalf("iteration %d: blinding the private op CHANGED the signature bytes — "+
				"the wire format moved and every retry-dedup assertion with it", i)
		}
		if _, uerr := Unblind(pub, serial, sig, secret); uerr != nil {
			t.Fatalf("iteration %d: %v", i, uerr)
		}
	}
}

func TestC2_SignBlindedRefusesANonCanonicalOrOutOfRangeInput(t *testing.T) {
	priv := testKey(t)
	pub := &priv.PublicKey
	serial, _ := NewSerial(rand.Reader)
	blinded, _, err := Blind(rand.Reader, pub, serial)
	if err != nil {
		t.Fatal(err)
	}
	sig := mustSign(t, priv, blinded)

	// The two re-encodings of ONE representative that the old `b.Mod(b, N)` accepted.
	plusN := new(big.Int).Add(new(big.Int).SetBytes(blinded), priv.N).Bytes()
	padded := append([]byte{0}, blinded...)
	for name, bad := range map[string][]byte{
		"blinded + N":      plusN,
		"zero-padded":      padded,
		"empty":            {},
		"N itself":         priv.N.Bytes(),
		"way out of range": new(big.Int).Lsh(priv.N, 8).Bytes(),
	} {
		got, serr := SignBlinded(rand.Reader, priv, bad)
		if serr == nil {
			t.Fatalf("BREAK C-5: the issuer signed a %s spelling of a blinded value. That is a "+
				"DEDUP BYPASS: core/node's demandDedupKey is keyed on the RAW blinded bytes, so "+
				"a re-encoded blind is a fresh cache key for an issuance already settled — the "+
				"requester is charged twice for one signature (got %x…)", name, got[:8])
		}
	}
	// Positive control.
	if _, serr := SignBlinded(rand.Reader, priv, blinded); serr != nil {
		t.Fatalf("the canonical blinded value must sign: %v", serr)
	}
	_ = sig
}

// TestC2_FuzzBlindedInputsNeverPanicAndNeverForge is the advisory's "fuzz of blinded
// inputs": arbitrary attacker bytes at the signing oracle must never panic, and
// anything the oracle DOES sign must verify under its own public key (the
// verify-after-sign guarantee, stated as a property over random input).
func TestC2_FuzzBlindedInputsNeverPanicAndNeverForge(t *testing.T) {
	priv := testKey(t)
	rng := mrand.New(mrand.NewSource(20260903))
	nLen := (priv.N.BitLen() + 7) / 8
	accepted := 0
	for i := 0; i < 512; i++ {
		b := make([]byte, 1+rng.Intn(nLen+8))
		for j := range b {
			b[j] = byte(rng.Intn(256))
		}
		sig, err := SignBlinded(rand.Reader, priv, b)
		if err != nil {
			continue
		}
		accepted++
		v := new(big.Int).Exp(new(big.Int).SetBytes(sig), big.NewInt(int64(priv.E)), priv.N)
		if v.Cmp(new(big.Int).SetBytes(b)) != 0 {
			t.Fatalf("iteration %d: the oracle released a signature that does not verify "+
				"against the value it signed — verify-after-sign is not running", i)
		}
	}
	if accepted == 0 {
		t.Fatal("the fuzz accepted nothing at all; it is measuring the rejection path only")
	}
	t.Logf("fuzz: 512 arbitrary blinded inputs, %d signed, all verify-after-sign clean", accepted)
}

func TestC2_VerifyAfterSignRefusesAFaultedModexp(t *testing.T) {
	priv := testKey(t)
	serial, _ := NewSerial(rand.Reader)
	blinded, _, err := Blind(rand.Reader, &priv.PublicKey, serial)
	if err != nil {
		t.Fatal(err)
	}
	// Model the Boneh-DeMillo-Lipton fault as a corrupted private exponent: the modexp
	// runs and returns a WRONG value. Without verify-after-sign the issuer publishes it.
	faulted := &rsa.PrivateKey{
		PublicKey: priv.PublicKey,
		D:         new(big.Int).Add(priv.D, bigOne),
		Primes:    priv.Primes,
	}
	if _, serr := SignBlinded(rand.Reader, faulted, blinded); serr == nil {
		t.Fatal("BREAK C-2: a signing operation that produced the WRONG value was released. " +
			"Verify-after-sign (s^e == b mod N) is the Boneh-DeMillo-Lipton countermeasure " +
			"Go pairs with CRT in decryptAndCheck; it must run before the signature leaves.")
	}
}

// ---------------------------------------------------------------------------
// C-5 — canonical signatures. A valid signature has exactly ONE wire spelling.
// ---------------------------------------------------------------------------

func TestC5_ANonCanonicalEncodingOfAValidSignatureIsRefused(t *testing.T) {
	priv := testKey(t)
	pub := &priv.PublicKey
	serial, _ := NewSerial(rand.Reader)
	blinded, secret, err := Blind(rand.Reader, pub, serial)
	if err != nil {
		t.Fatal(err)
	}
	sig, uerr := Unblind(pub, serial, mustSign(t, priv, blinded), secret)
	if uerr != nil {
		t.Fatal(uerr)
	}
	if !Verify(pub, serial, sig) {
		t.Fatal("setup: the canonical signature must verify")
	}
	for name, bad := range map[string][]byte{
		"zero-padded": append([]byte{0}, sig...),
		"plus N":      new(big.Int).Add(new(big.Int).SetBytes(sig), pub.N).Bytes(),
	} {
		if Verify(pub, serial, bad) {
			t.Fatalf("BREAK C-5: the %s spelling of a valid signature ALSO verifies. "+
				"RFC 8017 §5.2.2 requires the representative to be in [0, N-1]; silt "+
				"additionally requires minimal encoding, so that anything keyed on the raw "+
				"Token.Sig bytes cannot be re-spelled for free", name)
		}
	}
}

// ---------------------------------------------------------------------------
// C-6 — RFC 9474 §4.2 step 4, is_coprime(m, n).
//
// The predicate is gated directly rather than through blindD, and that is the honest
// shape: ValidatePub now rejects every modulus whose factors a test could find (smooth,
// perfect power, prime), and producing a message m with gcd(m, N) > 1 for an honest
// semiprime IS factoring N. So the reachable surface is the predicate.
// ---------------------------------------------------------------------------

func TestC6_CoprimePredicateRefusesASharedFactor(t *testing.T) {
	p, err := rand.Prime(rand.Reader, 1024)
	if err != nil {
		t.Fatal(err)
	}
	q, err := rand.Prime(rand.Reader, 1024)
	if err != nil {
		t.Fatal(err)
	}
	n := new(big.Int).Mul(p, q)
	if coprime(p, n) {
		t.Fatal("BREAK C-6: a message sharing a factor with N was reported coprime. The " +
			"issuer computes gcd(b, N) at issuance and gcd(FDH(E‖serial), N) at redemption " +
			"and LINKS them — a blindness break against a deliberately smooth modulus.")
	}
	if !coprime(new(big.Int).Add(p, bigOne), n) {
		t.Fatal("the predicate refuses a coprime message — it would deny the honest lane")
	}
}

// ---------------------------------------------------------------------------
// C-3 — ValidatePub hardness. The advisory's four shapes, verbatim.
// ---------------------------------------------------------------------------

func TestC3_ValidatePubRefusesTheFourWeakKeyShapes(t *testing.T) {
	honest := testKey(t)

	single, err := rand.Prime(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	// 122 distinct 17-bit primes ≈ 2049 bits, the advisory's smooth modulus.
	smooth := big.NewInt(1)
	seen := map[string]bool{}
	for len(seen) < 122 {
		f, perr := rand.Prime(rand.Reader, 17)
		if perr != nil {
			t.Fatal(perr)
		}
		if seen[f.String()] {
			continue
		}
		seen[f.String()] = true
		smooth.Mul(smooth, f)
	}
	sq, err := rand.Prime(rand.Reader, 1024)
	if err != nil {
		t.Fatal(err)
	}
	square := new(big.Int).Mul(sq, sq)

	for _, tc := range []struct {
		name string
		key  *rsa.PublicKey
		why  string
	}{
		{"one 2048-bit prime", &rsa.PublicKey{N: single, E: 65537},
			"phi(N) = N-1 is public, so ANY holder of the public key computes d and mints tokens"},
		{"122 seventeen-bit primes", &rsa.PublicKey{N: smooth, E: 65537},
			"any node that trial-divides N computes d and mints tokens"},
		{"p squared", &rsa.PublicKey{N: square, E: 65537},
			"a perfect power is factorable"},
		{"e = 3", &rsa.PublicKey{N: honest.N, E: 3},
			"below the FIPS 186-5 2^16 exponent floor; enables the gcd(e, phi(N)) > 1 blindness partition"},
	} {
		if err := ValidatePub(tc.key); err == nil {
			t.Errorf("BREAK C-3: ValidatePub ACCEPTED %q (%d bits) — %s. The consensus "+
				"commitment cannot catch it: it attests sha256(MarshalPub(key)), a binding on "+
				"WHICH BYTES an issuer serves, never on whether they are an unforgeable "+
				"signature scheme.", tc.name, tc.key.N.BitLen(), tc.why)
		} else {
			t.Logf("%-26s REFUSED: %v", tc.name, err)
		}
	}
	// Positive control: every key this tree generates must still be accepted.
	if err := ValidatePub(&honest.PublicKey); err != nil {
		t.Fatalf("an honest crypto/rsa 2048-bit key is now REFUSED (%v) — the hardening "+
			"would take the whole lane down", err)
	}
}

// TestC3_ValidatePubCostBudget lives in crypto_advisory_cost_test.go, behind
// `//go:build !race`: a wall-clock budget measured under the race detector measures the
// detector. Every other gate in this file runs under -race unchanged.

// TestC3_HardnessRunsAtAdmissionNotOnEveryModexp pins the split. The hardness checks
// cost ~3.5 ms; a verify costs ~12 µs. If ValidatePub ever moves back onto the modexp
// path, an attacker who can make a node verify gets a ~300x CPU amplifier on the
// single-threaded node loop — the same lane the N=0 panic reached.
func TestC3_HardnessRunsAtAdmissionNotOnEveryModexp(t *testing.T) {
	priv := testKey(t)
	pub := &priv.PublicKey
	PrewarmValidatePub()
	serial, _ := NewSerial(rand.Reader)
	blinded, secret, err := Blind(rand.Reader, pub, serial)
	if err != nil {
		t.Fatal(err)
	}
	sig, uerr := Unblind(pub, serial, mustSign(t, priv, blinded), secret)
	if uerr != nil {
		t.Fatal(uerr)
	}

	const n = 200
	start := time.Now()
	for i := 0; i < n; i++ {
		if !Verify(pub, serial, sig) {
			t.Fatal("setup: the signature must verify")
		}
	}
	perVerify := time.Since(start) / n

	start = time.Now()
	if verr := ValidatePub(pub); verr != nil {
		t.Fatal(verr)
	}
	admission := time.Since(start)

	t.Logf("MEASURED: verify = %v/call, full admission ValidatePub = %v", perVerify, admission)
	// The bound is deliberately loose (10x, not 300x): it must catch "the hardness
	// checks moved onto the verify path" without becoming a timing-flake on a busy box.
	if perVerify*10 > admission {
		t.Fatalf("a verify costs %v and full key admission costs %v — the hardness checks "+
			"appear to be running on the modexp path. They belong at the ADMISSION doors "+
			"(ParsePub, demand.Keyset.Put); the pre-modexp last line is validateShape.",
			perVerify, admission)
	}
}

// TestC3_ShapeStillRefusesTheDegenerateKeysBeforeEveryModexp is the other half: moving
// the hardness checks off the hot path must NOT re-open the F4 breaks that the
// pre-modexp call closed. Ablating validateShape from blindD/Unblind/verifyD returns
// the N=0 panic and the N=1 universal forgery.
func TestC3_ShapeStillRefusesTheDegenerateKeysBeforeEveryModexp(t *testing.T) {
	for _, tc := range []struct {
		name string
		key  *rsa.PublicKey
	}{
		{"N = 0", &rsa.PublicKey{N: big.NewInt(0), E: 65537}},
		{"N = 1", &rsa.PublicKey{N: big.NewInt(1), E: 65537}},
		{"E = 1", &rsa.PublicKey{N: testKey(t).N, E: 1}},
		{"N even", &rsa.PublicKey{N: new(big.Int).Lsh(bigOne, 2048), E: 65537}},
	} {
		if Verify(tc.key, []byte("serial"), []byte{1}) {
			t.Fatalf("%s: verifyD ACCEPTED a pair under a degenerate key", tc.name)
		}
		if _, _, berr := Blind(rand.Reader, tc.key, []byte("serial")); berr == nil {
			t.Fatalf("%s: blindD ran the modexp against a degenerate modulus", tc.name)
		}
		if _, uerr := Unblind(tc.key, []byte("serial"), []byte{1}, []byte{1}); uerr == nil {
			t.Fatalf("%s: Unblind ran the modexp against a degenerate modulus", tc.name)
		}
	}
}
