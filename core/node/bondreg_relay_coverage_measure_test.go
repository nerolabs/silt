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

// shippedBondTTL is the objective re-challenge cadence an untrusted validator gets
// when the operator sets none (cmd/silt DerivedBondTTL). Renewal falls due at half of
// it, so it is what sets how often a ~1.5 MB space-time proof has to cross the wire.
const shippedBondTTL = uint64(32)

// sweepSeconds is DefaultConfig().ChainSyncInterval in seconds — the renewal sweep's
// cadence, which is what turns a submit COUNT into a submit RATE.
const sweepSeconds = 30

// coverage is what one arm of the measurement below reports: the share of
// registration-bearing gather legs that crossed by digest, and — for those that did
// not — which evidence was missing.
type coverage struct {
	shed, carried              int
	ownUnacked, peerUnreported int
	head                       uint64
	// What the LINK was asked to carry, per message kind, over the whole run. The
	// coverage ratio says what share of legs shed; these say what that was worth and
	// what else was competing for the same bytes.
	// sweeps is the number of renewal sweeps every node ran, so the submit count can
	// be read as a RATE. One submit per peer per sweep is the design's cadence; more
	// than that would mean copies stacking on a connection the previous copy is still
	// crossing, which is a different defect with a different remedy.
	sweeps, peersPer          int
	rate                      int64
	submits, submitBytes      int64
	proposals, proposalBytes  int64
	prepareQCs, prepareQCByte int64
	// What ARRIVED, against what was offered above. A submit that is still crossing
	// when the run ends was never a delivery, and the difference is what separates
	// "the peer holds no bytes to report" from "the peer holds them and the receipt
	// was lost" — the same coverage figure, two different remedies.
	submitsArrived int64
	// And what each end made of them: how many the receivers QUEUED, and how many
	// acknowledgements the senders never saw return.
	held, acksLost             int
	probesAnswered, probesLost int
}

func (c coverage) legs() int { return c.shed + c.carried }

func (c coverage) pct() float64 {
	if c.legs() == 0 {
		return 0
	}
	return 100 * float64(c.shed) / float64(c.legs())
}

// driveCoverage runs four bonded validators on the REAL daemon wiring — each on its
// own periodic chain-sync sweep, minting real space-time proofs, broadcasting real
// renewals and proposing real blocks — over a wire with the supplied characteristics,
// and reports the digest relay's coverage across the whole run.
func driveCoverage(t *testing.T, netcfg simnet.Config, intervals int, ttl uint64) coverage {
	t.Helper()
	const (
		bondSize = int64(2) << 20
		N        = 4
	)
	sched := simclock.New()
	net := simnet.New(sched, 17, netcfg)

	ids := make([]*identity.Identity, N)
	anchors := map[ports.NodeID]bool{}
	peerIDs := make([]ports.NodeID, N)
	for i := range ids {
		ids[i] = identity.FromSeed(int64(9300 + i))
		peerIDs[i] = ids[i].NodeID()
		anchors[peerIDs[i]] = true
	}
	g := &chain.Block{Version: 1, Height: 0, Entries: []ports.Entry{mkEntry("coverage-genesis")}}
	chain.Sign(g, ids[0].Signer())

	// THE RC POSTURE: era 4 from height 1, so every minted block is witnessable and a
	// registration can be committed by digest at all. Below that boundary there is no
	// shed form, and the measurement would be reporting a relay that cannot engage.
	//
	// THE TTL IS THE SHIPPED ONE. Renewal is due at half of it, so the TTL sets how
	// often a ~1.5 MB registration has to cross the wire at all — which is the
	// quantity every byte figure below is proportional to. A tighter TTL makes a
	// livelier measurement and an unquotable one.
	cfg := chain.Config{
		Quorum: 2, MinBond: 1 << 20, Anchors: anchors, AnchorQuorum: 2,
		MatureValidators: 99, BondTTLBlocks: ttl,
		Era3ActivationHeight: 1, Era4ActivationHeight: 1,
	}

	nodes := make([]*Node, N)
	for i, id := range ids {
		nd := New(id.NodeID(), DefaultConfig(), sched, net.Endpoint(id.NodeID()), memstore.New())
		nd.SetLedger(credit.New(50_000, 0))
		nd.EnableBond(id.Signer(), bondSize) // a REAL plot: renewals mint real proofs
		ch := chain.New(cfg, func(ports.NodeID) int64 { return 0 })
		if err := ch.AppendGenesis(*g); err != nil {
			t.Fatalf("genesis: %v", err)
		}
		nd.EnableChain(ch, id.Signer())
		nd.EnableObjectiveChain()
		nodes[i] = nd
	}
	for i := 1; i < N; i++ {
		nodes[i].Bootstrap([]ports.NodeID{peerIDs[0]}, func() {})
	}
	sched.Run()

	for i := range nodes {
		seed := make([]ports.NodeID, 0, N-1)
		for j := range nodes {
			if j != i {
				seed = append(seed, peerIDs[j])
			}
		}
		nodes[i].StartChainSync(seed, nil)
	}
	sched.RunUntil(sched.Now().Add(nodes[0].cfg.ChainSyncInterval * ports.Duration(intervals)))

	var c coverage
	for i, nd := range nodes {
		s := nd.Stats
		c.shed += s.GatherLegsShed
		c.carried += s.GatherLegsCarried
		c.ownUnacked += s.GatherLegsCarriedOwnUnacked
		c.peerUnreported += s.GatherLegsCarriedPeerUnreported
		c.held += s.BondRegSubmitsHeld
		c.acksLost += s.BondRegSubmitAcksLost
		c.probesAnswered += s.ChainHeadProbesAnswered
		c.probesLost += s.ChainHeadProbesLost
		t.Logf("    node %d: shed %3d  carried %3d  (own-unacked %d, peer-unreported %d)",
			i, s.GatherLegsShed, s.GatherLegsCarried,
			s.GatherLegsCarriedOwnUnacked, s.GatherLegsCarriedPeerUnreported)
	}
	st := net.Stats
	c.sweeps, c.peersPer, c.rate = intervals, N-1, netcfg.RateBytesPerSec
	c.submits, c.submitBytes = int64(st.Kinds[ports.MsgSubmitBondReg]), st.KindBytes[ports.MsgSubmitBondReg]
	c.submitsArrived = int64(st.KindDelivered[ports.MsgSubmitBondReg])
	c.proposals, c.proposalBytes = int64(st.Kinds[ports.MsgProposeBlock]), st.KindBytes[ports.MsgProposeBlock]
	c.prepareQCs, c.prepareQCByte = int64(st.Kinds[ports.MsgPrepareQC]), st.KindBytes[ports.MsgPrepareQC]
	_, c.head = nodes[0].Chain().Head()
	return c
}

func (c coverage) report(t *testing.T, arm string) {
	t.Helper()
	t.Logf("  %s: head %d, %d registration-bearing legs", arm, c.head, c.legs())
	if c.legs() > 0 {
		t.Logf("    SHED    %3d  (%.0f%%)", c.shed, c.pct())
		t.Logf("    CARRIED %3d  (%.0f%%)  — own-reg unevidenced %d, peer-reg unevidenced %d",
			c.carried, 100-c.pct(), c.ownUnacked, c.peerUnreported)
	}
	t.Logf("    what the link was asked to carry:")
	perSlot := 0.0
	if slots := c.sweeps * c.peersPer * 4; slots > 0 {
		perSlot = float64(c.submits) / float64(slots)
	}
	t.Logf("      MsgSubmitBondReg %4d sends %12d B  (%.2f per peer per sweep), %d ARRIVED",
		c.submits, c.submitBytes, perSlot, c.submitsArrived)
	t.Logf("        of those arrivals the receivers QUEUED %d; the senders lost %d acknowledgements",
		c.held, c.acksLost)
	t.Logf("      chain-head probes (the other carrier of the same evidence): %d answered, %d lost",
		c.probesAnswered, c.probesLost)
	t.Logf("      MsgProposeBlock  %4d sends %12d B", c.proposals, c.proposalBytes)
	t.Logf("      MsgPrepareQC     %4d sends %12d B", c.prepareQCs, c.prepareQCByte)
	gather := c.proposalBytes + c.prepareQCByte
	if gather > 0 {
		t.Logf("      renewal submits are %.1fx the gather legs the relay covers", float64(c.submitBytes)/float64(gather))
	}
	if c.rate > 0 {
		// WHAT THE LINK COULD HAVE CARRIED in the same window, against what the
		// renewal path alone asked it for. A ratio above 1 means the submits
		// oversubscribe the wire before a single consensus frame is queued, which is
		// a different failure from a deadline being missed and wants a different fix.
		budget := c.rate * int64(c.sweeps) * int64(sweepSeconds) * int64(4*c.peersPer)
		t.Logf("      the link's whole capacity over the run was %d B across %d ordered pairs; "+
			"the renewal submits alone asked for %.1fx it", budget, 4*c.peersPer, float64(c.submitBytes)/float64(budget))
	}
}

// THE RELAY'S COVERAGE, MEASURED BELOW THE FIELD TIER FOR THE FIRST TIME.
//
// A gather leg — one proposal or one prepare-QC to one attester — either crosses by
// DIGEST at ~1 KB or CARRIES the space-time proofs at ~1,574,000 B each. A link that
// cannot move the carried form inside the per-attempt deadline cannot commit the
// height at all, so the share of legs that shed is the quantity the adverse-network
// liveness bound rests on. The field has measured it three times and only ever on a
// billable cloud run: 1-of-n−1 by construction, then 37.5%, then 47%.
//
// A number that can only be read on a cloud run cannot be iterated against, and the
// TWO ARMS are the point: the same four validators on a fast wire and on one whose
// rate is a quarter of the deadline's assumed floor. Coverage on the fast arm says
// what the mechanism achieves when nothing is in its way; the gap between the arms
// is the thing the field measured and the thing there is work to do about.
//
// It is a MEASUREMENT with a floor, not a target. The assertions are the two things
// that would make either number a lie — no registration-bearing leg ran at all, and
// nothing ever committed on the healthy arm. The coverage itself is reported.
func TestTheDigestRelaysCoverageOnAHealthyWireAndACongestedOne(t *testing.T) {
	if testing.Short() {
		t.Skip("mints real space-time proofs on every renewal, on two arms")
	}
	t.Log("MEASURED — the digest relay's coverage, by the wire it runs on:")

	healthy := driveCoverage(t, simnet.DefaultConfig(), 72, shippedBondTTL)
	healthy.report(t, "a fast wire (5-50 ms, no rate limit)")

	// A QUARTER OF THE ASSUMED FLOOR, which is the repro the wedge reduces to:
	// requestTimeoutFor extends a deadline by len(payload)/RequestSizeFloorBytesPerSec
	// = 256 KiB/s, so a 1.5 MB proposal is given 14 s and needs 24 s. Same arithmetic
	// as TestLargePayloadTimesOutOnALinkBelowTheAssumedFloor, driven here against the
	// whole consensus loop instead of one request.
	congestedNet := simnet.Config{
		LatencyMin: 5 * ports.Millisecond, LatencyMax: 50 * ports.Millisecond,
		RateBytesPerSec: 64 << 10,
	}
	congested := driveCoverage(t, congestedNet, 72, shippedBondTTL)
	congested.report(t, "a congested wire (64 KiB/s, a quarter of the assumed floor)")

	if healthy.legs() == 0 || congested.legs() == 0 {
		t.Fatal("VACUOUS: an arm ran not one gather leg carrying a bond registration, so it measured " +
			"nothing. Either the run never proposed a registration-bearing block or the era boundary " +
			"left every block below v5 — check the head height and Era4ActivationHeight before " +
			"reading any coverage figure here.")
	}
	if healthy.head == 0 {
		t.Fatal("VACUOUS: the healthy arm committed nothing. Coverage over a chain that never commits " +
			"is a ratio of attempts, not of the legs a live chain actually runs.")
	}

	// THE LIVENESS GATE, and it is the one this measurement exists to defend. A link
	// below the deadline's assumed floor used to stop the chain outright: the congested
	// arm reached height 1 and stayed there, because every exchange large enough to
	// carry a registration expired while its bytes were still crossing, so no receipt
	// was ever recorded, every gather leg carried ~1.5 MB, and the renewal path re-sent
	// the same proof to every peer on every sweep — the stall feeding the traffic that
	// sustained the stall.
	//
	// It is deliberately a floor on COMMITS and not on coverage. Coverage on this arm
	// is a property of the wire as much as of the relay and is reported rather than
	// asserted; whether the chain moves at all is a property of the system, and it is
	// the claim an adverse-network liveness bound actually makes.
	//
	// THE FLOOR IS WHERE THE WEDGE WAS, not where the work stops. The chain moves here
	// now and it did not before, and that is the whole of what this gate asserts. It is
	// NOT a claim that this arm is healthy: the deadline reaches the size-extension cap
	// and the link still queues multiple ~1.5 MB payloads per ordered pair behind it, so
	// most acknowledgements are still lost and coverage is far below the fast arm's.
	// What remains is above this mechanism, not inside it.
	const congestedCommitFloor = 2
	if congested.head < congestedCommitFloor {
		t.Fatalf("the chain reached height %d on a wire a quarter of the deadline's assumed floor "+
			"(%d gather legs, %.0f%% shed, %d acknowledgements lost, %d head probes lost). Below %d this is "+
			"the wedge again: a deadline sized against a constant the link is a quarter of expires while "+
			"the bytes are still on the wire, so the receipts that would shed the next block are destroyed "+
			"and the renewal path re-sends the full proof every sweep. Check the per-peer rate estimate "+
			"(peerrate.go) before reading any coverage figure on this arm.",
			congested.head, congested.legs(), congested.pct(), congested.acksLost, congested.probesLost,
			congestedCommitFloor)
	}

	// THE GATE ON THE FAST ARM, which is the only arm where a coverage figure is a
	// property of the relay rather than of the wire. The floor is set well under the
	// measured value and well over the two states this mechanism has already been in:
	// 1-of-n−1 by construction (~33% at four validators) and 37.5%. A regression that
	// put it back in either would fail here, on a developer's machine, in seconds.
	const fastArmCoverageFloor = 80
	if healthy.pct() < fastArmCoverageFloor {
		t.Fatalf("the digest relay covered only %.0f%% of gather legs on a wire with nothing in its way "+
			"(%d shed, %d carried — own-reg unevidenced %d, peer-reg unevidenced %d). Below %d%% the relay "+
			"is back to shedding for the registration's own author and little else, which is the coverage "+
			"it had before the holder-reported inventory existed.",
			healthy.pct(), healthy.shed, healthy.carried, healthy.ownUnacked, healthy.peerUnreported,
			fastArmCoverageFloor)
	}
	// AND THE PROPERTY UNDER IT: on a wire that delivers, a proposer never carries its
	// OWN registration for want of a receipt. The propose path embeds the copy it
	// broadcast, and the broadcast is acknowledged only once the peer has queued the
	// bytes — so every attester holds this node's own proof by the time it proposes.
	// A non-zero count here is that chain breaking, and it is the one carry reason the
	// proposer can do something about alone.
	if healthy.ownUnacked != 0 {
		t.Fatalf("%d gather legs on a FAST wire carried this node's own registration because no peer "+
			"evidence covered it. The registration a proposer commits is the one it broadcast, and a "+
			"broadcast is acknowledged only when the peer has queued the bytes — so on a wire that "+
			"delivers, this is zero. Check the mint-once-and-keep rule and the acknowledgement before "+
			"reading the coverage figure above.", healthy.ownUnacked)
	}
}
