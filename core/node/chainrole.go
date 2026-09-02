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
			// #451 synchronizer ingredient (b), second face: a VALID new-view
			// certificate for a round above ours is quorum-grade proof the
			// network is there — jump before attesting, so our round state
			// (and our subsequent round-changes) track the frontier instead
			// of climbing to it one timeout at a time.
			if env.Round > rs.Round {
				n.advanceToRound(rs, env.Round, "new-view")
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
		// #451 synchronizer ingredient (b): if the recorded round-changes above
		// our round now prove an honest member ahead (f+1 / >⅓ weight), jump —
		// responsive catch-up at message speed (PBFT), never waiting out the
		// full local timeout while the frontier moves on.
		n.maybeCatchUpRound(rs)
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
			n.pruneOnCommit() // shed heavy proofs below the horizon (slice 5b)
		}
		n.reply(from, msg, ports.Message{Kind: ports.MsgCommitAck, OK: ok})
	case ports.MsgGetChain:
		blocks := n.chain.Blocks(msg.Height)
		// RED-TEAM / TEST-HARNESS: an objective-mode equivocator serves its crafted
		// LOSING fork instead of its real chain (adversary.go PlaceConflictingSigned),
		// so a peer fetches the conflicting signed block and slashes the double-sign
		// on detection. The fork is invalid (a lone prepare, quorum-short) so no
		// honest node ever ADOPTS it — it is slashable evidence only. nil on any
		// honest node.
		if n.equivServedFork != nil {
			blocks = n.equivServedFork
		}
		// Serve a byte-bounded WINDOW, not the whole suffix (#466): marshaling a
		// bond-reg-laden chain into one buffer was the measured 144 MB serve-side
		// OOM driver. The requester loops windows (fetchFull); Reconcile validates
		// the reassembled linkage, so a windowed fetch cannot corrupt the chain.
		n.reply(from, msg, ports.Message{Kind: ports.MsgChainReply, OK: true,
			Data: chain.EncodeBlocksUpTo(blocks, n.maxChainReplyBytes())})
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
		// RED-TEAM / TEST-HARNESS: an objective-mode equivocator advertises its
		// crafted fork's head so a peer whose head matches the honest chain does
		// NOT skip the fetch on the #382 head probe — it fetches, sees the
		// conflicting signed block, and slashes. Without this the probe short-
		// circuits the double-sign (the fork's L@H hashes differently than W@H).
		if fork := n.equivServedFork; fork != nil {
			last := fork[len(fork)-1]
			fh := last.Hash()
			n.reply(from, msg, ports.Message{Kind: ports.MsgChainHeadReply, OK: true, Height: last.Height, Data: fh[:]})
			return true
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
			//
			// The CPU gate runs FIRST (Phase 1.2, the #424 shape): every well-formed
			// self-signed reg forces up to one VerifySpaceTime (~ms on the single
			// loop, core/bond/verifycost_bench_test.go), so submits are budgeted per
			// sender per sweep window BEFORE decode — a refusal costs a map lookup,
			// and a refused honest submit heals by the resubmit-next-sweep retry.
			if !n.allowBondSubmit(from) {
				n.logf(ports.LogInfo, "bond-reg submit REFUSED (rate)", "from", from, "budget", bondSubmitBurst)
			} else if n.chain.IsSlashed(from) {
				// #503 Q1(a): an F2-evicted identity can never re-earn standing
				// (apply skips it), yet its reg still validated, folded, and
				// COMMITTED as a fresh ~1.5 MB block every sweep — the bond-renewal
				// storm. Refuse at arrival, before decode: the sender-binding rule
				// below guarantees a queued reg is always the sender's own, so
				// IsSlashed(from) is exactly IsSlashed(validator), for a map lookup.
				n.logf(ports.LogInfo, "bond-reg submit REFUSED (evicted)", "from", from)
			} else if reg, err := bondRegDecode(msg.Data); err != nil {
				n.logf(ports.LogInfo, "bond-reg submit REFUSED (decode)", "from", from, "bytes", len(msg.Data), "err", err)
			} else if vid := reg.ValidatorID(); vid != from {
				// A submit is always the sender's OWN renewal (SubmitBondRenewal
				// self-submits; the reg is bound to the submitter's key). A relayed
				// third-party reg is refused BEFORE signature/proof work — otherwise
				// replaying one captured valid ~1.5 MB reg re-pays the full verify
				// per message (queue dedup sits after the verify).
				n.logf(ports.LogInfo, "bond-reg submit REFUSED (relay)", "from", from, "validator", vid)
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
				n.queuePendingBondReg(reg)
			}
		}
		n.reply(from, msg, ports.Message{Kind: ports.MsgSubmitBondRegAck, OK: true})
	case ports.MsgSubmitEntry:
		// #441: a publisher submitted an entry for this node's next block to
		// carry — entries are MEMPOOL CONTENT, never a second proposal stream.
		// Validate on arrival against the current head and refuse invalid ones
		// SYNCHRONOUSLY with the reason (certification §2.2: the client learns
		// immediately, an invalid-entry flood never reaches the designee's
		// fold, and nothing is dropped silently — B5).
		if n.chain == nil {
			n.reply(from, msg, ports.Message{Kind: ports.MsgSubmitEntryAck, OK: false, Data: []byte("no chain")})
			return true
		}
		// CPU gate FIRST (#183 red-team F-1, the #424/allowBondSubmit shape):
		// under -require-tokens, ValidateEntry runs an RSA verify per token
		// signature, so an un-budgeted flood rides per-message crypto onto the
		// single loop. Charge one unit before decode; a refusal is a map lookup,
		// and a refused honest submit heals by the client's resubmit (B5 NAK).
		if !n.allowEntrySubmit(from) {
			n.logf(ports.LogInfo, "entry submit REFUSED (rate)", "from", from, "budget", entrySubmitBurst)
			n.reply(from, msg, ports.Message{Kind: ports.MsgSubmitEntryAck, OK: false, Data: []byte("rate limited")})
		} else if e, err := entryDecode(msg.Data); err != nil {
			n.logf(ports.LogInfo, "entry submit REFUSED (decode)", "from", from, "bytes", len(msg.Data), "err", err)
			n.reply(from, msg, ports.Message{Kind: ports.MsgSubmitEntryAck, OK: false, Data: []byte(err.Error())})
		} else if verr := n.chain.ValidateEntry(e); verr != nil {
			_, next := n.chain.Head()
			n.logf(ports.LogInfo, "entry submit REFUSED", "from", from, "root", e.Root, "next_height", next, "err", verr)
			n.reply(from, msg, ports.Message{Kind: ports.MsgSubmitEntryAck, OK: false, Data: []byte(verr.Error())})
		} else {
			n.queuePendingEntry(e)
			n.reply(from, msg, ports.Message{Kind: ports.MsgSubmitEntryAck, OK: true})
		}
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
	case ports.MsgSubmitEntry:
		return ports.MsgSubmitEntryAck
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

// pruneOnCommit sheds the heavy bond proofs below this node's retention horizon after a
// commit (slice 5b — the enablement that closes the MATURING OOM). It is a no-op until BFT
// finality gives an immutable anchor (pruneFloor returns 0 otherwise) and with a degenerate
// BondTTL. A behind peer safely catches up AROUND the pruned window via suffix-sync from its
// own finalized head; a deep-cold node is told to use a checkpoint/archive (ErrNeedCheckpoint).
func (n *Node) pruneOnCommit() {
	if n.chain == nil {
		return
	}
	if pruned := n.chain.PruneBelowHorizon(); pruned > 0 {
		n.logf(ports.LogDebug, "pruned heavy bond proofs below the retention horizon", "blocks", pruned)
	}
}

// reconstructFork prepends this node's OWN verified prefix below the served start height
// to a peer-served run of blocks, so the reused genesis-rooted Reconcile sees a full chain
// (slice 5, M1). It keys off what the peer ACTUALLY served (served[0].Height), not what we
// requested: a peer honoring our suffix request serves [Fh, peerHead] (start Fh ⇒ we
// prepend [0, Fh)), a peer that serves the whole chain — an old peer, or an adversary
// serving a genesis-rooted fork — serves start 0 (we use it as-is). Either way our own
// prefix is authoritative (the peer supplies nothing below the served start we anchor on),
// and our own pruned prefix (below our trustFloor) is accepted by the slice-3 Q2 gate
// during replay, while Reconcile's pinned override (tmp.trustFloorOverride = c.trustFloor())
// keeps that floor OUR own, never the fork's. A peer that diverges below the anchor is
// caught by Reconcile (ErrWrongParent / the finality gate).
func (n *Node) reconstructFork(served []chain.Block) ([]chain.Block, error) {
	start := served[0].Height
	if start == 0 {
		return served, nil // genesis-rooted (full chain / no finality / adversary fork) — as-is
	}
	prefix := n.chain.Blocks(0)
	if uint64(len(prefix)) < start {
		return nil, fmt.Errorf("local chain len %d < served start height %d", len(prefix), start)
	}
	full := make([]chain.Block, 0, int(start)+len(served))
	full = append(full, prefix[:start]...)
	full = append(full, served...)
	return full, nil
}

// appendExtension adopts a served window iff it provably EXTENDS this node's
// exact committed head, through the normal Append commit path (#528). The
// proof is the hash chain: the served block at our next height carrying
// Prev == our head hash commits, transitively, to our entire validated
// history — so re-validating that history (the slow path's genesis replay)
// proves nothing new, and only the suffix needs validation. Served blocks at
// or below our head height are skipped unexamined: whatever the peer claims
// there, the Prev linkage pins the adopted blocks to OUR prefix (the C1/
// long-range posture is unchanged — we still anchor on our own chain, never
// a peer's). Each adopted block runs the full Append validation (ancestry,
// quorum/commit re-proof, bond re-verification), so a lying peer wastes only
// the suffix's validation time and cannot feed us an invalid block.
//
// Returns ext=false when the window is not a provable extension (no block at
// our next height, or its Prev differs — a divergent or equal-height fork, a
// peer behind us, or a gap): the caller falls back to the slow reconcile
// path, which keeps the reorg/equivocation/finality-gate guarantees for
// those shapes. err != nil means the append stopped mid-suffix; blocks
// already appended (counted in appended) are fully validated committed
// state and are kept.
//
// MUST only be called when BFT finality is active on the local chain: that
// is what makes "extension" the only adoptable shape (Reconcile's gate
// refuses any fork not containing our committed head) and makes adoption
// without a heavier() comparison sound (every appended block re-proves a
// super-quorum commit, so the extension is strictly heavier by construction).
func (n *Node) appendExtension(window []chain.Block) (appended int, ext bool, err error) {
	head, next := n.chain.Head()
	i := 0
	for i < len(window) && window[i].Height < next {
		i++
	}
	if i == len(window) || window[i].Height != next || window[i].Prev != head {
		return 0, false, nil
	}
	for ; i < len(window); i++ {
		if aerr := n.chain.Append(window[i]); aerr != nil {
			return appended, true, aerr
		}
		appended++
		n.Stats.ChainSyncSuffixAppends++
	}
	return appended, true, nil
}

var ErrNoChain = errors.New("node: validator role not enabled")

// ErrNeedCheckpoint signals that this node is behind by more than the weak-subjectivity
// window (safetyDepth ≈ 2·BondTTL): the peer has pruned the heavy bond proofs across the
// gap, so this node cannot re-verify the intervening history and MUST NOT trust it from a
// peer (the C1/long-range guard — slice 5, PE ruling). Cold sync in a weakly-subjective
// system needs an out-of-band anchor: obtain a recent -ws-checkpoint (socially), or sync
// from an archive node that retains full history. Surfaced (not silently swallowed) so an
// operator sees the remedy rather than an unexplained failure-to-catch-up (S5/I4).
var ErrNeedCheckpoint = errors.New("node: behind the weak-subjectivity window and the peer has pruned the gap — obtain a recent -ws-checkpoint out-of-band or sync from an archive node")

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
	// Mint-flip (era-3 step 2c, extended for era-4 step 4d): once an era is active, a
	// lower-version candidate would be rejected by the boundary rule, so a real publish
	// would falsely fail this local pre-check. Mint the version the real proposer would —
	// v5 with roots at/above H_era4, else v4 with roots at/above H_era3 — so the pre-check
	// tracks production. This candidate carries no folded content, so its post-apply roots
	// are over the single-entry block, exactly what the real single-entry proposal would
	// commit. era-4 is checked first (MintVersion returns v5 only where v4 also holds).
	if mv := n.chain.MintVersion(b.Height); mv >= chain.BlockVersionWitnessable {
		b.LastCommit = n.chain.HeadCarrier() // track production: a real v5 proposal carries the carrier
		if err := n.chain.PopulateEra4Roots(b); err != nil {
			return fmt.Errorf("validate-entry: era-4 root population: %w", err)
		}
	} else if mv >= chain.BlockVersionStateRoot {
		if err := n.chain.PopulateEra3Roots(b); err != nil {
			return fmt.Errorf("validate-entry: era-3 root population: %w", err)
		}
	}
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
	if n.chain.Objective() && n.bond != nil && !n.chain.IsSlashed(n.id) && n.chain.BondRenewalDue(n.id) {
		// The IsSlashed gate (#503 Q1(c) belt): an evicted id's BondRenewalDue is
		// true forever (bonded[id] deleted), and self-embedding a reg the apply
		// path will discard only bloats the block (~1.5 MB) for nothing.
		if reg, ok := n.RegisterBondReg(b.Prev); ok {
			b.BondRegs = append(b.BondRegs, reg)
		}
	}
	// Fold in peer-submitted renewals (H2 / RT-2): an attest-only validator can't
	// propose, so it SUBMITS its fresh BondReg (MsgSubmitBondReg) and whoever
	// proposes next records it — renewing that validator's TTL clock without it
	// ever proposing. Include only regs still valid for THIS head (ValidateBondReg),
	// so one stale or forged submission can't poison the block. FIFO BY ARRIVAL —
	// never sorted by validator ID: with the byte budget admitting ~one plot-sized
	// reg per block, ID order is a strict PRIORITY that starves the highest-ID
	// submitter for as long as lower-ID traffic flows (confirm run 54003f7-91159:
	// the 3rd maturer's first-time reg sat queued for 22 minutes while lower-ID
	// renewals won every slot — the same starvation class the #441 certification
	// closed for entries with FIFO, Addition 2). Arrival order is deterministic
	// for THIS proposer's block, which is all block-byte determinism requires —
	// the single (h, r) designee builds it.
	if n.chain.Objective() && len(n.pendingBondRegs) > 0 {
		fresh := n.pendingBondRegs[:0:0]
		for _, pr := range n.pendingBondRegs {
			// The IsSlashed re-check (#503 Q1(a)) covers the race the arrival
			// gate cannot: a reg queued while its owner's slash was still in
			// flight. Dropping it here is proposer POLICY, not validity — an
			// attester still accepts a block carrying such a reg, so a
			// mixed-version swarm cannot fork on it.
			if vid := pr.R.ValidatorID(); vid != n.id && !n.chain.IsSlashed(vid) && n.chain.ValidateBondReg(pr.R) {
				fresh = append(fresh, pr)
			}
		}
		// #286 Layer 2b: cap the total BYTES of bond registrations embedded per block. A
		// fresh multi-validator genesis where every founding validator submits its ~1.5 MB
		// space-time proof otherwise piles into one ~8 MB block the quorum gather can't move
		// + re-verify over a WAN (the cert stalled at regs=5 / 7.9 MB). The founding set are
		// anchors, so genesis commits SMALL on the anchor bootstrap while the deferred
		// registrations drain over the next blocks. A BYTE budget (not a count) is the right
		// lever: at genesis a full ~1.5 MB proof means ~1 reg/block, but small steady-state
		// renewals pack many per block so an attest-only validator is never starved under a
		// tight TTL (sim/bond_renewal). Embed in ARRIVAL order; account for the proposer's
		// own reg (F6) already appended.
		budget := n.cfg.MaxBondRegBytesPerBlock
		used := int64(0)
		for _, r := range b.BondRegs { // the proposer's own reg (F6), if any, spends budget first
			used += int64(len(bondRegEncode(r)))
		}
		embedded := make(map[ports.NodeID]bool, len(fresh))
		for _, pr := range fresh {
			sz := int64(len(bondRegEncode(pr.R)))
			// Always embed at least one reg (never stall the queue on a single oversized
			// proof); otherwise skip regs that would blow the budget — LATER, SMALLER
			// regs may still fit, and a skipped reg keeps its FIFO seniority for the
			// next block (fix B's durable queue).
			if budget > 0 && len(b.BondRegs) > 0 && used+sz > budget {
				continue
			}
			b.BondRegs = append(b.BondRegs, pr.R)
			embedded[pr.R.ValidatorID()] = true
			used += sz
		}
		// (B) Durable pending queue: KEEP the valid regs that didn't fit this block's
		// byte cap — IN ARRIVAL ORDER, so seniority is preserved and the drain
		// proceeds at the cap rate with no starvation. Drop the embedded (now
		// committed) regs and any no longer valid this sweep (stale beyond the head
		// window or otherwise invalid — the submitter's next sweep resubmits).
		kept := fresh[:0:0]
		for _, pr := range fresh {
			if !embedded[pr.R.ValidatorID()] {
				kept = append(kept, pr)
			}
		}
		n.pendingBondRegs = kept
	}
	// #441: fold mempool ENTRIES into this block too — the certified fix's core.
	// Entries are content the single (h, r) designee carries (FIFO, re-validated,
	// under the SEPARATE entry byte budget), so a publish never has to win a
	// proposal race it structurally lost: whichever proposal commits this height
	// — drain sweep, new-view re-proposal, or a direct ProposeEntry — carries the
	// queued entries with it.
	n.foldPendingEntries(b)
	// Record any equivocations we detected on-chain, so every replica evicts the
	// culprit from the objective set in lockstep (F2). Drop only records the
	// chain already confirms (IsSlashed); everything else RIDES THIS BLOCK AND
	// STAYS QUEUED until a commit confirms it (#397 Q4-ii — the queue was
	// previously zeroed after one attempt, so a slash whose carrier proposal
	// failed to gather quorum was silently dropped and the culprit's on-chain
	// eviction could be lost).
	// R0.6: pack under chain.SlashesBytesCap — the per-block encoded-BYTE ceiling
	// every replica enforces (validateSlashes). Evidence is now always a FULL body
	// pair, so a backlog of fat proofs can exceed one block; a proposal over the cap
	// would be rejected by every replica while the queue requeued it forever (the
	// doomed-proposal loop). So: embed in arrival order, always at least one, skip
	// what would overflow (a later, smaller proof may still fit), and KEEP the rest
	// queued — the drain proceeds at the cap rate with seniority preserved, the same
	// shape as the bond-reg byte budget above.
	if len(n.pendingSlashes) > 0 {
		var still []chain.Equivocation
		for _, e := range n.pendingSlashes {
			if n.chain.IsSlashed(e.CulpritID()) {
				continue
			}
			if chain.SlashesEncodedSize([]chain.Equivocation{e}) > chain.SlashesBytesCap {
				// Can never commit anywhere (see slashEquivocators): DROP, never embed —
				// embedding it would doom this and every later proposal by this node.
				// Release the on-chain latch so a later, smaller proof can be queued.
				delete(n.slashQueued, e.CulpritID())
				n.logf(ports.LogWarn, "dropping queued equivocation proof over SlashesBytesCap", "culprit", e.CulpritID(), "height", e.A.Height)
				continue
			}
			still = append(still, e)
			if len(b.Slashes) > 0 && chain.SlashesEncodedSize(append(b.Slashes[:len(b.Slashes):len(b.Slashes)], e)) > chain.SlashesBytesCap {
				continue // carried to a later block
			}
			b.Slashes = append(b.Slashes, e)
		}
		n.pendingSlashes = still
	}
	// R0.4b: fold this node's staged per-epoch demand-issuer key commitments in.
	// v5-ONLY — the era-3 leaf set does not commit this keyspace, so a v4 block
	// carrying one is REJECTED (chain.validateIssuerKeys); staging them below the
	// era-4 boundary would make our own proposal invalid. Registrations RIDE AND STAY
	// QUEUED until the chain confirms the commitment, the same #397-Q4-ii discipline
	// pendingSlashes uses: a commitment whose carrier proposal failed to gather quorum
	// must not be silently lost, or the issuer's keys stay unresolvable forever.
	if len(n.pendingIssuerKeys) > 0 && n.chain.MintVersion(b.Height) >= chain.BlockVersionWitnessable {
		var still []chain.IssuerKeyReg
		blockEpoch := n.chain.BlockEpoch(b.Height)
		for _, r := range n.pendingIssuerKeys {
			if _, committed := n.chain.IssuerKeyCommitment(r.IssuerID(), r.Epoch); committed {
				continue // already bound; append-only, never re-submit
			}
			// STALE REGISTRATION → DROP IT (red-team break 2, 2026-09-02).
			// validateIssuerKeys REJECTS a backdated registration — that rule is
			// correct, since committing key_E after epoch E has run is the
			// equivocation move. But a staged reg that missed its own epoch (the
			// node was not the designee inside its boot epoch, or it booted before
			// the era-4 flip, or it restarted late in an epoch) used to RIDE AND STAY
			// QUEUED, so every later proposal failed this node's OWN local pre-check
			// with ErrIssuerKeyEpoch — a permanent, restart-only proposer mute. Drop
			// it here instead and let the rotation schedule re-stage a registration
			// for an epoch that is still registrable. Proposer POLICY, never
			// validity: an attester's acceptance rule is untouched, so a mixed swarm
			// cannot fork on it (the same shape as the IsSlashed filter on pending
			// bond regs).
			if r.Epoch < blockEpoch {
				continue
			}
			// C2, the proposer self-wedge: validateIssuerKeys reads the bond ledger
			// PRE-apply, so folding a registration into the very block that first
			// records its issuer's bond fails our own local pre-check below — and the
			// registration stays queued, so every later proposal fails the same way.
			// DEFER instead: keep it staged and fold it once the bond is committed.
			// Proposer POLICY only (IssuerKeyRegAdmissible mirrors the validity clause
			// but decides nothing) — an attester still accepts a block carrying a reg
			// we deferred, so a mixed swarm cannot fork on it.
			if !n.chain.IssuerKeyRegAdmissible(r.IssuerID()) {
				still = append(still, r)
				continue
			}
			b.IssuerKeys = append(b.IssuerKeys, r)
			still = append(still, r)
		}
		n.pendingIssuerKeys = still
	}
	// Mint-flip (era-3 build step 2c, extended for era-4 step 4d): at/above the era-4
	// activation boundary the chain mints v5 with populated committed roots (the era-3
	// leaves PLUS the maintenance-spine keyspaces); at/above the era-3 boundary, v4; below
	// both, v2 (the #432 two-phase-certificate era). The chain owns the activation state,
	// so ask it — MintVersion(height) is a pure function of committed history, identical on
	// every honest proposer at this head (I5). PopulateEra{3,4}Roots runs AFTER all
	// apply-affecting content (BondRegs, entries, slashes) is folded in above, so the roots
	// cover the block as it will actually commit (post-apply state). era-4 is checked first
	// (MintVersion returns v5 only where v4 also holds, H_era4 >= H_era3). Below both
	// boundaries the path is byte-for-byte the old v2 behavior.
	if mv := n.chain.MintVersion(b.Height); mv >= chain.BlockVersionWitnessable {
		// era-4 (v5) ATTESTATION CARRIER (R-BOX-ATTESTS, owner call O1, ratified 2026-09-03).
		// Attach the PARENT's precommits BEFORE populating the roots: the carrier is folded into
		// Hash() and it is the v5 validatorsSeen transition input, so the roots must cover it and
		// the Sign below must sign it. That IS the fix — the proposer holds these bytes before it
		// gathers, so the root it signs is the root the block commits, and a certificate that
		// would seat a new attester no longer makes its own block invalid.
		//
		// HONEST-MAXIMAL, and the discretion is UNENFORCEABLE: "carry everything you hold" cannot
		// be a validity rule, because no replica can know what this proposer held. The discretion
		// is DOWNWARD-ONLY — signatures are genuine and unforgeable, so a proposer can DELAY a
		// seating by omitting a signer but can never FORGE one, and an under-carrying proposer
		// harms only its own fork. See Chain.HeadCarrier.
		b.LastCommit = n.chain.HeadCarrier()
		if err := n.chain.PopulateEra4Roots(b); err != nil {
			done(fmt.Errorf("propose: era-4 root population: %w", err))
			return
		}
	} else if mv >= chain.BlockVersionStateRoot {
		if err := n.chain.PopulateEra3Roots(b); err != nil {
			done(fmt.Errorf("propose: era-3 root population: %w", err))
			return
		}
	} else {
		b.Version = chain.BlockVersionRounds // era 2: minted blocks carry two-phase certificates (#432)
	}
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
		return n.chain.SupportMeetsQuorum(b.ProposerID(), ids, b.Height)
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
	// #456 (research-certified): the collection is CONCURRENT — the prepare-QC
	// is broadcast to every attester at once and replies are collected until
	// the quorum predicate holds. Gather latency = the slowest NEEDED (live)
	// replier; an unreachable member's transport timeout fires off the
	// critical path (the sequential ask-chain paid every dead peer's full
	// retry budget before asking the next — ~34s each in the field, the 10a
	// stall's mechanism). Safe because no certified property is
	// order-sensitive: there is ONE gatherer per proposal, the QC is carried
	// in the block (never re-derived), and the stop predicate below is the
	// SAME arithmetic ValidateCommit demands.
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
		finishedPC := false
		outstanding := 0
		sending := true
		finishPC := func() {
			if finishedPC {
				return
			}
			if counted(pcs) >= quorum && supportMet(pcs) {
				finishedPC = true
				b.CommitRound = round
				b.PrepareQC = qc
				// COPY at capture: replies landing after completion must never
				// mutate a committed certificate's backing array.
				b.Atts = append([]chain.Attestation(nil), pcs...)
				if err := n.chain.Append(*b); err != nil {
					done(fmt.Errorf("propose: commit rejected by own replica: %w", err))
					return
				}
				n.Stats.BlocksCommitted++
				n.logf(ports.LogInfo, "block committed", "height", b.Height, "round", round, "entries", len(b.Entries), "regs", len(b.BondRegs), "attestations", len(b.Atts), "via", "proposal")
				if n.onCommit != nil {
					n.onCommit(*b)
				}
				n.pruneOnCommit() // shed heavy proofs below the horizon (slice 5b)
				n.broadcastCommit(b, broadcast, 0, func() { done(nil) })
				return
			}
			if outstanding == 0 && !sending {
				finishedPC = true
				n.logf(ports.LogDebug, "gather: NO PRECOMMIT QUORUM", "height", b.Height, "round", round, "gathered", counted(pcs), "needed", quorum)
				done(fmt.Errorf("propose height %d round %d: %w: %d precommits of %d gathered",
					b.Height, round, chain.ErrNoQuorum, counted(pcs), quorum))
				return
			}
		}
		for _, v := range attesters {
			if v == n.id {
				continue
			}
			v := v
			outstanding++
			n.request(v, ports.Message{Kind: ports.MsgPrepareQC, Data: qcRaw},
				func(resp ports.Message, err error) {
					outstanding--
					if finishedPC {
						return
					}
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
					finishPC()
				})
		}
		sending = false
		finishPC()
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
	finishedPrep := false
	outstandingPrep := 0
	sendingPrep := true
	finishPrep := func() {
		if finishedPrep {
			return
		}
		if counted(atts) >= quorum && supportMet(atts) {
			finishedPrep = true
			// COPY at capture (#456): late replies must never mutate the QC.
			qc := append([]chain.Attestation(nil), atts...)
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
		if outstandingPrep == 0 && !sendingPrep {
			finishedPrep = true
			n.logf(ports.LogDebug, "gather: NO PREPARE QUORUM", "height", b.Height, "round", round, "bytes", len(raw), "gathered", counted(atts), "needed", quorum)
			done(fmt.Errorf("propose height %d round %d: %w: %d prepares of %d gathered",
				b.Height, round, chain.ErrNoQuorum, counted(atts), quorum))
			return
		}
	}
	for _, v := range attesters {
		if v == n.id {
			continue
		}
		v := v
		outstandingPrep++
		n.logf(ports.LogDebug, "gather: requesting prepare", "to", v, "height", b.Height, "round", round, "bytes", len(raw), "have", counted(atts), "need", quorum)
		n.request(v, ports.Message{Kind: ports.MsgProposeBlock, Data: envRaw},
			func(resp ports.Message, err error) {
				outstandingPrep--
				if finishedPrep {
					return
				}
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
				finishPrep()
			})
	}
	sendingPrep = false
	finishPrep()
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
		if n.chain.AttesterEligibleAt(p, rs.Height) {
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
		// Queue the proof for on-chain recording so the OBJECTIVE set evicts the
		// culprit in lockstep on every replica (F2), not just this local ledger.
		// The on-chain latch (slashQueued) is SEPARATE from the local one below
		// (Researcher V-1): a culprit whose first proof could not be queued must still
		// get a later one queued.
		//
		// R0.6: a proof that ALONE exceeds chain.SlashesBytesCap can never commit on any
		// replica, so it is never queued — queuing it would make every proposal by this
		// node invalid for good (the local pre-check fails forever; the only removal is
		// IsSlashed, which never flips). The local ledger penalty below still applies.
		// Reachable by one Byzantine validator that signs a fat garbage block at a height
		// it also honestly attested and serves it as a peer (PE ruling F-1), so it is
		// logged by name, once per culprit; the culprit keeps its on-chain seat
		// (R-BIG-EVIDENCE-UNSLASHABLE — the cap's second face, see SlashesBytesCap).
		if n.chain != nil && !n.chain.IsSlashed(cid) && !n.slashQueued[cid] {
			if sz := chain.SlashesEncodedSize([]chain.Equivocation{e}); sz > chain.SlashesBytesCap {
				if !n.slashOverCapLogged[cid] {
					n.slashOverCapLogged[cid] = true
					n.logf(ports.LogWarn, "equivocation proof exceeds SlashesBytesCap; not queued for on-chain slashing", "culprit", cid, "height", e.A.Height, "bytes", sz, "cap", chain.SlashesBytesCap)
				}
			} else {
				n.pendingSlashes = append(n.pendingSlashes, e)
				n.slashQueued[cid] = true
			}
		}
		if n.slashedLocal[cid] {
			continue
		}
		n.slashedLocal[cid] = true
		n.ledger.SlashEquivocation(cid) // legacy/rep path
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
	// Per-sweep diagnostic (#572): every failure branch of this walk logs at
	// debug or not at all, which left the 027c354-deep val-c catch-up stall
	// (100+ min of no-progress sweeps, info-level field capture) structurally
	// unattributable. When a sweep ends with ZERO adopted blocks while a probe
	// showed a peer ahead (or every probe failed), ONE warn line names what
	// each branch did — so the next field occurrence carries its mechanism.
	diag := struct {
		probeFails, headMatches, windows, suffixAppends, reconciles int
		maxPeerHead                                                 uint64
		peerAhead                                                   bool
		lastErr                                                     string
	}{}
	var ask func(i int)
	// fetchFull runs the full-chain fetch + slash + Reconcile against peer p, then
	// advances to the next peer. Used whenever the head probe shows a difference or
	// cannot be answered. WINDOWED (#466): the peer serves byte-bounded windows
	// (EncodeBlocksUpTo), and this loop re-requests from each window's last decoded
	// height until it holds the peer's whole suffix — each raw reply buffer is
	// decoded and released per iteration, so the only accumulation is the decoded
	// []Block itself (the requester-retention axis, #299's, unchanged). peerHead
	// (when known, from the head probe) ends the loop without a final empty-window
	// round-trip; against a pre-window server that returns the whole suffix in one
	// over-full reply the same loop accepts it and terminates on the next request
	// (accept-and-cap — the transport's maxFrame bounds any single reply).
	fetchFull := func(p ports.NodeID, peerHead uint64, peerHeadKnown bool, next func()) {
		n.Stats.ChainSyncFullFetches++
		// Suffix-sync (slice 5, M1): request from our OWN finalized head, not genesis, so a
		// peer that has pruned its old heavy bond proofs still serves us un-pruned blocks
		// across the gap. reqHeight is 0 without BFT finality (no immutable anchor) ⇒ a
		// full-genesis fetch, unchanged. The peer serves [reqHeight, peerHead]; we anchor it
		// on our OWN verified prefix (never a peer-served head — the C1/long-range guard).
		reqHeight, finActive := n.chain.FinalizedHeight()
		var served []chain.Block
		finish := func() {
			if len(served) == 0 {
				next()
				return
			}
			full, ferr := n.reconstructFork(served)
			if ferr != nil {
				diag.lastErr = fmt.Sprintf("reconstruct from %x: %v", p[:4], ferr)
				n.logf(ports.LogDebug, "could not reconstruct fork from peer suffix", "peer", p, "err", ferr)
				next()
				return
			}
			before := n.chain.Len()
			old := n.chain.Blocks(0) // snapshot to catch cross-fork double-signs
			// Slash on DETECTION, not on adoption (seam-7). Scan the reconstructed
			// fork against our LOCAL one for cross-fork double-signs BEFORE the
			// heavier test and regardless of whether we adopt: a validator that
			// signed a block at a height we hold AND a conflicting block at that
			// height is provably guilty even if its fork is LIGHTER and never
			// reconciled onto. The evidence is self-verifying
			// (chain.VerifyEquivocation), so an honest sequential signer is never
			// caught. Detection now covers heights ≥ reqHeight (our finalized head)
			// — where forks can exist; below it, finality forbids a fork, so a
			// sub-finalized double-sign is either already-slashed at commit or a
			// >f attack out of the safety model.
			// R0.6 (PE ruling F-3): candidate selection body-hashes both sides, and
			// reconstructFork is genesis-rooted, so scanning `full` would re-hash our
			// own verified prefix twice per sweep (the #555 class, measured 43 µs →
			// 228 ms at n=600). The prefix below the served start is OUR OWN blocks on
			// both sides (identical, never a candidate), so scan only the heights the
			// peer actually served: a cross-fork double-sign can exist only there.
			cmp := old
			if start := served[0].Height; start > 0 && uint64(len(old)) >= start {
				cmp = old[start:]
			}
			n.slashEquivocators(cmp, served)
			n.Stats.ChainSyncFullReconciles++
			diag.reconciles++
			if ok, rerr := n.chain.Reconcile(full); ok {
				now := n.chain.Blocks(0)
				if d := n.chain.Len() - before; d > 0 {
					added += d
				}
				if dropped := reorgDropped(old, now); dropped > 0 && n.onReorg != nil {
					n.onReorg(dropped, uint64(len(now)-1))
				}
				n.logf(ports.LogInfo, "chain reconciled from peer", "peer", p, "len", n.chain.Len())
			} else if errors.Is(rerr, chain.ErrPrunedAboveHorizon) {
				// Deep-cold (behind > safetyDepth): the peer pruned the gap and we
				// cannot re-verify it — refuse to trust it from a peer (C1 guard).
				// SIGNAL the remedy, never silently fail to adopt (I4/S5).
				n.Stats.ChainSyncNeedCheckpoint++
				n.logf(ports.LogWarn, "cannot catch up from peer: behind the weak-subjectivity window and the peer pruned the gap — obtain a recent -ws-checkpoint out-of-band or sync from an archive node", "peer", p, "err", ErrNeedCheckpoint)
			} else if rerr != nil {
				diag.lastErr = fmt.Sprintf("not adopted from %x: %v", p[:4], rerr)
				n.logf(ports.LogDebug, "peer chain not adopted", "peer", p, "err", rerr)
			}
			next()
		}
		var fetchWindow func(h uint64)
		fetchWindow = func(h uint64) {
			n.request(p, ports.Message{Kind: ports.MsgGetChain, Height: h},
				func(resp ports.Message, err error) {
					if err != nil || !resp.OK {
						// Mid-loop failure: the partial suffix is DISCARDED, never
						// reconciled — sync fails closed for this peer this sweep and
						// the next sweep retries (#466).
						diag.lastErr = fmt.Sprintf("window@%d from %x: %v (ok=%v)", h, p[:4], err, resp.OK)
						if len(served) > 0 {
							n.logf(ports.LogDebug, "windowed chain fetch aborted mid-loop", "peer", p, "at-height", h, "err", err)
						}
						next()
						return
					}
					window, derr := chain.DecodeBlocks(resp.Data)
					if derr != nil {
						diag.lastErr = fmt.Sprintf("undecodable window@%d from %x: %v", h, p[:4], derr)
						n.logf(ports.LogDebug, "windowed chain fetch: undecodable reply", "peer", p, "err", derr)
						next()
						return
					}
					if len(window) == 0 {
						finish() // nothing above h: the suffix is complete
						return
					}
					n.Stats.ChainSyncWindows++
					diag.windows++
					last := window[len(window)-1].Height
					// #528 fast path — a window that provably EXTENDS our exact committed
					// head is adopted through the normal Append commit path, validating
					// ONLY the new blocks. The slow tail below re-validates the WHOLE
					// reconstructed fork from genesis (~1s per 1.5 MB reg block, ON the
					// event loop), which is O(height) per catch-up: at h≈56 under MATURING
					// reg weight one reconcile outlasted the round durations, starved the
					// sweeps, and wedged the chain (the field-measured knee). Gated on
					// finality being ACTIVE: Reconcile's finality gate then refuses any
					// fork that does not contain our committed head, so an extension is
					// the only adoptable shape — and each appended block re-proves a
					// super-quorum commit inside Append (strictly positive weight), so
					// adoption here is exactly the outcome heavier() would force. Gated on
					// len(served)==0 so a window run is judged in ONE mode, never spliced
					// across fast and slow handling; loop occupancy per callback is
					// bounded by the window byte budget, and control returns to the loop
					// between windows.
					if finActive && len(served) == 0 {
						if k, ext, aerr := n.appendExtension(window); ext {
							added += k
							diag.suffixAppends += k
							if k > 0 {
								n.logf(ports.LogInfo, "chain caught up from peer (suffix append)", "peer", p, "appended", k, "len", n.chain.Len())
							}
							if aerr != nil {
								diag.lastErr = fmt.Sprintf("suffix stop@%d from %x: %v", h, p[:4], aerr)
								// Mid-suffix stop: blocks appended so far are fully
								// validated committed blocks (the same state as having
								// synced one sweep earlier) — keep them; drop the peer
								// for this sweep. A pruned gap keeps the slice-5 deep-cold
								// signal: refuse-to-trust + name the remedy (I4/S5).
								if errors.Is(aerr, chain.ErrPrunedAboveHorizon) {
									n.Stats.ChainSyncNeedCheckpoint++
									n.logf(ports.LogWarn, "cannot catch up from peer: behind the weak-subjectivity window and the peer pruned the gap — obtain a recent -ws-checkpoint out-of-band or sync from an archive node", "peer", p, "err", ErrNeedCheckpoint)
								} else {
									n.logf(ports.LogDebug, "suffix append stopped mid-window", "peer", p, "err", aerr)
								}
								next()
								return
							}
							if peerHeadKnown && last >= peerHead {
								next() // caught up to the probed head; nothing to reconcile
								return
							}
							fetchWindow(last + 1)
							return
						}
						// Not a provable extension (divergent fork, equal-height fork,
						// peer behind, or a gap) — fall through to the slow path, which
						// keeps every reorg / equivocation-scan / finality-gate guarantee.
					}
					served = append(served, window...)
					if last < h {
						// A server whose windows do not advance is broken or lying;
						// stop rather than loop — Reconcile fails closed on whatever
						// this produced.
						finish()
						return
					}
					if peerHeadKnown && last >= peerHead {
						finish()
						return
					}
					fetchWindow(last + 1)
				})
		}
		fetchWindow(reqHeight)
	}
	ask = func(i int) {
		if i >= len(peers) {
			// #572: a no-progress sweep while demonstrably behind (a probe showed
			// a peer ahead), or blind (every probe failed), is the exact state the
			// 027c354-deep val-c stall sat in silently for 100+ minutes. Name the
			// branch at WARN so the field capture carries the mechanism.
			if added == 0 && (diag.peerAhead || (diag.probeFails > 0 && diag.headMatches == 0)) {
				_, ourNext := n.chain.Head()
				n.logf(ports.LogWarn, "chain sync sweep made NO progress while behind (#572)",
					"our-next", ourNext, "max-peer-head", diag.maxPeerHead,
					"peers", len(peers), "probe-fails", diag.probeFails,
					"head-matches", diag.headMatches, "windows", diag.windows,
					"suffix-appends", diag.suffixAppends, "reconciles", diag.reconciles,
					"last-err", diag.lastErr)
			}
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
				ourHead, ourNext := n.chain.Head()
				peerHeadHeight, peerHeadKnown := uint64(0), false
				if err == nil && resp.Kind == ports.MsgChainHeadReply && resp.OK {
					// The probe's height also serves the windowed fetch (#466): it
					// ends the window loop without a final empty-window round-trip.
					peerHeadHeight, peerHeadKnown = resp.Height, true
					if peerHeadHeight > diag.maxPeerHead {
						diag.maxPeerHead = peerHeadHeight
					}
					if peerHeadHeight >= ourNext {
						diag.peerAhead = true
					}
					if len(resp.Data) == len(ourHead) {
						var peerHead ports.Hash
						copy(peerHead[:], resp.Data)
						if peerHead == ourHead {
							// Identical head ⇒ identical committed history: nothing to do.
							n.Stats.ChainSyncHeadMatches++
							diag.headMatches++
							ask(i + 1)
							return
						}
					}
				} else {
					diag.probeFails++
					if err != nil {
						diag.lastErr = fmt.Sprintf("head probe %x: %v", p[:4], err)
					}
				}
				// Head differs, or the peer is too old to answer the probe (or it
				// timed out) — fall back to the full fetch, which preserves every
				// catch-up / reorg / equivocation-detection guarantee.
				fetchFull(p, peerHeadHeight, peerHeadKnown, func() { ask(i + 1) })
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
		})
		// Renew objective standing without proposing (H2 / RT-2): submit a fresh
		// bond proof to the same validator set we reconcile against, so an
		// attest-only validator sustains its TTL-bound standing. No-op off the
		// objective path or with no bond.
		n.SubmitBondRenewal(peers)
	}
	// #432: count non-progress sweeps and fire a round-change when the working
	// height is stuck with pending work (the deterministic, quorum-observable
	// view-change trigger). ON THE TICK, not in the SyncChain callback (#561):
	// the escape needs only local state (pending work + the sweep count), and
	// the callback fires only after the sequential peer walk completes — dead
	// peers stretch that walk by their full retry budgets, which in the field
	// held the first round-change to ~8min against a 430s bound (a434494-deep
	// 10a). Tick cadence is also the #549 Q3 premise: the cross-node skew bound
	// (< ChainSyncInterval) assumes the escape runs once per interval. The
	// DRAIN stays in the callback — proposing wants the freshest head (#338),
	// and once the escape fires, the new-view proposal path is message-driven
	// at the designee (recordRoundChange), independent of any walk.
	n.maybeAdvanceRound()
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
	// The IsSlashed gate (#503 Q1(c) belt): an evicted node's BondRenewalDue
	// reads true forever, and without the gate own-due kept arming drain sweeps
	// for a registration the chain will never honor. (In practice an evicted
	// node also fails ProposerEligible below — this keeps the quiescence rule
	// (B6) honest on its own terms.)
	ownDue := n.bond != nil && !n.chain.IsSlashed(n.id) && n.chain.BondRenewalDue(n.id)
	// #441: pending mempool ENTRIES are designee work exactly like pending regs —
	// the designee's block carries them (foldPendingEntries), so an entry-only
	// queue must fire this sweep too, or a publish on an idle chain would wait
	// for unrelated renewal traffic to move it. B6 quiescence holds when truly
	// idle: no regs, no entries, no own renewal due.
	if len(n.pendingBondRegs) == 0 && len(n.pendingEntries) == 0 && !ownDue {
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
		if len(n.pendingBondRegs) > 0 || len(n.pendingEntries) > 0 {
			n.logf(ports.LogInfo, "bond-reg drain blocked at own sign slot — awaiting round advance (#432)",
				"height", height, "round", rs.Round, "mark_height", n.signMark.Height, "mark_round", n.signMark.Round, "pending", len(n.pendingBondRegs), "pending_entries", len(n.pendingEntries))
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
		if len(n.pendingBondRegs) == 0 && len(n.pendingEntries) == 0 {
			// Own renewal only, and we are not the designated proposer: submit,
			// never propose (Q2b-2). The reg reaches the chain via the
			// designated proposer's queue; nothing to take over for. Pending
			// ENTRIES count as takeover-worthy work (#441): a silent designee
			// must not strand the mempool.
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
		if n.chain.AttesterEligibleAt(p, height) {
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
	// Deterministic order (B2): this list drives gather ask-order, round-change
	// broadcast order, and — in the model-check — the entire schedule. Returning
	// raw map-iteration order made every gather's QC composition a per-call
	// dice-roll (caught by the #451 fixture flaking 2-in-10 under -count=10:
	// the sybil author's prepare-QC landed on order-lucky subsets). ID-sort is
	// safe here: ask ORDER is not an inclusion-fairness surface (any assembled
	// quorum is valid) — unlike the reg/entry queues, which are FIFO by #448/
	// #441 Addition 2. EligibleProposers sorts for the same reason.
	sort.Slice(out, func(i, j int) bool { return bytes.Compare(out[i][:], out[j][:]) < 0 })
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
