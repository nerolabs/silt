package chain

import (
	"bytes"
	"fmt"

	"github.com/nerolabs/silt/core/publishtoken"
	"github.com/nerolabs/silt/core/statehash"
	"github.com/nerolabs/silt/ports"
)

// era-4 (v5) composition predicates — the mirrors of the node's P4, P6, P8, P8b, P9, P10/P11 and
// the regime helpers they read, written ONCE over StateView.
//
// Every function here returns (FloorBoxOutcome, error) with the SAME three-valued discipline as
// the composition: Accept means "this step passed", Reject is a positive disproof, and
// IndeterminateTrustlessly means the VIEW could not see a read — never a default, never a
// fall-through. The one shape to watch for in review is a `_, av := ...` that ignores av: that is
// how a stall silently becomes an "absent", and it is the class R-VIEW-FAITHFULNESS names as this
// round's red-team target.
//
// EVERY MIRROR NAMES THE NODE FUNCTION IT MIRRORS, and was re-derived against that body on main at
// 5239625 — never copied from the donor branch, whose P4/P5/P7/P8 had all drifted (build record
// docs/thinking/2026-09-07-floorbox-structure-round-1a-build.md §2 step 4).

// ---------------------------------------------------------------------------
// scalars and regime predicates
// ---------------------------------------------------------------------------

// v5ScalarBool decodes a committed boolean scalar leaf. The comparison is against the canonical
// encoding, so an empty slice is NOT silently false — statehash.EncodeBool(false) is a specific
// byte, and a value that matches neither encoding is a stall.
func v5ScalarBool(v StateView, tag, name string) (bool, FloorBoxOutcome, error) {
	raw, av := v.Scalar(tag)
	if av != Present {
		return false, IndeterminateTrustlessly, stall(name)
	}
	switch {
	case bytes.Equal(raw, statehash.EncodeBool(true)):
		return true, Accept, nil
	case bytes.Equal(raw, statehash.EncodeBool(false)):
		return false, Accept, nil
	}
	return false, IndeterminateTrustlessly, stall(name + " (uninterpretable scalar encoding)")
}

// v5ScalarUint64 decodes a committed uint64 scalar leaf.
func v5ScalarUint64(v StateView, tag, name string) (uint64, FloorBoxOutcome, error) {
	raw, av := v.Scalar(tag)
	if av != Present {
		return 0, IndeterminateTrustlessly, stall(name)
	}
	u, ok := decodeUint64Leaf(raw)
	if !ok {
		return 0, IndeterminateTrustlessly, stall(name + " (uninterpretable scalar encoding)")
	}
	return u, Accept, nil
}

// v5EpochsEnabled mirrors Chain.epochsEnabled: own cfg AND objective mode.
func v5EpochsEnabled(v StateView) bool {
	return v.Params().EpochBlocks > 0 && v.Objective()
}

// v5HandedOff mirrors Chain.handedOff — the young→mature handoff, read from the COMMITTED latch
// scalars rather than a live field. With epochs enabled it is matureEpoch; without, everMature.
func v5HandedOff(v StateView) (bool, FloorBoxOutcome, error) {
	if v5EpochsEnabled(v) {
		return v5ScalarBool(v, tagMatureEpoch, "matureEpoch")
	}
	return v5ScalarBool(v, tagEverMature, "everMature")
}

// v5MatureEpochRegime reports (epochsEnabled && matureEpoch) — the frozen-set regime selector both
// qualification predicates, RequiredQuorum, validatorSetSize and the weight quorum branch on.
func v5MatureEpochRegime(v StateView) (bool, FloorBoxOutcome, error) {
	if !v5EpochsEnabled(v) {
		return false, Accept, nil
	}
	me, out, err := v5ScalarBool(v, tagMatureEpoch, "matureEpoch")
	if out != Accept {
		return false, out, err
	}
	return me, Accept, nil
}

// v5LaunchAnchorRule mirrors Chain.launchAnchorGiven — the ONE definition of the launch-anchor
// rule with the handoff predicate SUPPLIED. It reads own-cfg only (Anchors).
func v5LaunchAnchorRule(anchors map[ports.NodeID]bool, id ports.NodeID, handedOff bool) bool {
	return len(anchors) > 0 && anchors[id] && !handedOff
}

// v5EffectiveEpochSet is the #535 substitution rule, and it lives HERE — in the composition —
// rather than behind a view method taking h. Its inputs are own cfg plus two whole-set reads, so
// a view method would permit a SECOND implementation of the substitution: the #402 trap, and the
// exact seam cert gate G-A was filed on.
//
// It mirrors Chain.effectiveEpochSet exactly: the frozen snapshot everywhere except the one
// operator-directed recovery boundary, where the governing set is the LIVE qualified set. The
// live arm reads Qualified() — the era-4 committed accelerator — rather than re-filtering
// Bonded(), because filter(bonded, slashed, MinBond) == qualified IS the era-4 maintenance claim
// and re-deriving it here would be the second copy.
func v5EffectiveEpochSet(v StateView, h uint64) (map[ports.NodeID]int64, FloorBoxOutcome, error) {
	p := v.Params()
	if p.LivenessRecoveryHeight != 0 && h == p.LivenessRecoveryHeight &&
		v5EpochsEnabled(v) && h%p.EpochBlocks == 0 {
		q, av := v.Qualified()
		if av != Present {
			return nil, IndeterminateTrustlessly, stall("qualified (whole set, #535 recovery re-base)")
		}
		return q, Accept, nil
	}
	set, av := v.EpochSet()
	if av != Present {
		return nil, IndeterminateTrustlessly, stall("epochSet (whole set)")
	}
	return set, Accept, nil
}

// ---------------------------------------------------------------------------
// qualification (P4, and the attester filter inside collectQuorumSigs / ValidateEntry)
// ---------------------------------------------------------------------------

// v5AttesterQualifiedAt mirrors Chain.attesterQualifiedAt, BOTH branches: slashed first (the ONE
// live mid-epoch disqualification), then objective (frozen set in a mature epoch, else
// bonded ≥ MinBond or launch anchor), else the LEGACY rep ≥ MinAttesterRep (M-1).
func v5AttesterQualifiedAt(v StateView, id ports.NodeID, h uint64) (bool, FloorBoxOutcome, error) {
	slashed, av := v.Slashed(id)
	if av == NoWitness {
		return false, IndeterminateTrustlessly, stall("slashed[" + id.String() + "]")
	}
	if slashed {
		return false, Accept, nil
	}
	if !v.Objective() {
		rep, av := v.Rep(id)
		if av != Present {
			return false, IndeterminateTrustlessly, stall("rep[" + id.String() + "] (legacy mode)")
		}
		return rep >= v.Params().MinAttesterRep, Accept, nil
	}
	mature, out, err := v5MatureEpochRegime(v)
	if out != Accept {
		return false, out, err
	}
	if mature {
		set, out, err := v5EffectiveEpochSet(v, h)
		if out != Accept {
			return false, out, err
		}
		_, ok := set[id]
		return ok, Accept, nil
	}
	sz, av := v.BondedOf(id)
	if av == NoWitness {
		return false, IndeterminateTrustlessly, stall("bonded[" + id.String() + "]")
	}
	if sz >= v.Params().MinBond {
		return true, Accept, nil
	}
	ho, out, err := v5HandedOff(v)
	if out != Accept {
		return false, out, err
	}
	return v5LaunchAnchorRule(v.Params().Anchors, id, ho), Accept, nil
}

// v5RequireProposerQualified is P4 — Chain.proposerQualifiedAt — with the #572 ATTRIBUTION
// BRANCHES (P4a) preserved verbatim. Those branches are an observable contract: "bonded 1048576,
// needs 1048576" — equal-but-failing — sent a live debug down a false trail for a week because
// the rendering did not name the ACTUAL disqualifying branch. The node evaluates the predicate
// first and attributes second, and the attribution is OBJECTIVE-ONLY: a legacy refusal (including
// a slashed proposer in legacy mode) renders the rep message. Mirrored in that order.
func v5RequireProposerQualified(v StateView, b *Block) (FloorBoxOutcome, error) {
	id := b.ProposerID()
	p := v.Params()

	slashed, av := v.Slashed(id)
	if av == NoWitness {
		return IndeterminateTrustlessly, stall("slashed[proposer]")
	}
	bondedOf := func() (int64, FloorBoxOutcome, error) {
		sz, av := v.BondedOf(id)
		if av == NoWitness {
			return 0, IndeterminateTrustlessly, stall("bonded[proposer]")
		}
		return sz, Accept, nil
	}

	// ---- LEGACY (M-1): !slashed && rep >= MinProposerRep, rendered the node's way. ----
	if !v.Objective() {
		rep, av := v.Rep(id)
		if av != Present {
			return IndeterminateTrustlessly, stall("rep[proposer] (legacy mode)")
		}
		if !slashed && rep >= p.MinProposerRep {
			return Accept, nil
		}
		return Reject, fmt.Errorf("%w: proposer %s has %d, needs %d", ErrLowReputation, id, rep, p.MinProposerRep)
	}

	// ---- OBJECTIVE: the predicate, then the attribution branch that fired. ----
	if slashed {
		return Reject, fmt.Errorf("%w: proposer %s is slashed (F2 eviction)", ErrLowReputation, id)
	}
	mature, out, err := v5MatureEpochRegime(v)
	if out != Accept {
		return out, err
	}
	if mature {
		set, out, err := v5EffectiveEpochSet(v, b.Height)
		if out != Accept {
			return out, err
		}
		if _, ok := set[id]; ok {
			return Accept, nil
		}
		sz, out, err := bondedOf()
		if out != Accept {
			return out, err
		}
		return Reject, fmt.Errorf("%w: proposer %s not in the frozen epoch set governing height %d (mature epoch; bonded %d)",
			ErrLowReputation, id, b.Height, sz)
	}
	ho, out, err := v5HandedOff(v)
	if out != Accept {
		return out, err
	}
	// LAUNCH WINDOW — ANCHOR-ONLY PROPOSING (#402 encoding B).
	if len(p.Anchors) > 0 && !ho {
		if v5LaunchAnchorRule(p.Anchors, id, ho) {
			return Accept, nil
		}
		sz, out, err := bondedOf()
		if out != Accept {
			return out, err
		}
		return Reject, fmt.Errorf("%w: proposer %s is not a launch anchor (young network proposes anchor-only, #402; bonded %d)",
			ErrLowReputation, id, sz)
	}
	sz, out, err := bondedOf()
	if out != Accept {
		return out, err
	}
	if sz >= p.MinBond || v5LaunchAnchorRule(p.Anchors, id, ho) {
		return Accept, nil
	}
	return Reject, fmt.Errorf("%w: proposer %s bonded %d, needs %d", ErrLowReputation, id, sz, p.MinBond)
}

// ---------------------------------------------------------------------------
// P6 takedowns, P8 slashes, P8b issuer keys, P9 entries, P10/P11 era versions
// ---------------------------------------------------------------------------

// v5ValidateTakedowns mirrors Chain.validateTakedowns.
func v5ValidateTakedowns(v StateView, b *Block) (FloorBoxOutcome, error) {
	for _, r := range b.Revocations {
		committed, av := v.ByRoot(r)
		if av == NoWitness {
			return IndeterminateTrustlessly, stall("byRoot[" + r.String() + "]")
		}
		if !committed {
			return Reject, fmt.Errorf("%w: %s", ErrRevokeUnknownRoot, r)
		}
	}
	for _, r := range b.Unrevocations {
		revoked, av := v.Revoked(r)
		if av == NoWitness {
			return IndeterminateTrustlessly, stall("revoked[" + r.String() + "]")
		}
		if !revoked {
			return Reject, fmt.Errorf("%w: %s", ErrUnrevokeNotRevoked, r)
		}
	}
	return Accept, nil
}

// v5ValidateSlashes mirrors Chain.validateSlashes (block-local, no committed read): the
// SlashesBytesCap ceiling FIRST, before any signature work, then CheckEquivocation per proof with
// both hashes recomputed from full bodies (R0.6).
func v5ValidateSlashes(b *Block) (FloorBoxOutcome, error) {
	if len(b.Slashes) == 0 {
		return Accept, nil
	}
	if n := SlashesEncodedSize(b.Slashes); n > SlashesBytesCap {
		return Reject, fmt.Errorf("%w: %d bytes (cap %d)", ErrSlashesBytesCapExceeded, n, SlashesBytesCap)
	}
	for i := range b.Slashes {
		if err := CheckEquivocation(&b.Slashes[i]); err != nil {
			return Reject, fmt.Errorf("%w: proof %d: %w", ErrBadSlash, i, err)
		}
	}
	return Accept, nil
}

// v5ValidateIssuerKeys is P8b — Chain.validateIssuerKeys (issuerkey.go), the stage the 2026-09-03
// table never enumerated (M-5). Every clause is a REJECT, never a silent drop. Reads own cfg
// (EpochBlocks via blockEpoch, MinBond) and the committed bonded leaf of each issuer.
func v5ValidateIssuerKeys(v StateView, b *Block) (FloorBoxOutcome, error) {
	if len(b.IssuerKeys) == 0 {
		return Accept, nil
	}
	if b.Version < BlockVersionWitnessable {
		return Reject, fmt.Errorf("%w: block version %d", ErrIssuerKeyEra, b.Version)
	}
	p := v.Params()
	cur := v5BlockEpoch(p, b.Height)
	for i := range b.IssuerKeys {
		r := b.IssuerKeys[i]
		if !VerifyIssuerKeyReg(r) {
			return Reject, fmt.Errorf("%w (index %d)", ErrIssuerKeySig, i)
		}
		if r.Epoch < cur || r.Epoch > cur+issuerKeyPrePublish {
			return Reject, fmt.Errorf("%w: epoch %d, block epoch %d, pre-publish window %d",
				ErrIssuerKeyEpoch, r.Epoch, cur, issuerKeyPrePublish)
		}
		if p.MinBond > 0 {
			sz, av := v.BondedOf(r.IssuerID())
			if av == NoWitness {
				return IndeterminateTrustlessly, stall("bonded[" + r.IssuerID().String() + "] (issuer key)")
			}
			if sz <= 0 {
				return Reject, fmt.Errorf("%w: %x", ErrIssuerKeyUnbonded, r.IssuerID())
			}
		}
	}
	return Accept, nil
}

// v5BlockEpoch mirrors Chain.blockEpoch: h / EpochBlocks, epoch 0 with epochs disabled.
func v5BlockEpoch(p Params, h uint64) uint64 {
	if p.EpochBlocks == 0 {
		return 0
	}
	return h / p.EpochBlocks
}

// v5ValidateEntries is P9 — the entry loop of ValidateProposal (intra-block dup root / dup serial)
// around the per-entry rule.
func v5ValidateEntries(v StateView, b *Block) (FloorBoxOutcome, error) {
	p := v.Params()
	seen := make(map[ports.Hash]bool, len(b.Entries))
	seenSerial := make(map[string]bool, len(b.Entries))
	for _, e := range b.Entries {
		if seen[e.Root] {
			return Reject, fmt.Errorf("%w: %s", ErrDupRoot, e.Root)
		}
		if e.Token != nil && seenSerial[string(e.Token.Serial)] {
			return Reject, fmt.Errorf("%w: %x", ErrTokenSpent, e.Token.Serial)
		}
		if out, err := v5ValidateEntry(v, p, e); out != Accept {
			return out, err
		}
		if e.Token != nil {
			seenSerial[string(e.Token.Serial)] = true
		}
		seen[e.Root] = true
	}
	return Accept, nil
}

// v5ValidateEntry mirrors Chain.ValidateEntry.
func v5ValidateEntry(v StateView, p Params, e ports.Entry) (FloorBoxOutcome, error) {
	exists, av := v.ByRoot(e.Root)
	if av == NoWitness {
		return IndeterminateTrustlessly, stall("byRoot[" + e.Root.String() + "]")
	}
	if exists {
		return Reject, fmt.Errorf("%w: %s", ErrDupRoot, e.Root)
	}
	if len(e.ManifestChunks) == 0 {
		return Reject, fmt.Errorf("chain: entry %s has no manifest pointers", e.Root)
	}
	if !p.AllowPublisher && e.Publisher != (ports.NodeID{}) {
		return Reject, fmt.Errorf("%w: entry %s", ErrPublisherEntry, e.Root)
	}
	if p.TokenQuorum > 0 {
		if e.Token == nil {
			return Reject, fmt.Errorf("%w: entry %s", ErrTokenRequired, e.Root)
		}
		// The cheap replay reject BEFORE the RSA work (#183 red-team F-1) — its PLACEMENT is the
		// rule: a harvested valid token paired with a novel Root would otherwise run all N modexps
		// before a spent-check caught the replay.
		spent, av := v.Spent(e.Token.Serial)
		if av == NoWitness {
			return IndeterminateTrustlessly, stall(fmt.Sprintf("spent[%x]", e.Token.Serial))
		}
		if spent {
			return Reject, fmt.Errorf("%w: %x", ErrTokenSpent, e.Token.Serial)
		}
		// The issuer-quorum membership probe is HEIGHT-LESS (height 0 is genesis, never a
		// recovery boundary) — the same frozen rule Chain.attesterQualified uses. A stall inside
		// this callback cannot be returned through publishtoken.Verify's bool, so it is captured.
		var stallErr error
		qualified := func(id ports.NodeID) bool {
			ok, out, err := v5AttesterQualifiedAt(v, id, 0)
			if out != Accept {
				if stallErr == nil {
					stallErr = err
				}
				return false
			}
			return ok
		}
		err := publishtoken.Verify(*e.Token, p.TokenQuorum, p.IssuerKey, qualified)
		if stallErr != nil {
			return IndeterminateTrustlessly, stallErr // a stall NEVER renders as a token failure
		}
		if err != nil {
			return Reject, fmt.Errorf("chain: entry %s: %w", e.Root, err)
		}
	}
	return Accept, nil
}

// v5ValidateEraVersions is P10/P11 — Chain.validateEra3Version then Chain.validateEra4Version.
// Both activation predicates read the committed lock-in scalars, so they are witnessable; the
// order (versions before roots) is preserved so a failure names the version, not a missing root.
func v5ValidateEraVersions(v StateView, b *Block) (FloorBoxOutcome, error) {
	active, out, err := v5EraActive(v, b.Height, tagEra3LockedIn, tagEra3Height, v.Params().Era3ActivationHeight, "era3")
	if out != Accept {
		return out, err
	}
	if active && b.Version < BlockVersionStateRoot {
		return Reject, fmt.Errorf("%w: height %d version %d", ErrEra3VersionRequired, b.Height, b.Version)
	}
	active, out, err = v5EraActive(v, b.Height, tagEra4LockedIn, tagEra4Height, v.Params().Era4ActivationHeight, "era4")
	if out != Accept {
		return out, err
	}
	if active && b.Version < BlockVersionWitnessable {
		return Reject, fmt.Errorf("%w: height %d version %d", ErrEra4VersionRequired, b.Height, b.Version)
	}
	return Accept, nil
}

// v5EraActive mirrors Chain.era3Active / Chain.era4Active: a genesis config override, else the
// chain-derived lock-in scalars. At-or-greater at the derived height, matching the node.
func v5EraActive(v StateView, h uint64, lockedTag, heightTag string, cfgHeight uint64, name string) (bool, FloorBoxOutcome, error) {
	if cfgHeight > 0 {
		return h >= cfgHeight, Accept, nil
	}
	locked, out, err := v5ScalarBool(v, lockedTag, name+"LockedIn")
	if out != Accept {
		return false, out, err
	}
	if !locked {
		return false, Accept, nil
	}
	at, out, err := v5ScalarUint64(v, heightTag, name+"Height")
	if out != Accept {
		return false, out, err
	}
	return h >= at, Accept, nil
}
