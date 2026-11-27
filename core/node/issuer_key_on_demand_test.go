package node

import (
	"crypto/rand"
	"crypto/rsa"
	"testing"
)

// TestAnUnknownTokenSignerTriggersAKeyFetch. A validator verifying a token signed by an
// issuer whose key it never fetched ignored the signature, and kept ignoring it: the
// takedown on a matured field network was refused "0 of 2" round after round. The chain's
// key lookup now fetches an unknown signer's key, once per backoff, so the next round can
// verify it.
func TestAnUnknownTokenSignerTriggersAKeyFetch(t *testing.T) {
	r := newNetgetRig(t)
	issuer, verifier := r.nodes[0], r.nodes[1]
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	issuer.EnableTokenIssuer(rand.Reader, key)

	if verifier.IssuerKeyOf(issuer.ID()) != nil {
		t.Fatal("CONTROL: the verifier already knew the issuer's key, so nothing here is unknown")
	}
	if verifier.IssuerKeyOrFetch(issuer.ID()) != nil {
		t.Fatal("the lookup returned a key it could not have had yet")
	}
	if verifier.IssuerKeyOrFetch(issuer.ID()); len(verifier.issuerKeyAsked) != 1 {
		t.Fatalf("a second lookup inside the backoff must not ask again: %d outstanding", len(verifier.issuerKeyAsked))
	}
	r.sched.Run()
	pub := verifier.IssuerKeyOrFetch(issuer.ID())
	if pub == nil || pub.N.Cmp(key.PublicKey.N) != 0 {
		t.Fatal("after the fetch completed the verifier still did not hold the issuer's key")
	}
	if len(verifier.issuerKeyAsked) != 0 {
		t.Fatal("a fetched key left its backoff entry behind")
	}
}
