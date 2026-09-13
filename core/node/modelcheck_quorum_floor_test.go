package node

import (
	"strings"
	"testing"

	"github.com/nerolabs/silt/adapters/identity"
	"github.com/nerolabs/silt/adapters/markstore"
	"github.com/nerolabs/silt/adapters/memstore"
	"github.com/nerolabs/silt/adapters/simclock"
	"github.com/nerolabs/silt/adapters/simnet"
	"github.com/nerolabs/silt/core/chain"
	"github.com/nerolabs/silt/core/credit"
	"github.com/nerolabs/silt/ports"
)

// ──, the new-view face ──────────────────────────────────────────────
//
// TestModelCheck_DivergentQuorumFloorMustNotBlockNewViewCertificate is, per §5.2 / the row (line 304): "In a mature epoch with heterogeneous
// local -quorum, a node with the higher floor can still assemble a new-view certificate." §5.2 spells out WHY the row's own wording reads as a
// requirement, not a description: "A node with a higher local floor can therefore never assemble a view-change certificate the rest of the
// network considers valid — so as designee it is a permanently dead round at every height, and as attester it refuses proposals the network
// accepts." The direction, “ (20): "in objective mode ValidateCommit ignores the local Config.Quorum floor and defers to bftThreshold; Quorum
// stays a proposer-side gather target only. A separate GATED item (I1) behind." This test asserts the healthy property that direction implies
// for the view-change path specifically: a divergent local -quorum floor must not stop a node from assembling a certificate its peers already
// treat as quorum.
//
// THIS IS ARM 8b of the follow-up predicate research “ §5: "Drive newViewFor at a
// round > 0 with exactly bftThreshold round-changes on a node whose cfg.Quorum
// exceeds it; assert a certificate validates. Ablation arm: the old max ⇒ 'new-view
// certificate below quorum'. NEEDED, sufficient as stated." bftThreshold(4) = 2
// (chain.go) is exactly the 2 round-changes this fixture delivers; the
// verdict is this arm needs no fixture change, only this cross-reference.
//
// THE MECHANISM (verified against this branch at the file:line cited):
// `SupportMeetsQuorum` (chain.go) tests `len(seen) < c.RequiredQuorum`
// (chain.go) using the CALLING chain's OWN `c.cfg.Quorum`.
// `RequiredQuorum` (chain.go) returns `q:= c.cfg.Quorum` unmodified in
// the MATURE regime (chain.go, "the weight rule carries the
// Byzantine bar") — the case the row names. `newViewFor` (rounds.go)
// calls `SupportMeetsQuorum` at rounds.go to admit a round-change quorum,
// erroring "new-view certificate below quorum (%d round-changes)" at
// rounds.go when it does not clear the bar. `checkRoundQuorum`
// (rounds.go) treats that error as "wait for more" (rounds.go)
// and returns silently — so a designee whose OWN floor exceeds what its peers
// will ever independently produce can NEVER cache a certificate, NEVER call
// `fireDesignee`, and NEVER propose, at any round, for the rest of the
// height's life. That is the "permanently dead round".
//
// THE REGIME THIS FIXTURE REACHES: young/launch-phase, NOT the mature epoch the
// row names. Reaching `c.matureEpoch == true` needs an epoch boundary
// (`EpochBlocks` > 0, the `everMature` latch tripped, then a rotation) — the
// shape `matureWorld12` in modelcheck_locked_stall_test.go builds, at the cost
// of a 12-member cohort driven through 8 proposal rounds. That plumbing is not
// reused here (new plumbing this file does not have standalone); THE
// MATURE-EPOCH ARM OF IS OWED. What this fixture DOES reach is
// control-flow-identical for the property under test: `RequiredQuorum` returns
// `q = c.cfg.Quorum` VERBATIM whenever a node's own floor already exceeds
// `bftThreshold(N)` (chain.go, the young-window Byzantine-escalation
// branch — `if bq:= bftThreshold(.); bq > q { q = bq }` never raises `q` when
// `q` already leads), which is the exact same returned value the mature
// early-return produces (chain.go) — the LINE that runs differs, the
// OBSERVABLE RESULT (the local floor governs, unmodified) does not. With
// `nAnchors = 4`, `bftThreshold(4) = 4 - ⌊3/3⌋ - 1 = 2` (chain.go); a
// designee configured at `Quorum: 3` therefore has `RequiredQuorum = 3` while
// its two peers, at the fixture's default `Quorum: 1`, have `RequiredQuorum =
// max(1, 2) = 2` — the exact divergence the row names, reached without touching
// epoch machinery.
//
// THE FOURTH ANCHOR is live but SILENT on round-changes: it never advances a
// round on its own (nothing here drives its sweep), so exactly 2 non-proposer
// round-change(1) envelopes reach the designee before it fires — and 2 is
// EXACTLY what the peers' own floor (`RequiredQuorum = 2`) already calls
// quorum, and (with the proposer itself counted as an anchor) EXACTLY enough
// anchors to also clear the majority (2 non-proposer + 1 anchor-proposer = 3
// of 4, chain.go) — so the ONLY thing standing between this
// certificate and acceptance is the designee's OWN raised floor. It IS in the
// designee's sync targets, so it is asked to ATTEST: the designee's proposal
// gathers its own operator floor (`Quorum: 3`, the "proposer-side gather
// target", raised in proposeBlockAt on every path — arm 8e), which three live
// peers can satisfy. Without it the certificate validates but the designee's
// own gather target (3) is unsatisfiable by two peers — (-quorum above the
// live peer count is the operator's call, not a validity fact), not this
// arm's subject. The round-cert broadcast may reach the fourth anchor after
// the designee has fired; a late third envelope changes nothing the
// assertions below read.
//
// RED at HEAD (both assertions): (1) `newViewFor` called directly, with
// EXACTLY the 2 envelopes the network already considers quorum, returns the
// error above rather than nil. (2) delivered through the real
// `MsgRoundChange` handler (the natural trigger), the height never commits —
// no certificate is ever cached, so `fireDesignee` never runs. (the
// M2 fix, `chainrole.go`, `proposeAtNewView`'s forced==nil leg
// already passes the certificate's round through rather than re-deriving it)
// is SHIPPED on this branch and is not what (2) is pinning — it never gets
// the chance to run, because `checkRoundQuorum` returns before ever calling
// `fireDesignee`.
func TestModelCheck_DivergentQuorumFloorMustNotBlockNewViewCertificate(t *testing.T) {
	const nAnchors = 4
	sched := simclock.New()
	net := simnet.New(sched, 1, simnet.DefaultConfig())
	net.EnableHeldDelivery()

	ids := make([]*identity.Identity, nAnchors)
	anchors := map[ports.NodeID]bool{}
	for i := range ids {
		ids[i] = identity.FromSeed(int64(8600 + i))
		anchors[ids[i].NodeID()] = true
	}
	g := &chain.Block{Version: 1, Height: 0, Entries: []ports.Entry{mkEntry("g--8")}}
	chain.Sign(g, ids[0].Signer())

	baseCfg := chain.Config{Quorum: 1, MinBond: 1 << 20, ByzantineQuorum: true, Anchors: anchors, MatureValidators: 99}

	// Determine the round-1 designee from the anchor-set ordering alone
	// (EligibleProposers/designatedProposer, rounds.go, depend on
	// cfg.Anchors, never on cfg.Quorum) via a throwaway chain sharing the
	// same genesis and anchor set — so the REAL nodes below can each be
	// built ONCE, with the correct per-node Quorum from the start.
	probe := chain.New(baseCfg, func(ports.NodeID) int64 { return 0 })
	probe.SetBondVerifier(mcStubVerify)
	if err := probe.AppendGenesis(*g); err != nil {
		t.Fatalf("probe genesis: %v", err)
	}
	props := probe.EligibleProposers()
	if len(props) != nAnchors {
		t.Fatalf("premise: EligibleProposers = %d, want %d", len(props), nAnchors)
	}
	designeeID := props[int((uint64(1)+uint64(1))%uint64(len(props)))] // designatedProposer(height=1, round=1)
	designeeIdx := -1
	for i, id := range ids {
		if id.NodeID() == designeeID {
			designeeIdx = i
		}
	}
	if designeeIdx == -1 {
		t.Fatalf("premise: designee id not found among the 4 configured anchors")
	}
	var liverIdx []int
	for i := range ids {
		if i != designeeIdx {
			liverIdx = append(liverIdx, i)
		}
	}
	if len(liverIdx) != 3 {
		t.Fatalf("premise: want 3 non-designee anchor slots, got %d", len(liverIdx))
	}
	// Only the first 2 non-designee anchors ever DECLARE round 1; the third
	// (liverIdx[2]) is live and attests, but is never driven to a round-change
	// (see the doc comment above).

	highCfg := baseCfg
	highCfg.Quorum = 3 // the divergent floor: RequiredQuorum = max(3, bftThreshold(4)=2) = 3 (chain.go)

	mk := func(id *identity.Identity, cfg chain.Config) *Node {
		nd := New(id.NodeID(), DefaultConfig(), sched, net.Endpoint(id.NodeID()), memstore.New())
		nd.SetLedger(credit.New(50_000, 0))
		ch := chain.New(cfg, func(ports.NodeID) int64 { return 0 })
		ch.SetBondVerifier(mcStubVerify)
		if err := ch.AppendGenesis(*g); err != nil {
			t.Fatalf("genesis: %v", err)
		}
		nd.EnableChain(ch, id.Signer())
		if err := nd.SetSignMarkStore(markstore.NewMem()); err != nil {
			t.Fatalf("sign-mark store: %v", err)
		}
		return nd
	}

	designee := mk(ids[designeeIdx], highCfg)
	liver1 := mk(ids[liverIdx[0]], baseCfg)
	liver2 := mk(ids[liverIdx[1]], baseCfg)
	silent := mk(ids[liverIdx[2]], baseCfg) // attests; never declares a round

	// Each live node's sync targets are the OTHER three live nodes.
	designee.chainSyncSeed = []ports.NodeID{liver1.id, liver2.id, silent.id}
	liver1.chainSyncSeed = []ports.NodeID{designee.id, liver2.id, silent.id}
	liver2.chainSyncSeed = []ports.NodeID{designee.id, liver1.id, silent.id}
	silent.chainSyncSeed = []ports.NodeID{designee.id, liver1.id, liver2.id}

	_, height := designee.chain.Head()
	if height != 1 {
		t.Fatalf("premise: want the working height to be 1 (right after genesis), got %d", height)
	}
	// The designee must hold WORK to carry: under a designee with
	// nothing to carry declines to propose an empty block ("propose:
	// nothing to carry", chainrole.go), which is a different, settled
	// refusal from the quorum floor this arm pins. One pending entry
	// is the minimum that makes the end-to-end half attributable.
	designee.pendingEntries = []pendingEntry{{E: mkEntry("g--8-work"), At: height}}
	if got := designee.designatedProposer(height, 1); got != designee.id {
		t.Fatalf("premise: designatedProposer(%d, 1) did not return the constructed high-floor node", height)
	}

	// Each liver independently produces a real, individually-verified
	// round-change(1) envelope — the same call maybeAdvanceRound's timeout
	// path makes (a simulated local timeout, not a network delivery) — per
	// the construction modelcheck_liveness_gates_test.go's uses.
	rawRoundChange1 := func(nd *Node) []byte {
		nd.advanceToRound(nd.roundsFor(), 1, "test")
		raw := nd.roundsFor().Changes[1][nd.id]
		if raw == nil {
			t.Fatalf("premise: %s did not record its own round-change(1) envelope", nd.id)
		}
		return raw
	}
	raw1, raw2 := rawRoundChange1(liver1), rawRoundChange1(liver2)

	// ── Mechanism pin
	// ──────────────────────────────────────────────────── Call
	// newViewFor directly with EXACTLY the 2 envelopes the network
	// already treats as quorum, bypassing the wire entirely, so a RED
	// here is attributed to the quorum-floor line and not to any fixture
	// defect (delivery order, attester eligibility, a dropped envelope).
	// RED-FIRST with "new-view certificate below quorum (2
	// round-changes)"; GREEN under direction (1). Ablation: restore
	// max(cfg.Quorum, bft) in RequiredQuorum's regime (a) ⇒ that error
	// returns here.
	if _, err := designee.newViewFor(height, 1, [][]byte{raw1, raw2}); err != nil {
		if strings.Contains(err.Error(), "new-view certificate below quorum") {
			t.Fatalf("REGRESSED (rounds.go newViewFor → SupportMeetsQuorum → RequiredQuorum): the designee's "+
				"raised local -quorum floor (%d) rejected a certificate carrying exactly the %d round-changes its peers "+
				"(floor %d) already consider quorum — the local floor is back inside the validity rule: %v",
				highCfg.Quorum, 2, baseCfg.Quorum, err)
		}
		t.Fatalf("newViewFor(2 round-changes) failed for a reason OTHER than the quorum floor (%v) — "+
			"a fixture defect, not ", err)
	}
	t.Logf("mechanism pin: newViewFor accepted the 2-round-change certificate at the high-floor designee")

	// ── Premise (re-derived for direction (1)), checked AFTER the
	// mechanism pin so an ablation reddens on the mechanism first: the
	// designee's local floor (3) exceeds the derived bar
	// (bftThreshold(4)=2), and RequiredQuorum IGNORES it — so the pass
	// above is attributable to the derived rule, not to a fixture whose
	// config already asked for 2.
	if got := designee.chain.RequiredQuorum(); got != 2 || highCfg.Quorum <= got {
		t.Fatalf("premise: the designee's RequiredQuorum() must be the derived bftThreshold(4)=2 with its local "+
			"cfg.Quorum (%d) strictly above it; got RequiredQuorum()=%d", highCfg.Quorum, got)
	}

	// ── End-to-end: the natural trigger must still commit the height
	// ────── Deliver the SAME 2 envelopes through the real MsgRoundChange
	// handler: recordRoundChange -> checkRoundQuorum -> newViewFor -> (on
	// success) fireDesignee -> proposeAtNewView -> the real gather. The
	// healthy property (20): a divergent local -quorum floor must not
	// stop a node from assembling a certificate the REST OF THE NETWORK
	// already accepts, and the height must commit.
	rs := designee.roundsFor() // captured BEFORE delivery: Head is a NEXT-height counter (chain.go),
	// So this pointer — not a fresh roundsFor call after a possible commit resets it — is what
	// makes the Certs[1] check below meaningful.

	designee.handleChain(liver1.id, ports.Message{Kind: ports.MsgRoundChange, Data: raw1})
	designee.handleChain(liver2.id, ports.Message{Kind: ports.MsgRoundChange, Data: raw2})
	drainHeld(t, net, fifo)

	if got := rs.Certs[1]; got == nil {
		t.Fatalf("the designee never cached a round-1 certificate from the 2 envelopes the direct " +
			"newViewFor call above accepted — checkRoundQuorum and newViewFor disagree; re-derive before trusting either")
	} else if len(got.Raws) < 2 {
		t.Fatalf("the cached round-1 certificate carries %d envelopes, want at least the 2 delivered", len(got.Raws))
	}
	_, h := designee.chain.Head()
	if h <= height {
		t.Fatalf("REGRESSED: height %d never committed — the designee's raised local -quorum "+
			"floor (%d) rejected a new-view certificate carrying exactly the %d round-changes its own "+
			"peers (floor %d) already consider quorum; SupportMeetsQuorum/newViewFor must ignore the LOCAL "+
			"floor per (20) — this designee is a permanently dead round at this height, "+
			"exactly as the rule §5.2 describes", h, highCfg.Quorum, 2, baseCfg.Quorum)
	}
	blk := designee.Chain().Blocks(1)[0]
	got := nonProposerAttCount(&blk)
	if got < highCfg.Quorum {
		t.Fatalf("the designee's round-1 block carries %d non-proposer attestations, below its own gather "+
			"target cfg.Quorum=%d — the proposer-side gather target did not survive on the new-view path (arm 8e)", got, highCfg.Quorum)
	}
	t.Logf("height %d committed at the high-floor designee's own round-1 new-view certificate, gathered to its own floor (%d attestations).", h, got)
}
