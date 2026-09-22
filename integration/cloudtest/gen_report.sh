#!/usr/bin/env bash
# gen_report.sh — turn results.jsonl into a shareable report (Markdown + a
# self-contained HTML file). This is the artifact an outside developer hands back
# after running the field test: a per-flow verdict table, then every finding.
set -euo pipefail
FT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
RESULTS="${RESULTS_JSONL:-$FT_DIR/results.jsonl}"
OUT_MD="${1:-$FT_DIR/report.md}"
OUT_HTML="${2:-$FT_DIR/report.html}"

# BOTH COMMITS COME FROM STAMPS WRITTEN WHEN THE INPUT WAS CONSUMED, never from
# live HEAD. Reading HEAD here dated the evidence to report time: a run takes about
# an hour, and run d531fbf-90914 built its fleet binary at d531fbf while this line
# printed a0ff08a, because two commits landed mid-run. The run id was the only
# honest field on the page. cloudtest.sh writes .run-silt-sha at build_binary and
# .run-harness-sha/.run-harness-digest at the top of run_scenarios.
#
# The fallback is LOUD rather than silent. A missing stamp means gen_report.sh was
# invoked outside a run, so HEAD is a guess about which binary produced these rows
# and the header says so instead of presenting a guess as provenance.
read_stamp() { [ -s "$FT_DIR/$1" ] && tr -d '[:space:]' < "$FT_DIR/$1"; }

SILT_COMMIT="$(read_stamp .run-silt-sha || true)"
[ -n "$SILT_COMMIT" ] || SILT_COMMIT="$(git -C "$FT_DIR" rev-parse --short HEAD 2>/dev/null || echo unknown) (UNPINNED — no build stamp; this is HEAD now, not necessarily what ran)"

# Harness commit, stamped SEPARATELY: a harness-only re-drive against the same
# binaries is honest exactly when it is attributable as that — the last commit
# touching integration/cloudtest names which drive logic graded this sheet.
HARNESS_COMMIT="$(read_stamp .run-harness-sha || true)"
[ -n "$HARNESS_COMMIT" ] || HARNESS_COMMIT="$(git -C "$FT_DIR" log -1 --format=%h -- . 2>/dev/null || echo unknown) (UNPINNED)"

# THE MID-RUN EDIT CHECK. A sha cannot see an uncommitted change, and the failure
# that motivated this was exactly that: scenarios.sh edited in the working tree six
# minutes before the first flow graded, with the commit landing later still. So the
# digest taken at grading time is compared with the files as they stand now, and a
# difference is reported on the page rather than left for someone to reconstruct
# from mtimes. It does not invalidate the sheet; it says the sheet was graded by
# something other than what is on disk, which is a thing a reader must be told.
HARNESS_DRIFT=""
if [ -s "$FT_DIR/.run-harness-digest" ]; then
  _at_grade="$(tr -d '[:space:]' < "$FT_DIR/.run-harness-digest")"
  _now="$(cat "$FT_DIR/scenarios.sh" "$FT_DIR/lib.sh" 2>/dev/null | shasum -a 256 | cut -c1-16)"
  [ "$_at_grade" = "$_now" ] || HARNESS_DRIFT="⚠ THE GRADING HARNESS CHANGED AFTER THIS SHEET WAS GRADED — scenarios.sh+lib.sh digested ${_at_grade} when the first flow ran and ${_now} now. The verdicts below came from the FORMER. Re-drive before citing a row whose flow was edited."
fi
RUN_ID="${RUN_ID:-unknown}"
BOND_MODE="$(python3 -c "import json;print(json.load(open('$FT_DIR/topology.json'))['meta']['bond_mode'])" 2>/dev/null || echo unknown)"
GEN_TS="$(date -u +%Y-%m-%dT%H:%M:%SZ)"

python3 - "$RESULTS" "$OUT_MD" "$OUT_HTML" "$SILT_COMMIT" "$RUN_ID" "$BOND_MODE" "$GEN_TS" "$HARNESS_COMMIT" "$HARNESS_DRIFT" <<'PY'
import json, sys, html
results_path, out_md, out_html, commit, run_id, bond_mode, gen_ts, harness, drift = sys.argv[1:10]

rows = []
try:
    for line in open(results_path):
        line = line.strip()
        if line:
            rows.append(json.loads(line))
except FileNotFoundError:
    pass

order = ["pass", "gap", "fail", "skip"]
counts = {k: sum(1 for r in rows if r["verdict"] == k) for k in order}
graded = [r for r in rows if r["verdict"] != "skip"]
# A `gap` is a shortfall (couldn't confirm the property), not a pass — per field-test
# immutable #4 it must NOT roll up to a green PASS. Only an all-pass graded run is PASS;
# any gap downgrades to REVIEW (amber); any fail is FAIL.
if not graded:
    overall = "NO RESULTS"
elif counts["fail"]:
    overall = "FAIL"
elif counts["gap"]:
    overall = "REVIEW"
else:
    overall = "PASS"
sev_rank = {"blocker": 0, "major": 1, "minor": 2, "cosmetic": 3}

# ── Markdown ────────────────────────────────────────────────────────────────
md = []
md.append(f"# silt field-test report\n")
md.append(f"- **run:** `{run_id}`  ·  **silt commit:** `{commit}`  ·  **harness commit:** `{harness}`  ·  **bond mode:** `{bond_mode}`  ·  **generated:** {gen_ts}")
if drift:
    md.append("")
    md.append(f"> {drift}")
md.append(f"- **result:** **{overall}**  ·  {counts['pass']} pass / {counts['gap']} gap / {counts['fail']} fail / {counts['skip']} skip\n")
md.append("## Per-flow verdict\n")
md.append("| flow | verdict | severity | elapsed | detail |")
md.append("|------|---------|----------|---------|--------|")
def badge(v): return {"pass":"✅ pass","gap":"⚠️ gap","fail":"❌ fail","skip":"➖ skip"}.get(v, v)
for r in sorted(rows, key=lambda r: r["flow"]):
    el = f"{r['elapsed_s']}s" if r.get("elapsed_s") not in (None, "null") else ""
    md.append(f"| `{r['flow']}` | {badge(r['verdict'])} | {r.get('severity','')} | {el} | {r['detail']} |")
md.append("")
findings = [r for r in rows if r["verdict"] != "pass"]
if findings:
    md.append("## Findings (gaps + failures), most severe first\n")
    for r in sorted(findings, key=lambda r: (sev_rank.get(r.get('severity'),9), r['flow'])):
        md.append(f"### {r['flow']} — {badge(r['verdict'])} ({r.get('severity','')})")
        md.append(f"{r['detail']}\n")
else:
    md.append("_No gaps or failures recorded._\n")
md.append("---\n")
md.append("_Generated by `integration/cloudtest`. The field network is ephemeral and was torn down after this run._")
open(out_md, "w").write("\n".join(md))

# ── self-contained HTML ─────────────────────────────────────────────────────
def h(s): return html.escape(str(s))
color = {"pass":"#1a7f37","gap":"#9a6700","fail":"#cf222e","skip":"#57606a"}
trows = []
for r in sorted(rows, key=lambda r: r["flow"]):
    el = f"{r['elapsed_s']}s" if r.get("elapsed_s") not in (None,"null") else ""
    trows.append(f"<tr><td><code>{h(r['flow'])}</code></td>"
                 f"<td style='color:{color.get(r['verdict'],'#333')};font-weight:600'>{h(r['verdict']).upper()}</td>"
                 f"<td>{h(r.get('severity',''))}</td><td>{h(el)}</td><td>{h(r['detail'])}</td></tr>")
ocolor = {"PASS":"#1a7f37","REVIEW":"#9a6700","FAIL":"#cf222e","NO RESULTS":"#57606a"}[overall]
drift_html = (f"<p style='border-left:4px solid #9a6700;background:#fff8c5;padding:.5rem .8rem;margin:.6rem 0'>{h(drift)}</p>"
              if drift else "")
doc = f"""<!doctype html><html><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>silt field-test report — {h(run_id)}</title>
<style>
 body{{font:15px/1.5 -apple-system,Segoe UI,Roboto,sans-serif;max-width:960px;margin:2rem auto;padding:0 1rem;color:#1f2328}}
 h1{{margin-bottom:.2rem}} .meta{{color:#57606a;font-size:13px}}
 .result{{display:inline-block;padding:.2rem .6rem;border-radius:6px;color:#fff;background:{ocolor};font-weight:700}}
 table{{border-collapse:collapse;width:100%;margin:1rem 0;font-size:14px}}
 th,td{{border:1px solid #d0d7de;padding:.4rem.6rem;text-align:left;vertical-align:top}}
 th{{background:#f6f8fa}} code{{background:#f6f8fa;padding:.05rem .3rem;border-radius:4px}}
 .tally span{{margin-right:1rem;font-weight:600}}
</style></head><body>
<h1>silt field-test report</h1>
<p class="meta">run <code>{h(run_id)}</code> · silt commit <code>{h(commit)}</code> · harness <code>{h(harness)}</code> · bond mode <code>{h(bond_mode)}</code> · {h(gen_ts)}</p>
{drift_html}
<p><span class="result">{overall}</span></p>
<p class="tally"><span style="color:#1a7f37">{counts['pass']} pass</span>
<span style="color:#9a6700">{counts['gap']} gap</span>
<span style="color:#cf222e">{counts['fail']} fail</span>
<span style="color:#57606a">{counts['skip']} skip</span></p>
<table><thead><tr><th>flow</th><th>verdict</th><th>severity</th><th>elapsed</th><th>detail</th></tr></thead>
<tbody>{''.join(trows)}</tbody></table>
<p class="meta">Generated by <code>integration/cloudtest</code>. The field network is ephemeral and was torn down after this run.</p>
</body></html>"""
open(out_html, "w").write(doc)
print(f"report: {overall} — {counts['pass']} pass / {counts['gap']} gap / {counts['fail']} fail")
print(f"  {out_md}")
print(f"  {out_html}")
PY
