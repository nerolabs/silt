package node

import (
	"crypto/ed25519"
	"fmt"
	"testing"
	"time"

	"github.com/nerolabs/silt/adapters/identity"
	"github.com/nerolabs/silt/adapters/memstore"
	"github.com/nerolabs/silt/adapters/simclock"
	"github.com/nerolabs/silt/adapters/simnet"
	"github.com/nerolabs/silt/core/chain"
	"github.com/nerolabs/silt/core/credit"
	"github.com/nerolabs/silt/ports"
)

// #528 — the h≈56 liveness knee. A catch-up reconcile re-validated the ENTIRE
// chain from genesis in a throwaway replica (~1s per 1.5 MB reg block on the
// event loop), so at accumulated chain weight one reconcile outlasted the
// round durations, starved the sweeps, and h57 never committed (RC run
// 0de4b96-64567, 198 ChainReply watchdog HANGs; 2/2 deterministic with
// 94ef1e8-36901). The fix: a served window that provably EXTENDS our exact
// committed head (finality active; first new block's Prev chains from our
// head hash) is adopted through the normal Append commit path — validating
// only the new suffix — and never touches reconstructFork/Reconcile. Every
// other shape (divergence, equal-height fork, legacy no-finality config,
// peer behind) keeps the unchanged slow path.

// TestSuffixAppend_CatchUpSkipsGenesisReplay528 is the defect made RED/GREEN:
// a plain multi-window catch-up (the exact shape that wedged the RC run) must
// adopt the whole suffix WITHOUT a single slow-path Reconcile — zero genesis
// replays — while converging to the identical head.
func TestSuffixAppend_CatchUpSkipsGenesisReplay528(t *testing.T) {
	const floor = 64 // window = 960 bytes — a few blocks per window (the #466 rig)
	n1, n2, a1, a2, sched := windowedPair(t, floor, floor)
	const grown = 24
	growChain(t, n1, a1, a2, grown)

	var added int
	done := false
	n2.SyncChain([]ports.NodeID{n1.ID()}, func(a int, _ error) { added, done = a, true })
	sched.Run()
	if !done {
		t.Fatal("SyncChain did not complete")
	}
	if added != grown {
		t.Fatalf("catch-up must adopt all %d blocks in one sweep, added=%d", grown, added)
	}
	h1, _ := n1.Chain().Head()
	h2, _ := n2.Chain().Head()
	if h1 != h2 {
		t.Fatalf("heads must converge (n1=%x n2=%x)", h1[:4], h2[:4])
	}
	if n2.Stats.ChainSyncWindows < 3 {
		t.Fatalf("rig error: the catch-up must span several windows (got %d) or the loop-occupancy claim is untested", n2.Stats.ChainSyncWindows)
	}
	if got := n2.Stats.ChainSyncFullReconciles; got != 0 {
		t.Fatalf("#528: a pure-extension catch-up ran %d full genesis replay(s) — the O(height) reconcile cost is the liveness knee; it must be 0", got)
	}
	if got := n2.Stats.ChainSyncSuffixAppends; got != grown {
		t.Fatalf("#528: all %d blocks must arrive via the suffix-append fast path, got %d", grown, got)
	}
}

// TestSuffixAppend_DivergentHeadStillFullReconciles528 pins the boundary: a
// peer whose block at our head height DIFFERS from ours (a same-height fork —
// the shape the #382 comment warns is invisible to "give me blocks above my
// head") must NOT enter the fast path. It takes the unchanged slow path, where
// the finality gate refuses the reorg (D-1 prefer-stall) and the local chain
// is untouched.
func TestSuffixAppend_DivergentHeadStillFullReconciles528(t *testing.T) {
	const floor = 64
	n1, n2, a1, a2, sched := windowedPair(t, floor, floor)

	// Shared height 1, then divergence: n1 holds heights 1..4; n2 adopts only
	// n1's block 1 and then commits a DIFFERENT block at height 2 (distinct
	// entry ⇒ distinct hash), so n2's committed head conflicts with n1's history.
	growChain(t, n1, a1, a2, 4)
	shared := n1.Chain().Blocks(1)[0]
	if err := n2.Chain().Append(shared); err != nil {
		t.Fatalf("shared block: %v", err)
	}
	prev, next := n2.Chain().Head()
	alt := &chain.Block{Version: 1, Height: next, Prev: prev,
		Entries: []ports.Entry{mkEntry("divergent-head-528")}}
	chain.Sign(alt, a1.Signer())
	alt.Atts = []chain.Attestation{chain.Attest(alt, a2.Signer())}
	if err := n2.Chain().Append(*alt); err != nil {
		t.Fatalf("divergent block: %v", err)
	}

	beforeLen := n2.Chain().Len()
	beforeHead, _ := n2.Chain().Head()
	done := false
	n2.SyncChain([]ports.NodeID{n1.ID()}, func(int, error) { done = true })
	sched.Run()
	if !done {
		t.Fatal("SyncChain did not complete")
	}
	if got := n2.Stats.ChainSyncSuffixAppends; got != 0 {
		t.Fatalf("a divergent head must never take the suffix-append fast path, appended %d", got)
	}
	if got := n2.Stats.ChainSyncFullReconciles; got != 1 {
		t.Fatalf("a divergent head must take exactly one slow-path Reconcile, got %d", got)
	}
	if n2.Chain().Len() != beforeLen {
		t.Fatalf("the finality gate must refuse the reorg (D-1): len %d → %d", beforeLen, n2.Chain().Len())
	}
	if h, _ := n2.Chain().Head(); h != beforeHead {
		t.Fatal("the finality gate must leave the committed head untouched")
	}
}

// TestSuffixAppend_InvalidTailKeepsValidPrefix528: a lying peer's window that
// anchors correctly but carries an invalid later block stops the append at the
// first failure. Blocks appended before it are fully validated committed state
// (the same state as having synced one sweep earlier) and are KEPT; the bad
// block and everything after are refused.
func TestSuffixAppend_InvalidTailKeepsValidPrefix528(t *testing.T) {
	n1, n2, a1, a2, _ := windowedPair(t, 64, 64)
	growChain(t, n1, a1, a2, 3)

	served := n1.Chain().Blocks(1) // heights 1..3; Blocks returns copies
	served[2].Atts = nil           // strip the commit quorum from the tail
	k, ext, err := n2.appendExtension(served)
	if !ext {
		t.Fatal("a window anchored on our head must be judged an extension")
	}
	if err == nil {
		t.Fatal("the attestation-stripped tail must stop the append")
	}
	if k != 2 {
		t.Fatalf("the valid prefix must be kept: appended %d, want 2", k)
	}
	if got := n2.Chain().Len(); got != 3 { // genesis + heights 1..2
		t.Fatalf("chain must hold exactly the validated prefix, len=%d want 3", got)
	}
}

// heavyWorld builds count committed blocks each carrying a heavy stub bond reg
// (answerBytes of payload — the MATURING renewal-treadmill shape that created
// the knee), applied to every target chain. Modeled on the prune-rig commitTo.
func heavyWorld(t *testing.T, a1, a2 *identity.Identity, count int, answerBytes int, targets ...*chain.Chain) {
	t.Helper()
	for i := 0; i < count; i++ {
		prev, next := targets[0].Head()
		v := identity.FromSeed(int64(52800) + int64(next)) // a fresh bonded identity per height
		pub := append([]byte(nil), v.Signer().Public().(ed25519.PublicKey)...)
		reg := chain.NewBondReg(v.Signer(), ports.HashBytes(pub), 2<<20, make([]byte, answerBytes), prev, next)
		b := &chain.Block{Version: 1, Height: next, Prev: prev,
			Entries: []ports.Entry{mkEntry(fmt.Sprintf("heavy-528-%d", next))}, BondRegs: []chain.BondReg{reg}}
		chain.Sign(b, a1.Signer())
		b.Atts = []chain.Attestation{chain.Attest(b, a2.Signer())}
		for _, c := range targets {
			if err := c.Append(*b); err != nil {
				t.Fatalf("heavy block h%d: %v", next, err)
			}
		}
	}
}

// nodeFor528 wraps an existing chain in a minimal Node so appendExtension can
// be exercised directly (no network traffic is involved in the fast path).
func nodeFor528(t *testing.T, id *identity.Identity, ch *chain.Chain) *Node {
	t.Helper()
	sched := simclock.New()
	net := simnet.New(sched, 4, simnet.DefaultConfig())
	nd := New(id.NodeID(), DefaultConfig(), sched, net.Endpoint(id.NodeID()), memstore.New())
	nd.SetLedger(credit.New(50_000, 0))
	nd.EnableChain(ch, id.Signer())
	return nd
}

// TestSuffixAppend_NearHeadCatchUpCostIsDeltaNotHeight528 is the knee made
// measurable locally: a node ONE block behind a heavy-reg chain adopts it in
// O(delta) via the fast path, where the slow path re-validates the whole
// history from genesis (O(height) — the cost that outgrew the round durations
// at h≈56 in the field). The structural regression gate is the
// validation-count assert in TestSuffixAppend_CatchUpSkipsGenesisReplay528;
// this test pins the cost ratio on the exact near-head shape that wedged the
// RC run and logs the measured times as the citable local artifact.
func TestSuffixAppend_NearHeadCatchUpCostIsDeltaNotHeight528(t *testing.T) {
	const height = 60
	const answerBytes = 256 << 10
	a1, a2 := identity.FromSeed(52801), identity.FromSeed(52802)
	mkChain := func() *chain.Chain {
		ccfg := chain.Config{Quorum: 1, MinBond: 1 << 20, ByzantineQuorum: true,
			Anchors:      map[ports.NodeID]bool{a1.NodeID(): true, a2.NodeID(): true},
			AnchorQuorum: 1, MatureValidators: 99}
		c := chain.New(ccfg, func(ports.NodeID) int64 { return 0 })
		c.SetBondVerifier(mcStubVerify)
		g := &chain.Block{Version: 1, Height: 0, Entries: []ports.Entry{mkEntry("genesis-528-heavy")}}
		chain.Sign(g, a1.Signer())
		if err := c.AppendGenesis(*g); err != nil {
			t.Fatal(err)
		}
		return c
	}
	full, behindSlow, behindFast := mkChain(), mkChain(), mkChain()
	heavyWorld(t, a1, a2, height-1, answerBytes, full, behindSlow, behindFast) // shared 1..59
	heavyWorld(t, a1, a2, 1, answerBytes, full)                                // full alone holds 60

	// Slow path: the pre-#528 shape — reconstruct the genesis-rooted fork and
	// let Reconcile replay ALL of it in a throwaway replica.
	suffix := full.Blocks(uint64(height - 1)) // [59, 60] — what a suffix fetch serves
	fork := append(behindSlow.Blocks(0), suffix[1:]...)
	t0 := time.Now()
	ok, err := behindSlow.Reconcile(fork)
	slow := time.Since(t0)
	if !ok || err != nil {
		t.Fatalf("slow-path reconcile must adopt the extension: ok=%v err=%v", ok, err)
	}

	// Fast path: the same one-block catch-up through appendExtension.
	nd := nodeFor528(t, a2, behindFast)
	t1 := time.Now()
	k, ext, aerr := nd.appendExtension(suffix)
	fast := time.Since(t1)
	if !ext || aerr != nil || k != 1 {
		t.Fatalf("fast path must adopt exactly the new block: k=%d ext=%v err=%v", k, ext, aerr)
	}
	hs, _ := behindSlow.Head()
	hf, _ := behindFast.Head()
	if hs != hf {
		t.Fatal("both paths must converge to the identical head")
	}
	t.Logf("#528 near-head catch-up on a %d-block heavy-reg chain (%d KiB/reg): slow full replay %v, suffix append %v (%.0f×)",
		height, answerBytes>>10, slow, fast, float64(slow)/float64(fast))
	if slow < 3*fast {
		t.Fatalf("the slow path (%v) must cost at least 3× the fast path (%v) on a %d-block heavy chain — if not, the rig no longer exercises the O(height) replay", slow, fast, height)
	}
}
