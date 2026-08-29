package chain

import (
	"bytes"
	"encoding/binary"
	"sort"

	"github.com/nerolabs/silt/core/statehash"
	"github.com/nerolabs/silt/core/translog"
	"github.com/nerolabs/silt/ports"
)

// era-3 committed state root — build step 1 (certified sequence, choice 5 step 2).
//
// This file marshals the 18 committedSet fields of Chain into field-tagged,
// canonically-encoded SMT leaves and computes the StateRoot. It lives in package
// chain because it reads the unexported committed fields directly (the same reason
// the keystone oracles do). The SMT mechanics and the value encoders live in
// core/statehash, which carries no dependency on chain.
//
// STOP boundary (step 1): this computes a root. It does NOT add a Block field,
// change Hash(), add a validity predicate, or touch BlockVersion/versionSupported.
// Those re-trigger certification and are later steps.
//
// The per-field-class value encoding is pinned in
// docs/thinking/2026-08-28-era3-state-root-value-encoding.md and certified by
// era3-committed-state-root-format-RESEARCH-CERTIFICATION-2026-08-28.md (Q2/Q3/Q6).
// The field tags are exactly the 18 committedSet field names classified in
// modelcheck_state_completeness_test.go:81-96. revLog (committedLog -> its own
// LogRoot) and epochStart (observable -> no root) are deliberately excluded.

// State-root field tags. Each is a committedSet field name followed by a single
// NUL, making the tag || rawKey concatenation injective across all fields and the
// scalar reserved keys (research cert Q3). The set is EXACTLY the 18 committedSet
// fields — a field added to Chain fails the completeness guard until classified,
// and a committedSet field added here without a tag fails stateRootLeaves' coverage
// assertion (both are enforced, not asserted by inspection).
const (
	tagByRoot         = "byRoot\x00"
	tagSpent          = "spent\x00"
	tagRevoked        = "revoked\x00"
	tagSlashed        = "slashed\x00"
	tagValidatorsSeen = "validatorsSeen\x00"
	tagBonded         = "bonded\x00"
	tagEpochSet       = "epochSet\x00"
	tagBondRootOwner  = "bondRootOwner\x00"
	tagBondRootProven = "bondRootProven\x00"
	tagBondRegHeight  = "bondRegHeight\x00"
	tagRegVersion     = "regVersion\x00"
	tagBondDomain     = "bondDomain\x00"
	// Scalar reserved-key tags (leaf key = tag || ""). Cannot collide with any map
	// keyspace: map raw keys are non-empty 32-byte NodeIDs/Hashes or non-empty
	// serials, so the empty raw key is unique per scalar tag.
	tagEverMature   = "everMature\x00"
	tagMatureEpoch  = "matureEpoch\x00"
	tagGateLockedIn = "gateLockedIn\x00"
	tagGateHeight   = "gateHeight\x00"
	tagEra3LockedIn = "era3LockedIn\x00"
	tagEra3Height   = "era3Height\x00"

	// era-4 (v5) field tags — reserved in 4a, COMMITTED in 4b as V5-ONLY leaves. The
	// maintenance maps they tag (qualified, the due-bucket index) and the frozen
	// epochStart scalar are emitted by stateRootLeavesV5, which appends them to the
	// era-3 leaf set. They are gated on the era: StateRoot() (the era-3 entry) still
	// emits exactly the 18 era-3 leaves, so a v4 block's committed root stays
	// byte-identical to era-3 (hazard-1). Only the v5 root computation
	// (postApplyRoots on a v5 block) includes these leaves. The on-wire byte layout
	// (tag || rawKey) was fixed in 4a so 4b cannot silently pick a colliding prefix.
	tagDueBucket  = "dueBucket\x00"
	tagQualified  = "qualified\x00"
	tagEpochStart = "epochStart\x00"
)

// stateRootTagsV5 is the era-4 (v5) committedSet field names committed ONLY under the
// v5 state root — the three maintenance-spine fields the v5 marshaller adds on top of
// the 18 era-3 fields. Bound to the live classification by
// TestStateRootV5CoversExactlyTheV5Fields so it cannot drift. These are NOT in
// stateRootTags (the era-3 set), which keeps the v4 root byte-identical to era-3.
var stateRootTagsV5 = []string{"qualified", "dueBucket", "epochStart"}

// stateRootTags is the set of committedSet field names this file commits, used by
// the oracle to assert coverage against the live classification: exactly the 18
// committedSet fields, no more, no fewer. If a committedSet field is added without
// a leaf here, the coverage guard fails.
var stateRootTags = []string{
	"byRoot", "spent", "revoked", "slashed", "validatorsSeen",
	"bonded", "epochSet", "bondRootOwner", "bondRootProven",
	"bondRegHeight", "regVersion", "bondDomain",
	"everMature", "matureEpoch", "gateLockedIn", "gateHeight",
	"era3LockedIn", "era3Height",
}

// stateRootLeaves marshals the 18 committedSet fields into canonically-encoded,
// field-tagged leaves. The result is a pure function of the committed set: two
// nodes with the same logical state produce byte-identical leaves regardless of map
// iteration order (Go map order is random; the SMT root is order-invariant, so the
// leaf SET is what matters, not the slice order).
func (c *Chain) stateRootLeaves() []statehash.Leaf {
	var leaves []statehash.Leaf
	add := func(tag string, rawKey, value []byte) {
		leaves = append(leaves, statehash.Leaf{Key: statehash.Key(tag, rawKey), Value: value})
	}

	// ---- Class A: set-membership (value = Present marker) ----
	for root := range c.byRoot {
		add(tagByRoot, root[:], statehash.Present)
	}
	for serial := range c.spent {
		add(tagSpent, []byte(serial), statehash.Present)
	}
	for root := range c.revoked {
		add(tagRevoked, root[:], statehash.Present)
	}
	for id := range c.slashed {
		add(tagSlashed, id[:], statehash.Present)
	}
	for id := range c.validatorsSeen {
		add(tagValidatorsSeen, id[:], statehash.Present)
	}

	// ---- Class B: value-carrying (value = canonical encoding of the value) ----
	for id, w := range c.bonded {
		add(tagBonded, id[:], statehash.EncodeInt64(w))
	}
	for id, w := range c.epochSet {
		add(tagEpochSet, id[:], statehash.EncodeInt64(w))
	}
	for root, owner := range c.bondRootOwner {
		add(tagBondRootOwner, root[:], statehash.EncodeID(owner))
	}
	for root, proven := range c.bondRootProven {
		add(tagBondRootProven, root[:], statehash.EncodeBool(proven))
	}
	for id, h := range c.bondRegHeight {
		add(tagBondRegHeight, id[:], statehash.EncodeUint64(h))
	}
	for id, v := range c.regVersion {
		add(tagRegVersion, id[:], statehash.EncodeUint8(v))
	}
	for id, d := range c.bondDomain {
		add(tagBondDomain, id[:], statehash.EncodeUint64(d))
	}

	// ---- Class C: scalars (one leaf each at a reserved key) ----
	add(tagEverMature, nil, statehash.EncodeBool(c.everMature))
	add(tagMatureEpoch, nil, statehash.EncodeBool(c.matureEpoch))
	add(tagGateLockedIn, nil, statehash.EncodeBool(c.gateLockedIn))
	add(tagGateHeight, nil, statehash.EncodeUint64(c.gateHeight))
	add(tagEra3LockedIn, nil, statehash.EncodeBool(c.era3LockedIn))
	add(tagEra3Height, nil, statehash.EncodeUint64(c.era3Height))

	return leaves
}

// StateRoot computes the era-3 committed state SMT root over the 18 committedSet
// fields. It is a pure function of the committed set, identical whether the state
// was reached by replay or by snapshot boot (the property the keystone rests on).
// Step 1: computed and proven deterministic behind the oracles; not yet a Block
// field, not yet validated against.
func (c *Chain) StateRoot() (ports.Hash, error) {
	return statehash.Root(c.stateRootLeaves())
}

// stateRootLeavesV5 is the era-4 (v5) committed leaf set: the 18 era-3 leaves PLUS the
// three maintenance-spine keyspaces (qualified, the due-bucket index, the epochStart
// scalar). It is emitted ONLY for a v5 root computation (StateRootForVersion on a v5
// block); the era-3 marshaller (stateRootLeaves) is untouched, so a v4 block's root is
// byte-identical to era-3 (hazard-1). Like the era-3 leaves it is a pure function of
// the committed set: the qualified leaves are order-free (one leaf per member); the
// due-bucket leaf value is the MTH over the CANONICAL sorted id list, so it is
// independent of Go map iteration order.
func (c *Chain) stateRootLeavesV5() []statehash.Leaf {
	leaves := c.stateRootLeaves()
	add := func(tag string, rawKey, value []byte) {
		leaves = append(leaves, statehash.Leaf{Key: statehash.Key(tag, rawKey), Value: value})
	}

	// qualified: one value-carrying leaf per member (weight = bonded size), the same
	// shape as bonded/epochSet.
	for id, w := range c.qualified {
		add(tagQualified, id[:], statehash.EncodeInt64(w))
	}

	// dueBucket: one leaf per occupied due-height, key = uint64BE(height), value =
	// RFC-6962 MTH over the CANONICAL (sorted-ascending / dedup / unpadded) id list.
	// The canonical order makes the committed value independent of map iteration order
	// and forecloses MTH malleability (design §4b; RECERT2 canonical-list pin). Empty
	// buckets never occur — dueBucketRemove deletes a bucket once its last id leaves.
	for h, ids := range c.dueBucket {
		var key [8]byte
		binary.BigEndian.PutUint64(key[:], h)
		add(tagDueBucket, key[:], dueBucketMTH(ids))
	}

	// epochStart: one scalar leaf (O-1).
	add(tagEpochStart, nil, statehash.EncodeUint64(c.epochStart))

	return leaves
}

// dueBucketMTH commits a due-height bucket's id set as the RFC-6962 MTH over the
// CANONICAL id list: sorted ascending by raw NodeID bytes, deduplicated (a set is
// already unique), unpadded. The canonical order is what makes "recompute to this
// bucket root" uniquely identify the set — two encodings of the same set cannot hash
// to different roots (the malleability seam RECERT2 closes). The leaf entries are the
// raw 32-byte NodeIDs; translog.MTH is the one audited RFC-6962 implementation, reused
// here rather than re-derived.
func dueBucketMTH(ids map[ports.NodeID]struct{}) []byte {
	entries := make([]ports.Hash, 0, len(ids))
	for id := range ids {
		entries = append(entries, ports.Hash(id))
	}
	sort.Slice(entries, func(i, j int) bool {
		return bytes.Compare(entries[i][:], entries[j][:]) < 0
	})
	root := translog.MTH(entries)
	return root[:]
}

// StateRootForVersion computes the committed state root for a block of the given
// version: the era-4 (v5) leaf set for v5+, the era-3 leaf set otherwise. This is the
// era-gate that keeps a v4 block's root byte-identical to era-3 while a v5 block
// commits the maintenance-spine keyspaces. postApplyRoots selects by the block's
// version, so the recompute a validator runs matches the era of the block it checks.
func (c *Chain) StateRootForVersion(version uint64) (ports.Hash, error) {
	if version >= BlockVersionWitnessable {
		return statehash.Root(c.stateRootLeavesV5())
	}
	return statehash.Root(c.stateRootLeaves())
}

// LogRoot is the era-3 transparency-log root: the existing RFC-6962 MTH over
// revLog. It is REUSED, not reimplemented — revLog is history-dependent, so it
// stays an ordered CT root and never becomes an SMT leaf (#597). Exposed here
// alongside StateRoot so the two-root shape reads from one place; RevocationLogRoot
// remains the canonical accessor.
func (c *Chain) LogRoot() ports.Hash { return c.RevocationLogRoot() }

// newV4BlockWithRoots constructs an era-3 (v4) block that carries this chain's
// committed StateRoot and LogRoot. It is the step-2a population WIRING: it proves a
// well-formed v4 block CAN be built with the correct roots, without minting v4 by
// default (production minting stays BlockVersionRounds until step 2c height-gates the
// flip). It is unexported and used by the schema oracle; the propose path does not call
// it in 2a. The roots are non-zero constants for any era-3 chain (empty-state SMT /
// sha256("") log), so omitempty never drops them from Hash.
func (c *Chain) newV4BlockWithRoots(height uint64, prev ports.Hash, entries []ports.Entry) Block {
	sr, err := c.StateRoot()
	if err != nil {
		// StateRoot marshals our own committed fields under a fixed encoding; it cannot
		// fail for a well-formed chain. A duplicate-key error here is a marshalling bug,
		// surfaced loudly rather than committing a wrong root.
		panic(err)
	}
	lr := c.LogRoot()
	return Block{
		Version:   BlockVersionStateRoot,
		Height:    height,
		Prev:      prev,
		Entries:   entries,
		StateRoot: &sr,
		LogRoot:   &lr,
	}
}
