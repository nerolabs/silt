package node

import (
	"errors"
	"fmt"

	"github.com/fxamacker/cbor/v2"
	"github.com/nerolabs/silt/core/advisory"
	"github.com/nerolabs/silt/core/chain"
	"github.com/nerolabs/silt/core/release"
	"github.com/nerolabs/silt/ports"
)

// The version-floor advisory at the node (TENETS R4; core/advisory, core/chain/versionfloor.go).
//
// The chain records the advisory in force. This file is what a node does about it:
//
//   - carry a submitted advisory to a block (SubmitAdvisory, MsgSubmitAdvisory, foldAdvisory);
//   - warn its operator when its own release is below the floor, at every tier but a recall;
//   - at the enforce tier, once the grace period has passed, refuse consensus messages from
//     nodes advertising a release below the floor (floorRefuses).
//
// Nothing here replaces a binary or stops a node serving content. An old node keeps storing,
// serving, fetching and following the chain; it stops counting in consensus.

var errNoChain = errors.New("node: this node holds no chain")

// release is the release this node advertises: its build's, unless configured otherwise.
func (n *Node) release() uint32 {
	if n.cfg.AdvertiseRelease != 0 {
		return n.cfg.AdvertiseRelease
	}
	return release.Number
}

// floorConsensusKinds are the messages an enforced floor refuses from a node below it: block
// proposals, the votes that commit them, round changes, and a validator's submissions. Chain
// reads (sync, heads, witnesses), content traffic and a publisher's entry submissions are not
// among them: an old node keeps its users.
var floorConsensusKinds = map[ports.MsgKind]bool{
	ports.MsgProposeBlock: true, ports.MsgAttestReply: true,
	ports.MsgPrepareQC: true, ports.MsgPrecommitReply: true,
	ports.MsgCommitBlock: true,
	ports.MsgRoundChange: true, ports.MsgRoundCert: true,
	ports.MsgSubmitBondReg: true, ports.MsgHaveBondReg: true, ports.MsgSubmitIssuerKeyReg: true,
	ports.MsgSubmitSlash: true, ports.MsgSubmitAdvisory: true,
}

// floorEnforced returns the floor this node enforces at its head, or 0 when none: an
// enforce-tier advisory is in force and its grace has passed.
func (n *Node) floorEnforced() uint32 {
	if n.chain == nil {
		return 0
	}
	a, at, ok := n.chain.VersionFloor()
	if !ok {
		return 0
	}
	_, h := n.chain.Head() // h is the next height; the head block is h-1
	if h == 0 || !advisory.Enforcing(a, at, h-1) {
		return 0
	}
	return a.Floor
}

// floorRefuses reports whether msg is a consensus message from a node below the enforced floor.
// Only a validator enforces: a follower takes part in no consensus to refuse.
func (n *Node) floorRefuses(msg ports.Message) bool {
	if !floorConsensusKinds[msg.Kind] || n.chainFollower() {
		return false
	}
	floor := n.floorEnforced()
	return floor != 0 && msg.Release < floor
}

// refuseBelowFloor drops a consensus message from a node below the enforced floor: a request
// is answered with a refusal, a reply (a vote) is not counted.
func (n *Node) refuseBelowFloor(from ports.NodeID, msg ports.Message) {
	n.Stats.FloorRefused++
	if n.floorRefusedLogged == nil {
		n.floorRefusedLogged = map[ports.NodeID]bool{}
	}
	if !n.floorRefusedLogged[from] {
		evictPeerInfoIfFull(n.floorRefusedLogged, from)
		n.floorRefusedLogged[from] = true
		n.logf(ports.LogWarn, "version-floor: refusing consensus messages from a node below the network's floor",
			"from", from, "release", msg.Release, "floor", n.floorEnforced(), "kind", msg.Kind)
	}
	if !msg.IsReply() {
		n.reply(from, msg, ports.Message{Kind: replyKind(msg.Kind), OK: false})
	}
}

// VersionFloorStatus is what a node reports about the advisory in force.
type VersionFloorStatus struct {
	Seq         uint64 `json:"seq"`
	Floor       uint32 `json:"floor"`
	Tier        string `json:"tier"`
	GraceBlocks uint64 `json:"graceBlocks"`
	CommittedAt uint64 `json:"committedAt"`
	EnforcingAt uint64 `json:"enforcingAt,omitempty"`
	Enforcing   bool   `json:"enforcing"`
	Release     uint32 `json:"release"`
	BelowFloor  bool   `json:"belowFloor"`
	Note        string `json:"note,omitempty"`
	// Adoption, as this node has heard it: of the validators eligible to propose, how many last
	// advertised a release below the floor, and how many this node has not heard from. An
	// enforced floor removes the weight below it from the quorum, so this is what tells the
	// maintainers whether enforcing would halt the chain.
	Validators          int   `json:"validators"`
	ValidatorsBelow     int   `json:"validatorsBelow"`
	ValidatorsUnheard   int   `json:"validatorsUnheard"`
	BondedBelowPermille int64 `json:"bondedBelowPermille"`
}

func tierName(t advisory.Tier) string {
	switch t {
	case advisory.TierRecall:
		return "recall"
	case advisory.TierNotice:
		return "notice"
	case advisory.TierUrgent:
		return "urgent"
	case advisory.TierEnforce:
		return "enforce"
	}
	return "unknown"
}

// VersionFloor reports the advisory in force and this node's place against it; ok is false when
// none has been committed.
func (n *Node) VersionFloor() (VersionFloorStatus, bool) {
	if n.chain == nil {
		return VersionFloorStatus{}, false
	}
	a, at, ok := n.chain.VersionFloor()
	if !ok {
		return VersionFloorStatus{}, false
	}
	s := VersionFloorStatus{Seq: a.Seq, Floor: a.Floor, Tier: tierName(a.Tier), GraceBlocks: a.GraceBlocks,
		CommittedAt: at, Release: n.release(), Note: a.Note}
	s.BelowFloor = a.Floor != 0 && s.Release < a.Floor
	if a.Tier == advisory.TierEnforce {
		s.EnforcingAt = at + a.GraceBlocks
		s.Enforcing = n.floorEnforced() != 0
	}
	if a.Floor != 0 {
		var total, below int64
		for _, id := range n.chain.EligibleProposers() {
			s.Validators++
			w := n.chain.BondedSize(id)
			total += w
			rel, heard := n.peerRelease[id]
			if id == n.id {
				rel, heard = s.Release, true
			}
			switch {
			case !heard:
				s.ValidatorsUnheard++
			case rel < a.Floor:
				s.ValidatorsBelow++
				below += w
			}
		}
		if total > 0 {
			s.BondedBelowPermille = below * 1000 / total
		}
	}
	return s, true
}

// SetFloorNotice routes the operator-facing version-floor lines to fn as well as the log: a
// daemon prints them on its console, where an operator who has not opened the log still sees
// them.
func (n *Node) SetFloorNotice(fn func(line string)) { n.floorNotice = fn }

// noteVersionFloor tells the operator about a new advisory once, and about enforcement once it
// starts. It runs on the chain-sync tick.
func (n *Node) noteVersionFloor() {
	s, ok := n.VersionFloor()
	if !ok {
		return
	}
	say := func(lvl ports.LogLevel, line string, kv ...any) {
		n.logf(lvl, line, kv...)
		if n.floorNotice != nil {
			n.floorNotice(line)
		}
	}
	if s.Seq != n.floorNotedSeq {
		n.floorNotedSeq = s.Seq
		n.floorEnforceNoted = false
		switch {
		case s.Tier == "recall":
			say(ports.LogInfo, fmt.Sprintf("version-floor: the maintainers recalled the floor (advisory %d); no release is required", s.Seq), "seq", s.Seq)
		case s.BelowFloor:
			when := "it is a warning; nothing is enforced"
			if s.EnforcingAt != 0 {
				when = fmt.Sprintf("from block %d validators stop counting this node's consensus messages", s.EnforcingAt)
			}
			say(ports.LogWarn, fmt.Sprintf("version-floor: this node runs a release below the network's floor — upgrade (release %d, floor %d, tier %s; %s). %s",
				s.Release, s.Floor, s.Tier, when, s.Note), "release", s.Release, "floor", s.Floor, "tier", s.Tier, "enforcingAt", s.EnforcingAt)
		default:
			say(ports.LogInfo, fmt.Sprintf("version-floor: advisory %d in force (floor %d, tier %s); this node runs release %d and meets it", s.Seq, s.Floor, s.Tier, s.Release),
				"release", s.Release, "floor", s.Floor, "tier", s.Tier)
		}
	}
	if s.Enforcing && !n.floorEnforceNoted {
		n.floorEnforceNoted = true
		if s.BelowFloor {
			say(ports.LogWarn, fmt.Sprintf("version-floor: ENFORCED — validators no longer count this node's consensus messages; it still stores and serves (release %d, floor %d)", s.Release, s.Floor),
				"release", s.Release, "floor", s.Floor)
		} else {
			say(ports.LogWarn, fmt.Sprintf("version-floor: ENFORCED — refusing consensus messages from nodes below release %d", s.Floor), "floor", s.Floor)
		}
	}
}

// SubmitAdvisory hands a maintainers' advisory to the validators: verified against this node's
// chain, queued here if this node proposes, and sent to every validator it syncs from.
func (n *Node) SubmitAdvisory(s *advisory.Signed) error {
	if n.chain == nil {
		return errNoChain
	}
	if err := n.chain.AdvisoryAdmissible(s); err != nil {
		return err
	}
	if !n.chainFollower() && n.signer != nil {
		n.queueAdvisory(s)
	}
	raw, err := s.Marshal()
	if err != nil {
		return err
	}
	for _, p := range n.syncTargets() {
		if p == n.id {
			continue
		}
		n.request(p, ports.Message{Kind: ports.MsgSubmitAdvisory, Data: raw}, func(ports.Message, error) {})
	}
	return nil
}

// queueAdvisory keeps the highest-sequence advisory submitted to this node.
func (n *Node) queueAdvisory(s *advisory.Signed) {
	if n.pendingAdvisory != nil && n.pendingAdvisory.Advisory.Seq >= s.Advisory.Seq {
		return
	}
	cp := *s
	n.pendingAdvisory = &cp
}

func (n *Node) handleSubmitAdvisory(from ports.NodeID, msg ports.Message) {
	var s advisory.Signed
	if err := cbor.Unmarshal(msg.Data, &s); err != nil {
		n.reply(from, msg, ports.Message{Kind: ports.MsgSubmitAdvisoryAck, OK: false})
		return
	}
	if err := n.chain.AdvisoryAdmissible(&s); err != nil {
		n.logf(ports.LogInfo, "submitted version-floor advisory refused", "from", from, "err", err)
		n.reply(from, msg, ports.Message{Kind: ports.MsgSubmitAdvisoryAck, OK: false})
		return
	}
	n.queueAdvisory(&s)
	n.logf(ports.LogWarn, "version-floor advisory accepted for on-chain recording", "seq", s.Advisory.Seq,
		"floor", s.Advisory.Floor, "tier", tierName(s.Advisory.Tier), "from", from)
	n.reply(from, msg, ports.Message{Kind: ports.MsgSubmitAdvisoryAck, OK: true})
}

// advisoryPending reports whether a queued advisory is still owed to the chain, dropping one the
// chain has already superseded.
func (n *Node) advisoryPending() bool {
	if n.pendingAdvisory == nil || n.chain == nil {
		return false
	}
	if n.chain.AdvisoryAdmissible(n.pendingAdvisory) != nil {
		n.pendingAdvisory = nil // committed, superseded, or no longer admissible: nothing owed
		return false
	}
	return true
}

// foldAdvisory puts the queued advisory on a witnessable block. It stays queued until the chain
// records it, so a proposal that fails to gather quorum does not lose it.
func (n *Node) foldAdvisory(b *chain.Block) {
	if !n.advisoryPending() || n.chain.MintVersion(b.Height) < chain.BlockVersionWitnessable {
		return
	}
	cp := *n.pendingAdvisory
	b.Advisory = &cp
}
