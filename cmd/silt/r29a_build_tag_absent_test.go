//go:build !bbootstrap

package main

// D-BB-BUILD-TAG (ratified 2026-09-05) — the daemon tier's default-build gates. Each one
// asserts an ABSENCE, so the file compiles only without the `bbootstrap` tag.

import (
	"encoding/json"
	"flag"
	"go/ast"
	"go/build"
	"go/parser"
	"go/token"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/nerolabs/silt/adapters/walltime"
	"github.com/nerolabs/silt/ports"
)

// TestR29aDefaultBuildHasNoBBootstrapFlag asserts a default silt binary does not
// RECOGNISE -bbootstrap. It drives the same seam daemon.go does, on a real FlagSet, so
// it observes the flag surface rather than reading source text.
//
// The consequence for an operator is `silt daemon -bbootstrap` failing at flag parse
// with "flag provided but not defined", which is the intended answer: the mechanism is
// not disabled, it is absent, and there is nothing to enable.
//
// THE SECOND ARM IS THE ONE THAT MATTERS, and it exists because the first one is not
// enough. Driving registerBBootstrapFlag on a synthetic FlagSet cannot see the daemon's
// OWN flag set: a blind review put `fs.Bool("bbootstrap", false, …)` straight into the
// untagged cmd/silt/daemon.go, and the default binary then declared and ACCEPTED
// -bbootstrap while this test stayed green. cmdDaemon builds its FlagSet inline with
// flag.ExitOnError, so no in-process test can parse it without exiting the test binary;
// the closest observation available is the default build's OWN FILE SET, which is what
// the scan below walks. It is the file set the linker sees, not a hand-written list —
// go/build applies the same build constraints, so bbootstrap.go is excluded and
// bbootstrap_off.go included, exactly as in `go build`.
//
// The CI containment step reads the linked binary itself (`daemon -bbootstrap` must be
// rejected), which is the only complete form; this gate is the part that runs in the
// ordinary test job.
func TestR29aDefaultBuildHasNoBBootstrapFlag(t *testing.T) {
	fs := flag.NewFlagSet("silt", flag.ContinueOnError)
	fs.SetOutput(new(strings.Builder))
	on := registerBBootstrapFlag(fs)
	if f := fs.Lookup("bbootstrap"); f != nil {
		t.Fatalf("-bbootstrap is declared in a DEFAULT build (default %q). The instrument compiles only under the `bbootstrap` build tag (D-BB-BUILD-TAG)", f.DefValue)
	}
	if on() {
		t.Fatalf("registerBBootstrapFlag reports the instrument ON in a default build; it must be permanently false")
	}
	if err := fs.Parse([]string{"-bbootstrap"}); err == nil {
		t.Fatalf("a default build ACCEPTED -bbootstrap. It must be rejected as an unknown flag")
	}

	// ARM 2: no file in the default build declares a flag whose name contains
	// "bbootstrap", wherever it lives and whichever flag type it uses.
	files, err := defaultBuildGoFiles(".")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) < 5 {
		t.Fatalf("the default build file set has only %d files (%v) — the scan below would be vacuous", len(files), files)
	}
	sawDaemon := false
	for _, f := range files {
		if filepath.Base(f) == "daemon.go" {
			sawDaemon = true
		}
		if filepath.Base(f) == "bbootstrap.go" {
			t.Fatalf("SOURCE GATE: cmd/silt/bbootstrap.go is in the DEFAULT build file set. It carries //go:build bbootstrap and must compile only under the tag")
		}
		for _, decl := range flagNamesDeclaredIn(t, f) {
			if strings.Contains(strings.ToLower(decl.name), "bbootstrap") {
				t.Fatalf("SOURCE GATE: %s declares the flag %q (via %s) and compiles in a DEFAULT build. That puts -bbootstrap — and every reference the instrument drags in behind it — into every shipped silt binary, which is exactly what D-BB-BUILD-TAG removed. Declare it in a file carrying //go:build bbootstrap", filepath.Base(f), decl.name, decl.method)
			}
		}
	}
	if !sawDaemon {
		t.Fatalf("SOURCE GATE: daemon.go is not in the default build file set (%v) — the flag scan must cover the file that builds the daemon's own FlagSet", files)
	}
}

// defaultBuildGoFiles returns the non-test .go files go/build selects for dir with NO
// build tags set — i.e. exactly the file set a plain `go build` compiles. This test file
// is itself //go:build !bbootstrap, so it only runs in that build.
func defaultBuildGoFiles(dir string) ([]string, error) {
	ctx := build.Default
	ctx.BuildTags = nil
	pkg, err := ctx.ImportDir(dir, 0)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(pkg.GoFiles))
	for _, f := range pkg.GoFiles {
		out = append(out, filepath.Join(dir, f))
	}
	return out, nil
}

// flagDecl is one flag declaration found in the source: the name it registers and the
// method that registered it, for the failure message.
type flagDecl struct{ name, method string }

// flagNamesDeclaredIn parses one Go file and returns every flag NAME it registers. It
// matches on the shape of the stdlib flag API — x.Bool("name", …), x.StringVar(&v,
// "name", …), flag.Duration("name", …) — so it catches a re-declaration under any flag
// type and in any file, not just the one literal a string search would find.
func flagNamesDeclaredIn(t *testing.T, path string) []flagDecl {
	t.Helper()
	// nameArg maps a flag-API method to the index of its flag-NAME argument. The Var
	// forms take the destination pointer first.
	nameArg := map[string]int{
		"Bool": 0, "Int": 0, "Int64": 0, "Uint": 0, "Uint64": 0,
		"String": 0, "Float64": 0, "Duration": 0, "Func": 0, "TextVar": 1,
		"BoolVar": 1, "IntVar": 1, "Int64Var": 1, "UintVar": 1, "Uint64Var": 1,
		"StringVar": 1, "Float64Var": 1, "DurationVar": 1, "Var": 1,
	}
	src, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	file, err := parser.ParseFile(token.NewFileSet(), path, src, 0)
	if err != nil {
		t.Fatal(err)
	}
	var out []flagDecl
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		i, ok := nameArg[sel.Sel.Name]
		if !ok || i >= len(call.Args) {
			return true
		}
		lit, ok := call.Args[i].(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			return true
		}
		out = append(out, flagDecl{name: strings.Trim(lit.Value, `"`), method: sel.Sel.Name})
		return true
	})
	return out
}

// TestR29aDaemonHandsTheNodeAWallClock pins the fact the D-BB-BUILD-TAG texts got
// BACKWARDS. Four sites said RecordBondChallenge's tick was "stamped from the bond
// auditor's own request counter rather than from a wall clock". It is a wall clock:
// core/node/bondaudit.go computes uint64(n.clock.Now())+1, and the clock the daemon
// hands the node is the walltime adapter, i.e. time.Now().UnixNano().
//
// The first-seen stamp that fact exposed (residual R-BB-BOND-STAMP-TUPLE) has since been
// DELETED under G-BB-28 — nothing read it. The unit still matters after the deletion:
// the same tick is kept as lastBondTick and DecayStale compares it against a nanosecond
// BondMaxAge, so a node clock that was not a wall clock would silently disable
// retention (core/credit's TestR29aRetentionReadsLastBondTickInNanoseconds).
//
// This gate covers the one link no behavioural test can see — that the daemon's node
// clock is the wall clock and not a sim clock — so it is a SOURCE gate and says so.
//
// RUNTIME GATE: TestR29aWalltimeAdapterIsAWallClock (below) runs the adapter this reads
// the name of; core/node's TestR29aBondAuditStampsAWallClockNanosecondNotACounter
// observes the stamp the auditor derives from it.
func TestR29aDaemonHandsTheNodeAWallClock(t *testing.T) {
	src, err := os.ReadFile("daemon.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, lit := range []string{"clk := walltime.New(loop)", "nd := node.New(id, cfg, clk,"} {
		if !strings.Contains(string(src), lit) {
			t.Fatalf("SOURCE GATE: daemon.go no longer contains the literal %q. This gate reads the daemon's clock wiring as text because no test in this repo boots the real daemon; if the wiring moved, re-anchor it here and re-check that lastBondTick is still fed a wall-clock nanosecond on a validator node (DecayStale's BondMaxAge comparison depends on it)", lit)
		}
	}
}

// TestR29aDefaultBuildStatusHasNoBBootstrapKey asserts on the BYTES /api/status emits,
// which is the boundary that matters — the same boundary BB-20 asserts the floor's
// property on, one build over.
//
// It drives real requester traffic first, so the absence is not the absence of a census:
// there IS a census on that ledger, and the default binary has no way to render it.
// statusExtras is an empty embedded struct here, so encoding/json promotes no field and
// the key cannot appear.
func TestR29aDefaultBuildStatusHasNoBBootstrapKey(t *testing.T) {
	s, _, _, led := economyServer(t, 0)
	s.peerCount = func() int { return 0 } // nil func in this fixture; /api/status segfaults without it
	server := ports.HashBytes([]byte("srv"))
	// Ten requesters: credit.BBootstrapMinRequesters does not exist in a default build
	// (it is tagged out with the rest of the instrument), so the count is a literal here
	// and its only job is to make the census non-trivial.
	for i := 0; i < 10; i++ {
		led.RecordServe(server, ports.HashBytes([]byte{byte(i), 0x29}), ports.Hash{}, 4096)
	}
	// statusExtras must be EMPTY, not merely un-populated. A field added to it in a
	// default build would put the key back on the wire the moment anything filled it,
	// and the body check below only catches the populated case.
	if n := reflect.TypeOf(statusExtras{}).NumField(); n != 0 {
		t.Fatalf("statusExtras has %d field(s) in a DEFAULT build, want 0. It is embedded in the /api/status payload, so any field here is a key a default silt binary can publish (D-BB-BUILD-TAG)", n)
	}
	if s.statusExtra != nil {
		t.Fatalf("uiServer.statusExtra is non-nil in a DEFAULT build — nothing can set it, because the only implementation is behind the `bbootstrap` build tag")
	}

	r := httptest.NewRequest("GET", "http://127.0.0.1:8080/api/status", nil)
	w := httptest.NewRecorder()
	s.guard(http.HandlerFunc(s.apiStatus)).ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatalf("status: %d, body %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if strings.Contains(body, "bBootstrap") {
		t.Fatalf("GET /api/status carries a bBootstrap key in a DEFAULT build: %s", body)
	}
	// And the payload is still well-formed with the empty extras embedded — an empty
	// embedded struct must contribute no key AND break nothing.
	var top map[string]json.RawMessage
	if err := json.Unmarshal([]byte(body), &top); err != nil {
		t.Fatalf("decode status: %v (body %s)", err, body)
	}
	if _, ok := top["id"]; !ok {
		t.Fatalf("status payload lost its id key — the embedded statusExtras seam broke the rest of the block: %s", body)
	}
}

// TestR29aWalltimeAdapterIsAWallClock is the runtime half: the adapter named in the
// source gate above genuinely reads the wall clock, at Unix-nanosecond magnitude. Without
// this, that gate would be asserting a type name and nothing about time.
func TestR29aWalltimeAdapterIsAWallClock(t *testing.T) {
	got := int64(walltime.New(nil).Now())
	if d := time.Now().UnixNano() - got; d < 0 || d > int64(10*time.Second) {
		t.Fatalf("walltime.Clock.Now() = %d, which is %d ns from time.Now().UnixNano() — the adapter the daemon hands the node must be a wall clock", got, d)
	}
	if got < 1_500_000_000_000_000_000 {
		t.Fatalf("walltime.Clock.Now() = %d, below Unix-nanosecond magnitude — a tick of this size would read as a counter, which is exactly the misreading that put a false fact in four D-BB-BUILD-TAG texts (R-BB-BOND-STAMP-TUPLE)", got)
	}
}
