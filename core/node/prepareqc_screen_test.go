package node

// R-CARRIER-ATTS-PREPAREQC, steps 0 and 1 — the gates.
//
// THE MECHANISM (certification §1.2, §5.4, §7). (*Node).handleChain's
// ports.MsgPrepareQC arm hands an attacker-supplied attestation list straight to
// (*Chain).VerifyPrepareQC → (*Chain).collectQuorumSigs, which pays one
// ed25519.Verify per entry. collectQuorumSigs records seen[id] only AFTER the
// qualification test, so byte-identical entries are never deduplicated: one
// offline ed25519.Sign, replayed to the CBOR decoder's 131,072-element ceiling,
// buys 131,072 verifies (4.19–6.89 s of one core) for 13.1 MiB of wire, from ANY
// peer, at line rate. The arm has no sender screen, no rate budget and no length
// check.
//
// Gates in this file, each naming the function whose behaviour changes:
//
//	G-QC-0  (*Node).handleChain, ports.MsgPrepareQC — the sender screen (step 0).
//	G-QC-1  (*Node).gatherTwoPhase — the producer screen (step 1). Shared with
//	        R-CB-ATTS-UNBOUNDED's G-ATTS-1: one screen, both closures.
//	G-QC-2  the at-ceiling ACCEPT arm, on the launch regime that has zero slack.
//	G-QC-4  the sender screen refuses nothing honest, on the one path where the
//	        gatherer is NOT the block's author.
//
// Deliberation: docs/thinking/2026-09-11-prepareqc-sender-and-producer-screens.md

import (
	"crypto/ed25519"
	"fmt"
	"strings"
	"testing"

	"github.com/fxamacker/cbor/v2"
	"github.com/nerolabs/silt/adapters/identity"
	"github.com/nerolabs/silt/adapters/markstore"
	"github.com/nerolabs/silt/adapters/memstore"
	"github.com/nerolabs/silt/adapters/simclock"
	"github.com/nerolabs/silt/adapters/simnet"
	"github.com/nerolabs/silt/core/chain"
	"github.com/nerolabs/silt/core/credit"
	"github.com/nerolabs/silt/ports"
)

// qcLog records every emitted event with its key/values flattened to one
// string, so a gate can assert WHICH refusal fired — the only observable that
// separates "refused at the screen, verifier never entered" from "refused by
// the verifier after it paid for every entry".
type qcLog struct{ lines []string }

func (l *qcLog) Enabled(ports.LogLevel) bool { return true }
func (l *qcLog) Log(_ ports.LogLevel, event string, kv ...any) {
	var b strings.Builder
	b.WriteString(event)
	for _, v := range kv {
		fmt.Fprintf(&b, " %v", v)
	}
	l.lines = append(l.lines, b.String())
}

// last returns the most recent line whose prefix matches, or "".
func (l *qcLog) last(prefix string) string {
	for i := len(l.lines) - 1; i >= 0; i-- {
		if strings.HasPrefix(l.lines[i], prefix) {
			return l.lines[i]
		}
	}
	return ""
}

// ── the fixture ───────────────────────────────────────────────────────────────

// qcOutsiderNode builds a live peer that shares the anchors' chain config and
// genesis — so it decodes, validates and REPLIES to a proposal exactly as an
// anchor does — but whose identity is in no anchor set and holds no bond. That
// is the shipped `-attesters` shape: chainhost.Host.Attesters is the flag,
// unfiltered, and the ports.MsgProposeBlock arm replies for any node that
// passes (*Chain).ValidateProposal with no test of the REPLIER's own
// qualification (certification §3.1(3), §3.2).
func qcOutsiderNode(t *testing.T, clock ports.Clock, net *simnet.Network, id *identity.Identity, cfg chain.Config, g *chain.Block) *Node {
	t.Helper()
	nd := New(id.NodeID(), DefaultConfig(), clock, net.Endpoint(id.NodeID()), memstore.New())
	nd.SetLedger(credit.New(50_000, 0))
	ch := chain.New(cfg, func(ports.NodeID) int64 { return 0 })
	ch.SetBondVerifier(mcStubVerify)
	if err := ch.AppendGenesis(*g); err != nil {
		t.Fatalf("outsider genesis: %v", err)
	}
	nd.EnableChain(ch, id.Signer())
	if err := nd.SetSignMarkStore(markstore.NewMem()); err != nil {
		t.Fatalf("outsider sign-mark store: %v", err)
	}
	return nd
}

// ── G-QC-0 ────────────────────────────────────────────────────────────────────
//
// TestG_QC_0_PrepareQCFromUnscreenedSenderIsRefusedBeforeTheVerifier.
//
// THE SCHEDULE. A 4-anchor launch network. n0 gathers a REAL prepare-QC for a
// real block among n0/n1/n2, with n3 excluded from both the attester and the
// broadcast set, so n3 learns of the round through exactly one channel: the
// bytes this test hands it. Those captured bytes are the genuine article — the
// same message a real attester received.
//
// ARM 1 (the primitive). The genuine QC is delivered to n3 `from` an identity
// that is in no anchor set and holds no bond — the attacker of §1.2, who needs
// no keypair farm, no bond and no proposal.
//   - RED at HEAD: n3 verifies the whole list, adopts the lock and precommits.
//     The attacker got a validator's full CPU budget for one replayed message.
//   - GREEN: refused. No lock, no mark.
//
// ARM 2 (the control, and it is what makes arm 1 mean anything). The IDENTICAL
// bytes from a QUALIFIED sender are accepted. Without this the gate could pass
// on a QC that was simply unacceptable.
//
// ARM 3 (zero verifications). The same QC, padded with one attestation whose
// signature is corrupt — a FATAL entry for (*Chain).collectQuorumSigs. From a
// qualified sender the refusal reason is the signature error, which is the
// verifier's own voice and proves it walked the list. From the unscreened
// sender the refusal must be the SCREEN's, and must not carry the signature
// error: the verifier was never entered, so not one ed25519.Verify was paid.
func TestG_QC_0_PrepareQCFromUnscreenedSenderIsRefusedBeforeTheVerifier(t *testing.T) {
	nodes, _, net, g, _ := tier2AnchorNet(t, 4)
	n0, n1, n2, n3 := nodes[0], nodes[1], nodes[2], nodes[3]

	lg := &qcLog{}
	n3.SetLogger(lg)

	var captured []byte
	ep1 := net.Endpoint(n1.id)
	orig := n1.handle
	ep1.SetHandler(func(from ports.NodeID, msg ports.Message) {
		if msg.Kind == ports.MsgPrepareQC && captured == nil {
			captured = append([]byte(nil), msg.Data...)
		}
		orig(from, msg)
	})

	b := &chain.Block{Version: 1, Height: 1, Prev: g.Hash(), Entries: []ports.Entry{mkEntry("qc-screen")}}
	var done bool
	var perr error
	n0.proposeBlock(b, []ports.NodeID{n1.id, n2.id}, []ports.NodeID{n1.id, n2.id}, 1,
		func(err error) { done, perr = true, err })
	drainHeld(t, net, fifo)
	if !done || perr != nil {
		t.Fatalf("premise: the n0/n1/n2 gather must commit: done=%v err=%v", done, perr)
	}
	if captured == nil {
		t.Fatal("premise: never captured a MsgPrepareQC — the gather did not reach the precommit phase")
	}
	if _, h := n3.chain.Head(); h != 1 {
		t.Fatalf("premise: n3 must not have heard of the round by any other channel (head=%d, want 1)", h)
	}
	if !n3.chain.Objective() {
		t.Fatal("premise: the screen is objective-guarded — this fixture must be objective")
	}

	// ── ARM 1 ────────────────────────────────────────────────────────────────
	attacker := identity.FromSeed(90210) // no anchor seat, no bond
	if n3.chain.AttesterEligibleAt(attacker.NodeID(), 1) {
		t.Fatal("premise: the attacker identity must NOT be in n3's governing set")
	}
	n3.handle(attacker.NodeID(), ports.Message{Kind: ports.MsgPrepareQC, Data: captured})
	drainHeld(t, net, fifo)

	if rs := n3.roundsFor(); rs.Lock != nil {
		t.Fatalf("G-QC-0 VIOLATION: an identity with no anchor seat and no bond spent n3's whole "+
			"VerifyPrepareQC budget and made it LOCK (round %d, hash %x). collectQuorumSigs pays one "+
			"ed25519.Verify per entry and dedups only qualified ids, so one offline signature replayed "+
			"to the CBOR ceiling buys 131,072 verifies from any peer at line rate (certification §1.2).",
			rs.Lock.Round, rs.Lock.Hash)
	}
	if n3.signMarkSet {
		t.Fatal("G-QC-0 VIOLATION: an unscreened sender moved n3's sign mark")
	}

	// ── ARM 2: the control ───────────────────────────────────────────────────
	n3.handle(n0.id, ports.Message{Kind: ports.MsgPrepareQC, Data: captured})
	drainHeld(t, net, fifo)
	rs := n3.roundsFor()
	if rs.Lock == nil {
		t.Fatal("G-QC-0 CONTROL FAILED: the same bytes from a QUALIFIED sender were refused — the screen " +
			"is refusing honest traffic, or the fixture's QC was never acceptable and arm 1 proved nothing")
	}

	// ── ARM 3: zero verifications ────────────────────────────────────────────
	padded := qcPlusCorruptEntry(t, captured)
	n3.SetLogger(&qcLog{}) // fresh, so `last` cannot read arm 1/2's lines
	lgQual := &qcLog{}
	n3.SetLogger(lgQual)
	n3.handle(n0.id, ports.Message{Kind: ports.MsgPrepareQC, Data: padded})
	drainHeld(t, net, fifo)
	qualLine := lgQual.last("gather/precommit: REFUSED")
	if !strings.Contains(qualLine, "signature") {
		t.Fatalf("premise: a corrupt-signature entry from a QUALIFIED sender must be refused BY THE VERIFIER "+
			"(that is what proves the verifier walked the list); got %q", qualLine)
	}

	lgAtk := &qcLog{}
	n3.SetLogger(lgAtk)
	n3.handle(attacker.NodeID(), ports.Message{Kind: ports.MsgPrepareQC, Data: padded})
	drainHeld(t, net, fifo)
	atkLine := lgAtk.last("gather/precommit: REFUSED")
	if atkLine == "" {
		t.Fatal("G-QC-0 VIOLATION: the unscreened sender's padded prepare-QC was not refused at all")
	}
	if strings.Contains(atkLine, "signature") {
		t.Fatalf("G-QC-0 VIOLATION: the unscreened sender's padded prepare-QC reached collectQuorumSigs — "+
			"the refusal is the VERIFIER's (%q), so every entry before the corrupt one was paid for. "+
			"The screen must refuse before the first ed25519.Verify.", atkLine)
	}
	t.Logf("G-QC-0: unscreened sender refused at %q; qualified sender's identical bytes accepted; "+
		"padded list never reached the verifier.", atkLine)
}

// qcPlusCorruptEntry appends one attestation whose signature is flipped — a
// FATAL entry for collectQuorumSigs (`return ErrBadSignature`), reached only if
// the verifier actually walks the list. Re-encoded through the production
// envelope, never hand-rolled bytes.
func qcPlusCorruptEntry(t *testing.T, raw []byte) []byte {
	t.Helper()
	var env prepareQCEnv
	if err := cbor.Unmarshal(raw, &env); err != nil {
		t.Fatalf("decode captured prepare-QC: %v", err)
	}
	if len(env.QC) == 0 {
		t.Fatal("captured prepare-QC is empty")
	}
	bad := env.QC[0]
	bad.PubKey = append([]byte(nil), bad.PubKey...)
	bad.Sig = append([]byte(nil), bad.Sig...)
	// A DISTINCT, unqualified pubkey, so the entry is neither deduped nor
	// proposer-skipped: it reaches the verify and fails there.
	rogue := identity.FromSeed(90211)
	bad.PubKey = append([]byte(nil), rogue.Signer().Public().(ed25519.PublicKey)...)
	bad.Sig[0] ^= 0xff
	env.QC = append(append([]chain.Attestation(nil), env.QC...), bad)
	out, err := cbor.Marshal(env)
	if err != nil {
		t.Fatalf("re-encode prepare-QC: %v", err)
	}
	return out
}

// ── G-QC-1 ────────────────────────────────────────────────────────────────────
//
// TestG_QC_1_GatherSolicitsOnlyTheGoverningSet.
//
// THE DEFECT (certification §3.2, canon rule 8). (*Node).gatherTwoPhase
// solicits `attesters` verbatim. On the client-publish path that slice is
// chainhost.Host.Attesters — the `-attesters` flag, UNFILTERED — so the shipped
// honest maximum for len(env.QC) is `2 + |-attesters|`, a function of LOCAL
// CONFIG, not of the chain. The #380 class, and it blocks every bound over
// committed quantities.
//
// THE SCHEDULE. A 4-anchor launch network (GoverningSetCap() = 4) plus SIX live
// outsiders that share the genesis and reply to a proposal but hold no seat and
// no bond. n0 publishes with the outsiders listed FIRST, so their replies land
// before the anchors' and the gather cannot close before they are counted —
// which is the honest `-attesters` ordering hazard, not a contrivance.
//
//   - RED at HEAD: len(qc) = 1 + 6 + (anchors needed) — strictly above
//     GoverningSetCap() + 2, so no bound expressed over committed quantities
//     can be stated at all.
//   - GREEN: len(qc) ≤ GoverningSetCap() + 2, and the block still commits.
//
// The ceiling is the CERTIFIED expression, not the screen's own predicate: the
// screen filters on (*Chain).AttesterEligibleAt; the assertion is against
// (*Chain).GoverningSetCap()+2, an independent quantity, pinned to its literal
// value by the premise below.
func TestG_QC_1_GatherSolicitsOnlyTheGoverningSet(t *testing.T) {
	nodes, ids, net, g, cfg := tier2AnchorNet(t, 4)
	n0 := nodes[0]

	if cap := n0.chain.GoverningSetCap(); cap != 4 {
		t.Fatalf("premise: a 4-anchor launch network must have GoverningSetCap()==4, got %d", cap)
	}

	const outsiders = 6 // > GoverningSetCap(), so the RED is not a rounding artefact
	attesters := make([]ports.NodeID, 0, outsiders+3)
	for i := 0; i < outsiders; i++ {
		oid := identity.FromSeed(int64(91000 + i))
		qcOutsiderNode(t, n0.clock, net, oid, cfg, g)
		if n0.chain.AttesterEligibleAt(oid.NodeID(), 1) {
			t.Fatalf("premise: outsider %d must not be in the governing set", i)
		}
		attesters = append(attesters, oid.NodeID())
	}
	broadcast := make([]ports.NodeID, 0, 3)
	for _, id := range ids[1:] {
		attesters = append(attesters, id.NodeID())
		broadcast = append(broadcast, id.NodeID())
	}

	var qcLen int
	ep := net.Endpoint(ids[1].NodeID())
	orig := nodes[1].handle
	ep.SetHandler(func(from ports.NodeID, msg ports.Message) {
		if msg.Kind == ports.MsgPrepareQC && qcLen == 0 {
			var env prepareQCEnv
			if cbor.Unmarshal(msg.Data, &env) == nil {
				qcLen = len(env.QC)
			}
		}
		orig(from, msg)
	})

	b := &chain.Block{Version: 1, Height: 1, Prev: g.Hash(), Entries: []ports.Entry{mkEntry("wide-attesters")}}
	var done bool
	var perr error
	n0.proposeBlock(b, attesters, broadcast, 1, func(err error) { done, perr = true, err })
	drainHeld(t, net, fifo)

	if !done || perr != nil {
		t.Fatalf("G-QC-1 VIOLATION (liveness): a publish whose -attesters list names unqualified peers "+
			"must still commit — the screen removed something the gather needed: done=%v err=%v", done, perr)
	}
	if qcLen == 0 {
		t.Fatal("premise: never observed a MsgPrepareQC — the gather did not reach the precommit phase")
	}
	want := n0.chain.GoverningSetCap() + 2
	if qcLen > want {
		t.Fatalf("G-QC-1 VIOLATION: the gather emitted a prepare-QC of %d entries against a governing-set "+
			"ceiling of %d (GoverningSetCap()=%d, +2 for the two seeds collectQuorumSigs structurally "+
			"skips). The solicitation set is the -attesters flag, so len(env.QC)'s honest maximum is a "+
			"function of LOCAL CONFIG, not of the chain (#380 class, canon rule 8) — and no wire bound "+
			"over committed quantities can be stated until it is not.", qcLen, want, n0.chain.GoverningSetCap())
	}
	t.Logf("G-QC-1: %d outsiders solicited, prepare-QC carried %d entries (ceiling %d); block committed.",
		outsiders, qcLen, want)
}

// ── G-QC-2 + G-QC-4 ───────────────────────────────────────────────────────────
//
// TestG_QC_2_4_ForcedReproposalAtTheLaunchCeilingIsAcceptedByEveryAttester.
//
// THE TWO CLAIMS, on the one regime that has zero slack (certification §4.1).
//
// G-QC-4 — the sender screen refuses nothing honest on the ONE path where the
// gatherer is not the block's author. Every other prepare-QC arrives from the
// block's own author, who has already passed the receiver's stronger
// proposerQualifiedAt test at the prepare phase. On a FORCED RE-PROPOSAL the
// sender is the designee and the author is a third party, so the screen is
// genuinely new there.
//
// G-QC-2 (accept arm) — that honest re-proposal's certificate carries MORE than
// GoverningSetCap() entries. The two extra are the seeds
// (*Chain).collectQuorumSigs skips via `id == b.ProposerID()` WITHOUT consuming
// a `seen` slot — the gatherer's own prepare and the absent author's carried
// self-prepare — so no set-size term can ever cover them. This is the measured
// evidence that a RAW GoverningSetCap() cap would wedge an honest round, and it
// is why step 2's bound is `+2` rather than a chosen constant.
//
// The refuse arm of G-QC-2 (ceiling+1 refused with zero verifies) belongs to
// the cap itself, which the certification's landing order (§8) puts at step 2,
// ≥ one deployment window after step 1. Not built here; owed with the cap.
func TestG_QC_2_4_ForcedReproposalAtTheLaunchCeilingIsAcceptedByEveryAttester(t *testing.T) {
	nodes, ids, net, g, _ := tier2AnchorNet(t, 4)
	all := make([]ports.NodeID, len(ids))
	byID := map[ports.NodeID]*Node{}
	for i, id := range ids {
		all[i] = id.NodeID()
		byID[id.NodeID()] = nodes[i]
	}
	if capN := nodes[0].chain.GoverningSetCap(); capN != 4 {
		t.Fatalf("premise: want GoverningSetCap()==4 on a 4-anchor launch network, got %d", capN)
	}

	// The DESIGNEE for (height 1, round 1) is DERIVED, never chosen — the same
	// call (*Node).proposeAtNewView makes. The AUTHOR is another anchor, so the
	// gatherer is not the block's author: the one path on which the sender
	// screen is genuinely new (certification §4.4).
	designeeID := nodes[0].designatedProposer(1, 1)
	designee, ok := byID[designeeID]
	if !ok {
		t.Fatal("premise: designatedProposer(1, 1) returned an unknown id")
	}
	var author *identity.Identity
	var rest []*identity.Identity
	for i, id := range ids {
		switch {
		case id.NodeID() == designeeID:
		case author == nil:
			author = ids[i]
		default:
			rest = append(rest, ids[i])
		}
	}
	if author == nil || len(rest) < 2 {
		t.Fatal("premise: need an author and two other anchors distinct from the designee")
	}

	// BlockVersionRounds: the era (*Node).proposeBlock stamps for the two-phase
	// round machinery. An era-1 block's quorum demands (PhaseLegacy, round 0),
	// so a round-1 certificate over one could never commit.
	b := &chain.Block{Version: chain.BlockVersionRounds, Height: 1, Prev: g.Hash(), Entries: []ports.Entry{mkEntry("forced-reproposal")}}
	chain.Sign(b, author.Signer())
	if err := nodes[0].chain.ValidateProposal(b); err != nil {
		t.Fatalf("premise: the author's block must be a valid proposal: %v", err)
	}

	// The round-0 prepare-QC the lock carries — the object
	// (*Node).gatherTwoPhase's finishPrep closure assembles: the author's own
	// self-prepare (the seed (*Chain).requireProposerPrepare demands) then the
	// other anchors' replies. Real chain.AttestAt signatures; the PRODUCTION
	// verifier is the premise.
	qc0 := []chain.Attestation{chain.AttestAt(b, author.Signer(), 0, chain.PhasePrepare, nodes[0].chainID())}
	for _, id := range rest {
		qc0 = append(qc0, chain.AttestAt(b, id.Signer(), 0, chain.PhasePrepare, nodes[0].chainID()))
	}
	if err := nodes[0].chain.VerifyPrepareQC(b, qc0, 0); err != nil {
		t.Fatalf("premise: the round-0 prepare-QC must verify: %v", err)
	}

	// The new-view certificate for round 1: lock-carrying round-change
	// envelopes from the designee and the other live anchors (the author is the
	// node that went down — the reason the view changed).
	rawBlock := chain.Encode(b)
	signers := append([]*identity.Identity{ids[indexOfID(ids, designeeID)]}, rest...)
	var newView [][]byte
	for _, id := range signers {
		rc := roundChangeEnv{Height: 1, NewRound: 1,
			Sender:    append([]byte(nil), id.Signer().Public().(ed25519.PublicKey)...),
			LockRound: 0, LockQC: qc0, LockBlock: rawBlock}
		rc.Sig = ed25519.Sign(id.Signer(), rc.sigBytes())
		raw, err := cbor.Marshal(rc)
		if err != nil {
			t.Fatal(err)
		}
		newView = append(newView, raw)
	}

	// The FORCED value, derived by the production function the designee itself
	// calls — not asserted by this test.
	forced, err := designee.newViewFor(1, 1, newView)
	if err != nil {
		t.Fatalf("premise: the new-view certificate must validate: %v", err)
	}
	if forced == nil || forced.Hash != b.Hash() {
		t.Fatal("premise: the certificate must force the author's block")
	}
	lb, err := chain.Decode(forced.Block)
	if err != nil {
		t.Fatal(err)
	}

	var qcLen, exempt int
	for _, nd := range nodes {
		if nd.id == designeeID {
			continue
		}
		nd := nd
		ep := net.Endpoint(nd.id)
		orig := nd.handle
		ep.SetHandler(func(from ports.NodeID, msg ports.Message) {
			if msg.Kind == ports.MsgPrepareQC {
				var env prepareQCEnv
				if cbor.Unmarshal(msg.Data, &env) == nil && len(env.QC) > qcLen {
					qcLen = len(env.QC)
					exempt = 0
					for _, a := range env.QC {
						// The entries (*Chain).collectQuorumSigs skips via
						// `id == b.ProposerID()`: a list slot and no `seen`
						// slot, so no set-size term can ever cover them.
						if a.AttesterID() == lb.ProposerID() {
							exempt++
						}
					}
				}
			}
			orig(from, msg)
		})
	}

	// The AUTHOR is solicited FIRST. `attesters` is syncTargets() order in
	// production, so any order is reachable, and this one maximises the exempt
	// count: the author is alive here (a designee re-proposes on a TIMEOUT, not
	// on a proof of death), so its own reply lands alongside its carried
	// self-prepare and both are proposer-skipped.
	order := []ports.NodeID{author.NodeID()}
	for _, id := range rest {
		order = append(order, id.NodeID())
	}
	order = append(order, designeeID)

	// Exactly (*Node).proposeAtNewView's forced leg: chainrole.go's
	// gatherTwoPhase(lb, attesters, peers, 0, round, newView, forced.QC, fin).
	var done bool
	var perr error
	designee.gatherTwoPhase(lb, order, all, 0, 1, newView, forced.QC, func(e error) { done, perr = true, e })
	drainHeld(t, net, fifo)

	if !done || perr != nil {
		t.Fatalf("G-QC-4 VIOLATION: an honest forced re-proposal by a non-author designee was not "+
			"accepted — the sender screen refused honest traffic on the one path where the gatherer "+
			"is not the block's author: done=%v err=%v", done, perr)
	}
	for i, nd := range nodes {
		if _, h := nd.chain.Head(); h != 2 {
			t.Fatalf("G-QC-4 VIOLATION: node %d did not end up holding the re-proposed block (head=%d)", i, h)
		}
	}
	if qcLen == 0 {
		t.Fatal("premise: no MsgPrepareQC observed — the forced re-proposal never reached the precommit phase")
	}
	capN := nodes[0].chain.GoverningSetCap()

	// THE +2, DRIVEN. `len(qc) = |seen| + exempt`. |seen| is a set of qualified
	// distinct ids, so GoverningSetCap() bounds it — and NOTHING bounds `exempt`
	// by a set-size term, because those entries are skipped before `seen` is
	// touched. `exempt` is at most 2 by construction: gatherTwoPhase lifts AT
	// MOST ONE carried author self-prepare (`break` on first match) and the
	// author answers AT MOST ONE solicitation. This gate drives the case where
	// both are present, which is what makes the ceiling GoverningSetCap() + 2
	// and not GoverningSetCap() + 1.
	if exempt != 2 {
		t.Fatalf("G-QC-2 PREMISE LOST: the honest forced re-proposal carried %d entries of which %d were "+
			"proposer-skipped, want 2 (the carried author self-prepare AND the live author's own reply). "+
			"The +2 in step 2's bound is exactly this count; if the fixture no longer produces both, "+
			"the bound is being validated against a case that does not exercise it.", qcLen, exempt)
	}
	if qcLen > capN+2 {
		t.Fatalf("G-QC-2 VIOLATION: an honest forced re-proposal carried %d entries, above the certified "+
			"ceiling GoverningSetCap()+2 = %d — step 2's cap would refuse honest traffic.", qcLen, capN+2)
	}
	if qcLen-exempt > capN {
		t.Fatalf("G-QC-2 VIOLATION: %d counted entries against GoverningSetCap()=%d — the set-size term "+
			"does not dominate the qualified signer set the certificate carries", qcLen-exempt, capN)
	}
	// CORRECTION, driven: the certification's §4.1 arithmetic ("1 + 1 + 3 = 5 > 4")
	// over-counts. finishPrep closes on the FIRST reply that satisfies supportMet,
	// so a 4-anchor launch network produces |seen| = 2 and len(qc) = 4 — EXACTLY
	// GoverningSetCap(), not one above it. A raw cap refuses nothing here; it has
	// ZERO SLACK, which is why the +2 is still required and not merely prudent.
	if qcLen != capN {
		t.Logf("NOTE: the honest launch-window certificate is %d entries against GoverningSetCap()=%d "+
			"(it was exactly at the cap when this gate was written); the raw cap's slack has moved.", qcLen, capN)
	}
	t.Logf("G-QC-2/G-QC-4: honest forced re-proposal by non-author designee %s accepted by every attester; "+
		"certificate carried %d entries = %d counted + %d proposer-skipped, against GoverningSetCap()=%d "+
		"and the certified ceiling %d.", designeeID, qcLen, qcLen-exempt, exempt, capN, capN+2)
}

// indexOfID locates an identity by its NodeID.
func indexOfID(ids []*identity.Identity, want ports.NodeID) int {
	for i, id := range ids {
		if id.NodeID() == want {
			return i
		}
	}
	return -1
}

// ── R-CARRIER-QC-LEGACY-UNCAPPED, driven ──────────────────────────────────────
//
// TestQC_LegacyPostureKeepsThePrimitive_Driven pins the residual the screen
// leaves open, because a residual carried as prose decays and a coverage row
// nobody drives is decoration (simplicity rule 7).
//
// The screen is guarded by (*Chain).Objective(). In a trusted/demo posture
// (*Chain).attesterQualifiedAt falls through to `rep >= MinAttesterRep`, a
// quantity no committed state bounds, and (*Chain).GoverningSetCap() is 0 — so
// the guard is what keeps an honest legacy network from halting, and the price
// is that the ports.MsgPrepareQC primitive stays OPEN there.
//
// This test asserts the vulnerability, deliberately. It goes RED the moment
// somebody removes the guard — at which point the residual is CLOSED and this
// probe must be replaced by its inverse, not deleted.
func TestQC_LegacyPostureKeepsThePrimitive_Driven(t *testing.T) {
	sched := simclock.New()
	net := simnet.New(sched, 1, simnet.DefaultConfig())
	net.EnableHeldDelivery()

	ids := make([]*identity.Identity, 4)
	rep := map[ports.NodeID]int64{}
	for i := range ids {
		ids[i] = identity.FromSeed(int64(8700 + i))
		rep[ids[i].NodeID()] = 100
	}
	g := &chain.Block{Version: 1, Height: 0, Entries: []ports.Entry{mkEntry("g-legacy")}}
	chain.Sign(g, ids[0].Signer())
	// MinBond 0 and no bond verifier ⇒ objective() is FALSE: the trusted/demo
	// posture. MinAttesterRep 100 against a rep oracle is how such a network
	// actually qualifies attesters.
	cfg := chain.Config{Quorum: 1, MinProposerRep: 100, MinAttesterRep: 100}

	nodes := make([]*Node, len(ids))
	for i, id := range ids {
		nd := New(id.NodeID(), DefaultConfig(), sched, net.Endpoint(id.NodeID()), memstore.New())
		nd.SetLedger(credit.New(50_000, 0))
		ch := chain.New(cfg, func(who ports.NodeID) int64 { return rep[who] })
		if err := ch.AppendGenesis(*g); err != nil {
			t.Fatalf("genesis: %v", err)
		}
		nd.EnableChain(ch, id.Signer())
		if err := nd.SetSignMarkStore(markstore.NewMem()); err != nil {
			t.Fatalf("sign-mark store: %v", err)
		}
		nodes[i] = nd
	}
	n0, n1, n3 := nodes[0], nodes[1], nodes[3]
	if n0.chain.Objective() {
		t.Fatal("premise: this fixture must be the NON-objective (trusted/demo) posture")
	}
	if n0.chain.GoverningSetCap() != 0 {
		t.Fatalf("premise: GoverningSetCap() is 0 in a legacy posture, got %d", n0.chain.GoverningSetCap())
	}

	var captured []byte
	ep1 := net.Endpoint(n1.id)
	orig := n1.handle
	ep1.SetHandler(func(from ports.NodeID, msg ports.Message) {
		if msg.Kind == ports.MsgPrepareQC && captured == nil {
			captured = append([]byte(nil), msg.Data...)
		}
		orig(from, msg)
	})

	b := &chain.Block{Version: 1, Height: 1, Prev: g.Hash(), Entries: []ports.Entry{mkEntry("legacy-round")}}
	var done bool
	var perr error
	n0.proposeBlock(b, []ports.NodeID{n1.id, nodes[2].id}, []ports.NodeID{n1.id, nodes[2].id}, 1,
		func(err error) { done, perr = true, err })
	drainHeld(t, net, fifo)
	if !done || perr != nil {
		t.Fatalf("premise: the two-phase gather RUNS in a legacy posture (gatherTwoPhase has no "+
			"Objective() gate and proposeBlock stamps BlockVersionRounds unconditionally): done=%v err=%v", done, perr)
	}
	if captured == nil {
		t.Fatal("premise: no MsgPrepareQC observed — the legacy gather did not reach the precommit phase")
	}

	attacker := identity.FromSeed(90300) // rep 0: outside the legacy attester rule
	if n3.chain.AttesterEligibleAt(attacker.NodeID(), 1) {
		t.Fatal("premise: the attacker must fail the legacy attester predicate")
	}
	n3.handle(attacker.NodeID(), ports.Message{Kind: ports.MsgPrepareQC, Data: captured})
	drainHeld(t, net, fifo)
	if rs := n3.roundsFor(); rs.Lock == nil {
		t.Fatal("R-CARRIER-QC-LEGACY-UNCAPPED IS CLOSED: the legacy posture now refuses an unqualified " +
			"sender's prepare-QC. That is an improvement, not a failure — but the residual in the " +
			"certification and in this file's doc is now stale, and an honest legacy network whose " +
			"MinProposerRep is below its MinAttesterRep can no longer precommit. Update both, then " +
			"replace this probe with its inverse.")
	}
	t.Log("R-CARRIER-QC-LEGACY-UNCAPPED: driven — in the trusted/demo posture an unqualified sender " +
		"still spends the full VerifyPrepareQC budget. The objective() guard is what keeps that open.")
}
