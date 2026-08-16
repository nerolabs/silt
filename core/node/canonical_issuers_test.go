package node

import (
	"crypto/ed25519"
	"testing"

	"github.com/nerolabs/silt/adapters/identity"
	"github.com/nerolabs/silt/adapters/memstore"
	"github.com/nerolabs/silt/adapters/simclock"
	"github.com/nerolabs/silt/adapters/simnet"
	"github.com/nerolabs/silt/core/chain"
	"github.com/nerolabs/silt/ports"
)

// TestFetchCanonicalIssuers_ReturnsLedgerRankedSet pins R-3: a chainless publisher
// can fetch the deterministic canonical issuer ordering (validators ranked by
// committed bond) from a chain-holding validator, so it can pick its publish-token
// signers by a ledger-derived ranking that is the SAME for every publisher — the
// signer subset then stops being a per-publisher quasi-identifier (seam-4).
func TestFetchCanonicalIssuers_ReturnsLedgerRankedSet(t *testing.T) {
	sched := simclock.New()
	net := simnet.New(sched, 1, simnet.DefaultConfig())
	pub := func(id *identity.Identity) []byte {
		return append([]byte(nil), id.Signer().Public().(ed25519.PublicKey)...)
	}

	vID := identity.FromSeed(861) // the chain-holding validator (also bonded)
	heavy := identity.FromSeed(862)
	light := identity.FromSeed(863)
	anchors := map[ports.NodeID]bool{vID.NodeID(): true}
	g := &chain.Block{Version: 1, Height: 0, Entries: []ports.Entry{mkEntry("g")},
		BondRegs: []chain.BondReg{
			{Validator: pub(vID), Root: ports.HashBytes(pub(vID)), Size: 5 << 20},
			{Validator: pub(heavy), Root: ports.HashBytes(pub(heavy)), Size: 9 << 20}, // heaviest
			{Validator: pub(light), Root: ports.HashBytes(pub(light)), Size: 3 << 20},
		}}
	chain.Sign(g, vID.Signer())
	cfg := chain.Config{Quorum: 1, MinBond: 1 << 20, Anchors: anchors, MatureValidators: 99}

	v := New(vID.NodeID(), DefaultConfig(), sched, net.Endpoint(vID.NodeID()), memstore.New())
	ch := chain.New(cfg, func(ports.NodeID) int64 { return 0 })
	if err := ch.AppendGenesis(*g); err != nil {
		t.Fatal(err)
	}
	v.EnableChain(ch, vID.Signer())
	v.EnableObjectiveChain()

	// A chainless publisher fetches the canonical ordering from the validator.
	cID := identity.FromSeed(870)
	client := New(cID.NodeID(), DefaultConfig(), sched, net.Endpoint(cID.NodeID()), memstore.New())
	client.Bootstrap([]ports.NodeID{vID.NodeID()}, func() {})
	sched.Run()

	var got []ports.NodeID
	var ferr error
	done := false
	client.FetchCanonicalIssuers(vID.NodeID(), func(ids []ports.NodeID, e error) { got, ferr, done = ids, e, true })
	sched.Run()
	if !done || ferr != nil {
		t.Fatalf("fetch canonical issuers: done=%v err=%v", done, ferr)
	}
	if len(got) != 3 {
		t.Fatalf("expected all 3 bonded validators, got %d", len(got))
	}
	// Ledger-ranked: the heaviest bond (9M) ranks first, the lightest (3M) last.
	if got[0] != heavy.NodeID() {
		t.Fatalf("canonical set must be ranked by committed bond (heaviest first): got[0]=%x want heavy=%x", got[0], heavy.NodeID())
	}
	if got[2] != light.NodeID() {
		t.Fatalf("lightest bond must rank last: got[2]=%x want light=%x", got[2], light.NodeID())
	}

	// A chainless peer serves nothing (OK=false → an error, so a publisher falls
	// back to its -peers). The validator knows the client (it bootstrapped to v), so
	// v can query it; the client holds no chain.
	done = false
	var fb error
	v.FetchCanonicalIssuers(cID.NodeID(), func(_ []ports.NodeID, e error) { fb, done = e, true })
	sched.Run()
	if !done || fb == nil {
		t.Fatalf("a chainless peer must not serve a canonical set (expected an error), done=%v err=%v", done, fb)
	}

	// #351: FetchCanonicalIssuersFromAny must SKIP a validator that serves no chain
	// (un-synced / just restarted) and get the deterministic set from the next one —
	// so a single down validator can't drop the publisher into the anonymity-narrowing
	// -peers fallback. Ask the chainless client FIRST, then the chain-holding v: the
	// single-target FetchCanonicalIssuers on the bad one errors, but FromAny falls
	// through to v and returns the same ledger-ranked set.
	var any []ports.NodeID
	var anyErr error
	done = false
	client.FetchCanonicalIssuersFromAny([]ports.NodeID{cID.NodeID(), vID.NodeID()},
		func(ids []ports.NodeID, e error) { any, anyErr, done = ids, e, true })
	sched.Run()
	if !done || anyErr != nil {
		t.Fatalf("#351: FromAny must fall through the un-synced validator to a chain-holder: done=%v err=%v", done, anyErr)
	}
	if len(any) != 3 || any[0] != heavy.NodeID() {
		t.Fatalf("#351: FromAny must return the chain-holder's ledger-ranked set (heaviest first): got %d, want 3", len(any))
	}
}
