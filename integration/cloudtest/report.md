# silt field-test report

- **run:** `c843276-15755`  ·  **silt commit:** `c843276`  ·  **harness commit:** `c843276`  ·  **bond mode:** `fast`  ·  **generated:** 2026-08-22T16:31:42Z
- **result:** **REVIEW**  ·  10 pass / 1 gap / 0 fail / 12 skip

## Per-flow verdict

| flow | verdict | severity | elapsed | detail |
|------|---------|----------|---------|--------|
| `1-first-run` | ✅ pass | blocker |  | all silt nodes report service active |
| `10-maturing-handoff` | ➖ skip | major |  | not a MATURING topology — opt in with MATURING=1 SYBILS=8 ./cloudtest.sh to field-exercise the handoff/post-shed regime (the external red team's sharpest seam-#8 target; until then it is proven only in-process: core/chain/quorum_weight_test.go + sim/maturequorum_test.go) |
| `11-economy-repair` | ➖ skip | minor |  | opt-in (ECONOMY=1): the S7 repair-bounty-on-the-wire grade |
| `184-equivocation` | ➖ skip | blocker |  | runs on the contained equivocation ISLAND every sheet (flow_equivocation_island — a separate consensus universe; its slash never taxes main-sheet fault tolerance, PE 2026-08-17). This row is the historical pointer; the island row is the graded verdict. |
| `184-equivocation-island` | ➖ skip | blocker |  | skipped — node 'island-a' not in this topology |
| `184-forged-block` | ➖ skip | major |  | skipped — node 'adversary' not in this topology |
| `184-partition` | ➖ skip | major |  | skipped — node 'val-c' not in this topology |
| `2-publish-fetch` | ✅ pass | blocker | 30s | fetched from store-1 bit-perfect |
| `3-care-link` | ✅ pass | minor |  | publish exposes a siltcare: link (repair/audit without the key) |
| `4-become-validator` | ✅ pass | major |  | non-anchor validators earn their OWN standing on the objective path |
| `5-convergence` | ✅ pass | major |  | all validators within 2 of tip=5 AND every tip-height validator shares head hash 06c4ffb051d8… (heights: val-a=5:06c4ffb051d8 val-b=5:06c4ffb051d8); DURABLE (val-a head 5->5 over 20s, no regression) |
| `5-sybil-no-capture` | ➖ skip | major |  | no Sybil cohort in this topology — opt in with SYBILS=8 ./cloudtest.sh to certify the PURE anchor gate on cloud (the local integration/sybil suite reaches only the standing gate) |
| `6-fault-tolerance` | ➖ skip | major |  | skipped — node 'val-d' not in this topology |
| `7-restart-content` | ➖ skip | major |  | skipped — needs store-2 (absent in this topology, e.g. SMOKE) |
| `7-restart-standing` | ✅ pass | major | 5s | val-b standing returned after restart without re-bonding |
| `8-takedown` | ⚠️ gap | major |  | takedown scoping not confirmed (denied=1 served=0) — daemon never narrated denylist enforcement, or store-2 failed to serve |
| `9-cross-nat` | ➖ skip | major |  | skipped — node 'nat-1' not in this topology |
| `chaos-crash` | ➖ skip | major |  | skipped — node 'store-2' not in this topology |
| `durability-turnover` | ➖ skip | major |  | skipped — node 'store-2' not in this topology |
| `infra-node-liveness` | ✅ pass | blocker |  | node-liveness precondition HELD — no OOM-kill or crash-loop across the cohort, so the sheet was graded on a HEALTHY network |
| `infra-node-memory` | ✅ pass | info |  | RSS envelope measured (cgroup MemoryCurrent, every 30s → rss-c843276-15755.jsonl): worst peak 0.20GiB across the cohort. fetch-1 peak=0.01GiB final=0.01GiB n=6; store-1 peak=0.01GiB final=0.01GiB n=6; val-a peak=0.20GiB final=0.18GiB n=6; val-b peak=0.20GiB final=0.20GiB n=6 |
| `priv-unlinkability` | ✅ pass | major |  | default chain REFUSED a durable file→publisher link (refuse-to-surveil) |
| `web-ui-guard` | ✅ pass | major |  | web-UI guard held on a real VM: no-token POST=401 (want 401), DNS-rebinding Host=403 (want 403), token-free read=200 (want 200) |

## Findings (gaps + failures), most severe first

### 184-equivocation — ➖ skip (blocker)
runs on the contained equivocation ISLAND every sheet (flow_equivocation_island — a separate consensus universe; its slash never taxes main-sheet fault tolerance, PE 2026-08-17). This row is the historical pointer; the island row is the graded verdict.

### 184-equivocation-island — ➖ skip (blocker)
skipped — node 'island-a' not in this topology

### 10-maturing-handoff — ➖ skip (major)
not a MATURING topology — opt in with MATURING=1 SYBILS=8 ./cloudtest.sh to field-exercise the handoff/post-shed regime (the external red team's sharpest seam-#8 target; until then it is proven only in-process: core/chain/quorum_weight_test.go + sim/maturequorum_test.go)

### 184-forged-block — ➖ skip (major)
skipped — node 'adversary' not in this topology

### 184-partition — ➖ skip (major)
skipped — node 'val-c' not in this topology

### 5-sybil-no-capture — ➖ skip (major)
no Sybil cohort in this topology — opt in with SYBILS=8 ./cloudtest.sh to certify the PURE anchor gate on cloud (the local integration/sybil suite reaches only the standing gate)

### 6-fault-tolerance — ➖ skip (major)
skipped — node 'val-d' not in this topology

### 7-restart-content — ➖ skip (major)
skipped — needs store-2 (absent in this topology, e.g. SMOKE)

### 8-takedown — ⚠️ gap (major)
takedown scoping not confirmed (denied=1 served=0) — daemon never narrated denylist enforcement, or store-2 failed to serve

### 9-cross-nat — ➖ skip (major)
skipped — node 'nat-1' not in this topology

### chaos-crash — ➖ skip (major)
skipped — node 'store-2' not in this topology

### durability-turnover — ➖ skip (major)
skipped — node 'store-2' not in this topology

### 11-economy-repair — ➖ skip (minor)
opt-in (ECONOMY=1): the S7 repair-bounty-on-the-wire grade

---

_Generated by `integration/cloudtest`. The field network is ephemeral and was torn down after this run._