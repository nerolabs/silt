# 2026-08-22 — #502: boot reconciliation of the orphaned repair working set

**Status: deliberation (pace-before-code), decision at the bottom.**

## The hazard (code-read certainty; latent, not field-observed)

A repairing caretaker pulls up to k×stripes survivor chunks (`fetchStripeByColumn`
→ `fetchFrom`) and drops them only in the post-reconstruction cleanup
continuation (`repairStripe`'s `cleanup`; the judge's verify fetches in
`repairclaim.go` share the pattern). A restart in that window — operator, crash,
or the harness's `relaunch_with`/`econ_restore` — kills the continuation chain,
and nothing at boot reconciles: the pulls sit in the store permanently,
record-less, counting against the pledge and read as local by
`probeShard(includeLocal=true)`. The plausible source of the persistent ~3×
census on re-driven fleets that motivated #497.

## The identification rule (what makes this safe post-#500)

An orphan is: **a LEAF (data/parity shard) of a cared root, present in the
store, with no proof in the persisted proof backing.** Every legitimate leaf
holding carries a persisted proof — `MsgStoreChunk` receivers refuse
proof-less coded shards, the repair self-hold (`hostShardLocally`) mints one,
and since #500 `NetGetRetain` does too, while plain `NetGet` drops its working
set. Manifest chunks are exempt: caretakers hold them bare BY DESIGN (the Care
warm start — "caretakers ARE the manifest's redundancy in v1"). Before #500
this rule would have mislabeled retained NetGet pulls; after #500 those either
carry proofs (retained) or don't exist (dropped) — #502 was sequenced after
#500 for exactly this reason. A bonus: legacy proof-less NetGet leftovers from
pre-#500 fleets are cleaned by the same sweep at their next boot.

## Design points resolved

- **Boot-time, not sweep-time.** At Care-registration (boot) nothing is in
  flight, so the rule has no false positives. A sweep-start reconcile could
  drop a concurrent NetGet's in-flight working set on a caretaker that also
  serves fetches. (Residual: a UI fetch racing the boot reconcile itself — a
  seconds-wide window, transient, the fetch retries. Accepted.)
- **Consult the persisted proof backing (`n.proofs`), never `proofMeta`.**
  `proofMeta` is rebuilt lazily at boot (`reloadProofBatch`) — racing it would
  mislabel legit holdings. The backing is the durable truth.
- **Hook: Care's warm-start continuation.** The manifest chunks are just
  fetched there (the layout is loadable), and the daemon calls `Care` per
  cared root on every boot — no new daemon wiring.
- **`AnnounceHeld` runs before `Care`** in the daemon, so an orphan gets
  announced once and then dropped — a dangling record on the near nodes.
  Self-heals: the record ages out at `ProviderRecordTTL`, and a fetcher that
  dials gets a clean not-found. Not worth reordering the boot.
- **Narrated:** `repair working set reconciled` (root, dropped count) at info,
  so a `relaunch_with` fleet's journal shows the cleanup happening.

## Decision

Implement `reconcileWorkingSet` in Care's warm-start continuation per the rule
above. Regression at the sim tier with a REAL crash injection: drive an actual
repair sweep (killed holders → survivor fetches) with `simclock.Step()` and
stop mid-window — between fetch and drop — then boot a fresh Node incarnation
on the same chunk+proof stores (the on-disk state IS the crash artifact),
`Care` the root, and assert: orphans dropped, warm-start manifest copies kept,
a proof-backed legit column holding kept. Two sim worlds (the crashed one is
abandoned wholesale) because a shared scheduler would let the "dead" node's
pending callbacks fire and finish its cleanup — a crash fires nothing.
