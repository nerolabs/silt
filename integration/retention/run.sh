#!/usr/bin/env bash
# ─────────────────────────────────────────────────────────────────────────────
# Field test — RETENTION DECAY: standing that is not re-proven is taken away.
#
# The claim, in one line: a validator that registers a bond ONCE and then stops
# re-proving loses its standing, so a single proof cannot be coasted on forever.
# This is one of the economies of scale a Sybil farm relies on — buy one proof,
# vote with it indefinitely — and the denial is the objective re-challenge TTL:
# standing lapses `-bond-ttl` committed blocks after a validator's latest
# on-chain bond registration unless it renews with a fresh space-time proof.
#
# WHAT THIS SUITE MEASURES THAT THE UNIT TIER CANNOT. The unit tier drives the
# sweep directly against a chain it constructs. Here the registrations are real
# ones that travelled over TCP and committed in real blocks, the renewals are
# the daemon's own (SubmitBondRenewal on each chain-sync sweep, plus the
# proposer's renewal as it proposes), and the identity that stops re-proving
# stops because its PROCESS IS GONE — which is what releasing a plot looks like
# from the network's side.
#
# THE OBSERVABLE is the daemon's own committed-state line, printed on every
# commit and registered in cmd/silt/observable_contract.go:
#
#   chain: saved N block(s) [commit] head=H:… (… bonded=B …)
#
# `bonded` is the size of the COMMITTED bonded set — the set the TTL sweep
# prunes. It is read from v1, which never leaves, so every number here comes
# from a node that saw the whole run. No assertion reads the harness's own
# bookkeeping, and none reads the stopped node's opinion of itself.
#
# THE ARM DOES NOT PASS VACUOUSLY, and the control is on its own axis. A
# falling counter means nothing on its own: it would fall just as far if the
# swarm were dying, or if standing simply decayed with time regardless of
# renewal. So leg 2 runs the SAME window with the attack removed — nobody
# stopped — and requires the counter to hold at four. Only then does leg 3's
# fall attribute to the one thing that changed: v4 stopped re-proving.
#
# The legs:
#  1. THE SET FILLS — all four validators commit real bond registrations and
#     the committed bonded set reaches 4. Standing is real before anything is
#     taken away; without this the rest measures an empty set.
#  2. CONTROL: NO DECAY WITHOUT A STOP — with all four alive, the chain
#     advances a full TTL window and more, and `bonded` never leaves 4. This is
#     the ablation: remove exactly the attack and the denial stops firing.
#  3. DECAY — v4 is stopped, so it can no longer renew. Within the TTL the
#     committed bonded set drops to 3, ON A CHAIN THAT IS STILL COMMITTING.
#     Both halves are asserted: a stalled chain with a frozen counter is not a
#     denial, it is a dead swarm.
#  4. RE-EARNED, NOT RESTORED — v4 comes back onto its OWN store, with the same
#     identity and the same plot, and the set returns to 4. Standing came back
#     because a FRESH registration committed, which is the other half of "one
#     proof does not last forever": the proof is what buys the standing, every
#     time. A node that could coast would never have needed this leg.
#
# THE ARM IS NOT VACUOUS, and that is DRIVEN rather than argued. Remove exactly
# the denial and leg 3 goes red:
#
#   BOND_TTL=0 ./run.sh          # the shipped disable switch: standing never expires
#
# `-bond-ttl=0` is the daemon's own documented opt-out, so the ablation changes
# a configuration this product ships rather than a line of test scaffolding. Legs
# 1 and 2 still pass under it — four bonds commit, and nothing decays while all
# four are alive — and leg 3 fails with "the committed bonded set is STILL 4 …
# An identity is coasting on a proof it can no longer answer for", on a chain
# that is still advancing. That last clause is what makes the red mean something:
# the liveness assertion in leg 3 runs first, so an ablation that merely killed
# the swarm would fail for a different, clearly-named reason.
#
# WHY THIS TOPOLOGY AND NOT AN EXISTING SUITE. Three sibling suites were
# checked first and none can carry the arm:
#   • `bond` sets -bond-ttl=0 (expiry off, deliberately — its own arms need
#     standing to hold still) and never advances a chain for a bond to age
#     against.
#   • `sybil` leaves the TTL at the derived default of 32 blocks while its
#     chain reaches a height in the low single digits, so the window is
#     unreachable there; and its standing gates are the property IT tests.
#   • `floor` runs three validators, where the Byzantine support set is all
#     three — removing one stops the chain, and the sweep that would evict the
#     removed node rides on the very blocks the removal prevents. At three
#     validators the mechanism cannot be observed by removal at all. That is
#     the reason this topology has four; the compose file carries the
#     arithmetic.
#
# Usage:
#   ./run.sh                    # the declared defaults
#   BOND_TTL=8 ./run.sh         # a longer re-challenge window
#   KEEP=1 ./run.sh             # leave the topology up for inspection
# exit 0 = PASS; non-zero = FAIL
# ─────────────────────────────────────────────────────────────────────────────
set -uo pipefail
cd "$(dirname "$0")"
ROOT=$(cd ../.. && pwd)
# shellcheck source=../lib.sh
. "$ROOT/integration/lib.sh"

# The re-challenge cadence, in committed blocks. It is consensus-critical and
# genesis-bound, so every node gets the same value from this one variable.
# It has to clear a live validator's renewal margin: an honest node renews on
# every chain-sync sweep, so it gets several inclusion chances per window, and
# raising this only makes the suite slower and safer. Lowering it below a live
# node's renewal cadence would evict the honest nodes too — which leg 2 exists
# to catch, rather than to tolerate.
BOND_TTL=${BOND_TTL:-4}
BOND_SIZE=${BOND_SIZE:-8M}
export BOND_TTL BOND_SIZE

VALIDATORS=4                                  # the topology's size; see the compose file
FILL_BUDGET=${FILL_BUDGET:-420}               # seconds for all four bonds to commit
CONTROL_BLOCKS=${CONTROL_BLOCKS:-$((BOND_TTL + 2))}   # the no-stop window, in blocks
CONTROL_BUDGET=${CONTROL_BUDGET:-420}         # seconds for that window to pass
DECAY_BUDGET=${DECAY_BUDGET:-420}             # seconds for the stopped node to lapse
REJOIN_BUDGET=${REJOIN_BUDGET:-300}           # seconds for it to re-earn standing

PROJECT=retention
dc() { docker compose -p "$PROJECT" "$@"; }
cleanup() {
  [ "${KEEP:-0}" = 1 ] && { echo "KEEP=1 — topology left up; tear it down with: docker compose -p ${PROJECT} down -v"; return 0; }
  ft_sweep "$PROJECT"
  rm -f "$(dirname "$0")/silt"
}
trap cleanup EXIT INT TERM
fail() { echo "RESULT: FAIL ❌  $*"; exit 1; }

# ---- oracles ---------------------------------------------------------------
# Every assertion reads a real daemon line or a real CLI line.

# The COMMITTED bonded set size, from the node's own commit-time state line.
# Empty when the node has not committed yet; callers treat empty as "no
# measurement", which is a failure and not a pass.
bonded_now() { # bonded_now <svc>
  dc logs "$1" 2>/dev/null | grep -oE 'chain: saved .*bonded=[0-9]+' \
    | grep -oE 'bonded=[0-9]+$' | tail -1 | cut -d= -f2
}

# The LOWEST bonded value this node has reported since the set first reached
# `full`. A transient dip that healed would be invisible to a single reading at
# the end of a window, and a control that cannot see a dip is not a control.
min_bonded_since_full() { # min_bonded_since_full <svc> <full>
  dc logs "$1" 2>/dev/null \
    | grep -oE 'chain: saved .*bonded=[0-9]+' \
    | grep -oE 'bonded=[0-9]+$' | cut -d= -f2 \
    | awk -v full="$2" 'BEGIN{seen=0; min=-1} { if ($1>=full) seen=1; if (seen && (min<0 || $1<min)) min=$1 } END{print min}'
}

head_height() { dc exec -T "$1" silt chain-status -store /data 2>/dev/null | awk '/head height/{print $3}' | tr -d '\r'; }

# Block until a service's committed bonded set reaches N, printing progress so a
# slow run is legible rather than silent. Echoes the value it actually reached.
# Progress goes to STDERR: this function's STDOUT is its return value.
await_bonded() { # await_bonded <svc> <target> <budget-seconds>
  local svc="$1" target="$2" budget="$3" b="" waited=0
  while :; do
    b=$(bonded_now "$svc")
    case "$b" in ''|*[!0-9]*) b=0 ;; esac
    if [ "$b" -ge "$target" ]; then echo "$b"; return 0; fi
    [ "$waited" -ge "$budget" ] && break
    sleep 5; waited=$((waited + 5))
    [ $((waited % 30)) -eq 0 ] && echo "    ${svc} bonded=${b} (${waited}s/${budget}s)" >&2
  done
  echo "$b"; return 1
}

# Block until the committed bonded set FALLS to N or below.
await_bonded_down_to() { # await_bonded_down_to <svc> <target> <budget-seconds>
  local svc="$1" target="$2" budget="$3" b="" waited=0
  while :; do
    b=$(bonded_now "$svc")
    case "$b" in ''|*[!0-9]*) b=99 ;; esac
    if [ "$b" -le "$target" ]; then echo "$b"; return 0; fi
    [ "$waited" -ge "$budget" ] && break
    sleep 5; waited=$((waited + 5))
    [ $((waited % 30)) -eq 0 ] && echo "    ${svc} bonded=${b} height=$(head_height "$svc") (${waited}s/${budget}s)" >&2
  done
  echo "$b"; return 1
}

# Block until a service's head height reaches N. Echoes the height reached.
await_height() { # await_height <svc> <target> <budget-seconds>
  local svc="$1" target="$2" budget="$3" h="" waited=0
  while :; do
    h=$(head_height "$svc")
    case "$h" in ''|*[!0-9]*) h=0 ;; esac
    if [ "$h" -ge "$target" ]; then echo "$h"; return 0; fi
    [ "$waited" -ge "$budget" ] && break
    sleep 5; waited=$((waited + 5))
    [ $((waited % 30)) -eq 0 ] && echo "    ${svc} height=${h} bonded=$(bonded_now "$svc") (${waited}s/${budget}s)" >&2
  done
  echo "$h"; return 1
}

# ---- prereqs ---------------------------------------------------------------
ft_require docker go awk || exit 2
ft_docker_up || fail "no reachable docker daemon"

echo "silt field test — retention decay: standing not re-proven is taken away"
echo "  ${VALIDATORS} bonded validators, re-challenge TTL ${BOND_TTL} blocks, bond ${BOND_SIZE}"
echo

ft_preflight "$PROJECT" || exit 2
echo

# ---- build -----------------------------------------------------------------
echo "== phase 0: build =="
CGO_ENABLED=0 GOOS=linux GOARCH="$(go env GOARCH)" go build -trimpath -o silt "$ROOT/cmd/silt" || fail "host build"
docker build -q -t silt-retention . >/dev/null || fail "image build"
echo "  built"

# ---- topology --------------------------------------------------------------
echo "== phase 1: the anchor set, resolved before launch =="
# The anchor set must be identical in every node's genesis, so it has to be
# known BEFORE any daemon starts. `silt id -id-seed` derives the NodeID from the
# seed alone, with no store and no network. The binary is a linux image, so run
# it in a throwaway container.
silt_id() { docker run --rm silt-retention silt id -id-seed "$1" | tr -d '\r'; }
ID1=$(silt_id 7101); ID2=$(silt_id 7102); ID3=$(silt_id 7103); ID4=$(silt_id 7104)
for v in ID1 ID2 ID3 ID4; do
  [ -n "${!v}" ] || fail "could not derive $v"
  printf "  %-4s %s\n" "$v" "${!v}"
done
export ANCHORS="$ID1,$ID2,$ID3,$ID4"
export ATTESTERS_1="$ID2,$ID3,$ID4"
export ATTESTERS_2="$ID1,$ID3,$ID4"
export ATTESTERS_3="$ID1,$ID2,$ID4"
export ATTESTERS_4="$ID1,$ID2,$ID3"
export PEER_1="$ID1@10.160.0.11:4001"
export REG_1="$ID1@https://10.160.0.11:4003"
export PEERS_ALL="$PEER_1,$ID2@10.160.0.12:4001,$ID3@10.160.0.13:4001,$ID4@10.160.0.14:4001"

dc up -d v1 v2 v3 v4 >/dev/null 2>&1 || fail "could not bring the topology up"
for svc in v1 v2 v3 v4; do
  [ -n "$(dc ps -q "$svc")" ] || fail "$svc did not start — $(dc logs "$svc" 2>&1 | tail -5)"
done
echo "  four validators up, anchored on one set"

# ---- leg 1: the bonded set fills -------------------------------------------
# Standing has to be REAL before it can be taken away. Four separate bonds seal
# on four container disks, register over TCP, and commit; the committed set is
# what the TTL prunes, so this is the only number the later legs move.
echo "== leg 1: four real bonds commit, and the bonded set reaches ${VALIDATORS} =="
FILLED=$(await_bonded v1 "$VALIDATORS" "$FILL_BUDGET")
if [ "$FILLED" -lt "$VALIDATORS" ]; then
  echo "  bonded reached ${FILLED} of ${VALIDATORS} in ${FILL_BUDGET}s"
  for svc in v1 v2 v3 v4; do
    echo "    ${svc}: $(dc logs "$svc" 2>&1 | grep -cE 'chain: (saved|committed)' || true) commit line(s), height $(head_height "$svc")"
  done
  fail "the committed bonded set never reached ${VALIDATORS}. Nothing below measures decay — there is no standing yet to decay."
fi
H_FILL=$(head_height v1)
echo "  ✓ bonded=${FILLED} at height ${H_FILL} — four bonds are committed and standing is real"

# ---- leg 2: the control — no decay without a stop ---------------------------
# THE ABLATION, on retention's own axis. If standing decayed with time rather
# than with failure to re-prove, the counter would fall here too and leg 3 would
# be green for a reason that has nothing to do with the attack. So the same
# window runs with nothing removed, and the counter has to hold.
echo "== leg 2: CONTROL — with all four alive, standing does NOT decay =="
echo "  (advancing ${CONTROL_BLOCKS} blocks, past a full TTL of ${BOND_TTL}, with nobody stopped)"
H_TARGET=$((H_FILL + CONTROL_BLOCKS))
H_CTRL=$(await_height v1 "$H_TARGET" "$CONTROL_BUDGET")
CTRL_OK=$?
if [ "$CTRL_OK" -ne 0 ]; then
  fail "the chain reached only height ${H_CTRL} of ${H_TARGET} in ${CONTROL_BUDGET}s with all four validators alive. The control cannot run on a chain that is not advancing, and neither can leg 3. Raise CONTROL_BUDGET only after confirming in the progress lines that the height is climbing."
fi
CTRL_MIN=$(min_bonded_since_full v1 "$VALIDATORS")
CTRL_NOW=$(bonded_now v1)
echo "  height ${H_FILL} → ${H_CTRL}; bonded now=${CTRL_NOW}, lowest seen since the set filled=${CTRL_MIN}"
[ "$CTRL_NOW" = "$VALIDATORS" ] \
  || fail "bonded is ${CTRL_NOW} after a full TTL window in which NOTHING was stopped. Standing is decaying on its own, so leg 3 would prove nothing: raise BOND_TTL above the live renewal cadence, or this is a real defect in the renewal path."
[ "$CTRL_MIN" = "$VALIDATORS" ] \
  || fail "bonded DIPPED to ${CTRL_MIN} during the no-stop window before recovering. A live validator is lapsing and re-registering, so the margin between the TTL and the renewal cadence is too thin for leg 3's fall to be attributable."
echo "  ✓ four alive, four bonded, across a window longer than the TTL — decay does not fire on its own"

# ---- leg 3: decay ----------------------------------------------------------
# The attack: v4 stops re-proving. Its process is gone, which is what releasing
# a plot looks like from the network's side — it cannot answer a fresh challenge
# and cannot submit a renewal.
echo "== leg 3: v4 stops re-proving, and loses its standing =="
H_STOP=$(head_height v1)
dc stop v4 >/dev/null 2>&1 || fail "could not stop v4"
echo "  v4 stopped at height ${H_STOP}; the other three keep committing and keep renewing"
DECAYED=$(await_bonded_down_to v1 $((VALIDATORS - 1)) "$DECAY_BUDGET")
DECAY_OK=$?
H_DECAY=$(head_height v1)
echo "  height ${H_STOP} → ${H_DECAY}; bonded=${DECAYED}"

# The chain must have been LIVE across the window. A frozen counter on a stalled
# chain is not a denial, and a counter that fell while the chain stood still
# would not have come from the sweep, which runs inside block application.
if [ "${H_DECAY:-0}" -le "${H_STOP:-0}" ]; then
  fail "the chain did not advance past height ${H_STOP} after v4 stopped (now ${H_DECAY}). The TTL sweep runs inside block application, so with no blocks there is no eviction to observe — this is a liveness failure of the three survivors, not a decay result."
fi
if [ "$DECAY_OK" -ne 0 ]; then
  fail "v4 stopped re-proving at height ${H_STOP} and the committed bonded set is STILL ${DECAYED} at height ${H_DECAY}, ${DECAY_BUDGET}s later — past a TTL of ${BOND_TTL} blocks. An identity is coasting on a proof it can no longer answer for, which is the economy of scale this arm exists to deny."
fi
[ "$DECAYED" -eq $((VALIDATORS - 1)) ] \
  || fail "the bonded set fell to ${DECAYED}, not $((VALIDATORS - 1)). More than the stopped identity lost standing, so this is not decay-denies-coasting: the three that kept re-proving were supposed to keep theirs."
echo "  ✓ bonded ${VALIDATORS} → ${DECAYED} on a chain that kept committing — the identity that stopped"
echo "    re-proving lost its standing, and the three that kept re-proving kept theirs"

# ---- leg 4: re-earned, not restored ----------------------------------------
# The other half of the claim. If standing came back merely because the node
# came back, the proof was never what bought it. v4 restarts onto its OWN
# volume — same identity, same plot, same store — so the only thing that can
# restore its standing is a FRESH registration committing on-chain.
echo "== leg 4: v4 returns, and must RE-EARN standing with a fresh registration =="
dc start v4 >/dev/null 2>&1 || fail "could not restart v4"
REJOINED=$(await_bonded v1 "$VALIDATORS" "$REJOIN_BUDGET")
H_REJOIN=$(head_height v1)
if [ "$REJOINED" -lt "$VALIDATORS" ]; then
  echo "  bonded=${REJOINED} at height ${H_REJOIN} after ${REJOIN_BUDGET}s"
  dc logs v4 2>&1 | tail -10 | sed 's/^/    v4: /'
  fail "v4 came back on its own store and did not regain standing within ${REJOIN_BUDGET}s. Decay is supposed to be recoverable by re-proving; a bond that can never be re-earned is a different rule than the one this suite describes."
fi
echo "  ✓ bonded=${REJOINED} at height ${H_REJOIN} — standing returned when a fresh registration committed,"
echo "    not when the process did"

# ---- verdict ---------------------------------------------------------------
echo
echo "  re-challenge TTL   : ${BOND_TTL} blocks, ${BOND_SIZE} bonds, ${VALIDATORS} validators"
echo "  control (no stop)  : bonded held at ${VALIDATORS} across ${CONTROL_BLOCKS} blocks (heights ${H_FILL}→${H_CTRL})"
echo "  decay (v4 stopped) : bonded ${VALIDATORS} → ${DECAYED} (heights ${H_STOP}→${H_DECAY})"
echo "  re-earned          : bonded back to ${REJOINED} at height ${H_REJOIN}"
echo
echo "RESULT: PASS ✅  retention decay denies coasting over real containers: four real bonds commit and hold standing while all four keep re-proving; the one identity that stops re-proving is evicted from the committed bonded set within the TTL, on a chain that keeps committing; and it regains standing only when a fresh registration commits"
