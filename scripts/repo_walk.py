#!/usr/bin/env python3
"""The file walk every repo lint shares: this checkout's files, and no other's.

  scar:lint-walks-into-another-checkout-2026-09-11

THE DEFECT
  A lint that walks from the repo root with `rglob` descends into every nested checkout
  of the same repository. This machine keeps up to a dozen of them live at once — agent
  worktrees under `.claude/worktrees/`, each a full tree on a different branch. Measured
  2026-09-11 on a clean `main`:

      check_short_dark_tests.py   EXIT=1   610 problems, 610 of them from worktrees
      check_source_gates.py       EXIT=1     3 problems,   3 of them from worktrees
      check_conflict_markers.py   EXIT=0   but scanning 12 foreign trees to say it

  Zero of those findings were about this tree. They are other branches' files, judged
  against this branch's workflows and allowlists, and the verdict is meaningless.

WHY IT WAS INVISIBLE
  A CI checkout has no worktrees, so all three lints are green there and stay green.
  The whole cost is local, and it is the cost that matters: a lint that reports 610
  false problems is a lint nobody runs, and an unrun lint is not a gate. That is the
  same "a green nobody can read is not a gate" shape these lints exist to prevent,
  turned on the lints themselves. It cost a seat real time on 2026-09-11, reasoning
  past a wall of foreign findings to reach a non-zero exit that meant nothing.

THE RULE, AND WHY IT IS NOT A PATH
  `.claude/worktrees/` is where THIS harness puts them. Excluding that one path would
  leave the class open — a clone dropped anywhere under the root reopens it, and the
  next lint to be written would have to remember the same path by hand. That is how
  this defect exists at all: two skip-lists that disagreed about one directory name.

  So the test is the marker git itself writes. A directory below the root that holds a
  `.git` entry is the top of a SEPARATE checkout with its own HEAD:

    - a `git worktree` writes `.git` as a FILE containing `gitdir: ...`
    - a nested clone or a submodule writes `.git` as a DIRECTORY

  `is_other_checkout` tests for either. It holds wherever the checkout sits, it needs
  no subprocess, and it still works in a `git archive` export with no `.git` anywhere
  (nothing is pruned, which is correct: an export has no nested checkouts). The root
  itself is never a candidate — it is the checkout doing the walking.

SELF-TEST
  CI can never observe this defect: a CI checkout has no nested tree to walk into, so
  a gate that merely runs the lints is decoration. `--self-test` MANUFACTURES the
  condition in a temp directory and asserts both directions, including the direction
  that matters most — a fix that excluded too much would produce exactly the green you
  were hoping for. Run: `python3 scripts/repo_walk.py --self-test`.
"""

import os
import sys
from pathlib import Path

SCAR_ID = "scar:lint-walks-into-another-checkout-2026-09-11"

# `.git` is never a source directory, in this checkout or any other. Pruned always, so
# a caller cannot reopen the hole by forgetting it.
ALWAYS_PRUNE = {".git"}


def is_other_checkout(path) -> bool:
    """True iff `path` is the top of a DIFFERENT checkout of a repository.

    A worktree's `.git` is a file, a nested clone's is a directory; `exists()` covers
    both. Pass a directory BELOW the walk root only: the root holds the very `.git`
    entry that makes it a checkout, and pruning it would empty every walk.
    """
    return (Path(path) / ".git").exists()


def repo_files(root, skip_dirs=(), suffix=None):
    """Yield every file under `root` that belongs to THIS checkout.

    Prunes any directory named in `skip_dirs` (matched by NAME at any depth, which is
    what the callers' previous `set(rel.parts)` tests did) and any directory that is
    the top of another checkout. Symlinked directories are not followed, so the
    gitignored `.claude/agent-memory` symlink cannot pull an out-of-tree store into the
    scan. Order is os.walk order; callers that print findings should `sorted()` it.
    """
    root = Path(root)
    skip = set(skip_dirs) | ALWAYS_PRUNE
    for dirpath, dirs, files in os.walk(root):
        dirs[:] = [d for d in sorted(dirs)
                   if d not in skip and not is_other_checkout(Path(dirpath) / d)]
        for name in sorted(files):
            if suffix is not None and not name.endswith(suffix):
                continue
            yield Path(dirpath) / name


# ------------------------------------------------------------------------- self-test

def _self_test() -> int:
    """Build a root with a nested checkout in it and assert the walk's two directions."""
    import tempfile

    failures = []
    with tempfile.TemporaryDirectory() as tmp:
        root = Path(tmp) / "repo"
        (root / ".git").mkdir(parents=True)               # the root IS a checkout
        (root / "core").mkdir()
        (root / "core" / "own_test.go").write_text("own\n")

        # A worktree: `.git` is a FILE. This is the shape on this machine.
        wt = root / ".claude" / "worktrees" / "agent-x" / "core"
        wt.mkdir(parents=True)
        (wt.parent / ".git").write_text("gitdir: /elsewhere\n")
        (wt / "foreign_test.go").write_text("foreign\n")

        # A nested clone: `.git` is a DIRECTORY, and it is NOT under `.claude/`. A
        # path-based exclusion misses this one; the marker-based rule does not.
        clone = root / "third_party" / "silt-clone"
        (clone / ".git").mkdir(parents=True)
        (clone / "clone_test.go").write_text("clone\n")

        # A directory that merely LOOKS like a worktree but carries no `.git`. It is
        # this tree's own file and MUST still be scanned: this is the over-exclusion
        # arm, and it is the one a too-broad fix fails.
        lookalike = root / ".claude" / "worktrees-notes"
        lookalike.mkdir(parents=True)
        (lookalike / "notes_test.go").write_text("mine\n")

        got = {p.relative_to(root).as_posix()
               for p in repo_files(root, suffix="_test.go")}

        for want in ("core/own_test.go", ".claude/worktrees-notes/notes_test.go"):
            if want not in got:
                failures.append(f"own file {want!r} was NOT yielded — the walk excludes "
                                f"too much, and its green says nothing")
        for unwanted in (".claude/worktrees/agent-x/core/foreign_test.go",
                         "third_party/silt-clone/clone_test.go"):
            if unwanted in got:
                failures.append(f"foreign file {unwanted!r} WAS yielded — the walk "
                                f"descended into another checkout")
        if not is_other_checkout(wt.parent):
            failures.append("a worktree (`.git` FILE) was not recognised as a checkout")
        if not is_other_checkout(clone):
            failures.append("a clone (`.git` DIRECTORY) was not recognised as a checkout")
        if is_other_checkout(lookalike):
            failures.append("a plain directory was misread as a checkout")

    if failures:
        print(f"FAIL [{SCAR_ID}] — the shared repo walk is wrong:", file=sys.stderr)
        for f in failures:
            print(f"  {f}", file=sys.stderr)
        return 1
    print(f"OK [{SCAR_ID}] — the shared walk skips a nested worktree and a nested clone, "
          f"and still yields this checkout's own files.")
    return 0


if __name__ == "__main__":
    if "--self-test" in sys.argv[1:]:
        raise SystemExit(_self_test())
    print(__doc__.split("\n")[0])
    print(f"usage: {Path(__file__).name} --self-test", file=sys.stderr)
    raise SystemExit(2)
