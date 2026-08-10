#!/usr/bin/env bash
# integration/lib.sh — shared helpers for the field-test suites and the run-all
# driver. SOURCED, never executed. It keeps the common shell out of every
# suite's run.sh; today the run-all driver is the primary consumer, and the
# suites can adopt these helpers incrementally (they still stand alone).
#
# The convention every suite already follows, which these helpers key off:
#   • a suite prints a final line  RESULT: PASS|FAIL|FINDING <text>
#   • and exits 0 on pass (or a deliberately-reproduced FINDING), non-zero on fail
#   • repair/relay/commit events are asserted from <store>/debug.log, not stdout
#
# shellcheck disable=SC2034  # colors are used by sourcing scripts

# ---- pretty output ---------------------------------------------------------
if [ -t 1 ]; then
  C_GREEN=$'\033[32m'; C_RED=$'\033[31m'; C_YELLOW=$'\033[33m'; C_DIM=$'\033[2m'; C_RESET=$'\033[0m'
else
  C_GREEN=""; C_RED=""; C_YELLOW=""; C_DIM=""; C_RESET=""
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

# ft_classify <exit_code> <result_line> — PASS | FINDING | FAIL.
# A non-zero exit is always FAIL. On exit 0 the RESULT word decides. A suite that
# exits 0 with NO RESULT line — or an unrecognized one — is a FAIL, not a PASS:
# the suites run `set -uo pipefail` (not `-e`), so one that dies silently after
# its setup can still exit 0 without ever printing a verdict. Every real suite
# prints a RESULT line on its happy path, so a missing verdict means a broken run,
# and defaulting it to PASS would fake green (field-test immutable #4).
ft_classify() {
  local rc="$1" line="$2"
  if [ "$rc" -ne 0 ]; then echo FAIL; return; fi
  case "$line" in
    *[Rr]"ESULT: PASS"*)    echo PASS ;;
    *[Rr]"ESULT: FINDING"*) echo FINDING ;;
    *[Rr]"ESULT: FAIL"*)    echo FAIL ;;
    "")                     echo FAIL ;;   # exit 0 but no verdict = broken run
    *)                      echo FAIL ;;   # exit 0 with an unrecognized line = broken run
  esac
}

# ft_status_color <PASS|FINDING|FAIL> — echoes the matching color escape.
ft_status_color() {
  case "$1" in
    PASS)    printf '%s' "$C_GREEN" ;;
    FINDING) printf '%s' "$C_YELLOW" ;;
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
