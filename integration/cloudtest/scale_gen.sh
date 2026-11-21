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

now="$(date +%s)"
[ "$start_at" -gt "$now" ] && sleep $(( start_at - now ))
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
