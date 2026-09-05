package e2e

// The LOCAL proof of the full S7 economy loop — the Phase 2 Slice 4 integration
// that must be green before the billable ECONOMY=1 cloud run confirms it
// (build-immutable #7; the cloudtest preflight runs this as RUN_LOCAL_PROOF):
//
//   publish erasure-coded → `swarm holders` → kill 3 columns' holders →
//   a caretaker RECONSTRUCTS from parity → a caretaker-JUDGE verifies both legs
//   and the bounty PAYS the new holder from the object's escrow (paid > 0) →
//   the file still fetches bit-perfect.
//
// The pieces pass alone (e2e/economy_test.go: flags+fund+telemetry;
// e2e/holders_test.go: placement observability; sim/repair_bounty_test.go:
// claim/judge/payout under the scheduler); THIS test is the composition on real
// daemons over real TCP. Two caretakers are structural, not decoration: the
// paramedic never judges its own claim (repairclaim.go — emitRepairClaim skips
// itself and the holder), credit is per-node-local, so `paid` materializes on
// the OTHER caretaker's ledger — the judge's — and only if that judge's own
// escrow was funded. Design: docs/thinking/2026-08-20-economy-local-loop-design.md.

import (
	"bytes"
	"fmt"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"
)

// econChunk mirrors the cloud economy geometry (§0.1: the economy grade runs
// 256 KiB chunks so a repair fits the hobbyist box). k=10 data columns per
// stripe (erasure.DefaultParams), so a k·chunk file is exactly one stripe:
// 10 data + 6 parity = 16 columns, n−k = 6 the collateral budget.
const (
	econChunk   = 262144
	econK       = 10
	econParity  = 6
	econColumns = econK + econParity
)

var reCareLink = regexp.MustCompile(`siltcare:\S+`)
var reCaretaking = regexp.MustCompile(`caretaking ([0-9a-f]{64})`)
var reColumnHolders = regexp.MustCompile(`^column (\d+): (\S+)$`)

// sameHolderMap reports whether two byte-confirmed holder views (col → NodeID-hex
// set) are identical — the convergence predicate for #514's STABILIZE step.
func sameHolderMap(a, b map[int][]string) bool {
	if len(a) != len(b) {
		return false
	}
	for col, ah := range a {
		bh, ok := b[col]
		if !ok || len(ah) != len(bh) {
			return false
		}
		set := map[string]bool{}
		for _, id := range ah {
			set[id] = true
		}
		for _, id := range bh {
			if !set[id] {
				return false
			}
		}
	}
	return true
}

// TestCareWithoutRegistryRefusesToStart pins the guard for the silent-caretaker
// shape (V5, found while building the wire proof): -care with no -registry and no
// -serve-registry used to come up looking healthy while the care loop silently
// never started (reg == nil skips it) — the cloud economy scenario armed exactly
// that no-op caretaker. The daemon must refuse the config instead.
func TestCareWithoutRegistryRefusesToStart(t *testing.T) {
	if testing.Short() {
		t.Skip("e2e spawns processes; skipped under -short")
	}
	d := startDaemon(t, "careless",
		"-listen", "127.0.0.1:0", "-store", t.TempDir(),
		"-care", "siltcare:v1:deadbeef",
		"-capacity", "1G", "-mdns=false", "-id-seed", "9301")
	d.waitFor(t, regexp.MustCompile(`-care needs a registry`), 20*time.Second)
}

func TestRepairBountyPaysOnTheWire(t *testing.T) {
	if testing.Short() {
		t.Skip("e2e spawns processes; skipped under -short")
	}

	// A: lone trusted validator + registry (-quorum 0), same shape as
	// TestPublishCommitFetchOverTCP — consensus-on-the-wire has its own e2e
	// tests; the seam under test here is the repair economy.
	a := startDaemon(t, "A",
		"-listen", "127.0.0.1:0", "-store", t.TempDir(),
		"-serve-registry", "127.0.0.1:0",
		"-validator", "-quorum", "0", "-min-rep", "0",
		"-capacity", "1G", "-mdns=false", "-id-seed", "9001")
	peer := a.waitFor(t, rePeer, 20*time.Second)
	idA, addrA := peer[1], peer[2]
	regRef := a.waitFor(t, reRegistry, 20*time.Second)[1]
	bootstrapA := idA + "@" + addrA

	// 12 storage nodes: the killable pool. With -replication 1 each coded
	// column lands on one holder, so killing 3 columns kills ~3 daemons and
	// the stripe stays comfortably above k reachable.
	storage := map[string]*daemon{} // NodeID hex → daemon
	for i := 0; i < 12; i++ {
		seed := int64(9101 + i)
		d := startDaemon(t, fmt.Sprintf("S%d", i+1),
			"-listen", "127.0.0.1:0", "-store", t.TempDir(),
			"-bootstrap", bootstrapA,
			"-capacity", "1G", "-mdns=false", "-id-seed", fmt.Sprint(seed))
		p := d.waitFor(t, rePeer, 20*time.Second)
		d.waitFor(t, reBootstrap, 20*time.Second)
		storage[p[1]] = d
	}

	// resolveHolders runs `swarm holders` for a link and parses the byte-confirmed
	// (MsgHasChunk, #514) per-column holder view: col → NodeID-hex list. This is the
	// SAME view the caretaker repairs on (probeShard also byte-confirms), so a listed
	// holder provably holds one of the column's shards. A column absent has none.
	resolveHolders := func(link string) map[int][]string {
		out := runClient(t, "swarm", "holders", link, "-peers", bootstrapA, "-registry", regRef)
		m := map[int][]string{}
		for _, ln := range strings.Split(out, "\n") {
			g := reColumnHolders.FindStringSubmatch(strings.TrimSpace(ln))
			if g == nil {
				continue
			}
			var col int
			fmt.Sscanf(g[1], "%d", &col)
			var live []string
			for _, id := range strings.Split(g[2], ",") {
				if id != "" {
					live = append(live, id)
				}
			}
			if len(live) > 0 {
				m[col] = live
			}
		}
		return m
	}
	// canForcePremise reports whether the byte-confirmed placement admits a kill set
	// that loses between 3 (> RepairSlack) and n−k (recoverable) columns. Placement
	// occasionally concentrates all columns onto 2-3 nodes (a DHT convergence-timing
	// artifact of colKey closest-selection with -replication 1): then every kill
	// loses either <3 or >n−k columns, and no over-slack-but-recoverable loss can be
	// forced. The cloud grade records this as "economy UNTESTED, not failed"
	// (scenarios.sh:2095); here we simply re-publish under a fresh root, which
	// re-rolls the placement. Mirrors the eventual targets selection below.
	canForcePremise := func(holders map[int][]string) bool {
		var cand []int
		for col, ids := range holders {
			if len(ids) == 0 {
				continue
			}
			ok := true
			for _, id := range ids {
				if _, isStore := storage[id]; !isStore {
					ok = false
					break
				}
			}
			if ok {
				cand = append(cand, col)
			}
		}
		lost := func(kill map[string]bool) int {
			n := 0
			for _, ids := range holders {
				if len(ids) == 0 {
					continue
				}
				gone := true
				for _, id := range ids {
					if !kill[id] {
						gone = false
						break
					}
				}
				if gone {
					n++
				}
			}
			return n
		}
		for i := 0; i < len(cand); i++ {
			for j := i + 1; j < len(cand); j++ {
				for k := j + 1; k < len(cand); k++ {
					kill := map[string]bool{}
					for _, col := range []int{cand[i], cand[j], cand[k]} {
						for _, id := range holders[col] {
							kill[id] = true
						}
					}
					if l := lost(kill); l >= 3 && l <= econParity {
						return true
					}
				}
			}
		}
		return false
	}

	// Publish, retrying under a FRESH root if placement concentrates too much to
	// force an over-slack-but-recoverable loss. A fresh random payload changes the
	// root, so colKey(root,col) picks different closest nodes — re-rolling placement.
	// The care link (repair rights, no decryption) prints on stderr; take combined.
	src := filepath.Join(t.TempDir(), "payload.bin")
	want := make([]byte, econK*econChunk)
	var link, careLink string
	const maxPublishTries = 6
	for try := 0; try < maxPublishTries; try++ {
		rand.New(rand.NewSource(0x57EC0 + int64(try))).Read(want)
		if err := os.WriteFile(src, want, 0o644); err != nil {
			t.Fatal(err)
		}
		out, err := runClientAllowErr(t, "swarm", "add", src,
			"-peers", bootstrapA, "-registry", regRef,
			"-chunk-size", fmt.Sprint(econChunk), "-replication", "1")
		if err != nil {
			t.Fatalf("publish failed: %v\n%s", err, out)
		}
		link = regexp.MustCompile(`silt:v1:\S+`).FindString(out)
		careLink = reCareLink.FindString(out)
		if link == "" || careLink == "" {
			t.Fatalf("publish printed no link+carelink:\n%s", out)
		}
		// STABILIZE this publish's placement (two consecutive reads agree), then
		// decide if it can force the premise. Stabilizing here makes the decision
		// authoritative: if it passes, the post-caretaker SELECT step sees the same
		// converged placement and will not fail. If it concentrates, re-publish under
		// a fresh root. Bounded — a publish that never stabilizes is itself re-rolled.
		var pv map[int][]string
		for d := time.Now().Add(40 * time.Second); time.Now().Before(d); {
			time.Sleep(3 * time.Second)
			nv := resolveHolders(link)
			if pv != nil && sameHolderMap(pv, nv) {
				pv = nv
				break
			}
			pv = nv
		}
		if pv != nil && canForcePremise(pv) {
			if try > 0 {
				t.Logf("publish attempt %d: placement admits an over-slack-but-recoverable kill set", try)
			}
			break
		}
		if try == maxPublishTries-1 {
			t.Fatalf("premise setup (#514): placement stayed too concentrated across %d publishes — "+
				"no over-slack-but-recoverable kill set (all columns cluster onto 2-3 nodes):\n%v",
				maxPublishTries, pv)
		}
		t.Logf("publish attempt %d: placement too concentrated to force the premise, re-publishing under a fresh root", try)
	}

	// Two caretakers, started now that the care link exists — the operator
	// shape ("I was handed a care link"). Both run -economy (a judge with the
	// economy off never disburses) and a fast sweep so the proof runs in
	// seconds, not 60 s ticks.
	type caretaker struct {
		d           *daemon
		store       string
		base, token string
	}
	var caretakers []caretaker
	for i, seed := range []string{"9201", "9202"} {
		store := t.TempDir()
		d := startDaemon(t, fmt.Sprintf("C%d", i+1),
			"-listen", "127.0.0.1:0", "-store", store,
			"-bootstrap", bootstrapA, "-registry", regRef,
			"-care", careLink, "-economy", "-repair-interval", "2s",
			"-ui", "127.0.0.1:0", "-log", "info",
			"-capacity", "1G", "-mdns=false", "-id-seed", seed)
		ui := d.waitFor(t, reUI, 20*time.Second)
		d.waitFor(t, reCaretaking, 20*time.Second)
		caretakers = append(caretakers, caretaker{d: d, store: store, base: "http://" + ui[1], token: ui[2]})
	}
	// The repair/claim/judge narration lands in <store>/debug.log (-log info);
	// surface it when the loop fails so the break names itself (#7). The tail
	// alone is NOT enough: at the 2s sweep cadence the last 80 lines are all
	// sweep chatter, and the one-shot claim-chain events (pending confirmation,
	// stripe repaired, no-eligible-judge, bounty released) scroll out — a CI
	// failure of the #518 judge-starvation mode was unattributable from the
	// tail. So dump the tail PLUS every claim-chain line from the whole file.
	debugTail := func(c caretaker) string {
		b, err := os.ReadFile(filepath.Join(c.store, "debug.log"))
		if err != nil {
			return "(no debug.log: " + err.Error() + ")"
		}
		lines := strings.Split(strings.TrimSpace(string(b)), "\n")
		var chain []string
		for _, ln := range lines {
			if strings.Contains(ln, "pending confirmation") || strings.Contains(ln, "stripe repaired") ||
				strings.Contains(ln, "repair below k") || strings.Contains(ln, "claim") ||
				strings.Contains(ln, "bounty") || strings.Contains(ln, "reconciled") ||
				strings.Contains(ln, "repair sweep complete") || strings.Contains(ln, "stripe degraded") ||
				strings.Contains(ln, "manifest not yet") || strings.Contains(ln, "repair sweep waiting") ||
				strings.Contains(ln, "repair sweep skipped") {
				chain = append(chain, ln)
			}
		}
		// Keep the last 30 chain lines: sweep-complete narration is emitted every
		// ~2s, so on a long run the early ones scroll past usefulness, and the dial
		// spam otherwise buries the sweep/repair story in the raw tail (#514: a
		// premise-vs-caretaker divergence was unattributable from the tail alone).
		if len(chain) > 30 {
			chain = chain[len(chain)-30:]
		}
		tail := lines
		if len(tail) > 40 {
			tail = tail[len(tail)-40:]
		}
		return "-- repair/sweep narration (last 30) --\n" + strings.Join(chain, "\n") +
			"\n-- tail --\n" + strings.Join(tail, "\n")
	}

	// Fund BOTH caretakers' escrows from their own starter grants: credit is
	// per-node-local and which one ends up the judge is timing, so the payer's
	// ledger must hold a funded reserve whichever way the race goes. The
	// amount must fit the 500k starter grant (FundEscrow refuses more);
	// PayBounty pays min(bounty, reserve), so a partial reserve still proves
	// paid > 0 — that partial payment IS the finite-but-renewable horizon.
	const endow = 400_000
	var root string
	for _, c := range caretakers {
		for deadline := time.Now().Add(20 * time.Second); ; {
			s := getStatus(t, c.base, c.token)
			if s.Durability != nil && len(s.Durability.Objects) > 0 {
				root = s.Durability.Objects[0].Root
				break
			}
			if time.Now().After(deadline) {
				t.Fatalf("cared object never appeared in %s durability telemetry", c.d.name)
			}
			time.Sleep(500 * time.Millisecond)
		}
		status, body, err := apiFund(c.base, c.token, link, endow)
		if err != nil || status != http.StatusOK {
			t.Fatalf("fund on %s: status=%d err=%v body=%s", c.d.name, status, err, body)
		}
	}

	// resolveColHolders reads the byte-confirmed holders view for the cared link.
	resolveColHolders := func() map[int][]string { return resolveHolders(link) }

	// The deterministic premise (#514, superseding the pure-filter #607). The kill
	// only proves the economy loop if a CARETAKER actually observes a stripe over
	// RepairSlack (2) and repairs. The real root cause — pinned by the caretaker's
	// own sweep trace (docs/thinking/2026-08-27-514-deterministic-premise-and-
	// holders-liveness.md) — is that the test used to kill BEFORE DHT convergence
	// completed. The object carries publish-time lost-ack extra copies (#497:
	// -replication 1 does NOT mean one holder — a lost ack mints a silent extra
	// copy) whose provider records converge a sweep or two AFTER the kill. So the
	// caretaker's first post-kill sweep saw the loss over slack, but reachable then
	// climbed as the hidden copies surfaced, the loss healed within slack, and the
	// #517 two-sweep confirmation gate reset — the ~20% flake. #607's selector
	// byte-confirm could not see the hidden copies either, so it did not close it.
	//
	// The robust close, three steps, all harness/observability:
	//   1. STABILIZE: wait until the byte-confirmed holders view stops changing
	//      across reads — convergence is complete and every real byte-holder
	//      (including the lost-ack copies) is now listed.
	//   2. KILL ALL: kill every byte-holder of the target columns. With a stable
	//      view no hidden copy survives; re-read once and kill any straggler.
	//   3. CONFIRM: wait for a caretaker's OWN sweep to narrate a stripe over slack,
	//      over a window covering the cold manifest heal AND the two-sweep gate.
	//      Because the columns are now genuinely byte-gone, the loss does not heal,
	//      so both sweeps agree and the gate fires — deterministically.
	const slack = 2 // RepairSlack default

	killed := map[string]bool{}
	holders := resolveColHolders()
	// killableHolders: a column's byte-holders that are still-alive storage daemons.
	killableHolders := func(cols map[int][]string, col int) []string {
		var out []string
		for _, id := range cols[col] {
			if _, ok := storage[id]; ok && !killed[id] {
				out = append(out, id)
			}
		}
		return out
	}
	// allKillable: every byte-holder of the column is a live storage daemon (never
	// the validator or a caretaker), so killing them removes the whole column.
	allKillable := func(cols map[int][]string, col int) bool {
		if len(cols[col]) == 0 {
			return false
		}
		for _, id := range cols[col] {
			if _, ok := storage[id]; !ok || killed[id] {
				return false
			}
		}
		return true
	}

	// STEP 1 — STABILIZE. Read the byte-confirmed view until two consecutive reads
	// agree (convergence complete). Bounded; fail loud if it never settles.
	stable := false
	for deadline := time.Now().Add(60 * time.Second); time.Now().Before(deadline); {
		time.Sleep(3 * time.Second)
		next := resolveColHolders()
		if sameHolderMap(holders, next) {
			holders = next
			stable = true
			break
		}
		holders = next
	}
	if !stable {
		t.Fatalf("premise setup (#514): the byte-confirmed holders view never stabilized within 60s — "+
			"DHT convergence did not settle, so a kill cannot be proven complete:\n%v", holders)
	}
	if len(holders) < econColumns-econParity {
		t.Fatalf("swarm holders resolved only %d coded columns with live byte-holders, want ≥ %d (placement or byte-confirm broke)",
			len(holders), econColumns-econParity)
	}

	// Pick 3 target columns to kill, BOUNDING the collateral. Killing a storage node
	// removes EVERY column it holds, and with 12 nodes for 16 columns some nodes hold
	// two columns — so the union of 3 target columns' holders can take out more than 3
	// columns. The loss must exceed RepairSlack (2 → repair MUST fire) but stay ≤ n−k
	// (6 → a stripe keeps ≥ k=10 shards, so reconstruction is possible and the bounty
	// can pay). Killing past n−k drops the stripe below k ("repair below k — data
	// unrecoverable"), no repair ever succeeds, and no bounty pays — the destroy-the-
	// object failure. So enumerate candidate columns (all-killable holders) and choose
	// the 3-combo whose union loses the FEWEST columns in [3, n−k]; every extra lost
	// column eats survivor margin.
	var candidates []int
	for col := range holders {
		if allKillable(holders, col) {
			candidates = append(candidates, col)
		}
	}
	sort.Ints(candidates)
	if len(candidates) < 3 {
		t.Fatalf("premise setup (#514): only %d columns have all-killable byte-holders after convergence "+
			"(placement concentrated onto the validator/caretakers):\n%v", len(candidates), holders)
	}
	// columnsLost counts columns whose EVERY byte-holder is in the kill set of nodes.
	columnsLost := func(kill map[string]bool) int {
		lost := 0
		for _, ids := range holders {
			if len(ids) == 0 {
				continue
			}
			gone := true
			for _, id := range ids {
				if !kill[id] {
					gone = false
					break
				}
			}
			if gone {
				lost++
			}
		}
		return lost
	}
	var targets []int
	bestLost := econParity + 1 // n−k = 6; require lost ≤ this
	for i := 0; i < len(candidates); i++ {
		for j := i + 1; j < len(candidates); j++ {
			for k := j + 1; k < len(candidates); k++ {
				trial := map[string]bool{}
				for _, col := range []int{candidates[i], candidates[j], candidates[k]} {
					for _, id := range holders[col] {
						trial[id] = true
					}
				}
				if l := columnsLost(trial); l >= 3 && l < bestLost {
					targets = []int{candidates[i], candidates[j], candidates[k]}
					bestLost = l
				}
			}
		}
	}
	if len(targets) < 3 || bestLost > econParity {
		t.Fatalf("premise setup (#514): no 3-column combination loses between 3 and %d columns — "+
			"placement too concentrated (killing 3 columns' holders would drop a stripe below k):\n%v", econParity, holders)
	}
	t.Logf("target columns %v chosen: killing their holders loses %d columns (slack 2 < %d ≤ n−k %d)",
		targets, bestLost, bestLost, econParity)

	// STEP 2 — KILL ALL byte-holders of the target columns, then re-read once and
	// kill any straggler that converged between the stabilize check and the kill.
	killedAt := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	for pass := 0; pass < 3; pass++ {
		killedThisPass := 0
		for _, col := range targets {
			for _, id := range killableHolders(holders, col) {
				killed[id] = true
				killedThisPass++
				t.Logf("pass %d: killing %s (byte-holder of target column %d)", pass, storage[id].name, col)
				storage[id].cmd.Process.Kill()
			}
		}
		if pass > 0 && killedThisPass == 0 {
			break // no straggler surfaced: the target columns are byte-gone
		}
		time.Sleep(3 * time.Second) // let any straggler's record converge into view
		holders = resolveColHolders()
	}
	// After the kill, the target columns must show ZERO killable live holders. Any
	// remaining holder is unkillable (a caretaker/validator legitimately hosting a
	// converged copy) — rare, and it means this premise cannot be forced; fail loud.
	stillHeld := 0
	for _, col := range targets {
		if len(killableHolders(holders, col)) > 0 {
			stillHeld++
		}
	}
	if stillHeld > 0 {
		t.Fatalf("premise setup (#514): %d of %d target columns still have a killable holder after 3 kill passes — "+
			"copies keep converging (harness bug):\n%v", stillHeld, len(targets), holders)
	}
	// A straggler kill can cascade (a killed node may hold a non-target column too),
	// so re-verify the loss stayed within n−k: a stripe below k is unrecoverable and
	// no bounty can ever pay. If a cascade over-killed, fail as a harness bug rather
	// than time out on a destroyed object.
	if lost := columnsLost(killed); lost > econParity {
		t.Fatalf("premise setup (#514): kill cascaded to %d lost columns (> n−k %d) — a stripe is below k and "+
			"unrecoverable, no bounty can pay. A straggler kill hit a multi-column node:\n%v", lost, econParity, holders)
	}

	// STEP 3 — CONFIRM on the caretaker's OWN sweep, RE-KILLING surfaced copies. Wait
	// for a stripe over slack: a pending-confirmation (missing > slack this sweep) or
	// a successful repair. "repair below k" is NOT accepted — it means the stripe
	// dropped below k (unrecoverable), which the bounded selection prevents.
	//
	// The caretaker is a DIFFERENT node with its OWN DHT vantage, so even after the
	// selector's stable view shows the columns byte-gone, the caretaker can still
	// resolve a target column's holder the selector could not (a lost-ack copy that
	// converged to the caretaker but not the selector's ephemeral client, or a copy
	// that re-surfaces via reprovide). That was the residual ~2% divergence. So this
	// is a confirm-OR-re-kill loop: each round, if the caretaker has not yet narrated
	// an over-slack loss, re-read the selector view and kill any SURFACED killable
	// holder of the target columns — staying within the n−k bound — then wait again.
	// Each re-kill removes strictly more real bytes of the SAME target columns, so it
	// never widens the loss past the columns already chosen (the bound holds).
	reOverSlack := regexp.MustCompile(`stripe repair pending confirmation.*missing=(\d+)`)
	caretakerOverSlack := func() bool {
		for _, c := range caretakers {
			b, err := os.ReadFile(filepath.Join(c.store, "debug.log"))
			if err != nil {
				continue
			}
			for _, ln := range strings.Split(string(b), "\n") {
				if len(ln) < 24 || ln[:24] < killedAt {
					continue
				}
				if strings.Contains(ln, "stripe repaired") || reOverSlack.MatchString(ln) {
					return true
				}
			}
		}
		return false
	}
	premiseEstablished := false
	const confirmRounds = 5
	for round := 0; round < confirmRounds && !premiseEstablished; round++ {
		// Watch the caretaker ground truth for ~2 sweeps + heal margin.
		for deadline := time.Now().Add(30 * time.Second); time.Now().Before(deadline); {
			if caretakerOverSlack() {
				premiseEstablished = true
				break
			}
			time.Sleep(1 * time.Second)
		}
		if premiseEstablished {
			break
		}
		// The caretaker still sees the target columns as reachable — a copy surfaced
		// to its vantage that the selector's kill missed. Re-read and kill any
		// surfaced killable holder of the target columns.
		holders = resolveColHolders()
		surfaced := 0
		for _, col := range targets {
			for _, id := range killableHolders(holders, col) {
				killed[id] = true
				surfaced++
				t.Logf("confirm round %d: re-killing surfaced %s (byte-holder of target column %d)", round, storage[id].name, col)
				storage[id].cmd.Process.Kill()
			}
		}
		// A surfaced holder may sit on a multi-column node — re-check the n−k bound.
		if lost := columnsLost(killed); lost > econParity {
			t.Fatalf("premise setup (#514): re-killing a surfaced holder cascaded to %d lost columns (> n−k %d) — "+
				"stripe below k, unrecoverable:\n%v", lost, econParity, holders)
		}
		if surfaced == 0 {
			t.Logf("confirm round %d: no surfaced holder to re-kill; waiting on the caretaker sweep", round)
		}
	}
	if !premiseEstablished {
		t.Fatalf("premise unestablishable (#514): the byte-confirmed view proved %d target columns byte-gone and re-killed "+
			"every surfaced copy across %d rounds, yet no caretaker's own sweep saw a stripe over slack (%d) — the caretaker "+
			"cannot observe a loss the selector proved (byte-confirm or corpse-gating regression).\nC1:\n%s\nC2:\n%s",
			len(targets), confirmRounds, slack, debugTail(caretakers[0]), debugTail(caretakers[1]))
	}
	t.Logf("premise established: %d target columns byte-gone (stable view) and a caretaker's own sweep saw a stripe over slack; %d holders killed total",
		len(targets), len(killed))

	// The exit-gate signal: a verified reconstruction PAID. The premise is now
	// established from the caretaker's OWN ground truth (a stripe over slack), so a
	// missing pay here is a break in the repair→claim→judge→payout loop, not a
	// premise defeat. Poll both caretakers — the paramedic emits the claim, the
	// OTHER one judges and pays on its own ledger.
	//
	// The pay window must cover the MEASURED repair cycle, not an optimistic
	// guess. History: the unbounded sweep under dead holders ran ~3-4 min
	// (#501), the old 180s budget sat inside that band (5 identical ~181.8s CI
	// failures on 2026-08-21), and PR #511 widened the window to the certified
	// 600s premise pending the mechanism fix. #501 is now FIXED (sweep-scoped
	// corpse gating + decaying cooldown bound the sweep to ≤1 discovery ladder
	// per corpse per tick): the whole test measures well under this locally, so
	// 180s holds with margin — and this deadline is the #501 regression signal: a
	// failure here means the sweep bound broke, not calibration.
	var paid, funded, repairs int64
	deadline := time.Now().Add(180 * time.Second)
	for paid == 0 && time.Now().Before(deadline) {
		for _, c := range caretakers {
			s := getStatus(t, c.base, c.token)
			if s.Durability == nil {
				continue
			}
			for _, o := range s.Durability.Objects {
				if o.Root == root && o.Paid > paid {
					paid, funded, repairs = o.Paid, o.Funded, o.Repairs
				}
			}
		}
		if paid == 0 {
			time.Sleep(2 * time.Second)
		}
	}
	if paid == 0 {
		t.Fatalf("no bounty paid within the window — the loop broke between repair, claim, judge, and payout.\n--- C1 debug.log ---\n%s\n--- C2 debug.log ---\n%s",
			debugTail(caretakers[0]), debugTail(caretakers[1]))
	}
	if repairs < 1 || paid > funded {
		t.Fatalf("escrow accounting dishonest: paid=%d funded=%d repairs=%d", paid, funded, repairs)
	}
	t.Logf("bounty paid on the wire: paid=%d over %d repair(s), funded=%d", paid, repairs, funded)

	// The S2 half: the bounty paid for a reconstruction that actually restored
	// availability — the file still round-trips bit-perfect with 3 columns'
	// original holders dead.
	dst := filepath.Join(t.TempDir(), "fetched.bin")
	runClient(t, "swarm", "get", link, "-o", dst, "-peers", bootstrapA, "-registry", regRef)
	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("post-repair fetch corrupted: got %d bytes, want %d", len(got), len(want))
	}
}
