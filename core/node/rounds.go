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
// message-driven under the sim clock — B2), never wall-clock.
//
// WHY 2, DERIVED (#549 cert Q3, 2026-08-24 — not a magic constant, build-
// immutable #5): the base round duration must OUTRUN the cross-region skew in
// when honest members enter a common round. That skew is STRUCTURALLY bounded
// by ChainSyncInterval: each node's chainSyncTick fires once per interval at an
// arbitrary phase (a mass restart re-randomizes it), so two nodes' timeout
// triggers differ by < ChainSyncInterval; WAN delivery (~80ms netem) is
// negligible on top. base = roundAdvanceSweeps × ChainSyncInterval, so
// roundAdvanceSweeps = 1 (= 30s = exactly the skew) has ZERO overlap margin and
// is unreliable; = 2 (= 60s = 2× the skew) is the SMALLEST integer base that
// reliably outruns it (a worst-case-late member still overlaps 30s inside the
// round); = 3+ is larger than necessary — slower recovery + more churn on the
// 2 GB box, against the cert's "smallest base, not larger" M1 guidance. Even
// round 0 (duration = base) outruns the skew, so the ladder outruns skew from
// the first round. The derivation is guarded by TestRoundBaseOutrunsSkew; if
// the round-change timer is ever decoupled from the chain-sync tick, or
// ChainSyncInterval made per-node-variable, the < ChainSyncInterval skew bound
// must be re-derived (docs/thinking/2026-08-24-549-q3-round-duration.md).
//
// #555 ADDENDUM (2026-08-25): the deep-drive crawl asked whether the two-phase
// GATHER latency G, not skew, is the binding lower bound (the #555 cert held
// this base pending a measured G). Measured: intrinsic G ≈ 10 s at 12-seat WAN
// (95d39e8-deep h74) — the crawl's 90–150 s "gather" was event-loop saturation
// from redundant Block.Hash work on the chain-sync path (fixed: hash memo,
// chain.TestReconcileHashWorkIsLinear_555). Skew remains binding; base stays 2
// (docs/thinking/2026-08-25-555-crawl-attribution.md).
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
	// Armed (h43, D-CONSENSUS-ARMING (A)): this node has VERIFIED at least one
	// consensus message for this height — a proposal, a prepare-QC, a
	// round-change or a round certificate — so the round clock runs here
	// whether or not this node holds pending work of its own. The arming
	// condition is REPLICATED (one member's first round-change arms every
	// recipient within one hop), which is the precondition every published
	// liveness bound silently assumes: all correct members are in the
	// pacemaker (PBFT §4.4 request-arms-the-timer, restored to network
	// uniformity; Tendermint L21). Recreated with the height, so a commit
	// disarms every seat — B6 quiescence is exactly today's when nothing is
	// in flight anywhere.
	Armed bool
	// CertSent (h43 (B)): the rounds whose transferable certificate this node
	// has already broadcast or received — a certificate travels one hop from
	// its assembler (G-H43-4), never floods.
	CertSent map[uint64]bool
	// Certs (h43, G-H43-12): the quorum-grade certificate this node holds per
	// round, VERIFIED ONCE when first assembled or received. Later arrivals for
	// a round already held cost no signature work — the verification budget is
	// O(N) per round per node, not O(N³) (the as-built first cut re-ran
	// newViewFor on every arrival and on every envelope of every received
	// certificate; the delta certification priced that at ~1,000–1,700
	// verifies per round per node at N = 12).
	Certs map[uint64]*roundCert
	// Attempted (h43, G-H43-13): the mempool signature at this node's last
	// designee proposal attempt per round. A designee proposes at most once per
	// (h, r) unless its mempool changed — the as-built first cut re-fired on
	// every certificate envelope (~40 attempts in one round, each paying the
	// fold + era roots + Sign before the empty-block check).
	Attempted map[uint64]uint64
}

// roundCert is a held quorum-grade round certificate: the envelopes it was
// assembled or received with (round-exact — every NewRound == Round) and the
// forced value newViewFor derived from them (nil ⇒ fresh proposal allowed).
type roundCert struct {
	Raws   [][]byte
	Forced *nodeLock
}

// roundsFor returns the round state for the CURRENT working height (head+1),
// resetting it if the head has advanced since it was created. The lock does
// NOT survive into a new height: a lock protects a potentially-committed value
// at ITS height, and once some block commits there the question is settled.
func (n *Node) roundsFor() *heightRounds {
	_, next := n.chain.Head()
	if n.rounds == nil || n.rounds.Height != next {
		rs := &heightRounds{Height: next, Changes: map[uint64]map[ports.NodeID][]byte{}, CertSent: map[uint64]bool{},
			Certs: map[uint64]*roundCert{}, Attempted: map[uint64]uint64{}}
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

// roundCertEnv is the TRANSFERABLE round certificate (h43, D-CONSENSUS-ARMING
// (B); the DiemBFT/Jolteon TC schema, generalised to all peers as Tendermint's
// gossip does): the quorum of signed round-change envelopes for exactly
// (Height, Round). Carries no signature of its own — every envelope inside is
// individually signed and re-verified by the receiver (newViewFor), so the
// object is exactly as trustworthy as a proposal-carried new-view certificate
// and a relay cannot forge one. Wire object only: never a block field, never a
// transition or fork-choice input (I5).
type roundCertEnv struct {
	Height uint64   `cbor:"1,keyasint"`
	Round  uint64   `cbor:"2,keyasint"`
	Raws   [][]byte `cbor:"3,keyasint"`
}

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
	if rc.NewRound == 0 {
		// `R-H43-CERT-ROUND-ZERO-UNVERIFIED` (C-3): a round-change for round 0
		// cannot exist — advanceToRound only ever produces next ≥ 1 — and
		// newViewFor's round-0 short-circuit verifies NOTHING, so letting one
		// in would let checkRoundQuorum cache and broadcast an unverified
		// round-0 "certificate".
		return fmt.Errorf("round-change for round 0 is meaningless")
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
	// h43 / D-CONSENSUS-ARMING (A) — the arming rule is REPLICATED: the clock
	// runs while this node holds pending work (regs — #338; entries — #441
	// §2.4; a drain in flight) OR it has verified any consensus message for
	// the working height (rs.Armed). Before this, the guard read only LOCAL
	// mempool content, so the round number was a function of unreplicated
	// private state: on run c450985-deep a 13-seat network ran its pacemaker
	// on the 3 seats holding entries while 10 sat at r0 for ten minutes, and
	// the certified #451 bound was proved over a population that did not
	// exist on the wire (M1; G-H43-1). B6 quiescence is preserved when TRULY
	// idle — no work here, nothing seen for this height — which is exactly
	// today's idle behaviour; the price is ≤ N round timers per CONTESTED
	// height, driven off the existing sweep, no new timer source.
	//
	// M1b: when disarmed, HOLD the sweep counter — never zero it. Zeroing
	// discarded accumulated progress on every momentarily-empty sweep, which
	// is why the field's val-b (a work-holding validator) stopped laddering
	// after r3. Reset-on-quiescence is not reset-on-entry (DiemBFT Fig. 1
	// resets the timer on ENTERING a round — advanceToRound does that).
	rs := n.roundsFor()
	if len(n.pendingBondRegs) == 0 && len(n.pendingEntries) == 0 && !n.bondDrainInFlight && !rs.Armed {
		return // truly idle — quiesce (B6), counter held
	}
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
	// ENTER the round first (the timer resets on entry — DiemBFT Fig. 1), THEN
	// record our own round-change (we are part of our own quorum): recording
	// may complete the certificate for `next`, and checkRoundQuorum decides
	// whether to enter a round by reading rs.Round — with the round already
	// entered it takes the designee branch directly instead of recursing
	// through advanceToRound's `next <= rs.Round` guard (a tidy-up, not a
	// necessity — PE F-10). Then broadcast to every sync target.
	rs.Round = next
	rs.Sweeps = 0
	// h43 (D3): the #338 takeover walk is keyed on (height + round) in the
	// drain path, so its rank distance tracks THIS round's designee. The
	// accumulated wait is deliberately NOT reset here: rounds 0–3 last 2/3/5/8
	// sweeps while a rank-k walk needs 3+k, so a per-round reset could never
	// reach a far rank until the ladder outgrew it (the #441 rotation-wait
	// oracle went RED on exactly that) — monotone accumulation keeps the
	// certified ≤ (N+2)·ChainSyncInterval backstop for a designee whose
	// forwarded work was lost, at the price of the pre-existing #397 Q2b-1
	// residue (a near-rank taker may race the designee at one (h, r); the
	// watermark bounds it to one value per attester per slot).
	n.recordRoundChange(rs, next, n.id, raw)
	for _, p := range n.syncTargets() {
		if p == n.id {
			continue
		}
		n.request(p, ports.Message{Kind: ports.MsgRoundChange, Data: raw}, func(ports.Message, error) {})
	}
	n.forwardPendingWorkToDesignee(rs.Height, next)
}

// h43ForwardEntries caps the ENTRIES one work-holder forwards to a round's
// designee on entering the round (h43 (D1), G-H43-10a; a security parameter,
// owner call owed). Its OWN constant, never entrySubmitBurst: that budget was
// derived for one CLIENT's honest cadence, whereas here up to N−1 forwarders
// fire at ONE seat on every round entry — at N = 12 inheriting 32 would land
// 352 ValidateEntry calls (an RSA verify each under -require-tokens) on the
// designee inside one window. The designee needs ONE entry to make a
// non-empty block; 4 is headroom over one and far under the client burst, so
// a genuine client submit in the same window is never starved. FIFO, so a
// longer queue drains over rounds with seniority intact.
const h43ForwardEntries = 4

// forwardPendingWorkToDesignee (h43 (D1), certified on the ENTRY lane —
// `R-H43-WORKLESS-DESIGNEE`, M4): a round whose designee is LIVE but holds
// none of the height's pending work is wasted like a round on a DOWN
// designee — silt refuses an empty block (a validity rule, chain.go
// "chain: empty block") and the designee has priority at its round — so the
// ≤ f+1 bound is only as good as the designee's mempool. The literature never
// has this problem because the request reaches every replica (PBFT §4.4: the
// client multicasts to all; Tendermint gossips the mempool), whereas silt's
// entry lane reaches only the peers the CLIENT knew and a receiver never
// re-gossips. So, on entering round r, a work-holder forwards its queued
// ENTRIES to the designee of r over the same submit lane a client uses —
// an entry is self-validating (root, token, serial), re-validated on arrival
// and again at fold, so a forwarded entry is indistinguishable from a client
// submit. Leader-directed mempool gossip: ≤ h43ForwardEntries messages per
// round entry, to ONE peer.
//
// REGISTRATIONS ARE NOT FORWARDED (REFUTED by the delta certification):
// the reg lane is sender-bound — a receiver refuses any reg not submitted by
// its own validator BEFORE the ~ms space-time verify ("submit REFUSED
// (relay)", the #424 CPU-DoS closer) — so a forwarded reg is dropped 100 % of
// the time after ~1.5 MB of egress and after burning the forwarder's own
// per-window submit budget at the designee. And it is redundant: an owner's
// SubmitBondRenewal already broadcasts its due reg to every peer it syncs
// with, so in the field every eligible proposer already holds it.
func (n *Node) forwardPendingWorkToDesignee(height, round uint64) {
	d := n.designatedProposer(height, round)
	if d == (ports.NodeID{}) || d == n.id {
		return
	}
	entries := 0
	for i := range n.pendingEntries {
		if entries >= h43ForwardEntries {
			break
		}
		n.request(d, ports.Message{Kind: ports.MsgSubmitEntry, Data: entryEncode(n.pendingEntries[i].E)}, func(ports.Message, error) {})
		entries++
	}
	if entries > 0 {
		n.logf(ports.LogInfo, "round-change: pending entries forwarded to the round's designee (h43)", "height", height, "round", round, "designee", d, "entries", entries)
	}
}

// maybeProposeAtRound (h43 (D)): work that reaches the designee AFTER its
// round certificate assembled must not wait for the next sweep — if this node
// is the designee for its current round > 0 and already holds the quorum
// certificate, propose now. Called on every queued submission.
func (n *Node) maybeProposeAtRound() {
	if n.chain == nil || !n.chain.Objective() || n.signer == nil || n.bondDrainInFlight {
		return
	}
	rs := n.roundsFor()
	if rs.Round == 0 {
		return
	}
	c := rs.Certs[rs.Round]
	if c == nil {
		return // no certificate yet — checkRoundQuorum fires when it completes
	}
	n.fireDesignee(rs, rs.Round, c)
}

// recordRoundChange stores a verified round-change (arming this node — h43
// (A)) and, once the round-EXACT quorum for newRound is met at THIS node: (i)
// broadcasts the certificate once as a transferable object (h43 (B), the
// DiemBFT TC — before this, only the designee ever assembled it, and a
// round-change was one-shot, unacked and un-relayed, so a dropped one was
// lost forever); (ii) enters newRound if above ours (the same entry rule a
// proposal-carried certificate already triggers); (iii) if this node is the
// designated proposer for (height, newRound), fires the proposal at THAT
// round (re-proposing the forced value if the certificate carries one).
//
// Cost (re-derived after the delta certification's `R-H43-VERIFY-COST-
// UNDERSTATED`): every envelope stored here was verified ONCE by its wire
// handler; newViewFor re-verifies the stored set on each arrival until the
// round's quorum forms, then the certificate is CACHED (rs.Certs) and no
// later arrival for that round costs a signature — O(N) verifies per arrival
// before quorum, so ≤ O(N²) per round per node in the worst case and O(N) once
// the certificate is held; a received certificate costs one newViewFor over
// its envelopes, and is refused unverified when the round is already held or
// the envelope count exceeds the governing set (G-H43-12).
func (n *Node) recordRoundChange(rs *heightRounds, newRound uint64, from ports.NodeID, raw []byte) {
	n.storeRoundChange(rs, newRound, from, raw)
	n.checkRoundQuorum(rs, newRound)
}

// storeRoundChange records one verified envelope and arms this node (h43 (A)).
func (n *Node) storeRoundChange(rs *heightRounds, newRound uint64, from ports.NodeID, raw []byte) {
	m := rs.Changes[newRound]
	if m == nil {
		m = map[ports.NodeID][]byte{}
		rs.Changes[newRound] = m
	}
	m[from] = raw
	rs.Armed = true
}

// checkRoundQuorum acts on the recorded envelopes for `round`: if this node
// already holds the round's certificate, only the (deduped) designee fire is
// re-evaluated — no signature work; otherwise newViewFor verifies the stored
// set ONCE, and on quorum the certificate is cached, sent (narrowed — see
// broadcastRoundCert), entered if above our round, and proposed at if we are
// its designee.
func (n *Node) checkRoundQuorum(rs *heightRounds, round uint64) {
	if round == 0 {
		return // round 0 needs no certificate and newViewFor verifies nothing there (C-2)
	}
	if c := rs.Certs[round]; c != nil {
		if round < rs.Round {
			return // a stale arrival for a round we have left never re-fires a proposal (PE F-8)
		}
		n.fireDesignee(rs, round, c)
		return
	}
	m := rs.Changes[round]
	raws := make([][]byte, 0, len(m))
	for _, r := range m {
		raws = append(raws, r)
	}
	forced, err := n.newViewFor(rs.Height, round, raws)
	if err != nil {
		return // below quorum (or a bad envelope excluded) — wait for more
	}
	c := &roundCert{Raws: raws, Forced: forced}
	rs.Certs[round] = c
	if !rs.CertSent[round] {
		rs.CertSent[round] = true
		n.broadcastRoundCert(rs.Height, round, raws, m)
	}
	if rs.Round < round {
		// A quorum-grade certificate for a round above ours is proof the
		// network is there: enter it. advanceToRound records our own envelope,
		// which re-enters here at rs.Round == round and fires the designee
		// branch below if it is ours.
		n.advanceToRound(rs, round, "round-cert")
		return
	}
	n.fireDesignee(rs, round, c)
}

// fireDesignee proposes at `round` if this node is its designee, at most once
// per (h, r) unless the mempool changed since the last attempt (h43,
// G-H43-13). The designee has PRIORITY, never exclusivity: the #338 takeover
// (re-keyed to the round, D3) lets another work-holder propose at the same
// round after its window, and every attester admits either — I1 rests on the
// watermark and the forced value, never on who proposed.
func (n *Node) fireDesignee(rs *heightRounds, round uint64, c *roundCert) {
	if n.designatedProposer(rs.Height, round) != n.id {
		return
	}
	sig := n.mempoolSig()
	if last, tried := rs.Attempted[round]; tried && last == sig {
		return
	}
	if n.proposeAtNewView(rs, round, c.Raws, c.Forced) {
		rs.Attempted[round] = sig
	}
}

// mempoolSig is a cheap signature of this node's foldable work — the queue
// lengths and the issuer-key foldability — so a designee re-attempts a round
// only when something it could carry has changed.
//
// Packing (PE F-11): the queues are each capped at maxMempool (entries and
// regs; slashes are bounded by the culprit set), far below the 2^20 field
// width, so the fields cannot collide; if a queue cap ever exceeds 2^20 the
// shifts must widen with it.
func (n *Node) mempoolSig() uint64 {
	sig := uint64(len(n.pendingEntries))<<40 | uint64(len(n.pendingBondRegs))<<20 | uint64(len(n.pendingSlashes))<<8
	if n.issuerKeysFoldable() {
		sig |= 1
	}
	if n.bond != nil && n.chain.BondRenewalDue(n.id) {
		sig |= 2
	}
	return sig
}

// broadcastRoundCert sends the assembled round certificate for (height,
// round) ONCE, to the round's DESIGNEE (DiemBFT: "sends the TC to L_{r+1}")
// plus the peers whose envelopes are ABSENT from it — the ones that evidently
// have not declared the round and so may never have seen the round-changes
// that formed it (G-H43-4: a node whose only copy of a peer's round-change
// was dropped still learns the round within one hop of whoever assembled the
// quorum). NEVER to every peer: a round-change carries the sender's full
// locked block (sigBytes commits HashBytes(LockBlock)), so a certificate is
// O(N · block) once locks are carried, and up to N nodes can assemble one
// independently before any hears another's — an all-peers broadcast was
// priced at ~1.2 GB per contested round at N = 12 (G-H43-11,
// `R-H43-CERT-CARRIES-BLOCKS`). Fire-and-forget like a round-change: the
// certificate is idempotent evidence, and every jump it causes gossips a
// fresh round-change of its own.
func (n *Node) broadcastRoundCert(height, round uint64, raws [][]byte, present map[ports.NodeID][]byte) {
	data, err := cbor.Marshal(roundCertEnv{Height: height, Round: round, Raws: raws})
	if err != nil {
		return
	}
	designee := n.designatedProposer(height, round)
	sent := 0
	for _, p := range n.syncTargets() {
		// The GOVERNING set only (PE F-5, build-immutables #4/#8): syncTargets
		// is seed ∪ static ∪ every peer that ever advertised a bond root — up
		// to maxPeerInfo storage-tier peers that can never use a certificate.
		if p == n.id || !n.chain.AttesterEligibleAt(p, height) || (p != designee && present[p] != nil) {
			continue
		}
		n.request(p, ports.Message{Kind: ports.MsgRoundCert, Data: data}, func(ports.Message, error) {})
		sent++
	}
	n.logf(ports.LogInfo, "round-cert: sent (h43 transferable certificate)", "height", height, "round", round, "envelopes", len(raws), "to", sent, "designee", designee)
}

// acceptRoundCert is the receive side of the transferable certificate (h43
// (B)): validate it with exactly the rule an attester applies to a
// proposal-carried certificate (newViewFor — every envelope signed by a
// qualified sender for this height and round, distinct senders, the same
// support quorum a commit needs), cache it, record its envelopes as if each
// had arrived directly (arming this node and advancing the declared rounds),
// ENTER the round if it is above ours, and fire the designee's proposal if
// that is us. The entry is made HERE (PE F-9), not left to the handler's
// following maybeCatchUpRound. Returns an error if the certificate is for
// another height, for round 0, empty, over the governing-set cap, or invalid.
func (n *Node) acceptRoundCert(rs *heightRounds, env *roundCertEnv) error {
	if env.Height != rs.Height {
		return fmt.Errorf("round-cert for height %d, want %d", env.Height, rs.Height)
	}
	if env.Round == 0 || len(env.Raws) == 0 {
		// `R-H43-CERT-ROUND-ZERO-UNVERIFIED` (C-1, the composed-diff
		// certification's merge blocker): newViewFor returns (nil, nil) at
		// round 0 — the SAME shape as "verified, distinct senders, at quorum"
		// — without decoding a byte, so a round-0 "certificate" would let any
		// peer, with no signature and no eligibility, write attacker-chosen
		// envelopes under attacker-chosen sender IDs at attacker-chosen rounds
		// into rs.Changes: a forced jump to any round, permanent per-round
		// certificate poisoning (newViewFor hard-fails a set on its first bad
		// envelope), and amplification through the round-0 designee's
		// proposal. Refuse before anything else; every write into rs.Changes
		// is then preceded by a verification (#424 class, fourth recurrence).
		return fmt.Errorf("round-cert for round %d with %d envelopes is meaningless", env.Round, len(env.Raws))
	}
	if rs.Certs[env.Round] != nil {
		return nil // already held: no signature work (G-H43-12)
	}
	// G-H43-12 (`R-H43-CERT-UNBUDGETED-VERIFY`, the #424 class): bound the work
	// BEFORE any signature — a certificate can carry at most one envelope per
	// governing-set member, so anything larger is malformed by construction.
	if cap := n.chain.GoverningSetCap(); len(env.Raws) > cap {
		return fmt.Errorf("round-cert carries %d envelopes, governing set is %d", len(env.Raws), cap)
	}
	forced, err := n.newViewFor(env.Height, env.Round, env.Raws)
	if err != nil {
		return err
	}
	rs.CertSent[env.Round] = true // received, not assembled: one hop, no re-broadcast
	rs.Certs[env.Round] = &roundCert{Raws: env.Raws, Forced: forced}
	for _, raw := range env.Raws {
		var rc roundChangeEnv
		if cbor.Unmarshal(raw, &rc) != nil {
			continue // newViewFor decoded and verified every envelope above
		}
		n.storeRoundChange(rs, rc.NewRound, rc.senderID(), raw)
	}
	if rs.Round < env.Round {
		n.advanceToRound(rs, env.Round, "round-cert") // records our own envelope and fires the designee branch
		return nil
	}
	n.checkRoundQuorum(rs, env.Round)
	return nil
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
//
// SUFFIX SEMANTICS (h43, D-CONSENSUS-ARMING (B), 2026-09-07). A round-change
// for r is the claim "I am at round ≥ r" (PBFT §4.5.2), so the per-round
// membership is {senders whose DECLARED round ≥ r} — each sender's highest
// recorded round (declaredRounds) — not the point-in-time Changes[r]. The #549 target rule is unchanged — still the
// highest individually-qualifying round — but evaluated over a suffix, which
// is monotone-decreasing in r and therefore well-defined. Under point
// semantics a node at r4 was invisible at r2 and r3, so the high rounds an
// armed minority climbed could never self-prove and the catch-up target was
// structurally the LOWEST quorum-bearing round (the field's ten seats jumping
// to r1 while the frontier sat at r4/r5 — M3, G-H43-3).
func (n *Node) maybeCatchUpRound(rs *heightRounds) {
	if n.signer == nil || n.chain == nil {
		return
	}
	var target uint64
	declared := rs.declaredRounds()
	for _, r := range distinctRoundsAbove(declared, rs.Round) {
		senders := make(map[ports.NodeID]bool, len(declared))
		for id, d := range declared {
			if d >= r {
				senders[id] = true
			}
		}
		// Per-round threshold over the SUFFIX: round r proves an honest member
		// at or beyond it. Keep the highest such round so the ladder climbs.
		if n.chain.RoundCatchupMet(senders) && r > target {
			target = r
		}
	}
	if target == 0 {
		return
	}
	n.advanceToRound(rs, target, "catch-up")
}

// declaredRounds derives each sender's DECLARED round — the highest round it
// has a recorded round-change for — from the recorded envelopes (h43 (B),
// suffix semantics). Derived, never a second store: whatever populates
// Changes (the wire handler, a received certificate, our own advance)
// populates this.
func (rs *heightRounds) declaredRounds() map[ports.NodeID]uint64 {
	declared := map[ports.NodeID]uint64{}
	for r, m := range rs.Changes {
		for id := range m {
			if declared[id] < r {
				declared[id] = r
			}
		}
	}
	return declared
}

// distinctRoundsAbove lists the distinct declared rounds strictly above
// `above` — the candidate catch-up targets (h43 (B)).
func distinctRoundsAbove(declared map[ports.NodeID]uint64, above uint64) []uint64 {
	seen := map[uint64]bool{}
	var out []uint64
	for _, d := range declared {
		if d > above && !seen[d] {
			seen[d] = true
			out = append(out, d)
		}
	}
	return out
}
