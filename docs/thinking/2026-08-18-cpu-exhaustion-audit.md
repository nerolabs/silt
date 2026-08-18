# CPU-exhaustion audit — expensive work per remote input (the #424 family)

**Date:** 2026-08-18 · **Companion to** the memory boundedness audit
(`2026-08-18-boundedness-audit.md`). Same lens, CPU as the resource: **can a remote
party force expensive/unbounded CPU on the single loop (B2) without paying a cost?**
An ungated expensive-per-message op monopolizes the loop and starves consensus — a
liveness DoS — even after the inbound MEMORY gate (which bounds bytes in flight, not
CPU per message). *bounded-then-fast* applies to CPU too.

## Method

For each inbound message kind, find the most expensive work it triggers, and ask: is
there a CHEAP gate in front of the costly op (a per-source rate-limit that refuses
WITHOUT paying), or does every remote message pay full price?

## Findings

| inbound kind | expensive work | gated? | verdict |
|---|---|---|---|
| `MsgBondChallenge` | `AnswerSpaceTime` (VDF-eval) | **YES** — `allowBondChallenge` per-challenger burst (#424) | ok (the template) |
| **`MsgSubmitBondReg`** | **`ValidateBondRegErr` → `VerifySpaceTime`** (VDF-eval + label samples over a ~1.5 MB proof) | **NO per-submitter gate** | **FINDING — the #424 gap on the submit path** |
| `MsgChallengeReply` (audit) | `por.Verify` | self-initiated (we challenge; replies bounded by our own audit rate) | ok — not remote-driven |
| `MsgStoreChunk` | `verifyStorageProof` (Merkle, O(log n)) + `c.Verify` (one hash) | cheap; + inbound gate | low |
| `MsgTokenRequest` | blind-sign (RSA) | blind-token protocol gates issuance | verify separately (medium) |
| per-message signature verify | ed25519 (~tens of µs) | cheap; + inbound gate | low |

## The finding — `MsgSubmitBondReg` forces an ungated VDF-eval

`chainrole.go:447` runs `n.chain.ValidateBondRegErr(reg)` on every submitted reg,
which invokes the bond verifier → `bond.VerifySpaceTime(...)` (`objectivechain.go:36`)
— a VDF-eval + label-sample verification of a ~1.5 MB space-time proof. There is **no
per-submitter rate-limit** in front of it (unlike the #424-gated bond-CHALLENGE path).
An attacker signing garbage-proof regs with its own key (signing is cheap; the proof
is what's expensive to *verify*, and it's verified to be found bad) forces the node to
burn a VDF-eval per submission, monopolizing the single loop and starving consensus.
The inbound backpressure gate does not help: it admits the message; the loop then pays
full verify cost.

## Why I did NOT fix it overnight (flagged for PE)

The fix is not a quick safe change — it is **structurally the inbound-backpressure saga
again**, on the **most drain-sensitive path in the system**:
- **v1-equivalent (per-submitter rate-limit):** mirror `allowBondChallenge` —
  `allowBondRegSubmit(from)`, a cheap per-submitter burst gate that refuses the
  expensive `ValidateBondRegErr` when a submitter floods. Refuse ≠ drop-forever: a
  refused submit is re-sent on the submitter's next sweep (exactly like the existing
  self-healing "signature" refusals, `chainrole.go:452`), so maturity isn't starved —
  **provided the burst clears the honest resubmit rate.**
- **v2-equivalent (the cohort case):** per-submitter alone doesn't bound a MULTI-KEY
  flood (signing is cheap, so an attacker mints many keys). A **global verify-budget**
  + a **consensus-priority reservation** (never let reg-verify starve prepare/precommit
  processing) is the cohort defense — the exact parallel to the inbound v2b reserve.

**The risk is the drain.** #338/#441/#448 were hard-won fixes for regs *not draining*
(refused/stale → maturity starves). A rate-limit that refuses regs must be sized above
the genuine resubmit rate (per-submitter, so genesis's many-distinct-validators burst
is fine; the concern is one skewed validator's retry rate — the field showed ~tens of
resubmits per validator per run, well under a `#424`-style burst of 8/window). I'm
fairly confident an 8–16/window per-submitter burst is safe, but the **PE owns the
drain gate** and should confirm the sizing against the run history before it lands. A
too-tight gate reintroduces a certified-closed starvation bug — not worth guessing
autonomously.

## Recommendation / staging

- **Before red-team #183** (it's a real DoS seam, #7): land `allowBondRegSubmit`
  (per-submitter, generous burst, refusal LOGGED per B5) + the global verify-budget +
  consensus-priority, mirroring the inbound v1/v2a/v2b structure. PE to confirm the
  burst sizing.
- Verify the `MsgTokenRequest` blind-sign path is rate-limited (medium; RSA sign is
  not free).
- Regression: a CPU-scaling test (a submit flood must not exceed N verifies/window;
  honest resubmit bursts pass) — the CPU twin of the memory-scaling regressions.

The two audits together (memory + CPU) are the *bounded-then-fast* discipline applied
across both resources: no input path, memory or CPU, unbounded without a cheap gate.
