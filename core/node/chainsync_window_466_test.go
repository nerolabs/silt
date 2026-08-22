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

// #466 — chain-serve pagination. The serve side must never marshal the whole
// suffix into one buffer (the measured 144 MB EncodeBlocks OOM driver); the
// requester loops byte-bounded windows and reassembles; Reconcile's linkage
// validation backstops the whole scheme (a spliced window fails closed). The
// window and the reply deadline derive from the SAME two symbols
// (RequestSizeFloorBytesPerSec, requestSizeExtensionCap) so they cannot drift
// (PE ruling 2026-08-22).

// windowedPair builds two objective validators sharing a genesis, with the
// given RequestSizeFloorBytesPerSec (0 = legacy unbounded serve) per node —
// floorA for the first (the server in these tests), floorB for the second (the
// requester). Modeled on twoAgreeingValidators (#382 rig).
func windowedPair(t *testing.T, floorA, floorB int64) (*Node, *Node, *identity.Identity, *identity.Identity, *simclock.Scheduler) {
	t.Helper()
	sched := simclock.New()
	net := simnet.New(sched, 4, simnet.DefaultConfig())
	a1, a2 := identity.FromSeed(46601), identity.FromSeed(46602)
	anchors := map[ports.NodeID]bool{a1.NodeID(): true, a2.NodeID(): true}
	g := &chain.Block{Version: 1, Height: 0, Entries: []ports.Entry{mkEntry("genesis-466")}}
	chain.Sign(g, a1.Signer())
	ccfg := chain.Config{Quorum: 1, MinBond: 1 << 20, ByzantineQuorum: true, Anchors: anchors, AnchorQuorum: 1, MatureValidators: 99}
	mk := func(id *identity.Identity, floor int64) *Node {
		cfg := DefaultConfig()
		cfg.RequestSizeFloorBytesPerSec = floor
		nd := New(id.NodeID(), cfg, sched, net.Endpoint(id.NodeID()), memstore.New())
		nd.SetLedger(credit.New(50_000, 0))
		nd.EnableBond(id.Signer(), 2<<20)
		ch := chain.New(ccfg, func(ports.NodeID) int64 { return 0 })
		if err := ch.AppendGenesis(*g); err != nil {
			t.Fatal(err)
		}
		nd.EnableChain(ch, id.Signer())
		nd.EnableObjectiveChain()
		return nd
	}
	n1, n2 := mk(a1, floorA), mk(a2, floorB)
	n2.Bootstrap([]ports.NodeID{a1.NodeID()}, func() {})
	sched.Run()
	return n1, n2, a1, a2, sched
}

// growChain appends `count` signed+attested blocks to n1's chain (the pattern
// the #382 catch-up test uses), each carrying a distinct entry so blocks have
// real size and the encoded suffix spans several small windows.
func growChain(t *testing.T, n1 *Node, a1, a2 *identity.Identity, count int) {
	t.Helper()
	for i := 0; i < count; i++ {
		prev, next := n1.Chain().Head()
		b := &chain.Block{Version: 1, Height: next, Prev: prev,
			Entries: []ports.Entry{mkEntry("padding-block-for-window-sizing-466-#" + string(rune('A'+i%26)) + string(rune('a'+i/26)))}}
		chain.Sign(b, a1.Signer())
		b.Atts = []chain.Attestation{chain.Attest(b, a2.Signer())}
		if err := n1.Chain().Append(*b); err != nil {
			t.Fatalf("grow block %d: %v", i, err)
		}
	}
}

// TestMaxChainReplyBytesDerivation466: the window is DERIVED from the two
// deadline symbols, never a literal — floor × cap × ½ — so a window's transfer
// time at the floor (window/floor = cap/2 = 15 s) always fits inside the
// extension cap with the base deadline left for RTT + decode. No floor ⇒ 0
// (the legacy unbounded encode).
func TestMaxChainReplyBytesDerivation466(t *testing.T) {
	n, _ := aloneNode(t, 0)
	capSec := int(requestSizeExtensionCap / ports.Second)
	want := int(n.cfg.RequestSizeFloorBytesPerSec) * capSec / 2
	if got := n.maxChainReplyBytes(); got != want {
		t.Fatalf("window must derive from floor×cap/2: got %d want %d", got, want)
	}
	// At daemon defaults that is 3.75 MiB — sanity-pin the magnitude so a config
	// default change is a conscious re-derivation, not a silent drift.
	if got := n.maxChainReplyBytes(); got != 3_932_160 {
		t.Fatalf("window at defaults must be 3.75 MiB (256 KiB/s × 30 s / 2), got %d", got)
	}
	n.cfg.RequestSizeFloorBytesPerSec = 0
	if got := n.maxChainReplyBytes(); got != 0 {
		t.Fatalf("no floor must mean unbounded (0), got %d", got)
	}
}

// TestChainReplyDeadlineCoversWindow466: the PE's load-bearing finding — the
// size extension keys off the OUTBOUND payload, and MsgGetChain{Height} has
// empty Data, so before the fix a windowed reply had to arrive within the BASE
// RequestTimeout (2 s at daemon defaults): no 4 MiB window near the floor can
// meet that, stalling exactly the slow-but-honest WAN peers pagination serves.
// The requester knows the largest reply it will accept before it asks, so the
// deadline must be base + window/floor (≤ cap): with window = floor×cap/2 that
// is base + cap/2.
func TestChainReplyDeadlineCoversWindow466(t *testing.T) {
	n, _ := aloneNode(t, 0)
	base := n.cfg.RequestTimeout
	want := base + requestSizeExtensionCap/2
	if got := n.requestTimeoutFor(ports.Message{Kind: ports.MsgGetChain}); got != want {
		t.Fatalf("a windowed chain fetch must arm a reply-sized deadline (base %v + window/floor %v), got %v",
			base, requestSizeExtensionCap/2, got)
	}
	// Every other empty-payload request keeps the base deadline — the extension
	// is for the anticipated chain reply, not a blanket raise.
	if got := n.requestTimeoutFor(ports.Message{Kind: ports.MsgGetChainHead}); got != base {
		t.Fatalf("the head probe must keep the base deadline, got %v", got)
	}
	// No floor ⇒ no derivable window ⇒ base deadline (legacy behavior).
	n.cfg.RequestSizeFloorBytesPerSec = 0
	if got := n.requestTimeoutFor(ports.Message{Kind: ports.MsgGetChain}); got != base {
		t.Fatalf("without a floor the chain fetch must keep the base deadline, got %v", got)
	}
}

// TestChainServeReplyIsWindowBounded466 is the #466 defect made RED/GREEN: a
// server asked for its chain from height 0 must reply with a byte-bounded
// window, never the whole suffix in one buffer. Bound: the window, plus one
// block of slack (EncodeBlocksUpTo guarantees ≥1 block so a lone oversized
// block still moves — never stall).
func TestChainServeReplyIsWindowBounded466(t *testing.T) {
	const floor = 64 // window = 64 × 30 / 2 = 960 bytes — a few blocks per window
	n1, n2, a1, a2, sched := windowedPair(t, floor, floor)
	growChain(t, n1, a1, a2, 24)

	window := n1.maxChainReplyBytes()
	suffix := len(chain.EncodeBlocks(n1.Chain().Blocks(0)))
	if suffix <= window {
		t.Fatalf("rig error: the chain's encoded suffix (%d B) must exceed the window (%d B) for this test to bite", suffix, window)
	}

	var got int
	done := false
	n2.request(n1.ID(), ports.Message{Kind: ports.MsgGetChain, Height: 0},
		func(resp ports.Message, err error) {
			if err != nil || !resp.OK {
				t.Errorf("chain fetch failed: %v", err)
			}
			got, done = len(resp.Data), true
		})
	sched.Run()
	if !done {
		t.Fatal("chain fetch never completed")
	}
	oneBlock := len(chain.EncodeBlocks(n1.Chain().Blocks(0)[:1]))
	if got > window+oneBlock {
		t.Fatalf("#466: the serve reply must be window-bounded (≤ %d B + one-block slack %d B), got %d B — "+
			"the server marshaled the whole suffix into one buffer (the 144 MB OOM driver)", window, oneBlock, got)
	}
}

// TestWindowedCatchUpConverges466: correctness across the window loop — a
// requester several windows behind reassembles the full suffix across multiple
// round-trips and converges to the identical head, with every adopted block
// re-validated by Reconcile exactly as before. Multiple windows must actually
// have been used (else the rig proved nothing).
func TestWindowedCatchUpConverges466(t *testing.T) {
	const floor = 64
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
		t.Fatalf("windowed catch-up must adopt all %d blocks in one sweep, added=%d", grown, added)
	}
	h1, _ := n1.Chain().Head()
	h2, _ := n2.Chain().Head()
	if h1 != h2 {
		t.Fatalf("heads must converge after a windowed catch-up (n1=%x n2=%x)", h1[:4], h2[:4])
	}
	if n2.Stats.ChainSyncWindows < 3 {
		t.Fatalf("the catch-up must have spanned several windows (got %d) — a single window means the rig didn't exercise pagination", n2.Stats.ChainSyncWindows)
	}
}

// TestNewRequesterAcceptsWholeSuffixFromLegacyServer466: mixed-fleet, the
// server side un-upgraded (no floor ⇒ unbounded legacy encode). The windowed
// requester must ACCEPT the over-full reply and terminate — cap-and-accept,
// never reject (the transport's maxFrame bounds any single reply; Reconcile
// validates linkage). One window, full convergence.
func TestNewRequesterAcceptsWholeSuffixFromLegacyServer466(t *testing.T) {
	n1, n2, a1, a2, sched := windowedPair(t, 0 /* legacy server */, 64)
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
		t.Fatalf("the windowed requester must adopt a legacy server's whole-suffix reply, added=%d of %d", added, grown)
	}
	if n2.Stats.ChainSyncWindows != 1 {
		t.Fatalf("a legacy server's whole suffix is one (over-full) window, got %d", n2.Stats.ChainSyncWindows)
	}
}

// TestLegacyRequesterConvergesAgainstWindowedServer466 is the PE's mixed-fleet
// drill: an un-upgraded requester does ONE MsgGetChain fetch per sweep and
// reconciles whatever it got (this test replays that exact legacy loop body
// against a windowed server). Termination is head-match, which a truncated
// window cannot satisfy, so the legacy requester converges across SWEEPS —
// more round-trips, never a silent under-sync. Each window's adoption advances
// its FinalizedHeight (adopted blocks carry the quorum attestations), so
// windows are additive sweep over sweep.
func TestLegacyRequesterConvergesAgainstWindowedServer466(t *testing.T) {
	const floor = 64
	n1, n2, a1, a2, sched := windowedPair(t, floor, floor)
	const grown = 24
	growChain(t, n1, a1, a2, grown)

	legacySweep := func() (advanced bool) {
		before := n2.Chain().Len()
		reqHeight, _ := n2.Chain().FinalizedHeight()
		done := false
		n2.request(n1.ID(), ports.Message{Kind: ports.MsgGetChain, Height: reqHeight},
			func(resp ports.Message, err error) {
				defer func() { done = true }()
				if err != nil || !resp.OK {
					return
				}
				served, derr := chain.DecodeBlocks(resp.Data)
				if derr != nil || len(served) == 0 {
					return
				}
				full, ferr := n2.reconstructFork(served)
				if ferr != nil {
					return
				}
				n2.slashEquivocators(n2.Chain().Blocks(0), full)
				n2.Chain().Reconcile(full)
			})
		sched.Run()
		if !done {
			t.Fatal("legacy sweep never completed")
		}
		return n2.Chain().Len() > before
	}

	h1, _ := n1.Chain().Head()
	sweeps := 0
	for {
		h2, _ := n2.Chain().Head()
		if h1 == h2 {
			break
		}
		sweeps++
		if sweeps > grown+2 {
			t.Fatalf("legacy requester did not converge within %d sweeps against a windowed server — the mixed-fleet crawl became a stall", sweeps)
		}
		if !legacySweep() {
			t.Fatalf("legacy sweep %d adopted nothing — windows are not additive across sweeps (FinalizedHeight did not advance)", sweeps)
		}
	}
	if sweeps < 2 {
		t.Fatalf("rig error: convergence in %d sweep(s) means the suffix fit one window and the drill proved nothing", sweeps)
	}
	t.Logf("legacy requester converged in %d sweeps (%d blocks, window %d B)", sweeps, grown, n1.maxChainReplyBytes())
}

// TestSplicedWindowsFailClosed466 pins the safety hinge the whole scheme rests
// on: the ONLY state the windowed loop adds is the concatenation of windows, so
// its one new failure surface is a suffix spliced from INCONSISTENT serves (the
// peer reorged between windows). Such a splice must be rejected by the
// unchanged reconcile tail — linkage validation fails closed — leaving the
// local chain untouched; the next sweep retries against the settled peer.
func TestSplicedWindowsFailClosed466(t *testing.T) {
	const floor = 64
	n1, n2, a1, a2, sched := windowedPair(t, floor, floor)
	growChain(t, n1, a1, a2, 6)
	_ = sched

	// Window 1: the peer's real blocks [1..3]. Window 2: blocks [4..6] from a
	// DIFFERENT history — re-signed with a different entry at height 4, so its
	// Prev chain does not link onto window 1's last block.
	real := n1.Chain().Blocks(1)
	w1 := real[:3]
	prev := w1[len(w1)-1]                                                     // fork point: re-derive 4..6 on a divergent parent
	alt := &chain.Block{Version: 1, Height: prev.Height + 1, Prev: prev.Prev, // wrong parent: skips w1's last
		Entries: []ports.Entry{mkEntry("divergent-history-466")}}
	chain.Sign(alt, a1.Signer())
	alt.Atts = []chain.Attestation{chain.Attest(alt, a2.Signer())}
	spliced := append(append([]chain.Block{}, w1...), *alt)

	before := n2.Chain().Len()
	if full, ferr := n2.reconstructFork(spliced); ferr == nil {
		if ok, _ := n2.Chain().Reconcile(full); ok {
			t.Fatal("#466: a suffix spliced from inconsistent windows was ADOPTED — linkage validation must fail closed")
		}
	}
	if n2.Chain().Len() != before {
		t.Fatal("#466: a rejected splice must leave the local chain untouched")
	}
}
