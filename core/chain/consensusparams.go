package chain

import (
	"fmt"
	"sort"

	"github.com/nerolabs/silt/ports"
)

// ConsensusParams is the CONSENSUS-CRITICAL GENESIS CONFIG, committed into the genesis block so
// that the genesis hash covers it.
//
// WHY IT EXISTS — canon rule 8 (docs/build-process.md). A consensus quantity must be a function of
// the CHAIN. silt named this class in prose for a long time ("consensus-critical genesis config",
// Config's own field docs) with NO enumeration and NO enforcement, so membership was a human
// remembering to write the sentence — and three instances slipped through: #380's Config.Quorum on
// the objective path, SlashesBytesCap's invariant derived from proposer-side flag defaults, and
// MinBond, a validity threshold that was a bare command-line flag.
//
// WHY A REFUSE-TO-START WAS REFUTED FOR THIS CLASS. Rule 8's first arm binds a LOCALLY CHECKABLE
// invariant with a start-up check. MinBond divergence is NOT locally observable: no node can tell
// from its own config that a peer set a different value, so a start-up assertion has nothing to
// assert against and would ship a gate that looks green and enforces nothing. Rule 8's second arm
// applies instead — bind it to committed state.
//
// WHY VALUES AND NOT A DIGEST. Values reuse Block.Hash()'s canonical CBOR, so no new injectivity
// proof is owed, and a mismatch is DIAGNOSABLE — an operator can be told which field differs
// instead of that two hashes differ. A gossiped digest was refuted separately: it is an
// unauthenticated claim, where a genesis hash is self-authenticating.
//
// THE FAILURE SURFACE. Because the genesis hash covers these values, a node configured differently
// computes a DIFFERENT genesis hash and cannot join at all — Reconcile refuses the fork with
// ErrForeignGenesis before any validity question arises. Divergence becomes impossible to join
// with rather than fatal at validation, which is the right surface.
//
// THE TWO ARMS COMPOSE (T-REFERENT). Committing the values MANUFACTURES the referent a local
// assertion previously lacked, which makes a refuse-to-start REQUIRED rather than redundant: it
// catches the one case joining cannot — an operator editing a flag and restarting on a chain it
// has ALREADY joined. CheckConsensusParams is that arm.
//
// MEMBERSHIP IS DELIBERATE IN BOTH DIRECTIONS. Five Config fields are OUT, each for a different
// reason, and each exclusion is load-bearing: Archive is retention only and reaches no verdict;
// WSCheckpoint is narrowing-only and sharing it would DESTROY weak subjectivity, since it is the
// operator's own trust anchor; MinProposerRep/MinAttesterRep cannot be usefully bound because the
// INPUT is the local reputation view, so a shared threshold still diverges; and
// LivenessRecoveryHeight is structurally unbindable — it is set AFTER launch, on a chain that by
// construction cannot commit it (R-LIVENESS-RECOVERY-UNBOUND).
//
// NO `omitempty` ON ANY FIELD. A zero value here is a MEANING (Quorum 0, MinBond 0 = legacy mode),
// not an absence, and omitting it would make two different configurations encode identically. The
// pointer is on Block.Params instead, which is what keeps a paramless genesis byte-identical to
// one written before this field existed.
//
// Certification: GENESIS-CONFIG-FAMILY-BIND-RESEARCH-CERTIFICATION-2026-09-10.
type ConsensusParams struct {
	// --- the quorum rule ---
	Quorum          int  `cbor:"1,keyasint"`
	ByzantineQuorum bool `cbor:"2,keyasint"`
	// Anchors is a SORTED slice, not the live map: CBOR map ordering is not a wire guarantee we
	// want a consensus hash to depend on, and a sorted slice is canonical by construction.
	Anchors          []ports.NodeID `cbor:"3,keyasint"`
	AnchorQuorum     int            `cbor:"4,keyasint"`
	MatureValidators int            `cbor:"5,keyasint"`
	OperatorMargin   int            `cbor:"6,keyasint"`
	// --- bond standing ---
	MinBond           int64  `cbor:"7,keyasint"`
	MinBondBytes      int64  `cbor:"8,keyasint"`
	BondTTLBlocks     uint64 `cbor:"9,keyasint"`
	BondRegHeadWindow int    `cbor:"10,keyasint"`
	// --- epochs and era activation ---
	EpochBlocks             uint64 `cbor:"11,keyasint"`
	RegGateActivationHeight uint64 `cbor:"12,keyasint"`
	Era3ActivationHeight    uint64 `cbor:"13,keyasint"`
	Era4ActivationHeight    uint64 `cbor:"14,keyasint"`
	// --- entry admission ---
	AllowPublisher bool `cbor:"15,keyasint"`
	// --- the node-side verifier parameters (core/node.Config, carried here by VALUE to avoid an
	// import cycle). These are the sharpest members of the family and the reason its membership had
	// to be re-derived: core/bond's verifier compares a proof's label count against the verifier's
	// OWN local k, so a k=32 node rejects EVERY bond registration a k=64 swarm accepts. The flag
	// help states the coordination requirement and in the same breath invites the change. They live
	// outside chain.Config, which is why the divergence gate's reflection never saw them
	// (R-CONFIG-GATE-NODE-SCOPE).
	BondLabelSamples int    `cbor:"16,keyasint"`
	BondVDFDelay     uint64 `cbor:"17,keyasint"`
}

// SortedAnchors renders an anchor set as the canonical sorted slice this struct commits.
func SortedAnchors(m map[ports.NodeID]bool) []ports.NodeID {
	if len(m) == 0 {
		return nil
	}
	out := make([]ports.NodeID, 0, len(m))
	for id, on := range m {
		if on {
			out = append(out, id)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		for b := range out[i] {
			if out[i][b] != out[j][b] {
				return out[i][b] < out[j][b]
			}
		}
		return false
	})
	return out
}

// ParamsFromConfig projects a live Config plus the two node-side verifier knobs onto the committed
// form. It is the ONE place the membership is written, so adding a Config field forces a decision
// here rather than a silent omission — the gate TestConsensusParamsMembershipIsComplete reflects
// over Config and fails on any field that is neither carried nor explicitly excluded.
func ParamsFromConfig(cfg Config, bondLabelSamples int, bondVDFDelay uint64) ConsensusParams {
	return ConsensusParams{
		Quorum:                  cfg.Quorum,
		ByzantineQuorum:         cfg.ByzantineQuorum,
		Anchors:                 SortedAnchors(cfg.Anchors),
		AnchorQuorum:            cfg.AnchorQuorum,
		MatureValidators:        cfg.MatureValidators,
		OperatorMargin:          cfg.OperatorMargin,
		MinBond:                 cfg.MinBond,
		MinBondBytes:            cfg.MinBondBytes,
		BondTTLBlocks:           cfg.BondTTLBlocks,
		BondRegHeadWindow:       cfg.BondRegHeadWindow,
		EpochBlocks:             cfg.EpochBlocks,
		RegGateActivationHeight: cfg.RegGateActivationHeight,
		Era3ActivationHeight:    cfg.Era3ActivationHeight,
		Era4ActivationHeight:    cfg.Era4ActivationHeight,
		AllowPublisher:          cfg.AllowPublisher,
		BondLabelSamples:        bondLabelSamples,
		BondVDFDelay:            bondVDFDelay,
	}
}

// Diff reports the fields on which two parameter sets differ, as human-readable lines. It is what
// makes the refusal DIAGNOSABLE — the reason this commits values rather than a digest.
func (p ConsensusParams) Diff(q ConsensusParams) []string {
	var out []string
	add := func(name string, a, b any) {
		if fmt.Sprint(a) != fmt.Sprint(b) {
			out = append(out, fmt.Sprintf("%s: this node has %v, the chain's genesis commits %v", name, a, b))
		}
	}
	add("-quorum", p.Quorum, q.Quorum)
	add("byzantine-quorum", p.ByzantineQuorum, q.ByzantineQuorum)
	add("-anchors", p.Anchors, q.Anchors)
	add("-anchor-quorum", p.AnchorQuorum, q.AnchorQuorum)
	add("-mature-validators", p.MatureValidators, q.MatureValidators)
	add("operator-margin", p.OperatorMargin, q.OperatorMargin)
	add("-min-bond", p.MinBond, q.MinBond)
	add("-min-bond-floor", p.MinBondBytes, q.MinBondBytes)
	add("bond-ttl-blocks", p.BondTTLBlocks, q.BondTTLBlocks)
	add("bondreg-head-window", p.BondRegHeadWindow, q.BondRegHeadWindow)
	add("-epoch-blocks", p.EpochBlocks, q.EpochBlocks)
	add("reg-gate-activation-height", p.RegGateActivationHeight, q.RegGateActivationHeight)
	add("era3-activation-height", p.Era3ActivationHeight, q.Era3ActivationHeight)
	add("era4-activation-height", p.Era4ActivationHeight, q.Era4ActivationHeight)
	add("-allow-publisher", p.AllowPublisher, q.AllowPublisher)
	add("-bond-label-k", p.BondLabelSamples, q.BondLabelSamples)
	add("-bond-vdf", p.BondVDFDelay, q.BondVDFDelay)
	return out
}

// ErrParamsNotOnGenesis is a non-genesis block carrying committed consensus params.
var ErrParamsNotOnGenesis = fmt.Errorf("chain: only the genesis block may carry committed consensus params")

// validateParamsPlacement is the placement rule: ONLY height 0 may carry Params.
//
// It matters because the whole mechanism rests on the params being covered by the GENESIS hash
// specifically. A later block carrying them would be hash-covered too, but by a hash no joining
// node compares — Reconcile's foreign-genesis check reads blocks[0] alone. Params anywhere else
// would look committed while binding nothing, which is the decoration shape.
//
// NARROWING: it can only refuse. A pre-bind genesis carries nil and is unaffected.
func validateParamsPlacement(b *Block) error {
	if b.Height != 0 && b.Params != nil {
		return fmt.Errorf("%w: height %d", ErrParamsNotOnGenesis, b.Height)
	}
	return nil
}

// ErrParamsDiverge is a local configuration that contradicts the chain's committed params.
var ErrParamsDiverge = fmt.Errorf("chain: local consensus config contradicts the genesis this chain commits")

// CheckConsensusParams is rule 8's FIRST arm, which only exists because the second arm manufactured
// its referent (T-REFERENT). Joining is already guarded — a divergent node computes a different
// genesis hash and Reconcile refuses the fork. What joining CANNOT catch is an operator who edits a
// flag and restarts on a chain the node has ALREADY joined: the genesis on disk is unchanged, so
// there is no mismatch to detect at the fork boundary, and the node would simply start applying
// different rules to the same history. This is that check.
//
// Returns nil when the chain's genesis predates the bind (Params == nil) — the surviving paramless
// path, disclosed rather than silently tolerated.
func (c *Chain) CheckConsensusParams(bondLabelSamples int, bondVDFDelay uint64) error {
	if len(c.blocks) == 0 || c.blocks[0].Params == nil {
		return nil
	}
	committed := *c.blocks[0].Params
	local := ParamsFromConfig(c.cfg, bondLabelSamples, bondVDFDelay)
	diff := local.Diff(committed)
	if len(diff) == 0 {
		return nil
	}
	msg := "\n  " + diff[0]
	for _, d := range diff[1:] {
		msg += "\n  " + d
	}
	return fmt.Errorf("%w — %d field(s) differ:%s\n\nThese are CONSENSUS-CRITICAL: every validator in a swarm must run the same values, "+
		"or two honest nodes reach different verdicts on the same block (I1). The genesis commits them, so this node "+
		"would apply different rules to a history it has already joined. Restore the committed values, or start a "+
		"different network", ErrParamsDiverge, len(diff), msg)
}
