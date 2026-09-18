package node

import (
	"testing"

	"github.com/nerolabs/silt/adapters/identity"
	"github.com/nerolabs/silt/adapters/memstore"
	"github.com/nerolabs/silt/adapters/simclock"
	"github.com/nerolabs/silt/adapters/simnet"
	"github.com/nerolabs/silt/ports"
)

// Answering a storage challenge reads the whole shard back and aggregates it —
// measured at 8.3 ms and 8,643 field multiplications over a 256 KiB shard — on the
// node's single serialized loop, and MsgChallenge carries no signature and no
// standing requirement. allowPorChallenge is the cheap gate in front of that work.
//
// It is the last of the two work bounds item 10 names; the other is the survivor
// fetch on the repair-claim path.
func TestPorChallengeRateLimitPerChallenger(t *testing.T) {
	sched := simclock.New()
	net := simnet.New(sched, 1, simnet.DefaultConfig())
	ident := identity.FromSeed(1)
	nd := New(ident.NodeID(), DefaultConfig(), sched, net.Endpoint(ident.NodeID()), memstore.New())

	x := identity.FromSeed(100).NodeID()
	y := identity.FromSeed(200).NodeID()

	// The burst budget from X is admitted: an honest sweep is serialized and never
	// reaches it, so everything inside the budget must pass untouched.
	for i := 0; i < porChallengeBurst; i++ {
		if !nd.allowPorChallenge(x) {
			t.Fatalf("challenge %d/%d from X refused inside its burst budget", i+1, porChallengeBurst)
		}
	}
	// The next one in the same window is refused — BEFORE the shard read and the
	// aggregation, which is the entire point of putting the gate where it is.
	if nd.allowPorChallenge(x) {
		t.Fatal("a challenge past the per-window burst must be refused, or the flood is unbounded")
	}

	// CONTROL 1 — per-challenger, not global. A flooder must not be able to spend
	// an honest auditor's budget, or the gate would be a denial-of-audit primitive
	// rather than a defence. This also proves the refusal above is not simply
	// "refuse everything once busy".
	if !nd.allowPorChallenge(y) {
		t.Fatal("a distinct challenger was charged for X's spent budget — a flooder could then " +
			"starve every honest auditor of its own audit")
	}

	// CONTROL 2 — the window rolls. A budget that never refilled would turn one
	// burst into a permanent refusal for that peer.
	sched.AfterFunc(DefaultConfig().ChainSyncInterval, func() {})
	sched.Run()
	if !nd.allowPorChallenge(x) {
		t.Fatal("X was still refused after its window elapsed — the budget does not refill, so a " +
			"single burst silences that challenger forever")
	}
}

// THE ARM THAT MATTERS. A rate limit on an audit response is only safe if refusing
// cannot be mistaken for failing: the auditor grades Found=false as a prover that
// could not produce a proof — the same verdict as a liar — so a gate that replied
// would hand any peer a way to slash an honest holder by first spending its budget.
//
// This asserts the refusal is a DROP: the handler emits nothing at all, so the
// auditor sees a transport error and auditLeaf never counts it. A dropped challenge
// is not a failed audit, it is no audit.
func TestRefusedPorChallengeSendsNoReply(t *testing.T) {
	sched := simclock.New()
	net := simnet.New(sched, 1, simnet.DefaultConfig())
	ident := identity.FromSeed(7)
	nd := New(ident.NodeID(), DefaultConfig(), sched, net.Endpoint(ident.NodeID()), memstore.New())

	// A held chunk, so the honest path has something real to answer with and the
	// admitted arm below is not passing because there was nothing to prove.
	data := []byte("shard-bytes-for-the-challenge-gate")
	id := ports.HashBytes(data)
	if err := nd.store.Put(bg(), ports.Chunk{ID: id, Data: data}); err != nil {
		t.Fatalf("store put: %v", err)
	}

	challenger := identity.FromSeed(300).NodeID()
	msg := ports.Message{Kind: ports.MsgChallenge, ChunkID: id, PorCount: porSampleCount}

	// Spend the budget through the gate directly, so this test measures the HANDLER's
	// behaviour once refused rather than re-measuring the counter.
	for i := 0; i < porChallengeBurst; i++ {
		nd.allowPorChallenge(challenger)
	}

	before := net.Stats.Kinds[ports.MsgChallengeReply]
	nd.handle(challenger, msg)
	if got := net.Stats.Kinds[ports.MsgChallengeReply] - before; got != 0 {
		t.Fatalf("a rate-refused challenge produced %d repl(ies) — any reply here is graded, and a "+
			"Found=false verdict from a RATE LIMIT is an honest holder being slashed by a peer that "+
			"chose to flood it", got)
	}

	// CONTROL: the same handler on a challenger with budget does reply, so the zero
	// above is the gate and not a handler that never answers anything.
	fresh := identity.FromSeed(301).NodeID()
	before = net.Stats.Kinds[ports.MsgChallengeReply]
	nd.handle(fresh, msg)
	if got := net.Stats.Kinds[ports.MsgChallengeReply] - before; got == 0 {
		t.Fatal("the handler produced no reply for a challenger INSIDE its budget either — the arm " +
			"above is vacuous and proves nothing about the gate")
	}
}
