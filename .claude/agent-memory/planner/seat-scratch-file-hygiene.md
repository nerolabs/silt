---
name: seat-scratch-file-hygiene
description: When routing to any seat (esp. Researcher/PE with no delete tool), require scratch/repro files go to a non-committed location — never core/*_test.go in the tree.
metadata:
  type: feedback
---

Any seat that writes a scratch or reproduction file MUST put it in a location that cannot be accidentally
committed — NOT inside the source tree (e.g. never `core/chain/researcher_scratch_test.go`).

**Why:** Session-9, the Researcher wrote a repro to `core/chain/researcher_scratch_test.go`, could only
BLANK it (it has no delete tool), and left it in main's working tree where a stray commit could pick it up.
Andrew flagged it and told me to remind the seats to use safer directories. A blanked-but-present
`*_test.go` in a package tree is a landmine (compiles, clutters, risks commit).

**How to apply:** In every Researcher/PE/Tester prompt that may need a scratch repro, instruct: write
throwaway repros to `/tmp/…` (or a `git`-ignored scratch dir), and if a repro must live in-repo to compile
against internal symbols, DELETE it before finishing (Testers/Builders have a shell and can `rm`; the
Researcher/PE cannot — so THEY must use `/tmp`, or hand the repro to a shell-capable seat to place+remove).
When a seat reports leaving a scratch file, route its removal immediately (a shell-capable seat), the way
the session-9 blank-scratch cleanup was handled.

Related: the seats are a deployed SNAPSHOT — this reminder is applied via PROMPTS, not by editing the seat
personas in `silt/.claude/agents/` (that would fork the source of truth). If it should be permanent,
iterate it in `../agent-orchestra/` and re-deploy.
