package node

import (
	"testing"

	"github.com/nerolabs/silt/ports"
)

// THE THIRD GATE IN A SET OF TWO.
//
// cfg.RepairEconomy already gates both of the other sides of the repair-bounty
// protocol. announceRepairQuorum returns early when it is off, so the node never
// plants itself under careKey(root) and is never DISCOVERABLE as a caretaker-judge;
// emitRepairClaim returns early too, so it never SENDS one. The judging side had no
// such gate: handleRepairClaim ran the registry lookup, the manifest fetch and the
// survivor walk, and cfg.RepairEconomy was not consulted until settlement.
//
// That asymmetry is what makes the amplification free rather than merely large. A
// node with the economy off is not reachable through the honest rendezvous, so its
// legitimate inbound claim traffic is ZERO BY CONSTRUCTION — every claim it receives
// came from someone who did not resolve it the honest way. There is no honest case
// to protect, and the whole cost is attacker-directed.
//
// WHAT THIS GATE DOES NOT CLOSE, said plainly because the shape invites the
// misreading: it narrows the WORK, it does not bound the COUNT. A node running the
// economy still pays the measured cost per claim, unsigned and unlimited, and
// TestSurvivorFetchIsUnboundedPerSender_PINNED_DEFECT stays green over exactly that.
// Narrowing is not bounding.
func TestRepairClaimBuysNoWorkWithoutTheEconomy(t *testing.T) {
	// measure delivers ONE honest-looking, unpunishable claim to a caretaker judge
	// and reports what it cost. The claim names the real manifest-committed shard id
	// (so the correctness leg cannot slash it) and a holder that does not hold the
	// shard (so the retrievability leg denies and nothing is paid) — the sender pays
	// nothing either way, which is what makes the cost pure amplification.
	measure := func(economy bool) (shards int, chunks int, bytes int64, votes int) {
		s := newRepairAdv(t, 1201)
		s.fundEscrow(5_000_000)
		judge := s.careJudge()
		judge.cfg.RepairEconomy = economy
		cs := newCountStore(judge.store)
		judge.store = cs

		pos, parityID, _ := s.parityTarget()
		attacker := s.nodes[2]
		s.bond(attacker)
		claim := repairClaimFor(s.root, 0, pos, parityID, s.nodes[7].ID())
		data, err := claim.Marshal()
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		before := s.net.Stats.Kinds[ports.MsgRepairVote]
		judge.handleRepairClaim(attacker.ID(), ports.Message{Kind: ports.MsgRepairClaim, Data: data})
		s.sched.Run()
		return s.shardFetches(cs), len(cs.distinct), cs.putBytes,
			s.net.Stats.Kinds[ports.MsgRepairVote] - before
	}

	offShards, offChunks, offBytes, offVotes := measure(false)
	onShards, onChunks, onBytes, onVotes := measure(true)

	t.Logf("economy OFF: %d shards, %d distinct chunks, %d B, %d vote(s)", offShards, offChunks, offBytes, offVotes)
	t.Logf("economy ON : %d shards, %d distinct chunks, %d B, %d vote(s)", onShards, onChunks, onBytes, onVotes)

	// THE CONTROL RUNS FIRST, because a gate that is never reached passes for free.
	// The economy-ON arm is simultaneously the positive control (a participating node
	// still judges) and the ABLATION (it is the pre-gate behaviour, since the gate's
	// only effect is to make OFF differ from ON). If this arm is cheap, the zero below
	// measures a broken fixture rather than a gate.
	if onShards == 0 || onBytes == 0 {
		t.Fatalf("CONTROL BROKEN: an economy-ON judge fetched %d shards / %d B for the same claim — it must still JUDGE, "+
			"or the OFF arm's zero is a dead fixture and not a gate. Check that newRepairAdv still stages a judgeable stripe.",
			onShards, onBytes)
	}

	// THE GATE. Zero shards, and zero bytes of ANY kind: the gate sits ahead of the
	// registry lookup, so the manifest chunk the position screen needs is not fetched
	// either. A non-zero chunk count with zero shards would mean the gate landed after
	// the manifest fetch — narrower than it should be, and worth failing on.
	if offShards != 0 || offChunks != 0 || offBytes != 0 {
		t.Fatalf("A JUDGE WITH THE ECONOMY OFF DID WORK FOR AN UNSIGNED CLAIM: %d shards, %d distinct chunks, %d B (want 0/0/0; the "+
			"economy-ON control paid %d shards / %d B for the same claim).\n"+
			"  IF THE SHARD COUNT IS %d, the gate is gone and handleRepairClaim runs the full survivor walk again: a 110-byte unsigned\n"+
			"  claim costs a node that cannot pay a bounty the whole stripe, on a path no honest paramedic can even discover it through\n"+
			"  (announceRepairQuorum never planted it under careKey).\n"+
			"  IF SHARDS IS 0 BUT CHUNKS IS NOT, the gate moved BELOW the registry lookup or the manifest fetch. Those are not free, and\n"+
			"  the reason to gate at the top is that an economy-off node has no honest claim traffic to serve at any depth.",
			offShards, offChunks, offBytes, onShards, onBytes, onShards)
	}

	// THE REFUSAL IS STILL ANSWERED. Every other deny on this path replies, and going
	// silent here would be a new behaviour rather than a narrowing: a paramedic that
	// somehow reached this node would wait on a reply that never comes instead of
	// learning it asked the wrong peer. One small frame out for one small frame in is
	// an amplification factor of about one, against the 42,909x the walk cost.
	if offVotes != 1 {
		t.Fatalf("the economy-off refusal emitted %d MsgRepairVote replies, want exactly 1 — a claim must be ANSWERED and not "+
			"dropped, or a caller cannot tell a refusing judge from an unreachable one", offVotes)
	}
}

// TestRepairClaimEconomyGateIsWhereTheOtherTwoAre asserts the SYMMETRY the gate
// above rests on, rather than leaving it as a claim in a comment. If a later change
// gates announce or emit differently — or ungates one of them — the argument that an
// economy-off node has no honest inbound claim traffic stops holding, and the gate
// above becomes a policy choice that needs re-deciding rather than a missing third
// case. This test is what makes that visible instead of silent.
func TestRepairClaimEconomyGateIsWhereTheOtherTwoAre(t *testing.T) {
	s := newRepairAdv(t, 1203)
	nd := s.nodes[3]
	nd.cfg.RepairEconomy = false

	// ANNOUNCE — an economy-off node must not become discoverable as a judge. The
	// assertion is on the careKey rendezvous specifically, not on a provider-table
	// count: being findable for the ROOT is ordinary storage behaviour, and being
	// findable under careKey(root) is what makes a node a caretaker-judge.
	key := careKey(s.root)
	selfListed := func() bool {
		for _, id := range nd.provs.IDs(ports.Hash(key)) {
			if id == nd.ID() {
				return true
			}
		}
		return false
	}
	if selfListed() {
		t.Fatal("PREMISE BROKEN: the node was already listed under careKey before announceRepairQuorum ran")
	}
	nd.announceRepairQuorum(s.root)
	s.sched.Run()
	if selfListed() {
		t.Fatal("announceRepairQuorum planted this node under careKey(root) with the economy OFF — it is now DISCOVERABLE as " +
			"a caretaker-judge it will refuse to act as, which is worse than either state alone, and it breaks the premise the " +
			"judging gate rests on (an economy-off node has no honest inbound claim traffic)")
	}

	// EMIT — an economy-off node must not send a claim of its own.
	pos, parityID, _ := s.parityTarget()
	claims := nd.Stats.RepairClaims
	nd.emitRepairClaim(s.root, shardRef{id: parityID, stripe: 0, pos: pos}, s.nodes[7].ID(), true)
	s.sched.Run()
	if nd.Stats.RepairClaims != claims {
		t.Fatalf("emitRepairClaim sent a claim with the economy OFF (RepairClaims %d -> %d)", claims, nd.Stats.RepairClaims)
	}
}
