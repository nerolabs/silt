package chain

import (
	"bytes"
	"crypto/ed25519"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/nerolabs/silt/ports"
)

// =============================================================================
// O3 DIRECTION T — GATE (a): R-558-VERIFIER-INVENTORY
// =============================================================================
//
// Binding spec: O3-Direction-T-I5-restatement-and-divergence-RESEARCH-CERTIFICATION-2026-09-04.md
// §8.1 (a1)+(a2). Scar: #558 at its THIRD verify site — `blockWeight` verified attestations
// against the BARE block hash while every production attestation has signed
// consensusSigBytes(phase, round, hash) since #432; the term was identically 0 for 18 days and
// nobody saw it, because the only fixtures that exercised it were era-1.
//
// THE RULE the pin encodes (the cert's own words): a domain-separation change is a property of the
// SCHEME — every verifier moves together, or the missed one silently rejects every valid signature
// and the path it guards goes dead. So: no attestation is verified in non-test core/chain outside
// `verifyAtt` (the one era-aware verifier) and the era-1-gated `signedBlock`.
//
// FORM CHOSEN (the task offered two): the allowlist NAMES `blockWeight` with a
// MUST-BE-DELETED-BY-T tombstone and the walk asserts the tombstoned site is GONE. Reason: with the
// alternative form (site simply absent from the allowlist), the RED on main reads as "unclassified
// site" — the same message a NEW bare-hash site would produce, so the Builder could green it by
// adding an allowlist row (scar-allowlist-rationale-is-a-claim: the widening reflex). With the
// tombstone the RED on main names the deletion owed, and after T the tombstone row stays as the
// permanent record that this site may never return. RED now; GREEN after `blockWeight` is deleted
// with no allowlist edit.
//
// SOURCE GATE: this file walks the package's own non-test .go source with go/ast. It sees
// call sites and enclosing function names, not behaviour.
// RUNTIME GATE: TestO3T_Era2CertificateAcceptedByEveryEra2Verifier (below) drives one genuine
// era-2 certificate through every attestation verifier this allowlist marks era-2-reachable.

// o3tVerifyClass classifies one ed25519.Verify site.
type o3tVerifyClass string

const (
	o3tClassAttestation       o3tVerifyClass = "attestation"           // MUST be verifyAtt or signedBlock
	o3tClassProposerSig       o3tVerifyClass = "proposer-sig"          // bare hash IS the proposer scheme
	o3tClassBondRegSig        o3tVerifyClass = "bondreg-sig"           // r.signingBytes(nonce)
	o3tClassIssuerKeySig      o3tVerifyClass = "issuerkey-sig"         // issuerKeyRegMsg(...)
	o3tClassAttestationBare   o3tVerifyClass = "attestation-bare-hash" // the #558 defect class
	o3tMustBeDeletedByTMarker                = "MUST-BE-DELETED-BY-T"
)

// o3tVerifySite keys one allowlist row: (file, enclosing function).
type o3tVerifySite struct {
	File string // base name of the non-test file
	Func string // enclosing FuncDecl name (receiver-less form)
}

// o3tVerifyRow is one classified allowlist row. Count is the number of ed25519.Verify calls the
// walk must find inside that function; Classes lists each call's class in source order.
type o3tVerifyRow struct {
	Count   int
	Classes []o3tVerifyClass
	Why     string
	Marker  string // "" or o3tMustBeDeletedByTMarker
}

// o3tVerifyAllowlist is the CLASSIFIED inventory at 59509b1 (cert §8.1 table, re-enumerated from
// the code — the cert was read at b328268; PR #720 added carrier.go since).
var o3tVerifyAllowlist = map[o3tVerifySite]o3tVerifyRow{
	{"chain.go", "verifyAtt"}: {Count: 2, Classes: []o3tVerifyClass{o3tClassAttestation, o3tClassAttestation},
		Why: "THE era-aware attestation verifier: era-1 arm (a.Round == 0 guard, bare hash) and era-2 arm (consensusSigBytes(phase, round, h)). Every attestation verify in the package routes here except signedBlock's era-1-only arm."},
	{"equivocation.go", "signedBlock"}: {Count: 2, Classes: []o3tVerifyClass{o3tClassProposerSig, o3tClassAttestation},
		Why: "era-1 evidence ONLY: reachable from CheckEquivocation only when both evidence blocks are < BlockVersionRounds; era-2 evidence goes through consensusSigScopes -> verifyAtt. Folding the attestation arm into verifyAtt touches the R0.6-certified slash path: follow-on O3-R13, not T."},
	{"chain.go", "ValidateProposal"}: {Count: 1, Classes: []o3tVerifyClass{o3tClassProposerSig},
		Why: "bare hash is the proposer scheme"},
	{"chain.go", "validateStructural"}: {Count: 1, Classes: []o3tVerifyClass{o3tClassProposerSig},
		Why: "bare hash is the proposer scheme (the Reload path; its attester loop uses verifyAtt — the #558 first-site fix)"},
	{"chain.go", "AppendGenesis"}: {Count: 1, Classes: []o3tVerifyClass{o3tClassProposerSig},
		Why: "bare hash is the proposer scheme (genesis proposer)"},
	{"chain.go", "validateBondReg"}: {Count: 1, Classes: []o3tVerifyClass{o3tClassBondRegSig},
		Why: "BondReg signature over r.signingBytes(nonce)"},
	{"issuerkey.go", "VerifyIssuerKeyReg"}: {Count: 1, Classes: []o3tVerifyClass{o3tClassIssuerKeySig},
		Why: "IssuerKey registration signature over issuerKeyRegMsg(id, epoch, fp)"},
	{"carrier.go", "carrierParentProposerFromWitness"}: {Count: 1, Classes: []o3tVerifyClass{o3tClassProposerSig},
		Why: "the parent-proposer witness: the PARENT's bare-hash ProposerSig re-verified over b.Prev (PR #720 LastCommit carrier; same proposer arithmetic as ValidateProposal, over the parent's hash)"},
	// THE TOMBSTONE. Present on main at 59509b1 (chain.go:3997). The walk FAILS while it exists.
	{"chain.go", "blockWeight"}: {Count: 1, Classes: []o3tVerifyClass{o3tClassAttestationBare},
		Why:    "#558 THIRD SITE: verifies attestations against the bare block hash; 0 on every AttestAt certificate since #432. Retired, not repaired (O3 Direction T, owner-ratified 2026-09-03): a repair would make fork-choice a function of the evaluating replica's live state (the #357 oscillation mechanism).",
		Marker: o3tMustBeDeletedByTMarker},
}

// o3tVerifyHit is one ed25519.Verify call the walk found.
type o3tVerifyHit struct {
	Site o3tVerifySite
	Line int
}

// o3tWalkVerifySites collects every ed25519.Verify call in the given parsed files with its
// enclosing function. Shared by the pin and the teeth test so the two cannot drift.
func o3tWalkVerifySites(fset *token.FileSet, files map[string]*ast.File) []o3tVerifyHit {
	var hits []o3tVerifyHit
	for name, af := range files {
		for _, decl := range af.Decls {
			fd, ok := decl.(*ast.FuncDecl)
			if !ok {
				continue
			}
			ast.Inspect(fd, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				sel, ok := call.Fun.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				pkg, ok := sel.X.(*ast.Ident)
				if !ok || pkg.Name != "ed25519" || sel.Sel.Name != "Verify" {
					return true
				}
				hits = append(hits, o3tVerifyHit{
					Site: o3tVerifySite{File: filepath.Base(name), Func: fd.Name.Name},
					Line: fset.Position(call.Pos()).Line,
				})
				return true
			})
		}
	}
	sort.Slice(hits, func(i, j int) bool {
		if hits[i].Site.File != hits[j].Site.File {
			return hits[i].Site.File < hits[j].Site.File
		}
		return hits[i].Line < hits[j].Line
	})
	return hits
}

// o3tClassifyVerifySites compares the walked hits against the allowlist and returns the
// violations (empty = GREEN). Pure so the teeth test can run it on a synthetic file.
func o3tClassifyVerifySites(hits []o3tVerifyHit) []string {
	var violations []string
	counts := map[o3tVerifySite]int{}
	firstLine := map[o3tVerifySite]int{}
	for _, h := range hits {
		counts[h.Site]++
		if _, ok := firstLine[h.Site]; !ok {
			firstLine[h.Site] = h.Line
		}
	}
	for site, n := range counts {
		row, ok := o3tVerifyAllowlist[site]
		switch {
		case !ok:
			violations = append(violations, "UNCLASSIFIED ed25519.Verify site: "+site.File+":"+itoa(firstLine[site])+
				" in "+site.Func+" ("+itoa(n)+" call(s)). Classify it: if it verifies an ATTESTATION it must be "+
				"verifyAtt or the era-1-gated signedBlock — a third bare-hash attestation verifier is #558 again.")
		case row.Marker == o3tMustBeDeletedByTMarker:
			violations = append(violations, o3tMustBeDeletedByTMarker+": "+site.File+":"+itoa(firstLine[site])+
				" in "+site.Func+" still verifies ("+string(row.Classes[0])+"). "+row.Why)
		case n != row.Count:
			violations = append(violations, "COUNT DRIFT: "+site.File+" "+site.Func+" has "+itoa(n)+
				" ed25519.Verify call(s), allowlist says "+itoa(row.Count)+". Re-classify every call in that function.")
		}
	}
	for site, row := range o3tVerifyAllowlist {
		if row.Marker != "" {
			continue // a tombstone's ABSENCE is the pass condition
		}
		if counts[site] == 0 {
			violations = append(violations, "STALE ALLOWLIST ROW: "+site.File+" "+site.Func+
				" has no ed25519.Verify call any more. If the verify moved, move the row; if it was deleted, delete the row — a stale row is a hole.")
		}
	}
	sort.Strings(violations)
	return violations
}

// TestO3T_VerifierInventoryAllowlistIsWellFormed asserts the allowlist's own invariants before the
// walk trusts it: every row classed "attestation" is verifyAtt or signedBlock (the cert §8.1 pass
// condition); no non-tombstone row carries the defect class; the tombstone is exactly blockWeight.
func TestO3T_VerifierInventoryAllowlistIsWellFormed(t *testing.T) {
	tombstones := 0
	for site, row := range o3tVerifyAllowlist {
		if len(row.Classes) != row.Count {
			t.Fatalf("SOURCE GATE: allowlist row %v lists %d classes for %d calls", site, len(row.Classes), row.Count)
		}
		for _, cl := range row.Classes {
			if cl == o3tClassAttestation && site.Func != "verifyAtt" && site.Func != "signedBlock" {
				t.Fatalf("SOURCE GATE: allowlist row %v is classed attestation but is neither verifyAtt nor signedBlock — "+
					"no attestation is verified outside those two (cert §8.1)", site)
			}
			if cl == o3tClassAttestationBare && row.Marker != o3tMustBeDeletedByTMarker {
				t.Fatalf("SOURCE GATE: allowlist row %v carries the #558 defect class WITHOUT a tombstone marker — "+
					"a bare-hash attestation verifier is never allowlisted live", site)
			}
		}
		if row.Marker == o3tMustBeDeletedByTMarker {
			tombstones++
			if site != (o3tVerifySite{"chain.go", "blockWeight"}) {
				t.Fatalf("SOURCE GATE: unexpected tombstone %v — the only site T deletes is chain.go blockWeight", site)
			}
		}
	}
	if tombstones != 1 {
		t.Fatalf("SOURCE GATE: %d tombstone rows, want exactly 1 (chain.go blockWeight)", tombstones)
	}
}

// TestO3T_VerifierInventoryPin is gate (a1): the walked set of ed25519.Verify sites in every
// non-test core/chain file equals the classified allowlist, and the tombstoned blockWeight site
// is GONE. RED on main at 59509b1 (chain.go:3997 blockWeight); GREEN after T.
//
// Scope: EVERY non-test *.go in the package (not a name glob — scar-ast-pin-glob-misses-the-defect-file).
func TestO3T_VerifierInventoryPin(t *testing.T) {
	all, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	files := map[string]*ast.File{}
	for _, f := range all {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		af, pErr := parser.ParseFile(fset, f, nil, 0)
		if pErr != nil {
			t.Fatalf("parse %s: %v", f, pErr)
		}
		files[f] = af
	}
	if len(files) < 20 {
		t.Fatalf("SOURCE GATE: PIN VACUOUS — only %d non-test files parsed in core/chain", len(files))
	}
	hits := o3tWalkVerifySites(fset, files)
	if len(hits) < 8 {
		t.Fatalf("SOURCE GATE: PIN VACUOUS — walk found only %d ed25519.Verify call(s); the allowlist expects at least the 8 non-tombstone rows' calls", len(hits))
	}
	var inv []string
	for _, h := range hits {
		inv = append(inv, "  "+h.Site.File+":"+itoa(h.Line)+"  "+h.Site.Func)
	}
	t.Logf("ed25519.Verify inventory (%d calls):\n%s", len(hits), strings.Join(inv, "\n"))
	if v := o3tClassifyVerifySites(hits); len(v) > 0 {
		t.Fatalf("SOURCE GATE: R-558-VERIFIER-INVENTORY — %d violation(s):\n  %s\n\n"+
			"  Do NOT widen the allowlist to green this. An attestation verify belongs in verifyAtt; the\n"+
			"  tombstoned blockWeight site is retired by O3 Direction T (delete Weight/blockWeight/anchorWeight/\n"+
			"  Config.AnchorWeight; heavier becomes height -> head-hash).", len(v), strings.Join(v, "\n  "))
	}
}

// TestO3T_VerifierInventoryPinHasTeeth proves the walk bites: the SAME walk + classification over a
// synthetic file that re-injects a bare-hash attestation verify in a new function flags it as
// UNCLASSIFIED, and over a synthetic copy of blockWeight flags the tombstone.
func TestO3T_VerifierInventoryPinHasTeeth(t *testing.T) {
	const injected = `package chain

import "crypto/ed25519"

func (c *Chain) reInjectedWeight(b *Block) int64 {
	h := b.Hash()
	var n int64
	for _, a := range b.Atts {
		if !ed25519.Verify(ed25519.PublicKey(a.PubKey), h[:], a.Sig) { // the #558 shape, re-injected
			continue
		}
		n++
	}
	return n
}

func (c *Chain) blockWeight(b *Block) int64 {
	h := b.Hash()
	for _, a := range b.Atts {
		if ed25519.Verify(ed25519.PublicKey(a.PubKey), h[:], a.Sig) {
			return 1
		}
	}
	return 0
}
`
	fset := token.NewFileSet()
	af, err := parser.ParseFile(fset, "chain.go", injected, 0)
	if err != nil {
		t.Fatalf("parse synthetic: %v", err)
	}
	hits := o3tWalkVerifySites(fset, map[string]*ast.File{"chain.go": af})
	if len(hits) != 2 {
		t.Fatalf("SOURCE GATE: PIN HAS NO TEETH — walk found %d call(s) in the synthetic file, want 2", len(hits))
	}
	v := o3tClassifyVerifySites(hits)
	joined := strings.Join(v, "\n")
	if !strings.Contains(joined, "UNCLASSIFIED ed25519.Verify site: chain.go:") || !strings.Contains(joined, "reInjectedWeight") {
		t.Fatalf("SOURCE GATE: PIN HAS NO TEETH — re-injected bare-hash attestation verify was not flagged UNCLASSIFIED:\n%s", joined)
	}
	if !strings.Contains(joined, o3tMustBeDeletedByTMarker+": chain.go:") || !strings.Contains(joined, "blockWeight") {
		t.Fatalf("SOURCE GATE: PIN HAS NO TEETH — a re-appearing blockWeight was not flagged by the tombstone:\n%s", joined)
	}
}

// -----------------------------------------------------------------------------
// (a2) ONE REAL ERA-2 CERTIFICATE THROUGH EVERY ERA-2-REACHABLE ATTESTATION VERIFIER
// -----------------------------------------------------------------------------

// TestO3T_Era2CertificateAcceptedByEveryEra2Verifier is gate (a2) and the RUNTIME cover for the
// inventory pin. One genuine era-2 certificate (roundsWorld: proposer self-prepare + prepare-QC +
// precommits, all AttestAt) is driven through every attestation verifier the allowlist marks
// era-2-reachable, and each must accept:
//
//	verifyAtt via collectQuorumSigs      — ValidateCommit (the live commit path)
//	verifyAtt via requireProposerPrepare — ValidateCommit
//	verifyAtt via validateStructural     — Reload (the #558 first site)
//	verifyAtt via consensusSigScopes     — CheckEquivocation's era-2 branch
//	verifyAtt via HeadCarrier            — the carrier the NEXT proposer attaches (PR #720)
//	verifyAtt via validateCarrier        — the carrier a v5 child carries over b.Prev (PR #720)
//
// signedBlock is NOT in this set (era-1 only): asserting it accepts an era-2 certificate would be
// asserting a falsehood (cert §8.1 scope note). This single fixture would have caught all three
// #558 sites. GREEN now; must stay green. RED-first artifact: point any one verifier at the bare
// hash and the fixture fails there (recorded in the Tester's memory, o3-direction-t-gates-2026-09-04).
func TestO3T_Era2CertificateAcceptedByEveryEra2Verifier(t *testing.T) {
	c, keys, g := roundsWorld(t)
	b := v2Block(g, keys, 0)
	h := b.Hash()

	// Precondition: this is an era-2 certificate — NOT ONE attestation verifies over the bare hash.
	for _, set := range [][]Attestation{b.PrepareQC, b.Atts} {
		for _, a := range set {
			if a.Phase == PhaseLegacy {
				t.Fatal("fixture: an era-2 certificate must carry no PhaseLegacy attestation")
			}
			if signedBlock(a.PubKey, b, h) && !bytes.Equal(a.PubKey, b.Proposer) {
				t.Fatalf("fixture: attester %x verifies over the BARE hash — this is not an era-2 certificate", a.PubKey[:4])
			}
		}
	}

	// 1+2. The live commit path: collectQuorumSigs (both phases) + requireProposerPrepare.
	if err := c.ValidateCommit(b); err != nil {
		t.Fatalf("ValidateCommit (verifyAtt via collectQuorumSigs/requireProposerPrepare) refused the era-2 certificate: %v", err)
	}
	if err := c.Append(*b); err != nil {
		t.Fatalf("Append: %v", err)
	}

	// 3. The Reload path: validateStructural over the persisted representation.
	persisted, err := DecodeBlocks(EncodeBlocks(c.Blocks(0)))
	if err != nil {
		t.Fatal(err)
	}
	anchors := map[ports.NodeID]bool{}
	for _, k := range keys {
		anchors[idOf(k)] = true
	}
	fresh := New(Config{Quorum: 1, MinBond: 1 << 20, ByzantineQuorum: true, Anchors: anchors, AnchorQuorum: 1, MatureValidators: 99},
		func(ports.NodeID) int64 { return 0 })
	fresh.SetBondVerifier(objectiveVerify)
	if n, rErr := fresh.Reload(persisted); rErr != nil || n != len(persisted) {
		t.Fatalf("Reload (verifyAtt via validateStructural) refused the era-2 certificate: restored %d of %d, err=%v", n, len(persisted), rErr)
	}

	// 4. The slash path's era-2 branch: consensusSigScopes must see every signer's two slots.
	for i, k := range keys {
		pub := []byte(k.Public().(ed25519.PublicKey))
		scopes := consensusSigScopes(pub, b, h)
		want := 2 // one prepare + one precommit, both at round 0
		if len(scopes) != want {
			t.Fatalf("consensusSigScopes (verifyAtt) saw %d slot(s) for signer %d, want %d: %v", len(scopes), i, want, scopes)
		}
	}

	// 5. HeadCarrier: the precommit set the next proposer carries, re-verified with verifyAtt.
	carrier := c.HeadCarrier()
	if len(carrier) != len(b.Atts) {
		t.Fatalf("HeadCarrier (verifyAtt) returned %d of %d precommits", len(carrier), len(b.Atts))
	}

	// 6. validateCarrier: a v5 child carrying that precommit set over b.Prev == b.Hash().
	child := &Block{Version: BlockVersionWitnessable, Height: b.Height + 1, Prev: h, LastCommit: carrier}
	if err := validateCarrier(child); err != nil {
		t.Fatalf("validateCarrier (verifyAtt over b.Prev) refused the carried era-2 precommits: %v", err)
	}
}
