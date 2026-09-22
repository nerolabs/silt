// The hash-only spot check: the shipped proof of retrievability.
//
// An auditor names sampled LEAVES of a shard; the prover returns those leaves'
// bytes together with their Merkle paths; the auditor checks each path against a
// per-shard root committed by the publisher in the file's sealed layout. There is no
// key anywhere in the scheme, and that absence is the point.
//
// WHY IT REPLACED THE AGGREGATE SCHEME IN THIS PACKAGE. Shacham-Waters private
// verification (por.go) is sound only while the verifying key is unknown to the
// prover (ASIACRYPT 2008, Definition 2.1). In silt the verifying key is derived from
// the file's LAYOUT key, which is exactly what a care link publishes, so every
// caretaker, every full-link reader and the publisher held the key that the
// soundness proof assumes no prover holds. A party with the key and no bytes could
// set every mu to zero and solve the verification equation for sigma directly. The
// remedy could not live inside the primitive: the party that must VERIFY was the
// party that could FORGE. A scheme with no key has no such party.
//
// WHAT IT COSTS, STATED PLAINLY. The aggregate scheme's response was
// size-independent; this one is not. An audit now moves SpotSampleCount leaves plus
// their paths, so "verify without fetching" becomes "verify by fetching a sample".
// The geometry is chosen so that a sweep costs no more wire than the aggregate it
// replaces (spotcheck_geometry_test.go) and materially less to produce
// (production_cost_floor_test.go), which is the trade build-immutable #8 asks to be
// measured before the mechanism is committed to.
//
// WHAT IT DOES NOT CHANGE. A shard is public: anyone may fetch it and then answer an
// audit over it. Retrievability therefore proves possession NOW, never exclusive
// provision, and that residual is why a passing audit mints spendable credit and no
// consensus standing at all (core/credit Reputation). The identity binding that
// makes one prover's answer useless to another lives in the SEED, not in the scheme,
// and is unchanged (core/node porProverSeed).
package por

import (
	"errors"

	"github.com/nerolabs/silt/core/manifest"
	"github.com/nerolabs/silt/ports"
)

// SpotLeafBytes is the width of one leaf of a shard's spot-check tree.
//
// It is the parameter the geometry measurement picks, and it moves four things at
// once: production (a smaller leaf means more tree nodes to hash), audit wire (a
// sample is one leaf plus a 32-byte-per-level path, so the path dominates below
// ~512 B and shrinking further buys nothing), detection granularity, and the FLOOR
// ON WHAT CHEATING SAVES. That last one is the least obvious and is a security
// property: a holder that drops a leaf must keep that leaf's 32-byte hash, or it
// cannot build an honest path for any of the leaves it kept. At 128 B the hash is a
// quarter of the leaf, so a cheater that discards every byte of a shard still stores
// 25% of it — and is caught on its first sample with certainty. A wider leaf makes
// that floor smaller. The
// measured table is in spotcheck_geometry_test.go and two assertions there hold this
// value to the two properties that justified the scheme. At 128 B over the shipped
// 262,160-byte shard: 2,049 leaves, a 12-hash path, 512 B per sample, and a
// commitment that costs 653 us to produce against the aggregate scheme's 6.2 ms.
//
// It is NOT read from this constant on the audit path. Every object commits the leaf
// size it was built at (manifest.Layout.LeafBytes), so changing this constant
// re-geometries NEW objects and leaves published ones verifiable. That is what keeps
// it an Evolving parameter instead of a value frozen for every network that has
// already committed it.
//
// ADVERSARY-SHAPE: capability=RetainedLeafHashes fixture=TestCareLinkHolderWithZeroBytesFailsTheAudit
const SpotLeafBytes = 128

// SpotSampleCount is how many leaves one challenge samples.
//
// Detection against a holder that dropped a fraction f of its shard is 1-(1-f)^l, so
// this is bought at one sample's wire cost each. The number is set from what the
// adversary can actually gain rather than from a round confidence target: silt is
// content-addressed and re-verifies every read against its hash (B3), so a holder
// missing ANY leaf can serve nothing, and the only retention strategy that pays is
// dropping the shard outright — which one sample catches with certainty. The margin
// above one is what prices the strategies in between, where a holder keeps enough to
// look busy; it is an Evolving parameter and re-pricing it against the slash the
// ledger actually applies is open work, not a settled number.
//
// At eight samples over the shipped geometry an audit moves 4,096 B, just under the
// 4,128 B aggregate response it replaces, and catches a holder that dropped the
// shard with certainty, half of it 99.61% of the time, a quarter 89.99% and a tenth
// 56.95%.
const SpotSampleCount = 8

// MaxSpotLeaves bounds the leaf count a caller may be asked to build a tree over. A
// shard is bounded by manifest.MaxChunkSize, so at the smallest sane leaf this is
// the widest tree that can legitimately occur; anything past it is a declared number
// that has not been checked (B7), and a decoder that allocates against it is the
// memory-exhaustion shape the bounds in core/manifest exist to refuse.
const MaxSpotLeaves = manifest.MaxChunkSize / 32

// SpotLeaves reports how many leaves an n-byte shard splits into at leafBytes. The
// final leaf is short rather than padded: its true bytes are what is hashed, and the
// count plus the shard size are both committed, so neither side has to agree on a
// padding convention.
func SpotLeaves(n, leafBytes int) int {
	if n <= 0 || leafBytes <= 0 {
		return 0
	}
	return (n + leafBytes - 1) / leafBytes
}

// LeafHashes splits data into leaves and hashes each one. It is the only place the
// leaf decomposition is written down; ShardRoot and Open both go through it, so an
// honest prover and an honest publisher cannot drift apart on the geometry.
//
// ADVERSARY-SHAPE: NOT-A-DEFENCE: the parties in that sentence are the HONEST publisher and the HONEST prover, and the property is that one decomposition exists rather than two. It says nothing about what an adversary can do; an adversary that decomposes differently simply produces an answer that fails.
func LeafHashes(data []byte, leafBytes int) []ports.Hash {
	n := SpotLeaves(len(data), leafBytes)
	out := make([]ports.Hash, n)
	for i := 0; i < n; i++ {
		lo := i * leafBytes
		hi := lo + leafBytes
		if hi > len(data) {
			hi = len(data)
		}
		out[i] = ports.HashBytes(data[lo:hi])
	}
	return out
}

// ShardRoot is the commitment a publisher writes into the sealed layout: the Merkle
// root over one shard's leaf hashes, under the same RFC 6962 construction the file's
// own root uses (core/manifest). It is a function of the shard's bytes alone — no
// key, no identity, nothing secret — so recomputing it is how a repairer confirms
// that what it rebuilt is what was committed.
func ShardRoot(data []byte, leafBytes int) ports.Hash {
	return manifest.MerkleRoot(LeafHashes(data, leafBytes))
}

// SpotIndices expands a challenge seed into the sampled leaf indices, distinct and
// in draw order. Both sides derive them from the seed alone, so the challenge on the
// wire stays a seed and a count. It reuses the aggregate scheme's index PRF
// deliberately: one derivation, one domain, and the seed binding that makes a
// challenge prover-specific is inherited unchanged.
func SpotIndices(seed [32]byte, leaves, count int) []int {
	if leaves <= 0 || count <= 0 {
		return nil
	}
	if count > leaves {
		count = leaves
	}
	out := make([]int, 0, count)
	used := make(map[int]bool, count)
	var ctr uint32
	for len(out) < count {
		i := int(prfUint(seed, "index", ctr) % uint64(leaves))
		ctr++
		if used[i] {
			continue
		}
		used[i] = true
		out = append(out, i)
	}
	return out
}

// Opening is one sampled leaf: its bytes and the sibling hashes that carry it to the
// shard root. No index is carried; the auditor derives the index list itself and
// reads the openings positionally against it (VerifyOpenings).
type Opening struct {
	Leaf []byte
	Path []ports.Hash
}

// Open is the prover side: the openings for the leaves this seed samples. It needs
// the bytes and nothing else — no key, no stored tags, no state beyond the shard
// itself, which is why a holder that dropped the shard has nothing to send.
func Open(data []byte, leafBytes int, seed [32]byte, count int) ([]Opening, error) {
	leaves := LeafHashes(data, leafBytes)
	if len(leaves) == 0 {
		return nil, errors.New("por: shard has no leaves to open")
	}
	idx := SpotIndices(seed, len(leaves), count)
	// BUILD THE TREE ONCE. The standalone manifest.Prove recomputes subtree hashes
	// over half the leaves on every call, which is O(n) per proof and would put
	// SpotSampleCount passes over the whole shard on the prover for one challenge.
	// A prepared tree is O(n) once and O(log n) per proof, and answering a
	// challenge is on the single serialized loop under a budget sized from what it
	// costs (core/node bondaudit.go).
	tree := manifest.BuildTree(leaves)
	out := make([]Opening, 0, len(idx))
	for _, i := range idx {
		p, err := tree.Prove(i)
		if err != nil {
			return nil, err
		}
		lo := i * leafBytes
		hi := lo + leafBytes
		if hi > len(data) {
			hi = len(data)
		}
		out = append(out, Opening{Leaf: append([]byte(nil), data[lo:hi]...), Path: p.Path})
	}
	return out, nil
}

// VerifyOpenings is the auditor side. It holds the committed root and the committed
// geometry; the prover holds only bytes. An answer passes iff there is exactly one
// opening per sampled index, in the order the seed drew them, and each one's leaf
// bytes hash to a leaf that the supplied path carries to the committed root.
//
// EVERY NUMBER HERE IS THE AUDITOR'S. root, leaves, leafBytes and the index list all
// come from the sealed layout and the auditor's own seed, never from the response.
// A prover cannot name the tree its sliver sits in, and cannot answer five samples
// with the one leaf it kept, because the openings are read POSITIONALLY against an
// index list it never sees a say in.
//
// ADVERSARY-SHAPE: capability=CareLinkWithoutBytes UNCOVERED: TestCareLinkHolderWithZeroBytesFailsTheAudit DOES grant the capability -- it hands a prover everything the care link yields and zero shard bytes -- but it carries no control that DISCRIMINATES, because the only way to run that attack WITHOUT the capability is to send the same fabricated leaves, and those fail for every capability. Same structural reason as ProverSuppliedRoot in core/node/por.go: a CAPABILITY-CONTROL is only well defined where the defence is BROKEN, and this one has held since the key left the scheme. What the fixture carries instead is a no-over-rejection control and a detection control.
func VerifyOpenings(root ports.Hash, leaves, leafBytes int, seed [32]byte, count int, ops []Opening) bool {
	if leaves <= 0 || leaves > MaxSpotLeaves || leafBytes <= 0 {
		return false
	}
	idx := SpotIndices(seed, leaves, count)
	if len(idx) == 0 || len(ops) != len(idx) {
		return false
	}
	for k, i := range idx {
		o := ops[k]
		if len(o.Leaf) == 0 || len(o.Leaf) > leafBytes {
			return false
		}
		// Only the LAST leaf of a shard may be short. A short leaf anywhere else
		// would let a prover answer with a prefix of a leaf it half-kept.
		if len(o.Leaf) != leafBytes && i != leaves-1 {
			return false
		}
		p := manifest.Proof{Index: i, Total: leaves, Path: o.Path}
		if !manifest.VerifyProof(root, ports.HashBytes(o.Leaf), p) {
			return false
		}
	}
	return true
}
