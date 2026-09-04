package credit

// R2.14 — the relay-lane prepayment anchor's LEDGER half (docs/design/pod.md
// §7.3.2 step 1; the construction is certified in
// silt-reviews/research/research-outcome/R2.14-relay-prepayment-anchor-CONSTRUCTION-RESEARCH-CERTIFICATION-2026-09-04.md
// §2 (conservation), §2.4 (the six doors), §5 (guard window == keyset window)).
//
// An anchor is a blind signature under the RELAY's own per-epoch demand key,
// bought by the fetcher's DURABLE identity through the ordinary withdrawal path
// (core/node answerDemandTokenRequest → tokenChargeFor → ChargePublish on THIS
// ledger — the burn is refusable and lands on the ledger that later settles). The
// node verifies the signature under its committed key; what reaches the ledger is
// the (issue epoch, serial) pair, and the ledger's whole job is to make that pair
// fund exactly one session:
//
//   - SPENT ONCE. The pair goes into the paidSerial guard — the SAME map, cap,
//     expiry sweep and durable store the delivery lane's R0.4b guard uses. Sharing
//     the map (rather than a third in-memory twin) is what makes "restart is not
//     an eviction" and "refuse at cap, never evict a live entry" hold here for
//     free (advisory §1.4; the creditSpent lesson, R2.13b). One consequence, in
//     the safe direction: a serial a fetcher chose to blind in BOTH domains (two
//     withdrawals, two fees, one 32-byte random serial — which no honest client
//     does) can fund one lane only. An under-pay, never a mint.
//   - ALL OR NOTHING. Every anchor in the batch is checked before any is
//     recorded, so a refused open records nothing and the fetcher does not lose
//     anchor 1 because anchor 2 was spent (cert §2.2, T-10). The one exception is
//     the accepted under-pay direction: a durable store that fails MID-append
//     leaves the earlier anchors recorded and the open refused — burned with no
//     session, never paid twice.
//   - EXPIRES WITH THE KEY. The guard is swept on the same window the demand
//     keyset prunes with (paidSerialWindow == demand.DefaultWindow), driven by the
//     same consensus epoch the node pruned its keyset with, so "the keyset still
//     verifies the anchor" and "the guard still remembers it" are one predicate
//     (cert §5, T-12; the TestGuardLifetimeMatchesDemandKeysetLifetime twin is
//     TestRelayAnchorGuardWindowMatchesKeysetWindow).
//   - FACE = FEE. The issuer signs opaque blinded bytes and cannot see the
//     domain, so it charged l.fee for this anchor exactly as for a demand token;
//     face is an identity with the burn, not a chosen value (cert §2.1). Fee
//     constancy over an anchor's W+1-epoch life is the unstated assumption of
//     BOTH lanes (cert F-3); inert on a per-node ledger, an FP-2 / R2.10
//     precondition.
//
// Conservation on this ledger, per session: issuance −k·fee, open 0, pay 0,
// settle +min(count, k·fee). Δ Σ_L = settled − Σ face ≤ 0, equality iff the
// session consumed its anchors exactly (cert C-1). Never standing: nothing here
// touches a field Reputation reads (invariant_a_test.go classifies
// SpendRelayAnchors neutral and presses it against an anchored session).

import "github.com/nerolabs/silt/ports"

// RelayAnchor is the (issue epoch, serial) pair the guard keys on. The type lives
// in ports so core/node can hand it across the CreditLedger port without importing
// this package.
type RelayAnchor = ports.RelayAnchor

// relayAnchorSerialSize is the one serial length an honest withdrawal produces
// (blindtoken.SerialSize). The serial is attacker-chosen bytes that become a map
// key, so its length is bounded HERE as well as at the wire decode (the F5
// amplifier shape; cert §8), and pinned to the exact width so the guard key is
// injective. Duplicated rather than imported: core/credit carries no production
// dependency on core/blindtoken.
const relayAnchorSerialSize = 32

// The relay-anchor refusal reasons (observability only, like the Reason* set in
// delivery.go — no accounting rule reads them). ReasonAlreadyPaid, ReasonGuardFull,
// ReasonGuardUnloaded, ReasonGuardStore, ReasonBackdated and ReasonNoFee are
// shared with the delivery lane verbatim: they name the same guard.
const (
	// ReasonNoAnchor: an empty batch — nothing to spend, so a zero budget.
	ReasonNoAnchor = "no-anchor"
	// ReasonAnchorMalformed: a serial of the wrong length.
	ReasonAnchorMalformed = "anchor-serial-malformed"
	// ReasonAnchorFuture: the anchor's issue epoch is AHEAD of the ledger's
	// clock. The node's keyset holds no future key, so this cannot verify
	// upstream; the ledger refuses independently because a future-dated entry
	// could never be swept (the ReasonBackdated mirror).
	ReasonAnchorFuture = "anchor-future-dated"
)

// SpendRelayAnchors is the ledger half of a relay session open (R2.14). See the
// package note above for what it guarantees. It returns the summed face of the
// anchors it recorded — the session budget — or 0 and the named reason it recorded
// nothing.
//
// current is n.chainEpoch() at the relay, the epoch its self keyset was pruned
// with. It advances the ledger's monotone epoch watermark exactly as
// RedeemDeliveryCreditReason does (R0.4b-5), so the two lanes share one clock on
// one guard.
func (l *Ledger) SpendRelayAnchors(anchors []RelayAnchor, current uint64) (face int64, reason string) {
	if len(anchors) == 0 {
		return 0, ReasonNoAnchor
	}
	if current > l.epochWatermark {
		l.epochWatermark = current
		l.sweepIfEpochAdvanced(l.epochWatermark) // the band advance sweeps (advisory C-7)
	}
	if l.paidStore != nil && !l.guardLoaded {
		return 0, ReasonGuardUnloaded // a ledger that does not know what it accepted must not accept
	}
	if l.fee <= 0 {
		return 0, ReasonNoFee
	}
	// CHECK EVERYTHING BEFORE RECORDING ANYTHING (all-or-nothing). A batch that
	// presents one anchor twice is a double spend inside the batch and is refused
	// the same way.
	inBatch := make(map[string]struct{}, len(anchors))
	for _, a := range anchors {
		if len(a.Serial) != relayAnchorSerialSize {
			return 0, ReasonAnchorMalformed
		}
		if a.Epoch > l.epochWatermark {
			return 0, ReasonAnchorFuture
		}
		if a.Epoch+paidSerialWindow < l.epochWatermark {
			return 0, ReasonBackdated
		}
		key := paidKey(a.Epoch, a.Serial)
		if _, spent := l.paidSerial[key]; spent {
			return 0, ReasonAlreadyPaid
		}
		if _, dup := inBatch[key]; dup {
			return 0, ReasonAlreadyPaid
		}
		inBatch[key] = struct{}{}
	}
	// Reserve k slots. The guard REFUSES at a cap full of still-live entries, never
	// evicts one (G-A2 — the self-financing eviction pump, closed by R0.4b, must
	// not re-open on the relay lane).
	if !l.reservePaidSerials(l.epochWatermark, len(anchors)) {
		l.guardFullRefusals++
		return 0, ReasonGuardFull
	}
	// RECORD DURABLY BEFORE THE SESSION IS ADMITTED (red-team re-break F2): the
	// caller admits only on a non-empty face, so a crash after this returns leaves
	// a guard entry for a session that never forwarded — an under-pay — and never a
	// session whose anchors a restart would re-open for a second spend.
	for _, a := range anchors {
		if err := l.addPaidSerial(a.Serial, ports.NodeID{}, a.Epoch); err != nil {
			return 0, ReasonGuardStore
		}
	}
	return int64(len(anchors)) * l.fee, ""
}
