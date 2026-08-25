# silt field-test report

- **run:** `027c354-deep`  ·  **silt commit:** `7c64cd6`  ·  **harness commit:** `027c354`  ·  **bond mode:** `fast`  ·  **generated:** 2026-08-25T18:08:54Z
- **result:** **FAIL**  ·  16 pass / 4 gap / 4 fail / 2 skip

## Per-flow verdict

| flow | verdict | severity | elapsed | detail |
|------|---------|----------|---------|--------|
| `1-first-run` | ✅ pass | blocker |  | all silt nodes report service active |
| `10-maturing-handoff` | ❌ fail | major |  | young→mature HANDOFF: latch tripped on the wire; drive reached h36 (target h41) within 5490s — TARGET NOT REACHED IN BOUND |
| `11-economy-repair` | ⚠️ gap | major |  | setup publish got EMPTY RESPONSES from fetch-1 for the whole 360s window — fetch-1 UNREACHABLE or the node MAP is wrong (check nodes.json), a PLUMBING failure NOT a product/latency issue; economy UNTESTED |
| `12-deep-heights` | ⚠️ gap | major |  | post-drill steady state NOT reached within 1220s (#549 Q4 barrier): the network did not both converge on one head AND land a clean commit after the maturing drills mass-restarted 8/12 seats — the deep drive is UNTESTED (degraded premise / post-restart convergence), NOT a depth FAIL. If this recurs after the #549 catch-up fix, attribute from the validator journals (round-change smear) before re-running. |
| `184-equivocation` | ➖ skip | blocker |  | runs on the contained equivocation ISLAND every sheet (flow_equivocation_island — a separate consensus universe; its slash never taxes main-sheet fault tolerance, PE 2026-08-17). This row is the historical pointer; the island row is the graded verdict. |
| `184-equivocation-island` | ✅ pass | blocker |  | accountability FIRED on the wire: a contained island anchor double-signed and an honest anchor SLASHED it (slashed equivocator 6c5f111568172664ae5c47077f59620f93cf8a352e0ab8d1877c730972f3b701 (double-signed at height 1)) — proven equivocation → permanent eviction (F2), zero blast radius to the main sheet (separate consensus universe) |
| `184-forged-block` | ✅ pass | major |  | forged-signature proposal rejected (adversary logged 'correctly REJECTED by val-a') |
| `184-low-bond` | ✅ pass | major |  | under-bonded proposer rejected (adversary logged 'correctly REJECTED by val-a') |
| `184-partition` | ⚠️ gap | major |  | val-c did not reconverge to the heal-time majority head h27 within 300s of heal (val-c=24:b04e6fb7af5a… stalled-from h23) — read the captured validator journals before attributing (slow catch-up sync vs a real reconverge break) |
| `2-publish-fetch` | ✅ pass | blocker | 95s | fetched from store-2 bit-perfect |
| `3-care-link` | ✅ pass | minor |  | publish exposes a siltcare: link (repair/audit without the key) |
| `4-become-validator` | ✅ pass | major |  | non-anchor validators earn their OWN standing on the objective path |
| `5-convergence` | ❌ fail | major |  | all validators within 2 of tip=36 AND every tip-height validator shares head hash 4da18f76c77a… (heights: val-a=36:4da18f76c77a val-b=36:4da18f76c77a val-c=24:b04e6fb7af5a val-d=36:4da18f76c77a) |
| `5-sybil-no-capture` | ➖ skip | major |  | MATURING=1 topology sheds the anchors by design — the anchor-gate premise doesn't exist here; the post-shed capture property is certified by 10-maturing-handoff's B2 drills (run without MATURING for flow 5) |
| `6-fault-tolerance` | ⚠️ gap | major |  | no new commit within the computed 1445s r≤5 hard cap with val-d down (fingerprint rc=16 h=0 → rc=19 h=0: ladder advancing but uncommitted — OUT OF MODEL) — read the captured client error (publish-diag / .ft_publish_lasterr) and survivor journals before attributing (#509/#7) |
| `7-restart-content` | ✅ pass | major |  | content still fetchable BIT-PERFECT after a storage-node restart |
| `7-restart-standing` | ✅ pass | major | 7s | val-b standing returned after restart without re-bonding |
| `8-takedown` | ✅ pass | major |  | store-1 enforces the operator denylist ([1704]: denylist: honoring 1 denied root(s)) while store-2 still serves BIT-PERFECT (no global switch) |
| `9-cross-nat` | ❌ fail | major |  | natted nodes did not exchange a file via the relay — publish landed () but the FETCH leg on nat-2 returned '<none>' (want reachable-peers=4/4) |
| `chaos-fetch` | ✅ pass | major |  | content fetchable BIT-PERFECT after a hard-crash (SIGKILL) + restart of a storage node |
| `chaos-reprovide` | ✅ pass | major |  | SIGKILLed storage node re-announced its held chunks (#69) after a hard crash (35s to re-announce; latency scales with held-chunk count, #402/M1) |
| `durability-turnover` | ❌ fail | major |  | content survived a PERMANENT storage-node departure — fetched bit-perfect from a survivor (want=reachable-peers=4/4 got=<none>; client: silt: link: not a silt:v1: link) |
| `infra-node-liveness` | ✅ pass | blocker |  | node-liveness precondition HELD — no OOM-kill or crash-loop across the cohort, so the sheet was graded on a HEALTHY network |
| `infra-node-memory` | ✅ pass | info |  | RSS envelope measured (cgroup MemoryCurrent, every 30s → rss-027c354-deep.jsonl): worst peak 0.88GiB across the cohort. adversary peak=0.81GiB final=0.57GiB n=50; fetch-1 peak=0.01GiB final=0.01GiB n=50; island-a peak=0.36GiB final=0.30GiB n=50; island-b peak=0.36GiB final=0.30GiB n=50; island-c peak=0.32GiB final=0.29GiB n=50; island-d peak=0.37GiB final=0.31GiB n=50; maturer-1 peak=0.85GiB final=0.45GiB n=50; maturer-2 peak=0.79GiB final=0.55GiB n=50; maturer-3 peak=0.66GiB final=0.45GiB n=50; maturer-4 peak=0.69GiB final=0.43GiB n=50; nat-1 peak=0.01GiB final=0.01GiB n=50; nat-2 peak=0.02GiB final=0.02GiB n=50; registry peak=0.00GiB final=0.00GiB n=50; relay peak=0.02GiB final=0.02GiB n=50; store-1 peak=0.01GiB final=0.01GiB n=49; store-2 peak=0.02GiB final=0.02GiB n=50; store-3 peak=0.02GiB final=0.02GiB n=50; store-4 peak=0.02GiB final=0.02GiB n=50; sybil-1 peak=0.66GiB final=0.32GiB n=50; sybil-2 peak=0.71GiB final=0.44GiB n=50; sybil-3 peak=0.64GiB final=0.43GiB n=50; sybil-4 peak=0.58GiB final=0.41GiB n=50; val-a peak=0.88GiB final=0.59GiB n=50; val-b peak=0.83GiB final=0.58GiB n=50; val-c peak=0.60GiB final=0.34GiB n=50; val-d peak=0.77GiB final=0.41GiB n=36 |
| `priv-unlinkability` | ✅ pass | major |  | default chain REFUSED a durable file→publisher link (refuse-to-surveil) |
| `web-ui-guard` | ✅ pass | major |  | web-UI guard held on a real VM: no-token POST=401 (want 401), DNS-rebinding Host=403 (want 403), token-free read=200 (want 200) |

## Findings (gaps + failures), most severe first

### 184-equivocation — ➖ skip (blocker)
runs on the contained equivocation ISLAND every sheet (flow_equivocation_island — a separate consensus universe; its slash never taxes main-sheet fault tolerance, PE 2026-08-17). This row is the historical pointer; the island row is the graded verdict.

### 10-maturing-handoff — ❌ fail (major)
young→mature HANDOFF: latch tripped on the wire; drive reached h36 (target h41) within 5490s — TARGET NOT REACHED IN BOUND

### 11-economy-repair — ⚠️ gap (major)
setup publish got EMPTY RESPONSES from fetch-1 for the whole 360s window — fetch-1 UNREACHABLE or the node MAP is wrong (check nodes.json), a PLUMBING failure NOT a product/latency issue; economy UNTESTED

### 12-deep-heights — ⚠️ gap (major)
post-drill steady state NOT reached within 1220s (#549 Q4 barrier): the network did not both converge on one head AND land a clean commit after the maturing drills mass-restarted 8/12 seats — the deep drive is UNTESTED (degraded premise / post-restart convergence), NOT a depth FAIL. If this recurs after the #549 catch-up fix, attribute from the validator journals (round-change smear) before re-running.

### 184-partition — ⚠️ gap (major)
val-c did not reconverge to the heal-time majority head h27 within 300s of heal (val-c=24:b04e6fb7af5a… stalled-from h23) — read the captured validator journals before attributing (slow catch-up sync vs a real reconverge break)

### 5-convergence — ❌ fail (major)
all validators within 2 of tip=36 AND every tip-height validator shares head hash 4da18f76c77a… (heights: val-a=36:4da18f76c77a val-b=36:4da18f76c77a val-c=24:b04e6fb7af5a val-d=36:4da18f76c77a)

### 5-sybil-no-capture — ➖ skip (major)
MATURING=1 topology sheds the anchors by design — the anchor-gate premise doesn't exist here; the post-shed capture property is certified by 10-maturing-handoff's B2 drills (run without MATURING for flow 5)

### 6-fault-tolerance — ⚠️ gap (major)
no new commit within the computed 1445s r≤5 hard cap with val-d down (fingerprint rc=16 h=0 → rc=19 h=0: ladder advancing but uncommitted — OUT OF MODEL) — read the captured client error (publish-diag / .ft_publish_lasterr) and survivor journals before attributing (#509/#7)

### 9-cross-nat — ❌ fail (major)
natted nodes did not exchange a file via the relay — publish landed () but the FETCH leg on nat-2 returned '<none>' (want reachable-peers=4/4)

### durability-turnover — ❌ fail (major)
content survived a PERMANENT storage-node departure — fetched bit-perfect from a survivor (want=reachable-peers=4/4 got=<none>; client: silt: link: not a silt:v1: link)

---

_Generated by `integration/cloudtest`. The field network is ephemeral and was torn down after this run._