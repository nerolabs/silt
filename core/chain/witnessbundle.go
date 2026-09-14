package chain

import (
	"encoding/binary"
	"errors"
	"fmt"
	"sort"

	"github.com/nerolabs/silt/core/statehash"
	"github.com/nerolabs/silt/ports"
)

// The witness BUNDLE: the O(payload) pre-state evidence a floor box needs to reproduce one block's
// committed state-root transition, produced by a node that holds the tree.
//
// WHY IT IS A SEPARATE CALL FROM THE POINT ACCESSORS. WitnessSource answers questions the box asks
// one at a time, driven by the composition's own reads. StateRootWitness is different in kind: it
// is the evidence the recompute FOLDS, and the box cannot ask for it key by key because the fold
// needs every changed leaf's pre-state proof in one bundle before it can compute a post-root at
// all. So the provider assembles it from the block's payload and the committed state it holds.
//
// HOW IT IS BUILT, and why this shape rather than a per-class derivation. The provider applies the
// candidate to a private copy of its own chain and DIFFS the committed leaf set before against
// after. That diff IS the changed-key set — the same set the box derives from the payload, arrived
// at by running the transition instead of by re-deriving it. The alternative, a second
// implementation of every class's write-set living on the serving side, would be a second rule for
// one transition, and the two would drift the first time a class changed.
//
// IT IS NOT A DELEGATION OF THE BOX'S JUDGEMENT, and that is what makes it safe to take from a
// stranger. The box DERIVES the changed-key set from the payload itself and matches each derived
// write to a supplied witness; a witness for a key the box did not derive is IGNORED, and a derived
// write with no matching witness STALLS. Every proof is verified against prevStateRoot by the fold
// before it is used, and the terminal post-root equality catches any remaining lie. A provider that
// omits, injects, or forges produces a stall, never an acceptance — the same property the point
// accessors have. It follows that the provider may OVER-supply, and does: the five whole-set digest
// pre-images travel whether or not this block touches them, because the cost is small and the
// alternative is the provider predicting the box's scope decisions.
//
// THE BLOCK IS UNTRUSTED HERE TOO. It is read for its payload only — which leaves it would change,
// which ids its carrier names, which roots it registers — and nothing it claims is believed: every
// value in the bundle is this provider's own committed state, before or after applying the payload.

var (
	// ErrBundleNoPostState is the provider's refusal when it cannot produce the post-apply state the
	// diff and the class-M maturity witness are built from. A provider that cannot apply a block to
	// a copy of its own chain is broken, not merely unhelpful — and serving a bundle without that
	// state would stall the box with a fold error that reads as a forged root, sending the operator
	// after the wrong thing.
	ErrBundleNoPostState = errors.New("chain: witness bundle — the provider could not produce the post-apply state the bundle is derived from")
	// ErrBundleNoPreState is the provider's refusal when a leaf the bundle must anchor cannot be
	// proven against the head it was built over. A provider that cannot prove its own committed
	// state is broken too.
	ErrBundleNoPreState = errors.New("chain: witness bundle — the provider could not prove a leaf against the committed state root it was built over")
)

// Bundle assembles the StateRootWitness for block b against the committed state this provider
// snapshots.
func (p *WitnessProvider) Bundle(b Block) (StateRootWitness, error) {
	if p.snap == nil {
		return StateRootWitness{}, fmt.Errorf("%w: this provider holds no applicable copy of its chain", ErrBundleNoPostState)
	}
	post := p.snap.cloneForDryRun()
	post.apply(b)
	postLeaves := post.stateRootLeavesV5()
	postValue := make(map[string][]byte, len(postLeaves))
	for _, l := range postLeaves {
		postValue[string(l.Key)] = l.Value
	}

	var w StateRootWitness
	var err error

	// (1) THE CHANGED LEAVES, as the diff of the committed leaf set across the payload. Every leaf
	// is served with its delete siblings, because whether a derived write is a delete is the box's
	// derivation and not the provider's.
	for _, key := range p.bundleKeys(b, postValue) {
		cl, cErr := p.changedLeaf(key)
		if cErr != nil {
			return StateRootWitness{}, cErr
		}
		w.ChangedLeaves = append(w.ChangedLeaves, cl)
	}

	// (2) The five whole-set digest pre-images, served unconditionally.
	for _, tag := range []string{tagSlashedRoot, tagBondedRoot, tagQualifiedRoot, tagValidatorsSeenRoot, tagEpochSetRoot} {
		proof, pErr := p.prove(statehash.Key(tag, nil))
		if pErr != nil {
			return StateRootWitness{}, pErr
		}
		w.DigestPreSets = append(w.DigestPreSets, StateRootDigestWitness{
			Tag: tag, PreIDs: sortIDs(p.members[tag]), Proof: proof,
		})
	}

	// (3) One qualification screen per id the carrier names. The box computes qualification itself,
	// from its own config, over these anchored values.
	seenCarried := map[ports.NodeID]struct{}{}
	for i := range b.LastCommit {
		id := b.LastCommit[i].AttesterID()
		if _, done := seenCarried[id]; done {
			continue
		}
		seenCarried[id] = struct{}{}
		sc, sErr := p.attScreen(id)
		if sErr != nil {
			return StateRootWitness{}, sErr
		}
		w.AttScreens = append(w.AttScreens, sc)
	}

	// (4) The maturity carrier, required on every block. On a chain that has not latched, the box
	// also has to reproduce the metric that decides the latch, and that metric reads the
	// validatorsSeen set as it stands AFTER this block — so it is proven against the root the block
	// itself commits, which is what makes a floor box work at LAUNCH and not only on a matured
	// network.
	mat := &StateRootMaturityWitness{}
	if mat.EverMature, err = p.scalarWitness(tagEverMature); err != nil {
		return StateRootWitness{}, err
	}
	if mat.MatureEpoch, err = p.scalarWitness(tagMatureEpoch); err != nil {
		return StateRootWitness{}, err
	}
	if !boolLeaf(p.leaves[string(statehash.Key(tagEverMature, nil))]) {
		if mat.SeenSet, err = seenSetWitness(post, postValue); err != nil {
			return StateRootWitness{}, err
		}
	}
	w.Maturity = mat

	// (5) The TTL scope gate reads dueBucket[b.Height] on every block when the box's own config has
	// a TTL, and the sweep fires when that bucket is occupied. Both are served from the pre-state
	// bucket this provider holds: one proof, plus the expired set when there is one.
	if w.DueBucketProof, err = p.prove(dueBucketKey(b.Height)); err != nil {
		return StateRootWitness{}, err
	}
	if expired := sortIDs(idsOf(p.snap.dueBucket[b.Height])); len(expired) > 0 {
		wit, sibs, sErr := p.prover.ProveWithSiblings(dueBucketKey(b.Height))
		if sErr != nil {
			return StateRootWitness{}, fmt.Errorf("%w: dueBucket[%d]: %v", ErrBundleNoPreState, b.Height, sErr)
		}
		w.TTLSweep = &StateRootTTLWitness{
			Height: b.Height, Members: expired, BucketProof: wit, BucketDeleteSiblings: sibs,
		}
	}

	// (6) Bond registrations: the committed ownership the displacement branch reads, per registered
	// root, and the pre-state members of every due-bucket the registrations move.
	for _, reg := range b.BondRegs {
		sc, rErr := p.bondRegScreen(reg.Root)
		if rErr != nil {
			return StateRootWitness{}, rErr
		}
		w.BondRegScreens = append(w.BondRegScreens, sc)
	}
	for _, due := range movedBuckets(p.snap.dueBucket, post.dueBucket, b.Height) {
		wit, sibs, bErr := p.prover.ProveWithSiblings(dueBucketKey(due))
		if bErr != nil {
			return StateRootWitness{}, fmt.Errorf("%w: dueBucket[%d]: %v", ErrBundleNoPreState, due, bErr)
		}
		w.BondRegBuckets = append(w.BondRegBuckets, StateRootBucketWitness{
			DueHeight: due, PreMembers: sortIDs(idsOf(p.snap.dueBucket[due])), Proof: wit, DeleteSiblings: sibs,
		})
	}

	// (7) The epoch rotation. A boundary is the one block where the validator set is re-frozen, and
	// it is recognised here by the epoch actually turning — epochStart advances at every boundary
	// and at no other block — rather than by re-deriving the box's own boundary rule.
	if post.epochStart != p.snap.epochStart {
		if w.Rotate, err = p.rotateWitness(post); err != nil {
			return StateRootWitness{}, err
		}
	}
	return w, nil
}

// bundleKeys is the set of leaf keys the bundle carries a pre-state proof for: every leaf the
// payload CHANGED, plus every leaf it TOUCHED without changing.
//
// THE DIFF ALONE IS NOT ENOUGH, and the reason is the whole shape of the thing. The box derives its
// write-set from the payload, and some of those writes are idempotent — a validator renewing its
// bond re-writes the same owner into the same root, and the committed leaf does not move. The box
// still derives that write and still requires a witness for it, so a provider that served only what
// changed would stall an honest box on the most ordinary block a live network produces.
//
// So the identities and roots the payload NAMES contribute their whole per-key family, whether or
// not this block moved them. That is over-supply, and over-supply is free: the box's derived set is
// authoritative, and a witness for a key it did not derive is ignored.
func (p *WitnessProvider) bundleKeys(b Block, post map[string][]byte) [][]byte {
	seen := map[string]bool{}
	var keys []string
	add := func(k []byte) {
		if !seen[string(k)] {
			seen[string(k)] = true
			keys = append(keys, string(k))
		}
	}
	// What moved.
	for k, v := range post {
		if string(p.leaves[k]) != string(v) {
			add([]byte(k))
		}
	}
	for k, v := range p.leaves {
		if string(post[k]) != string(v) {
			add([]byte(k))
		}
	}
	// What the payload touched. The two derivations the box itself runs are called here rather
	// than restated, so the entry/revocation and slash write-sets cannot drift between the side
	// that serves and the side that asks.
	for _, wr := range applyEntriesRevocationsWriteSet(b) {
		add(wr.key)
	}
	for _, wr := range stateRootSlashWriteSet(b, p.idSet(tagBondedRoot), p.idSet(tagQualifiedRoot)) {
		add(wr.key)
	}
	ids := map[ports.NodeID]struct{}{}
	roots := map[ports.Hash]struct{}{}
	for i := range b.LastCommit {
		ids[b.LastCommit[i].AttesterID()] = struct{}{}
	}
	for i := range b.Slashes {
		ids[b.Slashes[i].CulpritID()] = struct{}{}
	}
	for i := range b.BondRegs {
		ids[b.BondRegs[i].ValidatorID()] = struct{}{}
		root := b.BondRegs[i].Root
		roots[root] = struct{}{}
		// The root's CURRENT owner, whose standing the displacement branch may strip.
		if v := p.leaves[string(statehash.Key(tagBondRootOwner, root[:]))]; len(v) == len(ports.Hash{}) {
			var prior ports.NodeID
			copy(prior[:], v)
			ids[prior] = struct{}{}
		}
	}
	for id := range p.snap.dueBucket[b.Height] { // the bonds this height expires
		ids[id] = struct{}{}
	}
	for id := range p.snap.epochSet { // the set a boundary re-freezes, both sides of the turn
		ids[id] = struct{}{}
	}
	for i := range b.Entries {
		roots[b.Entries[i].Root] = struct{}{}
	}
	for _, r := range b.Revocations {
		roots[r] = struct{}{}
	}
	for _, r := range b.Unrevocations {
		roots[r] = struct{}{}
	}
	for id := range ids {
		for _, tag := range []string{tagBonded, tagBondRegHeight, tagRegVersion, tagBondDomain,
			tagQualified, tagValidatorsSeen, tagSlashed, tagEpochSet} {
			add(statehash.Key(tag, id[:]))
		}
	}
	for root := range roots {
		for _, tag := range []string{tagByRoot, tagRevoked, tagBondRootOwner, tagBondRootProven} {
			add(statehash.Key(tag, root[:]))
		}
	}
	sort.Strings(keys)
	out := make([][]byte, 0, len(keys))
	for _, k := range keys {
		out = append(out, []byte(k))
	}
	return out
}

// changedLeaf proves one leaf's committed pre-state. A leaf that is ABSENT pre-state is served with
// a nil OldValue and a non-membership proof; that is what an add looks like, and it is a proven
// fact rather than a silence.
func (p *WitnessProvider) changedLeaf(key []byte) (StateRootChangedLeafWitness, error) {
	wit, sibs, err := p.prover.ProveWithSiblings(key)
	if err != nil {
		return StateRootChangedLeafWitness{}, fmt.Errorf("%w: leaf %x: %v", ErrBundleNoPreState, key, err)
	}
	return StateRootChangedLeafWitness{
		Key: key, OldValue: p.leaves[string(key)], Proof: wit, DeleteSiblings: sibs,
	}, nil
}

// attScreen reads one carried id's three committed qualification inputs and proves each.
func (p *WitnessProvider) attScreen(id ports.NodeID) (StateRootAttScreen, error) {
	sc := StateRootAttScreen{Attester: id}
	var err error
	if sc.SlashedProof, err = p.prove(statehash.Key(tagSlashed, id[:])); err != nil {
		return sc, err
	}
	sc.Slashed = len(p.leaves[string(statehash.Key(tagSlashed, id[:]))]) > 0
	epochKey := statehash.Key(tagEpochSet, id[:])
	if sc.EpochSetProof, err = p.prove(epochKey); err != nil {
		return sc, err
	}
	sc.EpochSetValue = p.leaves[string(epochKey)]
	sc.InEpochSet = len(sc.EpochSetValue) > 0
	bondedKey := statehash.Key(tagBonded, id[:])
	if sc.BondedProof, err = p.prove(bondedKey); err != nil {
		return sc, err
	}
	sc.BondedPresent, sc.BondedSize = int64Leaf(p.leaves[string(bondedKey)])
	return sc, nil
}

// bondRegScreen reads the committed ownership of one registered bond root: whether it is already
// claimed, by whom, and whether that prior claim was proven.
func (p *WitnessProvider) bondRegScreen(root ports.Hash) (StateRootBondRegScreen, error) {
	sc := StateRootBondRegScreen{Root: root}
	var err error
	ownerKey := statehash.Key(tagBondRootOwner, root[:])
	if sc.OwnerProof, err = p.prove(ownerKey); err != nil {
		return sc, err
	}
	if v := p.leaves[string(ownerKey)]; len(v) == len(ports.Hash{}) {
		sc.Claimed = true
		copy(sc.PriorOwner[:], v)
	}
	provenKey := statehash.Key(tagBondRootProven, root[:])
	if sc.ProvenProof, err = p.prove(provenKey); err != nil {
		return sc, err
	}
	sc.PriorProven = boolLeaf(p.leaves[string(provenKey)])
	return sc, nil
}

// rotateWitness builds the class-P epoch-boundary witness: the frozen set as the block leaves it,
// the prior members that leave it, and the eight scalars a boundary can move. Every member's
// committed PRE-state is proven against the parent root; the values are read from the state the
// block produces, which is what the box cross-checks them against.
func (p *WitnessProvider) rotateWitness(post *Chain) (*StateRootRotateWitness, error) {
	r := &StateRootRotateWitness{}
	member := func(id ports.NodeID, weight int64, withSiblings bool) (StateRootRotateMember, error) {
		m := StateRootRotateMember{ID: id, Weight: weight}
		if v, known := post.regVersion[id]; known {
			m.RegVersion, m.RegVersionKnown = v, true
		}
		var err error
		if m.RegVersionProof, err = p.prove(statehash.Key(tagRegVersion, id[:])); err != nil {
			return m, err
		}
		if m.QualifiedProof, err = p.prove(statehash.Key(tagQualified, id[:])); err != nil {
			return m, err
		}
		epochKey := statehash.Key(tagEpochSet, id[:])
		m.EpochSetOldValue = p.leaves[string(epochKey)]
		if withSiblings {
			wit, sibs, sErr := p.prover.ProveWithSiblings(epochKey)
			if sErr != nil {
				return m, fmt.Errorf("%w: epochSet[%x]: %v", ErrBundleNoPreState, id[:8], sErr)
			}
			m.EpochSetProof, m.EpochSetDeleteSiblings = wit, sibs
			return m, nil
		}
		m.EpochSetProof, err = p.prove(epochKey)
		return m, err
	}
	for _, id := range sortIDs(weightedIDs(post.epochSet)) {
		m, err := member(id, post.epochSet[id], false)
		if err != nil {
			return nil, err
		}
		r.Members = append(r.Members, m)
	}
	// The members the freeze drops: their epochSet leaf is DELETED, so they travel with the
	// off-path siblings the delete's replay resolves.
	for _, id := range sortIDs(weightedIDs(p.snap.epochSet)) {
		if _, stays := post.epochSet[id]; stays {
			continue
		}
		m, err := member(id, p.snap.epochSet[id], true)
		if err != nil {
			return nil, err
		}
		r.PriorEpochSet = append(r.PriorEpochSet, m)
	}
	for _, s := range []struct {
		tag  string
		into *StateRootRotateScalar
	}{
		{tagEpochStart, &r.EpochStart}, {tagMatureEpoch, &r.MatureEpoch},
		{tagGateLockedIn, &r.GateLockedIn}, {tagGateHeight, &r.GateHeight},
		{tagEra3LockedIn, &r.Era3LockedIn}, {tagEra3Height, &r.Era3Height},
		{tagEra4LockedIn, &r.Era4LockedIn}, {tagEra4Height, &r.Era4Height},
	} {
		sw, err := p.scalarWitness(s.tag)
		if err != nil {
			return nil, err
		}
		*s.into = sw
	}
	return r, nil
}

// seenSetWitness builds the class-M maturity set for a block on a chain that has not yet latched.
// Every proof resolves against the state root the BLOCK commits, not against the parent's — that is
// what the recompute checks it against, and it is why this needs the post-apply state.
func seenSetWitness(post *Chain, postValue map[string][]byte) (SeenSetWitness, error) {
	prover, err := statehash.NewProver(post.stateRootLeavesV5())
	if err != nil {
		return SeenSetWitness{}, fmt.Errorf("%w: %v", ErrBundleNoPostState, err)
	}
	prove := func(key []byte) (statehash.Witness, error) {
		wit, pErr := prover.Prove(key)
		if pErr != nil {
			return statehash.Witness{}, fmt.Errorf("%w: post-state leaf %x: %v", ErrBundleNoPostState, key, pErr)
		}
		return wit, nil
	}
	rootKey := statehash.Key(tagValidatorsSeenRoot, nil)
	rootWit, err := prove(rootKey)
	if err != nil {
		return SeenSetWitness{}, err
	}
	out := SeenSetWitness{
		IDs:             sortIDs(flaggedIDs(post.validatorsSeen)),
		SeenRootWitness: rootWit,
		SeenRootValue:   postValue[string(rootKey)],
		Members:         map[ports.NodeID]MemberStateWitness{},
	}
	for _, id := range out.IDs {
		m := MemberStateWitness{}
		bondedKey := statehash.Key(tagBonded, id[:])
		if m.BondedProof, err = prove(bondedKey); err != nil {
			return SeenSetWitness{}, err
		}
		_, m.Bonded = int64Leaf(postValue[string(bondedKey)])
		domainKey := statehash.Key(tagBondDomain, id[:])
		if m.DomainProof, err = prove(domainKey); err != nil {
			return SeenSetWitness{}, err
		}
		if v := postValue[string(domainKey)]; len(v) == 8 {
			m.DomainPresent, m.Domain = true, binary.BigEndian.Uint64(v)
		}
		slashedKey := statehash.Key(tagSlashed, id[:])
		if m.SlashedProof, err = prove(slashedKey); err != nil {
			return SeenSetWitness{}, err
		}
		m.Slashed = len(postValue[string(slashedKey)]) > 0
		out.Members[id] = m
	}
	return out, nil
}

// prove is the provider's pre-state proof of one leaf, with the failure named for the key.
func (p *WitnessProvider) prove(key []byte) (statehash.Witness, error) {
	wit, err := p.prover.Prove(key)
	if err != nil {
		return statehash.Witness{}, fmt.Errorf("%w: leaf %x: %v", ErrBundleNoPreState, key, err)
	}
	return wit, nil
}

// scalarWitness proves one committed scalar leaf and carries its committed value.
func (p *WitnessProvider) scalarWitness(tag string) (StateRootRotateScalar, error) {
	key := statehash.Key(tag, nil)
	wit, err := p.prove(key)
	if err != nil {
		return StateRootRotateScalar{}, err
	}
	return StateRootRotateScalar{OldValue: p.leaves[string(key)], Proof: wit}, nil
}

// dueBucketKey is the committed leaf key of the TTL bucket at one height.
func dueBucketKey(h uint64) []byte {
	var hk [8]byte
	binary.BigEndian.PutUint64(hk[:], h)
	return statehash.Key(tagDueBucket, hk[:])
}

// movedBuckets is the set of due-heights whose bucket membership changed across the payload,
// ascending. The sweep height is excluded: its bucket is the class-T witness's own, and serving it
// twice would put two witnesses in front of one key.
func movedBuckets(pre, post map[uint64]map[ports.NodeID]struct{}, sweepHeight uint64) []uint64 {
	moved := map[uint64]bool{}
	mark := func(m map[uint64]map[ports.NodeID]struct{}, other map[uint64]map[ports.NodeID]struct{}) {
		for h, members := range m {
			if h == sweepHeight {
				continue
			}
			if !idSetsEqual(setOf(members), setOf(other[h])) {
				moved[h] = true
			}
		}
	}
	mark(pre, post)
	mark(post, pre)
	out := make([]uint64, 0, len(moved))
	for h := range moved {
		out = append(out, h)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// idSet projects one whole-set keyspace's member list into a set.
func (p *WitnessProvider) idSet(digestTag string) map[ports.NodeID]struct{} {
	out := make(map[ports.NodeID]struct{}, len(p.members[digestTag]))
	for _, id := range p.members[digestTag] {
		out[id] = struct{}{}
	}
	return out
}

// idsOf and setOf project a bucket's member set both ways.
func idsOf(m map[ports.NodeID]struct{}) []ports.NodeID {
	out := make([]ports.NodeID, 0, len(m))
	for id := range m {
		out = append(out, id)
	}
	return out
}

func setOf(m map[ports.NodeID]struct{}) map[ports.NodeID]struct{} {
	if m == nil {
		return map[ports.NodeID]struct{}{}
	}
	return m
}

// boolLeaf reads a committed bool scalar leaf. An absent leaf is false, which is what an un-latched
// chain commits.
func boolLeaf(v []byte) bool { return len(v) == 1 && v[0] == 1 }

// int64Leaf reads a committed int64 weight leaf, reporting whether it is present at all — the
// distinction between a committed zero and an absent key, which the committed tree keeps.
func int64Leaf(v []byte) (bool, int64) {
	if len(v) != 8 {
		return false, 0
	}
	return true, int64(binary.BigEndian.Uint64(v))
}
