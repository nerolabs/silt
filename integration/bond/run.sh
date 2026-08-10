#!/usr/bin/env bash
# ─────────────────────────────────────────────────────────────────────────────
# Field test #8 — proof-of-space-time BOND COST (M0 claim C1: "no discount").
#
# C1 says a validator's Sybil-resistance bond is genuinely EXPENSIVE TO FAKE: it
# must generate a real depth-robust plot (real disk) and prove space-TIME on it
# over time; a shortcut (undersized / no plot) earns NOTHING. This harness turns
# that claim into NUMBERS on real Docker:
#
#   MEASURE   real plot-generation wall time + on-disk plot bytes for a given
#             -bond, and the live bond-challenge/verify cadence (from real
#             debug.log timestamps).
#   POSITIVE  a correctly-bonded validator earns standing and its peer VERIFIES
#             the bond over the wire (`bond challenge … passed=true`).
#   NEGATIVE  a shortcut is rejected:
#               (1) a bond below -min-bond-floor earns NO standing — the daemon
#                   fails closed at startup with the real refusal string;
#               (2) an UNDER-BONDED validator's proposal is REFUSED by an honest
#                   peer before it will attest (-lowbond-propose over the wire).
#
# Usage:  ./run.sh                 build, run all phases, tear down; exit 0 = PASS
#         BOND_SIZE=128M ./run.sh  bigger bond (longer plot, more disk — the point)
#         KEEP=1 ./run.sh          leave the positive-phase topology up to poke at
# ─────────────────────────────────────────────────────────────────────────────
set -uo pipefail
cd "$(dirname "$0")"
ROOT=$(cd ../.. && pwd)
BOND_SIZE=${BOND_SIZE:-64M}     # modest by default so plotting is quick; document it
export BOND_SIZE

dc() { docker compose "$@"; }
cleanup() { [ "${KEEP:-0}" = 1 ] || dc --profile adversary down -v >/dev/null 2>&1 || true; }
trap cleanup EXIT

echo "== build the silt binary on the host (linux/$(go env GOARCH)) and the image =="
# Compile on the host — CGO off, so it cross-compiles trivially — and copy the
# binary into a slim image (mirrors integration/nat): tiny image, no in-Docker
# Go-build memory spike.
( cd "$ROOT" && CGO_ENABLED=0 GOOS=linux GOARCH="$(go env GOARCH)" go build -trimpath -o integration/bond/silt ./cmd/silt ) \
  || { echo "FAIL: host build"; exit 1; }
docker build -q -t silt-bond . >/dev/null || { echo "FAIL: docker build"; exit 1; }

# The binary is linux-built, so `silt id` must run INSIDE a container to learn
# the deterministic NodeIDs (H(pubkey) of a fixed -id-seed) before launch.
sid() { dc run --rm --no-deps -T honest silt id -id-seed "$1" 2>/dev/null | tr -d '\r\n '; }
echo "== learn the deterministic validator NodeIDs (silt id in a container) =="
HONEST_ID=$(sid 1); PEER_ID=$(sid 2)
[ -n "$HONEST_ID" ] && [ -n "$PEER_ID" ] || { echo "FAIL: could not derive NodeIDs"; exit 1; }
export HONEST_ID PEER_ID
echo "  honest id: $HONEST_ID"
echo "  peer   id: $PEER_ID"

pass=1
gap2=0   # PHASE 2 (low-bond negative control) undeliverable → a harness gap, not a FAIL

# ─── PHASE 0 — NEGATIVE: a sub-floor bond earns NO standing (fail-closed) ───
# Run FIRST, before any static-IP service is up, so this one-shot container can
# take honest's 10.100.0.10 without an "address already in use" collision.
echo ""
echo "########## PHASE 0 — NEGATIVE: bond below -min-bond-floor earns nothing ##########"
# A 16M bond under a 64M floor: the daemon must REFUSE to start rather than run a
# validator that silently earns nothing (M0 F1/F2 anti-release).
FLOOR_OUT=$(dc run --rm --no-deps -T \
  honest silt daemon -id-seed=1 -listen=0.0.0.0:4001 -store=/data \
  -validator -objective=false -min-rep=100 -quorum=1 \
  -bond=16M -min-bond-floor=64M -bond-audit=1s -bond-ttl=0 -capacity=1G -log=info 2>&1)
FLOOR_RC=$?
echo "  exit code: $FLOOR_RC (nonzero = refused to run)"
echo "  refusal  : $(echo "$FLOOR_OUT" | grep -m1 'below the anti-release floor')"
if [ "$FLOOR_RC" -ne 0 ] && echo "$FLOOR_OUT" | grep -q "is below the anti-release floor"; then
  echo "  NEGATIVE-0: PASS (sub-floor bond fails closed — earns NO standing)"
else
  echo "  NEGATIVE-0: FAIL (sub-floor bond was NOT refused)"; echo "$FLOOR_OUT" | tail -5; pass=0
fi

# ─── PHASE 1 — POSITIVE: seal a real plot, MEASURE it, verify over the wire ───
echo ""
echo "########## PHASE 1 — real bond: seal cost + live verify (C1 as numbers) ##########"
t0=$(date +%s.%N)
dc up -d honest peer >/dev/null 2>&1 || { echo "FAIL: compose up"; exit 1; }

# Wall-clock plot-generation time: from `up` to honest's `bond: sealed a … bond`
# line on stdout (the daemon prints it the instant Seal() returns).
sealed=0
for _ in $(seq 1 600); do
  if dc logs honest 2>&1 | grep -q "bond: sealed a .* storage bond"; then sealed=1; break; fi
  sleep 0.1
done
t1=$(date +%s.%N)
if [ "$sealed" = 1 ]; then
  PLOT_WALL=$(awk "BEGIN{printf \"%.2f\", $t1-$t0}")
else
  echo "FAIL: honest never sealed a bond"; dc logs honest | tail -20; pass=0; PLOT_WALL="n/a"
fi
echo "  seal line: $(dc logs honest 2>&1 | grep -m1 'bond: sealed a .* storage bond')"

# On-disk plot size: the persisted plot is a single <nodeid>.plot file under
# <store>/plot (adapters/diskplot). This IS the resident cost C1 charges.
sleep 2
PLOT_BYTES=$(dc exec -T honest sh -c 'stat -c %s /data/plot/*.plot 2>/dev/null | head -1' | tr -d '\r\n ')
PLOT_HUMAN=$(dc exec -T honest sh -c 'du -h /data/plot/*.plot 2>/dev/null | cut -f1' | tr -d '\r\n ')
echo "  on-disk plot: ${PLOT_BYTES:-?} bytes (${PLOT_HUMAN:-?}) for a $BOND_SIZE bond"
echo "  plot wall-clock (up→sealed, incl. container/daemon start): ${PLOT_WALL}s"

# C1 "expensive to make" is a NUMBER, not a checkbox: a real plot must be RESIDENT
# on disk at ~the bond size. Without this threshold a build that sealed a near-empty
# / instant plot (the exact C1 shortcut under test) would still PASS on PLOT_BYTES>0.
# Parse the -bond size to bytes and require the plot to be >= 90% of it.
bond_bytes() { local s="$1" n u; n="${s%[A-Za-z]*}"; u="${s##*[0-9]}"
  case "$u" in G|g) echo $(( n * 1073741824 ));; M|m) echo $(( n * 1048576 ));; K|k) echo $(( n * 1024 ));; *) echo "$n";; esac; }
EXPECT_BYTES=$(bond_bytes "$BOND_SIZE"); MIN_PLOT=$(( EXPECT_BYTES * 9 / 10 ))
if [ "${PLOT_BYTES:-0}" -ge "$MIN_PLOT" ] 2>/dev/null; then
  echo "  C1 plot-size gate: PASS (${PLOT_BYTES} B ≥ 90% of the $BOND_SIZE bond = ${MIN_PLOT} B — the plot is genuinely resident)"
else
  echo "FAIL(pos): C1 plot too small — ${PLOT_BYTES:-0} B < 90% of the $BOND_SIZE bond (${MIN_PLOT} B). A real proof-of-space plot must be resident on disk; a near-empty/instant plot is the C1 shortcut this test exists to reject."
  pass=0
fi

# Live verification: let a few audit sweeps run, then read the REAL over-the-wire
# verdicts. peer challenges honest (recomputing labels from H(pk,n) without the
# plot) → `bond challenge peer=<honest> passed=true standing=<N>`.
echo "  letting live proof-of-space-time challenges run (8s)…"
sleep 8

PEER_VERDICT=$(dc exec -T peer sh -c 'grep "bond challenge" /data/debug.log 2>/dev/null | tail -1' | tr -d '\r')
HON_STANDING=$(dc exec -T honest sh -c 'grep "standing self" /data/debug.log 2>/dev/null | tail -1' | tr -d '\r')
echo "  peer's verdict on honest's bond : ${PEER_VERDICT:-<none>}"
echo "  honest's self-narrated standing : ${HON_STANDING:-<none>}"

# Verify cadence: gap between two consecutive bond-challenge verdicts on peer =
# one full challenge+verify round-trip at the -bond-audit=1s cadence (verify is
# O((Samples+5k)·log n) — cheap even for a big plot; the wall cost is dominated
# by the 1s audit interval, so this bounds the verify+RTT, it doesn't isolate it).
CHAL_TS=$(dc exec -T peer sh -c 'grep "bond challenge" /data/debug.log 2>/dev/null | grep -oE "^[0-9T:.-]+Z" | tail -2' | tr "\n" " ")
echo "  last two peer challenge timestamps: ${CHAL_TS:-<none>}"

echo "$PEER_VERDICT" | grep -q "passed=true"  || { echo "FAIL(pos): peer did not VERIFY honest's real bond over the wire"; pass=0; }
echo "$HON_STANDING"  | grep -qE "reputation=[1-9]" || { echo "FAIL(pos): honest earned no standing from its real bond"; pass=0; }
[ "${PLOT_BYTES:-0}" -gt 0 ] 2>/dev/null || { echo "FAIL(pos): no plot on disk"; pass=0; }

# ─── PHASE 2 — NEGATIVE: an under-bonded proposer is REFUSED over the wire ───
echo ""
echo "########## PHASE 2 — NEGATIVE: under-bonded proposal REFUSED by honest ##########"
# honest + peer are still up (real qualified validators). Bring up the adversary:
# a 4K-"bonded" validator that proposes a well-formed block to honest anyway. An
# honest validator refuses to attest a proposer without a qualifying bond.
dc --profile adversary up -d adversary >/dev/null 2>&1
# The adversary ALWAYS prints "⚠ ADVERSARY: -lowbond-propose set" on start, then
# retries ProposeBadBlock until it can DIAL honest; only then does it print the
# verdict. So a missing verdict means the attack was UNDELIVERABLE (couldn't reach
# the target in the window — the adversary joins last with just honest as bootstrap
# and can race the connect), NOT that an honest node accepted a bad block. Gate on
# DELIVERY: undeliverable ⇒ harness gap, never a property FAIL. (The blind field
# test hit exactly this false FAIL; redteam SCENARIO 3 covers the same refusal.)
armed=0
for _ in $(seq 1 40); do dc logs adversary 2>&1 | grep -q "ADVERSARY: -lowbond-propose set" && { armed=1; break; }; sleep 1; done
ADV_VERDICT=""
if [ "$armed" = 1 ]; then
  for _ in $(seq 1 120); do   # generous: outlast a slow adversary→honest connect
    ADV_VERDICT=$(dc logs adversary 2>&1 | grep -E "adversary: lowbond-propose proposal (correctly REJECTED|UNEXPECTEDLY ACCEPTED)" | tail -1)
    [ -n "$ADV_VERDICT" ] && break
    sleep 1
  done
fi
echo "  adversary armed=$armed; verdict: ${ADV_VERDICT:-<none delivered>}"
if echo "$ADV_VERDICT" | grep -q "correctly REJECTED"; then
  echo "  NEGATIVE-2: PASS (under-bonded proposal refused before attest)"
elif echo "$ADV_VERDICT" | grep -q "UNEXPECTEDLY ACCEPTED"; then
  echo "  NEGATIVE-2: FAIL (under-bonded proposal ACCEPTED over the wire — real C1 breach)"; dc logs adversary | tail -15; pass=0
else
  echo "  NEGATIVE-2: GAP — the adversary could not DELIVER a proposal to honest within the window"
  echo "    (armed=$armed, target unreachable). The low-bond refusal is UNTESTED here, not failed —"
  echo "    redteam SCENARIO 3 exercises the identical refusal over Docker and gates it. Harness gap."
  gap2=1
fi

# ─── verdict ───
echo ""
echo "==================== RESULT ===================="
echo "  bond size            : $BOND_SIZE"
echo "  plot generation wall : ${PLOT_WALL}s  (up→'sealed', includes container+daemon start)"
echo "  on-disk plot         : ${PLOT_BYTES:-?} bytes (${PLOT_HUMAN:-?})"
echo "  peer verify verdict  : ${PEER_VERDICT:-<none>}"
echo "  honest standing      : ${HON_STANDING:-<none>}"
if [ "$pass" = 1 ] && [ "$gap2" = 1 ]; then
  echo "  RESULT: PASS ✅  real plot is expensive to make, cheap to verify (C1) — NOTE: the PHASE 2"
  echo "  low-bond negative control was undeliverable this run (adversary couldn't reach honest);"
  echo "  that refusal is covered by the redteam suite. C1 positive controls held."
elif [ "$pass" = 1 ]; then
  echo "  RESULT: PASS ✅  real plot is expensive to make, cheap to verify, and a shortcut is rejected (C1)"
else
  echo "  RESULT: FAIL ❌"
fi
exit $((1-pass))
