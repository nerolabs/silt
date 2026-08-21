#!/usr/bin/env bash
# lib.sh — shared helpers for the field-test orchestrator + scenarios.
# Sourced, not executed. Targets bash 3.2+ (macOS default) — no associative arrays.

FT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# Backend: gcp (default — real VMs over IAP ssh) or local (LOCAL=1 — docker
# containers on this machine; the SAME scenarios.sh executes, so the graded
# drive logic is exercised before it ever runs against a billable VM).
: "${FT_BACKEND:=$([ "${LOCAL:-0}" = 1 ] && echo local || echo gcp)}"
if [ "$FT_BACKEND" = local ]; then : "${PROJECT_ID:=local}"; fi
: "${PROJECT_ID:?PROJECT_ID must be set (config.env)}"
# State-file NAMESPACE — separate per backend so a LOCAL run can NEVER clobber a
# live cloud run's node map (the 2026-08-20 root cause: LOCAL island verifications
# overwrote a running cloud sheet's nodes.json/topology.json, breaking its ssh_node
# on zone=local and masking the real verdicts). The cloud uses the bare names
# (terraform/report tooling expects them); LOCAL uses .local.json siblings. Every
# reader routes through $NODES_JSON / $FT_TOPO, never a hardcoded path.
if [ "$FT_BACKEND" = local ]; then
  : "${NODES_JSON:=$FT_DIR/nodes.local.json}"
  : "${FT_TOPO:=$FT_DIR/topology.local.json}"
else
  : "${NODES_JSON:=$FT_DIR/nodes.json}"
  : "${FT_TOPO:=$FT_DIR/topology.json}"
fi
export NODES_JSON FT_TOPO FT_BACKEND
: "${RESULTS_JSONL:=$FT_DIR/results.jsonl}"  # one line per SLO check

# ── node metadata (from terraform output) ──────────────────────────────────────
node_field() { # node_field NAME FIELD
  python3 -c "import json,sys;print(json.load(open('$NODES_JSON'))['$1']['$2'])"
}
node_names() { python3 -c "import json;print(' '.join(json.load(open('$NODES_JSON')).keys()))"; }
node_exists() { python3 -c "import json,sys;sys.exit(0 if '$1' in json.load(open('$NODES_JSON')) else 1)" 2>/dev/null; }

# ── remote exec over IAP (reaches natted nodes too; no key management) ──────────
# HARD TIMEOUT (SSH_NODE_TIMEOUT, default 90s): a `gcloud compute ssh` over an IAP
# tunnel has no internal deadline, so a single stalled tunnel blocks the caller
# FOREVER — with no timeout, one stuck stop-in-the-capture-drill wedged a whole
# MATURING run for an hour (run 1ebd487-7457, no verdict, VMs left burning until a
# manual kill). `timeout` bounds every remote call so a stalled node degrades that
# ONE call (the caller's `|| true` / retry then proceeds), never the whole run.
# 124 = timeout's own exit; the caller sees a non-zero exit exactly as a real ssh
# failure, which every ssh_node site already tolerates.
ssh_node() { # ssh_node NAME "remote command"
  local name="$1"; shift
  local inst zone
  inst="$(node_field "$name" instance_name)"
  if [ "$FT_BACKEND" = local ]; then
    # Same funnel, same timeout discipline — docker exec instead of IAP ssh.
    timeout "${SSH_NODE_TIMEOUT:-90}" docker exec "$inst" bash -c "$*" 2>/dev/null
    return
  fi
  zone="$(node_field "$name" zone)"
  timeout "${SSH_NODE_TIMEOUT:-90}" gcloud compute ssh "$inst" --zone "$zone" --project "$PROJECT_ID" \
    --tunnel-through-iap --quiet --command "$*" 2>/dev/null
}

jlog() { ssh_node "$1" "sudo journalctl -u silt --no-pager -n ${2:-400}"; }

# The daemon splits its output: fmt.Printf banners (chain: committed block,
# registry: …, reorged, slashed) go to stdout → journald (read via jlog), but
# every STRUCTURED n.logf(...) line — the per-node signals like `standing self=…
# reputation=N` and `bond challenge peer=… passed=…` — goes to an on-disk file
# (cmd/silt/daemon.go openLog → $STORE/debug.log), NEVER to journald. Assert on
# those via dlog, not jlog. (A #310 assertion greped journald for the standing
# line and could never match — SMOKE caught it.)
dlog() { ssh_node "$1" "sudo tail -n ${2:-1200} /var/lib/silt/debug.log 2>/dev/null"; }

# Poll a node's silt journal until PATTERN appears (or TIMEOUT s). Echoes the
# matching line on success. Field networks are noisy → we assert over logs, and
# never require an exact count.
waitfor() { # waitfor NAME 'EXTENDED_REGEX' TIMEOUT_S
  local name="$1" pat="$2" timeout="${3:-120}" start now line
  start="$(date +%s)"
  while :; do
    line="$(jlog "$name" 800 | grep -E "$pat" | tail -1 || true)"
    [ -n "$line" ] && { printf '%s\n' "$line"; return 0; }
    now="$(date +%s)"; [ $((now - start)) -ge "$timeout" ] && return 1
    sleep 4
  done
}

# Same poll, but SCOPED to journald lines emitted at/after a captured epoch (SINCE).
# Use for restart/crash-recovery assertions where the SAME line ('re-announced N
# held chunks', 'bond: reloaded …') is also emitted on a PRIOR boot and can still
# sit in the last-800-line window — an unscoped waitfor would match the stale
# earlier-boot line and false-pass a restart that actually hung/failed to reload
# (audit #303 cloudtest restart-standing + chaos-reprovide stale-gaps). Capture
# `t0="$(date +%s)"` immediately BEFORE the restart/pkill, pass it as SINCE, and
# only a line emitted by the post-restart boot can satisfy the match.
waitfor_since() { # waitfor_since NAME 'EXTENDED_REGEX' SINCE_EPOCH TIMEOUT_S
  local name="$1" pat="$2" since="$3" timeout="${4:-120}" start now line
  start="$(date +%s)"
  while :; do
    line="$(ssh_node "$name" "sudo journalctl -u silt --no-pager --since \"@${since}\" -n 800" | grep -E "$pat" | tail -1 || true)"
    [ -n "$line" ] && { printf '%s\n' "$line"; return 0; }
    now="$(date +%s)"; [ $((now - start)) -ge "$timeout" ] && return 1
    sleep 4
  done
}

# Same poll, but against the on-disk structured debug.log (see dlog). Use for the
# per-node earned-standing / bond-challenge signals journald never carries.
waitfor_dlog() { # waitfor_dlog NAME 'EXTENDED_REGEX' TIMEOUT_S
  local name="$1" pat="$2" timeout="${3:-120}" start now line
  start="$(date +%s)"
  while :; do
    line="$(dlog "$name" 2000 | grep -E "$pat" | tail -1 || true)"
    [ -n "$line" ] && { printf '%s\n' "$line"; return 0; }
    now="$(date +%s)"; [ $((now - start)) -ge "$timeout" ] && return 1
    sleep 4
  done
}

# ── node control (restart / partition / adversarial relaunch) ──────────────────
svc() { ssh_node "$1" "sudo systemctl $2 silt.service"; }   # svc NAME start|stop|restart

# Append extra flags to a node's silt ExecStart and restart it — used to turn a
# node adversarial (-equivocate/-forge-block) or apply a runtime -denylist without
# re-running Terraform. `restore_argv` puts the baked argv back.
relaunch_with() { # relaunch_with NAME "-extra flags"
  local name="$1" extra="$2"
  ssh_node "$name" "sudo sed -i 's#^ExecStart=/usr/local/bin/silt .*#&#; s#^ExecStart=/usr/local/bin/silt \\(.*\\)\$#ExecStart=/usr/local/bin/silt \\1 ${extra}#' /etc/systemd/system/silt.service && sudo systemctl daemon-reload && sudo systemctl restart silt.service"
  _ensure_silt_active "$name"
}
restore_argv() { # restore_argv NAME  — reset ExecStart to the baked argv
  local name="$1" argv
  if [ "$FT_BACKEND" = local ]; then
    # LOCAL provision bakes the argv to a file (no GCP metadata server here).
    argv="$(ssh_node "$name" 'cat /var/lib/silt-argv.baked')"
  else
    argv="$(ssh_node "$name" 'curl -s -H "Metadata-Flavor: Google" http://metadata.google.internal/computeMetadata/v1/instance/attributes/silt-argv')"
  fi
  ssh_node "$name" "sudo sed -i 's#^ExecStart=.*#ExecStart=/usr/local/bin/silt ${argv}#' /etc/systemd/system/silt.service && sudo systemctl daemon-reload && sudo systemctl restart silt.service"
  _ensure_silt_active "$name"
}
# _ensure_silt_active NAME — verify a relaunch actually came up; retry the start
# if not. A restart can race the OLD instance's port release (the LOCAL shim
# lacked systemd's SIGKILL escalation, so a >10s graceful shutdown left the port
# held): the new daemon died on 'bind: address already in use', nothing
# restarted it, and the driving flow graded a DEAD daemon for its whole window —
# run 577f0f1-27476: 184-forged-block false FAIL + 184-partition false GAP, one
# mechanism, two flows. The shim now escalates (parity fix); this belt covers
# both backends and any other transient boot death. Loud when it can't recover.
_ensure_silt_active() {
  local name="$1" i state
  for i in 1 2 3; do
    sleep 3
    state="$(ssh_node "$name" "systemctl is-active silt.service" 2>/dev/null | tr -d '[:space:]')"
    [ "$state" = active ] && return 0
    ssh_node "$name" "sudo systemctl restart silt.service" >/dev/null 2>&1 || true
  done
  echo "  ⚠ $name: silt.service NOT active after relaunch + 3 retries — the driving flow will mis-grade; read its journal (bind race / bad flag?)"
  return 0
}

# ── result recording (feeds gen_report.sh) ─────────────────────────────────────
_json_str() { python3 -c 'import json,sys;print(json.dumps(sys.argv[1]))' "$1"; }

# ── evidence capture on FAIL/GAP (build-immutable #7) ──────────────────────────
# A scenario-level FAIL/GAP used to leave NO evidence: journals were captured only
# for nodes that never came READY (wait_ready), the console isn't persisted, and
# the network is destroyed right after the run — so run beb3628-95860's two fails
# (9-cross-nat, chaos-reprovide) and its sybil-resume gap could not be attributed
# afterwards, only guessed at. Now every flow's involved nodes are stashed (by
# require_nodes, or explicitly via flow_evidence_nodes for flows that don't use
# it), and record() snapshots their service state + journal + debug.log into
# flow-evidence-$RUN_ID.log AT THE MOMENT a fail/gap verdict lands — while the
# nodes still exist. FT_NO_CAPTURE=1 disables (e.g. when iterating on a KEEP_UP
# network where the journals are still live).
FT_FLOW_NODES=""
flow_evidence_nodes() { FT_FLOW_NODES="$*"; }

capture_flow_evidence() { # capture_flow_evidence FLOW VERDICT — snapshot FT_FLOW_NODES
  [ "${FT_NO_CAPTURE:-0}" = 1 ] && return 0
  [ -n "$FT_FLOW_NODES" ] || return 0
  local out="$FT_DIR/flow-evidence-${RUN_ID:-local}.log" n
  {
    printf '\n######## %s — %s @ %s (nodes: %s)\n' "$1" "$2" "$(date -u +%FT%TZ)" "$FT_FLOW_NODES"
    for n in $FT_FLOW_NODES; do
      node_exists "$n" || continue
      printf '======== %s ========\n-- systemctl status --\n' "$n"
      ssh_node "$n" "sudo systemctl status silt.service --no-pager -l 2>&1 | head -12" || echo "(status unavailable — node unreachable)"
      # Depth (#402): a verdict fires MINUTES after the event that explains it (the
      # fork-committed blocks were ~30 min old when flow 5 graded), so a shallow tail
      # rotates past the cause. Prefer the WHOLE run's journal (--since the boot that
      # started this run's service ≈ run start) with a high-N safety cap; deepen the
      # debug.log tail likewise. FT_CAPTURE_JOURNAL_N / FT_CAPTURE_DLOG_N override.
      echo "-- journalctl -u silt (this boot, ≤${FT_CAPTURE_JOURNAL_N:-4000} lines) --"
      ssh_node "$n" "sudo journalctl -u silt --no-pager -b -n ${FT_CAPTURE_JOURNAL_N:-4000} 2>&1" || echo "(journal unavailable — node unreachable)"
      echo "-- debug.log (last ${FT_CAPTURE_DLOG_N:-2000}) --"
      ssh_node "$n" "sudo tail -n ${FT_CAPTURE_DLOG_N:-2000} /var/lib/silt/debug.log 2>&1" || echo "(no debug.log)"
    done
  } >> "$out" 2>&1
  printf '    (evidence captured → %s)\n' "${out##*/}"
}

record() { # record FLOW VERDICT SEVERITY DETAIL [ELAPSED_S]
  local flow="$1" verdict="$2" sev="$3" detail="$4" elapsed="${5:-}"
  printf '{"flow":%s,"verdict":%s,"severity":%s,"detail":%s,"elapsed_s":%s,"ts":%s}\n' \
    "$(_json_str "$flow")" "$(_json_str "$verdict")" "$(_json_str "$sev")" \
    "$(_json_str "$detail")" "${elapsed:-null}" "$(date +%s)" >> "$RESULTS_JSONL"
  case "$verdict" in
    pass) printf '  \033[32m✓ PASS\033[0m  %s — %s\n' "$flow" "$detail" ;;
    gap)  printf '  \033[33m~ GAP \033[0m  %s — %s\n' "$flow" "$detail" ;;
    skip) printf '  \033[90m- SKIP\033[0m  %s — %s\n' "$flow" "$detail" ;;
    *)    printf '  \033[31m✗ FAIL\033[0m  %s — %s\n' "$flow" "$detail" ;;
  esac
  # Evidence-or-nothing (#7): a non-green verdict grabs its flow's journals NOW,
  # before teardown makes the cause unknowable.
  case "$verdict" in fail|gap) capture_flow_evidence "$flow" "$verdict" ;; esac
}

# scan_node_liveness — the PE's NODE-LIVENESS PRECONDITION (proof-OOM field
# corroboration 2026-08-17). A run whose cohort OOM-crash-loops CANNOT grade its
# flows: a node dying and restarting is indistinguishable from a slow/dead peer, so
# every flow verdict on a crash-looping network is noise. This exact kill signature
# sat UNINSPECTED in the journals for the whole 2026-08-16/17 marathon (the harness
# had NO crash detection), masquerading as "SPOT preemption?" / M1-latency GAPs.
# Scan every node's journal for the kernel OOM-kill / crash signature and surface it
# as a FIRST-CLASS blocker finding, so it can never again hide. Called at run end,
# before teardown (nodes still reachable). The harness REPORTS the crash-loop; it
# must NOT assert a root cause (the first "resident PoR proof map" attribution was
# doubly falsified — #464 shipped and the OOM persisted, and it was a consensus
# node, not a storage one). Attribution belongs to a heap profile (DEBUG_PROFILE=1
# → ./cloudtest.sh heap <node>), not a hard-coded verdict string.
scan_node_liveness() {
  local n oomnodes="" total=0 c
  for n in $(node_names); do
    node_exists "$n" || continue
    # Count REAL crashes by their AUTHORITATIVE signatures over the whole boot journal:
    # the kernel oom-killer ("Out of memory: Killed process", "invoked oom-killer",
    # "oom-kill:constraint=" — RSS blew past the VM) OR any Go runtime fatal error
    # ("fatal error:" — an unrecoverable crash: stack overflow, out of memory, concurrent
    # map write, all-goroutines-asleep deadlock). None of these can be produced by a
    # userspace kill -9. The old scan read the SERVICE journal for `Main process exited,
    # code=killed, status=9` — but status=9 is SIGKILL, which the chaos drill sends
    # DELIBERATELY to store-2 (flow_chaos_crash), so a scripted test kill was miscounted
    # as an OOM and FALSE-FAILED clean runs (run cd1a719-98020: chaos-reprovide PASSED,
    # yet infra-node-liveness counted store-2×1, with no kernel oom-killer line anywhere;
    # cd1a719-26323 same). Matching the real crash signatures instead catches every genuine
    # OOM AND every Go fatal (a load test found a `fatal error: stack overflow` from a
    # provider-resolution recursion the old OOM-only signature would have missed) and never
    # the deliberate chaos SIGKILL. (A real oom-kill DOES also emit status=9, so nothing is lost.)
    c="$(ssh_node "$n" "sudo journalctl -b --no-pager 2>/dev/null | grep -cE 'Out of memory: Killed process|invoked oom-killer|oom-kill:constraint|fatal error:'" 2>/dev/null | tr -dc '0-9')"
    c="${c:-0}"
    [ "$c" -gt 0 ] 2>/dev/null && { oomnodes="$oomnodes ${n}×${c}"; total=$((total + c)); }
  done
  if [ -n "$oomnodes" ]; then
    record "infra-node-liveness" fail blocker "NODE CRASH-LOOP — ${total} node crash(es) (kernel OOM-kill or Go fatal error) across:${oomnodes}. A run whose cohort DIES cannot grade its flows (a crashing node is indistinguishable from a slow/dead peer), so EVERY verdict on this sheet is PROVISIONAL until a clean no-crash re-run — including the computed bounds (which may be inflated). This is INFRASTRUCTURE FAILURE, not independent flow results. Attribute from a heap profile (re-run with DEBUG_PROFILE=1, then ./cloudtest.sh heap <node>) or the node's journal (fatal-error stack trace) — do NOT presume a cause."
  else
    record "infra-node-liveness" pass blocker "node-liveness precondition HELD — no OOM-kill or crash-loop across the cohort, so the sheet was graded on a HEALTHY network"
  fi
}

# ── RSS/memory telemetry (Phase 1.3 — build-immutable #7 on our OWN headlines) ──
# scan_node_liveness answers "did it crash?" (a binary journal signal). It does NOT
# answer "what was the memory ENVELOPE?" — the shape the MATURING OOM headline claims
# (RSS climbs, peaks, returns to a bounded ~2GB plateau, never OOM-kills). That is a
# time-series question, and until now NO committed artifact backed it: return-to-2GB
# rested on the ABSENCE of a crash, not a MEASURED ceiling. These helpers sample each
# node's cgroup memory across the run into a committed rss-<RUN_ID>.jsonl and summarise
# it to peak/final per node, so the claim becomes a citable file + a number.
# Design + options: docs/thinking/2026-08-19-cloudtest-rss-telemetry.md.

MEM_SERIES="${MEM_SERIES:-$FT_DIR/rss-${RUN_ID:-local}.jsonl}"
: "${MEM_SAMPLE_INTERVAL:=30}"   # seconds between sweeps (coarse — an envelope, not a profiler)

# node_mem_bytes NODE — the cgroup's live accounted memory (systemd MemoryCurrent), the
# exact quantity GOMEMLIMIT and the OOM-killer act against. Empty on any failure (never
# fatal). MemoryCurrent is "[not set]"/"" before the service is up; callers skip blanks.
node_mem_bytes() {
  ssh_node "$1" "systemctl show silt.service -p MemoryCurrent --value 2>/dev/null" \
    2>/dev/null | tr -dc '0-9'
}

# mem_sampler_start — background loop: one JSONL row per node per sweep. Writes its PID
# to $MEM_SAMPLER_PIDFILE so mem_sampler_stop can end it. Strictly additive and
# failure-tolerant: a missed read is simply skipped, and the sampler never touches a
# verdict or the run's exit status.
MEM_SAMPLER_PIDFILE="$FT_DIR/.mem-sampler.pid"
mem_sampler_start() {
  [ "${MEM_SAMPLE:-1}" = 1 ] || { echo "mem-sampler: disabled (MEM_SAMPLE=0)"; return 0; }
  : > "$MEM_SERIES"
  ( while :; do
      for n in $(node_names); do
        b="$(node_mem_bytes "$n")"
        [ -n "$b" ] && printf '{"node":%s,"rss_bytes":%s,"ts":%s}\n' \
          "$(_json_str "$n")" "$b" "$(date +%s)" >> "$MEM_SERIES"
      done
      sleep "$MEM_SAMPLE_INTERVAL"
    done ) &
  echo $! > "$MEM_SAMPLER_PIDFILE"
  echo "mem-sampler: started (pid $(cat "$MEM_SAMPLER_PIDFILE"), every ${MEM_SAMPLE_INTERVAL}s → $(basename "$MEM_SERIES"))"
}
mem_sampler_stop() {
  [ -f "$MEM_SAMPLER_PIDFILE" ] || return 0
  kill "$(cat "$MEM_SAMPLER_PIDFILE")" 2>/dev/null || true
  rm -f "$MEM_SAMPLER_PIDFILE"
}

# scan_node_memory — summarise rss-<RUN_ID>.jsonl into a per-node peak/final envelope
# and record it as a first-class finding, so the memory headline is citable. Purely
# observational (S5): it reports the measured ceiling; it does NOT assert pass/fail on a
# memory number (the OOM-killer / scan_node_liveness owns the crash verdict). Called at
# run end, before teardown, after mem_sampler_stop.
scan_node_memory() {
  if [ ! -s "$MEM_SERIES" ]; then
    record "infra-node-memory" skip info "no RSS samples captured (MEM_SAMPLE=0, or the network never came up) — memory envelope UNMEASURED this run"
    return 0
  fi
  local summary
  summary="$(python3 - "$MEM_SERIES" <<'PY'
import json, sys
peak, final, count = {}, {}, {}
for line in open(sys.argv[1]):
    line = line.strip()
    if not line:
        continue
    try:
        r = json.loads(line)
    except Exception:
        continue
    n, b = r["node"], int(r["rss_bytes"])
    peak[n] = max(peak.get(n, 0), b)
    final[n] = b            # last row wins (file is append-order = time-order)
    count[n] = count.get(n, 0) + 1
gib = 1 << 30
worst = max(peak.values()) if peak else 0
parts = []
for n in sorted(peak):
    parts.append("%s peak=%.2fGiB final=%.2fGiB n=%d" % (n, peak[n]/gib, final[n]/gib, count[n]))
print("%.2f" % (worst/gib))              # line 1: worst peak GiB (for the caller)
print("; ".join(parts))                  # line 2: per-node detail
PY
)" || { record "infra-node-memory" skip info "RSS series present but summary failed to parse"; return 0; }
  local worst detail
  worst="$(printf '%s\n' "$summary" | sed -n '1p')"
  detail="$(printf '%s\n' "$summary" | sed -n '2p')"
  record "infra-node-memory" pass info "RSS envelope measured (cgroup MemoryCurrent, every ${MEM_SAMPLE_INTERVAL}s → $(basename "$MEM_SERIES")): worst peak ${worst}GiB across the cohort. ${detail}"
}

# run_flow FN — dispatch one scenario function, honoring the FLOWS= re-drive filter:
# FLOWS unset/empty runs everything (the graded sheet); FLOWS="11-economy-repair
# chaos" (space/comma separated, matched as substring against the FUNCTION name or
# its flow ids) re-runs only the named flows against a standing network — the
# selective re-drive loop (TEARDOWN=0 / KEEP_UP=1 + ./cloudtest.sh run). A
# re-driven subset is a CONVERGENCE aid, never a grade: the exit-gate/RC artifact
# stays one clean uninterrupted sheet (docs/thinking/2026-08-20-harness-local-first.md).
run_flow() {
  local fn="$1" f
  [ -z "${FLOWS:-}" ] && { "$fn"; return; }
  for f in $(printf '%s' "$FLOWS" | tr ',' ' '); do
    case "$fn" in *"$f"*) "$fn"; return ;; esac
    # Also match against the flow ids the function records (e.g. 11-economy-repair
    # → flow_economy_repair): normalize digits/dashes away and compare substrings.
    case "$(printf '%s' "$fn" | tr '_' '-')" in *"$(printf '%s' "$f" | sed 's/^[0-9]*[a-z]*-//')"*) "$fn"; return ;; esac
  done
  echo "  · skipped by FLOWS filter: $fn"
}

# require_nodes FLOW SEVERITY NODE...  — record skip + return 1 if any node is absent
# (so SMOKE=1 / trimmed topologies don't false-fail scenarios they can't run).
# Also stashes the node list for capture_flow_evidence: the nodes a flow REQUIRES
# are the nodes whose journals attribute its failure.
require_nodes() {
  local flow="$1" sev="$2"; shift 2
  local n
  FT_FLOW_NODES="$*"
  for n in "$@"; do
    if ! node_exists "$n"; then record "$flow" skip "$sev" "skipped — node '$n' not in this topology"; return 1; fi
  done
  return 0
}

# require_live FLOW SEVERITY NODE...  — record GAP + return 1 if any required node's
# silt.service is not active. A node the substrate killed (SPOT preemption) is not a
# property failure: the flow is UNTESTED, so it must GAP, never FAIL (the 2026-08-12
# blind SMOKE false-FAILed web-ui-guard/publisher-unlinkability with empty output
# because their node was preempted). Mirrors the is-active probe flow_first_run uses.
require_live() {
  local flow="$1" sev="$2"; shift 2
  local n down=""
  FT_FLOW_NODES="$*"
  for n in "$@"; do
    if ! ssh_node "$n" "systemctl is-active --quiet silt.service"; then down="$down $n"; fi
  done
  if [ -n "$down" ]; then
    record "$flow" gap "$sev" "prerequisite node(s) not active (down:$down) — substrate/preemption, property UNTESTED not failed"
    return 1
  fi
  return 0
}

# require_link FLOW SEVERITY  — record GAP + return 1 if no prior publish landed a
# link this run (FT_LAST_LINK empty). A flow whose SETUP publish never produced a link
# tested nothing; it must GAP (as 8-takedown already does), not FAIL with
# `want=? got=<none>` (the 2026-08-12 7-restart-content cascade false-FAIL).
require_link() {
  if [ -z "${FT_LAST_LINK:-}" ]; then
    record "$1" gap "$2" "no prior published link (upstream publish did not land) — property UNTESTED not failed"
    return 1
  fi
  return 0
}

# slo_assert FLOW SEVERITY DETAIL  — call after setting `ok` (0/1) and `elapsed`.
slo_assert() { # slo_assert FLOW SEVERITY "detail" OK ELAPSED
  local flow="$1" sev="$2" detail="$3" ok="$4" elapsed="${5:-}"
  if [ "$ok" = 1 ]; then record "$flow" pass "$sev" "$detail" "$elapsed"
  else record "$flow" fail "$sev" "$detail" "$elapsed"; fi
}
