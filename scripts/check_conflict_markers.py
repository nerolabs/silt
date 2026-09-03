#!/usr/bin/env python3
"""Repo lint: no unresolved git merge-conflict marker may reach a branch.

SCAR (scar:conflict-markers-shipped-in-ci-yml-2026-09-03):
  A rebase resolution missed `.github/workflows/ci.yml`. The three conflict markers sat
  in the tree across a full review round. The file is YAML, so the markers did not just
  look untidy — the whole `docs` job stopped parsing, and with it BOTH
  `scripts/check_cited_tests.py` and `scripts/check_source_gates.py`. Two shipped
  documents then described the source-gate lint as "in CI" when nothing in the docs job
  ran at all. Nothing was red: the job did not fail, it did not exist. That is the
  failure mode this lint exists for — a silently absent gate, not a noisy one.

THE RULE:
  No tracked text file may contain a line beginning with a git conflict marker.
    - a line starting with seven `<` followed by a space (the "ours" header)
    - a line starting with seven `>` followed by a space (the "theirs" header)
    - a line that is exactly seven `=` — flagged ONLY in a file that also carries one of
      the two headers, because a bare row of `=` is legal Markdown (a setext heading
      underline) and a real conflict always carries all three

  Excluded: `.git/` (packed objects and MERGE_MSG legitimately contain markers) and
  `website/` (generated from the Markdown sources, which are themselves linted).

Dependency-free (stdlib only). Run: python3 scripts/check_conflict_markers.py
"""
import re
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent

SCAR_ID = "scar:conflict-markers-shipped-in-ci-yml-2026-09-03"

# Built from parts so that no line of this file is itself a marker line.
_LT = "<" * 7
_GT = ">" * 7
_EQ = "=" * 7

HEAD_RE = re.compile(rf"^(?:{re.escape(_LT)}|{re.escape(_GT)})(?: |$)")
SEP_RE = re.compile(rf"^{re.escape(_EQ)}$")

SKIP_DIRS = {".git", "website"}


def text_lines(path: Path):
    """Return the file's lines, or None if it is not decodable text."""
    try:
        return path.read_text(encoding="utf-8").splitlines()
    except (UnicodeDecodeError, OSError):
        return None


def main() -> int:
    scanned = 0
    failures = []  # (rel, lineno, line)

    for path in sorted(ROOT.rglob("*")):
        if not path.is_file() or path.is_symlink():
            continue
        rel = path.relative_to(ROOT)
        if SKIP_DIRS & set(rel.parts):
            continue
        lines = text_lines(path)
        if lines is None:
            continue
        scanned += 1

        heads = [(i + 1, ln) for i, ln in enumerate(lines) if HEAD_RE.match(ln)]
        if not heads:
            continue
        seps = [(i + 1, ln) for i, ln in enumerate(lines) if SEP_RE.match(ln)]
        for lineno, ln in sorted(heads + seps):
            failures.append((str(rel), lineno, ln[:72]))

    if failures:
        print(
            f"FAIL [{SCAR_ID}] — unresolved merge-conflict marker(s) in {scanned} scanned files:",
            file=sys.stderr,
        )
        for rel, lineno, ln in failures:
            print(f"  {rel}:{lineno}  {ln}", file=sys.stderr)
        print(
            "\nFix: finish the merge/rebase resolution. If the two sides are both wanted,\n"
            "the resolution is the UNION, not a choice — that is how the ci.yml scar was\n"
            "created (one lint step silently replaced another).\n",
            file=sys.stderr,
        )
        return 1

    print(f"OK [{SCAR_ID}] — {scanned} text files scanned; no conflict markers.")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
