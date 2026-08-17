#!/usr/bin/env bash
# lib.sh — shared helpers for the field-test orchestrator + scenarios.
# Sourced, not executed. Targets bash 3.2+ (macOS default) — no associative arrays.

FT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
: "${NODES_JSON:=$FT_DIR/nodes.json}"        # terraform output -json nodes, written by cloudtest.sh
: "${RESULTS_JSONL:=$FT_DIR/results.jsonl}"  # one line per SLO check
: "${PROJECT_ID:?PROJECT_ID must be set (config.env)}"

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
  inst="$(node_field "$name" instance_name)"; zone="$(node_field "$name" zone)"
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
}
restore_argv() { # restore_argv NAME  — reset ExecStart to the baked metadata argv
  local name="$1" argv
  argv="$(ssh_node "$name" 'curl -s -H "Metadata-Flavor: Google" http://metadata.google.internal/computeMetadata/v1/instance/attributes/silt-argv')"
  ssh_node "$name" "sudo sed -i 's#^ExecStart=.*#ExecStart=/usr/local/bin/silt ${argv}#' /etc/systemd/system/silt.service && sudo systemctl daemon-reload && sudo systemctl restart silt.service"
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
    c="$(ssh_node "$n" "sudo journalctl -u silt --no-pager -b 2>/dev/null | grep -cE 'oom-kill|killed by the OOM killer|Main process exited, code=killed, status=9'" 2>/dev/null | tr -dc '0-9')"
    c="${c:-0}"
    [ "$c" -gt 0 ] 2>/dev/null && { oomnodes="$oomnodes ${n}×${c}"; total=$((total + c)); }
  done
  if [ -n "$oomnodes" ]; then
    record "infra-node-liveness" fail blocker "NODE CRASH-LOOP — ${total} kernel kill(s) (OOM/signal) across:${oomnodes}. A run whose cohort DIES cannot grade its flows (a crashing node is indistinguishable from a slow/dead peer), so EVERY verdict on this sheet is PROVISIONAL until a clean no-OOM re-run — including the computed bounds (which may be OOM-inflated). This is INFRASTRUCTURE FAILURE, not independent flow results. Attribute from a heap profile (re-run with DEBUG_PROFILE=1, then ./cloudtest.sh heap <node>) — do NOT presume a cause."
  else
    record "infra-node-liveness" pass blocker "node-liveness precondition HELD — no OOM-kill or crash-loop across the cohort, so the sheet was graded on a HEALTHY network"
  fi
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
