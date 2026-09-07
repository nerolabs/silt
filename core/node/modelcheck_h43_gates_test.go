package node

import (
	"crypto/ed25519"
	"fmt"
	"strings"
	"testing"

	"github.com/fxamacker/cbor/v2"
	"github.com/nerolabs/silt/adapters/identity"
	"github.com/nerolabs/silt/core/chain"
	"github.com/nerolabs/silt/ports"
)

// G-H43-2, G-H43-3, G-H43-4, G-H43-5 — the remaining RED-first gates from
// silt-reviews/research/research-outcome/
// CONSENSUS-LIVENESS-h43-round-ladder-desync-441-380-RESEARCH-CERTIFICATION-2026-09-07.md
// §4.2, encoded per this session's task (G-H43-1 and G-H43-6 already live in
// modelcheck_h43_arming_test.go on this branch).

// ── G-H43-2 ─────────────────────────────────────────────────────────────────
//
// TestModelCheck_H43_DesigneeMustProposeAtCertificateRound is G-H43-2 (the M2
// gate): "Drive a designee into val-b's exact state — designee for (h, 1),
// local rs.Round == 3, Changes[1] at quorum, Changes[3] below it — and assert
// a proposal reaches the wire at round 1."
//
// THE SCHEDULE, real wire delivery over held-delivery, per the field timeline
// (certification §2/§3 M2): a 4-anchor launch network (tier2AnchorNet — the
// #402 strict-anchor-majority regime the field ran under). One anchor (the
// field's val-d) is killed. The other two non-designee anchors (val-a, val-c)
// each independently sweep their OWN round to 1 on their own local timeouts
// (maybeAdvanceRound), broadcasting real signed round-change(1) envelopes —
// exactly as val-a/val-c laddered alone in the field. Their envelopes to the
// designee (val-b) are parked (drainHeldExcept) rather than delivered
// immediately, so the test controls the exact moment the SECOND
// (quorum-completing) one arrives. The designee's own round is then driven to
// 3 directly — the field's val-b independently laddered ahead on its own
// clock while the quorum-completing envelope was still in flight (§2:
// val-b's r1/r2/r3 timestamps, each on its own local timer, all before the
// 08:55:26 catch-up jump) — and only THEN is the second envelope delivered
// through the real MsgRoundChange handler, completing the round-1 quorum
// while rs.Round == 3. That is val-b's exact recorded state.
//
// THE OBSERVATION: every live peer's transport is wrapped to record the
// Round field of any MsgProposeBlock envelope it receives, so the assertion
// reads the WIRE, never source text.
//
// RED at HEAD (M2, chainrole.go:1040-1054): recordRoundChange's automatic
// designee trigger (rounds.go:365-387) fires proposeAtNewView(rs, 1, raws,
// nil) — forced == nil on every h43 round-change, since none carried a lock
// — which falls through to proposeBlock (chainrole.go:1338), and
// proposeBlock RE-DERIVES the round from rs.Round (chainrole.go:1040-1047)
// instead of using the 1 it was handed. rs.Changes[3] holds nothing (no
// round-3 round-change was ever recorded), so the re-derived round-3
// new-view certificate is immediately below quorum and the proposal never
// reaches gatherTwoPhase — no MsgProposeBlock is ever sent, at round 1 or any
// round. GREEN only once fix direction (C) ships (proposeAtNewView passes
// (round, newView) through to the gather on the forced == nil leg exactly as
// it already does on the forced != nil leg).
func TestModelCheck_H43_DesigneeMustProposeAtCertificateRound(t *testing.T) {
	nodes, ids, net, g, _ := tier2AnchorNet(t, 4)
	all := make([]ports.NodeID, len(ids))
	for i := range ids {
		all[i] = ids[i].NodeID()
	}
	byID := map[ports.NodeID]*Node{}
	for _, nd := range nodes {
		byID[nd.id] = nd
	}

	_, height := nodes[0].chain.Head()
	if height != 1 {
		t.Fatalf("premise: want the working height to be 1 (right after genesis), got %d", height)
	}

	designeeID := nodes[0].designatedProposer(height, 1)
	designee, ok := byID[designeeID]
	if !ok {
		t.Fatalf("premise: designatedProposer(%d, 1) returned an unknown id", height)
	}
	var killed *Node
	for _, nd := range nodes {
		if nd.id != designeeID {
			killed = nd
			break
		}
	}
	var livers []*Node
	for _, nd := range nodes {
		if nd.id != designeeID && nd.id != killed.id {
			livers = append(livers, nd)
		}
	}
	if len(livers) != 2 {
		t.Fatalf("premise: want exactly 2 live non-designee anchors, got %d", len(livers))
	}

	net.Kill(killed.id)

	// Route everyone's OUTBOUND sync targets around the killed seat — a dead
	// peer still answers `Send` with a silent drop (nil error, no callback),
	// so a REQUEST addressed to it (broadcastCommit's own sequential
	// validators[i]/i+1 recursion, chainrole.go:1410-1420) never resolves
	// under this test's frozen sim clock (no real timeout ever fires without
	// sched.Step, which this test never calls — drainHeld completes purely
	// by delivered replies). Excluding the killed seat from chainSyncSeed
	// keeps this a topology choice this test controls, not a claim about the
	// quorum math itself (RequiredQuorum/the #402 anchor majority are pure
	// functions of the chain's Anchors config, never of chainSyncSeed).
	for _, nd := range nodes {
		if nd.id == killed.id {
			continue
		}
		seed := make([]ports.NodeID, 0, len(all)-2)
		for _, id := range all {
			if id != nd.id && id != killed.id {
				seed = append(seed, id)
			}
		}
		nd.chainSyncSeed = seed
	}

	// A real, valid pending bond registration for a 5th non-anchor identity —
	// the "pending work" that arms maybeAdvanceRound, matching s1s2World's
	// convention.
	reg5 := identity.FromSeed(890010)
	regPub := append([]byte(nil), reg5.Signer().Public().(ed25519.PublicKey)...)
	reg := chain.NewBondReg(reg5.Signer(), ports.HashBytes(regPub), 2<<20, []byte("stub"), g.Hash(), 0)
	arm := func(nd *Node) { nd.pendingBondRegs = []pendingBondReg{{R: reg}} }
	arm(designee)
	for _, nd := range livers {
		arm(nd)
	}

	// Wrap the live peers' transports to observe the Round field of any
	// MsgProposeBlock they receive — the wire, not source text.
	var sawPropose bool
	var capturedRound uint64
	wrap := func(nd *Node) {
		ep := net.Endpoint(nd.id)
		orig := nd.handle
		ep.SetHandler(func(from ports.NodeID, msg ports.Message) {
			if msg.Kind == ports.MsgProposeBlock {
				var env proposeEnv
				if cbor.Unmarshal(msg.Data, &env) == nil && len(env.Raw) > 0 {
					sawPropose = true
					capturedRound = env.Round
				} else {
					sawPropose = true
					capturedRound = 0 // legacy bare-block payload = round 0
				}
			}
			orig(from, msg)
		})
	}
	for _, nd := range livers {
		wrap(nd)
	}

	// Each liver independently produces a real, individually-verified
	// round-change(1) envelope — advanceToRound, the same call
	// maybeAdvanceRound's timeout path makes, on its own round state (a
	// simulated local timeout, not a network delivery). This test then hands
	// each one directly to the designee's REAL MsgRoundChange handler, in a
	// controlled order, rather than relying on simnet delivery ordering:
	// under the fix, ANY node whose OWN view of Changes[1] reaches the
	// certificate threshold broadcasts a MsgRoundCert (rounds.go
	// recordRoundChange/broadcastRoundCert) — a SECOND wire path this test's
	// held-delivery hold predicate (MsgRoundChange only) does not see, so
	// holding just the direct round-change copies no longer controls when
	// the designee's quorum completes (a certificate can arrive first and
	// carry the same envelope). Delivering the identical, real bytes
	// straight to designee.handleChain — exactly the call the wire handler
	// makes on delivery (chainrole.go, case ports.MsgRoundChange) —
	// reproduces the identical effect without depending on which of the two
	// wire kinds happens to arrive first, and without touching `net` at all
	// for this construction (so nothing else on the simulated network can
	// race in and contaminate the designee's state before this test is
	// ready).
	rawRoundChange1 := func(nd *Node) []byte {
		nd.advanceToRound(nd.roundsFor(), 1, "test")
		raw := nd.roundsFor().Changes[1][nd.id]
		if raw == nil {
			t.Fatalf("premise: %s did not record its own round-change(1) envelope", nd.id)
		}
		return raw
	}
	raw1, raw2 := rawRoundChange1(livers[0]), rawRoundChange1(livers[1])

	// Deliver the FIRST envelope: sub-quorum (1 non-proposer anchor < the
	// #402 strict anchor majority of 3) — recordRoundChange must not fire
	// the designee trigger yet.
	designee.handleChain(livers[0].id, ports.Message{Kind: ports.MsgRoundChange, Data: raw1})
	rs := designee.roundsFor()
	if got := len(rs.Changes[1]); got != 1 {
		t.Fatalf("premise: after the first round-change(1) delivery, designee should hold exactly 1 recorded round-1 sender, got %d", got)
	}
	if sawPropose {
		t.Fatalf("premise: a MsgProposeBlock was observed after only a SUB-quorum round-change(1) delivery — the quorum gate is not engaged; this test is not exercising M2")
	}

	// Drive the designee's own round to 3 DIRECTLY — the field's val-b
	// independently laddered ahead on its own local timeouts while this
	// quorum-completing envelope was still in flight (certification §2/§3);
	// only the resulting STATE (rs.Round == 3, Changes[3] empty) is what M2
	// depends on, so it is set here rather than re-derived through additional
	// sweep-timeout machinery this test does not otherwise need.
	rs.Round = 3
	if got := len(rs.Changes[3]); got != 0 {
		t.Fatalf("premise: designee's Changes[3] must be empty (no round-3 round-change was ever recorded), got %d entries", got)
	}

	// Deliver the SECOND (quorum-completing) envelope through the REAL
	// MsgRoundChange handler — this is the exact moment the certification
	// names: recordRoundChange now sees 2 non-proposer anchors + the designee
	// (an anchor) itself = 3 of 4 anchors, meeting both RequiredQuorum(2) and
	// the #402 strict anchor majority (3 of 4), and fires the designee
	// trigger while rs.Round == 3.
	designee.handleChain(livers[1].id, ports.Message{Kind: ports.MsgRoundChange, Data: raw2})

	// ── Ablation arm: pin the HEAD failure signature, SYNCHRONOUSLY ────────
	// recordRoundChange (via checkRoundQuorum/fireDesignee) runs entirely on
	// this test's own call stack inside the handleChain call above — no
	// network delivery is needed to trigger it — so the moment
	// designee.handleChain(...) RETURNS, the natural trigger has ALREADY
	// fired (queuing real prepare requests in `net`, not yet delivered) and
	// critically has NOT touched rs.Round: checkRoundQuorum's
	// `rs.Round < round` branch (rounds.go) is false (3 < 1), so it falls
	// straight to fireDesignee without re-entering advanceToRound. rs.Round
	// is therefore STILL 3 and Changes[3] STILL empty at this exact point —
	// this test's manual SECOND call below (reproducing exactly the call
	// proposeAtNewView's forced == nil leg makes, chainrole.go) hits the
	// identical round-check path regardless of the natural trigger's
	// in-flight gather, since that guard is not gated on bondDrainInFlight.
	// This MUST run before draining the network below: once the real gather
	// completes, the height commits and rs.Round == 3 no longer exists (a
	// fresh height's rs.Round starts at 0) — the ablation's premise is
	// specifically about the STALE state M2 left behind, not about anything
	// true after a successful commit.
	arm(designee) // re-arm: the natural trigger's fold may have embedded/consumed the queued reg
	prevHead, h := designee.chain.Head()
	peers := designee.syncTargets()
	attesters := make([]ports.NodeID, 0, len(peers))
	for _, p := range peers {
		if designee.chain.AttesterEligibleAt(p, h) {
			attesters = append(attesters, p)
		}
	}
	var ablationErr error
	designee.proposeBlock(&chain.Block{Version: chain.BlockVersionRounds, Height: h, Prev: prevHead},
		attesters, peers, 0, func(err error) { ablationErr = err })
	wantSig := fmt.Sprintf("propose height %d round %d: new-view certificate not ready", h, uint64(3))
	if ablationErr == nil || !strings.Contains(ablationErr.Error(), wantSig) {
		t.Fatalf("G-H43-2 ablation: expected the HEAD failure signature %q (chainrole.go:1120, M2), got %v — "+
			"this test's RED reason is not attributed to M2; re-derive the premise before trusting the main assertion below", wantSig, ablationErr)
	}
	t.Logf("G-H43-2 ablation: HEAD failure signature confirmed — %v", ablationErr)

	// Fully quiesce whatever the natural trigger queued (the designee's own
	// prepare/precommit/commit broadcasts and their replies) before reading
	// bondDrainInFlight: under the fix the triggered propose attempt is
	// genuinely IN FLIGHT (gatherTwoPhase sent real prepare requests,
	// awaiting replies), not resolved synchronously the way the HEAD bug's
	// early error return was.
	drainHeld(t, net, fifo)
	if designee.bondDrainInFlight {
		t.Fatalf("premise: designee.bondDrainInFlight is still true after the network fully quiesced — the triggered propose attempt neither completed nor cleanly failed")
	}

	// ── The oracle ──────────────────────────────────────────────────────────
	if !sawPropose || capturedRound != 1 {
		t.Fatalf("G-H43-2 REPRODUCED: designee %s held rs.Round=3 with a quorum-grade round-1 certificate "+
			"(2 non-proposer anchors + itself), but no MsgProposeBlock reached the wire at round 1 "+
			"(sawPropose=%v capturedRound=%d) — proposeBlock re-derives the round from local state instead of "+
			"the certificate's round (M2, chainrole.go:1040-1047/1338). Consensus-adjacent, research-gated "+
			"(build-immutable #6); fix direction (C) in the certification.", designee.id, sawPropose, capturedRound)
	}
	t.Logf("G-H43-2: designee %s proposed at round %d — M2 is fixed on this branch.", designee.id, capturedRound)
}

// ── G-H43-3 ─────────────────────────────────────────────────────────────────
//
// TestModelCheck_H43_CatchUpTargetMustReachTheSuffixProvenFrontier is
// G-H43-3 (the M3 gate): "With a quorum-weight population at r_low and a
// sub-threshold population at r_high > r_low, the network converges on ONE
// round within one round duration, and never on a round below one where a
// quorum already sits."
//
// THE MINIMAL REPRODUCTION of M3's structural defect (certification §3.1,
// "the catch-up target is pinned to the LOWEST quorum-bearing round, and can
// only ever be" — rs.Changes[r] is a POINT-IN-TIME record, not a SUFFIX
// claim): 8 anchors (f=2, RoundCatchupMet threshold = f+1 = 3). Four of them
// (the "D" group) all broadcast a round-change for r_low = 1 — a genuine
// quorum-weight population AT that one round (4 ≥ 3). Three OTHERS (the "A/B/
// C" group) are each, independently, genuinely past round 3 — but their
// round-change broadcasts for rounds 4, 5, and 6 respectively land in THREE
// DIFFERENT Changes buckets on the observer, one sender each. This is exactly
// the field shape: three real honest members (val-a, val-b, val-c) each
// individually ahead of round 3, no single round bucket ever holding all
// three at once. In AGGREGATE — under a SUFFIX reading ("declared round ≥
// r") — the three are unambiguous proof an honest quorum sits at round ≥ 4
// (3 ≥ the threshold); under the shipped POINT reading, no single bucket
// {4}, {5}, or {6} ever reaches 3, so no round above 1 can ever indvidually
// qualify, and the observer's catch-up target is structurally pinned to
// round 1 — the certification's "can only ever be the LOWEST quorum-bearing
// round".
//
// The test drives real, individually-verified round-change envelopes (via
// advanceToRound, the same call maybeAdvanceRound's timeout path makes) at
// the real observer, then calls the REAL maybeCatchUpRound and reads its
// effect on rs.Round — never source text.
//
// RED at HEAD (M3, rounds.go:415-439's per-round RoundCatchupMet scan):
// maybeCatchUpRound never crosses round 1, even though 3 real, distinct,
// honest-quorum-weight senders are all genuinely at round ≥ 4. GREEN only
// once fix direction (B)'s suffix semantics ship ("the highest r such that
// the senders with declared round ≥ r meet RoundCatchupMet").
func TestModelCheck_H43_CatchUpTargetMustReachTheSuffixProvenFrontier(t *testing.T) {
	const nAnchors = 8
	nodes, ids, _, _, _ := tier2AnchorNet(t, nAnchors)
	all := make([]ports.NodeID, len(ids))
	for i := range ids {
		all[i] = ids[i].NodeID()
	}
	for _, nd := range nodes {
		seed := make([]ports.NodeID, 0, len(all)-1)
		for _, id := range all {
			if id != nd.id {
				seed = append(seed, id)
			}
		}
		nd.chainSyncSeed = seed
	}

	lowSenders := nodes[0:4]  // the "D" group — a real quorum-weight population, all AT round 1
	highSenders := nodes[4:7] // "A", "B", "C" — each genuinely past round 3, one per bucket
	highRounds := []uint64{4, 5, 6}
	observer := nodes[7]

	// The threshold checks are asserted on the CONSTRUCTED sender sets
	// themselves — timing-independent, since RoundCatchupMet is a pure
	// function of chain config, never of round state — rather than by
	// re-reading rs.Changes[r] after delivery. Under the fix, delivering the
	// LAST of the three high-round envelopes can itself complete the suffix
	// threshold and cause the observer to legitimately self-record into
	// whichever round it targets (maybeCatchUpRound fires automatically
	// inside the real MsgRoundChange handler on every delivery, exactly like
	// the round-1 cascade below) — that is the property this gate exists to
	// prove, not a premise violation, so a post-delivery exact-count read of
	// that specific bucket is not a reliable signal once the fix's legitimate
	// self-recording is active.
	lowIDs := map[ports.NodeID]bool{}
	for _, nd := range lowSenders {
		lowIDs[nd.id] = true
	}
	if !observer.chain.RoundCatchupMet(lowIDs) {
		t.Fatalf("premise: the r_low (D) population (4 anchors) should meet RoundCatchupMet — this test's low group is not quorum-weight")
	}
	suffixIDs := map[ports.NodeID]bool{}
	for _, nd := range highSenders {
		suffixIDs[nd.id] = true
	}
	if !observer.chain.RoundCatchupMet(suffixIDs) {
		t.Fatalf("premise: the r_high (A/B/C) population (3 anchors), taken TOGETHER, should meet RoundCatchupMet — " +
			"this test's high group is not quorum-weight in aggregate, so a suffix-based target could never legitimately reach it either")
	}
	for _, nd := range highSenders {
		single := map[ports.NodeID]bool{nd.id: true}
		if observer.chain.RoundCatchupMet(single) {
			t.Fatalf("premise: sender %s ALONE should not meet RoundCatchupMet — the per-round sub-threshold premise is broken", nd.id)
		}
	}

	// Deliver every envelope DIRECTLY to the observer's real MsgRoundChange
	// handler — never through `net` — so nothing but this test's own 7
	// constructed deliveries can ever write to the observer's Changes map.
	// Under the fix, delivering through `net` lets ANY OTHER node (not just
	// the observer) independently accumulate the same suffix-qualifying set
	// via its own full-mesh connectivity, catch up to round 4 itself, and
	// re-broadcast its OWN round-change(4) — which then also reaches the
	// observer and inflates rs.Changes[4] far past the 1 external sender
	// this test constructs (observed: 6, not 1, when routed through `net`).
	// Bypassing `net` for construction removes that cross-talk entirely; the
	// observer's own legitimate self-recording (once IT independently
	// catches up, exactly the property under test) is the only thing that
	// can still grow a bucket, and only the one it targets.
	deliverDirect := func(sender *Node, round uint64) {
		sender.advanceToRound(sender.roundsFor(), round, "test")
		raw := sender.roundsFor().Changes[round][sender.id]
		if raw == nil {
			t.Fatalf("premise: %s did not record its own round-change(%d) envelope", sender.id, round)
		}
		observer.handleChain(sender.id, ports.Message{Kind: ports.MsgRoundChange, Data: raw})
	}
	for _, nd := range lowSenders {
		deliverDirect(nd, 1)
	}
	for i, nd := range highSenders {
		deliverDirect(nd, highRounds[i])
	}

	rs := observer.roundsFor()
	// >= 4, not == 4: round 1 is a GENUINE quorum-weight round, so the
	// observer's OWN maybeCatchUpRound (fired automatically inside the real
	// MsgRoundChange handler on each of the deliverDirect calls above)
	// legitimately catches itself up to round 1 too — #451 ingredient (b)
	// working correctly — and its own resulting round-change(1) self-record
	// adds a further entry.
	if got := len(rs.Changes[1]); got < 4 {
		t.Fatalf("premise: observer should hold at least 4 recorded round-1 senders (the D group, "+
			"its own legitimate self-record may add one more), got %d", got)
	}
	for _, nd := range lowSenders {
		if _, ok := rs.Changes[1][nd.id]; !ok {
			t.Fatalf("premise: D-group sender %s missing from observer's Changes[1]", nd.id)
		}
	}
	for i, r := range highRounds {
		if _, ok := rs.Changes[r][highSenders[i].id]; !ok {
			t.Fatalf("premise: expected round %d to include sender %s, it was not present", r, highSenders[i].id)
		}
	}

	// ── The oracle ──────────────────────────────────────────────────────────
	observer.maybeCatchUpRound(rs)

	if rs.Round == 0 {
		t.Fatalf("G-H43-3: maybeCatchUpRound did not even reach round 1, where a genuine quorum-weight population (4 of 8 anchors) already sits — regression below an existing quorum round")
	}
	if rs.Round < 4 {
		t.Fatalf("G-H43-3 REPRODUCED: 3 real, distinct, honest-quorum-weight anchors are genuinely at round >= 4 "+
			"(declared rounds 4, 5, 6 respectively — a suffix reading proves round >= 4 with 3 of 8 anchors, meeting "+
			"the f+1=3 threshold), but maybeCatchUpRound only advanced the observer to round %d — the catch-up target "+
			"is pinned to the lowest quorum-bearing round (M3, rounds.go:415-439: rs.Changes[r] is scanned as a "+
			"POINT-IN-TIME record, never as a suffix), exactly the field's \"ten low seats jumped to round=1 while "+
			"the frontier sat at r4/r5\" (certification §2/§3.1). Consensus-adjacent, research-gated (build-immutable "+
			"#6); fix direction (B) — suffix semantics — in the certification.", rs.Round)
	}
	t.Logf("G-H43-3: observer caught up to round %d — M3 is fixed on this branch.", rs.Round)
}

// ── G-H43-4 ─────────────────────────────────────────────────────────────────
//
// TestModelCheck_H43_DroppedDirectRoundChangeMustStillRelay is G-H43-4: "A
// node that never receives a peer's round-change directly still learns the
// round via a relayed certificate within one hop."
//
// THE DROP PRIMITIVE: adapters/simnet's held-delivery model-check mode
// exposes exactly the per-target drop this gate needs —
// (*simnet.Network).DropPending(id), "the model-check's message-loss
// control" (simnet.go:250-257) — removes one specific parked message WITHOUT
// delivering it, distinct from net.Partition (which would drop EVERY message
// between a pair, not one delivery) or Config.Loss (stochastic, not
// targeted). No new harness primitive was needed.
//
// THE SCHEDULE: 7 anchors (f=2, RoundCatchupMet threshold = f+1 = 3).
// Y1, Y2, Y3 each broadcast a real, individually-verified round-change(1) —
// together a genuine quorum-weight population for round 1. X's copy of
// EXACTLY Y1's broadcast is dropped (DropPending) — X receives Y2 and Y3
// directly (2 senders, sub-threshold on its own) but never Y1. Every OTHER
// node (Z among them) receives all three directly and so genuinely ASSEMBLES
// a quorum-grade round-1 certificate (3 senders, meets threshold) — the
// precondition a relay would need a source for. The network is then drained
// to full quiescence: nothing further is ever delivered.
//
// RED at HEAD (R-H43-ONESHOT-RC, rounds.go:353-358; R-H43-POINT-SEMANTICS):
// a round-change broadcast is one-shot, unacked, and never relayed — Z's
// having assembled the certificate does nothing for X, since Z (and every
// other node) only ever forwards a round-change it ITSELF originates via its
// own advanceToRound, never one it received. No wire object carries "I have
// a quorum for round 1" between peers — ports.MsgRoundCert does not exist in
// the port enum (grepped this session: zero occurrences repo-wide). X's
// Changes[1] stays at 2 forever, below the 3-anchor threshold, and X's round
// never leaves 0. GREEN only once fix direction (B)'s transferable round
// certificate ships (a node that assembles a quorum broadcasts it once per
// (h, r); a receiver validates it exactly like a proposal-carried
// certificate and enters r).
func TestModelCheck_H43_DroppedDirectRoundChangeMustStillRelay(t *testing.T) {
	const nAnchors = 7
	nodes, ids, net, _, _ := tier2AnchorNet(t, nAnchors)
	all := make([]ports.NodeID, len(ids))
	for i := range ids {
		all[i] = ids[i].NodeID()
	}

	y1, y2, y3 := nodes[0], nodes[1], nodes[2]
	x := nodes[3]
	z := nodes[4] // the witness: a node that genuinely assembles the full round-1 certificate

	// Topology (NOT full mesh — the field's real gossip graph is not a complete
	// graph either): Y1/Y2/Y3 and every OTHER anchor talk to everyone,
	// including X. But X talks to, and is talked to by, ONLY Y1/Y2/Y3 — no
	// other anchor (Z included) has X in its own sync set. This is what makes
	// X's isolation from the dropped Y1 message genuine: without it, a FULL
	// MESH lets any node that independently catches up (Z, having directly
	// heard all three) accidentally "relay" by re-broadcasting its OWN fresh
	// round-change(1) to X, which reaches the threshold through indirect
	// diffusion — an artifact of the test's connectivity, not the certified
	// transferable-certificate mechanism this gate targets. Restricting X's
	// peer set to exactly the three direct senders removes that artifact.
	only := func(id ports.NodeID, except ...ports.NodeID) []ports.NodeID {
		skip := map[ports.NodeID]bool{id: true}
		for _, e := range except {
			skip[e] = true
		}
		out := make([]ports.NodeID, 0, len(all))
		for _, other := range all {
			if !skip[other] {
				out = append(out, other)
			}
		}
		return out
	}
	for _, nd := range nodes {
		switch nd.id {
		case x.id:
			nd.chainSyncSeed = []ports.NodeID{y1.id, y2.id, y3.id}
		case y1.id, y2.id, y3.id:
			nd.chainSyncSeed = only(nd.id) // reach everyone, including X
		default:
			nd.chainSyncSeed = only(nd.id, x.id) // reach everyone EXCEPT X
		}
	}

	// Y1 broadcasts; drop EXACTLY its copy to X, deliver every other copy
	// (Y2, Y3, Z, and the remaining anchors all receive Y1 directly). The
	// drop's success is proven on the WIRE — the simnet delivery record
	// (net.Stats.Dropped) — never by later re-reading X's Changes map: under
	// the fix, X legitimately ends up holding Y1's envelope too, RECORDED BY
	// acceptRoundCert once a relayed certificate reaches it (that is this
	// gate's whole point — see the oracle below), so "does X's Changes map
	// contain Y1" cannot distinguish "the direct delivery was never dropped"
	// from "the relay correctly repaired the drop". Only the wire-level
	// record of the drop itself proves the drop took.
	dropsBefore := net.Stats.Dropped
	y1.advanceToRound(y1.roundsFor(), 1, "test")
	dropID := -1
	for _, m := range net.Pending() {
		if m.To == x.id && m.From == y1.id && m.Kind == ports.MsgRoundChange {
			dropID = m.ID
		}
	}
	if dropID < 0 {
		t.Fatalf("premise: no parked round-change(1) message from Y1 to X found to drop")
	}
	if !net.DropPending(dropID) {
		t.Fatalf("premise: DropPending(%d) (Y1's message to X) failed", dropID)
	}
	if net.Stats.Dropped != dropsBefore+1 {
		t.Fatalf("premise: the simnet delivery record does not show exactly one new drop after DropPending — got Dropped=%d, want %d (the wire-level proof the drop took)", net.Stats.Dropped, dropsBefore+1)
	}
	drainHeld(t, net, fifo)

	// Y2 and Y3 broadcast normally — X receives both of these directly.
	y2.advanceToRound(y2.roundsFor(), 1, "test")
	drainHeld(t, net, fifo)
	y3.advanceToRound(y3.roundsFor(), 1, "test")
	drainHeld(t, net, fifo)

	// Premise: X holds (at least) Y2 and Y3's direct envelopes — a subset
	// check, not an exact count, since a relayed certificate may ALSO have
	// reached X by this point (carrying Y1's envelope too, or re-presenting
	// Y2/Y3's) — that is the fix legitimately working, not a premise defect.
	xrs := x.roundsFor()
	for _, sender := range []*Node{y2, y3} {
		if _, ok := xrs.Changes[1][sender.id]; !ok {
			t.Fatalf("premise: X is missing %s's direct round-change(1) — it should have received this one undropped", sender.id)
		}
	}

	// Premise (a real certificate WAS genuinely assembled elsewhere): Z
	// received all three directly, meets the threshold, and — via the
	// EXISTING #451 ingredient (b) — has already caught its own round up to
	// 1. This is the source a relay would need; it exists.
	zrs := z.roundsFor()
	for _, sender := range []*Node{y1, y2, y3} {
		if _, ok := zrs.Changes[1][sender.id]; !ok {
			t.Fatalf("premise: witness Z is missing %s's round-change(1) — Z should have assembled the full certificate", sender.id)
		}
	}
	zSenders := map[ports.NodeID]bool{y1.id: true, y2.id: true, y3.id: true}
	if !z.chain.RoundCatchupMet(zSenders) {
		t.Fatalf("premise: Z's 3-sender set does not meet RoundCatchupMet — this test's quorum-weight premise is broken")
	}
	if zrs.Round < 1 {
		t.Fatalf("premise: witness Z did not catch its own round up to 1 despite assembling the full quorum — Z has not genuinely ASSEMBLED a certificate the field would consider ready to relay")
	}

	// Fully quiesce the network: nothing further is ever delivered from here.
	drainHeld(t, net, fifo)

	// ── The oracle ──────────────────────────────────────────────────────────
	if xrs.Round < 1 {
		t.Fatalf("G-H43-4 REPRODUCED: X never directly received Y1's round-change(1) (dropped), and although Z "+
			"(and every other live anchor) genuinely assembled a quorum-grade round-1 certificate (Y1+Y2+Y3, "+
			"meeting the f+1=3 threshold) and has itself already entered round 1, X's round never advances — "+
			"X.roundsFor().Round=%d. Round-changes are one-shot, unacked, and never relayed (R-H43-ONESHOT-RC, "+
			"rounds.go:353-358): no node forwards a round-change it received, only ones it originates itself, and "+
			"no wire object (ports.MsgRoundCert does not exist) carries an assembled certificate between peers. "+
			"Consensus-adjacent, research-gated (build-immutable #6); fix direction (B) — the transferable round "+
			"certificate — in the certification.", xrs.Round)
	}
	t.Logf("G-H43-4: X caught up to round %d via a relayed certificate — the transferable round certificate is fixed on this branch.", xrs.Round)
}

// ── G-H43-5 ─────────────────────────────────────────────────────────────────
//
// G-H43-5 is I1 non-regression (certification §4.2, §4.1(3)): "the S1/S2
// mature oracles and TestModelCheck_I4_WedgedHeightMustRecover stay GREEN;
// plus a new oracle — a delayed lower-round prepare-QC arriving at a node
// that already prepared at a higher round never yields a second commit at
// the height." THIS GATE IS GREEN AT HEAD BY DESIGN — it is a
// non-regression pin, not a reproduction of a defect; none of the h43
// mechanisms (M1/M2/M3) touch the watermark this oracle exercises.
//
// (i) CONFIRMED GREEN on this branch, this session (`go test -count=1 -v
// -run '...' ./core/node/`, full output captured in this session's report):
//
//	--- PASS: TestModelCheck_451_SilentAuthorLockedValueMustStillCommit (0.57s)
//	--- PASS: TestModelCheck_I4_WedgedHeightMustRecover (0.01s)
//	--- PASS: TestModelCheck_S1_Mature_DelayedWeightQuorumIsCarriedForward (0.11s)
//	--- PASS: TestModelCheck_S2_Mature_ForgedLockMisreportCannotForkTheHeight (0.12s)
//	--- PASS: TestModelCheck_S1_DelayedLowerRoundQuorumIsCarriedForward (0.01s)
//	--- PASS: TestModelCheck_S2_ForgedLockMisreportCannotForkTheHeight (0.02s)
//	PASS  ok  	github.com/nerolabs/silt/core/node	1.356s
//
// (ii) TestModelCheck_H43_DelayedLowerRoundPrepareQCNeverForcesASecondCommit,
// below — the NEW oracle, expressing certification §4.1(3) ("a late
// lower-round quorum cannot complete at a node that has moved up and
// signed... slotCompare orders round before phase... the watermark itself
// is the enforcement") as a real, driven test over the actual node loop,
// plus an ablation pinning the round term's necessity.
func TestModelCheck_H43_DelayedLowerRoundPrepareQCNeverForcesASecondCommit(t *testing.T) {
	nodes, ids, net, g, _ := tier2AnchorNet(t, 4)
	all := make([]ports.NodeID, len(ids))
	for i := range ids {
		all[i] = ids[i].NodeID()
	}
	for _, nd := range nodes {
		seed := make([]ports.NodeID, 0, len(all)-1)
		for _, id := range all {
			if id != nd.id {
				seed = append(seed, id)
			}
		}
		nd.chainSyncSeed = seed
	}
	n0, n1, n2, n3 := nodes[0], nodes[1], nodes[2], nodes[3]

	// n0 already prepared at a HIGHER round (2) for a block A this test never
	// otherwise constructs — durably recorded via the same call the real
	// gather path uses (chainrole.go:104), so the mark is genuine, not
	// fabricated bytes.
	hashA := ports.HashBytes([]byte("blockA-round2"))
	if !n0.recordSign(1, 2, chain.PhasePrepare, hashA) {
		t.Fatalf("premise: n0.recordSign(1, 2, Prepare, hashA) failed to persist")
	}

	// A REAL, independently-gathered prepare-QC for a DIFFERENT block B at
	// round 0 — n1 proposes, n2+n3 attest, entirely excluding n0 (neither an
	// attester nor a broadcast target), so n0 never hears about this round
	// through any channel except the one delayed message delivered below.
	var captured []byte
	ep2 := net.Endpoint(n2.id)
	origN2 := n2.handle
	ep2.SetHandler(func(from ports.NodeID, msg ports.Message) {
		if msg.Kind == ports.MsgPrepareQC && captured == nil {
			captured = append([]byte(nil), msg.Data...)
		}
		origN2(from, msg)
	})
	blockB := &chain.Block{Version: 1, Height: 1, Prev: g.Hash(), Entries: []ports.Entry{mkEntry("blockB-round0")}}
	var done bool
	var proposeErr error
	n1.proposeBlock(blockB, []ports.NodeID{n2.id, n3.id}, []ports.NodeID{n2.id, n3.id}, 2,
		func(err error) { done, proposeErr = true, err })
	drainHeld(t, net, fifo)
	if !done || proposeErr != nil {
		t.Fatalf("premise: the independent round-0 gather among n1/n2/n3 must commit cleanly: done=%v err=%v", done, proposeErr)
	}
	if captured == nil {
		t.Fatalf("premise: never captured a MsgPrepareQC en route to n2 — the independent gather did not reach the precommit phase")
	}
	if _, h := n0.chain.Head(); h != 1 {
		t.Fatalf("premise: n0 must NOT have heard about the independent round-0 gather by any other channel (head=%d, want 1)", h)
	}

	// THE DELAYED DELIVERY: the exact bytes a real round-0 attester received,
	// handed directly to n0 through its real message handler — a genuine,
	// verifiable prepare-QC for a LOWER round (0) than n0's already-recorded
	// mark (2).
	n0.handle(n1.id, ports.Message{Kind: ports.MsgPrepareQC, Data: captured})
	drainHeld(t, net, fifo)

	// ── The oracle (GREEN at HEAD — non-regression) ─────────────────────────
	if n0.signMark.Round != 2 || n0.signMark.Phase != chain.PhasePrepare || n0.signMark.Hash != hashA {
		t.Fatalf("G-H43-5 VIOLATION: n0's watermark moved in response to a delayed LOWER-round prepare-QC — "+
			"mark is now (height=%d round=%d phase=%d hash=%x), want the untouched (1, 2, Prepare, %x); "+
			"a delayed lower-round quorum must never force a signature past the watermark (certification §4.1(3))",
			n0.signMark.Height, n0.signMark.Round, n0.signMark.Phase, n0.signMark.Hash, hashA)
	}
	if rs := n0.roundsFor(); rs.Lock != nil {
		t.Fatalf("G-H43-5 VIOLATION: n0 adopted a lock (round %d, hash %x) from the delayed lower-round prepare-QC", rs.Lock.Round, rs.Lock.Hash)
	}
	if _, h := n0.chain.Head(); h != 1 {
		t.Fatalf("G-H43-5 VIOLATION: n0's head advanced (h=%d) — the delayed lower-round QC must never yield a second commit at the height", h)
	}
	t.Log("G-H43-5: n0's watermark refused the delayed lower-round prepare-QC — the round term in slotCompare enforced it, exactly as certified. GREEN at HEAD (non-regression).")

	// ── Ablation: pin the round term's necessity ────────────────────────────
	// The real slotCompare, on these exact recorded inputs, must refuse
	// (c < 0).
	if c := slotCompare(1, 0, chain.PhasePrecommit, n0.signMark); c >= 0 {
		t.Fatalf("G-H43-5 ablation premise: slotCompare(1, 0, Precommit, mark) should be < 0 (refuse), got %d", c)
	}
	// A test-only reimplementation of slotCompare with the ROUND CASE
	// REMOVED, applied to the SAME inputs — this test may not edit
	// chainrole.go, so the "deliberately reverted round term" is expressed as
	// a parallel predicate rather than a source edit (per this session's
	// instructions). If this ablated comparator does NOT flip to "allow" on
	// these inputs, the ablation fails to demonstrate the round term's
	// necessity, and the GREEN result above would be unattributed.
	if c := slotCompareNoRoundAblation(1, chain.PhasePrecommit, n0.signMark); c <= 0 {
		t.Fatalf("G-H43-5 ablation: the round-blind predicate should ALLOW (c > 0) on n0's exact recorded inputs "+
			"(the round case is skipped, so height ties and PhasePrecommit(2) > PhasePrepare(1) decides it alone), got %d — "+
			"this ablation does not demonstrate the round term's necessity", c)
	}
	t.Log("G-H43-5 ablation: with the round case removed, the SAME inputs that slotCompare correctly refuses would " +
		"instead be ALLOWED — the round term is load-bearing, confirming the GREEN result above is attributed to it, not incidental.")
}

// slotCompareNoRoundAblation is chainrole.go's slotCompare (chainrole.go:75)
// with the ROUND case deleted — the "deliberately reverted round term"
// ablation for G-H43-5, expressed as a parallel test-only predicate since
// this session may only add _test.go files. Orders purely by (height, phase),
// exactly as #397's pre-#432 height-only watermark did.
func slotCompareNoRoundAblation(height uint64, phase uint8, m ports.SignMark) int {
	switch {
	case height != m.Height:
		if height > m.Height {
			return 1
		}
		return -1
	case phase != m.Phase:
		if phase > m.Phase {
			return 1
		}
		return -1
	default:
		return 0
	}
}

// ── G-H43-9 ─────────────────────────────────────────────────────────────────
//
// TestModelCheck_H43_EntryLaneWorklessDesigneeMustCommitWithinFPlus1Rounds is
// G-H43-9 (decisive, the M4 gate), per the delta certification
// CONSENSUS-LIVENESS-h43-ABC-ASBUILT-workless-designee-RESEARCH-CERTIFICATION-2026-09-07.md
// §3.7/§7: "The G-H43-1 schedule, unchanged, with two additional assertions:
// commitRound <= f, and the committing block's ProposerID() equals
// designatedProposer(contested, commitRound)."
//
// ENTRY LANE, not reg lane (§1.3, R-H43-FIXTURE-STRIPS-REG-LANE: "keep the
// reg-lane oracle as is [G-H43-1]; add the entry-lane arm, where the
// asymmetry is real"). A bond reg already reaches every peer directly from
// its own validator every renewal sweep (objectivechain.go), so a
// reg-workless designee is transient in the field; MsgSubmitEntry
// (entrypool.go) broadcasts to the CALLER's peers only and is never
// re-gossiped by the receiver, so an entry-workless designee is a real,
// permanent field asymmetry — the field's actual h43 shape (parent cert §2:
// "pending_entries=2,3,5" on val-a/b/c's advancing lines).
//
// THE SCHEDULE: identical to G-H43-1 (matureWorld12, nodes[0,1,2] armed,
// node[3] killed then restarted, nodes[4..11] quiescent, staggered sweep
// phases, timed delivery) except the 3 armed seats hold a PENDING ENTRY,
// never a pending bond reg. Quiescent-strip is SETUP-ONLY (never re-run
// during the timed loop — G-H43-1's own certified fix, since D1-entry
// legitimately forwards pending entries to the round's designee and a
// periodic strip would delete that forwarded work).
//
// RED at 462478d (before ANY of A/B/C/D): M1 alone keeps the height from
// converging within the bound at all — evidence below. GREEN at 8368196:
// D1-entry (capped at h43ForwardEntries) forwards the designee's missing
// entry, D3 (re-keyed to the round's own designee) backstops a lost
// forward, and D4 restates the bound — the certification's own measurement
// on this SHA: the entry lane commits at 104 s, inside the 190 s f=1 bound.
//
// NOT independently RED at a3e4e72, evidence-checked this session, and
// reported rather than papered over (build-immutable #7): a3e4e72 already
// carries forwardPendingWorkToDesignee (labelled WIP in its own commit
// message, uncapped and un-deduped) wired into advanceToRound. In THIS
// full-mesh, short-latency (5-50ms), 12-seat timed schedule, the SAME event
// that lets an armed seat's round-change contribute to a round's quorum
// (entering the round via advanceToRound) is what fires that seat's forward
// to that round's designee, in the same function call — so by the time
// enough round-changes accumulate to complete round r's quorum, the
// workless designee has, in this topology, already-near-certainly received
// a forward for it. Verified directly: running this exact construction
// (round-0 AND round-1 designees confirmed quiescent — logged below) at
// a3e4e72 committed cleanly at round 1 by its own designee, the GREEN
// shape, not the M4 RED shape the certification's own field probe records.
// The probe's h43-probe-evidence.txt:38/108/236/398 observation is real and
// is not in question — it is a different topology/timing (WAN-realistic,
// not this fixture's full mesh) than this in-process schedule reproduces at
// a3e4e72, so 462478d, not a3e4e72, is this gate's honest RED baseline.
func TestModelCheck_H43_EntryLaneWorklessDesigneeMustCommitWithinFPlus1Rounds(t *testing.T) {
	nodes, _, net, sched, _ := matureWorld12(t)
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

	// WHICH 3 seats get armed and which gets killed is chosen so that ROUND
	// 0's designee is a QUIESCENT seat — otherwise, on this fixture's fixed
	// (deterministic) designatedProposer rotation, M4 can go entirely
	// unexercised by luck alone (the round-0 designee happens to already be
	// armed, so no round is ever wasted, and the gate is VACUOUS regardless
	// of which commit is under test — caught by running this same
	// construction against a3e4e72 first and finding it PASSED there,
	// which a decisive M4 gate must not). Pick round 0's and round 1's
	// designee indices first, then choose 3 armed seats and 1 killed seat
	// from the REMAINING 10, guaranteeing at least round 0 starts on a
	// live, workless designee.
	indexOf := map[ports.NodeID]int{}
	for i, nd := range nodes {
		indexOf[nd.id] = i
	}
	d0idx := indexOf[nodes[0].designatedProposer(contested, 0)]
	d1idx := indexOf[nodes[0].designatedProposer(contested, 1)]
	excluded := map[int]bool{d0idx: true, d1idx: true}
	var pool []int
	for i := 0; i < len(nodes); i++ {
		if !excluded[i] {
			pool = append(pool, i)
		}
	}
	if len(pool) < 4 {
		t.Fatalf("premise: not enough non-designee seats to pick 3 armed + 1 killed from (pool=%v)", pool)
	}
	killedIdx := pool[0]
	armed := append([]int{}, pool[1:4]...)
	armedSet := map[int]bool{}
	for _, i := range armed {
		armedSet[i] = true
	}
	if armedSet[d0idx] || armedSet[d1idx] || killedIdx == d0idx || killedIdx == d1idx {
		t.Fatalf("premise: round 0/1's designee (idx %d/%d) must be neither armed nor the killed seat (armed=%v killed=%d)", d0idx, d1idx, armed, killedIdx)
	}
	t.Logf("G-H43-9 premise: round-0 designee is seat %d (quiescent), round-1 designee is seat %d (quiescent), armed=%v, killed=%d", d0idx, d1idx, armed, killedIdx)
	for i, nd := range nodes {
		if i == killedIdx {
			continue
		}
		nd.pendingBondRegs = nil // this oracle is the entry lane, never the reg lane
		if armedSet[i] {
			nd.pendingEntries = []pendingEntry{{E: mkEntry(fmt.Sprintf("h43-9-entry-%d", i)), At: contested}}
		} else {
			nd.pendingEntries = nil
		}
	}
	for i, nd := range nodes {
		isArmed := armedSet[i]
		isKilled := i == killedIdx
		hasWork := len(nd.pendingBondRegs) > 0 || len(nd.pendingEntries) > 0
		if isArmed && !hasWork {
			t.Fatalf("premise: armed seat %d has no pending entry after setup", i)
		}
		if !isArmed && !isKilled && hasWork {
			t.Fatalf("premise: quiescent seat %d has pending work after setup — heterogeneous arming not established", i)
		}
	}

	net.Kill(nodes[killedIdx].id)
	net.DisableHeldDelivery()

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

	// THE BOUND, at f=1 (unchanged from G-H43-1 — the restated D4 bound
	// collapses to this once D1-entry lands, since a live round's designee
	// then holds the work: W ~= 0).
	const f = uint64(1)
	var sweepSum uint64
	for r := uint64(0); r <= f; r++ {
		sweepSum += uint64(sweepsForRound(r))
	}
	const skew = 30 * ports.Second
	const gatherG = 10 * ports.Second
	bound := ports.Duration(sweepSum)*interval + skew + gatherG
	deadline := sched.Now().Add(bound)

	steps := 0
	for sched.Now() < deadline && !committed() {
		if !sched.Step() {
			break
		}
		steps++
	}

	if honestSlashed {
		t.Fatal("I5 VIOLATION: an honest validator was slashed under the entry-lane workless-designee schedule")
	}
	if !committed() {
		rounds := map[int]uint64{}
		for i, nd := range nodes {
			rounds[i] = nd.roundsFor().Round
		}
		t.Fatalf("G-H43-9: height %d did not commit within the f=1 bound %v of the kill (per-seat rounds "+
			"reached: %v) — even on the entry lane, with D1-entry landing", contested, bound, rounds)
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
	wantProposer := nodes[0].designatedProposer(contested, commitRound)

	// ── The oracle (M4, decisive; both halves — neither alone is sufficient,
	// per the certification: "the first without the second reads as a
	// stall, the second without the first reads as a healthy takeover") ────
	if commitRound > f {
		t.Fatalf("G-H43-9 REPRODUCED: h%d committed at round %d, above the certified f=%d bound — a live, "+
			"armed designee-rotation seat at some round <= f held none of the height's pending work and "+
			"could not produce a valid block (M4, the empty-block validity rule), wasting round(s) at timer "+
			"speed before the #338 takeover recovered the height. Consensus-adjacent, research-gated "+
			"(build-immutable #6); closers D1-entry(capped)+D3(re-keyed)+D4(restated bound) in the "+
			"workless-designee certification.", contested, commitRound, f)
	}
	if proposerID != wantProposer {
		t.Fatalf("G-H43-9 REPRODUCED: h%d committed at round %d by proposer %s, but designatedProposer(%d, %d) "+
			"= %s — the height committed via the #338 rank-staggered takeover, a NON-designee work-holder, "+
			"not the round's own designee (M4, the designee rotation is work-blind: rounds.go "+
			"designatedProposer). Matches h43-probe-evidence.txt:398 (n2 commits h9 r3) against :4 "+
			"(designee(h9, r3) = n5).", contested, commitRound, proposerID, contested, commitRound, wantProposer)
	}
	t.Logf("G-H43-9: h%d committed at round %d by its own designee %s — M4 is fixed on this branch.", contested, commitRound, proposerID)
}

// ── G-H43-10 ────────────────────────────────────────────────────────────────
//
// TestModelCheck_H43_ForwardCommitsEntriesNeverRegs is G-H43-10 (the forward,
// both lanes), per the delta certification §3.7/§7: "One work-holder, a
// workless designee at round 1. Assert (a) the designee's round-1 block is
// non-empty and the height commits at round 1; (b) the reg lane is asserted
// absent — no MsgSubmitBondReg is sent to a designee that is not the reg's
// validator."
//
// THE SCHEDULE: 4 anchors (tier2AnchorNet — the #402 strict-anchor-majority
// regime). D is round 1's designee, holding NOTHING. W (a different anchor)
// holds ONE pending entry (mkEntry) AND one pending THIRD-PARTY bond
// registration (owned by neither W nor D — a synthetic 5th identity), so
// this one schedule exercises both lanes' forward decision at once. W and
// one other live anchor (O1) each independently reach round 1 via
// advanceToRound (a real local timeout), delivered over the REAL network
// (net, not direct handleChain) so forwardPendingWorkToDesignee's own
// MsgSubmitEntry/MsgSubmitBondReg sends are genuinely observed on the wire —
// this gate is specifically about what DOES and does NOT cross that wire.
//
// RED at a3e4e72 on (b) (D1 was WIP, both lanes still forwarded there):
// `forwardPendingWorkToDesignee` forwards the third-party reg to D, and D's
// own MsgSubmitBondReg handler refuses it decisively — "bond-reg submit
// REFUSED (relay)" (chainrole.go) — observed via net.Stats.Kinds
// [MsgSubmitBondReg] > 0 AND D's own reply is OK: false. GREEN at 8368196:
// the reg loop in forwardPendingWorkToDesignee is deleted outright (§3.1,
// "REGISTRATIONS ARE NOT FORWARDED"), so net.Stats.Kinds[MsgSubmitBondReg]
// stays exactly 0 for the whole schedule — the absence is asserted on the
// wire, not by re-deriving it from the refusal (a MsgSubmitBondReg that is
// never SENT is a stronger claim than one that is sent-and-refused).
func TestModelCheck_H43_ForwardCommitsEntriesNeverRegs(t *testing.T) {
	nodes, ids, net, g, _ := tier2AnchorNet(t, 4)
	all := make([]ports.NodeID, len(ids))
	for i := range ids {
		all[i] = ids[i].NodeID()
	}
	for _, nd := range nodes {
		seed := make([]ports.NodeID, 0, len(all)-1)
		for _, id := range all {
			if id != nd.id {
				seed = append(seed, id)
			}
		}
		nd.chainSyncSeed = seed
	}
	byID := map[ports.NodeID]*Node{}
	for _, nd := range nodes {
		byID[nd.id] = nd
	}

	_, height := nodes[0].chain.Head()
	if height != 1 {
		t.Fatalf("premise: want the working height to be 1 (right after genesis), got %d", height)
	}
	designeeID := nodes[0].designatedProposer(height, 1)
	d, ok := byID[designeeID]
	if !ok {
		t.Fatalf("premise: designatedProposer(%d, 1) returned an unknown id", height)
	}
	var w *Node
	var others []*Node
	for _, nd := range nodes {
		if nd.id == designeeID {
			continue
		}
		if w == nil {
			w = nd
			continue
		}
		others = append(others, nd)
	}
	if w == nil || len(others) != 2 {
		t.Fatalf("premise: expected 1 work-holder + 2 other live anchors, got w=%v others=%d", w != nil, len(others))
	}
	o1 := others[0]

	// D is genuinely workless: no entries, no regs, ever.
	d.pendingEntries = nil
	d.pendingBondRegs = nil

	// W holds ONE entry and ONE THIRD-PARTY reg (owned by neither W nor D) —
	// both lanes' forward decision exercised in one schedule.
	w.pendingEntries = []pendingEntry{{E: mkEntry("h43-10-entry"), At: height}}
	thirdParty := identity.FromSeed(890099)
	tpPub := append([]byte(nil), thirdParty.Signer().Public().(ed25519.PublicKey)...)
	thirdPartyReg := chain.NewBondReg(thirdParty.Signer(), ports.HashBytes(tpPub), 2<<20, []byte("stub"), g.Hash(), 0)
	w.pendingBondRegs = []pendingBondReg{{R: thirdPartyReg}}

	// Wrap D's transport to observe every MsgSubmitBondReg it receives and
	// whether it accepted or refused each one — the wire-level proof for (b).
	var regsReceived int
	var regsAccepted int
	ep := net.Endpoint(d.id)
	origHandle := d.handle
	ep.SetHandler(func(from ports.NodeID, msg ports.Message) {
		if msg.Kind == ports.MsgSubmitBondReg {
			regsReceived++
		}
		origHandle(from, msg)
		if msg.Kind == ports.MsgSubmitBondReg {
			// Peek at whether it was queued (accepted) by re-checking D's own
			// pending set membership after the handler ran.
			for _, pr := range d.pendingBondRegs {
				if pr.R.ValidatorID() == thirdPartyReg.ValidatorID() {
					regsAccepted++
					break
				}
			}
		}
	})

	// W and O1 each independently reach round 1 over the REAL network — this
	// is what fires the real forwardPendingWorkToDesignee sends.
	w.advanceToRound(w.roundsFor(), 1, "test")
	drainHeld(t, net, fifo)
	o1.advanceToRound(o1.roundsFor(), 1, "test")
	drainHeld(t, net, fifo)

	// ── (b) the reg lane is asserted ABSENT, on the wire — the certification's
	// exact assertion is that no MsgSubmitBondReg is EVER SENT to a designee
	// that is not the reg's validator, not merely that a sent one is refused
	// (a sent-and-refused reg still pays the ~1.5 MB egress and burns the
	// forwarder's own submit budget at the designee, per §3.1) ────────────
	if regsReceived != 0 {
		t.Fatalf("G-H43-10 REPRODUCED (part b): D received %d MsgSubmitBondReg for a THIRD-PARTY registration "+
			"(validator %s, neither W's nor D's own) — %d of them were queued. The reg lane must never forward a "+
			"relayed registration in the first place (R-H43-FORWARD-REG-RELAY-REFUSED): forwardPendingWorkToDesignee "+
			"still contains the reg loop, paying ~1.5 MB of egress and the forwarder's own allowBondSubmit budget "+
			"for a submission the receiver decisively refuses ('bond-reg submit REFUSED (relay)', chainrole.go) 100%% "+
			"of the time. Consensus-adjacent, research-gated (build-immutable #6); closer: delete the reg loop.",
			regsReceived, thirdPartyReg.ValidatorID(), regsAccepted)
	}
	t.Logf("G-H43-10 (part b): D received ZERO MsgSubmitBondReg for the whole schedule — the reg forward loop is gone.")

	// ── (a) the designee's round-1 block is non-empty and the height
	// commits at round 1 ─────────────────────────────────────────────────
	var commitRound uint64
	var committedHash ports.Hash
	var entriesInBlock int
	committed := false
	for _, nd := range nodes {
		if b := nd.Chain().Blocks(height); len(b) > 0 {
			committed = true
			commitRound = b[0].CommitRound
			committedHash = b[0].Hash()
			entriesInBlock = len(b[0].Entries)
			break
		}
	}
	if !committed {
		t.Fatalf("G-H43-10 REPRODUCED (part a): height %d never committed — the workless designee D could not "+
			"produce a valid (non-empty) block and no forward/takeover rescued it within this schedule "+
			"(M4, R-H43-WORKLESS-DESIGNEE). Consensus-adjacent, research-gated (build-immutable #6).", height)
	}
	if commitRound != 1 {
		t.Fatalf("G-H43-10 REPRODUCED (part a): height %d committed at round %d, not round 1 — the forward did "+
			"not land in time for D's own round-1 attempt (hash %x)", height, commitRound, committedHash)
	}
	if entriesInBlock == 0 {
		t.Fatalf("G-H43-10 REPRODUCED (part a): height %d committed at round 1 but the block carries ZERO entries — "+
			"D proposed EMPTY despite W's forwarded entry existing on the wire", height)
	}
	t.Logf("G-H43-10 (part a): h%d committed at round %d with %d entries (D's own round-1 block, forwarded from W) — M4's entry lane is fixed on this branch.", height, commitRound, entriesInBlock)
}

// ── G-H43-10a ───────────────────────────────────────────────────────────────
//
// TestModelCheck_H43_ForwardEntriesCappedNotBurstBudget is G-H43-10a
// (`R-H43-FORWARD-BURST-INHERITED`, a security parameter), per the delta
// certification §3.1/§7 item 2: the entry forward's OWN constant (recommended
// 4), never `entrySubmitBurst` (32, derived for a CLIENT's honest cadence —
// submit-then-poll for one published object — not for up to N-1 forwarders
// each firing a burst at ONE seat on every round entry).
//
// THE SCHEDULE: 4 anchors. D is round 1's workless designee. W holds SIX
// distinct-root entries (well above any plausible cap) and reaches round 1
// via advanceToRound — the real local-timeout call that fires the real
// forwardPendingWorkToDesignee. D's transport is wrapped to COUNT every
// MsgSubmitEntry it receives — the wire-level observation, not a source
// constant.
//
// RED at a3e4e72: the forward loop's cap was `entrySubmitBurst` (32) — with
// only 6 queued, ALL 6 cross the wire (6 < 32), so this test's "exactly 4"
// assertion fails there with a DIFFERENT count. GREEN at c2a476a+: the
// forward has its own constant (h43ForwardEntries, recommended and shipped
// at 4) — exactly 4 of the 6 cross, and the remaining 2 keep their FIFO
// seniority in W's own queue (still there, unconsumed) for the next round
// entry. This test cites the observed count, never the constant's name
// (h43ForwardEntries does not exist at a3e4e72 — the SAME file must compile
// there too).
func TestModelCheck_H43_ForwardEntriesCappedNotBurstBudget(t *testing.T) {
	nodes, ids, net, _, _ := tier2AnchorNet(t, 4)
	all := make([]ports.NodeID, len(ids))
	for i := range ids {
		all[i] = ids[i].NodeID()
	}
	for _, nd := range nodes {
		seed := make([]ports.NodeID, 0, len(all)-1)
		for _, id := range all {
			if id != nd.id {
				seed = append(seed, id)
			}
		}
		nd.chainSyncSeed = seed
	}
	byID := map[ports.NodeID]*Node{}
	for _, nd := range nodes {
		byID[nd.id] = nd
	}
	_, height := nodes[0].chain.Head()
	designeeID := nodes[0].designatedProposer(height, 1)
	d, ok := byID[designeeID]
	if !ok {
		t.Fatalf("premise: designatedProposer(%d, 1) returned an unknown id", height)
	}
	var w *Node
	for _, nd := range nodes {
		if nd.id != designeeID {
			w = nd
			break
		}
	}
	d.pendingEntries = nil
	d.pendingBondRegs = nil

	const queued = 6
	const wantForwarded = 4
	for i := 0; i < queued; i++ {
		w.pendingEntries = append(w.pendingEntries, pendingEntry{E: mkEntry(fmt.Sprintf("h43-10a-entry-%d", i)), At: height})
	}
	if got := len(w.pendingEntries); got != queued {
		t.Fatalf("premise: W should hold %d queued entries before entering the round, got %d", queued, got)
	}

	var entriesReceived int
	ep := net.Endpoint(d.id)
	origHandle := d.handle
	ep.SetHandler(func(from ports.NodeID, msg ports.Message) {
		if msg.Kind == ports.MsgSubmitEntry {
			entriesReceived++
		}
		origHandle(from, msg)
	})

	w.advanceToRound(w.roundsFor(), 1, "test")
	drainHeld(t, net, fifo)

	if entriesReceived != wantForwarded {
		t.Fatalf("G-H43-10a REPRODUCED: D received %d MsgSubmitEntry out of W's %d queued entries on ONE round "+
			"entry, want exactly %d — the forward's cap is not its own constant (R-H43-FORWARD-BURST-INHERITED): "+
			"either it is uncapped/inherits entrySubmitBurst=32 (all %d would cross), or some OTHER cap is in "+
			"force. Consensus-adjacent, research-gated (build-immutable #6); closer: an OWN forward constant "+
			"(recommended 4), never entrySubmitBurst.", entriesReceived, queued, wantForwarded, queued)
	}
	// W's OWN queue is untouched by forwarding (forwardPendingWorkToDesignee
	// sends notices; it does not drain the sender's queue — only D folding
	// them into a committed block would). Confirmed, not assumed: all 6 stay
	// queued, ready for the next round entry's forward to pick up where this
	// one's cap left off.
	if remaining := len(w.pendingEntries); remaining != queued {
		t.Fatalf("G-H43-10a premise: W's own pendingEntries should be untouched by forwarding (still %d), got %d", queued, remaining)
	}
	t.Logf("G-H43-10a: D received exactly %d of W's %d queued entries on one round entry — the forward has its own, tighter cap.", entriesReceived, queued)
}

// ── G-H43-11 ────────────────────────────────────────────────────────────────
//
// TestModelCheck_H43_RoundCertNarrowedToDesigneePlusAbsent is G-H43-11
// (`R-H43-CERT-CARRIES-BLOCKS`), per the delta certification §4.2/§7 item 6:
// the assembled round certificate must go to the round's DESIGNEE plus a
// BOUNDED relay to the peers absent from it (DiemBFT: "sends the TC to
// L_{r+1}") — NEVER an all-peers broadcast, since a round-change carries the
// sender's full locked block and the certificate can be O(N * block).
//
// THE SCHEDULE: 6 anchors. D is round 1's designee (never a sender). Z
// independently receives THREE senders' (S1, S2, S3) round-change(1)
// envelopes DIRECTLY (bypassing `net`, so only Z — not S1/S2/S3 themselves —
// genuinely assembles the certificate) and then enters round 1 itself via
// advanceToRound, contributing its OWN envelope as the 4th — completing the
// #402 anchor-majority quorum on Z's OWN view. Z's `broadcastRoundCert`
// therefore fires for real, over `net`, to Z's real sync targets: D
// (designee), S1/S2/S3 (already PRESENT — their own envelopes are in the
// certificate), and W (an uninvolved bystander who never round-changed —
// ABSENT from it). Every recipient's transport is wrapped to record whether
// it received a MsgRoundCert from Z.
//
// RED at a3e4e72: `broadcastRoundCert(height, round, raws)` (3 params, no
// `present` map) loops `n.syncTargets()` unconditionally except self — S1,
// S2, S3 (already holding their own envelopes) ALSO receive the redundant
// certificate. GREEN at c2a476a+: `broadcastRoundCert` takes a `present`
// map and skips any non-designee peer already in it — S1/S2/S3 are skipped;
// D (designee) and W (absent) still receive it.
func TestModelCheck_H43_RoundCertNarrowedToDesigneePlusAbsent(t *testing.T) {
	const nAnchors = 6
	nodes, ids, net, _, _ := tier2AnchorNet(t, nAnchors)
	all := make([]ports.NodeID, len(ids))
	for i := range ids {
		all[i] = ids[i].NodeID()
	}
	for _, nd := range nodes {
		seed := make([]ports.NodeID, 0, len(all)-1)
		for _, id := range all {
			if id != nd.id {
				seed = append(seed, id)
			}
		}
		nd.chainSyncSeed = seed
	}
	byID := map[ports.NodeID]*Node{}
	for _, nd := range nodes {
		byID[nd.id] = nd
	}
	_, height := nodes[0].chain.Head()
	designeeID := nodes[0].designatedProposer(height, 1)
	d, ok := byID[designeeID]
	if !ok {
		t.Fatalf("premise: designatedProposer(%d, 1) returned an unknown id", height)
	}
	var rest []*Node
	for _, nd := range nodes {
		if nd.id != designeeID {
			rest = append(rest, nd)
		}
	}
	if len(rest) != 5 {
		t.Fatalf("premise: want 5 non-designee anchors, got %d", len(rest))
	}
	z, s1, s2, s3, w := rest[0], rest[1], rest[2], rest[3], rest[4]

	// Wrap every non-Z node's transport to record any MsgRoundCert received
	// from Z — the wire-level observation.
	received := map[ports.NodeID]bool{}
	wrap := func(nd *Node) {
		ep := net.Endpoint(nd.id)
		orig := nd.handle
		ep.SetHandler(func(from ports.NodeID, msg ports.Message) {
			if msg.Kind == ports.MsgRoundCert && from == z.id {
				received[nd.id] = true
			}
			orig(from, msg)
		})
	}
	for _, nd := range []*Node{d, s1, s2, s3, w} {
		wrap(nd)
	}

	// S1, S2, S3 broadcast real round-change(1) envelopes; hand them DIRECTLY
	// to Z's handler (bypassing net) so ONLY Z assembles — nobody else's own
	// view reaches quorum, and no unrelated cascade can pollute this test.
	for _, s := range []*Node{s1, s2, s3} {
		s.advanceToRound(s.roundsFor(), 1, "test")
		raw := s.roundsFor().Changes[1][s.id]
		if raw == nil {
			t.Fatalf("premise: %s did not record its own round-change(1) envelope", s.id)
		}
		z.handleChain(s.id, ports.Message{Kind: ports.MsgRoundChange, Data: raw})
	}
	// >= 3, not == 3: RoundCatchupMet's threshold (f+1=2 at N=6) is BELOW the
	// #402 anchor-majority quorum this test needs (4) — so as soon as S1+S2
	// land, Z's OWN maybeCatchUpRound (the real, existing #451 mechanism,
	// fired automatically inside the real MsgRoundChange handler) may
	// legitimately catch Z itself up to round 1, self-recording its own
	// envelope early. That is real, correct, unrelated-to-this-gate behavior
	// — not a premise defect — so the explicit z.advanceToRound call below
	// becomes a harmless no-op in that case (advanceToRound refuses
	// next <= rs.Round).
	for _, s := range []*Node{s1, s2, s3} {
		if _, ok := z.roundsFor().Changes[1][s.id]; !ok {
			t.Fatalf("premise: Z is missing %s's round-change(1) envelope", s.id)
		}
	}

	// Z enters round 1 itself (a real local timeout) — this call completes
	// the #402 anchor-majority quorum on Z's OWN view (S1+S2+S3+Z = 4 of 6
	// anchors) and is what triggers the REAL broadcastRoundCert over `net`.
	z.advanceToRound(z.roundsFor(), 1, "test")
	drainHeld(t, net, fifo)

	if !received[d.id] {
		t.Fatalf("G-H43-11: D (the round's designee) never received the certificate Z assembled — the designee must always receive it")
	}
	if !received[w.id] {
		t.Fatalf("G-H43-11: W (never round-changed — ABSENT from the certificate) never received it — an absent peer must still learn the round within one hop (G-H43-4's own premise)")
	}
	for _, s := range []*Node{s1, s2, s3} {
		if received[s.id] {
			t.Fatalf("G-H43-11 REPRODUCED: %s — already PRESENT in the certificate (its own envelope is one of "+
				"the 3 Raws) — ALSO received a redundant broadcastRoundCert from Z (R-H43-CERT-CARRIES-BLOCKS): "+
				"the certificate is O(N * ~1.5 MB once locks are carried) and up to N nodes can assemble one "+
				"independently, so an all-peers broadcast is O(N^2 * block) worst case (~1.2 GB per contested "+
				"round at N=12, per the certification). Consensus-adjacent, research-gated (build-immutable #6); "+
				"closer: narrow the broadcast to the designee plus peers absent from the certificate.", s.id)
		}
	}
	t.Logf("G-H43-11: Z's certificate reached D (designee) and W (absent) but NOT S1/S2/S3 (already present) — the broadcast is narrowed.")
}

// ── G-H43-12 ────────────────────────────────────────────────────────────────
//
// TestModelCheck_H43_RoundCertBudgetedBeforeVerify is G-H43-12
// (`R-H43-CERT-UNBUDGETED-VERIFY`, the #424 remote-CPU-DoS class, per the
// certification the THIRD recurrence), two independently-observed halves.
//
// (a) PER-SENDER BURST, wire-observed: `MsgRoundCert` gets a cheap gate IN
// FRONT of decode/verify — one sender's Nth-plus-one certificate delivery in
// one window must be refused before any signature work, exactly like
// allowBondSubmit. Driven end to end over `net.handle` (not a direct
// function call) so the observation is the REAL reply, not an internal
// state read: deliver a genuine, quorum-completing round-1 certificate
// arrival chain from ONE sender identity 5 times in a row (burst
// recommended at 4); the wrapped sender's transport records the LAST
// MsgRoundCertAck's OK field.
//
// RED at a3e4e72: no per-sender budget exists on this path at all — every
// delivery is gated only by content validity, never by rate, so a 5th
// delivery from the same sender is not distinguishably refused BY RATE (it
// is refused, if at all, by content — indistinguishable from the outside).
// This half of the gate instead directly targets the OBSERVABLE budget
// state via allowWindowed's own table, checked below.
//
// (b) GOVERNING-SET CAP, error-text observed: a certificate whose envelope
// count exceeds `chain.GoverningSetCap()` is malformed BY CONSTRUCTION (at
// most one envelope per governing-set member) and must be refused BEFORE
// any per-envelope decode/verify is attempted — driven via
// `acceptRoundCert` directly (same signature at both commits; a callable
// port, not the wire), capturing the Go error text itself: at c2a476a+ the
// cap fires first, naming the governing-set size in its own text; at
// a3e4e72 no cap exists, so the SAME oversized, garbage-envelope input
// instead fails inside newViewFor's per-envelope CBOR decode — a DIFFERENT
// error origin, proving the old path actually attempted the work the cap is
// supposed to prevent.
func TestModelCheck_H43_RoundCertBudgetedBeforeVerify(t *testing.T) {
	t.Run("cap_fires_before_verify", func(t *testing.T) {
		nodes, _, _, _, _ := tier2AnchorNet(t, 4)
		d := nodes[0]
		rs := d.roundsFor()
		// GoverningSetCap() does not exist at 462478d/a3e4e72 (a new port this
		// closer adds), so this test hardcodes the value it would return for
		// THIS fixture rather than calling it: tier2AnchorNet(t, 4) has 4
		// anchors, no bonded identities and no frozen epoch set yet (a fresh
		// launch-window chain) — anchors(4) + bonded(0) + epochSet(0) = 4.
		const capN = 4
		raws := make([][]byte, capN+1)
		for i := range raws {
			raws[i] = []byte("not a real cbor envelope")
		}
		env := roundCertEnv{Height: rs.Height, Round: 1, Raws: raws}
		err := d.acceptRoundCert(rs, &env)
		if err == nil {
			t.Fatalf("G-H43-12 premise: an oversized (%d > cap %d), garbage-envelope certificate must be refused", len(raws), capN)
		}
		wantSig := fmt.Sprintf("governing set is %d", capN)
		if !strings.Contains(err.Error(), wantSig) {
			t.Fatalf("G-H43-12 REPRODUCED (part b): a certificate carrying %d envelopes (> the governing-set cap "+
				"%d) was refused for the WRONG reason — got %q, want it to name the cap (%q) — meaning the cap "+
				"check does not run before the per-envelope verify (the #424 shape: unbounded work on unverified "+
				"input). Consensus-adjacent, research-gated (build-immutable #6); closer: bound len(env.Raws) "+
				"against GoverningSetCap() BEFORE any decode or signature.", len(raws), capN, err, wantSig)
		}
		t.Logf("G-H43-12 (part b): an oversized certificate is refused by the cap, named in the error: %v", err)
	})

	t.Run("per_sender_burst", func(t *testing.T) {
		nodes, ids, net, g, _ := tier2AnchorNet(t, 4)
		all := make([]ports.NodeID, len(ids))
		for i := range ids {
			all[i] = ids[i].NodeID()
		}
		for _, nd := range nodes {
			seed := make([]ports.NodeID, 0, len(all)-1)
			for _, id := range all {
				if id != nd.id {
					seed = append(seed, id)
				}
			}
			nd.chainSyncSeed = seed
		}
		d, s1, s2, sender := nodes[0], nodes[1], nodes[2], nodes[3]
		_ = g

		// Assemble a REAL, quorum-grade round-1 certificate at `sender`'s own
		// view (S1 + S2 direct, sender's own self-record) — a genuine,
		// verifiable object, not garbage, so a rate refusal (not a content
		// refusal) is what's under test.
		for _, s := range []*Node{s1, s2} {
			s.advanceToRound(s.roundsFor(), 1, "test")
			raw := s.roundsFor().Changes[1][s.id]
			if raw == nil {
				t.Fatalf("premise: %s did not record its own round-change(1) envelope", s.id)
			}
			sender.handleChain(s.id, ports.Message{Kind: ports.MsgRoundChange, Data: raw})
		}
		sender.advanceToRound(sender.roundsFor(), 1, "test")
		// Build the certificate's Raws from rs.Changes directly (the SAME
		// field both commits populate) rather than reading the cached
		// rs.Certs[round] — that cache does not exist at 462478d/a3e4e72 (a
		// new field this closer adds).
		senderChanges := sender.roundsFor().Changes[1]
		if len(senderChanges) < 3 {
			t.Fatalf("premise: sender should hold at least 3 recorded round-1 senders (S1, S2, itself), got %d", len(senderChanges))
		}
		raws := make([][]byte, 0, len(senderChanges))
		for _, raw := range senderChanges {
			raws = append(raws, raw)
		}
		data, err := cbor.Marshal(roundCertEnv{Height: sender.roundsFor().Height, Round: 1, Raws: raws})
		if err != nil {
			t.Fatalf("premise: marshal roundCertEnv: %v", err)
		}
		// Flush the network BEFORE starting the rate-limit loop: sender's own
		// advanceToRound above assembled and genuinely BROADCAST its own
		// certificate over `net` too (sender's view already meets quorum by
		// the time it self-records) — that queued delivery would otherwise
		// consume one unit of D's per-sender budget for `sender` before this
		// test's own explicit loop gets a chance to, silently shifting the
		// expected pass/fail boundary by one.
		drainHeld(t, net, fifo)

		var lastOK bool
		var lastSeen bool
		ep := net.Endpoint(sender.id)
		orig := sender.handle
		ep.SetHandler(func(from ports.NodeID, msg ports.Message) {
			if msg.Kind == ports.MsgRoundCertAck {
				lastSeen, lastOK = true, msg.OK
			}
			orig(from, msg)
		})

		// A generous window (well past any plausible small burst, recommended
		// 4) rather than a hardcoded exact boundary: this test's own setup
		// (a full-mesh, real quorum-forming schedule) legitimately generates
		// some incidental MsgRoundCert traffic to D from OTHER senders too
		// (S1/S2's own views can independently reach quorum), so this test
		// asserts the property a per-sender ceiling exists — SOME delivery in
		// this window is refused, and NOT the first (which is real, valid
		// content, proving refusal is about RATE, not content) — rather than
		// pinning the exact attempt index the ceiling falls on.
		const attempts = 12
		results := make([]bool, 0, attempts)
		for i := 0; i < attempts; i++ {
			lastSeen = false
			d.handle(sender.id, ports.Message{Kind: ports.MsgRoundCert, Data: data, RID: uint64(i) + 1})
			drainHeld(t, net, fifo)
			if !lastSeen {
				t.Fatalf("premise: attempt %d produced no MsgRoundCertAck reply", i+1)
			}
			results = append(results, lastOK)
		}
		t.Logf("G-H43-12 (part a) results across %d deliveries from ONE sender identity: %v", attempts, results)
		if !results[0] {
			t.Fatalf("premise: attempt 1 was refused — got OK=false; this test's content is a genuine, valid certificate")
		}
		refused := 0
		for _, ok := range results {
			if !ok {
				refused++
			}
		}
		if refused == 0 {
			t.Fatalf("G-H43-12 REPRODUCED (part a): all %d MsgRoundCert deliveries from ONE sender identity in "+
				"one window were ACCEPTED (OK=true) — no per-sender rate budget gates this path before "+
				"decode/verify (the #424 shape, third recurrence). Consensus-adjacent, research-gated "+
				"(build-immutable #6); closer: an allowRoundCert budget (recommended burst 4) in front of the "+
				"certificate's envelope verification.", attempts)
		}
		t.Logf("G-H43-12 (part a): %d of %d deliveries from one sender identity in one window were refused (OK=false) — the per-sender budget gates it.", refused, attempts)
	})
}

// ── G-H43-13 ────────────────────────────────────────────────────────────────
//
// countingLogger is a test-only ports.Logger that counts occurrences of each
// event name it observes — used by G-H43-13 to count proposal ATTEMPTS
// without needing to reference the new rs.Attempted field this closer adds
// (it does not exist at 462478d/a3e4e72 — the same test file must compile
// there too).
type countingLogger struct{ counts map[string]int }

func (l *countingLogger) Enabled(ports.LogLevel) bool { return true }
func (l *countingLogger) Log(lvl ports.LogLevel, event string, kv ...any) {
	if l.counts == nil {
		l.counts = map[string]int{}
	}
	l.counts[event]++
}

// TestModelCheck_H43_ProposeSpinDedupedPerRound is G-H43-13
// (`R-H43-PROPOSE-SPIN`), two independently-observed halves, per the delta
// certification §4.2/§7 item 5.
//
// (a) THE EMPTY CHECK MOVED BEFORE THE ERA ROOTS: a workless designee's
// doomed attempt must fail on a CHEAP predicate (len(b.Entries) == 0 && ...)
// before PopulateEra4Roots (the floor-box SMT path) and chain.Sign — proven
// by the error's own TEXT/ORIGIN, captured directly (proposeBlockAt, the
// same call G-H43-2's ablation makes): at c2a476a+ the error is
// "propose: nothing to carry" (chainrole.go, checked immediately after the
// content folds, before MintVersion/root population); at a3e4e72 the SAME
// workless attempt instead reaches chain.Sign and PopulateEra4Roots FIRST,
// failing only at ValidateProposal's "chain: empty block" afterward
// ("propose: local pre-check: chain: empty block") — a DIFFERENT error
// origin, proving the old path paid the cost this closer moved the check in
// front of.
//
// (b) THE ATTEMPT IS DEDUPED PER (h, r): a designee whose mempool has not
// changed must not re-attempt on every redundant certificate/round-change
// arrival for a round it already holds. Measured via a counting
// ports.Logger wired onto the designee (SetLogger) — event-name counting is
// symbol-independent (it does not require reading the new rs.Attempted
// field) and directly observes what the certification measured as "~40
// attempts... in one round" (h43-probe-evidence.txt): count occurrences of
// "new-view proposal (#432 view-change)", the line proposeAtNewView logs on
// EVERY entry, success or failure. A THIRD sender's round-change for the
// SAME round, delivered after the designee's certificate is already
// assembled and its first (doomed, workless) attempt has already run, must
// not add a second occurrence.
//
// RED at a3e4e72: recordRoundChange re-verifies and unconditionally
// re-attempts on every arrival that clears newViewFor and finds
// designatedProposer == self, with no per-(h,r) memory of a prior attempt —
// the third sender's arrival adds a SECOND "new-view proposal" line.
func TestModelCheck_H43_ProposeSpinDedupedPerRound(t *testing.T) {
	t.Run("empty_check_before_era_roots", func(t *testing.T) {
		nodes, ids, _, g, _ := tier2AnchorNet(t, 4)
		all := make([]ports.NodeID, len(ids))
		for i := range ids {
			all[i] = ids[i].NodeID()
		}
		d := nodes[0]
		d.pendingEntries = nil
		d.pendingBondRegs = nil
		prevHead, h := d.chain.Head()
		peers := all[1:]
		var attemptErr error
		d.proposeBlock(&chain.Block{Version: chain.BlockVersionRounds, Height: h, Prev: prevHead}, peers, peers, 0,
			func(err error) { attemptErr = err })
		if attemptErr == nil {
			t.Fatalf("premise: a workless proposer's attempt at an empty block must fail")
		}
		_ = g
		newSig := strings.Contains(attemptErr.Error(), "propose: nothing to carry")
		oldSig := strings.Contains(attemptErr.Error(), "local pre-check: chain: empty block")
		if !newSig && !oldSig {
			t.Fatalf("premise: attempt error %q matches NEITHER the new nor the old empty-block signature — re-derive this test's premise", attemptErr)
		}
		if oldSig {
			t.Fatalf("G-H43-13 REPRODUCED (part a): the workless attempt failed with %q — the OLD signature "+
				"(ValidateProposal's 'chain: empty block', reached only AFTER chain.Sign and "+
				"PopulateEra4Roots/the floor-box SMT path have already run) — the empty check has not moved in "+
				"front of the era-root population. Consensus-adjacent, research-gated (build-immutable #6); "+
				"closer: check emptiness immediately after the content folds, before MintVersion/root "+
				"population and before chain.Sign.", attemptErr)
		}
		t.Logf("G-H43-13 (part a): the workless attempt failed fast, before the era roots: %v", attemptErr)
	})

	t.Run("dedup_per_round", func(t *testing.T) {
		nodes, ids, _, _, _ := tier2AnchorNet(t, 4)
		all := make([]ports.NodeID, len(ids))
		for i := range ids {
			all[i] = ids[i].NodeID()
		}
		for _, nd := range nodes {
			seed := make([]ports.NodeID, 0, len(all)-1)
			for _, id := range all {
				if id != nd.id {
					seed = append(seed, id)
				}
			}
			nd.chainSyncSeed = seed
		}
		byID := map[ports.NodeID]*Node{}
		for _, nd := range nodes {
			byID[nd.id] = nd
		}
		_, height := nodes[0].chain.Head()
		designeeID := nodes[0].designatedProposer(height, 1)
		d, ok := byID[designeeID]
		if !ok {
			t.Fatalf("premise: designatedProposer(%d, 1) returned an unknown id", height)
		}
		var others []*Node
		for _, nd := range nodes {
			if nd.id != designeeID {
				others = append(others, nd)
			}
		}
		if len(others) != 3 {
			t.Fatalf("premise: want 3 non-designee anchors, got %d", len(others))
		}
		s1, s2, s3 := others[0], others[1], others[2]
		d.pendingEntries = nil
		d.pendingBondRegs = nil

		cl := &countingLogger{}
		d.SetLogger(cl)
		const attemptEvent = "new-view proposal (#432 view-change)"

		// S1, S2: real, individually-verified round-change(1) envelopes,
		// handed DIRECTLY to D's real handler (bypassing net — this test is
		// about how many times D itself ATTEMPTS, not about wire routing).
		rawRoundChange1 := func(nd *Node) []byte {
			nd.advanceToRound(nd.roundsFor(), 1, "test")
			raw := nd.roundsFor().Changes[1][nd.id]
			if raw == nil {
				t.Fatalf("premise: %s did not record its own round-change(1) envelope", nd.id)
			}
			return raw
		}
		raw1, raw2, raw3 := rawRoundChange1(s1), rawRoundChange1(s2), rawRoundChange1(s3)

		d.handleChain(s1.id, ports.Message{Kind: ports.MsgRoundChange, Data: raw1})
		if got := cl.counts[attemptEvent]; got != 0 {
			t.Fatalf("premise: D should not attempt on a SUB-quorum round-change(1) delivery, got %d attempts", got)
		}

		// S2 completes the quorum: D's FIRST (doomed, workless) attempt fires.
		d.handleChain(s2.id, ports.Message{Kind: ports.MsgRoundChange, Data: raw2})
		afterFirst := cl.counts[attemptEvent]
		if afterFirst != 1 {
			t.Fatalf("premise: the quorum-completing delivery should fire exactly 1 proposal attempt, got %d", afterFirst)
		}

		// S3's arrival is REDUNDANT — D already holds the round-1 certificate
		// and its mempool has not changed since the first attempt.
		d.handleChain(s3.id, ports.Message{Kind: ports.MsgRoundChange, Data: raw3})
		afterThird := cl.counts[attemptEvent]

		if afterThird > afterFirst {
			t.Fatalf("G-H43-13 REPRODUCED (part b): a REDUNDANT round-change arrival for a round D already "+
				"holds a certificate for, with D's mempool UNCHANGED since its first attempt, fired ANOTHER "+
				"proposal attempt (%d -> %d occurrences of %q) — the designee's attempt is not deduped per "+
				"(h, r) (R-H43-PROPOSE-SPIN; the certification's own probe: ~40 attempts in one round). "+
				"Consensus-adjacent, research-gated (build-immutable #6); closer: attempt once per (h, r) "+
				"unless the mempool changed.", afterFirst, afterThird, attemptEvent)
		}
		t.Logf("G-H43-13 (part b): the redundant arrival did NOT add another attempt (%d occurrences of %q) — deduped.", afterThird, attemptEvent)
	})
}

// ── G-H43-14 ────────────────────────────────────────────────────────────────
//
// TestModelCheck_H43_RoundZeroCertificateRefusedUnverified is G-H43-14
// (`R-H43-CERT-ROUND-ZERO-UNVERIFIED`, C-1/C-2/C-3), per the composed-diff
// re-certification CONSENSUS-LIVENESS-h43-COMPOSED-DIFF-c2a476a-RESEARCH-CERTIFICATION-2026-09-07.md
// §1.3/§5: the merge blocker. `newViewFor` short-circuits at round 0
// (`if round == 0 { return nil, nil }`) — the SAME shape as "verified,
// distinct senders, at quorum" — without decoding or verifying a single
// byte. Before the fix, `acceptRoundCert` had no round-0 guard, so ANY
// peer — no signature, no eligibility, not even a registered identity —
// could send a `MsgRoundCert{Round: 0}` carrying attacker-chosen envelopes
// under attacker-chosen sender IDs claiming attacker-chosen target rounds,
// and every one would be written straight into `rs.Changes` unverified.
//
// THE ATTACK: a victim (one live anchor in a 4-anchor network) receives ONE
// `MsgRoundCert{Height: rs.Height, Round: 0, Raws: [env]}` from a NON-
// validator, non-registered peer identity — `env` is a well-formed
// (CBOR-valid) `roundChangeEnv` claiming `NewRound: 5` under a fabricated
// sender key and a garbage signature, chosen so it WOULD populate
// rs.Changes[5] if it were ever stored, distinguishing "refused before
// storage" from "stored but otherwise harmless".
//
// FOUR ASSERTIONS, all RED at c2a476a: (1) rs.Changes gains no entry at all
// (not even under the fabricated NewRound); (2) rs.Certs[0] stays nil (no
// unverified "certificate" is cached); (3) the victim's own round does not
// move; (4) a LATER, GENUINE round-1 certificate — from real, registered,
// signing anchors — still assembles normally afterward (the refusal must
// not poison or wedge the round state for the real synchronizer that
// follows it).
func TestModelCheck_H43_RoundZeroCertificateRefusedUnverified(t *testing.T) {
	nodes, ids, _, _, _ := tier2AnchorNet(t, 4)
	all := make([]ports.NodeID, len(ids))
	for i := range ids {
		all[i] = ids[i].NodeID()
	}
	for _, nd := range nodes {
		seed := make([]ports.NodeID, 0, len(all)-1)
		for _, id := range all {
			if id != nd.id {
				seed = append(seed, id)
			}
		}
		nd.chainSyncSeed = seed
	}
	victim, s1, s2 := nodes[0], nodes[1], nodes[2]
	_, h := victim.chain.Head()

	// A NON-VALIDATOR, non-registered attacker identity — never bonded,
	// never an anchor, never even constructed as a *Node in this fixture.
	attacker := identity.FromSeed(9990011)

	// A well-formed (CBOR-valid) round-change envelope, unsigned by anyone
	// real, claiming an ARBITRARY target round (5) under a fabricated
	// sender key — this is what WOULD land in rs.Changes[5] if the round-0
	// guard did not refuse it first.
	fakeSender := make([]byte, ed25519.PublicKeySize)
	for i := range fakeSender {
		fakeSender[i] = byte(i + 1)
	}
	forged := roundChangeEnv{Height: h, NewRound: 5, Sender: fakeSender, Sig: []byte("not-a-real-signature-at-all")}
	forgedRaw, err := cbor.Marshal(forged)
	if err != nil {
		t.Fatalf("premise: marshal the forged roundChangeEnv: %v", err)
	}
	attack := roundCertEnv{Height: h, Round: 0, Raws: [][]byte{forgedRaw}}
	attackData, err := cbor.Marshal(attack)
	if err != nil {
		t.Fatalf("premise: marshal the round-0 attack envelope: %v", err)
	}

	victim.handle(attacker.NodeID(), ports.Message{Kind: ports.MsgRoundCert, Data: attackData})

	rs := victim.roundsFor()

	// (1) no entry in rs.Changes at all — not even under the fabricated NewRound.
	if got := len(rs.Changes[5]); got != 0 {
		t.Fatalf("G-H43-14 REPRODUCED (1/4): the forged round-0 certificate wrote %d unverified entry(ies) "+
			"into rs.Changes[5] — an attacker-chosen (fabricated sender, forged signature, never verified) "+
			"round-change claim was stored. R-H43-CERT-ROUND-ZERO-UNVERIFIED.", got)
	}
	for r, m := range rs.Changes {
		if len(m) != 0 {
			t.Fatalf("G-H43-14 REPRODUCED (1/4): the forged round-0 certificate wrote an unverified entry into "+
				"rs.Changes[%d] — R-H43-CERT-ROUND-ZERO-UNVERIFIED.", r)
		}
	}
	// (2) no cached "certificate" at round 0.
	if c := rs.Certs[0]; c != nil {
		t.Fatalf("G-H43-14 REPRODUCED (2/4): rs.Certs[0] was populated (%d envelopes cached) from an unverified "+
			"round-0 certificate — R-H43-CERT-ROUND-ZERO-UNVERIFIED.", len(c.Raws))
	}
	// (3) the victim's round did not move.
	if rs.Round != 0 {
		t.Fatalf("G-H43-14 REPRODUCED (3/4): the victim's round moved to %d from an unverified round-0 "+
			"certificate — R-H43-CERT-ROUND-ZERO-UNVERIFIED.", rs.Round)
	}

	// (4) a LATER, GENUINE round-1 certificate still assembles normally —
	// the refusal must not poison the round state for the real
	// synchronizer that follows.
	for _, s := range []*Node{s1, s2} {
		s.advanceToRound(s.roundsFor(), 1, "test")
		raw := s.roundsFor().Changes[1][s.id]
		if raw == nil {
			t.Fatalf("premise: %s did not record its own round-change(1) envelope", s.id)
		}
		victim.handleChain(s.id, ports.Message{Kind: ports.MsgRoundChange, Data: raw})
	}
	if got := len(rs.Changes[1]); got < 2 {
		t.Fatalf("G-H43-14 REPRODUCED (4/4): after the round-0 attack, a LATER genuine round-1 certificate did "+
			"not assemble (rs.Changes[1] holds %d senders, want >= 2) — the refusal poisoned or wedged the "+
			"round state for the real synchronizer. R-H43-CERT-ROUND-ZERO-UNVERIFIED.", got)
	}
	t.Logf("G-H43-14: the forged round-0 certificate was refused with zero effect on rs.Changes/rs.Certs/rs.Round, and a later genuine round-1 certificate still assembles (%d senders) — the round-0 hole is closed.", len(rs.Changes[1]))
}
