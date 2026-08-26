# silt field-test report

- **run:** `474718e-deep`  ·  **silt commit:** `474718e`  ·  **harness commit:** `474718e`  ·  **bond mode:** `fast`  ·  **generated:** 2026-08-26T09:14:57Z
- **result:** **FAIL**  ·  22 pass / 2 gap / 2 fail / 2 skip

## Per-flow verdict

| flow | verdict | severity | elapsed | detail |
|------|---------|----------|---------|--------|
| `1-first-run` | ✅ pass | blocker |  | all silt nodes report service active |
| `10-maturing-handoff` | ❌ fail | major |  | young→mature HANDOFF: latch tripped on the wire; drive reached h56 (target h65) within 5490s — TARGET NOT REACHED IN BOUND |
| `11-economy-repair` | ✅ pass | major |  | the S7 repair economy CLOSED on the wire: killed 3 columns' holders → the caretaker RECONSTRUCTED from parity → a verified-repair bounty drew the object's reserve down (paid=465540 credits over 1 repair(s)) — durability paid for itself on a real network, standing untouched (Invariant A). Post-kill cycle: store-2 last-sweep=23/29 stripes-repaired=2; relay last-sweep=25/29 stripes-repaired=1 |
| `11b-economy-skim` | ✅ pass | major |  | the SKIM leg closed on the wire: serve traffic (reconstruction reads + driven fetches) routed revenue into the object's durability reserve on the serving holder's ledger (funded 400000 → 465540 on relay: +65540 pure skim above the prepay baseline) — the object pays for its own repair (S7) |
| `11c-economy-horizon` | ✅ pass | info |  | g-instrumentation sample (S7 finite-but-renewable): paid=465540 over 1 repair(s), reserve-after=0, horizonSec=0 (−1 = no burn window yet). One row per graded run — the g trend needs the series, not this sample |
| `12-deep-heights` | ⚠️ gap | major |  | post-drill steady state NOT reached within 1220s (#549 Q4 barrier): the network did not both converge on one head AND land a clean commit after the maturing drills mass-restarted 8/12 seats — the deep drive is UNTESTED (degraded premise / post-restart convergence), NOT a depth FAIL. If this recurs after the #549 catch-up fix, attribute from the validator journals (round-change smear) before re-running. |
| `184-equivocation` | ➖ skip | blocker |  | runs on the contained equivocation ISLAND every sheet (flow_equivocation_island — a separate consensus universe; its slash never taxes main-sheet fault tolerance, PE 2026-08-17). This row is the historical pointer; the island row is the graded verdict. |
| `184-equivocation-island` | ✅ pass | blocker |  | accountability FIRED on the wire: a contained island anchor double-signed and an honest anchor SLASHED it (slashed equivocator 6c5f111568172664ae5c47077f59620f93cf8a352e0ab8d1877c730972f3b701 (double-signed at height 1)) — proven equivocation → permanent eviction (F2), zero blast radius to the main sheet (separate consensus universe) |
| `184-forged-block` | ✅ pass | major |  | forged-signature proposal rejected (adversary logged 'correctly REJECTED by val-a') |
| `184-low-bond` | ✅ pass | major |  | under-bonded proposer rejected (adversary logged 'correctly REJECTED by val-a') |
| `184-partition` | ⚠️ gap | major |  | the majority committed NO heavier chain during the window (val-a head h56 ≤ val-c's pre-partition h56) — nothing to catch up to, heal UNTESTED not failed (drive under-committed; the majority publishes did not land) |
| `2-publish-fetch` | ✅ pass | blocker | 206s | fetched from store-2 bit-perfect |
| `3-care-link` | ✅ pass | minor |  | publish exposes a siltcare: link (repair/audit without the key) |
| `4-become-validator` | ✅ pass | major |  | non-anchor validators earn their OWN standing on the objective path |
| `5-convergence` | ❌ fail | major |  | all validators within 2 of tip=56 AND every tip-height validator shares head hash 7c9401ef1b90… (heights: val-a=56:7c9401ef1b90 val-b=56:7c9401ef1b90 val-c=56:7c9401ef1b90 val-d=31:84b39d6766e5) |
| `5-sybil-no-capture` | ➖ skip | major |  | MATURING=1 topology sheds the anchors by design — the anchor-gate premise doesn't exist here; the post-shed capture property is certified by 10-maturing-handoff's B2 drills (run without MATURING for flow 5) |
| `6-fault-tolerance` | ✅ pass | major |  | publish still committed with one validator (val-d) down (within the computed 650s down-designee escape bound) |
| `7-restart-content` | ✅ pass | major |  | content still fetchable BIT-PERFECT after a storage-node restart |
| `7-restart-standing` | ✅ pass | major | 12s | val-b standing returned after restart without re-bonding |
| `8-takedown` | ✅ pass | major |  | store-1 enforces the operator denylist ([1404]: denylist: honoring 1 denied root(s)) while store-2 still serves BIT-PERFECT (no global switch) |
| `9-cross-nat` | ✅ pass | major |  | natted nodes exchanged a file through the relay/hole-punch |
| `chaos-fetch` | ✅ pass | major |  | content fetchable BIT-PERFECT after a hard-crash (SIGKILL) + restart of a storage node |
| `chaos-reprovide` | ✅ pass | major |  | SIGKILLed storage node re-announced its held chunks (#69) after a hard crash (26s to re-announce; latency scales with held-chunk count, #402/M1) |
| `durability-turnover` | ✅ pass | major |  | content survived a PERMANENT storage-node departure — fetched bit-perfect from a survivor |
| `infra-node-liveness` | ✅ pass | blocker |  | node-liveness precondition HELD — no OOM-kill or crash-loop across the cohort, so the sheet was graded on a HEALTHY network |
| `infra-node-memory` | ✅ pass | info |  | RSS envelope measured (cgroup MemoryCurrent, every 30s → rss-474718e-deep.jsonl): worst peak 1.11GiB across the cohort. adversary peak=0.98GiB final=0.34GiB n=31; fetch-1 peak=0.01GiB final=0.01GiB n=33; island-a peak=0.34GiB final=0.28GiB n=33; island-b peak=0.36GiB final=0.30GiB n=33; island-c peak=0.38GiB final=0.30GiB n=33; island-d peak=0.34GiB final=0.30GiB n=33; maturer-1 peak=0.89GiB final=0.50GiB n=33; maturer-2 peak=0.86GiB final=0.50GiB n=32; maturer-3 peak=0.90GiB final=0.51GiB n=32; maturer-4 peak=0.86GiB final=0.52GiB n=32; nat-1 peak=0.02GiB final=0.02GiB n=32; nat-2 peak=0.02GiB final=0.02GiB n=32; registry peak=0.00GiB final=0.00GiB n=32; relay peak=0.03GiB final=0.01GiB n=32; store-1 peak=0.02GiB final=0.02GiB n=32; store-2 peak=0.03GiB final=0.01GiB n=32; store-3 peak=0.02GiB final=0.01GiB n=30; store-4 peak=0.02GiB final=0.02GiB n=32; sybil-1 peak=0.80GiB final=0.39GiB n=32; sybil-2 peak=0.84GiB final=0.42GiB n=32; sybil-3 peak=0.82GiB final=0.25GiB n=30; sybil-4 peak=0.82GiB final=0.39GiB n=32; val-a peak=1.11GiB final=0.45GiB n=32; val-b peak=0.88GiB final=0.42GiB n=32; val-c peak=0.99GiB final=0.36GiB n=32; val-d peak=0.61GiB final=0.31GiB n=31 |
| `priv-unlinkability` | ✅ pass | major |  | default chain REFUSED a durable file→publisher link (refuse-to-surveil) |
| `web-ui-guard` | ✅ pass | major |  | web-UI guard held on a real VM: no-token POST=401 (want 401), DNS-rebinding Host=403 (want 403), token-free read=200 (want 200) |

## Findings (gaps + failures), most severe first

### 184-equivocation — ➖ skip (blocker)
runs on the contained equivocation ISLAND every sheet (flow_equivocation_island — a separate consensus universe; its slash never taxes main-sheet fault tolerance, PE 2026-08-17). This row is the historical pointer; the island row is the graded verdict.

### 10-maturing-handoff — ❌ fail (major)
young→mature HANDOFF: latch tripped on the wire; drive reached h56 (target h65) within 5490s — TARGET NOT REACHED IN BOUND

### 12-deep-heights — ⚠️ gap (major)
post-drill steady state NOT reached within 1220s (#549 Q4 barrier): the network did not both converge on one head AND land a clean commit after the maturing drills mass-restarted 8/12 seats — the deep drive is UNTESTED (degraded premise / post-restart convergence), NOT a depth FAIL. If this recurs after the #549 catch-up fix, attribute from the validator journals (round-change smear) before re-running.

### 184-partition — ⚠️ gap (major)
the majority committed NO heavier chain during the window (val-a head h56 ≤ val-c's pre-partition h56) — nothing to catch up to, heal UNTESTED not failed (drive under-committed; the majority publishes did not land)

### 5-convergence — ❌ fail (major)
all validators within 2 of tip=56 AND every tip-height validator shares head hash 7c9401ef1b90… (heights: val-a=56:7c9401ef1b90 val-b=56:7c9401ef1b90 val-c=56:7c9401ef1b90 val-d=31:84b39d6766e5)

### 5-sybil-no-capture — ➖ skip (major)
MATURING=1 topology sheds the anchors by design — the anchor-gate premise doesn't exist here; the post-shed capture property is certified by 10-maturing-handoff's B2 drills (run without MATURING for flow 5)

---

_Generated by `integration/cloudtest`. The field network is ephemeral and was torn down after this run._