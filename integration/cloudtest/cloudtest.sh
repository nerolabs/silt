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
# The caller's env must WIN over config.env (§F footgun): `. ./config.env` runs AFTER the
# environment is inherited, so a config.env `export REGION=…` silently CLOBBERS a
# command-line `REGION=us-west1 ./cloudtest.sh` — a confusing dead end that cost a wasted
# apply+teardown to diagnose. Snapshot the caller's REGION before sourcing and restore it.
_CALLER_REGION="${REGION:-}"
# shellcheck disable=SC1091
. ./config.env
[ -n "$_CALLER_REGION" ] && REGION="$_CALLER_REGION"
: "${PROJECT_ID:?set PROJECT_ID in config.env}"
: "${REGION:=us-central1}"
export PROJECT_ID REGION
echo "==> region: $REGION (primary cluster zone follows REGION via topology.py PRIMARY_ZONE)"

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

# Pre-flight the external-IP quota per region BEFORE apply (§F): the 13-node topology
# needs ~11 external IPs, but the default IN_USE_ADDRESSES quota is 8/region — a shortfall
# only surfaces ~30s/instance into a doomed apply. Count the public (non-natted/natgw,
# they egress via Cloud NAT) nodes per region from topology.json and compare to each
# region's live IN_USE_ADDRESSES headroom. PREFLIGHT=0 skips. Zone E2 capacity is not
# queryable via the API, so this covers the IP quota only (the other half of §F).
preflight_quota() {
  [ "${PREFLIGHT:-1}" = 0 ] && { echo "==> preflight: skipped (PREFLIGHT=0)"; return 0; }
  [ -f "$FT_DIR/topology.json" ] || return 0
  echo "==> preflight: external-IP (IN_USE_ADDRESSES) quota per region"
  python3 - "$FT_DIR/topology.json" > /tmp/ft_ipneeds.txt <<'PY'
import json, sys
from collections import Counter
nodes = json.load(open(sys.argv[1]))["nodes"]
c = Counter()
for n in nodes.values():
    if n.get("role") in ("natted", "natgw"):  # egress via Cloud NAT — no external IP
        continue
    c[n.get("region", "?")] += 1
for r, k in c.items():
    print(r, k)
PY
  local ok=1 region need q head limit
  while read -r region need; do
    [ -n "$region" ] && [ "$region" != "?" ] || continue
    q="$(gcloud compute regions describe "$region" --project "$PROJECT_ID" --format="json(quotas)" 2>/dev/null \
      | python3 -c "
import json,sys
try: qs=json.load(sys.stdin).get('quotas',[])
except Exception: qs=[]
for q in qs:
    if q.get('metric')=='IN_USE_ADDRESSES':
        print(int(q['limit']-q['usage']), int(q['limit'])); break
else: print('-1 -1')" 2>/dev/null)"
    head="${q%% *}"; limit="${q##* }"
    if [ "${head:-'-1'}" = "-1" ]; then
      echo "  ? $region: could not read IN_USE_ADDRESSES quota (gcloud/auth?) — proceeding, but $need IPs are needed"
    elif [ "${head:-0}" -lt "$need" ] 2>/dev/null; then
      echo "  ✗ $region: needs $need external IP(s), only ${head} free (IN_USE_ADDRESSES limit $limit)"; ok=0
    else
      echo "  ✓ $region: needs $need external IP(s), ${head} free (limit $limit)"
    fi
  done < /tmp/ft_ipneeds.txt
  if [ "$ok" != 1 ]; then
    echo "  preflight FAILED — free addresses, request an IN_USE_ADDRESSES bump, or relocate the primary"
    echo "  cluster to a region with headroom (REGION=<region> ./cloudtest.sh). PREFLIGHT=0 to override."
    exit 1
  fi
}

apply() {
  preflight_quota
  # Provisioning model, stated LOUDLY (never silent): a graded/cert run defaults every
  # role to on-demand STANDARD, because a mid-run SPOT preemption of storage/relay/
  # adversary/sybil cascades into false FAILs of nearly every flow (variables.tf) — two
  # runs (841fa1b, 6218ba1) were lost to exactly this: 7 SPOT VMs preempted, wait_ready
  # timed out, ZERO scenarios graded. e2-small on-demand is ~cents/hr for the fleet.
  # SMOKE stays all-SPOT (cheap shakedown); explicit ALL_ON_DEMAND=false opts back in.
  echo "==> provisioning model: $([ "${ALL_ON_DEMAND:-$([ "${SMOKE:-0}" = 1 ] && echo false || echo true)}" = true ] && echo 'ALL on-demand (STANDARD) — preemption-safe cert run' || echo 'SPOT for non-core (cheap; may be preempted — NOT for a graded cert)')"
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
    -var "all_on_demand=${ALL_ON_DEMAND:-$([ "${SMOKE:-0}" = 1 ] && echo false || echo true)}"
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

# capture_failed_nodes NODES... — grab each node's silt journal + service state to a
# per-run file BEFORE teardown, so a came-up failure is DIAGNOSABLE instead of lost with
# the VMs. This is the fix for the "harness discarded the evidence" trap: without it, a
# node that crash-loops during startup takes its crash log to the grave on destroy, and
# the operator is left guessing why. Cheap, bounded (200 lines/node), never fatal.
capture_failed_nodes() {
  local out="$FT_DIR/failed-nodes-$RUN_ID.log" n
  echo "==> capturing journals for un-ready nodes → $out"
  { echo "# failed/un-ready nodes for run=$RUN_ID @ $(date -u +%FT%TZ 2>/dev/null || echo unknown)"; } > "$out"
  for n in "$@"; do
    {
      echo ""; echo "======== $n (role=$(node_field "$n" role)) ========"
      echo "-- systemctl status --"; ssh_node "$n" "systemctl status silt.service --no-pager -l 2>&1 | head -20" 2>/dev/null || echo "(status unavailable — node unreachable)"
      echo "-- journalctl -u silt (last 200) --"; ssh_node "$n" "sudo journalctl -u silt --no-pager -n 200 2>&1" 2>/dev/null || echo "(journal unavailable — node unreachable)"
    } >> "$out"
  done
  echo "    captured $# node journal(s); read $out to attribute the failure"
}

wait_ready() {
  echo "==> waiting for silt.service on every node (startup pulls the binary + boots)"
  # shellcheck disable=SC1091
  . ./lib.sh
  local n deadline=$(( $(date +%s) + 600 )) pending core_pending syb_pending
  while :; do
    pending=""
    for n in $(node_names); do
      [ "$(node_field "$n" role)" = natgw ] && continue
      ssh_node "$n" "systemctl is-active --quiet silt.service" 2>/dev/null || pending="$pending $n"
    done
    [ -z "$pending" ] && { echo "    all nodes ready"; return 0; }
    if [ "$(date +%s)" -ge "$deadline" ]; then
      # Split pending into CORE vs the OPT-IN sybil cohort. The sybils exist only for the
      # C2 corner; if they fail to boot but every core node is up, the base corners
      # (convergence, publish, fault-tolerance, takedown, …) are still fully gradeable —
      # so DON'T abort the whole billable run on the opt-in cohort. Only a core node not
      # coming up is a real infra failure that invalidates the run.
      core_pending=""; syb_pending=""
      for n in $pending; do
        if [ "$(node_field "$n" role)" = sybil ]; then syb_pending="$syb_pending $n"; else core_pending="$core_pending $n"; fi
      done
      # shellcheck disable=SC2086
      capture_failed_nodes $pending
      if [ -n "$core_pending" ]; then
        echo "    timed out — CORE nodes not ready:$core_pending (sybils:$syb_pending) — aborting (a core-node startup failure invalidates the run)"
        return 1
      fi
      echo "    timed out — only the OPT-IN sybil cohort is not ready:$syb_pending — PROCEEDING; the C2/maturing corners will GAP honestly (their journals captured above), the base corners still grade"
      return 0
    fi
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
  # Persist the console (#7): ft_publish diagnostics and per-flow narration used to
  # die with the terminal, leaving a FAIL verdict with no trail after teardown. The
  # tee'd copy lands next to the run's report. (The pipeline subshell is fine: flows
  # append to results.jsonl / evidence logs by path, and report() reads files.)
  { run_all_scenarios || true; } 2>&1 | tee -a "$FT_DIR/console-$RUN_ID.log"
}

report() {
  echo "==> report"; RUN_ID="$RUN_ID" ./gen_report.sh
  # Archive per run: results.jsonl/report.md are OVERWRITTEN by the next run, which
  # erased the pass/fail history needed to tell a regression from a never-passed
  # flow (this session had to reconstruct run c815091's verdicts from memory).
  mkdir -p "$FT_DIR/archive"
  cp -f "$FT_DIR/results.jsonl" "$FT_DIR/archive/results-$RUN_ID.jsonl" 2>/dev/null || true
  cp -f "$FT_DIR/report.md"     "$FT_DIR/archive/report-$RUN_ID.md"     2>/dev/null || true
  echo "    archived → archive/results-$RUN_ID.jsonl + archive/report-$RUN_ID.md (evidence: flow-evidence-$RUN_ID.log, console-$RUN_ID.log, publish-diag-$RUN_ID.log)"
}

teardown() {
  [ "${KEEP_UP:-0}" = 1 ] && { echo "==> KEEP_UP=1 — leaving the network up. './cloudtest.sh down' when done."; return; }
  echo "==> DESTROY (run=$RUN_ID)"
  # Do NOT swallow destroy stderr (§D): a partial-apply state makes `terraform destroy`
  # fail, and hiding its error makes the destroy-failed→nuke handoff (and the VPC leak it
  # can cause) invisible. Let terraform's error print so the fallback is diagnosable.
  tf destroy -input=false -auto-approve \
    -var "project_id=$PROJECT_ID" -var "default_region=$REGION" -var "run_id=$RUN_ID" \
    -var "silt_binary_path=$FT_DIR/silt-linux-amd64" \
    || { echo "    terraform destroy failed (see stderr above) — falling back to nuke-by-name"; nuke; }
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
  # so a lost-state nuke does not leave it billing.
  gcloud storage rm -r "gs://silt-ft-${target}-${PROJECT_ID}" --quiet 2>/dev/null \
    || echo "    (bucket gs://silt-ft-${target}-${PROJECT_ID} not found or already gone)"
  # Network resources (VPC/subnets/firewalls/routes): GCP networks & firewalls carry NO
  # labels, so terraform-destroy is preferred — but on a partial-apply state that fails,
  # and the old nuke left them orphaned. Free, but they ACCUMULATE across failed runs
  # toward network/subnet/firewall quotas (§D VPC-leak finding). All are named
  # `silt-ft-…-<run_id>`, so sweep them by name in dependency order (firewalls+routes →
  # subnets → the VPC itself). `|| true` on each: a resource already gone is fine.
  local re="silt-ft.*${target}\$"
  gcloud compute firewall-rules list --project "$PROJECT_ID" --filter "name~${re}" --format 'value(name)' \
    | while read -r fw; do [ -n "$fw" ] && gcloud compute firewall-rules delete "$fw" --project "$PROJECT_ID" --quiet || true; done
  gcloud compute routes list --project "$PROJECT_ID" --filter "name~${re}" --format 'value(name)' \
    | while read -r rt; do [ -n "$rt" ] && gcloud compute routes delete "$rt" --project "$PROJECT_ID" --quiet || true; done
  gcloud compute networks subnets list --project "$PROJECT_ID" --filter "name~${re}" --format 'value(name,region)' \
    | while read -r sn rg; do [ -n "$sn" ] && gcloud compute networks subnets delete "$sn" --region "$rg" --project "$PROJECT_ID" --quiet || true; done
  gcloud compute networks delete "silt-ft-${target}" --project "$PROJECT_ID" --quiet 2>/dev/null \
    || echo "    (network silt-ft-${target} not found or already gone)"
  # Full-sweep assert: nothing named for this run may remain (instances/addresses/
  # networks/subnets/firewalls/routes). A non-empty list is a real residual to chase.
  local left
  left="$( { gcloud compute instances list      --project "$PROJECT_ID" --filter "name~${re}" --format 'value(name)';
             gcloud compute addresses list       --project "$PROJECT_ID" --filter "name~${re}" --format 'value(name)';
             gcloud compute networks list         --project "$PROJECT_ID" --filter "name~${re}" --format 'value(name)';
             gcloud compute networks subnets list --project "$PROJECT_ID" --filter "name~${re}" --format 'value(name)';
             gcloud compute firewall-rules list   --project "$PROJECT_ID" --filter "name~${re}" --format 'value(name)';
             gcloud compute routes list           --project "$PROJECT_ID" --filter "name~${re}" --format 'value(name)'; } 2>/dev/null | grep -c . || true)"
  if [ "${left:-0}" -gt 0 ] 2>/dev/null; then
    echo "    ⚠ nuke: ${left} resource(s) still labelled/named for run $target remain — investigate:"
    gcloud compute instances list --project "$PROJECT_ID" --filter "name~${re}" --format 'value(name,zone)' 2>/dev/null | sed 's/^/      /'
  else
    echo "    nuke: full sweep clean — zero residual for run $target (instances/addresses/networks/subnets/firewalls/routes)"
  fi
}

# ── build-immutable #6 pre-flight gate ───────────────────────────────────────
# An expensive multi-region run CONFIRMS an already-understood, locally-reproduced
# fix, or certifies liveness/timing at scale — the one thing a real WAN uniquely
# proves. It NEVER discovers a cause or tests a guess: that is what the laptop
# harnesses are for (integration/adversarial netem · integration/flakynet ·
# go test ./e2e). The #286 rabbit-hole was five billable runs spent DISCOVERING
# causes that were reproducible for free (docs/reviews/286-wan-rabbithole-POSTMORTEM.md).
# So a billable `up`/`all` is refused unless the operator has WRITTEN DOWN, per
# build-immutable #6 (docs/build-process.md):
#   RUN_MECHANISM — the one-paragraph mechanism ("X because Y; fixed by Z"), OR the
#                   sanctioned purpose ("liveness/timing at scale — R1 gate #360"),
#                   OR a path/issue reference to it.
#   RUN_REPRO     — the local reproduction that already reproduces/confirms it
#                   (e.g. "go test ./e2e -run Equivocator", "integration/adversarial/run.sh",
#                   or "n/a — liveness-only; attacks certified off-cloud, #360").
# There is deliberately NO silent bypass — stating intent before spending money is
# the whole point of the gate.
preflight_gate() {
  if [ -z "${RUN_MECHANISM:-}" ] || [ -z "${RUN_REPRO:-}" ]; then
    cat >&2 <<'EOF'
✗ pre-flight gate (build-immutable #6): a billable cloud run needs a written justification.

  An expensive multi-region run CONFIRMS an understood, locally-reproduced fix — or
  certifies liveness/timing at scale. It never DISCOVERS a cause or tests a guess.
  If you are here to find out WHY something fails, stop: reproduce it on a laptop first
  (integration/adversarial netem · integration/flakynet · go test ./e2e). See
  docs/build-process.md and docs/reviews/286-wan-rabbithole-POSTMORTEM.md.

  Re-run with BOTH set, e.g.:
    RUN_MECHANISM="liveness/timing at scale — R1 gate #360; #357 fork-choice fix confirmed locally" \
    RUN_REPRO="integration/adversarial/run.sh green; attacks certified off-cloud" \
    ./cloudtest.sh up
EOF
    exit 2
  fi
  { echo "run=${RUN_ID:-unknown}  ts=$(date -u +%FT%TZ 2>/dev/null || echo unknown)"
    echo "RUN_MECHANISM: $RUN_MECHANISM"
    echo "RUN_REPRO:     $RUN_REPRO"
    echo "---"
  } >> run-justification.log
  echo "✓ pre-flight gate: justification recorded (build-immutable #6) — proceeding to a BILLABLE run."
}

case "${1:-all}" in
  all)
    preflight_gate
    check_prereqs; build_binary; gen_topology
    # Clear last run's results + report BEFORE spending money, stamped with THIS run_id.
    # An aborted run (e.g. wait_ready fails) otherwise leaves the PREVIOUS run's
    # report.md/results.jsonl in place, so a came-up failure reads as if it had verdicts —
    # exactly the stale-data trap that nearly got a prior run's numbers reported as this
    # run's. Truncate now so a stale report can never be mistaken for a fresh one.
    : > "$FT_DIR/results.jsonl"
    printf '# run=%s — no scenarios have completed yet (network came up? see failed-nodes-%s.log)\n' "$RUN_ID" "$RUN_ID" > "$FT_DIR/report.md"
    trap teardown EXIT
    apply; wait_ready; run_scenarios; report
    ;;
  up)     preflight_gate; check_prereqs; build_binary; gen_topology; KEEP_UP=1 apply; wait_ready; echo "network up (run=$RUN_ID)"; ;;
  run)    run_scenarios; report; ;;
  report) report; ;;
  down)   KEEP_UP=0 teardown; ;;
  nuke)   nuke; ;;
  *) echo "usage: ./cloudtest.sh [setup|all|up|run|report|down|nuke]"; exit 1; ;;
esac
