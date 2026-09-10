package chain

import (
	"bytes"
	"crypto/ed25519"
	"fmt"
	"sort"

	"github.com/nerolabs/silt/ports"
)

// era-4 (v5) composition — P7 (bond registrations) and C1..C5 (the two quorum stacks), the
// mirrors of the node's validateBondRegs / requireProposerPrepare / collectQuorumSigs /
// requireQuorumStack and the helpers they read. Same three-valued discipline as
// validate_v5_predicates.go; every mirror names the node function and was re-derived against it.

// ---------------------------------------------------------------------------
// P7 bond registrations
// ---------------------------------------------------------------------------

// v5ValidateBondRegs mirrors Chain.validateBondRegs: the legacy early return (a legacy chain
// ignores BondRegs entirely — M-1), the Q2 pruned-tolerance gate keyed on the reader's OWN trust
// floor, the v5 RegCap, the #506 R-rule past the gate, the unconditional per-root dedup, and the
// nonce-window per-registration check. The pruned leg is the NODE's rule (v.PrunedTolerated); a box
// stalls on a pruned block at its entry before the composition runs, and stalls here too if it ever
// reached this leg — the view answers NoWitness, never a floor (H-4).
func v5ValidateBondRegs(v StateView, b *Block) (FloorBoxOutcome, error) {
	if !v.Objective() {
		return Accept, nil
	}
	p := v.Params()
	// BOND POSSESSION, not identity. This refusal exists because a block whose space-time proofs
	// are gone cannot have them re-verified — it is nothing to do with whether the block can
	// recompute its own hash. (d-3) retires `Pruned` for v5, so IsPruned() no longer detects a v5
	// block that shed its proofs; HeavyProofsShed() does. Using IsPruned() here after (d-3) would
	// be the disqualifying widening the delta cert named: "identity != bond possession, trustFloor
	// stays". Gate: the P7 pruned arm of the v4/v5 parity oracle.
	if b.HeavyProofsShed() {
		tolerated, av := v.PrunedTolerated(b.Height)
		if av != Present {
			// No view of the pruned-tolerance rule: STALL. A view that cannot answer must not have
			// the answer invented for it — that is the floor-shaped hole H-4 closed.
			return IndeterminateTrustlessly, fmt.Errorf("%w: pruned block at height %d, no view of the pruned-tolerance rule", ErrViewNoWitness, b.Height)
		}
		if !tolerated {
			return Reject, fmt.Errorf("%w: pruned block at height %d", ErrPrunedAboveHorizon, b.Height)
		}
		for _, r := range b.BondRegs {
			if r.Answer != nil {
				return Reject, fmt.Errorf("%w: validator %s", ErrMalformedPruned, r.ValidatorID())
			}
		}
		return Accept, nil
	}
	// era-4 RegCap validity rule (4c). v5-gated on the node; the composition is v5-only (BG-1),
	// and the version gate is kept verbatim so the mirror reads as the node does.
	if b.Version >= BlockVersionWitnessable {
		if n := len(canonicalBondRegs(b.BondRegs)); n > RegCap {
			return Reject, fmt.Errorf("%w: %d BondRegs (cap %d)", ErrRegCapExceeded, n, RegCap)
		}
	}
	nonces, out, err := v5RecentBondRegNonces(v)
	if out != Accept {
		return out, err
	}
	gate, out, err := v5RegGateActive(v, b.Height)
	if out != Accept {
		return out, err
	}
	var seenReg map[ports.NodeID]bool
	if gate {
		seenReg = make(map[ports.NodeID]bool, len(b.BondRegs))
	}
	// PER-ROOT DEDUP is UNCONDITIONAL (not gate-gated): apply() resolves a same-root collision by
	// intra-block slice order, so two honest replicas applying the identical block in a different
	// BondReg order would commit different state. It dedups on (root x DISTINCT id) only.
	seenRoot := make(map[ports.Hash]ports.NodeID, len(b.BondRegs))
	for _, r := range b.BondRegs {
		id := r.ValidatorID()
		if prev, ok := seenRoot[r.Root]; ok && prev != id {
			return Reject, fmt.Errorf("%w: root claimed by both %s and %s in one block", ErrSharedRootInBlock, prev, id)
		}
		seenRoot[r.Root] = id
		if gate {
			slashed, av := v.Slashed(id)
			if av == NoWitness {
				return IndeterminateTrustlessly, stall("slashed[" + id.String() + "] (reg gate)")
			}
			if slashed {
				return Reject, fmt.Errorf("%w: validator %s is slashed", ErrRegGate, id)
			}
			if seenReg[id] {
				return Reject, fmt.Errorf("%w: validator %s registered twice in one block", ErrRegGate, id)
			}
			seenReg[id] = true
			regH, av := v.BondRegHeight(id)
			if av == NoWitness {
				return IndeterminateTrustlessly, stall("bondRegHeight[" + id.String() + "]")
			}
			if av == Present && b.Height-regH < v5RegMinInterval(p) {
				restores, out, err := v5RestoresHeldStanding(v, id, r.Root)
				if out != Accept {
					return out, err
				}
				if !restores {
					return Reject, fmt.Errorf("%w: validator %s re-registered %d blocks after its last reg (R=%d)",
						ErrRegGate, id, b.Height-regH, v5RegMinInterval(p))
				}
			}
		}
		if err := v5ValidateBondRegWindow(v, p, r, nonces); err != nil {
			return Reject, err
		}
	}
	return Accept, nil
}

// v5RecentBondRegNonces mirrors Chain.recentBondRegNonces over the bounded header-chain read. The
// window is SELF-AUTHENTICATING — each hash is fixed by the Prev linkage of the one before it — so
// it is class 3, not a class-4 fact a driver could choose.
func v5RecentBondRegNonces(v StateView) ([]uint64, FloorBoxOutcome, error) {
	k := v.Params().BondRegHeadWindow
	if k <= 0 {
		k = DefaultBondRegHeadWindow
	}
	hashes, av := v.Ancestors(k)
	if av != Present {
		return nil, IndeterminateTrustlessly, stall(fmt.Sprintf("ancestors(%d) (bond-reg nonce window)", k))
	}
	nonces := make([]uint64, 0, len(hashes))
	for _, h := range hashes {
		nonces = append(nonces, BondRegNonce(h))
	}
	return nonces, Accept, nil
}

// v5RegMinInterval mirrors Chain.regMinInterval.
func v5RegMinInterval(p Params) uint64 {
	k := uint64(p.BondRegHeadWindow)
	if k == 0 {
		k = DefaultBondRegHeadWindow
	}
	r := p.BondTTLBlocks / 4
	if r < k+2 {
		r = k + 2
	}
	if ttl := p.BondTTLBlocks; ttl > 0 && r >= ttl/2 {
		r = ttl/2 - 1
	}
	return r
}

// v5RegGateActive mirrors Chain.regGateActive: strictly greater at the boundary.
func v5RegGateActive(v StateView, h uint64) (bool, FloorBoxOutcome, error) {
	if p := v.Params(); p.RegGateActivationHeight > 0 {
		return h > p.RegGateActivationHeight, Accept, nil
	}
	locked, out, err := v5ScalarBool(v, tagGateLockedIn, "gateLockedIn")
	if out != Accept {
		return false, out, err
	}
	if !locked {
		return false, Accept, nil
	}
	at, out, err := v5ScalarUint64(v, tagGateHeight, "gateHeight")
	if out != Accept {
		return false, out, err
	}
	return h > at, Accept, nil
}

// v5RestoresHeldStanding mirrors Chain.restoresHeldStanding: the #506 R-interval exemption for a
// LAPSED frozen-epoch member re-proving a root it already owns.
func v5RestoresHeldStanding(v StateView, id ports.NodeID, root ports.Hash) (bool, FloorBoxOutcome, error) {
	mature, out, err := v5MatureEpochRegime(v)
	if out != Accept {
		return false, out, err
	}
	if !mature {
		return false, Accept, nil
	}
	owner, av := v.BondRootOwner(root)
	if av == NoWitness {
		return false, IndeterminateTrustlessly, stall("bondRootOwner[" + root.String() + "]")
	}
	if av != Present || owner != id {
		return false, Accept, nil // not re-proving a root this identity already owns
	}
	sz, av := v.BondedOf(id)
	if av == NoWitness {
		return false, IndeterminateTrustlessly, stall("bonded[" + id.String() + "] (restore check)")
	}
	if sz >= v.Params().MinBond {
		return false, Accept, nil // still holds LIVE standing: a re-reg here is padding volume
	}
	set, av := v.EpochSet()
	if av != Present {
		return false, IndeterminateTrustlessly, stall("epochSet (whole set, restore check)")
	}
	_, inEpoch := set[id]
	return inEpoch, Accept, nil
}

// v5ValidateBondRegWindow mirrors Chain.validateBondRegWindow: accept if ANY nonce in the window
// validates; on failure return the CURRENT-head error.
func v5ValidateBondRegWindow(v StateView, p Params, r BondReg, nonces []uint64) error {
	var firstErr error
	for i, n := range nonces {
		err := v5ValidateBondReg(v, p, r, n)
		if err == nil {
			return nil
		}
		if i == 0 {
			firstErr = err
		}
	}
	return firstErr
}

// v5ValidateBondReg mirrors Chain.validateBondReg.
func v5ValidateBondReg(v StateView, p Params, r BondReg, nonce uint64) error {
	if len(r.Validator) != ed25519.PublicKeySize {
		return fmt.Errorf("%w: bond registration has no valid validator key", ErrBadBondReg)
	}
	if !ed25519.Verify(ed25519.PublicKey(r.Validator), r.signingBytes(nonce), r.Sig) {
		return fmt.Errorf("%w: validator %s signature", ErrBadBondReg, r.ValidatorID())
	}
	if r.Size < p.MinBond {
		return fmt.Errorf("%w: validator %s size %d below MinBond %d", ErrBadBondReg, r.ValidatorID(), r.Size, p.MinBond)
	}
	if r.Size < p.MinBondBytes {
		return fmt.Errorf("%w: validator %s size %d below anti-release floor %d", ErrBadBondReg, r.ValidatorID(), r.Size, p.MinBondBytes)
	}
	if !v.VerifyBond(r.Validator, r.Root, r.Size, nonce, r.Answer) {
		return fmt.Errorf("%w: validator %s space-time proof", ErrBadBondReg, r.ValidatorID())
	}
	return nil
}

// ---------------------------------------------------------------------------
// C1..C5 the two quorum stacks
// ---------------------------------------------------------------------------

// v5RequireProposerPrepare is C1 — Chain.requireProposerPrepare. Block-local: no state read.
func v5RequireProposerPrepare(b *Block) (FloorBoxOutcome, error) {
	h := b.Hash()
	for _, a := range b.PrepareQC {
		if a.Phase == PhasePrepare && a.Round <= b.CommitRound &&
			bytes.Equal(a.PubKey, b.Proposer) && verifyAtt(a, h) {
			return Accept, nil
		}
	}
	return Reject, ErrProposerPrepare
}

// v5CollectQuorumSigs is C2/C4 — Chain.collectQuorumSigs, the SHARED signer-set CONSTRUCTOR. The
// composition calls it; it never takes a pre-built `seen` as a parameter. Two placements inside
// are load-bearing and are preserved exactly:
//
//   - the AUTHOR SKIP happens BEFORE the phase/round exactness check, so a proposer's own
//     round-mismatched signature is not fatal (that is what makes requireProposerPrepare's
//     round <= CommitRound count-neutral);
//   - the UNQUALIFIED DROP happens AFTER the signature check, so a forged signature from an
//     unqualified id is FATAL, not silently ignored. Reversing these two is N5.
func v5CollectQuorumSigs(v StateView, b *Block, sigs []Attestation, phase uint8, round uint64) (map[ports.NodeID]bool, FloorBoxOutcome, error) {
	h := b.Hash()
	seen := make(map[ports.NodeID]bool)
	for _, a := range sigs {
		if len(a.PubKey) != ed25519.PublicKeySize {
			continue
		}
		id := a.AttesterID()
		if seen[id] || id == b.ProposerID() {
			continue // duplicates and self-attestation don't count
		}
		if a.Phase != phase || a.Round != round {
			return nil, Reject, fmt.Errorf("%w: attester %s signed (phase %d, round %d), this quorum demands (phase %d, round %d)",
				ErrBadSignature, id, a.Phase, a.Round, phase, round)
		}
		if !verifyAtt(a, h) {
			return nil, Reject, fmt.Errorf("%w: attester %s", ErrBadSignature, id)
		}
		ok, out, err := v5AttesterQualifiedAt(v, id, b.Height)
		if out != Accept {
			return nil, out, err
		}
		if !ok {
			continue // unqualified signatures are ignored, not fatal
		}
		seen[id] = true
	}
	return seen, Accept, nil
}

// v5RequireQuorumStack is C3/C5 — Chain.requireQuorumStack: the four phase-independent
// requirements Q1..Q4, IN ORDER.
func v5RequireQuorumStack(v StateView, b *Block, seen map[ports.NodeID]bool) (FloorBoxOutcome, error) {
	// Q1 count quorum.
	req, out, err := v5RequiredQuorum(v)
	if out != Accept {
		return out, err
	}
	if len(seen) < req {
		return Reject, fmt.Errorf("%w: %d qualified, need %d", ErrNoQuorum, len(seen), req)
	}
	// Q2 launch-anchor majority.
	need, out, err := v5RequiredLaunchAnchors(v)
	if out != Accept {
		return out, err
	}
	if need > 0 {
		if got := v5CountAnchorSupport(v, b.ProposerID(), seen); got < need {
			return Reject, fmt.Errorf("%w: %d of required %d", ErrAnchorRequired, got, need)
		}
	}
	// Q3 mature-epoch WEIGHT quorum: ByzantineQuorum && objective && epochsEnabled && matureEpoch.
	p := v.Params()
	if p.ByzantineQuorum && v.Objective() {
		mature, out, err := v5MatureEpochRegime(v)
		if out != Accept {
			return out, err
		}
		if mature {
			if out, err := v5RequireEpochWeightQuorum(v, b.ProposerID(), seen, b.Height); out != Accept {
				return out, err
			}
		}
	}
	// Q4 de-maturation super-quorum: everMature && objective && !matureNow().
	if v.Objective() {
		everMature, out, err := v5ScalarBool(v, tagEverMature, "everMature")
		if out != Accept {
			return out, err
		}
		if everMature {
			now, out, err := v5MatureNow(v)
			if out != Accept {
				return out, err
			}
			if !now {
				if out, err := v5RequireDeMatureSuperQuorum(v, b, seen); out != Accept {
					return out, err
				}
			}
		}
	}
	return Accept, nil
}

// v5RequiredQuorum mirrors Chain.RequiredQuorum EXACTLY, regime by regime (#380 direction (1),
// research certification CONSENSUS-380-quorum-floor-direction-1-PREDICATE-AND-CERTIFICATION
// §3): (c)/(d) the config's Quorum; (b) mature epoch: 0, the >2/3 frozen-WEIGHT rule is the bar;
// (a) bftThreshold(N). Params.Quorum is a threshold input on the legacy / opt-out leg ONLY.
// Pinned by G-D13 through the compositionHelpers row; driven by the parity oracle's mature
// regimes (the whale world, where the node accepts a zero-attestation commit).
func v5RequiredQuorum(v StateView) (int, FloorBoxOutcome, error) {
	p := v.Params()
	if !p.ByzantineQuorum || !v.Objective() {
		return p.Quorum, Accept, nil // (c) trusted opt-out, (d) legacy
	}
	mature, out, err := v5MatureEpochRegime(v)
	if out != Accept {
		return 0, out, err
	}
	if mature {
		return 0, Accept, nil // (b) the >2/3 frozen-weight rule IS the Byzantine bar (B2)
	}
	n, out, err := v5ValidatorSetSize(v)
	if out != Accept {
		return 0, out, err
	}
	return bftThreshold(n), Accept, nil // (a)
}

// v5ValidatorSetSize mirrors Chain.validatorSetSize.
func v5ValidatorSetSize(v StateView) (int, FloorBoxOutcome, error) {
	p := v.Params()
	ho, out, err := v5HandedOff(v)
	if out != Accept {
		return 0, out, err
	}
	if v.Objective() && !ho && len(p.Anchors) > 0 {
		return len(p.Anchors), Accept, nil
	}
	mature, out, err := v5MatureEpochRegime(v)
	if out != Accept {
		return 0, out, err
	}
	if mature {
		set, av := v.EpochSet()
		if av != Present {
			return 0, IndeterminateTrustlessly, stall("epochSet (whole set, N)")
		}
		return len(set), Accept, nil
	}
	// qualifiedCount: the era-4 committed accelerator, whose equality with
	// filter(bonded, slashed, MinBond) is the maintenance claim. ONE source, not two.
	q, av := v.Qualified()
	if av != Present {
		return 0, IndeterminateTrustlessly, stall("qualified (whole set, N)")
	}
	return len(q), Accept, nil
}

// v5RequiredLaunchAnchors mirrors Chain.requiredLaunchAnchors, both modes.
func v5RequiredLaunchAnchors(v StateView) (int, FloorBoxOutcome, error) {
	p := v.Params()
	if len(p.Anchors) == 0 {
		return 0, Accept, nil
	}
	ho, out, err := v5HandedOff(v)
	if out != Accept {
		return 0, out, err
	}
	if ho {
		return 0, Accept, nil
	}
	if v.Objective() {
		return len(p.Anchors)/2 + 1, Accept, nil
	}
	return p.AnchorQuorum, Accept, nil
}

// v5CountAnchorSupport mirrors Chain.countAnchorSupport.
func v5CountAnchorSupport(v StateView, proposer ports.NodeID, seen map[ports.NodeID]bool) int {
	p := v.Params()
	n := 0
	for id := range seen {
		if p.Anchors[id] {
			n++
		}
	}
	if v.Objective() && p.Anchors[proposer] {
		n++
	}
	return n
}

// v5RequireEpochWeightQuorum is Q3 — Chain.requireEpochWeightQuorum. Its tail is
// `support := set[proposer] + sum(set[seen])` with NO author screen, and that is CORRECT: P4
// already refused a slashed or non-member proposer. The screen belongs to the PATH, not to this
// function — reproducing this tally without P4 is N1.
func v5RequireEpochWeightQuorum(v StateView, proposer ports.NodeID, seen map[ports.NodeID]bool, h uint64) (FloorBoxOutcome, error) {
	set, out, err := v5EffectiveEpochSet(v, h)
	if out != Accept {
		return out, err
	}
	var total int64
	for _, w := range set {
		total += w
	}
	if total <= 0 {
		return Accept, nil
	}
	support := set[proposer]
	for id := range seen {
		support += set[id]
	}
	if 3*support <= 2*total {
		return Reject, fmt.Errorf("%w: coalition holds %d of %d bonded weight (need >%d)",
			ErrNoQuorumWeight, support, total, 2*total/3)
	}
	return Accept, nil
}

// v5RequireDeMatureSuperQuorum is Q4 — Chain.requireDeMatureSuperQuorum.
func v5RequireDeMatureSuperQuorum(v StateView, b *Block, seen map[ports.NodeID]bool) (FloorBoxOutcome, error) {
	bonded, av := v.Bonded()
	if av != Present {
		return IndeterminateTrustlessly, stall("bonded (whole set, de-mature super-quorum)")
	}
	var total int64
	for _, w := range bonded {
		total += w
	}
	if total <= 0 {
		return Accept, nil
	}
	committed := bonded[b.ProposerID()]
	for id := range seen {
		committed += bonded[id]
	}
	need := (2*total + 2) / 3 // ⌈2·total/3⌉
	if committed < need {
		return Reject, fmt.Errorf("%w: coalition holds %d MiB of %d MiB bonded (need ≥%d MiB)",
			ErrDeMatureQuorum, committed>>20, total>>20, need>>20)
	}
	return Accept, nil
}

// v5MatureNow mirrors Chain.matureNow's OBJECTIVE branch — MatureCoefficient() =
// min(NakamotoOperators, NakamotoDomains) over the committed ledger (C2Metric), restricted to
// validatorsSeen, non-anchor, non-slashed, bonded ≥ MinBond. The legacy branch is unreachable
// from the accept path: Q4 is gated on objective() and only Q4 reads matureNow.
func v5MatureNow(v StateView) (bool, FloorBoxOutcome, error) {
	p := v.Params()
	seenSet, av := v.ValidatorsSeen()
	if av != Present {
		return false, IndeterminateTrustlessly, stall("validatorsSeen (whole set, maturity metric)")
	}
	// Canonical id order so the fold is visibly independent of map order (it is a sum and a
	// sort, so order-free by construction; the explicit sort makes that checkable).
	ids := make([]ports.NodeID, 0, len(seenSet))
	for id := range seenSet {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return bytes.Compare(ids[i][:], ids[j][:]) < 0 })

	sizes := make([]int64, 0, len(ids))
	domainWeight := make(map[uint64]int64)
	var zeroDomainWeights []int64
	var total int64
	for _, id := range ids {
		if p.Anchors[id] {
			continue
		}
		slashed, av := v.Slashed(id)
		if av == NoWitness {
			return false, IndeterminateTrustlessly, stall("slashed[" + id.String() + "] (maturity metric)")
		}
		if slashed {
			continue
		}
		sz, av := v.BondedOf(id)
		if av == NoWitness {
			return false, IndeterminateTrustlessly, stall("bonded[" + id.String() + "] (maturity metric)")
		}
		if sz < p.MinBond {
			continue
		}
		sizes = append(sizes, sz)
		total += sz
		d, av := v.BondDomain(id)
		if av == NoWitness {
			return false, IndeterminateTrustlessly, stall("bondDomain[" + id.String() + "] (maturity metric)")
		}
		if d != 0 {
			domainWeight[d] += sz
		} else {
			zeroDomainWeights = append(zeroDomainWeights, sz)
		}
	}
	if total == 0 {
		return 0 >= p.MatureValidators, Accept, nil
	}
	margin := p.OperatorMargin
	if margin < 1 {
		margin = 1
	}
	// ONE body for the coefficient arithmetic (M-1A-1): nakamotoCoefficient is the receiverless
	// fold the box's maturity recompute already uses (floorbox_recompute_maturity_v5.go), pinned
	// byte-for-byte to C2Metric. A third inline copy here was the drift surface the round exists
	// to close; TestM1A1_V5MatureNowEqualsNodeMatureNow reddens on a divergent coefficient.
	bondsK := nakamotoCoefficient(sizes, total)
	groups := make([]int64, 0, len(domainWeight)+len(zeroDomainWeights))
	for _, w := range domainWeight {
		groups = append(groups, w)
	}
	groups = append(groups, zeroDomainWeights...)
	domainsK := nakamotoCoefficient(groups, total)
	k := bondsK / margin
	if domainsK < k {
		k = domainsK
	}
	return k >= p.MatureValidators, Accept, nil
}
