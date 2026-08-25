package chain

import (
	"crypto/ed25519"
	"testing"

	"github.com/nerolabs/silt/ports"
)

// #555 hash-work oracle — the deep-drive crawl's mechanism, made deterministic.
//
// FIELD OBSERVATION (95d39e8-deep, 12-deep-heights window 22:00–22:30Z): per-height
// time grew ~82 s (maturing) → ~390 s (deep) and heights escaped r0→r1→r2. The
// eventloop watchdog attributed it: ChainReply processing blocked the single node
// thread 16–86 s per reply (324 slow + 131 HANG events, cost growing 2.4 s → 42 s
// with depth), which stretched the sweep timers (waited p50 18 s, p90 146 s) and
// starved the two-phase gather — the gather itself, measured on the same wire,
// completed in ~10 s (h74: new-view 22:16:50.9 → commit 22:17:00.5), well inside
// the 60 s r0. The HANG stacks pin the work: Reconcile → Append → ValidateProposal
// → validateBondRegs → recentBondRegNonces → blockByHash → Block.Hash →
// sha256.Sum256 — Hash() re-marshaled and re-hashed the full block body (BondRegs'
// ~1.5 MB proofs included) on EVERY call, blockByHash recomputed it per scan step,
// and recentBondRegNonces does up to K=8 such lookups per validated block. A deep
// Reconcile therefore paid O(depth × K × scan) full-body hashes — work that grows
// with height, exactly the crawl's scaling.
//
// THE ORACLE: hash WORK (actual computations, counted by blockHashComputes — a
// wall-clock bound would be flaky) during a cold-sync Reconcile of an n-block
// reg-carrying chain must be O(n): a handful of computes per block, not
// O(n × K × scan). RED-proven by neutering the memo (skip-the-memo-check):
// pre-fix code computes ~33×n hashes for this shape (798 at n=24 — budget 192);
// with the memo it is ~1×n (25). Head() and repeated Hash() must add ZERO work —
// the field's GetChainHead probes re-hashed the multi-MB tip on every ask.
func TestReconcileHashWorkIsLinear_555(t *testing.T) {
	prop := key(1)
	vals := []ed25519.PrivateKey{key(2), key(3), key(4), key(5)}
	src, g := objectiveChain(prop, vals, func(ports.NodeID) int64 { return 0 })

	// Mint n heights, each carrying a bond re-registration (the field's renewal
	// treadmill: every deep block carries reg work, which is what routes
	// validation through recentBondRegNonces' K-deep window walk).
	const n = 24
	minted := []Block{*g}
	prev := g.Hash()
	for h := uint64(1); h <= n; h++ {
		b := &Block{Version: 1, Height: h, Prev: prev, Entries: []ports.Entry{entry(byte(h))}}
		b.BondRegs = append(b.BondRegs, bondReg(prop, twoMiB, prev))
		Sign(b, prop)
		for _, v := range vals[:3] {
			b.Atts = append(b.Atts, Attest(b, v))
		}
		if err := src.Append(*b); err != nil {
			t.Fatalf("mint h%d: %v", h, err)
		}
		minted = append(minted, *b)
		prev = b.Hash()
	}

	// The wire path: a cold-syncing replica receives the chain as BYTES — every
	// decoded block arrives memo-less, exactly like a real ChainReply.
	fork, err := DecodeBlocks(EncodeBlocks(minted))
	if err != nil {
		t.Fatalf("roundtrip: %v", err)
	}
	rec, _ := objectiveChain(prop, vals, func(ports.NodeID) int64 { return 0 })

	start := blockHashComputes.Load()
	adopted, err := rec.Reconcile(fork)
	if err != nil || !adopted {
		t.Fatalf("reconcile: adopted=%v err=%v", adopted, err)
	}
	got := blockHashComputes.Load() - start

	// Budget: 8 computes per fork block. The memoized path needs ~1; pre-fix
	// code pays the K=8 window walk × scan-step re-hashes per block
	// (~33/block at this shape) and blows through this by 4×. Not tuned to
	// the exact GREEN count so an unrelated extra compute or two never flakes
	// it — the defect is an order of magnitude, not a unit.
	const budget = 8 * n
	if got > budget {
		t.Fatalf("#555 REPRODUCED: Reconcile of %d blocks did %d full-body hash computations (budget %d) — the O(depth × window × scan) re-hash that saturated the field's event loop (ChainReply 16–86 s stalls). Block.Hash must be memoized.", n, got, budget)
	}

	// The head-probe cost (GetChainHead handler → Head() → tip re-hash, 1602
	// saturated waits in the field window): once committed, asking for the head
	// must do ZERO hash work.
	start = blockHashComputes.Load()
	rec.Head()
	rec.Head()
	if extra := blockHashComputes.Load() - start; extra != 0 {
		t.Fatalf("Head() recomputed the tip hash %d times — the memo must make head probes free (#555)", extra)
	}
	t.Logf("#555: Reconcile of %d reg-carrying blocks = %d hash computations (budget %d); Head() free", n, got, budget)
}
