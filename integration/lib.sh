#!/usr/bin/env bash
# integration/lib.sh — shared helpers for the field-test suites and the run-all
# driver. SOURCED, never executed. It keeps the common shell out of every
# suite's run.sh; today the run-all driver is the primary consumer, and the
# suites can adopt these helpers incrementally (they still stand alone).
#
# The convention every suite already follows, which these helpers key off:
#  • a suite prints a final line RESULT: PASS|FAIL|FINDING <text>
#  • and exits 0 on pass (or a deliberately-reproduced FINDING), non-zero on fail
#  • repair/relay/commit events are asserted from <store>/debug.log, not stdout
#
# shellcheck disable=SC2034 # colors are used by sourcing scripts

# ---- pretty output ---------------------------------------------------------
if [ -t 1 ]; then
  C_GREEN=$'\033[32m'; C_RED=$'\033[31m'; C_YELLOW=$'\033[33m'; C_MAGENTA=$'\033[35m'; C_DIM=$'\033[2m'; C_RESET=$'\033[0m'
else
  C_GREEN=""; C_RED=""; C_YELLOW=""; C_MAGENTA=""; C_DIM=""; C_RESET=""
fi

ft_human_time() { # ft_human_time <seconds>
  local s="$1"
  if [ "$s" -lt 60 ]; then printf '%ds' "$s"; else printf '%dm%02ds' "$((s / 60))" "$((s % 60))"; fi
}

# ---- result classification -------------------------------------------------
# ft_result_line <logfile> — the suite's final RESULT: line (empty if none).
ft_result_line() {
  grep -oiE "RESULT: (PASS|FAIL|FINDING)[^\"]*" "$1" 2>/dev/null | tail -1
}

# ft_classify <exit_code> <result_line> — PASS | FINDING | FAIL | TIMEOUT.
# rc==124 is `timeout` killing a suite that ran long — a distinct TIMEOUT, NOT a
# product FAIL (a slow/loaded host or a too-tight per-suite cap, not a regression;
# the blind field test hit this on `soak`). Any OTHER non-zero exit is FAIL. On
# exit 0 the RESULT word decides; a suite that exits 0 with NO RESULT line — or an
# unrecognized one — is a FAIL, not a PASS: the suites run `set -uo pipefail` (not
# `-e`), so one that dies silently after its setup can still exit 0 without ever
# printing a verdict, and defaulting that to PASS would fake green (immutable #4).
ft_classify() {
  local rc="$1" line="$2"
  if [ "$rc" -eq 124 ]; then echo TIMEOUT; return; fi
  if [ "$rc" -ne 0 ]; then echo FAIL; return; fi
  case "$line" in
    *[Rr]"ESULT: PASS"*)    echo PASS ;;
    *[Rr]"ESULT: FINDING"*) echo FINDING ;;
    *[Rr]"ESULT: FAIL"*)    echo FAIL ;;
    "")                     echo FAIL ;;   # exit 0 but no verdict = broken run
    *)                      echo FAIL ;;   # exit 0 with an unrecognized line = broken run
  esac
}

# ft_status_color <PASS|FINDING|FAIL|TIMEOUT> — echoes the matching color escape.
ft_status_color() {
  case "$1" in
    PASS)    printf '%s' "$C_GREEN" ;;
    FINDING) printf '%s' "$C_YELLOW" ;;
    TIMEOUT) printf '%s' "$C_MAGENTA" ;;
    *)       printf '%s' "$C_RED" ;;
  esac
}

# ---- prereqs ---------------------------------------------------------------
# ft_require <cmd...> — fail loudly if any tool is missing, listing all of them.
ft_require() {
  local missing="" c
  for c in "$@"; do command -v "$c" >/dev/null 2>&1 || missing="$missing $c"; done
  if [ -n "$missing" ]; then
    echo "missing required tool(s):$missing" >&2
    return 1
  fi
}

# ft_docker_up — true if a docker daemon is reachable.
ft_docker_up() { docker info >/dev/null 2>&1; }

# ---- start clean, end clean ------------------------------------------------
# A suite that starts on a dirty box measures the wrong machine, and a suite
# that ends dirty poisons the next one. Both have happened here: a run left
# twelve containers up for hours, and the next run's suites returned 137 (the
# guest OOM-killer) and 124 (the per-suite cap) — read at the time as host
# contention rather than as the leftovers they were. So the check is mechanical,
# not a habit.
#
# The asymmetry is deliberate. ft_preflight REFUSES on someone else's leftovers
# rather than removing them: another session or another suite may be mid-run,
# and a harness that silently kills what it finds is worse than one that stops.
# It only cleans its OWN project, which is by definition finished. ft_sweep is
# the mirror — it removes exactly what this suite created.

# ft_preflight <project> — assert the box is clean enough to measure on.
# Removes this project's own leftovers, reports foreign ones and fails.
ft_preflight() {
  local project="$1" stray foreign

  # This suite's own leftovers are always safe to remove: whatever run created
  # them is over (we are the next one), and reusing a half-torn-down topology is
  # how a stale volume silently answers a fresh assertion.
  docker compose -p "$project" down -v --remove-orphans >/dev/null 2>&1 || true

  # A `silt` daemon running on the HOST shares this machine's memory and ports
  # with the containers under test. On a memory-ceiling suite that is not noise,
  # it is the measurement.
  stray=$(pgrep -f '(^|/)silt (daemon|swarm)' 2>/dev/null | tr '\n' ' ')
  if [ -n "$stray" ]; then
    echo "${C_RED}preflight FAILED${C_RESET}: silt is already running on this host (pid(s): $stray)." >&2
    echo "  A host daemon competes with the containers for memory and ports. Stop it, then re-run." >&2
    return 1
  fi

  # Containers from ANY silt suite, including this one's siblings. Named so the
  # operator can see whose they are instead of guessing.
  foreign=$(docker ps --filter 'ancestor=silt-consensus' --filter 'ancestor=silt-durability' \
                      --format '{{.Names}}' 2>/dev/null | tr '\n' ' ')
  foreign="$foreign$(docker ps --format '{{.Names}} {{.Image}}' 2>/dev/null \
                      | awk '$2 ~ /^silt-/ {printf "%s ", $1}')"
  foreign=$(echo "$foreign" | tr -s ' ' | sed 's/^ //;s/ $//')
  if [ -n "$foreign" ]; then
    echo "${C_RED}preflight FAILED${C_RESET}: containers from another silt suite are still up:" >&2
    echo "  $foreign" >&2
    echo "  They hold memory and CPU this run would be measured against. Tear them down" >&2
    echo "  (their suite's ./run.sh does it, or: docker compose -p <project> down -v) and re-run." >&2
    echo "  NOT removed automatically: another session may be mid-run." >&2
    return 1
  fi

  echo "  preflight: no stray silt process, no foreign suite containers"
  return 0
}

# ft_sweep <project> — tear down everything this suite built. Safe to call more
# than once (it is the EXIT trap), and it reports what it could NOT remove
# rather than exiting quietly, because a silent sweep that failed is how the
# next run inherits the mess.
ft_sweep() {
  local project="$1" left
  docker compose -p "$project" down -v --remove-orphans >/dev/null 2>&1 || true
  left=$(docker ps -a --filter "label=com.docker.compose.project=$project" --format '{{.Names}}' 2>/dev/null | tr '\n' ' ')
  if [ -n "$(echo "$left" | tr -d ' ')" ]; then
    echo "${C_YELLOW}sweep INCOMPLETE${C_RESET}: still present after teardown: $left" >&2
    echo "  remove them before the next run: docker rm -f $left" >&2
    return 1
  fi
  return 0
}
