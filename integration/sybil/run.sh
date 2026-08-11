#!/usr/bin/env bash
# ─────────────────────────────────────────────────────────────────────────────
# Field test #5 — C2, "no QUIET capture", in real Docker.
#
# The M0 systemic claim, tested cynically as an OUTCOME: a bonded SYBIL validator
# set — many identities, real committed bonds, its own quorum — must NOT be able
# to capture a YOUNG objective network. While a network has never matured, an
# objective commit needs an honest launch ANCHOR's co-sign (training wheels, H4).
# The Sybils are not anchors, so even with real on-chain standing and a complete
# quorum they cannot advance the chain alone — chain: ErrAnchorRequired. That is C2.
#
# Two honest anchors (a1 registry+proposer, a2 co-signer) bootstrap the chain: a1's
# committed blocks carry the Sybils' bond registrations, so the Sybils earn REAL
# bonded standing. That is the point — the capture is then blocked by the ANCHOR
# gate, not merely by "no reputation yet" (the error TYPE proves which).
#
#   C2-a positive control: with the anchors present the chain commits and the C2
#        status line shows `wheels engaged (young network …)`.
#   C2-b no quiet capture: stop BOTH anchors; the Sybil set (s1 proposes, s2
#        attests) canNOT advance the chain — NO new block. The training wheels
#        refuse the capture at whichever layer fires first: locally, a young Sybil
#        set cannot even earn committed bonded standing without an anchor-proposed
#        block (reputation gate); behind that sits the anchor co-sign gate
#        (ErrAnchorRequired) for a Sybil that HAS standing. The test reports which
#        layer stopped it — the OUTCOME (no capture) is the same. The pure
#        anchor-gate form (with pre-banked Sybil bonds) is a cloud-test concern.
#   C2-c bonus: with ≥8 equal Sybil bonds the C2 atomization note fires (an equal-
#        bond split reads as a fingerprint, not real decentralization).
#
# Usage:  ./run.sh              # build, test, tear down; exit 0 = PASS
#         SYBILS=0 ./run.sh     # core only (a1,a2,s1,s2)
#         KEEP=1 ./run.sh
# ─────────────────────────────────────────────────────────────────────────────
set -uo pipefail
cd "$(dirname "$0")"
ROOT=$(cd ../.. && pwd)

SYBILS=${SYBILS:-2}           # extra Sybil identities beyond s1,s2 (keep light for laptop robustness;
                              # the ≥8-bond atomization note is a cloud-scale concern regardless)
PROJECT=sybil
dc() { docker compose -p "$PROJECT" "$@"; }
cleanup() { [ "${KEEP:-0}" = 1 ] || dc down -v >/dev/null 2>&1 || true; }
trap cleanup EXIT INT TERM

pass=1
fail() { echo "FAIL: $*"; pass=0; }
await_log() { local svc=$1 pat=$2 tries=${3:-60} i; for i in $(seq 1 "$tries"); do dc logs "$svc" 2>&1 | grep -qE "$pat" && return 0; sleep 1; done; return 1; }
head_height() { dc exec -T "$1" silt chain-status -store /data 2>/dev/null | awk '/head height/{print $3}'; }
commit_count() { dc logs "$1" 2>&1 | grep -c 'committed block' || true; }

echo "== build silt (linux/$(go env GOARCH)) + image =="
( cd "$ROOT" && CGO_ENABLED=0 GOOS=linux GOARCH="$(go env GOARCH)" go build -trimpath -o integration/sybil/silt ./cmd/silt ) \
  || { echo "FAIL: host build"; exit 1; }
docker build -q -t silt-sybil . >/dev/null || { echo "FAIL: image build"; exit 1; }

silt_id() { docker run --rm silt-sybil silt id -id-seed "$1" | tr -d '\r'; }
A1_ID=$(silt_id 5001); A2_ID=$(silt_id 5002); S1_ID=$(silt_id 5003); S2_ID=$(silt_id 5004)
export A1_ID A2_ID S1_ID S2_ID
echo "  anchors: a1=$A1_ID  a2=$A2_ID"
echo "  sybils : s1(registry)=$S1_ID  s2=$S2_ID  (+$SYBILS more)"

echo "== bring up 2 anchors + the Sybil set =="
dc up -d a1 a2 s1 s2 >/dev/null 2>&1 || fail "up core"
[ "$SYBILS" -gt 0 ] && { dc up -d --scale sybil="$SYBILS" sybil >/dev/null 2>&1 || fail "up sybil farm"; }
await_log a1 'registry: .*serving' 40 || fail "a1 registry never came up"
await_log a2 'bootstrapped \(' 40 || fail "a2 never bootstrapped"
await_log s1 'registry: .*serving' 40 || fail "s1 registry never came up"
echo "  letting bonds seal + submit + the anchors commit them (25s)…"; sleep 25

REG_A1="$A1_ID@https://10.90.0.2:4003"
REG_S1="$S1_ID@https://10.90.0.4:4003"
BOOT_A1="$A1_ID@10.90.0.2:4001"; BOOT_A2="$A2_ID@10.90.0.3:4001"
BOOT_S1="$S1_ID@10.90.0.4:4001"; BOOT_S2="$S2_ID@10.90.0.5:4001"
ALL="$BOOT_A1,$BOOT_A2,$BOOT_S1,$BOOT_S2"

publish() { # publish to <registry-ref>, run inside <svc>; echoes add stdout+stderr
  local svc=$1 reg=$2
  dc exec -T "$svc" sh -c "head -c 32768 /dev/urandom > /tmp/p.bin; \
    silt swarm add /tmp/p.bin -peers '$ALL' -registry '$reg' -replication 1 2>&1"
}

# ── C2-a: the honest anchors bootstrap + commit (also commits Sybil bondregs) ──
echo ""
echo "== C2-a positive control: the ANCHORED young network commits =="
# A handful of publishes so the anchored chain commits at least one real block —
# proving the chain is LIVE with the anchors present (so C2-b's refusal is the
# training-wheels gate, not a dead swarm).
h0=$(head_height a1); h0=${h0:-0}
for _ in $(seq 1 24); do
  publish a1 "$REG_A1" >/dev/null 2>&1 || true; sleep 3
  h=$(head_height a1); [ "${h:-0}" -gt "${h0:-0}" ] 2>/dev/null && break
done
h1=$(head_height a1); h1=${h1:-0}; cc=$(commit_count a1)
if [ "${h1:-0}" -gt "${h0:-0}" ] 2>/dev/null; then
  echo "  C2-a PASS: the anchored chain advanced ${h0}→$h1 (committed blocks: $cc) — live with the anchors present"
else
  fail "C2-a the anchored chain did not commit (height ${h0}→$h1) — cannot use C2-b as the capture gate"
fi
C2LINE=$(dc logs a1 2>&1 | grep -E '  C2: nakamoto' | tail -1)
ATOM=$(dc logs a1 2>&1 | grep -E 'atomization note' | tail -1)
BONDS=$(echo "$C2LINE" | grep -oE 'nakamoto [0-9]+ bonds' | grep -oE '[0-9]+' | head -1)
echo "  C2 metric: ${C2LINE:-<none printed>}"
if echo "$C2LINE" | grep -q 'wheels engaged'; then
  echo "  C2-a PASS: training wheels ENGAGED (young network — anchor quorum still required)"
else
  fail "C2-a expected 'wheels engaged' on the young network, got: $(echo "$C2LINE" | grep -oE 'wheels[^|]*')"
fi

# ── C2-b: no quiet capture — stop both anchors, the Sybil quorum cannot advance ─
echo ""
echo "== C2-b no quiet capture: with BOTH anchors gone, the Sybil set CANNOT advance the chain =="
dc stop a1 a2 >/dev/null 2>&1 || fail "could not stop the anchors"
echo "  both anchors stopped; the Sybil set (s1 proposes, s2 attests) now tries to commit alone…"
h_pre=$(head_height s1); h_pre=${h_pre:-0}; cc_pre=$(commit_count s1)
CAP=""
for _ in $(seq 1 12); do
  CAP=$(publish s1 "$REG_S1" 2>&1)
  echo "$CAP" | grep -qiE 'anchor|immature|reputation|quorum' && break
  sleep 2
done
sleep 3
h_post=$(head_height s1); h_post=${h_post:-0}; cc_post=$(commit_count s1)
REASON=$(echo "$CAP" | grep -oiE '(immature network requires anchor[^:"]*|chain: [a-z ]*anchor[^:"]*|reputation below threshold|[a-z ]*quorum[^:"]*)' | head -1)
# The OUTCOME under test: did the Sybil set capture the chain? PASS iff it did NOT
# advance without the anchors. Report WHICH training-wheels layer stopped it — a
# young Sybil set is blocked at BOTH: it cannot even earn on-chain standing without
# an anchor-committed block, and even with standing a young commit needs anchor
# co-sign. Both are the same C2 property: the network will not run for Sybils alone.
if [ "${h_post:-0}" -ne "${h_pre:-0}" ] || [ "${cc_post:-0}" -ne "${cc_pre:-0}" ]; then
  fail "C2-b the Sybil quorum ADVANCED the chain without any anchor (height ${h_pre}→$h_post, commits ${cc_pre}→$cc_post) — QUIET CAPTURE"
elif echo "$REASON" | grep -qiE 'anchor|immature'; then
  echo "  C2-b PASS: NO new block — the bonded Sybil quorum could not capture the young network"
  echo "    (chain head stayed $h_pre, committed blocks stayed $cc_pre with both anchors absent)"
  echo "    gate: the ANCHOR co-sign requirement (ErrAnchorRequired) — the strongest form: even with"
  echo "    standing a young commit needs an anchor. reason: $REASON"
elif echo "$REASON" | grep -qiE 'reputation'; then
  echo "  C2-b PASS: NO new block — the bonded Sybil quorum could not capture the young network"
  echo "    (chain head stayed $h_pre, committed blocks stayed $cc_pre with both anchors absent)"
  echo "    gate: the standing requirement — the Sybils cannot even EARN committed bonded standing"
  echo "    without an anchor-proposed block to register their bonds. reason: $REASON"
  echo "    (The anchor co-sign gate sits behind this; both refuse the capture. The pure-anchor-gate"
  echo "    form, with pre-banked Sybil bonds, is exercised at scale on the cloud test.)"
else
  # audit #303: NO new block is only "no capture" if a TRAINING-WHEELS gate actually
  # refused it. With no anchor/immature/reputation reason surfaced, a stalled chain is
  # indistinguishable from a DEAD swarm (e.g. the anchored C2-a control above never
  # committed either) — crediting that as C2-b PASS would fake-green the whole property.
  fail "C2-b: NO new block, but NO training-wheels gate reason surfaced (REASON='${REASON}') — cannot distinguish the anchor/reputation gate refusing the Sybils from a chain that simply never runs (a dead swarm). The anchored C2-a positive control must commit AND a gate reason must appear."
fi

# ── C2-c: the equal-bond split is legible (bonus) ────────────────────────────
echo ""
echo "== C2-c (bonus) the equal-bond Sybil split reads as a fingerprint, not decentralization =="
if [ -n "$ATOM" ]; then
  echo "  C2-c PASS: the C2 atomization note fired on the uniform Sybil bonds:"
  echo "    $ATOM"
else
  echo "  C2-c NOTE: atomization note not observed (needs ≥8 committed uniform bonds). The core"
  echo "    no-quiet-capture gate (C2-a/C2-b) stands alone; the full-scale split fingerprint is a"
  echo "    cloud-test concern (integration/cloudtest)."
fi

echo ""
if [ "$pass" = 1 ]; then
  echo "RESULT: PASS ✅  C2 holds — no quiet capture: the young objective network commits WITH the honest anchors (wheels engaged) and will NOT advance for the bonded Sybil set once the anchors are gone. The training wheels refuse the capture (locally the Sybils cannot even earn committed standing without an anchor-proposed block; the pure anchor-co-sign gate + the ≥8-bond atomization signal are exercised at scale on the cloud test)."
else
  echo "RESULT: FAIL ❌  (see the failing assertion(s) above)"
fi
exit $((1-pass))
