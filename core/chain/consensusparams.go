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
// THE SECOND CATEGORY — an identity property of the network (owner ruling, 2026-09-11). The
// membership rule used to read: every field that can change a validity verdict is BOUND TO THE
// CHAIN, or is EXPLICITLY EXCLUDED WITH A RECORDED REASON. NetworkName fits neither arm — it
// reaches NO verdict, and "it reaches no verdict" is this struct's own recorded reason for
// EXCLUDING Archive. Shipping it under the old rule would have made a machine-checked doctrine
// unfalsifiable and admitted the next non-consensus field by precedent instead of by argument.
//
// The rule is amended to THREE closed categories, not two plus an exception. A chain.Config field
// is exactly one of:
//
//	(a) BOUND          — it can change a validity verdict, so it is bound to the chain;
//	(b) NETWORK IDENTITY — it changes NO validity verdict, and it IS genesis-covered;
//	(c) EXCLUDED       — with a recorded reason.
//
// WHY (b) IS CLOSED AND NOT AN ESCAPE HATCH — the owner's binding condition, and both of its arms
// are machine-checked from opposite sides:
//
//   - "changes no validity verdict" is MEASURED, never asserted. The field must carry a
//     perturbation that actually ran, and it must diverge in ZERO regimes. A field that diverges
//     anywhere is (a), and declaring it (b) is RED.
//   - "is genesis-covered" is RESOLVED BY REFLECTION against this struct. A field that is not
//     carried is (c), and declaring it (b) is RED.
//
// So (b) admits exactly the fields that ride in the genesis hash and move no verdict. Such a
// field has precisely ONE observable effect: it partitions networks and names them. That is what
// "an identity property of the network" means, and nothing else fits through. The gate is
// TestConsensusVerdictIsNotAFunctionOfLocalConfig; the declarations are configDecls.
//
// MEMBERSHIP IS DELIBERATE IN BOTH DIRECTIONS. Five Config fields are OUT, each for a different
// reason, and each exclusion is load-bearing: Archive is retention only and reaches no verdict;
// sharing WSCheckpoint would DESTROY weak subjectivity, since it is the operator's own trust
// anchor (this sentence read "WSCheckpoint is narrowing-only and sharing it would ..." until
// 2026-09-11, when narrowing-only was MEASURED FALSE: setting the pin raises trustFloor() and
// WIDENS what the node trusts unverified. The exclusion never rested on that clause);
// MinProposerRep/MinAttesterRep cannot be usefully bound because the
// INPUT is the local reputation view, so a shared threshold still diverges; and
// LivenessRecoveryHeight is structurally unbindable — it is set AFTER launch, on a chain that by
// construction cannot commit it (R-LIVENESS-RECOVERY-UNBOUND, which is a DISCLOSURE in
// docs/design/m0.md 10.1 rather than a register row — it has no closer).
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
	// (R-CONFIG-GATE-NODE-SCOPE — now CLOSED: core/node's
	// TestNodeConsensusVerdictIsNotAFunctionOfLocalConfig closes the complement over node.Config, and
	// the two declaration tables are a checked bijection onto this struct).
	BondLabelSamples int    `cbor:"16,keyasint"`
	BondVDFDelay     uint64 `cbor:"17,keyasint"`
	// --- the network's own identity (category (b); see THE SECOND CATEGORY above) ---
	//
	// NetworkName reaches NO validity verdict. It is here so a node can report the network it
	// is serving by a name READ FROM THE CHAIN rather than from its own flags, and so that two
	// networks that differ only by name are different networks. See Config.NetworkName.
	NetworkName string `cbor:"18,keyasint"`
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
		NetworkName:             cfg.NetworkName,
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
	// NOT a flag: node.Config.BondVDFDelay is a compiled default (core/node/node.go), so a
	// divergence here means the two nodes are running different BUILDS. Naming "-bond-vdf" here
	// told the operator to change something that does not exist.
	add("bond-vdf-delay (compiled default, no flag)", p.BondVDFDelay, q.BondVDFDelay)
	// The name reaches no validity verdict, but it IS committed, so a node whose -network-name
	// disagrees with the genesis is serving a chain it would mis-report. Naming it here is what
	// makes that diagnosable instead of a bare hash mismatch.
	add("-network-name", p.NetworkName, q.NetworkName)
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

// ConsensusParams projects THIS chain's live configuration onto the committed form, so the
// genesis a node MINTS and the params that node later CHECKS are read from one place.
//
// WHY THIS EXISTS RATHER THAN A DIRECT ParamsFromConfig CALL AT THE MINT SITE. The two arms must
// project the SAME config or the mechanism inverts: the node that founded the network would refuse
// its own genesis at the next restart. CheckConsensusParams reads c.cfg; a mint site that built
// params from the Config LITERAL it passed to New would be reading a second copy, and any future
// normalisation inside New would silently separate them. Routing both arms through the chain makes
// that class of drift unrepresentable instead of merely absent today.
//
// The two node-side verifier knobs are arguments because they live in core/node.Config, which
// core/chain cannot import (cycle). See the ConsensusParams field docs.
func (c *Chain) ConsensusParams(bondLabelSamples int, bondVDFDelay uint64) ConsensusParams {
	return ParamsFromConfig(c.cfg, bondLabelSamples, bondVDFDelay)
}
