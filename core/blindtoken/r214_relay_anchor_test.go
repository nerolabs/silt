package blindtoken

// R2.14 — the relay-lane prepayment anchor: the PRIMITIVE-tier RED-first gates
// (Tester, 2026-09-04). Binding spec:
// silt-reviews/research/research-outcome/R2.14-relay-prepayment-anchor-CONSTRUCTION-RESEARCH-CERTIFICATION-2026-09-04.md
// §6 (one RSA key, two FDH domains — sound by BNPS one-more-inversion), §9 T-6 and
// T-14, §5 (the domain string is a FORMAT CONSTANT the T-6 proof depends on: "pin
// byte-exact (the TestDemandFDHInputBindsTheEpochByteExactly twin); a change is a
// token-format version, never an edit"). Build shape:
// silt-reviews/crypto-specialist/ADVISORY-R2.14-relay-prepayment-anchor-build-2026-09-04.md
// §1.1 (domain = "silt/blindrelay/fdh/v1", message = uint64BE(issueEpoch) ‖ serial,
// byte-identical to demandMsg) and §5 step 1 (three one-line wrappers over
// blindD / unblindD / verifyD).
//
// ABLATIONS that must redden (cert §9): remove the domain string (T-6, T-14).
//
// Every test here is RED on main: no relay-anchor primitive exists. They pass
// only once BlindRelayAnchor / UnblindRelayAnchor / VerifyRelayAnchor exist under
// the byte-exact domain below.

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/binary"
	"math/big"
	"testing"
)

// relayAnchorDomainLiteral is the certified format constant, written out here
// INDEPENDENTLY of the implementation so the pin is against the certification,
// not against whatever the code happens to say.
const relayAnchorDomainLiteral = "silt/blindrelay/fdh/v1"

// independentRelayFDH recomputes H(domain ‖ ctr ‖ epoch(8B BE) ‖ serial) in counter
// mode, expanded past the modulus and reduced mod N — the exact
// TestDemandFDHInputBindsTheEpochByteExactly recomputation, under the relay domain.
func independentRelayFDH(pub *rsa.PublicKey, epoch uint64, serial []byte) *big.Int {
	nLen := (pub.N.BitLen() + 7) / 8
	var out []byte
	for ctr := uint32(0); len(out) < nLen+8; ctr++ {
		h := sha256.New()
		h.Write([]byte(relayAnchorDomainLiteral))
		var cb [4]byte
		binary.BigEndian.PutUint32(cb[:], ctr)
		h.Write(cb[:])
		var eb [8]byte
		binary.BigEndian.PutUint64(eb[:], epoch)
		h.Write(eb[:])
		h.Write(serial)
		out = h.Sum(out)
	}
	m := new(big.Int).SetBytes(out)
	return m.Mod(m, pub.N)
}

// signRelayAnchorAt runs a full blind withdrawal in the RELAY domain for issue
// epoch e and returns the unblinded signature on serial.
func signRelayAnchorAt(t *testing.T, k *rsa.PrivateKey, e uint64, serial []byte) []byte {
	t.Helper()
	blinded, secret, err := BlindRelayAnchor(rand.Reader, &k.PublicKey, e, serial)
	if err != nil {
		t.Fatalf("BlindRelayAnchor: %v", err)
	}
	sig, err := UnblindRelayAnchor(&k.PublicKey, e, serial, mustSign(t, k, blinded), secret)
	if err != nil {
		t.Fatalf("UnblindRelayAnchor: %v", err)
	}
	return sig
}

// TestRelayAnchorDomainIsPinnedByteExactly is T-14 (cert §9): the FDH input under
// "silt/blindrelay/fdh/v1" is uint64BE(epoch) ‖ serial, byte-pinned against an
// INDEPENDENT recomputation, exactly as the demand twin pins its own domain.
//
// Four halves, each load-bearing:
//  1. the constant is the certified literal (a rename is a token-format version);
//  2. the implementation's FDH over demandMsg(epoch, serial) under that constant
//     equals the independent recomputation;
//  3. a RAW RSA signature m^d mod N over the independent FDH — made with no
//     blind/unblind code at all — verifies under VerifyRelayAnchor. This pins the
//     VERIFY path to the byte-exact input, so a Builder who wires blind and verify
//     to a private encoding that agrees with itself but not with the spec still
//     goes RED;
//  4. that same raw signature fails under every other domain (Verify, VerifyCredit,
//     VerifyDemand) — the four domains pairwise distinct (the T-6 precondition).
func TestRelayAnchorDomainIsPinnedByteExactly(t *testing.T) {
	if relayAnchorDomain != relayAnchorDomainLiteral {
		t.Fatalf("relayAnchorDomain = %q, want the certified format constant %q (cert §5: a change is a token-format version, never an edit)", relayAnchorDomain, relayAnchorDomainLiteral)
	}
	for _, other := range []struct{ name, v string }{{"publish", fdhDomain}, {"credit", creditDomain}, {"demand", demandDomain}} {
		if relayAnchorDomain == other.v {
			t.Fatalf("relayAnchorDomain collides with the %s domain %q — one signature would verify in two lanes (cert §2.4 door (iii))", other.name, other.v)
		}
	}

	k := testKey(t)
	pub := &k.PublicKey
	serial := []byte("0123456789abcdef0123456789abcdef")
	const epoch = uint64(0x0102030405060708)

	want := independentRelayFDH(pub, epoch, serial)
	if got := fullDomainHashD(pub, demandMsg(epoch, serial), relayAnchorDomain); got.Cmp(want) != 0 {
		t.Fatal("the relay-anchor FDH input is not \"silt/blindrelay/fdh/v1\" ‖ ctr ‖ epoch(8B big-endian) ‖ serial — " +
			"two binaries that disagree here cannot verify each other's anchors")
	}

	// Raw RSA over the independent FDH: sig = m^d mod N, minimal big-endian bytes.
	rawSig := new(big.Int).Exp(want, k.D, k.N).Bytes()
	if !VerifyRelayAnchor(pub, epoch, serial, rawSig) {
		t.Fatal("VerifyRelayAnchor rejects a raw RSA signature over the independently recomputed " +
			"relay-domain FDH — the verify path is not pinned to uint64BE(epoch) ‖ serial under \"silt/blindrelay/fdh/v1\"")
	}
	if Verify(pub, serial, rawSig) || VerifyCredit(pub, serial, rawSig) || VerifyDemand(pub, epoch, serial, rawSig) {
		t.Fatal("a relay-anchor signature verified as a publish token, a credit, or a demand token — domain separation is broken")
	}
}

// TestRelayAnchorSignatureIsNotADemandSignature is the primitive half of T-6
// (cert §9; the node-tier half is core/node TestRelayAnchorDomainIsNotADemandToken):
// under the SAME key_E, the SAME epoch and the SAME serial, an anchor signature
// fails VerifyDemand and a demand signature fails VerifyRelayAnchor. This is what
// makes one 50,000 fee unable to pay both the delivery lane and the relay lane
// (advisory finding A; cert §6.3: one signature, one domain).
func TestRelayAnchorSignatureIsNotADemandSignature(t *testing.T) {
	k := testKey(t)
	pub := &k.PublicKey
	serial, _ := NewSerial(rand.Reader)
	const epoch = uint64(3)

	anchorSig := signRelayAnchorAt(t, k, epoch, serial)
	if !VerifyRelayAnchor(pub, epoch, serial, anchorSig) {
		t.Fatal("liveness: an honest relay anchor does not verify at its own epoch")
	}
	if VerifyDemand(pub, epoch, serial, anchorSig) {
		t.Fatal("a relay ANCHOR signature verified as a DEMAND token under the same key/epoch/serial — one fee could pay two lanes")
	}
	if Verify(pub, serial, anchorSig) || VerifyCredit(pub, serial, anchorSig) {
		t.Fatal("a relay anchor signature verified as a publish token or a credit")
	}

	demandSig := signDemandAt(t, k, epoch, serial)
	if VerifyRelayAnchor(pub, epoch, serial, demandSig) {
		t.Fatal("a DEMAND token signature verified as a relay ANCHOR under the same key/epoch/serial — a delivery token could open a paid relay session")
	}
}

// TestRelayAnchorSignatureDoesNotVerifyAtAnotherEpoch transfers R0.4b (b1) to the
// anchor lane (advisory §4.4: replay across epochs / re-dating). The issue epoch is
// inside the signed message, so an anchor verifies under exactly the pair
// (key_E, E) — which is what lets the guard key on (epoch, serial) and expire with
// the keyset (cert §5, gate T-12).
func TestRelayAnchorSignatureDoesNotVerifyAtAnotherEpoch(t *testing.T) {
	k := testKey(t)
	pub := &k.PublicKey
	serial, _ := NewSerial(rand.Reader)
	sig := signRelayAnchorAt(t, k, 0, serial)
	if !VerifyRelayAnchor(pub, 0, serial, sig) {
		t.Fatal("liveness: an epoch-0 anchor does not verify at epoch 0")
	}
	for _, e := range []uint64{1, 2, 3, 4, 5, 100} {
		if VerifyRelayAnchor(pub, e, serial, sig) {
			t.Fatalf("an epoch-0 anchor verified at epoch %d under the SAME key — the issue epoch is not bound into the anchor's signed message, so an anchor can be re-dated past its guard entry", e)
		}
	}
	// The message layout the anchor is signed over is the demand layout, byte for byte.
	var eb [8]byte
	binary.BigEndian.PutUint64(eb[:], 7)
	if !bytes.Equal(demandMsg(7, serial), append(eb[:], serial...)) {
		t.Fatal("demandMsg (the anchor's message layout) is not epoch(8B big-endian) ‖ serial")
	}
}
