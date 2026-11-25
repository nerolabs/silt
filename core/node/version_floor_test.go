package node

import (
	"crypto/ed25519"
	"errors"
	"testing"

	"github.com/nerolabs/silt/adapters/identity"
	"github.com/nerolabs/silt/adapters/memstore"
	"github.com/nerolabs/silt/adapters/simclock"
	"github.com/nerolabs/silt/adapters/simnet"
	"github.com/nerolabs/silt/core/advisory"
	"github.com/nerolabs/silt/core/chain"
	"github.com/nerolabs/silt/core/credit"
	"github.com/nerolabs/silt/core/release"
	"github.com/nerolabs/silt/ports"
)

func floorMaintainers() ([]ed25519.PrivateKey, [][]byte) {
	var privs []ed25519.PrivateKey
	var pubs [][]byte
	for i := 0; i < 3; i++ {
		seed := make([]byte, ed25519.SeedSize)
		seed[0], seed[1] = 0x6d, byte(i+1)
		p := ed25519.NewKeyFromSeed(seed)
		privs = append(privs, p)
		pubs = append(pubs, p.Public().(ed25519.PublicKey))
	}
	return privs, pubs
}

// TestVersionFloorWarnsThenRefusesConsensusFromOldReleases drives the advisory through three
// validators, two presenting as upgraded to the floor's release and one still below it.
//
//   - An advisory one maintainer signed is refused at submission and changes nothing.
//   - The maintainers' threshold advisory commits on every replica, and each reports whether it
//     is below the floor.
//   - Until the grace period passes no consensus message is refused; after it, the upgraded
//     validators refuse the old one's, while the old node still follows the chain and fetches
//     content is not refused.
func TestVersionFloorWarnsThenRefusesConsensusFromOldReleases(t *testing.T) {
	const bondSize = int64(2) << 20
	sched := simclock.New()
	net := simnet.New(sched, 7, simnet.DefaultConfig())
	privs, pubs := floorMaintainers()
	upgraded := release.Number + 1

	ids := []*identity.Identity{identity.FromSeed(8240), identity.FromSeed(8241), identity.FromSeed(8242)}
	anchors := map[ports.NodeID]bool{}
	var all []ports.NodeID
	for _, id := range ids {
		anchors[id.NodeID()] = true
		all = append(all, id.NodeID())
	}
	g := &chain.Block{Version: 1, Height: 0, Entries: []ports.Entry{mkEntry("genesis-version-floor")}}
	chain.Sign(g, ids[0].Signer())
	cfg := chain.Config{Quorum: 1, MinBond: 1 << 20, Anchors: anchors, AnchorQuorum: 1, MatureValidators: 99,
		BondTTLBlocks: 8, Era3ActivationHeight: 1, Era4ActivationHeight: 1,
		Maintainers: advisory.SortKeys(pubs), MaintainerThreshold: 2}
	var vals []*Node
	for i, id := range ids {
		ncfg := DefaultConfig()
		if i < 2 {
			ncfg.AdvertiseRelease = upgraded
		}
		nd := New(id.NodeID(), ncfg, sched, net.Endpoint(id.NodeID()), memstore.New())
		nd.SetLedger(credit.New(50_000, 0))
		ch := chain.New(cfg, func(ports.NodeID) int64 { return 0 })
		if err := ch.AppendGenesis(*g); err != nil {
			t.Fatal(err)
		}
		nd.EnableBond(id.Signer(), bondSize)
		nd.EnableChain(ch, id.Signer())
		nd.EnableObjectiveChain()
		vals = append(vals, nd)
	}
	old := vals[2]
	for _, v := range vals[1:] {
		v.Bootstrap([]ports.NodeID{all[0]}, func() {})
	}
	sched.Run()
	for i, v := range vals {
		var seed []ports.NodeID
		for j := range all {
			if j != i {
				seed = append(seed, all[j])
			}
		}
		v.StartChainSync(seed, nil)
	}
	interval := vals[0].cfg.ChainSyncInterval
	sched.RunUntil(sched.Now().Add(interval * 6))
	// work keeps the chain committing, so heights advance past the grace period
	work := 0
	tick := func() {
		work++
		vals[0].SubmitEntry(mkEntry("floor-work-"+string(rune('a'+work%26))+string(rune('a'+work/26))), all)
		sched.RunUntil(sched.Now().Add(interval))
	}

	id := vals[0].chainID()
	sign := func(a advisory.Advisory, signers ...int) *advisory.Signed {
		s := advisory.Signed{Advisory: a}
		for _, i := range signers {
			var err error
			if s, err = advisory.Sign(s, privs[i], id); err != nil {
				t.Fatal(err)
			}
		}
		return &s
	}
	enforce := advisory.Advisory{Seq: 1, Floor: upgraded, Tier: advisory.TierEnforce, GraceBlocks: 4, Note: "upgrade"}

	if err := vals[0].SubmitAdvisory(sign(enforce, 0)); !errors.Is(err, chain.ErrAdvisoryRefused) {
		t.Fatalf("one maintainer's advisory must be refused at submission, got %v", err)
	}
	sched.RunUntil(sched.Now().Add(interval * 4))
	for _, v := range vals {
		if _, ok := v.VersionFloor(); ok {
			t.Fatal("an advisory below the threshold changed something: an advisory is in force")
		}
	}

	if err := vals[1].SubmitAdvisory(sign(enforce, 0, 2)); err != nil {
		t.Fatalf("the threshold's advisory must be accepted: %v", err)
	}
	var committedAt uint64
	for i := 0; i < 40; i++ {
		tick()
		if s, ok := vals[0].VersionFloor(); ok {
			committedAt = s.CommittedAt
			break
		}
	}
	if committedAt == 0 {
		_, h := vals[0].Chain().Head()
		t.Fatalf("the advisory never committed (head %d)", h)
	}
	for i, v := range vals {
		s, ok := v.VersionFloor()
		if !ok || s.Seq != 1 || s.CommittedAt != committedAt {
			t.Fatalf("validator %d: the advisory in force must be the same on every replica, got %+v ok=%v", i, s, ok)
		}
		if s.BelowFloor != (v == old) {
			t.Fatalf("validator %d: below-floor must be true only for the old release, got %v", i, s.BelowFloor)
		}
	}

	// Inside the grace period nothing is refused.
	for i, v := range vals {
		if v.Stats.FloorRefused != 0 {
			t.Fatalf("validator %d refused %d consensus message(s) before enforcement", i, v.Stats.FloorRefused)
		}
	}

	// Past the grace period the upgraded validators refuse the old one's consensus messages.
	for i := 0; i < 60; i++ {
		tick()
		if s, _ := vals[0].VersionFloor(); s.Enforcing && vals[0].Stats.FloorRefused+vals[1].Stats.FloorRefused > 0 {
			break
		}
	}
	s0, _ := vals[0].VersionFloor()
	if !s0.Enforcing {
		_, h := vals[0].Chain().Head()
		t.Fatalf("enforcement never started (head %d, enforcing at %d)", h, s0.EnforcingAt)
	}
	if vals[0].Stats.FloorRefused+vals[1].Stats.FloorRefused == 0 {
		t.Fatal("past the grace period no upgraded validator refused a consensus message from the old release")
	}

	// A consensus message from the old release is refused; a content request is not.
	if !vals[0].floorRefuses(ports.Message{Kind: ports.MsgProposeBlock, Release: release.Number}) {
		t.Fatal("a block proposal from the old release must be refused under enforcement")
	}
	if vals[0].floorRefuses(ports.Message{Kind: ports.MsgFetchChunk, Release: release.Number}) ||
		vals[0].floorRefuses(ports.Message{Kind: ports.MsgGetChain, Release: release.Number}) ||
		vals[0].floorRefuses(ports.Message{Kind: ports.MsgSubmitEntry, Release: release.Number}) {
		t.Fatal("content, chain sync and a publisher's entry must not be refused from an old release")
	}
	if vals[0].floorRefuses(ports.Message{Kind: ports.MsgProposeBlock, Release: upgraded}) {
		t.Fatal("a proposal from a release at the floor must not be refused")
	}

	// The old node still follows the chain the upgraded validators commit.
	for i := 0; i < 10; i++ {
		tick()
	}
	_, hNew := vals[0].Chain().Head()
	_, hOld := old.Chain().Head()
	if hOld+2 < hNew {
		t.Fatalf("the old node stopped following the chain: head %d, network head %d", hOld, hNew)
	}

	// A recall lifts the floor.
	if err := vals[0].SubmitAdvisory(sign(advisory.Advisory{Seq: 2, Tier: advisory.TierRecall}, 1, 2)); err != nil {
		t.Fatalf("a recall must be accepted: %v", err)
	}
	for i := 0; i < 40; i++ {
		tick()
		if s, _ := vals[0].VersionFloor(); s.Seq == 2 {
			break
		}
	}
	if s, _ := vals[0].VersionFloor(); s.Seq != 2 || s.Enforcing || vals[0].floorRefuses(ports.Message{Kind: ports.MsgProposeBlock, Release: release.Number}) {
		t.Fatalf("after the recall nothing is enforced, got %+v", s)
	}
}
