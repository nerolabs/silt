# silt field-test report

- **run:** `54003f7-91159`  ·  **silt commit:** `54003f7`  ·  **bond mode:** `fast`  ·  **generated:** 2026-08-16T11:46:35Z
- **result:** **FAIL**  ·  16 pass / 3 gap / 1 fail / 1 skip

## Per-flow verdict

| flow | verdict | severity | elapsed | detail |
|------|---------|----------|---------|--------|
| `1-first-run` | ✅ pass | blocker |  | all silt nodes report service active |
| `10-maturing-handoff` | ❌ fail | major |  | the everMature latch did not trip within the COMPUTED 630s bound with the full maturer cohort live (4 maturers; bound = 9 reg-blocks × 64s worst-case + submit leg) — a real drain/maturity FINDING (PE cadence ruling §4), not a window artifact; read the drain curve in the evidence journals |
| `184-equivocation` | ⚠️ gap | blocker |  | adversary did not reach qualified-proposer standing over WAN within 180s (its bond never committed on-chain), so the double-sign could not be placed — equivocation UNTESTED this run, not a product failure (slashing is certified in-process #204). See #345/#350. |
| `184-forged-block` | ✅ pass | major |  | forged-signature proposal rejected (adversary logged 'correctly REJECTED by val-a') |
| `184-low-bond` | ✅ pass | major |  | under-bonded proposer rejected (adversary logged 'correctly REJECTED by val-a') |
| `184-partition` | ⚠️ gap | major |  | no reorg line on val-c within 120s — a heavier fork may not have formed during the partition (idle chain), so the heal was UNTESTED this run, not a failure (certified in-process #204, #350) |
| `2-publish-fetch` | ✅ pass | blocker | 78s | fetched from store-2 bit-perfect |
| `3-care-link` | ✅ pass | minor |  | publish exposes a siltcare: link (repair/audit without the key) |
| `4-become-validator` | ✅ pass | major |  | non-anchor validators earn their OWN standing on the objective path |
| `5-convergence` | ✅ pass | major |  | all validators within 2 of tip=12 AND every tip-height validator shares head hash e59fe4008b4b… (heights: val-a=11:34c4d7a7c711 val-b=12:e59fe4008b4b val-c=12:e59fe4008b4b val-d=12:e59fe4008b4b); DURABLE (val-a head 11->13 over 20s, no regression) |
| `5-sybil-no-capture` | ➖ skip | major |  | MATURING=1 topology sheds the anchors by design — the anchor-gate premise doesn't exist here; the post-shed capture property is certified by 10-maturing-handoff's B2 drills (run without MATURING for flow 5) |
| `6-fault-tolerance` | ⚠️ gap | major |  | surviving validators did not commit with val-d down — likely quorum/byzantine-quorum sizing; pin -quorum for the validator count (see README shakedown notes) |
| `7-restart-content` | ✅ pass | major |  | content still fetchable BIT-PERFECT after a storage-node restart |
| `7-restart-standing` | ✅ pass | major | 18s | val-b standing returned after restart without re-bonding |
| `8-takedown` | ✅ pass | major |  | store-1 enforces the operator denylist ([1441]: denylist: honoring 1 denied root(s)) while store-2 still serves BIT-PERFECT (no global switch) |
| `9-cross-nat` | ✅ pass | major |  | natted nodes exchanged a file through the relay/hole-punch |
| `chaos-fetch` | ✅ pass | major |  | content fetchable BIT-PERFECT after a hard-crash (SIGKILL) + restart of a storage node |
| `chaos-reprovide` | ✅ pass | major |  | SIGKILLed storage node re-announced its held chunks (#69) after a hard crash (68s to re-announce; latency scales with held-chunk count, #402/M1) |
| `durability-turnover` | ✅ pass | major |  | content survived a PERMANENT storage-node departure — fetched bit-perfect from a survivor |
| `priv-unlinkability` | ✅ pass | major |  | default chain REFUSED a durable file→publisher link (refuse-to-surveil) |
| `web-ui-guard` | ✅ pass | major |  | web-UI guard held on a real VM: no-token POST=401 (want 401), DNS-rebinding Host=403 (want 403), token-free read=200 (want 200) |

## Findings (gaps + failures), most severe first

### 184-equivocation — ⚠️ gap (blocker)
adversary did not reach qualified-proposer standing over WAN within 180s (its bond never committed on-chain), so the double-sign could not be placed — equivocation UNTESTED this run, not a product failure (slashing is certified in-process #204). See #345/#350.

### 10-maturing-handoff — ❌ fail (major)
the everMature latch did not trip within the COMPUTED 630s bound with the full maturer cohort live (4 maturers; bound = 9 reg-blocks × 64s worst-case + submit leg) — a real drain/maturity FINDING (PE cadence ruling §4), not a window artifact; read the drain curve in the evidence journals

### 184-partition — ⚠️ gap (major)
no reorg line on val-c within 120s — a heavier fork may not have formed during the partition (idle chain), so the heal was UNTESTED this run, not a failure (certified in-process #204, #350)

### 5-sybil-no-capture — ➖ skip (major)
MATURING=1 topology sheds the anchors by design — the anchor-gate premise doesn't exist here; the post-shed capture property is certified by 10-maturing-handoff's B2 drills (run without MATURING for flow 5)

### 6-fault-tolerance — ⚠️ gap (major)
surviving validators did not commit with val-d down — likely quorum/byzantine-quorum sizing; pin -quorum for the validator count (see README shakedown notes)

---

_Generated by `integration/cloudtest`. The field network is ephemeral and was torn down after this run._