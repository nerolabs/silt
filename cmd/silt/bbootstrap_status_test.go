package main

// R2.9a — the B_bootstrap export on GET /api/status, under economy.bBootstrap.
// The published payload is (age, bytes) pairs plus the epoch, the requester count
// and a truncation flag. No identity, no root, no sub-epoch time (immutable #4).

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/nerolabs/silt/core/credit"
	"github.com/nerolabs/silt/ports"
)

type r29aStatus struct {
	Economy struct {
		BBootstrap struct {
			Epoch      uint64 `json:"epoch"`
			Requesters int    `json:"requesters"`
			Truncated  bool   `json:"truncated"`
			Series     []struct {
				AgeEpochs    uint64 `json:"ageEpochs"`
				FetchedBytes int64  `json:"fetchedBytes"`
			} `json:"series"`
		} `json:"bBootstrap"`
	} `json:"economy"`
}

// r29aServer is economyServer plus the two /api/status collaborators the SELF-panel
// fixture does not need (peerCount is a func field, nil there).
// r29aEpoch is a fixed ports.EpochSource for the status fixtures (R2.10).
type r29aEpoch uint64

func (e r29aEpoch) Epoch() uint64 { return uint64(e) }

func r29aServer(t *testing.T) (*uiServer, *credit.Ledger) {
	t.Helper()
	s, _, _, led := economyServer(t, 0)
	s.peerCount = func() int { return 0 }
	return s, led
}

func getR29aStatus(t *testing.T, s *uiServer) (r29aStatus, string) {
	t.Helper()
	r := httptest.NewRequest("GET", "http://127.0.0.1:8080/api/status", nil)
	w := httptest.NewRecorder()
	s.guard(http.HandlerFunc(s.apiStatus)).ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatalf("status: %d, body %s", w.Code, w.Body.String())
	}
	var out r29aStatus
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode status: %v (body %s)", err, w.Body.String())
	}
	return out, w.Body.String()
}

// TestR29aStatusPublishesTheAgeVsBytesSeries: the shape an operator (and the flixz
// handoff) reads — and the shape it must NOT contain.
func TestR29aStatusPublishesTheAgeVsBytesSeries(t *testing.T) {
	s, led := r29aServer(t)
	server := ports.HashBytes([]byte("srv"))
	old := ports.HashBytes([]byte("old-fetcher"))
	young := ports.HashBytes([]byte("young-fetcher"))

	led.RecordServe(server, old, ports.HashBytes([]byte("c1")), 4_000)
	// R2.10: the ledger's epoch is an injected source, not a call argument.
	led.SetEpochSource(r29aEpoch(5))
	led.RedeemDeliveryCredit(server, ports.HashBytes([]byte("x")), ports.HashBytes([]byte("t")), []byte("tick-5"), 5)
	led.RecordServe(server, young, ports.HashBytes([]byte("c2")), 900)

	out, body := getR29aStatus(t, s)
	bb := out.Economy.BBootstrap
	if bb.Epoch != 5 {
		t.Fatalf("epoch = %d, want 5", bb.Epoch)
	}
	if bb.Requesters != 2 || bb.Truncated {
		t.Fatalf("requesters = %d truncated = %v, want 2 false", bb.Requesters, bb.Truncated)
	}
	if len(bb.Series) != 2 {
		t.Fatalf("series = %+v, want 2 rows", bb.Series)
	}
	if bb.Series[0].AgeEpochs != 0 || bb.Series[0].FetchedBytes != 900 {
		t.Fatalf("row 0 = %+v, want {0, 900} (youngest first)", bb.Series[0])
	}
	if bb.Series[1].AgeEpochs != 5 || bb.Series[1].FetchedBytes != 4_000 {
		t.Fatalf("row 1 = %+v, want {5, 4000}", bb.Series[1])
	}
	// The privacy floor, asserted on the WIRE. The published block's key set is
	// closed, each row is exactly (ageEpochs, fetchedBytes), and no requester id —
	// raw or salted — nor any object root appears anywhere in it.
	blob := bBootstrapBlob(t, body)
	for _, banned := range []string{old.String(), young.String(), "salted", "root", "chunk", "object", "time"} {
		if strings.Contains(strings.ToLower(blob), strings.ToLower(banned)) {
			t.Fatalf("economy.bBootstrap contains %q — the series must carry age and bytes only:\n%s", banned, blob)
		}
	}
}

// bBootstrapBlob re-serialises just the economy.bBootstrap object and pins its key
// set, so the privacy scan below cannot be fooled by an added field and cannot trip
// over an unrelated part of /api/status.
func bBootstrapBlob(t *testing.T, body string) string {
	t.Helper()
	var whole map[string]any
	if err := json.Unmarshal([]byte(body), &whole); err != nil {
		t.Fatal(err)
	}
	econ, ok := whole["economy"].(map[string]any)
	if !ok {
		t.Fatalf("no economy block in /api/status: %s", body)
	}
	bb, ok := econ["bBootstrap"].(map[string]any)
	if !ok {
		t.Fatalf("no economy.bBootstrap block: %s", body)
	}
	want := map[string]bool{"epoch": true, "requesters": true, "truncated": true, "series": true}
	for k := range bb {
		if !want[k] {
			t.Fatalf("economy.bBootstrap carries an unexpected field %q — the published shape is closed", k)
		}
	}
	rows, _ := bb["series"].([]any)
	for _, r := range rows {
		row, ok := r.(map[string]any)
		if !ok {
			t.Fatalf("series row is not an object: %v", r)
		}
		for k := range row {
			if k != "ageEpochs" && k != "fetchedBytes" {
				t.Fatalf("series row carries %q — a row is (ageEpochs, fetchedBytes) ONLY", k)
			}
		}
	}
	out, err := json.Marshal(bb)
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}

// TestR29aStatusCapsTheSeriesAndFlagsTruncation: the published series is bounded
// (build-immutable #8) and says so when it dropped a tail.
func TestR29aStatusCapsTheSeriesAndFlagsTruncation(t *testing.T) {
	s, led := r29aServer(t)
	server := ports.HashBytes([]byte("srv"))
	const n = credit.MaxRequesterFetchRows + 5
	for i := 0; i < n; i++ {
		led.RecordServe(server, ports.HashBytes([]byte(fmt.Sprintf("f-%d", i))), ports.HashBytes([]byte("c")), int64(i+1))
	}
	out, _ := getR29aStatus(t, s)
	bb := out.Economy.BBootstrap
	if len(bb.Series) != credit.MaxRequesterFetchRows {
		t.Fatalf("series rows = %d, want the cap %d", len(bb.Series), credit.MaxRequesterFetchRows)
	}
	if bb.Requesters != n {
		t.Fatalf("requesters = %d, want %d (the true total)", bb.Requesters, n)
	}
	if !bb.Truncated {
		t.Fatal("truncated = false while the series dropped a tail")
	}
}

// TestR29aStatusEmitsAnEmptySeriesNotNull: a node nobody has fetched from still
// publishes the block (a counter, not a payout — economy-off nodes emit it too),
// with an empty array rather than a null a consumer has to special-case.
func TestR29aStatusEmitsAnEmptySeriesNotNull(t *testing.T) {
	s, _ := r29aServer(t)
	out, body := getR29aStatus(t, s)
	if out.Economy.BBootstrap.Requesters != 0 || len(out.Economy.BBootstrap.Series) != 0 {
		t.Fatalf("fresh node series = %+v, want empty", out.Economy.BBootstrap)
	}
	if !strings.Contains(body, `"series":[]`) {
		t.Fatalf("empty series is not an empty JSON array: %s", body)
	}
}
