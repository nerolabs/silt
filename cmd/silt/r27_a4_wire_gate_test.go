package main

// R2.7 / A4-3 — the wire gate on withheldEconomySelf's revenue rebuild (blind PE ruling
// RULING-c4-r27-blocking-telemetry-93deb56-2026-09-08 B1).
//
// WHY THIS IS A SIBLING AND NOT AN EDIT TO THE EXISTING WHOLE-SURFACE SCAN.
// TestR29aF2NoUnauthenticatedResponseOnTheWholeSurfaceCarriesTheWithheldCounter scans
// for ONE value, objects[0].funded, and its strength is that at `paid == 0` the
// per-object `reserve`, `net` and `skimIn` are ALIASES of that same value — four
// quantities covered by one scan.
//
// Do not read the split as forced; it is a CHOICE, and the blind PE measured the
// arithmetic this comment first got wrong. Paying a bounty into that fixture destroys
// TWO of the four aliases, not three: `skimIn` is an alias unconditionally, because
// economyObject.SkimIn is assigned o.Funded and never subtracts `paid`. And a
// single-scan fix WAS available — A4-3 is node-wide while the per-object walk covers
// cared roots only, so paying from an UNCARED root drives this field non-zero with all
// four aliases intact. Two properties in two fixtures is simply the more robust shape:
// each fixture stays legible, and r29aWholeSurfaceGETRoutes couples them so a new GET
// route reddens both until it has been examined against each one's property.
//
// WHAT IT PROVES. The scan generalises the same way its sibling does: it walks the real
// apiRoutes table and asks "what reconstructs the quantity", not "where is the field
// called bountyPaidToPriorFetcherCredits". The positive control comes FIRST — without a
// tokened body carrying the real figures, a clean untokened walk would pass on a fixture
// that paid no bounty, which is exactly how the existing gates missed this field.
//
// ABLATION (PE ablation H): reverting withheldEconomySelf's rebuild to
// `rev := full.Revenue` must redden this, with the leaked credits figure in the text.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/nerolabs/silt/core/link"
	"github.com/nerolabs/silt/ports"
)

// r27WashBountyCredits is the bounty this fixture pays to a PRIOR FETCHER. Chosen so the
// figure is a value nothing else on the surface holds by coincidence, and so it is
// distinct from the object's funded (900,000), its reserve and its net (245,679) — the
// scan below must not be able to pass by matching some other quantity.
const (
	r27WashUnits         = int64(900_000)
	r27WashBountyCredits = int64(654_321)
	r27WashServedBytes   = r27WashUnits * econMintUnit
)

func TestR27A4PriorFetcherCreditsAreTokenGatedOnTheWholeSurface(t *testing.T) {
	s, led := statusServer(t)
	root := ports.Hash{0xA4, 0x03, 0x11}
	repairer := ports.NodeID{0xB0, 0x07}
	s.onLoop(func() {
		s.nd.Care(emptyRegistry{}, link.CareHandle{Root: root})
		// The serve fills the object's escrow through the auto-skim AND gives the
		// repairer fetchedBytes > 0 — it is the same identity on both legs, which is the
		// round-trip shape A4-3 exists to make visible.
		led.RecordServeToObject(s.nd.ID(), repairer, root, ports.ChunkID{0x1}, r27WashServedBytes)
		if got := led.PayBounty(root, repairer, r27WashBountyCredits); got != r27WashBountyCredits {
			t.Errorf("fixture: PayBounty paid %d, want %d — the escrow is too thin to drive the detector", got, r27WashBountyCredits)
		}
	})
	at := s.started

	// POSITIVE CONTROL FIRST. The detector must actually have fired, or the untokened
	// walk below is vacuous: it would pass on a fixture that paid no bounty, which is
	// precisely how every shipped gate missed this field.
	full := decodeRevenue(t, economySelfAt(t, s, at, true))
	if full.Payments != 1 || full.Credits != r27WashBountyCredits {
		t.Fatalf("fixture is vacuous: TOKENED revenue carries payments=%d credits=%d, want 1 and %d — A4-3 never fired",
			full.Payments, full.Credits, r27WashBountyCredits)
	}
	if strings.Contains(full.Note, "withheld") {
		t.Fatalf("TOKENED revenue note = %q, want the SHAPE caveat — the operator's own read must carry the honest limit, not a withhold marker", full.Note)
	}

	// The untokened document: both figures zeroed, and the withhold NAMED. Absent and
	// zero are different objects, so the note is what tells "we paid nobody" from
	// "you may not see this".
	open := decodeRevenue(t, economySelfAt(t, s, at, false))
	if open.Payments != 0 || open.Credits != 0 {
		t.Fatalf("UNAUTHENTICATED /api/economy/self carries revenue.bountyPaidToPriorFetcher payments=%d credits=%d. That is a real bounty-out figure: on a node caretaking ONE root it IS that root's withheld objects[].bountyOut, and /api/roots supplies the name half of the join (red-team F2)",
			open.Payments, open.Credits)
	}
	if !strings.Contains(open.Note, "withheld") {
		t.Fatalf("untokened revenue note = %q, want a note that NAMES the withhold — a zeroed pair with the SHAPE caveat reads as \"this node paid no prior fetcher\", which is a false absence (Don't #4)", open.Note)
	}

	// The whole GET surface, untokened: no route may republish the credits figure under
	// any name. Walks the real route table, so a route added later is examined by
	// construction.
	walked := 0
	for pattern, h := range s.apiRoutes() {
		method, path, _ := strings.Cut(pattern, " ")
		if method != http.MethodGet {
			continue
		}
		walked++
		s.now = func() time.Time { return at }
		r := httptest.NewRequest(method, "http://127.0.0.1:8080"+path, nil)
		w := httptest.NewRecorder()
		s.guard(h).ServeHTTP(w, r) // NO Authorization header
		body := w.Body.String()
		for _, n := range jsonNumbers(t, body) {
			if n == r27WashBountyCredits {
				t.Fatalf("%s carries %d on the UNAUTHENTICATED wire. That is the A4-3 bounty-out figure under whatever key this route calls it; gating it on /api/economy/self while a sibling republishes the same quantity closes nothing:\n%s", pattern, n, body)
			}
		}
	}
	if walked != r29aWholeSurfaceGETRoutes {
		t.Fatalf("walked %d GET routes, want %d — a route was added or removed; examine what it republishes before raising the count", walked, r29aWholeSurfaceGETRoutes)
	}
}

// r27Revenue is the A4-3 slice of the revenue block. Decoded on its own so a change to
// the block's other fields cannot make this gate stop compiling and quietly stop running.
type r27Revenue struct {
	Payments int64  `json:"bountyPaidToPriorFetcherPayments"`
	Credits  int64  `json:"bountyPaidToPriorFetcherCredits"`
	Note     string `json:"bountyPaidToPriorFetcherNote"`
}

func decodeRevenue(t *testing.T, body string) r27Revenue {
	t.Helper()
	var doc struct {
		Revenue *r27Revenue `json:"revenue"`
	}
	if err := json.Unmarshal([]byte(body), &doc); err != nil {
		t.Fatalf("decode economy/self: %v (%s)", err, body)
	}
	if doc.Revenue == nil {
		t.Fatalf("no revenue block on /api/economy/self — this gate is about what revenue CARRIES, so an absent block means the fixture changed:\n%s", body)
	}
	return *doc.Revenue
}
