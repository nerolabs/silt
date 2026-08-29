# Deploying the orchestra to silt — do this when the tree is CLEAN

Staged, not applied. Do not run any of this until the current silt builder session has
landed and `git status` in `silt/` is clean — introducing agents to a dirty tree recreates
the merge mess you are avoiding.

## Prerequisites (all three, before step 1)

1. **Task 001 has run in `agent-orchestra/` and the tension read as signal, not noise.**
   This is the evidence that the seats work at all. Do not skip it.
2. **The silt tree is clean** (builder session landed, `git status` clean).
3. **You are on the gates.** The orchestra augments silt's process; you still ratify every
   immutable-trade and scope call.

## Step 1 — copy the seats and the coordination file

```bash
# from ~/Claude/claude
mkdir -p silt/.claude/agents
cp agent-orchestra/.claude/agents/*.md            silt/.claude/agents/
cp agent-orchestra/deploy/silt/CLAUDE.md          silt/.claude/CLAUDE.md
```

That gives silt the five seats and the coordination bridge. silt's own canon
(`docs/TENETS.md`, `decisions.md`, `build-process.md`) is untouched and remains the law.

## Step 2 — read-only shakedown (no build risk)

Before the Builder edits anything, run the two read-only seats on real silt work:

- **PE seat on the next consult or diff.** It is read-only and files to
  `silt-reviews/principle-engineer/`. Compare its ruling to what your hand-review would
  have said. If it catches what you would have caught — and pushes where you would have
  pushed — the persona is calibrated. If not, fix it in `agent-orchestra/` and re-copy.
- **Tester shadowing a deep run.** It runs silt's existing harness read-only and reports.
  Confirm its evidence-first discipline (captures before teardown) on a real run.

## Step 3 — one small, off-critical-path task, full loop

Pick a self-contained silt task that cannot hurt the RC. Route it through the Planner:
Builder builds → Tester proves → PE reviews blind → Planner surfaces tension / escalates.
Watch whether the tension sharpens the answer or just adds noise.

## Step 4 — only then, real work — with you on the gates

Graduate to critical-path work only after step 3 reads clean, and keep every veto-gate
decision at your desk.

## If you change a seat later

The seats in `silt/.claude/agents/` are copies. Iterate in `agent-orchestra/` (the
sandbox), prove the change there, then re-copy. Do not fork the two sets — the orchestra is
the source of truth; silt holds a deployed snapshot. Once the design is stable and you move
to the SDK (see `../../SDK-MIGRATION.md`), the personas port verbatim and this copy step
goes away.
