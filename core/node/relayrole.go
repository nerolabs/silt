package node

// PoD relay lane — the relay-accept role (docs/design/pod.md §7.3). A relay/gateway
// forwards content-blind bytes toward a fetcher and is paid as-it-goes by a
// sender-funded PayWord hash chain (core/relaypay). It cannot sign a
// completed-delivery receipt because it never holds a verifiable object, so there
// is no delivery-receipt lane here — the fetcher commits a chain root once, reveals
// one preimage per forwarded increment, and the relay redeems the highest preimage
// it holds at session close (credit.RedeemRelayCredit; balance-only, never
// standing).
//
// R2.14 (2026-09-04): THE CHAIN ROOT IS ANCHORED. A session opens only against k
// prepayment anchors — blind signatures under THIS relay's own chain-committed
// per-epoch demand key (blindtoken relayAnchorDomain) that the fetcher's durable
// identity bought from this relay through the ordinary withdrawal (a refusable
// ChargePublish on this relay's ledger) — spent once into the ledger's bounded,
// durable guard BEFORE admission, and the session's budget is the ledger's own
// Σ face of what it spent. Rivest–Shamir's authorization half, bilateral form
// (issuer == relay). Certification:
// silt-reviews/research/research-outcome/R2.14-relay-prepayment-anchor-CONSTRUCTION-RESEARCH-CERTIFICATION-2026-09-04.md.
// The R0.7 interim (pays 0) is retired by it. BUILT ≠ LIVE: an anchor verifies
// only under a v5 IssuerKeyReg, so the lane is dark until era-4 and every open is
// refused with a named reason until then (cert §8).
//
// TWO M0 GUARDS (bright-line, non-negotiable — immutable Don't-#3):
//
//   (i)  The chain root MUST bind to a blind credential, never to a durable
//        account. ENFORCED BY the anchor: a blind bearer credential verified
//        under a chain-committed key. The relay signed it blind at purchase and
//        holds no serial↔buyer map, so the ledger cannot link the anchor to the
//        durable identity that paid (cert §4.1 — guard (i) is satisfied
//        cryptographically). FundingSource is kept as the guard's TEST OBJECT (a
//        durable-funded declaration is still refused); it enforces nothing on
//        its own. The NETWORK residual is the delivery lane's D3 residual,
//        unchanged: the relay saw the buyer's IP at purchase and the ephemeral's
//        IP at open, so the anonymity set is this relay's anchor buyers in the
//        W+1-epoch band, partitioned by k and by IP (R-RELAY-ANON-SET, cert §4.2).
//        Buy ahead and in fixed bundles.
//
//   (ii) A FRESH ephemeral identity AND a FRESH chain per session. No ephemeral
//        identity and no chain root is reused across sessions. Reuse upgrades the
//        relay from a per-session observer (which sees only the IP it already
//        routes) to a LONGITUDINAL one — a real Don't-#3 regression. The anchor
//        guard REINFORCES it: an anchor spent in one session cannot appear in
//        another for its whole W+1-epoch life.
//
// Both guards are enforced by construction here, not by a note: OpenRelaySession
// rejects FundingDurableAccount, rejects any already-seen ephemeral identity or
// chain root, and admits nothing without verified, freshly spent anchors. Their
// failing-first guards are in relayrole_test.go and r214_relay_anchor_test.go.

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"sync"

	"github.com/nerolabs/silt/core/blindtoken"
	"github.com/nerolabs/silt/core/credit"
	"github.com/nerolabs/silt/core/demand"
	"github.com/nerolabs/silt/core/relaypay"
	"github.com/nerolabs/silt/ports"
)

// FundingSource identifies how the fetcher funded the PayWord chain root. Only
// the ephemeral-blind path is accepted (M0 guard (i)); the durable-account form
// exists solely so a caller can present it and be rejected — the guard has an
// object to test.
type FundingSource int

const (
	// FundingEphemeralBlind is the ONLY accepted funding source. R2.14: the funding
	// that actually settles is the k blind-signed anchors bought under the fetcher's
	// DURABLE identity (AcquireRelayAnchors → ChargePublish on the relay's ledger); the
	// D3 path (a publish credit converted via WithdrawDemandTokenPrivately) is NOT
	// anchor-eligible: F-4 (creditSpent durability) is closed by R2.13b, but eligibility
	// is the Researcher's C-4 re-certification, not a consequence of the close.
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
	errRelayBadRoot        = relayError("relay: chain root must be exactly one hash wide")

	// The R2.14 anchor refusals — named S5 reasons, each surfaced in the open ack.
	errRelayNoAnchor        = relayError("relay: session open carries no prepayment anchor (an unanchored session funds nothing; R2.14)")
	errRelayTooManyAnchors  = relayError("relay: session open carries more anchors than MaxAnchorsPerSession")
	errRelayAnchorMalformed = relayError("relay: prepayment anchor serial or signature is malformed")
	errRelayFetcherMismatch = relayError("relay: sha256(Fetcher) != authenticated sender — the session-open commitment is not the sender's")
	errRelayOpenSigInvalid  = relayError("relay: session-open commitment signature invalid (M binds relayID, root, S, k and every serial)")
	errRelayNoIssuerKey     = relayError("relay: no self demand-issuer keyset (no chain commitment for key_E, or keys not scheduled) — the anchor lane is dark until era-4")
	errRelayAnchorInvalid   = relayError("relay: prepayment anchor does not verify under this relay's committed key (wrong relay, wrong lane, or expired)")
	errRelayAnchorSpent     = relayError("relay: prepayment anchor already spent on this ledger")
	errRelayGuardFull       = relayError("relay: anchor guard full of live entries — refused, never evicted")
	errRelayGuardRefused    = relayError("relay: anchor guard refused the spend")
	errRelayNoLedger        = relayError("relay: no ledger wired — no paying ledger to spend anchors on")
	errRelayNoSigner        = relayError("relay: this node has no signer to commit the session open with")
)

// relayOpenDomain is the domain of the session-open commitment M (R2.14; the
// crypto advisory §1.3): M = sha256(relayOpenDomain ‖ relayID ‖ Root ‖ uint32BE(S)
// ‖ uint32BE(k) ‖ serial_1 ‖ … ‖ serial_k). Every field is fixed-width (the root is
// one hash, a serial is blindtoken.SerialSize — both enforced before M is
// recomputed), so the encoding is injective without length prefixes. This is
// Rivest–Shamir's M = {vendor, C_U, w_0, …}_SK_U: vendor = relayID, C_U = the
// anchor serials, w_0 = Root, SK_U = the session ephemeral.
const relayOpenDomain = "silt/relay/open/v1"

// relayOpenCommitment recomputes M for the relay named by relayID. The fetcher
// signs it with the session ephemeral (OpenRelaySessionRemote); the relay verifies
// it under the presented Fetcher key before any RSA work (OpenRelaySession).
func relayOpenCommitment(relayID ports.NodeID, root []byte, S int, anchors []relaypay.Anchor) []byte {
	h := sha256.New()
	h.Write([]byte(relayOpenDomain))
	h.Write(relayID[:])
	h.Write(root)
	var b4 [4]byte
	binary.BigEndian.PutUint32(b4[:], uint32(S))
	h.Write(b4[:])
	binary.BigEndian.PutUint32(b4[:], uint32(len(anchors)))
	h.Write(b4[:])
	for _, a := range anchors {
		h.Write(a.Serial)
	}
	return h.Sum(nil)
}

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
	budget int64        // Σ face of the anchors spent at open — the conservation cap (R2.14; never S × inc)

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
//
// The ceiling is min(count, budget) × RelayIncrementBytes (R2.14, cert T-9): the
// relay never forwards past the increments the spent anchors fund, so the funding
// cap is visible on the wire rather than discovered at settlement.
func (s *RelaySession) raiseCeiling() {
	s.mu.Lock()
	s.authBytes = s.fundedCount() * relaypay.RelayIncrementBytes
	s.mu.Unlock()
	s.wake()
}

// fundedCount is min(count, budget / RelayIncrementCredit): the increments both
// revealed and paid for.
func (s *RelaySession) fundedCount() int64 {
	return min(int64(s.verifier.Count()), s.budget/relaypay.RelayIncrementCredit)
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

// OpenRelaySession opens a relay-payment session for a fetcher's PayWord chain,
// in the certified verify ORDER (cert §9 T-7; advisory §1.4) so a refused open costs
// the least it can and leaves no trace:
//
//  1. the relay-accept gate; the epoch sweep; the live-session cap (free);
//  2. guard (i) funding; guard (ii) ephemeral + root seen-maps (map lookups);
//  3. S and root bounds, the #644 clamp;
//  4. k bounds: 1 ≤ k ≤ MaxAnchorsPerSession; serial and signature widths;
//  5. sha256(fetcherPub) == ephID, then ed25519 over the commitment M (~50 µs);
//  6. each anchor under the SELF keyset — n.DemandIssuerKeyset(n.id), NEVER a
//     peer's (G-A5: an anchor is a claim on the ledger that burned its fee, and only
//     this relay's ledger did) — newest epoch first, stop at the first failure;
//  7. ledger.SpendRelayAnchors, all-or-nothing, durable, before admission;
//  8. record the seen-maps; budget := Σ face; admit.
//
// ephID is the fresh ephemeral NodeID the transport authenticated; root is the
// committed chain root x_0; S is the chain length; anchors are the prepayment
// credentials the root is committed under; fetcherPub and sig are the ephemeral's
// public key and its signature over M (relayOpenCommitment). On success it records
// ephID and root as seen and returns a session whose verifier starts from the root
// and whose budget is the ledger's Σ face.
func (n *Node) OpenRelaySession(ephID ports.NodeID, root []byte, S int, funding FundingSource,
	anchors []relaypay.Anchor, fetcherPub, sig []byte) (*RelaySession, error) {
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
	// the ceiling, refuse the open rather than admit an unbounded entry.
	if len(n.relaySessions) >= relayMaxLiveSessions {
		return nil, errRelaySessionCap
	}

	// Guard (i): the declared funding MUST be the ephemeral blind form.
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
	// RelayIncrementBytes = 262,144), never trusted from the fetcher.
	if S > relaypay.MaxChainLength {
		return nil, errRelayChainTooLong
	}
	if len(root) != sha256.Size {
		return nil, errRelayBadRoot
	}

	// R2.14 step 4: k bounds and field widths, before any crypto.
	if len(anchors) == 0 {
		return nil, errRelayNoAnchor
	}
	if len(anchors) > relaypay.MaxAnchorsPerSession {
		return nil, errRelayTooManyAnchors
	}
	for _, a := range anchors {
		if len(a.Serial) != blindtoken.SerialSize || len(a.Sig) == 0 || len(a.Sig) > blindtoken.MaxModulusBits/8 {
			return nil, errRelayAnchorMalformed
		}
	}
	// Step 5: the commitment. The ephemeral that signed M must be the sender the
	// transport authenticated, and M must cover exactly what is presented.
	if len(fetcherPub) != ed25519.PublicKeySize || sha256.Sum256(fetcherPub) != ephID {
		return nil, errRelayFetcherMismatch
	}
	if !ed25519.Verify(ed25519.PublicKey(fetcherPub), relayOpenCommitment(n.id, root, S, anchors), sig) {
		return nil, errRelayOpenSigInvalid
	}
	// Step 6: RSA under the SELF keyset only. With no chain, no committed key_E for
	// this relay, or keys never scheduled, there is no keyset and the lane is dark
	// with a named reason — the certified direction until era-4 (cert §8, T-13).
	if n.ledger == nil {
		return nil, errRelayNoLedger
	}
	ks := n.DemandIssuerKeyset(n.id)
	if ks == nil {
		return nil, errRelayNoIssuerKey
	}
	cur := n.chainEpoch() // the epoch the keyset was just pruned with — and the guard's clock (T-12)
	spend := make([]ports.RelayAnchor, 0, len(anchors))
	for _, a := range anchors {
		e, ok := ks.VerifyAnchorInWindow(cur, demand.Token{Serial: a.Serial, Sig: a.Sig})
		if !ok {
			return nil, errRelayAnchorInvalid // stop at the first failure: ≤ W+1 modexps for a garbage open
		}
		spend = append(spend, ports.RelayAnchor{Epoch: e, Serial: a.Serial})
	}
	// Step 7: spend all-or-nothing into the ledger's bounded durable guard. A
	// refusal records nothing (T-10) and leaves no seen-map entry either — the
	// fetcher may re-present the good anchors under the same ephemeral and root.
	face, reason := n.ledger.SpendRelayAnchors(spend, cur)
	if face <= 0 {
		switch reason {
		case credit.ReasonAlreadyPaid:
			return nil, errRelayAnchorSpent
		case credit.ReasonGuardFull:
			return nil, errRelayGuardFull
		default:
			return nil, fmt.Errorf("%w: %s", errRelayGuardRefused, reason)
		}
	}
	// Step 8: admit. Record the admit epoch so #645 eviction can age the entry out.
	n.relaySeenEph[ephID] = epoch
	n.relaySeenRoot[rootKey] = epoch
	sess := newRelaySession(relaypay.NewVerifier(root, S), ephID, face)
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

// Count returns the number of increments the fetcher has revealed for this
// session. The settled amount is min(Count, budget) × RelayIncrementCredit via
// credit.RedeemRelayCredit (R2.14).
func (s *RelaySession) Count() int { return s.verifier.Count() }
