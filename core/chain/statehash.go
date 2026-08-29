package chain

import (
	"github.com/nerolabs/silt/core/statehash"
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

	// era-4 (v5) field tags — DEFINED here in 4a, but NOT yet in stateRootTags and
	// NOT yet emitted by stateRootLeaves. In 4a these keyspaces are RESERVED, not
	// committed: the maintenance maps they tag (qualified, the due-bucket index) and
	// the frozen epochStart marker do not exist on Chain until 4b, and no leaf is
	// emitted under these tags until 4b commits them as v5-only. Adding them to
	// stateRootTags now would redden the coverage guards (a tag must map to a
	// committedSet struct field and emit a populated leaf) — that wiring lands in 4b
	// with the fields and leaf loops, which is the only point classification can go
	// red the correct way. Reserving the tag strings here fixes the on-wire byte
	// layout (tag || rawKey) before any leaf uses it, so 4b cannot silently pick a
	// colliding or re-ordered prefix. See
	// docs/thinking/2026-08-29-era4-build-decomposition-options.md, increment 4a.
	tagDueBucket  = "dueBucket\x00"
	tagQualified  = "qualified\x00"
	tagEpochStart = "epochStart\x00"
)

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
