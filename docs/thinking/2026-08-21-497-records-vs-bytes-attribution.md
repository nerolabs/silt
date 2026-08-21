# 2026-08-21 — #497 records-vs-bytes: code-read attribution + the instrumented repro decision

**Goal (the one scoped goal this session):** attribute #497 — a `-replication 1` publish
leaves ~3× copies on disk (~87 files for a 29-shard object, in the publish second) while
`swarm holders` shows one holder per column. Gates the ECONOMY=1 cloud run.

## What the code read established (evidence: cited lines)

The complete set of paths that write chunk bytes into a daemon's store is three:

1. **`MsgStoreChunk` handler** — `core/node/node.go:1305`. Stores, then self-records a
   provider under the placement key (`:1310`). Senders: `placeAt` (publish + repair
   reseed), `fanOut` (demand, `Lease=true`), repair push.
2. **`fetchFrom`** — `core/node/file.go:467`. Stores and records **nothing** — no provider
   record, no announce. Retained by: `NetGet` (whole object; **no cleanup exists** — contrast
   `core/node/repairclaim.go:205` and `core/node/repair.go:576`, which both drop what they
   fetched) and `Care`'s warm start (manifest chunks, announced). Repair/judge fetches drop.
3. **`hostShardLocally`** — `core/node/node.go:942`. Economy repair self-hold only; records.

Corollaries the read pinned down:

- The publish client honors `-replication 1`: `joinSwarm` plumbs the flag
  (`cmd/silt/daemon.go:1458`) and `placeAt` stops at `want` acceptances
  (`core/node/file.go:278`). `placed=30` printed by the client counts **acks**, not
  deliveries.
- **`placeAt` re-sends the same chunk to the next candidate on ANY request error**
  (`core/node/file.go:288-296`), and the group-level retry re-sends a whole group that
  acked nowhere (`file.go:166-170`, `placeAttempts=4`). A store that was **delivered and
  completed but whose ack was lost or late** therefore mints a silent real copy while
  `placed` stays 1. The ephemeral client's deadline is a hardcoded 2 s
  (`cmd/silt/daemon.go:1454`) + payload scaling (+1 s at 256 KiB,
  `core/node/node.go:1087`); the fleet daemons run `-request-timeout 8s`
  (`integration/cloudtest/topology.py:328`). The client/daemon divergence is itself a
  build-immutable #5 smell.
- The flow-evidence journals (info level) carry **zero per-chunk lines** — successful
  stores/fetches log nothing today; only failures warn. Attribution from existing logs is
  impossible; that is the instrumentation gap.
- `ColumnHolders` (`core/node/file.go:686`) fetches manifest chunks despite its
  "Read-only" comment — ephemeral-client store only, so it does not mint on the fleet,
  but the comment overstates.
- Flow order A means no `swarm get` runs before the kill, so **NetGet retention cannot
  explain the publish-second copies** in the census runs (577f0f1-27476, 1642465-55153).
  It remains a real records-vs-bytes minting path for any later fetch window and for the
  skim leg's driven fetches.

## The candidate mechanisms (bounded set)

- **M1 — multi-delivery on unacked store:** client times out / errors on an ack whose
  store completed; walks to the next candidate; extra copy per lost ack. Same-second ✓.
  Receivers DO self-record — so this only reproduces the records side if the extra
  receivers are not on the `swarm holders` walk's query path (ties to the recorded
  fresh-client placement divergence: candidates skew to long-known nodes, not
  argmin-XOR).
- **M2 — fetch-pull retention (NetGet / Care):** bytes with no records, the exact S5
  signature — but excluded for the publish second by flow order A; applies to fetch
  windows.
- **M3 — repair-fetch cleanup hole / M4 — hostShardLocally:** not same-second; repair
  had not fired.

The code read cannot select M1's trigger (which error path, which candidates, why
records stay at 1/column) — that requires observation (#7: gather, don't guess).

## Decision: one instrumented LOCAL repro

Options weighed:
1. Mine more past-run logs — dead end (no per-chunk lines exist at info).
2. Keep reading code — the mechanism space is already bounded; selection needs data.
3. **Instrument the three write sites with debug narration + rerun the publish on the
   LOCAL fleet at `LOG_LEVEL=debug` — chosen.** Minutes-local, and every on-disk file
   becomes attributable to its minting call site and sender.

Instrumentation (ships with the eventual fix PR; B5 — the debug path exists to narrate
per-event detail):
- `MsgStoreChunk` success → debug `chunk stored` (chunk, from, key, lease).
- `fetchFrom` success → debug `chunk pulled` (chunk, provider).
- `placeAt` per-attempt outcome → debug `place attempt` (chunk, target, ok/err).

Repro recipe (from the #497 memory): `LOCAL=1 ECONOMY=1 KEEP_UP=1 ./cloudtest.sh up`
with `LOG_LEVEL=debug`, one `swarm add -replication 1 -chunk-size 262144` from fetch-1,
then the per-node disk census (mtime bucketing), the holders map, and the debug-log
correlation. A dedup re-add is NOT a read-only probe — one publish only.

## RESULT — the mechanism, observed end to end (same day)

Two instrumented observations on a fresh LOCAL fleet (run f58d599-17479; evidence
preserved at `/Users/andrewedmond/Claude/claude/silt/integration/cloudtest/497-attribution-evidence-f58d599-17479/`
— caretaker debug logs, both censuses, the publish-client log, the drive console).

**Observation 1 — a cold-fleet publish is CLEAN.** One `-replication 1` publish placed
exactly 30 files for 30 client acks; every file carries one `chunk stored
from=<ephemeral-client>` line; the `swarm holders` map agrees with the disk (the four
val-a column keys on disk are the four val-a columns in the map). Records == bytes.
The publish-path suspects (ack-timeout resend, dedup re-place, receiver fan-out) did
not fire. `-replication 1` means what it says at publish time.

**Observation 2 — the economy drive reproduces the divergence, and the narration
names the writer.** `FLOWS=flow_economy_repair` on the same fleet (GAP reproduced:
paid=0). Timeline, all from the preserved logs (times UTC):

- `08:00:05` publish: 30 placements, records == bytes (map echoed by the flow).
- `08:00:11–16` caretakers (store-2, relay) arm; each pulls the manifest chunk
  (Care warm start — `chunk pulled`, retained, announced).
- `08:00:13–43` repair sweeps at the 2 s cadence, every one `29/29 reachable`, each
  sweep completing in ~2.1 s. Healthy and fast.
- `~08:00:50` the harness kills adversary + store-3 (holders of 3 columns, 7 shards).
  Their logs stop at 08:00:35/41 and resume 08:04:51/52 — dead the whole window.
- The sweep in flight at the kill takes **~3–4 minutes** (relay completes 08:03:34,
  store-2 08:04:08) and returns the CORRECT verdict `reachable=22/29` (29−7=22).
  Sweep DURATION under dead holders is minutes, probe/lookup-timeout dominated;
  `-repair-interval 2s` bounds only the idle gap between sweeps.
- `08:04:42–08:05:01` repair fetches survivors: **26–27 `chunk pulled` lines on EACH
  caretaker** — the whole object, via `fetchStripeByColumn` → `fetchFrom`, in stripe
  order. These are the #497 "extra copies": real bytes, NO provider records, invisible
  to `swarm holders`, counted by `probeShard(includeLocal=true)`.
- `08:04:50` the 240 s pay window expires → GAP recorded → the harness immediately
  restarts the killed holders (step 6c, 08:04:51/52).
- `08:04:59/08:05:01` reconstruction completes and finds `missing=0` — the
  just-restarted holders answered the re-check — so nothing is pushed, no claim is
  emitted, no bounty paid. The fetched survivors are then dropped (verified GONE from
  disk). **The repair loop was correct and ~10 s late; the harness's own
  restart-at-window-end healed the object out from under it.**

**The #497 answers:**

(a) *What mints the extra copies and why don't they announce:* `fetchFrom`
(`core/node/file.go:467`) writes bytes and deliberately mints no provider record. Its
minting callers: the repair sweep's survivor fetches (up to k×stripes chunks per
caretaker, held for the minutes-long sweep+reconstruct window, dropped after —
transient), Care's manifest warm start (retained; announced separately), and NetGet
(retained forever, never announced — including the UI `apiFetch` consumer==provider
path, whose stated promise "the node becomes a real provider of what it consumed" is
unmet: it never announces; code-read, not exercised in this drive). The ~87-file /
~3× census of the prior session is consistent with placed(30) + two caretakers'
in-flight survivor fetches (~26 each) + manifest pulls — observed mid-reconstruction.
Its "in the publish second" mtime reading was made without writer attribution and is
not reproduced by the instrumented run (pulls land minutes after publish); the
attributed mechanism is what stands.

(b) *Is a caretaker-held-but-unannounced copy "reachable":* the question is now
precisely scoped — it concerns the transient repair working set and retained NetGet
copies. During a reconstruction window `includeLocal=true` legitimately reads its own
in-flight pulls; a fetcher cannot discover them. Durability semantics call, still open.

(c) *New, sharper finding — the GAP's real cause is a timing budget, not economy
logic:* sweep-under-failure duration (~3–4 min: probe/lookup timeouts toward dead
nodes — the #277 dial-storm shape one layer up) ≈ the harness's whole 240 s pay
window, and step 6c restarts the dead holders the moment the window expires, so a
late-but-correct repair concludes `missing=0` and never claims. Also latent: a
daemon RESTART mid-reconstruction (relaunch_with / econ_restore) would orphan the
fetched survivors on disk — `dropHosted` never runs — turning the transient copies
persistent on re-driven fleets (plausible source of the prior census's persistent
extras; not directly observed).

**Follow-ups this decomposes into:**
1. Harness (gates ECONOMY=1 cloud): the pay window must cover ≥ one full
   sweep-under-failure + fetch/rebuild/push/judge (measured ~4–5 min here), and the
   step-6c restart must wait for a completed post-kill repair cycle, not the window's
   expiry.
2. Product (S5): fetch-retained copies (NetGet, UI apiFetch) never announce — the
   consumer==provider promise is unwired; and the sweep's probe phase is unbounded
   under dead holders (build-immutable #5: bound it, negative-cache walk targets).
3. Product (hygiene): restart during reconstruction orphans the fetched survivor
   working set (no drop on next boot).
