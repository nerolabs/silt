#!/usr/bin/env bash
# awstest.sh — one-command AWS field test for silt (the fallback substrate; mirrors
# integration/cloudtest/cloudtest.sh on AWS). Runs the SAME flows by SOURCING
# ../cloudtest/scenarios.sh + ../cloudtest/gen_report.sh — only the substrate differs.
#
#   ./awstest.sh setup      interactive: pick AWS profile/region → write config.env
#   ./awstest.sh            build → topology → apply → run flows → report → DESTROY
#   ./awstest.sh up         bring the fleet up and leave it (implies KEEP_UP)
#   ./awstest.sh run        run the scenarios against an already-up fleet
#   ./awstest.sh report     regenerate the report from results.jsonl
#   ./awstest.sh down       terraform destroy
#   ./awstest.sh nuke       last-resort: terminate/release billable resources tagged silt:awstest=<run>
#
# STATUS: first cut, dry-validated only — see README. Teardown is guaranteed:
# destroy-on-EXIT + a per-instance `shutdown -h +TTL` (terminate-on-shutdown).
set -euo pipefail
FT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$FT_DIR/../.." && pwd)"
GCP_DIR="$FT_DIR/../cloudtest"     # the shared scenarios.sh + gen_report.sh live here
cd "$FT_DIR"

if [ "${1:-}" = setup ]; then
  echo "silt AWS field test — interactive setup"
  command -v aws >/dev/null 2>&1 || { echo "  ✗ aws CLI not installed — https://docs.aws.amazon.com/cli/"; exit 1; }
  command -v session-manager-plugin >/dev/null 2>&1 || echo "  ! session-manager-plugin not found — SSM exec needs it (https://docs.aws.amazon.com/systems-manager/latest/userguide/session-manager-working-with-install-plugin.html)"
  acct="$(aws sts get-caller-identity --query Account --output text 2>/dev/null || true)"
  [ -n "$acct" ] && [ "$acct" != None ] || { echo "  ✗ no working AWS credentials — run 'aws configure' or set AWS_PROFILE"; exit 1; }
  printf '  AWS region [%s]: ' "${AWS_REGION:-us-west-2}"; read -r reg; reg="${reg:-${AWS_REGION:-us-west-2}}"
  if [ -f config.env ]; then
    echo "  config.env already exists — leaving it"
  else
    sed "s#^export AWS_REGION=.*#export AWS_REGION=\"$reg\"#" config.env.example > config.env
    echo "  ✓ wrote config.env (account $acct, region $reg)"
  fi
  echo "  ✓ setup complete. Next: SMOKE=1 ./awstest.sh   (cheap shakeout), then ./awstest.sh"
  exit 0
fi

[ -f config.env ] || { echo "no config.env — run './awstest.sh setup', or copy config.env.example and fill it in"; exit 1; }
# The caller's env wins over config.env (same footgun the GCP §F fix addressed).
_CALLER_REGION="${AWS_REGION:-}"
# shellcheck disable=SC1091
. ./config.env
[ -n "$_CALLER_REGION" ] && AWS_REGION="$_CALLER_REGION"
: "${AWS_REGION:=us-west-2}"
export AWS_REGION
echo "==> region: $AWS_REGION"

: "${RUN_ID:=$(git -C "$REPO_ROOT" rev-parse --short HEAD 2>/dev/null || echo dev)-$$}"
RUN_ID="$(printf '%s' "$RUN_ID" | tr -c 'a-z0-9-' '-' | cut -c1-24)"
export RUN_ID

need() { command -v "$1" >/dev/null 2>&1 || { echo "missing prerequisite: $1"; exit 1; }; }
check_prereqs() { need terraform; need aws; need go; need python3; need base64; }

build_binary() {
  echo "==> building silt (linux/amd64) @ $RUN_ID"
  ( cd "$REPO_ROOT" && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags '-s -w' -o "$FT_DIR/silt-linux-amd64" ./cmd/silt )
  ( cd "$REPO_ROOT" && go build -o "$FT_DIR/.silt-local" ./cmd/silt )   # for topology id-gen
}

gen_topology() {
  echo "==> generating deterministic topology${SMOKE:+ (SMOKE — trimmed 4-node set)}"
  SILT_BIN="$FT_DIR/.silt-local" BOND_MODE="${BOND_MODE:-fast}" SMOKE="${SMOKE:-0}" AWS_REGION="$AWS_REGION" python3 topology.py
}

tf() { terraform -chdir="$FT_DIR/terraform" "$@"; }

apply() {
  echo "==> terraform apply (run=$RUN_ID, region=$AWS_REGION)"
  printf '%s\n' "$RUN_ID" > "$FT_DIR/.last_run_id"
  tf init -input=false >/dev/null
  tf apply -input=false -auto-approve \
    -var "run_id=$RUN_ID" \
    -var "instance_type=${INSTANCE_TYPE:-t3.small}" \
    -var "boot_disk_gb=${BOOT_DISK_GB:-20}" \
    -var "ttl_minutes=${TTL_MINUTES:-180}" \
    -var "silt_binary_path=$FT_DIR/silt-linux-amd64" \
    -var "core_on_demand=${CORE_ON_DEMAND:-true}" \
    -var "all_on_demand=${ALL_ON_DEMAND:-false}"
  tf output -json nodes > "$FT_DIR/nodes.json"
  # Merge the deterministic nodeid from topology.json into nodes.json (terraform output
  # carries instance_id/az/ips/role but NOT the silt NodeID the #184 drills derive peers
  # from). Identical merge to the GCP harness.
  python3 - "$FT_DIR/nodes.json" "$FT_DIR/topology.json" <<'PY'
import json, sys
nodes = json.load(open(sys.argv[1]))
topo  = json.load(open(sys.argv[2]))["nodes"]
for name, n in nodes.items():
    if name in topo and topo[name].get("nodeid"):
        n["nodeid"] = topo[name]["nodeid"]
json.dump(nodes, open(sys.argv[1], "w"), indent=2)
PY
  echo "    nodes.json written ($(python3 -c "import json;print(len(json.load(open('$FT_DIR/nodes.json'))))") instances)"
}

wait_ready() {
  echo "==> waiting for silt.service on every node (SSM: agent up + user-data pulls the binary + boots)"
  # shellcheck disable=SC1091
  . ./lib.sh
  local n deadline=$(( $(date +%s) + 600 )) pending
  while :; do
    pending=""
    for n in $(node_names); do
      ssh_node "$n" "systemctl is-active --quiet silt.service" >/dev/null 2>&1 || pending="$pending $n"
    done
    [ -z "$pending" ] && { echo "    all nodes ready"; return 0; }
    [ "$(date +%s)" -ge "$deadline" ] && { echo "    timed out waiting for:$pending"; return 1; }
    echo "    still starting:$pending"; sleep 20
  done
}

run_scenarios() {
  echo "==> running scenarios (shared flows from ../cloudtest/scenarios.sh)"
  : > "$FT_DIR/results.jsonl"
  # shellcheck disable=SC1091
  . ./lib.sh                    # AWS substrate (ssh_node/jlog/…) — FT_DIR stays this dir
  # shellcheck disable=SC1091
  . "$GCP_DIR/scenarios.sh"     # the SAME flows, unchanged
  run_all_scenarios || true     # a failing check is recorded, never aborts the run
}

report() { echo "==> report"; RUN_ID="$RUN_ID" FT_DIR="$FT_DIR" "$GCP_DIR/gen_report.sh" 2>/dev/null || RUN_ID="$RUN_ID" "$GCP_DIR/gen_report.sh"; }

teardown() {
  [ "${KEEP_UP:-0}" = 1 ] && { echo "==> KEEP_UP=1 — leaving the fleet up. './awstest.sh down' when done."; return; }
  echo "==> DESTROY (run=$RUN_ID)"
  tf destroy -input=false -auto-approve \
    -var "run_id=$RUN_ID" -var "silt_binary_path=$FT_DIR/silt-linux-amd64" \
    || { echo "    terraform destroy failed (see stderr) — falling back to nuke-by-tag"; nuke; }
}

nuke() {
  local target="$RUN_ID"
  [ -z "${RUN_ID_EXPLICIT:-}" ] && [ -f "$FT_DIR/.last_run_id" ] && target="$(cat "$FT_DIR/.last_run_id")"
  echo "==> nuke: terminating BILLABLE resources tagged silt:awstest=$target in $AWS_REGION"
  # Instances (the main cost).
  local ids
  ids="$(aws ec2 describe-instances --region "$AWS_REGION" \
    --filters "Name=tag:silt:awstest,Values=$target" "Name=instance-state-name,Values=pending,running,stopping,stopped" \
    --query 'Reservations[].Instances[].InstanceId' --output text 2>/dev/null || true)"
  [ -n "$ids" ] && aws ec2 terminate-instances --region "$AWS_REGION" --instance-ids $ids >/dev/null 2>&1 || true
  # NAT gateways + their EIPs (billed hourly) — delete NAT then release the EIP.
  local ng
  for ng in $(aws ec2 describe-nat-gateways --region "$AWS_REGION" --filter "Name=tag:silt:awstest,Values=$target" --query 'NatGateways[?State==`available`].NatGatewayId' --output text 2>/dev/null || true); do
    [ -n "$ng" ] && aws ec2 delete-nat-gateway --region "$AWS_REGION" --nat-gateway-id "$ng" >/dev/null 2>&1 || true
  done
  local eip
  for eip in $(aws ec2 describe-addresses --region "$AWS_REGION" --filters "Name=tag:silt:awstest,Values=$target" --query 'Addresses[].AllocationId' --output text 2>/dev/null || true); do
    [ -n "$eip" ] && aws ec2 release-address --region "$AWS_REGION" --allocation-id "$eip" >/dev/null 2>&1 || true
  done
  # Artifacts bucket.
  local acct; acct="$(aws sts get-caller-identity --query Account --output text 2>/dev/null || true)"
  aws s3 rb "s3://silt-awstest-${target}-${acct}" --force >/dev/null 2>&1 || echo "    (bucket not found or already gone)"
  echo "    nuke: billable resources swept. NOTE: the (free) VPC/subnets/SG/route-tables are best removed by 'terraform destroy'."
  # Full-sweep note on the billable set.
  local left
  left="$(aws ec2 describe-instances --region "$AWS_REGION" --filters "Name=tag:silt:awstest,Values=$target" "Name=instance-state-name,Values=pending,running,stopping,stopped" --query 'Reservations[].Instances[].InstanceId' --output text 2>/dev/null | grep -c . || true)"
  [ "${left:-0}" -gt 0 ] 2>/dev/null && echo "    ⚠ $left instance(s) still terminating — re-check with: aws ec2 describe-instances --filters Name=tag:silt:awstest,Values=$target" || echo "    nuke: zero running instances for run $target"
}

case "${1:-all}" in
  all)    check_prereqs; build_binary; gen_topology; trap teardown EXIT; apply; wait_ready; run_scenarios; report ;;
  up)     check_prereqs; build_binary; gen_topology; KEEP_UP=1 apply; wait_ready; echo "fleet up (run=$RUN_ID)" ;;
  run)    run_scenarios; report ;;
  report) report ;;
  down)   KEEP_UP=0 teardown ;;
  nuke)   RUN_ID_EXPLICIT="${RUN_ID_EXPLICIT:-}" nuke ;;
  *) echo "usage: ./awstest.sh [setup|all|up|run|report|down|nuke]"; exit 1 ;;
esac
