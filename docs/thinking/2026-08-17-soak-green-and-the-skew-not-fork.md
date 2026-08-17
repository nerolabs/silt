# 2026-08-17 — the soak re-run (6fbcf2e-18553): the PE gate's launch half GREEN; the flow-5 "fork" was hash-identical skew

18 pass / 3 gap / 0 fail — the second consecutive zero-fail run.

## The soak PASSED — the launch-regime liveness gate closes

`soak-publish-drain`: 13 heights under continuously interleaved publish + natural
renewal drain, **10/10 publishes landed** (the first execution, pre-fixes: 2/15 and a
361s stall), max inter-commit gap **209s ≤ the 220s computed escape bound**, zero
honest slashes. With the zero-fail MATURING sheet (82bcd2b-39478), **both red-team
#183 regime blockers named by the PE gate are now closed**: the mature half (publish
liveness + the B2 drills) and the launch half (the interleaved publish/drain race).

## The flow-5 anomaly: attributed to harness read-skew, NOT a fork

The drill flagged a sybil head (h45) above the anchored ceiling (h44) pre-drill and
presumed "#402 fork" — but the captured journals discriminate by HASH: sybil-1's
h43 = `624c3c5061df` = val-b's = val-d's (h40 matches too). **One chain; the sybil
had merely synced a fresh broadcast the ceiling-read anchors hadn't landed at read
time.** The premise check compared heights only — heights cannot distinguish skew
from divergence; hashes can. Fixed in this PR: the guard now hash-compares at the
shared height (same ⇒ re-read the ceiling and proceed; different ⇒ a real fork
finding), and the verdict text stops presuming #402 (rule 7).

## Where this leaves the sequence

Red-team #183's field gates are met per PE §7. Remaining before release: the PE's
own review of the two zero-fail sheets + the day's five research certifications
(#441-A, #448-flagged, #451/#453, #456/#457, and the syncTargets determinism note),
and the #183 entry criteria in release-checklist.md. Flow 5 itself re-grades on any
future MATURING=0 run with the fixed premise check — its property is already
certified by 10a/10b at chain level and on the wire.
