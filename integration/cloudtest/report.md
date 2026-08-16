# silt field-test report

- **run:** `82bcd2b-39478`  ·  **silt commit:** `82bcd2b`  ·  **bond mode:** `fast`  ·  **generated:** 2026-08-16T21:37:35Z
- **result:** **REVIEW**  ·  20 pass / 3 gap / 0 fail / 1 skip

## Per-flow verdict

| flow | verdict | severity | elapsed | detail |
|------|---------|----------|---------|--------|
| `1-first-run` | ✅ pass | blocker |  | all silt nodes report service active |
| `10-maturing-handoff` | ✅ pass | major |  | young→mature HANDOFF: latch tripped on the wire; drive reached h59 (target h57) within 1980s |
| `10a-stall-drill` | ✅ pass | major |  | B2 stall drill: with the 4 cheap epoch members DECLINING to attest, the honest >⅔-weight coalition still commits on the wire within the computed 430s bound (head-counted quorum left this exact network born-unable-to-commit at 4×MinBond) |
| `10b-capture-drill` | ✅ pass | major |  | B2 capture drill: the 4 MinBond epoch members alone could NOT advance the mature chain past the honest ceiling h62 (cohort head →63, fresh cohort commit: 1), and it resumed past h62 once honest weight returned — post-shed capture is weight-priced, not head-priced |
| `10c-ws-cold-sync` | ✅ pass | major |  | WS cold-sync under the latch: val-b restarted pinned to checkpoint 60:30b738ff5f09e151bcdc814b731644029dcb228adce72c51f761a0032d37a831, caught up to h63 (sync=1) and came back with the wheels STILL shed (latch_held=1 — a restart must never re-arm the anchors, F-1) |
| `184-equivocation` | ⚠️ gap | blocker |  | adversary could not PLACE the double-sign on the live chain within 120s (honest validators had already attested at that height — 'already attested a different block at height'), so equivocation was UNTESTED this run, not a failure (certified in-process #204, #345/#350) |
| `184-forged-block` | ✅ pass | major |  | forged-signature proposal rejected (adversary logged 'correctly REJECTED by val-a') |
| `184-low-bond` | ✅ pass | major |  | under-bonded proposer rejected (adversary logged 'correctly REJECTED by val-a') |
| `184-partition` | ⚠️ gap | major |  | no reorg line on val-c within 120s — a heavier fork may not have formed during the partition (idle chain), so the heal was UNTESTED this run, not a failure (certified in-process #204, #350) |
| `2-publish-fetch` | ✅ pass | blocker | 92s | fetched from store-2 bit-perfect |
| `3-care-link` | ✅ pass | minor |  | publish exposes a siltcare: link (repair/audit without the key) |
| `4-become-validator` | ✅ pass | major |  | non-anchor validators earn their OWN standing on the objective path |
| `5-convergence` | ✅ pass | major |  | all validators within 2 of tip=14 AND every tip-height validator shares head hash 95e1720787cd… (heights: val-a=14:95e1720787cd val-b=13:c985fc732937 val-c=13:c985fc732937 val-d=14:95e1720787cd); DURABLE (val-a head 14->14 over 20s, no regression) |
| `5-sybil-no-capture` | ➖ skip | major |  | MATURING=1 topology sheds the anchors by design — the anchor-gate premise doesn't exist here; the post-shed capture property is certified by 10-maturing-handoff's B2 drills (run without MATURING for flow 5) |
| `6-fault-tolerance` | ✅ pass | major |  | publish still committed with one validator (val-d) down (within the computed 260s down-designee escape bound) |
| `7-restart-content` | ✅ pass | major |  | content still fetchable BIT-PERFECT after a storage-node restart |
| `7-restart-standing` | ✅ pass | major | 7s | val-b standing returned after restart without re-bonding |
| `8-takedown` | ✅ pass | major |  | store-1 enforces the operator denylist ([1391]: denylist: 1 root(s) denied; purged 10 held chunk(s)) while store-2 still serves BIT-PERFECT (no global switch) |
| `9-cross-nat` | ✅ pass | major |  | natted nodes exchanged a file through the relay/hole-punch |
| `chaos-fetch` | ✅ pass | major |  | content fetchable BIT-PERFECT after a hard-crash (SIGKILL) + restart of a storage node |
| `chaos-reprovide` | ✅ pass | major |  | SIGKILLed storage node re-announced its held chunks (#69) after a hard crash (285s to re-announce; latency scales with held-chunk count, #402/M1) |
| `durability-turnover` | ⚠️ gap | major |  | setup publish did not land a link — durability UNTESTED this run, not a durability failure (publish subsystem degraded: see the captured client error in publish-diag / .ft_publish_lasterr — discovery #351 or mature-regime quorum starvation #441; never presume which) |
| `priv-unlinkability` | ✅ pass | major |  | default chain REFUSED a durable file→publisher link (refuse-to-surveil) |
| `web-ui-guard` | ✅ pass | major |  | web-UI guard held on a real VM: no-token POST=401 (want 401), DNS-rebinding Host=403 (want 403), token-free read=200 (want 200) |

## Findings (gaps + failures), most severe first

### 184-equivocation — ⚠️ gap (blocker)
adversary could not PLACE the double-sign on the live chain within 120s (honest validators had already attested at that height — 'already attested a different block at height'), so equivocation was UNTESTED this run, not a failure (certified in-process #204, #345/#350)

### 184-partition — ⚠️ gap (major)
no reorg line on val-c within 120s — a heavier fork may not have formed during the partition (idle chain), so the heal was UNTESTED this run, not a failure (certified in-process #204, #350)

### 5-sybil-no-capture — ➖ skip (major)
MATURING=1 topology sheds the anchors by design — the anchor-gate premise doesn't exist here; the post-shed capture property is certified by 10-maturing-handoff's B2 drills (run without MATURING for flow 5)

### durability-turnover — ⚠️ gap (major)
setup publish did not land a link — durability UNTESTED this run, not a durability failure (publish subsystem degraded: see the captured client error in publish-diag / .ft_publish_lasterr — discovery #351 or mature-regime quorum starvation #441; never presume which)

---

_Generated by `integration/cloudtest`. The field network is ephemeral and was torn down after this run._