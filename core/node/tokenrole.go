// Publish-token issuance and acquisition (T3, persona 14 / F1). A validator can be a
// token ISSUER: it blind-signs a requester's blinded serial, charging the fee
// to the requester's durable identity but learning nothing about the serial.
// A publisher ACQUIRES a token by collecting k such blind signatures from
// DISTINCT validators, so its publish carries a quorum credential and no
// durable identity. Verification and double-spend live in the chain
// (chain.RequireTokens); the fee links to identity, the token does not.
package node

import (
	"bytes"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"sort"

	"github.com/nerolabs/silt/core/blindtoken"
	"github.com/nerolabs/silt/ports"
)

var (
	// ErrTokenAcquire means fewer than k issuers granted a signature (e.g.
	// offline or out of the requester's credit). It is a COUNT outcome, and the
	// causes that produced it are separate errors — do not add a second meaning to
	// it. ITS SCOPE IS STATED HONESTLY RATHER THAN IDEALISED: two other sites still
	// return it, AcquireCredits for a malformed request (self-issuer or count <= 0,
	// both programming errors) and demandkeys.go for an issuer that answered a
	// demand-token request not-OK.
	ErrTokenAcquire = errors.New("node: could not gather enough publish-token signatures")
	// ErrCreditIssuerKeyUnknown means this client holds NO CACHED
	// publish-credit issuer key for that validator, so the prepaid-credit
	// request never left the client. It is the credit lane's own refusal and
	// it is deliberately NOT ErrTokenAcquire: one sentinel covering both made
	// the composed error name itself, because cmd/silt joins the first mint
	// cause onto the acquisition error and both legs were the same object ken
	// signatures", which is the outer sentence repeated.
	//
	// WHY NOT REUSE ErrNoIssuerKey below, and the reason is mechanical rather than
	// semantic: cmd/silt's demandKeyResolutionError carries a LIVE
	// `errors.Is(keyErr, ErrNoIssuerKey)` arm that emits the operator-facing "NOT
	// banked" text ("that server serves no demand issuer key — it is not running
	// -accept-delivery-receipts"), and cmd/silt/notbanked_test.go pins
	// That classification. Feeding a publish-credit cache miss into that
	// sentinel would put a second, unrelated condition under a classification a
	// test pins — "start -accept-delivery-receipts", which is no remedy for a
	// missing cached key. STATED AT ITS TRUE STRENGTH: that classifier reads
	// only the error FetchDemandIssuerKeys returns, so the misclassification
	// would be LATENT today, not immediate. That is the shape that bites later.
	// (An earlier version of this comment said ErrNoIssuerKey is "a peer's
	// answer over the wire" and that this one never reaches the wire. That was
	// FALSE and it was PUBLISHED to the changelog: measured, ErrNoIssuerKey has
	// SEVEN return sites and FIVE of them never reach the wire — the local
	// keyset/epoch-key cache misses in demandkeys.go and relaytransport.go, and
	// demandrole.go's issuerPub == nil, which is structurally the same predicate
	// this sentinel splits off. ErrNoIssuerKey was ALREADY a sentinel with two
	// jobs.)
	ErrCreditIssuerKeyUnknown = errors.New("node: no cached publish-credit issuer key for this validator — fetch the canonical issuer set first")
	// ErrNoIssuerKey means NO ISSUER KEY IS AVAILABLE to blind against, from either
	// direction: the peer answered the key request and serves none (FetchIssuerKey,
	// FetchDemandIssuerKeys — the two sites that reach the wire), or this client
	// holds no pinned keyset, no key for the current epoch, or was handed a nil key
	// by its parent (AcquireDemandTokenInWindow, AcquireRelayAnchors,
	// AcquireDemandTokenWithCredit — the five that do not). THE SCOPE IS STATED
	// THIS WAY BECAUSE THE NARROWER SENTENCE WAS WRONG: this comment used to say
	// only the first, and five of its own seven call sites contradicted it.
	//
	// EXPORTED because the client has to tell it apart from "the issuer served
	// keys, none resolved against a committed binding": the first is "that server
	// does not run the lane", the second is "that server's chain has no era-4
	// binding yet", and cmd/silt owes the operator a different sentence for each
	// (S5, cmd/silt/swarm.go). That classifier is why a new condition gets a new
	// sentinel instead of this one.
	ErrNoIssuerKey = errors.New("node: peer has no issuer key")

	// ErrNoChainToResolve: the issuer served demand keys, but this node holds no chain
	// replica, so none of them can be checked against the committed E ↦ key_E binding
	// and none is pinned. It is distinct from "keys served, none on-commitment": that
	// is a fact about the issuer's chain, this one is a fact about the caller.
	ErrNoChainToResolve = errors.New("node: no chain replica to resolve issuer keys against")

	errNoCanonicalIssuers = errors.New("node: peer served no canonical issuer set (no chain)")

	// errNoTokenIssuer refuses a CREDIT-BEARING request on a node that runs no
	// publish issuer. There is no key to verify the credit against, so the credit
	// cannot be honoured — fail closed. Before this guard the verify call
	// dereferenced a nil issuer ((*blindtoken.Issuer).Public reads i.key
	// unconditionally), so one crafted credit-bearing request from any peer
	// crashed the node. Reachable on the demand lane, where the handler gates on
	// the DEMAND issuer for the epoch and not on the publish issuer.
	errNoTokenIssuer = errors.New("node: no publish token issuer — cannot verify an attached credit")
	// errCreditStoreForeignKey refuses to boot on a creditspent.log written under a
	// different publish key (F2): the file and the key rotate together.
	errCreditStoreForeignKey = errors.New("node: credit-spent guard file belongs to another publish key")

	// errCreditRefused means a credit was presented but does not verify, or is
	// already spent. The requester meant to spend a credit; do not silently
	// charge the durable identity instead.
	errCreditRefused = errors.New("node: attached publish credit is invalid or already spent")

	// ErrDemandEpochMismatch refuses a demand-token reply signed for an epoch
	// the withdrawal did not name ((b1)). The issue epoch is inside the
	// blind-signed message, so such a signature unblinds to nothing redeemable
	// — failing here makes an issuer's attempt to hand a cohort a different
	// key a DENIAL the fetcher sees, not a tagged token it discovers is
	// worthless at redemption.
	ErrDemandEpochMismatch = errors.New("node: demand-token reply names a different issue epoch than the withdrawal")
)

// EnableTokenIssuer makes this validator blind-sign publish-token requests
// (charging the fee to each requester) and serve its issuer public key to
// peers who ask (MsgGetIssuerKey).
//
// rng is the randomness the PRIVATE-KEY operation blinds with. This is
// a network-facing signing oracle over attacker-chosen input, which is the
// Brumley-Boneh remote-timing setting, so the blinding is not optional. It is injected
// like every other randomness source in core (internal/depcheck bans crypto/rand
// here); it does not change the signature produced, only the modexp's timing profile.
func (n *Node) EnableTokenIssuer(rng io.Reader, key *rsa.PrivateKey) {
	n.tokenIssuer = blindtoken.NewIssuer(rng, key)
	n.issuerKeyDER = blindtoken.MarshalPub(&key.PublicKey)
}

func (n *Node) answerIssuerKey() ports.Message {
	return ports.Message{Kind: ports.MsgIssuerKeyReply, Data: n.issuerKeyDER, OK: len(n.issuerKeyDER) > 0}
}

// FetchIssuerKey asks v for its token-issuer public key and caches it, so this
// node can blind against it (as a publisher) or verify its token signatures
// (as a validator).
func (n *Node) FetchIssuerKey(v ports.NodeID, done func(error)) {
	n.request(v, ports.Message{Kind: ports.MsgGetIssuerKey}, func(resp ports.Message, err error) {
		switch {
		case err != nil:
			done(err)
		case !resp.OK || len(resp.Data) == 0:
			done(ErrNoIssuerKey)
		default:
			pub, perr := blindtoken.ParsePub(resp.Data)
			if perr == nil {
				n.peerIssuerKeys[v] = pub
			}
			done(perr)
		}
	})
}

// IssuerKeyOf returns v's token-issuer public key — this node's own, or one
// cached from a FetchIssuerKey. Used as the chain's issuerKey lookup and as the
// publisher's blind-against key.
func (n *Node) IssuerKeyOf(v ports.NodeID) *rsa.PublicKey {
	if v == n.id && n.tokenIssuer != nil {
		return n.tokenIssuer.Public()
	}
	return n.peerIssuerKeys[v]
}

// tokenIssuedEntry is one cached issuance for the retry-dedup window.
type tokenIssuedEntry struct {
	sig    []byte
	expiry ports.Time
}

// maxTokenIssued bounds the issuance-dedup cache; past this, expired entries
// are swept before inserting (same idiom as maxDeadHolders).
const maxTokenIssued = 4096

// tokenDedupTTL is how long an issuance stays replay-safe: the requester's
// transport-level retry window (attempts × per-attempt timeout, plus the
// decaying backoffs between them), doubled for headroom. Derived from the
// transport config — never a fixed constant.
func (n *Node) tokenDedupTTL() ports.Duration {
	window := n.cfg.RequestTimeout * ports.Duration(n.cfg.RequestRetries+1)
	for a := 0; a < n.cfg.RequestRetries; a++ {
		window += n.cfg.RequestBackoff << a
	}
	return 2 * window
}

// answerTokenRequest blind-signs a token request, IDEMPOTENTLY: signing is
// deterministic RSA-FDH, so re-presenting the same blinded serial must yield
// the same signature with the fee charged ONCE. The transport layer re-sends
// msg.Data verbatim on a retry (a lost reply is indistinguishable from a lost
// request), so without this dedup a retry double-charged the legacy fee — and
// on the prepaid-credit path was REFUSED as a credit double-spend, failing the
// whole gather. The dedup key is the blinded-serial hash: issuer-local,
// high-entropy (fresh blinding factor per issuer), and it reveals nothing the
// issuer did not already see.
func (n *Node) answerTokenRequest(from ports.NodeID, msg ports.Message) ports.Message {
	reply := ports.Message{Kind: ports.MsgTokenReply}
	if n.tokenIssuer == nil || len(msg.Data) == 0 {
		return reply // not an issuer / nothing to sign → OK=false
	}
	sum := sha256.Sum256(msg.Data)
	key := string(sum[:])
	now := n.clock.Now()
	if e, ok := n.tokenIssued[key]; ok && now < e.expiry {
		reply.Data = e.sig
		reply.OK = true
		return reply // a retry of an issuance already settled: same sig, no new charge
	}
	charge, err := n.tokenChargeFor(from, msg.Credit)
	if err != nil {
		n.logCreditGuardRefusal(err) // F1: a full / broken / unloaded guard is loud on the issuer
		return reply                 // no issuer to verify against, or the credit is invalid/spent
	}
	blindSig, err := n.tokenIssuer.Issue(charge, msg.Data)
	if err != nil {
		return reply // e.g. insufficient credit → OK=false, no token
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
	reply.OK = true
	return reply
}

// tokenChargeFor returns the settlement closure for a token request, or a typed
// refusal error if the request must not be served. The fee-decoupling is purely ADDITIVE (M0 privacy
// D3 / F4): if the request carries a valid, unspent prepaid credit, that credit
// is SPENT and the requester's durable identity is NOT charged — severing the
// per-publish fee link. With no credit attached the legacy path charges the
// durable identity, so every existing flow is unchanged. A credit that is
// present but invalid or already spent is refused (nil) rather than silently
// charged — the requester meant to spend a credit. Domain separation
// (VerifyCredit) means a credit can never double as a publish token.
func (n *Node) tokenChargeFor(from ports.NodeID, credit *ports.PublishCredit) (func() error, error) {
	if credit == nil {
		return func() error {
			if n.ledger != nil {
				return n.ledger.ChargePublish(from) // legacy: charges the durable identity
			}
			return nil
		}, nil
	}
	if n.tokenIssuer == nil {
		return nil, errNoTokenIssuer // no key to verify the credit against — fail closed
	}
	if len(credit.Serial) != blindtoken.SerialSize || !blindtoken.VerifyCredit(n.tokenIssuer.Public(), credit.Epoch, credit.Serial, credit.Sig) {
		return nil, errCreditRefused // a credit was presented but does not verify
	}
	if n.creditStore != nil && !n.creditLoaded {
		// A guard that does not yet know what it already spent must not spend.
		return nil, errCreditGuardUnloaded
	}
	now := creditEpochAt(n.clock.Now())
	n.sweepCreditSpent(now)
	if !creditEpochSpendable(credit.Epoch, now, n.creditFloor) {
		return nil, errCreditExpired
	}
	key := creditSpentKey(credit.Epoch, credit.Serial)
	if n.creditSpent[key] {
		return nil, errCreditRefused // double-spend
	}
	if len(n.creditSpent) >= maxCreditSpent {
		return nil, errCreditGuardFull // refuse, never evict (see maxCreditSpent)
	}
	return func() error {
		// Spend on settlement: Issue runs this closure AFTER the input check and
		// BEFORE SignBlinded, so a rejected spelling costs nothing, and the durable
		// record lands before any signature exists. A crash between the append and
		// the signature burns the credit without issuing a token — the under-issue
		// direction (a lost fee, never a mint), the same direction the paid-serial
		// guard chose. (The earlier comment here, "a signing failure does not burn
		// the credit", was false: the charge precedes the signature.)
		if n.creditStore != nil {
			if err := n.creditStore.Append(ports.PaidSerial{Serial: credit.Serial, Server: n.creditGuardOwner(), Epoch: credit.Epoch}); err != nil {
				return fmt.Errorf("%w: %w", errCreditStore, err) // not marked: the requester retries once the store heals
			}
		}
		n.creditSpent[key] = true
		return nil
	}, nil
}

// The credit-spent guard is DURABLE.
//
// creditSpent used to be process memory. A publish credit carries no epoch
// (silt/blindcredit/fdh/v1 blinds the serial only), so its validity is the persisted
// publish key's lifetime — unbounded — while the guard's memory ended at the next
// restart. Every held credit re-opened for a second spend per restart: one 50,000
// burn, two demand tokens with distinct serials, two conserved payouts (measured by
// it). The durable paid-serial guard cannot catch it: it keys on the TOKEN serial
// and the replay yields a fresh token.
//
// The store is a SECOND guardstore.Disk (creditspent.log) behind the unchanged
// ports.PaidSerialStore — never a namespace in paidserials.log, whose Compact keeps
// only the ledger's live paid serials and would evict every credit record at the
// first epoch sweep. Record shape: Serial = credit serial, Epoch = the credit's epoch,
// Server = the publish key's owner; plus one floor record (creditFloorSerial).
//
// CREDITS EXPIRE, SO THE SET SWEEPS. A credit's epoch is signed into it (blindtoken
// creditDomain v2), and an issuer accepts a credit only while its epoch is the current
// credit epoch or the one before (one ahead is allowed for clock skew). A credit epoch
// is creditEpochLength of the issuer's own clock, so a credit stays spendable for one
// to two of them. When the epoch advances, the issuer raises its floor to the oldest
// spendable epoch, drops every record below it, and compacts the log with the floor
// written into it. A record leaves the set only once its credit can no longer be
// spent, so the sweep never re-opens a double spend.
//
// THE FLOOR IS DURABLE AND ONLY RISES. Spendability is judged against the clock, and
// a clock that moves backwards would make swept epochs spendable again with their
// records gone. The floor record makes the sweep's own judgement outlive the clock:
// no credit below the floor is accepted, whatever the clock says later, including
// after a restart.
//
// PRIVACY. The issuer sees a spent credit's epoch, so a credit hides among the credits
// minted for the same epoch rather than among all credits ever minted.
// creditEpochLength sets that trade.
//
// The cap below remains, now a bound on two epochs' spends rather than on all time:
// an issuer that has recorded maxCreditSpent live spends refuses further credit-bearing
// requests (errCreditGuardFull) until the next sweep, and keeps serving credit-free
// ones. Refusal is the under-issue direction; eviction would re-open the hole one
// record at a time.
var (
	// errCreditStore: the durable append failed. The credit is NOT marked spent and
	// no token is signed; the requester can retry once the store heals.
	errCreditStore = errors.New("node: credit-spent store write failed — credit not spent, no token issued")
	// errCreditGuardFull: the durable set holds maxCreditSpent records. Refuse the
	// new credit; never evict a recorded one.
	errCreditGuardFull = errors.New("node: credit-spent guard full — refusing credit-bearing requests (no eviction)")
	// errCreditGuardUnloaded: a store is attached but LoadCreditSpent has not
	// succeeded. Credit-bearing requests are refused; credit-free ones are served.
	errCreditGuardUnloaded = errors.New("node: credit-spent guard attached but not loaded")
	// errCreditExpired: the credit verifies, but its epoch is outside the spending
	// window, or below the floor of epochs already swept.
	errCreditExpired = errors.New("node: publish credit epoch is not spendable now")
)

// creditEpochLength is the length of one credit epoch. A credit is spendable through
// its own epoch and the next, so for two to four weeks after it is minted, and it
// hides among the credits minted in the same fortnight.
const creditEpochLength = 14 * 24 * 3600 * ports.Second

// creditEpochAt is the credit epoch containing t. The daemon's clock counts from the
// Unix epoch, so credit epochs are fixed fortnights of UTC.
func creditEpochAt(t ports.Time) uint64 {
	if t < 0 {
		return 0
	}
	return uint64(t) / uint64(creditEpochLength)
}

// creditEpochSpendable reports whether a credit of epoch e may be spent at credit
// epoch now, given the floor of epochs already swept.
func creditEpochSpendable(e, now, floor uint64) bool {
	return e >= floor && e+1 >= now && e <= now+1
}

// creditSpentKey keys the spent set by epoch and serial: a credit is the signature
// over both, so the same serial under two epochs is two credits, each paid for.
func creditSpentKey(epoch uint64, serial []byte) string {
	k := make([]byte, 8, 8+len(serial))
	binary.BigEndian.PutUint64(k, epoch)
	return string(append(k, serial...))
}

// creditFloorSerial marks the floor record in creditspent.log. It is shorter than
// any credit serial (spending requires exactly blindtoken.SerialSize bytes), so no
// credit can be mistaken for it.
var creditFloorSerial = []byte("silt/credit-spent/floor")

// sweepCreditSpent raises the floor to the oldest epoch still spendable at now and
// drops the records below it, compacting the log first. If the compaction fails the
// floor stays where it was and nothing is dropped: the window check still refuses
// the expired credits, and the next spend retries the sweep.
func (n *Node) sweepCreditSpent(now uint64) {
	if now == 0 || now-1 <= n.creditFloor {
		return
	}
	floor := now - 1
	if n.creditStore != nil {
		owner := n.creditGuardOwner()
		live := []ports.PaidSerial{{Serial: creditFloorSerial, Server: owner, Epoch: floor}}
		for k := range n.creditSpent {
			if e := binary.BigEndian.Uint64([]byte(k[:8])); e >= floor {
				live = append(live, ports.PaidSerial{Serial: []byte(k[8:]), Server: owner, Epoch: e})
			}
		}
		if err := n.creditStore.Compact(live); err != nil {
			n.logf(ports.LogWarn, "credit-spent sweep: compaction failed; expired records kept until the next spend", "err", err)
			return
		}
	}
	for k := range n.creditSpent {
		if binary.BigEndian.Uint64([]byte(k[:8])) < floor {
			delete(n.creditSpent, k)
		}
	}
	n.creditFloor = floor
}

// maxCreditSpent is the guard's record cap. A cap at MINT is impossible: the issuer
// cannot count outstanding blind credits.
//
// Every publish spends one credit at each issuer whose signature its token carries,
// so the cap is also how many credit-paid publishes an issuer can ever sign for: the
// network as a whole stops accepting them after about issuers × cap / quorum. At
// 65,536 a four-validator network reached it in under an hour of field load. 2^20
// holds a million records in about 89 MB of memory and 78 MB of log per issuer, which
// covers weeks at tens of thousands of publishes a day. It postpones the cliff; it
// does not remove it. Removing it needs credits that expire, so the set can sweep.
const maxCreditSpent = 1 << 20

// SetCreditSpentStore attaches the durable credit-spent store and marks the guard
// UNLOADED. Until LoadCreditSpent succeeds every credit-bearing token request is
// refused (errCreditGuardUnloaded); credit-free requests are unaffected.
func (n *Node) SetCreditSpentStore(s ports.PaidSerialStore) {
	n.creditStore = s
	n.creditLoaded = false
}

// LoadCreditSpent restores the guard from its store: the RESTORE half of "a restart
// is not an eviction". The daemon calls it before the node serves anything. A store
// holding more than maxCreditSpent records is a refuse-to-start error, not a
// truncation: dropping the surplus is the eviction the guard exists to prevent, and
// no node writing through this guard can produce such a file.
func (n *Node) LoadCreditSpent() error {
	if n.creditStore == nil {
		return nil
	}
	entries, err := n.creditStore.Load()
	if err != nil {
		return err
	}
	fresh := make(map[string]bool, len(entries))
	var floor uint64
	for _, e := range entries {
		if len(e.Serial) == 0 {
			continue
		}
		if bytes.Equal(e.Serial, creditFloorSerial) {
			floor = max(floor, e.Epoch)
			continue
		}
		fresh[creditSpentKey(e.Epoch, e.Serial)] = true
	}
	if len(fresh) > maxCreditSpent {
		return fmt.Errorf("node: persisted credit-spent guard holds %d entries, cap is %d", len(fresh), maxCreditSpent)
	}
	// The file is BOUND to the publish key it was written under: the only recovery from a
	// full guard is rotating the publish key AND clearing this file together, and a file
	// that outlives its key would re-open F-4 for every credit under the still-valid key. A
	// record written under another key refuses the boot by name; an unbound record (zero
	// Server) is tolerated (test-padded or pre-binding files — none exist in the field).
	owner := n.creditGuardOwner()
	for _, e := range entries {
		if e.Server != (ports.NodeID{}) && e.Server != owner {
			return fmt.Errorf("%w: creditspent.log was written under another publish key (%x…); rotate-and-clear go together", errCreditStoreForeignKey, e.Server[:4])
		}
	}
	n.creditSpent = fresh
	n.creditFloor = floor
	n.creditLoaded = true
	return nil
}

// AcquireCredits mints `count` prepaid publish credits from issuer `v` (a normal,
// fee-charged token request, blinded in the CREDIT domain), so the fee is paid
// now, in bulk, decoupled from any later publish. The returned credits are spent
// one-per-token at publish time (AcquireToken with a credit attached) with no
// further debit. rng is injected; issuerPub gives v's issuer public key.
func (n *Node) AcquireCredits(rng io.Reader, v ports.NodeID, count int,
	issuerPub func(ports.NodeID) *rsa.PublicKey, done func([]ports.PublishCredit, error)) {

	pub := issuerPub(v)
	// The two refusals are SPLIT because they are different diagnoses. No cached issuer key is
	// the operator-visible one — it is what a client that never fetched the canonical issuer set
	// hits, and it is the cause cmd/silt reports when a publish is refused. Self-issuer and a
	// non-positive count are programming errors and keep the count sentinel.
	if pub == nil {
		done(nil, ErrCreditIssuerKeyUnknown)
		return
	}
	if v == n.id || count <= 0 {
		done(nil, ErrTokenAcquire)
		return
	}
	var credits []ports.PublishCredit
	var next func(i int)
	next = func(i int) {
		if i >= count {
			done(credits, nil)
			return
		}
		serial, err := blindtoken.NewSerial(rng)
		if err != nil {
			done(nil, err)
			return
		}
		epoch := creditEpochAt(n.clock.Now())
		blinded, secret, err := blindtoken.BlindCredit(rng, pub, epoch, serial)
		if err != nil {
			done(nil, err)
			return
		}
		n.request(v, ports.Message{Kind: ports.MsgTokenRequest, Data: blinded},
			func(resp ports.Message, err error) {
				if err == nil && resp.OK && len(resp.Data) > 0 {
					// RFC 9474 §4.4 Finalize: a credit that does not
					// verify under the issuer's own key is dropped here, not carried
					// forward as a credit that fails at spend time.
					if sig, uerr := blindtoken.UnblindCredit(pub, epoch, serial, resp.Data, secret); uerr == nil {
						credits = append(credits, ports.PublishCredit{Serial: serial, Sig: sig, Epoch: epoch})
					}
				}
				next(i + 1)
			})
	}
	next(0)
}

// AcquireToken collects blind signatures from up to len(validators) distinct
// issuers (never this node itself) until it has k, assembling an unlinkable
// publish token for serial. The requester's durable identity is charged the fee
// at each issuer. rng is injected (core touches no ambient randomness);
// issuerPub gives each validator's issuer public key.
func (n *Node) AcquireToken(rng io.Reader, serial []byte, validators []ports.NodeID,
	issuerPub func(ports.NodeID) *rsa.PublicKey, k int, done func(*ports.PublishToken, error)) {
	n.acquireToken(rng, serial, validators, issuerPub, nil, k, done)
}

// AcquireTokenWithCredits is the fee-decoupled acquisition (M0 privacy D3 / F4):
// each issuer's signature is paid for by SPENDING a prepaid credit from credits[v]
// (minted earlier via AcquireCredits) instead of charging the durable identity,
// so no per-publish fee links the standing key to this publish. An issuer for
// which no credit is held is skipped. Combine with an ephemeral/relayed transport
// to also sever the network-layer link (the remaining D3 residual).
func (n *Node) AcquireTokenWithCredits(rng io.Reader, serial []byte, validators []ports.NodeID,
	issuerPub func(ports.NodeID) *rsa.PublicKey, credits map[ports.NodeID]ports.PublishCredit, k int,
	done func(*ports.PublishToken, error)) {
	n.acquireToken(rng, serial, validators, issuerPub, func(v ports.NodeID) *ports.PublishCredit {
		if c, ok := credits[v]; ok {
			return &c
		}
		return nil
	}, k, done)
}

// acquireToken is the shared collector. credit, if non-nil, supplies the prepaid
// credit to attach for each issuer; a nil credit lookup (or a nil result) means
// the legacy fee-at-request path. When a credit lookup is given, an issuer with
// no credit is skipped rather than charged.
//
// TRANSPORT IS PARALLEL; SELECTION STAYS CANONICAL (research stamp 2026-08-13,
// A1). The k round-trips fire CONCURRENTLY at the first k eligible validators
// in the caller's order — which swarm.go has already ranked network-canonically
// (R-3) — and the accepted signer set is always the canonical prefix of the
// validators that turn out to be live: on a definitive per-issuer failure
// (transport retries exhausted, or a refusal) the collector FALLS FORWARD to
// the next validator in canonical order, exactly as the sequential gather did.
// The forbidden shape is first-k-of-N-to-reply, which would make the token's
// revealed signer set a function of the publisher's network position — a
// positional fingerprint re-opening the R-3 leak. Two consequences here:
// - completion waits for every in-flight canonical target to RESOLVE
// (a slow issuer is waited out, never raced past), and
// - Sigs are assembled in canonical order, not arrival order, so the token's
// structure cannot leak the arrival permutation either.
func (n *Node) acquireToken(rng io.Reader, serial []byte, validators []ports.NodeID,
	issuerPub func(ports.NodeID) *rsa.PublicKey, credit func(ports.NodeID) *ports.PublishCredit, k int,
	done func(*ports.PublishToken, error)) {

	if k <= 0 {
		done(&ports.PublishToken{Serial: serial}, nil)
		return
	}
	start := n.clock.Now()
	sigs := make(map[int][]byte) // canonical index → unblinded signature
	inflight := 0
	nextIdx := 0 // next canonical candidate not yet fired at
	finished := false

	settle := func() {
		if finished {
			return
		}
		if len(sigs) >= k {
			finished = true
			idxs := make([]int, 0, len(sigs))
			for i := range sigs {
				idxs = append(idxs, i)
			}
			sort.Ints(idxs)
			tok := &ports.PublishToken{Serial: serial}
			for _, i := range idxs {
				tok.Sigs = append(tok.Sigs, ports.TokenSig{Validator: validators[i], Sig: sigs[i]})
			}
			n.logf(ports.LogDebug, "token gather complete", "k", k,
				"fired", nextIdx, "elapsed_ms", int64(n.clock.Now()-start)/int64(ports.Millisecond))
			done(tok, nil)
			return
		}
		if inflight == 0 && nextIdx >= len(validators) {
			finished = true
			done(nil, ErrTokenAcquire)
		}
	}

	var fire func()
	fire = func() {
		// Keep (accepted + in-flight) topped up to k from the canonical tail.
		for !finished && len(sigs)+inflight < k && nextIdx < len(validators) {
			i := nextIdx
			nextIdx++
			v := validators[i]
			pub := issuerPub(v)
			if pub == nil || v == n.id {
				continue
			}
			var attached *ports.PublishCredit
			if credit != nil {
				if attached = credit(v); attached == nil {
					continue // credit-backed acquisition, but none held for v
				}
			}
			blinded, secret, err := blindtoken.Blind(rng, pub, serial)
			if err != nil {
				continue
			}
			inflight++
			sent := n.clock.Now()
			n.request(v, ports.Message{Kind: ports.MsgTokenRequest, Data: blinded, Credit: attached},
				func(resp ports.Message, rerr error) {
					inflight--
					if rerr == nil && resp.OK && len(resp.Data) > 0 {
						// RFC 9474 §4.4 Finalize.
						if sig, uerr := blindtoken.Unblind(pub, serial, resp.Data, secret); uerr == nil {
							sigs[i] = sig
						}
					}
					elapsed := int64(n.clock.Now()-sent) / int64(ports.Millisecond)
					if _, ok := sigs[i]; ok {
						n.logf(ports.LogDebug, "token gather leg", "issuer", v, "rank", i, "elapsed_ms", elapsed)
					} else {
						n.logf(ports.LogDebug, "token gather leg FAILED", "issuer", v, "rank", i,
							"elapsed_ms", elapsed, "err", rerr)
					}
					fire() // a failure frees a slot → fall forward in canonical order
					settle()
				})
		}
		settle() // covers "no eligible candidates at all" and the k<=len cases
	}
	fire()
}

// creditGuardOwner is the fingerprint the credit-spent guard file is bound to: the
// SHA-256 of this node's publish issuer public key (zero when no issuer is enabled).
// Records carry it in the Server slot of ports.PaidSerial (no other use on this lane).
func (n *Node) creditGuardOwner() ports.NodeID {
	if n.tokenIssuer == nil {
		return ports.NodeID{}
	}
	return ports.NodeID(sha256.Sum256(x509.MarshalPKCS1PublicKey(n.tokenIssuer.Public())))
}

// logCreditGuardRefusal makes the three guard-state refusals visible on the ISSUER: a
// credit refused because the durable guard is full, broken, or not yet loaded is an
// operator condition, not a client error; without this line the only symptom of an
// issuer at cap was a DEBUG line on the client. A full guard clears itself when the
// next credit epoch sweeps the oldest spends. An expired credit is the client's
// condition, so it logs at debug only.
func (n *Node) logCreditGuardRefusal(err error) {
	switch {
	case errors.Is(err, errCreditGuardFull):
		n.creditGuardRefusals++
		n.logf(ports.LogWarn, "publish credit refused: credit-spent guard FULL — the spends of the current credit window reached the cap; credits are refused until the next credit epoch sweeps the oldest", "recorded", len(n.creditSpent), "cap", maxCreditSpent, "floor", n.creditFloor)
	case errors.Is(err, errCreditExpired):
		n.logf(ports.LogDebug, "publish credit refused: its epoch is outside the spending window", "floor", n.creditFloor)
	case errors.Is(err, errCreditStore):
		n.creditGuardRefusals++
		n.logf(ports.LogWarn, "publish credit refused: credit-spent store write failed", "err", err)
	case errors.Is(err, errCreditGuardUnloaded):
		n.creditGuardRefusals++
		n.logf(ports.LogWarn, "publish credit refused: credit-spent guard attached but not loaded", "err", err)
	}
}
