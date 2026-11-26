#!/usr/bin/env bash
# Run a live silt network on this machine, with simulated people using it, and watch it in the
# web UI while it runs.
#
# Starts, all on loopback:
#   - 3 validators (the launch anchors), each keeping a full chain replica;
#   - storage nodes of different sizes (STORAGE, default 4);
#   - floor boxes (FLOOR_BOXES, default 3): validators that keep no chain and judge each new
#     block against witnesses, following the chain from a checkpoint by their own verdicts;
#   - simulated people (PEOPLE, default 4) who publish files and fetch each other's, at random
#     intervals, each through a different node.
#
# Every node runs a web UI. The script prints one observatory link that watches all of them:
# roles, storage, chunks, serving bandwidth, chain height, each floor box's progress and verdict,
# and every published file's spread across nodes. The people's own feed (who published what, who
# fetched what, and whether it came back bit-perfect) prints here and to $W/people.log. That feed
# comes from the simulated people, not from the nodes: silt nodes keep no record of who fetched
# what.
#
#   ./examples/live-network.sh              # run until Ctrl-C
#   ./examples/live-network.sh --check 180  # run 180 s, then check and stop (exit 0 = healthy)
#
# It uses `silt` from PATH when there is one, and otherwise builds it from this repository.
# Scratch state lives in $SILT_LIVE_DIR (default: a fresh temporary directory). Everything stays
# on this machine.

set -u
CHECK=0; CHECK_SECS=180
if [ "${1:-}" = "--check" ]; then CHECK=1; CHECK_SECS="${2:-180}"; fi
STORAGE="${STORAGE:-4}"; FLOOR_BOXES="${FLOOR_BOXES:-3}"; PEOPLE="${PEOPLE:-4}"

if command -v silt >/dev/null 2>&1; then
  SILT="$(command -v silt)"
else
  REPO="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
  [ -d "$REPO/cmd/silt" ] || { echo "no silt on PATH and not inside the silt repository"; exit 1; }
  echo "building silt…"
  (cd "$REPO" && go build -o "$REPO/silt" ./cmd/silt) || exit 1
  SILT="$REPO/silt"
fi

W="${SILT_LIVE_DIR:-$(mktemp -d "${TMPDIR:-/tmp}/silt-live.XXXXXX")}"
mkdir -p "$W/people" || { echo "could not create $W"; exit 1; }
: >"$W/links.txt"; : >"$W/people.log"
PIDS=()
cleanup() { for p in "${PIDS[@]:-}"; do kill "$p" 2>/dev/null; done; wait 2>/dev/null; }
trap cleanup EXIT INT TERM

await() { # await FILE REGEX SECONDS — wait for a line, print it
  local i
  for i in $(seq 1 $(( $3 * 5 ))); do
    if grep -qE "$2" "$1" 2>/dev/null; then grep -E "$2" "$1" | head -1; return 0; fi
    sleep 0.2
  done
  echo "timed out waiting for /$2/ in $1:" >&2; tail -20 "$1" >&2; return 1
}

IDA=$("$SILT" id -id-seed 9201); IDB=$("$SILT" id -id-seed 9202); IDC=$("$SILT" id -id-seed 9203)
ANCHORS="$IDA,$IDB,$IDC"
# One consensus configuration, shared by every validator and floor box: a box judges blocks by the
# same rules the replicas apply, so it must be configured identically.
CONSENSUS=(-anchors "$ANCHORS" -mature-validators 3 -quorum 2 -min-bond 1M -min-bond-floor 0)
NAMES=(); UIS=()

node() { # node NAME [flags…] — start a daemon with a web UI and remember its UI address
  local name=$1; shift
  "$SILT" daemon -listen 127.0.0.1:0 -store "$W/$name" -mdns=false -ui 127.0.0.1:0 "$@" >"$W/$name.out" 2>&1 &
  PIDS+=($!); NAMES+=("$name")
}
ui_of() { await "$W/$1.out" '^ui: http' 60 | awk '{print $2}'; }

echo "starting the network in $W …"
node val-a -id-seed 9201 -validator -bond 8M -capacity 1G -attesters "$IDB,$IDC" -serve-registry 127.0.0.1:0 "${CONSENSUS[@]}"
PEERA=$(await "$W/val-a.out" '^peer:' 30 | awk '{print $2}') || exit 1
node val-b -id-seed 9202 -validator -bond 8M -capacity 1G -attesters "$IDA,$IDC" -bootstrap "$PEERA" "${CONSENSUS[@]}"
node val-c -id-seed 9203 -validator -bond 8M -capacity 1G -attesters "$IDA,$IDB" -bootstrap "$PEERA" "${CONSENSUS[@]}"
SIZES=(256M 512M 1G 2G 4G 8G)
for i in $(seq 1 "$STORAGE"); do
  node "store-$i" -capacity "${SIZES[$(( (i - 1) % ${#SIZES[@]} ))]}" -bootstrap "$PEERA"
done
REG=$(await "$W/val-a.out" '^registry:' 30 | sed -E 's/.*serving ([^ ]+).*/\1/') || exit 1
ENTRY=("$PEERA")
for n in val-b val-c $(seq -f 'store-%g' 1 "$STORAGE"); do
  ENTRY+=("$(await "$W/$n.out" '^peer:' 30 | awk '{print $2}')") || exit 1
done

# One publish commits the validators' bonds into the chain, so there is a head to anchor on.
head -c 50000 /dev/urandom >"$W/warm.bin"
echo "waiting for the first block…"
for _ in $(seq 1 60); do
  "$SILT" swarm add "$W/warm.bin" -peers "$PEERA" -registry "$REG" >"$W/warm.out" 2>&1 && break
  sleep 2
done
grep -qE 'silt:v1:' "$W/warm.out" || { echo "the network never committed a first block"; tail -5 "$W/warm.out"; exit 1; }

# The floor boxes' checkpoint: validator A's head, as an operator would take it from a node they
# trust. Each box follows the chain from there by its own verdicts.
STATUS=$("$SILT" chain-status -store "$W/val-a")
CKH=$(printf '%s\n' "$STATUS" | sed -nE 's/.*head height[: ]+([0-9]+).*/\1/p' | head -1)
CKX=$(printf '%s\n' "$STATUS" | sed -nE 's/.*head hash[: ]+([0-9a-f]{64}).*/\1/p' | head -1)
[ -n "$CKH" ] && [ -n "$CKX" ] || { echo "could not read a checkpoint from val-a:"; printf '%s\n' "$STATUS"; exit 1; }
for i in $(seq 1 "$FLOOR_BOXES"); do
  node "floorbox-$i" -id-seed $(( 9300 + i )) -floor-box -ws-checkpoint "$CKH:$CKX" -floor-box-interval 2s \
    -witness-from "$ANCHORS" -attesters "$ANCHORS" -bootstrap "$PEERA" -capacity 512M "${CONSENSUS[@]}"
done

for n in "${NAMES[@]}"; do UIS+=("$(ui_of "$n")") || exit 1; done
OBS="${UIS[0]%/}/observatory.html?peers=$(IFS=,; echo "${UIS[*]:1}")"

# A simulated person: publishes files of varied sizes and fetches other people's, through their
# own entry node, and checks every fetch against the publisher's hash.
PEOPLE_NAMES=(alice bob carol dave erin frank grace heidi)
person() { # person NAME ENTRY SEED
  local me=$1 entry=$2 n=0
  RANDOM=$3 # each person its own stream: no two people publish in lockstep
  while :; do
    sleep $(( 3 + RANDOM % 8 ))
    n=$((n + 1))
    if [ $(( RANDOM % 10 )) -lt 5 ] || [ ! -s "$W/links.txt" ]; then
      local f="$W/people/$me-$n.bin" kb=$(( 32 + RANDOM % 1500 ))
      head -c $(( kb * 1024 )) /dev/urandom >"$f"
      local sum; sum=$(shasum -a 256 "$f" | awk '{print $1}')
      local t0=$SECONDS out link
      out=$("$SILT" swarm add "$f" -peers "$entry" -registry "$REG" 2>&1)
      link=$(printf '%s' "$out" | grep -oE 'silt:v1:[^ ]+' | head -1)
      if [ -n "$link" ]; then
        echo "$me $sum $kb $link" >>"$W/links.txt"
        log "$me published a ${kb} KB file ($(( SECONDS - t0 ))s)"
      else
        log "$me's publish did not commit: $(printf '%s' "$out" | tail -1)"
      fi
    else
      local row; row=$(grep -v "^$me " "$W/links.txt" | awk 'BEGIN{srand()} {a[NR]=$0} END{if (NR) print a[int(rand()*NR)+1]}')
      [ -n "$row" ] || continue
      local owner sum kb link; read -r owner sum kb link <<<"$row"
      local o="$W/people/$me-got-$n.bin" t0=$SECONDS
      if "$SILT" swarm get "$link" -o "$o" -peers "$entry" -registry "$REG" >/dev/null 2>&1 \
         && [ "$(shasum -a 256 "$o" | awk '{print $1}')" = "$sum" ]; then
        log "$me fetched $owner's ${kb} KB file, bit-perfect ($(( SECONDS - t0 ))s)"
      else
        log "$me FAILED to fetch $owner's ${kb} KB file"
      fi
      rm -f "$o"
    fi
  done
}
log() { printf '%s  %s\n' "$(date +%H:%M:%S)" "$*" | tee -a "$W/people.log"; }

for ((i = 0; i < PEOPLE; i++)); do
  person "${PEOPLE_NAMES[$(( i % ${#PEOPLE_NAMES[@]} ))]}" "${ENTRY[$(( i % ${#ENTRY[@]} ))]}" $(( 1000 + i * 7919 + $$ )) &
  PIDS+=($!)
done

cat <<MSG

silt is running on this machine: 3 validators, $STORAGE storage nodes, $FLOOR_BOXES floor boxes,
and $PEOPLE simulated people publishing and fetching.

  Watch it:   $OBS
  This node:  ${UIS[0]}

Floor boxes follow the chain from checkpoint $CKH by their own verdicts. Node logs are in $W.
The people's feed follows. Press Ctrl-C to stop the network.

MSG

if [ "$CHECK" = 0 ]; then wait; exit 0; fi

sleep "$CHECK_SECS"
echo; echo "=== check after ${CHECK_SECS}s ==="
pubs=$(grep -c ' published ' "$W/people.log"); gets=$(grep -c ' bit-perfect ' "$W/people.log")
fails=$(grep -c 'FAILED\|did not commit' "$W/people.log")
ok=1
echo "publishes committed: $pubs · fetches bit-perfect: $gets · failures: $fails"
[ "$pubs" -ge 3 ] || { echo "FAIL: fewer than 3 publishes committed"; ok=0; }
[ "$gets" -ge 3 ] || { echo "FAIL: fewer than 3 fetches came back bit-perfect"; ok=0; }
[ "$fails" -eq 0 ] || { echo "FAIL: $fails publish or fetch failures (see $W/people.log)"; ok=0; }
head_now=$("$SILT" chain-status -store "$W/val-a" | sed -nE 's/.*head height[: ]+([0-9]+).*/\1/p' | head -1)
for i in $(seq 1 "$FLOOR_BOXES"); do
  acc=$(grep -c 'floor-box: verdict=ACCEPTED' "$W/floorbox-$i.out")
  last=$(grep -oE 'pin advanced to [0-9]+' "$W/floorbox-$i.out" | tail -1 | awk '{print $4}')
  bad=$(grep -c 'floor-box: verdict=REFUSED' "$W/floorbox-$i.out")
  echo "floorbox-$i: $acc block(s) accepted, following h${last:-$CKH} (validator head $head_now), $bad refused"
  [ "$acc" -ge 3 ] || { echo "FAIL: floorbox-$i accepted fewer than 3 blocks"; ok=0; }
  [ "$bad" -eq 0 ] || { echo "FAIL: floorbox-$i refused an honest block"; ok=0; }
done
[ "$ok" = 1 ] && { echo "CHECK PASSED"; exit 0; }
echo "CHECK FAILED (logs in $W)"; exit 1
