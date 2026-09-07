package node

import (
	"testing"

	"github.com/nerolabs/silt/ports"
)

// TestModelCheck_H43_HeterogeneousArmingMustCommitWithinFPlus1Rounds is
// G-H43-1, decisive, per
// silt-reviews/research/research-outcome/CONSENSUS-LIVENESS-h43-round-ladder-desync-441-380-RESEARCH-CERTIFICATION-2026-09-07.md
// §4.2: "matureWorld12 variant with heterogeneous arming: exactly 3 of 12
// seats hold pending work, one heavy seat killed, staggered sweep phases,
// timed delivery on the sim clock."
//
// THIS TEST IS ALSO G-H43-15 (the delta certification's own instruction:
// "pin the takeover re-key `(height + rs.Round) % N` — this is the same pin
// as the reg-lane G-H43-1 re-shape; one gate is enough if it names both").
// The "── The pin (D3) ──" block below reads chainrole.go's
// `maybeProposeBondDrain`'s `d := int((height + rs.Round) % uint64(len(props)))`
// live, against the ACTUAL committing proposer — that IS the re-key pin;
// no second gate duplicates it.
//
// THE ROOT (M1, certified): core/node/rounds.go:306-310 arms the round clock
// on LOCAL mempool content — `maybeAdvanceRound` quiesces
// (`rs.Sweeps = 0`) whenever a node's OWN pendingBondRegs/pendingEntries/
// bondDrainInFlight are empty, so a seat that never holds work never runs
// the pacemaker at all. Every EXISTING oracle in this family
// (`TestModelCheck_451_*`, `TestModelCheck_560_*`) calls the shared
// `refill()` from `matureWorld12`, which sets `pendingBondRegs` on EVERY
// node (`modelcheck_451_locked_stall_test.go:143-146`) — so none of them can
// ever exercise this branch. This test is the first in the family that
// does not.
//
// THIS IS THE REG LANE, PLAIN STATEMENT (delta certification
// CONSENSUS-LIVENESS-h43-ABC-ASBUILT-workless-designee-RESEARCH-CERTIFICATION-2026-09-07.md
// §3.1/§7): a bond registration is NEVER forwarded to a round's designee —
// the receiver decisively refuses any reg it does not own
// ("bond-reg submit REFUSED (relay)", chainrole.go; R-H43-FORWARD-REG-RELAY-REFUSED,
// gated by G-H43-10(b)) — so THIS lane gets no D1-entry rescue and its
// certified bound is the D4 BACKSTOP `(N+2)·ChainSyncInterval + G`, never
// the tighter `f+1` ladder bound (that bound is the ENTRY lane's,
// G-H43-9). What THIS gate pins, once M1 no longer masks it, is D3: the
// #338 rank-staggered takeover's walk is re-keyed from the ROUND's own
// designee `props[(height+round) % N]` (chainrole.go's
// `maybeProposeBondDrain`), not from the height's `props[height % N]`
// (the pre-D3 walk, still what `462478d` computes).
//
// THE SCHEDULE: nodes[0..3] are the four heavy (64 MiB) anchors — the
// field's val-a..d. ARMED = nodes[2, 4, 9] (one anchor, one maturer, one
// sybil — deliberately NOT the contiguous val-a/b/c trio this gate used
// before the re-shape); nodes[3] is KILLED at the contested height's start
// (the field's val-d, stopped) and RESTARTED at downDuration=540s (~9 min,
// after the backstop has already decided the RED/GREEN run — the "down
// then back mid-ladder" shape, layered onto the cert's "one heavy seat
// killed"). The rest of nodes[4..11] are left QUIESCENT — pendingBondRegs
// forced empty and never touched (setup-only strip — see
// stripQuiescentRegsOnly) — the field's silent rotation seats.
//
// THE SEARCH FOR A WINNER-NAME DIVERGENCE, AND WHY THIS FIXTURE CANNOT
// SHOW ONE — reported with the arithmetic, per instruction, rather than
// shipping a seat assignment that only coincidentally passes. props :=
// EligibleProposers() is fixed for this fixture (hash-sorted, deterministic
// identity seeds). The height's own designee (round 0) sits at
// props-position 9 (node 3 — killed). This run settles at round 2, whose
// OWN designee sits at position 11 (node 7, confirmed workless below).
// rank-distance is `dist(p, d) = (p - d + N) mod N`, walked FORWARD. Fix
// d_new = d_old + r (mod N): for ANY candidate p with dist(p, d_old) >= r,
// dist(p, d_new) = dist(p, d_old) - r — a CONSTANT SHIFT, which preserves
// the RELATIVE ORDER of every such candidate. The only candidates that
// could REORDER are those with dist(p, d_old) < r — i.e., the positions the
// round ladder sweeps THROUGH on its way from d_old to d_new. At r = 2
// those are positions 9 and 10 (node 3 — killed — and node 6). But arming
// EITHER of those two seats does not produce a comparison at round 2 at
// all: node 6 sits exactly at position 10, round 1's own designee, so
// arming it makes round 1 a DIRECT proposal (verified this session — armed
// sets containing node 6 commit at round 1, by node 6, before round 2 is
// ever reached) — a violation of "the round's designee itself confirmed
// workless", not a reachable comparison. So for this fixture, at the round
// this schedule actually settles at, NO seat assignment can both (a) keep
// round 2's designee workless and (b) make the height-keyed and
// round-keyed walks name different winners — the shift is order-preserving
// over exactly the candidates that remain eligible to compare. Verified
// directly: armed=[2, 4, 9] names the SAME winner (node 9) under both the
// height-keyed formula (dist 4 from position 9) and the round-keyed one
// (dist 4 from position 11) — see the logged arithmetic below.
//
// WHAT THIS GATE STILL PROVES, given that: (1) the pin reads the ACTUAL
// committing proposer against the CORRECT (round-keyed) rank formula, live
// — the formula this fix's chainrole.go now runs, cited by file:line, not
// merely a value that happens to coincide; (2) RED is carried by
// CONVERGENCE, not by winner identity — armed=[2, 4, 9] does not commit
// within the D4 backstop at `462478d` at all (verified in a scratch
// worktree, this session), so the pin cannot even be reached pre-fix. A
// future seat/height choice that DOES separate the two walks (this
// fixture's height is fixed by matureWorld12's own build sequence; a
// different h43 fixture at a height further from its own round-0
// designee's position, giving r >= 3 room without an intervening armed
// direct-hit, could) is a genuine follow-up, not attempted here.
func TestModelCheck_H43_HeterogeneousArmingMustCommitWithinFPlus1Rounds(t *testing.T) {
	nodes, ids, net, sched, refill := matureWorld12(t)
	all := make([]ports.NodeID, len(ids))
	for i := range ids {
		all[i] = ids[i].NodeID()
	}
	for i, nd := range nodes {
		if !nd.chain.EverMature() {
			t.Fatalf("premise: node %d not latched", i)
		}
	}
	var honestSlashed bool
	for _, nd := range nodes {
		nd.OnSlash(func(ports.NodeID, uint64) { honestSlashed = true })
	}

	_, contested := nodes[0].chain.Head()

	// ARM nodes[2, 4, 9] (one anchor, one maturer, one sybil — the docstring's
	// arithmetic explains why THIS fixture cannot separate the height-keyed
	// and round-keyed walks by winner NAME; convergence carries RED instead);
	// KILL nodes[3] (val-d); force the rest of nodes[4..11] QUIESCENT — the
	// heterogeneous-arming premise G-H43-6 checks the harness CAN express.
	armed := []int{2, 4, 9}
	killedIdx := 3

	// props is the SAME EligibleProposers() ordering both the takeover
	// (chainrole.go's maybeProposeBondDrain) and this test's pin read — a
	// pure function of committed state, fixed for the whole run.
	props := nodes[0].chain.EligibleProposers()
	if len(props) != 12 {
		t.Fatalf("premise: want a 12-member epoch, got %d eligible proposers", len(props))
	}
	posOf := map[ports.NodeID]int{}
	for i, p := range props {
		posOf[p] = i
	}
	// rankFrom mirrors chainrole.go's takeover rank formula exactly:
	// dist = (self - d + N) % N, walking FORWARD from d. Returns the armed
	// candidate with the smallest such distance from position `from`.
	rankFrom := func(from int) (winnerIdx, winnerDist int) {
		winnerDist = -1
		for _, c := range armed {
			d := (posOf[nodes[c].id] - from + len(props)) % len(props)
			if winnerDist < 0 || d < winnerDist {
				winnerDist, winnerIdx = d, c
			}
		}
		return
	}
	dHeight := int(contested % uint64(len(props)))
	heightKeyedWinner, heightKeyedDist := rankFrom(dHeight)
	t.Logf("G-H43-1 premise: props fixed (N=%d); height %d's OWN designee sits at position %d; the OLD "+
		"height-keyed takeover walk (pre-D3) would favor node %d (dist %d) among the armed set %v",
		len(props), contested, dHeight, heightKeyedWinner, heightKeyedDist, armed)
	// stripQuiescent (h43-ABC-asbuilt certification §1.3, R-H43-FIXTURE-STRIPS-REG-LANE):
	// SETUP-ONLY on the entries lane. Run once, to establish the
	// heterogeneous-arming premise (nodes 4-11 start with none of the
	// height's pending work — the field's 8 silent rotation seats), on BOTH
	// lanes. It must not zero pendingEntries again after setup: the fix's
	// part (D) legitimately FORWARDS pending entries to a round's designee
	// (forwardPendingWorkToDesignee, rounds.go), and a periodic strip of
	// every quiescent seat's ENTRY queue would delete that forwarded work
	// before the designee ever gets to fold it — an artifact of this
	// fixture, not a property of the mechanism under test. The reg lane is
	// different: the certification's own ruling (delta cert §3.1) is that a
	// forwarded THIRD-PARTY reg is, and must stay, REFUSED at the receiver
	// ("bond-reg submit REFUSED (relay)", chainrole.go) — so a quiescent
	// seat's pendingBondRegs can never legitimately grow from a forward
	// either way, and this oracle keeps re-stripping it every top-up purely
	// to cancel refill()'s own uniform reseed (below), never because a real
	// forward could have landed one.
	stripQuiescentRegsOnly := func() {
		armedSet := map[int]bool{}
		for _, i := range armed {
			armedSet[i] = true
		}
		for i, nd := range nodes {
			if i == killedIdx || armedSet[i] {
				continue
			}
			nd.pendingBondRegs = nil
		}
	}
	arm := func() {
		refill() // seeds pendingBondRegs on EVERY node (the shared, uniform helper)
		stripQuiescentRegsOnly()
		armedSet := map[int]bool{}
		for _, i := range armed {
			armedSet[i] = true
		}
		for i, nd := range nodes {
			if i == killedIdx || armedSet[i] {
				continue
			}
			nd.pendingEntries = nil // setup only — the caller below never repeats this
		}
	}
	arm()
	armedIdxSet := map[int]bool{}
	for _, i := range armed {
		armedIdxSet[i] = true
	}
	for i, nd := range nodes {
		isArmed := armedIdxSet[i]
		isKilled := i == killedIdx
		hasWork := len(nd.pendingBondRegs) > 0 || len(nd.pendingEntries) > 0
		if isArmed && !hasWork {
			t.Fatalf("premise: armed seat %d has no pending work after arm()", i)
		}
		if !isArmed && !isKilled && hasWork {
			t.Fatalf("premise: quiescent seat %d has pending work after arm() — heterogeneous arming not established", i)
		}
	}

	net.Kill(nodes[killedIdx].id)
	net.DisableHeldDelivery()

	// Staggered sweep phases across the interval (the field's independent
	// 30s timers, re-randomized by restart — the #560 convention).
	interval := DefaultConfig().ChainSyncInterval
	live := make([]*Node, 0, 11)
	for i, nd := range nodes {
		if i == killedIdx {
			continue
		}
		nd := nd
		idx := len(live)
		live = append(live, nd)
		phase := ports.Duration(idx) * (interval / 11)
		sched.AfterFunc(phase, func() { nd.StartChainSync(nd.chainSyncSeed, nil) })
	}

	// The field's val-d: down then back mid-ladder (coordinator's requested
	// shape, layered onto the cert's "one heavy seat killed"). 430s < 540s,
	// so the restated D4 backstop below is decided before this fires in the
	// RED run; it exists in the schedule so a future GREEN run (and any
	// resolution of R-H43-RECOVERY-UNATTRIBUTED, §3.2/§9 of the parent
	// certification) also covers the return dynamic, not just the
	// permanently-down case.
	const downDuration = 9 * 60 * ports.Second
	sched.AfterFunc(downDuration, func() {
		net.Restart(nodes[killedIdx].id)
		nodes[killedIdx].StartChainSync(nodes[killedIdx].chainSyncSeed, nil)
	})

	committed := func() bool {
		for _, nd := range nodes {
			if _, h := nd.chain.Head(); h > contested {
				return true
			}
		}
		return false
	}

	// THE RESTATED BOUND (D4, delta certification §2.2/§3.4): the reg lane
	// gets no forward-based rescue (D1-entry does not apply to it, by design
	// — G-H43-10(b)), so its worst case is the #338 takeover walk, never the
	// f+1 ladder escape (that bound belongs to the entry lane, G-H43-9):
	// `(N + 2) · ChainSyncInterval + G`.
	const gatherG = 10 * ports.Second // the #555-measured intrinsic gather latency at 12-seat WAN
	const nGoverning = 12
	bound := ports.Duration(nGoverning+2)*interval + gatherG
	deadline := sched.Now().Add(bound)

	steps := 0
	for sched.Now() < deadline && !committed() {
		if !sched.Step() {
			break // scheduler drained before the bound — report state below, do not loop forever
		}
		steps++
		if steps%64 == 0 {
			// Top up the 3 armed seats' pendingBondRegs (refill() reseeds ALL 12
			// uniformly, so immediately re-strip the reg lane on the quiescent 8
			// — that lane can never legitimately hold forwarded work, certified
			// above). Deliberately do NOT touch pendingEntries here: a legitimate
			// (D) forward may have landed real pending entries on a quiescent
			// seat since the last top-up, and wiping them would delete work the
			// fix forwards, not model a genuine silent seat (certification
			// §1.3/§7 item 7, R-H43-FIXTURE-STRIPS-REG-LANE).
			refill()
			stripQuiescentRegsOnly()
		}
	}

	if honestSlashed {
		t.Fatal("I5 VIOLATION: an honest validator was slashed under the heterogeneous-arming schedule")
	}
	if !committed() {
		rounds := map[int]uint64{}
		for i, nd := range nodes {
			rounds[i] = nd.roundsFor().Round
		}
		t.Fatalf("G-H43-1 REPRODUCED: with armed=%v, 8 quiescent, one heavy seat killed (node 3, restart "+
			"scheduled at %v uncalled by the backstop), height %d did not commit within the restated D4 "+
			"reg-lane backstop (N+2)*Delta+G = %v of the kill (deadline %v; per-seat rounds reached: %v). The "+
			"reg lane has no forward-based rescue by design (R-H43-FORWARD-REG-RELAY-REFUSED, deleted at "+
			"8368196), so its only rescue is the #338 takeover — which does not converge within the backstop "+
			"at all on this seat assignment before this fix (D3's re-key, verified against this fix: converges "+
			"at round 2, proposer node 9, the nearest work-holder from round 2's own workless designee at "+
			"props-position 11; the height-keyed formula (props-position %d) predicts the SAME winner (node %d, "+
			"dist %d) in THIS specific fixture — this fixture's own arithmetic, docstring above, proves no seat "+
			"assignment here can separate the two walks by winner NAME while keeping the round's designee "+
			"workless, so this RED is carried by convergence, not by a different name). Consensus-adjacent, "+
			"research-gated (build-immutable #6); fix direction (A) (M1) + D3 (the re-key) in the certification.",
			armed, downDuration, contested, bound, deadline, rounds, dHeight, heightKeyedWinner, heightKeyedDist)
	}
	var commitRound uint64
	var proposerID ports.NodeID
	for _, nd := range nodes {
		if b := nd.Chain().Blocks(contested); len(b) > 0 {
			commitRound = b[0].CommitRound
			proposerID = b[0].ProposerID()
			break
		}
	}
	dRound := int((contested + commitRound) % uint64(len(props)))
	roundDesigneeID := props[dRound]

	// Premise: the commit round's OWN designee is genuinely workless — never
	// armed, never the killed seat — the M4 precondition this pin depends on
	// (a designee that is itself armed just proposes directly; a designee
	// that is down is a different, already-covered class).
	armedIDs := map[ports.NodeID]bool{}
	for _, i := range armed {
		armedIDs[nodes[i].id] = true
	}
	if armedIDs[roundDesigneeID] {
		t.Fatalf("premise: round %d's own designee %s is ARMED — this commit is a direct proposal, not a "+
			"takeover; the seat assignment must keep the commit round's designee workless for this pin to mean "+
			"anything", commitRound, roundDesigneeID)
	}
	if roundDesigneeID == nodes[killedIdx].id {
		t.Fatalf("premise: round %d's own designee %s is the KILLED seat — that tests a DOWN designee, a "+
			"different, already-covered class, not a workless-but-live one", commitRound, roundDesigneeID)
	}
	// G-H43-6 teeth: this IS the runtime-confirmed, load-bearing observation
	// of a live+eligible+workless designee the fix (a vacuous G-H43-6
	// otherwise) needs evidence of.
	recordH43QuiescentDesigneeObserved("G-H43-1", roundDesigneeID)

	roundKeyedWinner, roundKeyedDist := rankFrom(dRound)

	// ── The pin (D3): the committing proposer is the nearest work-holding
	// eligible proposer in rank order FROM THE ROUND'S OWN DESIGNEE, never
	// from the height's — the re-key this gate exists to prove ─────────────
	if proposerID != nodes[roundKeyedWinner].id {
		t.Fatalf("G-H43-1 REPRODUCED (D3 not applied, or applied incorrectly): height %d committed at round %d "+
			"by %s, but the nearest work-holder in rank order FROM the round's own (confirmed workless) "+
			"designee %s is node %d = %s (dist %d) — the takeover walk is not measured from the round's "+
			"designee (chainrole.go's maybeProposeBondDrain). Consensus-adjacent, research-gated "+
			"(build-immutable #6); closer: D3 (the re-key).",
			contested, commitRound, proposerID, roundDesigneeID, roundKeyedWinner, nodes[roundKeyedWinner].id, roundKeyedDist)
	}
	if heightKeyedWinner == roundKeyedWinner {
		t.Logf("G-H43-1 note: the height-keyed and round-keyed walks name the SAME winner (node %d) for this "+
			"fixture, exactly as the docstring's arithmetic proves they must at round 2 — this gate's RED is "+
			"carried by CONVERGENCE at 462478d (verified in a scratch worktree: does not commit within the "+
			"backstop), not by a winner-name difference; the pin below still holds because it is measured from "+
			"the round's designee, never the height's, which is what this fix actually changed.", roundKeyedWinner)
	} else {
		t.Logf("G-H43-1: the walks diverge exactly as designed — height-keyed (from position %d) favors node "+
			"%d (dist %d); round-keyed (from round %d's own designee %s, position %d) favors node %d (dist "+
			"%d), and node %d is who actually committed.",
			dHeight, heightKeyedWinner, heightKeyedDist, commitRound, roundDesigneeID, dRound, roundKeyedWinner, roundKeyedDist, roundKeyedWinner)
	}
	t.Logf("G-H43-1: h%d committed at round %d, %v (virtual), by node %d (%s) — the nearest work-holder from "+
		"round %d's own workless designee %s — within the restated D4 backstop %v of the kill. M1 fixed; D3's "+
		"re-key pinned.",
		contested, commitRound, sched.Now(), roundKeyedWinner, proposerID, commitRound, roundDesigneeID, bound)
}

// TestModelCheck_H43_AtLeastOneRoundLivenessOracleArmsNonUniformly is
// G-H43-6 (method), the fixture-blindness pin the certification calls the
// SIXTH recurrence of one blind spot (docs/design/consensus-model-check.md
// amendment log records five: #357, the I5 benign-call, #441 entry-liveness,
// #451 round-advance skew, #456 dead-peer cost).
//
// THE CLAIM UNDER TEST: "at least one round-liveness oracle in this family
// runs with a NON-UNIFORM arming distribution" — i.e. the harness CAN
// express a quiescent seat (a node with permanently empty
// pendingBondRegs/pendingEntries while others hold work), and at least one
// committed oracle exercises that state, rather than every oracle routing
// through the shared `refill()` (`modelcheck_451_locked_stall_test.go:143-146`)
// which arms EVERY node uniformly.
//
// TWO HALVES, both runtime-observed (not source-text inspection — this is
// not a SOURCE GATE per build-process.md rule 8, since it reads live Node
// state, not .go text):
//  1. CAPABILITY: matureWorld12 itself does not force uniform arming — a
//     caller can leave a node's queues empty. Proven by direct field
//     inspection immediately after fixture construction.
//  2. TEETH: the shared `refill()` helper — the one every pre-existing
//     oracle in the family calls — is confirmed, by running it and reading
//     the resulting state, to arm ALL nodes uniformly. This is the exact
//     trap the certification names: a test that reached for `refill()`
//     alone could never have produced heterogeneous arming, so a REGRESSION
//     that silently reverted G-H43-1's fixture to plain `refill()` and
//     nothing else would leave the family with ZERO non-uniform oracles —
//     which is what this pin exists to catch. It does not (and cannot, at
//     the runtime level) certify that G-H43-1 itself does not regress; that
//     is G-H43-1's own premise check
//     (TestModelCheck_H43_HeterogeneousArmingMustCommitWithinFPlus1Rounds's
//     "heterogeneous arming not established" fatal, immediately above it).
func TestModelCheck_H43_AtLeastOneRoundLivenessOracleArmsNonUniformly(t *testing.T) {
	nodes, _, _, _, refill := matureWorld12(t)

	// HALF 1 — capability, with a finding: matureWorld12 ITSELF calls the
	// shared refill() as its own last setup step
	// (modelcheck_451_locked_stall_test.go:147) — so every node ALREADY
	// holds uniform pending work the instant the fixture returns, before any
	// oracle gets a chance to choose otherwise. This is a THIRD layer of the
	// same blind spot the certification names at the caller level (every
	// oracle's OWN refill() loop) — it is baked into the constructor too.
	// Confirm that fact first (evidence, not an assumption), then confirm
	// the harness still PERMITS reaching a non-uniform state by clearing.
	uniformAtReturn := true
	for _, nd := range nodes {
		if len(nd.pendingBondRegs) == 0 && len(nd.pendingEntries) == 0 {
			uniformAtReturn = false
		}
	}
	if !uniformAtReturn {
		t.Fatal("G-H43-6 finding check: matureWorld12 no longer arms every node uniformly at construction — " +
			"re-verify the THIRD-layer uniform-arming claim (constructor-level) before relying on it")
	}
	t.Log("G-H43-6 finding: matureWorld12's OWN constructor arms all 12 nodes uniformly before returning " +
		"(its internal refill() call, modelcheck_451_locked_stall_test.go:147) — a third layer of the same " +
		"blind spot, beneath the caller-level refill() loop the certification names.")

	// CAPABILITY: clear every queue, then arm a strict subset directly — no
	// fixture machinery prevents a non-uniform state; the DEFAULT is uniform,
	// but the STATE SPACE is not restricted to it.
	for _, nd := range nodes {
		nd.pendingBondRegs = nil
		nd.pendingEntries = nil
	}
	nodes[0].pendingBondRegs = []pendingBondReg{{}}
	for i, nd := range nodes {
		hasWork := len(nd.pendingBondRegs) != 0 || len(nd.pendingEntries) != 0
		if i == 0 && !hasWork {
			t.Fatal("G-H43-6 capability check: setting pendingBondRegs on node 0 directly did not take")
		}
		if i != 0 && hasWork {
			t.Fatalf("G-H43-6 capability check: arming node 0 alone leaked pending work onto node %d — "+
				"the fixture does not permit non-uniform arming", i)
		}
	}
	nodes[0].pendingBondRegs = nil // reset before the teeth half

	// HALF 2 — teeth: the shared `refill()` every PRE-EXISTING oracle in this
	// family (#451, #560) calls arms EVERY node — confirmed by reading the
	// live state after calling it, not by trusting the docstring.
	refill()
	uniform := true
	for i, nd := range nodes {
		if len(nd.pendingBondRegs) == 0 && len(nd.pendingEntries) == 0 {
			uniform = false
			t.Logf("teeth check: node %d was NOT armed by refill() — the shared helper is no longer uniform; "+
				"re-verify which pre-existing oracle this changes before treating G-H43-6 as satisfied by them", i)
		}
	}
	if !uniform {
		t.Fatal("G-H43-6 teeth check FAILED: the shared refill() helper no longer arms every node uniformly — " +
			"the premise this pin's SIXTH-recurrence claim rests on (every pre-existing oracle in the family " +
			"arms uniformly via refill()) no longer holds; re-attribute before relying on this pin")
	}
	t.Logf("G-H43-6: capability confirmed (matureWorld12 permits a strict-subset arm), teeth confirmed "+
		"(the shared refill() helper arms all %d nodes uniformly — the exact trap the certification names). "+
		"TestModelCheck_H43_HeterogeneousArmingMustCommitWithinFPlus1Rounds is, as of this commit, the first "+
		"oracle in the family that does not rely on refill() alone.", len(nodes))

	// HALF 3 — LOAD-BEARING EXERCISE (the composed-diff re-certification's
	// fix for G-H43-6's own VACUOUS ruling): halves 1 and 2 above prove only
	// that the HARNESS permits non-uniform arming and that refill() is
	// uniform — neither can catch G-H43-1's (or G-H43-9's) fixture silently
	// reverting to plain refill(), because neither reads whether any REAL
	// oracle actually exercised the capability. h43QuiescentDesigneeObserved
	// is written, at runtime, by any test that confirms a LIVE, ELIGIBLE
	// designee held none of the height's pending work — the M4 shape this
	// whole arc targets. Package tests within one file run in declaration
	// order, and `go test` compiles a package's files in filename order
	// ("modelcheck_h43_arming_test.go" precedes "modelcheck_h43_gates_test.go"),
	// so G-H43-1 (declared earlier in THIS file) has already run and
	// recorded by the time this check runs.
	h43QuiescentDesigneeMu.Lock()
	observed := append([]string(nil), h43QuiescentDesigneeObserved...)
	h43QuiescentDesigneeMu.Unlock()
	if len(observed) == 0 {
		t.Fatal("G-H43-6 REPRODUCED (load-bearing-exercise check): h43QuiescentDesigneeObserved is EMPTY — no " +
			"test in this package run has confirmed, at runtime, a live+eligible+workless designee. Either " +
			"this test ran in isolation (a -run filter that excludes the recorders — G-H43-1/G-H43-9 must run " +
			"first; this failure is then expected and uninformative, not a regression) or G-H43-1's fixture has " +
			"silently reverted to a uniform refill() (the exact regression capability+teeth above cannot catch).")
	}
	t.Logf("G-H43-6 (load-bearing-exercise): %d real observation(s) recorded this package run: %v", len(observed), observed)
}
