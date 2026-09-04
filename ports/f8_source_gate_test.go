package ports

// R2.10 / F8 — G-F8-1, the SOURCE gate (Tester, 2026-09-04). Binding spec:
// silt-reviews/research/research-outcome/R2.10-F8-chain-anchored-epoch-RESEARCH-CERTIFICATION-2026-09-04.md
// §6 G-F8-1 (AMENDED: adds SpendRelayAnchors and the core/node deliveryReasoner
// twin), rule R-F8-SOURCE: "No ports.CreditLedger method takes an epoch."
//
// This gate parses SIGNATURES with go/parser. It sees parameter lists, never
// behaviour: it cannot tell whether the ledger READS an EpochSource, only that no
// caller can HAND it one. RUNTIME GATE: core/credit
// TestF8_FallingSourceLowersNothingAndReadmitsNothing_Delivery and _Relay (the
// ledger follows an injected source), core/node
// TestF8_LedgerFollowsItsSourceNotTheCaller (the node-tier dual).
//
// It lives in ports rather than core/credit so it still COMPILES while the credit
// signatures are in flight: the credit-tier gates call the new five-argument form
// and cannot build until the Builder lands it.

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
	"testing"
)

// f8Params flattens a parameter list to (name, type-text) pairs.
func f8Params(fl *ast.FieldList) [][2]string {
	var out [][2]string
	if fl == nil {
		return out
	}
	for _, f := range fl.List {
		ty := fmt.Sprintf("%s", f.Type)
		if id, ok := f.Type.(*ast.Ident); ok {
			ty = id.Name
		} else if sel, ok := f.Type.(*ast.SelectorExpr); ok {
			ty = fmt.Sprintf("%v.%s", sel.X, sel.Sel.Name)
		} else if arr, ok := f.Type.(*ast.ArrayType); ok {
			ty = "[]" + fmt.Sprintf("%v", arr.Elt)
		}
		if len(f.Names) == 0 {
			out = append(out, [2]string{"_", ty})
			continue
		}
		for _, n := range f.Names {
			out = append(out, [2]string{n.Name, ty})
		}
	}
	return out
}

// f8MethodParams finds method `name` on receiver type `recv` (a FuncDecl) or on
// interface `iface` (an InterfaceType field) in the parsed file.
func f8MethodParams(t *testing.T, path, recv, iface, name string) [][2]string {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		t.Fatalf("SOURCE GATE: cannot parse %s: %v", path, err)
	}
	for _, d := range f.Decls {
		switch d := d.(type) {
		case *ast.FuncDecl:
			if recv == "" || d.Recv == nil || d.Name.Name != name {
				continue
			}
			rt := d.Recv.List[0].Type
			if star, ok := rt.(*ast.StarExpr); ok {
				rt = star.X
			}
			if id, ok := rt.(*ast.Ident); ok && id.Name == recv {
				return f8Params(d.Type.Params)
			}
		case *ast.GenDecl:
			if iface == "" {
				continue
			}
			for _, s := range d.Specs {
				ts, ok := s.(*ast.TypeSpec)
				if !ok || ts.Name.Name != iface {
					continue
				}
				it, ok := ts.Type.(*ast.InterfaceType)
				if !ok {
					continue
				}
				for _, m := range it.Methods.List {
					ft, ok := m.Type.(*ast.FuncType)
					if !ok || len(m.Names) == 0 || m.Names[0].Name != name {
						continue
					}
					return f8Params(ft.Params)
				}
			}
		}
	}
	t.Fatalf("SOURCE GATE: %s: method %s not found on %q/%q — re-anchor this gate", path, name, recv, iface)
	return nil
}

func f8Names(ps [][2]string) string {
	var s []string
	for _, p := range ps {
		s = append(s, p[0]+" "+p[1])
	}
	return "(" + strings.Join(s, ", ") + ")"
}

// TestF8_NoPortMethodCarriesAnEpoch is G-F8-1. RED on main: all four signatures end
// in an epoch parameter the CALLER supplies (ports.go:221 currentEpoch, :241 current;
// delivery.go:280/:353 currentEpoch; relayanchor.go:89 current; demandrole.go:148
// currentEpoch).
//
// RUNTIME GATE: core/credit TestF8_FallingSourceLowersNothingAndReadmitsNothing_Delivery.
func TestF8_NoPortMethodCarriesAnEpoch(t *testing.T) {
	type site struct {
		path, recv, iface, name string
		wantN                   int
		lastName                string // the LAST parameter the port keeps ("" = none required)
	}
	sites := []site{
		{"ports.go", "", "CreditLedger", "RedeemDeliveryCredit", 5, "issuedEpoch"},
		{"ports.go", "", "CreditLedger", "SpendRelayAnchors", 1, "anchors"},
		{"../core/credit/delivery.go", "Ledger", "", "RedeemDeliveryCredit", 5, "issuedEpoch"},
		{"../core/credit/delivery.go", "Ledger", "", "RedeemDeliveryCreditReason", 5, "issuedEpoch"},
		{"../core/credit/relayanchor.go", "Ledger", "", "SpendRelayAnchors", 1, "anchors"},
		{"../core/node/demandrole.go", "", "deliveryReasoner", "RedeemDeliveryCreditReason", 5, "issuedEpoch"},
	}
	for _, s := range sites {
		ps := f8MethodParams(t, s.path, s.recv, s.iface, s.name)
		for _, p := range ps {
			if p[0] == "currentEpoch" || p[0] == "current" {
				t.Errorf("SOURCE GATE: %s %s%s still takes the caller-supplied epoch %q. Checked: "+
					"parameter NAMES of one signature. R-F8-SOURCE: the ledger READS its epoch from "+
					"one injected EpochSource; a per-call epoch is the unauthenticated boundary F8 "+
					"names (one call at 2^62 refuses every honest redeem thereafter)",
					s.path, s.name, f8Names(ps), p[0])
			}
		}
		if len(ps) != s.wantN {
			t.Errorf("SOURCE GATE: %s %s has %d parameters %s, want %d ending in %q. Checked: "+
				"a parameter COUNT; the runtime property (the ledger follows its source) is "+
				"observed by core/credit TestF8_FallingSourceLowersNothingAndReadmitsNothing_*",
				s.path, s.name, len(ps), f8Names(ps), s.wantN, s.lastName)
			continue
		}
		if last := ps[len(ps)-1]; last[0] != s.lastName {
			t.Errorf("SOURCE GATE: %s %s's last parameter is %q %s, want %q. Checked: one "+
				"parameter NAME", s.path, s.name, last[0], f8Names(ps), s.lastName)
		}
		if s.wantN == 1 && ps[0][1] == "uint64" {
			t.Errorf("SOURCE GATE: %s %s takes a bare uint64 %s. Checked: one parameter TYPE",
				s.path, s.name, f8Names(ps))
		}
	}
}
