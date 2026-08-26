# silt field-test report

- **run:** `8a52aba-deep`  ·  **silt commit:** `dcc9186`  ·  **harness commit:** `dcc9186`  ·  **bond mode:** `fast`  ·  **generated:** 2026-08-26T12:37:29Z
- **result:** **FAIL**  ·  21 pass / 7 gap / 1 fail / 2 skip

## Per-flow verdict

| flow | verdict | severity | elapsed | detail |
|------|---------|----------|---------|--------|
| `1-first-run` | ✅ pass | blocker |  | all silt nodes report service active |
| `10-maturing-handoff` | ✅ pass | major |  | young→mature HANDOFF: latch tripped on the wire; drive reached h58 (target h57) within 5490s |
| `10a-stall-drill` | ❌ fail | major |  | B2 stall drill: with the 4 cheap epoch members DECLINING to attest, the honest >⅔-weight coalition still commits on the wire within the computed 430s bound (head-counted quorum left this exact network born-unable-to-commit at 4×MinBond) |
| `10b-capture-drill` | ✅ pass | major |  | B2 capture drill: the 4 MinBond epoch members alone could NOT advance the mature chain past the honest ceiling h58 (cohort head →58, fresh cohort commit: 1), and it resumed past h58 once honest weight returned — post-shed capture is weight-priced, not head-priced |
| `10c-ws-cold-sync` | ✅ pass | major |  | WS cold-sync under the latch: val-b restarted pinned to checkpoint 60:b6bda6dfc0dd65634ce7766adbc0fbda7eb5fd787b9407d0ca42876358163966, caught up to h62 (sync=1) and came back with the wheels STILL shed (latch_held=1 — a restart must never re-arm the anchors, F-1) |
| `11-economy-repair` | ✅ pass | major |  | the S7 repair economy CLOSED on the wire: killed 3 columns' holders → the caretaker RECONSTRUCTED from parity → a verified-repair bounty drew the object's reserve down (paid=432770 credits over 1 repair(s)) — durability paid for itself on a real network, standing untouched (Invariant A). Post-kill cycle: store-2 last-sweep=29/29 stripes-repaired=0; relay last-sweep=29/29 stripes-repaired=2 |
| `11b-economy-skim` | ✅ pass | major |  | the SKIM leg closed on the wire: serve traffic (reconstruction reads + driven fetches) routed revenue into the object's durability reserve on the serving holder's ledger (funded 400000 → 432770 on store-2: +32770 pure skim above the prepay baseline) — the object pays for its own repair (S7) |
| `11c-economy-horizon` | ✅ pass | info |  | g-instrumentation sample (S7 finite-but-renewable): paid=432770 over 1 repair(s), reserve-after=400000, horizonSec=-1 (−1 = no burn window yet). One row per graded run — the g trend needs the series, not this sample |
| `12-deep-heights` | ⚠️ gap | major |  | post-drill steady state NOT reached within 1220s (#549 Q4 barrier): the network did not both converge on one head AND land a clean commit after the maturing drills mass-restarted 8/12 seats — the deep drive is UNTESTED (degraded premise / post-restart convergence), NOT a depth FAIL. If this recurs after the #549 catch-up fix, attribute from the validator journals (round-change smear) before re-running. |
| `184-equivocation` | ➖ skip | blocker |  | runs on the contained equivocation ISLAND every sheet (flow_equivocation_island — a separate consensus universe; its slash never taxes main-sheet fault tolerance, PE 2026-08-17). This row is the historical pointer; the island row is the graded verdict. |
| `184-equivocation-island` | ✅ pass | blocker |  | accountability FIRED on the wire: a contained island anchor double-signed and an honest anchor SLASHED it (slashed equivocator 6c5f111568172664ae5c47077f59620f93cf8a352e0ab8d1877c730972f3b701 (double-signed at height 1)) — proven equivocation → permanent eviction (F2), zero blast radius to the main sheet (separate consensus universe) |
| `184-forged-block` | ✅ pass | major |  | forged-signature proposal rejected (adversary logged 'correctly REJECTED by val-a') |
| `184-low-bond` | ⚠️ gap | major |  | adversary holds a qualifying 64M bond and was CORRECTLY accepted as a proposer — an under-bond REJECTION test needs a dedicated sub-min-bond identity (#350); the property is certified in-process (#204) |
| `184-partition` | ⚠️ gap | major |  | val-c did not reconverge to the heal-time majority head h43 within 300s of heal (val-c=39:6098011e52e7… stalled-from h39) — read the captured validator journals before attributing (slow catch-up sync vs a real reconverge break) |
| `2-publish-fetch` | ⚠️ gap | blocker |  | publish never produced a silt: link within 360s — the publish could not be gathered (egress/preemption, or issuer-set discovery not landing over WAN, #351); last publish error: EMPTY RESPONSE from fetch-1 — ssh_node returned NOTHING across the whole window: the node is UNREACHABLE or the node MAP is wrong (check nodes.json zones/ips), NOT a publish failure. reachable-peers=4/4; property UNTESTED, not failed |
| `3-care-link` | ✅ pass | minor |  | publish exposes a siltcare: link (repair/audit without the key) |
| `4-become-validator` | ✅ pass | major |  | non-anchor validators earn their OWN standing on the objective path |
| `5-convergence` | ✅ pass | major |  | all validators within 2 of tip=18 AND every tip-height validator shares head hash a13e82e69f70… (heights: val-a=16:540bfe3d9d00 val-b=17:ea53cf99d3ae val-c=16:540bfe3d9d00 val-d=18:a13e82e69f70); DURABLE (val-a head 16->20 over 20s, no regression) |
| `5-sybil-no-capture` | ➖ skip | major |  | MATURING=1 topology sheds the anchors by design — the anchor-gate premise doesn't exist here; the post-shed capture property is certified by 10-maturing-handoff's B2 drills (run without MATURING for flow 5) |
| `6-fault-tolerance` | ✅ pass | major |  | publish still committed with one validator (val-d) down (within the computed 650s down-designee escape bound) |
| `7-restart-content` | ⚠️ gap | major |  | no link (reuse+self-publish both failed) — restart-content UNTESTED — the publish could not be gathered (egress/preemption, or issuer-set discovery not landing over WAN, #351); last publish error: EMPTY RESPONSE from fetch-1 — ssh_node returned NOTHING across the whole window: the node is UNREACHABLE or the node MAP is wrong (check nodes.json zones/ips), NOT a publish failure. reachable-peers=4/4; property UNTESTED, not failed |
| `7-restart-standing` | ✅ pass | major | 6s | val-b standing returned after restart without re-bonding |
| `8-takedown` | ⚠️ gap | major |  | no link (reuse+self-publish both failed) — takedown UNTESTED — the publish could not be gathered (egress/preemption, or issuer-set discovery not landing over WAN, #351); last publish error: EMPTY RESPONSE from fetch-1 — ssh_node returned NOTHING across the whole window: the node is UNREACHABLE or the node MAP is wrong (check nodes.json zones/ips), NOT a publish failure. reachable-peers=4/4; property UNTESTED, not failed |
| `9-cross-nat` | ✅ pass | major |  | natted nodes exchanged a file through the relay/hole-punch |
| `chaos-fetch` | ✅ pass | major |  | content fetchable BIT-PERFECT after a hard-crash (SIGKILL) + restart of a storage node |
| `chaos-reprovide` | ✅ pass | major |  | SIGKILLed storage node re-announced its held chunks (#69) after a hard crash (40s to re-announce; latency scales with held-chunk count, #402/M1) |
| `durability-turnover` | ⚠️ gap | major |  | setup publish did not land a link — durability UNTESTED this run, not a durability failure (read .ft_publish_lasterr to decompose: 'accepted but not committed' = the accept→commit path — commit latency vs the client poll window, #441-family; token/issuer-set errors = discovery #351. The validator journals for the publish window are captured with this verdict) |
| `infra-node-liveness` | ✅ pass | blocker |  | node-liveness precondition HELD — no OOM-kill or crash-loop across the cohort, so the sheet was graded on a HEALTHY network |
| `infra-node-memory` | ✅ pass | info |  | RSS envelope measured (cgroup MemoryCurrent, every 30s → rss-8a52aba-deep.jsonl): worst peak 1.05GiB across the cohort. adversary peak=0.89GiB final=0.34GiB n=25; fetch-1 peak=0.02GiB final=0.01GiB n=25; island-a peak=0.35GiB final=0.28GiB n=24; island-b peak=0.37GiB final=0.29GiB n=24; island-c peak=0.39GiB final=0.30GiB n=24; island-d peak=0.35GiB final=0.29GiB n=24; maturer-1 peak=0.64GiB final=0.37GiB n=22; maturer-2 peak=0.92GiB final=0.48GiB n=24; maturer-3 peak=1.05GiB final=0.47GiB n=24; maturer-4 peak=0.87GiB final=0.40GiB n=24; nat-1 peak=0.02GiB final=0.02GiB n=24; nat-2 peak=0.02GiB final=0.02GiB n=24; registry peak=0.00GiB final=0.00GiB n=24; relay peak=0.02GiB final=0.02GiB n=24; store-1 peak=0.02GiB final=0.01GiB n=24; store-2 peak=0.03GiB final=0.02GiB n=24; store-3 peak=0.02GiB final=0.02GiB n=24; store-4 peak=0.02GiB final=0.02GiB n=24; sybil-1 peak=0.81GiB final=0.35GiB n=24; sybil-2 peak=0.70GiB final=0.32GiB n=24; sybil-3 peak=0.61GiB final=0.36GiB n=24; sybil-4 peak=0.32GiB final=0.19GiB n=22; val-a peak=0.86GiB final=0.45GiB n=24; val-b peak=0.95GiB final=0.41GiB n=24; val-c peak=0.87GiB final=0.34GiB n=24; val-d peak=0.86GiB final=0.48GiB n=23 |
| `priv-unlinkability` | ✅ pass | major |  | default chain REFUSED a durable file→publisher link (refuse-to-surveil) |
| `web-ui-guard` | ✅ pass | major |  | web-UI guard held on a real VM: no-token POST=401 (want 401), DNS-rebinding Host=403 (want 403), token-free read=200 (want 200) |

## Findings (gaps + failures), most severe first

### 184-equivocation — ➖ skip (blocker)
runs on the contained equivocation ISLAND every sheet (flow_equivocation_island — a separate consensus universe; its slash never taxes main-sheet fault tolerance, PE 2026-08-17). This row is the historical pointer; the island row is the graded verdict.

### 2-publish-fetch — ⚠️ gap (blocker)
publish never produced a silt: link within 360s — the publish could not be gathered (egress/preemption, or issuer-set discovery not landing over WAN, #351); last publish error: EMPTY RESPONSE from fetch-1 — ssh_node returned NOTHING across the whole window: the node is UNREACHABLE or the node MAP is wrong (check nodes.json zones/ips), NOT a publish failure. reachable-peers=4/4; property UNTESTED, not failed

### 10a-stall-drill — ❌ fail (major)
B2 stall drill: with the 4 cheap epoch members DECLINING to attest, the honest >⅔-weight coalition still commits on the wire within the computed 430s bound (head-counted quorum left this exact network born-unable-to-commit at 4×MinBond)

### 12-deep-heights — ⚠️ gap (major)
post-drill steady state NOT reached within 1220s (#549 Q4 barrier): the network did not both converge on one head AND land a clean commit after the maturing drills mass-restarted 8/12 seats — the deep drive is UNTESTED (degraded premise / post-restart convergence), NOT a depth FAIL. If this recurs after the #549 catch-up fix, attribute from the validator journals (round-change smear) before re-running.

### 184-low-bond — ⚠️ gap (major)
adversary holds a qualifying 64M bond and was CORRECTLY accepted as a proposer — an under-bond REJECTION test needs a dedicated sub-min-bond identity (#350); the property is certified in-process (#204)

### 184-partition — ⚠️ gap (major)
val-c did not reconverge to the heal-time majority head h43 within 300s of heal (val-c=39:6098011e52e7… stalled-from h39) — read the captured validator journals before attributing (slow catch-up sync vs a real reconverge break)

### 5-sybil-no-capture — ➖ skip (major)
MATURING=1 topology sheds the anchors by design — the anchor-gate premise doesn't exist here; the post-shed capture property is certified by 10-maturing-handoff's B2 drills (run without MATURING for flow 5)

### 7-restart-content — ⚠️ gap (major)
no link (reuse+self-publish both failed) — restart-content UNTESTED — the publish could not be gathered (egress/preemption, or issuer-set discovery not landing over WAN, #351); last publish error: EMPTY RESPONSE from fetch-1 — ssh_node returned NOTHING across the whole window: the node is UNREACHABLE or the node MAP is wrong (check nodes.json zones/ips), NOT a publish failure. reachable-peers=4/4; property UNTESTED, not failed

### 8-takedown — ⚠️ gap (major)
no link (reuse+self-publish both failed) — takedown UNTESTED — the publish could not be gathered (egress/preemption, or issuer-set discovery not landing over WAN, #351); last publish error: EMPTY RESPONSE from fetch-1 — ssh_node returned NOTHING across the whole window: the node is UNREACHABLE or the node MAP is wrong (check nodes.json zones/ips), NOT a publish failure. reachable-peers=4/4; property UNTESTED, not failed

### durability-turnover — ⚠️ gap (major)
setup publish did not land a link — durability UNTESTED this run, not a durability failure (read .ft_publish_lasterr to decompose: 'accepted but not committed' = the accept→commit path — commit latency vs the client poll window, #441-family; token/issuer-set errors = discovery #351. The validator journals for the publish window are captured with this verdict)

---

_Generated by `integration/cloudtest`. The field network is ephemeral and was torn down after this run._