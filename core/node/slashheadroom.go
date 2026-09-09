package node

import (
	"fmt"

	"github.com/nerolabs/silt/core/chain"
)

// The SlashesBytesCap route-close (owner call 2026-09-09: "CLOSE THE ROUTE. Not a
// re-ratification. The value stays 16 MiB. The route goes.").
//
// chain.SlashesBytesCap is a CONSENSUS VALIDITY rule enforced on every validator
// (core/chain/validate_v5_predicates.go). Its documented invariant is
//
//	SlashesBytesCap >= 2 * (honest block) + overhead
//
// and "honest block" was derived from the DEFAULT VALUES of two flags — -max-bondreg-
// bytes-per-block and -max-entry-bytes-per-block — that are PROPOSER-SIDE ONLY. Every
// non-test read is core/node/chainrole.go (foldPendingBondRegs) and core/node/entrypool.go
// (foldPendingEntries); no validator checks a peer's budgets, and the shipped help
// documents "0 = unbounded". So nothing bound the running value to the derivation, and an
// operator could raise its own budget past the point where a LEGITIMATE evidence pair no
// longer fits the cap. Past that point a real double-signer's evidence is rejected by the
// cap before CheckEquivocation ever runs, and the equivocator KEEPS ITS SEAT: slashing
// defeated by making the evidence too big. Accountability is a Part-0 corner.
//
// This is the #380 class, which silt paid for once already in the same week: #380 was
// RequiredQuorum() returning the LOCAL cfg.Quorum in the mature regime, producing an I1
// divergence. Here it is SlashesBytesCap's invariant resting on local cfg budgets. THE
// RULE THE CLASS TEACHES: a consensus quantity must be a function of the CHAIN, never of
// local config. Where a consensus rule is nonetheless derived from a configurable bound,
// the configuration must be REFUSED when it violates the derivation — which is what this
// file does.
//
// Timing (owner, 2026-09-09): this is a VALIDITY rule (freeze manifest item 10), so its
// deadline is the STAMP RAISE, not the freeze. It is not deferrable past that, because
// RAISING a cap later is a WIDENING rule change and therefore outside the narrowing
// exemption that makes post-freeze rule changes cheap — it is a coordinated fleet fork.
//
// SCOPE, stated so it is not over-read: this closes the CONFIGURATION route only. The
// disclosed second face of the cap — that a >=1/3 coalition can make every evidence pair
// over-cap with its own valid renewals, so accountable safety degrades to plain safety for
// fat coalitions — is UNTOUCHED here and no admissible cap value closes it. Only the v5
// two-level block hash (d-3, fixed-size evidence) removes that face. See the second-face
// paragraph on chain.SlashesBytesCap.

// slashEvidenceHeaderSlack is the per-block allowance for everything outside the two
// packed budgets: the header, the attestation set, LastCommit, and cbor framing. It is
// deliberately generous, because being wrong in this direction is an accountability break
// rather than a resource overrun.
const slashEvidenceHeaderSlack = 64 << 10

// maxHonestBondRegBytes reports the largest -max-bondreg-bytes-per-block that still
// satisfies the invariant, at a given entry budget. It is a function of the RUNTIME entry
// budget, never of that flag's default — the whole defect being closed here is a bound
// computed from a default instead of from the value in force.
func maxHonestBondRegBytes(entryBudget int64) int64 {
	return chain.SlashesBytesCap/2 - entryBudget - slashEvidenceHeaderSlack
}

// CheckSlashEvidenceHeadroom refuses a configuration whose per-block packing budgets would
// let a legitimate equivocation proof exceed chain.SlashesBytesCap. Call it before New on
// any path that accepts operator configuration; cmd/silt refuses to start on a non-nil
// return. Driven by G-SLASHCAP-1.
func CheckSlashEvidenceHeadroom(cfg Config) error {
	regs, entries := cfg.MaxBondRegBytesPerBlock, cfg.MaxEntryBytesPerBlock

	// The proposer's guard is `budget > 0`, so zero AND negative both mean unbounded
	// (core/node/chainrole.go, core/node/entrypool.go). An unbounded honest block makes
	// the invariant unsatisfiable at any cap value, so it is refused rather than clamped:
	// clamping would silently re-interpret an operator's explicit request.
	if regs <= 0 {
		return fmt.Errorf("-max-bondreg-bytes-per-block=%d is UNBOUNDED (the proposer's guard is `budget > 0`), "+
			"which defeats the invariant SlashesBytesCap (%d bytes) is derived from: an equivocation proof carries "+
			"two FULL block bodies, so an unbounded body makes a real double-signer's evidence unprovable at any cap "+
			"and the equivocator keeps its seat. Set a positive budget at or below %d bytes",
			regs, int64(chain.SlashesBytesCap), maxHonestBondRegBytes(defaultedEntryBudget(entries)))
	}
	if entries <= 0 {
		return fmt.Errorf("-max-entry-bytes-per-block=%d is UNBOUNDED (the proposer's guard is `budget > 0`), "+
			"which defeats the invariant SlashesBytesCap (%d bytes) is derived from: the cap bounds the WHOLE "+
			"encoded body, and entries are not exempt from it. Set a positive budget",
			entries, int64(chain.SlashesBytesCap))
	}

	// The invariant itself, on the values in force.
	pair := 2 * (regs + entries + slashEvidenceHeaderSlack)
	if pair > int64(chain.SlashesBytesCap) {
		return fmt.Errorf("the configured per-block budgets defeat slash evidence: "+
			"2 x (bondregs %d + entries %d + %d overhead) = %d bytes exceeds SlashesBytesCap (%d). "+
			"An equivocation proof carries two FULL block bodies, so at these budgets a REAL double-signer's "+
			"evidence is rejected by the cap before CheckEquivocation runs and the equivocator KEEPS ITS SEAT "+
			"(accountability is a Part-0 corner). Lower -max-bondreg-bytes-per-block to at most %d bytes, "+
			"or lower -max-entry-bytes-per-block",
			regs, entries, int64(slashEvidenceHeaderSlack), pair, int64(chain.SlashesBytesCap),
			maxHonestBondRegBytes(entries))
	}
	return nil
}

// defaultedEntryBudget keeps the advisory ceiling in the unbounded-regs message useful
// when the entry budget is itself unset.
func defaultedEntryBudget(entries int64) int64 {
	if entries <= 0 {
		return 64 << 10
	}
	return entries
}
