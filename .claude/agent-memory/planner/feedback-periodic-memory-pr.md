---
name: feedback-periodic-memory-pr
description: Seat memory reaches main via a PERIODIC memory PR (batched at milestones), not per-change and not Andrew committing directly.
metadata:
  type: feedback
---

Seat memory (`.claude/agent-memory/**`) reaches `main` via a **periodic memory PR** — batch the accumulated
memory changes and open one docs-style PR at a sensible checkpoint, rather than a PR per edit or Andrew committing directly.

**Why:** since #636 tracks `.claude/` and `main` is PR-only, seat memory written during a session lives UNCOMMITTED in the
working tree — no longer wiped by a checkout (the session-12 scar) but also not durable until committed. Andrew chose the
periodic-PR cadence (2026-08-29) over per-change commits (too much churn) and over committing directly (main is PR-only anyway).

**How to apply:**
- Batch memory at milestone boundaries (a merge banked, a design closed, session end) — not after every Write.
- The Planner has NO Bash → route a Bash seat to stage ONLY `.claude/agent-memory/**` and open the PR. Do NOT sweep
  unrelated working-tree changes or other seats' in-progress edits; gather deliberately (the session-12 sweep scar).
- Keep the memory PR docs-only so it stays off the code-review critical path.
- Related: [[session-resume]] flags the pending uncommitted memory each session.
