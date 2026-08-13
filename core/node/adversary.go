package node

// RED-TEAM / TEST-HARNESS ONLY. This file holds DELIBERATELY BYZANTINE consensus
// behaviour a correct node NEVER performs — the double-sign that honest nodes refuse
// (handleChain / chainrole.go). It exists so the accountability property "a proven
// double-sign costs standing" (D2, #184) can be exercised over the REAL WIRE, not only
// in the in-process sim, and so an external adversary can drive the same attack against
// a deployment to check the defence holds. It is reached only through the daemon's
// loudly-announced `-equivocate` flag; no honest path calls it.

import (
	"fmt"

	"github.com/nerolabs/silt/core/chain"
	"github.com/nerolabs/silt/ports"
)

// advEntry is a synthetic registry entry an equivocating proposer puts in its
// conflicting blocks — distinct labels make the two same-height blocks genuinely
// different (so their hashes differ and the equivocation is provable).
func advEntry(label string) ports.Entry {
	return ports.Entry{
		Root:           ports.HashBytes([]byte("silt/adversary/" + label)),
		ManifestChunks: []ports.ChunkID{ports.HashBytes([]byte("silt/adversary/" + label + "/m"))},
	}
}

// proposeAndCommitTo drives a single PRE-SIGNED block into ONE target's replica: it
// sends the proposal, takes the target's attestation, staples it on, and commits the
// block to that target — WITHOUT appending to this node's own chain. That is the
// primitive an equivocator needs and an honest proposer lacks: it lets one node place
// a conflicting fork onto a specific peer. Adding attestations does not change a
// block's hash (attesters sign the header hash), so a block signed once here keeps a
// stable identity across the propose→commit pair.
//
// done reports whether the target actually COMMITTED the block (committed=true only on
// a fresh MsgCommitAck OK). Two gates can delay a placement, and the caller must retry
// through BOTH — which is why committed, not merely "the round-trip finished", is the
// latch signal (#378): (1) the PROPOSAL — a target that has not yet earned this node's
// standing refuses to attest; (2) the COMMIT — even after it attests, a target whose
// OWN attestation is not yet qualified from its local view cannot Append the block, so
// it acks not-OK. Under WAN delay these two warm-ups land at different times, so a leg
// routinely attests-but-doesn't-commit on one attempt and commits on a later one.
// Latching a leg "done" on the round-trip alone (the old best-effort commit) then
// stranded a fork whose next block could never extend an uncommitted parent — the
// second #378 wedge. A transport error is fatal; a not-OK commit ack is a RETRY, not a
// success.
func (n *Node) proposeAndCommitTo(b *chain.Block, target ports.NodeID, done func(committed bool, err error)) {
	raw := chain.Encode(b)
	n.request(target, ports.Message{Kind: ports.MsgProposeBlock, Data: raw}, func(resp ports.Message, err error) {
		if err != nil {
			done(false, err)
			return
		}
		if !resp.OK {
			done(false, fmt.Errorf("target %s refused the proposal at height %d (not yet standing?)", target, b.Height))
			return
		}
		att, aerr := attDecode(resp.Data)
		if aerr != nil {
			done(false, fmt.Errorf("decode attestation: %w", aerr))
			return
		}
		b.Atts = []chain.Attestation{att}
		n.request(target, ports.Message{Kind: ports.MsgCommitBlock, Data: chain.Encode(b)}, func(ack ports.Message, err2 error) {
			if err2 != nil {
				done(false, err2)
				return
			}
			done(ack.OK, nil) // OK=false ⇒ target attested but hasn't committed yet ⇒ caller retries
		})
	})
}

// ProposeBadBlock sends ONE crafted proposal to an honest target and reports whether
// the target REFUSED it — how #184 proves over the wire that honest validators reject a
// proposal that fails ValidateProposal before attesting. The block is built at the
// target's head with a valid entry, so the ONLY thing wrong is the fault under test:
//
//   - forge=true (forged-block→reject): the proposer signature is corrupted AFTER
//     signing, so ValidateProposal's ed25519.Verify fails — refused for a bad signature,
//     regardless of this node's standing.
//   - forge=false (low-bond→reject): the block is correctly signed and well-formed, so
//     the only reason to refuse is that this node is not a qualified (sufficiently
//     bonded) proposer — refused with ErrLowReputation.
//
// A correct node never proposes a forged block, and an honest under-bonded node's own
// pre-check stops it from proposing at all; this bypasses both so honest PEERS are the
// ones proving the defence. RED-TEAM / TEST-HARNESS ONLY — see the file header.
func (n *Node) ProposeBadBlock(target ports.NodeID, forge bool, done func(refused bool, err error)) {
	if n.chain == nil || n.signer == nil {
		done(false, ErrNoChain)
		return
	}
	prev, height := n.chain.Head()
	b := &chain.Block{Version: chain.BlockVersion, Height: height, Prev: prev, Entries: []ports.Entry{advEntry("badproposal")}}
	chain.Sign(b, n.signer)
	if forge && len(b.ProposerSig) > 0 {
		b.ProposerSig[0] ^= 0xFF // corrupt the signature so ValidateProposal's verify fails
	}
	n.request(target, ports.Message{Kind: ports.MsgProposeBlock, Data: chain.Encode(b)}, func(resp ports.Message, err error) {
		if err != nil {
			done(false, err)
			return
		}
		done(!resp.OK, nil) // OK:false = refused to attest = the defence held
	})
}

// ProposeGoodBlock is the POSITIVE CONTROL for the ProposeBadBlock rejections: a
// WELL-FORMED, correctly-signed block from THIS (properly-bonded) proposer, sent
// to target. An honest, working target ATTESTS it (OK:true). Without this control,
// a target that refuses EVERY proposal (chain role wedged, head mismatch, etc.)
// is indistinguishable from one correctly rejecting a forged/under-bonded block —
// so the forged-block / low-bond reject tests could pass on a broken target
// (audit #303). Callers gate those reject checks on this good proposal being
// ACCEPTED first, so a rejection is attributed to the real defence, not a dead node.
func (n *Node) ProposeGoodBlock(target ports.NodeID, done func(accepted bool, err error)) {
	if n.chain == nil || n.signer == nil {
		done(false, ErrNoChain)
		return
	}
	prev, height := n.chain.Head()
	b := &chain.Block{Version: chain.BlockVersion, Height: height, Prev: prev, Entries: []ports.Entry{advEntry("goodproposal")}}
	chain.Sign(b, n.signer)
	n.request(target, ports.Message{Kind: ports.MsgProposeBlock, Data: chain.Encode(b)}, func(resp ports.Message, err error) {
		if err != nil {
			done(false, err)
			return
		}
		done(resp.OK, nil) // OK:true = attested = the target accepts a well-formed, bonded proposal
	})
}

// equivPlan is the RESUMABLE state for the double-sign drill (#378). The three
// conflicting blocks are built ONCE against a pinned fork base and each leg's
// placement is latched, so a retry after a partial placement resumes only the
// un-placed legs. Height is the pinned double-sign height (for the log line).
type equivPlan struct {
	x, y, z             *chain.Block
	height              uint64
	xDone, yDone, zDone bool
}

// Equivocate makes this node DOUBLE-SIGN at a pinned height — the Byzantine act
// honest nodes refuse. It builds two DIFFERENT blocks at that height, both signed by
// this node as proposer, and places them on two different honest peers:
//
//   - the heavier fork Y@height then its extension Z@height+1 land on honestYZ, so
//     honestYZ = [g, Y, Z] with weight 2;
//   - the conflicting block X@height lands on honestX (honestX = [g, X], weight 1).
//
// When honestX later syncs the heavier fork from honestYZ it reconciles across the two
// histories, and chain.FindEquivocations catches this node signing X and Y at the same
// height — a self-verifying equivocation proof — and slashes it (chainrole.go). The
// weight ordering is deterministic (fork-choice is summed qualified-attester weight),
// so the detection is not a race. Requires this node to already hold consensus standing
// with both peers (so its proposals are accepted); callers retry until that is true.
//
// RESUMABLE PLACEMENT (#378, the structural close). Three wedges under WAN-ish delay,
// each closed here:
//
//  1. ErrDupRoot re-placement. The two peers earn this node's standing at INDEPENDENT
//     moments, so a first attempt routinely lands one leg and has the next refused. The
//     old driver rebuilt the blocks at the LIVE head every retry; once a leg committed
//     and this node synced it back (its head advanced), later attempts re-proposed the
//     same deterministic root at a new height — refused forever (ErrDupRoot). Fix: PIN
//     the fork base + blocks on the first call and LATCH each placed leg, so a committed
//     root is never re-proposed. (The #345 tip-targeting win is preserved: the base is
//     the live tip AT PIN TIME, so the drill still double-signs live on an advanced
//     chain — it just no longer chases the tip across retries.)
//
//  2. Attested-but-not-committed. A target attests on the PROPOSER's standing but commits
//     on its OWN attestation's qualification, which warms up later — so a leg can attest
//     and ack the commit not-OK. Latching on the round-trip alone stranded the next block
//     chasing an uncommitted parent. Fix: latch a leg only on a CONFIRMED commit
//     (proposeAndCommitTo's committed=true) and retry an attested-but-uncommitted one.
//
//  3. Detection disqualifies the proposer mid-drill. The moment an honest node holds BOTH
//     forks it slashes this node — which removes it from that node's qualified proposer
//     set, so any REMAINING placement on that node is refused "not yet standing" forever
//     (the property firing wedges the drill). Fix: place the COMPLETE heavier fork [Y,Z]
//     on honestYZ FIRST, then X on honestX LAST. No honest node can see both forks until
//     the final X placement, so detection cannot disqualify this node before every leg is
//     down; the slash then propagates asynchronously (honestX reconciles the heavier fork
//     and slashes), which is the property under test.
func (n *Node) Equivocate(honestX, honestYZ ports.NodeID, done func(error)) {
	if n.chain == nil || n.signer == nil {
		done(ErrNoChain)
		return
	}
	// Pin the fork base + build the three blocks ONCE; reuse across retries.
	if n.equivPlan == nil {
		prev, height := n.chain.Head()
		if height == 0 {
			done(fmt.Errorf("equivocate: no genesis"))
			return
		}
		x := &chain.Block{Version: chain.BlockVersion, Height: height, Prev: prev, Entries: []ports.Entry{advEntry("X")}}
		chain.Sign(x, n.signer)
		y := &chain.Block{Version: chain.BlockVersion, Height: height, Prev: prev, Entries: []ports.Entry{advEntry("Y")}}
		chain.Sign(y, n.signer)
		z := &chain.Block{Version: chain.BlockVersion, Height: height + 1, Prev: y.Hash(), Entries: []ports.Entry{advEntry("Z")}}
		chain.Sign(z, n.signer)
		n.equivPlan = &equivPlan{x: x, y: y, z: z, height: height}
	}
	p := n.equivPlan

	// Place the heavier fork Y → Z on honestYZ FIRST, then X on honestX LAST (wedge #3).
	// A leg latches ONLY on a CONFIRMED commit (wedge #2); a latched leg is never
	// re-proposed, so a committed root is never re-placed (wedge #1). Retries drive only
	// the un-placed legs against the SAME pinned base.
	if !p.yDone {
		n.proposeAndCommitTo(p.y, honestYZ, func(committed bool, err error) {
			if err != nil {
				done(fmt.Errorf("place Y on %s: %w", honestYZ, err))
				return
			}
			if !committed {
				done(fmt.Errorf("place Y on %s: attested but not yet committed (target warming up)", honestYZ))
				return
			}
			p.yDone = true
			n.Equivocate(honestX, honestYZ, done) // resume: Z next
		})
		return
	}
	if !p.zDone {
		n.proposeAndCommitTo(p.z, honestYZ, func(committed bool, err error) {
			if err != nil {
				done(fmt.Errorf("extend Z on %s: %w", honestYZ, err))
				return
			}
			if !committed {
				done(fmt.Errorf("extend Z on %s: attested but not yet committed (target warming up)", honestYZ))
				return
			}
			p.zDone = true
			n.Equivocate(honestX, honestYZ, done) // resume: X last
		})
		return
	}
	if !p.xDone {
		n.proposeAndCommitTo(p.x, honestX, func(committed bool, err error) {
			if err != nil {
				done(fmt.Errorf("place X on %s: %w", honestX, err))
				return
			}
			if !committed {
				done(fmt.Errorf("place X on %s: attested but not yet committed (target warming up)", honestX))
				return
			}
			p.xDone = true
			done(nil)
		})
		return
	}
	done(nil)
}

// EquivocateHeight is the pinned double-sign height once a drill has started (0
// before). Lets the driver narrate the true height instead of a hardcoded "1".
func (n *Node) EquivocateHeight() uint64 {
	if n.equivPlan == nil {
		return 0
	}
	return n.equivPlan.height
}
