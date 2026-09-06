package relay

import (
	"os"
	"strings"
	"testing"

	"github.com/nerolabs/silt/adapters/identity"
	"github.com/nerolabs/silt/core/relaypay"
)

// TestRelayServeHoldsOneCapAtTheProtocolCeiling is the G-R212-2 adapter gate (blind PE
// F-2, 2026-09-06): the per-splice cap defaults to the protocol session ceiling, Serve
// refuses a lower explicit cap (a COHERENCE refusal — a lower cap would burn the bytes a
// fetcher paid for past it, T-RELAY-GRAN), and accepts the ceiling itself. Ablation
// "default back to 1 GiB / delete the Serve refusal" reddens here.
func TestRelayServeHoldsOneCapAtTheProtocolCeiling(t *testing.T) {
	if got := (Config{}).withDefaults().MaxSessionBytes; got != relaypay.MaxSessionBytes {
		t.Fatalf("default MaxSessionBytes = %d, want the protocol ceiling %d", got, relaypay.MaxSessionBytes)
	}
	ident := identity.FromSeed(7301)
	if srv, err := Serve("127.0.0.1:0", ident, Config{MaxSessionBytes: relaypay.MaxSessionBytes - 1}, nil); err == nil {
		srv.Close()
		t.Fatal("Serve accepted a per-splice cap one byte BELOW the protocol ceiling — a paid session past the cap would burn what the fetcher paid for")
	}
	srv, err := Serve("127.0.0.1:0", ident, Config{MaxSessionBytes: relaypay.MaxSessionBytes}, nil)
	if err != nil {
		t.Fatalf("Serve refused the protocol ceiling itself: %v", err)
	}
	srv.Close()
}

// TestRelayFreeAndPaidSplicesReadTheSameCap is the D-POD-RELAY-COEXIST seam: the free
// splice and the paid pump are capped by the SAME config field, so no free/paid
// differential can exist in this adapter. SOURCE GATE: the two call sites name
// s.cfg.MaxSessionBytes. RUNTIME GATE: TestRelayServeHoldsOneCapAtTheProtocolCeiling
// covers the field's value; the differential itself has no runtime observable short of
// pushing 24.4 GiB through each path.
func TestRelayFreeAndPaidSplicesReadTheSameCap(t *testing.T) {
	server, err := os.ReadFile("server.go")
	if err != nil {
		t.Fatal(err)
	}
	paid, err := os.ReadFile("paid.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(server), "io.LimitReader(src, s.cfg.MaxSessionBytes)") {
		t.Fatal("SOURCE GATE: the free splice no longer caps on s.cfg.MaxSessionBytes — a free/paid differential is possible (D-POD-RELAY-COEXIST)")
	}
	if !strings.Contains(string(paid), "paidSession(a, b, auth, s.cfg.MaxSessionBytes)") {
		t.Fatal("SOURCE GATE: the paid pump no longer caps on s.cfg.MaxSessionBytes — a free/paid differential is possible (D-POD-RELAY-COEXIST)")
	}
}
