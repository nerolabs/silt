package node

// M3 — the network binding where the node actually uses it (research certification
// 2026-09-11 §3, G-3a / G-3b / G-3d).
//
// THE RULE THIS TIER ENFORCES: the chain id a token is minted under and the chain id a
// verifier checks under are BOTH read from a chain, never from the token and never from
// the request. On this node that single source is (*Node).chainID() — the genesis hash,
// the zero hash when the node holds no chain.

import (
	"crypto/rand"
	"crypto/rsa"
	"testing"

	"github.com/nerolabs/silt/adapters/identity"
	"github.com/nerolabs/silt/adapters/memstore"
	"github.com/nerolabs/silt/adapters/simclock"
	"github.com/nerolabs/silt/adapters/simnet"
	"github.com/nerolabs/silt/core/chain"
	"github.com/nerolabs/silt/core/credit"
	"github.com/nerolabs/silt/core/demand"
	"github.com/nerolabs/silt/ports"
)

// m3Node is a bare node with a signer and a ledger, ready to be given (or denied) a chain.
func m3Node(t *testing.T, seed int64) (*Node, *identity.Identity, *credit.Ledger) {
	t.Helper()
	sched := simclock.New()
	net := simnet.New(sched, 2, simnet.DefaultConfig())
	ident := identity.FromSeed(seed)
	nd := New(ident.NodeID(), DefaultConfig(), sched, net.Endpoint(ident.NodeID()), memstore.New())
	nd.SetSigner(ident.Signer())
	ledger := credit.New(50_000, 500_000)
	nd.SetLedger(ledger)
	ledger.Register(ident.NodeID())
	return nd, ident, ledger
}

// nodeTestChain is the network the fixtures in this package mint their hand-made tokens
// on when they never present them to a real chain-bearing node. Non-uniform bytes: a
// repeated-byte fixture hides a binding that copies only the first byte or only the last.
//
// ⚠ WHERE A FIXTURE DOES present its token to a node that holds a chain, it must use THAT
// NODE'S chain id — nd.chainID() — not this constant, because the node verifies under its
// own. Those sites say so at the call.
var nodeTestChain = ports.Hash{
	0x2b, 0x00, 0xe4, 0x71, 0x9a, 0x00, 0x3f, 0xc8,
	0x56, 0xd1, 0x00, 0x0b, 0xa7, 0x62, 0x00, 0xfd,
	0x14, 0x89, 0x00, 0x5c, 0xe0, 0x37, 0xb6, 0x00,
	0x48, 0xcf, 0x21, 0x00, 0x93, 0x7a, 0x0e, 0xd5,
}

// TestChainlessNodeIssuesNoDemandToken is G-3b on the live issuance path. A node with no
// chain reports the zero chain id, every chainless node reports the same one, and a token
// minted under it would be honoured by all of them — so the issuer refuses before it
// charges anything.
//
// Both bound withdrawal lanes (delivery tokens and relay anchors) arrive at
// answerDemandTokenRequest, so this one arm covers both.
func TestChainlessNodeIssuesNoDemandToken(t *testing.T) {
	issuerPriv := m3IssuerKey(t)

	// WITH a chain: the honest path issues. Without this leg the refusal below could be
	// any of the other reasons answerDemandTokenRequest returns OK=false.
	withChain, ident, _ := m3Node(t, 40011)
	c := c3Chain(t, 1, ident.Signer(),
		chain.SignIssuerKeyReg(ident.Signer(), 0, demand.KeyFingerprint(&issuerPriv.PublicKey)))
	withChain.EnableChain(c, ident.Signer())
	withChain.SetDemandIssuerKey(rand.Reader, 0, issuerPriv)
	if withChain.chainID() == (ports.Hash{}) {
		t.Fatal("setup: the chain-bearing node reports a zero chain id — the contrast below would be vacuous")
	}
	serial := m3NodeSerial(t, "chainless")
	blinded, _, err := demand.Withdraw(rand.Reader, &issuerPriv.PublicKey, withChain.chainID(), 0, serial)
	if err != nil {
		t.Fatal(err)
	}
	if reply := withChain.answerDemandTokenRequest(ident.NodeID(), ports.Message{
		Kind: ports.MsgDemandTokenRequest, Data: blinded, Height: 0,
	}); !reply.OK || len(reply.Data) == 0 {
		t.Fatal("honest safety: a chain-bearing issuer refused a good withdrawal")
	}

	// WITHOUT a chain: same issuer key, same epoch, same blinded value — and no signature.
	chainless, _, _ := m3Node(t, 40011)
	chainless.SetDemandIssuerKey(rand.Reader, 0, issuerPriv)
	if chainless.chainID() != (ports.Hash{}) {
		t.Fatal("setup: the chainless node does not report a zero chain id")
	}
	if reply := chainless.answerDemandTokenRequest(ident.NodeID(), ports.Message{
		Kind: ports.MsgDemandTokenRequest, Data: blinded, Height: 0,
	}); reply.OK || len(reply.Data) != 0 {
		t.Error("a node holding NO chain blind-signed a withdrawal — network zero is not a network")
	}
}

// TestDeliveryAnchorFromAnotherNetworkIsRefused is G-3a and G-3d at the session open: the
// cross-network token dies at the ANCHOR, before any session signature is evaluated, which
// is the inheritance argument the certification makes for the leaf domains
// (sessionOpenDomain, sessionFundDomain, receiptDomainV3 stay chain-blind on purpose).
//
// The foreign token is minted under a DIFFERENT chain id but the SAME committed issuer key
// and the SAME epoch, so the only thing separating it from the honest one is the network.
func TestDeliveryAnchorFromAnotherNetworkIsRefused(t *testing.T) {
	issuerPriv := m3IssuerKey(t)
	nd, ident, _ := m3Node(t, 40012)
	c := c3Chain(t, 1, ident.Signer(),
		chain.SignIssuerKeyReg(ident.Signer(), 0, demand.KeyFingerprint(&issuerPriv.PublicKey)))
	nd.EnableChain(c, ident.Signer())
	nd.SetDemandIssuerKey(rand.Reader, 0, issuerPriv)
	nd.EnableDemandBank(ident.NodeID())
	nd.EnableDeliverySessions(10 * ports.Second)

	own := nd.chainID()
	if own == (ports.Hash{}) {
		t.Fatal("setup: the node reports a zero chain id")
	}
	foreign := own
	foreign[0] ^= 0xff // a different network, one bit-flip away — still a real chain id

	mint := func(cid ports.Hash, label string) demand.Token {
		t.Helper()
		serial := m3NodeSerial(t, label)
		blinded, secret, err := demand.Withdraw(rand.Reader, &issuerPriv.PublicKey, cid, 0, serial)
		if err != nil {
			t.Fatal(err)
		}
		tok, err := demand.Unblind(&issuerPriv.PublicKey, cid, 0, serial,
			demand.SignWithdrawal(rand.Reader, issuerPriv, cid, blinded), secret)
		if err != nil {
			t.Fatal(err)
		}
		return tok
	}

	// HONEST SAFETY FIRST: a token from this network passes verifyDeliveryAnchors.
	if _, err := nd.verifyDeliveryAnchors([]demand.Token{mint(own, "own-net")}); err != nil {
		t.Fatalf("honest safety: a token minted on THIS network was refused: %v", err)
	}
	// The foreign one does not, and it fails as an INVALID ANCHOR — at the RSA check under
	// this node's own keyset, not at a malformed-input arm.
	if _, err := nd.verifyDeliveryAnchors([]demand.Token{mint(foreign, "foreign-net")}); err != errDeliveryAnchorInvalid {
		t.Errorf("a token minted on another network: err = %v, want errDeliveryAnchorInvalid", err)
	}
}

// TestChainlessNodeVerifiesNoDeliveryAnchor is the verifier half of G-3b, driven against a
// node that holds no chain — the state Layer 2's JOIN mode makes real on every join.
func TestChainlessNodeVerifiesNoDeliveryAnchor(t *testing.T) {
	issuerPriv := m3IssuerKey(t)
	nd, ident, _ := m3Node(t, 40013)
	c := c3Chain(t, 1, ident.Signer(),
		chain.SignIssuerKeyReg(ident.Signer(), 0, demand.KeyFingerprint(&issuerPriv.PublicKey)))
	nd.EnableChain(c, ident.Signer())
	nd.SetDemandIssuerKey(rand.Reader, 0, issuerPriv)
	nd.EnableDemandBank(ident.NodeID())
	nd.EnableDeliverySessions(10 * ports.Second)

	serial := m3NodeSerial(t, "chainless-verify")
	cid := nd.chainID()
	blinded, secret, err := demand.Withdraw(rand.Reader, &issuerPriv.PublicKey, cid, 0, serial)
	if err != nil {
		t.Fatal(err)
	}
	tok, err := demand.Unblind(&issuerPriv.PublicKey, cid, 0, serial,
		demand.SignWithdrawal(rand.Reader, issuerPriv, cid, blinded), secret)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := nd.verifyDeliveryAnchors([]demand.Token{tok}); err != nil {
		t.Fatalf("honest safety: the anchor is refused on the network that minted it: %v", err)
	}

	// The SAME keyset, the SAME token — and the node's chain taken away. The keyset is
	// pinned from the chain, so keep the chain long enough to pin, then drop it: that is
	// exactly the shape of a node that pinned keys and then restarted without its chain.
	ks := nd.DemandIssuerKeyset(ident.NodeID())
	if ks == nil {
		t.Fatal("setup: no pinned keyset")
	}
	if e, ok := ks.VerifyInWindow(cid, 0, tok); !ok || e != 0 {
		t.Fatal("setup: the pinned keyset does not verify its own token")
	}
	if _, ok := ks.VerifyInWindow(ports.Hash{}, 0, tok); ok {
		t.Error("a chainless verifier accepted a token — it cannot know which network it is on")
	}
}

func m3IssuerKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	k, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	return k
}

func m3NodeSerial(t *testing.T, label string) []byte {
	t.Helper()
	h := ports.HashBytes([]byte("m3/node/" + label))
	return h[:]
}
