# silt field-test report

- **run:** `c450985-deep`  ·  **silt commit:** `c450985`  ·  **harness commit:** `3ea8449`  ·  **bond mode:** `fast`  ·  **generated:** 2026-09-07T10:18:16Z
- **result:** **REVIEW**  ·  29 pass / 2 gap / 0 fail / 2 skip

## Per-flow verdict

| flow | verdict | severity | elapsed | detail |
|------|---------|----------|---------|--------|
| `1-first-run` | ✅ pass | blocker |  | all silt nodes report service active |
| `10-maturing-handoff` | ✅ pass | major |  | young→mature HANDOFF: latch tripped on the wire; drive reached h54 (target h49) within 5490s |
| `10a-stall-drill` | ✅ pass | major |  | B2 stall drill: with the 4 cheap epoch members DECLINING to attest, the honest >⅔-weight coalition still commits on the wire within the computed 430s bound (head-counted quorum left this exact network born-unable-to-commit at 4×MinBond) |
| `10b-capture-drill` | ✅ pass | major |  | B2 capture drill: the 4 MinBond epoch members alone could NOT advance the mature chain past the honest ceiling h58 (cohort head →58, fresh cohort commit: 1), and it resumed past h58 once honest weight returned — post-shed capture is weight-priced, not head-priced |
| `10c-ws-cold-sync` | ✅ pass | major |  | WS cold-sync under the latch: val-b restarted pinned to checkpoint 59:c5a34afcf41b813d703d6b60bf768c8a4570a449f23b2174a353f20ef6f4c270, caught up to h60 (sync=1) and came back with the wheels STILL shed (latch_held=1 — a restart must never re-arm the anchors, F-1) |
| `11-economy-repair` | ✅ pass | major |  | the S7 repair economy CLOSED on the wire: killed 3 columns' holders → the caretaker RECONSTRUCTED from parity → a verified-repair bounty drew the object's reserve down (paid=170 credits over 7 repair(s)) — durability paid for itself on a real network, standing untouched (Invariant A). Post-kill cycle: store-2 last-sweep=29/29 stripes-repaired=2; relay last-sweep=29/29 stripes-repaired=2 |
| `11b-economy-skim` | ✅ pass | major |  | the SKIM leg closed on the wire: serve traffic (reconstruction reads + driven fetches) routed revenue into the object's durability reserve on the serving holder's ledger (funded 400000 → 400002 on relay: +2 pure skim above the prepay baseline) — the object pays for its own repair (S7) |
| `11c-economy-horizon` | ✅ pass | info |  | g-instrumentation sample (S7 finite-but-renewable): paid=170 over 7 repair(s), reserve-after=8, horizonSec=860551 (−1 = no burn window yet). One row per graded run — the g trend needs the series, not this sample |
| `12-deep-heights` | ✅ pass | major |  | DEEP drive (Phase 3 exit gate): honest ceiling reached h128 (target h128, from h64) within 2877s of the 7200s wall (~44s/height measured) |
| `12b-deep-prune` | ✅ pass | major |  | retention prune ENGAGED on every validator at depth (horizon ≈ h64 = epoch-floored h_end−2·TTL): val-a=56pruned/78MiB val-b=56pruned/80MiB val-c=51pruned/80MiB val-d=56pruned/80MiB — payload-stripped counts read from persisted chain.cbor via chain-status, on-disk bytes carried as the weight evidence |
| `12c-deep-converge` | ✅ pass | major |  | convergence at depth on the pruned chain: all validators within 2 of tip=h129 and tip-height validators share head hash a7126b8a27d3… (val-a=h129:a7126b8a27d3 val-b=h130:0cce25cc0b9c val-c=h130:0cce25cc0b9c val-d=h131:8f07eed95249) |
| `184-equivocation` | ➖ skip | blocker |  | runs on the contained equivocation ISLAND every sheet (flow_equivocation_island — a separate consensus universe; its slash never taxes main-sheet fault tolerance, PE 2026-08-17). This row is the historical pointer; the island row is the graded verdict. |
| `184-equivocation-island` | ✅ pass | blocker |  | accountability FIRED on the wire: a contained island anchor double-signed and an honest anchor SLASHED it (slashed equivocator 6c5f111568172664ae5c47077f59620f93cf8a352e0ab8d1877c730972f3b701 (double-signed at height 1)) — proven equivocation → permanent eviction (F2), zero blast radius to the main sheet (separate consensus universe) |
| `184-forged-block` | ✅ pass | major |  | forged-signature proposal rejected (adversary logged 'correctly REJECTED by val-a') |
| `184-low-bond` | ⚠️ gap | major |  | adversary holds a qualifying 64M bond and was CORRECTLY accepted as a proposer — an under-bond REJECTION test needs a dedicated sub-min-bond identity (#350); the property is certified in-process (#204) |
| `184-partition` | ✅ pass | major |  | minority val-c STALLED at h25 through the partition (a < ⅓ island cannot commit) then CAUGHT UP to the heal-time majority head h34 (now at h34) on heal — BFT partition→heal reconverged over the real wire (a catch-up, NOT a reorg — a minority never committed a conflicting fork) |
| `2-publish-fetch` | ✅ pass | blocker | 82s | fetched from store-2 bit-perfect |
| `3-care-link` | ✅ pass | minor |  | publish exposes a siltcare: link (repair/audit without the key) |
| `4-become-validator` | ✅ pass | major |  | non-anchor validators earn their OWN standing on the objective path |
| `5-convergence` | ✅ pass | major |  | all validators within 2 of tip=42 AND every tip-height validator shares head hash a000cbba9c4f… (heights: val-a=42:a000cbba9c4f val-b=42:a000cbba9c4f val-c=42:a000cbba9c4f val-d=42:a000cbba9c4f); DURABLE (val-a head 42->42 over 20s, no regression) |
| `5-sybil-no-capture` | ➖ skip | major |  | MATURING=1 topology sheds the anchors by design — the anchor-gate premise doesn't exist here; the post-shed capture property is certified by 10-maturing-handoff's B2 drills (run without MATURING for flow 5) |
| `6-fault-tolerance` | ⚠️ gap | major |  | no new commit within the computed 1445s r≤5 hard cap with val-d down (fingerprint  → n/a: ladder advancing but uncommitted — OUT OF MODEL) — read the captured client error (publish-diag / .ft_publish_lasterr) and survivor journals before attributing (#509/#7) |
| `7-restart-content` | ✅ pass | major |  | content still fetchable BIT-PERFECT after a storage-node restart |
| `7-restart-standing` | ✅ pass | major | 10s | val-b standing returned after restart without re-bonding |
| `8-takedown` | ✅ pass | major |  | store-1 enforces the operator denylist ([2526]: denylist: honoring 1 denied root(s)) while store-2 still serves BIT-PERFECT (no global switch) |
| `9-cross-nat` | ✅ pass | major |  | natted nodes exchanged a file through the relay/hole-punch |
| `chaos-fetch` | ✅ pass | major |  | content fetchable BIT-PERFECT after a hard-crash (SIGKILL) + restart of a storage node |
| `chaos-reprovide` | ✅ pass | major |  | SIGKILLed storage node re-announced its held chunks (#69) after a hard crash (21s to re-announce; latency scales with held-chunk count, #402/M1) |
| `durability-turnover` | ✅ pass | major |  | content survived a PERMANENT storage-node departure — fetched bit-perfect from a survivor |
| `infra-node-liveness` | ✅ pass | blocker |  | node-liveness precondition HELD — no OOM-kill or crash-loop across the cohort, so the sheet was graded on a HEALTHY network |
| `infra-node-memory` | ✅ pass | info |  | RSS envelope measured (cgroup MemoryCurrent, every 30s → rss-c450985-deep.jsonl): worst peak 1.58GiB across the cohort. adversary peak=1.58GiB final=1.12GiB n=37; fetch-1 peak=0.01GiB final=0.01GiB n=36; island-a peak=0.34GiB final=0.29GiB n=37; island-b peak=0.39GiB final=0.31GiB n=37; island-c peak=0.37GiB final=0.30GiB n=37; island-d peak=0.39GiB final=0.31GiB n=37; maturer-1 peak=1.51GiB final=1.27GiB n=35; maturer-2 peak=1.43GiB final=1.29GiB n=36; maturer-3 peak=1.51GiB final=1.23GiB n=36; maturer-4 peak=1.43GiB final=0.64GiB n=36; nat-1 peak=0.02GiB final=0.02GiB n=37; nat-2 peak=0.02GiB final=0.02GiB n=37; registry peak=0.00GiB final=0.00GiB n=37; relay peak=0.03GiB final=0.02GiB n=37; store-1 peak=0.02GiB final=0.01GiB n=37; store-2 peak=0.02GiB final=0.02GiB n=37; store-3 peak=0.02GiB final=0.02GiB n=37; store-4 peak=0.02GiB final=0.02GiB n=37; sybil-1 peak=1.12GiB final=1.08GiB n=36; sybil-2 peak=1.34GiB final=0.89GiB n=36; sybil-3 peak=1.49GiB final=0.98GiB n=35; sybil-4 peak=1.18GiB final=1.08GiB n=36; val-a peak=1.34GiB final=1.11GiB n=35; val-b peak=1.34GiB final=1.21GiB n=35; val-c peak=1.43GiB final=1.37GiB n=35; val-d peak=1.42GiB final=1.02GiB n=33 |
| `priv-unlinkability` | ✅ pass | major |  | default chain REFUSED a durable file→publisher link (refuse-to-surveil) |
| `web-ui-guard` | ✅ pass | major |  | web-UI guard held on a real VM: no-token POST=401 (want 401), DNS-rebinding Host=403 (want 403), token-free read=200 (want 200) |

## Findings (gaps + failures), most severe first

### 184-equivocation — ➖ skip (blocker)
runs on the contained equivocation ISLAND every sheet (flow_equivocation_island — a separate consensus universe; its slash never taxes main-sheet fault tolerance, PE 2026-08-17). This row is the historical pointer; the island row is the graded verdict.

### 184-low-bond — ⚠️ gap (major)
adversary holds a qualifying 64M bond and was CORRECTLY accepted as a proposer — an under-bond REJECTION test needs a dedicated sub-min-bond identity (#350); the property is certified in-process (#204)

### 5-sybil-no-capture — ➖ skip (major)
MATURING=1 topology sheds the anchors by design — the anchor-gate premise doesn't exist here; the post-shed capture property is certified by 10-maturing-handoff's B2 drills (run without MATURING for flow 5)

### 6-fault-tolerance — ⚠️ gap (major)
no new commit within the computed 1445s r≤5 hard cap with val-d down (fingerprint  → n/a: ladder advancing but uncommitted — OUT OF MODEL) — read the captured client error (publish-diag / .ft_publish_lasterr) and survivor journals before attributing (#509/#7)

---

_Generated by `integration/cloudtest`. The field network is ephemeral and was torn down after this run._