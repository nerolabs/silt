package blindtoken

// M3 — THE NETWORK BINDING GATES (research certification 2026-09-11 §3, Layer 3,
// G-3a..G-3c).
//
// THE DEFECT THESE DRIVE. Before M3 the chain-bound lanes signed a message that named no
// network: the credit domain signed `serial`, the demand and relay-anchor domains signed
// `epoch ‖ serial`. Under a shared issuer key a token minted on network A therefore
// verified on network B. This is EIP-155's unlearned lesson.
//
// EVERY GATE HERE IS DRIVEN, AND THE NEGATIVE ARM IS DRIVEN IN BOTH DIRECTIONS: a token
// minted under A is presented under B, and a token minted under B is presented under A.
// A one-direction cross-network probe passes vacuously against an implementation that
// binds nothing but happens to fail for some other reason.

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/binary"
	"math/big"
	"testing"
)

// testChain is the chain id the pre-existing lane tests mint and verify under. It is the
// honest-path value: one network, mint and verify agree, and everything that passed before
// M3 still passes. It is deliberately NOT all-ones, all-zeroes or a repeated byte — a
// uniform fixture hides a copy that takes only the first byte, or only the last.
var testChain = [ChainIDSize]byte{
	0x9c, 0x21, 0x00, 0x4e, 0xff, 0x03, 0x77, 0xa1,
	0x00, 0x00, 0x5b, 0xd2, 0x18, 0x64, 0x90, 0x0c,
	0xe7, 0x33, 0x81, 0x00, 0x2a, 0xbc, 0x00, 0x56,
	0x41, 0x0f, 0x99, 0x3d, 0x00, 0xa8, 0x6e, 0xb4,
}

// chainA and chainB are two DIFFERENT networks. They differ in the LAST byte only, which
// is the case a truncating or prefix-only binding would wrongly pass.
var (
	chainA = [ChainIDSize]byte{
		0x11, 0x9f, 0x40, 0x02, 0xcd, 0x7a, 0x00, 0x63,
		0xb8, 0x25, 0x00, 0xee, 0x14, 0x51, 0x9a, 0x37,
		0x00, 0xd4, 0x6b, 0x82, 0xf0, 0x0c, 0x59, 0x00,
		0xa3, 0x77, 0x1e, 0xc6, 0x38, 0x00, 0x85, 0x2f,
	}
	chainB = [ChainIDSize]byte{
		0x11, 0x9f, 0x40, 0x02, 0xcd, 0x7a, 0x00, 0x63,
		0xb8, 0x25, 0x00, 0xee, 0x14, 0x51, 0x9a, 0x37,
		0x00, 0xd4, 0x6b, 0x82, 0xf0, 0x0c, 0x59, 0x00,
		0xa3, 0x77, 0x1e, 0xc6, 0x38, 0x00, 0x85, 0x30,
	}
)

var m3Lanes = []string{"credit", "demand", "relay"}

// mintUnder is one honest withdrawal end to end under chainID: blind, issuer-sign,
// unblind. The issuer key is the SAME for both networks, which is the premise of the whole
// attack — a shared issuer is what makes a cross-network replay worth anything.
func mintUnder(t *testing.T, priv *rsa.PrivateKey, chainID [ChainIDSize]byte, lane string, epoch uint64, serial []byte) []byte {
	t.Helper()
	pub := &priv.PublicKey
	var blinded, secret []byte
	var err error
	switch lane {
	case "credit":
		blinded, secret, err = BlindCredit(rand.Reader, pub, chainID, serial)
	case "demand":
		blinded, secret, err = BlindDemand(rand.Reader, pub, chainID, epoch, serial)
	case "relay":
		blinded, secret, err = BlindRelayAnchor(rand.Reader, pub, chainID, epoch, serial)
	default:
		t.Fatalf("unknown lane %q", lane)
	}
	if err != nil {
		t.Fatalf("%s: blind under %x: %v", lane, chainID[28:], err)
	}
	blindSig, err := SignBlinded(rand.Reader, priv, blinded)
	if err != nil {
		t.Fatalf("%s: sign: %v", lane, err)
	}
	var sig []byte
	switch lane {
	case "credit":
		sig, err = UnblindCredit(pub, chainID, serial, blindSig, secret)
	case "demand":
		sig, err = UnblindDemand(pub, chainID, epoch, serial, blindSig, secret)
	case "relay":
		sig, err = UnblindRelayAnchor(pub, chainID, epoch, serial, blindSig, secret)
	}
	if err != nil {
		t.Fatalf("%s: unblind under %x: %v", lane, chainID[28:], err)
	}
	return sig
}

func verifyUnder(pub *rsa.PublicKey, chainID [ChainIDSize]byte, lane string, epoch uint64, serial, sig []byte) bool {
	switch lane {
	case "credit":
		return VerifyCredit(pub, chainID, serial, sig)
	case "demand":
		return VerifyDemand(pub, chainID, epoch, serial, sig)
	case "relay":
		return VerifyRelayAnchor(pub, chainID, epoch, serial, sig)
	}
	return false
}

// TestTokenMintedOnOneNetworkFailsOnAnother is G-3a's core: the cross-network replay, in
// BOTH directions, on all three chain-bound lanes, under ONE shared issuer key.
//
// RED BEFORE THE FIX: with the chain id absent from the FDH input every cross arm below
// returned true — the same (serial, sig) verified under chainA and chainB alike.
func TestTokenMintedOnOneNetworkFailsOnAnother(t *testing.T) {
	priv := testKey(t)
	pub := &priv.PublicKey
	const epoch = 7
	for _, lane := range m3Lanes {
		for _, d := range []struct {
			name       string
			mint, show [ChainIDSize]byte
		}{
			{"A-mint-shown-on-B", chainA, chainB},
			{"B-mint-shown-on-A", chainB, chainA},
		} {
			serial := m3Serial(t, lane+d.name)
			sig := mintUnder(t, priv, d.mint, lane, epoch, serial)
			// HONEST SAFETY FIRST: the token must verify on the network it was minted on,
			// or the refusal below proves nothing but a broken mint.
			if !verifyUnder(pub, d.mint, lane, epoch, serial, sig) {
				t.Fatalf("%s/%s: the token does not verify on its OWN network — the cross-network arm would be vacuous", lane, d.name)
			}
			if verifyUnder(pub, d.show, lane, epoch, serial, sig) {
				t.Errorf("%s/%s: a token minted on ...%x VERIFIED on ...%x — the network binding is absent",
					lane, d.name, d.mint[31:], d.show[31:])
			}
		}
	}
}

// TestZeroChainIDRefusesAtEveryBoundEntryPoint is G-3b. A zero chain id means "this node
// holds no chain", and every chainless node holds the same zero — so accepting it would
// make "network zero" a real, shared network. Every bound entry point refuses.
//
// The gate ENUMERATES the entry points rather than sampling one, because the refusal lives
// in each wrapper: a lane added later without the check is exactly the hole this catches.
func TestZeroChainIDRefusesAtEveryBoundEntryPoint(t *testing.T) {
	priv := testKey(t)
	pub := &priv.PublicKey
	var zero [ChainIDSize]byte
	serial := m3Serial(t, "zero-chain")
	const epoch = 3

	// A real signature, minted on a REAL network, so the refusals below are about the zero
	// chain id and not about a malformed input.
	good := mintUnder(t, priv, chainA, "demand", epoch, serial)

	t.Run("blind", func(t *testing.T) {
		if _, _, err := BlindCredit(rand.Reader, pub, zero, serial); err != ErrZeroChainID {
			t.Errorf("BlindCredit under a zero chain id: err = %v, want ErrZeroChainID", err)
		}
		if _, _, err := BlindDemand(rand.Reader, pub, zero, epoch, serial); err != ErrZeroChainID {
			t.Errorf("BlindDemand under a zero chain id: err = %v, want ErrZeroChainID", err)
		}
		if _, _, err := BlindRelayAnchor(rand.Reader, pub, zero, epoch, serial); err != ErrZeroChainID {
			t.Errorf("BlindRelayAnchor under a zero chain id: err = %v, want ErrZeroChainID", err)
		}
	})
	t.Run("unblind", func(t *testing.T) {
		if _, err := UnblindCredit(pub, zero, serial, good, good); err != ErrZeroChainID {
			t.Errorf("UnblindCredit under a zero chain id: err = %v, want ErrZeroChainID", err)
		}
		if _, err := UnblindDemand(pub, zero, epoch, serial, good, good); err != ErrZeroChainID {
			t.Errorf("UnblindDemand under a zero chain id: err = %v, want ErrZeroChainID", err)
		}
		if _, err := UnblindRelayAnchor(pub, zero, epoch, serial, good, good); err != ErrZeroChainID {
			t.Errorf("UnblindRelayAnchor under a zero chain id: err = %v, want ErrZeroChainID", err)
		}
	})
	t.Run("verify", func(t *testing.T) {
		if VerifyCredit(pub, zero, serial, good) {
			t.Error("VerifyCredit accepted a zero chain id")
		}
		if VerifyDemand(pub, zero, epoch, serial, good) {
			t.Error("VerifyDemand accepted a zero chain id")
		}
		if VerifyRelayAnchor(pub, zero, epoch, serial, good) {
			t.Error("VerifyRelayAnchor accepted a zero chain id")
		}
	})
}

// TestZeroChainIDRefusalIsNotJustABadSignature separates the two ways the zero arm could
// pass. A verifier that hashed the zero chain id like any other value would ALSO reject
// the token above — for the wrong reason — and would then happily accept a token genuinely
// MINTED under the zero chain id. Hand-mint one, past the refusal, and show the verifier
// still refuses. That is exactly what a peer running a patched build would present.
func TestZeroChainIDRefusalIsNotJustABadSignature(t *testing.T) {
	priv := testKey(t)
	var zero [ChainIDSize]byte
	serial := m3Serial(t, "zero-minted")
	const epoch = 11

	msg := make([]byte, ChainIDSize, ChainIDSize+8+len(serial))
	copy(msg, zero[:])
	var eb [8]byte
	binary.BigEndian.PutUint64(eb[:], epoch)
	msg = append(msg, eb[:]...)
	msg = append(msg, serial...)
	m := fullDomainHashD(&priv.PublicKey, msg, "silt/blinddemand/fdh/v3")
	sig := new(big.Int).Exp(m, priv.D, priv.N).Bytes()

	// The signature is GENUINE under the zero chain id: prove that before asserting the
	// refusal, or the gate is indistinguishable from "the hand-mint was wrong".
	check := new(big.Int).Exp(new(big.Int).SetBytes(sig), big.NewInt(int64(priv.E)), priv.N)
	if check.Cmp(m) != 0 {
		t.Fatal("setup: the hand-minted zero-chain signature is not genuine — the gate would be vacuous")
	}
	if VerifyDemand(&priv.PublicKey, zero, epoch, serial, sig) {
		t.Error("a token genuinely signed under the ZERO chain id verified — network zero is a network")
	}
}

// TestRetiredDomainVersionsAreRefused is the observability half of the version bump
// (G-3c). The three bound domains moved v1->v2, v2->v3 and v1->v2, and an OLD signature
// must be REFUSED rather than silently re-interpreted under the new layout.
//
// The legacy domains and the legacy message layouts are written out LITERALLY here. They
// are NOT read from the package under test: a gate that takes its guard condition from its
// own subject moves with the subject and stops being a gate.
func TestRetiredDomainVersionsAreRefused(t *testing.T) {
	priv := testKey(t)
	pub := &priv.PublicKey
	const epoch = 5

	epochPrefixed := func(serial []byte) []byte {
		var eb [8]byte
		binary.BigEndian.PutUint64(eb[:], epoch)
		return append(eb[:], serial...)
	}

	for _, c := range []struct {
		name      string
		oldDomain string
		oldMsg    func(serial []byte) []byte
		verifyNow func(serial, sig []byte) bool
		liveNow   string
		wantLive  string
	}{
		{
			name: "credit v1", oldDomain: "silt/blindcredit/fdh/v1",
			oldMsg:    func(s []byte) []byte { return s },
			verifyNow: func(s, sig []byte) bool { return VerifyCredit(pub, testChain, s, sig) },
			liveNow:   creditDomain, wantLive: "silt/blindcredit/fdh/v2",
		},
		{
			name: "demand v2", oldDomain: "silt/blinddemand/fdh/v2",
			oldMsg:    epochPrefixed,
			verifyNow: func(s, sig []byte) bool { return VerifyDemand(pub, testChain, epoch, s, sig) },
			liveNow:   demandDomain, wantLive: "silt/blinddemand/fdh/v3",
		},
		{
			name: "relay anchor v1", oldDomain: "silt/blindrelay/fdh/v1",
			oldMsg:    epochPrefixed,
			verifyNow: func(s, sig []byte) bool { return VerifyRelayAnchor(pub, testChain, epoch, s, sig) },
			liveNow:   relayAnchorDomain, wantLive: "silt/blindrelay/fdh/v2",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			serial := m3Serial(t, c.name)
			old := c.oldMsg(serial)
			m := fullDomainHashD(pub, old, c.oldDomain)
			sig := new(big.Int).Exp(m, priv.D, priv.N).Bytes()
			// The legacy signature is GENUINE in its own domain. Assert that, or a refusal
			// below could just be arithmetic that went wrong.
			check := new(big.Int).Exp(new(big.Int).SetBytes(sig), big.NewInt(int64(pub.E)), pub.N)
			if check.Cmp(m) != 0 {
				t.Fatal("setup: the legacy signature is not genuine in its own domain — the gate would be vacuous")
			}
			if c.verifyNow(serial, sig) {
				t.Errorf("a %s signature still verifies under the current domain — the version bump is not observable", c.oldDomain)
			}
			if c.liveNow != c.wantLive {
				t.Errorf("live domain = %q, want %q", c.liveNow, c.wantLive)
			}
		})
	}
}

// TestChainBoundFDHInputIsPinnedByteExactly re-pins the bound layouts (G-3c). The expected
// bytes are assembled here from first principles — chain id, then the epoch where the lane
// has one, then the serial — never by calling the function that builds them.
func TestChainBoundFDHInputIsPinnedByteExactly(t *testing.T) {
	serial := bytes.Repeat([]byte{0xa7}, SerialSize)
	const epoch uint64 = 0x0102030405060708

	creditWant := append(append([]byte{}, testChain[:]...), serial...)
	got, err := chainBoundMsg(testChain, serial)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, creditWant) {
		t.Errorf("credit FDH input = %x, want %x", got, creditWant)
	}
	if len(got) != ChainIDSize+SerialSize {
		t.Errorf("credit FDH input length = %d, want %d", len(got), ChainIDSize+SerialSize)
	}

	demandWant := append(append(append([]byte{}, testChain[:]...),
		0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08), serial...)
	got, err = chainBoundMsg(testChain, demandMsg(epoch, serial))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, demandWant) {
		t.Errorf("demand FDH input = %x, want %x", got, demandWant)
	}
	if len(got) != ChainIDSize+8+SerialSize {
		t.Errorf("demand FDH input length = %d, want %d", len(got), ChainIDSize+8+SerialSize)
	}
	// The chain id comes FIRST. A layout that appended it would still be injective and
	// would still pass the cross-network probe, so the ORDER is pinned separately.
	if !bytes.Equal(got[:ChainIDSize], testChain[:]) {
		t.Error("the chain id is not the leading field of the FDH input")
	}
}

// TestBoundDomainsAreDistinctUnderOneChain keeps the pre-M3 lane separation honest: adding
// a common prefix to three messages must not collapse the three domains into one. One fee,
// one lane (cert T-6) is a property M3 must not spend.
func TestBoundDomainsAreDistinctUnderOneChain(t *testing.T) {
	priv := testKey(t)
	pub := &priv.PublicKey
	const epoch = 2
	serial := m3Serial(t, "lane-separation")

	demandSig := mintUnder(t, priv, chainA, "demand", epoch, serial)
	if VerifyRelayAnchor(pub, chainA, epoch, serial, demandSig) {
		t.Error("a demand token verified as a relay anchor — one fee, two lanes")
	}
	if VerifyCredit(pub, chainA, serial, demandSig) {
		t.Error("a demand token verified as a publish credit")
	}
	relaySig := mintUnder(t, priv, chainA, "relay", epoch, serial)
	if VerifyDemand(pub, chainA, epoch, serial, relaySig) {
		t.Error("a relay anchor verified as a demand token")
	}
}

func m3Serial(t *testing.T, label string) []byte {
	t.Helper()
	h := sha256.Sum256([]byte("m3/" + label))
	return h[:]
}
