# Governance

Silt is infrastructure owned by none and run by its participants. This
document states the stance that shapes the architecture.

Silt is **use-agnostic**: it takes zero position on what the network is
used for. It is neutral, content-blind storage — the code cannot know or
attach meaning to what it carries. Any application built *on top* of Silt
(named "Aslan" — a resolver that maps human meaning to opaque roots) is a
separate product in a separate codebase; Silt ships none of it. The boundary is
immutable #6 in [docs/TENETS.md](docs/TENETS.md): core resolves hashes, never names.

## The project publishes software. It does not run the network.

AE Laboratories, the maintainer, writes and publishes source code. There are
no binary releases. For every network but one, that is all it does. It:

- **runs no nodes** on any network but its own closed test network (below),
- **maintains no denylist** for anyone else's nodes and holds no takedown authority,
- ships **no built-in list, no phone-home, no override, no backdoor**,
- receives, at most, a DMCA-designated agent for the *project's own*
  touchpoints (the website and the release artifacts) — not for the
  network, which it does not operate.

**The one exception is a closed test network.** AE Laboratories runs one
small test network to host the [flixz.com](https://flixz.com) demo. It
stores only the maintainer's own files, and its nodes accept connections
only from the demo's server. No one else can publish to it or join it.
Abuse reports about it go to abuse@silthq.com and follow the rules in the
[operator guide](docs/OPERATING.md#reporting-abuse).

This is deliberate and load-bearing. A pure software publisher stands on
the strongest legal footing — publishing code is expression — and there
is no central service to seize, subpoena, or coerce. Whoever runs the
network runs the policy.

## Policy is distributed and earned

Everything that governs the network is decided by its participants
through the same mechanism, and it is all in the code, operated by no one
in particular:

- **Publishing** a file's identifier to the registry requires a quorum
  of validators whose reputation is *earned* (passed storage audits,
  bytes served) — not bought, not granted.
- **Takedown** of a published identifier is a *curator's* signed
  listing, paid for with the same token a publish costs and committed
  through the same quorum: an append-only record, naming its curator,
  that every replica applies and no single node can force. Anyone can be
  a curator; each operator chooses which curators to follow
  (`-honor-curators`), and a node following none honours no on-chain
  takedown. Removing content is exactly as governed as adding it, and
  just as auditable.
- **Operators additionally choose** which local denylists to honor —
  their jurisdiction's, a trusted third party's — the way every network
  operator chooses which blocklists to run. The project supplies none of
  these lists.

## Updates are operator-autonomous and security-gated

The software **never silently auto-updates**. An operator chooses if
and when to upgrade; the project pushes nothing to a running node. The one
concession is *security-gated*: a release that fixes a security-critical
flaw is labeled as such so operators can prioritize it — but the decision,
and the act, remain the operator's. There is no override, no phone-home,
no channel by which the project reaches a node it does not run.

## Contributing

Anyone may contribute. Nothing reaches `main` or a release without a
pull request, green CI, and review — the branch is protected to enforce
it. See [CONTRIBUTING.md](CONTRIBUTING.md) for the flow.
