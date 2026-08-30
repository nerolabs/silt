package node

import (
	"testing"

	"github.com/nerolabs/silt/adapters/simnet"
	"github.com/nerolabs/silt/core/chain"
	"github.com/nerolabs/silt/ports"
)

// Consensus model-check — the UNIFIED step-oracle (#406).
//
// The spec (docs/design/consensus-model-check.md §"The oracle", DoD, First steps)
// names a piece the per-invariant oracles do NOT provide:
//
//	"Write the I1–I5 oracle as a single assertInvariants(replicas) called each step."
//
// The shipped tier already has strong EXHAUSTIVE per-invariant oracles
// (modelcheck_test.go I1-launch, modelcheck_i3_test.go I1-mature/I3,
// modelcheck_i2_exhaustive_test.go I2, modelcheck_i4_liveness_test.go +
// modelcheck_441/451 I4, modelcheck_i5_accountable_test.go I5). Each drives its
// OWN bespoke scenario and asserts its OWN invariant inside it. What was missing
// — verified by `grep -rn assertInvariants core/` returning nothing on
// 61c75eb — is a single CROSS-CUTTING monitor that runs over the LIVE
// multi-replica state of an ARBITRARY driven scenario and asserts the two
// continuously-observable invariants after EVERY delivery step. That is the
// thing that turns a scenario driver into a property harness: an invariant break
// surfacing in a scenario written to probe a DIFFERENT invariant is now watched.
//
// SCOPE (honest, S5). This monitor covers the two invariants that are pure
// functions of observable cross-replica state:
//   - I1 (AGREEMENT): no two replicas expose DIFFERENT finalized block hashes at
//     the same height. This is I1's observable consequence — an intersecting
//     finality quorum admits at most one final block per height, so every replica
//     that finalized height h agrees on its hash. A non-intersecting quorum shows
//     here as two replicas final-disagreeing (the #357/#397/#402 fork face).
//   - I5 (ACCOUNTABLE SAFETY): no honest replica is ever slashed, observed live
//     via OnSlash across the whole schedule (the #397 honest-self-slash face).
// I2/I3/I4 are predicate/liveness properties owned exhaustively by their dedicated
// oracles; folding them in here would add no red those oracles do not already
// own, so this monitor does not reimplement them (simplicity — cover what the
// step-monitor genuinely adds).
//
// TEST-INFRA ONLY: no consensus rule, validity predicate, or apply() change.

// invReplica is the observable surface the unified oracle reads from one replica:
// its finalized suffix (for the I1 agreement check) and whether an honest node it
// watches was slashed (for I5). It is satisfied by *Node in the real scenario and
// by a hand-built stub in the ablation, so the oracle is exercised against both
// real and injected divergence.
type invReplica interface {
	// finalizedBlocks returns this replica's finalized block suffix (height h ↦
	// block), keyed by height. Only heights this replica treats as irreversibly
	// final appear — the I1 agreement check compares hashes only at heights BOTH
	// replicas finalized.
	finalizedBlocks() map[uint64]ports.Hash
	// honestSlashed reports whether this replica observed a slash of a node the
	// scenario asserts is HONEST (I5 accountable safety).
	honestSlashed() bool
	label() string
}

// assertInvariants is THE unified oracle — the single check the spec calls for,
// run after every scheduler step over the live replica set. It halts on the first
// violation with the offending replicas, height, and hashes (the seed+schedule
// repro is the caller's frozen deterministic driver).
func assertInvariants(t *testing.T, replicas []invReplica) {
	t.Helper()
	// I1 — cross-replica finalized-hash AGREEMENT. Pairwise over every height both
	// replicas finalized; a disagreement is two conflicting final blocks at one
	// height, the safety failure I1 forbids.
	for i := 0; i < len(replicas); i++ {
		fi := replicas[i].finalizedBlocks()
		for j := i + 1; j < len(replicas); j++ {
			fj := replicas[j].finalizedBlocks()
			for h, hi := range fi {
				hj, ok := fj[h]
				if !ok {
					continue
				}
				if hi != hj {
					t.Fatalf("I1 VIOLATION — replicas %s and %s finalized DIFFERENT blocks at height %d (%x vs %x): two conflicting blocks are irreversibly final at one height → permanent partition. The finality quorum did not intersect.",
						replicas[i].label(), replicas[j].label(), h, hi[:6], hj[:6])
				}
			}
		}
	}
	// I5 — no honest replica slashed (accountable safety; the honest side of the
	// slash predicate).
	for _, r := range replicas {
		if r.honestSlashed() {
			t.Fatalf("I5 VIOLATION — an honest replica (%s) was slashed: accountable safety broke (an honest node must NEVER be slashed; a slash is proof of malice, not of a race). This is the #397 honest-self-slash.",
				r.label())
		}
	}
}

// nodeReplica adapts a real *Node (plus an honest-slash flag the scenario wires
// through OnSlash) to invReplica. finalizedBlocks reads the chain's finalized
// suffix through the SHIPPING accessors (FinalizedHeight + Blocks), never a test
// backdoor.
type nodeReplica struct {
	nd      *Node
	name    string
	slashed *bool
}

func (r *nodeReplica) label() string       { return r.name }
func (r *nodeReplica) honestSlashed() bool { return r.slashed != nil && *r.slashed }
func (r *nodeReplica) finalizedBlocks() map[uint64]ports.Hash {
	out := map[uint64]ports.Hash{}
	fh, ok := r.nd.Chain().FinalizedHeight()
	if !ok {
		return out
	}
	for _, b := range r.nd.Chain().Blocks(0) {
		if b.Height > fh {
			break
		}
		bb := b
		out[b.Height] = bb.Hash()
	}
	return out
}

// stepDriver delivers exactly one held message per Step over a matureWorld net,
// calling the unified oracle after EACH delivery — the spec's "assert after every
// step" contract. Deterministic: FIFO message pick over net.Pending(), fixed
// replica order, no wall-clock, no map-iteration in the ordering. Same world +
// same schedule ⇒ same result every run (spec v1 seed-replay).
type stepDriver struct {
	t        *testing.T
	net      *simnet.Network
	replicas []invReplica
	steps    int
}

// drain delivers held messages ONE AT A TIME (FIFO), asserting the unified oracle
// after each, until the network quiesces. This is the property-harness form of
// drainHeld: where drainHeld only delivers, this checks the invariant continuously
// so a violation is caught at the exact step it appears.
func (d *stepDriver) drain() {
	d.t.Helper()
	const bound = 10000
	for {
		p := d.net.Pending()
		if len(p) == 0 {
			return
		}
		if d.steps++; d.steps > bound {
			d.t.Fatalf("step-driver did not quiesce within %d deliveries (livelock?)", bound)
		}
		d.net.Deliver(p[0].ID) // FIFO — deterministic
		assertInvariants(d.t, d.replicas)
	}
}

// TestModelCheck_Unified_HonestScheduleStaysInvariant drives the REAL mature
// 4+4 world through several honest commit rounds, delivering one message at a
// time and asserting the unified oracle after every single delivery. Across the
// whole schedule every replica that finalizes a height agrees on its hash (I1)
// and no honest node is slashed (I5) — the oracle stays GREEN over live state,
// proving it composes with the real node loop, not just hand-built inputs.
func TestModelCheck_Unified_HonestScheduleStaysInvariant(t *testing.T) {
	nodes, ids, net, refill := matureWorld(t)
	all := make([]ports.NodeID, len(ids))
	for i := range ids {
		all[i] = ids[i].NodeID()
	}

	replicas := make([]invReplica, len(nodes))
	slashFlags := make([]bool, len(nodes))
	for i, nd := range nodes {
		i := i
		nd.OnSlash(func(ports.NodeID, uint64) { slashFlags[i] = true }) // every fixture node is honest here
		replicas[i] = &nodeReplica{nd: nd, name: string(rune('A' + i)), slashed: &slashFlags[i]}
	}

	d := &stepDriver{t: t, net: net, replicas: replicas}
	d.drain() // clear the matureWorld setup traffic, oracle-checked

	// Drive three honest commit rounds through the real gather, delivering one
	// message at a time and asserting the oracle after each.
	committedRounds := 0
	for round := 0; round < 3; round++ {
		refill()
		prev, h := nodes[0].chain.Head()
		desig := nodes[0].designatedProposer(h, 0)
		var proposer *Node
		for _, nd := range nodes {
			if nd.id == desig {
				proposer = nd
				break
			}
		}
		if proposer == nil {
			t.Fatalf("round %d: designated proposer for (h%d,r0) is not a fixture node", round, h)
		}
		// Distinct payload per round — a content root registers once (ErrDupRoot).
		b := &chain.Block{Version: chain.BlockVersionRounds, Height: h, Prev: prev, Entries: []ports.Entry{mkEntry("unified-honest-" + string(rune('a'+round)))}}
		var committed bool
		proposer.proposeBlock(b, all, all, 0, func(err error) { committed = err == nil })
		d.drain() // the whole gather, one delivery at a time, oracle after each
		if !committed {
			t.Fatalf("round %d: honest commit at h%d did not land", round, h)
		}
		committedRounds++
	}

	// Positive control: the schedule actually MADE PROGRESS (finalized heights
	// grew) so the green oracle covered real committed state, not an idle net.
	fh, ok := nodes[0].Chain().FinalizedHeight()
	if !ok || fh == 0 {
		t.Fatalf("no finalized progress (fh=%d ok=%v) — the green oracle covered nothing", fh, ok)
	}
	// Every replica agrees on the finalized head (the I1 property, spot-checked
	// directly at quiescence in addition to the per-step assertions).
	assertInvariants(t, replicas)
	t.Logf("unified oracle GREEN over %d deliveries, %d honest rounds, finalized height %d; all %d replicas agree",
		d.steps, committedRounds, fh, len(replicas))
}

// stubReplica is a hand-built replica for the ablation: it returns an INJECTED
// finalized suffix and honest-slash flag, so the oracle is exercised against
// deliberate divergence that the real intersecting-quorum path cannot produce.
// This is the controlled-input ablation pattern the sibling oracles use for
// predicate checks (modelcheck_i5_accountable_test.go builds culprit slot-sets
// directly; modelcheck_i2_exhaustive_test.go builds a wiped store) — the
// injected defect is fed to the SAME oracle the real scenario uses.
type stubReplica struct {
	name    string
	fin     map[uint64]ports.Hash
	slashed bool
}

func (s *stubReplica) label() string                          { return s.name }
func (s *stubReplica) honestSlashed() bool                    { return s.slashed }
func (s *stubReplica) finalizedBlocks() map[uint64]ports.Hash { return s.fin }

// TestModelCheck_Unified_AblationCatchesInjectedViolations is the HARD-BAR
// proof: the unified oracle goes RED on an injected I1 disagreement and on an
// injected I5 honest-slash, and GREEN once the divergence is removed. Without a
// demonstrated red these checks would be decoration.
//
// The RED is produced by feeding the oracle deliberately divergent replica STATE
// — NOT by editing any consensus rule. A real intersecting finality quorum makes
// two replicas finalizing different hashes at one height UNREACHABLE (that is
// exactly what I1 guarantees and what the honest scenario above confirms), so the
// only way to exercise the CATCH is to inject the state the invariant forbids and
// confirm the monitor fires.
func TestModelCheck_Unified_AblationCatchesInjectedViolations(t *testing.T) {
	hashX := ports.HashBytes([]byte("block-X-at-h5"))
	hashY := ports.HashBytes([]byte("block-Y-at-h5")) // conflicting fork at the same height

	// --- I1 ablation: two replicas finalize DIFFERENT hashes at height 5. ---
	i1Diverged := []invReplica{
		&stubReplica{name: "R0", fin: map[uint64]ports.Hash{5: hashX}},
		&stubReplica{name: "R1", fin: map[uint64]ports.Hash{5: hashY}},
	}
	if !oracleFires(t, i1Diverged) {
		t.Fatal("I1 ABLATION FAILED: the unified oracle did NOT fire on two replicas finalizing conflicting blocks at one height — the I1 check is decoration")
	}

	// GREEN control: the same two replicas AGREE at height 5 → no fire.
	i1Agree := []invReplica{
		&stubReplica{name: "R0", fin: map[uint64]ports.Hash{5: hashX}},
		&stubReplica{name: "R1", fin: map[uint64]ports.Hash{5: hashX}},
	}
	if oracleFires(t, i1Agree) {
		t.Fatal("I1 GREEN control FAILED: the oracle fired though both replicas agree at height 5 — it flags honest agreement")
	}

	// Disjoint finalized heights must NOT fire (a replica simply behind is not a
	// disagreement — the check compares only COMMON finalized heights).
	i1Disjoint := []invReplica{
		&stubReplica{name: "R0", fin: map[uint64]ports.Hash{5: hashX}},
		&stubReplica{name: "R1", fin: map[uint64]ports.Hash{4: hashY}},
	}
	if oracleFires(t, i1Disjoint) {
		t.Fatal("I1 control FAILED: the oracle fired on disjoint finalized heights — a lagging replica is not an I1 violation")
	}

	// --- I5 ablation: an honest replica is slashed. ---
	i5Slashed := []invReplica{
		&stubReplica{name: "R0", fin: map[uint64]ports.Hash{}},
		&stubReplica{name: "R1", fin: map[uint64]ports.Hash{}, slashed: true},
	}
	if !oracleFires(t, i5Slashed) {
		t.Fatal("I5 ABLATION FAILED: the unified oracle did NOT fire when an honest replica was slashed — the I5 check is decoration")
	}

	// GREEN control: no honest slash → no fire.
	i5Clean := []invReplica{
		&stubReplica{name: "R0", fin: map[uint64]ports.Hash{}},
		&stubReplica{name: "R1", fin: map[uint64]ports.Hash{}},
	}
	if oracleFires(t, i5Clean) {
		t.Fatal("I5 GREEN control FAILED: the oracle fired with no honest slash present")
	}
}

// oracleFires runs assertInvariants in a sub-test and reports whether it FAILED —
// the mechanism that lets the ablation observe the oracle's red without failing
// the parent test. A t.Fatalf in the sub-test marks it failed; oracleFires
// returns that as `fired`.
func oracleFires(t *testing.T, replicas []invReplica) (fired bool) {
	t.Helper()
	sub := &testing.T{}
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer func() { recover() }() // t.FailNow (via Fatalf) calls runtime.Goexit
		assertInvariants(sub, replicas)
	}()
	<-done
	return sub.Failed()
}
