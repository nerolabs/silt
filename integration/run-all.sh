#!/usr/bin/env bash
# integration/run-all.sh — the clone-and-run field-test experience.
#
# Runs the local Docker suites one at a time (each assumes exclusive use of its
# topology), captures each one's real RESULT + duration, and writes a shareable
# consolidated report.md alongside a terminal summary. Exit 0 iff every suite
# that ran is PASS or a deliberately-reproduced FINDING; any FAIL exits 1.
#
# Usage:
#   ./integration/run-all.sh                 # the fast gate set (default)
#   FULL=1 ./integration/run-all.sh          # + the slow suites (soak, upgrade)
#   SUITES="consensus bond nat" ./integration/run-all.sh   # an explicit subset
#   SOAK_DURATION=120 ./integration/run-all.sh FULL=1
#
# Each suite still stands alone (./integration/<name>/run.sh); this only
# orchestrates them and rolls up the result. See integration/README.md.
set -uo pipefail
cd "$(dirname "$0")"
HERE="$(pwd)"
# shellcheck source=/dev/null
. "$HERE/lib.sh"

OUT="${OUT:-$HERE/.run-all}"
REPORT="$OUT/report.md"
mkdir -p "$OUT"

# ── suite catalog: name | tier | per-suite timeout(s) | the M0 claim it gates ──
# tier: gate = cheap, fast, run by default; slow = opt-in via FULL=1.
SUITES_CATALOG=(
  "consensus|gate|300|Objective on-chain-bond fork-choice: fork under partition, reorg to the heavier chain on heal"
  "bond|gate|180|Proof-of-space-time bond cost (C1): a real plot is dear to make, cheap to verify, shortcut rejected"
  "redteam|gate|300|#184 accountability: equivocator slashed, forged block rejected, low-bond proposer refused"
  "takedown|gate|180|Per-operator, existence-checked, reversible takedown"
  "privacy|gate|360|Publisher unlinkability: the default chain refuses a durable file→publisher link (refuse-to-surveil), the private path works, token-quorum authorizes without identity"
  "nat|gate|180|Cross-NAT publish→fetch bit-perfect via the relay"
  "audit|gate|240|A liar deletes shards but keeps proofs → the loss is caught over the wire and repaired"
  "economy|gate|180|Per-byte earning + blind-signed, publisher-unlinkable credits"
  "churn|gate|540|Repair-under-churn: kill holders, caretaker reconstructs from parity + re-scatters, bit-perfect"
  "soak|slow|400|Sustained load + gentle churn: bit-perfect throughout, no crash-loop, bounded memory"
  "durability|slow|1800|Durability under permanent loss: shrink the swarm, caretaker reconstructs+re-scatters, content outlives the nodes (surfaces the durability↔retrievability boundary)"
  "retrieval|slow|900|Retrieval/discoverability at scale + identity churn: cold-fetch success-rate floor (reproduces #43)"
  "upgrade|slow|180|Rolling binary upgrade on a persisted store (reproduces the #237 format-migration FINDING)"
)

# Resolve the list of suites to run.
declare -a RUN
if [ -n "${SUITES:-}" ]; then
  for s in $SUITES; do RUN+=("$s"); done
else
  for row in "${SUITES_CATALOG[@]}"; do
    IFS='|' read -r name tier _ _ <<<"$row"
    { [ "$tier" = gate ] || [ "${FULL:-0}" = 1 ]; } && RUN+=("$name")
  done
fi

catalog_field() { # catalog_field <name> <1=tier|2=timeout|3=claim>
  local name="$1" idx="$2" row
  for row in "${SUITES_CATALOG[@]}"; do
    IFS='|' read -r n tier to claim <<<"$row"
    if [ "$n" = "$name" ]; then
      case "$idx" in 1) echo "$tier" ;; 2) echo "$to" ;; 3) echo "$claim" ;; esac
      return
    fi
  done
}

# ── prereqs ────────────────────────────────────────────────────────────────
ft_require docker go awk grep || exit 1
ft_docker_up || { echo "docker is not running (start Docker / colima first)"; exit 1; }

echo "silt field-test suite — running ${#RUN[@]} suite(s): ${RUN[*]}"
echo "reports → $OUT"
echo

# ── report header ──────────────────────────────────────────────────────────
{
  echo "# silt field-test report"
  echo
  echo "_Local Docker suites, one topology each, asserting real daemon/disk/socket behavior._"
  echo
  echo "| suite | status | time | M0 claim under test |"
  echo "|---|---|---|---|"
} >"$REPORT"

# ── run each suite ─────────────────────────────────────────────────────────
declare -a NAMES STATUSES
overall=0
for name in "${RUN[@]}"; do
  dir="$HERE/$name"
  claim="$(catalog_field "$name" 3)"; [ -n "$claim" ] || claim="(uncataloged suite)"
  timeout_s="$(catalog_field "$name" 2)"; [ -n "$timeout_s" ] || timeout_s=600
  log="$OUT/$name.log"

  if [ ! -x "$dir/run.sh" ]; then
    printf '  %-11s %sSKIP%s  (no run.sh)\n' "$name" "$C_YELLOW" "$C_RESET"
    echo "| $name | SKIP | — | no run.sh |" >>"$REPORT"
    NAMES+=("$name"); STATUSES+=("SKIP"); continue
  fi

  # Slow suites take an env knob; pass it through if the caller set one.
  extra=()
  [ "$name" = soak ] && [ -n "${SOAK_DURATION:-}" ] && extra+=("DURATION=$SOAK_DURATION")

  printf '  %-11s running…' "$name"
  t0=$(date +%s)
  ( cd "$dir" && timeout "$timeout_s" env ${extra[@]+"${extra[@]}"} ./run.sh ) >"$log" 2>&1
  rc=$?
  t1=$(date +%s); dur=$((t1 - t0))

  line="$(ft_result_line "$log")"
  status="$(ft_classify "$rc" "$line")"
  [ "$rc" -eq 124 ] && line="${line:-timed out after $(ft_human_time "$timeout_s")}"
  col="$(ft_status_color "$status")"
  printf '\r  %-11s %s%-7s%s %s\n' "$name" "$col" "$status" "$C_RESET" "$C_DIM$(ft_human_time "$dur")$C_RESET"

  echo "| $name | $status | $(ft_human_time "$dur") | $claim |" >>"$REPORT"
  NAMES+=("$name"); STATUSES+=("$status")
  [ "$status" = FAIL ] && overall=1
done

# ── roll-up ────────────────────────────────────────────────────────────────
pass=0; finding=0; fail=0; skip=0
for st in "${STATUSES[@]}"; do
  case "$st" in PASS) pass=$((pass + 1));; FINDING) finding=$((finding + 1));; FAIL) fail=$((fail + 1));; SKIP) skip=$((skip + 1));; esac
done

{
  echo
  echo "**$pass pass · $finding finding · $fail fail · $skip skip** — per-suite logs in \`$(basename "$OUT")/\`."
  echo
  echo "Status legend: **PASS** = green gate · **FINDING** = a deliberately-reproduced open"
  echo "defect (e.g. \`upgrade\` reproduces #237) · **FAIL** = a real regression to investigate."
} >>"$REPORT"

echo
echo "──────────────────────────────────────────────────────"
printf 'summary: %s%d pass%s · %s%d finding%s · %s%d fail%s · %d skip\n' \
  "$C_GREEN" "$pass" "$C_RESET" "$C_YELLOW" "$finding" "$C_RESET" "$C_RED" "$fail" "$C_RESET" "$skip"
echo "report:  $REPORT"
[ "$overall" -eq 0 ] && echo "${C_GREEN}OK${C_RESET}: no failing suites" || echo "${C_RED}FAIL${C_RESET}: $fail suite(s) failed"
exit "$overall"
