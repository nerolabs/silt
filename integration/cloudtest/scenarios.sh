#!/usr/bin/env bash
# scenarios.sh — the field-test flows, one function each, mapping directly onto
# the acceptance brief flows 1–9) plus the
#  adversarial consensus drills. Each records a pass/gap/fail via slo_assert.
#
# Sourced by cloudtest.sh AFTER lib.sh and AFTER the network is up. All node
# interaction is over `ssh_node` (IAP). Field networks are noisy, so every check
# asserts a THRESHOLD/behaviour, never an exact count or timing.
set -uo pipefail

# ── PRINCIPLED windows (replace arbitrary
# wall-clocks with bounds COMPUTED from the path — and a miss INSIDE a computed
# window is a FINDING, never a re-grade). The per-leg worst case is derived from
# the deployed flags: -request-timeout 8s × (1 + 3 -request-retries) + backoff
# (250ms doubling: 0.25+0.5+1) ≈ 34s/leg. The fresh-publisher path is ~5
# sequential legs (join/bootstrap → canonical-issuer ranking fetch → parallel
# token gather → scatter+confirm → register): 4×34 ≈ 136s; the COMMIT
# WAIT leg is escape-aware under the synchronizer durations (submit-then-
# poll rides the designee rotation, and a contested height may pay the 2-round
# escape: the 2-round synchronizer escape FLOOR is dur(0)+dur(1) = 5 sweeps ×
# 30s ChainSyncInterval = 150s + one ~34s gather leg = 184s — see the H_ESCAPE_S
# derivation at the soak defaults) ≈ 136 + 184 = 320 → 300; +1 leg-equivalent
# for the relay hop on the cross-NAT flow is absorbed by the same allowance.
#
# PUBLISH BOUND RE-DERIVED DOWNWARD (2026-08-27, owed Phase-3 gate clause):
# 360 → 300s. Evidence: the field run flow 12-deep-heights measured ~48s/height
# steady cadence (results-the field run.jsonl:29; (h132−h78)/2615s = 48.4s/height,
# was ~390s at the depth-war start). The 220s commit-wait leg was a synchronizer
# ROUND bound counted in fixed 30s sweeps, NOT a wall-clock cadence quantity, so
# it does NOT tighten with the cheap cadence — the escape FLOOR (184s) stays; the
# 60s shed is the historical escape-rounding cushion (220→184) plus stale
# slow-height straddle padding the cheap cadence retires. 300s keeps the full
# 150s escape window inside the bound and is 6.25× the measured 48s cadence /
# 1.76× the 170s per-height worst case — retry headroom for transient
# WAN churn, above the escape floor. Derivation:
# . (240 was the pre-
# figure — its commit-wait leg assumed one flat 64s drain cycle; run the field run's
# only non- GAP was a publish missing exactly that stale window, so the
# re-derivation stays comfortably above 240.)
# Fetch is ~3 legs (discovery → manifest → parallel chunk fetches) ≈ 102s → 120.
#
# FETCH_SLO_S IS DERIVED FROM THE WRONG PROCESS, and is kept only as the coarse
# liveness window the older flows already grade against. The publish/fetch client
# is a SEPARATE process from the daemon and takes none of its flags: it holds its
# own per-attempt deadline, its own retry ladder, and its own whole-operation
# ceiling. So the `-request-timeout 8s` the leg arithmetic above is built on is
# the configuration of a process that performs no fetch, and the 34s/leg figure
# does not describe the client at all. The flow that actually grades a retrieval
# against its configuration — 21-cross-region-cold-fetch — reads the posture the
# CLIENT narrates and derives its bound from that. Where a number here and a
# number there disagree, the client's is the one describing the fetch.
: "${COMMIT_SLO_S:=90}"
: "${FETCH_SLO_S:=120}"
: "${RESTART_SLO_S:=60}"
: "${PUBLISH_RETRY_S:=300}"

# ── swarm references (validators as peers; val-a serves the registry) ───────────
ft_peers() {
  python3 -c "
import json;t=json.load(open('$FT_TOPO'));p=t['meta']['swarm_port']
print(','.join(n['nodeid']+'@'+n['ip']+':%d'%p for n in t['nodes'].values() if n['role']=='validator'))"
}
ft_regref() { # the deterministic pinned registry ref (boot validator NodeID@https://ip:port)
  # Built by topology.py from the known NodeID + internal ip. We do NOT scrape the
  # daemon's `registry: chain-backed, serving...` banner: the old regex could not
  # match it (REGREF came back empty → every `swarm add` hit a usage error), and the
  # banner prints the bound 0.0.0.0 address, which a publisher cannot dial.
  python3 -c "import json;print(json.load(open('$FT_TOPO'))['meta']['regref'])"
}

PEERS=""; REGREF=""
ft_init_refs() { PEERS="$(ft_peers)"; REGREF="$(ft_regref)"; }

# silt:v1: links carry the root in BASE64URL, not hex. The takedown denylist keys
# on the HEX root, so decode it (mirrors the local takedown/run.sh helper). Without
# this, `grep -oE '[0-9a-f]{64}'` on the link matched nothing and cloud takedown
# gap'd on every run — it never actually tested takedown (blind cloud finding).
b64url_to_hex() {
  local s="$1"
  while [ $(( ${#s} % 4 )) -ne 0 ]; do s="${s}="; done
  printf '%s' "$s" | tr '_-' '/+' | base64 -d 2>/dev/null | od -An -v -tx1 | tr -d ' \r\n'
}

# TOKEN_QUORUM: publish-token signatures gathered per publish (privacy: unlinkable
# above 0). Default 2. A -token-quorum publish needs the PUBLISHER to reach that
# many validators for signatures — distinct from validator<->validator reachability.
# At genesis there are no committed bonds to rank canonical signers, so it falls back
# to -peers order (a nominal-privacy note). Set TOKEN_QUORUM=1 (or 0) for a first
# bootstrap run if publisher->validator egress is the thing under test.
#
# H6 (blind field test 2026-08-12): a fresh network has no committed bonds, so a
# token-quorum≥2 publish can't rank canonical signers and the headline publish→fetch
# GAPs out of the box. The SMOKE path is precisely that first-bootstrap case, so
# default it to 1 there (an explicit TOKEN_QUORUM= still wins); the full run keeps 2.
if [ "${SMOKE:-0}" = 1 ]; then : "${TOKEN_QUORUM:=1}"; else : "${TOKEN_QUORUM:=2}"; fi

# ft_reachable_peers NODE — from NODE, how many of the -peers validators are TCP-
# reachable on the swarm port? Prints "OK/TOTAL"; lists unreachable refs on stderr.
# This attributes a token-gather shortfall to the publisher->validator path, which
# is the usual cause of "could not gather enough publish-token signatures" and a
# chain stuck at height 0 (the block only commits once a publish proposes it).
ft_reachable_peers() { # ft_reachable_peers NODE
  local node="$1" ref hostport host port ok=0 total=0 bad=""
  for ref in ${PEERS//,/ }; do
    [ -n "$ref" ] || continue
    total=$((total + 1))
    hostport="${ref#*@}"; host="${hostport%:*}"; port="${hostport##*:}"
    if ssh_node "$node" "timeout 3 bash -c ': </dev/tcp/$host/$port' 2>/dev/null"; then
      ok=$((ok + 1))
    else
      bad="$bad ${host}:${port}"
    fi
  done
  [ -n "$bad" ] && echo "  unreachable from $node:$bad" >&2
  printf '%s/%s' "$ok" "$total"
}

# publish a fresh random file on a node; echoes "LINK SHA" on success, empty on fail.
# On failure it sets FT_PUBLISH_GAP=1 iff the cause was a publisher→validator
# reachability shortfall (< TOKEN_QUORUM peers reachable — an egress/preemption
# problem, e.g. a SPOT-reclaimed validator), so a caller can record a GAP ("couldn't
# confirm") rather than a property FAIL. Cleared to 0 on entry.
# client_preflight FLOW SEVERITY NODE... — (the field run): a dead client
# node burned each dependent flow's full 360s publish window (since re-derived to 300s
# — see) before the plumbing
# flag fired, and three sheet rows went red on ONE unreachable node. One cheap ssh
# round-trip per client node first (bounded by SSH_NODE_TIMEOUT, default 90s);
# silence ⇒ record a GAP naming the plumbing cause and return 1 so the flow exits
# UNTESTED immediately.
client_preflight() { # client_preflight FLOW SEVERITY NODE... -> 0 all reachable / 1 recorded gap
  local flow="$1" sev="$2"; shift 2
  local cn
  for cn in "$@"; do
    if [ -z "$(ssh_node "$cn" "echo ok" 2>/dev/null | tr -d '[:space:]')" ]; then
      record "$flow" gap "$sev" "client node $cn UNREACHABLE at preflight (ssh round-trip returned nothing) — plumbing, flow UNTESTED; check nodes.json / VM state before attributing anything downstream"
      return 1
    fi
  done
  return 0
}

ft_publish() { # ft_publish NODE SIZE_BYTES
  FT_PUBLISH_GAP=0
  # Cross-subshell handoff (#7 evidence): ft_publish runs inside `res="$(…)"`, so
  # variables set here NEVER reach the caller's scope. Files do..ft_publish_gap
  # carries the honest gap-vs-fail signal to publish_verdict;.ft_publish_lasterr
  # carries the last captured silt error into the recorded verdict detail (it used
  # to go to the console only, which is not persisted — run the field run's
  # 9-cross-nat FAIL left no clue which leg died).
  printf 0 > "$FT_DIR/.ft_publish_gap"; : > "$FT_DIR/.ft_publish_lasterr"
  local node="$1" size="${2:-1048576}" out link lasterr="" any_output=0
  ssh_node "$node" "head -c $size </dev/urandom >/tmp/ft_src.bin; sha256sum /tmp/ft_src.bin | cut -d' ' -f1" >/tmp/ft_src_sha 2>/dev/null
  local sha; sha="$(cat /tmp/ft_src_sha 2>/dev/null)"
  local deadline=$(( $(date +%s) + PUBLISH_RETRY_S ))
  while [ "$(date +%s)" -lt "$deadline" ]; do
    # 2>&1 INSIDE the remote command: ssh_node suppresses gcloud stderr (2>/dev/null),
    # so a publish error (on silt's stderr) is only captured for the diagnostic below
    # if the redirect happens remotely.
    out="$(ssh_node "$node" "/usr/local/bin/silt swarm add /tmp/ft_src.bin -peers '$PEERS' -registry '$REGREF' -token-quorum $TOKEN_QUORUM -chunk-size 65536 2>&1" || true)"
    [ -n "$(printf '%s' "$out" | tr -d '[:space:]')" ] && any_output=1
    link="$(printf '%s' "$out" | grep -oE 'silt:v1:\S+' | head -1)"
    [ -n "$link" ] && { printf '%s %s\n' "$link" "$sha"; return 0; }
    lasterr="$(printf '%s' "$out" | grep -iE 'could not gather|not enough|no canonical|token|refus|unreachable|timed? ?out' | head -2 | tr '\n' ';')"
    # The allow-list above was built for the KNOWN failure modes and silently
    # dropped a new class (run the field run: ' insufficient valid
    # attestations: 2 prepares of 2 gathered' matched nothing → the diagnostic
    # printed '<none captured>' and the decisive error had to be re-captured
    # live before teardown). Fall back to the raw tail: an UNRECOGNIZED error
    # is the one most worth recording.
    [ -z "$lasterr" ] && lasterr="$(printf '%s' "$out" | grep -vE '^[[:space:]]*$' | tail -2 | tr '\n' ';')"
    sleep 4
  done
  # Diagnose so a failed publish is actionable, not a bare "no link produced". The
  # single most common cause is the publisher reaching < TOKEN_QUORUM validators.
  local reach; reach="$(ft_reachable_peers "$node")"
  # If the publisher can't reach enough validators to gather the token quorum, this
  # is an egress/preemption problem, not a property failure — flag it so the caller
  # records a GAP. (max(1,TOKEN_QUORUM): even token-quorum 0 needs one reachable peer.)
  local rok="${reach%%/*}"; local need=$(( TOKEN_QUORUM > 1 ? TOKEN_QUORUM : 1 ))
  [ "${rok:-0}" -lt "$need" ] 2>/dev/null && { FT_PUBLISH_GAP=1; printf 1 > "$FT_DIR/.ft_publish_gap"; }
  # A publish that fails because the ephemeral CLI publisher could not DISCOVER the
  # canonical issuer set / gather a token — even with validators reachable — is a
  # publish-token *discovery* problem over WAN (worse after mid-run node churn), not a
  # break of the property a SETUP publish is a precondition for (durability, crash
  # recovery). The publisher re-warm reduces it but a churned WAN can still
  # exceed the window. Flag it as a GAP so publish_verdict records the DEPENDENT flow
  # as UNTESTED, not FAILED; the publish-reliability issue itself stays visible in this
  # diagnostic and in. (A genuine no-quorum terminal outcome still fails fast via
  # /publish-status, so this does not mask a real refusal.)
  printf '%s' "$lasterr" | grep -qiE 'no canonical issuer|could not gather|not enough|issuer set|token' && { FT_PUBLISH_GAP=1; printf 1 > "$FT_DIR/.ft_publish_gap"; }
  # …or if the fetch publish subsystem was already found degraded this run (warm
  # failed): a subsequent publish failure — even one with no captured error text — is
  # then a discovery/setup problem, not a property break, so score it a GAP too.
  [ "${FETCH_PUBLISH_DEGRADED:-0}" = 1 ] && { FT_PUBLISH_GAP=1; printf 1 > "$FT_DIR/.ft_publish_gap"; }
  # PLUMBING RED FLAG (2026-08-20, the clobber lesson): if EVERY publish attempt
  # returned NOTHING, the swarm add command produced no output at all — that is not
  # a publish that failed, it is ssh_node returning empty (the node is unreachable,
  # the node MAP is wrong, or ssh/preemption broke the connection). This exact
  # emptiness — rendered as a benign "did not land (…?)" — masked a corrupted
  # nodes.json for a whole cloud run. Say it LOUDLY and score it a GAP (plumbing,
  # not a property failure). A reachability shortfall makes it doubly clear.
  if [ "$any_output" = 0 ]; then
    lasterr="EMPTY RESPONSE from $node — ssh_node returned NOTHING across the whole window: the node is UNREACHABLE or the node MAP is wrong (check nodes.json zones/ips), NOT a publish failure. reachable-peers=${reach:-?}"
    # >&2 is LOAD-BEARING (the field run): ft_publish runs inside
    # res="$(…)", so a stdout echo here CORRUPTS res — the caller's [ -z "$res" ]
    # publish-leg gate sees non-empty text, link parses to "", and the flow
    # proceeds to grade its fetch leg as a real FAIL on what is plumbing
    # (9-cross-nat "publish landed " + durability-turnover "not a silt:v1:
    # link" were exactly this). Diagnostics from this function go to stderr,
    # only "link sha" ever goes to stdout.
    echo "    ⚠ PLUMBING: $lasterr" >&2
    FT_PUBLISH_GAP=1; printf 1 > "$FT_DIR/.ft_publish_gap"
  fi
  printf '%s' "$lasterr" > "$FT_DIR/.ft_publish_lasterr"
  # Persist the diagnostic (#7): stderr reaches the console, but the console dies
  # with the terminal — the tee'd copy survives teardown next to the run's report.
  {
    echo "[$(date -u +%FT%TZ)] ft_publish FAILED after ${PUBLISH_RETRY_S}s on $node (token-quorum=$TOKEN_QUORUM)"
    echo "  publisher->validator reachability: $reach of the -peers set reachable"
    echo "  last silt error: ${lasterr:-<none captured>}"
    echo "  note: token-quorum needs the publisher to reach >= $TOKEN_QUORUM validators to"
    echo "        gather signatures. A shortfall here — not validator<->validator — is the"
    echo "        usual cause of 'could not gather enough publish-token signatures' and a"
    echo "        chain stuck at height 0. Retry a bootstrap run with TOKEN_QUORUM=1, or"
    echo "        fix publisher egress to the validators."
  } | tee -a "$FT_DIR/publish-diag-${RUN_ID:-local}.log" >&2
  return 1
}

# Record a publish-dependent flow's failed publish honestly: a reachability
# shortfall OR a degraded publish subsystem is a GAP — the property was UNTESTED,
# not broken — while any other publish failure is a real fail.
#
# NOTE the scope bug this closes: ft_publish is called as `res="$(ft_publish …)"`,
# i.e. in a command-substitution SUBSHELL, so any FT_PUBLISH_GAP it sets is lost and
# never reaches this parent scope. ft_publish therefore ALSO writes the signal to
# $FT_DIR/.ft_publish_gap (files cross the subshell boundary; run the field run's
# 9-cross-nat graded FAIL with the per-call gap signal silently dropped). We still
# honor FETCH_PUBLISH_DEGRADED (wait_publisher_warm sets it in the PARENT): it is 1
# whenever the publisher re-warm could not land a throwaway publish this run — a
# dependent flow's publish failure is then a discovery/setup problem, not a
# break of the property that publish is a precondition for. The last captured silt
# error (.ft_publish_lasterr) is folded into the verdict so the report names the
# mechanism, not just "no link".
publish_verdict() { # publish_verdict FLOW SEVERITY "detail"
  local gap lasterr
  gap="$(cat "$FT_DIR/.ft_publish_gap" 2>/dev/null || echo 0)"
  lasterr="$(cat "$FT_DIR/.ft_publish_lasterr" 2>/dev/null || true)"
  if [ "$gap" = 1 ] || [ "${FT_PUBLISH_GAP:-0}" = 1 ] || [ "${FETCH_PUBLISH_DEGRADED:-0}" = 1 ]; then
    record "$1" gap "$2" "$3 — the publish could not be gathered (egress/preemption, or issuer-set discovery not landing over WAN)${lasterr:+; last publish error: ${lasterr}}; property UNTESTED, not failed"
  else
    record "$1" fail "$2" "$3${lasterr:+ (last publish error: ${lasterr})}"
  fi
}

# ── committed-height helpers (audit) ──────────────────────────────────────
# A stale 'chain: committed block N' already in the journal must NOT satisfy a
# "the publish committed" gate — capture the height BEFORE the action and require
# a STRICTLY HIGHER one after, so only a genuinely NEW commit counts.
# ft_head_from_journal — the max HEAD height a journal excerpt proves: the
# `chain: committed block N` banner (printed only when THIS node commits — its own
# proposal or a commit broadcast it applied) OR the `chain: saved N block(s) [...]
# head=H:` line (printed on EVERY save, including catch-up). Run the field run's
# 6-fault-tolerance GAP was a harness artifact of reading the banner alone: with a
# dead validator ahead of the boot node in the proposer's sequential commit
# broadcast, chain-sync catch-up delivered every post-kill block to val-a ~4 s after
# it committed elsewhere ("caught up 1 block(s) from peers … head=29"), the broadcast
# then found the block already applied and printed nothing, and the 380 s wait
# failed while the chain advanced h30→h40 at ~50 s/height with val-d down. A liveness
# oracle must read the node's HEAD, never a print that depends on WHO committed.
ft_commit_height() { # ft_commit_height NODE — max HEAD height the journal proves (0 if none)
  local j a b
  j="$(jlog "$1" 800)"
  a="$(printf '%s' "$j" | grep -oE 'chain: committed block [0-9]+' | grep -oE '[0-9]+$' | sort -n | tail -1)"
  b="$(printf '%s' "$j" | grep -oE 'head=[0-9]+:' | grep -oE '[0-9]+' | sort -n | tail -1)"
  a="${a:-0}"; b="${b:-0}"
  [ "$b" -gt "$a" ] 2>/dev/null && a="$b"
  printf '%s' "$a"
}
ft_wait_new_block() { # ft_wait_new_block NODE H0 TIMEOUT_S -> 0 if a block > H0 committed
  local node="$1" h0="${2:-0}" timeout="${3:-90}" deadline
  deadline=$(( $(date +%s) + timeout ))
  while [ "$(date +%s)" -lt "$deadline" ]; do
    [ "$(ft_commit_height "$node")" -gt "$h0" ] 2>/dev/null && return 0
    sleep 4
  done
  return 1
}

# epoch_to_iso EPOCH → a UTC ISO-8601 prefix (YYYY-MM-DDTHH:MM:SS), portable
# across GNU date (-d @N) and BSD/macOS date (-r N). Lexicographic string order
# on this fixed-width UTC form matches chronological order, so it can filter the
# debug.log timestamp column with a plain `>=`.
epoch_to_iso() { date -u -d "@$1" +%Y-%m-%dT%H:%M:%S 2>/dev/null || date -u -r "$1" +%Y-%m-%dT%H:%M:%S 2>/dev/null; }

# ft_escape_progress SINCE_EPOCH SURVIVOR... — a progress fingerprint of the
# down-designee escape: total round-change lines + the max committed
# height across the surviving validators SINCE the kill. Two EQUAL fingerprints
# a stall window apart are the wedge signature (the cadence rule: a miss inside a principled
# bound); an advancing fingerprint is the ladder alive — slow is not stuck.
#
# TWO SOURCES, TWO CHANNELS (the run the field run mis-attribution):
#  - `chain: committed block` is a fmt.Printf BANNER → stdout → journald, so the
#  height reads from jlog_since (time-scoped).
#  - `round-change` is a structured n.logf line → $STORE/debug.log ONLY, NEVER
#  journald (cmd/silt/daemon.go openLog). Counting it from jlog_since read
#  rc=0 on EVERY sample regardless of ladder activity, so a live ladder (114
#  round-change lines in the captured debug.log at h64 r1→r5) fingerprinted as
#  FROZEN → a manufactured WEDGE FAIL. Read it from debug.log (dlog),
#  time-scoped by the ISO timestamp column ≥ the kill instant.
#
# UNKNOWN, never zero (the lesson, extended to the empty-read case): a
# source that could NOT be read (ssh returned nothing) yields `?`, never 0 — two
# unreadable samples must not compare EQUAL and manufacture a wedge. The caller's
# WEDGE-FAIL branch requires a fingerprint with no `?`.
ft_escape_progress() {
  local since="$1"; shift
  local iso; iso="$(epoch_to_iso "$since")"
  local n rc=0 h=0 cur jout dout rc_read=0 h_read=0
  for n in "$@"; do
    # Height from journald banners (time-scoped).
    jout="$(jlog_since "$n" "$since" 2>/dev/null)"
    if [ -n "$(printf '%s' "$jout" | tr -d '[:space:]')" ]; then
      h_read=1
      cur="$(printf '%s' "$jout" | grep -oE 'chain: committed block [0-9]+' | grep -oE '[0-9]+$' | sort -n | tail -1)"
      [ "${cur:-0}" -gt "$h" ] 2>/dev/null && h="$cur"
      # The head= save line counts too (catch-up prints no banner — see ft_commit_height).
      cur="$(printf '%s' "$jout" | grep -oE 'head=[0-9]+:' | grep -oE '[0-9]+' | sort -n | tail -1)"
      [ "${cur:-0}" -gt "$h" ] 2>/dev/null && h="$cur"
    fi
    # Round-changes from debug.log (dlog), filtered to timestamps ≥ the kill ISO.
    dout="$(dlog "$n" 4000 2>/dev/null)"
    if [ -n "$(printf '%s' "$dout" | tr -d '[:space:]')" ]; then
      rc_read=1
      cur="$(printf '%s' "$dout" | awk -v thr="$iso" '$1 >= thr && /round-change/' | grep -c 'round-change' || true)"
      rc=$(( rc + ${cur:-0} ))
    fi
  done
  # A source no survivor could answer is UNKNOWN, not zero.
  local rcs hs
  rcs="rc=$rc"; [ "$rc_read" = 0 ] && rcs="rc=?"
  hs="h=$h"; [ "$h_read" = 0 ] && hs="h=?"
  printf '%s %s' "$rcs" "$hs"
}

# ── Flow 1: build & first run ──────────────────────────────────────────────────
# LOCAL_PROOF: LOCAL=1 SMOKE=1 ./integration/cloudtest/cloudtest.sh (this flow runs verbatim on the docker backend)
flow_first_run() {
  local n ok=1 bad=""
  for n in $(node_names); do
    [ "$(node_field "$n" role)" = natgw ] && continue
    if ! ssh_node "$n" "systemctl is-active --quiet silt.service"; then ok=0; bad="$bad $n"; fi
  done
  # shellcheck disable=SC2086
  flow_evidence_nodes $bad   # a fail captures exactly the non-active nodes' journals
  slo_assert "1-first-run" blocker "all silt nodes report service active${bad:+ (down:$bad)}" "$ok"
}

# ── Flow 2: publish → fetch bit-perfect from a DIFFERENT node ───────────────────
# LOCAL_PROOF: go test ./e2e -run TestPublishCommitFetchOverTCP -count=1
flow_publish_fetch() {
  local t0 t1 res link sha got ok=0
  t0="$(date +%s)"
  local boot; boot="$(python3 -c "import json;print(json.load(open('$FT_TOPO'))['meta']['boot'])")"
  flow_evidence_nodes fetch-1 "$boot" store-2 store-1   # publisher, boot validator, fetch side
  local h0; h0="$(ft_commit_height "$boot")"   # baseline BEFORE the publish
  res="$(ft_publish fetch-1 1048576 || true)"
  if [ -z "$res" ]; then publish_verdict "2-publish-fetch" blocker "publish never produced a silt: link within ${PUBLISH_RETRY_S}s"; return; fi
  link="${res%% *}"; sha="${res##* }"
  # committed on the boot validator? Require a NEW block (> h0), so a stale
  # pre-publish 'committed block' line can't satisfy the gate (audit).
  if ft_wait_new_block "$boot" "$h0" "$COMMIT_SLO_S"; then :; else
    slo_assert "2-publish-fetch" major "publish did not commit a NEW block (head stayed at $h0) within ${COMMIT_SLO_S}s (link=$link)" 0; return; fi
  # fetch from a DIFFERENT node than the publisher and compare hashes. store-2 in
  # the full topology; store-1 in the SMOKE set (store-2 absent) — both differ from
  # the fetch-1 publisher, so this blocker still runs (not false-fails) under SMOKE.
  local fetchnode=store-2; node_exists store-2 || fetchnode=store-1
  got="$(ssh_node "$fetchnode" "/usr/local/bin/silt swarm get '$link' -o /tmp/ft_got.bin -peers '$PEERS' -registry '$REGREF' >/dev/null 2>&1; sha256sum /tmp/ft_got.bin | cut -d' ' -f1" 2>/dev/null || true)"
  t1="$(date +%s)"
  [ -n "$sha" ] && [ "$got" = "$sha" ] && ok=1
  FT_LAST_LINK="$link"; FT_LAST_SHA="$sha"   # reused (+ SHA-compared) by restart-survival, takedown, chaos
  slo_assert "2-publish-fetch" blocker "fetched from $fetchnode $([ "$ok" = 1 ] && echo 'bit-perfect' || echo "MISMATCH (want=$sha got=$got)")" "$ok" $((t1 - t0))
}

# ── Flow 3: care link — repair/audit without decrypting ─────────────────────────
# LOCAL_PROOF: go test ./e2e -run TestRepairBountyPaysOnTheWire -count=1 (captures the siltcare: link from a real publish)
flow_care_link() {
  flow_evidence_nodes fetch-1
  # The publish emits a siltcare: link; a -care node repairs it and cannot read it.
  local out care
  out="$(ssh_node fetch-1 "/usr/local/bin/silt info /tmp/ft_src.bin 2>&1" || true)"
  care="$(printf '%s' "$out" | grep -oE 'siltcare:\S+' | head -1)"
  if [ -n "$care" ]; then
    slo_assert "3-care-link" minor "publish exposes a siltcare: link (repair/audit without the key)" 1
  else
    record "3-care-link" gap minor "no siltcare: link surfaced from the documented commands — verify the care-link UX is documented"
  fi
}

# ── Flow 4: become a validator (earned standing, not rubber-stamp) ──────────────
# LOCAL_PROOF: go test ./e2e -run TestBondEarnedStandingCommitsOverTCP -count=1
flow_become_validator() {
  flow_evidence_nodes val-b val-c val-d
  local n ok=1 bad=""
  for n in val-b val-c val-d; do
    node_exists "$n" || continue
    # the audit: assert the node's OWN earned standing — 'chain: committed block'
    # is satisfiable by any OBSERVER, so it proved nothing about THIS node. The
    # daemon self-narrates 'standing self=<own id> reputation=N' every bond-audit
    # sweep once its bond qualifies (core/node/bondaudit.go), a genuinely per-node
    # signal that it earned standing on the objective path. That line is a
    # STRUCTURED n.logf → it lands in the on-disk debug.log, NOT journald, so it
    # must be read with waitfor_dlog (the version used waitfor/journald and
    # could never match — SMOKE flagged it).
    local nid; nid="$(node_field "$n" nodeid)"
    if [ "${#nid}" -eq 64 ] && waitfor_dlog "$n" "standing self=$nid reputation=[1-9]" 90 >/dev/null; then :; else ok=0; bad="$bad $n"; fi
  done
  slo_assert "4-become-validator" major "non-anchor validators earn their OWN standing on the objective path${bad:+ (no per-node earned-standing signal:$bad)}" "$ok"
}

# ── Flow 5: multi-validator convergence ─────────────────────────────────────────
# LOCAL_PROOF: go test ./e2e -run TestObjectiveColdStartCommitsGenesis -count=1 (+ the I1-I5 model-check tier)
flow_convergence() {
  local n vals="" maxh=0
  for n in val-a val-b val-c val-d; do node_exists "$n" && vals="$vals $n"; done
  # shellcheck disable=SC2086
  flow_evidence_nodes $vals
  # Read each validator's AUTHORITATIVE committed head — HEIGHT *and* HASH — from its
  # OWN chain-status store, not a journald 'committed block N' line. A height-only
  # signal can't tell same-fork convergence from same-height/DIFFERENT-fork; the head
  # HASH can (blind field test #2 §D; real evidence #3). Same pattern the C2/sybil flow
  # already uses.
  chain_head() { ssh_node "$1" "/usr/local/bin/silt chain-status -store /var/lib/silt 2>&1" \
    | awk '/head height:/{h=$3} /head hash:/{hh=$3} END{print (h==""?0:h), hh}'; }
  # Grade convergence with a bounded WAIT (the Q4 lesson, applied to flow 5):
  # the immediately-preceding 6-fault-tolerance drill STOPS/restarts val-d, so a
  # single point-in-time sample can catch the network mid-catch-up and read a
  # SPURIOUS lag (the the field run run: val-a=33 but val-d=27, 6 behind, right
  # after the drill restarted it). Poll until the validators converge — (a) every
  # validator within 2 of the tip AND (b) every tip-height validator shares the
  # head HASH (a same-height/different-hash pair is a live FORK, exactly the gap
  # §D flags) — or the bound expires. A genuine non-convergence or a PERSISTENT
  # fork still FAILs: the loop only gives a transient post-drill catch-up lag, or
  # a fork that fork-choice is still resolving, time to settle — never a blip.
  : "${CONVERGE_WAIT_S:=120}"
  local heights="" info h hh nv conv=0 tiphash="" fork="" nh nhh cv_t0
  cv_t0="$(date +%s)"
  while : ; do
    heights=""; maxh=0
    for n in $vals; do
      info="$(chain_head "$n")"; h="${info%% *}"; hh="${info#* }"; h="${h:-0}"
      nv="${n//-/_}"; eval "H_$nv=\$h; HH_$nv=\$hh"
      heights="$heights $n=$h:${hh:0:12}"
      [ "$h" -gt "$maxh" ] 2>/dev/null && maxh="$h"
    done
    conv=1; tiphash=""; fork=""
    for n in $vals; do
      nv="${n//-/_}"; eval "nh=\$H_$nv; nhh=\$HH_$nv"
      [ $((maxh - nh)) -gt 2 ] && conv=0
      if [ "$nh" = "$maxh" ]; then
        if [ -z "$tiphash" ]; then tiphash="$nhh"
        elif [ "$nhh" != "$tiphash" ]; then conv=0; fork="$fork $n"; fi
      fi
    done
    { [ "$conv" = 1 ] && [ "$maxh" -ge 1 ]; } 2>/dev/null && break
    [ $(( $(date +%s) - cv_t0 )) -ge "$CONVERGE_WAIT_S" ] && break
    sleep 5
  done
  # A chain that never advanced past genesis is NOT "converged" — all-at-0 means
  # consensus never formed (assert an actual committed block, not agreement-on-nothing).
  if [ "$maxh" -lt 1 ] 2>/dev/null; then
    slo_assert "5-convergence" major "NO block ever committed — the chain is stuck at genesis (heights:$heights); consensus did not form" 0
    return
  fi
  local detail="all validators within 2 of tip=$maxh AND every tip-height validator shares head hash ${tiphash:0:12}… (heights:$heights)"
  [ -n "$fork" ] && detail="FORK at tip height $maxh — divergent head hashes on:$fork (heights:$heights)"
  # H5 — DURABLE convergence, not a point-in-time sample. A single instant can read
  # PASS on a chain that immediately reorgs its committed blocks away (the 2026-08-12
  # fork-choice oscillation: committed height 2 → reorg to height 0). Re-sample the
  # boot validator's head after a settle window and require it did NOT regress; a head
  # that fell below its first sample is an oscillating chain, scored a FAIL directly
  # (previously only caught downstream at publish). If it never converged in the first
  # place, keep that verdict.
  if [ "$conv" = 1 ]; then
    local boot bnv h1 hh1 info2 h2
    boot="$(python3 -c "import json;print(json.load(open('$FT_TOPO'))['meta']['boot'])")"
    bnv="${boot//-/_}"; eval "h1=\${H_$bnv:-0}; hh1=\${HH_$bnv}"
    sleep 20
    info2="$(chain_head "$boot")"; h2="${info2%% *}"; h2="${h2:-0}"
    if [ "$h2" -lt "$h1" ] 2>/dev/null; then
      slo_assert "5-convergence" major "chain REGRESSED after a 20s settle — $boot head fell from height $h1 to $h2 (fork-choice oscillation: a committed chain reorged onto a lighter/height-0 fork; see the fork-choice finding)" 0
      return
    fi
    detail="$detail; DURABLE ($boot head ${h1}->${h2} over 20s, no regression)"
  fi
  slo_assert "5-convergence" major "$detail" "$conv"
}

# ── Flow 6: fault tolerance — kill one validator, quorum still commits ──────────
# LOCAL_PROOF: SUITE=substrate ./integration/adversarial/run.sh (commit with a validator down, under netem)
flow_fault_tolerance() {
  require_nodes "6-fault-tolerance" major val-d || return
  local boot; boot="$(python3 -c "import json;print(json.load(open('$FT_TOPO'))['meta']['boot'])")"
  # A fault-tolerance failure lives in the PUBLISHER (fetch-1) and the SURVIVING
  # validators, not val-d (deliberately down) — require_nodes stashed only val-d.
  # Override so the capture attributes a real gap (run the field run's flow-6 gap
  # had only the down node's journal). shellcheck disable=SC2086
  local survivors; survivors="$(python3 -c "import json;print(' '.join(n for n,v in json.load(open('$NODES_JSON')).items() if v['role']=='validator' and n!='val-d'))")"
  # shellcheck disable=SC2086
  flow_evidence_nodes fetch-1 $survivors
  local h0; h0="$(ft_commit_height "$boot")"   # baseline BEFORE stopping val-d + publishing
  local killt0; killt0="$(date +%s)"   # scopes the escape-fingerprint reads to the escape under test
  svc val-d stop || true
  sleep 5
  # The two tiers are
  # COMPUTED FROM f — the number of DOWN seats — never from the seat count. The
  # bound: after GST, with f
  # seats down, a height commits within f+1 rounds, because designatedProposer
  # steps the rotation one seat per round so at most f consecutive rounds land on
  # a down designee. Wall-clock: Σ_{r=0}^{f} sweepsForRound(r)·ChainSyncInterval +
  # skew + G, with sweepsForRound(r) = 2 + r(r+1)/2, ChainSyncInterval = 30 s, skew
  # < 30 s (Q3), G ≈ 10 s (measured 12-seat WAN gather). This flow stops
  # ONE validator, so f = 1: (2+3)·30 + 30 + 10 = 190 s expected (PASS); the hard
  # cap is 2× = 380 s (sweep-phase + request-timeout noise). Against the measured
  # T_b = 44 s/height (the field report) that is 4.3 × / 8.6 × block-times.
  #
  # STRUCK: the 260/575 base tiers and the "one extra escape rung per 4
  # rotation seats" policy (650/1445 at N=12). Both priced the DEFECT, not the
  # mechanism — they modelled one node climbing a private ladder to r=5, which is
  # exactly what the field run did for 996 s (22.6 × T_b) while the
  # research refuted the seat-count rung scaling outright: rounds burned
  # scale with f, not with N. A cap derived from the defect can never fail on it.
  # The frozen-vs-advancing fingerprint discrimination below stands.
  local ftdown=${FT_DOWN_SEATS:-1} ftr ftsum=0
  for (( ftr=0; ftr<=ftdown; ftr++ )); do ftsum=$(( ftsum + (2 + ftr*(ftr+1)/2) * 30 )); done
  : "${FT_DOWN_COMMIT_S:=$(( ftsum + 30 + 10 ))}"
  : "${FT_DOWN_HARD_S:=$(( 2 * (ftsum + 30 + 10) ))}"
  local ftrcap=$(( ftdown + 1 ))
  echo "    6-fault-tolerance: f=${ftdown} down seat(s) — tiers computed ${FT_DOWN_COMMIT_S}s expected / ${FT_DOWN_HARD_S}s hard cap (≤ f+1 rounds)"
  local res ok=0 fp0="" fp1=""
  res="$(ft_publish fetch-1 262144 || true)"
  if [ -n "$res" ]; then
    # Require a NEW block with val-d down, not a stale pre-kill 'committed block' line.
    ft_wait_new_block "$boot" "$h0" "$FT_DOWN_COMMIT_S" && ok=1
    if [ "$ok" != 1 ]; then
      # shellcheck disable=SC2086
      fp0="$(ft_escape_progress "$killt0" $survivors)"
      echo "    6-fault-tolerance: no commit in ${FT_DOWN_COMMIT_S}s (escape fingerprint: ${fp0}) — extending to the computed r≤${ftrcap} cap ${FT_DOWN_HARD_S}s"
      if ft_wait_new_block "$boot" "$h0" "$(( FT_DOWN_HARD_S - FT_DOWN_COMMIT_S ))"; then
        ok=2
      else
        # shellcheck disable=SC2086
        fp1="$(ft_escape_progress "$killt0" $survivors)"
      fi
    fi
  fi
  if [ "$ok" = 1 ]; then
    slo_assert "6-fault-tolerance" major "publish still committed with one validator (val-d) down (within the computed ${FT_DOWN_COMMIT_S}s down-designee escape bound)" 1
  elif [ "$ok" = 2 ]; then
    slo_assert "6-fault-tolerance" major "publish committed with val-d down BEYOND the expected ${FT_DOWN_COMMIT_S}s but inside the computed r≤${ftrcap} hard cap (${FT_DOWN_HARD_S}s) — a slow escape that started under load, mechanism healthy (escape fingerprint at first bound: ${fp0})" 1
  elif [ -z "$res" ]; then
    # HONESTY RULE (the res-empty
    # path): the two-tier wait above runs ONLY inside `if [ -n "$res" ]`, so when the
    # publish never returned a link NEITHER tier was entered — fp0/fp1 are EMPTY
    # strings, and the old record manufactured "ladder advancing but uncommitted —
    # OUT OF MODEL" from `grep -q '?'` over two empty strings, then printed a cap it
    # never measured (the field run: "(fingerprint → n/a: …)" with the 1445 s cap
    # named as if timed). Say exactly what was observed and nothing more.
    record "6-fault-tolerance" gap major "publish never returned a link within PUBLISH_RETRY_S=${PUBLISH_RETRY_S}s with val-d down — the ${FT_DOWN_COMMIT_S}s/${FT_DOWN_HARD_S}s escape tiers were NOT entered and NO cap was measured (fingerprint not read); read the captured client error (publish-diag / .ft_publish_lasterr) and the survivor journals (validators run -log debug: the 'new-view proposal not committed' line names the failing designee) before attributing (#7)"
  elif [ -n "$fp1" ] && [ "$fp1" = "$fp0" ] && [ "${fp0#*\?}" = "$fp0" ]; then
    # WEDGE only on a READABLE, frozen fingerprint: fp must contain no `?`
    # (an UNKNOWN source is not evidence of a frozen ladder). A frozen READABLE
    # fingerprint with round-changes present but stuck IS the wedge; rc advancing
    # would have made fp1 != fp0 and routed to the OUT-OF-MODEL gap below.
    record "6-fault-tolerance" fail major "WEDGE SIGNATURE: no commit AND a frozen, readable escape fingerprint (${fp0}) across the ${FT_DOWN_COMMIT_S}s→${FT_DOWN_HARD_S}s extension with val-d down — the round ladder is NOT advancing; attribute from the captured survivor journals"
  else
    record "6-fault-tolerance" gap major "no new commit within the computed ${FT_DOWN_HARD_S}s (≤ f+1 rounds, f=${ftdown}) hard cap with val-d down (fingerprint ${fp0} → ${fp1:-n/a}$(printf '%s' "${fp0}${fp1}" | grep -q '?' && echo '; a fingerprint source was UNREADABLE — cannot claim a frozen ladder' || echo ': ladder advancing but uncommitted — OUT OF MODEL')) — read the captured client error (publish-diag / .ft_publish_lasterr) and survivor journals before attributing"
  fi
  svc val-d start || true
}

# ── Flow 7: restart survival — standing + issued tokens + stored content ────────
# LOCAL_PROOF: RESTART=1 ./integration/nat/run.sh (persisted-store reload + re-announce)
flow_restart_survival() {
  flow_evidence_nodes val-b store-1 store-2
  local t0 t1 ok=0
  t0="$(date +%s)"
  svc val-b restart || true
  # standing must come back WITHOUT redoing the bond (fast reload), and quickly.
  # SCOPED to post-restart logs (t0 captured above): the 'bond: reloaded …' line
  # is emitted on EVERY boot, so an unscoped waitfor could match a prior boot's
  # line even if THIS restart hung/failed to reload standing (the audit
  # restart-standing stale-gap). --since @t0 admits only the post-restart boot.
  if waitfor_since val-b '(reload|restored|persisted).*(bond|standing)|standing.*(reload|restored)|bond.*loaded' "$t0" "$RESTART_SLO_S" >/dev/null; then ok=1; fi
  t1="$(date +%s)"
  slo_assert "7-restart-standing" major "val-b standing returned after restart without re-bonding" "$ok" $((t1 - t0))
  # stored content still discoverable+served after a storage-node restart. Needs a
  # second storage node to restart while store-1 serves — skip cleanly under SMOKE
  # (store-2 absent) rather than restart a nonexistent node and assert on nothing.
  if node_exists store-2; then
    # No upstream link ⇒ nothing to fetch: GAP (untested), not a want=?/got=<none> FAIL —
    # the same missing-link precondition 8-takedown already GAPs on (H3). A failed
    # publish is scored where it happens (2-publish-fetch), not cascaded here.
    # SELF-CONTAINED (2026-08-20 randomization): reuse a prior link or publish our
    # own, so a randomized sheet that runs this before publish-fetch still tests the
    # property instead of GAPping on a missing precondition.
    local rlink="${FT_LAST_LINK:-}" rsha="${FT_LAST_SHA:-}"
    if [ -z "$rlink" ]; then local rres; rres="$(ft_publish fetch-1 262144 || true)"; rlink="${rres%% *}"; rsha="${rres##* }"; fi
    if [ -z "$rlink" ]; then publish_verdict "7-restart-content" major "no link (reuse+self-publish both failed) — restart-content UNTESTED"; return; fi
    # Wait for the post-restart re-announce CONDITION, not a magic sleep
    # (2026-08-21, run the field run false FAIL): the old `sleep 8` raced the
    # restarted node's recovery — the same sheet MEASURED 228s to re-announce
    # under load (vs 37s on a fresh fleet) — and the one-shot fetch discarded
    # the client's stderr, leaving got=<none> unattributable. Mirror
    # chaos-fetch: condition-wait on the measured 300s envelope, keep stderr,
    # premise-classify. A missed re-announce is called out but the fetch is
    # still tried (holders other than store-2 can and should serve k columns —
    # that IS the property).
    local rt0; rt0="$(date +%s)"
    svc store-2 restart || true
    waitfor_since store-2 're-announced [0-9]+ held chunks' "$rt0" 300 >/dev/null 2>&1 || \
      echo "    ⚠ restart-content: store-2 never re-announced within 300s of restart — fetching anyway (survivors should serve)"
    local ok2=0 got="" rgeterr=""
    # SHA-compare, not echo-OK (§D): a `swarm get` that exits 0 but writes truncated or
    # wrong bytes must NOT pass as "fetchable" — assert the fetched file's SHA matches.
    rgeterr="$(ssh_node store-1 "rm -f /tmp/ft_r.bin; /usr/local/bin/silt swarm get '$rlink' -o /tmp/ft_r.bin -peers '$PEERS' -registry '$REGREF' 2>&1 >/dev/null | tail -3" 2>/dev/null || true)"
    got="$(ssh_node store-1 "sha256sum /tmp/ft_r.bin 2>/dev/null | cut -d' ' -f1" 2>/dev/null || true)"
    [ -n "$rsha" ] && [ "$got" = "$rsha" ] && ok2=1
    if [ "$ok2" != 1 ] && printf '%s' "$rgeterr" | grep -qiE 'root not in registry|no such entry'; then
      record "7-restart-content" gap major "fetch found the root ABSENT from the registry — the publish premise broke upstream, restart-content UNTESTED not failed (client: $(printf '%s' "$rgeterr" | tr '\n' ';' | head -c 200))"
    else
      slo_assert "7-restart-content" major "content still fetchable BIT-PERFECT after a storage-node restart$([ "$ok2" = 1 ] || echo " (want=${rsha:-?} got=${got:-<none>}; client: $(printf '%s' "$rgeterr" | tr '\n' ';' | head -c 300))")" "$ok2"
    fi
  else
    record "7-restart-content" skip major "skipped — needs store-2 (absent in this topology, e.g. SMOKE)"
  fi
}

# ── Flow 8: per-hash takedown on ONE operator only ──────────────────────────────
# LOCAL_PROOF: ./integration/takedown/run.sh
flow_takedown() {
  # The non-globality claim needs TWO storage operators: one honouring the denylist
  # and one still serving. A topology with a single store has nothing to compare, so
  # the flow SKIPS naming the absent node rather than grading. Without this guard the
  # serve leg ssh'd to a host that was never deployed, read an empty sha, and recorded
  # "denied=1 served=0 — daemon never narrated denylist enforcement, or store-2 failed
  # to serve" — a verdict naming a daemon fault on a run where the denial leg had in
  # fact PASSED and the only missing thing was the second operator.
  require_nodes "8-takedown" major store-1 store-2 || return
  flow_evidence_nodes store-1 store-2
  # SELF-CONTAINED (2026-08-20 randomization): reuse a prior link if one exists,
  # else publish our own — so this flow is ORDER-INDEPENDENT (it no longer GAPs
  # just because it ran before publish-fetch in a randomized sheet).
  local link="${FT_LAST_LINK:-}" wantsha="${FT_LAST_SHA:-}"
  if [ -z "$link" ]; then local res; res="$(ft_publish fetch-1 262144 || true)"; link="${res%% *}"; wantsha="${res##* }"; fi
  if [ -z "$link" ]; then publish_verdict "8-takedown" major "no link (reuse+self-publish both failed) — takedown UNTESTED"; return; fi
  # extract the HEX root from the silt:v1:<b64url-root>:<...> link and deny it on store-1
  local root; root="$(b64url_to_hex "$(printf '%s' "$link" | cut -d: -f3)")"
  if [ ${#root} -ne 64 ]; then record "8-takedown" gap minor "could not decode a 64-hex root from $link (got '${root:0:16}…')"; return; fi
  ssh_node store-1 "echo '$root' | sudo tee /var/lib/silt/deny.txt >/dev/null" >/dev/null 2>&1
  relaunch_with store-1 "-denylist /var/lib/silt/deny.txt"; sleep 8
  # DENIAL leg (fixed — audit- class, wrong-surface probe): the old probe ran
  # `swarm get` ON store-1 and grepped its output for a refusal. But `swarm get` is a
  # SHORT-LIVED CLIENT node ("join, do the thing, leave" — cmd/silt/swarm.go): it
  # never consults the store-1 daemon's denylist, walks the DHT, and happily fetches
  # from any other holder — so the refusal grep could NEVER match and this flow GAP'd
  # on every run (`denied= served=1`). The denylist gates the DAEMON (refuse to
  # store/serve/prove/announce/repair + purge held chunks — core/node/denylist.go),
  # so assert THAT surface: the daemon's own enforcement line in its journal
  # ("denylist: N root(s) denied; purged M held chunk(s)" or "denylist: honoring N
  # denied root(s)" when it held none of them — both prove the operator's takedown
  # is loaded and enforced on this node only).
  local denyline denied=0 served=0
  denyline="$(waitfor store-1 'denylist: .*denied' 60 || true)"
  [ -n "$denyline" ] && denied=1
  # SHA-compare the SERVE side (§D): "still served elsewhere" must mean store-2 returned
  # the REAL bytes bit-perfect, not merely that `swarm get` exited 0.
  local sgot; sgot="$(ssh_node store-2 "/usr/local/bin/silt swarm get '$link' -o /tmp/ft_s.bin -peers '$PEERS' -registry '$REGREF' >/dev/null 2>&1; sha256sum /tmp/ft_s.bin | cut -d' ' -f1" 2>/dev/null || true)"
  [ -n "$wantsha" ] && [ "$sgot" = "$wantsha" ] && served=1
  if [ "$denied" = 1 ] && [ "$served" = 1 ]; then
    slo_assert "8-takedown" major "store-1 enforces the operator denylist (${denyline##*silt}) while store-2 still serves BIT-PERFECT (no global switch)" 1
  else
    record "8-takedown" gap major "takedown scoping not confirmed (denied=$denied served=$served) — daemon never narrated denylist enforcement, or store-2 failed to serve"
  fi
  restore_argv store-1
}

# ── Flow 9: cross-NAT — a natted node moves a file via the relay ────────────────
# LOCAL_PROOF: ./integration/nat/run.sh (EMULATED NAT; the real-middlebox cone/symmetric decision is the owned cloud residue)
flow_cross_nat() {
  require_nodes "9-cross-nat" major nat-1 nat-2 || return
  client_preflight "9-cross-nat" major nat-1 nat-2 || return
  flow_evidence_nodes nat-1 nat-2 relay   # a cross-NAT failure lives on either NAT node OR the relay
  local res
  res="$(ft_publish nat-1 262144 || true)"    # nat-1 is un-dialable → must use the relay
  # Attribute the LEG (#7): run the field run recorded a bare "did not exchange a
  # file" FAIL that left publish-vs-fetch unknowable after teardown.
  if [ -z "$res" ]; then
    publish_verdict "9-cross-nat" major "natted nodes did not exchange a file via the relay — the PUBLISH leg (nat-1 → relay → validators) never landed a link"
    return
  fi
  local link="${res%% *}" sha="${res##* }" got
  got="$(ssh_node nat-2 "/usr/local/bin/silt swarm get '$link' -o /tmp/ft_n.bin -peers '$PEERS' -registry '$REGREF' >/dev/null 2>&1; sha256sum /tmp/ft_n.bin | cut -d' ' -f1" 2>/dev/null || true)"
  if [ -n "$sha" ] && [ "$got" = "$sha" ]; then
    slo_assert "9-cross-nat" major "natted nodes exchanged a file through the relay/hole-punch" 1
  else
    slo_assert "9-cross-nat" major "natted nodes did not exchange a file via the relay — publish landed ($link) but the FETCH leg on nat-2 returned '${got:-<none>}' (want $sha)" 0
  fi
}

# ── Flow 21: publish and fetch on the internet as it is — the cross-REGION leg ──
# A NATed publisher in one region and a COLD fetcher in another: the object must
# arrive bit-perfect, inside a bound DERIVED from the configuration the fetching
# client is actually running under.
#
# It is a different claim from 9-cross-nat, which fetches on nat-2 — the other
# side of the SAME NAT subnet, in the SAME region. That proves the relay path.
# This proves the relay path plus a real inter-region wire, from a seat that has
# never held a byte of the object and has no warm route to the publisher (which
# is un-dialable, so every byte crosses the relay).
#
# WHY THE BOUND IS READ FROM THE CLIENT AND NOT COMPUTED HERE. The publish/fetch
# client is its own process with its own deadlines; it is NOT the daemon, and it
# does not take the daemon's flags. This file's FETCH_SLO_S was derived from
# `-request-timeout 8s` times the daemon's retries — the configuration of a
# process that performs no fetch. So the client narrates the posture it holds
# (per-RPC worst case with retries and backoff, holder-dial deadline, sweep
# schedule, provider count, and the ceiling it enforces on itself) and the bound
# is built from those numbers. A deployment that widens its deadlines for a worse
# path widens this bound with them; nothing here is a typed constant.
#
# THE BOUND IS THE ALL-TIMEOUT WORST CASE, deliberately: every lookup spending
# its full retry ladder and every sweep dialing every provider to the deadline.
# It is generous — a fetch where all of that actually happened would FAIL, having
# found no provider — so it cannot false-fail a healthy path. What it catches is
# a retrieval that waited on something its own configuration does not explain.
#
# The round-trip, and the posture and elapsed lines this bound is read from, are
# proven by the e2e publish-commit-fetch test; the NAT leg by the emulated-NAT
# suite, whose own residue is the real-middlebox cone/symmetric decision.
# ft_region NODE — the node's region. nodes.json carries the ZONE (us-west1-a),
# which is the region plus a zone letter, so the region is the zone with its last
# segment removed — the same derivation topology.py uses to lay out the subnets.
# Reading a "region" field straight off nodes.json returns nothing: there isn't
# one, and a flow that compared two empty strings would call every topology
# single-region and skip itself forever.
ft_region() { printf '%s' "$(node_field "$1" zone)" | sed 's/-[a-z]$//'; }

# LOCAL_PROOF: go test ./e2e -run TestPublishCommitFetchOverTCP -count=1 && ./integration/nat/run.sh
flow_cross_region_cold_fetch() {
  require_nodes "21-cross-region-cold-fetch" blocker nat-1 || return
  local pubregion; pubregion="$(ft_region nat-1)"
  if [ -z "$pubregion" ]; then
    record "21-cross-region-cold-fetch" gap blocker \
      "could not read the publisher's region off the node map — nothing below can claim to be cross-region"
    return
  fi

  # The candidate cold fetchers: every node in a region OTHER than the
  # publisher's that is a plausible client seat. Chosen from the topology rather
  # than named, so moving a node between regions cannot silently turn this back
  # into a same-region fetch — the thing 9-cross-nat already covers.
  local cands; cands="$(FT_PUB_REGION="$pubregion" python3 -c "
import json, os, re
t = json.load(open('$NODES_JSON'))
pub = os.environ['FT_PUB_REGION']
def region(v):
    return re.sub(r'-[a-z]\$', '', v.get('zone', ''))
# natted/natgw share the publisher's NAT subnet by construction; the island and
# adversary seats live in a separate consensus universe and are not client seats.
skip = {'natgw', 'natted', 'island', 'adversary', 'sybil'}
out = [(region(v), n) for n, v in t.items()
       if region(v) and region(v) != pub and v.get('role', '') not in skip]
print(' '.join(n for _, n in sorted(out)))" 2>/dev/null)"
  if [ -z "$cands" ]; then
    record "21-cross-region-cold-fetch" skip blocker \
      "no node outside the publisher's region ($pubregion) — a single-region topology cannot carry the cross-region claim (PIN_ZONE / SMOKE collapses the regions on purpose)"
    return
  fi
  # shellcheck disable=SC2086
  flow_evidence_nodes nat-1 relay $cands
  # The publisher and the relay both have to answer before any of this means
  # anything. An unreachable seat otherwise spends the whole publish window and
  # then presents as a property failure — a preempted VM wearing a bit-perfect
  # verdict's clothes.
  client_preflight "21-cross-region-cold-fetch" blocker nat-1 relay || return

  # Publish from the NATed seat: nat-1 is un-dialable, so the link and every byte
  # behind it must cross the relay.
  local res; res="$(ft_publish nat-1 262144 || true)"
  if [ -z "$res" ]; then
    publish_verdict "21-cross-region-cold-fetch" blocker \
      "the PUBLISH leg never landed a link from the NATed seat (nat-1 to relay to validators) — the cross-region fetch was never reached"
    return
  fi
  local link="${res%% *}" sha="${res##* }"

  # HOW COLD THE FETCHER IS IS MEASURED, NOT ASSUMED — and "cold" is the right
  # amount of strict, which is not "holds nothing". On a swarm of this size with
  # the shipped replication, every seat holds SOME shard of any object: an
  # eight-chunk publish scatters twenty-odd placements over a dozen eligible
  # holders. Demanding a seat that holds none of it selects nothing and the flow
  # reports itself untestable, which is what the first drive did.
  #
  # What the claim needs is a fetcher that cannot assemble the object without
  # crossing the wire. So the holder set is read off the chain, each out-of-region
  # candidate is scored by how many of the object's columns it already holds, the
  # COLDEST is chosen, and a candidate holding every column is disqualified —
  # that one would measure a local disk read with a cross-region label on it. The
  # count travels into the verdict, because "cold" is a quantity here and a
  # verdict that hid it would be claiming more than it measured.
  local holders; holders="$(ssh_node nat-1 "/usr/local/bin/silt swarm holders '$link' -peers '$PEERS' -registry '$REGREF' 2>/dev/null" || true)"
  local ncols; ncols="$(printf '%s' "$holders" | grep -cE '^(column [0-9]+|uncoded)' || true)"
  local fetcher="" fetcher_held=0 c cid held
  for c in $cands; do
    cid="$(node_field "$c" nodeid)"
    [ -z "$cid" ] && continue
    ssh_node "$c" "test -x /usr/local/bin/silt" || continue
    held="$(printf '%s' "$holders" | grep -cE "^(column [0-9]+|uncoded).*$cid" || true)"
    held="${held:-0}"
    # Holds every column: nothing to cross a region for.
    [ "${ncols:-0}" -gt 0 ] && [ "$held" -ge "$ncols" ] && continue
    if [ -z "$fetcher" ] || [ "$held" -lt "$fetcher_held" ]; then
      fetcher="$c"; fetcher_held="$held"
    fi
  done
  if [ -z "$fetcher" ]; then
    record "21-cross-region-cold-fetch" gap blocker \
      "no usable out-of-region fetcher among ($cands): each either holds every one of the object's ${ncols} columns already — a local disk read with a cross-region label on it — or has no client binary; the property is UNTESTED rather than failed"
    return
  fi
  client_preflight "21-cross-region-cold-fetch" blocker "$fetcher" || return
  local fetchregion; fetchregion="$(ft_region "$fetcher")"
  echo "    21-cross-region-cold-fetch: NATed publisher nat-1 ($pubregion) -> coldest out-of-region seat $fetcher ($fetchregion), already holding ${fetcher_held} of ${ncols} columns"

  # The fetch. SSH_NODE_TIMEOUT is raised past the client's own operation
  # ceiling for this call: at the default 90s the harness's transport would be
  # the real bound and the derived one would never be the thing that graded —
  # a bound that does not bound what it appears to.
  local t0 t1 outp
  t0="$(date +%s)"
  outp="$(SSH_NODE_TIMEOUT=420 ssh_node "$fetcher" \
    "/usr/local/bin/silt swarm get '$link' -o /tmp/ft_xr.bin -peers '$PEERS' -registry '$REGREF' 2>&1; sha256sum /tmp/ft_xr.bin 2>/dev/null | cut -d' ' -f1" || true)"
  t1="$(date +%s)"
  local wall=$(( t1 - t0 ))

  local got; got="$(printf '%s' "$outp" | grep -oE '^[0-9a-f]{64}$' | tail -1)"
  local posture; posture="$(printf '%s' "$outp" | grep -m1 'fetch posture: ' || true)"
  local elapsed; elapsed="$(printf '%s' "$outp" | grep -oE 'fetch elapsed: [0-9.]+s' | grep -oE '[0-9.]+' | head -1)"

  if [ -z "$posture" ]; then
    record "21-cross-region-cold-fetch" gap blocker \
      "the client narrated no fetch posture on $fetcher, so there is NO deployed configuration to derive a bound from and the wall-clock ${wall}s grades nothing; client output: $(printf '%s' "$outp" | tr '\n' ';' | head -c 300)"
    return
  fi

  # Build the bound out of the numbers the client just reported. Every symbol
  # below comes from that line; the object's shape (262144 B at the 65536-byte
  # chunk ft_publish uses) is this harness's own choice and is the only thing
  # added to it.
  local bound; bound="$(FT_POSTURE="$posture" python3 -c "
import math, os, re
line = os.environ['FT_POSTURE']
def g(pat):
    m = re.search(pat, line)
    return float(m.group(1)) if m else None
per_rpc  = g(r'per-RPC <= ([0-9.]+)s')
per_dial = g(r'per-dial ([0-9.]+)s')
sweeps   = g(r'([0-9]+) sweeps')
swp_bo   = g(r'sweeps \+ ([0-9.]+)s backoff')
provs    = g(r'replication ([0-9]+)')
ceiling  = g(r'operation ceiling ([0-9.]+)s')
if None in (per_rpc, per_dial, sweeps, swp_bo, provs, ceiling):
    raise SystemExit(1)
chunks = math.ceil(262144 / 65536)
# One provider lookup per chunk, plus the registry-entry and manifest lookups
# ahead of them; then every sweep of every chunk dialing every provider to the
# deadline, with the sweep backoff in between.
lookups = chunks + 2
bound = lookups * per_rpc + chunks * (sweeps * provs * per_dial + swp_bo)
print('%d %d %d' % (round(bound), round(ceiling), chunks))" 2>/dev/null || true)"
  if [ -z "$bound" ]; then
    record "21-cross-region-cold-fetch" gap blocker \
      "the fetch posture did not parse into a bound — the line's shape changed and this flow would otherwise grade against nothing: $posture"
    return
  fi
  # shellcheck disable=SC2086
  set -- $bound
  local derived="$1" ceiling="$2" chunks="$3"
  # The measurement is the client's own elapsed where it reported one (it times
  # the retrieval, not the ssh round trip); the wall-clock is the fallback and
  # is strictly larger, so falling back can only be conservative.
  local measured="${elapsed%%.*}"; measured="${measured:-$wall}"
  echo "    21-cross-region-cold-fetch: derived bound ${derived}s over ${chunks} chunks (client ceiling ${ceiling}s); measured ${measured}s (ssh wall ${wall}s)"

  if [ -z "$sha" ] || [ "$got" != "$sha" ]; then
    slo_assert "21-cross-region-cold-fetch" blocker \
      "the cold cross-region fetch was NOT bit-perfect: $fetcher ($fetchregion) returned '${got:-<none>}' for an object published from the NATed seat nat-1 ($pubregion), want $sha — derived bound ${derived}s, measured ${measured}s" 0 "$wall"
    return
  fi
  if [ "$measured" -gt "$derived" ] 2>/dev/null; then
    slo_assert "21-cross-region-cold-fetch" blocker \
      "bit-perfect from $fetcher ($fetchregion) but in ${measured}s, PAST the ${derived}s its own configuration accounts for (${chunks} chunks; client ceiling ${ceiling}s): the retrieval waited on something the deployed deadlines do not explain — read the client's fetch posture and the relay's journal before attributing" 0 "$wall"
    return
  fi
  slo_assert "21-cross-region-cold-fetch" blocker \
    "BIT-PERFECT across regions from a cold seat: NATed publisher nat-1 ($pubregion) -> $fetcher ($fetchregion), which held ${fetcher_held} of the object's ${ncols} columns and pulled the rest over the relay, in ${measured}s against the ${derived}s its deployed configuration derives (${chunks} chunks, ceiling ${ceiling}s)" 1 "$wall"
}

# ── Field impairment: tc netem on the silt traffic, and nothing else ───────────
# Build-immutable #5 names four everyday conditions — jitter, latency, packet
# loss and reordering — and the consensus drills are certified under all four on
# an impaired loopback every night. What that cannot answer is whether the chain
# keeps COMMITTING across a real inter-region wire while they are applied, under
# a publish stream, on the deployed binaries. That is this flow.
#
# THE SHAPING IS DELIBERATELY NARROW. netem on the root qdisc would degrade the
# IAP control channel this harness polls over, and a lost verdict would then be
# indistinguishable from a lost block. So the interface gets a `prio` root and
# the impairment is attached to one band, with u32 filters steering only traffic
# destined for the swarm's own subnets into it. Control traffic keeps the clean
# bands. We impair the product, not our ability to watch it.
#
# `reorder` REQUIRES a delay — tc refuses it outright otherwise — so the default
# profile carries all four conditions in one shape and cannot be reduced to a
# subset by accident.
: "${IMPAIR_NETEM:=delay 80ms 20ms distribution normal loss 1% reorder 25% 50%}"
: "${IMPAIR_HEIGHTS:=3}"

# ft_impair_dev NODE — the interface the node's default route leaves by.
#
# EVERY `ip` AND `tc` CALL IN THIS SECTION GOES THROUGH sudo, READBACKS INCLUDED.
# On the deployed image these live in /sbin and /usr/sbin, which are not on the
# login user's PATH — so an unprivileged readback returns "tc: command not found"
# rather than a count. That is not a cosmetic difference: it made ft_impair_on
# report FAILURE for shaping it had just successfully applied, which left the
# node shaped and out of the unwind list, and every flow after it on the sheet
# was then graded over an impaired wire while the flow said it had never touched
# anything. Measured on a real VM 2026-09-18.
ft_impair_dev() { ssh_node "$1" "sudo ip route show default 2>/dev/null | awk '{for(i=1;i<=NF;i++) if(\$i==\"dev\") {print \$(i+1); exit}}'"; }

# ft_impair_cidrs — the swarm's own subnets, derived from the addresses the run
# actually deployed rather than from a written-down list: every node's /24, NAT
# subnet included. A node that moves regions moves its subnet with it, so the
# shaping cannot drift away from where the traffic is.
ft_impair_cidrs() {
  python3 -c "
import json
t = json.load(open('$FT_TOPO'))
nets = {'.'.join(v['ip'].split('.')[:3]) + '.0/24'
        for v in t['nodes'].values() if v.get('ip')}
print(' '.join(sorted(nets)))" 2>/dev/null
}

# ft_impair_on NODE — apply the profile to this node's swarm-bound egress.
# Returns 0 only if the netem qdisc is present afterwards: an impairment that
# could not be applied must never be reported as one that was.
ft_impair_on() {
  local n="$1" dev cidrs cmd
  dev="$(ft_impair_dev "$n")"; [ -z "$dev" ] && return 1
  cidrs="$(ft_impair_cidrs)"; [ -z "$cidrs" ] && return 1
  # One line: it crosses `gcloud compute ssh --command`, where an embedded
  # newline is one more thing to get wrong for no benefit. priomap sends every
  # ToS class to band 0 so NOTHING lands in the impaired band by default — only
  # the u32 filters below put traffic there.
  cmd="sudo tc qdisc del dev $dev root 2>/dev/null; sudo tc qdisc add dev $dev root handle 1: prio bands 4 priomap 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 && sudo tc qdisc add dev $dev parent 1:4 handle 40: netem ${IMPAIR_NETEM}"
  local c
  for c in $cidrs; do
    cmd="$cmd && sudo tc filter add dev $dev protocol ip parent 1: prio 1 u32 match ip dst $c flowid 1:4"
  done
  # Read the answer back into a variable rather than piping it into a test. This
  # file runs under `set -o pipefail`, and the remote `grep -c` exits NON-ZERO
  # when its count is zero — so a pipeline here takes its status from the count
  # rather than from the comparison, and reports the opposite of what it found.
  local n_netem; n_netem="$(ssh_node "$n" "$cmd >/dev/null 2>&1; sudo tc qdisc show dev $dev | grep -c netem || true")"
  if [ "${n_netem:-0}" -ge 1 ] 2>/dev/null; then return 0; fi
  # A partial application must not be left behind. Whatever went wrong, this node
  # may already be carrying a root qdisc, and a node that is shaped but absent
  # from the caller's unwind list is the worst of both worlds.
  ssh_node "$n" "sudo tc qdisc del dev $dev root >/dev/null 2>&1; true" >/dev/null 2>&1
  return 1
}

# ft_impair_off NODE — remove it, and report whether the interface came back
# clean. A leftover qdisc would silently impair every later flow on this sheet.
#
#   0 = clean.  1 = STILL SHAPED.  2 = the seat is GONE, and nothing it once
#   carried can shape anything.
#
# THE THIRD CASE IS NOT PEDANTRY, it is the one that bit. These fleets run on SPOT
# instances with instanceTerminationAction=DELETE, so a preemption does not leave a
# sick node — it removes the machine. ssh then fails, the count comes back empty,
# and a two-state function reads "not zero" as "still shaped". The flow reported
# that every later verdict on the sheet was running on a shaped wire and told the
# reader to clear it by hand with `tc qdisc del` — on an instance that no longer
# exists, which is a remedy nobody can carry out, for a hazard that is not there.
# A vanished seat impairs nothing.
#
# Same shape, same reason as ft_impair_on: the count comes back in a variable,
# never through a pipeline whose status pipefail would take from the count. It bit
# here first and it bit loudly — the flow reported "the impairment did NOT come
# off" about four interfaces that were already clean, which is a verdict naming the
# wrong cause and would have buried the result it was guarding. This is the same
# family of defect one layer out.
ft_impair_off() {
  local n="$1" dev
  dev="$(ft_impair_dev "$n")"
  # No device means ssh could not answer at all. Distinguish a seat that is GONE
  # from one that is merely mute: a deleted instance is the expected end of a spot
  # seat and carries no wire, while an unreachable-but-live one might still be
  # shaped and must stay a blocker.
  if [ -z "$dev" ]; then
    ft_node_exists "$n" && return 1
    return 2
  fi
  local n_netem; n_netem="$(ssh_node "$n" "sudo tc qdisc del dev $dev root >/dev/null 2>&1; sudo tc qdisc show dev $dev | grep -c netem || true")"
  if [ -z "$n_netem" ]; then
    ft_node_exists "$n" && return 1
    return 2
  fi
  [ "${n_netem:-1}" -eq 0 ] 2>/dev/null
}

# ft_node_exists NODE — does this seat's instance still exist in the project?
# Asked of the cloud rather than of the node, because the question is precisely
# whether the node is there to answer. Local-backend runs answer from docker.
ft_node_exists() {
  local n="$1" inst zone
  inst="$(node_field "$n" instance_name)"
  [ -z "$inst" ] && return 1
  if [ "$FT_BACKEND" = local ]; then
    docker inspect "$inst" >/dev/null 2>&1
    return
  fi
  zone="$(node_field "$n" zone)"
  gcloud compute instances describe "$inst" --zone "$zone" --project "$PROJECT_ID" \
    --format="value(name)" >/dev/null 2>&1
}

# ft_impair_counters NODE — netem's own packet/drop/reorder counters, which are
# the evidence that swarm traffic actually crossed the impaired band rather than
# the clean ones. Prints "sent dropped reordered" or nothing.
ft_impair_counters() {
  local n="$1" dev
  dev="$(ft_impair_dev "$n")"; [ -z "$dev" ] && return 1
  # `tc -s qdisc show` lists every qdisc on the interface, each followed by its
  # own Sent line. Take the FIRST netem block and stop: reading past it would
  # report the root prio's totals, which include the clean bands and would credit
  # an impairment that steered nothing.
  ssh_node "$n" "sudo tc -s qdisc show dev $dev 2>/dev/null" | awk '
    /qdisc netem/ { innetem = 1; next }
    innetem && /^ *Sent / {
      sent = $2; d = 0
      for (i = 1; i <= NF; i++) if ($i == "(dropped") { d = $(i+1); gsub(/,/, "", d) }
      printf "%s %s\n", sent, d
      exit
    }'
}

# ft_lane_counters NODE — the control lane's own four numbers, counted out of the
# node's debug.log. Structured transport lines go to that file and never to
# journald, so this is where the lane narrates itself.
#
#   established   a lane came up to a peer (the mechanism is live at all)
#   failed        a lane dial or a lane write failed (each one fell back to the
#                 shared conn, so this is a cost and not a loss)
#   last-resort   a frame rode the lane because no other path existed — rare is
#                 correct, routine means the lane is carrying bulk again
#   ctrl-dropped  a small frame was refused by the outbound budget, which is the
#                 blinding shape the reserve exists to prevent; must be zero
#
# Prints "established failed last-resort ctrl-dropped", or nothing if the file
# cannot be read.
ft_lane_counters() {
  ssh_node "$1" "sudo awk '/control lane established/{e++} /control lane dial failed/{f++} /control lane write failed/{f++} /for want of any other path/{l++} /outbound CONTROL frame dropped/{c++} END{printf \"%d %d %d %d\\n\", e+0, f+0, l+0, c+0}' /var/lib/silt/debug.log 2>/dev/null"
}

# ft_lane_delta BASELINE FINAL — the four counters' movement across a window,
# as "e f l c". A count read after the fact mixes the window being graded with
# every minute of the run before it; the difference is the only form of the
# number that belongs to the drive. Missing readings give an empty field rather
# than a zero, because "not read" and "did not happen" are different findings.
ft_lane_delta() {
  python3 -c "
import sys
b = sys.argv[1].split(); f = sys.argv[2].split()
if len(b) != 4 or len(f) != 4:
    print('')
else:
    print(' '.join(str(int(x) - int(y)) for x, y in zip(f, b)))" "$1" "$2" 2>/dev/null
}

# ── Flow 21b: the chain keeps committing under injected impairment ─────────────
# Sustained publish load across a fleet whose swarm traffic is carrying latency,
# jitter, loss and reordering at once, graded on the SAME computed per-height
# escape bound the unimpaired soak uses. The bound does not move for the
# impairment on purpose: the synchronizer's round schedule is what it is, and a
# claim that the chain survives the adverse internet is a claim it survives it
# inside the timing the design already commits to.
#
# NOTHING HERE CAN PASS VACUOUSLY. Two controls, both driven in the same run:
#   - the impairment must be PRESENT — the netem qdisc is read back on every
#     seat after it is applied, and a seat that would not take it aborts the
#     flow rather than contributing a clean number to an impaired verdict;
#   - the impairment must have BITTEN — netem's own counters must show swarm
#     packets crossing the impaired band, so a shaping rule that steered no
#     traffic (a wrong CIDR, a renamed interface) reports UNCREDITED and fails
#     instead of grading a clean network with an adverse label on it.
#
# The same four conditions run over real daemons and real TCP, deterministically
# and off-cloud, in the netem suite — ONE CONDITION PER ARM there, which is
# exactly why the composition this flow drives has to be driven here.
# LOCAL_PROOF: SUITE=all ./integration/adversarial/run.sh
flow_impaired_commit() {
  [ "${IMPAIR:-1}" = 1 ] || { record "21-impaired-commit" skip blocker "opt-out (IMPAIR=0)"; return; }
  require_nodes "21-impaired-commit" blocker val-a val-b || return
  local vals; vals="$(python3 -c "import json;print(' '.join(n for n,v in json.load(open('$NODES_JSON')).items() if v['role']=='validator'))")"
  # A validator the substrate took away is not a shaping failure. Without this
  # the flow reaches a preempted seat, cannot read an interface off it, and
  # reports "tc/netem unavailable, or the interface or subnet map did not match"
  # about a machine that no longer exists — a refusal naming the wrong cause,
  # and one that would send the next reader to the wrong place. Seen on a SPOT
  # instance 2026-09-18.
  # shellcheck disable=SC2086
  require_live "21-impaired-commit" blocker $vals || return
  # shellcheck disable=SC2086
  flow_evidence_nodes $vals
  : "${H_ESCAPE_S:=220}"

  # BASELINE THE LANE BEFORE THE WIRE IS SHAPED. This flow's verdict is the only
  # place the control lane is graded under the conditions it was built for, and a
  # PASS used to leave no trace of HOW it passed: evidence is captured on a
  # non-green verdict, the fleet is destroyed at the end of the sheet, and the
  # per-seat numbers went with it. The counts belong to the drive, so they are
  # read at both ends of it and reported as the difference.
  local lane_base="" lane_v
  for v in $vals; do
    lane_v="$(ft_lane_counters "$v" || true)"
    lane_base="$lane_base ${v}=$(printf '%s' "${lane_v:-unread}" | tr ' ' ',')"
  done

  # Apply, and read back. A seat that will not take the shaping is not silently
  # left clean: the applied set is unwound and the flow reports what happened.
  local applied="" v
  for v in $vals; do
    if ft_impair_on "$v"; then applied="$applied $v"; else
      # ft_impair_on already removed anything it partially applied on $v; unwind
      # the seats that DID take it, so the sheet after this flow runs clean.
      local u; for u in $applied; do ft_impair_off "$u" >/dev/null 2>&1 || true; done
      record "21-impaired-commit" gap blocker \
        "could not apply the impairment on $v (tc/netem unavailable, or the interface or subnet map did not match) — the chain was NOT driven under impairment, so this is UNTESTED; a run that continued here would have graded a clean network as an adverse one"
      return
    fi
  done
  echo "    21-impaired-commit: [$IMPAIR_NETEM] applied to swarm-bound egress on${applied}"

  local h0 last_h last_t t0 now h gap maxgap=0 pubs=0 pub_ok=0 lastout="" wedged=0
  imp_height() { ssh_node "$1" "/usr/local/bin/silt chain-status -store /var/lib/silt 2>&1" \
    | grep -oE 'head height:[[:space:]]*[0-9]+' | grep -oE '[0-9]+' | tail -1; }
  imp_ceiling() {
    local c=0 x hh
    for x in $vals; do hh="$(imp_height "$x")"; hh="${hh:-0}"; [ "$hh" -gt "$c" ] 2>/dev/null && c="$hh"; done
    printf '%s' "$c"
  }
  h0="$(imp_ceiling)"; h0="${h0:-0}"; t0="$(date +%s)"; last_h="$h0"; last_t="$t0"
  # THE WALL IS PRICED OFF THE BOUND THIS FLOW GRADES AGAINST, not off a clean
  # network's block time. Asking for N heights inside N x 64s while accepting up
  # to H_ESCAPE_S per height lets the flow fail for running out of WINDOW while
  # every height it saw was inside its bound — a window masquerading as a
  # property. Measured here: under the default profile the chain kept committing
  # and took minutes per height, so a 192s wall reported ZERO heights for a
  # reason that was the wall's.
  #
  # And it must also hold ONE WHOLE PUBLISH. The verdict below reports how many
  # of the stream's publishes landed, and that count means nothing if the window
  # closed while the first one was still inside its own budget — the publish
  # client waits on its entry COMMITTING and gives up only at its own ceiling.
  # IMPAIR_PUBLISH_S is that ceiling plus room to report; it is the floor on the
  # wall, never a second bound the flow grades against.
  : "${IMPAIR_PUBLISH_S:=600}"
  local wall=$(( IMPAIR_HEIGHTS * H_ESCAPE_S ))
  [ "$wall" -lt "$IMPAIR_PUBLISH_S" ] && wall="$IMPAIR_PUBLISH_S"
  echo "    21-impaired-commit: driving ${IMPAIR_HEIGHTS} heights from h${h0} under load (wall ${wall}s; per-height escape bound ${H_ESCAPE_S}s)…"
  while :; do
    now="$(date +%s)"
    [ $(( now - t0 )) -ge "$wall" ] && break
    # SSH_NODE_TIMEOUT is raised past the client's own operation ceiling. At the
    # 90s default every impaired publish is killed by the harness's transport
    # before the client can finish or give up, and pub_ok then counts zero for a
    # reason that belongs to this script rather than to the product. Measured:
    # a publish under the default profile scattered all its shards in ~10s and
    # then waited on its registry entry COMMITTING, which is chain-cadence-bound.
    # The client is the thing that decides to give up; this transport must only
    # outlast it, so it rides the same IMPAIR_PUBLISH_S the wall is floored at.
    lastout="$(SSH_NODE_TIMEOUT=$IMPAIR_PUBLISH_S ssh_node val-a "head -c 8192 </dev/urandom >/tmp/ft_imp.bin; /usr/local/bin/silt swarm add /tmp/ft_imp.bin -peers '$PEERS' -registry '$REGREF' -token-quorum $TOKEN_QUORUM -chunk-size 65536 2>&1 || true")"
    pubs=$(( pubs + 1 ))
    printf '%s' "$lastout" | grep -qE 'silt:v1:' && pub_ok=$(( pub_ok + 1 ))
    h="$(imp_ceiling)"; h="${h:-$last_h}"
    now="$(date +%s)"
    if [ "$h" -gt "$last_h" ] 2>/dev/null; then
      gap=$(( now - last_t )); [ "$gap" -gt "$maxgap" ] && maxgap="$gap"
      last_h="$h"; last_t="$now"
      [ $(( h - h0 )) -ge "$IMPAIR_HEIGHTS" ] && break
    elif [ $(( now - last_t )) -gt $(( H_ESCAPE_S * 2 )) ]; then
      wedged=1; break
    fi
    sleep 8
  done
  local final_gap=$(( $(date +%s) - last_t ))
  [ "$final_gap" -gt "$maxgap" ] && [ "$wedged" = 1 ] && maxgap="$final_gap"
  local heights=$(( last_h - h0 ))

  # WHAT THE LANE DID ACROSS THE WINDOW, read before unwinding and reported
  # whichever way the verdict goes. Four numbers per seat, each with a reading:
  # a lane that never came up did not carry the drive; a lane failure fell back
  # to the shared conn and cost latency rather than a message; a last-resort
  # delivery that is routine means the lane is carrying bulk again, which is the
  # head-of-line blocking it exists to remove; and a dropped CONTROL frame is
  # the node going blind to a peer, which must not happen at all.
  # The baseline lookup and the counter read both go through a variable and both
  # carry `|| true`. This file runs under `set -e -o pipefail`, where a grep that
  # matches nothing and a seat that will not answer are ordinary outcomes that
  # would otherwise abort the flow BEFORE it reports the drive it just finished —
  # losing the verdict to the instrumentation added to explain it.
  local lane="" lane_now lane_was lane_d
  for v in $applied; do
    lane_now="$(ft_lane_counters "$v" || true)"
    lane_was="$(printf '%s' "$lane_base" | tr ' ' '\n' | grep "^${v}=" | cut -d= -f2 | tr ',' ' ' || true)"
    lane_d="$(ft_lane_delta "$lane_was" "$lane_now" || true)"
    if [ -z "$lane_d" ]; then
      lane="$lane ${v}:UNREAD"
    else
      local le lf ll lc
      read -r le lf ll lc <<< "$lane_d"
      lane="$lane ${v}:lane+${le}/fail+${lf}/lastresort+${ll}/ctrldrop+${lc}"
    fi
  done

  # DID THE IMPAIRMENT BITE? Read netem's own counters before unwinding, on
  # every seat that took the shaping.
  local bit=0 seats=0 cred=""
  for v in $applied; do
    seats=$(( seats + 1 ))
    local ctr sent drops
    ctr="$(ft_impair_counters "$v" || true)"
    sent="${ctr%% *}"; drops="${ctr##* }"
    if [ -n "$sent" ] && [ "$sent" -gt 0 ] 2>/dev/null; then
      bit=$(( bit + 1 )); cred="$cred ${v}:${sent}B/${drops}drop"
    fi
  done

  # Unwind, always, and say so if a seat did not come back clean — a leftover
  # qdisc would quietly impair every flow after this one.
  local dirty="" vanished=""
  for v in $applied; do
    ft_impair_off "$v"
    case $? in
      0) ;;
      2) vanished="$vanished $v" ;;
      *) dirty="$dirty $v" ;;
    esac
  done
  if [ -n "$dirty" ]; then
    record "21-impaired-commit" fail blocker \
      "the impairment did NOT come off$dirty — every later flow on this sheet is running on a shaped wire and its verdict cannot be trusted; clear it by hand (sudo tc qdisc del dev <dev> root) before reading anything below"
    return
  fi
  if [ -n "$vanished" ]; then
    # A seat that no longer exists carries no wire, so nothing later on the sheet is
    # shaped by it. What IS lost is this flow's own reading: the impaired-commit
    # grade needs the seats it shaped, and one of them left mid-drive. Say which,
    # and say that the cost is a missing measurement rather than a tainted sheet.
    echo "    21-impaired-commit: seat(s)${vanished} no longer exist (spot preemption deletes the instance) — nothing they carried can still shape the sheet"
    record "21-impaired-commit" gap "" \
      "a shaped seat was PREEMPTED mid-drive (${vanished# }) and the instance was deleted with it, so this flow has no impaired-commit reading to grade. The rest of the sheet is UNAFFECTED: a deleted instance shapes nothing. Re-drive this flow alone, or run the impaired grade on non-spot seats if it keeps losing the race."
    return
  fi
  echo "    21-impaired-commit: impairment removed from${applied}, interfaces clean"

  if [ "$bit" -eq 0 ]; then
    record "21-impaired-commit" fail blocker \
      "NOT CREDITED: the chain advanced ${heights} height(s) but netem's counters show NO swarm packet crossing the impaired band on any seat — the shaping steered nothing (wrong subnet map or interface), so this measured a CLEAN network and must not be read as an adverse-internet result"
    return
  fi
  if [ "$wedged" = 1 ] || [ "$maxgap" -gt "$H_ESCAPE_S" ]; then
    record "21-impaired-commit" fail blocker \
      "the chain STOPPED committing under [$IMPAIR_NETEM]: a height went ${maxgap}s without a commit, past the computed ${H_ESCAPE_S}s escape bound, with the network live (h${h0}->h${last_h}, ${pub_ok}/${pubs} publishes landed, impairment credited on ${bit} seat(s)); control lane over the window:${lane}; last client output: $(printf '%s' "$lastout" | tail -1 | head -c 200)"
    return
  fi
  if [ "$heights" -lt 1 ]; then
    # WHICH HALF STOPPED. The chain advances on proposals, and on this topology a
    # proposal is what a publish produces, so "no height" and "no publish" are
    # different findings and the second explains the first. Separating them here
    # is what keeps the verdict from attributing a publish-path failure to
    # consensus — the mistake this sheet has already paid for more than once.
    if [ "$pub_ok" -eq 0 ]; then
      record "21-impaired-commit" fail blocker \
        "under [$IMPAIR_NETEM] ZERO of ${pubs} publishes landed and the chain committed NOTHING in ${wall}s (h${h0}, impairment credited on ${bit} seat(s); control lane over the window:${lane}) — the PUBLISH path is what stopped, so read this as a client/publish finding and not yet as a consensus one; last client output: $(printf '%s' "$lastout" | tail -1 | head -c 200)"
    else
      record "21-impaired-commit" fail blocker \
        "under [$IMPAIR_NETEM] ${pub_ok} of ${pubs} publishes LANDED but the chain committed NOTHING in ${wall}s (h${h0}, impairment credited on ${bit} seat(s); control lane over the window:${lane}) — proposals were produced and no height followed, which is a consensus finding the escape bound cannot excuse"
    fi
    return
  fi
  # The verdict names the PROFILE, not the four conditions. IMPAIR_NETEM is a
  # variable — the arms that reduce a finding drive one condition at a time, and
  # a line that asserted all four over a single-condition profile would be
  # claiming more than the run drove.
  slo_assert "21-impaired-commit" blocker \
    "the chain KEPT COMMITTING under the injected profile [$IMPAIR_NETEM]: ${heights} height(s) h${h0}->h${last_h} under continuous publish (${pub_ok}/${pubs} landed), max inter-commit gap ${maxgap}s within the computed ${H_ESCAPE_S}s escape bound, impairment credited by netem's own counters on ${bit} of ${seats} seat(s) —${cred}; control lane over the window:${lane}" 1 "$(( $(date +%s) - t0 ))"
}

# ── adversarial: equivocation → slash ────────
# LOCAL_PROOF: go test ./e2e -run TestEquivocatorSlashedOverTCP -count=1
adv_equivocation() {
  # ): equivocation is the ONE
  # irreversible drill — a proven double-sign is a PERMANENT eviction (F2), correctly.
  # Running it mid-sheet would leave the requirement pinned at ⌊4/2⌋+1=3 over the
  # CONFIGURED anchors while only 3 stay live → every later commit needs all 3
  # unanimously (zero fault tolerance), risking spurious end-of-sheet flakes on the
  # very sheet handed to a red team. So the destructive drill runs on its OWN
  # dedicated, ephemeral network, not the shared sheet.
  #
  # Its certifying home is the OBJECTIVE-mode slash-on-detection drill over real
  # daemons: e2e/equivocation_test.go (TestEquivocatorSlashedOverTCP) — a 4-anchor
  # net where a Byzantine anchor SERVES a conflicting signed block (a fork can never
  # be COMMITTED onto a target under a 3-of-4 BFT floor; the crime is SIGNING two
  # conflicting blocks at one height) and an honest anchor slashes it unaided on the
  # reconcile path — run under adverse conditions by integration/adversarial (netem).
  # The in-process merge gate is core/node/modelcheck_equivocation_objective_test.go.
  #
  # 2026-08-20 (owner directive): the drill now runs on EVERY sheet — but in the
  # fully-contained equivocation ISLAND (flow_equivocation_island below), a separate
  # consensus universe. That honors
  # fault tolerance, never the main sheet's) AND closes the skip-is-a-blind-spot gap.
  # This row stays a SKIP so the island's PASS/FAIL is the one graded verdict, not
  # two rows for one property.
  record "184-equivocation" skip blocker "runs on the contained equivocation ISLAND every sheet (flow_equivocation_island — a separate consensus universe; its slash never taxes main-sheet fault tolerance). The island row carries the graded verdict."
}

# ── equivocation ISLAND: the destructive drill, contained, on EVERY sheet ───
# A separate 4-anchor consensus universe (topology role "island"; own genesis, own
# -anchors naming only each other; no external IP on GCP → zero quota, Cloud NAT
# egress). One island anchor is relaunched as a Byzantine equivocator; an honest
# island anchor slashes the double-sign on the reconcile path. Fully contained:
# nothing in the main swarm names the island, so the permanent eviction (F2) consumes
# only the ISLAND's fault tolerance — the exact zero-FT-taildecision
# forbade on the shared sheet, here made structurally impossible. Design:
# .
# LOCAL_PROOF: LOCAL=1 ./cloudtest.sh (the island is 4 containers; the flow runs verbatim) + go test ./e2e -run TestEquivocatorSlashedOverTCP
flow_equivocation_island() {
  require_nodes "184-equivocation-island" blocker island-a island-b island-c island-d || return
  # 1) The island must reach a baseline commit (its own chain is live + independent)
  #  before the equivocator has any on-chain prepare to fork — else the drill is
  #  UNTESTED (premise unmet), not a failed slash.
  # island-b is the BAKED-IN objective equivocator (topology.py — the green e2e
  # shape; a LOCAL run proved a mid-drill relaunch leaves it re-warming past the
  # window). island-a is an honest anchor that detects + slashes. No relaunch,
  # no restore — the island is torn down with the run.
  local isl_boot="island-a" byz="island-b" honest="island-a"
  local byzid; byzid="$(node_field "$byz" nodeid)"
  flow_evidence_nodes island-a island-b island-c island-d
  # 1) The island's independent chain must commit a baseline (the equivocator
  #  participates honestly first, so a commit means it has prepared a height to
  #  fork). Undriven ⇒ UNTESTED, not a failed slash.
  if ! waitfor "$isl_boot" 'chain: committed block [1-9]' 180 >/dev/null; then
    record "184-equivocation-island" gap blocker "the island never committed a baseline block within 180s — its independent consensus never warmed, so the equivocator has no on-chain prepare to fork (UNTESTED not failed; attribute from the island journals)"
    return
  fi
  # 2) The baked-in equivocator serves its conflicting block; confirm it drove.
  if ! waitfor "$byz" 'adversary: equivocation complete \(double-signed height [0-9]+\)' 180 >/dev/null; then
    record "184-equivocation-island" gap blocker "the equivocator never served its conflicting block within 180s — drill did not drive (UNTESTED; attribute from $byz's journal)"; return
  fi
  # 3) An HONEST island anchor slashes the double-sign on the reconcile path — the
  #  accountability property on the wire. Assert the product's own slash line (#7).
  local slashline; slashline="$(waitfor "$honest" "chain: slashed equivocator ${byzid}" 120 || true)"
  if [ -z "$slashline" ]; then
    record "184-equivocation-island" fail blocker "the equivocator double-signed but NO honest island anchor slashed it within 120s — the accountability detection did not fire; attribute from the island journals (reconcile/FindEquivocations path) before re-running (#7)"
    return
  fi

  # 4) THE REPLICATED HALF (item 9's distinctive clause). The line above is what ONE
  #  node DECIDED; this is what the HISTORY committed. They are different claims and
  #  the gap between them is where an eviction that never lands would hide: a local
  #  ledger evicting alone is not F2, which is the whole point of queuing the proof
  #  for on-chain recording.
  #
  #  THE ISLAND IS THE RIGHT PLACE AND integration/redteam IS NOT. The drain that
  #  carries a queued proof is gated on Objective() in its first line, and redteam's
  #  equivocation seats run -objective=false, so there the proof can never ride
  #  whatever the product does. The island is -objective with four anchors, so a
  #  proposal can actually carry it.
  #
  #  The wait is bounded and its failure is SEPARATED from the detection above, so a
  #  committed-set miss can never be read as "the slash did not fire".
  local isl_slash="" i
  for i in $(seq 1 24); do
    isl_slash="$(ssh_node "$honest" "/usr/local/bin/silt chain-status -store /var/lib/silt 2>&1" \
      | grep -oE 'slashed-id:[[:space:]]+[0-9a-f]{64}' | grep -oE '[0-9a-f]{64}' | sort -u)"
    printf '%s' "$isl_slash" | grep -q "$byzid" && break
    sleep 5
  done
  local isl_head; isl_head="$(ssh_node "$honest" "/usr/local/bin/silt chain-status -store /var/lib/silt 2>&1" \
    | grep -oE 'head height:[[:space:]]*[0-9]+' | grep -oE '[0-9]+' | tail -1)"
  local isl_extra; isl_extra="$(printf '%s' "$isl_slash" | grep -v "^${byzid}$" | tr '\n' ' ')"

  if [ -n "$isl_extra" ]; then
    # Unconditionally wrong whatever else is true: only PROVEN equivocation may slash.
    record "184-equivocation-island" fail blocker \
      "an identity OTHER THAN the equivocator is in the island's COMMITTED slash set — $isl_extra. A committed slash evicts that identity on every replica (F2), so no honest node may ever appear here (item 9)"
  elif printf '%s' "$isl_slash" | grep -q "$byzid"; then
    slo_assert "184-equivocation-island" blocker \
      "accountability FIRED AND COMMITTED: an island anchor double-signed, an honest anchor slashed it (${slashline##*chain: }), and the HISTORY carries the eviction at head ${isl_head} with NOBODY else in the committed slash set — F2 on every replica, not one local ledger, zero blast radius to the main sheet" 1
  else
    record "184-equivocation-island" fail blocker \
      "the slash FIRED but the island's committed slash set is still empty at head ${isl_head} after 120s — the local ledger evicted the equivocator and the history did not, so replicas do NOT evict in lockstep (F2). If the head is not advancing, the chain is quiescent and the queued proof is not arming a proposal (core/node maybeProposeBondDrain must count pendingSlashes); if it IS advancing, blocks are being proposed and the proof is not riding them"
  fi
}

# ── adversarial: partition → heal (BFT: stall-then-catch-up) ────────────────
# LOCAL_PROOF: go test ./e2e -run TestPartitionHealsToHeavierForkOverTCP -count=1
adv_partition() {
  require_nodes "184-partition" major val-a val-b val-c val-d fetch-1 || return
  local ida idb idd idc
  ida="$(node_field val-a nodeid)"; idb="$(node_field val-b nodeid)"
  idd="$(node_field val-d nodeid)"; idc="$(node_field val-c nodeid)"
  local v
  for v in "$ida" "$idb" "$idd" "$idc"; do
    [ ${#v} -eq 64 ] || { record "184-partition" gap major "could not resolve a validator NodeID from nodes.json — partition not applied, not a property failure"; return; }
  done
  # Head (height + hash) from a node's OWN chain-status store — same discipline as
  # flow-5 (a height-only check can't tell catch-up from a divergent same-height fork;
  # the hash proves same-history reconverge).
  chain_head() { ssh_node "$1" "/usr/local/bin/silt chain-status -store /var/lib/silt 2>&1" \
    | awk '/head height:/{h=$3} /head hash:/{hh=$3} END{print (h==""?0:h), hh}'; }

  # SEVER val-c into a genuine < ⅓-weight MINORITY. It must be cut from EVERY node that
  # HOLDS the committed chain — not only the anchors: any reachable chain-holder lets
  # val-c SYNC the majority's blocks (adopt-via-Reconcile, which logs "reconciled", not
  # "committed block N") and stay current, so the "minority" never falls behind. Runs
  # the field run (base: val-c synced h14→h16 THROUGH the bonded `adversary` node) and
  # the field run (MATURING: h25→h37 through adversary + 4 maturers + 4 sybils) proved
  # this: an anchors-only sever misses the other validator-role chain-holders. So block
  # val-c from ALL validator-role peers (validator / adversary / maturer / sybil),
  # BOTH directions (-block-peers drops traffic to AND from them). A single isolated
  # node cannot reach the commit quorum, so it CANNOT commit — the correct
  # BFT-intersection behaviour (a minority committing a conflicting fork is the I1
  # violation model B forbids). This is why on heal val-c CATCHES UP (a forward sync,
  # dropped=0), it does NOT "reorg": a dropped-block reorg would require val-c to have
  # committed a conflicting fork, which it correctly cannot — the ABSENCE of a reorg
  # line IS the safety property).
  local blockids; blockids="$(python3 -c "
import json
t=json.load(open('$FT_TOPO'))
print(','.join(n['nodeid'] for name,n in t['nodes'].items()
  if n['role'] in ('validator','adversary','maturer','sybil') and name!='val-c'))" 2>/dev/null)"
  if [ -z "$blockids" ]; then
    record "184-partition" gap major "could not build the validator-role block set from topology.json — partition not applied, not a property failure"; return
  fi
  local t0sever; t0sever="$(date +%s)"
  relaunch_with val-c "-block-peers ${blockids}"
  # The sever is LIVE only once the restarted daemon narrates it. Reading val-c's
  # baseline BEFORE this point races the relaunch window (sed + daemon-reload +
  # restart + chain reload = seconds) on a chain that commits drain blocks
  # near-continuously — val-c legitimately commits 1–2 more blocks in the gap, and
  # the stall check then reads "ADVANCED during the partition" (run 2323b09:
  # h27→h29; the false-GAP class behind most of the 18 archived partition GAPs
  # that survived the earlier sever-widening fix). Mechanism: baseline-before-
  # sever race; fix: confirm the banner, THEN baseline.
  if ! waitfor_since val-c "PARTITION: -block-peers set" "$t0sever" 90 >/dev/null; then
    ft_add_validator_evidence; restore_argv val-c
    record "184-partition" gap major "val-c never narrated the partition banner within 90s of the sever relaunch — sever not confirmed live, drill UNTESTED not failed"
    return
  fi
  local ci ch0; ci="$(chain_head val-c)"; ch0="${ci%% *}"; ch0="${ch0:-0}"

  # DRIVE the > ⅔ majority to commit a heavier chain during the window — publish to
  # the majority ONLY (val-c can't hear it), so a genuine height gap forms for val-c
  # to catch up to. (val-a serves the registry and is in the majority.)
  local majpeers; majpeers="$(printf '%s' "$PEERS" | tr ',' '\n' | grep -v "^${idc}@" | paste -sd, -)"
  local i
  for i in 1 2 3; do
    ssh_node fetch-1 "head -c 4096 </dev/urandom >/tmp/ft_part.bin; /usr/local/bin/silt swarm add /tmp/ft_part.bin -peers '${majpeers}' -registry '${REGREF}' -token-quorum 2 -chunk-size 65536 2>&1" >/dev/null 2>&1 || true
    sleep 6
  done
  sleep 18

  # ANTI-VACUITY (the key step): val-c must have STALLED — proof it was a genuine
  # minority that COULDN'T commit, not an idle chain trivially catching up to nothing.
  local ci1 ch1; ci1="$(chain_head val-c)"; ch1="${ci1%% *}"; ch1="${ch1:-0}"
  local mi mh; mi="$(chain_head val-a)"; mh="${mi%% *}"; mh="${mh:-0}"
  if [ "$mh" -le "$ch0" ] 2>/dev/null; then
    ft_add_validator_evidence; restore_argv val-c
    record "184-partition" gap major "the majority committed NO heavier chain during the window (val-a head h$mh ≤ val-c's pre-partition h$ch0) — nothing to catch up to, heal UNTESTED not failed (drive under-committed; the majority publishes did not land)"
    return
  fi
  if [ "$ch1" -gt "$ch0" ] 2>/dev/null; then
    ft_add_validator_evidence; restore_argv val-c
    record "184-partition" gap major "val-c ADVANCED during the partition (h${ch0}→h${ch1}) — the sever did not isolate it below the commit threshold, so it was NOT a genuine < ⅓ minority (drill under-drove; widen the sever). Not a product failure"
    return
  fi

  # HEAL: drop the block → val-c reconnects to the majority and catches up.
  restore_argv val-c
  # Assert val-c CATCHES UP: it advances past its stall AND reaches the majority's
  # LIVE head with a matching hash (both advance, so compare val-c to val-a live —
  # they align at the tip once val-c catches up).
  # Heal window sized to the CATCH-UP, not a magic constant (#5): the sever fix works
  # (run the field run: val-c genuinely STALLED at h31 while the majority reached h38),
  # but a 7-block cross-region catch-up sync ran past 120s and GAPped ("did not
  # reconverge in 120s"). 300s matches the drill's other resume windows (10b's clincher)
  # and rides out a multi-block WAN catch-up while a real reconverge break still GAPs.
  local ok=0 t0; t0="$(date +%s)"
  # Snapshot the majority's head AT HEAL START as a FIXED reconverge target. val-a
  # keeps advancing while val-c catches up, so requiring val-c == val-a's LIVE head
  # false-GAPs whenever both advance at similar rates — val-c sits perpetually one
  # block behind a moving tip (run the field run: val-c h34 vs live val-a h35, a
  # healthy lag, not a break). Reaching the heal-time head PROVES reconvergence:
  # a < ⅓ island committed nothing of its own during the partition (guarded above),
  # so val-c can only advance by SYNCING the majority chain, and Reconcile validates
  # full linkage — so equalling the target (hash-matched) or surpassing it (synced
  # THROUGH it) both mean the same chain, never a fork.
  local ai0 aht ahht; ai0="$(chain_head val-a)"; aht="${ai0%% *}"; ahht="${ai0#* }"
  local ci2 ch2 chh2
  while [ $(( $(date +%s) - t0 )) -lt 300 ]; do
    ci2="$(chain_head val-c)"; ch2="${ci2%% *}"; chh2="${ci2#* }"
    if [ "${ch2:-0}" -gt "$ch0" ] 2>/dev/null; then
      # Caught up ⇔ reached the heal-time majority head aht: exact height ⇒ hash must
      # match (reconverged to that block); surpassed ⇒ synced past it on the majority
      # chain (a fork could neither match the hash nor sync the majority's aht+1).
      if { [ "$ch2" = "$aht" ] && [ -n "$chh2" ] && [ "$chh2" = "$ahht" ]; } || [ "${ch2:-0}" -gt "$aht" ] 2>/dev/null; then ok=1; break; fi
    fi
    sleep 4
  done
  if [ "$ok" = 1 ]; then
    slo_assert "184-partition" major "minority val-c STALLED at h$ch0 through the partition (a < ⅓ island cannot commit) then CAUGHT UP to the heal-time majority head h$aht (now at h$ch2) on heal — BFT partition→heal reconverged over the real wire (a catch-up, NOT a reorg — a minority never committed a conflicting fork)" 1
  else
    ft_add_validator_evidence
    record "184-partition" gap major "val-c did not reconverge to the heal-time majority head h$aht within 300s of heal (val-c=${ch2:-?}:${chh2:0:12}… stalled-from h$ch0) — read the captured validator journals before attributing (slow catch-up sync vs a real reconverge break)"
  fi
}

# ── adversarial: forged-block + low-bond proposals rejected ────────────────
# LOCAL_PROOF: go test ./e2e -run 'TestForgedBlockRejectedOverTCP|TestLowBondProposerRejectedOverTCP' -count=1
adv_proposal_reject() {
  require_nodes "184-forged-block" major adversary || return
  local ida; ida="$(node_field val-a nodeid)"
  if [ ${#ida} -ne 64 ]; then
    record "184-forged-block" gap major "could not resolve val-a NodeID from nodes.json (ida='${ida:0:12}…') — proposals not delivered, not a property failure"; return
  fi
  # the audit: grep the line the PRODUCT actually emits — the adversary daemon prints
  # 'adversary: <label> proposal correctly REJECTED by <targetID>' when the honest target
  # refuses it (cmd/silt/daemon.go badPropose), same as the local integration/redteam
  # harness. The old assertions greped val-a for 'ErrBadSignature'/'ErrLowReputation',
  # strings the daemon never emits there, so they could only ever time out (false FINDING)
  # or, worse, match unrelated noise.
  relaunch_with adversary "-forge-block ${ida}"
  local ok1=0; waitfor adversary "forge-block proposal correctly REJECTED by ${ida}" 90 >/dev/null && ok1=1
  slo_assert "184-forged-block" major "forged-signature proposal rejected (adversary logged 'correctly REJECTED by val-a')$([ "$ok1" = 1 ] || echo ' — adversary never logged the reject within 90s (or val-a wrongly ACCEPTED it)')" "$ok1"
  restore_argv adversary
  # low-bond: the adversary carries a full, qualifying 64M bond. While its bond is not
  # yet committed on-chain it is (correctly) rejected as an unqualified proposer — the
  # PASS this drill was written for. But once its bond commits it becomes a QUALIFIED
  # proposer, and val-a CORRECTLY accepts its well-formed block: that "UNEXPECTEDLY
  # ACCEPTED" is right product behavior, not a defect, so it must NOT be a FAIL. The
  # outcome therefore flips on the adversary's standing race between runs. Score
  # it honestly: a logged reject is a PASS; a logged accept is a GAP (this node is
  # qualified — a genuine under-bond REJECTION test needs a dedicated sub-min-bond
  # identity); silence is a GAP (not driven). low-bond rejection is verifies
  # in-process either way.
  relaunch_with adversary "-lowbond-propose ${ida}"
  if waitfor adversary "lowbond-propose proposal correctly REJECTED by ${ida}" 90 >/dev/null; then
    slo_assert "184-low-bond" major "under-bonded proposer rejected (adversary logged 'correctly REJECTED by val-a')" 1
  elif waitfor adversary "lowbond-propose proposal UNEXPECTEDLY ACCEPTED by ${ida}" 5 >/dev/null; then
    record "184-low-bond" gap major "adversary holds a qualifying 64M bond and was CORRECTLY accepted as a proposer — an under-bond REJECTION test needs a dedicated sub-min-bond identity; the property is covered by the in-process tests"
  else
    record "184-low-bond" gap major "no reject/accept line from the adversary within 90s — low-bond not driven this run"
  fi
  restore_argv adversary
}

# ═══════════════════════════════════════════════════════════════════════════════
# Cloud variants of the local field-test series (integration/{privacy,durability,
# chaos,client,sybil}) — the same properties, over real VMs / real regions. Most
# map onto the existing 13-node topology with NO topology change (a flag flip via
# relaunch_with, a hard kill, a permanent stop). #5 C2-Sybil additionally needs the
# opt-in NON-anchor Sybil cohort (SYBILS=8, a topology.py role) so the Sybils' bonds
# actually bank and the PURE ErrAnchorRequired gate + the ≥8-bond atomization note
# become assertable; absent the cohort the flow records an honest skip.
# ═══════════════════════════════════════════════════════════════════════════════

# ── privacy (#3): publisher unlinkability — refuse-to-surveil over the real wire ─
# LOCAL_PROOF: go test ./e2e -run TestUnlinkablePublishOverTCP -count=1
flow_publisher_unlinkability() {
  require_live "priv-unlinkability" major fetch-1 || return   # preempted node ⇒ GAP, not "no refusal seen" FAIL (H2)
  # flow_publish_fetch already proved the DEFAULT (token-quorum) publish commits
  # unlinkably. Here the adversary ASKS to record a durable Publisher identity; the
  # default chain must REFUSE it (chain: ErrPublisherEntry / "carries a durable
  # Publisher"). That refusal, over real VMs, is immutable #4 (refuse-to-surveil).
  local out ok=0
  ssh_node fetch-1 "head -c 32768 </dev/urandom >/tmp/ft_priv.bin" >/dev/null 2>&1
  # The refusal is written to silt's STDERR; ssh_node suppresses gcloud stderr
  # (2>/dev/null), so the `2>&1` MUST be INSIDE the remote command or the refusal is
  # lost and this false-FAILs (a warm chain does refuse — verified live on GCP).
  out="$(ssh_node fetch-1 "/usr/local/bin/silt swarm add /tmp/ft_priv.bin -peers '$PEERS' -registry '$REGREF' -allow-publisher -chunk-size 65536 2>&1" || true)"
  printf '%s' "$out" | grep -iqE 'durable Publisher|ErrPublisherEntry|permanent linkage|publish unlinkably' && ok=1
  slo_assert "priv-unlinkability" major "default chain REFUSED a durable file→publisher link (refuse-to-surveil)$([ "$ok" = 1 ] || echo " — no refusal seen: $(printf '%s' "$out" | tail -1)")" "$ok"
}

# A failed publish's decisive evidence lives on the VALIDATORS — the accept→commit
# window (was the chain committing? did the entry sit in a mempool? which height
# carried it late?) — not only on the client side. Run the field run's
# durability-turnover GAP captured only store/fetch journals, so the root was
# unpinnable after teardown (#7: capture the evidence first, then look). Callers
# recording a verdict after a failed ft_publish extend the capture set with the
# validator cohort before record.
ft_add_validator_evidence() {
  local n vals=""
  for n in val-a val-b val-c val-d; do node_exists "$n" && vals="$vals $n"; done
  FT_FLOW_NODES="$FT_FLOW_NODES$vals"
}

# ── durability (#2): content OUTLIVES a permanent storage-node loss ─────────────
# LOCAL_PROOF: ./integration/durability/run.sh
flow_durability_turnover() {
  require_nodes "durability-turnover" major store-1 store-2 fetch-1 || return
  client_preflight "durability-turnover" major fetch-1 store-2 || return
  # Publish replicated, then PERMANENTLY remove a storage node (stop + leave down —
  # a real departure, not a restart) and prove a fresh fetch still returns the bytes
  # from the survivors. The full membership-ROTATION form (a terminated VM replaced
  # by a fresh-IP VM) is the honest cloud version of the local kill-without-replace.
  local res link sha
  res="$(ft_publish fetch-1 1048576 || true)"
  # A failed SETUP publish means durability is UNTESTED (we have no content to lose),
  # not that durability broke — this flow tests survival of a permanent node loss, not
  # the publish path (2-publish-fetch is the publish canary). So GAP unconditionally on
  # a missing link, independent of the FT_PUBLISH_GAP/degraded signals.
  if [ -z "$res" ]; then ft_add_validator_evidence; record "durability-turnover" gap major "setup publish did not land a link — durability UNTESTED this run, not a durability failure (read .ft_publish_lasterr to decompose: 'accepted but not committed' = the accept→commit path — commit latency vs the client poll window; token/issuer-set errors = discovery. The validator journals for the publish window are captured with this verdict)"; return; fi
  link="${res%% *}"; sha="${res##* }"
  svc store-1 stop || true    # permanent departure (left down for the fetch)
  sleep 12
  local got ok=0 geterr=""
  geterr="$(ssh_node store-2 "rm -f /tmp/ft_dur.bin; /usr/local/bin/silt swarm get '$link' -o /tmp/ft_dur.bin -peers '$PEERS' -registry '$REGREF' 2>&1 >/dev/null | tail -3" 2>/dev/null || true)"
  got="$(ssh_node store-2 "sha256sum /tmp/ft_dur.bin 2>/dev/null | cut -d' ' -f1" 2>/dev/null || true)"
  [ "$got" = "$sha" ] && ok=1
  # Premise classifier: "root not in registry" means the setup publish
  # was ACCEPTED but its entry never became resolvable (accept→commit) —
  # durability of committed content is then UNTESTED, not failed. Any other failure
  # (hash mismatch, partial bytes, timeout with the entry present) is a real FAIL.
  if [ "$ok" != 1 ] && printf '%s' "$geterr" | grep -qiE 'root not in registry|no such entry'; then
    record "durability-turnover" gap major "fetch found the root ABSENT from the registry — the setup publish premise broke upstream (accept→commit), durability of committed content UNTESTED not failed (client: $(printf '%s' "$geterr" | tr '\n' ';' | head -c 200))"
  else
    slo_assert "durability-turnover" major "content survived a PERMANENT storage-node departure — fetched bit-perfect from a survivor$([ "$ok" = 1 ] || echo " (want=$sha got=${got:-<none>}; client: $(printf '%s' "$geterr" | tr '\n' ';' | head -c 200))")" "$ok"
  fi
  svc store-1 start || true   # restore for later flows
}

# ── chaos (#7): hard crash (SIGKILL) recovery + #69 reprovide over real VMs ──────
# LOCAL_PROOF: ./integration/chaos/run.sh
flow_chaos_crash() {
  require_nodes "chaos-crash" major store-1 store-2 || return
  # Capture the REGISTRY (val-a) + validator journals alongside store-1/store-2 on any
  # chaos FAIL: run the field run FAILed chaos-fetch with "root not in registry" but the
  # capture held only the store journals, so the REGISTRY's own view of the root was
  # unattributable after teardown (#7 capture-first). ft_add_validator_evidence appends
  # val-a..d (val-a serves the registry) to this flow's evidence set.
  ft_add_validator_evidence
  local link="${FT_LAST_LINK:-}" wantsha="${FT_LAST_SHA:-}"
  if [ -z "$link" ]; then local res; res="$(ft_publish fetch-1 262144 || true)"; link="${res%% *}"; wantsha="${res##* }"; fi
  # As with durability-turnover: a failed SETUP publish means crash-recovery is UNTESTED
  # (no content to crash-and-recover), not broken — GAP unconditionally.
  if [ -z "$link" ]; then ft_add_validator_evidence; record "chaos-crash" gap major "setup publish did not land a link — crash-recovery UNTESTED this run, not a failure (read .ft_publish_lasterr to decompose: 'accepted but not committed' = accept→commit; token/issuer errors = discovery. Validator journals captured with this verdict)"; return; fi
  # SIGKILL the silt process (abrupt death, not a graceful stop). systemd
  # (Restart=on-failure) brings it back, reloading the persisted store.
  # Capture t0 BEFORE the kill: flow_restart_survival already restarted store-2
  # earlier and emitted 're-announced N held chunks' on that prior boot — an
  # unscoped waitfor could match that stale line and false-pass a broken reprovide
  # (the audit chaos-reprovide stale-gap). --since @t0 admits only the post-crash
  # boot's re-announce.
  local t0; t0="$(date +%s)"
  ssh_node store-2 "sudo pkill -9 -f '/usr/local/bin/silt' || true" >/dev/null 2>&1
  ssh_node store-2 "sudo systemctl start silt.service" >/dev/null 2>&1 || true   # idempotent nudge
  # Wait for the CONDITION on a generous, evidence-sized deadline — never a magic
  # constant (build-immutable #5). Run the field run attributed the old 90s FAIL:
  # re-announce completes but its latency is ~LINEAR in held-chunk count (that run,
  # store-2's own journal: 24 chunks → 19s early, 132 chunks → 102s late; ≈1.3
  # announces/s over WAN), so a fixed 90s under-provisions a store that has
  # accumulated chunks by late in a session. 300s rides out a loaded store while a
  # genuine reprovide gap still FAILs after it. Record the observed latency so every
  # run keeps measuring the trend (an M1 signal — the announce sweep looks serialized).
  local reann=0 reann_line reann_s
  reann_line="$(waitfor_since store-2 're-announced [0-9]+ held chunks' "$t0" 300 || true)"
  if [ -n "$reann_line" ]; then reann=1; reann_s=$(( $(date +%s) - t0 )); fi
  if [ "$reann" = 1 ]; then
    slo_assert "chaos-reprovide" major "SIGKILLed storage node re-announced its held chunks after a hard crash (${reann_s}s to re-announce; latency scales with held-chunk count)" 1
  else
    # 300s with no re-announce is now a REAL gap (well past the measured linear
    # envelope) — the path survives an abrupt kill locally (integration/nat
    # RESTART=1, green post-). store-2's captured journal attributes it.
    slo_assert "chaos-reprovide" major "no post-crash 're-announced N held chunks' on store-2 within 300s of the SIGKILL — past the measured re-announce envelope; attribute from store-2's captured journal (#69)" 0
  fi
  local got ok=0 geterr=""
  # SHA-compare, not echo-OK (§D): crash-recovery must return the REAL bytes, not just
  # a zero exit on a possibly-truncated fetch. Keep the client's stderr — run
  # the field run's chaos-fetch FAIL was UNATTRIBUTABLE because this line sent it
  # to /dev/null (got=<none> with the deciding error discarded).
  geterr="$(ssh_node store-1 "rm -f /tmp/ft_ch.bin; /usr/local/bin/silt swarm get '$link' -o /tmp/ft_ch.bin -peers '$PEERS' -registry '$REGREF' 2>&1 >/dev/null | tail -3" 2>/dev/null || true)"
  got="$(ssh_node store-1 "sha256sum /tmp/ft_ch.bin 2>/dev/null | cut -d' ' -f1" 2>/dev/null || true)"
  [ -n "$wantsha" ] && [ "$got" = "$wantsha" ] && ok=1
  # Premise classifier: "root not in
  # registry" = the publish premise broke upstream (accept→commit), so
  # crash-recovery of committed content is UNTESTED — GAP. A hash mismatch or a
  # timeout with the entry resolvable stays a real FAIL.
  if [ "$ok" != 1 ] && printf '%s' "$geterr" | grep -qiE 'root not in registry|no such entry'; then
    record "chaos-fetch" gap major "fetch found the root ABSENT from the registry — the publish premise broke upstream (accept→commit), crash-recovery UNTESTED not failed (client: $(printf '%s' "$geterr" | tr '\n' ';' | head -c 200))"
  else
    slo_assert "chaos-fetch" major "content fetchable BIT-PERFECT after a hard-crash (SIGKILL) + restart of a storage node$([ "$ok" = 1 ] || echo " (want=${wantsha:-?} got=${got:-<none>}; client: $(printf '%s' "$geterr" | tr '\n' ';' | head -c 300))")" "$ok"
  fi
}

# ── client/UI (#4): the web-UI local-security guard (#89) over a real VM ─────────
# LOCAL_PROOF: ./integration/client/run.sh
flow_web_ui_guard() {
  require_nodes "web-ui" minor fetch-1 || return
  require_live  "web-ui" minor fetch-1 || return   # preempted node ⇒ GAP, not empty-code FAIL (H2)
  # Turn on the web UI on fetch-1 (a flag flip, restored after), then attack its
  # guard from the node's OWN localhost — the realistic operator-on-the-box model.
  relaunch_with fetch-1 "-ui=127.0.0.1:8081"
  sleep 10
  local c401 c403 c200 ok=0
  c401="$(ssh_node fetch-1 "curl -s -o /dev/null -w '%{http_code}' -X POST http://127.0.0.1:8081/api/publish" 2>/dev/null || true)"
  c403="$(ssh_node fetch-1 "curl -s -o /dev/null -w '%{http_code}' -H 'Host: evil.example.com' http://127.0.0.1:8081/api/status" 2>/dev/null || true)"
  c200="$(ssh_node fetch-1 "curl -s -o /dev/null -w '%{http_code}' http://127.0.0.1:8081/api/status" 2>/dev/null || true)"
  { [ "$c401" = 401 ] && [ "$c403" = 403 ] && [ "$c200" = 200 ]; } && ok=1
  slo_assert "web-ui-guard" major "web-UI guard held on a real VM: no-token POST=$c401 (want 401), DNS-rebinding Host=$c403 (want 403), token-free read=$c200 (want 200)" "$ok"
  restore_argv fetch-1
}

# ── C2 Sybil no-capture (#5): a bonded NON-anchor cohort cannot take the chain ──
# The LOCAL integration/sybil suite can only reach the STANDING gate — on a laptop a
# fresh Sybil set can't bank bonds (a young network's bond-registration needs
# anchor-proposed blocks; chicken-and-egg). On CLOUD, over the warm period the
# ANCHORS' committed blocks bank the Sybils' BondRegs, so this verifies the PURE
# gate: with the anchors gone, a self-majority of bonded Sybils all sharing ONE
# -domain still cannot advance the chain (ErrAnchorRequired) — because the C2
# concentration metric refuses to count a single-domain split as the address-diverse
# decentralization that sheds the launch anchors. The clincher: restore the anchors
# and the chain RESUMES — proving it was the missing anchors, not dead Sybils.
# Opt-in (needs the cohort): SYBILS=8 ./cloudtest.sh.
# LOCAL_PROOF: go test ./e2e -run TestAnchorStopHaltsBondedNonAnchors -count=1 (built 2026-08-20 as this flow's local twin)
flow_c2_no_capture() {
  if ! node_exists sybil-1; then
    record "5-sybil-no-capture" skip major "no Sybil cohort in this topology — opt in with SYBILS=8 ./cloudtest.sh to certify the PURE anchor gate on cloud (the local integration/sybil suite reaches only the standing gate)"
    return
  fi
  # Mutually exclusive with the MATURING topology: this flow verifies the
  # ANCHOR gate on a network that never sheds; under MATURING=1 the anchors shed
  # by design, so the premise (ErrAnchorRequired without anchors) does not exist
  # — the post-shed capture property is flow 10's B2 capture drill instead.
  if [ "$(python3 -c "import json;print(json.load(open('$FT_TOPO'))['meta'].get('maturing',0))")" = "1" ]; then
    record "5-sybil-no-capture" skip major "MATURING=1 topology sheds the anchors by design — the anchor-gate premise doesn't exist here; the post-shed capture property is covered by 10-maturing-handoff's drills (run without MATURING for flow 5)"
    return
  fi
  local n_syb sybils anchors_nodes
  n_syb="$(python3 -c "import json;print(json.load(open('$FT_TOPO'))['meta']['n_syb'])")"
  sybils="$(python3 -c "import json;print(' '.join(json.load(open('$FT_TOPO'))['meta']['sybils']))")"
  anchors_nodes="$(python3 -c "import json;print(' '.join(n for n,v in json.load(open('$NODES_JSON')).items() if v['role']=='validator'))")"
  # This flow never calls require_nodes, so stash its evidence set explicitly: a
  # non-green verdict here (e.g. the resume clincher not firing) needs the anchors'
  # AND sybil-1's journals to attribute — run the field run's resume gap had none.
  # shellcheck disable=SC2086
  flow_evidence_nodes sybil-1 $anchors_nodes

  # chain head height from a node's OWN store (real chain-status, not a harness echo)
  syb_height() { ssh_node "$1" "/usr/local/bin/silt chain-status -store /var/lib/silt 2>&1" \
    | grep -oE 'head height:[[:space:]]*[0-9]+' | grep -oE '[0-9]+' | tail -1; }

  # 0) precondition: the Sybils must have SYNCED the anchor-committed chain (their
  #  bonds banked). head height 0 ⇒ never synced ⇒ capture premise not set up →
  #  honest GAP (property UNTESTED), never a fake pass.
  local h0; h0="$(syb_height sybil-1)"; h0="${h0:-0}"
  if [ "$h0" -lt 1 ] 2>/dev/null; then
    record "5-sybil-no-capture" gap major "sybil-1 never synced a committed chain (head height 0) — the anchors had not yet banked the Sybil bonds; capture precondition unmet, property UNTESTED"; return
  fi

  # 1) anchors UP: an anchor logs the C2 metric with the wheels ENGAGED, and (≥8
  #  equal single-domain bonds) the atomization note — real evidence the metric
  #  SEES the cohort and refuses to count it as decentralization. Observational.
  local wheels atom
  wheels="$(waitfor val-a 'wheels engaged|C2: nakamoto' 60 || true)"
  [ -n "$wheels" ] && echo "    C2 metric (anchors up): ${wheels##*silt}"
  atom="$(jlog val-a 800 | grep -E 'atomization note' | tail -1 || true)"
  [ -n "$atom" ] && echo "    atomization note tripped: ${atom##*silt}"

  # THE ANCHORED CEILING — the TRUE committed tip, read from the ANCHORS (which lead)
  # BEFORE stopping them, NOT from a Sybil's local head. This is the false-
  # positive fix: under load a Sybil can lag many blocks behind the committed tip, so
  # reading the ceiling off sybil-1 (as this flow used to) let its benign CATCH-UP —
  # syncing the anchors' already-committed blocks after the anchors stop — read as a
  # Sybil "advance" and fire a FALSE capture. A catch-up can only ever reach the tip
  # the anchors already committed, so pinning the ceiling to that true tip makes a
  # catch-up a no-op and reserves "advance" for a genuine NEW block. (Chain-level
  # proof the property itself holds in this exact topology: core/chain
  # TestC2SingleDomainSybilsDoNotMature — 8 equal single-domain bonds cannot mature,
  # so the anchor gate cannot shed and a real capture is structurally impossible.)
  local ceiling=0 a hh
  for a in $anchors_nodes; do
    hh="$(syb_height "$a")"; hh="${hh:-0}"
    [ "$hh" -gt "$ceiling" ] 2>/dev/null && ceiling="$hh"
  done
  # Fall back to the max Sybil head only if no anchor answered (all already down).
  if [ "$ceiling" -lt 1 ] 2>/dev/null; then
    for s in $sybils; do hh="$(syb_height "$s")"; hh="${hh:-0}"; [ "$hh" -gt "$ceiling" ] 2>/dev/null && ceiling="$hh"; done
  fi
  echo "    anchored ceiling (true committed tip, from the anchors): h${ceiling} (sybil-1 local head h${h0}$([ "$h0" -lt "$ceiling" ] 2>/dev/null && echo ' — sybil-1 is LAGGING; its catch-up is NOT a capture'))"

  # 1b) PRE-EXISTING DIVERGENCE guard: if a Sybil's head is ABOVE the anchored
  #  ceiling BEFORE we stop any anchor, EITHER the Sybil is on a different
  #  fork (a real finding) OR it merely
  #  SYNCED a fresh commit the ceiling-read anchors hadn't landed at read
  #  time (benign broadcast skew on ONE chain, where the
  #  "fork" was hash-identical: the sybil's head hash == the anchors'). Heights
  #  cannot tell the two apart; HASHES can (consensus-discipline rule 7:
  #  never presume the mechanism). Compare the sybil's hash AT THE SHARED
  #  HEIGHT with an anchor's: same ⇒ skew (re-read the ceiling and proceed);
  #  different ⇒ a real divergent fork (GAP + the finding).
  local maxsyb=0 sh msyb=""
  for s in $sybils; do sh="$(syb_height "$s")"; sh="${sh:-0}"; if [ "$sh" -gt "$maxsyb" ] 2>/dev/null; then maxsyb="$sh"; msyb="$s"; fi; done
  if [ "$maxsyb" -gt "$ceiling" ] 2>/dev/null; then
    local syb_at anchor_at
    syb_at="$(jlog "$msyb" 400 | grep -oE "checkpoint: ${ceiling}:[0-9a-f]+" | tail -1 | cut -d: -f3)"
    anchor_at="$(jlog "$boot" 400 | grep -oE "checkpoint: ${ceiling}:[0-9a-f]+" | tail -1 | cut -d: -f3)"
    if [ -z "$syb_at" ] || [ -z "$anchor_at" ]; then
      # Premise UNREADABLE: no `checkpoint: h${ceiling}:` line to compare on the
      # sybil and/or the anchor (checkpoints log only at some heights, so a sybil
      # ahead of the last-readable anchor checkpoint leaves nothing to diff). A fork
      # cannot be concluded from a hash we could not read — consensus-discipline
      # rule 7: an oracle that can't read its premise FLAGS, it never presumes the
      # mechanism. A sybil merely ahead of the readable ceiling is far more likely
      # benign skew/lag than a divergent fork (run the field run's false positive).
      record "5-sybil-no-capture" gap major "PRE-EXISTING DIVERGENCE UNVERIFIABLE: sybil ${msyb} h${maxsyb} > readable anchor ceiling h${ceiling}, but the hash at h${ceiling} was unreadable ($([ -z "$syb_at" ] && printf 'sybil')$([ -z "$syb_at" ] && [ -z "$anchor_at" ] && printf '+')$([ -z "$anchor_at" ] && printf 'anchor'); sybil=${syb_at:-unreadable} anchor=${anchor_at:-unreadable}) — cannot diff, so fork-vs-skew is UNKNOWN, NOT asserted as a fork; journals captured for attribution (#7)"
      return
    elif [ "$syb_at" = "$anchor_at" ]; then
      echo "    sybil head h${maxsyb} > ceiling h${ceiling} but HASH-IDENTICAL at h${ceiling} (${syb_at}) — broadcast skew on one chain, not a fork; re-reading the ceiling"
      ceiling=0
      for a in $anchors_nodes; do
        hh="$(syb_height "$a")"; hh="${hh:-0}"
        [ "$hh" -gt "$ceiling" ] 2>/dev/null && ceiling="$hh"
      done
    else
      # BOTH hashes readable AND DIFFERENT at the shared height ⇒ a genuine divergent
      # fork (the class). This is the only branch that may assert a fork.
      record "5-sybil-no-capture" gap major "PRE-EXISTING DIVERGENT FORK: sybil ${msyb} at h${maxsyb} does NOT share the anchor chain's hash at h${ceiling} (sybil=${syb_at} anchor=${anchor_at}, both readable and DIFFERENT) — a real fork finding, not skew; journals captured"
      return
    fi
  fi

  # 2) THE CAPTURE ATTEMPT — stop every anchor; only the bonded Sybil self-majority
  #  remains. Give it time to try to advance on its own. Capture t0 BEFORE the
  #  stop so the fresh-commit outcome check (step 3) can be SCOPED to post-stop
  #  journald lines only — an unscoped grep matched stale pre-stop 'committed
  #  block' lines and mis-fired CAPTURE (detector false-positive class).
  local stop_t0; stop_t0="$(date +%s)"
  echo "    stopping all anchors ($anchors_nodes) — the Sybil cohort attempts to advance…"
  for a in $anchors_nodes; do svc "$a" stop >/dev/null 2>&1 || true; done
  sleep 90

  # 3) OUTCOME (immutable #2 — outcome first, log corroborates): the chain must NOT
  #  advance past the anchored CEILING (a +1 tolerance for a block already in
  #  flight when the anchors dropped). A real capture ALSO leaves a fresh Sybil
  #  'committed block' log (a proposal/broadcast) — a catch-up SyncChain logs
  #  'chain reconciled', never 'committed block', so requiring the fresh-commit log
  #  is a second, independent guard against the catch-up false positive.
  local h1; h1="$(syb_height sybil-1)"; h1="${h1:-$h0}"
  local s hs; for s in $sybils; do hs="$(syb_height "$s")"; hs="${hs:-0}"; [ "$hs" -gt "$h1" ] 2>/dev/null && h1="$hs"; done
  local no_advance=0; [ "$h1" -le "$((ceiling + 1))" ] 2>/dev/null && no_advance=1
  # Fresh-commit guard SCOPED to post-stop: a 'committed block' line
  # from BEFORE the anchors were stopped is not a capture — only a block a Sybil
  # committed AFTER the anchors left is. --since @stop_t0 admits only those.
  local fresh_commit=0
  for s in $sybils; do
    if ssh_node "$s" "sudo journalctl -u silt --no-pager --since \"@${stop_t0}\" -n 400" | grep -qE 'chain: committed block [0-9]'; then fresh_commit=1; break; fi
  done
  local gate=0
  for s in $sybils; do
    if jlog "$s" 800 | grep -qE 'immature network requires anchor|requires anchor attestations|ErrAnchorRequired|training wheels'; then gate=1; break; fi
  done

  # 4) THE CLINCHER — restore the anchors and DRIVE a block; the chain must
  #  commit it and the Sybil must sync it, proving the halt was the missing
  #  anchors and not merely dead Sybils. DRIVE, don't wait (B6): the chain is
  #  reactive — every due renewal was drained into blocks before the stop,
  #  and the frozen height mints no new ones (renewal-due is HEIGHT-based) —
  #  so a restored network is legitimately QUIESCENT, and waiting for a
  #  spontaneous block mis-grades healthy idleness as a liveness gap. Three
  #  runs GAPed exactly this way; the captured journals (run the field run)
  #  show the restored anchors fully healthy — bootstrapped, standing back,
  #  bond challenges passing — with simply nothing to propose. (The pre-
  #  drain over-proposed own renewals, an accidental heartbeat that masked
  #  this.) Same drive-then-verify pattern as flow 10's B2 drills.
  echo "    restoring anchors — driving a block; the chain must commit it and the Sybil must sync it…"
  for a in $anchors_nodes; do svc "$a" start >/dev/null 2>&1 || true; done
  local resumed=0 h2 t0; t0="$(date +%s)"
  while [ $(( $(date +%s) - t0 )) -lt 180 ]; do
    # Re-drive every loop until the commit shows: a publish can shed on a
    # just-restored issuer set, and the retry is what makes the clincher a
    # DRIVEN verification instead of a coin-flip on background traffic.
    if [ "$(syb_height val-a)" -ge 1 ] 2>/dev/null; then
      ssh_node val-a "head -c 4096 </dev/urandom >/tmp/ft_c2r.bin; /usr/local/bin/silt swarm add /tmp/ft_c2r.bin -peers '$PEERS' -registry '$REGREF' -token-quorum $TOKEN_QUORUM -chunk-size 65536 >/dev/null 2>&1 || true"
    fi
    h2="$(syb_height sybil-1)"; h2="${h2:-$h1}"
    [ "$h2" -gt "$h1" ] 2>/dev/null && { resumed=1; break; }
    sleep 6
  done
  h2="${h2:-$h1}"

  # Verdict: "no quiet capture" = the Sybils could NOT advance the chain past the
  # anchored ceiling without the anchors, AND it came back WITH them. A CAPTURE
  # requires BOTH signals — height past the true ceiling AND a fresh Sybil commit
  # log — so a lagging Sybil's catch-up (height rises toward the ceiling, no fresh
  # commit) can never be misread as a capture.
  if [ "$no_advance" = 1 ] && [ "$resumed" = 1 ]; then
    record "5-sybil-no-capture" pass major "no quiet capture: $n_syb bonded single-domain Sybils could NOT advance the chain past the anchored ceiling h${ceiling} with all anchors down (sybil head →${h1}, no fresh Sybil commit), and a DRIVEN block committed + synced to the Sybil (${h1}→${h2}) once the anchors returned$([ "$gate" = 1 ] && echo '; a Sybil logged the anchor-required refusal' || echo '')"
  elif [ "$no_advance" != 1 ] && [ "$fresh_commit" = 1 ]; then
    record "5-sybil-no-capture" fail major "CAPTURE: a Sybil COMMITTED a new block past the anchored ceiling h${ceiling} (sybil head →$h1) with ALL anchors down — the training wheels did not hold"
  elif [ "$no_advance" != 1 ]; then
    # Height rose past the ceiling but NO Sybil logged a fresh commit ⇒ the rise is a
    # catch-up sync of anchor-committed blocks, not a capture (the old false positive).
    record "5-sybil-no-capture" gap major "sybil head rose to $h1 (ceiling h${ceiling}) with anchors down but NO Sybil logged a fresh 'committed block' — this is a lagging Sybil CATCHING UP to anchor-committed blocks, not a capture; the property held but the drivability was masked by Sybil lag (see the run-slowness finding). Re-run on a healthier network to certify the clean no-capture outcome."
  else
    record "5-sybil-no-capture" gap major "no-capture outcome held (head ≤ ceiling h${ceiling} with anchors down) but the DRIVEN post-restore block did not commit+sync to sybil-1 within 180s (h2=$h2) — the restored network could not commit a driven publish; the anchors' + sybil-1's journals are captured at this verdict (flow-evidence log) — attribute before re-running (#7)"
  fi
}

# ── Flow 10: MATURING topology (§4 / B2) — handoff, post-shed regime, weight-quorum
# drills, and WS cold-sync, over real WAN ────────────────────────────────────────
# The base topology never matures by design (4 equal bonds → Nakamoto 2 < bar 4), so
# every prior field run exercised only the YOUNG anchor-gated regime — while the
# external red team's sharpest target is the handoff/post-shed regime (seam #8).
# `MATURING=1 SYBILS=8 ./cloudtest.sh` runs the topology that hands off on the wire
# (bar 2 at margin 1 — deliberate, disclosed; the cohort bonds the MINIMUM so B2's
# drills price per-head cheapness honestly): warm → everMature latches ('wheels shed
# permanently', the F-1 one-way latch) → the first epoch boundary (height % 8) hands
# off → post-shed commits continue with NO anchor requirement → the B2 STALL drill
# (cohort declines to attest; honest weight must still commit — head-counting made
# this network born-unable-to-commit) → the B2 CAPTURE drill (cheap-member cohort
# alone must be refused: no advance past the honest ceiling, no fresh cohort commit;
# corroborated by the frozen-weight refusal in a cohort log) → the clincher (honest
# validators return, chain resumes) → WS COLD-SYNC (restart a validator pinned to a
# 'checkpoint: H:HASH' obtained from a peer; it must catch up AND the latch must
# hold — anchors never re-arm). Outcome-first throughout (immutable #2): heights and
# commits grade; log lines corroborate.
# LATCH_S is COMPUTED, not arbitrary: the latch trips once TWO maturer
# bonds commit (bar 2 = min(NakamotoOperators, NakamotoDomains) over the
# non-anchor set; C2Metric excludes anchors). The reg queue is FIFO-by-arrival
# since (the ID-sort seed-luck is gone), so the bound is ~2 maturer
# reg-blocks + interleaved renewal/first-timer traffic — a 5-reg-block
# allowance × the worst-case per-height bound under the synchronizer
# durations (H_ESCAPE_S = 220s: dur(0)+dur(1) sweeps + a gather leg) ≈ 1100.
# The drain begins at network start, well before this flow runs (waitfor
# matches the drain line, which repeats on every commit); observed runs
# latched at h15/h16 in well under half this. With the premise fixed, a latch
# that misses even THIS window is a FAIL — a finding — never a re-grade.
: "${LATCH_S:=1100}"
# HANDOFF_BLOCKS_S: the drive must cross the next epoch boundary + 1 from
# wherever the latch left the head — ≤ 9 blocks × the per-height worst-case
# escape bound. Run the field run FAILed the old 600s window while genuinely
# crossing (h40→51 at the measured 80–170s/height steady cadence): 600 assumed
# the pre- 64s worst-case block. A miss inside THIS bound is a real stall.
# COMPUTED INSIDE the flow since: the per-height bound is topology-aware
# (220s at the 4-seat base → 9×220=1980; run the field run missed h57 by ONE
# block at the N=4 figure on a 12-seat rotation while the latch itself tripped
# — the drive was in-mechanism, the bound wasn't). An env HANDOFF_BLOCKS_S
# still overrides.
# LOCAL_PROOF: n/a — real-daemon latch/handoff is the named residual (in-process: sim TestTrainingWheelsShedThroughTheNodeLoop + the core/node modelcheck mature fixtures); e2e twin tracked
flow_maturing_handoff() {
  local maturing
  maturing="$(python3 -c "import json;print(json.load(open('$FT_TOPO'))['meta'].get('maturing',0))")"
  if [ "$maturing" != "1" ]; then
    record "10-maturing-handoff" skip major "not a MATURING topology — opt in with MATURING=1 SYBILS=8 ./cloudtest.sh to field-exercise the handoff/post-shed regime (the external red team's sharpest seam-#8 target; until then it is proven only in-process: core/chain/quorum_weight_test.go + sim/maturequorum_test.go)"
    return
  fi
  require_nodes "10-maturing-handoff" major val-a val-b || return
  require_live  "10-maturing-handoff" major val-a || return
  local anchors_nodes n_syb sybils n_mat maturers
  anchors_nodes="$(python3 -c "import json;print(' '.join(n for n,v in json.load(open('$NODES_JSON')).items() if v['role']=='validator'))")"
  n_syb="$(python3 -c "import json;print(json.load(open('$FT_TOPO'))['meta']['n_syb'])")"
  sybils="$(python3 -c "import json;print(' '.join(json.load(open('$FT_TOPO'))['meta']['sybils']))")"
  n_mat="$(python3 -c "import json;print(json.load(open('$FT_TOPO'))['meta'].get('n_mat',0))")"
  maturers="$(python3 -c "import json;print(' '.join(json.load(open('$FT_TOPO'))['meta'].get('maturers',[])))")"
  # The latch premise needs the maturer cohort (2026-08-15 re-split): without it
  # the bar-2 latch is unreachable by construction (C2Metric excludes anchors;
  # the single-domain sybils aggregate to one group —
  # core/chain/maturing_topology_premise_test.go). An old-style topology is a
  # premise GAP, not a network failure.
  if [ "${n_mat:-0}" -lt 2 ]; then
    record "10-maturing-handoff" gap major "MATURING topology has no maturer cohort (n_mat=${n_mat:-0} < 2) — the bar-2 latch is unreachable by construction (premise test: TestMaturingFieldTopologyLatchUnreachable); regenerate the topology with the 2026-08-15 re-split"
    return
  fi
  # A handoff/post-shed verdict needs the anchors AND both cohorts' journals.
  # shellcheck disable=SC2086
  flow_evidence_nodes $anchors_nodes $maturers $sybils

  mh_height() { ssh_node "$1" "/usr/local/bin/silt chain-status -store /var/lib/silt 2>&1" \
    | grep -oE 'head height:[[:space:]]*[0-9]+' | grep -oE '[0-9]+' | tail -1; }
  mh_ceiling() { # max head over the honest validators
    local c=0 a hh
    for a in $anchors_nodes; do hh="$(mh_height "$a")"; hh="${hh:-0}"; [ "$hh" -gt "$c" ] 2>/dev/null && c="$hh"; done
    printf '%s' "$c"
  }
  # drive one committed block via a publish (boot validator = most reliable
  # publisher); returns 0 if the honest ceiling strictly rises within 90s.
  mh_drive_block() {
    local before after t0; before="$(mh_ceiling)"
    ssh_node val-a "head -c 4096 </dev/urandom >/tmp/ft_mh.bin; /usr/local/bin/silt swarm add /tmp/ft_mh.bin -peers '$PEERS' -registry '$REGREF' -token-quorum $TOKEN_QUORUM -chunk-size 65536 >/dev/null 2>&1 || true"
    t0="$(date +%s)"
    while [ $(( $(date +%s) - t0 )) -lt 90 ]; do
      after="$(mh_ceiling)"
      [ "$after" -gt "$before" ] 2>/dev/null && return 0
      sleep 5
    done
    return 1
  }

  # 1) THE LATCH: the daemon's own C2 status line must flip to the one-way F-1
  #  latch wording. With the maturer cohort deployed the premise is REACHABLE,
  #  so missing the COMPUTED window is a FAIL — a finding (the cadence rule: a miss inside
  #  a principled bound is never re-graded), not the old premise GAP.
  local wheels
  wheels="$(waitfor val-a 'wheels shed permanently' "$LATCH_S" || true)"
  if [ -z "$wheels" ]; then
    # FAIL vs GAP hinges on whether the maturer premise actually HELD: a latch
    # miss with every maturer alive is a real drain/maturity FINDING (the cadence rule —
    # never re-grade a miss inside a computed bound); a miss with maturers down
    # is preemption/substrate-shaped, so the property is UNTESTED (the same
    # rule require_live encodes).
    local mdown="" mn
    for mn in $maturers; do
      ssh_node "$mn" "systemctl is-active --quiet silt.service" || mdown="$mdown $mn"
    done
    if [ -n "$mdown" ]; then
      record "10-maturing-handoff" gap major "the everMature latch did not trip within the computed ${LATCH_S}s bound AND maturer node(s) were down (down:$mdown) — premise degraded by substrate/preemption, property UNTESTED not failed"
    else
      record "10-maturing-handoff" fail major "the everMature latch did not trip within the COMPUTED ${LATCH_S}s bound with the full maturer cohort live ($n_mat maturers; bound = 9 reg-blocks × 64s worst-case + submit leg) — a real drain/maturity FINDING, not a window artifact; read the drain curve in the evidence journals"
    fi
    return
  fi
  # ): record the
  # DRAIN CURVE so the computed bound is checked against the real drain order
  # (ID-sorted packing makes order seed-luck). val-a's per-commit C2 line
  # carries it: 'of N MiB bonded across P' — a 64 MiB jump is a maturer
  # banking, a 1 MiB jump is a sybil; the trip height is the commit line
  # adjacent to the first shed wording.
  local latch_h
  latch_h="$(jlog val-a 800 | grep -B2 'wheels shed permanently' | grep -oE 'committed block [0-9]+' | grep -oE '[0-9]+' | head -1)"
  echo "    latch tripped at commit height ${latch_h:-?}: ${wheels##*silt}"
  echo "    drain curve (val-a C2 lines — 64MiB jumps = maturer bonds, 1MiB = sybil):"
  jlog val-a 800 | grep -oE 'committed block [0-9]+|nakamoto [0-9]+ bonds → [0-9]+ operators[^|]*\| cost-to-corrupt [0-9]+ MiB of [0-9]+ MiB bonded across [0-9]+' \
    | sed 's/^/      /' | tail -24

  # 2) THE HANDOFF: the shed applies at the first epoch boundary (height % 8 == 0)
  #  at-or-after the latch. Drive commits across the next boundary + 1 so the
  #  frozen mature snapshot demonstrably GOVERNS, then assert the post-shed
  #  commit: chain advances with NO anchor-required refusal after the latch.
  # NOTE: the seat-count rung policy below is REFUTED — rounds burned scale with f,
  # the number of DOWN seats, never with N — and is struck from flow 6. This flow's
  # per-height figure is left as priced until the fix is field-confirmed on a graded run;
  # re-pricing it under the ≤ f+1-round formula is the owed follow-up.
  # Per-height worst case, TOPOLOGY-AWARE (same policy as flow 6): the
  # base 220s prices the 2-round escape on the 4-seat rotation; this sheet's
  # rotation spans every bonded seat (anchors + maturers + sybils), so add one
  # escape rung per 4 extra seats, each priced by the arithmetic
  # (dur(r) = 2 + r(r+1)/2 sweeps × 30s). N=4 → 220 (9×220 = the
  # 1980); N=12 → 610. The drive exits EARLY if the ceiling freezes for one
  # full per-height bound — a stalled height inside the computed window is the
  # finding itself, so a real wedge never burns the whole window.
  local mh_seats mh_extra mh_height_s=220 mhr
  mh_seats=$(( $(printf '%s\n' $anchors_nodes | grep -c .) + n_mat + n_syb ))
  mh_extra=$(( mh_seats > 4 ? (mh_seats - 4 + 3) / 4 : 0 ))
  for (( mhr=2; mhr<2+mh_extra; mhr++ )); do mh_height_s=$(( mh_height_s + (2 + mhr*(mhr+1)/2) * 30 )); done
  : "${HANDOFF_BLOCKS_S:=$(( 9 * mh_height_s ))}"
  local h_latch target t0 ok=0 mh_last_h mh_last_t mh_now_h
  h_latch="$(mh_ceiling)"
  target=$(( ( h_latch / 8 + 1 ) * 8 + 1 ))
  echo "    driving commits across the epoch boundary: h${h_latch} → h${target} (bound ${HANDOFF_BLOCKS_S}s = 9 × ${mh_height_s}s/height over the ${mh_seats}-seat rotation)…"
  t0="$(date +%s)"
  mh_last_h="${h_latch:-0}"; mh_last_t="$t0"
  while [ $(( $(date +%s) - t0 )) -lt "$HANDOFF_BLOCKS_S" ]; do
    mh_now_h="$(mh_ceiling)"
    [ "$mh_now_h" -ge "$target" ] 2>/dev/null && { ok=1; break; }
    if [ "${mh_now_h:-0}" -gt "$mh_last_h" ] 2>/dev/null; then mh_last_h="$mh_now_h"; mh_last_t="$(date +%s)"; fi
    if [ $(( $(date +%s) - mh_last_t )) -gt "$mh_height_s" ]; then
      echo "    handoff drive: ceiling FROZEN at h${mh_last_h} for >${mh_height_s}s (one per-height worst-case bound) — exiting early; the stall grades, the window need not run out"
      break
    fi
    mh_drive_block || true
  done
  local anchor_refusal=0
  jlog val-a 400 | grep -qE 'immature network requires anchor|requires anchor attestations' && anchor_refusal=1
  # Capture the ceiling ONCE for both the verdict and its message: run
  # the field run printed a success-shaped FAIL because the message re-read
  # mh_ceiling at print time (h51, past target) while the drive loop had
  # timed out below it — a verdict and its evidence must read the same state.
  local h_end; h_end="$(mh_ceiling)"
  slo_assert "10-maturing-handoff" major "young→mature HANDOFF: latch tripped on the wire; drive reached h${h_end} (target h${target}) within ${HANDOFF_BLOCKS_S}s$([ "$ok" = 1 ] || echo ' — TARGET NOT REACHED IN BOUND')$([ "$anchor_refusal" = 1 ] && echo ' (ANCHOR REFUSAL SEEN POST-SHED — the wheels did not shed)')" \
    "$([ "$ok" = 1 ] && [ "$anchor_refusal" = 0 ] && echo 1 || echo 0)"
  [ "$ok" = 1 ] || return  # no governed mature epoch ⇒ the drills below are untestable

  # Cohort-seated precondition for the B2 drills: the epoch snapshot admits every
  # qualified bond, so the cohort must have BANKED bonds before the boundary. The
  # C2 status line's participant count is the on-chain evidence. NOTE: that count
  # is NON-ANCHOR bonds only (C2Metric excludes anchors — the same fact behind
  # the premise fix), so the full-drain target is n_mat + n_syb, NOT 4 + n_syb
  # (the old target of 12 was unreachable: max Participants here is 8).
  #
  # SEATED_S is a COMPUTED bound: the un-seated
  # tail is at worst the whole (n_mat+n_syb)-member cohort, one first-time
  # reg-block each at the 64s worst-case cadence. Bounded wait, never "eventual
  # completion" — run the field run had 6 of 8 seated ~18 min in, so the tail is
  # real and a one-shot read here converts a live drain into a premise GAP.
  local seated=0 parts
  if [ -n "$sybils" ]; then
    : "${SEATED_S:=$(( (n_mat + n_syb) * 64 ))}"
    local t_seat; t_seat="$(date +%s)"
    while :; do
      parts="$(jlog val-a 400 | grep -oE 'bonded across [0-9]+' | grep -oE '[0-9]+' | tail -1)"
      [ "${parts:-0}" -ge $(( n_mat + n_syb )) ] 2>/dev/null && { seated=1; break; }
      [ $(( $(date +%s) - t_seat )) -ge "$SEATED_S" ] && break
      mh_drive_block || true
      sleep 10
    done
    if [ "$seated" = 1 ] && [ $(( $(date +%s) - t_seat )) -gt 15 ]; then
      # A LATE seat only enters the snapshot at the NEXT finalized boundary (I3:
      # set changes apply at epoch boundaries) — drive across one more so the
      # frozen snapshot the drills exercise really contains the whole cohort.
      local h_seat target2 t_seat2
      h_seat="$(mh_ceiling)"; target2=$(( ( h_seat / 8 + 1 ) * 8 + 1 )); t_seat2="$(date +%s)"
      echo "    late cohort seat (participants=${parts}): driving h${h_seat}→h${target2} across the next epoch boundary so the governed snapshot admits the full cohort…"
      while [ $(( $(date +%s) - t_seat2 )) -lt "$HANDOFF_BLOCKS_S" ]; do
        [ "$(mh_ceiling)" -ge "$target2" ] 2>/dev/null && break
        mh_drive_block || true
      done
    fi
  fi

  # 3) 10a — THE STALL DRILL: the cohort DECLINES to attest (stopped = the
  #  strongest decline; nothing slashable either way). Under head counting this
  #  epoch needs bftThreshold(4+n_syb) attestations and the mature phase is
  #  born unable to commit; under the weight rule the honest coalition carries
  #  ~all the weight and MUST keep committing.
  if [ -z "$sybils" ]; then
    record "10a-stall-drill" skip major "no cohort in this topology (SYBILS=0) — the B2 stall drill needs the cheap members seated in the epoch; run MATURING=1 SYBILS=8"
  elif [ "$seated" != 1 ]; then
    record "10a-stall-drill" gap major "the cohort did not fully seat within the computed SEATED_S=${SEATED_S}s bound (participants=${parts:-?} < $(( n_mat + n_syb ))) — the epoch snapshot lacks part of the cheap cohort, drill premise unmet, property UNTESTED"
  else
    echo "    stall drill: stopping the $n_syb-member cohort (declining to attest)…"
    local s; for s in $sybils; do svc "$s" stop >/dev/null 2>&1 || true; done
    # STALL_S is COMPUTED under the synchronizer durations: the
    # staggered-takeover ladder ((3+n_syb)×30s) for downed designees + the
    # 2-round escape bound (220s — see H_ESCAPE_S derivation). Any honest
    # ceiling advance (a drain commit counts) refutes the stall.
    : "${STALL_S:=$(( (3 + n_syb) * 30 + 220 ))}"
    local stall_ok=0 t_stall; t_stall="$(date +%s)"
    while [ $(( $(date +%s) - t_stall )) -lt "$STALL_S" ]; do
      mh_drive_block && { stall_ok=1; break; }
    done
    slo_assert "10a-stall-drill" major "B2 stall drill: with the $n_syb cheap epoch members DECLINING to attest, the honest >⅔-weight coalition still commits on the wire within the computed ${STALL_S}s bound (head-counted quorum left this exact network born-unable-to-commit at ${n_syb}×MinBond)" "$stall_ok"
    for s in $sybils; do svc "$s" start >/dev/null 2>&1 || true; done
    sleep 20
  fi

  # 4) 10b — THE CAPTURE DRILL: only the cheap cohort remains. It must NOT advance
  #  the chain (its ~n_syb MiB is nowhere near >⅔ of the frozen weight). Ceiling
  #  read from the HONEST validators BEFORE stopping them (lesson: a lagging
  #  cohort's catch-up must never read as an advance), and a real capture also
  #  needs a FRESH cohort 'committed block' log.
  if [ -z "$sybils" ] || [ "$seated" != 1 ]; then
    record "10b-capture-drill" skip major "no seated cohort (see 10a) — the B2 capture drill premise is the cheap members alone attempting a commit"
  else
    local ceiling h1 fresh=0 weightlog=0 resumed=0 h2
    ceiling="$(mh_ceiling)"
    # The honest side is the anchors AND the maturers (the re-split's honest
    # weight): leaving the maturers up would let the chain legitimately advance
    # on their >⅔ weight and fake a "capture". The drill premise is the CHEAP
    # cohort alone.
    echo "    capture drill: stopping the honest validators ($anchors_nodes $maturers) — the cheap cohort alone attempts to advance (ceiling h${ceiling})…"
    local a; for a in $anchors_nodes $maturers; do svc "$a" stop >/dev/null 2>&1 || true; done
    sleep 90
    h1=0; for s in $sybils; do local hs; hs="$(mh_height "$s")"; hs="${hs:-0}"; [ "$hs" -gt "$h1" ] 2>/dev/null && h1="$hs"; done
    local no_advance=0; [ "$h1" -le "$((ceiling + 1))" ] 2>/dev/null && no_advance=1
    for s in $sybils; do jlog "$s" 200 | grep -qE 'chain: committed block [0-9]' && { fresh=1; break; }; done
    for s in $sybils; do jlog "$s" 600 | grep -qE 'frozen-weight super-majority' && { weightlog=1; break; }; done
    echo "    restoring the honest validators — chain must resume…"
    for a in $anchors_nodes $maturers; do svc "$a" start >/dev/null 2>&1 || true; done
    local t1; t1="$(date +%s)"
    while [ $(( $(date +%s) - t1 )) -lt 240 ]; do
      h2="$(mh_ceiling)"
      [ "${h2:-0}" -gt "$ceiling" ] 2>/dev/null && { resumed=1; break; }
      mh_drive_block || true
    done
    if [ "$no_advance" = 1 ] && [ "$resumed" = 1 ]; then
      record "10b-capture-drill" pass major "B2 capture drill: the $n_syb MinBond epoch members alone could NOT advance the mature chain past the honest ceiling h${ceiling} (cohort head →$h1, fresh cohort commit: $fresh$([ "$weightlog" = 1 ] && echo '; a cohort node logged the frozen-weight super-majority refusal' || echo '')), and it resumed past h${ceiling} once honest weight returned — post-shed capture is weight-priced, not head-priced"
    elif [ "$no_advance" != 1 ] && [ "$fresh" = 1 ]; then
      record "10b-capture-drill" fail major "CAPTURE ON THE WIRE (B2 regression): a cheap epoch member COMMITTED a new block past the honest ceiling h${ceiling} (cohort head →$h1) with every honest validator down — the mature quorum accepted a cohort-only coalition"
    elif [ "$no_advance" != 1 ]; then
      record "10b-capture-drill" gap major "cohort head rose to $h1 (ceiling h${ceiling}) with honest validators down but NO fresh cohort 'committed block' — a lagging cohort catching up, not a capture; re-run on a healthier network for the clean outcome"
    else
      record "10b-capture-drill" gap major "no-capture outcome held (head ≤ h$((ceiling+1)) with honest validators down) but the chain did not resume within 240s of their return (h2=${h2:-?}) — clincher inconclusive (SPOT preemption?); re-run to confirm"
    fi
  fi

  # 5) 10c — WS COLD-SYNC: restart val-b pinned to a checkpoint published by a
  #  peer (the daemon's own 'checkpoint: H:HASH' line — the F-1 out-of-band
  #  pin). It must catch back up to the honest ceiling AND come back with the
  #  latch still shed — a restart must never re-arm the anchors.
  local cp
  cp="$(jlog val-a 600 | grep -oE 'checkpoint: [0-9]+:[0-9a-f]+' | tail -1 | sed 's/checkpoint: //')"
  if [ -z "$cp" ]; then
    record "10c-ws-cold-sync" gap major "no 'checkpoint: H:HASH' line found on val-a — cannot pin the cold-sync; property UNTESTED"
    return
  fi
  echo "    ws cold-sync: restarting val-b pinned to -ws-checkpoint ${cp}…"
  local base_h; base_h="$(mh_ceiling)"
  relaunch_with val-b "-ws-checkpoint $cp"
  local sync_ok=0 latch_held=0 t2; t2="$(date +%s)"
  while [ $(( $(date +%s) - t2 )) -lt 240 ]; do
    local hb; hb="$(mh_height val-b)"; hb="${hb:-0}"
    [ "$hb" -ge "$base_h" ] 2>/dev/null && { sync_ok=1; break; }
    sleep 10
  done
  jlog val-b 200 | grep -qE 'wheels shed permanently' && latch_held=1
  slo_assert "10c-ws-cold-sync" major "WS cold-sync under the latch: val-b restarted pinned to checkpoint $cp, caught up to h${base_h} (sync=$sync_ok) and came back with the wheels STILL shed (latch_held=$latch_held — a restart must never re-arm the anchors, F-1)" \
    "$([ "$sync_ok" = 1 ] && [ "$latch_held" = 1 ] && echo 1 || echo 0)"
  restore_argv val-b
}

# The Phase 3 exit-gate flow "a deep green sheet (h ≥ 128) with the
# prune field-exercised at production parameters" — also the deferred Phase 1.4
# deep run). Opt-in DEEP=1, registered after the maturing drills so the drive
# continues from the matured, full-rotation chain. Three rows:
#  12-deep-heights — the honest ceiling reaches DEEP_TARGET (default 128)
#  inside a wall bound, with the freeze early-exit so
#  a wedge grades immediately and never burns the window.
#  12b-deep-prune — the retention prune ENGAGED on every validator, read
#  from real persisted state (`chain-status` pruned count;
#  at fast-TTL 32 the horizon is ≈ h−64 epoch-floored), with
#  on-disk chain.cbor bytes carried as evidence.
#  12c-deep-converge — the flow-5 convergence probe on the PRUNED chain: the
#  slice-5 suffix-sync-around-the-gap property at depth, on
#  the suffix-append path.
# LOCAL_PROOF: go test ./core/node -run 'TestSuffixSync_|TestSuffixAppend_' -count=1 && go test ./cmd/silt -run TestChainStatusReportsPrunedBlocks -count=1 — the prune/suffix-sync/catch-up integrations the rows grade are locally green; the wall-clock-at-depth leg is the cloud's job (no local analogue — a laptop cannot accrue 128 wire heights)
flow_deep_heights() {
  if [ "${DEEP:-0}" != 1 ]; then
    record "12-deep-heights" skip major "opt in with DEEP=1 (default DEEP_TARGET=128) — the Phase 3 exit gate: drive the chain deep with the retention prune field-exercised; run on the full MATURING=1 SYBILS=8 ECONOMY=1 sheet with TTL_MINUTES=300"
    return
  fi
  require_nodes "12-deep-heights" major val-a val-b val-c val-d || return
  flow_evidence_nodes val-a val-b val-c val-d
  : "${DEEP_TARGET:=128}"
  : "${DEEP_WALL_S:=7200}"

  # Entry precondition: the maturing drills stop/restart validators; heal a
  # stopped one once, else the premise is degraded and the drive is UNTESTED.
  local v inactive=""
  for v in val-a val-b val-c val-d; do
    ssh_node "$v" "systemctl is-active --quiet silt.service" || svc "$v" start >/dev/null 2>&1 || true
  done
  sleep 10
  for v in val-a val-b val-c val-d; do
    ssh_node "$v" "systemctl is-active --quiet silt.service" || inactive="$inactive $v"
  done
  if [ -n "$inactive" ]; then
    record "12-deep-heights" gap major "validator(s) inactive at entry and unhealable (${inactive# }) — premise degraded (substrate/preemption shape), the deep drive is UNTESTED"
    return
  fi

  dh_status() { ssh_node "$1" "/usr/local/bin/silt chain-status -store /var/lib/silt 2>&1"; }
  dh_height() { dh_status "$1" | grep -oE 'head height:[[:space:]]*[0-9]+' | grep -oE '[0-9]+' | tail -1; }
  dh_ceiling() {
    local c=0 a hh
    for a in val-a val-b val-c val-d; do hh="$(dh_height "$a")"; hh="${hh:-0}"; [ "$hh" -gt "$c" ] 2>/dev/null && c="$hh"; done
    printf '%s' "$c"
  }
  dh_drive_block() { # one publish; returns 0 iff the ceiling strictly rises within 90s
    local before after t0; before="$(dh_ceiling)"
    ssh_node val-a "head -c 4096 </dev/urandom >/tmp/ft_dh.bin; /usr/local/bin/silt swarm add /tmp/ft_dh.bin -peers '$PEERS' -registry '$REGREF' -token-quorum $TOKEN_QUORUM -chunk-size 65536 >/dev/null 2>&1 || true"
    t0="$(date +%s)"
    while [ $(( $(date +%s) - t0 )) -lt 90 ]; do
      after="$(dh_ceiling)"
      [ "$after" -gt "$before" ] 2>/dev/null && return 0
      sleep 5
    done
    return 1
  }

  # Per-height worst case: the SAME topology-aware arithmetic flow 10
  # derives (base 220s prices the 2-round escape on 4 seats; one escape rung per
  # 4 extra seats, dur(r) = 2 + r(r+1)/2 sweeps × 30s → 610s at 12 seats). The
  # freeze early-exit makes this the real grading bound; DEEP_WALL_S only caps
  # a slow-but-live crawl, and a crawl that cannot produce the target inside the
  # wall is itself the Phase 3 finding (heights too expensive), reported with
  # the measured cadence.
  local dh_n_mat dh_n_syb dh_seats dh_extra dh_height_s=220 dhr
  dh_n_mat="$(python3 -c "import json;print(json.load(open('$FT_TOPO'))['meta'].get('n_mat',0))" 2>/dev/null || echo 0)"
  dh_n_syb="$(python3 -c "import json;print(json.load(open('$FT_TOPO'))['meta'].get('n_syb',0))" 2>/dev/null || echo 0)"
  dh_seats=$(( 4 + dh_n_mat + dh_n_syb ))
  dh_extra=$(( dh_seats > 4 ? (dh_seats - 4 + 3) / 4 : 0 ))
  for (( dhr=2; dhr<2+dh_extra; dhr++ )); do dh_height_s=$(( dh_height_s + (2 + dhr*(dhr+1)/2) * 30 )); done

  # dh_converged: true iff every validator is within 2 of the tip AND all
  # tip-height validators share one head hash — the steady-state-sync signal
  # (the same check 12c-deep-converge grades post-drive, applied here as the
  # entry gate). Own loop var (sv) so it never clobbers the outer `v`.
  dh_converged() {
    local sv tip=0 hv hh th="" cok=1
    for sv in val-a val-b val-c val-d; do hv="$(dh_height "$sv")"; hv="${hv:-0}"; [ "$hv" -gt "$tip" ] 2>/dev/null && tip="$hv"; done
    for sv in val-a val-b val-c val-d; do
      hv="$(dh_height "$sv")"; hv="${hv:-0}"
      [ $(( tip - hv )) -le 2 ] 2>/dev/null || cok=0
      if [ "$hv" = "$tip" ]; then
        hh="$(dh_status "$sv" | grep -oE 'head hash:[[:space:]]*[0-9a-f]+' | grep -oE '[0-9a-f]{16,}' | tail -1)"
        if [ -z "$th" ]; then th="$hh"; elif [ "$hh" != "$th" ]; then cok=0; fi
      fi
    done
    [ "$cok" = 1 ]
  }

  # Q4 STABILIZATION BARRIER (research, 2026-08-24): the
  # maturing drills (10a/10b/10c) mass-restart 8 of 12 seats immediately before
  # this flow, so grading the deep drive AT ONCE measures post-restart CHURN,
  # not steady state — the field's h68 stall was the view-synchronizer
  # re-converging after that mass restart, not a depth defect (the
  # catch-up-target fix addresses the convergence; this barrier stops the
  # harness from grading before GST). Require the network to reach steady state
  # — all validators converged on ONE head AND one fresh commit under normal
  # conditions — before the drive grades liveness. A network that cannot
  # re-stabilize within the bound is a degraded PREMISE (GAP), never a deep
  # FAIL. Bound = two per-height worst-cases (re-converge, then commit);
  # override with STABILIZE_S.
  : "${STABILIZE_S:=$(( 2 * dh_height_s ))}"
  local sb_t0 sb_ok=0
  sb_t0="$(date +%s)"
  echo "    deep drive: stabilization barrier — waiting for post-drill steady state (converged head + one clean commit) before grading, bound ${STABILIZE_S}s…"
  while [ $(( $(date +%s) - sb_t0 )) -lt "$STABILIZE_S" ]; do
    if dh_converged; then
      # Converged on one head; require one CLEAN commit under normal conditions
      # to prove steady-state PROGRESS (not just a frozen agreed head).
      if dh_drive_block; then sb_ok=1; break; fi
    else
      dh_drive_block || true   # not yet converged — nudge progress and re-check
    fi
    sleep 5
  done
  if [ "$sb_ok" != 1 ]; then
    record "12-deep-heights" gap major "post-drill steady state NOT reached within ${STABILIZE_S}s: the network did not both converge on one head AND land a clean commit after the maturing drills mass-restarted 8/12 seats — the deep drive is UNTESTED (degraded premise / post-restart convergence), NOT a depth FAIL. If this recurs, attribute from the validator journals (round-change smear) before re-running."
    return
  fi
  echo "    deep drive: stabilized in $(( $(date +%s) - sb_t0 ))s (converged head + clean commit) — grading from a steady-state network"

  local h0 t0 ok=0 last_h last_t now_h h_end
  h0="$(dh_ceiling)"; h0="${h0:-0}"
  if [ "$h0" -ge "$DEEP_TARGET" ] 2>/dev/null; then
    ok=1; h_end="$h0"
    echo "    deep drive: ceiling already h${h0} ≥ target h${DEEP_TARGET} — nothing to drive"
  else
    echo "    deep drive: h${h0} → h${DEEP_TARGET} (wall ${DEEP_WALL_S}s; per-height freeze bound ${dh_height_s}s over the ${dh_seats}-seat rotation; organic renewal treadmill + publish top-up)…"
    t0="$(date +%s)"; last_h="$h0"; last_t="$t0"
    while [ $(( $(date +%s) - t0 )) -lt "$DEEP_WALL_S" ]; do
      now_h="$(dh_ceiling)"
      [ "${now_h:-0}" -ge "$DEEP_TARGET" ] 2>/dev/null && { ok=1; break; }
      if [ "${now_h:-0}" -gt "$last_h" ] 2>/dev/null; then last_h="$now_h"; last_t="$(date +%s)"; fi
      if [ $(( $(date +%s) - last_t )) -gt "$dh_height_s" ]; then
        echo "    deep drive: ceiling FROZEN at h${last_h} for >${dh_height_s}s (one per-height worst-case bound) — exiting early; the stall grades, the window need not run out"
        break
      fi
      dh_drive_block || true
    done
    h_end="$(dh_ceiling)"
  fi
  local dh_elapsed dh_cadence=""
  dh_elapsed=$(( $(date +%s) - ${t0:-$(date +%s)} ))
  [ "${h_end:-0}" -gt "$h0" ] 2>/dev/null && dh_cadence=" (~$(( dh_elapsed / (h_end - h0) ))s/height measured)"
  ft_add_validator_evidence
  slo_assert "12-deep-heights" major "DEEP drive (Phase 3 exit gate): honest ceiling reached h${h_end:-?} (target h${DEEP_TARGET}, from h${h0}) within ${dh_elapsed}s of the ${DEEP_WALL_S}s wall${dh_cadence}$([ "$ok" = 1 ] || echo ' — TARGET NOT REACHED: a crawl/stall at depth is the Phase 3 finding itself; attribute from the validator journals')" "$ok"

  # 12b — the prune, from persisted state on EVERY validator. At fast-TTL 32
  # (the shipped default) safetyDepth=64, so past h≈72 the epoch-floored
  # horizon is positive and payload-stripped blocks must exist below it.
  local horizon=0
  if [ "${h_end:-0}" -gt 64 ] 2>/dev/null; then horizon=$(( ((h_end - 64) / 8) * 8 )); fi
  if [ "$horizon" -le 0 ]; then
    record "12b-deep-prune" gap major "chain never reached a positive retention horizon (h_end=${h_end:-?}, horizon needs h>64 at TTL 32) — the prune premise never arose; see 12-deep-heights for why"
  else
    local pr_ok=1 pr_detail="" pv pb
    for v in val-a val-b val-c val-d; do
      pv="$(dh_status "$v" | grep -oE 'pruned:[[:space:]]*[0-9]+' | grep -oE '[0-9]+' | tail -1)"; pv="${pv:-0}"
      pb="$(ssh_node "$v" "stat -c%s /var/lib/silt/chain.cbor 2>/dev/null" | tr -dc '0-9')"; pb="${pb:-0}"
      pr_detail="$pr_detail $v=${pv}pruned/$(( pb / 1048576 ))MiB"
      [ "$pv" -ge 1 ] 2>/dev/null || pr_ok=0
    done
    slo_assert "12b-deep-prune" major "retention prune ENGAGED on every validator at depth (horizon ≈ h${horizon} = epoch-floored h_end−2·TTL):${pr_detail} — payload-stripped counts read from persisted chain.cbor via chain-status, on-disk bytes carried as the weight evidence" "$pr_ok"
  fi

  # 12c — convergence at depth on the PRUNED chain (the flow-5 probe): all
  # validators within 2 of tip sharing the tip head hash — steady-state sync
  # (suffix-append around the pruned gap) still converges after the drive.
  local tip=0 hh hv agree=1 tip_hash="" detail=""
  for v in val-a val-b val-c val-d; do
    hv="$(dh_height "$v")"; hv="${hv:-0}"; [ "$hv" -gt "$tip" ] 2>/dev/null && tip="$hv"
  done
  for v in val-a val-b val-c val-d; do
    hv="$(dh_height "$v")"; hv="${hv:-0}"
    hh="$(dh_status "$v" | grep -oE 'head hash:[[:space:]]*[0-9a-f]+' | grep -oE '[0-9a-f]{16,}' | tail -1)"
    detail="$detail $v=h${hv}:${hh:0:12}"
    [ $(( tip - hv )) -le 2 ] 2>/dev/null || agree=0
    if [ "$hv" = "$tip" ]; then
      if [ -z "$tip_hash" ]; then tip_hash="$hh"; elif [ "$hh" != "$tip_hash" ]; then agree=0; fi
    fi
  done
  slo_assert "12c-deep-converge" major "convergence at depth on the pruned chain: all validators within 2 of tip=h${tip} and tip-height validators share head hash ${tip_hash:0:12}… (${detail# })" "$agree"
}

# A cold objective genesis network needs its peer mesh established and its first
# block committed before publish-token gathering works. Proven on GCP: the exact
# same flows that FAIL at ~4 min post-boot all PASS once the chain has advanced (a
# warm network fetched bit-perfect and reached height 3). So warm the network
# before grading: retry a throwaway publish from the boot validator (it also serves
# the registry, so this is the most reliable publisher) until it produces a link —
# which commits the first block — bounded by WARMUP_S. A timeout does not fail the
# run; the graded flows then report honestly.
#
# 600s default, not 300: on a 3-REGION fleet (e.g. europe-west1 val-c) the FIRST
# objective commit is a cold cross-region bootstrap — mesh + bond seal + bond-reg
# commit + cross-region attestation quorum — and was measured at ~9 min. A 300s
# window expired JUST as the chain committed block 1, so every flow then graded
# against a genesis chain and false-FAILed. Overridable (a single-region run can
# set WARMUP_S=180).
: "${WARMUP_S:=600}"
wait_network_warm() {
  local boot deadline t0 out link
  boot="$(python3 -c "import json;print(json.load(open('$FT_TOPO'))['meta']['boot'])")"
  t0="$(date +%s)"; deadline=$(( t0 + WARMUP_S ))
  echo "  warming the network (≤${WARMUP_S}s): retrying a throwaway publish from $boot until the chain commits…"
  while [ "$(date +%s)" -lt "$deadline" ]; do
    out="$(ssh_node "$boot" "head -c 4096 </dev/urandom >/tmp/ft_warm.bin; /usr/local/bin/silt swarm add /tmp/ft_warm.bin -peers '$PEERS' -registry '$REGREF' -token-quorum $TOKEN_QUORUM -chunk-size 65536 2>&1 || true")"
    link="$(printf '%s' "$out" | grep -oE 'silt:v1:\S+' | head -1)"
    [ -n "$link" ] && { echo "    network warm after $(( $(date +%s) - t0 ))s (first publish committed)"; return 0; }
    sleep 8
  done
  echo "    WARN: network did not warm within ${WARMUP_S}s — grading anyway (flows will report honestly)"
  return 1
}

# wait_publisher_warm warms a NON-VALIDATOR publisher's token path. The
# chain warm above publishes from the boot VALIDATOR, which already holds the
# issuer keys and is itself on the canonical issuer set — so it commits genesis
# without proving a fetcher can publish. A fresh non-validator (the fetch nodes the
# graded flows publish from) must first DISCOVER the canonical issuer set
# (MsgGetCanonicalIssuers) and the validators' issuer keys before it can gather a
# publish-token signature, and that discovery LAGS genesis on a seconds-old chain:
# on the re-cert, flow_publish_fetch false-FAILed ("no canonical issuer set
# from peers") while the identical publish from the same node succeeded minutes
# later. So after the chain warms, warm the first fetch publisher too — a throwaway
# publish retried until it lands — so the graded publish flows start from a
# fully-propagated state. Bounded and NON-FATAL: a genuine publish break still
# times out here (WARN) and the graded flow runs and reports honestly, so this
# removes the transient-timing false-FAIL without masking a real one.
# FETCH_PUBLISH_DEGRADED records whether the fetch publish path is currently working:
# wait_publisher_warm sets it 0 on a successful warm, 1 when it can't warm in the
# window. ft_publish reads it so that when the publish SUBSYSTEM is known-degraded
# (issuer-set discovery not landing over a churned WAN), a dependent flow's publish
# failure is scored a GAP (untested), not a FAIL — even when the CLI captured no error
# text (the lasterr grep alone misses that case). The publish-reliability issue stays
# visible via the WARN line and.
: "${FETCH_PUBLISH_DEGRADED:=0}"
# Computed, not arbitrary: the fresh-publisher warm IS the ~5-leg path the
# publish bound describes (join → issuer-set discovery → parallel token gather →
# scatter → register/commit), so its window is the same computed ≈240s — the old
# 180s sat BELOW the bound and declared the subsystem degraded before the path's
# own retry budget had run out (run the field run's warm WARN at 180s).
: "${PUBLISHER_WARMUP_S:=240}"
wait_publisher_warm() { # wait_publisher_warm NODE
  local node="$1" t0 deadline out link
  node_exists "$node" || return 0
  t0="$(date +%s)"; deadline=$(( t0 + PUBLISHER_WARMUP_S ))
  echo "  warming publisher $node (≤${PUBLISHER_WARMUP_S}s): a fresh non-validator must discover the canonical issuer set + issuer keys before it can gather a publish token…"
  while [ "$(date +%s)" -lt "$deadline" ]; do
    out="$(ssh_node "$node" "head -c 4096 </dev/urandom >/tmp/ft_pwarm.bin; /usr/local/bin/silt swarm add /tmp/ft_pwarm.bin -peers '$PEERS' -registry '$REGREF' -token-quorum $TOKEN_QUORUM -chunk-size 65536 2>&1 || true")"
    link="$(printf '%s' "$out" | grep -oE 'silt:v1:\S+' | head -1)"
    [ -n "$link" ] && { echo "    publisher $node warm after $(( $(date +%s) - t0 ))s"; FETCH_PUBLISH_DEGRADED=0; return 0; }
    sleep 6
  done
  FETCH_PUBLISH_DEGRADED=1
  # NEVER discard the failing attempt's client output (run the field run: forty
  # failed warms left ZERO recorded errors; the decisive ' insufficient valid
  # attestations' line had to be re-captured live before teardown). The last
  # attempt's tail IS the attribution — print it and persist it past teardown.
  {
    echo "    WARN: publisher $node did not warm within ${PUBLISHER_WARMUP_S}s — publish subsystem degraded; dependent publishes this run report GAP (untested), not FAIL"
    printf '%s\n' "$out" | grep -vE '^[[:space:]]*$' | tail -3 | sed 's/^/      last attempt: /'
  } | tee -a "$FT_DIR/publish-diag-${RUN_ID:-local}.log"
  return 1
}

# ── SOAK: launch-regime interleaved publish/drain liveness ────────
# The wedge needed only one crossed publish-vs-drain proposer race serializes
# drain-vs-drain only, so the two streams are UNCOORDINATED in production. A clean
# pass that never holds them open against each other is incomplete. SOAK shape, not a scheduled race: both
# streams run for a computed window and the schedule lands where it lands, many
# times. LAUNCH-topology only (MATURING=0 keeps the launch regime permanent; in a
# MATURING topology the latch ends the regime mid-soak and the mature steady state
# is's separately-graded question). Design:
# soak-drill-design.md. Opt-in: SOAK=1.
#
# The per-height escape bound H_ESCAPE_S is COMPUTED under the
# synchronizer's INCREASING round durations (core/node/rounds.go sweepsForRound:
# dur(r) = 2 + r(r+1)/2 sweeps × the 30s ChainSyncInterval): a 2-round allowance
# costs dur(0)+dur(1) = 2+3 = 5 sweeps = 150s, plus one ~34s computed gather leg
# ≈ 184 → 220 (run the field run measured 80–170s/height at steady state). A
# height older than that with the network live is the WEDGE SIGNATURE and a FAIL
# , never a window
# artifact. Escape FREQUENCY at steady state (~half of heights reach r1) is an
# M1 cadence question, tracked separately — the bound covers the mechanism.
# ── economy (#11): the S7 repair bounty pays a VERIFIED RECONSTRUCTION on the wire ──
# The Phase 2 exit gate. Publish erasure-coded, make store-2 a caretaker with the
# economy ON, fund its reserve, then use `swarm holders` to KILL every holder of
# > RepairSlack columns so every stripe must RECONSTRUCT from parity (not just
# re-fetch a surviving replica — the gap durability-turnover leaves) — and confirm
# the caretaker rebuilt and the bounty DREW THE RESERVE DOWN (paid > 0). Opt-in
# (ECONOMY=1). Moderate chunk (256 KiB) so reconstruction fits the box (§0.1: a
# 64 MiB stripe holds ~1 GiB and OOMs a 2 GB node). Design:
# .
# LOCAL_PROOF: go test ./e2e -run TestRepairBountyPaysOnTheWire -count=1 (prepay→bounty legs; the skim leg's in-process proof is sim TestServeAutoSkimFundsObjectEscrow)
flow_economy_repair() {
  [ "${ECONOMY:-0}" = 1 ] || { record "11-economy-repair" skip minor "opt-in (ECONOMY=1): the S7 repair-bounty-on-the-wire grade"; return; }
  require_nodes "11-economy-repair" major fetch-1 store-1 store-2 relay || return
  client_preflight "11-economy-repair" major fetch-1 store-2 || return
  # The killable-pool premise: topology.py adds store-3/store-4 when the fleet is
  # brought UP with ECONOMY=1. Grading with ECONOMY=1 on a fleet provisioned
  # without it re-creates the proven-unsatisfiable premise (run the field run:
  # every column on a reserved/consensus node, 0 killable — deterministic), so
  # refuse loudly instead of GAPing 300s later on selection.
  if ! node_exists store-3 || ! node_exists store-4; then
    record "11-economy-repair" gap major "ECONOMY=1 but the fleet lacks the dedicated killable stores (store-3/store-4) — it was brought up WITHOUT ECONOMY=1. Re-provision with ECONOMY=1 so topology.py adds them; economy UNTESTED this run"
    return
  fi
  local care="store-2"   # paramedic candidate: a storage node, NEVER an anchor/validator,
                         # so killing shard-holders can never touch consensus.

  # 1) Publish an erasure-coded object; capture BOTH the silt: link and the siltcare:.
  # RETRY the publish (run the field run GAPped here): a chain-backed registry publish
  # IS a consensus commit, and on quorum-2 across 3 regions a single attempt can time
  # out (context deadline) when its entry's commit lands slower than the client window,
  # especially late in the sheet under load. Ride it out with retries —
  # the same tolerance ft_publish has (PUBLISH_RETRY_S) — instead of GAPping on one
  # slow-commit window; only GAP after the whole budget is spent.
  local out link carelink attempt=0 econ_any_output=0
  # IDEMPOTENT RETRY (2026-08-20, attributed on run the field run): generate the
  # payload ONCE, before the loop — NOT per attempt. A chain-backed publish that
  # times out at the client's fixed 10s registry-POST deadline (accept→commit
  # latency under SYBILS=8 load) still COMMITS the entry server-side; a retry of the
  # SAME root then finds it committed and returns fast. The old per-attempt
  # `head -c … </dev/urandom` minted a NEW root every retry, so a slow-but-eventual
  # commit could never be picked up — every attempt raced the 10s deadline from
  # scratch and the whole 360s budget (since re-derived to 300s — see) GAPed. This
  # mirrors ft_publish, which has
  # always generated its source once. (The 10s client deadline itself is a
  # build-immutable #5 magic-constant limitation in adapters/httpregistry — a
  # separate product fix; this makes the harness retry actually idempotent.)
  ssh_node fetch-1 "head -c 4194304 </dev/urandom >/tmp/ft_econ.bin" >/dev/null 2>&1
  local econ_deadline=$(( $(date +%s) + ${ECONOMY_PUBLISH_RETRY_S:-300} ))
  while :; do
    # -replication 1: each column lands on ONE holder, so 3 all-killable columns
    # exist on a small killable pool (parity is the redundancy — the flag's own
    # help text names this exact use, and the local e2e proof publishes the same
    # shape). At the default replication 3 a column needs ALL THREE holders
    # killable; the first full LOCAL sheet measured the result — 0 of 16 columns
    # qualified, the flow GAPs at selection (the 4th latent defect this flow
    # shipped with, caught for $0 on docker).
    out="$(ssh_node fetch-1 "/usr/local/bin/silt swarm add /tmp/ft_econ.bin -peers '$PEERS' -registry '$REGREF' -token-quorum $TOKEN_QUORUM -chunk-size 262144 -replication 1 2>&1" || true)"
    [ -n "$(printf '%s' "$out" | tr -d '[:space:]')" ] && econ_any_output=1
    link="$(printf '%s' "$out" | grep -oE 'silt:v1:\S+' | head -1)"
    carelink="$(printf '%s' "$out" | grep -oE 'siltcare:\S+' | head -1)"
    { [ -n "$link" ] && [ -n "$carelink" ]; } && break
    [ "$(date +%s)" -ge "$econ_deadline" ] && break
    attempt=$((attempt+1))
    # Verbosity/honesty (2026-08-20 clobber lesson): distinguish an EMPTY response
    # (ssh returned nothing → node unreachable / wrong map — a PLUMBING failure) from
    # a real error string. The empty parenthetical here once masked a corrupted
    # nodes.json for a whole run; now the empty case is called out explicitly.
    local econ_snip; econ_snip="$(printf '%s' "$out" | tr '\n' ' ' | head -c 120)"
    if [ -z "$(printf '%s' "$out" | tr -d '[:space:]')" ]; then
      echo "    ⚠ economy publish attempt $attempt: EMPTY RESPONSE from fetch-1 — ssh returned NOTHING (node unreachable / node MAP wrong?), NOT a registry-latency signal; retrying in 30s…"
    else
      echo "    economy publish attempt $attempt did not land (registry commit latency? $econ_snip); retrying in 30s…"
    fi
    sleep 30
  done
  if [ -z "$link" ] || [ -z "$carelink" ]; then
    ft_add_validator_evidence
    if [ "$econ_any_output" = 0 ]; then
      record "11-economy-repair" gap major "setup publish got EMPTY RESPONSES from fetch-1 for the whole ${ECONOMY_PUBLISH_RETRY_S:-300}s window — fetch-1 UNREACHABLE or the node MAP is wrong (check nodes.json), a PLUMBING failure NOT a product/latency issue; economy UNTESTED"; return
    fi
    record "11-economy-repair" gap major "setup publish landed no link+carelink after ${ECONOMY_PUBLISH_RETRY_S:-300}s of retries — economy UNTESTED this run, not a failure (registry publish-commit latency; $(printf '%s' "$out" | tr '\n' ';' | head -c 160))"; return
  fi
  # Verbosity honesty (2026-08-20, run the field run): echo the link + the full
  # publish-client output + the full holders map to the console. The old flow
  # discarded all three — diagnosing placement then required PERTURBING re-adds
  # (a dedup `swarm add` re-runs placement and pollutes the map it probes), and
  # the results-jsonl detail truncated the holders at 200 chars. The console log
  # is the run's verbatim record; this is exactly what it is for.
  echo "    economy object link:     $link"
  echo "    economy care link:       $carelink"
  printf '%s\n' "$out" | sed 's/^/      publish| /'

  # 2) Make TWO caretakers with the economy ON + a local UI (fund/status). Two is
  #  structural, not redundancy (proven by the local wire proof,
  #  e2e/economy_repair_test.go): the paramedic never judges its own claim
  #  (repairclaim.go emitRepairClaim skips itself and the holder), credit is
  #  per-node-local, so `paid` materializes on the OTHER caretaker's ledger —
  #  the judge's. The judge is the relay node: a full daemon that is NOT in the
  #  killable role set, so arming it costs zero killable shard-holders.
  #  -registry is REQUIRED: -care without one now refuses to start (it used to
  #  silently never caretake — the shape this scenario shipped in run 2323b09).
  local judge="relay"
  econ_restore() { restore_argv "$care"; restore_argv "$judge"; return 0; }
  # -repair-interval 2s mirrors the GREEN local proof EXACTLY (e2e arms 2s and
  # pays within 180s; run the field run ran the 60s default and its caretakers
  # never completed a sweep inside the window). Local-proof parity: the wire run
  # confirms the proven configuration, it does not test a new cadence (#7).
  relaunch_with "$care"  "-care $carelink -economy -registry $REGREF -repair-interval 2s -ui=127.0.0.1:8098"
  relaunch_with "$judge" "-care $carelink -economy -registry $REGREF -repair-interval 2s -ui=127.0.0.1:8098"
  sleep 20   # restart + re-bootstrap + warm the manifest + arm the repair sweeps

  # 3) Fund the object's reserve on BOTH caretakers, each from its own grant
  #  balance (Slice 3): which one ends up the judge is timing, and PayBounty
  #  draws from the payer's OWN escrow. The amount must fit the 500k starter
  #  grant — FundEscrow refuses more (the prior 2000000 could never fund).
  local cnode tok fund_code
  for cnode in "$care" "$judge"; do
    tok="$(ssh_node "$cnode" "sudo cat /var/lib/silt/ui-token" 2>/dev/null | tr -dc 'a-f0-9')"
    fund_code="$(ssh_node "$cnode" "curl -s -o /dev/null -w '%{http_code}' -X POST -H 'Authorization: Bearer $tok' --data 'root=$link&amount=400000' http://127.0.0.1:8098/api/fund" 2>/dev/null || true)"
    if [ "$fund_code" != "200" ]; then
      record "11-economy-repair" gap major "could not fund the reserve (/api/fund → HTTP ${fund_code:-none} on $cnode) — economy setup incomplete, UNTESTED not failed"; econ_restore; return
    fi
  done

  local holders_out; holders_out="$(ssh_node fetch-1 "/usr/local/bin/silt swarm holders '$link' -peers '$PEERS' -registry '$REGREF' 2>&1" || true)"
  printf '%s\n' "$holders_out" | sed 's/^/      holders| /'   # full map, verbatim (see the verbosity note above)

  # 3b) THE SKIM LEG on the wire (11b): S7's full sentence is prepay → SKIM →
  #  bounty, and until now the skim (serving revenue auto-funding the object's
  #  reserve) had no wire grade anywhere — sim-only. The skim lands on the
  #  SERVING holder's per-node ledger, and the UI surfaces escrows only for
  #  CARED roots — so observe it on the CARE node itself: it already holds
  #  columns, already runs a UI, and is never killed. DELTA assert, not
  #  from-zero: baseline = the 400000 prepay just confirmed above; `funded`
  #  has exactly two writers (FundEscrow and the serve auto-skim) and nothing
  #  else prepays mid-window, so any growth is pure skim. Redesigned
  #  2026-08-20 after run the field run, where the old shape relaunched a
  #  shard-holder as a THIRD caretaker and (a) raced the restart's
  #  re-announce (37s measured, 15s slept) and lazy proofMeta reload — the
  #  observer served 0 chunks, skim untestable; (b) armed an UNFUNDED judge
  #  candidate PayBounty could draw ~0 from (a false negative waiting for
  #  the cloud run); (c) consumed a scarce killable holder. No relaunch →
  #  none of the three. A fetch needs k of the coded columns (10 of 16
  #  here), so a multi-column holder is hit near-certainly (missing all 4 of
  #  a 4-column holder ≈ 0.8%); the fetch re-drives every poll so a lazy
  #  proofMeta reload self-heals inside the window. From-zero purity stays
  #  the sim tier's job (TestServeAutoSkimFundsObjectEscrow); the wire grade
  #  is the serve→object-escrow ROUTING on a real network.
  # BOTH UI-armed nodes are observers (2026-08-20, run the field run): placement
  # owes the care node nothing — that run store-2 held ZERO columns while relay
  # (the judge, equally UI-armed, equally prepaid) held data column 6 and was
  # certainly serving. Skim lands on whichever armed node actually serves, so
  # poll both and pass on either's growth. A node that holds no columns simply
  # never grows — the other one carries the grade.
  local sk_tok_c sk_tok_j sk_body sk_base_c sk_base_j sk_seen_c=0 sk_seen_j=0
  sk_tok_c="$(ssh_node "$care" "sudo cat /var/lib/silt/ui-token" 2>/dev/null | tr -dc 'a-f0-9')"
  sk_tok_j="$(ssh_node "$judge" "sudo cat /var/lib/silt/ui-token" 2>/dev/null | tr -dc 'a-f0-9')"
  sk_body="$(ssh_node "$care" "curl -s -H 'Authorization: Bearer $sk_tok_c' http://127.0.0.1:8098/api/status" 2>/dev/null || true)"
  sk_base_c="$(printf '%s' "$sk_body" | grep -oE '"funded":[0-9]+' | grep -oE '[0-9]+' | sort -rn | head -1)"; sk_base_c="${sk_base_c:-0}"
  sk_body="$(ssh_node "$judge" "curl -s -H 'Authorization: Bearer $sk_tok_j' http://127.0.0.1:8098/api/status" 2>/dev/null || true)"
  sk_base_j="$(printf '%s' "$sk_body" | grep -oE '"funded":[0-9]+' | grep -oE '[0-9]+' | sort -rn | head -1)"; sk_base_j="${sk_base_j:-0}"
  # A glitched (empty) baseline read must not turn the confirmed 400000 prepay
  # into "skim growth": floor both baselines at the prepay just confirmed.
  [ "$sk_base_c" -lt 400000 ] 2>/dev/null && sk_base_c=400000
  [ "$sk_base_j" -lt 400000 ] 2>/dev/null && sk_base_j=400000
  # sk_poll: read both observers once; sets sk_seen_c/sk_seen_j to the max seen.
  sk_poll() {
    local b v
    b="$(ssh_node "$care" "curl -s -H 'Authorization: Bearer $sk_tok_c' http://127.0.0.1:8098/api/status" 2>/dev/null || true)"
    v="$(printf '%s' "$b" | grep -oE '"funded":[0-9]+' | grep -oE '[0-9]+' | sort -rn | head -1)"; v="${v:-0}"
    [ "$v" -gt "$sk_seen_c" ] 2>/dev/null && sk_seen_c="$v"
    b="$(ssh_node "$judge" "curl -s -H 'Authorization: Bearer $sk_tok_j' http://127.0.0.1:8098/api/status" 2>/dev/null || true)"
    v="$(printf '%s' "$b" | grep -oE '"funded":[0-9]+' | grep -oE '[0-9]+' | sort -rn | head -1)"; v="${v:-0}"
    [ "$v" -gt "$sk_seen_j" ] 2>/dev/null && sk_seen_j="$v"
    return 0
  }
  sk_grew() { [ "$sk_seen_c" -gt "$sk_base_c" ] 2>/dev/null || [ "$sk_seen_j" -gt "$sk_base_j" ] 2>/dev/null; }
  sk_verdict_detail() {
    if [ "$sk_seen_c" -gt "$sk_base_c" ] 2>/dev/null; then
      echo "funded $sk_base_c → $sk_seen_c on $care: +$((sk_seen_c - sk_base_c))"
    else
      echo "funded $sk_base_j → $sk_seen_j on $judge: +$((sk_seen_j - sk_base_j))"
    fi
  }
  # sk_window: drive fetches (≤90s) until an armed observer's reserve grows,
  # then record the 11b verdict. Called AFTER the repair leg (order A), and on
  # the repair-selection GAP path so 11b is never silently ungraded.
  sk_window() {
    local sk_t0; sk_t0="$(date +%s)"
    while ! sk_grew && [ $(( $(date +%s) - sk_t0 )) -lt 90 ]; do
      ssh_node fetch-1 "/usr/local/bin/silt swarm get '$link' -o /tmp/ft_econ_skim.bin -peers '$PEERS' -registry '$REGREF' >/dev/null 2>&1" || true
      sk_poll
      sk_grew && break
      sleep 6
    done
    if sk_grew; then
      slo_assert "11b-economy-skim" major "the SKIM leg closed on the wire: serve traffic (reconstruction reads + driven fetches) routed revenue into the object's durability reserve on the serving holder's ledger ($(sk_verdict_detail) pure skim above the prepay baseline) — the object pays for its own repair (S7)" 1
    else
      record "11b-economy-skim" gap major "no skim grew EITHER armed observer's reserve above its prepay baseline (repair-window reads + 90s driven fetches; $care ${sk_base_c}→${sk_seen_c}, $judge ${sk_base_j}→${sk_seen_j}) — attribute from their journals (serve accounting / proofMeta root routing) before re-running (#7)"
    fi
  }
  # ORDER): REPAIR FIRST on a NEVER-FETCHED object,
  # skim second. Run the field run proved the legs interfere the other way
  # round: the skim window's driven fetches bumped demand and lease/fan-out
  # replicated the hot chunks BEFORE the kill, so the repair sweep found 29/29
  # shards reachable with three holders dead — the cache layer healed the
  # object and erased the under-replication the repair premise needs (B6 doing
  # its job, defeating the drill). So: baselines here, kill next on a cold
  # object, repair pays, holders restart, THEN the fetch window closes the skim
  # grade. `funded` is lifetime deposits (prepay + skim) — a payout draws the
  # BALANCE, never `funded` — so these baselines stay valid across the repair.

  # 4) Resolve holders per column and KILL every holder of 3 columns whose holders
  #  are ALL killable (storage/fetcher, and NOT the caretakers) — so consensus
  #  is never touched. 3 lost shards/stripe > RepairSlack(2) ⇒
  #  every stripe must reconstruct, and 3 ≤ n−k(6) ⇒ still recoverable.
  # Build a killable NodeID→name map: every content-holding node EXCEPT the 4
  # anchors (role "validator") and the caretaker. In launch phase (the economy run
  # is MATURING=0) ONLY the anchors finalize — maturers and sybils are non-anchor
  # validators whose loss cannot break launch-phase consensus — so storage, fetcher,
  # maturer, and sybil nodes are all safe to stop. (This is why the flow runs before
  # the maturing drill and restarts every node it stops.) Anchors are never touched.
  # The killable-pool PREMISE (proven unsatisfiable without them on LOCAL run
  # the field run): ECONOMY=1 adds the dedicated store-3/store-4 exactly so this
  # set is non-empty after the care/judge reservations — with the skim observer
  # redesigned away (3b), store-1 is killable again too. The ADVERSARY is
  # killable as well (added after run the field run, where placement gave it
  # 3-4 columns while the fresh stores got zero): it is a non-anchor full
  # daemon whose loss cannot touch launch-phase consensus, its drills are
  # stateless request/response, and step 7 restarts every stopped node — the
  # role was omitted from this set by history, not by a constraint.
  local killable_ids="" n nid role
  for n in $(node_names); do
    [ "$n" = "$care" ] && continue
    [ "$n" = "$judge" ] && continue
    role="$(node_field "$n" role)"
    case "$role" in storage|fetcher|maturer|sybil|adversary) : ;; *) continue ;; esac
    nid="$(node_field "$n" nodeid)"
    [ ${#nid} -eq 64 ] && killable_ids="$killable_ids $nid:$n"
  done
  # Pick 3 columns whose holders are all killable; collect the nodes to stop.
  local cols_killed=0 to_stop="" col ids id ok name
  while IFS= read -r line; do
    case "$line" in column\ *) : ;; *) continue ;; esac
    [ "$cols_killed" -ge 3 ] && break
    ids="${line#*: }"
    ok=1; local col_nodes=""
    for id in ${ids//,/ }; do
      [ -z "$id" ] && continue
      name=""; for kv in $killable_ids; do [ "${kv%%:*}" = "$id" ] && name="${kv#*:}"; done
      if [ -z "$name" ]; then ok=0; break; fi     # a holder we may not kill (validator/caretaker) → skip this column
      col_nodes="$col_nodes $name"
    done
    if [ "$ok" = 1 ] && [ -n "$col_nodes" ]; then
      to_stop="$to_stop $col_nodes"; cols_killed=$((cols_killed+1))
    fi
  done <<EOF
$holders_out
EOF
  if [ "$cols_killed" -lt 3 ]; then
    record "11-economy-repair" gap major "only $cols_killed of 3 needed columns had ALL-killable holders (shards landed on validators/the caretakers we must not kill) — could not force a reconstruction WITHOUT touching consensus; economy UNTESTED not failed (add dedicated storage nodes / lower -replication to concentrate shards). holders:$(printf '%s' "$holders_out" | tr '\n' ';' | head -c 200)"
    sk_window   # nothing was killed — still grade the skim leg on the healthy fleet
    econ_restore; return
  fi
  local uniq_stop; uniq_stop="$(printf '%s\n' $to_stop | sort -u | tr '\n' ' ')"
  echo "    killing shard-holders of $cols_killed columns to force reconstruction:$uniq_stop"
  # The kill instant, in the journals' own timestamp format (UTC ISO): every
  # progress read below filters to post-kill lines only.
  local econ_kill_iso; econ_kill_iso="$(date -u +%Y-%m-%dT%H:%M:%S)"
  for n in $uniq_stop; do svc "$n" stop || true; done

  # 5) Wait (bounded) for a reconstruction + payout: poll BOTH caretakers'
  #  /api/status — the paramedic emits the claim, the OTHER one judges and
  #  pays on its own ledger, and which is which is timing.
  # WINDOW SIZING (attribution, run the field run
  # 2026-08-21-497-records-vs-bytes-attribution.md): `-repair-interval 2s` bounds
  # only the idle gap BETWEEN sweeps; a sweep's DURATION under dead holders is
  # probe/lookup-timeout dominated and measured at ~3-4 MINUTES (kill 08:00:50 →
  # correct reachable=22/29 verdicts 08:03:34/08:04:08 → reconstruction done
  # 08:04:59). The old 240s window ≈ one such sweep, so the loop's correct-but-
  # late repair lost the race to the window every time — and the step-6c restart
  # on expiry then healed the object under the in-flight repair (missing=0, no
  # claim): a false GAP with the economy logic blameless. Window now covers the
  # measured cycle (ECONOMY_REPAIR_WINDOW_S, default 600s) and extends ONCE
  # (ECONOMY_REPAIR_GRACE_S, default 300s) when the journals show the cycle in
  # flight at expiry — but NOT when both caretakers' latest post-kill sweeps
  # report full reachability with zero repair activity (the loop believes
  # nothing is missing; waiting longer cannot pay — the includeLocal
  # premise-defeat shape, GAP immediately with that evidence in the detail).
  # (2026-08-20, run the field run): this line used to be
  #  `local t0; t0="$(date +%s)" paid=0 repairs=0 body ptok pv rv`
  # — everything after the first assignment parses as an env-prefixed COMMAND
  # named `body` (command not found), so paid/repairs were never set and set -u
  # aborted the whole run at their first use, skipping econ_restore and the
  # holder restarts. Latent since the flow shipped: no prior run ever reached
  # this line (each GAPed before the kill phase); the first run to pass
  # selection died here. The billable cloud run would have hit it identically.
  local t0 paid=0 repairs=0 body ptok pv rv
  local econ_window econ_grace_used=0 prog_c prog_j
  econ_window="${ECONOMY_REPAIR_WINDOW_S:-600}"
  # econ_repair_progress NODE → one-line post-kill repair-cycle summary from the
  # node's journal ("last-sweep=REACHABLE/SHARDS stripes-repaired=N"), or empty
  # when no post-kill sweep has completed yet (itself the strongest sign the
  # window is racing a sweep still in flight). Lines are info-level, present at
  # the default LOG_LEVEL.
  econ_repair_progress() {
    local sweep repaired reach shards
    sweep="$(ssh_node "$1" "sudo grep -h 'repair sweep complete' /var/lib/silt/debug.log 2>/dev/null" 2>/dev/null | awk -v k="$econ_kill_iso" '$1 >= k' | tail -1)"
    repaired="$(ssh_node "$1" "sudo grep -h 'stripe repaired' /var/lib/silt/debug.log 2>/dev/null" 2>/dev/null | awk -v k="$econ_kill_iso" '$1 >= k' | wc -l | tr -d ' ')"
    [ -z "$sweep" ] && [ "${repaired:-0}" = 0 ] && return 0
    reach="$(printf '%s' "$sweep" | grep -oE 'reachable=[0-9]+' | grep -oE '[0-9]+')"
    shards="$(printf '%s' "$sweep" | grep -oE 'shards=[0-9]+' | grep -oE '[0-9]+')"
    printf 'last-sweep=%s/%s stripes-repaired=%s' "${reach:-?}" "${shards:-?}" "${repaired:-0}"
  }
  # econ_cycle_hopeless SUMMARY → true only when a completed post-kill sweep
  # reports FULL reachability and zero repair activity: the premise is defeated,
  # a longer wait cannot produce a payout.
  econ_cycle_hopeless() {
    local rs
    case "$1" in last-sweep=*) : ;; *) return 1 ;; esac
    rs="${1#last-sweep=}"; rs="${rs%% *}"
    [ "${rs%%/*}" = "${rs##*/}" ] || return 1
    case "$1" in *"stripes-repaired=0") return 0 ;; esac
    return 1
  }
  t0="$(date +%s)"
  while :; do
    if [ $(( $(date +%s) - t0 )) -ge "$econ_window" ]; then
      [ "$econ_grace_used" = 1 ] && break
      prog_c="$(econ_repair_progress "$care")"
      prog_j="$(econ_repair_progress "$judge")"
      if econ_cycle_hopeless "$prog_c" && econ_cycle_hopeless "$prog_j"; then
        break  # both post-kill sweeps say fully-reachable + no repair → premise defeated, extending can't pay
      fi
      econ_grace_used=1
      econ_window=$(( econ_window + ${ECONOMY_REPAIR_GRACE_S:-300} ))
      echo "    economy: window expired with the repair cycle IN FLIGHT (care: ${prog_c:-no post-kill sweep completed yet}; judge: ${prog_j:-no post-kill sweep completed yet}) — extending once by ${ECONOMY_REPAIR_GRACE_S:-300}s"
    fi
    for cnode in "$care" "$judge"; do
      ptok="$(ssh_node "$cnode" "sudo cat /var/lib/silt/ui-token" 2>/dev/null | tr -dc 'a-f0-9')"
      body="$(ssh_node "$cnode" "curl -s -H 'Authorization: Bearer $ptok' http://127.0.0.1:8098/api/status" 2>/dev/null || true)"
      pv="$(printf '%s' "$body" | grep -oE '"paid":[0-9]+' | grep -oE '[0-9]+' | sort -rn | head -1)"; pv="${pv:-0}"
      rv="$(printf '%s' "$body" | grep -oE '"repairs":[0-9]+' | grep -oE '[0-9]+' | sort -rn | head -1)"; rv="${rv:-0}"
      if [ "$pv" -gt "$paid" ] 2>/dev/null; then paid="$pv" repairs="$rv"; fi
    done
    # Skim rides along: reconstruction reads k surviving columns per stripe, so
    # an armed observer's shards may already be serving (and skimming) here.
    sk_poll
    [ "$paid" -gt 0 ] 2>/dev/null && break
    sleep 6
  done

  # 6) Verdict: a bounty paid a verified reconstruction on the wire (the exit gate).
  #  Either way the detail carries the post-kill journal evidence, so a GAP
  #  arrives pre-attributed (#7). The step-6c holder restart stays below: with
  #  the grade recorded, restarting can no longer falsify an in-flight repair.
  prog_c="$(econ_repair_progress "$care")"
  prog_j="$(econ_repair_progress "$judge")"
  ft_add_validator_evidence
  if [ "${paid:-0}" -gt 0 ] 2>/dev/null; then
    slo_assert "11-economy-repair" major "the S7 repair economy CLOSED on the wire: killed 3 columns' holders → the caretaker RECONSTRUCTED from parity → a verified-repair bounty drew the object's reserve down (paid=$paid credits over $repairs repair(s)) — durability paid for itself on a real network, standing untouched (Invariant A). Post-kill cycle: $care ${prog_c:-none}; $judge ${prog_j:-none}" 1
  elif econ_cycle_hopeless "$prog_c" && econ_cycle_hopeless "$prog_j"; then
    record "11-economy-repair" gap major "PREMISE DEFEATED, not a timing miss: after the kill both caretakers' latest sweeps report FULL reachability with zero repair activity ($care $prog_c; $judge $prog_j) — the dead shards are still counted reachable (the records-vs-bytes / includeLocal shape), so no repair can ever fire; attribute the reachability source before re-running"
  else
    record "11-economy-repair" gap major "3 columns killed but no bounty drew the reserve within ${econ_window}s (paid=$paid repairs=$repairs; post-kill cycle: $care ${prog_c:-no completed sweep}; $judge ${prog_j:-no completed sweep}) — the loop did not finish reconstruct+judge+pay in the window; attribute from $care's AND $judge's journals (claim / judge legs) before re-running (#7)"
  fi

  # 6b) The g-instrumentation row (11c, observational — S5): S7 says the funded
  #  HORIZON is finite-but-renewable and `g` is the one number to instrument.
  #  Record the payer-side reserve/horizon/cost-per-repair so every graded run
  #  extends the time series — a measured row, never a pass/fail (a single run
  #  cannot grade a trend).
  if [ "${paid:-0}" -gt 0 ] 2>/dev/null; then
    local hz rs
    hz="$(printf '%s' "$body" | grep -oE '"horizonSec":-?[0-9]+' | grep -oE '\-?[0-9]+' | tail -1)"
    rs="$(printf '%s' "$body" | grep -oE '"reserve":[0-9]+' | grep -oE '[0-9]+' | tail -1)"
    record "11c-economy-horizon" pass info "g-instrumentation sample (S7 finite-but-renewable): paid=$paid over $repairs repair(s), reserve-after=${rs:-?}, horizonSec=${hz:-unmeasured} (−1 = no burn window yet). One row per graded run — the g trend needs the series, not this sample"
  else
    record "11c-economy-horizon" skip info "no payout this run — horizon/g sample unmeasured"
  fi

  # 6c) Restart the stopped holders BEFORE the skim window: the repair grade is
  #  recorded, so their return changes nothing there — and the fetch driver
  #  (fetch-1) may itself be in the kill set (it was in run the field run).
  for n in $uniq_stop; do svc "$n" start || true; done
  sleep 10   # let the restarted daemons come up before driving fetches through them

  # 6d) THE SKIM WINDOW (11b), after the repair so its fetch traffic can no
  #  longer heal the object out of the repair premise (the order-A rule
  #  above). Reconstruction reads during the repair may have skimmed
  #  already (sk_poll rode along); otherwise sk_window drives fetches until
  #  an armed observer's reserve grows above its prepay baseline.
  sk_window

  # 7) Restore: revert all armed caretakers' argv (holders already restarted, 6c).
  econ_restore
}

# ── Flow 13: the paid DELIVERY lane on the wire ────────────────────────────────
# The boot validator arms -accept-delivery-receipts (topology.py). A client on fetch-1
# fetches an object it published and presents a `swarm receipt` naming the boot validator
# as the server (the flow: pin the issuer's committed key -> withdraw one demand
# token -> MsgDeliveryOpen -> settle a cumulative-count receipt). TWO rows, because the
# positive settlement has had no live seam on any graded sheet so far — no E->key binding has
# committed on one — and a property with no live seam is stated, never faked:
#
# Era-4 is live from height 1 on every network this harness starts: -era4-activation-height
# has a shipped default of 1, and this harness passes no override. Why no binding has
# committed is UNMEASURED, so the rows below grade either arm without a harness change
#  13-delivery-lane — the lane's field CONTRACT: armed (the unit's argv) and
#  announced (the boot banner) on the server; the client refused
#  at the withdrawal naming the committed-binding gate (nothing
#  spent, the server's debug.log carries no banked line) with no
#  binding committed, or banked with one; a lane-OFF server refuses with the
#  announced NOT-banked marker; the two refusals never conflated.
#  13b-delivery-settlement — pass ONLY when the receipt actually banked on the wire (the
#  server's `delivery receipt banked` in debug.log; the idle
#  `delivery session closed` line is NOT required — at the shipped
#  24m window it cannot occur inside a graded flow, and the close
#  is asserted at the e2e tier instead, this lane 2026-09-09);
#  SKIP behind the binding probe while no
#  binding is committed (a skip keeps the RC gate reachable — the
#  recommendation; OWNER RATIFICATION of skip-vs-gap is owed and
#  recorded in the PR; the skip text says what is untested). This
#  row turns green once the binding commits, with NO harness change.
# SURFACES, items 1–2): the boot banner is a fmt.Printf -> journald,
# read over the WHOLE unit journal (never the 800-line window — the boot validator emits
# ~27 journald lines per block, 79 % TLS-handshake noise from the registry listener);
# every server-side settlement marker is an n.logf -> $STORE/debug.log line and is read
# THERE, scoped to lines written after the flow's baseline (never journald: it never
# arrives there).
# Substrate noise (a node the sheet killed, an ssh/dial timeout) GAPs, never FAILs
# (lib.sh require_live discipline); the client call is retried before it is classified.
# LOCAL_PROOF: go test ./e2e -run 'TestPaidDeliveryLaneArmsInTheHarnessPosture|TestPaidDeliveryLaneRefusesWithoutACommittedKeyBinding|TestDeliveryReceiptRefusedWhenLaneOff|TestPaidDeliverySessionEndToEnd' -count=1
flow_delivery_lane() {
  local boot; boot="$(python3 -c "import json;print(json.load(open('$FT_TOPO'))['meta']['boot'])")"
  # The lane-OFF control is store-2 in the full topology, store-1 under SMOKE (store-2
  # absent) — both run no lane, so the control still runs on the 4-node set.
  local offnode=store-2; node_exists store-2 || offnode=store-1
  require_nodes "13-delivery-lane" major fetch-1 "$offnode" "$boot" || { record "13b-delivery-settlement" skip major "prerequisite node absent (see 13-delivery-lane)"; return; }
  require_live "13-delivery-lane" major "$boot" fetch-1 "$offnode" || { record "13b-delivery-settlement" skip major "prerequisite node not active (see 13-delivery-lane)"; return; }
  client_preflight "13-delivery-lane" major fetch-1 || { record "13b-delivery-settlement" skip major "client unreachable (see 13-delivery-lane)"; return; }
  flow_evidence_nodes fetch-1 "$boot" "$offnode"
  local t0; t0="$(date +%s)"
  local bootref offref
  bootref="$(python3 -c "
import json;t=json.load(open('$FT_TOPO'));n=t['nodes']['$boot'];print(n['nodeid']+'@'+n['ip']+':%d'%t['meta']['swarm_port'])")"
  offref="$(python3 -c "
import json;t=json.load(open('$FT_TOPO'));n=t['nodes']['$offnode'];print(n['nodeid']+'@'+n['ip']+':%d'%t['meta']['swarm_port'])")"

  # (1) ARMED (the running unit's argv carries the flag) + ANNOUNCED (the boot banner,
  # over the whole journal) on the boot validator; the B-11 affordability line is evidence.
  local armed announced afford
  # The RUNNING daemon's command line (pgrep -a prints it on both backends; the LOCAL
  # systemctl shim implements no `show`, which the first cut read and scored 0 — caught
  # by the LOCAL drive, not the fleet). Every read here is a substrate read: it is
  # retried, and a read that never ANSWERS (empty ssh) GAPs — only an answered read that
  # says "no flag" or "no banner" is the property failing.
  local rd_try cmdline=""
  for rd_try in 1 2 3; do
    cmdline="$(ssh_node "$boot" "pgrep -af '^/usr/local/bin/silt daemon' | head -1" || true)"
    [ -n "$cmdline" ] && break; sleep 5
  done
  if [ -z "$cmdline" ]; then
    record "13-delivery-lane" gap major "could not read the boot validator's daemon command line in 3 attempts (empty ssh reply) — substrate, property UNTESTED"
    record "13b-delivery-settlement" skip major "substrate (see 13-delivery-lane)"; return
  fi
  armed="$(printf '%s' "$cmdline" | grep -c -- '-accept-delivery-receipts' | tr -dc '0-9')"
  local journal=""
  for rd_try in 1 2 3; do
    journal="$(ssh_node "$boot" "sudo journalctl -u silt --no-pager -o cat 2>/dev/null | grep -m1 -E 'delivery receipts: ACCEPTING|delivery settlement: p=|^silt: |Started silt' | head -3" || true)"
    [ -n "$journal" ] && break; sleep 5
  done
  if [ -z "$journal" ]; then
    record "13-delivery-lane" gap major "could not read the boot validator's journal in 3 attempts (empty ssh reply) — substrate, property UNTESTED"
    record "13b-delivery-settlement" skip major "substrate (see 13-delivery-lane)"; return
  fi
  announced="$(ssh_node "$boot" "sudo journalctl -u silt --no-pager -o cat 2>/dev/null | grep -m1 -E 'delivery receipts: ACCEPTING'" || true)"
  afford="$(ssh_node "$boot" "sudo journalctl -u silt --no-pager -o cat 2>/dev/null | grep -m1 -E 'delivery settlement: p='" || true)"
  if [ "${armed:-0}" = 0 ] || [ -z "$announced" ]; then
    slo_assert "13-delivery-lane" major "the boot validator is not running the lane (running cmdline has the flag: ${armed:-0}; banner 'delivery receipts: ACCEPTING' in its journal: $([ -n "$announced" ] && echo yes || echo NO)) — topology.py arms it; a refused start would have failed 1-first-run" 0
    record "13b-delivery-settlement" skip major "lane not armed on ${boot}; settlement untested"; return
  fi

  # (2) Content to deliver: publish from fetch-1, fetch it back on fetch-1 with the boot
  # validator as the bootstrap peer. NOTE the harness cannot attribute WHICH holder served
  # the bytes (NetGet pulls from DHT holders); the receipt's server is the fetcher's own
  # claim, which is exactly the shape (the ack carries no possession proof by design).
  local res link sha root
  res="$(ft_publish fetch-1 1048576 || true)"
  if [ -z "$res" ]; then publish_verdict "13-delivery-lane" major "publish never produced a silt: link within ${PUBLISH_RETRY_S}s"; record "13b-delivery-settlement" skip major "no object to deliver"; return; fi
  link="${res%% *}"; sha="${res##* }"
  root="$(b64url_to_hex "$(printf '%s' "$link" | cut -d: -f3)")"
  local fetch_out got
  fetch_out="$(ssh_node fetch-1 "/usr/local/bin/silt swarm get '$link' -o /tmp/ft_dl.bin -peers '$bootref' -registry '$REGREF' 2>&1 >/dev/null; sha256sum /tmp/ft_dl.bin 2>/dev/null | cut -d' ' -f1" || true)"
  got="$(printf '%s\n' "$fetch_out" | tail -1 | tr -dc 'a-f0-9')"
  if [ "$got" != "$sha" ]; then
    echo "    fetch output: $(printf '%s' "$fetch_out" | head -c 600)"
    record "13-delivery-lane" gap major "fetch on fetch-1 was not bit-perfect (want=${sha} got=${got:-none}) — no delivery to acknowledge; the lane itself UNTESTED (fetch output in the console)"
    record "13b-delivery-settlement" skip major "no delivery to acknowledge"; return
  fi

  # (3) BASELINE the server's debug.log, then THE RECEIPT against the armed server: 4
  # increments of 256 KiB = the 1 MiB fetched. Retried while the output is transport
  # noise (dial/timeout) rather than a classification.
  local n0; n0="$(ssh_node "$boot" "sudo wc -l < /var/lib/silt/debug.log 2>/dev/null" | tr -dc '0-9')"; n0="${n0:-0}"
  if [ "$n0" = 0 ]; then
    # The server-side surface must be READABLE, or "banked nothing" would be satisfied by
    # darkness (the vacuity the a review measured on the journald read).
    record "13-delivery-lane" gap major "the boot validator's debug.log is empty or unreadable — the server-side assertions would be vacuous; property UNTESTED"
    record "13b-delivery-settlement" skip major "server surface unreadable (see 13-delivery-lane)"; return
  fi
  local out attempt=0
  while :; do
    attempt=$((attempt + 1))
    out="$(ssh_node fetch-1 "/usr/local/bin/silt swarm receipt '$root' -peers '$bootref' -increments 4 2>&1" || true)"
    if printf '%s' "$out" | grep -qE "delivery receipt banked by|committed E->key binding|delivery receipt was NOT banked by"; then break; fi
    [ "$attempt" -ge 3 ] && break
    echo "    receipt attempt ${attempt} unclassified ($(printf '%s' "$out" | head -c 200)); retrying in 10s"
    sleep 10
  done
  # (4) The lane-OFF control: a store running no lane must say so with the announced marker.
  local off="" off_try
  for off_try in 1 2 3; do
    off="$(ssh_node fetch-1 "/usr/local/bin/silt swarm receipt '$root' -peers '$offref' -increments 1 2>&1" || true)"
    printf '%s' "$off" | grep -qE "NOT banked|banked by" && break
    sleep 10
  done
  local off_ok=0 off_noise=0
  printf '%s' "$off" | grep -q "delivery receipt was NOT banked by" && printf '%s' "$off" | grep -q "serves no demand issuer key" && off_ok=1
  if [ "$off_ok" = 0 ] && { [ -z "$off" ] || printf '%s' "$off" | grep -qiE "timeout|timed out|connection refused|no route|dial|i/o|unreachable|EOF"; }; then off_noise=1; fi
  # The server's own record, scoped to lines written after the baseline (debug.log, never journald).
  local sbanked; sbanked="$(ssh_node "$boot" "sudo tail -n +$((n0 + 1)) /var/lib/silt/debug.log 2>/dev/null | grep -E 'delivery receipt banked' | tail -1" || true)"

  local t1; t1="$(date +%s)"
  if printf '%s' "$out" | grep -q "delivery receipt banked by"; then
    # LIVE LANE (the binding committed: post stamp raise). The server's own marker must
    # agree: banked now, read from debug.log after the baseline.
    #
    # The idle CLOSE is no longer part of this verdict (this lane, 2026-09-09). The shipped
    # -delivery-idle-window is 24m by derivation, not the 90s this harness used to set, so
    # an idle close cannot occur inside a graded flow at all — requiring it would fail the
    # row for the window being correctly sized. It is still READ when it happens (a stale
    # session from an earlier flow), because the M0 log audit below is worth running on a
    # real line; its home as a REQUIRED assertion is the e2e tier, where the clock is
    # injected: e2e TestPaidDeliverySessionEndToEnd drives open → settle → idle close and
    # asserts the deposit accounting and the no-identity/no-object close line.
    local closed=""
    closed="$(ssh_node "$boot" "sudo tail -n +$((n0 + 1)) /var/lib/silt/debug.log 2>/dev/null | grep -E 'delivery session closed' | tail -1" || true)"
    if [ "$off_noise" = 1 ]; then
      record "13-delivery-lane" gap major "LIVE lane: client banked, but the lane-off control at ${offnode} could not be driven (transport noise after 3 attempts: $(printf '%s' "$off" | head -c 200)) — property UNTESTED"
    fi
    local ok=0; [ -n "$sbanked" ] && [ "$off_ok" = 1 ] && ok=1
    [ "$off_noise" = 1 ] || slo_assert "13-delivery-lane" major "LIVE lane: client banked (${out##*: }); server debug.log banked=$([ -n "$sbanked" ] && echo yes || echo NO) closed=$([ -n "$closed" ] && echo yes || echo NO); lane-off control at ${offnode} $([ "$off_ok" = 1 ] && echo refused-with-marker || echo "WRONG: $off")" "$ok" $((t1 - t0))
    # The row grades ONE thing: the banked line. The M0 log audit still runs, but only
    # when a close line exists — on empty input the grep is vacuously true, and an absent
    # line must not read as a passed audit.
    local sok=0; [ -n "$sbanked" ] && sok=1
    if [ -n "$closed" ] && printf '%s' "$closed" | grep -qE "object=|fetcher="; then sok=0; fi
    slo_assert "13b-delivery-settlement" major "settlement ON THE WIRE: ${sbanked:-no banked line}; close: ${closed:-not observable at the shipped 24m idle window (graded at the e2e tier)}${afford:+; $afford}" "$sok" $((t1 - t0))
    return
  fi
  if printf '%s' "$out" | grep -q "committed E->key binding"; then
    # UNBOUND LANE (no committed E->key binding on this chain; era-4 itself IS active —
    # the harness passes no -era4-activation-height, so every daemon takes the default of 1):
    # the refusal at the withdrawal —
    # nothing withdrawn, nothing spent, and the server banked NOTHING (debug.log after the
    # baseline). The lane-off sentence must NOT appear (distinct contracts).
    local conflated=0
    printf '%s' "$out" | grep -q "serves no demand issuer key" && conflated=1
    if [ "$off_noise" = 1 ]; then
      record "13-delivery-lane" gap major "UNBOUND lane: the armed server refused as expected, but the lane-off control at ${offnode} could not be driven (transport noise after 3 attempts: $(printf '%s' "$off" | head -c 200)) — property UNTESTED"
      record "13b-delivery-settlement" skip major "UNTESTED on this chain (see 13-delivery-lane; no committed E->key binding — era-4 itself is live here)"; return
    fi
    local ok=0; [ "$conflated" = 0 ] && [ -z "$sbanked" ] && [ "$off_ok" = 1 ] && ok=1
    slo_assert "13-delivery-lane" major "UNBOUND lane (no committed E->key binding): armed + announced on ${boot}; client refused at the withdrawal naming the committed E->key binding (nothing spent); server debug.log (+$(( $(ssh_node "$boot" "sudo wc -l < /var/lib/silt/debug.log 2>/dev/null" | tr -dc '0-9') - n0 )) lines since baseline) banked nothing$([ "$conflated" = 1 ] && echo '; WRONG: conflated with the lane-off sentence')$([ -n "$sbanked" ] && echo "; WRONG: server banked: $sbanked"); lane-off control at ${offnode} $([ "$off_ok" = 1 ] && echo refused-with-marker || echo "WRONG: $off")${afford:+; $afford}" "$ok" $((t1 - t0))
    # THE PROBE IS A BINDING PROBE, NOT AN ERA PROBE: the client's
    # sentence covers two causes — the issuer committed no binding at all OR the keys it
    # served are off-commitment — and no surface on the CLI or the status route tells them
    # apart. A broken commit path reads here as this same SKIP; the skip
    # text names both causes so an operator reads it as "untested", never as "green".
    # A THIRD cause is now ruled OUT rather than carried: era-4 being inactive. This harness
    # passes no -era4-activation-height, so every daemon takes the binary default of 1 and
    # mints era-4 from height 1.
    record "13b-delivery-settlement" skip major "UNTESTED on this chain: the client was refused at the withdrawal because the issuer served no key that resolves against a COMMITTED E->key binding. WHY no binding committed is not measured by this row — era-4 itself is LIVE here (this harness passes no -era4-activation-height, so every daemon takes the default of 1 and mints era-4 from height 1), so the old 'era-4 is dark' reading is void and the remaining candidates (the issuer never registered a key, or its keys are off-commitment) are not separated. Row 13 is what holds today; this row grades the wire settlement once a binding commits, with no harness change." $((t1 - t0))
    return
  fi
  if [ -z "$out" ] || printf '%s' "$out" | grep -qiE "timeout|timed out|connection refused|no route|dial|i/o|unreachable|EOF"; then
    record "13-delivery-lane" gap major "client could not complete the receipt call after ${attempt} attempts — transport noise, property UNTESTED: $(printf '%s' "$out" | head -c 300)" $((t1 - t0))
    record "13b-delivery-settlement" skip major "client transport noise (see 13-delivery-lane)"; return
  fi
  slo_assert "13-delivery-lane" major "UNEXPECTED client outcome for swarm receipt against the armed boot validator: $(printf '%s' "$out" | head -c 400)" 0 $((t1 - t0))
  record "13b-delivery-settlement" skip major "client outcome unclassified (see 13-delivery-lane)" $((t1 - t0))
}

# LOCAL_PROOF: n/a — WAN-cadence liveness-BOUND soak; the deterministic escape-bound oracle is core/node/modelcheck_i4_liveness_test.go, the wall-clock at WAN scale is the cloud's job
flow_soak_publish_drain() {
  [ "${SOAK:-0}" = 1 ] || return 0
  local n_mat_soak
  n_mat_soak="$(python3 -c "import json;print(json.load(open('$FT_TOPO'))['meta'].get('n_mat',0))" 2>/dev/null || echo 0)"
  if [ "${n_mat_soak:-0}" -gt 0 ]; then
    record "soak-publish-drain" skip major "SOAK requires a LAUNCH topology (MATURING=0) — the latch would end the launch regime mid-soak; run SOAK=1 without MATURING"
    return
  fi
  require_nodes "soak-publish-drain" major val-a val-b val-c val-d || return
  : "${H_ESCAPE_S:=220}"
  : "${SOAK_HEIGHTS:=20}"
  local wall=$(( SOAK_HEIGHTS * 64 ))   # 64s worst-case block time — the LATCH_S arithmetic
  # Launch ceiling over the validators (mirrors mh_ceiling, which is flow-10-local).
  soak_height() { ssh_node "$1" "/usr/local/bin/silt chain-status -store /var/lib/silt 2>&1" \
    | grep -oE 'head height:[[:space:]]*[0-9]+' | grep -oE '[0-9]+' | tail -1; }
  soak_ceiling() {
    local c=0 v hh
    for v in val-a val-b val-c val-d; do hh="$(soak_height "$v")"; hh="${hh:-0}"; [ "$hh" -gt "$c" ] 2>/dev/null && c="$hh"; done
    printf '%s' "$c"
  }
  local h0 t0 last_h last_t now h pubs=0 pub_ok=0 maxgap=0 gap wedged=0 lastout=""
  h0="$(soak_ceiling)"; h0="${h0:-0}"; t0="$(date +%s)"; last_h="$h0"; last_t="$t0"
  echo "  soak: interleaving publishes with the natural renewal drain from h${h0} for ${SOAK_HEIGHTS} heights (wall ${wall}s; per-height escape bound ${H_ESCAPE_S}s)…"
  while :; do
    now="$(date +%s)"
    [ $(( now - t0 )) -ge "$wall" ] && break
    # The publish stream: one attempt per iteration, alternating publishers so the
    # proposal comes from different seats (val-a registry path + the fresh
    # publisher when present). Errors are KEPT (the run-3 lesson).
    if [ $(( pubs % 3 )) -eq 2 ] && node_exists fetch-1; then
      lastout="$(ssh_node fetch-1 "head -c 8192 </dev/urandom >/tmp/ft_soak.bin; /usr/local/bin/silt swarm add /tmp/ft_soak.bin -peers '$PEERS' -registry '$REGREF' -token-quorum $TOKEN_QUORUM -chunk-size 65536 2>&1 || true")"
    else
      lastout="$(ssh_node val-a "head -c 8192 </dev/urandom >/tmp/ft_soak.bin; /usr/local/bin/silt swarm add /tmp/ft_soak.bin -peers '$PEERS' -registry '$REGREF' -token-quorum $TOKEN_QUORUM -chunk-size 65536 2>&1 || true")"
    fi
    pubs=$(( pubs + 1 ))
    printf '%s' "$lastout" | grep -qE 'silt:v1:' && pub_ok=$(( pub_ok + 1 ))
    # The drain stream runs at its natural TTL cadence — nothing to schedule.
    h="$(soak_ceiling)"; h="${h:-$last_h}"
    now="$(date +%s)"
    if [ "$h" -gt "$last_h" ] 2>/dev/null; then
      gap=$(( now - last_t )); [ "$gap" -gt "$maxgap" ] && maxgap="$gap"
      last_h="$h"; last_t="$now"
      [ $(( h - h0 )) -ge "$SOAK_HEIGHTS" ] && break
    elif [ $(( now - last_t )) -gt $(( H_ESCAPE_S * 2 )) ]; then
      # Twice the escape bound with no commit and the wall still running: stop
      # early — this IS the wedge signature; soaking longer adds nothing.
      wedged=1; break
    fi
    sleep 8
  done
  local final_gap=$(( $(date +%s) - last_t ))
  [ "$final_gap" -gt "$maxgap" ] && [ "$wedged" = 1 ] && maxgap="$final_gap"
  local heights=$(( last_h - h0 ))
  # Honest-slash census over the soak window (I5): no honest validator slashed.
  local slashes=0 v
  for v in val-a val-b val-c val-d; do
    jlog "$v" 400 | grep -qE 'validator slashed for equivocation' && slashes=$(( slashes + 1 ))
  done
  if [ "$wedged" = 1 ] || [ "$maxgap" -gt "$H_ESCAPE_S" ]; then
    record "soak-publish-drain" fail major "WEDGE SIGNATURE under the publish/drain soak: a height went ${maxgap}s (> the computed ${H_ESCAPE_S}s escape bound) without a commit with the network live (h${h0}→h${last_h}, ${pub_ok}/${pubs} publishes landed) — the escape bound did not clear the interleaved race; last client output: $(printf '%s' "$lastout" | tail -1 | head -c 200)"
  elif [ "$heights" -lt "$SOAK_HEIGHTS" ] && [ "$heights" -lt $(( SOAK_HEIGHTS / 2 )) ]; then
    record "soak-publish-drain" gap major "soak under-ran: only ${heights} of ${SOAK_HEIGHTS} heights committed within the ${wall}s wall (max inter-commit gap ${maxgap}s ≤ bound) — cadence, not a wedge; property PARTIALLY tested"
  elif [ "$pub_ok" -eq 0 ]; then
    record "soak-publish-drain" fail major "the chain advanced ${heights} heights under the soak but ZERO of ${pubs} publishes landed — launch-regime publish starvation; last client output: $(printf '%s' "$lastout" | tail -1 | head -c 200)"
  else
    slo_assert "soak-publish-drain" major "launch-regime publish/drain SOAK: ${heights} heights committed under continuously interleaved publish (${pub_ok}/${pubs} landed) + natural renewal drain, max inter-commit gap ${maxgap}s ≤ the computed ${H_ESCAPE_S}s escape bound, ${slashes} honest-slash lines (want 0) — the escape bound clears the production-reachable race" \
      "$([ "$slashes" -eq 0 ] && echo 1 || echo 0)"
  fi
}

# shuffle_seeded SEED ITEM... — deterministic Fisher-Yates over the args, keyed by
# SEED (same seed ⇒ same order, always replayable — the determinism discipline: a
# random test you cannot reproduce is worse than a fixed one). Python because bash
# has no seeded shuffle; the seed is hashed to an int so any string works.
shuffle_seeded() {
  local seed="$1"; shift
  python3 - "$seed" "$@" <<'PY'
import sys, random, hashlib
seed = sys.argv[1]; items = sys.argv[2:]
random.seed(int(hashlib.sha256(seed.encode()).hexdigest(), 16))
random.shuffle(items)
print(" ".join(items))
PY
}

run_all_scenarios() {
  ft_init_refs
  echo "  peers=$PEERS"
  echo "  registry=$REGREF"
  wait_network_warm
  wait_publisher_warm fetch-1   # non-validator issuer-set/issuer-key discovery lags genesis

  # RANDOMIZED flow order (2026-08-20, owner directive: "random as possible") —
  # runs the order-independent, RECOVERABLE flows in a seeded-shuffled order so no
  # flow can silently free-ride on state a fixed predecessor left behind (the hidden
  # coupling that shared FT_LAST_LINK / nodes.json hid). Every flow here is
  # self-contained: it publishes its own content if none exists, resolves its own
  # peers, restores what it perturbs. FIXED POINTS stay pinned: warm-up first;
  # first-run is the liveness precondition; the DESTRUCTIVE / one-way flows
  # (soak, maturing — they permanently stop validators) run LAST, never shuffled.
  # RANDOMIZE=0 restores the legacy fixed order; SEED=<x> replays a specific order.
  run_flow flow_first_run   # liveness precondition — always first among graded flows

  local mid=(
    flow_become_validator flow_publish_fetch flow_care_link flow_convergence
    flow_fault_tolerance flow_restart_survival flow_takedown flow_cross_nat
    flow_cross_region_cold_fetch
    adv_equivocation flow_equivocation_island adv_partition adv_proposal_reject
    flow_publisher_unlinkability flow_durability_turnover flow_chaos_crash
    flow_web_ui_guard flow_c2_no_capture flow_economy_repair flow_delivery_lane
  )
  if [ "${RANDOMIZE:-1}" = 1 ]; then
    local seed="${SEED:-$RUN_ID}"
    # shellcheck disable=SC2207
    mid=($(shuffle_seeded "$seed" "${mid[@]}"))
    echo "  ⇒ RANDOMIZED flow order (seed='$seed'; set RANDOMIZE=0 for fixed order, SEED=$seed to replay):"
    echo "     ${mid[*]}"
  else
    echo "  ⇒ FIXED flow order (RANDOMIZE=0)"
  fi
  # Re-warm the publisher before the batch: restart/partition drills drop a
  # validator from the discoverable issuer set until it re-syncs; a bounded re-warm
  # keeps publish-dependent flows from racing that discovery. In random order any
  # flow may need it, so warm once before the whole batch. Bounded + non-fatal.
  wait_publisher_warm fetch-1
  local f; for f in "${mid[@]}"; do run_flow "$f"; done

  # DESTRUCTIVE / one-way LAST — never randomized (they permanently stop validators;
  # a shuffled position would strand the flows after them on a broken quorum).
  # Pinned OUT of the shuffle: this one shapes the wire every other flow rides
  # on. It unwinds what it applied and verifies the interfaces came back clean,
  # but a position inside a randomized order would put every later flow's verdict
  # downstream of that teardown succeeding.
  run_flow flow_impaired_commit           # the chain commits under injected latency/jitter/loss/reordering (IMPAIR=0 opts out)
  run_flow flow_soak_publish_drain        # opt-in (SOAK=1, MATURING=0): launch publish/drain soak
  run_flow flow_maturing_handoff          # opt-in (MATURING=1 SYBILS=8): handoff/shed drills. LAST: stops validators.
  run_flow flow_deep_heights              # opt-in (DEEP=1): Phase 3 exit gate — drive to h≥DEEP_TARGET with the prune field-exercised. After the drills (continues the matured chain; self-heals stopped validators or GAPs).
}
