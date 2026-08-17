# silt field-test report

- **run:** `6fbcf2e-18553`  ·  **silt commit:** `6fbcf2e`  ·  **bond mode:** `fast`  ·  **generated:** 2026-08-16T23:15:15Z
- **result:** **REVIEW**  ·  18 pass / 3 gap / 0 fail / 1 skip

## Per-flow verdict

| flow | verdict | severity | elapsed | detail |
|------|---------|----------|---------|--------|
| `1-first-run` | ✅ pass | blocker |  | all silt nodes report service active |
| `10-maturing-handoff` | ➖ skip | major |  | not a MATURING topology — opt in with MATURING=1 SYBILS=8 ./cloudtest.sh to field-exercise the handoff/post-shed regime (the external red team's sharpest seam-#8 target; until then it is proven only in-process: core/chain/quorum_weight_test.go + sim/maturequorum_test.go) |
| `184-equivocation` | ⚠️ gap | blocker |  | adversary did not reach qualified-proposer standing over WAN within 180s (its bond never committed on-chain), so the double-sign could not be placed — equivocation UNTESTED this run, not a product failure (slashing is certified in-process #204). See #345/#350. |
| `184-forged-block` | ✅ pass | major |  | forged-signature proposal rejected (adversary logged 'correctly REJECTED by val-a') |
| `184-low-bond` | ✅ pass | major |  | under-bonded proposer rejected (adversary logged 'correctly REJECTED by val-a') |
| `184-partition` | ⚠️ gap | major |  | no reorg line on val-c within 120s — a heavier fork may not have formed during the partition (idle chain), so the heal was UNTESTED this run, not a failure (certified in-process #204, #350) |
| `2-publish-fetch` | ✅ pass | blocker | 95s | fetched from store-2 bit-perfect |
| `3-care-link` | ✅ pass | minor |  | publish exposes a siltcare: link (repair/audit without the key) |
| `4-become-validator` | ✅ pass | major |  | non-anchor validators earn their OWN standing on the objective path |
| `5-convergence` | ✅ pass | major |  | all validators within 2 of tip=14 AND every tip-height validator shares head hash 903cd1fe22e6… (heights: val-a=14:903cd1fe22e6 val-b=14:903cd1fe22e6 val-c=14:903cd1fe22e6 val-d=14:903cd1fe22e6); DURABLE (val-a head 14->15 over 20s, no regression) |
| `5-sybil-no-capture` | ⚠️ gap | major |  | PRE-EXISTING FORK (#402): a Sybil head (h45) is ABOVE the anchored ceiling (h44) BEFORE any anchor was stopped — the Sybils are on a divergent fork (a launch anchor-gate fork, one free anchor co-signing; see #402), not synced to the anchor chain. The no-capture PREMISE is unmet, so this run cannot grade capture; the fork itself is the finding. Journals captured at this verdict. |
| `6-fault-tolerance` | ✅ pass | major |  | publish still committed with one validator (val-d) down (within the computed 260s down-designee escape bound) |
| `7-restart-content` | ✅ pass | major |  | content still fetchable BIT-PERFECT after a storage-node restart |
| `7-restart-standing` | ✅ pass | major | 8s | val-b standing returned after restart without re-bonding |
| `8-takedown` | ✅ pass | major |  | store-1 enforces the operator denylist ([1362]: denylist: 1 root(s) denied; purged 6 held chunk(s)) while store-2 still serves BIT-PERFECT (no global switch) |
| `9-cross-nat` | ✅ pass | major |  | natted nodes exchanged a file through the relay/hole-punch |
| `chaos-fetch` | ✅ pass | major |  | content fetchable BIT-PERFECT after a hard-crash (SIGKILL) + restart of a storage node |
| `chaos-reprovide` | ✅ pass | major |  | SIGKILLed storage node re-announced its held chunks (#69) after a hard crash (145s to re-announce; latency scales with held-chunk count, #402/M1) |
| `durability-turnover` | ✅ pass | major |  | content survived a PERMANENT storage-node departure — fetched bit-perfect from a survivor |
| `priv-unlinkability` | ✅ pass | major |  | default chain REFUSED a durable file→publisher link (refuse-to-surveil) |
| `soak-publish-drain` | ✅ pass | major |  | launch-regime publish/drain SOAK: 13 heights committed under continuously interleaved publish (10/10 landed) + natural renewal drain, max inter-commit gap 209s ≤ the computed 220s escape bound, 0 honest-slash lines (want 0) — the #432 escape clears the production-reachable race the PE gate names |
| `web-ui-guard` | ✅ pass | major |  | web-UI guard held on a real VM: no-token POST=401 (want 401), DNS-rebinding Host=403 (want 403), token-free read=200 (want 200) |

## Findings (gaps + failures), most severe first

### 184-equivocation — ⚠️ gap (blocker)
adversary did not reach qualified-proposer standing over WAN within 180s (its bond never committed on-chain), so the double-sign could not be placed — equivocation UNTESTED this run, not a product failure (slashing is certified in-process #204). See #345/#350.

### 10-maturing-handoff — ➖ skip (major)
not a MATURING topology — opt in with MATURING=1 SYBILS=8 ./cloudtest.sh to field-exercise the handoff/post-shed regime (the external red team's sharpest seam-#8 target; until then it is proven only in-process: core/chain/quorum_weight_test.go + sim/maturequorum_test.go)

### 184-partition — ⚠️ gap (major)
no reorg line on val-c within 120s — a heavier fork may not have formed during the partition (idle chain), so the heal was UNTESTED this run, not a failure (certified in-process #204, #350)

### 5-sybil-no-capture — ⚠️ gap (major)
PRE-EXISTING FORK (#402): a Sybil head (h45) is ABOVE the anchored ceiling (h44) BEFORE any anchor was stopped — the Sybils are on a divergent fork (a launch anchor-gate fork, one free anchor co-signing; see #402), not synced to the anchor chain. The no-capture PREMISE is unmet, so this run cannot grade capture; the fork itself is the finding. Journals captured at this verdict.

---

_Generated by `integration/cloudtest`. The field network is ephemeral and was torn down after this run._