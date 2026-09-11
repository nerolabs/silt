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

AND THE OTHER HALF OF THE SAME EXPOSURE (scar:agent-memory-store-ten-days-unpushed-2026-09-11):
  The symlink rule keeps memory landing IN the shared store. It says nothing about
  whether the store has an off-box copy. On 2026-09-11 it did not: the store's last
  commit was ten days old, which is why five of the leaked tester files were
  unrecoverable rather than merely leaked. `--autosave` closes that half, from the same
  triggers, for the same reason — this runs where a seat is already looking. See the
  AUTOSAVE block below for the cadence and for why it is not a cron job.

Dependency-free (stdlib only).
Run: python3 scripts/check_agent_memory_link.py [--hook|--ci|--autosave|--self-test] [--quiet]
  --hook       read a Claude Code hook payload on stdin; block (exit 2) a memory write
               into an unlinked path; autosave the store on SessionStart/Stop/
               SubagentStop; stay silent (exit 0) on every unrelated tool call
  --ci         assert no live memory is tracked by git (#636/#638); ignores the symlink
  --autosave   commit and push the shared store now, by hand
  --self-test  drive autosave against a manufactured store + remote in a temp directory
  --quiet      suppress the OK line on success
"""
import json
import os
import subprocess
import sys
import time
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

    if event in ("Stop", "SubagentStop"):
        # A turn (or a seat) has finished. Bank whatever it wrote. Never blocks: autosave
        # returns 0 by contract, and exit 0 on Stop is "carry on".
        return autosave(STORE, event)

    if event == "SessionStart":
        root = Path(payload.get("cwd") or os.environ.get("CLAUDE_PROJECT_DIR") or ROOT)
        status, lines = evaluate(root / MEMORY_REL)
        if status != OK:
            # SessionStart cannot block. stdout becomes context the seat reads, which is
            # exactly what is wanted: it learns the remedy before it writes anything.
            print(f"[{SCAR_ID}] agent memory is NOT wired in this checkout — " + lines[0])
            for ln in lines[1:]:
                print(ln)
            print("\n" + remedy(root))
        # Bank what the PREVIOUS session left behind, before this one can disturb it.
        # This runs whatever the symlink verdict was: the store is a different object
        # from this checkout's link to it, and an unwired checkout is the case where
        # the off-box copy matters most.
        return autosave(STORE, "SessionStart")

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


# ---------------------------------------------------------------------------
# AUTOSAVE — scar:agent-memory-store-ten-days-unpushed-2026-09-11
# ---------------------------------------------------------------------------
#
# THE DEFECT
#   The shared store is its own git repo with a remote, and committing it was a purely
#   manual act. Measured 2026-09-11: the last commit was `30210da`, 2026-09-01. TEN DAYS.
#   Nothing was red for any of them, because nothing checks. On 2026-09-11 a worktree
#   leak destroyed live memory; the off-box copy that should have recovered it was ten
#   days behind, and five tester scar files were lost for good.
#
# WHY NOT A CRON JOB
#   The class being fixed is "nothing went red for ten days". A scheduled job that stops
#   firing reproduces that class exactly — it is the same silent absence wearing a
#   different hat. This runs where a seat is already looking, so its own failure is
#   printed into the session that depends on it.
#
# THE CADENCE, AND WHY IT IS TRIGGERED AND NOT TIMED
#   Triggers, all wired in the TRACKED `.claude/settings.json` (so they reach worktrees):
#
#     SessionStart   bank whatever the previous session left behind, before this one
#                    can disturb it.
#     Stop           the main agent has finished a turn.
#     SubagentStop   a SEAT has finished. The seats ARE subagents, so this is the
#                    event that actually follows a memory write on this harness.
#
#   There is no time threshold and no minimum interval, on purpose. A floor ("at most
#   once per N minutes") reopens the hole at the edge: a write one minute after a commit,
#   followed by the session ending, is never banked at all. Without a floor the property
#   is flat and statable — EVERY memory change is committed by the end of the turn that
#   made it — and the cost is bounded by the fact that `git add -A` with no change
#   commits nothing and pushes nothing. Autosave on an unchanged tree is a no-op.
#
# THE HISTORY STAYS NAVIGABLE
#   An autosave is a byte backup, not a curated recovery point. Every one of them carries
#   the `autosave:` subject prefix and no deliberate safe point does, so:
#
#       git log --invert-grep --grep='^autosave:'      # the human-marked points only
#
# A COMMIT WHOSE INDEXES DANGLE IS NOT A RECOVERY POINT
#   ...but it is still bytes, and bytes beat purity. Integrity is checked BEFORE the
#   commit and the commit happens either way; a degraded tree is LABELLED `DEGRADED` in
#   the subject line so nobody mistakes it for a clean point later.
#
# FAIL SOFT, ALWAYS
#   Every path here returns 0. An unreachable remote commits locally and says so. A
#   memory guard that blocked a seat because GitHub was down would be worse than the
#   problem it fixes.

AUTOSAVE_SCAR_ID = "scar:agent-memory-store-ten-days-unpushed-2026-09-11"

AUTOSAVE_PREFIX = "autosave:"

# A seat's memory index. Both link directions are checked against it.
INDEX_NAME = "MEMORY.md"

# The review record — certifications, rulings, red-team findings, field-run reports —
# moved under `<seat>/reviews/` on 2026-09-11 (owner ratified). It is NOT memory and is
# NOT reached index-first: every one of those documents is cited by absolute path from a
# certification, a ruling, or the ROADMAP residual register. Counting all 641 of them as
# ORPHANs would label every autosave commit DEGRADED from that day on, and a health
# signal that is always red is a signal nobody reads.
#
# The exclusion is ONE-DIRECTIONAL and deliberately so. An index entry that points INTO
# `reviews/` and misses is still DANGLING: the index lying is a defect wherever it points.
# Only the orphan direction is relaxed, because only the orphan direction assumes the
# index is the way in.
REVIEW_SUBTREE = "reviews"

# Held across the commit so two seats stopping at the same instant do not race git's own
# index.lock. Stale after this many seconds, so a killed process cannot wedge autosave.
LOCK_STALE_SECONDS = 120

PUSH_TIMEOUT_SECONDS = 25

_MD_LINK = None  # compiled lazily; `re` is only needed on the autosave path


def _git(store, *args, timeout=30):
    """Run one git command in the store. Returns (returncode, stdout+stderr)."""
    try:
        p = subprocess.run(
            ["git", "-C", str(store), *args],
            capture_output=True,
            text=True,
            timeout=timeout,
        )
        return p.returncode, (p.stdout or "") + (p.stderr or "")
    except subprocess.TimeoutExpired:
        return 124, f"timed out after {timeout}s"
    except OSError as exc:
        return 127, str(exc)


def _exists_cased(base, target):
    """Case-EXACT existence test for an index link, relative to `base`.

    `Path.exists()` is case-INSENSITIVE on APFS (measured: `(seat / "Real.md").exists()`
    is True when the file on disk is `real.md`), which is the volume every seat and the
    hook actually run on. So a link differing from its file only in case read as PRESENT
    and never dangled — while the same typo made the real file an ORPHAN, because the
    orphan direction compares link STRINGS. One typo, two wrong answers, and the half a
    reader trusts said clean. `os.listdir()` membership is case-exact on every volume.

    Returns False on anything it cannot resolve, including a link that climbs out of the
    seat: this is a reachability test, and unreachable is the answer in both cases.
    """
    cur = base
    for part in target.split("/"):
        if part in ("", "."):
            continue
        if part == "..":
            return False
        try:
            if part not in os.listdir(cur):
                return False
        except OSError:
            return False
        cur = cur / part
    return True


def index_integrity(store):
    """Resolve every seat index's links BOTH ways. Returns (indexes, dangling, orphans).

    A commit whose indexes dangle is not a recovery point, so the counts go in the
    commit message. Two directions, because only one of them is obvious:

      DANGLING  an index entry points at a file that is not there. The index lies.
      ORPHAN    a memory file no index links to. The file is there and unreachable,
                which is the same as lost for any seat that reads the index first.
    """
    global _MD_LINK
    if _MD_LINK is None:
        import re

        _MD_LINK = re.compile(r"\]\(([^)\s]+)\)")

    indexes, dangling, orphans = 0, [], []
    try:
        seats = sorted(p for p in store.iterdir() if p.is_dir() and not p.name.startswith("."))
    except OSError:
        return 0, [], []

    for seat in seats:
        index = seat / INDEX_NAME
        if not index.is_file():
            continue
        indexes += 1
        try:
            text = index.read_text(encoding="utf-8", errors="replace")
        except OSError:
            continue

        linked = set()
        for target in _MD_LINK.findall(text):
            if "://" in target or target.startswith("#"):
                continue
            target = target.split("#", 1)[0]
            if not target:
                continue
            linked.add(target)
            if not _exists_cased(seat, target):
                dangling.append(f"{seat.name}/{INDEX_NAME} -> {target}")

        try:
            present = sorted(p for p in seat.rglob("*.md") if p.is_file())
        except OSError:
            present = []
        for p in present:
            rel = p.relative_to(seat).as_posix()
            if rel == INDEX_NAME or rel in linked:
                continue
            if rel.startswith(REVIEW_SUBTREE + "/"):
                continue
            orphans.append(f"{seat.name}/{rel}")

    return indexes, dangling, orphans


def _acquire_lock(store):
    """O_EXCL lock in the store's `.git`. Returns the path, or None if someone else has it."""
    lock = store / ".git" / "silt-autosave.lock"
    try:
        age = time.time() - lock.stat().st_mtime
        if age > LOCK_STALE_SECONDS:
            lock.unlink()
    except OSError:
        pass
    try:
        fd = os.open(str(lock), os.O_CREAT | os.O_EXCL | os.O_WRONLY, 0o644)
    except OSError:
        return None
    os.write(fd, f"{os.getpid()}\n".encode())
    os.close(fd)
    return lock


def autosave(store, trigger, push=True, out=sys.stderr):
    """Commit (and try to push) the shared store. ALWAYS returns 0.

    Returns 0 unconditionally by contract: this runs inside a hook, and no state of the
    memory store or of the network is a reason to interrupt a seat.
    """
    store = Path(store)
    if not (store / ".git").exists():
        return 0  # Not a repo (or absent). The symlink guard owns that problem, not this.

    rc, dirty = _git(store, "status", "--porcelain")
    if rc != 0:
        print(f"[{AUTOSAVE_SCAR_ID}] autosave skipped: git status failed in {store}: "
              f"{dirty.strip()}", file=out)
        return 0

    changed = [ln for ln in dirty.splitlines() if ln.strip()]
    if not changed:
        # Nothing to bank. Still try to push, because a previous run may have committed
        # locally and failed to push — that is exactly the stale-off-box-copy state.
        if push:
            _push(store, out)
        return 0

    lock = _acquire_lock(store)
    if lock is None:
        return 0  # Another seat is banking the same tree. The tree is the consistency unit.

    try:
        indexes, dangling, orphans = index_integrity(store)
        if dangling or orphans:
            health = (f"DEGRADED: {len(dangling)} dangling link(s), "
                      f"{len(orphans)} orphan(s)")
        else:
            health = "integrity clean"

        subject = f"{AUTOSAVE_PREFIX} {len(changed)} change(s) — {health}"
        body = [
            "",
            "Automatic snapshot of the shared agent-memory store, written by",
            "silt/scripts/check_agent_memory_link.py --autosave.",
            "",
            f"Trigger:   {trigger}",
            f"When:      {time.strftime('%Y-%m-%dT%H:%M:%SZ', time.gmtime())}",
            f"Integrity: {indexes} seat index(es) checked, {len(dangling)} dangling, "
            f"{len(orphans)} orphan",
        ]
        for d in dangling[:20]:
            body.append(f"  dangling: {d}")
        for o in orphans[:20]:
            body.append(f"  orphan:   {o}")
        if len(dangling) > 20 or len(orphans) > 20:
            body.append("  (list truncated)")
        body += [
            "",
            "An `autosave:` commit is a BYTE BACKUP, not a curated safe point. Deliberate",
            "safe points never carry this prefix, so they stay findable:",
            "",
            "    git log --invert-grep --grep='^autosave:'",
            "",
            AUTOSAVE_SCAR_ID,
        ]

        rc, msg = _git(store, "add", "-A")
        if rc != 0:
            print(f"[{AUTOSAVE_SCAR_ID}] autosave FAILED to stage {store}: {msg.strip()}",
                  file=out)
            return 0
        rc, msg = _git(store, "commit", "-m", subject, "-m", "\n".join(body))
        if rc != 0:
            print(f"[{AUTOSAVE_SCAR_ID}] autosave FAILED to commit {store}: {msg.strip()}",
                  file=out)
            return 0
        print(f"[{AUTOSAVE_SCAR_ID}] {subject}", file=out)
    finally:
        try:
            lock.unlink()
        except OSError:
            pass

    if push:
        _push(store, out)
    return 0


def _push(store, out=sys.stderr) -> bool:
    """Best effort. A failed push is REPORTED and never fatal — the bytes are committed."""
    rc, msg = _git(store, "push", timeout=PUSH_TIMEOUT_SECONDS)
    if rc == 0:
        return True
    if "Everything up-to-date" in msg:
        return True
    print(
        f"[{AUTOSAVE_SCAR_ID}] autosave committed LOCALLY but the push FAILED — the\n"
        f"  off-box copy of agent memory is now behind. It will be retried on the next\n"
        f"  trigger. If this repeats, push {store} by hand.\n"
        f"  git said: {msg.strip().splitlines()[-1] if msg.strip() else '(no output)'}",
        file=out,
    )
    return False


# ---------------------------------------------------------------------------


def _autosave_self_test() -> int:
    """CI has no memory store, so a gate that merely runs autosave is decoration.

    This MANUFACTURES a store plus a bare remote in a temp directory and drives the five
    behaviours the brief is buying. Two of them are over-action arms — a fix that
    committed on every invocation, or that refused a degraded tree, would produce exactly
    the green you were hoping for.
    """
    import tempfile

    failures = []
    with tempfile.TemporaryDirectory() as tmp:
        tmp = Path(tmp)
        store = tmp / "store"
        seat = store / "builder"
        seat.mkdir(parents=True)
        (seat / INDEX_NAME).write_text("# index\n\n- [One](one.md) — hook\n")
        (seat / "one.md").write_text("one\n")

        remote = tmp / "remote.git"
        _git(tmp, "init", "--bare", str(remote))
        for cmd in (
            ("init", "-b", "main"),
            ("config", "user.email", "autosave@silt.invalid"),
            ("config", "user.name", "autosave self-test"),
            ("remote", "add", "origin", str(remote)),
        ):
            _git(store, *cmd)
        _git(store, "add", "-A")
        _git(store, "commit", "-m", "memory: SAFE POINT — the human-written kind")
        _git(store, "push", "-u", "origin", "main")

        def subjects():
            return _git(store, "log", "--format=%s")[1].splitlines()

        # 1. UNCHANGED TREE — the over-action arm, and it is asserted on SILENCE as well
        #    as on the log. `git commit` with nothing staged already refuses, so "no new
        #    commit" alone is satisfied by a fix that tries anyway and is rebuffed. What
        #    that fix costs is a FAILED-to-commit line on stderr at the end of every turn
        #    that touched no memory — which is most of them. Noise on the common path is
        #    how a guard gets switched off, so the silence is part of the contract.
        before = subjects()
        quiet = tmp / "quiet.txt"
        with open(quiet, "w") as fh:
            rc = autosave(store, "self-test/unchanged", out=fh)
        if rc != 0:
            failures.append(f"autosave on an unchanged tree returned {rc}, not 0")
        if subjects() != before:
            failures.append("autosave committed on an UNCHANGED tree — it would fill the "
                            "history with empty snapshots and bury the safe points")
        if quiet.read_text().strip():
            failures.append("autosave on an UNCHANGED tree was not SILENT: "
                            f"{quiet.read_text()!r}")

        # 2. A CHANGE, CLEAN INDEXES — commits, prefixed, and pushes to the reachable remote.
        (seat / "two.md").write_text("two\n")
        (seat / INDEX_NAME).write_text("# index\n\n- [One](one.md)\n- [Two](two.md)\n")
        rc = autosave(store, "self-test/clean", out=open(os.devnull, "w"))
        head = subjects()[0] if subjects() else ""
        if rc != 0:
            failures.append(f"autosave on a changed tree returned {rc}, not 0")
        if not head.startswith(AUTOSAVE_PREFIX):
            failures.append(f"autosave subject {head!r} lacks the {AUTOSAVE_PREFIX!r} prefix, "
                            f"so it is indistinguishable from a deliberate safe point")
        if "integrity clean" not in head:
            failures.append(f"a clean tree was not labelled 'integrity clean': {head!r}")
        local = _git(store, "rev-parse", "HEAD")[1].strip()
        pushed = _git(store, "rev-parse", "origin/main")[1].strip()
        if local != pushed:
            failures.append("autosave did not push to a REACHABLE remote "
                            f"(local {local[:8]} != origin/main {pushed[:8]})")

        # 3. SAFE POINTS STAY FINDABLE through the autosaves.
        marked = _git(store, "log", "--format=%s", "--invert-grep",
                      f"--grep=^{AUTOSAVE_PREFIX}")[1].splitlines()
        if marked != ["memory: SAFE POINT — the human-written kind"]:
            failures.append(f"--invert-grep did not isolate the human safe point: {marked!r}")

        # 4. DEGRADED TREE — bytes beat purity. It MUST still commit, and MUST say so.
        #    The over-action arm: a fix that refused to commit a dangling tree would
        #    withhold the backup at the exact moment the tree is worst.
        (seat / INDEX_NAME).write_text(
            "# index\n\n- [One](one.md)\n- [Two](two.md)\n- [Gone](gone.md)\n")
        (seat / "three.md").write_text("three\n")          # linked by nobody -> orphan
        idx, dangling, orphans = index_integrity(store)
        if [d for d in dangling if d.endswith("gone.md")] == []:
            failures.append("the dangling index link was not detected")
        if [o for o in orphans if o.endswith("three.md")] == []:
            failures.append("the orphan memory file was not detected")
        if [o for o in orphans if o.endswith("two.md")] != []:
            failures.append("a LINKED file was reported as an orphan — the check is too "
                            "eager and its DEGRADED label would mean nothing")
        # 4b. THE REVIEW SUBTREE IS NOT MEMORY. Three arms, because the middle one is
        #     what catches a lazy fix (a blanket "ignore anything nested" would pass
        #     the first arm and silently stop reporting real orphans).
        (seat / REVIEW_SUBTREE).mkdir(exist_ok=True)
        (seat / REVIEW_SUBTREE / "RULING-x.md").write_text("a ruling, cited by path\n")
        (seat / "notes").mkdir(exist_ok=True)
        (seat / "notes" / "real-orphan.md").write_text("memory nobody links\n")
        (seat / INDEX_NAME).write_text(
            "# index\n\n- [One](one.md)\n- [Two](two.md)\n- [Gone](gone.md)\n"
            "- [MissingReview](reviews/NOT-THERE.md)\n")
        idx, dangling, orphans = index_integrity(store)
        if [o for o in orphans if "RULING-x.md" in o] != []:
            failures.append("a file under reviews/ was reported as an ORPHAN — 641 of "
                            "them would label every autosave DEGRADED and the signal dies")
        if [o for o in orphans if o.endswith("notes/real-orphan.md")] == []:
            failures.append("OVER-EXCLUSION: a nested memory file OUTSIDE reviews/ stopped "
                            "being reported as an orphan — the exclusion is too broad")
        if [d for d in dangling if d.endswith("reviews/NOT-THERE.md")] == []:
            failures.append("an index entry pointing INTO reviews/ at a missing file was "
                            "not DANGLING — the exclusion must be orphan-direction only")
        (seat / REVIEW_SUBTREE / "RULING-x.md").unlink()
        (seat / REVIEW_SUBTREE).rmdir()
        (seat / "notes" / "real-orphan.md").unlink()
        (seat / "notes").rmdir()
        # 4c. CASE-ONLY MISMATCH. The dangling test must be case-EXACT (_exists_cased),
        #     not Path.exists(), which resolves the wrong case on APFS — the volume the
        #     seats and the PreToolUse/Stop hooks run on.
        #
        #     WHERE THIS DISCRIMINATES: on a case-INSENSITIVE volume the first arm goes
        #     RED against the old `(seat / target).exists()` and GREEN after the fix. On a
        #     case-SENSITIVE volume (Linux CI) the filesystem already answers correctly, so
        #     that arm is a restatement rather than a discrimination there. Say it plainly
        #     rather than let the CI green be read as proof. The SECOND arm is the
        #     over-action control and is volume-independent: a fix that rejected every
        #     nested or unusual link would pass the first arm and break every real one.
        (seat / "cased-memory.md").write_text("cased\n")
        (seat / INDEX_NAME).write_text(
            "# index\n\n- [One](one.md)\n- [Two](two.md)\n- [Cased](Cased-Memory.md)\n")
        idx, dangling, orphans = index_integrity(store)
        if [d for d in dangling if d.endswith("Cased-Memory.md")] == []:
            failures.append("a case-only mismatch in an index link was NOT reported "
                            "dangling — the wrong case resolves on this volume and the "
                            "index's lie reads as clean")
        (seat / INDEX_NAME).write_text(
            "# index\n\n- [One](one.md)\n- [Two](two.md)\n- [Cased](cased-memory.md)\n")
        idx, dangling, orphans = index_integrity(store)
        if [d for d in dangling if d.endswith("cased-memory.md")] != []:
            failures.append("OVER-ACTION: the CORRECTLY-cased link was reported dangling "
                            "— a case-exact test that rejects real links dangles the "
                            "whole store and the DEGRADED label stops meaning anything")
        (seat / "cased-memory.md").unlink()
        (seat / INDEX_NAME).write_text(
            "# index\n\n- [One](one.md)\n- [Two](two.md)\n- [Gone](gone.md)\n")

        n_before = len(subjects())
        autosave(store, "self-test/degraded", out=open(os.devnull, "w"))
        head = subjects()[0]
        if len(subjects()) != n_before + 1:
            failures.append("autosave REFUSED to commit a degraded tree — bytes beat purity")
        if "DEGRADED" not in head:
            failures.append(f"a degraded tree was not labelled DEGRADED: {head!r}")

        # 5. UNREACHABLE REMOTE — fail soft. Commits locally, returns 0, reports.
        _git(store, "remote", "set-url", "origin", str(tmp / "no-such-remote.git"))
        (seat / "four.md").write_text("four\n")
        n_before = len(subjects())
        report = tmp / "report.txt"
        with open(report, "w") as fh:
            rc = autosave(store, "self-test/unreachable", out=fh)
        said = report.read_text()
        if rc != 0:
            failures.append(f"an unreachable remote made autosave return {rc} — a memory "
                            f"guard must not block a seat because the network is down")
        if len(subjects()) != n_before + 1:
            failures.append("autosave did not commit LOCALLY when the remote was unreachable")
        if "push FAILED" not in said:
            failures.append(f"a failed push was not reported to the seat: {said!r}")

    if failures:
        print(f"FAIL [{AUTOSAVE_SCAR_ID}] — memory-store autosave is wrong:", file=sys.stderr)
        for f in failures:
            print(f"  {f}", file=sys.stderr)
        return 1
    print(f"OK [{AUTOSAVE_SCAR_ID}] — autosave commits a changed store with an "
          f"`{AUTOSAVE_PREFIX}` prefix, skips an unchanged one, labels a degraded tree "
          f"DEGRADED and commits it anyway, keeps human safe points isolable by "
          f"--invert-grep, and fails soft on an unreachable remote.")
    return 0


def main(argv=None) -> int:
    argv = sys.argv[1:] if argv is None else argv
    if "--self-test" in argv:
        return _autosave_self_test()
    if "--hook" in argv:
        return run_hook()
    if "--ci" in argv:
        return run_ci("--quiet" in argv)
    if "--autosave" in argv:
        return autosave(STORE, "manual", out=sys.stdout)

    link = ROOT / MEMORY_REL
    status, lines = evaluate(link)
    if status != OK:
        return _fail(status, lines, ROOT, 1)
    if "--quiet" not in argv:
        print(f"OK [{SCAR_ID}] — " + lines[0])
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
