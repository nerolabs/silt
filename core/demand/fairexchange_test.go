package demand

// The optimistic fair-exchange floor (P2), ON THE ANCHORED SESSION LANE.
//
// C1 (2026-09-08) retired Bank.Redeem / DeliveryReceipt / Ack, so each property below
// is asserted through the surface that survived: a serial is spent at session OPEN (the
// credit ledger's shared paid-serial guard), and a delivery is acknowledged by a
// session-domain SessionReceipt.

import (
	"testing"

	"github.com/nerolabs/silt/ports"
)

// TestAbortLeavesAnchorReusable (was TestAbortLeavesTokenReusable) is the fetcher-side
// floor: an aborted exchange never consumes the fetcher's token. The fetcher commits
// and the server then vanishes — delivering nothing, opening nothing — so the anchor
// was never spent into any guard, and the SAME token opens a real session at another
// server.
//
// What moved with the lane: on the flat path the serial was spent at REDEEM, so "the
// abort did not burn it" was a statement about the bank. Under R2.9 the anchor is spent
// at OPEN, which makes the statement stronger and simpler — a server that never opened a
// session cannot have spent anything. The ledger half (the same anchor spends exactly
// once, and only where a session was opened) is core/credit
// TestDeliveryFundTopUpSpendsFreshAnchorsOnce.
func TestAbortLeavesAnchorReusable(t *testing.T) {
	s := newScene(t, "obj-C")
	tok := s.token(t)

	// Optimistic phase begins: the fetcher signs a pre-release commitment to serverA…
	serverA := ports.HashBytes([]byte("server-A-aborts"))
	c := Commit(s.fetcher, tok, s.object, serverA)
	if !VerifyCommitment(c) {
		t.Fatal("a well-formed pre-release commitment must verify")
	}
	// …then serverA aborts: no bytes delivered, no session opened. Only an OPEN spends
	// an anchor, and the commitment is not one.

	// The fetcher retries at serverB (s.server) and opens a genuine session on the same
	// anchor — proving the abort did not burn it.
	handle, m := s.openSession(t, tok)
	if !AckSession(s.fetcher, handle, m, s.object, s.server, 1).VerifySig() {
		t.Fatal("an aborted exchange must leave the anchor reusable, but the retry session did not carry a settlement")
	}
}

// TestPreReleaseCommitmentIsNotAReceipt is the server-side floor: a fetcher's
// pre-release commitment cannot be turned into a settlement. A malicious server holding
// a valid ExchangeCommitment (the fetcher engaged) cannot lift its signature onto a
// SessionReceipt — the two sit in different domains, so the receipt fails VerifySig and
// the ledger is never asked to settle.
func TestPreReleaseCommitmentIsNotAReceipt(t *testing.T) {
	s := newScene(t, "obj-C")
	tok := s.token(t)
	handle, m := s.openSession(t, tok)
	c := Commit(s.fetcher, tok, s.object, s.server)
	if !VerifyCommitment(c) {
		t.Fatal("setup: commitment should verify")
	}

	// The server copies the commitment's fields and its signature into a receipt on the
	// session it really did open — but the sig sits in the commitment domain.
	forged := SessionReceipt{
		Handle:     handle,
		Commitment: append([]byte(nil), m...),
		Object:     c.Object,
		Server:     c.Server,
		Fetcher:    append([]byte(nil), c.Fetcher...),
		Count:      1,
		Sig:        append([]byte(nil), c.Sig...), // a commitment sig, over the wrong domain
	}
	if forged.VerifySig() {
		t.Fatal("a pre-release commitment verified as a session settlement — the domains are not separated")
	}
	// The control: the honest receipt over the same tuple DOES verify, so the refusal is
	// the domain and not a malformed fixture.
	if !AckSession(s.fetcher, handle, m, c.Object, c.Server, 1).VerifySig() {
		t.Fatal("the control receipt does not verify — the arm above measures darkness")
	}
}

// TestCommitmentDomainSeparation pins the crypto behind the server-side floor in the
// other direction: a signature good as a session receipt is NOT good as a commitment.
func TestCommitmentDomainSeparation(t *testing.T) {
	s := newScene(t, "obj-C")
	tok := s.token(t)
	handle, m := s.openSession(t, tok)
	good := AckSession(s.fetcher, handle, m, s.object, s.server, 1)
	asCommit := ExchangeCommitment{Serial: tok.Serial, Object: good.Object, Server: good.Server,
		Fetcher: good.Fetcher, Sig: good.Sig}
	if VerifyCommitment(asCommit) {
		t.Fatal("a session-receipt signature must not verify as an exchange commitment (domains not separated)")
	}
	// A tampered-key commitment must fail.
	c := Commit(s.fetcher, tok, s.object, s.server)
	c.Fetcher[0] ^= 0xFF
	if VerifyCommitment(c) {
		t.Fatal("a commitment whose fetcher key was altered must not verify")
	}
}

// TestOptimisticPathStillCredits: committing first does not disturb the happy path —
// after a pre-release commitment, a genuine session settlement still credits the
// witnessed-demand observable exactly once.
func TestOptimisticPathStillCredits(t *testing.T) {
	s := newScene(t, "obj-C")
	tok := s.token(t)
	_ = Commit(s.fetcher, tok, s.object, s.server) // optimistic phase
	handle, m := s.openSession(t, tok)
	r := AckSession(s.fetcher, handle, m, s.object, s.server, 1)
	if !r.VerifySig() {
		t.Fatal("optimistic completion should produce a verifying receipt")
	}
	bank := NewBank()
	if ok, reason := bank.Witness(s.object, r.Fetcher, 1); !ok {
		t.Fatalf("optimistic completion should credit: %s", reason)
	}
	if got := bank.WitnessedIncrements(s.object); got != 1 {
		t.Fatalf("witnessed increments = %d, want 1", got)
	}
}
