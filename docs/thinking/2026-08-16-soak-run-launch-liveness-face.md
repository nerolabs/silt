# 2026-08-16 — the soak run (9453325-7258): the drill catches the launch-regime face of #441 on its first execution

The PE-required launch-topology publish/drain soak (the #432 gate's second half),
first execution: **18 pass / 2 gap / 1 fail / 1 skip** — every standard flow green
(including chaos-fetch and durability-turnover, which failed/GAPped on the MATURING
run — more regime-correlation evidence for #441), the two known #345/#350 GAPs, and
the single FAIL is **the soak drill doing exactly its job**:

```
WEDGE SIGNATURE under the publish/drain soak: a height went 361s (> the computed
160s escape bound) without a commit with the network live (h51→h60, 2/15
publishes landed) — last client output: propose height 60 round 0: already
signed a different block in this slot (never-sign-twice, #397/#432)
```

## The mechanism — #441's SECOND face (code-cited)

`maybeAdvanceRound` quiesces when `len(pendingBondRegs) == 0 && !bondDrainInFlight`
— **the #432 escape is armed by pending drain work only.** A height whose r0
prepare slots are consumed by a crossed publish race, on a launch network whose
renewal queues are momentarily empty (sparse TTL cadence — unlike MATURING's
continuous first-timer traffic), has **no escape driver**: every publish retry
dies at the same (h, r0) slot until the next renewal happens to arrive and arm a
sweep. The observed 361 s ≈ one renewal-cadence gap, 2.3× the computed bound. Also:
only 2/15 publishes landed across the soak window — launch-regime publish
starvation-in-degree, mature's zero being the limit case.

**One root, two faces:** the round-escape machinery (its arming condition AND its
new-view seat) belongs exclusively to the drain path. Face (a): mature starvation
(run a56ac10-42834, the born-RED oracle). Face (b): launch per-height liveness
violation (this run). Both regimes of red-team #183 now gate on the #441 consult
(`/Users/andrewedmond/Claude/claude/silt-reviews/research/441-publish-starvation-CONSULT.md`,
§2b added). The certified fix must arm the escape on ANY stuck proposal work and
let the new-view carry it; the starvation oracle needs a launch-face sibling.

## Process notes

- The drill's computed bounds (H_ESCAPE_S=160 s; per PE §4 a miss inside a
  principled bound is a FINDING) did exactly what arbitrary windows never could:
  361 s is not "slow WAN", it is 2.3× a derived bound with a named mechanism.
- The PR #443 never-discard-the-client-error fix paid off on its first run: the
  drill verdict carries the exact client error inline.
- **Console-log anomaly (flagged, unresolved):** the run's console redirect file
  ends at the startup phase while the Monitor (tailing the same path, following
  the original inode) received the full stream through teardown. The graded
  artifacts (report.md, results.jsonl, the 3.4 MB flow-evidence capture) are the
  harness's own files and are complete — they, not a session redirect, are the
  record. Mechanism not attributed; noting per discipline rather than guessing.
- Cloud verified empty after teardown; no lingering processes.
