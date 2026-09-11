#!/usr/bin/env python3
"""A changed SCHEDULED workflow must show a green workflow_dispatch run.

  scar:scheduled-workflow-never-observed-green-2026-09-11

SCAR. `nightly-netem` landed inside a seven-item omnibus commit and was RED on its
very first scheduled run, the night after it merged. It stayed red for 19 of its
21 lifetime runs. Nobody had ever watched it run. A scheduled workflow is the one
kind of CI that merges WITHOUT executing: the PR that adds it goes green on the
checks it did not change, the workflow first executes hours later on a cron, and
its result lands in a tab no reviewer opens.

THE RULE. If a PR adds or changes a workflow with an `on: schedule:` trigger, it
must record a `workflow_dispatch` run of that workflow that CONCLUDED SUCCESS, and
this gate verifies that claim against the GitHub API rather than trusting the row.

  .github/scheduled-workflow-proofs.tsv
  <workflow path>\t<run id>\t<head sha>\t<YYYY-MM-DD>\t<note>

WHY THE ROW IS NOT ENOUGH. A pasted run URL is itself a claim, and a claim about a
gate decays exactly like a cited test name — the repo has a scar for that too. So
the run id is resolved through the API and four things are checked: the run exists,
it belongs to THIS workflow file, its event was `workflow_dispatch`, and its
conclusion was `success`. A row naming a red run, a run of a different workflow, or
a run that never happened fails here.

WHY IT FAILS CLOSED ON A NETWORK ERROR. If the API cannot be reached, this exits
non-zero rather than waving the change through. The gate only fires when a
scheduled workflow file actually changed, which is rare; a rare gate that
occasionally needs a re-run costs far less than a gate that silently passes the one
case it exists to catch. Pass --offline to skip verification deliberately (local
editing), never in CI.

THE PROOF SHA IS EXPECTED TO TRAIL HEAD. The dispatch necessarily runs BEFORE the
commit that records its id, so the proof's head sha is an ancestor of the PR head,
not equal to it. It is recorded and checked against the API so the row names WHICH
tree was proven — a reviewer can then see for themselves whether later commits
moved the workflow again.
"""
import json
import os
import re
import subprocess
import sys
import urllib.error
import urllib.request

PROOFS = ".github/scheduled-workflow-proofs.tsv"
WORKFLOW_DIR = ".github/workflows/"
API = "https://api.github.com"


def sh(*args):
    return subprocess.run(args, capture_output=True, text=True).stdout


def git_show(ref, path):
    p = subprocess.run(["git", "show", f"{ref}:{path}"], capture_output=True, text=True)
    return p.stdout if p.returncode == 0 else ""


def has_schedule_trigger(text):
    """True if the workflow declares an `on: schedule:` trigger.

    Deliberately crude and deliberately OVER-inclusive: a `schedule:` key at any
    indentation counts. Over-inclusion costs one extra proof row; under-inclusion
    is the whole scar.
    """
    return re.search(r"^\s*schedule:\s*$", text, re.M) is not None


def parse_proofs(text):
    rows = {}
    for line in text.splitlines():
        line = line.strip()
        if not line or line.startswith("#"):
            continue
        parts = line.split("\t")
        if len(parts) < 4:
            continue
        rows[parts[0].strip()] = {
            "run_id": parts[1].strip(),
            "head_sha": parts[2].strip(),
            "date": parts[3].strip(),
        }
    return rows


def repo_slug():
    slug = os.environ.get("GITHUB_REPOSITORY")
    if slug:
        return slug
    url = sh("git", "remote", "get-url", "origin").strip()
    m = re.search(r"[:/]([^/:]+/[^/]+?)(?:\.git)?$", url)
    return m.group(1) if m else ""


def fetch_run(slug, run_id):
    req = urllib.request.Request(f"{API}/repos/{slug}/actions/runs/{run_id}")
    req.add_header("Accept", "application/vnd.github+json")
    token = os.environ.get("GITHUB_TOKEN")
    if token:
        req.add_header("Authorization", f"Bearer {token}")
    with urllib.request.urlopen(req, timeout=30) as r:
        return json.load(r)


def main():
    offline = "--offline" in sys.argv

    # CI passes these as `${{ github.event.pull_request.* }}`, which expands to the
    # EMPTY STRING on a push event — so test for truthiness, not for presence.
    base = os.environ.get("BASE_SHA") or ""
    head = os.environ.get("HEAD_SHA") or "HEAD"
    if not base:
        base = sh("git", "merge-base", "origin/main", "HEAD").strip() or "origin/main"

    changed = [f for f in sh("git", "diff", "--name-only", base, head).split() if f]
    scheduled = []
    for f in changed:
        if not (f.startswith(WORKFLOW_DIR) and f.endswith((".yml", ".yaml"))):
            continue
        text = git_show(head, f)
        if not text:  # deleted — nothing left to prove
            continue
        if has_schedule_trigger(text):
            scheduled.append(f)

    if not scheduled:
        print("OK — no scheduled workflow changed in this diff")
        return 0

    print("scheduled workflows changed in this diff:")
    for f in scheduled:
        print(f"  {f}")

    now = parse_proofs(git_show(head, PROOFS))
    before = parse_proofs(git_show(base, PROOFS))
    slug = repo_slug()
    failures = []

    for wf in scheduled:
        row = now.get(wf)
        if not row:
            failures.append(
                f"{wf} changed but {PROOFS} has no proof row for it.\n"
                f"    Dispatch it on your branch (gh workflow run {os.path.basename(wf)} --ref <branch>),\n"
                f"    wait for it to go GREEN, then add:\n"
                f"      {wf}\t<run id>\t<head sha>\t<YYYY-MM-DD>\t<note>"
            )
            continue
        if before.get(wf, {}).get("run_id") == row["run_id"]:
            failures.append(
                f"{wf} changed but its proof row still names run {row['run_id']} — the run that\n"
                f"    certified the PREVIOUS version. Dispatch the CHANGED workflow and record that run."
            )
            continue
        if offline:
            print(f"  {wf}: row present (run {row['run_id']}) — NOT verified (--offline)")
            continue
        try:
            run = fetch_run(slug, row["run_id"])
        except urllib.error.HTTPError as e:
            failures.append(f"{wf}: run {row['run_id']} could not be fetched (HTTP {e.code}). Gate fails closed.")
            continue
        except Exception as e:  # network, DNS, timeout
            failures.append(f"{wf}: could not reach the GitHub API to verify run {row['run_id']} ({e}). Gate fails closed.")
            continue
        problems = []
        if run.get("path") != wf:
            problems.append(f"belongs to {run.get('path')!r}, not {wf!r}")
        if run.get("event") != "workflow_dispatch":
            problems.append(f"event was {run.get('event')!r}, not 'workflow_dispatch'")
        if run.get("conclusion") != "success":
            problems.append(f"conclusion was {run.get('conclusion')!r}, not 'success'")
        if run.get("head_sha") != row["head_sha"]:
            problems.append(f"ran on {str(run.get('head_sha'))[:12]}, but the row claims {row['head_sha'][:12]}")
        if problems:
            failures.append(f"{wf}: run {row['run_id']} " + "; ".join(problems))
        else:
            print(f"  {wf}: VERIFIED — run {row['run_id']} on {row['head_sha'][:12]} "
                  f"was a green workflow_dispatch ({run.get('html_url')})")

    if failures:
        print()
        for f in failures:
            print(f"::error::a changed scheduled workflow has no verified green dispatch run: {f}")
        print()
        print("A scheduled workflow merges WITHOUT ever executing. nightly-netem was red on its")
        print("first cron run and 19 of its first 21 — it had never been watched. Watch it.")
        return 1

    print("OK — every changed scheduled workflow has a verified green workflow_dispatch run")
    return 0


if __name__ == "__main__":
    sys.exit(main())
