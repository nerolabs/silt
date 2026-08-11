#!/usr/bin/env bash
# One automated economy + blind-credit field test. Builds the real binary,
# stands up two validator/issuers (one serves the registry) + a holder + a
# freeloader on a flat container net, and asserts against REAL daemon behavior:
#
#   FIELD TEST 5 claims
#   (a) hosts earn per byte served and freeloaders go broke   -> `silt sim run economy`
#   (b) M0 adds publisher-unlinkable prepaid blind-signed publish credits:
#         - a token-less publish is REFUSED when validators -require-tokens   (control)
#         - a Publisher-identity publish is REFUSED (durable linkage rejected)(control)
#         - a tokened publish (blind sigs from -token-quorum validators) COMMITS
#           carrying NO durable Publisher — proven on the committed chain.cbor
#         - a DOUBLE-SPEND of a token is rejected  ... see the FINDING below.
#
# Usage:  ./run.sh          # build, test, tear down; exit 0 = PASS
#         KEEP=1 ./run.sh   # leave the topology up afterward to poke at
set -uo pipefail
cd "$(dirname "$0")"
ROOT=$(cd ../.. && pwd)

dc() { docker compose -p economy "$@"; }
cleanup() { [ "${KEEP:-0}" = 1 ] || dc down -v >/dev/null 2>&1 || true; }
trap cleanup EXIT

VAL1_ID=171e68f02e6f66bf9ff65c13c75d9b2b492c2f40ed61e06507cb8b227c3970d5
VAL2_ID=49a396496b0973f889ae30bf6f546ef366c535c2b13f6be4f91791cf42375fbd
PEERS="$VAL1_ID@10.80.0.11:4001,$VAL2_ID@10.80.0.12:4001"
REG="$VAL1_ID@https://10.80.0.11:4003"

pass=1
fail() { echo "FAIL: $*"; pass=0; }

echo "== build the silt binary on the host (linux/$(go env GOARCH)) and the image =="
( cd "$ROOT" && CGO_ENABLED=0 GOOS=linux GOARCH="$(go env GOARCH)" go build -trimpath -o integration/economy/silt ./cmd/silt ) \
  || { echo "FAIL: host (linux) build"; exit 1; }
docker build -q -t silt-economy . >/dev/null || { echo "FAIL: image build"; exit 1; }
# The linux binary above runs in the container; the sim (CLAIM a) runs on the
# HOST, so also build a host-native binary for it (not committed; scratch temp).
SIM_BIN=$(mktemp -t silt-economy-sim.XXXXXX)
( cd "$ROOT" && go build -trimpath -o "$SIM_BIN" ./cmd/silt ) || { echo "FAIL: host-native build"; exit 1; }
trap 'cleanup; rm -f "$SIM_BIN"' EXIT

# ─────────────────────────────────────────────────────────────────────────────
# CLAIM (a): the economy observatory — per-byte earning, freeloaders go broke.
# This runs IN-PROCESS (simnet) against the real core/credit ledger; there is no
# wire seam that exposes per-byte credit accounting on the daemon (see FINDING 2
# in README). We still assert against its REAL stdout so a regression is caught.
# ─────────────────────────────────────────────────────────────────────────────
echo ""
echo "== CLAIM (a): silt sim run economy (per-byte earning; freeloaders broke) =="
"$SIM_BIN" sim run economy >/tmp/econ_sim.out 2>&1 || fail "sim run economy errored"
sed 's/^/  sim: /' /tmp/econ_sim.out
# Gate on the sim's MACHINE-READABLE summary line (not the human prose), so a wording
# drift can't silently stop catching a broken ledger (blind field test #2 §E). Each
# assertion reads one space-free key=value field off the same `economy-summary:` line.
SUM=$(grep -E '^economy-summary:' /tmp/econ_sim.out | tail -1)
[ -n "$SUM" ] || fail "sim emitted no machine-readable 'economy-summary:' line (economy observatory did not run)"
# The top earner served >0 bytes and CAN publish again.
echo "$SUM" | grep -qE 'top_served=[1-9][0-9]*'   || fail "top earner served 0 bytes (no per-byte earning)"
echo "$SUM" | grep -qE 'top_can_publish=true'      || fail "top earner served bytes yet CANNOT re-publish (earning did not credit)"
# A freeloader served 0 bytes and CANNOT publish (broke).
echo "$SUM" | grep -qE 'freeloader_served=0'       || fail "the tracked freeloader served >0 bytes (not a real freeloader)"
echo "$SUM" | grep -qE 'freeloader_can_publish=false' || fail "a freeloader that served 0 bytes CAN still publish (freeloader not broke)"
# Every freeloader was gate-rejected on the second publish.
echo "$SUM" | grep -qE 'all_freeloaders_rejected=true' || fail "not all freeloaders were gate-rejected"
[ "$pass" = 1 ] && echo "  CLAIM (a): PASS — hosts earn per byte served; freeloaders cannot re-publish"

# ─────────────────────────────────────────────────────────────────────────────
# Bring up the real swarm for CLAIM (b).
# ─────────────────────────────────────────────────────────────────────────────
echo ""
echo "== phase: bring up validators (val2 first, then val1+registry), holder, leech =="
dc up -d val2
# val1 bootstraps to val2; both must reach standing before they can commit.
dc up -d val1 holder leech

echo "== wait for val1 to serve the registry and require tokens =="
ok=0
for _ in $(seq 1 40); do
  if dc logs val1 2>&1 | grep -qE 'registry: chain-backed'; then ok=1; break; fi
  sleep 1
done
[ "$ok" = 1 ] || { fail "val1 never served the registry"; dc logs val1 | tail -20; }
dc logs val1 2>&1 | grep -E 'publish tokens: required|registry: chain-backed' | head -2 | sed 's/^/  val1: /'
dc logs val2 2>&1 | grep -E 'publish tokens: required' | head -1 | sed 's/^/  val2: /'

echo "== let both validators accrue consensus standing (bond audits) =="
sleep 16

# ─────────────────────────────────────────────────────────────────────────────
# CLAIM (b) CONTROL 1: a token-less publish is REFUSED (-require-tokens is on).
# ─────────────────────────────────────────────────────────────────────────────
echo ""
echo "== CLAIM (b) control 1: a token-less publish is REFUSED =="
dc exec -T holder sh -c "head -c 65536 /dev/urandom > /tmp/f0.bin; silt swarm add /tmp/f0.bin -peers '$PEERS' -registry '$REG'" >/tmp/econ_add0.out 2>&1 || true
tail -1 /tmp/econ_add0.out | sed 's/^/  /'
if grep -qiE 'entry has no publish token|token.*required' /tmp/econ_add0.out; then
  echo "  control 1: PASS — token-less publish refused ('entry has no publish token (required)')"
else
  fail "token-less publish was NOT refused"; cat /tmp/econ_add0.out
fi

# ─────────────────────────────────────────────────────────────────────────────
# CLAIM (b) CONTROL 2: a durable-Publisher publish is REFUSED (unlinkability is
# ENFORCED — the chain rejects any entry that would record a permanent
# Publisher->root link, so a committed entry provably carries no Publisher).
# ─────────────────────────────────────────────────────────────────────────────
echo ""
echo "== CLAIM (b) control 2: an -allow-publisher (durable identity) publish is REFUSED =="
dc exec -T holder sh -c "silt swarm add /tmp/f0.bin -allow-publisher -peers '$PEERS' -registry '$REG'" >/tmp/econ_addp.out 2>&1 || true
tail -1 /tmp/econ_addp.out | sed 's/^/  /'
if grep -qiE 'carries a durable Publisher|permanent linkage' /tmp/econ_addp.out; then
  echo "  control 2: PASS — durable-Publisher publish refused (permanent linkage rejected)"
else
  fail "durable-Publisher publish was NOT refused"; cat /tmp/econ_addp.out
fi

# ─────────────────────────────────────────────────────────────────────────────
# CLAIM (b) POSITIVE: a tokened publish (blind sigs from -token-quorum=2 distinct
# validators) COMMITS and carries NO durable Publisher. Proven two ways:
#   (i)  val1 prints "chain: committed block N";
#   (ii) the committed entry in val1's chain.cbor carries a Token+Serial and a
#        ZERO NodeID Publisher (32 zero bytes) — the unlinkability wire proof.
# ─────────────────────────────────────────────────────────────────────────────
echo ""
echo "== CLAIM (b) positive: a tokened publish (quorum 2) COMMITS with no Publisher =="
H_BEFORE=$(dc exec -T val1 sh -c 'silt chain-status -store /data' 2>/dev/null | awk '/head height/{print $3}' | tr -d '\r')
dc exec -T holder sh -c "silt swarm add /tmp/f0.bin -token-quorum 2 -save-token /tmp/tok -peers '$PEERS' -registry '$REG'" >/tmp/econ_add1.out 2>&1 || true
LINK=$(grep -oE 'silt:v1:[A-Za-z0-9_:-]+' /tmp/econ_add1.out | head -1)
echo "  link: ${LINK:-<none>}"
sleep 4
H_AFTER=$(dc exec -T val1 sh -c 'silt chain-status -store /data' 2>/dev/null | awk '/head height/{print $3}' | tr -d '\r')
echo "  chain head height: ${H_BEFORE:-?} -> ${H_AFTER:-?}"
[ -n "$LINK" ] || fail "tokened publish returned no link"
dc logs val1 2>&1 | grep -E 'chain: committed block' | tail -1 | sed 's/^/  val1: /'
if [ "${H_AFTER:-0}" -gt "${H_BEFORE:-0}" ]; then
  echo "  positive (i): PASS — tokened publish committed (head $H_BEFORE -> $H_AFTER)"
else
  fail "tokened publish did not advance the chain head"; cat /tmp/econ_add1.out
fi

# (ii) Unlinkability on the wire: the committed user entry has a Token/Serial and
# a zero-NodeID Publisher. We copy val1's chain.cbor out and inspect it.
echo "== unlinkability proof: inspect the committed entry in val1's chain.cbor =="
dc cp val1:/data/chain.cbor /tmp/econ_chain.cbor >/dev/null 2>&1 || true
if [ -s /tmp/econ_chain.cbor ]; then
  UNLINK=$(python3 - /tmp/econ_chain.cbor <<'PY'
import sys
d=open(sys.argv[1],'rb').read()
# Each entry stamps a Publisher field: the CBOR marker "PublisherX " (X=0x58
# byte-string, 0x20=32 len) immediately followed by the 32-byte NodeID. A tokened
# (unlinkable) entry stamps 32 ZERO bytes; a durable-identity entry stamps a real
# key; and the genesis/bond entries carry the validators' real (non-zero) NodeIDs,
# so "all Publishers are zero" is NOT true. The old check only asked "does a zero
# Publisher appear SOMEWHERE and a Token appear SOMEWHERE", which would pass even
# if the TOKENED entry itself carried a durable Publisher. Tighten it by tying the
# two: CBOR sorts an entry's map keys by length, so a tokened entry's Publisher
# field falls a few dozen bytes AFTER its Token/Serial. For each Serial marker,
# require the next Publisher within the same entry to be the ZERO NodeID.
mk=b'PublisherX '
has_token = d.find(b'Token')>=0
ok=False; i=d.find(b'Serial')
while i>=0:
    p=d.find(mk,i,i+250)  # the tokened entry's Publisher follows its Serial
    if p>=0 and d[p+len(mk):p+len(mk)+32]==b'\x00'*32:
        ok=True; break
    i=d.find(b'Serial',i+1)
print("token=%s tokened_entry_zero_publisher=%s"%(has_token,ok))
sys.exit(0 if (has_token and ok) else 1)
PY
) && ZOK=1 || ZOK=0
  echo "  chain.cbor: $UNLINK"
  if [ "$ZOK" = 1 ]; then
    echo "  positive (ii): PASS — committed entry carries a Token+Serial and a ZERO Publisher (unlinkable)"
  else
    fail "committed entry did not show a token + zero-NodeID Publisher"
  fi
else
  fail "could not read val1's chain.cbor for the unlinkability proof"
fi

# ─────────────────────────────────────────────────────────────────────────────
# CLAIM (b) DOUBLE-SPEND — now GATED over the wire (#233). The positive publish
# above saved its token with -save-token; here we RE-PRESENT that same token for a
# DIFFERENT file with -use-token. Its serial is already committed, so the chain
# rejects the second publish (core/chain ErrTokenSpent, "publish token serial
# already spent (double-spend)") — the entry never commits and the head does not
# advance for it. A control publish with a FRESH token still commits, so the
# rejection is a real double-spend defence, not a dead swarm.
# ─────────────────────────────────────────────────────────────────────────────
echo ""
echo "== CLAIM (b) double-spend: RE-PRESENT the saved token for a different file (must be REJECTED) =="
H_DS_BEFORE=$(dc exec -T val1 sh -c 'silt chain-status -store /data' 2>/dev/null | awk '/head height/{print $3}' | tr -d '\r')
dc exec -T holder sh -c "head -c 40000 /dev/urandom > /tmp/f_ds.bin; silt swarm add /tmp/f_ds.bin -use-token /tmp/tok -peers '$PEERS' -registry '$REG'" >/tmp/econ_ds.out 2>&1 || true
sleep 4
H_DS_AFTER=$(dc exec -T val1 sh -c 'silt chain-status -store /data' 2>/dev/null | awk '/head height/{print $3}' | tr -d '\r')
echo "  chain head height around the replay: ${H_DS_BEFORE:-?} -> ${H_DS_AFTER:-?}"
# The registry's local pre-check rejects the replayed serial and returns the exact
# ErrTokenSpent reason to the client (as it does for the token-less control above).
if grep -qiE 'already spent|double-spend|serial.*spent' /tmp/econ_ds.out || \
   dc logs val1 2>&1 | grep -qiE 'already spent|double-spend|token.*spent'; then
  echo "  double-spend: PASS — the re-presented serial was refused with ErrTokenSpent"
  grep -iE 'already spent|double-spend|serial.*spent' /tmp/econ_ds.out | tail -1 | sed 's/^/    client: /'
  # Belt-and-braces: the replayed publish must also NOT have committed.
  [ "${H_DS_AFTER:-0}" -le "${H_DS_BEFORE:-0}" ] || fail "spent-serial was logged yet the head still advanced — a replay slipped through"
else
  fail "the re-presented token was NOT rejected with a spent-serial error over the wire"
  cat /tmp/econ_ds.out
fi

# Control: a FRESH token still commits, so the rejection above is a real defence.
echo "== double-spend control: a FRESH-token publish still COMMITS =="
H_C_BEFORE=$(dc exec -T val1 sh -c 'silt chain-status -store /data' 2>/dev/null | awk '/head height/{print $3}' | tr -d '\r')
dc exec -T holder sh -c "head -c 40000 /dev/urandom > /tmp/f_c.bin; silt swarm add /tmp/f_c.bin -token-quorum 2 -peers '$PEERS' -registry '$REG'" >/tmp/econ_c.out 2>&1 || true
sleep 4
H_C_AFTER=$(dc exec -T val1 sh -c 'silt chain-status -store /data' 2>/dev/null | awk '/head height/{print $3}' | tr -d '\r')
echo "  chain head height around the control: ${H_C_BEFORE:-?} -> ${H_C_AFTER:-?}"
if [ "${H_C_AFTER:-0}" -gt "${H_C_BEFORE:-0}" ]; then
  echo "  control: PASS — a fresh token still commits (the swarm is live; the rejection was real)"
else
  fail "fresh-token control did not commit — cannot distinguish the rejection from a dead swarm"; cat /tmp/econ_c.out
fi

echo ""
if [ "$pass" = 1 ]; then
  echo "RESULT: PASS ✅  economy observatory + blind-credit publisher-unlinkability validated,"
  echo "  and the token DOUBLE-SPEND rejection is now driven over the real wire (#233):"
  echo "  a re-presented serial is refused (ErrTokenSpent) while a fresh token still commits."
  echo "  tier note (immutable #3): CLAIM (a) per-byte earning / freeloaders-broke stays SIM-TIER"
  echo "    (in-process 'silt sim run economy' — the daemon still does not wire the credit-gated"
  echo "    registry, so per-byte balance is not yet wire-exposed; see README FINDING 2 / #233(B))."
else
  echo "RESULT: FAIL ❌"
fi
exit $((1-pass))
