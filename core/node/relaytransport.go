package node

// PoD §7.3 transport Batch 2 — the wire handlers + settle-at-close (step 3,
// design §1). Three request/reply message kinds mirror the delivery-receipt lane
// (handleDeliveryReceipt): MsgRelayOpen (open a paid session), MsgRelayPay (a
// preimage reveal), and their acks. Settlement is LOCAL at close — no wire message
// (design §1, §5): the relay redeems its highest held preimage into its operator
// balance via credit.RedeemRelayCredit.
//
// The two M0 guards and the #644 S-clamp fire on the LIVE path because
// handleRelayOpen routes through OpenRelaySession — the SAME tested entry the
// Batch-1 guards live in. The wire path does not bypass them (design §6; the
// live-path guard-reuse test asserts it).

import (
	"time"

	"github.com/nerolabs/silt/core/relaypay"
	"github.com/nerolabs/silt/ports"
)

// resolveRelayTimeout bounds how long the off-loop resolver blocks on the loop
// reply before it DEGRADES TO REFUSE (PoD §7.3 Batch 3, PE LOW follow-up
// RULING-pod-7.3-transport-batch3-daemon-binding-2026-08-30, finding 1). The loop
// replies in microseconds normally; this is a generous ceiling that only fires if
// the loop has stopped or stalled (e.g. a future graceful-shutdown calling
// loop.Stop() while the relay Server still accepts). On timeout the resolver
// returns ok=false, so the Server refuses the paid connect — never hangs, never
// downgrades to free (certified residual #2). This removes the reliance on the
// un-asserted invariant "the production loop never stops".
//
// It is a var (not a const) so the seam test can shorten it; production never
// reassigns it.
var resolveRelayTimeout = 5 * time.Second

// handleRelayOpen is the relay side of MsgRelayOpen: decode the fetcher's chain
// commitment, run it through OpenRelaySession (M0 guards + #644 clamp), and on
// success store the live session under a fresh handle returned in the ack's Height.
// A refusal is an OK=false ack carrying the reason in Data — never a silent drop
// (the delivery lane's deny() shape).
func (n *Node) handleRelayOpen(from ports.NodeID, msg ports.Message) {
	deny := func(reason string) {
		n.reply(from, msg, ports.Message{Kind: ports.MsgRelayOpenAck, OK: false, Data: []byte(reason)})
	}
	if !n.relayAccept {
		deny(errRelayAcceptDisabled.Error())
		return
	}
	open, err := relaypay.UnmarshalRelayOpen(msg.Data)
	if err != nil {
		deny("relay: malformed RelayOpen")
		return
	}
	// The fetcher's ephemeral identity IS the authenticated sender (from): the blind
	// withdrawal produced a fresh ephemeral keypair, and the transport authenticates
	// the sender by that key. Binding the session to `from` (not a value inside the
	// payload) is what makes guard (ii)'s per-session-identity check bind the party
	// that actually opened the conn.
	funding := FundingSource(open.Funding)
	sess, err := n.OpenRelaySession(from, open.Root, open.S, funding)
	if err != nil {
		deny(err.Error()) // M0 guard / S-clamp refusal — surfaced, never silent
		return
	}
	n.relaySessionSeq++
	handle := n.relaySessionSeq
	n.relaySessions[handle] = sess
	n.reply(from, msg, ports.Message{Kind: ports.MsgRelayOpenAck, OK: true, Height: handle})
}

// handleRelayPay is the relay side of MsgRelayPay: look up the live session by
// handle, verify the revealed preimage (the carried-S Verifier, bounded to at most
// S hashes — the #644 clamp), and on success raise the paid pump's byte ceiling.
// The ack's Height carries the authorized increment count so the fetcher can
// confirm progress. A session that does not exist or a preimage that does not
// verify is an OK=false ack.
func (n *Node) handleRelayPay(from ports.NodeID, msg ports.Message) {
	deny := func() { n.reply(from, msg, ports.Message{Kind: ports.MsgRelayPayAck, OK: false}) }
	pay, err := relaypay.UnmarshalRelayPay(msg.Data)
	if err != nil {
		deny()
		return
	}
	sess, ok := n.relaySessions[pay.Handle]
	if !ok {
		deny()
		return
	}
	// A pay may only advance the session opened by the SAME ephemeral identity: the
	// authenticated sender must be the session's fetcher. This stops a third party
	// from driving another fetcher's session (a payment-channel hijack).
	if sess.ephID != from {
		deny()
		return
	}
	if err := sess.PayTo(pay.Preimage, pay.Count); err != nil {
		deny()
		return
	}
	n.reply(from, msg, ports.Message{Kind: ports.MsgRelayPayAck, OK: true, Height: uint64(sess.Count())})
}

// SettleRelaySession settles the paid session at close and removes it from the
// table (design §5: single settlement at close). It redeems the highest held
// preimage ONCE — count × RelayIncrementCredit credit — capped at the committed
// budget (S × increment), itself bounded by the fetcher's paid-in blind credit.
// The verifier's monotonic count is the accumulator, so no per-call summation can
// exceed the budget. Returns the credit paid to the relay (0 if the session is
// unknown or the ledger is unset). The M0 residual (design §6): the settlement log
// line carries NO durable or cross-session-stable field — only the count and the
// paid credit, both per-session values.
func (n *Node) SettleRelaySession(handle uint64) int64 {
	sess, ok := n.relaySessions[handle]
	if !ok {
		return 0
	}
	delete(n.relaySessions, handle)
	sess.closeSession() // wake the pump so it drains and stops
	if n.ledger == nil {
		return 0
	}
	chainValue := int64(sess.Count()) * relaypay.RelayIncrementCredit
	paid := n.ledger.RedeemRelayCredit(n.id, sess.ephID, chainValue, sess.budget)
	// M0 audit (design §6, session-6 observable-log-contract discipline): log only
	// per-session, non-durable values. No ephemeral or durable identity, no chain
	// root — a relay operator's log must not carry a cross-session-stable field the
	// settlement could be correlated on.
	n.logf(ports.LogInfo, "relay session settled", "increments", sess.Count(), "credit", paid)
	return paid
}

// RelaySessionForTest exposes a live session by handle for tests that drive the
// paid pump against the node-owned authorizer. Not part of the wire path.
func (n *Node) RelaySessionForTest(handle uint64) (*RelaySession, bool) {
	s, ok := n.relaySessions[handle]
	return s, ok
}

// ResolveRelayAuthorizer is the node's half of the adapter/node seam (PoD §7.3
// Batch 3, design §3). The relay Server calls it from its accept goroutine to resolve
// a paid connect's handle to the node-owned authorizer, checking that `fetcher` (the
// authenticated connector) OWNS the handle — the SAME ephID-ownership check
// handleRelayPay enforces (relaytransport.go: sess.ephID != from → deny). It returns
// ok=false when no live session exists for the handle or the fetcher does not own it;
// the Server then REFUSES the connect, never downgrading to free (certified residual
// #2).
//
// CONCURRENCY SEAM (design §3, flagged): relaySessions is touched only from the
// serialized event-loop path (node.go). This method is called OFF that loop (the
// Server's accept goroutine), so it MARSHALS the lookup onto the loop via
// clock.AfterFunc(0, …) and blocks on a reply channel. This keeps the single-threaded
// invariant on the session table — no new mutex on the map, and -race clean by
// construction, because the map is still only ever read/written on the loop. The
// caller (accept goroutine) is never the loop goroutine, so blocking here cannot
// deadlock the loop.
//
// The reply read is TIMEOUT-BOUNDED (resolveRelayTimeout): if the loop has stopped
// or stalled and the reply never arrives, the resolver returns ok=false and the
// Server refuses the connect, rather than leaking a blocked goroutine per paid
// connect. This makes the seam self-safe under a future graceful-shutdown that
// calls loop.Stop() while the relay Server still accepts (PE finding 1). The reply
// channel is cap-1, so the loop's send never blocks even after the resolver has
// given up and returned.
func (n *Node) ResolveRelayAuthorizer(fetcher ports.NodeID, handle uint64) (*RelaySession, bool) {
	type result struct {
		sess *RelaySession
		ok   bool
	}
	reply := make(chan result, 1)
	n.clock.AfterFunc(0, func() {
		sess, ok := n.relaySessions[handle]
		if !ok || sess.ephID != fetcher {
			reply <- result{nil, false}
			return
		}
		reply <- result{sess, true}
	})
	select {
	case r := <-reply:
		return r.sess, r.ok
	case <-time.After(resolveRelayTimeout):
		// The loop did not reply within the ceiling: stopped or stalled. Degrade
		// to REFUSE (never hang, never downgrade to free).
		return nil, false
	}
}

// SettleRelaySessionForHandle is the node's settle-at-close driver for the live paid
// path (design §3). The relay Server calls it from the paid pump's return (the
// PaidSettler seam) with the handle and the forwarded byte count. It MARSHALS the
// settlement onto the event loop (same concurrency seam as the resolver): SettleRelay
// Session touches relaySessions and the ledger, both loop-only. The forwarded count is
// informational — the verifier's monotonic count is the settlement basis, so the count
// argument is not trusted for the credit amount. Single-settle is preserved:
// SettleRelaySession deletes the handle on first call, so a reaper that already closed
// and dropped the handle makes this a harmless no-op (no double-settle, design §3b).
func (n *Node) SettleRelaySessionForHandle(handle uint64, forwarded int64) {
	n.clock.AfterFunc(0, func() { n.SettleRelaySession(handle) })
}

// OpenRelaySessionRemote is the fetcher side of MsgRelayOpen: it sends the chain
// commitment (root + S + funding) to relay and reports the session handle the relay
// returns (or the refusal reason). The fetcher funds the chain under a fresh
// ephemeral identity BEFORE calling this (client.WithdrawDemandTokenPrivately); this
// node's authenticated identity on the wire IS that ephemeral key.
func (n *Node) OpenRelaySessionRemote(relay ports.NodeID, root []byte, S int, funding FundingSource, done func(handle uint64, err error)) {
	open := relaypay.RelayOpen{Root: root, S: S, Funding: int(funding)}
	blob, err := open.Marshal()
	if err != nil {
		done(0, err)
		return
	}
	n.request(relay, ports.Message{Kind: ports.MsgRelayOpen, Data: blob}, func(resp ports.Message, rerr error) {
		if rerr != nil {
			done(0, rerr)
			return
		}
		if !resp.OK {
			done(0, relayError("relay refused session open: "+string(resp.Data)))
			return
		}
		done(resp.Height, nil) // Height carries the session handle
	})
}

// SubmitRelayPay is the fetcher side of MsgRelayPay: reveal x_count to authorize the
// relay to advance to increment count on the named session. done reports the count
// the relay confirmed authorized (or an error / a refusal). Mirrors
// SubmitDeliveryReceipt.
func (n *Node) SubmitRelayPay(relay ports.NodeID, handle uint64, preimage []byte, count int, done func(authorized int, err error)) {
	pay := relaypay.RelayPay{Handle: handle, Preimage: preimage, Count: count}
	blob, err := pay.Marshal()
	if err != nil {
		done(0, err)
		return
	}
	n.request(relay, ports.Message{Kind: ports.MsgRelayPay, Data: blob}, func(resp ports.Message, rerr error) {
		if rerr != nil {
			done(0, rerr)
			return
		}
		if !resp.OK {
			done(0, relayError("relay rejected the preimage reveal"))
			return
		}
		done(int(resp.Height), nil)
	})
}
