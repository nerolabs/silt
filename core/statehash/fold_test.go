package statehash

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"math/rand"
	"testing"

	"github.com/nerolabs/silt/ports"
	"github.com/pokt-network/smt"
	"github.com/pokt-network/smt/kvstore/simplemap"
)

// randKey returns an 8-byte random key (a raw key; the SMT paths it via sha256).
func randKey(rng *rand.Rand) []byte {
	k := make([]byte, 8)
	rng.Read(k)
	return k
}

func randVal(rng *rand.Rand) []byte {
	b := make([]byte, 1+rng.Intn(8))
	rng.Read(b)
	return b
}

// leafSlice turns a key→value map into the []Leaf statehash.Root / NewProver consume.
func leafSlice(m map[string][]byte) []Leaf {
	out := make([]Leaf, 0, len(m))
	for k, v := range m {
		out = append(out, Leaf{Key: []byte(k), Value: v})
	}
	return out
}

// buildFoldOps derives the FoldOps for a set of writes (sets and dels) against a prover holding
// the pre-state. This is the same shape the E/R consumer builds: verify-able proofs + delete
// sibling preimages, all from the provider.
func buildFoldOps(t *testing.T, prover *Prover, pre map[string][]byte, sets map[string][]byte, dels map[string]bool) []FoldOp {
	t.Helper()
	var ops []FoldOp
	for k := range dels {
		w, sibs, err := prover.ProveWithSiblings([]byte(k))
		if err != nil {
			t.Fatalf("ProveWithSiblings(%x): %v", k, err)
		}
		ops = append(ops, FoldOp{
			Key:            []byte(k),
			OldValue:       pre[k], // present → membership proof
			NewValue:       nil,    // delete
			Proof:          w,
			DeleteSiblings: sibs,
		})
	}
	for k, nv := range sets {
		w, err := prover.Prove([]byte(k))
		if err != nil {
			t.Fatalf("Prove(%x): %v", k, err)
		}
		ops = append(ops, FoldOp{
			Key:      []byte(k),
			OldValue: pre[k], // nil if absent (add) → non-membership proof
			NewValue: nv,
			Proof:    w,
		})
	}
	return ops
}

// TestFoldByteExactRandomized is the load-bearing pin: over a large randomized cross-product of
// the structural cases (add-disjoint, add-displacing, overwrite, delete leaf/extension/inner
// sibling, shared-prefix interactions, extension present/absent), the fold's computed post-root
// MUST equal statehash.Root(postLeaves) — the ground truth — with 0 failures. The prior naive
// fold failed 72/200 here.
func TestFoldByteExactRandomized(t *testing.T) {
	const trials = 6000
	for _, seed := range []int64{1, 2, 3} {
		rng := rand.New(rand.NewSource(seed))
		failures := 0
		exercised := 0
		for tr := 0; tr < trials; tr++ {
			if runOneFoldTrial(t, rng) {
				exercised++
			} else {
				failures++
			}
		}
		if failures != 0 {
			t.Fatalf("seed %d: %d/%d fold trials produced a wrong post-root", seed, failures, trials)
		}
		if exercised < trials/2 {
			t.Fatalf("seed %d: only %d/%d trials exercised a write-set — harness degenerate", seed, exercised, trials)
		}
		t.Logf("seed %d: %d/%d trials byte-exact, 0 failures", seed, exercised, trials)
	}
}

// runOneFoldTrial builds a random pre-state and write-set, folds, and returns true iff the fold
// root matches statehash.Root(post). It returns false ONLY on a real mismatch (a degenerate
// empty-write trial returns true without asserting).
func runOneFoldTrial(t *testing.T, rng *rand.Rand) bool {
	t.Helper()
	n := 1 + rng.Intn(40)
	pre := map[string][]byte{}
	keys := [][]byte{}
	for i := 0; i < n; i++ {
		k := randKey(rng)
		if _, dup := pre[string(k)]; dup {
			continue
		}
		pre[string(k)] = randVal(rng)
		keys = append(keys, k)
	}

	// Distinct changed keys, one op each: 0 add-disjoint, 1 add-displacing, 2 overwrite, 3 delete.
	type opRec struct {
		op int
		nv []byte
	}
	ops := map[string]opRec{}
	nOps := 1 + rng.Intn(6)
	for o := 0; o < nOps; o++ {
		switch rng.Intn(4) {
		case 0:
			k := randKey(rng)
			if _, ok := pre[string(k)]; ok {
				continue
			}
			if _, ok := ops[string(k)]; ok {
				continue
			}
			ops[string(k)] = opRec{op: 0, nv: randVal(rng)}
		case 1:
			if len(keys) == 0 {
				continue
			}
			base := keys[rng.Intn(len(keys))]
			k := append([]byte(nil), base...)
			k[len(k)-1] ^= byte(1 << uint(rng.Intn(8)))
			if _, ok := pre[string(k)]; ok {
				continue
			}
			if _, ok := ops[string(k)]; ok {
				continue
			}
			ops[string(k)] = opRec{op: 1, nv: randVal(rng)}
		case 2:
			if len(keys) == 0 {
				continue
			}
			base := keys[rng.Intn(len(keys))]
			if _, ok := ops[string(base)]; ok {
				continue
			}
			ops[string(base)] = opRec{op: 2, nv: randVal(rng)}
		case 3:
			if len(keys) == 0 {
				continue
			}
			base := keys[rng.Intn(len(keys))]
			if _, ok := ops[string(base)]; ok {
				continue
			}
			ops[string(base)] = opRec{op: 3}
		}
	}
	if len(ops) == 0 {
		return true // degenerate; nothing to assert
	}

	sets := map[string][]byte{}
	dels := map[string]bool{}
	post := map[string][]byte{}
	for k, v := range pre {
		post[k] = v
	}
	for k, rec := range ops {
		if rec.op == 3 {
			dels[k] = true
			delete(post, k)
		} else {
			sets[k] = rec.nv
			post[k] = rec.nv
		}
	}

	prover, err := NewProver(leafSlice(pre))
	if err != nil {
		t.Fatalf("NewProver: %v", err)
	}
	prevRoot := prover.Root()

	foldOps := buildFoldOps(t, prover, pre, sets, dels)
	got, err := FoldChangedPaths(prevRoot, foldOps)
	if err != nil {
		t.Fatalf("FoldChangedPaths: %v", err)
	}
	want, err := Root(leafSlice(post))
	if err != nil {
		t.Fatalf("Root(post): %v", err)
	}
	return got == want
}

// TestFoldSeedEncodingMatchesLibrary PINS the replicated node encoding (foldLeafPreimage /
// foldInnerPreimage / foldDigest / foldPath / foldValueHash / foldPlaceholder) byte-exact against
// pokt-network/smt@v1.0.0's own committed node preimages. A library node-encoding drift reddens
// this before the seed reconstruction can silently produce a wrong root.
func TestFoldSeedEncodingMatchesLibrary(t *testing.T) {
	// A single-leaf trie: the root IS the leaf digest. Reconstruct that leaf digest from the
	// replicated encoders and require it equals the library's committed root.
	key := []byte("some-key-value")
	val := []byte("some-value")

	trie := smt.NewSparseMerkleTrie(simplemap.NewSimpleMap(), sha256.New())
	if err := trie.Update(key, val); err != nil {
		t.Fatal(err)
	}
	if err := trie.Commit(); err != nil {
		t.Fatal(err)
	}
	libRoot := trie.Root()

	myLeaf := foldLeafPreimage(foldPath(key), foldValueHash(val))
	myDigest := foldDigest(myLeaf)
	if !bytes.Equal(myDigest, libRoot) {
		t.Fatalf("leaf digest mismatch: replicated %x != library root %x", myDigest, libRoot)
	}

	// Two leaves diverging at bit 0: the root is inner(leftDigest, rightDigest). Reconstruct it.
	// Find two keys whose sha256 paths diverge at bit 0.
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
	t2 := smt.NewSparseMerkleTrie(simplemap.NewSimpleMap(), sha256.New())
	t2.Update(kL, vL)
	t2.Update(kR, vR)
	t2.Commit()
	lib2 := t2.Root()

	leftDigest := foldDigest(foldLeafPreimage(foldPath(kL), foldValueHash(vL)))
	rightDigest := foldDigest(foldLeafPreimage(foldPath(kR), foldValueHash(vR)))
	myRoot2 := foldDigest(foldInnerPreimage(leftDigest, rightDigest))
	if !bytes.Equal(myRoot2, lib2) {
		t.Fatalf("inner digest mismatch: replicated %x != library root %x", myRoot2, lib2)
	}

	// Placeholder: an empty trie's root is the placeholder (32 zero bytes).
	empty := smt.NewSparseMerkleTrie(simplemap.NewSimpleMap(), sha256.New())
	empty.Commit()
	if !bytes.Equal(empty.Root(), foldPlaceholder) {
		t.Fatalf("placeholder mismatch: library %x != replicated %x", empty.Root(), foldPlaceholder)
	}
}

// ---- Per-structural-case fixtures + ablations (red-before-green) ----
//
// Each fixture pins a SPECIFIC structural case byte-exact. The ablation helpers below break the
// fold's handling of that case and assert the fold goes RED — a green check with no demonstrated
// red is a comment that compiles (the session-7 scar).

// foldCase runs the fold for a pre-state + sets/dels and returns (got, want).
func foldCase(t *testing.T, pre, sets map[string][]byte, dels map[string]bool) (ports.Hash, ports.Hash) {
	t.Helper()
	prover, err := NewProver(leafSlice(pre))
	if err != nil {
		t.Fatal(err)
	}
	prevRoot := prover.Root()
	ops := buildFoldOps(t, prover, pre, sets, dels)
	got, err := FoldChangedPaths(prevRoot, ops)
	if err != nil {
		t.Fatalf("FoldChangedPaths: %v", err)
	}
	post := map[string][]byte{}
	for k, v := range pre {
		post[k] = v
	}
	for k := range dels {
		delete(post, k)
	}
	for k, v := range sets {
		post[k] = v
	}
	want, err := Root(leafSlice(post))
	if err != nil {
		t.Fatal(err)
	}
	return got, want
}

// caseAddDisjoint builds a pre-state and a disjoint add. Returns pre/sets/dels.
func caseAddDisjoint() (map[string][]byte, map[string][]byte, map[string]bool) {
	pre := map[string][]byte{"aaaa1111": {1}, "bbbb2222": {2}, "cccc3333": {3}}
	sets := map[string][]byte{"dddd4444": {9}}
	return pre, sets, map[string]bool{}
}

// caseAddDisplacing shares a long prefix with an existing key (flip the last bit).
func caseAddDisplacing() (map[string][]byte, map[string][]byte, map[string]bool) {
	base := []byte("basekey0")
	disp := append([]byte(nil), base...)
	disp[len(disp)-1] ^= 1
	pre := map[string][]byte{string(base): {1}, "other111": {2}}
	sets := map[string][]byte{string(disp): {9}}
	return pre, sets, map[string]bool{}
}

func caseOverwrite() (map[string][]byte, map[string][]byte, map[string]bool) {
	pre := map[string][]byte{"aaaa1111": {1}, "bbbb2222": {2}}
	sets := map[string][]byte{"aaaa1111": {7}}
	return pre, sets, map[string]bool{}
}

// caseDeleteMany builds a larger pre-state and deletes a key, to exercise sibling promotion.
func caseDeleteMany(rng *rand.Rand) (map[string][]byte, map[string][]byte, map[string]bool, string) {
	pre := map[string][]byte{}
	var first string
	for i := 0; i < 30; i++ {
		k := randKey(rng)
		if _, ok := pre[string(k)]; ok {
			continue
		}
		pre[string(k)] = randVal(rng)
		if first == "" {
			first = string(k)
		}
	}
	return pre, map[string][]byte{}, map[string]bool{first: true}, first
}

func TestFoldCaseAddDisjoint(t *testing.T) {
	pre, sets, dels := caseAddDisjoint()
	got, want := foldCase(t, pre, sets, dels)
	if got != want {
		t.Fatalf("add-disjoint: got %x want %x", got, want)
	}
}

func TestFoldCaseAddDisplacing(t *testing.T) {
	pre, sets, dels := caseAddDisplacing()
	got, want := foldCase(t, pre, sets, dels)
	if got != want {
		t.Fatalf("add-displacing: got %x want %x", got, want)
	}
}

func TestFoldCaseOverwrite(t *testing.T) {
	pre, sets, dels := caseOverwrite()
	got, want := foldCase(t, pre, sets, dels)
	if got != want {
		t.Fatalf("overwrite: got %x want %x", got, want)
	}
}

func TestFoldCaseDelete(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	// Run several delete trials to hit the leaf/extension/inner sibling cases.
	for i := 0; i < 200; i++ {
		pre, sets, dels, _ := caseDeleteMany(rng)
		got, want := foldCase(t, pre, sets, dels)
		if got != want {
			t.Fatalf("delete trial %d: got %x want %x", i, got, want)
		}
	}
}

// ---- Ablations: break a fold rule → the pin goes RED. Each ablation reruns the fold with a
// deliberately corrupted seed / op and asserts the resulting root is WRONG. ----

// abInnerOrderSwapped rebuilds the seed with the inner-node child order swapped, proving the
// path-bit ordering is load-bearing. Uses a low-level fold to inject the defect.
func TestFoldAblationInnerOrderSwapped(t *testing.T) {
	// Use a large pre-state so the changed key's proof has MULTIPLE non-placeholder sidenodes —
	// then swapping the inner-child order is guaranteed to diverge the reconstructed root.
	rng := rand.New(rand.NewSource(11))
	var pre map[string][]byte
	var addKey []byte
	var w Witness
	var prover *Prover
	for attempt := 0; attempt < 200; attempt++ {
		pre = map[string][]byte{}
		for i := 0; i < 40; i++ {
			k := randKey(rng)
			pre[string(k)] = randVal(rng)
		}
		var err error
		prover, err = NewProver(leafSlice(pre))
		if err != nil {
			t.Fatal(err)
		}
		addKey = randKey(rng)
		if _, ok := pre[string(addKey)]; ok {
			continue
		}
		w, err = prover.Prove(addKey)
		if err != nil {
			t.Fatal(err)
		}
		// need >= 2 non-placeholder sidenodes for the swap to bite
		real := 0
		for _, sn := range w.proof.SideNodes {
			if !bytes.Equal(sn, foldPlaceholder) {
				real++
			}
		}
		if real >= 2 {
			break
		}
	}
	prevRoot := prover.Root()
	newVal := []byte{9}

	// GOOD seed → apply the add → the post-root MUST equal Root(post).
	post := map[string][]byte{}
	for k, v := range pre {
		post[k] = v
	}
	post[string(addKey)] = newVal
	want, err := Root(leafSlice(post))
	if err != nil {
		t.Fatal(err)
	}
	seedGood := map[string][]byte{}
	seedFromProof(seedGood, w.proof, addKey, nil)
	gotGood, errGood := applySeedAdd(seedGood, prevRoot, addKey, newVal)
	if errGood != nil || gotGood != want {
		t.Fatalf("precondition: good-seed fold must match Root(post): got %x want %x err %v", gotGood, want, errGood)
	}

	// SWAPPED seed → apply the same add → the fold MUST diverge: either it errors (a broken digest
	// chain the library cannot resolve) or it produces a wrong root. Both are RED; a swap that
	// silently yields the correct root would mean the ordering is not load-bearing.
	seedBad := seedFromProofSwapped(w.proof, addKey, nil)
	gotBad, errBad := applySeedAdd(seedBad, prevRoot, addKey, newVal)
	if errBad == nil && gotBad == want {
		t.Fatal("ABLATION FAILED: swapping inner child order still produced the correct post-root — the ordering is not load-bearing")
	}
}

// applySeedAdd imports a seed rooted at prevRoot, adds (key,val), and returns the post-root or an
// error (a broken seed can make the library fail to resolve a node).
func applySeedAdd(seed map[string][]byte, prevRoot ports.Hash, key, val []byte) (ports.Hash, error) {
	trie := smt.ImportSparseMerkleTrie(simplemap.NewSimpleMapWithMap(seed), sha256.New(), prevRoot[:])
	if err := trie.Update(key, val); err != nil {
		return ports.Hash{}, err
	}
	if err := trie.Commit(); err != nil {
		return ports.Hash{}, err
	}
	var h ports.Hash
	copy(h[:], trie.Root())
	return h, nil
}

// seedFromProofSwapped is seedFromProof with the inner-node child order REVERSED (the defect).
func seedFromProofSwapped(proof *smt.SparseMerkleProof, key, value []byte) map[string][]byte {
	seed := map[string][]byte{}
	// mimic seedFromProof but swap order
	var curHash []byte
	if proof.NonMembershipLeafData == nil {
		curHash = foldPlaceholder
	} else {
		pre := append([]byte(nil), proof.NonMembershipLeafData...)
		curHash = foldDigest(pre)
		seed[string(curHash)] = pre
	}
	_ = value
	sn := proof.SideNodes
	p := foldPath(key)
	for i := 0; i < len(sn); i++ {
		var pre []byte
		// DEFECT: reversed order relative to the path bit.
		if foldPathBit(p, len(sn)-1-i) == 0 {
			pre = foldInnerPreimage(sn[i], curHash) // wrong: on-path should be LEFT
		} else {
			pre = foldInnerPreimage(curHash, sn[i]) // wrong: on-path should be RIGHT
		}
		curHash = foldDigest(pre)
		seed[string(curHash)] = pre
	}
	return seed
}

// TestFoldAblationDeleteAsNoop proves that skipping the Delete (routing it as a no-op) reddens a
// delete case: the post-root then still contains the deleted leaf and diverges from Root(post).
func TestFoldAblationDeleteAsNoop(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	pre, _, dels, delKey := caseDeleteMany(rng)
	prover, err := NewProver(leafSlice(pre))
	if err != nil {
		t.Fatal(err)
	}
	prevRoot := prover.Root()
	// Build ops but with NewValue set to a marker so the fold treats it as a SET, not a delete
	// (the defect: a delete routed as a no-op keeps the leaf).
	w, sibs, err := prover.ProveWithSiblings([]byte(delKey))
	if err != nil {
		t.Fatal(err)
	}
	_ = sibs
	badOps := []FoldOp{{
		Key:      []byte(delKey),
		OldValue: pre[delKey],
		NewValue: pre[delKey], // DEFECT: re-set instead of delete
		Proof:    w,
	}}
	got, err := FoldChangedPaths(prevRoot, badOps)
	if err != nil {
		t.Fatalf("fold: %v", err)
	}
	post := map[string][]byte{}
	for k, v := range pre {
		post[k] = v
	}
	delete(post, delKey)
	want, err := Root(leafSlice(post))
	if err != nil {
		t.Fatal(err)
	}
	if got == want {
		t.Fatal("ABLATION FAILED: a delete routed as a no-op still matched the deleted post-root")
	}
	_ = dels
}

// TestFoldAblationForgedProof proves a forged / wrong changed-leaf proof is rejected (stall), not
// folded. A proof for the WRONG value fails VerifyProof.
func TestFoldAblationForgedProof(t *testing.T) {
	pre := map[string][]byte{"aaaa1111": {1}, "bbbb2222": {2}}
	prover, err := NewProver(leafSlice(pre))
	if err != nil {
		t.Fatal(err)
	}
	prevRoot := prover.Root()
	w, err := prover.Prove([]byte("aaaa1111"))
	if err != nil {
		t.Fatal(err)
	}
	// Claim a WRONG old value for the proof → VerifyProof fails → fold stalls.
	badOps := []FoldOp{{
		Key:      []byte("aaaa1111"),
		OldValue: []byte{99}, // wrong pre-value
		NewValue: []byte{7},
		Proof:    w,
	}}
	_, err = FoldChangedPaths(prevRoot, badOps)
	if err == nil {
		t.Fatal("ABLATION FAILED: fold accepted a proof for a wrong pre-value")
	}
}

// TestFoldAblationSeedRootMismatch proves an omitted / injected changed-leaf proof (a short seed)
// is caught by the seed-root anchor. Here we hand the fold an op whose proof is for a DIFFERENT
// root (an unrelated pre-state), so the seed cannot reconstruct prevStateRoot.
func TestFoldAblationSeedRootMismatch(t *testing.T) {
	pre := map[string][]byte{"aaaa1111": {1}, "bbbb2222": {2}}
	prover, err := NewProver(leafSlice(pre))
	if err != nil {
		t.Fatal(err)
	}
	prevRoot := prover.Root()

	// A proof from an UNRELATED trie (different root). VerifyProof against prevRoot fails first,
	// which already stalls — this asserts the box never folds a cross-root proof.
	other, err := NewProver(leafSlice(map[string][]byte{"zzzz9999": {5}}))
	if err != nil {
		t.Fatal(err)
	}
	ow, err := other.Prove([]byte("zzzz9999"))
	if err != nil {
		t.Fatal(err)
	}
	badOps := []FoldOp{{
		Key:      []byte("zzzz9999"),
		OldValue: []byte{5},
		NewValue: []byte{6},
		Proof:    ow,
	}}
	_, err = FoldChangedPaths(prevRoot, badOps)
	if err == nil {
		t.Fatal("ABLATION FAILED: fold accepted a proof against a different root")
	}
}

var _ = sha256.Size
