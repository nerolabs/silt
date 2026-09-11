#!/usr/bin/env python3
"""A green check that contains ZERO EXECUTION.

  scar:short-run-is-zero-execution-2026-09-10

SCAR, COUNT 2. A test that calls `t.Skip` under `testing.Short()` runs nowhere if every
merge-gating invocation of its package passes `-short`. `go test` reports the skip as a
pass, the job goes green, and the artifact ships with a check mark standing for nothing:

    --- SKIP: TestSlashesBytesCapWorstCaseCost
    --- SKIP: TestReconcileMemoryBounded_563
    PASS   SHORT_EXIT=0

  Instance 1 was a SEAT choosing `-short` for a local run — human error, correctable by
  telling the human. It recorded the owed gate in the same breath: *"CI must fail any
  artifact whose only suite evidence is a `-short` run, or print the skipped-tier count."*
  Nobody encoded it.

  Instance 2 is `.github/workflows/ci.yml` ITSELF passing `-short` on every invocation
  but two. That is not human error. It is permanent, it recurs on every push, and it is
  why instance 1's owed gate is now this file.

THE PREDICATE, AND WHY THE OBVIOUS ONE IS WRONG
  The naive predicate — "is this test's NAME mentioned in a workflow?" — reports 51 dark
  tests in this repo, and 36 of them are false. The whole `e2e` package self-skips under
  `-short` and is named nowhere, because the `e2e` job runs it BY PACKAGE and without
  `-short`. Those tests execute on every PR.

  So the unit is the INVOCATION, not the name:

      dark  ==  the test self-skips under -short
                AND every merge-gating `go test` that covers it passes -short

  "covers" means the invocation's package arguments include the test's package (`./...`
  includes everything) AND its `-run` filter, if any, matches the test's name.

WHY "MERGE-GATING" AND NOT "ANY WORKFLOW"
  `release.yml` runs `go test ./...` with NO `-short`, so a predicate over all workflows
  reports zero dark tests and this whole file is decoration. But release.yml triggers on
  a version TAG. It cannot gate a pull request, and it runs after the decision to ship
  has already been made. A workflow is merge-gating here iff its `on:` block carries a
  `pull_request` trigger or a `push` restricted to branches. That is `ci.yml` alone
  today; `fuzz.yml` and `nightly-netem.yml` are schedule/dispatch.

WHAT THIS GATE DOES *NOT* CATCH, STATED PLAINLY
  A test that DEGRADES under `-short` instead of skipping — smaller N, fewer attempts —
  still executes and still asserts, so it is not this scar and is not reported. Three
  tests in this repo do that (adapters/diskissuer, core/credit, sim). The evasion is
  real: converting a `t.Skip` into a smaller fixture will silence this gate. It is also
  a legitimate fix, because the result actually runs. Reviewers, not this script, judge
  whether the shrunk fixture still covers the property.

THE ALLOWLIST (scripts/short_dark_tests_allowlist.txt)
  A dark test is permitted only with a WRITTEN REASON. An unreasoned row FAILS — an
  allowlist rationale is itself a claim (scar:allowlist-rationale-is-itself-a-claim).
  Two mechanical companions keep the reasons from rotting:

    - Any `Test...` name inside a reason must RESOLVE to a real test that is itself not
      dark. "The always-on gate is TestFooStructural" is a claim about another test; if
      that test is renamed away or goes dark itself, the reason is false and this fails.
    - A row for a test that is no longer dark FAILS as STALE. When a test gets wired
      into CI, its row must come out. The complement is closed in both directions, so
      the allowlist cannot quietly accumulate rows that excuse nothing.
"""

import argparse
import re
import shlex
import sys
from pathlib import Path

from repo_walk import repo_files

ROOT = Path(__file__).resolve().parent.parent
SCAR_ID = "scar:short-run-is-zero-execution-2026-09-10"

ALLOWLIST = ROOT / "scripts" / "short_dark_tests_allowlist.txt"
WORKFLOWS = ROOT / ".github" / "workflows"

# Names pruned by the walk, on top of the nested-checkout rule `repo_files` applies
# always: an agent worktree under `.claude/worktrees/` is a different branch's tree, and
# judging its tests against THIS branch's workflows and allowlist is meaningless.
SKIP_DIRS = {".git", "dist", "node_modules", "website", "testdata", "vendor"}

# A written reason is a sentence, not a shrug. Same bar as check_reachability.py.
MIN_REASON_CHARS = 40
NULL_REASONS = {"n/a", "na", "none", "tbd", "todo", "-", "see above", "obvious", "?",
                "measurement", "measurement only", "slow"}

TEST_FUNC = re.compile(r"^func (Test\w+)\(", re.M)
TEST_NAME = re.compile(r"\bTest[A-Za-z0-9_]+\b")
SHORT_CALL = "testing.Short()"


# --------------------------------------------------------------------------- tests

def find_short_gated_tests(root):
    """Every test whose body branches on testing.Short(), classified skip vs degrade.

    Returns a list of dicts. A `skip` is one where a `t.Skip*` call appears inside the
    testing.Short() branch; anything else is a `degrade` (smaller N, fewer attempts).
    """
    out = []
    for path in sorted(repo_files(root, SKIP_DIRS, "_test.go")):
        lines = path.read_text(encoding="utf-8", errors="replace").split("\n")
        spans, cur, start = [], None, 0
        for i, line in enumerate(lines):
            m = TEST_FUNC.match(line)
            if m:
                if cur:
                    spans.append((cur, start, i))
                cur, start = m.group(1), i
            elif line.startswith("func ") and cur:
                spans.append((cur, start, i))
                cur = None
        if cur:
            spans.append((cur, start, len(lines)))

        for name, a, b in spans:
            body = "\n".join(lines[a:b])
            if SHORT_CALL not in body:
                continue
            kind = "degrade"
            for m in re.finditer(re.escape(SHORT_CALL), body):
                # The guarded block ends at the first closing brace at func-body indent.
                block = body[m.end():].split("\n\t}")[0]
                if re.search(r"\bt\.Skip", block):
                    kind = "skip"
                    break
            out.append({
                "pkg": path.parent.relative_to(root).as_posix(),
                "name": name,
                "kind": kind,
                "where": f"{path.relative_to(root).as_posix()}:{a + 1}",
            })
    return out


# ----------------------------------------------------------------------- workflows

def is_merge_gating(text):
    """True iff the workflow's `on:` block gates a merge: a pull_request trigger, or a
    push restricted to BRANCHES. A tag-only push (release.yml) is not merge-gating: it
    runs after the decision to ship, and never on a pull request."""
    lines = text.split("\n")
    block, inside = [], False
    for line in lines:
        if re.match(r"^(on|'on'|\"on\"|true):\s*$", line) or re.match(r"^on:\s*\S", line):
            inside = True
            continue
        if inside:
            if line.strip() and not line[0].isspace():
                break
            block.append(line)
    body = "\n".join(block)
    if re.search(r"^\s+pull_request:", body, re.M):
        return True
    push = re.search(r"^\s+push:\s*$(.*?)(?=^\s{2}\S|\Z)", body, re.M | re.S)
    return bool(push and re.search(r"^\s+branches:", push.group(1), re.M))


def run_block_lines(text):
    """Yield (lineno, line) for SHELL lines only — the body of every `run:` key.

    A step's `name:` is a LABEL, not a command, and this repo writes labels like
    `- name: go test ./e2e`. Scanning raw lines would read that label as an unshortened
    invocation and credit coverage to a package no command actually runs unshortened —
    the same class of defect this gate exists to catch, one level up.
    """
    lines = text.split("\n")
    i = 0
    while i < len(lines):
        line = lines[i]
        m = re.match(r"^(\s*)(?:-\s+)?run:\s*(.*)$", line)
        if not m:
            i += 1
            continue
        indent, rest = len(m.group(1)), m.group(2).strip()
        if rest and rest not in ("|", ">", "|-", ">-", "|+", ">+"):
            yield i + 1, rest          # single-line form: `run: go test ./...`
            i += 1
            continue
        i += 1
        while i < len(lines):
            body = lines[i]
            if body.strip() and (len(body) - len(body.lstrip())) <= indent:
                break
            yield i + 1, body
            i += 1


def go_test_invocations(text):
    """Every `go test ...` command in a workflow's shell, parsed into (short, pkgs, run)."""
    invocations = []
    for lineno, line in run_block_lines(text.replace("\\\n", " ")):
        stripped = line.strip()
        if stripped.startswith("#") or "go test" not in line:
            continue
        cmd = line[line.index("go test"):]
        # Trim the shell wrapping we actually use: `out=$(go test ... 2>&1)`.
        cmd = re.sub(r"\s*2>&1.*$", "", cmd).rstrip().rstrip(")")
        try:
            tokens = shlex.split(cmd)
        except ValueError:
            continue
        pkgs, run, short = [], None, False
        i = 0
        while i < len(tokens):
            t = tokens[i]
            if t in ("-short", "--short", "-test.short"):
                short = True
            elif t.startswith("-short="):
                short = t.split("=", 1)[1] not in ("0", "false")
            elif t in ("-run", "--run") and i + 1 < len(tokens):
                run = tokens[i + 1]
                i += 1
            elif t.startswith("-run="):
                run = t.split("=", 1)[1]
            elif t.startswith("./"):
                pkgs.append(t)
            i += 1
        if pkgs:
            invocations.append({"short": short, "pkgs": pkgs, "run": run, "line": lineno})
    return invocations


def invocation_covers(inv, pkg, name):
    """Does this `go test` actually execute `name` in `pkg`?"""
    hit = False
    for arg in inv["pkgs"]:
        target = arg[2:].rstrip("/")
        if target == "..." or target == "":
            hit = True
        elif target.endswith("/..."):
            base = target[:-4].rstrip("/")
            hit = pkg == base or pkg.startswith(base + "/")
        else:
            hit = pkg == target
        if hit:
            break
    if not hit:
        return False
    if inv["run"]:
        # `go test -run` matches each slash-separated part unanchored. The top-level
        # part is all that matters here: these are whole test functions.
        top = inv["run"].split("/")[0]
        try:
            if not re.search(top, name):
                return False
        except re.error:
            return False
    return True


def dark_tests(tests, invocations):
    """A test is dark iff it self-skips and no non-`-short` merge-gating run covers it."""
    dark = []
    for t in tests:
        if t["kind"] != "skip":
            continue
        if any(not inv["short"] and invocation_covers(inv, t["pkg"], t["name"])
               for inv in invocations):
            continue
        dark.append(t)
    return dark


# ----------------------------------------------------------------------- allowlist

def parse_allowlist(path):
    """`<pkg> <TestName>  # <written reason>`, one per line. Returns (rows, errors)."""
    rows, errors = {}, []
    if not path.exists():
        return rows, [f"{path.name}: missing — the allowlist is part of the gate"]
    for n, raw in enumerate(path.read_text(encoding="utf-8").splitlines(), 1):
        line = raw.strip()
        if not line or line.startswith("#"):
            continue
        where = f"{path.name}:{n}"
        head, sep, reason = line.partition("#")
        fields = head.split()
        if len(fields) != 2:
            errors.append(f"{where}: expected `<pkg> <TestName>  # <reason>`, got {raw.strip()!r}")
            continue
        pkg, name = fields
        reason = reason.strip()
        if not sep or reason.lower().rstrip(".") in NULL_REASONS or len(reason) < MIN_REASON_CHARS:
            errors.append(
                f"{where}: {pkg} {name} carries no written reason ({reason!r}). An "
                f"unreasoned row FAILS — an allowlist rationale is itself a claim. Say "
                f"what runs instead, or why nothing needs to.")
            continue
        if (pkg, name) in rows:
            errors.append(f"{where}: duplicate row for {pkg} {name}")
            continue
        rows[(pkg, name)] = {"reason": reason, "where": where}
    return rows, errors


def check_reason_companions(rows, tests, dark_keys):
    """Every Test name a reason cites must resolve, and must not itself be dark."""
    known = {(t["pkg"], t["name"]) for t in tests}
    all_names = set()
    for path in repo_files(ROOT, SKIP_DIRS, "_test.go"):
        for m in TEST_FUNC.finditer(path.read_text(encoding="utf-8", errors="replace")):
            all_names.add(m.group(1))
    dark_names = {name for _, name in dark_keys}

    errors = []
    for (pkg, name), row in sorted(rows.items()):
        for cited in TEST_NAME.findall(row["reason"]):
            if cited == name:
                continue
            if cited not in all_names:
                errors.append(
                    f"{row['where']}: the reason for {name} cites `{cited}`, which is "
                    f"not a test in this repo. A reason that names a test is making a "
                    f"claim about it; the claim decayed.")
            elif cited in dark_names:
                errors.append(
                    f"{row['where']}: the reason for {name} points at `{cited}` as the "
                    f"cover, but `{cited}` is ITSELF dark. The cover does not run either.")
    _ = known
    return errors


# -------------------------------------------------------------------------- report

def report(errors, dark_count):
    print(f"FAIL [{SCAR_ID}] — a test executes in no merge-gating CI job.\n\n"
          "  A `t.Skip` under `testing.Short()` is reported by `go test` as a PASS. If\n"
          "  every merge-gating invocation of the package passes `-short`, the check\n"
          "  mark on the artifact stands for zero execution of that test.\n",
          file=sys.stderr)
    for e in errors:
        print(f"  {e}", file=sys.stderr)
    print(f"\n{len(errors)} problem(s); {dark_count} dark test(s) found.", file=sys.stderr)


def main():
    ap = argparse.ArgumentParser(description=__doc__.split("\n")[0])
    ap.add_argument("--allowlist", default=str(ALLOWLIST))
    ap.add_argument("--workflows", default=str(WORKFLOWS))
    ap.add_argument("--list", action="store_true",
                    help="print the classification and exit 0 (derivation, not a gate)")
    args = ap.parse_args()

    tests = find_short_gated_tests(ROOT)

    wf_dir = Path(args.workflows)
    gating, invocations = [], []
    for wf in sorted(wf_dir.glob("*.yml")) + sorted(wf_dir.glob("*.yaml")):
        text = wf.read_text(encoding="utf-8")
        if not is_merge_gating(text):
            continue
        gating.append(wf.name)
        invocations += go_test_invocations(text)

    if not gating:
        print(f"FAIL [{SCAR_ID}] — no merge-gating workflow found under {wf_dir}. "
              f"Every test is dark and this gate cannot say anything useful.", file=sys.stderr)
        return 1

    dark = dark_tests(tests, invocations)
    dark_keys = {(t["pkg"], t["name"]) for t in dark}

    if args.list:
        for t in sorted(tests, key=lambda x: (x["kind"], x["pkg"], x["name"])):
            state = "DARK" if (t["pkg"], t["name"]) in dark_keys else "runs"
            print(f"{t['kind']:8} {state:5} {t['pkg']:24} {t['name']}")
        print(f"\nmerge-gating workflows: {', '.join(gating)}")
        print(f"go test invocations parsed: {len(invocations)} "
              f"({sum(1 for i in invocations if not i['short'])} without -short)")
        print(f"short-gated tests: {len(tests)}; dark: {len(dark)}")
        return 0

    rows, errors = parse_allowlist(Path(args.allowlist))

    for t in sorted(dark, key=lambda x: (x["pkg"], x["name"])):
        if (t["pkg"], t["name"]) not in rows:
            errors.append(
                f"{t['where']}: {t['pkg']} {t['name']} skips under -short and NO "
                f"merge-gating `go test` runs it without -short. Run it in a required "
                f"job, or baseline it in {Path(args.allowlist).name} with a written reason.")

    for (pkg, name), row in sorted(rows.items()):
        if (pkg, name) not in dark_keys:
            errors.append(
                f"{row['where']}: STALE row — {pkg} {name} is not dark (it runs in CI, "
                f"or no longer skips under -short, or no longer exists). Remove the row; "
                f"an allowlist that excuses nothing is a place for rows to hide.")

    errors += check_reason_companions(rows, tests, dark_keys)

    if errors:
        report(errors, len(dark))
        return 1

    degraded = sum(1 for t in tests if t["kind"] == "degrade")
    print(f"OK [{SCAR_ID}] — {len(tests)} test(s) branch on -short across "
          f"{', '.join(gating)}: {len(tests) - len(dark) - degraded} run unshortened in a "
          f"merge-gating job, {degraded} degrade rather than skip, {len(dark)} are dark "
          f"and each carries a written reason.")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
