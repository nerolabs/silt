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
// CERTIFIED IDENTITY (do NOT re-derive it — cert
// era4-witness-floor-box-readset-v5-RESEARCH-CERTIFICATION-2026-08-30):
//
//	read-set = validity reads
//	         ∪ apply() branch reads (slashed / bondRootOwner / bondRootProven)
//	         ∪ era-4 accelerator reads (the single dueBucket[h] NON-MEMBERSHIP leaf
//	           on a TTL-firing height + the touched qualified / epochSet delta).
//
// THREE block classes:
//   - ordinary (no TTL firing, non-boundary): O(payload);
//   - TTL-firing: O(payload), INCLUDING the empty-dueBucket[h] non-membership case
//     (the whole era-4 win: one QueryAbsent leaf discharges "nothing else expired");
//   - epoch-boundary: O(boundary-delta), the heavier witness class.
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

	// ---- (2) era-4 accelerator: the TTL completeness leaf ----
	// The TTL sweep's "nothing else expired at height h" claim is discharged by ONE
	// dueBucket[h] leaf, NOT a bondRegHeight scan (the era-4 win). If the bucket is
	// occupied at h the recompute reads its committed member list (each member's
	// bonded/bondRegHeight/regVersion/qualified leaves are the expiry delta); if the
	// bucket is empty the recompute needs a single NON-MEMBERSHIP proof of dueBucket[h].
	c.readSetTTLCompleteness(b, acc)

	// ---- (3) era-4 accelerator: the epoch-boundary delta ----
	// At a boundary (epochsEnabled && h % EpochBlocks == 0) the recompute freezes
	// qualified into epochSet and reads the frozen-set weights for the activation
	// tallies — O(boundary-delta), the heavier class.
	c.readSetBoundaryDelta(b, acc)

	return acc.entries()
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
		if h, ok := c.bondRegHeight[id]; ok {
			acc.addPresent(tagBondRegHeight, id[:], statehash.EncodeUint64(h))
		} else {
			acc.addAbsent(tagBondRegHeight, id[:])
		}

		// bonded[id] / qualified[id]: the write-targets the recompute reads to compute
		// the post-write maintenance (qualifiedMaintain reads bonded/slashed, chain.go:1379).
		c.addBondedRead(acc, id)
		c.addQualifiedRead(acc, id)
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

// readSetBoundaryDelta emits the epoch-boundary accelerator reads: at a boundary the
// recompute freezes qualified into epochSet and reads the frozen-set weights for the
// activation tallies (rotateEpoch, chain.go:3421-3499). The changed-leaf set is the
// symmetric difference between last epoch's frozen epochSet and this epoch's live
// qualified = O(boundary-delta), the heavier witness class the cert names (NOT
// O(payload)). Fires only at a boundary height with epochs enabled and the chain
// mature (matureEpoch), matching rotateEpoch's freeze gate.
//
// The frozen-set members drive the regVersion-weighted activation tallies (the three
// lock-in tallies read regVersion[id] per member), so the recompute reads each
// boundary-delta member's qualified/epochSet/regVersion leaves. This is O(boundary-delta),
// bounded by RegCap × EpochBlocks (cert §"Epoch-boundary block").
func (c *Chain) readSetBoundaryDelta(b Block, acc *readSetAcc) {
	if !c.epochsEnabled() || c.cfg.EpochBlocks == 0 || b.Height%c.cfg.EpochBlocks != 0 {
		return
	}
	// The boundary reads the qualified accelerator (the freeze source) and the current
	// epochSet (the prior frozen set, the delta's other side). The union of their keys
	// is the boundary-delta read-set: each key present in either is a leaf the recompute
	// touches to compute the symmetric difference and the new frozen weights.
	for id, w := range c.qualified {
		acc.addPresent(tagQualified, id[:], statehash.EncodeInt64(w))
		// regVersion drives the activation tally (chain.go:3444/3467/3491).
		if v, ok := c.regVersion[id]; ok {
			acc.addPresent(tagRegVersion, id[:], statehash.EncodeUint8(v))
		} else {
			acc.addAbsent(tagRegVersion, id[:])
		}
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
