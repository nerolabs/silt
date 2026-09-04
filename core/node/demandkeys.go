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
// The issuer retains the whole window's PRIVATE keys, not just the current epoch's
// (the map below holds a blindtoken.Issuer per epoch, private half included), which
// is what lets it honour a withdrawal that NAMES an in-window past epoch — the
// epoch-boundary race, where the requester's view of the consensus clock trails the
// issuer's by one. It is also why a token withdrawn under an older key is not
// stranded on rotation (crypto advisory §3, residual R5).
//
// ROTATION IS SCHEDULED, NOT OPS POLICY (R0.4b C3 close). The lane can only issue
// for an epoch whose key is both held here and committed on-chain, so a one-shot
// install kills the lane W+1 epochs after boot while the fee-charging paths keep
// running (red-team probe B, 2026-09-02). cmd/silt/daemon.go drives the schedule off
// the commit stream, pre-publishing key_E for the whole [cur, cur+W] band; this
// method is the per-epoch step it calls.
// rng is the randomness the private-key operation blinds with (advisory C-2); see
// EnableTokenIssuer.
func (n *Node) SetDemandIssuerKey(rng io.Reader, epoch uint64, priv *rsa.PrivateKey) {
	if priv == nil {
		return
	}
	if n.demandIssuers == nil {
		n.demandIssuers = map[uint64]*blindtoken.Issuer{}
	}
	n.demandIssuers[epoch] = blindtoken.NewIssuer(rng, priv)
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
	// Drop staged registrations whose epoch has already passed. The proposer fold
	// drops them too (it must — the epoch can turn between staging and proposing),
	// but a node that never gets to propose would otherwise accumulate W+1 dead regs
	// per epoch forever: unbounded state on the floor box (build-immutable #8).
	if len(n.pendingIssuerKeys) > 0 {
		live := n.pendingIssuerKeys[:0]
		for _, r := range n.pendingIssuerKeys {
			if r.Epoch >= cur {
				live = append(live, r)
			}
		}
		n.pendingIssuerKeys = live
	}
	// Never stage a BACKDATED registration. validateIssuerKeys rejects r.Epoch < the
	// block's epoch, so staging one queues a proposal our own local pre-check will
	// refuse. The proposer fold drops such regs too (the belt to this brace, since the
	// epoch can advance between staging and proposing), but not staging it in the
	// first place keeps the queue honest: the scheduler re-stages for the epochs that
	// are still registrable.
	if epoch < cur {
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
// only widen what a redeemer might wrongly hold. Pre-published FUTURE keys are held
// (the scheduler generates the whole [cur, cur+W] band so the commitments land ahead
// of time) but not served — a withdrawal names the CURRENT epoch, so a future key has
// no honest use at a redeemer.
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
	// THE PIN FOLLOWS THE CHAIN (red-team break 4, 2026-09-02). Append-only belongs to
	// the CHAIN's binding (applyIssuerKeys is first-write-wins); the pin is only a
	// CACHE of it. The earlier rule — "once key_E is pinned it is never replaced" —
	// left a redeemer that reorged onto a fork committing a DIFFERENT key_E verifying
	// against the abandoned fork's key and refusing the canonical one for W+1 epochs,
	// while reporting the re-pin as a success. Any key that reaches this line has
	// already matched the CURRENT commitment above, so writing it is exactly
	// "hold what the chain says": a held key is replaced only by the committed one,
	// never by a peer's say-so.
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
	// Re-validate every held key against the CURRENT chain on every read. The pin is a
	// cache of the committed binding, and a reorg (or any adopt that re-points the
	// commitment for an epoch) must not leave a stale key verifying tokens
	// (red-team break 4). Cost is at most W+1 sha256 over a 2048-bit modulus per read
	// — negligible next to the RSA verify the read exists to perform. With no chain
	// there is nothing to resolve against, so nothing may be held.
	ks.Retain(func(e uint64, pub *rsa.PublicKey) bool {
		if n.chain == nil {
			return false
		}
		fp, ok := n.chain.IssuerKeyCommitment(issuer, e)
		return ok && fp == demand.KeyFingerprint(pub)
	})
	ks.Prune(n.chainEpoch())
	return ks
}

// ResolvedDemandIssuerKey returns the issuer's CHAIN-RESOLVED demand key for the
// epoch a withdrawal should name right now: the current consensus epoch's key, held
// only because its fingerprint matched the committed E ↦ key_E binding.
//
// It is the single seam a caller OUTSIDE the event loop's own withdrawal path (the
// D3 client's durable parent, which resolves against ITS chain and hands the pair to
// a chain-less ephemeral node) uses, so that no shipped lane blinds against a key
// the network has not agreed on. A false return means "this node cannot name a sound
// key for this issuer right now" — a denial, never a fall-back to an unpinned key.
func (n *Node) ResolvedDemandIssuerKey(issuer ports.NodeID) (*rsa.PublicKey, uint64, bool) {
	ks := n.DemandIssuerKeyset(issuer)
	if ks == nil {
		return nil, 0, false
	}
	cur := n.chainEpoch()
	pub := ks.Key(cur)
	return pub, cur, pub != nil
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
			done(0, ErrNoIssuerKey)
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

// AcquireDemandTokenInWindow is the fetcher side of a per-epoch blind withdrawal
// (R0.4b): it blinds a fresh serial for the CURRENT consensus epoch under that
// epoch's key, names the epoch in the request, and refuses a reply signed for any
// other. It is the withdrawal `swarm receipt` runs.
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
		done(demand.Token{}, 0, ErrNoIssuerKey)
		return
	}
	cur := n.chainEpoch()
	pub := ks.Key(cur)
	if pub == nil {
		done(demand.Token{}, 0, ErrNoIssuerKey)
		return
	}
	n.withdrawDemandToken(rng, issuer, pub, cur, nil, done)
}

// withdrawDemandToken is the shared fetcher half of every sound demand withdrawal:
// blind the serial for epoch E under the CHAIN-RESOLVED key_E, name E in the
// request, and accept the reply only if the issuer signed for that same E.
//
// THE REQUESTER NAMES THE EPOCH (R0.4b (b1)). E is inside the blind-signed message,
// so an issuer that signs under any other epoch's key produces a signature this
// withdrawal cannot unblind into anything redeemable — a DENIAL, never a silently
// re-dated token. Refusing on resp.Height != E turns that into a loud, immediate
// failure instead of a token discovered worthless at redemption. It also closes the
// epoch-boundary race in the honest direction: an issuer one epoch ahead still holds
// key_E and signs for the E we asked for.
//
// credit, when non-nil, pays the fee with a PREPAID BLIND CREDIT instead of charging
// this node's account (the D3 path).
func (n *Node) withdrawDemandToken(rng io.Reader, issuer ports.NodeID, pub *rsa.PublicKey,
	epoch uint64, credit *ports.PublishCredit, done func(demand.Token, uint64, error)) {
	n.withdrawBlind(rng, issuer, pub, epoch, credit, demand.Withdraw,
		func(p *rsa.PublicKey, e uint64, serial, blindSig, secret []byte) ([]byte, error) {
			tok, err := demand.Unblind(p, e, serial, blindSig, secret)
			return tok.Sig, err
		},
		func(serial, sig []byte, err error) {
			if err != nil {
				done(demand.Token{}, 0, err)
				return
			}
			done(demand.Token{Serial: serial, Sig: sig}, epoch, nil)
		})
}

// withdrawBlind is the lane-generic withdrawal withdrawDemandToken and
// AcquireRelayAnchors share (R2.14): the blind and unblind primitives decide the
// FDH domain (demand token vs relay anchor); everything else — the fresh serial,
// the epoch named in the request, the refusal of a reply signed for any other
// epoch, the verify-after-unblind — is identical, and the issuer side is untouched
// (it signs opaque blinded bytes). done fires once with the serial and the
// unblinded signature, or the error.
func (n *Node) withdrawBlind(rng io.Reader, issuer ports.NodeID, pub *rsa.PublicKey,
	epoch uint64, credit *ports.PublishCredit,
	blind func(io.Reader, *rsa.PublicKey, uint64, []byte) (blinded, secret []byte, err error),
	unblind func(pub *rsa.PublicKey, epoch uint64, serial, blindSig, secret []byte) ([]byte, error),
	done func(serial, sig []byte, err error)) {

	serial, err := blindtoken.NewSerial(rng)
	if err != nil {
		done(nil, nil, err)
		return
	}
	blinded, secret, err := blind(rng, pub, epoch, serial)
	if err != nil {
		done(nil, nil, err)
		return
	}
	req := ports.Message{Kind: ports.MsgDemandTokenRequest, Data: blinded, Height: epoch, Credit: credit}
	n.request(issuer, req, func(resp ports.Message, rerr error) {
		if rerr != nil {
			done(nil, nil, rerr)
			return
		}
		if !resp.OK || len(resp.Data) == 0 {
			done(nil, nil, ErrTokenAcquire)
			return
		}
		if resp.Height != epoch {
			// The issuer signed for an epoch we did not ask for. The signature is
			// over OUR epoch's message, so it can only be a mismatch or a targeting
			// attempt; either way the token would be un-redeemable.
			done(nil, nil, ErrDemandEpochMismatch)
			return
		}
		// RFC 9474 §4.4 Finalize (advisory C-1): verify the unblinded signature under
		// key_epoch BEFORE handing the caller a token. A malicious issuer that returns
		// a dud used to charge the withdrawal fee and hand back something that only
		// failed at redemption — and a receipt that fails Bank.Redeem never reaches
		// the ledger, so the serve's eager self-mint is never reversed. Refuse here.
		sig, uerr := unblind(pub, epoch, serial, resp.Data, secret)
		if uerr != nil {
			done(nil, nil, uerr)
			return
		}
		done(serial, sig, nil)
	})
}

// answerDemandTokenRequest blind-signs a demand withdrawal under the key for the
// epoch THE REQUEST NAMES (msg.Height) and echoes that epoch in the reply, so the
// withdrawer knows which key_E its token was signed under.
//
// THE THREE ADMISSION RULES, all load-bearing (R0.4b (b1)):
//   - E <= cur. Signing for a FUTURE epoch would hand out a token that outlives the
//     honest window (it expires at E+W), so it is refused even though the issuer
//     already holds the pre-published key.
//   - cur − E <= W. Below that the token is born expired; the requester gains
//     nothing and the redeemer would reject it anyway.
//   - key_E is held. The window's PRIVATE keys are retained (SetDemandIssuerKey), so
//     an in-window past epoch is signable — which is exactly what lets a requester
//     whose consensus clock trails by one epoch still buy a usable token instead of
//     burning a fee on the boundary race.
//
// A requester that names an earlier E only shortens its OWN token's life, so there
// is nothing to gain by lying and nothing to police beyond the window.
//
// It reuses answerTokenRequest's fee/credit settlement and its retry-dedup
// discipline verbatim (research certification 2026-08-13, A2: a lost reply makes the
// requester re-present the same blinded serial, and without dedup the issuer charges
// twice). The dedup key is namespaced by the SIGNED epoch so a re-presented blind
// after a rotation is a genuinely new issuance, not a stale cache hit.
func (n *Node) answerDemandTokenRequest(from ports.NodeID, msg ports.Message) ports.Message {
	reply := ports.Message{Kind: ports.MsgDemandTokenReply}
	cur := n.chainEpoch()
	e := msg.Height
	if e > cur || cur-e > demand.DefaultWindow {
		return reply // out of the issuing window → OK=false, nothing charged
	}
	iss := n.demandIssuers[e]
	if iss == nil || len(msg.Data) == 0 {
		return reply // not a demand issuer for this epoch / nothing to sign → OK=false
	}
	key := demandDedupKey(e, msg.Data)
	now := n.clock.Now()
	if c, ok := n.tokenIssued[key]; ok && now < c.expiry {
		reply.Data = c.sig
		reply.Height = e
		reply.OK = true
		return reply // a retry of an issuance already settled: same sig, no new charge
	}
	charge, err := n.tokenChargeFor(from, msg.Credit)
	if err != nil {
		return reply // no publish issuer to verify an attached credit, or the credit is invalid/spent
	}
	blindSig, err := iss.Issue(charge, msg.Data)
	if err != nil {
		return reply
	}
	if len(n.tokenIssued) >= maxTokenIssued {
		for k, c := range n.tokenIssued {
			if now >= c.expiry {
				delete(n.tokenIssued, k)
			}
		}
	}
	n.tokenIssued[key] = tokenIssuedEntry{sig: blindSig, expiry: now.Add(n.tokenDedupTTL())}
	reply.Data = blindSig
	reply.Height = e
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
