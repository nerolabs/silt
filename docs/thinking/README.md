# Thinking log — the "why" behind directions taken

**Purpose.** This directory records the *deliberation* behind build decisions — the
pace-before-code thinking (`TENETS.md` build-immutable #7 governs *evidence* for a step;
[the pace-before-code discipline] governs *which* step among alternatives). It exists so
the owner, the principal engineer, and the research team can examine the historical
reasoning — not just the code and not just the per-bug consults — and **spot trends in the
thinking that need correcting.** On a problem this hard, more recorded thinking is a
first-class asset: recording is cheap compared to the thinking itself.

Provoked by the standing feedback (Andrew, 2026-08-14): *"I feel sometimes you get in your
own way by acting before thinking — this problem takes thinking before acting."* The whole
consensus arc (#357 → B2 → #397 → #402: one bug re-derived four times) is the receipt for
what skipped deliberation costs.

## The rules

1. **One dated entry per deliberation:** `docs/thinking/YYYY-MM-DD-<slug>.md`.
2. **It ships in the SAME PR as the work it steered** (a standalone docs PR when the
   deliberation produced no code), so the "why" travels with the change and is visible in
   review — never reconstructed from archaeology later.
3. **It is a decision record, not narrative therapy.** PE's watch-out (canon-as-real-time-
   self-therapy during the WAN thrash) applies here in full. Keep to the template. The
   **third-time rule** holds: when a thinking *trend* is spotted (here or by a reviewer),
   the fix is a **test or a gate**, not another prose entry.
4. **Link it** from the PR body and from the issue it addresses.

## The template

```markdown
# <date> — <one-line title of the decision>

**Context / trigger:** what task or finding prompted this. Link the issue/PR.

**Evidence (per build-immutable #7 — cite artifacts, not vibes):**
- <the specific log line / code site `file:line` / test / measured number that grounds this>

**Options weighed:**
- **(A) <name>** — cost / benefit / risk / what it forecloses.
- **(B) <name>** — cost / benefit / risk / what it forecloses.
- **(C) <name>** — …

**Decision + rationale:** which option, and *why* it beat the others (not just why it's good).

**What would change my mind:** the evidence or ruling that would reopen this. (For
consensus/security/claim-touching calls: note the research/owner gate explicitly.)

**Status:** proposed / built / superseded-by <entry>.
```

[the pace-before-code discipline]: ../../ — see the standing memory `silt-pace-before-code`
and `TENETS.md` Part IX build-immutables #6/#7.
