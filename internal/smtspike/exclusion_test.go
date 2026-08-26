package smtspike

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"strings"
	"testing"

	"github.com/pokt-network/smt"
	"github.com/pokt-network/smt/kvstore/simplemap"
)

// Field tags keep one field's key space from colliding with another's. silt's
// keystone commits several maps under one root (byRoot, spent, ...), so a key
// is the tag concatenated with the raw key, never the raw key alone.
const (
	tagByRoot = "byRoot\x00"
	tagSpent  = "spent\x00"
)

func stateKey(tag string, raw []byte) []byte {
	return append([]byte(tag), raw...)
}

// present is the value stored for a set-membership field. The exclusion
// consumers ask "is this key committed?", so the value is a fixed marker; the
// security rests on the key's presence, not on what it maps to.
var present = []byte{1}

// newTrie builds a committed trie over the given keys and returns the trie and
// its root. Commit() flushes to the node store, which is the path a real
// keystone takes before publishing a root.
func newTrie(t testing.TB, keys ...[]byte) (*smt.SMT, []byte) {
	t.Helper()
	trie := smt.NewSparseMerkleTrie(simplemap.NewSimpleMap(), sha256.New())
	for _, k := range keys {
		if err := trie.Update(k, present); err != nil {
			t.Fatalf("Update(%q): %v", k, err)
		}
	}
	if err := trie.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}
	return trie, trie.Root()
}

// emptyValue is what VerifyProof treats as the non-membership claim. The
// library compares against a nil/empty slice, so this is the exclusion query.
var emptyValue []byte

// TestAbsentKeyProvesAbsent is gate step 2, first half: a specific key absent
// from the trie verifies as absent against the committed root.
func TestAbsentKeyProvesAbsent(t *testing.T) {
	keyA := stateKey(tagByRoot, []byte("A"))
	keyB := stateKey(tagByRoot, []byte("B"))
	keyC := stateKey(tagByRoot, []byte("C")) // never inserted

	trie, root := newTrie(t, keyA, keyB)

	proof, err := trie.Prove(keyC)
	if err != nil {
		t.Fatalf("Prove(absent key): %v", err)
	}
	ok, err := smt.VerifyProof(proof, root, keyC, emptyValue, trie.Spec())
	if err != nil {
		t.Fatalf("VerifyProof(absence of C): unexpected error: %v", err)
	}
	if !ok {
		t.Fatal("absence proof for an absent key did not verify — the library " +
			"cannot serve silt's exclusion consumers")
	}
}

// TestAbsenceProofForPresentKeyFails is THE assertion the gate turns on. Every
// exclusion consumer is unsound if a present key can be shown absent, so this
// covers all three shapes an adversary has: an honest membership proof
// re-read as an absence claim, another key's absence proof replayed, and the
// "unrelated leaf" branch fed the related leaf.
func TestAbsenceProofForPresentKeyFails(t *testing.T) {
	keyA := stateKey(tagByRoot, []byte("A"))
	keyB := stateKey(tagByRoot, []byte("B"))

	trie, root := newTrie(t, keyA, keyB)

	t.Run("membership proof reused as an absence claim", func(t *testing.T) {
		proof, err := trie.Prove(keyA)
		if err != nil {
			t.Fatalf("Prove(keyA): %v", err)
		}
		// Same proof bytes, but the verifier is asked to accept the empty
		// value — i.e. "A is absent".
		ok, err := smt.VerifyProof(proof, root, keyA, emptyValue, trie.Spec())
		if ok {
			t.Fatal("UNSOUND: a membership proof verified as an absence proof " +
				"for the same present key")
		}
		t.Logf("rejected as required (ok=%v, err=%v)", ok, err)
	})

	t.Run("another key's absence proof replayed against a present key", func(t *testing.T) {
		// Sweep candidate absent keys. Each one's proof is offered as evidence
		// that keyA — which IS present — is absent. None may verify, and at
		// least one must land on keyA's leaf so the "related leaf" guard at
		// proofs.go:422 is actually exercised rather than assumed.
		relatedLeafGuardFired := false
		const candidates = 256

		for i := 0; i < candidates; i++ {
			var suffix [8]byte
			binary.BigEndian.PutUint64(suffix[:], uint64(i))
			keyC := stateKey(tagByRoot, append([]byte("absent-"), suffix[:]...))

			if _, err := trie.Get(keyC); err == nil {
				// Guard the premise: the candidate must really be absent.
				if v, _ := trie.Get(keyC); len(v) != 0 {
					t.Fatalf("candidate %d was unexpectedly present", i)
				}
			}

			proof, err := trie.Prove(keyC)
			if err != nil {
				t.Fatalf("Prove(candidate %d): %v", i, err)
			}

			// Sanity: it must verify as absent for its OWN key.
			ok, err := smt.VerifyProof(proof, root, keyC, emptyValue, trie.Spec())
			if err != nil || !ok {
				t.Fatalf("candidate %d: own absence proof failed (ok=%v, err=%v)", i, ok, err)
			}

			// The attack: same proof, present key.
			ok, err = smt.VerifyProof(proof, root, keyA, emptyValue, trie.Spec())
			if ok {
				t.Fatalf("UNSOUND: candidate %d's absence proof verified absence "+
					"of the PRESENT key A", i)
			}
			if err != nil && strings.Contains(err.Error(), "related leaf") {
				if !errors.Is(err, smt.ErrBadProof) {
					t.Fatalf("related-leaf rejection did not wrap ErrBadProof: %v", err)
				}
				if !relatedLeafGuardFired {
					t.Logf("related-leaf guard fired on candidate %d: %v", i, err)
				}
				relatedLeafGuardFired = true
			}
		}

		if !relatedLeafGuardFired {
			t.Fatal("the 'non-membership proof on related leaf' guard was never " +
				"exercised — the branch that makes this sound is unproven")
		}
	})
}

// TestMembershipProofStillVerifies keeps the negative tests honest: a verifier
// that rejects everything would pass them all.
func TestMembershipProofStillVerifies(t *testing.T) {
	keyA := stateKey(tagByRoot, []byte("A"))
	keyB := stateKey(tagByRoot, []byte("B"))
	trie, root := newTrie(t, keyA, keyB)

	proof, err := trie.Prove(keyA)
	if err != nil {
		t.Fatalf("Prove(keyA): %v", err)
	}
	ok, err := smt.VerifyProof(proof, root, keyA, present, trie.Spec())
	if err != nil || !ok {
		t.Fatalf("membership proof for a present key failed (ok=%v, err=%v)", ok, err)
	}

	// A wrong value at a present key must not verify.
	if ok, _ := smt.VerifyProof(proof, root, keyA, []byte{2}, trie.Spec()); ok {
		t.Fatal("UNSOUND: membership verified against the wrong value")
	}
	// A proof must not transplant onto a different root.
	otherKey := stateKey(tagByRoot, []byte("Z"))
	_, otherRoot := newTrie(t, keyA, keyB, otherKey)
	if bytes.Equal(root, otherRoot) {
		t.Fatal("adding a key did not change the root")
	}
	if ok, _ := smt.VerifyProof(proof, root, keyB, present, trie.Spec()); ok {
		t.Fatal("UNSOUND: A's proof verified membership of B")
	}
}

// TestFieldTagSeparatesKeySpaces covers the keystone's multi-map commitment:
// the same raw key under two field tags must be two independent trie entries,
// so committing a root under byRoot never implies the serial was spent.
func TestFieldTagSeparatesKeySpaces(t *testing.T) {
	raw := []byte("shared-32-byte-looking-identifier")
	inRoot := stateKey(tagByRoot, raw)
	inSpent := stateKey(tagSpent, raw)

	trie, root := newTrie(t, inRoot)

	proof, err := trie.Prove(inSpent)
	if err != nil {
		t.Fatalf("Prove(inSpent): %v", err)
	}
	ok, err := smt.VerifyProof(proof, root, inSpent, emptyValue, trie.Spec())
	if err != nil || !ok {
		t.Fatalf("the same raw key under a different field tag was not provably "+
			"absent (ok=%v, err=%v)", ok, err)
	}
}

// TestAdversarialKeysDoNotDeepenTheTrie covers the implementation note carried
// out of the library call: silt's keys are adversary-influenced (a byRoot key
// is H(content), and content can be ground), so an attacker who could choose
// the leaf POSITION could force pathological depth — inflating every proof and
// every path update against a 2 GB box (build-immutable #8).
//
// The default path hasher digests the key, so the position is SHA-256(key) and
// grinding the key body buys no control over the path. This asserts that
// behaviourally: keys sharing a long common prefix must not produce deeper
// proofs than random keys. It also pins the reason the field tag is for domain
// separation, not for grind resistance — the library already provides the
// latter, and only WithPathHasher(nil-path-hasher) would forfeit it.
func TestAdversarialKeysDoNotDeepenTheTrie(t *testing.T) {
	const n = 4096

	// Ground keys: a fixed 32-byte prefix, differing only in a trailing counter.
	groundPrefix := bytes.Repeat([]byte{0xAB}, 32)
	ground := smt.NewSparseMerkleTrie(simplemap.NewSimpleMap(), sha256.New())
	// Spread keys: independent digests.
	spread := smt.NewSparseMerkleTrie(simplemap.NewSimpleMap(), sha256.New())

	groundKeys := make([][]byte, n)
	spreadKeys := make([][]byte, n)
	for i := 0; i < n; i++ {
		var ctr [8]byte
		binary.BigEndian.PutUint64(ctr[:], uint64(i))

		groundKeys[i] = stateKey(tagByRoot, append(append([]byte{}, groundPrefix...), ctr[:]...))
		body := sha256.Sum256(ctr[:])
		spreadKeys[i] = stateKey(tagByRoot, body[:])

		if err := ground.Update(groundKeys[i], present); err != nil {
			t.Fatalf("ground Update(%d): %v", i, err)
		}
		if err := spread.Update(spreadKeys[i], present); err != nil {
			t.Fatalf("spread Update(%d): %v", i, err)
		}
	}
	if err := ground.Commit(); err != nil {
		t.Fatalf("ground Commit: %v", err)
	}
	if err := spread.Commit(); err != nil {
		t.Fatalf("spread Commit: %v", err)
	}

	maxDepth := func(trie *smt.SMT, keys [][]byte) int {
		worst := 0
		for _, k := range keys {
			proof, err := trie.Prove(k)
			if err != nil {
				t.Fatalf("Prove: %v", err)
			}
			if d := len(proof.SideNodes); d > worst {
				worst = d
			}
		}
		return worst
	}

	groundMax := maxDepth(ground, groundKeys)
	spreadMax := maxDepth(spread, spreadKeys)
	t.Logf("n=%d  max proof depth: ground-prefix keys=%d, spread keys=%d", n, groundMax, spreadMax)

	// Both must stay near log2(n), nowhere near the 256-bit path length. The
	// slack absorbs the ordinary tail of random-path collisions.
	const bound = 40 // log2(4096)=12, with generous headroom; 256 is the failure case
	if groundMax > bound {
		t.Fatalf("adversarially prefixed keys reached depth %d (> %d): the path "+
			"hasher is not protecting position choice", groundMax, bound)
	}
	if groundMax > spreadMax+8 {
		t.Fatalf("ground-prefix keys (depth %d) are materially deeper than spread "+
			"keys (depth %d): key grinding is influencing trie shape", groundMax, spreadMax)
	}
}
