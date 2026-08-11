#!/usr/bin/env bash
# twocloud.sh — SKELETON orchestrator for the split GCP+AWS field test. It composes the
# GCP (cloudtest) and AWS (awstest) harnesses and runs the SAME ../cloudtest/scenarios.sh
# against the combined fleet, dispatched per-node by the unified lib.sh router.
#
# STATUS: SCAFFOLD. The phase structure, the split, the shared-scenario wiring, and the
# teardown-both are here; the phases marked TODO (phase-0 public-IP reservation + the
# argv rebind + the per-cloud terraform apply with node subsets) need both substrates
# certified and real creds — see README §3/§6. Running it today stops at the first TODO.
set -euo pipefail
FT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$FT_DIR/../.." && pwd)"
GCP_DIR="$FT_DIR/../cloudtest"
AWS_DIR="$FT_DIR/../awstest"
cd "$FT_DIR"

[ -f config.env ] || { echo "no config.env — copy config.env.example and fill in BOTH clouds"; exit 1; }
# shellcheck disable=SC1091
. ./config.env
: "${PROJECT_ID:?set PROJECT_ID (GCP) in config.env}"
: "${AWS_REGION:=us-west-2}"
export PROJECT_ID AWS_REGION
: "${RUN_ID:=$(git -C "$REPO_ROOT" rev-parse --short HEAD 2>/dev/null || echo dev)-$$}"
RUN_ID="$(printf '%s' "$RUN_ID" | tr -c 'a-z0-9-' '-' | cut -c1-24)"
export RUN_ID

need() { command -v "$1" >/dev/null 2>&1 || { echo "missing prerequisite: $1"; exit 1; }; }
check_prereqs() {
  need terraform; need go; need python3; need base64
  need gcloud                                   # GCP half
  need aws                                       # AWS half
  command -v session-manager-plugin >/dev/null 2>&1 || echo "  ! session-manager-plugin missing (AWS SSM exec)"
}

build_binary() {
  echo "==> building silt (linux/amd64) @ $RUN_ID"
  ( cd "$REPO_ROOT" && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags '-s -w' -o "$FT_DIR/silt-linux-amd64" ./cmd/silt )
  ( cd "$REPO_ROOT" && go build -o "$FT_DIR/.silt-local" ./cmd/silt )
}

gen_topology() {
  echo "==> generating split topology (GCP + AWS)"
  SILT_BIN="$FT_DIR/.silt-local" BOND_MODE="${BOND_MODE:-fast}" SMOKE="${SMOKE:-0}" SPLIT="${SPLIT:-}" python3 topology.py
}

# ── PHASE 0 — reserve a static PUBLIC IP per dialable node on each cloud ──────────
# TODO: add a small IP-reservation stack to cloudtest/terraform (google_compute_address)
# and awstest/terraform (aws_eip), applied here for THIS run's dialable nodes, then read
# the addresses back into $FT_DIR/reservations.json { name: public_ip }. Natted nodes get
# none. This is what lets phase 1 attach a KNOWN public IP and the argv reference it.
reserve_ips() {
  echo "==> phase 0: reserve cross-provider public IPs  [TODO — see README §3]"
  echo "    (google_compute_address on GCP + aws_eip on AWS for each dialable node)"
  return 1   # scaffold stop
}

# ── REBIND — bake the argv against the reserved public IPs, write per-cloud tfvars ─
# TODO: with reservations.json in hand, render each node's silt argv exactly as the
# single-cloud topology.py does (identical logic) but with every -advertise/-bootstrap/
# -anchors/-attesters/-relay-via/regref using the PUBLIC IP. Emit gcp/terraform tfvars
# (var.nodes = the GCP subset) and aws/terraform tfvars (the AWS subset).
rebind_argv() {
  echo "==> rebind: bake argv against reserved public IPs → per-cloud tfvars  [TODO]"
  return 1
}

# ── PHASE 1 — apply BOTH clouds' subsets in parallel, using their existing Terraform ─
apply_both() {
  echo "==> phase 1: terraform apply — GCP subset + AWS subset  [TODO — wire subsets]"
  echo "    GCP:  terraform -chdir=$GCP_DIR/terraform apply … (var.nodes = gcp subset)"
  echo "    AWS:  terraform -chdir=$AWS_DIR/terraform apply … (var.nodes = aws subset)"
  echo "    then merge both 'terraform output -json nodes' + each node's cloud into $FT_DIR/nodes.json"
  return 1
}

wait_ready() {
  echo "==> waiting for silt.service on every node (router dispatches per cloud)"
  # shellcheck disable=SC1091
  . ./lib.sh
  local n deadline=$(( $(date +%s) + 600 )) pending
  while :; do
    pending=""
    for n in $(node_names); do ssh_node "$n" "systemctl is-active --quiet silt.service" >/dev/null 2>&1 || pending="$pending $n"; done
    [ -z "$pending" ] && { echo "    all nodes ready"; return 0; }
    [ "$(date +%s)" -ge "$deadline" ] && { echo "    timed out:$pending"; return 1; }
    echo "    still starting:$pending"; sleep 20
  done
}

# ── RUN — the SAME flows, dispatched across both clouds by the router (DONE wiring) ─
run_scenarios() {
  echo "==> running scenarios (shared ../cloudtest/scenarios.sh, split across clouds)"
  : > "$FT_DIR/results.jsonl"
  # shellcheck disable=SC1091
  . ./lib.sh                     # the UNIFIED router
  # shellcheck disable=SC1091
  . "$GCP_DIR/scenarios.sh"      # the SAME flows, unchanged
  run_all_scenarios || true
}
report() { echo "==> report"; RUN_ID="$RUN_ID" "$GCP_DIR/gen_report.sh"; }

# ── TEARDOWN — destroy BOTH stacks + release reserved IPs on both clouds ──────────
teardown() {
  [ "${KEEP_UP:-0}" = 1 ] && { echo "==> KEEP_UP=1 — leaving both fleets up"; return; }
  echo "==> DESTROY both clouds (run=$RUN_ID)"
  ( cd "$GCP_DIR" && RUN_ID="$RUN_ID" KEEP_UP=0 ./cloudtest.sh down ) || echo "  ! GCP teardown needs attention — ./cloudtest.sh nuke"
  ( cd "$AWS_DIR" && RUN_ID="$RUN_ID" KEEP_UP=0 ./awstest.sh down )  || echo "  ! AWS teardown needs attention — ./awstest.sh nuke"
  echo "  reminder: release the reserved static IPs/EIPs on both clouds (phase-0 stack)."
}

case "${1:-all}" in
  all)
    check_prereqs; build_binary; gen_topology
    trap teardown EXIT
    reserve_ips; rebind_argv; apply_both; wait_ready; run_scenarios; report
    ;;
  topology) build_binary; gen_topology ;;   # the runnable-today part: emit the split
  run)      run_scenarios; report ;;
  down)     KEEP_UP=0 teardown ;;
  *) echo "usage: ./twocloud.sh [all|topology|run|down]   (SCAFFOLD — 'all' stops at the first TODO phase)"; exit 1 ;;
esac
