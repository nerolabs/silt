// D-DEMAND wiring (#181): the delivery-receipt path that turns the pure
// core/demand primitive into a live network capability. A fetcher that received an
// object's bytes spends a blind-withdrawn retrieval token by submitting a
// signed receipt to the server; the server banks it into a NEUTRAL
// witnessed-demand observable and settles the conserved delivery credit (the
// PoD neutral lane, docs/design/pod.md — certified 2026-08-26).
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

	"github.com/nerolabs/silt/core/credit"
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

// EnableDemandBank makes this node BANK delivery receipts: on a MsgDeliveryReceipt
// it verifies the receipt against the per-epoch key WINDOW of the retrieval-token
// issuer it trusts and, if valid + IN-WINDOW + first-seen + a correct delivery to
// THIS server, credits the object's witnessed-demand counter. Demand is a neutral
// observable — never standing.
//
// R0.4b: issuer is a NodeID, not an RSA key. The bank verifies against
// {key_E : current−W <= E <= current}, and a key enters that window ONLY after its
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

// WitnessedDemand is the neutral count of correctly-delivered, token-backed
// retrievals banked for object. 0 when demand banking is off. Observability only.
func (n *Node) WitnessedDemand(object ports.Hash) int64 {
	if n.demandBank == nil {
		return 0
	}
	return n.demandBank.Demand(object)
}

// SubmitDeliveryReceipt is the fetcher side: having received AND content-verified
// object's bytes from server (the fetch path re-verifies every read — tenet B3),
// it spends token by signing a delivery receipt and submitting it. done reports
// whether the server banked it. The receipt carries no bytes and no PoR — the
// certified neutral-lane shape (see the core/demand package comment).
func (n *Node) SubmitDeliveryReceipt(server ports.NodeID, token demand.Token, object ports.Hash, done func(credited bool, err error)) {
	if n.signer == nil {
		done(false, ErrNoSigner)
		return
	}
	receipt := demand.Ack(n.signer, token, object, server)
	blob, err := demand.SubmittedReceipt{Token: token, Receipt: receipt}.Marshal()
	if err != nil {
		done(false, err)
		return
	}
	n.request(server, ports.Message{Kind: ports.MsgDeliveryReceipt, Data: blob}, func(resp ports.Message, rerr error) {
		if rerr != nil {
			done(false, rerr)
			return
		}
		done(resp.OK, nil)
	})
}

// deliveryReasoner is the optional half of ports.CreditLedger that reports WHY a
// delivery redeem paid nothing (core/credit implements it). Optional so the port
// stays narrow and test doubles need not carry it; a ledger without it simply logs
// no reason.
type deliveryReasoner interface {
	RedeemDeliveryCreditReason(server, fetcher ports.NodeID, root ports.Hash,
		serial []byte, issuedEpoch uint64) (int64, string)
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

// handleDeliveryReceipt is the server side: verify + bank a submitted receipt, then
// ack whether it credited witnessed demand. It only banks receipts naming THIS node
// as the delivering server, so a receipt for another server can't be redirected here.
func (n *Node) handleDeliveryReceipt(from ports.NodeID, msg ports.Message) {
	deny := func() { n.reply(from, msg, ports.Message{Kind: ports.MsgDeliveryReceiptAck, OK: false}) }
	if n.demandBank == nil {
		deny()
		return
	}
	// R0.4b: the keyset is the validity window. Pruned to [current−W, current] on
	// read, so a token whose issuing epoch has left the window has NO key that
	// verifies it and is rejected before any credit path.
	keys := n.DemandIssuerKeyset(n.demandIssuer)
	if keys == nil {
		deny()
		return
	}
	sub, err := demand.UnmarshalSubmittedReceipt(msg.Data)
	if err != nil {
		deny()
		return
	}
	if sub.Receipt.Server != n.id {
		deny() // a receipt must name this server; don't bank one addressed elsewhere
		return
	}
	current := n.chainEpoch()
	credited, issuedEpoch, reason := n.demandBank.Redeem(keys, current, sub.Token, sub.Receipt)
	if credited {
		// The PoD neutral lane (docs/design/pod.md §3, certified): a banked
		// receipt settles the conserved delivery credit — the fetcher's
		// withdrawal fee less the durability skim — superseding this
		// delivery's provisional serve self-credit. Balance only, never
		// standing. The receipt's Fetcher key hashes to the requester NodeID
		// (NodeID = sha256(pubkey)), which is how the credit finds the serve
		// lane it supersedes.
		var paid int64
		var why string
		if n.ledger != nil {
			// The serial gates the cross-server double-redeem (one token funds ONE
			// conserved payout); the issuing epoch is what lets the guard set evict
			// BY EXPIRY, so an evicted serial is always an un-redeemable one (R0.4b-3,
			// the certification's coupling condition). The CURRENT epoch is not
			// passed: the ledger reads it from its own EpochSource, which the daemon
			// wires to this node's chainEpoch() — the same `current` the bank just
			// redeemed against (R2.10 / F8).
			if r, ok := n.ledger.(deliveryReasoner); ok {
				paid, why = r.RedeemDeliveryCreditReason(n.id, ports.HashBytes(sub.Receipt.Fetcher),
					sub.Receipt.Object, sub.Receipt.Serial, issuedEpoch)

			} else {
				paid = n.ledger.RedeemDeliveryCredit(n.id, ports.HashBytes(sub.Receipt.Fetcher),
					sub.Receipt.Object, sub.Receipt.Serial, issuedEpoch)

			}
		}
		n.logf(ports.LogInfo, "delivery receipt banked", "object", sub.Receipt.Object,
			"demand", n.demandBank.Demand(sub.Receipt.Object), "credit", paid)
		// A banked receipt that settled NOTHING gets its own line, at WARN, naming
		// the reason. The "banked" line above is an announced observable (S5) and
		// stays exactly as it is — but on its own it reads as success while the
		// server was in fact refused a payout, and the operator-actionable case (the
		// paid-serial guard full of live serials = a serve rate above the bound the
		// cap was derived against) was invisible. Deliberately NOT the word "banked".
		if paid == 0 && why != "" && why != credit.ReasonPaid {
			n.logf(ports.LogWarn, "delivery receipt paid NO credit", "object", sub.Receipt.Object,
				"reason", why, "serial_guard_refusals", guardFullRefusals(n.ledger))
		}
	} else {
		n.logf(ports.LogDebug, "delivery receipt rejected", "reason", reason)
	}
	n.reply(from, msg, ports.Message{Kind: ports.MsgDeliveryReceiptAck, OK: credited})
}
