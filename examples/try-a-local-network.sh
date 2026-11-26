#!/usr/bin/env bash
# Run a small silt network on this machine, to try it.
#
# Starts three validators and one storage node on loopback, with the web UI on the first
# validator, and prints the UI's address. Everything stays on this machine: the nodes
# talk to each other over 127.0.0.1 and to nothing else. Ctrl-C stops them all.
#
#   ./examples/try-a-local-network.sh           # run until Ctrl-C
#   ./examples/try-a-local-network.sh --check   # publish and fetch one file, then stop
#
# It uses `silt` from PATH when there is one, and otherwise builds it from this
# repository. Scratch state lives in $SILT_TRY_DIR (default: a fresh temporary directory).
#
# The validators run the defended configuration — objective, bond-weighted consensus with
# three launch anchors — with small bonds (8 MiB each) so a laptop seals them in seconds.
# A real network uses gigabyte bonds; see the README.
set -uo pipefail

CHECK=0
[ "${1:-}" = "--check" ] && CHECK=1

if command -v silt >/dev/null 2>&1; then
  SILT="$(command -v silt)"
else
  REPO="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
  [ -d "$REPO/cmd/silt" ] || { echo "no silt on PATH and not inside the silt repository"; exit 1; }
  echo "building silt…"
  (cd "$REPO" && go build -o "$REPO/silt" ./cmd/silt) || exit 1
  SILT="$REPO/silt"
fi

W="${SILT_TRY_DIR:-$(mktemp -d "${TMPDIR:-/tmp}/silt-try.XXXXXX")}"
[ -n "$W" ] && mkdir -p "$W" || { echo "could not create a scratch directory"; exit 1; }
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

IDA=$("$SILT" id -id-seed 9101); IDB=$("$SILT" id -id-seed 9102); IDC=$("$SILT" id -id-seed 9103)
ANCHORS="$IDA,$IDB,$IDC"
validator() { # validator NAME SEED ATTESTERS [extra flags…]
  local name=$1 seed=$2 att=$3; shift 3
  "$SILT" daemon -id-seed "$seed" -listen 127.0.0.1:0 -store "$W/$name" -mdns=false \
    -validator -anchors "$ANCHORS" -attesters "$att" -mature-validators 3 -quorum 2 \
    -min-bond 1M -min-bond-floor 0 -bond 8M -capacity 1G "$@" >"$W/$name.out" 2>&1 &
  PIDS+=($!)
}

echo "starting the network in $W …"
validator a 9101 "$IDB,$IDC" -serve-registry 127.0.0.1:0 -ui 127.0.0.1:0
PEERA=$(await "$W/a.out" '^peer:' 30 | awk '{print $2}') || exit 1
validator b 9102 "$IDA,$IDC" -bootstrap "$PEERA"
validator c 9103 "$IDA,$IDB" -bootstrap "$PEERA"
"$SILT" daemon -listen 127.0.0.1:0 -store "$W/store" -mdns=false -bootstrap "$PEERA" -capacity 1G >"$W/store.out" 2>&1 &
PIDS+=($!)
REG=$(await "$W/a.out" '^registry:' 30 | sed -E 's/.*serving ([^ ]+).*/\1/') || exit 1
UI=$(await "$W/a.out" '^ui: http' 30 | awk '{print $2}') || exit 1
for n in b c store; do await "$W/$n.out" '^peer:' 30 >/dev/null || exit 1; done

if [ "$CHECK" = 1 ]; then
  head -c 300000 /dev/urandom >"$W/hello.bin"
  link=""
  for _ in $(seq 1 30); do
    out=$("$SILT" swarm add "$W/hello.bin" -peers "$PEERA" -registry "$REG" 2>&1)
    link=$(printf '%s' "$out" | grep -oE 'silt:v1:[^ ]+' | head -1)
    [ -n "$link" ] && break
    sleep 2
  done
  [ -n "$link" ] || { echo "CHECK FAILED: the network never committed a publish"; printf '%s\n' "$out" | tail -5; exit 1; }
  "$SILT" swarm get "$link" -o "$W/hello.out" -peers "$PEERA" -registry "$REG" >/dev/null 2>&1
  if cmp -s "$W/hello.bin" "$W/hello.out"; then echo "CHECK PASSED: published and fetched back bit-perfect"; exit 0; fi
  echo "CHECK FAILED: the fetched file differs from the published one"; exit 1
fi

cat <<MSG

silt is running on this machine: three validators and one storage node.

  Web UI:     $UI
  Publish:    $SILT swarm add FILE -peers $PEERA -registry $REG
  Fetch:      $SILT swarm get LINK -o OUT -peers $PEERA -registry $REG

Node logs are in $W. Press Ctrl-C to stop the network.
MSG
wait
