//go:build bbootstrap

package main

// The code, not a handoff note, keeps the B_bootstrap block off every reader that is not
// the operator. This file holds the startup refusal of a routable bind, and the two source
// gates that keep the refusal in place and the block unreachable on a daemon.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/nerolabs/silt/core/credit"
)

// TestRoutableBindWithBBootstrapIsRefusedAtStartup pins the predicate on the
// operator's own flag string: loopback literals and "localhost" pass, everything that
// could put the block on a network — all-interfaces, a LAN or public literal, a hostname
// the node cannot vouch for — is refused, and the refusal names both flags so the
// operator knows which combination to change. With the flag off nothing is refused: a
// tagged binary run without -bbootstrap is a default daemon.
func TestRoutableBindWithBBootstrapIsRefusedAtStartup(t *testing.T) {
	accept := []string{"127.0.0.1:8081", "localhost:8081", "[::1]:8081", "127.0.0.2:9000", "127.255.255.254:1"}
	refuse := []string{"0.0.0.0:8081", "[::]:8081", ":8081", "192.168.1.5:8081", "10.0.0.7:8081", "203.0.113.7:8081", "silt.example.org:8081", "[2001:db8::1]:8081"}
	for _, a := range accept {
		if err := bbootstrapRefuseRoutableBind(a, true); err != nil {
			t.Fatalf("-ui %q -bbootstrap was REFUSED; a loopback bind is the one configuration the run is allowed: %v", a, err)
		}
	}
	for _, a := range refuse {
		err := bbootstrapRefuseRoutableBind(a, true)
		if err == nil {
			t.Fatalf("-ui %q -bbootstrap was ACCEPTED: the histogram would be published on a routable bind (art A refuses this at startup)", a)
		}
		for _, must := range []string{"-bbootstrap", "-ui", a, "loopback"} {
			if !strings.Contains(err.Error(), must) {
				t.Fatalf("refusal for %q does not name %q — the operator must be told which flags and which fix: %v", a, must, err)
			}
		}
		// The same bind with the flag OFF is an ordinary daemon and is not refused.
		if err := bbootstrapRefuseRoutableBind(a, false); err != nil {
			t.Fatalf("-ui %q WITHOUT -bbootstrap was refused; the refusal must bind only the flag combination: %v", a, err)
		}
	}
	// No UI at all: nothing is published, nothing to refuse.
	if err := bbootstrapRefuseRoutableBind("", true); err != nil {
		t.Fatalf("-bbootstrap with no -ui was refused: %v", err)
	}
}

// TestDaemonRefusesBeforeItBindsOrMintsAToken is a source gate on daemon.go: the
// refusal runs inside the -ui block, BEFORE loadOrCreateUIToken and BEFORE ui.serve, so a
// refused combination leaves no token file behind and never binds the UI socket. (The
// P2P transport and the store directory already exist by then — the refusal sits after
// node startup — so "leaves nothing behind" would be false; what it pins is the UI
// bind and the token file.) Order is the property; a call that moved after ui.serve
// would refuse a daemon whose UI is already listening. The same order holds for the token-file refusal: it must run before
// loadOrCreateUIToken, so a bad mode is caught before the file is ever read.
//
// RUNTIME GATE: TestRoutableBindWithBBootstrapIsRefusedAtStartup and
// TestInsecureTokenFileIsRefusedAtStartup observe the two predicates. The call
// ORDER inside daemon.go is what this source gate adds; no runtime test boots the daemon.
func TestDaemonRefusesBeforeItBindsOrMintsAToken(t *testing.T) {
	src, err := os.ReadFile("daemon.go")
	if err != nil {
		t.Fatal(err)
	}
	s := string(src)
	refuse := strings.Index(s, "bbootstrapRefuseRoutableBind(*uiAddr, bbootstrapOn())")
	if refuse < 0 {
		t.Fatalf("SOURCE GATE: daemon.go does not call bbootstrapRefuseRoutableBind(*uiAddr, bbootstrapOn) —  Part A's startup refusal is gone")
	}
	mint := strings.Index(s, "loadOrCreateUIToken(*storeDir)")
	serve := strings.Index(s, "ui.serve(*uiAddr)")
	if mint < 0 || serve < 0 {
		t.Fatalf("SOURCE GATE: daemon.go lost the literal loadOrCreateUIToken(*storeDir) or ui.serve(*uiAddr) this gate anchors on")
	}
	if refuse > mint || refuse > serve {
		t.Fatalf("SOURCE GATE: the refusal (byte %d) must run BEFORE the token is minted (byte %d) and BEFORE the UI is bound (byte %d)", refuse, mint, serve)
	}
	check := strings.Index(s, "bbootstrapRefuseInsecureTokenFile(*storeDir, bbootstrapOn())")
	if check < 0 || check > mint {
		t.Fatalf("SOURCE GATE: daemon.go must call bbootstrapRefuseInsecureTokenFile(*storeDir, bbootstrapOn()) (byte %d) BEFORE loadOrCreateUIToken (byte %d) — the mode is checked before the file is read", check, mint)
	}
}

// TestClientSubcommandWiresNoInstrument keeps the F6 unreachable on the tree:
// -allow-web-origin (a page on an allow-listed web origin reads the local API with no
// token exists ONLY on the `client` subcommand, and the client wires neither the flag
// nor the renderer, so no build and no flag combination puts the histogram behind an
// allow-listed origin. Two literals must stay absent from client.go and one must stay
// present, so a future "let the client publish it too" is a deliberate, reviewed change.
//
// UNGATED: no runtime test boots the client subcommand and reads its /api/status; the
// absence of the instrument on that surface is pinned by source text only.
func TestClientSubcommandWiresNoInstrument(t *testing.T) {
	src, err := os.ReadFile("client.go")
	if err != nil {
		t.Fatal(err)
	}
	s := string(src)
	if !strings.Contains(s, `"allow-web-origin"`) {
		t.Fatalf("SOURCE GATE: client.go no longer declares -allow-web-origin; if it moved to the daemon, the F6 exposure is live there and this gate must be rewritten, not deleted")
	}
	for _, forbidden := range []string{"registerBBootstrapFlag(", "bbootstrapWireUI(", "bbootstrapInject("} {
		if strings.Contains(s, forbidden) {
			t.Fatalf("SOURCE GATE: client.go calls %s — the client subcommand carries -allow-web-origin, so wiring the instrument there publishes the histogram to an allow-listed web origin with no token (Red-team F6)", forbidden)
		}
	}
	dsrc, err := os.ReadFile("daemon.go")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(dsrc), `"allow-web-origin"`) {
		t.Fatalf("SOURCE GATE: daemon.go declares -allow-web-origin; the daemon is the one place the instrument is wired, so the F6 origin bypass is now reachable there and needs its own gate before this one is relaxed")
	}
}

// --- the (b) half: the block is served to the operator and to nobody else ---------------

// r29aStatusAs issues GET /api/status through the real guard with the caller's chosen
// Host, Origin and Authorization, and returns the decoded top-level document.
func r29aStatusAs(t *testing.T, s *uiServer, host, origin, auth, query string) map[string]json.RawMessage {
	t.Helper()
	r := httptest.NewRequest("GET", "http://"+host+"/api/status"+query, nil)
	r.Host = host
	if origin != "" {
		r.Header.Set("Origin", origin)
	}
	if auth != "" {
		r.Header.Set("Authorization", auth)
	}
	w := httptest.NewRecorder()
	s.guard(http.HandlerFunc(s.apiStatus)).ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatalf("GET /api/status as host=%q origin=%q auth=%q query=%q: %d %s", host, origin, auth, query, w.Code, w.Body.String())
	}
	var top map[string]json.RawMessage
	if err := json.Unmarshal(w.Body.Bytes(), &top); err != nil {
		t.Fatalf("decode: %v (%s)", err, w.Body.String())
	}
	return top
}

// r29aKeys reports which of the two the gate keys a document carries.
func r29aKeys(top map[string]json.RawMessage) (block, withheld bool) {
	_, block = top["bBootstrap"]
	raw, has := top["bBootstrapWithheld"]
	return block, has && string(raw) == "true"
}

// TestThreeWireStatesAreDistinctKeySets is the S1 gate at the wire: the three
// states an /api/status reader can be in are three DIFFERENT key sets, none of which is a
// zero-valued block full of false facts. Flag off: neither key. Flag on, not the
// operator: bBootstrapWithheld alone. Flag on, the operator (token in the Authorization
// header): bBootstrap alone. Absent, withheld and published stay different objects.
func TestThreeWireStatesAreDistinctKeySets(t *testing.T) {
	off, offLed, _ := r29aServer(t, false)
	r29aFetch(offLed, 1, 4096)
	if b, w := r29aKeys(r29aStatusAs(t, off, "127.0.0.1:8080", "", "", "")); b || w {
		t.Fatalf("instrument OFF: block=%v withheld=%v, want neither key — a withheld marker on a node with nothing to withhold reads as 'the instrument is on'", b, w)
	}
	if b, w := r29aKeys(r29aStatusAs(t, off, "127.0.0.1:8080", "", "Bearer tok", "")); b || w {
		t.Fatalf("instrument OFF, operator: block=%v withheld=%v, want neither key", b, w)
	}

	on, led, _ := r29aServer(t, true)
	for i := 0; i < credit.BBootstrapMinRequesters+2; i++ {
		r29aFetch(led, i, int64(4096*(i+1)))
	}
	if b, w := r29aKeys(r29aStatusAs(t, on, "127.0.0.1:8080", "", "", "")); b || !w {
		t.Fatalf("instrument ON, untokened: block=%v withheld=%v, want the marker alone — the block went to a reader the code has not established is the operator ", b, w)
	}
	top := r29aStatusAs(t, on, "127.0.0.1:8080", "", "Bearer tok", "")
	if b, w := r29aKeys(top); !b || w {
		t.Fatalf("instrument ON, operator: block=%v withheld=%v, want the block alone", b, w)
	}
	// And the block the operator gets is the real one, not a zero value.
	var block map[string]any
	if err := json.Unmarshal(top["bBootstrap"], &block); err != nil {
		t.Fatal(err)
	}
	if block["ageAxisLive"] != true || block["suppressed"] != false {
		t.Fatalf("operator's block is not the live census: %s", top["bBootstrap"])
	}
}

// TestQueryTokenDoesNotUnlockTheBlock is S3: the block's predicate is the
// Authorization HEADER only. The same token in the URL query — which the mutation guard
// accepts, and which lands in access logs, proxy logs, Referer headers and browser history
// — unlocks the F2 per-object detail (unchanged) but NOT the histogram.
func TestQueryTokenDoesNotUnlockTheBlock(t *testing.T) {
	s, led, _ := r29aServer(t, true)
	for i := 0; i < credit.BBootstrapMinRequesters+2; i++ {
		r29aFetch(led, i, 8192)
	}
	top := r29aStatusAs(t, s, "127.0.0.1:8080", "", "", "?token=tok")
	if b, w := r29aKeys(top); b || !w {
		t.Fatalf("?token= in the URL unlocked the block (block=%v withheld=%v): a URL secret cannot be the operator predicate (Red-team F9)", b, w)
	}
	var d map[string]any
	if err := json.Unmarshal(top["durability"], &d); err != nil {
		t.Fatal(err)
	}
	if d["detailWithheld"] == true {
		t.Fatalf("the F2 durability withhold changed behaviour under a query token; this gate is about the block only and the query token must still satisfy validToken: %s", top["durability"])
	}
}

// TestProxyUpstreamHostAndLocalOriginReadWithheld encodes the F5 and the
// reflected-localhost-origin shape as permanent gates. A request that passes the Host
// guard because a reverse proxy forwarded its upstream's loopback authority, and a
// cross-origin GET from a localhost page (the observatory, or any http://localhost:* page
// the guard reflects), both arrive without the header token and both read the marker.
func TestProxyUpstreamHostAndLocalOriginReadWithheld(t *testing.T) {
	s, led, _ := r29aServer(t, true)
	for i := 0; i < credit.BBootstrapMinRequesters+2; i++ {
		r29aFetch(led, i, 8192)
	}
	for _, host := range []string{"127.0.0.1:9000", "localhost:9000", "127.0.0.2:9000", "[::1]:9000"} {
		if b, w := r29aKeys(r29aStatusAs(t, s, host, "", "", "")); b || !w {
			t.Fatalf("upstream Host %q read the block (block=%v withheld=%v) — a reverse proxy in front of a loopback bind is the standard production shape (Red-team F5)", host, b, w)
		}
	}
	for _, origin := range []string{"http://localhost:8082", "http://127.0.0.1:8083"} {
		if b, w := r29aKeys(r29aStatusAs(t, s, "127.0.0.1:8080", origin, "", "")); b || !w {
			t.Fatalf("cross-origin GET from %q read the block (block=%v withheld=%v) — the guard reflects localhost origins so the observatory works; that must not extend to the census", origin, b, w)
		}
	}
}

// TestWithholdingDoesNotPoisonTheSharedCache is S2's non-obvious constraint: the
// cache holds ONE full document and the withhold is applied to a COPY. An untokened read
// and then an operator read inside the SAME snapshot interval must give the operator the
// block; a clause that cleared the cached pointer in place would withhold it from the
// operator until the next recompute.
func TestWithholdingDoesNotPoisonTheSharedCache(t *testing.T) {
	s, led := statusServer(t)
	bbootstrapWireUI(s, true)
	clk := &r29aClock{}
	led.SetObservabilityClock(clk, clk.monotonic)
	for i := 0; i < credit.BBootstrapMinRequesters+2; i++ {
		r29aFetch(led, i, 8192)
	}
	fixed := s.started.Add(statusSnapshotInterval) // ONE instant: every read below shares one snapshot
	s.now = func() time.Time { return fixed }

	if b, w := r29aKeys(r29aStatusAs(t, s, "127.0.0.1:8080", "", "", "")); b || !w {
		t.Fatalf("untokened first read: block=%v withheld=%v", b, w)
	}
	if b, w := r29aKeys(r29aStatusAs(t, s, "127.0.0.1:8080", "", "Bearer tok", "")); !b || w {
		t.Fatalf("operator read inside the same interval got block=%v withheld=%v — the withhold mutated the cached document (S2)", b, w)
	}
	if b, w := r29aKeys(r29aStatusAs(t, s, "127.0.0.1:8080", "", "", "")); b || !w {
		t.Fatalf("untokened read after the operator's: block=%v withheld=%v", b, w)
	}
	if s.statusDoc == nil || s.statusDoc.BBootstrap == nil {
		t.Fatalf("the cached document lost its block: the withhold must assign into the copy, never clear the cache")
	}
}

// TestInsecureTokenFileIsRefusedAtStartup is S5: with -bbootstrap set, a token
// file readable by group or other refuses the start, because "the reader can read the
// 0600 token" is the premise the whole (b) half rests on. A missing file is fine (it is
// about to be created 0600); with the flag off nothing is checked. The daemon's call
// ORDER (before the file is read) is the source gate
// TestDaemonRefusesBeforeItBindsOrMintsAToken.
func TestInsecureTokenFileIsRefusedAtStartup(t *testing.T) {
	dir := t.TempDir()
	if err := bbootstrapRefuseInsecureTokenFile(dir, true); err != nil {
		t.Fatalf("missing token file refused: %v", err)
	}
	path := filepath.Join(dir, "ui-token")
	for _, mode := range []os.FileMode{0o644, 0o640, 0o604, 0o660, 0o666, 0o400 | 0o040} {
		if err := os.WriteFile(path, []byte("tok"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(path, mode); err != nil {
			t.Fatal(err)
		}
		err := bbootstrapRefuseInsecureTokenFile(dir, true)
		if err == nil {
			t.Fatalf("ui-token mode %04o was ACCEPTED with -bbootstrap: every local user who can read it is now 'the operator' ", mode)
		}
		if !strings.Contains(err.Error(), "chmod 0600") || !strings.Contains(err.Error(), path) {
			t.Fatalf("refusal for mode %04o does not name the file and the fix: %v", mode, err)
		}
		if err := bbootstrapRefuseInsecureTokenFile(dir, false); err != nil {
			t.Fatalf("mode %04o refused with the flag OFF: %v", mode, err)
		}
	}
	for _, mode := range []os.FileMode{0o600, 0o400} {
		if err := os.Chmod(path, mode); err != nil {
			t.Fatal(err)
		}
		if err := bbootstrapRefuseInsecureTokenFile(dir, true); err != nil {
			t.Fatalf("owner-only mode %04o refused: %v", mode, err)
		}
	}
}

// TestReaderViewIsTheOneCompositionPoint is a source gate on the required shape:
// every serve-time withhold on GET /api/status is a clause inside uiServer.readerView,
// and apiStatus itself rewrites nothing but the snapshot age. When lands it adds a
// clause there, not a second rewrite in the handler.
//
// RUNTIME GATE: TestThreeWireStatesAreDistinctKeySets and
// TestWithholdingDoesNotPoisonTheSharedCache observe the withholds' behaviour;
// TestStatusWithholdsPerObjectDetailWithoutAToken observes the F2 clause. This
// source gate adds only WHERE the clauses live.
func TestReaderViewIsTheOneCompositionPoint(t *testing.T) {
	src, err := os.ReadFile("ui.go")
	if err != nil {
		t.Fatal(err)
	}
	s := string(src)
	start := strings.Index(s, "func (s *uiServer) apiStatus(")
	if start < 0 {
		t.Fatal("SOURCE GATE: the literal func (s *uiServer) apiStatus( is absent from ui.go")
	}
	body := s[start:]
	body = body[:strings.Index(body, "\n}\n")]
	for _, forbidden := range []string{"withheldDurability(", "withholdBBootstrap(", "validToken(", "Durability ="} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("SOURCE GATE: apiStatus contains %q — a withhold outside readerView. Every serve-time withhold is one clause in readerView (one composition point, N clauses; never N rewrites)", forbidden)
		}
	}
	if !strings.Contains(body, "s.readerView(doc, r)") {
		t.Fatalf("SOURCE GATE: apiStatus no longer routes the document through s.readerView(doc, r)")
	}
	view := s[strings.Index(s, "func (s *uiServer) readerView("):]
	view = view[:strings.Index(view, "\n}\n")]
	for _, clause := range []string{"withheldDurability(doc.Durability)", "withholdBBootstrap(&out.statusExtras, auth.tokenHeader)"} {
		if !strings.Contains(view, clause) {
			t.Fatalf("SOURCE GATE: readerView lost the clause %q", clause)
		}
	}
}

// fakeOwnedFile is an os.FileInfo whose Sys reports a chosen owner, so the ownership
// clause can be exercised without root (a real chown to another uid is not available to
// an unprivileged test).
type fakeOwnedFile struct {
	os.FileInfo
	uid uint32
}

func (f fakeOwnedFile) Sys() any { return &syscall.Stat_t{Uid: f.uid} }

// TestTokenFileMustBeTheDaemonUsers is the ownership clause: a token file that is 0600 but
// belongs to ANOTHER user — pre-planted in a shared or bind-mounted store — is refused,
// because loadOrCreateUIToken would adopt its value and that user would then hold the
// operator predicate.
func TestTokenFileMustBeTheDaemonUsers(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ui-token")
	if err := os.WriteFile(path, []byte("tok"), 0o600); err != nil {
		t.Fatal(err)
	}
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if why := bbootstrapTokenFileIssue(fi, os.Geteuid()); why != "" {
		t.Fatalf("the daemon user's own 0600 token was refused: %s", why)
	}
	other := fakeOwnedFile{FileInfo: fi, uid: uint32(os.Geteuid()) + 1}
	why := bbootstrapTokenFileIssue(other, os.Geteuid())
	if why == "" {
		t.Fatalf("a 0600 token file owned by ANOTHER uid was accepted: whoever planted it holds the operator predicate ")
	}
	if !strings.Contains(why, "owned by uid") {
		t.Fatalf("the ownership refusal does not say whose file it is: %s", why)
	}
	// Mode is checked first: a world-readable file names the mode, whoever owns it.
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
	fi, _ = os.Stat(path)
	if why := bbootstrapTokenFileIssue(fakeOwnedFile{FileInfo: fi, uid: uint32(os.Geteuid()) + 1}, os.Geteuid()); !strings.Contains(why, "mode 0644") {
		t.Fatalf("mode should be reported before ownership: %s", why)
	}
	// And the live refusal wraps the predicate with the fix on the same line.
	if err := os.Chmod(path, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := bbootstrapRefuseInsecureTokenFile(dir, true); err != nil {
		t.Fatalf("own 0600 file refused through the live path: %v", err)
	}
}
