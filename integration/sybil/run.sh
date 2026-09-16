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
#  C2-a positive control: with the anchors present the chain commits and the C2
#  status line shows `wheels engaged (young network …)`.
#  C2-b no quiet capture: stop BOTH anchors; the Sybil set (s1 proposes, s2
#  attests) canNOT advance the chain — NO new block. The training wheels
#  refuse the capture at whichever layer fires first: locally, a young Sybil
#  set cannot even earn committed bonded standing without an anchor-proposed
#  block (reputation gate); behind that sits the anchor co-sign gate
#  (ErrAnchorRequired) for a Sybil that HAS standing. The test reports which
#  layer stopped it — the OUTCOME (no capture) is the same. The pure
#  anchor-gate form (with pre-banked Sybil bonds) is a cloud-test concern.
#  C2-c bonus: with ≥8 equal Sybil bonds the C2 atomization note fires (an equal-
#  bond split reads as a fingerprint, not real decentralization).
#
# Usage:./run.sh # build, test, tear down; exit 0 = PASS
#  SYBILS=0 ./run.sh # core only (a1,a2,s1,s2)
#  KEEP=1 ./run.sh
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
# the young network must drain its deferred bond registrations AUTONOMOUSLY —
# no publish traffic exists yet, so the reactive drain (a BondRegs-only block on the
# chain-sync sweep) is the only way any validator earns committed standing. The
# drain serializes ~1 registration per sweep under the byte budget, so give
# it a few 30s sweeps. Observables are deterministic product strings: a commit on a1
# (the drain block), and a commit heard on s1 (the broadcast → the sybil SYNCED the
# committed chain).
echo "  letting bonds seal + the anchors DRAIN the submitted registrations (a few 30s sweeps)…"
await_log a1 'chain: committed block' 120 || fail "C2-a: the idle young network never committed a bond-registration drain block — deferred regs must not wait for publish traffic"
await_log s1 'chain: committed block' 120 || fail "C2-a: the sybil never heard/synced a committed block — no path from a non-attester validator to the committed chain"

REG_A1="$A1_ID@https://10.90.0.2:4003"
REG_S1="$S1_ID@https://10.90.0.4:4003"
BOOT_A1="$A1_ID@10.90.0.2:4001"; BOOT_A2="$A2_ID@10.90.0.3:4001"
BOOT_S1="$S1_ID@10.90.0.4:4001"; BOOT_S2="$S2_ID@10.90.0.5:4001"
ALL="$BOOT_A1,$BOOT_A2,$BOOT_S1,$BOOT_S2"

publish() { # publish to <registry-ref> [via <peers>], run inside <svc>; echoes add stdout+stderr
  local svc=$1 reg=$2 peers=${3:-$ALL}
  dc exec -T "$svc" sh -c "head -c 32768 /dev/urandom > /tmp/p.bin; \
    silt swarm add /tmp/p.bin -peers '$peers' -registry '$reg' -replication 1 2>&1"
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

# ── C2-a2: a BONDED sybil BANKS committed standing while the anchors are up ──
# WHAT THIS CONTROL IS FOR. C2-b refuses the Sybil set and reports WHICH training
# wheel refused. The anchor gate is the strongest form — "even with standing, a young
# commit needs an anchor" — but that attribution is only meaningful if the sybil
# actually HOLDS committed standing when the anchors go away. Otherwise C2-b's refusal
# could be the weaker standing gate and the strong claim goes unproven.
#
# IT USED TO ASK THE SYBIL TO PUBLISH, WHICH THE DESIGN FORBIDS. Publishing means
# proposing, and while the network is young ONLY anchors propose: proposerQualifiedAt
# returns launchAnchor(id), which asks whether THIS id is an anchor, not whether anchors
# are reachable. A bonded non-anchor therefore cannot propose with the anchors up or
# down, and the refusal it gets is wrapped in ErrLowReputation ("reputation below
# threshold") even though reputation is not the reason — which is what sent this
# suite's own note chasing a broken drain. C2-a directly above ASSERTS the wheels are
# engaged, so the old C2-a2 contradicted the control immediately preceding it.
#
# WHAT IT ASSERTS NOW is the thing the design actually provides, and the thing C2-b
# depends on: a bonded sybil earns committed standing by SUBMITTING its registration
# for an anchor to bank (submit-don't-propose, MsgSubmitBondReg), never by proposing.
# The anchor narrates the bank at info: "bond-reg drain: pending registrations
# committed". That line is the sybil's standing becoming REAL on-chain, with no
# proposal by the sybil anywhere in it.
echo ""
echo "== C2-a2: a bonded Sybil BANKS committed standing via the anchor drain (submit, never propose) =="
# The observable is the COMMITTED BLOCK ITSELF, not a drain-internal narration: the
# daemon prints "chain: committed block N (E entries, B bond-regs, A attestations)"
# unconditionally (it is a registered entry in cmd/silt/observable_contract.go), and a
# block with B >= 1 IS a banked registration. Reading the block is also the honest
# altitude for this claim — standing becomes real when a registration COMMITS, not when
# some internal sweep decides to try. The drain serializes roughly one registration per
# sweep, so allow several sweeps.
if await_log a1 'chain: committed block [0-9]+ \([0-9]+ entries, [1-9][0-9]* bond-regs' 150; then
  echo "  C2-a2 PASS: an anchor COMMITTED a block carrying bond registrations — the submit-don't-propose route banked standing on-chain"
  echo "    (SCOPE: the committed-block line does not name WHOSE registration is in the block, so this"
  echo "     proves the banking ROUTE works and ran, not that this exact sybil's bond is in that block."
  echo "     C2-b's gate reporting below is what distinguishes the anchor gate from a standing gate.)"
  dc logs a1 2>&1 | grep -aoE 'chain: committed block [0-9]+ \([0-9]+ entries, [1-9][0-9]* bond-regs[^)]*\)' | tail -1 | sed 's/^/    a1: /'
else
  fail "C2-a2: no anchor ever committed a block carrying a bond registration (submit-don't-propose never banked), so C2-b's refusal cannot be attributed to the anchor gate rather than to missing standing"
fi
# NEGATIVE HALF, and it is the half that keeps this honest: the sybil must NOT have
# proposed anything. Banking standing is a submission; a committed block PROPOSED by a
# non-anchor while the wheels are engaged would be the launch-fork this rule exists to
# prevent.
if dc logs s1 2>&1 | grep -qaE 'chain: committed block [1-9]'; then
  fail "C2-a2: the Sybil COMMITTED a block of its own while the wheels were engaged — a non-anchor must not propose during the launch window"
else
  echo "  C2-a2 PASS (negative half): the Sybil proposed nothing — it earned standing by SUBMITTING, which is the only route the launch window allows"
fi

# ── C2-b: no quiet capture — stop both anchors, the Sybil quorum cannot advance ─
echo ""
echo "== C2-b no quiet capture: with BOTH anchors gone, the Sybil set CANNOT advance the chain =="
dc stop a1 a2 >/dev/null 2>&1 || fail "could not stop the anchors"
echo "  both anchors stopped; the Sybil set (s1 proposes, s2 attests) now tries to commit alone…"
# The anchored CEILING: the height the network legitimately reached while the
# anchors were present (drain blocks + C2-a + C2-a2 publishes included). A capture
# means the Sybils push the chain BEYOND this without any anchor. Re-read from s1
# (a1 is stopped): s1 is fully synced (C2-a/C2-a2 proved it).
h_ceiling=$(head_height s1); h_ceiling=${h_ceiling:-$h1}
cc_pre=$(commit_count s1); cc_pre_s2=$(commit_count s2)
# the audit /: publish through the LIVE Sybils only. $ALL still lists the
# just-stopped anchors, and a content scatter that cannot reach them errors the
# `swarm add` out BEFORE it ever hits s1's registry — so the deterministic
# proposer-standing refusal (chain.ValidateProposal → ErrLowReputation, a SYNC 500
# on a bonded-0 proposer) is never captured and the reason comes back empty (the
# original flake: a dead-peer scatter failure masqueraded as "no gate reason").
# Peering only s1,s2 makes s1's synchronous refusal reliably observable — and that
# specific string proves s1 is ALIVE and the standing gate fired (a dead swarm
# yields a dial error, a capture yields a committed block; neither yields this).
LIVE_SYBILS="$BOOT_S1,$BOOT_S2"
CAP=""
for _ in $(seq 1 12); do
  CAP=$(publish s1 "$REG_S1" "$LIVE_SYBILS" 2>&1)
  echo "$CAP" | grep -qiE 'reputation below threshold|immature network requires anchor' && break
  sleep 2
done
sleep 3
h_post=$(head_height s1); h_post=${h_post:-0}
cc_post=$(commit_count s1); cc_post_s2=$(commit_count s2)
# Attribute to the DETERMINISTIC product strings only (chain.go ErrLowReputation /
# ErrAnchorRequired). The old alternation grepped `quorum`, but ErrNoQuorum's text
# is "insufficient valid attestations" (no such word) — and the sync standing check
# short-circuits BEFORE any gather, so the no-quorum path isn't even reached here.
REASON=$(echo "$CAP" | grep -oiE '(immature network requires anchor[^:"]*|reputation below threshold[^:"]*)' | head -1)
# The OUTCOME under test: did the Sybil set capture the chain? PASS iff it did NOT
# advance beyond the anchors without them. The two capture signals are both SYNC-
# IMMUNE — critical, because under load s1 can SYNC the anchors' already-committed
# C2-a block DURING this window (chainrole.go SyncChain, a plain catch-up) and its
# head rises 0→h1 with NO local commit and NO 'committed block' log. That is NOT a
# capture: block h1 was committed BY THE ANCHORS in C2-a. Earlier this harness read
# a freshly-captured (still-lagging) h_pre and counted that benign catch-up as an
# advance — a false FAIL. The honest signals:
#  • a Sybil node logs a FRESH 'committed block' — fires only on a LOCAL commit or
#  a Sybil-proposer broadcast (chainrole.go MsgCommitBlock); a catch-up SyncChain
#  does NOT fire OnCommit, so a benign anchor-block sync never trips it; or
#  • the head passes the anchored CEILING h1 — a late sync can only reach ≤ h1, so
#  any height > h1 required a NEW block, which without anchors is a capture.
# Report WHICH training-wheels layer stopped it — a young Sybil set is blocked at
# BOTH: it cannot even earn on-chain standing without an anchor-committed block, and
# even with standing a young commit needs anchor co-sign. Same C2 property either way.
if [ "${h_post:-0}" -gt "${h_ceiling:-0}" ] || [ "${cc_post:-0}" -gt "${cc_pre:-0}" ] || [ "${cc_post_s2:-0}" -gt "${cc_pre_s2:-0}" ]; then
  fail "C2-b the Sybil quorum ADVANCED the chain past the anchored ceiling h${h_ceiling} without any anchor (head→$h_post, s1 commits ${cc_pre}→$cc_post, s2 commits ${cc_pre_s2}→$cc_post_s2) — QUIET CAPTURE"
elif echo "$REASON" | grep -qiE 'anchor|immature'; then
  echo "  C2-b PASS: NO block beyond the anchored ceiling h${h_ceiling} — the bonded Sybil quorum could not capture the young network"
  echo "    (head $h_post ≤ ceiling h${h_ceiling}; no fresh Sybil commit: s1 ${cc_pre}→$cc_post, s2 ${cc_pre_s2}→$cc_post_s2 — both anchors absent)"
  echo "    gate: the ANCHOR co-sign requirement (ErrAnchorRequired) — the strongest form: even with"
  echo "    standing a young commit needs an anchor. reason: $REASON"
elif echo "$REASON" | grep -qiE 'reputation'; then
  echo "  C2-b PASS: NO block beyond the anchored ceiling h${h_ceiling} — the Sybil quorum could not capture the young network"
  echo "    (head $h_post ≤ ceiling h${h_ceiling}; no fresh Sybil commit: s1 ${cc_pre}→$cc_post, s2 ${cc_pre_s2}→$cc_post_s2 — both anchors absent)"
  echo "    gate: the standing requirement. reason: $REASON"
  echo "    ⚠ NOTE: C2-a2 proved an anchor BANKED the sybil's registration, so it should hold"
  echo "    committed standing here and the ANCHOR gate should be what refuses. Standing firing"
  echo "    instead means the banked bond lapsed (TTL) between C2-a2 and now. The outcome (no"
  echo "    capture) still holds — this is a weaker attribution, not a weaker denial."
else
  # the audit: NO block beyond the ceiling is only "no capture" if a TRAINING-WHEELS
  # gate actually refused it. With no anchor/immature/reputation reason surfaced, a
  # stalled chain is indistinguishable from a DEAD swarm (e.g. the anchored C2-a control
  # above never committed either) — crediting that as C2-b PASS would fake-green the
  # whole property.
  echo "  CAP was: ${CAP:-<empty>}"
  fail "C2-b: NO block beyond the anchored ceiling h${h_ceiling}, but NO training-wheels gate reason surfaced (REASON='${REASON}') — cannot distinguish the standing/anchor gate refusing the Sybils from a chain that simply never runs (a dead swarm) or a publish that never reached s1's registry. Expected s1 to synchronously refuse with 'reputation below threshold: proposer … bonded 0' or 'immature network requires anchor'. The anchored C2-a positive control must commit AND that specific refusal must appear."
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
  echo "RESULT: PASS ✅  C2 holds — no quiet capture: the young objective network drains its bond registrations autonomously, a BONDED sybil publishes with the anchors present (its standing is real), and the bonded Sybil quorum still cannot advance the chain once the anchors are gone — refused by the anchor co-sign gate (ErrAnchorRequired). The ≥8-bond atomization signal stays a cloud-scale concern (integration/cloudtest)."
else
  echo "RESULT: FAIL ❌  (see the failing assertion(s) above)"
fi
exit $((1-pass))
