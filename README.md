# silt

A content-addressed, erasure-coded storage and distribution network.

> **Early and experimental — 0.x, unaudited.** silt is published to get technical
> feedback, not to be trusted with data you cannot afford to lose.

## What silt is

silt is **two planes on one substrate**: a *storage plane* (content-addressed,
erasure-coded, peer-served chunks with NAT traversal) and a *trust plane*
(consensus-secured registry, reputation, and revocation). The storage plane stands
alone and is the default; the trust plane is opt-in and secures governance.

Neither plane is the product. silt is the **substrate** other things are built on.

Three properties shape everything else:

- **The link is the primitive.** A `silt:` link is the whole product surface: a
  content-addressed identity plus the key to read it. Durability, placement, and
  repair are the network's job, not the holder's.
- **Capabilities, not infrastructure.** Store, relay, registry, validate, caretake
  are capabilities *any node can offer*, never special nodes baked into the binary.
  No node is permanently load-bearing.
- **The naming boundary.** silt core resolves *hashes*, never *names*. Turning an
  opaque root into human meaning — names, descriptions, curation — is a separate
  resolver layer, and core carries zero meaning. A link guarantees the bytes it
  names; trusting a *name* means trusting whatever resolver you asked.

silt is use-agnostic. Core carries zero meaning and takes zero position on use.

## What silt promises

silt exists to **hold the privacy × accountability × Sybil trilemma** — to refuse to
trade any corner away. Every prior system in this space picks two corners and
surrenders the third.

- **Privacy.** Publishing is unlinkable to a durable identity. silt refuses to
  surveil who fetches what, pursuing access-privacy to the metadata-layer limit the
  anonymity trilemma allows — not an absolute blob-layer guarantee. A serving node
  necessarily sees what it serves and to whom, for as long as serving requires;
  silt records nothing beyond that, and nothing of it leaves the node.
- **Accountability.** Genuinely harmful content can be removed by acting on a
  **hash**, never on an identity and never through a global switch. Takedown is
  pluralistic, curators are themselves accountable, and every honored removal is
  committed to an append-only transparency log with inclusion and consistency
  proofs.
- **Sybil-resistance.** Standing cannot be cheaply forged. Identity is free and
  pseudonymous; *influence* costs sustained, challenged, real work — and the
  publishing act stays cryptographically unlinkable from the bonded identity that
  did the work.

This is **hold**, not **resolve**. It is not a claim to have solved a research
problem all at once. It is a refusal to trade a corner away, and a design in which
the corners co-mature: privacy is architectural from day one, accountability is
content-level and reactive from day one, and Sybil-resistance is the corner that
bootstraps — weakest on a young network, strengthening as real, sustained work
accrues.

### The bet, stated plainly

Decouple the cost of *creating* an identity from the cost of *having standing*.

No single mechanism can prevent Sybils under free identity minting with no permanent
center; that is a settled impossibility. The guarantee lives in the composition.
Each part denies one economy of scale a Sybil relies on — a size-bound bond, unique
sealed content, witnessed unlinkable demand receipts, address and AS diversity,
retention decay — so that every shortcut on one axis trips another axis's check. The
target is that forging N standings costs N× of every non-substitutable resource,
which is exactly what honest provision costs.

**This multiplicative interlock is the target, not yet the operative guarantee.**
Today consensus standing is gated by the bond axis alone. The other axes are
designed and staged, not fully wired.

### How you would know

silt is finished when an external red-team — a party other than the author — runs the
adversarial suite and denies all three failure modes: no publish-to-identity linkage,
no identity-level or global takedown, and no Sybil-farmed standing at a discount.
That answer belongs to an outsider, not to the builder.

## Design posture

- **Consensus is boring, by policy.** The novelty budget is spent entirely on the
  Sybil composition. The consensus layer is literature-faithful BFT, hardened, not
  reinvented. Admission is
  objective, bond-weighted commit admission — a block
  commits only on an intersecting super-quorum of a validator set the
  chain itself sizes (a strict anchor majority at launch, >⅔ of the
  epoch's frozen on-chain bond once standing is earned), so a sub-quorum
  partition commits nothing, stalls, and catches up to the majority's
  history on heal rather than reorging onto it.
  A validator never signs twice at a height, and that memory survives
  restart. The validator set changes only at finalized boundaries. Commit and final
  are distinct. Fork-choice is a deterministic total order, and every safety
  violation is attributable — an honest node is never slashed.
- **Storage is tiered.** *Archival* nodes retain all history to genesis. *Pruning*
  nodes keep a rolling retention horizon. *Edge* nodes serve content and relay
  bandwidth without carrying validation weight. The registry's validity-relevant
  state is committed each block under a state root, so a validator on a small box
  validates by checking transitions against witnesses instead of holding the tree.
- **The economy prices value and never mints a subsidy.** Bandwidth earns
  balance-lane credit that can never become standing. A delivery receipt mints no
  credit. Repair bounties are funded only from an object's own escrow, never from a
  network mint.
- **Durability is the default, not a hardening pass.** Every network path assumes
  the adverse internet — jitter, loss, reordering — as the everyday case. Security
  never rests on a wall-clock number an adversary's own path can move.

It runs on a hobbyist's ~1 vCPU / 2 GB box. Cheap honest participation is treated as
a security property, not a courtesy.

## Build

Requires Go 1.26.5 or newer. No cgo, no native toolkit.

```sh
go build ./cmd/silt          # single binary at ./silt
go test ./...                # the suite
```

Cross-compile release binaries for macOS, Windows, and Linux, with checksums:

```sh
./build.sh v0.1.0            # writes dist/ plus dist/SHA256SUMS
```

Each artifact is one self-contained binary. The web UI is embedded with `go:embed`;
there is nothing else to ship.

## Run

Store a file and get a link back. `add` prints the root (the public name) plus the
key (the private capability), and a care link that grants repair rights without
decryption. Files are erasure-coded — with the default k=10, n=16, any 10 shards of
each stripe reconstruct it.

```sh
silt add <file> [-store DIR] [-mode convergent|private] [-k K] [-n N]
silt get <link> -o <out> [-store DIR]
silt info <link-or-care-link> [-store DIR]
silt ls [-store DIR]
```

Run a node:

```sh
# desktop app: serves and consumes, opens a browser UI
silt client [-store DIR] [-capacity 5G] [-bootstrap ID@ADDR,...] [-ui ADDR]

# headless swarm node
silt daemon [-listen ADDR] [-store DIR] [-capacity 2G] [-bootstrap ID@ADDR,...]

# as a validator
silt daemon -validator -bond 8M -quorum Q -attesters ID[,...]
```

Work against a real swarm, and inspect:

```sh
silt swarm add <file>     -peers ID@ADDR[,...] -registry REF
silt swarm get <link>     -o <out> -peers ID@ADDR[,...] -registry REF
silt swarm holders <link> -peers ID@ADDR[,...] -registry REF

silt id [-store DIR] [-listen ADDR]     # a node's ID, without launching it
silt chain-status [-store DIR]          # head height and hash, read-only
silt genesis [-text]                    # the founding block a fresh network carries
```

Simulations run the whole network in-process, no sockets:

```sh
silt sim run scatter | churn | economy | audit | capacity | consensus | bondstanding | takedown
silt net demo [-nodes N] [-size B]      # the same, over real TCP
```

Run `silt help` for the full flag surface.

## Canon

Two documents govern this repository:

- [`docs/TENETS.md`](docs/TENETS.md) — the mission, what silt is, how it is built and
  tested, the bright lines, and the three tiers that say which decisions are
  immutable.
- [`docs/VISION.md`](docs/VISION.md) — the finished system, told through the people
  it serves.

Where the build differs from the vision, the code and its tests are the honest
record.
