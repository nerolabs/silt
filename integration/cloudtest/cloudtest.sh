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
#   ./cloudtest.sh net-up     create the PERSISTENT network (then launch runs with PERSIST_NET=1)
#   ./cloudtest.sh net-down   destroy the persistent network
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

# LOCAL=1 — run the SAME harness against docker containers on this machine ($0,
# no GCP, no config.env). The graded drive logic (severs, kill selection,
# relaunch/restore, verdicts) executes here first, so harness defects die before
# a billable run (docs/thinking/2026-08-20-harness-local-first.md). What LOCAL
# cannot cover stays the cloud's job: real WAN latency, real NAT (natgw/nat-*
# are excluded — 9-cross-nat SKIPs; integration/nat owns that locally), scale.
if [ "${LOCAL:-0}" = 1 ]; then
  export PROJECT_ID="${PROJECT_ID:-local}" REGION="${REGION:-local}" FT_BACKEND=local
  # Namespace LOCAL state so a LOCAL run can NEVER clobber a live cloud run's map
  # (2026-08-20 root cause). Set here — before gen_topology/provision_local, which
  # run before lib.sh is sourced. lib.sh re-affirms the same values.
  export NODES_JSON="${NODES_JSON:-$FT_DIR/nodes.local.json}"
  export FT_TOPO="${FT_TOPO:-$FT_DIR/topology.local.json}"
else
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
fi   # end of the non-LOCAL config block

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
  echo "==> generating deterministic topology${SMOKE:+ (SMOKE — trimmed 4-node set)}${LOCAL:+ (LOCAL → topology.local.json, no tfvars)}"
  # Namespace the topology file per backend + skip tfvars for LOCAL (docker, no
  # terraform), so a LOCAL generation can never clobber a live cloud run's map or
  # its terraform tfvars (the 2026-08-20 root cause). TOPO_OUT/$FT_TOPO is set by
  # the LOCAL branch; cloud uses the default topology.json + writes tfvars.
  SILT_BIN="$FT_DIR/.silt-local" BOND_MODE="${BOND_MODE:-fast}" SMOKE="${SMOKE:-0}" \
    TOPO_OUT="${FT_TOPO:-$FT_DIR/topology.json}" \
    WRITE_TFVARS="$([ "${LOCAL:-0}" = 1 ] && echo 0 || echo 1)" \
    python3 topology.py
}

tf() { terraform -chdir="$FT_DIR/terraform" "$@"; }

# Pre-flight the external-IP quota per region BEFORE apply (§F): a shortfall
# only surfaces ~30s/instance into a doomed apply. Count every ADDRESS-consuming
# node per region (public nodes AND the natgw — it holds the masquerade IP;
# natted/island/internal_only are genuinely address-free) from topology.json and
# compare to each region's live IN_USE_ADDRESSES headroom. PREFLIGHT=0 skips.
# Zone E2 capacity is not queryable via the API, so this covers the IP quota
# only (the other half of §F).
preflight_quota() {
  [ "${PREFLIGHT:-1}" = 0 ] && { echo "==> preflight: skipped (PREFLIGHT=0)"; return 0; }
  [ -f "${FT_TOPO:-$FT_DIR/topology.json}" ] || return 0
  echo "==> preflight: external-IP (IN_USE_ADDRESSES) quota per region"
  # Count the PUBLIC nodes exactly as terraform does (main.tf `public_nodes`):
  # natted/natgw egress via Cloud NAT, the equivocation ISLAND is
  # no-external-IP by design (#494), and internal_only nodes (the ECONOMY
  # killable stores, #495) carry no address — all zero IN_USE_ADDRESSES.
  # internal_only lives only in the tfvars gen_topology writes, so prefer it
  # over topology.json. This counter once lagged terraform's predicate (it
  # skipped only natted/natgw) and refused an ECONOMY run as "needs 13 of 8"
  # when the real demand was 7 — the counter and main.tf must not drift.
  local ipsrc="${FT_TOPO:-$FT_DIR/topology.json}"
  [ -f "$FT_DIR/terraform/topology.auto.tfvars.json" ] && ipsrc="$FT_DIR/terraform/topology.auto.tfvars.json"
  python3 - "$ipsrc" > /tmp/ft_ipneeds.txt <<'PY'
import json, sys
from collections import Counter
nodes = json.load(open(sys.argv[1]))["nodes"]
c = Counter()
for n in nodes.values():
    # Mirror terraform main.tf's ADDRESS consumers, not just its public_nodes set:
    # the natgw is excluded from public_nodes (its own resource block) but its
    # network_interface carries access_config — it IS the masquerade box, so it
    # holds an external IP and counts against IN_USE_ADDRESSES like any public
    # node. Skipping it undercounted the natgw's region by 1 (at the 8/8 exact
    # fit of the full SYBILS+MATURING+ECONOMY topology, the margin this hides is
    # the whole margin). natted/island/internal_only remain genuinely address-free.
    if n.get("role") in ("natted", "island") or n.get("internal_only"):
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
  # Persistent-network mode (harness-hardening c): PERSIST_NET=1 reuses the
  # long-lived VPC owned by terraform/network (create once: ./cloudtest.sh net-up)
  # instead of building/destroying a per-run one (~minutes/run). Record the mode
  # so a fresh-shell destroy evaluates the same config it applied with.
  printf '%s' "${PERSIST_NET:-0}" > "$FT_DIR/.persist_net"
  [ "${PERSIST_NET:-0}" = 1 ] && echo "==> network: PERSISTENT (silt-persist — run 'net-up' once beforehand)" || echo "==> network: per-run (PERSIST_NET=1 + 'net-up' to reuse one across runs)"
  tf apply -input=false -auto-approve \
    -var "persistent_network=$([ "${PERSIST_NET:-0}" = 1 ] && echo true || echo false)" \
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
  tf output -json nodes > "${NODES_JSON:-$FT_DIR/nodes.json}"
  # Terraform's node output carries instance_name/zone/ips/role but NOT the silt
  # NodeID — yet scenarios.sh reads node_field <n> nodeid (the #184 drills derive
  # peer IDs from it). topology.json HAS the deterministic nodeid per node, so merge
  # it in here. Without this every 184-* flow crashed with KeyError: 'nodeid' and
  # the adversarial consensus drills never ran on GCP (blind cloud finding #1).
  python3 - "${NODES_JSON:-$FT_DIR/nodes.json}" "${FT_TOPO:-$FT_DIR/topology.json}" <<'PY'
import json, sys
nodes = json.load(open(sys.argv[1]))
topo  = json.load(open(sys.argv[2]))["nodes"]
for name, n in nodes.items():
    if name in topo and topo[name].get("nodeid"):
        n["nodeid"] = topo[name]["nodeid"]
json.dump(nodes, open(sys.argv[1], "w"), indent=2)
PY
  echo "    nodes written ($(python3 -c "import json;print(len(json.load(open('${NODES_JSON:-$FT_DIR/nodes.json}'))))") instances, nodeid merged)"
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
      # Split pending into CORE vs the OPT-IN cohort (sybils + maturers). The cohort
      # exists only for the C2/maturing corners; if it fails to boot but every core
      # node is up, the base corners (convergence, publish, fault-tolerance,
      # takedown, …) are still fully gradeable — so DON'T abort the whole billable
      # run on the opt-in cohort (flow 10 guards its own maturer-live premise and
      # GAPs honestly, never a false FAIL). Only a core node not coming up is a real
      # infra failure that invalidates the run.
      core_pending=""; syb_pending=""
      for n in $pending; do
        case "$(node_field "$n" role)" in
          sybil|maturer) syb_pending="$syb_pending $n" ;;
          *) core_pending="$core_pending $n" ;;
        esac
      done
      # shellcheck disable=SC2086
      capture_failed_nodes $pending
      if [ -n "$core_pending" ]; then
        echo "    timed out — CORE nodes not ready:$core_pending (sybils:$syb_pending) — aborting (a core-node startup failure invalidates the run)"
        return 1
      fi
      echo "    timed out — only the OPT-IN cohort (sybils/maturers) is not ready:$syb_pending — PROCEEDING; the C2/maturing corners will GAP honestly (their journals captured above), the base corners still grade"
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
  # RSS/memory telemetry (Phase 1.3): sample each node's cgroup memory across the run
  # into a committed rss-<RUN_ID>.jsonl, so the MATURING OOM "return-to-2GB" headline
  # carries a measured envelope, not just the absence of a crash (build-immutable #7).
  # Strictly additive and failure-tolerant; stopped + summarised before teardown.
  mem_sampler_start
  # The node-liveness precondition runs LAST, before teardown: scan the whole cohort
  # for OOM-kills / crash-loops and surface it as a first-class finding (a run whose
  # nodes died cannot grade its flows — proof-oom field corroboration 2026-08-17).
  { run_all_scenarios || true; mem_sampler_stop; scan_node_memory || true; scan_node_liveness || true; } 2>&1 | tee -a "$FT_DIR/console-$RUN_ID.log"
}

report() {
  echo "==> report"; RUN_ID="$RUN_ID" ./gen_report.sh
  # Archive per run: results.jsonl/report.md are OVERWRITTEN by the next run, which
  # erased the pass/fail history needed to tell a regression from a never-passed
  # flow (this session had to reconstruct run c815091's verdicts from memory).
  mkdir -p "$FT_DIR/archive"
  cp -f "$FT_DIR/results.jsonl" "$FT_DIR/archive/results-$RUN_ID.jsonl" 2>/dev/null || true
  cp -f "$FT_DIR/report.md"     "$FT_DIR/archive/report-$RUN_ID.md"     2>/dev/null || true
  # Also emit a TOP-LEVEL per-RUN_ID report + results (same convention as
  # console-$RUN_ID.log). archive/ is git-ignored, so committing a run's grade as
  # EVIDENCE meant force-committing the MUTABLE report.md — which the next run
  # overwrites, the exact cause of the fresh-eyes audit's "committed report.md is a
  # stale pre-fix run". A per-RUN_ID top-level copy is the file to force-commit, and
  # it can never be clobbered by a later run.
  cp -f "$FT_DIR/report.md"     "$FT_DIR/report-$RUN_ID.md"     2>/dev/null || true
  cp -f "$FT_DIR/results.jsonl" "$FT_DIR/results-$RUN_ID.jsonl" 2>/dev/null || true
  echo "    archived → report-$RUN_ID.md + results-$RUN_ID.jsonl (top-level, force-commit these as the run's grade) + archive/ copies (evidence: flow-evidence-$RUN_ID.log, console-$RUN_ID.log)"
}

teardown() {
  # TEARDOWN=0 is the re-drive alias of KEEP_UP: leave the fleet standing so a
  # fix can redeploy + re-run a flow subset (FLOWS="…" ./cloudtest.sh run) in ~1
  # minute instead of a ~10-minute provision cycle. Convergence aid only — the
  # exit-gate/RC artifact stays one clean uninterrupted sheet.
  [ "${TEARDOWN:-1}" = 0 ] && { echo "==> TEARDOWN=0 — fleet left standing for re-drive (FLOWS=… ./cloudtest.sh run | ./cloudtest.sh down)"; return; }
  [ "${KEEP_UP:-0}" = 1 ] && { echo "==> KEEP_UP=1 — leaving the network up. './cloudtest.sh down' when done."; return; }
  echo "==> DESTROY (run=$RUN_ID)"
  # Do NOT swallow destroy stderr (§D): a partial-apply state makes `terraform destroy`
  # fail, and hiding its error makes the destroy-failed→nuke handoff (and the VPC leak it
  # can cause) invisible. Let terraform's error print so the fallback is diagnosable.
  local pn; pn="${PERSIST_NET:-$(cat "$FT_DIR/.persist_net" 2>/dev/null || echo 0)}"
  tf destroy -input=false -auto-approve \
    -var "persistent_network=$([ "$pn" = 1 ] && echo true || echo false)" \
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

# ── LOCAL=1 backend: docker provisioning (see docs/thinking/2026-08-20-harness-local-first.md) ──
LOCAL_IMG="silt-cloudtest-local"
local_prefix() { printf 'silt-ft-%s' "$RUN_ID"; }

check_prereqs_local() { need docker; need go; need python3; }

build_binary_local() {
  local arch; arch="$(docker version --format '{{.Server.Arch}}' 2>/dev/null || echo amd64)"
  echo "==> building silt (linux/$arch for the docker backend) @ $RUN_ID"
  ( cd "$REPO_ROOT" && CGO_ENABLED=0 GOOS=linux GOARCH="$arch" \
      go build -trimpath -ldflags '-s -w' -o "$FT_DIR/silt-linux-local" ./cmd/silt )
  ( cd "$REPO_ROOT" && go build -o "$FT_DIR/.silt-local" ./cmd/silt )   # for topology id-gen
}

# provision_local — the terraform-apply analogue: one container per topology node
# (natgw/natted excluded — real NAT is integration/nat's job locally), on a bridge
# network reproducing the topology's static IPs, with the binary + shims mounted at
# their REAL paths and the unit file + baked argv written exactly as GCP metadata
# startup does. nodes.json gets the same shape terraform output produces, so every
# lib.sh reader works unchanged.
provision_local() {
  local prefix net; prefix="$(local_prefix)"; net="${prefix}-net"
  printf '%s\n' "$RUN_ID" > "$FT_DIR/.last_run_id"
  echo "==> docker image ($LOCAL_IMG)"
  docker build -q -t "$LOCAL_IMG" "$FT_DIR/local" >/dev/null
  echo "==> docker network $net (10.16.0.0/12 — covers the topology's per-region subnets 10.2x.0.0/16)"
  docker network inspect "$net" >/dev/null 2>&1 || docker network create --subnet 10.16.0.0/12 "$net" >/dev/null
  # nodes.json in the terraform-output shape (instance_name/zone/role/nodeid/ip).
  python3 - "$FT_TOPO" "$NODES_JSON" "$prefix" <<'PY'
import json, sys
topo = json.load(open(sys.argv[1]))["nodes"]
out = {}
for name, n in topo.items():
    if n.get("role") in ("natgw", "natted"):
        continue  # LOCAL v1: real NAT stays integration/nat's job — 9-cross-nat SKIPs
    out[name] = {"instance_name": f"{sys.argv[3]}-{name}", "zone": "local",
                 "role": n["role"], "nodeid": n.get("nodeid", ""), "ip": n["ip"]}
json.dump(out, open(sys.argv[2], "w"), indent=2)
print(f"    nodes.json written ({len(out)} containers; natgw/natted excluded)")
PY
  local name inst ip argv
  for name in $(python3 -c "import json;print(' '.join(json.load(open('$NODES_JSON')).keys()))"); do
    inst="${prefix}-${name}"
    ip="$(python3 -c "import json;print(json.load(open('$NODES_JSON'))['$name']['ip'])")"
    argv="$(python3 -c "import json;print(json.load(open('$FT_TOPO'))['nodes']['$name']['argv'])")"
    argv="${argv#daemon }"; argv="daemon $argv"
    docker rm -f "$inst" >/dev/null 2>&1 || true
    docker run -d --name "$inst" --network "$net" --ip "$ip" \
      --label "cloudtest-local=$RUN_ID" \
      "$LOCAL_IMG" >/dev/null
    # COPY (never bind-mount) the binary + shims: a host-side edit of a bind-mounted
    # file mid-run leaves the container reading a torn/stale inode — the shim
    # "syntax error line 59" trap this mode's own first run hit.
    docker cp "$FT_DIR/silt-linux-local"        "$inst:/usr/local/bin/silt"        >/dev/null
    docker cp "$FT_DIR/local/shims/systemctl"   "$inst:/usr/bin/systemctl"         >/dev/null
    docker cp "$FT_DIR/local/shims/journalctl"  "$inst:/usr/bin/journalctl"        >/dev/null
    docker cp "$FT_DIR/local/shims/sudo"        "$inst:/usr/bin/sudo"              >/dev/null
    docker cp "$FT_DIR/local/shims/silt-run"    "$inst:/usr/local/bin/silt-run"    >/dev/null
    docker exec "$inst" bash -c "chmod +x /usr/local/bin/silt /usr/bin/systemctl /usr/bin/journalctl /usr/bin/sudo /usr/local/bin/silt-run"
    docker exec "$inst" bash -c "
      mkdir -p /var/lib/silt /etc/systemd/system
      printf '%s\n' '$argv' > /var/lib/silt-argv.baked
      printf '[Service]\nExecStart=/usr/local/bin/silt %s\n' '$argv' > /etc/systemd/system/silt.service
      systemctl start silt.service"
    echo "    up: $name ($ip)"
  done
}

teardown_local() {
  [ "${TEARDOWN:-1}" = 0 ] && { echo "==> TEARDOWN=0 — containers left standing for re-drive (FLOWS=… LOCAL=1 ./cloudtest.sh run | … down)"; return; }
  [ "${KEEP_UP:-0}" = 1 ] && { echo "==> KEEP_UP=1 — containers left running. 'LOCAL=1 ./cloudtest.sh down' when done."; return; }
  # A fresh shell's RUN_ID embeds a different pid — resolve the last run's id,
  # same convention as nuke.
  local target="$RUN_ID"
  [ -f "$FT_DIR/.last_run_id" ] && target="$(cat "$FT_DIR/.last_run_id")"
  echo "==> removing local containers + network (silt-ft-$target-*)"
  docker ps -aq --filter "label=cloudtest-local=$target" | xargs -r docker rm -f >/dev/null 2>&1 || true
  docker network rm "silt-ft-${target}-net" >/dev/null 2>&1 || true
}

nuke_local() { # remove EVERY local cloudtest container/network regardless of run id
  docker ps -aq --filter "label=cloudtest-local" | xargs -r docker rm -f >/dev/null 2>&1 || true
  docker network ls --format '{{.Name}}' | grep -E '^silt-ft-.*-net$' | xargs -r -n1 docker network rm >/dev/null 2>&1 || true
  echo "==> local nuke: all cloudtest-local containers + silt-ft-*-net networks removed"
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
  if [ -z "${RUN_MECHANISM:-}" ] || [ -z "${RUN_REPRO:-}" ] || [ -z "${RUN_LOCAL_PROOF:-}" ]; then
    cat >&2 <<'EOF'
✗ pre-flight gate (build-immutables #6/#7): a billable cloud run needs a written justification
  AND a LOCAL PROOF THAT PASSES. A cloud run CONFIRMS what a local test already proved green; it
  never DISCOVERS whether an integration works.

  WHY THIS GATE EXISTS (2026-08-19): a run was spent to "test" the economy integration
  (flow_economy_repair) that had NEVER passed locally end-to-end. It GAPped on a foreseeable
  publish-latency issue — a billable run burned to discover what a laptop would have shown for free.
  This gate makes that structurally impossible.

  RUN_LOCAL_PROOF must be a command this gate RUNS; the run proceeds ONLY if it EXITS 0.
  For a genuine liveness/scale run with no local analogue, set it EXPLICITLY to "n/a: <reason>".

  Re-run with all THREE set, e.g.:
    RUN_MECHANISM="economy on the wire — flow_economy_repair (Phase 2 exit gate)" \
    RUN_REPRO="the full reconstruct→bounty loop proven locally, green" \
    RUN_LOCAL_PROOF="go test ./e2e -run TestRepairBountyPaysOnTheWire -count=1" \
    ECONOMY=1 ./cloudtest.sh
EOF
    exit 2
  fi
  case "$RUN_LOCAL_PROOF" in
    n/a:*|N/A:*)
      echo "⚠ pre-flight: RUN_LOCAL_PROOF is an EXPLICIT n/a — '$RUN_LOCAL_PROOF'. Proceeding on the operator's stated word that no local proof applies (a liveness/scale run). This is the ONLY bypass, and it is logged."
      ;;
    *)
      echo "==> pre-flight: running the LOCAL PROOF before spending a cent — '$RUN_LOCAL_PROOF'"
      if ! ( cd "$REPO_ROOT" && eval "$RUN_LOCAL_PROOF" ); then
        echo "✗ pre-flight gate: the local proof FAILED — the integration this run would confirm is NOT green locally. NO billable run launched. Reproduce and fix it on a laptop first (build-immutable #7)." >&2
        exit 2
      fi
      echo "✓ pre-flight: local proof PASSED — the cloud run now CONFIRMS a green local integration, not a guess."
      ;;
  esac
  { echo "run=${RUN_ID:-unknown}  ts=$(date -u +%FT%TZ 2>/dev/null || echo unknown)"
    echo "RUN_MECHANISM:   $RUN_MECHANISM"
    echo "RUN_REPRO:       $RUN_REPRO"
    echo "RUN_LOCAL_PROOF: $RUN_LOCAL_PROOF"
    echo "---"
  } >> run-justification.log
  echo "✓ pre-flight gate: justification + local proof recorded (build-immutables #6/#7) — proceeding to a BILLABLE run."
}

if [ "${LOCAL:-0}" = 1 ]; then
  case "${1:-all}" in
    all)
      echo "==> LOCAL=1: free run — the billable pre-flight gate does not apply (it guards money)"
      check_prereqs_local; build_binary_local; gen_topology
      : > "$FT_DIR/results.jsonl"
      printf '# run=%s (LOCAL) — no scenarios have completed yet\n' "$RUN_ID" > "$FT_DIR/report.md"
      trap teardown_local EXIT
      provision_local; wait_ready; run_scenarios; report
      ;;
    up)   check_prereqs_local; build_binary_local; gen_topology; provision_local; wait_ready; echo "local network up (run=$RUN_ID) — 'LOCAL=1 ./cloudtest.sh run' to grade, '… down' to remove" ;;
    run)  run_scenarios; report ;;
    report) report ;;
    down) KEEP_UP=0 teardown_local ;;
    nuke) nuke_local ;;
    *) echo "usage: LOCAL=1 ./cloudtest.sh [all|up|run|report|down|nuke]"; exit 1 ;;
  esac
  exit 0
fi

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
  net-up)
    # Create (or update) the PERSISTENT network the PERSIST_NET=1 runs attach to.
    # Own state under terraform/network — a run's destroy never touches it. Idle
    # cost is $0 (VPC/subnets/firewalls free; Cloud NAT bills only processed egress).
    terraform -chdir="$FT_DIR/terraform/network" init -input=false >/dev/null
    terraform -chdir="$FT_DIR/terraform/network" apply -input=false -auto-approve \
      -var "project_id=$PROJECT_ID" -var "default_region=$REGION"
    echo "persistent network up — launch runs with PERSIST_NET=1"
    ;;
  net-down)
    terraform -chdir="$FT_DIR/terraform/network" destroy -input=false -auto-approve \
      -var "project_id=$PROJECT_ID" -var "default_region=$REGION"
    ;;
  heap)
    # ./cloudtest.sh heap <node> [heap|goroutine|allocs] — pull a live pprof profile
    # from a node to attribute the MATURING consensus-node OOM. Needs the run to have
    # been launched with DEBUG_PROFILE=1 (adds -debug-addr to every node). Streams the
    # binary profile out base64-wrapped (ssh stdout is not binary-safe) and decodes via
    # python3 (portable across mac/linux base64 flag differences).
    . ./lib.sh
    node="${2:?usage: ./cloudtest.sh heap <node> [heap|goroutine|allocs]}"
    prof="${3:-heap}"
    node_exists "$node" || { echo "unknown node '$node' — have: $(node_names)"; exit 1; }
    out="$FT_DIR/${prof}-${node}-${RUN_ID}.pprof"
    echo "==> pulling '$prof' profile from $node (local debug port ${DEBUG_PORT:-6060})…"
    ssh_node "$node" "curl -s http://127.0.0.1:${DEBUG_PORT:-6060}/debug/pprof/${prof} | base64" \
      | python3 -c 'import sys,base64; sys.stdout.buffer.write(base64.b64decode(sys.stdin.read()))' > "$out" 2>/dev/null || true
    if [ -s "$out" ]; then
      echo "wrote $out ($(wc -c < "$out") bytes)"
      echo "  inspect: go tool pprof -inuse_space -top $out   (or -inuse_objects / a -base <earlier>.pprof diff)"
    else
      rm -f "$out"
      echo "no profile captured — was the run launched with DEBUG_PROFILE=1? (that adds -debug-addr; the node's 127.0.0.1:${DEBUG_PORT:-6060} must be listening)"
      exit 1
    fi
    ;;
  *) echo "usage: ./cloudtest.sh [setup|all|up|run|report|down|nuke|heap]"; exit 1; ;;
esac
