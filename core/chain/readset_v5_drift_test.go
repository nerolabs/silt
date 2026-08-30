package chain

import (
	"crypto/ed25519"
	"encoding/binary"
	"sort"
	"testing"

	"github.com/nerolabs/silt/core/statehash"
	"github.com/nerolabs/silt/ports"
)

// R3 drift guard — the load-bearing defense for the v5 witness read-set producer
// (lane-1 Part A, cert era4-witness-floor-box-readset-v5-RESEARCH-CERTIFICATION-
// 2026-08-30, residual R3).
//
// The certified obligation: the producer's enumerated read-set MUST equal the keys
// the v5 witnessable recompute actually reads, and a future refactor that desyncs the
// two MUST re-redden. This guard is the #654-vacuity-guard-class defense: two
// INDEPENDENT enumerations of the same set, asserted equal over a branch-covering
// corpus, ablated RED on a dropped key.
//
// The two independent sides:
//   - THE PRODUCER (readset_v5.go, WitnessReadSetV5): walks the block's TRANSITIONS
//     (per-entry, per-reg, per-slash) and emits the keys each reads.
//   - THE RECOMPUTE-READS enumerator (recomputeWitnessReadsV5, below): walks the
//     CERTIFIED IDENTITY grouped BY FIELD (all byRoot reads, then all spent reads,
//     then the accelerator reads), a DISTINCT code path over the same block + state.
//
// A copy-paste bug in one side does not hide in the other (they are structured
// differently), so their set-equality is a real cross-check, not a tautology. This is
// the same dual-source discipline the RECERT2 maintenance drift guards use
// (recomputeQualified / recomputeDueBucket in modelcheck_era4_maintenance_test.go).
//
// THE ABLATION (cert R3, the session-7 "green while covering nothing" scar): the
// corpus MUST cover the empty-dueBucket[h] non-membership path (the whole era-4 win)
// AND drop a dueBucket key + a qualified/epochSet delta key must redden. The ablation
// subtests below inject exactly those divergences and assert the guard catches them.

// keySet is the comparable projection of a read-set: the sorted set of (tag||rawKey,
// kind) pairs. Two read-sets are equal iff their keySets are equal. The value is NOT
// compared here — the read-set IDENTITY is the (key, kind) pairs (which committed keys
// the box must witness, and whether present/absent); the VALUE is a downstream shape
// concern the producer fills from committed state and the recompute recomputes. The
// guard's job is the KEYSET completeness the cert names.
func keySet(rs []statehash.ReadEntry) []string {
	out := make([]string, 0, len(rs))
	for _, e := range rs {
		out = append(out, string(e.Key)+"|"+kindStr(e.Kind))
	}
	sort.Strings(out)
	return out
}

func kindStr(k statehash.QueryKind) string {
	if k == statehash.QueryPresent {
		return "present"
	}
	return "absent"
}

// recomputeWitnessReadsV5 is the INDEPENDENT recompute-reads enumeration: the certified
// read-set identity, grouped by field, computed over the pre-apply committed state. It
// is deliberately a DIFFERENT code path from WitnessReadSetV5 (grouped by field, not by
// transition) so their set-equality is a genuine cross-check. It targets the BOUNDED
// WITNESSABLE RECOMPUTE — it NEVER ranges bondRegHeight (the O(registry) hazard); the
// TTL completeness is the single dueBucket[h] leaf, exactly as the producer emits.
func (c *Chain) recomputeWitnessReadsV5(b Block) []statehash.ReadEntry {
	if b.Version < BlockVersionWitnessable {
		return nil
	}
	acc := newReadSetAcc()

	// --- byRoot: publish dup-root (absent) ∪ revoke existence (present) ---
	for i := range b.Entries {
		acc.addAbsent(tagByRoot, b.Entries[i].Root[:])
	}
	for _, r := range b.Revocations {
		acc.addPresent(tagByRoot, r[:], statehash.Present)
	}
	// --- spent: publish double-spend (absent) when a token rides ---
	for i := range b.Entries {
		if t := b.Entries[i].Token; t != nil {
			acc.addAbsent(tagSpent, []byte(t.Serial))
		}
	}
	// --- revoked: un-revoke existence (present) ---
	for _, r := range b.Unrevocations {
		acc.addPresent(tagRevoked, r[:], statehash.Present)
	}

	// --- the bond-reg reads, grouped by field over the canonical reg set ---
	regs := canonicalBondRegs(b.BondRegs)
	touched := make(map[ports.NodeID]struct{}) // ids whose bonded/qualified the recompute reads
	for _, r := range regs {
		if len(r.Validator) != ed25519.PublicKeySize || r.Size < c.cfg.MinBondBytes {
			continue
		}
		id := r.ValidatorID()
		// slashed gate.
		c.recEmitSlashed(acc, id)
		if c.slashed[id] {
			continue
		}
		// bondRootOwner / bondRootProven (displacement branch).
		if owner, claimed := c.bondRootOwner[r.Root]; claimed {
			acc.addPresent(tagBondRootOwner, r.Root[:], statehash.EncodeID(owner))
			c.recEmitBondRootProven(acc, r.Root)
			if owner != id {
				touched[owner] = struct{}{}
			}
		} else {
			acc.addAbsent(tagBondRootOwner, r.Root[:])
			acc.addAbsent(tagBondRootProven, r.Root[:])
		}
		// bondRegHeight (old-due-bucket derivation), one key per named id.
		c.recEmitBondRegHeight(acc, id)
		touched[id] = struct{}{}
	}
	// --- the slash reads, grouped by field ---
	for i := range b.Slashes {
		culprit := b.Slashes[i].CulpritID()
		c.recEmitSlashed(acc, culprit)
		touched[culprit] = struct{}{}
	}
	// bonded/qualified for every touched id (write-targets the recompute reads).
	for id := range touched {
		c.recEmitBonded(acc, id)
		c.recEmitQualified(acc, id)
	}

	// --- era-4 TTL completeness: the single dueBucket[h] leaf ---
	if c.cfg.BondTTLBlocks > 0 {
		var hk [8]byte
		binary.BigEndian.PutUint64(hk[:], b.Height)
		if ids, occupied := c.dueBucket[b.Height]; occupied {
			acc.addPresent(tagDueBucket, hk[:], dueBucketMTH(ids))
			for id := range ids {
				c.recEmitBonded(acc, id)
				c.recEmitBondRegHeight(acc, id)
				c.recEmitRegVersion(acc, id)
				c.recEmitQualified(acc, id)
			}
		} else {
			// The empty-bucket non-membership path (the whole era-4 win): one absent leaf.
			acc.addAbsent(tagDueBucket, hk[:])
		}
	}

	// --- era-4 boundary delta: qualified freeze source ∪ prior epochSet ---
	if c.epochsEnabled() && c.cfg.EpochBlocks > 0 && b.Height%c.cfg.EpochBlocks == 0 {
		for id, w := range c.qualified {
			acc.addPresent(tagQualified, id[:], statehash.EncodeInt64(w))
			c.recEmitRegVersion(acc, id)
		}
		for id, w := range c.epochSet {
			acc.addPresent(tagEpochSet, id[:], statehash.EncodeInt64(w))
		}
	}
	return acc.entries()
}

func (c *Chain) recEmitSlashed(acc *readSetAcc, id ports.NodeID) {
	if c.slashed[id] {
		acc.addPresent(tagSlashed, id[:], statehash.Present)
	} else {
		acc.addAbsent(tagSlashed, id[:])
	}
}

func (c *Chain) recEmitBondRootProven(acc *readSetAcc, root ports.Hash) {
	if pv, ok := c.bondRootProven[root]; ok {
		acc.addPresent(tagBondRootProven, root[:], statehash.EncodeBool(pv))
	} else {
		acc.addAbsent(tagBondRootProven, root[:])
	}
}

func (c *Chain) recEmitBondRegHeight(acc *readSetAcc, id ports.NodeID) {
	if h, ok := c.bondRegHeight[id]; ok {
		acc.addPresent(tagBondRegHeight, id[:], statehash.EncodeUint64(h))
	} else {
		acc.addAbsent(tagBondRegHeight, id[:])
	}
}

func (c *Chain) recEmitRegVersion(acc *readSetAcc, id ports.NodeID) {
	if v, ok := c.regVersion[id]; ok {
		acc.addPresent(tagRegVersion, id[:], statehash.EncodeUint8(v))
	} else {
		acc.addAbsent(tagRegVersion, id[:])
	}
}

func (c *Chain) recEmitBonded(acc *readSetAcc, id ports.NodeID) {
	if w, ok := c.bonded[id]; ok {
		acc.addPresent(tagBonded, id[:], statehash.EncodeInt64(w))
	} else {
		acc.addAbsent(tagBonded, id[:])
	}
}

func (c *Chain) recEmitQualified(acc *readSetAcc, id ports.NodeID) {
	if w, ok := c.qualified[id]; ok {
		acc.addPresent(tagQualified, id[:], statehash.EncodeInt64(w))
	} else {
		acc.addAbsent(tagQualified, id[:])
	}
}

// v5ReadSetCorpusBlock labels one corpus block with the class it exercises, so a test
// can assert the corpus actually covered each certified class (the vacuity guard).
type v5ReadSetCorpusBlock struct {
	block Block
	label string
	class string // "ordinary" | "ttl-empty" | "ttl-occupied" | "renew-move" | "slash" | "boundary"
}

// buildV5ReadSetCorpus applies a branch-covering block stream to a chain and returns
// the PRE-APPLY snapshot for each block paired with the block, so the read-set producer
// (which reads pre-apply state) can be exercised on each class. The corpus MUST cover
// (cert R3): an ordinary block, a TTL-firing block with a NON-EMPTY dueBucket[h], a
// TTL-firing block with an EMPTY dueBucket[h] (the whole era-4 win), a renew that moves
// a bucket, a slash-before-due block, and an epoch-boundary block.
//
// The chain is objective + TTL-enabled + epochs-enabled so all three accelerator paths
// are live. Blocks are minted at Version 1 for apply() (era-4 is not activated in the
// fixture — like the RECERT2 maintenance tests, the point is read-set correctness over
// the v5 recompute, which the producer computes on demand for a Version-5 view of each
// block), but the READ-SET is computed for a v5 view (setV5 forces Version 5).
func buildV5ReadSetCorpus(t *testing.T) []v5ReadSetCorpusBlock {
	t.Helper()
	a, bb, cc, sq := key(90001), key(90002), key(90003), key(90004)
	honest := key(90005)

	// MatureValidators=0 hands maturity off at the genesis boundary (the
	// TestBoundaryCopyStaleCaptureOrderingAblation pattern), so post-genesis boundaries
	// freeze a REAL epochSet — the epochSet-delta read path is genuinely exercised, and
	// the h8 boundary reads a populated prior epochSet (the young→mature handoff the cert
	// requires the corpus to cover).
	cfg := Config{Quorum: 1, MinBond: era4MinBond, ByzantineQuorum: true,
		EpochBlocks: 4, MatureValidators: 0, BondTTLBlocks: 4}
	c := New(cfg, func(ports.NodeID) int64 { return 0 })
	c.SetBondVerifier(objectiveVerify)

	var corpus []v5ReadSetCorpusBlock
	// snapshot(block, label, class) records the PRE-APPLY read-set inputs, then applies.
	snapshot := func(blk *Block, label, class string) {
		t.Helper()
		corpus = append(corpus, v5ReadSetCorpusBlock{block: *blk, label: label, class: class})
		c.apply(*blk)
	}

	honestRoot := ports.HashBytes(pubOf(honest))
	// Genesis (boundary h0): seat a, bb, cc; sq squats honest's root (unproven).
	g := &Block{Version: 1, Height: 0, Entries: []ports.Entry{entry(0)}}
	g.BondRegs = []BondReg{
		bondReg(a, 4<<20, ports.Hash{}),
		bondReg(bb, 4<<20, ports.Hash{}),
		bondReg(cc, 4<<20, ports.Hash{}),
		{Validator: pubOf(sq), Root: honestRoot, Size: 4 << 20, Answer: []byte("valid")},
	}
	Sign(g, a)
	snapshot(g, "genesis(boundary+regs)", "boundary")

	// h1: ORDINARY block — a plain publish, no reg/slash/TTL-fire, non-boundary.
	prev := g.Hash()
	b1 := &Block{Version: 1, Height: 1, Prev: prev, Entries: []ports.Entry{entry(1)}}
	Sign(b1, a)
	snapshot(b1, "h1 ordinary publish", "ordinary")

	// h2: honest PROVES honestRoot → DISPLACES sq (the displacement branch: bondRootOwner
	// present, owner != id, reads bonded[sq]/qualified[sq]). Non-boundary, TTL empty at h2.
	prev = b1.Hash()
	b2 := &Block{Version: 1, Height: 2, Prev: prev, Entries: []ports.Entry{entry(2)},
		BondRegs: []BondReg{bondRegFull(honest, honestRoot, 4<<20, prev, 5, 7)}}
	Sign(b2, a)
	snapshot(b2, "h2 displacement", "ordinary")

	// h3: RENEW a (moves its due-bucket D_old=0+4+1=5 → D_new=3+4+1=8) — the renew-move path.
	prev = b2.Hash()
	b3 := &Block{Version: 1, Height: 3, Prev: prev, Entries: []ports.Entry{entry(3)},
		BondRegs: []BondReg{bondReg(a, 4<<20, prev)}}
	Sign(b3, a)
	snapshot(b3, "h3 renew-move", "renew-move")

	// h4: BOUNDARY block (h4 % EpochBlocks(4) == 0) AND slashes cc — boundary + slash.
	prev = b3.Hash()
	b4 := &Block{Version: 1, Height: 4, Prev: prev, Entries: []ports.Entry{entry(4)},
		Slashes: []Equivocation{slashProof(cc, prev, 0x41, 0x42)}}
	Sign(b4, a)
	snapshot(b4, "h4 boundary+slash", "boundary")

	// h5: TTL-FIRING with an OCCUPIED bucket. bb/cc/sq/honest reg'd at genesis (h0), due
	// 0+4+1=5. So dueBucket[5] is occupied at h5 — the occupied-member-list path.
	prev = b4.Hash()
	b5 := &Block{Version: 1, Height: 5, Prev: prev, Entries: []ports.Entry{entry(5)}}
	Sign(b5, a)
	snapshot(b5, "h5 ttl-occupied", "ttl-occupied")

	// h6: TTL-FIRING with an EMPTY bucket — the whole era-4 win. Nothing is due at h6
	// (genesis regs due at 5 already swept; a renewed at h3 due 8; honest at h2 due 7).
	// dueBucket[6] is empty → one non-membership leaf.
	prev = b5.Hash()
	b6 := &Block{Version: 1, Height: 6, Prev: prev, Entries: []ports.Entry{entry(6)}}
	Sign(b6, a)
	snapshot(b6, "h6 ttl-empty", "ttl-empty")

	// h7: ordinary filler to reach the next boundary.
	prev = b6.Hash()
	b7 := &Block{Version: 1, Height: 7, Prev: prev, Entries: []ports.Entry{entry(7)}}
	Sign(b7, a)
	snapshot(b7, "h7 ordinary filler", "ordinary")

	// h8: MATURE BOUNDARY with a populated PRIOR epochSet. The h4 boundary (mature, since
	// MatureValidators=0 handed off at genesis) froze qualified into epochSet, so at h8 the
	// recompute reads BOTH the live qualified freeze source AND the prior frozen epochSet —
	// the epochSet-delta read path. Covers the cert's boundary-delta class with real data.
	prev = b7.Hash()
	b8 := &Block{Version: 1, Height: 8, Prev: prev, Entries: []ports.Entry{entry(8)}}
	Sign(b8, a)
	snapshot(b8, "h8 mature-boundary", "boundary")

	return corpus
}

// setV5 returns a copy of b with Version forced to BlockVersionWitnessable, so the
// read-set producer computes the v5 read-set for a block the fixture minted at v1
// (the fixture applies at v1 to keep apply() on the era-2 path; the producer's job is
// the v5 read-set, which it computes for any block viewed as v5).
func setV5(b Block) Block {
	b.Version = BlockVersionWitnessable
	return b
}

// TestWitnessReadSetV5DriftGuard is the R3 drift guard: over a branch-covering corpus,
// the producer's read-set keyset EQUALS the independent recompute-reads enumeration for
// every block. A refactor of either side that changes what is read reddens here.
//
// RED (ablation subtests below): dropping a dueBucket key or a qualified/epochSet delta
// key from the producer diverges the two keysets and reddens.
func TestWitnessReadSetV5DriftGuard(t *testing.T) {
	corpus := buildV5ReadSetCorpus(t)

	// Rebuild the pre-apply chain state per block so the read-set is computed against the
	// state a floor box holds BEFORE applying that block.
	c := replayV5Corpus(t)
	classesSeen := make(map[string]bool)
	for i, cb := range corpus {
		v5b := setV5(cb.block)
		produced := c.WitnessReadSetV5(v5b)
		recompute := c.recomputeWitnessReadsV5(v5b)

		pk, rk := keySet(produced), keySet(recompute)
		if !equalStrSlices(pk, rk) {
			t.Fatalf("[%s] read-set DRIFT: producer keyset != recompute-reads keyset\n producer=%v\n recompute=%v",
				cb.label, pk, rk)
		}
		classesSeen[cb.class] = true
		// Advance the chain by applying this block, so block i+1 sees the correct pre-apply state.
		c.apply(cb.block)
		_ = i
	}

	// Vacuity guard (the session-7 scar): assert the corpus actually covered every class,
	// ESPECIALLY the empty-dueBucket non-membership path — the whole era-4 win.
	for _, class := range []string{"ordinary", "ttl-empty", "ttl-occupied", "renew-move", "boundary"} {
		if !classesSeen[class] {
			t.Fatalf("corpus never covered class %q — the drift guard is vacuous for it", class)
		}
	}
}

// replayV5Corpus rebuilds a fresh chain seeded identically to buildV5ReadSetCorpus but
// applies NOTHING yet — the caller applies each corpus block after computing its
// pre-apply read-set. This keeps the read-set input = the pre-apply committed state.
func replayV5Corpus(t *testing.T) *Chain {
	t.Helper()
	cfg := Config{Quorum: 1, MinBond: era4MinBond, ByzantineQuorum: true,
		EpochBlocks: 4, MatureValidators: 0, BondTTLBlocks: 4}
	c := New(cfg, func(ports.NodeID) int64 { return 0 })
	c.SetBondVerifier(objectiveVerify)
	return c
}

func equalStrSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// TestWitnessReadSetV5BoundedNotRegistrySized is the O(payload) property (cert
// §"Sub-question 2"): an ordinary block's and a TTL-EMPTY block's read-set size must NOT
// scale with the registry — this is the whole era-4 win the producer must deliver (the
// TTL completeness collapses to ONE dueBucket[h] non-membership leaf, never a
// bondRegHeight scan). Two chains with DIFFERENT registry sizes but the SAME small block
// payload must produce read-sets of the SAME size.
//
// This is the direct counter-proof to the certified sharpest hazard: a producer that
// instrumented apply()'s TTL sweep would emit O(registry) keys here (one per id in
// bondRegHeight), and the two sizes would DIVERGE. Equal sizes prove the producer targets
// the bounded witnessable recompute, not apply()'s scan.
func TestWitnessReadSetV5BoundedNotRegistrySized(t *testing.T) {
	// build(nReg) seats nReg bonded validators at genesis, then returns the chain plus a
	// TTL-EMPTY block at a height where nothing is due (so the TTL path is the single
	// non-membership leaf). Genesis regs (height 0) are due at 0+ttl+1 = 5; the probe block
	// is at height 3 (< 5), so dueBucket[3] is empty — the empty-bucket path — regardless
	// of how many ids are in the registry.
	build := func(nReg int) (*Chain, Block) {
		cfg := Config{Quorum: 1, MinBond: era4MinBond, ByzantineQuorum: true,
			EpochBlocks: 64, MatureValidators: 0, BondTTLBlocks: 4}
		c := New(cfg, func(ports.NodeID) int64 { return 0 })
		c.SetBondVerifier(objectiveVerify)

		g := &Block{Version: 1, Height: 0, Entries: []ports.Entry{entry(0)}}
		for i := 0; i < nReg; i++ {
			g.BondRegs = append(g.BondRegs, bondReg(key(int64(91000+i)), 4<<20, ports.Hash{}))
		}
		Sign(g, key(91000))
		c.apply(*g)

		// Advance to height 3 with empty ordinary blocks, so the probe block sees a fully
		// seated registry but nothing due at its height.
		prev := g.Hash()
		for h := uint64(1); h <= 2; h++ {
			bh := &Block{Version: 1, Height: h, Prev: prev, Entries: []ports.Entry{entry(byte(h))}}
			Sign(bh, key(91000))
			c.apply(*bh)
			prev = bh.Hash()
		}
		probe := Block{Version: BlockVersionWitnessable, Height: 3, Prev: prev,
			Entries: []ports.Entry{entry(3)}}
		return c, probe
	}

	small, probeSmall := build(3)   // 3 registered validators
	large, probeLarge := build(300) // 300 registered validators — 100x the registry

	// Confirm the probe height's bucket is EMPTY on both (the era-4 win path).
	if _, occ := small.dueBucket[3]; occ {
		t.Fatal("probe height bucket unexpectedly occupied on small chain — not the empty-bucket path")
	}
	if _, occ := large.dueBucket[3]; occ {
		t.Fatal("probe height bucket unexpectedly occupied on large chain — not the empty-bucket path")
	}
	// Confirm the registries genuinely differ (the test would be vacuous otherwise).
	if len(small.bondRegHeight) == len(large.bondRegHeight) {
		t.Fatalf("registries did not differ: small=%d large=%d", len(small.bondRegHeight), len(large.bondRegHeight))
	}

	rsSmall := small.WitnessReadSetV5(probeSmall)
	rsLarge := large.WitnessReadSetV5(probeLarge)

	if len(rsSmall) != len(rsLarge) {
		t.Fatalf("TTL-EMPTY read-set SCALED WITH THE REGISTRY: |small(reg=%d)|=%d, |large(reg=%d)|=%d — "+
			"the producer is reading O(registry) keys (the certified sharpest hazard: it instrumented "+
			"apply()'s bondRegHeight scan instead of the bounded dueBucket[h] non-membership leaf)",
			len(small.bondRegHeight), len(rsSmall), len(large.bondRegHeight), len(rsLarge))
	}
}

// TestWitnessReadSetV5DriftGuardAblation proves the guard is not decoration: it injects
// each certified divergence (a dropped dueBucket key, a dropped qualified/epochSet delta
// key) into a COPY of the producer's output and asserts the guard's equality check goes
// RED. This is the "inject the defect and watch it go red" discipline (cert R3, the
// session-7 rule). GREEN-on-restore is the un-mutated TestWitnessReadSetV5DriftGuard.
func TestWitnessReadSetV5DriftGuardAblation(t *testing.T) {
	corpus := buildV5ReadSetCorpus(t)
	c := replayV5Corpus(t)

	// Find and hold the pre-apply state at the empty-dueBucket block (ttl-empty) and the
	// boundary block, so the ablation targets a block that actually reads those keys.
	var ttlEmpty, boundary *Chain
	var ttlEmptyBlk, boundaryBlk Block
	for _, cb := range corpus {
		switch cb.class {
		case "ttl-empty":
			ttlEmpty = c.cloneForDryRun() // pre-apply committed-state snapshot (read-only here)
			ttlEmptyBlk = cb.block
		case "boundary":
			// Use the LAST boundary (h4) — it has a populated qualified/epochSet to drop from.
			boundary = c.cloneForDryRun()
			boundaryBlk = cb.block
		}
		c.apply(cb.block)
	}
	if ttlEmpty == nil || boundary == nil {
		t.Fatal("corpus did not yield the ttl-empty and boundary pre-apply states")
	}

	// --- Ablation 1: drop the dueBucket[h] key from the producer's read-set. ---
	t.Run("drop-dueBucket-key-reddens", func(t *testing.T) {
		v5b := setV5(ttlEmptyBlk)
		full := ttlEmpty.WitnessReadSetV5(v5b)
		recompute := ttlEmpty.recomputeWitnessReadsV5(v5b)
		// Sanity: the un-ablated pair AGREES (the guard is green before ablation).
		if !equalStrSlices(keySet(full), keySet(recompute)) {
			t.Fatalf("pre-ablation drift on ttl-empty: producer=%v recompute=%v", keySet(full), keySet(recompute))
		}
		var hk [8]byte
		binary.BigEndian.PutUint64(hk[:], v5b.Height)
		dueKey := statehash.Key(tagDueBucket, hk[:])
		ablated := dropKey(full, dueKey)
		if len(ablated) == len(full) {
			t.Fatalf("ablation targeted a key the read-set does not contain: %x — the ttl-empty block must read dueBucket[h]", dueKey)
		}
		if equalStrSlices(keySet(ablated), keySet(recompute)) {
			t.Fatal("GUARD FAILED TO REDDEN: dropping the dueBucket[h] non-membership key still matched the recompute reads — the empty-bucket win is unguarded")
		}
	})

	// --- Ablation 2: drop a qualified delta key from the boundary read-set. ---
	t.Run("drop-qualified-delta-key-reddens", func(t *testing.T) {
		v5b := setV5(boundaryBlk)
		full := boundary.WitnessReadSetV5(v5b)
		recompute := boundary.recomputeWitnessReadsV5(v5b)
		if !equalStrSlices(keySet(full), keySet(recompute)) {
			t.Fatalf("pre-ablation drift on boundary: producer=%v recompute=%v", keySet(full), keySet(recompute))
		}
		qKey, ok := firstKeyWithTag(full, tagQualified)
		if !ok {
			t.Fatal("boundary read-set carries no qualified delta key — the corpus did not populate qualified at the boundary")
		}
		ablated := dropKey(full, qKey)
		if equalStrSlices(keySet(ablated), keySet(recompute)) {
			t.Fatal("GUARD FAILED TO REDDEN: dropping a qualified delta key still matched the recompute reads — the boundary delta is unguarded")
		}
	})

	// --- Ablation 3: drop an epochSet delta key from the boundary read-set. ---
	t.Run("drop-epochSet-delta-key-reddens", func(t *testing.T) {
		v5b := setV5(boundaryBlk)
		full := boundary.WitnessReadSetV5(v5b)
		recompute := boundary.recomputeWitnessReadsV5(v5b)
		eKey, ok := firstKeyWithTag(full, tagEpochSet)
		if !ok {
			// If this boundary froze no prior epochSet (first boundary), skip — but the h4
			// boundary follows the genesis boundary, so epochSet is populated.
			t.Skip("boundary read-set carries no epochSet delta key (first-boundary case)")
		}
		ablated := dropKey(full, eKey)
		if equalStrSlices(keySet(ablated), keySet(recompute)) {
			t.Fatal("GUARD FAILED TO REDDEN: dropping an epochSet delta key still matched the recompute reads")
		}
	})
}

// dropKey returns rs with the first entry whose Key equals key removed. Used only by
// the ablation to simulate a producer that forgot a read-set member.
func dropKey(rs []statehash.ReadEntry, key []byte) []statehash.ReadEntry {
	out := make([]statehash.ReadEntry, 0, len(rs))
	dropped := false
	for _, e := range rs {
		if !dropped && string(e.Key) == string(key) {
			dropped = true
			continue
		}
		out = append(out, e)
	}
	return out
}

// firstKeyWithTag returns the first read-set key whose field tag matches tag.
func firstKeyWithTag(rs []statehash.ReadEntry, tag string) ([]byte, bool) {
	for _, e := range rs {
		if len(e.Key) >= len(tag) && string(e.Key[:len(tag)]) == tag {
			return e.Key, true
		}
	}
	return nil, false
}
