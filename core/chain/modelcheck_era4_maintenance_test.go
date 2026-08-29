package chain

import (
	"crypto/ed25519"
	"reflect"
	"testing"

	"github.com/nerolabs/silt/ports"
)

// Consensus model-check — era-4 (v5) step 4b: the MAINTENANCE SPINE drift guards.
//
// The certified design (docs/thinking/2026-08-29-era4-witnessable-transitions-options.md,
// RECERT2 era4-witnessable-transitions-RECERT2-2026-08-29.md) makes two build-time
// obligations load-bearing, each ablated RED before it is trusted (the session-7 rule:
// a green check with no demonstrated red is a comment that compiles):
//
//   1. qualified == filter(bonded, slashed, MinBond) after EVERY block — the E-2
//      maintenance invariant. The guard ablates PER SITE; dropping the 3007
//      displacement hook (delete(qualified, owner)) MUST redden specifically (RECERT2 R1).
//   2. bucket-membership(id) ⟺ (bondRegHeight[id] + ttl + 1 == D AND bonded[id] present)
//      — the T-3 dual-source invariant. Drops on a missed renew old-bucket delete.
//
// Plus the byte-identical post-apply StateRoot replay vs an era-3-shape recompute, and
// the rotate-LAST stale-capture ordering ablation.
//
// These run against v5 root computations the test constructs; 4b does not activate v5,
// so no PRODUCED block is v5. The point is maintenance correctness BEFORE the 4c
// predicate makes the maps load-bearing.

// era4MinBond is the qualification floor these fixtures use.
const era4MinBond = int64(1) << 20

// recomputeQualified is the RECOMPUTE side of the E-2 invariant: filter(bonded,
// slashed, MinBond), independent of the incrementally-maintained qualified map. The
// guard asserts the maintained map equals this recompute after every block.
func recomputeQualified(c *Chain) map[ports.NodeID]int64 {
	out := make(map[ports.NodeID]int64)
	for id, sz := range c.bonded {
		if sz >= c.cfg.MinBond && !c.slashed[id] {
			out[id] = sz
		}
	}
	return out
}

// recomputeDueBucket is the RECOMPUTE side of the T-3 dual-source invariant. The
// AUTHORITY the era-4 TTL sweep still iterates is bondRegHeight (era-4 kept the era-3
// sweep over bondRegHeight and merely ADDED the bucket delete alongside it), so the
// bucket must mirror bondRegHeight, NOT bonded. This matters at a displacement: era-3
// deletes bonded[owner] but LEAVES bondRegHeight[owner], so the era-3 sweep still
// processes owner at its due height (a harmless no-op bonded delete). The bucket must
// keep owner until then, or the era-4 sweep would leave bondRegHeight[owner] unswept —
// a divergence era-3 cannot have. Only meaningful with TTL enabled.
func recomputeDueBucket(c *Chain) map[uint64]map[ports.NodeID]struct{} {
	out := make(map[uint64]map[ports.NodeID]struct{})
	ttl := c.cfg.BondTTLBlocks
	if ttl == 0 {
		return out
	}
	for id, regH := range c.bondRegHeight {
		d := regH + ttl + 1
		if out[d] == nil {
			out[d] = make(map[ports.NodeID]struct{})
		}
		out[d][id] = struct{}{}
	}
	return out
}

// era4Corpus builds a chain and applies a branch-covering block stream that exercises
// displacement (site 3007), fresh/renew/resize (3013), TTL expiry (3026), and slash
// (3037/3038) — several in the SAME block — with TTL enabled so the due-bucket is live.
// It returns the chain and asserts nothing; callers assert the invariants after each
// block via applyAndCheck.
func era4Corpus(t *testing.T, check func(t *testing.T, c *Chain, label string)) *Chain {
	t.Helper()
	// Two validators that renew, one that gets slashed, one squatter that is displaced.
	a, b, cc, sq := key(84001), key(84002), key(84003), key(84004)
	honest := key(84005) // proves the displacer's real root

	cfg := Config{Quorum: 1, MinBond: era4MinBond, ByzantineQuorum: true,
		EpochBlocks: 2, MatureValidators: 2, BondTTLBlocks: 4}
	c := New(cfg, func(ports.NodeID) int64 { return 0 })
	c.SetBondVerifier(objectiveVerify)

	apply := func(blk *Block, label string) {
		t.Helper()
		c.apply(*blk)
		check(t, c, label)
	}

	// Genesis: a, b, cc bonded; sq squats honest's real root (unproven genesis claim).
	honestRoot := ports.HashBytes(pubOf(honest))
	g := &Block{Version: 1, Height: 0, Entries: []ports.Entry{entry(0)}}
	g.BondRegs = []BondReg{
		bondReg(a, 4<<20, ports.Hash{}),
		bondReg(b, 4<<20, ports.Hash{}),
		bondReg(cc, 4<<20, ports.Hash{}),
		// sq declares honest's root at genesis (unproven), taking bonded standing.
		{Validator: pubOf(sq), Root: honestRoot, Size: 4 << 20, Answer: []byte("valid")},
	}
	Sign(g, a)
	apply(g, "genesis")

	// h1: honest PROVES honestRoot → displaces sq (site 3007 delete(qualified, owner)).
	prev := g.Hash()
	b1 := &Block{Version: 1, Height: 1, Prev: prev, Entries: []ports.Entry{entry(1)},
		BondRegs: []BondReg{bondRegFull(honest, honestRoot, 4<<20, prev, 3, 7)}}
	Sign(b1, a)
	apply(b1, "h1 displacement")

	// h2: a RENEWS (moves its due-bucket 3013+move), and b RESIZES up — both in one
	// block. This is the renew old-bucket-delete path.
	prev = b1.Hash()
	b2 := &Block{Version: 1, Height: 2, Prev: prev, Entries: []ports.Entry{entry(2)},
		BondRegs: []BondReg{
			bondReg(a, 4<<20, prev), // renew a (same size)
			bondReg(b, 8<<20, prev), // resize b up
		}}
	Sign(b2, a)
	apply(b2, "h2 renew+resize")

	// h3: SLASH cc (sites 3037/3038 delete(qualified, culprit)) AND fresh-register a
	// brand-new id in the SAME block, so slash and register interleave.
	prev = b2.Hash()
	fresh := key(84006)
	b3 := &Block{Version: 1, Height: 3, Prev: prev, Entries: []ports.Entry{entry(3)},
		BondRegs: []BondReg{bondReg(fresh, 4<<20, prev)},
		Slashes:  []Equivocation{slashProof(cc, prev, 0x51, 0x52)}}
	Sign(b3, a)
	apply(b3, "h3 slash+fresh")

	// h5, h6: let a's ORIGINAL genesis registration lapse — a renewed at h2, so its due
	// height is 2+4+1 = 7. b renewed at h2 (due 7). honest reg'd at h1 (due 6). fresh
	// reg'd at h3 (due 8). Drive to h6 so honest's bond (due 6) TTL-expires at h6 (site
	// 3026 delete + due-bucket delete). h6-1 = 5 > regH(1)+... the sweep fires when
	// height-regH > ttl, i.e. 6-1=5 > 4.
	for h := uint64(4); h <= 6; h++ {
		prev = c.blocks[len(c.blocks)-1].Hash()
		bh := &Block{Version: 1, Height: h, Prev: prev, Entries: []ports.Entry{entry(byte(h))}}
		Sign(bh, a)
		apply(bh, "h"+string(rune('0'+h))+" ttl-drive")
	}
	return c
}

// TestQualifiedMaintenanceDriftGuard is the E-2 invariant: qualified ==
// filter(bonded, slashed, MinBond) after every block, over a corpus exercising
// displacement/renew/slash/expiry (several per block). The maintenance hooks are the
// production code under test; this asserts they keep the materialized map in agreement
// with the recompute.
//
// RED (per-site ablation, demonstrated in the 4b report): drop the maintenance at any
// of the five sites and this reddens. RECERT2 R1 requires it to redden on the 3007
// displacement hook SPECIFICALLY — because 3007 deletes the displaced `owner` (a
// DIFFERENT key from the `id` written at 3013), a mirror that only follows 3013 leaves
// a stale qualified entry for the displaced owner.
func TestQualifiedMaintenanceDriftGuard(t *testing.T) {
	var sawDisplacement bool
	era4Corpus(t, func(t *testing.T, c *Chain, label string) {
		want := recomputeQualified(c)
		if !reflect.DeepEqual(c.qualified, want) {
			t.Fatalf("[%s] qualified DRIFT: maintained=%v want(filter)=%v", label, c.qualified, want)
		}
		if label == "h1 displacement" {
			sawDisplacement = true
		}
	})
	if !sawDisplacement {
		t.Fatal("corpus never exercised the displacement block — the 3007 ablation would be vacuous")
	}
}

// TestDueBucketDualSourceDriftGuard is the T-3 dual-source invariant:
// bucket-membership(id) ⟺ (bondRegHeight[id]+ttl+1 == D AND bonded[id] present),
// after every block, over the same corpus (TTL enabled). era-4 keeps BOTH bondRegHeight
// and the due-bucket; a drift between them is a divergence era-3 (one source) cannot
// have.
//
// RED (demonstrated in the 4b report): drop the OLD-bucket delete on renew
// (dueBucketMoveOnReg) and the maintained bucket names a due-height that no longer
// matches bondRegHeight[id]+ttl+1 — this reddens.
func TestDueBucketDualSourceDriftGuard(t *testing.T) {
	era4Corpus(t, func(t *testing.T, c *Chain, label string) {
		want := recomputeDueBucket(c)
		if !reflect.DeepEqual(c.dueBucket, want) {
			t.Fatalf("[%s] dueBucket DUAL-SOURCE DRIFT: maintained=%v want(from bondRegHeight)=%v",
				label, c.dueBucket, want)
		}
	})
}

// TestV5PostApplyRootByteIdenticalAcrossOrderings is the byte-identical post-apply
// StateRoot replay: two block streams that reach the same final committedSet (including
// the era-4 maintenance-spine maps) must produce byte-identical v5 StateRoots. It lifts
// the per-field drift guards to the committed ROOT — a maintenance bug that changed a
// committed leaf would diverge the root here.
//
// The two orderings differ in the intra-block SLICE ORDER of a renew+resize block; the
// canonical marshaller (order-free qualified leaves, canonical-MTH buckets) must produce
// the identical root regardless.
func TestV5PostApplyRootByteIdenticalAcrossOrderings(t *testing.T) {
	build := func(renewFirst bool) *Chain {
		a, b := key(85001), key(85002)
		cfg := Config{Quorum: 1, MinBond: era4MinBond, ByzantineQuorum: true,
			EpochBlocks: 2, BondTTLBlocks: 4}
		c := New(cfg, func(ports.NodeID) int64 { return 0 })
		c.SetBondVerifier(objectiveVerify)

		g := &Block{Version: 1, Height: 0, Entries: []ports.Entry{entry(0)}}
		g.BondRegs = []BondReg{bondReg(a, 4<<20, ports.Hash{}), bondReg(b, 4<<20, ports.Hash{})}
		Sign(g, a)
		c.apply(*g)

		prev := g.Hash()
		ra := bondReg(a, 4<<20, prev) // renew a
		rb := bondReg(b, 8<<20, prev) // resize b
		regs := []BondReg{ra, rb}
		if renewFirst {
			regs = []BondReg{rb, ra}
		}
		b1 := &Block{Version: 1, Height: 1, Prev: prev, Entries: []ports.Entry{entry(1)}, BondRegs: regs}
		Sign(b1, a)
		c.apply(*b1)
		return c
	}
	x, y := build(false), build(true)
	rx, err := x.StateRootForVersion(BlockVersionWitnessable)
	if err != nil {
		t.Fatalf("v5 root x: %v", err)
	}
	ry, err := y.StateRootForVersion(BlockVersionWitnessable)
	if err != nil {
		t.Fatalf("v5 root y: %v", err)
	}
	if rx != ry {
		t.Fatalf("v5 post-apply StateRoot differs across intra-block orderings: %x != %x — "+
			"a maintenance or encoding bug made a committed leaf order-dependent", rx, ry)
	}
}

// TestBoundaryCopyStaleCaptureOrderingAblation is the rotate-LAST stale-capture
// ordering ablation (the sharpest). The boundary freeze (epochSet := qualified) runs
// LAST, AFTER this block's bonds/TTL/slashes have maintained qualified. If a boundary
// block ALSO slashes a member, the frozen epochSet must EXCLUDE the slashed member —
// which holds only because the slash maintenance (site 3037) ran BEFORE the rotate-LAST
// copy. Freezing from a PRE-maintenance snapshot of qualified would re-admit the slashed
// member — an I3 mid-epoch-churn divergence.
//
// This asserts the correct ordering (frozen set excludes the slashed member); the report
// shows the RED when the boundary copy is sourced from a stale (pre-maintenance) set.
func TestBoundaryCopyStaleCaptureOrderingAblation(t *testing.T) {
	a, b, cc := key(86001), key(86002), key(86003)
	// MatureValidators=0 hands maturity off at the genesis boundary (the #535 test's
	// pattern), so rotateEpoch freezes a real set from the first boundary — the fixture
	// does not have to drive the full maturity latch.
	cfg := Config{Quorum: 1, MinBond: era4MinBond, ByzantineQuorum: true,
		MatureValidators: 0, EpochBlocks: 2}
	c := New(cfg, func(ports.NodeID) int64 { return 0 })
	c.SetBondVerifier(objectiveVerify)

	// Genesis seats a, b, cc and hands off maturity (boundary h0).
	g := &Block{Version: 1, Height: 0, Entries: []ports.Entry{entry(0)}}
	g.BondRegs = []BondReg{
		bondReg(a, 4<<20, ports.Hash{}), bondReg(b, 4<<20, ports.Hash{}), bondReg(cc, 4<<20, ports.Hash{})}
	Sign(g, a)
	c.apply(*g)

	// h1: an ordinary mid-epoch block.
	prev := g.Hash()
	b1 := &Block{Version: 1, Height: 1, Prev: prev, Entries: []ports.Entry{entry(1)}}
	Sign(b1, a)
	c.apply(*b1)

	// h2: the BOUNDARY block ALSO slashes cc. rotate-LAST must see the post-slash
	// qualified (cc gone) and freeze {a, b} only.
	prev = b1.Hash()
	b2 := &Block{Version: 1, Height: 2, Prev: prev, Entries: []ports.Entry{entry(2)},
		Slashes: []Equivocation{slashProof(cc, prev, 0x61, 0x62)}}
	Sign(b2, a)
	c.apply(*b2)

	if !c.matureEpoch {
		t.Fatal("fixture did not mature — the boundary freeze never ran, the ablation is vacuous")
	}
	if _, in := c.epochSet[idOf(cc)]; in {
		t.Fatal("STALE CAPTURE: the boundary froze cc into epochSet DESPITE the same block's slash — " +
			"rotate-LAST read qualified BEFORE the slash maintenance (I3 mid-epoch-churn divergence)")
	}
	if _, in := c.epochSet[idOf(a)]; !in {
		t.Fatal("the boundary must freeze the surviving members (a) into epochSet")
	}
	// The frozen set must equal the post-apply recompute (era-3 equivalence at the boundary).
	if !reflect.DeepEqual(c.epochSet, recomputeQualified(c)) {
		t.Fatalf("boundary epochSet %v != post-apply filter %v — the freeze source is not this block's post-apply set",
			c.epochSet, recomputeQualified(c))
	}
}

// TestQ5RecoveryBranchAgreement is the Q5 coupling (RECERT2): at the #535 recovery
// boundary, the materialized qualified and the recomputed liveQualifiedSet() are the
// two producers of the recovery set and MUST agree (both are filter(bonded, slashed,
// MinBond)). era-4 freezes the recovery boundary from liveQualifiedSet() explicitly, so
// the frozen set re-bases against the live set the operator recovered to — never the
// stale accelerator. This asserts the two producers agree over the maintenance corpus.
//
// RED (demonstrated in the 4b report): inject a qualified drift, hit the recovery
// boundary, and the two producers disagree.
func TestQ5RecoveryBranchAgreement(t *testing.T) {
	era4Corpus(t, func(t *testing.T, c *Chain, label string) {
		// At any height, the two producers of the recovery re-base must agree.
		if !reflect.DeepEqual(c.qualified, c.liveQualifiedSet()) {
			t.Fatalf("[%s] Q5: materialized qualified %v != recomputed liveQualifiedSet() %v — "+
				"the recovery re-base and the boundary accelerator would disagree", label, c.qualified, c.liveQualifiedSet())
		}
	})
}

// TestEra3ReplayByteIdenticalOverCorpus is the byte-identical post-apply StateRoot
// replay vs an era-3-shape recompute (RECERT2 owed obligation), over a corpus covering
// renew-reset, ttl==0, and slash-before-due. It asserts the era-4 maintenance spine does
// NOT perturb the era-3 (v4) committed root: after every block, the v4 root over the
// era-4-maintained chain equals the v4 root over a chain whose new keyspaces are cleared.
// Because the v4 marshaller (StateRoot / StateRootForVersion(v4)) ignores the new maps
// by construction (hazard-1 v5-gating), this holds block-by-block over ANY history — the
// spine is inert on the era-3 root.
//
// RED (the hazard-1 ablation, demonstrated in the 4b report): route the v4 path through
// the v5 marshaller and this reddens on the first block that populates a new keyspace.
func TestEra3ReplayByteIdenticalOverCorpus(t *testing.T) {
	run := func(t *testing.T, ttl uint64) {
		a, b, cc := key(87001), key(87002), key(87003)
		cfg := Config{Quorum: 1, MinBond: era4MinBond, ByzantineQuorum: true,
			EpochBlocks: 2, BondTTLBlocks: ttl}
		c := New(cfg, func(ports.NodeID) int64 { return 0 })
		c.SetBondVerifier(objectiveVerify)

		checkV4Stable := func(label string) {
			t.Helper()
			// The v4 root the era-4-maintained chain commits.
			withSpine, err := c.StateRootForVersion(BlockVersionStateRoot)
			if err != nil {
				t.Fatalf("[%s ttl=%d] v4 root: %v", label, ttl, err)
			}
			// The v4 root a chain with EMPTY new keyspaces commits — the era-3 shape.
			savedQ, savedD, savedE := c.qualified, c.dueBucket, c.epochStart
			c.qualified = map[ports.NodeID]int64{}
			c.dueBucket = map[uint64]map[ports.NodeID]struct{}{}
			c.epochStart = 0
			era3Shape, err := c.StateRootForVersion(BlockVersionStateRoot)
			c.qualified, c.dueBucket, c.epochStart = savedQ, savedD, savedE
			if err != nil {
				t.Fatalf("[%s ttl=%d] era-3-shape v4 root: %v", label, ttl, err)
			}
			if withSpine != era3Shape {
				t.Fatalf("[%s ttl=%d] era-4 maintenance PERTURBED the v4 root: %x != %x — "+
					"the new keyspaces leaked into the era-3 root (hazard-1)", label, ttl, withSpine, era3Shape)
			}
		}

		g := &Block{Version: 1, Height: 0, Entries: []ports.Entry{entry(0)}}
		g.BondRegs = []BondReg{
			bondReg(a, 4<<20, ports.Hash{}), bondReg(b, 4<<20, ports.Hash{}), bondReg(cc, 4<<20, ports.Hash{})}
		Sign(g, a)
		c.apply(*g)
		checkV4Stable("genesis")

		// renew-reset: a renews (resets its TTL clock / moves its bucket).
		prev := g.Hash()
		b1 := &Block{Version: 1, Height: 1, Prev: prev, Entries: []ports.Entry{entry(1)},
			BondRegs: []BondReg{bondReg(a, 4<<20, prev)}}
		Sign(b1, a)
		c.apply(*b1)
		checkV4Stable("renew-reset")

		// slash-before-due: slash cc while it is still within its TTL window.
		prev = b1.Hash()
		b2 := &Block{Version: 1, Height: 2, Prev: prev, Entries: []ports.Entry{entry(2)},
			Slashes: []Equivocation{slashProof(cc, prev, 0x71, 0x72)}}
		Sign(b2, a)
		c.apply(*b2)
		checkV4Stable("slash-before-due")

		// Drive a few more heights (TTL expiry fires here when ttl>0).
		for h := uint64(3); h <= 7; h++ {
			prev = c.blocks[len(c.blocks)-1].Hash()
			bh := &Block{Version: 1, Height: h, Prev: prev, Entries: []ports.Entry{entry(byte(h))}}
			Sign(bh, a)
			c.apply(*bh)
			checkV4Stable("drive")
		}
	}
	t.Run("ttl-enabled", func(t *testing.T) { run(t, 4) })
	t.Run("ttl-disabled", func(t *testing.T) { run(t, 0) })
}

// TestCloneDueBucketDeepCopiesInnerMaps closes the gap the generic clone guard
// (TestDryRunCloneCopiesEveryAppliedField) leaves: it checks only TOP-LEVEL map
// distinctness, so a dueBucket whose OUTER map is fresh but whose INNER id-sets alias
// the source would pass it — yet a dry-run apply() that inserts/removes a bucket id
// would write THROUGH the shared inner map into live state (the #558 class, one level
// deeper). This asserts each inner bucket is a distinct object.
func TestCloneDueBucketDeepCopiesInnerMaps(t *testing.T) {
	src := &Chain{}
	populateCommitted(src)
	src.cfg = DefaultConfig()
	src.dueBucket = map[uint64]map[ports.NodeID]struct{}{
		43: {ports.NodeID{1}: {}, ports.NodeID{2}: {}},
		44: {ports.NodeID{3}: {}},
	}

	clone := src.cloneForDryRun()

	// Mutating the clone's inner bucket must NOT touch the source's.
	clone.dueBucket[43][ports.NodeID{9}] = struct{}{}
	if _, leaked := src.dueBucket[43][ports.NodeID{9}]; leaked {
		t.Fatal("cloneForDryRun shares the INNER dueBucket map — a dry-run apply() would corrupt live state")
	}
	delete(clone.dueBucket[44], ports.NodeID{3})
	if _, gone := src.dueBucket[44][ports.NodeID{3}]; !gone {
		t.Fatal("deleting from the clone's inner bucket also deleted from the source — shared inner map")
	}
}

// ---- compile-time reference so an unused import is caught if helpers change ----
var _ = ed25519.PublicKeySize
