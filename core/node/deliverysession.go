package node

// R2.9 — the paid DELIVERY SESSION: the node half (the ledger half is
// core/credit/deliveryanchor.go; the wire vocabulary core/demand/session.go). The
// certified shape, clause by clause (Researcher certification
// silt-agent-memory/researcher/reviews/research-outcome/R2.9-G-R212-8-delivery-anchor-quantization-RESEARCH-CERTIFICATION-2026-09-06.md
// §3.1; deliberation docs/thinking/2026-09-06-r2.9-node-half.md):
//
//	C1  one live session per (this server, durable fetcher); the fetcher is the
//	    authenticated wire peer, the identity the serve path already credits.
//	C2  open = k = 1 demand-domain anchor (the demand token this fetcher bought here),
//	    verified under this node's OWN committed key_E newest-epoch-first, then spent
//	    all-or-nothing into the shared paid-serial guard, DURABLY, before admission
//	    (OpenRelaySession steps 4–8, one lane over). Budget = Σ face.
//	C3  the byte ceiling is DERIVED from the face (credit.DeliveryBytesPerAnchor), never
//	    pinned: a session may acknowledge at most budget/p increments in total.
//	C4  increments are authorized by the fetcher's SIGNED cumulative count (receipt v3),
//	    not a preimage chain — the fetcher signs only after the fetch path
//	    content-verified the bytes (B3).
//	C5  settle INCREMENTALLY against a MONOTONE counter: a receipt with cumulative count
//	    j pays for the delta since the last one; a lower or equal count pays 0 by
//	    arithmetic, not by a guard lookup; settled ≤ budget always (R2.14's C-1).
//	C6  per-increment reversal per OBJECT is the ledger's (SettleDelivery).
//	C7  a session spans objects and fetch episodes until its budget is exhausted;
//	    nothing else closes it but an idle timeout. No object-keyed collection lives on
//	    the session (Don't #3, cert §7; gate G-λ-8-7).
//	C8  top-up = MsgDeliveryFund with a fresh anchor: one all-or-nothing guard spend,
//	    budget += face.
//	C9  close on exhaustion, or on IDLE measured from the last settlement on the node's
//	    INJECTED clock (never the admit epoch, never the chain — cert §6.2; wall time in
//	    production, so a forward clock step reaps every live session at once —
//	    R-SESSION-WALLCLOCK-STEP).
//	    The idle window is NO LONGER refuse-until-set. Owner call 4 of
//	    D-TRUE-UP-CALLS-2026-09-07 released it on its own conditional once the liveness
//	    bound was field-confirmed, and the SHIPPED default is 24m (cmd/silt/numeraire.go
//	    deliveryIdleDefault; value ratified D-C2-IDLE-WINDOW-VALUE 2026-09-09). What
//	    survives of the refusal: the daemon refuses a window below the derived floor
//	    (bound × divisor/(divisor−1)), and a non-positive window here leaves the lane off.
//	    The remainder budget − settled is accounted ONCE at close through the ledger's
//	    CloseDeliverySession. Under G-6 as RATIFIED (D-R2.9-NODE-HALF-CALLS call 1, amended
//	    1′) that is a REFUND, not a burn: the remainder is a DEPOSIT returned to the durable
//	    fetcher at the LATER of the anchors' release epoch and the close. The owner call is
//	    CLOSED. It burns only in the two named corners — the pending-refund table at its cap,
//	    and a fetcher with no account on this ledger (R-REFUND-NEEDS-AN-ACCOUNT).
//	C10 a hard live-session cap (deliveryMaxLiveSessions): refuse at cap, never evict.
//
// The v2 flat path (MsgDeliveryReceipt: token spent at REDEEM) is RETIRED (B-9): the
// kind is refused with a named reason (demandrole.go handleDeliveryReceipt).
//
// M0 (cert §7): the session record adds no who-fetched-what capability the shipped
// receipt does not already hand the server (Receipt.Fetcher is the durable key in the
// clear); the idle stamp is stored COARSE (deliveryStampGranularity) so it is no finer
// an access record than the reaper needs; the close log line carries no identity and
// no object (the relay lane's audit rule). Serving stays FREE: nothing here gates
// MsgFetchChunk (cert §3.2).

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"errors"
	"fmt"

	"github.com/nerolabs/silt/core/credit"
	"github.com/nerolabs/silt/core/demand"
	"github.com/nerolabs/silt/ports"
)

type deliveryError string

func (e deliveryError) Error() string { return string(e) }

const (
	errDeliveryAcceptDisabled   = deliveryError("delivery: paid sessions not accepted (the delivery-receipt lane is off)")
	errDeliveryIdleUnset        = deliveryError("delivery: idle window unset — refuse-until-set (a liveness choice: how long an idle session holds one of the node's session slots and how long the fetcher's deposit stays locked past its anchor's expiry; no forfeiture — the remainder is a deposit)")
	errDeliverySessionCap       = deliveryError("delivery: live session table at capacity (per-node cap; refuse, never evict)")
	errDeliverySessionExists    = deliveryError("delivery: this fetcher already holds a live session here — fund it (MsgDeliveryFund) instead of opening another (one session per fetcher)")
	errDeliveryNoAnchor         = deliveryError("delivery: session open carries no anchor (an unanchored session funds nothing)")
	errDeliveryTooManyAnchors   = deliveryError("delivery: more anchors than MaxAnchorsPerOpen")
	errDeliveryAnchorMalformed  = deliveryError("delivery: anchor serial or signature is malformed")
	errDeliveryFetcherMismatch  = deliveryError("delivery: sha256(Fetcher) != authenticated sender — the commitment is not the sender's")
	errDeliverySigInvalid       = deliveryError("delivery: commitment signature invalid")
	errDeliveryNoIssuerKey      = deliveryError("delivery: no self demand-issuer keyset (no chain commitment for key_E) — the anchor lane is dark until this node commits one (era-4 is NOT the gate: -era4-activation-height defaults to 1, so v5 is live from height 1; what is missing is the committed IssuerKeyReg, which needs an objective, bonded, epoch-enabled validator)")
	errDeliveryAnchorInvalid    = deliveryError("delivery: anchor does not verify under this server's committed key in the DEMAND domain (wrong server, wrong lane, or expired)")
	errDeliveryAnchorSpent      = deliveryError("delivery: anchor already spent on this ledger")
	errDeliveryGuardFull        = deliveryError("delivery: paid-serial guard full of live entries — refused, never evicted")
	errDeliveryGuardRefused     = deliveryError("delivery: paid-serial guard refused the spend")
	errDeliveryNoLedger         = deliveryError("delivery: no ledger wired — no paying ledger to spend anchors on")
	errDeliveryNoSession        = deliveryError("delivery: no live session for that handle (closed, reaped, or never opened) — open one")
	errDeliveryNotOwner         = deliveryError("delivery: the session belongs to another fetcher")
	errDeliveryCommitment       = deliveryError("delivery: receipt commitment is not this session's open commitment")
	errDeliveryWrongServer      = deliveryError("delivery: receipt names another server")
	errDeliveryCountAboveBudget = deliveryError("delivery: cumulative count exceeds what the session's budget can fund — top up first")
	errDeliverySettleRefused    = deliveryError("delivery: the ledger refused the settlement")
	errDeliveryNoSigner         = deliveryError("delivery: this node has no signer to commit a session with")
)

// deliveryMaxLiveSessions is the hard per-node ceiling on concurrent live delivery
// sessions (C10; the relayMaxLiveSessions shape and value). Past it a new open is
// refused; idle expiry, not the cap, reclaims.
const deliveryMaxLiveSessions = 4096

// deliveryStampDivisor sets the idle stamp's granularity: idle/4. The reaper needs
// no finer a resolution, and a fine last-activity timestamp per durable identity would
// be a finer access record than anything shipped (cert §7, GATED: coarse).
const deliveryStampDivisor = 4

// DeliverySession is one live paid delivery session. It holds exactly the fields a
// decided function reads (cert §7 T-DONT3): the budget and counters for conservation,
// the stamp for the reaper, the commitment for receipt binding. NO object-keyed
// collection (gate G-λ-8-7 asserts this by reflection).
type DeliverySession struct {
	handle     uint64
	fetcher    ports.NodeID // the durable fetcher (the authenticated peer)
	fetcherPub []byte       // its ed25519 key, as presented at open
	commitment []byte       // M of the open — what every receipt must carry
	budget     int64        // Σ face spent in (open + top-ups)
	settled    int64        // credits settled so far, gross; monotone; ≤ budget
	count      uint64       // cumulative acknowledged increments; monotone
	lastSettle ports.Time   // COARSE stamp of the last settlement (or the open)
	// maxAnchorEpoch is the newest issue epoch among the session's anchors (open + funds):
	// the unsettled remainder is a deposit released when that anchor leaves the guard
	// window (M2, D-R2.9-NODE-HALF-CALLS 1′), so the epoch rides the session, never a new
	// dimension on the guard entry (G-6R-8).
	maxAnchorEpoch uint64
}

// Budget / Settled / Count are read-only views for tests and observability.
func (s *DeliverySession) Budget() int64  { return s.budget }
func (s *DeliverySession) Settled() int64 { return s.settled }
func (s *DeliverySession) Count() uint64  { return s.count }

// deliveryAnchorSpender / deliverySettler / deliveryCloser are the optional halves of
// ports.CreditLedger the R2.9 lane needs (core/credit implements them). Optional so the
// port stays narrow and test doubles need not carry them; a ledger without them
// refuses every open with a named reason.
type deliveryAnchorSpender interface {
	SpendDeliveryAnchors(server ports.NodeID, anchors []ports.RelayAnchor) (face int64, reason string)
}
type deliverySettler interface {
	SettleDelivery(server, fetcher ports.NodeID, root ports.Hash, count, budget, prior int64) (settled, paid int64, reason string)
}
type deliveryCloser interface {
	CloseDeliverySession(fetcher ports.NodeID, remaining int64, maxAnchorEpoch uint64) int64
}
type refundReleaser interface{ ReleaseDueRefunds() }

// EnableDeliverySessions opts this node into paid delivery sessions with the given
// idle window (C9). A non-positive window is REFUSED at the daemon (refuse-until-set);
// here it leaves the lane off so a misuse cannot admit a session the reaper would
// never close. The reaper is LAZY — it sweeps on every open, fund and settle (the
// relay lane's sweepRelaySeen shape, and the cap check sweeps first) — plus
// SweepDeliverySessions for a periodic caller (the daemon), so a silent server still
// closes idle sessions. No self-re-arming timer: one would keep the deterministic sim
// from ever reaching quiescence (the demandTick rule).
func (n *Node) EnableDeliverySessions(idle ports.Duration) {
	if idle <= 0 {
		return
	}
	n.deliveryAccept = true
	n.deliveryIdle = idle
	n.deliverySessions = make(map[uint64]*DeliverySession)
	n.deliveryByFetcher = make(map[ports.NodeID]uint64)
}

// DeliveryIdleWindow reports the idle window the REAPER is actually running on — the
// value EnableDeliverySessions installed, read back out of the node. It exists so a
// caller that has to state the window (a boot banner, a status surface) states what is
// installed rather than re-reading whatever input it thinks was used: the daemon's flag
// check and the daemon's install are two reads of one variable, and only the check is
// gated, so the announcement is what ties the second read to the first (G-C2-18).
// Zero when the lane is off.
func (n *Node) DeliveryIdleWindow() ports.Duration { return n.deliveryIdle }

// SweepDeliverySessions closes every session idle for at least the window, on the
// node's clock, for a periodic caller. Loop-only (touches the session table).
func (n *Node) SweepDeliverySessions() {
	n.sweepDeliverySessions(n.clock.Now())
	if r, ok := n.ledger.(refundReleaser); ok && n.ledger != nil {
		r.ReleaseDueRefunds() // deposits whose anchors left the window return even on a silent server
	}
}

// DisableDeliverySessions closes every live session (each remainder accounted once)
// and turns the lane off. For tests and shutdown.
func (n *Node) DisableDeliverySessions() {
	for h := range n.deliverySessions {
		n.closeDeliverySession(h, "disabled")
	}
	n.deliveryAccept = false
}

// DeliverySessionForTest exposes a live session by handle. Not part of the wire path.
func (n *Node) DeliverySessionForTest(handle uint64) (*DeliverySession, bool) {
	s, ok := n.deliverySessions[handle]
	return s, ok
}

// LiveDeliverySessions is the observability count of live sessions.
func (n *Node) LiveDeliverySessions() int { return len(n.deliverySessions) }

func (n *Node) deliveryStamp(now ports.Time) ports.Time {
	g := ports.Time(n.deliveryIdle / deliveryStampDivisor)
	if g <= 0 {
		return now
	}
	return now - now%g
}

// sweepDeliverySessions closes every session idle for at least the window, measured
// from its last settlement on the node's clock (C9; cert §6.2 point 1: idleness, not
// the admit epoch — a session settling every tick lives; a silent one is reaped).
func (n *Node) sweepDeliverySessions(now ports.Time) {
	if !n.deliveryAccept {
		return
	}
	for h, s := range n.deliverySessions {
		if now-s.lastSettle >= ports.Time(n.deliveryIdle) {
			n.closeDeliverySession(h, "idle")
		}
	}
}

// closeDeliverySession removes the session FIRST (the SettleRelaySession delete-first
// ordering: nothing can settle or fund it after this) and then accounts the remainder
// ONCE through the ledger (G-λ-8-6), as a DEPOSIT released to the fetcher when the
// session's anchors leave the guard window (M1 + M2, D-R2.9-NODE-HALF-CALLS 1′). The
// log line carries per-session numbers and the reason only — no identity, no object
// (M0 audit).
func (n *Node) closeDeliverySession(handle uint64, why string) {
	s, ok := n.deliverySessions[handle]
	if !ok {
		return
	}
	delete(n.deliverySessions, handle)
	if cur, ok := n.deliveryByFetcher[s.fetcher]; ok && cur == handle {
		delete(n.deliveryByFetcher, s.fetcher)
	}
	remaining := s.budget - s.settled
	var pending int64
	if c, ok := n.ledger.(deliveryCloser); ok && n.ledger != nil {
		pending = c.CloseDeliverySession(s.fetcher, remaining, s.maxAnchorEpoch)
	}
	n.logf(ports.LogInfo, "delivery session closed", "reason", why, "increments", s.count, "settled", s.settled, "remainder", remaining, "deposit", pending)
}

// verifyDeliveryAnchors verifies each anchor under this node's OWN keyset in the
// DEMAND domain (B-7: a relay-domain credential fails here, before any ledger call)
// and returns the (epoch, serial) pairs the ledger guards.
func (n *Node) verifyDeliveryAnchors(anchors []demand.Token) ([]ports.RelayAnchor, error) {
	if len(anchors) == 0 {
		return nil, errDeliveryNoAnchor
	}
	if len(anchors) > demand.MaxAnchorsPerOpen {
		return nil, errDeliveryTooManyAnchors
	}
	for _, a := range anchors {
		if len(a.Serial) != 32 || len(a.Sig) == 0 {
			return nil, errDeliveryAnchorMalformed
		}
	}
	if n.ledger == nil {
		return nil, errDeliveryNoLedger
	}
	ks := n.DemandIssuerKeyset(n.id)
	if ks == nil {
		return nil, errDeliveryNoIssuerKey
	}
	cur := n.chainEpoch() // the epoch the keyset was pruned with; the ledger reads the same clock (R2.10 / F8)
	spend := make([]ports.RelayAnchor, 0, len(anchors))
	for _, a := range anchors {
		e, ok := ks.VerifyInWindow(cur, a)
		if !ok {
			return nil, errDeliveryAnchorInvalid // ≤ W+1 modexps for a garbage open (T-7)
		}
		spend = append(spend, ports.RelayAnchor{Epoch: e, Serial: a.Serial})
	}
	return spend, nil
}

func (n *Node) spendDeliveryAnchors(spend []ports.RelayAnchor) (int64, error) {
	sp, ok := n.ledger.(deliveryAnchorSpender)
	if !ok {
		return 0, errDeliveryNoLedger
	}
	face, reason := sp.SpendDeliveryAnchors(n.id, spend)
	if face <= 0 {
		switch reason {
		case credit.ReasonAlreadyPaid:
			return 0, errDeliveryAnchorSpent
		case credit.ReasonGuardFull:
			return 0, errDeliveryGuardFull
		default:
			return 0, fmt.Errorf("%w: %s", errDeliveryGuardRefused, reason)
		}
	}
	return face, nil
}

// OpenDeliverySession is the server side of MsgDeliveryOpen (C1–C3, C10). from is the
// authenticated durable fetcher.
func (n *Node) OpenDeliverySession(from ports.NodeID, open demand.SessionOpen) (*DeliverySession, error) {
	if !n.deliveryAccept {
		return nil, errDeliveryAcceptDisabled
	}
	now := n.clock.Now()
	n.sweepDeliverySessions(now) // reclaim idle sessions before the cap is judged
	if len(n.deliverySessions) >= deliveryMaxLiveSessions {
		return nil, errDeliverySessionCap
	}
	if _, live := n.deliveryByFetcher[from]; live {
		return nil, errDeliverySessionExists
	}
	if len(open.Fetcher) != ed25519.PublicKeySize || sha256.Sum256(open.Fetcher) != from {
		return nil, errDeliveryFetcherMismatch
	}
	if !ed25519.Verify(ed25519.PublicKey(open.Fetcher), demand.SessionOpenCommitment(n.id, open.Anchors), open.Sig) {
		return nil, errDeliverySigInvalid
	}
	spend, err := n.verifyDeliveryAnchors(open.Anchors)
	if err != nil {
		return nil, err
	}
	face, err := n.spendDeliveryAnchors(spend) // all-or-nothing, durable before admission (G-λ-8-9)
	if err != nil {
		return nil, err
	}
	n.deliverySessionSeq++
	s := &DeliverySession{handle: n.deliverySessionSeq, fetcher: from, fetcherPub: append([]byte(nil), open.Fetcher...),
		commitment: demand.SessionOpenCommitment(n.id, open.Anchors), budget: face, lastSettle: n.deliveryStamp(now),
		maxAnchorEpoch: maxEpoch(spend)}
	n.deliverySessions[s.handle] = s
	n.deliveryByFetcher[from] = s.handle
	return s, nil
}

// FundDeliverySession is the server side of MsgDeliveryFund (C8).
func (n *Node) FundDeliverySession(from ports.NodeID, fund demand.SessionFund) (*DeliverySession, error) {
	if !n.deliveryAccept {
		return nil, errDeliveryAcceptDisabled
	}
	n.sweepDeliverySessions(n.clock.Now())
	s, ok := n.deliverySessions[fund.Handle]
	if !ok {
		return nil, errDeliveryNoSession
	}
	if s.fetcher != from || !bytes.Equal(s.fetcherPub, fund.Fetcher) {
		return nil, errDeliveryNotOwner
	}
	if !ed25519.Verify(ed25519.PublicKey(fund.Fetcher), demand.SessionFundCommitment(n.id, fund.Handle, fund.Anchors), fund.Sig) {
		return nil, errDeliverySigInvalid
	}
	spend, err := n.verifyDeliveryAnchors(fund.Anchors)
	if err != nil {
		return nil, err
	}
	face, err := n.spendDeliveryAnchors(spend)
	if err != nil {
		return nil, err
	}
	s.budget += face
	if e := maxEpoch(spend); e > s.maxAnchorEpoch {
		s.maxAnchorEpoch = e
	}
	return s, nil
}

// maxEpoch is the newest issue epoch in a verified anchor batch.
func maxEpoch(spend []ports.RelayAnchor) uint64 {
	var m uint64
	for _, a := range spend {
		if a.Epoch > m {
			m = a.Epoch
		}
	}
	return m
}

// SettleDeliveryReceipt is the server side of MsgDeliverySettle (C4–C7). It returns
// the GROSS credits this receipt settled (0 for a non-advancing count) or a named
// refusal. A receipt that advances the count past what the budget funds is refused
// whole (the fetcher tops up first); a receipt naming no live session is refused
// (B-9's anchored half: the unanchored receipt funds nothing).
func (n *Node) SettleDeliveryReceipt(from ports.NodeID, r demand.SessionReceipt) (int64, error) {
	if !n.deliveryAccept {
		return 0, errDeliveryAcceptDisabled
	}
	n.sweepDeliverySessions(n.clock.Now())
	s, ok := n.deliverySessions[r.Handle]
	if !ok {
		return 0, errDeliveryNoSession
	}
	if s.fetcher != from || !bytes.Equal(s.fetcherPub, r.Fetcher) {
		return 0, errDeliveryNotOwner
	}
	if r.Server != n.id {
		return 0, errDeliveryWrongServer
	}
	if !bytes.Equal(r.Commitment, s.commitment) {
		return 0, errDeliveryCommitment
	}
	if !r.VerifySig() {
		return 0, errDeliverySigInvalid
	}
	if r.Count <= s.count {
		return 0, nil // a re-presented or lower count pays 0 by arithmetic (C5)
	}
	// The ceiling DERIVED from the budget (C3): count·p ≤ budget, in whole increments.
	if r.Count > uint64(s.budget/credit.DeliveryIncrementCredit) {
		return 0, errDeliveryCountAboveBudget
	}
	st, ok := n.ledger.(deliverySettler)
	if !ok {
		return 0, errDeliveryNoLedger
	}
	delta := int64(r.Count - s.count)
	settled, paid, why := st.SettleDelivery(n.id, from, r.Object, delta, s.budget-s.settled, s.settled) // prior = the session's cumulative settled: the skim floors on the SESSION (G-SKIM)
	if why != credit.ReasonPaid {
		return 0, fmt.Errorf("%w: %s", errDeliverySettleRefused, why)
	}
	s.settled += settled
	s.count = r.Count
	s.lastSettle = n.deliveryStamp(n.clock.Now())
	// The witnessed-demand observable, denominated in the LEDGER's settled increments
	// (certified 2026-09-06, demand.Bank.Witness rule 1–2): never the receipt's count,
	// never the delta; bumped AFTER ReasonPaid and never gating the settlement (rule 3).
	// Balance only, never standing.
	var witnessed bool
	if n.demandBank != nil {
		witnessed, _ = n.demandBank.Witness(r.Object, r.Fetcher, settled/credit.DeliveryIncrementCredit)
	}
	// "delivery receipt banked" is an announced S5 marker (observable_contract.go). Its
	// denomination on this lane is INCREMENTS (256 KiB each), not tokens.
	n.logf(ports.LogInfo, "delivery receipt banked", "object", r.Object, "increments", n.WitnessedIncrements(r.Object),
		"credit", paid, "settled", settled, "witnessed", witnessed)
	if s.settled >= s.budget {
		n.closeDeliverySession(s.handle, "exhausted") // a fully consumed face: nothing to burn or refund
	}
	return settled, nil
}

// WitnessedIncrements is the v3 witnessed-demand surface for object: increments of
// DeliveryIncrementBytes settled on session receipts. 0 when demand banking is off.
// Observability only, never standing.
func (n *Node) WitnessedIncrements(object ports.Hash) int64 {
	if n.demandBank == nil {
		return 0
	}
	return n.demandBank.WitnessedIncrements(object)
}

// DistinctBondedFetchers is the P3b surface for object: how many bond-distinct fetchers
// have been credited on it (either lane). 0 when demand banking is off or the credential
// is not required. Observability only, never standing.
func (n *Node) DistinctBondedFetchers(object ports.Hash) int64 {
	if n.demandBank == nil {
		return 0
	}
	return n.demandBank.DistinctBondedFetchers(object)
}

// ---- the wire handlers (server side)

func (n *Node) handleDeliveryOpen(from ports.NodeID, msg ports.Message) {
	deny := func(err error) {
		n.reply(from, msg, ports.Message{Kind: ports.MsgDeliveryOpenAck, OK: false, Data: []byte(err.Error())})
	}
	open, err := demand.UnmarshalSessionOpen(msg.Data)
	if err != nil {
		deny(fmt.Errorf("delivery: malformed SessionOpen: %w", err))
		return
	}
	s, err := n.OpenDeliverySession(from, open)
	if err != nil {
		n.logDeliveryAdmissionRefusal("open", err)
		deny(err) // named, never silent
		return
	}
	n.reply(from, msg, ports.Message{Kind: ports.MsgDeliveryOpenAck, OK: true, Height: s.handle})
}

func (n *Node) handleDeliveryFund(from ports.NodeID, msg ports.Message) {
	deny := func(err error) {
		n.reply(from, msg, ports.Message{Kind: ports.MsgDeliveryFundAck, OK: false, Data: []byte(err.Error())})
	}
	fund, err := demand.UnmarshalSessionFund(msg.Data)
	if err != nil {
		deny(fmt.Errorf("delivery: malformed SessionFund: %w", err))
		return
	}
	s, err := n.FundDeliverySession(from, fund)
	if err != nil {
		n.logDeliveryAdmissionRefusal("fund", err)
		deny(err)
		return
	}
	n.reply(from, msg, ports.Message{Kind: ports.MsgDeliveryFundAck, OK: true, Height: uint64(s.budget)})
}

func (n *Node) handleDeliverySettle(from ports.NodeID, msg ports.Message) {
	deny := func(err error) {
		n.reply(from, msg, ports.Message{Kind: ports.MsgDeliverySettleAck, OK: false, Data: []byte(err.Error())})
	}
	r, err := demand.UnmarshalSessionReceipt(msg.Data)
	if err != nil {
		deny(fmt.Errorf("delivery: malformed SessionReceipt: %w", err))
		return
	}
	settled, err := n.SettleDeliveryReceipt(from, r)
	if err != nil {
		if deliveryPostAuth(err) {
			// "delivery receipt paid NO credit" is the announced S5 marker (observable_contract.go)
			// — the one signal an operator gets when an AUTHENTICATED receipt on a live session
			// settles nothing. Post-auth only: the owner, commitment and signature checks passed.
			n.logf(ports.LogWarn, "delivery receipt paid NO credit", "object", r.Object, "reason", err.Error(),
				"serial_guard_refusals", guardFullRefusals(n.ledger))
		} else {
			// Pre-auth refusals (no session, not the owner, bad signature, lane off) are one
			// unauthenticated message per line with no rate limit on logf: Debug, never WARN
			// (blind PE, 2026-09-07).
			n.logf(ports.LogDebug, "delivery settle refused", "reason", err.Error())
		}
		deny(err)
		return
	}
	n.reply(from, msg, ports.Message{Kind: ports.MsgDeliverySettleAck, OK: true, Height: uint64(settled)})
}

// deliveryPostAuth reports whether a settle refusal arose AFTER the session's owner,
// commitment and signature checks passed — the refusals worth an operator's WARN. The
// pre-auth classes are reachable by any peer and stay at Debug.
func deliveryPostAuth(err error) bool {
	return errors.Is(err, errDeliveryCountAboveBudget) || errors.Is(err, errDeliverySettleRefused) ||
		errors.Is(err, errDeliveryNoLedger)
}

// logDeliveryAdmissionRefusal is the operator signal for a refused open or fund. The
// paid-serial guard filling with LIVE entries — a serve rate above the bound the cap was
// derived against — arises ONLY here (spendDeliveryAnchors is called from open and fund and
// nowhere else; Ledger.SettleDelivery has no guard-full path), so the announced
// "delivery anchor refused: guard full" marker (observable_contract.go) is emitted here at
// WARN with the ledger's monotone counter. Every other admission refusal is reachable by
// an unauthenticated peer at one message per line and stays at Debug.
func (n *Node) logDeliveryAdmissionRefusal(step string, err error) {
	if errors.Is(err, errDeliveryGuardFull) {
		n.logf(ports.LogWarn, "delivery anchor refused: guard full", "step", step,
			"serial_guard_refusals", guardFullRefusals(n.ledger))
		return
	}
	n.logf(ports.LogDebug, "delivery admission refused", "step", step, "reason", err.Error())
}

// ---- the fetcher side

// OpenDeliverySessionRemote opens a paid delivery session at server with anchors this
// node's DURABLE identity bought there (AcquireDemandTokenInWindow — the demand token
// IS the anchor). done reports the handle and the open commitment M the receipts must
// carry, or the server's named refusal.
func (n *Node) OpenDeliverySessionRemote(server ports.NodeID, anchors []demand.Token, done func(handle uint64, commitment []byte, err error)) {
	if n.signer == nil {
		done(0, nil, errDeliveryNoSigner)
		return
	}
	open := demand.SignSessionOpen(n.signer, server, anchors)
	blob, err := open.Marshal()
	if err != nil {
		done(0, nil, err)
		return
	}
	commitment := demand.SessionOpenCommitment(server, anchors)
	n.request(server, ports.Message{Kind: ports.MsgDeliveryOpen, Data: blob}, func(resp ports.Message, rerr error) {
		if rerr != nil {
			done(0, nil, rerr)
			return
		}
		if !resp.OK {
			done(0, nil, deliveryError("server refused session open: "+string(resp.Data)))
			return
		}
		done(resp.Height, commitment, nil)
	})
}

// FundDeliverySessionRemote tops up an admitted session with fresh anchors. done
// reports the session's budget after the top-up.
func (n *Node) FundDeliverySessionRemote(server ports.NodeID, handle uint64, anchors []demand.Token, done func(budget int64, err error)) {
	if n.signer == nil {
		done(0, errDeliveryNoSigner)
		return
	}
	fund := demand.SignSessionFund(n.signer, server, handle, anchors)
	blob, err := fund.Marshal()
	if err != nil {
		done(0, err)
		return
	}
	n.request(server, ports.Message{Kind: ports.MsgDeliveryFund, Data: blob}, func(resp ports.Message, rerr error) {
		if rerr != nil {
			done(0, rerr)
			return
		}
		if !resp.OK {
			done(0, deliveryError("server refused top-up: "+string(resp.Data)))
			return
		}
		done(int64(resp.Height), nil)
	})
}

// SubmitDeliverySettle is the fetcher side of MsgDeliverySettle: having content-
// verified object's bytes from server, sign the CUMULATIVE count on the session and
// submit it. done reports the gross credits the server settled for this receipt.
func (n *Node) SubmitDeliverySettle(server ports.NodeID, handle uint64, commitment []byte, object ports.Hash, count uint64, done func(settled int64, err error)) {
	if n.signer == nil {
		done(0, ErrNoSigner)
		return
	}
	r := demand.AckSession(n.signer, handle, commitment, object, server, count)
	blob, err := r.Marshal()
	if err != nil {
		done(0, err)
		return
	}
	n.request(server, ports.Message{Kind: ports.MsgDeliverySettle, Data: blob}, func(resp ports.Message, rerr error) {
		if rerr != nil {
			done(0, rerr)
			return
		}
		if !resp.OK {
			done(0, deliveryError("server refused the receipt: "+string(resp.Data)))
			return
		}
		done(int64(resp.Height), nil)
	})
}
