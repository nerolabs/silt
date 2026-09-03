package blindtoken

// R0.4b (b1) — the ISSUE EPOCH is inside the blind-signed message.
//
// These are the primitive-level gates for the fix the red-team reconciliation verdict
// certified (2026-09-02, §2.4): a demand signature must verify under exactly the pair
// (key_E, E). Everything above this file — the keyset window, the credit layer's
// expiry guard, the whole "evicted ⇒ expired ⇒ un-redeemable" coupling — is only as
// true as these three properties.
//
// ABLATION (drop the epoch from the FDH input, i.e. make demandMsg return the serial):
// TestDemandSignatureDoesNotVerifyAtAnotherEpoch and
// TestDemandFDHInputBindsTheEpochByteExactly both go RED. The credit-layer pump gate
// (core/credit TestGuardHealsUnderASharedKey) goes RED with them.

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"

	"crypto/sha256"
	"encoding/binary"
	"math/big"
	"testing"
)

// signDemandAt runs a full blind withdrawal for issue epoch e and returns the
// unblinded signature on serial.
func signDemandAt(t *testing.T, k *rsa.PrivateKey, e uint64, serial []byte) []byte {
	t.Helper()
	blinded, secret, err := BlindDemand(rand.Reader, &k.PublicKey, e, serial)
	if err != nil {
		t.Fatalf("blind: %v", err)
	}
	sig, err := UnblindDemand(&k.PublicKey, e, serial, mustSign(t, k, blinded), secret)
	if err != nil {
		t.Fatalf("unblind: %v", err)
	}
	return sig
}

// TestDemandSignatureVerifiesAtItsOwnEpoch is the liveness half: the honest pair works.
func TestDemandSignatureVerifiesAtItsOwnEpoch(t *testing.T) {
	k := testKey(t)
	serial, _ := NewSerial(rand.Reader)
	for _, e := range []uint64{0, 1, 4, 12345} {
		if !VerifyDemand(&k.PublicKey, e, serial, signDemandAt(t, k, e, serial)) {
			t.Fatalf("a demand token withdrawn for epoch %d does not verify at that epoch", e)
		}
	}
}

// TestDemandSignatureDoesNotVerifyAtAnotherEpoch is THE property (b1) exists for.
// Under the SAME key — the case a persisted key re-registered every epoch produces,
// which nothing forbids and an ordinary restart causes — a token from epoch E must
// not verify at any other epoch. Without it, issuedEpoch(token) is a function of the
// verifier's keyset rather than of the token, guard entries expire while tokens do
// not, and the cross-server double-redeem pump re-opens (red-team probes G and I).
func TestDemandSignatureDoesNotVerifyAtAnotherEpoch(t *testing.T) {
	k := testKey(t)
	serial, _ := NewSerial(rand.Reader)
	sig := signDemandAt(t, k, 0, serial)
	for _, e := range []uint64{1, 2, 3, 4, 5, 100} {
		if VerifyDemand(&k.PublicKey, e, serial, sig) {
			t.Fatalf("an epoch-0 token verified at epoch %d under the SAME key — the "+
				"issue epoch is not bound into the signed message, so a token can be "+
				"re-dated and expiry is a no-op", e)
		}
	}
}

// TestDemandFDHInputBindsTheEpochByteExactly pins the wire-level encoding of the
// signed message. The encoding is a compatibility surface (two silt binaries must
// agree byte-for-byte or every cross-node redemption fails), so it is pinned against
// an INDEPENDENTLY recomputed expectation, not against the implementation.
func TestDemandFDHInputBindsTheEpochByteExactly(t *testing.T) {
	k := testKey(t)
	serial := []byte("0123456789abcdef0123456789abcdef")
	const epoch = uint64(0x0102030405060708)

	// Independent recomputation of H(domain ‖ ctr ‖ epoch(8B BE) ‖ serial), counter
	// mode, expanded past the modulus and reduced mod N.
	pub := &k.PublicKey
	nLen := (pub.N.BitLen() + 7) / 8
	var out []byte
	for ctr := uint32(0); len(out) < nLen+8; ctr++ {
		h := sha256.New()
		h.Write([]byte("silt/blinddemand/fdh/v2"))
		var cb [4]byte
		binary.BigEndian.PutUint32(cb[:], ctr)
		h.Write(cb[:])
		var eb [8]byte
		binary.BigEndian.PutUint64(eb[:], epoch)
		h.Write(eb[:])
		h.Write(serial)
		out = h.Sum(out)
	}
	want := new(big.Int).SetBytes(out)
	want.Mod(want, pub.N)

	if got := fullDomainHashD(pub, demandMsg(epoch, serial), demandDomain); got.Cmp(want) != 0 {
		t.Fatal("the demand FDH input is not domain ‖ ctr ‖ epoch(8B big-endian) ‖ serial — " +
			"two binaries that disagree here cannot redeem each other's tokens")
	}
	// And the message itself is exactly the 8-byte big-endian epoch then the serial.
	var eb [8]byte
	binary.BigEndian.PutUint64(eb[:], epoch)
	if !bytes.Equal(demandMsg(epoch, serial), append(eb[:], serial...)) {
		t.Fatal("demandMsg is not epoch(8B big-endian) ‖ serial")
	}
}

// TestDemandDomainStillSeparatesFromPublishAndCredit: binding the epoch must not
// weaken the three-way domain separation one issuer key relies on. A publish token, a
// credit, and a demand token stay mutually unusable.
func TestDemandDomainStillSeparatesFromPublishAndCredit(t *testing.T) {
	k := testKey(t)
	serial, _ := NewSerial(rand.Reader)

	pb, ps, _ := Blind(rand.Reader, &k.PublicKey, serial)
	publishSig := mustUnblind(t, &k.PublicKey, serial, mustSign(t, k, pb), ps)
	cb, cs, _ := BlindCredit(rand.Reader, &k.PublicKey, serial)
	creditSig, cerr := UnblindCredit(&k.PublicKey, serial, mustSign(t, k, cb), cs)
	if cerr != nil {
		t.Fatalf("unblind credit: %v", cerr)
	}
	demandSig := signDemandAt(t, k, 0, serial)

	if VerifyDemand(&k.PublicKey, 0, serial, publishSig) || VerifyDemand(&k.PublicKey, 0, serial, creditSig) {
		t.Fatal("a publish token or credit verified as a demand token")
	}
	if Verify(&k.PublicKey, serial, demandSig) || VerifyCredit(&k.PublicKey, serial, demandSig) {
		t.Fatal("a demand token verified as a publish token or credit")
	}
}

// TestPublishAndCreditFDHAreUnchanged: the publish and credit domains must hash
// EXACTLY as before (v1, no epoch). Committed publish tokens are re-verified against
// the issuer key on every chain replay, so a change here would invalidate history.
func TestPublishAndCreditFDHAreUnchanged(t *testing.T) {
	k := testKey(t)
	serial := []byte("0123456789abcdef0123456789abcdef")
	for _, tc := range []struct{ domain, name string }{
		{"silt/blindtoken/fdh/v1", "publish"},
		{"silt/blindcredit/fdh/v1", "credit"},
	} {
		pub := &k.PublicKey
		nLen := (pub.N.BitLen() + 7) / 8
		var out []byte
		for ctr := uint32(0); len(out) < nLen+8; ctr++ {
			h := sha256.New()
			h.Write([]byte(tc.domain))
			var cb [4]byte
			binary.BigEndian.PutUint32(cb[:], ctr)
			h.Write(cb[:])
			h.Write(serial)
			out = h.Sum(out)
		}
		want := new(big.Int).SetBytes(out)
		want.Mod(want, pub.N)
		if got := fullDomainHashD(pub, serial, tc.domain); got.Cmp(want) != 0 {
			t.Fatalf("the %s FDH input changed — committed publish tokens re-verify "+
				"against this on every replay", tc.name)
		}
	}
}
