package chain

import (
	"crypto/ed25519"
	"go/ast"
	"go/parser"
	"go/token"
	"sort"
	"strings"
	"testing"

	"github.com/nerolabs/silt/ports"
)

// =============================================================================
// O3 DIRECTION T — GATE (b): R-INTERLOCK-GATE
// =============================================================================
//
// Binding spec: O3-Direction-T-I5-restatement-and-divergence-RESEARCH-CERTIFICATION-2026-09-04.md
// §8.2 (b1)+(b2)+(b3). Prior: RULING-O3-fork-choice-weight-R-vs-T-2026-09-03.md §5 — the interlock
// ("never repair the blockWeight verify alone") had ZERO test enforcement; the PE applied the
// forbidden one-line repair and ./core/... + ./sim/... stayed green
// (scar-binding-interlock-without-a-test).
//
// The interlock is mechanised by making the HAZARD unbuildable: `heavier` may read nothing that is
// not Hash()-covered. Three parts:
//
//	(b1) the purity pin      — an AST walk of heavier: the only selectors it may read on the two
//	                           *Chain arguments are blocks[...].Height and blocks[...].Hash().
//	(b2) the determinism     — two replicas holding the same committed chain and DIFFERENT valid
//	     oracle                certificates for its blocks select the same head, in the finality
//	                           posture (adopt-bit) and in the no-finality posture (head itself).
//	(b3) fast/slow equality  — Append-each (what appendExtension does) and own-prefix+served ->
//	                           Reconcile (what reconstructFork does) yield the same head.
//
// SOURCE GATE: (b1) walks chain.go with go/ast; it sees selectors, not behaviour.
// RUNTIME GATE: TestO3T_CertificateVariantNeverRanksHeavier,
// TestO3T_ForkChoiceIsCertificateIndependentWithoutFinality, TestO3T_FastSlowPathSameHead.

// o3tHeavierDenied is the explicit denylist for heavier's body: every name here is a
// non-Hash()-covered or replica-local read. Asserted against the allowlist so it cannot be
// re-admitted quietly.
var o3tHeavierDenied = map[string]string{
	"Weight":            "the retired fork-choice weight (O3 Direction T)",
	"Atts":              "a certificate slot, outside Hash()",
	"PrepareQC":         "a certificate slot, outside Hash()",
	"CommitRound":       "a certificate slot, outside Hash()",
	"LastCommit":        "hash-covered on v5, but a CERTIFICATE by content — fork-choice reads no certificate",
	"bonded":            "replica-local applied state",
	"epochSet":          "replica-local applied state",
	"slashed":           "replica-local applied state",
	"matureEpoch":       "replica-local latch",
	"everMature":        "replica-local latch",
	"rep":               "the legacy reputation view — replica-local by construction",
	"attesterQualified": "reads bonded/epochSet/launchAnchor — the #357 oscillation mechanism",
	"launchAnchor":      "reads handedOff()",
	"cfg":               "own-config — a fork-choice input must be shared by every replica, not configured",
	"Pruned":            "attacker-choosable where no independent hash binds it (R0.6)",
}

// o3tHeavierAllowedOnBlock is the whole allowlist: selectors permitted on an element of
// a.blocks / b.blocks.
var o3tHeavierAllowedOnBlock = map[string]bool{"Height": true, "Hash": true}

// o3tHeavierAllowedCalls is the set of free functions heavier's body may call. Anything else could
// smuggle a read through a helper.
var o3tHeavierAllowedCalls = map[string]bool{"len": true, "bytesLess": true}

// o3tWalkHeavier classifies every read in a `heavier(a, b *Chain) bool` body and returns the
// violations. Shared by the pin and the teeth test.
func o3tWalkHeavier(fset *token.FileSet, fd *ast.FuncDecl) []string {
	var violations []string
	params := map[string]bool{}
	if fd.Type.Params != nil {
		for _, f := range fd.Type.Params.List {
			for _, n := range f.Names {
				params[n.Name] = true
			}
		}
	}
	if len(params) != 2 || !params["a"] || !params["b"] {
		violations = append(violations, "heavier's parameters are not exactly (a, b *Chain)")
	}
	// rootParam returns the parameter name an expression is rooted at (a or b), or "".
	var rootParam func(e ast.Expr) string
	rootParam = func(e ast.Expr) string {
		switch x := e.(type) {
		case *ast.Ident:
			if params[x.Name] {
				return x.Name
			}
		case *ast.SelectorExpr:
			return rootParam(x.X)
		case *ast.IndexExpr:
			return rootParam(x.X)
		case *ast.CallExpr:
			return rootParam(x.Fun)
		case *ast.ParenExpr:
			return rootParam(x.X)
		case *ast.SliceExpr:
			return rootParam(x.X)
		}
		return ""
	}
	ast.Inspect(fd.Body, func(n ast.Node) bool {
		pos := func(p token.Pos) string { return itoa(fset.Position(p).Line) }
		switch x := n.(type) {
		case *ast.SelectorExpr:
			root := rootParam(x.X)
			if root == "" {
				return true
			}
			name := x.Sel.Name
			switch inner := x.X.(type) {
			case *ast.SelectorExpr:
				// a.<x>.<sel>: the inner a.<x> is flagged on its own visit; do not double-report.
				return true
			case *ast.Ident:
				// a.<sel>: only blocks.
				if name != "blocks" {
					why := o3tHeavierDenied[name]
					if why == "" {
						why = "not Height/Hash of a block — unclassified read"
					}
					violations = append(violations, "chain.go:"+pos(x.Pos())+"  "+root+"."+name+"  — "+why)
				}
			case *ast.IndexExpr:
				// a.blocks[i].<sel>: only Height / Hash.
				if s, ok := inner.X.(*ast.SelectorExpr); ok && s.Sel.Name == "blocks" && o3tHeavierAllowedOnBlock[name] {
					return true
				}
				why := o3tHeavierDenied[name]
				if why == "" {
					why = "not Height/Hash of a block — unclassified read"
				}
				violations = append(violations, "chain.go:"+pos(x.Pos())+"  "+root+".blocks[...]."+name+"  — "+why)
			default:
				violations = append(violations, "chain.go:"+pos(x.Pos())+"  "+root+"...."+name+"  — a read through an unexpected expression shape")
			}
		case *ast.CallExpr:
			if id, ok := x.Fun.(*ast.Ident); ok && !o3tHeavierAllowedCalls[id.Name] {
				violations = append(violations, "chain.go:"+pos(x.Pos())+"  call to "+id.Name+"()  — heavier may call only len and bytesLess")
			}
		}
		return true
	})
	sort.Strings(violations)
	return violations
}

func o3tFindFunc(t *testing.T, fset *token.FileSet, af *ast.File, name string) *ast.FuncDecl {
	t.Helper()
	for _, decl := range af.Decls {
		if fd, ok := decl.(*ast.FuncDecl); ok && fd.Name.Name == name && fd.Recv == nil {
			return fd
		}
	}
	t.Fatalf("SOURCE GATE: free function %s not found in chain.go — if fork-choice moved or was renamed, move this pin with it", name)
	return nil
}

// TestO3T_HeavierReadsOnlyHeightAndHeadHash is gate (b1), the fork-choice purity pin. RED on main
// at 59509b1 (chain.go:4151 reads a.Weight()/b.Weight()); GREEN after T.
func TestO3T_HeavierReadsOnlyHeightAndHeadHash(t *testing.T) {
	for name := range o3tHeavierDenied {
		if o3tHeavierAllowedOnBlock[name] {
			t.Fatalf("SOURCE GATE: PIN CORRUPTED — %q is both denied and allowed on a block", name)
		}
	}
	if o3tHeavierDenied["Weight"] == "" || len(o3tHeavierAllowedOnBlock) != 2 {
		t.Fatal("SOURCE GATE: PIN CORRUPTED — the allowlist must be exactly {Height, Hash} and Weight must be denied")
	}
	fset := token.NewFileSet()
	af, err := parser.ParseFile(fset, "chain.go", nil, 0)
	if err != nil {
		t.Fatalf("parse chain.go: %v", err)
	}
	fd := o3tFindFunc(t, fset, af, "heavier")
	if v := o3tWalkHeavier(fset, fd); len(v) > 0 {
		t.Fatalf("SOURCE GATE: heavier reads outside {blocks[...].Height, blocks[...].Hash()} (%d site(s)):\n  %s\n\n"+
			"  Fork-choice must be a pure function of Hash()-covered content (I5). A certificate slot or a\n"+
			"  replica-local field makes two honest replicas rank the same forks differently — the\n"+
			"  interlock the PE showed had no enforcement (RULING-O3 §5). Retire the term (Direction T);\n"+
			"  do not add the name to the allowlist.", len(v), strings.Join(v, "\n  "))
	}
	// bytesLess is the one helper allowed. Pin that it takes no *Chain, so it cannot become a door.
	bl := o3tFindFunc(t, fset, af, "bytesLess")
	for _, f := range bl.Type.Params.List {
		if star, ok := f.Type.(*ast.StarExpr); ok {
			if id, ok := star.X.(*ast.Ident); ok && id.Name == "Chain" {
				t.Fatal("SOURCE GATE: bytesLess takes a *Chain — the one allowed helper has become a door into chain state")
			}
		}
	}
}

// TestO3T_HeavierPinHasTeeth proves the walk bites: a synthetic heavier that re-injects
// len(a.blocks[len-1].Atts) (the cert §8.2 RED-first injection), a.Weight(), and a helper call
// is flagged at each site.
func TestO3T_HeavierPinHasTeeth(t *testing.T) {
	const injected = `package chain

func heavier(a, b *Chain) bool {
	na, nb := len(a.blocks[len(a.blocks)-1].Atts), len(b.blocks[len(b.blocks)-1].Atts)
	if na != nb {
		return na > nb
	}
	if wa, wb := a.Weight(), b.Weight(); wa != wb {
		return wa > wb
	}
	if a.cfg.MinBond > 0 && weightOf(a) > 0 {
		return true
	}
	ah, bh := a.blocks[len(a.blocks)-1].Height, b.blocks[len(b.blocks)-1].Height
	if ah != bh {
		return ah > bh
	}
	ha, hb := a.blocks[len(a.blocks)-1].Hash(), b.blocks[len(b.blocks)-1].Hash()
	return bytesLess(ha[:], hb[:])
}
`
	fset := token.NewFileSet()
	af, err := parser.ParseFile(fset, "chain.go", injected, 0)
	if err != nil {
		t.Fatalf("parse synthetic: %v", err)
	}
	v := o3tWalkHeavier(fset, o3tFindFunc(t, fset, af, "heavier"))
	joined := strings.Join(v, "\n")
	for _, want := range []string{"a.blocks[...].Atts", "b.blocks[...].Atts", "a.Weight", "b.Weight", "a.cfg", "call to weightOf()"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("SOURCE GATE: PIN HAS NO TEETH — injected read %q was not flagged. Flagged:\n%s", want, joined)
		}
	}
	if strings.Contains(joined, "].Height") || strings.Contains(joined, "].Hash") {
		t.Fatalf("SOURCE GATE: PIN OVER-BITES — Height/Hash reads were flagged:\n%s", joined)
	}
	if len(v) != 6 {
		t.Fatalf("SOURCE GATE: PIN TEETH COUNT — %d violation(s) flagged, want exactly 6 (Atts x2, Weight x2, cfg, weightOf):\n%s", len(v), joined)
	}
}

// -----------------------------------------------------------------------------
// (b2) THE CERTIFICATE-VARIANT DETERMINISM ORACLE
// -----------------------------------------------------------------------------

// o3tEra2Block builds an era-2 block at height h on prev with entry tag, proposer keys[0]
// (round-scoped self-prepare + count-neutral self-precommit), and prepare+precommit signatures
// from the given non-proposer signers, all at round. CommitRound = round.
func o3tEra2Block(keys []ed25519.PrivateKey, signers []ed25519.PrivateKey, h uint64, prev ports.Hash, tag byte, round uint64) *Block {
	b := &Block{Version: BlockVersionRounds, Height: h, Prev: prev, Entries: []ports.Entry{entry(tag)}}
	Sign(b, keys[0])
	b.CommitRound = round
	b.PrepareQC = append(b.PrepareQC, AttestAt(b, keys[0], round, PhasePrepare))
	for _, k := range signers {
		b.PrepareQC = append(b.PrepareQC, AttestAt(b, k, round, PhasePrepare))
	}
	b.Atts = append(b.Atts, AttestAt(b, keys[0], round, PhasePrecommit))
	for _, k := range signers {
		b.Atts = append(b.Atts, AttestAt(b, k, round, PhasePrecommit))
	}
	return b
}

// TestO3T_CertificateVariantNeverRanksHeavier is gate (b2) in the FINALITY posture (roundsWorld:
// objective + ByzantineQuorum, 4 anchors — P1a). Two replicas append the same block under two
// different valid certificates; each then Reconciles the other's chain. The certificate variant is
// the same committed chain, so it must never be ranked heavier: adopted == false in BOTH
// directions, heads equal throughout.
//
//	arm 1: one extra genuine same-round precommit (+prepare)   — |cert| differs
//	arm 2: a different-round (PrepareQC_r', Atts_r') pair, CommitRound = r' — CommitRound is not hash-covered
//
// Both must Append (the oracle is scoped to forks both replicas ADMIT). RED under the recorded
// controlled revert (a heavier that reads len(head.Atts)): arm 1's thinner-certificate replica
// ADOPTS the thicker copy. GREEN after T; GREEN on main today only because the dead term is 0.
func TestO3T_CertificateVariantNeverRanksHeavier(t *testing.T) {
	arms := []struct {
		name    string
		variant func(keys []ed25519.PrivateKey, g *Block) *Block
	}{
		{"same-round-extra-precommit", func(keys []ed25519.PrivateKey, g *Block) *Block {
			return o3tEra2Block(keys, keys[1:4], 1, g.Hash(), 9, 0)
		}},
		{"different-round-certificate", func(keys []ed25519.PrivateKey, g *Block) *Block {
			return o3tEra2Block(keys, keys[1:3], 1, g.Hash(), 9, 1)
		}},
	}
	for _, arm := range arms {
		t.Run(arm.name, func(t *testing.T) {
			r1, keys, g := roundsWorld(t)
			r2, _, g2 := roundsWorld(t)
			if g.Hash() != g2.Hash() {
				t.Fatal("fixture: the two replicas must share a genesis")
			}
			if !r1.finalityQuorumActive() {
				t.Fatal("fixture: this arm is the FINALITY posture; the gate must be engaged")
			}
			base := o3tEra2Block(keys, keys[1:3], 1, g.Hash(), 9, 0)
			variant := arm.variant(keys, g)
			if base.Hash() != variant.Hash() {
				t.Fatal("fixture: the certificate variant must be the SAME block (Hash() excludes Atts/PrepareQC/CommitRound)")
			}
			if len(base.Atts) == len(variant.Atts) && base.CommitRound == variant.CommitRound {
				t.Fatal("fixture: the variant must differ in certificate size or round")
			}
			if err := r1.Append(*base); err != nil {
				t.Fatalf("r1 must ADMIT the base certificate: %v", err)
			}
			if err := r2.Append(*variant); err != nil {
				t.Fatalf("r2 must ADMIT the variant certificate: %v", err)
			}
			h1, n1 := r1.Head()
			h2, n2 := r2.Head()
			if h1 != h2 || n1 != n2 {
				t.Fatalf("heads differ after append: %x@%d vs %x@%d", h1[:4], n1, h2[:4], n2)
			}
			// Cross-reconcile. The fork CONTAINS each replica's head, so the finality gate admits it;
			// the only thing left to decide is ranking — and it must be indifferent to the certificate.
			for _, dir := range []struct {
				name string
				me   *Chain
				peer *Chain
			}{{"r1<-r2", r1, r2}, {"r2<-r1", r2, r1}} {
				adopted, err := dir.me.Reconcile(dir.peer.Blocks(0))
				if err != nil {
					t.Fatalf("%s: Reconcile must admit the same committed chain: %v", dir.name, err)
				}
				if adopted {
					t.Fatalf("I5 VIOLATION (%s, %s): a replica ADOPTED a certificate variant of its own committed head — "+
						"fork-choice ranked by something outside Hash() (the certificate). Two honest replicas holding the "+
						"same chain with different valid certificates would each reorg onto the other's bytes.", dir.name, arm.name)
				}
			}
			h1, n1 = r1.Head()
			h2, n2 = r2.Head()
			if h1 != h2 || n1 != n2 {
				t.Fatalf("heads differ after cross-reconcile: %x@%d vs %x@%d", h1[:4], n1, h2[:4], n2)
			}
		})
	}
}

// o3tNoFinalityWorld builds an OBJECTIVE chain in the documented no-finality posture (cert §4.3
// P2-off: -byzantine-quorum=false with Quorum below bftThreshold(N)): 7 anchors, Quorum 1,
// ByzantineQuorum off. Every valid genesis-rooted fork is admitted, so the RANKING itself is
// observable — the posture the cert says T makes replica-independent for the first time.
func o3tNoFinalityWorld(t *testing.T) (*Chain, []ed25519.PrivateKey, *Block) {
	t.Helper()
	keys := make([]ed25519.PrivateKey, 7)
	anchors := map[ports.NodeID]bool{}
	for i := range keys {
		keys[i] = key(int64(12000 + i))
		anchors[idOf(keys[i])] = true
	}
	cfg := Config{Quorum: 1, MinBond: 1 << 20, ByzantineQuorum: false, Anchors: anchors, AnchorQuorum: 1, MatureValidators: 99}
	c := New(cfg, func(ports.NodeID) int64 { return 0 })
	c.SetBondVerifier(objectiveVerify)
	g := &Block{Version: 1, Height: 0, Entries: []ports.Entry{entry(0)}}
	Sign(g, keys[0])
	if err := c.AppendGenesis(*g); err != nil {
		t.Fatalf("genesis: %v", err)
	}
	if !c.objective() {
		t.Fatal("fixture: must be objective")
	}
	if c.finalityQuorumActive() {
		t.Fatal("fixture: this is the NO-finality posture; the gate must be off (Quorum 1 < bftThreshold(7))")
	}
	return c, keys, g
}

// TestO3T_ForkChoiceIsCertificateIndependentWithoutFinality is gate (b2)'s head-level face. In the
// no-finality posture two replicas hold EQUAL-HEIGHT conflicting heads b2x (r1) and b2y (r2) on a
// shared prefix; r1 holds b2x with a THICK certificate (6 signers) and r2 is served b2x with a
// THIN but valid one (3 signers). After each Reconciles the other's fork, both must hold the same
// head: under T the order is (height, head-hash), a pure function of the two chains.
//
// The fixture picks b2y so that b2y.Hash() < b2x.Hash(), and asserts it. Under the recorded
// controlled revert (heavier reads len(head.Atts)) r1 keeps b2x (6 > 3) while r2 ties 3 == 3 and
// falls to the hash (b2y): the two honest replicas DIVERGE. RED under the revert; GREEN after T.
func TestO3T_ForkChoiceIsCertificateIndependentWithoutFinality(t *testing.T) {
	r1, keys, g := o3tNoFinalityWorld(t)
	r2, _, g2 := o3tNoFinalityWorld(t)
	if g.Hash() != g2.Hash() {
		t.Fatal("fixture: shared genesis")
	}
	// Shared prefix: b1, same certificate on both.
	b1 := o3tEra2Block(keys, keys[1:], 1, g.Hash(), 1, 0)
	for _, r := range []*Chain{r1, r2} {
		if err := r.Append(*b1); err != nil {
			t.Fatalf("b1: %v", err)
		}
	}
	// r1's head: b2x, thick certificate. r2 is served b2x with a thin certificate (3 non-proposer
	// anchors: with the proposer that is the strict 4-of-7 anchor majority).
	b2xThick := o3tEra2Block(keys, keys[1:], 2, b1.Hash(), 40, 0)
	b2xThin := o3tEra2Block(keys, keys[1:4], 2, b1.Hash(), 40, 0)
	if b2xThick.Hash() != b2xThin.Hash() {
		t.Fatal("fixture: thick and thin are the same block")
	}
	// r2's head: b2y with the thin certificate, chosen so that b2y sorts BELOW b2x.
	var b2y *Block
	for tag := byte(41); tag < 250; tag++ {
		cand := o3tEra2Block(keys, keys[1:4], 2, b1.Hash(), tag, 0)
		hx, hy := b2xThick.Hash(), cand.Hash()
		if bytesLess(hy[:], hx[:]) {
			b2y = cand
			break
		}
	}
	if b2y == nil {
		t.Fatal("fixture: could not find a b2y below b2x in 200 tags")
	}
	if err := r1.Append(*b2xThick); err != nil {
		t.Fatalf("r1 b2x(thick): %v", err)
	}
	if err := r2.Append(*b2y); err != nil {
		t.Fatalf("r2 b2y(thin): %v", err)
	}
	// Each replica receives the other's fork. r2 receives b2x under the THIN certificate.
	fork1 := []Block{*g, *b1, *b2y}
	fork2 := []Block{*g, *b1, *b2xThin}
	if _, err := r1.Reconcile(fork1); err != nil {
		t.Fatalf("r1 must ADMIT fork [g,b1,b2y]: %v", err)
	}
	if _, err := r2.Reconcile(fork2); err != nil {
		t.Fatalf("r2 must ADMIT fork [g,b1,b2x-thin]: %v", err)
	}
	h1, n1 := r1.Head()
	h2, n2 := r2.Head()
	hx, hy := b2xThick.Hash(), b2y.Hash()
	if h1 != h2 || n1 != n2 {
		t.Fatalf("I5 VIOLATION — two honest replicas that admitted the same two forks picked DIFFERENT heads: "+
			"r1=%x@%d r2=%x@%d. Fork-choice read the certificate (how many signers each replica happened to hold), "+
			"not the committed chain. (b2x=%x b2y=%x)", h1[:4], n1, h2[:4], n2, hx[:4], hy[:4])
	}
	if h1 != hy {
		t.Fatalf("under height -> head-hash both replicas converge on the LOWER head hash b2y=%x, got %x", hy[:4], h1[:4])
	}
}

// -----------------------------------------------------------------------------
// (b3) FAST-PATH / SLOW-PATH EQUIVALENCE — chain-level
// -----------------------------------------------------------------------------

// TestO3T_FastSlowPathSameHead is gate (b3) at the chain tier. The fast path (core/node
// appendExtension) is Append-each of the served window; the slow path (reconstructFork -> Reconcile)
// is own-prefix + served window replayed in a throwaway replica and ranked by heavier. On the same
// served strict extension both must land on the same head, EVEN when the two servings carry
// different valid certificates for the suffix. The core/node twin drives the real functions
// (core/node/o3t_interlock_fastslow_test.go).
//
// RED-first: a heavier that ranks by the HEAD certificate's size (a non-extension-monotone
// certificate term) refuses the slow path's thinner-certificate extension while the fast path
// appends it — recorded in the Tester's memory. The cumulative len(Atts) revert does NOT redden
// this gate (a positive cumulative term is extension-monotone); the (b2) oracles carry that revert.
func TestO3T_FastSlowPathSameHead(t *testing.T) {
	fast, keys, g := roundsWorld(t)
	slow, _, _ := roundsWorld(t)
	b1 := o3tEra2Block(keys, keys[1:], 1, g.Hash(), 1, 0)
	for _, r := range []*Chain{fast, slow} {
		if err := r.Append(*b1); err != nil {
			t.Fatalf("b1: %v", err)
		}
	}
	// The served suffix: b2 under two valid certificates — thick to the fast node, thin to the slow.
	b2Thick := o3tEra2Block(keys, keys[1:], 2, b1.Hash(), 2, 0)
	b2Thin := o3tEra2Block(keys, keys[1:3], 2, b1.Hash(), 2, 0)
	if b2Thick.Hash() != b2Thin.Hash() {
		t.Fatal("fixture: same block")
	}
	// Fast path: appendExtension == Append each served block that extends our head.
	if err := fast.Append(*b2Thick); err != nil {
		t.Fatalf("fast path Append: %v", err)
	}
	// Slow path: reconstructFork == own prefix below the served start + the served window.
	full := append(slow.Blocks(0), *b2Thin)
	adopted, err := slow.Reconcile(full)
	if err != nil {
		t.Fatalf("slow path Reconcile: %v", err)
	}
	if !adopted {
		t.Fatal("I4 VIOLATION — the slow path REFUSED a valid strict extension the fast path appended: " +
			"fork-choice ranked the extension below the current head. Under height -> head-hash a strict " +
			"extension is always taller, so the two sync paths cannot disagree.")
	}
	hf, nf := fast.Head()
	hs, ns := slow.Head()
	if hf != hs || nf != ns {
		t.Fatalf("fast/slow path heads differ: fast=%x@%d slow=%x@%d", hf[:4], nf, hs[:4], ns)
	}
}
