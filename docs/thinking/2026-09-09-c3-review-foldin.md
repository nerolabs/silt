# C3 / R2.2 — folding in two blind reviews and a research certification

**Date:** 2026-09-09 · **Seat:** Builder · **Type:** BUILD deliberation (code ships in the
same PR) · **Base:** `f03ab50`, the build recorded in
`docs/thinking/2026-09-08-c3-r22-observability-build.md`

**What this round answers**
- PE (blind): `silt-reviews/principle-engineer/RULING-c3-r22-observability-f03ab50-2026-09-09.md`
  — MERGE-AFTER, four blockers, two simplicity items, five non-blocking findings.
- Red-team (blind, independent): `silt-reviews/red-team/REDTEAM-c3-gossip-disclosure-f03ab50-2026-09-09.md`
  — F1 confirms the PE's B3 and amplifies it three ways.
- Researcher: `silt-reviews/research/research-outcome/C3-GOSSIP-DISCLOSURE-vs-D-UI-PRIVACY-FLAG-RESEARCH-CERTIFICATION-2026-09-09.md`
  — the gossip half is **GATED**, certifiable only as alternative A, under M-1/M-2/M-3, and
  **the owner must ratify before it merges in any form**.

**Owner ruling folded in (2026-09-09):** the privacy default wins. Empty panels on the
shipped default are preferred over the disclosure.

---

## 1. The one I got wrong, and what the error actually was

The first cut left `/api/economy/concentration` and `/api/economy/network` open. My stated
reason, written into `r29a_status_surface_test.go` and §7 of the previous doc:

> "Self's own served bytes are inside the serve-Gini sample, so it is the FLOOR, not the
> aggregation, that makes these safe unauthenticated."

**False, and both blind seats produced a working solve.** The red team recovered a planted
7,777,777 exactly through the real `Node.EconomySample()`; the PE recovered 987,654,321 with
error 0.

The error was not "I picked the wrong threshold". It was a **category error about what the
floor bounds.** `minGossipSample` bounds the sample SIZE. The quantity that has to be bounded
is **the number of terms the reader does not already know**, and those are not the same thing
the moment identity is free — which is M0's own premise. An adversary furnishes n−1 terms with
sybils gossiping chosen values and a classifiable `CapTotal`; the published `(Gini, size)` is
then one equation in one unknown. I derived the floor against an honest-stranger sample and
never asked what an adversary who is IN the sample can do. That is the generalisable lesson:
**a privacy argument about an aggregate must state which terms the adversary supplies.**

Three amplifications from the red-team pass changed the shape of the fix, not just its size:

1. **The recovered term need not be self.** Any node with an open route is an ORACLE for its
   PEERS' withheld counters — a web client that never peers with V reads V's counter through
   A. So "drop self from the sample", the narrowest of the three fixes the PE offered, is not
   a fix at all; it moves the target.
2. **The counters are monotone**, so re-solving over time yields a per-node activity
   timeline, not a snapshot.
3. **`RepairGini` fingerprints the load-bearing repairers** — eclipse-targeting material.

I encoded amplification 1 as a measurement rather than taking it on the page: controlled
revert **H2** sets `SelfIncluded: false` in the reconstruction fixture and the gate stays
GREEN. That is the proof that a self-only fix would have passed every gate keyed on self, and
it is why the fix is at the route.

## 2. The shape chosen for the crux, and why it is minimal

Both documents now honour the **same privacy clause as the rest of the read surface** —
`auth.privacy && !auth.token`, byte for byte the predicate `readerView` uses for the node-wide
counters — under the **same `countersWithheld` marker**, because this is the same covered set
one derivation removed.

Why each neighbouring option is wrong:

| candidate | why not |
|---|---|
| withhold unconditionally | withholds a derivation of numbers a `-privacy=off` node publishes RAW two routes over. Closes nothing, and the disagreement invites a later edit to open the wrong one |
| withhold only the two Ginis | leaves the tier mix, which publishes the sample SIZE — half of the equation |
| drop self from the sample | refuted by amplification 1, and measured GREEN under revert H2 |
| token-gate both routes entirely | the predicate would then be stricter than the one protecting the SOURCE quantity, so a `-privacy=off` operator loses a panel for no gain |

**What stays open, and why each is not a withhold in name only:** the published bands and the
target ratio (constants, no measurement); `estimatedNodes` (the SAME number `/api/status`
already publishes in `network`, which the privacy clause does not touch — pinned by a gate so
the claim cannot rot); and the C2 block (chain-derived, committed-global, unforgeable via
gossip, and named an honest negative by the red-team pass).

**The cost, stated:** a cross-origin observatory can no longer read the two Ginis from a
`-privacy=on` node. The operator's own dashboard is unaffected — `app.js` attaches the bearer
token to every same-origin `/api/` call.

## 3. The gossip half: gated, and what alternative A cost

The Researcher's finding is sharper than "the same class of counter": the gossiped integers
are the **same account fields**, and they go to a wider audience over a more public port.
`-ui` is empty by default, so a hobbyist daemon with **no HTTP surface at all** was publishing
to every DHT peer — precisely the node `D-UI-PRIVACY-FLAG` was ratified to protect. And
`D-STATUS-SNAPSHOT-INTERVAL` ratified the 5 s snapshot as a **security parameter** on the
reasoning that "the poll rate is the reader's choice, so that was never a bound"; a counter
stamped on every reply hands the observer its own rate back and voids that bound.

Built to alternative A. `Config.PublishWorkCounters` gates the stamp, and **the zero value is
the withholding one on purpose** — a caller that forgets the field leaks nothing. `cmd/silt`
sets it from `-privacy`, which is now parsed before the node config.

### 3.1 M-2 needed more care than "drop the zeros"

Both wire fields are `omitempty`, so a withholding peer and an idle peer are **the same
bytes**. Counting the absence as a zero drives the serve Gini toward 1.0 — a false
total-capture reading on the shipped default. But the obvious fix, dropping every zero, would
have **destroyed the alarm**: repair concentrated on one of six capable nodes would collapse
to a one-element series and read as "sample too small".

**The discriminator is the PAIR, not the field.** A peer that serves but has never repaired
emits key 29 and not key 30; a withholding peer emits neither. So any peer with a positive
term is a REPORTING peer and its zero in the other field is a genuine measured zero that
counts. Only the all-zero peer is truly ambiguous — there "withholding" and "has done nothing
yet" really are the same bytes — and it is excluded, which understates inequality, the safe
direction for an alarm that reads "someone captured the work". Both properties are gated: the
withholding flip must not move the Gini toward 1, AND repair-on-one-of-six must still redden.

Each series now carries its OWN size against its own floor, over its own population, with the
population named in a `scope` string on the wire.

### 3.2 The restart reset hurts

Recorded because it inverts the intuition: `D-FP2-SCOPE`'s ephemeral ledger does not mitigate
this. It hands an observer a **known zero baseline**, removing the differencing step rather
than adding one, and via `omitempty` it makes every restart an **unforgeable beacon** to every
peer. It must be re-priced at the R2.4 economy-ON flip.

## 4. B1 — one rule, not two guards

The per-object path differenced from a root's first appearance and said why; the pooled path,
25 lines above, differenced two whole-sample totals. Measured: pooled `net 5000` while every
row it summed reported `net 0`.

I did not copy the guard across. **Pooled is now derived from the per-object rows** — the
window total is their sum, and a step takes a root's term only when the root is in both
samples. A pooled figure that is not the sum of the rows printed beneath it is a lie whatever
the arithmetic behind it, so the property is structural rather than a second guard that can
drift. Three directions gated as subtests (entry, departure, entry-does-not-mask-a-drain),
each RED under the revert, with a fourth arm that stays GREEN so the masking arm is not
vacuous.

## 5. B2 — two facts, one zero

`credit.Gini` returns 0 both for a measured equality and for a zero sum. `giniValue` now
carries `known`, the pattern `economyGRow` already used in the same file. The signature change
in `render.js` is the load-bearing part: the old call site was
`gossipCell(doc, doc.serveGini && doc.serveGini.value)`, and **a value of 0 is falsy** — the
one case that must not render was the one the old signature could not express.

The counter-arm is what makes this a distinction and not a blanket refusal to publish zeros:
eight nodes that each served the same real byte count still publish 0.

## 6. B4 — the caveat rides the number

`epoch` is published on `giniValue` and rendered in the panel's sub-line rather than kept in a
note. Placement is the decision: this is the same class of caveat as a sample size, so it
belongs beside the figure.

## 7. Controlled reverts run this round

| # | revert | reddens |
|---|---|---|
| G1 | pooled differences whole-sample totals | entry `net 5000 vs rows 0`; departure `-9000 vs 0`; masking `consecutiveNegative 2` |
| E2 | `giniOver` drops the zero-sum branch | `Known:true Value:0` over a sample that reported nothing |
| E3 | `render.js` ignores `known` | the cell renders `0.0000` |
| H1 | `gossipWithheld` returns false | `recovered 987654321 from the UNAUTHENTICATED document` |
| **H2** | **self dropped from the sample** | **GREEN — deliberately; the measurement behind §1's amplification 1** |
| M1a | `send` stamps regardless of posture | `gossiped ServedBytes=987654321` |
| M2a | a non-reporter counted as zero | `moved serveGini from 0.0000 to 0.8333` |
| M3a | `EconomySample` always reads `selfWork` | `serve series = 5 nodes totalling 991654321` |
| J1 | the epoch stamp dropped from the wire | `serveGini carries epoch ""` |
| J2 | the epoch dropped from the cell | sub-line without it |

## 8. Findings taken and not taken

**Taken:** N3 (the panels gate now drives `gossipCell` with 0 in both wire forms); N5 and the
`economyGNet` deletion (the simplicity ruling); N1's substance — the route-constant
examination text is rewritten, and it now states that the two existing whole-surface scans run
on the `-privacy=off` posture and therefore do NOT cover the privacy clause, naming the gates
that do.

**Not taken, and flagged rather than fixed:** N4 — the ring's resident cost is not the
magnitude the comment claimed. The `economyGNet`/`snap` merge halves it and the sentence is
corrected, but the figure is still unmeasured on the floor box. That is a build-immutable #8
measurement, not a code change, and it belongs to whoever prices it. **F2 (red-team, LOW):**
the Ginis are Sybil-steerable and share a route with the genuine committed C2. Nothing
consumes them today — the Researcher re-grepped and confirmed — and the standing constraint
is that a future monitor or C6 canary reading either Gini as a health signal is an `m0.md` §7
seam-1 regression and must be blocked at review.

## 9. What is still owed before this merges

The owner's ratification of the sentence in §7 of the research certification: that
`D-UI-PRIVACY-FLAG` is extended from a reader containment to a **disclosure** containment, so
`-privacy` governs these two counters on the wire as well as on the HTTP surface. Nothing here
should merge on the strength of this document.
