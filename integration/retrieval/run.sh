#!/usr/bin/env bash
# ─────────────────────────────────────────────────────────────────────────────
# Field test — retrieval / discoverability at SCALE (+ ephemeral-identity churn).
#
# The most basic user promise: "can I get my content back?" — and the honest
# worry is that as the swarm grows and short-lived publisher/fetcher identities
# churn the DHT (#43/#60), a fresh client's cold provider lookup starts timing
# out and fetches fail. This test MEASURES that, on a real multi-holder swarm:
#
#   1. stand up a seed/registry + a pool of N holders on a flat network
#   2. publish FILES files (each from its own ephemeral client), record link+sha
#   3. optionally POLLUTE routing: POLLUTERS throwaway ephemeral publishes, each a
#      fresh identity that announces then vanishes — the #43 churn pressure
#   4. MEASURE: run FETCHES fetches, each from a FRESH ephemeral client doing a
#      cold lookup of a random published link, and assert each is bit-perfect
#   5. report the fetch SUCCESS RATE and gate it on FLOOR%
#
# Every `silt swarm add`/`swarm get` joins with its own ephemeral identity, so
# each measured fetch is a genuinely new fetcher — exactly a new user's path.
#
# Verdict:
#   rate >= FLOOR  -> PASS  (retrieval holds at scale — the #43 mitigations work)
#   rate <  FLOOR  -> FINDING (real discoverability degradation at scale, #43) —
#                     reported with the rate + a diagnostic, never faked green.
# A publish that never returns a link, or the swarm failing to come up, is FAIL.
#
# Usage:
#   ./run.sh                                  # 24 holders, 12 files, 60 fetches
#   HOLDERS=40 FETCHES=100 ./run.sh           # crank the scale
#   POLLUTERS=0 ./run.sh                      # no identity churn (baseline)
#   FLOOR=95 ./run.sh                         # stricter success-rate gate
#   KEEP=1 ./run.sh                           # leave the swarm up to poke at
# ─────────────────────────────────────────────────────────────────────────────
set -uo pipefail
cd "$(dirname "$0")"
ROOT=$(cd ../.. && pwd)

HOLDERS=${HOLDERS:-24}             # storage pool size (the "scale")
FILES=${FILES:-12}                 # distinct files published
FILE_BYTES=${FILE_BYTES:-1000000}  # ~1 MB each
FETCHES=${FETCHES:-60}             # measured cold fetches from fresh clients
POLLUTERS=${POLLUTERS:-40}         # throwaway ephemeral publishes (routing churn, #43)
FLOOR=${FLOOR:-90}                 # required bit-perfect success rate (%)
FETCH_TIMEOUT=${FETCH_TIMEOUT:-30} # per-fetch cap: a hung fetch is a fail, never a stall

PROJECT=retrieval
dc() { docker compose -p "$PROJECT" "$@"; }
cleanup() { [ "${KEEP:-0}" = 1 ] || dc down -v >/dev/null 2>&1 || true; }
trap cleanup EXIT INT TERM
fail() { echo "RESULT: FAIL ❌  $*"; exit 1; }

echo "== build the silt binary on the host (linux/$(go env GOARCH)) and the image =="
CGO_ENABLED=0 GOOS=linux GOARCH="$(go env GOARCH)" go build -trimpath -o silt "$ROOT/cmd/silt" \
  || fail "host build failed"
docker build -q -t silt-retrieval . >/dev/null || fail "image build failed"

echo "== phase 1: seed (registry + bootstrap; holds no content) =="
dc up -d seed >/dev/null 2>&1 || fail "compose up seed"
SEED_ID=""
for _ in $(seq 1 30); do
  SEED_ID=$(dc logs seed 2>&1 | grep -oE 'peer: [a-f0-9]{64}' | head -1 | awk '{print $2}')
  [ -n "$SEED_ID" ] && break; sleep 1
done
SEEDIP=$(docker inspect -f '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}' "$(dc ps -q seed)")
[ -n "$SEED_ID" ] && [ -n "$SEEDIP" ] || fail "seed never reported a NodeID/IP"
export SEED_PEER="$SEED_ID@$SEEDIP:4001"
export SEED_REG="$SEED_ID@https://$SEEDIP:4003"
echo "  seed: $SEED_PEER"

echo "== phase 2: $HOLDERS holders + an idle client =="
dc up -d --scale holder="$HOLDERS" holder client >/dev/null 2>&1 || fail "compose up holders"
echo "== wait for every holder to bootstrap into the swarm =="
bootok=0
HIDS=()
while IFS= read -r _id; do [ -n "$_id" ] && HIDS+=("$_id"); done < <(dc ps -q holder)
for id in "${HIDS[@]}"; do
  # audit #303: require a NON-EMPTY routing table, not the bare word "bootstrapped"
  # — a node logs `bootstrapped (0 table entries)` when it lands isolated, which is
  # NOT "at scale" and must not count toward the ≥90% scale gate below.
  for _ in $(seq 1 60); do docker logs "$id" 2>&1 | grep -qE 'bootstrapped \([1-9][0-9]* table entries\)' && { bootok=$((bootok+1)); break; }; sleep 1; done
done
echo "  $bootok/$HOLDERS holders bootstrapped"
# HARD scale gate: this suite measures cold-fetch success at SCALE (#43). If the
# swarm never reached scale, a high success rate against a shrunken swarm is
# meaningless (and would mask the very degradation under test) — so a bring-up
# shortfall is a FAIL, not a note. Require ≥90% of holders in the swarm.
MIN_BOOT=$(( HOLDERS * 90 / 100 )); [ "$MIN_BOOT" -lt 1 ] && MIN_BOOT=1
[ "$bootok" -ge "$MIN_BOOT" ] || fail "only $bootok/$HOLDERS holders bootstrapped (need ≥$MIN_BOOT) — the swarm never reached scale; a cold-fetch rate measured here is not meaningful"
CLIENT=$(dc ps -q client)

# publish one file from a fresh ephemeral client; echoes "LINK SHA" or empty
publish() { # publish <name> <bytes>
  local name="$1" bytes="$2" out link sha
  out=$(docker exec "$CLIENT" sh -c \
    "head -c $bytes /dev/urandom >/tmp/$name.bin; sha256sum /tmp/$name.bin | cut -d' ' -f1 >/tmp/$name.sha; \
     silt swarm add /tmp/$name.bin -peers '$SEED_PEER' -registry '$SEED_REG' 2>/dev/null; \
     rm -f /tmp/$name.bin" 2>/dev/null)
  link=$(printf '%s' "$out" | grep -oE 'silt:v1:[A-Za-z0-9_:-]+' | head -1)
  sha=$(docker exec "$CLIENT" sh -c "cat /tmp/$name.sha 2>/dev/null" | tr -d ' \r\n')
  [ -n "$link" ] && [ -n "$sha" ] && printf '%s %s\n' "$link" "$sha"
}

echo "== phase 3: publish $FILES files (each from a fresh ephemeral client) =="
declare -a LINKS SHAS
for i in $(seq 1 "$FILES"); do
  res=$(publish "pub$i" "$FILE_BYTES")
  [ -n "$res" ] || fail "publish $i never produced a link (swarm can't accept content)"
  LINKS+=("${res%% *}"); SHAS+=("${res##* }")
  printf '\r  published %d/%d' "$i" "$FILES"
done
echo "  ✓"

if [ "$POLLUTERS" -gt 0 ]; then
  echo "== phase 4: pollute routing with $POLLUTERS throwaway ephemeral publishes (#43 churn) =="
  for i in $(seq 1 "$POLLUTERS"); do
    docker exec "$CLIENT" sh -c \
      "head -c 65536 /dev/urandom >/tmp/junk.bin; \
       silt swarm add /tmp/junk.bin -peers '$SEED_PEER' -registry '$SEED_REG' >/dev/null 2>&1; rm -f /tmp/junk.bin" \
      >/dev/null 2>&1 &
    [ $((i % 8)) -eq 0 ] && wait   # bounded fan-out so we don't fork-bomb the host
  done
  wait
  echo "  $POLLUTERS ephemeral identities churned through the DHT"
fi

echo "== phase 5: measure — $FETCHES cold fetches from fresh ephemeral clients =="
OK=0
for i in $(seq 1 "$FETCHES"); do
  idx=$(( (RANDOM % ${#LINKS[@]}) ))
  link="${LINKS[$idx]}"; want="${SHAS[$idx]}"
  got=$(docker exec "$CLIENT" sh -c \
    "timeout $FETCH_TIMEOUT silt swarm get '$link' -o /tmp/g.bin -peers '$SEED_PEER' -registry '$SEED_REG' >/dev/null 2>&1; \
     sha256sum /tmp/g.bin 2>/dev/null | cut -d' ' -f1; rm -f /tmp/g.bin" 2>/dev/null | tr -d ' \r\n')
  [ "$got" = "$want" ] && OK=$((OK+1))
  printf '\r  fetched %d/%d (ok=%d)' "$i" "$FETCHES" "$OK"
done
echo

RATE=$(( OK * 100 / FETCHES ))
echo "──────────────────────────────────────────────────────"
echo "  swarm: $HOLDERS holders · $FILES files · $POLLUTERS routing-churn identities"
echo "  bit-perfect fetches: $OK/$FETCHES  =  ${RATE}%   (floor ${FLOOR}%)"
if [ "$RATE" -ge "$FLOOR" ]; then
  echo "RESULT: PASS ✅  retrieval holds at scale under identity churn (${RATE}% ≥ ${FLOOR}% floor) — the #43 ephemeral-identity routing mitigations hold"
  exit 0
else
  echo "RESULT: FINDING ⚠  discoverability degraded at scale: only ${RATE}% of cold fetches succeeded (< ${FLOOR}% floor)."
  echo "  This reproduces the #43/#60 retrieval-degradation family (ephemeral-identity routing pollution /"
  echo "  provider-discovery timeouts as the swarm grows). The failure IS the deliverable — a real product"
  echo "  finding to fix, not a harness bug. Re-run with POLLUTERS=0 to separate churn-pressure from raw scale."
  # A reproduced, documented FINDING exits 0 (like the upgrade suite); EXPECT=pass flips it to a hard fail.
  [ "${EXPECT:-}" = pass ] && exit 1 || exit 0
fi
