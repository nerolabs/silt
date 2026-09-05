package main

// D-UI-PRIVACY-FLAG (owner-ratified 2026-09-05) — the -privacy flag's gates, held to the
// blind PE design ruling RULING-UI-PRIVACY-FLAG-design-2026-09-05 §6. Untagged: this is
// default-build code. Every withheld field is ABSENT with a sibling marker, never a zero;
// every privacy withhold is an ALLOW-LIST; the pages never throw on a withheld document.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/nerolabs/silt/adapters/linkbook"
)

// privacyServer is the economy fixture with the privacy flag set as asked. The fixture's
// token is "tok" (economyServer).
func privacyServer(t *testing.T, on bool) *uiServer {
	t.Helper()
	s, _ := statusServer(t)
	s.privacy = on
	return s
}

func privacyGet(t *testing.T, s *uiServer, h http.HandlerFunc, path, auth, query string) map[string]json.RawMessage {
	t.Helper()
	r := httptest.NewRequest("GET", "http://127.0.0.1:8080"+path+query, nil)
	if auth != "" {
		r.Header.Set("Authorization", auth)
	}
	w := httptest.NewRecorder()
	s.guard(http.HandlerFunc(h)).ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatalf("GET %s%s auth=%q: %d %s", path, query, auth, w.Code, w.Body.String())
	}
	var top map[string]json.RawMessage
	if err := json.Unmarshal(w.Body.Bytes(), &top); err != nil {
		t.Fatalf("decode %s: %v (%s)", path, err, w.Body.String())
	}
	return top
}

func keys(m map[string]json.RawMessage) map[string]bool {
	out := map[string]bool{}
	for k := range m {
		out[k] = true
	}
	return out
}

// TestPrivacyStatusWithholdsCountersFromUnauthenticatedReaders is the /api/status
// contract in all three postures. privacy=on untokened: no stats, no balance, the
// countersWithheld marker, privacy.mode "on". privacy=on tokened: everything, no marker.
// privacy=off untokened: everything, no marker, privacy.mode "off".
func TestPrivacyStatusWithholdsCountersFromUnauthenticatedReaders(t *testing.T) {
	on := privacyServer(t, true)
	top := privacyGet(t, on, on.apiStatus, "/api/status", "", "")
	if _, has := top["stats"]; has {
		t.Fatalf("privacy=on, untokened: stats present: %s", top["stats"])
	}
	if string(top["countersWithheld"]) != "true" {
		t.Fatalf("privacy=on, untokened: countersWithheld marker missing; keys %v", keys(top))
	}
	var dur map[string]json.RawMessage
	if err := json.Unmarshal(top["durability"], &dur); err != nil {
		t.Fatal(err)
	}
	if _, has := dur["balance"]; has {
		t.Fatalf("privacy=on, untokened: durability.balance present: %s", top["durability"])
	}
	if _, has := dur["bountyOn"]; !has || string(dur["detailWithheld"]) != "true" {
		t.Fatalf("privacy=on, untokened: durability lost bountyOn or the F2 marker: %s", top["durability"])
	}
	var priv privacyInfo
	if err := json.Unmarshal(top["privacy"], &priv); err != nil || priv.Mode != "on" || priv.Default != "on" {
		t.Fatalf("privacy block = %s (err %v), want mode on / default on", top["privacy"], err)
	}

	tok := privacyGet(t, on, on.apiStatus, "/api/status", "Bearer tok", "")
	if _, has := tok["stats"]; !has {
		t.Fatalf("privacy=on, tokened: stats ABSENT — the operator's own read must be unchanged")
	}
	if _, has := tok["countersWithheld"]; has {
		t.Fatalf("privacy=on, tokened: marker present on a full document")
	}
	json.Unmarshal(tok["durability"], &dur)
	if _, has := dur["balance"]; !has {
		t.Fatalf("privacy=on, tokened: durability.balance absent")
	}

	off := privacyServer(t, false)
	pub := privacyGet(t, off, off.apiStatus, "/api/status", "", "")
	if _, has := pub["stats"]; !has {
		t.Fatalf("privacy=off, untokened: stats absent")
	}
	if _, has := pub["countersWithheld"]; has {
		t.Fatalf("privacy=off: marker present")
	}
	json.Unmarshal(pub["privacy"], &priv)
	if priv.Mode != "off" || priv.Default != "on" {
		t.Fatalf("privacy=off: block = %s, want mode off / default on", pub["privacy"])
	}
}

// TestPrivacyZeroBalanceIsPublishedNotOmitted pins S7: Balance is a pointer so that a
// legitimate zero survives omitempty. A fresh node with a zero balance publishes 0.
func TestPrivacyZeroBalanceIsPublishedNotOmitted(t *testing.T) {
	s := privacyServer(t, false)
	top := privacyGet(t, s, s.apiStatus, "/api/status", "", "")
	var dur map[string]json.RawMessage
	json.Unmarshal(top["durability"], &dur)
	raw, has := dur["balance"]
	if !has {
		t.Fatalf("balance key absent on a published document — a zero balance was omitted as if withheld: %s", top["durability"])
	}
	_ = raw
}

// TestPrivacyEconomySelfIsAnAllowList is the S3 gate: the privacy view of the self
// document is a fresh struct. Reflection over the wire: every key present on the
// untokened privacy=on document is in the declared allow-list, so a field added to
// economySelf later cannot appear here without also being added to this list.
func TestPrivacyEconomySelfIsAnAllowList(t *testing.T) {
	s := privacyServer(t, true)
	top := privacyGet(t, s, s.apiEconomySelf, "/api/economy/self", "", "")
	allowed := map[string]bool{"tier": true, "detailWithheld": true, "countersWithheld": true,
		"snapshotTakenAtUnix": true, "snapshotAgeSec": true, "snapshotIntervalSec": true}
	for k := range top {
		if !allowed[k] {
			t.Fatalf("privacy=on, untokened /api/economy/self carries key %q outside the allow-list %v: %s", k, allowed, top[k])
		}
	}
	if string(top["countersWithheld"]) != "true" || string(top["detailWithheld"]) != "true" {
		t.Fatalf("markers missing: %v", keys(top))
	}
	// The operator's read is the full document.
	tok := privacyGet(t, s, s.apiEconomySelf, "/api/economy/self", "Bearer tok", "")
	for _, k := range []string{"revenue", "margin", "wash"} {
		if _, has := tok[k]; !has {
			t.Fatalf("tokened self document lost %q", k)
		}
	}
	if _, has := tok["countersWithheld"]; has {
		t.Fatalf("tokened self document carries the marker")
	}
}

// TestPrivacyLibraryLinkTakesTheHeaderPredicate is the Q4 split: a link is a permanent
// capability, so a QUERY token does not unlock it, the HEADER token does, and with the
// flag off it is published as before. The withheld rows are rebuilt from the allow-list.
func TestPrivacyLibraryLinkTakesTheHeaderPredicate(t *testing.T) {
	s := privacyServer(t, true)
	dir := t.TempDir()
	lb, err := linkbook.Open(filepath.Join(dir, "links.json"))
	if err != nil {
		t.Fatal(err)
	}
	const lnk = "silt:v1:I9WkeOSffIMeS-Eqpxh6WmgLc52VPtkH364Anoi_pHA:FMEZm_C82M7qqkuaOFbZuGvq617Oho9mGDGxFfxZ2a0"
	if _, err := lb.Add(lnk, "movie"); err != nil {
		t.Fatal(err)
	}
	s.links = lb

	check := func(auth, query string, wantLink bool) {
		t.Helper()
		top := privacyGet(t, s, s.apiLibrary, "/api/library", auth, query)
		var rows []map[string]json.RawMessage
		if err := json.Unmarshal(top["library"], &rows); err != nil || len(rows) != 1 {
			t.Fatalf("library rows = %s (err %v)", top["library"], err)
		}
		_, hasLink := rows[0]["link"]
		_, marker := top["linksWithheld"]
		if hasLink != wantLink || marker == wantLink {
			t.Fatalf("auth=%q query=%q: link present=%v (want %v), linksWithheld present=%v", auth, query, hasLink, wantLink, marker)
		}
		if !wantLink {
			for k := range rows[0] {
				if !map[string]bool{"root": true, "label": true, "added": true, "onChain": true, "fileSize": true}[k] {
					t.Fatalf("withheld row carries key %q outside the allow-list: %s", k, top["library"])
				}
			}
		}
	}
	check("", "", false)
	check("", "?token=tok", false) // a URL token is a logged token; it cannot unlock a permanent capability
	check("Bearer tok", "", true)
	s.privacy = false
	check("", "", true)
}

// TestPrivacyFlagRefusesAnythingButOnOrOff is S5: -privacy is a string flag accepting
// exactly "on" and "off", and any other value refuses to start naming both.
func TestPrivacyFlagRefusesAnythingButOnOrOff(t *testing.T) {
	if on, err := parsePrivacyFlag("on"); err != nil || !on {
		t.Fatalf("on: %v %v", on, err)
	}
	if on, err := parsePrivacyFlag("off"); err != nil || on {
		t.Fatalf("off: %v %v", on, err)
	}
	for _, bad := range []string{"", "true", "false", "0", "1", "ON", "yes"} {
		_, err := parsePrivacyFlag(bad)
		if err == nil {
			t.Fatalf("-privacy=%q was ACCEPTED; a typo must refuse, never silently mean on or off", bad)
		}
		if !strings.Contains(err.Error(), "on") || !strings.Contains(err.Error(), "off") {
			t.Fatalf("refusal for %q does not name both accepted values: %v", bad, err)
		}
	}
	if !privacyDefaultWithheld {
		t.Fatalf("the compiled default is EXPOSED — D-UI-PRIVACY-FLAG's guarantee sentence and the release.yml assertion both require withheld by default (PE ruling S4, option E)")
	}
}

// TestPrivacyBothSubcommandsParseTheFlagBeforeServing is a source gate: daemon.go and
// client.go both declare -privacy through parsePrivacyFlag, and each refuses before
// ui.serve. RUNTIME GATE: TestPrivacyFlagRefusesAnythingButOnOrOff observes the predicate;
// this gate adds only that both call sites exist and precede the bind.
func TestPrivacyBothSubcommandsParseTheFlagBeforeServing(t *testing.T) {
	for _, f := range []string{"daemon.go", "client.go"} {
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		s := string(src)
		decl := strings.Index(s, `fs.String("privacy", privacyModeName(privacyDefaultWithheld)`)
		parse := strings.Index(s, "parsePrivacyFlag(*privacyFlag)")
		serve := strings.Index(s, "ui.serve(*uiAddr)")
		if decl < 0 || parse < 0 || serve < 0 {
			t.Fatalf("SOURCE GATE: %s lacks the -privacy declaration (%d), the parsePrivacyFlag call (%d) or ui.serve (%d)", f, decl, parse, serve)
		}
		if parse > serve {
			t.Fatalf("SOURCE GATE: %s parses -privacy (byte %d) AFTER ui.serve (byte %d); a refused value must never leave a UI listening", f, parse, serve)
		}
	}
}

// TestPrivacyAllThreeHandlersRouteThroughTheirView widens the composition-point gate the
// G-BB-12′ build left covering /api/status only: apiStatus → readerView, apiEconomySelf →
// economyView, apiLibrary → libraryView, no withhold outside them, and the three view
// functions are declared adjacent under one doc comment. RUNTIME GATE: the three privacy
// wire tests above observe the behaviour; this gate adds only WHERE the clauses live.
func TestPrivacyAllThreeHandlersRouteThroughTheirView(t *testing.T) {
	src, err := os.ReadFile("ui.go")
	if err != nil {
		t.Fatal(err)
	}
	s := string(src)
	for handler, view := range map[string]string{
		"func (s *uiServer) apiStatus(":      "s.readerView(doc, r)",
		"func (s *uiServer) apiEconomySelf(": "s.economyView(out, s.readerAuthFor(r))",
		"func (s *uiServer) apiLibrary(":     "s.libraryView(",
	} {
		body := funcBody(s, handler)
		if !strings.Contains(body, view) {
			t.Fatalf("SOURCE GATE: %s does not route through %s", handler, view)
		}
		for _, forbidden := range []string{"withheldDurability(", "withheldEconomySelf(", "privacyWithheld", "withholdBBootstrap(", "validToken(r)", "Link = \"\""} {
			if strings.Contains(body, forbidden) {
				t.Fatalf("SOURCE GATE: %s contains %q — a withhold outside its view function", handler, forbidden)
			}
		}
	}
	rv := strings.Index(s, "func (s *uiServer) readerView(")
	ev := strings.Index(s, "func (s *uiServer) economyView(")
	lv := strings.Index(s, "func (s *uiServer) libraryView(")
	if rv < 0 || ev < 0 || lv < 0 || !(rv < ev && ev < lv) || lv-rv > 6000 {
		t.Fatalf("SOURCE GATE: the three view functions must be declared adjacent, in order readerView (%d) → economyView (%d) → libraryView (%d)", rv, ev, lv)
	}
	if !strings.Contains(s, "THE COMPOSITION POINT for every serve-time withhold") || !strings.Contains(s, "THE PREDICATE RULE") {
		t.Fatalf("SOURCE GATE: the shared doc comment (marker convention + predicate rule) is missing")
	}
}

// TestPrivacyPagesRenderWithheldDocumentsWithoutThrowing is the S1 gate, BEHAVIOURAL: it
// runs the pages' shared render functions (cmd/silt/ui/render.js) under node against a
// fixture holding a withheld daemon, a publishing daemon and an old daemon that predates
// the marker, and asserts no throw, the word "withheld" where a number would have been,
// the published daemon's bytes summed, and the pre-release banner text for a -privacy=off
// node. node is required (GitHub's runners and the dev boxes have it); a missing node is a
// FAILURE, not a skip — a skipped gate is a gate that rots.
func TestPrivacyPagesRenderWithheldDocumentsWithoutThrowing(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Fatalf("node is required to run the page render gate (cmd/silt/ui/render.js): %v", err)
	}
	script := `
const r = require(require("path").resolve(process.argv[1]));
const withheld = { countersWithheld: true, chunks: 3, privacy: { mode: "on", default: "on" } };
const published = { stats: { BytesServed: 3145728, ChunksServed: 3, ChunksReceived: 1 }, chunks: 5, privacy: { mode: "off", default: "on" } };
const old = { stats: { BytesServed: 1024 }, chunks: 1 }; // predates the marker and the privacy block
const out = {};
out.cardsW = r.statusCards(withheld); out.cardsP = r.statusCards(published); out.cardsO = r.statusCards(old);
out.totals = r.observatoryTotals([{status: withheld}, {status: published}, {status: old}, {status: undefined}]);
out.cellW = r.servedCell(withheld); out.cellP = r.servedCell(published);
out.bannerP = r.prereleaseBanner(published); out.bannerW = r.prereleaseBanner(withheld); out.bannerO = r.prereleaseBanner(old);
console.log(JSON.stringify(out));`
	cmd := exec.Command(node, "-e", script, filepath.Join("ui", "render.js"))
	raw, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("render.js THREW on the fixture (the abort-the-page shape the PE measured): %v\n%s", err, raw)
	}
	var out struct {
		CardsW, CardsP, CardsO struct {
			Served, Servedsub string
			Withheld          bool
		}
		Totals                    struct{ Served, Chunks, WithheldCount int64 }
		CellW, CellP              string
		BannerP, BannerW, BannerO string
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("decode render output: %v\n%s", err, raw)
	}
	if out.CardsW.Served != "withheld" || !out.CardsW.Withheld || !strings.Contains(out.CardsW.Servedsub, "-privacy") {
		t.Fatalf("withheld card = %+v — must say withheld and name the recovery, never a number", out.CardsW)
	}
	if out.CardsP.Served != "3.0 MB" || out.CardsP.Withheld {
		t.Fatalf("published card = %+v", out.CardsP)
	}
	if out.CardsO.Served != "1.0 KB" {
		t.Fatalf("old-daemon card = %+v — a document without the privacy block must render as before", out.CardsO)
	}
	if out.Totals.Served != 3145728+1024 || out.Totals.WithheldCount != 1 || out.Totals.Chunks != 9 {
		t.Fatalf("observatory totals = %+v: published bytes summed, withheld daemons counted not zeroed", out.Totals)
	}
	if !strings.Contains(out.CellW, "withheld") || out.CellP != "3.0 MB" {
		t.Fatalf("served cells = %q / %q", out.CellW, out.CellP)
	}
	if !strings.HasPrefix(out.BannerP, "PRE-RELEASE") || out.BannerW != "" || out.BannerO != "" {
		t.Fatalf("banners = %q / %q / %q: only a -privacy=off node is labelled", out.BannerP, out.BannerW, out.BannerO)
	}
	// And the pages actually use render.js: no bare stats dereference survives in either.
	for _, page := range []string{"index.html", "observatory.html"} {
		html, err := os.ReadFile(filepath.Join("ui", page))
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(html), ".stats.BytesServed") || strings.Contains(string(html), ".stats.ChunksServed") {
			t.Fatalf("SOURCE GATE: %s dereferences .stats.* inline; on a withheld document that is a TypeError that aborts the render — route through render.js", page)
		}
		if !strings.Contains(string(html), `<script src="render.js"></script>`) || !strings.Contains(string(html), "siltRender.") {
			t.Fatalf("SOURCE GATE: %s does not load and use render.js", page)
		}
	}
}

// TestPrivacyWithheldDocumentsCarryOnlyAllowedKeys is gate 7: reflection over the three
// untokened privacy=on documents — every top-level key is either declared allowed or is a
// marker. Anchored on the key SET, never a name match (the bBootstrapWithheld lesson).
func TestPrivacyWithheldDocumentsCarryOnlyAllowedKeys(t *testing.T) {
	s := privacyServer(t, true)
	status := privacyGet(t, s, s.apiStatus, "/api/status", "", "")
	allowedStatus := map[string]bool{"id": true, "peer": true, "uptimeSec": true, "capUsed": true, "capTotal": true,
		"chunks": true, "peers": true, "network": true, "validator": true, "reachability": true, "chain": true,
		"durability": true, "addressCap": true, "countersWithheld": true, "privacy": true,
		"snapshotTakenAtUnix": true, "snapshotAgeSec": true, "snapshotIntervalSec": true}
	for k := range status {
		if !allowedStatus[k] {
			t.Fatalf("privacy=on untokened /api/status carries undeclared key %q", k)
		}
	}
	// The Go type says the same thing: every statusInfo field either is in the allow-list
	// by its JSON name, or is one of the two withheld fields (stats, and balance inside
	// durability). A new field on statusInfo must be classified here before it ships.
	for i := 0; i < reflect.TypeOf(statusInfo{}).NumField(); i++ {
		f := reflect.TypeOf(statusInfo{}).Field(i)
		name := strings.Split(f.Tag.Get("json"), ",")[0]
		if name == "" || name == "-" {
			continue
		}
		if !allowedStatus[name] && name != "stats" {
			t.Fatalf("statusInfo field %s (json %q) is neither allowed on the privacy view nor declared withheld — classify it", f.Name, name)
		}
	}
}

// funcBody returns the source of the function starting at decl through its closing brace.
func funcBody(src, decl string) string {
	i := strings.Index(src, decl)
	if i < 0 {
		return ""
	}
	rest := src[i:]
	end := strings.Index(rest, "\n}\n")
	if end < 0 {
		return rest
	}
	return rest[:end]
}

// TestPrivacyEconomySelfViewIsSpelledAsAnAllowList is the source half of S3: the privacy
// view CONSTRUCTS a fresh economySelf literal and never nils a named field of the full
// document, which is the spelling that once shipped the pooled selfFunding figures open.
// RUNTIME GATE: TestPrivacyEconomySelfIsAnAllowList observes the wire; this gate adds only
// the spelling, so a future edit that switches to nil-ing fails by name.
func TestPrivacyEconomySelfViewIsSpelledAsAnAllowList(t *testing.T) {
	src, err := os.ReadFile("ui.go")
	if err != nil {
		t.Fatal(err)
	}
	body := funcBody(string(src), "func privacyWithheldEconomySelf(")
	if body == "" {
		t.Fatalf("SOURCE GATE: the literal func privacyWithheldEconomySelf( is absent from ui.go")
	}
	if !strings.Contains(body, "return economySelf{") || strings.Contains(body, "= nil") {
		t.Fatalf("SOURCE GATE: privacyWithheldEconomySelf must contain the literal `return economySelf{` and no `= nil` — a fresh allow-list struct, never a deny-list over the full document; body:\n%s", body)
	}
}
