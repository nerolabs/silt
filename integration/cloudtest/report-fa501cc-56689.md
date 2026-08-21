# silt field-test report

- **run:** `fa501cc-56689`  ·  **silt commit:** `fa501cc`  ·  **harness commit:** `fa501cc`  ·  **bond mode:** `fast`  ·  **generated:** 2026-08-21T11:22:56Z
- **result:** **FAIL**  ·  23 pass / 0 gap / 1 fail / 3 skip

## Per-flow verdict

| flow | verdict | severity | elapsed | detail |
|------|---------|----------|---------|--------|
| `1-first-run` | ✅ pass | blocker |  | all silt nodes report service active |
| `10-maturing-handoff` | ➖ skip | major |  | not a MATURING topology — opt in with MATURING=1 SYBILS=8 ./cloudtest.sh to field-exercise the handoff/post-shed regime (the external red team's sharpest seam-#8 target; until then it is proven only in-process: core/chain/quorum_weight_test.go + sim/maturequorum_test.go) |
| `11-economy-repair` | ✅ pass | major |  | the S7 repair economy CLOSED on the wire: killed 3 columns' holders → the caretaker RECONSTRUCTED from parity → a verified-repair bounty drew the object's reserve down (paid=432770 credits over 1 repair(s)) — durability paid for itself on a real network, standing untouched (Invariant A). Post-kill cycle: store-2 last-sweep=21/29 stripes-repaired=2; relay last-sweep=29/29 stripes-repaired=2 |
| `11b-economy-skim` | ✅ pass | major |  | the SKIM leg closed on the wire: serve traffic (reconstruction reads + driven fetches) routed revenue into the object's durability reserve on the serving holder's ledger (funded 400000 → 432770 on relay: +32770 pure skim above the prepay baseline) — the object pays for its own repair (S7) |
| `11c-economy-horizon` | ✅ pass | info |  | g-instrumentation sample (S7 finite-but-renewable): paid=432770 over 1 repair(s), reserve-after=0, horizonSec=0 (−1 = no burn window yet). One row per graded run — the g trend needs the series, not this sample |
| `184-equivocation` | ➖ skip | blocker |  | runs on the contained equivocation ISLAND every sheet (flow_equivocation_island — a separate consensus universe; its slash never taxes main-sheet fault tolerance, PE 2026-08-17). This row is the historical pointer; the island row is the graded verdict. |
| `184-equivocation-island` | ✅ pass | blocker |  | accountability FIRED on the wire: a contained island anchor double-signed and an honest anchor SLASHED it (slashed equivocator 6c5f111568172664ae5c47077f59620f93cf8a352e0ab8d1877c730972f3b701 (double-signed at height 1)) — proven equivocation → permanent eviction (F2), zero blast radius to the main sheet (separate consensus universe) |
| `184-forged-block` | ✅ pass | major |  | forged-signature proposal rejected (adversary logged 'correctly REJECTED by val-a') |
| `184-low-bond` | ✅ pass | major |  | under-bonded proposer rejected (adversary logged 'correctly REJECTED by val-a') |
| `184-partition` | ✅ pass | major |  | minority val-c STALLED at h9 through the partition (a < ⅓ island cannot commit) then CAUGHT UP to the heal-time majority head h12 (now at h12) on heal — BFT partition→heal reconverged over the real wire (a catch-up, NOT a reorg — a minority never committed a conflicting fork) |
| `2-publish-fetch` | ✅ pass | blocker | 66s | fetched from store-2 bit-perfect |
| `3-care-link` | ✅ pass | minor |  | publish exposes a siltcare: link (repair/audit without the key) |
| `4-become-validator` | ✅ pass | major |  | non-anchor validators earn their OWN standing on the objective path |
| `5-convergence` | ✅ pass | major |  | all validators within 2 of tip=12 AND every tip-height validator shares head hash c2dba8e8a3cf… (heights: val-a=12:c2dba8e8a3cf val-b=12:c2dba8e8a3cf val-c=12:c2dba8e8a3cf val-d=12:c2dba8e8a3cf); DURABLE (val-a head 12->12 over 20s, no regression) |
| `5-sybil-no-capture` | ➖ skip | major |  | no Sybil cohort in this topology — opt in with SYBILS=8 ./cloudtest.sh to certify the PURE anchor gate on cloud (the local integration/sybil suite reaches only the standing gate) |
| `6-fault-tolerance` | ✅ pass | major |  | publish still committed with one validator (val-d) down (within the computed 260s down-designee escape bound) |
| `7-restart-content` | ✅ pass | major |  | content still fetchable BIT-PERFECT after a storage-node restart |
| `7-restart-standing` | ✅ pass | major | 11s | val-b standing returned after restart without re-bonding |
| `8-takedown` | ✅ pass | major |  | store-1 enforces the operator denylist ([1866]: denylist: honoring 1 denied root(s)) while store-2 still serves BIT-PERFECT (no global switch) |
| `9-cross-nat` | ✅ pass | major |  | natted nodes exchanged a file through the relay/hole-punch |
| `chaos-fetch` | ✅ pass | major |  | content fetchable BIT-PERFECT after a hard-crash (SIGKILL) + restart of a storage node |
| `chaos-reprovide` | ✅ pass | major |  | SIGKILLed storage node re-announced its held chunks (#69) after a hard crash (35s to re-announce; latency scales with held-chunk count, #402/M1) |
| `durability-turnover` | ✅ pass | major |  | content survived a PERMANENT storage-node departure — fetched bit-perfect from a survivor |
| `infra-node-liveness` | ❌ fail | blocker |  | NODE CRASH-LOOP — 3 node crash(es) (kernel OOM-kill or Go fatal error) across: island-c×3. A run whose cohort DIES cannot grade its flows (a crashing node is indistinguishable from a slow/dead peer), so EVERY verdict on this sheet is PROVISIONAL until a clean no-crash re-run — including the computed bounds (which may be inflated). This is INFRASTRUCTURE FAILURE, not independent flow results. Attribute from a heap profile (re-run with DEBUG_PROFILE=1, then ./cloudtest.sh heap <node>) or the node's journal (fatal-error stack trace) — do NOT presume a cause. |
| `infra-node-memory` | ✅ pass | info |  | RSS envelope measured (cgroup MemoryCurrent, every 30s → rss-fa501cc-56689.jsonl): worst peak 1.60GiB across the cohort. adversary peak=0.48GiB final=0.48GiB n=19; fetch-1 peak=0.02GiB final=0.02GiB n=19; island-a peak=1.53GiB final=1.17GiB n=19; island-b peak=1.60GiB final=1.49GiB n=19; island-c peak=1.58GiB final=0.07GiB n=19; island-d peak=1.58GiB final=1.55GiB n=19; nat-1 peak=0.02GiB final=0.02GiB n=19; nat-2 peak=0.02GiB final=0.02GiB n=19; registry peak=0.00GiB final=0.00GiB n=19; relay peak=0.02GiB final=0.01GiB n=19; store-1 peak=0.01GiB final=0.01GiB n=12; store-2 peak=0.02GiB final=0.01GiB n=19; store-3 peak=0.02GiB final=0.02GiB n=13; store-4 peak=0.01GiB final=0.01GiB n=19; val-a peak=0.55GiB final=0.55GiB n=19; val-b peak=0.56GiB final=0.56GiB n=19; val-c peak=0.49GiB final=0.47GiB n=19; val-d peak=0.47GiB final=0.43GiB n=18 |
| `priv-unlinkability` | ✅ pass | major |  | default chain REFUSED a durable file→publisher link (refuse-to-surveil) |
| `web-ui-guard` | ✅ pass | major |  | web-UI guard held on a real VM: no-token POST=401 (want 401), DNS-rebinding Host=403 (want 403), token-free read=200 (want 200) |

## Findings (gaps + failures), most severe first

### 184-equivocation — ➖ skip (blocker)
runs on the contained equivocation ISLAND every sheet (flow_equivocation_island — a separate consensus universe; its slash never taxes main-sheet fault tolerance, PE 2026-08-17). This row is the historical pointer; the island row is the graded verdict.

### infra-node-liveness — ❌ fail (blocker)
NODE CRASH-LOOP — 3 node crash(es) (kernel OOM-kill or Go fatal error) across: island-c×3. A run whose cohort DIES cannot grade its flows (a crashing node is indistinguishable from a slow/dead peer), so EVERY verdict on this sheet is PROVISIONAL until a clean no-crash re-run — including the computed bounds (which may be inflated). This is INFRASTRUCTURE FAILURE, not independent flow results. Attribute from a heap profile (re-run with DEBUG_PROFILE=1, then ./cloudtest.sh heap <node>) or the node's journal (fatal-error stack trace) — do NOT presume a cause.

### 10-maturing-handoff — ➖ skip (major)
not a MATURING topology — opt in with MATURING=1 SYBILS=8 ./cloudtest.sh to field-exercise the handoff/post-shed regime (the external red team's sharpest seam-#8 target; until then it is proven only in-process: core/chain/quorum_weight_test.go + sim/maturequorum_test.go)

### 5-sybil-no-capture — ➖ skip (major)
no Sybil cohort in this topology — opt in with SYBILS=8 ./cloudtest.sh to certify the PURE anchor gate on cloud (the local integration/sybil suite reaches only the standing gate)

---

_Generated by `integration/cloudtest`. The field network is ephemeral and was torn down after this run._