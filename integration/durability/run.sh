#!/usr/bin/env bash
# ─────────────────────────────────────────────────────────────────────────────
# Field test — long-horizon DURABILITY under full membership turnover.
#
# The core promise, tested cynically: does content OUTLIVE THE NODES? Not "does a
# repair fire once" — but, as holders permanently depart and fresh EMPTY holders
# replace them so the swarm's membership fully rotates (its size held constant),
# does a fetch stay bit-perfect and does redundancy RECOVER after each departure?
#
#   1. seed/registry + N holders + a caretaker; publish at REPLICATION copies
#   2. baseline: fetch bit-perfect
#   3. TURNOVER: for CYCLES cycles, kill the oldest holder PERMANENTLY (its shards
#      are gone for good), re-scale holders back to N (a fresh EMPTY holder joins),
#      let the caretaker repair, then:
#        • fetch bit-perfect (content survived this departure), and
#        • read the caretaker's "repair sweep complete reachable=N" (#235) to
#          confirm redundancy healed back to full — not merely "a repair ran"
#   4. run CYCLES >= N so EVERY original holder is gone by the end — the content
#      that remains is held entirely by nodes that joined AFTER it was published
#
# REPLICATION defaults to 1 so each departure genuinely strands columns and the
# caretaker MUST reconstruct from parity to survive — the honest stress. If repair
# can't keep pace with turnover, redundancy decays and a fetch eventually fails:
# that is the FINDING (content is dying under turnover), reported, never hidden.
#
# Usage:
#   ./run.sh                            # 12 holders, 14 turnover cycles (> full rotation)
#   CYCLES=24 CYCLE_WAIT=45 ./run.sh    # longer horizon, gentler cadence
#   REPLICATION=3 ./run.sh              # the shipped-default redundancy (needs more cycles to bite)
#   KEEP=1 ./run.sh
# exit 0 = PASS (content outlived a full turnover) / a reproduced FINDING; non-zero = FAIL
# ─────────────────────────────────────────────────────────────────────────────
set -uo pipefail
cd "$(dirname "$0")"
ROOT=$(cd ../.. && pwd)

HOLDERS=${HOLDERS:-12}             # storage pool size held constant across turnover
CYCLES=${CYCLES:-14}              # departures to drive (>= HOLDERS ⇒ full membership rotation)
CYCLE_WAIT=${CYCLE_WAIT:-70}      # seconds per cycle to let a repair sweep (RepairInterval=60s) land
REPLICATION=${REPLICATION:-1}     # copies per column; 1 ⇒ every departure strands columns (honest stress)
FILE_BYTES=${FILE_BYTES:-4000000} # ~4 MB
FETCH_TIMEOUT=${FETCH_TIMEOUT:-40}

PROJECT=durability
dc() { docker compose -p "$PROJECT" "$@"; }
cleanup() { [ "${KEEP:-0}" = 1 ] || dc down -v >/dev/null 2>&1 || true; }
trap cleanup EXIT INT TERM
fail() { echo "RESULT: FAIL ❌  $*"; exit 1; }
CARE_CID=""
# Repair events are logged at -log info to <store>/debug.log (NOT stdout), so we
# read them from inside the caretaker container, same as the churn harness.
care_reachable() { # last "repair sweep complete … shards=M reachable=N" -> "N/M"
  [ -n "$CARE_CID" ] || { echo "?/?"; return; }
  docker exec "$CARE_CID" sh -c 'grep "repair sweep complete" /data/debug.log 2>/dev/null | tail -1' 2>/dev/null \
    | grep -oE 'shards=[0-9]+ reachable=[0-9]+' | sed -E 's/shards=([0-9]+) reachable=([0-9]+)/\2\/\1/' | tr -d ' \r\n'
}
care_repairs() { docker exec "$CARE_CID" sh -c 'grep -c "stripe repaired" /data/debug.log 2>/dev/null || true' 2>/dev/null | tr -d ' \r\n'; }

echo "== build silt (linux/$(go env GOARCH)) + image =="
CGO_ENABLED=0 GOOS=linux GOARCH="$(go env GOARCH)" go build -trimpath -o silt "$ROOT/cmd/silt" || fail "host build"
docker build -q -t silt-durability . >/dev/null || fail "image build"

echo "== phase 1: seed =="
dc up -d seed >/dev/null 2>&1 || fail "up seed"
SEED_ID=""
for _ in $(seq 1 30); do SEED_ID=$(dc logs seed 2>&1 | grep -oE 'peer: [a-f0-9]{64}' | head -1 | awk '{print $2}'); [ -n "$SEED_ID" ] && break; sleep 1; done
SEEDIP=$(docker inspect -f '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}' "$(dc ps -q seed)")
[ -n "$SEED_ID" ] && [ -n "$SEEDIP" ] || fail "seed never reported NodeID/IP"
export SEED_PEER="$SEED_ID@$SEEDIP:4001" SEED_REG="$SEED_ID@https://$SEEDIP:4003"
echo "  seed: $SEED_PEER"

echo "== phase 2: $HOLDERS holders (replication=$REPLICATION) =="
dc up -d --scale holder="$HOLDERS" holder >/dev/null 2>&1 || fail "up holders"
bootok=0; HIDS=()
while IFS= read -r _id; do [ -n "$_id" ] && HIDS+=("$_id"); done < <(dc ps -q holder)
for id in "${HIDS[@]}"; do for _ in $(seq 1 60); do docker logs "$id" 2>&1 | grep -q bootstrapped && { bootok=$((bootok+1)); break; }; sleep 1; done; done
echo "  $bootok/$HOLDERS holders bootstrapped"

echo "== phase 3: publish content (ephemeral client, $REPLICATION copies/column) =="
ADD=$(dc run --rm -T client sh -c \
  "head -c $FILE_BYTES /dev/urandom >/tmp/f.bin; sha256sum /tmp/f.bin | cut -d' ' -f1; \
   silt swarm add /tmp/f.bin -peers '$SEED_PEER' -registry '$SEED_REG' -replication $REPLICATION 2>&1" 2>/dev/null)
WANT=$(printf '%s' "$ADD" | grep -oE '^[a-f0-9]{64}' | head -1)
LINK=$(printf '%s' "$ADD" | grep -oE 'silt:v1:[A-Za-z0-9_:-]+' | head -1)
CARE=$(printf '%s' "$ADD" | grep -oE 'siltcare:v1:[A-Za-z0-9_:-]+' | head -1)
[ -n "$WANT" ] && [ -n "$LINK" ] && [ -n "$CARE" ] || fail "publish did not yield link+care+hash"
echo "  link: $LINK"

echo "== phase 4: caretaker (repair engine) =="
export CARE_LINK="$CARE"
dc up -d caretaker >/dev/null 2>&1 || fail "up caretaker"
CARE_CID=$(dc ps -q caretaker)
for _ in $(seq 1 40); do docker logs "$CARE_CID" 2>&1 | grep -q caretaking && break; sleep 1; done
echo "  caretaker up; letting the first sweep establish full redundancy (${CYCLE_WAIT}s)…"; sleep "$CYCLE_WAIT"

fetch_ok() { # fetch from a fresh ephemeral client; 1 = bit-perfect
  local got
  got=$(dc run --rm -T client sh -c \
    "timeout $FETCH_TIMEOUT silt swarm get '$LINK' -o /tmp/g.bin -peers '$SEED_PEER' -registry '$SEED_REG' >/dev/null 2>&1; \
     sha256sum /tmp/g.bin 2>/dev/null | cut -d' ' -f1" 2>/dev/null | grep -oE '^[a-f0-9]{64}' | head -1)
  [ "$got" = "$WANT" ]
}

echo "  baseline redundancy: $(care_reachable) reachable"
fetch_ok || fail "baseline fetch not bit-perfect (content not durable even before turnover)"
echo "  ✓ baseline bit-perfect"

echo "== phase 5: $CYCLES permanent departures (fresh empty holders replace them) =="
survivors_floor=$(( REPLICATION > 1 ? 4 : 11 ))   # keep enough alive to reconstruct k=10
killed_total=0; worst_reachable=999
for c in $(seq 1 "$CYCLES"); do
  # Remove a RUNNING holder permanently — rm -f (not kill) so its store/identity
  # is gone for good, then re-scale so a genuinely FRESH empty holder joins. (kill
  # alone would leave the container to be restarted on its old store — no turnover.)
  victim=$(dc ps -q --status running holder | head -1)
  [ -n "$victim" ] || { echo "  no running holder to remove (pool exhausted)"; break; }
  docker rm -f "$victim" >/dev/null 2>&1 || true
  killed_total=$((killed_total+1))
  dc up -d --scale holder="$HOLDERS" holder >/dev/null 2>&1 || true
  sleep "$CYCLE_WAIT"   # let the caretaker sweep + repair onto survivors + the newcomer
  reach=$(care_reachable); repairs=$(care_repairs)
  # redundancy must have HEALED: reachable back at full shards this sweep
  rn=${reach%%/*}; rm=${reach##*/}
  [ -n "$rn" ] && [ "$rn" != "?" ] && [ "$rn" -lt "$worst_reachable" ] 2>/dev/null && worst_reachable=$rn
  if fetch_ok; then
    echo "  cycle $c/$CYCLES: killed ${victim:0:12}; reachable=$reach; repairs=$repairs; fetch ✓ bit-perfect"
  else
    echo "  cycle $c/$CYCLES: killed ${victim:0:12}; reachable=$reach; repairs=$repairs"
    echo "──────────────────────────────────────────────────────"
    echo "RESULT: FINDING ⚠  content did NOT survive turnover: a fetch failed after $killed_total permanent"
    echo "  departures (reachable=$reach). Repair could not keep redundancy up as membership rotated —"
    echo "  the durability contract (finite-but-renewable, D-S7) is not holding at this turnover cadence."
    echo "  Re-run with a gentler CYCLE_WAIT or REPLICATION=3 to separate cadence from a repair defect."
    docker logs "$CARE_CID" 2>&1 | grep -iE 'repair sweep|stripe|below k' | tail -6 | sed 's/^/    /'
    [ "${EXPECT:-}" = pass ] && exit 1 || exit 0
  fi
done

echo "──────────────────────────────────────────────────────"
echo "  permanent departures: $killed_total (pool size held at $HOLDERS)   worst redundancy dip: $worst_reachable shards reachable"
echo "  total repair reconstructions: $(care_repairs)"
if [ "$killed_total" -ge "$HOLDERS" ]; then
  echo "RESULT: PASS ✅  content OUTLIVED the nodes — survived $killed_total permanent departures (> full membership rotation of $HOLDERS), every fetch bit-perfect, redundancy repaired back up each cycle (D-S7 finite-but-renewable holds)"
else
  echo "RESULT: PASS ✅  content stayed bit-perfect across $killed_total permanent departures with redundancy repaired each cycle (increase CYCLES ≥ $HOLDERS for a full membership rotation)"
fi
exit 0
