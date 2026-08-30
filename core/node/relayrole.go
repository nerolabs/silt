package node

// PoD relay lane — the relay-accept role (docs/design/pod.md §7.3, certified
// 2026-08-30). A relay/gateway forwards content-blind bytes toward a fetcher and
// is paid as-it-goes by a sender-funded PayWord hash chain (core/relaypay). It
// cannot sign a completed-delivery receipt because it never holds a verifiable
// object, so there is no delivery-receipt lane here — the fetcher commits a
// chain root once, reveals one preimage per forwarded increment, and the relay
// redeems the highest preimage it holds at epoch net-settlement
// (credit.RedeemRelayCredit, the balance-only conserved transfer).
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

// EnableRelayAccept opts this node into accepting sender-funded PayWord chains.
// Off by default; the mirror of EnableDemandBank for the delivery lane. The
// daemon wires it behind a flag (cmd/silt/daemon.go).
func (n *Node) EnableRelayAccept() {
	n.relayAccept = true
	n.relaySeenEph = make(map[ports.NodeID]uint64)
	n.relaySeenRoot = make(map[string]uint64)
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

// sweepRelaySeen evicts seen-map entries older than the retention window, keyed on
// the current epoch, with a MONOTONIC eviction floor (#645). The floor never
// lowers: a reorg that moves the epoch backward (epochStart is reorg-swapped) must
// not un-evict a swept entry, or a previously-seen root could be re-admitted — a
// guard-(ii) regression. So the floor is max(previousFloor, epoch - retention).
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
}

// RelaySession is one live relay-payment session: the relay-side PayWord
// verifier keyed on the committed chain root. It holds exactly one preimage
// (32 B) regardless of chain length.
type RelaySession struct {
	verifier *relaypay.Verifier
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
	return &RelaySession{verifier: relaypay.NewVerifier(root, S)}, nil
}

// Pay authorizes the next increment by verifying a revealed preimage — one
// SHA-256 (relaypay.Verifier.Advance). If the fetcher stops revealing, the relay
// stops forwarding; the irreducible one-increment stiff is bounded small by the
// increment size (relaypay.RelayIncrementBytes, the owed floor-box measurement).
func (s *RelaySession) Pay(preimage []byte) error {
	return s.verifier.Advance(preimage)
}

// Count returns the number of increments the relay is authorized to redeem for
// this session (the settled amount is Count × RelayIncrementBytes' credit value,
// drawn from the fetcher's paid-in blind credit at settlement via
// credit.RedeemRelayCredit).
func (s *RelaySession) Count() int { return s.verifier.Count() }
