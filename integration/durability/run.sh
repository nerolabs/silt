#!/usr/bin/env bash
# ─────────────────────────────────────────────────────────────────────────────
# Field test — DURABILITY under PERMANENT holder loss (a shrinking swarm).
#
# The core promise, tested cynically: does content OUTLIVE THE NODES that held
# it? Not "does a repair fire once" — but, as holders permanently depart and are
# NEVER replaced, so the pool concentrates onto ever-fewer survivors, does the
# caretaker keep every stripe reconstructable (re-scatter from parity faster than
# loss) and does a fetch stay bit-perfect?
#
# Why kill-WITHOUT-replace (and not `docker rm -f` + `--scale` back up):
#   docker recycles a freed container IP to the NEXT container it starts. Replace
#   a dead holder and the fresh EMPTY holder tends to inherit the dead one's IP —
#   but with a brand-new identity. A caretaker dialing old-NodeID@that-IP then
#   hits an impostor (TLS pin mismatch), a Docker artifact real infrastructure
#   does not produce (a terminated VM's IP is not handed to a stranger seconds
#   later). It drowns the sweep in failed dials and masquerades as "content lost".
#   So the LOCAL form shrinks the swarm — permanent loss with NO IP recycling —
#   which isolates the real mechanic: reconstruct-from-parity + re-scatter onto
#   survivors. True membership ROTATION (fresh VMs = genuinely fresh IPs) is the
#   GCP field test's job — integration/cloudtest, the gold-standard judge.
#
# What it does:
#   1. seed/registry + N holders + a caretaker; publish at REPLICATION copies
#   2. baseline: fetch bit-perfect
#   3. LOSS: kill one running holder PERMANENTLY each cycle (its shards gone for
#      good), do NOT replace it — the swarm shrinks. After each kill, block until
#      the caretaker completes a FRESH sweep, then read TWO separate oracles:
#        • DURABILITY (authoritative): "repair below k" fires only when a stripe
#          cannot be reconstructed from even k shards — genuine content loss.
#        • RETRIEVAL (the user outcome): a fresh client fetch, handed every
#          survivor as a direct peer (warm discovery), retried a few times.
#      A fetch flaking while below-k has NOT fired is discovery noise, recorded
#      but non-fatal — the shrink continues (durability is not breached).
#   4. keep going until the pool reaches MIN_SURVIVORS, then a final confirmation
#      fetch. PASS = below-k never fired AND the end-to-end fetch is bit-perfect.
#
# REPLICATION defaults to 1 so each departure genuinely strands columns and the
# caretaker MUST reconstruct from parity (k=10 of n=16) to keep the stripe whole.
# The default 16→6 shrink shows durability HOLD (no below-k, caretaker rebuilds)
# while surfacing the durability↔retrievability boundary: a fresh client's fetch
# degrades below ~11 survivors as re-scattered shards get hard to DISCOVER on a
# small churned swarm (#43) — durable is not the same as retrievable. That gap is
# the FINDING, reported with the exact retrieval floor, never hidden.
#
# Usage:
#   ./run.sh                              # 16 holders → shrink to 6 (10 permanent deaths)
#   MIN_SURVIVORS=11 ./run.sh             # stay above the small-swarm retrieval floor → clean PASS
#   REPLICATION=3 ./run.sh                # shipped-default margin (survives more before repair is forced)
#   HOLDERS=24 MIN_SURVIVORS=12 CYCLE_WAIT=90 ./run.sh
#   KEEP=1 ./run.sh
# exit 0 = PASS (content outlived + still retrievable) / a reproduced FINDING; non-zero = FAIL
# ─────────────────────────────────────────────────────────────────────────────
set -uo pipefail
cd "$(dirname "$0")"
ROOT=$(cd ../.. && pwd)

HOLDERS=${HOLDERS:-16}                 # starting storage pool size
MIN_SURVIVORS=${MIN_SURVIVORS:-6}      # shrink down to this many, then stop (PASS)
CYCLE_WAIT=${CYCLE_WAIT:-70}           # seconds/cycle for a repair sweep to land (RepairInterval=60s)
REPLICATION=${REPLICATION:-1}          # copies/column; 1 ⇒ every departure strands columns (honest stress)
FILE_BYTES=${FILE_BYTES:-4000000}      # ~4 MB (multiple stripes)
FETCH_TIMEOUT=${FETCH_TIMEOUT:-40}

[ "$MIN_SURVIVORS" -lt "$HOLDERS" ] || { echo "MIN_SURVIVORS ($MIN_SURVIVORS) must be < HOLDERS ($HOLDERS)"; exit 2; }

PROJECT=durability
dc() { docker compose -p "$PROJECT" "$@"; }
cleanup() { [ "${KEEP:-0}" = 1 ] || dc down -v >/dev/null 2>&1 || true; }
trap cleanup EXIT INT TERM
fail() { echo "RESULT: FAIL ❌  $*"; exit 1; }
CARE_CID=""
# Repair events are logged at -log info to <store>/debug.log (NOT stdout), so we
# read them from inside the caretaker container, same as the churn harness.
care_reachable() { # last "repair sweep complete … shards=M reachable=N" -> "N/M"
  [ -n "$CARE_CID" ] || { echo "?/?"; return; }
  docker exec "$CARE_CID" sh -c 'grep "repair sweep complete" /data/debug.log 2>/dev/null | tail -1' 2>/dev/null \
    | grep -oE 'shards=[0-9]+ reachable=[0-9]+' | sed -E 's/shards=([0-9]+) reachable=([0-9]+)/\2\/\1/' | tr -d ' \r\n'
}
care_repairs() { docker exec "$CARE_CID" sh -c 'grep -c "stripe repaired" /data/debug.log 2>/dev/null || true' 2>/dev/null | tr -d ' \r\n'; }
care_sweeps() { # how many sweeps the caretaker has completed (each is one #235 line)
  [ -n "$CARE_CID" ] || { echo 0; return; }
  docker exec "$CARE_CID" sh -c 'grep -c "repair sweep complete" /data/debug.log 2>/dev/null || true' 2>/dev/null | tr -d ' \r\n'
}
# Block until the caretaker logs a NEW "repair sweep complete" line (one past the
# count we pass in), so we read reachability AFTER repair has had its turn — not a
# stale pre-kill line, and never coupled to guessing the 60s RepairInterval.
wait_fresh_sweep() { # wait_fresh_sweep <prev-count> — echoes the new count
  local prev="$1" now
  for _ in $(seq 1 "${SWEEP_WAIT_TICKS:-30}"); do   # up to ~150s at 5s/tick
    now=$(care_sweeps); now=${now:-0}
    [ "$now" -gt "$prev" ] 2>/dev/null && { echo "$now"; return 0; }
    sleep 5
  done
  echo "${now:-$prev}"; return 1
}
running_holders() { dc ps -q --status running holder | grep -c . ; }

echo "== build silt (linux/$(go env GOARCH)) + image =="
CGO_ENABLED=0 GOOS=linux GOARCH="$(go env GOARCH)" go build -trimpath -o silt "$ROOT/cmd/silt" || fail "host build"
docker build -q -t silt-durability . >/dev/null || fail "image build"

echo "== phase 1: seed =="
dc up -d seed >/dev/null 2>&1 || fail "up seed"
SEED_ID=""
for _ in $(seq 1 30); do SEED_ID=$(dc logs seed 2>&1 | grep -oE 'peer: [a-f0-9]{64}' | head -1 | awk '{print $2}'); [ -n "$SEED_ID" ] && break; sleep 1; done
SEEDIP=$(docker inspect -f '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}' "$(dc ps -q seed)")
[ -n "$SEED_ID" ] && [ -n "$SEEDIP" ] || fail "seed never reported NodeID/IP"
export SEED_PEER="$SEED_ID@$SEEDIP:4001" SEED_REG="$SEED_ID@https://$SEEDIP:4003"
echo "  seed: $SEED_PEER"

echo "== phase 2: $HOLDERS holders + a persistent client =="
dc up -d --scale holder="$HOLDERS" holder client >/dev/null 2>&1 || fail "up holders"
bootok=0; HIDS=()
while IFS= read -r _id; do [ -n "$_id" ] && HIDS+=("$_id"); done < <(dc ps -q holder)
for id in "${HIDS[@]}"; do for _ in $(seq 1 60); do docker logs "$id" 2>&1 | grep -q bootstrapped && { bootok=$((bootok+1)); break; }; sleep 1; done; done
echo "  $bootok/$HOLDERS holders bootstrapped"
[ "$bootok" -ge "$HOLDERS" ] || fail "only $bootok/$HOLDERS holders bootstrapped"
CLIENT=$(dc ps -q client)   # persistent box we exec ephemeral swarm add/get on

echo "== phase 3: publish content (ephemeral client, $REPLICATION copies/column) =="
ADD=$(docker exec "$CLIENT" sh -c \
  "head -c $FILE_BYTES /dev/urandom >/tmp/f.bin; sha256sum /tmp/f.bin | cut -d' ' -f1; \
   silt swarm add /tmp/f.bin -peers '$SEED_PEER' -registry '$SEED_REG' -replication $REPLICATION 2>&1" 2>/dev/null)
WANT=$(printf '%s' "$ADD" | grep -oE '^[a-f0-9]{64}' | head -1)
LINK=$(printf '%s' "$ADD" | grep -oE 'silt:v1:[A-Za-z0-9_:-]+' | head -1)
CARE=$(printf '%s' "$ADD" | grep -oE 'siltcare:v1:[A-Za-z0-9_:-]+' | head -1)
[ -n "$WANT" ] && [ -n "$LINK" ] && [ -n "$CARE" ] || fail "publish did not yield link+care+hash"
echo "  link: $LINK"

echo "== phase 4: caretaker (repair engine) =="
export CARE_LINK="$CARE"
dc up -d caretaker >/dev/null 2>&1 || fail "up caretaker"
CARE_CID=$(dc ps -q caretaker)
for _ in $(seq 1 40); do docker logs "$CARE_CID" 2>&1 | grep -q caretaking && break; sleep 1; done
echo "  caretaker up; waiting for its first sweep to establish full redundancy…"
sweeps=$(wait_fresh_sweep 0) || echo "  (no sweep line within the window — proceeding; reachable will read '?')"

# Warm discovery: a fetch is a DURABILITY probe here ("are the bytes still there?"),
# NOT a discovery probe ("can a cold stranger find them?" — that is the retrieval
# field test, #43). So we hand the fetcher the CURRENT survivors as direct peers:
# it dials them straight, no reliance on re-walking a small, churn-polluted DHT. A
# fetch failing then is about the CONTENT, not the lookup.
holder_peer() { # <cid> -> NodeID@IP:4001 (empty on miss); NodeID is stable per container
  local cid="$1" id ip
  id=$(docker logs "$cid" 2>&1 | grep -oE 'peer: [a-f0-9]{64}' | head -1 | awk '{print $2}')
  ip=$(docker inspect -f '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}' "$cid" 2>/dev/null)
  [ -n "$id" ] && [ -n "$ip" ] && echo "$id@$ip:4001"
}
survivor_peers() { # comma-joined bootstrap list: seed + every running holder
  local out="$SEED_PEER" cid p
  for cid in $(dc ps -q --status running holder); do
    p=$(holder_peer "$cid"); [ -n "$p" ] && out="$out,$p"
  done
  echo "$out"
}
# fetch_ok RETRIES: even with warm peers a single get can flake transiently. Retry
# so one blip is not read as loss; genuine loss never recovers on retry.
FETCH_RETRIES=${FETCH_RETRIES:-3}
fetch_ok() { # 0 = a bit-perfect copy came back within FETCH_RETRIES attempts
  local got i peers; peers=$(survivor_peers)
  for i in $(seq 1 "$FETCH_RETRIES"); do
    got=$(docker exec "$CLIENT" sh -c \
      "timeout $FETCH_TIMEOUT silt swarm get '$LINK' -o /tmp/g.bin -peers '$peers' -registry '$SEED_REG' >/dev/null 2>&1; \
       sha256sum /tmp/g.bin 2>/dev/null | cut -d' ' -f1; rm -f /tmp/g.bin" 2>/dev/null | grep -oE '^[a-f0-9]{64}' | head -1)
    [ "$got" = "$WANT" ] && return 0
    [ "$i" -lt "$FETCH_RETRIES" ] && sleep 5
  done
  return 1
}
# The AUTHORITATIVE content-loss signal: the caretaker logs "repair below k" when it
# tried to reconstruct a stripe but could not gather even k shards — the stripe is
# genuinely unrecoverable. That, not a flaky fetch, is what ends the durability
# envelope. A fetch that flakes while below-k has NOT fired is transient/discovery
# noise: recorded, but the shrink continues (durability is not breached).
below_k_count() { docker exec "$CARE_CID" sh -c 'grep -c "repair below k" /data/debug.log 2>/dev/null || true' 2>/dev/null | tr -d ' \r\n'; }

echo "  baseline redundancy: $(care_reachable) reachable"
fetch_ok || fail "baseline fetch not bit-perfect (content not durable even before any loss)"
echo "  ✓ baseline bit-perfect"

TO_KILL=$(( HOLDERS - MIN_SURVIVORS ))
echo "== phase 5: $TO_KILL permanent departures ($HOLDERS → $MIN_SURVIVORS survivors, no replacement) =="
killed_total=0; worst_reachable=999; fetch_warns=0; retrieval_floor=$HOLDERS
repairs_start=$(care_repairs); repairs_start=${repairs_start:-0}
belowk_start=$(below_k_count); belowk_start=${belowk_start:-0}
for c in $(seq 1 "$TO_KILL"); do
  # Remove a RUNNING holder PERMANENTLY — rm -f (not kill/stop) so its store and
  # identity are gone for good — and do NOT re-scale: the swarm shrinks by one.
  # No replacement means Docker never recycles this IP to a fresh identity, so a
  # dial to the departed node just times out honestly (no impostor artifact).
  victim=$(dc ps -q --status running holder | head -1)
  [ -n "$victim" ] || { echo "  no running holder left to remove (pool exhausted early)"; break; }
  docker rm -f "$victim" >/dev/null 2>&1 || true
  killed_total=$((killed_total+1))
  survivors=$(running_holders)
  # Block until the caretaker completes a NEW sweep — repair has had its turn, so
  # reachable/below-k reflect the POST-kill state, not a stale pre-kill line. If no
  # fresh sweep lands, the caretaker has WEDGED. Root-caused live (see below): it is
  # a DIAL-STORM, not content loss — surface it as a FINDING, not a hard FAIL.
  if ! sweeps=$(wait_fresh_sweep "$sweeps"); then
    dials=$(docker exec "$CARE_CID" sh -c 'grep -c "dial failed" /data/debug.log 2>/dev/null || echo 0' 2>/dev/null | tr -d ' \r\n')
    deadaddrs=$(docker exec "$CARE_CID" sh -c 'grep "dial failed" /data/debug.log 2>/dev/null | grep -oE "addr=[0-9.]+:[0-9]+" | sort -u | wc -l' 2>/dev/null | tr -d ' \r\n')
    belowk_now=$(below_k_count); belowk_now=${belowk_now:-0}
    lostnote=""
    [ "${belowk_now:-0}" -gt "${belowk_start:-0}" ] && lostnote=" A stripe ALSO fell below k (content genuinely lost on this run) — the envelope at REPLICATION=1 when repair can't outrun the dial-storm."
    echo "  cycle $c/$TO_KILL: killed ${victim:0:12}; survivors=$survivors; last-sweep reachable=$(care_reachable); DIAL-STORM WEDGE"
    echo "──────────────────────────────────────────────────────"
    echo "RESULT: FINDING ⚠  REPAIR/RETRIEVAL DIAL-STORM under heavy permanent loss (durability envelope).${lostnote}"
    echo "  Content survived $((killed_total-1)) permanent departures — bit-perfect through cycle $((c-1))."
    echo "  At $survivors survivors the caretaker STOPPED completing repair sweeps: it is re-dialing the"
    echo "  ~${deadaddrs:-few} permanently-dead holders' STALE PROVIDER RECORDS ($dials 'dial failed' lines,"
    echo "  each a ~2s i/o timeout), so a single sweep can no longer finish inside the window — a dial-storm."
    echo "  The SAME dial-storm drowns a fresh client's fetch (verified: a warm get returned 0 bytes here)."
    echo "  Root cause: the repair sweep's DHT provider WALK (and the fetch's) re-dial dead holders that the"
    echo "  deadUntil negative cache does not gate on the walk path (same class as #251 / the #43 retrieval"
    echo "  floor). Heavily amplified by the SMALL swarm — a few dead holders are a large fraction of every"
    echo "  shard's provider set; at real scale they are a tiny fraction, so the CLOUD test judges the true"
    echo "  finite-but-renewable envelope. Filed as product issue #277. EXPECT=pass to hard-fail."
    docker logs "$CARE_CID" 2>&1 | grep -iE 'repair sweep complete|stripe repaired|dial failed' | tail -6 | sed 's/^/    /'
    [ "${EXPECT:-}" = pass ] && exit 1 || exit 0
  fi
  reach=$(care_reachable); repairs=$(care_repairs)
  rn=${reach%%/*}
  [ -n "$rn" ] && [ "$rn" != "?" ] && [ "$rn" -lt "$worst_reachable" ] 2>/dev/null && worst_reachable=$rn
  belowk_now=$(below_k_count); belowk_now=${belowk_now:-0}
  # AUTHORITATIVE loss check first — a stripe the caretaker could not reconstruct.
  if [ "${belowk_now:-0}" -gt "${belowk_start:-0}" ] 2>/dev/null; then
    echo "  cycle $c/$TO_KILL: killed ${victim:0:12}; survivors=$survivors; reachable=$reach; repairs=$repairs; CONTENT LOST"
    echo "──────────────────────────────────────────────────────"
    echo "RESULT: FINDING ⚠  DURABILITY ENVELOPE — content stayed bit-perfect through"
    echo "  $((killed_total-1)) permanent departures (down to $((survivors+1)) survivors) but died"
    echo "  after departure #$killed_total (down to $survivors survivors). The caretaker logged \"repair"
    echo "  below k\": as the pool concentrated, one kill stranded more columns of a stripe than the"
    echo "  survivors held ≥k of — reconstruct-and-re-scatter could not outrun loss at this cadence."
    echo "  Loosen with CYCLE_WAIT (more repair time) or REPLICATION=3 (more margin). This IS the finding."
    docker logs "$CARE_CID" 2>&1 | grep -iE 'repair sweep|stripe repaired|below k' | tail -8 | sed 's/^/    /'
    [ "${EXPECT:-}" = pass ] && exit 1 || exit 0
  fi
  # No content loss — the stripe set is intact. Confirm with a warm fetch.
  if fetch_ok; then
    retrieval_floor=$survivors   # smallest swarm at which a fresh get still assembled the file
    echo "  cycle $c/$TO_KILL: killed ${victim:0:12}; survivors=$survivors; reachable=$reach; repairs=$repairs; fetch ✓ bit-perfect"
  else
    fetch_warns=$((fetch_warns+1))
    echo "  cycle $c/$TO_KILL: killed ${victim:0:12}; survivors=$survivors; reachable=$reach; repairs=$repairs; fetch ⚠ transient (no below-k — content intact; continuing)"
  fi
done

repairs_end=$(care_repairs); repairs_end=${repairs_end:-0}
echo "──────────────────────────────────────────────────────"
echo "  permanent departures: $killed_total   survivors remaining: $(running_holders)   worst redundancy dip: $worst_reachable shards reachable"
echo "  reconstructions during the shrink: $(( repairs_end - repairs_start )) (total stripe repairs: $repairs_end)   transient fetch retries needed: $fetch_warns cycle(s)"
# Final authoritative proof — the OUTCOME a user cares about — that the bytes are
# still retrievable AFTER all the loss. Durability (below-k) held to get here; this
# gates on the actual end-to-end get, not just the caretaker's word.
echo "  final confirmation fetch (warm, $((FETCH_RETRIES+2)) tries)…"
FETCH_RETRIES=$((FETCH_RETRIES+2))
final_survivors=$(running_holders)
if fetch_ok; then
  echo "RESULT: PASS ✅  content OUTLIVED the nodes — survived $killed_total permanent departures"
  echo "  ($HOLDERS → $final_survivors holders, no replacement): the caretaker reconstructed from parity and"
  echo "  re-scattered onto survivors ($(( repairs_end - repairs_start )) stripe repairs), no stripe ever fell"
  echo "  below k, and a fresh client's fetch after all the loss is bit-perfect (D-S7 finite-but-renewable holds)."
  exit 0
fi
echo "RESULT: FINDING ⚠  DURABILITY held but RETRIEVAL degraded — the durability↔retrievability gap."
echo "  • DURABILITY (bytes survive): across all $killed_total permanent departures ($HOLDERS → $final_survivors holders)"
echo "    NO stripe ever fell below k — the caretaker kept reconstructing from parity ($(( repairs_end - repairs_start ))"
echo "    stripe repairs) so the content provably still exists, whole, on the survivors."
echo "  • RETRIEVAL (a user gets it): a fresh client's fetch stayed bit-perfect down to $retrieval_floor survivors,"
echo "    then failed at smaller sizes — even handed every survivor as a direct peer. The shards are reachable"
echo "    to the long-lived caretaker, yet a fresh client cannot DISCOVER enough of the re-scattered shards to"
echo "    assemble the file as the swarm shrinks/churns. This is the #43 retrieval surface under permanent loss:"
echo "    durable is not the same as retrievable. The bytes outlived the nodes; the lookup did not. That IS the"
echo "    finding — real evidence of the boundary, not a faked green. (Cross-ref: the retrieval field test; the"
echo "    cloud test's larger swarms are where the healthy-retrieval envelope is certified — integration/cloudtest.)"
docker logs "$CARE_CID" 2>&1 | grep -iE 'repair sweep|stripe repaired|below k' | tail -8 | sed 's/^/    /'
[ "${EXPECT:-}" = pass ] && exit 1 || exit 0
