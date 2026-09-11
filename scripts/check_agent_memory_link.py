#!/usr/bin/env python3
"""Repo lint + agent hook: live agent memory must be a SYMLINK to the shared store.

SCAR (scar:worktree-agent-memory-written-to-a-doomed-dir-2026-09-11):
  `.claude/agent-memory` is gitignored and lives OUTSIDE git, as a symlink to the shared
  store `~/.claude/silt-agent-memory`. A fresh `git worktree` does NOT carry a gitignored
  symlink. A seat running in such a worktree that saved a memory therefore had the path
  created for it as a REAL LOCAL DIRECTORY — and that directory died with the worktree.

  Measured 2026-09-11: ALL TEN live worktrees under `.claude/worktrees/` held a real
  directory. Nine held content: 25 memory files across the builder and tester seats, all
  recovered. Five further files were already unrecoverable, their worktrees pruned.
  Nothing was red at any point. The seats reported their memories saved, and they were —
  to a location scheduled for deletion. That is the failure mode this guard exists for:
  not a write that FAILS, but a write that SUCCEEDS into a doomed location.

THE RULE:
  A checkout's `.claude/agent-memory` must be a symlink whose target is the shared store
  directory. Three states fail:

    LEAK    — the path exists and is not a symlink. Memory is being written somewhere
              that will be deleted. Any content already there is named in the failure.
    UNWIRED — the path is absent. This is not a safe steady state: it is the state ONE
              WRITE away from LEAK, because the write creates the parent directory. It
              is red for the same reason and takes the same one-command remedy. A guard
              that only fires once the doomed directory exists fires after the condition
              it exists to prevent.
    TARGET  — the path is a symlink somewhere other than the shared store, or dangling.

  The remedy for all three is the same and is named in every failure message:
  `.claude/setup-agent-memory.sh`.

WHERE THIS RUNS (the whole point):
  CI on `main` can never see a worktree, so a CI-only wiring would be decoration. The
  load-bearing wiring is `.claude/settings.json` — tracked, therefore present in every
  worktree from the moment it is created:

    PreToolUse on Write|Edit  fires on a seat's FIRST attempt to save a memory, BEFORE
                              any byte is written, and BLOCKS it (exit 2). No data can
                              be lost, because the losing write never happens.
    SessionStart              tells a seat at the top of its session, before it has
                              written anything at all.

  In `--hook` mode the checkout root is derived from the WRITE TARGET PATH, not from
  this file's location and not from the cwd. A hook process may be launched from the
  main tree while the seat writes into a worktree; deriving the root any other way
  checks a directory that is not the one at risk, and passes.

  `--ci` is a DIFFERENT question, because a CI checkout never has the symlink at all
  (it is gitignored, so "absent" is CI's correct and harmless steady state — there are
  no seats there and no memory to lose). Running the symlink rule in CI would be red on
  every build. What CI CAN see, and nothing else can, is live memory committed INTO git:
  that is the #636/#638 incident, where tracked agent memory conflicted on every pull.
  So `--ci` asserts that nothing under `.claude/agent-memory` is tracked.

Dependency-free (stdlib only).
Run: python3 scripts/check_agent_memory_link.py [--hook|--ci] [--quiet]
  --hook   read a Claude Code hook payload on stdin; block (exit 2) a memory write into
           an unlinked path; stay silent (exit 0) on every unrelated tool call
  --ci     assert no live memory is tracked by git (#636/#638); ignores the symlink
  --quiet  suppress the OK line on success
"""
import json
import os
import subprocess
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent

SCAR_ID = "scar:worktree-agent-memory-written-to-a-doomed-dir-2026-09-11"

MEMORY_REL = Path(".claude") / "agent-memory"

# Kept in lockstep with `.claude/setup-agent-memory.sh`, which is the remedy this guard
# names. If that script's `target=` changes, this must change with it.
STORE = Path(
    os.environ.get("SILT_AGENT_MEMORY_STORE", Path.home() / ".claude" / "silt-agent-memory")
)

OK, LEAK, UNWIRED, TARGET = "OK", "LEAK", "UNWIRED", "TARGET"


def remedy(root: Path) -> str:
    return (
        "Fix (one command, idempotent, non-destructive) — run from the root of the\n"
        "checkout/worktree that is at risk:\n"
        f"    cd {root} && ./.claude/setup-agent-memory.sh\n"
        "  If it refuses because a real directory is already there, MOVE that\n"
        f"  directory's contents into {STORE}\n"
        "  first (never overwrite a same-named file there — the shared store is the\n"
        "  live copy), then remove the local directory and re-run.\n"
    )


def evaluate(link: Path):
    """Classify one `.claude/agent-memory` path. Returns (status, [message lines])."""
    # LEAK first, and `is_symlink()` before `exists()`: a dangling symlink is TARGET.
    if link.exists() and not link.is_symlink():
        kind = "directory" if link.is_dir() else "file"
        msg = [
            f"LEAK: {link} is a real {kind}, not a symlink.",
            "  Agent memory written here is NOT in the shared store. If this is a git",
            "  worktree, everything here is deleted when the worktree is removed.",
        ]
        if link.is_dir():
            try:
                files = sorted(p for p in link.rglob("*") if p.is_file())
            except OSError:
                files = []
            if files:
                msg.append(f"  {len(files)} file(s) currently at risk:")
                msg += [f"    {f}" for f in files]
        return LEAK, msg

    if link.is_symlink():
        try:
            resolved = link.resolve(strict=True)
        except (OSError, RuntimeError):
            return TARGET, [f"TARGET: {link} is a DANGLING symlink -> {os.readlink(link)}"]
        if resolved != STORE.resolve():
            return TARGET, [
                f"TARGET: {link} points at {resolved},",
                f"  not the shared store {STORE.resolve()}.",
                "  Memory saved here does not join the shared store, so other checkouts",
                "  and later sessions will not see it.",
            ]
        return OK, [f"{link} -> {resolved}"]

    return UNWIRED, [
        f"UNWIRED: {link} does not exist.",
        "  Agent memory is not linked to the shared store in this checkout. The next",
        "  memory a seat saves will CREATE this path as a real local directory, and in",
        "  a git worktree that directory dies with the worktree. Wire it before writing.",
    ]


def root_of_memory_path(p: Path):
    """The checkout root for a path that lies under some `.claude/agent-memory`.

    Returns None when the path is not a memory path at all — which is the common case,
    and the reason an unrelated Write is never blocked.
    """
    parts = p.parts
    for i in range(len(parts) - 1):
        if parts[i] == ".claude" and parts[i + 1] == "agent-memory":
            return Path(*parts[:i]) if i else Path(p.anchor or ".")
    return None


def _fail(status, lines, root, exit_code) -> int:
    print(f"FAIL [{SCAR_ID}] — " + lines[0], file=sys.stderr)
    for ln in lines[1:]:
        print(ln, file=sys.stderr)
    print("\n" + remedy(root), file=sys.stderr)
    return exit_code


def run_hook() -> int:
    """Claude Code hook entry point. Never blocks anything it does not understand."""
    try:
        payload = json.load(sys.stdin)
    except (json.JSONDecodeError, ValueError):
        return 0  # Unparseable payload must not wedge the session.
    if not isinstance(payload, dict):
        return 0

    event = payload.get("hook_event_name", "")
    tool_input = payload.get("tool_input") or {}
    raw = tool_input.get("file_path") or tool_input.get("notebook_path") or ""

    if raw:
        # A file-touching tool call. Only a MEMORY write is our business.
        root = root_of_memory_path(Path(raw))
        if root is None:
            return 0
        link = root / MEMORY_REL
        status, lines = evaluate(link)
        if status == OK:
            return 0
        # Exit 2 on PreToolUse blocks the call and feeds stderr back to the agent.
        return _fail(status, lines, root, 2)

    if event == "SessionStart":
        root = Path(payload.get("cwd") or os.environ.get("CLAUDE_PROJECT_DIR") or ROOT)
        status, lines = evaluate(root / MEMORY_REL)
        if status == OK:
            return 0
        # SessionStart cannot block. stdout becomes context the seat reads, which is
        # exactly what is wanted: it learns the remedy before it writes anything.
        print(f"[{SCAR_ID}] agent memory is NOT wired in this checkout — " + lines[0])
        for ln in lines[1:]:
            print(ln)
        print("\n" + remedy(root))
        return 0

    return 0


def run_ci(quiet: bool) -> int:
    """Assert git tracks nothing under `.claude/agent-memory` (the #636/#638 incident)."""
    try:
        out = subprocess.run(
            ["git", "ls-files", "--", str(MEMORY_REL)],
            cwd=ROOT,
            capture_output=True,
            text=True,
            check=True,
        ).stdout
    except (OSError, subprocess.CalledProcessError) as exc:
        print(f"FAIL [{SCAR_ID}] — could not ask git what is tracked: {exc}", file=sys.stderr)
        return 1

    tracked = [ln for ln in out.splitlines() if ln.strip()]
    if tracked:
        print(
            f"FAIL [{SCAR_ID}] — {len(tracked)} live agent-memory file(s) are TRACKED BY GIT:",
            file=sys.stderr,
        )
        for t in tracked:
            print(f"    {t}", file=sys.stderr)
        print(
            "\n  Live agent memory is gitignored and lives OUTSIDE git on purpose. Committing\n"
            "  it caused a conflict on every pull (#636/#638).\n"
            "  Fix: git rm --cached -r .claude/agent-memory   (the files stay on disk)\n",
            file=sys.stderr,
        )
        return 1
    if not quiet:
        print(f"OK [{SCAR_ID}] — git tracks no file under {MEMORY_REL}.")
    return 0


def main(argv=None) -> int:
    argv = sys.argv[1:] if argv is None else argv
    if "--hook" in argv:
        return run_hook()
    if "--ci" in argv:
        return run_ci("--quiet" in argv)

    link = ROOT / MEMORY_REL
    status, lines = evaluate(link)
    if status != OK:
        return _fail(status, lines, ROOT, 1)
    if "--quiet" not in argv:
        print(f"OK [{SCAR_ID}] — " + lines[0])
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
