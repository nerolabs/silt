# 2026-09-09 — silt can see who is present, never who does the work

**Status: DECIDED 2026-09-09 — see `docs/decisions.md` `D-WORK-VISIBILITY`.** Alternative (4),
harness-only, is ratified for the RC; alternative (3), the committed-ledger route, is DEFERRED to as
late as possible before the cut, deliberately, because it is research territory that may spin while
well-defined work remains. The body below is the deliberation as written before the call, unedited.

**Status when written: an open decision for the owner, with the alternatives priced.** Not a build item. The
ratified position (`D-UI-PRIVACY-FLAG`, extended 2026-09-09) is the *starting* point of this
deliberation, not its conclusion: the extension was correct on the evidence, and it has a
consequence nobody chose deliberately because nobody had measured it yet.

## The fact

`cmd/silt/daemon.go` welds `PublishWorkCounters = !privacyOn`, and `-privacy` is the compiled
default in every build. So on the shipped default:

1. no node gossips `ServedBytes` or `RepairsDone`;
2. the certified M-2 rule excludes a peer that reports neither, so it is dropped from the series
   rather than counted as a zero (this rule is correct and closes a real false-capture reading);
3. therefore both concentration series are **empty** on a default fleet, and every gate over them
   returns INDETERMINATE by construction.

silt can still see **who is present** (the crowd estimate, the capacity pledge, the tier mix, the
committed `C2` standing metric). It cannot see **who does the work**.

## Who established what, independently

| Seat | Finding |
|---|---|
| Blind PE | The published aggregate reconstructs a withheld per-node counter: an attacker supplies n−1 of n sample terms with free identities and solves the last one exactly. Measured, error 0. |
| Red team | Independently confirmed the same break and amplified it: the recovered term need not be self (a peer oracle), the counters are monotone so resampling yields an activity rate over time, and the repair figure fingerprints the few load-bearing repairers (eclipse targeting). |
| Researcher | GATED. Only alternative A is certifiable (gossip under `-privacy=off`). Coarsening is REFUTED — a band leaks a monotone step sequence under probing, a rate publishes the derivative the attack must compute. Owner ratification required in every alternative. |
| Economist | Flagged the consequence as dominating every number in its own advisory: *"silt can see who is present and never who does the work… every concentration baseline is measured on an opted-out topology."* |
| Tester | Reached the same conclusion from the other end: on the default posture every one of its gates returns INDETERMINATE by construction, and its fixtures set the counters directly, so the gates cannot see this. |

Five seats, four of them blind to each other's reasoning, converging on one fact is the strongest
signal this process produces. Nothing here is disputed. The disagreement, if there is one, is only
about what to do.

## Why this matters beyond a dashboard

It is not a reporting inconvenience. Three things depend on measuring work concentration:

- **The R2.4 flip canary (ROADMAP C6)** aborts on concentration signals. On a default fleet those
  aborts are inoperative — the canary would pass by seeing nothing.
- **The tenet the edge tier is supposed to satisfy** (the pony tier does the MAJORITY of the work)
  is a claim about work, and no shipped surface measures it. The tier mix publishes a share of NODE
  COUNT, which reads ~0.99 under the vision ratio while the true byte share can be 0.20 — pinned as
  a substitution trap so nobody wires it to the floor.
- **The M0 §7 seam-1 lesson** was "compute the numerator from the committed ledger, not gossip". A
  gossip-derived concentration figure is a weaker object than the committed `C2` metric, and the two
  currently share a route and a name.

## The alternatives, with their costs

**(1) Keep it as ratified — privacy default wins, work invisible by default.**
Cost: the flip canary has no decentralization signal on a real fleet; the edge-majority tenet stays
unmeasured; concentration baselines describe an opted-out topology. Benefit: the strongest privacy
posture, and the one already certified and ratified. **This is the status quo and needs no action.**

**(2) Flip the publish default for work counters only, keeping the reader containment.**
Untie `PublishWorkCounters` from `-privacy` and default it ON, while the HTTP surface stays
withheld. Cost: re-opens exactly what the Researcher gated — the wire disclosure with its peer
oracle, activity timeline and repairer fingerprint — and would need a fresh certification. Benefit:
the series populate. **Not recommended: it undoes the ratification on the evidence that produced it.**

**(3) Measure work from the committed ledger instead of gossip.**
Concentration over quantities already committed on-chain, the way `C2` is computed. Cost: a design
and certification effort, and the committed ledger may not carry per-node serve work at the needed
granularity — that has to be established, not assumed. Benefit: no new disclosure at all, an
unforgeable numerator rather than a self-reported one, and it repairs the seam-1 objection that the
gossip figure is Sybil-settable. **The strongest long-term answer if the quantity exists.**

**(4) Measure it in the harness only, and publish nothing.**
Accept that the field number is unobtainable and grade decentralization in the deterministic tiers
and the graded cloud runs, where the topology is known and every node is instrumented by the
operator. Cost: the property is never checked on the real network — a field run confirms it, and no
production alarm exists. Benefit: zero disclosure, zero new mechanism, and it is honest about what a
privacy-preserving network can know about itself. **The cheapest, and it may be the right answer.**

**(5) Opt-in telemetry with an explicit operator choice.**
A separate flag from `-privacy`, off by default, that an operator turns on to contribute work
counters to the network's own health measurement. Cost: a self-selected sample, which is a biased
denominator — and the bias is in the dangerous direction, since an operator running a capture would
not opt in. Benefit: some signal, honestly labelled. **Weakest of the measurement options: a biased
sample presented as a measurement is the failure mode this whole lane exists to avoid.**

## The recommendation, and the one call that is the owner's

**Recommended: (4) now, (3) investigated for the RC's successor.** Grade decentralization where the
topology is known, and stop pretending a production alarm exists. Then establish whether the
committed ledger carries a per-node work quantity that would make (3) possible; if it does, that is
the answer, because it removes both the disclosure and the self-reporting at once.

What must NOT happen, and is the reason this doc exists: shipping the R2.4 canary with concentration
aborts that cannot fire, while the release notes say the flip is guarded by them. That is a green
gate with no demonstrated red — the exact failure the simplicity rules name — and it would be
carried into the external B8 pass as a claim the network cannot support.

**The owner's call:** which of (1)/(3)/(4)/(5) the RC ships with, and whether the C6 canary's
concentration aborts are removed, re-pointed at the harness, or left in place with their limitation
disclosed. Whichever is chosen, the C6 row must stop implying an abort that cannot fire.
