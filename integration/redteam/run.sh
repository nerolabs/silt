#!/usr/bin/env bash
# Byzantine red-team field test (validator accountability), fully
# automated on one host in real containers over real TCP. It proves an honest
# silt validator holds three concrete, wire-reachable accountability properties
# against the repo's own ADVERSARY harness flags:
#
#  1. EQUIVOCATION — a double-signing validator is CAUGHT and SLASHED.
#  2. FORGED BLOCK — a block with a forged proposer sig is REJECTED pre-attest.
#  3. LOW-BOND — an under-bonded proposer's block is REFUSED.
#
# Plus a POSITIVE CONTROL: two honest validators still commit a normal block,
# so a rejection is a real defence, not a dead/quorum-broken swarm.
#
# And a fourth property, measured on the same drills: THE FLOOR BOX HOLDS UNDER
# ADVERSARIAL INPUT. Two honest seats — equiv-x, the detector that has to hold
# two conflicting chains and reconcile them, and h3, the target the crafted
# proposals are aimed at — are cgroup-pinned to the declared floor spec: one
# core, 2 GiB, no swap. Each reports `memory.peak`, the cgroup's own high-water
# mark, against that ceiling.
#
# THE MEASUREMENT IS ONLY EVIDENCE IF TWO THINGS HOLD, and both are asserted:
#  • the kernel really enforced the spec (ft_spec_binds reads memory.max,
#    memory.swap.max and nproc from INSIDE the seat, before any number from it
#    is believed) — otherwise the peak is a measurement of an ordinary box; and
#  • the attack actually LANDED on that seat — otherwise a small peak is a box
#    that sat idle. The drills above are what prove the attack landed, so the
#    ceiling verdict is credited only when the drill on that seat passed.
# A peak read from a seat whose drill failed is printed and explicitly NOT
# credited, because the two readings are not interchangeable.
#
# If a pinned seat is OOM-killed, that is the FINDING and not a tuning problem:
# an unbounded system on a small box is unsafe rather than slow, so the suite
# reports the kill rather than raising a limit around it.
#
# Every assertion keys off a REAL log line the daemon prints (confirmed against
# cmd/silt/daemon.go and the in-process analogs e2e/equivocation_test.go +
# e2e/proposal_reject_test.go) — no invented strings.
#
# Usage:./run.sh # build, run all scenarios, tear down; exit 0 = PASS
#  KEEP=1 ./run.sh # leave the topology up afterward to poke at
set -uo pipefail
cd "$(dirname "$0")"
ROOT=$(cd ../.. && pwd)

# shellcheck source=../lib.sh
. "$ROOT/integration/lib.sh"

# The declared floor spec, and it is the spec the compose file pins equiv-x and
# h3 to. The env knobs exist to probe a TIGHTER box, never to quietly loosen a
# failing run: the guard below compares what the kernel reports against what was
# asked for, so a loosened value shows up in the report rather than hiding in it.
FLOOR_CPUS=${FLOOR_CPUS:-1}
FLOOR_MEM=${FLOOR_MEM:-2g}

PROJECT=redteam
dc() { docker compose -p "$PROJECT" "$@"; }
cleanup() {
  [ "${KEEP:-0}" = 1 ] && return 0
  dc --profile equiv --profile propose down -v >/dev/null 2>&1 || true
  ft_sweep "$PROJECT"
  rm -f "$(dirname "$0")/silt"
}
# EXIT alone is not enough. The run-all driver kills a suite that overruns its
# cap with SIGTERM precisely so this trap can tear the topology down, and a
# suite that leaves containers up makes the NEXT suite's preflight refuse — or,
# worse, makes it measure a box that is still holding another suite's memory.
trap cleanup EXIT INT TERM

# Wait until a container's stdout (docker compose logs) contains a pattern.
# The adversary/slash/reject lines are printed to STDOUT by the daemon
# (fmt.Printf in cmd/silt/daemon.go), so we grep the compose logs, not debug.log.
# The timeout is a DEADLINE, not an iteration count. `docker compose logs` costs real
# time and its cost grows with the log, so a loop of N sleep-1 iterations takes far
# longer than N seconds on a loaded host — which is how this suite blew a 300 s cap
# while every individual wait looked small enough to fit inside it. Reading the clock
# makes the numbers above mean what they say.
wait_log() { # svc pattern timeout_s
  local svc=$1 pat=$2 t=${3:-60} deadline
  deadline=$(( $(date +%s) + t ))
  while [ "$(date +%s)" -lt "$deadline" ]; do
    dc logs "$svc" 2>&1 | grep -qE "$pat" && return 0
    sleep 1
  done
  return 1
}

PASS=1
fail() { echo "  FAIL: $*"; PASS=0; }

# Per-drill verdicts, kept apart from the suite-wide PASS because the floor-spec
# ceiling report below credits a seat's peak only if the attack on THAT seat
# landed. A peak from a seat nothing reached is not evidence about the ceiling.
EQUIV_OK=0      # the detector caught and slashed the double-signer
PROPOSE_OK=0    # h3 refused both crafted proposals and committed neither
SPEC_X=UNREAD; SPEC_H3=UNREAD      # did the floor spec bind on that seat
PEAK_X=""; PEAK_H3=""              # memory.peak, read before the seat is removed
OOM_X=unknown; OOM_H3=unknown

ft_require docker go awk || exit 2
ft_docker_up || { echo "FAIL: no reachable docker daemon"; exit 2; }

# This suite now measures a memory ceiling, so a dirty box is not noise — it is
# the measurement. Anything else holding memory on this host shows up as a
# pinned seat's headroom.
ft_preflight "$PROJECT" || exit 2

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

# THE VACUITY GUARD for the detector's seat. Read before the attack, because a
# peak measured on a seat whose limits did not bind is a measurement of an
# ordinary box and would be a confident green from nothing.
echo "  -- equiv-x is on the declared floor spec (${FLOOR_CPUS} core / ${FLOOR_MEM} / no swap) --"
if ft_spec_binds "$PROJECT" equiv-x "$FLOOR_MEM" "$FLOOR_CPUS"; then
  SPEC_X=BOUND
  echo "  ✓ the kernel is enforcing the floor spec on the detector's seat"
else
  SPEC_X=UNBOUND
  fail "the floor spec did NOT bind on equiv-x — its memory.peak below would be a measurement of an ordinary box, not of the floor"
fi

# THE GUARD'S OWN NEGATIVE CONTROL. A vacuity guard that cannot fail is a
# comment, not evidence. h2 is the same image running the same daemon with NO
# cgroup pin at all, so the guard must report it UNBOUND. If it reported h2
# bound too, it would be reading something other than the seat's own limits and
# every BOUND verdict in this run would be free.
if ft_spec_binds "$PROJECT" h2 "$FLOOR_MEM" "$FLOOR_CPUS" >/dev/null 2>&1; then
  fail "the floor-spec guard called the UNPINNED seat h2 bound. It is not reading a seat's own limits, so every BOUND verdict here is vacuous and no peak in this run is evidence."
else
  echo "  ✓ negative control: the same guard reports the unpinned seat (h2) UNBOUND"
fi

# The adversary retries until it has earned standing with BOTH peers, then reports.
if wait_log equiv-a 'adversary: equivocation complete \(double-signed height [0-9]+\)' 90; then
  echo "  adversary double-signed (real: 'adversary: equivocation complete')"
else
  fail "equivocator never completed the double-sign (could not earn standing with both peers)"
  # The adversary only sees an OK=false reply, so its own line can do no better than
  # GUESS a cause ("not yet standing?"). The TARGETS know the real reason and print it
  # under -debug: 'gather/prepare: REJECTED (ValidateProposal)' carries the error, and
  # the per-sweep 'standing' line carries the reputation the proposer rule reads. A
  # failure diagnosed only from the adversary's guess is undiagnosable from the outside.
  dc logs equiv-a 2>&1 | tail -8 | sed 's/^/    equiv-a: /'
  for tgt in equiv-x equiv-yz; do
    dc logs "$tgt" 2>&1 | grep -aiE 'REJECTED|REFUSED|standing|anti-release floor|reputation' \
      | tail -8 | sed "s/^/    $tgt: /"
  done
fi
# The honest detector catches it and prints the slash for the adversary's ID.
if wait_log equiv-x "chain: slashed equivocator $ID_EQUIV_A" 90; then
  EQUIV_OK=1
  echo "  SCENARIO 1: PASS — honest replica caught the double-sign and SLASHED $ID_EQUIV_A"
  dc logs equiv-x 2>&1 | grep -E "chain: slashed equivocator $ID_EQUIV_A" | tail -1 | sed 's/^/    equiv-x: /'
else
  fail "honest replica did NOT slash the equivocator — accountability property (D2) not observed"
  dc logs equiv-x 2>&1 | tail -20 | sed 's/^/    equiv-x: /'
fi

# THE CEILING, on the seat the fork was driven onto. memory.peak lives with the
# container's cgroup, so it has to be read BEFORE the topology comes down — this
# is the last moment it exists.
PEAK_X=$(ft_peak_bytes "$PROJECT" equiv-x)
OOM_X=$(ft_oom_killed "$PROJECT" equiv-x)
echo "  -- the detector's memory ceiling, measured across the attack --"
echo "  equiv-x memory.peak = $(ft_peak_report "${PEAK_X:-}" "$FLOOR_MEM")"
[ "$OOM_X" != "true" ] || fail "equiv-x was OOM-KILLED at ${FLOOR_MEM} while reconciling the forks. An unbounded working set on a small box is unsafe, not slow: this is a finding to instrument and reduce to a local repro, not a limit to raise."

dc --profile equiv rm -sf equiv-a equiv-x equiv-yz >/dev/null 2>&1 || true

# ─────────────────────────────────────────────────────────────────────────────
echo ""
echo "########## SCENARIO 2 & 3 — FORGED BLOCK and LOW-BOND proposals rejected ##########"
# A fresh honest target H3; the forger and low-bond proposer each send ONE
# crafted proposal and report whether H3 refused. The daemon prints
# 'adversary: <label> proposal correctly REJECTED by <id>' on refusal (and
# 'UNEXPECTEDLY ACCEPTED... (DEFECT)' if H3 wrongly attested).
dc --profile propose up -d h3
wait_log h3 'peer: [0-9a-f]{64}' 20 || fail "h3 never came up"

# THE VACUITY GUARD for the target's seat, same reason as equiv-x's above.
echo "  -- h3 is on the declared floor spec (${FLOOR_CPUS} core / ${FLOOR_MEM} / no swap) --"
if ft_spec_binds "$PROJECT" h3 "$FLOOR_MEM" "$FLOOR_CPUS"; then
  SPEC_H3=BOUND
  echo "  ✓ the kernel is enforcing the floor spec on the target's seat"
else
  SPEC_H3=UNBOUND
  fail "the floor spec did NOT bind on h3 — its memory.peak below would be a measurement of an ordinary box, not of the floor"
fi
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

# POSITIVE CONTROL (audit): before crediting H3's REJECTIONS below, prove H3
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
  # Both crafted proposals were refused AND neither committed: the attack landed
  # on this seat and was denied, which is what makes its peak below mean something.
  dc logs forger  2>&1 | grep -qE "forge-block proposal correctly REJECTED by $ID_H3" \
    && dc logs lowbond 2>&1 | grep -qE "lowbond-propose proposal correctly REJECTED by $ID_H3" \
    && PROPOSE_OK=1
fi

# THE CEILING, on the seat the crafted proposals were aimed at.
PEAK_H3=$(ft_peak_bytes "$PROJECT" h3)
OOM_H3=$(ft_oom_killed "$PROJECT" h3)
echo "  -- the target's memory ceiling, measured across both crafted proposals --"
echo "  h3 memory.peak = $(ft_peak_report "${PEAK_H3:-}" "$FLOOR_MEM")"
[ "$OOM_H3" != "true" ] || fail "h3 was OOM-KILLED at ${FLOOR_MEM} under the crafted proposals. An unbounded working set on a small box is unsafe, not slow: this is a finding to instrument and reduce to a local repro, not a limit to raise."

# ─────────────────────────────────────────────────────────────────────────────
# THE FLOOR BOX UNDER ADVERSARIAL INPUT — the fourth property, reported as a
# number with its margin so a regression surfaces as shrinking headroom and not
# only as an eventual failure.
#
# A seat's ceiling reading is CREDITED only when all four hold: the kernel bound
# the spec, the attack on that seat landed and was denied, the peak was actually
# read, and it stayed under the ceiling without an OOM-kill. Anything short of
# that is printed as NOT CREDITED with the reason, because an uncredited number
# and a measured one are not interchangeable — and an uncredited one is a gap,
# which is a failure rather than a quiet omission.
echo ""
echo "########## THE FLOOR SPEC UNDER ADVERSARIAL INPUT ##########"
echo "  spec: ${FLOOR_CPUS} core / ${FLOOR_MEM} RAM / no swap, kernel-enforced per seat"
CEILING_HELD=1
seat_ceiling() { # seat_ceiling <svc> <spec-verdict> <drill-ok> <peak> <oom> <what-landed>
  local svc=$1 spec=$2 drill=$3 peak=$4 oom=$5 what=$6 want
  want=$(ft_iec_bytes "$FLOOR_MEM")
  printf '  %-8s %s\n' "$svc" "$(ft_peak_report "${peak:-}" "$FLOOR_MEM")"
  if [ "$spec" != BOUND ]; then
    echo "           NOT CREDITED: the floor spec did not bind on this seat (${spec})"; CEILING_HELD=0; return
  fi
  if [ "$drill" != 1 ]; then
    echo "           NOT CREDITED: ${what} did not land and get denied on this seat, so a low peak is an idle box"; CEILING_HELD=0; return
  fi
  case "$peak" in ''|*[!0-9]*)
    echo "           NOT CREDITED: memory.peak could not be read — no measurement is a failure, not a pass"; CEILING_HELD=0; return ;;
  esac
  if [ "$oom" = true ]; then
    echo "           FINDING: OOM-KILLED at the ceiling under ${what}"; CEILING_HELD=0; return
  fi
  if [ "$peak" -ge "$want" ]; then
    echo "           FINDING: the peak reached the ceiling under ${what}"; CEILING_HELD=0; return
  fi
  echo "           CREDITED: under the ceiling while ${what}"
}
seat_ceiling equiv-x "$SPEC_X"  "$EQUIV_OK"   "$PEAK_X"  "$OOM_X"  "the double-signed forks were driven in and reconciled"
seat_ceiling h3      "$SPEC_H3" "$PROPOSE_OK" "$PEAK_H3" "$OOM_H3" "the forged and under-bonded proposals were refused"
if [ "$CEILING_HELD" = 1 ]; then
  echo "  CEILING: HELD on both pinned seats, under adversarial input"
else
  fail "the memory ceiling under adversarial input is NOT credited on every pinned seat — see the reasons above"
fi

echo ""
if [ "$PASS" = 1 ]; then
  echo "RESULT: PASS ✅  honest validator caught+slashed the equivocator, rejected the forged block, and refused the under-bonded proposer (positive control committed a normal block), and both floor-spec seats stayed under their kernel-enforced ${FLOOR_MEM} ceiling while the attacks landed"
else
  echo "RESULT: FAIL ❌  see failures above"
fi
exit $((1-PASS))
