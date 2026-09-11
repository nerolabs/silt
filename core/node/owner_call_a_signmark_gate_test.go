package node

import (
	"crypto/ed25519"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/nerolabs/silt/adapters/markstore"
	"github.com/nerolabs/silt/core/chain"
	"github.com/nerolabs/silt/ports"
)

// =============================================================================
// G-PRE-3 — THE DURABLE SIGN-MARK MUST NOT BE RE-INTERPRETED BY THE ERA-4 UPGRADE.
// =============================================================================
//
// THE HAZARD, certification §5.4 (the seam the owner-call-A proposal hid).
//
// ports.SignMark is FSYNCED TO DISK before any consensus signature is released — it is silt's
// copy of Tendermint's priv_validator_state, i.e. the #397 artifact itself. slotCompare orders it
// by comparing the phase byte NUMERICALLY. So a mark written by a pre-upgrade binary as
// (H, r, PhasePrecommit = 2) and read back by a binary that PROBES with an era-4 constant
// (PhasePrepareV5 = 3) compares 3 > 2, does NOT block, and the node proposes and signs a different
// block at a height it has already precommitted.
//
// That is a self-manufactured double-sign produced BY THE UPGRADE, at exactly the boundary this
// change creates, and it is permanently slashable. Reachability is not exotic: the mark is durable
// across restarts and an era boundary IS a coordinated upgrade window, so a mark written under the
// old constants and read under the new ones is the EXPECTED case, not an edge one.
//
// THE FIX, certified, and it is one line of discipline rather than a migration:
//
//	THE WATERMARK RECORDS THE STEP. THE ATTESTATION RECORDS THE ERA-FORM.
//
// recordSign / recordSignLock / signAllowedAt / slotCompare keep taking the CANONICAL step
// (PhasePrepare / PhasePrecommit) at every era; only AttestAt, verifyAtt, the wire
// Attestation.Phase and the evidence form carry the era-4 constants. ports.SignMark's on-disk
// encoding is then BYTE-IDENTICAL across the upgrade — no migration, no #397 replay.
//
// This gate drives all three halves: the runtime restart, the on-disk bytes, and a SOURCE gate
// that no era-4 constant can reach a watermark function.

// --- PART A: THE RESTART, DRIVEN. ---------------------------------------------------------
//
// A validator precommits block X at (H, r) under a pre-upgrade binary. The process dies. A NEW
// process — the era-4 binary — loads the same durable mark. It must still refuse to sign a
// DIFFERENT block at that slot.
func TestGPRE3_UpgradeMustNotReinterpretTheDurableSignMark(t *testing.T) {
	const H, R = 7, 2
	blockX := ports.HashBytes([]byte("the block this validator already precommitted"))
	blockY := ports.HashBytes([]byte("a DIFFERENT block at the same slot"))

	// The durable mark as a PRE-UPGRADE binary wrote it: the canonical precommit step.
	store := markstore.NewMem()
	if err := store.Save(ports.SignMark{Height: H, Round: R, Phase: chain.PhasePrecommit, Hash: blockX}); err != nil {
		t.Fatalf("seed the pre-upgrade mark: %v", err)
	}

	// THE RESTART: a fresh Node — the era-4 binary — loading that mark off the same store.
	restarted := &Node{}
	if err := restarted.SetSignMarkStore(store); err != nil {
		t.Fatalf("restart: %v", err)
	}
	if !restarted.signMarkSet || restarted.signMark.Phase != chain.PhasePrecommit {
		t.Fatalf("GATE VACUOUS: the restarted node did not load the pre-upgrade mark (set=%v phase=%d) — "+
			"a gate over an unloaded mark carries zero bits", restarted.signMarkSet, restarted.signMark.Phase)
	}

	// --- THE SHIPPED PROPERTY. Every canonical probe at or below the marked slot REFUSES a
	// different block, one probe at a time. ---
	for _, step := range []uint8{chain.PhaseLegacy, chain.PhasePrepare, chain.PhasePrecommit} {
		if restarted.signAllowedAt(H, R, step, blockY) {
			t.Fatalf("G-PRE-3 VIOLATED: after an upgrade, a node holding a durable (H=%d, r=%d, precommit) "+
				"mark signed a DIFFERENT block at step %d. That is a self-manufactured double-sign produced "+
				"by the upgrade itself — the #397 crash variant, through the front door", H, R, step)
		}
	}
	// Idempotence is preserved: re-signing the SAME block at the marked slot is still allowed.
	if !restarted.signAllowedAt(H, R, chain.PhasePrecommit, blockX) {
		t.Fatal("G-PRE-3: an idempotent re-sign of the SAME block at the marked slot must stay allowed, " +
			"or a restarted validator loses its turn forever")
	}

	// --- THE ABLATION, DRIVEN: probe the SAME durable mark with the era-4 constants, which is
	// exactly what happens if the upgrade lets the v5 phase values into the watermark. ---
	for _, form := range []uint8{chain.PhasePrepareV5, chain.PhasePrecommitV5} {
		if !restarted.signAllowedAt(H, R, form, blockY) {
			t.Fatalf("G-PRE-3 GATE IS DECORATION: probing the durable mark with the era-4 constant %d must "+
				"NOT be blocked (%d > %d numerically) — if it is, this gate is not exercising slotCompare's "+
				"real arithmetic and its PASS proves nothing about the hazard", form, form, chain.PhasePrecommit)
		}
	}
	t.Logf("G-PRE-3 ablation RED as required: the durable mark (H=%d, r=%d, phase=%d) does NOT block a probe "+
		"at PhasePrepareV5 (%d) or PhasePrecommitV5 (%d) — letting the era-4 constants into the watermark "+
		"makes the node sign a different block at a height it already precommitted",
		H, R, chain.PhasePrecommit, chain.PhasePrepareV5, chain.PhasePrecommitV5)
}

// --- PART B: THE ON-DISK BYTES DO NOT MOVE. ------------------------------------------------
//
// The certified fix's whole claim is "no migration": ports.SignMark's persisted encoding is
// byte-identical across the era-4 upgrade. Driven against the REAL disk store, not the mem one.
func TestGPRE3_DurableSignMarkEncodingIsUnchangedAcrossTheUpgrade(t *testing.T) {
	path := filepath.Join(t.TempDir(), "signmark.json")
	d := markstore.New(path)
	want := ports.SignMark{Height: 41, Round: 3, Phase: chain.PhasePrecommit,
		Hash: ports.HashBytes([]byte("committed"))}
	if err := d.Save(want); err != nil {
		t.Fatalf("save: %v", err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	// The phase on disk is the CANONICAL step. If an era-4 constant ever appears here, every
	// previously persisted mark is being re-interpreted and the #397 replay window is open.
	if !strings.Contains(string(raw), `"phase":2`) {
		t.Fatalf("G-PRE-3 VIOLATED: the persisted mark does not carry the canonical precommit step (2). "+
			"On-disk: %s", raw)
	}
	for _, form := range []uint8{chain.PhasePrepareV5, chain.PhasePrecommitV5} {
		if strings.Contains(string(raw), `"phase":`+string(rune('0'+form))) {
			t.Fatalf("G-PRE-3 VIOLATED: an era-4 wire constant (%d) reached the DURABLE watermark. The "+
				"watermark records the STEP; the attestation records the ERA-FORM (T-STEP-VS-FORM). On-disk: %s",
				form, raw)
		}
	}
	got, ok, err := d.Load()
	if err != nil || !ok || got.Height != want.Height || got.Round != want.Round ||
		got.Phase != want.Phase || got.Hash != want.Hash {
		t.Fatalf("G-PRE-3 VIOLATED: the mark did not round-trip unchanged: %+v ok=%v err=%v", got, ok, err)
	}
}

// --- PART C: THE SOURCE GATE. --------------------------------------------------------------
//
// Parts A and B observe behaviour at two points. This one holds the RULE: no era-4 wire constant
// may be passed to any watermark function, anywhere in core/node, ever. A runtime gate can only
// ever observe the sites it happens to drive; a wedge introduced at a site no test reaches is
// exactly how the #432 drain-slot gate (chainrole.go, the h43 field wedge) came to be missing.
//
// The watermark functions, and why each is in the list:
//
//	signAllowedAt   the read — decides whether a signature may be released
//	recordSign      the write — advances the watermark
//	recordSignLock  the write with the #432 lock QC
//	slotCompare     the comparison both rest on, called directly by the h43 drain-slot gate
func TestGPRE3_NoEra4ConstantReachesTheWatermark(t *testing.T) {
	// phaseArg is the 0-based index of the phase parameter in each watermark function.
	phaseArg := map[string]int{
		"signAllowedAt":  2, // (height, round, phase, hash)
		"recordSign":     2, // (height, round, phase, hash)
		"recordSignLock": 2, // (height, round, phase, hash, lockQC)
		"slotCompare":    2, // (height, round, phase, mark)
	}
	// The alphabet the DURABLE watermark is frozen at. Anything else is a re-interpretation of
	// persisted state.
	canonical := map[string]bool{"PhaseLegacy": true, "PhasePrepare": true, "PhasePrecommit": true}

	fset := token.NewFileSet()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	var problems []string
	seen := map[string]int{}
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			var fn string
			switch c := call.Fun.(type) {
			case *ast.Ident:
				fn = c.Name
			case *ast.SelectorExpr:
				fn = c.Sel.Name
			}
			idx, watched := phaseArg[fn]
			if !watched || idx >= len(call.Args) {
				return true
			}
			seen[fn]++
			pos := fset.Position(call.Pos())
			arg := call.Args[idx]
			// Accept the canonical constants (chain.PhaseX or a bare PhaseX) and a pass-through
			// parameter named `phase`/`step` — the latter is recordSign's own signature forwarding
			// to recordSignLock, which cannot introduce a constant.
			switch a := arg.(type) {
			case *ast.SelectorExpr:
				if id, ok := a.X.(*ast.Ident); ok && id.Name == "chain" && canonical[a.Sel.Name] {
					return true
				}
			case *ast.Ident:
				if canonical[a.Name] || a.Name == "phase" || a.Name == "step" {
					return true
				}
			}
			problems = append(problems, "  "+pos.Filename+":"+itoa(pos.Line)+"  "+fn+
				" is handed a phase this gate cannot prove is a CANONICAL step")
			return true
		})
	}
	// STALE-SITE CHECK: every watched function must actually be called, or this gate is green over
	// a mechanism that no longer exists.
	var missing []string
	for fn := range phaseArg {
		if seen[fn] == 0 {
			missing = append(missing, fn)
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		t.Fatalf("G-PRE-3 SOURCE GATE IS VACUOUS: no non-test call site found for %v. The watermark "+
			"functions were renamed or deleted; re-derive this gate rather than leaving it green over "+
			"nothing", missing)
	}
	if len(problems) > 0 {
		sort.Strings(problems)
		t.Fatalf("G-PRE-3 SOURCE GATE — an era-4 wire constant can reach the DURABLE watermark:\n%s\n\n"+
			"  THE WATERMARK RECORDS THE STEP; THE ATTESTATION RECORDS THE ERA-FORM (T-STEP-VS-FORM,\n"+
			"  certification §5.4). ports.SignMark is fsynced to disk and slotCompare orders it\n"+
			"  NUMERICALLY, so a mark written (H, r, PhasePrecommit=2) and probed with 3 compares 3 > 2,\n"+
			"  is NOT blocked, and the node signs a different block at a height it already precommitted.\n"+
			"  Keep the watermark's alphabet frozen at {0,1,2} and let the WIRE alphabet grow:\n"+
			"  chain.AttestAt maps the canonical step to the block's era form on its own.",
			strings.Join(problems, "\n"))
	}
	t.Logf("G-PRE-3 source gate: %d watermark call sites, all on the canonical {PhaseLegacy, PhasePrepare, "+
		"PhasePrecommit} alphabet", seen["signAllowedAt"]+seen["recordSign"]+seen["recordSignLock"]+seen["slotCompare"])
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

// --- THE RUNTIME COMPANION: the producer records the STEP while the wire carries the FORM. ---
//
// The source gate above proves no era-4 constant is PASSED to the watermark. This proves the other
// half at runtime: for a v5 block, the attestation on the wire carries PhasePrecommitV5 while the
// slot the node reserves is the canonical precommit. If those two ever converged, either the wire
// would lose its era discriminator (the G-PRE-1 wedge) or the watermark would gain one (the #397
// replay above).
func TestGPRE3_WireCarriesTheFormWhileTheWatermarkCarriesTheStep(t *testing.T) {
	cid := ports.HashBytes([]byte("a chain"))
	_, k, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	v5 := &chain.Block{Version: chain.BlockVersionWitnessable, Height: 9,
		Prev: ports.HashBytes([]byte("p")), Entries: []ports.Entry{{Root: ports.HashBytes([]byte("e"))}}}
	v2 := &chain.Block{Version: chain.BlockVersionRounds, Height: 9,
		Prev: ports.HashBytes([]byte("p")), Entries: []ports.Entry{{Root: ports.HashBytes([]byte("e"))}}}

	for _, step := range []uint8{chain.PhasePrepare, chain.PhasePrecommit} {
		gotV5 := chain.AttestAt(v5, k, 1, step, cid).Phase
		wantV5 := chain.AttPhase(chain.BlockVersionWitnessable, step)
		if gotV5 != wantV5 || gotV5 == step {
			t.Fatalf("step %d: a v5 attestation must carry the ERA-4 form on the wire, got phase %d (want %d)",
				step, gotV5, wantV5)
		}
		if got := chain.AttestAt(v2, k, 1, step, cid).Phase; got != step {
			t.Fatalf("step %d: a v2 attestation must carry the canonical step unchanged, got %d", step, got)
		}
		// And the slot the node reserves alongside it is the CANONICAL step — the value the source
		// gate above proves is the only thing that reaches recordSign.
		n := &Node{}
		if !n.recordSign(v5.Height, 1, step, v5.Hash()) {
			t.Fatalf("step %d: recordSign must succeed", step)
		}
		if n.signMark.Phase != step {
			t.Fatalf("step %d: the watermark must record the CANONICAL step, got %d — the durable mark's "+
				"alphabet is frozen at {0,1,2}", step, n.signMark.Phase)
		}
	}
}
