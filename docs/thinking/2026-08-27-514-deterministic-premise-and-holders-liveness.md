# #514 — deterministic repair-bounty premise + the holders byte-confirm liveness fix

Date: 2026-08-27
Build-immutables in force: #6 root-cause-before-patch, #7 evidence-or-nothing.
Extends: `2026-08-27-514-repair-bounty-flake-holders-view-vs-bytes.md` (PR #607).
Ruling this answers:
`/Users/andrewedmond/Claude/claude/silt-reviews/principle-engineer/RULING-PR607-514-holders-byte-confirm-2026-08-27.md`

## Where #607 landed and why it is not enough

#607 byte-confirms `swarm holders` so the read stops reporting phantom holders (a provider
record whose bytes are gone). That is the right DIRECTION and it fixes a real observability
bug on a live product command. But two gates rejected it:

- **Tester (ground truth):** #607 still flaked 1/20 (~5%) — iteration 18 reproduced the
  exact premise defeat. The filter reduced the race, it did not close it.
- **PE (blind):** #607 is a pure FILTER, so it can only close the PHANTOM direction (record,
  no bytes). It cannot close the OMISSION direction (bytes on a node whose provider record
  has not DHT-converged to the reader — a filter cannot ADD an omitted holder). And it ships
  a liveness/DoS defect: `confirmColumnHolders` probes EVERY shard of a column per stale
  provider, serial, 2s each, no corpse-gating — the #226/#277/#501 dial-storm class,
  ~200s on a 100-stripe object.

## Root cause — BOTH directions, one mechanism

The premise selects a kill set from a `swarm holders` view taken at instant T0, then the
caretaker judges byte-reality with `probeShard` at a LATER instant T1 (its next sweep).
`swarm holders` (the selector) and `probeShard` (the caretaker) each resolve DHT provider
records and byte-confirm them, but they read the DHT at DIFFERENT INSTANTS. A "doomed"
column survives the kill in either of two ways, and both are the same underlying fact — the
pre-kill view is a stale snapshot of DHT convergence:

1. **Phantom (record without bytes), T0 over-lists.** At T0 the record view lists node X for
   column C, but X holds no bytes; the real bytes are on Y (a #497 lost-ack copy or #517
   false-repair copy) that the record view never listed. The test kills X, Y survives,
   `probeShard` finds Y → `missing ≤ slack` → no repair. #607's filter closes THIS: X is
   dropped, so the column is either not all-killable or a different column is chosen.

2. **Omission (bytes without a converged record), T0 under-lists.** The bytes live on Y and
   Y self-registered a provider record on a successful store (`node.go:1502`), but at T0
   that record has not propagated to the ephemeral client running `swarm holders`. The
   selector lists only the killable holders it can see. The test kills them. By T1 the
   caretaker's `probeShard` — reading the DHT a sweep or two later — DOES resolve Y (record
   converged, or Y re-announced via `StartReprovide`), finds the bytes → `missing ≤ slack` →
   no repair. A filter on the selector cannot reach this: Y was never in the selector's
   resolved set to keep or drop. This is the residual 1/20 #607 could not close.

The unifying mechanism: **the kill decision is made against a snapshot of byte-reality at
T0, but the premise is only true if byte-reality is over-slack-lost at T1.** Any fix that
only cleans up the T0 selector view is racing DHT convergence and will flake at some rate.

## The fix — two parts

### Part 1 (the robust close): STABILIZE convergence, kill all byte-holders, confirm on the caretaker

**Two more failed approaches localized the true root cause — and it is deeper than either
view's byte-confirm. The evidence is the caretaker's own sweep trace
(`scratchpad/e2e_diag.log`):**

```
16:57:23  repair sweep complete  reachable=12  |  stripe0 missing=7  stripe1 missing=4   (BOTH over slack)
16:57:25  repair sweep complete  reachable=12
16:57:27  repair sweep complete  reachable=14  |  stripe1 missing=2  (within slack)
16:57:29  repair sweep complete  reachable=16  |  (fully healed)
```

The caretaker's FIRST post-kill sweep DID see the loss over slack (missing=7/4). But
`reachable` then climbed 12→14→16 with NO repair logged, and the loss healed to within slack.
The climb without a repair means the killed columns' bytes **re-appeared** — the object
carries publish-time **lost-ack extra copies (#497)** whose provider records had not yet
DHT-converged when the kill happened, and which surface a sweep or two later. `-replication 1`
does NOT mean one holder: a lost-ack during Distribute mints silent extra copies (`file.go`
comment: "a lost ack mints a silent extra copy"), so a column killed at its visible holder
still has bytes elsewhere.

Two consequences pin the flake:

- The premise "kill 3 columns → over slack" is defeated by **publish-time over-replication**
  the selector cannot see at kill time.
- Even a caretaker-view escalation that kills more columns races the same convergence: a
  second local run killed 8 of 12 storage nodes and STILL never drove a stripe persistently
  over slack, because each newly-killed column's hidden copy re-converged inside the settle
  window (which was also too short — the cold first sweep pays a ~21 s manifest heal).

**The root cause of the flake: the test kills BEFORE DHT convergence completes, so hidden
extra copies survive and re-converge, and whether two consecutive caretaker sweeps both see
over slack (the #517 confirmation gate) is a race with convergence — won ~80% of the time,
lost ~20%.**

The robust close has four parts, all harness/observability, zero product-economics:

1. **STABILIZE first.** Poll the byte-confirmed `swarm holders` view until it stops changing
   across consecutive reads (convergence complete). Now the view is the COMPLETE truth: every
   real byte-holder of every column is listed, including the lost-ack copies. Bounded wait,
   fail loud if it never stabilizes.
2. **SELECT within bounds.** Killing a storage NODE removes EVERY column it holds, and with
   12 nodes for 16 columns some nodes hold two columns — so the union of 3 target columns'
   holders can take out more than 3 columns. The loss must exceed `RepairSlack` (2 → repair
   fires) but stay ≤ n−k (6 → a stripe keeps ≥ k=10 shards, so reconstruction is possible and
   the bounty can pay). A first draft dropped this bound and a local run destroyed the object:
   `stripe0 missing=9 → repair below k, only 7 of 16 shards, data unrecoverable`, no repair,
   no pay. So enumerate all-killable candidate columns and choose the 3-combo whose union
   loses the FEWEST columns in `[3, n−k]`. Re-verify post-kill against a straggler cascade
   into a multi-column node.
3. **Kill ALL byte-holders of the target columns.** With a stable view, killing every listed
   holder of a column genuinely removes it — no hidden copy survives. Re-read once and kill
   any straggler (a copy that converged between the stabilize check and the kill); with a
   stable starting view this is at most one extra round.
4. **Confirm on the caretaker's OWN sweep, with an adequate window.** Wait for the caretaker
   to narrate a stripe over slack (pending-confirmation, or a successful `stripe repaired`)
   across a window that covers the cold manifest heal AND the #517 two-sweep gate. `repair
   below k` is NOT an accepted signal — it means the stripe dropped below k (unrecoverable),
   which the bounded selection prevents; accepting it would mask a destroyed object. Because
   the killed columns are now genuinely byte-gone (stable view, all holders killed), the loss
   does NOT heal, so two consecutive sweeps both see it and the gate fires — deterministically.

This is the PE's prescription ("re-derive from the caretaker's own sweep") plus the missing
STABILIZE step that defeats the hidden-copy race the two view-based attempts could not, plus
the bounded SELECT that keeps the loss inside `(slack, n−k]` so the object stays recoverable.

Two more residual causes surfaced under a 50× run and are closed:

- **Caretaker-vantage divergence (~2%).** Even after the SELECTOR's stable view shows the
  columns byte-gone, the CARETAKER (a different node, its own DHT vantage) can still resolve
  a target column's copy the selector could not — a lost-ack copy converged to the caretaker
  but not the selector's ephemeral client, so the caretaker sweep reported `reachable=23`
  (nothing lost). Fix: STEP 4 is a confirm-OR-re-kill loop — if the caretaker has not narrated
  an over-slack loss, re-read the selector view and kill any surfaced killable holder of the
  target columns (still within the n−k bound), then wait again.
- **Placement concentration (~2%).** With `-replication 1`, `colKey(root,col)` closest-
  selection occasionally clusters all 16 columns onto 2-3 nodes (a DHT convergence-timing
  artifact — the payload seed is fixed, so the root is fixed, but which nodes are closest-and-
  reachable at publish time varies with join ordering). Then killing any node loses <3 or
  >n−k columns and no over-slack-but-recoverable loss can be forced. The cloud grade records
  this as "economy UNTESTED, not failed" (`integration/cloudtest/scenarios.sh:2095`) because a
  fixed fleet cannot re-roll. The e2e CAN: publish in a retry loop under a FRESH root (fresh
  random seed → different `colKey` → different closest nodes → re-rolled placement), stabilize
  each attempt, and only start the caretakers once a publish admits a valid kill set. This is
  strictly better than a `t.Skip` — the test always runs — and it is the reason the flake is
  now genuinely closed rather than merely rare.

Timing: each iteration is ~35-67s locally (occasional slow-convergence outlier ~144s); the
180s pay deadline holds with margin.

Superseded attempt (kept for the record): re-derive from the CARETAKER's own sweep alone

**A first attempt keyed the re-kill loop off a re-run of `swarm holders` (the SELECTOR's
byte-confirmed view). A local run proved that insufficient — and the run is the evidence
that pins the true mechanism.** The selector view showed 3 target columns byte-lost over
slack, and the harness declared the premise established. Yet the caretaker's OWN sweep, a
few instants later, saw the loss HEAL: `stripe repair pending confirmation … stripe=0
missing=7` on the first post-kill sweep, then `reachable` climbed 12→14→16 as survivors'
provider records converged, and by the second sweep stripe 0 was fully reachable and stripe
1 sat at `missing=2 ≤ slack`. No repair. Captured:
`scratchpad/e2e_diag.log`, sweeps at 16:57:23 (missing=7/4) → 16:57:27 (missing=2).

Two facts the run establishes:

- **The object is MULTI-STRIPE at this size** (`shards=23`, stripes 0 and 1), so killing 3
  columns spreads the loss across stripes rather than concentrating > slack on one.
- **The killed columns carry hidden extra copies** (#497 lost-ack / #517 false-repair) that
  the SELECTOR could not see at its instant but that re-converge into the CARETAKER's view a
  sweep or two later — so the caretaker's STEADY-STATE loss is smaller than the selector's
  post-kill snapshot. The selector view is not just stale, it is systematically more
  pessimistic than the caretaker's converged view.

This is exactly the coupling the PE flagged (ruling, "The coupling the consult missed"): the
selector view and the caretaker view read the DHT at different instants with different
corpse-gating, so making BOTH byte-confirm does not make them agree. The right invariant is
not "the selector shows byte-lost" but **"the caretaker's own sweep shows a stripe
persistently over slack."** Re-derive the premise from the caretaker, and ESCALATE the kill
until the caretaker's ground-truth view confirms it:

1. Select target columns whose holders are all killable storage nodes (byte-confirmed
   `swarm holders`).
2. Kill those holders.
3. Poll the CARETAKERS' debug.log for the ground-truth signal: a stripe that reaches
   `stripe repaired` / `repair below k` (the premise is proven — repair armed), OR a stripe
   PERSISTENTLY over slack (`stripe repair pending confirmation` seen on two consecutive
   sweeps without healing).
4. If, after a settle window, no stripe is persistently over slack (the loss healed to
   within slack — hidden copies re-converged), ESCALATE: re-read `swarm holders`, kill any
   newly-visible killable holder of the target columns AND add one more all-killable target
   column, then go to 3. Each escalation removes strictly more real bytes.
5. Terminate when the caretaker confirms (repair armed / paid), or FAIL LOUD as a harness
   bug after a bounded number of escalations — never silently pass a defeated premise.

Why this closes BOTH directions AND the multi-stripe/hidden-copy reality: the premise is
read from the SAME view the caretaker judges on, so there is no cross-instant divergence to
race. Escalation drives the caretaker's own converged view over slack, defeating hidden
copies by killing them once they surface. The loop terminates because each escalation kills
≥1 more real byte-holder of a finite pool, and the target columns can only regain a holder
from a repair — which is itself the success signal the loop is waiting for.

This makes the outcome deterministic: the loop returns only when the caretaker's ground
truth confirms the premise (or proves it unestablishable and fails loud).

### Part 2 (keep #607's product fix, fix its liveness): byte-confirm without the dial-storm

The re-run in Part 1 is only trustworthy if `swarm holders` does not list phantoms — else
step 4 would re-kill nothing (a phantom is not a live daemon to kill) and loop to the
harness-bug failure even when the real premise holds. So keep #607's product byte-confirm.
But fix its two liveness defects (PE Q2/Q4, blocking):

- **Corpse-gate `confirmColumnHolders`** to match `probeShard` (`repair.go:479`,`498`),
  including the `anyLive` sole-candidate guard: skip a provider proven dead this walk so a
  cooled corpse is not re-dialed at full `HolderDialTimeout`, but never skip the only
  candidate.
- **Probe ONE representative shard per provider, not all shards.** A holder that holds the
  column holds one of its shards; probing every shard of the column per provider is the
  amplification (`100 stripes × 2s`). Probe the FIRST shard id of the column. Correctness:
  a column's shards for one stripe object is length 1, so no change there; for a multistripe
  object a genuine byte-holder of the column holds the shard at ITS stripe, which may not be
  shard 0 — so probing only shard 0 could false-drop a real holder that holds a later
  stripe's shard of the column. `probeShard` sidesteps this because it probes per-`shardRef`
  (one specific shard id), not per-column. The correct representative for the holders view is
  therefore to probe the provider for the shard id it is MOST likely to hold — but the
  holders read resolves by COLUMN key, and a provider record under the column key does not
  say which stripe's shard it backs. Resolution: probe the provider for each of the column's
  shard ids but STOP at the first found AND corpse-gate + cap the walk, and additionally
  measure/bound it. See "The multistripe correctness question" below for the chosen bound.

## The multistripe correctness question (the real scoping call)

PE said "probe one representative shard per provider, OR justify a measured bound." Probing
only shard 0 is WRONG for correctness: a provider holding stripe-3's shard of column C
answers `not found` for stripe-0's shard and would be false-dropped. The economy grade uses
ONE stripe (16 columns, 1 shard each), so shard 0 is the only shard and the point is moot
there — but `swarm holders` is a product command and must be correct on large objects too.

The honest fix keeps the "found any shard ⇒ confirmed" loop (correct on all stripe counts)
but bounds its COST the way `probeShard` bounds its:

- Corpse-gate each provider once per walk (skip a proven-dead provider entirely after its
  first ladder — `anyLive`-guarded). A dead provider costs at most ONE `HolderDialTimeout`
  per walk, not one per shard.
- For a LIVE provider that genuinely holds the column, the very first shard it holds returns
  found and stops the inner loop. A live provider that holds NONE of the column's shards is
  a phantom — it will be dropped, but only after walking the column's shards; that walk is
  the price of correctness for the omission-vs-phantom distinction. Bound it: the inner shard
  walk stops at the first found and is skipped entirely for corpse-gated providers, so the
  worst case is (live-but-phantom providers) × (shards in column) dials. A live-but-phantom
  provider under the column key is rare (a provider records under a column key only on a
  successful store of that column's shard — `node.go:1502`). The dominant cost — dead
  record-holders — is now O(providers), not O(providers × shards), matching `probeShard`.

This is the measured bound PE asked for: the amplification PE flagged was the DEAD-holder
case (all shards dialed at full timeout because nothing is found). Corpse-gating collapses
that to one dial per dead provider. The residual per-shard walk only survives for a LIVE
provider that answers `not found` on real shards — a genuinely-recoverable corner, not the
100-stripe dial-storm.

## Extend vs supersede #607

**Extend.** #607's product byte-confirm is load-bearing for Part 1's re-check (a re-run must
not list phantoms). Its direction is sound; only its scoping (dial-storm) and completeness
(omission direction) fail. This PR keeps `confirmColumnHolders`, corpse-gates and bounds it,
and adds the deterministic re-kill loop that closes the omission direction #607's filter
structurally cannot. Superseding would throw away the correct product fix and re-litigate a
settled product-vs-harness call (PE Q2: product fix is the right call).

## Gate check (Q3, re-verified independently)

`confirmColumnHolders` and the harness loop touch `core/node/file.go` (the DHT holders read)
and `e2e/economy_repair_test.go`. Only product caller of `ColumnHolders` is
`cmd/silt/swarm.go` (`swarm holders`). No I1–I5 consensus rule, no fork-choice/epoch/
slashing, no escrow/skim/bounty math, no M0/C1/C2 claim. The invariant the test proves (a
verified reconstruction PAYS, `paid > 0`) is untouched — only the PREMISE arming is made
deterministic. Not research-gated.

## Evidence plan (build-immutable #7)

- Node-tier RED for the byte-confirm (kept from #607): a column whose record points at X
  while bytes live on Y; byte-confirmed holders = {Y}, ablation (skip confirm) goes red.
- A node-tier RED for the liveness fix: a column with a DEAD record-holder plus one live
  holder; assert the dead holder is dialed at most once (corpse-gated), not once per shard.
- The deterministic premise: ≥50 green iterations of `TestRepairBountyPaysOnTheWire`
  (`-timeout 900s`), committed as an artifact. A deterministic re-check should make it
  50/50, not probabilistic — any premise-defeat failure in the 50 is a real defect, not a
  timing miss.

## Un-skip

Main will carry a `t.Skip("#514…")` from a parallel quarantine PR. This PR rebases onto main
and REMOVES that skip as part of landing; the ≥50-iteration green run is with the skip gone.
