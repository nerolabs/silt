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

Where a publish line carries the client's legs (join, issuers, keys, credits,
sign, stage, place, submit, commit), the row reports each leg's p50 and p95 over
the committed publishes, and "outside": the generator's time less the client's
own total, which is process start and writing the object. Where it names the
registry that accepted the entry, the row counts committed publishes per registry.
Where the generators sampled their own CPU, the row reports busy and steal over
every sample, so a step limited by its generators reads differently from one
limited by the network.

For each --validator NAME=WINDOW=CPU the row reports the cores the validator's
process used over the step (from two cumulative CPU readings) and its event
loop's busy share and queue wait (from the loop's per-handler budget lines). The
loop is one thread: a validator near one core with a busy loop is loop-bound,
however many cores its machine has.
"""
import argparse, json, re, sys

COMMIT = re.compile(r"^(\d+(?:\.\d+)?)\s.*chain: committed block (\d+) \((\d+) entries")


def pct(xs, p):
    if not xs:
        return None
    xs = sorted(xs)
    return xs[min(len(xs) - 1, int(p * len(xs)))]


def read_gen(path):
    g = {"ok": [], "fail": 0, "fail_detail": "", "skip": 0, "done": None, "seen": 0, "legs": [], "regs": {}, "cpu": []}
    with open(path) as f:
        for line in f:
            p = line.split()
            if not p:
                continue
            if p[0] == "pub" and len(p) >= 6:
                g["seen"] += 1
                if p[5] == "ok":
                    lat = (int(p[3]) - int(p[2])) / 1e9
                    g["ok"].append(lat)
                    legs = read_legs(p[7] if len(p) > 7 else "-")
                    if "total" in legs:
                        legs["outside"] = lat - legs["total"]
                        g["legs"].append(legs)
                    if len(p) > 8 and p[8] != "-":
                        g["regs"][p[8]] = g["regs"].get(p[8], 0) + 1
                else:
                    g["fail"] += 1
                    g["fail_detail"] = (p[6] if len(p) > 6 else "")[:200]
            elif p[0] == "skip":
                g["skip"] += 1
            elif p[0] == "done" and len(p) >= 2:
                g["done"] = int(p[1])
            elif p[0] == "cpu" and len(p) >= 4:
                g["cpu"].append((float(p[2]), float(p[3])))
    return g


def read_legs(tok):
    """join=0.312,issuers=0.101,…,total=6.004 -> an ordered dict of seconds; - -> {}."""
    legs = {}
    for kv in tok.split(","):
        k, sep, v = kv.partition("=")
        if sep:
            try:
                legs[k] = float(v)
            except ValueError:
                pass
    return legs


def read_validator(spec, dur):
    """NAME=WINDOW=CPU -> (name, {cores, loop_busy, loop_wait_s, loop_top}); a value
    that could not be read is None, never zero."""
    name, window, cpu = spec.split("=", 2)
    v = {"cores": None, "loop_busy": None, "loop_wait_s": None, "loop_top": []}
    try:
        reads = [l.split() for l in open(cpu) if len(l.split()) == 2]
        # A restart between the reads resets a process's count; that is unknown.
        if len(reads) == 2 and int(reads[1][0]) > int(reads[0][0]) and int(reads[1][1]) >= int(reads[0][1]):
            v["cores"] = round((int(reads[1][1]) - int(reads[0][1])) / 1e9 / (int(reads[1][0]) - int(reads[0][0])), 2)
    except (OSError, ValueError):
        pass
    try:
        by, wait, seen = {}, 0, False
        for l in open(window):
            p = l.split()
            if len(p) == 4 and p[0] == "budget":
                seen = True
                by[p[1]] = by.get(p[1], 0) + int(p[2] or 0)
                wait += int(p[3] or 0)
        if seen and dur:
            v["loop_busy"] = round(sum(by.values()) / 1000 / dur, 2)
            v["loop_wait_s"] = round(wait / 1000, 1)
            v["loop_top"] = [[k, round(ms / 1000, 1)] for k, ms in sorted(by.items(), key=lambda kv: -kv[1])[:3]]
    except OSError:
        pass
    return name, v


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
    ap.add_argument("--validator", action="append", default=[], help="NAME=WINDOW=CPU: a validator's window and CPU files")
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
    leg_rows = [l for g in gens for l in g["legs"]]
    leg_names = []
    for l in leg_rows:
        leg_names += [k for k in l if k not in leg_names]
    legs = {k: [l[k] for l in leg_rows if k in l] for k in leg_names}
    busy = [b for g in gens for b, _ in g["cpu"]]
    steal = [st for g in gens for _, st in g["cpu"]]
    regs = {}
    for g in gens:
        for r, c in g["regs"].items():
            regs[r] = regs.get(r, 0) + c

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
        "legs_n": len(leg_rows),
        "legs_p50_s": {k: round(pct(v, 0.5), 3) for k, v in legs.items()},
        "legs_p95_s": {k: round(pct(v, 0.95), 3) for k, v in legs.items()},
        "accepted_by": dict(sorted(regs.items())),
        "gen_cpu_n": len(busy),
        "gen_cpu_busy_p50": pct(busy, 0.5), "gen_cpu_busy_max": max(busy) if busy else None,
        "gen_cpu_steal_p50": pct(steal, 0.5),
        "validators": dict(sorted(read_validator(v, a.dur) for v in a.validator)),
        "last_fail": next((g["fail_detail"] for g in gens if g["fail_detail"]), ""),
        "verdict": "fail" if why else "pass", "why": "; ".join(why),
    }
    json.dump(row, sys.stdout)
    print()


if __name__ == "__main__":
    main()
