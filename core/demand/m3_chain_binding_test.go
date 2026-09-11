package demand

// M3 — the network binding at the KEYSET tier (research certification 2026-09-11 §3,
// G-3a / G-3b, and the §3.4 hard gate: the chain id is a PARAMETER, never a field).
//
// The blindtoken tier proves the FDH input carries the chain id. This tier proves the
// thing a redeemer actually calls — Keyset.VerifyInWindow and VerifyAnchorInWindow, the
// walk over the held (key_e, e) pairs — refuses a foreign network at EVERY pair rather
// than at one, and refuses to run at all without a network of its own.

import (
	"crypto/rand"
	"crypto/rsa"
	"reflect"
	"testing"

	"github.com/nerolabs/silt/core/blindtoken"
	"github.com/nerolabs/silt/ports"
)

// testChainID is the network the rest of this package's tests mint and verify under: one
// network, honest path, unchanged behaviour. Not a repeated byte — a uniform fixture hides
// a binding that copies only the first byte or only the last.
var testChainID = ports.Hash{
	0x4d, 0x0a, 0xb7, 0x00, 0x61, 0xfe, 0x2c, 0x93,
	0x00, 0x88, 0x15, 0x7d, 0xc0, 0x00, 0x3a, 0xe6,
	0x52, 0x00, 0x9b, 0x41, 0x00, 0xd8, 0x6f, 0x27,
	0x00, 0xba, 0x34, 0x00, 0x71, 0xec, 0x19, 0x5a,
}

// netOne and netTwo are two different networks. They differ in ONE byte, in the middle,
// which is the case both a prefix-only and a suffix-only binding would wrongly pass.
var (
	netOne = ports.Hash{
		0xe1, 0x27, 0x00, 0x4b, 0xa9, 0x00, 0x63, 0xd2,
		0x18, 0x00, 0x7f, 0xcc, 0x35, 0x91, 0x00, 0x4e,
		0x6a, 0xb3, 0x00, 0x22, 0xf5, 0x08, 0x9d, 0x00,
		0x51, 0x00, 0xc7, 0x3e, 0x86, 0x1b, 0x00, 0xfa,
	}
	netTwo = ports.Hash{
		0xe1, 0x27, 0x00, 0x4b, 0xa9, 0x00, 0x63, 0xd2,
		0x18, 0x00, 0x7f, 0xcc, 0x35, 0x91, 0x00, 0x4e,
		0x6a, 0xb3, 0x00, 0x22, 0xf5, 0x08, 0x9d, 0x00,
		0x51, 0x00, 0xc7, 0x3e, 0x86, 0x1b, 0x01, 0xfa,
	}
)

// mintDemandOn runs a real blind withdrawal on network cid at epoch e under key.
func mintDemandOn(t *testing.T, key *rsa.PrivateKey, cid ports.Hash, e uint64, serial []byte) Token {
	t.Helper()
	blinded, secret, err := Withdraw(rand.Reader, &key.PublicKey, cid, e, serial)
	if err != nil {
		t.Fatalf("Withdraw on %x: %v", cid[31:], err)
	}
	blindSig := SignWithdrawal(rand.Reader, key, cid, blinded)
	if len(blindSig) == 0 {
		t.Fatalf("SignWithdrawal on %x returned nothing", cid[31:])
	}
	tok, err := Unblind(&key.PublicKey, cid, e, serial, blindSig, secret)
	if err != nil {
		t.Fatalf("Unblind on %x: %v", cid[31:], err)
	}
	return tok
}

// mintAnchorOn is mintDemandOn for the relay lane.
func mintAnchorOn(t *testing.T, key *rsa.PrivateKey, cid ports.Hash, e uint64, serial []byte) Token {
	t.Helper()
	blinded, secret, err := blindtoken.BlindRelayAnchor(rand.Reader, &key.PublicKey, cid, e, serial)
	if err != nil {
		t.Fatalf("BlindRelayAnchor on %x: %v", cid[31:], err)
	}
	blindSig, err := blindtoken.SignBlinded(rand.Reader, key, blinded)
	if err != nil {
		t.Fatalf("SignBlinded: %v", err)
	}
	sig, err := blindtoken.UnblindRelayAnchor(&key.PublicKey, cid, e, serial, blindSig, secret)
	if err != nil {
		t.Fatalf("UnblindRelayAnchor on %x: %v", cid[31:], err)
	}
	return Token{Serial: serial, Sig: sig}
}

func m3Keyset(t *testing.T, key *rsa.PrivateKey, epochs ...uint64) *Keyset {
	t.Helper()
	ks := NewKeyset(DefaultWindow)
	for _, e := range epochs {
		ks.Put(e, &key.PublicKey)
	}
	return ks
}

// TestKeysetRefusesATokenFromAnotherNetwork is G-3a at the redeemer. The issuer key is the
// SAME on both networks — that is the whole premise — and the keyset holds it for every
// epoch in the window, so the refusal cannot come from a missing key.
//
// RED BEFORE THE FIX: VerifyInWindow returned (epoch, true) for the foreign token.
func TestKeysetRefusesATokenFromAnotherNetwork(t *testing.T) {
	key := m3Key(t)
	const cur = 4
	ks := m3Keyset(t, key, 0, 1, 2, 3, 4)

	for _, d := range []struct {
		name       string
		mint, show ports.Hash
	}{
		{"one-mint-shown-on-two", netOne, netTwo},
		{"two-mint-shown-on-one", netTwo, netOne},
	} {
		t.Run(d.name, func(t *testing.T) {
			serial := m3Serial(t, "demand/"+d.name)
			tok := mintDemandOn(t, key, d.mint, 2, serial)
			// HONEST SAFETY: the token redeems on its own network, at its own epoch,
			// through the same window walk. Without this the refusal below is vacuous.
			if e, ok := ks.VerifyInWindow(d.mint, cur, tok); !ok || e != 2 {
				t.Fatalf("the token does not redeem on its OWN network (epoch %d, ok %v)", e, ok)
			}
			if e, ok := ks.VerifyInWindow(d.show, cur, tok); ok {
				t.Errorf("a token minted on ...%x redeemed on ...%x at epoch %d — the network binding is absent",
					d.mint[30:], d.show[30:], e)
			}

			aserial := m3Serial(t, "anchor/"+d.name)
			anc := mintAnchorOn(t, key, d.mint, 2, aserial)
			if e, ok := ks.VerifyAnchorInWindow(d.mint, cur, anc); !ok || e != 2 {
				t.Fatalf("the anchor does not verify on its OWN network (epoch %d, ok %v)", e, ok)
			}
			if e, ok := ks.VerifyAnchorInWindow(d.show, cur, anc); ok {
				t.Errorf("an anchor minted on ...%x verified on ...%x at epoch %d — the network binding is absent",
					d.mint[30:], d.show[30:], e)
			}
		})
	}
}

// TestKeysetRefusesEveryHeldPairNotJustTheCurrentOne closes the walk. VerifyInWindow tries
// up to W+1 pairs newest-first; a binding checked at only one epoch would let a foreign
// token in at another. Mint the foreign token at EVERY epoch in the window and show none
// of them lands.
func TestKeysetRefusesEveryHeldPairNotJustTheCurrentOne(t *testing.T) {
	key := m3Key(t)
	const cur = DefaultWindow
	epochs := make([]uint64, 0, cur+1)
	for e := uint64(0); e <= cur; e++ {
		epochs = append(epochs, e)
	}
	ks := m3Keyset(t, key, epochs...)

	for _, e := range epochs {
		serial := m3Serial(t, "walk/"+string(rune('a'+e)))
		tok := mintDemandOn(t, key, netOne, e, serial)
		if got, ok := ks.VerifyInWindow(netOne, cur, tok); !ok || got != e {
			t.Fatalf("epoch %d: the token does not redeem on its own network (got %d, ok %v)", e, got, ok)
		}
		if got, ok := ks.VerifyInWindow(netTwo, cur, tok); ok {
			t.Errorf("epoch %d: a foreign-network token redeemed at pair %d", e, got)
		}
	}
}

// TestZeroChainIDRefusesAtTheRedeemerAndTheIssuer is G-3b at this tier. A node holding no
// chain reports the zero hash, and every chainless node reports the same one: accepting it
// would make "network zero" a shared network.
func TestZeroChainIDRefusesAtTheRedeemerAndTheIssuer(t *testing.T) {
	key := m3Key(t)
	var zero ports.Hash
	ks := m3Keyset(t, key, 0, 1, 2)
	serial := m3Serial(t, "zero")
	tok := mintDemandOn(t, key, netOne, 1, serial)

	if _, ok := ks.VerifyInWindow(zero, 2, tok); ok {
		t.Error("VerifyInWindow accepted a token under a ZERO chain id")
	}
	if _, ok := ks.VerifyAnchorInWindow(zero, 2, mintAnchorOn(t, key, netOne, 1, m3Serial(t, "zero-anchor"))); ok {
		t.Error("VerifyAnchorInWindow accepted an anchor under a ZERO chain id")
	}
	if !VerifyToken(&key.PublicKey, netOne, 1, tok) {
		t.Fatal("honest safety: the token does not verify on its own network")
	}
	if VerifyToken(&key.PublicKey, zero, 1, tok) {
		t.Error("VerifyToken accepted a ZERO chain id")
	}

	// The ISSUER arm: an issuer that does not know its network does not sign. The blinded
	// value is a real one, so the refusal is about the chain id and nothing else.
	blinded, _, err := Withdraw(rand.Reader, &key.PublicKey, netOne, 1, m3Serial(t, "issuer-zero"))
	if err != nil {
		t.Fatal(err)
	}
	if sig := SignWithdrawal(rand.Reader, key, netOne, blinded); len(sig) == 0 {
		t.Fatal("honest safety: SignWithdrawal refused a good withdrawal on a real network")
	}
	if sig := SignWithdrawal(rand.Reader, key, zero, blinded); len(sig) != 0 {
		t.Error("SignWithdrawal signed while holding a ZERO chain id — a chainless issuer must not be a signing oracle")
	}
	if _, _, err := Withdraw(rand.Reader, &key.PublicKey, zero, 1, serial); err != blindtoken.ErrZeroChainID {
		t.Errorf("Withdraw under a zero chain id: err = %v, want ErrZeroChainID", err)
	}
}

// TestTokenCarriesNoChainIDField is the §3.4 hard gate made STRUCTURAL. The certification
// is explicit that the chain id must be a parameter of the issuer and the verifier and
// never a field of the request, the token or the wire: a field is an attacker input and
// the binding would be decoration.
//
// Enumerating the fields is the check. If someone later adds `ChainID` to Token — the
// natural way to "make the binding explicit on the wire" — this goes red before the
// decoration ships. The redeemer must keep reading the network from its OWN chain.
func TestTokenCarriesNoChainIDField(t *testing.T) {
	want := []string{"Serial", "Sig"}
	ty := reflect.TypeOf(Token{})
	if ty.NumField() != len(want) {
		t.Fatalf("demand.Token has %d fields, want exactly %v — a chain id on the token is an attacker input, not a binding", ty.NumField(), want)
	}
	for i, name := range want {
		if got := ty.Field(i).Name; got != name {
			t.Errorf("demand.Token field %d = %q, want %q", i, got, name)
		}
	}
}

func m3Key(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	k, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	return k
}

func m3Serial(t *testing.T, label string) []byte {
	t.Helper()
	h := ports.HashBytes([]byte("m3/demand/" + label))
	return h[:]
}
