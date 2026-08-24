// Bond audit (T1b, #78): validators challenge each other's identity-bound
// storage bonds over the network, so consensus standing is continuously
// backed by real held storage rather than self-reported serving. This is the
// live half of the mechanism whose primitive (core/bond) and ledger
// (credit.RecordBondChallenge / DecayStale) landed in T1a. Design:
// docs/design/bond-audit.md.
package node

import (
	"crypto/ed25519"
	"crypto/sha256"

	"github.com/nerolabs/silt/core/bond"
	"github.com/nerolabs/silt/core/vdf"
	"github.com/nerolabs/silt/ports"
)

// plotPubKey is the validator ed25519 public key the plot seed H(pk, n) binds to
// (M0 Sybil G2). It is PUBLIC by design: the verifier recomputes labels from it,
// so the plot can no longer be sealed from a private secret. Identity binding is
// now a CHECKED property of the plot (a plot for pk_A fails recomputation under
// pk_B), not the un-grief-ability of a private seed — see docs/design/m0-sybil-rebind.md.
func plotPubKey(signer ed25519.PrivateKey) []byte {
	return append([]byte(nil), signer.Public().(ed25519.PublicKey)...)
}

// bondInfo is a peer's advertised bond, learned from gossip (BondRoot /
// BondSize on every message the peer sends).
type bondInfo struct {
	root ports.Hash
	size int64
}

// latWindow is a small ring of a peer's recent bond-challenge reply latencies.
// Its MINIMUM is the low-quantile floor estimate the C1 timing signal reads
// (build-immutable #3): network delay is one-sided (jitter and retransmits can
// only ADD latency), so the windowed minimum filters them out and estimates the
// peer's true transport+compute floor. An honest node on a bad path is RANDOMLY
// slow, so its minimum collapses to fast; a partial-storage prover that must
// recompute on EVERY challenge is CONSISTENTLY slow, so its minimum stays
// elevated. (Because the minimum ignores the high samples, a retried/jittered
// reply never poisons the estimate — the min subsumes Karn's-algorithm filtering
// for this signal.) full() gates on a warm window so one or two early samples
// can't raise a false suspicion.
const latWindowSize = 8

type latWindow struct {
	samples [latWindowSize]ports.Duration
	n       int // total observed (caps display; ring index is n%size)
}

func (w *latWindow) observe(d ports.Duration) {
	w.samples[w.n%latWindowSize] = d
	w.n++
}

func (w *latWindow) full() bool { return w.n >= latWindowSize }

// BondLatencyFloor reports a peer's windowed-minimum bond-challenge reply latency
// (the low-quantile floor) and whether that floor is SUSTAINED above the deadline
// — the soft C1 partial-storage suspicion (build-immutable #3). ok is false until
// the window is warm. It is read-only and never gates standing; it exists for
// disclosure/observability and tests.
func (n *Node) BondLatencyFloor(id ports.NodeID) (floor ports.Duration, sustainedSlow, ok bool) {
	w := n.peerBondRTT[id]
	if w == nil || !w.full() {
		return 0, false, false
	}
	m := w.min()
	return m, n.cfg.BondMaxAnswerLatency > 0 && m > n.cfg.BondMaxAnswerLatency, true
}

// min returns the smallest latency in the window (the floor estimate).
func (w *latWindow) min() ports.Duration {
	m := w.samples[0]
	lim := latWindowSize
	if w.n < lim {
		lim = w.n
	}
	for i := 1; i < lim; i++ {
		if w.samples[i] < m {
			m = w.samples[i]
		}
	}
	return m
}

// EnableBond makes this node hold an identity-bound storage bond of size bytes
// and starts advertising its root on outgoing messages, so peers can challenge
// it. Holding the plot is the cost; a validator must EnableBond to build
// consensus standing. If a plot store is attached (SetPlotStore) and already
// holds this identity's plot, it is RELOADED and re-verified against its
// committed root (B7) — a restart never re-plots (#93); otherwise the plot is
// generated once and persisted. (The plot is still held in memory; a
// disk-backed lazy commitment and moving plotting off the core loop are the
// recorded hardening follow-ups — see the core/bond package doc.)
// It returns whether the plot was RELOADED from disk (true) rather than freshly
// sealed (false), so a restart can honestly say it reused the plot instead of
// logging the identical wording as a first-time seal (acceptance F7).
func (n *Node) EnableBond(signer ed25519.PrivateKey, size int64) (reloaded bool) {
	// Record the identity signer so a node holding a bond can mint an on-chain
	// bond registration (RegisterBondReg, F6) even before it joins consensus. It
	// is the same key EnableChain uses (the node's one identity), so this is
	// consistent whether a validator enables the bond or the chain first.
	if n.signer == nil {
		n.signer = signer
	}
	pk := plotPubKey(signer)
	if n.plotStore != nil {
		if root, blocks, ok, err := n.plotStore.Load(n.id); ok && err == nil {
			if c, rerr := bond.Reconstruct(pk, size, blocks); rerr == nil && c.Root == root {
				n.bond = c // reloaded from disk, re-verified — no re-plot (#93)
				return true
			}
			n.logf(ports.LogWarn, "bond plot reload failed; re-plotting", "id", n.id)
		} else if err != nil {
			n.logf(ports.LogWarn, "bond plot load error; re-plotting", "err", err)
		}
	}
	n.bond = bond.Seal(pk, size)
	if n.plotStore != nil {
		if err := n.plotStore.Save(n.id, n.bond.Root, n.bond.Blocks()); err != nil {
			n.logf(ports.LogWarn, "bond plot persist failed", "err", err)
		}
	}
	return false
}

// StartBondAudit begins the periodic sweep in which this validator challenges
// the bonds of the validators it knows. It fires an immediate first sweep (so
// the node's own standing exists before peers are known — see the self-record
// in bondAuditOnce) and then reschedules. Needs a ledger to settle into.
func (n *Node) StartBondAudit() {
	if n.ledger == nil {
		return
	}
	n.bondAuditTick()
}

// AuditBondsOnce runs a single bond-audit sweep now — no reschedule, no decay.
// The daemon uses StartBondAudit (the loop); this is for deterministic drives
// (sim/tests) and observable manual triggers.
func (n *Node) AuditBondsOnce() { n.bondAuditOnce(uint64(n.clock.Now()) + 1) }

// ReleaseBond frees the node's resident plot bytes while it keeps ADVERTISING
// the bond (root/size still gossip out) — the F1/F2 adversary that pledged space
// then freed it to save disk, planning to recompute on demand. Under the
// byte-binding + read-bound-VDF plot it can no longer answer a live challenge, so
// it earns no standing. Adversary/test seam (cf. SetLiar for PoR).
func (n *Node) ReleaseBond() {
	if n.bond != nil {
		n.bond.ReleaseBlocks()
	}
}

func (n *Node) bondAuditTick() {
	now := uint64(n.clock.Now()) + 1 // +1 so the first tick is never 0 ("unset")
	n.bondAuditOnce(now)
	// Standing must be SUSTAINED: retire any bond not re-proven within
	// BondMaxAge, so a validator that stops answering loses its vote.
	n.ledger.DecayStale(now, uint64(n.cfg.BondMaxAge))
	n.clock.AfterFunc(n.cfg.BondAuditInterval, n.bondAuditTick)
}

// bondAuditOnce challenges every known peer bond once and settles the results
// into the ledger. Exposed (unexported, same package) so tests can drive a
// single deterministic sweep without the self-rescheduling timer.
func (n *Node) bondAuditOnce(now uint64) {
	if n.ledger == nil {
		return
	}
	// Record our OWN bond first: we hold it, so asserting it to ourselves is
	// honest self-knowledge — and it is what our local proposer/attester
	// pre-check reads, which a node cannot otherwise satisfy since it never
	// challenges itself (each validator judges by its own ledger). PEERS
	// still verify our bond independently over the wire, so a self-assertion
	// buys nothing with the quorum — only real held storage does.
	if n.bond != nil && n.bond.Size >= n.cfg.MinBondBytes {
		n.ledger.RecordBondChallenge(n.id, n.bond.Root, n.bond.Size, true, now)
		// Narrate our own standing every sweep so an operator can SEE the
		// earned-standing mechanism the whole of M0 rests on actually working —
		// rising as bonds prove out, decaying if they lapse (acceptance F7).
		n.logf(ports.LogInfo, "standing", "self", n.id, "reputation", n.ledger.Reputation(n.id))
	} else if n.bond != nil {
		// Below the anti-release floor: too small to be safe against release +
		// just-in-time re-plot, so it earns nothing (F1/F2).
		n.logf(ports.LogWarn, "bond below anti-release floor — earns no standing", "size", n.bond.Size, "floor", n.cfg.MinBondBytes)
	}
	// Snapshot: the callbacks below mutate nothing here, but a peer could be
	// learned mid-sweep — challenge the set we knew at sweep start.
	type target struct {
		id   ports.NodeID
		info bondInfo
	}
	var targets []target
	for id, info := range n.peerBonds {
		if id == n.id {
			continue
		}
		targets = append(targets, target{id, info})
	}
	for _, t := range targets {
		// Piggyback issuer-key discovery on the audit: a validator needs each
		// peer validator's token-issuer key to verify the publish tokens it
		// blind-signed (T3). Peers may not have been up at bootstrap, so fetch
		// lazily and self-heal here (cheap; cached once obtained).
		if n.IssuerKeyOf(t.id) == nil {
			n.FetchIssuerKey(t.id, func(error) {})
		}
		n.rid++
		nonce := n.rid
		id, info := t.id, t.info
		sent := n.clock.Now() // for the reply-latency gate (BREAK 1 / A5)
		n.request(id, ports.Message{Kind: ports.MsgBondChallenge, Nonce: nonce},
			func(resp ports.Message, err error) {
				if err != nil {
					return // unreachable this round; DecayStale handles sustained absence
				}
				ans, derr := bond.DecodeAnswer(resp.Data)
				// Standing rests on the SOUND signals only: the anti-release floor
				// (info.size ≥ MinBondBytes — a compute-window bond, PR1), a decodable
				// answer, identity binding (sha256(PK)==id, so a plot sealed for another
				// identity/size cannot pass), and the space+labeling proof (VerifySpaceTime,
				// G2). A single slow reply is NOT in this conjunction — build-immutable #3:
				// reply-latency is transport+compute, and gating security on the sum is
				// unsound on the open internet (it read network jitter/loss as a cheat, #289).
				ok := info.size >= n.cfg.MinBondBytes && derr == nil &&
					sha256.Sum256(ans.PK) == id &&
					bond.VerifySpaceTime(ans.PK, info.root, info.size, nonce, ans, vdf.Default(), n.cfg.BondVDFDelay, n.cfg.BondLabelSamples)
				// Replied-but-can't-prove is a FAIL (a liar advertising a bond it doesn't
				// hold) → standing zeroed; a valid answer earns it. The root binds standing
				// to the plot: a shared root credits only its first owner (credit dedup),
				// so co-located Sybils pointing at one plot earn one bond's standing.
				n.ledger.RecordBondChallenge(id, info.root, info.size, ok, now)
				// C1 partial-storage timing signal — now a SOFT, disclosed deterrent
				// (owned-residual A5; build-immutable #3), never a standing gate. A
				// partial-storage prover recomputes the ε it deleted on EVERY challenge,
				// so past the DRSample work knee its reply floor is CONSISTENTLY elevated;
				// an honest node on a jittery/lossy path is only RANDOMLY slow. We read the
				// windowed MINIMUM of the peer's reply latencies (the low quantile), which
				// filters the one-sided network noise, and flag only a SUSTAINED floor over
				// the deadline. No hard action in M0 (the sound structural close is
				// tight-PoS / H-track): the flag is disclosed for out-of-band review and a
				// harder re-challenge, so a bad network path can never cost standing.
				if n.cfg.BondMaxAnswerLatency > 0 {
					w := n.peerBondRTT[id]
					if w == nil {
						w = &latWindow{}
						n.peerBondRTT[id] = w
					}
					if ok {
						w.observe(ports.Duration(n.clock.Now() - sent)) // only real proofs estimate the floor
					}
					if sustainedSlow := ok && w.full() && w.min() > n.cfg.BondMaxAnswerLatency; sustainedSlow {
						n.logf(ports.LogWarn, "bond challenge: sustained-latency suspicion (soft, non-gating)",
							"peer", id, "floor", ports.Duration(w.min()), "deadline", n.cfg.BondMaxAnswerLatency)
					}
				}
				// Narrate the verdict: a peer proving (or failing to prove) its bond is
				// exactly the "is the trust plane working?" moment an operator needs (F7).
				late := n.cfg.BondMaxAnswerLatency > 0 && ports.Duration(n.clock.Now()-sent) > n.cfg.BondMaxAnswerLatency
				n.logf(ports.LogInfo, "bond challenge", "peer", id, "passed", ok, "late", late, "standing", n.ledger.Reputation(id))
			})
	}
}

// answerBondChallenge is the prover side: prove we still hold the bond we
// advertised by answering the challenge from our sealed blob. No bond, or a
// block we no longer hold, yields an empty reply — which the challenger scores
// as a failure.
// challengerRate tracks one challenger's bond-challenge eval budget in the
// current window (#424). The same shape budgets bond-reg SUBMITS per sender
// (allowBondSubmit — the Phase 1.2 CPU gate).
type challengerRate struct {
	windowStart ports.Time
	count       int
}

// bondSubmitBurst caps the MsgSubmitBondReg messages ONE sender may have
// examined per ChainSyncInterval window. A well-formed self-signed reg forces
// up to one VerifySpaceTime (~ms of single-loop CPU — measured in
// core/bond/verifycost_bench_test.go), and nothing else bounds the rate, so an
// authenticated flooder holds the loop at a permanent duty cycle for free (the
// #424 CPU-DoS, one message kind over). Honest cadence is ONE submit per sweep
// (SubmitBondRenewal fires only while BondRenewalDue, once per
// ChainSyncInterval), plus transport retries; 8 clears that with wide headroom.
// Per-sender (not global) so a flooder cannot starve honest submitters.
const bondSubmitBurst = 8

// entrySubmitBurst caps the MsgSubmitEntry messages ONE sender may have
// examined per ChainSyncInterval window (#183 red-team F-1). Under
// -require-tokens, ValidateEntry runs an RSA verify per token signature, and
// nothing else bounds the arrival rate — so an authenticated flooder rides
// per-message crypto onto the single consensus loop for a few fabricated
// bytes, exactly the sibling #424 CPU-DoS that hardened MsgSubmitBondReg. This
// is the same cheap FRONT gate: a refusal costs a map lookup. Honest cadence is
// a client submit-then-poll per published object; 32 clears a modest
// batch-publish with headroom, and a refused honest submit heals by the client
// resubmitting next window (the B5 NAK carries the reason). Per-sender (not
// global) so a flooder cannot starve honest publishers. The reorder in
// ValidateEntry (spent-check before Verify) removes the N×-per-message replay
// amplifier; this gate bounds the residual per-message floor.
const entrySubmitBurst = 32

// allowEntrySubmit reports whether a MsgSubmitEntry from `from` may be examined
// now, charging one unit against its per-window budget. The cheap gate in
// FRONT of entryDecode + ValidateEntry (which, under -require-tokens, does the
// RSA work) — a refusal costs a map lookup, so a flooder gains no
// amplification. Window = ChainSyncInterval (the honest submit cadence clock);
// a refused honest submit heals by the client's resubmit, exactly like
// allowBondSubmit.
func (n *Node) allowEntrySubmit(from ports.NodeID) bool {
	now := n.clock.Now()
	window := n.cfg.ChainSyncInterval
	if window <= 0 {
		window = 30 * ports.Second
	}
	r := n.entrySubmitRate[from]
	if r == nil || ports.Duration(now-r.windowStart) >= window {
		if r == nil && len(n.entrySubmitRate) >= maxBondChallengers {
			for id, e := range n.entrySubmitRate {
				if ports.Duration(now-e.windowStart) >= window {
					delete(n.entrySubmitRate, id)
				}
			}
		}
		n.entrySubmitRate[from] = &challengerRate{windowStart: now, count: 1}
		return true
	}
	if r.count >= entrySubmitBurst {
		return false // budget spent this window — refuse before decode/verify
	}
	r.count++
	return true
}

// allowBondSubmit reports whether a MsgSubmitBondReg from `from` may be
// examined now, charging one unit against its per-window budget. It is the
// cheap gate in FRONT of decode+signature+VerifySpaceTime — a refusal costs a
// map lookup, so a flooder gains no amplification. Window = ChainSyncInterval
// (the honest submit cadence clock); a refused honest submit heals by the
// existing resubmit-next-sweep retry, exactly like a WAN-skew refusal.
func (n *Node) allowBondSubmit(from ports.NodeID) bool {
	now := n.clock.Now()
	window := n.cfg.ChainSyncInterval
	if window <= 0 {
		window = 30 * ports.Second
	}
	r := n.bondSubmitRate[from]
	if r == nil || ports.Duration(now-r.windowStart) >= window {
		if r == nil && len(n.bondSubmitRate) >= maxBondChallengers {
			for id, e := range n.bondSubmitRate {
				if ports.Duration(now-e.windowStart) >= window {
					delete(n.bondSubmitRate, id)
				}
			}
		}
		n.bondSubmitRate[from] = &challengerRate{windowStart: now, count: 1}
		return true
	}
	if r.count >= bondSubmitBurst {
		return false // budget spent this window — refuse before decode/verify
	}
	r.count++
	return true
}

const (
	// bondChallengeBurst caps the bond-challenge VDF-evals this node will serve
	// to ONE challenger per BondAuditInterval window. Answering forces a fresh
	// sequential VDF-eval — the unpredictable nonce is exactly what cannot be
	// precomputed — all on the node's single goroutine, so an unbounded
	// challenger is a remote CPU-DoS (#424, red-team seam #7). Honest cadence is
	// one challenge per peer per BondAuditInterval, plus a few transport retries
	// of the same nonce; this cap clears that with wide headroom and denies the
	// flood. Per-challenger (not global) so a flooder cannot starve honest
	// challengers of their own audit budget.
	bondChallengeBurst = 8
	// maxBondChallengers bounds the per-challenger table; past it, stale windows
	// are swept before a new challenger is admitted (same idiom as maxTokenIssued).
	maxBondChallengers = 4096
)

// allowBondChallenge reports whether a bond challenge from `from` may be
// answered now, charging one unit against its per-window budget (#424). It is
// the cheap gate in front of the expensive AnswerSpaceTime eval — a refusal
// costs nothing, so a flooder gains no amplification.
func (n *Node) allowBondChallenge(from ports.NodeID) bool {
	now := n.clock.Now()
	window := n.cfg.BondAuditInterval
	if window <= 0 {
		window = 60 * ports.Second
	}
	r := n.bondChallengeRate[from]
	if r == nil || ports.Duration(now-r.windowStart) >= window {
		if r == nil && len(n.bondChallengeRate) >= maxBondChallengers {
			for id, e := range n.bondChallengeRate {
				if ports.Duration(now-e.windowStart) >= window {
					delete(n.bondChallengeRate, id)
				}
			}
		}
		n.bondChallengeRate[from] = &challengerRate{windowStart: now, count: 1}
		return true
	}
	if r.count >= bondChallengeBurst {
		return false // budget spent this window — refuse before the costly eval
	}
	r.count++
	return true
}

func (n *Node) answerBondChallenge(from ports.NodeID, msg ports.Message) ports.Message {
	reply := ports.Message{Kind: ports.MsgBondReply}
	if n.bond == nil {
		return reply
	}
	if !n.allowBondChallenge(from) {
		return reply // #424: per-challenger rate-limited — refuse WITHOUT the VDF-eval
	}
	ans, ok := n.bond.AnswerSpaceTime(msg.Nonce, vdf.Default(), n.cfg.BondVDFDelay, n.cfg.BondLabelSamples)
	if !ok {
		return reply
	}
	if data, err := bond.EncodeAnswer(ans); err == nil {
		reply.Data = data
	}
	return reply
}
