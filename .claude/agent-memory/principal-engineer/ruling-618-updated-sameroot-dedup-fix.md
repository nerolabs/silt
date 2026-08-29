---
name: ruling-618-updated-sameroot-dedup-fix
description: PR #618 updated — same-root distinct-ID bond-reg validity rejection; SHIP; genesis apply() seam is the residual the freeze depends on
metadata:
  type: project
---

# Ruling: PR #618 (updated) — same-root distinct-ID bond-reg validity rejection

**Verdict: SHIP.** Head `4c10525`, branch `order-independence-bond-registration-family`.
Ruling: `/Users/andrewedmond/Claude/claude/silt-reviews/principle-engineer/RULING-618-updated-sameroot-dedup-fix-2026-08-28.md`.
Certified against `same-root-intrablock-bondreg-contention-RESEARCH-CERTIFICATION-2026-08-28.md` (resolution (a)).

**Why:** The certified fix (validity-layer per-root dedup) is implemented faithfully.
The change is confined to `validateBondRegs`.

**Premises I verified myself (file:line at PR head):**
- `core/chain/chain.go:1482` — `seenRoot` created OUTSIDE `if gate` → runs unconditionally (certified caveat met).
- `chain.go:1485` — `ok && prev != id` rejects distinct-ID same-root; `prev==id` (renew/resize) admitted.
- `chain.go:2199` — `validateBondRegs` has ONE caller: `ValidateCommit`.
- `Reconcile` (`chain.go:3171`) re-validates a peer fork via `Append`→`ValidateCommit` → guard RUNS on the untrusted path.
- `appendStructural` (2665) skips `validateBondRegs` but only replays own already-committed history → safe by induction.
- I ablated the guard and MEASURED the divergence: order [A,B]→owner=A, [B,A]→owner=B, byte-different `bonded`/`bondRootOwner`. Restored → order-independent by rejection. Ablation is honest RED-then-GREEN.
- Flipped `TestSharedRootDeniedViaValidatedBlock`: strictly stronger (0 bonded + reject vs old "exactly 1 bonded" which WAS the order-dependent commit). Forced consequence of the rule, not a goalpost move.
- Scope: diffed `main...4c10525`; no production line in `apply()`/Weight/epoch/quorum changed — every such mention in the diff is a comment.

**The coupling the consult missed (residual R-G):** `AppendGenesis` (`chain.go:2768`)
goes straight to `apply()` and does NOT run the dedup. Genesis `apply()` IS
order-dependent for two distinct-ID UNPROVEN same-root regs (traced 2800-2831,
proven=false). Safe today ONLY by genesis byte-identity across nodes ("declared not
agreed", 2724-2727) — an EXTERNAL invariant, not the guard. The era-3 freeze property
("for any block B, apply(B) order-independent") does NOT hold for genesis in isolation.
Before the freeze: record the byte-identity premise in the gate OR extend `seenRoot` to
the genesis apply path. Not a #618 blocker.

**How to apply:** #618 is shippable. Carry R-G into the era-3 freeze-gate checklist next
to the bonded/bondRootOwner/bondRootProven probe requirements from
[[ruling-618-bond-order-independence]].
