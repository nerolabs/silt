# R2.9a — the status surface: a cached snapshot, a fetch-only stamp, and the object leak

- **Date:** 2026-09-05 · **Seat:** BUILDER · **Branch:** `builder/r2.9a-status-surface`
- **Base:** `origin/main` = `4e67b5d`
- **Inputs of record:**
  - `/Users/andrewedmond/Claude/claude/silt-reviews/research/research-outcome/R2.9a-instrument-necessity-geometry-bound-and-tail-merging-RESEARCH-CERTIFICATION-2026-09-05.md`
    — **G-BB-26** (cached fixed-interval snapshot, CERTIFIED REQUIRED) and **G-BB-24**
    (`R-BB-STAMP-BY-ANY-PATH`) are the two gates built here. Tester gates **BB-21** and
    **BB-22** are the matching pins.
  - `/Users/andrewedmond/Claude/claude/silt-reviews/red-team/RED-TEAM-R2.9a-bbootstrap-instrument-and-containments-2026-09-05.md`
    — finding **F2**, the object half of who-fetched-what. Owner ratified closing it.
- **Not revisited here:** the build tag (PR #736), `q`, `W`, the population, the byte-axis
  bin count (G-BB-23, owner's), tail merging and count rounding (both REFUTED, §3.1–3.6 of
  the certification), and the transport-level `guard()` host/origin checks.

---

## 0. The three mechanisms, stated before the fix (build-immutable #6)

**Change 1.** `GET /api/status` recomputes the whole document on every request, inside the
node's event loop (`cmd/silt/ui.go:405`, within `s.onLoop`). Two consequences follow.
*The disclosed privacy bound is not a bound:* `core/credit/bbootstrap.go:171-177` discloses
`R-BB-DELTA-TRAJECTORY` as "bounded by the poll rate", and the poll rate is the reader's
own choice — there is no rate limiter anywhere on the UI server. *And it is a resource
finding on its own:* the handler walks the whole append-only, never-evicted account set
(`core/credit/bbootstrap.go:748-776`) plus the whole chunk store (`ui.go:395-397`), on the
event loop, per unauthenticated GET. Build-immutable #8: "an unbounded system on a small
box is not inefficient, it is unsafe." This change addresses both by recomputing at most
once per fixed interval `T` and serving the cached copy in between, which makes the
per-request cost `O(1)` and caps the amplification of a GET flood at 1 per interval instead
of at the attacker's request rate.

**Change 2.** The `B_bootstrap` age axis is specified as time since first *fetch*
(`core/credit/bbootstrap.go:6-10`). It is written in `Register`
(`core/credit/credit.go:363-371`), which every ledger path reaches through `acct()`, so it
actually records first ledger touch by **any** path — bond audit
(`core/node/bondaudit.go:234`), PoR grading (`core/node/por.go:348`), bounty payment
(`core/credit/escrow.go:168`) and false-repair slash (`core/credit/credit.go:480`). Any
identity that is also a DHT participant therefore publishes an over-stated age, unbounded
above by the ledger's uptime. This change addresses it by moving the stamp to the one place
`fetchedBytes` is written.

**Change 3.** `GET /api/status` publishes `durability.objects[]` — per content root, with
`funded` and `paid` — with no flag and no token. `RecordServeToObject` adds
`bytes * SkimNum / SkimDen` to `funded` (`core/credit/escrow.go:124-140`) and
`SkimNum/SkimDen = 1/8`, so `Δfunded × 8` is the **exact** byte count served of a **named**
root. `GET /api/economy/self` republishes the same per-root array (`ui.go:658-672`). That is
the object half of who-fetches-what, unauthenticated, and it predates R2.9a. Don't #3 is a
bright line. This change addresses it by requiring the API token for the per-object detail
on both endpoints, leaving every aggregate open.

---

## 1. Change 1 — the interval `T`

`T` is a **security parameter** (certification §3.5), so the owner ratifies its value. It
ships provisional, named once, in the shape `SlashesBytesCap` uses
(`core/chain/chain.go:430`).

### The options

| Option | Cost | Benefit |
|---|---|---|
| **A. No cache, correct the disclosure only** | Free | Closes nothing. The cert calls the cache REQUIRED on two independent grounds, and one of them is a resource finding. **Rejected.** |
| **B. Cache only the `bBootstrap` block** | Smallest diff | The cert's own ruling binds only that block, but the O(R) walk and the F2 leak both ride the rest of the document. Two caches later is worse than one now. **Rejected.** |
| **C. Cache the whole document** (chosen) | The document goes stale by up to `T` | One mechanism covers the trajectory bound, the immutable-#8 walk, and the F2 extraction rate. |

### The value, derived rather than picked

Four shipped numbers bound it. Two from above:

1. The shipped dashboard polls `/api/status` every **3,000 ms**
   (`cmd/silt/ui/index.html:108`). A `T` at or just above the poll period keeps the
   operator's view essentially live and still makes the recompute rate strictly lower than
   the request rate, so the amplification cap bites for the ordinary reader too.
2. The narrowest positive-width age bucket is **60 s**
   (`bbAgeEdgeNanos[2] − bbAgeEdgeNanos[1]`, `core/credit/bbootstrap.go:96-97`), and the
   `W` values the edges bracket run from an hour to a week (`:88-93`). Any `T` well inside
   60 s is over-sampled for the estimand by orders of magnitude — the cost to the fit is
   **zero**, as the certification derives.

Two from below: a larger `T` means fewer observations per identity
(`⌊uptime/T⌋` instead of the attacker's request rate) and a lower loop cost.

**Chosen: `T = 5 s`** — above the 3 s poll period, 12× inside the 60 s narrowest bucket.
It caps an unauthenticated observer at 12 observations per minute against an unbounded rate
today.

**The trade the owner owns, stated plainly:** the privacy side wants `T` much larger — at
`T = 5 s` an observer still gets 17,280 observations a day. Only the owner can trade the
dashboard's liveness for that, so the constant carries the `PROVISIONAL VALUE — OWNER
RATIFIES` marker and one named site changes it.

### Staleness must be visible

A cache that silently serves an old number is a silent-loss failure shape (Don't #4). The
document publishes `snapshotTakenAtUnix` (fixed for the life of one snapshot, so two reads
inside one interval stay byte-identical on it), `snapshotAgeSec` (computed at serve time,
so a reader cannot mistake a cached value for a live one), and `snapshotIntervalSec` (the
constant, on the wire, so an analyst can price the residual — the same discipline
`ByteBinRule` and `AgeEdgeNanos` already follow).

`snapshotAgeSec` is the one field that differs between two reads inside one interval. It is
top-level and outside the `bBootstrap` block, so **BB-21 — two reads inside one interval
return byte-identical `bBootstrap` blocks — holds exactly.**

### One thing the cache broke, found by a test rather than by reasoning

`TestEconomyEndToEndOnLiveDaemon` went RED on the first build with
`funding did not debit the balance: 500000 -> 500000`. Mechanism: `POST /api/fund` debits
the balance, and the very next `GET /api/status` served a document taken *before* the
write, so the operator's own mutation was invisible for up to `T`. That is a silent-loss
shape (Don't #4), and a worse one than ordinary polling staleness, because the client
knows it just wrote and reads the unchanged number as "the action failed".

**Fix: a token-gated mutation invalidates the snapshot.** `guard()` drops the cached
document after any mutating request that passed the token gate. It does not reopen the
amplification the cache closes: the hook is reachable only *after* `validToken`, so an
unauthenticated reader still cannot drive a recompute, and the operator's own mutations
are rare and already cost far more than an `O(R)` walk. Gate:
`TestR29aBB21OperatorsOwnWriteIsNotHiddenByTheCache`, RED without the hook at both the
unit and the e2e tier.

**Passive staleness is left alone and disclosed.** A GET that changes counters
(`/api/fetch`) does not invalidate. `integration/cloudtest/scenarios.sh` reads `funded`
and `paid` inside retry loops with a 300 s grace window and an SSH round trip per poll,
so 5 s of staleness is far inside its tolerance, and both loops already send the token.

---

## 2. Change 2 — where the stamp goes

`RecordBondChallenge`'s writer stays exactly as it is (it predates all of this and serves
the bond auditor). That forecloses the one-field option:

| Option | Verdict |
|---|---|
| **A. Keep one field, stamp it on the fetch path** | **REFUTED, and it is not a taste call.** `RecordBondChallenge` writes `firstSeenTick` at a **different event** — the first bond challenge this identity answered, which for a routing-table peer is long before its first fetch. A peer the node bond-audits first therefore already carries a non-zero `firstSeenTick`, so a fetch stamp guarded on `== 0` would never fire and the census would publish the **challenge** instant as the fetch age. Sharing the field *is* the defect. |
| **B. A dedicated `firstFetchTick`** (chosen) | Fires only on the fetch path, leaves the bond writer untouched, and closes the event collision structurally rather than by a comment. |

**CORRECTION, 2026-09-05, appended not substituted.** Row A originally refuted the
one-field option on **units**: it said the auditor's tick was "a small integer, not
nanoseconds". That is wrong, and PR #736 corrected the same error at four other sites
(`D-BB-BUILD-TAG`, CHANGELOG). `core/node/bondaudit.go` stamps `uint64(n.clock.Now())+1`
over the daemon's `walltime` node clock, so the auditor's tick is a wall-clock nanosecond
on the same axis the census reads. The row's text above is the corrected argument. **The
verdict does not move**: same unit, same clock, *different event*, and the different event
is the whole reason for the second field — with one shared field, an audited peer's
published age is measured from the challenge, which is the defect G-BB-24 exists to close.
`TestR29aBB22AgeIsMeasuredFromTheFetchNotTheBondChallenge` is the gate, and its fixture
(challenge at tick 7, fetch a day later, on one clock) reads correctly under the
correction.

Cost: 8 bytes per account. Accounts are already unbounded and never evicted
(`R-FP2-ACCOUNTS-UNBOUNDED`, open); this does not change that class.

**Placement.** `fetchedBytes` has exactly two writers — `RecordServe`
(`core/credit/credit.go:385`) and `RecordServeToObject` (`core/credit/escrow.go:132`).
Both are funnelled through one unexported `recordFetched`, so "stamped if and only if the
account is in the census" is structural, the same argument that put the old stamp in
`Register`.

---

## 3. Change 3 — the mechanism, and how much change 1 already closed

### What change 1 closes on its own, honestly

It closes **sub-interval attribution and nothing else**. Before, an observer could bracket
a single victim fetch between two reads and recover that fetch's exact byte count against a
named root. After, a delta covers a whole interval, so any set of fetches inside one
interval is unresolvable. At low traffic — which is this deployment — an interval still
routinely contains exactly one fetch, so the join survives, degraded. The certification
says the same in §3.5: "It degrades the join; it does not remove it. `R-BB-SIBLING-AGGREGATES`
stays open."

### Why not reduce precision

Rounding a **cumulative** counter does not stop delta extraction: an observer polling
across the rounding boundary recovers the increments, and the increments are the leak. Same
trap as adding noise per request to a cumulative cell. **REFUTED before it was tried.**

### The mechanism: token-gate the per-object detail, leave the aggregates open

| Published without a token | Published only with the token |
|---|---|
| `durability.bountyOn`, `durability.balance`; `stats.bytesServed`; the economy panels' pooled `skimIn`/`bountyOut`, revenue, margin, wash | `durability.objects[]` and `/api/economy/self`'s `objects[]` — the root and its `reserve`/`funded`/`paid`/`repairs`/`horizonSec`/`cliff` |

Nothing left open names a root. The pooled and node-wide figures are the aggregates that
already shipped; they carry no object identity, which is exactly the term the red-team's F2
join needs.

**Why the token is the right key here, against the red-team's own F9 critique of it.** F9's
objections all turn on the secret having to *travel* to a remote analyst: it is printed to
stdout, it rides the URL query, and it doubles as a write capability. Every one of those
bites for the histogram, whose consumer is a remote analyst. **None of them bite for the
durability detail, whose consumer is the operator's own node.** The token never leaves the
box, so F9 #1–#4 do not engage; and the holder is the operator, who already has write
control, so the privilege-escalation objection is vacuous. F9 #6 — "a token on the histogram
does not close F2, because `funded` is untokened" — is precisely what this change fixes,
from the other side.

**Withheld is not empty.** `detailWithheld: true` rides the block and `objects` is ABSENT,
not `[]`. A reader that sees an empty array knows the node caretakes nothing; a reader that
sees the key missing knows it was withheld. Same absent-vs-empty discipline the
minimum-requester floor already uses (`ui.go:493-543`).

### The hard constraint: the operator's solvency view keeps working

The durability horizon and the cliff early-warning are shipped features and this change
touches neither what is stored nor what is computed — only what is published, to whom.

- The embedded UI attaches `Authorization: Bearer` to every same-origin `/api/` call
  already (`cmd/silt/ui/app.js:26-29`), with no change needed.
- `integration/cloudtest/scenarios.sh` already reads `funded` with
  `-H "Authorization: Bearer $tok"` (`:2009-2024`, `:2192`).
- The observatory reads `capUsed`, `stats.BytesServed`, `chunks`, `chain`, `network` and
  `validator` cross-origin (`cmd/silt/ui/observatory.html:82-110`) and never touches
  `durability`, so the cross-origin path — which by design never receives a sibling's token
  — is unaffected.
- `e2e/economy_test.go`'s `getStatus` was the one untokened reader of the detail. It now
  sends the token it already holds, and a new arm asserts the withholding.

---

## 4. Evidence discipline

Each change ships one gate plus a controlled revert that turns it RED, recorded in the
commit message and reported to the planner. No knob moved without a named mechanism above.
PACE done before code, per the standing rule.
