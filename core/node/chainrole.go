// The validator role (M12): a node that keeps a chain replica, judges
// proposals, and helps commit blocks. All chain traffic rides the same
// Transport port as everything else, so the sim exercises consensus
// deterministically and tcpnet carries it over pinned TLS unchanged.
package node

import (
	"bytes"
	"crypto/ed25519"
	"errors"
	"fmt"
	"sort"

	"github.com/fxamacker/cbor/v2"
	"github.com/nerolabs/silt/core/chain"
	"github.com/nerolabs/silt/ports"
)

// EnableChain turns on the validator role: ch is this node's replica,
// priv its signing key (the SAME key whose hash is its NodeID — M10's
// identity doing double duty).
func (n *Node) EnableChain(ch *chain.Chain, priv ed25519.PrivateKey) {
	n.chain = ch
	n.signer = priv
}

// Chain exposes the local replica (dashboards, tests).
func (n *Node) Chain() *chain.Chain { return n.chain }

// SetSignMarkStore wires the durable never-sign-twice watermark (#397 Q1b) and
// loads any persisted mark, so a restarted validator refuses to contradict a
// signature it released on a previous boot (the crash variant of the honest
// double-sign — without this, a crash between signing and committing wipes the
// in-memory mark and the restarted node can be permanently slashed for
// equivocating against itself). A Load error is the caller's refuse-to-start
// condition: running without the mark re-opens exactly that window.
func (n *Node) SetSignMarkStore(s ports.SignMarkStore) error {
	n.signMarkStore = s
	m, ok, err := s.Load()
	if err != nil {
		return err
	}
	if ok {
		n.signMark, n.signMarkSet = m, true
	}
	return nil
}

// signAllowedAt reports whether releasing a consensus signature over block
// (height, hash) at (round, phase) is consistent with the never-sign-twice
// watermark. #432 rounds: the slot is (height, round, phase), ordered
// lexicographically — anything strictly above the mark's slot is fine,
// re-signing the SAME block in the same slot is idempotent, and any OTHER
// signature at or below the mark is the double-sign an honest validator must
// refuse (#397, round-scoped per the certification: a different-hash signature
// at a HIGHER round is honest — that is the liveness escape — while at the
// same (h, r, phase) it is equivocation). The lexicographic order matches an
// honest validator's real signing order: prepare < precommit within a round,
// rounds ascend within a height, heights ascend.
func (n *Node) signAllowedAt(height, round uint64, phase uint8, h ports.Hash) bool {
	if !n.signMarkSet {
		return true
	}
	switch c := slotCompare(height, round, phase, n.signMark); {
	case c > 0:
		return true
	case c == 0:
		return h == n.signMark.Hash
	default:
		return false
	}
}

// slotCompare orders (height, round, phase) against the mark's slot.
func slotCompare(height, round uint64, phase uint8, m ports.SignMark) int {
	switch {
	case height != m.Height:
		if height > m.Height {
			return 1
		}
		return -1
	case round != m.Round:
		if round > m.Round {
			return 1
		}
		return -1
	case phase != m.Phase:
		if phase > m.Phase {
			return 1
		}
		return -1
	default:
		return 0
	}
}

// recordSign advances the watermark to slot (height, round, phase, hash) and,
// when a store is wired, makes it DURABLE before returning — the signature may
// be released to the wire only after this succeeds (mark-then-sign; a crash in
// between leaves an unused mark, which is safe). Returns false if the mark
// could not be persisted: the caller must then NOT release the signature
// (fail-safe: a missed signing turn costs one gather round; a signature
// without a durable mark risks a permanent honest self-slash).
func (n *Node) recordSign(height, round uint64, phase uint8, h ports.Hash) bool {
	return n.recordSignLock(height, round, phase, h, nil)
}

// recordSignLock is recordSign carrying the #432 lock: for a precommit-phase
// mark, lockQC is the CBOR prepare-QC envelope justifying it, persisted WITH
// the mark so a restarted validator re-presents its lock in a round-change
// (certification §5.3). Preserves any existing LockQC when advancing within
// the same height without a new lock (a prepare mark after a precommit cannot
// occur within a height by slot order; a NEW height naturally clears it).
func (n *Node) recordSignLock(height, round uint64, phase uint8, h ports.Hash, lockQC []byte) bool {
	if n.signMarkSet && slotCompare(height, round, phase, n.signMark) == 0 && h == n.signMark.Hash {
		return true // idempotent re-sign of the same block in the same slot
	}
	mark := ports.SignMark{Height: height, Round: round, Phase: phase, Hash: h, LockQC: lockQC}
	if lockQC == nil && n.signMarkSet && n.signMark.Height == height {
		mark.LockQC = n.signMark.LockQC // carry the height's lock forward
	}
	if n.signMarkStore != nil {
		if err := n.signMarkStore.Save(mark); err != nil {
			n.logf(ports.LogWarn, "sign-mark persist FAILED — refusing to sign", "height", height, "round", round, "err", err)
			return false
		}
	}
	n.signMark, n.signMarkSet = mark, true
	return true
}

// CanonicalIssuers is the deterministic, on-chain-bonded issuer set a publisher
// should acquire tokens/credits from for privacy (M0 D3 / F4 §2c): every
// publisher asks the SAME validators, so the subset chosen leaks nothing. Empty
// if there is no chain or no on-chain bonds (caller falls back to its peer list).
func (n *Node) CanonicalIssuers(max int) []ports.NodeID {
	if n.chain == nil {
		return nil
	}
	return n.chain.CanonicalIssuers(max)
}

// canonicalIssuerCap bounds the canonical issuer set a validator serves to a
// chainless publisher — enough to rank a publisher's reachable peers, small enough
// to keep the reply cheap.
const canonicalIssuerCap = 32

// answerCanonicalIssuers replies with this node's deterministic canonical issuer
// ORDERING (validators ranked by committed bond, top first) so a chainless
// publisher can select its publish-token signers by a ledger-derived ranking that
// is the SAME for every publisher — the signer subset then stops being a
// per-publisher quasi-identifier (R-3 / seam-4). Encoded as concatenated 32-byte
// NodeIDs. OK=false if this node holds no chain.
func (n *Node) answerCanonicalIssuers() ports.Message {
	if n.chain == nil {
		return ports.Message{Kind: ports.MsgCanonicalIssuersReply, OK: false}
	}
	ids := n.chain.CanonicalIssuers(canonicalIssuerCap)
	data := make([]byte, 0, len(ids)*len(ports.Hash{}))
	for _, id := range ids {
		data = append(data, id[:]...)
	}
	return ports.Message{Kind: ports.MsgCanonicalIssuersReply, OK: true, Data: data}
}

// FetchCanonicalIssuers asks a chain-holding validator v for the deterministic
// canonical issuer ordering (ranked by committed bond). A chainless publisher uses
// it to pick its publish-token signers by a network-canonical ranking instead of an
// arbitrary subset of its own peer list, closing the signer-subset deanonymization
// channel (R-3, seam-4). done fires with the ids (heaviest bond first) or an error.
func (n *Node) FetchCanonicalIssuers(v ports.NodeID, done func([]ports.NodeID, error)) {
	n.request(v, ports.Message{Kind: ports.MsgGetCanonicalIssuers}, func(resp ports.Message, err error) {
		const idLen = len(ports.Hash{})
		switch {
		case err != nil:
			done(nil, err)
		case !resp.OK || len(resp.Data) == 0 || len(resp.Data)%idLen != 0:
			done(nil, errNoCanonicalIssuers)
		default:
			ids := make([]ports.NodeID, 0, len(resp.Data)/idLen)
			for i := 0; i+idLen <= len(resp.Data); i += idLen {
				var id ports.NodeID
				copy(id[:], resp.Data[i:i+idLen])
				ids = append(ids, id)
			}
			done(ids, nil)
		}
	})
}

// FetchCanonicalIssuersFromAny asks EVERY validator concurrently and returns the
// first success (or the last error once all have failed). This closes the #351
// canonical-set half: a chainless publisher used to ask only vs[0], so a SINGLE
// un-synced or transiently-unreachable validator — e.g. one that just restarted
// mid-run — dropped the publisher into the -peers fallback, which NARROWS the
// publisher anonymity set (swarm.go's honest privacy note). The canonical ranking
// is DETERMINISTIC — every chain-holder computes the same bond-ranked order — so
// racing the fetch is privacy-neutral (research stamp 2026-08-13): who answers
// first changes nothing about WHAT is answered, so no publisher-position
// fingerprint can enter the signer selection through this leg; it is the
// sequential try-each-in-order collapsed to one round-trip time. (The
// token-ACQUISITION-after-restart path — reaching enough signers for the
// token-quorum when one is down — is a separate, privacy-sensitive residual,
// tracked under #351, not addressed here.)
func (n *Node) FetchCanonicalIssuersFromAny(vs []ports.NodeID, done func([]ports.NodeID, error)) {
	if len(vs) == 0 {
		done(nil, errNoCanonicalIssuers)
		return
	}
	pending := len(vs)
	finished := false
	lastErr := errNoCanonicalIssuers
	for _, v := range vs {
		n.FetchCanonicalIssuers(v, func(ids []ports.NodeID, err error) {
			pending--
			if finished {
				return
			}
			if err == nil && len(ids) > 0 {
				finished = true
				done(ids, nil)
				return
			}
			if err != nil {
				lastErr = err
			}
			if pending == 0 {
				finished = true
				done(nil, lastErr)
			}
		})
	}
}

// handleChain processes validator messages; returns false if the kind
// isn't chain-related.
func (n *Node) handleChain(from ports.NodeID, msg ports.Message) bool {
	if n.chain == nil {
		switch msg.Kind {
		case ports.MsgProposeBlock, ports.MsgCommitBlock, ports.MsgGetChain:
			n.reply(from, msg, ports.Message{Kind: replyKind(msg.Kind), OK: false})
			return true
		}
		return false
	}
	switch msg.Kind {
	case ports.MsgProposeBlock:
		// PREPARE phase (#432 two-phase gather). Attest only what we would
		// accept: same rules, our reputation view. The envelope carries the
		// round and (for round > 0) the new-view certificate; a bare legacy
		// block payload is treated as (round 0, no certificate).
		var env proposeEnv
		if cbor.Unmarshal(msg.Data, &env) != nil || len(env.Raw) == 0 {
			env = proposeEnv{Raw: msg.Data} // legacy bare-block payload, round 0
		}
		b, err := chain.Decode(env.Raw)
		if err != nil {
			n.logf(ports.LogDebug, "gather/prepare: DECODE failed", "from", from, "bytes", len(msg.Data), "err", err)
			n.reply(from, msg, ports.Message{Kind: ports.MsgAttestReply, OK: false})
			return true
		}
		if verr := n.chain.ValidateProposal(b); verr != nil {
			n.logf(ports.LogDebug, "gather/prepare: REJECTED (ValidateProposal)", "from", from, "height", b.Height, "bytes", len(msg.Data), "regs", len(b.BondRegs), "reason", verr)
			n.reply(from, msg, ports.Message{Kind: ports.MsgAttestReply, OK: false})
			return true
		}
		rs := n.roundsFor()
		if b.Height == rs.Height {
			// A round > 0 needs its new-view certificate, and the certificate
			// FORCES the proposer's value: re-propose the highest carried lock
			// or (only if none was carried) fresh. A proposer that ignores the
			// forced value is refused — that refusal is what carries a
			// potentially-committed value forward (certification §4).
			forced, nerr := n.newViewFor(b.Height, env.Round, env.NewView)
			if nerr != nil {
				n.logf(ports.LogDebug, "gather/prepare: REFUSED (new-view certificate)", "from", from, "height", b.Height, "round", env.Round, "reason", nerr)
				n.reply(from, msg, ports.Message{Kind: ports.MsgAttestReply, OK: false})
				return true
			}
			if forced != nil && forced.Hash != b.Hash() {
				n.logf(ports.LogInfo, "gather/prepare: REFUSED — proposal ignores the new-view's carried lock (#432 safety)", "from", from, "height", b.Height, "round", env.Round)
				n.reply(from, msg, ports.Message{Kind: ports.MsgAttestReply, OK: false})
				return true
			}
			// Defensive lock rule (Tendermint's belt over PBFT's braces): if WE
			// hold a lock on a different value and this proposal's certificate
			// does not carry a lock at least as high, refuse — our own lock is
			// evidence of a possibly-committed value the certificate missed.
			if rs.Lock != nil && rs.Lock.Hash != b.Hash() && (forced == nil || forced.Round < rs.Lock.Round) {
				n.logf(ports.LogInfo, "gather/prepare: REFUSED — locked on a different value the new-view does not supersede (#432)", "from", from, "height", b.Height, "round", env.Round, "lock_round", rs.Lock.Round)
				n.reply(from, msg, ports.Message{Kind: ports.MsgAttestReply, OK: false})
				return true
			}
		}
		// Never equivocate within a slot: the mark is (height, round, phase)-
		// scoped (#397 round-scoped per #432) and made durable BEFORE the
		// prepare leaves. A different block at a HIGHER round is honest — the
		// liveness escape; at the SAME (h, r, prepare) it is refused.
		if !n.signAllowedAt(b.Height, env.Round, chain.PhasePrepare, b.Hash()) {
			n.logf(ports.LogDebug, "gather/prepare: REFUSED (already signed this slot)", "from", from, "height", b.Height, "round", env.Round)
			n.reply(from, msg, ports.Message{Kind: ports.MsgAttestReply, OK: false})
			return true
		}
		if !n.recordSign(b.Height, env.Round, chain.PhasePrepare, b.Hash()) {
			n.reply(from, msg, ports.Message{Kind: ports.MsgAttestReply, OK: false})
			return true
		}
		att := chain.AttestAt(b, n.signer, env.Round, chain.PhasePrepare)
		raw, _ := attEncode(att)
		n.logf(ports.LogDebug, "gather/prepare: PREPARED", "from", from, "height", b.Height, "round", env.Round, "bytes", len(msg.Data), "regs", len(b.BondRegs))
		n.reply(from, msg, ports.Message{Kind: ports.MsgAttestReply, OK: true, Data: raw})
	case ports.MsgPrepareQC:
		// PRECOMMIT phase (#432): a verified prepare-QC for (h, r) IS the POL —
		// lock on its value (monotone by round, persisted with the mark) and
		// precommit it. The lock is what a round-change carries forward.
		var env prepareQCEnv
		if cbor.Unmarshal(msg.Data, &env) != nil {
			n.reply(from, msg, ports.Message{Kind: ports.MsgPrecommitReply, OK: false})
			return true
		}
		b, err := chain.Decode(env.Raw)
		if err != nil {
			n.reply(from, msg, ports.Message{Kind: ports.MsgPrecommitReply, OK: false})
			return true
		}
		if verr := n.chain.VerifyPrepareQC(b, env.QC, env.Round); verr != nil {
			n.logf(ports.LogDebug, "gather/precommit: REFUSED (prepare-QC invalid)", "from", from, "height", b.Height, "round", env.Round, "reason", verr)
			n.reply(from, msg, ports.Message{Kind: ports.MsgPrecommitReply, OK: false})
			return true
		}
		if !n.signAllowedAt(b.Height, env.Round, chain.PhasePrecommit, b.Hash()) {
			n.logf(ports.LogDebug, "gather/precommit: REFUSED (already signed this slot)", "from", from, "height", b.Height, "round", env.Round)
			n.reply(from, msg, ports.Message{Kind: ports.MsgPrecommitReply, OK: false})
			return true
		}
		rs := n.roundsFor()
		if b.Height == rs.Height {
			if !n.adoptLock(rs, b, env.Round, env.QC, env.Raw) {
				n.reply(from, msg, ports.Message{Kind: ports.MsgPrecommitReply, OK: false})
				return true
			}
		} else if !n.recordSign(b.Height, env.Round, chain.PhasePrecommit, b.Hash()) {
			n.reply(from, msg, ports.Message{Kind: ports.MsgPrecommitReply, OK: false})
			return true
		}
		att := chain.AttestAt(b, n.signer, env.Round, chain.PhasePrecommit)
		raw, _ := attEncode(att)
		n.logf(ports.LogDebug, "gather/precommit: PRECOMMITTED", "from", from, "height", b.Height, "round", env.Round)
		n.reply(from, msg, ports.Message{Kind: ports.MsgPrecommitReply, OK: true, Data: raw})
	case ports.MsgRoundChange:
		// View-change vote (#432): verify, record, ack; recordRoundChange fires
		// the new-view proposal if we are the designated proposer for the new
		// round and the quorum is now met.
		var rc roundChangeEnv
		if cbor.Unmarshal(msg.Data, &rc) != nil {
			n.reply(from, msg, ports.Message{Kind: ports.MsgRoundChangeAck, OK: false})
			return true
		}
		rs := n.roundsFor()
		if rc.Height != rs.Height {
			// Stale (their head is behind) or ahead (ours is) — either way the
			// chain-sync sweep reconciles heads; a round-change is only
			// meaningful at the height we are both stuck on.
			n.reply(from, msg, ports.Message{Kind: ports.MsgRoundChangeAck, OK: false})
			return true
		}
		if err := n.verifyRoundChange(&rc, rs.Height); err != nil {
			n.logf(ports.LogDebug, "round-change: REFUSED", "from", from, "height", rc.Height, "round", rc.NewRound, "reason", err)
			n.reply(from, msg, ports.Message{Kind: ports.MsgRoundChangeAck, OK: false})
			return true
		}
		n.logf(ports.LogInfo, "round-change: recorded (#432 view-change)", "from", rc.senderID(), "height", rc.Height, "round", rc.NewRound, "carries_lock", len(rc.LockQC) > 0)
		n.recordRoundChange(rs, rc.NewRound, rc.senderID(), msg.Data)
		n.reply(from, msg, ports.Message{Kind: ports.MsgRoundChangeAck, OK: true})
	case ports.MsgCommitBlock:
		b, err := chain.Decode(msg.Data)
		ok := err == nil && n.chain.Append(*b) == nil
		if ok {
			n.Stats.BlocksCommitted++
			n.logf(ports.LogInfo, "block committed", "height", b.Height, "entries", len(b.Entries), "regs", len(b.BondRegs), "attestations", len(b.Atts), "via", "broadcast")
			if n.onCommit != nil {
				n.onCommit(*b)
			}
		}
		n.reply(from, msg, ports.Message{Kind: ports.MsgCommitAck, OK: ok})
	case ports.MsgGetChain:
		blocks := n.chain.Blocks(msg.Height)
		n.reply(from, msg, ports.Message{Kind: ports.MsgChainReply, OK: true, Data: chain.EncodeBlocks(blocks)})
	case ports.MsgGetChainHead:
		// Cheap head probe (#382): answer "what is your head?" with (height, hash) so
		// a peer whose head matches ours can SKIP the full-chain fetch + re-validate.
		// A block hash commits its entire ancestry, so an identical head hash proves
		// an identical committed history — nothing to catch up, reorg, or slash.
		hh, next := n.chain.Head()
		var h uint64
		if n.chain.Len() > 0 {
			h = next - 1
		}
		n.reply(from, msg, ports.Message{Kind: ports.MsgChainHeadReply, OK: true, Height: h, Data: hh[:]})
	case ports.MsgSubmitBondReg:
		// A peer submitted a fresh bond renewal for us to include when we next
		// propose (H2 non-proposer renewal). Queue it only if it verifies for our
		// current head — a stale or forged one is dropped on arrival, and the
		// submitter resubmits on its next renewal sweep. The reg is self-signed and
		// self-verifying (bound to the submitter's own key), so accepting a peer's
		// reg grants standing to the PEER, never to us.
		if n.chain != nil && n.chain.Objective() {
			// NEVER refuse silently (B5 / #432): a dropped submit is indistinguishable
			// from a discovery failure to the submitter AND to a field investigator —
			// the wedge's cohort-regs-never-drain symptom was mis-attributed across
			// three runs partly because this branch ate every refusal without a line.
			if reg, err := bondRegDecode(msg.Data); err != nil {
				n.logf(ports.LogInfo, "bond-reg submit REFUSED (decode)", "from", from, "bytes", len(msg.Data), "err", err)
			} else if verr := n.chain.ValidateBondRegErr(reg); verr != nil {
				// A "signature" refusal here is usually TEMPORAL, not forgery: the reg
				// is signed over the submitter's head, and this replica accepts only
				// nonces of its own last-K COMMITTED heads — a reg signed over a head
				// we haven't committed yet (WAN skew: submitter ahead) fails every
				// window nonce and heals on the submitter's next-sweep resubmit (run
				// 09fbe60-84613: 54 refusals, all "signature", all self-healed). Log
				// our next height so a field read can correlate refusal with skew.
				_, next := n.chain.Head()
				n.logf(ports.LogInfo, "bond-reg submit REFUSED", "from", from, "validator", reg.ValidatorID(), "size", reg.Size, "next_height", next, "err", verr)
			} else {
				if n.pendingBondRegs == nil {
					n.pendingBondRegs = make(map[ports.NodeID]chain.BondReg)
				}
				n.pendingBondRegs[reg.ValidatorID()] = reg
			}
		}
		n.reply(from, msg, ports.Message{Kind: ports.MsgSubmitBondRegAck, OK: true})
	default:
		return false
	}
	return true
}

func replyKind(k ports.MsgKind) ports.MsgKind {
	switch k {
	case ports.MsgProposeBlock:
		return ports.MsgAttestReply
	case ports.MsgCommitBlock:
		return ports.MsgCommitAck
	case ports.MsgGetChainHead:
		return ports.MsgChainHeadReply
	default:
		return ports.MsgChainReply
	}
}

// OnCommit registers a callback fired when a block lands in the local
// replica (daemon logging/persistence).
func (n *Node) OnCommit(fn func(chain.Block)) { n.onCommit = fn }

// OnSlash registers a callback fired when this node slashes a validator for a
// proven equivocation (double-sign) detected while reconciling a fork — so the
// daemon can surface this significant accountability event on stdout, not only in
// the debug log. Fired once per distinct culprit per detection.
func (n *Node) OnSlash(fn func(culprit ports.NodeID, height uint64)) { n.onSlash = fn }

// OnReorg registers a callback fired when reconciling a peer's chain REORGS the
// local replica — adopts a heavier competing fork that DROPS previously-committed
// blocks (not a plain catch-up extension). `dropped` is how many committed blocks
// the reorg replaced; newHeight is the height of the new head. A significant,
// operator-visible consensus event (and how #184's partition→heal is observed): a
// partitioned validator, on heal, reorgs its lighter fork onto the heavier one.
func (n *Node) OnReorg(fn func(dropped int, newHeight uint64)) { n.onReorg = fn }

// reorgDropped counts blocks in the pre-reconcile chain (above genesis) that the
// reconciled chain no longer holds at the same height — i.e. blocks a reorg replaced.
// Zero for a pure catch-up extension (the old chain is a prefix of the new one).
func reorgDropped(old, now []chain.Block) int {
	dropped := 0
	for i := 1; i < len(old); i++ { // genesis (i=0) is shared by construction
		if i >= len(now) || old[i].Hash() != now[i].Hash() {
			dropped++
		}
	}
	return dropped
}

var ErrNoChain = errors.New("node: validator role not enabled")

// ProposeEntry runs one round of consensus: build a block at the local
// head, sign it, gather attestations from attesters until quorum,
// commit locally, then broadcast the committed block to every replica
// holder in broadcast (validators attest; ALL replicas hear commits).
// done receives nil once the block is in the LOCAL replica and the
// broadcast has been attempted; peers that missed it will sync.
func (n *Node) ProposeEntry(e ports.Entry, attesters, broadcast []ports.NodeID, quorum int, done func(error)) {
	if n.chain == nil {
		done(ErrNoChain)
		return
	}
	prev, height := n.chain.Head()
	n.proposeBlock(&chain.Block{Version: chain.BlockVersion, Height: height, Prev: prev, Entries: []ports.Entry{e}},
		attesters, broadcast, quorum, done)
}

// ValidateEntryProposal runs the LOCAL pre-check for publishing e — the same checks a
// proposeBlock does before it gathers: proposer eligibility + the entry-level refusals a
// client must learn SYNCHRONOUSLY (no publish token when required, a durable Publisher
// identity the refuse-to-surveil chain rejects, a double-spent token) — WITHOUT gathering
// or committing. The async publish path (chainhost.Host.PublishAsync) calls this to return
// those refusals to the HTTP client immediately, then runs the slow commit gather in the
// background (so the ~1.5MB genesis gather no longer blocks the handler under a flat 10s
// deadline — #286 Layer 1). A lean single-entry candidate block suffices: the entry-level
// refusals and proposer eligibility do not depend on the bond-registration the real
// proposeBlock also attaches (a bad bond-reg surfaces async, as the proposer's own concern).
func (n *Node) ValidateEntryProposal(e ports.Entry) error {
	if n.chain == nil {
		return ErrNoChain
	}
	prev, height := n.chain.Head()
	b := &chain.Block{Version: chain.BlockVersion, Height: height, Prev: prev, Entries: []ports.Entry{e}}
	// Watermark-exempt (#397): this signed candidate exists only for the local
	// pre-check and is NEVER released to the wire — a signature no peer can
	// hold is not equivocation evidence. Only released signatures (proposeBlock
	// gather, attest replies) consult and advance the sign mark.
	chain.Sign(b, n.signer)
	return n.chain.ValidateProposal(b)
}

// ProposeRevocation runs consensus on a TAKEDOWN: an append-only block
// that marks roots denied. It needs the same reputation quorum as a
// publication, so removing content is exactly as governed as adding it —
// no single node can take a file down, and the record of the takedown is
// itself immutable and replicated.
func (n *Node) ProposeRevocation(roots []ports.Hash, attesters, broadcast []ports.NodeID, quorum int, done func(error)) {
	if n.chain == nil {
		done(ErrNoChain)
		return
	}
	prev, height := n.chain.Head()
	n.proposeBlock(&chain.Block{Version: chain.BlockVersion, Height: height, Prev: prev, Revocations: roots},
		attesters, broadcast, quorum, done)
}

// proposeBlock signs b, gathers attestations to quorum, commits locally,
// and broadcasts — shared by entry and revocation proposals.
func (n *Node) proposeBlock(b *chain.Block, attesters, broadcast []ports.NodeID, quorum int, done func(error)) {
	// Gather at least what ValidateCommit will demand: with Byzantine quorum sizing
	// (H4) the chain requires 2f+1 over the qualified set, which can exceed the
	// caller's floor. Under-gathering would just fail our own Append; raise it here.
	if req := n.chain.RequiredQuorum(); req > quorum {
		quorum = req
	}
	// Objective fork-choice (F6): a proposer attaches its own live bond
	// registration, so proposing IS registering — an anchor bootstrapping the
	// network records its real bond in its first block, and every validator
	// renews as it proposes. No-op in legacy mode (BondRegs are ignored) and when
	// the node holds no bond.
	//
	// ONLY when a (re)registration is actually due (BondRenewalDue): not yet in the
	// objective set, or past the TTL renewal point. Re-embedding the full space-time
	// proof in EVERY proposal bought nothing (the latest registration already stands)
	// and on a real cross-region network bloated every block past what attestation
	// could carry in time — WEDGING the chain right after the first bond (#313).
	if n.chain.Objective() && n.bond != nil && n.chain.BondRenewalDue(n.id) {
		if reg, ok := n.RegisterBondReg(b.Prev); ok {
			b.BondRegs = append(b.BondRegs, reg)
		}
	}
	// Fold in peer-submitted renewals (H2 / RT-2): an attest-only validator can't
	// propose, so it SUBMITS its fresh BondReg (MsgSubmitBondReg) and whoever
	// proposes next records it — renewing that validator's TTL clock without it
	// ever proposing. Include only regs still valid for THIS head (ValidateBondReg),
	// so one stale or forged submission can't poison the block; sort by validator
	// id so the block bytes are deterministic. Clear the queue either way — a peer
	// resubmits on its next renewal sweep, so a dropped stale reg is self-healing.
	if n.chain.Objective() && len(n.pendingBondRegs) > 0 {
		fresh := make([]chain.BondReg, 0, len(n.pendingBondRegs))
		for vid, reg := range n.pendingBondRegs {
			if vid != n.id && n.chain.ValidateBondReg(reg) {
				fresh = append(fresh, reg)
			}
		}
		sort.Slice(fresh, func(i, j int) bool {
			a, b := fresh[i].ValidatorID(), fresh[j].ValidatorID()
			return bytes.Compare(a[:], b[:]) < 0
		})
		// #286 Layer 2b: cap the total BYTES of bond registrations embedded per block. A
		// fresh multi-validator genesis where every founding validator submits its ~1.5 MB
		// space-time proof otherwise piles into one ~8 MB block the quorum gather can't move
		// + re-verify over a WAN (the cert stalled at regs=5 / 7.9 MB). The founding set are
		// anchors, so genesis commits SMALL on the anchor bootstrap while the deferred
		// registrations drain over the next blocks. A BYTE budget (not a count) is the right
		// lever: at genesis a full ~1.5 MB proof means ~1 reg/block, but small steady-state
		// renewals pack many per block so an attest-only validator is never starved under a
		// tight TTL (sim/bond_renewal). Embed lowest-id peers first (fresh is sorted) so the
		// drain is deterministic; account for the proposer's own reg (F6) already appended.
		// Un-embedded peers are dropped here and RESUBMIT next block bound to the new head (a
		// reg is signed over BondRegNonce(prev), so it goes stale as the head moves — the same
		// resubmit that keeps the queue live and drains it over blocks).
		budget := n.cfg.MaxBondRegBytesPerBlock
		used := int64(0)
		for _, r := range b.BondRegs { // the proposer's own reg (F6), if any, spends budget first
			used += int64(len(bondRegEncode(r)))
		}
		for _, reg := range fresh {
			sz := int64(len(bondRegEncode(reg)))
			// Always embed at least one reg (never stall the queue on a single oversized
			// proof); otherwise stop once this reg would blow the budget.
			if budget > 0 && len(b.BondRegs) > 0 && used+sz > budget {
				break
			}
			b.BondRegs = append(b.BondRegs, reg)
			used += sz
		}
		// (B) Durable pending queue: KEEP the valid regs that didn't fit this block's
		// byte cap so the drain proceeds at the cap rate on the next block, instead of
		// discarding the whole queue and relying on a re-broadcast to refill it (a race
		// that — with the pre-fix single-head staleness — starved the drain). Drop the
		// embedded (now committed) regs and any that were no longer valid this sweep
		// (not in `fresh`: stale beyond the head window or otherwise invalid). Composes
		// with fix A (the wider validity window) — together the drain runs at ~1/block.
		embedded := make(map[ports.NodeID]bool, len(b.BondRegs))
		for _, r := range b.BondRegs {
			embedded[r.ValidatorID()] = true
		}
		kept := make(map[ports.NodeID]chain.BondReg)
		for _, reg := range fresh {
			if !embedded[reg.ValidatorID()] {
				kept[reg.ValidatorID()] = reg
			}
		}
		n.pendingBondRegs = kept
	}
	// Record any equivocations we detected on-chain, so every replica evicts the
	// culprit from the objective set in lockstep (F2). Drop only records the
	// chain already confirms (IsSlashed); everything else RIDES THIS BLOCK AND
	// STAYS QUEUED until a commit confirms it (#397 Q4-ii — the queue was
	// previously zeroed after one attempt, so a slash whose carrier proposal
	// failed to gather quorum was silently dropped and the culprit's on-chain
	// eviction could be lost).
	if len(n.pendingSlashes) > 0 {
		var still []chain.Equivocation
		for _, e := range n.pendingSlashes {
			if n.chain.IsSlashed(e.CulpritID()) {
				continue
			}
			b.Slashes = append(b.Slashes, e)
			still = append(still, e)
		}
		n.pendingSlashes = still
	}
	b.Version = chain.BlockVersionRounds // era 2: minted blocks carry two-phase certificates (#432)
	chain.Sign(b, n.signer)
	if err := n.chain.ValidateProposal(b); err != nil {
		done(fmt.Errorf("propose: local pre-check: %w", err))
		return
	}
	// The round this proposal runs at, with its new-view certificate for any
	// round > 0 (assembled by recordRoundChange; a proposer cannot invent a
	// round — attesters verify the certificate).
	rs := n.roundsFor()
	round := uint64(0)
	var newView [][]byte
	if b.Height == rs.Height {
		round = rs.Round
		if round > 0 {
			for _, r := range rs.Changes[round] {
				newView = append(newView, r)
			}
			// The forced-value rule binds US too: if the certificate carries a
			// lock for a different value, this fresh proposal must yield (the
			// caller's work stays pending; the locked value is re-proposed by
			// proposeAtNewView).
			if forced, err := n.newViewFor(b.Height, round, newView); err != nil {
				done(fmt.Errorf("propose height %d round %d: new-view certificate not ready: %w", b.Height, round, err))
				return
			} else if forced != nil && forced.Hash != b.Hash() {
				done(fmt.Errorf("propose height %d round %d: the new-view carries a lock for a different value — re-propose that (#432)", b.Height, round))
				return
			}
		}
	}
	n.gatherTwoPhase(b, attesters, broadcast, quorum, round, newView, nil, done)
}

// gatherTwoPhase runs the #432 two-phase gather for b at (b.Height, round):
// prepare quorum → prepare-QC → lock (durable) → precommit quorum → commit +
// broadcast. b may be this node's own freshly-signed proposal OR a FORCED
// re-proposal from a new-view certificate — a re-proposed block keeps its
// ORIGINAL author (re-signing would change the hash and orphan the lock).
// The gatherer always contributes its own prepare/precommit: as an attester
// they count like any other's; as the AUTHOR they count toward nothing
// (validation counts the proposer by authorship) but are REQUIRED evidence —
// the round-scoped self-prepare is what makes a double-proposal at one (h, r)
// slashable (chain.requireProposerPrepare). `carried` is the forced lock's
// prepare-QC on a re-proposal (nil otherwise): the absent original author's
// self-prepare is lifted from it into the fresh certificate, still valid at
// its lower round (the hash excludes the round; the author is exempt from the
// round-exactness rule).
func (n *Node) gatherTwoPhase(b *chain.Block, attesters, broadcast []ports.NodeID, quorum int, round uint64, newView [][]byte, carried []chain.Attestation, done func(error)) {
	// Never sign twice in a slot (#397, round-scoped per #432): our PREPARE
	// enters the same never-sign-twice ledger as any attestation, durable
	// BEFORE anything is released — whether or not it ever commits. A
	// different block at a HIGHER round stays signable: that is the #432
	// liveness escape the height-only mark lacked.
	if !n.signAllowedAt(b.Height, round, chain.PhasePrepare, b.Hash()) {
		done(fmt.Errorf("propose height %d round %d: already signed a different block in this slot (never-sign-twice, #397/#432)", b.Height, round))
		return
	}
	if !n.recordSign(b.Height, round, chain.PhasePrepare, b.Hash()) {
		done(fmt.Errorf("propose height %d: sign-mark could not be persisted — refusing to sign (#397 Q1b)", b.Height))
		return
	}
	raw := chain.Encode(b)
	envRaw, err := cbor.Marshal(proposeEnv{Raw: raw, Round: round, NewView: newView})
	if err != nil {
		done(fmt.Errorf("propose: encode envelope: %w", err))
		return
	}
	n.logf(ports.LogDebug, "gather: starting (two-phase)", "height", b.Height, "round", round, "bytes", len(raw), "regs", len(b.BondRegs), "quorum", quorum, "attesters", len(attesters))

	// supportMet: would this coalition commit? The count floor is the caller's
	// `quorum`; the chain adds the regime gates (anchor majority / mature >⅔
	// weight) directly — identical for BOTH phases (POL threshold = commit
	// threshold, certification §4).
	supportMet := func(atts []chain.Attestation) bool {
		ids := make([]ports.NodeID, 0, len(atts))
		for _, a := range atts {
			ids = append(ids, a.AttesterID())
		}
		// Counted around the block's AUTHOR (which may not be this node on a
		// new-view re-proposal) — mirrors ValidateCommit exactly.
		return n.chain.SupportMeetsQuorum(b.ProposerID(), ids)
	}
	// counted: the caller's `quorum` floor is a NON-AUTHOR attestation count
	// (the proposer is counted by authorship, never by signature — the #402
	// arithmetic), so the author's own required self-signatures in the
	// certificate must not satisfy it. Mirrors collectQuorumSigs's
	// proposer-skip exactly.
	counted := func(atts []chain.Attestation) int {
		nc := 0
		for _, a := range atts {
			if a.AttesterID() != b.ProposerID() {
				nc++
			}
		}
		return nc
	}

	// PHASE 2 (precommit): runs once the prepare-QC is assembled and locked.
	gatherPrecommits := func(qc []chain.Attestation) {
		qcRaw, err := cbor.Marshal(prepareQCEnv{Raw: raw, Round: round, QC: qc})
		if err != nil {
			done(fmt.Errorf("propose: encode prepare-QC: %w", err))
			return
		}
		// Our own precommit ALWAYS — as attester it counts; as author it is
		// count-neutral extra evidence (mark already durable: adoptLock /
		// recordSign ran before gatherPrecommits was entered).
		pcs := []chain.Attestation{chain.AttestAt(b, n.signer, round, chain.PhasePrecommit)}
		var askPC func(i int)
		askPC = func(i int) {
			if counted(pcs) >= quorum && supportMet(pcs) {
				b.CommitRound = round
				b.PrepareQC = qc
				b.Atts = pcs
				if err := n.chain.Append(*b); err != nil {
					done(fmt.Errorf("propose: commit rejected by own replica: %w", err))
					return
				}
				n.Stats.BlocksCommitted++
				n.logf(ports.LogInfo, "block committed", "height", b.Height, "round", round, "entries", len(b.Entries), "regs", len(b.BondRegs), "attestations", len(pcs), "via", "proposal")
				if n.onCommit != nil {
					n.onCommit(*b)
				}
				n.broadcastCommit(b, broadcast, 0, func() { done(nil) })
				return
			}
			if i >= len(attesters) {
				n.logf(ports.LogDebug, "gather: NO PRECOMMIT QUORUM", "height", b.Height, "round", round, "gathered", counted(pcs), "needed", quorum)
				done(fmt.Errorf("propose height %d round %d: %w: %d precommits of %d gathered",
					b.Height, round, chain.ErrNoQuorum, counted(pcs), quorum))
				return
			}
			v := attesters[i]
			if v == n.id {
				askPC(i + 1)
				return
			}
			n.request(v, ports.Message{Kind: ports.MsgPrepareQC, Data: qcRaw},
				func(resp ports.Message, err error) {
					switch {
					case err != nil:
						n.logf(ports.LogDebug, "gather: precommit request FAILED", "to", v, "height", b.Height, "err", err)
					case !resp.OK:
						n.logf(ports.LogDebug, "gather: precommit REFUSED", "to", v, "height", b.Height)
					default:
						if att, aerr := attDecode(resp.Data); aerr == nil {
							pcs = append(pcs, att)
						}
					}
					askPC(i + 1)
				})
		}
		askPC(0)
	}

	// PHASE 1 (prepare): gather prepare signatures to the quorum, assemble the
	// prepare-QC, LOCK on it (durable — the lock is what a round-change
	// carries), then run the precommit phase. The gatherer's own prepare goes
	// in first (recorded above); on a forced re-proposal the ORIGINAL author's
	// self-prepare is lifted from the carried lock QC so the fresh certificate
	// keeps satisfying requireProposerPrepare (the author may be down — the
	// reason the view changed).
	atts := []chain.Attestation{chain.AttestAt(b, n.signer, round, chain.PhasePrepare)}
	if n.id != b.ProposerID() {
		for _, a := range carried {
			if a.Phase == chain.PhasePrepare && a.AttesterID() == b.ProposerID() {
				atts = append(atts, a)
				break
			}
		}
	}
	var ask func(i int)
	ask = func(i int) {
		if counted(atts) >= quorum && supportMet(atts) {
			qc := atts
			if b.Height == n.roundsFor().Height {
				if !n.adoptLock(n.roundsFor(), b, round, qc, raw) {
					done(fmt.Errorf("propose height %d: lock could not be persisted — refusing to precommit (#432)", b.Height))
					return
				}
			} else if !n.recordSign(b.Height, round, chain.PhasePrecommit, b.Hash()) {
				done(fmt.Errorf("propose height %d: sign-mark could not be persisted (#397 Q1b)", b.Height))
				return
			}
			n.logf(ports.LogDebug, "gather: prepare-QC assembled — LOCKED", "height", b.Height, "round", round, "prepares", len(qc))
			gatherPrecommits(qc)
			return
		}
		if i >= len(attesters) {
			n.logf(ports.LogDebug, "gather: NO PREPARE QUORUM", "height", b.Height, "round", round, "bytes", len(raw), "gathered", counted(atts), "needed", quorum)
			done(fmt.Errorf("propose height %d round %d: %w: %d prepares of %d gathered",
				b.Height, round, chain.ErrNoQuorum, counted(atts), quorum))
			return
		}
		v := attesters[i]
		if v == n.id {
			ask(i + 1)
			return
		}
		n.logf(ports.LogDebug, "gather: requesting prepare", "to", v, "height", b.Height, "round", round, "bytes", len(raw), "have", counted(atts), "need", quorum)
		n.request(v, ports.Message{Kind: ports.MsgProposeBlock, Data: envRaw},
			func(resp ports.Message, err error) {
				switch {
				case err != nil:
					n.logf(ports.LogDebug, "gather: prepare request FAILED", "to", v, "height", b.Height, "err", err)
				case !resp.OK:
					n.logf(ports.LogDebug, "gather: prepare REFUSED", "to", v, "height", b.Height)
				default:
					if att, aerr := attDecode(resp.Data); aerr == nil {
						atts = append(atts, att)
						n.logf(ports.LogDebug, "gather: prepare collected", "from", v, "height", b.Height, "have", counted(atts), "need", quorum)
					} else {
						n.logf(ports.LogDebug, "gather: prepare DECODE failed", "from", v, "height", b.Height, "err", aerr)
					}
				}
				ask(i + 1)
			})
	}
	ask(0)
}

// proposeAtNewView is the view-change proposal path (#432): fired by
// recordRoundChange when this node is the designated proposer for (height,
// round) and the round-change quorum is assembled. Re-proposes the FORCED
// value (the certificate's highest carried lock — keeping its ORIGINAL author,
// straight into the two-phase gather) if any; otherwise a fresh drain proposal
// for the pending queue via proposeBlock. Attesters re-verify the same
// certificate, so an equivocating designated proposer gains nothing.
func (n *Node) proposeAtNewView(rs *heightRounds, round uint64, newView [][]byte, forced *nodeLock) {
	if n.bondDrainInFlight {
		return // one proposal in flight at a time; the next sweep retries
	}
	peers := n.syncTargets()
	attesters := make([]ports.NodeID, 0, len(peers))
	for _, p := range peers {
		if n.chain.AttesterEligible(p) {
			attesters = append(attesters, p)
		}
	}
	if len(attesters) == 0 {
		return
	}
	n.bondDrainInFlight = true
	fin := func(err error) {
		n.bondDrainInFlight = false
		if err != nil {
			n.logf(ports.LogDebug, "new-view proposal not committed", "height", rs.Height, "round", round, "err", err)
		}
	}
	n.logf(ports.LogInfo, "new-view proposal (#432 view-change)", "height", rs.Height, "round", round, "forced", forced != nil)
	if forced != nil {
		lb, err := chain.Decode(forced.Block)
		if err != nil {
			fin(err)
			return
		}
		n.gatherTwoPhase(lb, attesters, peers, 0, round, newView, forced.QC, fin)
		return
	}
	prevHead, height := n.chain.Head()
	if height != rs.Height {
		fin(nil)
		return
	}
	n.proposeBlock(&chain.Block{Version: chain.BlockVersionRounds, Height: height, Prev: prevHead}, attesters, peers, 0, fin)
}

func (n *Node) broadcastCommit(b *chain.Block, validators []ports.NodeID, i int, done func()) {
	if i >= len(validators) {
		done()
		return
	}
	if validators[i] == n.id {
		n.broadcastCommit(b, validators, i+1, done)
		return
	}
	n.request(validators[i], ports.Message{Kind: ports.MsgCommitBlock, Data: chain.Encode(b)},
		func(ports.Message, error) { n.broadcastCommit(b, validators, i+1, done) })
}

// slashEquivocators finds validators who signed a different block at the same
// height across two competing histories and slashes each in the local ledger —
// a proven double-sign costs standing (D2). Called on DETECTION (every fetched
// peer chain vs the local one, seam-7), not only on adoption, so a double-sign
// onto a losing fork is caught too. The evidence is self-verifying
// (chain.VerifyEquivocation, inside FindEquivocations), so this cannot be
// triggered by an honest validator signing sequential heights. On-chain
// inclusion so every replica evicts in lockstep is the recorded follow-up (the
// pendingSlashes queue); here each validator acts on what it sees.
func (n *Node) slashEquivocators(a, b []chain.Block) {
	if n.ledger == nil {
		return
	}
	for _, e := range chain.FindEquivocations(a, b) {
		cid := e.CulpritID()
		// Idempotent-once (#397 Q4-i): a live fork is re-observed by EVERY
		// reconcile sweep until it heals, so the same double-sign is re-detected
		// over and over — the field wedge re-slashed and re-logged both culprits
		// every ~2s indefinitely. The local penalty, warn line, and callback
		// fire once per culprit; the ON-CHAIN record has its own lifecycle
		// (pendingSlashes requeues until a commit confirms it).
		if n.slashedLocal[cid] {
			continue
		}
		n.slashedLocal[cid] = true
		n.ledger.SlashEquivocation(cid) // legacy/rep path
		// Queue the proof for on-chain recording so the OBJECTIVE set evicts the
		// culprit in lockstep on every replica (F2), not just this local ledger.
		if n.chain != nil && !n.chain.IsSlashed(cid) {
			n.pendingSlashes = append(n.pendingSlashes, e)
		}
		n.logf(ports.LogWarn, "validator slashed for equivocation", "culprit", cid, "height", e.A.Height)
		if n.onSlash != nil {
			n.onSlash(cid, e.A.Height)
		}
	}
}

// SyncChain reconciles the local replica against peers — how a latecomer or a
// restarted daemon catches up AND how a partitioned validator heals a fork
// (D2). For each peer it first sends a CHEAP HEAD PROBE (#382): if the peer's
// head hash equals ours we are provably on the identical committed history (a
// block hash commits its whole ancestry), so there is nothing to catch up,
// reorg, or slash — we skip the peer entirely. Only on a head DIFFERENCE (peer
// ahead, or a divergent fork) — or when a peer is too old to answer the probe —
// do we fall back to fetching the peer's FULL chain and asking the replica to
// Reconcile: a peer that merely extends us is adopted as a catch-up, a peer on a
// heavier competing fork triggers a reorg, and an equal-or-lighter history is
// ignored — one uniform path (an equal-length fork, invisible to "give me blocks
// above my head", is exactly why we compare whole chains, not suffixes). Every
// block is fully re-validated inside Reconcile, so a lying peer wastes our time
// but cannot feed us an invalid or foreign chain.
//
// The head probe is what makes steady-state sync cheap: without it every sweep
// re-fetched and re-validated every peer's ENTIRE chain — O(chain × peers) bytes
// and CPU per sweep even when the whole network already agreed (#382 M1 cost). It
// is trust-NEUTRAL: the full fetch + Reconcile + equivocation scan is unchanged
// and runs on every real difference; the probe only elides work when the peer's
// head is byte-identical to ours. (A genuine genesis-to-head block DIFF within
// Reconcile is a further follow-up; this closes the dominant no-op-sweep cost.)
func (n *Node) SyncChain(peers []ports.NodeID, done func(added int, err error)) {
	if n.chain == nil {
		done(0, ErrNoChain)
		return
	}
	added := 0
	var ask func(i int)
	// fetchFull runs the unchanged full-chain fetch + slash + Reconcile against
	// peer p, then advances to the next peer. Used whenever the head probe shows a
	// difference or cannot be answered.
	fetchFull := func(p ports.NodeID, next func()) {
		n.Stats.ChainSyncFullFetches++
		n.request(p, ports.Message{Kind: ports.MsgGetChain, Height: 0},
			func(resp ports.Message, err error) {
				if err == nil && resp.OK {
					if full, derr := chain.DecodeBlocks(resp.Data); derr == nil && len(full) > 0 {
						before := n.chain.Len()
						old := n.chain.Blocks(0) // snapshot to catch cross-fork double-signs
						// Slash on DETECTION, not on adoption (seam-7). Scan the fetched
						// peer chain against our LOCAL one for cross-fork double-signs
						// BEFORE the heavier test and regardless of whether we adopt: a
						// validator that signed a block at a height we hold AND a
						// conflicting block at that height on this peer's fork is provably
						// guilty even if its fork is LIGHTER and never reconciled onto.
						// Previously this ran only on the adopted branch, so a double-sign
						// onto a doomed/losing fork (to confuse late joiners, split gossip,
						// or bait a partition) cost the actor nothing. The evidence is
						// self-verifying (chain.VerifyEquivocation), so an honest
						// sequential signer is never caught. This subsumes the old
						// adopted-branch (old,now) scan, since the adopted fork is `full`.
						n.slashEquivocators(old, full)
						if ok, rerr := n.chain.Reconcile(full); ok {
							now := n.chain.Blocks(0)
							if d := n.chain.Len() - before; d > 0 {
								added += d
							}
							if dropped := reorgDropped(old, now); dropped > 0 && n.onReorg != nil {
								n.onReorg(dropped, uint64(len(now)-1))
							}
							n.logf(ports.LogInfo, "chain reconciled from peer", "peer", p, "len", n.chain.Len())
						} else if rerr != nil {
							n.logf(ports.LogDebug, "peer chain not adopted", "peer", p, "err", rerr)
						}
					}
				}
				next()
			})
	}
	ask = func(i int) {
		if i >= len(peers) {
			done(added, nil)
			return
		}
		if peers[i] == n.id {
			ask(i + 1)
			return
		}
		p := peers[i]
		// Head probe first. Compare against our CURRENT head each peer (it may have
		// moved if we adopted from an earlier peer this sweep).
		n.request(p, ports.Message{Kind: ports.MsgGetChainHead},
			func(resp ports.Message, err error) {
				ourHead, _ := n.chain.Head()
				if err == nil && resp.Kind == ports.MsgChainHeadReply && resp.OK && len(resp.Data) == len(ourHead) {
					var peerHead ports.Hash
					copy(peerHead[:], resp.Data)
					if peerHead == ourHead {
						// Identical head ⇒ identical committed history: nothing to do.
						n.Stats.ChainSyncHeadMatches++
						ask(i + 1)
						return
					}
				}
				// Head differs, or the peer is too old to answer the probe (or it
				// timed out) — fall back to the full fetch, which preserves every
				// catch-up / reorg / equivocation-detection guarantee.
				fetchFull(p, func() { ask(i + 1) })
			})
	}
	ask(0)
}

// StartChainSync begins the periodic reconciliation sweep in which this
// validator catches its replica up against the validators it knows — the
// restart/partition recovery path (F1). It fires once now and then reschedules
// on ChainSyncInterval. Catch-up MUST retry rather than fire once at boot: a
// restarted node re-earns its view of peer reputation live via bond audits, so
// the peers whose blocks it needs to adopt may not clear the qualification bar
// the instant the daemon starts — a later sweep, once audits have landed,
// succeeds. seed is the explicit -attesters set (may be empty); every validator
// this node has since learned a bond from is also a target, so a node restarted
// WITHOUT -attesters still catches up. onCatchUp fires (added > 0) after an
// adoption so the caller can persist the grown chain.
func (n *Node) StartChainSync(seed []ports.NodeID, onCatchUp func(added int)) {
	if n.chain == nil || n.chainSyncRunning {
		return
	}
	n.chainSyncRunning = true
	n.chainSyncSeed = seed
	n.chainSyncOnCatchUp = onCatchUp
	n.chainSyncTick()
}

func (n *Node) chainSyncTick() {
	peers := n.syncTargets()
	if len(peers) > 0 {
		n.SyncChain(peers, func(added int, _ error) {
			if added > 0 && n.chainSyncOnCatchUp != nil {
				n.chainSyncOnCatchUp(added)
			}
			// Drain pending bond registrations AFTER the reconcile settles, so the
			// proposal builds on the freshest head this sweep can know (#338).
			n.maybeProposeBondDrain()
			// #432: count non-progress sweeps and fire a round-change when the
			// working height is stuck with pending work (the deterministic,
			// quorum-observable view-change trigger).
			n.maybeAdvanceRound()
		})
		// Renew objective standing without proposing (H2 / RT-2): submit a fresh
		// bond proof to the same validator set we reconcile against, so an
		// attest-only validator sustains its TTL-bound standing. No-op off the
		// objective path or with no bond.
		n.SubmitBondRenewal(peers)
	}
	n.clock.AfterFunc(n.cfg.ChainSyncInterval, n.chainSyncTick)
}

// maybeProposeBondDrain is the #338 REACTIVE drain: a proposer-eligible
// validator holding peer-submitted bond registrations (or whose own
// registration is due) proposes a BondRegs-only block, so a young objective
// network drains its deferred founding bonds WITHOUT depending on unrelated
// publish traffic. Before this, pending registrations were only ever folded
// into publish/revocation proposals — on an idle young network nothing
// proposed, the #336-deferred regs sat forever, no validator (anchors
// included) earned committed standing, and maturity was unreachable (the
// `nakamoto 0 bonds` local state; the "anchors hadn't banked the Sybil bonds"
// field GAP). Reactive, not eager (B6): it fires only when pending state
// exists and quiesces when the queue is empty and every bond is current; the
// per-block registration byte budget (#286 L2b) applies inside proposeBlock as
// on any other proposal. Racing drains from two eligible proposers converge
// like racing publishes (fork-choice; under §3 finality a quorum commit is
// final), and the loser's peers simply resubmit.
func (n *Node) maybeProposeBondDrain() {
	if n.chain == nil || !n.chain.Objective() || n.signer == nil || n.bondDrainInFlight {
		return
	}
	ownDue := n.bond != nil && n.chain.BondRenewalDue(n.id)
	if len(n.pendingBondRegs) == 0 && !ownDue {
		n.drainWaitSweeps = 0
		return // nothing pending — stay quiet (B6)
	}
	if !n.chain.ProposerEligible(n.id) {
		return // not our block to make (a young non-anchor waits for an anchor to drain it)
	}
	prevHead, height := n.chain.Head()
	// NEVER sign twice at one height (Tendermint locking): if we already signed
	// a block at this height — attested OR proposed — a drain proposal here
	// would be a second signature on a different block, which a peer's
	// cross-fork scan reads as equivocation. #432: the check is SLOT-scoped —
	// a fresh proposal needs the (height, current round, prepare) slot to sit
	// strictly above the mark; a marked slot at this height no longer blocks
	// the height forever, because maybeAdvanceRound moves the round and the
	// next round's slot is signable (the liveness escape).
	if rs := n.roundsFor(); n.signMarkSet && slotCompare(height, rs.Round, chain.PhasePrepare, n.signMark) <= 0 {
		// NEVER refuse silently when work is being blocked (B5 / #432): pending
		// regs + a blocked slot, sweep after sweep, is the wedge signature —
		// the one line that would have named the field stall on the first run.
		// maybeAdvanceRound (same sweep cadence) is what unblocks it.
		if len(n.pendingBondRegs) > 0 {
			n.logf(ports.LogInfo, "bond-reg drain blocked at own sign slot — awaiting round advance (#432)",
				"height", height, "round", rs.Round, "mark_height", n.signMark.Height, "mark_round", n.signMark.Round, "pending", len(n.pendingBondRegs))
		}
		return
	}
	// One designated drain proposer per height, derived from COMMITTED state so
	// every honest replica agrees (chain.EligibleProposers is sorted): this
	// makes an honest drain race structurally impossible, rather than merely
	// unlikely.
	//
	// Two #397 (research-certified) race-closures on top of the designated rule:
	//
	//  · SUBMIT-DON'T-PROPOSE (Q2b-2): our OWN renewal being due is never by
	//    itself a reason for a NON-designated proposer to propose —
	//    SubmitBondRenewal already broadcast the fresh reg on this same sweep
	//    (chainSyncTick), so it sits in every eligible proposer's pending queue
	//    and the DESIGNATED one drains it. This removes the field wedge's
	//    driver: genesis-aligned renewal clocks made two anchors both propose
	//    the same height (b88245d-3496).
	//
	//  · STAGGERED TAKEOVER (Q2b-1): if the designated proposer hasn't drained
	//    after 3 idle sweeps, eligible proposers take over ONE PER SWEEP in
	//    rank order (rank distance from the designated), instead of all rushing
	//    at once — a dual-proposer race can then only arise if a takeover
	//    proposer is itself silent for a full sweep, and the never-sign-twice
	//    watermark still bounds that residue.
	dist := 0
	if props := n.chain.EligibleProposers(); len(props) > 0 {
		self := -1
		for i, p := range props {
			if p == n.id {
				self = i
				break
			}
		}
		if self >= 0 {
			d := int(height) % len(props)
			dist = (self - d + len(props)) % len(props)
		}
	}
	if dist > 0 {
		if len(n.pendingBondRegs) == 0 {
			// Own renewal only, and we are not the designated proposer: submit,
			// never propose (Q2b-2). The reg reaches the chain via the
			// designated proposer's queue; nothing to take over for.
			n.drainWaitSweeps = 0
			return
		}
		if n.drainWaitSweeps < 3+dist {
			n.drainWaitSweeps++
			return
		}
		// designated (and every closer rank) idle past its window — our turn
	}
	n.drainWaitSweeps = 0
	// Attesters: only peers whose attestation will actually COUNT (the gather
	// collects any verifying signature, but ValidateCommit skips unqualified
	// ones — soliciting them would fail our own commit). Broadcast: everyone we
	// sync with, so non-attester replicas hear the commit too.
	peers := n.syncTargets()
	attesters := make([]ports.NodeID, 0, len(peers))
	for _, p := range peers {
		if n.chain.AttesterEligible(p) {
			attesters = append(attesters, p)
		}
	}
	if len(attesters) == 0 {
		return // no qualified co-signer reachable; retry next sweep
	}
	b := &chain.Block{Version: chain.BlockVersion, Height: height, Prev: prevHead}
	n.bondDrainInFlight = true
	n.proposeBlock(b, attesters, peers, 0, func(err error) {
		n.bondDrainInFlight = false
		if err != nil {
			// A lost race or a not-yet-warm quorum — peers resubmit and the next
			// sweep retries; debug-level because this is the normal retry path.
			n.logf(ports.LogDebug, "bond-reg drain proposal not committed", "height", height, "err", err)
			return
		}
		n.logf(ports.LogInfo, "bond-reg drain: pending registrations committed", "height", height)
	})
}

// syncTargets is the set of validators to reconcile against this sweep: the
// explicit seed, the CONFIGURED static consensus tier (-persistent-peers), plus
// every peer we've learned a storage bond from (a bond is what a validator
// advertises), minus ourselves. The static tier is included because it IS the
// consensus set by definition (#338 finding 2 / network-durability §8,
// configure-not-discover): a validator whose attester seed holds no
// chain-carrying peer (the cloud sybil cohort — its -attesters are only other
// sybils) and whose bond gossip hasn't warmed yet otherwise has NO path to the
// committed chain — it can neither catch up nor submit its bond registration,
// which is exactly the "sybil never synced a committed chain (head 0)" field
// GAP. The rest is learned lazily from gossip, so the set fills in as the
// swarm comes into view — a node restarted with only a -bootstrap peer still
// discovers the rest and catches up.
func (n *Node) syncTargets() []ports.NodeID {
	set := make(map[ports.NodeID]bool, len(n.chainSyncSeed)+len(n.staticPeers)+len(n.peerBonds))
	for _, id := range n.chainSyncSeed {
		if id != n.id {
			set[id] = true
		}
	}
	for id := range n.staticPeers {
		if id != n.id {
			set[id] = true
		}
	}
	for id := range n.peerBonds {
		if id != n.id {
			set[id] = true
		}
	}
	out := make([]ports.NodeID, 0, len(set))
	for id := range set {
		out = append(out, id)
	}
	return out
}

// attestations share the block CBOR mode via small wrappers. The wrapper
// is a chain.Block, so it carries the current version like any other —
// chain.Decode (used by attDecode) requires it.
func attEncode(a chain.Attestation) ([]byte, error) {
	b := chain.Block{Version: chain.BlockVersion, Atts: []chain.Attestation{a}}
	return chain.Encode(&b), nil
}

func attDecode(raw []byte) (chain.Attestation, error) {
	b, err := chain.Decode(raw)
	if err != nil || len(b.Atts) != 1 {
		return chain.Attestation{}, fmt.Errorf("bad attestation payload")
	}
	return b.Atts[0], nil
}

// bond renewals ride the same block-CBOR wrapper as attestations, so a submitted
// BondReg (MsgSubmitBondReg) travels the wire without a new codec.
func bondRegEncode(r chain.BondReg) []byte {
	b := chain.Block{Version: chain.BlockVersion, BondRegs: []chain.BondReg{r}}
	return chain.Encode(&b)
}

func bondRegDecode(raw []byte) (chain.BondReg, error) {
	b, err := chain.Decode(raw)
	if err != nil || len(b.BondRegs) != 1 {
		return chain.BondReg{}, fmt.Errorf("bad bondreg payload")
	}
	return b.BondRegs[0], nil
}
