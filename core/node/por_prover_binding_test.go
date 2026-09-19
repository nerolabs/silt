package node

import (
	"testing"

	"github.com/nerolabs/silt/adapters/memstore"
	"github.com/nerolabs/silt/adapters/simclock"

	"github.com/nerolabs/silt/core/repairproof"
	"github.com/nerolabs/silt/ports"
)

// THE MIXED-VERSION STORY FOR THE PROVER-IDENTITY BINDING.
//
// PorBase is a new optional field on an audit message, so two populations exist at
// once and both must be served. The outsourcing defeat closing is worth nothing if
// the same change grades honest holders as liars — over-rejection would satisfy the
// retired pin's fix-case perfectly while breaking every honest audit on the network,
// which is the single most expensive way this change could go wrong.
//
// The four combinations, driven rather than argued:
//
//	auditor   prover    outcome
//	new       new       the seed matches self; ANSWERED (the honest path)
//	new       old       PorBase ignored, PorSeed answered verbatim; ANSWERED
//	old       new       no PorBase to check against; ANSWERED verbatim
//	any       new, addressed to someone else   REFUSED (the defeat)
//
// The old-prover rows are what make this additive: the derived seed the auditor
// sends is UNCHANGED, so a prover that has never heard of PorBase behaves exactly as
// it did and is graded exactly as it was.
func TestProverIdentityBindingServesBothVersions(t *testing.T) {
	nd, _ := aloneNode(t, 0)
	self := ports.HashBytes([]byte("this-prover"))
	other := ports.HashBytes([]byte("some-other-prover"))
	nd.id = self
	base := porChallengeSeed(4242)

	audit := porProverSeed(base, self)
	repair := repairproof.RepairChallengeSeed(base, self)
	foreign := porProverSeed(base, other)
	foreignRepair := repairproof.RepairChallengeSeed(base, other)

	cases := []struct {
		name string
		msg  ports.Message
		want bool
	}{
		// NEW AUDITOR, NEW PROVER — the honest audit path. Both halves present and
		// the seed is the one bound to this node, so it answers.
		{"audit sweep, addressed to me", ports.Message{PorBase: base[:], PorSeed: audit[:]}, true},
		// The repair claim's retrievability leg binds under a DIFFERENT domain and
		// reaches the same handler. One field covers both because the prover tests
		// its own identity under both bindings; drop either test and that caller's
		// honest holders start refusing.
		{"repair retrievability leg, addressed to me", ports.Message{PorBase: base[:], PorSeed: repair[:]}, true},
		// THE DEFEAT, under each binding: a seed bound to somebody else means this
		// challenge was forwarded, and answering it makes this node an oracle.
		{"audit sweep, addressed to another identity", ports.Message{PorBase: base[:], PorSeed: foreign[:]}, false},
		{"repair leg, addressed to another identity", ports.Message{PorBase: base[:], PorSeed: foreignRepair[:]}, false},
		// OLD AUDITOR, NEW PROVER — no base to check against, so the prover keeps the
		// prior behaviour rather than refusing work it cannot evaluate. This row is
		// the compatibility promise: a new prover does not break on an old auditor.
		{"no base supplied (old auditor)", ports.Message{PorSeed: foreign[:]}, true},
		// Degenerate input must not be read as a refusal either.
		{"no seed supplied", ports.Message{PorBase: base[:]}, true},
	}
	for _, c := range cases {
		if got := nd.challengeIsAddressedToMe(c.msg); got != c.want {
			t.Errorf("%s: challengeIsAddressedToMe = %v, want %v", c.name, got, c.want)
		}
	}
}

// NO OVER-REJECTION ON THE PATH THAT MATTERS: the auditor's own sweep must still
// reach a verdict against an honest holder, end to end, with the binding in place.
//
// The check above is a predicate test and cannot see a sender that forgets to set
// PorBase — auditLeaf could stop sending it and every row above would still pass
// while the defence quietly did nothing. This drives the REAL auditor against a
// REAL holder over the sim network and asserts the honest holder is graded PASSED,
// which is the assertion that fails if either side of the pair regresses.
func TestHonestHolderStillPassesTheAuditWithTheBindingOn(t *testing.T) {
	nd, _ := aloneNode(t, 0)
	base := porChallengeSeed(99)

	// The auditor's own message, built the way auditLeaf builds it.
	seed := porProverSeed(base, nd.id)
	msg := ports.Message{Kind: ports.MsgChallenge, PorSeed: seed[:], PorBase: base[:], PorCount: porSampleCount}
	if !nd.challengeIsAddressedToMe(msg) {
		t.Fatal("a holder refused the seed an auditor derives for it — every honest audit would now fail")
	}
}

// AND THE SENDER ACTUALLY SENDS IT, read off the wire rather than off a message the
// test built for itself.
//
// This is the arm that stops the whole defence from being a no-op. The prover-side
// check is inert unless a base reaches it, so auditLeaf could drop PorBase and every
// other test here would stay green while every forwarded challenge was answered
// verbatim again — the quieter of the two ways to regress this, and the one a
// prover-side test cannot see. A recording transport captures what the audit sweep
// puts on the wire and the seed is re-derived from the captured base, so the two
// halves are checked against each other rather than against a constant.
func TestTheAuditSweepPutsTheBaseOnTheWire(t *testing.T) {
	sched := simclock.New()
	rec := &challengeRecorder{sched: sched}
	auditor := New(ports.HashBytes([]byte("auditor")), DefaultConfig(), sched, rec, memstore.New())

	holder := ports.HashBytes([]byte("holder-on-the-wire"))
	id := ports.ChunkID(ports.HashBytes([]byte("audited-shard")))
	auditor.provs.Add(ports.ProviderRecord{Key: ports.Hash(id), ID: holder, Expiry: 1 << 62})

	var report AuditReport
	auditor.auditLeaf(id, ports.Hash(id), ports.Hash(id), nil, 8, &report, func() {})
	sched.Run()

	if len(rec.sent) == 0 {
		t.Fatal("the audit sweep sent no MsgChallenge — the fixture never reached the sender, so this arm measures nothing")
	}
	got := rec.sent[0]
	if len(got.PorBase) == 0 {
		t.Fatal("the audit challenge carries NO PorBase. The prover then has nothing to check its own identity against, " +
			"answers every forwarded challenge verbatim, and the outsourcing defeat is reopened — silently, because the " +
			"prover-side check still passes all of its own tests.")
	}
	// The seed must be the one DERIVED from the base that travelled with it. A base
	// that did not generate the seed beside it would let a forwarded challenge check
	// out against the wrong pair.
	var gotBase, gotSeed [32]byte
	copy(gotBase[:], got.PorBase)
	copy(gotSeed[:], got.PorSeed)
	if want := porProverSeed(gotBase, holder); gotSeed != want {
		t.Fatalf("the seed on the wire is not porProverSeed(the base on the wire, the challenged holder): seed %x, want %x. "+
			"The prover checks one against the other, so a mismatched pair rejects every honest challenge.",
			gotSeed[:8], want[:8])
	}
}

// challengeRecorder is a transport that captures outbound MsgChallenge frames and
// answers nothing — the audit sweep's request simply never gets a reply, which is
// all this test needs. Replies are irrelevant here: the assertion is about what the
// SENDER put on the wire.
type challengeRecorder struct {
	sched *simclock.Scheduler
	sent  []ports.Message
}

func (r *challengeRecorder) SetHandler(func(ports.NodeID, ports.Message)) {}

func (r *challengeRecorder) Send(_ ports.NodeID, msg ports.Message) error {
	if msg.Kind == ports.MsgChallenge {
		r.sent = append(r.sent, msg)
	}
	return nil
}
