# silt field-test report

- **run:** `97e3101-deep`  ·  **silt commit:** `97e3101`  ·  **harness commit:** `cc2cc6f`  ·  **bond mode:** `fast`  ·  **generated:** 2026-09-09T00:49:56Z
- **result:** **REVIEW**  ·  31 pass / 1 gap / 0 fail / 3 skip

## Per-flow verdict

| flow | verdict | severity | elapsed | detail |
|------|---------|----------|---------|--------|
| `1-first-run` | ✅ pass | blocker |  | all silt nodes report service active |
| `10-maturing-handoff` | ✅ pass | major |  | young→mature HANDOFF: latch tripped on the wire; drive reached h83 (target h81) within 5490s |
| `10a-stall-drill` | ✅ pass | major |  | B2 stall drill: with the 4 cheap epoch members DECLINING to attest, the honest >⅔-weight coalition still commits on the wire within the computed 430s bound (head-counted quorum left this exact network born-unable-to-commit at 4×MinBond) |
| `10b-capture-drill` | ✅ pass | major |  | B2 capture drill: the 4 MinBond epoch members alone could NOT advance the mature chain past the honest ceiling h88 (cohort head →88, fresh cohort commit: 1), and it resumed past h88 once honest weight returned — post-shed capture is weight-priced, not head-priced |
| `10c-ws-cold-sync` | ✅ pass | major |  | WS cold-sync under the latch: val-b restarted pinned to checkpoint 90:cddbaccfb50a20ee1d83061cde13959548066814781247f3affc46d88a6f7892, caught up to h90 (sync=1) and came back with the wheels STILL shed (latch_held=1 — a restart must never re-arm the anchors, F-1) |
| `11-economy-repair` | ✅ pass | major |  | the S7 repair economy CLOSED on the wire: killed 3 columns' holders → the caretaker RECONSTRUCTED from parity → a verified-repair bounty drew the object's reserve down (paid=40 credits over 1 repair(s)) — durability paid for itself on a real network, standing untouched (Invariant A). Post-kill cycle: store-2 last-sweep=21/29 stripes-repaired=2; relay last-sweep=29/29 stripes-repaired=2 |
| `11b-economy-skim` | ✅ pass | major |  | the SKIM leg closed on the wire: serve traffic (reconstruction reads + driven fetches) routed revenue into the object's durability reserve on the serving holder's ledger (funded 400000 → 400001 on store-2: +1 pure skim above the prepay baseline) — the object pays for its own repair (S7) |
| `11c-economy-horizon` | ✅ pass | info |  | g-instrumentation sample (S7 finite-but-renewable): paid=40 over 1 repair(s), reserve-after=8, horizonSec=-1 (−1 = no burn window yet). One row per graded run — the g trend needs the series, not this sample |
| `12-deep-heights` | ✅ pass | major |  | DEEP drive (Phase 3 exit gate): honest ceiling reached h130 (target h128, from h93) within 1697s of the 7200s wall (~45s/height measured) |
| `12b-deep-prune` | ✅ pass | major |  | retention prune ENGAGED on every validator at depth (horizon ≈ h64 = epoch-floored h_end−2·TTL): val-a=59pruned/81MiB val-b=59pruned/83MiB val-c=59pruned/84MiB val-d=59pruned/85MiB — payload-stripped counts read from persisted chain.cbor via chain-status, on-disk bytes carried as the weight evidence |
| `12c-deep-converge` | ✅ pass | major |  | convergence at depth on the pruned chain: all validators within 2 of tip=h134 and tip-height validators share head hash f79774ca619f… (val-a=h134:f79774ca619f val-b=h134:f79774ca619f val-c=h134:f79774ca619f val-d=h135:8b3778983e2e) |
| `13-delivery-lane` | ✅ pass | major | 163s | DARK lane (era-4 not active): armed + announced on val-a; client refused at the withdrawal naming the committed E->key binding (nothing spent); server debug.log (+44 lines since baseline) banked nothing; lane-off control at store-2 refused-with-marker; delivery settlement: p=1 credit per 262144 B (U/p=262144 B/credit; self-mint Dλ=393216 B/credit, PF 1.50); anchor face 50000 funds 50000 increments = 13107200000 B (12.21 GiB) per session, k_max=1; one grant = 10 faces, the 64 GiB pin needs 9 faces across delivery+relay; idle window 1m30s; unsettled remainder: a DEPOSIT returned to the fetcher's account when its anchor leaves the 5-epoch guard window (D-R2.9-NODE-HALF-CALLS 1′; the relay lane keeps the burn) |
| `13b-delivery-settlement` | ➖ skip | major | 163s | UNTESTED on this chain: the client was refused at the withdrawal because the issuer served no key that resolves against a COMMITTED E->key binding — on today's networks because era-4 is dark (no binding can commit until the R3.4 stamp raise; no activation override, owner-ratified), though the same sentence would also cover off-commitment keys (no era surface exists to tell them apart: R-CLOUD-ERA-PROBE). Row 13 is what holds today; this row grades the wire settlement once the binding commits, with no harness change. |
| `184-equivocation` | ➖ skip | blocker |  | runs on the contained equivocation ISLAND every sheet (flow_equivocation_island — a separate consensus universe; its slash never taxes main-sheet fault tolerance, PE 2026-08-17). This row is the historical pointer; the island row is the graded verdict. |
| `184-equivocation-island` | ✅ pass | blocker |  | accountability FIRED on the wire: a contained island anchor double-signed and an honest anchor SLASHED it (slashed equivocator 6c5f111568172664ae5c47077f59620f93cf8a352e0ab8d1877c730972f3b701 (double-signed at height 1)) — proven equivocation → permanent eviction (F2), zero blast radius to the main sheet (separate consensus universe) |
| `184-forged-block` | ✅ pass | major |  | forged-signature proposal rejected (adversary logged 'correctly REJECTED by val-a') |
| `184-low-bond` | ⚠️ gap | major |  | adversary holds a qualifying 64M bond and was CORRECTLY accepted as a proposer — an under-bond REJECTION test needs a dedicated sub-min-bond identity (#350); the property is certified in-process (#204) |
| `184-partition` | ✅ pass | major |  | minority val-c STALLED at h32 through the partition (a < ⅓ island cannot commit) then CAUGHT UP to the heal-time majority head h36 (now at h37) on heal — BFT partition→heal reconverged over the real wire (a catch-up, NOT a reorg — a minority never committed a conflicting fork) |
| `2-publish-fetch` | ✅ pass | blocker | 195s | fetched from store-2 bit-perfect |
| `3-care-link` | ✅ pass | minor |  | publish exposes a siltcare: link (repair/audit without the key) |
| `4-become-validator` | ✅ pass | major |  | non-anchor validators earn their OWN standing on the objective path |
| `5-convergence` | ✅ pass | major |  | all validators within 2 of tip=56 AND every tip-height validator shares head hash fd31de41c9b7… (heights: val-a=56:fd31de41c9b7 val-b=56:fd31de41c9b7 val-c=56:fd31de41c9b7 val-d=56:fd31de41c9b7); DURABLE (val-a head 56->56 over 20s, no regression) |
| `5-sybil-no-capture` | ➖ skip | major |  | MATURING=1 topology sheds the anchors by design — the anchor-gate premise doesn't exist here; the post-shed capture property is certified by 10-maturing-handoff's B2 drills (run without MATURING for flow 5) |
| `6-fault-tolerance` | ✅ pass | major |  | publish still committed with one validator (val-d) down (within the computed 190s down-designee escape bound) |
| `7-restart-content` | ✅ pass | major |  | content still fetchable BIT-PERFECT after a storage-node restart |
| `7-restart-standing` | ✅ pass | major | 14s | val-b standing returned after restart without re-bonding |
| `8-takedown` | ✅ pass | major |  | store-1 enforces the operator denylist ([1339]: denylist: honoring 1 denied root(s)) while store-2 still serves BIT-PERFECT (no global switch) |
| `9-cross-nat` | ✅ pass | major |  | natted nodes exchanged a file through the relay/hole-punch |
| `chaos-fetch` | ✅ pass | major |  | content fetchable BIT-PERFECT after a hard-crash (SIGKILL) + restart of a storage node |
| `chaos-reprovide` | ✅ pass | major |  | SIGKILLed storage node re-announced its held chunks (#69) after a hard crash (41s to re-announce; latency scales with held-chunk count, #402/M1) |
| `durability-turnover` | ✅ pass | major |  | content survived a PERMANENT storage-node departure — fetched bit-perfect from a survivor |
| `infra-node-liveness` | ✅ pass | blocker |  | node-liveness precondition HELD — no OOM-kill or crash-loop across the cohort, so the sheet was graded on a HEALTHY network |
| `infra-node-memory` | ✅ pass | info |  | RSS envelope measured (cgroup MemoryCurrent, every 30s → rss-97e3101-deep.jsonl): worst peak 0.80GiB across the cohort. adversary peak=0.62GiB final=0.62GiB n=3; fetch-1 peak=0.01GiB final=0.01GiB n=3; island-a peak=0.38GiB final=0.38GiB n=3; island-b peak=0.40GiB final=0.35GiB n=3; island-c peak=0.41GiB final=0.39GiB n=3; island-d peak=0.39GiB final=0.35GiB n=3; maturer-1 peak=0.80GiB final=0.80GiB n=3; maturer-2 peak=0.78GiB final=0.78GiB n=3; maturer-3 peak=0.76GiB final=0.76GiB n=3; maturer-4 peak=0.78GiB final=0.78GiB n=3; nat-1 peak=0.01GiB final=0.01GiB n=3; nat-2 peak=0.01GiB final=0.01GiB n=3; registry peak=0.00GiB final=0.00GiB n=2; relay peak=0.01GiB final=0.01GiB n=2; store-1 peak=0.01GiB final=0.01GiB n=2; store-2 peak=0.01GiB final=0.01GiB n=2; store-3 peak=0.01GiB final=0.01GiB n=2; store-4 peak=0.01GiB final=0.01GiB n=2; sybil-1 peak=0.31GiB final=0.31GiB n=2; sybil-2 peak=0.38GiB final=0.38GiB n=2; sybil-3 peak=0.46GiB final=0.46GiB n=2; sybil-4 peak=0.41GiB final=0.41GiB n=2; val-a peak=0.54GiB final=0.54GiB n=2; val-b peak=0.62GiB final=0.62GiB n=2; val-c peak=0.57GiB final=0.57GiB n=2; val-d peak=0.57GiB final=0.57GiB n=2 |
| `priv-unlinkability` | ✅ pass | major |  | default chain REFUSED a durable file→publisher link (refuse-to-surveil) |
| `web-ui-guard` | ✅ pass | major |  | web-UI guard held on a real VM: no-token POST=401 (want 401), DNS-rebinding Host=403 (want 403), token-free read=200 (want 200) |

## Findings (gaps + failures), most severe first

### 184-equivocation — ➖ skip (blocker)
runs on the contained equivocation ISLAND every sheet (flow_equivocation_island — a separate consensus universe; its slash never taxes main-sheet fault tolerance, PE 2026-08-17). This row is the historical pointer; the island row is the graded verdict.

### 13b-delivery-settlement — ➖ skip (major)
UNTESTED on this chain: the client was refused at the withdrawal because the issuer served no key that resolves against a COMMITTED E->key binding — on today's networks because era-4 is dark (no binding can commit until the R3.4 stamp raise; no activation override, owner-ratified), though the same sentence would also cover off-commitment keys (no era surface exists to tell them apart: R-CLOUD-ERA-PROBE). Row 13 is what holds today; this row grades the wire settlement once the binding commits, with no harness change.

### 184-low-bond — ⚠️ gap (major)
adversary holds a qualifying 64M bond and was CORRECTLY accepted as a proposer — an under-bond REJECTION test needs a dedicated sub-min-bond identity (#350); the property is certified in-process (#204)

### 5-sybil-no-capture — ➖ skip (major)
MATURING=1 topology sheds the anchors by design — the anchor-gate premise doesn't exist here; the post-shed capture property is certified by 10-maturing-handoff's B2 drills (run without MATURING for flow 5)

---

_Generated by `integration/cloudtest`. The field network is ephemeral and was torn down after this run._