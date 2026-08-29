---
name: planner-isolate-mutating-seats
description: When spawning parallel seats on the same repo, isolate any MUTATOR into its own worktree; require a COMMIT HASH before review; and make structural guards robust (check emission not declaration; match call-syntax not symbol names).
metadata:
  type: feedback
---

When launching multiple seats in parallel on the same repo, give any seat that MUTATES the working
tree its own git worktree (`isolation: "worktree"`). Read-only reviewers can share the tree; a mutator
cannot safely share it with anything.

**Why:** session-8 (2026-08-27) I launched a blind PE review and a Tester in parallel on branch
`keystone-probes-bonded-epochset`. Both ran an injected-defect ablation that edits the tree
(inject → run → revert). They also raced each other's `go test` compiles against the shared tree. It
did not bite this time — both returned coherent results — but that was luck, not design. A mid-window
interleave could have made the PE compile the Tester's injected-defect state and render a false
verdict, or produced a git conflict.

**How to apply:** before sending parallel `Agent(...)` calls that operate on the same repo, ask which
of them writes files or runs git mutations. Put each writer in `isolation: "worktree"`. A pure
non-tree action (e.g. `gh issue create`) is safe to share. This is the orchestration analogue of the
cloudtest shared-state-clobber scar the Tester already carries.

## ★ COMMIT-BEFORE-REVIEW — a deliverable with no commit hash is NOT a pinned artifact (session-10, era-3 step 1)
A worktree-isolated Builder finished era-3 step 1 and reported "Branch: era3-state-root-step1" but gave
**no commit hash** — the work was UNCOMMITTED, living only in the Builder's live worktree; the branch ref
still pointed at main. I routed PE + Tester to `git checkout era3-state-root-step1` anyway. The branch had
none of the work; both reviewers reached into the Builder's LIVE worktree, and the PE snapshotted a
non-compiling MID-EDIT and had to RECONSTRUCT a compiling version to review — a non-reproducible verdict.

**Rules now:**
1. A Builder's report MUST include the commit HASH + `git show --stat`. NO hash = not pinned = do NOT route
   to review. Send it back to commit first. (The leave-one-out Builders all reported hashes; this one didn't.)
2. Reviewers check out a specific COMMIT HASH (`git checkout --detach <hash>`), NOT a branch name, and must
   STOP + report if the artifact isn't committed — never reconstruct it from a live worktree.
3. Instruct the Builder to `git add` explicitly (a `git commit -am` skips new untracked files — the #623 scar).

## ★ STRUCTURAL GUARDS MUST FIRE ON THE EFFECT, AND MATCH THE CALL — not declarations or symbol names (session-10)
Two sibling misses in the same era-3 work; both are the decoration/false-confidence class one level down from
the probe meta-guard:
- **Check EMISSION, not DECLARATION.** The state-root coverage guard checked the static tag LIST, not that a
  leaf is actually EMITTED — a field's leaf-emission loop could be dropped (tag still present) with the suite
  GREEN, so the field silently escapes the root. A guard must fire on the ABSENCE OF THE EFFECT (a missing leaf).
- **Match CALL-SYNTAX, not a symbol NAME.** The Reload write-set guard decided a disk-write method was "guarded"
  via `strings.Contains(body, "validateEra3Roots")` — which matches COMMENT text, so `// validateEra3Roots
  skipped` + a bare `c.apply(b)` scored GUARDED while running no check. Fix: strip comments + require the name
  followed by `(`. A grep-guard that keys on a symbol name is defeatable by a comment containing that name.
- **General rule:** when a guard protects an invariant structurally, adversarially ablate the GUARD itself —
  can it be satisfied WITHOUT the real effect (a stale declaration, a comment, a shadowing probe)? If yes, it's
  decoration. (Tester ledger sibling: `builder/feedback_grep-guards-match-calls.md`.)
