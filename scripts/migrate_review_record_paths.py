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

# THE ROOT-LESS FORM — the class ROOT_RE cannot see.
# ==================================================
# A large part of the record cites the old tree with NO root at all:
#
#     research/research-outcome/FOO-RESEARCH-CERTIFICATION-2026-09-07.md
#     principle-engineer/RULING-residual-register-true-up-e963034-2026-09-07.md
#
# Its root was never written down — it was the reader's working-directory convention,
# one level up from this repo. ROOT_RE requires the `silt-reviews` token, so it never
# matched these, and `--verify` (which tests only for that token) reported OK while the
# reference went dark. 23 of them sat in this repo, 14 in the LIVE `docs/design/m0.md`
# §10 disclosure table that the era-4 freeze reads.
# scar:a-root-less-citation-resolved-against-one-assumed-root-2026-09-11 (PR #815, C-2).
#
# The rule is a CLOSED COMPLEMENT, not a pattern: flag exactly those old first components
# that CANNOT name anything under the new root, i.e. the ones SEAT_MAP renames. Derived
# from SEAT_MAP rather than hand-listed, so it cannot drift from it.
#
# `planner/`, `economist/`, `red-team/` and `crypto-specialist/` are deliberately NOT in
# the set: they keep their names as live directories in the store, so `planner/MEMORY.md`
# is a VALID store-relative pointer. Measured before drawing this line — 3 such live-seat
# occurrences exist in the store today, and flagging them would be a false positive. A
# lint that cries wolf gets disabled, so the FP rate is a correctness property here.
#
# A reference may name a DIRECTORY rather than a file — "their outputs live under
# `research/research-outcome/` and `redteam/m0-field-test/`". A `.md`-only pattern
# leaves those describing a layout that does not exist, which is the same defect one
# level up. Measured before widening: the trailing-slash form adds 5 occurrences in this
# repo and ZERO false positives, and 3 of the 5 are in the live `docs/design/m0.md` §10
# table. So `rest` ends in `.md` OR in `/`.
ROOTLESS_MAP = {
    old: new
    for old, new in SEAT_MAP.items()
    if not old.endswith(".md") and new.split("/")[0] != old
}
ROOTLESS_RE = re.compile(
    r"(?<![/\w.-])(?P<comp>"
    + "|".join(re.escape(c) for c in sorted(
        list(ROOTLESS_MAP) + list(RETIRED), key=len, reverse=True))
    + r")/(?P<rest>[A-Za-z0-9][A-Za-z0-9._/-]*(?:\.md|/))"
)

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

    def sub_rootless(m):
        nonlocal n
        comp = m.group("comp")
        if comp in RETIRED:
            unmapped.append(comp)
            return m.group(0)          # history, not a pointer — leave it
        n += 1
        return ROOTLESS_MAP[comp] + "/" + m.group("rest")

    # LINE-WISE, and a line carrying the history license is left ALONE.
    #
    # The license means "this occurrence names the old tree ON PURPOSE" — a gate's own
    # name in ci.yml, a docstring saying what moved. It has always suppressed --verify.
    # It did NOT suppress the rewrite, so re-running the tool silently converted
    #   "no reference to the retired `silt-reviews/` tree may reach a branch"
    # into a sentence about the tree that is NOT retired. Measured on 2026-09-11: a
    # second run falsified exactly those two licensed lines. The workflow above tells
    # every held branch to run this before merging, so each one re-broke them.
    #
    # A path rewrite must never rewrite a CLAIM. The license is the one place a human
    # has said which is which, so it binds in BOTH directions.
    # scar:a-path-rewrite-falsifies-claims-2026-09-11
    out = []
    for line in text.splitlines(keepends=True):
        if HISTORY_LICENSE in line:
            out.append(line)
            continue
        out.append(ROOTLESS_RE.sub(sub_rootless, ROOT_RE.sub(sub, line)))
    return "".join(out), n, unmapped


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


def self_test() -> int:
    """DRIVE both halves of the gate on a manufactured tree.

    WHY THIS EXISTS. `--verify` is green on this repo, and a green gate with no
    demonstrated red is decoration (canon simplicity rule 7). CI cannot manufacture
    either defect — a real root-less citation would have to be committed to see the
    gate fire — so the test builds the defect itself. Two of the four arms encode
    defects that were LIVE and shipped:

      V-1  a ROOT-LESS citation must fail --verify.  Twenty-three of them sat in this
           repo, fourteen in the `docs/design/m0.md` §10 disclosure table, and
           --verify reported OK because it tested only for the `silt-reviews` token.
      V-2  a line carrying the history license must be left ALONE by the REWRITE.
           It suppressed --verify but not the rewrite, so a second run turned
           "no reference to the retired `silt-reviews/` tree" into a sentence about
           the tree that is not retired. A path rewrite must never rewrite a CLAIM.
      V-3  the rewrite is idempotent — every held branch runs it before merging.
      V-4  the OVER-EXCLUSION arm. `planner/MEMORY.md` is a live store-relative
           pointer and must NOT be flagged. Without this arm, "flag every seat-shaped
           path" passes V-1 and the gate cries wolf on valid references.
    """
    import contextlib
    import io
    import tempfile

    fails = []

    def quiet(*argv):
        """Run main() with its output captured.

        The driven arms deliberately make the tool FAIL. Letting that FAIL banner
        reach the CI log inside a PASSING self-test is how a gate teaches its reader
        to ignore it (scar:a-verification-can-fail-toward-alarm).
        """
        buf = io.StringIO()
        with contextlib.redirect_stdout(buf), contextlib.redirect_stderr(buf):
            return main(list(argv))

    def check(name, cond, detail):
        if not cond:
            fails.append("%s: %s" % (name, detail))

    with tempfile.TemporaryDirectory() as td:
        # V-1 — root-less citation, no `silt-reviews` token anywhere in the file.
        p = os.path.join(td, "rootless.md")
        with open(p, "w") as fh:
            fh.write("see research/research-outcome/FORGED-CERTIFICATION-2026-01-01.md\n")
        rc = quiet("--root", td, "--verify")
        check("V-1", rc == 1,
              "a ROOT-LESS citation did NOT fail --verify — the 23-citation class is "
              "invisible again (exit %d)" % rc)

        # V-4 — a live seat directory keeps its name in the store: NOT a stale pointer.
        os.remove(p)
        p4 = os.path.join(td, "live.md")
        with open(p4, "w") as fh:
            fh.write("see planner/MEMORY.md and economist/notes.md\n")
        rc = quiet("--root", td, "--verify")
        check("V-4", rc == 0,
              "OVER-EXCLUSION: a VALID store-relative pointer (`planner/MEMORY.md`) was "
              "flagged — the closed complement is too broad and the gate cries wolf")
        os.remove(p4)

        # V-2 — the history license binds the REWRITE, not only --verify.
        licensed = ("# no reference to the retired `silt-reviews/` tree may reach a "
                    "branch (%s)\n" % HISTORY_LICENSE)
        p2 = os.path.join(td, "licensed.md")
        with open(p2, "w") as fh:
            fh.write(licensed)
        quiet("--root", td)
        after = open(p2).read()
        check("V-2", after == licensed,
              "a LICENSED claim line was rewritten: %r — the tool falsified a sentence "
              "it was told names the old tree on purpose" % after.strip()[:90])
        os.remove(p2)

        # V-3 — idempotence, over both forms at once.
        p3 = os.path.join(td, "both.md")
        with open(p3, "w") as fh:
            fh.write("silt-reviews/principle-engineer/A.md and research/B.md\n")
        quiet("--root", td)
        once = open(p3).read()
        quiet("--root", td)
        twice = open(p3).read()
        check("V-3", once == twice,
              "the rewrite is NOT idempotent: %r then %r" % (once.strip(), twice.strip()))
        check("V-3b", "silt-reviews" not in once and "principal-engineer/reviews/A.md" in once
              and "researcher/reviews/B.md" in once,
              "one pass did not rewrite BOTH forms: %r" % once.strip())

    if fails:
        print("FAIL [%s] — self-test:" % SCAR, file=sys.stderr)
        for f in fails:
            print("  " + f, file=sys.stderr)
        return 1
    print("OK [%s] — self-test: --verify CATCHES a root-less citation (V-1), does NOT "
          "flag a live store-relative one (V-4), the history license protects a claim "
          "line from the REWRITE (V-2), and the rewrite is idempotent over both forms "
          "(V-3)." % SCAR)
    return 0


def main(argv=None) -> int:
    ap = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    ap.add_argument("--root", default=None, help="tree to process (default: this git repo)")
    ap.add_argument("--verify", action="store_true",
                    help="do not write; exit 1 if any un-exempt occurrence remains")
    ap.add_argument("--self-test", action="store_true",
                    help="drive the gate on a manufactured tree; exit 1 if an arm fails")
    args = ap.parse_args(argv)

    if args.self_test:
        return self_test()

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
        # A file may carry ONLY the root-less form, which has no `silt-reviews`
        # token at all. Skipping on the token alone is exactly how the 23 stayed
        # invisible; test BOTH forms.
        if "silt-reviews" not in text and not ROOTLESS_RE.search(text):
            continue

        new, n, unmapped = rewrite_text(text)
        for u in unmapped:
            unmapped_all.setdefault(u, []).append(path)

        if args.verify:
            for i, line in enumerate(text.splitlines(), 1):
                if HISTORY_LICENSE in line:
                    continue
                if "silt-reviews" in line:
                    remaining.append("%s:%d: %s" % (
                        os.path.relpath(path, root), i, line.strip()[:140]))
                elif ROOTLESS_RE.search(line):
                    # Same defect, no token to grep for. Name the match so the
                    # failure text points at the citation and not just the line.
                    for m in ROOTLESS_RE.finditer(line):
                        remaining.append("%s:%d: ROOT-LESS `%s` — %s" % (
                            os.path.relpath(path, root), i, m.group(0)[:100],
                            line.strip()[:80]))
            continue

        if new != text:
            with open(path, "w", encoding="utf-8") as fh:
                fh.write(new)
            changed += 1
            occurrences += n

    if args.verify:
        if remaining:
            print("FAIL [%s] — %d reference(s) to the retired review tree remain "
                  "(rooted `silt-reviews/...` and/or ROOT-LESS `research/...`, "
                  "`principle-engineer/...`):" % (SCAR, len(remaining)), file=sys.stderr)
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
        print("OK [%s] — no reference to the retired review tree under %s, in EITHER "
              "form: rooted `silt-reviews/...` or ROOT-LESS `%s/...`"
              % (SCAR, root, ", ".join(sorted(ROOTLESS_MAP))))
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
