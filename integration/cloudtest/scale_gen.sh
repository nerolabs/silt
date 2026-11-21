#!/usr/bin/env bash
# scale_gen.sh — a paced publish load generator that runs ON a client node.
#
#   scale_gen.sh INTERVAL_MS DURATION_S MAX_INFLIGHT OUT PEERS REGREF TOKEN_QUORUM [START_AT]
#
# Starts one `silt swarm add` of a fresh 8 KiB object every INTERVAL_MS for
# DURATION_S, on a fixed schedule: the i-th publish is due at t0 + i*INTERVAL, so a
# slow publish never delays the next one and the offered rate is the scheduled
# rate, not whatever the client manages to sustain. Each publish is its own
# process with its own ephemeral identity, as independent publishers are.
# SCALE_SILT names the silt binary (default /usr/local/bin/silt).
# START_AT (Unix seconds) holds the schedule until then, so generators launched one
# after another over SSH still share one window.
#
# OUT receives one line per event (times are Unix nanoseconds):
#   pub  IDX START END RC ok|fail DETAIL LEGS REG
#                                          a publish finished; ok means a link came back.
#                                          LEGS is the client's `publish elapsed:` line as
#                                          join=0.312,issuers=0.101,…,total=6.004, or -;
#                                          REG the registry that accepted the entry, or -
#   skip IDX AT                            MAX_INFLIGHT publishes were still running when
#                                          IDX was due, so it was not started
#   done ATTEMPTED AT                      the schedule ended and every publish finished
#   cpu  AT BUSY STEAL                     every 5 s: the host's CPU busy and steal over the
#                                          last 5 s, in percent, so a grade can tell a
#                                          saturated generator from a saturated network
# Lines are short single writes to an O_APPEND file, so concurrent publishers do
# not interleave within a line.
set -u
iv_ns=$(( $1 * 1000000 )); dur_ns=$(( $2 * 1000000000 )); cap="$3"; out="$4"
peers="$5"; reg="$6"; tq="$7"; start_at="${8:-0}"
silt="${SCALE_SILT:-/usr/local/bin/silt}"
dir="$(mktemp -d)"
: > "$out"

one() {
  local i="$1" f s e rc o link detail legs took
  f="$dir/$i.bin"
  s="$(date +%s%N)"
  head -c 8192 </dev/urandom >"$f"
  o="$("$silt" swarm add "$f" -peers "$peers" -registry "$reg" -token-quorum "$tq" -chunk-size 65536 2>&1)"; rc=$?
  e="$(date +%s%N)"
  rm -f "$f"
  link="$(printf '%s' "$o" | grep -oE 'silt:v1:[^[:space:]]+' | head -1)"
  legs="$(printf '%s\n' "$o" | sed -n 's/^publish elapsed: //p' | tail -1 |
    awk '{ for (j = 1; j < NF; j += 2) { v = $(j + 1); sub(/s$/, "", v); printf "%s%s=%s", (j > 1 ? "," : ""), $j, v } }')"
  took="$(printf '%s\n' "$o" | sed -n 's/^publish accepted by registry //p' | tail -1)"
  if [ -n "$link" ]; then
    echo "pub $i $s $e $rc ok - ${legs:--} ${took:--}" >>"$out"
  else
    detail="$(printf '%s' "$o" | grep -vE '^[[:space:]]*$' | tail -1 | tr -s ' \t' '_' | cut -c1-200)"
    echo "pub $i $s $e $rc fail ${detail:--} ${legs:--} ${took:--}" >>"$out"
  fi
}

# cpu_sample reads /proc/stat's aggregate line every 5 s. Busy is everything but
# idle and iowait, steal included: time the hypervisor withheld is time the
# generator wanted and did not get. It is disowned, so the in-flight count below
# never counts it.
cpu_sample() {
  local a b
  a="$(head -1 /proc/stat)"
  while sleep 5; do
    b="$(head -1 /proc/stat)"
    printf '%s\n%s\n' "$a" "$b" | awk -v at="$(date +%s%N)" '
      { t = 0; for (j = 2; j <= NF; j++) t += $j; tot[NR] = t; idle[NR] = $5 + $6; st[NR] = $9 }
      END { d = tot[2] - tot[1]; if (d > 0) printf "cpu %s %.1f %.1f\n", at, 100 * (d - (idle[2] - idle[1])) / d, 100 * (st[2] - st[1]) / d }' >>"$out"
    a="$b"
  done
}

now="$(date +%s)"
[ "$start_at" -gt "$now" ] && sleep $(( start_at - now ))
if [ -r /proc/stat ]; then
  cpu_sample & cpu_pid=$!; disown "$cpu_pid"
  trap 'kill "$cpu_pid" 2>/dev/null' EXIT
fi
t0="$(date +%s%N)"; i=0
while :; do
  due=$(( t0 + i * iv_ns ))
  [ $(( due - t0 )) -ge "$dur_ns" ] && break
  now="$(date +%s%N)"
  [ "$due" -gt "$now" ] && sleep "$(awk -v n=$(( due - now )) 'BEGIN { printf "%.3f", n / 1e9 }')"
  if [ "$(jobs -rp | wc -l)" -ge "$cap" ]; then
    echo "skip $i $(date +%s%N)" >>"$out"
  else
    one "$i" &
  fi
  i=$(( i + 1 ))
done
wait
echo "done $i $(date +%s%N)" >>"$out"
rm -rf "$dir"
