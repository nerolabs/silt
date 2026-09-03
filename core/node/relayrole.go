package node

// PoD relay lane — the relay-accept role (docs/design/pod.md §7.3). R0.7 INTERIM
// (2026-09-03): settlement PAYS 0 until the R2.14 prepayment anchor lands; the
// 2026-08-30 certification did not cover the shipped code's conservation (the
// RELAY-LANE fix certification of 2026-09-03 supersedes it). A relay/gateway forwards content-blind bytes toward a fetcher and
// is paid as-it-goes by a sender-funded PayWord hash chain (core/relaypay). It
// cannot sign a completed-delivery receipt because it never holds a verifiable
// object, so there is no delivery-receipt lane here — the fetcher commits a
// chain root once, reveals one preimage per forwarded increment, and the relay
// redeems the highest preimage it holds at session close
// (credit.RedeemRelayCredit — pays 0 until R2.14; balance-only when it pays).
//
// TWO M0 GUARDS (bright-line, non-negotiable — immutable Don't-#3):
//
//   (i)  The chain root MUST bind to a blind credit under a FRESH EPHEMERAL
//        identity, never a durable one. The fetcher funds the chain through
//        client.WithdrawDemandTokenPrivately (client/privissue.go:48), which
//        spins up a throwaway keypair, withdraws a blind credit over it, and
//        tears it down — so the issuer authenticates only an ephemeral key and
//        charges no account it can tie to the fetcher. A durable-funded chain is
//        REJECTED at OpenRelaySession: binding a chain to a durable identity
//        turns the relay into a party that can tie the fetcher's durable
//        identity to what it fetched.
//
//   (ii) A FRESH ephemeral identity AND a FRESH chain per session. No ephemeral
//        identity and no chain root is reused across sessions. Reuse upgrades the
//        relay from a per-session observer (which sees only the IP it already
//        routes) to a LONGITUDINAL one — a real Don't-#3 regression.
//
// Both guards are enforced by construction here, not by a note: OpenRelaySession
// rejects FundingDurableAccount and rejects any already-seen ephemeral identity
// or chain root. Their failing-first guards are in relayrole_test.go.

import (
	"sync"

	"github.com/nerolabs/silt/core/relaypay"
	"github.com/nerolabs/silt/ports"
)

// FundingSource identifies how the fetcher funded the PayWord chain root. Only
// the ephemeral-blind path is accepted (M0 guard (i)); the durable-account form
// exists solely so a caller can present it and be rejected — the guard has an
// object to test.
type FundingSource int

const (
	// FundingEphemeralBlind is the ONLY accepted funding source: a blind credit
	// withdrawn under a fresh ephemeral identity (client.WithdrawDemandTokenPrivately).
	FundingEphemeralBlind FundingSource = iota
	// FundingDurableAccount is a chain funded by a durable-account credit. It is
	// always REJECTED (M0 guard (i)) — it would link the fetcher's durable
	// identity to what it fetched.
	FundingDurableAccount
)

// ErrRelayAcceptDisabled is returned when the relay-accept gate is off.
type relayError string

func (e relayError) Error() string { return string(e) }

const (
	errRelayAcceptDisabled = relayError("relay: PayWord chains not accepted (relay-accept gate off)")
	errRelayDurableFunding = relayError("relay: chain root funded by a durable account rejected (M0 guard (i): funding must be an ephemeral blind credit)")
	errRelayEphemeralReuse = relayError("relay: ephemeral identity reused across sessions rejected (M0 guard (ii): one ephemeral identity per session)")
	errRelayChainReuse     = relayError("relay: chain root reused across sessions rejected (M0 guard (ii): one chain per session)")
	errRelayChainTooLong   = relayError("relay: committed chain length S exceeds S_max = MaxSessionBytes / RelayIncrementBytes (#644 DoS clamp: bound the AdvanceTo walk)")
)

// relayRetentionEpochs is the seen-map retention window in epochs (#645). Keeping
// the CURRENT and PREVIOUS epoch (window = 1 epoch back) protects a session opened
// just before a boundary across the boundary, while bounding the maps to
// sessions-per-epoch × 2 entries rather than the full admit history. A chain root
// and ephemeral identity are single-use within an epoch, so an entry only needs to
// outlive the sessions that could reuse it — one epoch back covers that.
const relayRetentionEpochs = 1

// relayMaxLiveSessions is the hard per-node ceiling on concurrent live paid-relay
// sessions (Batch-2 leak fix, PE ruling 2026-08-30). The session table is written
// from the wire on every guard-passing MsgRelayOpen and drained only at settlement
// (never called on the live path yet), so without a cap a flood of cheap fresh-
// identity opens grows it without bound between epoch sweeps. This ceiling bounds
// growth BETWEEN sweeps: past it, a new open is refused (OK=false) rather than
// admitted. The specific value is a TUNING call, not a correctness one — 4096 is a
// conservative default (~4096 × ~200 B ≈ 800 KB worst-case table), well above any
// honest concurrent-session count on a single relay and far below an OOM. Raise it
// if a legitimate deployment ever saturates it.
const relayMaxLiveSessions = 4096

// errRelaySessionCap is returned when the live session table is at the hard cap.
const errRelaySessionCap = relayError("relay: live session table at capacity (per-node concurrent-session cap); retry after in-flight sessions settle")

// EnableRelayAccept opts this node into accepting sender-funded PayWord chains.
// Off by default; the mirror of EnableDemandBank for the delivery lane. The
// daemon wires it behind a flag (cmd/silt/daemon.go).
func (n *Node) EnableRelayAccept() {
	n.relayAccept = true
	n.relaySeenEph = make(map[ports.NodeID]uint64)
	n.relaySeenRoot = make(map[string]uint64)
	n.relaySessions = make(map[uint64]*RelaySession)
	n.relayEvictionFloor = 0
	// Production epoch source: the chain's head height / EpochBlocks — a sequential
	// epoch index. With no chain or epochs disabled the epoch is 0 (no eviction; the
	// maps still bound-check but never rotate, which is the pre-epoch behavior).
	n.relayEpochFn = func() uint64 {
		if n.chain == nil {
			return 0
		}
		eb := n.chain.EpochBlocks()
		if eb == 0 {
			return 0
		}
		_, height := n.chain.Head()
		return height / eb
	}
}

// sweepRelaySeen evicts seen-map entries AND stale unsettled live sessions older
// than the retention window, keyed on the current epoch, with a MONOTONIC eviction
// floor (#645). The floor never lowers: a reorg that moves the epoch backward
// (epochStart is reorg-swapped) must not un-evict a swept entry, or a previously-
// seen root could be re-admitted — a guard-(ii) regression. So the floor is
// max(previousFloor, epoch - retention).
//
// The session sweep shares the SAME floor as the seen-map sweep (Batch-2 leak fix,
// PE ruling 2026-08-30): a session admitted in an epoch below the floor is an
// unsettled leak — the settlement path is not wired on the live wire path yet — so
// it is reaped here. This is the epoch/TTL half of the fix; relayMaxLiveSessions is
// the hard-cap half that bounds growth BETWEEN sweeps.
func (n *Node) sweepRelaySeen(epoch uint64) {
	var newFloor uint64
	if epoch > relayRetentionEpochs {
		newFloor = epoch - relayRetentionEpochs
	}
	// Monotonic: the floor only moves forward, never backward on a reorg.
	if newFloor <= n.relayEvictionFloor {
		return // nothing new to evict; the floor did not advance
	}
	n.relayEvictionFloor = newFloor
	for id, e := range n.relaySeenEph {
		if e < newFloor {
			delete(n.relaySeenEph, id)
		}
	}
	for root, e := range n.relaySeenRoot {
		if e < newFloor {
			delete(n.relaySeenRoot, root)
		}
	}
	// Reap stale unsettled sessions on the same monotonic floor. Batch-3 wired the
	// daemon control-frame binding, so a reaped session may have a LIVE pump
	// goroutine blocked in paidPump on auth.Wait(), waiting for a ceiling the
	// departed fetcher will never raise. closeSession() marks the session done and
	// wakes the pump so it drains to its current ceiling and returns — no leaked
	// goroutine. closeSession is idempotent (wake is a non-blocking send), so a later
	// SettleRelaySession close on the same handle is harmless; but the delete here
	// removes the handle, so the normal-close settle finds it absent and cannot
	// double-settle (design §3b). A reaped-but-unsettled session forwarded only up to
	// its authorized ceiling, so dropping it without a settle forfeits no owed credit
	// beyond what an unclaimed session already forfeits.
	for handle, sess := range n.relaySessions {
		if sess.admitEpoch < newFloor {
			sess.closeSession() // stop the now-live pump before dropping the entry
			delete(n.relaySessions, handle)
		}
	}
}

// RelaySession is one live relay-payment session: the relay-side PayWord
// verifier keyed on the committed chain root. It holds exactly one preimage
// (32 B) regardless of chain length.
//
// The transport (Batch 2) adds the state a live session needs beyond the pure
// verifier: the fetcher's ephemeral identity (the settlement payee-source), the
// committed budget (S credits, the conservation cap), and the authorizer bridge
// the paid pump reads (authorized-byte ceiling + a wake signal). The verifier's
// monotonic count IS the settlement accumulator — settlement at close redeems
// count × increment once, capped at the budget (design §5).
type RelaySession struct {
	verifier *relaypay.Verifier

	ephID  ports.NodeID // the fetcher's fresh ephemeral identity (settlement source)
	budget int64        // S × RelayIncrementCredit — the conservation cap

	// admitEpoch is the epoch this session was opened in. The session sweep
	// (sweepRelaySessions) evicts sessions whose admit epoch has aged past the
	// retention window, mirroring the #645 seen-map eviction. A wire open that is
	// never settled cannot grow the table forever: it is reaped at the next
	// epoch-tied sweep, and the hard cap bounds growth between sweeps.
	admitEpoch uint64

	// authorizer bridge (design §2 Option A: the node drives, the adapter gates).
	// authBytes is the forward-byte ceiling the paid pump may deliver up to; it
	// rises to count × RelayIncrementBytes on each verified Pay. signal wakes the
	// pump; done marks the session closing so the pump stops blocking.
	mu        sync.Mutex
	authBytes int64
	done      bool
	signal    chan struct{}
}

// newRelaySession builds a live session from a fresh verifier and its M0-verified
// identity/budget. The signal channel is buffered depth-1 so a Pay that raises the
// ceiling never blocks on a pump that is mid-forward.
func newRelaySession(v *relaypay.Verifier, ephID ports.NodeID, budget int64) *RelaySession {
	return &RelaySession{verifier: v, ephID: ephID, budget: budget, signal: make(chan struct{}, 1)}
}

// AuthorizedBytes reports the forward-byte ceiling the paid pump may deliver up to
// (the relay adapter's authorizer seam). It is count × RelayIncrementBytes.
func (s *RelaySession) AuthorizedBytes() int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.authBytes
}

// Wait returns the pump's wake channel (the authorizer seam).
func (s *RelaySession) Wait() <-chan struct{} { return s.signal }

// Done reports the session is closing — no further authorization will arrive (the
// authorizer seam). The pump stops blocking and drains to its current ceiling.
func (s *RelaySession) Done() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.done
}

// raiseCeiling recomputes the authorized-byte ceiling from the verifier count and
// wakes the pump. Called after each verified Pay. The ceiling only ever rises (the
// count is monotonic), so a pump reading a stale value only under-delivers, never
// over-delivers.
func (s *RelaySession) raiseCeiling() {
	s.mu.Lock()
	s.authBytes = int64(s.verifier.Count()) * relaypay.RelayIncrementBytes
	s.mu.Unlock()
	s.wake()
}

// closeSession marks the session done and wakes the pump so it can drain and stop.
func (s *RelaySession) closeSession() {
	s.mu.Lock()
	s.done = true
	s.mu.Unlock()
	s.wake()
}

func (s *RelaySession) wake() {
	select {
	case s.signal <- struct{}{}:
	default:
	}
}

// OpenRelaySession opens a relay-payment session for a fetcher's PayWord chain.
// It enforces the gate and the two M0 guards:
//   - the relay-accept gate must be on (errRelayAcceptDisabled);
//   - funding must be FundingEphemeralBlind, never durable (guard (i));
//   - ephID must not have opened a prior session, and root must not have been
//     seen before (guard (ii)).
//
// ephID is the fresh ephemeral NodeID the issuer authenticated (the value
// client.WithdrawDemandTokenPrivately returns), root is the committed chain root
// x_0, and S is the chain length. On success it records ephID and root as seen
// and returns a session whose verifier starts from the root.
func (n *Node) OpenRelaySession(ephID ports.NodeID, root []byte, S int, funding FundingSource) (*RelaySession, error) {
	if !n.relayAccept {
		return nil, errRelayAcceptDisabled
	}
	// #645: sweep stale-epoch seen entries lazily on the open path (this is exactly
	// when the maps grow). Eviction is epoch-tied with a monotonic floor, so a
	// reorg cannot un-evict a swept entry (guard-(ii)-safe).
	epoch := n.relayEpochFn()
	n.sweepRelaySeen(epoch)

	// Hard per-node concurrent-session cap (Batch-2 leak fix, PE ruling 2026-08-30).
	// The sweep above frees any stale-epoch sessions first; if the table is STILL at
	// the ceiling, refuse the open rather than admit an unbounded entry. This bounds
	// growth between epoch sweeps against a flood of cheap fresh-identity opens.
	if len(n.relaySessions) >= relayMaxLiveSessions {
		return nil, errRelaySessionCap
	}

	// Guard (i): funding MUST be an ephemeral blind credit, never a durable account.
	if funding != FundingEphemeralBlind {
		return nil, errRelayDurableFunding
	}
	// Guard (ii): no ephemeral identity reused across sessions.
	if _, seen := n.relaySeenEph[ephID]; seen {
		return nil, errRelayEphemeralReuse
	}
	// Guard (ii): no chain root reused across sessions.
	rootKey := string(root)
	if _, seen := n.relaySeenRoot[rootKey]; seen {
		return nil, errRelayChainReuse
	}
	if S <= 0 {
		return nil, relayError("relay: chain length S must be positive")
	}
	// #644 open-side clamp: reject a chain longer than the relay will ever forward.
	// S_max is derived RELAY-SIDE (relaypay.MaxChainLength = MaxSessionBytes /
	// RelayIncrementBytes = 262,144), never trusted from the fetcher. This caps the
	// stored S the per-message AdvanceTo walk is then clamped against, bounding the
	// worst-case walk to ~262K hashes instead of the attacker-chosen millions.
	if S > relaypay.MaxChainLength {
		return nil, errRelayChainTooLong
	}
	// Record the admit epoch so #645 eviction can age the entry out.
	n.relaySeenEph[ephID] = epoch
	n.relaySeenRoot[rootKey] = epoch
	budget := int64(S) * relaypay.RelayIncrementCredit
	sess := newRelaySession(relaypay.NewVerifier(root, S), ephID, budget)
	sess.admitEpoch = epoch // stamp for the session sweep (sweepRelaySeen)
	return sess, nil
}

// Pay authorizes the next increment by verifying a revealed preimage — one
// SHA-256 (relaypay.Verifier.Advance). If the fetcher stops revealing, the relay
// stops forwarding; the irreducible one-increment stiff is bounded small by the
// increment size (relaypay.RelayIncrementBytes, the owed floor-box measurement).
// On success it raises the paid pump's byte ceiling so the forward stream releases
// the newly-authorized increment.
func (s *RelaySession) Pay(preimage []byte) error {
	if err := s.verifier.Advance(preimage); err != nil {
		return err
	}
	s.raiseCeiling()
	return nil
}

// PayTo authorizes several increments at once by revealing x_claimedCount, walking
// the chain forward (relaypay.Verifier.AdvanceTo). The walk is bounded to at most S
// hashes by the #644 clamp. On success it raises the paid pump's ceiling to the new
// count. This is the live-path entry the wire MsgRelayPay drives.
func (s *RelaySession) PayTo(preimage []byte, claimedCount int) error {
	if err := s.verifier.AdvanceTo(preimage, claimedCount); err != nil {
		return err
	}
	s.raiseCeiling()
	return nil
}

// Count returns the number of increments the relay is authorized to redeem for
// this session (the settled amount would be Count × RelayIncrementCredit via
// credit.RedeemRelayCredit — which pays 0 until the R2.14 anchor lands).
func (s *RelaySession) Count() int { return s.verifier.Count() }
