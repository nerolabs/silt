---
name: era4-regcap-value-verdict-2026-08-29
description: Q2 RegCap-value close — GATED; fresh-only counting REFUTED (renewals also fill buckets, #506 is per-identity not per-block); measurement CERTIFIED but sizes wrong rule.
metadata:
  type: project
---

Q2 (era-4 RegCap value) does NOT lift to CERTIFIED. Verdict GATED, filed
`/Users/andrewedmond/Claude/claude/silt-reviews/research/research-outcome/era4-regcap-value-VERDICT-2026-08-29.md`.

**The split:** the Tester measurement is CERTIFIED (honest fresh ceiling = 1 reg/block under
heavy-`Answer` `verifyBond`; #299 NOT shipped — grep confirms forward-refs + one measurement-only
test `core/bond/answer_size_measure_test.go`; upper bound ≤16,384 tight; boundary composition
256×8×16KiB=32MiB fits). But the RULE the value sizes — "RegCap counts FRESH (ok==false) regs
ONLY, renewals excluded" — is **REFUTED**.

**Why (the decisive artifact):** due-buckets are filled by EVERY BondReg, fresh AND renewal, at
the SAME site `chain.go:2995-2996` (renew resets `bondRegHeight` → lands in bucket
`D=h+ttl+1`). #506 (`regMinInterval` `chain.go:3288-3300`, gated `chain.go:1587` on `ok`) bounds
renewals PER IDENTITY, not per block. O(registry) distinct existing ids can each renew once in one
block (`seenReg` `chain.go:1583-1585` only blocks same-id-twice), all landing in one bucket → at
fire height that bucket is O(registry) → TTL-firing `C_block=O(registry)×SProofMax`. Fresh-only
cap of 256 sits idle. NO validity count cap exists in core today (grep RegCap/len(b.BondRegs) =
only alloc + empty-guards; byte budget is proposer-only `node.go:79-80`).

**Correction to prior direction:** this is the SAME refutation shape RECERT round-1 applied
(#506 = per-identity freq, NOT per-block distinct-id count). The fresh-only variant re-opened the
gap on the RENEWAL axis. The task's own premise ("renewals excluded because #506-rate-limited")
carries the error: per-identity rate-limiting ≠ per-block volume-limiting.

**Corrected rule to lift:** cap the DUE-BUCKET population (Option A per-bucket size cap) or
per-BLOCK total BondReg count fresh+renewal (Option B). Then re-measure honest ceiling as
`2 MiB / min-valid-RENEWAL-reg-byte-size` (renewals pack SMALLER than fresh per `node.go:71-73`,
so ceiling > 1, un-measured). That min-renewal-reg-size is the one input still owed; until then
256 not certified above the honest renewal floor. #299 re-mint stays a HARD residual but is NOT
the only one — R-CAP-RULE (fresh-vs-bucket) is the prior, #299-independent gate.

Cross-ref [[era4-witnessable-transitions-recert]] (Q2 was MEASUREMENT-REQUIRED there) and
[[era4-regcap-measurement-2026-08-29]] (the tester inputs — measured the FRESH min, owes the
RENEWAL min). NOTE: design doc is RATIFIED at value=256 (options doc header) — this verdict
REOPENS that value on the counting-rule axis; a human/Builder call.
