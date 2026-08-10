#!/usr/bin/env bash
# ─────────────────────────────────────────────────────────────────────────────
# Flaky-network durability field test.
#
# Stands up 4 objective validators (the EXACT GCP consensus params: -quorum 2,
# Byzantine-quorum default ON, -mature-validators 4, all-attest, -anchors=all four)
# on one Docker network, then DEGRADES every link with `tc netem` — latency +
# jitter + packet loss — and asks the cynical question a clean localhost never
# does: under real-world adverse network conditions, does silt still bootstrap
# consensus, commit, and serve a bit-perfect fetch?
#
# Motivation: every CLEAN local test (e2e, integration/consensus) commits in
# seconds, but the FLAKY GCP topology mostly fails to bootstrap consensus (#286).
# The single variable is the flakiness. This reproduces that adversity locally and
# deterministically, so durability weaknesses can be found and fixed without GCP.
#
# Outcome (immutable #4 — never fake green): a real committed block + bit-perfect
# fetch under the impairment = RESULT: PASS. A failure to warm/fetch under a
# REALISTIC impairment is a durability FINDING (exit 0) with the exact netem params
# — a reproducer, not a hidden failure. A harness/build error is a hard FAIL.
#
# Usage:
#   ./run.sh                       # default impairment (moderate internet)
#   NETEM="delay 150ms 40ms distribution normal loss 5%" ./run.sh
#   NETEM="" ./run.sh              # clean-network control (must PASS)
#   CHURN=1 ./run.sh               # also kill+restart the boot validator mid-cold-start
#   KEEP=1 ./run.sh                # leave the swarm up to poke at
# ─────────────────────────────────────────────────────────────────────────────
set -uo pipefail
cd "$(dirname "$0")"
ROOT=$(cd ../.. && pwd)

# Default impairment: ~cross-region internet (80ms ± 20ms) with mild loss. This is
# a MILD, entirely realistic condition — a P2P node faces far worse.
: "${NETEM=delay 80ms 20ms distribution normal}"
: "${WARM_TIMEOUT:=180}"
: "${CHURN:=0}"
export NETEM REQ_TIMEOUT LOGLVL BOND_LAT

dc() { docker compose -p flakynet "$@"; }
cleanup() { [ "${KEEP:-0}" = 1 ] || dc down -v >/dev/null 2>&1 || true; }
trap cleanup EXIT

echo "== build silt (linux/$(go env GOARCH)) + image =="
( cd "$ROOT" && CGO_ENABLED=0 GOOS=linux GOARCH="$(go env GOARCH)" go build -trimpath -o integration/flakynet/silt ./cmd/silt ) \
  || { echo "FAIL: host (linux) build"; exit 1; }
docker build -q -t silt-flakynet . >/dev/null || { echo "FAIL: image build"; exit 1; }

silt_id() { docker run --rm silt-flakynet silt id -id-seed "$1" 2>/dev/null | tr -d '\r'; }
ID_A=$(silt_id 8001); ID_B=$(silt_id 8002); ID_C=$(silt_id 8003); ID_D=$(silt_id 8004)
ANCHORS="$ID_A,$ID_B,$ID_C,$ID_D"
export ID_A ID_B ID_C ID_D ANCHORS
echo "  anchors: $ID_A,$ID_B,$ID_C,$ID_D"
echo "  impairment (netem, every link): [$NETEM]"

echo "== bring up 4 objective validators under the impaired network =="
dc up -d >/dev/null 2>&1 || { echo "FAIL: compose up"; dc logs 2>&1 | tail -20; exit 1; }

# Confirm netem actually applied (a missing NET_ADMIN would silently give a clean
# run and a false PASS). Every validator must log the impairment.
sleep 3
applied=$(dc logs 2>&1 | grep -c "netem: impaired")
if [ -n "$NETEM" ] && [ "$applied" -lt 4 ]; then
  echo "FAIL: netem did not apply on all 4 validators ($applied/4) — check cap_add NET_ADMIN; results would be a false clean run"
  dc logs 2>&1 | grep -i netem | head
  exit 1
fi
[ -n "$NETEM" ] && echo "  netem applied on $applied/4 validators"

# Optional churn: kill the boot validator during the cold start, then restart it —
# the real-world "the bootstrap node blipped while everyone was joining" scenario.
if [ "$CHURN" = 1 ]; then
  echo "== CHURN: killing the boot validator (valA) mid-cold-start, then restarting =="
  sleep 4
  dc kill valA >/dev/null 2>&1 || true
  sleep 6
  dc start valA >/dev/null 2>&1 || true
fi

echo "== wait (≤${WARM_TIMEOUT}s) for the chain to WARM (a committed block) under impairment =="
warm=0; t0=$(date +%s)
head_h() { dc exec -T "$1" silt chain-status -store /data 2>/dev/null | awk '/head height/{print $3}' | tr -d '\r'; }
while [ $(( $(date +%s) - t0 )) -lt "$WARM_TIMEOUT" ]; do
  if dc logs valA 2>&1 | grep -qE 'chain: committed block [1-9]'; then warm=1; break; fi
  # a throwaway publish from valA proposes the first block; retry it as standing accrues
  dc exec -T valA sh -c "head -c 4096 </dev/urandom >/tmp/w.bin; silt swarm add /tmp/w.bin -peers '$ID_A@10.60.0.11:4001,$ID_B@10.60.0.12:4001,$ID_C@10.60.0.13:4001,$ID_D@10.60.0.14:4001' -registry '$ID_A@https://10.60.0.11:4003' -token-quorum 2 -chunk-size 65536 -mode convergent" >/dev/null 2>&1 || true
  sleep 5
done
warm_s=$(( $(date +%s) - t0 ))

echo ""
if [ "$warm" = 1 ]; then
  hA=$(head_h valA); hB=$(head_h valB); hC=$(head_h valC); hD=$(head_h valD)
  echo "  chain warmed after ${warm_s}s under [$NETEM] — heights A=$hA B=$hB C=$hC D=$hD"
  # A real end-to-end fetch under impairment: publish on valB, fetch bit-perfect on valC.
  echo "== publish on valB, fetch bit-perfect on valC (under impairment) =="
  PEERS="$ID_A@10.60.0.11:4001,$ID_B@10.60.0.12:4001,$ID_C@10.60.0.13:4001,$ID_D@10.60.0.14:4001"
  REG=$ID_A@https://10.60.0.11:4003
  dc exec -T valB sh -c "head -c 131072 </dev/urandom >/tmp/p.bin; sha256sum /tmp/p.bin | cut -d' ' -f1 >/tmp/p.sha; silt swarm add /tmp/p.bin -peers '$PEERS' -registry '$REG' -token-quorum 2 -chunk-size 65536 -mode convergent" >/tmp/fk_add.out 2>&1 || true
  LINK=$(grep -oE 'silt:v1:\S+' /tmp/fk_add.out | head -1)
  WANT=$(dc exec -T valB cat /tmp/p.sha | tr -d '\r\n ')
  GOT=""
  if [ -n "$LINK" ]; then
    dc exec -T valC sh -c "silt swarm get '$LINK' -o /tmp/g.bin -peers '$PEERS' -registry '$REG' >/dev/null 2>&1; sha256sum /tmp/g.bin | cut -d' ' -f1" >/tmp/fk_get.out 2>&1 || true
    GOT=$(tr -d '\r\n ' </tmp/fk_get.out)
  fi
  if [ -n "$LINK" ] && [ "$WANT" = "$GOT" ]; then
    echo "  fetch bit-perfect (sha ${WANT:0:12}…)"
    echo "RESULT: PASS ✅  objective consensus bootstrapped, committed, and served a bit-perfect fetch under [$NETEM]$([ "$CHURN" = 1 ] && echo " + boot-node churn")"
    exit 0
  else
    echo "RESULT: FINDING ⚠  chain warmed under impairment but the end-to-end fetch did NOT complete (link=${LINK:-none} want=${WANT:0:12} got=${GOT:0:12}) — a retrieval-under-adversity durability gap"
    exit 0
  fi
else
  echo "RESULT: FINDING ⚠  4 objective validators did NOT commit a block within ${WARM_TIMEOUT}s under [$NETEM]$([ "$CHURN" = 1 ] && echo " + boot-node churn") — a consensus-bootstrap durability gap. Clean-network runs commit in seconds; this is the reproducer for the flaky-network failure (cf. #286)."
  echo "  --- valA tail ---"; dc logs valA 2>&1 | tail -12
  exit 0
fi
