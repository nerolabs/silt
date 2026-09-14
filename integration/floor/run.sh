#!/usr/bin/env bash
# ─────────────────────────────────────────────────────────────────────────────
# Field test — A VALIDATOR ON THE DECLARED FLOOR SPEC.
#
# The promise, tested against the kernel rather than against a flag: silt stays
# a full participant on the smallest machine it claims to run on — one core,
# 2 GiB of RAM, 10 GiB of disk. Not "it usually fits", but "the kernel was told
# 2 GiB with no swap, and it did not OOM".
#
# Why a cgroup and not `-mem-limit`: `-mem-limit` is a SOFT ceiling. It sets
# runtime/debug.SetMemoryLimit, so the Go GC reclaims harder as the heap climbs,
# and a live set that genuinely exceeds it thrashes rather than crashes. That is
# the right production behavior and the wrong instrument — a soft ceiling cannot
# tell you whether the node survives on a 2 GiB box, because nothing enforces
# the 2 GiB. The container's cgroup does: `mem_limit` == `memswap_limit` means
# no swap at all, so crossing the ceiling is a clean OOM-kill (exit 137) that
# this harness can read, and `memory.peak` gives the high-water mark directly
# instead of a sampled RSS that can miss the spike that mattered.
#
# `cpuset` and not a CPU quota, for the same reason: a quota still reports every
# host core to `nproc`, so the Go runtime sizes GOMAXPROCS and its worker pools
# for a multi-core box. The node under test would not be the node we ship.
#
# WHAT THIS SUITE DOES NOT YET SHOW, stated here so a green run is not read as
# more than it is: the load below is HONEST consensus traffic. The claim this
# suite serves says "under its memory ceiling on ADVERSARIAL input", and that
# wants the redteam and sybil topologies re-pointed at a floor-spec seat. The
# ceiling leg is evidence for the honest case only.
#
# WHAT THIS SUITE CANNOT DRIVE, and says so rather than skipping:
#  The floor box's OTHER half — validating against WITNESSES without holding
#  the tree, and stalling rather than accepting when no witness provider is
#  reachable — has no process to run. `chain.NewBox` has no caller outside
#  core/chain's own tests, nothing implements `chain.WitnessSource` in
#  production, and no daemon flag puts a node in that mode. Those two legs are
#  reported UNWIRED. Unwired is not skipped and not green: it is the suite
#  saying the product does not expose the thing the claim is about. The legs
#  below are the half that a binary can be made to do today.
#
# The legs:
#  1. SPEC BINDS (the vacuity guard) — read memory.max, memory.swap.max and
#     nproc from INSIDE the floor container. If the limits did not bind, every
#     other leg is measuring an ordinary box and means nothing. This leg is why
#     a green run is evidence.
#  2. VALIDATES — the floor box commits, and converges on the same head hash as
#     the two ordinary validators. A box that stays under its ceiling by not
#     participating has not passed anything.
#  3. MEMORY CEILING — memory.peak stays under the 2 GiB the kernel enforced,
#     and the container was never OOM-killed. Reported as a real number so a
#     regression shows as a shrinking margin, not only as a failure. Honest
#     load only — see the note above.
#  4. PRUNES AT DEPTH — `chain-status` reports blocks that have shed their heavy
#     bond proofs below the retention horizon. The floor box is the one that has
#     to prune: full history is what OOMs a small machine.
#  5. FROM PERSISTED STATE — restart the floor box onto the SAME volume. It must
#     reload the already-pruned store, still report the shed, and commit again.
#     Without this leg the prune could be a live in-memory view that a restart
#     silently undoes.
#
# The prune arithmetic this suite must clear (core/chain/retention.go): the node
# retains max(2·BondTTL, BondRegHeadWindow+4) full-proof blocks below its
# FINALIZED head, then epoch-aligns down. BondRegHeadWindow is 8 and is not
# operator-settable, so the guard is 12 whatever BondTTL is set to — the chain
# must finalize past height 12 before ANY block is prune-eligible. That is why
# this suite runs longer than its siblings, and why TARGET_HEIGHT defaults above
# the guard rather than to the first height that commits.
#
# Usage:
#   ./run.sh                      # the declared floor spec
#   TARGET_HEIGHT=20 ./run.sh     # drive deeper before asserting the shed
#   FLOOR_MEM=1g ./run.sh         # probe a tighter box than the declared floor
#   KEEP=1 ./run.sh               # leave the topology up for inspection
# exit 0 = PASS / a reported FINDING; non-zero = FAIL
# ─────────────────────────────────────────────────────────────────────────────
set -uo pipefail
cd "$(dirname "$0")"
ROOT=$(cd ../.. && pwd)
# shellcheck source=../lib.sh
. "$ROOT/integration/lib.sh"

# The declared floor spec. These are the numbers the product claims, so they are
# the defaults; the env knobs exist to probe tighter, never to quietly loosen a
# failing run.
FLOOR_CPUS=${FLOOR_CPUS:-1}
FLOOR_MEM=${FLOOR_MEM:-2g}
FLOOR_DISK_GIB=${FLOOR_DISK_GIB:-10}
TARGET_HEIGHT=${TARGET_HEIGHT:-17}     # above the 12-block prune guard, epoch-aligned headroom
EPOCH_BLOCKS=${EPOCH_BLOCKS:-4}
BOND_TTL=${BOND_TTL:-2}
HEIGHT_BUDGET=${HEIGHT_BUDGET:-420}    # seconds to reach TARGET_HEIGHT
PRUNE_BUDGET=${PRUNE_BUDGET:-180}      # seconds, after the height, to observe the shed
RESTART_BUDGET=${RESTART_BUDGET:-120}  # seconds for the restarted box to re-commit
export EPOCH_BLOCKS BOND_TTL

PROJECT=floor
dc() { docker compose -p "$PROJECT" "$@"; }
# Teardown is ft_sweep (integration/lib.sh): it removes everything this suite
# built and REPORTS what it could not remove, rather than failing quietly. A
# sweep that silently failed is how the next run inherits a dirty box.
cleanup() {
  [ "${KEEP:-0}" = 1 ] && { echo "KEEP=1 — topology left up; tear it down with: docker compose -p ${PROJECT} down -v"; return 0; }
  ft_sweep "$PROJECT"
}
trap cleanup EXIT INT TERM
fail() { echo "RESULT: FAIL ❌  $*"; exit 1; }

# ---- oracles ---------------------------------------------------------------
# Every assertion below reads a real CLI line, a real cgroup file, or a real
# container exit code. None of them reads this harness's own bookkeeping.

head_height() { dc exec -T "$1" silt chain-status -store /data 2>/dev/null | awk '/head height/{print $3}' | tr -d '\r'; }
head_hash()   { dc exec -T "$1" silt chain-status -store /data 2>/dev/null | awk '/head hash/{print $3}'   | tr -d '\r'; }

# The shed counter. chain-status prints it as
#   "pruned:  N blocks have shed their heavy bond proofs below the retention horizon"
# Take the LAST match: the word "pruned" appears in that line's prose too, and a
# future line could add another. Empty output becomes 0, never a bare string
# that would compare as greater than anything in a numeric test.
pruned_count() {
  local n
  n=$(dc exec -T "$1" silt chain-status -store /data 2>/dev/null \
        | grep -oE 'pruned:[[:space:]]*[0-9]+' | tail -1 | grep -oE '[0-9]+')
  echo "${n:-0}"
}

# cgroup facts, read from inside the container under test.
cg() { dc exec -T floor sh -c "cat /sys/fs/cgroup/$1 2>/dev/null" | tr -d '\r\n'; }
floor_nproc() { dc exec -T floor sh -c 'nproc' | tr -d '\r\n'; }
floor_store_bytes() { dc exec -T floor sh -c 'du -sb /data 2>/dev/null | cut -f1' | tr -d '\r\n'; }

# Container exit code, and whether the kernel OOM-killed it. 137 is SIGKILL,
# which under a memory cgroup with no swap is the OOM-killer and not a timeout.
floor_exit_code() { docker inspect -f '{{.State.ExitCode}}' "$(dc ps -aq floor)" 2>/dev/null | tr -d '\r\n'; }
floor_oom_killed() { docker inspect -f '{{.State.OOMKilled}}' "$(dc ps -aq floor)" 2>/dev/null | tr -d '\r\n'; }

# Block until a service's head height reaches N, printing progress so a slow run
# is legible rather than silent. Echoes the height it actually reached.
await_height() { # await_height <svc> <target> <budget-seconds>
  local svc="$1" target="$2" budget="$3" h="" waited=0
  while [ "$waited" -lt "$budget" ]; do
    h=$(head_height "$svc")
    case "$h" in ''|*[!0-9]*) h=0 ;; esac
    if [ "$h" -ge "$target" ]; then echo "$h"; return 0; fi
    # A dead box will never climb. Fail fast instead of burning the budget.
    if [ "$(dc ps -q floor | wc -l | tr -d ' ')" = "0" ]; then echo "$h"; return 1; fi
    sleep 5; waited=$((waited + 5))
    [ $((waited % 30)) -eq 0 ] && echo "    ${svc} height=${h} (${waited}s/${budget}s)"
  done
  echo "$h"; return 1
}

# Byte formatting and IEC parsing in pure shell: `numfmt` is GNU coreutils and is
# NOT on a stock macOS, so depending on it would make this suite unrunnable from
# a clean clone on a developer laptop.
human_bytes() { # human_bytes <bytes>
  local b="$1"
  case "$b" in ''|*[!0-9]*) echo "?"; return ;; esac
  if   [ "$b" -ge 1073741824 ]; then awk -v b="$b" 'BEGIN{printf "%.2f GiB", b/1073741824}'
  elif [ "$b" -ge 1048576 ];    then awk -v b="$b" 'BEGIN{printf "%.1f MiB", b/1048576}'
  elif [ "$b" -ge 1024 ];       then awk -v b="$b" 'BEGIN{printf "%.1f KiB", b/1024}'
  else echo "${b}B"; fi
}

# iec_bytes parses the same suffixes docker's mem_limit accepts (b/k/m/g, case
# insensitive, bare number = bytes) so leg 1 can compare what we ASKED the
# kernel for against what it reports, instead of hardcoding the default.
iec_bytes() { # iec_bytes <2g|1500m|…>
  local v="$1" n u
  n=$(printf '%s' "$v" | tr -d '[:alpha:]')
  u=$(printf '%s' "$v" | tr -d '[:digit:]' | tr '[:upper:]' '[:lower:]')
  case "$n" in ''|*[!0-9]*) echo 0; return 1 ;; esac
  case "$u" in
    g|gb|gi|gib) echo $(( n * 1073741824 )) ;;
    m|mb|mi|mib) echo $(( n * 1048576 )) ;;
    k|kb|ki|kib) echo $(( n * 1024 )) ;;
    ''|b)        echo "$n" ;;
    *)           echo 0; return 1 ;;
  esac
}

# ---- prereqs ---------------------------------------------------------------
ft_require docker go awk || exit 2
ft_docker_up || fail "no reachable docker daemon"

echo "silt field test — a validator on the declared floor spec"
echo "  floor spec: ${FLOOR_CPUS} core / ${FLOOR_MEM} RAM / ${FLOOR_DISK_GIB} GiB disk"
echo

# This suite measures a memory ceiling, so a dirty box is not noise — it is the
# measurement. Anything else holding memory on this host shows up as the floor
# box's headroom.
ft_preflight "$PROJECT" || exit 2
echo

# ---- build -----------------------------------------------------------------
echo "== phase 0: build =="
CGO_ENABLED=0 GOOS=linux GOARCH="$(go env GOARCH)" go build -trimpath -o silt "$ROOT/cmd/silt" || fail "host build"
docker build -q -t silt-floor . >/dev/null || fail "image build"
echo "  built $(human_bytes "$(stat -f%z silt 2>/dev/null || stat -c%s silt)")"

# ---- topology --------------------------------------------------------------
echo "== phase 1: the anchor set, resolved before launch =="
# The anchor set must be identical in every node's genesis, so it has to be
# known BEFORE any daemon starts — and a daemon cannot report an ID it has not
# been given a genesis for. `silt id -id-seed` derives the NodeID from the seed
# alone, with no store and no network, which is what makes the set fillable up
# front. The binary is a linux image, so run it in a throwaway container.
silt_id() { docker run --rm silt-floor silt id -id-seed "$1" | tr -d '\r'; }
ID_A=$(silt_id 9101); ID_B=$(silt_id 9102); ID_F=$(silt_id 9103)
for pair in "valA:$ID_A" "valB:$ID_B" "floor:$ID_F"; do
  case "${pair#*:}" in
    "" ) fail "could not derive the NodeID for ${pair%%:*} — \`silt id\` returned nothing" ;;
  esac
done
export ID_A ID_B ID_F
export ANCHORS="$ID_A,$ID_B,$ID_F"
export ATTESTERS="$ID_B,$ID_F"
export PEER_A="$ID_A@10.140.0.11:4001" REG_A="$ID_A@https://10.140.0.11:4003"
# At genesis there is no chain to discover peer ADDRESSES through (the routing
# table holds bare NodeIDs), so proposer-initiated quorum needs the whole set
# configured up front on every seat, not just a path back to valA.
export PEERS_ALL="$PEER_A,$ID_B@10.140.0.12:4001,$ID_F@10.140.0.13:4001"
echo "  valA  = ${ID_A:0:16}…"
echo "  valB  = ${ID_B:0:16}…"
echo "  floor = ${ID_F:0:16}…  (the box under test)"

echo "== phase 2: the topology, one genesis =="
dc up -d valA valB floor >/dev/null 2>&1 || fail "up the topology"
sleep 8
for svc in valA valB floor; do
  [ "$(dc ps -q "$svc" | wc -l | tr -d ' ')" != "0" ] \
    || fail "$svc exited during startup — $(dc logs "$svc" 2>&1 | tail -5)"
done
echo "  three validators up, anchored on one set"

# ---- leg 1: the spec actually binds ----------------------------------------
# THE VACUITY GUARD. Everything after this is only evidence if the kernel really
# enforced the floor spec. A compose file that silently dropped mem_limit — an
# older schema, a swarm-mode deploy block, a daemon without cgroup v2 — would
# otherwise produce a confident green from an ordinary box.
echo "== leg 1: the floor spec binds in the kernel =="
WANT_MEM=$(iec_bytes "$FLOOR_MEM") || fail "FLOOR_MEM='${FLOOR_MEM}' is not a size docker would accept (try 2g, 1500m)"
GOT_MEM=$(cg memory.max)
GOT_SWAP=$(cg memory.swap.max)
GOT_CPUS=$(floor_nproc)

echo "  memory.max      = ${GOT_MEM:-<unset>} (want ${WANT_MEM})"
echo "  memory.swap.max = ${GOT_SWAP:-<unset>} (want 0 — no swap to escape into)"
echo "  nproc           = ${GOT_CPUS:-<unset>} (want ${FLOOR_CPUS})"

[ "$GOT_MEM" = "$WANT_MEM" ] || fail "the memory ceiling did NOT bind: memory.max is '${GOT_MEM:-unset}', not ${WANT_MEM}. Every later leg would be measuring an unconstrained box."
[ "$GOT_SWAP" = "0" ] || fail "the container has swap (memory.swap.max='${GOT_SWAP:-unset}'): the box can exceed its RAM ceiling by thrashing, so 'stayed under 2 GiB' would not mean what it says."
[ "$GOT_CPUS" = "$FLOOR_CPUS" ] || fail "nproc is '${GOT_CPUS:-unset}', not ${FLOOR_CPUS}: the Go runtime is sizing for a wider box than the floor spec, so this is not the node we ship."
echo "  ✓ the kernel is enforcing the floor spec"

# ---- leg 2: it validates ---------------------------------------------------
echo "== leg 2: the floor box validates, to height ${TARGET_HEIGHT} =="
echo "  (the prune guard is 12 blocks below the finalized head, so the shed cannot"
echo "   appear before the chain finalizes past it — this leg is the long one)"
REACHED=$(await_height floor "$TARGET_HEIGHT" "$HEIGHT_BUDGET")
HEIGHT_OK=$?
HF=$(head_height floor); HA=$(head_height valA); HB=$(head_height valB)
XF=$(head_hash floor);   XA=$(head_hash valA)
echo "  heights — floor=${HF:-?} valA=${HA:-?} valB=${HB:-?}"
echo "  head    — floor=${XF:0:16}… valA=${XA:0:16}…"

if [ "$HEIGHT_OK" -ne 0 ]; then
  echo "  the floor box reached height ${REACHED:-0} of ${TARGET_HEIGHT} in ${HEIGHT_BUDGET}s"
  # Distinguish a dead box from a slow one: they are different failures.
  if [ "$(dc ps -q floor | wc -l | tr -d ' ')" = "0" ]; then
    EC=$(floor_exit_code); OOM=$(floor_oom_killed)
    [ "$OOM" = "true" ] && fail "the floor box was OOM-KILLED at ${FLOOR_MEM} (exit ${EC}). silt does not fit the machine it claims to run on."
    fail "the floor box died (exit ${EC}, OOMKilled=${OOM}) — $(dc logs floor 2>&1 | tail -5)"
  fi
  fail "the floor box is alive but did not reach height ${TARGET_HEIGHT} within ${HEIGHT_BUDGET}s (reached ${REACHED:-0}). Raise HEIGHT_BUDGET only after confirming the chain is actually advancing."
fi

[ -n "$XF" ] && [ "$XF" = "$XA" ] || fail "the floor box is on a DIFFERENT head than valA (floor=${XF:-none} valA=${XA:-none}): it is not validating the same chain, so staying under its ceiling proves nothing."
echo "  ✓ committed to height ${HF} on the same head as the ordinary validators"

# ---- leg 3: the memory ceiling ---------------------------------------------
# memory.peak is the cgroup's own high-water mark since the container started.
# It is the whole run's maximum, not a sample, so a spike between two polls
# cannot hide in it.
echo "== leg 3: under the memory ceiling, on honest load =="
PEAK=$(cg memory.peak)
case "$PEAK" in ''|*[!0-9]*) fail "could not read memory.peak from the floor container — the ceiling leg has no measurement, which is a failure and not a pass" ;; esac
MARGIN=$(( WANT_MEM - PEAK ))
PCT=$(( PEAK * 100 / WANT_MEM ))
echo "  memory.peak = $(human_bytes "$PEAK") of $(human_bytes "$WANT_MEM") (${PCT}%, $(human_bytes "$MARGIN") of headroom)"
[ "$(floor_oom_killed)" != "true" ] || fail "the floor box was OOM-killed during the run"
[ "$PEAK" -lt "$WANT_MEM" ] || fail "memory.peak ($PEAK) reached the ceiling ($WANT_MEM)"

STORE=$(floor_store_bytes)
DISK_CAP=$(( FLOOR_DISK_GIB * 1024 * 1024 * 1024 ))
case "$STORE" in ''|*[!0-9]*) STORE=0 ;; esac
echo "  store       = $(human_bytes "$STORE") of the ${FLOOR_DISK_GIB} GiB the floor spec allows"
# The disk figure is MEASURED, not kernel-enforced: compose has no portable disk
# quota, so unlike memory this one is an observation. Said plainly so nobody
# reads it as the same class of evidence as leg 1.
[ "$STORE" -lt "$DISK_CAP" ] || fail "the store is $(human_bytes "$STORE"), past the ${FLOOR_DISK_GIB} GiB floor-spec disk budget"
echo "  ✓ under the ceiling, with the disk budget measured (not cgroup-enforced)"

# ---- leg 4: it prunes at depth ---------------------------------------------
echo "== leg 4: the heavy bond proofs are shed below the retention horizon =="
SHED=0; waited=0
while [ "$waited" -lt "$PRUNE_BUDGET" ]; do
  SHED=$(pruned_count floor)
  [ "$SHED" -ge 1 ] && break
  sleep 5; waited=$((waited + 5))
  [ $((waited % 30)) -eq 0 ] && echo "    pruned=${SHED} height=$(head_height floor) (${waited}s/${PRUNE_BUDGET}s)"
done
HF=$(head_height floor)
echo "  pruned=${SHED} at height ${HF}"
[ "$SHED" -ge 1 ] || fail "the floor box shed NOTHING by height ${HF} (prune guard is 12 below the finalized head). Full history is exactly what OOMs a small box, so an unpruned floor box is the defect this leg exists to catch."
echo "  ✓ ${SHED} block(s) have shed their heavy bond proofs"

# ---- leg 5: and it is a property of persisted state -------------------------
# Restart onto the SAME named volume. A prune that only ever lived in the
# running process's memory would come back unpruned here, and a box that cannot
# restart from its own pruned store is worse than one that never pruned.
echo "== leg 5: the shed survives a restart, from the persisted store =="
PRE_PEAK=$PEAK
dc restart floor >/dev/null 2>&1 || fail "could not restart the floor box"
sleep 10
[ "$(dc ps -q floor | wc -l | tr -d ' ')" != "0" ] || fail "the floor box did not come back up from its pruned store (exit $(floor_exit_code)) — $(dc logs floor 2>&1 | tail -10)"

SHED2=$(pruned_count floor)
H_AFTER=$(head_height floor)
echo "  after restart: pruned=${SHED2} height=${H_AFTER:-?}"
[ "$SHED2" -ge 1 ] || fail "the shed did not survive the restart (pruned=${SHED2}): the prune was a live view, not a property of the persisted store."

# It must not merely survive — it must keep working. A box that reloads a pruned
# store and then cannot commit has traded the OOM for a stall.
TARGET2=$(( H_AFTER + 1 ))
REACHED2=$(await_height floor "$TARGET2" "$RESTART_BUDGET")
if [ "$?" -ne 0 ]; then
  fail "the restarted floor box reloaded its pruned store but did not commit again within ${RESTART_BUDGET}s (stuck at ${REACHED2:-?}): the prune traded an OOM for a stall."
fi
POST_PEAK=$(cg memory.peak)
echo "  committed to ${REACHED2} from the pruned store; peak since restart $(human_bytes "${POST_PEAK:-0}")"
echo "  ✓ the shed is a property of persisted state, and the box still commits"

# ---- the legs no binary can run --------------------------------------------
# Reported, not skipped. The distinction matters: a skip says "we chose not to
# run this", and what is true here is "there is nothing to run".
echo
echo "== unwired: the witness half of the floor box =="
echo "  NOT RUN, and not skippable — no process exists to drive:"
echo "    • validates against witnesses without holding the tree"
echo "    • stalls rather than accepts when no witness provider is reachable"
echo "  chain.NewBox has no caller outside core/chain's own tests, nothing"
echo "  implements chain.WitnessSource in production, and no daemon flag puts a"
echo "  node in that mode. The mechanism is gated at the unit tier; it has no"
echo "  transport and no entry point, so there is no e2e to drive. These legs"
echo "  turn green when a witness server and a floor-box daemon mode exist."

# ---- verdict ---------------------------------------------------------------
echo
echo "  floor spec enforced : ${FLOOR_CPUS} core / $(human_bytes "$WANT_MEM") / no swap"
echo "  peak memory         : $(human_bytes "$PRE_PEAK") (${PCT}% of the ceiling)"
echo "  height / shed       : ${HF} / ${SHED} blocks"
echo "  restart             : reloaded the pruned store, committed to ${REACHED2}"
echo
echo "  not shown here       : the ceiling under ADVERSARIAL input (this load is honest)"
echo "  unwired              : validates against witnesses; stalls with no provider"
echo
echo "RESULT: FINDING ⚠  the floor box validates, stays under a kernel-enforced 2 GiB at ${PCT}% peak on HONEST load, prunes at depth, and restarts from the pruned store — but the ceiling is not yet driven on adversarial input, and the witness half of the claim has no process to run and is reported UNWIRED, so the floor-box claim is not wholly demonstrated"
exit 0
