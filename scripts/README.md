# scripts/

Dependency-free (stdlib-only) Python. Everything here runs on a bare `python3` with
no install step, so the same command works locally and in CI.

These are **repo lints**. Each one exists because a green check can be a green check
for the wrong reason — a gate that reads source text and promises runtime behaviour, a
test that skips in every job that runs it, a cited test that does not exist, a
mechanism the linker dropped out of the shipped binary. Each lint makes one of those
loud.

## The shared walk

Every check that walks the tree does it through `repo_walk.repo_files`, which prunes
any directory below the root holding a `.git` entry. That is what makes a directory a
DIFFERENT checkout — a `.git` file for a `git worktree`, a `.git` directory for a
nested clone — so a lint run on a machine with other checkouts under the root judges
this tree only. Use it for any new check; `rglob` from the root reopens the hole.

## The checks

| Script | Fails the build when |
| --- | --- |
| `check_cited_tests.py` | a Go comment, string or doc cites a `TestXxx` with no `func TestXxx(` anywhere; **or** a `path.go:NNN` coordinate no longer points at the symbol cited beside it |
| `check_conflict_markers.py` | an unresolved git merge-conflict marker reaches a branch |
| `check_reachability.py` | a lane the release notes claim has no client entry point in the linked `./cmd/silt` binary, and the posture file does not say the lane **cannot be exercised** |
| `check_scheduled_workflow_proof.py` | a changed `on: schedule:` workflow has no `workflow_dispatch` run that concluded success, verified against the GitHub API |
| `check_short_dark_tests.py` | a test self-skips under `-short` and every merge-gating `go test` that covers it passes `-short` — a green check containing zero execution |
| `check_source_gates.py` | a test that reads the project's own `.go` source does not say so in its failure text, or does not name its runtime cover |
| `check_tenet_qualifiers.py` | `docs/TENETS.md`'s Sybil-composition claim drops its design-target qualifier and reads as an achieved property |
| `repo_walk.py --self-test` | the shared file walk descends into a nested checkout, or excludes a directory of this tree that merely looks like one |

All of these run in CI. Each exits `0` on pass and `1` on failure, and prints its
findings to stderr.

```sh
for c in cited_tests conflict_markers reachability scheduled_workflow_proof \
         short_dark_tests source_gates tenet_qualifiers; do
  python3 "scripts/check_$c.py" || echo "FAILED: $c"
done
python3 scripts/repo_walk.py --self-test || echo "FAILED: repo_walk"
```

## Not wired into CI

| Script | Fails when |
| --- | --- |
| `check_adversary_shape.py` | a comment asserts an adversary *cannot* do something, and no fixture grants that adversary the capability the defence assumes it lacks |

It is not referenced by `.github/workflows/ci.yml`, so nothing runs it
automatically. It runs in ratchet mode: a fixed allow-list of pre-existing uncovered
claims is grandfathered, and a new uncovered claim fails. Growing that list takes an
edit to `RATCHET` and `RATCHET_COUNT` together, in a diff a reader sees.

## Data files

| File | Read by |
| --- | --- |
| `cited_tests_allowlist.txt` | `check_cited_tests.py` — deliberately historical citations, each with a reason |
| `short_dark_tests_allowlist.txt` | `check_short_dark_tests.py` — dark tests permitted, each with a written reason |
| `reachability_lanes.txt` | `check_reachability.py` — lane → client entry symbol |
| `reachability_postures.md` | `check_reachability.py` — the posture of each lane whose symbol is absent |

An allow-list reason is itself a claim: a `Test…` name inside one must resolve to a
real test that is not itself dark, and a row for something no longer in the state it
excuses fails as stale. The complement is closed in both directions, so a list cannot
quietly accumulate rows that excuse nothing.
