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
//   - CORRECTNESS (deterministic, publicly recomputable): first SCREEN the claimed
//     position against the manifest alone, at zero network cost — a position the
//     manifest does not list for that stripe DENIES, and a listed position whose
//     committed id disagrees with claim.ShardID SLASHES. Only then fetch the
//     stripe's survivor shards — EVERY manifest-listed position except the one the
//     claim names, so n−1 on a full stripe and NOT k. There is no early exit once k
//     are in hand. Each survivor is verified against its own manifest-committed id,
//     then the claimed position is recomputed (repairproof.VerifyByRecompute, which
//     NEEDS k; the fetch is simply not budgeted to k).
//
//     ⚠ THE SCREEN IS THE ONLY LIVE ROUTE TO THE SLASH (credit.SlashFalseRepair).
//     This paragraph used to say a claim whose RECOMPUTE disagrees with the committed
//     shard id is a self-attributing lie. Corrected 2026-09-12: since the screen that
//     case has no reachable instance. Over the claimant's whole input set the screen
//     forces claim.ShardID == the manifest-committed id, fetchSurvivors hash-verifies
//     every survivor against its OWN committed id, and realData comes from the judge's
//     manifest — so correctnessOK is a function of the MANIFEST ALONE. A false return
//     from the recompute leg is a manifest-consistency failure (ReconstructStripe
//     failing, or the recomputed target mis-hashing — publisher faults), not a
//     claimant-attributable lie, even though repairproof.Decide still slashes it. The
//     lie is now caught EARLIER and MORE attributably, at zero network cost, by the
//     screen: the id comparison needs no survivors and cannot be confounded by a
//     publisher-inconsistent manifest. That is why the screen must SLASH and not merely
//     deny (D-BOUNTY-REPAIR-MECHANISM-GATED-2026-09-12, direction A) — a screen that
//     denied would land as a validation and silently RETIRE the punishment. The
//     recompute leg is also unreachable by an HONEST paramedic: repairStripe hash-checks
//     every ref of the stripe against its committed id and bails before emitRepairClaim,
//     so a self-inconsistent stripe never produces a claim at all.
//
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
//
// ADVERSARY-SHAPE: capability=UntrustedClaimFields UNCOVERED: no fixture GRANTS AND CONTROLS FOR 'never trusting the claim on its face'. NARROWED 2026-09-12: the already-paid half is CLOSED -- node-side (root, stripe, pos) dedup on PAID, asserted by TestRTRC2_ReplayedClaimForAPaidPositionDrawsNothing -- and the claimed position is now screened against the manifest before any fetch. What remains uncovered is that the position was ever LOST: TestRTRC3_ClaimWithNoLossIsPaid_PINNED_DEFECT drives attacker-chosen claim fields straight at the judge and pins that gap, its closer is GATED behind R-PROBE-FALSE-NEGATIVE-RATE, and it carries no control that removes the capability, so it is not declared as cover. ROADMAP row F1.
package node

import (
	"fmt"

	"github.com/nerolabs/silt/core/credit"
	"github.com/nerolabs/silt/core/crypto"
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
//
// ADVERSARY-SHAPE: capability=JudgeWithoutCareHandle UNCOVERED: no fixture GRANTS AND CONTROLS FOR a non-caretaker a CareHandle or a layout key it should not have. ROADMAP row F1.
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
	var targetRef shardRef
	targetListed := false
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
			continue
		}
		targetRef, targetListed = r, true
	}
	if len(stripeRefs) == 0 || realData == 0 {
		deny("no such stripe in this manifest")
		return
	}

	// POSITION SCREEN — the claimed position judged against the manifest ALONE,
	// before a single survivor is fetched. storedShards already built a ref for the
	// claimed position carrying its manifest-committed id; until 2026-09-12 the loop
	// above threw that ref away and nothing ever compared it to claim.ShardID.
	//
	// The two outcomes are deliberately DIFFERENT, and the difference is the whole
	// point of the screen (D-BOUNTY-REPAIR-MECHANISM-GATED-2026-09-12, direction A):
	//
	//   - A position the manifest does not list for this stripe is STRUCTURALLY
	//     IMPOSSIBLE — out of range, or implicit-zero padding that is never stored and
	//     never repaired. That is malformed input, so it DENIES, matching how every
	//     other malformed claim on this path is treated. It is also the case that
	//     never heals: VerifyByRecompute returns a structural error for it, the
	//     deferral path below cannot tell that from a transient short-survivor fetch,
	//     and the claim therefore cost the judge FOUR full stripe fetches — all n, one
	//     MORE than an honest claim, because an out-of-range position excludes no ref.
	//     Screened here it costs zero fetches and zero deferrals.
	//   - A WELL-FORMED position whose claimed id disagrees with the id the manifest
	//     commits to is a SELF-ATTRIBUTING LIE, and it is already slashable today
	//     through the recompute leg. So this screen must SLASH it, not merely deny it.
	//     A screen that denied would land as a validation and act as a silent RETIREMENT
	//     of an existing punishment, leaving silt strictly weaker against the exact
	//     adversary the slash was built for. The id comparison is MORE attributable than
	//     the recompute path, not less: it needs no survivors and cannot be confounded
	//     by a publisher-inconsistent manifest.
	if !targetListed {
		// No fetch, no deferral. Named separately from "no such stripe" so the journal
		// distinguishes a bad stripe from a bad position within a real stripe.
		deny(fmt.Sprintf("claimed position %d is not a stored position of stripe %d (out of range, or implicit-zero padding)",
			claim.ShardPos, claim.Stripe))
		return
	}
	if targetRef.id != claim.ShardID {
		n.logf(ports.LogWarn, "repair claim names a shard id the manifest does not commit at that position",
			"root", claim.Root, "stripe", claim.Stripe, "pos", claim.ShardPos,
			"claimed", claim.ShardID, "committed", targetRef.id, "claimant", from)
		n.settleRepairVerdict(from, claim, p, 0, 0, repairproof.Decision{Release: false, Slash: true})
		n.reply(from, msg, ports.Message{Kind: ports.MsgRepairVote, OK: false})
		return
	}

	// CORRECTNESS leg — fetch the survivors, verify each against its committed id,
	// recompute the claimed position and check it against the manifest anchor.
	//
	// ⚠ THE FETCH IS n−1 SHARDS ON A FULL STRIPE, NOT k. survivorRefs above is the
	// COMPLEMENT of claim.ShardPos over this stripe's manifest-listed positions, and
	// fetchSurvivors walks all of it — there is no early exit once k are in hand.
	// VerifyByRecompute needs k; nothing budgets the FETCH to k, and no per-sender
	// bound exists (D-REPAIR-RATE-LIMIT-REFUTED-2026-09-12 refuted the remedy on its
	// precondition). So one honest in-range claim still costs the judge n−1 fetches.
	// (Record correction 2026-09-12: the ratified entry
	// D-REPAIR-CLAIM-GATES-PINNED-2026-09-12 and this comment both said "k
	// survivors". Both were false in the same direction — they understated the
	// amplification the RT-RC-1 pin exists to hold.)
	//
	// WHAT THE POSITION SCREEN ABOVE DID CLOSE: the all-n case. An out-of-range
	// claim.ShardPos used to exclude no ref, fetch all n — one MORE than an honest
	// claim — and then error out of VerifyByRecompute into the deferral path below,
	// which retries a structurally impossible claim FOUR times because its predicate
	// is `cerr != nil` and cannot tell "too few survivors" (transient) from "target
	// out of range" (never heals). That claim now costs zero fetches and zero
	// deferrals. Every cerr that still reaches the deferral path is a genuine
	// short-survivor fetch, and with the short-final-stripe `present` fix in
	// repairproof it is genuinely transient: padding now counts toward k, so no
	// geometry is permanently unjudgeable.
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

		// shardBytes for the relative bounty price: every shard in a stripe is
		// equal-length, so any survivor's length is the shard size (PE Q3 — the base
		// scales with the erasure geometry, not a constant). The price FORMULA is
		// credit.RepairBountyBase's and is deliberately not restated in this package:
		// it moved at G-R212-7 and again at F1 (D-BOUNTY-PRICE-F1-2026-09-12), and
		// every restatement here went stale both times.
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

// bountyPosKey is the coordinate a durability bounty is paid AGAINST: one shard
// position of one stripe of one object. It is the key of Node.bountyPaid, and it
// is deliberately NOT the shard's content id — see that field's doc for why
// (R-SHARDID-ALIASES-POSITION).
type bountyPosKey struct {
	root   ports.Hash
	stripe int
	pos    int
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
	// DEDUP — a (root, stripe, position) is repaired once, so the second claim for
	// that position draws nothing. See Node.bountyPaid for why the record lives here
	// and not in the ledger, why it is keyed on the POSITION and not on claim.ShardID,
	// and why it is written on PAID and never on judged.
	pos := bountyPosKey{root: claim.Root, stripe: claim.Stripe, pos: claim.ShardPos}
	if n.bountyPaid[pos] {
		n.Stats.BountyDuplicatePosition++
		n.logf(ports.LogInfo, "repair bounty already paid for this position — claim draws nothing",
			"root", claim.Root, "stripe", claim.Stripe, "pos", claim.ShardPos,
			"shard", claim.ShardID, "holder", claim.Holder)
		return
	}
	{
		// Protocol price, relative to the erasure geometry (PE Q1/Q3): a repair is
		// worth the byte basis credit.RepairBountyCoeffNum/Den derives, scaled by the
		// rarest-shard multiplier, and credit.RepairBounty divides that whole product
		// into credits once.
		base := credit.RepairBountyBase(p.K, shardBytes)
		if base == 0 {
			// G-λ-8 (G-R212-7): the geometry is below one credit of fetch, so the bounty
			// is OFF for this object. A bounty silently off reads as a lost claim; name it
			// and count it — the daemon has no chunk geometry at start-up to refuse on.
			// The zero is read on the UNMULTIPLIED base, never on the price paid, so a
			// stripe near the cliff (whose multiplier can lift the price above zero) can
			// never mask a geometry that pays nothing on a healthy stripe (G-BT-2).
			n.Stats.BountyBaseZero++
			n.logf(ports.LogWarn, "repair bounty base is ZERO for this geometry", "root", claim.Root,
				"k", p.K, "shardBytes", shardBytes, "bytesPerCredit", int64(credit.DeliveryBytesPerCredit),
				"fix", "publish with -chunk-size >= "+fmt.Sprint(credit.MinBountyChunkBytesFor(p.K, crypto.Overhead)))
		}
		// The whole price in ONE division, at the END: ⌊basis·(lost+1)/(U/p)⌋, not
		// ⌊basis/(U/p)⌋·(lost+1) (G-BT-2), with `basis` the per-repair byte quantity
		// credit.RepairBountyCoeffNum/Den derives. Flooring before the multiplier threw
		// away up to (n−k+1)−1 credits of the repairer's wage on every rare-stripe repair.
		bounty := credit.RepairBounty(p.K, p.N, reachable, shardBytes)
		paid := n.ledger.PayBounty(claim.Root, claim.Holder, bounty)
		if paid == 0 {
			// A release that pays NOTHING is an empty escrow on THIS judge's
			// ledger — narrate it, or paid=0 is unattributable (#518).
			n.logf(ports.LogWarn, "repair bounty release paid nothing — escrow empty on this judge",
				"root", claim.Root, "holder", claim.Holder, "bounty", bounty)
			return
		}
		if paid > 0 {
			// Record the position ONLY here, where a payment actually happened. The
			// paid == 0 arm above returned without recording on purpose: an empty
			// escrow is not a payment, and the position stays claimable when the
			// object is re-endowed.
			n.bountyPaid[pos] = true
			n.Stats.BountiesReleased++
			if claim.Holder == n.id {
				// A4-2: this judge just paid a bounty to itself. Counted where `paid`
				// returns and both ids are in hand. An honest judge never gets here.
				n.Stats.BountyPaidToSelf++
				n.Stats.BountyCreditsPaidToSelf += paid
			}
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
//
// ADVERSARY-SHAPE: capability=CaretakerDiscoveryWithoutCareKey UNCOVERED: no fixture GRANTS AND CONTROLS FOR an adversary the caretaker set by walking to the root instead of the careKey rendezvous. ROADMAP row F1.
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
//
// ADVERSARY-SHAPE: NOT-A-DEFENCE: 'a claim the holder could never satisfy is never worth emitting' is an emit-side economy rule about the HONEST paramedic's own behaviour. It asserts no incapability of any adversary, and suppressing an emit an adversary would not make is not a defence.
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
