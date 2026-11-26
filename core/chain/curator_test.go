package chain

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/rsa"
	"errors"
	mrand "math/rand"
	"testing"

	"github.com/nerolabs/silt/core/blindtoken"
	"github.com/nerolabs/silt/ports"
)

// bothRefuse asserts the full node and the floor box's P6 mirror refuse b with the same sentinel.
func bothRefuse(t *testing.T, f structFixture, b Block, want error, why string) {
	t.Helper()
	if err := f.c.ValidateCommit(&b); !errors.Is(err, want) {
		t.Fatalf("%s: the node must refuse with %v, got %v", why, want, err)
	}
	out, err := v5ValidateTakedowns(liveView{f.c}, &b)
	if out != Reject || !errors.Is(err, want) {
		t.Fatalf("%s: the floor box must refuse with %v, got %s / %v", why, want, out, err)
	}
}

// commitBoth appends b after checking the floor box's P6 mirror accepts it too.
func commitBoth(t *testing.T, f structFixture, b Block, why string) {
	t.Helper()
	if out, err := v5ValidateTakedowns(liveView{f.c}, &b); out != Accept {
		t.Fatalf("%s: the floor box must accept, got %s / %v", why, out, err)
	}
	if err := f.c.Append(b); err != nil {
		t.Fatalf("%s: the node must commit, got %v", why, err)
	}
}

// TestATakedownNeedsItsCuratorsSignature. A witnessable block's takedown is authorized by its
// curator's signature over (network, operation, root) and by nothing else: a validator adds no
// takedown of its own, and a signature for another root, operation or network authorizes nothing.
// This closes "any proposer can take down anything" (the September red team).
func TestATakedownNeedsItsCuratorsSignature(t *testing.T) {
	f := buildStructFixture(t)
	root := entry(1).Root
	other := ports.HashBytes([]byte("another network"))
	forged := SignTakedown(secondCurator, f.c.ChainID(), RevOp, root, nextNonce())
	forged.Curator = append([]byte(nil), testCurator.Public().(ed25519.PublicKey)...) // claims testCurator, signed by another

	for _, tc := range []struct {
		name string
		auth []TakedownAuth
		want error
	}{
		{"no authority (a bare proposer takedown)", []TakedownAuth{}, ErrTakedownUnauthorized},
		{"a signature by a key other than the named curator", []TakedownAuth{forged}, ErrTakedownSig},
		{"a signature for another network", []TakedownAuth{SignTakedown(testCurator, other, RevOp, root, nextNonce())}, ErrTakedownSig},
		{"a signature for the reverse operation", []TakedownAuth{SignTakedown(testCurator, f.c.ChainID(), UnrevOp, root, nextNonce())}, ErrTakedownSig},
		{"a signature for another root", []TakedownAuth{SignTakedown(testCurator, f.c.ChainID(), RevOp, entry(0).Root, nextNonce())}, ErrTakedownSig},
		{"two authorities for one takedown", []TakedownAuth{SignTakedown(testCurator, f.c.ChainID(), RevOp, root, nextNonce()), SignTakedown(testCurator, f.c.ChainID(), RevOp, root, nextNonce())}, ErrTakedownUnauthorized},
	} {
		auth := tc.auth
		b := f.mkBlock(t, func(b *Block) { b.Revocations, b.TakedownAuths = []ports.Hash{root}, auth })
		if len(auth) == 0 {
			b.TakedownAuths = nil // the bare twin, as an older proposer would send it
		}
		bothRefuse(t, f, b, tc.want, tc.name)
	}
	// The honest twin: the same takedown with its curator's signature commits.
	commitBoth(t, f, f.mkBlock(t, func(b *Block) { b.Revocations = []ports.Hash{root} }), "the signed takedown")
	if !f.c.Listed(testListing(root)) || !f.c.ListedBy(root, []ports.NodeID{testCuratorID}) {
		t.Fatal("the committed takedown is not testCurator's live listing")
	}
	if f.c.Revoked(root) {
		t.Fatal("a curated listing must not read as a pre-curator bare takedown")
	}
}

// TestListingsArePerCurator. Two curators list the same root independently, a curator cannot list
// a root twice, and only the curator that listed a root can lift its own listing — lifting one
// leaves the other standing. An operator following only one sees only that one's listing.
func TestListingsArePerCurator(t *testing.T) {
	f := buildStructFixture(t)
	root := entry(1).Root
	secondID := ports.HashBytes(secondCurator.Public().(ed25519.PublicKey))
	sign := func(k ed25519.PrivateKey, op byte) []TakedownAuth {
		return []TakedownAuth{SignTakedown(k, f.c.ChainID(), op, root, nextNonce())}
	}

	commitBoth(t, f, f.mkBlock(t, func(b *Block) { b.Revocations = []ports.Hash{root} }), "testCurator lists the root")
	bothRefuse(t, f, f.mkBlock(t, func(b *Block) { b.Revocations, b.TakedownAuths = []ports.Hash{root}, sign(testCurator, RevOp) }),
		ErrAlreadyListed, "testCurator listing it again")
	bothRefuse(t, f, f.mkBlock(t, func(b *Block) { b.Unrevocations, b.TakedownAuths = []ports.Hash{root}, sign(secondCurator, UnrevOp) }),
		ErrUnrevokeNotRevoked, "secondCurator lifting testCurator's listing")

	commitBoth(t, f, f.mkBlock(t, func(b *Block) { b.Revocations, b.TakedownAuths = []ports.Hash{root}, sign(secondCurator, RevOp) }),
		"secondCurator lists the same root")
	if !f.c.ListedBy(root, []ports.NodeID{secondID}) || !f.c.ListedBy(root, []ports.NodeID{testCuratorID}) {
		t.Fatal("both curators' listings must be live")
	}
	commitBoth(t, f, f.mkBlock(t, func(b *Block) { b.Unrevocations, b.TakedownAuths = []ports.Hash{root}, sign(secondCurator, UnrevOp) }),
		"secondCurator lifts its own listing")
	if f.c.ListedBy(root, []ports.NodeID{secondID}) {
		t.Fatal("secondCurator's lifted listing is still live")
	}
	if !f.c.ListedBy(root, []ports.NodeID{testCuratorID}) {
		t.Fatal("lifting one curator's listing removed another's")
	}
	if f.c.ListedBy(root, nil) {
		t.Fatal("an operator following no curator must see no listing")
	}
}

// TestATakedownReceiptNamesItsCurator. The transparency log's leaf names the curator, so the
// receipt an honouring operator publishes proves who asked for the removal: rename the curator and
// it no longer verifies. A replica's blocks alone rebuild the same receipt.
func TestATakedownReceiptNamesItsCurator(t *testing.T) {
	f := buildStructFixture(t)
	root := entry(1).Root
	commitBoth(t, f, f.mkBlock(t, func(b *Block) { b.Revocations = []ports.Hash{root} }), "testCurator lists the root")

	r, err := f.c.RevocationReceiptFor(testListing(root))
	if err != nil {
		t.Fatalf("receipt: %v", err)
	}
	if r.Curator != testCuratorID {
		t.Fatalf("the receipt names curator %s, want %s", r.Curator, testCuratorID)
	}
	anchor := f.c.BlockAt(r.At)
	if err := VerifyRevocationReceipt(r, anchor); err != nil {
		t.Fatalf("the honest receipt must verify: %v", err)
	}
	renamed := r
	renamed.Curator = ports.HashBytes(secondCurator.Public().(ed25519.PublicKey))
	if VerifyRevocationReceipt(renamed, anchor) == nil {
		t.Fatal("a receipt with its curator renamed still verified: the log does not bind who asked")
	}
	if _, err := f.c.RevocationReceiptFor(Listing{Curator: renamed.Curator, Root: root}); !errors.Is(err, ErrNotRevoked) {
		t.Fatalf("a curator with no listing has no receipt, got %v", err)
	}
	fromBlocks, err := ReceiptFromBlocks(f.c.Blocks(0), testListing(root), 0)
	if err != nil || fromBlocks.Index != r.Index || fromBlocks.Curator != r.Curator {
		t.Fatalf("a replica's blocks must rebuild the same receipt: %+v / %v", fromBlocks, err)
	}
}

// TestATakedownPaysAFee. On a network that requires publish tokens, a takedown pays with one,
// exactly as a publish does: no token is refused, a spent token is refused, and one token cannot
// pay for a publish and a takedown in the same block.
func TestATakedownPaysAFee(t *testing.T) {
	f := buildStructFixture(t)
	issuers := map[ports.NodeID]*rsa.PrivateKey{}
	for _, k := range f.keys[:2] {
		rk, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			t.Fatal(err)
		}
		issuers[idOf(k)] = rk
	}
	f.c.RequireTokens(2, func(n ports.NodeID) *rsa.PublicKey {
		if k, ok := issuers[n]; ok {
			return &k.PublicKey
		}
		return nil
	})
	rng := mrand.New(mrand.NewSource(7))
	mint := func() *ports.PublishToken {
		serial, _ := blindtoken.NewSerial(rng)
		tok := &ports.PublishToken{Serial: serial}
		for _, k := range f.keys[:2] {
			iss := blindtoken.NewIssuer(rand.Reader, issuers[idOf(k)])
			blinded, secret, err := blindtoken.Blind(rng, iss.Public(), serial)
			if err != nil {
				t.Fatal(err)
			}
			blindSig, _ := iss.Issue(func() error { return nil }, blinded)
			sig, err := blindtoken.Unblind(iss.Public(), serial, blindSig, secret)
			if err != nil {
				t.Fatal(err)
			}
			tok.Sigs = append(tok.Sigs, ports.TokenSig{Validator: idOf(k), Sig: sig})
		}
		return tok
	}
	takedown := func(root ports.Hash, fee *ports.PublishToken, entries ...ports.Entry) Block {
		return f.mkBlock(t, func(b *Block) {
			a := SignTakedown(testCurator, f.c.ChainID(), RevOp, root, nextNonce())
			a.Token = fee
			b.Entries, b.Revocations, b.TakedownAuths = entries, []ports.Hash{root}, []TakedownAuth{a}
		})
	}

	bothRefuse(t, f, takedown(entry(1).Root, nil), ErrTokenRequired, "a takedown with no fee")
	fee := mint()
	commitBoth(t, f, takedown(entry(1).Root, fee), "a takedown paid with a fresh token")
	bothRefuse(t, f, takedown(entry(0).Root, fee), ErrTokenSpent, "a takedown paid with a spent token")
	shared := mint()
	e := entry(0x61)
	e.Token = shared
	bothRefuse(t, f, takedown(entry(0).Root, shared, e), ErrTokenSpent, "one token paying for a publish and a takedown in one block")
}

// TestACuratorsAuthorityIsSingleUse. Every committed authority is spent, so a curator's signature
// cannot be replayed by anyone who saw it: not in the same block, not to re-list a root the curator
// lifted. The curator itself re-lists with a fresh nonce.
func TestACuratorsAuthorityIsSingleUse(t *testing.T) {
	f := buildStructFixture(t)
	root := entry(1).Root
	listing := SignTakedown(testCurator, f.c.ChainID(), RevOp, root, nextNonce())
	with := func(revs, unrevs []ports.Hash, auths ...TakedownAuth) Block {
		return f.mkBlock(t, func(b *Block) { b.Revocations, b.Unrevocations, b.TakedownAuths = revs, unrevs, auths })
	}

	bothRefuse(t, f, with([]ports.Hash{root, root}, nil, listing, listing), ErrTakedownReplayed, "one authority used twice in a block")
	commitBoth(t, f, with([]ports.Hash{root}, nil, listing), "the curator lists the root")
	commitBoth(t, f, with(nil, []ports.Hash{root}, SignTakedown(testCurator, f.c.ChainID(), UnrevOp, root, nextNonce())), "the curator lifts it")
	bothRefuse(t, f, with([]ports.Hash{root}, nil, listing), ErrTakedownReplayed, "a third party replaying the lifted listing")
	commitBoth(t, f, with([]ports.Hash{root}, nil, SignTakedown(testCurator, f.c.ChainID(), RevOp, root, nextNonce())),
		"the curator re-listing under a fresh nonce")
}
