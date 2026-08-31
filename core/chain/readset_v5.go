package chain

import (
	"crypto/ed25519"
	"encoding/binary"

	"github.com/nerolabs/silt/core/statehash"
	"github.com/nerolabs/silt/ports"
)

// era-4 (v5) witness read-set producer — lane-1 Part A.
//
// For a v5 block, WitnessReadSetV5 emits the WITNESS read-set as a
// []statehash.ReadEntry: the set of committed-state keys the v5 WITNESSABLE
// RECOMPUTE reads to trustlessly re-derive the post-state root. A semi-stateless
// floor box holds the two committed roots (StateRoot/LogRoot), not the tree; it
// fetches one SMT witness per read-set key and verifies each against the committed
// StateRoot (the R4 accessor, core/statehash/witness.go). This producer computes
// exactly which keys the box must witness for a given v5 block.
//
// CERTIFIED IDENTITY (do NOT re-derive it — AMENDED cert
// era4-witness-floor-box-readset-v5-AMENDED-RESEARCH-CERTIFICATION-2026-08-30, the
// per-leaf read-membership table over the 23 committed v5 keyspaces):
//
//	read-set = validity reads
//	         ∪ apply() branch reads (slashed / bondRootOwner / bondRootProven / bonded /
//	           bondRegHeight / regVersion / bondDomain + qualified maintenance)
//	         ∪ THE ATTESTATION LOOP (per attester in b.Atts: slashed[id] +
//	           qualification-set membership + the validatorsSeen[id] write-target)
//	         ∪ THE MATURITY LATCH (everMature pre-state + Mature() inputs:
//	           validatorsSeen in legacy mode, or bonded/bondDomain/C2Metric +
//	           matureEpoch in objective mode)
//	         ∪ THE COMMITTED SCALAR LEAVES (epochStart / era4LockedIn / era4Height /
//	           matureEpoch / gateLockedIn / gateHeight / era3LockedIn / era3Height —
//	           each gated on its own pre-state in apply()/rotateEpoch)
//	         ∪ era-4 accelerator reads (the single dueBucket[h] NON-MEMBERSHIP leaf
//	           on a TTL-firing height + the O(RegCap) boundary frozen-set read).
//
// (The prior cert's identity — validity ∪ branch reads ∪ accelerator only — was
// INCOMPLETE: it omitted the attestation loop, the maturity latch, and the scalars.
// A floor box witnessing only that subset can be made to accept a forged block on
// validatorsSeen/everMature/any scalar. The amended cert closes the identity; this
// producer implements it and the execution-derived guard proves it against the real
// v5 recompute — readset_v5_drift_test.go.)
//
// THREE block classes:
//   - ordinary (no TTL firing, non-boundary): O(payload) (the atts loop adds one
//     read-group per attester, bounded by the quorum ⊆ RegCap);
//   - TTL-firing: O(payload), INCLUDING the empty-dueBucket[h] non-membership case
//     (the whole era-4 win: one QueryAbsent leaf discharges "nothing else expired");
//   - epoch-boundary: O(RegCap) — the three activation tallies read regVersion and
//     weight over the WHOLE frozen set, so the boundary READ-set scales with the
//     frozen-set size (= RegCap), NOT the boundary delta. Box-fits at RegCap=256.
//
// THE SHARP HAZARD (cert §"Sub-question 2", the single sharpest build hazard):
// this producer targets the BOUNDED WITNESSABLE RECOMPUTE, NOT apply()'s literal
// reads. apply() still scans the WHOLE bondRegHeight map every block (the TTL sweep,
// chain.go:3272) and ranges the whole frozen set at the boundary. Instrumenting
// apply()'s reads would yield the O(registry) set and DEFEAT era-4. So the producer
// is payload-DRIVEN: it walks the block's transitions and emits the O(1)-per-transition
// keys they read, plus the bounded accelerator keys — it NEVER ranges bondRegHeight.
// The TTL "nothing else expired" completeness claim collapses to ONE dueBucket[h]
// leaf (QueryAbsent when empty; the committed member list when non-empty), never a
// per-id scan.
//
// PRE-APPLY STATE. The producer reads THIS chain's committed state (the state the
// floor box holds before applying b) to decide, per transition, what the recompute
// reads and whether a key is present or absent. This is the same state the box
// witnesses against the committed StateRoot of the PARENT block; the read-set kinds
// (QueryPresent/QueryAbsent) are the claims the recompute needs proven about that
// state. A duplicate key across transitions is deduplicated by the shape gate's
// exactly-once contract, so the producer emits a deduplicated set.
//
// SCOPE (Part A): this is the read-set PRODUCER only. It does NOT wire
// IngestBlockWitnesses into acceptance (Part B), does NOT decide the #535 boundary
// policy (Part B), and changes NO consensus rule or validity predicate. The v5
// witnessable recompute's SOUNDNESS (that re-deriving from these witnesses yields the
// committed root) is Part B; Part A produces the read-set and proves it stays in sync
// with the recompute's defined reads (the R3 drift guard, readset_v5_drift_test.go).

// WitnessReadSetV5 returns the v5 witness read-set for block b against this chain's
// pre-apply committed state: the deduplicated set of committed-state keys the v5
// witnessable recompute reads. It is defined only for v5+ blocks; for a sub-v5 block
// it returns nil (a v4/era-3 block commits no maintenance-spine keyspaces and its
// witness story is the era-3 read-set, out of this producer's scope).
//
// Bounded by construction: ordinary/TTL-firing blocks are O(payload); the boundary is
// O(boundary-delta). No branch here ranges a whole committed map keyed by registry
// size — the TTL completeness is one dueBucket[h] leaf, not a bondRegHeight scan.
func (c *Chain) WitnessReadSetV5(b Block) []statehash.ReadEntry {
	if b.Version < BlockVersionWitnessable {
		return nil
	}
	acc := newReadSetAcc()

	// ---- (1) validity + apply() branch reads, per transition, O(1) each ----
	c.readSetEntries(b, acc)   // publish: byRoot absent, spent absent (when a token rides)
	c.readSetTakedowns(b, acc) // revoke: byRoot present; unrevoke: revoked present
	c.readSetBondRegs(b, acc)  // reg: slashed/bondRootOwner/bondRootProven/bonded/bondRegHeight + qualified/dueBucket delta
	c.readSetSlashes(b, acc)   // slash: slashed write, bonded delete, qualified maintain

	// ---- (2) the attestation loop (apply:3293-3298) ----
	// Per attester in b.Atts that is a qualified non-proposer, the recompute reads the
	// attesterQualified inputs (slashed[id]; the qualification-set membership) and the
	// validatorsSeen[id] write-target. This fires on essentially every real block. The
	// prior build OMITTED it — the amended cert's confirmed completeness gap.
	c.readSetAtts(b, acc)

	// ---- (3) the maturity latch (apply:3303-3305) ----
	// everMature pre-state + the Mature() inputs the latch gates on: the validatorsSeen
	// set (legacy mode) or the C2Metric inputs bonded/bondDomain + the matureEpoch
	// branch-selector (objective mode). The prior build OMITTED it.
	c.readSetMaturityLatch(b, acc)

	// ---- (4) the committed scalar leaves ----
	// Each of epochStart/era4LockedIn/era4Height/matureEpoch/gateLockedIn/gateHeight/
	// era3LockedIn/era3Height is committed and gated on its own pre-state in apply()/
	// rotateEpoch (a monotonic-write guard or an unconditional marker), so the recompute
	// reads each. The prior build OMITTED these entirely.
	c.readSetScalars(acc)

	// ---- (5) era-4 accelerator: the TTL completeness leaf ----
	// The TTL sweep's "nothing else expired at height h" claim is discharged by ONE
	// dueBucket[h] leaf, NOT a bondRegHeight scan (the era-4 win). If the bucket is
	// occupied at h the recompute reads its committed member list (each member's
	// bonded/bondRegHeight/regVersion/qualified leaves are the expiry delta); if the
	// bucket is empty the recompute needs a single NON-MEMBERSHIP proof of dueBucket[h].
	c.readSetTTLCompleteness(b, acc)

	// ---- (6) era-4 accelerator: the epoch-boundary frozen-set read ----
	// At a boundary (epochsEnabled && h % EpochBlocks == 0) the recompute freezes
	// qualified into epochSet and reads regVersion + weight over the WHOLE frozen set
	// for the three activation tallies — O(RegCap), the heavier class (NOT the delta).
	c.readSetBoundaryDelta(b, acc)

	// ---- (7) trustless recompute increment 1: the epochSetRoot completeness leaf ----
	// The root-only recompute of requireEpochWeightQuorum (floorbox_recompute_v5.go) proves
	// SET-COMPLETENESS of the frozen epochSet by reconstructing the committed epochSetRoot
	// digest from the witnessed id-list. So the box must witness the epochSetRoot leaf itself
	// (a presence proof of the committed MTH). One leaf, O(1) — the per-member epochSet weights
	// (the C-1 composition) are already emitted by readSetBoundaryDelta and readSetAtts. F1
	// committed this root inert; increment 1 makes it a genuine read.
	c.readSetEpochSetRoot(acc)

	// ---- (8) trustless recompute increment 2: the validatorsSeenRoot completeness leaf ----
	// The root-only recompute of matureNow (the maturity-latch metric, floorbox_recompute_maturity_v5.go)
	// proves SET-COMPLETENESS of validatorsSeen by reconstructing the committed validatorsSeenRoot
	// digest from the witnessed id-list. So the box must witness the validatorsSeenRoot leaf itself
	// (a presence proof of the committed MTH), plus each member's slashed/bonded/bondDomain leaves
	// (the C2Metric per-member inputs, the C-1 composition). F1 committed this root inert;
	// increment 2 makes it a genuine read.
	c.readSetValidatorsSeenRoot(acc)

	return acc.entries()
}

// readSetValidatorsSeenRoot emits the reads the root-only recompute of matureNow performs
// (floorbox_recompute_maturity_v5.go), the C2Metric composition:
//
//   - the validatorsSeenRoot DIGEST leaf (set-completeness): the recompute reconstructs the
//     validatorsSeen set's committed MTH from the witnessed id-list and compares it to this
//     committed leaf. One omitted member ⇒ a different MTH ⇒ stall. An empty validatorsSeen
//     commits the fixed empty-MTH constant (C-4 always-emit); the fold is then degenerate.
//   - EVERY validatorsSeen MEMBER's slashed / bonded / bondDomain leaves (C-1): the digest binds
//     MEMBERSHIP only — the operator/domain-distinct coefficient is forgeable without a per-member
//     value proof for each id. So the box must witness every member's committed state to fold
//     C2Metric. O(RegCap), bounded by the seen set (box-fits at RegCap=256).
//
// This fires whenever validatorsSeen is non-empty (whenever the maturity latch has a set to fold),
// NOT only when the latch is not yet set — the de-mature super-quorum gate (chain.go:2827) runs
// matureNow in ValidateCommit on every mature block AFTER the latch is set, so the recompute reads
// the whole seen set every block. The per-member slashed/bonded/bondDomain leaves emitted here
// overlap (dedup) with readSetAtts's and readSetMaturityLatch's reads.
func (c *Chain) readSetValidatorsSeenRoot(acc *readSetAcc) {
	// The digest-root completeness leaf (membership commitment). ALWAYS emitted (C-4 always-emit:
	// an empty validatorsSeen commits the fixed empty-MTH constant), because the digest root is a
	// committed leaf on every v5 block AND a write-target where validatorsSeen mutates — the box
	// must witness its pre-state either way.
	acc.addScalar(tagValidatorsSeenRoot, nodeSetMTHFromBool(c.validatorsSeen))
	// Per-member: the validatorsSeen[id] MEMBERSHIP leaf (the set-completeness reconstruction reads
	// each member's presence to rebuild the MTH) + the C2Metric inputs (C-1): slashed membership,
	// bonded weight, bondDomain domain. One (non-)inclusion proof each. Empty when the seen set is
	// empty (degenerate metric).
	for id := range c.validatorsSeen {
		acc.addPresent(tagValidatorsSeen, id[:], statehash.Present)
		if c.slashed[id] {
			acc.addPresent(tagSlashed, id[:], statehash.Present)
		} else {
			acc.addAbsent(tagSlashed, id[:])
		}
		c.addBondedRead(acc, id)
		c.addBondDomainRead(acc, id)
	}
}

// readSetEpochSetRoot emits the reads the root-only recompute of requireEpochWeightQuorum
// performs (floorbox_recompute_v5.go), the C-1 composition:
//
//   - the epochSetRoot DIGEST leaf (set-completeness): the recompute reconstructs the frozen
//     epochSet's committed MTH from the witnessed id-list and compares it to this committed
//     leaf. One omitted member ⇒ a different MTH ⇒ stall. An empty epochSet commits the fixed
//     empty-MTH constant (C-4 always-emit); the fold is then degenerate (total <= 0).
//   - EVERY epochSet MEMBER's weight leaf (C-1): the digest binds MEMBERSHIP only — the weight
//     tally is forgeable without a per-member value proof for each id. So the box must witness
//     every epochSet[id] weight leaf to fold Σ epochSet. O(RegCap), the whole-set weight fold
//     the frozen quorum needs (bounded by the frozen-set size, box-fits at RegCap=256).
//
// This fires whenever epochSet is non-empty (whenever the weight quorum has a set to fold), NOT
// only at a boundary — requireEpochWeightQuorum runs in ValidateCommit on every mature block,
// so the recompute reads the whole frozen set every block, not just when it is re-frozen. The
// per-member epochSet leaves emitted here overlap (dedup) with readSetBoundaryDelta's and
// readSetAtts's epochSet reads.
func (c *Chain) readSetEpochSetRoot(acc *readSetAcc) {
	// The digest-root completeness leaf (membership commitment). ALWAYS emitted (C-4 always-emit:
	// an empty epochSet commits the fixed empty-MTH constant), because the digest root is a
	// committed leaf on every v5 block AND a write-target at a boundary where epochSet is (re)frozen
	// — the box must witness its pre-state either way. When epochSet is empty the reconstructed MTH
	// is the empty-MTH and the weight fold is degenerate (total <= 0).
	acc.addScalar(tagEpochSetRoot, nodeSetMTHFromInt64(c.epochSet))
	// The per-member weight leaves (C-1): the values the fold sums, one inclusion proof each. Empty
	// when the frozen set is empty (degenerate quorum), O(frozen-set) otherwise.
	for id := range c.epochSet {
		c.addEpochSetRead(acc, id)
	}
}

// readSetAcc accumulates read-set entries, deduplicating by (key, kind). The shape
// gate downstream (IngestBlockWitnesses) requires EXACTLY one proof per read key, so
// the producer must not emit a key twice. A key read as both present and absent by
// two transitions cannot occur for a single committed key in one block's recompute
// (a key is present or absent in the pre-apply state, not both); if two transitions
// name the same key with the same kind, one entry suffices.
type readSetAcc struct {
	seen  map[string]struct{}
	items []statehash.ReadEntry
}

func newReadSetAcc() *readSetAcc {
	return &readSetAcc{seen: make(map[string]struct{})}
}

// addAbsent emits a QueryAbsent read of a field-tagged committed key: the recompute
// needs a NON-MEMBERSHIP proof that this key is absent from the committed keyspace.
func (a *readSetAcc) addAbsent(tag string, rawKey []byte) {
	a.add(statehash.ReadEntry{Key: statehash.Key(tag, rawKey), Kind: statehash.QueryAbsent})
}

// addPresent emits a QueryPresent read of a field-tagged committed key with the
// expected committed value: the recompute needs a MEMBERSHIP proof of (key → value).
// The value MUST be non-empty (the shape gate rejects a QueryPresent with an empty
// value), so a caller that cannot supply the committed value must use addAbsent.
func (a *readSetAcc) addPresent(tag string, rawKey, value []byte) {
	a.add(statehash.ReadEntry{Key: statehash.Key(tag, rawKey), Kind: statehash.QueryPresent, Value: value})
}

// addScalar emits a QueryPresent read of a scalar reserved-key leaf (rawKey = empty):
// a committed scalar is ALWAYS present (one leaf at Key(tag)), and the recompute reads
// its pre-state to reproduce the monotonic-write guard, so the box always witnesses it
// present with its committed value. The value MUST be non-empty; the scalar encoders
// (EncodeBool/EncodeUint64) return a single byte even for the zero value, so this is safe.
func (a *readSetAcc) addScalar(tag string, value []byte) {
	a.add(statehash.ReadEntry{Key: statehash.Key(tag, nil), Kind: statehash.QueryPresent, Value: value})
}

func (a *readSetAcc) add(e statehash.ReadEntry) {
	// Dedup on the key alone: a committed key has one committed state (present with a
	// value, or absent), so two reads of the same key in one block's recompute agree
	// on kind. The FIRST-seen entry wins; a later duplicate is dropped.
	if _, dup := a.seen[string(e.Key)]; dup {
		return
	}
	a.seen[string(e.Key)] = struct{}{}
	a.items = append(a.items, e)
}

func (a *readSetAcc) entries() []statehash.ReadEntry { return a.items }

// readSetEntries emits the publish-validity reads for each block entry: byRoot[root]
// ABSENT (the dup-root check, chain.go:2591) and, when a token rides, spent[serial]
// ABSENT (the double-spend check, chain.go:2617). Both are absence claims (the entry
// is valid only if the root/serial is not already committed). O(len(Entries)).
func (c *Chain) readSetEntries(b Block, acc *readSetAcc) {
	for i := range b.Entries {
		e := b.Entries[i]
		acc.addAbsent(tagByRoot, e.Root[:])
		if e.Token != nil {
			acc.addAbsent(tagSpent, []byte(e.Token.Serial))
		}
	}
}

// readSetTakedowns emits the revocation-validity reads: a revocation requires
// byRoot[root] PRESENT (revoke only committed content, chain.go:2638); an
// un-revocation requires revoked[root] PRESENT (chain.go:2643). Both are presence
// claims. The byRoot value is the committed Entry — but the floor box does not carry
// the Entry bytes to encode the membership value, and the validity predicate reads
// only EXISTENCE, not the value. The recompute proves existence, which the R4
// accessor models as a QueryPresent with the committed value; where the producer
// cannot supply the committed value it emits QueryAbsent-complement... see note.
//
// A byRoot presence read here uses the committed Entry as the value ONLY if this
// chain holds it (it does — the producer runs against the committed state). O(payload).
func (c *Chain) readSetTakedowns(b Block, acc *readSetAcc) {
	for _, r := range b.Revocations {
		if e, ok := c.byRoot[r]; ok {
			acc.addPresent(tagByRoot, r[:], encodeEntryPresence(e))
		} else {
			// The root is not committed on this chain: the revocation is invalid and the
			// recompute rejects it. The read is still a presence CLAIM the box must be
			// able to disprove — modelled as the presence read whose witness will fail to
			// verify (the box then stalls/rejects). Emit the presence key with the marker.
			acc.addPresent(tagByRoot, r[:], statehash.Present)
		}
	}
	for _, r := range b.Unrevocations {
		acc.addPresent(tagRevoked, r[:], statehash.Present)
	}
}

// encodeEntryPresence returns the committed leaf value for a byRoot membership read.
// byRoot is a Class-A set-membership field (statehash.go:115): its committed leaf
// value is the fixed Present marker, never the Entry bytes. The predicate reads
// existence, so the membership proof is of (Key(tagByRoot, root) → Present).
func encodeEntryPresence(_ ports.Entry) []byte { return statehash.Present }

// readSetBondRegs emits the bond-registration reads for each canonical reg in the
// block: the displacement-branch reads (bondRootOwner[root], bondRootProven[root]),
// the slashed[id] gate, the bonded[id] write-target, the bondRegHeight[id] TTL-clock
// read (for the dueBucketMoveOnReg old-bucket derivation), and the qualified[id]
// write-target. It uses canonicalBondRegs so the read-set matches the reg set apply()
// actually processes (the same canonicalization the recompute runs). O(len(BondRegs)).
//
// NOTE: this reads bondRootOwner/bondRootProven — the apply() BRANCH reads the cert
// names (the displacement branch, chain.go:3239-3253) — NOT the whole bondRegHeight
// map. bondRegHeight[id] is read ONLY for the ids named in the block, one key each,
// to derive the old due-bucket on a renew. This is O(payload), never O(registry).
func (c *Chain) readSetBondRegs(b Block, acc *readSetAcc) {
	for _, r := range canonicalBondRegs(b.BondRegs) {
		if len(r.Validator) != ed25519.PublicKeySize {
			continue
		}
		if r.Size < c.cfg.MinBondBytes {
			continue // below the objective floor: no standing, no state read
		}
		id := r.ValidatorID()

		// slashed[id]: the gate (chain.go:3236) — a slashed id earns nothing.
		if c.slashed[id] {
			acc.addPresent(tagSlashed, id[:], statehash.Present)
			continue
		}
		acc.addAbsent(tagSlashed, id[:])

		// bondRootOwner[root] / bondRootProven[root]: the displacement-branch reads
		// (chain.go:3239/3245). The recompute reads the current owner and proven flag to
		// decide whether this reg displaces a squatter.
		if owner, claimed := c.bondRootOwner[r.Root]; claimed {
			acc.addPresent(tagBondRootOwner, r.Root[:], statehash.EncodeID(owner))
			if pv, ok := c.bondRootProven[r.Root]; ok {
				acc.addPresent(tagBondRootProven, r.Root[:], statehash.EncodeBool(pv))
			} else {
				acc.addAbsent(tagBondRootProven, r.Root[:])
			}
			if owner != id {
				// Displacement: the recompute reads bonded[owner] and qualified[owner] to
				// strip the displaced squatter (chain.go:3248-3249).
				c.addBondedRead(acc, owner)
				c.addQualifiedRead(acc, owner)
			}
		} else {
			acc.addAbsent(tagBondRootOwner, r.Root[:])
			acc.addAbsent(tagBondRootProven, r.Root[:])
		}

		// bondRegHeight[id]: read for the OLD due-bucket derivation on a renew
		// (dueBucketMoveOnReg, chain.go:1399). ONE key per named id, never the whole map.
		// It is BOTH a read (the old bucket) and a write-target (reset to h, chain.go:3261).
		oldReg, hadOldReg := c.bondRegHeight[id]
		if hadOldReg {
			acc.addPresent(tagBondRegHeight, id[:], statehash.EncodeUint64(oldReg))
		} else {
			acc.addAbsent(tagBondRegHeight, id[:])
		}

		// The due-bucket writes the reg performs (dueBucketMoveOnReg, chain.go:1394-1402):
		// on a renew it REMOVES id from the OLD bucket (oldReg+ttl+1), and it always INSERTS
		// id at the NEW bucket (b.Height+ttl+1). Both buckets are committed dueBucket leaves
		// the recompute writes — the box must witness each. Bounded: at most two keys per reg.
		if ttl := c.cfg.BondTTLBlocks; ttl > 0 {
			c.addDueBucketRead(acc, b.Height+ttl+1)
			if hadOldReg {
				c.addDueBucketRead(acc, oldReg+ttl+1)
			}
		}

		// bonded[id] / qualified[id] / regVersion[id] / bondDomain[id]: the write-targets the
		// recompute reads to compute the post-write leaves (chain.go:3260-3264). regVersion and
		// bondDomain are committed v5 leaves the reg overwrites; qualifiedMaintain reads
		// bonded/slashed (chain.go:1379).
		c.addBondedRead(acc, id)
		c.addQualifiedRead(acc, id)
		c.addRegVersionRead(acc, id)
		c.addBondDomainRead(acc, id)
	}
}

// readSetSlashes emits the slash reads: slashed[culprit] (write-target, chain.go:3287),
// bonded[culprit] (evicted, chain.go:3288), qualified[culprit] (maintained,
// chain.go:3289). O(len(Slashes)).
func (c *Chain) readSetSlashes(b Block, acc *readSetAcc) {
	for i := range b.Slashes {
		culprit := b.Slashes[i].CulpritID()
		c.addBondedRead(acc, culprit)
		c.addQualifiedRead(acc, culprit)
		// slashed[culprit] is a write; the recompute reads its pre-state to compute the
		// post-write leaf. Model as the pre-apply presence/absence.
		if c.slashed[culprit] {
			acc.addPresent(tagSlashed, culprit[:], statehash.Present)
		} else {
			acc.addAbsent(tagSlashed, culprit[:])
		}
	}
}

// readSetAtts emits the attestation-loop reads (apply:3293-3298). For each attester in
// b.Atts that is NOT the proposer, the recompute evaluates attesterQualified(id) and, if
// qualified, writes validatorsSeen[id]. It therefore reads, per attester:
//   - slashed[id] (the F2 gate, attesterQualifiedAt:1280);
//   - the qualification-set membership: under objective+matureEpoch the effectiveEpochSet
//     membership (epochSet — attesterQualifiedAt:1297), else bonded[id]
//     (attesterQualifiedAt:1300);
//   - validatorsSeen[id], the write-target (apply:3296), read to compute the post-write
//     leaf (present iff already seen, else absent → set present).
//
// O(len(b.Atts)) — the attester set is bounded by the quorum ⊆ RegCap, so this stays
// O(payload). The read matches attesterQualified(id) = attesterQualifiedAt(id, 0): height 0
// is never a #535 recovery boundary, so effectiveEpochSet(0) is the frozen epochSet (R2 is
// the recovery-boundary residual, out of scope here). The legacy rep(id) branch reads no
// committed SMT leaf, so it contributes nothing to the committed read-set.
func (c *Chain) readSetAtts(b Block, acc *readSetAcc) {
	proposer := b.ProposerID()
	for i := range b.Atts {
		id := b.Atts[i].AttesterID()
		if id == proposer {
			continue // apply() skips the proposer's own attestation (apply:3295)
		}
		// slashed[id]: the F2 gate, read for every attester.
		if c.slashed[id] {
			acc.addPresent(tagSlashed, id[:], statehash.Present)
		} else {
			acc.addAbsent(tagSlashed, id[:])
		}
		// The qualification-set membership read (objective mode only — the legacy rep
		// branch reads no committed leaf). In a mature objective epoch the membership is
		// the frozen epochSet; otherwise it is bonded[id].
		if c.objective() {
			if c.epochsEnabled() && c.matureEpoch {
				c.addEpochSetRead(acc, id)
			} else {
				c.addBondedRead(acc, id)
			}
		}
		// validatorsSeen[id]: the write-target the recompute reads to compute the
		// post-write leaf (a Class-A set-membership leaf, statehash.go:127).
		if c.validatorsSeen[id] {
			acc.addPresent(tagValidatorsSeen, id[:], statehash.Present)
		} else {
			acc.addAbsent(tagValidatorsSeen, id[:])
		}
	}
}

// readSetMaturityLatch emits the maturity-latch reads (apply:3303-3305):
// `if !c.everMature && c.Mature() { c.everMature = true }`. The recompute reads:
//   - everMature scalar pre-state (the latch guard) — always, emitted by readSetScalars;
//   - the Mature() inputs, only when the latch is not yet set (else Mature() is not
//     evaluated, short-circuited by !everMature). In legacy mode Mature()→matureNow ranges
//     the whole validatorsSeen set; in objective mode it reads MatureCoefficient→C2Metric
//     over the whole bonded ledger AND bondDomain (the domain-distinct coefficient), plus
//     matureEpoch selects the qualification branch (emitted by readSetScalars).
//
// The everMature scalar itself is emitted by readSetScalars (it is a committed scalar leaf
// the recompute reads unconditionally). This method adds the Mature() INPUT leaves. When
// MatureValidators<=0, Mature() short-circuits true without reading the set (chain.go:2140),
// so no maturity inputs are read.
func (c *Chain) readSetMaturityLatch(b Block, acc *readSetAcc) {
	if c.everMature {
		return // latch already set: !everMature short-circuits, Mature() is not evaluated
	}
	if c.cfg.MatureValidators <= 0 {
		return // Mature() returns true without reading the committed set (chain.go:2140)
	}
	if !c.objective() {
		// Legacy: matureNow ranges the whole validatorsSeen map (chain.go:2181). The
		// recompute reads each validatorsSeen member (minus anchors). Bounded by RegCap.
		for id := range c.validatorsSeen {
			acc.addPresent(tagValidatorsSeen, id[:], statehash.Present)
		}
		return
	}
	// Objective: MatureCoefficient → C2Metric ranges validatorsSeen (chain.go:2305) — NOT the
	// whole bonded ledger — and for each SEEN id reads slashed[id], bonded[id], bondDomain[id]
	// (chain.go:2306-2312). The recompute reads each validatorsSeen member and its slashed /
	// bonded / bondDomain leaves. Bounded by RegCap. (The amended cert said "over the whole
	// bonded ledger"; the SOURCE ranges validatorsSeen — the execution-derived guard pinned
	// this, which is exactly why the guard is ground-truth, not a mirror of the cert prose.)
	for id := range c.validatorsSeen {
		acc.addPresent(tagValidatorsSeen, id[:], statehash.Present)
		if c.slashed[id] {
			acc.addPresent(tagSlashed, id[:], statehash.Present)
		} else {
			acc.addAbsent(tagSlashed, id[:])
		}
		c.addBondedRead(acc, id)
		c.addBondDomainRead(acc, id)
	}
}

// readSetScalars emits the committed scalar-leaf reads. Each scalar is committed under the
// v5 root and gated on its own pre-state in apply()/rotateEpoch (a monotonic-write guard,
// e.g. `if !c.era4LockedIn`, or an unconditional marker like epochStart), so the recompute
// reads every one of them to reproduce the post-state. They are ALWAYS present (one leaf per
// scalar at its reserved key), so each is a QueryPresent with the committed encoded value.
//
// The eight era-3 + era-4 committed scalars (statehash.go:155-160,206,211-212): everMature,
// matureEpoch, gateLockedIn, gateHeight, era3LockedIn, era3Height, epochStart, era4LockedIn,
// era4Height. The prior build emitted NONE of these — the amended cert's confirmed omission.
func (c *Chain) readSetScalars(acc *readSetAcc) {
	acc.addScalar(tagEverMature, statehash.EncodeBool(c.everMature))
	acc.addScalar(tagMatureEpoch, statehash.EncodeBool(c.matureEpoch))
	acc.addScalar(tagGateLockedIn, statehash.EncodeBool(c.gateLockedIn))
	acc.addScalar(tagGateHeight, statehash.EncodeUint64(c.gateHeight))
	acc.addScalar(tagEra3LockedIn, statehash.EncodeBool(c.era3LockedIn))
	acc.addScalar(tagEra3Height, statehash.EncodeUint64(c.era3Height))
	acc.addScalar(tagEpochStart, statehash.EncodeUint64(c.epochStart))
	acc.addScalar(tagEra4LockedIn, statehash.EncodeBool(c.era4LockedIn))
	acc.addScalar(tagEra4Height, statehash.EncodeUint64(c.era4Height))
}

// readSetTTLCompleteness emits the era-4 TTL accelerator read: the dueBucket[h] leaf
// for the block's height. This is the read that REPLACES the O(registry) bondRegHeight
// scan in the witnessable recompute (the whole era-4 win). Only when TTL is enabled
// (BondTTLBlocks > 0), matching apply()'s TTL sweep gate (chain.go:3271).
//
//   - dueBucket[h] ABSENT (empty bucket): ONE non-membership proof discharges the
//     ENTIRE "nothing expired at h" completeness claim. THIS IS THE EMPTY-BUCKET PATH,
//     the core certified win — the read-set carries one QueryAbsent leaf and reads
//     NOTHING from bondRegHeight.
//   - dueBucket[h] PRESENT (occupied): the recompute reads the committed bucket MTH,
//     and each expiring member's bonded/bondRegHeight/regVersion/qualified leaves (the
//     expiry delta). Bounded by the bucket size (RegCap-bounded, cert R1).
func (c *Chain) readSetTTLCompleteness(b Block, acc *readSetAcc) {
	if c.cfg.BondTTLBlocks == 0 {
		return
	}
	var hk [8]byte
	binary.BigEndian.PutUint64(hk[:], b.Height)

	ids, occupied := c.dueBucket[b.Height]
	if !occupied {
		// The empty-bucket non-membership path: one QueryAbsent leaf for dueBucket[h].
		acc.addAbsent(tagDueBucket, hk[:])
		return
	}
	// Occupied: the recompute reads the committed bucket MTH (a presence proof of
	// dueBucket[h] → MTH) plus each expiring member's delta leaves.
	acc.addPresent(tagDueBucket, hk[:], dueBucketMTH(ids))
	for id := range ids {
		c.addBondedRead(acc, id)
		if h, ok := c.bondRegHeight[id]; ok {
			acc.addPresent(tagBondRegHeight, id[:], statehash.EncodeUint64(h))
		} else {
			acc.addAbsent(tagBondRegHeight, id[:])
		}
		if v, ok := c.regVersion[id]; ok {
			acc.addPresent(tagRegVersion, id[:], statehash.EncodeUint8(v))
		} else {
			acc.addAbsent(tagRegVersion, id[:])
		}
		c.addQualifiedRead(acc, id)
	}
}

// readSetBoundaryDelta emits the epoch-boundary reads: at a boundary the recompute freezes
// qualified into epochSet and reads regVersion + weight over the WHOLE frozen set for the
// three activation tallies (rotateEpoch, chain.go:3442/3465/3489). The predicate
// `3*ready > 2*total` is a super-quorum over the FULL frozen weight — it CANNOT be computed
// from a symmetric-difference delta, so the recompute reads every frozen-set member. The
// boundary READ-set is therefore O(frozen-set) = O(RegCap), NOT O(boundary-delta) (the
// AMENDED cert, PE Claim 2: O(boundary-delta) is the WRITE-set — the changed leaves — not
// the read-set). Fires only at a boundary height with epochs enabled.
//
// This producer already ranges the whole qualified (the freeze source) and the whole
// epochSet (the prior frozen set), reading regVersion per qualified member — which IS the
// O(RegCap) frozen-set read. Box-fits at RegCap=256 (amended cert; kilobytes of witness).
func (c *Chain) readSetBoundaryDelta(b Block, acc *readSetAcc) {
	if !c.epochsEnabled() || c.cfg.EpochBlocks == 0 || b.Height%c.cfg.EpochBlocks != 0 {
		return
	}
	// The boundary reads the qualified accelerator (the freeze SOURCE, chain.go:3425) and,
	// for the activation tallies, regVersion + weight over the whole frozen set. The freeze
	// OVERWRITES epochSet = clone(qualified), so each qualified member is an epochSet
	// WRITE-TARGET the recompute reads. The producer emits, per qualified member: qualified[id]
	// (source), regVersion[id] (tally, chain.go:3444/3467/3491), and epochSet[id] (freeze
	// write-target). It ALSO emits the PRIOR epochSet members (write-targets the freeze may
	// remove). O(RegCap).
	//
	// The freeze is gated on everMature (rotateEpoch:3395 early-return before it sets
	// matureEpoch and freezes). This producer emits the frozen-set reads at EVERY boundary,
	// even a pre-maturity one where the freeze does not fire — over-witnessing is SOUND (a
	// superset never causes a wrong-accept), and it keeps the producer from having to predict
	// the same-block maturity latch. The execution-derived guard proves the producer covers
	// the actual freeze writes when it does fire.
	for id, w := range c.qualified {
		acc.addPresent(tagQualified, id[:], statehash.EncodeInt64(w))
		if v, ok := c.regVersion[id]; ok {
			acc.addPresent(tagRegVersion, id[:], statehash.EncodeUint8(v))
		} else {
			acc.addAbsent(tagRegVersion, id[:])
		}
		c.addEpochSetRead(acc, id) // the freeze write-target for this member
	}
	for id, w := range c.epochSet {
		acc.addPresent(tagEpochSet, id[:], statehash.EncodeInt64(w))
	}
}

// addBondedRead emits the bonded[id] read as the pre-apply presence/absence: a
// value-carrying membership proof when the id is bonded, a non-membership proof
// otherwise. bonded is Class-B (value = EncodeInt64(weight), statehash.go:132).
func (c *Chain) addBondedRead(acc *readSetAcc, id ports.NodeID) {
	if w, ok := c.bonded[id]; ok {
		acc.addPresent(tagBonded, id[:], statehash.EncodeInt64(w))
	} else {
		acc.addAbsent(tagBonded, id[:])
	}
}

// addQualifiedRead emits the qualified[id] read as the pre-apply presence/absence:
// the maintenance write-target the recompute reads to compute the post-write leaf.
// qualified is v5-only, Class-B (value = EncodeInt64(weight), statehash.go:191).
func (c *Chain) addQualifiedRead(acc *readSetAcc, id ports.NodeID) {
	if w, ok := c.qualified[id]; ok {
		acc.addPresent(tagQualified, id[:], statehash.EncodeInt64(w))
	} else {
		acc.addAbsent(tagQualified, id[:])
	}
}

// addRegVersionRead emits the regVersion[id] read as the pre-apply presence/absence: a
// write-target the reg overwrites (chain.go:3262) and the TTL sweep / boundary tally read.
// regVersion is Class-B (value = EncodeUint8, statehash.go:148).
func (c *Chain) addRegVersionRead(acc *readSetAcc, id ports.NodeID) {
	if v, ok := c.regVersion[id]; ok {
		acc.addPresent(tagRegVersion, id[:], statehash.EncodeUint8(v))
	} else {
		acc.addAbsent(tagRegVersion, id[:])
	}
}

// addBondDomainRead emits the bondDomain[id] read as the pre-apply presence/absence: a
// write-target the reg overwrites (chain.go:3263) and a C2Metric maturity input. bondDomain
// is Class-B (value = EncodeUint64, statehash.go:151).
func (c *Chain) addBondDomainRead(acc *readSetAcc, id ports.NodeID) {
	if d, ok := c.bondDomain[id]; ok {
		acc.addPresent(tagBondDomain, id[:], statehash.EncodeUint64(d))
	} else {
		acc.addAbsent(tagBondDomain, id[:])
	}
}

// addDueBucketRead emits the dueBucket[h] read as the pre-apply presence/absence: the
// committed bucket leaf (value = MTH over the canonical id list) when occupied, else a
// non-membership leaf. Used for the reg's bucket-move write-targets and the TTL sweep.
func (c *Chain) addDueBucketRead(acc *readSetAcc, h uint64) {
	var hk [8]byte
	binary.BigEndian.PutUint64(hk[:], h)
	if ids, occ := c.dueBucket[h]; occ {
		acc.addPresent(tagDueBucket, hk[:], dueBucketMTH(ids))
	} else {
		acc.addAbsent(tagDueBucket, hk[:])
	}
}

// addEpochSetRead emits the epochSet[id] membership read as the pre-apply presence/absence:
// the frozen-epoch qualification membership the atts-loop reads in a mature objective epoch
// (effectiveEpochSet membership, attesterQualifiedAt:1297). epochSet is Class-B (value =
// EncodeInt64(weight), statehash.go:135).
func (c *Chain) addEpochSetRead(acc *readSetAcc, id ports.NodeID) {
	if w, ok := c.epochSet[id]; ok {
		acc.addPresent(tagEpochSet, id[:], statehash.EncodeInt64(w))
	} else {
		acc.addAbsent(tagEpochSet, id[:])
	}
}
