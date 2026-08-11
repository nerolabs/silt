#!/usr/bin/env bash
# lib.sh (two-cloud) — the UNIFIED substrate layer. Its ssh_node reads each node's
# `cloud` field from nodes.json and routes to the right provider (GCP IAP SSH or AWS
# SSM), so the SHARED ../cloudtest/scenarios.sh runs UNCHANGED across a topology split
# over two clouds — a flow whose two endpoints live on different providers never knows.
# This router is the one genuinely-new piece of the two-cloud harness; the flows,
# result recording, and per-cloud Terraform are all reused.
# Sourced, not executed. bash 3.2+ (no associative arrays).

FT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
: "${NODES_JSON:=$FT_DIR/nodes.json}"        # combined map: each node carries a `cloud` field
: "${RESULTS_JSONL:=$FT_DIR/results.jsonl}"
: "${PROJECT_ID:?PROJECT_ID must be set (config.env — the GCP half)}"
: "${AWS_REGION:?AWS_REGION must be set (config.env — the AWS half)}"

# ── node metadata — SUBSTRATE-AGNOSTIC (identical to the single-cloud harnesses) ──
node_field() { python3 -c "import json,sys;print(json.load(open('$NODES_JSON'))['$1']['$2'])"; }
node_names() { python3 -c "import json;print(' '.join(json.load(open('$NODES_JSON')).keys()))"; }
node_exists() { python3 -c "import json,sys;sys.exit(0 if '$1' in json.load(open('$NODES_JSON')) else 1)" 2>/dev/null; }
node_cloud() { node_field "$1" cloud; }   # "gcp" | "aws"

# ── the router: exec a command on NODE, on whichever cloud hosts it ──────────────
_ssh_gcp() { # _ssh_gcp NAME "cmd"  (GCP IAP — same as cloudtest/lib.sh)
  local name="$1"; shift
  local inst zone
  inst="$(node_field "$name" instance_name)"; zone="$(node_field "$name" zone)"
  gcloud compute ssh "$inst" --zone "$zone" --project "$PROJECT_ID" \
    --tunnel-through-iap --quiet --command "$*" 2>/dev/null
}
_ssh_aws() { # _ssh_aws NAME "cmd"  (AWS SSM — same as awstest/lib.sh, base64 to dodge quoting)
  local name="$1"; shift
  local iid b64 cid st i
  iid="$(node_field "$name" instance_id)" || return 1
  b64="$(printf '%s' "$*" | base64 | tr -d '\n')"
  cid="$(aws ssm send-command --region "$AWS_REGION" --instance-ids "$iid" \
    --document-name AWS-RunShellScript \
    --parameters "{\"commands\":[\"echo $b64 | base64 -d | bash\"]}" \
    --query 'Command.CommandId' --output text 2>/dev/null)" || return 1
  [ -n "$cid" ] && [ "$cid" != None ] || return 1
  for i in $(seq 1 90); do
    st="$(aws ssm get-command-invocation --region "$AWS_REGION" --command-id "$cid" \
      --instance-id "$iid" --query 'Status' --output text 2>/dev/null || true)"
    case "$st" in Success | Failed | Cancelled | TimedOut) break ;; esac
    sleep 2
  done
  aws ssm get-command-invocation --region "$AWS_REGION" --command-id "$cid" \
    --instance-id "$iid" --query 'StandardOutputContent' --output text 2>/dev/null
}
ssh_node() { # ssh_node NAME "remote command" — dispatch by the node's cloud
  local name="$1"; shift
  case "$(node_cloud "$name")" in
    gcp) _ssh_gcp "$name" "$@" ;;
    aws) _ssh_aws "$name" "$@" ;;
    *)   echo "ssh_node: node '$name' has no known cloud" >&2; return 1 ;;
  esac
}

# ── everything below is substrate-AGNOSTIC (built on the router above) ───────────
jlog() { ssh_node "$1" "sudo journalctl -u silt --no-pager -n ${2:-400}"; }
dlog() { ssh_node "$1" "sudo tail -n ${2:-1200} /var/lib/silt/debug.log 2>/dev/null"; }

waitfor() { # waitfor NAME 'REGEX' TIMEOUT_S — poll journald
  local name="$1" pat="$2" timeout="${3:-120}" start now line
  start="$(date +%s)"
  while :; do
    line="$(jlog "$name" 800 | grep -E "$pat" | tail -1 || true)"
    [ -n "$line" ] && { printf '%s\n' "$line"; return 0; }
    now="$(date +%s)"; [ $((now - start)) -ge "$timeout" ] && return 1
    sleep 4
  done
}
waitfor_dlog() { # waitfor_dlog NAME 'REGEX' TIMEOUT_S — poll the on-disk debug.log
  local name="$1" pat="$2" timeout="${3:-120}" start now line
  start="$(date +%s)"
  while :; do
    line="$(dlog "$name" 2000 | grep -E "$pat" | tail -1 || true)"
    [ -n "$line" ] && { printf '%s\n' "$line"; return 0; }
    now="$(date +%s)"; [ $((now - start)) -ge "$timeout" ] && return 1
    sleep 4
  done
}

svc() { ssh_node "$1" "sudo systemctl $2 silt.service"; }
relaunch_with() { # relaunch_with NAME "-extra flags"
  local name="$1" extra="$2"
  ssh_node "$name" "sudo sed -i 's#^ExecStart=/usr/local/bin/silt \\(.*\\)\$#ExecStart=/usr/local/bin/silt \\1 ${extra}#' /etc/systemd/system/silt.service && sudo systemctl daemon-reload && sudo systemctl restart silt.service"
}
restore_argv() { # restore_argv NAME — reset ExecStart to the baked argv.
  # Both clouds' provisioning persist the argv to /etc/silt/argv, so one path works
  # regardless of which cloud hosts the node (AWS writes it; GCP's silt-startup can too).
  local name="$1"
  ssh_node "$name" 'sudo sed -i "s#^ExecStart=.*#ExecStart=/usr/local/bin/silt $(cat /etc/silt/argv)#" /etc/systemd/system/silt.service && sudo systemctl daemon-reload && sudo systemctl restart silt.service'
}

# ── result recording (feeds ../cloudtest/gen_report.sh) — SUBSTRATE-AGNOSTIC ─────
_json_str() { python3 -c 'import json,sys;print(json.dumps(sys.argv[1]))' "$1"; }
record() { # record FLOW VERDICT SEVERITY DETAIL [ELAPSED_S]
  local flow="$1" verdict="$2" sev="$3" detail="$4" elapsed="${5:-}"
  printf '{"flow":%s,"verdict":%s,"severity":%s,"detail":%s,"elapsed_s":%s,"ts":%s}\n' \
    "$(_json_str "$flow")" "$(_json_str "$verdict")" "$(_json_str "$sev")" \
    "$(_json_str "$detail")" "${elapsed:-null}" "$(date +%s)" >>"$RESULTS_JSONL"
  case "$verdict" in
    pass) printf '  \033[32m✓ PASS\033[0m  %s — %s\n' "$flow" "$detail" ;;
    gap)  printf '  \033[33m~ GAP \033[0m  %s — %s\n' "$flow" "$detail" ;;
    skip) printf '  \033[90m- SKIP\033[0m  %s — %s\n' "$flow" "$detail" ;;
    *)    printf '  \033[31m✗ FAIL\033[0m  %s — %s\n' "$flow" "$detail" ;;
  esac
}
require_nodes() { # require_nodes FLOW SEVERITY NODE...
  local flow="$1" sev="$2"; shift 2
  local n
  for n in "$@"; do
    if ! node_exists "$n"; then record "$flow" skip "$sev" "skipped — node '$n' not in this topology"; return 1; fi
  done
  return 0
}
slo_assert() { # slo_assert FLOW SEVERITY DETAIL OK ELAPSED
  local flow="$1" sev="$2" detail="$3" ok="$4" elapsed="${5:-}"
  if [ "$ok" = 1 ]; then record "$flow" pass "$sev" "$detail" "$elapsed"
  else record "$flow" fail "$sev" "$detail" "$elapsed"; fi
}
