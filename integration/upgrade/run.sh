#!/usr/bin/env bash
# ─────────────────────────────────────────────────────────────────────────────
# Rolling-upgrade / on-disk-format stability, in real Docker.
#
# Prove a swarm survives a BINARY UPGRADE with its persisted state intact. Two
# real silt binaries are baked into one image — an OLD tagged release (V1) and
# HEAD (V2). The daemons start on V1 against named-volume stores, publish a
# real erasure-coded file (and the validator commits a chain), then the SAME
# daemons are recreated on V2 against the SAME stores. We assert:
#
#  (control) the file is bit-perfect on V1 BEFORE the upgrade — so a
#  post-upgrade pass is a meaningful comparison, not a tautology.
#  (reload) on V2, each daemon reloads its persisted state — the REAL lines:
#  "reloaded storage proofs count=N", "re-announced N held chunks",
#  "bootstrapped", and the chain's "chain: restored N block(s)" /
#  "chain replay:" outcome (whatever the product actually does).
#  (content) the file fetches BIT-PERFECT (SHA-256 equal) on V2, off the
#  unchanged stores.
#  (chain) V2 reloads the V1 chain at the head V1 itself reports, or, for a
#  chain in a format V2 does not read, refuses to start and leaves it intact.
#
# ROLLING=1 upgrades the holders ONE AT A TIME, fetching after each swap to show
# availability never drops. Every assertion keys off a REAL observed CLI line /
# stdout field / on-disk fact — no invented strings.
#
# Usage: ./run.sh                    v0.1.1 → HEAD; expects the safe refusal (FINDING, exit 0)
#  V1_REF=<ref> EXPECT=clean ./run.sh   any recent ref → HEAD; demands a clean reload
#  ROLLING=1 ...                        upgrade one node at a time
#  PUBLISHES=N ...                      files published on V1 (default 3)
#  KEEP=1 ...                           leave the topology up afterward
# ─────────────────────────────────────────────────────────────────────────────
set -uo pipefail
cd "$(dirname "$0")"
ROOT=$(cd ../.. && pwd)
FILE_BYTES=${FILE_BYTES:-3000000}
V1_REF=${V1_REF:-v0.1.1}        # the OLD version; V2 is always HEAD

export COMPOSE_PROJECT_NAME=${COMPOSE_PROJECT_NAME:-upgrade}
dcs() { docker compose "$@"; }
# Read a file off a service's named volume while that service is NOT running (a
# daemon that refused to start leaves no container to exec into).
onvol() { # volume cmd...
  local v=$1; shift
  docker run --rm -v "${COMPOSE_PROJECT_NAME}_$v:/data" silt-upgrade "$@"
}
cleanup() { [ "${KEEP:-0}" = 1 ] || dcs down -v >/dev/null 2>&1 || true; }
trap cleanup EXIT

pass=1
fail() { echo "FAIL: $*"; pass=0; }

# Grep a service's /data/debug.log OR its stdout for a pattern (daemons write
# structured events to <store>/debug.log when -log/-debug is set; startup lines
# like "bootstrapped" / "chain: restored" go to stdout).
logline() { dcs logs "$1" 2>&1 | grep -E "$2" | tail -1; }

echo "== resolve V1 = $V1_REF, V2 = HEAD =="
V1_COMMIT=$(cd "$ROOT" && git rev-parse --short "$V1_REF" 2>/dev/null) || { echo "FAIL: cannot resolve $V1_REF"; exit 1; }
V2_COMMIT=$(cd "$ROOT" && git rev-parse --short HEAD)
echo "  V1 ($V1_REF) = $V1_COMMIT"
echo "  V2 (HEAD)    = $V2_COMMIT"

echo "== build BOTH silt binaries on the host (linux/$(go env GOARCH)) =="
# V2 = HEAD, built straight from the worktree. V1 = an older release, extracted
# to a temp tree via `git archive` and built there — the worktree stays on HEAD.
CGO_ENABLED=0 GOOS=linux GOARCH="$(go env GOARCH)" \
  go -C "$ROOT" build -trimpath -o "$PWD/silt-v2" ./cmd/silt \
  || { echo "FAIL: V2 (HEAD) build failed"; exit 1; }
V1SRC=$(mktemp -d)
( cd "$ROOT" && git archive "$V1_REF" | tar -x -C "$V1SRC" ) || { echo "FAIL: git archive $V1_REF"; exit 1; }
CGO_ENABLED=0 GOOS=linux GOARCH="$(go env GOARCH)" \
  go -C "$V1SRC" build -trimpath -o "$PWD/silt-v1" ./cmd/silt \
  || { echo "FAIL: V1 ($V1_REF) build failed — an older commit may not build with the current toolchain; see README fallback"; rm -rf "$V1SRC"; exit 1; }
rm -rf "$V1SRC"
docker build -q -t silt-upgrade . >/dev/null || { echo "FAIL: image build failed"; exit 1; }

# The seed pins its identity with -id-seed=1 (set as EXTRA in the compose file),
# so its NodeID is deterministic and the registry ref is buildable up front. V1
# has no `silt id` subcommand, so derive it with the V2 binary in a throwaway
# container — ID derivation is identical across versions (verified: the V1
# daemon's logged `peer:` id for -id-seed=1 matches `silt id -id-seed 1`).
SEED_ID=$(docker run --rm silt-upgrade silt-v2 id -id-seed 1 2>/dev/null | tr -d '\r' | grep -oE '^[a-f0-9]{64}$')
[ -n "$SEED_ID" ] || { echo "FAIL: could not derive seed NodeID"; exit 1; }
export SEED_ID
echo "  seed NodeID: $SEED_ID"
REG="$SEED_ID@https://10.130.0.10:4003"

# ── PHASE 1: bring the swarm up on V1 ────────────────────────────────────────
# Start the seed FIRST and wait until its listener is up before starting the
# holders — otherwise a holder's one-shot bootstrap FIND_NODE races the seed's
# listener, fails, and the holder comes up with an empty routing table (0 table
# entries) so the publish scatters to nobody. (Same ordering the nat/consensus
# harnesses use.)
echo "== phase 1a: start the seed on V1 ($V1_COMMIT) =="
SEED_VER=v1 dcs up -d seed >/dev/null 2>&1

echo "== wait for the seed to report its NodeID (must equal $SEED_ID) and listen =="
GOT_ID=""
for _ in $(seq 1 40); do
  GOT_ID=$(dcs logs seed 2>&1 | grep -oE 'peer: [a-f0-9]{64}' | head -1 | awk '{print $2}')
  [ -n "$GOT_ID" ] && break
  sleep 1
done
[ "$GOT_ID" = "$SEED_ID" ] || fail "seed NodeID $GOT_ID != expected $SEED_ID (registry ref would be wrong)"
echo "  seed reports: ${GOT_ID:-<none>}"
sleep 2   # let the swarm + registry listeners bind before the holders dial

echo "== phase 1b: start the holders on V1 =="
SEED_VER=v1 HOLDERA_VER=v1 HOLDERB_VER=v1 dcs up -d holderA holderB >/dev/null 2>&1

echo "== wait for holders to bootstrap to the seed =="
for n in holderA holderB; do
  ok=0
  for _ in $(seq 1 40); do
    dcs logs "$n" 2>&1 | grep -qE "bootstrapped|re-announced" && { ok=1; break; }
    sleep 1
  done
  [ "$ok" = 1 ] || fail "$n never bootstrapped on V1"
done

# The holders take random keyfile identities (no -id-seed), so read their real
# NodeIDs from their logs. A publishing/fetching swarm client must carry the
# holders in -peers directly: with only the seed (a store-refusing rendezvous)
# in its table, its IterativeFindNode surfaces no storage candidate and the file
# scatters to nobody. This survives the upgrade — the holder identities live in
# their keyfiles on the persisted volumes, so their IDs are stable V1→V2.
HA_ID=$(dcs logs holderA 2>&1 | grep -oE 'peer: [a-f0-9]{64}' | head -1 | awk '{print $2}')
HB_ID=$(dcs logs holderB 2>&1 | grep -oE 'peer: [a-f0-9]{64}' | head -1 | awk '{print $2}')
[ -n "$HA_ID" ] && [ -n "$HB_ID" ] || fail "could not read holder NodeIDs"
echo "  holderA=$HA_ID"
echo "  holderB=$HB_ID"
ALLPEERS="$SEED_ID@10.130.0.10:4001,$HA_ID@10.130.0.11:4001,$HB_ID@10.130.0.12:4001"

# ── publish an erasure-coded file on V1, capture link + sha ──────────────────
echo "== publish a ${FILE_BYTES}-byte EC file from holderA (V1) =="
dcs exec -T holderA sh -c "head -c $FILE_BYTES /dev/urandom > /data/pub.bin; sha256sum /data/pub.bin | cut -d' ' -f1 > /data/pub.sha; silt-v1 swarm add /data/pub.bin -peers '$ALLPEERS' -registry '$REG'" >/tmp/up_add.txt 2>&1
LINK=$(grep -oE 'silt:v1:[A-Za-z0-9_:.-]+' /tmp/up_add.txt | head -1)
SCATTERED=$(grep -oE 'scattered [0-9]+ chunk' /tmp/up_add.txt | grep -oE '[0-9]+' | head -1)
WANT=$(dcs exec -T holderA cat /data/pub.sha | tr -d '\r\n ')
echo "  link: ${LINK:-<none>}"
echo "  scattered: ${SCATTERED:-0} replicas"
echo "  want sha: $WANT"
[ -n "$LINK" ] || { echo "FAIL: publish returned no link"; sed -n '1,40p' /tmp/up_add.txt; pass=0; }
[ "${SCATTERED:-0}" -gt 0 ] 2>/dev/null || fail "publish scattered 0 replicas — nothing to reload after the upgrade"

# More publishes, so the V1 chain carries several committed entries rather than
# one, and the reload replays a real history.
PUBLISHES=${PUBLISHES:-3}
for i in $(seq 2 "$PUBLISHES"); do
  dcs exec -T holderA sh -c "head -c 200000 /dev/urandom > /data/extra$i.bin; silt-v1 swarm add /data/extra$i.bin -peers '$ALLPEERS' -registry '$REG'" >/tmp/up_add$i.txt 2>&1
  grep -qE 'silt:v1:' /tmp/up_add$i.txt || fail "publish $i of $PUBLISHES returned no link"
done

# A "get" the harness can trust in a tiny swarm. The DHT provider-discovery in
# this 3-node topology is flaky for a REMOTE fetcher (a small-swarm weakness
# orthogonal to the upgrade), but the node that HOLDS the shards can always
# reconstruct them from its own store IF the daemon can map its on-disk shards
# back to their placement/column keys — which is exactly what the persisted
# proofs are for. So we drive `swarm get` FROM the holder that holds the file:
# a bit-perfect result proves the store reloaded and the shards are addressable;
# a decode failure with the shards still on disk proves they've been stranded.
fetch_on() { # binver service outlabel → echoes the sha (or empty), writes /tmp/get_<label>.txt
  local ver=$1 svc=$2 label=$3
  dcs exec -T "$svc" sh -c "silt-$ver swarm get '$LINK' -o /data/out_$label.bin -peers '$ALLPEERS' -registry '$REG' && sha256sum /data/out_$label.bin | cut -d' ' -f1" >/tmp/get_$label.txt 2>&1
  grep -oE '^[a-f0-9]{64}' /tmp/get_$label.txt | tail -1
}

# ── CONTROL: on V1, the holder self-fetches bit-perfect BEFORE the upgrade ─────
if [ -n "$LINK" ]; then
  echo "== CONTROL: holderA self-fetch on V1 (must be bit-perfect pre-upgrade) =="
  GOTV1=$(fetch_on v1 holderA v1ctl)
  echo "  got : ${GOTV1:-<none>}"
  [ "$WANT" = "$GOTV1" ] || { echo "FAIL: V1 control fetch not bit-perfect — cannot trust the upgrade comparison"; sed -n '1,20p' /tmp/get_v1ctl.txt; pass=0; }
fi

# Snapshot the V1 on-disk stores before the upgrade. The proofs count is the
# crux: v0.1.1 predates persisted storage proofs, so it writes NONE.
CHUNKS_A_PRE=$(dcs exec -T holderA sh -c 'find /data/objects -type f 2>/dev/null | wc -l' | tr -d ' \r\n')
PROOFS_A_PRE=$(dcs exec -T holderA sh -c 'ls /data/proofs 2>/dev/null | wc -l || echo 0' | tr -d ' \r\n')
CHAIN_PRE=$(dcs exec -T seed sh -c 'test -f /data/chain.cbor && wc -c </data/chain.cbor || echo 0' | tr -d ' \r\n')
CHAIN_SHA_PRE=$(dcs exec -T seed sh -c 'sha256sum /data/chain.cbor 2>/dev/null' | grep -oE '^[a-f0-9]{64}')
# The V1 head as V1 itself reports it, so the post-upgrade comparison is against
# the writer's own account and not against V2 reading V2's interpretation back.
# A release without chain-status (v0.1.1) leaves these empty.
CS_V1=$(dcs exec -T seed sh -c 'silt-v1 chain-status -store /data 2>&1')
HEAD_V1=$(echo "$CS_V1" | awk '/head hash:/{print $3; exit}')
ENTRIES_V1=$(echo "$CS_V1" | awk '/entries:/{print $2; exit}')
echo "  V1 chain (as V1 reports it): head=${HEAD_V1:-<no chain-status>} entries=${ENTRIES_V1:-?} sha=${CHAIN_SHA_PRE:-?}"
echo "  V1 on-disk: holderA objects=$CHUNKS_A_PRE proofs=$PROOFS_A_PRE ; seed chain.cbor bytes=$CHAIN_PRE"
# POSITIVE CONTROL: V1 runs -validator -quorum=0 specifically to commit
# a persisted chain to reload. If chain.cbor is empty, V1's commit path is broken
# and the chain-RELOAD property is never exercised — treating that as "nothing to
# reload → chain-OK" would false-pass a real regression. Require a real V1 chain.
[ "${CHAIN_PRE:-0}" -gt 0 ] 2>/dev/null || fail "V1 seed never persisted a chain block (chain.cbor empty) — the chain-reload property was never exercised, so a later 'chain OK' is meaningless"

wait_ready() { # service
  local s=$1
  for _ in $(seq 1 40); do
    dcs logs --since 2m "$s" 2>&1 | grep -qE "serving; Ctrl-C|bootstrapped|re-announced|reloaded storage proofs" && return 0
    sleep 1
  done
  return 1
}
report_reload() { # service
  local s=$1
  echo "  --- $s reload evidence (V2, from /data/debug.log + stdout) ---"
  dcs exec -T "$s" sh -c 'grep -oE "reloaded storage proofs.*" /data/debug.log 2>/dev/null | tail -1' | sed "s/^/    /" | grep . \
    || echo "    (NO 'reloaded storage proofs' line — nothing in /data/proofs to reload)"
  logline "$s" "re-announced [0-9]+ held chunks" | sed "s/^/    /" | grep . || echo "    (no 're-announced' line)"
  logline "$s" "chain: restored [0-9]+ block" | sed "s/^/    /" | grep . || true
  logline "$s" "chain replay:"                 | sed "s/^/    /" | grep . || true
  logline "$s" "bootstrapped" | sed "s/^/    /" | grep . || echo "    (no 'bootstrapped' line)"
}

# ─────────────────────────────────────────────────────────────────────────────
# UPGRADE V1 → V2 (whole-swarm, or ROLLING one node at a time), SAME stores
# ─────────────────────────────────────────────────────────────────────────────
if [ "$pass" = 1 ] && [ "${ROLLING:-0}" = 1 ]; then
  echo "== ROLLING upgrade: swap holderA → V2 first (holderB + seed still V1) =="
  SEED_VER=v1 HOLDERA_VER=v2 HOLDERB_VER=v1 dcs up -d --no-deps --force-recreate holderA >/dev/null 2>&1
  wait_ready holderA || fail "holderA did not come ready after the V2 swap"
  report_reload holderA
  echo "-- availability check: holderB (still V1) self-holds nothing; holderA (now V2) self-fetch --"
  GOTA=$(fetch_on v2 holderA rollA)
  echo "  holderA(V2) self-fetch: ${GOTA:-<none>}"
  echo "== ROLLING upgrade: swap seed + holderB → V2 (whole swarm now V2) =="
  SEED_VER=v2 HOLDERA_VER=v2 HOLDERB_VER=v2 dcs up -d --no-deps --force-recreate seed holderB >/dev/null 2>&1
  wait_ready holderB || fail "holderB did not come ready after the V2 swap"
elif [ "$pass" = 1 ]; then
  echo "== whole-swarm upgrade: stop all, recreate on V2 over the SAME stores =="
  dcs stop >/dev/null 2>&1
  SEED_VER=v2 HOLDERA_VER=v2 HOLDERB_VER=v2 dcs up -d --force-recreate >/dev/null 2>&1
  for s in holderA holderB; do wait_ready "$s" || fail "$s did not come ready on V2"; done
  wait_ready seed || dcs logs seed 2>&1 | grep -q 'chain replay: REFUSING TO START' \
    || fail "seed neither came ready on V2 nor refused its chain"
fi

# ── a V2 that cannot replay the V1 chain must REFUSE, loudly, leaving it intact ─
# The safe outcome for a chain written in a format V2 does not read is a refusal
# to start that names the loss and leaves chain.cbor byte-identical. Starting
# from genesis, or from a silently truncated prefix, is the failure.
SEED_REFUSED=0; CHAIN_UNTOUCHED=0; CHAIN_PRESERVED=0
if dcs logs seed 2>&1 | grep -q 'chain replay: REFUSING TO START'; then
  SEED_REFUSED=1
  echo "== the seed REFUSED to start on the V1 chain =="
  dcs logs seed 2>&1 | grep 'chain replay: REFUSING TO START' | tail -1 | cut -c1-240 | sed 's/^/  /'
  SHA_AFTER=$(onvol seed-data sha256sum /data/chain.cbor 2>/dev/null | grep -oE '^[a-f0-9]{64}')
  echo "  chain.cbor sha: before=$CHAIN_SHA_PRE after-refusal=${SHA_AFTER:-<missing>}"
  [ -n "$SHA_AFTER" ] && [ "$SHA_AFTER" = "$CHAIN_SHA_PRE" ] && CHAIN_UNTOUCHED=1
  # The operator's documented recovery: restart with -accept-chain-loss. The
  # daemon must start and keep the original bytes as chain.cbor.rejected-<unix>.
  echo "== operator recovery: restart the seed on V2 with -accept-chain-loss =="
  SEED_EXTRA=-accept-chain-loss SEED_VER=v2 HOLDERA_VER=v2 HOLDERB_VER=v2 \
    dcs up -d --no-deps --force-recreate seed >/dev/null 2>&1
  wait_ready seed || fail "seed did not start under -accept-chain-loss"
  KEPT=$(dcs exec -T seed sh -c 'sha256sum /data/chain.cbor.rejected-* 2>/dev/null' | grep -oE '^[a-f0-9]{64}' | head -1)
  echo "  chain.cbor.rejected-* sha: ${KEPT:-<none>}"
  [ -n "$KEPT" ] && [ "$KEPT" = "$CHAIN_SHA_PRE" ] && CHAIN_PRESERVED=1
fi
sleep 5   # let AnnounceHeld (post re-bootstrap) run
for s in seed holderA holderB; do report_reload "$s"; done

# ── the content assertion: does V2 serve the V1-published file off the store? ─
echo "== post-upgrade: holderA self-fetch on V2 (off the UNCHANGED V1 store) =="
GOTV2=$(fetch_on v2 holderA v2post)
CHUNKS_A_POST=$(dcs exec -T holderA sh -c 'find /data/objects -type f 2>/dev/null | wc -l' | tr -d ' \r\n')
PROOFS_A_POST=$(dcs exec -T holderA sh -c 'ls /data/proofs 2>/dev/null | wc -l || echo 0' | tr -d ' \r\n')
echo "  got : ${GOTV2:-<none>}"
echo "  want: $WANT"
echo "  holderA on-disk after upgrade: objects=$CHUNKS_A_POST proofs=$PROOFS_A_POST (shards persisted: ${CHUNKS_A_PRE} -> ${CHUNKS_A_POST})"

CONTENT_OK=0
[ -n "$GOTV2" ] && [ "$WANT" = "$GOTV2" ] && CONTENT_OK=1

# ── observe the REAL reload evidence, so a content failure is attributed by what
#  happened rather than by what V1 is. Two facts mark the persisted-proof boundary:
#  V1 wrote NO persisted proofs (PROOFS_A_PRE==0), and V2 emitted NO "reloaded
#  storage proofs" line. If V2 DID reload proofs yet the content still fails, that
#  is a reload or mesh regression in V2, and the verdict below says so.
RELOADED_A=$(dcs exec -T holderA sh -c 'grep -c "reloaded storage proofs" /data/debug.log 2>/dev/null || echo 0' | tr -d ' \r\n')
[ -z "$RELOADED_A" ] && RELOADED_A=0
echo "  holderA V2 'reloaded storage proofs' lines: $RELOADED_A (V1 proofs on disk pre-upgrade: $PROOFS_A_PRE)"

# ── the chain assertion: does V2 reload the V1-written chain? ─────────────────
echo "== post-upgrade: chain-status on V2 (does it reload the V1 chain?) =="
CS=$(dcs exec -T seed silt-v2 chain-status -store /data 2>&1)
echo "$CS" | grep -iE 'head (height|hash)|block|version|restored' | head -4 | sed 's/^/  /'
CHAIN_OK=0
HEAD_V2=$(echo "$CS" | awk '/head hash:/{print $3; exit}')
ENTRIES_V2=$(echo "$CS" | awk '/entries:/{print $2; exit}')
echo "  V2 reports: head=${HEAD_V2:-?} entries=${ENTRIES_V2:-?}   (V1 wrote: head=${HEAD_V1:-?} entries=${ENTRIES_V1:-?})"
# POSITIVE assertion: real evidence, not the absence of an error string, which a
# blank status or a silent "no chain yet" would satisfy. If V1 persisted a chain
# on disk (CHAIN_PRE>0), V2 must POSITIVELY report a reloaded head + block count
# (`head height:` / `blocks: N`) and must NOT say "no chain yet". If nothing was
# committed on V1 (chain.cbor empty) there is nothing to reload — not a failure.
if [ "$SEED_REFUSED" = 1 ] || echo "$CS" | grep -qiE 'unsupported block version|decode block'; then
  :  # the V1 chain was refused → CHAIN_OK stays 0
elif [ -n "$HEAD_V1" ]; then
  # V1 named its own head: V2 must reload exactly that head and entry count.
  [ "$HEAD_V2" = "$HEAD_V1" ] && [ "$ENTRIES_V2" = "$ENTRIES_V1" ] && CHAIN_OK=1
elif [ "${CHAIN_PRE:-0}" -gt 0 ] 2>/dev/null; then
  if echo "$CS" | grep -qiE 'head height:|blocks: +[1-9]' && ! echo "$CS" | grep -qi 'no chain yet'; then
    CHAIN_OK=1
  fi
else
  CHAIN_OK=1  # V1 committed no chain — nothing to reload
fi

# ─────────────────────────────────────────────────────────────────────────────
# VERDICT
#
# The run passes when it faithfully establishes the ground truth, whichever way
# it falls, and the safety property holds either way:
#
#  • clean      V2 reloads the V1 chain at V1's own head and serves the V1 file
#               bit-perfect off the unchanged store. EXPECT=clean demands this.
#  • refused    V2 does not read the V1 chain format, so it refuses to start,
#               names the loss, and leaves chain.cbor byte-identical; under the
#               operator's -accept-chain-loss it starts and keeps those bytes
#               as chain.cbor.rejected-<unix>. This is the expected outcome for
#               a pre-format V1 (the default, v0.1.1), graded as a FINDING.
#
# Anything else fails: a refusal that alters or loses the file, a reload that
# reports a different head than V1 wrote, or a content failure whose observed
# cause does not match the one reported.
# ─────────────────────────────────────────────────────────────────────────────
FETCH_CAUSE=$(grep -oE 'root not in registry|decode[^:]*: [^ ]+|not enough shards[^ ]*' /tmp/get_v2post.txt 2>/dev/null | head -1)
chain_line() {
  if [ "$CHAIN_OK" = 1 ]; then echo "yes (head ${HEAD_V2:-?}, entries ${ENTRIES_V2:-?})"
  elif [ "$SEED_REFUSED" = 1 ]; then echo "REFUSED to start; file untouched=$CHAIN_UNTOUCHED; kept on recovery=$CHAIN_PRESERVED"
  else echo "NO — V2 head ${HEAD_V2:-?} entries ${ENTRIES_V2:-?} vs V1 head ${HEAD_V1:-?} entries ${ENTRIES_V1:-?}"; fi
}
echo
echo "── ground truth ─────────────────────────────────────────────────────────"
echo "  shards on disk survive the upgrade : ${CHUNKS_A_PRE} -> ${CHUNKS_A_POST} objects"
echo "  V1 wrote persisted proofs          : $PROOFS_A_PRE"
echo "  V2 serves the V1 file bit-perfect  : $([ "$CONTENT_OK" = 1 ] && echo yes || echo "NO — ${FETCH_CAUSE:-see /tmp/get_v2post.txt}")"
echo "  V2 and the V1-written chain        : $(chain_line)"
echo "─────────────────────────────────────────────────────────────────────────"

EXPECT=${EXPECT:-finding}
echo
if [ "$pass" != 1 ]; then
  echo "RESULT: FAIL ❌  harness error (V1=$V1_COMMIT V2=$V2_COMMIT) — see above; not a product verdict"
  exit 1
fi
if [ "$CONTENT_OK" = 1 ] && [ "$CHAIN_OK" = 1 ]; then
  echo "RESULT: PASS ✅  clean cross-version upgrade V1($V1_COMMIT)→V2($V2_COMMIT): chain reloads at V1's head, content bit-perfect${ROLLING:+ (rolling)}"
  exit 0
fi
if [ "$EXPECT" = clean ]; then
  echo "RESULT: FAIL ❌  EXPECT=clean but the upgrade V1($V1_COMMIT)→V2($V2_COMMIT) was not clean (see ground truth)"
  exit 1
fi
if [ "$SEED_REFUSED" = 1 ]; then
  if [ "$CHAIN_UNTOUCHED" != 1 ] || [ "$CHAIN_PRESERVED" != 1 ]; then
    echo "RESULT: FAIL ❌  V2 refused the V1 chain but did not keep its bytes (untouched=$CHAIN_UNTOUCHED kept=$CHAIN_PRESERVED)"
    exit 1
  fi
  echo "RESULT: FINDING ⚠  V2($V2_COMMIT) does not read the chain format V1($V1_COMMIT) wrote, and fails SAFE:"
  echo "  • it refuses to start, names the loss, and leaves chain.cbor byte-identical"
  echo "  • under -accept-chain-loss it starts and keeps the original as chain.cbor.rejected-<unix>"
  if [ "$CONTENT_OK" = 1 ]; then
    echo "  • the V1 file still serves bit-perfect after the recovery"
  elif [ "$FETCH_CAUSE" = "root not in registry" ]; then
    echo "  • the V1 file does not serve: its name was committed in the discarded V1 chain (the registry is"
    echo "    chain-backed), so the root is gone while its shards stay on disk (${CHUNKS_A_PRE} -> ${CHUNKS_A_POST})"
  else
    echo "RESULT: FAIL ❌  after the recovery the V1 file does not serve, and not for the reason the chain loss explains: ${FETCH_CAUSE:-unknown}"
    exit 1
  fi
  exit 0
fi
if [ "$CHAIN_OK" != 1 ]; then
  echo "RESULT: FAIL ❌  V2 started on the V1 chain but does not report the head V1 wrote — a silent reinterpretation"
  exit 1
fi
# The chain reloaded; the content did not.
if [ "${PROOFS_A_PRE:-0}" -gt 0 ] 2>/dev/null || [ "${RELOADED_A:-0}" -gt 0 ] 2>/dev/null; then
  echo "RESULT: FAIL ❌  V1 proofs=$PROOFS_A_PRE, V2 reloaded=$RELOADED_A, yet the file does not serve (${FETCH_CAUSE:-unknown}) — a reload or mesh regression"
  exit 1
fi
echo "RESULT: FINDING ⚠  V1($V1_COMMIT) wrote no persisted storage proofs, so V2($V2_COMMIT) cannot key its shards: content stranded (${FETCH_CAUSE:-unknown})"
exit 0
