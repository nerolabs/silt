package node

// R0.4b — the MANDATORY targeted-key equivocation gate (research certification
// 2026-09-02, Verdict 2: "a targeted-key equivocation gate ... is MANDATORY; without
// it the construction is certified-unsafe").
//
// The attack these gates close: per-epoch issuer keys make "which key verified you" a
// property of the token. An issuer that serves a DISTINCT key_E to a small cohort
// therefore partitions the epoch's anonymity set into cohorts — a fingerprint the
// no-epoch design did not have. Pinning a fetched keyset does NOT close it: an issuer
// willing to equivocate on keys equivocates on its published key LIST too, serving
// list A to cohort A and list B to cohort B. Only a CONSENSUS-ATTESTED binding forces
// one answer, and these gates prove the redeemer actually enforces it.

import (
	"crypto/rand"
	"crypto/rsa"
	"testing"

	"github.com/nerolabs/silt/adapters/identity"
	"github.com/nerolabs/silt/adapters/memstore"
	"github.com/nerolabs/silt/adapters/simclock"
	"github.com/nerolabs/silt/adapters/simnet"
	"github.com/nerolabs/silt/core/chain"
	"github.com/nerolabs/silt/core/demand"
	"github.com/nerolabs/silt/ports"
)

// issuerKeyFixture is a node holding a chain whose genesis commits `committed` as the
// issuer's key for epoch 0.
type issuerKeyFixture struct {
	nd        *Node
	issuer    ports.NodeID
	committed *rsa.PrivateKey // the key whose fingerprint is bound on-chain
}

func newIssuerKeyFixture(t *testing.T, seed int64) issuerKeyFixture {
	t.Helper()
	committed, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("committed issuer key: %v", err)
	}

	sched := simclock.New()
	net := simnet.New(sched, 2, simnet.DefaultConfig())
	ident := identity.FromSeed(seed)
	id := ident.NodeID()
	nd := New(id, DefaultConfig(), sched, net.Endpoint(id), memstore.New())
	nd.SetSigner(ident.Signer())

	// A minimal chain whose GENESIS commits the binding for epoch 0. Epochs are off
	// (EpochBlocks == 0), so the node's consensus epoch is 0 throughout.
	c := chain.New(chain.Config{Quorum: 1}, func(ports.NodeID) int64 { return 1 << 30 })
	g := chain.Block{
		Version: chain.BlockVersionWitnessable,
		Height:  0,
		Entries: []ports.Entry{{Root: ports.HashBytes([]byte("issuerkey-genesis-entry"))}},
		IssuerKeys: []chain.IssuerKeyReg{
			chain.SignIssuerKeyReg(ident.Signer(), 0, demand.KeyFingerprint(&committed.PublicKey)),
		},
	}
	chain.Sign(&g, ident.Signer())
	if err := c.AppendGenesis(g); err != nil {
		t.Fatalf("genesis with an issuer-key binding: %v", err)
	}
	if got, ok := c.IssuerKeyCommitment(id, 0); !ok || got != demand.KeyFingerprint(&committed.PublicKey) {
		t.Fatalf("setup: the binding was not committed (got %x, ok=%v)", got, ok)
	}
	nd.EnableChain(c, ident.Signer())
	return issuerKeyFixture{nd: nd, issuer: id, committed: committed}
}

// TestIssuerKey_OffCommitmentKeyIsRefused is THE gate. A key that is not the
// committed one for its epoch must be REFUSED — it never enters the keyset, so no
// token signed by it can ever redeem.
//
// The forged key is a perfectly well-formed RSA key that signs perfectly valid blind
// signatures. Nothing about the key itself is detectable; only the commitment
// distinguishes it. That is the whole point.
func TestIssuerKey_OffCommitmentKeyIsRefused(t *testing.T) {
	f := newIssuerKeyFixture(t, 9301)

	// A DIFFERENT, valid RSA key — the per-cohort key a targeting issuer would serve.
	targeted, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("targeted key: %v", err)
	}
	if f.nd.pinDemandIssuerKey(f.issuer, 0, &targeted.PublicKey) {
		t.Fatal("an OFF-COMMITMENT key_E was pinned — a Byzantine issuer can hand a " +
			"per-cohort key and turn 'which key verified you' into a fingerprint")
	}
	if ks := f.nd.DemandIssuerKeyset(f.issuer); ks != nil && ks.Key(0) != nil {
		t.Fatal("the refused key still landed in the keyset")
	}

	// A token signed by the targeted key must therefore verify against nothing.
	if ks := f.nd.DemandIssuerKeyset(f.issuer); ks != nil {
		tok := blindTokenUnder(t, targeted)
		if _, ok := ks.VerifyInWindow(0, tok); ok {
			t.Fatal("a token signed by an off-commitment key verified")
		}
	}

	// The COMMITTED key is accepted, and its tokens verify — the gate must refuse the
	// forgery without breaking the honest path.
	if !f.nd.pinDemandIssuerKey(f.issuer, 0, &f.committed.PublicKey) {
		t.Fatal("the COMMITTED key_E was refused — the binding rejects the honest key")
	}
	ks := f.nd.DemandIssuerKeyset(f.issuer)
	if ks == nil || ks.Key(0) == nil {
		t.Fatal("the committed key was not held after a successful pin")
	}
	if _, ok := ks.VerifyInWindow(0, blindTokenUnder(t, f.committed)); !ok {
		t.Fatal("a token signed by the COMMITTED key did not verify")
	}
}

// TestIssuerKey_UncommittedEpochIsRefused closes the bypass that would make the whole
// binding optional: if a key for an epoch with NO commitment were accepted, an issuer
// could evade the binding entirely by simply never registering.
func TestIssuerKey_UncommittedEpochIsRefused(t *testing.T) {
	f := newIssuerKeyFixture(t, 9302)
	// Epoch 3 has no commitment. Even the CORRECT key must be refused there.
	if f.nd.pinDemandIssuerKey(f.issuer, 3, &f.committed.PublicKey) {
		t.Fatal("a key for an epoch with NO committed binding was pinned — an issuer " +
			"could then bypass the binding by never registering, making it worthless")
	}
}

// TestIssuerKey_PinFollowsTheChain: while the COMMITMENT is unchanged, a later serve
// of a different key for the same epoch cannot displace the pinned one — a targeting
// issuer gets no second chance after the commitment has been checked once.
//
// The invariant is stated over the CHAIN, not over the pin: "a keyset never holds a
// key whose fingerprint differs from the current commitment for that (issuer, epoch)".
// Append-only belongs to the chain (applyIssuerKeys is first-write-wins); the pin is a
// cache of it, and it must FOLLOW a re-pointed commitment — see
// TestPinFollowsTheChainAcrossAReorg, which is the other half of this rule.
func TestIssuerKey_PinFollowsTheChain(t *testing.T) {
	f := newIssuerKeyFixture(t, 9303)
	if !f.nd.pinDemandIssuerKey(f.issuer, 0, &f.committed.PublicKey) {
		t.Fatal("setup: the committed key was refused")
	}
	other, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("other key: %v", err)
	}
	f.nd.pinDemandIssuerKey(f.issuer, 0, &other.PublicKey)
	ks := f.nd.DemandIssuerKeyset(f.issuer)
	if _, ok := ks.VerifyInWindow(0, blindTokenUnder(t, f.committed)); !ok {
		t.Fatal("the pinned committed key was displaced by a later serve")
	}
	if _, ok := ks.VerifyInWindow(0, blindTokenUnder(t, other)); ok {
		t.Fatal("a second key for the SAME epoch was accepted after the pin")
	}
}

// TestIssuerKey_NoChainRefusesEverything: with no chain there is nothing
// consensus-attested to resolve against. Refusing is the certified behavior — the
// certification is explicit that running the per-epoch-key construction WITHOUT the
// binding is unsafe, because it manufactures a linkable tag the no-epoch design did
// not have. Fail closed, never open.
func TestIssuerKey_NoChainRefusesEverything(t *testing.T) {
	sched := simclock.New()
	net := simnet.New(sched, 2, simnet.DefaultConfig())
	ident := identity.FromSeed(9304)
	id := ident.NodeID()
	nd := New(id, DefaultConfig(), sched, net.Endpoint(id), memstore.New())

	k, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("key: %v", err)
	}
	if nd.pinDemandIssuerKey(id, 0, &k.PublicKey) {
		t.Fatal("a key was pinned with NO chain to resolve it against — the redeemer " +
			"has no anti-fingerprinting anchor and must fail closed")
	}
}

// TestIssuerKey_SelfIssuanceGetsNoException: a node that issues its own demand tokens
// resolves its OWN key through the same commitment. A self-trust shortcut would be a
// complete bypass — every issuer is "self" from its own point of view.
func TestIssuerKey_SelfIssuanceGetsNoException(t *testing.T) {
	f := newIssuerKeyFixture(t, 9305)

	// Install an UNCOMMITTED key as this node's own issuing key for epoch 0.
	rogue, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("rogue key: %v", err)
	}
	f.nd.SetDemandIssuerKey(rand.Reader, 0, rogue)
	if ks := f.nd.DemandIssuerKeyset(f.issuer); ks != nil && ks.Key(0) != nil {
		t.Fatal("a node self-pinned its OWN uncommitted key — self-issuance must go " +
			"through the same committed binding as any peer's key")
	}

	// The committed key, installed as its own, DOES resolve.
	f.nd.SetDemandIssuerKey(rand.Reader, 0, f.committed)
	ks := f.nd.DemandIssuerKeyset(f.issuer)
	if ks == nil || ks.Key(0) == nil {
		t.Fatal("a node could not resolve its OWN committed key")
	}
}

// blindTokenUnder runs a full blind withdrawal under priv for issue epoch 0,
// producing a token that verifies under priv's public key AT EPOCH 0 and nothing
// else.
func blindTokenUnder(t *testing.T, priv *rsa.PrivateKey) demand.Token {
	t.Helper()
	return blindTokenUnderAt(t, priv, 0)
}

// blindTokenUnderAt is blindTokenUnder for an explicit issue epoch — the (b1) shape.
func blindTokenUnderAt(t *testing.T, priv *rsa.PrivateKey, epoch uint64) demand.Token {
	t.Helper()
	serial := make([]byte, 32)
	if _, err := rand.Read(serial); err != nil {
		t.Fatalf("serial: %v", err)
	}
	blinded, secret, err := demand.Withdraw(rand.Reader, &priv.PublicKey, epoch, serial)
	if err != nil {
		t.Fatalf("withdraw: %v", err)
	}
	tok, uerr := demand.Unblind(&priv.PublicKey, epoch, serial, demand.SignWithdrawal(rand.Reader, priv, blinded), secret)
	if uerr != nil {
		t.Fatalf("unblind: %v", uerr)
	}
	return tok
}
