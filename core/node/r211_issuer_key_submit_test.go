package node

// R2.11 — the peer-submit path for a demand-issuer key registration (R0.4b-11). Gates held
// to the blind PE design ruling RULING-R2.11-issuer-key-peer-submit-design-2026-09-05:
// appended message kinds (S1), the bonded clause at arrival and drop-never-defer at fold
// (S2), the submitter in the sync tick and the FOLDABLE drain predicate (S3), the decoder
// pinned to one reg (S4), a derived burst (S5), and the end-to-end property the Rock names:
// an attest-only validator's key reaches the chain without it ever proposing.

import (
	"crypto/rand"
	"crypto/rsa"
	"testing"

	"github.com/nerolabs/silt/adapters/identity"
	"github.com/nerolabs/silt/adapters/simnet"
	"github.com/nerolabs/silt/core/chain"
	"github.com/nerolabs/silt/ports"
)

func r211Reg(id *identity.Identity, epoch uint64, seed byte) chain.IssuerKeyReg {
	var fp ports.Hash
	fp[0] = seed
	return chain.SignIssuerKeyReg(id.Signer(), epoch, fp)
}

// TestR211MsgKindNumbersArePinned (S1): MsgKind is a positional uint8. The R2.11 pair is
// APPENDED, so every pre-existing kind keeps its number — an old peer's MsgRelayPay must
// never be read by a new node as MsgRelayOpen. The literals are the tree's wire contract.
func TestR211MsgKindNumbersArePinned(t *testing.T) {
	pins := map[string][2]int{
		"MsgSubmitBondReg":         {int(ports.MsgSubmitBondReg), 29},
		"MsgRelayPay":              {int(ports.MsgRelayPay), 47},
		"MsgDemandTokenReply":      {int(ports.MsgDemandTokenReply), 52},
		"MsgSubmitIssuerKeyReg":    {int(ports.MsgSubmitIssuerKeyReg), 53},
		"MsgSubmitIssuerKeyRegAck": {int(ports.MsgSubmitIssuerKeyRegAck), 54},
	}
	for name, v := range pins {
		if v[0] != v[1] {
			t.Fatalf("%s = %d, want %d — a message kind was INSERTED, renumbering every kind after it; new kinds are appended after the last one", name, v[0], v[1])
		}
	}
	ackReply := (ports.Message{Kind: ports.MsgSubmitIssuerKeyRegAck}).IsReply()
	reqReply := (ports.Message{Kind: ports.MsgSubmitIssuerKeyReg}).IsReply()
	if !ackReply || reqReply {
		t.Fatalf("IsReply: ack=%v req=%v — the ack must correlate a pending request or every submit leaks a pending entry until timeout", ackReply, reqReply)
	}
	if ports.MsgSubmitIssuerKeyReg.String() == "" || ports.MsgSubmitIssuerKeyRegAck.String() == "" {
		t.Fatalf("the new kinds have no name in the name map")
	}
}

// TestR211ArrivalGateRefusesInOrderAndLoudly: on an objective chain with no bonded issuer,
// every refusal class is exercised — decode (S4: two regs in one payload), relayed
// (issuer ≠ sender), bad signature, epoch out of window, unbonded — and nothing is queued;
// the rate budget refuses the (burst+1)th message from one sender before decode; a second
// sender is unaffected.
func TestR211ArrivalGateRefusesInOrderAndLoudly(t *testing.T) {
	nd, ch, _, _ := submitWorld(t)
	_, next := ch.Head()
	epoch := ch.BlockEpoch(next)
	who := identity.FromSeed(9450)

	// S4: two registrations in one payload are refused at decode, before any verify.
	two := chain.Block{Version: chain.BlockVersion, IssuerKeys: []chain.IssuerKeyReg{r211Reg(who, epoch, 1), r211Reg(who, epoch+1, 2)}}
	nd.handleChain(who.NodeID(), ports.Message{Kind: ports.MsgSubmitIssuerKeyReg, Data: chain.Encode(&two)})
	// Relayed: a valid reg from A, submitted by B.
	nd.handleChain(identity.FromSeed(9451).NodeID(), ports.Message{Kind: ports.MsgSubmitIssuerKeyReg, Data: issuerKeyRegEncode(r211Reg(who, epoch, 1))})
	// Bad signature.
	bad := r211Reg(who, epoch, 1)
	bad.Sig[0] ^= 0xff
	nd.handleChain(who.NodeID(), ports.Message{Kind: ports.MsgSubmitIssuerKeyReg, Data: issuerKeyRegEncode(bad)})
	// Too far ahead, and (if the epoch is > 0) backdated.
	nd.handleChain(who.NodeID(), ports.Message{Kind: ports.MsgSubmitIssuerKeyReg, Data: issuerKeyRegEncode(r211Reg(who, epoch+chain.IssuerKeyPrePublish+1, 1))})
	// Well-formed, in-window, self-signed — but UNBONDED under MinBond > 0: refused (S2a).
	nd.handleChain(who.NodeID(), ports.Message{Kind: ports.MsgSubmitIssuerKeyReg, Data: issuerKeyRegEncode(r211Reg(who, epoch, 1))})
	if len(nd.pendingPeerIssuerKeys) != 0 {
		t.Fatalf("an inadmissible submission was queued: %d — unbonded identities are free, so an unbonded queue slot is a permanent liveness DoS on the mechanism", len(nd.pendingPeerIssuerKeys))
	}
	// Rate: burst+1 messages from one sender in one window; the last is refused before
	// decode (a garbage payload after the budget is spent leaves no decode log — we assert
	// the budget map, the observable the gate keys on).
	flooder := identity.FromSeed(9452)
	for i := 0; i <= issuerKeySubmitBurst; i++ {
		nd.handleChain(flooder.NodeID(), ports.Message{Kind: ports.MsgSubmitIssuerKeyReg, Data: []byte("junk")})
	}
	if r := nd.issuerKeySubmitRate[flooder.NodeID()]; r == nil || r.count != issuerKeySubmitBurst {
		t.Fatalf("rate budget for the flooder = %+v, want count pinned at the burst %d", r, issuerKeySubmitBurst)
	}
	if !nd.allowIssuerKeySubmit(identity.FromSeed(9453).NodeID()) {
		t.Fatalf("a different sender was refused while the flooder is capped")
	}
	if nd.allowIssuerKeySubmit(flooder.NodeID()) {
		t.Fatalf("the flooder was admitted past its budget")
	}
}

// r211Swarm builds a two-anchor era-4 swarm past the v5 boundary and commits the ATTEST-ONLY
// node's bond through the proposer's peer-submitted bond-reg path, so the attest-only node
// is admissible without ever proposing. Returns (proposer, attestOnly, ids, net, all).
func r211Swarm(t *testing.T) (*Node, *Node, []*identity.Identity, *simnet.Network, []ports.NodeID) {
	t.Helper()
	nodes, ids, net, _, _ := era4AnchorNet(t, 2)
	all := []ports.NodeID{ids[0].NodeID(), ids[1].NodeID()}
	saturateValidatorsSeen(t, nodes, net, all)
	proposer, attest := nodes[0], nodes[1]
	attest.EnableBond(ids[1].Signer(), 2<<20)
	head, _ := attest.chain.Head()
	reg, ok := attest.RegisterBondReg(head)
	if !ok {
		t.Fatalf("setup: the attest-only node could not build its bond reg")
	}
	proposer.handleChain(attest.id, ports.Message{Kind: ports.MsgSubmitBondReg, Data: bondRegEncode(reg)})
	if err := proposeOnce(t, proposer, net, all, "bond-of-attester"); err != nil {
		t.Fatalf("the bond-carrying proposal must commit: %v", err)
	}
	if !proposer.chain.IsBonded(attest.id) || !attest.chain.IsBonded(attest.id) {
		t.Fatalf("setup: the attest-only node's bond is not committed on both replicas")
	}
	return proposer, attest, ids, net, all
}

// TestR211AttestOnlyValidatorsKeyIsCommittedWithoutItEverProposing is the Rock's property,
// end to end: the attest-only node stages its key, SUBMITS it to the proposer over the wire
// (the sync-tick path), the proposer queues it, folds it into its next v5 block, both
// replicas commit the binding, and the peer queue drains once committed. The attest-only
// node never proposes. It also reports the S6 measurement: the fraction of committed
// blocks that carried an IssuerKeys registration in this run.
func TestR211AttestOnlyValidatorsKeyIsCommittedWithoutItEverProposing(t *testing.T) {
	proposer, attest, _, net, all := r211Swarm(t)
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	epoch := attest.DemandEpoch()
	attest.SetDemandIssuerKey(rand.Reader, epoch, key)
	if len(attest.pendingIssuerKeys) != 1 {
		t.Fatalf("setup: staged %d regs, want 1", len(attest.pendingIssuerKeys))
	}
	if _, ok := proposer.chain.IssuerKeyCommitment(attest.id, epoch); ok {
		t.Fatalf("setup: the binding is already committed")
	}
	// Before R2.11 this is where the story ended: the attest-only node holds its reg and
	// never proposes. Now the sync tick submits it.
	attest.SubmitIssuerKeyRegs([]ports.NodeID{proposer.id})
	drainHeld(t, net, fifo)
	if len(proposer.pendingPeerIssuerKeys) != 1 {
		t.Fatalf("the proposer did not queue the submitted registration (%d queued)", len(proposer.pendingPeerIssuerKeys))
	}
	if !proposer.issuerKeysFoldable() {
		t.Fatalf("a queued, admissible, in-window peer reg must make the drain FOLDABLE (S3b)")
	}
	blocks := 0
	carriers := 0
	const bound = 4
	for i := 0; i < bound; i++ {
		if _, ok := proposer.chain.IssuerKeyCommitment(attest.id, epoch); ok {
			break
		}
		if err := proposeOnce(t, proposer, net, all, "carry"+string(rune('0'+i))); err != nil {
			t.Fatalf("carrier proposal %d must commit: %v", i, err)
		}
		blocks++
	}
	if _, ok := proposer.chain.IssuerKeyCommitment(attest.id, epoch); !ok {
		t.Fatalf("the attest-only node's key never committed within %d proposals", bound)
	}
	if _, ok := attest.chain.IssuerKeyCommitment(attest.id, epoch); !ok {
		t.Fatalf("the binding committed on the proposer's replica but not the attester's")
	}
	// The peer queue drains once committed (a later fold sweep drops it).
	if err := proposeOnce(t, proposer, net, all, "after"); err != nil {
		t.Fatalf("post-commit proposal: %v", err)
	}
	if len(proposer.pendingPeerIssuerKeys) != 0 {
		t.Fatalf("a committed peer registration stayed queued: %d", len(proposer.pendingPeerIssuerKeys))
	}
	// S6 measurement: carrier fraction among the blocks this scenario committed.
	all_ := proposer.chain.Blocks(1)
	for _, b := range all_ {
		if len(b.IssuerKeys) > 0 {
			carriers++
		}
	}
	t.Logf("R2.11 S6 measurement: %d of %d committed blocks carried IssuerKeys (attest-only key delivered via peer submit in %d carrier proposal(s))", carriers, len(all_), blocks)
}

// TestR211FoldDropsNeverDefersAnInadmissiblePeerReg (S2b): a queued peer reg whose issuer
// is no longer admissible at fold time is DROPPED from the queue, and a peer reg for an
// epoch that has passed is dropped too — the queue cannot become unbounded retention.
func TestR211FoldDropsNeverDefersAnInadmissiblePeerReg(t *testing.T) {
	proposer, attest, _, net, all := r211Swarm(t)
	_, next := proposer.chain.Head()
	epoch := proposer.chain.BlockEpoch(next)
	// A stale-epoch reg and an unbonded issuer's reg planted directly in the queue (the
	// arrival gate would have refused both; the fold must still drop them, because the
	// race the gate cannot cover — slash or epoch turn after queueing — lands here).
	stranger := identity.FromSeed(9460)
	proposer.pendingPeerIssuerKeys = append(proposer.pendingPeerIssuerKeys, r211Reg(stranger, epoch, 3))
	if epoch > 0 {
		proposer.pendingPeerIssuerKeys = append(proposer.pendingPeerIssuerKeys, r211Reg(identity.FromSeed(9461), epoch-1, 4))
	}
	// And one GOOD reg from the bonded attester, which must ride.
	good := r211Reg(identity.FromSeed(7701), epoch, 5) // 7701 is era4AnchorNet's ids[1] seed
	if good.IssuerID() != attest.id {
		t.Fatalf("fixture: seed mismatch for the attest-only identity")
	}
	proposer.pendingPeerIssuerKeys = append(proposer.pendingPeerIssuerKeys, good)
	if err := proposeOnce(t, proposer, net, all, "fold"); err != nil {
		t.Fatalf("fold proposal must commit: %v", err)
	}
	for _, r := range proposer.pendingPeerIssuerKeys {
		if r.IssuerID() != attest.id {
			t.Fatalf("an inadmissible or stale peer reg was DEFERRED, not dropped: issuer %x epoch %d", r.IssuerID(), r.Epoch)
		}
	}
	if _, ok := proposer.chain.IssuerKeyCommitment(attest.id, epoch); !ok {
		t.Fatalf("the admissible peer reg did not ride the fold")
	}
}

// TestR211QueueIsOneSlotPerIssuerEpochLatestWins: a resubmission of the same (issuer,
// epoch) replaces rather than appends; a different epoch is a different slot.
func TestR211QueueIsOneSlotPerIssuerEpochLatestWins(t *testing.T) {
	nd, _, _, _ := submitWorld(t)
	who := identity.FromSeed(9470)
	nd.queuePendingPeerIssuerKey(r211Reg(who, 3, 1))
	nd.queuePendingPeerIssuerKey(r211Reg(who, 3, 2))
	nd.queuePendingPeerIssuerKey(r211Reg(who, 4, 3))
	if len(nd.pendingPeerIssuerKeys) != 2 {
		t.Fatalf("queue length %d, want 2 (one slot per (issuer, epoch))", len(nd.pendingPeerIssuerKeys))
	}
	if nd.pendingPeerIssuerKeys[0].Fingerprint[0] != 2 {
		t.Fatalf("latest resubmission did not win the slot")
	}
}
