package statehash

// R3.1 V1 — the SMT second-preimage / domain-separation scope gate (verification half).
//
// Binding spec: docs/thinking/2026-09-01-smt-domain-separation-close-design.md, Part 3 "V1".
// The Researcher certifies the disjoint-preimage ARGUMENT separately; this file pins the
// two facts the argument RESTS ON, against OBSERVABLE digests (never the library's
// unexported leafNodePrefix/innerNodePrefix vars):
//
//  1. TestDomainSeparationLeafInnerPrefixBytesDiffer — the leaf preimage's first byte and the
//     inner preimage's first byte differ, reconstructed exactly as fold.go does
//     (foldLeafPreimage / foldInnerPreimage — the byte-exact replica TestFoldSeedEncodingMatch
//     esLibrary already pins against the real library).
//  2. TestDomainSeparationTypeSwapChangesDigest — a type-swap (the same 64-byte body under the
//     OTHER node type's prefix) produces a digest that is NOT the real library-committed root
//     for either a genuine single-leaf trie or a genuine two-leaf (one inner node) trie. This is
//     the operational form of "a 65-byte string valid as a leaf preimage is not accepted as an
//     inner node, and vice versa."
//
// MANDATORY ABLATION (recorded, not just asserted here): with foldInnerPrefix temporarily set
// equal to foldLeafPrefix, both tests below go RED. See
// core/statehash/../../.claude/agent-memory/tester/r3.1-smt-domain-separation-gates-2026-09-04.md
// for the verbatim captured failure — a green check with no demonstrated red is decoration
// (the session-7 scar).
//
// RUNTIME GATE: none — this is a pure-math assertion about SHA256 preimage disjointness, not an
// I/O or consensus-path behaviour. UNGATED: N/A (this test file itself IS the gate; nothing else
// observes the property at runtime because the property is "a forgery would require a generic
// SHA256 second-preimage", which cannot be runtime-tested by definition).

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"testing"

	"github.com/pokt-network/smt"
	"github.com/pokt-network/smt/kvstore/simplemap"
)

// TestDomainSeparationLeafInnerPrefixBytesDiffer pins the one-byte fact the whole close depends
// on: the leaf node-type prefix and the inner node-type prefix are distinct. Both preimages are
// reconstructed via fold.go's own foldLeafPreimage/foldInnerPreimage (the byte-exact replica of
// the library's node_encoders.go, itself pinned against the real library by
// TestFoldSeedEncodingMatchesLibrary) — never against the library's unexported prefix vars.
func TestDomainSeparationLeafInnerPrefixBytesDiffer(t *testing.T) {
	path := foldPath([]byte("some-key"))
	valueHash := foldValueHash([]byte("some-value"))

	leafPre := foldLeafPreimage(path, valueHash)
	innerPre := foldInnerPreimage(path, valueHash) // same 64-byte body, the OTHER node type

	if len(leafPre) == 0 || len(innerPre) == 0 {
		t.Fatalf("empty preimage: leaf=%d inner=%d bytes", len(leafPre), len(innerPre))
	}
	if leafPre[0] == innerPre[0] {
		t.Fatalf("domain separation broken: leaf prefix byte 0x%02x == inner prefix byte 0x%02x — "+
			"leaf and inner preimages are no longer disjoint by leading byte", leafPre[0], innerPre[0])
	}
	// Pin the exact values too (node_encoders.go: leafNodePrefix=0x00, innerNodePrefix=0x01), so
	// a drift to some OTHER pair of distinct bytes is visible in the failure diff, not just "equal".
	if leafPre[0] != 0x00 {
		t.Fatalf("leaf prefix byte drifted: got 0x%02x, want 0x00 (node_encoders.go leafNodePrefix)", leafPre[0])
	}
	if innerPre[0] != 0x01 {
		t.Fatalf("inner prefix byte drifted: got 0x%02x, want 0x01 (node_encoders.go innerNodePrefix)", innerPre[0])
	}
}

// TestDomainSeparationTypeSwapChangesDigest demonstrates the operational consequence of the
// prefix-byte disjointness against the REAL library, not just the replica: a node preimage's
// 64-byte body, re-hashed under the OTHER type's prefix, never equals the real committed root
// that body's genuine type produced. Concretely:
//
//   - A genuine single-leaf trie's root IS the leaf digest SHA256(0x00‖path‖valueHash). Re-hash
//     the SAME path/valueHash body under the inner prefix (0x01‖path‖valueHash) and assert the
//     result is NOT that leaf digest — an attacker holding this leaf's (path, valueHash) cannot
//     present it as a colliding inner-node preimage.
//   - A genuine two-leaf trie (leaves diverging at path bit 0) has an inner root
//     SHA256(0x01‖leftDigest‖rightDigest). Re-hash the SAME leftDigest/rightDigest body under the
//     leaf prefix (0x00‖leftDigest‖rightDigest) and assert the result is NOT that inner root — an
//     attacker holding the two child digests cannot present them as a colliding leaf preimage.
//
// This is the "shortened proof" / type-confusion attack this file exists to foreclose: it fails
// because the swapped digest differs from the honest root, for library-genuine inputs, not just
// for arbitrary bytes.
func TestDomainSeparationTypeSwapChangesDigest(t *testing.T) {
	// --- Case A: swap a genuine LEAF's body onto the INNER prefix. ---
	key, val := []byte("dst-key"), []byte("dst-value")
	leafTrie := smt.NewSparseMerkleTrie(simplemap.NewSimpleMap(), sha256.New())
	if err := leafTrie.Update(key, val); err != nil {
		t.Fatal(err)
	}
	if err := leafTrie.Commit(); err != nil {
		t.Fatal(err)
	}
	genuineLeafRoot := leafTrie.Root()

	path := foldPath(key)
	valueHash := foldValueHash(val)
	genuineLeafPre := foldLeafPreimage(path, valueHash)
	if !bytes.Equal(foldDigest(genuineLeafPre), genuineLeafRoot) {
		t.Fatalf("precondition failed: replicated leaf digest %x != library leaf root %x",
			foldDigest(genuineLeafPre), genuineLeafRoot)
	}

	swappedAsInner := foldInnerPreimage(path, valueHash) // same body, inner (0x01) prefix
	swappedDigest := foldDigest(swappedAsInner)
	if bytes.Equal(swappedDigest, genuineLeafRoot) {
		t.Fatalf("TYPE-CONFUSION FORGERY: leaf body (path=%x, valueHash=%x) re-hashed under the "+
			"inner prefix produced the SAME digest as the genuine leaf root %x — a shortened proof "+
			"could substitute an inner node for this leaf", path, valueHash, genuineLeafRoot)
	}

	// --- Case B: swap a genuine INNER node's body onto the LEAF prefix. ---
	var kL, kR []byte
	for i := 0; i < 10000 && (kL == nil || kR == nil); i++ {
		var b [4]byte
		binary.BigEndian.PutUint32(b[:], uint32(i))
		k := append([]byte("k"), b[:]...)
		if foldPathBit(foldPath(k), 0) == 0 && kL == nil {
			kL = k
		} else if foldPathBit(foldPath(k), 0) == 1 && kR == nil {
			kR = k
		}
	}
	if kL == nil || kR == nil {
		t.Fatal("could not find bit-0-diverging keys")
	}
	vL, vR := []byte("L"), []byte("R")
	innerTrie := smt.NewSparseMerkleTrie(simplemap.NewSimpleMap(), sha256.New())
	if err := innerTrie.Update(kL, vL); err != nil {
		t.Fatal(err)
	}
	if err := innerTrie.Update(kR, vR); err != nil {
		t.Fatal(err)
	}
	if err := innerTrie.Commit(); err != nil {
		t.Fatal(err)
	}
	genuineInnerRoot := innerTrie.Root()

	leftDigest := foldDigest(foldLeafPreimage(foldPath(kL), foldValueHash(vL)))
	rightDigest := foldDigest(foldLeafPreimage(foldPath(kR), foldValueHash(vR)))
	genuineInnerPre := foldInnerPreimage(leftDigest, rightDigest)
	if !bytes.Equal(foldDigest(genuineInnerPre), genuineInnerRoot) {
		t.Fatalf("precondition failed: replicated inner digest %x != library inner root %x",
			foldDigest(genuineInnerPre), genuineInnerRoot)
	}

	swappedAsLeaf := foldLeafPreimage(leftDigest, rightDigest) // same body, leaf (0x00) prefix
	swappedLeafDigest := foldDigest(swappedAsLeaf)
	if bytes.Equal(swappedLeafDigest, genuineInnerRoot) {
		t.Fatalf("TYPE-CONFUSION FORGERY: inner body (left=%x, right=%x) re-hashed under the leaf "+
			"prefix produced the SAME digest as the genuine inner root %x — a shortened proof could "+
			"substitute a leaf for this inner node", leftDigest, rightDigest, genuineInnerRoot)
	}

	// Sanity: the two genuine roots (unrelated tries) are not equal either.
	if bytes.Equal(genuineLeafRoot, genuineInnerRoot) {
		t.Fatalf("degenerate fixture: genuine leaf root %x == genuine inner root %x", genuineLeafRoot, genuineInnerRoot)
	}
}
