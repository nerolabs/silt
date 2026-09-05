package e2e

// Phase 2 economy path, validated on a REAL daemon over real HTTP (the local
// prerequisite before the billable cloud economy run — build-immutables #6/#7:
// reproduce locally first). This exercises Slices 1–3 integrated end to end:
// -economy (Slice 1) arms the bounty payout; -care-published makes the daemon
// caretake what it publishes; /api/fund (Slice 3) endows the object's durability
// reserve from the daemon's own earned balance; and /api/status's durability block
// (Slice 2) surfaces bountyOn + the per-object reserve. It does NOT test the bounty
// PAYOUT (that needs a multi-node reconstruction + a judge quorum — covered in sim
// by TestRepairBountyPaysHolderWithoutMovingStanding); it validates that the economy
// config, the endowment path, and the telemetry all come up and agree on a live
// daemon. Design: docs/thinking/2026-08-19-cloudtest-economy-scenario-design.md.

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

// statusDurability is the subset of GET /api/status this test asserts on.
type statusDurability struct {
	Durability *struct {
		BountyOn       bool  `json:"bountyOn"`
		Balance        int64 `json:"balance"`
		DetailWithheld bool  `json:"detailWithheld"`
		Objects        []struct {
			Root    string `json:"root"`
			Reserve int64  `json:"reserve"`
			Funded  int64  `json:"funded"`
			Paid    int64  `json:"paid"`
			Repairs int64  `json:"repairs"`
		} `json:"objects"`
	} `json:"durability"`
}

// getStatus reads the OPERATOR's view. The per-object durability array is token-gated
// (red-team F2: delta funded x 8 is the exact byte count served of a NAMED content
// root), so the operator's own reader presents the token — which is what the daemon's
// own web UI does on every same-origin /api/ call. getStatusUntokened below is the
// public reader, and the e2e gate asserts it sees no roots.
func getStatus(t *testing.T, base, token string) statusDurability {
	t.Helper()
	return decodeStatus(t, base, token)
}

func decodeStatus(t *testing.T, base, token string) statusDurability {
	t.Helper()
	req, err := http.NewRequest("GET", base+"/api/status", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	if err != nil {
		t.Fatalf("GET /api/status: %v", err)
	}
	defer resp.Body.Close()
	var s statusDurability
	if err := json.NewDecoder(resp.Body).Decode(&s); err != nil {
		t.Fatalf("decode status: %v", err)
	}
	return s
}

// apiFund POSTs to /api/fund with the bearer token; returns status + body.
func apiFund(base, token, root string, amount int64) (int, string, error) {
	form := url.Values{"root": {root}, "amount": {fmt.Sprint(amount)}}
	req, err := http.NewRequest("POST", base+"/api/fund", strings.NewReader(form.Encode()))
	if err != nil {
		return 0, "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	if err != nil {
		return 0, "", err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<10))
	return resp.StatusCode, string(b), nil
}

func TestEconomyEndToEndOnLiveDaemon(t *testing.T) {
	if testing.Short() {
		t.Skip("e2e spawns processes; skipped under -short")
	}

	// A validator that serves a registry, caretakes what it publishes, and has the
	// repair economy ON — the single-box shape an operator running -economy gets.
	a := startDaemon(t, "A",
		"-listen", "127.0.0.1:0", "-store", t.TempDir(),
		"-serve-registry", "127.0.0.1:0",
		"-validator", "-quorum", "0", "-min-rep", "0",
		"-economy", "-care-published",
		"-capacity", "1G", "-mdns=false", "-id-seed", "1001",
		"-ui", "127.0.0.1:0")
	ui := a.waitFor(t, reUI, 20*time.Second)
	base, token := "http://"+ui[1], ui[2]

	// The economy must report ON (Slice 1's -economy flag → bountyOn), and the daemon
	// carries its starter credit balance (what it could spend to fund durability).
	s0 := getStatus(t, base, token)
	if s0.Durability == nil || !s0.Durability.BountyOn {
		t.Fatalf("economy not reported ON with -economy: %+v", s0.Durability)
	}
	if s0.Durability.Balance <= 0 {
		t.Fatalf("daemon should carry a starter credit balance, got %d", s0.Durability.Balance)
	}

	// Publish through the UI: -care-published makes the daemon caretake this root,
	// so it appears in the durability telemetry as an object it maintains.
	payload := make([]byte, 256<<10)
	for i := range payload {
		payload[i] = byte(i * 7)
	}
	status, body, err := uiPublish(base, token, "econ.bin", payload)
	if err != nil || status != http.StatusOK {
		t.Fatalf("publish: status=%d err=%v body=%s", status, err, body)
	}
	var pub struct {
		Link string `json:"link"`
	}
	_ = json.Unmarshal([]byte(body), &pub)
	if pub.Link == "" {
		t.Fatalf("publish returned no link: %s", body)
	}

	// The cared object shows up in the durability block (may take a sweep to register).
	var root string
	for deadline := time.Now().Add(15 * time.Second); time.Now().Before(deadline); {
		s := getStatus(t, base, token)
		if s.Durability != nil && len(s.Durability.Objects) > 0 {
			root = s.Durability.Objects[0].Root
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	if root == "" {
		t.Fatal("published+cared object never appeared in the durability telemetry")
	}

	// Endow the object's reserve from the daemon's own balance (Slice 3), and confirm
	// the telemetry reflects the funded reserve + the debited balance.
	balBefore := getStatus(t, base, token).Durability.Balance
	const endow = 5000
	fs, fb, err := apiFund(base, token, pub.Link, endow)
	if err != nil || fs != http.StatusOK {
		t.Fatalf("fund: status=%d err=%v body=%s", fs, err, fb)
	}
	s1 := getStatus(t, base, token)
	if s1.Durability.Balance != balBefore-endow {
		t.Fatalf("funding did not debit the balance: %d → %d (want -%d)", balBefore, s1.Durability.Balance, endow)
	}
	var funded int64 = -1
	for _, o := range s1.Durability.Objects {
		if o.Root == root {
			funded = o.Reserve
		}
	}
	if funded < endow {
		t.Fatalf("object reserve %d < endowment %d — /api/fund did not credit the escrow the telemetry reads", funded, endow)
	}

	// R2.9a / red-team F2, on a REAL daemon over real HTTP (build-immutable #1, the
	// e2e tier). The same GET without the token must carry the aggregates and NO
	// per-object entry: `funded` moves by one eighth of every byte served of a NAMED
	// root, so an unauthenticated reader polling it recovers the exact byte count
	// served of that object. The operator's view above is unchanged; only the
	// unauthenticated one loses the decomposition.
	pubView := decodeStatus(t, base, "")
	if pubView.Durability == nil {
		t.Fatalf("the durability block vanished for an unauthenticated reader — the AGGREGATES stay open; only the per-object array is gated")
	}
	if len(pubView.Durability.Objects) != 0 {
		t.Fatalf("unauthenticated /api/status published per-object durability: %+v — eight times the delta in `funded` is the exact byte count served of that named root", pubView.Durability.Objects)
	}
	if !pubView.Durability.DetailWithheld {
		t.Fatalf("objects withheld but detailWithheld is false — a reader must be able to tell a withholding from a node that caretakes nothing")
	}
	if pubView.Durability.Balance != s1.Durability.Balance {
		t.Fatalf("the node-wide balance changed with the token (%d vs %d) — this change publishes less, it never counts less", pubView.Durability.Balance, s1.Durability.Balance)
	}
}
