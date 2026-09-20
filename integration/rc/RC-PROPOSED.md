# silt — the release candidate list

**The line this list draws:** a release candidate is the point at which handing silt to an
adversary is a responsible act. Not the point at which it is pleasant to use.

Every item carries a done-condition, the evidence tiers it must clear, and a gate test that
is proven RED before its fix. A gate that was never red is a comment, not evidence — so
each one also carries a vacuity guard proving its fixture could fail.

**The date: 2026-09-27.** Anything not green on that date ships disclosed rather than fixed.

**The verdict this list ships toward is not on it.** An external adversary — given the
artifact and the claims, but not the rationale — returns DENIED on publish-to-identity
linkage, on identity-level or global takedown, and on Sybil-farmed standing at a discount.
That is the mission, and it is not the builder's to turn green. The items below are the
evidence that makes commissioning it worth doing. They are not a substitute for it.

---

## Tier A — the handoff is not responsible without these

**1. Eligibility resolves from the block's parent state, never the replica's live latch.** ✅ *done*
*Done:* the floor box proves every whole-set pre-state member list against `prevStateRoot`
where it reads it, so an omitted or injected id stalls. It no longer depends on a later fold
op, which was emitted only when the set changed — and the post-set is derived from the
pre-set, so the forgery itself decided whether it would be checked.
*Evidence:* unit → consensus model-check → e2e.
*What it closed:* re-seating a slashed equivocator as bonded and qualified; erasing a bond
registration so its block folds to the root before it; keeping an under-bonded validator's
standing; evicting an honest validator from the frozen epoch set for an epoch; staling the
decentralization digest that decides whether the launch anchors have shed. Each is a gate
that was seen red before the fix and asserts a stall after it.
*Also closed:* the cross-phase seam — quorum intersection holds across the launch→mature
handoff, phase follows applied history rather than delivery order, and the one-way shed does
not re-arm under fork adoption.

**2. Forging N standings costs N×, with no arm passing vacuously.** ⚠ *three of five axes deny, each driven to the integration tier; the README disclosure is the owner's call*
*Done:* every arm paired with a positive control **on its own axis**. The bond and
possession arms pass. The demand and diversity arms do not construct, and are reported
**unwired** rather than denied.
*Evidence:* unit → integration → field.

*THE ITEM UNDERCOUNTED THE ARMS.* `VISION.md` names FIVE economies of scale a Sybil relies on and
one denial for each, not four — the fifth is retention decay, which is wired and denying, and the
item did not credit it. `core/chain/sybil_cost_census_v5_test.go` drives all five at the place
standing is decided and reports:

| axis | verdict | the economy of scale |
|---|---|---|
| bond size | **DENIED** | one sealed plot backing many identities |
| possession | **DENIED** | synthetic bytes standing in for storage |
| demand | *UNWIRED* | self-dealt demand buying standing |
| diversity | *UNWIRED* | massing identities behind one operator or subnet |
| retention | **DENIED** | coasting on a single one-time proof |

Three of five deny an economy of scale. Consensus standing is gated by the bond axis **and its
time dimension** — which is a correction to the loose form "the bond axis alone".

*UNWIRED IS A RESULT, NOT A SKIP,* and the arms that report it drive the fact rather than assert
it: standing is the bonded size and nothing else, so demand neither earns standing nor can be
inflated to buy it; and four identities behind one declared domain each hold FULL standing, because
the domain enters the C2 concentration metric that gates the maturity shed rather than
`idQualifies` — and it is self-declared, so a splitter separates into four domains at no cost.

*NO ARM PASSES VACUOUSLY.* Each denial carries a positive control on its own axis — the same
fixture made to GRANT standing by removing exactly the attack — because otherwise an arm passes
whenever anything at all goes wrong. Four ablations, each red for its own reason: remove per-root
ownership and three identities on one plot all earn standing; stop evicting on the TTL and coasting
works; skip the space-time verify and synthetic bytes earn standing; and WIRE the diversity axis
into `idQualifies` and the arm reporting it UNWIRED goes red rather than staying stale — which is
what stops an UNWIRED verdict rotting into a lie once an axis is connected.

*A FIXTURE ERROR WORTH KEEPING IN THE RECORD:* the possession arm first drove `apply` directly and
reported the axis broken. `apply` does not verify a space-time proof — its own comment says a
height>0 registration "was already VERIFIED by validateBondRegs" — so driving it bypasses the
screen entirely. The arm drives validation now, which is where a registration from the network
actually arrives, and its doc comment says so because the next person will reach for `apply` too.

*THE COUPLED CANON EDIT IS DONE.* The build-status paragraph is removed from `VISION.md`, in the
same change that leaves the census reporting what it recorded. `VISION.md`'s own header says the
document is "deliberately **not** a status report: where today's build differs from this picture,
the code and its tests are the honest record" — so the paragraph contradicted the page it sat on,
and the honest record it points at is now a test that runs. Nothing overclaims as a result: the
preceding sentence already calls the interlock "the target", and the census carries the detail.

*THE SAME PARAGRAPH IS ALSO IN `README.md`, AND IT WAS CORRECTED — the second of the three options,
taken on 2026-09-15.* The front door had said "Today consensus standing is gated by the bond axis
alone", which the census had already falsified: standing is gated by the bond axis AND ITS TIME
DIMENSION, because retention decay is wired and denying. It now says so, keeps the disclosure that
the multiplicative interlock is the target rather than the operative guarantee, and points at
`core/chain/sybil_cost_census_v5_test.go` so the grader can read which axes deny from the test rather
than from prose. The wording was corrected to match a measurement, never to make a gate pass —
which is the distinction `README.md`-as-published-claim exists to protect.

*THIS SHEET CARRIED THE OPEN QUESTION FOR FOUR DAYS AFTER IT WAS ANSWERED,* which is the drift R2
forbids, and it is worth naming rather than quietly fixing: a decision that is made in the tree and
left open on the sheet reads to the next session as work still owed, and the next session budgets
for it.

*THE INTEGRATION TIER, driven 2026-09-15 over real containers.* It needed no new suite: two of the
three DENYING arms already have integration coverage in `integration/bond/`, and both passed — at
HEAD and again at `d968fa2` in an isolated worktree, so the evidence is not an artefact of this
session's changes.

| arm | integration evidence | result |
|---|---|---|
| bond size | `bond` PHASE 3 — a second identity re-advertising the SAME root | **PASS** — earns ZERO (one plot, one standing) |
| possession | `bond` PHASE 1 — a real plot sealed and challenged over the wire | **PASS** — 64M sealed in 0.90 s, 67,108,912 B on disk, peer challenge passed |
| retention | `retention` — an identity stops re-proving while three keep going | **PASS** — evicted from the committed bonded set at the TTL; the three that kept re-proving kept their standing |
| demand | — | unwired: nothing to drive at any tier |
| diversity | — | unwired for standing: nothing to drive at any tier |

*THE RETENTION ARM IS DRIVEN, in `integration/retention`.* Four bonded objective validators seal
real plots and commit real registrations; one of them is stopped so it can no longer renew; and the
committed bonded set — the daemon's own `chain: saved … bonded=N` line, read from a node that never
leaves — falls from 4 to 3 while the chain keeps committing. It comes back to 4 only when the
returning node's FRESH registration commits, which is the other half of the claim: the proof is what
buys the standing, every time.

| leg | measured |
|---|---|
| the set fills | four real bonds commit; `bonded=4` at height 4 |
| CONTROL — nobody stopped | `bonded` held at 4 across heights 4→10, a window longer than the TTL, and never dipped |
| decay | one identity stopped re-proving at height 10; `bonded` 4 → 3 by height 12 |
| re-earned, not restored | it returned on its own store and `bonded` went back to 4 at height 14 |

*THE CONTROL IS THE POINT, and it is on retention's own axis.* A falling counter proves nothing by
itself — it would fall just as far if the swarm were dying, or if standing decayed with time
regardless of renewal. So the same window runs first with the attack REMOVED, and the counter has to
hold at four. Only then does the fall attribute to the one thing that changed.

*AND THE ARM WAS SEEN RED.* `BOND_TTL=0 ./run.sh` — the daemon's own documented opt-out, not test
scaffolding — keeps legs 1 and 2 green and fails leg 3 with "the committed bonded set is STILL 4 at
height 11 … An identity is coasting on a proof it can no longer answer for", on a chain that
advanced 10→11. The liveness assertion runs first, so an ablation that had merely killed the swarm
would have failed for a different and clearly-named reason.

*IT NEEDED ITS OWN TOPOLOGY, and the reason is worth recording rather than filed as a preference.*
The expectation here was that this would be an assertion in an existing suite. Three were checked
and none can carry it. `bond` sets `-bond-ttl=0` deliberately — its own arms need standing to hold
still — and never advances a chain for a bond to age against. `sybil` leaves the TTL at the derived
default of 32 blocks while its chain reaches a height in the low single digits, so the window is
unreachable there. And `floor` runs THREE validators, where the Byzantine support set is n−f = 3 —
every validator. Removing one stops the chain, and the TTL sweep runs inside block application, so
the eviction rides on the very blocks the removal prevents: **at three validators the mechanism
cannot be observed by removal at all.** That is why the new topology has four, where f = 1 and the
three survivors still commit.

*A SIDE OBSERVATION FROM THE ABLATION, not claimed as a result.* With the TTL ON the swarm recovered
from losing a validator — the eviction dropped the set to three, and three of three is a support set
the survivors can meet. With the TTL OFF the set stayed at four with one member permanently absent,
and the chain advanced once and then crawled. Retention decay is doing liveness work here as well as
Sybil work. That was measured on one run of each and is recorded as a thing to look at, not as a
claim.

*A caveat that travelled with the bond evidence, now CLOSED:* `integration/bond` used to RESULT IN
FAIL on its `POSITIVE-2` control. That was the same adversary-harness era-mint defect as `redteam`,
fixed at `ee4b21f`; the suite passes as a whole and `POSITIVE-2` now reads "goodpropose proposal
ACCEPTED". The PHASE 1 and PHASE 3 arms this item reads were never the failing part.

*Still owed:* the field tier.

**3. The shipped default is the defended configuration, and the stock binary runs.** ⚠ *the defended half is gated; "reaches serving" is unreachable BY DESIGN and the condition is restated*
*Done:* every defence this list demonstrates runs flagless; the stock EDGE node reaches serving on
pure defaults; the stock VALIDATOR refuses, for a reason that names what the operator must supply.
*Evidence:* integration → e2e → field. Gates four other items.

*THE DONE-CONDITION WAS WRONG, and the measurement is what corrected it.* It asked that
`silt daemon -validator` with no other flags reach serving. It cannot, and it must not. Driven
against the built binary 2026-09-15, the stock validator posture meets TWO refusals in order:

1. ~~**The `-bond` default (64 MiB) is below the anti-release floor (1029 MiB)**~~ **— CLOSED.**
   The bond now DERIVES from the floor when the operator names neither, the same way the floor, the
   re-challenge TTL, the quorum sizing, the operator margin and the epoch cadence already do. The
   shipped literal stays 64 MiB on purpose: the floor applies only on the objective path, and a
   trusted or demo swarm should keep paying for the small plot it asked for rather than sealing a
   gigabyte it has no use for — raising the literal for every posture would have raised the floor of
   honest participation, which build-immutable #4 forbids. An EXPLICIT sub-floor `-bond` is still
   refused rather than silently raised: an operator who asks for a bond that earns no standing is
   told so.
2. **Behind it, the one that cannot be defaulted away.** With the floor cleared the daemon still
   exits: *"refusing to start — an untrusted objective validator with no cold-start scaffolding
   would treat itself as mature from genesis (no anchor co-sign), letting a young or Sybil quorum
   self-certify and capture."* That is the M0 cold-start capture defence working. The remedies it
   names — `-anchors`, `-mature-validators`, `-ws-checkpoint` — carry NETWORK-SPECIFIC values, so
   no shipped default can supply them. A validator that started without them would be the exact
   quiet capture this list's third claim denies.

So "the stock binary runs" is true of the edge node and correctly FALSE of the validator, and the
item now says that instead. Removing the spurious refusal was worth doing and is done — the
substantive one is no longer hidden behind it — and, as predicted, it did not make the original
wording true. Nothing can: that is the point of the second refusal.

*DRIVEN, and newly gated:* `TestShippedDefaultLane_EveryDefenceIsOnBeforeAnyFlag` starts the stock
validator posture with nothing but the role selector and requires five defences to announce
themselves ARMED — the anti-release bond floor, the objective re-challenge TTL, Byzantine quorum
sizing, the operator split margin, and the epoch freeze cadence — each naming its own override in
the same line. It reads the daemon's own output rather than the flag table, because a default read
from a flag declaration cannot see a value DERIVED at start-up from the swarm's trust posture, which
is what the floor and the TTL are. Red under ablation twice: a defence that stops defaulting ON, and
a defence that arms without naming its off switch.

*Also measured, and reported as numbers rather than asserted:* the publish-token replay guard is ON
unconditionally — it is a validity rule in the accept path, not a flag, so no configuration can
disable it. The validator's bond possession audit defaults to 60 s. The proof-of-retrievability
audit sweep (`-audit`) defaults to 0, and `-require-tokens` defaults to 0; both are per-deployment
choices rather than defences this item can call flagless, and naming them here is what stops the
item from claiming more than it drove.

**4. The repo reads without the record that was deleted.** ✅ *gated*
*Done:* no milestone, lane, catalog, slice, issue or change-request identifiers in source,
test names or file names. Comments describe the product or cite external work.
*Evidence:* unit — `internal/depcheck`, a permanent source gate kept in the suite so it cannot
regress, driven RED on the tree before the source was cleared.

*The gate was built first and seen red first,* which is the only order that proves it can fire on
this repository rather than on a synthetic sample. It reported **236 hits**, and clearing them is
what turned it green — 73 files, 219 process identifiers plus four dead document pointers. Four
arms: the banned vocabulary in source, the same vocabulary in file names, every in-repo document a
comment cites actually existing, and teeth that feed every term its own violating line and every
exemption its own allowed phrase.

*What the cleanup did NOT do is delete reasons.* A comment that pointed at a real thing by a build
number now points at it by name: `increment 2's recomputeMatureNow` became `recomputeMatureNow`,
and the recompute files cite each other by file name. The reader keeps the cross-reference and
loses only the build's private numbering.

*Three matches were the product's own words and were KEPT,* which is why the gate carries an
allowlist of exact phrases with reasons rather than looser patterns. `PHASE 1 (prepare)` /
`PHASE 2 (precommit)` in the consensus loop are BFT phases with `PhasePrepare` / `PhasePrecommit`
behind them — renamed to name the leg rather than number it. A relay pump's `sub-increment read
buffer` is sized below one authorized payment increment, and a hash-chain payer really does advance
to `increment 5 by revealing x_5`. The gate also fails on a DEAD exemption: if the text an
exemption excused moves, nobody re-read the rule against what replaced it.

*Two limits, stated rather than left to be discovered:*
- **Bare letter-number tags** (`D3`, `H-4`, `F1`) are not in the gate. `D3` alone has 65 hits and
  they collide with real domain names here; a gate that fires on domain terms teaches people to
  work around it. Their absence is a limit of the gate, not a permission.
- **GitHub issue references are gone too, and the gate bans them.** 214 references to 27 issues
  across 78 files. They resolved — the issues are real and readable — and an earlier draft of this
  item argued that made them live citations worth keeping. That argument is retired: direction comes
  from `VISION.md`, `TENETS.md` and this list, and from nothing else, so a comment that sends its
  reader to a tracker is pointing at a record this project deliberately does not keep. The pattern
  is `#` followed by two or more digits, which cannot reach the canon's own numbered items: the
  immutables and the don'ts are single digits, and the persona references were rewritten to name the
  persona rather than hash a number, so the rule has no exceptions to remember.
- **Bare letter-number tags** (`D3`, `H-4`, `F1`) are still not in the gate. `D3` alone has 65 hits
  and they collide with real domain names here; a gate that fires on a domain term teaches people to
  work around it. Their absence is a limit of the gate, written into its own header.

**5. One command from a clean clone reproduces every gate.** ⚠ *all five causes CLOSED (three were product defects); 11 of 12 gate suites pass locally + 22/22 in cloud; `churn` is undrivable in this environment*
*Done:* fresh clone, no credentials, no committed binary → the suite runner builds from
source and maps each item onto a named suite.
*Evidence:* a scratch clone with a COLD build cache for the unit and e2e tiers; the whole set
driven locally, start-clean and end-clean.

*THE LOCAL CLONE, driven 2026-09-15:* no credentials (`config.env.example` travels, `config.env`
does not); no committed binary (zero executables, the only tracked `application/octet-stream` files
are CBOR fixtures); builds from source in 110 s from an EMPTY `GOCACHE`, leaving 1,889 fresh
entries; the runner's catalog carries 17 entries each naming a suite, its tier, its timeout and the
claim it gates; and `go test ./e2e/ -count=1` over that cold cache is green, 43 tests, 479.9 s. The
honest limit: `GOMODCACHE` was shared, so third-party dependencies were not re-downloaded — they are
not silt, and re-fetching them measures a network.

*THE WHOLE SET, DRIVEN — and the record it replaces was wrong.* This item said "the run itself
returned rc=137 or rc=124 on all twelve suites", first attributed to host contention, then corrected
to inherited leftovers. **Neither holds.** Driven on a box verified clean at the start — zero
containers, zero stray daemons, 2.4 GB wired, load 1.80 — the result is not uniform at all:

    summary: 7 pass · 0 finding · 3 fail · 2 timeout · 0 skip        (~40 minutes)

| PASS | FAIL | TIMEOUT |
|---|---|---|
| `privacy` `client` `nat` `audit` `economy` `churn` `chaos` | `bond` `sybil` `takedown` | `consensus` `redteam` |

That table is the 2026-09-15 sweep and it is the baseline, not the current state. `redteam` has since
moved: its wedge was a product defect, it is fixed, and all four accountability drills now report PASS
— see the `redteam` entry below for what still fails there and why it is a lesser thing.

Seven suites pass, including `churn` at 16m52s — six of sixteen holders killed across two waves,
fifteen stripe-repair sweeps, every fetch bit-perfect — and `audit`, which catches a liar WITHOUT
fetching its bytes. There was never one cause to find, which is why both previous explanations were
wrong: they reasoned about a symptom pattern that does not exist.

*THE FIVE, EACH WITH ITS OWN CAUSE:*
- **`takedown` — DIAGNOSED, a stale assertion, not a product fault.** The daemon prints
  `denylist: N root(s) denied; purged M held chunk(s)` when it purges and
  `denylist: honoring N denied root(s)` when there is nothing to purge. The suite greps
  `denylist: [0-9]+ root`, which matches only the first. It therefore fails whenever the daemon
  legitimately takes the honoring branch, which makes it timing-dependent by construction. The same
  capability PASSES in `cloudtest` on the same commit, because that harness asserts the EFFECT —
  one operator stops serving while another keeps serving bit-perfect. Asserting on a log string
  where an effect was available is the defect.
- **`bond` — a positive control refused, and NOT caused by this session.** `POSITIVE-2: FAIL —
  honest REFUSED a well-formed, 64M-bonded proposal; its attest path rejects EVERYTHING`. Driven
  again at `d968fa2`, the commit before this session's first change, in an isolated worktree: the
  SAME assertion fails byte-identically. Pre-existing. The suite catches it itself and says so — its
  own negative result would otherwise read as proof of the bond gate when the node is rejecting
  everything.
- **`bond` — SAME ROOT CAUSE AS `redteam`, fixed by the same commit.** `POSITIVE-2` reported
  "honest REFUSED a well-formed, 64M-bonded proposal; its attest path rejects EVERYTHING". It drives
  `-goodpropose`, which is `ProposeGoodBlock` — the same builder that stamped a pre-v5 version into
  every block it made. The honest target refused it for the ERA, not the bond, and the suite read
  that as the attest path being broken. One defect, two suites; only one of them showed it as a
  wedge. Needs a re-drive to confirm, no further change expected.
- **`sybil` — the same shape, denials intact.** `C2-a` and `C2-b` both PASS: the young network stays
  live with anchors present, and NO block passes the anchored ceiling, so the Sybil quorum cannot
  capture. What fails is `C2-a2`, a liveness control — a Sybil node never earns publishable standing.

  *DIAGNOSED 2026-09-16: the control rests on a false premise, and an error name hid it.* The full
  refusal is `proposer da799b47… is not a launch anchor (young network proposes anchor-only; bonded
  8388608)`. s1 IS bonded. It is refused by the LAUNCH-WINDOW rule, which admits only anchors as
  proposers while the network is young — the rule that exists to remove the sybil-proposed launch
  fork at its source. `launchAnchor(id)` asks whether *id* is an anchor, not whether anchors are up,
  so s1 can never propose while young, anchors present or not. C2-a2's premise — "the drain banked
  the sybil's standing, so s1 must now clear the proposer gate" — is wrong: banking a bond does not
  override the anchor-only rule. And C2-a, immediately above it, ASSERTS the training wheels are
  engaged. The two controls contradict each other.

  The name is why this took a re-read: the refusal is wrapped in `ErrLowReputation`
  ("chain: reputation below threshold") though it has nothing to do with reputation, so the suite's
  own note blamed the drain. That is the third time in this sweep a refusal named the wrong cause.

  *FIXED AND DRIVEN GREEN 2026-09-16: `RESULT: PASS`, whole suite.* C2-a2 now asserts the thing the
  design actually provides and the thing C2-b depends on: a bonded sybil earns committed standing by
  SUBMITTING its registration for an anchor to bank (submit-don't-propose), never by proposing. The
  observable is the committed block itself — `chain: committed block N (E entries, B bond-regs, …)`
  with B ≥ 1, a registered entry in `cmd/silt/observable_contract.go` — because standing becomes real
  when a registration COMMITS, not when an internal sweep decides to try. A negative half was added and then REMOVED as
  unsound: `chain: committed block N` is printed by whoever COMMITS a block, proposer and syncer
  alike, and C2-a depends on exactly that — it waits for that line ON THE SYBIL to prove the sybil
  SYNCED the anchors' block. Grepping it as "the sybil proposed" therefore failed the moment the
  sybil did the very thing C2-a requires, and passed only while it was still behind: a race dressed
  as a security assertion, which is why it passed standalone and failed in the sweep. No
  proposer-side observable exists at this altitude, so that property stays where it can actually be
  measured — C2-b's anchored-ceiling gate. Scope is stated in the output: the committed-block line does not name WHOSE registration is in the block,
  so this proves the banking route ran, and C2-b's gate reporting distinguishes the anchor gate from
  a standing gate.
- **`takedown` — A PRODUCT DEFECT, not suite rot. Fixed 2026-09-16.** The operator takedown purge
  did nothing on a restart, which is the only workflow it has. `EnforceDenylist` sweeps the resident
  chunk index; that index is rebuilt by `StartProofReload`, made ASYNC on purpose because a
  synchronous scan held the relay and registry listeners down for ~9 minutes per restart on a 14 GB
  store. The purge ran at config time, swept a map the scan had not filled, purged nothing, and
  printed `denylist: honoring 1 denied root(s)`. The operator is told the list is honored; the denied
  bytes stay on disk. Purging zero is indistinguishable from having nothing to purge, so it was
  silent. Evidence: opA held 18 objects, served the target, purged 0.

  *Why the race was permanent.* The async scan is safe for the re-announce path because an announce
  that races it self-corrects on the next reprovide sweep — the code says so. The purge is a ONE-SHOT
  sweep with no next pass. `sim/takedown.go` calls it on in-memory nodes that already hold their
  chunks, which is why `silt sim run takedown` passed: the gate lived only on the path that works.

  *The quieter half was worse.* `chunkDenied` reads the same index and returns `ok && denied`, so an
  ABSENT entry reads as NOT DENIED — and `AnnounceHeld` skips advertising only what `chunkDenied`
  reports. A restarted node RE-ANNOUNCED the taken-down chunks to the DHT. The refusal-to-serve half
  always held (correct per-operator scope, reversible); it is the deletion and the non-advertisement
  that failed.

  *Fixed* by running the purge from the reload-completion callback, so it sweeps a full index.
  Gated by `core/node/denylist_restart_purge_test.go` (rule, vacuity guard, and a positive control
  that the callback is not a general-purpose shredder); ablated back to a config-time sweep it
  reports `PURGED 0 OF 6 DENIED CHUNKS`. The suite's grep was also genuinely rotten — it matched only
  the purge branch, so it could not tell "never enforced" from "enforced and purged nothing", which
  are different bugs and the second hid the first.

  *A SECOND DEFECT AT THE SAME SEAM, found by the full sweep.* The first standalone drive passed;
  the sweep drive FAILED on a different assertion — `control file died on opA — takedown was not
  per-hash` — with the purge evidence byte-identical in both. `AnnounceHeld` runs in synchronous
  startup, BEFORE any reload batch, so it advertises from a cold index and `placementKey` falls back
  to the bare chunk id — a key no reader queries. `daemon.go` states that contract and its
  consequence ("AnnounceHeld, below, reads the reloaded proofs — otherwise a disk full of content is
  invisible until re-hosted"); the lazy reload broke it without moving the comment. It self-heals on
  the next reprovide sweep, which is why it presented as a flake. Fixed by announcing again from the
  reload-completion callback, ordered after the purge; gated by
  `core/node/announce_after_reload_test.go` with a paired ablation proving a cold announce misses the
  column key.

  *RE-DRIVEN 2026-09-16: `RESULT: PASS`, whole suite, first time green — and PASS again in the
  verification drive after the announce fix.* The deployed path now
  reports `denylist: purged 8 held chunk(s) once the proof index finished loading`, and opA's object
  count drops 18 → 10 while opB stays at 9. Same fixture as the failing run (18/9), so it is a
  like-for-like comparison: 8 chunks physically deleted where the old build deleted none. The
  config-time call still prints `honoring 1 denied root(s)` and still purges nothing — it is left in
  place because it is correct for a node whose index is already resident (the sim path), and the
  callback covers the restart.
- **`consensus` — DIAGNOSED 2026-09-16: it cannot commit as written, and the budget is not why.**
  The first reading was that the cap fired while the suite was visibly progressing, so 300 s looked
  thin. That hypothesis was tested and is WRONG. Driven on an idle box with the timeout helpers made
  honest it TIMED OUT again; driven at 900 s it TIMED OUT at 15m02s, having reached the same place.

  *The cause.* The suite seats FOUR anchors and partitions them 2–2 (A,B | C,D). Objective mode
  requires a DERIVED strict anchor majority, `⌊A/2⌋+1` = 3 of 4, derived exactly so configuration
  cannot disable quorum intersection — the daemon prints it at start-up: `training wheels: 4
  anchor(s), strict majority 3 required (objective; derived)`. Neither side of a 2–2 split reaches
  3, so nothing commits on either side. Observed: all four validators at ZERO committed blocks.
  Every publish retry is doomed by construction, which is what consumes the budget.

  *The rule is not a regression, and this is the second suite to encode a pre-rule expectation.*
  `proposerQualifiedAt` names this shape as the thing it exists to refuse — "the both-sybil-proposed
  2-2 anchor split the intersecting-quorum invariant (I1) must otherwise refuse". Like `sybil`'s
  C2-a2, the suite asserts something a deliberately-added safety rule now forbids.

  *FIXED AND DRIVEN GREEN 2026-09-16: `RESULT: PASS`, every phase.* The suite is now split 3–1, which
  is what its own catalog claim always said — "a sub-quorum partition commits nothing, stalls, and
  catches up to the majority history on heal". A 2–2 split has no majority side, so there was no
  majority history to catch up to and the claim was untestable. P2 no longer asserts the two sides
  commit RIVAL heads (a fork, which is the thing intersection prevents); it asserts the majority
  advances while the severed minority commits NOTHING. Observed: majority to h=3 with the minority
  at zero committed blocks, then the healed minority converging at h=4 by CATCH-UP, with nothing to
  drop because it had committed nothing.

  *Three harness bugs surfaced on the way, all of the same family — a bound that does not bound what
  it appears to.* (1) The minority publish used the success-path retry budget, spending 90 s proving
  a foregone conclusion. (2) That budget is checked only BETWEEN attempts, so one hung attempt sailed
  past it — measured at ~5 minutes. (3) The per-attempt fix wrapped `timeout` around `dc`, a shell
  FUNCTION, and timeout(1) execs a binary: every attempt died instantly with "cannot open file:
  exec". That third one is the dangerous shape, because the chain still advances on its own sweep,
  so the suite looked like it had merely lost a link rather than never having published at all.
  The budget is 600 s, measured from a run that reached P2 at the 300 s mark rather than guessed.
- **`redteam` — WEDGED; DIAGNOSED AND FIXED 2026-09-16, re-drive outstanding.** The symptom was
  three progress lines, then `adversary: equivocation attempt refused: place Y on …` repeating
  until the cap.

  *The cause was not standing.* The red-team block builders in `core/node/adversary.go` stamped
  `BlockVersionRounds` (v2) into every block they made. `-era4-activation-height` defaults to 1 and
  the daemon refuses to start with any other value, so v5 is required at height 1 — the FIRST
  height an adversary can ever propose. Every adversary block was refused by the era rule before
  any consensus property was reached. No warm-up fixes that and no retry outlasts it. An honest
  proposer never picks a version; it asks `chain.MintVersion`, and the objective adversary path
  (`PlaceConflictingSigned`) already derived its version from the committed block. The older
  drive-in builders were simply never moved up with the era.

  *The refusal named the wrong cause, and that is the second defect.* The reply is a bare
  `OK=false`, so the adversary could not know why; it guessed `(not yet standing?)`. The guess was
  plausible and wrong, and it held the diagnosis on the standing axis for a full suite budget.
  The line now names the block it offered (height and version) and points at the target's debug
  log, which has carried the real reason all along.

  *The blast radius was wider than the wedge.* `ProposeGoodBlock` is the harness's own positive
  control, the proposal an honest target must ACCEPT, and scenarios 2 and 3 are gated on it. It
  built v2 too. So the forged-block and low-bond drills were refusing for the version rather than
  for the forgery or the bond — REPORTING PASS while proving nothing. A false green is worse than
  the red beside it, and only the equivocation wedge was visible from the outside.

  *Why no gate caught it.* Every in-process drill gate builds its chain without an era-4
  activation height, so all of them exercise the legacy v2 format and none could see a mint-era
  defect. `core/node/adversary_era_mint_test.go` now drives the drill at the SHIPPED era, with
  standing granted by fiat so a refusal can only be the era. Ablated back to the hardcoded
  version it reproduces the wire symptom byte-for-byte, including the false `(not yet standing?)`
  attribution.

  *DRIVEN 2026-09-16, and every accountability property is now observed.* The four drills that
  were unobtainable all report, on real containers over real TCP:

  | drill | before | after |
  |---|---|---|
  | 1 — equivocator caught and SLASHED | never placed | **PASS** |
  | positive control — H3 accepts a bonded proposal | would have refused | **PASS** |
  | 2 — forged block rejected | passed for the wrong reason | **PASS** |
  | 3 — low-bond proposer refused | passed for the wrong reason | **PASS** |
  | H3 cross-check — no adversarial block committed | not reached | **PASS** |

  The adversary logged ZERO refusals, against 15 in the wedged run, and the chain carried the
  eviction: `chain: slashed equivocator d4f5ec0d… (double-signed at height 1)`. The fork shapes are
  the drill's own design, confirmed on the wire — `equiv-x` at height 1 (the losing fork),
  `equiv-yz` at height 2 (the heavier Y→Z fork).

  *The suite still exits FAIL, on a different and lesser thing.* The FIRST positive control — H1
  and H2 committing an ordinary publish — failed because the publish could not place a manifest
  chunk (`placed on no node after 4 attempts`). That is storage placement timing out on a host at
  load 49 with the owner's game running, not a consensus or accountability defect, and it is the
  one leg that did not reproduce the earlier run (where it PASSED). It needs a re-drive on a quiet
  box before it can be called anything else.

  *Two suite-mechanics defects were fixed alongside.* The budget went 300 s → 480 s, because the
  failure path's own waits could not report inside 300 s — which is why this arrived as `rc=124`
  with no diagnostics rather than a FAIL that named itself. And `wait_log` counted ITERATIONS while
  documenting SECONDS: each poll pays for a `docker compose logs` whose cost grows with the log, so
  every nominal timeout in this suite understated its true wall-clock, without bound, on a loaded
  host. It now reads a deadline off the clock. The failure path also dumps the TARGETS' logs, which
  hold the real refusal reason, instead of only the adversary's guess.

*WHAT THIS ITEM NOW MEANS.* The suite set was never green, and nobody knew, because it had never
been driven to completion on a box proven clean — the leftovers masked what each suite would
otherwise have reported. That is a more uncomfortable finding than a starved host and a more useful
one: seven suites are real evidence today, and the other five are five separate pieces of work with
five separate causes, four of them already named above.

*ALL FIVE ARE NOW CLOSED, AND THE SET IS RE-DRIVEN (2026-09-16).* Three of the five were REAL
PRODUCT DEFECTS wearing a suite failure's clothes, and all three had one signature: a gate that
lived only on the path that works (in-process, or the sim) while the deployed path went ungated.
The adversary harness minted pre-v5 blocks, which took down `redteam` AND `bond` from one cause.
The operator takedown purge swept a chunk index the async reload had not filled. `AnnounceHeld`
advertised a restarted holder's coded shards under bare ids instead of placement keys — the widest
of the three, because it makes a restarted node's content unfindable until a reprovide sweep, and
it self-heals just fast enough to present as a flaky fetch. The other two suites (`consensus`,
`sybil`) asserted things the shipped safety rules deliberately forbid, and were rewritten to assert
what the rules actually provide.

*LOCAL, current HEAD:* `consensus` `bond` `redteam` `sybil` `retention` `chaos` `takedown` `privacy`
`client` `nat` `audit` `economy` all PASS. `churn` could not be driven to completion in this environment
(~18 min exceeds what a background task survives here); it passed before the daemon changes and its
ground is covered below. `floor` was re-driven at this HEAD and now returns PASS rather than a
FINDING, because the gap its verdict named — the ceiling on adversarial input — is closed.
`redteam` and `sybil` carry item 14's floor-spec seats and were re-driven green with them —
`redteam` four times, `sybil` at two farm sizes — each on a box proven clean by its own preflight.

*CLOUD, real GCP hardware, run `46f3224-79920`: 22 PASS · 0 FAIL · 6 SKIP.* 17 nodes, 4 validators,
three regions, randomized flow order, torn down with zero orphans. It confirms the two daemon fixes
where they actually matter: `7-restart-content` (content still fetchable BIT-PERFECT after a
storage-node restart — the announce fix end to end), `7-restart-standing`, and `8-takedown`. It also
covers the ground `churn` could not be driven over: `chaos-reprovide` (a SIGKILLed node re-announces
its held chunks), `chaos-fetch` (bit-perfect after hard-crash + restart) and `durability-turnover`
(content survives a PERMANENT departure). The accountability set matches the local `redteam` result:
`184-equivocation-island`, `184-forged-block`, `184-low-bond`, `184-partition`. The 6 SKIPs are all
opt-in or not-in-topology, none a failure. The harness's own pre-flight refused to spend a cent
until a LOCAL PROOF command was supplied and EXITED 0 — build-immutables #6/#7 enforced
structurally, so the run confirmed a green local integration rather than discovering one.

*UNIT TIER:* `go test -short ./...` → 58 packages ok, 0 failures. The non-short lane is
`go test -timeout 40m ./...` (what `release.yml` runs); its 1M-element fold-cost MEASUREMENT rung
exceeds Go's default 10m timeout, which is a property of the invocation, not a defect.

**6. The claims the adversary receives exist as an artifact.** ⚠ *written, not yet cold-read*
*Done:* an in-repo statement of the three denials in checkable form, written so a cold reader
can construct attacks from it. `ADVERSARY.md`, at the repository root.
*Evidence:* a cold read — by someone who has not read this list.

*CLAIMS IN, GAPS OUT — the owner ruled this 2026-09-15, and it amended this item.* The
done-condition used to read "plus what is out of scope". It no longer does. Handing the grader a
list of our own gaps means the grader confirms the gaps we already knew, and a verdict reached
that way proves nothing about the ones we did not. The artifact therefore states the three
claims at full strength, as claims UNDER TEST, and says so — it does not assert they hold, and it
does not map where they are weakest. The grader finds that itself, and a denial it reaches
independently is worth what one we steered it to is not.

*What "full strength" does and does not license.* Each claim is stated with its OWN boundaries,
because a boundary is part of the claim and omitting it would overclaim: access-unobservability
is metadata-layer and bounded by the anonymity trilemma; Sybil-resistance is re-pricing and
concentration-bounding rather than prevention. Those are the claim. A catalogue of where the
build falls short of the claim is not, and is not there.

*The out-of-scope material still exists* — in this list, under *What this list deliberately does
not cover*. It is for the owner, to hand over separately if the grader spends its budget on a
gap the vision already concedes. It is not in the artifact.

**7. No consensus state derives from a field the block hash does not cover.** ✅ *gated*
*Done:* the seating map — which sets the maturity coefficient and decides whether the launch
anchors have shed — cannot be changed without changing the block's hash. Below the
witnessable format it can: removing a block's bonded attestations leaves it hashing
identically, so a rewritten history is invisible to the finality gate, fork-choice and the
checkpoint, all of which compare by hash. The witnessable format resolves it by taking the
seating from a carrier of precommits over the parent, which is folded into the hash.
*Evidence:* unit → e2e. The daemon refuses any configuration that would run a height on a
pre-witnessable format.

**8. A validator set that cannot grow is refused.** ✅ *gated*
*Done:* a block can seat a validator the chain has not seen. Below the witnessable format it
cannot — the state root commits the seating while the seating is read from the block's own
attestations, which sign over that root — so the set is frozen, the network never matures,
and the anchors never shed.
*Evidence:* unit → e2e, in every mintable format.

## Tier B — reachable, none free

**9. Consensus denials hold, and no honest node is ever slashed.** ✅ *the distinctive clause EXISTS
and BOTH halves are driven GREEN — and chasing the undriven one turned up a product defect that is
now fixed*
Equivocation attributed;
forged and under-bonded proposals rejected pre-attestation; a partition heals to one order.
The honest-never-slashed property asserted over the whole run's slash set, not per attack.
*model-check → e2e → field.*

*THE CLAUSE DID NOT EXIST, WHICH IS WHY IT COULD NOT FAIL.* `integration/redteam` asserted that the
equivocator WAS slashed — one assertion per attack — and nothing asserted the complement. Every
attack in that suite could pass while an honest seat was being slashed beside it, because no seat's
slash set was ever read. The complement is a claim about a SET, and the set had no reading surface:
the only one was the daemon's `chain: slashed equivocator` narration, which says what a node
DECIDED, not what the history COMMITTED.

*THE SURFACE EXISTS NOW.* `silt chain-status` reports the committed slash set — one line per
CULPRIT, keyed by identity rather than by evidence, with the height each was first committed at,
and an explicit narrated zero rather than a missing line (a missing line and a zero are the same
character to a scraper). Gated by `cmd/silt/chainstatus_slashset_test.go`, driven RED against the
reader without it and green after.

*IT IS ASSERTED OVER TWO SETS, BECAUSE THEY ANSWER DIFFERENT QUESTIONS.* The suite collects both
from every seat that ever held a chain, before each is torn down:

| set | what it answers | verdict |
|---|---|---|
| NARRATED — every identity any seat DECIDED to slash, from its own journal | item 9's clause at this suite's tier: a node that slashes an honest peer has violated it whether or not the proof reached a block | **PASS** — across 9 seats, exactly ONE identity was slashed and it is the equivocator's |
| COMMITTED — the identities the HISTORY carries | the replicated, objective eviction (F2) — every replica evicting in lockstep rather than one local ledger | **PASS**, on the equivocation island — see below |

*NEITHER CAN PASS VACUOUSLY.* The narrated complement is paired with scenario 1, which guarantees
the set is non-empty — an empty slash set satisfies "no honest node was slashed" perfectly and
proves nothing. The committed complement is paired with the culprit's own presence and, when the
set is empty, with whether the head ADVANCED after the detection: a chain that committed nothing had
no block to carry the proof, and calling that a dropped slash would be a verdict naming the wrong
cause. The rule asserted is stricter than the clause — exactly one identity may be slashed anywhere
in the run — because silt slashes PROVEN EQUIVOCATION and nothing else, so a slash landing on the
forger or the low-bond proposer would be an attributability failure too.

*THE COMMITTED HALF IS RED, AND IT IS A PREMISE FAILURE RATHER THAN A PRODUCT ONE — which took a
second drive to establish, because the first one could not tell the two apart.* Detection only
QUEUES the proof (`core/node` `pendingSlashes`) for on-chain recording, whose own comment says it
exists "so the OBJECTIVE set evicts the culprit in lockstep on every replica (F2), not just this
local ledger". An absent committed slash therefore means one of two very different things, and only
the head tells them apart. The gate now records it: **`the on-chain slash did not land within 90s on
equiv-yz; head 2 → 2, advanced=no`.**

The equivocation chain does not commit another block after the double-sign. So no block ever existed
to carry the proof, and the replicated eviction has never been exercised — not here, and, since this
is the only topology that produces an equivocation, not anywhere.

*THE FIRST ATTRIBUTION WAS WRONG, AND THE PROBE THAT CORRECTED IT FOUND A PRODUCT DEFECT.* This
sheet said the cause was the quorum floor: three anchors, the double-signer is one, evicting it
locally leaves two of three and the chain stops — "the eviction the chain would need to record is
what stops the chain that would record it." That reads well and it is FALSE. It was inferred from
the topology rather than measured, and the measurement says otherwise.

*WHAT THE JOURNALS ACTUALLY SHOW,* driven on the equivocation topology alone 2026-09-19: both
honest nodes committed their blocks, slashed the culprit, and then committed nothing for the rest of
a 180 s window — with NO `bond-reg drain blocked at own sign slot`, no `round-change: advancing`, and
no `stalled-at-boundary`. A chain short of quorum ladders rounds and says so. This one said nothing,
because it was not wedged and was not short of quorum. **It was QUIESCENT** — idle, by design, while
holding an eviction it had already proven.

*THE DEFECT: A QUEUED EQUIVOCATION PROOF WAS NOT WORK.* `slashEquivocators` queues the proof so the
objective set evicts the culprit in lockstep on every replica (F2) rather than in one local ledger.
But the proof rides only a block someone proposes, and nothing armed a proposal BECAUSE one was
held: the drain sweep's quiescence rule counted pending bond registrations, pending entries, an own
renewal due and foldable issuer keys, and the rank-walk takeover branch counted entries. Neither
counted a slash. On a busy chain unrelated traffic carried the proof along soon enough to hide it;
on an idle one it never landed — and idle is the case that matters, because an attacker equivocates
and then goes quiet. Fixed in both branches, each ablated separately because one test cannot see
both, and a slash-only block is not empty so this cannot arm a proposal with nothing to carry.

*THE COMMITTED HALF IS NOW DRIVEN, AND THE ISLAND IS WHERE IT BELONGS.* `integration/redteam`'s
equivocation seats run `-objective=false`, and the drain that carries a queued proof is gated on
`Objective()` in its first line — so there the proof can never ride, fix or no fix. That is a
property of the DRILL, not the product, and the right answer was not to convert a suite whose other
scenarios depend on the subjective posture. The cloud sheet already runs a contained equivocation
ISLAND that is `-objective` with four anchors and its own genesis, and it already asserted the slash
FIRED. It now also asserts what the history COMMITTED.

*DRIVEN 2026-09-19, LOCAL, and the numbers are the claim:*

```
accountability FIRED AND COMMITTED: an island anchor double-signed, an honest anchor
slashed it (slashed equivocator 6c5f1115… (double-signed at height 2)), and the HISTORY
carries the eviction at head 6 with NOBODY else in the committed slash set
```

Read after the flow, every island replica agrees — **head 9, exactly 1 identity in the committed
slash set, on all four seats including the equivocator's own**. That is the lockstep the clause
asks for: not one local ledger's opinion but the same committed history everywhere. The chain also
kept committing past the permanent eviction (3 of 4 anchors clearing the ⌊A/2⌋+1 floor), which is
what makes the proof landable at all.

*IT IS ALSO THE FIRST END-TO-END CONFIRMATION OF THE QUIESCENCE FIX,* over real daemons rather than
the sim: the proof rides a block because a queued slash now arms the drain. The three verdicts the
assertion can return are deliberately distinct — an identity other than the equivocator is an
unconditional fail; the equivocator alone is the pass; an empty set names WHICH half to look at,
because a non-advancing head means the chain is quiescent and the proof is not arming a proposal
while an advancing one means blocks are being proposed and the proof is not riding them. That
distinction is exactly the one this sheet got wrong the first time.

*SO THE SUITE IS RED FOR A REASON WORTH BEING RED FOR.* "Skipped", "gap" and "not run" are all
failures here, and an undriven half of the accountability claim is exactly that. What is NOT claimed
is that the product drops a queued slash — this run cannot see that far, and the verdict says so in
those words rather than implying a defect it did not measure.

**10. A prover without the bytes fails the audit and is paid nothing.** ⚠ *all three defeats and both
work bounds are CLOSED at unit and e2e, and the claim path's shipped-default exposure with them; what
remains is one COUNT bound, named, and a cloud run this has not been through*
Three defeats: the
care link printed on every publish is the storage-proof verification key; a data-less
identity passes by relaying the challenge to a real holder; the inclusion proof is checked
against a root the prover supplies. Plus work bounds on the unsigned repair claim and the
challenge frame, neither of which is rate-limited.
*unit → integration → e2e under impairment → field.* Largest code item in scope.

*THE DIAGNOSIS WAS ALREADY DONE AND THE FIXES WERE NOT.* Every one of the five is carried by a
`_PINNED_DEFECT` gate that asserts the BROKEN behaviour — green while the defect is live, red when
it is fixed — with a vacuity guard proving each pin can see its own remediation. Nothing here had to
be discovered; what follows is three of them redeemed on 2026-09-18, each by the route the pins
prescribe: the fix reddens the pin, and the pin is replaced by the positive assertion of the rule
that reddened it, in the same change. No test was deleted.

| defect | gate | state |
|---|---|---|
| the inclusion proof is checked against a root the PROVER supplies | GATE 5a → `TestForeignRootedProofIsRefused` | **CLOSED** |
| the unsigned repair claim's fetch is not budgeted (arm b) | GATE 1 (b) → `rc1SurvivorBudget` | **CLOSED** |
| the challenge frame is not rate-limited | `TestPorChallengeRateLimitPerChallenger`, `TestRefusedPorChallengeSendsNoReply` | **CLOSED** |
| the unsigned repair claim is judged by a node that cannot pay (arm a, the shipped-default half) | `TestRepairClaimBuysNoWorkWithoutTheEconomy` | **CLOSED** — 4,719,978 B → 0 |
| the unsigned repair claim is unbounded PER SENDER (arm a, the count) | `TestSurvivorFetchIsUnboundedPerSender_PINNED_DEFECT` | open — remedy REFUTED |
| the care link IS the storage-proof verification key | `TestCareLinkHolderWithZeroBytesFailsTheAudit` | **CLOSED** — the key left the scheme |
| a data-less identity outsources the challenge to a real holder | `TestChallengeOutsourcingIsRefusedByTheProver` | **CLOSED** — Passed 1 → 0, mint 1000 → slashed |

*THE ROOT WAS NEVER MISSING, ONLY UNUSED — which is the first thing build-immutable #6 says to
check.* `verifyStorageProof` read `p.Root` from the RESPONSE, and with `{Index:0, Total:1, Path:nil}`
`manifest.VerifyProof` reduces to `leafHash(leaf)==root`, so anyone knowing a chunk id satisfied the
Merkle leg with zero knowledge. `auditEntry` already computed `root := m.Root()` and already passed
it to `colKey`; it simply never reached the verify. The two questions are now two functions, because
one name for both is what let them be conflated: `verifyStorageProofAgainst` (root supplied by the
VERIFIER — the audit and the repair-claim judge) and `storageProofSelfConsistent` (store-time
acceptance, where a receiver holds no independent root for content it was not asked to care for,
documented as NOT a binding check).

*AND IT DECOUPLED A SECOND PIN, WHICH IS THE PART WORTH KEEPING IN THE RECORD.* The care-link pin
went red on the root binding. Its own text predicted exactly that and gave the disambiguation — *"5a
red with isolation=false is the leg-1 fix; 5a still accepting the self-rooted proof in isolation
means the key distribution is what moved."* 5a refuses in isolation, so the red was the leg-1 fix
leaking across, not a key-distribution remediation. That pin's forger had been carrying a
SELF-ROOTED proof, so it was riding the tautology next door. It now carries the honest inclusion
proof — which any care-link holder can derive from the layout it is entitled to read — and is green
again on its own merits. The care-link break is untouched and still live: `Passed=1`, 1000 credit
minted to a prover holding zero bytes. Two pins that were coupled are now independent.

*THE JUDGE'S FETCH IS BUDGETED TO WHAT THE VERDICT READS.* `survivorRefs` is the complement of ONE
position over the stripe, so walking it to the end cost the judge n−1 = 15 shard fetches at the
shipped k=10/n=16 for a `VerifyByRecompute` that consumes 10 — and a repair claim is unsigned and
free to send, so every shard beyond what the verdict reads is pure amplification. Measured 15 → 10.
The exit is on SUCCESSFUL fetches, not a shorter ref list: trimming refs to k would turn "k of these
happen to be unreachable" into a failure the judge could have avoided by asking one more holder.
Repair passes a nil budget deliberately — its `usedDomains` is a census the re-seed reads, and a
partial one would place a rebuilt shard into a domain the walk never looked at.

*THE CHALLENGE-FRAME REFUSAL IS A DROP, AND THAT IS THE LOAD-BEARING HALF.* Answering `MsgChallenge`
reads the whole shard back and aggregates it — 8.3 ms and 8,643 field multiplications over a 256 KiB
shard, measured — on the single serialized loop, from an unsigned frame with no standing
requirement. Every other expensive inbound kind on the node already had a per-sender window budget;
this one had none. The budget is derived from the loop share it concedes (128 × 8.3 ms ≈ 3.5% of a
30 s window per challenger) and the honest auditor clears it because its sweep is SERIALIZED. But a
rate limit on an audit response is only safe if refusing cannot be mistaken for failing: the auditor
grades `Found=false` as a prover that could not produce a proof, the same verdict as a liar, so a
gate that REPLIED would hand any peer a way to SLASH AN HONEST HOLDER by first spending its budget.
`auditLeaf` counts an answer only when `err == nil`, so the refusal emits nothing and a dropped
challenge is not a failed audit — it is no audit. Both ablations are driven red, including that one.

*THE RULING (2026-09-19): ALL THREE WERE BUILT, NOT DISCLOSED — AND ALL THREE ARE NOW CLOSED.* The
prior reading — that this item reaches the date at three of five with the other two disclosed — was
optimising for what fits in the window rather than for what a release candidate is. The
done-condition is "a prover without the bytes fails the audit and is paid nothing", and an audit a
caretaker can forge does not survive an outside adversary whatever the sheet says about it. What
stays open is the per-sender COUNT bound on the repair claim, which is a work bound and not one of
the three defeats; it is named in its own paragraph below rather than folded into this one.

*THE MEASUREMENTS, TAKEN 2026-09-19 rather than recalled.* Each pin was driven and its own numbers
read off the run:

| defect | measured |
|---|---|
| care-link forgery (the pin, then `TestCareLinkHolderWithZeroBytesFailsTheAudit`) | 0 bytes read, 67 field mults against the honest prover's 8,643 — **129× less work, 17× faster** — graded `Passed:1`, mints 1000 credit — **CLOSED 2026-09-19**, now `Passed:0 Failed:1` and slashed |
| challenge proxy (`TestChallengeOutsourcingIsRefusedByTheProver`, then the pin) | a data-less identity has a real holder answer its own identity-bound challenge; graded `Passed:1`, mints 1000 credit — **CLOSED 2026-09-19**, now `Passed:0 Failed:1` and the proxy is slashed |
| unbounded claim, arm (a) (`TestSurvivorFetchIsUnboundedPerSender_PINNED_DEFECT`) | a **110-byte** claim costs the judge **4,719,978 B** over 10 distinct chunks — **~42,900×** — and the sender pays nothing; 8 claims cost 8× that, linearly |

*ARM (a) IS LIVE ON THE SHIPPED DEFAULT, which this sheet did not say and should have.*
`handleRepairClaim` runs the registry lookup, the manifest fetch and the survivor walk BEFORE any
economy check — `cfg.RepairEconomy` is tested only at settlement. So the amplification does not need
`-economy`; it applies to every node that is a caretaker of the root. That makes it the only one of
the three whose exposure is not behind a double opt-in, and the cheapest to narrow: the gate exists,
it is in the wrong place. The per-sender BOUND stays open — its remedy is refuted on its own
precondition, since claim emission binds an empty reply callback and a refused claim is lost forever
— and narrowing the work is not the same as bounding the count, which the fix must say plainly.

*THE SHIPPED-DEFAULT HALF IS CLOSED, 2026-09-19, and the fix is smaller than "narrow the work"
because the code already decided the question twice.* `announceRepairQuorum` and `emitRepairClaim`
are BOTH already gated on `cfg.RepairEconomy`. A node with the economy off never plants itself under
`careKey(root)`, so no honest paramedic can discover it as a caretaker-judge, and it never emits a
claim of its own. Its legitimate inbound claim traffic is therefore ZERO BY CONSTRUCTION, and the
whole 4,719,978 B was attacker-directed with no honest case underneath it. This was the missing
THIRD gate in a set of two, not a new policy — and the symmetry it rests on is asserted by its own
test (`TestRepairClaimEconomyGateIsWhereTheOtherTwoAre`), so a later change that ungates announce or
emit makes the argument's collapse visible rather than silent.

| arm | shards fetched | distinct chunks | bytes | vote |
|---|---|---|---|---|
| economy OFF — the shipped default | **0** | **0** | **0** | 1 |
| economy ON — positive control AND ablation | 9 | 10 | **4,719,978** | 1 |

*THE CONTROL IS ALSO THE ABLATION, deliberately.* The gate's only effect is to make OFF differ from
ON, so the economy-ON arm is simultaneously the proof that a participating node still judges and the
pre-gate behaviour the gate removed. The test FAILS when that arm is cheap, because a gate that is
never reached passes for free — the same shape every other gate on this list carries.

*WHAT IT DOES NOT CLOSE, and the fix says so in the code rather than reading as if it had.* The
per-sender bound. A node RUNNING the economy still pays the full cost per claim, from an unsigned
frame, without limit. `TestSurvivorFetchIsUnboundedPerSender_PINNED_DEFECT` is still GREEN at the
same 4,719,978 B, which is the correct outcome and not an oversight: it pins the count, and this
change moved the work.

*WHAT IT GIVES UP, named rather than discovered later.* An economy-off node no longer slashes a
claimant that names a shard id the manifest does not commit at that position. That punishment was a
reduction on a local ledger the node never pays out of, delivered by a judge nobody could discover,
and reported in a vote the emitting paramedic discards. The refusal is still ANSWERED rather than
dropped, so a caller can tell a refusing judge from an unreachable one.

*THE OUTSOURCING DEFEAT IS TWO RESIDUALS, AND THE SHEET HAD MERGED THEM.* "The literature closes it
with sealing or with latency" describes the WILLING COLLUDER — a holder running patched software
that computes under any seed it is handed. That one is a Douceur limit, stays disclosed, and is why
PoR grants no standing at all. The pinned defect is narrower and is not that: an honest holder is
used as an ORACLE because the challenge seed travels on the wire in `msg.PorSeed` and
`answerChallenge(msg)` copies it blindly, taking no prover parameter — while `answerBondChallenge`
already takes `from` and `Node.handle` already has it in scope. Sending the challenge BASE and having
the holder derive `porProverSeed(base, self)` leaves nothing to proxy. Structural, no latency gate,
and precedented in this tree.

*BUILT 2026-09-19, AND THE PRECEDENT THIS SHEET CITED WOULD HAVE BUILT THE WRONG THING.* The
paragraph above — and the pin's own fix-case — pointed at `answerBondChallenge` already taking
`from`. `from` is the FORWARDER. Binding the challenge to it would have authorised exactly the
attack, and the guidance was followed only as far as checking it. The identity that must reach the
prover is SELF. What shipped: the auditor sends the unbound base beside the derived seed, and the
prover folds in its OWN id and compares. It is a CHECK rather than a re-derivation, which is what
lets one new field serve both callers — the audit sweep binds under `porProverSeed`, the repair
claim's retrievability leg under `repairproof.RepairChallengeSeed`, and the prover tests its own
identity under both domains rather than being told which caller sent the frame.

| | before | after |
|---|---|---|
| the data-less proxy's grade | `Passed:1 Failed:0` | `Passed:0 Failed:1` |
| what it earned | **1000 credit minted** | slashed; the assertion is `minted <= 0` |
| the holder's own record | — | `ProxiedChallengesRefused=1`, so the refusal is attributed rather than inferred |

*IT IS ADDITIVE ON THE WIRE, AND THE LIMIT IS STATED.* `PorBase` is optional, on a fresh field
number — 31, not the retired 11, because reusing a retired slot lets an old peer's value decode as
the new one. The derived seed is unchanged, so an old prover behaves and grades exactly as before,
and a new prover facing an old auditor has no base to check and answers verbatim. All four
combinations are driven. The defeat therefore closes for every pair whose PROVER is current, which
is as far as a change on this side can reach: it cannot fix a peer's software.

*THE OVER-REJECTION ARM IS THE ONE THAT MATTERED, and catching it needed the fixture fixed first.*
Refusing every challenge would satisfy the pin's fix-case perfectly while failing every honest audit
on the network. The control that catches it is inside the converted gate — A answers its OWN
identity-bound challenge, genuinely computing, and B still fails — and it could not have caught
anything before: the old relay arm used a FABRICATED holder id unrelated to the node's own identity,
so it would have passed however the prover behaved. Three ablations are driven, and the sender's
half has its own arm because dropping `PorBase` at the auditor leaves every prover-side test green
while the defence does nothing.

*THE CARE-LINK BREAK IS CLOSED, 2026-09-19, BY REMOVING THE KEY RATHER THAN MOVING IT.* All three
audit legs were satisfiable by a party holding the layout key and no bytes: the inclusion proof is
derivable from the layout it is entitled to read, the block count is public, and the PoR equation is
solvable with the key. There was no fix inside the primitive, because the party that had to VERIFY
was the party that could FORGE — Shacham-Waters private verification assumes the key is unknown to
the prover, and here it was a published capability. What shipped is option B: the auditor names
sampled LEAVES, the prover returns their bytes with Merkle paths, and both are checked against a
per-shard root the publisher commits in the sealed layout. There is no key in the scheme, so there
is no party holding one.

| | before | after |
|---|---|---|
| the zero-byte care-link holder's grade | `Passed:1 Failed:0` | `Passed:0 Failed:1` |
| what it earned | **1000 credit minted** | slashed; the assertion is `minted <= 0` |
| what the forger is handed | the layout key | the layout key, the object root, the COMMITTED shard root, the chunk id, its honest inclusion proof and the exact geometry — a strictly stronger adversary |
| what it must produce | a solvable equation | a second preimage of a Merkle leaf |

*THIS PIN'S FIX-CASE WAS RIGHT, AND THE ONE NEXT DOOR WAS NOT — BOTH WERE CHECKED RATHER THAN
FOLLOWED.* The pin predicted two shapes — "the PoR key no longer rides the care link, or the grade no longer accepts a
proof the key holder can solve for" — and the first is what landed. It also warned that the pin was
COUPLED to leg 1 and gave the disambiguation; that coupling was already resolved when the root
binding landed and the forger was moved onto the honest inclusion proof, so this RED is the key
distribution and nothing else. That is the opposite outcome to D2's pin the same day, whose fix-case
named `from` where the answer was `self` and would have authorised the attack it pinned; the
practice that caught one and confirmed the other is the same practice. Its instruction to re-read a
research certification could not be followed: that document went with the written record deleted on 2026-09-13, so the option space was
re-derived from the mechanism instead, in `core/por/production_cost_floor_test.go`.

*THE GEOMETRY IS MEASURED, NOT PICKED, and it is the second half of what #8 asks.* The production
measurement chose the SCHEME; it left the scheme's two parameters open, and those are not free in
the way a tuning knob is free — the leaf size moves production cost and audit wire cost in OPPOSITE
directions. `core/por/spotcheck_geometry_test.go` sweeps it and two assertions hold the shipped
constants to the two properties that justified the choice.

| leaf | leaves/shard | path | bytes per sample | produce one commitment |
|---|---|---|---|---|
| 64 B | 4,097 | 13 | 480 B | 1.52 ms |
| **128 B (shipped)** | **2,049** | **12** | **512 B** | **653 µs** |
| 512 B | 513 | 10 | 832 B | 328 µs |
| 3,968 B | 67 | 7 | 4,192 B | 149 µs |

At the shipped leaf and 8 samples an audit moves **4,096 B per shard against the aggregate scheme's
4,128 B** — so the wire cost did not move — and the commitment costs **653 µs against 6.2 ms to
produce**, a 9.5x saving on the floor box. Detection: a holder that dropped the shard is caught with
certainty, half of it 99.61% of the time, a quarter 89.99%, a tenth 56.95%.

*THE SHEET'S OWN OBJECTION IS RETIRED, AND IT WAS ABOUT A NUMBER RATHER THAN A SCHEME.* "On the
measured 262,160 B shard a 67-sample challenge over 4 KiB blocks is the whole shard" was true and is
no longer the geometry: 67 was `porSampleCount` carried over from an aggregate scheme, where a large
sample was free because the response was size-independent. A spot check picks its sample count from
the detection it wants, and its leaf size from where the Merkle path stops dominating.

*WHAT IT COSTS, STATED RATHER THAN BURIED.* The audit's response is no longer independent of shard
size. It is bytes now, not arithmetic, and a much larger sample count would not stay under the
aggregate it replaced. Three costs moved the other way and are worth the same sentence.

| | aggregate scheme | hash-only spot check |
|---|---|---|
| producing one shard's commitment | 6.2 ms | **653 µs** |
| answering one challenge | 8.3 ms | **1.44 ms** |
| audit response per shard | 4,128 B | **4,096 B** |
| per-shard state on every host | ~2,144 B of authenticators | **0 B** |
| per-shard state in the object | 0 B | 32 B, in the sealed layout, once rather than per replica |

The host-side removal is the same one that shrank a full storage proof from ~5.4 KB to under 700 B.
The prover figure is measured with a PREPARED tree and the measurement asserts that it is the path
taken: drawing each sample's Merkle proof standalone costs 6.3 ms, because `manifest.Prove`
recomputes subtree hashes over half the leaves on every call. It matters beyond tidiness — the
per-challenger DoS budget in `bondaudit.go` was sized from the 8.3 ms figure, and at 1.44 ms the same
128-proof budget concedes ~0.6% of a 30 s window instead of ~3.5%. The number is left where it is
rather than raised, in the safe direction.

*A CHEATER MUST KEEP THE HASHES, AND THAT IS A FLOOR ON WHAT CHEATING SAVES.* The holder that simply
loses a leaf fails every challenge, because the sibling hash on every other leaf's path is computed
over the part it lost. The holder that PAYS keeps the 32-byte hash of each leaf it drops, so it can
still answer for everything it kept. At a 128-byte leaf that costs it a quarter of the shard: a
cheater that dropped every byte and kept the list still stores 25% of what it claims, and is caught
on the first sample with certainty. Both arms are driven deterministically in the gate — a dropped
leaf that IS sampled fails every round, one that is NOT sampled passes every round — rather than
waiting on a 1-in-2,049 draw.

*AND ONE THING IT DELIBERATELY DOES NOT CLOSE.* A shard is public: any node may fetch it and then
answer an audit over it. Retrievability proves possession NOW, never exclusive provision, and that
residual is unchanged — it is why a passing audit mints spendable credit and no consensus standing
at all. The identity binding that makes one prover's answer useless to another lives in the SEED and
is untouched by the scheme change.

*WHAT TIER THIS IS PROVEN AT, SAID PLAINLY RATHER THAN LEFT TO BE ASSUMED.* **UNIT AND E2E.** The
care-link close is driven at unit by the composed three-leg grade over the product's own derivations,
with a no-over-rejection control and a detection control at both ends. E2E is the tier that matters
here, because the scheme changes the publisher, the wire, the on-disk proof sidecar, repair and the
bounty judge, and a unit suite sees none of those seams: the full `./e2e` package passes uncached in
455 s, with `TestRepairBountyPaysOnTheWire` (78 s) driving publish → placement → proof sidecar →
repair claim → identity-bound retrievability challenge → bounty over real sockets, and
`TestPublishCommitFetchOverTCP`, `TestSwarmHoldersReportsPerColumnPlacement` and
`TestUIFetchMakesTheConsumerAProvider` confirmed individually. `-short` was NOT used; it skips 39 of
the 41.

*AND THE TWO TIERS THAT ARE NOT DRIVEN, NAMED RATHER THAN IMPLIED.* E2E UNDER NETWORK IMPAIRMENT and
FIELD. Build-immutable #1 wants a skipped tier stated with a reason: the reason is sequencing, and
the next cloud run at HEAD is what converts this row from a local close into a held one.

*AND THE IMPAIRMENT RISK THIS SHEET REACHED FOR IS NOT THERE, WHICH IS ONLY KNOWN BECAUSE IT WAS
MEASURED.* The reading was that a sampled response is several openings the auditor must receive all
of, where the aggregate was one fixed-size value — so a lossy path would bite the new shape harder.
It does not. The openings ride in ONE `MsgChallengeReply`, and encoded through the real codec that
frame is **4,344 B against the aggregate shape's 4,591 B**: one frame either way, and the new one is
SMALLER. A frame arrives or it does not, exactly as before. What actually grew on the wire is the
MANIFEST, by 34 B per shard, which a fetcher pays once per object and not per audit. Stating a
mechanism this sheet had not measured would have been the guess-in-a-lab-coat build-immutable #7
names; the number above is what replaced it.

*ONE AMPLIFICATION THE NEW SHAPE INTRODUCES, FOUND AND CLOSED INSIDE THE CHANGE.* The aggregate
response was a fixed size whatever the challenge asked for; a sampled one is not. `PorCount` is
attacker-controlled, so an unclamped prover asked for every leaf would answer a 110-byte frame with
a ~1 MB reply and one Merkle proof per leaf. The prover now answers at most the protocol's own
sample count regardless of what the frame says — the honest auditor sends exactly that many, so
nothing legitimate narrows — and the LEAF WIDTH is read from the proof that arrived with the shard
rather than from the challenge, for the same reason: a challenger that could name a one-byte leaf
could make a small frame cost a tree over a quarter of a million leaves.

*AN OBJECT WITH NO COMMITMENT IS NOT AUDITABLE, AND THE SWEEP SAYS SO.* The auditor needs a number of
its own; taking the shard root from a response would be the tautology the root binding closed next
door. An object whose layout carries no shard roots is therefore counted as UNAUDITED and graded
neither way, in a field of its own beside the verdicts rather than folded into them — nobody lied and
nobody was checked, and a report that showed either as the other would be the dashboard that flatters
(S5). The repair-bounty judge runs the same rule and DENIES rather than paying an unchecked claim.

**11. Publishing is unlinkable and no surveillance artifact exists.** ❓ *NO VERDICT RECORDED — and one
clause names a live defect*
A matching score
against chance; seize every disk and emitted byte after a fetch-heavy run and find no
(fetcher, content) pair; published metadata no longer leaks the exact plaintext byte count,
which today makes the padding defence a no-op.
*integration → e2e → field.*

*WHAT EXISTS, AND WHAT IT IS NOT.* `integration/privacy` passes and `flow_publisher_unlinkability`
runs in the cloud, so there is evidence in the neighbourhood. Neither was driven against THIS item's
clauses: no matching score against chance has been computed, and no seize-every-disk-and-emitted-byte
sweep has been run. A passing suite nearby is not a verdict on the claim, and recording it as one is
the exact move this list refuses elsewhere.

*THE THIRD CLAUSE IS AN OPEN PRODUCT DEFECT STATED INSIDE A DONE-CONDITION,* which is a shape worth
naming: "published metadata no longer leaks the exact plaintext byte count, WHICH TODAY MAKES THE
PADDING DEFENCE A NO-OP". That is not a thing to test, it is a thing to fix, and nothing currently
drives it either way.

**12. Takedown bites, cannot go global, and is provable.** ❓ *NO VERDICT RECORDED — the strongest of the
unattributed six*
Honouring operators stop serving
and others keep serving; no accepted operation removes more than one named root or works by
identity; every honoured removal carries inclusion and consistency proofs.
*integration → e2e → field.*

*TWO OF THE THREE CLAUSES HAVE REAL EVIDENCE, UNATTRIBUTED.* `integration/takedown` PASSES at HEAD —
after two product defects were found and fixed (a restart purge that swept a chunk index the async
reload had not filled, so denied bytes stayed on disk; and a cold `AnnounceHeld` that re-advertised
them to the DHT under bare ids). `8-takedown` in the cloud shows one operator stopping while another
keeps serving bit-perfect, which is the per-operator, non-global half.

*THE THIRD CLAUSE IS UNDRIVEN:* every honoured removal carrying INCLUSION AND CONSISTENCY PROOFS.
That is the half that makes non-globality PROVABLE rather than merely true on the day, and it is the
half `VISION.md` leans on ("silt can *prove* it never flipped a global switch"). Nothing reads those
proofs today.

**13. Bit-perfect or an explicit failure, and crash recovery needs no human.** ❓ *NO VERDICT RECORDED —
the storage half has evidence, the two named clauses do not*
An unplaceable
publish names what could not be placed, returns no link, leaves no registry entry, and still
succeeds on retry. A validator killed mid-consensus re-pins from its own last finalized
checkpoint and never contradicts a signature it made before the crash.
*e2e under impairment → field.*

*WHAT EXISTS:* `integration/chaos` PASSES (SIGKILL every holder, restart, re-announce, cold-fetch
bit-perfect), and the cloud's `7-restart-content` and `chaos-fetch` confirm bit-perfect retrieval
after a hard crash on real hardware.

*BOTH CLAUSES THIS ITEM ACTUALLY NAMES ARE UNDRIVEN.* Nobody has driven an unplaceable publish and
asserted the S3 shape — names what could not be placed, returns NO link, leaves NO registry entry,
and still succeeds on retry. And nobody has killed a validator mid-consensus and asserted it re-pins
from its own last finalized checkpoint without contradicting a signature it made before the crash.
The second is a consensus-safety property (a validator never signs twice at a height, and that memory
survives restart) and it is asserted nowhere at this tier.

**14. The floor box holds, and the chain prunes.** ✅ *done*
A validator on the declared floor spec — one core, 2 GiB, 10 GiB of disk — validates against
witnesses without holding the tree, stays under its memory ceiling on adversarial input, stalls
rather than accepts when no witness provider is reachable, and prunes at depth from persisted state.
*unit → e2e under impairment → field.*

*State:* the claim has four legs and all four are driven. The `floor` suite drives them; it enforces
the spec with a cgroup rather than a flag, because `-mem-limit` is a SOFT ceiling and a soft ceiling
cannot answer "does this survive on a 2 GiB box". The witness half runs as a SECOND box on the same
spec in the witness-validating posture (`silt daemon -floor-box -witness-from=...`), with a
no-provider control beside it. The memory ceiling is measured on four seats in three topologies: the
`floor` box under honest load at depth, and three honest seats inside the adversarial topologies.

- **Under its memory ceiling — DEMONSTRATED, on honest load at depth AND on adversarial input.** On
  every seat below, `memory.max` is 2 GiB with `memory.swap.max` 0 and `nproc` 1, asserted from
  inside that container before any number from it is believed, and the peak is the cgroup's own
  high-water mark rather than a sample.

  *Honest load, at depth — `floor`.* Peak 260.8 MiB, 12% of the ceiling, 1.75 GiB of headroom. The
  box committed to height 17 on the same head hash as two ordinary validators, so it was
  participating and not merely surviving, and it shed below the retention horizon on the way. This
  figure moves between runs like the adversarial ones do — 209.6 to 291.1 MiB, 10% to 14%, across
  the runs on this hardware — so it is the order that is the claim, not the digit.

  *Adversarial input — three more honest seats on the same spec, inside the topologies where the
  adversaries already live.*

  | seat | what was fed to it | peak | of 2 GiB |
  |---|---|---|---|
  | `redteam` `equiv-x` | a double-signer's two conflicting chains, synced, reconciled, equivocator slashed | 29.9 MiB | 1% |
  | `redteam` `h3` | a forged-proposer-signature block and an under-bonded proposal, both refused, head never moving | 42.5 MiB | 2% |
  | `sybil` `a1` | a bonded Sybil farm of 4 challenging it every second, 3 of whose registrations it verified and committed | 87.5 MiB | 4% |
  | `sybil` `a1` | the same, with the farm at 8 | 99.0 MiB | 4% |

  The pin is the same three lines the `floor` service uses — `cpuset`, and `mem_limit` equal to
  `memswap_limit` so there is no swap to escape into. The seats were not chosen for convenience:
  each is a seat the adversaries already point at AND that the suite already asserts from, so the
  attack lands where the assertions are watching.

  *THE NUMBERS ARE NOISY AT THIS SCALE, and the ranges belong next to them* — a single figure read
  as a fixed cost would make the next run look like a regression. Across four `redteam` runs
  `equiv-x` fell in 29.9–39.2 MiB and `h3` in 30.6–42.5 MiB; the table cites the last run of each.
  What is stable is the ORDER: every adversarial seat sits at 1–4% of the ceiling, an order of
  magnitude clear of it, and the whole spread is smaller than the gap to the honest-load figure.
  The one signal that IS a slope rather than noise is the farm size: doubling it moved the anchor's
  peak 11.5 MiB, which is the shape a real regression would show up in.

  *NO SEAT PASSES VACUOUSLY, and both controls are driven rather than asserted.* Two things could
  make a small peak worthless, and each has its own control in the same run:
  - *The limits might not have bound*, and then the peak measures an ordinary box. The same guard is
    pointed at an UNPINNED seat in the same run — `h2` in `redteam`, `a2` in `sybil` — and must
    report it UNBOUND. It does. If it did not, the guard would be reading something other than a
    seat's own limits and every BOUND verdict in the run would be free.
  - *The attack might not have landed*, and then the seat was idle. Each seat's ceiling verdict is
    gated on the drill that proves the adversary reached it and was denied: the slash line for
    `equiv-x`; both refusals plus the unchanged-head cross-check for `h3`; a committed block
    carrying bond registrations for `a1`. A peak from a seat whose drill failed prints NOT CREDITED
    and FAILS the suite, because an uncredited number and a measured one are not interchangeable.

  *WHAT THE ADVERSARIAL NUMBERS DO NOT COVER, said plainly.* Both adversarial topologies run SHALLOW
  chains — `equiv-x` judges at height 1, and `h3`'s head is asserted never to move at all. Those
  three seats therefore measure the ADVERSARIAL PATHS, not adversarial input at depth; the depth
  figure is `floor`'s 209.6 MiB at height 18, and that load is honest. No single run combines both,
  and the leg rests on the composition. It is worth naming which number a regression would move
  first: the closest to its ceiling is the honest-load one, at 12%.
- **Prunes at depth from persisted state — DEMONSTRATED.** Three blocks shed their heavy bond proofs
  at height 17, which is what the retention rule predicts: the node keeps
  `max(2·BondTTL, BondRegHeadWindow+4) = 12` full-proof blocks below its finalized head and then
  epoch-aligns down. A restart onto the same volume reloaded the already-pruned store, still
  reported the shed, and committed again to height 18 — so the prune is a property of persisted
  state and did not trade an OOM for a stall.
- **Stalls rather than accepts when no witness provider is reachable — DEMONSTRATED.** A box on the
  same genesis, anchored on an operator checkpoint and pointed at a provider that does not exist,
  reached the block above its anchor and stalled: one stall, zero verdicts, zero accepts. Safety does
  not rest on the tier above.
- **Validates against witnesses without holding the tree — DEMONSTRATED.** The box is on the same
  genesis as the validators (it derives the hash from its own configuration; the suite compares the
  two) and holds zero blocks of its own. It reached a VALIDATED verdict at three distinct committed
  heights and stalled on NONE. The control above is what keeps that reading honest: the same binary,
  denied its providers, validates nothing.

*What closed the last leg:* the box could not reproduce a block touching two committed-state classes
at once. The recompute composed slashes, bond registrations and TTL expiry by APPENDING each class's
reconstruction, and each derived its post-set from the anchored pre-state alone — so a block touching
two emitted two fold operations for one committed key. The classes now run once over ONE running
post-state, in the order `apply` runs them (registrations → expiry → slashes), and each digest,
per-member leaf and due-bucket leaf is emitted once from the state the last class leaves. The order
is load-bearing rather than cosmetic: `apply` resets `bondRegHeight` inside the registration loop and
the sweep reads that map afterwards, so a validator renewing on the very height its bond falls due
keeps its standing and its bucket leaf moves once. On this topology that is most blocks.

*What the fix also closed, and it was sharper than a stall.* Measured against the HONEST committed
root the compound block mismatched and the box stalled — safe, and what every shipped ablation
observed. But a real proposer commits the root ITS OWN fold produced, and against that root the box
returned no objection: the duplicate operations carried a byte-identical pre-state value and proof,
so every one of them verified, the fold took whichever was appended last, and the terminal equality
passed by construction. That is box-accept with node-reject, reached with no forgery at all, and the
accepted state contradicted itself — the per-member leaf recorded an eviction the whole-set digest
covering it still counted. It was contained only by the door's accept downgrade. The pins that
recorded it are retired and replaced by the straight assertion that a compound block folds to the
root `apply` commits, with one fold operation per committed key.

*The two risks the adversarial half was expected to run into, and what actually happened.* Neither
bit. Pinning a seat to one core changes the CADENCE of a suite whose budgets were sized without it,
and a floor-spec seat competes for the same two-CPU docker VM as the attackers — so a first run
could time out for a host reason rather than a product one. In the event NO budget was raised:
`redteam` cleared its existing 90 s and 60 s windows on three consecutive runs and `sybil` cleared
its existing drain sweeps, on a 2-CPU / 3.8 GiB guest, with the attackers running beside the pinned
seat. The peaks make the reason plain — the seats that carry the attack run at 1–4% of the ceiling,
so one core is not the constraint at this scale.

*One correction the measurement forced, and it was the harness rather than the product.* The first
version of `sybil`'s load line reported the C2 `nakamoto N bonds` metric as the size of what the
anchor was carrying. That metric is printed on a sweep that can predate the drain block entirely,
so it read 0 identities on a run where a registration demonstrably committed — a number that would
have travelled next to the peak and made it look like a measurement of an idle anchor. The line now
sums the `bond-regs` field of the daemon's own committed-block line, which is what the seat actually
verified and committed.

*And if a pinned seat ever OOMs, that is the finding, not a tuning problem.* Build-immutable 8 says
an unbounded system on a small box is unsafe rather than slow. Both suites read `.State.OOMKilled`
and report the kill as a FINDING to instrument and reduce to a local repro, rather than raising a
limit around it.

*What is not claimed:* that the box PARTICIPATES. Its door maps Accept to a downgrade by design, so
it audits and reports, adopts nothing, and advances no head. Taking that downgrade is **item 20**,
with its own preconditions, and is the owner's call.

**15. The floor box can post the bond its disk allows.** ✅ *done*
*Done:* the plot is sealed to disk block by block and answered by sparse reads, so residency
is the leaves and their tree rather than the plot's size. A 5 GiB plot needs ~247 MiB
against a 1 GiB budget, where it previously needed 6,219 MiB.
*Why it is on this list:* standing is proportional to bonded size. When the largest plot a
node can hold was set by its memory, the biggest bond a small operator could post was a
sixth of the disk they bought, and consensus weight concentrated on larger machines for no
reason anyone chose.
*Evidence:* unit → e2e. A byte-identical-plot test proves the streamed and resident seals
commit the same bond and that a disk-backed plot answers a live challenge that verifies.

**16. Full history fits a volunteer.** ✅ *done*
*Done:* the archival tier keeps every block to genesis and keeps heavy possession proofs for
a bounded window rather than forever. Five years of full history at 100 bonded validators is
~104 GiB, against ~30 TiB unshed.
*Why it is on this list:* a validator republishes a multi-megabyte possession proof every few
minutes to hold its standing, so retained proof volume grows with the number of independent
operators — making a more decentralized network one that fewer parties can archive, and the
deep past the property of whoever can afford terabytes.
*Evidence:* unit → integration.

**17. The economy mints nothing, and the core squeeze is measured.** ❓ *NO VERDICT RECORDED — the
no-minting half is gated elsewhere; the measured half is not taken*
Balance-lane credit
never becomes consensus standing; a colluding pair strictly loses; repair is funded from the
object's own escrow; a false claim is slashed. Plus core-node net margin at two edge
populations against held-constant demand, reported as a signed number.
*unit → integration → e2e.*

*THE NO-MINTING HALF HAS COVER, UNATTRIBUTED:* `integration/economy` passes, `flow_economy_repair`
and `flow_delivery_lane` run in the cloud, and the γ→1/N firewall — credit never becoming standing —
is asserted directly in `core/credit`'s invariant gate, which is the load-bearing one.

*THE CORE SQUEEZE HAS NOT BEEN MEASURED.* "Core-node net margin at two edge populations against
held-constant demand, REPORTED AS A SIGNED NUMBER" is the T-AR measurement — whether the edge tier
that does most of the work stays a net-positive place to do it. A signed number is the entire point
of the clause, and no run produces one. Note the tension the sheet should not hide: the vision
already accepts witness pricing as the fix for the unpriced core-node externality, so this
measurement is the one that would say whether the acceptance was warranted.

**18. Core carries nothing.** ❓ *NO VERDICT RECORDED — and no suite is named for it*
Seize a holder and fail to recover known plaintext, with a
key-holder succeeding on the same objects in the same run; core resolves hashes, never names.
*unit → e2e.*

*THE NAMING HALF IS STRUCTURAL AND THE SEIZURE HALF IS UNDRIVEN.* "Core resolves hashes, never
names" is the Aslan boundary, and it is a property of what the core does not contain rather than
something a run observes. The other half is a drill nobody has built: seize a holder and fail to
recover known plaintext.

*THE PAIRED CONTROL IS WHAT WOULD MAKE IT MEAN ANYTHING,* and the done-condition already names it —
A KEY-HOLDER SUCCEEDING ON THE SAME OBJECTS IN THE SAME RUN. Without that arm, a seizure that
recovers nothing proves only that the seizure was performed badly, which is the vacuity shape this
list rejects everywhere else. Cheap to build, and it is the drill that answers the content-blind
immutable for an outsider.

**19. An un-upgraded node stalls, and the network never updates itself.** ❓ *NO VERDICT RECORDED — its
suite sits in the opt-in tier and was not in the sweep*
A node that cannot
validate a shipped format stops rather than accepting it and says so while staying alive. A
node whose consensus-reaching configuration diverges from what the chain committed refuses to
start. No version-floor advisory below the signing threshold changes anything, and no node
ever replaces its own binary.
*integration → e2e.*

*THE SUITE EXISTS AND WAS NOT RUN.* `integration/upgrade` is catalogued `slow`, so it is opt-in
behind `FULL=1` and was not among the twelve gate suites in the sweep that produced this list's
current local result. Its own catalog line says it reproduces the format-migration FINDING, so it is
not a formality.

*WHY THIS ONE IS SHARPER THAN ITS PLACE ON THE LIST SUGGESTS.* The stall behaviour is what the
frozen-format immutable RESTS ON: "an un-upgraded node **stalls** at the era boundary rather than
accept a format it cannot validate; that stall is the correct safety-first behaviour." Item 20 ships
disclosed because closing its blocker needs a new era. An era boundary whose stall behaviour has
never been driven is a gap directly under the mechanism the release leans on.

**20. The floor box's verdict counts, or it is disclosed that it does not.** ⚠ *SHIPS DISCLOSED — two preconditions closed, one waits on the flip, one waits on a new era*
*Done:* a validator on the floor spec returns the SAME verdict set as a tree-holding node —
Accept for a block a full node accepts, Reject for one it refuses, a stall only where it
genuinely cannot see — established from committed roots and witnesses alone.
*Why it is on this list:* `VISION.md` calls the witness-validating posture **settled** and claims
"**same security as a tree-holding node**, narrower self-sufficiency". A box that audits and
adopts nothing is not that validator. Leaving the verdict withheld is the deviation from canon;
the flip is the alignment. So this ships either green or **disclosed** — it does not ship silent.
*Evidence:* unit → consensus model-check → e2e → field.

*The change is one line,* and it is one line deliberately, so that it is reviewed on its own.
`(*Box).Validate` (`core/chain/floorbox_box_v5.go`) ends:

```go
out, err := ValidateCommitV5(v, &b)
if out == Accept {
    return IndeterminateTrustlessly, ErrRecomputeGated // the flip is not this round
}
```

*It is not a missing accept path.* `ValidateCommitV5` is THE ONE accept composition: the full node
runs it over `liveView` at both write entries and accepts on Accept; the box runs the same function
over `provenView` and throws the Accept away. `StateView` is sealed, so there is no third door. A
VALIDATED verdict in the field suite means the composition already reached accept and this line
suppressed it.

*It is the owner's call, not a builder's,* under the frozen-format immutable: it changes which
nodes may say yes to a block, so the fleet would hold two classes of validator whose accept sets
must be provably identical. That is a validity-rule claim at the "deliberate, reviewed consensus"
bar, not a refactor.

*Preconditions — the flip is not sound until each is closed:*

1. ~~**A bound on committed-set membership.**~~ **— CLOSED for bonded and qualified; named for
   slashed.** The bound needed no new mechanism: two shipped validity rules compose into one.
   `RegCap` caps registrations per block after the same-id fold, fresh and renewal together, and
   the TTL sweep evicts every id whose latest registration is older than `BondTTLBlocks`. A bonded
   id must therefore have registered inside the last `ttl` blocks, each admitting at most `RegCap`
   distinct ids, so `|bonded| <= RegCap * (BondTTLBlocks + 1)`, and `qualified` is a subset that
   inherits it. Driven in `core/chain/membership_bound_v5_test.go`: 480 fresh registrations across
   12 blocks at TTL 2 collapse to exactly the ceiling; ablate the sweep and the gate reddens. At
   the shipped objective defaults (`RegCap` 256, TTL 32) the ceiling is **8,448**, against the ~100
   bonded validators the finished system describes, and the filed size measurement puts the member
   list at ~0.27 MB there — the O(registry) term is real and bounded two orders of magnitude below
   where it bites (at 1,000,000 members the prover's build costs 1,222 MB of live heap).
   TWO RESIDUALS, gated rather than rounded away: the bound is CONDITIONAL on the TTL, and a swarm
   that disables it has none; and `slashed` is MONOTONIC — apply never evicts a slashed id, so that
   keyspace grows with every attributable equivocation for the life of the chain and has no
   ceiling to assert. The test asserts its monotonicity instead, so if slashing ever became
   reversible both the defence and this cost argument re-open.
2. ~~**A bound on per-block verification cost.**~~ **— CLOSED.** `validateCarrier` verified an
   ed25519 signature for every `LastCommit` entry with no count checked first, so the work one
   block could demand was bounded only by the transport frame: ~1.3M entries, ~68 s of single-core
   verification, from any peer that can send a block. It now checks the COUNT before any signature,
   against a ceiling derived from the chain's own committed configuration — `carrierCap` returns
   `RegCap * (BondTTLBlocks + 1)`, which is precondition 1's membership bound restated as a count,
   because a carrier holds at most one precommit per qualified validator and the duplicate-id
   refusal enforces the "at most one". At the shipped defaults that is 8,448 entries, ~0.44 s.
   NO NEW SECURITY PARAMETER: the rule refuses only carriers no honest proposer could have built,
   which makes it a strict narrowing. ZERO MEANS UNCAPPED, honestly — a chain with no re-challenge
   cadence has no bounded qualified set to derive a ceiling from, and that is the trusted posture
   where the threat does not apply. `BondTTLBlocks` is consensus-critical and genesis-bound, so
   every replica derives the same ceiling. The ordering is the rule and is gated directly: entries
   whose signatures cannot verify are refused for SIZE, proving the count ran first.
3. **The anchor — DECIDED, and it lands with the flip.** The rule is that an ACCEPTING box must be
   anchored on an operator checkpoint. Without `-ws-checkpoint` a box pins on a provider's reported
   head — trust-on-first-use, disclosed on the line it prints — which is acceptable for an auditor
   and not for a validator, because an accepting box anchored that way inherits its provider's
   choice of history. It is not implemented separately ON PURPOSE: there is no accepting posture to
   attach the requirement to, so a flag added now would gate nothing and could not be driven red.
   It is a daemon refusal in the same idiom as the cold-start one, in the same commit as the flip.
4. **A pruned block's body is not bound to its hash — THE BLOCKER, and the owner ruled it must be
   closed first (2026-09-15).** For a pruned block the hash is a stored linkage token rather than a
   content commitment, so an adversary can keep the token and the real signatures while rewriting
   the body — including the carrier that seats validators. Closing it means the block must carry a
   commitment that SURVIVES pruning, which is a FORMAT change and therefore a new era, not a
   validity tightening. That does not happen before 2026-09-27.

*THE CONSEQUENCE, STATED PLAINLY.* This item ships **disclosed rather than fixed**, which is what
the date's stopping rule asks of anything not green. The floor box audits and reports; it adopts
nothing and advances no head; and the release says so rather than letting "validated" be read as
"participating". `VISION.md` calls the witness-validating posture settled with "same security as a
tree-holding node" — that is the destination, and at this release the box does not reach it. The
gap is recorded here, in item 14, and in the daemon's own verdict line.

*A NOTE ON WHY THE BAR IS THE OWNER'S, since the builder argued the other way.* The case for
accepting the pruned-body residual was that it is not specific to an accepting box: a full node
replaying a rewritten pruned ancestor has the same exposure today, so the box would inherit a
chain-wide residual rather than create one. The owner's call is that a box whose verdict COUNTS
must not be flipped on while a block's body can be rewritten under its hash, whoever else shares
the exposure. That is the conservative direction, and it is the one the frozen-format immutable
points at.

*And one gate this item must add rather than inherit:* a differential that drives the SAME
composition over `liveView` and `provenView` across a block corpus and requires the two verdict
sets to be identical. Flipping without it asserts "same security" rather than demonstrating it.

*What the flip does NOT deliver on its own, stated so it is not read as more:* participation.
Nothing adopts the verdict today — `AuditAbovePin` reports and returns, the daemon prints and
re-anchors, and no path appends a block or advances a head. Signing, head advance and pin adoption
are separate work, explicitly out of the box's current scope. Flipping this line alone buys a true
verdict, not a participating validator.

## Tier C — field

**21. Publish and fetch work on the internet as it is.** ⚠ *DRIVEN IN THE FIELD. The publish/fetch half is GREEN; the chain does NOT keep committing under all four conditions at once. The mechanism is NAMED — the ~1.5 MB bond proof on the consensus critical path against a deadline sized off a link rate the composed wire does not deliver — the route off that path is BUILT (3,030,653 B -> 1,131 B per attester per round, transport, no era), and it has now been DRIVEN UNDER IMPAIRMENT — the tier that can hold the claim — where it DOES NOT CLOSE IT: run `5af09a9-88230` wedged for 796 s at HEAD with the relay firing, because the relay covers 1 of n-1 attesters by construction and a stalled renewal re-broadcast 1.5 MB every 30 s until it saturated the outbound budget and dropped the consensus frames that would clear the stall. THREE CAUSES WERE FOUND, BUILT AND DRIVEN (runs `7eaf3bd-75421`, `1b0b933-52703`): the renewal storm (29 submits -> 0), the outbound bound's blindness to frame size (20 control frames dropped -> 0), and the relay's 1-of-n-1 coverage (an inventory that reaches 42:25 on a healthy chain). The chain STILL wedges at 796 s, six reproductions deep, and the reason is now a single sentence: THE CONSENSUS CRITICAL PATH SHARES ONE ORDERED PER-PEER CONNECTION WITH THE PAYLOAD THAT CONGESTS IT. Every fix above the transport moved its own number without moving the wedge, because each still needs a round trip on the connection the payload owns. This is transport work, not consensus work. The instrumentation also turned up a SECOND failure, unbounded outbound frames, that OOM-killed a validator — that one is CLOSED, the outbound path has a bound*
A NATed publisher in one region, a cold fetcher in another, bit-perfect bytes inside a bound
derived from the deployed configuration. The chain keeps committing under sustained load with
injected latency, jitter, loss and reordering.
*field.* Run it early: a failure needs time to reduce to a local reproduction.

*BOTH HALVES WERE UNBUILT, and the sheet read as though they were not.* `9-cross-nat` fetches on
`nat-2` — the other side of the SAME NAT subnet in the SAME region, because `topology.py` pins every
natted node to a single NAT subnet in the default region by construction. That proves the relay
path and it is not this claim. And no impairment existed anywhere on the cloud sheet: the four
conditions build-immutable #5 names are certified nightly on an impaired LOOPBACK
(`integration/adversarial`), over real daemons and real TCP, which is the e2e tier and not the
field one. Two flows now carry the two halves.

*THE BOUND IS READ FROM THE CLIENT, because the harness was deriving it from the wrong process.*
`FETCH_SLO_S` is documented as `-request-timeout 8s` × the daemon's retries ≈ 34 s/leg. The
publish/fetch client is a SEPARATE process and takes none of the daemon's flags — its per-attempt
deadline is its own — so that arithmetic describes a process that performs no fetch. The client now
narrates the posture it holds (per-RPC worst case with retries and backoff, holder-dial deadline,
sweep schedule, provider count, and the ceiling it enforces on itself) and
`21-cross-region-cold-fetch` builds its bound out of those numbers. A deployment that widens its
deadlines for a worse path widens the bound with them. The stale note stays where it is, corrected
in place, because older flows still grade against it.

*THE COLD FETCHER IS CHOSEN, NOT NAMED, AND ITS COLDNESS IS ASSERTED.* The flow reads the regions
off the deployed node map, takes the first eligible seat outside the publisher's region, and then
disqualifies any candidate that appears in the object's committed holder set — a fetch from a holder
measures a local disk read with a cross-region label on it. On the shipped topology that selects the
europe-west1 seat against a us-west1 publisher.

*ONE PRODUCT DEFECT, FOUND BEFORE ANY SPEND: the publish/fetch client ran with RPC retries OFF.*
A single dropped or slow packet evicted the peer it was talking to and negative-cached it. The
client's whole routing table at start-up is the bootstrap peers named on its command line, so on a
one-peer `-peers` that is its entire route into the network: one lost packet and the operation has
nobody left to ask. The daemon flag that sets this says what the value means — "0 = evict on the
first miss (fast/trusted LAN only)" — and the client, the only silt process an ordinary user runs,
was holding it. The comment above the client's configuration asserted the opposite ("it inherits the
retry"); it did not. Gated by `core/node/swarmclient_test.go`, which loses exactly one packet and
delivers every packet after it, with a vacuity arm and a retry-removed ablation.

*A SECOND PRODUCT DEFECT, and this one hid a diagnosis.* A publish waits for its entry to COMMIT on
a 360 s budget derived from the synchronizer's escape bound, whose own comment says "a client window
below the chain's in-spec height cost manufactures failure verdicts for healthy commits". The CLI's
whole-operation cap was a bare 5-minute literal — BELOW it — so the inner budget was unreachable
from the command line, every time, on exactly the slow paths it exists to tolerate, and the caller
heard `swarm operation timed out` instead of `accepted but not committed within 6m0s — the consensus
gather did not finish; see the validators' -log debug`. The cap is now derived from the budget it
wraps plus the legs the client runs first, 6m39s at shipped values, gated in
`cmd/silt/clientceiling_test.go` and driven red on the literal. Confirmed on the wire: the impaired
run below now prints the registry's own sentence where it used to print the generic one.

*THE IMPAIRMENT IS SHAPED NARROWLY, AND CANNOT PASS VACUOUSLY.* netem on the root qdisc would
degrade the IAP channel the sheet polls over, and a lost verdict is indistinguishable from a lost
block. So the interface takes a `prio` root and the impairment hangs off one band, with `u32`
filters steering only the swarm's own subnets — derived from the addresses the run deployed — into
it. Two controls run every time: the qdisc is read back on every seat, and a seat that will not take
the shaping aborts the flow rather than contributing a clean number; and netem's OWN counters must
show swarm packets crossing the impaired band, so shaping that steered nothing reports NOT CREDITED
and fails instead of grading a clean network with an adverse label on it.

*DRIVEN LOCALLY, 2026-09-18, one condition at a time against a no-op control.* Same box, same
cohort, minutes apart. Every arm credited on all four validator seats by netem's own counters.

| profile | max inter-commit gap | heights | publishes landed |
|---|---|---|---|
| CONTROL — shaping present, `delay 0ms` | 4 s | 3 | 1/1 |
| latency + jitter — `delay 80ms 20ms distribution normal` | 44 s | 1 | 1/1 |
| loss — `loss 1%` (24/9/11/9 packets dropped) | 32 s | 2 | 1/1 |
| reordering — `delay 20ms reorder 25% 50%` | 26 s | 1 | 1/1 |
| **all four at once** | **802 s — 3.6× the bound** | **0** | **0/2** |

*THE CONTROL IS WHAT MAKES THAT TABLE MEAN ANYTHING.* At `delay 0ms` the shaping machinery is
present and credited and the chain commits three heights with a 4 s worst gap, so neither the qdisc
nor the box nor the harness is the cost. Each condition ALONE then sits inside the 220 s escape
bound with room to spare. The composition does not: the chain went 802 s without committing, no
publish landed, and the registry reported that the consensus gather did not finish. The conditions
compose super-additively — 44 + 32 + 26 individually against >802 s together.

*WHAT THAT IS AND IS NOT EVIDENCE OF.* It is a reproduced, controlled observation that the four
conditions cost far more together than apart on this hardware. It is NOT yet a product verdict: the
box is a 2-CPU docker VM carrying thirteen containers, and this project's own record has that VM's
per-height cadence swinging threefold between runs of an unchanged suite. The absolute seconds are
the box's. The SHAPE — four arms inside the bound, the composition far outside it, with a 4 s
control between them — is what the field tier has to confirm or refute on real hardware with one
node per machine. That is the question only the cloud can answer, and it is the question this item
exists to ask.

*THREE HARNESS DEFECTS WERE FOUND BY DRIVING THE FLOWS, all of the "a bound that does not bound what
it appears to" family this sheet has paid for before.* The teardown check read its answer through a
pipeline, and this file runs under `pipefail`, so it took its status from a remote `grep -c` whose
count was zero rather than from the comparison — it reported "the impairment did NOT come off" about
four interfaces that were already clean, which is a verdict naming the wrong cause standing in front
of the result it was guarding. The wall was priced at a clean network's block time while the flow
graded against the escape bound, so it could report ZERO heights for a reason that was the wall's.
And the publish was run through a 90 s transport timeout while the client's own ceiling is minutes,
so the landed-publish count was the harness's number rather than the product's. All three are fixed
and the reasons are written where they bit.

*THE FIELD TIER IS DRIVEN, run `5adb538-9631` — 17 nodes, 4 validators, three regions, torn down
with zero orphans.* **22 pass · 1 gap · 1 fail · 6 skip.** Every flow that passed before still
passes with both client fixes in the shipped binary, so neither is a regression. The gap and the
fail are the two new flows, and they say opposite things.

*THE FIRST HALF IS GREEN, AND THE BOUND IS THE CLIENT'S OWN.* A NATed publisher in **us-west1**
scattered an object it could only reach the network through the relay to publish; the coldest
out-of-region seat, **us-east1**, held 3 of the object's 16 columns and pulled the rest across the
region boundary; the bytes came back **BIT-PERFECT in 13 s against a 133 s bound** the client's
posture derives over 4 chunks, inside its own 399 s operation ceiling. Every term in that bound came
off the line the client printed.

*AND IT IS STRONGER AT HEAD, on the FIRST drive of the sheet rather than a re-drive.* Run
`d531fbf-90914` grades this row **PASS** where `5adb538-9631`'s sheet recorded a gap: the chosen
out-of-region seat `val-b` (us-east1) held **0 of the object's 16 columns** — not 3 — and pulled
every one of them across the region boundary over the relay from a NATed publisher in us-west1,
bit-perfect in **10 s against the same 133 s derived bound** and the same 399 s ceiling. A seat
holding none of the object is the strongest form of the claim and the form the flow originally asked
for and could not find; the scoring rewrite is what let the sheet select it. This half of item 21 is
now green at the field tier without a re-drive standing behind it.

*"COLD" HAD TO BECOME A QUANTITY, and the first drive is what taught it.* The flow originally
required a seat holding NONE of the object, and on a seventeen-node swarm at the shipped replication
no such seat exists — an eight-chunk publish scatters twenty-odd placements over a dozen eligible
holders, so every candidate was disqualified and the flow reported itself untestable. What the claim
actually needs is a fetcher that cannot assemble the object without crossing the wire, so the flow
now scores each out-of-region candidate by how many columns it already holds, takes the coldest,
disqualifies only one holding every column, and carries the count into the verdict. A verdict that
hid how cold the seat was would be claiming more than it measured.

*THE SECOND HALF FAILS IN THE FIELD, and it is the composition that fails.* Same shape as the local
reduction, on real hardware, one node per machine, across three regions. Every arm credited on all
four validator seats by netem's own counters; interfaces verified clean after every arm.

| profile | max inter-commit gap | heights | publishes landed |
|---|---|---|---|
| CONTROL — shaping present, `delay 0ms` | 62 s | 1 | 1/1 |
| latency + jitter — `delay 80ms 20ms distribution normal` | 55 s | 1 | 1/1 |
| loss — `loss 1%` (340/574/737/351 packets dropped) | 165 s | 1 | 1/1 |
| reordering — `delay 20ms reorder 25% 50%` | 47 s | 1 | 1/1 |
| **all four at once** | **843 s — 3.8× the bound** | **0** | **0/2** |

*No single condition breaks it.* Loss is the costliest alone — 165 s against a 62 s control, 2.7× —
and still lands inside the 220 s escape bound. Latency and reordering are indistinguishable from the
control at this scale. Put together, the chain went **843 s without committing**, no publish landed,
and the registry reported `accepted but not committed within 6m0s — the consensus gather did not
finish`. The local box gave 802 s for the same profile. Two very different machines, the same shape
and the same order.

*SO THIS ITEM'S SECOND CLAIM IS NOT HELD, and the finding is specific.* "The chain keeps committing
under sustained load with injected latency, jitter, loss and reordering" is false as stated: it keeps
committing under each of them and stops under all of them. That is a composition failure, not a
threshold one, and it is the shape build-immutable #5 is least equipped to catch — the nightly netem
gate drives one arm per run, by design, so the interaction has never been exercised anywhere.

*THE MECHANISM IS NAMED, from the validators' own `-log debug` journals across an impaired window
rather than from another run.* Driven locally 2026-09-18, reproducing the field shape a third time:
749 s without a commit, zero heights, 0 of 2 publishes landed, impairment credited by netem's own
counters on all four validator seats.

**The failure is that the chain stops committing under all four conditions at once BECAUSE every
consensus proposal carries the full ~1.5 MB space-time bond proof (`BondReg.Answer`) on the critical
path, and its per-attempt transport deadline is sized from an ASSUMED 256 KiB/s floor
(`RequestSizeFloorBytesPerSec`) that the four-condition wire does not deliver.** Measured on the
impaired validator links: 53–181 KB/s, 0.2×–0.7× the assumed floor, because reordering and loss
TOGETHER keep cubic permanently in recovery in a way neither does alone. The attester does receive
the block and does prepare it — and its signed reply lands after the proposer has already declared
the attempt timed out and re-sent the whole 1.5 MB on the same shared, ordered per-peer TLS
connection. Each retry adds another 1.5 MB to a queue draining at ~110 KB/s, which lowers the
delivered rate further, so no attempt in the ladder ever fits.

*THE ARITHMETIC CLOSES, which is what makes this an attribution rather than a story.*

| quantity | value |
|---|---|
| proposal size | `bytes=1575208`, `regs=1` — one space-time proof, on EVERY block h1–h5 |
| per-attempt deadline | 8 s base + 1,575,208 B / 262,144 B/s = **14.009 s** |
| first copy finishes arriving at val-c | `gather/prepare: PREPARED` at **t+14.004 s** |
| whole ladder (4 attempts + 1.75 s backoff) | predicted 57.8 s, observed **57.43 s**, `NO PREPARE QUORUM … gathered=0 needed=2` |
| the same 1.5 MB gather, unimpaired, minutes earlier | **19 ms** |

The block finishes crossing at the instant the sender stops waiting for it, leaving 5 ms for a reply
that needs a full round trip. That is the whole failure in one line.

*THE RETRY LADDER FEEDS THE CONGESTION IT IS RECOVERING FROM, and the attesters' journals prove it.*
All three attesters logged `gather/prepare: PREPARED` for val-a's h5 r1 block FOUR times each — once
per retry — the last copies landing 86 s AFTER the gather that sent them had already terminated. Four
attempts × three attesters × 1.5 MB is 18 MB pushed onto links delivering ~110 KB/s, for one round of
one height that was already declared failed. Sampled `ss -tin` on val-a → val-d through the window:

| t+ | Send-Q | cwnd | rtt | `dsack_dups` | delivery_rate |
|---|---|---|---|---|---|
| 91 s | 1.93 MB | 16 | 158 ms | 309 | 181 KB/s |
| 112 s | 2.43 MB | 4 | 139 ms | 395 | 76 KB/s |
| 132 s | 1.63 MB | 9 | 163 ms | 468 | **53 KB/s** |
| 152 s | 1.28 MB | 11 | 155 ms | 564 | 79 KB/s |
| 172 s | 2.33 MB | 10 | 140 ms | 649 | 63 KB/s |

A standing 1.3–2.4 MB backlog that never drains, with spurious-retransmit counts climbing
monotonically. Any consensus frame written into it — a 200-byte round-change included — waits
8–40 s before its first byte reaches the wire.

*AND THE BACKLOG IS ONLY ON THE LINKS CARRYING THE PROOF.* Under the identical shaping, val-a's links
to store-1, store-2, relay and fetch-1 sat at `sendq=0` the whole window. The bulk traffic starving
consensus is CONSENSUS'S OWN payload, not the storage plane — which is the opposite of the obvious
guess and is why this had to be measured rather than reasoned about.

*CLIMBING THE ROUND LADDER CANNOT ESCAPE IT.* At h5 r2 val-c proposed and its attesters prepared at
+25 s and +33 s against the same fixed 14.0 s deadline; val-c had already advanced to r3. The
synchronizer's escape is an INCREASING ROUND DURATION (`sweepsForRound`: 2, 3, 5, 8 … sweeps × 30 s
`ChainSyncInterval`). The thing that fails is a FIXED per-attempt transport deadline against a link
the previous round's undrained copies have already slowed. The escape is orthogonal to the failure,
so each additional round adds backlog and makes the next one worse. That is why this is a wedge
rather than a slowdown, and why the 220 s escape bound was never going to cover it.

*THIS PROMOTES A RESIDUAL THIS SHEET ALREADY CARRIES.* "The bonded set is capped by bandwidth", under
*Known open*, says each validator republishes a multi-megabyte possession proof every few minutes and
every other validator must receive and verify all of it. That residual is worse than carried: at
FOUR validators, on the everyday impaired internet build-immutable #5 names as the default case, the
renewal traffic does not merely cap the practical set size — it stops the chain.

*A SECOND FAILURE THE INSTRUMENTATION TURNED UP, and it is a build-immutable #8 hit.* Under the same
profile every shaped validator's resident set grows MONOTONICALLY with no plateau, and one was
OOM-killed. RSS sampled every ~34 s through one 12-minute window:

| node | shaped? | t+0 | t+374 | t+748 | cgroup `memory.events` |
|---|---|---|---|---|---|
| val-d | yes | 48 MB | 436 MB | **1054 MB** | **`oom_kill 1`**, peak 1075 MiB (1.05 GiB) |
| val-b | yes | 58 MB | 359 MB | 906 MB | peak 1.010 GiB |
| val-a | yes | 47 MB | 316 MB | 749 MB | peak 0.937 GiB |
| val-c | yes | 64 MB | 221 MB | 463 MB | peak 0.674 GiB |
| `adversary` — a bonded chain-holder, egress **NOT** shaped | no | 45 MB | 83 MB | 73 MB | flat all window |

The unshaped control is what makes that column a measurement: same run, same box, same blocks, flat
at 70–95 MB while every shaped seat climbs an order of magnitude.

*IT REPRODUCED ON A SECOND RUN, on a freshly provisioned fleet.* Same profile, same seat role,
`oom_kill 1` again, and a higher peak: 1223 MiB against the first run's 1075 MiB, with the other
three seats at 642–815 MiB. The chain verdict reproduced with it — 747 s without a commit against
the first run's 749 s. Two runs, two OOM kills, the same order on both.

*ITS CAUSE IS AT THE SOURCE, not inferred.* `tcpnet.(*Transport).Send` marshals each frame and hands
it to `go t.deliver(…)` — one unbounded goroutine per frame — and each goroutine RETAINS its whole
marshalled frame while it waits on the per-peer write mutex `peerConn.wmu` and then on a socket whose
send buffer already holds megabytes. The inbound path has a 256 MB gate with per-peer fairness
(`adapters/tcpnet/inbound.go`); there is no outbound equivalent. Profiled on a second run with
`DEBUG_PROFILE=1`:

| goroutines, total | clean | t+0 | t+130 | t+260 | t+390 | t+520 | t+650 |
|---|---|---|---|---|---|---|---|
| val-d | 26 | 26 | 160 | 451 | 761 | 1101 | **1449** |
| val-a | 30 | 30 | 87 | 172 | 339 | 445 | **708** |

No plateau in eleven minutes. Where they are, at the two sampled depths:

| | t+130 s | t+260 s |
|---|---|---|
| val-d in `deliver` → `peerConn.write` | **134 of 160 (84%)**, 130 parked on `wmu` | **425 of 451 (94%)**, 421 parked |
| val-a in `deliver` → `peerConn.write` | **56 of 87 (64%)**, 54 parked on `wmu` | **141 of 172 (82%)**, 138 parked |

Nothing else grows. The whole goroutine population IS the outbound backlog, and the fraction parked
on one peer's write mutex rises with it — 84% to 94% on val-d across two minutes. Each of those
goroutines holds its own marshalled frame, so the count IS the memory curve, counted.

The heap agrees from the other side: at t+130 s `cbor.(*encMode).Marshal` — which is `Send`'s own
frame encode — held **70.4% of val-d's live heap (65.0 MB of 92 MB)** and 43.9% of val-a's. The live
heap IS the retained outbound frames.

*WHAT THAT SECOND FINDING DOES NOT CLAIM.* These containers carry no cgroup ceiling — the LOCAL
provisioner sets no `mem_limit` — so val-d's kill came from VM-wide pressure, not from a 2 GiB limit
being crossed. What is measured is the SLOPE and its attribution: monotone growth to 1075 MiB with
no plateau, in a structure that has no bound at all. Item 14's memory-ceiling leg measured honest
load at depth (12% of the ceiling) and three adversarial seats (1–4%); none of them is a validator
under the composed impairment, and item 14 already says how to read this — "*if a pinned seat ever
OOMs, that is the finding, not a tuning problem.*" Whether a cgroup-pinned floor seat OOMs under this
profile is a drill nobody has run.

*THE TWO FINDINGS SHARE AN UPSTREAM AND ARE STILL TWO FINDINGS.* The 1.5 MB payload is what makes the
unbounded outbound queue expensive rather than merely unbounded: each retry re-marshals and re-parks
another 1.5 MB. Taking the proof off the critical path shrinks the second finding by three orders of
magnitude but does not BOUND it — the outbound path would still have no cap, and #8 and S3 want a
bound, not a small number.

*THE SECOND FINDING IS NOW CLOSED: THE OUTBOUND PATH HAS A BOUND.* `Send` charges every frame —
and the delivery goroutine that will carry it — against a budget before starting that goroutine,
with a per-peer share so one backed-up link cannot consume what every other peer is owed. It is the
dual of the inbound gate that already sits on the read path, and it is deliberately written to read
like it (`adapters/tcpnet/outbound.go` beside `inbound.go`), with one difference that is forced
rather than chosen: **it REFUSES where the inbound gate BLOCKS.** The inbound gate blocks a
per-connection reader, which is safe — that stops draining one socket and TCP flow-control pushes
back on the sender. `Send` runs on the node's single serialized loop (B2), so blocking there would
stall every peer, every timer and every API call behind one slow socket: a bounded memory failure
traded for a whole-node liveness failure. A frame that does not fit is therefore DROPPED, which is
the transport's already-documented loss semantics — "a failed write or dial just drops the message
and the core's timeout machinery owns recovery" — and it is returned to the caller and narrated in
the debug log rather than swallowed.

*THE GOROUTINE IS CHARGED WITH ITS FRAME, so the bound is on memory rather than on bytes.* A
byte-only budget would let a flood of small frames sit comfortably inside it while the goroutine
stacks carrying them — the larger cost at that size — grew without limit, which is the same
unboundedness in a different allocation. Each admitted frame costs its own length plus a fixed
charge for the goroutine that owns it, so one number bounds the whole outbound footprint and there
is no second knob to size.

*THE GATE IS DRIVEN AGAINST ITS OWN CONTROL, in the same test, on a peer that completes the TLS
handshake and then never reads — the field condition reduced to a fixture that runs in a tenth of a
second.* Forty-eight 1 MiB frames at that peer:

| arm | peak in-flight outbound | frames refused |
|---|---|---|
| UNBOUNDED — the documented `0` sentinel, which is the pre-bound behaviour | **50,727,984 B** (everything the sender offered) | 0 |
| BOUNDED — an 8 MiB budget, 2 MiB per-peer share | **1,056,833 B** | 47 |

The control is what makes the second row mean anything: a bound that is never reached passes for
free, so the unbounded arm has to blow past the cap the bounded arm asserts, and the test says so in
those words when it does not. The bounded arm also fails if NOTHING was refused — a flood that
fizzled would otherwise read as a bound that held.

*AND THE POSITIVE CONTROL IS ON THE GATE'S OWN AXIS, because a bound that drops frames on a healthy
link is a liveness regression wearing a memory fix's clothes — and it would pass the table above
perfectly.* At the shipped budget a draining peer receives all 200 of 200 back-to-back 64 KiB frames
and the budget returns to zero. That control is what SIZED the default rather than taste: driven at
8 MiB instead, the same honest burst starts losing frames at number 28. The shipped value is
`DefaultOutboundCap`, and the daemon's flag default is derived from that constant rather than
restating it, so the number the tests exercise and the number an operator gets cannot drift apart.

*THE SCOPE IS THE DAEMON, and that is a boundary rather than an oversight.* The transport ships
unbounded and the DAEMON sets the cap, exactly as it already does for the inbound gate. The
short-lived client processes — publish, fetch, the ephemeral issuer dial — keep the unbounded
default, because the measurement is a long-running validator under sustained impairment and there
is none for a client. Capping them on the strength of the validator's number would be a guess with
a cost attached: the publish path's placement leg is the first thing a contended box loses, and
this list has paid for that reading twice. Named here rather than left to be discovered.

*WHAT THIS DOES NOT CLAIM.* It does not close the chain wedge. The wedge is the FIRST finding — the
1.5 MB proof on the consensus critical path against a deadline sized off a link rate the impaired
wire does not deliver — and a bound on the sender's queue does not make a block arrive inside its
deadline. What it closes is the build-immutable #8 hit found beside it: the outbound path now has a
ceiling instead of none, which is what #8 asks for ("a bound, not a small number"). The two findings
share an upstream and remain two findings.

*WHY NOTHING BELOW THE FIELD COULD HAVE CAUGHT EITHER.* `adapters/simnet.Config` is
`{LatencyMin, LatencyMax, Loss}`, and a message is delivered atomically after a latency draw — no
bandwidth, no serialization delay, no send queue. A failure whose entire mechanism is "payload bytes
÷ link rate exceeds the deadline" cannot exist in the sim tier however many scenarios are enumerated,
and neither can a queue that grows because the wire is slower than the producer. That is a gap in the
evidence pipeline, not in the code under test, and it is the reason V1's sim-first tier saw nothing.

*WHAT THE FIX IS NOT.* A knob. `-request-timeout`, `RequestSizeFloorBytesPerSec` and the retry count
are transport numbers whose only effect is to move where the cliff sits; #5 names "payload-scaled,
never a magic constant" and #3 forbids resting anything on a number the adversary's own path can
move. Raising the floor would also widen `maxChainReplyBytes`, which is derived from it. The
structural close is the one #5 already names — **keep large payloads off the critical path (succinct
proofs > FEC > QUIC)** — and v5 already has the hook: `BondReg.AnswerDigest` exists so a block can
COMMIT to the heavy proof without CARRYING it.

*OWED ITEM (1) IS DONE: THE REPRO EXISTS BELOW THE FIELD TIER.* `simnet.Config` now carries
`RateBytesPerSec`, so a link has finite capacity — a message costs serialization time and messages
behind it WAIT. Opt-in: with no rate configured a 64 MiB message still arrives at t=0, which is what
leaves every scenario written against atomic delivery unchanged. On top of it, `core/node` drives
the same 1.5 MiB request on either side of the assumed floor — it times out at a quarter of the
floor (24 s of wire against a 14 s deadline) and succeeds at the floor.

*AND WORKING OUT WHAT "BELOW THE FLOOR" MEANS WAS THE EXERCISE.* The first cut used HALF the floor
and PASSED, because the 8 s base absorbs it. A request fits whenever the wire beats
`payload / deadline` = 112 KiB/s, itself below the floor. The tolerance is
`base / (base + extension)`, so it SHRINKS as the payload grows: at 1.5 MiB the wire may run 43%
slow; at 15 MiB the extension caps at 30 s and the tolerance collapses. The under-budgeting is worst
exactly where the payload is largest, which is the consensus critical path.

*OWED ITEM (2) WAS DRIVEN, AND THE RESULT REFRAMES IT.* The composed profile — all four conditions
at once — was run against the full P0 gate on an impaired LOOPBACK, and the drills PASS:
`TestBondEarnedStandingCommitsOverTCP` 27 s, `TestObjectiveConsensusCommitsOverTCP` 50 s,
`TestPublishCommitFetchOverTCP` 48 s, `TestEquivocatorSlashedOverTCP` 42 s. **The field wedge does
not reproduce on loopback.** The bond-standing drill carries the same ~1.5 MB registration that
stopped the chain in the field, and it finished in 27 s.

*THAT IS A FINDING ABOUT THE TIER, NOT A CLEAN BILL.* The mechanism is the DELIVERED RATE falling
below the deadline's assumed floor. Loopback's capacity is not the constraint, so the four
conditions degrade timing there without ever starving the wire — the same structural blindness the
sim had, one tier up. The sim could not express rate because it had none; loopback cannot express it
because it has too much. A composed arm is therefore cheap to add and will not destabilise the
nightly, but it must NOT be sold as covering this item's failure: it closes a COVERAGE gap (the four
conditions had never been exercised together anywhere) and buys no early warning of the wedge.

*AND THE COMPOSED ARM DOES NOT REPRODUCE GREEN ON A DEVELOPER BOX, which is worth writing down
before someone re-drives it and thinks they broke it.* Driven locally 2026-09-19 on a 2-CPU docker
VM at host load 6–12: six of the seven P0 drills PASS, and
`TestObjectiveConsensusCommitsOverTCP` FAILS at 371.9 s with `accepted but not committed within
6m0s — the consensus gather did not finish`. The same arm at the SAME commit as the recorded green
dispatch, driven on the same box, same path, same impairment minutes later, fails the SAME drill
with the SAME sentence at 377.5 s — so the red is a property of this hardware, not of any change
after that dispatch, and the control is what says so rather than an inference from the timing.

| arm | bond-standing | objective-consensus | the other five |
|---|---|---|---|
| local, current HEAD | PASS 26.9 s | **FAIL 371.9 s** | PASS |
| local, the dispatched-green commit | PASS 37.95 s | **FAIL 377.5 s** | PASS |
| the dispatched CI run | PASS 27 s | PASS 50 s | PASS |

*THAT IS A SECOND READING OF THE TIER, and it cuts against the line above.* The argument for the
composed arm was that loopback has too much capacity for the four conditions to starve the wire, so
the arm degrades timing without reproducing the field mechanism. On a contended two-core box the
same profile takes one drill from 50 s to over 370 s and past a six-minute commit budget — which is
either the arm being marginal on capacity after all, or a cadence effect this list has measured
before (a threefold per-height swing on this VM between runs of an unchanged suite). WHICH ONE IS
NOT ATTRIBUTED HERE: that needs the validators' debug journals across the failing window, and it is
its own piece of work. What IS established is that the arm is green on CI hardware, red on this box
at the same commit, and that a local red on it is not evidence about whatever change is in the tree.

*A RATE ARM WAS TRIED AND IS NOT ADDED, AND THE REASON IS WORTH THE LINES.* `tc netem rate` caps
bandwidth directly and is the loopback analogue of the `RateBytesPerSec` term now in simnet, so it
looked like the arm that would see this class. Driven at `rate 1mbit` (128 KiB/s, half the assumed
floor) it DOES break the gate — and it breaks it in the wrong place:

```
TestBondEarnedStandingCommitsOverTCP FAIL (31.8s)
  silt: manifest chunk 93678b1f… placed on no node after 4 attempts (network full or unreachable)
```

That is the PUBLISH/PLACEMENT leg timing out, not a consensus deadline being missed. The publish
path gives up placing chunks long before consensus notices anything, so the red names a cause that
is not the mechanism — and an arm whose failure points at the wrong subsystem is worse in the
nightly than no arm, because the next reader spends their time there.

*SO THE RATE ARM IS OWED A CALIBRATION, not a decision.* It needs a rate low enough to squeeze the
consensus payload against its size-extended deadline but high enough that placement still completes
— or it needs to drive the consensus drills alone rather than the full gate. Until that number is
measured the arm stays out; what ships is the composed arm, which is green and closes the coverage
gap honestly. The unit-tier repro above already covers the arithmetic deterministically, so nothing
is UNTESTED here — what is missing is the loopback early-warning, and it is missing on purpose
rather than by omission.

*THE ERA QUESTION IS NOT SETTLED, AND THE PREMISE IS THE PART TO TEST (2026-09-19).* The owed item
below asks whether the structural close is a new era. Read carefully, it may be answering the wrong
question, and the difference decides whether this item can close before the date.

Moving the proof out of the COMMITTED BLOCK is a widening — registrations that are invalid today
become valid — and a widening splits, so that is an era. But two facts in the tree point elsewhere.
First, `Prune()` already drops `BondReg.Answer` from a finalized v5 block and the block STILL
reproduces its own hash, because `v5PreimageBondRegs` folds `AnswerDigest` in place of `Answer` — the
format already expresses a proof-less body. Second, and more to the point:
**every attester already receives and verifies the registration before the block carrying it is
proposed.** `SubmitBondRenewal(peers)` broadcasts the full ~1.5 MB reg to the same validator set the
node reconciles against; each receiver runs `ValidateBondRegErr` — which pays the full
`VerifySpaceTime` — and queues it. The proposer then folds that same registration into a block and
re-ships the same 1.5 MB to the same attesters, who verify it a second time.

So what the PROPOSAL MESSAGE carries may be separable from what the BLOCK COMMITS. If an attester
reconstructs the full block from its own pending queue by digest, the committed bytes are identical,
validity is identical, the signed hash is identical, and nothing forks — which is compact-block relay,
settled prior art that was not a hard fork where it was first deployed. That is a transport change,
not a format change, and it needs no era.

*THIS IS A READING OF THE CODE, NOT A MEASUREMENT, and #7 says the next action is to gather the
evidence rather than to act on it.* Three things must be measured before any of it is built: whether
retry de-duplication alone (not re-shipping the payload on a retry of the same height and round) moves
the wedge, since the journals show four copies per attester for one already-failed round; whether the
attester's pending queue actually holds the registration at the moment the block arrives, given that a
proposer's own F6 reg is minted fresh over `b.Prev` and at bootstrap is new to everyone; and what the
fallback costs when it does not. The deterministic repro for all three now exists below the field tier
— `simnet.RateBytesPerSec` with the same 1.5 MiB request on either side of the assumed floor — so this
is answerable without a cloud spend.

*WHAT IS OWED.* (3) The structural close on the payload. The measurement it had to start with is
DONE (2026-09-19, `core/node/bondproof_criticalpath_measure_test.go`) and it names the route:
pre-deliver EVERY registration including the proposer's own — the one source an attester's queue
never holds — then relay the block by digest, so the bytes are off the per-attempt deadline and out
of the retry ladder. The bound on the outbound path is DONE (above).

(4) ~~A decision about what it means for the date — and the structural close is plausibly a NEW ERA
rather than a validity tightening, which by the frozen-format rule does not happen before it.~~
**DECIDED: it is NOT an era, so the date does not rule it out.** The committed bytes, the validity
rule and the signed hash are all unchanged under pre-delivery plus digest relay; only what the
PROPOSAL MESSAGE carries moves. `Prune()` already drops `Answer` from a finalized v5 block and the
hash still reproduces via `AnswerDigest`, so the format already expresses a proof-less body. That
makes the close reachable before 2026-09-27 rather than blocked behind a hard fork. It stays
sequenced after item 10's last defeat, which is the piece an outside adversary reaches first.

*THE RELAY IS DRIVEN UNDER IMPAIRMENT AND IT DOES NOT CLOSE THE WEDGE — run `5af09a9-88230`,
2026-09-19.* Seventeen nodes, four validators, three regions, every seat on-demand so none could be
preempted; 31 resources destroyed with zero orphans. **23 pass · 0 gap · 1 fail · 6 skip**, and the
one fail is this row. Under the composed profile the chain went **796 s without a commit** — h33→h33,
zero heights, 0 of 2 publishes landed, impairment credited by netem's own counters on all four seats,
interfaces verified clean afterwards. That is the FOURTH reproduction of the same shape: 802 s local,
843 s field, 749 s local, and now 796 s in the field **at HEAD, with the relay built and firing**.

*THE RELAY WAS NOT DARK, WHICH IS WHAT MAKES THIS A RESULT RATHER THAN A MISSED SETUP.* It engaged on
the fleet from the first heights, and every other flow on the sheet passed, so the change is not a
regression and the row is not a setup failure. What it is, is a coverage measurement — and the
coverage is the finding.

| prepare legs on the wedged heights (h33–h34) | count | bytes each |
|---|---|---|
| digest-relayed | **6** | 1,092–2,651 B |
| carried | **30** | ~1,575,000 B |

*THE SHED LEGS ARE ALWAYS ONE PEER PER PROPOSAL, AND THE REASON IS STRUCTURAL.*
`peerCanReconstruct` needs evidence for EVERY registration in the block, and it has exactly two:
the peer AUTHORED the registration, or the peer ACKNOWLEDGED this node's OWN. On a live chain
renewals are staggered, so a block carries ONE OTHER validator's registration — and only its author
qualifies. Every other attester is sent the full proof, which is the payload the wedge is made of.
Coverage is therefore **1 of n−1 attesters by construction**, not by accident, and the relay leaves
the critical path substantially as it found it.

*THE ONE PATH THAT LIFTS THAT IS DESTROYED EVERY 30 s, AND THE JOURNALS SHOW IT WORKING FIRST.* At
h1–h4 each proposer shed to ALL THREE peers — the acknowledgement path, full coverage, exactly as
designed. It never happens again on the whole run. `SubmitBondRenewal` clears `ownRegAcks` on every
sweep on the stated grounds that "a fresh registration means every prior ack is about different
bytes". On a wedged chain the head does not move, and a registration is a deterministic function of
its prev (`TestBondRegIsADeterministicFunctionOfItsPrev`) — so **the bytes are IDENTICAL and the
receipts are discarded anyway**, every 30 s, faster than a stalled round can ever use them. val-a's
h34 blocks carried val-a's own registration, its peers had acknowledged those exact bytes fifteen
times over, and it shed to nobody.

*AND THE STALLED RENEWAL STARVES CONSENSUS OF THE BUDGET THAT WAS ADDED TO SAVE IT.*
`BondRenewalDue` stays true until a block COMMITS the renewal, so a wedged chain re-broadcasts the
full ~1.5 MB to every peer on every sweep — **29 times on val-a inside this window, one per 30 s
`ChainSyncInterval`, exactly**. That saturates the per-peer outbound budget, which then drops the
consensus frames that would have ended the wedge:

```
gather: prepare request FAILED to=3247042e… height=34
  err="tcpnet: outbound budget full: 67066043 bytes already in flight
       against a 67108864-byte share; dropped a frame of 1575203 bytes"
```

All four of val-a's h34 r3 prepare legs died that way, on all four peers, within the same
millisecond. The wedge STARTS on the original mechanism — carried bytes against a deadline sized off
a floor the wire does not deliver — and then SUSTAINS ITSELF: the stall keeps the renewal due, the
renewal storm fills the budget, and the budget drops the frames that would clear the stall.

*THE BOUND IS NOT THE DEFECT, AND THE DISTINCTION MATTERS.* Worst RSS across the cohort was
**0.92 GiB with no OOM kill**, against `oom_kill 1` at 1075 MiB and 1223 MiB on the two runs before
the outbound gate existed. The gate did exactly what it was built to do. What it also did was
convert an unbounded MEMORY failure into a bounded LIVENESS one — the trade its own design note
predicted in writing ("a frame that does not fit is therefore DROPPED … and the core's timeout
machinery owns recovery"). The measurement here is that the timeout machinery cannot own the
recovery when the budget it needs is being consumed by the stall's own retransmissions.

*SO ALL THREE FINDINGS HAVE ONE ROOT, AND IT IS NOT THE RELAY.* `SubmitBondRenewal` cannot tell
"renewal due and not yet broadcast" from "renewal due, already broadcast, still waiting to commit".
The first costs a re-broadcast every sweep; the second wipes the receipts that are the relay's only
evidence; the third is the first one's traffic landing on a finite budget. The relay is correct and
its four properties hold — what is measured is that its coverage is thinnest exactly when the chain
needs it most.

*THAT ROOT IS NOW FIXED, AND WHAT THE FIX DOES NOT DO IS THE PART TO READ.* A renewal is broadcast
ONCE and remembered until it commits: the copy stands while the head it was minted over is still
the head, only peers WITHOUT a receipt are sent anything, and the acknowledgement now means the
bytes are HELD rather than that the message arrived — `MsgSubmitBondRegAck` replied `OK=true` after
every refusal, including the validity refusals, so a receipt the relay reads as "this peer holds the
proof" was being written for peers that had queued nothing. Per sweep with the chain committing
nothing: **[3 3 3 3 3 3] before, [3 0 0 0 0 0] after**.

| the rule | why it is not the obvious one |
|---|---|
| keep while the HEAD has not moved | `ValidateBondReg` alone accepts a copy over the last `BondRegHeadWindow` heads — a bound on the NONCE's freshness, not on whether committing it still renews standing. With a TTL tighter than that window a copy stays window-valid after the standing it defends has decayed, and the node broadcasts nothing while it drops out of the bonded set |
| re-send only where there is no receipt | a peer whose submit was lost must still be retried, or a single dropped packet lapses a validator — the positive control a traffic fix would otherwise pass by simply sending less |
| the ack reports HELD, not RECEIVED | the relay's whole evidence rule rests on this bit, and an unconditional OK sheds a proposal to a peer that cannot rebuild it |

The wrong version of the first rule was written first and
`sim.TestObjectiveBondRenewalSustainsAttestOnlyValidator` — TTL 3 against the default window of 8 —
stalled at round 5 with `0 prepares of 2 gathered`. The sim tier caught a liveness regression that
every unit assertion had passed, which is V1 working exactly as written.

*AND IT IS NOT A CLOSE.* The relay's coverage is STILL 1 of n−1 attesters by construction: a block
carries one OTHER validator's registration and only its author can be proven to hold the proof, so
blocks still push ~1.5 MB to n−2 attesters against a deadline sized off a floor the impaired wire
does not deliver. What is removed is the amplifier that made the stall self-sustaining and the
receipt churn that collapsed the relay's coverage where it had any. Whether that is enough is a
question only a drive under impairment answers, and **it has not been driven** — timing the bond
drill on loopback shows no difference (7.4/7.0/7.4 s against 8.7/7.1/7.0 s), which is expected,
because loopback has too much capacity to express a cost that is about a wire. Lifting the
structural 1-of-n−1 is a separate piece: it needs a proposer to learn that a peer holds a THIRD
party's registration, and nothing on the wire carries that today.

*RE-DRIVEN AFTER THE RENEWAL FIX — run `7eaf3bd-75421`, 2026-09-20. THE AMPLIFIER IS GONE, THE WEDGE
IS NOT, AND THE RUN FOUND SOMETHING SHARPER THAN EITHER.* Same fleet shape, every seat on-demand,
31 resources destroyed with zero orphans. **23 pass · 0 gap · 1 fail · 6 skip** — the one fail is
this row again, **814 s without a commit**, h46→h46, zero heights, 0 of 2 publishes landed,
impairment credited on all four seats. A fifth reproduction of the same shape.

*WHAT THE FIX DID, MEASURED RATHER THAN ARGUED.* The three findings the previous run attributed were
all downstream of one root, and that root is closed:

| | run `5af09a9` (before) | run `7eaf3bd` (after) |
|---|---|---|
| `bond renewal submitted` inside the impaired window | **29** on one seat | **1** across all four |
| relay coverage of registration-bearing prepare legs | 6 of 36 — **17%** | 9 of 24 — **37.5%** |
| a height shed to EVERY peer | none after h4 | **h45, 4 of 4** |
| `outbound budget full` | yes | **still yes — 23 frames dropped** |

The storm is gone in the field, not only in a unit test. Coverage more than doubled, and a height
shed to every peer for the first time since bootstrap — the receipt churn really was what collapsed
it. Worst RSS 0.87 GiB with no OOM, so the outbound bound still holds.

*AND THE WEDGE STILL REPRODUCES, WITH THE CAUSE NOW ISOLATED.* **Fifteen prepare legs still carried
~1,574,000 B each.** That is the 1-of-n−1 coverage limit and nothing else: a block carrying one
OTHER validator's registration can be shed only to its author, so the remaining attesters are sent
the proof by value against a deadline the impaired wire does not meet. Removing the amplifier did
not move the wedge, which is the cleanest possible statement of where the remaining work is.

*THE NEW FINDING IS THAT THE BOUND HAS NO FRAME-KIND FAIRNESS, AND IT IS ARGUABLY SHARPER THAN THE
COVERAGE LIMIT.* Of the 23 frames the outbound budget dropped, **20 were 121 bytes** — chain-sync
window requests and head probes, not proposals. The per-peer share is fair between PEERS and blind
between FRAME KINDS, so 1.57 MB of proposal retries sit in front of the small control frames that
recovery depends on. Every seat, simultaneously:

```
chain sync sweep made NO progress while behind our-next=47 max-peer-head=0
  peers=4 probe-fails=4
  last-err="window@46 from f9008cef: tcpnet: outbound budget full ..."
```

`max-peer-head=0` and `probe-fails=4` on **all four validators at once** — the entire set went blind
to each other's heads, attributed by the nodes' own error string to the budget rather than to the
shaping. A node that cannot send a 121-byte probe cannot discover that it is behind, cannot fetch
the window that would catch it up, and cannot fail fast either: a refusal reply is 121 bytes too.
This is head-of-line blocking INSIDE the bound that closed the OOM, and it would bite on any
congested link even if the coverage limit were closed tomorrow.

*SO THE ITEM NOW CARRIES TWO SEPARATE PIECES OF WORK, AND THEY ARE INDEPENDENT.* Lifting coverage
to every attester takes the heavy bytes off the critical path; reserving a share of the outbound
budget for small control frames keeps a congested link from blinding the node that is trying to
recover on it. Neither subsumes the other, and this run is what separated them — which is what the
previous sheet could not do, because the renewal storm was masking both.

*THE SECOND OF THOSE IS BUILT, 2026-09-20.* An eighth of every share, and of the global cap, is
reserved for frames of 64 KiB or less; bulk is admitted only up to the remainder. It is a SIZE rule
and deliberately not a message-kind rule — the transport is an adapter (B1), and teaching it which
of the core's kinds matter would put a consensus concern in the wire layer and leave every new kind
silently in the wrong class. The two populations are four orders of magnitude apart (121 B probes, a
shed proposal at ~511 B, against carried proofs at ~1,575,000 B and a 256 KiB chunk), so the
threshold is not delicate. **The reserve comes out of the share, never on top of it**, so the
ceiling an operator sets stays the ceiling.

| the arm | before | after |
|---|---|---|
| bulk fills a 16 MiB share, then a 121 B probe | **refused** — bulk reached within 1 B of the share | **admitted** |
| a peer's total in flight against its share | — | never exceeds it (the reserve is subtractive) |
| a draining peer at the shipped budget, 200 back-to-back frames | all delivered | all delivered |
| the unbounded control against the bounded arm | 50,727,984 B vs 1,056,833 B, 47 refused | unchanged |

*AND THE FILL IS WHAT MAKES THAT A TEST RATHER THAN A COINCIDENCE.* Admitting uniform 1 MiB frames
until one is refused stops an average of half a frame short — hundreds of kilobytes of slack, which
a 121-byte frame fits into comfortably, so the assertion would have passed on the very behaviour it
exists to catch. The field had 5,560 bytes of slack in a 67,108,864-byte share. The top-up frame is
therefore sized to the headroom actually left, bulk fills to within ONE BYTE, and the probe behind
it is refused before the fix.

A refused CONTROL frame now narrates itself at WARN with the count attached, instead of reading
identically to a bulk frame being held back: bulk refused is the gate working, a small frame refused
is the backlog consuming even the reserve. The original diagnosis took a journal cross-read over
four nodes; it is one grep now. **It is UNDRIVEN under impairment** — the next reading would say
whether the seats still go blind, and it does not close the wedge, which is the other piece.

*ALL THREE CAUSES CLOSED AND DRIVEN TOGETHER — run `1b0b933-52703`, 2026-09-20. TWO HOLD, THE THIRD
CANNOT, AND THE REASON IS THE MOST USEFUL THING THIS ITEM HAS LEARNED.* Same fleet, every seat
on-demand, 31 resources destroyed with zero orphans. **23 pass · 0 gap · 1 fail · 6 skip**, and the
one fail is this row at **796 s**, h54→h54 — a sixth reproduction.

| inside the impaired window | `5af09a9` | `7eaf3bd` | `1b0b933` |
|---|---|---|---|
| `bond renewal submitted` | 29 on one seat | 1 | **0** |
| `CONTROL frame dropped` (121 B probes) | 20 of 23 drops | 20 of 23 drops | **0** |
| `outbound budget full` | yes | yes | 7, all BULK — the gate working |
| relay coverage, registration-bearing legs | 6 of 36 | 9 of 24 | 9 of 28 |
| heavy legs still carried | — | 15 | **19** |

*TWO OF THE THREE FIXES HOLD UNDER IMPAIRMENT AND ARE NOW FIELD-CONFIRMED.* The renewal is
broadcast once and never re-broadcast on a stalled chain. No control frame was dropped at all, where
the previous run dropped twenty; the seven refusals left are bulk being held back, which is the
budget doing its job.

*THE THIRD CANNOT WORK WHERE IT IS NEEDED, AND THE DEPENDENCY IS CIRCULAR.* Coverage was **42 shed
against 25 carried at h54**, immediately before the shaping went on — the inventory works. Inside the
window it collapses to one peer per proposal, which is the author case and nothing else. The reason
is in the sweep log, on every seat, every 30 s, for the whole wedge:

```
chain sync sweep made NO progress while behind our-next=55 max-peer-head=0
  peers=4 probe-fails=4 head-matches=0 windows=0 suffix-appends=0 reconciles=0 last-err=
```

The inventory rides the head REPLY. The head reply is exactly what cannot arrive during the wedge.
**The evidence channel is starved by the same congestion the evidence exists to relieve**, so the
mechanism is strongest on a healthy chain and absent on a stalled one. That is a design fault in the
route, found by driving it, and no amount of local testing would have shown it: every tier below the
field delivers the reply.

*AND THE PROBE FAILURE HAS MOVED ONE LAYER DOWN, WHICH IS THE OTHER HALF OF THE READING.* Last run
`last-err` named `outbound budget full` and 20 of 23 dropped frames were 121-byte probes. This run
**no control frame was dropped and `last-err` is empty** — `probe-fails` now counts a TIMEOUT, not a
refusal. The frames are admitted by the gate, written to the socket, and then wait behind megabytes
in the **shared, ordered, per-peer TLS stream**. An application-level budget can decide what to hand
the socket; it cannot reorder what the socket has already accepted.

*SO THE RESERVE FIXED A REAL DEFECT AT THE WRONG LAYER FOR THIS PURPOSE.* It is still correct and
still wanted — dropping a control frame is strictly worse than delaying one, and the drops are gone.
But head-of-line blocking on a single ordered connection is below where any admission control
reaches, and that is what now starves the sweep. The remedies are structural and the list already
names them under build-immutable #5's settled prior art: **succinct proofs > FEC > QUIC** — either
stop putting megabytes on that connection, or stop making control traffic share one ordered stream
with them.

*WHAT THIS LEAVES ITEM 21 WITH, stated plainly.* The second claim is NOT held, and the remaining
cause is now a single sentence: **the consensus critical path shares one ordered per-peer connection
with the payload that congests it.** Every mechanism that tries to fix this above the transport —
retry de-duplication, alignment, digest relay, an evidence inventory — has been built and measured,
and each one moved the number without moving the wedge, because each one still needs a round trip on
the connection the payload owns. That is a coherent finding, it is six reproductions deep, and it
points at transport work rather than consensus work.

The publish/fetch half is done and is not affected by any of it. The chain half is NOT held today
and none of the above holds it: if the close does not land by the date, this item ships disclosed
with the mechanism named, the repro in the tree and the route measured.

**22. Every item has a cloud-harness run.** Each item above named in a cloud scenario, with
the gaps written down as decisions rather than left as silence.
*field.*

*It does NOT carry item 5's whole-suite run,* and an earlier revision of this list said it did.
`cloudtest` drives its own scenario set against a 27-node cloud topology; item 5 is about the
suites under `integration/*/run.sh`, which run sequentially on a developer box. The two are
different artifacts and the mistake would have left item 5 assigned to something that cannot close
it. `integration/cloudtest/` is the functional harness (23 real runs behind it);
`integration/twocloud/` is the cross-cloud scaffold this list defers.

*THE FREE TIER IS DRIVEN, 2026-09-15:* `LOCAL=1 ./cloudtest.sh all` over a 4-node local topology —
**21 pass / 0 gap / 0 fail**, swept clean. Among them: an equivocator slashed on the wire with zero
blast radius to the main sheet; a minority partition that STALLED and then CAUGHT UP rather than
reorging; takedown biting on one operator while another kept serving bit-perfect; the default chain
refusing a durable file→publisher link; bit-perfect fetch after a SIGKILL; and a worst-case RSS of
0.22 GiB across the whole cohort. That run is also the local proof the billable pre-flight gate
demands before any spend — it refuses to `apply` until a mechanism, a reproduction and a local
proof are recorded, which is build-immutables #6 and #7 wired into the money path.

*Every `apply` is still billable:* no spend, then a small smoke run, then the full topology only
once smoke is green. Two things to know before the full tier — `TTL_MINUTES=180` self-destructs
every VM regardless of what the harness does, and `BUDGET_AMOUNT_USD` is 0 with no billing account,
so NO budget alarm is configured and the TTL is the only backstop. Verify teardown explicitly
rather than trusting the exit trap.

---

## What "held" currently rests on, and where it does not reach

Asked and checked 2026-09-19: is every item marked held backed by BOTH local and cloud evidence? It
is not, in three different ways, and the three are worth keeping apart because only one of them is a
problem with the claims themselves.

**1 — Most held items never claimed a cloud tier, and are right not to.** Items 1, 4, 7, 8, 15 and 16
declare ladders that stop at unit, integration or e2e. Item 4 is a source gate over the tree; a cloud
run could not say anything about it. Items 7 and 8 are validity rules driven by injecting a forgery
and asserting a stall, which is a thing a unit and an e2e tier do precisely and a seventeen-node fleet
does not do at all. A missing cloud row there is a correct scope, not a gap — but it does mean "held"
means "held at the tiers it declares", never "held everywhere".

**2 — Two held items declare a FIELD tier they have not reached.** Item 9 (*model-check → e2e →
field*) and item 14 (*unit → e2e under impairment → field*) are both marked held with the field rung
undriven. By this list's own rule — "skipped", "gap" and "not run" are all failures — that is not
what held should mean. Item 9's committed-slash half is the sharper case: it was driven with `LOCAL=1`
over a four-node docker topology, which is the CLOUD HARNESS ON A LOCAL BOX and not the field. The
distinction matters exactly where item 9 lives, because the property is that every replica evicts in
lockstep, and a lockstep across four containers on one kernel is the cheapest possible version of
that claim.

**3 — ~~AND NO CLOUD RUN EXISTS AT THE CURRENT HEAD~~ — CLOSED 2026-09-19 by run `d531fbf-90914`.**
It was the sharpest of the three: the last full cloud run was `46f3224-79920` (2026-09-16) and the
field run `5adb538-9631` (2026-09-18), leaving the tree **28 commits past the first and 20 past the
second** — three product fixes on the audit path (the foreign-rooted proof refusal, the judge's
budgeted survivor fetch, the challenge rate limit), a consensus fix (a queued equivocation proof now
arms a proposal), and a transport change that bounds every outbound frame. Every one touches a path
the cloud run exercises, so several rows rested on evidence about a binary that no longer existed.
The re-validation below is what closes it, and nothing was built on top of the twenty-eight first.

*THE RUN.* 19 nodes, 4 validators, three regions, `ECONOMY=1`, torn down to zero VMs and verified
rather than trusted to the exit trap. **23 pass · 2 gap · 1 fail · 5 skip**, against `46f3224`'s
22/0/0/6 and `5adb538`'s 22/1/1/6. A SMOKE rung was driven first on the same commit
(`d531fbf-67723`, 11 pass, torn down) to price the cloud path before paying for the sheet.

**NOTHING REGRESSED, AND ONE ROW MOVED FORWARD.** `21-cross-region-cold-fetch` was a gap on
`5adb538` — no out-of-region candidate was cold enough to test — and is now a **PASS** in 14 s: a
NATed publisher in us-west1 to `val-b` in us-east1 holding **zero** of the object's columns,
bit-perfect. `184-equivocation-island` passes with the slash COMMITTED, which is the first full-sheet
field exercise of the consensus quiescence fix. `8-takedown`, `7-restart-content` and
`7-restart-standing` — the rows `46f3224` was run to confirm — still pass with the changed audit path
underneath them.

**THE MEMORY NUMBER, AND WHAT IT IS NOT.** Worst peak across the cohort 0.71 GiB; `val-d` 730.6 MiB,
`val-a` 680.6, `val-b` 662.7, with the composed-impairment arm running on the same sheet, and
`infra-node-liveness` PASS — no OOM-kill or crash-loop anywhere. The two pre-bound runs had `val-d`
at 1075 MiB and 1223 MiB and OOM-killed both times. That is CONSISTENT with the outbound bound
holding and is NOT a controlled measurement of it: different substrate (a cloud e2-small against a
local docker VM with no cgroup ceiling), and no unbounded arm ran here. The controlled evidence for
the bound stays the paired arms in `adapters/tcpnet`; this is corroboration at the field tier.

**THE FAIL IS THE KNOWN WEDGE, REPRODUCED A THIRD TIME.** `21-impaired-commit`: 794 s without a
commit against the computed 220 s escape bound, 0 of 2 publishes landed, impairment credited on all
four validator seats. 802 s local, 843 s field, 794 s field at HEAD — three machines, same shape,
same order. It reproduces WITH the outbound bound in the binary, which is the field confirmation of
that fix's own stated non-claim: a bound on the sender's queue does not make a block arrive inside
its deadline. Item 21's second claim is unchanged and still not held.

**THE TWO GAPS ARE BOTH ON THE ECONOMY ROWS, AND BOTH SAY "UNTESTED", NOT "BROKEN".** `ECONOMY=1`
was set deliberately — three of the five product commits live on the repair/audit path and
`11-economy-repair` is the only field row that exercises it, having skipped on BOTH prior runs, so a
default sheet would have re-confirmed every row except the ones the changed code is in. It converted
two silent skips into two named gaps. `11-economy-repair`: only 2 of 3 needed columns had
all-killable holders — shards landed on validators and caretakers the flow must not kill — so it
could not force a reconstruction without touching consensus; the row names its own remedy (dedicated
storage nodes, or a lower `-replication` to concentrate shards). `11b-economy-skim`: no skim grew
either armed observer's reserve above its prepay baseline, which needs the serve-accounting journals
attributed before a re-run rather than another drive.

*A DISCIPLINE FAILURE ON THIS RUN, RECORDED BECAUSE IT IS THE KIND THAT GOES UNNOTICED.*
`scenarios.sh` was edited at 13:01:20 and the first flow graded at 13:07:27, so the sheet was graded
by a harness that changed after the run started. It is harmless here — the edit adds a `require_nodes`
guard that is a no-op on a topology carrying `store-2`, and `8-takedown` passed on the same assertion
as before — but "it happened to be harmless" is the reasoning this list has twice paid for. It also
exposed a real defect: `gen_report.sh` resolved both provenance fields from live HEAD at report time,
so this run's report header named `a0ff08a` for a fleet built at `d531fbf`, with the run id the only
honest field on the page. Both stamps are now written when the input is consumed, and the report
carries a banner when the grading files change under a live sheet.

---

## The order of work, and why this order

Set 2026-09-19, after the owner ruled that every open defect on this list is BUILT, MEASURED AND
VALIDATED rather than disclosed. The ordering rule is how directly a piece converts into evidence,
not how large it is.

**0 — ~~A cloud run at HEAD, before any of the four below.~~ DONE 2026-09-19, run `d531fbf-90914`.**
Twenty-eight commits, three of them product fixes on the audit path and one on consensus, had landed
since the last full cloud run, so the held rows that lean on it leaned on a binary that no longer
existed. It was not a fifth fix; it was the precondition that keeps the next four attributable, and
it was driven before any of them. **23 pass · 2 gap · 1 fail · 5 skip** — nothing regressed,
`21-cross-region-cold-fetch` moved gap→pass, the one fail is the known impairment wedge and the two
gaps are newly-visible economy rows that `ECONOMY=1` turned from skips into findings. The evidence
is under *What "held" currently rests on* above. (Billable runs are pre-authorised, and the harness
refuses to spend until a local proof command has exited zero — it ran the e2e repair-bounty,
publish/fetch and equivocation drills plus the transport suite before releasing the spend.)

**1 — ~~The repair judge's work is gated before it happens.~~ DONE 2026-09-19.** `handleRepairClaim`
did the registry lookup, the manifest fetch and the survivor walk before `cfg.RepairEconomy` was
ever tested, so a 110-byte unsigned claim cost a judge 4.7 MB on the SHIPPED DEFAULT. The gate
existed and was in the wrong place. Driven red at the measured figure, then green: **4,719,978 B →
0** with the economy-ON control unchanged at 9 shards. It turned out smaller than "narrow the work"
— `announceRepairQuorum` and `emitRepairClaim` are already gated on the same switch, so this was the
missing third gate in a set of two and an economy-off node has no honest claim traffic to protect.
The per-sender bound stays open and the change says so; the pin over it is still green at the same
number. Detail under item 10.

**2 — ~~The prover identity reaches the answering side.~~ DONE 2026-09-19.** Send the challenge BASE,
derive `porProverSeed(base, self)` at the holder, and an honest holder can no longer be used as an
oracle. Driven: the proxy went from `Passed:1` and 1000 credit minted to `Passed:0 Failed:1` and
slashed, with the refusal attributed on the holder's own counter. Item 10 is now FOUR of five.
Second because it is a real fix with a named shape, an in-tree precedent (`answerBondChallenge`
already takes `from`), and it takes item 10 to four of five. It touches an audit wire field, so it
carries a mixed-version story and a control that honest holders are not over-rejected — over-rejection
would pass the pin's fix-case while breaking every honest audit.

**3 — ~~Whether the bond proof has to ride the consensus critical path at all.~~ MEASURED
2026-09-19; the answer is below and it is not the one the reading predicted.** Driven on the
deterministic rate repro rather than in the field, in
`core/node/bondproof_criticalpath_measure_test.go`.

| question | measured |
|---|---|
| does retry de-duplication alone move the wedge? | **no.** One 1,572,864 B proposal to ONE peer at a quarter of the assumed floor offers **6,291,456 B** under the shipped `-request-retries 3` against 1 send with retries off — **4.0x**, and 18,874,368 B across the field's three attesters for one height and round. But the FIRST attempt already misses, so de-duplication changes what the ladder COSTS and not whether anything FITS |
| does the attester's queue hold the registration? | **split.** A peer's reg arrives by `MsgSubmitBondReg` and is queued: 1 of 1. The PROPOSER'S OWN is minted over `b.Prev` at propose time with no submit: **0 queued, 0 submits**, guarded on `minted=true` so the zeros are a missing submit and not a missing plot |
| what does the fallback cost? | **more than carrying.** Carrying is 24.000 s of wire against a 14.000 s deadline and MISSES; a digest-only proposal fits; but a reconstruction MISS costs the digest round trip PLUS the same 24.000 s, arming the same 14.000 s deadline that already missed, later in the round |

*SO COMPACT-BLOCK RELAY ALONE IS NOT THE WAY IN, and that is the correction to the reading.* The
sheet suspected the proposal bytes were separable from the committed bytes because every attester
already receives and verifies a peer's registration before the block carrying it is proposed. True —
for a PEER's registration. The one source the queue never holds is the proposer's OWN, and that is
the case a scheme built on the queue would stall on at every self-renewal and at bootstrap, where a
fresh reg is new to everyone.

*THE ROUTE THE NUMBERS POINT AT.* Pre-deliver EVERY registration including the proposer's own, so
the queue covers 100% of sources and the fallback is never taken; the digest then replaces bytes
that have already crossed, off the per-attempt deadline and out of the retry ladder — which is
exactly what build-immutable #5 asks for, large payloads off the critical path. The committed bytes
are unchanged, the signed hash is unchanged, nothing forks. **It is a transport change and needs no
era**, so the frozen-format rule does not push it past the date. Retry de-duplication is a separate,
additive 4x saving that fixes nothing by itself.

*AND THE PREMISE OF THAT ROUTE IS HALF WRONG, MEASURED 2026-09-19 BEFORE ANY OF IT WAS BUILT.*
`core/node/bondproof_predelivery_measure_test.go`. The reading above says the proposer's own
registration reaches no attester, so pre-delivery has to be BUILT. That is true of `RegisterBondReg`
read in isolation — which is what question 2 measured — and **false of the path**. `chainSyncTick`
calls `SubmitBondRenewal` on every sweep with no proposer exemption, so a bonded validator with a due
renewal broadcasts its own registration to every peer before it proposes. Driven rather than read:
**3 of 3 peers**, which is the whole peer set.

| measured | |
|---|---|
| is a registration a deterministic function of its prev? | **yes** — two mints over one prev are byte-identical at 1,514,986 B, and a different prev gives different bytes, so the nonce binds |
| does a bonded validator broadcast its OWN registration before proposing? | **yes — 3 of 3 peers**, with `BondRenewalDue` true |
| can an attester reconstruct the PROPOSAL's registration from what it was handed? | **only if the head held still.** Same prev: yes. One head later, the ordinary case: **no** |

*SO THE GAP IS ALIGNMENT, NOT DELIVERY, AND THAT MAKES THE FIX SMALLER AND DIFFERENTLY SHAPED.*
`SubmitBondRenewal` signs over the head at SWEEP time; the proposal is built after the reconcile
settles, on whatever head that leaves. The two coincide exactly when the head does not move in
between, and a live chain moves its head constantly — so today's pre-delivery hands every attester a
registration that is real, verified, queued, and **the wrong bytes for the block that follows it**.
Nothing has to be built to make the queue cover 100% of sources; it already does. What has to change
is that the proposer mint ONCE, over the prev it will actually build on, and deliver THAT — an
ordering change inside the propose path rather than a new delivery mechanism.

*THE ALIGNMENT HALF IS BUILT, 2026-09-19.* A proposer now embeds the registration it BROADCAST when
the chain still accepts it, and mints fresh only when it does not. Both are valid and both commit;
the difference is who else holds the bytes. The head window exists precisely so a registration
survives the head advancing under it — `TestBondRegStaleAfterOneHead_factorII` already proves a
one-head-stale reg validates AND commits — so nothing about validity had to change, only which copy
the proposer reaches for.

| | before | after |
|---|---|---|
| can an attester reconstruct the proposal's registration? | **no** | **yes** — the committed bytes are the queued bytes, 1,514,986 B, identical |
| propose-path cost of the node's own registration | 17.27 ms (a fresh space-time answer) | **187 µs** |

The second row is a floor-box saving nobody asked for and it is worth naming: a fresh registration
is not a lookup — it reads the seed block out of the plot, proves that leaf, evaluates the VDF and
signs the result, on the single serialized loop, inside the propose path. Re-minting when a valid
registration was already in hand spent that twice for one claim. The gate is `ValidateBondReg`, the
same check the block itself faces, so a kept copy can never outlive the window: the control drives a
stale one and asserts the proposer mints fresh rather than burning its turn on a block its own
validity rule would reject. Ablated red before the fix and green after.

*AND THE RELAY HALF IS BUILT ON TOP OF IT, 2026-09-19.* A proposer now sends the digest form to
peers it has evidence hold the proof, and the full form to everyone else. The block is UNCHANGED:
`Prune` drops `Answer` and keeps the `AnswerDigest` the v5 preimage folds in its place, so the shed
form HASHES IDENTICALLY — every attester signs the same hash whichever form reached it, the
committed bytes are the same bytes, nothing forks. **Transport, no era.**

*THE PROOF CROSSED TWICE PER ROUND, WHICH THE READING MISSED AND THE CODE SAID.* The proposal is not
the only message carrying the block: `prepareQCEnv.Raw` is the same `chain.Encode(b)`, so the
prepare-QC that opens the precommit leg ships the whole thing again. Shedding only the proposal
would have halved a cost paid twice — and reported half the saving as the whole of it.

| ONE attester, ONE round, ONE registration | carried | shed |
|---|---|---|
| the proposal | 1,515,272 B | **511 B** |
| the prepare-QC | 1,515,381 B | **620 B** |
| **round total** | **3,030,653 B** | **1,131 B — 2,680x** |

The precommit leg is also where the evidence is STRONGEST, and it is free exactly when the second
copy would otherwise be sent: a peer named in the prepare-QC PREPARED on this block — it received
it, reconstructed it, validated it and signed its hash. It cannot have done that without holding the
proof. That is a stronger claim than an acknowledgement.

*FOUR PROPERTIES, EACH WITH A CONTROL.* (1) Shedding does not move the block hash — asserted here
rather than inherited, because this is the use that depends on it. (2) A receiver rebuilds the
COMMITTED proof and refuses any other candidate: the digest is covered by the proposer's signature,
so only bytes whose sha256 matches it are substituted, and the negative arm drives a poisoned
pending queue. (3) A proposer sheds only on EVIDENCE — an acknowledgement for those exact bytes, or
the peer authored them — and a receipt dies with the bytes it is about, so a fresh broadcast clears
every prior one. (4) **The relay stops at the GATHER legs.** A shed block offered at COMMIT is not
stored, because a stored Answer-less registration can never be re-verified and trusting one is the
no-discount break the trust floor refuses. That last one guards a hazard nobody would hit today but
that a later "natural generalization" of a change saving 3 MB a round would walk straight into.

*A MISS IS A RECOVERY AND NEVER THE PLAN.* Question 3 priced the naive fallback ABOVE carrying, so a
receiver that cannot reconstruct answers `NeedBody` — a refusal that names its own remedy — and the
proposer re-sends to that ONE peer with the proofs carried, dropping its stale receipt. `NeedBody` is
deliberately distinct from a bare `OK=false`: reading a transport gap as a validity refusal is the
attribution failure that hid this wedge for three billable runs. Both sides count their misses and
both should read zero.

*WHAT TIER, SAID PLAINLY.* Unit and e2e. NOT driven under network impairment and NOT in the field —
and that is the tier that matters most here, because the wedge only appears on a link slower than
the deadline assumes. The relay is not in the SMOKE fleet either; that was built at the alignment
commit, before this existed.

*AND THE PREDICTION THAT WAS WRONG IS WORTH AS MUCH AS THE RESULT.* This is the second time in two
days that a route's stated premise did not survive being driven, and both times the reading was of a
FUNCTION where the behaviour lives in a PATH. A build that had started from the sheet would have
added a pre-delivery broadcast the sweep was already doing, left the alignment defect untouched, and
measured no improvement at all.

**4 — A zero-byte prover fails the audit.** ⚠ *the measurement #8 demands is DONE and it picks the
entry; the protocol is not built.* The largest piece and the one an outside adversary reaches first.
It started with a floor-box measurement of the candidate schemes' PRODUCTION cost rather than a
library choice, because build-immutable #8 disqualifies a mechanism on production cost however good
its output — `core/por/production_cost_floor_test.go`, 2026-09-19, detail under item 10. Hash-only
spot-checking is ~14-42x CHEAPER to produce than the scheme now shipping and a pairing scheme is
~170-500x dearer, so the entry is decided and what remains is the audit protocol itself.

**Then: what item 21's unheld second claim means for the date. (3) has reported, so this is now
decidable, and the answer is that the structural close is REACHABLE before 2026-09-27.**

The blocker everyone expected was the frozen-format immutable: if taking the proof off the critical
path were a widening of what a block may contain, it would be a NEW ERA and would not happen before
the date. The measurement says it is not. The committed bytes, the validity rule and the signed hash
are all unchanged under pre-delivery plus digest relay — only what the PROPOSAL MESSAGE carries
moves, which is transport. `Prune()` already drops `Answer` from a finalized v5 block and the hash
still reproduces, because `v5PreimageBondRegs` folds `AnswerDigest` in its place, so the format
already expresses a proof-less body.

What remains is ordinary engineering with a named shape and a deterministic repro beneath it, which
is the position every other closed item on this list reached before it closed. It is sequenced AFTER
item 10's last defeat, because that one is the piece an outside adversary reaches first and this one
is a liveness bound on an adverse network rather than a claim the red team returns a verdict on.

*WHAT DOES NOT CHANGE — AND THE PART THAT DID, 2026-09-19.* Item 21's second claim is NOT held
today. The route was BUILT rather than measured: the proof rides by digest to peers with a receipt,
3,030,653 B -> 1,131 B per attester per round, hash unchanged, no era. What was missing was the only
tier that can hold the claim, a DRIVE UNDER IMPAIRMENT — the wedge exists on a link slower than the
deadline assumes, and unit and e2e do not run on such a link.

*THE SEATS WERE MADE NON-PREEMPTIBLE, AND THEN THE READING WAS TAKEN.* `21-impaired-commit` shapes
every validator seat and holds it for the whole 660 s drive. On run `5ad8344-49291` it returned no
reading at all: GCP PREEMPTED a shaped seat mid-drive, and because that sheet ran SPOT with
`instanceTerminationAction=DELETE` the instance was removed rather than stopped. The harness now
attributes a vanished seat as a GAP naming the lost measurement instead of declaring the sheet
untrustworthy, and the core (validator + registry) now goes on-demand whenever the impaired grade
will be driven — including under SMOKE, which is the sheet that lost it. Run `5af09a9-88230` then
drove it on four non-preemptible validator seats across three regions, and nothing vanished.

*AND THE READING REFUTES THE ROUTE RATHER THAN CLOSING IT.* **796 s without a commit at HEAD with
the relay firing** — the fourth reproduction of the same shape, and the detail is under item 21. The
relay's coverage is **1 of n−1 attesters by construction**, because a block carries another
validator's registration and only its author can be proven to hold the proof; the acknowledgement
path that would lift it is cleared every 30 s by a re-broadcast of bytes that did not change; and
that same re-broadcast — 29 of them in the window — saturates the per-peer outbound budget until it
drops the consensus frames that would clear the stall. Three findings, one root:
`SubmitBondRenewal` cannot tell a renewal that has not been broadcast from one that has been
broadcast and is still waiting to commit.

*AND THE ROOT IS BUILT, 2026-09-20.* A renewal is broadcast ONCE and remembered until it commits:
the copy stands while the head it was minted over is still the head, only peers without a receipt
are sent anything, and `MsgSubmitBondRegAck` reports that the bytes are HELD rather than that the
message arrived. Per sweep with the chain committing nothing, **[3 3 3 3 3 3] before, [3 0 0 0 0 0]
after**, with five unit properties red-before-green-after and each carrying its own control — the
first sweep must still reach every peer, and a peer whose submit was dropped must still be retried,
because a traffic fix that simply sends less would pass a no-traffic assertion while lapsing the
validator. The first version of the keep-rule gated on `ValidateBondReg` alone and
`sim.TestObjectiveBondRenewalSustainsAttestOnlyValidator` stalled at round 5; the sim tier caught a
liveness regression every unit assertion had passed.

*AND THE RE-DRIVE HAS REPORTED, 2026-09-20, run `7eaf3bd-75421`.* The renewal fix holds in the field:
**29 submits inside the impaired window became 1**, relay coverage went **17% → 37.5%**, and a height
shed to every peer for the first time since bootstrap. The chain still wedged, at **814 s**, on the
**15 prepare legs that still carry ~1,574,000 B** — which is the 1-of-n−1 coverage limit with the
amplifier removed from in front of it, and therefore the cleanest statement of the remaining work
this item has ever had.

The re-drive also turned up a defect neither previous run could see, because the renewal storm was
masking it: **the outbound bound is fair between peers and blind between frame kinds.** 20 of the 23
dropped frames were 121-byte chain-sync probes and window requests, and all four validators reported
`max-peer-head=0 probe-fails=4` simultaneously — blind to each other, by their own attribution, while
1.57 MB proposal retries held the budget. A node that cannot send a 121-byte probe cannot discover it
is behind, cannot fetch the window that would catch it up, and cannot refuse fast either.

So the item ships DISCLOSED, and TWO independent pieces of work are named rather than one. Lifting
coverage to every attester takes the heavy bytes off the critical path. Reserving a share of the
outbound budget for small control frames keeps a congested link from blinding the node trying to
recover on it. Neither subsumes the other; the second would bite on any congested link even if the
first landed tomorrow. Both are ordinary engineering with field evidence behind them, and neither is
an era.

**Unsequenced, and it needs the owner's call against the four above.** Six items on this list carry no
verdict — 11, 12, 13, 17, 18 and 19 — and two of those silences are sharper than the rest: item 11's
own done-condition names a live defect (published metadata leaks the exact plaintext byte count, which
makes the padding defence a no-op), and item 19's suite sits in the opt-in tier untested while item 20
ships disclosed on an era boundary whose stall behaviour has never been driven. Item 18's seizure
drill is cheap and answers the content-blind immutable for an outsider. None of them are ordered here,
because trading them against the four fixes above is not a builder's call to make silently.

---

## What this list deliberately does not cover

Blob-layer unobservability against a global passive adversary — the vision says outright that
silt does not promise the impossible. The full multiplicative Sybil interlock: the vision
calls it the target and not yet the guarantee, and a destination that concedes a gap cannot
be used to manufacture a gate it does not claim. Throughput numbers — bounded first, fast
second. Conformance across implementations, when there is one. Ergonomics, SDKs and
embeddability. Cross-cloud field runs: the harness is a scaffold that stops at its first
unbuilt phase, and finishing it costs days that Tier A needs.

Two limits worth carrying in writing rather than by implication. The non-globality metric and
the diversity axis both rest on **self-declared** operator domains, so both are claims about
declared diversity. And the profitable-edge commitment is about a trajectory at network
scale; any single run measures one point on it.

## Known open, carried rather than closed

**A pruned block's body is not bound to its hash.** For a pruned block, the hash is a
stored linkage token rather than a content commitment, so an adversary can keep the token and
the real signatures while rewriting the body — including the carrier that seats validators.
The only defence is the first non-pruned descendant, whose signed state root is recomputed
over the rewritten ancestor state, and the consequence is a silent head truncation at that
descendant with the forged seating live in the replayed state. Bounded, not eliminated.

*It stopped being only a carried residual on 2026-09-15.* It is now the blocker on item 20: the
owner ruled that a floor box whose verdict COUNTS must not be flipped on while a block's body can
be rewritten under its hash. Closing it means the block carrying a commitment that SURVIVES
pruning — a format change, so a new era rather than a validity tightening — which does not happen
before the date. Item 20 therefore ships disclosed, and this residual is the reason.

**The bonded set is capped by bandwidth.** Standing lapses after a short window and renewal
runs at half of it, so each validator republishes a multi-megabyte possession proof every few
minutes, and every other validator must receive and verify all of it. Each node ingests the
set size times that volume. This is live traffic, so retention policy does not touch it, and
it caps the practical bonded set well below the intended participant count.

*THIS STOPPED BEING ONLY A SCALE CAP ON 2026-09-18.* Item 21's second half attributes a chain
wedge to this same traffic at FOUR validators: the proof rides the consensus critical path, so a
link that cannot carry 1.5 MB inside the per-attempt deadline cannot commit a height at all, and
the retry ladder re-ships the proof into the congestion it is recovering from. The residual is
therefore a LIVENESS bound on the adverse internet, not only a participant-count bound on a
healthy one. The same traffic is also what made the outbound frame queue found beside it
expensive; that queue is now BOUNDED, which removes the OOM without touching the wedge. See item 21
for the evidence and the structural close.

*AND THE FIELD NOW SHOWS THE TRAFFIC IS SELF-SUSTAINING UNDER A STALL (run `5af09a9-88230`).*
`BondRenewalDue` stays true until a block COMMITS the renewal, so a chain that has stopped
committing re-broadcasts the full proof to every peer on every sweep — 29 times in one 10-minute
window, one per 30 s `ChainSyncInterval`. The traffic this residual describes is therefore not
merely periodic: on a wedged chain it is a positive feedback loop, and it fills the per-peer
outbound budget until consensus frames are dropped outright. The residual's cap on the bonded set
now has a second term — the renewal rate a STALLED chain emits, which is set by the sweep interval
and not by the TTL.

## Tenets that could not be reduced to a demonstration

Legibility. The hexagonal core and the single lock-free loop — architectural constraints
asserted by structure, whose consequence is testable but whose violation would not
necessarily show. "Never reinvent a primitive," a negative over all future choices.
Reactive-not-eager, which states no threshold. Canon-tracks-behaviour and
throwaway-stays-throwaway, which are disciplines rather than properties of a running system.
