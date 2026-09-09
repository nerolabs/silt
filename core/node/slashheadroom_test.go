package node

import (
	"strings"
	"testing"

	"github.com/nerolabs/silt/core/chain"
)

// G-SLASHCAP-1 (the DRIVEN ablation, owner call 2026-09-09 "CLOSE THE ROUTE").
//
// SlashesBytesCap is a CONSENSUS VALIDITY rule enforced on every validator
// (core/chain/validate_v5_predicates.go). Its value was derived from the DEFAULTS of two
// flags that are proposer-side only (core/node/chainrole.go foldPendingBondRegs,
// core/node/entrypool.go foldPendingEntries) and whose shipped help documents
// "0 = unbounded". Nothing bound the configured value to the derivation, so an operator
// could raise its own per-block bond-reg budget past the point where a legitimate evidence
// pair no longer fits the cap — and a REAL double-signer's evidence is then rejected by the
// cap, so the equivocator KEEPS ITS SEAT. That is an accountability break (a Part-0
// corner), reached from local config alone.
//
// This is the #380 class: a consensus quantity that is a function of LOCAL CONFIG rather
// than of the chain. #380 was RequiredQuorum() reading cfg.Quorum; this is
// SlashesBytesCap's invariant reading cfg.MaxBondRegBytesPerBlock.
//
// The rows below DRIVE the boundary, they do not describe it. Each names the runtime
// config values the gate reads, never the constants that feed them.
func TestG_SLASHCAP_1_ConfigCannotDefeatSlashEvidence(t *testing.T) {
	const mib = 1 << 20

	rows := []struct {
		name       string
		bondRegs   int64
		entries    int64
		wantRefuse bool
		because    string
	}{
		{
			name: "shipped defaults are inside the derivation", bondRegs: 2 * mib, entries: 64 << 10,
			wantRefuse: false,
			because:    "2*(2 MiB + 64 KiB + slack) is ~4.25 MiB against a 16 MiB cap — the derivation's own arithmetic",
		},
		{
			name: "the boundary itself is admitted", bondRegs: maxHonestBondRegBytes(64 << 10), entries: 64 << 10,
			wantRefuse: false,
			because:    "the largest budget the invariant admits must PASS, or the gate is stricter than the rule it protects",
		},
		{
			name: "one byte past the boundary is refused", bondRegs: maxHonestBondRegBytes(64<<10) + 1, entries: 64 << 10,
			wantRefuse: true,
			because:    "past here an honest pair exceeds the cap and a real double-signer keeps its seat",
		},
		{
			name: "the audit's ~7.9 MiB figure is refused", bondRegs: 8 * mib, entries: 64 << 10,
			wantRefuse: true,
			because:    "2*(8 MiB + 64 KiB) = 16.125 MiB > 16 MiB — the reported break",
		},
		{
			name: "an unbounded bond-reg budget is refused", bondRegs: 0, entries: 64 << 10,
			wantRefuse: true,
			because:    "the flag's help documents 0 = unbounded, and an unbounded honest block makes the invariant unsatisfiable",
		},
		{
			name: "an unbounded entry budget is refused", bondRegs: 2 * mib, entries: 0,
			wantRefuse: true,
			because:    "same door, the other flag",
		},
		{
			name: "a fat entry budget is refused even with a small reg budget", bondRegs: 64 << 10, entries: 9 * mib,
			wantRefuse: true,
			because:    "the cap bounds the WHOLE body; entries are not exempt from it",
		},
		{
			name: "a negative budget is refused", bondRegs: -1, entries: 64 << 10,
			wantRefuse: true,
			because:    "a negative budget is read as unbounded by the proposer's `budget > 0` guard",
		},
	}

	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			cfg := Config{
				MaxBondRegBytesPerBlock: row.bondRegs,
				MaxEntryBytesPerBlock:   row.entries,
			}
			err := CheckSlashEvidenceHeadroom(cfg)
			if row.wantRefuse && err == nil {
				t.Fatalf("GATE VACUOUS: budgets (bondregs=%d, entries=%d) must be REFUSED — %s. "+
					"A node started on this config makes its own equivocation unprovable: 2*(body) > SlashesBytesCap (%d), "+
					"so CheckEquivocation never gets to run and the seat survives.",
					row.bondRegs, row.entries, row.because, int64(chain.SlashesBytesCap))
			}
			if !row.wantRefuse && err != nil {
				t.Fatalf("GATE TOO STRICT: budgets (bondregs=%d, entries=%d) must be ADMITTED — %s. got: %v",
					row.bondRegs, row.entries, row.because, err)
			}
			if err != nil && !strings.Contains(err.Error(), "SlashesBytesCap") {
				t.Fatalf("the refusal must NAME the rule it protects so an operator can act on it; got %q", err)
			}
		})
	}
}

// The shipped DefaultConfig must satisfy the invariant. If a future edit moves a default
// past the boundary this fails here rather than in the field, where the symptom is an
// equivocator that cannot be evicted.
func TestG_SLASHCAP_2_ShippedDefaultsSatisfyTheInvariant(t *testing.T) {
	cfg := DefaultConfig()
	if err := CheckSlashEvidenceHeadroom(cfg); err != nil {
		t.Fatalf("the SHIPPED defaults violate the invariant SlashesBytesCap was derived from: %v", err)
	}
}
