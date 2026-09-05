# R2.9a — four residuals: the dead bond stamp, the untagged anchors, the M0 mislanding, the owner block

- **Date:** 2026-09-05 · **Seat:** BUILDER · **Branch:** `builder/r2.9a-residuals`
- **Base:** `origin/main` = `2d25b79` (PR #737 merged)
- **Inputs of record:**
  - `/Users/andrewedmond/Claude/claude/silt-reviews/research/research-outcome/R2.9a-DONT3-READING-AND-BOND-STAMP-TUPLE-RESEARCH-CERTIFICATION-2026-09-05.md`
    — Q2, **G-BB-28** (delete the stamp, invert one test, pin retention). Ratified under
    `D-DONT3-READING` (`docs/decisions.md`).
  - `/Users/andrewedmond/Claude/claude/silt-reviews/research/research-outcome/R2.9a-instrument-necessity-geometry-bound-and-tail-merging-RESEARCH-CERTIFICATION-2026-09-05.md`
    — Q1, **G-BB-22** (the M0 framing is refuted), **G-BB-21** (`W` withdrawn), **G-BB-23**
    (bin count is the owner's), §2.1 (the 134× provisional).
  - The reviewer's F4 on PR #737 and the two builder flags before it: the untagged R2.9a
    tests carry load and nothing asserts they exist by name.
- **Not decided here:** nothing. All four items are certified or owner-ratified. `q`, `P`,
  the bin count, `R-BB-SIBLING-AGGREGATES` and the `/api/library` link key stay the owner's
  and are written down as such.

---

## 0. The four mechanisms, stated before the change (build-immutable #6)

**1. The stamp.** `RecordBondChallenge` wrote `account.firstSeenTick` from the auditor's
tick, which is a wall-clock nanosecond. Reader inventory from the source at `2d25b79`, not
the comments:

| Reader | Line | Reads |
|---|---|---|
| `RecordBondChallenge` | `core/credit/credit.go:485-487` | the unset guard on its OWN write (`if a.firstSeenTick == 0`) |
| `DecayStale` | `core/credit/credit.go:555-559` | `lastBondTick` only |
| `Reputation` | `core/credit/credit.go:596-603` | `bondedBytes`, `auditsFailed`, `bondFails`, `falseRepairs`, `equivocations` — neither tick |
| the census, tag+flag | `core/credit/bbootstrap.go` (`stampFirstFetch`, `bBootstrapSnapshot`) | `firstFetchTick` only (since #737) |
| tests | `r29a_build_tag_test.go:57,91,101`, `r29a_stamp_by_fetch_test.go:131` | the field directly |

Whole-tree grep for `firstSeenTick` outside tests and comments: the write site and nothing
else. So the failure is a retained `when` with no reader, which T-DONT3 prong (a) names as
SURPLUS, **because** the write predated the census and was defended as "something else's
mechanism" when no mechanism read it. The change addresses it **by** deleting the write and
the field; `lastBondTick` is not touched, and its nanosecond unit is pinned because
`DecayStale` compares it against `BondMaxAge = 300 * ports.Second`.

**2. The anchors.** `go test` reports a renamed or deleted test as nothing and a `t.Skip` as
a pass. The tagged CI job asserts twelve named anchors; the default-build `go` job asserts
none. The untagged R2.9a gates (`!bbootstrap` files and untagged files in `core/credit`,
`core/node`, `cmd/silt`) are what keep the flag, the census reader and the serve-path stamp
out of a default build, and the F2 gates keep the withheld counter off the whole surface.
The change gives the `go` job the same step: eighteen named anchors, no skip.

**3. The mislanding.** Three shipped comments said the too-high direction of `grant/r`
"cheapens Sybil bootstrap, M0". M0's Sybil corner is about STANDING (`docs/TENETS.md`
Part 0); `Register` mints BALANCE; `RecordBondChallenge` is the sole standing press
(Invariant A, `core/credit/invariant_a_test.go`); free bytes build no bond. The true
landings: build-immutable #4 from below; Don't #7 / T-AR / build-immutable #8 from above.

**4. The owner block.** ROADMAP item 12 still asked for `W`, framed G-BB-9 as an M0 trade,
and did not name the bin count, `R-BB-SIBLING-AGGREGATES`, the `/api/library` link key, the
134× provisional, or the three ratifications of the week.

## 1. Options weighed

| Item | Options | Taken | Why |
|---|---|---|---|
| 1 | (a) delete write + field; (b) delete write, keep field; (c) keep both, gate the reader | **(a)** | (b) leaves a field that invites a re-write; (c) is the status quo the certification refuted. Zero behavioural cost measured by the four reverts below. |
| 1, the inversion | (a) keep the old name, flip the assertion; (b) rename to what it asserts | **(b)** | "StillStamps" green on a test that asserts NO stamp is the CHANGELOG-rename trap (memory). Old name goes to the cited-tests allowlist as (H) HISTORICAL with the reason. |
| 1, the structural half | (a) grep the source for `firstSeenTick`; (b) reflect over the `account` type | **(b)** | a source gate is a text match and needs a labelled runtime cover (`check_source_gates.py`); the type is the artifact. |
| 2 | (a) inline list in the `go` job, mirroring the tagged job; (b) one shared script both jobs call; (c) derive the list from `func TestR29a` in untagged files | **(a)** | (c) is self-referential: a rename updates the derived list, so it never fails — the exact defect. (b) re-plumbs a required check for no gain today. |
| 3, the third site | the certification cites `cmd/silt/ui.go:416` at `4e67b5d`; that block carried no M0 sentence at that commit and has since moved to `cmd/silt/bbootstrap.go` | add the correct landing there | the block names the `grant/r` purpose without saying where it lands; one sentence stops the next reader inheriting the error. Reported as a discrepancy, not hidden. |
| 3, the fifth sites | ROADMAP item 12's G-BB-9 sentence; two lines of the dated 2026-09-04 PACE record | rewrite the ROADMAP; dated inline corrections in the PACE record | `docs/thinking/` is frozen history to the lints; a silent rewrite would falsify the record. |

## 2. Ablations run (each deletion and each gate)

| # | Revert | Result |
|---|---|---|
| A1 | rename `TestR29aDefaultBuildHasNoBBootstrapFlag` | untagged anchor step **RED**: `did not PASS in the DEFAULT build` |
| A2 | `t.Skip` in `TestR29aWalltimeAdapterIsAWallClock` | untagged anchor step **RED** on the named anchor and on `--- SKIP` |
| B1 | restore `firstSeenTick` and its write in `RecordBondChallenge` | `TestR29aBondChallengeStampsNoFirstTouch` **RED**: `account has a field "firstSeenTick"` |
| B2 | widen the deletion into `a.lastBondTick = tick` | `TestR29aRetentionReadsLastBondTickInNanoseconds`, `TestR29aBondChallengeStampsNoFirstTouch`, `TestStandingMustBeSustainedToKeepVoting` **RED** |
| B3 | store `lastBondTick` in seconds | the same three **RED** (the exactly-`BondMaxAge` arm fires) |
| B4 | `DecayStale` never retires | `TestR29aRetentionReadsLastBondTickInNanoseconds`, `TestStandingMustBeSustainedToKeepVoting`, `TestInvariantA_TheOnlyMintingPressIsBondGated` **RED** |

`TestR29aBondAuditStampsAWallClockNanosecondNotACounter` (`core/node`) stayed green with no
code change: it captures the tick the auditor passes, not the stored field. Its doc block
now says the ledger keeps that tick as `lastBondTick`.

## 3. What I believe is wrong in the brief, with evidence

- The brief calls the `r29a_build_tag_test.go:55-56` comment "the third wrong comment". At
  `2d25b79` the sentence is at `:66-67` ("a future change that removes it is removing
  something else's mechanism"). Same sentence, moved by the #737 fold-in.
- The certification's `cmd/silt/ui.go:416` site carried no M0 sentence at `4e67b5d`
  (`git grep` for `Sybil|M0` in that file at that commit: two unrelated hits at `:633`,
  `:950`). The "four texts" in the repo were two `core/credit/bbootstrap.go` comments plus
  the ROADMAP G-BB-9 sentence and the dated PACE record; the researcher's three
  certifications are the rest and are not touched.

---

## 4. Round 2 — the blind review's two blockers and two stale claims (2026-09-05)

**Input of record:**
`/Users/andrewedmond/Claude/claude/silt-reviews/principle-engineer/RULING-R2.9a-four-residuals-2026-09-05.md`
— MERGE WITH TWO FIXES; every finding is a sentence that claims more than the code does.

### 4.1 Mechanisms, stated before the change

| Item | The failure is … | because … | this change addresses it by … |
|---|---|---|---|
| Blocker 1 | three texts say the structural gate stops the stamp returning "under another name" | the predicate was `strings.Contains(lower(name), "firstseen")` — a name match; the reviewer's `bondSeenTick uint64` with the identical unset-guarded write kept `core/credit` and the anchor step green | the gate now whitelists the `account` type's TICK-TYPED field set (`uint64`, `ports.Time`, `ports.Duration`) as exactly `{firstFetchTick, lastBondTick}`, both present; the three sentences say that |
| Blocker 2 | the owner block files `G-BB-13′` under "no longer owed" and omits `G-BB-12′` | the DONT3 cert §6 carries G-BB-13′ open, moot for the DEFAULT build only, and every `B_bootstrap` run is a TAGGED run; G-BB-12′ is required and unbuilt (`isLocalHost` reads the client-controlled `Host` header; the token gate covers `isMutating` only, `cmd/silt/ui.go`) | two OPEN lines, scoped: G-BB-13′ tagged-only; G-BB-12′ required-before-any-run, unbuilt, builder's |
| Stale 1 | `ROADMAP.md` calls the tagged CI job advisory | true when written; the `main` ruleset (`Protect main and staging`, id 19729396) now requires five contexts and the `bbootstrap` job is the fifth — ruleset `updated_at` 2026-09-05T10:22:28Z, PR #736 `mergedAt` 10:22:12Z | the sentence says REQUIRED and since when, and why it matters (the reviewer's Ablation G: a non-literal flag name leaves 0 `nm` symbols and a green AST gate; only the `-help` grep catches it) |
| Stale 2 | `bondaudit.go`'s `+1` says "so the first tick is never 0 (unset)" | at 9eaad7b (the bond ledger's first commit) the `+1`'s only consumer was `if a.firstSeenTick == 0 { a.firstSeenTick = tick }` — the guard this PR deleted; today `lastBondTick`'s sole reader is `DecayStale`, whose only test is `now − lastBondTick > maxAge`, and `bondedBytes > 0` is set only on the line that also sets `lastBondTick` | the comment names the dead consumer and says the `+1` serves no reader; the VALUE is untouched (retention; `TestR29aBondAuditStampsAWallClockNanosecondNotACounter` pins `t0+1`) |
| Follow-on 6 | G-BB-29 is neither applied nor recorded | the DONT3 cert §1.5 / §3 issues it to the Builder as a doc-only narrowing: "recording SURPLUS is the break", because "recording per se" condemns `delivery.go`'s D-S7 `provKey` | applied: an appended (not substituted) correction on `D-BB-BUILD-TAG`, and the `core/credit/bbootstrap.go` header |

### 4.2 Options weighed

| Item | Options | Taken | Why |
|---|---|---|---|
| Blocker 1 | (a) narrow the three sentences to "the same field name cannot return"; (b) whitelist the `uint64` fields; (c) whitelist the ENTIRE field list of `account` | **(b)**, plus `ports.Time`/`ports.Duration` as tick types | (a) ships a weaker gate to match a weaker claim, the wrong direction for a struct that IS the per-identity record. (c) catches any field of any type but taxes every legitimate `account` change; not traceable to a failure today. On this struct every `uint64` is a clock reading and every byte/count is `int64`/`int`, so type is the discriminator; the two `ports` clock types cost two lines. Honest limit, stated in the gate: a `when` as a bare `int64` under a byte-shaped name is not seen. |
| Blocker 2 | (a) one OPEN line for G-BB-13′ only; (b) both lines | **(b)** | G-BB-12′ gates the run; a "run preconditions" block that omits a required, unbuilt precondition is the same defect as the one it corrects. Marked as the builder's so the block stays honest about ownership. |
| Stale 2 | (a) delete the `+1`; (b) keep it and re-state; (c) keep it and say it serves nothing | **(c)** | (a) moves the stored tick — retention is not this change's, and a test pins the value. (b) "kept so a tick is never confused with a zero clock" would be a NEW reason with no reader either — the exact comment family this PR exists to stop. |
| Follow-ons 1–3 (behavioural) | build now / record open | **record open** in the R2.9a Rock | out of the brief; each is a CI/test hardening with a measured gap, and none changes the merge verdict. |

### 4.3 Ablations (each RED with the field name in the message, then restored and green)

| # | Revert | Result |
|---|---|---|
| F | `bondSeenTick uint64` + `if a.bondSeenTick == 0 { a.bondSeenTick = tick }` in `RecordBondChallenge` (the reviewer's) | `TestR29aBondChallengeStampsNoFirstTouch` **RED**: `account has a tick-typed field "bondSeenTick" (uint64) outside the closed set` |
| F2 | `bondSeenAt ports.Time` with the same guarded write | **RED**: `"bondSeenAt" (ports.Time) outside the closed set` |
| F3 | whitelist drift (`lastBondTick` → `lastBondTickX` in the gate's set only) | **RED** — the set and the struct disagree loudly in either direction; the whitelist cannot go vacuous by a quiet edit |

### 4.4 What I believe is wrong or incomplete in the ruling

- Nothing in the four findings. One scope note on §5's first coupling: the `+1` was not "half of
  the same mechanism" left behind by THIS change — it predates the stamp's R2.9a history by a
  month (9eaad7b, 2026-08-01) and its consumer was the ORIGINAL `firstSeenTick` guard from that
  same commit. The ruling's conclusion (the reason outlived its mechanism) stands; the
  attribution to this PR's deletion is right in effect and wrong in origin. Recorded so the
  next reader does not go looking for a second deletion.
- §3.1 calls the tagged job "advisory, not yet a required check" citing `ROADMAP.md:997`. The
  ROADMAP was stale, not the ruleset; the job has been required since 10:22:28Z on 2026-09-05.
