package node

import (
	"testing"

	"github.com/nerolabs/silt/adapters/identity"
	"github.com/nerolabs/silt/adapters/memstore"
	"github.com/nerolabs/silt/adapters/simclock"
	"github.com/nerolabs/silt/adapters/simnet"
	"github.com/nerolabs/silt/core/chain"
	"github.com/nerolabs/silt/core/credit"
	"github.com/nerolabs/silt/ports"
)

// TestProposerPacksPendingSlashesUnderTheBytesCap is the R0.6 packing gate the Tester named
// as OWED (pr-r0.6-i5-evidence-recompute-3131d5a-verification-2026-09-03): with a backlog of
// genuine proofs whose total exceeds chain.SlashesBytesCap, the proposal (1) never exceeds
// the cap — otherwise every replica rejects it while the queue requeues it forever, the
// doomed-proposal loop — (2) carries at least one proof (forward progress), and (3) KEEPS
// the overflow queued rather than dropping it. A single proof that alone exceeds the cap
// can never commit on any replica, so it is DROPPED from the queue rather than embedded
// (embedding it would make every proposal by this node invalid for good).
func TestProposerPacksPendingSlashesUnderTheBytesCap(t *testing.T) {
	sched := simclock.New()
	net := simnet.New(sched, 1, simnet.DefaultConfig())
	ledger := credit.New(50_000, 0)
	repFn := func(n ports.NodeID) int64 { return ledger.Reputation(n) }

	ti := identity.FromSeed(1)
	tid := ti.NodeID()
	tn := New(tid, DefaultConfig(), sched, net.Endpoint(tid), memstore.New())
	tn.SetLedger(ledger)

	ch := chain.New(chain.DefaultConfig(), repFn)
	g := &chain.Block{Version: 1, Height: 0, Entries: []ports.Entry{mkEntry("g")}}
	prop := identity.FromSeed(2)
	chain.Sign(g, prop.Signer())
	if err := ch.AppendGenesis(*g); err != nil {
		t.Fatalf("genesis: %v", err)
	}
	tn.EnableChain(ch, ti.Signer())
	ledger.RecordBondChallenge(tid, ports.HashBytes([]byte{1}), 64<<20, true, 1)

	// Distinct culprits, each a genuine double-sign over two ~1 MiB blocks, so a few
	// proofs cross the cap. FindEquivocations returns one proof per culprit.
	fat := func(seed byte, chunks int) ports.Entry {
		e := mkEntry(string(rune('a' + seed)))
		e.ManifestChunks = make([]ports.ChunkID, chunks)
		for i := range e.ManifestChunks {
			e.ManifestChunks[i] = ports.HashBytes([]byte{seed, byte(i), byte(i >> 8), byte(i >> 16)})
		}
		return e
	}
	proofFor := func(seed int, chunks int) chain.Equivocation {
		v := identity.FromSeed(int64(100 + seed))
		mk := func(side byte) *chain.Block {
			b := &chain.Block{Version: 1, Height: 1, Prev: g.Hash(), Entries: []ports.Entry{fat(byte(seed)*2+side, chunks)}}
			chain.Sign(b, prop.Signer())
			b.Atts = append(b.Atts, chain.Attest(b, v.Signer()))
			return b
		}
		// The shared proposer signs both sides too, so pick the attester's proof.
		for _, e := range chain.FindEquivocations([]chain.Block{*g, *mk(0)}, []chain.Block{*g, *mk(1)}) {
			if e.CulpritID() == v.NodeID() {
				return e
			}
		}
		t.Fatalf("setup: no proof for culprit %d", seed)
		return chain.Equivocation{}
	}
	var backlog []chain.Equivocation
	for i := 0; ; i++ {
		backlog = append(backlog, proofFor(i, 32<<10)) // ~2 MiB per proof
		if chain.SlashesEncodedSize(backlog) > chain.SlashesBytesCap {
			break
		}
		if i > 64 {
			t.Fatal("setup: could not exceed the cap")
		}
	}
	// One proof that ALONE exceeds the cap (two ~9 MiB bodies): unslashable on any replica.
	oversized := proofFor(99, 9*32<<10)
	if chain.SlashesEncodedSize([]chain.Equivocation{oversized}) <= chain.SlashesBytesCap {
		t.Fatal("setup: the oversized proof must exceed the cap alone")
	}
	tn.pendingSlashes = append([]chain.Equivocation{oversized}, backlog...)

	deadID := identity.FromSeed(9).NodeID()
	_ = net.Endpoint(deadID)
	blk := &chain.Block{Version: 1, Height: 1, Prev: g.Hash(), Entries: []ports.Entry{mkEntry("carrier")}}
	tn.proposeBlock(blk, []ports.NodeID{deadID}, nil, 1, func(error) {})
	sched.Run()

	if n := chain.SlashesEncodedSize(blk.Slashes); n > chain.SlashesBytesCap {
		t.Fatalf("the proposal's Slashes field is %d bytes, over SlashesBytesCap %d — every replica would reject it while the queue requeued it forever (the doomed-proposal loop)", n, chain.SlashesBytesCap)
	}
	if len(blk.Slashes) == 0 {
		t.Fatal("the proposal carried no proof at all — a backlog over the cap must still make forward progress")
	}
	if len(blk.Slashes) >= len(backlog) {
		t.Fatalf("fixture: expected the backlog (%d proofs) not to fit in one block, but %d were embedded", len(backlog), len(blk.Slashes))
	}
	for _, e := range blk.Slashes {
		if e.CulpritID() == oversized.CulpritID() {
			t.Fatal("the proposal embedded a proof that alone exceeds the cap — it can never commit, and embedding it dooms every proposal by this node")
		}
	}
	if len(tn.pendingSlashes) != len(backlog) {
		t.Fatalf("queue after packing: %d, want %d — the overflow must stay QUEUED (carried, not dropped) and the oversized proof must be DROPPED", len(tn.pendingSlashes), len(backlog))
	}
}

// TestOverCapProofDoesNotSilenceLaterProofsByTheSameCulprit is the Researcher's V-1 gate
// (R0.6-SlashesBytesCap-value-security-face-DELTA-CERTIFICATION-2026-09-03): the
// once-per-culprit latch (#397 Q4-i) that stops the LOCAL penalty repeating must not also
// stop the ON-CHAIN queue. With the over-cap drop, a culprit whose FIRST detected proof is
// over the cap must still have a LATER, small proof queued for on-chain eviction — and the
// over-cap refusal is an announced operator line (S5, the Researcher's V-6), pinned here.
func TestOverCapProofDoesNotSilenceLaterProofsByTheSameCulprit(t *testing.T) {
	sched := simclock.New()
	net := simnet.New(sched, 1, simnet.DefaultConfig())
	ledger := credit.New(50_000, 0)
	repFn := func(n ports.NodeID) int64 { return ledger.Reputation(n) }

	ti := identity.FromSeed(1)
	tid := ti.NodeID()
	tn := New(tid, DefaultConfig(), sched, net.Endpoint(tid), memstore.New())
	tn.SetLedger(ledger)
	lg := &captureLog{clock: sched}
	tn.SetLogger(lg)

	ch := chain.New(chain.DefaultConfig(), repFn)
	g := &chain.Block{Version: 1, Height: 0, Entries: []ports.Entry{mkEntry("g")}}
	prop := identity.FromSeed(2)
	chain.Sign(g, prop.Signer())
	if err := ch.AppendGenesis(*g); err != nil {
		t.Fatalf("genesis: %v", err)
	}
	tn.EnableChain(ch, ti.Signer())

	culprit := identity.FromSeed(300)
	fat := func(seed byte, chunks int) ports.Entry {
		e := mkEntry(string(rune('a' + seed)))
		e.ManifestChunks = make([]ports.ChunkID, chunks)
		for i := range e.ManifestChunks {
			e.ManifestChunks[i] = ports.HashBytes([]byte{seed, byte(i), byte(i >> 8), byte(i >> 16)})
		}
		return e
	}
	// One culprit, two double-signs: the first over the cap (two ~9 MiB bodies), the second
	// small (two header-only bodies). Two different-height pairs would not share a culprit
	// proof, so both pairs sit at height 1 — different forks the node syncs in sequence.
	fork := func(seed byte, chunks int) []chain.Block {
		b := &chain.Block{Version: 1, Height: 1, Prev: g.Hash(), Entries: []ports.Entry{fat(seed, chunks)}}
		chain.Sign(b, prop.Signer())
		b.Atts = append(b.Atts, chain.Attest(b, culprit.Signer()))
		return []chain.Block{*g, *b}
	}
	big := 9 * 32 << 10
	tn.slashEquivocators(fork(0, big), fork(1, big)) // over-cap: must NOT queue, must WARN by name
	if len(tn.pendingSlashes) != 0 {
		t.Fatalf("an over-cap proof was queued (%d) — it can never commit", len(tn.pendingSlashes))
	}
	warned := false
	for _, ev := range lg.events {
		if ev == "equivocation proof exceeds SlashesBytesCap; not queued for on-chain slashing" {
			warned = true
		}
	}
	if !warned {
		t.Fatalf("S5: the over-cap refusal must be announced by its contracted line; got %v", lg.events)
	}

	tn.slashEquivocators(fork(2, 1), fork(3, 1)) // a later SMALL proof by the same culprit
	queued := 0
	for _, e := range tn.pendingSlashes {
		if e.CulpritID() == culprit.NodeID() {
			queued++
		}
	}
	if queued != 1 {
		t.Fatalf("V-1: a small proof by a culprit whose first proof was over the cap was not queued (queued=%d, pending=%d) — the once-per-culprit LOCAL latch must not silence the ON-CHAIN queue", queued, len(tn.pendingSlashes))
	}
}
