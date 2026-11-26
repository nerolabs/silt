package chain

import (
	"crypto/ed25519"

	"github.com/nerolabs/silt/ports"
)

// testCurator is the curator every v5 test fixture's takedowns are signed by unless a case sets
// its own authority: one fixed key, so an un-revocation lifts the listing its revocation made.
var testCurator = ed25519.NewKeyFromSeed(make([]byte, ed25519.SeedSize))

// secondCurator is a curator independent of testCurator, for a listing of a root testCurator
// already lists.
var secondCurator = ed25519.NewKeyFromSeed(append(make([]byte, ed25519.SeedSize-1), 2))

// testCuratorID is testCurator's curator ID.
var testCuratorID = ports.HashBytes(testCurator.Public().(ed25519.PublicKey))

// fixtureNonce hands every fixture authority a fresh nonce, as an honest curator would.
var fixtureNonce uint64

func nextNonce() uint64 { fixtureNonce++; return fixtureNonce }

// curateTakedowns signs a witnessable block's takedowns as testCurator when the case has not set
// authorities of its own. Older blocks' takedowns stay bare, as their frozen rules require.
func curateTakedowns(b *Block, chainID ports.Hash) {
	if !curated(b) || b.TakedownAuths != nil || len(b.Revocations)+len(b.Unrevocations) == 0 {
		return
	}
	for _, r := range b.Revocations {
		b.TakedownAuths = append(b.TakedownAuths, SignTakedown(testCurator, chainID, RevOp, r, nextNonce()))
	}
	for _, r := range b.Unrevocations {
		b.TakedownAuths = append(b.TakedownAuths, SignTakedown(testCurator, chainID, UnrevOp, r, nextNonce()))
	}
}

// testListing is testCurator's listing of root.
func testListing(root ports.Hash) Listing { return Listing{Curator: testCuratorID, Root: root} }
