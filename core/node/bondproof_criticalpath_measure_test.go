package node

import (
	"testing"

	"github.com/nerolabs/silt/adapters/identity"
	"github.com/nerolabs/silt/adapters/memstore"
	"github.com/nerolabs/silt/adapters/simclock"
	"github.com/nerolabs/silt/adapters/simnet"
	"github.com/nerolabs/silt/ports"
)

// MEASURING WHETHER THE BOND PROOF HAS TO RIDE THE CONSENSUS CRITICAL PATH.
//
// The field finding is attributed: every consensus proposal carries the full
// ~1.5 MB space-time bond proof, and its per-attempt transport deadline is sized
// from an ASSUMED 256 KiB/s floor the impaired wire does not deliver (measured
// 53-181 KB/s). The structural close build-immutable #5 names is to keep the large
// payload OFF the critical path, and v5 already has the hook: BondReg.AnswerDigest
// lets a block COMMIT to the proof without CARRYING it.
//
// THE READING THAT PROMPTED THIS IS NOT YET A MEASUREMENT, and #7 says the next
// action is to gather rather than to act. Two facts in the tree suggest the
// committed bytes may be separable from the proposal bytes — Prune() already drops
// Answer from a finalized v5 block and the hash still reproduces, and every
// attester already receives and verifies a peer's registration before the block
// carrying it is proposed. If an attester can reconstruct the block from its own
// pending queue by digest, the committed bytes are identical and nothing forks:
// that is compact-block relay, a transport change needing no era.
//
// These tests answer the three questions that decide it, and their output is the
// deliverable. NONE of them is a gate on the fix — there is no fix yet. They are
// assertions about what is true NOW, so the numbers cannot rot while the decision
// is being made, and so the premise is re-checked rather than recalled if it takes
// more than one session.
//
// The three questions, from the sheet: does retry de-duplication alone move the
// wedge; does an attester's pending queue actually hold the registration when the
// block arrives; and what does the fallback cost when it does not.

// countingEnd totals the payload bytes a node OFFERS to the wire. Offered, not
// delivered, is the right quantity: the field journals show copies still arriving
// after the gather that sent them had already terminated, so bytes the sender
// pushed into a backlog are the cost even when nothing reads them.
type countingEnd struct {
	inner ports.Transport
	bytes int64
	sends int
	kind  ports.MsgKind
}

func (c *countingEnd) SetHandler(h func(ports.NodeID, ports.Message)) { c.inner.SetHandler(h) }

func (c *countingEnd) Send(to ports.NodeID, msg ports.Message) error {
	if msg.Kind == c.kind {
		c.bytes += int64(len(msg.Data))
		c.sends++
	}
	return c.inner.Send(to, msg)
}

// QUESTION 1 — DOES RETRY DE-DUPLICATION ALONE MOVE THE WEDGE?
//
// The field journals recorded all three attesters logging PREPARED for the same
// height and round FOUR times, the last copies landing 86 s after the gather that
// sent them had terminated. Four attempts x three attesters x 1.5 MB is 18 MB
// pushed onto links delivering ~110 KB/s, for one round already declared failed.
//
// This measures the multiplier at the shipped daemon setting (-request-retries 3)
// on a link a quarter of the assumed floor, against a no-retry control on the same
// payload. What it CANNOT say is whether removing the re-ship fixes the wedge: the
// first attempt is already under-budgeted (24 s of wire against a 14 s deadline),
// so de-duplication alone changes what the ladder COSTS, not whether the first
// attempt fits. That distinction is the finding.
func TestRetryLadderReshipsTheWholePayload(t *testing.T) {
	measure := func(retries int) (int64, int) {
		sched := simclock.New()
		net := simnet.New(sched, 1, simnet.Config{RateBytesPerSec: 64 << 10}) // a quarter of the floor
		me, peer := identity.FromSeed(7001), identity.FromSeed(7002)

		cfg := DefaultConfig()
		cfg.RequestTimeout = 8 * ports.Second
		cfg.RequestRetries = retries
		cfg.RequestBackoff = 250 * ports.Millisecond
		cfg.RequestSizeFloorBytesPerSec = 256 << 10

		end := &countingEnd{inner: net.Endpoint(me.NodeID()), kind: ports.MsgProposeBlock}
		nd := New(me.NodeID(), cfg, sched, end, memstore.New())
		// A peer that never answers: the ladder runs to exhaustion, which is the
		// case the field recorded. A peer that answered would measure a happy path.
		net.Endpoint(peer.NodeID())
		nd.table.Observe(peer.NodeID())

		nd.request(peer.NodeID(), ports.Message{Kind: ports.MsgProposeBlock, Data: make([]byte, payloadBytes)},
			func(ports.Message, error) {})
		sched.Run()
		return end.bytes, end.sends
	}

	oneBytes, oneSends := measure(0)
	ladderBytes, ladderSends := measure(3) // the shipped daemon default

	t.Logf("MEASURED — one %d-byte proposal to ONE peer on a link at a quarter of the assumed floor:", payloadBytes)
	t.Logf("  no retry (control):     %d sends, %d B offered", oneSends, oneBytes)
	t.Logf("  -request-retries 3:     %d sends, %d B offered  (%.1fx)", ladderSends, ladderBytes,
		float64(ladderBytes)/float64(oneBytes))
	t.Logf("  at the field's 3 attesters that is %d B offered for ONE height and round", ladderBytes*3)

	if oneSends != 1 || oneBytes != payloadBytes {
		t.Fatalf("CONTROL BROKEN: the no-retry arm offered %d sends / %d B, want exactly 1 send of %d B — "+
			"the rig is not measuring one attempt and neither number below means anything", oneSends, oneBytes, payloadBytes)
	}
	// The property: a retry re-ships the PAYLOAD, it does not re-ask for something
	// already in flight. This is what a de-duplicating retry would change.
	if ladderSends != 4 || ladderBytes != 4*payloadBytes {
		t.Fatalf("the shipped retry ladder offered %d sends / %d B for one proposal, want 4 x %d B. If it is LOWER, a "+
			"de-duplication landed and this measurement is stale — re-read it before citing the 4x. If it is HIGHER, the "+
			"ladder is running more attempts than -request-retries says.", ladderSends, ladderBytes, payloadBytes)
	}
}

// QUESTION 2 — DOES THE ATTESTER ALREADY HOLD THE REGISTRATION?
//
// Compact-block relay only works where the receiver can reconstruct the body it is
// missing. The answer here is SPLIT, and the split is the whole result:
//
//   - A PEER's registration: yes. SubmitBondRenewal broadcasts the full reg to the
//     validator set, each receiver runs ValidateBondRegErr — paying the full
//     VerifySpaceTime — and queues it. The proposer then folds that same reg into a
//     block and re-ships the identical bytes to the same attesters, who verify them
//     a SECOND time. That copy is pure duplication and a digest would replace it.
//
//   - The PROPOSER's OWN registration: no. chainrole mints it fresh at propose time
//     over b.Prev and it is never submitted to anyone, so no attester has ever seen
//     those bytes when the block arrives.
//
// So the separability the sheet suspected is real for one of the two sources and
// false for the other, which is exactly the thing that had to be measured rather
// than reasoned about: a compact-block scheme that assumed the queue always holds
// the reg would stall on every proposer's own renewal.
func TestAttesterHoldsAPeersRegButNeverTheProposersOwn(t *testing.T) {
	nodes, ids, net, _, _ := tier2AnchorNet(t, 4)
	attester, submitter, proposer := nodes[0], ids[1], nodes[2]
	// The proposer needs a real plot, or RegisterBondReg returns !ok and leg 2's
	// zeros are the absence of a registration rather than the absence of a submit.
	// That vacuity is guarded below rather than trusted to this line.
	proposer.EnableBond(ids[2].Signer(), 2<<20)

	// LEG 1 — a PEER's registration arrives the way SubmitBondRenewal sends it, on
	// the real arrival path rather than through a helper, and lands in the queue a
	// compact-block reconstruction would read.
	attester.handleChain(submitter.NodeID(), ports.Message{
		Kind: ports.MsgSubmitBondReg, Data: bondRegEncode(regFrom(attester.chain, submitter)),
	})
	heldFromPeer := 0
	for _, pr := range attester.pendingBondRegs {
		if pr.R.ValidatorID() == submitter.NodeID() {
			heldFromPeer++
		}
	}

	// LEG 2 — the PROPOSER's own registration. The question is not whether the
	// attester happens to hold it but whether anything ever PUTS it there, so this
	// counts what the proposer sends: RegisterBondReg mints over b.Prev at propose
	// time and no submit accompanies it.
	head, _ := proposer.chain.Head()
	before := net.Stats.Kinds[ports.MsgSubmitBondReg]
	_, minted := proposer.RegisterBondReg(head)
	submitsForOwn := net.Stats.Kinds[ports.MsgSubmitBondReg] - before

	heldOwn := 0
	for _, pr := range attester.pendingBondRegs {
		if pr.R.ValidatorID() == proposer.ID() {
			heldOwn++
		}
	}

	t.Logf("MEASURED — what an attester holds when a block carrying a registration arrives:")
	t.Logf("  a PEER's reg (MsgSubmitBondReg, the non-proposer renewal path): %d in the pending queue", heldFromPeer)
	t.Logf("  the PROPOSER's OWN reg (minted=%v over b.Prev at propose time): %d in the pending queue, "+
		"%d submits sent to anyone", minted, heldOwn, submitsForOwn)
	t.Logf("  => compact relay can reconstruct the FIRST source and not the SECOND")

	if heldFromPeer != 1 {
		t.Fatalf("PREMISE BROKEN: a submitted peer registration reached the attester's pending queue %d times, want 1. "+
			"The compact-relay reading rests on it being there, so find out why before drawing any conclusion from the "+
			"proposer-own leg.", heldFromPeer)
	}
	if !minted {
		t.Fatal("VACUOUS: the proposer minted no registration at all, so the zeros above measure a missing plot rather " +
			"than a missing submit. Give the proposer a bond before reading either number.")
	}
	if heldOwn != 0 || submitsForOwn != 0 {
		t.Fatalf("the proposer's OWN registration reached an attester (%d queued, %d submits sent). That contradicts the "+
			"read of chainrole — RegisterBondReg(b.Prev), never submitted — and if a proposer now broadcasts its own reg "+
			"first, compact relay covers BOTH sources and the fallback priced below is dead code. Re-derive before building "+
			"either way.", heldOwn, submitsForOwn)
	}
}

// QUESTION 3 — WHAT DOES THE FALLBACK COST?
//
// When the attester does not hold the reg, a digest-only proposal cannot be
// reconstructed and the attester must ask for the body. That is a round trip plus
// the same payload — so on the impaired wire the fallback costs MORE than shipping
// the bytes outright, and it arrives later in the round.
//
// This prices it against the deadline rather than in the abstract. The comparison
// is the point: a scheme that helps a peer-submitted reg and hurts a proposer's own
// is not a win until the mix is known, and the mix is what question 2 measured.
func TestFallbackCostsARoundTripOnTopOfTheSamePayload(t *testing.T) {
	const rate = 64 << 10 // the impaired quarter-floor

	sched := simclock.New()
	net := simnet.New(sched, 1, simnet.Config{RateBytesPerSec: rate})
	me := identity.FromSeed(7101)

	cfg := DefaultConfig()
	cfg.RequestTimeout = 8 * ports.Second
	cfg.RequestRetries = 0
	cfg.RequestSizeFloorBytesPerSec = 256 << 10
	nd := New(me.NodeID(), cfg, sched, net.Endpoint(me.NodeID()), memstore.New())

	// The deadline the sender arms for each shape, read off the product rather than
	// recomputed here — a test that restated the formula would pass through a change
	// to it.
	full := nd.requestTimeoutFor(ports.Message{Kind: ports.MsgProposeBlock, Data: make([]byte, payloadBytes)})
	digest := nd.requestTimeoutFor(ports.Message{Kind: ports.MsgProposeBlock, Data: make([]byte, 32)})
	wire := ports.Duration(int64(payloadBytes) * int64(ports.Second) / rate)

	t.Logf("MEASURED — one %d-byte registration on a link at %d B/s (a quarter of the assumed %d B/s floor):",
		payloadBytes, rate, cfg.RequestSizeFloorBytesPerSec)
	t.Logf("  CARRYING it:  deadline %s, one-way wire %s  -> %s", secs(full), secs(wire), fitVerdict(wire, full))
	t.Logf("  DIGEST only:  deadline %s for the 32-byte proposal -> %s", secs(digest), fitVerdict(
		ports.Duration(32*int64(ports.Second)/rate), digest))
	t.Logf("  ...but a MISS then costs the digest round trip PLUS the same %s of wire, arming a deadline of %s "+
		"for the body fetch — so the fallback is strictly WORSE than carrying, and later in the round",
		secs(wire), secs(full))

	// The finding, asserted so it cannot rot: carrying the proof does not fit, which
	// is the wedge; and the digest proposal DOES fit, which is why the idea is worth
	// measuring at all.
	if wire <= full {
		t.Fatalf("the full payload now FITS its deadline (%v of wire against %v) — the wedge's arithmetic has changed, "+
			"so the case for taking the proof off the critical path must be re-derived rather than assumed", wire, full)
	}
	if d := ports.Duration(32 * int64(ports.Second) / rate); d > digest {
		t.Fatalf("even a 32-byte digest proposal misses its deadline (%v of wire against %v) — the link is so slow that "+
			"compact relay buys nothing here, and this rig is not the one to price it on", d, digest)
	}
}

func fitVerdict(wire, deadline ports.Duration) string {
	if wire > deadline {
		return "MISSES"
	}
	return "fits"
}
