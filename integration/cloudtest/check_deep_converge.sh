#!/usr/bin/env bash
# check_deep_converge.sh — offline RED/GREEN for the deep-heights convergence row (12c).
#
# The chain keeps committing after the deep drive, about one block every 23 s. The
# row used to read each validator's height and head hash in two separate
# chain-status calls, so a commit between them paired height h with block h+1's hash
# and graded four validators holding identical blocks as a fork. This drives the REAL
# grade_deep_converge with dh_status stubbed to advance one block on every call, and
# asserts the two properties the row must hold: an agreeing chain that keeps moving
# passes, and a genuine fork at the tip fails. Runs in seconds; no network.
# Exit 0 = GREEN.
set -uo pipefail
cd "$(dirname "$0")"

export PROJECT_ID="${PROJECT_ID:-deep-converge-selftest}"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
export RESULTS_JSONL="$tmp/results.jsonl"

# shellcheck source=/dev/null
. ./lib.sh 2>/dev/null || true
# shellcheck source=/dev/null
. ./scenarios.sh

# The canonical chain: block h's hash is "h" padded to 64 hex digits. A forked
# validator holds a different block at every height from FORK_AT up.
hash_at() { printf '%064x' "$(( $1 + ${2:-0} ))"; }

# dh_status models the field: one global clock ticks on every status call, and the
# whole network commits a block every COMMIT_EVERY ticks, so each validator sits at
# its START height plus the blocks committed so far. Two reads of one validator can
# straddle a commit, exactly as they did after the deep drive. In the field a status
# read is an SSH round trip of a few seconds and a block commits about every 23 s,
# roughly one block per 7 reads; the default of 5 commits faster than that.
dh_status() {
  local v="$1" f="$tmp/calls" n h off=0
  n=$(( $(cat "$f" 2>/dev/null || echo 0) + 1 )); echo "$n" > "$f"
  eval "h=\$(( START_${v//-/_} + n / ${COMMIT_EVERY:-5} ))"
  [ "$v" = "${FORKED:-none}" ] && off=1000000
  printf 'head height: %s\nhead hash:   %s\n' "$h" "$(hash_at "$h" "$off")"
}

# dh_hash_at answers from the same model: the canonical chain's block at H, or the
# forked validator's own block there once its fork has begun.
dh_hash_at() {
  local off=0
  [ "$1" = "${FORKED:-none}" ] && [ "$2" -ge "${FORK_AT:-0}" ] && off=1000000
  hash_at "$2" "$off"
}

last_verdict() {
  python3 -c "import json,sys
rows=[json.loads(l) for l in open('$RESULTS_JSONL') if l.strip()]
v=[r['verdict'] for r in rows if r['flow']==sys.argv[1]]
print(v[-1] if v else 'none')" "$1"
}

fail=0
expect() { if [ "$2" = "$3" ]; then echo "  ok   $1"; else echo "  FAIL $1: want $2, got $3"; fail=1; fi; }
reset() { rm -f "$tmp/calls"; : > "$RESULTS_JSONL"; }

# The field case: val-a at h134 and val-b at h134, with val-c and val-d a block or
# two ahead, every one of them on the same chain while it keeps committing.
reset
START_val_a=134 START_val_b=134 START_val_c=135 START_val_d=136 FORKED=none grade_deep_converge >/dev/null
expect "an agreeing chain that keeps committing passes" pass "$(last_verdict 12c-deep-converge)"

# A real fork: val-b holds different blocks from h130 up, below every head.
reset
START_val_a=136 START_val_b=136 START_val_c=135 START_val_d=136 FORKED=val-b FORK_AT=130 grade_deep_converge >/dev/null
expect "a fork below the tip fails" fail "$(last_verdict 12c-deep-converge)"

# A validator more than two blocks behind fails, fork or not.
reset
START_val_a=140 START_val_b=140 START_val_c=134 START_val_d=140 FORKED=none grade_deep_converge >/dev/null
expect "a validator three or more blocks behind fails" fail "$(last_verdict 12c-deep-converge)"

if [ "$fail" = 0 ]; then echo "deep-converge self-test GREEN"; else echo "deep-converge self-test RED"; exit 1; fi
