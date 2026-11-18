#!/usr/bin/env bash
# check_empty_fleet.sh — offline RED/GREEN for grading a sheet with no fleet behind it.
#
# A `run` after an `up` that provisioned nothing graded "all silt nodes report service
# active" as PASS: flow_first_run starts from ok=1 and fails a node it finds down, so on
# an empty node list it finds nothing and passes. This drives the REAL require_fleet and
# flow_first_run against a missing, an empty and a one-node fleet file, with ssh_node
# stubbed so no cloud is touched. Runs in seconds; no network. Exit 0 = GREEN.
set -uo pipefail
cd "$(dirname "$0")"

# lib.sh guards PROJECT_ID with ${:?}; this offline test never touches a project.
export PROJECT_ID="${PROJECT_ID:-empty-fleet-selftest}"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
export RESULTS_JSONL="$tmp/results.jsonl"

# shellcheck source=/dev/null
. ./lib.sh 2>/dev/null || true
# shellcheck source=/dev/null
. ./scenarios.sh

# Stubs AFTER the sources: every node answers "active", and evidence capture is a no-op.
ssh_node() { return 0; }
flow_evidence_nodes() { :; }

fail=0
expect() { # expect LABEL WANT GOT
  if [ "$2" = "$3" ]; then echo "  ok   $1"; else echo "  FAIL $1: want $2, got $3"; fail=1; fi
}
last_verdict() { # last_verdict FLOW
  python3 -c "import json,sys
rows=[json.loads(l) for l in open('$RESULTS_JSONL') if l.strip()]
v=[r['verdict'] for r in rows if r['flow']==sys.argv[1]]
print(v[-1] if v else 'none')" "$1"
}

: > "$RESULTS_JSONL"
NODES_JSON="$tmp/missing.json"
require_fleet >/dev/null; expect "missing fleet file is refused" 1 "$?"
expect "missing fleet file records a FAIL" fail "$(last_verdict infra-fleet-present)"

echo '{}' > "$tmp/empty.json"
NODES_JSON="$tmp/empty.json"
require_fleet >/dev/null; expect "empty fleet is refused" 1 "$?"
flow_first_run >/dev/null
expect "first-run on an empty fleet is a FAIL" fail "$(last_verdict 1-first-run)"

echo '{"val-a": {"role": "validator", "instance_name": "x", "zone": "z"}}' > "$tmp/one.json"
NODES_JSON="$tmp/one.json"
require_fleet >/dev/null; expect "one-node fleet is accepted" 0 "$?"
flow_first_run >/dev/null
expect "first-run on a live one-node fleet is a PASS" pass "$(last_verdict 1-first-run)"

if [ "$fail" = 0 ]; then echo "empty-fleet self-test GREEN"; else echo "empty-fleet self-test RED"; exit 1; fi
