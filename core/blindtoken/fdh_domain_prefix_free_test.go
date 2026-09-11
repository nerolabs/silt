package blindtoken

// Freeze manifest item 11 / R0.4b-FDH — THE FDH DOMAIN SET IS PAIRWISE PREFIX-FREE.
//
// WHAT THE GATE HOLDS. fullDomainHashD hashes `domain ‖ ctr(4B BE) ‖ msg` with the domain
// BARE: no length prefix, no separator, no terminator. Prefix-freeness of the domain set
// is what makes that concatenation injective in (domain, msg): if neither of two domains
// is a prefix of the other they differ at some index below both lengths, so the hashed
// strings differ at that index whatever the messages are. Prefix-freeness subsumes
// distinctness — a string is a prefix of itself — and it holds for any message set, any
// message length, and any field order inside fullDomainHashD.
//
// WHAT A VIOLATION WOULD COST, stated accurately because the gate's own text is a claim
// that decays like any other. A prefix violation is NOT by itself a forgery in the
// construction as shipped, and this file does not say that it is.
// TestAPrefixPairFailsToCollideOnlyBecauseTheCounterVaries drives the real position: with
// a crafted message a prefix pair DOES collide on SHA-256 block 0 (measured), and the
// collision dies at block 1 because the counter sits between the domain and the message
// and changes per block. `MinModulusBits = 2048` forces nine blocks, so a one-block
// expansion never happens. That rescue is a property of fullDomainHashD's field order —
// pinned byte-exactly by TestPublishAndCreditFDHAreUnchanged and its two siblings, but never
// reviewed or reasoned about as a separation argument, and subtler than the property it
// rescues. silt's canon on proof-versus-structure says to gate the structure, not the
// accident. Prefix-freeness is the structure.
//
// If the separation ever DID fail, the cost is that one issuer key's signature moves
// lanes: a credit spent as a publish token, or a relay-lane anchor spent as a delivery-lane
// demand token — one 50,000-credit fee funding two payouts, because the two spent sets
// cannot see each other (R2.14 cert §2.4 door (iii)).
//
// WHY A GATE AND NOT THE FIX. Crypto advisory C-8 (2026-09-03) proposed length-prefixing
// the domain, the RFC 9380 §5.3 `expand_message_xmd` DST treatment. That changes the FDH
// input, so it changes every token's bytes, and the publish and credit domains are
// byte-frozen against chain replay. The change was DECLINED at the era-4 freeze in favour
// of this gate (ROADMAP R0.4b-FDH; PE re-audit item 11,
// /Users/andrewedmond/.claude/silt-agent-memory/principal-engineer/reviews/RULING-era4-freeze-manifest-re-audit-f826c72-2026-09-11.md).
// The advisory's own words for the status quo: sound "by accident of the constants". This
// file converts the accident into a checked property.
//
// DERIVED, NOT HAND-LISTED. The pairs come from a source walk of this package, so a fifth
// domain is covered the day it is declared (M3 is exactly that case). Enumeration by
// source is complete for the construction because the domain argument never crosses the
// package boundary — fullDomainHashD, blindD, unblindD and verifyD are all unexported, and
// TestFDHDomainArgumentStaysInsideThisPackage holds that closed.
//
// RUNTIME GATE: TestEveryOrderedDomainPairRefusesTheOthersSignature — the source walk sees
// only strings; that test drives a real blind withdrawal under every discovered domain and
// asserts the signature verifies under its own domain and under no other.
//
// ABLATIONS RUN 2026-09-11 against `blindtoken.go`, each patch diffed against a pristine
// copy before the run was believed, each reverted and re-diffed after. Baseline exit 0 on
// both sides; legs 1-4 exit 1.
//
//  1. Add `testFifthDomain = "silt/blindtoken/fdh/v1/extended"` →
//     TestFDHDomainSetIsPairwisePrefixFree RED. Under that same patch the pre-existing
//     DISTINCTNESS assertions (TestRelayAnchorDomainIsPinnedByteExactly,
//     TestDemandDomainStillSeparatesFromPublishAndCredit) stay GREEN: measured, in this
//     tree, that distinctness does not discharge prefix-freeness.
//  2. Declare `relayAnchorDomain` as `"silt/blindrelay/" + "fdh/v1"` — same value, invisible
//     to a literal walk → TestFDHDomainSetIsPairwisePrefixFree,
//     TestFDHDomainSetSourceWalkFindsTheRealDeclarations and
//     TestEveryOrderedDomainPairRefusesTheOthersSignature all RED. A gate that reads source
//     goes vacuous quietly; that is the leg that matters most.
//  3. Add an exported `ExportedFullDomainHash(..., domain string)` →
//     TestFDHDomainArgumentStaysInsideThisPackage RED.
//  4. Make `blindD` and `verifyD` pass `fdhDomain` instead of their `domain` argument →
//     TestEveryOrderedDomainPairRefusesTheOthersSignature RED, the three source gates GREEN.
//     The runtime half and the source half fail independently, which is the point of having
//     both.
//  5. CONTROL, and the reason this file does not duplicate a field-order pin: move the
//     counter to `domain ‖ msg ‖ ctr`. All five tests here stay GREEN; the three existing
//     byte-exact pins (TestPublishAndCreditFDHAreUnchanged,
//     TestDemandFDHInputBindsTheEpochByteExactly, TestRelayAnchorDomainIsPinnedByteExactly)
//     all go RED. Verified by running them under the patch, not assumed.

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"go/ast"
	"go/parser"
	"go/token"
	mrand "math/rand"
	"os"
	"strconv"
	"strings"
	"testing"
)

// fdhDomainDecl is one domain constant as the source walk found it.
type fdhDomainDecl struct {
	name  string
	value string
	file  string
}

// domainValuePrefix is the shape every FDH domain constant's VALUE carries. The walk
// selects on name suffix OR value prefix, so a new domain that keeps either convention is
// enumerated. One that keeps neither trips the anchor in
// TestFDHDomainSetSourceWalkFindsTheRealDeclarations. A `*Domain` constant that is not a
// plain string literal is refused outright, because the walk cannot read its value and the
// anchor covers only the four domains the compiler resolves here.
const domainValuePrefix = "silt/blind"

// collectFDHDomains walks every non-test .go file in this package and returns the string
// constants that are FDH domains.
func collectFDHDomains(t *testing.T) []fdhDomainDecl {
	t.Helper()
	ents, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("SOURCE GATE: cannot read the package directory: %v", err)
	}
	var found []fdhDomainDecl
	fset := token.NewFileSet()
	for _, e := range ents {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatalf("SOURCE GATE: cannot parse %s: %v", name, err)
		}
		ast.Inspect(f, func(n ast.Node) bool {
			vs, ok := n.(*ast.ValueSpec)
			if !ok {
				return true
			}
			for i, id := range vs.Names {
				byName := strings.HasSuffix(id.Name, "Domain")
				var lit *ast.BasicLit
				if i < len(vs.Values) {
					if bl, ok := vs.Values[i].(*ast.BasicLit); ok && bl.Kind == token.STRING {
						lit = bl
					}
				}
				if lit == nil {
					// A domain that is not a plain string literal is invisible to this walk, and an
					// invisible domain is an unchecked domain. Refuse the shape rather than skip it.
					if byName {
						t.Fatalf("SOURCE GATE: %s in %s is named as a domain but is not a plain string "+
							"literal. TestFDHDomainSetIsPairwisePrefixFree enumerates domains by reading "+
							"this source, so a domain assembled from an expression is one nothing checks. "+
							"Write the domain out byte for byte.", id.Name, name)
					}
					continue
				}
				val, err := strconv.Unquote(lit.Value)
				if err != nil {
					t.Fatalf("SOURCE GATE: cannot unquote %s in %s: %v", id.Name, name, err)
				}
				if !byName && !strings.HasPrefix(val, domainValuePrefix) {
					continue
				}
				found = append(found, fdhDomainDecl{name: id.Name, value: val, file: name})
			}
			return true
		})
	}
	return found
}

// TestFDHDomainSetIsPairwisePrefixFree is manifest item 11. Every ordered pair of domains
// in the set, derived by source walk.
func TestFDHDomainSetIsPairwisePrefixFree(t *testing.T) {
	found := collectFDHDomains(t)
	for _, a := range found {
		for _, b := range found {
			if a.name == b.name {
				continue
			}
			if a.value == b.value {
				t.Fatalf("SOURCE GATE: FDH DOMAIN SEPARATION BROKEN — %s (%s) and %s (%s) are both %q.\n"+
					"Two names for one domain means the two lanes are not separated at all: every signature "+
					"valid in one verifies in the other under the same issuer key. Give the new lane its own "+
					"domain string.",
					a.name, a.file, b.name, b.file, a.value)
			}
			if !strings.HasPrefix(a.value, b.value) {
				continue
			}
			t.Fatalf("SOURCE GATE: THE FDH DOMAIN SET IS NO LONGER PREFIX-FREE — %s = %q (%s) is a "+
				"PREFIX of %s = %q (%s).\n"+
				"WHAT BROKE: fullDomainHashD hashes domain ‖ ctr(4B BE) ‖ msg with the domain BARE — no "+
				"length prefix, no separator — so `domain ‖ …` is injective in (domain, msg) only while no "+
				"domain prefixes another. With %s a prefix of %s, one hashed byte string is a valid encoding "+
				"of two different (domain, msg) pairs, and msg is the requester's own serial: attacker-chosen "+
				"and variable-length.\n"+
				"WHAT IT COSTS IF THE SEPARATION FAILS: one issuer key's signature moves lanes — a credit "+
				"spent as a publish token, or a relay-lane anchor spent as a delivery-lane demand token, one "+
				"50,000-credit fee funding two payouts, because the two spent sets cannot see each other "+
				"(R2.14 cert §2.4 door (iii)).\n"+
				"READ THIS BEFORE YOU DISMISS IT: the separation does not fail TODAY on this alone. "+
				"TestAPrefixPairFailsToCollideOnlyBecauseTheCounterVaries measures a prefix pair colliding on "+
				"SHA-256 block 0 and NOT on block 1, because the counter sits between the domain and the "+
				"message and changes per block. That rescue lives in fullDomainHashD's field order, which no "+
				"separation argument anywhere rests on and no review approved as one. Do not spend it.\n"+
				"THE FIX IS A DIFFERENT DOMAIN STRING FOR THE NEW LANE, NOT AN EDIT TO AN EXISTING ONE: the "+
				"publish and credit domains are byte-frozen against chain replay (ROADMAP R0.4b-FDH), so "+
				"changing one invalidates history. Length-prefixing the domain (crypto advisory C-8) fixes "+
				"the class but rewrites every token's bytes, which is why it is era-gated and why this gate "+
				"stands in its place.",
				b.name, b.value, b.file, a.name, a.value, a.file, b.name, a.name)
		}
	}
}

// TestFDHDomainSetSourceWalkFindsTheRealDeclarations is the non-vacuity anchor. A source
// walk that matches nothing passes every pairwise check, so the walk is checked against
// the constants the compiler resolves. A domain renamed out of both conventions reddens
// here rather than silently leaving the pairwise gate.
func TestFDHDomainSetSourceWalkFindsTheRealDeclarations(t *testing.T) {
	found := collectFDHDomains(t)
	byValue := make(map[string]string, len(found))
	for _, d := range found {
		byValue[d.value] = d.name
	}
	for _, want := range []struct{ name, value string }{
		{"fdhDomain", fdhDomain},
		{"creditDomain", creditDomain},
		{"demandDomain", demandDomain},
		{"relayAnchorDomain", relayAnchorDomain},
	} {
		got, ok := byValue[want.value]
		if !ok {
			t.Fatalf("SOURCE GATE: the walk did not find %s = %q among %d domain constants. "+
				"TestFDHDomainSetIsPairwisePrefixFree runs over the walk's output, so a domain the walk "+
				"misses is a domain nothing checks. Keep the name suffix \"Domain\" or the value prefix %q.",
				want.name, want.value, len(found), domainValuePrefix)
		}
		if got != want.name {
			t.Fatalf("SOURCE GATE: %q is declared as %s in source but the compiler resolves it through %s — "+
				"two constants hold one domain value", want.value, got, want.name)
		}
	}
}

// TestFDHDomainArgumentStaysInsideThisPackage holds the completeness precondition the
// source walk rests on: the set of domains the FDH can be driven with is exactly the set
// declared in this package. Every function taking a `domain string` is unexported, so no
// caller outside blindtoken can supply one the walk never saw.
func TestFDHDomainArgumentStaysInsideThisPackage(t *testing.T) {
	ents, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("SOURCE GATE: cannot read the package directory: %v", err)
	}
	fset := token.NewFileSet()
	seen := 0
	for _, e := range ents {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatalf("SOURCE GATE: cannot parse %s: %v", name, err)
		}
		for _, decl := range f.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Type.Params == nil {
				continue
			}
			for _, p := range fn.Type.Params.List {
				id, ok := p.Type.(*ast.Ident)
				if !ok || id.Name != "string" {
					continue
				}
				for _, pn := range p.Names {
					if pn.Name != "domain" {
						continue
					}
					seen++
					if fn.Name.IsExported() {
						t.Fatalf("SOURCE GATE: %s in %s is EXPORTED and takes a `domain string`. "+
							"That lets a caller outside blindtoken drive the FDH with a domain that is not "+
							"declared here, so TestFDHDomainSetIsPairwisePrefixFree no longer covers the set "+
							"it checks. Keep the domain argument package-private, or move the gate to the "+
							"call sites.", fn.Name.Name, name)
					}
				}
			}
		}
	}
	if seen < 4 {
		t.Fatalf("SOURCE GATE: found only %d functions taking a `domain string`; expected at least four "+
			"(fullDomainHashD, blindD, unblindD, verifyD). The inventory anchor moved and this gate is "+
			"checking less than it reads.", seen)
	}
}

// TestEveryOrderedDomainPairRefusesTheOthersSignature is the RUNTIME half of item 11, over
// the same source-walked set. A full blind withdrawal runs under each domain; the resulting
// signature must verify under its own domain and under no other.
//
// It does not discharge prefix-freeness and does not claim to — one random serial cannot
// exhibit a crafted-message collision. It discharges the consequence for the set as it
// stands, and it grows with the set, which the hand-listed three-pair check inside
// TestRelayAnchorDomainIsPinnedByteExactly does not.
func TestEveryOrderedDomainPairRefusesTheOthersSignature(t *testing.T) {
	found := collectFDHDomains(t)
	priv := testKey(t)
	pub := &priv.PublicKey
	rng := mrand.New(mrand.NewSource(11))

	serial, err := NewSerial(rng)
	if err != nil {
		t.Fatalf("NewSerial: %v", err)
	}
	for _, a := range found {
		blinded, secret, err := blindD(rng, pub, serial, a.value)
		if err != nil {
			t.Fatalf("blindD under %s: %v", a.name, err)
		}
		sig, err := unblindD(pub, serial, mustSign(t, priv, blinded), secret, a.value)
		if err != nil {
			t.Fatalf("unblindD under %s: %v", a.name, err)
		}
		if !verifyD(pub, serial, sig, a.value) {
			t.Fatalf("a signature withdrawn under %s = %q does not verify under its own domain — "+
				"the positive control failed, so the refusals below prove nothing", a.name, a.value)
		}
		for _, b := range found {
			if a.name == b.name {
				continue
			}
			if verifyD(pub, serial, sig, b.value) {
				t.Fatalf("DOMAIN SEPARATION BROKEN AT RUNTIME: a signature withdrawn under %s = %q "+
					"verifies under %s = %q on the same serial and the same issuer key. One fee now buys "+
					"a token in two lanes, and the two spent sets cannot see each other (R2.14 cert §2.4 "+
					"door (iii)).", a.name, a.value, b.name, b.value)
			}
		}
	}
}

// TestAPrefixPairFailsToCollideOnlyBecauseTheCounterVaries measures the mechanism the
// prefix-freeness gate is usually explained with, and corrects it.
//
// The usual explanation: if domain A prefixes domain B, a message under B reparses as a
// message under A with trailing bytes, so a signature crosses lanes. In fullDomainHashD as
// shipped that is FALSE, and this test shows exactly where it stops being true: the crafted
// shift succeeds on SHA-256 block 0 and fails on block 1, because ctr sits between the
// domain and the message and changes per block. MinModulusBits = 2048 forces nine blocks,
// so the single-block case that would make the shift work cannot arise.
//
// It is an EXPLANATION with evidence, not a safety argument and not a field-order pin. The
// field order is already pinned three times over — TestPublishAndCreditFDHAreUnchanged,
// TestDemandFDHInputBindsTheEpochByteExactly and TestRelayAnchorDomainIsPinnedByteExactly
// each recompute `domain ‖ ctr ‖ msg` independently and compare against fullDomainHashD, so
// moving the counter reddens those. This test exists so the next reader of
// TestFDHDomainSetIsPairwisePrefixFree can tell a structural guard from a live exposure
// without re-deriving the algebra.
func TestAPrefixPairFailsToCollideOnlyBecauseTheCounterVaries(t *testing.T) {
	priv := testKey(t)
	pub := &priv.PublicKey

	nLen := (pub.N.BitLen() + 7) / 8
	blocks := (nLen + 8 + sha256.Size - 1) / sha256.Size
	if blocks < 2 {
		t.Fatalf("the expansion produces %d SHA-256 block(s) at a %d-bit modulus. With one block the "+
			"crafted prefix shift below COLLIDES and prefix-freeness becomes the only separation there "+
			"is — MinModulusBits = %d is load-bearing for this argument", blocks, pub.N.BitLen(), MinModulusBits)
	}

	// A prefix pair, hypothetical: domB extends domA. The suffix opens with four NULs so it
	// can absorb ctr = 0, which is what makes the block-0 shift work at all.
	domA := "silt/probe/prefix-pair"
	suffix := "\x00\x00\x00\x00xyz"
	domB := domA + suffix

	msgB := bytes.Repeat([]byte{0x5a}, SerialSize)
	// msgA is msgB shifted by the part of the suffix ctr(0) does not absorb.
	msgA := append([]byte("xyz"), append(make([]byte, 4), msgB...)...)

	block := func(domain string, msg []byte, ctr uint32) []byte {
		h := sha256.New()
		h.Write([]byte(domain))
		var cb [4]byte
		binary.BigEndian.PutUint32(cb[:], ctr)
		h.Write(cb[:])
		h.Write(msg)
		return h.Sum(nil)
	}

	if !bytes.Equal(block(domA, msgA, 0), block(domB, msgB, 0)) {
		t.Fatal("the crafted prefix shift does not collide on block 0, so the shift as written is " +
			"not the shift this test means to demonstrate. Both sides are computed here, from " +
			"constants in this function, so this is a self-check on the craft: msgA must be msgB " +
			"shifted by the part of the suffix that ctr(0) does not absorb.")
	}
	if bytes.Equal(block(domA, msgA, 1), block(domB, msgB, 1)) {
		t.Fatal("the crafted prefix shift collides on block 1 as well as block 0 — the counter no " +
			"longer separates the blocks, so a prefix pair is a REAL cross-domain collision. " +
			"TestFDHDomainSetIsPairwisePrefixFree is then load-bearing against a live forgery " +
			"rather than a structural guard, and this file's doc comment says the opposite. Fix " +
			"the comment in the same change.")
	}
	if fullDomainHashD(pub, msgA, domA).Cmp(fullDomainHashD(pub, msgB, domB)) == 0 {
		t.Fatal("a prefix pair produced one FDH value under two domains — cross-domain separation " +
			"has failed on the shipped function")
	}
}
