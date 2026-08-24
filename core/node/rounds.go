// #432 rounds: the node-side state and wire envelopes for the two-phase
// (prepare→precommit) gather with a lock-carrying view-change, per the research
// certification (432-rounds-locking-liveness, 2026-08-15) — PBFT/Tendermint
// locking instantiated over silt's launch anchor majority and mature weight
// quorum. The chain layer (core/chain, era 2) owns WHAT a valid certificate
// is; this file owns WHEN this node signs, locks, advances a round, and what a
// view-change carries.
package node

import (
	"crypto/ed25519"
	"encoding/binary"
	"fmt"

	"github.com/fxamacker/cbor/v2"
	"github.com/nerolabs/silt/core/chain"
	"github.com/nerolabs/silt/ports"
)

// roundAdvanceSweeps is the BASE round duration (certification §5.1 + the
// #451 view-synchronization certification): after sweepsForRound(r) chain-sync
// sweeps at one height with pending work and no commit, a validator broadcasts
// a round-change. A pure function of local deliveries (sweeps are
// message-driven under the sim clock — B2), never wall-clock. Two sweeps ≈
// 60s of real non-progress at the 30s ChainSyncInterval.
const roundAdvanceSweeps = 2

// sweepsForRound is the #451 synchronizer's load-bearing ingredient (a):
// INCREASING round duration — Tendermint's field-proven linear-increment form
// dur(r) = dur(r−1) + r·k (k=1), i.e. dur(r) = base + r(r+1)/2, expressed in
// deterministic sweep counts (never wall-clock — B2/#3, a principled backoff
// per build-immutable #5, not a magic constant). Why it is the GUARANTEE:
// under independent skewed sweep timers a FIXED duration lets the members
// smear across rounds forever (every member advances at the same rate, so
// nothing ever widens a round enough for their round-changes to assemble
// co-round — the field's 14-round/370s stall, run ce15a80-89365, and the
// mark-scatter oracle's deterministic repro); as rounds climb, the duration
// eventually OUTRUNS any timer skew + message delay, so after GST the honest
// members overlap in one round long enough to assemble the quorum — bounded
// convergence, not luck. Ingredient (b), message-driven catch-up
// (maybeCatchUpRound), is the responsive accelerator on top.
func sweepsForRound(r uint64) int {
	return roundAdvanceSweeps + int(r*(r+1)/2)
}

// nodeLock is the validator's #432 lock: the highest-round prepare-QC it has
// witnessed for the height it is working on. Monotone in Round; carried in
// every round-change; persisted with the precommit sign-mark (ports.SignMark
// .LockQC) so a restart re-presents it.
type nodeLock struct {
	Round uint64
	Hash  ports.Hash
	QC    []chain.Attestation
	Block []byte // the locked block's encoding — the re-proposal payload
}

// heightRounds is the per-height round state. It exists only for the height
// this node is currently trying to advance (head+1) and resets when the head
// moves — committed state needs none of it.
type heightRounds struct {
	Height uint64
	Round  uint64 // the round this node currently participates in at Height
	Sweeps int    // sweeps without progress at (Height, Round)
	Lock   *nodeLock
	// Changes collects verified round-change messages: newRound → sender →
	// the raw signed envelope (kept raw so a new-view certificate re-presents
	// exactly what was signed).
	Changes map[uint64]map[ports.NodeID][]byte
}

// roundsFor returns the round state for the CURRENT working height (head+1),
// resetting it if the head has advanced since it was created. The lock does
// NOT survive into a new height: a lock protects a potentially-committed value
// at ITS height, and once some block commits there the question is settled.
func (n *Node) roundsFor() *heightRounds {
	_, next := n.chain.Head()
	if n.rounds == nil || n.rounds.Height != next {
		rs := &heightRounds{Height: next, Changes: map[uint64]map[ports.NodeID][]byte{}}
		// Restart continuity (certification §5.3): if the persisted mark is for
		// this height, resume at ITS round and re-hydrate the lock from the
		// persisted prepare-QC — a restarted validator re-presents the same
		// lock it held before the crash, never a blank one.
		if n.signMarkSet && n.signMark.Height == next {
			rs.Round = n.signMark.Round
			if len(n.signMark.LockQC) > 0 {
				var env prepareQCEnv
				if cbor.Unmarshal(n.signMark.LockQC, &env) == nil {
					if lb, err := chain.Decode(env.Raw); err == nil {
						rs.Lock = &nodeLock{Round: env.Round, Hash: lb.Hash(), QC: env.QC, Block: env.Raw}
					}
				}
			}
		}
		n.rounds = rs
	}
	return n.rounds
}

// ── wire envelopes ────────────────────────────────────────────────────────────

// proposeEnv wraps an era-2 proposal: the block, the round it is proposed at,
// and — for any round > 0 — the NEW-VIEW CERTIFICATE: the quorum of signed
// round-change envelopes that justifies proposing at this round and forces the
// proposer's choice of value (re-propose the highest carried lock, else
// fresh). An attester verifies the certificate before preparing (a proposer
// cannot invent a round).
type proposeEnv struct {
	Raw     []byte   `cbor:"1,keyasint"`
	Round   uint64   `cbor:"2,keyasint,omitempty"`
	NewView [][]byte `cbor:"3,keyasint,omitempty"`
}

// prepareQCEnv carries the assembled prepare-QC back to the attesters for the
// precommit phase: "this value is PREPARED at (h, r) — lock on it and
// precommit". Receiving a valid one IS the POL that justifies (indeed,
// obliges) precommitting its value.
type prepareQCEnv struct {
	Raw   []byte              `cbor:"1,keyasint"`
	Round uint64              `cbor:"2,keyasint"`
	QC    []chain.Attestation `cbor:"3,keyasint"`
}

// roundChangeEnv is one validator's signed "advance to (h, r')" declaration,
// carrying its current lock (if any). The quorum of these is the new-view
// certificate. Signed: a round-change is consensus evidence (it can force a
// proposer's hand), so it must be attributable and unforgeable.
type roundChangeEnv struct {
	Height    uint64              `cbor:"1,keyasint"`
	NewRound  uint64              `cbor:"2,keyasint"`
	Sender    []byte              `cbor:"3,keyasint"` // ed25519 public key
	LockRound uint64              `cbor:"4,keyasint,omitempty"`
	LockQC    []chain.Attestation `cbor:"5,keyasint,omitempty"`
	LockBlock []byte              `cbor:"6,keyasint,omitempty"`
	Sig       []byte              `cbor:"7,keyasint"`
}

const roundChangeSigDomain = "silt/roundchange/v1\x00"

func (rc *roundChangeEnv) sigBytes() []byte {
	buf := make([]byte, 0, len(roundChangeSigDomain)+8*3+32)
	buf = append(buf, roundChangeSigDomain...)
	var u [8]byte
	binary.LittleEndian.PutUint64(u[:], rc.Height)
	buf = append(buf, u[:]...)
	binary.LittleEndian.PutUint64(u[:], rc.NewRound)
	buf = append(buf, u[:]...)
	binary.LittleEndian.PutUint64(u[:], rc.LockRound)
	buf = append(buf, u[:]...)
	lb := ports.HashBytes(rc.LockBlock)
	buf = append(buf, lb[:]...)
	return buf
}

func (rc *roundChangeEnv) senderID() ports.NodeID { return ports.HashBytes(rc.Sender) }

// verifyRoundChange checks one round-change envelope: genuine signature, the
// expected height, a qualified sender, and — when it carries a lock — a lock
// that VERIFIES (the LockQC is a real prepare-QC for the carried block at the
// carried round). An invalid lock invalidates the whole envelope: a Byzantine
// round-changer must not smuggle a forged lock into a new-view certificate
// (schedule S2's misreport half — a forged Y-lock dies here because no
// prepare-QC for Y can exist).
func (n *Node) verifyRoundChange(rc *roundChangeEnv, height uint64) error {
	if rc.Height != height {
		return fmt.Errorf("round-change for height %d, want %d", rc.Height, height)
	}
	if len(rc.Sender) != ed25519.PublicKeySize ||
		!ed25519.Verify(ed25519.PublicKey(rc.Sender), rc.sigBytes(), rc.Sig) {
		return fmt.Errorf("round-change signature invalid")
	}
	if !n.chain.AttesterEligible(rc.senderID()) {
		return fmt.Errorf("round-change from unqualified sender %s", rc.senderID())
	}
	if len(rc.LockQC) > 0 {
		lb, err := chain.Decode(rc.LockBlock)
		if err != nil {
			return fmt.Errorf("round-change locked block: %w", err)
		}
		if lb.Height != height {
			return fmt.Errorf("round-change lock is for height %d, want %d", lb.Height, height)
		}
		if err := n.chain.VerifyPrepareQC(lb, rc.LockQC, rc.LockRound); err != nil {
			return fmt.Errorf("round-change lock QC: %w", err)
		}
	}
	return nil
}

// newViewFor validates a new-view certificate (the raw signed round-change
// envelopes carried by a round->0 proposal): every envelope verifies for
// (height, round), senders are distinct, and together they meet the SAME
// support quorum a commit needs (counted around the r-designated proposer).
// Returns the forced value: the LockBlock of the HIGHEST-round valid lock in
// the set (nil hash ⇒ the proposer was free to propose fresh).
func (n *Node) newViewFor(height, round uint64, raws [][]byte) (forced *nodeLock, err error) {
	if round == 0 {
		return nil, nil // round 0 needs no certificate
	}
	seen := map[ports.NodeID]bool{}
	ids := make([]ports.NodeID, 0, len(raws))
	var best *nodeLock
	for _, raw := range raws {
		var rc roundChangeEnv
		if err := cbor.Unmarshal(raw, &rc); err != nil {
			return nil, fmt.Errorf("new-view envelope: %w", err)
		}
		if rc.NewRound != round {
			return nil, fmt.Errorf("new-view envelope is for round %d, want %d", rc.NewRound, round)
		}
		if err := n.verifyRoundChange(&rc, height); err != nil {
			return nil, err
		}
		id := rc.senderID()
		if seen[id] {
			continue
		}
		seen[id] = true
		ids = append(ids, id)
		if len(rc.LockQC) > 0 && (best == nil || rc.LockRound > best.Round) {
			lb, _ := chain.Decode(rc.LockBlock) // verified above
			best = &nodeLock{Round: rc.LockRound, Hash: lb.Hash(), QC: rc.LockQC, Block: rc.LockBlock}
		}
	}
	designated := n.designatedProposer(height, round)
	if !n.chain.SupportMeetsQuorum(designated, ids, height) {
		return nil, fmt.Errorf("new-view certificate below quorum (%d round-changes)", len(ids))
	}
	return best, nil
}

// designatedProposer rotates the drain-designation with the ROUND as well as
// the height, so a crashed or wedged designated proposer is escaped by the
// next round instead of stalling the height (build-plan risk iii).
func (n *Node) designatedProposer(height, round uint64) ports.NodeID {
	props := n.chain.EligibleProposers()
	if len(props) == 0 {
		return ports.NodeID{}
	}
	return props[int((height+round)%uint64(len(props)))]
}

// adoptLock replaces the node's lock if qc is higher-round (monotone), and
// persists it with the precommit mark via recordSignLock. Returns false if the
// mark could not be persisted (the caller must then not release a signature).
func (n *Node) adoptLock(rs *heightRounds, b *chain.Block, round uint64, qc []chain.Attestation, rawBlock []byte) bool {
	if rs.Lock != nil && rs.Lock.Round >= round {
		// An equal-or-lower-round QC never replaces the lock; equal-round can
		// only be the SAME value (two prepare-QCs at one round would share an
		// honest signer — impossible), so there is nothing to adopt.
		return true
	}
	qcRaw, err := cbor.Marshal(prepareQCEnv{Raw: rawBlock, Round: round, QC: qc})
	if err != nil {
		return false
	}
	if !n.recordSignLock(b.Height, round, chain.PhasePrecommit, b.Hash(), qcRaw) {
		return false
	}
	rs.Lock = &nodeLock{Round: round, Hash: b.Hash(), QC: qc, Block: rawBlock}
	return true
}

// maybeAdvanceRound is the deterministic round-advance rule (certification
// §5.1), called once per chain-sync sweep: with pending consensus work and no
// commit at the working height for roundAdvanceSweeps sweeps, broadcast a
// signed round-change carrying the current lock and enter the next round.
// Quorum-observability: nobody PROPOSES at the new round until a quorum of
// round-changes exists (newViewFor), so one griefing validator can neither
// force nor stall a view-change alone.
func (n *Node) maybeAdvanceRound() {
	if n.chain == nil || !n.chain.Objective() || n.signer == nil {
		return
	}
	// #441 (the launch-face fix, certification §2.4): pending ENTRIES arm the
	// escape exactly like pending regs. Before this, the escape's arming was
	// drain-only — a height whose r0 prepare slots were consumed by a crossed
	// publish race, on a network with momentarily-empty renewal queues, had NO
	// escape driver and stalled until the next renewal arrived (soak run
	// 9453325-7258: 361s > the 160s computed bound). B6 quiescence is preserved
	// when truly idle (no regs, no entries, nothing in flight).
	if len(n.pendingBondRegs) == 0 && len(n.pendingEntries) == 0 && !n.bondDrainInFlight {
		rs := n.roundsFor()
		rs.Sweeps = 0
		return // nothing stuck — quiesce (B6)
	}
	rs := n.roundsFor()
	rs.Sweeps++
	if rs.Sweeps < sweepsForRound(rs.Round) {
		return
	}
	rs.Sweeps = 0
	n.advanceToRound(rs, rs.Round+1, "timeout")
}

// advanceToRound signs + records + broadcasts this node's round-change for
// `next` and enters it — shared by the sweep-count timeout path
// (maybeAdvanceRound) and the #451 catch-up paths (maybeCatchUpRound and the
// valid-higher-round new-view jump). via names the driver for the field read.
func (n *Node) advanceToRound(rs *heightRounds, next uint64, via string) {
	if next <= rs.Round {
		return
	}
	rc := roundChangeEnv{Height: rs.Height, NewRound: next,
		Sender: append([]byte(nil), n.signer.Public().(ed25519.PublicKey)...)}
	if rs.Lock != nil {
		rc.LockRound, rc.LockQC, rc.LockBlock = rs.Lock.Round, rs.Lock.QC, rs.Lock.Block
	}
	rc.Sig = ed25519.Sign(n.signer, rc.sigBytes())
	raw, err := cbor.Marshal(rc)
	if err != nil {
		return
	}
	n.logf(ports.LogInfo, "round-change: advancing (#432 view-change)",
		"height", rs.Height, "round", next, "via", via, "locked", rs.Lock != nil, "pending", len(n.pendingBondRegs), "pending_entries", len(n.pendingEntries))
	// #535 operator visibility (S5): if this height is a mature-epoch boundary
	// whose live-bonded frozen members can no longer reach the frozen >⅔ bar,
	// no round of any ladder can commit it — say so, every escape, so the
	// operator learns WHY the head is stuck and what the recovery path is.
	if n.chain.BoundaryLivenessFloorLost(rs.Height) {
		n.logf(ports.LogWarn, "stalled-at-boundary: live-qualified weight is at or below the frozen 2/3 bar — no live coalition can commit this epoch boundary (#535); if the weight loss is a CONFIRMED real outage (not a partition or attack), a coordinated -liveness-recovery-height at this height is the recovery",
			"height", rs.Height, "round", next)
	}
	// Record our own round-change (we are part of our own quorum), then
	// broadcast to every sync target.
	n.recordRoundChange(rs, next, n.id, raw)
	rs.Round = next
	rs.Sweeps = 0
	for _, p := range n.syncTargets() {
		if p == n.id {
			continue
		}
		n.request(p, ports.Message{Kind: ports.MsgRoundChange, Data: raw}, func(ports.Message, error) {})
	}
}

// recordRoundChange stores a verified round-change and, if this node is the
// designated proposer for (height, newRound) and the quorum is now met, fires
// the drain proposal at that round (re-proposing the forced value if the
// certificate carries one).
func (n *Node) recordRoundChange(rs *heightRounds, newRound uint64, from ports.NodeID, raw []byte) {
	m := rs.Changes[newRound]
	if m == nil {
		m = map[ports.NodeID][]byte{}
		rs.Changes[newRound] = m
	}
	m[from] = raw
	if n.designatedProposer(rs.Height, newRound) != n.id {
		return
	}
	raws := make([][]byte, 0, len(m))
	for _, r := range m {
		raws = append(raws, r)
	}
	forced, err := n.newViewFor(rs.Height, newRound, raws)
	if err != nil {
		return // below quorum (or a bad envelope excluded) — wait for more
	}
	if rs.Round < newRound {
		rs.Round = newRound
	}
	n.proposeAtNewView(rs, newRound, raws, forced)
}

// maybeCatchUpRound is the #451 synchronizer's responsive ingredient (b),
// adopted from PBFT's f+1 view-change rule (B8): when the recorded
// round-changes for rounds ABOVE ours prove an HONEST member ahead (the
// catch-up weight threshold, chain.RoundCatchupMet — f+1 anchors at launch,
// >⅓ frozen weight mature), jump forward to join them at message speed instead
// of timer speed. Changes only WHEN this node is in a round, never which value
// it may sign: the #432 locking is untouched.
//
// JUMP TO THE HIGHEST INDIVIDUALLY-QUALIFYING ROUND (#549 research
// certification 2026-08-24). The threshold is evaluated PER ROUND — the target
// is the highest round whose OWN round-change senders meet RoundCatchupMet — not
// the smallest round of the senders UNIONED across all above-rounds. The prior
// union+smallest rule was a mis-application of PBFT's "smallest view ≥ v" (a
// SUFFIX quorum, where smallest-in-suffix is backed): silt's cross-round union
// is not a suffix, so its smallest member can carry only a fraction of the
// union's weight — a round structurally incapable of forming a QC. Targeting it
// pinned the effective round LOW, so the increasing round duration
// (sweepsForRound) never grew past 3-region WAN + 30s timer skew, so the
// after-GST convergence guarantee never engaged — the field's h68 26-minute
// r1-congestion thrash (#549; deterministic RED home
// modelcheck_549_timed_test.go). Jumping to the HIGHEST qualifying round
// coalesces the weight at the LEADING edge and lets the duration ladder climb.
// Still safe: a round carrying > ⅓ weight of round-changes has ≥ 1 HONEST
// member genuinely there (Byzantine < ⅓ cannot fabricate it), so the jump never
// overshoots past all honest — the same anti-overshoot PBFT's rule sought,
// evaluated per round rather than on the union. I1/locking untouched.
func (n *Node) maybeCatchUpRound(rs *heightRounds) {
	if n.signer == nil || n.chain == nil {
		return
	}
	var target uint64
	for r, m := range rs.Changes {
		if r <= rs.Round {
			continue
		}
		senders := make(map[ports.NodeID]bool, len(m))
		for id := range m {
			senders[id] = true
		}
		// Per-round threshold: this round INDIVIDUALLY proves an honest member
		// there. Keep the highest such round so the ladder climbs, not the
		// smallest of a cross-round union that no single round can commit.
		if n.chain.RoundCatchupMet(senders) && r > target {
			target = r
		}
	}
	if target == 0 {
		return
	}
	n.advanceToRound(rs, target, "catch-up")
}
