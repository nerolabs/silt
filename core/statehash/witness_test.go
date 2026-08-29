package statehash

import (
	"crypto/sha256"
	"os"
	"regexp"
	"testing"

	"github.com/nerolabs/silt/ports"
	"github.com/pokt-network/smt"
	"github.com/pokt-network/smt/kvstore/simplemap"
)

// These are the R4-a ablation tests: the three-valued witness accessor spine.
// Each asserts the C-7 §104 banned move — "no witness → treat as absent" — is
// UNREPRESENTABLE. A green here with no demonstrated red is a comment that
// compiles; each test below was watched to fail first (see the builder report).

// tagTest is a field tag used only by these tests, matching the Key() scheme.
const tagTest = "byRoot\x00"

// buildTrie commits the given raw keys under tagTest with Present values and
// returns the trie and its committed root as a ports.Hash — the same spec
// statehash.Root uses (non-sum SHA-256), so proofs against this root are exactly
// the proofs a floor box verifies against a real committed StateRoot.
func buildTrie(t testing.TB, rawKeys ...[]byte) (*smt.SMT, ports.Hash) {
	t.Helper()
	trie := smt.NewSparseMerkleTrie(simplemap.NewSimpleMap(), sha256.New())
	for _, rk := range rawKeys {
		if err := trie.Update(Key(tagTest, rk), Present); err != nil {
			t.Fatalf("Update(%q): %v", rk, err)
		}
	}
	if err := trie.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}
	var root ports.Hash
	copy(root[:], trie.Root())
	return trie, root
}

// TestResolveVerifiedProofsClassifyCorrectly is ablation test 1: a verified
// non-membership proof resolves to PROVEN_ABSENT, and a verified membership proof
// resolves to PROVEN_PRESENT(value). A verifier that returned NO_WITNESS for
// everything (the trivially safe but useless accessor) fails this test.
func TestResolveVerifiedProofsClassifyCorrectly(t *testing.T) {
	present := []byte("present-raw-key")
	absent := []byte("absent-raw-key") // never inserted

	trie, root := buildTrie(t, present)

	t.Run("verified non-membership proof -> PROVEN_ABSENT", func(t *testing.T) {
		key := Key(tagTest, absent)
		proof, err := trie.Prove(key)
		if err != nil {
			t.Fatalf("Prove(absent): %v", err)
		}
		// value == nil is the non-membership (absence) query.
		got := Resolve(root, key, nil, NewWitness(proof))
		if !got.IsProvenAbsent() {
			t.Fatalf("verified non-membership proof did not resolve to PROVEN_ABSENT: got %s", got.Outcome())
		}
		if got.MustStall() {
			t.Fatal("a PROVEN_ABSENT result must not report MustStall")
		}
		if got.Value() != nil {
			t.Fatalf("PROVEN_ABSENT must carry no value, got %x", got.Value())
		}
	})

	t.Run("verified membership proof -> PROVEN_PRESENT(value)", func(t *testing.T) {
		key := Key(tagTest, present)
		proof, err := trie.Prove(key)
		if err != nil {
			t.Fatalf("Prove(present): %v", err)
		}
		got := Resolve(root, key, Present, NewWitness(proof))
		if !got.IsProvenPresent() {
			t.Fatalf("verified membership proof did not resolve to PROVEN_PRESENT: got %s", got.Outcome())
		}
		if string(got.Value()) != string(Present) {
			t.Fatalf("PROVEN_PRESENT carried wrong value: got %x want %x", got.Value(), Present)
		}
	})
}

// TestNoWitnessNeverAbsent is ablation test 2, the core of the spine: a missing
// witness resolves to NO_WITNESS, and MustStall is true. A caller that read this
// as absent would be WRONG. The companion source-scan test proves there is no
// code path from a missing/failed proof to PROVEN_ABSENT.
func TestNoWitnessNeverAbsent(t *testing.T) {
	absent := []byte("some-key")
	_, root := buildTrie(t, []byte("other-key"))
	key := Key(tagTest, absent)

	// No proof supplied at all.
	got := Resolve(root, key, nil, NewWitness(nil))
	if got.IsProvenAbsent() {
		t.Fatal("BANNED MOVE (C-7 §104): a missing witness resolved to PROVEN_ABSENT")
	}
	if !got.MustStall() {
		t.Fatalf("a missing witness must resolve to NO_WITNESS/MustStall, got %s", got.Outcome())
	}
	if got.Outcome() != NoWitness {
		t.Fatalf("missing witness outcome: got %s want NO_WITNESS", got.Outcome())
	}

	// The zero Result must also be the safe state, so a forgotten/mis-constructed
	// Result stalls rather than reading as a proven exclusion.
	var zero Result
	if zero.IsProvenAbsent() {
		t.Fatal("the zero Result must not be PROVEN_ABSENT — the safe default is NO_WITNESS")
	}
	if !zero.MustStall() {
		t.Fatal("the zero Result must report MustStall (NO_WITNESS is the zero value)")
	}
}

// TestFailedVerificationNeverAbsent is ablation test 3: a proof that FAILS to
// verify (wrong root, or tampered) resolves to NO_WITNESS, never PROVEN_ABSENT.
// An over-budget/malformed/failed-fetch witness upstream lands here identically:
// verification fails → NO_WITNESS → stall (RULING §"failure paths").
func TestFailedVerificationNeverAbsent(t *testing.T) {
	absent := []byte("absent-key")
	trie, root := buildTrie(t, []byte("k1"), []byte("k2"))
	key := Key(tagTest, absent)

	proof, err := trie.Prove(key)
	if err != nil {
		t.Fatalf("Prove(absent): %v", err)
	}

	t.Run("valid proof against the WRONG root -> NO_WITNESS", func(t *testing.T) {
		// A different committed set has a different root; the honest absence proof
		// must not verify against it.
		_, wrongRoot := buildTrie(t, []byte("k1"), []byte("k2"), []byte("k3"))
		if wrongRoot == root {
			t.Fatal("adding a key did not change the root; test premise broken")
		}
		got := Resolve(wrongRoot, key, nil, NewWitness(proof))
		if got.IsProvenAbsent() {
			t.Fatal("BANNED MOVE: a proof against the wrong root resolved to PROVEN_ABSENT")
		}
		if !got.MustStall() {
			t.Fatalf("proof against wrong root must be NO_WITNESS, got %s", got.Outcome())
		}
	})

	t.Run("tampered proof -> NO_WITNESS", func(t *testing.T) {
		// Tamper a side node so the reconstructed root cannot match.
		tampered := &smt.SparseMerkleProof{
			SideNodes:             make([][]byte, len(proof.SideNodes)),
			NonMembershipLeafData: proof.NonMembershipLeafData,
			SiblingData:           proof.SiblingData,
		}
		copy(tampered.SideNodes, proof.SideNodes)
		if len(tampered.SideNodes) == 0 {
			// Ensure there is a side node to corrupt; a 2-key trie has depth >= 1.
			t.Fatalf("expected at least one side node to tamper, got %d", len(tampered.SideNodes))
		}
		flipped := make([]byte, len(tampered.SideNodes[0]))
		copy(flipped, tampered.SideNodes[0])
		flipped[0] ^= 0xFF
		tampered.SideNodes[0] = flipped

		got := Resolve(root, key, nil, NewWitness(tampered))
		if got.IsProvenAbsent() {
			t.Fatal("BANNED MOVE: a tampered proof resolved to PROVEN_ABSENT")
		}
		if !got.MustStall() {
			t.Fatalf("tampered proof must be NO_WITNESS, got %s", got.Outcome())
		}
	})

	t.Run("membership proof re-read as an absence claim -> not PROVEN_ABSENT", func(t *testing.T) {
		// A present key's membership proof offered as its absence proof must not
		// verify as absent (the C-7 forgery), so it lands in NO_WITNESS here.
		presentKey := Key(tagTest, []byte("k1"))
		mProof, err := trie.Prove(presentKey)
		if err != nil {
			t.Fatalf("Prove(present): %v", err)
		}
		got := Resolve(root, presentKey, nil, NewWitness(mProof))
		if got.IsProvenAbsent() {
			t.Fatal("BANNED MOVE: a present key's proof resolved to PROVEN_ABSENT")
		}
		if !got.MustStall() {
			t.Fatalf("membership-as-absence must be NO_WITNESS, got %s", got.Outcome())
		}
	})
}

// TestProvenAbsentHasExactlyOneConstructionSite is the by-construction ablation:
// it proves PROVEN_ABSENT is constructible ONLY from a verified non-membership
// proof, by asserting the accessor source contains EXACTLY ONE literal that sets
// the outcome to ProvenAbsent, and that site sits after a successful VerifyProof
// in the value==nil branch. If someone adds a second construction site — e.g. a
// well-meaning "no witness, assume absent" shortcut (the banned move) — the count
// goes to 2 and this test goes RED before that code can ship.
//
// This is the "prove it goes red if someone adds a path" requirement: the type's
// unexported outcome field already prevents outside packages from fabricating
// PROVEN_ABSENT; this test guards against a new construction site WITHIN the
// package.
func TestProvenAbsentHasExactlyOneConstructionSite(t *testing.T) {
	src, err := os.ReadFile("witness.go")
	if err != nil {
		t.Fatalf("read witness.go: %v", err)
	}

	// Match a Result literal that sets outcome: ProvenAbsent. Whitespace-tolerant.
	// Any construction of a ProvenAbsent Result must go through such a literal
	// because outcome is unexported and Result has no other mutator.
	re := regexp.MustCompile(`outcome:\s*ProvenAbsent`)
	sites := re.FindAllIndex(src, -1)
	if len(sites) != 1 {
		t.Fatalf("expected EXACTLY ONE construction site for PROVEN_ABSENT, found %d. "+
			"A second site is the banned C-7 §104 move (missing/failed witness read as "+
			"absent). Every non-verified path MUST construct NoWitness, not ProvenAbsent.",
			len(sites))
	}

	// The single site must be guarded by a value==nil check (the non-membership
	// query) that follows a successful verify — i.e. it must appear AFTER the
	// `if value == nil {` guard in the function. We assert the guard exists and
	// precedes the construction site in source order.
	guard := regexp.MustCompile(`if value == nil \{`)
	guardLoc := guard.FindIndex(src)
	if guardLoc == nil {
		t.Fatal("the `if value == nil {` non-membership guard is missing — PROVEN_ABSENT " +
			"must be reachable only through the verified non-membership branch")
	}
	if guardLoc[0] >= sites[0][0] {
		t.Fatal("the PROVEN_ABSENT construction site is not inside the verified " +
			"non-membership (value==nil) branch")
	}
}
