# silt field-test report

- **run:** `9453325-7258`  ·  **silt commit:** `9453325`  ·  **bond mode:** `fast`  ·  **generated:** 2026-08-16T05:08:32Z
- **result:** **FAIL**  ·  18 pass / 2 gap / 1 fail / 1 skip

## Per-flow verdict

| flow | verdict | severity | elapsed | detail |
|------|---------|----------|---------|--------|
| `1-first-run` | ✅ pass | blocker |  | all silt nodes report service active |
| `10-maturing-handoff` | ➖ skip | major |  | not a MATURING topology — opt in with MATURING=1 SYBILS=8 ./cloudtest.sh to field-exercise the handoff/post-shed regime (the external red team's sharpest seam-#8 target; until then it is proven only in-process: core/chain/quorum_weight_test.go + sim/maturequorum_test.go) |
| `184-equivocation` | ⚠️ gap | blocker |  | adversary did not reach qualified-proposer standing over WAN within 180s (its bond never committed on-chain), so the double-sign could not be placed — equivocation UNTESTED this run, not a product failure (slashing is certified in-process #204). See #345/#350. |
| `184-forged-block` | ✅ pass | major |  | forged-signature proposal rejected (adversary logged 'correctly REJECTED by val-a') |
| `184-low-bond` | ✅ pass | major |  | under-bonded proposer rejected (adversary logged 'correctly REJECTED by val-a') |
| `184-partition` | ⚠️ gap | major |  | no reorg line on val-c within 120s — a heavier fork may not have formed during the partition (idle chain), so the heal was UNTESTED this run, not a failure (certified in-process #204, #350) |
| `2-publish-fetch` | ✅ pass | blocker | 97s | fetched from store-2 bit-perfect |
| `3-care-link` | ✅ pass | minor |  | publish exposes a siltcare: link (repair/audit without the key) |
| `4-become-validator` | ✅ pass | major |  | non-anchor validators earn their OWN standing on the objective path |
| `5-convergence` | ✅ pass | major |  | all validators within 2 of tip=20 AND every tip-height validator shares head hash f19c3c70b8dd… (heights: val-a=20:f19c3c70b8dd val-b=19:b955f5d3b550 val-c=20:f19c3c70b8dd val-d=20:f19c3c70b8dd); DURABLE (val-a head 20->20 over 20s, no regression) |
| `5-sybil-no-capture` | ✅ pass | major |  | no quiet capture: 8 bonded single-domain Sybils could NOT advance the chain past the anchored ceiling h48 with all anchors down (sybil head →48, no fresh Sybil commit), and a DRIVEN block committed + synced to the Sybil (48→49) once the anchors returned; a Sybil logged the anchor-required refusal |
| `6-fault-tolerance` | ✅ pass | major |  | publish still committed with one validator (val-d) down |
| `7-restart-content` | ✅ pass | major |  | content still fetchable BIT-PERFECT after a storage-node restart |
| `7-restart-standing` | ✅ pass | major | 7s | val-b standing returned after restart without re-bonding |
| `8-takedown` | ✅ pass | major |  | store-1 enforces the operator denylist ([1419]: denylist: 1 root(s) denied; purged 3 held chunk(s)) while store-2 still serves BIT-PERFECT (no global switch) |
| `9-cross-nat` | ✅ pass | major |  | natted nodes exchanged a file through the relay/hole-punch |
| `chaos-fetch` | ✅ pass | major |  | content fetchable BIT-PERFECT after a hard-crash (SIGKILL) + restart of a storage node |
| `chaos-reprovide` | ✅ pass | major |  | SIGKILLed storage node re-announced its held chunks (#69) after a hard crash (250s to re-announce; latency scales with held-chunk count, #402/M1) |
| `durability-turnover` | ✅ pass | major |  | content survived a PERMANENT storage-node departure — fetched bit-perfect from a survivor |
| `priv-unlinkability` | ✅ pass | major |  | default chain REFUSED a durable file→publisher link (refuse-to-surveil) |
| `soak-publish-drain` | ❌ fail | major |  | WEDGE SIGNATURE under the publish/drain soak: a height went 361s (> the computed 160s escape bound) without a commit with the network live (h51→h60, 2/15 publishes landed) — the #432 escape did not clear the interleaved race; last client output: silt: httpregistry publish: propose height 60 round 0: already signed a different block in this slot (never-sign-twice, #397/#432) |
| `web-ui-guard` | ✅ pass | major |  | web-UI guard held on a real VM: no-token POST=401 (want 401), DNS-rebinding Host=403 (want 403), token-free read=200 (want 200) |

## Findings (gaps + failures), most severe first

### 184-equivocation — ⚠️ gap (blocker)
adversary did not reach qualified-proposer standing over WAN within 180s (its bond never committed on-chain), so the double-sign could not be placed — equivocation UNTESTED this run, not a product failure (slashing is certified in-process #204). See #345/#350.

### 10-maturing-handoff — ➖ skip (major)
not a MATURING topology — opt in with MATURING=1 SYBILS=8 ./cloudtest.sh to field-exercise the handoff/post-shed regime (the external red team's sharpest seam-#8 target; until then it is proven only in-process: core/chain/quorum_weight_test.go + sim/maturequorum_test.go)

### 184-partition — ⚠️ gap (major)
no reorg line on val-c within 120s — a heavier fork may not have formed during the partition (idle chain), so the heal was UNTESTED this run, not a failure (certified in-process #204, #350)

### soak-publish-drain — ❌ fail (major)
WEDGE SIGNATURE under the publish/drain soak: a height went 361s (> the computed 160s escape bound) without a commit with the network live (h51→h60, 2/15 publishes landed) — the #432 escape did not clear the interleaved race; last client output: silt: httpregistry publish: propose height 60 round 0: already signed a different block in this slot (never-sign-twice, #397/#432)

---

_Generated by `integration/cloudtest`. The field network is ephemeral and was torn down after this run._