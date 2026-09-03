package node

// C1 — the remote nil-receiver panic in the demand-token handler.
//
// MECHANISM. `answerDemandTokenRequest` gates on the DEMAND issuer for the current
// epoch (`n.demandIssuers[cur]`), not on the PUBLISH issuer. When the request carries
// a prepaid publish credit, the handler routes it to `tokenChargeFor`, which verified
// the credit against `n.tokenIssuer.Public()`. `(*blindtoken.Issuer).Public` reads
// `i.key` unconditionally, so a nil `n.tokenIssuer` was a nil dereference: one crafted
// `MsgDemandTokenRequest` from any peer crashed a node that runs the demand lane
// without a publish issuer. The shipped daemon happens to enable both, in that order;
// the sim fixtures and any future config are not bound to.
//
// FIX. `tokenChargeFor` returns a typed refusal (`errNoTokenIssuer`) instead of
// dereferencing. A credit cannot be honoured when there is no key to verify it
// against, so refusing is the semantics the neighbouring "credit does not verify"
// branch already had. The credit-free path is untouched: a demand issuer with no
// publish issuer still serves ordinary withdrawals.

import (
	"crypto/rand"
	"crypto/rsa"
	"errors"
	"testing"

	"github.com/nerolabs/silt/adapters/identity"
	"github.com/nerolabs/silt/core/blindtoken"
	"github.com/nerolabs/silt/core/demand"
	"github.com/nerolabs/silt/ports"
)

// TestDemandTokenRequestWithCreditAndNoPublishIssuerRefuses drives the crafted message
// over the handler AND over the wire dispatch. Before the fix both panic.
func TestDemandTokenRequestWithCreditAndNoPublishIssuerRefuses(t *testing.T) {
	f := newIssuerKeyFixture(t, 4001)
	nd := f.nd
	// The node runs the DEMAND lane for epoch 0 but never calls EnableTokenIssuer,
	// so it has no publish issuer. This is the reachable config the guard covers.
	nd.SetDemandIssuerKey(rand.Reader, 0, f.committed)
	if nd.tokenIssuer != nil {
		t.Fatal("setup: the fixture must have no publish issuer")
	}
	if nd.demandIssuers[0] == nil {
		t.Fatal("setup: the demand issuer for epoch 0 must be installed")
	}

	serial, err := blindtoken.NewSerial(rand.Reader)
	if err != nil {
		t.Fatalf("serial: %v", err)
	}
	blinded, _, err := demand.Withdraw(rand.Reader, &f.committed.PublicKey, 0, serial)
	if err != nil {
		t.Fatalf("withdraw: %v", err)
	}
	attacker := identity.FromSeed(4002).NodeID()
	msg := ports.Message{
		Kind: ports.MsgDemandTokenRequest,
		Data: blinded,
		// A credit that verifies against nothing. Its content is irrelevant: the
		// crash was on the VERIFY, before any check of the bytes.
		Credit: &ports.PublishCredit{Serial: []byte("crafted-serial"), Sig: []byte("crafted-sig")},
	}

	reply := nd.answerDemandTokenRequest(attacker, msg) // no panic
	if reply.OK {
		t.Fatal("a credit-bearing request must be REFUSED when there is no publish issuer to verify it against")
	}
	if len(reply.Data) != 0 {
		t.Fatalf("refusal must carry no blind signature, got %d bytes", len(reply.Data))
	}

	// The same message over the real dispatch path, which is how a peer delivers it.
	nd.handle(attacker, msg) // no panic

	// Liveness is not collateral damage: the CREDIT-FREE withdrawal still signs.
	free := nd.answerDemandTokenRequest(attacker, ports.Message{Kind: ports.MsgDemandTokenRequest, Data: blinded})
	if !free.OK || len(free.Data) == 0 {
		t.Fatal("a credit-free demand withdrawal must still be served by a node with no publish issuer")
	}
}

// TestTokenChargeForRefusalsAreTyped pins the refusal REASONS at the unit boundary, so
// "no issuer" is never silently folded into "bad credit" (or, worse, into a charge of
// the requester's durable identity).
func TestTokenChargeForRefusalsAreTyped(t *testing.T) {
	f := newIssuerKeyFixture(t, 4003)
	nd := f.nd
	from := identity.FromSeed(4004).NodeID()

	if _, err := nd.tokenChargeFor(from, &ports.PublishCredit{Serial: []byte("s"), Sig: []byte("g")}); !errors.Is(err, errNoTokenIssuer) {
		t.Fatalf("no publish issuer + credit: want errNoTokenIssuer, got %v", err)
	}
	charge, err := nd.tokenChargeFor(from, nil)
	if err != nil || charge == nil {
		t.Fatalf("credit-free must not be refused: charge=%v err=%v", charge != nil, err)
	}

	// With an issuer installed, a garbage credit is refused as a CREDIT problem.
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("issuer key: %v", err)
	}
	nd.EnableTokenIssuer(rand.Reader, key)
	if _, err := nd.tokenChargeFor(from, &ports.PublishCredit{Serial: []byte("s"), Sig: []byte("g")}); !errors.Is(err, errCreditRefused) {
		t.Fatalf("bad credit: want errCreditRefused, got %v", err)
	}
}
