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
	"os"
	"strings"
	"testing"

	"github.com/nerolabs/silt/adapters/identity"
	"github.com/nerolabs/silt/adapters/memstore"
	"github.com/nerolabs/silt/adapters/simclock"
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

// TestR211ArrivalGateRefusesInOrderAndLoudly (F1): every refusal class is exercised against
// an issuer that is BONDED at the receiver, so the bonded clause (last in the order) cannot
// mask the clause under test — decode (S4: two regs in one payload), relayed (issuer ≠
// sender), bad signature, epoch out of window, already committed after it lands, and the
// unbonded clause with a stranger. Nothing inadmissible is queued; the good reg is. Then the
// rate budget: burst+1 messages from one sender in one window; a second sender proceeds.
func TestR211ArrivalGateRefusesInOrderAndLoudly(t *testing.T) {
	proposer, attest, ids, _, _ := r211Swarm(t)
	_, next := proposer.chain.Head()
	epoch := proposer.chain.BlockEpoch(next)
	me := ids[1] // the BONDED attest-only identity
	good := r211Reg(me, epoch, 1)
	send := func(from ports.NodeID, raw []byte) {
		proposer.handleChain(from, ports.Message{Kind: ports.MsgSubmitIssuerKeyReg, Data: raw})
	}
	// S4: two registrations in one payload — refused at decode, before any verify.
	two := chain.Block{Version: chain.BlockVersion, IssuerKeys: []chain.IssuerKeyReg{good, r211Reg(me, epoch+1, 2)}}
	send(attest.id, chain.Encode(&two))
	// Relayed: the bonded issuer's valid reg submitted by SOMEONE ELSE.
	send(identity.FromSeed(9451).NodeID(), issuerKeyRegEncode(good))
	// Bad signature, from the right sender.
	bad := good
	bad.Sig = append([]byte(nil), good.Sig...)
	bad.Sig[0] ^= 0xff
	send(attest.id, issuerKeyRegEncode(bad))
	// Too far ahead; and backdated when the epoch allows it.
	send(attest.id, issuerKeyRegEncode(r211Reg(me, epoch+chain.IssuerKeyPrePublish+1, 1)))
	if epoch > 0 {
		send(attest.id, issuerKeyRegEncode(r211Reg(me, epoch-1, 1)))
	}
	// Unbonded stranger: well-formed, in-window, self-signed — refused (S2a).
	stranger := identity.FromSeed(9450)
	send(stranger.NodeID(), issuerKeyRegEncode(r211Reg(stranger, epoch, 1)))
	if len(proposer.pendingPeerIssuerKeys) != 0 {
		t.Fatalf("an inadmissible submission was queued: %d (%+v)", len(proposer.pendingPeerIssuerKeys), proposer.pendingPeerIssuerKeys)
	}
	// The good one: bonded issuer, own sender, valid, in-window — queued.
	send(attest.id, issuerKeyRegEncode(good))
	if len(proposer.pendingPeerIssuerKeys) != 1 {
		t.Fatalf("the admissible registration was not queued (%d)", len(proposer.pendingPeerIssuerKeys))
	}
	// Rate: burst+1 messages from one sender in one window; the last is refused before
	// decode. A different sender is unaffected.
	flooder := identity.FromSeed(9452)
	for i := 0; i <= issuerKeySubmitBurst; i++ {
		send(flooder.NodeID(), []byte("junk"))
	}
	if r := proposer.issuerKeySubmitRate[flooder.NodeID()]; r == nil || r.count != issuerKeySubmitBurst {
		t.Fatalf("rate budget for the flooder = %+v, want count pinned at the burst %d", r, issuerKeySubmitBurst)
	}
	if !proposer.allowIssuerKeySubmit(identity.FromSeed(9453).NodeID()) {
		t.Fatalf("a different sender was refused while the flooder is capped")
	}
	if proposer.allowIssuerKeySubmit(flooder.NodeID()) {
		t.Fatalf("the flooder was admitted past its budget")
	}
}

// TestR211UnbondedFloodThenHonestBondedSubmit (§7 gate 2): two hundred fresh unbonded
// identities each submit a valid self-signed reg; none is queued; the honest bonded
// submitter that follows is queued. The bonded clause, not the per-sender budget, is what
// bounds distinct senders (a fresh keypair is a fresh budget).
func TestR211UnbondedFloodThenHonestBondedSubmit(t *testing.T) {
	proposer, attest, ids, _, _ := r211Swarm(t)
	_, next := proposer.chain.Head()
	epoch := proposer.chain.BlockEpoch(next)
	for i := 0; i < 200; i++ {
		who := identity.FromSeed(int64(20000 + i))
		proposer.handleChain(who.NodeID(), ports.Message{Kind: ports.MsgSubmitIssuerKeyReg, Data: issuerKeyRegEncode(r211Reg(who, epoch, 1))})
	}
	if len(proposer.pendingPeerIssuerKeys) != 0 {
		t.Fatalf("%d unbonded registrations were queued", len(proposer.pendingPeerIssuerKeys))
	}
	proposer.handleChain(attest.id, ports.Message{Kind: ports.MsgSubmitIssuerKeyReg, Data: issuerKeyRegEncode(r211Reg(ids[1], epoch, 1))})
	if len(proposer.pendingPeerIssuerKeys) != 1 {
		t.Fatalf("the honest bonded submit after the flood was not queued")
	}
}

// TestR211ArrivalGateIsInertOffTheObjectivePath (S2a pin): the whole handler sits inside
// Objective(), where IssuerKeyRegAdmissible is exactly bonded[issuer] > 0. Off that path the
// admissibility clause is true for everyone, so the distinct-sender bound would evaporate;
// therefore a non-objective chain queues NOTHING from a peer submit.
func TestR211ArrivalGateIsInertOffTheObjectivePath(t *testing.T) {
	sched := simclock.New()
	net := simnet.New(sched, 5, simnet.DefaultConfig())
	id := identity.FromSeed(9480)
	g := &chain.Block{Version: 1, Height: 0, Entries: []ports.Entry{mkEntry("g-legacy")}}
	chain.Sign(g, id.Signer())
	nd := New(id.NodeID(), DefaultConfig(), sched, net.Endpoint(id.NodeID()), memstore.New())
	ch := chain.New(chain.Config{Quorum: 1}, func(ports.NodeID) int64 { return 1 << 30 }) // MinBond 0: NOT objective
	if err := ch.AppendGenesis(*g); err != nil {
		t.Fatal(err)
	}
	nd.EnableChain(ch, id.Signer())
	if ch.Objective() {
		t.Fatal("fixture: the chain must be non-objective")
	}
	who := identity.FromSeed(9481)
	_, next := ch.Head()
	nd.handleChain(who.NodeID(), ports.Message{Kind: ports.MsgSubmitIssuerKeyReg, Data: issuerKeyRegEncode(r211Reg(who, ch.BlockEpoch(next), 1))})
	if len(nd.pendingPeerIssuerKeys) != 0 {
		t.Fatalf("a non-objective chain queued a peer registration: the bonded bound is only real inside Objective()")
	}
}

// TestR211NonFoldableQueueArmsNoProposal (§7 gate 5, S3b): a peer queue holding ONLY regs
// that will drop at fold (an unbonded stranger's) is not FOLDABLE, so the drain driver stays
// quiet — it never arms a proposal that would fail the empty-block check and spin.
func TestR211NonFoldableQueueArmsNoProposal(t *testing.T) {
	proposer, _, _, _, _ := r211Swarm(t)
	_, next := proposer.chain.Head()
	epoch := proposer.chain.BlockEpoch(next)
	proposer.pendingPeerIssuerKeys = append(proposer.pendingPeerIssuerKeys, r211Reg(identity.FromSeed(9490), epoch, 7))
	if proposer.issuerKeysFoldable() {
		t.Fatalf("a queue of only inadmissible regs reads as FOLDABLE")
	}
	armedBefore := proposer.Stats.DrainProposalsArmed
	for i := 0; i < 6; i++ {
		proposer.maybeProposeBondDrain()
	}
	// The ARMING observable, not the head: a drain keyed on queue length would arm, build an
	// IssuerKeys-empty block, fail the proposer's own empty-block check and leave the head
	// unmoved and nothing in flight — byte-identical to quiescence on every other observable.
	if got := proposer.Stats.DrainProposalsArmed - armedBefore; got != 0 {
		t.Fatalf("the drain ARMED %d proposal(s) on a non-foldable queue — that is the empty-block spin (S3b)", got)
	}
	// Control: the same driver DOES arm when the queue holds a foldable reg (the bonded
	// attester's), so the assertion above has teeth.
	_, next2 := proposer.chain.Head()
	proposer.pendingPeerIssuerKeys = append(proposer.pendingPeerIssuerKeys, r211Reg(identity.FromSeed(7701), proposer.chain.BlockEpoch(next2), 8))
	if !proposer.issuerKeysFoldable() {
		t.Fatalf("control: the bonded attester's reg must be foldable")
	}
	for i := 0; i < 8 && proposer.Stats.DrainProposalsArmed == armedBefore; i++ {
		proposer.maybeProposeBondDrain()
	}
	if proposer.Stats.DrainProposalsArmed == armedBefore {
		t.Fatalf("control: the drain never armed with a foldable reg pending")
	}
}

// TestR211IdleChainCarriesTheKeyThroughTheRealDrainDriver (§7 gate 4, F2): with NO entries
// and NO bond work pending, a foldable peer registration alone must move the head through
// maybeProposeBondDrain — including when the proposer is not the height's designee, via the
// staggered takeover (F2: the takeover clause must count issuer-key work).
func TestR211IdleChainCarriesTheKeyThroughTheRealDrainDriver(t *testing.T) {
	proposer, attest, _, net, _ := r211Swarm(t)
	// FORCE THE TAKEOVER PATH FIRST (PE frozen-head check): the designation is height-keyed
	// (height % len(props)), so at some heights the proposer IS the designee and the
	// `dist > 0` branch under test is never entered. Propose fillers — before any reg is
	// staged, so nothing rides them — until the proposer is NOT the designee.
	all := []ports.NodeID{proposer.id, attest.id}
	for filler := 0; filler < 3; filler++ {
		props := proposer.chain.EligibleProposers()
		_, h := proposer.chain.Head()
		if len(props) > 0 && props[int(h)%len(props)] != proposer.id {
			break
		}
		if err := proposeOnce(t, proposer, net, all, "filler"+string(rune('0'+filler))); err != nil {
			t.Fatalf("filler proposal: %v", err)
		}
	}
	props := proposer.chain.EligibleProposers()
	_, h0 := proposer.chain.Head()
	if len(props) < 2 || props[int(h0)%len(props)] == proposer.id {
		t.Fatalf("fixture: could not make the proposer NON-designated (props %d, height %d) — the takeover branch is what this gate must exercise", len(props), h0)
	}
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	epoch := attest.DemandEpoch()
	attest.SetDemandIssuerKey(rand.Reader, epoch, key)
	attest.SubmitIssuerKeyRegs([]ports.NodeID{proposer.id})
	drainHeld(t, net, fifo)
	if len(proposer.pendingPeerIssuerKeys) != 1 {
		t.Fatalf("setup: reg not queued")
	}
	// Nothing else is pending: no entries, no bond work, the attester (the designee) never
	// proposes. Only a FOLDABLE issuer-key reg can arm the non-designated proposer's
	// takeover after its 3+dist idle sweeps (F2).
	if len(proposer.pendingEntries) != 0 || len(proposer.pendingBondRegs) != 0 {
		t.Fatalf("fixture: other work pending (%d entries, %d regs) would mask the issuer-key term", len(proposer.pendingEntries), len(proposer.pendingBondRegs))
	}
	_, before := proposer.chain.Head()
	committed := false
	for sweep := 0; sweep < 12 && !committed; sweep++ {
		proposer.maybeProposeBondDrain()
		drainHeld(t, net, fifo)
		_, committed = proposer.chain.IssuerKeyCommitment(attest.id, epoch)
	}
	if !committed {
		_, after := proposer.chain.Head()
		t.Fatalf("the non-designated proposer never took over to carry a foldable peer registration on an idle chain (head %d → %d, drainWaitSweeps %d)", before, after, proposer.drainWaitSweeps)
	}
	if _, ok := attest.chain.IssuerKeyCommitment(attest.id, epoch); !ok {
		t.Fatalf("committed on the proposer's replica only")
	}
}

// TestR211PruneIsWiredToTheSweepAndTheEnqueue is a source gate for F3's two wiring lines:
// prunePeerIssuerKeys is called from chainSyncTick (beside SubmitIssuerKeyRegs) and at the
// top of queuePendingPeerIssuerKey. RUNTIME GATE: TestR211PeerQueueIsPrunedWithoutAProposal
// holds the function body; this gate adds only that both call sites exist, since the sync
// tick needs live peers no unit fixture provides.
func TestR211PruneIsWiredToTheSweepAndTheEnqueue(t *testing.T) {
	src, err := os.ReadFile("chainrole.go")
	if err != nil {
		t.Fatal(err)
	}
	s := string(src)
	tick := strings.Index(s, "func (n *Node) chainSyncTick() {")
	tickEnd := strings.Index(s[tick:], "\n}\n") + tick
	if tick < 0 || !strings.Contains(s[tick:tickEnd], "n.prunePeerIssuerKeys()") {
		t.Fatalf("SOURCE GATE: chainSyncTick does not call n.prunePeerIssuerKeys() — a bonded receiver that never proposes would accumulate dead peer slots to maxMempool (F3)")
	}
	q := strings.Index(s, "func (n *Node) queuePendingPeerIssuerKey(")
	qEnd := strings.Index(s[q:], "\n}\n") + q
	if q < 0 || !strings.Contains(s[q:qEnd], "n.prunePeerIssuerKeys()") {
		t.Fatalf("SOURCE GATE: queuePendingPeerIssuerKey does not prune before enqueueing (F3)")
	}
}

// TestR211PeerQueueIsPrunedWithoutAProposal (F3): a receiver that never proposes still drops
// out-of-window and committed peer slots on the sync-tick prune, so the queue cannot grow to
// maxMempool by the clock and refuse honest arrivals.
func TestR211PeerQueueIsPrunedWithoutAProposal(t *testing.T) {
	proposer, _, _, _, _ := r211Swarm(t)
	_, next := proposer.chain.Head()
	epoch := proposer.chain.BlockEpoch(next)
	// Plant regs that are already dead: backdated (if the epoch allows) and far ahead.
	stale := r211Reg(identity.FromSeed(9495), epoch+chain.IssuerKeyPrePublish+3, 1)
	proposer.pendingPeerIssuerKeys = append(proposer.pendingPeerIssuerKeys, stale)
	if epoch > 0 {
		proposer.pendingPeerIssuerKeys = append(proposer.pendingPeerIssuerKeys, r211Reg(identity.FromSeed(9496), epoch-1, 1))
	}
	proposer.prunePeerIssuerKeys()
	if len(proposer.pendingPeerIssuerKeys) != 0 {
		t.Fatalf("dead peer slots survived the prune: %d", len(proposer.pendingPeerIssuerKeys))
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
