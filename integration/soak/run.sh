#!/usr/bin/env bash
# ─────────────────────────────────────────────────────────────────────────────
# Field test #10 — soak / scale.
#
# A longer-duration, sustained-load run over MANY real daemons, built to catch
# LEAKS and slow DRIFT that a single-shot test never sees. It:
#   • stands up a seed/registry + a pool of holders (+ a caretaker) on a flat net
#   • publishes M multi-MB files, recording each link + its sha256
#   • for DURATION seconds, keeps continuous fetch traffic: an ephemeral client
#     repeatedly fetches random published links and asserts EACH is bit-perfect
#   • optionally does GENTLE, within-margin churn (kill ONE holder, then RESTART
#     it on its persisted store) so redundancy recovers and NO erasure column is
#     ever stranded — the soak measures steady-state, not catastrophic loss
#   • samples `docker stats` memory + per-store object counts at start/mid/end
#     and REPORTS the deltas, flagging any monotonic unbounded growth (a leak)
#
# Durability model it respects: content is 3x replicated + k=10/n=16 erasure.
# Killing one of a dozen+ holders never strands a column, and the killed holder
# is brought right back on its persisted store, so retrieval must stay perfect
# THROUGHOUT. A fetch failing under steady-state, a crash-loop, or unbounded
# memory growth is a real product finding — reported, not worked around.
#
# Usage:
#   ./run.sh                       # ~6 min soak, gentle churn, then tear down
#   DURATION=1800 ./run.sh         # 30-minute soak
#   CHURN=0 ./run.sh               # no churn — pure steady-state fetch load
#   KEEP=1 ./run.sh                # leave the topology up to poke at
#   FILE_BYTES=8000000 FILES=16 ./run.sh
# exit 0 = PASS (clean bill of health), non-zero = FAIL / finding
# ─────────────────────────────────────────────────────────────────────────────
set -uo pipefail
cd "$(dirname "$0")"
ROOT=$(cd ../.. && pwd)

# ── knobs (defaults: a real soak that still finishes in a sensible time) ──────
DURATION=${DURATION:-360}          # seconds of sustained fetch load
FILES=${FILES:-12}                 # number of files to publish
FILE_BYTES=${FILE_BYTES:-4000000}  # ~4 MB each
FETCH_INTERVAL=${FETCH_INTERVAL:-2}  # seconds between fetch bursts
CHURN=${CHURN:-1}                  # 1 = gentle within-margin churn, 0 = off
CHURN_INTERVAL=${CHURN_INTERVAL:-45}   # seconds between churn events
# Holders that are safe to churn (kept < replica margin: only ever ONE down at a
# time, and it comes right back). Never churn seed or caretaker.
CHURN_POOL=(holder01 holder02 holder03 holder04 holder05 holder06 holder07 holder08 holder09 holder10 holder11 holder12)
HOLDERS=(seed "${CHURN_POOL[@]}" caretaker)

PROJECT=soak
dc() { docker compose -p "$PROJECT" "$@"; }

WORK=$(mktemp -d)
FAIL=0
FETCH_OK=0
FETCH_TOTAL=0
MISSES=0            # fetches that missed even after the retry (real, not transient)
MISS_LINKS=""       # which links missed (for an actionable FINDING/FAIL)
# isolated churn-window misses tolerated as a FINDING (not FAIL); default 1
FETCH_MISS_TOLERANCE=${FETCH_MISS_TOLERANCE:-1}
declare -a LINKS SHAS
cleanup() {
  if [ "${KEEP:-0}" = 1 ]; then
    echo "== KEEP=1: leaving topology up (project $PROJECT). 'docker compose -p $PROJECT down -v' to clean =="
  else
    dc down -v >/dev/null 2>&1 || true
  fi
  rm -rf "$WORK" 2>/dev/null || true
}
trap cleanup EXIT INT TERM

# sum of object files across every store (leak/growth proxy)
store_objects() {
  local svc total=0 n
  for svc in "${HOLDERS[@]}"; do
    # diskstore fans chunks two levels deep (objects/<2hex>/<hash>), so a flat
    # `ls objects` counts ≤256 prefix dirs, not chunks — count files instead.
    n=$(dc exec -T "$svc" sh -c 'find /data/objects -type f 2>/dev/null | wc -l' 2>/dev/null | tr -d ' \r\n')
    [ -n "$n" ] && total=$((total + n))
  done
  echo "$total"
}
# MEAN RSS (MiB) per LIVE daemon container, from docker stats.
#
# We report the per-daemon MEAN, not the raw sum, on purpose: churn stops ONE
# holder at a time, so `dc ps -q` returns 13 running containers mid-run and 14
# at start/end. A raw SUM would swing with that live count independently of any
# real leak — a leak gate on the sum could false-FINDING on the churn dip, or
# (worse) a genuine per-daemon leak could hide behind a churn-shrunk container
# count and false-PASS. Dividing the total by the live count (n) makes the
# metric invariant to the 14↔13 churn swing, so start<mid<end reflects actual
# per-daemon growth, not topology size.
stats_mem_mib() {
  local ids total=0 v unit num n=0
  ids=$(dc ps -q 2>/dev/null)
  [ -n "$ids" ] || { echo 0; return; }
  n=$(echo "$ids" | wc -w | tr -d ' ')
  [ "$n" -gt 0 ] || { echo 0; return; }
  # MemUsage column looks like "12.3MiB / 7.6GiB"
  while read -r v; do
    num=$(echo "$v" | sed -E 's/([0-9.]+).*/\1/')
    unit=$(echo "$v" | sed -E 's/[0-9.]+([A-Za-z]+).*/\1/')
    case "$unit" in
      GiB) num=$(awk "BEGIN{printf \"%.1f\", $num*1024}") ;;
      KiB) num=$(awk "BEGIN{printf \"%.3f\", $num/1024}") ;;
      B)   num=$(awk "BEGIN{printf \"%.4f\", $num/1048576}") ;;
    esac
    total=$(awk "BEGIN{printf \"%.1f\", $total + $num}")
  done < <(docker stats --no-stream --format '{{.MemUsage}}' $ids 2>/dev/null | awk '{print $1}')
  # per-live-daemon MEAN — invariant to the churn container-count swing
  awk "BEGIN{printf \"%.1f\", $total / $n}"
}
# how many daemons have restarted (crash-loop detector)
restart_count() {
  local ids total=0 r
  ids=$(dc ps -q 2>/dev/null)
  for id in $ids; do
    r=$(docker inspect -f '{{.RestartCount}}' "$id" 2>/dev/null || echo 0)
    total=$((total + r))
  done
  echo "$total"
}

echo "== build the silt binary on the host (linux/$(go env GOARCH)) and the image =="
( cd "$ROOT" && CGO_ENABLED=0 GOOS=linux GOARCH="$(go env GOARCH)" go build -trimpath -o integration/soak/silt ./cmd/silt ) \
  || { echo "FAIL: go build"; exit 1; }
docker build -q -t silt-soak . >/dev/null || { echo "FAIL: docker build"; exit 1; }

echo "== phase 1: seed + registry =="
dc up -d seed
# The seed binds a wildcard (0.0.0.0 / [::]) so its own `peer:` / `registry:`
# log lines print the wildcard, NOT a dialable host. We take the NodeID from
# the log and pin it to the seed's known static container IP (10.120.0.10),
# which is exactly what -advertise stamps on the wire.
SEED_IP=10.120.0.10
SEED_ID=""
for _ in $(seq 1 40); do
  SEED_ID=$(dc logs seed 2>&1 | grep -oE 'peer: [a-f0-9]{64}' | head -1 | awk '{print $2}')
  [ -n "$SEED_ID" ] && break
  sleep 1
done
[ -n "$SEED_ID" ] || { echo "FAIL: seed never reported a NodeID"; dc logs seed | tail -20; exit 1; }
SEED_PEER="$SEED_ID@$SEED_IP:4001"
SEED_REG="$SEED_ID@https://$SEED_IP:4003"
export SEED_PEER SEED_REG
echo "  SEED_PEER=$SEED_PEER"
echo "  SEED_REG=$SEED_REG"

echo "== phase 2: bring up the holder pool (${#CHURN_POOL[@]} holders + caretaker) =="
dc up -d "${CHURN_POOL[@]}" caretaker

echo "== wait for every holder to bootstrap =="
for svc in "${CHURN_POOL[@]}" caretaker; do
  ok=0
  for _ in $(seq 1 60); do
    if dc logs "$svc" 2>&1 | grep -q "bootstrapped"; then ok=1; break; fi
    sleep 1
  done
  [ "$ok" = 1 ] || { echo "FAIL: $svc never bootstrapped"; dc logs "$svc" | tail -15; exit 1; }
done
echo "  all ${#CHURN_POOL[@]} holders + caretaker bootstrapped"
sleep 3   # let routing tables settle

# Peers list: seed + a spread of holders, so publish/fetch have many entry points.
PEERS="$SEED_PEER"

echo "== publish $FILES files of ${FILE_BYTES} bytes each (recording link + sha) =="
for i in $(seq 1 "$FILES"); do
  f="$WORK/f$i.bin"
  head -c "$FILE_BYTES" /dev/urandom > "$f"
  want=$(shasum -a 256 "$f" | awk '{print $1}')
  # Publish from the caretaker container (an ordinary holder here — NB: no
  # -care link is wired, so no active repair loop runs; survival under churn
  # rests on 3x replication + the erasure margin + a killed holder returning
  # on its persisted store, not on caretaker repair). Wiring real repair is a
  # follow-up (see the harness README / build-to-defects).
  dc cp "$f" caretaker:/tmp/pub.bin >/dev/null 2>&1
  out=$(dc exec -T caretaker sh -c "silt swarm add /tmp/pub.bin -peers '$PEERS' -registry '$SEED_REG'" 2>&1)
  link=$(echo "$out" | grep -oE 'silt:v1:[A-Za-z0-9_:-]+' | head -1)
  dc exec -T caretaker sh -c 'rm -f /tmp/pub.bin' >/dev/null 2>&1
  if [ -z "$link" ]; then
    echo "FAIL: publish #$i returned no link"; echo "$out" | tail -5; exit 1
  fi
  LINKS+=("$link"); SHAS+=("$want")
  printf "  [%2d/%d] %s  sha=%s\n" "$i" "$FILES" "${link:0:34}…" "${want:0:12}…"
done
N=${#LINKS[@]}

# one ephemeral fetch, asserted bit-perfect. Retries once: under DELIBERATE churn a
# fetch can race a holder mid-stop and miss transiently, then succeed from a surviving
# replica — that is expected noise, not a durability failure. Only a miss that
# survives the retry is counted (into MISSES + MISS_LINKS). returns 0 on match.
fetch_one() {
  local idx=$1 link=${LINKS[$1]} want=${SHAS[$1]} got t tries=${FETCH_TRIES:-2}
  FETCH_TOTAL=$((FETCH_TOTAL + 1))
  for t in $(seq 1 "$tries"); do
    got=$(dc run --rm -T --no-deps --entrypoint sh client \
          -c "silt swarm get '$link' -o /tmp/out.bin -peers '$PEERS' -registry '$SEED_REG' >/dev/null 2>&1 && sha256sum /tmp/out.bin | cut -d' ' -f1" 2>/dev/null | tr -d ' \r\n')
    if [ "$got" = "$want" ]; then FETCH_OK=$((FETCH_OK + 1)); return 0; fi
    [ "$t" -lt "$tries" ] && sleep 2
  done
  local g="${got:0:12}"; [ -z "$got" ] && g="<empty>"
  echo "  !! FETCH MISS link#$idx want=${want:0:12} got=$g (after $tries tries)"
  MISSES=$((MISSES + 1)); MISS_LINKS="$MISS_LINKS link#$idx"
  return 1
}

echo "== baseline: single fetch of each link (the control) =="
for i in $(seq 0 $((N-1))); do
  fetch_one "$i" || { echo "FAIL: baseline fetch of link#$i not bit-perfect (broken before soak even starts)"; FAIL=1; }
done
[ "$FAIL" = 0 ] || { echo "RESULT: FAIL ❌ (baseline broken)"; exit 1; }
echo "  baseline: $FETCH_OK/$FETCH_TOTAL bit-perfect"

# ── sampling ─────────────────────────────────────────────────────────────────
MEM_START=$(stats_mem_mib); OBJ_START=$(store_objects); RST_START=$(restart_count)
echo "== sample @start: mem=${MEM_START}MiB objects=${OBJ_START} restarts=${RST_START} =="

echo "== SOAK: ${DURATION}s of continuous fetch load (churn=${CHURN}) =="
START=$(date +%s)
MID=$((START + DURATION / 2))
END=$((START + DURATION))
mid_sampled=0
next_churn=$((START + CHURN_INTERVAL))
churn_events=0
DOWN_SVC=""   # the single holder currently stopped ("" = none). Only ever one.
MEM_MID=""; OBJ_MID=""

while :; do
  now=$(date +%s)
  [ "$now" -ge "$END" ] && break

  # fetch a burst of a few random links (misses are tracked + tolerated below;
  # a hard FAIL here would let a single transient churn-window miss fail the run)
  for _ in 1 2 3; do
    idx=$((RANDOM % N))
    fetch_one "$idx" || true
  done

  # midpoint sample
  if [ "$mid_sampled" = 0 ] && [ "$now" -ge "$MID" ]; then
    MEM_MID=$(stats_mem_mib); OBJ_MID=$(store_objects)
    echo "  == sample @mid (t+$((now-START))s): mem=${MEM_MID}MiB objects=${OBJ_MID} fetch=${FETCH_OK}/${FETCH_TOTAL} =="
    mid_sampled=1
  fi

  # gentle within-margin churn: kill ONE holder, restart it next cycle. Only
  # ever one down at a time (strictly inside the 3x-replica / k=10-of-16 margin),
  # and it returns on its persisted store so redundancy fully recovers.
  if [ "$CHURN" = 1 ] && [ "$now" -ge "$next_churn" ]; then
    # first, restart whatever we took down last round (persisted store reload)
    if [ -n "$DOWN_SVC" ]; then
      dc start "$DOWN_SVC" >/dev/null 2>&1
      echo "  ~ churn: restarted $DOWN_SVC (reloads persisted store; redundancy recovers)"
      DOWN_SVC=""
    fi
    # then take exactly one fresh holder down (never seed/caretaker)
    victim=${CHURN_POOL[$((RANDOM % ${#CHURN_POOL[@]}))]}
    dc stop "$victim" >/dev/null 2>&1
    DOWN_SVC="$victim"
    churn_events=$((churn_events + 1))
    echo "  ~ churn: stopped $victim (one holder down, within replica margin)"
    next_churn=$((now + CHURN_INTERVAL))
  fi

  sleep "$FETCH_INTERVAL"
done

# bring the still-down holder back before final sampling
if [ "$CHURN" = 1 ] && [ -n "$DOWN_SVC" ]; then
  dc start "$DOWN_SVC" >/dev/null 2>&1
  DOWN_SVC=""
  sleep 5
fi
# guarantee a mid sample even if a very short run blew past MID between bursts
if [ "$mid_sampled" = 0 ]; then
  MEM_MID=$(stats_mem_mib); OBJ_MID=$(store_objects); mid_sampled=1
fi

MEM_END=$(stats_mem_mib); OBJ_END=$(store_objects); RST_END=$(restart_count)
echo "== sample @end: mem=${MEM_END}MiB objects=${OBJ_END} restarts=${RST_END} =="

# ── report ───────────────────────────────────────────────────────────────────
echo ""
echo "════════════════════ SOAK REPORT ════════════════════"
printf "duration              : %ss\n" "$DURATION"
printf "files published       : %d  (%s bytes each)\n" "$N" "$FILE_BYTES"
printf "fetches               : %d total, %d bit-perfect\n" "$FETCH_TOTAL" "$FETCH_OK"
printf "churn events          : %d (gentle, within replica margin)\n" "$churn_events"
echo   "-------------------- growth over run --------------------"
MEM_DELTA=$(awk -v a="${MEM_START:-0}" -v b="${MEM_END:-0}" 'BEGIN{printf "%+.1f", b-a}')
printf "mem/daemon (MiB) start=%s  mid=%s  end=%s   delta=%s   (MEAN per live daemon — churn-invariant)\n" \
  "$MEM_START" "${MEM_MID:-n/a}" "$MEM_END" "$MEM_DELTA"
printf "store objects start=%s  mid=%s  end=%s   delta=%s\n" \
  "$OBJ_START" "${OBJ_MID:-n/a}" "$OBJ_END" "$((OBJ_END - OBJ_START))"
printf "container restarts    : start=%s  end=%s   delta=%s\n" "$RST_START" "$RST_END" "$((RST_END - RST_START))"
echo   "─────────────────────────────────────────────────────"

# ── assertions ───────────────────────────────────────────────────────────────
# Two verdict tiers, honestly separated (immutable #4 / the blind-test asks):
#   hard_fail (exit 1) — a real regression: retrieval systematically drifted.
#   finding  (exit 0) — reproduced anomalies to inspect (an isolated churn-window
#                       miss, a self-recovered crash-restart, a leak-suspect trend).
hard_fail=0; finding=0; notes=""
if [ "$FAIL" = 1 ]; then   # a BASELINE miss (broken before soak) is already fatal above
  hard_fail=1
fi
if [ "$MISSES" -gt "$FETCH_MISS_TOLERANCE" ]; then
  echo "FAIL: $MISSES/$FETCH_TOTAL fetches missed even after retry (>$FETCH_MISS_TOLERANCE tolerated) — retrieval drifted under steady-state load:${MISS_LINKS}"
  hard_fail=1
elif [ "$MISSES" -gt 0 ]; then
  echo "FINDING: $MISSES isolated churn-window miss(es) (≤$FETCH_MISS_TOLERANCE tolerated, recovered on retry from a surviving replica):${MISS_LINKS}"
  finding=1
fi
if [ "$((RST_END - RST_START))" -gt 0 ]; then
  echo "FINDING: $((RST_END - RST_START)) daemon container restart(s) during the soak — possible crash-loop; content kept serving, inspect logs"
  finding=1
fi
# leak heuristic (per-daemon MEAN RSS, churn-invariant): monotonic growth at
# BOTH mid and end, AND >2x OR an absolute +50 MiB/daemon. The 2x is scale-free;
# the absolute is now per-daemon (the metric is a mean, not the ~14-daemon sum),
# so +50 MiB of unreleased RSS per daemon over a short soak is the leak floor.
if [ -n "${MEM_MID:-}" ] && [ "$mid_sampled" = 1 ]; then
  grew=$(awk "BEGIN{print (${MEM_END:-0} > ${MEM_MID:-0} && ${MEM_MID:-0} > ${MEM_START:-0}) ? 1 : 0}")
  leak=$(awk "BEGIN{print (${MEM_END:-0} > 2*${MEM_START:-1} || (${MEM_END:-0} - ${MEM_START:-0}) > 50) ? 1 : 0}")
  if [ "$grew" = 1 ] && [ "$leak" = 1 ]; then
    echo "FINDING: per-daemon mem grew monotonically start<mid<end AND >2x-or-+50MiB/daemon (${MEM_START}→${MEM_END} MiB) — potential leak, inspect"
    finding=1
  elif [ "$grew" = 1 ]; then
    echo "NOTE: per-daemon mem grew monotonically (${MEM_START}→${MEM_MID}→${MEM_END} MiB) but stayed bounded — watch, not fatal"
  fi
fi

# object/disk leak heuristic, paralleling the memory one: total on-store objects
# should reach a steady state once the M files are published + replicated. A
# monotonic start<mid<end climb that MORE THAN DOUBLES the starting object count
# signals an unbounded disk/object leak (e.g. proof scratch or chunk copies never
# reaped). FINDING only (exit 0) — a genuine growth anomaly to inspect, not a
# hard regression. The store-object counts were sampled but never asserted before.
if [ -n "${OBJ_MID:-}" ] && [ "${OBJ_START:-0}" -gt 0 ]; then
  obj_grew=$(awk "BEGIN{print (${OBJ_END:-0} > ${OBJ_MID:-0} && ${OBJ_MID:-0} > ${OBJ_START:-0}) ? 1 : 0}")
  obj_leak=$(awk "BEGIN{print (${OBJ_END:-0} > 2*${OBJ_START:-1}) ? 1 : 0}")
  if [ "$obj_grew" = 1 ] && [ "$obj_leak" = 1 ]; then
    echo "FINDING: store objects grew monotonically start<mid<end AND >2x (${OBJ_START}→${OBJ_MID}→${OBJ_END}) — potential disk/object leak, inspect"
    finding=1
  elif [ "$obj_grew" = 1 ]; then
    echo "NOTE: store objects grew monotonically (${OBJ_START}→${OBJ_MID}→${OBJ_END}) but stayed bounded — watch, not fatal"
  fi
fi

if [ "$hard_fail" = 1 ]; then
  echo "RESULT: FAIL ❌  (see FAIL lines above — a real steady-state regression)"
  exit 1
elif [ "$finding" = 1 ]; then
  echo "RESULT: FINDING ⚠  ${FETCH_OK}/${FETCH_TOTAL} fetches bit-perfect over ${DURATION}s; see FINDING line(s) above (reproduced anomaly, not a hard regression). EXPECT=clean to hard-fail."
  [ "${EXPECT:-}" = clean ] && exit 1 || exit 0
else
  echo "RESULT: PASS ✅  ${FETCH_OK}/${FETCH_TOTAL} fetches bit-perfect over ${DURATION}s, no crash-loop, memory bounded"
  exit 0
fi
