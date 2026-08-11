#!/usr/bin/env bash
# cloudtest.sh — one-command GCP field test for silt (roadmap #52).
#
#   ./cloudtest.sh setup      interactive: ask for project + walk through gcloud auth → write config.env
#   ./cloudtest.sh            build → topology → apply → run flows → report → DESTROY
#   ./cloudtest.sh up         bring the network up and leave it (implies KEEP_UP)
#   ./cloudtest.sh run        run the scenarios against an already-up network
#   ./cloudtest.sh report     regenerate the report from results.jsonl
#   ./cloudtest.sh down       terraform destroy
#   ./cloudtest.sh nuke       last-resort: delete every resource labelled cloudtest=<run>
#
# Teardown is guaranteed: the default lifecycle destroys on EXIT (even on error),
# every VM self-destructs after TTL_MINUTES, and `nuke` cleans up by label if
# Terraform state is ever lost. See README for the cost model.
set -euo pipefail
FT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$FT_DIR/../.." && pwd)"
cd "$FT_DIR"

# `setup` runs BEFORE the config.env requirement — it is what CREATES config.env.
# Interactive: asks for the project, walks the user through gcloud auth (both the
# user login AND the application-default credentials Terraform needs), enables the
# required APIs, and writes config.env from the example.
if [ "${1:-}" = setup ]; then
  echo "silt GCP field test — interactive setup"
  command -v gcloud >/dev/null 2>&1 || { echo "  ✗ gcloud not installed — https://cloud.google.com/sdk/docs/install"; exit 1; }
  cur="$(gcloud config get-value project 2>/dev/null || true)"
  printf '  GCP project id [%s]: ' "${cur:-none}"; read -r proj
  proj="${proj:-$cur}"
  [ -n "$proj" ] && [ "$proj" != none ] || { echo "  ✗ a project id is required (billing MUST be enabled on it)"; exit 1; }
  if ! gcloud auth list --filter=status:ACTIVE --format='value(account)' 2>/dev/null | grep -q .; then
    echo "  → no active gcloud login; opening the browser…"; gcloud auth login || exit 1
  fi
  if ! gcloud auth application-default print-access-token >/dev/null 2>&1; then
    echo "  → Terraform needs application-default credentials; opening the browser…"
    gcloud auth application-default login || exit 1
  fi
  gcloud config set project "$proj" >/dev/null 2>&1 || true
  echo "  → enabling required APIs (compute, iap, storage) — idempotent…"
  gcloud services enable compute.googleapis.com iap.googleapis.com storage.googleapis.com --project "$proj" 2>/dev/null \
    || echo "  ! could not enable APIs automatically — enable compute/iap/storage manually if apply fails"
  if [ -f config.env ]; then
    echo "  config.env already exists — leaving it (edit it directly to change knobs)"
  else
    sed "s#^export PROJECT_ID=.*#export PROJECT_ID=\"$proj\"#" config.env.example > config.env
    echo "  ✓ wrote config.env (PROJECT_ID=$proj) — edit it to tune region/size/cost guards"
  fi
  echo "  ✓ setup complete. Next:"
  echo "      SMOKE=1 ./cloudtest.sh     # cheap ~4-node first shakeout (pennies)"
  echo "      ./cloudtest.sh             # full 13-node run → report → DESTROY"
  exit 0
fi

[ -f config.env ] || { echo "no config.env — run './cloudtest.sh setup' (interactive), or copy config.env.example and fill it in"; exit 1; }
# shellcheck disable=SC1091
. ./config.env
: "${PROJECT_ID:?set PROJECT_ID in config.env}"
: "${REGION:=us-central1}"
export PROJECT_ID REGION

# A stable run id for this invocation (no Date.now flakiness — derived from git + pid).
: "${RUN_ID:=$(git -C "$REPO_ROOT" rev-parse --short HEAD 2>/dev/null || echo dev)-$$}"
RUN_ID="$(printf '%s' "$RUN_ID" | tr -c 'a-z0-9-' '-' | cut -c1-24)"
export RUN_ID
STATE_TFVARS="terraform/topology.auto.tfvars.json"

need() { command -v "$1" >/dev/null 2>&1 || { echo "missing prerequisite: $1"; exit 1; }; }
check_prereqs() { need terraform; need gcloud; need go; need python3; need curl; }

build_binary() {
  echo "==> building silt (linux/amd64) @ $RUN_ID"
  ( cd "$REPO_ROOT" && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
      go build -trimpath -ldflags '-s -w' -o "$FT_DIR/silt-linux-amd64" ./cmd/silt )
  ( cd "$REPO_ROOT" && go build -o "$FT_DIR/.silt-local" ./cmd/silt )   # for topology id-gen
}

gen_topology() {
  echo "==> generating deterministic topology${SMOKE:+ (SMOKE — trimmed 4-node set)}"
  SILT_BIN="$FT_DIR/.silt-local" BOND_MODE="${BOND_MODE:-fast}" SMOKE="${SMOKE:-0}" python3 topology.py
}

tf() { terraform -chdir="$FT_DIR/terraform" "$@"; }

apply() {
  echo "==> terraform apply (run=$RUN_ID)"
  # Persist the run id so `nuke`/`down` from a FRESH shell target the right label.
  # RUN_ID embeds $$ (pid) by default, so a later `./cloudtest.sh nuke` in a new
  # shell would compute a different id and match nothing — read this file instead.
  printf '%s\n' "$RUN_ID" > "$FT_DIR/.last_run_id"
  tf init -input=false >/dev/null
  tf apply -input=false -auto-approve \
    -var "project_id=$PROJECT_ID" \
    -var "default_region=$REGION" \
    -var "run_id=$RUN_ID" \
    -var "machine_type=${MACHINE_TYPE:-e2-small}" \
    -var "boot_disk_gb=${BOOT_DISK_GB:-20}" \
    -var "ttl_minutes=${TTL_MINUTES:-180}" \
    -var "silt_binary_path=$FT_DIR/silt-linux-amd64" \
    -var "budget_amount_usd=${BUDGET_AMOUNT_USD:-0}" \
    -var "billing_account=${BILLING_ACCOUNT:-}" \
    -var "core_on_demand=${CORE_ON_DEMAND:-$([ "${SMOKE:-0}" = 1 ] && echo false || echo true)}" \
    -var "all_on_demand=${ALL_ON_DEMAND:-false}"
  tf output -json nodes > "$FT_DIR/nodes.json"
  # Terraform's node output carries instance_name/zone/ips/role but NOT the silt
  # NodeID — yet scenarios.sh reads node_field <n> nodeid (the #184 drills derive
  # peer IDs from it). topology.json HAS the deterministic nodeid per node, so merge
  # it in here. Without this every 184-* flow crashed with KeyError: 'nodeid' and
  # the adversarial consensus drills never ran on GCP (blind cloud finding #1).
  python3 - "$FT_DIR/nodes.json" "$FT_DIR/topology.json" <<'PY'
import json, sys
nodes = json.load(open(sys.argv[1]))
topo  = json.load(open(sys.argv[2]))["nodes"]
for name, n in nodes.items():
    if name in topo and topo[name].get("nodeid"):
        n["nodeid"] = topo[name]["nodeid"]
json.dump(nodes, open(sys.argv[1], "w"), indent=2)
PY
  echo "    nodes.json written ($(python3 -c "import json;print(len(json.load(open('$FT_DIR/nodes.json'))))") instances, nodeid merged from topology.json)"
}

wait_ready() {
  echo "==> waiting for silt.service on every node (startup pulls the binary + boots)"
  # shellcheck disable=SC1091
  . ./lib.sh
  local n deadline=$(( $(date +%s) + 600 )) pending
  while :; do
    pending=""
    for n in $(node_names); do
      [ "$(node_field "$n" role)" = natgw ] && continue
      ssh_node "$n" "systemctl is-active --quiet silt.service" 2>/dev/null || pending="$pending $n"
    done
    [ -z "$pending" ] && { echo "    all nodes ready"; return 0; }
    [ "$(date +%s)" -ge "$deadline" ] && { echo "    timed out waiting for:$pending"; return 1; }
    echo "    still starting:$pending"; sleep 15
  done
}

run_scenarios() {
  echo "==> running scenarios"
  : > "$FT_DIR/results.jsonl"
  # shellcheck disable=SC1091
  . ./lib.sh
  # shellcheck disable=SC1091
  . ./scenarios.sh
  run_all_scenarios || true    # a failing check is recorded, never aborts the run
}

report() { echo "==> report"; RUN_ID="$RUN_ID" ./gen_report.sh; }

teardown() {
  [ "${KEEP_UP:-0}" = 1 ] && { echo "==> KEEP_UP=1 — leaving the network up. './cloudtest.sh down' when done."; return; }
  echo "==> DESTROY (run=$RUN_ID)"
  tf destroy -input=false -auto-approve \
    -var "project_id=$PROJECT_ID" -var "default_region=$REGION" -var "run_id=$RUN_ID" \
    -var "silt_binary_path=$FT_DIR/silt-linux-amd64" 2>/dev/null \
    || { echo "    terraform destroy failed — falling back to nuke-by-label"; nuke; }
}

nuke() {
  # Prefer the persisted run id (RUN_ID default embeds $$ and differs in a fresh shell).
  local target="$RUN_ID"
  if [ -z "${RUN_ID_EXPLICIT:-}" ] && [ -f "$FT_DIR/.last_run_id" ]; then
    target="$(cat "$FT_DIR/.last_run_id")"
  fi
  echo "==> nuke: deleting every resource labelled cloudtest=$target in $PROJECT_ID"
  gcloud compute instances list --project "$PROJECT_ID" \
    --filter "labels.cloudtest=$target" --format 'value(name,zone)' | while read -r name zone; do
    [ -n "$name" ] && gcloud compute instances delete "$name" --zone "$zone" --project "$PROJECT_ID" --quiet || true
  done
  # The artifacts bucket is labelled too but is not an instance — delete it by name
  # so a lost-state nuke does not leave it billing (network/subnets/routes are free).
  gcloud storage rm -r "gs://silt-ft-${target}-${PROJECT_ID}" --quiet 2>/dev/null \
    || echo "    (bucket gs://silt-ft-${target}-${PROJECT_ID} not found or already gone)"
  echo "    (network/subnets/firewall/routes: 'terraform destroy' is preferred; they are free if left)"
}

case "${1:-all}" in
  all)
    check_prereqs; build_binary; gen_topology
    trap teardown EXIT
    apply; wait_ready; run_scenarios; report
    ;;
  up)     check_prereqs; build_binary; gen_topology; KEEP_UP=1 apply; wait_ready; echo "network up (run=$RUN_ID)"; ;;
  run)    run_scenarios; report; ;;
  report) report; ;;
  down)   KEEP_UP=0 teardown; ;;
  nuke)   nuke; ;;
  *) echo "usage: ./cloudtest.sh [setup|all|up|run|report|down|nuke]"; exit 1; ;;
esac
