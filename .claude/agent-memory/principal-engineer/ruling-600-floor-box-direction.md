---
name: ruling-600-floor-box-direction
description: The #600 floor-box direction after the coexistence FAIL — recommend witness-validation as the shipped floor; hold-tree is a bigger-box posture. The measured thrash is a build-phase artifact, not proof steady-state fails.
metadata:
  type: project
---

Ruling date 2026-08-28. Consult: after the coexistence run FAILED my prior ship-gate.

**Verdict:** Recommend witness-validation (tree-free) as the SHIPPED floor-box path.
Hold-tree survives ONLY as a bigger-box decentralization posture behind `ports.NodeStore`,
never as the 2 GB-floor default. Andrew ratifies (immutable #8 × decentralization posture).

**Why:** C-7 certified witness-validation SOUND. My backend-lock ruling conditioned bbolt on
"coexistence test passes." It FAILED: box did not OOM but thrashed to network-death
(free -m used=1968/1976, available=8; sshd couldn't fork; ens4 dead at 22:05).

**The coupling the consult framing understated (VERIFIED myself):** the measured workload is a
BUILD-FROM-EMPTY of 1M keys — `trie.Update × 1M` in a tight loop (`store_profile_test.go:116-128`),
the most write-amplifying/page-cache-hostile phase a store ever runs — under a full-duration
balloon (branch `builder/coexistence-balloon`, held whole scale loop, guard PASSED). It did NOT
measure STEADY-STATE residency (small applies against a warm-then-reclaimed tree). So the FAIL
proves the FLOOR BOX MUST NOT BUILD A 1M TREE UNDER PRESSURE. It does NOT prove a
already-built tree with witness-style small applies OOMs. Steady-state coexistence is UNMEASURED.

**Answers:**
- Q1 witness-validation: YES, decisively for the shipped floor. Not because hold-tree is proven
  impossible steady-state, but because it's certified-sound, it removes the build-under-pressure
  wall entirely, and it needs no un-run measurement to ship.
- Q2 backend reopen: NO, does not reopen bbolt. Spiral is mmap/page-cache-under-pressure, inherent
  to ANY page-cache store on a ~1 GB-pressured 2 GB box during BUILD. pebble's heap tied at 1M
  (304 vs 305) — same wall. Not a bbolt problem; a hold-tree-build problem.
- Q3 immutable #8: "give the box more RAM" is LEGITIMATE for the hold-tree posture (opt-in bigger
  box), ILLEGITIMATE for the floor. The floor stays 2 GB; the tree moves off it.
- Q4 bottom line: witness floor default; hold-tree = bigger-box opt-in.

**The evidence hole owned:** the quantitative rssMB trace is on the box's inaccessible nohup log;
serial carried ZERO test output (confirmed: grep oom|killed = 0 hits; test writes user-space only).
Verdict rests on free -m snapshots + timeline, NOT a recovered RSS curve. Load-bearing conclusion
("did not OOM, did thrash") holds on the free -m + no-oom-in-serial evidence.

**The one gate that outlives this call:** witness-validation still needs the era-3 format to commit
BOTH roots over the completeness-proven field set (C-7 gate; Block commits neither today,
`chain.go:311-405`). And the consensus-weight probes (bonded/epochSet) must reach the oracle GREEN
before the format freezes. Witness completeness is DOWNSTREAM of field-enumeration completeness.

See [[ruling-keystone-node-store-backend]] (the gate this trips), [[ruling-keystone-probes-bonded-epochset]] (the freeze gate).
