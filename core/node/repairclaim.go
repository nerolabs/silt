// H7 proof-of-correct-repair — the node/network wiring (design doc §6, §8, §8b).
//
// A caretaker that rebuilds a lost shard and pushes it to a fresh holder emits a
// MsgRepairClaim (repairStripe, repair.go). This file is the OTHER side: a
// caretaker-judge that receives such a claim and independently decides whether to
// release the object's durability bounty to the holder — never trusting the claim
// on its face.
//
// The judge runs the two legs the pure core/repairproof package defines, split by
// their trust properties (design §6):
//
//   - CORRECTNESS (deterministic, publicly recomputable): fetch k survivor shards
//     of the stripe — verifying each against its own manifest-committed id — and
//     recompute the claimed position (repairproof.VerifyByRecompute). A claim whose
//     recompute disagrees with the committed shard id is a self-attributing lie, so
//     it is SLASHED (credit.SlashFalseRepair), not merely denied.
//   - RETRIEVABILITY (where independent verifiers add value): challenge the named
//     holder with an identity-bound Shacham–Waters PoR (repairproof.RepairChallengeSeed
//     closes the relay/double-count), so a data-less relay can't collect. A
//     retrievability shortfall DENIES the bounty but does not slash — it may be
//     transient.
//
// The verdict is applied to THIS node's own ledger (PayBounty the holder /
// SlashFalseRepair the claimant): credit is per-node-local accounting, so each
// caretaker-judge settles independently and the τ-of-q quorum is the emergent
// property that τ honest judges independently reach release. Nothing here mints
// standing — PayBounty is `neutral`, SlashFalseRepair is `reduces` — so the γ→1/N
// firewall holds (the one load-bearing invariant, core/credit/invariant_a_test.go).
package node

import (
	"fmt"

	"github.com/nerolabs/silt/core/credit"
	"github.com/nerolabs/silt/core/erasure"
	"github.com/nerolabs/silt/core/link"
	"github.com/nerolabs/silt/core/manifest"
	"github.com/nerolabs/silt/core/pipeline"
	"github.com/nerolabs/silt/core/por"
	"github.com/nerolabs/silt/core/repairproof"
	"github.com/nerolabs/silt/ports"
)

// handleRepairClaim is the caretaker-judge for an inbound MsgRepairClaim. It runs
// the correctness + retrievability legs, settles the verdict on the local ledger,
// and replies MsgRepairVote{OK} with whether it released the bounty. A node that
// isn't a caretaker of the claimed object (no matching CareHandle → no layout key)
// cannot judge and replies OK=false without side effects.
func (n *Node) handleRepairClaim(from ports.NodeID, msg ports.Message) {
	// Every deny NAMES ITS REASON in the journal (#518 capture lesson: a claim
	// chain that dies in a silent deny leaves paid=0 unattributable — the
	// judge is an economy actor and its verdicts must be evidence-carrying).
	var claim repairproof.RepairClaim
	deny := func(reason string) {
		n.logf(ports.LogInfo, "repair claim denied", "reason", reason,
			"root", claim.Root, "shard", claim.ShardID, "claimant", from)
		n.reply(from, msg, ports.Message{Kind: ports.MsgRepairVote, OK: false})
	}

	claim, err := repairproof.UnmarshalClaim(msg.Data)
	if err != nil {
		deny("malformed claim")
		return
	}
	n.logf(ports.LogDebug, "repair claim received",
		"root", claim.Root, "shard", claim.ShardID, "holder", claim.Holder, "claimant", from)
	ch, ok := n.careHandleFor(claim.Root)
	if !ok || n.reg == nil {
		deny("not a caretaker of this root, or no registry")
		return
	}
	n.lookupEntryAsync(n.reg, claim.Root, func(entry ports.Entry, ok bool, err error) {
		if err != nil || !ok {
			deny("registry lookup failed")
			return
		}
		// (Re)acquire the manifest — mostly a local cache hit for a caretaker — then
		// judge on the LAYOUT alone (the content keys stay sealed, M11).
		n.fetchAll(entry.ManifestChunks, func(missing []ports.ChunkID) {
			if len(missing) > 0 {
				deny("manifest unreachable (transient)")
				return
			}
			n.judgeRepairClaim(from, msg, claim, ch, entry, 0)
		})
	})
}

// judgeRetryAttempts bounds the deferred re-judgments of a transiently
// unjudgeable claim (#518): three attempts spaced HolderCooldown apart span
// the negative-cache window and its first decay doubling.
const judgeRetryAttempts = 3

// judgeRepairClaim runs the two legs once the manifest is in hand and settles.
func (n *Node) judgeRepairClaim(from ports.NodeID, msg ports.Message, claim repairproof.RepairClaim, ch link.CareHandle, entry ports.Entry, attempt int) {
	deny := func(reason string) {
		n.logf(ports.LogInfo, "repair claim denied", "reason", reason,
			"root", claim.Root, "shard", claim.ShardID, "claimant", from)
		n.reply(from, msg, ports.Message{Kind: ports.MsgRepairVote, OK: false})
	}

	m, err := pipeline.LoadLayout(bg(), n.store, entry, ch)
	if err != nil || m.K == 0 || len(m.Chunks) == 0 {
		deny("layout not loadable")
		return
	}
	p := erasure.Params{K: m.K, N: m.N}
	refs := storedShards(m, p)

	// Gather this stripe's shard refs: the target the claim names, its real-data
	// count, and every OTHER position as a candidate survivor.
	var stripeRefs, survivorRefs []shardRef
	realData := 0
	for _, r := range refs {
		if r.stripe != claim.Stripe {
			continue
		}
		stripeRefs = append(stripeRefs, r)
		if r.pos < p.K {
			realData++
		}
		if r.pos != claim.ShardPos {
			survivorRefs = append(survivorRefs, r)
		}
	}
	if len(stripeRefs) == 0 || realData == 0 {
		deny("no such stripe in this manifest")
		return
	}

	// CORRECTNESS leg — fetch k survivors, verify each against its committed id,
	// recompute the claimed position and check it against the manifest anchor.
	n.fetchSurvivors(m.Root(), survivorRefs, func(survivors map[int][]byte, reachable int) {
		correctnessOK, cerr := repairproof.VerifyByRecompute(p, survivors, realData, claim.ShardPos, claim.ShardID)
		if cerr != nil {
			// Structurally un-judgeable — usually TOO FEW SURVIVORS, and usually
			// TRANSIENT: a claim arrives moments after the repair-time fetch storm,
			// when live-but-slow holders sit freshly stamped in the negative cache
			// (a single 2s holder dial misses under load) and the judge's own
			// working set was just dropped. Claim emission is one-shot, so a
			// terminal deny here loses the bounty FOREVER for a 30s condition —
			// the captured #518 sub-mode (survivors fetched=2..5 of k=10, 4ms
			// after the judge's own rebuild). DEFER instead: re-judge after
			// HolderCooldown (the duration of the very transient being waited
			// out), bounded; deny with the reason only when retries exhaust.
			// Never a slash — not an attributable lie either way.
			if attempt < judgeRetryAttempts {
				n.logf(ports.LogInfo, "repair claim deferred — survivors transiently short",
					"root", claim.Root, "shard", claim.ShardID,
					"fetched", len(survivors), "need", p.K, "attempt", attempt+1)
				n.clock.AfterFunc(n.cfg.HolderCooldown, func() {
					n.judgeRepairClaim(from, msg, claim, ch, entry, attempt+1)
				})
				return
			}
			deny(fmt.Sprintf("unjudgeable: %v (survivors fetched=%d of k=%d needed, %d attempts)", cerr, len(survivors), p.K, attempt+1))
			return
		}

		// shardBytes for the relative bounty price (base = c·k·shardBytes): every
		// shard in a stripe is equal-length, so any survivor's length is the shard
		// size (PE Q3 — the base scales with the erasure geometry, not a constant).
		shardBytes := int64(0)
		for _, s := range survivors {
			shardBytes = int64(len(s))
			break
		}

		// RETRIEVABILITY leg — challenge the named holder under an identity-bound
		// seed. `reachable` counts survivors alive PLUS the freshly-placed target,
		// feeding the rarest-shard bounty multiplier.
		n.challengeHolderRetrievability(m, ch, claim, func(retrOK bool) {
			n.logf(ports.LogDebug, "repair claim holder challenge",
				"root", claim.Root, "holder", claim.Holder, "ok", retrOK)
			decision := repairproof.Decide(correctnessOK, []bool{retrOK}, n.cfg.RepairQuorumTau)
			n.settleRepairVerdict(from, claim, p, shardBytes, reachable+1, decision)
			n.reply(from, msg, ports.Message{Kind: ports.MsgRepairVote, OK: decision.Release})
		})
	})
}

// settleRepairVerdict applies the verdict to THIS node's local ledger: release
// pays the holder from the object's escrow (capped by the rarest-shard multiplier),
// a correctness lie slashes the claimant. Both are balance/standing motions on the
// local ledger only — never consensus state (design §8b).
func (n *Node) settleRepairVerdict(claimant ports.NodeID, claim repairproof.RepairClaim, p erasure.Params, shardBytes int64, reachable int, d repairproof.Decision) {
	if n.ledger == nil {
		return
	}
	if d.Slash {
		n.ledger.SlashFalseRepair(claimant)
		n.Stats.FalseRepairSlashes++
		n.logf(ports.LogWarn, "false repair claim slashed", "root", claim.Root, "claimant", claimant, "shard", claim.ShardID)
		return
	}
	// The OFF path is a true no-op (PE merge gate): economy off ⇒ no bounty
	// disburses, even though escrows still fill via the serve auto-skim.
	if !n.cfg.RepairEconomy {
		return
	}
	if !d.Release {
		// Verified but not released (retrievability shortfall past tau): name it —
		// a silent non-release reads as a lost claim in the journal (#518).
		n.logf(ports.LogInfo, "repair claim not released", "root", claim.Root,
			"shard", claim.ShardID, "holder", claim.Holder)
		return
	}
	{
		// Protocol price, relative to the erasure geometry (PE Q1/Q3): a repair is
		// worth c·k·shardBytes, scaled up by BountyFor's rarest-shard multiplier.
		base := credit.RepairBountyBase(p.K, shardBytes)
		bounty := credit.BountyFor(base, p.K, p.N, reachable)
		paid := n.ledger.PayBounty(claim.Root, claim.Holder, bounty)
		if paid == 0 {
			// A release that pays NOTHING is an empty escrow on THIS judge's
			// ledger — narrate it, or paid=0 is unattributable (#518).
			n.logf(ports.LogWarn, "repair bounty release paid nothing — escrow empty on this judge",
				"root", claim.Root, "holder", claim.Holder, "bounty", bounty)
			return
		}
		if paid > 0 {
			n.Stats.BountiesReleased++
			// Narrate the funded horizon and the realised cost-per-repair (the g
			// input) so an operator watches the finite-but-renewable reserve draw
			// down, not just an opaque payment (D-S7; acceptance F7-style).
			snap := n.ledger.DurabilitySnapshot(claim.Root)
			n.logf(ports.LogInfo, "repair bounty released",
				"root", claim.Root, "holder", claim.Holder, "paid", paid,
				"reserve", snap.Balance, "cost/repair", credit.CostPerRepair(snap))
		}
	}
}

// fetchSurvivors pulls the stripe's survivor shards by column into the store,
// verifies each against its committed id, and returns the verified bytes keyed by
// stripe position plus the count reachable. It is a paramedic, not a hoarder:
// copies it did not already host are dropped afterwards, so judging a claim never
// silently turns a judge into a holder.
func (n *Node) fetchSurvivors(root ports.Hash, refs []shardRef, done func(survivors map[int][]byte, reachable int)) {
	heldBefore := make(map[ports.ChunkID]bool, len(refs))
	for _, r := range refs {
		if ok, _ := n.store.Has(bg(), r.id); ok {
			heldBefore[r.id] = true
		}
	}
	n.fetchStripeByColumn(root, refs, func(_ []ports.ChunkID, _ map[uint64]int) {
		survivors := make(map[int][]byte, len(refs))
		for _, r := range refs {
			c, err := n.store.Get(bg(), r.id)
			if err != nil {
				continue
			}
			// Trust a survivor only as far as its bytes hash to the id the manifest
			// committed — a survivor whose bytes don't match isn't a survivor.
			if ports.HashBytes(c.Data) == r.id {
				survivors[r.pos] = c.Data
			}
		}
		for _, r := range refs {
			if !heldBefore[r.id] {
				n.dropHosted(r.id) // drop only what we fetched for this judgement
			}
		}
		done(survivors, len(survivors))
	})
}

// challengeHolderRetrievability issues one identity-bound SW PoR challenge to the
// claim's holder for the repaired shard and reports whether it verifies: the
// holder must present a Merkle proof binding the shard to the root, the committed
// full block count, and an aggregated response that satisfies the equation under a
// seed bound to the holder's own identity (so a relayed proof fails).
func (n *Node) challengeHolderRetrievability(m *manifest.Layout, ch link.CareHandle, claim repairproof.RepairClaim, done func(bool)) {
	porKey := DerivePorKey(ch.LayoutKey)
	want := por.DefaultParams.Blocks(int(m.ChunkSize) + ctOverhead)
	n.rid++ // fresh deterministic base nonce, as the audit path draws one
	base := porChallengeSeed(n.rid)
	seed := repairproof.RepairChallengeSeed(base, claim.Holder)
	n.request(claim.Holder, ports.Message{
		Kind: ports.MsgChallenge, ChunkID: claim.ShardID,
		PorSeed: seed[:], PorCount: porSampleCount,
	}, func(resp ports.Message, err error) {
		if err != nil || !resp.Found || resp.Proof == nil ||
			!verifyStorageProof(*resp.Proof, claim.ShardID) || !blocksOK(resp.PorBlocks, want) {
			done(false)
			return
		}
		ok := repairproof.VerifyRetrievability(porKey, claim.ShardID[:], claim.Holder, base,
			want, porSampleCount, por.Proof{Mu: resp.PorMu, Sigma: resp.PorSigma})
		done(ok)
	})
}

// careKey is the rendezvous DHT key the caretakers of an object register under:
// hash(root ‖ "silt/care/v1"). It is how the quorum becomes DISCOVERABLE — only a
// care-link holder can judge a claim (it needs the layout key), and caretakers
// cluster near the manifest-chunk keys, not the root, so a repairer can't find
// them by walking to the root. Each caretaker announces itself here (Care), and a
// repairer resolves this key to reach the quorum. The domain separator keeps it
// from ever colliding with a real chunk hash's preimage.
func careKey(root ports.Hash) ports.Hash {
	buf := make([]byte, 0, len(root)+len("silt/care/v1"))
	buf = append(buf, root[:]...)
	buf = append(buf, "silt/care/v1"...)
	return ports.HashBytes(buf)
}

// announceRepairQuorum makes this node discoverable as a caretaker-judge of root,
// by planting a provider record under careKey(root) on the nodes near that key. A
// no-op unless the bounty economy is on — non-bounty sims keep their exact prior
// traffic. Called from Care.
func (n *Node) announceRepairQuorum(root ports.Hash) {
	if !n.cfg.RepairEconomy {
		return
	}
	key := careKey(root)
	n.provs.Add(n.providerRecord(key)) // findable locally too
	n.announceAll([]ports.ChunkID{ports.ChunkID(key)}, func() {})
}

// emitRepairClaim is the claim-emit hook (design §8b): after a caretaker rebuilds
// a lost shard and places it on a fresh holder, it sends a MsgRepairClaim naming
// that holder to the object's caretaker quorum — resolved via the careKey
// rendezvous, so it reaches nodes that actually hold the layout key and can judge.
// Each judge verifies the two legs and settles the bounty on its own ledger; the
// paramedic keeps nothing and does not settle its own ledger from the claim (the
// holder is the payee).
//
// No-op unless the bounty economy is enabled (cfg.RepairEconomy) and the shard
// carries PoR tags (porKey != nil ⇒ the holder can answer the retrievability leg);
// a claim the holder could never satisfy is never worth emitting.
func (n *Node) emitRepairClaim(root ports.Hash, r shardRef, holder ports.NodeID, hasTags bool) {
	if !n.cfg.RepairEconomy || !hasTags || holder == (ports.NodeID{}) {
		return
	}
	claim := repairproof.RepairClaim{
		Root: root, Stripe: r.stripe, ShardPos: r.pos, ShardID: r.id, Holder: holder,
	}
	data, err := claim.Marshal()
	if err != nil {
		return
	}
	n.Stats.RepairClaims++
	n.resolveProviders(ports.ChunkID(careKey(root)), func(quorum []ports.NodeID) {
		sent := 0
		for _, t := range quorum {
			if t == n.id || t == holder {
				continue // the paramedic doesn't judge its own claim; the holder isn't a judge
			}
			n.request(t, ports.Message{Kind: ports.MsgRepairClaim, Data: data},
				func(ports.Message, error) {}) // fire-and-forget: each judge settles its own ledger
			sent++
		}
		// A claim with no eligible judge dies silently and no bounty can ever
		// pay — reachable with a 2-caretaker quorum whenever the rebuilt shard
		// landed ON the other caretaker (self and holder are both excluded).
		// Narrate it: judge starvation must be a named event in the journal,
		// not an unexplained paid=0.
		if sent == 0 {
			n.logf(ports.LogWarn, "repair claim found no eligible judge",
				"root", root, "shard", r.id, "holder", holder, "quorum", len(quorum))
		}
	})
}

// careHandleFor returns this node's care-link for root, if it is a caretaker of
// it — the layout key that gates verification.
func (n *Node) careHandleFor(root ports.Hash) (link.CareHandle, bool) {
	for _, ch := range n.care {
		if ch.Root == root {
			return ch, true
		}
	}
	return link.CareHandle{}, false
}
