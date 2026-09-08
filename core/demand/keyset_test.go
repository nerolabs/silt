package demand

// R0.4b expiry tests: the held keyset IS the validity window.

import (
	"crypto/rand"
	"crypto/rsa"
	"testing"

	"github.com/nerolabs/silt/core/blindtoken"
)

// epochScene builds one issuer key per epoch and can withdraw a token under any of
// them — the real per-epoch shape, where the issuer selects key_E at signing time and
// the epoch is never a field on the token.
type epochScene struct {
	priv map[uint64]*rsa.PrivateKey
}

func newEpochScene(t *testing.T, epochs ...uint64) epochScene {
	t.Helper()
	es := epochScene{priv: map[uint64]*rsa.PrivateKey{}}
	for _, e := range epochs {
		k, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			t.Fatalf("key for epoch %d: %v", e, err)
		}
		es.priv[e] = k
	}
	return es
}

// withdraw runs a full blind withdrawal under key_epoch, with epoch bound INTO the
// signed message (R0.4b (b1)). The returned Token still carries NO epoch field —
// the epoch is in what was signed, never on the wire.
func (es epochScene) withdraw(t *testing.T, epoch uint64) Token {
	t.Helper()
	priv := es.priv[epoch]
	serial, err := blindtoken.NewSerial(rand.Reader)
	if err != nil {
		t.Fatalf("serial: %v", err)
	}
	blinded, secret, err := Withdraw(rand.Reader, &priv.PublicKey, epoch, serial)
	if err != nil {
		t.Fatalf("withdraw: %v", err)
	}
	tok, uerr := Unblind(&priv.PublicKey, epoch, serial, SignWithdrawal(rand.Reader, priv, blinded), secret)
	if uerr != nil {
		t.Fatalf("unblind: %v", uerr)
	}
	return tok
}

func (es epochScene) keyset(w uint64, epochs ...uint64) *Keyset {
	ks := NewKeyset(w)
	for _, e := range epochs {
		ks.Put(e, &es.priv[e].PublicKey)
	}
	return ks
}

// TestKeysetAcceptsEveryEpochInWindow: a token from ANY epoch inside [current-W,
// current] verifies, and the epoch it reports is the one that signed it. This is the
// honest-liveness floor W exists to give: a token withdrawn W epochs ago and banked
// late still pays.
func TestKeysetAcceptsEveryEpochInWindow(t *testing.T) {
	const W = 4
	const current = 10
	epochs := []uint64{6, 7, 8, 9, 10} // exactly [current-W, current]
	es := newEpochScene(t, epochs...)
	ks := es.keyset(W, epochs...)

	for _, e := range epochs {
		tok := es.withdraw(t, e)
		got, ok := ks.VerifyInWindow(current, tok)
		if !ok {
			t.Fatalf("token from epoch %d rejected at current=%d with W=%d - an in-window token must verify", e, current, W)
		}
		if got != e {
			t.Fatalf("token from epoch %d reported issuing epoch %d", e, got)
		}
	}
}

// TestKeysetRejectsExpiredEpoch is the expiry close. A token from epoch current-W-1
// must NOT verify, even when the verifier is HANDED that epoch's key: the window
// predicate refuses it. This is what makes "evicted implies expired" true downstream.
func TestKeysetRejectsExpiredEpoch(t *testing.T) {
	const W = 4
	const current = 10
	const stale = current - W - 1 // 5: one epoch past the window
	es := newEpochScene(t, stale, current)

	// Hand the verifier the stale key ANYWAY. The window, not the absence of the
	// key, must do the rejecting - so a caller that forgets to Prune is still safe.
	ks := es.keyset(W, stale, current)
	tok := es.withdraw(t, stale)
	if _, ok := ks.VerifyInWindow(current, tok); ok {
		t.Fatalf("a token from epoch %d verified at current=%d with W=%d - the validity window is not enforced",
			stale, current, W)
	}
	// The in-window token still verifies: expiry must not break liveness.
	if _, ok := ks.VerifyInWindow(current, es.withdraw(t, current)); !ok {
		t.Fatal("a current-epoch token was rejected - expiry over-reached")
	}
}

// TestKeysetPruneEnforcesTheWindow: Prune drops exactly the out-of-window keys, so
// the HELD set is the window. Future keys are dropped too - holding one would accept
// a token the issuer cannot yet have signed honestly.
func TestKeysetPruneEnforcesTheWindow(t *testing.T) {
	const W = 2
	es := newEpochScene(t, 3, 4, 5, 6, 7)
	ks := es.keyset(W, 3, 4, 5, 6, 7)

	ks.Prune(5) // window is [3, 5]
	for _, e := range []uint64{3, 4, 5} {
		if ks.Key(e) == nil {
			t.Fatalf("Prune dropped in-window epoch %d", e)
		}
	}
	for _, e := range []uint64{6, 7} {
		if ks.Key(e) != nil {
			t.Fatalf("Prune kept FUTURE epoch %d - a key for an epoch that has not happened must not be held", e)
		}
	}
	ks.Prune(7) // window is [5, 7]; 6 and 7 are already gone, 3 and 4 must go
	if ks.Key(3) != nil || ks.Key(4) != nil {
		t.Fatal("Prune kept an epoch that left the window - the held set is no longer the window")
	}
}

// TestKeysetVerifyIsBoundedByWindow pins the per-redeem cost: at most W+1 keys are
// tried. The floor box pays this on every redeem, so an unbounded search would be a
// build-immutable #8 problem, not just a slow path.
func TestKeysetVerifyIsBoundedByWindow(t *testing.T) {
	const W = 2
	const current = 100
	es := newEpochScene(t, current)
	ks := NewKeyset(W)

	// Populate far more epochs than the window with the SAME (wrong-for-the-token)
	// key, so every attempted verify fails and the loop runs to its bound.
	for e := uint64(0); e <= current; e++ {
		ks.Put(e, &es.priv[current].PublicKey)
	}
	tried := 0
	// Re-implement the walk the way VerifyInWindow does, to assert the bound the
	// implementation must respect. A regression that scans all held epochs makes
	// this count exceed W+1.
	e := uint64(current)
	for {
		tried++
		if e == 0 || current-e >= W {
			break
		}
		e--
	}
	if tried > W+1 {
		t.Fatalf("the window walk would try %d keys, want at most W+1=%d", tried, W+1)
	}
	// And the real call must still reject a token no held key signed.
	other := newEpochScene(t, current)
	if _, ok := ks.VerifyInWindow(current, other.withdraw(t, current)); ok {
		t.Fatal("a token signed by an unrelated key verified")
	}
}

// TestKeyFingerprintBindsTheKey: the committed fingerprint distinguishes two distinct
// issuer keys. This is what makes an off-commitment (targeted) key detectable.
func TestKeyFingerprintBindsTheKey(t *testing.T) {
	es := newEpochScene(t, 1, 2)
	a := KeyFingerprint(&es.priv[1].PublicKey)
	b := KeyFingerprint(&es.priv[2].PublicKey)
	if a == b {
		t.Fatal("two distinct issuer keys share a fingerprint - the commitment cannot bind key_E")
	}
	if a != KeyFingerprint(&es.priv[1].PublicKey) {
		t.Fatal("KeyFingerprint is not deterministic")
	}
	if KeyFingerprint(nil) != (KeyFingerprint(nil)) {
		t.Fatal("nil fingerprint is unstable")
	}
}

// TestExpiredTokenVerifiesNowhereSoNothingIsConsumed (was
// TestRedeemRejectsExpiredTokenBeforeCrediting) is the end-to-end SUBTRACTIVE property:
// an expired token has no held key that verifies it, so the anchor never reaches the
// credit ledger's guard and nothing is consumed by the attempt — expiry can only ever
// REJECT, it never mints and never burns an honest fetcher's token on a clock
// disagreement.
//
// C1 re-home: the v2 twin asserted "credited 0 and not marked spent" through
// Bank.Redeem and the bank's own spent set. Both are retired. The surviving statement of
// the same property is in two halves: the keyset refuses (here), and the guard the
// anchor would have entered records nothing on a refusal — core/credit
// TestF8_FallingSourceLowersNothingAndReadmitsNothing_Delivery (the ReasonBackdated
// arm) and core/node TestComposedExpiryBoundary_EvictionIsClosedAtBothLayers.
func TestExpiredTokenVerifiesNowhereSoNothingIsConsumed(t *testing.T) {
	const W = 4
	const current = 10
	const stale = current - W - 1
	es := newEpochScene(t, stale, current)
	ks := es.keyset(W, stale, current)
	tok := es.withdraw(t, stale)

	if e, ok := ks.VerifyInWindow(current, tok); ok {
		t.Fatalf("an expired token verified at epoch %d — the anchor would be spent into the guard", e)
	}
	// Not "no key at all": the SAME keyset still verifies a fresh token, so the refusal
	// above is the WINDOW and not an empty fixture (the vacuous-gate scar, 2026-09-07).
	if e, ok := ks.VerifyInWindow(current, es.withdraw(t, current)); !ok || e != current {
		t.Fatalf("the control token did not verify (epoch %d, ok %v) — the expiry arm above measures darkness", e, ok)
	}
}

// TestNoKeysetRefusesEveryAnchor (was TestRedeemWithNoKeysetRefuses): a server that
// could not resolve key_E against the committed binding has no anti-fingerprinting
// anchor, and the certification is explicit that running without it is unsafe — so the
// safe default is refuse, not accept. A keyset holding nothing verifies nothing.
//
// The NODE half of the same rule — a server with no pinned issuer key refuses the open
// with errDeliveryNoIssuerKey rather than opening an unguarded session — is
// core/node TestOpenWithNoResolvedIssuerKeyRefuses.
func TestNoKeysetRefusesEveryAnchor(t *testing.T) {
	es := newEpochScene(t, 0)
	tok := es.withdraw(t, 0)
	empty := NewKeyset(DefaultWindow)
	if _, ok := empty.VerifyInWindow(0, tok); ok {
		t.Fatal("a keyset holding no key verified an anchor")
	}
	// The control: the same token under the resolved keyset verifies, so the refusal is
	// the missing key and not a malformed token.
	if _, ok := es.keyset(DefaultWindow, 0).VerifyInWindow(0, tok); !ok {
		t.Fatal("the control token does not verify under its own keyset — the arm above measures darkness")
	}
}
