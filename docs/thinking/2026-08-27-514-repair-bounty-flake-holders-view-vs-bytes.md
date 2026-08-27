# #514 — TestRepairBountyPaysOnTheWire flake: holders record-view ≠ byte-holders

Date: 2026-08-27
Build-immutables in force: #6 root-cause-before-patch, #7 evidence-or-nothing.

## The failure (Tester-confirmed, third-time rule, scar filed)

`e2e/TestRepairBountyPaysOnTheWire` fails ~20% of runs at
`e2e/economy_repair_test.go:330`:

> premise defeated (#514): no caretaker observed an over-slack loss within 60s of the
> kill — the killed columns still have live copies somewhere (holders-view vs bytes
> divergence).

The test kills the holders of 3 coded columns to force an over-slack loss (`missing >
RepairSlack`, slack = 2) so the repair bounty must arm. In the flaking runs the kill does
NOT eliminate any column's bytes: a live copy survives on a node the selector never
listed, so the caretaker's byte-confirmed sweep reports `missing ≤ slack` and correctly
refuses to repair. No repair → no claim → no pay → premise fast-fail.

## Root cause (attributed, with evidence)

The selector and the repair judgment look at two DIFFERENT views of "who holds a column."

- The selector reads `swarm holders` → `Node.ColumnHolders` →
  `columnHoldersEntry` (`core/node/file.go:835`), which resolves each column's holders
  with `resolveProviders(colKey(root, col), …)` (`file.go:873`). That returns the DHT
  **provider RECORDS** — nodes that once claimed to hold the column. It does NOT confirm
  the bytes are still there.
- The repair judgment reads `probeShard` (`core/node/repair.go:452`), which resolves the
  same provider records and then **confirms each with a `MsgHasChunk` round-trip** before
  counting the holder present. Its own comment states the principle: *"a bare provider
  record isn't trusted, so a stale record can't fool the dispersion audit."*

The two views diverge exactly when a byte copy exists on a node the record-view did not
list, or a record exists without bytes. Two documented mechanisms produce the extra byte
copy in this topology:

- **#497 lost-ack extra copy.** `placeAt` (`file.go:274`) walks candidates and, on an
  errored/refused store, moves to the NEXT candidate — but the store may have completed on
  the first. The comment at `file.go:292` names it: *"a lost ack mints a silent extra
  copy."* The receiver holds the bytes; the sender's record may point elsewhere.
- **#517 false repair re-replication.** A prior repair sweep can re-place a column onto a
  fresh holder that the current `swarm holders` snapshot has not converged on.

Pre-#501 (PR #513), the dead-holder dial-storm choked the probe walks, so marginal extra
copies went unconfirmed — `missing` looked bigger and repair fired anyway, masking the
class. The #501 fix made the probes accurate, so the byte-view now correctly sees the
survivors, and the record-view selector's under-kill is exposed as the flake.

Mechanism, one paragraph: the premise fails because the kill-selector kills provider
RECORD holders while the caretaker arms repair on byte-CONFIRMED holders; the two views
diverge when a #497/#517 extra byte copy of a "doomed" column lives on a node the
record-view omitted; the selector then kills nodes that do not eliminate the column's
bytes, the byte-confirmed sweep sees `missing ≤ slack`, and repair never arms. The fix is
to give the selector the SAME byte-confirmed view the repair uses, so the kill provably
eliminates the columns' bytes.

## Why the raw `t.Fatalf` premise-check is not the fix

The existing premise fast-fail (lines 284–333, added for #514) only *names* the defeat
after the window burns. It converts a silent hang into a loud failure — good for
attribution, useless for green. The test still fails ~20% of the time. #514 was closed
prematurely; the arming defect was never removed. This change removes it.

## Options

### (a) Byte-confirm the holders view — extend `swarm holders` to `MsgHasChunk`-confirm

Add byte-confirmation to the column-holders resolution: after resolving each column's
provider records, keep only the ones that answer `MsgHasChunk` = found. Reuse the exact
mechanism `probeShard` already uses. The selector then kills byte-holders, so the kill
provably eliminates ≥3 columns' bytes, and the selector's view matches the repair's view —
they cannot diverge.

- Cost: touches an observable command (`swarm holders`). Adds one round-trip per provider
  on the holders path. Blast radius: any reader of `ColumnHolders`.
- Benefit: fixes a REAL observability bug, not just the test. `ColumnHolders` is
  documented as *"the read an operator uses to see WHERE an object lives"* — reporting
  records that no longer back bytes is wrong for the operator AND for the cloud economy
  flow, which uses this same selector. One source of truth for "who holds a column."

### (b) Harness-local premise re-check + re-select (task option b)

After the record-view kill, probe the SURVIVING storage daemons' on-disk stores for each
doomed column's bytes; if a survivor holds them, add it to the kill set (or re-select).
Pure test code, zero product change.

- Cost: the harness must map column → chunk IDs (needs the manifest, which the harness
  does not currently parse) and know the on-disk store layout
  (`<store>/objects/<hex[:2]>/<hex>`). Couples the test to two internal formats. Leaves the
  observable command still lying to operators.
- Benefit: no product blast radius.

## Decision: (a), byte-confirm the holders view

The divergence IS the bug. The selector and the repair judgment must agree on "who holds a
column," and the honest answer is the byte-confirmed one — `probeShard` already decided
that a bare record isn't trusted. Fixing the harness alone (b) would leave `swarm holders`
reporting phantom holders to operators and to the cloud economy selector, which is the same
latent defect one topology-shuffle away from biting again. Reusing `MsgHasChunk` keeps the
new surface tiny. The extra round-trips are bounded (one per provider record, on a
read-only diagnostic command) and match what the repair sweep already pays.

Not a consensus/economic/published-claim change: this is an observability read and a test
harness. No I1–I5 rule, no escrow/skim/bounty math, no M0/C1/C2 claim moves. The invariant
the test proves (a verified reconstruction PAYS) is untouched — only the PREMISE arming is
fixed. No research gate.

## The regression test

Byte-confirmation is not shipped until its defect is injected and watched go red (the
session-7 ablation rule). A unit/integration RED that forces the record-view-vs-bytes
divergence deterministically: a column whose provider RECORD points at node X while the
BYTES live on node Y. Record-view `ColumnHolders` lists X; byte-confirmed lists Y. Killing
X (record-view) leaves the bytes; killing Y (byte-view) eliminates them. Assert the
byte-confirmed holders = {Y}, not {X}. This is the divergence the e2e flake rides, made
deterministic at the tier where the selector logic lives.

Then re-run the e2e ≥20 iterations to show the ~20% flake is gone.
