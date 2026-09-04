package main

// R2.9a — the B_bootstrap histogram on GET /api/status, under the top-level
// `bBootstrap` key. Counts only: 8 age buckets × 164 quarter-log2 byte bins, plus the
// census total, the clock self-report, the ledger's uptime and both axes. No identity,
// no label, no row, no exact age, and no per-cell byte SUM (immutable #4).

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/nerolabs/silt/core/credit"
	"github.com/nerolabs/silt/ports"
)

// r29aServer is economyServer plus the two /api/status collaborators the SELF-panel
// fixture does not need: peerCount is a nil FUNC FIELD there, so any test that drives
// /api/status off that fixture segfaults before it asserts anything.
func r29aServer(t *testing.T, publish bool) (*uiServer, *credit.Ledger, *r29aClock) {
	t.Helper()
	s, _, _, led := economyServer(t, 0)
	s.peerCount = func() int { return 0 }
	s.bBootstrap = publish
	clk := &r29aClock{}
	led.SetObservabilityClock(clk)
	return s, led, clk
}

// r29aClock is the injected observability clock: a ports.Clock the test moves by hand.
type r29aClock struct{ now ports.Time }

func (c *r29aClock) Now() ports.Time                         { return c.now }
func (c *r29aClock) AfterFunc(ports.Duration, func()) func() { return func() {} }

func getR29aStatusBody(t *testing.T, s *uiServer) string {
	t.Helper()
	r := httptest.NewRequest("GET", "http://127.0.0.1:8080/api/status", nil)
	w := httptest.NewRecorder()
	s.guard(http.HandlerFunc(s.apiStatus)).ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatalf("status: %d, body %s", w.Code, w.Body.String())
	}
	return w.Body.String()
}

// r29aBlock pulls the bBootstrap object out of /api/status as a raw map, so the test
// asserts on the ACTUAL wire key set rather than on a struct that would silently drop
// any key it does not declare.
func r29aBlock(t *testing.T, s *uiServer) (map[string]any, bool, string) {
	t.Helper()
	body := getR29aStatusBody(t, s)
	var top map[string]json.RawMessage
	if err := json.Unmarshal([]byte(body), &top); err != nil {
		t.Fatalf("decode status: %v (body %s)", err, body)
	}
	raw, present := top["bBootstrap"]
	if !present {
		return nil, false, body
	}
	var block map[string]any
	if err := json.Unmarshal(raw, &block); err != nil {
		t.Fatalf("decode bBootstrap: %v (raw %s)", err, raw)
	}
	return block, true, string(raw)
}

func r29aFetch(led *credit.Ledger, i int, bytes int64) {
	led.RecordServe(ports.HashBytes([]byte("srv")), ports.HashBytes([]byte{byte(i), byte(i >> 8), 0x29}), ports.Hash{}, bytes)
}

// --- BB-8: default off -------------------------------------------------------------

// TestR29aStatusOmitsTheBlockUnlessAsked is BB-8. With the switch unset the block is
// ABSENT from /api/status, not present-and-empty. GET /api/status needs no token (reads
// are exempt) and the Host allow-list stops a rebinding browser, not curl, so anything
// published there is world-readable wherever -ui is bound off loopback. Absent and empty
// must be different objects, and off must be the default.
func TestR29aStatusOmitsTheBlockUnlessAsked(t *testing.T) {
	s, led, clk := r29aServer(t, false)
	r29aFetch(led, 1, 4096)
	clk.now = ports.Time(3600 * 1e9)

	if _, present, body := r29aBlock(t, s); present {
		t.Fatalf("bBootstrap present on /api/status with the switch unset — the instrument must be ABSENT by default; body %s", body)
	}
	// Flip it on: now it is present, and present-with-zero-requesters is still a
	// different object from absent.
	s.bBootstrap = true
	block, present, _ := r29aBlock(t, s)
	if !present {
		t.Fatalf("bBootstrap absent with the switch on")
	}
	if got := block["requesters"]; got != float64(1) {
		t.Fatalf("requesters = %v, want 1", got)
	}
}

// TestR29aDaemonDefaultsTheInstrumentOff is the SOURCE GATE behind BB-8: the flag's
// declared default in cmd/silt/daemon.go is false. A runtime test cannot see a flag
// default that is never parsed, so this reads the declaration.
//
// RUNTIME GATE: TestR29aStatusOmitsTheBlockUnlessAsked observes the actual behaviour —
// that an unset switch produces no block on the wire.
func TestR29aDaemonDefaultsTheInstrumentOff(t *testing.T) {
	src, err := os.ReadFile("daemon.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(src), "fs.Bool(\"bbootstrap\", false,") {
		t.Fatalf("SOURCE GATE: daemon.go does not declare the -bbootstrap flag with a false default; the literal fs.Bool(\"bbootstrap\", false, is absent. GET /api/status needs no token, so the instrument must be OFF unless an operator asks for it")
	}
	if !strings.Contains(string(src), "ledger.SetObservabilityClock(clk)") {
		t.Fatalf("SOURCE GATE: daemon.go does not inject the observability clock; the literal ledger.SetObservabilityClock(clk) is absent. Without it the age axis is dead and the export refuses to publish any age-conditioned cell")
	}
}

// --- BB-5: the payload is bounded in R ----------------------------------------------

// TestR29aPayloadIsBoundedInTheRequesterCount is BB-5. The certification words this as
// "byte-identical in length at R = 10 and R = 500,000", which a JSON count histogram
// cannot literally satisfy: the counts are decimal numbers, so 500,000 costs more digits
// than 10. What IS asserted is the property behind it — the STRUCTURE is identical (same
// key set, same 8 × 164 grid, same axis arrays) and the serialized length stays under a
// fixed ceiling however large R gets. The row export's payload grew without bound in R;
// this one does not grow at all except by digits.
func TestR29aPayloadIsBoundedInTheRequesterCount(t *testing.T) {
	shape := func(R int) (keys []string, cells int, rowLens map[int]bool, size int) {
		s, led, clk := r29aServer(t, true)
		for i := 0; i < R; i++ {
			r29aFetch(led, i, int64(1+i))
		}
		clk.now = ports.Time(24 * 3600 * 1e9)
		block, present, raw := r29aBlock(t, s)
		if !present {
			t.Fatalf("bBootstrap absent at R = %d", R)
		}
		if got := int(block["requesters"].(float64)); got != R {
			t.Fatalf("requesters = %d at R = %d — the census must be complete on the wire too", got, R)
		}
		for k := range block {
			keys = append(keys, k)
		}
		rows := block["cells"].([]any)
		cells = len(rows)
		rowLens = map[int]bool{}
		for _, row := range rows {
			rowLens[len(row.([]any))] = true
		}
		return keys, cells, rowLens, len(raw)
	}

	smallKeys, smallCells, smallRows, smallSize := shape(10)
	bigKeys, bigCells, bigRows, bigSize := shape(20_000)

	if len(smallKeys) != len(bigKeys) {
		t.Fatalf("key count changed with R: %d at R=10 vs %d at R=20,000", len(smallKeys), len(bigKeys))
	}
	if smallCells != credit.BBootstrapAgeBuckets || bigCells != credit.BBootstrapAgeBuckets {
		t.Fatalf("cell grid rows = %d / %d, want %d at both R", smallCells, bigCells, credit.BBootstrapAgeBuckets)
	}
	for _, rows := range []map[int]bool{smallRows, bigRows} {
		if len(rows) != 1 || !rows[credit.BBootstrapByteBins] {
			t.Fatalf("cell grid rows are not all %d bins wide: %v", credit.BBootstrapByteBins, rows)
		}
	}
	// The ceiling: 32 KiB covers 1,312 counters, both axes and the metadata with room
	// for every count to be six digits. R = 20,000 is 4.9× the row cap the refuted shape
	// carried, and the payload does not notice.
	const ceiling = 32 << 10
	if bigSize > ceiling {
		t.Fatalf("payload %d bytes at R = 20,000, above the %d-byte ceiling", bigSize, ceiling)
	}
	t.Logf("BB-5: payload %d bytes at R=10 and %d bytes at R=20,000 (%d counters, ceiling %d)",
		smallSize, bigSize, credit.BBootstrapAgeBuckets*credit.BBootstrapByteBins, ceiling)
}

// --- BB-7: no join key ---------------------------------------------------------------

// r29aWireKeys is the CLOSED key set of the published block. Adding a key here is a
// privacy question, not a formatting one: the refuted shape's join key was a per-row
// exact age plus a monotone byte total, which let an observer polling /api/status match
// rows across snapshots and reconstruct "first touch ≈ now − age".
var r29aWireKeys = map[string]bool{
	"clockSource": true, "ageAxisLive": true,
	"requesters": true, "aged": true, "unstamped": true,
	"uptimeNanos": true, "maxOccupiedAgeEdgeNanos": true,
	"clockStepBack": true, "ageExceedsUptime": true,
	"ageEdgeNanos": true, "ageBuckets": true,
	"binsPerOctave": true, "byteBins": true, "byteBinRule": true,
	"cells": true,
}

// TestR29aWirePayloadCarriesNoJoinKey is BB-7. It pins the key set as CLOSED and then
// scans the block's own bytes for anything identity-shaped — a hex node id, or the
// telltale of a per-identity row. It scans ONLY the bBootstrap block, not the whole
// status body, because the rest of /api/status legitimately carries this node's own id.
func TestR29aWirePayloadCarriesNoJoinKey(t *testing.T) {
	s, led, clk := r29aServer(t, true)
	for i := 0; i < 40; i++ {
		r29aFetch(led, i, int64(1<<uint(i%30)))
	}
	clk.now = ports.Time(6 * 3600 * 1e9)

	block, present, raw := r29aBlock(t, s)
	if !present {
		t.Fatalf("bBootstrap absent")
	}
	for k := range block {
		if !r29aWireKeys[k] {
			t.Fatalf("unaudited key %q on the B_bootstrap wire payload: every key here must be an aggregate or an axis descriptor, and a new one is a privacy question", k)
		}
	}
	if len(block) != len(r29aWireKeys) {
		t.Fatalf("the payload has %d keys, the audited set has %d — a key was dropped without updating this gate", len(block), len(r29aWireKeys))
	}
	// Nothing that could be an identity: a NodeID renders as 64 hex characters.
	if m := regexp.MustCompile(`[0-9a-f]{32,}`).FindString(raw); m != "" {
		t.Fatalf("the payload contains a long hex string %q — an identity, salted label or root has reached the wire", m)
	}
	// And no per-cell byte SUM: a cell sum with count 1 is that identity's exact byte
	// total in disguise, so the only numbers in the grid are counts.
	for _, banned := range []string{"bytesTotal", "bytesSum", "sumBytes", "fetchedBytes"} {
		if strings.Contains(raw, banned) {
			t.Fatalf("the payload contains %q — the grid is COUNTS only; a per-cell byte sum re-identifies a singleton cell", banned)
		}
	}
	// Sanity: the census total on the wire matches the fixture, so this gate is
	// scanning a populated payload rather than an empty one.
	if got := block["requesters"]; got != float64(40) {
		t.Fatalf("requesters = %v, want 40 — the privacy scan must run against a populated payload", got)
	}
}

// TestR29aWirePayloadSelfReportsADeadClock is the wire half of BB-1 / G-BB-2: with no
// clock injected the block still publishes (so a reader can tell the instrument is ON),
// says so in clockSource, and carries a NULL cell grid — never an all-zero age column.
func TestR29aWirePayloadSelfReportsADeadClock(t *testing.T) {
	s, _, _, led := economyServer(t, 0)
	s.peerCount = func() int { return 0 }
	s.bBootstrap = true // switched ON, but no clock is injected
	r29aFetch(led, 1, 4096)

	block, present, raw := r29aBlock(t, s)
	if !present {
		t.Fatalf("bBootstrap absent with the switch on")
	}
	if block["clockSource"] != "none" || block["ageAxisLive"] != false {
		t.Fatalf("clockSource = %v, ageAxisLive = %v with no clock injected, want \"none\"/false", block["clockSource"], block["ageAxisLive"])
	}
	if block["cells"] != nil {
		t.Fatalf("cells published with no clock injected: %s — an all-zero grid is indistinguishable from a genuinely young population", raw)
	}
	if block["requesters"] != float64(1) {
		t.Fatalf("requesters = %v, want 1 — a dead clock must not hide the census, or 'off' and 'idle' fuse", block["requesters"])
	}
}
