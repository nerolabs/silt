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
		// Attest only what we would accept: same rules, our reputation view.
		// #286 Layer-2 root-cause aid: the gather path otherwise logs ONLY on success
		// (the "block committed" line below), so a stalled quorum-2 genesis gather is
		// invisible — a silent ValidateProposal reject of the ~1.5 MB first block over the
		// WAN looks identical to "never received". These debug lines make an attester's
		// receive → validate → reply visible under -log debug so the failing round can be
		// traced. Debug-level only (no info-level noise on the hot path).
		b, err := chain.Decode(msg.Data)
		if err != nil {
			n.logf(ports.LogDebug, "gather/attest: DECODE failed", "from", from, "bytes", len(msg.Data), "err", err)
			n.reply(from, msg, ports.Message{Kind: ports.MsgAttestReply, OK: false})
			return true
		}
		if verr := n.chain.ValidateProposal(b); verr != nil {
			n.logf(ports.LogDebug, "gather/attest: REJECTED (ValidateProposal)", "from", from, "height", b.Height, "bytes", len(msg.Data), "regs", len(b.BondRegs), "reason", verr)
			n.reply(from, msg, ports.Message{Kind: ports.MsgAttestReply, OK: false})
			return true
		}
		// Never equivocate: refuse to sign a DIFFERENT block at a height we
		// already attested, even if two competing proposals arrive before
		// either commits. An honest validator's signature at a height is
		// final; this is what makes a double-sign proof (chain.Equivocation)
		// evidence of malice, not an accident.
		if prev, ok := n.attested[b.Height]; ok && prev != b.Hash() {
			n.logf(ports.LogDebug, "gather/attest: REFUSED (already attested a different block at height)", "from", from, "height", b.Height)
			n.reply(from, msg, ports.Message{Kind: ports.MsgAttestReply, OK: false})
			return true
		}
		att := chain.Attest(b, n.signer)
		n.attested[b.Height] = b.Hash()
		raw, _ := attEncode(att)
		n.logf(ports.LogDebug, "gather/attest: ATTESTED", "from", from, "height", b.Height, "bytes", len(msg.Data), "regs", len(b.BondRegs))
		n.reply(from, msg, ports.Message{Kind: ports.MsgAttestReply, OK: true, Data: raw})
	case ports.MsgCommitBlock:
		b, err := chain.Decode(msg.Data)
		ok := err == nil && n.chain.Append(*b) == nil
		if ok {
			n.Stats.BlocksCommitted++
			n.logf(ports.LogInfo, "block committed", "height", b.Height, "entries", len(b.Entries), "attestations", len(b.Atts), "via", "broadcast")
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
			if reg, err := bondRegDecode(msg.Data); err == nil && n.chain.ValidateBondReg(reg) {
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
		n.pendingBondRegs = make(map[ports.NodeID]chain.BondReg)
	}
	// Record any equivocations we detected on-chain, so every replica evicts the
	// culprit from the objective set in lockstep (F2). Drop any already recorded.
	if len(n.pendingSlashes) > 0 {
		var still []chain.Equivocation
		for _, e := range n.pendingSlashes {
			if n.chain.IsSlashed(e.CulpritID()) {
				continue
			}
			b.Slashes = append(b.Slashes, e)
		}
		n.pendingSlashes = still
	}
	chain.Sign(b, n.signer)
	if err := n.chain.ValidateProposal(b); err != nil {
		done(fmt.Errorf("propose: local pre-check: %w", err))
		return
	}
	raw := chain.Encode(b)
	// #286 Layer-2 root-cause aid: at -log info the gather logs ONLY on the final commit
	// ("block committed" below), so a quorum-2 genesis gather that starts and never
	// completes over the WAN is invisible. These debug lines trace the round — the block
	// SIZE (the ~1.5 MB first block is the suspect), each attester request, and each
	// reply/failure — so the next multi-region run with -log debug shows exactly where it
	// stalls (proposer doesn't send? attester rejects the big block? attester never
	// replies?). Debug-level only.
	n.logf(ports.LogDebug, "gather: starting", "height", b.Height, "bytes", len(raw), "regs", len(b.BondRegs), "quorum", quorum, "attesters", len(attesters))

	var atts []chain.Attestation
	// supportMet: would the coalition gathered so far commit? The count floor is
	// the caller's `quorum`; in a MATURE EPOCH the chain additionally demands the
	// >⅔ frozen-WEIGHT super-majority (B2) — "how many" stops being sufficient,
	// so the chain answers "is this support enough" directly.
	supportMet := func() bool {
		ids := make([]ports.NodeID, 0, len(atts))
		for _, a := range atts {
			ids = append(ids, a.AttesterID())
		}
		return n.chain.SupportMeetsQuorum(n.id, ids)
	}
	var ask func(i int)
	ask = func(i int) {
		if len(atts) >= quorum && supportMet() {
			b.Atts = atts
			if err := n.chain.Append(*b); err != nil {
				done(fmt.Errorf("propose: commit rejected by own replica: %w", err))
				return
			}
			n.Stats.BlocksCommitted++
			n.logf(ports.LogInfo, "block committed", "height", b.Height, "entries", len(b.Entries), "attestations", len(atts), "via", "proposal")
			if n.onCommit != nil {
				n.onCommit(*b)
			}
			n.broadcastCommit(b, broadcast, 0, func() { done(nil) })
			return
		}
		if i >= len(attesters) {
			n.logf(ports.LogDebug, "gather: NO QUORUM", "height", b.Height, "bytes", len(raw), "gathered", len(atts), "needed", quorum)
			done(fmt.Errorf("propose height %d: %w: %d of %d gathered",
				b.Height, chain.ErrNoQuorum, len(atts), quorum))
			return
		}
		v := attesters[i]
		if v == n.id {
			ask(i + 1)
			return
		}
		n.logf(ports.LogDebug, "gather: requesting attestation", "to", v, "height", b.Height, "bytes", len(raw), "have", len(atts), "need", quorum)
		n.request(v, ports.Message{Kind: ports.MsgProposeBlock, Data: raw},
			func(resp ports.Message, err error) {
				switch {
				case err != nil:
					n.logf(ports.LogDebug, "gather: attester request FAILED", "to", v, "height", b.Height, "err", err)
				case !resp.OK:
					n.logf(ports.LogDebug, "gather: attester REFUSED", "to", v, "height", b.Height)
				default:
					if att, aerr := attDecode(resp.Data); aerr == nil {
						atts = append(atts, att)
						n.logf(ports.LogDebug, "gather: attestation collected", "from", v, "height", b.Height, "have", len(atts), "need", quorum)
					} else {
						n.logf(ports.LogDebug, "gather: attestation DECODE failed", "from", v, "height", b.Height, "err", aerr)
					}
				}
				ask(i + 1)
			})
	}
	ask(0)
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
		n.ledger.SlashEquivocation(e.CulpritID()) // legacy/rep path
		// Queue the proof for on-chain recording so the OBJECTIVE set evicts the
		// culprit in lockstep on every replica (F2), not just this local ledger.
		if n.chain != nil && !n.chain.IsSlashed(e.CulpritID()) {
			n.pendingSlashes = append(n.pendingSlashes, e)
		}
		n.logf(ports.LogWarn, "validator slashed for equivocation", "culprit", e.CulpritID(), "height", e.A.Height)
		if n.onSlash != nil {
			n.onSlash(e.CulpritID(), e.A.Height)
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
	// NEVER sign twice at one height (Tendermint locking): if we already
	// attested a block at this height, a drain proposal here would be a second
	// signature on a different block — which a peer's cross-fork scan reads as
	// equivocation. The failing-first repro showed exactly that: two anchors
	// drain-raced a height, cross-attested, and SLASHED EACH OTHER into a
	// wedged chain. The height clears when a commit moves the head.
	if _, signed := n.attested[height]; signed {
		return
	}
	// One designated drain proposer per height, derived from COMMITTED state so
	// every honest replica agrees (chain.EligibleProposers is sorted): this
	// makes an honest drain race structurally impossible, rather than merely
	// unlikely. Liveness fallback: if the designated proposer is absent (the
	// height hasn't moved after a few sweeps with work still pending), any
	// eligible proposer may pick it up — the race window returns only in that
	// degraded case, and the never-sign-twice guard above still bounds it.
	if props := n.chain.EligibleProposers(); len(props) > 0 {
		designated := props[int(height)%len(props)]
		if designated != n.id {
			if n.drainWaitSweeps < 3 {
				n.drainWaitSweeps++
				return
			}
			// designated proposer hasn't drained in 3 sweeps — take over
		}
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
