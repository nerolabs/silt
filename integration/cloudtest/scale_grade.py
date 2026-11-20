#!/usr/bin/env python3
"""scale_grade.py — grade one load step of the publish-throughput flow.

Reads the scale_gen.sh logs of every generator for the step, the committing
validator's `chain: committed block` lines (journalctl -o short-unix), and the
round-change count the validators logged in the step window, and prints one JSON
object: the measured numbers plus a verdict.

A step passes only when every generator finished its schedule, no publish was
skipped or left unfinished, at least --min-ok of the attempted publishes returned a
link, and the 95th-percentile time to a committed link is within --p95. Throughput
that merely keeps up on average is not enough: the p95 is what a publisher sees.
"""
import argparse, json, re, sys

COMMIT = re.compile(r"^(\d+(?:\.\d+)?)\s.*chain: committed block (\d+) \((\d+) entries")


def pct(xs, p):
    if not xs:
        return None
    xs = sorted(xs)
    return xs[min(len(xs) - 1, int(p * len(xs)))]


def read_gen(path):
    g = {"ok": [], "fail": 0, "fail_detail": "", "skip": 0, "done": None, "seen": 0}
    with open(path) as f:
        for line in f:
            p = line.split()
            if not p:
                continue
            if p[0] == "pub" and len(p) >= 6:
                g["seen"] += 1
                if p[5] == "ok":
                    g["ok"].append((int(p[3]) - int(p[2])) / 1e9)
                else:
                    g["fail"] += 1
                    g["fail_detail"] = " ".join(p[6:])[:200]
            elif p[0] == "skip":
                g["skip"] += 1
            elif p[0] == "done" and len(p) >= 2:
                g["done"] = int(p[1])
    return g


def read_commits(path, start, end):
    times, entries = [], 0
    with open(path) as f:
        for line in f:
            m = COMMIT.match(line.strip())
            if not m:
                continue
            t = float(m.group(1))
            if start <= t < end:
                times.append(t)
                entries += int(m.group(3))
    times.sort()
    gaps = [b - a for a, b in zip(times, times[1:])]
    return len(times), entries, gaps


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--label", required=True)
    ap.add_argument("--rate", type=float, required=True, help="offered publishes/s across all generators")
    ap.add_argument("--start", type=float, required=True, help="step start, Unix seconds")
    ap.add_argument("--dur", type=float, required=True, help="step length, seconds")
    ap.add_argument("--p95", type=float, required=True, help="p95 bound on time to a committed link, seconds")
    ap.add_argument("--min-ok", type=float, required=True, help="fraction of attempted publishes that must return a link")
    ap.add_argument("--commits", required=True)
    ap.add_argument("--rounds", default="?", help="round-change lines in the window, or ? if unread")
    ap.add_argument("--held", default="?", help="proposals held one height ahead in the window, or ? if unread")
    ap.add_argument("gens", nargs="+")
    a = ap.parse_args()

    gens = [read_gen(p) for p in a.gens]
    lat = [x for g in gens for x in g["ok"]]
    ok = len(lat)
    fail = sum(g["fail"] for g in gens)
    skip = sum(g["skip"] for g in gens)
    unfinished_gens = sum(1 for g in gens if g["done"] is None)
    attempted = sum(g["done"] if g["done"] is not None else g["seen"] + g["skip"] for g in gens)
    pending = attempted - ok - fail - skip
    blocks, entries, gaps = read_commits(a.commits, a.start, a.start + a.dur)
    p95 = pct(lat, 0.95)

    why = []
    if unfinished_gens:
        why.append(f"{unfinished_gens} generator(s) never finished their schedule")
    if skip:
        why.append(f"{skip} publishes skipped: the in-flight cap was full when they were due")
    if pending > 0:
        why.append(f"{pending} publishes still unfinished at the drain deadline")
    if attempted == 0:
        why.append("no publish was attempted")
    elif ok < a.min_ok * attempted:
        why.append(f"only {ok}/{attempted} publishes returned a link (need {a.min_ok:.0%})")
    if p95 is None or p95 > a.p95:
        why.append(f"p95 time to a committed link {'n/a' if p95 is None else f'{p95:.1f}s'} > {a.p95:.0f}s")

    row = {
        "label": a.label, "offered_per_s": a.rate, "attempted": attempted, "ok": ok, "fail": fail,
        "skip": skip, "pending": pending, "committed_per_s": round(ok / a.dur, 2) if a.dur else 0,
        "lat_p50_s": pct(lat, 0.5), "lat_p95_s": p95, "lat_max_s": max(lat) if lat else None,
        "blocks": blocks, "entries": entries,
        "entries_per_block": round(entries / blocks, 1) if blocks else 0,
        "block_gap_p50_s": pct(gaps, 0.5), "block_gap_max_s": max(gaps) if gaps else None,
        "round_changes": a.rounds, "held_proposals": a.held,
        "last_fail": next((g["fail_detail"] for g in gens if g["fail_detail"]), ""),
        "verdict": "fail" if why else "pass", "why": "; ".join(why),
    }
    json.dump(row, sys.stdout)
    print()


if __name__ == "__main__":
    main()
