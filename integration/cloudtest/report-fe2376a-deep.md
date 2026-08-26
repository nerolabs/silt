# silt field-test report

- **run:** `fe2376a-deep`  ·  **silt commit:** `fe2376a`  ·  **harness commit:** `fe2376a`  ·  **bond mode:** `fast`  ·  **generated:** 2026-08-26T15:39:17Z
- **result:** **REVIEW**  ·  30 pass / 1 gap / 0 fail / 2 skip

## Per-flow verdict

| flow | verdict | severity | elapsed | detail |
|------|---------|----------|---------|--------|
| `1-first-run` | ✅ pass | blocker |  | all silt nodes report service active |
| `10-maturing-handoff` | ✅ pass | major |  | young→mature HANDOFF: latch tripped on the wire; drive reached h69 (target h65) within 5490s |
| `10a-stall-drill` | ✅ pass | major |  | B2 stall drill: with the 4 cheap epoch members DECLINING to attest, the honest >⅔-weight coalition still commits on the wire within the computed 430s bound (head-counted quorum left this exact network born-unable-to-commit at 4×MinBond) |
| `10b-capture-drill` | ✅ pass | major |  | B2 capture drill: the 4 MinBond epoch members alone could NOT advance the mature chain past the honest ceiling h73 (cohort head →73, fresh cohort commit: 1), and it resumed past h73 once honest weight returned — post-shed capture is weight-priced, not head-priced |
| `10c-ws-cold-sync` | ✅ pass | major |  | WS cold-sync under the latch: val-b restarted pinned to checkpoint 74:f230833807b39089db1945e5b29dbfd93dd49132d0c8cafdd4909eef6876fdcf, caught up to h74 (sync=1) and came back with the wheels STILL shed (latch_held=1 — a restart must never re-arm the anchors, F-1) |
| `11-economy-repair` | ✅ pass | major |  | the S7 repair economy CLOSED on the wire: killed 3 columns' holders → the caretaker RECONSTRUCTED from parity → a verified-repair bounty drew the object's reserve down (paid=400000 credits over 1 repair(s)) — durability paid for itself on a real network, standing untouched (Invariant A). Post-kill cycle: store-2 last-sweep=21/29 stripes-repaired=0; relay last-sweep=21/29 stripes-repaired=1 |
| `11b-economy-skim` | ⚠️ gap | major |  | no skim grew EITHER armed observer's reserve above its prepay baseline (repair-window reads + 90s driven fetches; store-2 400000→400000, relay 400000→400000) — attribute from their journals (serve accounting / proofMeta root routing) before re-running (#7) |
| `11c-economy-horizon` | ✅ pass | info |  | g-instrumentation sample (S7 finite-but-renewable): paid=400000 over 1 repair(s), reserve-after=400000, horizonSec=-1 (−1 = no burn window yet). One row per graded run — the g trend needs the series, not this sample |
| `12-deep-heights` | ✅ pass | major |  | DEEP drive (Phase 3 exit gate): honest ceiling reached h132 (target h128, from h78) within 2615s of the 7200s wall (~48s/height measured) |
| `12b-deep-prune` | ✅ pass | major |  | retention prune ENGAGED on every validator at depth (horizon ≈ h64 = epoch-floored h_end−2·TTL): val-a=59pruned/86MiB val-b=59pruned/86MiB val-c=59pruned/86MiB val-d=59pruned/87MiB — payload-stripped counts read from persisted chain.cbor via chain-status, on-disk bytes carried as the weight evidence |
| `12c-deep-converge` | ✅ pass | major |  | convergence at depth on the pruned chain: all validators within 2 of tip=h134 and tip-height validators share head hash 3d839de112d8… (val-a=h134:3d839de112d8 val-b=h135:3761e241252d val-c=h135:3761e241252d val-d=h136:99c9dc605f41) |
| `184-equivocation` | ➖ skip | blocker |  | runs on the contained equivocation ISLAND every sheet (flow_equivocation_island — a separate consensus universe; its slash never taxes main-sheet fault tolerance, PE 2026-08-17). This row is the historical pointer; the island row is the graded verdict. |
| `184-equivocation-island` | ✅ pass | blocker |  | accountability FIRED on the wire: a contained island anchor double-signed and an honest anchor SLASHED it (slashed equivocator 6c5f111568172664ae5c47077f59620f93cf8a352e0ab8d1877c730972f3b701 (double-signed at height 1)) — proven equivocation → permanent eviction (F2), zero blast radius to the main sheet (separate consensus universe) |
| `184-forged-block` | ✅ pass | major |  | forged-signature proposal rejected (adversary logged 'correctly REJECTED by val-a') |
| `184-low-bond` | ✅ pass | major |  | under-bonded proposer rejected (adversary logged 'correctly REJECTED by val-a') |
| `184-partition` | ✅ pass | major |  | minority val-c STALLED at h29 through the partition (a < ⅓ island cannot commit) then CAUGHT UP to the heal-time majority head h33 (now at h33) on heal — BFT partition→heal reconverged over the real wire (a catch-up, NOT a reorg — a minority never committed a conflicting fork) |
| `2-publish-fetch` | ✅ pass | blocker | 120s | fetched from store-2 bit-perfect |
| `3-care-link` | ✅ pass | minor |  | publish exposes a siltcare: link (repair/audit without the key) |
| `4-become-validator` | ✅ pass | major |  | non-anchor validators earn their OWN standing on the objective path |
| `5-convergence` | ✅ pass | major |  | all validators within 2 of tip=57 AND every tip-height validator shares head hash ecbd2dd4d566… (heights: val-a=57:ecbd2dd4d566 val-b=57:ecbd2dd4d566 val-c=57:ecbd2dd4d566 val-d=57:ecbd2dd4d566); DURABLE (val-a head 57->57 over 20s, no regression) |
| `5-sybil-no-capture` | ➖ skip | major |  | MATURING=1 topology sheds the anchors by design — the anchor-gate premise doesn't exist here; the post-shed capture property is certified by 10-maturing-handoff's B2 drills (run without MATURING for flow 5) |
| `6-fault-tolerance` | ✅ pass | major |  | publish still committed with one validator (val-d) down (within the computed 650s down-designee escape bound) |
| `7-restart-content` | ✅ pass | major |  | content still fetchable BIT-PERFECT after a storage-node restart |
| `7-restart-standing` | ✅ pass | major | 8s | val-b standing returned after restart without re-bonding |
| `8-takedown` | ✅ pass | major |  | store-1 enforces the operator denylist ([1685]: denylist: honoring 1 denied root(s)) while store-2 still serves BIT-PERFECT (no global switch) |
| `9-cross-nat` | ✅ pass | major |  | natted nodes exchanged a file through the relay/hole-punch |
| `chaos-fetch` | ✅ pass | major |  | content fetchable BIT-PERFECT after a hard-crash (SIGKILL) + restart of a storage node |
| `chaos-reprovide` | ✅ pass | major |  | SIGKILLed storage node re-announced its held chunks (#69) after a hard crash (34s to re-announce; latency scales with held-chunk count, #402/M1) |
| `durability-turnover` | ✅ pass | major |  | content survived a PERMANENT storage-node departure — fetched bit-perfect from a survivor |
| `infra-node-liveness` | ✅ pass | blocker |  | node-liveness precondition HELD — no OOM-kill or crash-loop across the cohort, so the sheet was graded on a HEALTHY network |
| `infra-node-memory` | ✅ pass | info |  | RSS envelope measured (cgroup MemoryCurrent, every 30s → rss-fe2376a-deep.jsonl): worst peak 0.65GiB across the cohort. adversary peak=0.46GiB final=0.46GiB n=3; fetch-1 peak=0.01GiB final=0.01GiB n=3; island-a peak=0.36GiB final=0.36GiB n=3; island-b peak=0.36GiB final=0.32GiB n=3; island-c peak=0.36GiB final=0.35GiB n=3; island-d peak=0.39GiB final=0.37GiB n=3; maturer-1 peak=0.65GiB final=0.65GiB n=3; maturer-2 peak=0.54GiB final=0.54GiB n=3; maturer-3 peak=0.52GiB final=0.52GiB n=2; maturer-4 peak=0.45GiB final=0.45GiB n=2; nat-1 peak=0.01GiB final=0.01GiB n=2; nat-2 peak=0.01GiB final=0.01GiB n=2; registry peak=0.00GiB final=0.00GiB n=2; relay peak=0.01GiB final=0.01GiB n=2; store-1 peak=0.01GiB final=0.01GiB n=2; store-2 peak=0.01GiB final=0.01GiB n=2; store-3 peak=0.01GiB final=0.01GiB n=2; store-4 peak=0.01GiB final=0.01GiB n=2; sybil-1 peak=0.27GiB final=0.27GiB n=2; sybil-2 peak=0.30GiB final=0.30GiB n=2; sybil-3 peak=0.30GiB final=0.30GiB n=2; sybil-4 peak=0.31GiB final=0.31GiB n=2; val-a peak=0.52GiB final=0.52GiB n=2; val-b peak=0.49GiB final=0.49GiB n=2; val-c peak=0.55GiB final=0.55GiB n=2; val-d peak=0.56GiB final=0.56GiB n=2 |
| `priv-unlinkability` | ✅ pass | major |  | default chain REFUSED a durable file→publisher link (refuse-to-surveil) |
| `web-ui-guard` | ✅ pass | major |  | web-UI guard held on a real VM: no-token POST=401 (want 401), DNS-rebinding Host=403 (want 403), token-free read=200 (want 200) |

## Findings (gaps + failures), most severe first

### 184-equivocation — ➖ skip (blocker)
runs on the contained equivocation ISLAND every sheet (flow_equivocation_island — a separate consensus universe; its slash never taxes main-sheet fault tolerance, PE 2026-08-17). This row is the historical pointer; the island row is the graded verdict.

### 11b-economy-skim — ⚠️ gap (major)
no skim grew EITHER armed observer's reserve above its prepay baseline (repair-window reads + 90s driven fetches; store-2 400000→400000, relay 400000→400000) — attribute from their journals (serve accounting / proofMeta root routing) before re-running (#7)

### 5-sybil-no-capture — ➖ skip (major)
MATURING=1 topology sheds the anchors by design — the anchor-gate premise doesn't exist here; the post-shed capture property is certified by 10-maturing-handoff's B2 drills (run without MATURING for flow 5)

---

_Generated by `integration/cloudtest`. The field network is ephemeral and was torn down after this run._