package chain

import (
	"crypto/ed25519"
	"errors"
	"testing"

	"github.com/nerolabs/silt/core/advisory"
	"github.com/nerolabs/silt/ports"
)

// testMaintainers are three fixed maintainer keys; a chain configured with them at threshold 2
// admits an advisory two of them sign.
func testMaintainers() ([]ed25519.PrivateKey, [][]byte) {
	var privs []ed25519.PrivateKey
	var pubs [][]byte
	for i := 0; i < 3; i++ {
		seed := make([]byte, ed25519.SeedSize)
		seed[0], seed[1] = 0x4d, byte(i+1)
		p := ed25519.NewKeyFromSeed(seed)
		privs = append(privs, p)
		pubs = append(pubs, p.Public().(ed25519.PublicKey))
	}
	return privs, pubs
}

// withMaintainers returns cfg with the test maintainer set at threshold 2.
func withMaintainers(cfg Config) Config {
	_, pubs := testMaintainers()
	cfg.Maintainers, cfg.MaintainerThreshold = pubs, 2
	return cfg
}

// signedAdvisory signs a under chainID by the given test maintainers (by index).
func signedAdvisory(t testing.TB, chainID ports.Hash, a advisory.Advisory, signers ...int) *advisory.Signed {
	t.Helper()
	privs, _ := testMaintainers()
	s := advisory.Signed{Advisory: a}
	for _, i := range signers {
		var err error
		if s, err = advisory.Sign(s, privs[i], chainID); err != nil {
			t.Fatal(err)
		}
	}
	return &s
}

// stateRootFixture's witness for extra written keys (add or overwrite), proven against the
// pre-state like witnessForBlock's own writes.
func (f stateRootFixture) withKeys(t *testing.T, w StateRootWitness, keys [][]byte) StateRootWitness {
	t.Helper()
	for _, k := range keys {
		wit, err := f.prover.Prove(k)
		if err != nil {
			t.Fatalf("Prove(%x): %v", k, err)
		}
		w.ChangedLeaves = append(w.ChangedLeaves, StateRootChangedLeafWitness{Key: k, OldValue: f.preValue(k), Proof: wit})
	}
	return w
}

// TestAdvisoryBlockVerdictFollowsTheGenesisMaintainers drives item 19's fourth clause at the
// chain: an advisory below the maintainers' threshold changes nothing — the block carrying it is
// refused and no advisory is in force — while the threshold's advisory commits, is in force from
// its height, and refuses its own replay. A recall supersedes it. Both validation paths (the full
// node and the floor-box composition) return the same verdict on each block.
func TestAdvisoryBlockVerdictFollowsTheGenesisMaintainers(t *testing.T) {
	f := buildStateRootFixtureWith(t, withMaintainers)
	id := f.c.ChainID()
	next := func(adv *advisory.Signed) Block {
		prev, h := f.c.Head()
		b := Block{Version: BlockVersionWitnessable, Height: h, Prev: prev, Advisory: adv}
		return b
	}
	enforce := advisory.Advisory{Seq: 1, Floor: 2, Tier: advisory.TierEnforce, GraceBlocks: 5, Note: "release 2 fixes the relay crash"}

	for name, adv := range map[string]*advisory.Signed{
		"one maintainer":               signedAdvisory(t, id, enforce, 0),
		"one maintainer signing twice": signedAdvisory(t, id, enforce, 1, 1),
		"another network's":            signedAdvisory(t, ports.HashBytes([]byte("elsewhere")), enforce, 0, 1),
	} {
		b := next(adv)
		err := f.c.validateVersionFloor(&b)
		if !errors.Is(err, ErrAdvisoryRefused) {
			t.Fatalf("%s: the full node must refuse an advisory below the threshold, got %v", name, err)
		}
		if out, verr := v5ValidateVersionFloor(liveView{c: f.c}, &b); out != Reject || !errors.Is(verr, ErrAdvisoryRefused) {
			t.Fatalf("%s: the composition must refuse it too, got %v %v", name, out, verr)
		}
	}
	if _, _, ok := f.c.VersionFloor(); ok {
		t.Fatal("a refused advisory must leave no advisory in force")
	}

	// Pre-v5 blocks carry no advisory: the era-3 leaf set does not commit it.
	old := next(signedAdvisory(t, id, enforce, 0, 1))
	old.Version = BlockVersionWitnessable - 1
	if err := f.c.validateVersionFloor(&old); !errors.Is(err, ErrAdvisoryEra) {
		t.Fatalf("a pre-v5 advisory must be refused by era, got %v", err)
	}

	b := next(signedAdvisory(t, id, enforce, 0, 2))
	if err := f.c.validateVersionFloor(&b); err != nil {
		t.Fatalf("the threshold's advisory must be admitted: %v", err)
	}
	if out, err := v5ValidateVersionFloor(liveView{c: f.c}, &b); out != Accept {
		t.Fatalf("the composition must admit it too: %v %v", out, err)
	}
	f.c.apply(b)
	a, at, ok := f.c.VersionFloor()
	if !ok || a != enforce || at != b.Height {
		t.Fatalf("the advisory in force must be the committed one at its height: %+v at %d (ok %v)", a, at, ok)
	}
	if advisory.Enforcing(a, at, at+4) || !advisory.Enforcing(a, at, at+5) {
		t.Fatal("enforcement starts after the grace, counted in blocks from the committing height")
	}

	replay := next(signedAdvisory(t, id, enforce, 1, 2))
	if err := f.c.validateVersionFloor(&replay); !errors.Is(err, ErrAdvisoryStale) {
		t.Fatalf("a replay of the advisory in force must be refused as stale, got %v", err)
	}
	if out, err := v5ValidateVersionFloor(liveView{c: f.c}, &replay); out != Reject || !errors.Is(err, ErrAdvisoryStale) {
		t.Fatalf("the composition must refuse the replay too, got %v %v", out, err)
	}

	recall := next(signedAdvisory(t, id, advisory.Advisory{Seq: 2, Tier: advisory.TierRecall}, 1, 2))
	if err := f.c.validateVersionFloor(&recall); err != nil {
		t.Fatalf("a recall with a higher sequence must be admitted: %v", err)
	}
	f.c.apply(recall)
	if a, _, _ := f.c.VersionFloor(); a.Tier != advisory.TierRecall || advisory.Enforcing(a, 0, 1<<40) {
		t.Fatalf("after the recall no floor is enforced, got %+v", a)
	}
}

// TestGenesisCarriesNoAdvisory: a network starts with no floor.
func TestGenesisCarriesNoAdvisory(t *testing.T) {
	c := New(withMaintainers(Config{Quorum: 1}), func(ports.NodeID) int64 { return 0 })
	g := &Block{Version: BlockVersionWitnessable, Height: 0, Entries: []ports.Entry{entry(0)},
		Advisory: signedAdvisory(t, ports.Hash{}, advisory.Advisory{Seq: 1, Floor: 2, Tier: advisory.TierNotice}, 0, 1)}
	Sign(g, key(1))
	if err := c.AppendGenesis(*g); !errors.Is(err, ErrAdvisoryAtGenesis) {
		t.Fatalf("a genesis carrying an advisory must be refused, got %v", err)
	}
}
