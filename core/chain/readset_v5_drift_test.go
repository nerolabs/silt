package chain

import (
	"encoding/binary"
	"sort"
	"testing"

	"github.com/nerolabs/silt/core/statehash"
	"github.com/nerolabs/silt/ports"
)

// R3 EXECUTION-DERIVED completeness guard — the load-bearing defense for the v5 witness
// read-set producer (lane-1 Part A, AMENDED cert
// era4-witness-floor-box-readset-v5-AMENDED-RESEARCH-CERTIFICATION-2026-08-30, residual R3;
// PE ruling RULING-lane1-partA-readset-v5-producer-2026-08-30, fixes 4 + 5).
//
// WHY THIS REPLACES THE PRIOR GUARD. The prior guard was a SECOND HAND-WRITTEN enumeration
// (recomputeWitnessReadsV5) checked against the producer. Both inherited the prior cert's
// blind spot (validatorsSeen, everMature, the scalars), so set-equality was GREEN over a
// real accept-a-forgery gap. The amended cert makes an EXECUTION-DERIVED guard MANDATORY: the
// "expected" read-set must be derived from the RECORDED leaf-touch of the REAL v5 recompute,
// not from a mirror of the producer. A guard that compares the producer to a hand-written
// table certifies nothing.
//
// THE GROUND TRUTH. The real v5 recompute is postApplyRoots(b) → cloneForDryRun() → apply(b)
// → StateRootForVersion(5) → stateRootLeavesV5 (era3validity.go:145, statehash.go:182). This
// guard derives the recompute's read-set from that computation by TWO execution-derived
// sources, NEITHER hand-written:
//
//   - THE WRITE-DIFF (Category 1 — write-target reads): the committed leaves whose value
//     CHANGES between the pre-apply and post-apply stateRootLeavesV5. Every write-target is
//     read (a map write needs the pre-state to compute the post-value; a monotonic scalar
//     gates on its pre-state). This captures exactly the prior build's dropped leaves
//     (validatorsSeen / everMature / the scalars are all write-targets).
//   - THE LEAF-SENSITIVITY PERTURBATION (Category 2 — pure gate reads): for each committed
//     leaf present pre-apply, perturb its value on a fresh clone and re-run the REAL
//     postApplyRoots; a leaf whose perturbation CHANGES the output root is one the recompute
//     READS (a floor box must witness it, else it cannot detect a forged value there). This
//     catches the pure gate reads a write-diff misses (a slashed[id] gate on a non-slashed
//     id, the boundary regVersion reads for unchanged frozen members, bonded/bondDomain
//     maturity inputs).
//
// THE ASSERTION. The producer's read-set must COVER (⊇) the union of the two ground-truth
// sources. Over-witnessing is sound (a little extra witness bandwidth, never a wrong-accept);
// UNDER-witnessing is the soundness hole. So the guard's binding direction is: producer ⊇
// ground-truth. Over-emission to O(registry) is caught separately by
// TestWitnessReadSetV5BoundedNotRegistrySized.
//
// THE ABLATION (the "inject the defect and watch it go red" discipline). The guard reddens on
// the exact defects that escaped the prior build: a dropped attestation-loop read (the
// validatorsSeen omission), a dropped slash-path qualified read, and the certified
// boundedness ablation.

// keySet is the sorted set of leaf keys (tag||rawKey) in a read-set. The guard compares
// KEYS: which committed leaves the box must witness. The kind (present/absent) and value are
// downstream shape concerns; completeness is about the key set.
func keySet(rs []statehash.ReadEntry) map[string]struct{} {
	out := make(map[string]struct{}, len(rs))
	for _, e := range rs {
		out[string(e.Key)] = struct{}{}
	}
	return out
}

// leafKeySet is the set of committed leaf keys a state emits under the v5 root, keyed by
// string(leaf.Key) → string(leaf.Value). Two states' write-diff is the symmetric key/value
// difference of their leafKeySets.
func leafKeySet(c *Chain) map[string]string {
	out := make(map[string]string)
	for _, lf := range c.stateRootLeavesV5() {
		out[string(lf.Key)] = string(lf.Value)
	}
	return out
}

// groundTruthReadSet derives the REAL v5 recompute's read-set for block b against the
// pre-apply state c, by the two execution-derived sources (write-diff ∪ perturbation). It
// mutates nothing on c: every apply runs on a fresh cloneForDryRun clone. Returns the set of
// leaf keys the recompute reads.
func groundTruthReadSet(t *testing.T, c *Chain, b Block) map[string]struct{} {
	t.Helper()
	reads := make(map[string]struct{})

	// --- Source 1: the write-diff over the REAL recompute (cloneForDryRun + apply). ---
	pre := leafKeySet(c)
	post := c.cloneForDryRun()
	post.apply(b)
	postLeaves := leafKeySet(post)
	for k, v := range postLeaves {
		if pre[k] != v { // added or value-changed leaf → written → read
			reads[k] = struct{}{}
		}
	}
	for k := range pre {
		if _, still := postLeaves[k]; !still { // removed leaf → written (deleted) → read
			reads[k] = struct{}{}
		}
	}

	// --- Source 2: leaf-sensitivity perturbation over the REAL recompute. ---
	// For each committed leaf L present pre-apply, perturb its value on a fresh clone, run the
	// REAL apply, and compare the post-leaf-set to the unperturbed post-leaf-set — EXCLUDING
	// L's own key. A CROSS-LEAF difference (some OTHER leaf changed) ⟹ the recompute READ L and
	// used it to compute another leaf's post-value. That is a genuine read the box must witness.
	//
	// L's own key is EXCLUDED from the comparison because the perturbed value persists into the
	// post-state for a leaf apply() does not overwrite — that self-persistence is NOT a read
	// (the recompute never consulted L to produce anything). L's own write-target status (its
	// post-value depending on its pre-value) is already captured by Source 1 (the write-diff).
	// So perturbation isolates the PURE gate reads Source 1 misses.
	refPost := c.cloneForDryRun()
	refPost.apply(b)
	refLeaves := leafKeySet(refPost)
	for _, lf := range c.stateRootLeavesV5() {
		clone := c.cloneForDryRun()
		if !perturbLeaf(clone, lf.Key, lf.Value) {
			continue // keyspace not perturbable by this helper (asserted covered elsewhere)
		}
		clone.apply(b)
		pLeaves := leafKeySet(clone)
		if crossLeafDiffers(refLeaves, pLeaves, string(lf.Key)) {
			reads[string(lf.Key)] = struct{}{}
		}
	}

	// --- Source 3: the VALIDITY-READ perturbation (the apply()-blind reads). ---
	// The read-set a floor box needs is validity ∪ apply-recompute (AMENDED cert). Sources 1
	// and 2 are BOTH apply()-shaped, so they are structurally blind to a leaf read ONLY in the
	// VALIDITY predicate and never in apply(): spent[serial] (the double-spend gate,
	// chain.go:2617) and revoked[root] (the un-revocation gate, chain.go:2643). Perturbing
	// either changes NO apply-recompute leaf, so Sources 1/2 never list them — dropping them
	// from the producer stayed GREEN (the "a read no guard catches" failure this guard exists
	// to kill). Source 3 closes that: for each committed leaf L, perturb it on a fresh clone and
	// re-run the REAL validity read-predicates; if the accept/reject VERDICT flips, the
	// predicate READ L, so the box must witness it.
	//
	// This drives the production predicates that perform the reads — validateTakedowns (reads
	// revoked/byRoot) and, per entry, ValidateEntry (reads spent/byRoot) — and EXCLUDES
	// validateEra3Roots (the root recompute). The root predicate must be excluded: perturbing
	// ANY committed leaf changes the recomputed root, so including it would flip the verdict for
	// every leaf (all false positives). That root-recompute channel IS Source 1 (the write-diff
	// is exactly the leaves the root commits). The exclusion mirrors Source 2's exclusion of the
	// perturbed leaf's own key: each source isolates the channel the others miss. Source 3
	// isolates the pure validity-gate reads. It only ADDS reads to the ground truth; the binding
	// direction (producer ⊇ ground-truth) and soundness of over-witnessing are unchanged.
	refVerdict := validityVerdict(c, b)
	for _, lf := range c.stateRootLeavesV5() {
		clone := c.cloneForDryRun()
		if !perturbLeaf(clone, lf.Key, lf.Value) {
			continue
		}
		if validityVerdict(clone, b) != refVerdict {
			reads[string(lf.Key)] = struct{}{}
		}
	}
	return reads
}

// validityVerdict runs the REAL validity read-predicates for block b against pre-apply state c
// and returns "accept" or "reject" — the accept/reject verdict a floor box's validity check
// yields. It runs the production predicates that perform the committed-state reads
// (validateTakedowns → revoked/byRoot; per-entry ValidateEntry → spent/byRoot) and DELIBERATELY
// EXCLUDES validateEra3Roots (the root recompute), because a perturbed leaf changes the
// recomputed root and would flip the verdict for every leaf, swamping the pure-gate signal
// Source 3 isolates (the root-recompute channel is Source 1's write-diff). The first rejecting
// predicate short-circuits, matching ValidateProposal's own order.
func validityVerdict(c *Chain, b Block) string {
	if err := c.validateTakedowns(&b); err != nil {
		return "reject"
	}
	for _, e := range b.Entries {
		if err := c.ValidateEntry(e); err != nil {
			return "reject"
		}
	}
	return "accept"
}

// crossLeafDiffers reports whether two post-leaf-sets differ on ANY key other than `self`
// (the perturbed leaf's own key). A cross-leaf difference means perturbing `self` changed
// some other committed leaf — i.e. the recompute read `self`.
func crossLeafDiffers(ref, got map[string]string, self string) bool {
	for k, v := range got {
		if k == self {
			continue
		}
		if ref[k] != v {
			return true
		}
	}
	for k := range ref {
		if k == self {
			continue
		}
		if _, ok := got[k]; !ok {
			return true
		}
	}
	return false
}

// perturbLeaf mutates the committed map/scalar backing leaf `key` on clone to a value that
// DIFFERS from its committed value, so a recompute that reads it produces a different root.
// It is TEST-ONLY (same-package field access) and touches NO production code path — it only
// writes into the clone's committed state before the dry-run apply. Returns false if the
// keyspace is not one this helper perturbs (the caller skips it; TestGroundTruthPerturbation
// Covers asserts the reachable keyspaces are all perturbable so no leaf silently escapes).
//
// The perturbation strategy per class: for a map leaf, flip presence (delete if present) — a
// deleted leaf changes the leaf set, and a recompute that reads it computes a different
// post-state. For a scalar, bump the value. The perturbation only needs to be OBSERVABLE by a
// recompute that reads the leaf; it does not need to be a valid state.
func perturbLeaf(clone *Chain, key, curVal []byte) bool {
	tag, raw, ok := splitLeafKey(key)
	if !ok {
		return false
	}
	// Scalars: bump the scalar field so a reader sees a different pre-state.
	switch tag {
	case tagEverMature:
		clone.everMature = !clone.everMature
		return true
	case tagMatureEpoch:
		clone.matureEpoch = !clone.matureEpoch
		return true
	case tagGateLockedIn:
		clone.gateLockedIn = !clone.gateLockedIn
		return true
	case tagGateHeight:
		clone.gateHeight++
		return true
	case tagEra3LockedIn:
		clone.era3LockedIn = !clone.era3LockedIn
		return true
	case tagEra3Height:
		clone.era3Height++
		return true
	case tagEpochStart:
		clone.epochStart++
		return true
	case tagEra4LockedIn:
		clone.era4LockedIn = !clone.era4LockedIn
		return true
	case tagEra4Height:
		clone.era4Height++
		return true
	}
	// Map keyspaces: flip the leaf's presence (delete it), which any recompute that reads it
	// will observe. raw is the map key.
	var id ports.NodeID
	var hash ports.Hash
	copy(id[:], raw)
	copy(hash[:], raw)
	switch tag {
	case tagByRoot:
		delete(clone.byRoot, hash)
	case tagSpent:
		delete(clone.spent, string(raw))
	case tagRevoked:
		delete(clone.revoked, hash)
	case tagSlashed:
		delete(clone.slashed, id)
	case tagValidatorsSeen:
		delete(clone.validatorsSeen, id)
	case tagBonded:
		delete(clone.bonded, id)
	case tagEpochSet:
		delete(clone.epochSet, id)
	case tagBondRootOwner:
		delete(clone.bondRootOwner, hash)
	case tagBondRootProven:
		delete(clone.bondRootProven, hash)
	case tagBondRegHeight:
		delete(clone.bondRegHeight, id)
	case tagRegVersion:
		delete(clone.regVersion, id)
	case tagBondDomain:
		delete(clone.bondDomain, id)
	case tagQualified:
		delete(clone.qualified, id)
	case tagDueBucket:
		delete(clone.dueBucket, binary.BigEndian.Uint64(raw))
	default:
		return false
	}
	return true
}

// splitLeafKey splits a leaf key (tag||rawKey) into its field tag and raw key. The tag ends
// at the first NUL (statehash.Key's scheme). Returns false if no NUL is found.
func splitLeafKey(key []byte) (tag string, raw []byte, ok bool) {
	for i, b := range key {
		if b == 0 {
			return string(key[:i+1]), key[i+1:], true
		}
	}
	return "", nil, false
}

// v5ReadSetCorpusBlock labels one corpus block with the class it exercises, so a test can
// assert the corpus actually covered each certified class (the vacuity guard).
type v5ReadSetCorpusBlock struct {
	block Block
	label string
	// class: the certified transition class this block exercises. The vacuity guard requires
	// the corpus to cover every one, INCLUDING the amended cert's additions: attested (the
	// atts-loop / validatorsSeen path), maturity-latch (everMature false→true), and a
	// standalone slash at a non-boundary height.
	class string
}

// buildV5ReadSetCorpus applies a branch-covering block stream and returns the PRE-APPLY
// snapshot chain paired with each block, so the producer (which reads pre-apply state) and
// the execution-derived ground truth are computed against the state a floor box holds BEFORE
// applying each block.
//
// The corpus MUST cover (amended cert R3 + PE Q4): ordinary, ttl-empty (the era-4 win),
// ttl-occupied, renew-move, boundary, ATTESTED (populated b.Atts → validatorsSeen write),
// MATURITY-LATCH (everMature false→true), and a standalone SLASH at a non-boundary height.
//
// MatureValidators=2 (objective) so the maturity latch is a genuine false→true transition
// (with MatureValidators=0 the latch fires vacuously at genesis and no transition is
// witnessed). The network starts immature and matures as distinct bonds accrue.
func buildV5ReadSetCorpus(t *testing.T) ([]v5ReadSetCorpusBlock, *Chain) {
	t.Helper()
	a, bb, cc, dd := key(90001), key(90002), key(90003), key(90004)
	sq := key(90005)

	cfg := Config{Quorum: 1, MinBond: era4MinBond, ByzantineQuorum: true,
		EpochBlocks: 4, MatureValidators: 2, BondTTLBlocks: 4}
	c := New(cfg, func(ports.NodeID) int64 { return 0 })
	c.SetBondVerifier(objectiveVerify)

	// A SECOND chain replays the same stream to hand back per-block PRE-APPLY snapshots: the
	// caller computes the producer + ground truth against `snap` BEFORE applying each block.
	snap := New(cfg, func(ports.NodeID) int64 { return 0 })
	snap.SetBondVerifier(objectiveVerify)

	var corpus []v5ReadSetCorpusBlock
	record := func(blk *Block, label, class string) {
		t.Helper()
		corpus = append(corpus, v5ReadSetCorpusBlock{block: *blk, label: label, class: class})
		c.apply(*blk)
	}

	sqRoot := ports.HashBytes(pubOf(bb))
	// Genesis (boundary h0): seat a, bb, cc (three distinct bonds, still < MatureValidators?
	// MatureValidators counts distinct bonds; three ≥ 2 so the network can mature). sq squats
	// bb's root (unproven).
	g := &Block{Version: 1, Height: 0, Entries: []ports.Entry{entry(0)}}
	g.BondRegs = []BondReg{
		bondRegFull(a, ports.HashBytes(pubOf(a)), 4<<20, ports.Hash{}, 5, 1),
		{Validator: pubOf(sq), Root: sqRoot, Size: 4 << 20, Answer: []byte("valid"), Domain: 2, Version: 5},
	}
	Sign(g, a)
	record(g, "genesis(boundary+regs)", "boundary")

	// h1: ORDINARY publish, non-boundary, TTL empty.
	prev := g.Hash()
	b1 := &Block{Version: 1, Height: 1, Prev: prev, Entries: []ports.Entry{entry(1)}}
	Sign(b1, a)
	record(b1, "h1 ordinary publish", "ordinary")

	// h2: bb PROVES bb's root → DISPLACES sq (displacement branch: bondRootOwner present,
	// owner != id, reads bonded[sq]/qualified[sq]). Also regs cc + dd (distinct bonds/domains)
	// so the objective maturity coefficient rises toward MatureValidators=2. Non-boundary.
	prev = b1.Hash()
	b2 := &Block{Version: 1, Height: 2, Prev: prev, Entries: []ports.Entry{entry(2)},
		BondRegs: []BondReg{
			bondRegFull(bb, sqRoot, 4<<20, prev, 5, 3),
			bondRegFull(cc, ports.HashBytes(pubOf(cc)), 4<<20, prev, 5, 4),
			bondRegFull(dd, ports.HashBytes(pubOf(dd)), 4<<20, prev, 5, 5),
		}}
	Sign(b2, a)
	record(b2, "h2 displacement+regs", "ordinary")

	// h3: ATTESTED block — bb + cc (qualified non-proposers) attest, so the atts loop writes
	// validatorsSeen[bb]/[cc] (apply:3293-3298). The maturity latch READ path also fires here
	// (everMature is false pre-apply, so Mature()→C2Metric ranges validatorsSeen), but the
	// latch does NOT yet transition — it transitions at h4 (see below). Covers the attested
	// class (the validatorsSeen write path).
	prev = b2.Hash()
	b3 := &Block{Version: 1, Height: 3, Prev: prev, Entries: []ports.Entry{entry(3)}}
	Sign(b3, a)
	b3.Atts = []Attestation{Attest(b3, bb), Attest(b3, cc)}
	record(b3, "h3 attested", "attested")

	// h4: BOUNDARY block (h4 % 4 == 0) that ALSO trips the MATURITY LATCH. By h4 pre-apply
	// validatorsSeen holds bb + cc (attested at h3), both bonded with distinct domains ⇒
	// MatureCoefficient ≥ 2 = MatureValidators, so everMature latches false→true DURING h4's
	// apply, which then makes rotateEpoch freeze qualified→epochSet. The block genuinely
	// exercises BOTH the boundary freeze and the maturity-latch transition, so it is recorded
	// under both classes (the transition is real — the vacuity guard is not satisfied
	// vacuously). It also carries attestations (the mature-epoch atts path).
	prev = b3.Hash()
	b4 := &Block{Version: 1, Height: 4, Prev: prev, Entries: []ports.Entry{entry(4)}}
	Sign(b4, a)
	b4.Atts = []Attestation{Attest(b4, cc), Attest(b4, dd)}
	corpus = append(corpus, v5ReadSetCorpusBlock{block: *b4, label: "h4 boundary", class: "boundary"})
	corpus = append(corpus, v5ReadSetCorpusBlock{block: *b4, label: "h4 maturity-latch", class: "maturity-latch"})
	c.apply(*b4)

	// h5: TTL-FIRING with an OCCUPIED bucket. a/sq reg'd at genesis (h0) due 0+4+1=5; sq was
	// displaced (its bond stripped) but a is due at 5, so dueBucket[5] is occupied at h5.
	prev = b4.Hash()
	b5 := &Block{Version: 1, Height: 5, Prev: prev, Entries: []ports.Entry{entry(5)}}
	Sign(b5, a)
	record(b5, "h5 ttl-occupied", "ttl-occupied")

	// h6: STANDALONE SLASH at a NON-boundary height (h6 % 4 != 0). Slash cc: slashed[cc]
	// write, bonded[cc] delete, qualified[cc] maintain — the slash read path, unmasked by a
	// boundary. This is the amended cert's required standalone-slash class.
	prev = b5.Hash()
	b6 := &Block{Version: 1, Height: 6, Prev: prev, Entries: []ports.Entry{entry(6)},
		Slashes: []Equivocation{slashProof(cc, prev, 0x41, 0x42)}}
	Sign(b6, a)
	record(b6, "h6 standalone-slash", "slash")

	// h7: TTL-FIRING with an OCCUPIED bucket. bb/cc/dd reg'd at h2, due 2+4+1=7. cc was
	// slashed at h6 (removed from bondRegHeight), so dueBucket[7] carries bb + dd at h7 — the
	// occupied-member expiry path at the CURRENT height (heights are contiguous, chain.go:2490,
	// so only dueBucket[b.Height] can fire — the era-4 single-leaf accelerator is exact).
	prev = b6.Hash()
	b7 := &Block{Version: 1, Height: 7, Prev: prev, Entries: []ports.Entry{entry(7)}}
	Sign(b7, a)
	record(b7, "h7 ttl-occupied(expiry)", "ttl-occupied")

	// h8: BOUNDARY block (h8 % 4 == 0) — the mature boundary with a populated prior epochSet
	// (frozen at h4). Covers the boundary freeze-over-prior-epochSet path with real data.
	prev = b7.Hash()
	b8 := &Block{Version: 1, Height: 8, Prev: prev, Entries: []ports.Entry{entry(8)}}
	Sign(b8, a)
	record(b8, "h8 mature-boundary", "boundary")

	// h9: TTL-EMPTY — nothing is due at h9 (genesis due 5 swept at h5; h2 regs due 7 swept at
	// h7; a renewed below). dueBucket[9] is empty → the single non-membership leaf, the era-4
	// win. Non-boundary.
	prev = b8.Hash()
	b9 := &Block{Version: 1, Height: 9, Prev: prev, Entries: []ports.Entry{entry(9)}}
	Sign(b9, a)
	record(b9, "h9 ttl-empty", "ttl-empty")

	// h10: RENEW a (moves its due-bucket D_old→D_new) — the renew-move path. a was last reg'd
	// at genesis (due 5, already swept) then is renewed here, so the move inserts a fresh bucket.
	prev = b9.Hash()
	b10 := &Block{Version: 1, Height: 10, Prev: prev, Entries: []ports.Entry{entry(10)},
		BondRegs: []BondReg{bondReg(a, 4<<20, prev)}}
	Sign(b10, a)
	record(b10, "h10 renew-move", "renew-move")

	return corpus, snap
}

// setV5 returns a copy of b with Version forced to BlockVersionWitnessable, so the read-set
// producer and the v5 recompute run for a block the fixture minted at v1.
func setV5(b Block) Block {
	b.Version = BlockVersionWitnessable
	return b
}

// TestWitnessReadSetV5ExecutionDerivedGuard is the R3 execution-derived completeness guard:
// over a branch-covering corpus, the producer's read-set must COVER (⊇) the ground-truth
// read-set derived from the REAL v5 recompute (write-diff ∪ leaf-sensitivity perturbation).
// A producer that DROPS a read the recompute needs (the prior build's validatorsSeen /
// everMature / scalar omissions) reddens here against ground truth — NOT against a
// hand-written mirror.
func TestWitnessReadSetV5ExecutionDerivedGuard(t *testing.T) {
	corpus, snap := buildV5ReadSetCorpus(t)
	classesSeen := make(map[string]bool)
	// Paired corpus entries (boundary+maturity-latch at h4) share ONE block and MUST both be
	// evaluated against the SAME pre-apply state, so a block is applied only when advancing to
	// a strictly greater height (the pending-block flush below).
	var pending *Block
	for i := range corpus {
		cb := corpus[i]
		// A new height means the previous height's block was fully evaluated: apply it now.
		if pending != nil && cb.block.Height != pending.Height {
			snap.apply(*pending)
			pending = nil
		}

		v5b := setV5(cb.block)
		produced := keySet(snap.WitnessReadSetV5(v5b))
		truth := groundTruthReadSet(t, snap, v5b)

		// COVERAGE: every ground-truth read must be in the producer's read-set.
		var missing []string
		for k := range truth {
			if _, ok := produced[k]; !ok {
				missing = append(missing, prettyKey(k))
			}
		}
		if len(missing) > 0 {
			sort.Strings(missing)
			t.Fatalf("[%s] read-set INCOMPLETE: producer omits %d ground-truth read(s) the real v5 recompute performs:\n  %v",
				cb.label, len(missing), missing)
		}
		classesSeen[cb.class] = true

		// NON-VACUITY of the class-specific transitions (the session-7 scar: a class label the
		// block does not actually exercise makes the guard vacuous for it). Assert the
		// load-bearing transitions genuinely fire, from ground truth:
		switch cb.class {
		case "maturity-latch":
			// everMature MUST transition false→true on this block, else the latch read is a
			// constant and the class is covered vacuously.
			if snap.everMature {
				t.Fatalf("[%s] maturity-latch VACUOUS: everMature already true pre-apply", cb.label)
			}
			post := snap.cloneForDryRun()
			post.apply(cb.block)
			if !post.everMature {
				t.Fatalf("[%s] maturity-latch VACUOUS: everMature did not transition false→true", cb.label)
			}
		case "attested":
			// The block must carry a qualified non-proposer attester whose validatorsSeen is
			// newly written (the atts-loop write path), else the attested class is vacuous.
			pre := leafKeySet(snap)
			post := snap.cloneForDryRun()
			post.apply(cb.block)
			wroteVS := false
			for k := range leafKeySet(post) {
				tag, _, _ := splitLeafKey([]byte(k))
				if tag == tagValidatorsSeen {
					if _, had := pre[k]; !had {
						wroteVS = true
						break
					}
				}
			}
			if !wroteVS {
				t.Fatalf("[%s] attested VACUOUS: no new validatorsSeen leaf written by the atts loop", cb.label)
			}
		}

		// Mark this height's block pending; it is applied when the loop advances to a greater
		// height (so all paired entries at this height see the shared pre-apply state).
		blk := cb.block
		pending = &blk
	}
	if pending != nil {
		snap.apply(*pending)
	}

	// Vacuity guard (the session-7 scar): the corpus must have covered every certified class,
	// INCLUDING the amended cert's additions attested / maturity-latch / slash.
	for _, class := range []string{"ordinary", "ttl-empty", "ttl-occupied", "renew-move",
		"boundary", "attested", "maturity-latch", "slash"} {
		if !classesSeen[class] {
			t.Fatalf("corpus never covered class %q — the guard is vacuous for it", class)
		}
	}
}

// TestWitnessReadSetV5ValidityReadsCovered extends the execution-derived guard to the VALIDITY
// corpus: the producer's read-set must COVER (⊇) the ground-truth read-set (write-diff ∪
// apply-perturbation ∪ VALIDITY-read perturbation) for a block that fires the apply()-blind
// validity reads — spent[serial] (chain.go:2617) and revoked[root] (chain.go:2643). This is the
// coverage half of the fix: it proves the producer STILL emits spent/revoked completely, checked
// against execution-derived ground truth on the blocks that actually read them (the maintenance
// corpus cannot — it has no token quorum and no revoke). The definitive drop check
// (TestWitnessReadSetV5AllKeyspacesRedOnDrop) is the completeness half.
//
// The guard also asserts the two validity reads are GENUINELY in the ground truth (non-vacuity):
// spent must appear for the spent probe, revoked for the revoked probe — else the corpus did not
// exercise the read and the coverage check is empty for it.
func TestWitnessReadSetV5ValidityReadsCovered(t *testing.T) {
	corpus, snap := buildV5ValidityReadCorpus(t)
	wantTag := map[string]string{"validity-spent": tagSpent, "validity-revoked": tagRevoked}
	for _, cb := range corpus {
		v5b := setV5(cb.block)
		produced := keySet(snap.WitnessReadSetV5(v5b))
		truth := groundTruthReadSet(t, snap, v5b)

		var missing []string
		for k := range truth {
			if _, ok := produced[k]; !ok {
				missing = append(missing, prettyKey(k))
			}
		}
		if len(missing) > 0 {
			sort.Strings(missing)
			t.Fatalf("[%s] validity read-set INCOMPLETE: producer omits %d ground-truth read(s):\n  %v",
				cb.label, len(missing), missing)
		}

		// Non-vacuity: the class's validity read must genuinely be in the ground truth (Source 3
		// fired), else the probe did not exercise it.
		if tag, ok := wantTag[cb.class]; ok {
			found := false
			for k := range truth {
				if tg, _, _ := splitLeafKey([]byte(k)); tg == tag {
					found = true
					break
				}
			}
			if !found {
				t.Fatalf("[%s] VACUOUS: ground truth has no %s read — the validity probe did not exercise it", cb.label, prettyTag(tag))
			}
		}
	}
}

// prettyKey renders a leaf key (tag||rawKey) as "tag:hex" for readable failure output.
func prettyKey(k string) string {
	tag, raw, ok := splitLeafKey([]byte(k))
	if !ok {
		return k
	}
	const hexd = "0123456789abcdef"
	h := make([]byte, 0, len(raw)*2)
	for _, b := range []byte(raw) {
		h = append(h, hexd[b>>4], hexd[b&0xf])
	}
	return tag + string(h)
}

// TestGroundTruthPerturbationCovers guards the guard: every committed keyspace the corpus
// pre-apply states carry must be perturbable by perturbLeaf, so no leaf silently escapes the
// perturbation source (a keyspace perturbLeaf returns false for would be an unguarded read).
func TestGroundTruthPerturbationCovers(t *testing.T) {
	corpus, snap := buildV5ReadSetCorpus(t)
	applied := make(map[uint64]bool)
	for _, cb := range corpus {
		for _, lf := range snap.stateRootLeavesV5() {
			clone := snap.cloneForDryRun()
			if !perturbLeaf(clone, lf.Key, lf.Value) {
				tag, _, _ := splitLeafKey(lf.Key)
				t.Fatalf("[%s] perturbLeaf cannot perturb keyspace %q — that leaf escapes the perturbation ground truth", cb.label, tag)
			}
		}
		if !applied[cb.block.Height] {
			snap.apply(cb.block)
			applied[cb.block.Height] = true
		}
	}
}

// --- Ablations: inject the exact defects that escaped the prior build; assert RED. ---

// TestWitnessReadSetV5DriftGuardAblation re-injects each escaped defect into the producer's
// output and asserts the execution-derived guard's COVERAGE check goes RED (the read the real
// recompute performs is no longer covered). GREEN-on-restore is the un-mutated
// TestWitnessReadSetV5ExecutionDerivedGuard.
func TestWitnessReadSetV5DriftGuardAblation(t *testing.T) {
	corpus, snap := buildV5ReadSetCorpus(t)
	// Hold the pre-apply snapshot at the attested block (validatorsSeen write) and the slash
	// block (qualified[culprit] write) so the ablation targets a block that actually reads
	// those leaves.
	var attested, slash *Chain
	var attestedBlk, slashBlk Block
	applied := make(map[uint64]bool)
	for _, cb := range corpus {
		switch cb.class {
		case "attested":
			attested = snap.cloneForDryRun()
			attestedBlk = cb.block
		case "slash":
			slash = snap.cloneForDryRun()
			slashBlk = cb.block
		}
		if !applied[cb.block.Height] {
			snap.apply(cb.block)
			applied[cb.block.Height] = true
		}
	}
	if attested == nil || slash == nil {
		t.Fatal("corpus did not yield the attested and slash pre-apply states")
	}

	// coverageMisses reports the ground-truth reads the (possibly ablated) producer omits.
	coverageMisses := func(c *Chain, blk Block, ablate func([]statehash.ReadEntry) []statehash.ReadEntry) []string {
		v5b := setV5(blk)
		produced := keySet(ablate(c.WitnessReadSetV5(v5b)))
		truth := groundTruthReadSet(t, c, v5b)
		var missing []string
		for k := range truth {
			if _, ok := produced[k]; !ok {
				missing = append(missing, prettyKey(k))
			}
		}
		sort.Strings(missing)
		return missing
	}

	// --- Ablation 1: re-inject the validatorsSeen omission (drop the attestation-loop reads
	// = every validatorsSeen key) from the attested block's producer output. ---
	t.Run("drop-attestation-loop-reads-reddens", func(t *testing.T) {
		// Pre-ablation: the guard is GREEN (producer covers ground truth).
		if miss := coverageMisses(attested, attestedBlk, identityRS); len(miss) > 0 {
			t.Fatalf("pre-ablation drift on attested block: producer already omits %v", miss)
		}
		miss := coverageMisses(attested, attestedBlk, dropTag(tagValidatorsSeen))
		if len(miss) == 0 {
			t.Fatal("GUARD FAILED TO REDDEN: dropping the validatorsSeen atts-loop reads still covered the recompute reads — the exact prior-build gap is unguarded")
		}
		if !containsTag(miss, "validatorsSeen") {
			t.Fatalf("guard reddened but not on validatorsSeen: %v", miss)
		}
	})

	// --- Ablation 2: re-inject the slash-path qualified[culprit] drop. ---
	t.Run("drop-slash-qualified-read-reddens", func(t *testing.T) {
		if miss := coverageMisses(slash, slashBlk, identityRS); len(miss) > 0 {
			t.Fatalf("pre-ablation drift on slash block: producer already omits %v", miss)
		}
		miss := coverageMisses(slash, slashBlk, dropTag(tagQualified))
		if len(miss) == 0 {
			t.Fatal("GUARD FAILED TO REDDEN: dropping the slash-path qualified read still covered the recompute reads")
		}
		if !containsTag(miss, "qualified") {
			t.Fatalf("guard reddened but not on qualified: %v", miss)
		}
	})
}

// identityRS is the no-op ablation (the un-mutated producer output).
func identityRS(rs []statehash.ReadEntry) []statehash.ReadEntry { return rs }

// dropTag returns an ablation that removes every read-set entry with the given field tag,
// simulating a producer that forgot that keyspace's reads (the prior build's omission shape).
func dropTag(tag string) func([]statehash.ReadEntry) []statehash.ReadEntry {
	return func(rs []statehash.ReadEntry) []statehash.ReadEntry {
		out := make([]statehash.ReadEntry, 0, len(rs))
		for _, e := range rs {
			t, _, ok := splitLeafKey(e.Key)
			if ok && t == tag {
				continue
			}
			out = append(out, e)
		}
		return out
	}
}

func containsTag(pretty []string, tag string) bool {
	for _, p := range pretty {
		if len(p) >= len(tag) && p[:len(tag)] == tag {
			return true
		}
	}
	return false
}

// TestWitnessReadSetV5BoundedNotRegistrySized is the O(payload) boundedness property (amended
// cert §"Per-class read-set"): an ordinary block's and a TTL-EMPTY block's read-set size must
// NOT scale with the registry — the era-4 win the producer must deliver (the TTL completeness
// collapses to ONE dueBucket[h] non-membership leaf, never a bondRegHeight scan). This is the
// direct counter-proof to the certified sharpest hazard AND the over-emission guard: a
// producer that instrumented apply()'s TTL sweep would emit O(registry) keys here and the two
// sizes would DIVERGE. It reddens under an injected O(registry) scan (proven below).
func TestWitnessReadSetV5BoundedNotRegistrySized(t *testing.T) {
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

	small, probeSmall := build(3)
	large, probeLarge := build(300)

	if _, occ := small.dueBucket[3]; occ {
		t.Fatal("probe height bucket unexpectedly occupied on small chain — not the empty-bucket path")
	}
	if _, occ := large.dueBucket[3]; occ {
		t.Fatal("probe height bucket unexpectedly occupied on large chain — not the empty-bucket path")
	}
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

// TestWitnessReadSetV5BoundednessAblation proves the boundedness test is not decoration: an
// injected O(registry) bondRegHeight scan makes the read-set scale with the registry, and the
// equal-size assertion reddens. This is the certified boundedness ablation (the sharpest
// hazard: instrumenting apply()'s scan defeats era-4).
func TestWitnessReadSetV5BoundednessAblation(t *testing.T) {
	build := func(nReg int) (*Chain, Block) {
		cfg := Config{Quorum: 1, MinBond: era4MinBond, ByzantineQuorum: true,
			EpochBlocks: 64, MatureValidators: 0, BondTTLBlocks: 4}
		c := New(cfg, func(ports.NodeID) int64 { return 0 })
		c.SetBondVerifier(objectiveVerify)
		g := &Block{Version: 1, Height: 0, Entries: []ports.Entry{entry(0)}}
		for i := 0; i < nReg; i++ {
			g.BondRegs = append(g.BondRegs, bondReg(key(int64(92000+i)), 4<<20, ports.Hash{}))
		}
		Sign(g, key(92000))
		c.apply(*g)
		prev := g.Hash()
		for h := uint64(1); h <= 2; h++ {
			bh := &Block{Version: 1, Height: h, Prev: prev, Entries: []ports.Entry{entry(byte(h))}}
			Sign(bh, key(92000))
			c.apply(*bh)
			prev = bh.Hash()
		}
		return c, Block{Version: BlockVersionWitnessable, Height: 3, Prev: prev, Entries: []ports.Entry{entry(3)}}
	}
	small, probeSmall := build(3)
	large, probeLarge := build(300)

	// The INJECTED DEFECT: a producer that scans the whole bondRegHeight map (apply()'s TTL
	// sweep shape) instead of the single dueBucket[h] leaf. Emit one read per registry id.
	ablatedProducer := func(c *Chain, b Block) []statehash.ReadEntry {
		rs := c.WitnessReadSetV5(b)
		acc := newReadSetAcc()
		for _, e := range rs {
			acc.add(e)
		}
		for id := range c.bondRegHeight { // O(registry) scan — the defect
			acc.addPresent(tagBondRegHeight, id[:], statehash.EncodeUint64(c.bondRegHeight[id]))
		}
		return acc.entries()
	}

	rsSmall := ablatedProducer(small, probeSmall)
	rsLarge := ablatedProducer(large, probeLarge)
	if len(rsSmall) == len(rsLarge) {
		t.Fatalf("ABLATION FAILED TO REDDEN: the injected O(registry) scan did not scale the read-set with the registry (small=%d large=%d) — the boundedness guard would not catch it",
			len(rsSmall), len(rsLarge))
	}
}

// --- The validity-read corpus + the definitive all-23-keyspaces completeness proof. ---

// buildV5ValidityReadCorpus builds a corpus that genuinely EXERCISES the two committed leaves
// read ONLY in the validity predicate (never in apply()): spent[serial] (the double-spend gate,
// chain.go:2617) and revoked[root] (the un-revocation gate, chain.go:2643). The maintenance
// corpus (buildV5ReadSetCorpus) runs tokenQuorum=0 and no revoke, so it cannot fire either read
// — Source 3 needs a token-quorum world with a spend and a revoke→unrevoke to have a verdict to
// flip. This corpus is ADDITIVE (it does not touch the maintenance corpus's boundedness/vacuity
// properties) and its blocks are validity PROBES: both probe blocks are evaluated against the
// SAME post-h1 pre-apply state.
//
// Returns the corpus (each block paired with a label) and the pre-apply snapshot chain, same
// contract as buildV5ReadSetCorpus.
func buildV5ValidityReadCorpus(t *testing.T) ([]v5ReadSetCorpusBlock, *Chain) {
	t.Helper()
	oi := newOrderIssuers(t)
	// The two token issuers (oi.keys[0], oi.keys[1]) must be QUALIFIED attesters for
	// publishtoken.Verify to accept their signatures (Verify's qualified callback =
	// attesterQualified). Bond both, plus a third distinct bond, in an objective world with
	// tokens required. MatureValidators=0 (this corpus targets validity reads, not the maturity
	// latch). oi.keys[0] is the proposer.
	prop := oi.keys[0]
	cfg := Config{Quorum: 1, MinBond: era4MinBond, ByzantineQuorum: true,
		EpochBlocks: 64, MatureValidators: 0, BondTTLBlocks: 4}
	build := func() *Chain {
		c := New(cfg, func(ports.NodeID) int64 { return 0 })
		c.SetBondVerifier(objectiveVerify)
		c.RequireTokens(2, oi.issuer)
		return c
	}
	c := build()
	snap := build()

	// Genesis: bond the two issuer keys (so they qualify) + a third distinct bond, and publish a
	// TOKEN entry that spends serial S1 (drives spent[S1] = true). entry(20)'s root is the one we
	// later revoke → unrevoke.
	serialS1 := []byte("validity-corpus-serial-1")
	pubRoot := entry(20).Root
	g := &Block{Version: 1, Height: 0}
	g.Entries = []ports.Entry{tokenEntry(20, oi.mint(serialS1))}
	g.BondRegs = []BondReg{
		bondRegFull(oi.keys[0], ports.HashBytes(pubOf(oi.keys[0])), 4<<20, ports.Hash{}, 5, 1),
		bondRegFull(oi.keys[1], ports.HashBytes(pubOf(oi.keys[1])), 4<<20, ports.Hash{}, 5, 2),
		bondRegFull(oi.keys[2], ports.HashBytes(pubOf(oi.keys[2])), 4<<20, ports.Hash{}, 5, 3),
	}
	Sign(g, prop)
	c.apply(*g)

	// h1: REVOKE the published root (writes revoked[pubRoot]; reads byRoot[pubRoot]).
	prev := g.Hash()
	b1 := &Block{Version: 1, Height: 1, Prev: prev, Revocations: []ports.Hash{pubRoot}}
	Sign(b1, prop)
	c.apply(*b1)

	var corpus []v5ReadSetCorpusBlock

	// PROBE A — the spent read. An entry with a FRESH root (dup-root passes) carrying a token
	// with the ALREADY-SPENT serial S1. ValidateEntry reads spent[S1] → reject; deleting spent[S1]
	// → the token verifies → accept. The verdict flip is carried SOLELY by spent[S1], so Source 3
	// lists spent as a read and dropping it from the producer reddens.
	spentProbe := &Block{Version: 1, Height: 2, Prev: b1.Hash(),
		Entries: []ports.Entry{tokenEntry(21, oi.mint(serialS1))}}
	Sign(spentProbe, prop)
	corpus = append(corpus, v5ReadSetCorpusBlock{block: *spentProbe, label: "spent-read probe", class: "validity-spent"})

	// PROBE B — the revoked read. UN-REVOKE the revoked root. validateTakedowns reads
	// revoked[pubRoot] → present → accept; deleting revoked[pubRoot] → not revoked → reject. The
	// verdict flip is carried SOLELY by revoked[pubRoot].
	revokedProbe := &Block{Version: 1, Height: 2, Prev: b1.Hash(), Unrevocations: []ports.Hash{pubRoot}}
	Sign(revokedProbe, prop)
	corpus = append(corpus, v5ReadSetCorpusBlock{block: *revokedProbe, label: "revoked-read probe", class: "validity-revoked"})

	// Advance snap to the same post-h1 committed state the probes are evaluated against.
	snap.apply(*g)
	snap.apply(*b1)
	return corpus, snap
}

// v5GuardCorpus pairs a corpus with its pre-apply snapshot so the guard and the definitive drop
// check can run over BOTH the maintenance and validity corpora. The probe blocks in the validity
// corpus all sit at the SAME post-h1 pre-apply state, so the snapshot is NOT advanced between
// them (sequential=false), unlike the maintenance corpus, whose blocks are a height sequence.
type v5GuardCorpus struct {
	name       string
	corpus     []v5ReadSetCorpusBlock
	snap       *Chain
	sequential bool
}

func allV5GuardCorpora(t *testing.T) []v5GuardCorpus {
	t.Helper()
	mc, ms := buildV5ReadSetCorpus(t)
	vc, vs := buildV5ValidityReadCorpus(t)
	return []v5GuardCorpus{
		{name: "maintenance", corpus: mc, snap: ms, sequential: true},
		{name: "validity", corpus: vc, snap: vs, sequential: false},
	}
}

// TestWitnessReadSetV5AllKeyspacesRedOnDrop is THE DEFINITIVE COMPLETENESS PROOF: drop EACH of
// the 23 committed keyspaces from the producer, one at a time, over the UNION of the maintenance
// and validity corpora, and assert the execution-derived guard goes RED for EVERY one. A
// keyspace reddens if some corpus block has a ground-truth read of it the ablated producer no
// longer covers. All 23 must redden — including spent and revoked, which the prior guard was
// blind to (Sources 1/2 are apply()-shaped; Source 3, the validity-read perturbation, catches
// them). A keyspace that stays GREEN on drop is a read no guard catches — the exact defect this
// guard exists to kill.
func TestWitnessReadSetV5AllKeyspacesRedOnDrop(t *testing.T) {
	tags := v5CommittedKeyspaceTags()
	if len(tags) != 23 {
		t.Fatalf("expected 23 committed keyspaces, got %d — the completeness proof must cover the full set", len(tags))
	}
	for _, tag := range tags {
		tag := tag
		t.Run(prettyTag(tag), func(t *testing.T) {
			if !keyspaceReddensOnDrop(t, tag) {
				t.Fatalf("GUARD BLIND: dropping keyspace %q from the producer left the execution-derived guard GREEN across every corpus block — a read no guard catches (the exact soundness hole this guard exists to kill)", prettyTag(tag))
			}
		})
	}
}

// keyspaceReddensOnDrop reports whether dropping `tag` from the producer output makes the guard's
// coverage check RED on SOME block across all corpora — i.e. the ablated producer omits a
// ground-truth read of `tag` that the real recompute/validity performs.
func keyspaceReddensOnDrop(t *testing.T, tag string) bool {
	t.Helper()
	drop := dropTag(tag)
	for _, gc := range allV5GuardCorpora(t) {
		applied := make(map[uint64]bool)
		for _, cb := range gc.corpus {
			v5b := setV5(cb.block)
			produced := keySet(drop(gc.snap.WitnessReadSetV5(v5b)))
			truth := groundTruthReadSet(t, gc.snap, v5b)
			for k := range truth {
				if _, ok := produced[k]; ok {
					continue
				}
				tg, _, _ := splitLeafKey([]byte(k))
				if tg == tag {
					return true
				}
			}
			if gc.sequential && !applied[cb.block.Height] {
				gc.snap.apply(cb.block)
				applied[cb.block.Height] = true
			}
		}
	}
	return false
}

// v5CommittedKeyspaceTags is the closed set of the 23 committed v5 keyspaces (statehash.go:39-81).
func v5CommittedKeyspaceTags() []string {
	return []string{
		tagByRoot, tagSpent, tagRevoked, tagSlashed, tagValidatorsSeen, tagBonded, tagEpochSet,
		tagBondRootOwner, tagBondRootProven, tagBondRegHeight, tagRegVersion, tagBondDomain,
		tagQualified, tagDueBucket, tagEverMature, tagMatureEpoch, tagGateLockedIn, tagGateHeight,
		tagEra3LockedIn, tagEra3Height, tagEpochStart, tagEra4LockedIn, tagEra4Height,
	}
}

// prettyTag renders a field tag (which ends in a NUL) as a readable name for subtest output.
func prettyTag(tag string) string {
	if n := len(tag); n > 0 && tag[n-1] == 0 {
		return tag[:n-1]
	}
	return tag
}
