package node

import (
	"bytes"
	"crypto/ed25519"
	"testing"

	"github.com/nerolabs/silt/adapters/identity"
	"github.com/nerolabs/silt/core/chain"
	"github.com/nerolabs/silt/ports"
)

// The #184 equivocation drivability oracle — OBJECTIVE mode, 3-of-4 (the cloud
// regime). PE follow-up ruling 2026-08-17: the wire drill's placement can't be
// commit-based (a 2-attestation single-target commit is quorum-short under
// RequiredQuorum=3, and no minority can commit a fork — I1), so it must be
// slash-on-DETECTION (chainrole.go:1085 seam-7): the crime is SIGNING two
// conflicting blocks at one height, not COMMITTING two forks.
//
// This oracle proves the detection mechanism end-to-end under the era-2 rules the
// cloud runs, pinning the two facts the wire primitive depends on:
//
//  1. **Era-2 slashability is a shared (round, phase) CONSENSUS signature**
//     (VerifyEquivocation → consensusSigScopes: PrepareQC/Atts vote slots; the
//     bare-hash ProposerSig is authorship, NOT a vote, and is excluded). So the
//     equivocator must have ATTESTED (prepared) both conflicting blocks at the
//     same slot — proposer authorship alone is not enough. This is why the wire
//     equivocator must be a SIGNER inside the consensus set, not an outside
//     non-anchor trying to earn proposer standing (the old drill's premise).
//  2. **Detection needs the honest side to HOLD an adversary-signed block at H
//     and to FETCH a conflicting adversary-signed fork at H** — neither has to
//     commit the fork. The honest committed block carries the adversary's real
//     prepare (it attested honestly); the served losing fork carries the
//     adversary's prepare over a DIFFERENT block at the same (H, round, prepare)
//     slot. slashEquivocators fires pre-Reconcile, independent of fork validity,
//     and never adopts the lighter fork (I5: an honest sequential signer is
//     never caught).
//
// Failing-first is structural: if VerifyEquivocation ever stopped treating a
// same-slot cross-fork prepare pair as slashable (or slashEquivocators stopped
// scanning served forks pre-adoption), the honest node would not slash and this
// fails. Ref: core/node/redteam_seam7_losing_fork_test.go (the era-1 / A=3
// ancestor); this is its era-2 / A=4 form, the wire drill's merge gate.
func TestModelCheck_184_ObjectiveEquivocationSlashedOnDetection(t *testing.T) {
	nodes, ids, net, refill := matureWorld(t)
	all := make([]ports.NodeID, len(ids))
	for i := range ids {
		all[i] = ids[i].NodeID()
	}
	signerByID := map[ports.NodeID]*identity.Identity{}
	for _, id := range ids {
		signerByID[id.NodeID()] = id
	}

	// Drive ONE honest commit at the head via the real gather (the designee
	// proposes, the cohort prepares+precommits) — a genuine era-2 committed block
	// W@H carrying real prepare signatures from >= the 3-of-4 quorum.
	refill()
	prev, h := nodes[0].chain.Head()
	desig := nodes[0].designatedProposer(h, 0)
	var proposer *Node
	for _, nd := range nodes {
		if nd.id == desig {
			proposer = nd
			break
		}
	}
	if proposer == nil {
		t.Fatalf("designated proposer for (h%d, r0) is not a fixture node", h)
	}
	W := &chain.Block{Version: chain.BlockVersionRounds, Height: h, Prev: prev, Entries: []ports.Entry{mkEntry("honest-win")}}
	var committed bool
	proposer.proposeBlock(W, all, all, 0, func(err error) { committed = err == nil })
	drainHeld(t, net, fifo)
	if !committed {
		t.Fatalf("h%d: honest commit did not land", h)
	}
	// The committed copy carries the real certificates.
	winChain := nodes[0].Chain().Blocks(0)
	committedW := &winChain[len(winChain)-1]
	if committedW.Height != h {
		t.Fatalf("head is h%d, expected the committed W at h%d", committedW.Height, h)
	}

	// Pick a CULPRIT: any validator whose PREPARE signature is in W's PrepareQC —
	// it attested honestly, so the honest chain holds its (H, round, prepare)
	// consensus signature. This models the wire equivocator: a bonded validator
	// INSIDE the consensus set (an anchor or epoch member), not an outsider.
	pubKeyOf := func(id *identity.Identity) []byte {
		return append([]byte(nil), id.Signer().Public().(ed25519.PublicKey)...)
	}
	var culprit *identity.Identity
	var culpritRound uint64
	for _, att := range committedW.PrepareQC {
		if att.Phase != chain.PhasePrepare {
			continue
		}
		for _, id := range ids {
			if bytes.Equal(att.PubKey, pubKeyOf(id)) {
				culprit = id
				culpritRound = att.Round
				break
			}
		}
		if culprit != nil {
			break
		}
	}
	if culprit == nil {
		t.Fatalf("no fixture validator's prepare is in the committed W.PrepareQC (len=%d) — cannot stage a same-slot cross-fork double-sign", len(committedW.PrepareQC))
	}

	// The adversary's OWN malicious act (the Byzantine equivocating broadcast): it
	// signs a prepare over a CONFLICTING block L at the SAME (H, round, prepare)
	// slot it just used for W. This is the un-bypassable self-incrimination.
	L := &chain.Block{Version: chain.BlockVersionRounds, Height: h, Prev: prev, Entries: []ports.Entry{mkEntry("adversary-fork")}}
	L.PrepareQC = []chain.Attestation{chain.AttestAt(L, culprit.Signer(), culpritRound, chain.PhasePrepare)}
	if L.Hash() == committedW.Hash() {
		t.Fatal("L must conflict with W (different hash) to be an equivocation")
	}
	// The served LOSING fork: W's ancestors with L in place of W at height h.
	servedFork := append(append([]chain.Block{}, winChain[:len(winChain)-1]...), *L)

	// An honest detector that did NOT author W's proposal (so it is a clean
	// third party) slashes the culprit on scanning the served fork against its
	// own held chain — pre-Reconcile, without adopting the lighter fork.
	var detector *Node
	for _, nd := range nodes {
		if nd.id != culprit.NodeID() && nd.id != proposer.id {
			detector = nd
			break
		}
	}
	var slashedID ports.NodeID
	var slashedHeight uint64
	detector.OnSlash(func(c ports.NodeID, ht uint64) { slashedID, slashedHeight = c, ht })

	lenBefore := detector.chain.Len()
	detector.slashEquivocators(detector.Chain().Blocks(0), servedFork)

	if slashedID != culprit.NodeID() {
		t.Fatalf("the cross-fork double-signer was NOT slashed on detection: got slash of %v, want culprit %v (era-2 same-slot prepare pair must be slashable)", slashedID, culprit.NodeID())
	}
	if slashedHeight != h {
		t.Fatalf("slash reported height %d, want %d", slashedHeight, h)
	}
	if detector.chain.Len() != lenBefore {
		t.Fatalf("the detector adopted the lighter served fork (len %d→%d) — detection must be pre-Reconcile, never an adoption of the loser", lenBefore, detector.chain.Len())
	}
}

// TestModelCheck_184_PlaceConflictingSignedSlashedOverSync exercises the WIRE
// primitive end-to-end (beyond the oracle's direct slashEquivocators call): the
// Byzantine node proposes the honest head W@H (its self-prepare lands on-chain),
// then PlaceConflictingSigned serves a conflicting L@H on GetChain; an honest peer
// that SYNCS the Byzantine node fetches the fork, slashes the double-sign, and
// does NOT adopt the invalid loser. This is the objective-mode equivocation
// drill's core mechanism (the dedicated-net drill, PE ruling D) proven in-process.
func TestModelCheck_184_PlaceConflictingSignedSlashedOverSync(t *testing.T) {
	nodes, ids, net, refill := matureWorld(t)
	all := make([]ports.NodeID, len(ids))
	for i := range ids {
		all[i] = ids[i].NodeID()
	}

	// The Byzantine validator PROPOSES the honest head at its designee height, so
	// its round-scoped self-prepare is guaranteed on-chain in W.PrepareQC
	// (requireProposerPrepare) — the signer the detection needs.
	refill()
	prev, h := nodes[0].chain.Head()
	desig := nodes[0].designatedProposer(h, 0)
	var byz *Node
	for _, nd := range nodes {
		if nd.id == desig {
			byz = nd
			break
		}
	}
	if byz == nil {
		t.Fatalf("designated proposer for (h%d, r0) is not a fixture node", h)
	}
	W := &chain.Block{Version: chain.BlockVersionRounds, Height: h, Prev: prev, Entries: []ports.Entry{mkEntry("honest-win")}}
	var committed bool
	byz.proposeBlock(W, all, all, 0, func(err error) { committed = err == nil })
	drainHeld(t, net, fifo)
	if !committed {
		t.Fatalf("h%d: honest commit did not land", h)
	}
	for _, nd := range nodes { // every replica syncs the honest W first (holds the Byzantine prepare)
		nd.SyncChain(all, func(int, error) {})
	}
	drainHeld(t, net, fifo)

	// The Byzantine act: serve a conflicting L@H at the same prepare slot.
	dsHeight, err := byz.PlaceConflictingSigned()
	if err != nil {
		t.Fatalf("PlaceConflictingSigned: %v", err)
	}
	if dsHeight != h {
		t.Fatalf("double-sign height %d, want %d", dsHeight, h)
	}

	// An honest peer (not the Byzantine node) syncs the Byzantine node and must
	// slash it on detecting the fork — without adopting the invalid loser.
	var detector *Node
	for _, nd := range nodes {
		if nd.id != byz.id {
			detector = nd
			break
		}
	}
	var slashedID ports.NodeID
	detector.OnSlash(func(c ports.NodeID, _ uint64) { slashedID = c })
	lenBefore := detector.chain.Len()
	detector.SyncChain([]ports.NodeID{byz.id}, func(int, error) {})
	drainHeld(t, net, fifo)

	if slashedID != byz.id {
		t.Fatalf("honest peer did not slash the equivocator over sync: got %v, want %v", slashedID, byz.id)
	}
	if detector.chain.Len() != lenBefore {
		t.Fatalf("the detector adopted the invalid served fork (len %d→%d) — the quorum-short loser must never be adopted", lenBefore, detector.chain.Len())
	}
}
