// Network-facing ports: node identity, simulated time, transport, and
// the wire message vocabulary.
//
// Design note on determinism: the whole sim runs on ONE event loop.
// Nodes never block and never spawn goroutines; instead of Clock.After
// returning a channel (which invites blocking reads and scheduler
// nondeterminism), the port is AfterFunc — schedule a callback, get a
// cancel function. A wall-clock adapter can trivially implement this
// with time.AfterFunc later; the sim implements it with a priority
// queue, which is what makes same-seed runs byte-identical.
package ports

import (
	"crypto/ed25519"
	"fmt"
)

// ProviderRecord is a self-certifying "I hold content under key Key" DHT
// announcement (M0 H5 /). The provider signs (Key, its identity, expiry) with its
// own key, so a storing node — and any fetcher the record is later re-served to —
// can verify the claim was made by the provider ITSELF and is still fresh,
// instead of trusting whatever a (possibly eclipsing) intermediary hands back.
// Without this, a malicious node holding the k-closest slots to a key can
// fabricate provider records for identities that never announced. ID is the
// provider's NodeID; PubKey/Sig are nil on an UNSIGNED (legacy/trusted) record,
// which a strict verifier (RequireSignedProviders) rejects.
type ProviderRecord struct {
	Key    Hash   `cbor:"1,keyasint"`
	ID     NodeID `cbor:"2,keyasint"`
	PubKey []byte `cbor:"3,keyasint,omitempty"`
	Expiry int64  `cbor:"4,keyasint,omitempty"`
	Sig    []byte `cbor:"5,keyasint,omitempty"`
}

// ProviderSigningBytes is the domain-separated message a provider signs to claim
// a record — binding the content key, the claimant's own NodeID, and the expiry
// so a signature made for one (key, identity) can't be replayed onto another.
func ProviderSigningBytes(key Hash, id NodeID, expiry int64) []byte {
	b := make([]byte, 0, 24+len(key)+len(id)+8)
	b = append(b, []byte("silt/dht/provider/v1")...)
	b = append(b, key[:]...)
	b = append(b, id[:]...)
	var e [8]byte
	for i := 0; i < 8; i++ {
		e[i] = byte(expiry >> (8 * (7 - i)))
	}
	return append(b, e[:]...)
}

// Signed reports whether the record carries a signature at all.
func (r ProviderRecord) Signed() bool { return len(r.Sig) > 0 }

// Expired reports whether the record's freshness lease has lapsed as of now. An
// Expiry of 0 means "never expires" (an unsigned legacy record, or one minted with
// ProviderRecordTTL off) and is never expired. A provider refreshes its lease by
// re-announcing (reprovide); a departed holder that stops re-announcing lapses, so
// this is the signal that ages its record out of the dial-candidate set.
func (r ProviderRecord) Expired(now int64) bool { return r.Expiry != 0 && now > r.Expiry }

// Verify reports whether a signed record is authentic and unexpired at time now:
// the pubkey hashes to the claimed ID (so the signature is bound to that identity),
// the expiry (if set) has not passed, and the signature checks out. An UNSIGNED
// record never verifies — callers that accept legacy records must decide that
// separately (see RequireSignedProviders).
func (r ProviderRecord) Verify(now int64) bool {
	if !r.Signed() || len(r.PubKey) != ed25519.PublicKeySize {
		return false
	}
	if HashBytes(r.PubKey) != r.ID {
		return false // the signature must bind to the identity it claims to be
	}
	if r.Expired(now) {
		return false
	}
	return ed25519.Verify(ed25519.PublicKey(r.PubKey), ProviderSigningBytes(r.Key, r.ID, r.Expiry), r.Sig)
}

// NodeID identifies a node. Same 256-bit space as chunk hashes, which
// is exactly what Kademlia wants: nodes and content live in one metric
// space, and a chunk is stored/announced "near" the nodes whose IDs are
// closest to its hash.
type NodeID = Hash

// Time is nanoseconds since the sim epoch; Duration is a span of them.
// Deliberately not time.Time — core code must not touch the wall clock,
// and an int64 makes "the clock is just a number the scheduler owns"
// impossible to get wrong.
type Time int64

type Duration int64

const (
	Millisecond Duration = 1_000_000
	Second      Duration = 1000 * Millisecond
	// Year is the reference window the durability instrument g is expressed per
	// (credit-cost-of-a-shard-repair, per year). 365 days; a tuning reference,
	// not a calendar.
	Year Duration = 365 * 24 * 3600 * Second
)

func (t Time) Add(d Duration) Time { return t + Time(d) }

// Clock is injected everywhere core logic needs "later".
type Clock interface {
	Now() Time
	// AfterFunc schedules fn after d; the returned cancel is idempotent
	// and a no-op once fn has run.
	AfterFunc(d Duration, fn func()) (cancel func())
}

// MsgKind discriminates the wire messages. One flat message struct with
// a kind tag keeps the sim transport trivially copyable and keeps the
// future serialization story simple.
type MsgKind uint8

const (
	MsgFindNode          MsgKind = iota + 1 // Target: ID to search near
	MsgFindNodeReply                        // Nodes: up to k closer peers
	MsgGetProviders                         // Target: chunk hash
	MsgGetProvidersReply                    // Providers + Nodes (closer peers)
	MsgAddProvider                          // Target: chunk hash; sender announces itself
	MsgAddProviderAck
	MsgStoreChunk // ChunkID + Data: push a chunk to a peer
	MsgStoreChunkAck
	MsgFetchChunk            // ChunkID
	MsgFetchChunkReply       // Found + Data
	MsgHasChunk              // ChunkID: cheap availability probe (repair loop)
	MsgHasChunkReply         // Found
	MsgChallenge             // ChunkID + PorSeed/PorCount: prove you hold this shard of Proof.Root
	MsgChallengeReply        // Found + Proof + the opened leaves (PorOpen/PorPaths/PorBlocks)
	MsgProposeBlock          // Data: CBOR block awaiting attestation
	MsgAttestReply           // OK + Data: CBOR attestation (or OK=false refusal)
	MsgCommitBlock           // Data: CBOR block with quorum attached
	MsgCommitAck             // OK
	MsgGetChain              // Height: send me blocks from here up
	MsgChainReply            // Data: CBOR []Block
	MsgCheckReachability     // Nonce: "dial me back at my advertised address"
	MsgReachabilityReply     // Nonce: the dial-back landed (its arrival is the proof)
	MsgBondChallenge         // Nonce: prove you still hold the storage bond you advertised
	MsgBondReply             // Data: CBOR bond.Answer (empty if the bond isn't held)
	MsgTokenRequest          // Data: a blinded publish-token serial to blind-sign (fee charged to sender)
	MsgTokenReply            // Data: the blind signature; OK=false if refused
	MsgGetIssuerKey          // ask a validator for its publish-token issuer public key
	MsgIssuerKeyReply        // Data: the issuer public key (blindtoken.MarshalPub); OK=false if none
	MsgSubmitBondReg         // Data: a fresh CBOR BondReg a validator submits for a proposer to include (H2 non-proposer renewal)
	MsgSubmitBondRegAck      // OK: the renewal was received (queued if valid for the current head)
	MsgRepairClaim           // Data: a CBOR repairproof.RepairClaim — "I placed a correct rebuilt shard on Holder; verify and pay the bounty" (H7)
	MsgRepairVote            // OK: the caretaker independently verified correctness+retrievability and settled the verdict on its own ledger (H7)
	MsgDeliveryReceipt       // RETIRED: the v2 flat receipt. Kind number kept; a server answers OK=false with the named retirement (core/node handleDeliveryReceipt). Deliveries are sessions: MsgDeliveryOpen/Fund/Settle below
	MsgDeliveryReceiptAck    // OK is always false since; Data names the retirement
	MsgGetCanonicalIssuers   // ask a chain-holder for the deterministic canonical issuer set (top-k by committed bond) — publisher privacy (R-3)
	MsgCanonicalIssuersReply // Data: concatenated 32-byte NodeIDs, heaviest-bond first; OK=false if no chain
	MsgGetChainHead          // cheap chain-sync head probe: "what is your head?" — no payload
	MsgChainHeadReply        // Height: head height; Data: 32-byte head hash (so a matching head skips the full-chain fetch)
	MsgPrepareQC             // Data: CBOR prepareQCEnv: "here is the prepare-QC for (h, r) — precommit"
	MsgPrecommitReply        // OK + Data: CBOR precommit attestation (or OK=false refusal)
	MsgRoundChange           // Data: CBOR roundChangeEnv: signed "advance (h, r→r')" carrying the sender's lock
	MsgRoundChangeAck        // OK: the round-change was received and recorded
	MsgSubmitEntry           // Data: a CBOR entry a publisher submits for the designee's block to include
	MsgSubmitEntryAck        // OK: queued if valid; OK=false + Data: the synchronous refusal reason
	MsgRelayOpen             // Data: a CBOR relaypay.RelayOpen — a fetcher commits a chain root + funding + S to open a paid relay session (PoD §7.3 transport)
	MsgRelayOpenAck          // OK + Height: the relay opened the session; Height carries the session handle. OK=false + Data: the refusal reason (M0 guard / S-clamp)
	MsgRelayPay              // Data: a CBOR relaypay.RelayPay — a preimage reveal that authorizes the next forwarded increment(s) (PoD §7.3 transport)
	MsgRelayPayAck           // OK + Height: the relay advanced; Height carries the authorized increment count. OK=false if the preimage did not verify
	// per-epoch demand-issuer keys. A SEPARATE lane from MsgGetIssuerKey /
	// MsgTokenRequest, which serve the PUBLISH-token issuer key: that key is the
	// chain's issuerKey lookup and committed publish tokens are re-verified against
	// it on every replay, so it must NOT rotate per epoch. See core/node/demandkeys.go.
	MsgGetDemandIssuerKeys   // ask an issuer for its per-epoch demand-token key WINDOW {key_E: current−W <= E <= current}
	MsgDemandIssuerKeysReply // Data: CBOR demandKeysetWire (epoch → DER public key); OK=false if the peer issues no demand tokens
	MsgDemandTokenRequest    // Data: a blinded demand-token serial to blind-sign under the CURRENT epoch key (fee charged to sender, or to an attached credit)
	MsgDemandTokenReply      // Data: the blind signature; Height: the issuing epoch E (which key_E signed it); OK=false if refused
	// APPENDED, never inserted: MsgKind is a positional uint8 and an old peer's kinds must
	// keep their numbers (TestMsgKindNumbersArePinned). A validator that never wins a
	// proposal slot submits its per-epoch demand-issuer key registration to its peers, and
	// whoever proposes next folds it (the MsgSubmitBondReg shape, one keyspace over).
	MsgSubmitIssuerKeyReg    // Data: a block-CBOR wrapper carrying exactly ONE chain.IssuerKeyReg, the sender's own
	MsgSubmitIssuerKeyRegAck // OK: received (queued if valid for the receiver's head; every refusal is logged)
	// The paid DELIVERY session (APPENDED). A durable fetcher opens one session per server with a demand-domain anchor spent at OPEN,
	// tops it up with fresh anchors, and settles incrementally with cumulative-count receipts (receipt v3).
	MsgDeliveryOpen      // Data: a CBOR demand.SessionOpen — k = 1 demand-domain anchor + the durable fetcher's signature over the open commitment
	MsgDeliveryOpenAck   // OK + Height: the session handle. OK=false + Data: the refusal reason (named, never silent)
	MsgDeliveryFund      // Data: a CBOR demand.SessionFund — a top-up of an admitted session with a fresh anchor
	MsgDeliveryFundAck   // OK + Height: the session's budget after the top-up (credits). OK=false + Data: the refusal reason
	MsgDeliverySettle    // Data: a CBOR demand.SessionReceipt (receipt v3) — the fetcher's CUMULATIVE acknowledged increment count for one object
	MsgDeliverySettleAck // OK + Height: the credits this receipt settled (gross of the skim; 0 for a non-advancing count). OK=false + Data: the refusal reason
	// APPENDED, never inserted: the TRANSFERABLE round certificate — the
	// DiemBFT timeout-certificate shape, gossiped to every peer as Tendermint
	// does. A node that assembles a quorum of round-changes for (h, r) broadcasts
	// it ONCE; a receiver validates it with the same rule an attester applies to
	// a proposal-carried certificate and enters r. A wire object only — never a
	// block field, never a transition or fork-choice input (I5).
	MsgRoundCert    // Data: CBOR roundCertEnv {Height, Round, Raws}: the signed round-change envelopes for exactly Round
	MsgRoundCertAck // OK: the certificate verified and was recorded; OK=false: wrong height or below quorum
	// APPENDED, never inserted: the WITNESS seam — how a box that holds no tree asks a node
	// that does for the committed leaves, whole-set member lists, ancestor window and
	// transparency-log extension proofs it needs to validate a block. Serving is NOT a trusted
	// role: every answer is checked by the asker against a root it already holds, so a lying
	// server produces a stall and never an acceptance (core/chain WitnessProvider).
	MsgGetWitness   // Data: CBOR witnessReq — one accessor call (leaf, members, ancestors or log extension) against a named head
	MsgWitnessReply // Data: CBOR witnessResp — the answer, or OK=false when this node has no witness to serve
)

// StorageProof is a Merkle inclusion proof shipped alongside a chunk:
// "this chunk is leaf Index of the Total shards under Root". It travels
// with the chunk at store time (a host must be able to prove what it
// holds) and comes back in challenge replies. Path is bottom-up sibling
// hashes, exactly core/manifest's proof shape.
type StorageProof struct {
	Root  Hash
	Index int
	Total int
	Path  []Hash
	// Column is the shard's position within its stripe (0.n-1) for an
	// erasure-coded file — the placement/provider key is
	// hash(root‖Column) so a whole column co-locates and is discovered
	// together. -1 means "no column" (manifest chunks and uncoded
	// files), keyed by chunk id.
	Column int
	// LeafBytes is the width of one leaf of this shard's spot-check tree, as
	// the publisher committed it in the object's sealed layout. A host is
	// told the geometry rather than assuming a build constant, so a prover
	// answers about the tree its shard was actually committed under and a
	// retuned geometry cannot orphan a published object.
	//
	// It is also why the CHALLENGE does not carry a leaf width. A challenger
	// that could name one could name a single-byte leaf and make a small
	// frame cost the prover a tree over a quarter of a million leaves.
	// Zero for chunks shipped without a commitment (manifest chunks, which
	// audits do not sample).
	LeafBytes int
}

// Message is the single wire envelope. RID correlates requests with
// replies. Fields are used per-kind; unused fields stay zero.
type Message struct {
	Kind      MsgKind
	RID       uint64
	Target    Hash
	Nodes     []NodeID
	Providers []NodeID
	ChunkID   ChunkID
	Data      []byte
	Found     bool
	OK        bool
	// Proof-of-retrieval fields: Proof (the Merkle inclusion proof) rides on
	// StoreChunk so hosts can later prove which object a shard belongs to,
	// and comes back on ChallengeReply. Nonce freshens a bond challenge
	// (MsgBondChallenge).
	Proof *StorageProof
	Nonce uint64
	// PoR challenge (MsgChallenge → auditor): PorSeed expands to the sampled
	// leaf indices; PorCount is how many leaves to sample. The leaf count
	// itself is the auditor's own number, recomputed from the object's
	// committed geometry, and is echoed back as PorBlocks only so a
	// disagreement is visible rather than silently graded.
	PorSeed  []byte
	PorCount int
	// PorBase is the UNBOUND seed PorSeed was derived from, carried so the
	// PROVER can check that PorSeed is the one bound to its own identity
	// rather than computing under whatever seed it is handed. Without it an
	// honest holder is an ORACLE: a data-less identity forwards the auditor's
	// verbatim challenge, the holder proves under the forwarder's seed, and
	// the forwarder returns the answer as its own.
	//
	// It is a CHECK input, not a derivation input, and that is what lets one
	// field cover two callers: the audit path binds with core/node's
	// porProverSeed and the repair-claim retrievability leg with
	// repairproof.RepairChallengeSeed, so a prover handed a base tests its own
	// identity under both domains and answers only on a match. Deriving
	// instead would have needed a discriminator saying which.
	//
	// OPTIONAL BY DESIGN, so the change is additive on the wire. Absent, the
	// prover answers PorSeed verbatim exactly as before — an old auditor is
	// still served and an old prover still passes a new auditor's challenge,
	// since the new auditor sends both fields and the derived seed is
	// unchanged. The oracle closes for a pair where the PROVER is current,
	// which is the honest limit: this cannot fix a peer's software.
	PorBase []byte
	// PoR proof (MsgChallengeReply → prover): the opened leaves. PorOpen
	// carries one sampled leaf's bytes per entry, in the order the seed drew
	// the indices, and PorPaths the matching Merkle path — that sample's
	// sibling hashes concatenated, 32 bytes each, bottom-up. PorBlocks is
	// the prover's own leaf count.
	//
	// THE BYTES ARE THE PROOF, which is what makes this scheme keyless: a
	// prover answers only by producing shard content it holds. They are
	// ciphertext the auditor cannot read and could have fetched anyway, so
	// carrying them discloses nothing a care-link holder did not already
	// have (B4).
	//
	// These are core/por wire values; the ports layer stays crypto-agnostic
	// and never imports core/por.
	PorOpen   [][]byte
	PorPaths  [][]byte
	PorBlocks int
	// NeedBody is an attester's answer to a proposal whose heavy bond-registration
	// proofs were relayed by DIGEST and which it could not reconstruct from what it
	// already holds. It is a REFUSAL that names its own remedy: the proposer re-sends
	// the same block to this one peer with the proofs carried, and the round
	// continues.
	//
	// It is not an error and it is not a vote against the block. A plain OK=false
	// means the attester judged the block; this means it never got to. Conflating the
	// two would let a transport miss read as a validity refusal in the journal, which
	// is the attribution failure that hid the original wedge for three runs.
	NeedBody bool
	// Capacity gossip: every message from a capacity-pledging node
	// carries its current used/total, so peers accumulate a sample of
	// the network's storage for the M9 capacity estimate.
	CapUsed  int64
	CapTotal int64
	// Work gossip: the sender's own served bytes and repairs done SINCE ITS PROCESS
	// STARTED — not lifetime. keeps the credit ledger ephemeral through the RC
	// (core/credit's account map is in memory and only paid serials are restored at
	// boot), so both counters reset on every restart. A concentration measure over
	// them therefore partly measures UPTIME: a node up for a week and one up for an
	// hour differ by how long they have been running, not by how they behave. Every
	// consumer must carry that caveat; the concentration document stamps it
	// (cmd/silt/ui_economy.go, workCounterEpoch).
	//
	// It is not a mitigation either. The reset hands an observer a KNOWN ZERO
	// BASELINE, which removes the differencing step rather than adding one, and
	// because both fields are omitempty it makes every restart an unforgeable beacon
	// to every peer. It must be re-priced at the economy-ON flip that names as a
	// re-arm trigger.
	//
	// They ride the same messages as the capacity pledge so a node can
	// compute the serve-work and repair-work Gini over its local peer sample
	// without an aggregator. EXACTLY TWO FIELDS, and the tier class is NOT a third:
	// it is derived from CapTotal with published bands (core/node/economysample.go), because
	// all three are self-reported and a self-declared label buys nothing but surface.
	//
	// SELF-REPORTED, like the capacity pledge beside them: advisory sampling, never a
	// consensus input, never a standing input, never a disbursement input. Both are
	// NODE-WIDE totals with no object axis and no fetcher axis, so neither carries the
	// (fetcher x object) access record Don't #3 forbids.
	ServedBytes int64
	RepairsDone int64
	// Domain is a hash of the operator's failure-domain label (AS / rack /
	// geo / operator), gossiped so placement can spread a file's columns
	// across distinct domains, not just node IDs. 0 means "unset" — treated
	// as its own unique domain, never a grouping constraint.
	Domain uint64
	// Lease marks a StoreChunk as a demand-driven cache copy: the receiver
	// holds and serves it but lets it expire if it stops being read, so a
	// hot file fans out under load and contracts on cooldown.
	Lease bool
	// Ephemeral marks a message from a short-lived client (a `swarm add` /
	// `swarm get` publisher or fetcher that keeps nothing and then dies).
	// Receivers must NOT add such a sender to their routing table: routing
	// to a peer that will vanish poisons the table with ghosts and drowns
	// lookups in timeouts.
	Ephemeral bool
	// Height is the chain-sync cursor (MsgGetChain).
	Height uint64
	// Bond gossip: a validator advertises its storage-bond commitment root
	// and size on every message (like capacity/domain), so peers can
	// challenge it to prove it still holds real, identity-bound storage —
	// the Sybil cost behind consensus standing. A zero BondRoot means the
	// node advertises no bond and is never bond-challenged.
	BondRoot Hash
	BondSize int64
	// Credit optionally rides a MsgTokenRequest: a prepaid publish credit the
	// issuer verifies and SPENDS instead of charging the requester's durable
	// identity (M0 privacy D3 / F4). Nil ⇒ the legacy fee-at-request path. Only
	// honored when the issuer requires credits (Node.RequirePublishCredits).
	Credit *PublishCredit
	// Provider is the self-certifying record a node sends on MsgAddProvider to
	// announce itself under Target (M0 H5). Nil ⇒ the legacy self-vouch path
	// (the authenticated sender is taken as an unsigned provider). ProviderRecs
	// carries the signed records back on MsgGetProvidersReply so a fetcher can
	// verify each instead of trusting the responder's re-served NodeID list.
	Provider     *ProviderRecord
	ProviderRecs []ProviderRecord
}

func (k MsgKind) String() string {
	names := [...]string{
		MsgFindNode: "FindNode", MsgFindNodeReply: "FindNodeReply",
		MsgGetProviders: "GetProviders", MsgGetProvidersReply: "GetProvidersReply",
		MsgAddProvider: "AddProvider", MsgAddProviderAck: "AddProviderAck",
		MsgStoreChunk: "StoreChunk", MsgStoreChunkAck: "StoreChunkAck",
		MsgFetchChunk: "FetchChunk", MsgFetchChunkReply: "FetchChunkReply",
		MsgHasChunk: "HasChunk", MsgHasChunkReply: "HasChunkReply",
		MsgChallenge: "Challenge", MsgChallengeReply: "ChallengeReply",
		MsgProposeBlock: "ProposeBlock", MsgAttestReply: "AttestReply",
		MsgPrepareQC: "PrepareQC", MsgPrecommitReply: "PrecommitReply",
		MsgRoundChange: "RoundChange", MsgRoundChangeAck: "RoundChangeAck",
		MsgCommitBlock: "CommitBlock", MsgCommitAck: "CommitAck",
		MsgGetChain: "GetChain", MsgChainReply: "ChainReply",
		MsgGetWitness: "GetWitness", MsgWitnessReply: "WitnessReply",
		MsgGetChainHead: "GetChainHead", MsgChainHeadReply: "ChainHeadReply",
		MsgCheckReachability: "CheckReachability", MsgReachabilityReply: "ReachabilityReply",
		MsgSubmitBondReg: "SubmitBondReg", MsgSubmitBondRegAck: "SubmitBondRegAck",
		MsgSubmitEntry: "SubmitEntry", MsgSubmitEntryAck: "SubmitEntryAck",
		MsgRepairClaim: "RepairClaim", MsgRepairVote: "RepairVote",
		MsgDeliveryReceipt: "DeliveryReceipt", MsgDeliveryReceiptAck: "DeliveryReceiptAck",
		MsgGetCanonicalIssuers: "GetCanonicalIssuers", MsgCanonicalIssuersReply: "CanonicalIssuersReply",
		MsgGetIssuerKey: "GetIssuerKey", MsgIssuerKeyReply: "IssuerKeyReply",
		MsgGetDemandIssuerKeys: "GetDemandIssuerKeys", MsgDemandIssuerKeysReply: "DemandIssuerKeysReply",
		MsgDemandTokenRequest: "DemandTokenRequest", MsgDemandTokenReply: "DemandTokenReply",
		MsgSubmitIssuerKeyReg: "SubmitIssuerKeyReg", MsgSubmitIssuerKeyRegAck: "SubmitIssuerKeyRegAck",
		MsgBondChallenge: "BondChallenge", MsgBondReply: "BondReply",
		MsgTokenRequest: "TokenRequest", MsgTokenReply: "TokenReply",
		MsgRelayOpen: "RelayOpen", MsgRelayOpenAck: "RelayOpenAck",
		MsgRelayPay: "RelayPay", MsgRelayPayAck: "RelayPayAck",
		MsgDeliveryOpen: "DeliveryOpen", MsgDeliveryOpenAck: "DeliveryOpenAck",
		MsgDeliveryFund: "DeliveryFund", MsgDeliveryFundAck: "DeliveryFundAck",
		MsgDeliverySettle: "DeliverySettle", MsgDeliverySettleAck: "DeliverySettleAck",
		MsgRoundCert: "RoundCert", MsgRoundCertAck: "RoundCertAck",
	}
	if int(k) < len(names) && names[k] != "" {
		return names[k]
	}
	return fmt.Sprintf("MsgKind(%d)", k)
}

// IsReply reports whether this kind terminates a pending request.
func (m Message) IsReply() bool {
	switch m.Kind {
	case MsgFindNodeReply, MsgGetProvidersReply, MsgAddProviderAck, MsgStoreChunkAck, MsgFetchChunkReply, MsgHasChunkReply, MsgChallengeReply, MsgAttestReply, MsgCommitAck, MsgChainReply, MsgChainHeadReply, MsgBondReply, MsgTokenReply, MsgIssuerKeyReply, MsgSubmitBondRegAck, MsgSubmitEntryAck, MsgRepairVote, MsgDeliveryReceiptAck, MsgCanonicalIssuersReply, MsgPrecommitReply, MsgRoundChangeAck, MsgRelayOpenAck, MsgRelayPayAck, MsgDemandIssuerKeysReply, MsgDemandTokenReply, MsgSubmitIssuerKeyRegAck, MsgDeliveryOpenAck, MsgDeliveryFundAck, MsgDeliverySettleAck, MsgRoundCertAck, MsgWitnessReply:
		return true
	}
	return false
}

// Transport is node↔node messaging. Handlers are invoked one at a time
// (the sim guarantees single-threaded delivery); a handler must not
// block.
type Transport interface {
	Send(to NodeID, msg Message) error
	SetHandler(func(from NodeID, msg Message))
}

// PeerClass is the OBSERVED address class of a peer, as the transport saw it
// (the DHT eclipse cap keyed on the contacted-at address). Never a declared
// label: a class is a fact about a completed conversation.
type PeerClass uint8

const (
	// ClassUnverified: reply-learned, never contacted. Charged to the
	// INTRODUCER's group (Bitcoin Core's srcgroup rule).
	ClassUnverified PeerClass = iota
	// ClassDirect: a TLS-completed conversation at the peer's own address.
	ClassDirect
	// ClassRelayed: reached through a relay splice; keyed on the RELAY's
	// group, as its own class.
	ClassRelayed
)

// PeerClassifier is the OPTIONAL port through which the DHT learns the
// observed address class of a peer — never an IP. A transport that implements
// it exports, per NodeID, the (class, group) of the completed conversation it
// had with that peer: group = H(salt ‖ prefix(remoteAddr)), ONE namespace per
// prefix across classes; the salt is per process, never persisted, never on the
// wire. known=false means "no completed conversation": the DHT treats such an
// entry as unclassified, bounded by the reserve. Group 0 is exempt (loopback,
// link-local, unspecified): never capped per group.
type PeerClassifier interface {
	ClassOf(id NodeID) (class PeerClass, group uint64, known bool)
}
