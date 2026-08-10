#!/usr/bin/env bash
# Field test #2 — proof-of-retrieval (PoR) audit / the "liar" node, over REAL
# daemons in REAL containers.
#
# The `silt sim run audit` claim (in-process): a storage node that KEEPS its
# storage proof but DELETES the data is CAUGHT by a verify-without-fetch PoR
# challenge, loses standing, and durability survives.
#
# This harness drives the REAL daemon over TCP and gates the LITERAL claim — the
# gap this test used to only assert-around is now CLOSED (#232 wired -liar +
# -audit into cmd/silt), so the audit catch + slash is exercised over the wire:
#
#   1. Publish an erasure-coded file (-replication 2) so shards scatter across
#      four honest holders (1,2,3,5) plus one PoR liar (holder4, -liar).
#   2. A caretaker (-care <careLink> -audit 15s, -log info) runs both its repair
#      loop AND the verify-without-fetch PoR audit sweep.
#   3. POSITIVE CONTROL: before any damage, the caretaker reports NO repair and
#      the file is retrievable from the intact swarm.
#   4. GATED, #232: the -audit sweep CHALLENGES every shard's holders and grades
#      their proofs against the key derived from the care link — NO ground-truth
#      fetch. The honest holders PASS; the liar (which advertises + answers but
#      proves over data it dropped) FAILS and is slashed. Assert passed≥1 AND
#      FAILED≥1 — the "caught without fetch + standing slash" claim, over the wire.
#   5. THE ATTACK / OUTCOME: on top of the liar, `rm` an honest holder's shard
#      files while it keeps running. Gated on the OUTCOME (immutable: test the
#      outcome, not the mechanism) — the file must stay bit-perfect retrievable.
#      The caretaker's detect/repair/re-replicate activity is printed as
#      observability (placement decides which shards truly go missing, so the
#      exact counts vary; a wedged repair loop is still visible).
#   6. Regression gate: assert -liar and -audit still exist, so the #232
#      assertions above can't be silently skipped by a future build.
#
# Usage:  ./run.sh          # build, test, tear down; exit 0 = PASS
#         KEEP=1 ./run.sh   # leave the topology up afterward to poke at
set -uo pipefail
cd "$(dirname "$0")"
ROOT=$(cd ../.. && pwd)
export COMPOSE_PROJECT_NAME=audit

dc() { docker compose "$@"; }
cleanup() { [ "${KEEP:-0}" = 1 ] || dc down -v >/dev/null 2>&1 || true; }
trap cleanup EXIT

# The caretaker's repair sweep runs every RepairInterval (60s, not a daemon
# flag) — so a sweep-dependent assertion must allow ~1.5 intervals.
SWEEP_WAIT=${SWEEP_WAIT:-80}
objs()  { dc exec -T "$1" sh -c 'find /data/objects -type f 2>/dev/null | wc -l' | tr -d ' \r\n'; }
carelog() { dc exec -T caretaker sh -c "grep -c \"$1\" /data/debug.log 2>/dev/null || true" | tr -d ' \r\n'; }
# The -audit sweep prints its report to STDOUT (fmt.Printf), so grep the compose
# logs, not debug.log. audit_max <field> returns the largest value that field
# ("passed"/"FAILED") reached across all sweeps so far.
auditlog()  { dc logs caretaker 2>&1 | grep -E 'audit [0-9a-f]+:'; }
audit_max() { auditlog | grep -oE "[0-9]+ $1" | grep -oE '[0-9]+' | sort -n | tail -1; }

echo "== build the silt binary on the host (linux/$(go env GOARCH)) and the image =="
( cd "$ROOT" && CGO_ENABLED=0 GOOS=linux GOARCH="$(go env GOARCH)" go build -trimpath -o integration/audit/silt ./cmd/silt )
docker build -q -t silt-audit . >/dev/null

echo "== phase 1: registry/bootstrap seed =="
dc up -d seed
SEED_ID=""
for _ in $(seq 1 30); do
  SEED_ID=$(dc logs seed 2>&1 | grep -oE 'peer: [a-f0-9]{64}' | head -1 | awk '{print $2}')
  [ -n "$SEED_ID" ] && break
  sleep 1
done
[ -n "$SEED_ID" ] || { echo "FAIL: seed never reported a NodeID"; dc logs seed | tail -20; exit 1; }
export SEED_ID
echo "seed id: $SEED_ID"
PEERS="$SEED_ID@10.60.0.10:4001"
REG="$SEED_ID@https://10.60.0.10:4003"

echo "== phase 2: four honest holders (1,2,3,5) + one PoR liar (holder4) =="
dc up -d holder1 holder2 holder3 holder4 holder5
for h in holder1 holder2 holder3 holder4 holder5; do
  ok=0
  for _ in $(seq 1 40); do
    dc logs "$h" 2>&1 | grep -q "bootstrapped" && { ok=1; break; }
    sleep 1
  done
  [ "$ok" = 1 ] || { echo "FAIL: $h never bootstrapped"; dc logs "$h" | tail -20; exit 1; }
done
echo "  all five nodes bootstrapped (holder4 is a -liar)"

echo "== publish a 2 MB erasure-coded file (-replication 2 so a single liar can't threaten durability) =="
# -replication 2: every shard lands on TWO holders, so the byte-dropping liar
# (holder4) never SOLELY holds a shard — parity + a second copy backstop it,
# durability is preserved, and the audit still catches the liar's bad-proof copy.
# Wiping ONE honest holder (holder2) then under-replicates the shards whose second
# copy sat on the liar (dropped) — a reproducible degradation the caretaker
# re-replicates — while three honest survivors (h1,h3,h5) keep every stripe far
# above k=10/n=16, so the loss is always recoverable (no flaky below-k stripes).
dc exec -T holder1 sh -c "head -c 2000000 /dev/urandom > /tmp/f.bin; sha256sum /tmp/f.bin | cut -d' ' -f1 > /tmp/f.sha; silt swarm add /tmp/f.bin -peers '$PEERS' -registry '$REG' -mode convergent -replication 2" >/tmp/audit_add.txt 2>&1
CARE=$(grep -oE 'siltcare:v1:[A-Za-z0-9_:-]+' /tmp/audit_add.txt | head -1)
LINK=$(grep -oE 'silt:v1:[A-Za-z0-9_:-]+'    /tmp/audit_add.txt | head -1)
WANT=$(dc exec -T holder1 cat /tmp/f.sha | tr -d '\r\n ')
echo "  care: ${CARE:-<none>}"
echo "  link: ${LINK:-<none>}"
[ -n "$CARE" ] && [ -n "$LINK" ] || { echo "FAIL: publish returned no care/silt link"; cat /tmp/audit_add.txt; exit 1; }
echo "  placement: h1=$(objs holder1) h2=$(objs holder2) h3=$(objs holder3) h4=$(objs holder4 || echo 0)[liar] h5=$(objs holder5) seed=$(objs seed) objects"
[ "$(objs seed)" = 0 ] || echo "  note: seed unexpectedly holds objects (expected 0 with -capacity 1)"

echo "== phase 3: caretaker with the care link (-log info: its repair loop narrates) =="
export CARE_LINK="$CARE"
dc up -d caretaker
ok=0
for _ in $(seq 1 40); do
  dc logs caretaker 2>&1 | grep -q "caretaking" && { ok=1; break; }
  sleep 1
done
[ "$ok" = 1 ] || { echo "FAIL: caretaker never started caretaking"; dc logs caretaker | tail -20; exit 1; }
sleep 8   # let the warm-start manifest fetch land in the caretaker's store

pass=1

echo "== POSITIVE CONTROL: intact swarm — no repair, file retrievable =="
pre_repaired=$(carelog "stripe repaired")
echo "  caretaker 'stripe repaired' lines so far: ${pre_repaired:-0} (want 0)"
[ "${pre_repaired:-0}" = 0 ] || { echo "FAIL: caretaker repaired an intact file (false positive)"; pass=0; }
dc exec -T holder1 sh -c "silt swarm get '$LINK' -o /tmp/pre.bin -peers '$PEERS' -registry '$REG' && sha256sum /tmp/pre.bin | cut -d' ' -f1" >/tmp/audit_pre.txt 2>&1
PRE=$(grep -oE '^[a-f0-9]{64}' /tmp/audit_pre.txt | tail -1)
[ "$PRE" = "$WANT" ] && echo "  intact file retrievable: yes (bit-perfect)" || { echo "FAIL: intact file not retrievable"; cat /tmp/audit_pre.txt; pass=0; }

echo "== PoR AUDIT (#232): the caretaker's -audit sweep verifies proofs WITHOUT fetching =="
# holder4 is a -liar: it advertises as a provider and answers MsgChallenge, but
# proves over data it dropped, so the auditor's verify (no ground-truth fetch)
# REJECTS it and slashes it — while the honest holders PASS. This is the literal
# "caught without fetch + standing slash" half of the sim claim, now wire-driven.
echo "  waiting up to 40s for an audit sweep to challenge the swarm…"
apassed=0; afailed=0
for _ in $(seq 1 40); do
  apassed=$(audit_max passed); afailed=$(audit_max FAILED)
  [ "${apassed:-0}" -ge 1 ] && [ "${afailed:-0}" -ge 1 ] && break
  sleep 1
done
auditlog | tail -3 | sed 's/^/    /'
if [ "${apassed:-0}" -ge 1 ]; then
  echo "  honest holders PASSED the verify-without-fetch challenge (max passed=$apassed) ✅"
else
  echo "FAIL: no honest holder ever passed an audit challenge (auditor never got a valid proof)"; pass=0
fi
if [ "${afailed:-0}" -ge 1 ]; then
  echo "  the -liar was CAUGHT without fetching its bytes and slashed (max FAILED=$afailed) ✅"
else
  echo "FAIL: the -liar was never caught by the audit sweep — verify-without-fetch not reachable"; pass=0
fi

echo "== the attack: on TOP of the liar, rm an honest holder's shard files (it KEEPS running) =="
# The daemon retains its persisted storage proofs (n.proofs) — "keep the receipt,
# ditch the goods" — but the bytes under /data/objects are gone, so holder2 now
# answers MsgHasChunk=false and the caretaker's probe sees it (a liar, by
# contrast, LIES "of course I have it" — which is why the -audit above is needed).
# This stacks an honest-holder loss on top of the liar's silent drop: the cynical
# question is whether the DATA still survives (immutable: test the OUTCOME).
dc exec -T holder2 sh -c 'rm -rf /data/objects/*'
echo "  after rm: h1=$(objs holder1) h2=$(objs holder2) h3=$(objs holder3) h5=$(objs holder5)"
[ "$(objs holder2)" = 0 ] || { echo "FAIL: rm did not clear holder2's store"; pass=0; }

echo "== wait ~1.5 repair sweeps (${SWEEP_WAIT}s), then observe the caretaker's repair activity =="
sleep "$SWEEP_WAIT"

# Repair ACTIVITY is observability, not a gate: which shards actually go missing
# (vs stay covered by a second replica) is placement-dependent, so the exact
# degraded/repaired/re-scatter counts vary run to run. The gated OUTCOME is the
# durability check below — the file must survive bit-perfect. We still print the
# activity so a real regression (repair loop wedged) is visible.
degraded=$(carelog "stripe degraded")
repaired=$(carelog "stripe repaired")
belowk=$(carelog "repair below k")
h2after=$(objs holder2)
echo "  caretaker debug.log: stripe degraded=${degraded:-0}  stripe repaired=${repaired:-0}  repair below k=${belowk:-0}"
echo "  re-replication onto the emptied holder2: h2=$h2after object(s) (0 == its shards were covered elsewhere)"
if [ "${degraded:-0}" -ge 1 ] || [ "${repaired:-0}" -ge 1 ] || [ "$h2after" -gt 0 ]; then
  echo "  caretaker actively repaired the honest loss (detected/reconstructed/re-replicated) ✅"
else
  echo "  note: no active repair this run — every wiped shard still had a surviving replica"
fi

echo "== assert the file survived the loss (bit-perfect after repair) =="
dc exec -T holder1 sh -c "silt swarm get '$LINK' -o /tmp/post.bin -peers '$PEERS' -registry '$REG' && sha256sum /tmp/post.bin | cut -d' ' -f1" >/tmp/audit_post.txt 2>&1
POST=$(grep -oE '^[a-f0-9]{64}' /tmp/audit_post.txt | tail -1)
echo "  want $WANT"
echo "  got  ${POST:-<none>}"
[ "$POST" = "$WANT" ] || { echo "FAIL: file not bit-perfect after loss+repair"; cat /tmp/audit_post.txt; pass=0; }

echo "== regression gate (#232): the PoR-audit / liar path IS wire-reachable =="
# This USED to assert the gap (no -liar / no audit trigger, so the "caught without
# fetch + standing slash" claim was sim-only). #232 wired both flags, so the gap
# is closed and the slash is asserted directly above. Guard it: if a future build
# drops the flags, fail loudly so the audit assertions above aren't silently
# skipped.
# Plain `docker run` on the image (not `dc run seed`, whose static IP 10.60.0.10
# collides with the already-running seed and would fail to start → empty help).
help=$(docker run --rm --entrypoint silt silt-audit daemon -h 2>&1 || true)
if echo "$help" | grep -qE '^\s*-liar\b' && echo "$help" | grep -qE '^\s*-audit\b'; then
  echo "  confirmed: -liar and -audit both exist on 'silt daemon' — Node.Audit is"
  echo "  driven over the real wire, and the verify-without-fetch catch + slash is"
  echo "  asserted above (not just the detect-via-probe + repair half)."
else
  echo "FAIL: -liar and/or -audit flag missing — the PoR-audit assertions above could"
  echo "      not have exercised the real audit path. Re-wire the seam (#232)."; pass=0
fi

echo
if [ "$pass" = 1 ]; then
  echo "RESULT: PASS ✅  the -audit sweep caught the -liar WITHOUT fetching its bytes and"
  echo "        slashed it (honest holders passed); then an honest holder's bytes were"
  echo "        deleted on top of the liar and the file still survived bit-perfect"
  echo "        (durability outcome), with the caretaker's repair activity observed."
else
  echo "RESULT: FAIL ❌  (see the FAIL lines above)"
fi
exit $((1-pass))
