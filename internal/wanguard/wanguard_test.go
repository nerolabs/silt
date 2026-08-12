// Package wanguard enforces build-immutable #5 ("build for the adverse
// internet — durability is the default") with a test, so a flat transport
// deadline on a WAN path is a red build, not a code-review hope.
//
// The rule silt keeps losing days to (#286, #288): a wall-clock number is a
// TRANSPORT measurement (RTT + jitter + transfer time), never a security or
// correctness signal, and a *flat* one on a payload-bearing path is a category
// error (docs/network-durability.md §1). The durable policy already exists —
// core/node's requestAttempt: size-aware deadline + retry/backoff + evict-on-
// exhaustion + negative cache. New WAN code should route through it.
//
// This guard does NOT try to auto-classify "payload-bearing" (impossible
// statically). Instead it keeps a LEDGER: every low-level transport-deadline
// construct (net.Dialer{Timeout}, http.Client{Timeout}, http.Server timeouts,
// conn.Set*Deadline) in adapters/, core/, and cmd/ must be a registered site
// with a declared shape and a one-line rationale. A NEW deadline that isn't in
// the ledger fails the build — forcing the author to either route it through
// the durable-WAN policy or consciously declare its intent (payload-scaled /
// fail-fast / keepalive-idle / server-DoS-bound / first-contact-modest). A
// STALE ledger entry (registered but no longer present) also fails, so the
// ledger can't rot. This is the #329 regression guard: it can't fix a live
// violation (there are none as of the audit), but it stops the *next* #286.
//
// Deadline-CLEARING calls (conn.SetDeadline(time.Time{})) are not deadlines and
// are ignored. Test files are exempt.
package wanguard

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// scanDirs are the trees that carry silt's real network I/O. ports/ and sim/
// are pure (depcheck forbids net there); client/ has no transport deadlines.
var scanDirs = []string{"adapters", "core", "cmd"}

// site keys a transport-deadline construct stably across line edits:
// "<relpath>|<enclosing-func>|<kind>[#N]". Line numbers move; a func name +
// construct kind does not.
type siteKey string

// ledger is the sanctioned set. Each entry documents the site's DECLARED shape
// and why a flat/modest deadline is correct there. Adding a WAN deadline means
// adding a line here (and justifying it) — or routing through the durable policy
// so no raw construct appears at all. Keys are discovered by the finder; run the
// test and it prints any unregistered site verbatim for pasting.
var ledger = map[siteKey]string{
	// --- httpregistry ---------------------------------------------------------
	// Client GETs are wrapped in doGetRetry (bounded exp backoff, #334) and the
	// publish POST is async 202+poll (#328), so this 10s bounds only small,
	// idempotent, retried requests. The ~1.5 MB bond payload never crosses here
	// — it rides the size-aware consensus RPC (core/node). Small-payload +
	// retry-wrapped.
	"adapters/httpregistry/httpregistry.go|NewClient|http.Client.Timeout":       "small-payload + retry-wrapped (GETs via doGetRetry #334, publish async #328); the 1.5MB bond rides the size-aware consensus RPC, not this client",
	"adapters/httpregistry/httpregistry.go|NewPinnedClient|http.Client.Timeout": "small-payload + retry-wrapped (pinned variant of NewClient); same rationale",
	// Server-side receive/write/idle bounds protect a cheap public registry from
	// slow-loris / resource exhaustion (S6 keep-registries-cheap). Bodies are
	// small (manifest + token); this is DoS defense, not a client-durability path.
	"adapters/httpregistry/httpregistry.go|serve|http.Server.ReadHeaderTimeout": "server-DoS-bound — slow-loris / exhaustion defense for a cheap registry (S6); small request bodies",

	// --- relay (data plane) ---------------------------------------------------
	// opTimeout (10s) bounds a single control connect/accept exchange; the
	// deadline is CLEARED (SetDeadline(time.Time{})) before the bulk splice, so
	// it never bounds payload transfer. ctrlIdle (90s) is a keepalive-idle reaper.
	// The relay control conn itself reconnects with exp backoff + pinger (durable).
	"adapters/relay/client.go|DialThrough|conn.SetDeadline":  "op-bound — one relay connect exchange (opTimeout 10s); deadline cleared before the splice, never bounds payload",
	"adapters/relay/client.go|acceptStream|conn.SetDeadline": "op-bound — one relay accept exchange (opTimeout); cleared before splice",
	"adapters/relay/client.go|dialRelay|net.Dialer.Timeout":  "first-contact-modest — dial to the relay (5s); the control conn reconnects with exp backoff + keepalive (client.Run)",
	"adapters/relay/client.go|session|conn.SetDeadline":      "op-bound — control op exchange (opTimeout)",
	"adapters/relay/client.go|session|conn.SetDeadline#2":    "keepalive-idle — control-conn idle reaper (ctrlIdle 90s)",
	"adapters/relay/server.go|handle|conn.SetDeadline":       "op-bound — one relay connect/accept (opTimeout); cleared before splice",
	"adapters/relay/server.go|serveControl|conn.SetDeadline": "keepalive-idle — control-conn idle reaper (ctrlIdle 90s)",

	// --- tcpnet / NAT ---------------------------------------------------------
	// Best-effort NAT hole-punch: a tight fixed cadence is intentional (both
	// sides fire SYNs to cross); it falls back to the relay on failure, so it is
	// never on the consensus-critical path. Fail-fast by design.
	"adapters/tcpnet/holepunch.go|HolePunch|net.Dialer.Timeout": "fail-fast/best-effort — NAT hole-punch SYN storm (3s×15); falls back to relay, never consensus-critical",
	// Direct-peer dial: a MODEST first-contact deadline is the CORRECT reading of
	// network-durability §1 (the instinct to raise 2s→10s is the named trap). It
	// nests below RequestTimeout; the upper layer (core/node requestAttempt)
	// retries; and the consensus set is never-evicted (persistent-peers #331).
	// #332 confirmed the observed EOFs were µs teardowns, not this deadline firing.
	"adapters/tcpnet/tcpnet.go|dialPeer|net.Dialer.Timeout": "first-contact-modest — direct-peer dial (2s, nested < RequestTimeout); upper-layer requestAttempt retries; consensus set never-evicted (#331); §1-correct, not a flat steady-state constant",
	"adapters/tcpnet/tcpnet.go|dialPeer|conn.SetDeadline":   "first-contact-modest — relayed-peer TLS handshake (5s); cleared after handshake; upper-layer retries",
}

func TestTransportDeadlinesAreLedgered(t *testing.T) {
	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	found := map[siteKey]string{} // key -> "relpath:line" for diagnostics
	for _, dir := range scanDirs {
		root := filepath.Join(repoRoot, dir)
		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			fset := token.NewFileSet()
			f, err := parser.ParseFile(fset, path, nil, 0)
			if err != nil {
				return err
			}
			rel, _ := filepath.Rel(repoRoot, path)
			for _, s := range findSites(f) {
				key := siteKey(fmt.Sprintf("%s|%s|%s", rel, s.fn, s.kind))
				found[key] = fmt.Sprintf("%s:%d", rel, fset.Position(s.pos).Line)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}

	// Unregistered: a transport deadline with no ledger entry.
	var unregistered []siteKey
	for k := range found {
		if _, ok := ledger[k]; !ok {
			unregistered = append(unregistered, k)
		}
	}
	sort.Slice(unregistered, func(i, j int) bool { return unregistered[i] < unregistered[j] })
	for _, k := range unregistered {
		t.Errorf("unregistered transport deadline at %s\n  site: %q\n  → route it through the durable-WAN policy (core/node requestAttempt: size-aware + retry + negative-cache), OR add a wanguard ledger entry declaring its shape (payload-scaled / fail-fast / keepalive-idle / server-DoS-bound / first-contact-modest) and why a flat deadline is correct there (docs/network-durability.md, build-immutable #5).", found[k], k)
	}

	// Stale: a ledger entry whose site is gone — keep the ledger honest.
	for k := range ledger {
		if _, ok := found[k]; !ok {
			t.Errorf("stale wanguard ledger entry (site no longer exists): %q — remove it", k)
		}
	}
}

// TestFinderHasTeeth proves the finder actually detects the constructs it
// guards and correctly ignores deadline-clears — so the ledger check above can
// never silently degrade into a no-op that greenlights a real flat deadline.
func TestFinderHasTeeth(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want []string // expected "fn|kind" of detected sites
	}{
		{
			name: "net.Dialer timeout is caught",
			src:  `package p; import ("net";"time"); func f(){ _ = &net.Dialer{Timeout: 2*time.Second} }`,
			want: []string{"f|net.Dialer.Timeout"},
		},
		{
			name: "http.Client timeout is caught",
			src:  `package p; import ("net/http";"time"); func f(){ _ = &http.Client{Timeout: time.Second} }`,
			want: []string{"f|http.Client.Timeout"},
		},
		{
			name: "http.Server timeouts are caught",
			src:  `package p; import ("net/http";"time"); func f(){ _ = &http.Server{ReadTimeout: time.Second} }`,
			want: []string{"f|http.Server.ReadTimeout"},
		},
		{
			name: "SetDeadline arming is caught",
			src:  `package p; import ("net";"time"); func f(c net.Conn){ c.SetDeadline(time.Now().Add(time.Second)) }`,
			want: []string{"f|conn.SetDeadline"},
		},
		{
			name: "SetDeadline clearing is ignored",
			src:  `package p; import ("net";"time"); func f(c net.Conn){ c.SetDeadline(time.Time{}) }`,
			want: nil,
		},
		{
			name: "a non-transport composite/call is ignored",
			src:  `package p; import "time"; type T struct{Timeout time.Duration}; func f(){ _ = T{Timeout: time.Second} }`,
			want: nil,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fset := token.NewFileSet()
			f, err := parser.ParseFile(fset, "x.go", tc.src, 0)
			if err != nil {
				t.Fatal(err)
			}
			var got []string
			for _, s := range findSites(f) {
				got = append(got, s.fn+"|"+s.kind)
			}
			if strings.Join(got, ",") != strings.Join(tc.want, ",") {
				t.Errorf("findSites = %v, want %v", got, tc.want)
			}
		})
	}
}

type site struct {
	fn   string // enclosing function name
	kind string // construct kind, e.g. "net.Dialer.Timeout", "conn.SetDeadline"
	pos  token.Pos
}

// findSites walks a file's function bodies for low-level transport-deadline
// constructs. Deadline-clearing calls (arg is time.Time{}) are skipped.
func findSites(f *ast.File) []site {
	var sites []site
	counts := map[string]int{} // disambiguate multiple same-kind constructs in one func
	add := func(fn, kind string, pos token.Pos) {
		ck := fn + "|" + kind
		counts[ck]++
		if counts[ck] > 1 {
			kind = fmt.Sprintf("%s#%d", kind, counts[ck])
		}
		sites = append(sites, site{fn: fn, kind: kind, pos: pos})
	}
	for _, decl := range f.Decls {
		fd, ok := decl.(*ast.FuncDecl)
		if !ok {
			continue
		}
		fn := fd.Name.Name
		ast.Inspect(fd, func(n ast.Node) bool {
			switch x := n.(type) {
			case *ast.CompositeLit:
				if kind := compositeDeadlineKind(x); kind != "" {
					add(fn, kind, x.Pos())
				}
			case *ast.CallExpr:
				if kind := setDeadlineKind(x); kind != "" {
					add(fn, kind, x.Pos())
				}
			}
			return true
		})
	}
	return sites
}

// compositeDeadlineKind returns e.g. "net.Dialer.Timeout" if the composite
// literal is a transport type configured with a timeout field, else "".
func compositeDeadlineKind(x *ast.CompositeLit) string {
	sel, ok := x.Type.(*ast.SelectorExpr)
	if !ok {
		return ""
	}
	pkg, ok := sel.X.(*ast.Ident)
	if !ok {
		return ""
	}
	typ := pkg.Name + "." + sel.Sel.Name
	timeoutFields := map[string][]string{
		"net.Dialer":  {"Timeout"},
		"http.Client": {"Timeout"},
		"http.Server": {"ReadHeaderTimeout", "ReadTimeout", "WriteTimeout", "IdleTimeout"},
	}
	fields, watched := timeoutFields[typ]
	if !watched {
		return ""
	}
	for _, el := range x.Elts {
		kv, ok := el.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		key, ok := kv.Key.(*ast.Ident)
		if !ok {
			continue
		}
		for _, fname := range fields {
			if key.Name == fname {
				return typ + "." + fname
			}
		}
	}
	return ""
}

// setDeadlineKind returns "conn.<Method>" for a SetDeadline/SetReadDeadline/
// SetWriteDeadline call that ARMS a deadline, or "" for a non-deadline call or
// a deadline-clearing call (single arg == time.Time{}).
func setDeadlineKind(x *ast.CallExpr) string {
	sel, ok := x.Fun.(*ast.SelectorExpr)
	if !ok {
		return ""
	}
	switch sel.Sel.Name {
	case "SetDeadline", "SetReadDeadline", "SetWriteDeadline":
	default:
		return ""
	}
	if len(x.Args) == 1 && isZeroTime(x.Args[0]) {
		return "" // clearing a deadline, not setting one
	}
	return "conn." + sel.Sel.Name
}

// isZeroTime reports whether e is the composite literal time.Time{}.
func isZeroTime(e ast.Expr) bool {
	cl, ok := e.(*ast.CompositeLit)
	if !ok || len(cl.Elts) != 0 {
		return false
	}
	sel, ok := cl.Type.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	pkg, ok := sel.X.(*ast.Ident)
	return ok && pkg.Name == "time" && sel.Sel.Name == "Time"
}
