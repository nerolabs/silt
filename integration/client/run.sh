#!/usr/bin/env bash
# ─────────────────────────────────────────────────────────────────────────────
# Field test #4 — the CLIENT / WEB-UI path, adversary-aware, in real Docker.
#
# The outcome a real USER experiences: they run a daemon, open its web UI, drop a
# file in, get a link, and someone fetches it back — all over the daemon's HTTP
# API, not the `silt swarm` CLI. And the outcome a real ATTACKER experiences: a
# malicious web page or a LAN neighbour trying to drive that same local API must
# be REFUSED. Both are asserted against the live HTTP surface (curl, real status
# codes + real bytes), never a string the harness echoes.
#
# The UI is reached from INSIDE the operator's own container — the realistic
# model (a browser on the same machine), and the only way the guard's local-Host
# rule is satisfied. The attacks then spoof Host / Origin / token to prove the
# guard (#89) holds.
#
#   U1 publish:      POST /api/publish (multipart + bearer token) scatters the
#                    file across the swarm and returns a link. POSITIVE CONTROL:
#                    we then read each holder's own GET /api/status held-chunk
#                    count and require >=2 DISTINCT holders actually hold shards
#                    — otherwise every shard could have landed on the ui daemon
#                    alone (a valid remote node) with zero holders participating,
#                    and a single-node loopback would masquerade as a swarm
#                    scatter. That would be a false PASS on a broken product.
#   U2 roots:        GET /api/roots then lists the new root — AND the U1 holder
#                    floor still holds, so the "reflected" publish is a real one.
#   U3 fetch:        GET /api/fetch?link=… returns the bytes BIT-PERFECT — a real
#                    end-to-end user round-trip over HTTP that had to traverse the
#                    swarm (holders held the shards), not a ui-node-only replay.
#   U4 no token:     POST /api/publish with NO token → 401 (a drive-by can't publish).
#   U5 wrong token:  POST /api/publish with a bad token → 401.
#   U6 DNS-rebinding: any request with a non-local Host → 403.
#   U7 cross-origin: any request with an evil Origin → 403.
#   U8 ergonomics:   GET /api/status with no token → 200 (reads stay frictionless).
#
# Usage:  ./run.sh          # build, test, tear down; exit 0 = PASS
#         KEEP=1 ./run.sh   # leave the topology up to poke at
# exit 0 = PASS; non-zero = FAIL
# ─────────────────────────────────────────────────────────────────────────────
set -uo pipefail
cd "$(dirname "$0")"
ROOT=$(cd ../.. && pwd)

HOLDERS=${HOLDERS:-4}
PROJECT=client
dc() { docker compose -p "$PROJECT" "$@"; }
cleanup() { [ "${KEEP:-0}" = 1 ] || dc down -v >/dev/null 2>&1 || true; }
trap cleanup EXIT INT TERM

pass=1
fail() { echo "FAIL: $*"; pass=0; }
# curl the UI from INSIDE the ui container (Host defaults to 127.0.0.1 → local).
ucurl() { dc exec -T ui curl -s "$@"; }
# just the HTTP status code of a request.
code()  { dc exec -T ui curl -s -o /dev/null -w '%{http_code}' "$@"; }

# held_chunks IP — a holder's own held-chunk count, read from its GET /api/status
# from INSIDE the ui container. The guard requires a LOCAL Host, so we spoof
# Host: 127.0.0.1 while dialing the holder's real swarm IP (a read needs no
# token). This is the positive control: it proves a UI publish actually landed
# data on live holders, not just on the ui daemon alone (a single-node loopback
# would satisfy "over the wire" without any holder holding a byte).
held_chunks() {
  dc exec -T ui curl -s -H 'Host: 127.0.0.1:8081' "http://$1:8081/api/status" \
    | sed -En 's/.*"chunks":[[:space:]]*([0-9]+).*/\1/p'
}
# holders_with_data — how many DISTINCT holders report >0 held chunks right now.
holders_with_data() {
  local n=0 ip c
  for hid in $(dc ps -q holder); do
    ip=$(docker inspect -f '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}' "$hid")
    [ -n "$ip" ] || continue
    c=$(held_chunks "$ip"); c=${c:-0}
    [ "$c" -gt 0 ] 2>/dev/null && n=$((n+1))
  done
  echo "$n"
}

echo "== build silt (linux/$(go env GOARCH)) + image =="
( cd "$ROOT" && CGO_ENABLED=0 GOOS=linux GOARCH="$(go env GOARCH)" go build -trimpath -o integration/client/silt ./cmd/silt ) \
  || { echo "FAIL: host build"; exit 1; }
docker build -q -t silt-client . >/dev/null || { echo "FAIL: image build"; exit 1; }

echo "== phase 1: the operator's UI node =="
dc up -d ui >/dev/null 2>&1 || fail "up ui"
UI_ID=""; TOKEN=""
for _ in $(seq 1 40); do
  UI_ID=$(dc logs ui 2>&1 | grep -oE 'peer: [a-f0-9]{64}' | head -1 | awk '{print $2}')
  TOKEN=$(dc logs ui 2>&1 | grep -oE 'ui: https?://[^ ]*token=[A-Za-z0-9_-]+' | head -1 | sed -E 's/.*token=([A-Za-z0-9_-]+).*/\1/')
  [ -n "$UI_ID" ] && [ -n "$TOKEN" ] && break; sleep 1
done
UI_IP=$(docker inspect -f '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}' "$(dc ps -q ui)")
[ -n "$UI_ID" ] && [ -n "$UI_IP" ] && [ -n "$TOKEN" ] || { fail "ui never reported NodeID/IP/token"; echo "RESULT: FAIL ❌"; exit 1; }
export UI_PEER="$UI_ID@$UI_IP:4001" UI_REG="$UI_ID@https://$UI_IP:4003"
echo "  ui peer: $UI_PEER"
echo "  ui token: ${TOKEN:0:8}… (grabbed from the daemon's own 'ui: …?token=' line)"

echo "== phase 2: $HOLDERS holders (so a UI publish really scatters) =="
dc up -d --scale holder="$HOLDERS" holder >/dev/null 2>&1 || fail "up holders"
bootok=0
for id in $(dc ps -q holder); do
  for _ in $(seq 1 60); do docker logs "$id" 2>&1 | grep -q bootstrapped && { bootok=$((bootok+1)); break; }; sleep 1; done
done
echo "  $bootok/$HOLDERS holders bootstrapped"
# A UI publish can only "really scatter" if live holders actually joined the
# swarm. If none bootstrapped, the holder-participation controls below can
# never pass on a HEALTHY product either, so this is a setup failure, not a
# product finding — surface it now instead of letting U1 mislabel it.
[ "${bootok:-0}" -ge 2 ] 2>/dev/null || fail "phase 2 only $bootok/$HOLDERS holders bootstrapped — no swarm to scatter onto (need >=2)"
# let objective standing accrue so registry writes are accepted
sleep 8

# ── U1: publish a file through the web UI ────────────────────────────────────
roots_total() { ucurl http://127.0.0.1:8081/api/roots | sed -En 's/.*"total":[[:space:]]*([0-9]+).*/\1/p'; }

echo ""
echo "== U1 publish via POST /api/publish (multipart + bearer token) =="
ROOTS_BEFORE=$(roots_total); ROOTS_BEFORE=${ROOTS_BEFORE:-0}
dc exec -T ui sh -c "head -c 1500000 /dev/urandom > /tmp/up.bin"
WANT=$(dc exec -T ui sh -c "sha256sum /tmp/up.bin | cut -d' ' -f1")
PUB=""
for _ in $(seq 1 20); do
  PUB=$(ucurl -H "Authorization: Bearer $TOKEN" -F "file=@/tmp/up.bin" http://127.0.0.1:8081/api/publish)
  echo "$PUB" | grep -qoE 'silt:v1:[A-Za-z0-9_:-]+' && break; sleep 3
done
echo "  response: $(echo "$PUB" | head -c 240)"
LINK=$(echo "$PUB" | grep -oE 'silt:v1:[A-Za-z0-9_:-]+' | head -1)
PLACED=$(echo "$PUB" | sed -En 's/.*"placed":[[:space:]]*([0-9]+).*/\1/p')
# Positive control: a report of "placed" is not enough — ALL shards could have
# landed on the ui daemon itself (a valid remote node) with zero holders
# participating, and this test would still go green on a broken swarm. So gate
# on data actually reaching DISTINCT live holders. Poll: holders accept shards
# and re-advertise asynchronously, so give it a few seconds to settle.
HWD=0
for _ in $(seq 1 20); do
  HWD=$(holders_with_data); HWD=${HWD:-0}
  [ "$HWD" -ge 2 ] 2>/dev/null && break; sleep 3
done
echo "  distinct holders now holding shards: $HWD/$HOLDERS"
if [ -n "$LINK" ] && [ "${PLACED:-0}" -ge 2 ] 2>/dev/null && [ "${HWD:-0}" -ge 2 ] 2>/dev/null; then
  echo "  U1 PASS: the UI published a link ($LINK), scattered $PLACED shards onto $HWD distinct holders — a real swarm scatter, not a single-node loopback"
else
  fail "U1 the UI publish did not really scatter onto the swarm (link=$LINK placed=$PLACED holders_with_data=$HWD, need placed>=2 AND >=2 distinct holders holding shards)"
fi

# ── U2: the new root shows up in the roots listing ───────────────────────────
echo ""
echo "== U2 GET /api/roots reflects the new publish =="
# /api/roots reports the root in hex; the link carries it base64url — same bytes,
# different encoding — so assert the COUNT grew rather than string-matching.
ROOTS_AFTER=$(roots_total); ROOTS_AFTER=${ROOTS_AFTER:-0}
# The roots count alone lives on the SAME ui daemon that could hold the whole
# file alone, so pair it with the holder-participation floor from U1: the
# publish is only "reflected" if it also genuinely reached the swarm.
if [ "${ROOTS_AFTER:-0}" -gt "${ROOTS_BEFORE:-0}" ] 2>/dev/null && [ "${HWD:-0}" -ge 2 ] 2>/dev/null; then
  echo "  U2 PASS: /api/roots grew ${ROOTS_BEFORE}→$ROOTS_AFTER after the UI publish (with $HWD distinct holders holding shards)"
else
  fail "U2 /api/roots did not reflect a real swarm publish (total ${ROOTS_BEFORE}→$ROOTS_AFTER, holders_with_data=$HWD)"
fi

# ── U3: fetch it back through the UI, bit-perfect ────────────────────────────
echo ""
echo "== U3 GET /api/fetch?link=… returns the bytes BIT-PERFECT =="
GOT=""
for _ in $(seq 1 15); do
  dc exec -T ui sh -c "curl -s -G --data-urlencode 'link=$LINK' http://127.0.0.1:8081/api/fetch -o /tmp/down.bin"
  GOT=$(dc exec -T ui sh -c "sha256sum /tmp/down.bin 2>/dev/null | cut -d' ' -f1")
  [ "$GOT" = "$WANT" ] && break; sleep 3
done
if [ -n "$GOT" ] && [ "$GOT" = "$WANT" ] && [ "${HWD:-0}" -ge 2 ] 2>/dev/null; then
  echo "  U3 PASS: the UI fetched the file back bit-perfect (sha ${WANT:0:16}…) — a real end-to-end round-trip that had to traverse the swarm ($HWD distinct holders held shards)"
else
  fail "U3 the UI fetch was not a genuine over-the-wire round-trip (got=${GOT:-<none>} want=$WANT holders_with_data=$HWD — a bit-perfect fetch served by the ui node alone is not end-to-end)"
fi

# ── U4–U8: the guard (#89) — attack the local API surface ────────────────────
echo ""
echo "== U4 a drive-by publish with NO token → 401 =="
C=$(code -F "file=@/tmp/up.bin" http://127.0.0.1:8081/api/publish)
[ "$C" = 401 ] && echo "  U4 PASS: no-token POST refused (401)" || fail "U4 no-token publish returned $C, want 401"

echo ""
echo "== U5 a publish with a WRONG token → 401 =="
C=$(code -H "Authorization: Bearer not-the-real-token" -F "file=@/tmp/up.bin" http://127.0.0.1:8081/api/publish)
[ "$C" = 401 ] && echo "  U5 PASS: wrong-token POST refused (401)" || fail "U5 wrong-token publish returned $C, want 401"

echo ""
echo "== U6 a DNS-rebinding request (non-local Host) → 403 =="
C=$(code -H "Host: evil.example.com" http://127.0.0.1:8081/api/status)
[ "$C" = 403 ] && echo "  U6 PASS: non-local Host refused (403) — DNS-rebinding defense holds" || fail "U6 rebinding Host returned $C, want 403"

echo ""
echo "== U7 a cross-origin drive-by (evil Origin) → 403 =="
C=$(code -H "Origin: http://evil.example.com" http://127.0.0.1:8081/api/status)
[ "$C" = 403 ] && echo "  U7 PASS: cross-origin request refused (403)" || fail "U7 cross-origin returned $C, want 403"

echo ""
echo "== U8 a plain local read needs NO token → 200 =="
C=$(code http://127.0.0.1:8081/api/status)
[ "$C" = 200 ] && echo "  U8 PASS: token-free localhost read works (200) — ergonomics preserved" || fail "U8 plain read returned $C, want 200"

echo ""
if [ "$pass" = 1 ]; then
  echo "RESULT: PASS ✅  the web-UI path delivers for the user (publish scattered onto >=2 distinct live holders → list → fetch bit-perfect over HTTP, a genuine swarm round-trip) AND holds against a hostile page/neighbour (no-token & wrong-token POST → 401, DNS-rebinding & cross-origin → 403, token-free reads still 200)"
else
  echo "RESULT: FAIL ❌  (see the failing assertion(s) above)"
fi
exit $((1-pass))
