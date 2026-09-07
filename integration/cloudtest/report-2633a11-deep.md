# silt field-test report

- **run:** `2633a11-deep`  ·  **silt commit:** `f37bf71`  ·  **harness commit:** `bf1dab9`  ·  **bond mode:** `fast`  ·  **generated:** 2026-09-07T16:52:34Z
- **result:** **REVIEW**  ·  30 pass / 2 gap / 0 fail / 3 skip

## Per-flow verdict

| flow | verdict | severity | elapsed | detail |
|------|---------|----------|---------|--------|
| `1-first-run` | ✅ pass | blocker |  | all silt nodes report service active |
| `10-maturing-handoff` | ✅ pass | major |  | young→mature HANDOFF: latch tripped on the wire; drive reached h65 (target h65) within 5490s |
| `10a-stall-drill` | ✅ pass | major |  | B2 stall drill: with the 4 cheap epoch members DECLINING to attest, the honest >⅔-weight coalition still commits on the wire within the computed 430s bound (head-counted quorum left this exact network born-unable-to-commit at 4×MinBond) |
| `10b-capture-drill` | ✅ pass | major |  | B2 capture drill: the 4 MinBond epoch members alone could NOT advance the mature chain past the honest ceiling h70 (cohort head →71, fresh cohort commit: 1), and it resumed past h70 once honest weight returned — post-shed capture is weight-priced, not head-priced |
| `10c-ws-cold-sync` | ✅ pass | major |  | WS cold-sync under the latch: val-b restarted pinned to checkpoint 71:6103ee2cc3cf14be9395ded3bf63774e8383bc81af12b263eb5d2fa27b26055b, caught up to h71 (sync=1) and came back with the wheels STILL shed (latch_held=1 — a restart must never re-arm the anchors, F-1) |
| `11-economy-repair` | ✅ pass | major |  | the S7 repair economy CLOSED on the wire: killed 3 columns' holders → the caretaker RECONSTRUCTED from parity → a verified-repair bounty drew the object's reserve down (paid=190 credits over 6 repair(s)) — durability paid for itself on a real network, standing untouched (Invariant A). Post-kill cycle: store-2 last-sweep=20/29 stripes-repaired=2; relay last-sweep=20/29 stripes-repaired=2 |
| `11b-economy-skim` | ✅ pass | major |  | the SKIM leg closed on the wire: serve traffic (reconstruction reads + driven fetches) routed revenue into the object's durability reserve on the serving holder's ledger (funded 400000 → 400002 on store-2: +2 pure skim above the prepay baseline) — the object pays for its own repair (S7) |
| `11c-economy-horizon` | ✅ pass | info |  | g-instrumentation sample (S7 finite-but-renewable): paid=190 over 6 repair(s), reserve-after=8, horizonSec=11303053 (−1 = no burn window yet). One row per graded run — the g trend needs the series, not this sample |
| `12-deep-heights` | ✅ pass | major |  | DEEP drive (Phase 3 exit gate): honest ceiling reached h129 (target h128, from h74) within 2172s of the 7200s wall (~39s/height measured) |
| `12b-deep-prune` | ✅ pass | major |  | retention prune ENGAGED on every validator at depth (horizon ≈ h64 = epoch-floored h_end−2·TTL): val-a=52pruned/85MiB val-b=57pruned/80MiB val-c=57pruned/80MiB val-d=57pruned/81MiB — payload-stripped counts read from persisted chain.cbor via chain-status, on-disk bytes carried as the weight evidence |
| `12c-deep-converge` | ✅ pass | major |  | convergence at depth on the pruned chain: all validators within 2 of tip=h130 and tip-height validators share head hash cbdd84ce4297… (val-a=h130:cbdd84ce4297 val-b=h130:cbdd84ce4297 val-c=h130:cbdd84ce4297 val-d=h131:a1e128967bed) |
| `13-delivery-lane` | ✅ pass | major | 101s | DARK lane (era-4 not active): armed + announced on val-a; client refused at the withdrawal naming the committed E->key binding (nothing spent); server debug.log (+40 lines since baseline) banked nothing; lane-off control at store-2 refused-with-marker; delivery settlement: p=1 credit per 262144 B (U/p=262144 B/credit; self-mint Dλ=393216 B/credit, PF 1.50); anchor face 50000 funds 50000 increments = 13107200000 B (12.21 GiB) per session, k_max=1; one grant = 10 faces, the 64 GiB pin needs 9 faces across delivery+relay; idle window 1m30s; unsettled remainder: a DEPOSIT returned to the fetcher's account when its anchor leaves the 5-epoch guard window (D-R2.9-NODE-HALF-CALLS 1′; the relay lane keeps the burn) |
| `13b-delivery-settlement` | ➖ skip | major | 101s | UNTESTED on this chain: the client was refused at the withdrawal because the issuer served no key that resolves against a COMMITTED E->key binding — on today's networks because era-4 is dark (no binding can commit until the R3.4 stamp raise; no activation override, owner-ratified), though the same sentence would also cover off-commitment keys (no era surface exists to tell them apart: R-CLOUD-ERA-PROBE). Row 13 is what holds today; this row grades the wire settlement once the binding commits, with no harness change. |
| `184-equivocation` | ➖ skip | blocker |  | runs on the contained equivocation ISLAND every sheet (flow_equivocation_island — a separate consensus universe; its slash never taxes main-sheet fault tolerance, PE 2026-08-17). This row is the historical pointer; the island row is the graded verdict. |
| `184-equivocation-island` | ✅ pass | blocker |  | accountability FIRED on the wire: a contained island anchor double-signed and an honest anchor SLASHED it (slashed equivocator 6c5f111568172664ae5c47077f59620f93cf8a352e0ab8d1877c730972f3b701 (double-signed at height 1)) — proven equivocation → permanent eviction (F2), zero blast radius to the main sheet (separate consensus universe) |
| `184-forged-block` | ✅ pass | major |  | forged-signature proposal rejected (adversary logged 'correctly REJECTED by val-a') |
| `184-low-bond` | ⚠️ gap | major |  | adversary holds a qualifying 64M bond and was CORRECTLY accepted as a proposer — an under-bond REJECTION test needs a dedicated sub-min-bond identity (#350); the property is certified in-process (#204) |
| `184-partition` | ✅ pass | major |  | minority val-c STALLED at h46 through the partition (a < ⅓ island cannot commit) then CAUGHT UP to the heal-time majority head h50 (now at h50) on heal — BFT partition→heal reconverged over the real wire (a catch-up, NOT a reorg — a minority never committed a conflicting fork) |
| `2-publish-fetch` | ✅ pass | blocker | 72s | fetched from store-2 bit-perfect |
| `3-care-link` | ✅ pass | minor |  | publish exposes a siltcare: link (repair/audit without the key) |
| `4-become-validator` | ✅ pass | major |  | non-anchor validators earn their OWN standing on the objective path |
| `5-convergence` | ✅ pass | major |  | all validators within 2 of tip=26 AND every tip-height validator shares head hash bbebc0e04811… (heights: val-a=26:bbebc0e04811 val-b=26:bbebc0e04811 val-c=26:bbebc0e04811 val-d=26:bbebc0e04811); DURABLE (val-a head 26->26 over 20s, no regression) |
| `5-sybil-no-capture` | ➖ skip | major |  | MATURING=1 topology sheds the anchors by design — the anchor-gate premise doesn't exist here; the post-shed capture property is certified by 10-maturing-handoff's B2 drills (run without MATURING for flow 5) |
| `6-fault-tolerance` | ⚠️ gap | major |  | no new commit within the computed 380s (≤ f+1 rounds, f=1) hard cap with val-d down (fingerprint rc=130 h=0 → rc=198 h=40: ladder advancing but uncommitted — OUT OF MODEL) — read the captured client error (publish-diag / .ft_publish_lasterr) and survivor journals before attributing (#509/#7) |
| `7-restart-content` | ✅ pass | major |  | content still fetchable BIT-PERFECT after a storage-node restart |
| `7-restart-standing` | ✅ pass | major | 30s | val-b standing returned after restart without re-bonding |
| `8-takedown` | ✅ pass | major |  | store-1 enforces the operator denylist ([1953]: denylist: honoring 1 denied root(s)) while store-2 still serves BIT-PERFECT (no global switch) |
| `9-cross-nat` | ✅ pass | major |  | natted nodes exchanged a file through the relay/hole-punch |
| `chaos-fetch` | ✅ pass | major |  | content fetchable BIT-PERFECT after a hard-crash (SIGKILL) + restart of a storage node |
| `chaos-reprovide` | ✅ pass | major |  | SIGKILLed storage node re-announced its held chunks (#69) after a hard crash (54s to re-announce; latency scales with held-chunk count, #402/M1) |
| `durability-turnover` | ✅ pass | major |  | content survived a PERMANENT storage-node departure — fetched bit-perfect from a survivor |
| `infra-node-liveness` | ✅ pass | blocker |  | node-liveness precondition HELD — no OOM-kill or crash-loop across the cohort, so the sheet was graded on a HEALTHY network |
| `infra-node-memory` | ✅ pass | info |  | RSS envelope measured (cgroup MemoryCurrent, every 30s → rss-2633a11-deep.jsonl): worst peak 1.48GiB across the cohort. adversary peak=1.44GiB final=1.20GiB n=40; fetch-1 peak=0.02GiB final=0.01GiB n=42; island-a peak=0.34GiB final=0.29GiB n=42; island-b peak=0.33GiB final=0.29GiB n=42; island-c peak=0.32GiB final=0.29GiB n=42; island-d peak=0.32GiB final=0.28GiB n=42; maturer-1 peak=1.42GiB final=1.41GiB n=39; maturer-2 peak=1.48GiB final=1.36GiB n=41; maturer-3 peak=1.42GiB final=1.33GiB n=41; maturer-4 peak=1.31GiB final=1.22GiB n=41; nat-1 peak=0.02GiB final=0.02GiB n=42; nat-2 peak=0.02GiB final=0.02GiB n=42; registry peak=0.00GiB final=0.00GiB n=42; relay peak=0.02GiB final=0.02GiB n=42; store-1 peak=0.01GiB final=0.01GiB n=42; store-2 peak=0.03GiB final=0.01GiB n=41; store-3 peak=0.02GiB final=0.02GiB n=41; store-4 peak=0.02GiB final=0.02GiB n=41; sybil-1 peak=1.30GiB final=0.88GiB n=39; sybil-2 peak=1.29GiB final=0.95GiB n=39; sybil-3 peak=1.32GiB final=1.22GiB n=39; sybil-4 peak=1.41GiB final=0.91GiB n=38; val-a peak=1.43GiB final=1.28GiB n=40; val-b peak=1.34GiB final=1.33GiB n=40; val-c peak=1.40GiB final=1.17GiB n=40; val-d peak=1.38GiB final=1.19GiB n=36 |
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

### 6-fault-tolerance — ⚠️ gap (major)
no new commit within the computed 380s (≤ f+1 rounds, f=1) hard cap with val-d down (fingerprint rc=130 h=0 → rc=198 h=40: ladder advancing but uncommitted — OUT OF MODEL) — read the captured client error (publish-diag / .ft_publish_lasterr) and survivor journals before attributing (#509/#7)

---

_Generated by `integration/cloudtest`. The field network is ephemeral and was torn down after this run._