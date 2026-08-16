# 2026-08-16 — record correction: the everMature latch DID trip on run 09fbe60-84613; the drain-obs rescope

## The deliberation (pace-before-code)

The serial follow-up queue's item 1 was "promote the drain-curve C2 line to info under
MATURING" — premised on the confirm-run record's OPEN item (a): *"the decisive
drain-curve C2 lines are debug-level and the run was -log info"*. Build-immutable #7
says cite the artifact before building, so the first step was reading the emit site —
and the premise is **false**. This doc records what the evidence actually shows, what
was rescoped, and why.

## Finding 1 — the latch TRIPPED. The "not tripped" record entry was a measurement error.

**Evidence (committed at c56617b, `integration/cloudtest/flow-evidence-09fbe60-84613.log`):**
the per-commit C2 status line is an **unconditional stdout `fmt.Printf`** in the daemon's
`OnCommit` (`cmd/silt/daemon.go`), captured by journald at every log level — it was never
debug-gated. The captured journals contain 133 C2 lines, and at **23:17:32Z** they flip to:

```
C2: nakamoto 2 bonds → 2 operators (margin ×1) | cost-to-corrupt 65 MiB of 195 MiB
bonded across 6 | … | wheels shed permanently (network matured — F-1 one-way latch)
```

on the adversary node, val-b, **and** val-c (195 MiB = 3×64 MiB maturers + 3×1 MiB
sybils; nakamoto 2 ≥ bar 2). `Chain.EverMature()` is a pure function of the committed
blocks, and val-a committed the same heights via broadcast (its debug.log shows
`block committed … via=broadcast` for h42–45), so val-a's journald necessarily said the
same — **the network matured on the wire, ~13 minutes into the run.**

**How the wrong conclusion happened:** the live capture during the interrupted run pulled
val-a's **debug.log** (`dlog`) — where the structured `n.logf` lines live — and the
C2/wheels banner **never appears there**; it is a stdout banner that only journald
(`jlog`) sees. That is precisely the jlog/dlog split documented in
`integration/cloudtest/lib.sh` (the #310 trap: "a #310 assertion greped journald for the
standing line and could never match"). The harness's `capture_flow_evidence` already
captures BOTH logs; the ad-hoc session capture did not.

**Consequences:** flow-10's latch check (`waitfor val-a 'wheels shed permanently'`) would
have PASSED had the run survived to flow 10 — the line was on every subsequent commit's
C2 banner. The follow-up run's flow-10 goal stands (formal grade + handoff + B2 drills),
but "will the latch trip at all" is no longer open.

## Finding 2 — "blocks committing empty" was a misread; the commit lines don't show regs.

The drain read ("13 blocks committed but only 4 with entries=1, 9 EMPTY"; later
"entries=0 at h42–45") interpreted `entries=` as block content. **Bond registrations are
not entries** — `Block.BondRegs` is a separate field, and none of the three commit lines
(daemon banner, node `block committed` via=proposal / via=broadcast) printed its count.
val-b's journal shows `bond-reg drain: pending registrations committed height=45` while
val-a logged h45 as `entries=0` — regs were landing invisibly. **Obs fix shipped in this
PR:** all three commit lines now print the reg count. That is the drain curve's per-block
resolution, and it is what the follow-up run's flow-10 read needs.

## Finding 3 — the refusal census: 54 "signature" refusals are AHEAD-skew, mechanism pinned.

Census over the captured journals: **54 `bond-reg submit REFUSED` lines, every single one
`…: validator <id> signature`**, across ≥7 distinct honest validators — 37 at 64 MiB
(maturers), 17 at 1 MiB (sybils). Not forgery, and not the #427 behind-staleness either:

- A renewal is signed over the **submitter's** head (`SubmitBondRenewal` →
  `RegisterBondReg(head)`).
- A receiver validates against `recentBondRegNonces` — nonces of its **own last-K
  COMMITTED heads**. That window walks committed ancestry; it **cannot contain a head the
  receiver has not committed yet.**
- Under WAN commit-broadcast skew the submitter is routinely one head ahead of some
  receiver, so the reg fails every window nonce, and `validateBondRegWindow` surfaces the
  head-nonce error — a bare "signature".
- It **heals without resigning**: the moment the receiver commits that head, the same
  bytes validate; the next resubmit sweep (30 s) re-delivers.

Pinned by `TestBondRegAheadOfReceiverWindow_refusedThenHeals`
(`core/chain/bondreg_staleness_test.go`): refused while the receiver trails, valid after
it commits the head. The obs half shipped here: the refusal line logs the receiver's
`next_height`, and the submit side logs `signed_next_height`, so a field read correlates
the two instead of reading attack.

**Deliberately NOT built:** accepting ahead-signed regs into pending under deferred
validation would remove the retry tax but changes the submit-acceptance rule and opens an
unvalidated-64 MiB-queue DoS surface — consensus-adjacent, research-gated. The current
refuse-and-reheal is correct, just noisy; it is now attributable noise.

## Finding 4 — the renewal-crowding hypothesis is REFUTED.

Queue item 2 hypothesized TTL renewals crowding first-time maturer regs out of the byte
budget. The drain curve says otherwise: first-time maturer regs banked steadily (65 MiB
bonded by 23:08, 131 by 23:14, 195 by 23:17 — latch). Renewals and first-timers were not
in byte-budget contention; blocks were far below the 2 MiB cap throughout.

## Still open (named, routed — not assumed benign)

1. **Every height h42–h46 committed only after a round-change to r1** (all four
   validators' captures agree; new-view `forced=false` at r1, i.e. r0's prepare-QC never
   formed). Slow-but-live — ~2× block cadence at steady state. UNATTRIBUTED. Routed to
   the node-level mature-epoch fixture (serial queue item 5), which is the deterministic
   home where a mature-regime schedule can reproduce r0's systematic failure. Consensus
   discipline rule 7: flagged, not rationalized.
2. **The 4th maturer + 4th sybil had not seated by capture end** (`bonded across 6` from
   23:17 through 23:22+; pending 8–10 network-wide, dropping ~2 at h45). The B2 drills'
   seated gate needs all 8. With finding-3's mechanism and the regs= line, the follow-up
   run can attribute seating pace directly; if it under-runs the computed full-drain
   bound, that is a real finding, not a window artifact.
3. **strand-(a)** (fetch-1 missed the 240 s warm bound) — unchanged, still the follow-up
   run's second grade.

## Process lesson (owned)

Two of the four "open items / next steps" in the overnight record were artifacts of
reading the wrong log stream or a lossy log line — and both survived into the plan as
build tasks. The corrective was cheap: read the emit site before building the "fix", and
census the already-captured evidence before hypothesizing. Both are build-immutable #7
verbatim. The genuinely load-bearing obs gap (regs= on commit lines) was found only by
that census.
