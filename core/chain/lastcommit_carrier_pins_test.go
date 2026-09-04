package chain

import (
	"crypto/sha256"
	"go/ast"
	"testing"

	"github.com/fxamacker/cbor/v2"
	"github.com/nerolabs/silt/ports"
)

// =============================================================================
// R-BOX-ATTESTS — the two STRUCTURAL pins the carrier's correctness rests on
// =============================================================================
//
// Both facts below are load-bearing and neither is behavioural, so both guards are
// structural — the R-ROTATE-EPOCH-LAST shape (rotate_epoch_last_drift_test.go). A purely
// behavioural fixture can pass through a refactor that reorders statements when the scenario
// does not happen to distinguish them.

// preCarrierUnsigned mirrors the pre-carrier unsigned Hash() body EXACTLY as it stood at
// pre-carrier origin/main (d7e4df0) — the eleven fields, in order, with their cbor keys. It is a FROZEN
// literal, deliberately NOT derived from Block, so a change to Block cannot move both sides of
// the comparison at once.
type preCarrierUnsigned struct {
	Height        uint64         `cbor:"1,keyasint"`
	Prev          ports.Hash     `cbor:"2,keyasint"`
	Entries       []ports.Entry  `cbor:"3,keyasint"`
	Proposer      []byte         `cbor:"4,keyasint"`
	Revocations   []ports.Hash   `cbor:"7,keyasint,omitempty"`
	Version       uint64         `cbor:"8,keyasint"`
	Unrevocations []ports.Hash   `cbor:"9,keyasint,omitempty"`
	BondRegs      []BondReg      `cbor:"10,keyasint,omitempty"`
	Slashes       []Equivocation `cbor:"11,keyasint,omitempty"`
	// Pruned is a [32]byte, so cbor's omitempty never omits it — key 14 is present in EVERY
	// encoded block body, zero-valued for a non-pruned block. It is part of the frozen bytes.
	Pruned    ports.Hash  `cbor:"14,keyasint,omitempty"`
	StateRoot *ports.Hash `cbor:"15,keyasint,omitempty"`
	LogRoot   *ports.Hash `cbor:"16,keyasint,omitempty"`
}

// TestCarrierHashDriftGuard pins the additive-compat property O1 requires: adding LastCommit to
// Hash() leaves the hash of EVERY carrier-free block BYTE-IDENTICAL to pre-carrier code. It
// recomputes the pre-carrier hash from the frozen struct above and requires equality for an
// era-2 block, an era-3 (v4) block with committed roots, and a v5 block with no carrier.
//
// The era-3 format is FROZEN (#632). If this guard ever reddens, the freeze is broken.
func TestCarrierHashDriftGuard(t *testing.T) {
	enc, err := cbor.CanonicalEncOptions().EncMode()
	if err != nil {
		t.Fatal(err)
	}
	sr := ports.HashBytes([]byte("state"))
	lr := ports.HashBytes([]byte("log"))
	k := key(58401)

	cases := []struct {
		name string
		b    Block
	}{
		{"era-2 v2", Block{Version: BlockVersionRounds, Height: 7, Prev: ports.HashBytes([]byte("p")),
			Entries: []ports.Entry{entry(1)}, Proposer: pubOf(k)}},
		{"era-3 v4 with roots", Block{Version: BlockVersionStateRoot, Height: 8, Prev: ports.HashBytes([]byte("p")),
			Entries: []ports.Entry{entry(2)}, Proposer: pubOf(k), StateRoot: &sr, LogRoot: &lr,
			Revocations: []ports.Hash{ports.HashBytes([]byte("r"))}}},
		{"era-4 v5 with NO carrier", Block{Version: BlockVersionWitnessable, Height: 9, Prev: ports.HashBytes([]byte("p")),
			Entries: []ports.Entry{entry(3)}, Proposer: pubOf(k), StateRoot: &sr, LogRoot: &lr}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			b := tc.b
			raw, mErr := enc.Marshal(preCarrierUnsigned{
				Height: b.Height, Prev: b.Prev, Entries: b.Entries, Proposer: b.Proposer,
				Revocations: b.Revocations, Version: b.Version, Unrevocations: b.Unrevocations,
				BondRegs: b.BondRegs, Slashes: b.Slashes, Pruned: b.Pruned,
				StateRoot: b.StateRoot, LogRoot: b.LogRoot,
			})
			if mErr != nil {
				t.Fatal(mErr)
			}
			want := ports.Hash(sha256.Sum256(raw))
			if got := b.Hash(); got != want {
				t.Fatalf("CARRIER HASH DRIFT: a carrier-free %s block no longer hashes to its "+
					"pre-carrier bytes.\n  got  %x\n  want %x\nThe era-3 format is FROZEN (#632) and "+
					"the carrier is ADDITIVE + omitempty — every block that carries no LastCommit must "+
					"hash byte-identically to pre-carrier origin/main.", tc.name, got, want)
			}
		})
	}

	// And the complement: a block that DOES carry one hashes DIFFERENTLY. Without this the guard
	// above would also pass if Hash() simply ignored the field — which is the R-BOX-ATTESTS defect.
	withNo := cases[2].b
	withYes := withNo
	withYes.LastCommit = []Attestation{{PubKey: pubOf(k), Sig: make([]byte, 64), Phase: PhasePrecommit}}
	if withNo.Hash() == withYes.Hash() {
		t.Fatal("Hash() does NOT cover LastCommit — the carrier's seating transition would ride on " +
			"unsigned bytes, which IS the defect R-BOX-ATTESTS closes")
	}
}

// TestCarrierFoldPrecedesBondRegsInApply is the STRUCTURAL apply-ORDER pin required by O1
// ("the carrier fold runs BEFORE this block's bond regs / TTL / slashes, pinned the way
// rotate-LAST is pinned"). Modelled on TestRotateEpochIsLastInApply.
//
// WHY IT IS LOAD-BEARING. The carrier's qualification screen must read the CHILD'S PRE-STATE —
// the parent's committed post-state, which is exactly the floor box's prevStateRoot. Move the
// fold below the bond-reg loop and it screens a MID-APPLY state that no committed root names:
// the chain would seat an id the box (anchored on prevStateRoot) does not, re-opening the S3
// chain/box divergence the box-entry round-A screen closed, and making the seating rule depend
// on same-block bond content.
//
// RED-on-injection (verified, then restored): moving `c.applyCarrier(b, parentProposer)` below
// the `for _, r := range canonicalBondRegs(b.BondRegs)` loop makes the recorded index of the
// bond-reg range statement smaller than the carrier call's, and this test fatals.
func TestCarrierFoldPrecedesBondRegsInApply(t *testing.T) {
	_, f := parseChainAST(t)
	apply := chainMethod(t, f, "apply")

	carrierAt, bondRegsAt, ttlAt, slashAt := -1, -1, -1, -1
	for i, st := range apply.Body.List {
		switch s := st.(type) {
		case *ast.ExprStmt:
			if callName(s.X) == "applyCarrier" {
				carrierAt = i
			}
		case *ast.RangeStmt:
			// The bond-reg loop ranges over canonicalBondRegs(b.BondRegs); the slash loop
			// ranges over b.Slashes.
			if ce, ok := s.X.(*ast.CallExpr); ok {
				if id, ok := ce.Fun.(*ast.Ident); ok && id.Name == "canonicalBondRegs" {
					bondRegsAt = i
				}
			}
			if sel, ok := s.X.(*ast.SelectorExpr); ok && sel.Sel.Name == "Slashes" {
				slashAt = i
			}
		case *ast.IfStmt:
			// The TTL sweep gate: `if ttl := c.cfg.BondTTLBlocks; ttl > 0 { ... }`.
			if s.Init != nil {
				if as, ok := s.Init.(*ast.AssignStmt); ok && len(as.Lhs) == 1 {
					if id, ok := as.Lhs[0].(*ast.Ident); ok && id.Name == "ttl" {
						ttlAt = i
					}
				}
			}
		}
	}
	if carrierAt < 0 {
		t.Fatal("apply() no longer calls c.applyCarrier(...) as a top-level statement — the era-4 " +
			"seating transition moved, and this pin no longer describes it (R-BOX-ATTESTS O1)")
	}
	for _, later := range []struct {
		name string
		at   int
	}{{"the bond-reg loop", bondRegsAt}, {"the TTL sweep", ttlAt}, {"the slash loop", slashAt}} {
		if later.at < 0 {
			t.Fatalf("apply() no longer contains %s as a top-level statement — the order pin cannot "+
				"be checked; re-derive it before trusting the carrier's pre-state screen", later.name)
		}
		if carrierAt > later.at {
			t.Fatalf("APPLY-ORDER PIN BROKEN: the carrier fold (stmt %d) runs AFTER %s (stmt %d). "+
				"O1 requires it FIRST, so the qualification screen reads the CHILD'S PRE-STATE — the "+
				"parent's committed post-state, which is the floor box's prevStateRoot. Screening a "+
				"mid-apply state re-opens the S3 chain/box divergence.", carrierAt, later.name, later.at)
		}
	}
}
