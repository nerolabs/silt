# Network durability on the public internet

**Read this before you invent any timeout, retry, eviction, or large-payload
scheme.** silt is network-heavy software that runs on the open internet, where
**jitter, latency, packet loss, and reordering are the everyday case, not the
exception.** The engineering answers to "how do I stay live and correct on a bad
network" are *already settled* — by RFC 6298, Kademlia, the BBR/NTP minimum-filter
lineage, and the mature proof-of-storage cohort (Filecoin, Storj, Chia, …). silt
has lost **days** re-deriving them (#286, #288). This page is the settled answer,
so you don't have to.

It is the working companion to **build-immutable #5** ("build for the adverse
internet — durability is the default") and the *liveness* dual of **#3** ("never
fuse transport with security") and **#4** ("cheap honest participation is a
security constraint"), all in [`TENETS.md`](TENETS.md) Part IX. Source of record:
the research team's opinion at
`silt-reviews/research/research-outcome/network-durability-vs-spacetime-RESEARCH-OPINION.md`
(2026-08-10, all citations web-verified), extended by the genesis
address-convergence follow-up at
`silt-reviews/research/research-outcome/286-layer2-address-convergence-RESEARCH-RESPONSE.md`
(2026-08-12) — the source for §1's first-contact rule, §2's never-evict tier, and §8.

---

## The one idea

**A wall-clock number is a *transport* measurement (RTT + jitter + retransmit +
transfer time). It is not a security instrument, and it is not a correctness
signal.** Network delay is **one-sided** — it can only ever *add* latency, never
subtract — so:

- A **slow reply is indistinguishable from an honest node on a bad path.** You can
  never conclude "slow ⇒ cheating" or "slow ⇒ gone" through additive noise you
  don't control.
- Therefore the network layer's job is to **ride out** impairment (generous,
  adaptive deadlines + retry), and to **estimate the floor** of a noisy signal
  (minimum-filtering), never to trip a hard decision on a single sample.

Everything below is a corollary.

---

## 1. Transport deadlines: generous, adaptive, and payload-scaled

A per-attempt request deadline bounds **transport**, nothing else. Size it for the
**worst real path you expect to serve**, and adapt it:

- **Adaptive per-peer RTO** — Jacobson/Karels `RTO = SRTT + 4·RTTVAR` (RFC 6298),
  per-peer EWMA. Cheap, local, N-independent, jitter-robust. This is the correct
  general form; a fixed constant is the anti-pattern.
- **First contact has no RTT history** — a fresh dial/TLS-handshake to a never-seen
  peer can't be adaptive yet, so fall back to a **modest initial value + retry**,
  exactly RFC 6298's model (initial RTO ≈ 1 s before any sample, exponential backoff
  on retransmit) — **not** a large fixed constant. A big first-contact deadline is
  doubly wrong: still non-adaptive, *and* it pins a dialer to a genuinely-dead peer
  for that whole budget before retry, slowing convergence under the very loss you're
  surviving. **Seed the per-peer estimator from the first successful handshake**, then
  adapt. (This is the correct reading of the anti-pattern rule above: it forbids a
  large *steady-state* constant, not a modest *initial* one. The instinct to "just
  raise 2 s to 10 s" is the trap — #286 Layer 2.)
- **Nest deadlines: inner strictly < outer.** A dial/TLS-handshake deadline must sit
  **below** the request-timeout that wraps it — the same rule as `HolderDialTimeout <
  RequestTimeout` — so a dead dial is abandoned *within* the request window and the
  layer above retries, instead of the whole request expiring on a peer that was merely
  slow to handshake.
- **Scale the deadline with the outbound payload.** A one-time large message (in
  silt, a block carrying a validator's ~1.5 MB space-time bond registration) needs
  time to *transfer and be re-verified* before the peer can reply — a flat
  RTT-scale deadline cannot cover it. Add `len(payload) / throughput_floor` of
  headroom (bounded). **This is #286:** a flat 2–5 s deadline wedged quorum-2
  genesis across a 3-region network while the identical block committed in 5 s on a
  single-zone quorum-1 SMOKE. Fix: `Config.RequestSizeFloorBytesPerSec` in
  `core/node/node.go` (`requestTimeoutFor`).
- **Karn's algorithm:** do **not** feed *retried* samples into the RTT baseline —
  you can't tell which send a reply answered, so an ambiguous sample poisons the
  estimate. Learn only from unambiguous first-try/nonce-matched replies.
- **Decouple deadlines by job.** A speculative holder-fetch dial should fail *fast*
  (stored content is often gone under churn; a generous timeout there just deepens
  a dead-holder dial-storm, #277) — keep it on a **tighter, non-extended** deadline
  (`HolderDialTimeout`). A consensus/mesh RPC gets the generous, payload-scaled one.
  One size does not fit both.

## 2. Retry — don't evict a live peer on one miss

A single slow or dropped packet must **not** tear a good peer out of everyone's
routing table — that keeps the mesh sparse and starves consensus of the standing it
is trying to form. This is **literally the original Kademlia policy**
(least-recently-seen: evict only on a *failed liveness probe* of the bucket head;
"old peers are good peers"). libp2p and discv5 both had to re-fix this exact
footgun.

- Retry the same peer with decaying backoff over an unknown-duration impairment;
  give it up (evict + negative-cache) only after retries are **exhausted**.
- **A bond-audit / standing timeout must never evict** (#288): standing is a
  *consensus* signal, reachability is a *routing* signal — conflating them is what
  starved consensus under loss. (This is #3 in action.)
- Do negative-cache a *genuinely* dead holder for a cooldown so the fetch/repair
  loop doesn't re-eat a full timeout on the same corpse every sweep (#226/#277) —
  reachability failures (DHT lookups, fetches) evict; *slow-to-prove* (bond audit)
  does not.
- **The configured consensus set is a static, never-evicted tier.** A validator/anchor
  peer named in config (see §8) is re-dialed with backoff **forever** and is **never**
  dropped by churn/eviction — exactly Tendermint's `persistent_peers`. You never
  "forget" its address (it lives in config), so a transient WAN unreachability is a
  retry situation, not an eviction. Keep the "evict after N retries" policy for
  *discovered* peers only; place the consensus set **below** it. This alone stops a
  proposer from losing an attester's address mid-formation and stalling quorum (#286
  Layer 2, Q4).

## 3. Minimum-filter a noisy signal — never trust one sample

If you must extract a signal from latency (e.g. a soft storage-diligence deterrent),
estimate its **floor** over many measurements, exploiting the one-sidedness of
delay: `reply = transport_floor + compute_floor + (non-negative jitter)`. The
**low quantile / windowed-minimum** over a peer's history recovers the floor and
filters jitter out. This is exactly:

- **NTP clock filter** (Mills), **BBR `min_rtt`**, **LEDBAT base delay**, the
  **Moon–Skully–Towsley** minimum-delay estimator — all the same move.
- **Storj's reputation-over-many-audits** is the deployed storage-network instance:
  a partial-storage cheat must recompute on *every* audit → its floor is
  *consistently* elevated; an honest node on a bad path is *randomly* slow → its low
  quantile collapses to the true (fast) floor.

A single-sample latency gate is never sound; a distribution-over-many soft deterrent
can be (see §5 for its ceiling).

## 4. Security lives in the proof or the statistics — never in the clock (#3, #4)

No mature storage/PoST network (Filecoin, Storj, Chia, Arweave, Sia, Spacemesh)
uses a single-shot wall-clock reply-latency deadline as a **security** gate. They
put the cost in the **proof structure** with a *generous liveness* timeout, score
**statistically over many audits**, or use a **VDF/proof-of-time**. Where a latency
number appears it is either a generous propagation timeout (Filecoin's ~30 min
WindowPoSt absorbs global propagation; it does not time an RTT) or a *separate
uptime score walled off from correctness* (Storj).

Consequences for silt (all are #3/#4):

- A timing signal may ship as a **soft, disclosed deterrent**; a **hard** security
  gate must be **structural**, and an unbuilt structure is an **owned, named
  residual** (`design/owned-residuals.md`), not a wall-clock stopgap.
- **A VDF is the wrong tool for a "too-slow" detector**: it certifies a *minimum*
  elapsed time ("≥ T steps happened"); a storage-diligence gate needs a *maximum*
  ("this prover spent *extra* compute"). No cheap primitive produces "at most T
  elapsed," and a VDF measures against the *fastest* hardware, giving an ASIC more
  spare budget to hide recompute. (silt's per-challenge VDF still does its intended
  freshness/minimum-work job — it just can't be repurposed as a slowness detector.)
- **Anti-release is a *compute* window, not a transport timeout.** Size a min-bond
  against re-seal time × plot throughput, **never** against `-request-timeout`
  (#4). Raising the transport deadline for durability must **not** move the
  min-bond floor, or you price out the small operator silt exists to serve. This is
  already wired in `cmd/silt/daemon.go` (`AntiReleaseComputeWindow`, the
  `-min-bond-floor` help text).

## 5. The soft gate's ceiling — the frog-boiling caveat

A statistical latency deterrent is sound as a **soft** signal but has **no
soundness proof against an adaptive adversary that controls its own baseline**: a
cheat can inflate its transport floor (route badly on purpose) so recompute hides
inside a plausibly-far latency — and reply-latency alone cannot attribute the floor
to transport vs. compute. **Boundable, not closable**, with cheap/local measures:
anchor the threshold to a **network-wide cohort prior + the one-sided minimum**
(never the peer's own inflated history); **floor the baseline and rate-cap its
upward drift**; and never let it be the **sole** basis for a hard action (let it
trigger a harder challenge or a slow reputation penalty). This residual is *why* the
sound version is structural — the honest content of owned-residual **A5**.

## 6. Large payloads over a lossy path — keep them off the critical path

Once security is statistical, a slow/lossy large proof is a **durability** concern,
not a security trip. In preference order (all leave *what* is proven unchanged):

1. **Make proofs succinct** (dominant strategy). Filecoin ships ~192-byte Groth16
   proofs precisely so loss barely matters when a proof is a few packets. For silt
   this is **#299** (the structural close for #286): a succinct bond proof removes
   the 1.5 MB payload that forced the size-aware deadline in the first place. Absent
   a SNARK, trade `BondLabelSamples` k for size where the soundness margin allows.
2. **Chunk + FEC** — RaptorQ (RFC 6330), a systematic rateless code: reconstruct
   from ~any K symbols, send a few extra, and loss is absorbed without RTT-bound
   retransmit. Ideal for high-RTT lossy paths.
3. **QUIC/UDP** to kill TCP head-of-line blocking — a lost segment stalls only its
   own stream, not the whole proof. (Caveat: NAT/middlebox friction.)

## 7. Distance-bounding — the *one* place latency is a sound primitive (and why)

Worth knowing so you don't reach for it wrongly: wall-clock latency **is** sound for
**proof-of-location / distance-bounding**, and it works there for the **opposite**
reason a storage gate needs. Latency gives a *lower* bound on distance (an adversary
can *add* delay but can never reply faster than light, so latency can't be spoofed
*shorter*). A storage-diligence gate wants the *upper*-bound direction ("you were
slow, therefore you cheated"), which the same one-sidedness makes **unsound**.
Latency can prove *near*, never *diligent*.

## 8. Mesh formation: a peer you can't dial isn't "discovered" — configure the consensus set

Timeouts and retries keep a *formed* mesh alive; they do not *form* one. Address
convergence — every node learning every consensus peer's **dialable address** — is a
separate, prior problem, and it has a settled answer. It is **not** a timeout problem,
and no deadline tuning fixes it.

- **At genesis there is no chain, so no on-chain validator registry — the validator
  set's addresses must live in *config*, not discovery.** This is the chicken-and-egg
  every BFT/PoS network resolves the same way: you cannot bootstrap *proposer-initiated*
  quorum from an address book that only fills from inbound frames through a single hub.
  Configure the validator/anchor set as `ID@addr` in **every** validator — a static,
  persistently-dialed consensus tier. Precedent: **Tendermint/CometBFT
  `persistent_peers`**, **Ethereum static/trusted peers + `enode://…@ip:port` / ENR**,
  **libp2p bootstrap list + peerstore**. `seeds`/PEX are a *separate, weaker* role —
  never hang consensus off them.
- **Discovery converges addresses only if a lookup response carries the address.**
  Classic Kademlia's contact is `(NodeID, IP, port)`, so `FIND_NODE` returns *dialable*
  peers and even hub-and-spoke eventually converges. **If you split NodeID from address**
  (routing table holds bare IDs; addresses live only in the transport layer — silt does
  this), iterative lookup returns peers you *still can't dial*, and hub-and-spoke **never**
  converges addresses. Given that split, either (i) **restore the coupling** — carry the
  dialable address in lookup/`Contacts` responses — for the general mesh, or (ii)
  **configure** the consensus-critical set out-of-band. Do (ii) for genesis regardless.
- **Consensus must never *depend* on gossip/discovery for the addresses it needs to
  gather quorum.** Gossip / PEX / Kademlia-with-addresses is the right discipline for the
  *general* mesh; the *consensus* set is configured and never-evicted (§2). This is
  Tendermint's split exactly: PEX converges the general peer set; `persistent_peers`
  carries the set you must not lose.
- **This was #286 Layer 2.** A fresh 3-region quorum-2 chain never committed genesis: all
  spokes bootstrapped to one hub, the hub bootstrapped to nobody, so the hub could only
  learn spokes from *inbound* frames — and under the WAN inbound stampede those handshakes
  EOF'd — so the proposer had `send with no known address` for every attester and could
  not initiate the gather. It reproduced only over real WAN (the no-latency sim has no
  inbound overlap). The symptom *looked* like a timeout; the cause was configure-vs-discover.

## 9. One security property lives in the HOST, not the code — a demand issuer needs a cache-private box

**Operator rule: do not run a silt daemon with the demand lane on
(`-accept-delivery-receipts`, which makes the node a retrieval-token issuer) on a host
that shares a CPU cache with untrusted co-tenants.** Dedicated hardware, or a VM with
dedicated cores. That is the whole rule; the rest is why it cannot be fixed in code.

The issuer's blind signature is an RSA private-key operation over `math/big`. silt blinds
it (`SignBlinded`), which closes the **remote** timing channel — Go's Montgomery
exponentiation ends in a data-dependent conditional subtraction, and that extra-reduction
oracle is exactly the one Brumley–Boneh exploited across a network. Blinding does **not**
close the second channel: the exponentiation indexes a power table at an
**exponent-dependent** offset, and the private exponent is the same on every call, so a
local attacker sharing the L1/L2 cache can recover its bits (Percival 2005;
Yarom–Falkner FLUSH+RELOAD).

**No Go implementation can close it.** Go's own `crypto/rsa` moved private-key operations
onto a constant-time backend, but the standard library exports no raw RSA private
operation, so every blind-signature library in Go is back on `math/big` — Cloudflare's
CIRCL `blindrsa`, the reference Go implementation of RFC 9474, carries the identical
residual. It is therefore a **declared deployment assumption**, not a defect to be
patched: the same shape as §4's rule, one step further out. A property the code cannot
hold is stated where the operator can hold it instead.

Declared in code at `core/blindtoken/blindtoken.go` (`SignBlinded`) and for auditors in
`docs/thinking/2026-09-02-r0.4b-c3-close-design.md` §12. Source: crypto-specialist
advisory R6, 2026-09-03.

---

## silt's applied ledger (worked examples of this page)

| Issue | Symptom | This page's lens | Fix shipped |
|---|---|---|---|
| **#286 L1** | quorum-2 genesis wedged cross-region; the 1.5 MB registration block couldn't be attested in time | §1 flat deadline not payload-scaled | size-aware transport deadline (`RequestSizeFloorBytesPerSec`); async publish (202+poll); succinct-proof #299 is the structural close (§6) |
| **#286 L2** | genesis still never committed; proposer had no *address* for attesters (`send with no known address`); hub's inbound handshakes EOF'd; WAN-only | §8 configure-not-discover + §2 never-evict + §1 first-contact deadline | configure the validator set as `ID@addr` (static, never-evicted tier); first-contact deadline modest + nested `<` request-timeout; instrument the EOF before tuning it — *direction set by research 2026-08-12, build in progress* |
| **#288** | consensus starved under loss; live peers evicted on one miss; a flat deadline | §1 + §2 | adaptive/patient timeout + retry-don't-evict + bond-audit-never-evicts |
| **#289** | should reply-latency be a hard C1 storage-diligence gate? | §3 + §4 + §5 | **no** hard gate — soft statistical deterrent (A5); structure is H-track |
| **#277** | dead-holder dial-storm ate a full timeout per corpse every sweep | §1 decouple + §2 negative-cache | tighter `HolderDialTimeout`, no retry, `HolderCooldown` negative cache |

**Rule of thumb before you write network code:** *Is this deadline/threshold a
transport measurement being asked to also prove security or correctness?* If yes,
stop — split it (a generous, adaptive transport deadline **and** a separate
structural/statistical instrument). *Is it a single sample about to trip a hard
decision?* If yes, stop — minimum-filter over many, or retry instead of deciding.
*Does a symptom (a slow reply, an EOF, a stall) merely look like a timeout?* Before
you tune the number, **attribute it** — instrument the actual failure (which handshake
phase, how long, how many concurrent) and rule out an architecture cause (unconverged
addresses, a peer never learned, an eager eviction). "Looks like a timeout" is how
#286 Layer 2 masqueraded as a deadline when it was configure-vs-discover (§8).

---

## Sources (web-verified 2026-08-10, from the research opinion)

- **Adaptive timeout + minimum-filtering:** Jacobson & Karels, SIGCOMM 1988 —
  https://ee.lbl.gov/papers/congavoid.pdf ; RFC 6298 —
  https://www.rfc-editor.org/rfc/rfc6298.html ; Karn & Partridge, SIGCOMM 1987 —
  https://dl.acm.org/doi/10.1145/55482.55484 ; NTP clock filter (Mills, ToN 1995) —
  https://dl.acm.org/doi/pdf/10.1145/190314.190343 ; BBR —
  https://queue.acm.org/detail.cfm?id=3022184 ; LEDBAT RFC 6817 —
  https://www.rfc-editor.org/rfc/rfc6817
- **Kademlia churn discipline:** Maymounkov & Mazières, IPTPS 2002 —
  https://pdos.csail.mit.edu/~petar/papers/maymounkov-kademlia-lncs.pdf ; libp2p
  eviction footgun — https://github.com/libp2p/go-libp2p-kad-dht/issues/283
- **Configured persistent validator peers (address convergence, §8 / §2 never-evict):**
  Tendermint/CometBFT `persistent_peers` / seeds / PEX —
  https://docs.cometbft.com/v0.38/core/configuration ; Ethereum static/trusted peers
  + enode / ENR (EIP-778) — https://geth.ethereum.org/docs/fundamentals/peer-to-peer ,
  https://eips.ethereum.org/EIPS/eip-778 ; libp2p bootstrap + peerstore —
  https://docs.libp2p.io/concepts/fundamentals/peers/ (Kademlia contact = (ID, IP,
  port) is the Maymounkov–Mazières citation above)
- **PoST-network practice (nobody reply-latency-gates):** Filecoin PoSt spec —
  https://spec.filecoin.io/algorithms/pos/post/ ; Storj audit service —
  https://github.com/storj/storj/wiki/audit-service ; Chia consensus (PoT/timelords)
  — https://www.chia.net/wp-content/uploads/2022/09/Chia-New-Consensus-0.9.pdf
- **Distance-bounding (latency sound only one-way):** Kuhn/Luecken/Tippenhauer —
  https://www.cl.cam.ac.uk/~mgk25/sc2005-distance.pdf
- **VDF (min-not-max):** Boneh–Bonneau–Bünz–Fisch, CRYPTO 2018 —
  https://eprint.iacr.org/2018/601
- **Structural close:** Fisch, *Tight Proofs of Space and Replication*, EUROCRYPT
  2019 — https://eprint.iacr.org/2018/702
- **Large-payload-over-loss:** RaptorQ RFC 6330 —
  https://www.rfc-editor.org/rfc/rfc6330 ; QUIC HOL —
  https://github.com/rmarx/holblocking-blogpost
