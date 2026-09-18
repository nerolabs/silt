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

*THE SAME PARAGRAPH IS ALSO IN `README.md`, AND IT IS NOT REMOVED.* The repo's front door says
"This multiplicative interlock is the target, not yet the operative guarantee. Today consensus
standing is gated by the bond axis alone." That is build status in the first document any reader
opens — including the grader, who gets the repository and nothing else — and it is now imprecise:
the census shows standing gated by the bond axis AND ITS TIME DIMENSION, because retention decay is
wired and denying. It is left standing because `README.md` sentences are PUBLISHED CLAIMS, pinned as
such by `canon_text_test.go`, whose own text says the wording must not be changed to make a gate
pass. Changing it is the owner's call, and there are three options: remove it as the `VISION.md`
paragraph was removed; correct "the bond axis alone" to "the bond axis and its time dimension" and
keep the disclosure; or leave it and accept that the front door hands the grader a partial map of
where the composition is weakest.

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

**9. Consensus denials hold, and no honest node is ever slashed.** Equivocation attributed;
forged and under-bonded proposals rejected pre-attestation; a partition heals to one order.
The honest-never-slashed property asserted over the whole run's slash set, not per attack.
*model-check → e2e → field.*

**10. A prover without the bytes fails the audit and is paid nothing.** Three defeats: the
care link printed on every publish is the storage-proof verification key; a data-less
identity passes by relaying the challenge to a real holder; the inclusion proof is checked
against a root the prover supplies. Plus work bounds on the unsigned repair claim and the
challenge frame, neither of which is rate-limited.
*unit → integration → e2e under impairment → field.* Largest code item in scope.

**11. Publishing is unlinkable and no surveillance artifact exists.** A matching score
against chance; seize every disk and emitted byte after a fetch-heavy run and find no
(fetcher, content) pair; published metadata no longer leaks the exact plaintext byte count,
which today makes the padding defence a no-op.
*integration → e2e → field.*

**12. Takedown bites, cannot go global, and is provable.** Honouring operators stop serving
and others keep serving; no accepted operation removes more than one named root or works by
identity; every honoured removal carries inclusion and consistency proofs.
*integration → e2e → field.*

**13. Bit-perfect or an explicit failure, and crash recovery needs no human.** An unplaceable
publish names what could not be placed, returns no link, leaves no registry entry, and still
succeeds on retry. A validator killed mid-consensus re-pins from its own last finalized
checkpoint and never contradicts a signature it made before the crash.
*e2e under impairment → field.*

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

**17. The economy mints nothing, and the core squeeze is measured.** Balance-lane credit
never becomes consensus standing; a colluding pair strictly loses; repair is funded from the
object's own escrow; a false claim is slashed. Plus core-node net margin at two edge
populations against held-constant demand, reported as a signed number.
*unit → integration → e2e.*

**18. Core carries nothing.** Seize a holder and fail to recover known plaintext, with a
key-holder succeeding on the same objects in the same run; core resolves hashes, never names.
*unit → e2e.*

**19. An un-upgraded node stalls, and the network never updates itself.** A node that cannot
validate a shipped format stops rather than accepting it and says so while staying alive. A
node whose consensus-reaching configuration diverges from what the chain committed refuses to
start. No version-floor advisory below the signing threshold changes anything, and no node
ever replaces its own binary.
*integration → e2e.*

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

**21. Publish and fetch work on the internet as it is.** ⚠ *both halves are BUILT and reduced locally; two product defects closed on the path; the field tier is owed*
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

*Still owed: the field tier.* Neither flow has been driven on the cloud. `21-cross-region-cold-fetch`
cannot be rehearsed locally at all — the LOCAL backend excludes the natted nodes on purpose, so it
SKIPs there — and `21-impaired-commit` has been rehearsed only on one small box.

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

## Tenets that could not be reduced to a demonstration

Legibility. The hexagonal core and the single lock-free loop — architectural constraints
asserted by structure, whose consequence is testable but whose violation would not
necessarily show. "Never reinvent a primitive," a negative over all future choices.
Reactive-not-eager, which states no threshold. Canon-tracks-behaviour and
throwaway-stays-throwaway, which are disciplines rather than properties of a running system.
