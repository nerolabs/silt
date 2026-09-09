package node

import (
	"fmt"

	"github.com/nerolabs/silt/core/chain"
)

// The SlashesBytesCap CONFIG route-close (owner call 2026-09-09: "CLOSE THE ROUTE. Not a
// re-ratification. The value stays 16 MiB. The route goes.").
//
// WHAT THIS ENFORCES, STATED EXACTLY. A NECESSARY condition on the CONFIGURABLE terms of
// the honest-block size — nothing more. It is NOT sufficient, and the derivation is NOT
// "bound by construction": see THE FIXED POINT below, which no value of this predicate can
// repair.
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

// THE FIXED POINT — why a start-up check cannot close the accountability face (measured by
// the blind PE, 2026-09-10, on signature-valid fixtures at the SHIPPED defaults).
//
// chain.Equivocation carries two FULL chain.Blocks (equivocation.go:26-27), and a Block
// carries its own Slashes field (chain.go:518), bounded only by the cap being defended. So
//
//	cap >= 2*(body) + overhead   with   body includes Slashes <= cap
//
// has NO positive solution. It is a fixed point, not a tuning error. Concretely, with no
// coalition and no misconfiguration: a block committing two ordinary 4.14 MiB proofs is
// VALID (8.28 MiB of Slashes against a 16 MiB cap), and a LEGITIMATE equivocation proof
// about that block measures 17,373,935 B — 596 KB over cap. The equivocator keeps its seat.
//
// So the cap has a THIRD face beside the two on chain.SlashesBytesCap: the NESTED-EVIDENCE
// face, reachable at shipped defaults with no adversary, where the first two need a >=1/3
// coalition or a misconfiguration. Like them it routes to the v5 two-level block hash
// (d-3, fixed-size evidence), which is the only close any of the three has; the question of
// whether a body bound could close it instead is RESEARCH-GATED and open, because a rule
// bounding the encoded body binds PEERS, which no start-up check can.
//
// WHAT THIS FILE THEREFORE BUYS, honestly: it closes the OPERATOR MISCONFIGURATION route —
// a validator can no longer make its own equivocation unprovable by editing a local flag —
// and it makes the configurable half of the derivation true instead of assumed. It does not
// make a legitimate proof always admissible, and nothing here should be read as claiming so.

// slashEvidenceHeaderSlack is the per-block allowance for everything outside the two packed
// budgets: the header, the attestation set, LastCommit, and cbor framing.
//
// IT STANDS IN FOR A QUANTITY THAT SCALES WITH THE VALIDATOR SET, and that is a disclosed
// limit of this predicate rather than a hidden one. Era-2 evidence REQUIRES the culprit's
// signature inside PrepareQC/Atts and v5 adds a hash-folded LastCommit, which the blind PE
// measured at 639 B per validator per evidence pair. At the previous 64 KiB the gate blessed
// a configuration whose legitimate proof went over cap at N >= 205. 1 MiB covers N ~ 1600 on
// that measurement and still admits ~6.9 MiB of registrations against a 2 MiB default, so it
// costs no realistic operator anything. Above that N the predicate is again necessary but
// not sufficient — as it already is for the nested-evidence face above.
const slashEvidenceHeaderSlack = 1 << 20

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
		remedy := "lower -max-entry-bytes-per-block (the entry budget alone exhausts the headroom, so no " +
			"positive -max-bondreg-bytes-per-block can satisfy the invariant)"
		if ceiling, ok := advisoryRegCeiling(entries); ok {
			remedy = fmt.Sprintf("lower -max-bondreg-bytes-per-block to at most %d bytes, or lower -max-entry-bytes-per-block", ceiling)
		}
		return fmt.Errorf("the configured per-block budgets defeat slash evidence: "+
			"2 x (bondregs %d + entries %d + %d overhead) = %d bytes exceeds SlashesBytesCap (%d). "+
			"An equivocation proof carries two FULL block bodies, so at these budgets a REAL double-signer's "+
			"evidence is rejected by the cap before CheckEquivocation runs and the equivocator KEEPS ITS SEAT "+
			"(accountability is a Part-0 corner). To fix: %s",
			regs, entries, int64(slashEvidenceHeaderSlack), pair, int64(chain.SlashesBytesCap), remedy)
	}
	return nil
}

// advisoryRegCeiling is the "lower -max-bondreg-bytes-per-block to at most N" figure. When the
// entry budget alone already exhausts the headroom the reg ceiling is non-positive, and telling
// an operator to lower a budget to a negative number is not an actionable instruction — so it
// clamps at zero and the caller points at the entry budget instead (PE ruling S-6).
func advisoryRegCeiling(entryBudget int64) (ceiling int64, regsCanFix bool) {
	c := maxHonestBondRegBytes(entryBudget)
	if c <= 0 {
		return 0, false
	}
	return c, true
}

// defaultedEntryBudget keeps the advisory ceiling in the unbounded-regs message useful
// when the entry budget is itself unset.
func defaultedEntryBudget(entries int64) int64 {
	if entries <= 0 {
		return 64 << 10
	}
	return entries
}
