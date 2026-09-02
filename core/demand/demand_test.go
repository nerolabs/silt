package demand

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
// where bytes are checked, before any honest Ack.
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

// keys is the redeemer's per-epoch keyset for this scene: the issuer's key bound to
// epoch 0, with the default window. Every test here withdraws and redeems inside one
// epoch, so epoch 0 / current 0 is the in-window case; the expiry behaviour itself is
// pinned separately in keyset_test.go.
func (s scene) keys() *Keyset {
	ks := NewKeyset(DefaultWindow)
	ks.Put(0, s.issuerPub)
	return ks
}

// token runs a full blind withdrawal: the fetcher blinds a fresh serial, the issuer
// blind-signs it (never seeing the serial), and the fetcher unblinds into a Token.
func (s scene) token(t *testing.T) Token {
	t.Helper()
	serial, err := blindtoken.NewSerial(rand.Reader)
	if err != nil {
		t.Fatalf("serial: %v", err)
	}
	blinded, secret, err := Withdraw(rand.Reader, s.issuerPub, serial)
	if err != nil {
		t.Fatalf("withdraw: %v", err)
	}
	blindSig := SignWithdrawal(s.issuerPriv, blinded)
	return Unblind(s.issuerPub, serial, blindSig, secret)
}

// TestHonestDeliveryCreditsDemand: a real issued token, spent on a fetcher-signed
// delivery-ack, redeems once and credits the object's
// witnessed-demand counter — and only the observable counter, never standing.
func TestHonestDeliveryCreditsDemand(t *testing.T) {
	s := newScene(t, "obj-C")
	tok := s.token(t)
	r := Ack(s.fetcher, tok, s.object, s.server)
	bank := NewBank()
	if ok, _, reason := bank.Redeem(s.keys(), 0, tok, r); !ok {
		t.Fatalf("honest receipt rejected: %s", reason)
	}
	if got := bank.Demand(s.object); got != 1 {
		t.Fatalf("demand = %d, want 1", got)
	}
}

// TestDoubleSpendRejected: a token serial redeems at most once — banking the same
// receipt twice credits demand only once (#receipts ≤ #issued tokens).
func TestDoubleSpendRejected(t *testing.T) {
	s := newScene(t, "obj-C")
	tok := s.token(t)
	r := Ack(s.fetcher, tok, s.object, s.server)
	bank := NewBank()
	if ok, _, _ := bank.Redeem(s.keys(), 0, tok, r); !ok {
		t.Fatal("first redeem should succeed")
	}
	if ok, _, reason := bank.Redeem(s.keys(), 0, tok, r); ok || reason == "" {
		t.Fatalf("second redeem of the same serial must be rejected, got ok=%v", ok)
	}
	if got := bank.Demand(s.object); got != 1 {
		t.Fatalf("double-spend inflated demand to %d, want 1", got)
	}
}

// TestForgedTokenRejected: a serial not signed by the issuer buys nothing, even
// with an otherwise valid signed ack.
func TestForgedTokenRejected(t *testing.T) {
	s := newScene(t, "obj-C")
	// A token blind-signed by an IMPOSTOR issuer key.
	impostor, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("impostor key: %v", err)
	}
	serial, _ := blindtoken.NewSerial(rand.Reader)
	blinded, secret, _ := Withdraw(rand.Reader, &impostor.PublicKey, serial)
	forged := Unblind(&impostor.PublicKey, serial, SignWithdrawal(impostor, blinded), secret)
	r := Ack(s.fetcher, forged, s.object, s.server)
	bank := NewBank()
	if ok, _, _ := bank.Redeem(s.keys(), 0, forged, r); ok {
		t.Fatal("a token not signed by the REAL issuer must be rejected")
	}
	if bank.Demand(s.object) != 0 {
		t.Fatal("forged token credited demand")
	}
}

// TestBlindWithdrawalIsUnlinkable pins P1: the issuer, signing a blinded
// withdrawal, learns nothing that ties it to the serial the token later redeems
// under — the blinded value it saw is independent of the plain serial. (This is the
// cryptographic half; the IP/timing channel is D3's job, still nominal.)
func TestBlindWithdrawalIsUnlinkable(t *testing.T) {
	s := newScene(t, "obj-C")
	serial, _ := blindtoken.NewSerial(rand.Reader)
	blinded, secret, err := Withdraw(rand.Reader, s.issuerPub, serial)
	if err != nil {
		t.Fatalf("withdraw: %v", err)
	}
	// The token redeems under a valid signature the issuer never made on the serial
	// directly (it signed only `blinded`).
	tok := Unblind(s.issuerPub, serial, SignWithdrawal(s.issuerPriv, blinded), secret)
	if !VerifyToken(s.issuerPub, tok) {
		t.Fatal("a blind-withdrawn token must verify under the issuer key")
	}
	// The issuer's signing-time view (`blinded`) must not equal or reveal the serial:
	// a blinding factor r makes blinded = m·rᵉ, uniformly hiding m=FDH(serial).
	if string(blinded) == string(serial) || string(blinded) == string(tok.Serial) {
		t.Fatal("the blinded value the issuer signed leaked the serial — withdrawal is not blind")
	}
	// End to end: the unlinkable token still spends on a correct delivery.
	r := Ack(s.fetcher, tok, s.object, s.server)
	if ok, _, reason := NewBank().Redeem(s.keys(), 0, tok, r); !ok {
		t.Fatalf("blind-withdrawn token failed to redeem on a real delivery: %s", reason)
	}
}

// TestTamperedReceiptRejected: the fetcher signature covers what was delivered and
// to whom-for (serial, object, server, fetcher key) — so mutating any of them is
// caught. A server cannot mint a receipt the fetcher
// did not sign.
func TestTamperedReceiptRejected(t *testing.T) {
	s := newScene(t, "obj-C")
	tok := s.token(t)
	good := Ack(s.fetcher, tok, s.object, s.server)
	bank := NewBank()

	mutations := map[string]func(r *DeliveryReceipt){
		"flip server":      func(r *DeliveryReceipt) { r.Server = ports.HashBytes([]byte("server-B")) },
		"flip object":      func(r *DeliveryReceipt) { r.Object = ports.HashBytes([]byte("obj-D")) },
		"flip fetcher key": func(r *DeliveryReceipt) { pub, _, _ := ed25519.GenerateKey(rand.Reader); r.Fetcher = pub },
		"flip a sig byte":  func(r *DeliveryReceipt) { r.Sig[0] ^= 0xFF },
	}
	for name, mutate := range mutations {
		r := good // struct copy
		r.Sig = append([]byte(nil), good.Sig...)
		r.Fetcher = append([]byte(nil), good.Fetcher...)
		mutate(&r)
		if ok, _, _ := bank.Redeem(s.keys(), 0, tok, r); ok {
			t.Fatalf("%s: a tampered receipt must be rejected", name)
		}
	}
	if bank.Demand(s.object) != 0 {
		t.Fatal("a tampered receipt credited demand")
	}
}

// TestReceiptCarriesNoPossessionClaim pins the CERTIFIED boundary of the neutral
// lane (the 2026-08-26 PoD certification, Q2 / owned residual B3): a receipt is
// mintable with zero object bytes BY DESIGN — the willing fetcher's signature is
// the delivery attestation, and nothing in the receipt proves possession. This is
// sound because the sound properties live elsewhere: the token level
// (#receipts ≤ #tokens, fee paid at withdrawal) and conservation (a colluding
// pair's redeem returns at most its own fee minus the skim — a strict loss). If a
// future change makes this test's premise false (a possession proof returns to
// the receipt), it must be the content-committed recompute floor arriving with
// the strong form or relay — re-read the certification before touching this.
func TestReceiptCarriesNoPossessionClaim(t *testing.T) {
	s := newScene(t, "obj-C")
	tok := s.token(t)
	// The fetcher never saw a single byte of the object; the ack still signs.
	r := Ack(s.fetcher, tok, s.object, s.server)
	bank := NewBank()
	if ok, _, reason := bank.Redeem(s.keys(), 0, tok, r); !ok {
		t.Fatalf("the neutral-lane receipt must redeem without a possession proof (certified): %s", reason)
	}
	// What it bought: one unit of a NEUTRAL observable. Never standing (the
	// firewall test for the conserved credit lives in core/credit).
	if got := bank.Demand(s.object); got != 1 {
		t.Fatalf("demand = %d, want 1", got)
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

// deliver runs one full honest cycle for scene s's fetcher: withdraw a fresh token,
// sign a delivery ack for the object, and redeem it at bank. Returns the
// (credited, reason) the bank reported. Each call spends a DISTINCT token (serial),
// so N calls model N genuine, individually-valid deliveries by the same fetcher.
func (s scene) deliver(t *testing.T, bank *Bank) (bool, string) {
	t.Helper()
	tok := s.token(t)
	r := Ack(s.fetcher, tok, s.object, s.server)
	credited, _, reason := bank.Redeem(s.keys(), 0, tok, r)
	return credited, reason
}

// TestBondedGateRejectsUnbonded (P3b): with the credential required, a perfectly
// valid delivery from a fetcher that is NOT bond-distinct credits ZERO demand — yet
// its one-time token is still consumed (marked spent), so it cannot be retried.
func TestBondedGateRejectsUnbonded(t *testing.T) {
	s := newScene(t, "obj-C")
	bank := NewBank()
	bank.RequireBondedFetcher(bondedSet( /* nobody bonded */ ))

	tok := s.token(t)
	r := Ack(s.fetcher, tok, s.object, s.server)
	if ok, _, reason := bank.Redeem(s.keys(), 0, tok, r); ok {
		t.Fatal("an unbonded fetcher's receipt must not credit demand")
	} else if reason == "" {
		t.Fatal("rejection must carry a reason")
	}
	if bank.Demand(s.object) != 0 {
		t.Fatalf("unbonded delivery credited demand = %d, want 0", bank.Demand(s.object))
	}
	// The token is burned: re-submitting the same receipt is a double-spend, not a
	// second free attempt.
	if ok, _, reason := bank.Redeem(s.keys(), 0, tok, r); ok || reason != "double-spend: serial already redeemed" {
		t.Fatalf("consumed token must be spent; got ok=%v reason=%q", ok, reason)
	}
}

// TestSelfDealOneBondedIdentityCapsDemand is the P3b self-dealing red-team: a washer
// runs ONE bonded fetcher identity and mints N genuine, individually-valid delivery
// receipts (distinct tokens, valid signed acks — indistinguishable from honest demand,
// because a self-fetch IS a real paid delivery; Douceur is unbeaten). With the
// bonded-fetcher credential on, witnessed demand rises by exactly 1, not N: faking U
// units of demand would take U distinct bonded identities, i.e. U real storage bonds.
func TestSelfDealOneBondedIdentityCapsDemand(t *testing.T) {
	s := newScene(t, "obj-C")
	fetcherPub := s.fetcher.Public().(ed25519.PublicKey)
	bank := NewBank()
	bank.RequireBondedFetcher(bondedSet(fetcherPub)) // the washer's single bonded identity

	const N = 6
	for i := 0; i < N; i++ {
		ok, reason := s.deliver(t, bank)
		if i == 0 && !ok {
			t.Fatalf("first bonded delivery must credit: %s", reason)
		}
		if i > 0 && ok {
			t.Fatalf("wash #%d from the same bonded identity must not raise demand again", i)
		}
	}
	if got := bank.Demand(s.object); got != 1 {
		t.Fatalf("one bonded identity washed %d receipts to demand %d, want 1 (cost-to-wash = one bond per unit)", N, got)
	}
}

// TestDistinctBondedFetchersEachCountOnce is the honest control for the cap: M
// genuinely distinct bonded identities each delivering the object move demand to M.
// The gate re-prices fake demand without penalizing real, plural demand.
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
	if got := bank.Demand(s.object); got != int64(M) {
		t.Fatalf("%d distinct bonded fetchers gave demand %d, want %d", M, got, M)
	}
}

// TestBondedGateOffKeepsRawCount pins the default: with no credential required, the
// bank keeps its raw witnessed-delivery count — N deliveries from one fetcher are N.
// (Guards that P3b is strictly opt-in and did not change existing behavior.)
func TestBondedGateOffKeepsRawCount(t *testing.T) {
	s := newScene(t, "obj-C")
	bank := NewBank() // gate off
	const N = 3
	for i := 0; i < N; i++ {
		if ok, reason := s.deliver(t, bank); !ok {
			t.Fatalf("delivery %d rejected with gate off: %s", i, reason)
		}
	}
	if got := bank.Demand(s.object); got != int64(N) {
		t.Fatalf("gate-off demand = %d, want %d (raw count)", got, N)
	}
}
