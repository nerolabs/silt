# The era-4 freeze: what closes, and what stays open

**Audience: the owner, before signing the freeze act (ROADMAP row D3).** This is not the
manifest. The manifest (22 items, four classes, three deadlines) is the build list, and it is
certified elsewhere.[^cert] This page answers one question: **which doors close forever when you
sign, and which ones only look like they close.**

## The answer in one paragraph

Freezing era-4 locks **four things about a block, and nothing else**. It locks which fields exist
and which are hashed; it locks the committed leaf set and how each value is encoded; it locks the
activation rule; and it locks the verifier posture. Everything else in silt — every validity rule,
every test, every runbook, every node-local file format — stays changeable after the freeze. The
asymmetry that matters is this: **after the freeze you can always make the rules stricter, but you
can never change the shape.**

## Why the four doors are expensive

Changing a frozen format is not an edit. It is a new era: a new block version behind a
height-gated hard fork, with its own certification, its own activation height, and a coordinated
upgrade of every node. An un-upgraded node **stalls** at the boundary rather than accept a format
it cannot validate — correct, safety-first behavior, and also a network-wide event. That is why
`TENETS.md` Part IX puts frozen formats in the immutable tier alongside the mission itself.

So a format item that is *wrong* at the freeze is not a bug you patch. It is a fork you schedule.

## The four doors that close

| The door | What it means concretely | What a miss costs |
|---|---|---|
| **1. The block schema** | Which fields exist on a block, their cbor keys, and which of them are inside the `Hash()` preimage that attesters sign. | A new era. A frozen binary receiving an unknown key **drops it, re-marshals a different body, computes a different hash, and rejects the block** — so an added field is not backward-invisible, it is a fork. |
| **2. The committed leaves and their value encoding** | Which facts are folded under the committed state root, and the exact canonical bytes each value takes. | A new era. Two nodes disagreeing on an encoding disagree on the root, which is a chain split. |
| **3. The activation rule** | How the network decides it is running era-4 — the height gate, the one-way weight tally, the boundary semantics. | A new era, and a nastier one: you would be changing the rule that decides which rules apply. |
| **4. The verifier posture** | Which roots are *required*, on which paths — including "no witness supplied for a key a predicate reads → never accept." | A new era. This is a format property because it decides what a valid block *is*, not merely what a node happens to check. |

These four are exactly what the era-3 freeze froze, in its own words (`docs/decisions.md:620-650`).
The era-4 entry will be written in the same four-part shape, and per Part IX **a freeze with no
`decisions.md` entry is not a freeze.**

## What does NOT close — and this is the load-bearing half

It is natural to assume a freeze locks the consensus rules. It does not, and the difference is
worth real money in scope.

**A narrowing validity rule is outside the freeze surface.** A rule that adds no field, no leaf and
no encoding — one that only *rejects more* than before — is a consensus-rule change gated by fleet
coordination, not by an era mint. It is admissible whenever the set of already-committed blocks it
newly rejects is empty.

This is not a reading we are proposing. It is already settled canon, and there is a decisive
precedent: **`D-F2-EVIDENCE-RECOMPUTE` is an owner-ratified consensus-rule change, recorded in its
own entry as "a narrowing consensus-rule change, NO era gate" (`docs/decisions.md:1320`). It changed
era-1, era-2 AND era-3 validity — and it landed on 2026-09-03, five days after you froze era-3 on
2026-08-29.** Under a reading where the freeze locks all consensus rules, that would have been a
violation. It was not; it was correct.

So these stay open after you sign:

- **Narrowing validity rules** — size caps, distinctness clauses, refusals. Cheap now (no live
  network), expensive after launch (a coordinated fleet fork), but **never an era**.
- **Test-tier obligations** — model-check properties, e2e fixtures, driven ablations.
- **Docs, runbooks, observables** — including the era observable an operator uses to see whether
  the stamp raise actually took.
- **Node-local formats** — anything not committed to a block. The credit ledger's on-disk shape is
  not a consensus format and has no era deadline.

## The practical consequence for the manifest

The manifest sorts its 22 items by **deadline, not by importance**:

| Deadline | What it binds | A miss costs |
|---|---|---|
| **At the freeze** | The four doors above. | A new era. Non-negotiable. |
| **At the stamp raise** (same release) | Narrowing validity rules, test obligations, observables, the runbook. | A coordinated fleet fork later. Cheap today; expensive after launch. |
| **At the flip** (later) | Anything reachable only once the floor box Accepts. | Nothing at the RC. |

Only the first row is irreversible, and only the first row needs your signature item by item. That
is why the format items come to you individually and the rest do not.

## One trap worth naming

**Reserving a format slot buys you a future cheap change only when the future change merely
POPULATES a field every frozen binary already round-trips.** It buys nothing when the future change
alters the hash preimage, the leaf set, or a rule's activation.

That theorem cuts both ways in this manifest, which is why two superficially similar "just reserve
it now" proposals were resolved in opposite directions: the proof-of-possession slot is worth
reserving, and two others were refuted as reservations that buy nothing. If someone proposes a
"reserve it now, swap the meaning later" hedge for anything that changes the hash itself, that is
the same error in a new costume.

## What you are actually signing at D3

1. That the FORMAT items — and only those — are correct as built, each having come to you
   individually.
2. That every value inside them was reached by a sound derivation, not by a route that happened to
   land in a safe place. *(This is why the derivation-route audit is an input to D1 rather than a
   parallel errand: a wrong value that survives the freeze is a new-era fix.)*
3. That the readiness stamp goes 3 → 5. No release ever stamps 4.
4. That this is the **last format touch**. After it, anything not frozen correctly is a fork.

What you are *not* signing away is the ability to tighten a rule, add a test, fix a runbook, or
change a node-local file. Those stay in your hands, at ordinary cost, for era-4's whole life.

[^cert]: `/Users/andrewedmond/Claude/claude/silt-reviews/research/research-outcome/ERA4-V5-FREEZE-MANIFEST-RESEARCH-CERTIFICATION-2026-09-07.md` — the 22-item manifest, its four classes, the three deadlines, and the per-item derivations. Its nine §8 owner sentences were ratified 2026-09-07 (`D-TRUE-UP-CALLS-2026-09-07` (9)).
