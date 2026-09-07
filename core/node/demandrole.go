// D-DEMAND wiring (#181): the delivery-receipt path that turns the pure
// core/demand primitive into a live network capability. Since R2.9 the fetcher's
// blind-withdrawn token is the SESSION ANCHOR, spent at session open
// (deliverysession.go); deliveries are acknowledged by cumulative-count receipts on
// that session, banked into the NEUTRAL witnessed-demand observable and settled per
// increment. The v2 flat receipt (token spent at redeem) is RETIRED (B-9) and refused
// with a named reason below. What stays here: the demand bank's wiring, the bonded-
// fetcher credential, and the witnessed-demand readers.
//
// ISSUANCE IS ITS OWN LANE (R0.4b C3 close). Demand tokens are withdrawn on
// MsgDemandTokenRequest under a PER-EPOCH demand key, never on the publish-token lane
// (MsgTokenRequest) under the publish key. The publish key never enters the demand
// keyset, so a demand-domain blind bought on the publish lane is a token no bank will
// ever honour — and every shipped withdrawal path here goes through the pinned lane.
// A withdrawal blinds against a key that RESOLVED against the committed E ↦ key_E
// binding, names its issue epoch, and refuses a reply signed for any other epoch.
//
// NEUTRAL by construction: witnessed demand is an observable, never read by
// consensus standing, so a forged or self-dealt receipt buys ZERO standing (the
// γ→1/N firewall). The cost-to-wash re-pricing (fee-burn + bonded-fetcher
// credential) and fetcher-unlinkability (D3) are later D-DEMAND phases.
package node

import (
	"crypto/rsa"
	"errors"
	"io"

	"github.com/nerolabs/silt/core/demand"
	"github.com/nerolabs/silt/ports"
)

// ErrNoSigner is returned when a fetcher without a signing identity tries to
// produce a delivery receipt (SetSigner was never called).
var ErrNoSigner = errors.New("node: no signing identity (SetSigner) — cannot sign a delivery receipt")

// AcquireDemandTokenWithCredit is the D3 credit-paid withdrawal: it blind-withdraws a
// demand token from issuer for issue epoch epoch, paying the fee with a PREPAID BLIND
// CREDIT (acquired earlier under a durable identity via AcquireCredits) instead of
// charging this node's durable account. Because payment rides the credit — which the
// issuer verifies and spends without charging `from` — a node with NO balance and NO
// durable standing (an EPHEMERAL identity) can still withdraw. That is what lets the D3
// client withdraw over a throwaway identity so the issuer cannot link the withdrawal to
// the fetcher (the blind signature already hides the serial; this severs the
// account/identity link).
//
// issuerPub AND epoch ARE THE PARENT'S RESOLUTION, NOT THE ISSUER'S SAY-SO (R0.4b,
// red-team break 5). An ephemeral node has no chain, so it cannot resolve key_E against
// the committed E ↦ key_E binding itself; the DURABLE parent does that
// (Node.ResolvedDemandIssuerKey) and hands the pair down. Blinding against whatever key
// the issuer happens to serve is what made a per-cohort key an ACCEPTED tagged token
// instead of a denial. The withdrawal refuses a reply that names any other epoch, so a
// targeting issuer gets a denial, not a fingerprint.
//
// done fires once with the token or an error.
func (n *Node) AcquireDemandTokenWithCredit(rng io.Reader, issuer ports.NodeID, issuerPub *rsa.PublicKey,
	epoch uint64, credit ports.PublishCredit, done func(demand.Token, error)) {
	if issuerPub == nil {
		done(demand.Token{}, ErrNoIssuerKey)
		return
	}
	c := credit // the issuer spends this instead of charging `from` (tokenChargeFor)
	n.withdrawDemandToken(rng, issuer, issuerPub, epoch, &c, func(t demand.Token, _ uint64, err error) {
		done(t, err)
	})
}

// EnableDemandBank gives this node a demand bank: the witnessed-demand observable that
// a SETTLED delivery-session increment bumps (SettleDeliveryReceipt → Bank.Witness, in
// the ledger's settled increments) and the bonded-fetcher credential
// (RequireBondedFetchers) read. Demand is a neutral observable — never standing.
//
// issuer is the retrieval-token issuer this node RESOLVES keys for (FetchDemandIssuerKeys
// pins its committed key_E). It is NOT the issuer whose anchors this node accepts: a
// session anchor verifies under this server's OWN committed key only (verifyDeliveryAnchors,
// the own-key rule — gated by TestComposedSessions_ForeignIssuersFreshAnchorIsRefused).
// The v2 flat receipt this bank once banked (MsgDeliveryReceipt) is retired (B-9).
//
// R0.4b: issuer is a NodeID, not an RSA key. A key enters a keyset ONLY after its
// fingerprint matched the consensus-attested commitment (pinDemandIssuerKey). Taking
// a raw key here would be exactly the architecture the R0.4b certification refuses:
// a redeemer with nothing consensus-attested to resolve key_E against.
func (n *Node) EnableDemandBank(issuer ports.NodeID) {
	n.demandBank = demand.NewBank()
	n.demandIssuer = issuer
}

// RequireBondedFetchers turns on the P3b bonded-fetcher credential for this node's
// demand bank: a delivery receipt counts toward witnessed demand only if the
// fetcher's signing key hashes to a bond-distinct identity in this node's COMMITTED
// on-chain bond ledger (chain.IsBonded), and demand then counts distinct bonded
// fetchers per object. This prices fake demand onto the same Sybil-priced storage-
// bond supply C2 measures — so washing U units costs U real bonds, not U free tokens.
// No-op unless both demand banking (EnableDemandBank) and the chain (EnableChain) are
// on. Demand stays a neutral observable throughout — the gate changes what COUNTS as
// witnessed demand, never whether demand touches standing (it never does).
//
// Unlinkability residual: the fetcher's bonded key rides the receipt in the clear, so
// this LINKS a fetch to its bonded identity (fetcher-unlinkability stays nominal until
// D3/H8 — the demand.BondCheck doc marks where a blind bond-distinctness proof lands).
func (n *Node) RequireBondedFetchers() {
	if n.demandBank == nil || n.chain == nil {
		return
	}
	n.demandBank.RequireBondedFetcher(func(fetcherPub []byte) (string, bool) {
		id := ports.HashBytes(fetcherPub) // NodeID = sha256(pubkey)
		if !n.chain.IsBonded(id) {
			return "", false
		}
		return string(id[:]), true
	})
}

// WitnessedDemand is the v2 flat lane's per-TOKEN demand counter (Bank.Demand). It is
// RETIRED with the flat receipt (B-9): nothing in production writes it any more — only
// Bank.Redeem did, and MsgDeliveryReceipt no longer reaches it — so it returns 0 on every
// live node. The session lane's observable is WitnessedIncrements (settled increments of
// DeliveryIncrementBytes). This accessor leaves with the core/demand v2 primitive in the
// attended retirement PR (core/demand/demand.go, the SubmittedReceipt note); it stays
// only so that PR is the one that removes the primitive and its readers together.
func (n *Node) WitnessedDemand(object ports.Hash) int64 {
	if n.demandBank == nil {
		return 0
	}
	return n.demandBank.Demand(object)
}

// guardFullRefusalCounter is the monotone count of redeems refused for a full
// paid-serial guard — the number an operator watches to see the guard's cap being
// hit at all, rather than inferring it from one-off log lines.
type guardFullRefusalCounter interface{ GuardFullRefusals() int64 }

func guardFullRefusals(l ports.CreditLedger) int64 {
	if c, ok := l.(guardFullRefusalCounter); ok {
		return c.GuardFullRefusals()
	}
	return 0
}

// handleDeliveryReceipt REFUSES the retired v2 flat receipt (B-9, R2.9: the token is
// spent at session OPEN and acknowledged by cumulative-count receipts — MsgDeliveryOpen /
// MsgDeliverySettle, deliverysession.go). Nothing is parsed, banked or paid: a fetcher
// left on the flat path would re-create the suppression break the anchored lane closed
// (build-questions certification 2026-09-04 §2.2 point 1), and one face must never buy a
// v2 demand unit beside its v3 increments (R-V2-V3-DEMAND-DILUTION, closed here). The
// kind keeps its number (appended kinds are pinned) and answers with the named reason.
func (n *Node) handleDeliveryReceipt(from ports.NodeID, msg ports.Message) {
	n.reply(from, msg, ports.Message{Kind: ports.MsgDeliveryReceiptAck, OK: false, Data: []byte(errFlatReceiptRetired.Error())})
}

// errFlatReceiptRetired is the S5 reason the retired lane answers with.
const errFlatReceiptRetired = deliveryError("delivery: the flat receipt (token spent at redeem) is retired — open a session (MsgDeliveryOpen) and acknowledge with cumulative-count receipts (MsgDeliverySettle); R2.9")
