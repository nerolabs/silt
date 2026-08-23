#!/usr/bin/env bash
# check_escape_fingerprint.sh — offline RED/GREEN for ft_escape_progress (#536).
#
# The run 45da13c-17686 mis-attribution: the escape fingerprint counted
# `round-change` from journald, where that structured n.logf line NEVER
# appears (it is written to $STORE/debug.log only) — so rc read 0 on every
# sample and a LIVE ladder (114 round-change lines at h64 r1→r5) fingerprinted
# as FROZEN → a manufactured WEDGE FAIL. This self-test drives the REAL
# ft_escape_progress with a stubbed ssh_node so the parsing is exercised without
# a cloud, and asserts the three properties the fix must hold. Runs in seconds;
# no network. Exit 0 = GREEN.
set -uo pipefail
cd "$(dirname "$0")"

# lib.sh guards PROJECT_ID with ${:?} (exits under set -u) — this offline test
# never touches a project, so satisfy it before sourcing.
export PROJECT_ID="${PROJECT_ID:-escape-fingerprint-selftest}"

# shellcheck source=/dev/null
. ./lib.sh 2>/dev/null || true
# Pull in ft_escape_progress + epoch_to_iso (function defs only; no side effects).
# shellcheck source=/dev/null
. ./scenarios.sh

# Stub ssh_node AFTER the sources (lib.sh defines the real one) so dlog/
# jlog_since resolve to canned output. Dispatch on the remote command: debug.log
# reads get the round-change stream, journalctl reads get the committed-block
# banners. STUB_* env vars set per case.
ssh_node() {
  local cmd="$2"
  case "$cmd" in
    *debug.log*)   printf '%s' "${STUB_DEBUGLOG:-}" ;;
    *journalctl*)  printf '%s' "${STUB_JOURNAL:-}" ;;
    *)             : ;;
  esac
}

fail=0
say() { printf '%s\n' "$*"; }
ok()  { printf '  \033[32m✓\033[0m %s\n' "$*"; }
bad() { printf '  \033[31m✗\033[0m %s\n' "$*"; fail=1; }

KILL=1000000000                                   # a fixed kill epoch
ISO="$(epoch_to_iso "$KILL")"                     # its ISO prefix
AFTER="${ISO}.500Z"                               # a debug.log stamp after the kill
LATER="$(epoch_to_iso $((KILL + 300)))Z"          # 5 min later, still after

# ── Case 1: a LIVE ladder (round-changes present, advancing) must NOT read rc=0
STUB_JOURNAL=""                                   # no commit since kill (wedged height)
STUB_DEBUGLOG="$AFTER info round-change: advancing (#432 view-change) height=64 round=1
$AFTER info round-change: recorded (#432 view-change) height=64 round=1
$AFTER info round-change: advancing (#432 view-change) height=64 round=2"
fp="$(ft_escape_progress "$KILL" val-a)"
if printf '%s' "$fp" | grep -q 'rc=3 '; then ok "live ladder read rc=3 (was rc=0 before #536): $fp"
else bad "live ladder must count round-changes from debug.log, got: $fp"; fi

# ── Case 2: two samples of a LIVE ladder differ (more round-changes appear) →
#    the caller sees fp0 != fp1 → NOT a wedge (routes to OUT-OF-MODEL gap)
fp0="$fp"
STUB_DEBUGLOG="$STUB_DEBUGLOG
$LATER info round-change: advancing (#432 view-change) height=64 round=3"
fp1="$(ft_escape_progress "$KILL" val-a)"
if [ "$fp0" != "$fp1" ]; then ok "an advancing ladder yields fp0 != fp1 (no wedge): $fp0 → $fp1"
else bad "advancing ladder must change the fingerprint, got equal: $fp0"; fi

# ── Case 3: an UNREADABLE source (ssh returned nothing) yields `?`, never 0 —
#    two such samples must not compare equal-and-frozen and manufacture a wedge
STUB_JOURNAL=""; STUB_DEBUGLOG=""
fpu="$(ft_escape_progress "$KILL" val-a)"
if printf '%s' "$fpu" | grep -q 'rc=?'; then ok "unreadable debug.log reads rc=? (UNKNOWN, not 0): $fpu"
else bad "an unreadable source must be UNKNOWN (?), got: $fpu"; fi
# the WEDGE-FAIL guard: fp with a `?` must be rejected ("${fp#*?}" != fp)
if [ "${fpu#*\?}" != "$fpu" ]; then ok "the '?' guard rejects an UNKNOWN fingerprint from the WEDGE branch"
else bad "the '?' guard failed to detect UNKNOWN in: $fpu"; fi

# ── Case 4: a genuinely FROZEN readable ladder (round-changes present, but the
#    SAME across both samples, no new commit) IS a wedge — the true positive.
#    The journal is READABLE (non-empty) but carries no committed-block-since-kill
#    → h=0 (not h=?): a real down-designee wedge, not an unreadable source.
STUB_JOURNAL="Aug 23 16:12:00 host silt[1]: standing self=abc reputation=1024"
STUB_DEBUGLOG="$AFTER info round-change: recorded (#432 view-change) height=64 round=1
$AFTER info round-change: recorded (#432 view-change) height=64 round=1"
fa="$(ft_escape_progress "$KILL" val-a)"
fb="$(ft_escape_progress "$KILL" val-a)"
if [ "$fa" = "$fb" ] && [ "${fa#*\?}" = "$fa" ] && printf '%s' "$fa" | grep -q 'h=0'; then
  ok "a frozen readable ladder is a true wedge (fp0==fp1, no '?', h=0): $fa"
else bad "frozen readable ladder must be a stable readable fingerprint, got: $fa / $fb"; fi

[ "$fail" = 0 ] && say "escape-fingerprint self-test GREEN (#536)" || say "escape-fingerprint self-test RED"
exit "$fail"
