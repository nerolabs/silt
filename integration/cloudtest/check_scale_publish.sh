#!/usr/bin/env bash
# check_scale_publish.sh — offline RED/GREEN for the publish-throughput flow's two
# halves: the paced generator (scale_gen.sh) against a stub silt, and the step
# grade (scale_grade.py) against generator logs and commit lines it is handed.
# The properties: the generator offers the scheduled rate and logs every publish;
# a full in-flight cap shows up as skips, never as a silently slower rate; and a
# step passes only when every publish committed fast enough, so a skip, a failure,
# an unfinished generator or a slow p95 each fail it. Runs in seconds; no network.
# Exit 0 = GREEN.
set -uo pipefail
cd "$(dirname "$0")"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

# GNU date carries %N; on macOS it is gdate.
if ! date +%N | grep -qE '^[0-9]+$'; then
  command -v gdate >/dev/null || { echo "check_scale_publish: needs GNU date (coreutils)"; exit 1; }
  mkdir -p "$tmp/bin"; ln -s "$(command -v gdate)" "$tmp/bin/date"; export PATH="$tmp/bin:$PATH"
fi

# A stub silt: sleeps STUB_SLEEP, then prints a link, or an error for the index in
# the object's name when STUB_FAIL_EVERY divides it. Either way it prints its legs
# first, as the client does, so the error stays the last line.
cat > "$tmp/silt" <<'EOF'
#!/usr/bin/env bash
sleep "${STUB_SLEEP:-0.05}"
i="$(basename "$3" .bin)"
if [ "${STUB_FAIL_EVERY:-0}" -gt 0 ] && [ $(( i % STUB_FAIL_EVERY )) -eq 0 ]; then
  echo "publish elapsed: join 0.010s total 0.010s" >&2
  echo "swarm add: could not gather enough publish-token signatures" >&2; exit 1
fi
echo "publish elapsed: join 0.010s place 0.020s commit 0.030s total 0.060s" >&2
echo "publish accepted by registry https://10.0.0.$(( i % 2 + 1 )):8443" >&2
echo "published silt:v1:stub$i"
EOF
chmod +x "$tmp/silt"
export SCALE_SILT="$tmp/silt"

fail=0
expect() { if [ "$2" = "$3" ]; then echo "  ok   $1"; else echo "  FAIL $1: want $2, got $3"; fail=1; fi; }
count() { grep -c "^$1 " "$2" | tr -d ' '; }
field() { python3 -c "import json,sys;print(json.loads(open(sys.argv[1]).read())[sys.argv[2]])" "$1" "$2"; }

# 1. 20 publishes/s for 2 s: forty attempts, all logged, all ok, schedule finished.
./scale_gen.sh 50 2 64 "$tmp/g1.log" peers reg 2
expect "a 2 s schedule at 20/s attempts 40 publishes" 40 "$(awk '/^done/ {print $2}' "$tmp/g1.log")"
expect "every attempted publish is logged" 40 "$(count pub "$tmp/g1.log")"
expect "every stub publish returned a link" 40 "$(grep -c '^pub .* ok ' "$tmp/g1.log" | tr -d ' ')"
expect "a publish line carries the client's legs" 40 \
  "$(grep -c '^pub .* ok - join=0.010,place=0.020,commit=0.030,total=0.060 https://10.0.0.[12]:8443$' "$tmp/g1.log" | tr -d ' ')"

# 2. A full in-flight cap: publishes that take 1 s at 20/s with room for two in flight.
STUB_SLEEP=1 ./scale_gen.sh 50 1 2 "$tmp/g2.log" peers reg 2
skips="$(count skip "$tmp/g2.log")"
expect "a full in-flight cap is logged as skips" yes "$([ "${skips:-0}" -gt 0 ] && echo yes || echo no)"
expect "skips and publishes account for every attempt" "$(awk '/^done/ {print $2}' "$tmp/g2.log")" \
  "$(( $(count pub "$tmp/g2.log") + skips ))"

# 3. Failures are logged with the client's last error line.
STUB_FAIL_EVERY=4 ./scale_gen.sh 50 1 64 "$tmp/g3.log" peers reg 2
expect "a failed publish is logged as fail" 5 "$(grep -c '^pub .* fail ' "$tmp/g3.log" | tr -d ' ')"
expect "a failure carries the client's error" yes \
  "$(grep -q 'publish-token_signatures' "$tmp/g3.log" && echo yes || echo no)"

# The grade. Commit lines as journalctl -o short-unix prints them; a block a second
# from t=1000, two entries each.
start=1000
for h in $(seq 1 30); do
  printf '%s.250000 host silt[1]: chain: committed block %d (2 entries, 0 bond-regs, 3 attestations)\n' $(( start + h )) "$h"
done > "$tmp/commits.txt"
grade() { python3 ./scale_grade.py --label t --rate 20 --start "$start" --dur 30 --p95 "$1" --min-ok "${MIN_OK:-0.99}" \
  --commits "$tmp/commits.txt" --rounds 0 --held 0 "${@:2}" > "$tmp/row.json"; }

grade 20 "$tmp/g1.log"
expect "a step with every publish committed fast passes" pass "$(field "$tmp/row.json" verdict)"
expect "blocks in the window are counted" 29 "$(field "$tmp/row.json" blocks)"
expect "entries per block come from the commit lines" 2.0 "$(field "$tmp/row.json" entries_per_block)"
expect "the block gap is measured" 1.0 "$(field "$tmp/row.json" block_gap_p50_s)"
expect "legs are read from every committed publish" 40 "$(field "$tmp/row.json" legs_n)"
expect "committed publishes are counted per accepting registry" "{'https://10.0.0.1:8443': 20, 'https://10.0.0.2:8443': 20}" \
  "$(field "$tmp/row.json" accepted_by)"
expect "a leg's p50 is reported" 0.03 "$(python3 -c "import json,sys;print(json.load(open(sys.argv[1]))['legs_p50_s']['commit'])" "$tmp/row.json")"
expect "time outside the client is the generator's time less its total" yes \
  "$(python3 -c "import json,sys;print('yes' if json.load(open(sys.argv[1]))['legs_p50_s']['outside'] > 0 else 'no')" "$tmp/row.json")"

grade 0.01 "$tmp/g1.log"
expect "a p95 over the bound fails the step" fail "$(field "$tmp/row.json" verdict)"

# With no floor on the committed fraction, the skips alone must fail it.
MIN_OK=0 grade 20 "$tmp/g1.log" "$tmp/g2.log"
expect "a skipped publish fails the step" fail "$(field "$tmp/row.json" verdict)"

grade 20 "$tmp/g3.log"
expect "failed publishes below the bound fail the step" fail "$(field "$tmp/row.json" verdict)"

grep -v '^done' "$tmp/g1.log" > "$tmp/g4.log"
grade 20 "$tmp/g4.log"
expect "a generator that never finished fails the step" fail "$(field "$tmp/row.json" verdict)"

head -n 30 "$tmp/g1.log" > "$tmp/g5.log"; echo "done 40 0" >> "$tmp/g5.log"
MIN_OK=0 grade 20 "$tmp/g5.log"
expect "publishes still running at the deadline are counted" 10 "$(field "$tmp/row.json" pending)"
expect "publishes still running at the deadline fail the step" fail "$(field "$tmp/row.json" verdict)"

if [ "$fail" = 0 ]; then echo "scale-publish self-test GREEN"; else echo "scale-publish self-test RED"; exit 1; fi
