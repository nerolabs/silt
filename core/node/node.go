// Package node composes store + transport + clock + DHT into a peer.
// Behaviors: answer DHT queries, store pushed chunks, serve fetches,
// announce what it holds, and retrieve files from the swarm.
//
// Concurrency model: there is none, on purpose. All of a node's code
// runs inside single-threaded scheduler callbacks (transport handler or
// timer), so there are no locks and no goroutines; long operations are
// chains of callbacks. This is what keeps the sim deterministic — and a
// future real-network adapter just has to serialize its I/O onto one
// loop to inherit the same guarantee.
package node

import (
	"context"
	"crypto/ed25519"
	"crypto/rsa"
	"errors"
	"fmt"

	"github.com/nerolabs/silt/core/blindtoken"
	"github.com/nerolabs/silt/core/bond"
	"github.com/nerolabs/silt/core/chain"
	"github.com/nerolabs/silt/core/demand"
	"github.com/nerolabs/silt/core/denylist"
	"github.com/nerolabs/silt/core/dht"
	"github.com/nerolabs/silt/core/link"
	"github.com/nerolabs/silt/ports"
)

type Config struct {
	K              int            // Kademlia bucket size
	Alpha          int            // lookup parallelism
	RequestTimeout ports.Duration // per-attempt deadline; exceeded => this attempt failed
	// RequestRetries is how many times a timed-out RPC is RE-SENT (with exponential
	// backoff from RequestBackoff) before the peer is finally given up on — evicted
	// from the routing table and negative-cached. On a jittery/lossy link a single
	// slow or dropped packet must NOT tear a good peer out of everyone's table (that
	// keeps the mesh sparse and consensus from ever forming); a valuable connection
	// is worth retrying for over an unknown-duration impairment. 0 = no retry (evict
	// on the first miss) — the deterministic sim/tests keep that; the daemon enables
	// retries. Total wait to declare a peer dead ≈ (retries+1)·RequestTimeout + Σbackoff.
	RequestRetries int
	RequestBackoff ports.Duration // base backoff between RPC retries (doubles each attempt)
	// HolderDialTimeout is a TIGHTER per-attempt deadline for speculative holder-fetch
	// dials (MsgFetchChunk/MsgHasChunk), which are NOT retried (see requestAttempt). It
	// keeps a fetch from a dead holder cheap under churn even when the general
	// RequestTimeout is generous for jittery consensus. 0 = fall back to RequestTimeout.
	HolderDialTimeout ports.Duration
	// RequestSizeFloorBytesPerSec extends a request's per-attempt deadline for a LARGE
	// outbound payload (#286). A block carrying a validator's one-time ~1.5 MB space-time
	// bond registration must cross a real WAN and be re-verified before the attester can
	// reply — which a flat RTT-scale RequestTimeout cannot cover, so a fresh quorum-2
	// genesis WEDGED cross-region while the identical block committed on a single-zone
	// quorum-1 SMOKE (the two-substrate immutable earning its keep). A tight wall-clock
	// TRANSPORT deadline is a category error: transport deadlines must be generous, the
	// security lives in the proof not the clock (docs/network-durability.md, research
	// #289). The deadline gains len(payload)/floor of transfer time at this assumed
	// worst-case WAN throughput (bounded so a pathological block can't hang forever).
	// 0 = off (flat RequestTimeout). Few-KB steady-state blocks gain ~nothing; the
	// structural close is a succinct proof (#299). Holder-fetch dials never extend.
	RequestSizeFloorBytesPerSec int64
	// MaxBondRegBytesPerBlock caps the total BYTES of bond registrations a proposer
	// embeds in ONE block (#286 Layer 2b). At a fresh multi-validator objective genesis,
	// every founding validator submits its ~1.5 MB space-time bond proof; embedding all
	// of them piles into one ~8 MB block the quorum gather cannot move + re-verify over a
	// real 3-region WAN before the round churns — the cert stalled here with regs=5 /
	// 7.9 MB. The founding set are ANCHORS (chain.launchAnchor), so genesis commits SMALL
	// on anchor attestations at zero committed bond while the deferred registrations drain
	// over the next blocks (each validator still gains real bonded weight and reaches
	// MatureValidators). A BYTE budget (not a count) is the right lever because the
	// blocker is SIZE, not number: at genesis a full ~1.5 MB proof means ~1 reg/block,
	// but small steady-state renewals (fewer label samples / smaller plots) pack many per
	// block — so an attest-only validator's renewals are NOT starved under a tight TTL
	// (a count cap would lapse them; sim/bond_renewal proves it). Default ~2 MiB keeps a
	// block within the size #313 proved gathers while fitting one full proof or many small
	// renewals. The proposer always embeds at least ONE reg even if it alone exceeds the
	// budget (never stall the queue on an oversized proof). 0 = unbounded (legacy). This
	// is the near-term unblock; the structural close is a succinct + aggregated proof
	// (#299, docs/network-durability.md §6). Proposer-side policy only — validity is
	// unchanged (a block with N regs is valid), so mixed caps across proposers are safe.
	MaxBondRegBytesPerBlock int64
	// MaxEntryBytesPerBlock caps the total BYTES of mempool ENTRIES a proposer folds
	// into one block — SEPARATE from the reg budget by design (#441 certification
	// Addition 1): a single ~1.5 MB bond reg fills the whole reg cap, so a shared
	// budget would leave the tens-of-bytes entry no room and the publish starvation
	// would reappear one layer down; and the dual — an entry flood must never crowd
	// out consensus-critical renewals. Independent budgets guarantee each stream its
	// slice; their SUM must stay WAN-gatherable (#286 L2b). At least one entry is
	// always folded (never stall the queue). 0 = unbounded. Proposer-side policy
	// only — validity is unchanged. The value is an M1 tuning knob, not a consensus
	// rule (certification caveat 3).
	MaxEntryBytesPerBlock int64
	// Replication is how many closest nodes receive each chunk at
	// distribute/repair time. With erasure coding doing the heavy
	// lifting, even 1 is viable — parity across nodes replaces copies.
	Replication int
	// RepairInterval is how often a caretaker sweeps the files it cares
	// about; RepairSlack is how many missing shards a stripe may have
	// before repair kicks in (repair when missing > slack).
	RepairInterval ports.Duration
	RepairSlack    int
	// RepairBountyBase is the base durability bounty (credits) a caretaker's
	// repair-claim proposes for one rebuilt shard, before BountyFor's rarest-
	// shard multiplier (credit.BountyFor). It funds the NEW HOLDER of the
	// rebuilt shard out of the object's own escrow (H7 §8b). 0 disables the
	// bounty economy (sims/e2e that don't fund escrow) — repair still runs, it
	// just emits no claim.
	RepairBountyBase int64
	// RepairQuorumTau is τ, the retrievability-confirmation threshold a single
	// caretaker-judge requires before releasing the bounty from its own ledger
	// (credit is per-node-local accounting: each judge settles independently, so
	// τ is that judge's own confirmation count, ≥1). Correctness is always
	// single-verifier-sufficient (a failed recompute self-attributes → slash),
	// so τ gates only the retrievability leg. Defaults to 1.
	RepairQuorumTau int
	// Domain is the operator's failure-domain label (AS / rack / geo /
	// operator). Placement spreads a file's columns across distinct
	// domains, so a whole domain failing costs a stripe as little as
	// possible. Empty = unset (each such node is its own unique domain).
	Domain string
	// Demand-responsive dispersion: a node that serves a chunk more than
	// HotThreshold times within a DemandInterval pushes FanoutReplicas
	// extra cache copies to other hosts (leased, so they expire after
	// LeaseTTL of not being read). Baseline Replication is the floor; heat
	// a temporary multiplier. HotThreshold 0 disables the whole mechanism.
	HotThreshold   int
	DemandInterval ports.Duration
	LeaseTTL       ports.Duration
	FanoutReplicas int
	// ReachabilityTimeout bounds a reachability check: how long to wait for
	// a helper's dial-back before concluding this node is behind NAT. It
	// spans a fresh outbound dial + handshake on the helper's side, so it is
	// deliberately looser than RequestTimeout.
	ReachabilityTimeout ports.Duration
	// FetchAttempts is how many times a chunk fetch re-sweeps its providers
	// when every provider failed *transiently* (a timeout or a relay-at-
	// capacity refusal, not a clean "don't have it"): the post-cap relay path
	// saturates under concurrent fan-out (#65) and its freed slots make a
	// backed-off retry succeed. FetchBackoff is the base delay, grown
	// linearly per attempt. FetchAttempts <= 1 disables the retry.
	FetchAttempts int
	FetchBackoff  ports.Duration
	// HolderCooldown is how long the fetch/repair loop skips a holder after a
	// dial to it timed out, so stale provider records to dead holders can't make
	// a churny swarm re-dial the same corpses at full RequestTimeout every
	// sweep (#226). 0 disables the negative cache (every provider re-dialed).
	HolderCooldown ports.Duration
	// BondAuditInterval is how often a validator challenges the storage
	// bonds of the validators it knows, and BondMaxAge is how long a bond
	// may go un-re-proven before its standing decays — so consensus
	// standing must be backed by *sustained* held storage (T1b, #78).
	BondAuditInterval ports.Duration
	BondMaxAge        ports.Duration
	// ChainSyncInterval is how often a validator reconciles its chain replica
	// against the validators it knows (SyncChain) — so a restarted or briefly-
	// partitioned node catches up on blocks it missed and heals forks, instead
	// of being stranded at its pre-restart height (F1). Catch-up must RETRY,
	// not fire once at boot, because peer reputation is re-earned live by bond
	// audits and isn't ready the instant the daemon starts.
	ChainSyncInterval ports.Duration
	// BootstrapRetryInterval is how often an ISOLATED node (empty routing table)
	// re-runs the Kademlia join against its original -bootstrap seeds. silt's join
	// is otherwise one-shot, so a node that started before its bootstrap target was
	// listening — or that later lost every peer to churn — would stay isolated
	// forever (empty table ⇒ no consensus, no discovery) until a manual restart
	// (#281, found on the 13-node cross-region cloud cold start). 0 disables.
	BootstrapRetryInterval ports.Duration
	// BootstrapWellConnected is the routing-table size at/above which a node is
	// considered converged. Below it, the bootstrap-retry loop keeps doing a
	// Kademlia self-lookup (bucket refresh) each interval, so a SPARSE mesh left by
	// a simultaneous cold start converges enough for consensus quorum to form.
	// Recovering from an EMPTY table (#281) is necessary but not sufficient: a node
	// can re-bootstrap to just one or two peers and stall there. The seedless boot
	// validator is the worst case — it never re-bootstraps, so without this refresh
	// it stays stuck at the single entry an incoming dial gave it and can never
	// discover the rest of the validator set. 0 disables the sparse refresh
	// (empty-table recovery still runs).
	BootstrapWellConnected int
	// BondVDFDelay is the number of sequential squarings a bond proof must
	// bind (core/vdf) — the "time" in proof-of-space-time: a prover cannot
	// answer until it has done this much non-parallelisable work over the
	// fresh challenge, so it cannot have released the pledged space and
	// re-plotted just-in-time. A tuning knob (Evolving): raise it in a real
	// deployment for a stronger elapsed-time floor; the modest default keeps
	// the deterministic sim fast. 0 disables the time binding (space-only).
	BondVDFDelay uint64
	// BondMaxAnswerLatency is the reply-deadline on a LIVE bond challenge — the
	// enforcing leg of the C1 partial-storage-recompute residual (BREAK 1, red-team
	// 2026-08-08; owned-residuals A5). A prover that deleted ε of its plot recomputes
	// the dropped blocks on demand; recomputed bytes are content-identical to stored
	// ones, so no CONTENT check (verifyLabels) can catch it — enforcement is
	// necessarily the TIME leg. Past the ~0.25 work knee of the DRSample graph the
	// recompute is a large, sequential-depth-bearing cost (measured: ~free at ε≤0.10,
	// ~100ms at ε≈0.25, seconds beyond), so a reply arriving after this deadline
	// implies a materially-short prover and earns no standing. SOFT by nature
	// (wall-clock ⇒ fastest-evaluator-sensitive + network jitter), so it is
	// calibrated with a GENEROUS margin: it deters the rational SERIAL disk-saver
	// past the knee, not a well-resourced parallel farm (that residual is
	// compute-repriced and owned — A5; the tight close is Option A / H-track). Only
	// meaningful on a real wall clock: 0 (default) = OFF, which the deterministic sim
	// and tests need (their tick clock has no wall meaning); the daemon sets a real
	// duration. Composes with BondTTL: on-chain weight lapses without live re-proof,
	// so this gate bounds even the objective weight over time.
	BondMaxAnswerLatency ports.Duration
	// MinBondBytes is the anti-release floor (M0 Sybil, red-team F1/F2): a bond
	// smaller than this earns NO standing, self or peer. The read-bound plot makes
	// a released prover recompute (memory-hard) before it can answer, but that
	// only bites if re-sealing the pledged size takes LONGER than the anti-release
	// COMPUTE window — a re-seal budget that is DELIBERATELY DECOUPLED from the
	// transport RequestTimeout (build-immutable #3/#4, docs/TENETS.md). At the
	// measured plot throughput (bond.PlotSealThroughput, ~270 MB/s) a ~2 s compute
	// window re-seals ~540 MiB, so a bond at or below that could be released and
	// recomputed just-in-time. Setting this floor above that threshold (with
	// margin) makes storing the plot the only viable strategy — and because the
	// window is compute, raising RequestTimeout for durability (#288) never moves
	// this floor, so hardening the network does not price out small validators.
	// Enforcement is the floor plus the bond-audit statistics over history, NOT a
	// single hard reply deadline (a full re-seal is a multi-second cost, orders of
	// magnitude above network jitter — immutable #3). 0 = no floor (the
	// deterministic sim uses small bonds); a real deployment sets it — the daemon
	// defaults it safely.
	MinBondBytes int64
	// BondLabelSamples is k, the number of labeling-consistency opens a bond
	// challenge carries and a verifier checks (M0 Sybil G2,
	// docs/design/m0-sybil-rebind.md): each open recomputes one block's label from
	// its opened DRSample parents, catching a prover that committed bytes it did
	// not correctly plot for its (identity, size). Against an ε-short prover the
	// soundness error is ≤ (1-ε)^k. A per-network EVOLVING knob: prover and
	// verifier must agree (like BondVDFDelay), so a deployment sets it uniformly.
	// 0 resolves to bond.DefaultLabelSamples (64) — the check is never disabled by
	// leaving it unset (safe-by-default; sims/tests inherit the full k).
	BondLabelSamples int
	// RequireSignedProviders makes this node reject provider records that are not
	// self-certifying (M0 H5 / Memo 08): on announce it stores/re-serves only
	// signed records, and a fetch discards any provider record whose signature
	// doesn't verify — so a node holding the k-closest slots to a key cannot
	// fabricate provider records for identities that never announced. Off by
	// default (legacy/sim: unsigned records flow); the daemon defaults it ON for an
	// untrusted swarm. Requires the node's signing key (SetSigner) to produce its
	// own signed records.
	RequireSignedProviders bool
	// ProviderRecordTTL, when > 0, stamps signed provider records with an expiry
	// this far ahead of the clock, so a re-served record also proves FRESHNESS
	// (an eclipsing node can't replay an ancient claim forever). 0 (default) = no
	// expiry — used by the sim/tests, where the tick clock has no wall meaning;
	// the daemon sets a real duration.
	ProviderRecordTTL ports.Duration
	// DHTDomainCap is the failure-domain diversity cap for eclipse resistance
	// (M0 H5-B / Memo 08): at most this many peers sharing one domain are kept per
	// routing bucket, AND provider records are announced to / resolved from a set
	// spread across domains — so an adversary that owns the NodeIDs closest to a
	// key but sits in a SINGLE domain (a ~$4 /24 key-surround) cannot suppress
	// discovery: the record still lands on, and is found via, honest nodes in other
	// domains. 0 (default) = off (legacy/sim; no diversity constraint).
	DHTDomainCap int
}

func DefaultConfig() Config {
	return Config{
		K: 8, Alpha: 3,
		RequestTimeout:    500 * ports.Millisecond,
		RequestRetries:    0, // sim/tests: no RPC retry (evict on first miss); the daemon enables it
		RequestBackoff:    250 * ports.Millisecond,
		HolderDialTimeout: 0, // sim/tests: holder fetches use RequestTimeout; the daemon sets a tighter one
		// 256 KB/s — a pessimistic transcontinental lossy path. A ~1.5 MB bond-registration
		// block then gains ~6 s of transport headroom over the base RequestTimeout (#286).
		RequestSizeFloorBytesPerSec: 262144,
		MaxBondRegBytesPerBlock:     2 << 20,  // #286 L2b: ~2 MiB/block stays gatherable; one full ~1.5 MB genesis proof or many small renewals
		MaxEntryBytesPerBlock:       64 << 10, // #441: entries are tens of bytes — 64 KiB admits hundreds/block; reg+entry sum stays WAN-gatherable
		Replication:                 3,
		RepairInterval:              60 * ports.Second,
		RepairSlack:                 2,
		RepairQuorumTau:             1, // per-node-local ledgers: each caretaker-judge confirms retrievability itself
		HotThreshold:                8,
		DemandInterval:              60 * ports.Second,
		LeaseTTL:                    180 * ports.Second,
		FanoutReplicas:              2,

		ReachabilityTimeout: 3 * ports.Second,
		FetchAttempts:       3,
		FetchBackoff:        200 * ports.Millisecond,
		HolderCooldown:      30 * ports.Second, // skip a timed-out holder for ~½ a repair interval before re-probing (#226)

		BondAuditInterval:      60 * ports.Second,
		BondMaxAge:             300 * ports.Second, // ~5 audit intervals unproven → standing lapses
		ChainSyncInterval:      30 * ports.Second,  // catch-up retries so a restart rejoins once bond audits re-establish peer standing
		BootstrapRetryInterval: 15 * ports.Second,  // isolated-node self-heal: re-join while the table is empty (#281)
		BootstrapWellConnected: 8,                  // keep bucket-refreshing until a node knows ~this many peers (sparse cold-start convergence)
		BondVDFDelay:           1000,               // modest; a real deployment raises it for a stronger time floor
		BondLabelSamples:       64,                 // bond.DefaultLabelSamples: labeling-consistency opens per challenge (G2)
	}
}

var ErrTimeout = errors.New("node: request timed out")

// maxDeadHolders bounds the failed-holder negative cache (#226): past this
// many entries a stamp sweeps out lapsed cooldowns before adding one, so
// ephemeral-identity churn can't grow the map without limit.
const maxDeadHolders = 4096

// maxPeerInfo bounds each peer-keyed GOSSIP cache — peerCaps (capacity estimate),
// peerBonds (bond-audit targets), peerDomains (H5-B diversity). They are written
// keyed by the sender of any non-ephemeral message, so a flood of distinct (sybil)
// NodeIDs would otherwise grow them without bound (a resident-memory DoS; the
// boundedness audit's flagged path). These are SOFT caches, not authority: an
// evicted peer's info is simply re-learned on its next gossip. Same sweep-past-
// threshold idiom as maxDeadHolders. Far above any real active-peer count, so it
// only ever engages under a sybil-ID flood.
const maxPeerInfo = 4096

// evictPeerInfoIfFull drops one arbitrary OTHER-peer entry from a peer-keyed
// gossip cache when it is full and `keep` would be a NEW key, so the map stays
// bounded (≤ maxPeerInfo) under a flood of distinct sybil NodeIDs. Skips `keep`
// so the peer we just heard from is never the one evicted. Soft-cache semantics:
// the dropped peer's info is re-learned on its next gossip. Generic over the
// value type (peerCaps/peerBonds/peerDomains carry different values).
func evictPeerInfoIfFull[V any](m map[ports.NodeID]V, keep ports.NodeID) {
	if _, ok := m[keep]; ok || len(m) < maxPeerInfo {
		return
	}
	for k := range m {
		if k != keep {
			delete(m, k)
			return
		}
	}
}

// Stats are per-node counters the sim reports on.
type Stats struct {
	QueriesSent     int // DHT + fetch requests issued
	Timeouts        int
	ChunksServed    int
	ChunksReceived  int // chunks pushed to us via StoreChunk
	BytesServed     int64
	Probes          int // shard availability checks (repair loop)
	Repairs         int // stripes repaired
	ShardsRebuilt   int // shards reconstructed and re-distributed
	RepairFailures  int // repair attempts that couldn't reconstruct (retried next sweep)
	Dispersals      int // stripes re-spread by the dispersion audit (over-concentrated in a domain)
	BlocksCommitted int // chain blocks appended to the local replica
	// H7 durability-bounty accounting (this node's own view; ledgers are
	// per-node-local). RepairClaims: claims this node emitted after placing a
	// rebuilt shard. BountiesReleased / FalseRepairSlashes: verdicts this node
	// settled as a caretaker-judge (paid the holder / slashed a false claimant).
	RepairClaims       int
	BountiesReleased   int
	FalseRepairSlashes int
	// #277 dead-peer-envelope gauges (M1 baseline — the dial-storm is where trust
	// either stays cheap or floods the network). HolderDialsSkipped: full-timeout
	// holder dials AVOIDED because the target was in the deadUntil negative cache
	// (the "dials-per-fetch/repair stays bounded" gauge — an unbounded climb means a
	// leak reopened). DeadProviderRecordsPruned: stale provider records removed when
	// a holder was confirmed dead (the lifecycle gauge — records age out, not linger).
	HolderDialsSkipped        int
	DeadProviderRecordsPruned int
	// #382 chain-sync cost gauges (M1 baseline). ChainSyncHeadMatches: sweeps where a
	// peer's head hash matched ours and the full-chain fetch + re-validate was ELIDED
	// (the steady-state win — this should dominate once the network converges).
	// ChainSyncFullFetches: sweeps that DID fetch a peer's whole chain (a real
	// difference: catch-up, reorg, or an old peer that can't answer the head probe).
	// The ratio full/(full+match) is the "chain bytes per sweep" cost signal: near 0
	// in agreement, spiking only on genuine divergence. An unbounded climb in
	// FullFetches while heads agree means the elision regressed.
	ChainSyncHeadMatches int
	ChainSyncFullFetches int
	// ChainSyncNeedCheckpoint: sweeps where a peer could not be synced from because this
	// node is behind the weak-subjectivity window and the peer pruned the gap (slice 5).
	// A nonzero value means an operator must obtain a recent -ws-checkpoint out-of-band or
	// point at an archive node — surfaced, never silent (I4/S5).
	ChainSyncNeedCheckpoint int
}

type pending struct {
	cb     func(ports.Message, error)
	cancel func()
	to     ports.NodeID
}

type Node struct {
	cfg   Config
	id    ports.NodeID
	clock ports.Clock
	tr    ports.Transport
	store ports.ChunkStore
	table *dht.Table
	provs *dht.Providers

	// bootstrapSeeds are the original -bootstrap seed IDs, kept so an isolated
	// node (empty table) can re-run the Kademlia join instead of staying stranded
	// (#281). bootstrapRetryRunning guards the self-rescheduling retry tick;
	// bootstrapRejoin, if set, fires when a retry recovers an empty table (so the
	// daemon can surface the self-heal to operators).
	bootstrapSeeds        []ports.NodeID
	bootstrapRetryRunning bool
	bootstrapRejoin       func(tableSize int)

	rid     uint64
	pending map[uint64]*pending
	Stats   Stats

	// reachable tracks peers that have answered one of our requests and
	// not since timed out — proof WE can dial THEM, not merely that they
	// reached us. Only these are persisted as warm-restart seeds, so a
	// restart re-seeds from live peers instead of reloading every dead
	// ephemeral identity we ever heard from (#43).
	reachable map[ports.NodeID]ports.Time

	// deadUntil is a negative cache of holders a fetch just failed to reach:
	// a dialer skips a holder still in cooldown instead of eating another full
	// RequestTimeout on it. Stale provider records to dead holders otherwise
	// let a churny swarm re-dial the same corpses every repair sweep, starving
	// the serial fetch loop on timeouts (#226). Stamped on any request timeout;
	// consulted only in the fetch/repair dial path so consensus/DHT re-probes
	// are unaffected. Cooldown expiry lets a recovered holder back in.
	deadUntil map[ports.NodeID]ports.Time

	// staticPeers is the configured consensus/anchor tier (-persistent-peers): peers
	// whose address is CONFIGURED, not discovered, so they are NEVER evicted from the
	// routing table on a reachability timeout — a transient WAN miss is a retry
	// situation, not an eviction. This is what lets a fresh multi-region validator set
	// form proposer-initiated quorum at genesis (no chain yet ⇒ no discoverable address
	// registry): #286 Layer 2, docs/network-durability.md §2/§8, Tendermint
	// persistent_peers. Written once at startup, read on the loop in requestAttempt.
	staticPeers map[ports.NodeID]bool

	// reachability probes (our AutoNAT): a check sends helpers a nonce and
	// waits for one to dial us back. reachProbes maps an outstanding nonce
	// to its callback; reachSeq mints nonces; reach is the last verdict.
	reachSeq    uint64
	reachProbes map[uint64]*reachProbe
	reach       Reachability
	// observedAddr is this node's public host:port as a relay reported it
	// (STUN-style, #27) — the endpoint a peer aims a hole-punch at. "" until
	// a relay registration reports one.
	observedAddr string

	// caretaker state (repair loop)
	reg           ports.Registry
	care          []link.CareHandle
	repairRunning bool

	// ledger, when set, is credited for every chunk this node serves.
	ledger ports.CreditLedger
	// freeload makes the node a pure consumer: it refuses to store
	// pushed chunks and refuses to serve fetches, while still fetching
	// and using DHT routing. Exists so the economy scenario can watch
	// leeches go broke.
	freeload bool
	// ephemeral marks this node as a short-lived client (publish/fetch that
	// keeps nothing): its outgoing messages are stamped so peers don't route
	// to it. See ports.Message.Ephemeral (#43).
	ephemeral bool
	// liar makes the node accept chunk placements, keep the PROOF, and
	// throw away the DATA — then claim to have the chunk when asked.
	// It can still answer a challenge with a valid Merkle proof (it
	// kept that), but it cannot compute the nonce tag, which is the
	// whole point of the tag. Exists so audits have someone to catch.
	liar bool
	// equivPlan holds the RESUMABLE double-sign state for the -equivocate
	// red-team drill (adversary.go). It pins the fork base ONCE and latches
	// each leg's placement, so a retry after a partial placement resumes the
	// un-placed legs instead of rebuilding at the (since-advanced) live head —
	// the #378 wedge. Test-harness only; nil on an honest node.
	equivPlan *equivPlan
	// equivServedFork is the OBJECTIVE-mode equivocation primitive's crafted
	// GetChain response (adversary.go PlaceConflictingSigned): a losing fork the
	// Byzantine node SERVES so honest peers fetch it on sync and slash the
	// double-sign on detection (chainrole.go slashEquivocators), without ever
	// committing a fork (impossible under 3-of-4). nil on an honest node and on
	// the legacy commit-based -equivocate path. Test-harness only.
	equivServedFork []chain.Block
	// eclipser models a malicious node holding the NodeIDs closest to a key (a
	// key-surround): it DROPS provider announcements and returns NO provider
	// records, so a lookup that reaches only the surrounding NodeIDs finds nothing.
	// It still answers routing (FindNode), the way a real eclipser routes you
	// deeper into its own cluster. Adversary/test seam for H5-B (cf. SetLiar).
	eclipser bool
	// shrinkLiar makes the node hold only the FIRST block of each shard, then
	// answer a challenge reporting PorBlocks=1 and proving over that single
	// block — the red-team F4 "tail-shard shrink" attack. The auditor must
	// reject it by demanding the shard's committed block count, not the
	// prover's self-report. Exists so the F4 regression exercises the real
	// answerChallenge → gradeAnswers path end to end.
	shrinkLiar bool
	// proofMeta holds the ALWAYS-RESIDENT small fields of each hosted chunk's
	// storage proof (Root, Column, ...) — enough for existence checks, the
	// iterate-all sweeps, and re-announcing coded shards under the right column
	// key, without paging. The full proof (Merkle Path + PoR tags, ~5.4 KB) lives
	// in proofs, the durable backing, and is paged in only to serve or audit — so
	// resident proof RAM is O(hot), not O(total held) (the daemon OOM fix). See
	// proofbacking.go.
	proofMeta map[ports.ChunkID]proofMeta
	// proofs is the durable, full-proof backing (#69: a restart re-announces coded
	// shards under the right key and still answers audits). Defaults to an in-core
	// in-memory store (sims/clients); a daemon injects a bounded, disk-backed store
	// (adapters/proofcache over adapters/diskproofs) via SetProofStore.
	proofs ports.ProofStore

	// capacity gossip: capRep is the store's reporter (nil if the store
	// is unbounded); peerCaps accumulates what peers report about
	// themselves, the raw material of the network capacity estimate.
	capRep   ports.CapacityReporter
	peerCaps map[ports.NodeID]capInfo

	// bond audit (T1b, #78): bond is this node's own sealed storage bond
	// (nil = none advertised); peerBonds accumulates the bond roots/sizes
	// peers gossip, which a validator challenges to make consensus standing
	// cost real held storage. See bondaudit.go.
	bond      *bond.Commitment
	peerBonds map[ports.NodeID]bondInfo
	// peerBondRTT tracks each peer's recent bond-challenge reply latencies so the
	// C1 partial-storage timing signal is the windowed-MINIMUM (low quantile) of
	// the distribution, not a single wall-clock sample — build-immutable #3: a
	// slow reply is never a hard security trip, only its sustained floor is a soft,
	// disclosed deterrent. Persists across bond re-advertisement (peerBonds is
	// replaced wholesale). See bondaudit.go latWindow.
	peerBondRTT map[ports.NodeID]*latWindow
	// bondChallengeRate caps VDF-evals served per challenger per audit window —
	// the cheap gate in front of the costly AnswerSpaceTime so an unbounded
	// challenger can't pin the single goroutine (#424). See bondaudit.go.
	bondChallengeRate map[ports.NodeID]*challengerRate
	// plotStore persists the bond plot so a restart reloads it instead of
	// re-plotting (#93); nil = memory-only (re-plots each start).
	plotStore ports.PlotStore

	// tokenIssuer, when set, makes this validator blind-sign publish-token
	// requests (T3, #14/F1) — the publisher-privacy issuance role.
	tokenIssuer *blindtoken.Issuer
	// issuer-key distribution: our own issuer pubkey (DER, served on
	// MsgGetIssuerKey) and a cache of peers' issuer keys (fetched), so tokens
	// can be blinded against and verified across the network.
	issuerKeyDER   []byte
	peerIssuerKeys map[ports.NodeID]*rsa.PublicKey
	// Prepaid publish credits (M0 privacy D3 / F4). A token request may carry a
	// prepaid credit this issuer verifies and marks spent INSTEAD OF charging the
	// requester's durable identity — so the per-publish fee no longer links the
	// standing key to the publish. The fee was charged in bulk at mint (a normal,
	// charged token request blinded in the credit domain). creditSpent is this
	// issuer's online double-spend set.
	creditSpent map[string]bool
	// tokenIssued makes token ISSUANCE idempotent under transport retries
	// (research certification 2026-08-13, A2): a lost REPLY makes the requester
	// re-present the SAME blinded serial (requestAttempt re-sends msg.Data
	// verbatim), and without dedup the issuer would charge the fee twice — or,
	// on the credit path, refuse the retry outright as a credit double-spend.
	// Keyed on the blinded-serial hash (issuer-local, high-entropy, reveals
	// nothing beyond what the issuer already saw); entries expire after the
	// transport retry window (tokenDedupTTL) and the map is swept past a cap.
	tokenIssued map[string]tokenIssuedEntry

	// D-DEMAND (#181): the witnessed-demand ledger (demandBank) a server banks
	// delivery receipts into, and the issuer public key it trusts to have signed the
	// retrieval tokens those receipts spend (demandIssuer). Nil until EnableDemandBank.
	// Demand is a NEUTRAL observable — never wired to consensus standing (γ→1/N).
	demandBank   *demand.Bank
	demandIssuer *rsa.PublicKey

	// failure-domain gossip: domainID is this node's own domain hash
	// (0 = unset); peerDomains accumulates peers' domains from gossip, so
	// placement can spread columns across distinct domains.
	domainID    uint64
	peerDomains map[ports.NodeID]uint64

	// demand-responsive dispersion: serveLoad counts recent serves per
	// chunk (decayed each demand tick); leases holds the expiry of each
	// cache copy we took on under load; demandRunning guards the tick loop,
	// which sleeps itself when there's nothing hot or leased to manage.
	serveLoad     map[ports.ChunkID]int
	leases        map[ports.ChunkID]ports.Time
	demandRunning bool

	// validator role (M12): the local chain replica and the signing key
	// (nil = not a validator).
	chain    *chain.Chain
	signer   ed25519.PrivateKey
	onCommit func(chain.Block)
	onSlash  func(culprit ports.NodeID, height uint64)
	onReorg  func(dropped int, newHeight uint64)
	// blockedPeers simulates a NETWORK PARTITION: messages to/from these peers are
	// dropped, as if the link were down. Test-harness / field-drill control (the
	// daemon's -block-peers), used to exercise partition→heal over the real wire
	// (#184); nil/empty in normal operation.
	blockedPeers map[ports.NodeID]bool
	// pendingSlashes are equivocation proofs this node detected (during Reconcile)
	// and has not yet recorded on-chain; proposeBlock drains them into Block.Slashes
	// so every replica evicts the culprit from the objective set in lockstep (F2).
	pendingSlashes []chain.Equivocation
	// pendingBondRegs are fresh bond renewals PEERS submitted (MsgSubmitBondReg)
	// that this node will fold into the next block it proposes — the NON-PROPOSER
	// renewal path (H2 / red-team RT-2). Without it a validator renews its
	// objective standing only when IT proposes, so with a bond TTL on (RT-2), an
	// attest-only validator would never renew and would lapse, dropping the
	// quorum's weight. Keyed by validator id; the latest submission wins.
	pendingBondRegs []pendingBondReg
	// pendingEntries is the ENTRY MEMPOOL (#441, certified 2026-08-16): publish
	// entries PEERS submitted (MsgSubmitEntry) that the next block this node
	// proposes will fold in — entries are block CONTENT the single (h,r)
	// designee carries, never a second proposal stream that can lose the r0
	// race (the mature-regime starvation) or sit escape-less (the launch-face
	// wedge; pendingEntries also ARMS maybeAdvanceRound). Ordered FIFO by
	// arrival and dedup'd by root — a resubmission of a queued root is a no-op
	// that keeps the original position, so resubmitting cannot queue-jump
	// (certification Addition 2: FIFO ⇒ no valid entry is starved by another).
	pendingEntries []pendingEntry
	// signMark is this validator's monotonic last-signed watermark: the height
	// and hash of the newest consensus signature it has released — PROPOSAL or
	// attestation alike, committed or not — so it NEVER signs a second,
	// different block at a signed height. An honest validator does not
	// equivocate, even if two competing proposals reach it before either
	// commits, and even if IT proposed one of them (#397: the proposer's own
	// signature counting is exactly what was missing when two renewal-aligned
	// anchors cross-attested each other's block-6 and were both slashed). When
	// signMarkStore is set (the daemon: adapters/markstore), the mark is made
	// DURABLE before each signature is released, so a crash/restart cannot wipe
	// it and let the node contradict a signature it already shipped (research
	// #397 Q1b — permanent slashing is sound only with this). Nil store (sim /
	// unit tests) keeps the mark in-memory. See chainrole.go signAllowedAt /
	// recordSign.
	signMark      ports.SignMark
	signMarkSet   bool
	signMarkStore ports.SignMarkStore
	// slashedLocal latches the local-ledger slash per culprit (#397 Q4-i): a
	// live fork is re-observed by EVERY reconcile sweep until it heals, and
	// re-detecting the same double-sign must not re-apply the ledger penalty or
	// re-log/re-fire the callback every ~2s (the field hot-loop). The on-chain
	// record keeps its own lifecycle (pendingSlashes requeues until committed).
	slashedLocal map[ports.NodeID]bool
	// chain-sync loop (F1): chainSyncSeed is the explicit attester set to
	// reconcile against (may be empty — every learned validator bond is also a
	// target), chainSyncOnCatchUp fires after a catch-up so the daemon persists,
	// and chainSyncRunning guards the self-rescheduling tick.
	chainSyncSeed      []ports.NodeID
	chainSyncOnCatchUp func(added int)
	chainSyncRunning   bool
	// bondDrainInFlight guards the #338 reactive bond-registration drain — at
	// most one drain proposal in flight per sweep cadence; drainWaitSweeps
	// counts sweeps spent deferring to the designated drain proposer before the
	// liveness fallback lets this node take over (see maybeProposeBondDrain).
	bondDrainInFlight bool
	drainWaitSweeps   int
	// rounds is the #432 per-height round/lock/view-change state (see
	// core/node/rounds.go). Lazily (re)created for the current working height
	// by roundsFor; carries the lock (highest witnessed prepare-QC) and the
	// collected round-change messages.
	rounds *heightRounds
	// denylist is the operator's local takedown list (nil = none). The
	// effective set also includes on-chain revocations, but ONLY if the
	// operator subscribed via SetHonorChainRevocations; see isDenied.
	denylist *denylist.Set
	// honorChainRevocations opts this operator in to enforcing the chain's
	// on-chain takedown records (default off — per-operator, not global; F5).
	honorChainRevocations bool

	// lg narrates through the observability port; nil = off. The sim
	// leaves it nil, so determinism and benchmarks are untouched.
	lg ports.Logger
}

type capInfo struct{ used, total int64 }

// SetLedger wires credit accounting; nil disables it.
func (n *Node) SetLedger(l ports.CreditLedger) { n.ledger = l }

// ErrNoLedger is returned by the durability-funding API when the node has no
// credit ledger wired (SetLedger was never called).
var ErrNoLedger = errors.New("node: no credit ledger configured")

// FundDurability prepays object root's durability reserve from THIS node's own
// balance — a publisher or operator endowing a repair budget so the object
// outlives churn before it is popular enough to self-fund via the serve auto-skim.
// It is a pure balance move and never touches standing; it returns
// ErrInsufficientCredit if the node can't cover the amount, and ignores a
// non-positive amount (H7).
func (n *Node) FundDurability(root ports.Hash, amount int64) error {
	if n.ledger == nil {
		return ErrNoLedger
	}
	return n.ledger.FundEscrow(root, n.id, amount)
}

// DurabilityReserve is the credit currently available to pay repair bounties for
// object root — its remaining funded horizon. Observability; reading moves
// nothing. 0 when no ledger is wired or the object has no reserve.
func (n *Node) DurabilityReserve(root ports.Hash) int64 {
	if n.ledger == nil {
		return 0
	}
	return n.ledger.EscrowBalance(root)
}

// DurabilitySnapshot reports object root's durability accounting (reserve, lifetime
// funded/paid, repair count) — the raw material for the finite-but-renewable
// instruments (credit.CostPerRepair / Horizon / G). The zero snapshot when no
// ledger is wired. Observability; reading moves nothing.
func (n *Node) DurabilitySnapshot(root ports.Hash) ports.DurabilitySnapshot {
	if n.ledger == nil {
		return ports.DurabilitySnapshot{}
	}
	return n.ledger.DurabilitySnapshot(root)
}

// SetLogger wires the observability port; nil disables it.
func (n *Node) SetLogger(lg ports.Logger) { n.lg = lg }

// logf narrates. Only rare events go through it (timeouts, repairs,
// verdicts) — nothing per-byte, so the varargs cost is irrelevant and a
// disabled logger stays effectively free.
func (n *Node) logf(lvl ports.LogLevel, event string, kv ...any) {
	ports.LogIf(n.lg, lvl, event, kv...)
}

// SetFreeload toggles leech behavior.
func (n *Node) SetFreeload(v bool) { n.freeload = v }

// SetEphemeral marks this node as a short-lived client so peers don't add
// it to their routing tables (#43).
func (n *Node) SetEphemeral(v bool) { n.ephemeral = v }

// SetLiar toggles fake-storage behavior (see the liar field).
func (n *Node) SetLiar(v bool) { n.liar = v }

// SetEclipser toggles provider-record suppression (see the eclipser field) — the
// H5-B key-surround adversary that owns the NodeIDs closest to a key.
func (n *Node) SetEclipser(v bool) { n.eclipser = v }

// SetShrinkLiar toggles the F4 tail-shrink attack (see the shrinkLiar field):
// the node keeps one block per shard and under-reports its block count.
func (n *Node) SetShrinkLiar(v bool) { n.shrinkLiar = v }

// SetObservedAddr records this node's public host:port as a relay reported it
// (#27); ObservedAddr returns it ("" until known). A NATed node hands this to a
// peer as the endpoint to aim a hole-punch at.
func (n *Node) SetObservedAddr(a string) { n.observedAddr = a }

// AddStaticPeer marks id as a configured consensus/anchor peer (see -persistent-peers):
// it is NEVER evicted from the routing table on a reachability timeout — its address is
// configured, not discovered, so a transient WAN miss is a retry situation, not an
// eviction (#286 Layer 2 Q4; docs/network-durability.md §2/§8, Tendermint persistent_peers).
// Call at startup, before consensus runs.
func (n *Node) AddStaticPeer(id ports.NodeID) {
	if n.staticPeers == nil {
		n.staticPeers = make(map[ports.NodeID]bool)
	}
	n.staticPeers[id] = true
}

// IsStaticPeer reports whether id is a configured never-evicted consensus peer.
func (n *Node) IsStaticPeer(id ports.NodeID) bool { return n.staticPeers[id] }

// ObservedAddr is this node's relay-observed public endpoint ("" if unknown).
func (n *Node) ObservedAddr() string { return n.observedAddr }

// SetProofStore swaps in a durable proof backing (#69); call before LoadProofs
// and bootstrap. A daemon passes a bounded, disk-backed store (proofcache over
// diskproofs) so resident proof RAM stays O(hot). Passing nil is a no-op — the
// node keeps its default in-memory backing (sims, ephemeral clients).
func (n *Node) SetProofStore(ps ports.ProofStore) {
	if ps != nil {
		n.proofs = ps
	}
}

// SetPlotStore attaches durable bond-plot persistence (#93); call before
// EnableBond so a restart reloads the plot instead of re-plotting. nil keeps
// the plot memory-only (re-plotted each start; fine for sims/tests).
func (n *Node) SetPlotStore(ps ports.PlotStore) { n.plotStore = ps }

// LoadProofs repopulates the resident proof METADATA from the backing store, so
// a restarted node re-announces its held coded shards under the correct column
// key (AnnounceHeld reads proofMeta) instead of their bare ids — the difference
// between a disk full of content being discoverable or invisible (#69). It reads
// one proof at a time (Keys+Get) to extract its metadata, so startup RAM stays
// O(hot), not O(total held) — the full proofs stay in the backing, paged later
// only to serve or audit. Call after New (and SetProofStore), before AnnounceHeld.
func (n *Node) LoadProofs() {
	keys, err := n.proofs.Keys()
	if err != nil {
		n.logf(ports.LogWarn, "proof reload failed", "err", err)
		return
	}
	// The scan is O(store) — a page-from-disk read per held proof — so on a large node
	// it runs for minutes. Log progress at the start and each quarter so a long scan
	// reads as "maturing N%", not a hung daemon (the operator otherwise can't tell the
	// difference from the outside). Only for a store big enough to notice.
	total := len(keys)
	progressEvery := 0
	if total >= 20000 {
		n.logf(ports.LogInfo, "reloading storage proofs (maturing)", "total", total)
		progressEvery = total / 4
	}
	loaded := 0
	for i, id := range keys {
		if progressEvery > 0 && i > 0 && i%progressEvery == 0 {
			n.logf(ports.LogInfo, "reloading storage proofs", "done", i, "total", total)
		}
		p, ok, gerr := n.proofs.Get(id)
		if gerr != nil || !ok {
			continue // an unreadable/absent proof just re-announces under its bare id
		}
		n.proofMeta[id] = metaOf(p)
		loaded++
	}
	if loaded > 0 {
		n.logf(ports.LogInfo, "reloaded storage proofs", "count", loaded)
	}
}

// dropHosted removes a chunk this node hosts, keeping the resident metadata and
// the backing proof store in sync so a delete never leaves an orphan proof behind.
func (n *Node) dropHosted(id ports.ChunkID) {
	n.store.Delete(bg(), id)
	delete(n.proofMeta, id)
	n.proofs.Delete(id)
}

func New(id ports.NodeID, cfg Config, clock ports.Clock, tr ports.Transport, store ports.ChunkStore) *Node {
	n := &Node{
		cfg:               cfg,
		id:                id,
		clock:             clock,
		tr:                tr,
		store:             store,
		table:             dht.NewTable(id, cfg.K),
		provs:             dht.NewProviders(),
		pending:           make(map[uint64]*pending),
		reachable:         make(map[ports.NodeID]ports.Time),
		deadUntil:         make(map[ports.NodeID]ports.Time),
		staticPeers:       make(map[ports.NodeID]bool),
		reachProbes:       make(map[uint64]*reachProbe),
		proofMeta:         make(map[ports.ChunkID]proofMeta),
		proofs:            newMemProofs(), // default in-mem backing; daemon injects the bounded disk-backed store
		peerDomains:       make(map[ports.NodeID]uint64),
		peerBonds:         make(map[ports.NodeID]bondInfo),
		peerBondRTT:       make(map[ports.NodeID]*latWindow),
		bondChallengeRate: make(map[ports.NodeID]*challengerRate),
		slashedLocal:      make(map[ports.NodeID]bool),
		peerIssuerKeys:    make(map[ports.NodeID]*rsa.PublicKey),
		creditSpent:       make(map[string]bool),
		tokenIssued:       make(map[string]tokenIssuedEntry),
		serveLoad:         make(map[ports.ChunkID]int),
		leases:            make(map[ports.ChunkID]ports.Time),
	}
	if cfg.Domain != "" {
		n.domainID = domainHash(cfg.Domain)
	}
	if cfg.DHTDomainCap > 0 {
		// Cap same-domain peers per routing bucket (H5-B): an adversary can't fill
		// the buckets near a key with one-domain Sybils, so honest diverse peers
		// remain routable.
		n.table.SetDiversity(n.domainOf, cfg.DHTDomainCap)
	}
	if rep, ok := store.(ports.CapacityReporter); ok {
		n.capRep = rep
		n.peerCaps = make(map[ports.NodeID]capInfo)
	} else {
		n.peerCaps = make(map[ports.NodeID]capInfo)
	}
	tr.SetHandler(n.handle)
	return n
}

// send stamps outgoing messages with our current capacity pledge — the
// gossip that lets every node estimate the network's total storage.
func (n *Node) send(to ports.NodeID, msg ports.Message) error {
	if n.blockedPeers[to] {
		return errPartitioned // simulated partition: the link to this peer is down
	}
	if n.capRep != nil {
		msg.CapUsed, msg.CapTotal = n.capRep.Capacity()
	}
	msg.Domain = n.domainID
	msg.Ephemeral = n.ephemeral
	if n.bond != nil {
		msg.BondRoot, msg.BondSize = n.bond.Root, n.bond.Size
	}
	return n.tr.Send(to, msg)
}

func (n *Node) ID() ports.NodeID        { return n.id }
func (n *Node) Table() *dht.Table       { return n.table }
func (n *Node) Store() ports.ChunkStore { return n.store }

// ReachablePeers reports the peers we've had a successful round-trip with
// and haven't since timed out — the set worth persisting as warm-restart
// seeds, as opposed to every address we've ever observed. See the
// reachable field (#43).
func (n *Node) ReachablePeers() map[ports.NodeID]bool {
	out := make(map[ports.NodeID]bool, len(n.reachable))
	for id := range n.reachable {
		out[id] = true
	}
	return out
}

// bg is the context for local store calls. The event loop has no
// cancellation semantics, so a background context is honest.
func bg() context.Context { return context.Background() }

// request sends msg expecting a reply; cb fires exactly once, with
// ErrTimeout if none arrives in time. Timeouts also evict the peer from
// the routing table — a Kademlia table must only hold live peers.
func (n *Node) request(to ports.NodeID, msg ports.Message, cb func(ports.Message, error)) {
	n.requestAttempt(to, msg, 0, cb)
}

// requestAttempt sends one try of an RPC. On timeout it RE-SENDS with exponential
// backoff up to RequestRetries times — a jittery/lossy link must not evict a good
// peer on the first slow or dropped packet — and only gives the peer up (evict +
// negative-cache) once the retries are exhausted.
// requestTimeoutFor picks the per-attempt TRANSPORT deadline for msg:
//   - a speculative holder-fetch dial (MsgFetchChunk/MsgHasChunk) gets the TIGHTER
//     HolderDialTimeout (fail fast on a dead holder under churn, no size extension);
//   - a lean consensus/mesh RPC gets the base RequestTimeout;
//   - a LARGE outbound payload (a block carrying a validator's one-time ~1.5 MB
//     space-time bond registration) gets a size-extended deadline (#286): it must
//     cross a real WAN and be re-verified before the attester can reply, which a flat
//     RTT-scale deadline cannot cover — a tight fixed transport deadline is the
//     category error that wedged quorum-2 genesis cross-region while the identical
//     block committed on a single-zone quorum-1 SMOKE. The extension is
//     len(payload)/RequestSizeFloorBytesPerSec of transfer time, capped at 30 s so a
//     pathological block can't hang the round. Few-KB steady-state blocks gain ~nothing.
//     See docs/network-durability.md; the structural close is a succinct proof (#299).
func (n *Node) requestTimeoutFor(msg ports.Message) ports.Duration {
	if msg.Kind == ports.MsgFetchChunk || msg.Kind == ports.MsgHasChunk {
		if n.cfg.HolderDialTimeout > 0 && n.cfg.HolderDialTimeout < n.cfg.RequestTimeout {
			return n.cfg.HolderDialTimeout
		}
		return n.cfg.RequestTimeout
	}
	timeout := n.cfg.RequestTimeout
	if n.cfg.RequestSizeFloorBytesPerSec > 0 {
		extra := ports.Duration(int64(len(msg.Data)) * int64(ports.Second) / n.cfg.RequestSizeFloorBytesPerSec)
		if extra > 30*ports.Second {
			extra = 30 * ports.Second
		}
		timeout += extra
	}
	return timeout
}

func (n *Node) requestAttempt(to ports.NodeID, msg ports.Message, attempt int, cb func(ports.Message, error)) {
	n.rid++
	rid := n.rid
	msg.RID = rid
	// A HOLDER-FETCH dial (MsgFetchChunk/MsgHasChunk) is speculative — stored content
	// lives on arbitrary holders that are OFTEN gone under churn — so it fails FAST on a
	// tighter deadline and does NOT retry the same holder (the fetch loop already retries
	// at a higher level via FetchAttempts and skips known-dead holders via deadUntil).
	// A too-generous timeout + retry here just deepens the dead-holder dial-storm (#277).
	// MESH/CONSENSUS RPCs get the full RequestTimeout + retries, to ride out jitter
	// without tearing a live peer out of the mesh.
	holderFetch := msg.Kind == ports.MsgFetchChunk || msg.Kind == ports.MsgHasChunk
	timeout := n.requestTimeoutFor(msg)
	p := &pending{cb: cb, to: to}
	p.cancel = n.clock.AfterFunc(timeout, func() {
		delete(n.pending, rid)
		n.Stats.Timeouts++
		if !holderFetch && attempt < n.cfg.RequestRetries {
			// Transient miss — retry the SAME peer after a decaying backoff instead
			// of tearing it out of the table. Backoff doubles each attempt.
			backoff := n.cfg.RequestBackoff << attempt
			n.logf(ports.LogDebug, "request retry", "to", to, "kind", msg.Kind, "attempt", attempt+1)
			n.clock.AfterFunc(backoff, func() { n.requestAttempt(to, msg, attempt+1, cb) })
			return
		}
		// Retries exhausted. A timeout on a BOND AUDIT (MsgBondChallenge) means the
		// peer was slow to PROVE its bond, NOT that it is unreachable: the prover
		// does real space-time work and returns a large proof that packet loss hits
		// hardest, so under an adverse network these time out in droves. Evicting
		// such a peer from the routing table (and negative-caching it) conflates
		// "slow to prove standing" with "gone", tears the mesh apart, and starves
		// consensus of the very standing it was trying to establish — the durability
		// collapse reproduced by integration/flakynet. So on a bond-challenge
		// timeout, leave routing untouched: the bond audit simply grants no standing
		// this round and retries next interval. Genuine reachability failures (DHT
		// lookups, fetches) still evict + negative-cache as before.
		//
		// A CONFIGURED static peer (-persistent-peers: the validator/anchor consensus
		// tier) is likewise never evicted: its address is configured, not discovered,
		// so a transient WAN unreachability is a retry situation, not an eviction —
		// evicting it would let a proposer lose an attester mid-formation and stall
		// quorum (#286 Layer 2 Q4; docs/network-durability.md §2/§8).
		if msg.Kind != ports.MsgBondChallenge && !n.staticPeers[to] {
			n.table.Remove(to)
			delete(n.reachable, to) // no longer proven reachable (#43)
			// Negative-cache the corpse so the fetch/repair loop skips it for a
			// cooldown instead of re-eating a full RequestTimeout on the same dead
			// holder every sweep (#226). A reply later (n.reachable set in handle)
			// doesn't clear this, but cooldown expiry re-admits a recovered holder.
			if n.cfg.HolderCooldown > 0 {
				now := n.clock.Now()
				// Keep the map from growing without bound under ephemeral-identity
				// churn: once it's large, drop entries whose cooldown already
				// lapsed (they'd be re-admitted on next dial anyway). Cheap because
				// it only sweeps past a threshold, not on every timeout.
				if len(n.deadUntil) >= maxDeadHolders {
					for id, until := range n.deadUntil {
						if now >= until {
							delete(n.deadUntil, id)
						}
					}
				}
				n.deadUntil[to] = now.Add(n.cfg.HolderCooldown)
			}
			// The corpse is confirmed unreachable (retries exhausted) and just left
			// the routing table — so prune it from the provider-record candidate set
			// too, for every key where a LIVE alternative exists. This is the #277
			// dial-storm floor fix: without it, deadUntil only RATE-LIMITS the re-dial
			// (one full RequestTimeout per HolderCooldown, forever), because the stale
			// record is never removed. A SOLE holder's record is KEPT (RemoveIfNotSole)
			// so its content stays discoverable and re-probeable (#69/#226); a recovered
			// holder re-announces (reprovide) to rejoin the set, and its inbound message
			// clears deadUntil (proof of life, above).
			if pruned := n.provs.RemoveIfNotSole(to); pruned > 0 {
				n.Stats.DeadProviderRecordsPruned += pruned
			}
		}
		n.logf(ports.LogDebug, "request timeout", "to", to, "kind", msg.Kind, "attempts", attempt+1)
		cb(ports.Message{}, fmt.Errorf("%w (to %s)", ErrTimeout, to))
	})
	n.pending[rid] = p
	n.Stats.QueriesSent++
	if err := n.send(to, msg); err != nil {
		p.cancel()
		delete(n.pending, rid)
		cb(ports.Message{}, err)
	}
}

// handle is the single entry point for every incoming message.
func (n *Node) handle(from ports.NodeID, msg ports.Message) {
	if n.blockedPeers[from] {
		return // simulated partition: drop everything from a peer on the far side
	}
	// Learn the sender's failure domain BEFORE observing it, so the routing
	// table's per-domain diversity cap (H5-B) sees the domain on first contact
	// and a single-domain Sybil cluster can't crowd out honest peers.
	if msg.Domain != 0 {
		evictPeerInfoIfFull(n.peerDomains, from)
		n.peerDomains[from] = msg.Domain
	}
	// Any message is proof of life — but a short-lived client (publish/fetch
	// that keeps nothing) will vanish, so routing to it only poisons the
	// table with ghosts; process its message, but don't add it (#43).
	if !msg.Ephemeral {
		n.table.Observe(from)
		// Proof of life also clears the dead-peer negative cache: if this peer was
		// negative-cached as unreachable (a prior dial timed out → deadUntil), hearing
		// from it means it RECOVERED — a restart+reprovide (#69), or a NATed peer now
		// reachable via the relay. A truly departed holder sends nothing and stays
		// gated (so the #277 dial-storm gate on the diversity sweep still holds), but a
		// recovered one must be re-dialable at once or the sweep/walk keep skipping a
		// peer that is demonstrably back (this is what broke cross-NAT restart survival
		// when the sweep gate was added).
		delete(n.deadUntil, from)
	}
	if msg.CapTotal > 0 {
		evictPeerInfoIfFull(n.peerCaps, from)
		n.peerCaps[from] = capInfo{used: msg.CapUsed, total: msg.CapTotal}
	}
	if msg.BondRoot != (ports.Hash{}) && !msg.Ephemeral {
		evictPeerInfoIfFull(n.peerBonds, from)
		n.peerBonds[from] = bondInfo{root: msg.BondRoot, size: msg.BondSize}
	}

	if msg.IsReply() {
		p, ok := n.pending[msg.RID]
		if !ok || p.to != from {
			return // late, duplicate, or forged reply: drop
		}
		delete(n.pending, msg.RID)
		p.cancel()
		n.reachable[from] = n.clock.Now() // a reply proves we can dial them (#43)
		p.cb(msg, nil)
		return
	}

	if n.handleChain(from, msg) {
		return
	}

	switch msg.Kind {
	case ports.MsgFindNode:
		n.reply(from, msg, ports.Message{
			Kind:  ports.MsgFindNodeReply,
			Nodes: n.closestExcluding(msg.Target, from),
		})
	case ports.MsgGetProviders:
		// Re-serve the SIGNED records (H5), so the fetcher self-certifies each
		// instead of trusting a bare NodeID list we could have fabricated. An
		// eclipser (H5-B test seam) suppresses the records while still routing.
		var recs []ports.ProviderRecord
		if !n.eclipser {
			// Serve only LIVE records: a lapsed record is a departed holder, and
			// re-serving it propagates the #277 dial-storm to whoever asked us.
			recs = n.provs.Live(msg.Target, int64(n.clock.Now()))
		}
		n.reply(from, msg, ports.Message{
			Kind:         ports.MsgGetProvidersReply,
			ProviderRecs: recs,
			Nodes:        n.closestExcluding(msg.Target, from),
		})
	case ports.MsgAddProvider:
		// The announcer vouches for itself with a self-certifying record (H5); a
		// nil record is the legacy self-vouch (accepted only when not strict).
		// Fetchers also hash-verify chunk bytes, so a lying provider wastes time.
		// An eclipser drops the record (H5-B) but still acks, hiding the suppression.
		if rec, ok := n.acceptAnnounce(msg.Target, from, msg.Provider); ok && !n.eclipser {
			n.provs.Add(rec)
		}
		n.reply(from, msg, ports.Message{Kind: ports.MsgAddProviderAck, OK: true})
	case ports.MsgStoreChunk:
		if n.freeload || (msg.Proof != nil && n.isDenied(msg.Proof.Root)) {
			n.reply(from, msg, ports.Message{Kind: ports.MsgStoreChunkAck, OK: false})
			return
		}
		c := ports.Chunk{ID: msg.ChunkID, Data: msg.Data}
		// Providers register under the placement key — the column key for a
		// coded shard, the chunk's own id otherwise — so a reader walking to
		// that key finds the whole column here.
		key := ports.Hash(msg.ChunkID)
		if msg.Proof != nil {
			key = placementKey(msg.Proof.Root, msg.ChunkID, msg.Proof.Column)
		}
		ok := c.Verify() // never store what doesn't hash right
		if ok && msg.Proof != nil && !verifyStorageProof(*msg.Proof, msg.ChunkID) {
			ok = false // refuse chunks with proofs we couldn't defend under audit
		}
		if ok && n.liar {
			// Keep the receipt, ditch the goods: the liar keeps the proof so it
			// can later form a (doomed) audit answer, but never stores the bytes.
			if msg.Proof != nil {
				n.proofMeta[msg.ChunkID] = metaOf(*msg.Proof)
				n.proofs.Put(msg.ChunkID, *msg.Proof)
			}
			n.provs.Add(n.providerRecord(key))
			n.reply(from, msg, ports.Message{Kind: ports.MsgStoreChunkAck, OK: true})
			return
		}
		if ok {
			if err := n.store.Put(bg(), c); err != nil {
				n.logf(ports.LogWarn, "store put failed", "chunk", msg.ChunkID, "err", err)
				ok = false
			} else {
				n.Stats.ChunksReceived++
				n.provs.Add(n.providerRecord(key)) // we are now a provider
				if msg.Proof != nil {
					// Resident metadata for the hot-path sites; the full proof
					// write-throughs to the backing (persisted so a restart
					// re-announces under the right key and can still audit, #69),
					// paged back into the cache only when served or audited.
					n.proofMeta[msg.ChunkID] = metaOf(*msg.Proof)
					if err := n.proofs.Put(msg.ChunkID, *msg.Proof); err != nil {
						n.logf(ports.LogWarn, "proof persist failed", "chunk", msg.ChunkID, "err", err)
					}
				}
				if msg.Lease {
					n.takeLease(msg.ChunkID) // demand-driven cache copy: hold, but let it expire
				}
			}
		}
		n.reply(from, msg, ports.Message{Kind: ports.MsgStoreChunkAck, OK: ok})
	case ports.MsgFetchChunk:
		if n.freeload || n.chunkDenied(msg.ChunkID) {
			n.reply(from, msg, ports.Message{Kind: ports.MsgFetchChunkReply, Found: false})
			return
		}
		c, err := n.store.Get(bg(), msg.ChunkID)
		if err != nil {
			n.reply(from, msg, ports.Message{Kind: ports.MsgFetchChunkReply, Found: false})
			return
		}
		n.Stats.ChunksServed++
		n.Stats.BytesServed += int64(len(c.Data))
		if n.ledger != nil {
			bytes := int64(len(c.Data))
			// Object-aware serve: a coded shard carries its object root in the
			// storage proof, so route a durability auto-skim of the serve revenue
			// into THAT object's escrow (H7 slice 3 — popular data self-funds its
			// repair). Chunks with no proof-anchored root (manifest chunks, uncoded
			// files) keep the plain serve; the server's net credit is the same either
			// way, only a slice is diverted to durability.
			if m, ok := n.proofMeta[msg.ChunkID]; ok && m.Root != (ports.Hash{}) {
				n.ledger.RecordServeToObject(n.id, from, m.Root, msg.ChunkID, bytes)
			} else {
				n.ledger.RecordServe(n.id, from, msg.ChunkID, bytes)
			}
		}
		n.bumpDemand(msg.ChunkID)
		n.reply(from, msg, ports.Message{Kind: ports.MsgFetchChunkReply, Found: true, Data: c.Data})
	case ports.MsgHasChunk:
		ok, _ := n.store.Has(bg(), msg.ChunkID)
		if n.liar {
			_, ok = n.proofMeta[msg.ChunkID] // "of course I have it"
		}
		if n.chunkDenied(msg.ChunkID) {
			ok = false // taken down: no longer available here
		}
		n.reply(from, msg, ports.Message{Kind: ports.MsgHasChunkReply, Found: ok})
	case ports.MsgChallenge:
		if n.chunkDenied(msg.ChunkID) {
			n.reply(from, msg, ports.Message{Kind: ports.MsgChallengeReply, Found: false})
			return
		}
		n.reply(from, msg, n.answerChallenge(msg))
	case ports.MsgRepairClaim:
		n.handleRepairClaim(from, msg) // async: replies MsgRepairVote when the two legs settle (H7)
	case ports.MsgDeliveryReceipt:
		n.handleDeliveryReceipt(from, msg) // D-DEMAND: verify + bank a delivery receipt
	case ports.MsgBondChallenge:
		n.reply(from, msg, n.answerBondChallenge(from, msg))
	case ports.MsgTokenRequest:
		n.reply(from, msg, n.answerTokenRequest(from, msg))
	case ports.MsgGetIssuerKey:
		n.reply(from, msg, n.answerIssuerKey())
	case ports.MsgGetCanonicalIssuers:
		n.reply(from, msg, n.answerCanonicalIssuers())
	case ports.MsgCheckReachability:
		// A peer wants to know if it is publicly reachable. Answering means
		// dialing it back at its advertised address: if the reply lands, the
		// dial succeeded, which is itself the proof. We echo the nonce so the
		// asker can match the answer to its outstanding check.
		n.send(from, ports.Message{Kind: ports.MsgReachabilityReply, Nonce: msg.Nonce})
	case ports.MsgReachabilityReply:
		// A helper reached us back — we are reachable. Resolve the probe;
		// the timeout handles the silent (NATed) case.
		if p, ok := n.reachProbes[msg.Nonce]; ok {
			delete(n.reachProbes, msg.Nonce)
			p.cancel()
			p.done(true)
		}
	}
}

func (n *Node) reply(to ports.NodeID, req ports.Message, resp ports.Message) {
	resp.RID = req.RID
	n.send(to, resp)
}

func (n *Node) closestExcluding(target ports.Hash, exclude ports.NodeID) []ports.NodeID {
	out := make([]ports.NodeID, 0, n.cfg.K)
	for _, id := range n.table.Closest(target, n.cfg.K+1) {
		if id != exclude {
			out = append(out, id)
			if len(out) == n.cfg.K {
				break
			}
		}
	}
	return out
}

// Bootstrap introduces the node to the network via seed peers, then
// looks up its own ID — the standard Kademlia join, which populates the
// buckets closest to self (the ones that matter most).
func (n *Node) Bootstrap(seeds []ports.NodeID, done func()) {
	if len(seeds) > 0 {
		n.bootstrapSeeds = seeds // remember for empty-table re-bootstrap (#281)
	}
	for _, s := range seeds {
		n.table.Observe(s)
	}
	n.IterativeFindNode(n.id, func([]ports.NodeID) { done() })
}

// IterativeFindNode drives the pure dht.Lookup over the wire and calls
// done with the k closest live nodes to target.
func (n *Node) IterativeFindNode(target ports.Hash, done func([]ports.NodeID)) {
	n.newWalk(ports.MsgFindNode, target, nil, done).step()
}

// walk drives one dht.Lookup over the transport. finished guards
// against late replies re-entering a completed walk (every callback may
// fire after the walk has already converged or been aborted).
type walk struct {
	n           *Node
	l           *dht.Lookup
	kind        ports.MsgKind
	target      ports.Hash
	onProviders func([]ports.ProviderRecord) bool // optional; true = stop the walk
	done        func([]ports.NodeID)              // fires at most once
	finished    bool
}

func (n *Node) newWalk(kind ports.MsgKind, target ports.Hash,
	onProviders func([]ports.ProviderRecord) bool, done func([]ports.NodeID)) *walk {
	return &walk{
		n:           n,
		l:           dht.NewLookup(target, n.cfg.K, n.cfg.Alpha, n.table.Closest(target, n.cfg.K)),
		kind:        kind,
		target:      target,
		onProviders: onProviders,
		done:        done,
	}
}

func (w *walk) step() {
	if w.finished {
		return
	}
	now := w.n.clock.Now()
	for {
		if w.l.Done() {
			w.finished = true
			result := w.l.Result()
			// Trampoline the terminal callback through the event loop instead of firing
			// it inline. A walk that converges SYNCHRONOUSLY — every candidate dead/cooled,
			// or the node alone — otherwise runs done() on the same stack that entered
			// step(); when done() re-enters another walk (announceAll's per-key
			// IterativeFindNode; the resolveProviders⇄probeShard⇄sweepProviders repair
			// cycle) the frames pile up O(keys) deep and overflow the goroutine stack over
			// a large held set (flixz load test: 193K keys → fatal error: stack overflow →
			// watchdog kill, blocking publish of the two largest films). Deferring done()
			// bounds stack depth to O(1) across keys regardless of sync-vs-async completion.
			// AfterFunc(0) posts to the loop (walltime marshals onto it, sim enqueues) —
			// B2-safe, one extra tick per walk (negligible for background repair/announce).
			w.n.clock.AfterFunc(0, func() { w.done(result) })
			return
		}
		queries := w.l.NextQueries()
		if len(queries) == 0 {
			return // at alpha in-flight; the outstanding callbacks will re-step
		}
		dispatched := 0
		for _, peer := range queries {
			peer := peer
			// Skip a peer we recently failed to reach. Dialing a dead routing
			// entry costs a full RequestTimeout, and a repair sweep walking many
			// keys drowns in these stale dials before it can probe a single shard
			// — the churn dial-storm: the caretaker never finishes a sweep, never
			// sees the loss, never repairs. Fail it in the lookup without the dial
			// (mirrors the fetch/probe negative cache, #226); a later walk past
			// its cooldown re-queries it in case it recovered.
			if until, dead := w.n.deadUntil[peer]; dead && now < until {
				w.n.Stats.HolderDialsSkipped++
				w.l.OnFailure(peer)
				continue
			}
			dispatched++
			w.n.request(peer, ports.Message{Kind: w.kind, Target: w.target}, func(resp ports.Message, err error) {
				if w.finished {
					return
				}
				if err != nil {
					w.l.OnFailure(peer)
				} else {
					w.l.OnReply(peer, resp.Nodes)
					for _, id := range resp.Nodes {
						w.n.table.Observe(id)
					}
					if w.onProviders != nil && len(resp.ProviderRecs) > 0 && w.onProviders(resp.ProviderRecs) {
						w.finished = true // caller has what it needs
						return
					}
				}
				w.step()
			})
		}
		if dispatched > 0 {
			return // real requests in flight; their callbacks drive the walk
		}
		// every candidate this round was cooled down — loop to advance the lookup
	}
}
