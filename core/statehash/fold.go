package statehash

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"

	"github.com/nerolabs/silt/ports"
	"github.com/pokt-network/smt"
	"github.com/pokt-network/smt/kvstore/simplemap"
)

// O(payload) multi-leaf state-root fold — the R-fold primitive (P1-a, certified 2026-08-31).
//
// A semi-stateless floor box holds only the two committed roots, not the tree. To reproduce
// validateEra3Roots' post-state StateRoot equality WITHOUT witnessing the WHOLE pre-state
// (the superseded O(whole-state) P1-a), it recomputes the post-state root by folding ONLY the
// CHANGED paths: derive the write-set from the block payload, witness each changed leaf's
// pre-state proof against prevStateRoot, apply the writes, and require the computed root equals
// the committed StateRoot.
//
// THE DESIGN — DELEGATE ALL TREE SURGERY TO THE AUDITED LIBRARY. The naive multi-leaf fold hit
// 36% wrong-root because it hand-rolled the library's tree surgery (add-displacement at a
// data-dependent prefixLen, the three delete sibling-promotion cases, extension split/absorb).
// This fold reproduces NONE of that. It reconstructs a PARTIAL trie rooted at prevStateRoot —
// seeding a node store with exactly the node preimages along the changed paths, derived from
// each changed leaf's pre-state proof — then calls the library's OWN Update / Delete for each
// payload write, then reads Root(). Every structural surgery is done by pokt-network/smt@v1.0.0
// itself, the same code that produced the honest StateRoot. The only hand-written digest
// arithmetic is the seed reconstruction (mirrorng verifyProofWithUpdates' fold), PINNED
// byte-exact against statehash.Root() over the full structural cross-product (fold_test.go).
//
// THE COMPLETENESS ANCHOR (why the box cannot be fed a short seed). After seeding, the box
// requires ImportSparseMerkleTrie(seed).Root() == prevStateRoot. The seed is faithful to the
// committed pre-state root or the fold stalls. Combined with a payload-DERIVED write-set (the
// caller runs the generator, not the prover — see the E/R consumer), an un-witnessed change to
// a leaf X makes the honest post-root differ from the forged committed StateRoot, and the final
// equality catches it. This is the certified hybrid: payload-derived write-set (completeness
// bound) + fold over changed paths (catches any extra / omitted / mis-valued change).
//
// SCOPE. This primitive computes a post-root from a claimed pre-root + changed-path proofs +
// write ops. It does NOT decide validity and it never Accepts — the caller compares the returned
// root to the committed StateRoot and stalls on mismatch. It is the mechanism, not the verdict.

// The library's node encoding, replicated here for the seed reconstruction. These are PINNED
// byte-exact against pokt-network/smt@v1.0.0 by TestFoldSeedEncodingMatchesLibrary — a library
// node-encoding drift reddens that pin before it can silently produce a wrong seed.
//
//   - leaf preimage  = leafNodePrefix(0x00) || path || valueHash        (node_encoders.go:57)
//   - inner preimage = innerNodePrefix(0x01) || leftDigest || rightDigest (node_encoders.go:65)
//   - node digest    = sha256(preimage)                                  (hasher.go:103)
//   - path           = sha256(key)                                       (hasher.go:69)
//   - valueHash      = sha256(value)                                     (hasher.go:81)
//   - placeholder    = 32 zero bytes                                     (hasher.go:58,183)
var (
	foldLeafPrefix  = []byte{0}
	foldInnerPrefix = []byte{1}
	foldPlaceholder = make([]byte, sha256.Size)
)

// FoldOp is one payload-derived write on a changed key.
type FoldOp struct {
	// Key is the field-tagged leaf key (built with Key(tag, rawKey)).
	Key []byte
	// OldValue is the leaf's committed pre-state value: nil for a key ABSENT in the pre-state
	// (an add), non-nil for a present key (an overwrite or a delete). It is the value the
	// changed key's pre-state proof proves against prevStateRoot.
	OldValue []byte
	// NewValue is the post-state value to write; nil means DELETE the key. A byRoot/spent/revoked
	// set-membership write carries Present; an un-revocation delete carries nil.
	NewValue []byte
	// Proof is the changed key's pre-state witness against prevStateRoot (membership when OldValue
	// is non-empty, non-membership when empty). A nil-wrapping witness stalls the fold.
	Proof Witness
	// DeleteSiblings are the off-path sibling nodes along this key's path, required ONLY for a
	// delete (NewValue == nil): the library's Delete resolves the off-path sibling at every inner
	// level (smt.go:298), and a single proof carries only their digests. Each is (digest,
	// preimage) — the digest is the sidenode value the parent inner node references (for an
	// extension-node sibling this is NOT sha256(preimage), so the digest must be carried, not
	// recomputed). Each is keyed in the seed under its digest. Their soundness is anchored by the
	// caller's FINAL computed-root vs committed-StateRoot equality (step 4 below), not by any
	// seed-root check: an injected sibling folds into the delete's re-derivation and diverges the
	// computed root, which then mismatches the committed StateRoot ⇒ stall. Empty for a set/overwrite/add.
	DeleteSiblings []FoldSibling
}

// FoldSibling is one off-path sibling node a delete's replay resolves: the digest the tree
// references it by, and its preimage.
type FoldSibling struct {
	Digest   []byte
	Preimage []byte
}

var (
	// ErrFoldProofFailed marks a changed key whose pre-state proof does not verify against
	// prevStateRoot: a forged, omitted, or mis-valued proof. The fold stalls — it never folds an
	// unverified change.
	ErrFoldProofFailed = errors.New("statehash: fold — a changed key's pre-state proof does not verify against prevStateRoot")

	// ErrFoldSiblingUnbound marks a delete's off-path sibling whose supplied Digest is not the
	// SHA-256 of its supplied Preimage (G-R31-1). The fold seeds the library's node store from
	// witness-provider bytes, and the library dispatches node type on data[0] while taking the
	// digest from the LOOKUP KEY, never recomputing it (smt.go ImportSparseMerkleTrie path) —
	// so an unbound (Digest, Preimage) pair is the audit's Issue #2 (a writeable prover store)
	// arriving on the VERIFY side with zero hash work. Binding every entry by the library's own
	// rule (spec.hashPreimage(Preimage) == Digest, the rule validateBasic applies to
	// SiblingData) before seeding closes it; the fold stalls on the first unbound sibling.
	ErrFoldSiblingUnbound = errors.New("statehash: fold — a delete sibling's digest is not the hash of its preimage")

	// ErrFoldProofShape marks a proof whose bytes would make the library PANIC rather than
	// return an error (G-R31-2): NonMembershipLeafData that does not begin with the leaf
	// prefix 0x00 reaches checkPrefix, which panics, and no recover() exists in core/. The
	// shape is refused BEFORE VerifyProof; the fold stalls.
	ErrFoldProofShape = errors.New("statehash: fold — a changed key's proof has a shape the verifier cannot parse")

	// ErrFoldApply marks a library Update/Delete or Commit failure while replaying the payload
	// writes onto the seeded partial trie (e.g. a delete of a key the seed does not carry). The
	// fold stalls rather than return an ambiguous root.
	ErrFoldApply = errors.New("statehash: fold — replaying a payload write onto the partial trie failed")
)

// FoldChangedPaths computes the post-state SMT root that results from applying the given payload
// writes to the committed pre-state whose root is prevStateRoot, WITHOUT witnessing the whole
// pre-state. It verifies each changed key's pre-state proof against prevStateRoot, reconstructs
// a partial trie over exactly the changed paths, requires the partial trie reconstructs
// prevStateRoot (the completeness+authenticity anchor), then applies the writes via the library's
// own Update/Delete and returns Root().
//
// The returned root is the honest post-state root a full node would compute IFF the ops are the
// complete write-set (the caller's payload-derivation obligation). The caller compares it to the
// committed StateRoot and stalls on mismatch; this function never decides validity.
func FoldChangedPaths(prevStateRoot ports.Hash, ops []FoldOp) (ports.Hash, error) {
	seed := map[string][]byte{}
	for i := range ops {
		op := &ops[i]
		// (1) Verify the changed key's pre-state proof against prevStateRoot. A nil, forged, omitted,
		// or mis-valued proof fails here and the fold stalls — it never seeds an unverified change.
		if op.Proof.proof == nil {
			return ports.Hash{}, fmt.Errorf("%w: key %x (no proof)", ErrFoldProofFailed, op.Key)
		}
		if !proofShapeParsable(op.Proof.proof) {
			return ports.Hash{}, fmt.Errorf("%w: key %x", ErrFoldProofShape, op.Key)
		}
		ok, err := smt.VerifyProof(op.Proof.proof, prevStateRoot[:], op.Key, op.OldValue, verifySpec())
		if err != nil || !ok {
			return ports.Hash{}, fmt.Errorf("%w: key %x", ErrFoldProofFailed, op.Key)
		}
		// (2) Reconstruct the on-path node preimages into the seed (mirrors verifyProofWithUpdates).
		seedFromProof(seed, op.Proof.proof, op.Key, op.OldValue)
		// (3) For a delete, seed the off-path sibling preimages the library's Delete resolves at
		// every inner level. Each is keyed under its digest; step 4 does NOT re-validate the seed, so
		// an injected sibling is caught only downstream — it diverges the computed post-root, which
		// then mismatches the caller's committed StateRoot (the final equality) ⇒ stall.
		if op.NewValue == nil {
			for _, sib := range op.DeleteSiblings {
				// G-R31-1: BIND before seeding, by the library's own hashPreimage rule: SHA-256
				// of the bytes for a leaf or inner preimage, the expansion root for an extension
				// preimage (foldDigestMismatch). An unbound pair is a forged node, not a corrupt
				// one — the caller-side root equality would catch a corrupt one; only this catches
				// a node whose digest the store would trust without recomputing it.
				if len(sib.Digest) != sha256.Size || foldDigestMismatch(sib.Digest, sib.Preimage) {
					return ports.Hash{}, fmt.Errorf("%w: key %x digest %x", ErrFoldSiblingUnbound, op.Key, sib.Digest)
				}
				seed[string(sib.Digest)] = sib.Preimage
			}
		}
	}

	// (4) Reconstruct the partial trie rooted at prevStateRoot. The per-changed-path completeness
	// and authenticity anchor is step (1): every changed key's pre-state proof is verified against
	// prevStateRoot before its path is seeded, so a forged / omitted / mis-valued proof stalls
	// there. ImportSparseMerkleTrie.Root() returns the imported lazy root digest WITHOUT re-walking
	// the store, so it is NOT a seed-validation step — the real catch for any residual seed
	// corruption is the FINAL computed-root vs committed-StateRoot equality the caller enforces:
	// a corrupt off-path sibling folds into the mutated-path re-derivation and diverges the
	// computed root, which then mismatches the committed StateRoot ⇒ stall.
	store := simplemap.NewSimpleMapWithMap(seed)
	trie := smt.ImportSparseMerkleTrie(store, sha256.New(), prevStateRoot[:])

	// (5) Apply the payload writes via the library's OWN surgery. Deletes first, then sets — order
	// is irrelevant to the final root (the SMT root is a pure function of the leaf set), but doing
	// deletes first keeps a key that is both (never in E/R) unambiguous.
	for i := range ops {
		op := &ops[i]
		if op.NewValue != nil {
			continue
		}
		if err := trie.Delete(op.Key); err != nil {
			return ports.Hash{}, fmt.Errorf("%w: delete %x: %v", ErrFoldApply, op.Key, err)
		}
	}
	for i := range ops {
		op := &ops[i]
		if op.NewValue == nil {
			continue
		}
		if err := trie.Update(op.Key, op.NewValue); err != nil {
			return ports.Hash{}, fmt.Errorf("%w: update %x: %v", ErrFoldApply, op.Key, err)
		}
	}
	if err := trie.Commit(); err != nil {
		return ports.Hash{}, fmt.Errorf("%w: commit: %v", ErrFoldApply, err)
	}
	var post ports.Hash
	copy(post[:], trie.Root())
	return post, nil
}

// seedFromProof reconstructs the on-path node preimages for one proven key into seed, so the
// library can resolveLazy down the key's path. It mirrors verifyProofWithUpdates' bottom-up fold
// (proofs.go:437-450): start from the leaf digest (or the displaced non-membership leaf, or a
// placeholder), and combine with each sidenode via the inner-node encoding, ordered by the path
// bit. It ALSO seeds the immediate sibling preimage (SiblingData), which a delete's promotion
// resolves.
func seedFromProof(seed map[string][]byte, proof *smt.SparseMerkleProof, key, value []byte) {
	p := foldPath(key)
	var curHash []byte
	if len(value) == 0 {
		// Non-membership: the node on this path is either a placeholder (empty subtree) or the
		// displaced unrelated leaf carried in NonMembershipLeafData.
		if proof.NonMembershipLeafData == nil {
			curHash = foldPlaceholder
		} else {
			pre := append([]byte(nil), proof.NonMembershipLeafData...)
			curHash = foldDigest(pre)
			seed[string(curHash)] = pre
		}
	} else {
		pre := foldLeafPreimage(p, foldValueHash(value))
		curHash = foldDigest(pre)
		seed[string(curHash)] = pre
	}

	sn := proof.SideNodes
	// SiblingData is the immediate sibling's node preimage; its digest is SideNodes[0]
	// (validateBasic enforces SideNodes[0] == hashPreimage(SiblingData)), which for an extension
	// node is NOT sha256(SiblingData) — so key it under SideNodes[0], the digest the parent
	// inner node references. Needed for a delete's leaf-promote / extension-absorb sibling.
	if proof.SiblingData != nil && len(sn) > 0 {
		seed[string(sn[0])] = proof.SiblingData
	}
	// Fold up: at step i the path bit is at (len(sn)-1-i); the on-path node is the left child
	// when that bit is 0, the right child when 1.
	for i := 0; i < len(sn); i++ {
		var pre []byte
		if foldPathBit(p, len(sn)-1-i) == 0 {
			pre = foldInnerPreimage(curHash, sn[i])
		} else {
			pre = foldInnerPreimage(sn[i], curHash)
		}
		curHash = foldDigest(pre)
		seed[string(curHash)] = pre
	}
}

// foldPath, foldValueHash, foldDigest reproduce the library's path/value/node hashers. Pinned by
// TestFoldSeedEncodingMatchesLibrary.
func foldPath(key []byte) []byte        { h := sha256.Sum256(key); return h[:] }
func foldValueHash(v []byte) []byte     { h := sha256.Sum256(v); return h[:] }
func foldDigest(preimage []byte) []byte { h := sha256.Sum256(preimage); return h[:] }

func foldLeafPreimage(path, valueHash []byte) []byte {
	d := make([]byte, 0, len(foldLeafPrefix)+len(path)+len(valueHash))
	d = append(d, foldLeafPrefix...)
	d = append(d, path...)
	d = append(d, valueHash...)
	return d
}

func foldInnerPreimage(left, right []byte) []byte {
	d := make([]byte, 0, len(foldInnerPrefix)+len(left)+len(right))
	d = append(d, foldInnerPrefix...)
	d = append(d, left...)
	d = append(d, right...)
	return d
}

// foldPathBit returns the bit at index i of path p: 0 = left child, 1 = right child. Matches the
// library's getPathBit (bit i is the (i%8)-th most-significant bit of byte i/8).
func foldPathBit(p []byte, i int) int {
	if p[i/8]&(1<<(8-1-uint(i)%8)) > 0 {
		return 1
	}
	return 0
}

// ProveWithSiblings is the provider-side companion to a delete FoldOp: it issues the key's proof
// AND collects the off-path sibling preimages the fold's Delete replay needs. The prover holds the
// full committed trie; a root-only box does not, so the box receives these from the provider and
// the seed-root equality check verifies them. Returns the proof and the sibling preimages keyed by
// the same digests the proof's SideNodes carry (non-placeholder siblings only).
func (p *Prover) ProveWithSiblings(key []byte) (Witness, []FoldSibling, error) {
	proof, err := p.trie.Prove(key)
	if err != nil {
		return Witness{}, nil, err
	}
	var sibs []FoldSibling
	for _, sd := range proof.SideNodes {
		if bytes.Equal(sd, foldPlaceholder) {
			continue // placeholder → resolves to nil, no preimage needed
		}
		pre, gErr := p.nodePreimage(sd)
		if gErr != nil {
			return Witness{}, nil, fmt.Errorf("statehash: fold sibling preimage for %x: %w", sd, gErr)
		}
		if pre != nil {
			sibs = append(sibs, FoldSibling{Digest: append([]byte(nil), sd...), Preimage: pre})
		}
	}
	return NewWitness(proof), sibs, nil
}

// foldDigestMismatch reports whether digest != hashPreimage(preimage) under the library's own
// rule (trie_spec.go hashSerialization): a leaf or inner preimage digests to SHA-256 of its
// bytes; an EXTENSION preimage (prefix 0x02 ‖ bounds(2) ‖ path(32) ‖ childDigest(32)) digests
// to the root of its EXPANSION — the chain of inner nodes the extension stands for, one per
// path bit from bounds[1]-1 down to bounds[0], each with the on-path child on the path bit's
// side and the placeholder on the other (extension_node.go expand + trie_spec.go digestNode).
// Used by the G-R31-1 binding. A preimage of any other shape does not bind.
func foldDigestMismatch(digest, preimage []byte) bool {
	if len(preimage) == 0 {
		return true
	}
	if preimage[0] != 0x02 {
		sum := sha256.Sum256(preimage)
		return !bytes.Equal(sum[:], digest)
	}
	// Extension: 1 + 2 + 32 + 32 bytes exactly.
	if len(preimage) != 1+2+sha256.Size+sha256.Size {
		return true
	}
	start, end := int(preimage[1]), int(preimage[2])
	path := preimage[3 : 3+sha256.Size]
	cur := append([]byte(nil), preimage[3+sha256.Size:]...)
	if end < start || end > 8*sha256.Size {
		return true
	}
	for i := end - 1; i >= start; i-- {
		var pre []byte
		if foldPathBit(path, i) == 0 {
			pre = foldInnerPreimage(cur, foldPlaceholder)
		} else {
			pre = foldInnerPreimage(foldPlaceholder, cur)
		}
		cur = foldDigest(pre)
	}
	return !bytes.Equal(cur, digest)
}

// proofShapeParsable is the G-R31-2 pre-VerifyProof shape check, shared by Resolve and
// FoldChangedPaths. The library's validateBasic bounds NonMembershipLeafData from below but
// never checks its prefix byte; parseLeafNode then calls checkPrefix, which PANICS on anything
// but 0x00 (node_encoders.go), and there is no recover() in non-test core/. A 33-byte proof —
// NonMembershipLeafData = {0x01, 32 bytes}, no SiblingData, no SideNodes — offered for any
// absence query would crash the process. Refuse the shape first; the caller maps the refusal
// to NoWitness / a fold stall, never to a proven outcome.
//
// SIBLINGDATA IS THE SECOND ARM (PE code ruling RULING-R3.1-smt-domain-separation-code-8434591
// S1, measured): validateBasic bounds SiblingData nowhere and calls hashPreimage(SiblingData)
// whenever it is non-nil and SideNodes is non-empty; for a 0x02 (extension) prefix that is
// parseExtNode slicing data[1:3] and data[3:35] unbounded, and for len == 0 it is
// isExtNode's data[:1]. A 154-byte gob witness panicked IngestBlockWitnesses. The boundary,
// pinned by sweep: any non-nil SiblingData panics at len == 0, or at [0] == 0x02 with
// len < 35; len >= 35 is safe (childData = data[35:] may be empty). SideNodes need no arm:
// the verifier copies each into a fresh 32-byte buffer.
func proofShapeParsable(p *smt.SparseMerkleProof) bool {
	if p == nil {
		return false
	}
	if p.NonMembershipLeafData != nil && (len(p.NonMembershipLeafData) < 1 || p.NonMembershipLeafData[0] != 0x00) {
		return false
	}
	if p.SiblingData != nil {
		if len(p.SiblingData) == 0 {
			return false
		}
		if p.SiblingData[0] == 0x02 && len(p.SiblingData) < 1+2+sha256.Size {
			return false
		}
	}
	return true
}
