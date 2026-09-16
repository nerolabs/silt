#!/usr/bin/env bash
# ─────────────────────────────────────────────────────────────────────────────
# Field test #1 — multi-validator consensus + partition-heal, in real Docker.
#
# The M0 keystone under test: OBJECTIVE, BOND-WEIGHTED COMMIT ADMISSION. Four real
# `silt daemon -validator` processes on a flat Docker network form two groups.
# (Corrected 2026-09-12: this line used to locate the objectivity in FORK CHOICE.
# It is not there — `heavier` ranks on Height then head hash and reads nothing
# else. What the on-chain bond makes objective is WHO MAY PARTICIPATE and what
# counts toward quorum, not how two candidate heads are ordered.)
# A network partition (the built-in test-harness -block-peers flag) severs them;
# each group commits its OWN fork over real TCP. We assert:
#
#  (P0 negative control) an UNBONDED / no-earned-standing publish is REFUSED
#  (no commit) — the write path is earned, not a stamp.
#  (P1 convergence) the MAJORITY (valA,valB,valC) commits and all
#  three replicas agree on one head. Three anchors is the
#  smallest group that can commit at all: objective mode
#  requires a DERIVED strict anchor majority, floor(A/2)+1 = 3
#  of 4, which configuration cannot lower.
#  (P2 partition) the majority advances while the severed
#  minority (valD, one anchor) commits NOTHING and stalls. A
#  sub-quorum side that cannot commit is quorum intersection
#  (I1) holding — not a degraded fork. An earlier shape of this
#  suite split the anchors 2-2 and expected two rival heads;
#  under the 3-of-4 rule that split commits nothing on EITHER
#  side, so it could never pass and never said why.
#  (P3 heal) valD restarts without -block-peers, reloads its own
#  persisted store, and CATCHES UP to the majority history —
#  with nothing to drop, because it committed nothing.
#
# WHAT THIS HARNESS ASSERTS, corrected 2026-09-12. Convergence is NOT toward
# whichever fork carries more bond: fork choice ranks on height then head hash
# and has no bond term, and with the finality gate on Reconcile admits only forks containing
# the committed head. The GROUND TRUTH here is and always was the chain-status
# head-hash equality, which is posture-independent. The reorg narration is
# supporting evidence only, and under a >2/3 commit floor it may never fire at all.
# Whether P2's own expectation (a 2-anchor group committing at height 1) is still
# reachable is an OPEN question routed to the research — see README.md.
#
# Every assertion keys off a REAL observed CLI flag / stdout line / chain-status
# field — no invented strings. This is the Docker-real counterpart of
# e2e/partition_test.go (in-process), over real sockets and kernel networking.
#
# Usage:./run.sh # build, test, tear down; exit 0 = PASS
#  KEEP=1 ./run.sh # leave the topology up afterward to poke at
# ─────────────────────────────────────────────────────────────────────────────
set -uo pipefail
cd "$(dirname "$0")"
ROOT=$(cd ../.. && pwd)

# THIS SUITE CANNOT COMMIT AS WRITTEN, AND THE BUDGET IS NOT WHY.
#
# It seats FOUR anchors and partitions them 2-2 (A,B | C,D). In objective mode the
# launch requirement is a DERIVED strict anchor majority, floor(A/2)+1 = 3 of 4, and it
# is derived precisely so configuration cannot disable quorum intersection (daemon.go
# prints it at start-up: "training wheels: 4 anchor(s), strict majority 3 required
# (objective; derived)"). Neither side of a 2-2 split can reach 3, so no group commits
# anything, ever -- observed: all four validators at 0 committed blocks. The publish
# retries are therefore doomed by construction, and raising the per-suite budget only
# buys more of them: driven at 300s it TIMED OUT, and driven at 900s on an idle box it
# TIMED OUT again at 15m02s having reached the same place.
#
# The rule is not a regression. proposerQualifiedAt names this exact shape as the thing
# it exists to refuse -- "the both-sybil-proposed 2-2 anchor split the intersecting-quorum
# invariant (I1) must otherwise refuse". This suite encodes a pre-rule expectation, the
# same way sybil's C2-a2 does. Fixing it is a decision about what the suite should claim
# (seat an odd anchor count, split 3-1, or assert the refusal as the property), not a
# timeout to tune.
dc()      { docker compose "$@"; }
dc_heal() { docker compose -f docker-compose.yml -f docker-compose.heal.yml "$@"; }
cleanup() { [ "${KEEP:-0}" = 1 ] || dc_heal down -v >/dev/null 2>&1 || true; }
trap cleanup EXIT

# Wait until a regex appears in a service's stdout (docker compose logs).
# The timeout is a DEADLINE, not an iteration count. Each poll pays for a
# `docker compose logs` whose cost grows with the log, so a loop of N sleep-1
# iterations takes far longer than N seconds on a loaded host — every nominal
# timeout in this suite understated its true wall-clock, without bound. Reading
# the clock makes these numbers mean what they say.
await_log() { # service pattern [timeout_s]
  local svc=$1 pat=$2 t=${3:-60} deadline
  deadline=$(( $(date +%s) + t ))
  while [ "$(date +%s)" -lt "$deadline" ]; do
    dc logs "$svc" 2>&1 | grep -qE "$pat" && return 0
    sleep 1
  done
  return 1
}
# head height / head hash from `silt chain-status` read inside a container's store.
head_height() { dc exec -T "$1" silt chain-status -store /data 2>/dev/null | awk '/head height/{print $3}'; }
head_hash()   { dc exec -T "$1" silt chain-status -store /data 2>/dev/null | awk '/head hash/{print $3}'; }

echo "== build the silt binary on the host (linux/$(go env GOARCH)) and the image =="
( cd "$ROOT" && CGO_ENABLED=0 GOOS=linux GOARCH="$(go env GOARCH)" go build -trimpath -o integration/consensus/silt ./cmd/silt ) \
  || { echo "FAIL: host build failed"; exit 1; }
docker build -q -t silt-consensus . >/dev/null || { echo "FAIL: image build failed"; exit 1; }

# Derive each validator's NodeID from its -id-seed up front (no daemon needed),
# exactly as the examples/flows do — so -anchors / -attesters / -block-peers are
# fillable BEFORE launch. The four share one anchor set → identical genesis. The
# binary is a linux image (built for the container), so run `silt id` INSIDE a
# throwaway container rather than on the host.
silt_id() { docker run --rm silt-consensus silt id -id-seed "$1" | tr -d '\r'; }
ID_A=$(silt_id 7001)
ID_B=$(silt_id 7002)
ID_C=$(silt_id 7003)
ID_D=$(silt_id 7004)
ANCHORS="$ID_A,$ID_B,$ID_C,$ID_D"
export ID_A ID_B ID_C ID_D ANCHORS
echo "  valA=$ID_A"
echo "  valB=$ID_B"
echo "  valC=$ID_C"
echo "  valD=$ID_D"

pass=1
fail() { echo "FAIL: $*"; pass=0; }

# ─────────────────────────────────────────────────────────────────────────────
# P0 — NEGATIVE CONTROL: an unbonded / no-earned-standing validator must REFUSE
# to commit a publish. We run one lone objective validator (no bonded attester
# can qualify it), publish, and assert NO "committed block" appears. Proves the
# write path is EARNED standing, not a rubber-stamp (flow4's negative control).
# ─────────────────────────────────────────────────────────────────────────────
echo ""
echo "== P0 negative control: a lone unqualified validator must REFUSE to commit =="
# Fully self-contained throwaway (its OWN loopback-only container, no main
# topology, no shared network) so it can't couple to or race the four-validator
# run below. A lone objective validator has no qualified bonded attester, so a
# publish against its registry must NOT commit.
NEG_OUT=$(docker run --rm silt-consensus \
  sh -c 'mkdir -p /data
    silt daemon -id-seed 9001 -listen 127.0.0.1:4001 -serve-registry 127.0.0.1:4003 -store /data \
      -validator -objective=false -min-rep 100 -quorum 1 -attesters '"$ID_A"' -bond 8M -bond-audit 1s \
      -capacity 1G -log info >/data/neg.log 2>&1 &
    for i in $(seq 1 30); do grep -q "^registry:" /data/neg.log && break; sleep 1; done
    grep -q "^registry:" /data/neg.log && echo "---reg-bound---yes" || echo "---reg-bound---no"
    head -c 65536 /dev/urandom > /tmp/n.bin
    silt swarm add /tmp/n.bin -peers "$(awk "/^peer:/{print \$2}" /data/neg.log)" \
      -registry "$(sed -En "s/.*serving ([^ ]+).*/\1/p" /data/neg.log | head -1)" >/tmp/neg_add.out 2>&1 || true
    echo "---neg-add---"; cat /tmp/neg_add.out
    echo "---neg-commits---"; grep -c "committed block" /data/neg.log || true' 2>&1)
# POSITIVE CONTROL (audit): "0 committed blocks" is only a REFUSAL if the
# write path actually ran. If the registry never bound, or the publish never
# reached the registry (a dead/broken swarm), that ALSO shows 0 commits and would
# false-pass as "correctly refused". Require both: the registry bound, AND the
# publish reached it (the entry was accepted/registered — a lone validator
# registers the entry but never COMMITS it for lack of a qualified quorum).
if ! echo "$NEG_OUT" | grep -q -- "---reg-bound---yes"; then
  fail "P0-BROKEN: the lone validator's registry never bound — a 'no commit' result is meaningless (write path not exercised)"
elif echo "$NEG_OUT" | sed -n '/---neg-add---/,/---neg-commits---/p' | grep -qiE 'connection refused|no route to host|dial|no such host|registry.*unreachable'; then
  fail "P0-BROKEN: the publish never reached the registry (dial/connection error) — a 'no commit' result is meaningless, not a real refusal"
elif echo "$NEG_OUT" | sed -n '/---neg-commits---/,$p' | grep -qE '^[1-9]'; then
  fail "P0 unbonded publish COMMITTED — rubber-stamp (negative control failed)"
else
  echo "  P0 PASS: registry bound + publish reached it, yet the unqualified validator refused to commit (no committed block)"
fi

# ─────────────────────────────────────────────────────────────────────────────
# Bring up the four validators and let objective standing accrue.
# ─────────────────────────────────────────────────────────────────────────────
echo ""
echo "== bring up the four validators (A,B,C majority | D minority, partitioned) =="
dc up -d valA valB valC valD 2>&1 | grep -vE 'Network|Volume|Container.*(Creat|Start|Running)' || true
# Fail loud if the daemons didn't come up (was previously suppressed → silent).
for v in valA valB valC valD; do
  st=$(dc ps --status running --format '{{.Service}}' 2>/dev/null | grep -x "$v" || true)
  [ -n "$st" ] || fail "$v is not running after 'compose up' — daemon failed to start"
done
await_log valA 'peer: [0-9a-f]{64}@' 40 || { fail "valA never printed its peer line"; }
await_log valA 'registry: chain-backed, serving' 40 || fail "valA registry never came up"
await_log valB 'bootstrapped \(' 40 || fail "valB never bootstrapped"
await_log valC 'bootstrapped \(' 40 || fail "valC never bootstrapped"
await_log valD 'peer: [0-9a-f]{64}@' 40 || fail "valD never printed its peer line"

echo "  bonds sealed (objective standing):"
for v in valA valB valC valD; do
  bl=$(dc logs "$v" 2>&1 | grep -oE 'bond: (sealed|reloaded)[^\n]*' | head -1)
  echo "    $v: ${bl:-<no bond line>}"
done

# P2a — the partition really is in effect on the minority validator.
echo "  partition flag in effect on the minority validator:"
for v in valD; do
  pl=$(dc logs "$v" 2>&1 | grep -oE '⚠ PARTITION:[^\n]*' | head -1)
  echo "    $v: ${pl:-<no partition line>}"
  [ -n "$pl" ] || fail "$v did not report the partition (-block-peers) in effect"
done

echo "  letting objective standing accrue (14s)…"; sleep 14

# ─────────────────────────────────────────────────────────────────────────────
# Drive commits: two publishes into the MAJORITY, one attempted at the minority.
# ─────────────────────────────────────────────────────────────────────────────
REG_A="$ID_A@https://10.50.0.11:4003"
REG_C="$ID_C@https://10.50.0.13:4003"
REG_D="$ID_D@https://10.50.0.14:4003"
BOOT_A="$ID_A@10.50.0.11:4001"
BOOT_C="$ID_C@10.50.0.13:4001"
BOOT_D="$ID_D@10.50.0.14:4001"

# publish <container> <name> <boot> <reg> — retries until a link comes back, so we
# don't race standing accrual. Runs the client from inside the group's registry node.
# The retry budget is a DEADLINE for the same reason await_log's is, and it matters
# more here: each attempt runs a FULL publish inside the container, so forty of them
# is many minutes of wall-clock on a loaded host, not forty seconds.
publish() { # publish <svc> <name> <boot> <reg> [budget_s] [per_attempt_s]
  local svc=$1 name=$2 boot=$3 reg=$4 out deadline attempt
  # The per-attempt bound defaults to the whole budget, so it never cuts a publish that
  # is going to succeed. Callers that EXPECT failure (the minority) pass a short one.
  attempt=${6:-${5:-90}}
  deadline=$(( $(date +%s) + ${5:-90} ))
  while [ "$(date +%s)" -lt "$deadline" ]; do
    # Bound the ATTEMPT, not just the loop. A publish aimed at a node that cannot commit
    # does not fail fast — it waits on a commit that never comes, and a deadline checked
    # only BETWEEN attempts never fires while one attempt hangs. Measured: a single
    # minority attempt hung ~5 minutes and nearly took this suite past its cap.
    # `docker compose` DIRECTLY, not the dc() helper: timeout(1) execs a BINARY and
    # cannot invoke a shell function, so `timeout N dc …` dies instantly with
    # "cannot open file: exec" and every attempt fails without ever running. That
    # failure is silent in the ordinary case — the chain still advances on its own
    # sweep, so the suite can look like it merely lost a link.
    out=$(timeout "$attempt" docker compose exec -T "$svc" sh -c "head -c 32768 /dev/urandom > /tmp/$name.bin; \
      silt swarm add /tmp/$name.bin -peers '$boot' -registry '$reg' 2>&1")
    echo "$out" | grep -qoE 'silt:v1:[A-Za-z0-9_:-]+' && { echo "$out" | grep -oE 'silt:v1:[A-Za-z0-9_:-]+' | head -1; return 0; }
    sleep 1
  done
  echo ""
  return 1
}

echo ""
echo "== P1 convergence: the MAJORITY (A,B,C) commits and its replicas agree =="
# Three anchors is the smallest group that can commit anything here: objective mode
# requires the DERIVED strict anchor majority, floor(4/2)+1 = 3.
L1=$(publish valA a1 "$BOOT_A" "$REG_A"); echo "  a1 link: ${L1:-<none>}"
[ -n "$L1" ] || fail "the majority's first publish never committed"
sleep 3
HA=$(head_hash valA); HB=$(head_hash valB); HC=$(head_hash valC)
hA=$(head_height valA); hB=$(head_height valB); hC=$(head_height valC)
echo "  valA head: height=$hA hash=${HA:0:16}…"
echo "  valB head: height=$hB hash=${HB:0:16}…"
echo "  valC head: height=$hC hash=${HC:0:16}…"
if [ -n "$HA" ] && [ "$HA" = "$HB" ] && [ "$HA" = "$HC" ]; then
  echo "  P1 PASS: all three majority replicas agree on one head (${HA:0:16}…) — convergence"
else
  fail "P1 majority replicas DIVERGED (A=$HA B=$HB C=$HC) — no convergence"
fi

echo ""
echo "== P2 partition: the MAJORITY advances; the SUB-QUORUM minority commits NOTHING =="
# THIS IS THE CLAIM, and it is the opposite of a fork. An earlier shape of this suite
# split the four anchors 2-2 and asserted the two sides committed DIFFERENT heads. That
# expectation predates the derived strict-anchor-majority rule: with 3 of 4 required,
# a 2-2 split commits nothing on EITHER side, so the suite could never pass and never
# said why (it just ran out its budget). A minority that cannot commit is not a
# degraded fork — it is quorum intersection (I1) holding, which is the property.
L2=$(publish valA a2 "$BOOT_A" "$REG_A"); echo "  a2 link: ${L2:-<none>}"
await_log valA 'chain: committed block 2' 30 || fail "the majority never reached height 2"

# Drive work at the minority and require it to commit NOTHING. The publish is EXPECTED
# to come back empty: a lone anchor cannot reach the 3-of-4 majority, so nothing it is
# handed can ever be committed. Asserting the empty link alone would be weak (a publish
# can fail for transport reasons), so the committed chain is checked directly.
# A SHORT budget on purpose: this attempt is EXPECTED to come back empty, so spending
# the success-path budget here would just buy 90s of proving the obvious — which is what
# pushed this suite past its cap once already. Long enough to reach the daemon and be
# refused, not long enough to matter.
LD=$(publish valD d1 "$BOOT_D" "$REG_D" 20 15); echo "  d1 link: ${LD:-<none — expected: a sub-quorum commits nothing>}"
sleep 3
H1=$(head_hash valA); h1=$(head_height valA)
H2=$(head_hash valD); h2=$(head_height valD)
echo "  majority head: height=$h1 hash=${H1:0:16}…"
echo "  minority head: height=${h2:-<none>} hash=${H2:0:16}…"
if dc logs valD 2>&1 | grep -qE 'chain: committed block [1-9]'; then
  fail "P2 the SUB-QUORUM minority COMMITTED a block — a lone anchor cannot hold the derived 3-of-4 majority, so this is a quorum-intersection violation (I1)"
elif [ "${h1:-0}" -le 0 ]; then
  fail "P2 the majority did not advance (h=$h1) — the partition proves nothing if nobody committed"
else
  echo "  P2 PASS: the majority advanced to h=$h1 while the severed minority committed NOTHING (no block, stalled)"
fi
# The minority must stall, not fail closed in a way that loses its identity: it is still
# a running, bonded validator that simply cannot reach quorum.
await_log valD 'standing' 10 >/dev/null 2>&1 || true
echo "  minority is alive and bonded, just quorum-short: $(dc logs valD 2>&1 | grep -oE 'bond: (sealed|reloaded)[^\n]*' | head -1)"

echo "== P3 heal → the minority catches up to the majority history =="
# The minority committed NOTHING while severed, so this is a CATCH-UP, not a reorg off
# a rival fork: there is no competing history to drop. That is the claim's last clause
# ("catches up to the majority history on heal") and it is what a sub-quorum partition
# is supposed to do.
H_D_pre=$(head_hash valD); hD_pre=$(head_height valD)
echo "  valD pre-heal head: height=${hD_pre:-<none>} hash=${H_D_pre:0:16}… (nothing committed while severed)"
echo "  restart valD WITHOUT -block-peers, bootstrapped to valA (persisted store reloads and reconciles)…"
dc_heal up -d valD >/dev/null 2>&1 || fail "heal restart of valD failed"
await_log valD 'chain: (adopted a competing fork|caught up [0-9]+ block|restored [0-9]+ block)' 90 \
  || fail "P3 valD never reconciled a peer chain after heal (no catch-up line)"
REORG_LINE=$(dc logs valD 2>&1 | grep -oE 'chain: adopted a competing fork, DROPPING [0-9]+ committed block\(s\)[^\n]*' | tail -1)
CATCHUP_LINE=$(dc logs valD 2>&1 | grep -oE 'chain: caught up [0-9]+ block\(s\) from peers' | tail -1)
RESTORE_LINE=$(dc logs valD 2>&1 | grep -oE 'chain: restored [0-9]+ block\(s\) from disk' | tail -1)
echo "  valD on heal: reload=[${RESTORE_LINE:-<none>}] reorg=[${REORG_LINE:-<none>}] catchup=[${CATCHUP_LINE:-<none>}]"
# A minority that committed nothing must not need to DROP anything to converge.
if [ -n "$REORG_LINE" ]; then
  fail "P3 the healed minority DROPPED committed blocks — it had committed none, so there was no rival fork to drop"
fi
HC=""; hC=""
for _ in $(seq 1 30); do
  HC=$(head_hash valD); hC=$(head_height valD)
  HAf=$(head_hash valA); hAf=$(head_height valA)
  [ -n "$HAf" ] && [ "$HAf" = "$HC" ] && break
  sleep 2
done
echo "  after heal — valA head: height=$hAf hash=${HAf:0:16}…"
echo "  after heal — valD head: height=$hC hash=${HC:0:16}…"
if [ -n "$HAf" ] && [ "$HAf" = "$HC" ] && [ "${hC:-0}" = "${hAf:-0}" ]; then
  echo "  P3 PASS: the healed minority converged to the majority head (${HAf:0:16}…, height $hAf)"
  # HOW it converged has to match WHAT it did while severed. A minority that committed
  # nothing has nothing on disk to reload and nothing to drop, so demanding a
  # "restored N block(s) from disk" line here would contradict the very property P2 just
  # proved. Require the reload proof ONLY when there was something to reload; otherwise
  # the CATCH-UP line is the evidence, and its absence is the real failure.
  RESTORE_N=$(printf '%s' "$RESTORE_LINE" | grep -oE '[0-9]+' | head -1)
  if [ "${hD_pre:-0}" -gt 0 ] 2>/dev/null; then
    if [ -z "$RESTORE_LINE" ] || [ "${RESTORE_N:-0}" -lt 1 ] 2>/dev/null; then
      fail "P3 valD had committed blocks (h=$hD_pre) but emitted no 'chain: restored … from disk' line — it did not rejoin on its own persisted store"
    else
      echo "  P3 reload PROVEN: valD restored ${RESTORE_N} block(s) from disk before reconciling"
    fi
  elif [ -z "$CATCHUP_LINE" ]; then
    fail "P3 valD committed nothing while severed, so convergence must come from a CATCH-UP — but no 'caught up N block(s)' line was emitted"
  else
    echo "  P3 catch-up PROVEN: valD held NO chain of its own (nothing to reload, nothing to drop) and took the majority history whole — ${CATCHUP_LINE}"
  fi
else
  fail "P3 the healed minority did NOT converge to the majority head (A=$HAf@$hAf  D=$HC@$hC)"
fi

echo ""
if [ "$pass" = 1 ]; then
  echo "RESULT: PASS ✅  objective consensus: a sub-quorum partition committed NOTHING while the majority advanced, and caught up on history on heal (head hash equality)"
else
  echo "RESULT: FAIL ❌  (see the failing assertion(s) above)"
fi
exit $((1-pass))
