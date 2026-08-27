package smtspike

import (
	"crypto/sha256"
	"fmt"
	"hash"
	"math"
	"testing"

	"github.com/pokt-network/smt"
	"github.com/pokt-network/smt/kvstore/simplemap"
)

// RED home #2 — the incremental-cost oracle.
//
// The state-root certification's Q4 makes this the gate on the hot path:
//
//	Enforce with the incremental-cost oracle (RED home #2): count actual hash
//	computes per block over a wire-faithful run; RED = O(state), GREEN =
//	O(changed·log n) with an explicit budget. This is the correct gate and it
//	is the same discipline that closed #555.
//
// The scar it exists to stop is specific and already paid for once: #555 was
// `Hash()` re-marshaling the world on the hot path. A per-block full-tree
// recompute is that mistake wearing the keystone's clothes, and it would not
// show up as a correctness failure — only as a node that quietly falls over
// under load on the floor box.
//
// WHY COUNT DIGESTS RATHER THAN TIME: wall-clock on shared hardware carries ~2x
// run-to-run variance (measured on the floor box — see
// docs/thinking/2026-08-26-keystone-smt-spike-results.md), so a timing budget
// would be either too loose to catch a regression or flaky enough to be
// disabled. Digest COUNT is exact, deterministic, and machine-independent: the
// same assertion holds on a laptop and on a 1 vCPU box. smt's digestData is
// Write→Sum→Reset, so counting Sum() calls counts hash computations exactly.

// countingHasher wraps a real hash and counts completed digests.
type countingHasher struct {
	hash.Hash
	digests *int
}

func (c countingHasher) Sum(b []byte) []byte {
	*c.digests++
	return c.Hash.Sum(b)
}

// applyBlockDigests builds a trie of n keys, then measures the digests spent
// applying one block that changes `changed` keys. Only the apply phase is
// counted; the build is the boot-rebuild path, measured separately.
func applyBlockDigests(tb testing.TB, n, changed int) int {
	tb.Helper()
	var counter int
	trie := smt.NewSparseMerkleTrie(simplemap.NewSimpleMap(), countingHasher{
		Hash: sha256.New(), digests: &counter,
	})
	for i := 0; i < n; i++ {
		if err := trie.Update(keyAt(i), present); err != nil {
			tb.Fatalf("build Update(%d): %v", i, err)
		}
	}
	if err := trie.Commit(); err != nil {
		tb.Fatalf("build Commit: %v", err)
	}

	counter = 0 // the measured window: one block's worth of changes
	for j := 0; j < changed; j++ {
		if err := trie.Update(keyAt(n+j), present); err != nil {
			tb.Fatalf("apply Update: %v", err)
		}
	}
	if err := trie.Commit(); err != nil {
		tb.Fatalf("apply Commit: %v", err)
	}
	return counter
}

// digestBudget is the explicit budget the certification asks for.
//
// An SMT path update touches O(log n) nodes, and each node costs a small
// constant number of digests (leaf encode + inner-node combines along the
// path). budgetK is that constant, set with headroom above the measured value
// rather than guessed — see TestIncrementalCostReport for the measurements it
// is derived from.
//
// MEASURED (TestIncrementalCostReport, 1k–100k × 1–64 changed): digests per
// changed key per log2(n) lands between 0.85 and 1.61, worst case at
// changed=1 where the path cannot amortise shared prefixes. budgetK = 3 is
// therefore ~1.9x headroom over the measured worst case — tight enough to mean
// something, loose enough not to be flaky.
//
// The budget is strict on the SHAPE, not just the constant: it scales with
// changed·log2(n), so an O(state) implementation blows it by orders of
// magnitude however the constant is tuned. TestFullRecomputeBlowsTheBudget
// asserts exactly that, so the constant cannot be quietly inflated to hide a
// regression.
const budgetK = 3

func digestBudget(n, changed int) int {
	return int(float64(changed) * budgetK * math.Log2(float64(n)))
}

// TestIncrementalCostIsChangedTimesLogN is the GREEN assertion: the per-block
// digest count must fit changed·log(n), and — the load-bearing half — must NOT
// track the size of the state.
func TestIncrementalCostIsChangedTimesLogN(t *testing.T) {
	const changed = 64
	scales := []int{1_000, 10_000, 100_000}

	counts := make([]int, len(scales))
	for i, n := range scales {
		got := applyBlockDigests(t, n, changed)
		counts[i] = got
		budget := digestBudget(n, changed)
		t.Logf("n=%-7d changed=%d → %d digests (budget %d, %.1f per changed key)",
			n, changed, got, budget, float64(got)/float64(changed))
		if got > budget {
			t.Errorf("n=%d: %d digests exceeds the budget of %d for %d changed keys.\n"+
				"The update is not O(changed·log n) — this is the #555 scar "+
				"(re-hashing the world on the hot path) returning.", n, got, budget, changed)
		}
	}

	// The shape assertion. Between the smallest and largest scale the state
	// grows 100x; a log-shaped cost may not even double. An O(state)
	// implementation would grow ~100x and fail here even if someone inflated
	// budgetK to hide it.
	growth := float64(counts[len(counts)-1]) / float64(counts[0])
	const maxGrowth = 3.0
	t.Logf("state grew 100x; digest count grew %.2fx", growth)
	if growth > maxGrowth {
		t.Errorf("digest count grew %.2fx while state grew 100x (limit %.1fx).\n"+
			"Cost is tracking the SIZE OF STATE rather than the number of "+
			"CHANGED keys — the O(state) failure the certification's Q4 forbids.",
			growth, maxGrowth)
	}
}

// TestIncrementalCostScalesWithChangedNotState pins the other axis: at a fixed
// state size, doubling the changed keys should roughly double the work. If cost
// were dominated by a full recompute it would be flat in `changed` — which is
// the same defect seen from the other side, and a growth-only test would miss
// it.
func TestIncrementalCostScalesWithChangedNotState(t *testing.T) {
	const n = 10_000
	small := applyBlockDigests(t, n, 16)
	large := applyBlockDigests(t, n, 128)

	ratio := float64(large) / float64(small)
	t.Logf("n=%d: 16 changed → %d digests, 128 changed → %d digests (%.2fx for 8x the changes)",
		n, small, large, ratio)

	// 8x the changed keys should cost roughly 8x. Allow a wide band: shared
	// prefixes near the root are re-hashed once per commit, so the real factor
	// sits a little under 8.
	if ratio < 3.0 {
		t.Errorf("8x the changed keys cost only %.2fx the digests.\n"+
			"Per-block cost is nearly independent of how much changed, which is "+
			"the signature of a full-tree recompute dominating the measurement.", ratio)
	}
}

// TestFullRecomputeBlowsTheBudget is the RED demonstration. The oracle above is
// only meaningful if the defect it forbids actually trips it, so this measures
// what #555 did — rebuild the whole tree per block — and asserts it fails the
// same budget the incremental path passes.
func TestFullRecomputeBlowsTheBudget(t *testing.T) {
	const n, changed = 10_000, 64

	// The forbidden shape: rebuild every key, as a per-block Hash()-the-world
	// implementation would.
	var counter int
	rebuild := smt.NewSparseMerkleTrie(simplemap.NewSimpleMap(), countingHasher{
		Hash: sha256.New(), digests: &counter,
	})
	for i := 0; i < n+changed; i++ {
		if err := rebuild.Update(keyAt(i), present); err != nil {
			t.Fatalf("Update: %v", err)
		}
	}
	if err := rebuild.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	budget := digestBudget(n, changed)
	t.Logf("full recompute of %d keys: %d digests vs the per-block budget of %d (%.0fx over)",
		n+changed, counter, budget, float64(counter)/float64(budget))

	if counter <= budget {
		t.Fatalf("a full-tree recompute (%d digests) fits inside the per-block "+
			"budget (%d). The budget is too loose to catch the #555 scar, so the "+
			"GREEN assertions above prove nothing — tighten budgetK.", counter, budget)
	}
}

// TestIncrementalCostReport records the measurements budgetK is derived from,
// so the constant is evidence rather than a guess (build-immutable #7).
func TestIncrementalCostReport(t *testing.T) {
	t.Logf("%-9s %-8s %-9s %-12s %s", "n", "changed", "digests", "per-key", "per-key/log2(n)")
	for _, n := range []int{1_000, 10_000, 100_000} {
		for _, changed := range []int{1, 16, 64} {
			got := applyBlockDigests(t, n, changed)
			perKey := float64(got) / float64(changed)
			t.Logf("%-9d %-8d %-9d %-12.1f %.2f",
				n, changed, got, perKey, perKey/math.Log2(float64(n)))
		}
	}
	t.Log(fmt.Sprintf("budgetK is %d — compare against the last column, which is "+
		"the measured digests per changed key per log2(n).", budgetK))
}
