# silt field-test report

- **run:** `a56ac10-42834`  ·  **silt commit:** `a56ac10`  ·  **bond mode:** `fast`  ·  **generated:** 2026-08-16T03:44:36Z
- **result:** **FAIL**  ·  18 pass / 3 gap / 2 fail / 1 skip

## Per-flow verdict

| flow | verdict | severity | elapsed | detail |
|------|---------|----------|---------|--------|
| `1-first-run` | ✅ pass | blocker |  | all silt nodes report service active |
| `10-maturing-handoff` | ✅ pass | major |  | young→mature HANDOFF field-exercised: latch tripped on the wire, commits crossed the epoch boundary into the governed mature snapshot (h52→57, target h57), and no anchor-required refusal after the shed |
| `10a-stall-drill` | ❌ fail | major |  | B2 stall drill: with the 4 cheap epoch members DECLINING to attest, the honest >⅔-weight coalition still commits on the wire (head-counted quorum left this exact network born-unable-to-commit at 4×MinBond) |
| `10b-capture-drill` | ✅ pass | major |  | B2 capture drill: the 4 MinBond epoch members alone could NOT advance the mature chain past the honest ceiling h58 (cohort head →57, fresh cohort commit: 1), and it resumed past h58 once honest weight returned — post-shed capture is weight-priced, not head-priced |
| `10c-ws-cold-sync` | ✅ pass | major |  | WS cold-sync under the latch: val-b restarted pinned to checkpoint 58:606feecd006267250bcfa99df04b8a7fe080a4dac4cb3d46d0ae0218d2453092, caught up to h59 (sync=1) and came back with the wheels STILL shed (latch_held=1 — a restart must never re-arm the anchors, F-1) |
| `184-equivocation` | ⚠️ gap | blocker |  | adversary did not reach qualified-proposer standing over WAN within 180s (its bond never committed on-chain), so the double-sign could not be placed — equivocation UNTESTED this run, not a product failure (slashing is certified in-process #204). See #345/#350. |
| `184-forged-block` | ✅ pass | major |  | forged-signature proposal rejected (adversary logged 'correctly REJECTED by val-a') |
| `184-low-bond` | ✅ pass | major |  | under-bonded proposer rejected (adversary logged 'correctly REJECTED by val-a') |
| `184-partition` | ⚠️ gap | major |  | no reorg line on val-c within 120s — a heavier fork may not have formed during the partition (idle chain), so the heal was UNTESTED this run, not a failure (certified in-process #204, #350) |
| `2-publish-fetch` | ✅ pass | blocker | 175s | fetched from store-2 bit-perfect |
| `3-care-link` | ✅ pass | minor |  | publish exposes a siltcare: link (repair/audit without the key) |
| `4-become-validator` | ✅ pass | major |  | non-anchor validators earn their OWN standing on the objective path |
| `5-convergence` | ✅ pass | major |  | all validators within 2 of tip=13 AND every tip-height validator shares head hash f6484085e01d… (heights: val-a=12:bed8e3542d6b val-b=13:f6484085e01d val-c=13:f6484085e01d val-d=13:f6484085e01d); DURABLE (val-a head 12->13 over 20s, no regression) |
| `5-sybil-no-capture` | ➖ skip | major |  | MATURING=1 topology sheds the anchors by design — the anchor-gate premise doesn't exist here; the post-shed capture property is certified by 10-maturing-handoff's B2 drills (run without MATURING for flow 5) |
| `6-fault-tolerance` | ✅ pass | major |  | publish still committed with one validator (val-d) down |
| `7-restart-content` | ✅ pass | major |  | content still fetchable BIT-PERFECT after a storage-node restart |
| `7-restart-standing` | ✅ pass | major | 7s | val-b standing returned after restart without re-bonding |
| `8-takedown` | ✅ pass | major |  | store-1 enforces the operator denylist ([1310]: denylist: 1 root(s) denied; purged 2 held chunk(s)) while store-2 still serves BIT-PERFECT (no global switch) |
| `9-cross-nat` | ✅ pass | major |  | natted nodes exchanged a file through the relay/hole-punch |
| `chaos-fetch` | ❌ fail | major |  | content fetchable BIT-PERFECT after a hard-crash (SIGKILL) + restart of a storage node (want=e54d0dc3655675a36a43b80d622192989afc26483503ed9d96e2e5b57e209366 got=<none>) |
| `chaos-reprovide` | ✅ pass | major |  | SIGKILLed storage node re-announced its held chunks (#69) after a hard crash (217s to re-announce; latency scales with held-chunk count, #402/M1) |
| `durability-turnover` | ⚠️ gap | major |  | setup publish did not land a link — durability UNTESTED this run (ephemeral-CLI issuer-set discovery over WAN, #351), not a durability failure |
| `priv-unlinkability` | ✅ pass | major |  | default chain REFUSED a durable file→publisher link (refuse-to-surveil) |
| `web-ui-guard` | ✅ pass | major |  | web-UI guard held on a real VM: no-token POST=401 (want 401), DNS-rebinding Host=403 (want 403), token-free read=200 (want 200) |

## Findings (gaps + failures), most severe first

### 184-equivocation — ⚠️ gap (blocker)
adversary did not reach qualified-proposer standing over WAN within 180s (its bond never committed on-chain), so the double-sign could not be placed — equivocation UNTESTED this run, not a product failure (slashing is certified in-process #204). See #345/#350.

### 10a-stall-drill — ❌ fail (major)
B2 stall drill: with the 4 cheap epoch members DECLINING to attest, the honest >⅔-weight coalition still commits on the wire (head-counted quorum left this exact network born-unable-to-commit at 4×MinBond)

### 184-partition — ⚠️ gap (major)
no reorg line on val-c within 120s — a heavier fork may not have formed during the partition (idle chain), so the heal was UNTESTED this run, not a failure (certified in-process #204, #350)

### 5-sybil-no-capture — ➖ skip (major)
MATURING=1 topology sheds the anchors by design — the anchor-gate premise doesn't exist here; the post-shed capture property is certified by 10-maturing-handoff's B2 drills (run without MATURING for flow 5)

### chaos-fetch — ❌ fail (major)
content fetchable BIT-PERFECT after a hard-crash (SIGKILL) + restart of a storage node (want=e54d0dc3655675a36a43b80d622192989afc26483503ed9d96e2e5b57e209366 got=<none>)

### durability-turnover — ⚠️ gap (major)
setup publish did not land a link — durability UNTESTED this run (ephemeral-CLI issuer-set discovery over WAN, #351), not a durability failure

---

_Generated by `integration/cloudtest`. The field network is ephemeral and was torn down after this run._