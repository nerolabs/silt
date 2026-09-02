// R0.4b per-epoch demand-issuer key distribution: serve, fetch, CROSS-CHECK, pin.
//
// WHY THIS IS A SEPARATE LANE FROM MsgGetIssuerKey. The routing and the R0.4b
// certification both name MsgGetIssuerKey / answerIssuerKey / peerIssuerKeys as the
// key path to make epoch-plural. Verified against source, that identification is
// wrong and following it literally would be a CONSENSUS BREAK: MsgGetIssuerKey
// serves the PUBLISH-token issuer key (tokenrole.go), IssuerKeyOf is wired as the
// chain's issuerKey lookup (chain.RequireTokens), and publishtoken.Verify re-verifies
// COMMITTED publish tokens against it on every replay. Rotating that key per epoch
// would make historical committed publish tokens fail verification on replay.
//
// Demand withdrawal reuses that same key today only because RSA-FDH signing is
// domain-agnostic — answerTokenRequest cannot tell a demand blind from a publish
// blind, since the blinded value is opaque and the domain lives in the FDH message
// the REQUESTER built. So the per-epoch keyset is a SEPARATE demand-lane keyset and
// the issuer must be TOLD which lane it is signing for. This file is that lane; the
// publish-token lane is untouched. See
// docs/thinking/2026-09-02-r0.4b-per-epoch-key-expiry-design.md §7.
//
// THE CROSS-CHECK IS THE POINT (research certification 2026-09-02, Verdict 2). A
// fetched key is NEVER held on the peer's say-so. It is held only if its fingerprint
// equals the CONSENSUS-ATTESTED commitment for (issuer, epoch)
// (chain.IssuerKeyCommitment). Without that, per-epoch keys are WORSE for privacy
// than no epoch at all: a Byzantine issuer serves a distinct key_E to a small cohort
// and "which key verified you" becomes a fingerprint. Pinning a peer-served keyset
// does not close it — an issuer that equivocates keys equivocates its published list
// too. pinDemandIssuerKey is the single choke point; every path that would hold a
// key goes through it.
package node

import (
	"crypto/rsa"
	"io"

	"github.com/fxamacker/cbor/v2"
	"github.com/nerolabs/silt/core/blindtoken"
	"github.com/nerolabs/silt/core/chain"
	"github.com/nerolabs/silt/core/demand"
	"github.com/nerolabs/silt/ports"
)

// epochKeyDER is one (epoch, public key) pair on the wire.
type epochKeyDER struct {
	Epoch uint64 `cbor:"1,keyasint"`
	DER   []byte `cbor:"2,keyasint"` // blindtoken.MarshalPub of key_Epoch
}

// demandKeysetWire is the served window {key_E : current−W <= E <= current}.
type demandKeysetWire struct {
	Keys []epochKeyDER `cbor:"1,keyasint"`
}

// chainEpoch is the CONSENSUS epoch index at this node's head: head_height /
// EpochBlocks. Deterministic and Byzantine-agreed — never wall-clock, which is what
// the R0.4 certification's Q1 requires and what makes the validity window carry no
// skew channel. With no chain or epochs disabled it is 0, which degenerates the
// window to "epoch 0 only" without breaking anything.
func (n *Node) chainEpoch() uint64 {
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

// SetDemandIssuerKey installs priv as this node's demand-token blind-signing key for
// epoch, prunes the retained window, and STAGES the on-chain commitment of its
// fingerprint so redeemers can resolve key_epoch against consensus state.
//
// The issuer signs only under the CURRENT epoch's key but keeps the window's PUBLIC
// keys servable, so an in-window token withdrawn under an older key is not stranded
// on rotation (crypto advisory §3, residual R5). WHEN an operator rotates is ops
// policy and is deliberately not scheduled here.
func (n *Node) SetDemandIssuerKey(epoch uint64, priv *rsa.PrivateKey) {
	if priv == nil {
		return
	}
	if n.demandIssuers == nil {
		n.demandIssuers = map[uint64]*blindtoken.Issuer{}
	}
	n.demandIssuers[epoch] = blindtoken.NewIssuer(priv)
	// Retain only the window: older private keys are dropped, which is also what
	// makes expiry real on the issuing side.
	cur := n.chainEpoch()
	for e := range n.demandIssuers {
		if e+demand.DefaultWindow < cur {
			delete(n.demandIssuers, e)
		}
	}
	// Stage the commitment. Without a signer there is no identity to bind the key
	// to, so there is nothing sound to commit.
	if n.signer == nil {
		return
	}
	fp := demand.KeyFingerprint(&priv.PublicKey)
	reg := chain.SignIssuerKeyReg(n.signer, epoch, fp)
	for i := range n.pendingIssuerKeys {
		if n.pendingIssuerKeys[i].Epoch == epoch {
			return // already staged for this epoch; append-only, never re-point
		}
	}
	n.pendingIssuerKeys = append(n.pendingIssuerKeys, reg)
}

// DemandEpoch is the consensus epoch index this node would issue a demand token
// under right now. Exposed so the daemon can install its key for the live epoch
// without re-deriving the clock.
func (n *Node) DemandEpoch() uint64 { return n.chainEpoch() }

// answerDemandIssuerKeys serves this issuer's PUBLIC key window. Only in-window
// epochs are served: a key outside the window verifies nothing, so serving it would
// only widen what a redeemer might wrongly hold.
func (n *Node) answerDemandIssuerKeys() ports.Message {
	reply := ports.Message{Kind: ports.MsgDemandIssuerKeysReply}
	if len(n.demandIssuers) == 0 {
		return reply // not a demand issuer → OK=false
	}
	cur := n.chainEpoch()
	var w demandKeysetWire
	for e, iss := range n.demandIssuers {
		if e > cur || cur-e > demand.DefaultWindow {
			continue
		}
		w.Keys = append(w.Keys, epochKeyDER{Epoch: e, DER: blindtoken.MarshalPub(iss.Public())})
	}
	if len(w.Keys) == 0 {
		return reply
	}
	blob, err := cbor.Marshal(w)
	if err != nil {
		return reply
	}
	reply.Data = blob
	reply.OK = true
	return reply
}

// pinDemandIssuerKey is the SINGLE choke point where a fetched key_E becomes a key
// this node will verify against. It holds the key only if the consensus-attested
// commitment for (issuer, epoch) EXISTS and MATCHES its fingerprint.
//
// Both refusals are load-bearing:
//   - MISMATCH is a targeted (per-cohort) key — the fingerprinting attack itself.
//   - ABSENT is a key the issuer never committed. Accepting it would let an issuer
//     bypass the binding entirely by simply never registering, which would make the
//     whole commitment optional and therefore worthless.
//
// With no chain there is nothing consensus-attested to resolve against, so this
// refuses. That is deliberate: a redeemer with no chain has no anti-fingerprinting
// anchor, and the certification is explicit that shipping the construction WITHOUT
// the anchor is unsafe.
func (n *Node) pinDemandIssuerKey(issuer ports.NodeID, epoch uint64, pub *rsa.PublicKey) bool {
	if pub == nil || n.chain == nil {
		return false
	}
	committed, ok := n.chain.IssuerKeyCommitment(issuer, epoch)
	if !ok || committed != demand.KeyFingerprint(pub) {
		return false
	}
	ks := n.peerDemandKeys[issuer]
	if ks == nil {
		ks = demand.NewKeyset(demand.DefaultWindow)
		n.peerDemandKeys[issuer] = ks
	}
	// Append-only within a keyset too: once key_E is pinned it is never replaced, so
	// a later serve of a different key for the same epoch cannot re-point it.
	if ks.Key(epoch) != nil {
		return true
	}
	ks.Put(epoch, pub)
	return true
}

// DemandIssuerKeyset returns the pinned keyset for issuer, pruned to the window at
// this node's current consensus epoch. Pruning here is what ENFORCES expiry on the
// verifying side: a key whose epoch has left the window is dropped, and every token
// from that epoch stops verifying at once.
func (n *Node) DemandIssuerKeyset(issuer ports.NodeID) *demand.Keyset {
	if issuer == n.id {
		// Self-issuance goes through the SAME cross-check as any peer's key — no
		// self-trust exception. A node that pinned its own keys unconditionally
		// would accept its own uncommitted key_E, which is precisely the bypass the
		// binding exists to close. This re-tries the pin because the commitment
		// lands a block or more after SetDemandIssuerKey staged it.
		for e, iss := range n.demandIssuers {
			n.pinDemandIssuerKey(n.id, e, iss.Public())
		}
	}
	ks := n.peerDemandKeys[issuer]
	if ks == nil {
		return nil
	}
	ks.Prune(n.chainEpoch())
	return ks
}

// FetchDemandIssuerKeys asks issuer for its per-epoch demand key window and pins
// every key that resolves against the committed binding. Keys that do NOT resolve
// are silently DROPPED, not held — that is the refusal the equivocation gate pins.
// done reports how many keys were pinned; zero means nothing verifiable was served.
func (n *Node) FetchDemandIssuerKeys(issuer ports.NodeID, done func(pinned int, err error)) {
	n.request(issuer, ports.Message{Kind: ports.MsgGetDemandIssuerKeys}, func(resp ports.Message, err error) {
		switch {
		case err != nil:
			done(0, err)
		case !resp.OK || len(resp.Data) == 0:
			done(0, errNoIssuerKey)
		default:
			var w demandKeysetWire
			if uerr := cbor.Unmarshal(resp.Data, &w); uerr != nil {
				done(0, uerr)
				return
			}
			pinned := 0
			for _, k := range w.Keys {
				pub, perr := blindtoken.ParsePub(k.DER)
				if perr != nil {
					continue
				}
				if n.pinDemandIssuerKey(issuer, k.Epoch, pub) {
					pinned++
				}
			}
			done(pinned, nil)
		}
	})
}

// AcquireDemandToken is the fetcher side of a per-epoch blind withdrawal (R0.4b). It
// blinds a fresh serial against the issuer's CURRENT-epoch key, sends it on the
// demand lane, and unblinds the reply under the key for the epoch the issuer names.
//
// The key it blinds against comes from the PINNED keyset — a key that resolved
// against the committed E -> key_E binding — so a fetcher never withdraws under a
// key the network does not agree on. That matters on the fetcher side too: a token
// withdrawn under an off-commitment key would be un-redeemable everywhere, so a
// targeting issuer could strand a cohort's tokens instead of fingerprinting them.
//
// done fires once with the token and its ISSUING EPOCH.
func (n *Node) AcquireDemandTokenInWindow(rng io.Reader, issuer ports.NodeID, done func(demand.Token, uint64, error)) {
	ks := n.DemandIssuerKeyset(issuer)
	if ks == nil {
		done(demand.Token{}, 0, errNoIssuerKey)
		return
	}
	cur := n.chainEpoch()
	pub := ks.Key(cur)
	if pub == nil {
		done(demand.Token{}, 0, errNoIssuerKey)
		return
	}
	serial, err := blindtoken.NewSerial(rng)
	if err != nil {
		done(demand.Token{}, 0, err)
		return
	}
	blinded, secret, err := demand.Withdraw(rng, pub, serial)
	if err != nil {
		done(demand.Token{}, 0, err)
		return
	}
	n.request(issuer, ports.Message{Kind: ports.MsgDemandTokenRequest, Data: blinded}, func(resp ports.Message, rerr error) {
		if rerr != nil {
			done(demand.Token{}, 0, rerr)
			return
		}
		if !resp.OK || len(resp.Data) == 0 {
			done(demand.Token{}, 0, ErrTokenAcquire)
			return
		}
		// The issuer names the epoch it signed under. Unblind with THAT epoch's
		// pinned key: if the issuer signed under a key this node has not pinned
		// (an off-commitment key, or one outside the window), there is nothing to
		// unblind with and the withdrawal fails loudly rather than yielding a
		// token nobody will ever redeem.
		signPub := ks.Key(resp.Height)
		if signPub == nil {
			done(demand.Token{}, 0, errNoIssuerKey)
			return
		}
		done(demand.Unblind(signPub, serial, resp.Data, secret), resp.Height, nil)
	})
}

// answerDemandTokenRequest blind-signs a demand withdrawal under the CURRENT epoch's
// key and returns that epoch in Height, so the withdrawer knows which key_E its token
// was signed under. It reuses answerTokenRequest's fee/credit settlement and its
// retry-dedup discipline verbatim (research certification 2026-08-13, A2: a lost
// reply makes the requester re-present the same blinded serial, and without dedup the
// issuer charges twice). The dedup key is namespaced by epoch so a re-presented blind
// after a rotation is a genuinely new issuance, not a stale cache hit.
func (n *Node) answerDemandTokenRequest(from ports.NodeID, msg ports.Message) ports.Message {
	reply := ports.Message{Kind: ports.MsgDemandTokenReply}
	cur := n.chainEpoch()
	iss := n.demandIssuers[cur]
	if iss == nil || len(msg.Data) == 0 {
		return reply // not a demand issuer for this epoch / nothing to sign → OK=false
	}
	key := demandDedupKey(cur, msg.Data)
	now := n.clock.Now()
	if e, ok := n.tokenIssued[key]; ok && now < e.expiry {
		reply.Data = e.sig
		reply.Height = cur
		reply.OK = true
		return reply // a retry of an issuance already settled: same sig, no new charge
	}
	charge := n.tokenChargeFor(from, msg.Credit)
	if charge == nil {
		return reply
	}
	blindSig, err := iss.Issue(charge, msg.Data)
	if err != nil {
		return reply
	}
	if len(n.tokenIssued) >= maxTokenIssued {
		for k, e := range n.tokenIssued {
			if now >= e.expiry {
				delete(n.tokenIssued, k)
			}
		}
	}
	n.tokenIssued[key] = tokenIssuedEntry{sig: blindSig, expiry: now.Add(n.tokenDedupTTL())}
	reply.Data = blindSig
	reply.Height = cur
	reply.OK = true
	return reply
}

// demandDedupKey namespaces the issuance-dedup cache by lane and epoch, so a demand
// blind can never collide with a publish blind and a rotation is not masked by a
// stale entry.
func demandDedupKey(epoch uint64, blinded []byte) string {
	var e [8]byte
	for i := 0; i < 8; i++ {
		e[i] = byte(epoch >> (56 - 8*i))
	}
	sum := ports.HashBytes(append(append([]byte("silt/demandtoken/dedup/v1"), e[:]...), blinded...))
	return string(sum[:])
}
