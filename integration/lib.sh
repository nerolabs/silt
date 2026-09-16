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

# ---- the declared floor spec, measured from inside a container --------------
# silt claims a validator runs on ONE core, 2 GiB of RAM and 10 GiB of disk, and
# that it stays under that memory ceiling on ADVERSARIAL input. A suite that
# wants to measure a seat against the claim has to do two things, and these
# helpers are the two:
#
#  • Prove the kernel is ENFORCING the spec. `mem_limit` that silently did not
#    bind — an older compose schema, a swarm-mode deploy block, a daemon without
#    cgroup v2 — turns every later number into a measurement of an ordinary box,
#    and the green that follows is worth nothing. Read the limits from INSIDE
#    the container, never from the compose file that asked for them.
#  • Read `memory.peak`, the cgroup's own high-water mark since the container
#    started. It is the run's maximum rather than a sample, so a spike between
#    two polls cannot hide in it. It lives with the container's cgroup, so read
#    it BEFORE the service is stopped or removed.
#
# `cpuset` and not a CPU quota: a quota still reports every host core to `nproc`,
# so the Go runtime sizes GOMAXPROCS and its worker pools for a multi-core box
# and the node under test is not the node that ships. `memswap_limit` equal to
# `mem_limit` means no swap at all, so crossing the ceiling is a clean OOM-kill
# (exit 137) a harness can read instead of a thrash it would have to guess about.
#
# A caution for a topology that pins more than one seat: the container ceilings
# over-subscribe the docker VM freely (a node's real use is a couple of hundred
# MiB), but a seat that genuinely climbed toward 2 GiB would exhaust the VM
# before its own cgroup, and the guest OOM-killer would land on whichever
# container it chose. A peak near the ceiling is a finding to instrument, not a
# number to compare against the VM's size.

# ft_iec_bytes <2g|1500m|512k|1048576> — the suffixes docker's mem_limit accepts
# (b/k/m/g, case insensitive, bare number = bytes) as a byte count, so a guard
# compares what the kernel REPORTS against what the compose file ASKED for
# instead of hardcoding one of them. `numfmt` is GNU coreutils and is not on a
# stock macOS, so this is pure shell by necessity.
ft_iec_bytes() {
  local v="$1" n u
  n=$(printf '%s' "$v" | tr -d '[:alpha:]')
  u=$(printf '%s' "$v" | tr -d '[:digit:]' | tr '[:upper:]' '[:lower:]')
  case "$n" in ''|*[!0-9]*) echo 0; return 1 ;; esac
  case "$u" in
    g|gb|gi|gib) echo $(( n * 1073741824 )) ;;
    m|mb|mi|mib) echo $(( n * 1048576 )) ;;
    k|kb|ki|kib) echo $(( n * 1024 )) ;;
    ''|b)        echo "$n" ;;
    *)           echo 0; return 1 ;;
  esac
}

# ft_human_bytes <bytes> — a byte count in IEC units, for report lines.
ft_human_bytes() {
  local b="$1"
  case "$b" in ''|*[!0-9]*) echo "?"; return ;; esac
  if   [ "$b" -ge 1073741824 ]; then awk -v b="$b" 'BEGIN{printf "%.2f GiB", b/1073741824}'
  elif [ "$b" -ge 1048576 ];    then awk -v b="$b" 'BEGIN{printf "%.1f MiB", b/1048576}'
  elif [ "$b" -ge 1024 ];       then awk -v b="$b" 'BEGIN{printf "%.1f KiB", b/1024}'
  else echo "${b}B"; fi
}

# ft_cgroup <project> <service> <file> — one cgroup v2 file, read inside the
# container. Empty when the container is gone or the file does not exist; every
# caller treats empty as "no measurement", which is a failure and not a pass.
ft_cgroup() {
  docker compose -p "$1" exec -T "$2" sh -c "cat /sys/fs/cgroup/$3 2>/dev/null" 2>/dev/null | tr -d '\r\n'
}

# ft_spec_binds <project> <service> <mem> <cpus> — THE VACUITY GUARD. Prints the
# three facts it read and returns non-zero, with the reason, if any of them did
# not bind. Call it before believing any number measured on that service.
#
# It retries: a service is reachable by `exec` only once its container is
# running, and compose returns from `up -d` before that is necessarily true.
ft_spec_binds() {
  local project="$1" svc="$2" mem="$3" cpus="$4"
  local want got_mem got_swap got_cpus i
  want=$(ft_iec_bytes "$mem") || { echo "  '$mem' is not a size docker would accept (try 2g, 1500m)" >&2; return 1; }

  for i in 1 2 3 4 5 6 7 8 9 10; do
    got_mem=$(ft_cgroup "$project" "$svc" memory.max)
    [ -n "$got_mem" ] && break
    sleep 2
  done
  got_swap=$(ft_cgroup "$project" "$svc" memory.swap.max)
  got_cpus=$(docker compose -p "$project" exec -T "$svc" sh -c 'nproc' 2>/dev/null | tr -d '\r\n')

  echo "  ${svc}: memory.max      = ${got_mem:-<unreadable>} (want ${want})"
  echo "  ${svc}: memory.swap.max = ${got_swap:-<unreadable>} (want 0 — no swap to escape into)"
  echo "  ${svc}: nproc           = ${got_cpus:-<unreadable>} (want ${cpus})"

  [ "$got_mem" = "$want" ] || {
    echo "  the memory ceiling did NOT bind on ${svc}: memory.max is '${got_mem:-unreadable}', not ${want}. Any peak measured here would be a measurement of an ordinary box." >&2; return 1; }
  [ "$got_swap" = "0" ] || {
    echo "  ${svc} has swap (memory.swap.max='${got_swap:-unreadable}'): it can exceed its RAM ceiling by thrashing, so 'stayed under the ceiling' would not mean what it says." >&2; return 1; }
  [ "$got_cpus" = "$cpus" ] || {
    echo "  ${svc} sees nproc='${got_cpus:-unreadable}', not ${cpus}: the Go runtime is sizing for a wider box than the floor spec, so this is not the node we ship." >&2; return 1; }
  return 0
}

# ft_peak_bytes <project> <service> — memory.peak, the high-water mark. Empty if
# it could not be read, which the caller must treat as a missing measurement.
ft_peak_bytes() { ft_cgroup "$1" "$2" memory.peak; }

# ft_oom_killed <project> <service> — "true" when the kernel OOM-killed the
# container. Under a memory cgroup with no swap that is what exceeding the
# ceiling looks like, and it is a different failure from a slow or dead node.
ft_oom_killed() {
  local cid
  cid=$(docker compose -p "$1" ps -aq "$2" 2>/dev/null | head -1)
  [ -n "$cid" ] || { echo "unknown"; return; }
  docker inspect -f '{{.State.OOMKilled}}' "$cid" 2>/dev/null | tr -d '\r\n'
}

# ft_peak_report <peak> <mem> — one line: the peak, the ceiling it is measured
# against, the percentage and the headroom left. A regression then surfaces as a
# shrinking margin and not only as an eventual failure.
ft_peak_report() {
  local peak="$1" want
  want=$(ft_iec_bytes "$2")
  case "$peak" in ''|*[!0-9]*) echo "no measurement"; return 1 ;; esac
  printf '%s of %s (%d%%, %s of headroom)' \
    "$(ft_human_bytes "$peak")" "$(ft_human_bytes "$want")" \
    "$(( peak * 100 / want ))" "$(ft_human_bytes "$(( want - peak ))")"
  [ "$peak" -lt "$want" ]
}
