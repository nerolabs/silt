package main

import (
	"crypto/rand"
	"crypto/rsa"
	"flag"
	"fmt"
	"net"
	"net/http"
	_ "net/http/pprof" // registers /debug/pprof/* on http.DefaultServeMux (gated by -debug-addr)
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	rtdebug "runtime/debug"
	"runtime/pprof"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/nerolabs/silt/adapters/cachestore"
	"github.com/nerolabs/silt/adapters/capstore"
	"github.com/nerolabs/silt/adapters/chainhost"
	"github.com/nerolabs/silt/adapters/chainstore"
	"github.com/nerolabs/silt/adapters/discovery"
	"github.com/nerolabs/silt/adapters/diskissuer"
	"github.com/nerolabs/silt/adapters/diskplot"
	"github.com/nerolabs/silt/adapters/diskproofs"
	"github.com/nerolabs/silt/adapters/diskstore"
	"github.com/nerolabs/silt/adapters/eventloop"
	"github.com/nerolabs/silt/adapters/fileregistry"
	"github.com/nerolabs/silt/adapters/guardstore"
	"github.com/nerolabs/silt/adapters/httpregistry"
	"github.com/nerolabs/silt/adapters/identity"
	"github.com/nerolabs/silt/adapters/lan"
	"github.com/nerolabs/silt/adapters/logfile"
	"github.com/nerolabs/silt/adapters/markstore"
	"github.com/nerolabs/silt/adapters/memstore"
	"github.com/nerolabs/silt/adapters/proofcache"
	"github.com/nerolabs/silt/adapters/relay"
	"github.com/nerolabs/silt/adapters/tcpnet"
	"github.com/nerolabs/silt/adapters/walltime"
	"github.com/nerolabs/silt/core/blindtoken"
	"github.com/nerolabs/silt/core/bond"
	"github.com/nerolabs/silt/core/chain"
	"github.com/nerolabs/silt/core/credit"
	"github.com/nerolabs/silt/core/demand"
	"github.com/nerolabs/silt/core/denylist"
	"github.com/nerolabs/silt/core/genesis"
	"github.com/nerolabs/silt/core/link"
	"github.com/nerolabs/silt/core/node"
	"github.com/nerolabs/silt/ports"
)

// cmdDaemon runs a long-lived swarm node: real TCP listener, real disk
// store, wall clock. One daemon per swarm also hosts the registry
// (-serve-registry) — the v1 "single honest instance", now reachable
// over HTTP so separate processes (and machines) can share it.
func cmdDaemon(args []string) error {
	fs := flag.NewFlagSet("daemon", flag.ExitOnError)
	listen := fs.String("listen", "127.0.0.1:0", "TCP listen address for swarm traffic")
	storeDir := fs.String("store", ".silt-daemon", "chunk store directory")
	bootstrap := fs.String("bootstrap", "", "comma-separated peer list: ID@HOST:PORT")
	persistentPeers := fs.String("persistent-peers", "", "comma-separated ID@HOST:PORT of a STATIC consensus/anchor peer set — address-configured up front and NEVER evicted by churn (Tendermint persistent_peers). At genesis there is no chain, so a proposer cannot DISCOVER its attesters' addresses (silt's routing table holds bare NodeIDs; addresses live in the transport layer, learned only from inbound frames/gossip) — configure the validator set here so proposer-initiated quorum can form on a fresh multi-region net (#286 Layer 2; docs/network-durability.md §8). These are AddPeer'd at boot AND exempt from reachability eviction (§2)")
	registryURL := fs.String("registry", "", "registry ref: ID@https://host:port (key-pinned — copy the daemon's 'registry:' line verbatim; a bare http:// or unkeyed https:// is refused)")
	serveRegistry := fs.String("serve-registry", "", "host the registry at this address (persisted in the store dir)")
	idSeed := fs.Int64("id-seed", 0, "derive the identity from a seed (default: persistent keyfile) — for scripted demos")
	care := fs.String("care", "", "comma-separated care links (siltcare:...) to repair — no decryption possible or needed")
	repairInterval := fs.Duration("repair-interval", 60*time.Second, "how often a caretaker sweeps each -care'd root for lost shards (probe → reconstruct past the slack). A liveness cadence, not a security parameter — the repair-bounty legs stay structural at any setting. Lower it on a small/local swarm so repair (and the e2e proof) fires in seconds")
	capacity := fs.String("capacity", "5G", "storage pledge, e.g. 2G, 500M (matches the client's default so the node contributes measurable, countable storage; \"\" = unlimited but doesn't count toward network storage)")
	freeload := fs.Bool("freeload", false, "role separation (#47): serve the registry/relay/routing role but REFUSE to store or serve content — for public-infrastructure operators who run a rendezvous registry without being conscripted into hosting arbitrary content. The node still carries DHT routing; it just holds and serves no chunks")
	serveContent := fs.Bool("serve-content", true, "D-TIERING capability axis: hold and serve content shards (the edge tier's core contribution, bounded by -capacity). ON by default — this is what an ordinary node does. The explicit form exists so a tier profile composes positively (`-serve-content -archive=false -validator=false` is the transient edge box) rather than as a double negative. `-serve-content=false` is the same refusal as -freeload; passing both with opposite senses is refused rather than silently resolved")
	acceptReceipts := fs.Bool("accept-delivery-receipts", false, "PoD neutral lane (docs/design/pod.md, certified 2026-08-26): BANK delivery receipts from fetchers this node served, and settle the conserved delivery credit — the fetcher's retrieval fee less the durability skim, which routes to the delivered object's repair escrow. Requires the token-issuer role (implied by -validator), because a receipt is verified against the issuer key that signed its retrieval token; the bilateral issuer==server shape is what the certification's per-node settlement answer covers. Delivery credit is BALANCE ONLY and can never become consensus standing (the γ→1/N firewall) — a receipt is mintable with zero object bytes by design, and conservation, not possession, is what makes forging it unprofitable. Off by default")
	acceptRelayPayments := fs.Bool("accept-relay-payments", false, "PoD relay lane (docs/design/pod.md §7.3, certified 2026-08-30): ACCEPT sender-funded PayWord payment chains for forwarding content-blind bytes as a relay/gateway. A fetcher commits a chain root once (bound to a blind credit withdrawn under a FRESH EPHEMERAL identity — never a durable account, M0 guard (i)) and reveals one preimage per forwarded increment; this node verifies each with one SHA-256 and settles the highest into its operator BALANCE (never standing — the γ→1/N firewall; relay credit is fundable with zero object bytes by design). A fresh ephemeral identity and a fresh chain are required PER SESSION (M0 guard (ii): no cross-session linkage). Off by default")
	archive := fs.Bool("archive", false, "D-TIERING ARCHIVAL tier: retain every block's heavy space-time bond proof to genesis instead of shedding it below the rolling retention horizon, so this node can serve the deep history a pruning swarm has already dropped (what a node stranded past the prune horizon needs — #559's true-loss residual, ErrNeedCheckpoint). RETENTION ONLY, never validity: an archival node validates by exactly the same rules as a pruning one, so the tiers cannot fork against each other. Costs O(all history) resident payload — build-immutable #8 forbids it on the 1 vCPU / 2 GB box, which is the whole reason the tier model exists. Off by default")
	registryOnly := fs.Bool("registry-only", false, "the LEANEST public-registry role (#47): serve a file-backed registry over HTTPS and construct NO storage node at all — no DHT, chunk store, chain, or caretaker. Unlike -freeload (a full routing node that refuses to host content), this builds nothing but the registry server, so a public-infrastructure operator runs a rendezvous registry at minimal cost. Needs -serve-registry <addr>")
	// Empty default is deliberate: no built-in seed domain (neutral infra,
	// community-run) — see the discovery package doc (#27 Part A).
	dnsSeed := fs.String("dns-seed", "", "domain whose TXT records list bootstrap peers")
	mdns := fs.Bool("mdns", true, "announce and discover peers on the local network (LAN multicast); needs a non-loopback -listen")
	denylistPath := fs.String("denylist", "", "operator takedown list: a file of denied root hashes to refuse to store/serve (you choose which lists to honor)")
	honorRevocations := fs.Bool("honor-chain-revocations", false, "SUBSCRIBE to the chain's on-chain takedowns (M0 F5): also deny roots a quorum has revoked on-chain. Default OFF — following the chain does not impose someone else's takedowns; honoring is a per-operator choice, proportional to who trusts you, never a global switch. The operator-local -denylist is always honored")
	revokeRoot := fs.String("revoke", "", "as a validator, propose an on-chain takedown of this root hash once standing is earned and the root is committed (M0 F5: quorum-gated, existence-checked; honored only by nodes that -honor-chain-revocations)")
	validator := fs.Bool("validator", false, "keep a chain replica and take part in consensus")
	uiAddr := fs.String("ui", "", "serve the web UI at this address (e.g. 127.0.0.1:8081)")
	debugAddr := fs.String("debug-addr", "", "serve Go pprof (heap/goroutine/profile) at this address (e.g. 127.0.0.1:6060) — diagnostic only, off by default. Used to attribute the MATURING consensus-node memory footprint (`go tool pprof http://addr/debug/pprof/heap`). Also dumps a heap profile to <store>/heap-<pid>.pprof on SIGUSR1 for cloud nodes without a reachable port.")
	attesters := fs.String("attesters", "", "comma-separated validator IDs to gather attestations from")
	anchorList := fs.String("anchors", "", "launch-window training wheels: comma-separated anchor validator IDs whose sign-off an immature-network commit also requires (empty = no training wheels)")
	anchorQuorum := fs.Int("anchor-quorum", 0, "LEGACY (non-objective) only: anchor attestations an immature-network commit needs (0 = off). In OBJECTIVE mode this is IGNORED — the launch gate is a DERIVED strict anchor majority ⌊A/2⌋+1 (#402), so config cannot disable quorum intersection")
	matureValidators := fs.Int("mature-validators", 0, "required NAKAMOTO COEFFICIENT (M0 H4): the anchor requirement sheds only once this many bond-DISTINCT operators are needed to reach ⅓ of the bonded weight — cost-to-corrupt, not a head-count, so one operator with many keys can't trip the wheels off (0 = never require anchors)")
	lambdaHFloor := fs.Float64("lambda-h-floor", 0, "honest-arrival floor λ_H (CT-1 conditional theorem, research cert C1-maturity-before-capture-CONDITIONAL-THEOREM-LIFT-2026-08-27): the minimum operator/domain-distinct bonded-arrival RATE (distinct arrivals per block-height, measured over -lambda-h-window) the launch was certified against. Below this floor — while the network is still YOUNG (pre-maturity latch) — the deployment has LEFT the theorem's hypothesis H (T_mature→∞, maturity-precedes-capture no longer proven) and a LOUD λ_H FLOOR-EXIT marker is surfaced to the log. OBSERVABILITY ONLY: it changes no validity predicate, no consensus rule, no security parameter — it reads the committed C2 metric and narrates. 0 = disabled (default; existing deployments and sims unaffected). Set it to the floor you certified your adversary budget W_A / shed threshold M_req against (P2: M_req > W_A/(2·w_min))")
	lambdaHWindow := fs.Uint64("lambda-h-window", 20, "trailing window (block-heights) the honest-arrival rate λ_H is averaged over for the -lambda-h-floor alarm. λ_H = Δ(min-Nakamoto-coefficient)/Δheight over this many committed heights — the realized net operator/domain-distinct arrival rate. Widen it to smooth per-block churn (TTL lapses can transiently drop the coefficient), narrow it to react faster. Observability only")
	operatorMargin := fs.Int("operator-margin", 1, "operator margin M (M0 C2 / D-C2): the maturity shed discounts the bond-distinct Nakamoto coefficient by M (⌊k̂/M⌋) — since on-chain data carries no operator label, one operator may split a stake across ~M keys, so a splitter must clear mature-validators×M distinct bonds to shed the wheels. LEFT UNSET it defaults to a conservative M>1 for an untrusted objective swarm (safe-by-default, like -min-bond-floor); an explicit 1 = no split margin (single-operator/trusted). M stays a heuristic — unverifiable on-chain (#182)")
	quorum := fs.Int("quorum", 3, "MINIMUM attestations (excluding the proposer) to commit a block — a floor; with -byzantine-quorum the effective requirement rises to the Byzantine threshold over the qualified set. Lower only for a trusted/one-box swarm")
	byzantineQuorum := fs.Bool("byzantine-quorum", false, "size the commit quorum at the Byzantine threshold (M0 H4): the support set becomes a supermajority n−f of the qualified bonded set, so two quorums always share an honest validator (safety as the set grows). LEFT UNSET it defaults ON for an untrusted objective validator; an explicit =false opts out (trusted swarm). Only ever RAISES the bar")
	objective := fs.Bool("objective", true, "DEFAULT-ON for an untrusted validator: consensus fork-choice by OBJECTIVE on-chain bond (F6), so eligibility, quorum, and fork-choice weight are a function of verifiable on-chain bond registrations — identical on every replica — and honest replicas can't diverge under a partition (the M0 consensus denial). Bootstrap a multi-validator quorum with -anchors (the launch set); validators register their real bonds live as they propose. Auto-off for a trusted swarm (-min-rep 0). Pass -objective=false to run the legacy subjective path, which does NOT hold the M0 denial under an adversarial partition")
	minBond := fs.String("min-bond", "1M", "objective mode: the minimum bonded size a validator must prove on-chain to qualify (its -bond must clear this)")
	minRep := fs.Int64("min-rep", 100, "reputation a proposer/attester must have EARNED (bonds+audits) to write — safe default; 0 = trusted deployment (self-commit, unsafe on an open network)")
	bondSize := fs.String("bond", "64M", "storage bond a validator seals to earn consensus standing, proven to peers over time (persisted to disk; a restart reloads it) — a bigger bond earns more standing")
	minBondFloor := fs.String("min-bond-floor", "0", "anti-release floor (M0 F1/F2): a bond smaller than this earns NO standing, because it could be released and re-sealed just-in-time. Size it against the anti-release COMPUTE window (re-seal time × plot throughput) — NOT the transport -request-timeout: at ~270 MB/s and a ~2s compute window that is ~540 MiB, so a real open deployment sets e.g. 1G. Build-immutable #3/#4: raising -request-timeout for durability must NOT move this floor. 0 = off (safe only for a trusted/demo swarm)")
	bondAudit := fs.Duration("bond-audit", 60*time.Second, "how often a validator challenges its peers' bonds and refreshes its own standing")
	loopBudget := fs.Bool("loop-budget", false, "emit the per-window event-loop goroutine-budget decomposition at INFO instead of DEBUG — for a load/diagnostic run that needs the per-handler CPU breakdown without the full -log debug firehose (which, logged synchronously on the loop, would itself skew the measurement). Slow-task, queue-wait, and hang lines are always-on regardless.")
	bootstrapRetry := fs.Duration("bootstrap-retry", 15*time.Second, "how often an ISOLATED node (empty routing table) re-runs the Kademlia join against its -bootstrap seeds. silt's join is otherwise one-shot, so a node that started before its bootstrap target was listening would stay stranded forever (#281). 0 disables (single-shot join only)")
	requestTimeout := fs.Duration("request-timeout", 5*time.Second, "per-ATTEMPT deadline for a DHT/consensus RPC. Exceeding it fails THAT attempt; -request-retries then re-sends with backoff before the peer is given up. Keep it comfortably above the real one-way+reply time on your worst expected path")
	requestRetries := fs.Int("request-retries", 3, "how many times a timed-out RPC is re-sent (exponential backoff from -request-backoff) before the peer is evicted from the routing table and negative-cached. On a jittery/lossy internet path a single slow or dropped packet must NOT tear a good peer out of the mesh — that keeps the routing table sparse and consensus from ever committing (durability under adverse networks). 0 = evict on the first miss (fast/trusted LAN only)")
	requestBackoff := fs.Duration("request-backoff", 250*time.Millisecond, "base delay between RPC retries; doubles each attempt (250ms → 500ms → 1s …). A decaying retry rides out an unknown-duration impairment instead of guessing one big timeout")
	holderDialTimeout := fs.Duration("holder-dial-timeout", 2*time.Second, "tighter per-attempt deadline for speculative holder-fetch dials (chunk fetch / has-chunk), which are NOT retried: stored content lives on arbitrary holders that are often gone under churn, so a fetch from a dead holder must fail fast (the fetch loop retries at a higher level and skips known-dead holders). Kept below -request-timeout so a generous consensus timeout doesn't deepen the dead-holder dial-storm (#277)")
	auditInterval := fs.Duration("audit", 0, "run the verify-without-fetch PoR AUDIT sweep this often over every -care'd root: challenge each shard's holders and grade their proofs against the key derived from the care link — NO ground-truth fetch — settling rent for the honest and SLASHING a liar that kept its proof tags but dropped the bytes (#232). Requires -care (supplies the root + layout key) and a -registry. 0 = off (repair-only caretaker)")
	maxBondRegBytes := fs.Int64("max-bondreg-bytes-per-block", 2<<20, "byte budget for bond registrations embedded in ONE block (#286 Layer 2b). A fresh multi-validator OBJECTIVE genesis otherwise piles every founding validator's ~1.5 MB space-time proof into one ~8 MB block that can't gather to quorum over a real WAN (the cert stalled at regs=5). The founding set are anchors (training wheels), so genesis commits SMALL on anchor attestations and the registrations DRAIN over the next blocks — each validator still gains real bonded weight and reaches maturity. A BYTE budget (not a count) fits one full ~1.5 MB genesis proof OR many small steady-state renewals, so an attest-only validator is never starved under a tight TTL. Default ~2 MiB stays within the size that gathers cross-region; 0 = unbounded (legacy). The structural close is a succinct proof (#299)")
	maxEntryBytes := fs.Int64("max-entry-bytes-per-block", 64<<10, "byte budget for mempool publish ENTRIES folded into ONE block (#441) — SEPARATE from -max-bondreg-bytes-per-block by design: a single ~1.5 MB bond reg fills the whole reg cap, so a shared budget would leave the tens-of-bytes entry no room (the publish starvation one layer down), and the dual — an entry flood must never crowd out consensus-critical renewals. Each stream is guaranteed its own slice; their SUM must stay WAN-gatherable (#286 L2b). At least one entry always folds. 0 = unbounded")
	bondLabelK := fs.Int("bond-label-k", 64, "labeling-consistency opens per bond challenge (M0 Sybil G2): each recomputes one block's label from its DRSample parents, so a prover holding arbitrary/reused/wrong-size bytes (not a real plot for its identity+size) fails. Soundness error ≤ (1-ε)^k against an ε-short prover. A per-network knob — prover and verifier must MATCH (like -bond-vdf), so set it uniformly across the swarm. Lower it only to shrink on-chain proof size, at a soundness cost. 0 = default (64)")
	bondAnswerLatency := fs.Duration("bond-answer-latency", 1500*time.Millisecond, "SOFT partial-storage timing signal on a live bond challenge (M0 C1 / owned-residual A5). A validator that deleted part of its plot must RECOMPUTE the missing blocks on demand, and past the DRSample knee that is a sequential cost that shows up as reply latency. This is NOT a standing gate (build-immutable #3: reply-latency is transport+compute, and gating security on the sum reads network jitter/loss as a cheat — #289): a valid answer earns standing however slow it arrives. Instead the node tracks the windowed-MINIMUM of each peer's reply latencies (the low quantile, which filters one-sided network noise) and raises a DISCLOSED suspicion only when that floor is SUSTAINED above this deadline — a partial-storage prover is consistently slow, an honest bad-path node only randomly slow. Set generously above the honest answer time; 0 = off. The hard structural close is tight-PoS (H-track).")
	signedProviders := fs.Bool("signed-providers", true, "self-certifying DHT provider records (M0 H5): a node signs its 'I hold this' announcements with its identity key and re-verifies records served back on lookup, so a node holding the k-closest slots to a key cannot fabricate provider records for identities that never announced. Default ON; =false drops to the legacy unsigned path (trusted/demo swarm only)")
	signedProviderTTL := fs.Duration("signed-provider-ttl", 30*time.Minute, "freshness window stamped on signed provider records (M0 H5): a re-served record older than this is treated as expired, so an eclipsing node can't replay an ancient claim forever")
	dhtDomainCap := fs.Int("dht-domain-cap", 2, "failure-domain diversity cap for DHT eclipse resistance (M0 H5-B): at most this many peers sharing one -domain are kept per routing bucket, and provider records are announced to / resolved from a domain-spread set — so an adversary owning the NodeIDs closest to a key but sitting in one domain (a ~$4 /24 key-surround) can't suppress discovery. Only bites when peers set distinct -domain labels. 0 = off (no diversity constraint)")
	domain := fs.String("domain", "", "this node's failure-domain label (AS / rack / geo — e.g. \"as64500\" or \"us-east-1b\"). Two uses: DHT eclipse-resistance (H5-B, with -dht-domain-cap) AND, for a validator, it is COMMITTED in the bond so the C2 concentration metric counts ADDRESS-DIVERSE participants (A axis / D-C2) — a stake split across many keys in ONE domain cannot fake decentralization; shedding the launch anchors requires distinct domains, not just distinct keys. A WEAK signal (declared, transport-cross-checked, not proven); it prices concentration higher, it does not close the honest-whale residual. Empty = unset (independent).")
	bondTTL := fs.Uint64("bond-ttl", 0, "objective re-challenge cadence (M0 retest G4 / RT-2): objective standing LAPSES this many committed blocks after a validator's latest on-chain bond registration unless it renews with a fresh space-time proof — so a validator that registers once then releases its plot cannot keep voting. LEFT UNSET it defaults ON for an untrusted objective validator (derived cadence); an explicit 0 disables it (standing never expires; safe only for a trusted/demo swarm)")
	epochBlocks := fs.Uint64("epoch-blocks", 0, "mature-phase validator-set epoch (#357 research certification, Conditions A+B): after the young→mature handoff, the finality quorum, validator qualification, and fork-choice weight are read from a SNAPSHOT of the committed bonded set frozen at the last epoch boundary (a finalized block), rotated every this-many blocks — never recomputed live from the churning bond ledger, which would let two conflicting commits finalize against two different sets. The handoff itself waits for the first boundary after the maturity latch, so the anchor→bond weight transition is rooted at a finalized base. CONSENSUS-CRITICAL: set it identically across the swarm (like -min-bond). LEFT UNSET it defaults ON for an untrusted objective validator (derived cadence, well under the bond TTL); an explicit 0 disables epochs (live recompute; safe only for a trusted/demo swarm)")
	requireTokens := fs.Int("require-tokens", 0, "publisher privacy: require every published entry to carry a publish token blind-signed by this many validators, instead of a Publisher identity (0 = off; validators issue tokens)")
	allowPublisher := fs.Bool("allow-publisher", false, "permit entries that carry a durable Publisher identity (records a PERMANENT Publisher→root link on the append-only chain; off by default for privacy/M0 — only for explicitly trusted deployments)")
	blockPeers := fs.String("block-peers", "", "TEST-HARNESS / FIELD-DRILL: comma-separated peer IDs to PARTITION away from — this node drops all messages to/from them, simulating a severed link (#184 partition→heal). HEAL by restarting without the flag (the persisted chain reloads and reconciles). Empty = no partition; a real deployment never sets it")
	forgeBlock := fs.String("forge-block", "", "RED-TEAM / TEST-HARNESS ONLY: propose a block with a FORGED (corrupted) proposer signature to this peer ID, to prove an honest validator rejects it before attesting (#184 forged-block→reject). Never honest")
	lowbondPropose := fs.String("lowbond-propose", "", "RED-TEAM / TEST-HARNESS ONLY: as an under-bonded validator, propose a well-formed block to this peer ID, to prove an honest validator refuses a proposer without a qualifying bond (#184 low-bond→reject). Never honest")
	equivocate := fs.String("equivocate", "", "RED-TEAM / TEST-HARNESS ONLY: run this validator as a Byzantine EQUIVOCATOR (#184, proving accountability over the real wire). OBJECTIVE mode (3-of-4): the value is a trigger; this validator participates honestly then SERVES a conflicting signed block at a height it prepared, so an honest peer slashes the double-sign on sync (slash-on-detection — a fork can't be committed under a BFT quorum). LEGACY mode: given \"idX,idYZ\" it commit-places block X on idX and a heavier fork (Y,Z) on idYZ. NEVER use on a real network; a correct node refuses to equivocate")
	liar := fs.Bool("liar", false, "RED-TEAM / TEST-HARNESS ONLY: run this storage node as a PoR LIAR — it keeps its storage-proof tags but silently drops the shard bytes (\"keep the receipt, ditch the goods\"). It still answers a MsgChallenge, but with a proof that fails the auditor's verify-without-fetch check, so an -audit auditor CATCHES it and slashes its standing (#232). Never honest")
	goodPropose := fs.String("goodpropose", "", "TEST-HARNESS ONLY: POSITIVE CONTROL for -forge-block/-lowbond-propose. As a properly-bonded proposer, send a WELL-FORMED block to this peer ID and prove the honest target ACCEPTS it — so a target that refuses EVERY proposal (a broken/wedged node) cannot make the forged/low-bond REJECT tests false-pass ('reject the good one too' would otherwise look identical to 'reject the bad one', audit #303). Retries until its bond earns standing. Logs 'goodpropose proposal ACCEPTED by <id>' on accept, 'goodpropose proposal UNEXPECTEDLY REJECTED by <id>' after giving up")
	wsCheckpoint := fs.String("ws-checkpoint", "", "weak-subjectivity checkpoint HEIGHT:HASH (M0 F-1): a recent trusted committed block this node REFUSES to reorg at or before, regardless of fork weight — the long-range-attack defense that makes the objective maturity latch safe for a fresh/long-offline node. Obtain it out-of-band (the daemon prints `checkpoint: HEIGHT:HASH` for its committed head; cross-check several independent nodes). It must be recent — within ~the bond-TTL window. Empty = genesis-trusting (safe only at launch, on a trusted swarm, or before the network matures)")
	livenessRecoveryHeight := fs.Uint64("liveness-recovery-height", 0, "#535 OPERATOR-DIRECTED liveness-floor recovery (weak-subjectivity trust class, like -ws-checkpoint): an epoch-boundary height at which mature-epoch validation re-bases the finality quorum and validator qualification against the LIVE qualified bonded set instead of the frozen epoch snapshot — for ONE boundary only, after which the normal rotation governs. Use it ONLY when members holding > 1/3 of the frozen epoch's weight have genuinely left (bonds lapsed, not returning) and the chain is stalled at an epoch boundary (chain-status names the state): that loss is outside the BFT liveness model, so the stall is deliberate safety and recovery REQUIRES a human judgment the protocol cannot make. CONFIRM OUT-OF-BAND that the loss is a real outage — not a partition or an attack (a wrongly-invoked recovery can fork; that risk is the accepted weak-subjectivity residual) — and COORDINATE: every honest operator must set the SAME height, or replicas diverge. Must be a multiple of the epoch cadence (-epoch-blocks). 0 = off (default): a bled boundary stalls, which is the certified-correct behavior")
	debug := fs.Bool("debug", false, "shorthand for -log debug (the full firehose)")
	logLevel := fs.String("log", "", "write events at or above this level to <store>/debug.log (error|warn|info|debug); info narrates the normal path (placements, commits, repairs) to validate behavior in the field without the debug firehose")
	relayServe := fs.String("relay", "", "offer relay service at this address (e.g. 0.0.0.0:4002): content-blind ciphertext forwarding for NATed peers, capped; pointless unless this node is publicly reachable")
	relayVia := fs.String("relay-via", "", "RELAYID@HOST:PORT of a relay to lean on if this node turns out to be NATed — peers then reach us through it")
	advertise := fs.String("advertise", "", "publicly dialable HOST:PORT to stamp on outgoing messages — set this on a public box that listens on a wildcard address (a wildcard bind is never advertised on its own)")
	cacheSize := fs.String("cache", "", "in-RAM read cache for hot chunks, e.g. 512M (default off) — a cache hit skips the disk read and the per-read hash re-verify")
	proofCacheSize := fs.String("proof-cache", "64M", "resident RAM budget for HOT storage proofs; the rest live on disk and page in only to serve/audit, so proof RAM is O(hot) not O(held) (0 = unbounded, legacy)")
	memLimit := fs.String("mem-limit", "", "soft heap ceiling (e.g. 1500M, 85% of box RAM) — the Go GC reclaims aggressively as the heap approaches it, so a large-but-bounded working set can't grow into a kernel OOM-kill on a small box. Sets runtime/debug.SetMemoryLimit; equivalent to the GOMEMLIMIT env var (this flag wins if both are set). Empty = no soft limit (default). Not a hard cap: if the LIVE set genuinely exceeds it the GC thrashes rather than crashes — raise the limit or the box.")
	inboundCap := fs.String("inbound-cap", "256M", "bound the in-flight INBOUND message working set: bytes read off the wire but not yet processed on the single loop. A fast/adversarial sender that outruns the loop otherwise piles decoded messages onto an unbounded queue and OOMs the node (a resource-exhaustion DoS). At the cap the reader stops draining that socket → TCP flow-control pushes back on the sender (alive > crashed). A single legal-but-oversized frame is still admitted alone; no single peer may hold more than 1/4 of the budget. 0 = unbounded (legacy). SIZING pulls in two directions: the cap bounds the OOM working set (bigger cap = more RAM headroom needed) AND it bounds worst-case message latency — a full budget means ~cap/drain-rate of queued work ahead of every newly admitted frame, consensus frames included (a saturated 256M draining at 2 MiB/s is ~128s of delay). Size to satisfy both at your expected-worst drain rate; the default assumes a healthy drain (docs/design/owned-residuals.md E5 records the trade and the sequenced hardening).")
	carePublished := fs.Bool("care-published", true, "the daemon repairs content published through its own UI, so your own content stays alive as nodes churn (its manifest counts toward this node's pledge); =false to opt out")
	economy := fs.Bool("economy", false, "OPT IN to the S7 durability repair economy (default OFF): when on, a verified repair PAYS the new holder of a rebuilt shard from the object's own escrow, priced by the protocol formula c·(k·shardBytes) × the rarest-shard multiplier — a network-wide price, never an operator-set amount. Off, the serve auto-skim still fills escrows but no bounty disburses (the half-open state /api/status reports as bountyOn:false). Standing is never affected either way (Invariant A: credits fund durability, never consensus weight). The economy-ON config is what the confirming field runs + the #183 red team exercise")
	fs.Parse(args)

	// Soft heap ceiling (flixz OOM mitigation): the field cohort OOM-crash-loops
	// on a 2 GB box from a large-but-bounded working set colliding with Go's
	// default 2×-heap GC target (not a hard leak — small-scale is stable). A
	// GOMEMLIMIT makes the GC reclaim before the kernel does, trading CPU for
	// staying alive. Diagnostic + attribution of the true footprint is separate
	// (-debug-addr); this keeps a node UP meanwhile.
	if *memLimit != "" {
		bytes, err := parseSize(*memLimit)
		if err != nil {
			return err
		}
		if bytes > 0 {
			rtdebug.SetMemoryLimit(bytes)
			fmt.Printf("mem-limit: soft heap ceiling %s (GC reclaims before this — prevents OOM crash-loops on a small box)\n", *memLimit)
		}
	}

	// Heap-profiling seam (diagnostic-only; off unless -debug-addr is set). This
	// is how we attribute the MATURING consensus-node memory footprint that OOMs
	// the field cohort — the daemon had no heap profile, which is why the wrong
	// structure got blamed (silt-oom-NOT-the-proof-map-FINDING-2026-08-17). Edge
	// concern, cmd-only: net/http/pprof registers on http.DefaultServeMux.
	if *debugAddr != "" {
		go func() {
			// nil handler => DefaultServeMux, which net/http/pprof populates.
			if err := http.ListenAndServe(*debugAddr, nil); err != nil {
				fmt.Fprintf(os.Stderr, "debug pprof server: %v\n", err)
			}
		}()
		fmt.Printf("debug: pprof at http://%s/debug/pprof/ (heap attribution)\n", *debugAddr)
		// SIGUSR1 => write a heap profile to the store dir, for cloud nodes with
		// no reachable debug port (SSH in, `kill -USR1`, scp the file out).
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, syscall.SIGUSR1)
		go func() {
			for range sig {
				path := filepath.Join(*storeDir, fmt.Sprintf("heap-%d.pprof", os.Getpid()))
				f, err := os.Create(path)
				if err != nil {
					fmt.Fprintf(os.Stderr, "heap dump: %v\n", err)
					continue
				}
				runtime.GC() // get up-to-date statistics
				if err := pprof.WriteHeapProfile(f); err != nil {
					fmt.Fprintf(os.Stderr, "heap dump: %v\n", err)
				}
				f.Close()
				fmt.Printf("debug: heap profile written to %s\n", path)
			}
		}()
	}

	// Identity is a keypair: NodeID = SHA-256(public key), persisted so
	// a daemon's reputation survives restarts.
	var ident *identity.Identity
	if *idSeed != 0 {
		ident = identity.FromSeed(*idSeed)
	} else {
		if err := os.MkdirAll(*storeDir, 0o755); err != nil {
			return err
		}
		var err error
		ident, err = identity.LoadOrCreate(filepath.Join(*storeDir, "identity.pem"))
		if err != nil {
			return err
		}
	}
	id := ident.NodeID()

	// -registry-only (#47): the leanest public-registry role — serve a file-backed
	// registry over HTTPS and construct NO storage node (no DHT, chunk store, chain, or
	// caretaker). This returns before the node, transport, and everything downstream is
	// built, so a rendezvous registry runs at minimal cost. Blocks until the process is
	// stopped.
	if *registryOnly {
		if *serveRegistry == "" {
			return fmt.Errorf("-registry-only needs -serve-registry <addr> (the address to serve the registry on)")
		}
		if err := os.MkdirAll(*storeDir, 0o755); err != nil {
			return err
		}
		freg, err := fileregistry.Open(filepath.Join(*storeDir, "registry.jsonl"))
		if err != nil {
			return err
		}
		bound, shutdown, err := httpregistry.ServeTLS(*serveRegistry, ident, freg)
		if err != nil {
			return err
		}
		defer shutdown()
		fmt.Printf("registry-only: %s serving a file-backed registry at https://%s (no storage node, #47)\n", id, bound)
		fmt.Println("serving; Ctrl-C to stop")
		select {} // block until the process is stopped; the registry server runs in its own goroutine
	}

	loop := eventloop.New()
	tr, err := tcpnet.New(loop, ident, *listen)
	if err != nil {
		return err
	}
	// Bound the inbound working set so a fast/adversarial sender can't OOM the
	// single loop (the MATURING crash-loop root cause). The documented "0 =
	// unbounded" sentinel is handled at the flag level (parseSize requires a
	// positive size — the same pattern as -capacity in client.go).
	if *inboundCap != "" && *inboundCap != "0" {
		cap, err := parseSize(*inboundCap)
		if err != nil {
			return err
		}
		tr.SetInboundCap(cap)
		fmt.Printf("inbound-cap: %s in-flight message budget (backpressure over cap; a flood stalls, doesn't OOM)\n", *inboundCap)
	}
	if *advertise != "" {
		tr.SetAdvertise(*advertise)
	}
	var store ports.ChunkStore
	disk, err := diskstore.Open(*storeDir)
	if err != nil {
		return err
	}
	store = disk
	// -cache: an in-RAM read cache just above disk and below capacity
	// accounting, so hot chunks skip the disk read and the per-read hash
	// re-verify. Off by default; capstore stays outermost so it still
	// reports capacity.
	if *cacheSize != "" {
		budget, err := parseSize(*cacheSize)
		if err != nil {
			return err
		}
		if budget > 0 {
			store = cachestore.Open(store, budget)
			fmt.Printf("cache: %s hot-chunk read cache (hits skip disk + re-verify)\n", *cacheSize)
		}
	}
	if *capacity != "" {
		pledge, err := parseSize(*capacity)
		if err != nil {
			return err
		}
		capped, err := capstore.Open(store, pledge)
		if err != nil {
			return err
		}
		store = capped
		used, total := capped.Capacity()
		fmt.Printf("pledge: %d / %d bytes used\n", used, total)
	}
	// Base on DefaultConfig so new fields are inherited, not silently
	// dropped to their zero value (#71 — this is how demand-dispersion was
	// off in the daemon and the #65 fetch-retry shipped inert). Override
	// only what the daemon genuinely needs to differ on.
	cfg := node.DefaultConfig()
	cfg.RequestTimeout = ports.Duration(2 * time.Second) // patient vs the 500ms default (real WAN)
	cfg.BondAuditInterval = ports.Duration(*bondAudit)
	cfg.RepairInterval = ports.Duration(*repairInterval)
	cfg.BootstrapRetryInterval = ports.Duration(*bootstrapRetry)
	cfg.RequestTimeout = ports.Duration(*requestTimeout)
	cfg.RequestRetries = *requestRetries
	cfg.RequestBackoff = ports.Duration(*requestBackoff)
	cfg.HolderDialTimeout = ports.Duration(*holderDialTimeout)
	cfg.BondLabelSamples = *bondLabelK
	cfg.MaxBondRegBytesPerBlock = *maxBondRegBytes
	cfg.MaxEntryBytesPerBlock = *maxEntryBytes
	cfg.BondMaxAnswerLatency = ports.Duration(*bondAnswerLatency) // C1 recompute deterrent (BREAK 1 / A5); soft, generous
	cfg.RepairEconomy = *economy                                  // S7 repair-bounty payout; opt-in, default OFF (Invariant A untouched)
	// Self-certifying provider records (M0 H5): ON by default so a node can't
	// fabricate DHT provider records for identities that never announced — records
	// are signed by the provider and re-verified on lookup. Records carry an expiry
	// (freshness) tied to the wall clock. -signed-providers=false drops to the
	// legacy unsigned path (a trusted/demo swarm only).
	cfg.RequireSignedProviders = *signedProviders
	cfg.ProviderRecordTTL = ports.Duration(*signedProviderTTL)
	cfg.DHTDomainCap = *dhtDomainCap // failure-domain diversity for eclipse resistance (H5-B)
	cfg.Domain = *domain             // this node's failure-domain label (H5-B DHT diversity + committed in the bond for the A-axis C2 metric)
	// The anti-release floor is SAFE-BY-DEFAULT on the objective/open path (M0
	// retest G4-residual). Shipping the mechanism but defaulting it OFF left a
	// doc-following open validator admitting a sub-floor, releasable bond to full
	// standing — "fixed but off by default" is not fixed. So it gets the same
	// treatment -objective already has: auto-on for an untrusted swarm (-min-rep
	// > 0), and an operator can still opt out EXPLICITLY with -min-bond-floor 0
	// (a trusted/demo swarm, where objective mode is off anyway).
	//
	// The derived value follows DerivedBondFloor: a plot must be too big to
	// re-seal inside the anti-release COMPUTE window (AntiReleaseComputeWindow,
	// NOT the transport -request-timeout — build-immutable #3/#4), else it can be
	// released and recomputed just-in-time. At bond.PlotSealThroughput (~270 MB/s)
	// and the ~2s compute window that is ~540 MiB, so the default carries ~2x margin.
	floorSet, ttlSet, byzSet, marginSet, epochSet := false, false, false, false, false
	fs.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "min-bond-floor":
			floorSet = true
		case "bond-ttl":
			ttlSet = true
		case "byzantine-quorum":
			byzSet = true
		case "operator-margin":
			marginSet = true
		case "epoch-blocks":
			epochSet = true
		}
	})
	explicitFloor, ferr := parseSize(*minBondFloor)
	if ferr != nil {
		explicitFloor = 0
	}
	// objectivePath mirrors the decision made later for the chain config (see
	// useObjective): objective fork-choice is the default for an untrusted
	// VALIDATOR, auto-off when trusted. A non-validator claims no consensus
	// standing, so the floor never applies to it.
	objectivePath := *validator && *objective && *minRep > 0
	effFloor, defaulted := effectiveBondFloor(floorSet, explicitFloor, objectivePath)
	if defaulted {
		fmt.Printf("bond: anti-release floor defaulted to %d MiB for this untrusted (objective) swarm — a smaller plot could be released and re-sealed within the anti-release compute window (%s × plot throughput; independent of -request-timeout). Override with -min-bond-floor (0 disables; safe only for a trusted/demo swarm).\n", effFloor>>20, AntiReleaseComputeWindow)
	}
	// The bond TTL gets the same safe-by-default treatment as the floor (H2 / RT-2):
	// left OFF it admitted release-and-coast — a validator registers a bond once,
	// releases the plot, and keeps voting forever off a single one-time proof. On
	// the objective path it now decays un-renewed standing by default; the paired
	// non-proposer renewal path (node.SubmitBondRenewal, driven by the chain-sync
	// sweep) lets an honest attest-only validator renew without proposing, so the
	// default costs no liveness.
	effTTL, ttlDefaulted := effectiveBondTTL(ttlSet, *bondTTL, objectivePath)
	if ttlDefaulted {
		fmt.Printf("bond: objective re-challenge TTL defaulted to %d blocks for this untrusted (objective) swarm — standing lapses this many blocks after a validator's latest bond proof unless it renews, so a released plot can't keep voting. Override with -bond-ttl (0 disables; safe only for a trusted/demo swarm).\n", effTTL)
	}
	// Byzantine quorum sizing gets the same safe-by-default treatment (H4): a fixed
	// quorum loses quorum-intersection safety as the validator set grows, so an
	// untrusted objective validator sizes the commit quorum at the Byzantine
	// threshold unless the operator opts out. It only ever raises the bar.
	effByz, byzDefaulted := effectiveByzantineQuorum(byzSet, *byzantineQuorum, objectivePath)
	if byzDefaulted {
		fmt.Println("consensus: Byzantine quorum sizing defaulted ON for this untrusted (objective) swarm — a commit needs a supermajority of the qualified bonded set so two quorums always share an honest validator. Override with -byzantine-quorum=false (safe only for a trusted swarm).")
	}
	// The C2 operator margin M gets the same safe-by-default treatment (D-C2 / red-team
	// blind-2026-08-08): shipping the split-defense mechanism but defaulting M=1 left an
	// untrusted objective swarm with ZERO margin against one operator splitting real stake
	// across NodeIDs to fake decentralization — the Invariant-B footgun ("safe config is
	// the default"). It only ever RAISES the bar to shed the training wheels (a splitter
	// must clear mature-validators×M distinct bonds), so an auto-armed M>1 never weakens an
	// existing config. M stays an honest heuristic (on-chain data carries no operator label,
	// #182); this defaults it to a conservative value, tunable per deployment.
	effMargin, marginDefaulted := effectiveOperatorMargin(marginSet, *operatorMargin, objectivePath)
	if marginDefaulted {
		fmt.Printf("consensus: operator-margin defaulted to %d for this untrusted (objective) swarm — the C2 maturity shed discounts the bond-distinct Nakamoto coefficient by M, so one operator splitting real stake across ~M NodeIDs cannot fake the decentralization that sheds the launch anchors. Override with -operator-margin (1 = no split margin; safe only for a trusted/single-operator swarm).\n", effMargin)
	}
	// The mature-phase epoch gets the same safe-by-default treatment (#357
	// Conditions A+B): without it the post-handoff finality quorum is recomputed
	// live from the churning bond ledger, which forfeits the quorum-intersection
	// safety that makes a §3 super-quorum final. Consensus-critical — every
	// validator in the swarm must run the same value, which the shared default
	// provides; an explicit 0 opts a trusted/demo swarm out.
	effEpoch, epochDefaulted := effectiveEpochBlocks(epochSet, *epochBlocks, objectivePath)
	if epochDefaulted {
		fmt.Printf("consensus: epoch-blocks defaulted to %d for this untrusted (objective) swarm — after the young→mature handoff the finality quorum and validator set are frozen per epoch (rotated at finalized boundary blocks), so churning bonds cannot let two conflicting commits finalize against two different sets. Override with -epoch-blocks, identically across the swarm (0 disables; safe only for a trusted/demo swarm).\n", effEpoch)
	}
	if effFloor > 0 {
		cfg.MinBondBytes = effFloor
		if bsz, _ := parseSize(*bondSize); bsz < effFloor {
			// Fail closed rather than let the operator run a validator that
			// silently earns nothing — mirrors the -bond/-min-bond check below.
			return fmt.Errorf("bond: -bond (%s) is below the anti-release floor (%d MiB), so this validator would earn NO standing: raise -bond, or pass -min-bond-floor 0 to accept sub-floor bonds (trusted/demo swarms only)", *bondSize, effFloor>>20)
		}
	}
	clk := walltime.New(loop)
	nd := node.New(id, cfg, clk, tr, store)
	nd.SetSigner(ident.Signer()) // sign self-certifying provider records (H5), not just chain blocks
	if *blockPeers != "" {
		var blocked []ports.NodeID
		for _, s := range strings.Split(*blockPeers, ",") {
			if s = strings.TrimSpace(s); s != "" {
				bid, berr := ports.ParseHash(s)
				if berr != nil {
					return fmt.Errorf("-block-peers %q: %w", s, berr)
				}
				blocked = append(blocked, bid)
			}
		}
		nd.SetBlockedPeers(blocked)
		fmt.Printf("⚠ PARTITION: -block-peers set — dropping all traffic to/from %d peer(s) (test-harness / field-drill, never a real deployment)\n", len(blocked))
	}
	// The content-serving axis has two spellings: the positive -serve-content
	// (D-TIERING's composable form) and the older negative -freeload. They must
	// agree — an operator who passes both with opposite senses stated two
	// different intents, so refuse loudly rather than pick one (S3: no silent
	// resolution of a contradictory config).
	serveContentSet := false
	fs.Visit(func(f *flag.Flag) {
		if f.Name == "serve-content" {
			serveContentSet = true
		}
	})
	refusesContent, err := resolveContentServing(*serveContent, serveContentSet, *freeload)
	if err != nil {
		return err
	}
	if refusesContent {
		nd.SetFreeload(true)
		// Both spellings on one line, on purpose: `freeload: ON` is a STABLE
		// marker the e2e harness (and any operator tooling) greps for, so the
		// D-TIERING rename must not silently break it — an announced line is an
		// observable contract (S5). The new positive-axis name leads because that
		// is how the tier profile is now composed.
		fmt.Println("serve-content: OFF (freeload: ON) — this node refuses to store or serve content (registry/relay/routing only, #47)")
	}
	if *archive {
		fmt.Println("archive: ON — ARCHIVAL tier: every heavy bond proof retained to genesis, so this node can serve the deep history a pruning swarm has shed (O(all history) resident — not for the 2 GB box)")
	}

	// -log/-debug: dlog adds the daemon's own milestones (discovery,
	// bootstrap) to the same artifact the node and transport narrate to.
	level, logOn, err := resolveLogLevel(*logLevel, *debug, *validator)
	if err != nil {
		return err
	}
	var lg *logfile.Sink
	if logOn {
		if lg, err = openLog(*storeDir, level, tr, nd); err != nil {
			return err
		}
		defer lg.Close()
	}
	dlog := func(event string, kv ...any) {
		if lg != nil {
			lg.Log(ports.LogInfo, event, kv...)
		}
	}
	// #69: persist each hosted chunk's storage proof so a restart re-announces
	// coded shards under the right column key (AnnounceHeld, below, reads the
	// reloaded proofs) — otherwise a disk full of content is invisible until
	// re-hosted. The reload is scheduled LAZILY onto the event loop (below), so a
	// large store does not stall startup before the relay/registry listeners bind
	// (flixz F3: ~9 min of downtime scanning a 14 GB store synchronously).
	if pf, perr := diskproofs.Open(filepath.Join(*storeDir, "proofs")); perr != nil {
		return perr
	} else {
		// Bound resident proof RAM to O(hot): the node keeps tiny metadata for
		// every held chunk, but the full proofs (Merkle path + PoR tags, ~5 KB
		// each) live on disk and page into this bounded cache only to serve or
		// audit. Without it a disk full of chunks pins one full proof each in RAM
		// and OOM-crash-loops the daemon (the field-corroborated fix).
		var ps ports.ProofStore = pf
		if budget, berr := parseSize(*proofCacheSize); berr != nil {
			return berr
		} else if budget > 0 {
			ps = proofcache.Open(pf, budget)
			fmt.Printf("proof-cache: %s hot-proof RAM budget (rest page from disk to serve/audit)\n", *proofCacheSize)
		}
		nd.SetProofStore(ps)
		// The O(store) proof-maturation scan runs ASYNC in bounded batches on the
		// event loop (StartProofReload) — it never blocks startup, so the relay +
		// registry listeners below bind immediately (a public node's registry/relay
		// were connection-refused for ~9 min after every restart on a 14 GB store)
		// and proofMeta matures lazily while the daemon serves. An announce that
		// races the scan self-corrects on the next reprovide sweep (#69).
		nd.StartProofReload()
	}
	// #93: persist the bond plot so a restart reloads (and re-verifies) it
	// instead of re-plotting the deliberately-expensive dataset. Attach before
	// EnableBond, below, which loads-or-plots through it.
	if pl, perr := diskplot.Open(filepath.Join(*storeDir, "plot")); perr != nil {
		return perr
	} else {
		nd.SetPlotStore(pl)
	}
	// obs is lg as a nullable interface (a typed-nil *Sink would pass
	// the adapters' nil checks and then explode).
	var obs ports.Logger
	if lg != nil {
		obs = lg
	}
	// A task that panics on the node's thread (e.g. handling a malformed
	// frame that reaches a decoder) fails that task, not the daemon — and
	// says so, loudly. Any such panic is a top-severity bug until fixed.
	loop.OnPanic = func(r any) {
		ports.LogIf(obs, ports.LogError, "recovered panic on node thread", "panic", r)
	}

	// Single-goroutine latency instrumentation (Andrew's timing-for-evidence
	// idea, 2026-08-15). The event loop is the one serialization point, so
	// timing each task names the handler that eats the node's thread — or, via
	// the watchdog, one that HANGS it, with a stack dump of where it is stuck.
	// This is the observability that a starved MATURING field run lacked (the
	// per-issuer gather legs were debug-gated), and the per-kind window summary
	// IS the goroutine-budget decomposition (which message kind dominates).
	// Thresholds are deliberately conservative; a single task on this thread
	// blocking others for >250ms is already notable, and >15s means downstream
	// requests (8s transport deadline) are already timing out behind it.
	loop.SlowThreshold = 250 * time.Millisecond
	loop.OnSlow = func(label string, d time.Duration) {
		ports.LogIf(obs, ports.LogWarn, "eventloop task slow (node thread blocked)", "kind", label, "ms", d.Milliseconds())
	}
	loop.HangThreshold = 15 * time.Second
	loop.OnHang = func(label string, d time.Duration, stack []byte) {
		ports.LogIf(obs, ports.LogError, "eventloop task HANG — node thread stuck", "kind", label, "seconds", int64(d/time.Second), "stack", string(stack))
	}
	// Queue-wait is the causal signal (PE 2026-08-15): a task can execute fast yet
	// blow a downstream deadline purely by WAITING behind a saturated thread. 2s is
	// well under the 8s request-timeout, so a token blind-sign that waited this long
	// is on its way to timing out — logged always-on (it only fires on pathology).
	loop.QueueWaitThreshold = 2 * time.Second
	loop.OnQueueWait = func(label string, wait time.Duration) {
		ports.LogIf(obs, ports.LogWarn, "eventloop task waited (thread saturated)", "kind", label, "wait_ms", wait.Milliseconds())
	}
	// The per-window budget decomposition is diagnostic detail (a line per active
	// kind every 30s) — debug-level by default (off at steady state), raised to
	// INFO by -loop-budget so a load run captures the full per-handler breakdown
	// WITHOUT the -log debug firehose (whose synchronous on-loop writes would skew
	// the very measurement). Carries execution time (cause) AND queue-wait (effect).
	budgetLevel := ports.LogDebug
	if *loopBudget {
		budgetLevel = ports.LogInfo
	}
	loop.SummaryEvery = 30 * time.Second
	loop.OnSummary = func(window time.Duration, stats map[string]eventloop.LabelSummary) {
		for kind, s := range stats {
			ports.LogIf(obs, budgetLevel, "eventloop budget", "window_s", int64(window/time.Second),
				"kind", kind, "count", s.Count, "total_ms", s.Total.Milliseconds(), "max_ms", s.Max.Milliseconds(),
				"wait_total_ms", s.TotalWait.Milliseconds(), "wait_max_ms", s.MaxWait.Milliseconds())
		}
	}

	// -relay: this daemon offers to forward ciphertext between NATed
	// peers. A capability, not infrastructure: any reachable node can do
	// this, none is special, and no relay is baked into the binary.
	if *relayServe != "" {
		rs, err := relay.Serve(*relayServe, ident, relay.Config{}, obs)
		if err != nil {
			return err
		}
		defer rs.Close()
		fmt.Printf("relay: serving at %s@%s (content-blind forwarding, capped)\n", id, rs.Addr())
		// PoD §7.3 Batch 3: bind the paid relay pump to the live node. With
		// --accept-relay-payments on, install the in-process seam so a paid connect
		// (a connect frame carrying a session handle) resolves to the node-owned
		// authorizer and runs the pay-as-you-go splice, settling at close. The seam is
		// additive (D-POD-RELAY-COEXIST, Option B): free swarm relay is unchanged and
		// shares the same caps; only a handle-marked connect takes the paid path, and
		// an unresolved handle is REFUSED, never downgraded to free. Without this
		// binding a paid-marked connect is refused (nil resolver), so free relay works
		// with or without payments.
		if *acceptRelayPayments {
			rs.SetPaidResolver(func(fetcher ports.NodeID, handle uint64) (relay.Authorizer, bool) {
				return nd.ResolveRelayAuthorizer(fetcher, handle)
			})
			rs.SetPaidSettler(nd.SettleRelaySessionForHandle)
			fmt.Println("relay: paid sessions bound — a handle-marked connect runs the pay-as-you-go splice (free relay unchanged, shared caps)")
		}
		// Gossip the capability on every envelope — but only in a form
		// peers can actually dial: a wildcard-bound relay borrows the
		// -advertise host, and with neither there is nothing worth
		// spreading (the swarm can't be sent "0.0.0.0:4002").
		if svc := dialableRelayAddr(rs.Addr(), *advertise); svc != "" {
			tr.SetRelayService(svc)
			fmt.Printf("relay: gossiping the service — NATed peers can discover %s@%s without -relay-via\n", id, svc)
		} else {
			fmt.Println("relay: wildcard bind and no -advertise — service not gossiped (peers must be told -relay-via by hand)")
		}
	}
	// -relay-via: parsed up front so a typo fails at start, not at the
	// moment we discover we're NATed and need it.
	var viaID ports.NodeID
	var viaAddr string
	if *relayVia != "" {
		ps, err := discovery.ParseList(*relayVia)
		if err != nil || len(ps) != 1 {
			return fmt.Errorf("-relay-via wants one RELAYID@HOST:PORT: %w", err)
		}
		viaID, viaAddr = ps[0].ID, ps[0].Addr
	}

	// Validator role: local chain replica, persisted and re-validated on
	// load; reputation judged from this daemon's own ledger observations.
	var attesterIDs []ports.NodeID
	var chainPath string
	// Assigned in the validator block once the chain exists; a no-op on a
	// chain-less daemon so the chain-gated call sites below never nil-panic.
	saveChain := func(string) {}
	ledger := credit.New(50_000, 500_000) // starter grant so a fresh publisher can pay token fees
	// R0.4b re-break F2: the cross-server double-redeem guard is DURABLE, and it is
	// restored HERE — before the node exists, so before any receipt can be accepted. A
	// restart used to evict every guarded token, in-window or not, and the identical
	// wire receipt paid a second time. A store that cannot be opened or read is a
	// refuse-to-start, the same rule the sign-mark store keeps: starting with an empty
	// guard IS the eviction this closes.
	//
	// GATED ON THE LANE (PE ruling H-3, 2026-09-03). The refuse-to-start is correct for
	// a node that BANKS receipts: starting with an empty guard is the eviction. It is
	// wrong for every other node, and it used to run unconditionally — so a corrupt
	// paidserials.log bricked a pure storage node that has no delivery lane and could
	// never have written the file. That is the F7 blast-radius lesson, which this same
	// change applied to demandkeys.cbor and did not apply to the file it adds. The
	// asymmetry with F7 is deliberate and is the whole point: inside the lane the guard
	// is load-bearing and a failure MUST stop the daemon; outside the lane nothing can
	// read or write it, so opening it at all is the defect.
	if *acceptReceipts {
		if gs, gerr := guardstore.Open(filepath.Join(*storeDir, "paidserials.log")); gerr != nil {
			return fmt.Errorf("delivery-credit guard store: %w", gerr)
		} else {
			ledger.SetPaidSerialStore(gs)
			if lerr := ledger.LoadPaidSerials(); lerr != nil {
				return fmt.Errorf("delivery-credit guard store: %w", lerr)
			}
		}
	}
	nd0ledger := ledger // wired onto the node below
	if *validator {
		anchorSet := map[ports.NodeID]bool{}
		for _, s := range strings.Split(*anchorList, ",") {
			if strings.TrimSpace(s) == "" {
				continue
			}
			aid, err := ports.ParseHash(strings.TrimSpace(s))
			if err != nil {
				return fmt.Errorf("anchor %q: %w", s, err)
			}
			anchorSet[aid] = true
		}
		if len(anchorSet) > 0 {
			// In OBJECTIVE mode the launch requirement is DERIVED — a strict anchor
			// majority ⌊A/2⌋+1 (#402), NOT the -anchor-quorum knob, so config can't
			// disable intersection. -anchor-quorum still applies in legacy (non-objective)
			// mode. Report the derived majority so the operator sees the real rule.
			fmt.Printf("training wheels: %d anchor(s), strict majority %d required (objective; derived #402), shed at %d independent validators\n",
				len(anchorSet), len(anchorSet)/2+1, *matureValidators)
		}
		// Objective fork-choice is the DEFAULT for an untrusted validator (the M0
		// consensus path). A trusted deployment (-min-rep 0, self-commit) does not
		// need it, so it auto-disables there rather than forcing anchor config on a
		// single trusted box.
		useObjective := *objective && *minRep > 0
		var minBondBytes int64
		if useObjective {
			mb, perr := parseSize(*minBond)
			if perr != nil || mb <= 0 {
				return fmt.Errorf("objective consensus needs a positive -min-bond: %q", *minBond)
			}
			minBondBytes = mb
			// Cold-start safe-default (red-team seam-2, 2026-08-08). A stock untrusted
			// objective validator with no scaffolding treats itself as MATURE from
			// genesis — Mature() returns true when MatureValidators<=0, so everMature
			// latches on the first block and the anchor co-sign the young regime needs
			// never engages: a young or Sybil quorum can self-certify mature and
			// capture. There is no sound *synthesizable* anchor set (weak-subjectivity
			// irreducibility — you cannot bootstrap trust in the validator set from the
			// validator set), so the safe default is to REFUSE, not warn (mirroring the
			// -min-bond hard failure above). Two legitimate paths satisfy it: LAUNCH a
			// fresh network with the anchor training-wheels set, or JOIN an already-
			// mature one with a weak-subjectivity checkpoint. Asserted safe-by-default
			// in invariant_b_test.go (S6).
			if !coldStartScaffoldOK(useObjective, len(anchorSet), *matureValidators, *wsCheckpoint) {
				return fmt.Errorf("consensus: refusing to start — an untrusted objective validator with no cold-start scaffolding would treat itself as mature from genesis (no anchor co-sign), letting a young or Sybil quorum self-certify and capture. Launch a fresh network with -anchors ID,... and -mature-validators N (the training-wheels launch set), OR join an already-mature network with -ws-checkpoint HEIGHT:HASH; alternatively -min-rep 0 for a trusted swarm, or -objective=false for the legacy (non-M0) path")
			}
		}
		// The objective anti-release floor and re-challenge cadence (retest G4)
		// ride the same knobs as the node-side floor: a sub-floor bond earns no
		// on-chain standing, and standing lapses without a fresh proof within the
		// TTL. cfg.MinBondBytes is 0 unless -min-bond-floor was set.
		var wsCP chain.WSCheckpoint
		if *wsCheckpoint != "" {
			parts := strings.SplitN(*wsCheckpoint, ":", 2)
			if len(parts) != 2 {
				return fmt.Errorf("-ws-checkpoint must be HEIGHT:HASH, got %q", *wsCheckpoint)
			}
			h, herr := strconv.ParseUint(parts[0], 10, 64)
			if herr != nil {
				return fmt.Errorf("-ws-checkpoint height %q: %w", parts[0], herr)
			}
			hash, perr := ports.ParseHash(parts[1])
			if perr != nil {
				return fmt.Errorf("-ws-checkpoint hash %q: %w", parts[1], perr)
			}
			wsCP = chain.WSCheckpoint{Height: h, Hash: hash}
			fmt.Printf("chain: weak-subjectivity checkpoint pinned at %d:%s — a reorg at or before it is refused (F-1)\n", h, hash)
		}
		// The #535 recovery directive is consensus-coordination config: refuse a
		// value that can never fire (a non-boundary height) instead of silently
		// arming nothing, and announce loudly when it IS armed — an operator
		// must know this replica will re-base one boundary against the live set.
		if *livenessRecoveryHeight > 0 {
			if effEpoch == 0 || *livenessRecoveryHeight%effEpoch != 0 {
				return fmt.Errorf("-liveness-recovery-height %d is not an epoch boundary (epoch cadence %d): the #535 recovery re-bases at a finalized boundary only", *livenessRecoveryHeight, effEpoch)
			}
			fmt.Printf("chain: #535 LIVENESS RECOVERY ARMED at boundary %d — this replica will validate that one boundary against the LIVE qualified bonded set (weak-subjectivity trust: every honest operator must set the SAME height, and the operator vouches the > 1/3 weight loss is a real outage, not an attack)\n", *livenessRecoveryHeight)
		}
		ch := chain.New(chain.Config{
			MinProposerRep: *minRep, MinAttesterRep: *minRep, Quorum: *quorum,
			ByzantineQuorum: effByz,
			Anchors:         anchorSet, AnchorQuorum: *anchorQuorum, MatureValidators: *matureValidators,
			OperatorMargin: effMargin,
			AllowPublisher: *allowPublisher, MinBond: minBondBytes,
			MinBondBytes: cfg.MinBondBytes, BondTTLBlocks: effTTL,
			EpochBlocks:            effEpoch,
			WSCheckpoint:           wsCP,
			LivenessRecoveryHeight: *livenessRecoveryHeight,
			Archive:                *archive,
		}, ledger.Reputation)
		if *allowPublisher {
			fmt.Println("publisher: durable Publisher entries PERMITTED — publishes may record permanent linkage (trusted deployment)")
		}
		chainPath = filepath.Join(*storeDir, "chain.cbor")
		// #572 ROOT CAUSE FIX — wire the bond verifier BEFORE the replay.
		// objective() is MinBond>0 AND verifyBond!=nil; EnableObjectiveChain
		// used to wire it ~80 lines below, so every restore replayed under the
		// LEGACY rep-gated qualification (empty boot ledger ⇒ validatorsSeen
		// rebuilt EMPTY ⇒ the everMature latch silently lost ⇒ the restored
		// validator demanded launch-rule anchors for mature commits, forever —
		// the 474718e-deep/8a52aba-deep wedge, proven by the save/restore
		// regime pairs). Reload now also REFUSES an objective-config replay
		// with no verifier, so this ordering can never regress silently.
		ch.SetBondVerifier(node.SpaceTimeBondVerifier(cfg.BondVDFDelay, cfg.BondLabelSamples))
		if n, err := chainstore.Replay(chainPath, ch); err != nil {
			// NEVER quiet (#558): a replay failure discards finalized history.
			// Reload keeps the longest valid prefix; name the loss and the
			// consequence loudly — behind the swarm's prune horizon a
			// genesis-stranded validator cannot re-sync without an operator
			// -ws-checkpoint (#559).
			fmt.Fprintf(os.Stderr, "chain replay: FAILED at block %d: %v — continuing with the %d-block valid prefix; the suffix must re-sync from peers, which is IMPOSSIBLE below the swarm's prune horizon without a fresh -ws-checkpoint (#558/#559)\n", n, err, n)
		} else if n > 0 {
			// The regime line is LOAD-BEARING diagnostics (#572): 474718e-deep's
			// val-d restored 32 blocks whose live application had latched
			// everMature — and the restored replica demanded launch-rule anchors
			// forever. Chain-level replay is proven pure (write-site audit + the
			// field-shape oracle), so the next divergence must name which map
			// failed to rebuild — this line does that at every restore.
			r := ch.Regime()
			fmt.Printf("chain: restored %d block(s) from disk (everMature=%v matureEpoch=%v seen=%d bonded=%d epochStart=%d epochSet=%d)\n",
				n, r.EverMature, r.MatureEpoch, r.ValidatorsSeen, r.Bonded, r.EpochStart, r.EpochSetSize)
		}
		// Every fresh chain is born carrying the founding manifesto at
		// height 0 (declared, not agreed), and the daemon seeds the
		// genesis file into its own store so the whole swarm always hosts
		// it. Idempotent across restarts: genesis is deterministic and
		// chainstore already restored it if present.
		if ch.Len() == 0 {
			if gb, gh, _, gerr := genesis.Build(store); gerr == nil {
				if err := ch.AppendGenesis(gb); err == nil {
					fmt.Printf("genesis: %s\n", gh)
				}
			} else {
				fmt.Fprintln(os.Stderr, "genesis seed:", gerr)
			}
		}
		nd.EnableChain(ch, ident.Signer())
		// saveChain (assigned below, declared at daemon scope) persists the replica AND prints the regime snapshot + head it
		// went down with (#572 premise-(a)/(c) discriminator): paired with the
		// restore-time regime line, the next under-latch names its layer in one
		// diff — last-save regime ≠ restore regime ⇒ store/replay; equal-but-
		// wedged ⇒ downstream of restore; head hash pins the content itself.
		saveChain = func(why string) {
			if err := chainstore.Save(chainPath, ch.Blocks(0)); err != nil {
				fmt.Fprintln(os.Stderr, "chain save:", err)
				return
			}
			head, next := ch.Head()
			r := ch.Regime()
			fmt.Printf("chain: saved %d block(s) [%s] head=%d:%s (everMature=%v matureEpoch=%v seen=%d bonded=%d epochStart=%d epochSet=%d)\n",
				next, why, next-1, head, r.EverMature, r.MatureEpoch, r.ValidatorsSeen, r.Bonded, r.EpochStart, r.EpochSetSize)
		}
		// Durable never-sign-twice watermark (#397 Q1b): the mark is fsync'd
		// BEFORE any consensus signature is released, so a crash/restart cannot
		// make this validator contradict a signature it already shipped — which
		// would be a permanent honest self-slash (F2). A load failure is
		// refuse-to-start: running without the mark re-opens that window.
		if err := nd.SetSignMarkStore(markstore.New(filepath.Join(*storeDir, "signmark.json"))); err != nil {
			fmt.Fprintln(os.Stderr, "sign-mark:", err)
			os.Exit(1)
		}
		if *honorRevocations {
			nd.SetHonorChainRevocations(true) // per-operator opt-in (F5); default OFF
			fmt.Println("takedowns: honoring on-chain revocations (per-operator subscription, F5)")
		}
		if sz, perr := parseSize(*bondSize); perr == nil && sz > 0 {
			if nd.EnableBond(ident.Signer(), sz) {
				// Reloaded the existing plot — a restart reuses it, no re-plot
				// (#93). Say so; logging "sealed" would falsely suggest the
				// expensive one-time plotting ran again (acceptance F7).
				fmt.Printf("bond: reloaded the %s storage bond for consensus standing (no re-plot)\n", *bondSize)
			} else {
				fmt.Printf("bond: sealed a %s storage bond for consensus standing\n", *bondSize)
			}
		}
		// Objective fork-choice (F6): wire the on-chain-bond verifier so
		// registrations are re-checked against the real space-time primitive, and
		// (via proposeBlock) this validator registers its own bond live as it
		// proposes. Must follow EnableBond so a bond exists to register.
		if useObjective {
			if bsz, _ := parseSize(*bondSize); bsz < minBondBytes {
				return fmt.Errorf("objective consensus requires -bond (%s) to clear -min-bond (%s)", *bondSize, *minBond)
			}
			nd.EnableObjectiveChain()
			fmt.Printf("consensus: OBJECTIVE fork-choice (default; on-chain bond ≥ %s); %d anchor(s) bootstrap the launch set, shed at maturity\n",
				*minBond, len(anchorSet))
		} else if *minRep == 0 {
			fmt.Println("consensus: legacy self-commit (trusted deployment, -min-rep 0) — objective fork-choice off")
		} else {
			fmt.Println("consensus: legacy subjective fork-choice (-objective=false) — does NOT hold the M0 denial under an adversarial partition")
		}
		// Publisher privacy (T3): this validator issues blind-signed publish
		// tokens, and (when -require-tokens) the chain accepts only entries that
		// carry one — no Publisher identity on-chain. The issuer key PERSISTS
		// (#93 / §3d): a restart reuses it, so outstanding tokens stay verifiable
		// and peers' cached issuer keys don't go stale.
		// R0.4b demand-key rotation state: the epoch the band was last built for,
		// and the (off-loop) rotation step. Both stay nil/zero unless
		// -accept-delivery-receipts wires the demand lane.
		var rotateDemandKeys func(cur uint64)
		var demandEpoch uint64
		if is, ierr := diskissuer.Open(filepath.Join(*storeDir, "issuer")); ierr != nil {
			return fmt.Errorf("token issuer store: %w", ierr)
		} else if issuerKey, kerr := is.LoadOrCreate(rand.Reader); kerr == nil {
			nd.EnableTokenIssuer(rand.Reader, issuerKey)
			if *acceptReceipts {
				// PoD neutral lane: bank receipts against the key that signed
				// their tokens. issuer == server here — the bilateral shape the
				// certification's Q5 settlement answer covers (per-node
				// bookkeeping suffices; committed balances are only needed for a
				// credit a THIRD operator must honor).
				//
				// R0.4b C3 close: the demand issuer key is PER-EPOCH, SCHEDULED
				// here, and is NEVER the publish key. Two rules, both load-bearing:
				//
				//   - THE PUBLISH KEY NEVER ENTERS THE DEMAND KEYSET. It cannot be
				//     rotated (committed publish tokens re-verify against it on
				//     replay), so installing it as key_{boot} gave the demand lane a
				//     life of exactly W+1 epochs — after which the bank rejected
				//     every token while the fee-charging paths kept charging. With
				//     the lanes separated, a demand blind bought on the publish lane
				//     is simply not a demand token, and every shipped withdrawal
				//     path now runs on the pinned demand lane.
				//   - ROTATION IS SCHEDULED, NOT OPS POLICY. The band [cur, cur+W]
				//     is pre-published so key_E is committed before epoch E opens,
				//     and [cur−W, cur] is retained so an in-window past epoch is
				//     still signable (the epoch-boundary race). Keygen and the disk
				//     write run OFF the node loop; only the installs are posted back.
				//
				// Until the commitment for an epoch is on-chain, the bank verifies
				// nothing — refusing an unanchored key is the certified behavior, not
				// a bug (without the binding, per-epoch keys are worse for privacy
				// than no epoch at all).
				des, derr := diskissuer.OpenEpochs(filepath.Join(*storeDir, "issuer"))
				if derr != nil {
					return fmt.Errorf("demand issuer key store: %w", derr)
				}
				// DEGRADE TO LANE-OFF, NEVER TO DAEMON-DEAD (red-team re-break F7).
				// A corrupt or unreadable demand key file used to come straight out of
				// runDaemon, so one bad byte in demandkeys.cbor stopped chain participation,
				// storage and serving too — a blast radius wildly out of proportion to an
				// OPTIONAL receipt lane. The file is NEVER rewritten or regenerated on this
				// path: quietly minting new keys over already-committed fingerprints is the
				// UNRECOVERABLE failure (F6), so the store is left exactly as found for an
				// operator to restore. The refusal is loud, on stdout, at boot.
				// Build the issuer-key hardness primorial off the node loop, once, so
				// the first key pin does not pay ~40 ms on the loop (advisory C-3).
				blindtoken.PrewarmValidatePub()
				_, lerr := des.Load()
				switch {
				case lerr != nil:
					fmt.Printf("delivery receipts: LANE OFF — the demand issuer key store is unreadable: %v\n", lerr)
					fmt.Printf("delivery receipts: %s was NOT modified; restore it from backup and restart to re-enable the lane. Chain, storage and serving continue.\n",
						filepath.Join(*storeDir, "issuer", "demandkeys.cbor"))
				default:
					// Boot install runs synchronously: the lane must not be dark for the
					// first commits, and this is before the loop is running.
					demandEpoch = nd.DemandEpoch()
					if kerr := installDemandKeys(nd, des, rand.Reader, demandEpoch); kerr != nil {
						fmt.Printf("delivery receipts: LANE OFF — the boot key rotation failed: %v\n", kerr)
						break
					}
					// ARM THE EPOCH SCHEDULER — ONLY NOW, below the boot install's one
					// failure exit (PE ruling H-2, 2026-09-03). This assignment used to sit
					// ABOVE the install, so a failed boot rotation printed "LANE OFF" and
					// `break`ed out with rotateDemandKeys still non-nil: the OnCommit hook
					// kept rotating from the next epoch turn, and the node went on holding
					// demand keys, STAGING IssuerKeyReg commitments into consensus, serving
					// MsgGetDemandIssuerKeys and blind-signing withdrawals — charging the
					// withdrawal fee — while demandBank stayed nil and denied every receipt
					// those tokens bought. That is the "a fee burned for a token the system
					// will never honour" failure this scheduler exists to close, resurrected
					// on the degrade path.
					//
					// The fix is the ORDER, not a compensating `rotateDemandKeys = nil` on
					// the failure branch: with the only assignment below the only failure
					// exit, there is no branch left that can arm a lane it has just declared
					// off. Pinned by cmd/silt TestDaemonArmsTheRotatorOnlyAfterABootInstall.
					//
					// The band is generated and written to disk OFF the node loop (an
					// RSA-2048 keygen is hundreds of milliseconds and the loop is
					// single-threaded), and only the installs are posted back onto it.
					rotateDemandKeys = func(cur uint64) {
						var band []struct {
							e uint64
							k *rsa.PrivateKey
						}
						if err := des.RotateWindow(rand.Reader, cur, demand.DefaultWindow,
							func(e uint64, k *rsa.PrivateKey) {
								band = append(band, struct {
									e uint64
									k *rsa.PrivateKey
								}{e, k})
							}); err != nil {
							fmt.Printf("delivery receipts: demand key rotation for epoch %d FAILED: %v\n", cur, err)
							return
						}
						loop.Post("demand-keys", func() {
							for _, kv := range band {
								nd.SetDemandIssuerKey(rand.Reader, kv.e, kv.k)
							}
						})
					}
					nd.EnableDemandBank(nd.ID())
					fmt.Println("delivery receipts: ACCEPTING — banking witnessed deliveries and settling the conserved delivery credit (balance only, never standing)")
					fmt.Printf("delivery receipts: token validity window = %d epochs; per-epoch demand keys pre-published to epoch %d; key_E is resolved against the committed E→key binding (needs an era-4/v5 chain)\n",
						demand.DefaultWindow, demandEpoch+demand.DefaultWindow)
				}
			}
			if *acceptRelayPayments {
				// PoD relay lane (§7.3, certified 2026-08-30): accept sender-funded
				// PayWord chains for forwarding content-blind bytes. Settlement is
				// balance only, never standing (the γ→1/N firewall). The two M0
				// guards (ephemeral-blind funding; fresh identity + chain per
				// session) are enforced by OpenRelaySession.
				nd.EnableRelayAccept()
				fmt.Println("relay payments: ACCEPTING — verifying sender-funded PayWord chains and settling relay credit (balance only, never standing)")
			}
			if *requireTokens > 0 {
				ch.RequireTokens(*requireTokens, nd.IssuerKeyOf)
				fmt.Printf("publish tokens: required (%d validator signatures), issuing\n", *requireTokens)
			}
		} else {
			return fmt.Errorf("token issuer key: %w", kerr)
		}
		nd.OnSlash(func(culprit ports.NodeID, height uint64) {
			fmt.Printf("chain: slashed equivocator %s (double-signed at height %d)\n", culprit, height)
		})
		nd.OnReorg(func(dropped int, newHeight uint64) {
			fmt.Printf("chain: reorged onto a heavier fork (dropped %d block(s), new head height %d)\n", dropped, newHeight)
		})
		// λ_H arrival-rate observer (CT-1 conditional theorem, §6): trails the committed
		// distinctness coefficient A(t) to record the honest-arrival RATE and alarm on a
		// floor exit. Ephemeral observer state — the chain stays a pure reader.
		lambdaH := newLambdaHTracker(*lambdaHWindow)
		nd.OnCommit(func(b chain.Block) {
			// R0.4b: advance the per-epoch demand key schedule when the consensus
			// epoch turns. Generating RSA keys and writing them to disk would block
			// the single loop for hundreds of milliseconds, so the work runs in a
			// goroutine and posts the installs back.
			//
			// Bumping demandEpoch here, on the loop, before the goroutine starts
			// prevents a DUPLICATE rotation for the SAME epoch. It does NOT serialize
			// rotations: a band is ~5 RSA keygens ~1s and an epoch is 8 blocks, so
			// turns for DIFFERENT epochs routinely overlap and complete out of order.
			// That is the case that used to lose keys (red-team F6), and the store —
			// not this comment — is what makes it safe: EpochStore serializes the whole
			// load-generate-save cycle and its prune's upper edge is monotone, so no
			// turn can shrink another turn's pre-publication. See
			// adapters/diskissuer/epochkeys.go.
			if rotateDemandKeys != nil {
				if cur := nd.DemandEpoch(); cur > demandEpoch {
					demandEpoch = cur
					go rotateDemandKeys(cur)
				}
			}
			// bond-regs is the DRAIN CURVE's per-block resolution: without it a
			// journal cannot say which committed blocks banked which registrations
			// (run 09fbe60-84613 read entries=0 as "empty" while regs were landing).
			fmt.Printf("chain: committed block %d (%d entries, %d bond-regs, %d attestations)\n",
				b.Height, len(b.Entries), len(b.BondRegs), len(b.Atts))
			// C2 — no-quiet-capture concentration, from the COMMITTED bond ledger
			// (not gossip). Operators is the coefficient discounted by the operator
			// margin M; the training wheels are shed only once it clears the maturity bar.
			if ch.Objective() {
				m := ch.C2Metric()
				// In objective mode the launch gate is DERIVED (strict anchor majority
				// ⌊A/2⌋+1, #402) and cannot be disabled by config — so this line is true
				// whenever the network is objective and young, not contingent on a flag.
				wheels := "engaged (young network — strict anchor majority still required)"
				if ch.EverMature() {
					// One-way latch (F-1): once shed, the anchors never re-arm.
					wheels = "shed permanently (network matured — F-1 one-way latch)"
					if !ch.Mature() {
						wheels = "shed permanently (matured; live decentralization has since dropped — real-bond super-quorum in force, anchors NOT re-armed)"
					}
				}
				fmt.Printf("  C2: nakamoto %d bonds → %d operators (margin ×%d) | cost-to-corrupt %d MiB of %d MiB bonded across %d | concentration HHI %.2f Gini %.2f top %.0f%% uniformity %.0f%% | wheels %s\n",
					m.NakamotoBonds, m.NakamotoOperators, m.Margin,
					m.CostToCorruptBytes>>20, m.TotalBondedBytes>>20, m.Participants,
					m.HHI, m.Gini, m.TopShare*100, m.WeightUniformity*100, wheels)
				// Concentration alarm (D-C2 / F-1 follow-up): the honest whale C2 cannot
				// close on-chain, made LOUD out-of-band. A single bond at/above the ⅓
				// Byzantine capture fraction is one step from being able to stall or
				// (with more) capture consensus — a social/operational trigger, not an
				// on-chain enforcement (impossible per Kwon).
				if m.TopShare >= 1.0/3 {
					fmt.Printf("  ⚠ CONCENTRATION ALARM: one bond holds %.0f%% of bonded weight (≥ the ⅓ capture fraction) — real standing is concentrating; this is the honest-whale residual C2 measures but cannot close on-chain. Act out-of-band.\n", m.TopShare*100)
				}
				// Atomization note (seam-5): the whale alarm above reads TopShare, which an
				// equal-bond SPLIT (one operator, many identical min-bonds) drives to its
				// most-decentralized value — invisible to it. When the weight signals look
				// clean (no whale) but the distribution is many bonds at implausibly uniform
				// weight, surface the "many atoms" fingerprint so the operator verifies real
				// independence out-of-band. Necessary-not-sufficient (#182): a size-varying
				// splitter evades it, and healthy decentralization is also uniform.
				if m.TopShare < 1.0/3 && m.Participants >= 8 && m.WeightUniformity >= 0.9 {
					fmt.Printf("  ⓘ atomization note: %d bonds at near-identical weight (uniformity %.0f%%) read as maximally decentralized on HHI/Gini/top-share, but an equal-bond SPLIT (one operator across many keys) produces exactly this fingerprint. The weight signals can't tell it from real decentralization (#182) — verify independent operators via out-of-band address/timing diversity.\n", m.Participants, m.WeightUniformity*100)
				}
				// Export the committed head as a copy-pasteable weak-subjectivity
				// checkpoint (F-1): a fresh/long-offline node pins one via -ws-checkpoint
				// to refuse a long-range reorg. Publish it / cross-check across nodes.
				fmt.Printf("  checkpoint: %d:%s\n", b.Height, b.Hash())
				// λ_H — the honest-arrival RATE the CT-1 conditional theorem is owed
				// (research cert C1-maturity-before-capture-CONDITIONAL-THEOREM-LIFT-2026-08-27,
				// §6). A(t) is the SAME operator/domain-distinct distinctness the shed gates
				// on (MatureCoefficient); λ_H = ΔA/Δheight over the trailing window is the
				// realized net arrival rate. RECORD it; it parameterizes the certification,
				// not the code (reads the committed metric, changes no rule).
				a := ch.MatureCoefficient()
				lambdaH.observe(b.Height, a)
				if lambdaH.ready() {
					fmt.Printf("  λ_H: %.3f distinct arrivals/height (A=%d over %d heights) | shed at %d\n",
						lambdaH.rate(), a, lambdaH.span(), *matureValidators)
				} else {
					fmt.Printf("  λ_H: window filling (A=%d) — arrival rate reported once ≥2 heights are trailed\n", a)
				}
				// Floor-exit alarm (§6/§271): the launch was certified against an honest-
				// arrival floor. If the measured λ_H falls below it WHILE THE NETWORK IS
				// STILL YOUNG (pre-maturity latch), the deployment has left hypothesis H —
				// T_mature→∞, maturity-precedes-capture no longer holds — and the operator
				// must NOT treat CT-1 as holding. After the one-way latch (EverMature) the
				// floor is moot (P4: post-maturity concentration cannot re-arm anchors), so
				// the alarm is gated on !EverMature(); the λ_H LINE above always prints.
				if !ch.EverMature() && lambdaH.belowFloor(*lambdaHFloor) {
					fmt.Printf("  ⚠ λ_H FLOOR-EXIT: honest-arrival rate %.3f/height is BELOW the certified floor %.3f/height while the network is still young — the launch has LEFT the CT-1 hypothesis (T_mature→∞; maturity-precedes-capture is no longer proven). Do NOT treat the maturity race as certified. Investigate stalled/reversed honest bonded arrivals out-of-band.\n",
						lambdaH.rate(), *lambdaHFloor)
				}
			}
			saveChain("commit")
		})
		for _, s := range strings.Split(*attesters, ",") {
			if strings.TrimSpace(s) == "" {
				continue
			}
			aid, err := ports.ParseHash(strings.TrimSpace(s))
			if err != nil {
				return fmt.Errorf("attester %q: %w", s, err)
			}
			attesterIDs = append(attesterIDs, aid)
		}
	}

	// A validator running below the safe thresholds self-commits the
	// registry (a lone/fresh node can rubber-stamp). Legitimate for a
	// one-box or fully-trusted swarm, dangerous on an open network — so it
	// is an explicit, LOUD choice, never a silent default.
	if *validator && (*quorum < 1 || *minRep <= 0) {
		fmt.Printf("⚠ trusted-deployment mode (quorum %d, min-rep %d): this validator self-commits the registry — safe only for a one-box or fully-trusted swarm, NOT an open network\n", *quorum, *minRep)
	}

	// -care with no registry would silently never caretake (the care loop below
	// requires reg != nil to resolve the entry): a caretaker that looks healthy
	// and repairs nothing is the #235 silent-skip shape, so refuse to start
	// instead — the operator meant to caretake and the config can't.
	if *care != "" && *serveRegistry == "" && *registryURL == "" {
		return fmt.Errorf("-care needs a registry to resolve the cared entry: add -registry ID@https://host:port (or -serve-registry)")
	}
	var reg ports.Registry
	switch {
	case *serveRegistry != "" && *validator:
		host := &chainhost.Host{Loop: loop, Node: nd,
			Attesters: attesterIDs, Broadcast: attesterIDs, Quorum: *quorum}
		bound, _, err := httpregistry.ServeTLS(*serveRegistry, ident, host)
		if err != nil {
			return err
		}
		reg = host
		fmt.Printf("registry: chain-backed, serving %s@https://%s (quorum %d)\n", id, bound, *quorum)
	case *serveRegistry != "":
		freg, err := fileregistry.Open(filepath.Join(*storeDir, "registry.jsonl"))
		if err != nil {
			return err
		}
		bound, _, err := httpregistry.ServeTLS(*serveRegistry, ident, freg)
		if err != nil {
			return err
		}
		reg = freg
		fmt.Printf("registry: serving %s@https://%s (persisted in %s)\n", id, bound, *storeDir)
	case *registryURL != "":
		reg, err = openRegistry(*registryURL)
		if err != nil {
			return err
		}
		fmt.Printf("registry: %s\n", *registryURL)
	}

	if *denylistPath != "" {
		f, err := os.Open(*denylistPath)
		if err != nil {
			return err
		}
		dl := denylist.New()
		if err := denylist.LoadInto(dl, f); err != nil {
			f.Close()
			return err
		}
		f.Close()
		nd.SetDenylist(dl)
		if purged := nd.EnforceDenylist(); purged > 0 {
			fmt.Printf("denylist: %d root(s) denied; purged %d held chunk(s)\n", dl.Len(), purged)
		} else {
			fmt.Printf("denylist: honoring %d denied root(s)\n", dl.Len())
		}
	}

	fmt.Printf("peer: %s@%s\n", id, tr.Addr())
	fmt.Printf("store: %s\n", *storeDir)

	if *uiAddr != "" {
		var capRep ports.CapacityReporter
		if rep, ok := store.(ports.CapacityReporter); ok {
			capRep = rep
		}
		token, err := loadOrCreateUIToken(*storeDir)
		if err != nil {
			return err
		}
		ui := &uiServer{
			loop: loop, nd: nd, reg: reg, capRep: capRep,
			selfPeer:  fmt.Sprintf("%s@%s", id, tr.Addr()),
			validator: *validator, started: time.Now(),
			peerCount:     func() int { return tr.PeerCount() },
			carePublished: *carePublished,
			token:         token,
		}
		bound, err := ui.serve(*uiAddr)
		if err != nil {
			return err
		}
		// The token rides the URL query so the operator's browser is
		// authorized in one click; state-changing calls need it, reads don't.
		fmt.Printf("ui: http://%s/?token=%s\n", bound, token)
	}

	// Discovery, in layers: explicit flag, DNS seed, and the persisted
	// address book from last run.
	peersPath := filepath.Join(*storeDir, "peers.json")
	var seeds []ports.NodeID
	seeded := make(map[ports.NodeID]bool)
	addSeeds := func(peers []tcpnet.Peer, source string) {
		for _, p := range peers {
			if p.ID == id {
				continue
			}
			// A peer can appear once per address form (direct + relay);
			// both feed the book, the ID seeds the bootstrap once.
			tr.AddPeer(p.ID, p.Addr)
			if !seeded[p.ID] {
				seeded[p.ID] = true
				seeds = append(seeds, p.ID)
			}
		}
		if len(peers) > 0 {
			fmt.Printf("discovery: %d peer(s) via %s\n", len(peers), source)
			dlog("discovery", "peers", len(peers), "source", source)
		}
	}
	if *bootstrap != "" {
		ps, err := discovery.ParseList(*bootstrap)
		if err != nil {
			return err
		}
		addSeeds(ps, "-bootstrap")
	}
	// The static consensus/anchor tier (#286 Layer 2): configure the validator set's
	// addresses up front so a proposer can INITIATE the gather at genesis, and mark
	// each never-evicted so a transient WAN miss doesn't tear it out of the mesh
	// (docs/network-durability.md §8/§2). addSeeds already AddPeer's the address +
	// seeds the Kademlia join; AddStaticPeer adds the eviction exemption.
	if *persistentPeers != "" {
		ps, err := discovery.ParseList(*persistentPeers)
		if err != nil {
			return fmt.Errorf("-persistent-peers: %w", err)
		}
		addSeeds(ps, "-persistent-peers")
		nStatic := 0
		for _, p := range ps {
			if p.ID == id {
				continue
			}
			nd.AddStaticPeer(p.ID)
			nStatic++
		}
		if nStatic > 0 {
			fmt.Printf("persistent-peers: %d configured (static, never-evicted consensus tier)\n", nStatic)
		}
	}
	if *dnsSeed != "" {
		if ps, err := discovery.FromDNS(*dnsSeed); err == nil {
			addSeeds(ps, "dns:"+*dnsSeed)
		} else {
			fmt.Fprintln(os.Stderr, "dns seed:", err)
		}
	}
	if ps, err := discovery.LoadFile(peersPath); err == nil {
		addSeeds(ps, "peers.json (warm restart)")
	}
	// Local-network discovery: announce on the LAN and fold any peer we
	// hear into the routing table. This is the zero-config rung — two nodes
	// in one house find each other with no flags at all.
	if *mdns {
		if adv, err := lan.AdvertiseAddr(tr.Addr()); err != nil {
			fmt.Fprintln(os.Stderr, "mdns: local discovery off —", err)
		} else {
			beacon := lan.New(tcpnet.Peer{ID: id, Addr: adv}, func(p tcpnet.Peer) {
				loop.Post("mdns", func() {
					tr.AddPeer(p.ID, p.Addr)
					nd.Bootstrap([]ports.NodeID{p.ID}, func() {})
					fmt.Printf("mdns: discovered %s@%s\n", p.ID, p.Addr)
					dlog("mdns discovered", "peer", p.ID, "addr", p.Addr)
				})
			})
			if err := beacon.Start(); err != nil {
				fmt.Fprintln(os.Stderr, "mdns:", err)
			} else {
				defer beacon.Close()
				fmt.Printf("mdns: announcing %s on the local network\n", adv)
			}
		}
	}
	// Persist the living address book so the next start needs no flags —
	// but only peers we've actually reached, not every address ever
	// observed. Otherwise a warm restart reloads a graveyard of dead
	// ephemeral publisher identities and drowns lookups in timeouts (#43).
	// The reachable set lives on the (lock-free) node loop, so snapshot it
	// there and do the disk write off-loop.
	go func() {
		for range time.Tick(30 * time.Second) {
			done := make(chan []tcpnet.Peer, 1)
			loop.Post("peer-persist", func() {
				reachable := nd.ReachablePeers()
				all := tr.Peers()
				live := all[:0]
				for _, p := range all {
					if reachable[p.ID] {
						live = append(live, p)
					}
				}
				done <- live
			})
			discovery.SaveFile(peersPath, <-done)
		}
	}()
	// leanOnRelay registers with a relay (configured via -relay-via or
	// discovered through gossip), switches our advertised address to the
	// relay form, and — the important part — RE-bootstraps: the first
	// bootstrap of a NATed node may have come up empty (peers had no way
	// to answer someone with no dialable address), so the join is
	// retried now that every envelope carries an address the swarm can
	// actually reach us at.
	leanOnRelay := func(viaID ports.NodeID, viaAddr string) {
		rc, err := relay.NewClient(ident, viaID, viaAddr, tr.RelayInbound, obs)
		if err != nil {
			fmt.Fprintln(os.Stderr, "relay-via:", err)
			return
		}
		// #27: let the transport upgrade a relay path to a direct one. The
		// relay coordinates (RequestPunch); when it signals us, punch the peer
		// from our registration port (HolePunch).
		rc.SetOnPunch(func(peer ports.NodeID, peerAddr string, localPort int) {
			tr.HolePunch(peer, peerAddr, localPort)
		})
		tr.SetRequestPunch(rc.RequestPunch)
		go rc.Run(func(err error) {
			if err != nil {
				fmt.Fprintln(os.Stderr, "relay-via: registration failed:", err)
				return
			}
			loop.Post("relay-via", func() {
				tr.SetAdvertise(rc.Addr())
				fmt.Printf("relay-via: registered — peers reach us at %s\n", rc.Addr())
				dlog("relay-via registered", "addr", rc.Addr())
				if seen := rc.Observed(); seen != "" { // STUN-style, for hole-punching (#27)
					nd.SetObservedAddr(seen)
					fmt.Printf("relay-via: this node's public endpoint looks like %s (observed by the relay)\n", seen)
					dlog("observed public endpoint", "addr", seen)
				}
				nd.Bootstrap(seeds, func() {
					fmt.Printf("re-bootstrapped through the relay (%d table entries)\n", nd.Table().Size())
					dlog("re-bootstrapped via relay", "table", nd.Table().Size())
					nd.AnnounceHeld(func(int) {})
				})
			})
		})
	}
	nd.SetLedger(nd0ledger)
	if *liar {
		nd.SetLiar(true) // RED-TEAM (#232): keep the proof tags, drop the bytes
		fmt.Println("RED-TEAM: running as a PoR LIAR (keeps proof tags, drops shard bytes) — never honest")
	}
	loop.Post("bootstrap", func() {
		// Self-heal if the join races a not-yet-listening bootstrap target: while
		// the table stays empty, re-run the Kademlia join against the seeds so the
		// node recovers on its own instead of staying isolated (#281).
		nd.StartBootstrapRetry(func(size int) {
			fmt.Printf("re-bootstrapped: recovered from an empty routing table (%d table entries)\n", size)
			dlog("re-bootstrapped from empty table", "table", size)
		})
		nd.Bootstrap(seeds, func() {
			fmt.Printf("bootstrapped (%d table entries)\n", nd.Table().Size())
			dlog("bootstrapped", "table", nd.Table().Size())
			// Reachability (AutoNAT): ask a couple of known peers to dial us
			// back. A public node advertises its direct address; a NATed one
			// leans on the -relay-via relay, if given.
			if helpers := nd.Table().Closest(nd.ID(), 3); len(helpers) > 0 {
				nd.CheckReachability(helpers, func(reachable bool) {
					switch {
					case reachable:
						fmt.Println("reachability: public — peers can dial this node directly")
					case *relayVia != "":
						fmt.Println("reachability: no peer could dial back — NATed; leaning on the -relay-via relay")
						leanOnRelay(viaID, viaAddr)
					default:
						// No relay configured — but the swarm gossips
						// relay capability, so one may already be (or soon
						// become) known. Adopt the first that shows up.
						fmt.Println("reachability: no peer could dial back — this node looks NATed; watching the swarm for a gossiped relay (-relay-via RELAYID@HOST:PORT skips the wait)")
						dlog("natted, watching for gossiped relay")
						// First cut: adopt the lowest-ID gossiped relay and
						// commit to it (leanOnRelay then reconnects it with
						// backoff for the node's lifetime, exactly as an
						// explicit -relay-via would). Choosing among several
						// relays and failing over when the chosen one won't
						// register is a documented follow-up (see BACKLOG).
						go func() {
							for {
								if rs := tr.KnownRelays(); len(rs) > 0 {
									r := rs[0]
									fmt.Printf("relay: discovered %s@%s via gossip — leaning on it\n", r.ID, r.Addr)
									dlog("gossiped relay adopted", "relay", r.ID, "addr", r.Addr)
									leanOnRelay(r.ID, r.Addr)
									return
								}
								time.Sleep(5 * time.Second)
							}
						}()
					}
				})
			} else if *relayVia != "" {
				// Nobody to ask (a lone node bootstrapping into an empty
				// swarm): assume the conservative answer and take the relay.
				fmt.Println("reachability: no peers to check with — assuming NATed; leaning on the -relay-via relay")
				leanOnRelay(viaID, viaAddr)
			}
			// Start challenging peers' storage bonds (and refreshing our own
			// standing): consensus writes are gated on earned, held storage.
			// This comes BEFORE chain sync on purpose — a restarted node re-earns
			// its view of peer reputation here, and SyncChain needs that view to
			// judge which fork carries real standing (F1).
			if *validator {
				nd.StartBondAudit()
				// Fetch the other validators' token-issuer keys so we can verify
				// the publish tokens they blind-signed (chain token check).
				for _, aid := range attesterIDs {
					nd.FetchIssuerKey(aid, func(error) {})
				}
				// Catch up on any blocks committed while we were down — and keep
				// retrying, because peer standing (above) is re-earned live and
				// isn't ready the instant we boot. NOT gated on -attesters: a node
				// restarted with only -bootstrap still discovers validators via
				// gossip and rejoins (F1). Persist each catch-up.
				nd.StartChainSync(attesterIDs, func(added int) {
					fmt.Printf("chain: caught up %d block(s) from peers\n", added)
					saveChain("catch-up")
				})
				// -forge-block / -lowbond-propose: RED-TEAM / TEST HARNESS — send ONE crafted
				// proposal to a peer and report whether the honest validator refused it, proving
				// the two ValidateProposal defences (#184 forged-block→reject, low-bond→reject)
				// over the real wire. Retry only until the target is reachable. NEVER honest.
				badPropose := func(targetStr string, forge bool, label string) {
					if targetStr == "" {
						return
					}
					tid, terr := ports.ParseHash(strings.TrimSpace(targetStr))
					if terr != nil {
						fmt.Fprintf(os.Stderr, "-%s: %v\n", label, terr)
						return
					}
					fmt.Printf("⚠ ADVERSARY: -%s set — sending a bad proposal to prove honest rejection (red-team harness, never honest)\n", label)
					var try func()
					try = func() {
						nd.ProposeBadBlock(tid, forge, func(refused bool, err error) {
							if err != nil {
								clk.AfterFunc(1*ports.Second, try) // target not reachable yet; retry
								return
							}
							if refused {
								fmt.Printf("adversary: %s proposal correctly REJECTED by %s\n", label, tid)
							} else {
								fmt.Printf("adversary: %s proposal UNEXPECTEDLY ACCEPTED by %s (DEFECT)\n", label, tid)
							}
						})
					}
					clk.AfterFunc(2*ports.Second, try)
				}
				badPropose(*forgeBlock, true, "forge-block")
				badPropose(*lowbondPropose, false, "lowbond-propose")
				// -goodpropose: TEST HARNESS — the POSITIVE CONTROL for the two rejections.
				// A well-formed, properly-bonded proposal the honest target must ACCEPT, so a
				// reject-everything target can't false-pass the forged/low-bond REJECT tests
				// (#303). Unlike badPropose, RETRY ON REJECTION too: this proposer earns
				// standing over the first few bond audits, so early rejections are expected
				// until its bond qualifies — accept the first OK:true; give up after ~40 tries.
				if *goodPropose != "" {
					if gid, gerr := ports.ParseHash(strings.TrimSpace(*goodPropose)); gerr != nil {
						fmt.Fprintf(os.Stderr, "-goodpropose: %v\n", gerr)
					} else {
						fmt.Println("goodpropose set — sending a well-formed, bonded proposal to prove the honest target ACCEPTS it (positive control)")
						var tries int
						var gtry func()
						gtry = func() {
							tries++
							nd.ProposeGoodBlock(gid, func(accepted bool, err error) {
								if accepted {
									fmt.Printf("goodpropose proposal ACCEPTED by %s\n", gid)
									return
								}
								if tries >= 40 {
									fmt.Printf("goodpropose proposal UNEXPECTEDLY REJECTED by %s\n", gid)
									return
								}
								clk.AfterFunc(2*ports.Second, gtry) // not reachable / standing not yet earned — retry
							})
						}
						clk.AfterFunc(2*ports.Second, gtry)
					}
				}
				// -equivocate: RED-TEAM / TEST HARNESS — drive a deliberate double-sign so
				// honest replicas catch and slash it over the real wire (#184). Retry on the
				// loop-safe clock until this node has earned standing with both peers (early
				// attempts are refused). NEVER honest; a correct node refuses to equivocate.
				if *equivocate != "" && nd.Chain() != nil && nd.Chain().Objective() {
					// OBJECTIVE mode (3-of-4): a fork can NEVER be committed onto a
					// target (quorum-short; a minority fork is an I1 violation), so the
					// legacy commit-based placement cannot drive here. The faithful route
					// is slash-on-DETECTION (PE ruling 2026-08-17): this validator
					// participates honestly (its prepare lands on-chain), then SERVES a
					// conflicting signed block at that height; an honest peer fetches it on
					// sync and slashes the double-sign unaided. Peer IDs are irrelevant
					// (the fork is served on GetChain to whoever syncs). Retry until this
					// node has a committed prepare to fork.
					fmt.Println("⚠ ADVERSARY: -equivocate set (objective mode) — this validator will DELIBERATELY double-sign via a served conflicting fork (red-team harness, never honest)")
					var tryPlace func()
					tryPlace = func() {
						h, err := nd.PlaceConflictingSigned()
						if err != nil {
							clk.AfterFunc(1*ports.Second, tryPlace) // no committed prepare yet; retry
							return
						}
						fmt.Printf("adversary: equivocation complete (double-signed height %d)\n", h)
					}
					clk.AfterFunc(2*ports.Second, tryPlace)
				} else if *equivocate != "" {
					parts := strings.Split(*equivocate, ",")
					idX, e1 := ports.ParseHash(strings.TrimSpace(parts[0]))
					var idYZ ports.NodeID
					var e2 error
					if len(parts) == 2 {
						idYZ, e2 = ports.ParseHash(strings.TrimSpace(parts[1]))
					} else {
						e2 = fmt.Errorf("need exactly two peer IDs \"idX,idYZ\"")
					}
					if e1 != nil || e2 != nil {
						fmt.Fprintln(os.Stderr, "-equivocate: bad peer ids:", e1, e2)
					} else {
						fmt.Println("⚠ ADVERSARY: -equivocate set — this validator will DELIBERATELY double-sign (red-team harness, never honest)")
						var tryEquiv func()
						tryEquiv = func() {
							nd.Equivocate(idX, idYZ, func(err error) {
								if err != nil {
									// Narrate every refused attempt: a silent retry loop that can
									// wedge (e.g. a PARTIAL placement — X committed on idX, then the
									// fork synced back so later attempts build on a head idYZ refuses)
									// is undiagnosable from the outside (#345 family).
									fmt.Println("adversary: equivocation attempt refused:", err)
									clk.AfterFunc(1*ports.Second, tryEquiv) // not qualified with peers yet; retry
									return
								}
								fmt.Printf("adversary: equivocation complete (double-signed height %d)\n", nd.EquivocateHeight())
							})
						}
						clk.AfterFunc(2*ports.Second, tryEquiv)
					}
				}
				// -revoke: propose an on-chain takedown of the named root once it is
				// committed and we have earned standing to gather a quorum (F5). Poll
				// on the loop-safe clock; existence + quorum are enforced by the chain.
				if *revokeRoot != "" {
					root, rerr := ports.ParseHash(strings.TrimSpace(*revokeRoot))
					if rerr != nil {
						fmt.Fprintln(os.Stderr, "-revoke:", rerr)
					} else {
						// Tell the operator what -revoke is doing: it does NOT act
						// immediately — it polls until the root is committed on-chain and
						// this validator has standing to gather a quorum. Without this a
						// bogus/uncommitted root leaves the daemon silently inert (#235).
						fmt.Printf("revoke: target %s — waiting until it is committed on-chain and this validator has standing to gather a takedown quorum\n", root)
						sawCommitted := false
						var tryRevoke func()
						tryRevoke = func() {
							if _, ok := nd.Chain().LookupRoot(root); !ok {
								clk.AfterFunc(2*ports.Second, tryRevoke) // root not committed yet
								return
							}
							if !sawCommitted {
								sawCommitted = true
								fmt.Printf("revoke: %s is committed — gathering a takedown quorum\n", root)
							}
							nd.ProposeRevocation([]ports.Hash{root}, attesterIDs, attesterIDs, *quorum, func(err error) {
								if err != nil {
									clk.AfterFunc(2*ports.Second, tryRevoke) // no quorum yet; retry
									return
								}
								fmt.Printf("takedown: proposed on-chain revocation of %s\n", root)
								saveChain("takedown")
							})
						}
						clk.AfterFunc(2*ports.Second, tryRevoke)
					}
				}
			}
			nd.AnnounceHeld(func(count int) {
				if count > 0 {
					fmt.Printf("re-announced %d held chunks\n", count)
				}
				if reg != nil && *care != "" {
					for _, r := range strings.Split(*care, ",") {
						ch, err := link.ParseAnyCare(strings.TrimSpace(r))
						if err != nil {
							fmt.Fprintln(os.Stderr, "bad -care link:", err)
							continue
						}
						nd.Care(reg, ch)
						fmt.Printf("caretaking %s\n", ch.Root)
						if *auditInterval > 0 {
							ch := ch // capture per-root
							var sweep func()
							sweep = func() {
								nd.Audit(reg, ch, func(rep node.AuditReport) {
									fmt.Printf("audit %s: %d challenged, %d passed, %d FAILED (slashed liars), %d no-truth\n",
										ch.Root, rep.Challenges, rep.Passed, rep.Failed, rep.NoTruth)
								})
								clk.AfterFunc(ports.Duration(*auditInterval), sweep)
							}
							clk.AfterFunc(ports.Duration(*auditInterval), sweep)
							fmt.Printf("auditing %s every %s (verify-without-fetch PoR)\n", ch.Root, *auditInterval)
						}
					}
				}
			})
			// Provider records lease out after ProviderRecordTTL; a holder that never
			// re-announces goes invisible the moment its startup records lapse (#69).
			// Reprovide on a timer set well inside the TTL — a full re-announce, safe over
			// a large held set now that the DHT walk terminal is trampolined.
			nd.StartReprovide()
		})
	})

	fmt.Println("serving; Ctrl-C to stop")
	loop.Run() // forever
	return nil
}

// resolveLogLevel turns the -log/-debug flags into (level, on): -log
// wins when set, -debug is shorthand for the debug firehose. A VALIDATOR
// with neither flag defaults to info — the M0 trust plane (standing accrual,
// commits, catch-up, caretaker sweeps) is exactly what an operator must be
// able to see, so it narrates the normal path by default rather than staying
// silent until someone knows to ask (acceptance F7). A non-validator with
// resolveContentServing settles the D-TIERING content-serving axis from its two
// spellings: the positive -serve-content (the composable tier form) and the older
// negative -freeload. It reports whether the node REFUSES to store or serve
// content.
//
// The two must agree. An operator who passes -freeload together with an EXPLICIT
// -serve-content=true stated two contradictory intents, and silently picking one
// would give them a node doing the opposite of half their command line — so this
// refuses loudly instead (S3: a config contradiction fails visibly, never
// silently resolved). An unset -serve-content simply defaults true, so plain
// -freeload keeps working exactly as before.
func resolveContentServing(serveContent, serveContentSet, freeload bool) (refuses bool, err error) {
	if freeload && serveContentSet && serveContent {
		return false, fmt.Errorf("-freeload and -serve-content=true contradict each other: -freeload IS the refusal to serve content. Pass one (-serve-content=false has the same effect as -freeload)")
	}
	return freeload || !serveContent, nil
}

// neither flag keeps logging off (LogError is a harmless placeholder the
// caller ignores when on is false).
func resolveLogLevel(name string, debug, validatorDefault bool) (ports.LogLevel, bool, error) {
	if name != "" {
		lvl, err := ports.ParseLevel(name)
		if err != nil {
			return 0, false, err
		}
		return lvl, true, nil
	}
	if debug {
		return ports.LogDebug, true, nil
	}
	if validatorDefault {
		return ports.LogInfo, true, nil
	}
	return ports.LogError, false, nil
}

// openLog wires the file sink: everything the node and transport narrate
// at or above level lands in a grep-able debug.log next to the store, so
// a failure in the field leaves an artifact. The caller closes the sink.
func openLog(storeDir string, level ports.LogLevel, tr *tcpnet.Transport, nd *node.Node) (*logfile.Sink, error) {
	logPath := filepath.Join(storeDir, "debug.log")
	lg, err := logfile.Open(logPath, level)
	if err != nil {
		return nil, err
	}
	tr.SetLogger(lg)
	nd.SetLogger(lg)
	fmt.Printf("log: %s and above → %s\n", level, logPath)
	return lg, nil
}

// dialableRelayAddr turns the relay listener's bound address into one
// worth gossiping. A concrete bind speaks for itself; a wildcard bind
// ("0.0.0.0:4002") borrows the host the daemon already advertises for
// swarm traffic (-advertise), keeping the relay's own port. Neither →
// "" (nothing gossiped).
func dialableRelayAddr(bound, advertise string) string {
	host, port, err := net.SplitHostPort(bound)
	if err != nil {
		return ""
	}
	if ip := net.ParseIP(host); ip == nil || !ip.IsUnspecified() {
		return bound
	}
	if advertise == "" {
		return ""
	}
	advHost, _, err := net.SplitHostPort(advertise)
	if err != nil {
		return ""
	}
	return net.JoinHostPort(advHost, port)
}

// openRegistry accepts either "http://host:port" (plain, trusted
// loopback) or "ID@https://host:port" (TLS pinned to the hosting
// daemon's identity).
func openRegistry(ref string) (ports.Registry, error) {
	if at := strings.Index(ref, "@"); at == 64 {
		hostID, err := ports.ParseHash(ref[:at])
		if err != nil {
			return nil, fmt.Errorf("registry ref %q: %w", ref, err)
		}
		return httpregistry.NewPinnedClient(ref[at+1:], hostID), nil
	}
	// A bare https:// URL can't be verified without the host's identity to
	// pin. This is the common first-run mistake, so name the fix.
	if strings.HasPrefix(ref, "https://") {
		return nil, fmt.Errorf("registry %q is HTTPS but has no pinned identity; a key-pinned registry needs the ID@https://host:port form the daemon prints on start — copy its 'registry:' line verbatim", ref)
	}
	return httpregistry.NewClient(ref), nil
}

// parseSize reads human sizes: 2G, 500M, 64K, or plain bytes.
func parseSize(s string) (int64, error) {
	s = strings.TrimSpace(strings.ToUpper(s))
	mult := int64(1)
	switch {
	case strings.HasSuffix(s, "T"):
		mult, s = 1<<40, strings.TrimSuffix(s, "T")
	case strings.HasSuffix(s, "G"):
		mult, s = 1<<30, strings.TrimSuffix(s, "G")
	case strings.HasSuffix(s, "M"):
		mult, s = 1<<20, strings.TrimSuffix(s, "M")
	case strings.HasSuffix(s, "K"):
		mult, s = 1<<10, strings.TrimSuffix(s, "K")
	}
	var n int64
	if _, err := fmt.Sscanf(s, "%d", &n); err != nil || n <= 0 {
		return 0, fmt.Errorf("bad size %q (want e.g. 2G, 500M)", s)
	}
	return n * mult, nil
}

// ephemeral spins up a short-lived in-memory node joined to a swarm —
// the client side of daemon mode. Its identity is a throwaway keypair;
// clients don't accumulate reputation. Returned run posts fn onto the
// node's loop and waits for completion.
type ephemeral struct {
	nd   *node.Node
	loop *eventloop.Loop
	tr   *tcpnet.Transport
}

func joinSwarm(peers string, replication int) (*ephemeral, func(fn func(done func())) error, error) {
	ident, err := identity.Generate(rand.Reader)
	if err != nil {
		return nil, nil, err
	}
	loop := eventloop.New()
	go loop.Run()
	tr, err := tcpnet.New(loop, ident, "127.0.0.1:0")
	if err != nil {
		return nil, nil, err
	}
	// Same DefaultConfig base as the daemon (#71). This is the actual swarm
	// add/get fetcher, so it inherits the #65 retry; it stages and leaves,
	// so the repair/demand fields are harmless (it never caretakes).
	cfg := node.DefaultConfig()
	cfg.RequestTimeout = ports.Duration(2 * time.Second)
	cfg.RequireSignedProviders = true // reject forged/unsigned provider records on fetch (H5)
	cfg.ProviderRecordTTL = ports.Duration(30 * time.Minute)
	cfg.DHTDomainCap = 2 // resolve providers from a domain-spread set — eclipse resistance (H5-B)
	if replication > 0 {
		cfg.Replication = replication // a publisher may pick a lower redundancy (parity backstops copies)
	}
	nd := node.New(ident.NodeID(), cfg, walltime.New(loop), tr, memstore.New())
	nd.SetSigner(ident.Signer()) // sign self-certifying provider records (H5)
	nd.SetEphemeral(true)        // a publish/fetch client that keeps nothing — peers must not route to it (#43)
	if os.Getenv("SILT_SWARM_DEBUG") != "" {
		// Per-attempt narration to stderr (#497): a swarm add/get client is
		// otherwise silent about placement attempts, so a delivered-but-unacked
		// store (a silent extra copy on the receiver) is invisible from the
		// client side. Opt-in via env — the CLI's normal output stays one line.
		nd.SetLogger(logfile.New(os.Stderr, ports.LogDebug))
	}

	ps, err := discovery.ParseList(peers)
	if err != nil {
		return nil, nil, err
	}
	var seeds []ports.NodeID
	for _, p := range ps {
		tr.AddPeer(p.ID, p.Addr)
		seeds = append(seeds, p.ID)
	}
	run := func(fn func(done func())) error {
		ch := make(chan struct{})
		loop.Post("api", func() { fn(func() { close(ch) }) })
		select {
		case <-ch:
			return nil
		case <-time.After(5 * time.Minute):
			return fmt.Errorf("swarm operation timed out")
		}
	}
	e := &ephemeral{nd: nd, loop: loop, tr: tr}
	if err := run(func(done func()) { nd.Bootstrap(seeds, func() { done() }) }); err != nil {
		return nil, nil, err
	}
	return e, run, nil
}

func (e *ephemeral) close() {
	e.tr.Close()
	e.loop.Stop()
}

// AntiReleaseComputeWindow is the COMPUTE budget the anti-release floor is sized
// against: the time a released prover would need to re-seal its plot just-in-time
// to answer a challenge. Build-immutable #3/#4 (docs/TENETS.md): this is a
// *compute* window, deliberately DECOUPLED from the transport -request-timeout
// and the -bond-answer-latency reply gate. It is NOT the network reply deadline —
// so raising -request-timeout for durability (adverse-network hardening, #288)
// does not widen it, and the anti-release floor never balloons with a network
// timeout (that would price out small validators — immutable #4). Enforcement of
// the window is a floor large enough that a re-seal is a multi-second sequential
// cost, caught by the bond-audit statistics over history — not a single hard
// wall-clock reply deadline (immutable #3; #289).
const AntiReleaseComputeWindow = 2 * time.Second

// DerivedBondFloor is the anti-release floor an untrusted (objective) validator
// gets when the operator sets none — the smallest bond too big to re-seal inside
// AntiReleaseComputeWindow, with a 2× margin, so releasing and recomputing
// just-in-time is not a viable strategy. Derived (not a magic number) from the
// measured seal rate bond.PlotSealThroughput: 2s × ~270 MB/s ≈ 540 MiB × 2 ≈ 1 GiB.
const DerivedBondFloor = int64(2) * (int64(AntiReleaseComputeWindow/time.Second) * bond.PlotSealThroughput)

// coldStartScaffoldOK reports whether an untrusted objective validator has the
// cold-start scaffolding it needs to be safe from genesis (red-team seam-2 /
// Invariant B S6). Either satisfies it: the anchor LAUNCH set (anchors +
// mature-validators>0), which imposes the anchor co-sign until the network
// measurably decentralizes; or a weak-subjectivity CHECKPOINT, which is how a
// node safely JOINS an already-mature network without re-supplying the launch
// set. With neither, a stock validator latches everMature at genesis and runs
// with no cold-start gate — the seam-2 hole. Off the untrusted objective path
// (trusted/legacy) there is nothing to gate.
func coldStartScaffoldOK(useObjective bool, anchorCount, matureValidators int, wsCheckpoint string) bool {
	if !useObjective {
		return true
	}
	hasLaunchSet := anchorCount > 0 && matureValidators > 0
	hasCheckpoint := wsCheckpoint != ""
	return hasLaunchSet || hasCheckpoint
}

// effectiveBondFloor decides the anti-release floor (M0 retest G4-residual).
// Shipping the floor mechanism but defaulting it OFF left a doc-following open
// validator admitting sub-floor, releasable bonds — "fixed but off by default"
// is not fixed. So the floor is SAFE-BY-DEFAULT on the objective/open path, the
// same treatment -objective already has, while an operator can still opt out
// EXPLICITLY (-min-bond-floor 0) for a trusted/demo swarm. defaulted reports
// whether the value was derived rather than operator-set.
func effectiveBondFloor(floorSet bool, explicit int64, objectivePath bool) (floor int64, defaulted bool) {
	if floorSet {
		return explicit, false // an explicit choice always wins, including 0 (opt out)
	}
	if objectivePath {
		return DerivedBondFloor, true
	}
	return explicit, false
}

// DerivedBondTTL is the objective re-challenge cadence an untrusted (objective)
// validator gets when the operator sets none: standing lapses this many committed
// blocks after a validator's latest on-chain bond registration unless it renews
// with a fresh space-time proof (M0 hardening H2 / red-team RT-2). Shipping the
// TTL mechanism but defaulting it OFF left release-and-coast live — a validator
// registers a bond once, releases the plot, and votes forever off one proof — so
// like -objective and -min-bond-floor it is now SAFE-BY-DEFAULT on the objective
// path. The value trades liveness margin against how long a released bond can
// coast: the paired non-proposer renewal path (node.SubmitBondRenewal) fires
// every chain-sync sweep, so an honest validator gets many inclusion chances per
// window and never lapses, while a coaster is pruned within this many blocks. A
// tuning knob (Evolving), not a fixed law; a real deployment can tighten it.
const DerivedBondTTL = uint64(32)

// effectiveBondTTL decides the objective re-challenge TTL, mirroring
// effectiveBondFloor: an explicit -bond-ttl always wins (including 0, the
// trusted/demo opt-out), otherwise the untrusted objective path gets
// DerivedBondTTL and every other posture stays at the explicit (0) value.
func effectiveBondTTL(ttlSet bool, explicit uint64, objectivePath bool) (ttl uint64, defaulted bool) {
	if ttlSet {
		return explicit, false
	}
	if objectivePath {
		return DerivedBondTTL, true
	}
	return explicit, false
}

// DerivedEpochBlocks is the mature-phase validator-set epoch an untrusted
// (objective) validator gets when the operator sets none (#357 research
// certification, Condition A). The cadence trades two bounded windows: a bond
// that joins/renews/TTL-lapses mid-epoch integrates only at the next rotation
// (so a lapsed bond's vote can outlive its TTL by at most one epoch), against
// how often the finality set moves at all. 8 keeps that slop ≤ ¼ of
// DerivedBondTTL (32) — comfortably inside the TTL's own renewal margin — while
// still freezing the set across the multi-block window a WAN commit gathers in.
// Rotation itself is free (a map snapshot at an already-final boundary block).
// A tuning knob (Evolving), not a fixed law — but CONSENSUS-CRITICAL: it must
// match across the swarm, which the shared default provides.
const DerivedEpochBlocks = uint64(8)

// effectiveEpochBlocks decides the mature-phase epoch cadence, mirroring the
// floor/TTL derivations: an explicit -epoch-blocks always wins (including 0, the
// trusted/demo opt-out), otherwise the untrusted objective path gets
// DerivedEpochBlocks and every other posture stays at the explicit (0) value.
func effectiveEpochBlocks(epochSet bool, explicit uint64, objectivePath bool) (blocks uint64, defaulted bool) {
	if epochSet {
		return explicit, false
	}
	if objectivePath {
		return DerivedEpochBlocks, true
	}
	return explicit, false
}

// effectiveByzantineQuorum decides Byzantine quorum sizing (H4), mirroring the
// floor/TTL derivations: an explicit -byzantine-quorum always wins (including
// =false, the trusted opt-out), otherwise the untrusted objective path turns it on
// and every other posture leaves it off. It only ever RAISES the quorum, so
// defaulting it on for an untrusted validator can never weaken an existing config.
func effectiveByzantineQuorum(byzSet, explicit, objectivePath bool) (on, defaulted bool) {
	if byzSet {
		return explicit, false
	}
	if objectivePath {
		return true, true
	}
	return false, false
}

// DerivedOperatorMargin is the C2 split-defense margin M an untrusted (objective)
// validator gets when the operator sets none. M discounts the bond-distinct Nakamoto
// coefficient to ⌊k̂/M⌋, so a single operator splitting real stake across ~M NodeIDs
// cannot fake the decentralization that sheds the launch anchors — a splitter must
// clear mature-validators×M distinct bonds. Shipping the mechanism but defaulting
// M=1 (no margin) was the Invariant-B footgun the blind red-team flagged. The value is
// a conservative heuristic (Evolving) — M stays fundamentally unverifiable on-chain
// (#182), so this is margin, not proof; a real deployment can raise it.
const DerivedOperatorMargin = 2

// effectiveOperatorMargin decides the C2 operator margin, mirroring the floor/TTL/
// Byzantine derivations: an explicit -operator-margin always wins (including 1, the
// trusted/single-operator opt-out), otherwise the untrusted objective path gets
// DerivedOperatorMargin and every other posture keeps the explicit value. It only ever
// raises the bar to shed the wheels, so defaulting it up for an untrusted validator can
// never weaken an existing config.
func effectiveOperatorMargin(marginSet bool, explicit int, objectivePath bool) (margin int, defaulted bool) {
	if marginSet {
		return explicit, false
	}
	if objectivePath && explicit < DerivedOperatorMargin {
		return DerivedOperatorMargin, true
	}
	return explicit, false
}
