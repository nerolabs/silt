package demand

// The demand primitive's unit gates, ON THE ANCHORED SESSION LANE.
//
// C1 (2026-09-08) retired the v2 flat path — Bank.Redeem, DeliveryReceipt, Ack,
// SubmittedReceipt and the bank's own spent set — so every property these tests pin is
// asserted through the surface that survived: the anchor (Withdraw → Unblind →
// VerifyInWindow), the session open (SignSessionOpen / SessionOpenCommitment), the
// settlement acknowledgement (AckSession / VerifySig) and the observable (Witness /
// WitnessedIncrements / DistinctBondedFetchers). The re-homing ledger is in
// docs/thinking/2026-09-07-b9-flat-path-retirement.md.

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/rsa"
	"testing"

	"github.com/nerolabs/silt/core/blindtoken"
	"github.com/nerolabs/silt/ports"
)

// scene sets up a (blind-signing RSA) issuer, a fetcher, a server, and one
// object C. The receipt carries no bytes and no PoR (the certified neutral-lane
// shape), so the scene needs no object data — the fetch path's content-verify is
// where bytes are checked, before any honest acknowledgement.
type scene struct {
	issuerPub  *rsa.PublicKey
	issuerPriv *rsa.PrivateKey
	fetcher    ed25519.PrivateKey
	server     ports.NodeID
	object     ports.Hash
}

func newScene(t *testing.T, objectLabel string) scene {
	t.Helper()
	ipriv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("issuer key: %v", err)
	}
	_, fpriv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("fetcher key: %v", err)
	}
	return scene{
		issuerPub: &ipriv.PublicKey, issuerPriv: ipriv, fetcher: fpriv,
		server: ports.HashBytes([]byte("server-A")), object: ports.HashBytes([]byte(objectLabel)),
	}
}

// keys is the server's per-epoch issuer keyset for this scene: the issuer's key bound
// to epoch 0, with the default window. Every test here withdraws and opens inside one
// epoch, so epoch 0 / current 0 is the in-window case; the expiry behaviour itself is
// pinned separately in keyset_test.go.
func (s scene) keys() *Keyset {
	ks := NewKeyset(DefaultWindow)
	ks.Put(0, s.issuerPub)
	return ks
}

// token runs a full blind withdrawal: the fetcher blinds a fresh serial, the issuer
// blind-signs it (never seeing the serial), and the fetcher unblinds into a Token —
// the anchor it will present at a session open.
func (s scene) token(t *testing.T) Token {
	t.Helper()
	serial, err := blindtoken.NewSerial(rand.Reader)
	if err != nil {
		t.Fatalf("serial: %v", err)
	}
	blinded, secret, err := Withdraw(rand.Reader, s.issuerPub, testChainID, 0, serial)
	if err != nil {
		t.Fatalf("withdraw: %v", err)
	}
	blindSig := SignWithdrawal(rand.Reader, s.issuerPriv, testChainID, blinded)
	tok, uerr := Unblind(s.issuerPub, testChainID, 0, serial, blindSig, secret)
	if uerr != nil {
		t.Fatalf("unblind: %v", uerr)
	}
	return tok
}

// openSession is one honest open at s.server: the fetcher signs its anchors, the
// server verifies the anchor under its own in-window keyset and the signature over the
// commitment, and returns the (handle, M) the receipts will name. It is the demand-tier
// shape of core/node's OpenDeliverySession, without the node's session table.
func (s scene) openSession(t *testing.T, anchors ...Token) (uint64, []byte) {
	t.Helper()
	o := SignSessionOpen(s.fetcher, s.server, anchors)
	m := SessionOpenCommitment(s.server, anchors)
	if !ed25519.Verify(ed25519.PublicKey(o.Fetcher), m, o.Sig) {
		t.Fatal("setup: the open signature does not verify over its own commitment")
	}
	ks := s.keys()
	for i, a := range anchors {
		if _, ok := ks.VerifyInWindow(testChainID, 0, a); !ok {
			t.Fatalf("setup: anchor %d does not verify in window", i)
		}
	}
	return 1, m
}

// TestHonestSessionDeliveryCreditsWitnessedDemand (was TestHonestDeliveryCreditsDemand):
// a real issued token, presented as a session anchor and acknowledged by a
// fetcher-signed cumulative receipt, credits the object's witnessed-demand observable —
// and only the observable, never standing.
func TestHonestSessionDeliveryCreditsWitnessedDemand(t *testing.T) {
	s := newScene(t, "obj-C")
	handle, m := s.openSession(t, s.token(t))
	r := AckSession(s.fetcher, handle, m, s.object, s.server, 1)
	if !r.VerifySig() {
		t.Fatal("an honest acknowledgement must verify")
	}
	bank := NewBank()
	if ok, reason := bank.Witness(s.object, r.Fetcher, 1); !ok {
		t.Fatalf("honest settlement rejected: %s", reason)
	}
	if got := bank.WitnessedIncrements(s.object); got != 1 {
		t.Fatalf("witnessed increments = %d, want 1", got)
	}
}

// TestForgedAnchorIsRefusedAtTheOpen (was TestForgedTokenRejected): a serial not
// blind-signed by the issuer the server resolved buys nothing. On the flat lane the
// refusal was inside Redeem; on the session lane the anchor is verified at OPEN, under
// the server's own committed per-epoch key, before any guard entry or budget exists.
func TestForgedAnchorIsRefusedAtTheOpen(t *testing.T) {
	s := newScene(t, "obj-C")
	// A token blind-signed by an IMPOSTOR issuer key.
	impostor, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("impostor key: %v", err)
	}
	serial, _ := blindtoken.NewSerial(rand.Reader)
	blinded, secret, _ := Withdraw(rand.Reader, &impostor.PublicKey, testChainID, 0, serial)
	forged, ferr := Unblind(&impostor.PublicKey, testChainID, 0, serial, SignWithdrawal(rand.Reader, impostor, testChainID, blinded), secret)
	if ferr != nil {
		t.Fatalf("an impostor-signed token must still UNBLIND (it is valid under the impostor's own key); got %v", ferr)
	}
	if _, ok := s.keys().VerifyInWindow(testChainID, 0, forged); ok {
		t.Fatal("an anchor not signed by the REAL issuer verified in the server's window — the open would spend it into the guard")
	}
	// And the fetcher's own signature over the open does not rescue it: the anchor is
	// checked independently of who asked.
	o := SignSessionOpen(s.fetcher, s.server, []Token{forged})
	if !ed25519.Verify(ed25519.PublicKey(o.Fetcher), SessionOpenCommitment(s.server, []Token{forged}), o.Sig) {
		t.Fatal("setup: the open signature should be well formed — the refusal must come from the ANCHOR, not the request")
	}
}

// TestBlindWithdrawalIsUnlinkable pins P1: the issuer, signing a blinded
// withdrawal, learns nothing that ties it to the serial the token later anchors a
// session under — the blinded value it saw is independent of the plain serial. (This
// is the cryptographic half; the IP/timing channel is D3's job, still nominal.)
func TestBlindWithdrawalIsUnlinkable(t *testing.T) {
	s := newScene(t, "obj-C")
	serial, _ := blindtoken.NewSerial(rand.Reader)
	blinded, secret, err := Withdraw(rand.Reader, s.issuerPub, testChainID, 0, serial)
	if err != nil {
		t.Fatalf("withdraw: %v", err)
	}
	// The token verifies under a valid signature the issuer never made on the serial
	// directly (it signed only `blinded`).
	tok, uerr := Unblind(s.issuerPub, testChainID, 0, serial, SignWithdrawal(rand.Reader, s.issuerPriv, testChainID, blinded), secret)
	if uerr != nil {
		t.Fatalf("unblind: %v", uerr)
	}
	if !VerifyToken(s.issuerPub, testChainID, 0, tok) {
		t.Fatal("a blind-withdrawn token must verify under the issuer key")
	}
	// The issuer's signing-time view (`blinded`) must not equal or reveal the serial:
	// a blinding factor r makes blinded = m·rᵉ, uniformly hiding m=FDH(serial).
	if string(blinded) == string(serial) || string(blinded) == string(tok.Serial) {
		t.Fatal("the blinded value the issuer signed leaked the serial — withdrawal is not blind")
	}
	// End to end: the unlinkable token still anchors a real session.
	handle, m := s.openSession(t, tok)
	if !AckSession(s.fetcher, handle, m, s.object, s.server, 1).VerifySig() {
		t.Fatal("a blind-withdrawn token failed to carry a real session settlement")
	}
}

// TestAcknowledgementCarriesNoPossessionClaim (was TestReceiptCarriesNoPossessionClaim)
// pins the CERTIFIED boundary of the neutral lane (the 2026-08-26 PoD certification,
// Q2 / owned residual B3): an acknowledgement is mintable with zero object bytes BY
// DESIGN — the willing fetcher's signature is the delivery attestation, and nothing in
// it proves possession. This is sound because the sound properties live elsewhere: the
// anchor level (one token, one session budget, spent at open into the ledger's guard)
// and conservation (a colluding pair settles at most the face it burned — a strict
// loss). If a future change makes this test's premise false (a possession proof returns
// to the receipt), it must be the content-committed recompute floor arriving with the
// strong form or relay — re-read the certification before touching this.
func TestAcknowledgementCarriesNoPossessionClaim(t *testing.T) {
	s := newScene(t, "obj-C")
	handle, m := s.openSession(t, s.token(t))
	// The fetcher never saw a single byte of the object; the acknowledgement still signs.
	r := AckSession(s.fetcher, handle, m, s.object, s.server, 1)
	if !r.VerifySig() {
		t.Fatal("the neutral-lane acknowledgement must verify without a possession proof (certified)")
	}
	// What it bought: one unit of a NEUTRAL observable. Never standing (the
	// firewall test for the conserved credit lives in core/credit).
	bank := NewBank()
	if ok, why := bank.Witness(s.object, r.Fetcher, 1); !ok {
		t.Fatalf("witness refused a certified receipt: %s", why)
	}
	if got := bank.WitnessedIncrements(s.object); got != 1 {
		t.Fatalf("witnessed increments = %d, want 1", got)
	}
}

// bondedSet models a committed bond ledger for the P3b gate: the fetcher keys in it
// are bond-distinct (each maps to its own slot = hash(key)); everything else is
// unbonded. In production this closure is backed by chain.BondedSize (the node's
// RequireBondedFetchers); here it lets the demand red-team isolate the counting rule
// from the consensus scaffolding.
func bondedSet(keys ...ed25519.PublicKey) BondCheck {
	set := map[string]bool{}
	for _, k := range keys {
		set[string(k)] = true
	}
	return func(pub []byte) (string, bool) {
		if !set[string(pub)] {
			return "", false
		}
		id := ports.HashBytes(pub)
		return string(id[:]), true
	}
}

// deliver runs one full honest cycle for scene s's fetcher: withdraw a fresh anchor,
// open a session on it, sign a settlement acknowledgement, and witness it at bank.
// Returns the (credited, reason) the bank reported. Each call spends a DISTINCT anchor
// (serial), so N calls model N genuine, individually-funded sessions by the same
// fetcher.
func (s scene) deliver(t *testing.T, bank *Bank) (bool, string) {
	t.Helper()
	handle, m := s.openSession(t, s.token(t))
	r := AckSession(s.fetcher, handle, m, s.object, s.server, 1)
	credited, reason := bank.Witness(s.object, r.Fetcher, 1)
	return credited, reason
}

// TestBondedGateRejectsUnbonded (P3b, was the same name): with the credential
// required, a perfectly valid settlement from a fetcher that is NOT bond-distinct
// contributes to NEITHER observable.
//
// The v2 twin also asserted "and its one-time token is still consumed". That clause
// does not re-home to a refusal: on the session lane the anchor is burned at OPEN,
// before any settlement is offered, so a refused settlement can never return it. That
// ordering is pinned at core/node TestOpenIsAllOrNothingAndDurableBeforeAdmission and
// at core/credit TestDeliveryFundTopUpSpendsFreshAnchorsOnce (the ReasonAlreadyPaid
// arm).
func TestBondedGateRejectsUnbonded(t *testing.T) {
	s := newScene(t, "obj-C")
	bank := NewBank()
	bank.RequireBondedFetcher(bondedSet( /* nobody bonded */ ))

	ok, reason := s.deliver(t, bank)
	if ok {
		t.Fatal("an unbonded fetcher's settlement must not credit the observable")
	}
	if reason == "" {
		t.Fatal("rejection must carry a reason")
	}
	if bank.WitnessedIncrements(s.object) != 0 || bank.DistinctBondedFetchers(s.object) != 0 {
		t.Fatalf("unbonded settlement moved a surface: increments %d, distinct %d, want 0 / 0",
			bank.WitnessedIncrements(s.object), bank.DistinctBondedFetchers(s.object))
	}
}

// TestSelfDealOneBondedIdentityIsOneDistinctFetcher is the P3b self-dealing red-team
// (was TestSelfDealOneBondedIdentityCapsDemand): a washer runs ONE bonded fetcher
// identity and settles N genuine, individually-funded sessions — indistinguishable
// from honest demand, because a self-fetch IS a real paid delivery; Douceur is
// unbeaten. The DISTINCT-BONDED-FETCHER surface rises by exactly 1, not N: faking U
// units of that surface takes U distinct bonded identities, i.e. U real storage bonds.
//
// What CHANGED from the v2 twin, by certification and not by accident: P3b keeps its
// ADMISSION role and loses its dedup role on the INCREMENT counter (R2.9 cert §3.3
// rule 4 — two surfaces, never one field with a flag-dependent unit). So the increment
// counter here is N, and the cost-to-wash claim is carried by DistinctBondedFetchers.
func TestSelfDealOneBondedIdentityIsOneDistinctFetcher(t *testing.T) {
	s := newScene(t, "obj-C")
	fetcherPub := s.fetcher.Public().(ed25519.PublicKey)
	bank := NewBank()
	bank.RequireBondedFetcher(bondedSet(fetcherPub)) // the washer's single bonded identity

	const N = 6
	for i := 0; i < N; i++ {
		if ok, reason := s.deliver(t, bank); !ok {
			t.Fatalf("bonded settlement %d must credit: %s", i, reason)
		}
	}
	if got := bank.DistinctBondedFetchers(s.object); got != 1 {
		t.Fatalf("one bonded identity washed %d sessions to %d distinct fetchers, want 1 (cost-to-wash = one bond per unit)", N, got)
	}
	if got := bank.WitnessedIncrements(s.object); got != N {
		t.Fatalf("increments = %d, want %d — the increment counter is denominated in SETTLED increments, not in distinct identities", got, N)
	}
}

// TestDistinctBondedFetchersEachCountOnce is the honest control for the cap: M
// genuinely distinct bonded identities each settling on the object move the distinct
// surface to M. The gate re-prices fake demand without penalizing real, plural demand.
func TestDistinctBondedFetchersEachCountOnce(t *testing.T) {
	s := newScene(t, "obj-C")
	const M = 4
	fetchers := make([]ed25519.PrivateKey, M)
	pubs := make([]ed25519.PublicKey, M)
	for i := range fetchers {
		pub, priv, _ := ed25519.GenerateKey(rand.Reader)
		fetchers[i], pubs[i] = priv, pub
	}
	bank := NewBank()
	bank.RequireBondedFetcher(bondedSet(pubs...))

	for i, f := range fetchers {
		fs := s
		fs.fetcher = f // same object/server/issuer, a distinct bonded fetcher identity
		if ok, reason := fs.deliver(t, bank); !ok {
			t.Fatalf("distinct bonded fetcher %d rejected: %s", i, reason)
		}
	}
	if got := bank.DistinctBondedFetchers(s.object); got != int64(M) {
		t.Fatalf("%d distinct bonded fetchers gave %d, want %d", M, got, M)
	}
}

// TestBondedGateOffKeepsRawCount pins the default: with no credential required, the
// bank keeps its raw witnessed count — N settlements from one fetcher are N — and the
// distinct surface stays empty. (Guards that P3b is strictly opt-in.)
func TestBondedGateOffKeepsRawCount(t *testing.T) {
	s := newScene(t, "obj-C")
	bank := NewBank() // gate off
	const N = 3
	for i := 0; i < N; i++ {
		if ok, reason := s.deliver(t, bank); !ok {
			t.Fatalf("settlement %d rejected with gate off: %s", i, reason)
		}
	}
	if got := bank.WitnessedIncrements(s.object); got != int64(N) {
		t.Fatalf("gate-off increments = %d, want %d (raw count)", got, N)
	}
	if got := bank.DistinctBondedFetchers(s.object); got != 0 {
		t.Fatalf("gate-off distinct fetchers = %d, want 0 (the surface is P3b's)", got)
	}
}
