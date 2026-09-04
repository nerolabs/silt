package node

// R2.13b — the two gates the PE ruling lists beyond G-CS-1..4
// (RULING-F4-creditSpent-durability-and-F3-fee-constancy-2026-09-04.md §3, "the
// restart-replay gate"): the unloaded-guard refusal and the append-before-sign
// crash-order pin.

import (
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/nerolabs/silt/adapters/guardstore"
	"github.com/nerolabs/silt/adapters/identity"
)

// TestF4_UnloadedCreditStoreRefusesCreditBearingRequests: a store attached but not
// yet loaded refuses every CREDIT-BEARING request (errCreditGuardUnloaded) and keeps
// serving credit-free ones; once loaded, the same credit spends exactly once and the
// spend is in the store. A guard that does not know what it already spent must not
// spend — the ReasonGuardUnloaded twin.
func TestF4_UnloadedCreditStoreRefusesCreditBearingRequests(t *testing.T) {
	publishKey, committed := csKeys(t)
	store := guardstore.NewMem()
	durable := identity.FromSeed(csDurableSeed).NodeID()
	eph := identity.FromSeed(csEphSeed).NodeID()

	// Boot with no store, then attach WITHOUT loading: the window between the
	// daemon's SetCreditSpentStore and LoadCreditSpent.
	iss, ledger := csBootIssuer(t, csIssuerSeed, publishKey, committed, nil)
	iss.SetCreditSpentStore(store)

	// Credit-FREE requests are served while unloaded: minting a credit is one.
	cr, burned := csMintCredit(t, iss, ledger, &publishKey.PublicKey, durable)
	if burned != 50_000 {
		t.Fatalf("credit-free request while unloaded: burned %d, want the 50000 fee (the request must be served)", burned)
	}

	// Credit-BEARING requests are refused with the named reason, at the seam and on the wire.
	if _, err := iss.tokenChargeFor(eph, &cr); !errors.Is(err, errCreditGuardUnloaded) {
		t.Fatalf("unloaded guard: tokenChargeFor returned %v, want errors.Is errCreditGuardUnloaded", err)
	}
	if _, ok := csWithdraw(t, iss, eph, &committed.PublicKey, &cr); ok {
		t.Fatal("unloaded guard: the wire handler issued a demand token for a credit the guard cannot check against its store")
	}
	if iss.creditSpent[string(cr.Serial)] {
		t.Fatal("unloaded guard: the refused credit was marked spent")
	}
	if n, _ := csStoreHas(t, store, cr.Serial); n != 0 {
		t.Fatalf("unloaded guard: store holds %d entries after a refusal, want 0", n)
	}

	// Loaded: the same credit spends exactly once, durably.
	if err := iss.LoadCreditSpent(); err != nil {
		t.Fatal(err)
	}
	if _, ok := csWithdraw(t, iss, eph, &committed.PublicKey, &cr); !ok {
		t.Fatal("after LoadCreditSpent the unspent credit was refused")
	}
	if n, has := csStoreHas(t, store, cr.Serial); !has || n != 1 {
		t.Fatalf("after the loaded spend the store holds %d entries (has serial: %v), want exactly 1", n, has)
	}
	if _, ok := csWithdraw(t, iss, eph, &committed.PublicKey, &cr); ok {
		t.Fatal("the credit was spent twice after load")
	}
}

// TestF4_AppendLandsBeforeSignBlinded pins the crash order the fix relies on, as
// SOURCE order at the two sites that compose it: (1) blindtoken.Issue calls charge()
// before SignBlinded, (2) tokenChargeFor's settlement closure Appends to the store
// before it marks creditSpent. Together: the durable record exists before any
// signature does, so a crash between them is an under-issue (a lost fee), never a
// token a restart cannot see.
//
// RUNTIME GATE: TestCreditSpentStoreFailureRefusesTheWithdrawal (an Append failure
// yields no token and no in-memory mark) and core/blindtoken issuer_test.go ("a failed
// fee charge must mint no token"). UNGATED: the actual interleaving of a crash between
// the fsync and the modexp — no observer can sit between them; this pin is text order.
func TestF4_AppendLandsBeforeSignBlinded(t *testing.T) {
	issue := f4FuncBody(t, "../blindtoken/issuer.go", "func (i *Issuer) Issue(")
	iCharge := strings.Index(issue, "charge()")
	iSign := strings.Index(issue, "SignBlinded(")
	if iCharge < 0 || iSign < 0 || iCharge > iSign {
		t.Fatalf("SOURCE GATE: in blindtoken.Issue, `charge()` (offset %d) must precede `SignBlinded(` (offset %d). "+
			"Checked: text order of two literals inside one function body. If the signature came first, a "+
			"crash after signing and before the charge would hand out a token the credit-spent store never saw",
			iCharge, iSign)
	}
	closure := f4FuncBody(t, "tokenrole.go", "func (n *Node) tokenChargeFor(")
	iAppend := strings.Index(closure, ".Append(")
	iMark := strings.Index(closure, "n.creditSpent[key] = true")
	if iAppend < 0 || iMark < 0 || iAppend > iMark {
		t.Fatalf("SOURCE GATE: in tokenChargeFor, `.Append(` (offset %d) must precede `n.creditSpent[key] = true` "+
			"(offset %d). Checked: text order of two literals inside one function body. Marking first would let "+
			"a failed durable write leave an in-memory spend no restart remembers", iAppend, iMark)
	}
}

// f4FuncBody returns the comment-stripped text of the function starting at sig, up
// to its closing brace at column 0.
func f4FuncBody(t *testing.T, path, sig string) string {
	t.Helper()
	src, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	s := string(src)
	i := strings.Index(s, sig)
	if i < 0 {
		t.Fatalf("SOURCE GATE: `%s` not found in %s — re-anchor this pin", sig, path)
	}
	s = s[i:]
	if j := strings.Index(s, "\n}\n"); j >= 0 {
		s = s[:j]
	}
	var out []string
	for _, ln := range strings.Split(s, "\n") {
		if k := strings.Index(ln, "//"); k >= 0 {
			ln = ln[:k]
		}
		out = append(out, ln)
	}
	return strings.Join(out, "\n")
}
