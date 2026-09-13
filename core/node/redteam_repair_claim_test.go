package node

// H7 slice-2 acceptance red-team (design §11): drive the REAL repair-claim
// verification handler over a live network with self-dealing / false-repair
// adversaries and assert the WIRED verdict — slash for an attributable correctness
// lie, deny (never slash) for a retrievability shortfall. The pure legs are
// unit-tested in core/repairproof; these prove the node wiring
// (fetch-survivors + challenge-holder + Decide + settle) delivers the crypto's
// verdict to the ledger against a real attacker. Each is a permanent regression.

import (
	"bytes"
	"go/ast"
	"go/parser"
	"go/token"
	"sort"
	"strings"
	"testing"

	"github.com/nerolabs/silt/adapters/identity"
	"github.com/nerolabs/silt/adapters/memstore"
	"github.com/nerolabs/silt/adapters/simclock"
	"github.com/nerolabs/silt/adapters/simnet"
	"github.com/nerolabs/silt/core/credit"
	"github.com/nerolabs/silt/core/crypto"
	"github.com/nerolabs/silt/core/erasure"
	"github.com/nerolabs/silt/core/link"
	"github.com/nerolabs/silt/core/manifest"
	"github.com/nerolabs/silt/core/pipeline"
	"github.com/nerolabs/silt/core/por"
	"github.com/nerolabs/silt/core/registry"
	"github.com/nerolabs/silt/core/repairproof"
	"github.com/nerolabs/silt/ports"
)

// repairAdv is a small live cluster with a coded object distributed across it — the
// stage for driving handleRepairClaim against crafted claims.
type repairAdv struct {
	sched  *simclock.Scheduler
	net    *simnet.Network
	nodes  []*Node
	reg    ports.Registry
	ledger *credit.Ledger
	m      *manifest.Manifest
	root   ports.Hash
	porKey *por.Key
	h      link.Handle
}

func newRepairAdv(t *testing.T, seed int64) *repairAdv {
	t.Helper()
	const N = 24
	sched := simclock.New()
	net := simnet.New(sched, seed, simnet.DefaultConfig())
	reg := registry.New()
	ledger := credit.New(50_000, 10_000_000) // grant funders enough to prepay a reserve
	cfg := DefaultConfig()
	cfg.Replication = 3
	cfg.RepairEconomy = true

	var nodes []*Node
	for i := 0; i < N; i++ {
		id := identity.FromSeed(int64(1000 + i)).NodeID()
		nd := New(id, cfg, sched, net.Endpoint(id), memstore.New())
		nd.SetLedger(ledger)
		ledger.Register(id)
		nodes = append(nodes, nd)
	}
	for i, nd := range nodes {
		if i == 0 {
			continue
		}
		var seeds []ports.NodeID
		for j := 0; j < i && j < 3; j++ {
			seeds = append(seeds, nodes[j].ID())
		}
		nd.Bootstrap(seeds, func() {})
	}
	sched.Run()

	// 512 KiB frames: the shard clears one credit of fetch, so the bounty base is
	// NON-ZERO (G-R212-7 / G-λ-8; 4 KiB chunks paid 0).
	//
	// ⚠ NUMBER CORRECTION, MEASURED 2026-09-12. This line used to assert "the bounty
	// base is 2", which is wrong for the fixture it describes by a factor of k. Every
	// chunk of this object is a 524,304-byte shard (measured off the stores this helper
	// populates), and credit.RepairBountyBase(10, 524304) is 20 on the pre-F1 basis; 2
	// is what it prices at AFTER the F1 re-pricing (D-BOUNTY-PRICE-F1-2026-09-12). No
	// assertion in this file reads the base, so the property the fixture actually needs
	// — non-zero — replaces the number rather than tracking the re-price.
	//
	// ⚠ RECORD CORRECTION, MEASURED 2026-09-12. This comment used to read "exactly one
	// k=10 stripe", and the certification that routed the short-final-stripe fix
	// repeated it. BOTH ARE FALSE. splitFile reserves chunk.HeaderSize bytes of every
	// frame for its header, so 10·(512<<10) BYTES yield ELEVEN chunks, and the object
	// is TWO stripes: stripe 0 full (realData = 10, 16 stored refs) and STRIPE 1
	// SHORT (realData = 1, 7 stored refs). The conclusion that rested on the wrong
	// premise still holds — every pre-existing test in this package claims STRIPE 0,
	// so none of them ever measured a short stripe — but the fixture did not need a
	// geometry change to grow one. finalStripeParityTarget names it.
	data := make([]byte, 10*(512<<10))
	for i := range data {
		data[i] = byte(i*7 + 3)
	}
	h, err := pipeline.Add(bg(), nodes[0].Store(), reg, bytes.NewReader(data),
		pipeline.Options{ChunkSize: 512 << 10, Mode: crypto.Convergent, Erasure: erasure.DefaultParams})
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	entry, _, _ := reg.Lookup(bg(), h.Root)
	m, err := pipeline.LoadFull(bg(), nodes[0].Store(), entry, h)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	nodes[0].Distribute(entry, m, false, DerivePorKey(h.LayoutKey()), func(int, error) {})
	sched.Run()

	return &repairAdv{
		sched: sched, net: net, nodes: nodes, reg: reg, ledger: ledger,
		m: m, root: h.Root, porKey: DerivePorKey(h.LayoutKey()), h: h,
	}
}

// careJudge puts nodes[1] on duty as the caretaker-judge. It wires the caretaker
// state directly (registry + care-link) rather than calling Care(), which would
// also start the perpetual repair-tick loop and stop sched.Run() from ever reaching
// quiescence — the handler is driven directly here, so the loop isn't needed. The
// judge fetches the manifest itself when it judges.
func (s *repairAdv) careJudge() *Node {
	j := s.nodes[1]
	j.reg = s.reg
	j.care = append(j.care, s.h.Care())
	return j
}

func (s *repairAdv) fundEscrow(amount int64) {
	if err := s.ledger.FundEscrow(s.root, s.nodes[0].ID(), amount); err != nil {
		panic(err)
	}
}

// bond gives nd a comfortable bonded standing and returns it — the baseline a
// slash must lower and a deny must leave untouched.
func (s *repairAdv) bond(nd *Node) int64 {
	s.ledger.RecordBondChallenge(nd.ID(), nd.ID(), 128<<20, true, 1)
	return s.ledger.Reputation(nd.ID())
}

// deliverClaim hands a crafted claim to the judge as if `from` had broadcast it,
// then runs the network to quiescence so the two legs settle.
func (s *repairAdv) deliverClaim(judge *Node, from ports.NodeID, claim repairproof.RepairClaim) {
	data, err := claim.Marshal()
	if err != nil {
		panic(err)
	}
	judge.handleRepairClaim(from, ports.Message{Kind: ports.MsgRepairClaim, Data: data})
	s.sched.Run()
}

// parityTarget names STRIPE 0's first parity shard: its stripe position, its
// manifest-committed id, and its Merkle leaf index. Stripe 0 is the FULL stripe
// (realData = k), the row with maximum survivor slack.
func (s *repairAdv) parityTarget() (pos int, id ports.ChunkID, leafIdx int) {
	return s.m.K, s.m.ParityIDs()[0], len(s.m.ChunkIDs())
}

// finalStripeParityTarget names the FINAL stripe's first parity shard — the SHORT
// stripe (measured: realData = 1 of k = 10, 7 stored refs). A judge excluding the
// claimed position can supply at most 6 survivors there, so before the `present`
// fix that position was structurally unjudgeable FOREVER: the paramedic repairs it
// and no judge can ever judge it. It returns the stripe index alongside the rest,
// because unlike parityTarget it is not stripe 0.
//
// It derives every value from the fixture's own manifest rather than transcribing
// the measured numbers, so a fixture whose geometry moves reports the move instead
// of silently re-measuring stripe 0.
func (s *repairAdv) finalStripeParityTarget() (stripe, pos int, id ports.ChunkID, leafIdx int) {
	p := erasure.Params{K: s.m.K, N: s.m.N}
	dataIDs, parityIDs := s.m.ChunkIDs(), s.m.ParityIDs()
	stripe = p.Stripes(len(dataIDs)) - 1
	pos = p.K
	id = parityIDs[stripe*p.ParityShards()]
	leafIdx = len(dataIDs) + stripe*p.ParityShards()
	return stripe, pos, id, leafIdx
}

// finalStripeRealData is how many REAL data chunks the final stripe carries, read
// off the manifest. It is what makes the short-stripe gate's anti-vacuity check a
// derivation rather than a transcribed 1.
func (s *repairAdv) finalStripeRealData() int {
	p := erasure.Params{K: s.m.K, N: s.m.N}
	n := len(s.m.ChunkIDs())
	return n - (p.Stripes(n)-1)*p.K
}

// TestRedteamRepair_GarbageClaimIsSlashed (§11 a): a stripe position is really lost,
// and a caretaker claims a repair it did not do — the claim names the real position
// but a BOGUS shard id. The judge rejects it on the id the manifest commits at that
// position (a self-attributing fraud proof) and SLASHES the claimant. No bounty is
// paid.
//
// It differs from the positive control in exactly ONE input: the claimed id. Nothing
// is rebuilt here, because a garbage claimant rebuilt nothing.
//
// ⚠ WHICH CHECK IT REACHES, AND WHAT IT PINS — THEY ARE NOT THE SAME. Since the
// 2026-09-12 position screen this claim is slashed BEFORE a single survivor is
// fetched, so the check it REACHES is the screen and the recompute never runs here.
// What it PINS is the DISJUNCTION of the two: they are defence in depth over one
// quantity — claim.ShardID against the manifest — and neutering EITHER one alone
// still slashes, which is why the ablation that reddens this control has to make the
// judge TRUST claim.ShardID. The screen on its own is pinned properly, at zero
// survivor fetches and with an anti-vacuity witness, by
// TestRepairJudge_WrongIdAtAListedPositionIsSlashedBeforeAnyFetch.
func TestRedteamRepair_GarbageClaimIsSlashed(t *testing.T) {
	s := newRepairAdv(t, 42)
	s.fundEscrow(5_000_000)
	judge := s.careJudge()
	attacker := s.nodes[2]
	baseline := s.bond(attacker)

	pos, parityID, _ := s.parityTarget()
	s.loseStripePosition(t, s.nodes[3], parityID, pos)
	bogus := ports.HashBytes([]byte("not the real shard"))
	claim := repairproof.RepairClaim{
		Root: s.root, Stripe: 0, ShardPos: pos, ShardID: bogus, Holder: s.nodes[7].ID(),
	}
	s.deliverClaim(judge, attacker.ID(), claim)

	if judge.Stats.FalseRepairSlashes != 1 {
		t.Fatalf("garbage claim not slashed: FalseRepairSlashes=%d (the POSITION SCREEN in judgeRepairClaim must SLASH a listed position whose claimed id disagrees with the manifest-committed one; it fires before any survivor is fetched, so the correctness recompute never runs on this claim)", judge.Stats.FalseRepairSlashes)
	}
	if got := s.ledger.Reputation(attacker.ID()); got >= baseline {
		t.Fatalf("claimant standing not docked: %d >= %d", got, baseline)
	}
	if paid := s.ledger.EscrowPaid(s.root); paid != 0 {
		t.Fatalf("garbage claim drew a bounty: %d", paid)
	}
	if judge.Stats.BountiesReleased != 0 {
		t.Fatal("garbage claim released a bounty")
	}
}

// TestRedteamRepair_ComputeButDontStoreIsDenied (§11 c): a stripe position is really
// lost and the claimed shard id IS correct — the attacker really did reconstruct it
// from the survivors — but the NAMED holder is a liar that kept the proof + PoR tags
// and dropped the bytes. Its identity-bound retrievability challenge fails, so the
// bounty is DENIED — and, crucially, NOT slashed: a retrievability shortfall may be
// transient, and only the mathematically attributable correctness lie is ever
// punished. This also pins the anti-double-count property: retrievability binds to
// the NAMED holder, so "the correct bytes exist somewhere in the swarm" does not pay.
//
// It differs from the positive control in exactly ONE input: the holder's liar-ness.
// The loss is therefore still open at judgement time, which is the whole adversary —
// a claim that the position was restored when it was not.
//
// ⚠ THE LIAR IS WHY THE LOSS MUST BE PROVEN BEFORE IT IS STAGED ON. A liar answers
// MsgHasChunk from its proof metadata ("of course I have it"), so once the rebuilt
// shard is placed on it, shardIsReachable reports the position reachable again. The
// PoR challenge is the only thing that sees through that, and it is the judge's leg
// under test.
func TestRedteamRepair_ComputeButDontStoreIsDenied(t *testing.T) {
	s := newRepairAdv(t, 43)
	s.fundEscrow(5_000_000)
	judge := s.careJudge()
	attacker := s.nodes[2]
	baseline := s.bond(attacker)

	pos, parityID, leafIdx := s.parityTarget()
	liar := s.nodes[9]
	liar.SetLiar(true)
	s.loseStripePosition(t, s.nodes[3], parityID, pos)
	s.rebuildLostShard(t, attacker, liar, 0, pos, parityID, leafIdx)

	claim := repairproof.RepairClaim{
		Root: s.root, Stripe: 0, ShardPos: pos, ShardID: parityID, Holder: liar.ID(),
	}
	s.deliverClaim(judge, attacker.ID(), claim)

	if judge.Stats.BountiesReleased != 0 {
		t.Fatal("a data-less holder was paid — retrievability must bind to the named holder's OWN bytes")
	}
	if judge.Stats.FalseRepairSlashes != 0 {
		t.Fatal("a retrievability shortfall was slashed — only an attributable correctness lie is punished")
	}
	if paid := s.ledger.EscrowPaid(s.root); paid != 0 {
		t.Fatalf("bounty drawn for a data-less holder: %d", paid)
	}
	if got := s.ledger.Reputation(attacker.ID()); got != baseline {
		t.Fatalf("standing moved on a deny: %d != %d", got, baseline)
	}
}

// stageShardOn fetches the real shard from the swarm and re-places it on `holder`
// WITH a valid Merkle proof and PoR tags. An honest holder keeps the bytes (and can
// answer retrievability); a liar holder (SetLiar) keeps the receipt + tags but drops
// the bytes. The caller sets liar-ness.
//
// ⚠ IT STAGES A NO-LOSS ARRANGEMENT: the shard is live on its own providers the
// whole time and a COPY is moved. That is the arrangement
// TestRTRC3_ClaimWithNoLossIsPaid_PINNED_DEFECT exists to pin, and it is why the
// three §11 controls stopped using this helper on 2026-09-13 — see the loss
// arrangement banner below. Reach for loseStripePosition + rebuildLostShard
// instead unless a no-loss stage is the point of the test.
func (s *repairAdv) stageShardOn(fetcher, holder *Node, id ports.ChunkID, pos, leafIdx int) {
	// A coded shard registers under its COLUMN key, not its own id, so resolve the
	// column's providers and fetch the specific shard from them.
	fetcher.resolveProviders(colKey(s.root, pos), func(provs []ports.NodeID) {
		fetcher.fetchFrom(id, provs, func(bool) {})
	})
	s.sched.Run()
	c, err := fetcher.Store().Get(bg(), id)
	if err != nil {
		panic("could not fetch the shard to stage on the liar")
	}
	pr, err := manifest.Prove(s.m.Leaves(), leafIdx)
	if err != nil {
		panic(err)
	}
	proof := &ports.StorageProof{
		Root: s.root, Index: pr.Index, Total: pr.Total, Path: pr.Path, Column: pos,
		PorTags: s.porKey.Tags(id[:], c.Data),
	}
	s.nodes[0].placeAt(id, c.Data, proof, []ports.NodeID{holder.ID()}, 1, nil, func(int) {})
	s.sched.Run()
}

// ─────────────────────────────────────────────────────────────────────────────
// THE LOSS ARRANGEMENT — the stage the three §11 controls share.
//
// ROADMAP F8 / D-RTRC3-INTENT-STANDS-2026-09-12. Until 2026-09-13 all three
// controls stood on stageShardOn's NO-LOSS arrangement: fetch a shard that was
// never missing and re-place a live copy of it on a second holder. Under
// D-BOUNTY-PAYS-FOR-REPAIR-2026-09-12 a durability bounty pays for a REPAIR
// having happened, so that arrangement must pay NOTHING — which made the positive
// control an assertion that the defect was correct behaviour, and the two negative
// controls rest on that control's non-vacuity.
//
// The three now share ONE arrangement — a stripe position really destroyed across
// the whole swarm — and each negative control differs from the positive one in
// exactly ONE input: the claimed id (slash) or the holder's liar-ness (deny).
// Legs that can only fail together are one leg wearing two names, so keep that
// one-input discipline when editing any of the three.
//
// ⚠ THIS ASSERTS ONE HALF OF THE INTENT, AND ONLY ONE. The rule has two halves:
// a real repair IS paid, and a no-loss claim is NOT. The judge delivers the first
// today; the second is still broken, still pinned by
// TestRTRC3_ClaimWithNoLossIsPaid_PINNED_DEFECT, and its closer — the loss witness
// — is GATED behind R-PROBE-FALSE-NEGATIVE-RATE. A repair erases its own evidence
// (T-LOSS-IS-A-TRANSIENT), so the JUDGE still cannot see the loss these helpers
// stage; they stage it for the FIXTURE, not for the judge. Nothing in this file
// may be cited as "the bounty now pays for repair" or as grounds to retire the pin.
// ─────────────────────────────────────────────────────────────────────────────

// stripeRefsOf lists one stripe's STORED shard refs, derived from the fixture's own
// manifest with the arithmetic storedShards uses on a Layout. A short final stripe
// stores fewer than n — its implicit-zero positions are math, not storage.
func (s *repairAdv) stripeRefsOf(stripe int) []shardRef {
	p := erasure.Params{K: s.m.K, N: s.m.N}
	dataIDs, parityIDs := s.m.ChunkIDs(), s.m.ParityIDs()
	lo, hi := stripe*p.K, min((stripe+1)*p.K, len(dataIDs))
	var refs []shardRef
	for i, id := range dataIDs[lo:hi] {
		refs = append(refs, shardRef{id: id, stripe: stripe, pos: i, leafIdx: lo + i})
	}
	for q, id := range parityIDs[stripe*p.ParityShards() : (stripe+1)*p.ParityShards()] {
		refs = append(refs, shardRef{
			id: id, stripe: stripe, pos: p.K + q,
			leafIdx: len(dataIDs) + stripe*p.ParityShards() + q,
		})
	}
	return refs
}

// shardIsReachable asks the PRODUCTION probe whether the swarm still answers for a
// shard. probeShard confirms every provider with a HasChunk round-trip, so the stale
// provider record dropHosted deliberately leaves behind does NOT read as reachable.
// That distinction is the one R-RTRC3-PREMISE-CHECKS-A-RECORD says a resolveProviders
// check misses, and it is exactly what a loss premise has to get right.
func (s *repairAdv) shardIsReachable(from *Node, id ports.ChunkID, pos int) bool {
	reachable := false
	from.probeShard(id, colKey(s.root, pos), true, func(ok bool, _ map[uint64]bool) { reachable = ok })
	s.sched.Run()
	return reachable
}

// loseStripePosition destroys one stripe position across the WHOLE swarm: every node
// holding the bytes drops them, proof and PoR tags included.
//
// The loss is asserted as a DIFFERENTIAL — the probe answers YES before and NO after
// — because only the pair proves a loss. An "unreachable" reading alone is equally
// consistent with a probe that never worked, and a helper that silently stopped
// dropping anything would otherwise hand every caller a no-loss arrangement wearing
// a loss's name, which is the exact substitution this whole rework undoes.
//
// ⚠ THE FOUR t.Fatalf LINES BELOW ARE THE ENTIRE ENFORCEMENT OF THE LOSS. Reduce
// this body to t.Helper() and all three §11 controls still PASS — measured. That is
// not a fixture weakness, it is R-PROBE-FALSE-NEGATIVE-RATE showing through: nothing
// on judgeRepairClaim's path reads whether the claimed position was ever lost, so
// every judge-observable outcome (BountiesReleased, FalseRepairSlashes, EscrowPaid,
// Balance, Reputation) is identical with the loss and without it. The controls
// therefore cannot detect their own premise collapsing, and these guards are the only
// thing that can. Do not weaken one without reading
// TestRedteamRepair_ControlsStageTheLossAtTheirCallSites, which holds the other half
// — that the controls still CALL this helper.
func (s *repairAdv) loseStripePosition(t *testing.T, probe *Node, id ports.ChunkID, pos int) {
	t.Helper()
	if !s.shardIsReachable(probe, id, pos) {
		t.Fatalf("rig: stripe position %d was NOT reachable before the loss — nothing was lost, so a later unreachable reading proves nothing", pos)
	}
	held := 0
	for _, nd := range s.nodes {
		if ok, _ := nd.Store().Has(bg(), id); ok {
			nd.dropHosted(id)
			held++
		}
	}
	s.sched.Run()
	if held == 0 {
		t.Fatalf("rig: no node held stripe position %d, so no loss was staged", pos)
	}
	if s.shardIsReachable(probe, id, pos) {
		t.Fatalf("rig: stripe position %d is STILL reachable after %d holder(s) dropped it — this is not a loss arrangement", pos, held)
	}
}

// rebuildLostShard reconstructs a LOST stripe position from that stripe's SURVIVORS
// and places the rebuilt bytes on holder with a valid Merkle proof and PoR tags.
//
// Every step is the paramedic's own, in repairStripeFetch's order:
// fetchStripeByColumn, erasure.ReconstructStripe, the hash check against the
// manifest-committed id, then placeAt. What it leaves out is the placement roulette
// and the claim broadcast, so the caller names the holder and delivers the claim on
// the same deliverClaim path all three controls use.
//
// It mirrors repairStripeFetch's heldBefore discipline: the repairer keeps what it
// hosted before this repair and drops the copies it fetched for it, so the rebuilt
// position ends up on the holder and nowhere else.
func (s *repairAdv) rebuildLostShard(t *testing.T, repairer, holder *Node, stripe, pos int, id ports.ChunkID, leafIdx int) {
	t.Helper()
	p := erasure.Params{K: s.m.K, N: s.m.N}
	var survivors []shardRef
	realData := 0
	for _, r := range s.stripeRefsOf(stripe) {
		if r.pos < p.K {
			realData++
		}
		if r.pos != pos { // by POSITION, never by id — R-SHARDID-ALIASES-POSITION
			survivors = append(survivors, r)
		}
	}
	heldBefore := make(map[ports.ChunkID]bool, len(survivors))
	for _, r := range survivors {
		if ok, _ := repairer.Store().Has(bg(), r.id); ok {
			heldBefore[r.id] = true
		}
	}
	complete := false
	repairer.fetchStripeByColumn(s.root, survivors, func(unfetched []ports.ChunkID, _ map[uint64]int) {
		complete = len(unfetched) == 0
	})
	s.sched.Run()
	if !complete {
		t.Fatal("rig: the repairer could not fetch every surviving shard of the stripe")
	}
	shards := make([][]byte, p.N)
	for _, r := range survivors {
		if c, err := repairer.Store().Get(bg(), r.id); err == nil {
			shards[r.pos] = c.Data
		}
	}
	if shards[pos] != nil {
		t.Fatalf("rig: position %d arrived among the survivors — it was not lost, so nothing here is a rebuild", pos)
	}
	if err := erasure.ReconstructStripe(p, shards, realData); err != nil {
		t.Fatalf("rig: reconstruction from the survivors failed: %v", err)
	}
	if ports.HashBytes(shards[pos]) != id {
		t.Fatalf("rig: the bytes rebuilt for position %d do not hash to the id the manifest commits there", pos)
	}
	pr, err := manifest.Prove(s.m.Leaves(), leafIdx)
	if err != nil {
		t.Fatalf("rig: merkle proof for leaf %d: %v", leafIdx, err)
	}
	proof := &ports.StorageProof{
		Root: s.root, Index: pr.Index, Total: pr.Total, Path: pr.Path, Column: pos,
		PorTags: s.porKey.Tags(id[:], shards[pos]),
	}
	s.nodes[0].placeAt(id, shards[pos], proof, []ports.NodeID{holder.ID()}, 1, nil, func(int) {})
	s.sched.Run()
	for _, r := range survivors {
		if !heldBefore[r.id] {
			repairer.dropHosted(r.id)
		}
	}
}

// TestRedteamRepair_HonestClaimIsPaid is the positive control: a stripe position is
// really destroyed across the swarm, a paramedic rebuilds it from the surviving
// shards, and the SAME judge that slashes a garbage claim and denies a data-less one
// PAYS for that repair — the bounty flows to the holder, still moving no standing.
// Without this, the deny/slash tests could be passing by rejecting everything.
//
// ⚠ READ THE LOSS ARRANGEMENT BANNER ABOVE BEFORE CITING THIS TEST. It asserts one
// half of D-BOUNTY-PAYS-FOR-REPAIR-2026-09-12 — that a real repair is paid. The other
// half, that a no-loss claim pays NOTHING, is still broken and is what
// TestRTRC3_ClaimWithNoLossIsPaid_PINNED_DEFECT pins. This test does not move that pin.
func TestRedteamRepair_HonestClaimIsPaid(t *testing.T) {
	s := newRepairAdv(t, 44)
	s.fundEscrow(5_000_000)
	judge := s.careJudge()
	paramedic := s.nodes[2] // rebuilds the lost position, then claims for it

	pos, parityID, leafIdx := s.parityTarget()
	holder := s.nodes[9] // honest: keeps the rebuilt bytes
	holderStanding := s.bond(holder)
	holderBalance := s.ledger.Balance(holder.ID())
	s.loseStripePosition(t, s.nodes[3], parityID, pos)
	s.rebuildLostShard(t, paramedic, holder, 0, pos, parityID, leafIdx)

	claim := repairproof.RepairClaim{
		Root: s.root, Stripe: 0, ShardPos: pos, ShardID: parityID, Holder: holder.ID(),
	}
	s.deliverClaim(judge, paramedic.ID(), claim)

	if judge.Stats.BountiesReleased != 1 {
		t.Fatalf("an honest repair was not paid: BountiesReleased=%d", judge.Stats.BountiesReleased)
	}
	if judge.Stats.FalseRepairSlashes != 0 {
		t.Fatal("an honest repair was slashed")
	}
	if paid := s.ledger.EscrowPaid(s.root); paid <= 0 {
		t.Fatalf("EscrowPaid = %d, want > 0", paid)
	}
	if got := s.ledger.Balance(holder.ID()); got <= holderBalance {
		t.Fatalf("holder balance did not rise: %d <= %d", got, holderBalance)
	}
	// The position is whole again: a third party really pulls the bytes off the
	// named holder. Without this the payment assertion would be measuring the
	// judge's leniency rather than a repair.
	//
	// It fetches from the NAMED holder rather than re-running shardIsReachable,
	// and the difference is a fixture limitation worth stating. rebuildLostShard
	// places on the holder the CALLER names — the deny control needs that holder to
	// be a specific liar — whereas the paramedic places on IterativeFindNode's
	// candidates for colKey(root, pos). So the rebuilt shard sits outside its column
	// here and the column's stale provider records, left by the nodes that dropped
	// it, are all a DHT walk finds. The claim names its holder, so the judge's own
	// retrievability leg is unaffected; this arm asks the same question of the same
	// holder, over real bytes instead of a PoR proof.
	//
	// MEASURED, 2026-09-13, and it is state this fixture introduces: before the loss
	// the position sat on THREE nodes and shardIsReachable answered true; after the
	// rebuild it sits on ONE — the named holder — and shardIsReachable answers FALSE.
	// A loss witness shaped as a restoration differential would redden this control
	// for that reason alone, which is why rtRC3Pin's failure text names the fixture
	// before it names payment. The fix, when it is needed, is an announce in
	// rebuildLostShard, not a change to the judge.
	restored := false
	s.nodes[3].fetchFrom(parityID, []ports.NodeID{holder.ID()}, func(ok bool) { restored = ok })
	s.sched.Run()
	if !restored {
		t.Fatalf("stripe position %d could not be fetched from the paid holder — the bounty paid for nothing restored", pos)
	}
	if got := s.ledger.Reputation(holder.ID()); got != holderStanding {
		t.Fatalf("holder standing moved from %d to %d on a paid bounty", holderStanding, got)
	}
}

// TestRedteamRepair_QuorumIsDiscoverableAcrossNodes (§11 d, availability side): the
// quorum a repairer broadcasts to is discovered through the careKey rendezvous, and
// EVERY caretaker that announces there is found — so honest caretakers can't be
// silently excluded from the vote by a repairer that only knows nodes near itself.
// (Full domain-DIVERSE quorum SELECTION — refusing to let one failure domain pack
// the vote — is future hardening; today the guarantee is that all announcers are
// reachable.) Uses real Care(), so it advances the clock a bounded amount rather
// than running to quiescence (the repair loop reschedules forever).
func TestRedteamRepair_QuorumIsDiscoverableAcrossNodes(t *testing.T) {
	s := newRepairAdv(t, 45)
	caretakers := []*Node{s.nodes[3], s.nodes[5], s.nodes[7]}
	want := map[ports.NodeID]bool{}
	for _, c := range caretakers {
		c.Care(s.reg, s.h.Care()) // real Care → announces under careKey
		want[c.ID()] = true
	}
	s.sched.RunUntil(s.sched.Now().Add(5 * ports.Second))

	// A repairer resolves the careKey rendezvous and must find every announcer.
	found := map[ports.NodeID]bool{}
	s.nodes[0].resolveProviders(ports.ChunkID(careKey(s.root)), func(provs []ports.NodeID) {
		for _, p := range provs {
			found[p] = true
		}
	})
	s.sched.RunUntil(s.sched.Now().Add(5 * ports.Second))

	for id := range want {
		if !found[id] {
			t.Fatalf("caretaker %s announced under careKey but was not discovered — the quorum would silently exclude it", id.String()[:8])
		}
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// THE CALL-SITE GATE — the three §11 controls must STAGE the loss, not merely be
// documented as staging it.
//
// Swap a control's loseStripePosition back to stageShardOn and every test in the tree
// stays GREEN while the ROADMAP F8 row silently becomes false. Nothing else in the
// repo notices: the judge has no loss check, so a control cannot observe its own
// premise being removed. That is the repo's proof-vs-structure rule with a known
// substitution attached, and this is the structure.
//
// ⚠ WHAT IT PROVES, AND WHAT IT DOES NOT. A source gate proves the call is WRITTEN.
// It cannot prove the call RUNS, and it cannot prove the helper still asserts anything
// — reduce loseStripePosition's body to t.Helper() and this gate stays GREEN, as do
// all three controls. That half is held by the four in-line t.Fatalf guards inside the
// helper, and is stated in the helper's own doc comment. Neither half implies the
// other; keep both.
//
// THE RULE HAS A CLOSED COMPLEMENT, which is why it is worth having. In THIS file a
// test that hands the judge a repair claim (deliverClaim) is a §11 control and there
// is no other kind, so the rule is "every deliverClaim test also calls
// loseStripePosition" rather than a hand-written list of three that a fourth control
// would quietly escape.
//
// RUNTIME GATE: TestRedteamRepair_HonestClaimIsPaid (and the two negative controls).
// They are the tests that EXECUTE loseStripePosition's four t.Fatalf guards, which is
// the half this gate cannot see. This annotation follows scripts/check_source_gates.py's
// rule — a source-text gate says it is one and names its runtime cover — even though
// that lint's trigger is an os.ReadFile of a literal .go path and does not reach a
// parser.ParseFile gate like this one.
//
// IT IS FILE-SCOPED ON PURPOSE. The no-loss arrangement is the entire point of
// TestRTRC3_ClaimWithNoLossIsPaid_PINNED_DEFECT and of the judge fixtures in
// rt_repairclaim_gates_test.go, repairclaim_judge_fixes_test.go and
// judge_defer_518_test.go. They must keep staging it, and a repo-wide form of this
// rule would demand they stop.
// ─────────────────────────────────────────────────────────────────────────────

func TestRedteamRepair_ControlsStageTheLossAtTheirCallSites(t *testing.T) {
	const file = "redteam_repair_claim_test.go"
	const lossHelper, noLossHelper, claimCall = "loseStripePosition", "stageShardOn", "deliverClaim"

	f, err := parser.ParseFile(token.NewFileSet(), file, nil, 0)
	if err != nil {
		t.Fatalf("SOURCE GATE: could not parse %s: %v", file, err)
	}
	callees := func(fn *ast.FuncDecl) map[string]bool {
		got := map[string]bool{}
		ast.Inspect(fn, func(n ast.Node) bool {
			if c, ok := n.(*ast.CallExpr); ok {
				if sel, ok := c.Fun.(*ast.SelectorExpr); ok {
					got[sel.Sel.Name] = true
				}
			}
			return true
		})
		return got
	}

	declared := map[string]bool{}
	var controls, offenders []string
	for _, d := range f.Decls {
		fn, ok := d.(*ast.FuncDecl)
		if !ok {
			continue
		}
		declared[fn.Name.Name] = true
		if !strings.HasPrefix(fn.Name.Name, "Test") {
			continue
		}
		c := callees(fn)
		if !c[claimCall] {
			continue // not a repair-claim control
		}
		controls = append(controls, fn.Name.Name)
		if !c[lossHelper] {
			offenders = append(offenders, "  "+fn.Name.Name)
		}
	}

	// ANCHOR 1 — the helpers still exist under these names. A rename would turn the
	// walk into a green no-op over a mechanism that moved.
	for _, h := range []string{lossHelper, noLossHelper, claimCall} {
		if !declared[h] {
			t.Fatalf("SOURCE GATE IS VACUOUS — the symbol it reads is gone: %s is no longer declared in %s, so it "+
				"was renamed or removed. Re-derive this gate rather than leaving it green over nothing", h, file)
		}
	}
	// ANCHOR 2 — the walk actually found controls. Without a floor, "the walk broke"
	// and "every control is clean" are the same green.
	if len(controls) < 3 {
		t.Fatalf("SOURCE GATE IS VACUOUS — the walk found nothing to check: %d test(s) deliver a repair claim "+
			"in %s (%v). The three §11 controls are the floor, so either the walk broke or a control left "+
			"this file", len(controls), file, controls)
	}

	if len(offenders) > 0 {
		sort.Strings(offenders)
		t.Fatalf("SOURCE GATE: a §11 REPAIR-CLAIM CONTROL NO LONGER CALLS THE LOSS HELPER:\n%s\n\n"+
			"  Each of these delivers a repair claim to the judge without calling %s, so it is judging a\n"+
			"  claim for a position that was never missing. Under D-BOUNTY-PAYS-FOR-REPAIR-2026-09-12 that\n"+
			"  arrangement must pay NOTHING, and the shipped judge pays it — which is the defect\n"+
			"  TestRTRC3_ClaimWithNoLossIsPaid_PINNED_DEFECT pins, not a control.\n"+
			"  NOTHING ELSE CATCHES THIS. The judge has no loss check (R-PROBE-FALSE-NEGATIVE-RATE is still\n"+
			"  GATED), so the control itself stays GREEN either way and ROADMAP F8 silently becomes false.\n"+
			"  If a no-loss stage is genuinely the point of a new test, put it in rt_repairclaim_gates_test.go\n"+
			"  beside RT-RC-3, where that arrangement is the subject rather than the premise.",
			strings.Join(offenders, "\n"), lossHelper)
	}
	sort.Strings(controls)
	t.Logf("SOURCE GATE (call site): %d repair-claim control(s) in %s, all calling %s — %v",
		len(controls), file, lossHelper, controls)
}
