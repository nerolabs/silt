package credit

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// G-FP2-0 — the scope-close pin for D-FP2-SCOPE (owner-ratified 2026-09-04,
// docs/decisions.md). The credit ledger stays EPHEMERAL through the release
// candidate. That is a POSTURE, not an accident, and this is what tests it.
//
// The decision re-arms FP-1, FP-2 and R-F8-RESTART-REWIND automatically on the
// FIRST of three triggers. This gate fails when any of them arrives, so the
// re-arm cannot happen silently:
//
//	T1 · a durable store is attached to credit.Ledger beyond the paid-serial guard
//	T2 · the R2.4 economy-ON default flip (cmd/silt -economy defaults true)
//	T3 · any PR that persists a balance the ledger reads
//
// A shared or multi-operator ledger is the fourth trigger and is NOT mechanically
// checkable from source; it is named here so a reader knows the gate does not
// cover it. When any arm fires, the fix is NOT to narrow this gate. It is to land
// the FP-2 obligation set (§4.7 of the certification) in the SAME PR:
// provisional + provOrder/provIndex/provHead; epochWatermark + sweptEpoch under
// T-W; l.order; FP-1 Bank.spent; a certified account/escrow cap and eviction
// rule (build-immutable #8); and the exclusion of the standing fields.
//
// Certification and the obligation table:
// /Users/andrewedmond/Claude/claude/silt-reviews/research/research-outcome/FP-2-redeem-atom-and-ledger-durability-OWNER-BRIEF-AND-CERTIFICATION-2026-09-04.md
//
// SOURCE GATE: this test reads Go source text under core/credit and cmd/silt. It
// checks the DECLARED store surface of credit.Ledger, the DECLARED default of the
// -economy flag, and the argument text of durable-append call sites. It observes
// no runtime behaviour and proves no durability property.
//
// RUNTIME GATE: TestFP2_CrashBetweenTheGuardAppendAndThePayBurnsTheReceipt
// (r04b_c3_crashwindow_test.go) is the behavioural cover — it MEASURES the
// residual the ephemeral posture accepts (58,720,256 at the crash window). If a
// store lands and this gate is made green by narrowing rather than by the
// obligation set, that runtime test is where the loss reappears.

// gfp20Findings is what one source scan flagged, keyed by arm.
type gfp20Findings struct {
	storeFields  []string // Ledger struct fields whose type names a durable store
	storeSetters []string // methods on *Ledger that attach a store
	balanceAppnd []string // durable-append call sites whose arguments mention a balance
}

// gfp20DurableStoreType reports whether a type expression names a durable store
// port. The ledger's clock source (ports.EpochSource) is deliberately NOT one:
// a clock carries no state across a restart.
func gfp20DurableStoreType(expr ast.Expr) bool {
	var b strings.Builder
	var walk func(ast.Expr)
	walk = func(e ast.Expr) {
		switch t := e.(type) {
		case *ast.Ident:
			b.WriteString(t.Name)
		case *ast.SelectorExpr:
			walk(t.X)
			b.WriteString(".")
			b.WriteString(t.Sel.Name)
		case *ast.StarExpr:
			walk(t.X)
		}
	}
	walk(expr)
	return strings.HasSuffix(b.String(), "Store")
}

// gfp20ScanLedger walks one parsed file and records the three arms.
func gfp20ScanLedger(fset *token.FileSet, af *ast.File, src []byte) gfp20Findings {
	var f gfp20Findings
	ast.Inspect(af, func(n ast.Node) bool {
		switch d := n.(type) {
		case *ast.TypeSpec:
			if d.Name.Name != "Ledger" {
				return true
			}
			st, ok := d.Type.(*ast.StructType)
			if !ok {
				return true
			}
			for _, fld := range st.Fields.List {
				if !gfp20DurableStoreType(fld.Type) {
					continue
				}
				for _, nm := range fld.Names {
					f.storeFields = append(f.storeFields, nm.Name)
				}
			}
		case *ast.FuncDecl:
			if d.Recv == nil || len(d.Recv.List) == 0 {
				return true
			}
			recv := d.Recv.List[0].Type
			if star, ok := recv.(*ast.StarExpr); ok {
				recv = star.X
			}
			id, ok := recv.(*ast.Ident)
			if !ok || id.Name != "Ledger" {
				return true
			}
			if !strings.HasPrefix(d.Name.Name, "Set") {
				return true
			}
			if d.Type.Params == nil {
				return true
			}
			for _, p := range d.Type.Params.List {
				if gfp20DurableStoreType(p.Type) {
					f.storeSetters = append(f.storeSetters, d.Name.Name)
					break
				}
			}
		case *ast.CallExpr:
			sel, ok := d.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			switch sel.Sel.Name {
			case "Append", "Put", "Write", "Save":
			default:
				return true
			}
			for _, arg := range d.Args {
				pos := fset.Position(arg.Pos()).Offset
				end := fset.Position(arg.End()).Offset
				if pos < 0 || end > len(src) || end <= pos {
					continue
				}
				text := string(src[pos:end])
				low := strings.ToLower(text)
				if strings.Contains(low, "balance") || strings.Contains(low, "funded") {
					f.balanceAppnd = append(f.balanceAppnd,
						sel.Sel.Name+"("+text+")")
				}
			}
		}
		return true
	})
	return f
}

func gfp20RepoRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("SOURCE GATE: cannot resolve the repo root: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		t.Fatalf("SOURCE GATE: repo root not found at %s (no go.mod): %v", root, err)
	}
	return root
}

// TestGFP20_TheLedgerStaysEphemeralUnderDFP2Scope is arms T1 and T3.
func TestGFP20_TheLedgerStaysEphemeralUnderDFP2Scope(t *testing.T) {
	root := gfp20RepoRoot(t)
	dir := filepath.Join(root, "core", "credit")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("SOURCE GATE: cannot read core/credit: %v", err)
	}
	fset := token.NewFileSet()
	var all gfp20Findings
	scanned := 0
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		path := filepath.Join(dir, name)
		src, rErr := os.ReadFile(path)
		if rErr != nil {
			t.Fatalf("SOURCE GATE: cannot read %s: %v", name, rErr)
		}
		af, pErr := parser.ParseFile(fset, path, src, parser.ParseComments)
		if pErr != nil {
			t.Fatalf("SOURCE GATE: cannot parse %s: %v", name, pErr)
		}
		f := gfp20ScanLedger(fset, af, src)
		all.storeFields = append(all.storeFields, f.storeFields...)
		all.storeSetters = append(all.storeSetters, f.storeSetters...)
		all.balanceAppnd = append(all.balanceAppnd, f.balanceAppnd...)
		scanned++
	}
	if scanned < 5 {
		t.Fatalf("SOURCE GATE: GATE VACUOUS — only %d non-test .go files scanned under core/credit; "+
			"the gate cannot have looked at the Ledger", scanned)
	}

	// The ONE durable store D-FP2-SCOPE permits: the paid-serial guard. It stores
	// SERIALS (a spend guard), never a balance, so it does not persist ledger state.
	const wantField = "paidStore"
	const wantSetter = "SetPaidSerialStore"

	sort.Strings(all.storeFields)
	sort.Strings(all.storeSetters)

	if got := strings.Join(all.storeFields, ","); got != wantField {
		t.Fatalf("SOURCE GATE: T1 — credit.Ledger's durable-store FIELD set is %q, want exactly %q.\n"+
			"D-FP2-SCOPE keeps the ledger EPHEMERAL through the RC. A new store field re-arms FP-1, FP-2 "+
			"and R-F8-RESTART-REWIND. Do NOT narrow this gate: land the FP-2 obligation set in the SAME PR "+
			"(provisional + provOrder/provIndex/provHead; epochWatermark + sweptEpoch under T-W; l.order; "+
			"FP-1 Bank.spent; a certified account/escrow cap and eviction rule; the standing fields EXCLUDED). "+
			"Route the change to the Researcher — the eviction rule is build-immutable #8 and must be "+
			"certified first. Brief: FP-2-redeem-atom-and-ledger-durability-OWNER-BRIEF-AND-CERTIFICATION-2026-09-04.md",
			got, wantField)
	}
	if got := strings.Join(all.storeSetters, ","); got != wantSetter {
		t.Fatalf("SOURCE GATE: T1 — the *Ledger store-ATTACH method set is %q, want exactly %q. "+
			"Same obligation set as the field arm above; see D-FP2-SCOPE.", got, wantSetter)
	}
	if len(all.balanceAppnd) != 0 {
		t.Fatalf("SOURCE GATE: T3 — %d durable-append call site(s) under core/credit pass an argument whose "+
			"text names a balance: %s.\nD-FP2-SCOPE forbids persisting any balance the ledger reads until the "+
			"obligation set lands. If this is a false positive, the fix is to rename the argument, never to "+
			"drop the arm.", len(all.balanceAppnd), strings.Join(all.balanceAppnd, "; "))
	}
}

// TestGFP20_TheEconomyDefaultStaysOff is arm T2. The R2.4 economy-ON default flip
// is the first named re-arm trigger in D-FP2-SCOPE.
func TestGFP20_TheEconomyDefaultStaysOff(t *testing.T) {
	root := gfp20RepoRoot(t)
	path := filepath.Join(root, "cmd", "silt", "daemon.go")
	src, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("SOURCE GATE: cannot read cmd/silt/daemon.go: %v", err)
	}
	fset := token.NewFileSet()
	af, err := parser.ParseFile(fset, path, src, parser.ParseComments)
	if err != nil {
		t.Fatalf("SOURCE GATE: cannot parse cmd/silt/daemon.go: %v", err)
	}
	found := false
	bad := ""
	ast.Inspect(af, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "Bool" || len(call.Args) < 2 {
			return true
		}
		lit, ok := call.Args[0].(*ast.BasicLit)
		if !ok || lit.Value != `"economy"` {
			return true
		}
		found = true
		def, ok := call.Args[1].(*ast.Ident)
		if !ok || def.Name != "false" {
			pos := fset.Position(call.Args[1].Pos())
			bad = pos.String()
		}
		return true
	})
	if !found {
		t.Fatalf("SOURCE GATE: GATE VACUOUS — no fs.Bool(\"economy\", …) declaration found in " +
			"cmd/silt/daemon.go. The flag was renamed or removed; re-point this arm rather than deleting it.")
	}
	if bad != "" {
		t.Fatalf("SOURCE GATE: T2 — the -economy flag no longer defaults to false (%s). That is the R2.4 "+
			"economy-ON default flip, the FIRST re-arm trigger named by D-FP2-SCOPE: FP-1, FP-2 and "+
			"R-F8-RESTART-REWIND are live again and the ledger's ephemerality is no longer an accepted cost. "+
			"Land the FP-2 obligation set before flipping this default.", bad)
	}
}

// TestGFP20_TheGateHasTeeth injects a synthetic file carrying each violation and
// asserts the scanner flags all three. Without this, a scanner that silently
// stopped matching would read as a green posture.
func TestGFP20_TheGateHasTeeth(t *testing.T) {
	const injected = `package credit

type Ledger struct {
	paidStore    ports.PaidSerialStore
	balanceStore ports.BalanceStore
	epochSrc     ports.EpochSource
	order        []string
}

func (l *Ledger) SetBalanceStore(s ports.BalanceStore) { l.balanceStore = s }
func (l *Ledger) SetEpochSource(src ports.EpochSource) { l.epochSrc = src }

func (l *Ledger) persist(a *account) {
	l.balanceStore.Append(a.balance)
	l.paidStore.Append(serialKey)
}
`
	fset := token.NewFileSet()
	af, err := parser.ParseFile(fset, "x.go", injected, parser.ParseComments)
	if err != nil {
		t.Fatal(err)
	}
	f := gfp20ScanLedger(fset, af, []byte(injected))

	fields := strings.Join(f.storeFields, ",")
	if !strings.Contains(fields, "balanceStore") {
		t.Fatalf("SOURCE GATE: GATE HAS NO TEETH — a second store FIELD was not flagged; flagged: %q", fields)
	}
	// The clock source must NOT be flagged as a durable store.
	if strings.Contains(fields, "epochSrc") {
		t.Fatalf("SOURCE GATE: GATE IS OVER-BROAD — epochSrc (a clock, not a store) was flagged; flagged: %q", fields)
	}
	setters := strings.Join(f.storeSetters, ",")
	if !strings.Contains(setters, "SetBalanceStore") {
		t.Fatalf("SOURCE GATE: GATE HAS NO TEETH — a store-attach METHOD was not flagged; flagged: %q", setters)
	}
	if strings.Contains(setters, "SetEpochSource") {
		t.Fatalf("SOURCE GATE: GATE IS OVER-BROAD — SetEpochSource (a clock) was flagged; flagged: %q", setters)
	}
	appends := strings.Join(f.balanceAppnd, ";")
	if !strings.Contains(appends, "a.balance") {
		t.Fatalf("SOURCE GATE: GATE HAS NO TEETH — a balance-persisting APPEND was not flagged; flagged: %q", appends)
	}
	if strings.Contains(appends, "serialKey") {
		t.Fatalf("SOURCE GATE: GATE IS OVER-BROAD — the serial-guard append was flagged as a balance; flagged: %q", appends)
	}
}
