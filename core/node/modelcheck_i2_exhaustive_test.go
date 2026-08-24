package node

import (
	"testing"

	"github.com/nerolabs/silt/adapters/identity"
	"github.com/nerolabs/silt/adapters/markstore"
	"github.com/nerolabs/silt/adapters/memstore"
	"github.com/nerolabs/silt/adapters/simclock"
	"github.com/nerolabs/silt/adapters/simnet"
	"github.com/nerolabs/silt/core/chain"
	"github.com/nerolabs/silt/ports"
)

// Consensus model-check — tier 2, I2 NEVER-SIGN-TWICE ACROSS RESTART, EXHAUSTIVE.
//
// The red-team #183 verdict's coverage caveat C-1: I2-across-restart was a
// SCENARIO test (modelcheck_i2_rounds_test.go: one signed slot, one competitor)
// rather than an exhaustive sweep. This promotes it: it drives the REAL
// watermark predicate (signAllowedAt) against a mark RELOADED from a persisted
// store — the restart — over EVERY (signed slot) × (competitor slot) pair in
// the {height, round, phase, hash} space, and asserts the exact monotone rule.
//
// THE INVARIANT (chainrole.go signAllowedAt / slotCompare): after a validator
// has signed slot S = (h, r, phase) over hash H, a fresh signature at slot C
// over hash H' is allowed IFF C is STRICTLY ABOVE S in the lexicographic
// (height, round, phase) order, OR C == S and H' == H (idempotent re-sign).
// Anything at or below S over a different hash is the double-sign I2 forbids —
// and the mark must enforce it identically after a crash, reading only the
// persisted watermark (never live memory, which a restart wipes).
//
// FAILING-FIRST: the non-persisted control (a blank store after "restart")
// allows the same-slot competitor — the pre-#397 crash-wipe self-slash — so the
// REFUSE half is exercising persistence, not live state.

// i2Reload builds an objective validator wired to `mark`, seeded so a fresh
// instance pointed at the same store is a faithful restart.
func i2Reload(t *testing.T, id *identity.Identity, g *chain.Block, cfg chain.Config, mark ports.SignMarkStore) *Node {
	t.Helper()
	sched := simclock.New()
	net := simnet.New(sched, 1, simnet.DefaultConfig())
	nd := New(id.NodeID(), DefaultConfig(), sched, net.Endpoint(id.NodeID()), memstore.New())
	ch := chain.New(cfg, func(ports.NodeID) int64 { return 0 })
	ch.SetBondVerifier(mcStubVerify)
	if err := ch.AppendGenesis(*g); err != nil {
		t.Fatalf("genesis: %v", err)
	}
	nd.EnableChain(ch, id.Signer())
	if err := nd.SetSignMarkStore(mark); err != nil {
		t.Fatalf("mark store: %v", err)
	}
	return nd
}

// i2Slot is a signing slot in the enumerated space. Heights {1,2}, rounds
// {0,1}, phases {prepare, precommit} — two of each dimension, enough to
// exercise every strictly-above / equal / strictly-below relation across all
// three lexicographic levels.
type i2Slot struct {
	height uint64
	round  uint64
	phase  uint8
}

func allI2Slots() []i2Slot {
	var out []i2Slot
	for _, h := range []uint64{1, 2} {
		for _, r := range []uint64{0, 1} {
			for _, p := range []uint8{chain.PhasePrepare, chain.PhasePrecommit} {
				out = append(out, i2Slot{height: h, round: r, phase: p})
			}
		}
	}
	return out
}

// strictlyAbove is the reference (height, round, phase) lexicographic order the
// mark enforces — computed independently of slotCompare so the oracle is a
// genuine check, not a tautology.
func strictlyAbove(c, s i2Slot) bool {
	if c.height != s.height {
		return c.height > s.height
	}
	if c.round != s.round {
		return c.round > s.round
	}
	return c.phase > s.phase
}

// TestModelCheck_I2_AcrossRestart_Exhaustive: for every SIGNED slot S, persist
// the mark, RESTART (reload a fresh node from the same store), then for every
// COMPETITOR slot C assert signAllowedAt matches the monotone rule — with a
// DIFFERENT hash (the equivocation case) and with the SAME hash (idempotent).
func TestModelCheck_I2_AcrossRestart_Exhaustive(t *testing.T) {
	id := identity.FromSeed(9600)
	g := &chain.Block{Version: 1, Height: 0, Entries: []ports.Entry{mkEntry("i2-exhaustive-g")}}
	chain.Sign(g, id.Signer())
	cfg := chain.Config{Quorum: 1, MinBond: 1 << 20, ByzantineQuorum: true, MatureValidators: 99}

	signedHash := ports.HashBytes([]byte("signed-block"))
	otherHash := ports.HashBytes([]byte("competitor-block"))

	slots := allI2Slots()
	checks := 0
	for _, s := range slots {
		// A node signs slot S, persisting the mark, then crashes.
		store := markstore.NewMem()
		signer := i2Reload(t, id, g, cfg, store)
		if !signer.recordSign(s.height, s.round, s.phase, signedHash) {
			t.Fatalf("setup: recordSign(%v) failed", s)
		}

		// RESTART: a fresh node reloads ONLY the persisted mark (live memory
		// gone). This is the object under test.
		restarted := i2Reload(t, id, g, cfg, store)
		if !restarted.signMarkSet {
			t.Fatalf("restart did not reload the persisted mark for %v", s)
		}

		for _, c := range slots {
			// Different-hash competitor: allowed IFF strictly above S.
			wantDiff := strictlyAbove(c, s)
			if got := restarted.signAllowedAt(c.height, c.round, c.phase, otherHash); got != wantDiff {
				t.Fatalf("I2 VIOLATION (across restart): signed %v, competitor %v (diff hash) → signAllowedAt=%v, want %v (a below-or-equal slot over a NEW hash is the double-sign the persisted mark must refuse)",
					s, c, got, wantDiff)
			}
			// Same-hash competitor: allowed IFF strictly above OR the exact slot
			// (idempotent re-sign / re-broadcast of the block already signed).
			wantSame := strictlyAbove(c, s) || c == s
			if got := restarted.signAllowedAt(c.height, c.round, c.phase, signedHash); got != wantSame {
				t.Fatalf("I2 VIOLATION (across restart): signed %v, competitor %v (SAME hash) → signAllowedAt=%v, want %v (the exact slot re-signed with the same block is idempotent; a lower slot is still refused)",
					s, c, got, wantSame)
			}
			checks += 2
		}
	}
	if checks != len(slots)*len(slots)*2 {
		t.Fatalf("enumeration miscount: %d checks, want %d", checks, len(slots)*len(slots)*2)
	}

	// FAILING-FIRST control: a BLANK store after restart (the pre-#397
	// crash-wipe) allows the same-slot different-hash competitor — proving the
	// REFUSE assertions above are the persisted mark's work.
	store := markstore.NewMem()
	signer := i2Reload(t, id, g, cfg, store)
	s := i2Slot{height: 1, round: 0, phase: chain.PhasePrepare}
	signer.recordSign(s.height, s.round, s.phase, signedHash)
	wiped := i2Reload(t, id, g, cfg, markstore.NewMem()) // blank store = lost mark
	if !wiped.signAllowedAt(s.height, s.round, s.phase, otherHash) {
		t.Fatal("control broken: with a wiped mark the same-slot competitor should be allowed — the exhaustive REFUSE is not exercising persistence")
	}
}
