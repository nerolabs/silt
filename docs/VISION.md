# silt — the finished system (the north star)

> **Status: north star.** This is a picture of silt when it is *done* — what it is, what it
> feels like to use and to run, and the one bet it is making. It is the **destination** the
> [tenets](TENETS.md) define and the [roadmap](../ROADMAP.md) walks toward. It is deliberately
> **not** a status report: where today's build differs from this picture, the canon is the
> honest record — *what M0 asserts* lives in [`design/m0.md`](design/m0.md), *what is decided*
> in [`decisions.md`](decisions.md), *where we are on the path* in [`../ROADMAP.md`](../ROADMAP.md).
> This document exists so the vision survives the day-to-day, and so every increment can be
> checked against where it is going.

---

## In one paragraph

silt is a content-addressed storage and distribution network that holds the
**privacy × accountability × Sybil trilemma** without trading any corner away. Anyone can
publish, unlinkably, from an identity that costs nothing to create. Standing in the
network — the right to influence consensus, curate, or be trusted — costs sustained, real,
challenged work, and is cryptographically unlinkable from what its owner publishes.
Genuinely harmful content can be removed by a hash, pluralistically and provably without a
global switch. Hosts store ciphertext they cannot read and carry no liability for what they
cannot see. And the whole thing runs on a hobbyist's ~1 vCPU / 2 GB box, on the open
internet, surviving the jitter and loss of real networks as the default rather than the
exception. That combination — cheap to join, ruinous to farm, private by construction,
accountable at the content layer, and small enough for one person to run — is the finished
system.

---

## What it is, told through the people it serves

**The publisher** puts content into silt and gets back a durable link. They never reveal a
durable identity to do it: publishing spends a blind-signed token, so the network can
confirm the publish was paid for without learning *who* published. The link resolves for as
long as the content is retained, and the publisher can prove — to themselves and to anyone —
that their content was stored bit-perfect, or see an explicit failure. Publishing is a right,
not a privilege granted by standing.

**The fetcher** retrieves content and silt does not surveil them for it. There is no
mechanism, anywhere in the design, that logs or links who-fetched-what. Retrieval rides
unlinkable tokens over content-blind relays and private lookup, pushing access-privacy as
far as proven cryptography allows at the metadata layer. silt does not promise the
impossible — a global passive adversary watching all wires defeats any low-latency network —
but it promises never to *build the surveillance itself*, and to hold access-unobservability
to the anonymity trilemma's real limit.

**The operator** runs a node on a small box and it stays small. Memory is bounded under
adversarial input first, then fast — *bounded-then-fast*, never OOM. A validator does not
hold the whole registry to do its job: it validates by **proof**, checking each block's
state transition against witnesses supplied by the tier above, so the honest-validator
floor stays a floor as the network grows to all-content-ever. The operator earns standing by
providing real, sustained, address-diverse storage and serving real demand — and earns
balance-lane credit for the bandwidth they relay and the repairs they perform. Cheap honest
participation is a security property, not a courtesy: no defense is allowed to price the
small operator out.

**The curator** removes genuinely harmful content by acting on a **hash**, never on an
identity and never through a global switch. Every honored takedown is committed to an
append-only transparency log with inclusion and consistency proofs, so silt can *prove* it
never flipped a global switch — that some content survived, on some independent hosts.
Takedown is pluralistic: no single party, and no single machine, can make content vanish
everywhere. Curators are themselves accountable, and the non-globality of every removal is a
measured, provable quantity, not a promise.

**The adversary** — the party the whole design is written against — cannot do three things,
and an outside red-team, not the author, is the one who confirms it: they cannot link a
publish to a durable identity (privacy), they cannot achieve identity-level or global
takedown (accountability), and they cannot **farm Sybil standing at a discount**
(Sybil-resistance). That last one is the load-bearing, field-defining claim, and it is a
claim about the *composition*, not any single primitive.

---

## The one bet, stated without a slogan

Two edges of the trilemma dissolve by architecture silt already has. Privacy-vs-accountability
dissolves because silt acts on **content, not identity**: deny a hash; hosts are
content-blind and liable for nothing they cannot read. The live edge — where the novel
contribution concentrates — is **privacy vs. Sybil**, and silt's bet is to **decouple the
cost of *creating* an identity from the cost of *having standing***. Identity is free and
pseudonymous. Influence costs sustained, challenged, real work. And the publishing act stays
cryptographically unlinkable from the bonded identity that did the work.

> **Token-less, work-backed, identity-bound reputation that publishing stays
> cryptographically unlinkable from** — cheap for one honest node, ruinous for a Sybil farm,
> with no coin and no capital lockup.

No single mechanism can prevent Sybils under free identity minting with no permanent center —
that is a settled impossibility. The guarantee lives in the **system**. Each part denies one
economy of scale a Sybil relies on: a size-bound bond denies one plot backing many
identities; unique-sealed real content denies synthetic bytes standing in for storage;
witnessed, unlinkable demand receipts deny self-dealt demand; address- and AS-diversity
buckets deny massing free keys near a target; retention decay denies coasting on stale
standing. Composed so that every shortcut on one axis trips another axis's check, the target
makes **forging N standings cost N× of every non-substitutable resource** — which is exactly
what honest provision costs. Sybil-resistance is therefore *re-pricing plus
concentration-bounding*, not prevention; the residual — an honest whale who genuinely
provides that much — is *bounded* by the concentration metric, not eliminated.

The finished system holds all three corners because the corners **co-mature**. Privacy is
architectural from day one. Accountability is content-level and reactive from day one. And
Sybil-resistance is the corner that bootstraps: weakest on a young network, strengthening as
real, sustained work accrues. During the launch window, explicit, time-boxed anchor
validators are the training wheels — and they **shed** on measured decentralization through a
one-way latch that, once tripped, never re-arms, so a later dip in decentralization can never
hand the launch anchors permanent power. The bet, stated plainly: **maturity is reached
before the scaffolding can be captured.**

---

## The architecture that delivers it

**Consensus is boring, by policy.** The novelty budget is spent entirely on the Sybil
composition; the consensus layer is literature-faithful BFT, hardened, not reinvented.
Finality rests on intersecting quorums by real bonded weight, never head-count. A validator
never signs twice at a height, and that memory survives restart. The validator set changes
only at finalized boundaries. Commit and final are distinct, so a young network can make
optimistic progress at a low quorum without a non-intersecting quorum ever finalizing a fork.
Fork-choice is a deterministic total order, and every safety violation is attributable — an
honest node is never slashed. These five invariants are a closed, published set (see
[`design/consensus-invariants.md`](design/consensus-invariants.md)), asserted under
adversarial scheduling on a laptop before any expensive run, because the perimeter of BFT
correctness is finite and known. silt walks it deliberately, having walked several of its
doorways the hard way.

**Storage is tiered, and the tiers keep the floor a floor.** An **archival** node retains
all history to genesis and can serve the deep past a pruning swarm has dropped. A **pruning**
node keeps a rolling retention horizon — enough to validate and serve current content without
paying for all of history. An **edge** node serves content and relays bandwidth without
carrying validation weight. The registry's validity-relevant state is committed each block
under a **state root**: a history-independent sparse Merkle tree over the set-valued state,
plus a separate append-only root for the transparency log — two kinds of committed data,
each under a root whose structure matches it. Because the root is committed, a validator on
the floor box does not need the tree: it checks each transition against witnesses, and the
tree lives a tier above. A fresh node cold-syncs from a recent weak-subjectivity checkpoint —
silt is weakly subjective, like every proof-of-stake-class system, and honest about it — and
an organically-crashed node re-pins itself from its own last finalized checkpoint without a
human in the loop.

**The economy prices value and never mints a subsidy.** Storage is bonded and priced.
Bandwidth — the thing every prior system left unpriced — earns balance-lane credit that can
never become standing. A delivery receipt mints no credit; a colluding pair only rebuys its
own fee minus a skim, a strict loss, because the banned dual is any network-minted per-receipt
subsidy — a money pump. Relaying is paid by a self-enforcing incremental micropayment: the
fetcher authorizes each increment, the relay forwards only while paid, and neither can prove
the other cheated, so there is nothing to adjudicate and no trusted third party in the path —
the irreducible one-increment exposure is bounded small by construction. Repair is
incentivized by bounties that pay for real work and are never clawed back once earned.
Content curation and demand attestation reuse the same unlinkable blind-token primitive that
keeps publishing private, so the economy never becomes a deanonymization surface.

**Durability is the default, not a hardening pass.** Every network path assumes the adverse
internet — jitter, loss, reordering — as the everyday case. Transport deadlines are generous,
adaptive, and payload-scaled; a live peer is retried, not evicted, on a single slow packet;
noisy signals are minimum-filtered to their floor rather than trusted on one sample. Security
never rests on a wall-clock number an adversary's own path can move: latency proves proximity,
never diligence. A single measurement never serves two masters. These are not silt's
inventions; they are the settled answers of RFC 6298, Kademlia, the BBR/NTP lineage, and the
mature proof-of-storage cohort, imported deliberately (see
[`network-durability.md`](network-durability.md)).

---

## What "done" is checkable against

silt is finished when an external red-team — a party other than the author — runs the
adversarial suite and **denies all three failure modes**: no publish→identity linkage, no
identity-level or global takedown, and no Sybil-farmed standing at a discount, where the
Sybil mode is the systemic no-discount-plus-no-quiet-capture claim and not a per-primitive
test. "Did we hold the trilemma?" then has a yes/no answer, on the board, that an outsider
can check — not a victory the builder declares. Everything in this document is in service of
making that outside answer *yes*, on a box a hobbyist can afford, on the internet as it
actually is.

That is the north star. Every phase, every certification, every oracle, every measured number
is a step toward the finished system described here — and the way to know a step is real is
that it moves an outsider's checkable answer, not the builder's confidence.
