package node

import (
	"errors"

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
	return s, true
}

// noteVersionFloor tells the operator about a new advisory once, and about enforcement once it
// starts. It runs on the chain-sync tick.
func (n *Node) noteVersionFloor() {
	s, ok := n.VersionFloor()
	if !ok {
		return
	}
	if s.Seq != n.floorNotedSeq {
		n.floorNotedSeq = s.Seq
		n.floorEnforceNoted = false
		switch {
		case s.Tier == "recall":
			n.logf(ports.LogInfo, "version-floor: the maintainers recalled the floor; no release is required", "seq", s.Seq)
		case s.BelowFloor:
			n.logf(ports.LogWarn, "version-floor: this node runs a release below the network's floor — upgrade",
				"release", s.Release, "floor", s.Floor, "tier", s.Tier, "enforcingAt", s.EnforcingAt, "note", s.Note)
		default:
			n.logf(ports.LogInfo, "version-floor: advisory in force; this node meets it",
				"release", s.Release, "floor", s.Floor, "tier", s.Tier)
		}
	}
	if s.Enforcing && !n.floorEnforceNoted {
		n.floorEnforceNoted = true
		if s.BelowFloor {
			n.logf(ports.LogWarn, "version-floor: ENFORCED — validators no longer count this node's consensus messages; it still stores and serves",
				"release", s.Release, "floor", s.Floor)
		} else {
			n.logf(ports.LogWarn, "version-floor: ENFORCED — refusing consensus messages from nodes below the floor", "floor", s.Floor)
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
