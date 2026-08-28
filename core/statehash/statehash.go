// Package statehash computes the era-3 committed state SMT root over silt's
// set-valued validity state. It is the promotion of internal/smtspike into
// product code: the pokt-network/smt v1.0.0 library (adopted #596) now lives
// behind this package, and the state root is a pure function of the committed
// key→value set.
//
// This is build step 1 of the certified era-3 sequence
// (docs/thinking/2026-08-28-era3-format-design-options.md, choice 5). It computes
// the root and proves the encoding deterministic. It does NOT add a Block field,
// change Hash(), add a validity predicate, or touch BlockVersion — those are later
// certified steps. The root computed here goes behind the keystone oracles so a
// value-encoding defect is caught as a test failure before it can become a signed
// field.
//
// The value encoding is the load-bearing decision both consults flagged highest-
// severity: the SMT binds the leaf VALUE into the leaf digest, so the root is a
// function of the exact leaf bytes. Three super-quorum predicates SUM bonded/
// epochSet weights, so a wrong-value witness is a consensus-safety attack. The
// per-field-class byte encoding is pinned in
// docs/thinking/2026-08-28-era3-state-root-value-encoding.md and implemented by the
// EncodeInt64/EncodeUint64/... helpers below. Widths and endianness are CONSENSUS
// PARAMETERS (research cert Q6 flag 2): a change is a hard fork.
package statehash

import (
	"crypto/sha256"
	"encoding/binary"

	"github.com/nerolabs/silt/ports"
	"github.com/pokt-network/smt"
	"github.com/pokt-network/smt/kvstore/simplemap"
)

// Present is the leaf value for a set-membership (Class A) field. The predicate
// reads existence, never a value, so the leaf carries a fixed marker; the security
// rests on the key's presence/absence, which the SMT proves via inclusion and
// exclusion proofs. Matches the smtspike marker (exclusion_test.go:30).
var Present = []byte{1}

// EncodeInt64 is the canonical encoding for the int64 weight fields (bonded,
// epochSet): 8-byte big-endian two's-complement. This is TOTAL over the int64
// domain — every representable value maps to exactly one 8-byte leaf, so the leaf
// is a faithful image of the value a super-quorum predicate sums. It does NOT
// reject negatives: that would be a validity rule (a later step), and a non-total
// encoder would re-open an absent-vs-present ambiguity. See the value-encoding
// deliberation, "Sign note".
func EncodeInt64(v int64) []byte {
	var b [8]byte
	binary.BigEndian.PutUint64(b[:], uint64(v))
	return b[:]
}

// EncodeUint64 is the canonical encoding for the uint64 fields (bondRegHeight,
// bondDomain, gateHeight): 8-byte big-endian. bondDomain's 0 = unset is a legal
// in-domain value committed as the encoding of 0; a key ABSENT from the map has no
// leaf at all, so present-zero and proven-absent are distinct leaf states under the
// root (the distinction the eventual witness accessor must preserve — cert Q4).
func EncodeUint64(v uint64) []byte {
	var b [8]byte
	binary.BigEndian.PutUint64(b[:], v)
	return b[:]
}

// EncodeUint8 is the canonical encoding for regVersion: a single byte.
func EncodeUint8(v uint8) []byte { return []byte{v} }

// EncodeBool is the canonical encoding for the bool value fields (bondRootProven)
// and the bool scalars (everMature, matureEpoch, gateLockedIn): a single byte,
// 0x00 or 0x01. bondRootProven's value distinguishes proven from declared (G3), so
// it is committed as a value, never a presence marker.
func EncodeBool(v bool) []byte {
	if v {
		return []byte{1}
	}
	return []byte{0}
}

// EncodeID is the canonical encoding for identity values (bondRootOwner's NodeID):
// the raw 32 bytes, no transform and no length prefix. NodeID = Hash = [32]byte
// (ports/net.go:82, ports/ports.go:17); the value IS the owner identity.
func EncodeID(id ports.NodeID) []byte {
	out := make([]byte, len(id))
	copy(out, id[:])
	return out
}

// Key builds a field-tagged leaf key: tag ‖ rawKey, where tag is the field name
// followed by a single NUL. The NUL terminator makes the concatenation injective
// across all field tags and the scalar reserved keys (research cert Q3): the first
// \x00 terminates the field name, so no raw key under one tag can equal a key under
// another. This is the smtspike scheme (exclusion_test.go:18-25), kept verbatim.
//
// A scalar leaf uses an empty rawKey (Key(tag)), which cannot collide with any map
// keyspace because map raw keys (32-byte NodeIDs/Hashes, non-empty serials) are
// never empty.
func Key(tag string, rawKey []byte) []byte {
	k := make([]byte, 0, len(tag)+len(rawKey))
	k = append(k, tag...)
	k = append(k, rawKey...)
	return k
}

// Leaf is one committed (key, value) pair, already encoded to canonical bytes. A
// Builder accumulates Leaves and Root() commits them into the SMT. Keeping the
// encoding in the caller (package chain, which owns the field types) and the SMT
// mechanics here keeps this package free of any dependency on chain.
type Leaf struct {
	Key   []byte
	Value []byte
}

// Root computes the committed state SMT root over the given leaves. The result is a
// pure function of the SET of (key, value) pairs, independent of the order they are
// supplied in — the history-independence property the SMT was chosen for (#597 Q1).
// The determinism oracle proves this by execution.
//
// The hash is sha256 (research cert Q6 flag 1: a security parameter the exclusion
// soundness rests on — pinned, never config). The store is an in-memory simplemap:
// step 1 computes a root, it does not persist a tree (the disk-backed NodeStore is
// a separate ratified follow-on, RULING-keystone-node-store). A duplicate key in
// the input is a programming error in the caller's marshalling and is reported.
func Root(leaves []Leaf) (ports.Hash, error) {
	trie := smt.NewSparseMerkleTrie(simplemap.NewSimpleMap(), sha256.New())
	seen := make(map[string]struct{}, len(leaves))
	for _, lf := range leaves {
		if _, dup := seen[string(lf.Key)]; dup {
			return ports.Hash{}, &DuplicateKeyError{Key: append([]byte(nil), lf.Key...)}
		}
		seen[string(lf.Key)] = struct{}{}
		if err := trie.Update(lf.Key, lf.Value); err != nil {
			return ports.Hash{}, err
		}
	}
	if err := trie.Commit(); err != nil {
		return ports.Hash{}, err
	}
	// The SMT root is already a 32-byte sha256 digest; store it verbatim as a
	// ports.Hash (do NOT re-hash via HashBytes — that would double-hash and
	// discard the library's committed root value).
	var h ports.Hash
	copy(h[:], trie.Root())
	return h, nil
}

// DuplicateKeyError reports two leaves with the same key — a marshalling bug in the
// caller, not a valid state. The SMT would silently keep the last write; surfacing
// it keeps the root a faithful image of the committed set.
type DuplicateKeyError struct{ Key []byte }

func (e *DuplicateKeyError) Error() string {
	return "statehash: duplicate leaf key: " + string(e.Key)
}
