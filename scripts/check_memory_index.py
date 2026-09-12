#!/usr/bin/env python3
"""check_memory_index.py — a seat's MEMORY.md must be NAVIGABLE, not merely resolvable.

WHAT THIS DEFENDS, AND WHY IT IS NOT NOISE
  Each of the eight agent seats keeps a pointer list at `<store>/<seat>/MEMORY.md`. The
  file is truncated at ~24.4 KB with no warning, so it is compacted periodically: rows are
  shortened, grouped, and re-ordered by hand. A hand compaction is exactly the operation
  that loses things.

  The verification that existed for a compaction asserted LINK REACHABILITY in two
  directions — index -> file (no dangling) and file -> index (no orphans), i.e.
  `links_before` is a subset of `links_after`. It checks LINK TARGETS ONLY.

  That is a real property and it is not enough. A compaction can preserve every single
  target and still destroy every descriptive hook:

      - [Mode oracle CLOSED by padding the secrets box](mode-oracle-secrets-padding.md)
        — #821; a BOX hides content, never LENGTH — fix at the layer the length IS.

      becomes

      - [mode-oracle-secrets-padding](mode-oracle-secrets-padding.md)

  Both directions stay green. `links_before == links_after`. Nothing is dangling and
  nothing is orphaned. And the index is now unusable: the seat reading it has 150 file
  names and no way to know which one holds the scar it needs, so it opens none of them
  and the memory is lost in every sense that matters. This stayed green through a
  near-loss of 28 entries.

  So the third assertion here is the one that was missing: EVERY LINK CARRIES A HOOK.
  Do not delete it as noise. Reachability is about whether the file can be opened;
  the hook is about whether anyone will know to open it.

THE THREE ASSERTIONS
  1. DANGLING  — every `[text](target.md)` resolves.
  2. ORPHAN    — every `*.md` in the seat directory is linked from that seat's index.
  3. THIN      — every link carries a descriptive hook (the new one).

  Plus a REPORT of each index's byte size and line count against the truncation cap,
  because a row that is past the cap is dropped whether or not it is well-formed.

HOW "A HOOK" IS DEFINED, AND WHY IT IS AN `OR`
  A link is hooked when EITHER
    (a) its link TEXT says something the filename does not, or
    (b) its ROW carries prose outside the links — a trailing `— description`, or a
        `- **Topic:** [a](a.md) · [b](b.md)` group heading.

  A link is THIN only when NEITHER holds: the text normalizes to the filename stem
  (or is empty) AND the row is bare. That is precisely the shape a description-destroying
  compaction produces.

  The `and` form the brief first suggested — text informative AND row prose present —
  was MEASURED against the real store and rejected. Treating "link text is contained in
  the stem" as uninformative flags 136 of 730 links, and 119 of those 136 sit on rows
  that DO carry prose and are perfectly navigable
  (`[multi-leaf recompute](floorbox-Opayload-multileaf-recompute.md)` inside a
  `Floor-box:` group). Requiring row prose as well would redden every compacted group row
  in the store — and a lint that cries wolf is the one that gets switched off, which is
  the same silence this check exists to break. On the shipped `or` rule the same store
  reports 5 thin links out of 730 (0.7 %), all of one shape, plus 25 INFO echoes.

  THOSE FIGURES ARE A SNAPSHOT, re-driven 2026-09-12 against the live store. The store
  is written by eight seats every session, so they drift by design and an earlier revision
  of this paragraph had already drifted (it said 140 of 699 and 14 thin). Re-drive them
  with `--store`, do not cite them; what does NOT drift is the RATIO, two orders of
  magnitude between the two rules, and that is the whole argument.

  Stem-echoing text on a row that DOES carry prose is reported as INFO, never as a
  failure. It is waste (the target is right there in the raw markdown the seat reads;
  one seat carried 4.7 KB of it) but it is not a loss of navigability.

WHY BASENAME MEMBERSHIP AND NOT `os.path.exists()`
  The store lives on a case-INSENSITIVE volume. `os.path.exists()` says True for
  `apb-foo.md` when the file on disk is `APB-foo.md`, so the link looks fine locally and
  dangles on any case-sensitive filesystem. Worse, a naive audit then reports the REAL
  file as an orphan while counting the phantom lowercase name as linked — two wrong
  findings from one cause, and the one that looks actionable is the wrong one. Every
  target is resolved by testing its basename for MEMBERSHIP in `os.listdir(parent)`.
  One such link was live in the store for about two weeks.

SCOPE NOTES
  - A cross-seat link needs a path (`../tester/foo.md`, not `foo.md`). Those are resolved
    against the store, not reported dangling. They are live: `researcher/MEMORY.md` cites
    `../tester/era4-regcap-measurement-2026-08-29.md` and the file is there.
    THIS RULE IS SHARED. `scripts/check_agent_memory_link.py` `_exists_cased` reads the
    same store and must give the same answer for the same link; the two are gated
    separately (self-test arm 4 here, arm 4d there) and a change to either is a change to
    both. They disagreed once, for one review cycle: a case-exactness fix there returned
    False on every `..` component and called it "unreachable", which took the real store
    from 0 dangling to 1 and would have written a standing `DEGRADED` into every autosave
    subject. Reachability is the property; the seat directory is not a boundary.
  - `reviews/` is excluded from the ORPHAN direction only. Those documents are cited by
    absolute path from certifications and rulings; they are not reached index-first, and
    counting all of them would label every run red. The DANGLING direction still covers
    them: an index entry that points into `reviews/` and misses is still a lie.
  - Read-only. This never edits the store.

  Dependency-free (stdlib only). Exit 0 on pass, 1 on any finding, 2 on a usage error.
  An ABSENT store is exit 0 with a SKIP line — CI has no memory store, and a check that
  is red on every build is a check nobody reads. `--self-test` is the part CI can run.

Run: python3 scripts/check_memory_index.py [--store DIR] [--self-test] [--quiet]
"""
import os
import re
import sys
from pathlib import Path

SCAR_ID = "scar:memory-index-compaction-destroyed-the-hooks-2026-09-11"

STORE = Path(
    os.environ.get("SILT_AGENT_MEMORY_STORE", Path.home() / ".claude" / "silt-agent-memory")
)

INDEX_NAME = "MEMORY.md"
REVIEW_SUBTREE = "reviews"

# The context truncation cap, measured 2026-09-11. Past it, rows are dropped silently.
CAP_BYTES = 24400
NEAR_CAP_FRACTION = 0.90

# `[text](target)`. Targets never contain whitespace or parentheses in these indexes.
LINK_RE = re.compile(r"\[([^\]]*)\]\(([^()\s]+)\)")
# Normalizes a link text or a filename stem to comparable letters and digits, so
# "★ Reviewer read-exact-worktree SCAR" and "reviewer-read-exact-worktree-scar" are equal.
NON_ALNUM_RE = re.compile(r"[^a-z0-9]+")
# Row prose, once the links are removed. Bullets, stars, arrows and separators are not prose.
NON_WORD_RE = re.compile(r"[^A-Za-z0-9]+")


def normalize(s):
    return NON_ALNUM_RE.sub("", s.lower())


def is_external(target):
    return "://" in target or target.startswith(("#", "mailto:"))


def resolve(seat, target):
    """(exists, display) for one relative target, by PER-COMPONENT LISTDIR MEMBERSHIP.

    `os.path.exists()` is deliberately not used: see the docstring. Paths are normalized
    textually (`os.path.normpath`, for the display only) and never through
    `Path.resolve()`, which on macOS can hand back the on-disk casing and reintroduce the
    exact hole.

    EVERY component is checked, not just the basename. A basename-only test was measured
    against `check_agent_memory_link.py` `_exists_cased` and DISAGREED with it on
    `../Tester/scar.md` when the directory on disk is `tester`: this side said RESOLVES,
    because opening a wrong-cased directory succeeds on APFS, and the seat component is
    exactly where a cross-seat link's typo goes. Closing it for the basename and leaving
    it open for the path is the same defect at a different depth.

    `..` resolves against the store, FLOORED at the store root: a link that climbs above
    the store is outside the tree this check governs and cannot be resolved by it.
    """
    joined = os.path.normpath(os.path.join(str(seat), target))
    root, cur = Path(seat).parent, Path(seat)
    for part in target.split("/"):
        if part in ("", "."):
            continue
        if part == "..":
            if cur == root:
                return False, joined
            cur = cur.parent
            continue
        try:
            if part not in os.listdir(cur):
                return False, joined
        except OSError:
            return False, joined
        cur = cur / part
    return True, joined


def audit_seat(seat):
    """Audit one seat directory. Returns a dict of findings; never writes anything."""
    index = seat / INDEX_NAME
    text = index.read_text(encoding="utf-8", errors="replace")
    lines = text.splitlines()

    dangling, thin, echo_info = [], [], []
    linked = set()
    # Counted per OCCURRENCE, not by set size. `linked` is a set, so two rows pointing at
    # the same file collapse to one member; reporting its size as the link count would
    # understate every index that repeats a target (crypto-specialist links one file from
    # three rows). The set is for resolving orphans; the count is its own quantity.
    occurrences = 0

    for lineno, line in enumerate(lines, 1):
        links = LINK_RE.findall(line)
        if not links:
            continue
        # What the row says beyond its links: the `— description`, or a group heading.
        row_prose = NON_WORD_RE.sub(" ", LINK_RE.sub("", line)).strip()
        for link_text, target in links:
            if is_external(target):
                continue
            target = target.split("#", 1)[0]
            if not target:
                continue
            occurrences += 1
            exists, joined = resolve(seat, target)
            if exists:
                linked.add(os.path.normpath(joined))
            else:
                dangling.append((lineno, target))

            stem = Path(target).stem
            stem_echo = normalize(link_text) in ("", normalize(stem))
            if stem_echo and not row_prose:
                thin.append((lineno, link_text, target))
            elif stem_echo:
                echo_info.append((lineno, target))

    orphans = []
    for root, dirnames, filenames in os.walk(seat):
        rel_root = os.path.relpath(root, seat)
        if rel_root == ".":
            dirnames[:] = [d for d in dirnames if d != REVIEW_SUBTREE]
        for name in sorted(filenames):
            if not name.endswith(".md"):
                continue
            full = os.path.normpath(os.path.join(root, name))
            if full == os.path.normpath(str(index)):
                continue
            if full not in linked:
                orphans.append(os.path.relpath(full, seat))

    return {
        "seat": seat.name,
        "bytes": len(text.encode("utf-8")),
        "lines": len(lines),
        "links": occurrences,
        "targets": len(linked),
        "dangling": dangling,
        "orphans": sorted(orphans),
        "thin": thin,
        "echo": echo_info,
    }


def audit_store(store):
    """Audit every seat that has an index. Returns a list of per-seat report dicts."""
    seats = sorted(p for p in store.iterdir() if p.is_dir() and not p.name.startswith("."))
    return [audit_seat(s) for s in seats if (s / INDEX_NAME).is_file()]


def report(reports, quiet, out=sys.stdout, err=sys.stderr):
    """Print the findings. Returns the exit code."""
    total = sum(len(r["dangling"]) + len(r["orphans"]) + len(r["thin"]) for r in reports)

    if not quiet or total:
        stream = err if total else out
        print(f"{'seat':<20}{'bytes':>8}{'lines':>7}{'links':>7}{'files':>7}"
              f"{'dangling':>10}{'orphans':>9}{'thin':>6}", file=stream)
        for r in reports:
            flag = ""
            if r["bytes"] > CAP_BYTES:
                flag = f"  OVER CAP ({CAP_BYTES} B) — rows past the cut are already dark"
            elif r["bytes"] >= CAP_BYTES * NEAR_CAP_FRACTION:
                flag = f"  near cap ({CAP_BYTES} B) — compact before adding a row"
            print(f"{r['seat']:<20}{r['bytes']:>8}{r['lines']:>7}{r['links']:>7}"
                  f"{r['targets']:>7}{len(r['dangling']):>10}{len(r['orphans']):>9}"
                  f"{len(r['thin']):>6}{flag}", file=stream)

    if not total:
        if not quiet:
            print(f"\nOK [{SCAR_ID}] — {len(reports)} seat index(es): every link resolves by "
                  f"listdir membership, every memory file outside reviews/ is linked from its "
                  f"own index, and every link carries a descriptive hook.", file=out)
        return 0

    print(f"\nFAIL [{SCAR_ID}] — {total} finding(s):", file=err)
    for r in reports:
        for lineno, target in r["dangling"]:
            print(f"  DANGLING {r['seat']}/{INDEX_NAME}:{lineno} -> {target}\n"
                  f"           no such name in that directory (basename membership; a "
                  f"case-only mismatch reads as present on this volume and dangles elsewhere)",
                  file=err)
        for rel in r["orphans"]:
            print(f"  ORPHAN   {r['seat']}/{rel}\n"
                  f"           on disk and linked from no row of {r['seat']}/{INDEX_NAME}. "
                  f"Unreachable for any seat that reads the index first.", file=err)
        for lineno, link_text, target in r["thin"]:
            print(f"  THIN     {r['seat']}/{INDEX_NAME}:{lineno} -> {target}\n"
                  f"           link text {link_text!r} says nothing the filename does not, "
                  f"and the row carries no description. The target resolves; the hook is gone.",
                  file=err)
    print("\n  A THIN row is the finding the two reachability directions cannot see: the\n"
          "  link works and nobody knows to follow it. Give the row a `— one-line hook`,\n"
          "  or give the link text something the filename does not already say.\n",
          file=err)

    echoes = sum(len(r["echo"]) for r in reports)
    if echoes:
        print(f"  INFO: {echoes} further link(s) echo their filename in the link text but sit\n"
              f"  on a described row. Navigable, so not a failure — but the text is pure waste\n"
              f"  against the {CAP_BYTES} B cap and is the cheapest thing to cut when compacting.\n",
              file=err)
    return 1


# ---------------------------------------------------------------------------
# SELF-TEST — the gate is driven RED before it is believed
# ---------------------------------------------------------------------------


def _self_test():
    """Manufacture stores and drive each arm. CI has no real store, so this IS the gate.

    The load-bearing arm is 1/1b: a fixture where EVERY LINK RESOLVES and there are NO
    orphans, so both reachability directions are green, and only the descriptions have
    been destroyed. A check that passed that fixture would be the check we already had.
    Arm 1b is its control — the same fixture with the descriptions restored must be
    GREEN, which is what proves the red in 1 came from the descriptions and not from
    some other property of the fixture.
    """
    import tempfile

    failures = []

    def build(tmp, rows, extra_files=(), skip_dirs=()):
        store = Path(tmp)
        seat = store / "builder"
        seat.mkdir(parents=True, exist_ok=True)
        (seat / INDEX_NAME).write_text("# index\n\n" + "\n".join(rows) + "\n", encoding="utf-8")
        for rel, body in extra_files:
            p = store / rel
            p.parent.mkdir(parents=True, exist_ok=True)
            p.write_text(body, encoding="utf-8")
        for d in skip_dirs:
            (store / d).mkdir(parents=True, exist_ok=True)
        return store

    files = [
        ("builder/one.md", "one\n"),
        ("builder/two.md", "two\n"),
        ("builder/three.md", "three\n"),
    ]

    # 1. THE NEW ASSERTION. Every target resolves, nothing is orphaned, descriptions gone.
    with tempfile.TemporaryDirectory() as tmp:
        store = build(tmp, [
            "- [one](one.md)",
            "- [two](two.md)",
            "- [three](three.md)",
        ], files)
        reps = audit_store(store)
        r = reps[0]
        if r["dangling"]:
            failures.append("arm 1 fixture is wrong: it has dangling links, so a red would "
                            "not isolate the description defect")
        if r["orphans"]:
            failures.append("arm 1 fixture is wrong: it has orphans, so a red would not "
                            "isolate the description defect")
        if len(r["thin"]) != 3:
            failures.append(f"arm 1: a DESCRIPTION-DESTROYED index reported {len(r['thin'])} "
                            f"thin link(s), want 3 — the assertion that was missing is still "
                            f"missing")
        if report(reps, quiet=True, out=open(os.devnull, "w"),
                  err=open(os.devnull, "w")) == 0:
            failures.append("arm 1: the check EXITED 0 on an index whose every description "
                            "was destroyed — this is the exact green the old two-direction "
                            "audit gave, and the whole reason this script exists")

    # 1b. CONTROL. The same fixture, descriptions restored, must be GREEN.
    with tempfile.TemporaryDirectory() as tmp:
        store = build(tmp, [
            "- [one](one.md) — the first hook, a real sentence.",
            "- [Something the filename does not say](two.md)",
            "- **Grouped topic with prose:** [three](three.md)",
        ], files)
        reps = audit_store(store)
        if reps[0]["thin"]:
            failures.append(f"arm 1b CONTROL: a DESCRIBED index was reported thin "
                            f"({reps[0]['thin']}). Either the row-prose hook or the "
                            f"informative-text hook is not recognised, and the arm-1 red "
                            f"proves nothing")
        if report(reps, quiet=True, out=open(os.devnull, "w"),
                  err=open(os.devnull, "w")) != 0:
            failures.append("arm 1b CONTROL: the check failed a clean, fully described "
                            "index — it is red on everything and its red means nothing")

    # 2. DANGLING, including the case-only mismatch the volume hides.
    #
    #    WHERE THIS DISCRIMINATES, AND WHERE IT ONLY RESTATES. The case-only leg is
    #    VOLUME-DEPENDENT. On a case-INSENSITIVE volume (APFS — the one the seats and the
    #    hooks run on) it goes RED against a `resolve()` built on `os.path.exists()` and
    #    GREEN after the fix: measured, ablated script EXIT=1 with this arm's message. On
    #    a case-SENSITIVE volume — Linux CI, which is where `--self-test` actually runs —
    #    the filesystem already answers correctly, so the SAME ablation exits 0 and this
    #    arm is a restatement, not a discrimination: measured by monkeypatching
    #    `os.path.exists` to be case-exact and re-running the ablated script, EXIT=0.
    #    Say it plainly rather than let the CI green be read as proof that a
    #    `listdir` -> `exists()` regression would be caught there. It would not be.
    #    The ABSENT-file leg below and every other arm are volume-independent.
    #    `check_agent_memory_link.py` arm 4c carries the same disclosure for the same
    #    reason.
    with tempfile.TemporaryDirectory() as tmp:
        store = build(tmp, [
            "- [Gone entirely](gone.md) — a hook, so THIN cannot be what fires.",
            "- [The phantom lowercase](apb-foo.md) — a hook, so THIN cannot be what fires.",
        ], [("builder/APB-foo.md", "the real file, capitalised\n")])
        r = audit_store(store)[0]
        if not [t for _, t in r["dangling"] if t == "gone.md"]:
            failures.append("arm 2: a link to a file that is simply absent was not DANGLING")
        if not [t for _, t in r["dangling"] if t == "apb-foo.md"]:
            failures.append("arm 2: a CASE-ONLY mismatch was not DANGLING. On a "
                            "case-insensitive volume os.path.exists() says True, the link "
                            "passes here and breaks on any case-sensitive filesystem, and "
                            "the real file gets reported as an orphan instead")
        if not [o for o in r["orphans"] if o == "APB-foo.md"]:
            failures.append("arm 2: the real capitalised file was not reported as an orphan")

    # 3. ORPHANS, with both an under- and an over-exclusion arm around reviews/.
    with tempfile.TemporaryDirectory() as tmp:
        store = build(tmp, [
            "- [One, with a hook](one.md) — described.",
            "- [A ruling that moved](reviews/NOT-THERE.md) — described.",
        ], [
            ("builder/one.md", "one\n"),
            ("builder/unlinked.md", "nobody links this\n"),
            ("builder/reviews/RULING-x.md", "a ruling, cited by absolute path\n"),
            ("builder/notes/nested-orphan.md", "memory nobody links, outside reviews/\n"),
        ])
        r = audit_store(store)[0]
        if "unlinked.md" not in r["orphans"]:
            failures.append("arm 3: a top-level unlinked memory file was not an ORPHAN")
        if [o for o in r["orphans"] if "RULING-x.md" in o]:
            failures.append("arm 3: a file under reviews/ was reported as an ORPHAN — those "
                            "are cited by absolute path, and counting them makes every run red")
        if "notes/nested-orphan.md" not in r["orphans"]:
            failures.append("arm 3 OVER-EXCLUSION: a nested memory file OUTSIDE reviews/ was "
                            "not reported — a blanket 'ignore anything nested' fix passes the "
                            "reviews/ arm and silently stops reporting real orphans")
        if not [t for _, t in r["dangling"] if t == "reviews/NOT-THERE.md"]:
            failures.append("arm 3: an index entry pointing INTO reviews/ at a missing file "
                            "was not DANGLING — the exclusion must be orphan-direction only")

    # 4. CROSS-SEAT links carry a path and must resolve against the store.
    with tempfile.TemporaryDirectory() as tmp:
        store = build(tmp, [
            "- [A scar the tester owns](../tester/scar.md) — described.",
            "- [A scar the tester does not own](../tester/absent.md) — described.",
            "- [The seat name miscased](../Tester/scar.md) — described.",
            "- [Above the store](../../outside.md) — described.",
        ], [
            ("tester/scar.md", "the tester's scar\n"),
            ("tester/" + INDEX_NAME, "# tester\n\n- [Its own scar](scar.md) — described.\n"),
        ])
        # The climb target EXISTS, one level above the store. Without it the last leg
        # would pass on plain ABSENCE and never reach the floor it names.
        (Path(tmp).parent / "outside.md").write_text("not memory this store governs\n",
                                                     encoding="utf-8")
        reps = audit_store(store)
        b = [r for r in reps if r["seat"] == "builder"][0]
        if [t for _, t in b["dangling"] if t == "../tester/scar.md"]:
            failures.append("arm 4: a RESOLVING cross-seat link was reported dangling — the "
                            "`../seat/file.md` form must be resolved, not rejected")
        if not [t for _, t in b["dangling"] if t == "../tester/absent.md"]:
            failures.append("arm 4: a cross-seat link at a MISSING file was not dangling — "
                            "handling the path form must not mean skipping it")
        if not [t for _, t in b["dangling"] if t == "../Tester/scar.md"]:
            failures.append("arm 4: a cross-seat link whose SEAT component is wrong-cased "
                            "was not dangling — case-exactness must hold for every path "
                            "component, not just the basename, or the hole this check "
                            "closes just moves one level up. Measured: a basename-only "
                            "test answers this link differently from "
                            "check_agent_memory_link.py, and the two read one store")
        if not [t for _, t in b["dangling"] if t == "../../outside.md"]:
            failures.append("arm 4: a link climbing ABOVE the store root was not dangling "
                            "— that is outside the tree this check governs. The target "
                            "EXISTS on disk, so this leg fails on the missing floor and "
                            "not on mere absence")

    # 5. OVER-ACTION. A clean, well-described store exits 0 and says so.
    with tempfile.TemporaryDirectory() as tmp:
        store = build(tmp, [
            "- [The first memory](one.md) — a one-line hook that means something.",
            "- **A grouped topic:** [two](two.md) · [three](three.md)",
        ], files)
        reps = audit_store(store)
        if report(reps, quiet=True, out=open(os.devnull, "w"),
                  err=open(os.devnull, "w")) != 0:
            failures.append(f"arm 5: a CLEAN store did not exit 0 — findings were "
                            f"{reps[0]['dangling']} {reps[0]['orphans']} {reps[0]['thin']}")

    # 6. An ABSENT store is a SKIP, not a failure. CI has no memory store, and a check
    #    that is red on every build is one nobody reads.
    import contextlib

    with tempfile.TemporaryDirectory() as tmp:
        with contextlib.redirect_stdout(open(os.devnull, "w")):
            rc = main(["--store", str(Path(tmp) / "no-such-store"), "--quiet"])
        if rc != 0:
            failures.append("arm 6: an ABSENT store did not exit 0 — wired into CI, where "
                            "no store exists, this check would be red on every build")

    if failures:
        print(f"FAIL [{SCAR_ID}] — the memory-index check is wrong:", file=sys.stderr)
        for f in failures:
            print(f"  {f}", file=sys.stderr)
        return 1
    print(f"OK [{SCAR_ID}] — the check goes RED on an index whose links all resolve and "
          f"whose descriptions were destroyed, GREEN on the same index described; catches a "
          f"case-only dangling link the volume hides; excludes reviews/ from the orphan "
          f"direction only; resolves cross-seat paths; and is silent on a clean store and "
          f"on an absent one.")
    return 0


def main(argv=None):
    argv = sys.argv[1:] if argv is None else argv
    quiet = "--quiet" in argv
    if "--self-test" in argv:
        return _self_test()

    store = STORE
    if "--store" in argv:
        i = argv.index("--store")
        if i + 1 >= len(argv):
            print("usage: --store needs a directory", file=sys.stderr)
            return 2
        store = Path(argv[i + 1])

    if not store.is_dir():
        print(f"SKIP [{SCAR_ID}] — no agent-memory store at {store}. Nothing to check "
              f"(this is CI's normal state: live memory lives outside git).")
        return 0

    reports = audit_store(store)
    if not reports:
        print(f"SKIP [{SCAR_ID}] — no seat under {store} has a {INDEX_NAME}.")
        return 0
    return report(reports, quiet)


if __name__ == "__main__":
    raise SystemExit(main())
