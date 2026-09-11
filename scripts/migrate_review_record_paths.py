#!/usr/bin/env python3
"""Rewrite every reference to the retired `silt-reviews/` tree at its new home.

WHY THIS IS A SCRIPT AND NOT JUST A DIFF
========================================
The review record moved on 2026-09-11 (owner-ratified) from

    /Users/andrewedmond/Claude/claude/silt-reviews/<seat>/...

into the agent-memory store, one `reviews/` subtree per seat:

    /Users/andrewedmond/.claude/silt-agent-memory/<seat>/reviews/...

At the time of the move, four live worktrees carried ~3,400 occurrences of the old
path on UNMERGED branches, two of them held FORMAT items waiting on the owner's own
review queue. A one-shot rewrite would be partially REVERTED by each of those
branches at merge time — every held branch reintroduces the old paths in the files
it touches. So the rewrite ships as a re-runnable, idempotent tool:

    HELD BRANCH, BEFORE YOU MERGE IT:
        git checkout <held-branch>
        git merge origin/main            # or rebase
        python3 scripts/migrate_review_record_paths.py          # rewrite in place
        python3 scripts/migrate_review_record_paths.py --verify  # must exit 0
        python3 scripts/gen_changelog.py && python3 scripts/gen_roadmap.py
        git commit -am "chore: re-run the review-record path migration"

Running it twice is a no-op: the output contains no `silt-reviews` token, so the
second pass matches nothing. That is what makes it safe to hand to any branch.

WHAT IT DOES NOT DO
===================
It rewrites PATHS. It does not rewrite CLAIMS. A sentence such as "`silt-reviews` is
not a git repository" is a statement of fact about the old tree; mechanically swapping
the token would turn it into a false statement about the new one. Such lines were
corrected by hand in the migration and are not this tool's business. `--verify` still
catches them, which is the point: an un-rewritable occurrence should stop a human, not
be silently mangled.
"""

import argparse
import os
import re
import subprocess
import sys

SCAR = "scar:review-record-moved-and-every-citation-went-dark-2026-09-11"

OLD_ABS = "/Users/andrewedmond/Claude/claude/silt-reviews"
NEW_ABS = "/Users/andrewedmond/.claude/silt-agent-memory"
NEW_TILDE = "~/.claude/silt-agent-memory"
NEW_BARE = "silt-agent-memory"

# The four syntactic forms the old path was written in, longest first so the
# alternation never matches a suffix of a longer form.
ROOT_RE = re.compile(
    r"(?P<root>"
    + re.escape(OLD_ABS)
    + r"|\.\./\.\./silt-reviews"
    + r"|\.\./silt-reviews"
    + r"|silt-reviews"
    + r")"
    r"(?P<rest>/[A-Za-z0-9._-]+)?"
)

ROOT_MAP = {
    OLD_ABS: NEW_ABS,
    "../../silt-reviews": NEW_TILDE,
    "../silt-reviews": NEW_TILDE,
    "silt-reviews": NEW_BARE,
}

# The first path component under the old root -> its component(s) under the new one.
# This table is CLOSED: it was built by enumerating every distinct first component
# across all three repositories (16 shapes, 1,868 occurrences) rather than guessed.
# A component not in the table keeps its name and is reported by --verify as a shape
# the table does not know, so a new one cannot pass through unnoticed.
SEAT_MAP = {
    "principle-engineer": "principal-engineer/reviews",   # the source misspelled it
    "research": "researcher/reviews",                     # covers research/research-outcome
    "red-team": "red-team/reviews",
    "redteam": "red-team/reviews/redteam",                # legacy alias
    "redteam-august-8th": "red-team/reviews/redteam-august-8th",
    "redteam-august-23": "red-team/reviews/redteam-august-23",
    "economist": "economist/reviews",
    "crypto-specialist": "crypto-specialist/reviews",
    "planner": "planner/reviews",
    # the field runs map to the TESTER seat (owner: "fieldtest maps to the tester agent")
    "fieldtest": "tester/reviews/fieldtest",
    "fieldtest-august-10": "tester/reviews/fieldtest-august-10",
    "fieldtest-august-10b": "tester/reviews/fieldtest-august-10b",
    "fieldtest-august-10c": "tester/reviews/fieldtest-august-10c",
    "fieldtest-august-12a": "tester/reviews/fieldtest-august-12a",
    # cross-seat, moved to the seat that holds the cross-seat vantage
    "OVERNIGHT-SUMMARY-2026-08-18.md": "planner/reviews/OVERNIGHT-SUMMARY-2026-08-18.md",
}

# Components that no longer exist anywhere. A reference to one is HISTORY, not a
# pointer: rewriting it would invent a destination. Left alone, reported by --verify.
RETIRED = {"handoff"}

# Not path components at all: prose ellipsis (`silt-reviews/.../FOO.md`) and a sentence
# that ends on the tree name. The root moves, the rest is left alone, and neither is
# worth a line in the unmapped report — reporting them would bury the real findings.
NOT_A_COMPONENT = {".", "..", "..."}

# Files that legitimately still contain the old token and must never be rewritten.
#  - this script: it holds the literal by definition
#  - website/*.html: GENERATED. Fix the .md source and re-run scripts/gen_*.py
#  - archive/reviews/: see the note below — those 23 files do NOT move, but their
#    PATH references still do, so they are rewritten like everything else and are
#    NOT exempt here.
EXEMPT_SUFFIXES = (
    "scripts/migrate_review_record_paths.py",
    "website/roadmap.html",
    "website/changelog.html",
)

# A line may name the retired tree ON PURPOSE — the gate's own name in ci.yml, a
# docstring explaining what moved, a changelog entry. You cannot out-lex a lexical gate,
# so such a line DISCLOSES itself by carrying the scar prefix. That is deliberately
# awkward to type by accident and trivially greppable, and it keeps the default answer
# for an undecorated occurrence at "this is a dead pointer, fix it".
HISTORY_LICENSE = "scar:review-record-moved"

TEXT_SUFFIXES = (
    ".md", ".py", ".go", ".sh", ".yml", ".yaml", ".json", ".txt", ".tf", ".toml", "",
)

SKIP_DIRS = {".git", "node_modules", "dist", "vendor", "__pycache__", ".terraform"}


def is_exempt(path: str) -> bool:
    p = path.replace(os.sep, "/")
    return any(p.endswith(s) for s in EXEMPT_SUFFIXES)


def rewrite_text(text: str):
    """Return (new_text, n_rewritten, [unmapped components seen])."""
    unmapped = []
    n = 0

    def sub(m):
        nonlocal n
        root = m.group("root")
        rest = m.group("rest") or ""
        comp = rest[1:] if rest else ""
        if comp in RETIRED:
            unmapped.append(comp)
            return m.group(0)          # history, not a pointer — leave it
        n += 1
        new_root = ROOT_MAP[root]
        if not comp:
            return new_root
        if comp in SEAT_MAP:
            return new_root + "/" + SEAT_MAP[comp]
        if comp in NOT_A_COMPONENT:
            return new_root + rest
        # An unknown component. Move the root but keep the component, and say so:
        # a shape the closed table does not cover must reach a human.
        unmapped.append(comp)
        return new_root + rest

    return ROOT_RE.sub(sub, text), n, unmapped


def candidate_files(root: str):
    """Every tracked-ish text file under `root`, honouring SKIP_DIRS.

    NEVER descends into another checkout. `.claude/worktrees/` holds full copies of
    this repo on other seats' branches; a tool that walks by PATH rewrites their
    files and reports them as ours (scar:lint-walked-into-another-checkout-2026-09-11,
    PR #813). A checkout is identified by its `.git` MARKER — a directory OR a file —
    not by its name, because a worktree's marker is a file and can live anywhere.
    """
    root = os.path.abspath(root)
    for dirpath, dirs, files in os.walk(root):
        if os.path.abspath(dirpath) != root and (
                ".git" in dirs or ".git" in files):
            dirs[:] = []
            continue
        dirs[:] = sorted(d for d in dirs if d not in SKIP_DIRS)
        for f in sorted(files):
            path = os.path.join(dirpath, f)
            if os.path.islink(path):
                continue
            if not any(f.endswith(s) for s in TEXT_SUFFIXES if s):
                # extensionless files: only take them if they are small and texty
                if "." in f:
                    continue
                try:
                    if os.path.getsize(path) > 1 << 20:
                        continue
                except OSError:
                    continue
            yield path


def default_root() -> str:
    try:
        out = subprocess.run(
            ["git", "rev-parse", "--show-toplevel"],
            capture_output=True, text=True, cwd=os.path.dirname(os.path.abspath(__file__)),
        )
        if out.returncode == 0 and out.stdout.strip():
            return out.stdout.strip()
    except OSError:
        pass
    return os.getcwd()


def main(argv=None) -> int:
    ap = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    ap.add_argument("--root", default=None, help="tree to process (default: this git repo)")
    ap.add_argument("--verify", action="store_true",
                    help="do not write; exit 1 if any un-exempt occurrence remains")
    args = ap.parse_args(argv)

    root = args.root or default_root()
    changed, occurrences, remaining, unmapped_all = 0, 0, [], {}

    for path in candidate_files(root):
        if is_exempt(path):
            continue
        try:
            with open(path, "r", encoding="utf-8") as fh:
                text = fh.read()
        except (OSError, UnicodeDecodeError):
            continue
        if "silt-reviews" not in text:
            continue

        new, n, unmapped = rewrite_text(text)
        for u in unmapped:
            unmapped_all.setdefault(u, []).append(path)

        if args.verify:
            for i, line in enumerate(text.splitlines(), 1):
                if "silt-reviews" in line and HISTORY_LICENSE not in line:
                    remaining.append("%s:%d: %s" % (
                        os.path.relpath(path, root), i, line.strip()[:140]))
            continue

        if new != text:
            with open(path, "w", encoding="utf-8") as fh:
                fh.write(new)
            changed += 1
            occurrences += n

    if args.verify:
        if remaining:
            print("FAIL [%s] — %d occurrence(s) of the retired `silt-reviews` path remain:"
                  % (SCAR, len(remaining)), file=sys.stderr)
            for r in remaining[:40]:
                print("  " + r, file=sys.stderr)
            if len(remaining) > 40:
                print("  … and %d more" % (len(remaining) - 40), file=sys.stderr)
            print("\n  Rewrite them with: python3 scripts/migrate_review_record_paths.py",
                  file=sys.stderr)
            print("  A line that states a FACT about the old tree cannot be rewritten "
                  "mechanically — correct the sentence by hand.", file=sys.stderr)
            print("  A line that names the retired tree ON PURPOSE discloses itself by "
                  "carrying `%s` on the SAME line." % HISTORY_LICENSE, file=sys.stderr)
            return 1
        print("OK [%s] — no reference to the retired `silt-reviews` path under %s"
              % (SCAR, root))
        return 0

    print("rewrote %d occurrence(s) across %d file(s) under %s" % (occurrences, changed, root))
    if unmapped_all:
        print("\nNOT rewritten (component outside the closed table, or retired):")
        for comp, paths in sorted(unmapped_all.items()):
            print("  silt-reviews/%s — %d file(s), e.g. %s"
                  % (comp, len(paths), os.path.relpath(paths[0], root)))
    if changed:
        print("\nGenerated pages are NOT rewritten. Re-run:")
        print("  python3 scripts/gen_changelog.py && python3 scripts/gen_roadmap.py")
    return 0


if __name__ == "__main__":
    sys.exit(main())
