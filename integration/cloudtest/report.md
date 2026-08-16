# silt field-test report

- **run:** `ce15a80-89365`  ·  **silt commit:** `ce15a80`  ·  **bond mode:** `fast`  ·  **generated:** 2026-08-16T14:03:42Z
- **result:** **FAIL**  ·  19 pass / 3 gap / 1 fail / 1 skip

## Per-flow verdict

| flow | verdict | severity | elapsed | detail |
|------|---------|----------|---------|--------|
| `1-first-run` | ✅ pass | blocker |  | all silt nodes report service active |
| `10-maturing-handoff` | ✅ pass | major |  | young→mature HANDOFF field-exercised: latch tripped on the wire, commits crossed the epoch boundary into the governed mature snapshot (h45→49, target h49), and no anchor-required refusal after the shed |
| `10a-stall-drill` | ❌ fail | major |  | B2 stall drill: with the 4 cheap epoch members DECLINING to attest, the honest >⅔-weight coalition still commits on the wire within the computed 370s bound (head-counted quorum left this exact network born-unable-to-commit at 4×MinBond) |
| `10b-capture-drill` | ⚠️ gap | major |  | no-capture outcome held (head ≤ h50 with honest validators down) but the chain did not resume within 240s of their return (h2=49) — clincher inconclusive (SPOT preemption?); re-run to confirm |
| `10c-ws-cold-sync` | ✅ pass | major |  | WS cold-sync under the latch: val-b restarted pinned to checkpoint 54:7d0173bfa67bd79069ede1c6012cd1e7f7c864064b674006ba98cabc90b87776, caught up to h54 (sync=1) and came back with the wheels STILL shed (latch_held=1 — a restart must never re-arm the anchors, F-1) |
| `184-equivocation` | ⚠️ gap | blocker |  | adversary did not reach qualified-proposer standing over WAN within 180s (its bond never committed on-chain), so the double-sign could not be placed — equivocation UNTESTED this run, not a product failure (slashing is certified in-process #204). See #345/#350. |
| `184-forged-block` | ✅ pass | major |  | forged-signature proposal rejected (adversary logged 'correctly REJECTED by val-a') |
| `184-low-bond` | ✅ pass | major |  | under-bonded proposer rejected (adversary logged 'correctly REJECTED by val-a') |
| `184-partition` | ⚠️ gap | major |  | no reorg line on val-c within 120s — a heavier fork may not have formed during the partition (idle chain), so the heal was UNTESTED this run, not a failure (certified in-process #204, #350) |
| `2-publish-fetch` | ✅ pass | blocker | 93s | fetched from store-2 bit-perfect |
| `3-care-link` | ✅ pass | minor |  | publish exposes a siltcare: link (repair/audit without the key) |
| `4-become-validator` | ✅ pass | major |  | non-anchor validators earn their OWN standing on the objective path |
| `5-convergence` | ✅ pass | major |  | all validators within 2 of tip=16 AND every tip-height validator shares head hash 59869ff19edb… (heights: val-a=15:58b93d5b72d7 val-b=16:59869ff19edb val-c=16:59869ff19edb val-d=16:59869ff19edb); DURABLE (val-a head 15->17 over 20s, no regression) |
| `5-sybil-no-capture` | ➖ skip | major |  | MATURING=1 topology sheds the anchors by design — the anchor-gate premise doesn't exist here; the post-shed capture property is certified by 10-maturing-handoff's B2 drills (run without MATURING for flow 5) |
| `6-fault-tolerance` | ✅ pass | major |  | publish still committed with one validator (val-d) down (within the computed 200s down-designee escape bound) |
| `7-restart-content` | ✅ pass | major |  | content still fetchable BIT-PERFECT after a storage-node restart |
| `7-restart-standing` | ✅ pass | major | 7s | val-b standing returned after restart without re-bonding |
| `8-takedown` | ✅ pass | major |  | store-1 enforces the operator denylist ([1326]: denylist: 1 root(s) denied; purged 8 held chunk(s)) while store-2 still serves BIT-PERFECT (no global switch) |
| `9-cross-nat` | ✅ pass | major |  | natted nodes exchanged a file through the relay/hole-punch |
| `chaos-fetch` | ✅ pass | major |  | content fetchable BIT-PERFECT after a hard-crash (SIGKILL) + restart of a storage node |
| `chaos-reprovide` | ✅ pass | major |  | SIGKILLed storage node re-announced its held chunks (#69) after a hard crash (48s to re-announce; latency scales with held-chunk count, #402/M1) |
| `durability-turnover` | ✅ pass | major |  | content survived a PERMANENT storage-node departure — fetched bit-perfect from a survivor |
| `priv-unlinkability` | ✅ pass | major |  | default chain REFUSED a durable file→publisher link (refuse-to-surveil) |
| `web-ui-guard` | ✅ pass | major |  | web-UI guard held on a real VM: no-token POST=401 (want 401), DNS-rebinding Host=403 (want 403), token-free read=200 (want 200) |

## Findings (gaps + failures), most severe first

### 184-equivocation — ⚠️ gap (blocker)
adversary did not reach qualified-proposer standing over WAN within 180s (its bond never committed on-chain), so the double-sign could not be placed — equivocation UNTESTED this run, not a product failure (slashing is certified in-process #204). See #345/#350.

### 10a-stall-drill — ❌ fail (major)
B2 stall drill: with the 4 cheap epoch members DECLINING to attest, the honest >⅔-weight coalition still commits on the wire within the computed 370s bound (head-counted quorum left this exact network born-unable-to-commit at 4×MinBond)

### 10b-capture-drill — ⚠️ gap (major)
no-capture outcome held (head ≤ h50 with honest validators down) but the chain did not resume within 240s of their return (h2=49) — clincher inconclusive (SPOT preemption?); re-run to confirm

### 184-partition — ⚠️ gap (major)
no reorg line on val-c within 120s — a heavier fork may not have formed during the partition (idle chain), so the heal was UNTESTED this run, not a failure (certified in-process #204, #350)

### 5-sybil-no-capture — ➖ skip (major)
MATURING=1 topology sheds the anchors by design — the anchor-gate premise doesn't exist here; the post-shed capture property is certified by 10-maturing-handoff's B2 drills (run without MATURING for flow 5)

---

_Generated by `integration/cloudtest`. The field network is ephemeral and was torn down after this run._