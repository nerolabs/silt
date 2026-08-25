# silt field-test report

- **run:** `a434494-deep`  ·  **silt commit:** `a434494`  ·  **harness commit:** `95d39e8`  ·  **bond mode:** `fast`  ·  **generated:** 2026-08-25T10:02:21Z
- **result:** **FAIL**  ·  27 pass / 0 gap / 4 fail / 2 skip

## Per-flow verdict

| flow | verdict | severity | elapsed | detail |
|------|---------|----------|---------|--------|
| `1-first-run` | ✅ pass | blocker |  | all silt nodes report service active |
| `10-maturing-handoff` | ✅ pass | major |  | young→mature HANDOFF: latch tripped on the wire; drive reached h70 (target h65) within 5490s |
| `10a-stall-drill` | ❌ fail | major |  | B2 stall drill: with the 4 cheap epoch members DECLINING to attest, the honest >⅔-weight coalition still commits on the wire within the computed 430s bound (head-counted quorum left this exact network born-unable-to-commit at 4×MinBond) |
| `10b-capture-drill` | ✅ pass | major |  | B2 capture drill: the 4 MinBond epoch members alone could NOT advance the mature chain past the honest ceiling h73 (cohort head →73, fresh cohort commit: 1), and it resumed past h73 once honest weight returned — post-shed capture is weight-priced, not head-priced |
| `10c-ws-cold-sync` | ✅ pass | major |  | WS cold-sync under the latch: val-b restarted pinned to checkpoint 70:369579b2333e4ad2a795f928e4a35fc4efb6bb87265dd2a9ccb8fb54d0f0f835, caught up to h74 (sync=1) and came back with the wheels STILL shed (latch_held=1 — a restart must never re-arm the anchors, F-1) |
| `11-economy-repair` | ✅ pass | major |  | the S7 repair economy CLOSED on the wire: killed 3 columns' holders → the caretaker RECONSTRUCTED from parity → a verified-repair bounty drew the object's reserve down (paid=400000 credits over 1 repair(s)) — durability paid for itself on a real network, standing untouched (Invariant A). Post-kill cycle: store-2 last-sweep=21/29 stripes-repaired=2; relay last-sweep=21/29 stripes-repaired=2 |
| `11b-economy-skim` | ✅ pass | major |  | the SKIM leg closed on the wire: serve traffic (reconstruction reads + driven fetches) routed revenue into the object's durability reserve on the serving holder's ledger (funded 400000 → 727700 on store-2: +327700 pure skim above the prepay baseline) — the object pays for its own repair (S7) |
| `11c-economy-horizon` | ✅ pass | info |  | g-instrumentation sample (S7 finite-but-renewable): paid=400000 over 1 repair(s), reserve-after=0, horizonSec=0 (−1 = no burn window yet). One row per graded run — the g trend needs the series, not this sample |
| `12-deep-heights` | ❌ fail | major |  | DEEP drive (Phase 3 exit gate): honest ceiling reached h90 (target h128, from h75) within 1320s of the 7200s wall (~88s/height measured) — TARGET NOT REACHED: a crawl/stall at depth is the Phase 3 finding itself; attribute from the validator journals |
| `12b-deep-prune` | ✅ pass | major |  | retention prune ENGAGED on every validator at depth (horizon ≈ h24 = epoch-floored h_end−2·TTL): val-a=23pruned/82MiB val-b=23pruned/82MiB val-c=23pruned/82MiB val-d=15pruned/87MiB — payload-stripped counts read from persisted chain.cbor via chain-status, on-disk bytes carried as the weight evidence |
| `12c-deep-converge` | ❌ fail | major |  | convergence at depth on the pruned chain: all validators within 2 of tip=h90 and tip-height validators share head hash 4f3cf0814109… (val-a=h90:4f3cf0814109 val-b=h90:4f3cf0814109 val-c=h90:4f3cf0814109 val-d=h83:ce4d30abd39d) |
| `184-equivocation` | ➖ skip | blocker |  | runs on the contained equivocation ISLAND every sheet (flow_equivocation_island — a separate consensus universe; its slash never taxes main-sheet fault tolerance, PE 2026-08-17). This row is the historical pointer; the island row is the graded verdict. |
| `184-equivocation-island` | ✅ pass | blocker |  | accountability FIRED on the wire: a contained island anchor double-signed and an honest anchor SLASHED it (slashed equivocator 6c5f111568172664ae5c47077f59620f93cf8a352e0ab8d1877c730972f3b701 (double-signed at height 1)) — proven equivocation → permanent eviction (F2), zero blast radius to the main sheet (separate consensus universe) |
| `184-forged-block` | ✅ pass | major |  | forged-signature proposal rejected (adversary logged 'correctly REJECTED by val-a') |
| `184-low-bond` | ✅ pass | major |  | under-bonded proposer rejected (adversary logged 'correctly REJECTED by val-a') |
| `184-partition` | ✅ pass | major |  | minority val-c STALLED at h42 through the partition (a < ⅓ island cannot commit) then CAUGHT UP to the heal-time majority head h47 (now at h47) on heal — BFT partition→heal reconverged over the real wire (a catch-up, NOT a reorg — a minority never committed a conflicting fork) |
| `2-publish-fetch` | ✅ pass | blocker | 78s | fetched from store-2 bit-perfect |
| `3-care-link` | ✅ pass | minor |  | publish exposes a siltcare: link (repair/audit without the key) |
| `4-become-validator` | ✅ pass | major |  | non-anchor validators earn their OWN standing on the objective path |
| `5-convergence` | ✅ pass | major |  | all validators within 2 of tip=23 AND every tip-height validator shares head hash 6cab5bbf44b3… (heights: val-a=22:810ad7abdc98 val-b=22:810ad7abdc98 val-c=23:6cab5bbf44b3 val-d=23:6cab5bbf44b3); DURABLE (val-a head 22->23 over 20s, no regression) |
| `5-sybil-no-capture` | ➖ skip | major |  | MATURING=1 topology sheds the anchors by design — the anchor-gate premise doesn't exist here; the post-shed capture property is certified by 10-maturing-handoff's B2 drills (run without MATURING for flow 5) |
| `6-fault-tolerance` | ✅ pass | major |  | publish still committed with one validator (val-d) down (within the computed 650s down-designee escape bound) |
| `7-restart-content` | ✅ pass | major |  | content still fetchable BIT-PERFECT after a storage-node restart |
| `7-restart-standing` | ✅ pass | major | 12s | val-b standing returned after restart without re-bonding |
| `8-takedown` | ✅ pass | major |  | store-1 enforces the operator denylist ([2305]: denylist: honoring 1 denied root(s)) while store-2 still serves BIT-PERFECT (no global switch) |
| `9-cross-nat` | ✅ pass | major |  | natted nodes exchanged a file through the relay/hole-punch |
| `chaos-fetch` | ✅ pass | major |  | content fetchable BIT-PERFECT after a hard-crash (SIGKILL) + restart of a storage node |
| `chaos-reprovide` | ✅ pass | major |  | SIGKILLed storage node re-announced its held chunks (#69) after a hard crash (26s to re-announce; latency scales with held-chunk count, #402/M1) |
| `durability-turnover` | ✅ pass | major |  | content survived a PERMANENT storage-node departure — fetched bit-perfect from a survivor |
| `infra-node-liveness` | ❌ fail | blocker |  | NODE CRASH-LOOP — 6 node crash(es) (kernel OOM-kill or Go fatal error) across: val-d×6. A run whose cohort DIES cannot grade its flows (a crashing node is indistinguishable from a slow/dead peer), so EVERY verdict on this sheet is PROVISIONAL until a clean no-crash re-run — including the computed bounds (which may be inflated). This is INFRASTRUCTURE FAILURE, not independent flow results. Journals captured to failed-nodes-a434494-deep.log (#504); attribute from those + a heap profile (re-run with DEBUG_PROFILE=1, then ./cloudtest.sh heap <node>) — do NOT presume a cause. |
| `infra-node-memory` | ✅ pass | info |  | RSS envelope measured (cgroup MemoryCurrent, every 30s → rss-a434494-deep.jsonl): worst peak 1.44GiB across the cohort. adversary peak=1.39GiB final=0.66GiB n=35; fetch-1 peak=0.02GiB final=0.02GiB n=36; island-a peak=0.35GiB final=0.29GiB n=36; island-b peak=0.35GiB final=0.30GiB n=36; island-c peak=0.38GiB final=0.30GiB n=36; island-d peak=0.35GiB final=0.29GiB n=36; maturer-1 peak=1.39GiB final=0.92GiB n=35; maturer-2 peak=1.38GiB final=0.61GiB n=35; maturer-3 peak=1.44GiB final=0.87GiB n=35; maturer-4 peak=1.38GiB final=0.62GiB n=35; nat-1 peak=0.02GiB final=0.02GiB n=36; nat-2 peak=0.02GiB final=0.02GiB n=36; registry peak=0.01GiB final=0.01GiB n=36; relay peak=0.03GiB final=0.02GiB n=36; store-1 peak=0.01GiB final=0.01GiB n=36; store-2 peak=0.03GiB final=0.01GiB n=36; store-3 peak=0.02GiB final=0.01GiB n=35; store-4 peak=0.02GiB final=0.02GiB n=36; sybil-1 peak=1.34GiB final=0.53GiB n=32; sybil-2 peak=1.22GiB final=0.48GiB n=32; sybil-3 peak=1.15GiB final=0.49GiB n=31; sybil-4 peak=1.18GiB final=0.44GiB n=32; val-a peak=1.24GiB final=0.60GiB n=35; val-b peak=1.37GiB final=0.91GiB n=35; val-c peak=1.42GiB final=0.90GiB n=35; val-d peak=1.43GiB final=0.37GiB n=30 |
| `priv-unlinkability` | ✅ pass | major |  | default chain REFUSED a durable file→publisher link (refuse-to-surveil) |
| `web-ui-guard` | ✅ pass | major |  | web-UI guard held on a real VM: no-token POST=401 (want 401), DNS-rebinding Host=403 (want 403), token-free read=200 (want 200) |

## Findings (gaps + failures), most severe first

### 184-equivocation — ➖ skip (blocker)
runs on the contained equivocation ISLAND every sheet (flow_equivocation_island — a separate consensus universe; its slash never taxes main-sheet fault tolerance, PE 2026-08-17). This row is the historical pointer; the island row is the graded verdict.

### infra-node-liveness — ❌ fail (blocker)
NODE CRASH-LOOP — 6 node crash(es) (kernel OOM-kill or Go fatal error) across: val-d×6. A run whose cohort DIES cannot grade its flows (a crashing node is indistinguishable from a slow/dead peer), so EVERY verdict on this sheet is PROVISIONAL until a clean no-crash re-run — including the computed bounds (which may be inflated). This is INFRASTRUCTURE FAILURE, not independent flow results. Journals captured to failed-nodes-a434494-deep.log (#504); attribute from those + a heap profile (re-run with DEBUG_PROFILE=1, then ./cloudtest.sh heap <node>) — do NOT presume a cause.

### 10a-stall-drill — ❌ fail (major)
B2 stall drill: with the 4 cheap epoch members DECLINING to attest, the honest >⅔-weight coalition still commits on the wire within the computed 430s bound (head-counted quorum left this exact network born-unable-to-commit at 4×MinBond)

### 12-deep-heights — ❌ fail (major)
DEEP drive (Phase 3 exit gate): honest ceiling reached h90 (target h128, from h75) within 1320s of the 7200s wall (~88s/height measured) — TARGET NOT REACHED: a crawl/stall at depth is the Phase 3 finding itself; attribute from the validator journals

### 12c-deep-converge — ❌ fail (major)
convergence at depth on the pruned chain: all validators within 2 of tip=h90 and tip-height validators share head hash 4f3cf0814109… (val-a=h90:4f3cf0814109 val-b=h90:4f3cf0814109 val-c=h90:4f3cf0814109 val-d=h83:ce4d30abd39d)

### 5-sybil-no-capture — ➖ skip (major)
MATURING=1 topology sheds the anchors by design — the anchor-gate premise doesn't exist here; the post-shed capture property is certified by 10-maturing-handoff's B2 drills (run without MATURING for flow 5)

---

_Generated by `integration/cloudtest`. The field network is ephemeral and was torn down after this run._