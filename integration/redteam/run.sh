#!/usr/bin/env bash
# Byzantine red-team field test (#184 validator accountability), fully
# automated on one host in real containers over real TCP. It proves an honest
# silt validator holds three concrete, wire-reachable accountability properties
# against the repo's own RED-TEAM harness flags:
#
#   1. EQUIVOCATION  — a double-signing validator is CAUGHT and SLASHED.
#   2. FORGED BLOCK  — a block with a forged proposer sig is REJECTED pre-attest.
#   3. LOW-BOND      — an under-bonded proposer's block is REFUSED.
#
# Plus a POSITIVE CONTROL: two honest validators still commit a normal block,
# so a rejection is a real defence, not a dead/quorum-broken swarm.
#
# Every assertion keys off a REAL log line the daemon prints (confirmed against
# cmd/silt/daemon.go and the in-process analogs e2e/equivocation_test.go +
# e2e/proposal_reject_test.go) — no invented strings.
#
# Usage:  ./run.sh          # build, run all scenarios, tear down; exit 0 = PASS
#         KEEP=1 ./run.sh   # leave the topology up afterward to poke at
set -uo pipefail
cd "$(dirname "$0")"
ROOT=$(cd ../.. && pwd)

dc() { docker compose "$@"; }
cleanup() { [ "${KEEP:-0}" = 1 ] || { dc --profile equiv --profile propose down -v >/dev/null 2>&1 || true; rm -f "$(dirname "$0")/silt"; }; }
trap cleanup EXIT

# Wait until a container's stdout (docker compose logs) contains a pattern.
# The adversary/slash/reject lines are printed to STDOUT by the daemon
# (fmt.Printf in cmd/silt/daemon.go), so we grep the compose logs, not debug.log.
wait_log() { # svc pattern timeout_s
  local svc=$1 pat=$2 t=${3:-60} i
  for ((i=0; i<t; i++)); do
    dc logs "$svc" 2>&1 | grep -qE "$pat" && return 0
    sleep 1
  done
  return 1
}

PASS=1
fail() { echo "  FAIL: $*"; PASS=0; }

echo "== build the silt binary on the host (linux/$(go env GOARCH)) and the image =="
( cd "$ROOT" && CGO_ENABLED=0 GOOS=linux GOARCH="$(go env GOARCH)" go build -trimpath -o integration/redteam/silt ./cmd/silt ) \
  || { echo "FAIL: host build"; exit 1; }
docker build -q -t silt-redteam . >/dev/null || { echo "FAIL: image build"; exit 1; }

# Learn every deterministic NodeID up front by running `silt id` INSIDE a
# container (the binary is linux/$GOARCH, so it can't run on a macOS host).
sid() { docker run --rm silt-redteam silt id -id-seed "$1" | tr -d '\r\n'; }
echo "== learn deterministic NodeIDs (silt id, in-container) =="
export ID_H1=$(sid 1)
export ID_H2=$(sid 2)
export ID_H3=$(sid 8001)
export ID_EQUIV_A=$(sid 6001)
export ID_EQUIV_X=$(sid 6002)
export ID_EQUIV_YZ=$(sid 6003)
for v in ID_H1 ID_H2 ID_H3 ID_EQUIV_A ID_EQUIV_X ID_EQUIV_YZ; do
  [ -n "${!v}" ] || { echo "FAIL: could not derive $v"; exit 1; }
  printf "  %-11s %s\n" "$v" "${!v}"
done

# ─────────────────────────────────────────────────────────────────────────────
echo ""
echo "########## POSITIVE CONTROL — two honest validators commit a block ##########"
# H1+H2 earn mutual standing and commit a normal publish through consensus.
dc up -d h1 h2
wait_log h1 'registry:' 40 || { fail "h1 never served the registry"; }
wait_log h1 'peer: [0-9a-f]{64}' 20; wait_log h2 'peer: [0-9a-f]{64}' 20
echo "  letting bond audits accrue standing (14s)…"
sleep 14
# Publish a small file from inside h1 (it holds the registry) and expect a commit.
REG="$ID_H1@https://10.110.0.11:4003"
PEERS="$ID_H1@10.110.0.11:4001"
dc exec -T h1 sh -c "head -c 65536 /dev/urandom > /tmp/f.bin; silt swarm add /tmp/f.bin -peers '$PEERS' -registry '$REG'" >/tmp/rt_add.txt 2>&1
LINK=$(grep -oE 'silt:v1:[A-Za-z0-9_:-]+' /tmp/rt_add.txt | head -1)
echo "  publish link: ${LINK:-<none>}"
sleep 4
if wait_log h1 'chain: committed block [1-9]' 20 && wait_log h2 'chain: committed block [1-9]' 20; then
  echo "  POSITIVE CONTROL: PASS — H1 & H2 both committed a real block through consensus"
  dc logs h1 2>&1 | grep -oE 'chain: committed block [0-9]+ \([^)]*\)' | tail -1 | sed 's/^/    h1: /'
  dc logs h2 2>&1 | grep -oE 'chain: committed block [0-9]+ \([^)]*\)' | tail -1 | sed 's/^/    h2: /'
else
  fail "honest quorum did NOT commit a normal block — quorum is not healthy, later rejections are meaningless"
  dc logs h1 2>&1 | tail -15 | sed 's/^/    h1: /'
fi

# ─────────────────────────────────────────────────────────────────────────────
echo ""
echo "########## SCENARIO 1 — EQUIVOCATION: double-signer caught & slashed ##########"
# equiv-a is the bootstrap anchor; equiv-x (detector) and equiv-yz bootstrap off
# it. A double-signs at height 1: block X → equiv-x, heavier Y,Z → equiv-yz. The
# detector equiv-x syncs the heavier fork from equiv-yz, reconciles, and slashes A.
echo "  adversary (equiv-a) id: $ID_EQUIV_A"
dc --profile equiv up -d equiv-a equiv-x equiv-yz
# The adversary retries until it has earned standing with BOTH peers, then reports.
if wait_log equiv-a 'adversary: equivocation complete \(double-signed height 1\)' 150; then
  echo "  adversary double-signed (real: 'adversary: equivocation complete')"
else
  fail "equivocator never completed the double-sign (could not earn standing with both peers)"
  dc logs equiv-a 2>&1 | tail -15 | sed 's/^/    equiv-a: /'
fi
# The honest detector catches it and prints the slash for the adversary's ID.
if wait_log equiv-x "chain: slashed equivocator $ID_EQUIV_A" 120; then
  echo "  SCENARIO 1: PASS — honest replica caught the double-sign and SLASHED $ID_EQUIV_A"
  dc logs equiv-x 2>&1 | grep -E "chain: slashed equivocator $ID_EQUIV_A" | tail -1 | sed 's/^/    equiv-x: /'
else
  fail "honest replica did NOT slash the equivocator — accountability property (D2) not observed"
  dc logs equiv-x 2>&1 | tail -20 | sed 's/^/    equiv-x: /'
fi
dc --profile equiv rm -sf equiv-a equiv-x equiv-yz >/dev/null 2>&1 || true

# ─────────────────────────────────────────────────────────────────────────────
echo ""
echo "########## SCENARIO 2 & 3 — FORGED BLOCK and LOW-BOND proposals rejected ##########"
# A fresh honest target H3; the forger and low-bond proposer each send ONE
# crafted proposal and report whether H3 refused. The daemon prints
# 'adversary: <label> proposal correctly REJECTED by <id>' on refusal (and
# 'UNEXPECTEDLY ACCEPTED ... (DEFECT)' if H3 wrongly attested).
dc --profile propose up -d h3
wait_log h3 'peer: [0-9a-f]{64}' 20 || fail "h3 never came up"
echo "  letting H3 accrue standing (12s)…"
sleep 12
# H3's own committed head height — the GROUND-TRUTH cross-check that no forged/low-bond
# block slipped in (replaces the old print-only chain-status). Sequenced deliberately:
# launch the POSITIVE CONTROL (goodprop) FIRST and snapshot H3 once its one legit block
# has settled; only THEN launch the two adversaries. Any height growth across that
# pure-adversarial window is provably a bad block that committed → a real DEFECT.
# h3's committed head height as a number. An empty chain (goodpropose is ATTESTED but a
# single-attester block need not reach commit quorum) prints "no chain yet (0 blocks)" →
# normalized to 0. A genuine read failure (container gone) returns empty so the caller can
# tell "chain-status says 0 committed" apart from "couldn't read h3 at all".
h3_height() {
  local out; out=$(dc exec -T h3 sh -c 'silt chain-status -store /data' 2>/dev/null)
  if printf '%s' "$out" | grep -qE 'head height: +[0-9]+'; then
    printf '%s' "$out" | grep -oE 'head height: +[0-9]+' | grep -oE '[0-9]+' | head -1
  elif printf '%s' "$out" | grep -q 'no chain yet'; then
    printf '0'
  fi
}
# Read h3's head height only once it has STOPPED moving (two equal reads 3s apart, cap
# ~18s) so a legit commit that lags its attestation is fully accounted before we snapshot
# — otherwise the positive-control block could land AFTER the baseline and be misread as
# an adversarial commit. Returns the settled height.
h3_stable_height() { local a b i; a=$(h3_height); for i in $(seq 1 6); do sleep 3; b=$(h3_height); [ -n "$a" ] && [ "$a" = "$b" ] && { printf '%s' "$a"; return; }; a="$b"; done; printf '%s' "$a"; }
H3_PRE=$(h3_height); echo "  H3 pre-proposal committed head height: ${H3_PRE:-<none>}"
dc --profile propose up -d goodprop

# POSITIVE CONTROL (audit #303): before crediting H3's REJECTIONS below, prove H3
# ACCEPTS a well-formed, properly-bonded proposal — otherwise a target that refuses
# EVERY proposal (chain role wedged, head mismatch, …) would make both reject tests
# false-pass ('reject the good one too' looks identical to 'reject the bad one').
# goodprop is 8M-bonded like forger; it retries until its bond earns standing.
echo "  -- POSITIVE CONTROL: H3 must ACCEPT a well-formed, bonded proposal --"
if wait_log goodprop "goodpropose proposal ACCEPTED by $ID_H3" 90; then
  echo "  POSITIVE CONTROL: PASS — H3 attested a valid bonded proposal, so it is a LIVE attester"
  dc logs goodprop 2>&1 | grep -E "goodpropose proposal ACCEPTED by" | tail -1 | sed 's/^/    goodprop: /'
elif dc logs goodprop 2>&1 | grep -qE 'goodpropose proposal UNEXPECTEDLY REJECTED'; then
  fail "H3 REFUSED even a well-formed, properly-bonded proposal — it rejects EVERYTHING, so the SCENARIO 2 & 3 rejections below prove nothing (a broken H3, not a working defence)"
else
  fail "positive control did not resolve: H3 never accepted a valid bonded proposal within the window (cannot attribute the reject scenarios to the real defence)"
fi

# Baseline AFTER the one legit block has fully settled — everything above is legit; from
# here only the two adversaries act, so H3's head must not move again.
H3_BASE=$(h3_stable_height); echo "  H3 committed head height after the legit positive-control block (settled): ${H3_BASE:-<none>}"
# NOW launch the adversaries (forger + low-bond), so the window below is purely adversarial.
dc --profile propose up -d forger lowbond

echo "  -- SCENARIO 2: FORGED BLOCK (corrupted proposer signature) --"
if wait_log forger "forge-block proposal correctly REJECTED by $ID_H3" 60; then
  echo "  SCENARIO 2: PASS — H3 rejected the forged block before attesting"
  dc logs forger 2>&1 | grep -E "forge-block proposal correctly REJECTED by" | tail -1 | sed 's/^/    forger: /'
elif dc logs forger 2>&1 | grep -qE 'forge-block proposal UNEXPECTEDLY ACCEPTED'; then
  fail "H3 ACCEPTED a forged block (DEFECT) — forged-block→reject not enforced"
else
  fail "no forge-block reject observed within timeout"
  dc logs forger 2>&1 | tail -12 | sed 's/^/    forger: /'
fi

echo "  -- SCENARIO 3: LOW-BOND PROPOSE (under-bonded proposer) --"
if wait_log lowbond "lowbond-propose proposal correctly REJECTED by $ID_H3" 60; then
  echo "  SCENARIO 3: PASS — H3 refused the under-bonded proposer"
  dc logs lowbond 2>&1 | grep -E "lowbond-propose proposal correctly REJECTED by" | tail -1 | sed 's/^/    lowbond: /'
elif dc logs lowbond 2>&1 | grep -qE 'lowbond-propose proposal UNEXPECTEDLY ACCEPTED'; then
  fail "H3 ACCEPTED an under-bonded proposal (DEFECT) — low-bond→reject not enforced"
else
  fail "no lowbond-propose reject observed within timeout"
  dc logs lowbond 2>&1 | tail -12 | sed 's/^/    lowbond: /'
fi

# H3's chain must NOT have committed any bad block from either adversary. This is a
# GROUND-TRUTH cross-check on H3's own committed head height (not the adversary's
# self-report): the baseline H3_BASE was taken after the single legit positive-control
# block settled and BEFORE the adversaries launched, so an unchanged head proves the
# forged/low-bond proposals committed nothing. Retry-poll briefly to outlast any async
# commit of a bad block (a defect would still surface as a height increase).
echo "  -- H3 committed no adversarial block (head-height cross-check) --"
# Both adversary scenarios above already waited out their 60s windows, so any bad block
# that (wrongly) committed has had ample time to land; take a settled final read.
H3_FINAL=$(h3_stable_height)
dc exec -T h3 sh -c 'silt chain-status -store /data' 2>/dev/null | sed 's/^/    /' || echo "    (chain-status unavailable)"
echo "  H3 head height: baseline(after legit block)=${H3_BASE:-<none>}  final(after adversaries)=${H3_FINAL:-<none>}"
if [ -z "$H3_BASE" ] || [ -z "$H3_FINAL" ]; then
  fail "could not read H3's committed head height for the no-adversarial-block cross-check (chain-status unavailable)"
elif [ "$H3_FINAL" != "$H3_BASE" ]; then
  fail "H3's committed head height GREW ${H3_BASE}→${H3_FINAL} across the purely-adversarial window — a forged/low-bond block committed (DEFECT), the adversary's own REJECTED verdict notwithstanding"
else
  echo "  H3 CROSS-CHECK: PASS — head height unchanged (${H3_BASE}); neither adversary committed a block on the honest target"
fi

# ─────────────────────────────────────────────────────────────────────────────
echo ""
if [ "$PASS" = 1 ]; then
  echo "RESULT: PASS ✅  honest validator caught+slashed the equivocator, rejected the forged block, and refused the under-bonded proposer (positive control committed a normal block)"
else
  echo "RESULT: FAIL ❌  see failures above"
fi
exit $((1-PASS))
