# 2026-08-22 — #500: NetGet retention semantics (record-less pulls → explicit working set or announced provider)

**Status: deliberation (pace-before-code), decision at the bottom.**

## The question

`fetchFrom` writes bytes with no provider record (deliberate — #497 named it).
Two callers retain what they pull: `NetGet` keeps the whole object forever
(contrast both repair paths, which drop their working set), and the UI
`apiFetch` consumer==provider path retains on purpose but never announces — the
promise "the node becomes a real provider of what it consumed" is unwired.
Result (#500): disk and records disagree by design on any node that ever
fetched; `swarm holders` understates real redundancy; retained copies are
invisible redundancy AND phantom local reachability.

## Evidence

- The #497 attribution: 55 `chunk pulled` vs 30 `chunk stored` lines across one
  economy drive, none of the pulls discoverable
  (`integration/cloudtest/497-attribution-evidence-f58d599-17479/`).
- Code read (this session): `NetGet` (`core/node/file.go:605`) fetches manifest
  chunks + columns into the store and never cleans up; `apiFetch`
  (`cmd/silt/ui.go:664`) states the consumer==provider intent in its comment;
  no test anywhere exercises NetGet retention.

## The constraint the issue's option (a) missed

Announcing a retained pull under its placement key is NOT enough by itself: a
provider record invites challenges, and a pulled chunk arrives with **no
StorageProof / PoR tags** — a half-provider that can serve bytes but cannot
defend an audit is a liability, not redundancy (B3/B7: never host what a later
audit can't defend). But the NetGet caller holds the full link — `LayoutKey` —
so the retainer can mint the complete proof itself: the manifest tree gives
`Prove(leafIdx)` + column, `DerivePorKey(LayoutKey)` gives the tags. A
retaining consumer can be a FULL, audit-answerable provider, identical to a
`MsgStoreChunk` recipient.

And the mechanism already exists (build-process rule 6): `hostShardLocally`
(`core/node/node.go:985`, the repair self-hold primitive) verifies bytes +
proof, stores, registers the provider record under the placement key, and
persists the proof — mirroring the MsgStoreChunk success path exactly. Retain =
loop `hostShardLocally` over the pulled set + one `announceAll` to plant the
records on the near nodes. Retention then integrates with
`AnnounceHeld`/`StartReprovide` for free: retained copies re-announce under
their placement keys on restart and every reprovide cycle.

## Options

**O1 — announce-only retention (the issue's (a) as written).** Rejected: the
half-provider audit liability above.

**O2 — explicit working-set semantics (the issue's (b)), with full-provider
retention.** `NetGet` defaults to repair-path symmetry: track what THIS call
pulled (held-before check, the `repairStripe` discipline — never drop what the
node already hosted), drop it after assembly, success or failure.
`NetGetRetain` keeps the pulls as real hosting: minted proof + record via
`hostShardLocally`, then `announceAll`. `apiFetch` uses retain — the
consumer==provider promise becomes real and audit-defensible. Other callers
(`netcheck` self-test, `swarm get` ephemeral client) take the drop default —
retention was meaningless there anyway (ephemeral processes).

**O3 — retain-and-announce everywhere (no drop mode).** Rejected: a daemon-side
`NetGet` for a one-off read (any future caller) would permanently convert reads
into hosting obligations against the capacity pledge — surprising default, and
the repair paths already establish the drop-your-working-set convention.

## Trade-offs owned (O2)

- **Announce traffic per retained fetch:** one `IterativeFindNode` + K sends
  per distinct placement key (a column = one key). Same order as a publish of
  the same object; paid only on the retain path. Acceptable.
- **Proof-minting cost (#8):** `tree.Prove` O(log n) per shard off the
  already-built tree + PoR tags per retained chunk — the identical cost the
  publisher pays at `Distribute`. One-time per retained object, no new floor.
- **Capacity:** retained pulls were ALREADY counted against the pledge (bytes
  in store); this change makes them discoverable and auditable instead of
  silent. A full store fails `hostShardLocally`'s Put → the fetch still serves
  (bytes stream from the assembly), the node just doesn't become a provider of
  that shard — logged, not fatal.
- **Repair interplay:** repair's `heldBefore` snapshot keeps NetGet-retained
  copies safe from the paramedic cleanup; a NetGet-drop racing a mid-sweep
  reconstruction could at worst drop a survivor the sweep also pulled →
  that stripe retries next sweep (self-healing, transient). Accepted.
- **`includeLocal` (the #497(b) question):** unchanged and now honest — with
  drop-as-default the only record-less local copies are in-flight working sets
  (legitimate); retained copies carry records, so local reachability and
  fetcher-discoverability agree.

## Decision

**O2.** Tiers: (1) unit/sim — default drops pulled-only (pre-held survives),
retain registers proof+record and a third node discovers and fetches from the
retainer after the original holders die; (2) e2e — publish on one daemon,
`/api/fetch` on another, assert bytes + the fetching daemon appears as a
provider (`swarm holders`). CHANGELOG under Fixed (#500). `swarm get` and
`netcheck` keep the drop default.
