package chain

import (
	"fmt"
	"sort"

	"github.com/nerolabs/silt/core/statehash"
	"github.com/nerolabs/silt/ports"
)

// era-4 (v5) trustless floor-box RECOMPUTE — the COMPOSITION of the id-keyed transition classes.
//
// Three of the classes the recompute reproduces write the SAME committed keys. A bond registration
// (B) adds an id to bonded and qualified; a TTL expiry (T) removes one from both; a slash (S)
// removes one from both and adds it to slashed. Each of those keyspaces carries a whole-set digest
// scalar at one fixed key, and B and T both move the TTL due-buckets. A block may carry all three.
//
// WHY EACH CLASS CANNOT DERIVE ITS OWN POST-SET. A whole-set digest is an MTH over the complete
// post-state id list, so its value is a function of EVERY class's delta, not of one class's. A class
// that derived its post-set from the anchored pre-state alone therefore computes a digest for a
// state no block ever commits: on a block carrying a registration and a slash, B computes
// nodeSetMTH(pre u fresh) and S computes nodeSetMTH(pre \ culprit), while the committed value is
// nodeSetMTH((pre u fresh) \ culprit) — which is neither. Two ops at one key also make the fold's
// verdict depend on slice position, and a duplicate DELETE of one dueBucket leaf is refused outright.
//
// THE COMPOSITION, AND ITS ORDER. The classes run once, over ONE running post-state, in the order
// apply runs them (chain.go): bond registrations, then the TTL sweep, then slashes. Each digest,
// each per-member leaf and each due-bucket leaf is then emitted exactly ONCE, from the state the
// last class leaves behind.
//
// THE ORDER IS LOAD-BEARING, not a convention. apply resets bondRegHeight[id] inside the
// registration loop and the sweep that follows reads that map, so an id that renews at the very
// height its bond comes due does NOT expire — and its due-bucket leaf moves once, not twice. On a
// live swarm that is the ordinary block: a proposer renews its bond as it proposes, and under a
// short TTL the renewal lands on the height the old registration falls due. Running the sweep
// against the pre-state instead would expire a validator the chain kept bonded.
//
// WHAT IS STILL DERIVED, AND WHAT IS STILL ANCHORED. Nothing here trusts the witness any further
// than the classes already did: every pre-state set is proven against prevStateRoot before it is
// read, the per-class deltas are the same derivations as before, and every emitted op is verified by
// the fold and then by the terminal root equality. Composing them changes which state each class
// reads, not what the box is willing to believe.

// idKeyspaces pairs each composed keyspace's per-member leaf tag with the whole-set digest scalar
// that commits its membership. One table, because every part of the composition — anchoring, digest
// emission, and the pre-state membership a net-zero write is measured against — walks the same three.
var idKeyspaces = []struct{ leafTag, digestTag string }{
	{tagBonded, tagBondedRoot},
	{tagQualified, tagQualifiedRoot},
	{tagSlashed, tagSlashedRoot},
}

// idSetTransition is one block's reproduced effect on the three id-keyed committed keyspaces —
// bonded, qualified and slashed — and on the TTL due-buckets, composed across every class that
// writes them.
type idSetTransition struct {
	prevStateRoot ports.Hash
	byTag         map[string]*StateRootDigestWitness

	// pre holds each keyspace's committed pre-state member set, proven against prevStateRoot on
	// first read; post holds the running post-state, seeded from pre and carried through B -> T -> S
	// so every class reads what the classes before it wrote. Both are keyed by digest tag. A tag
	// absent from pre was never read, because no class this block carries reads that keyspace.
	pre  map[string]map[ports.NodeID]struct{}
	post map[string]map[ports.NodeID]struct{}

	// buckets is the merged per-due-height membership delta of every class that moves a bond's TTL
	// clock: a registration vacating its old bucket and filling a new one, and the sweep draining
	// the bucket that has come due.
	buckets map[uint64]bucketMove

	// writes is every per-member leaf write the classes derive, in apply order. A key written by
	// more than one class keeps the LAST value; see netWrites.
	writes []stateRootWrite

	// registered is the ids whose registration survived class B's screens — the ids whose TTL clock
	// this block reset, and so the members of the due bucket that do NOT expire.
	registered map[ports.NodeID]struct{}
	// expired is the ids the sweep actually removes: the committed members of the bucket that comes
	// due, less the ids that re-registered in this same block.
	expired []ports.NodeID

	// qualWrites and regVerWrites are class B's per-id qualified and regVersion writes, which the
	// class-P freeze cross-checks an in-block bond's frozen weight and tally readiness against.
	qualWrites   map[ports.NodeID][]byte
	regVerWrites map[ports.NodeID]uint8
}

// composeIDSetTransition runs classes B, T and S over one running post-state and returns the
// composed transition. It anchors only the keyspaces the block's classes actually read: a block with
// no registration, no sweep and no slash needs no id-set witness at all, and one that merely turns an
// epoch needs only the qualified set its freeze copies.
func (c *Chain) composeIDSetTransition(
	prevStateRoot ports.Hash,
	b Block,
	w StateRootWitness,
	boundary bool,
) (*idSetTransition, error) {
	t := &idSetTransition{
		prevStateRoot: prevStateRoot,
		byTag:         digestWitnessesByTag(w.DigestPreSets),
		pre:           map[string]map[ports.NodeID]struct{}{},
		post:          map[string]map[ports.NodeID]struct{}{},
		buckets:       map[uint64]bucketMove{},
		registered:    map[ports.NodeID]struct{}{},
		qualWrites:    map[ports.NodeID][]byte{},
		regVerWrites:  map[ports.NodeID]uint8{},
	}

	// (B) Bond registrations run FIRST, exactly as apply runs them. The delta is derived from the
	// block's own payload against the box's own screens; it reports the whole post-state of both sets
	// it touches, so the running state adopts them wholesale.
	if len(b.BondRegs) > 0 {
		if err := t.anchor(tagBondedRoot, tagQualifiedRoot, tagSlashedRoot); err != nil {
			return nil, err
		}
		delta, dErr := c.bondRegDelta(prevStateRoot, b, w,
			t.preSet(tagBondedRoot), t.preSet(tagQualifiedRoot), t.preSet(tagSlashedRoot))
		if dErr != nil {
			return nil, dErr
		}
		t.post[tagBondedRoot] = delta.postBonded
		t.post[tagQualifiedRoot] = delta.postQual
		t.writes = append(t.writes, delta.writes...)
		t.registered = delta.registered
		t.qualWrites = delta.qualWrites
		t.regVerWrites = delta.regVerWrites
		mergeBucketMoves(t.buckets, delta.bucketMoves)
	}

	// (T) The TTL sweep runs SECOND, over the state class B left. The expired set is the committed
	// members of the due bucket LESS the ids that just re-registered: apply's sweep reads
	// bondRegHeight after the registration loop has reset it, so a renewal at the due height keeps
	// its standing. The bucket empties either way — the renewals left through class B's own move,
	// the rest expire here.
	if w.TTLSweep != nil {
		if err := t.anchor(tagBondedRoot, tagQualifiedRoot); err != nil {
			return nil, err
		}
		t.expired = expiredAfterRenewals(w.TTLSweep.Members, t.registered)
		t.writes = append(t.writes, stateRootTTLWriteSet(t.expired, w.TTLSweep.Height, t.Qualified())...)
		drained := touchBucketMove(t.buckets, w.TTLSweep.Height)
		for _, id := range t.expired {
			delete(t.post[tagBondedRoot], id)
			delete(t.post[tagQualifiedRoot], id)
			drained.deletes[id] = struct{}{}
		}
	}

	// (S) Slashes run LAST. The per-member evictions read the RUNNING sets, so a culprit this same
	// block bonded is evicted from what the registration wrote rather than from a pre-state it was
	// never in.
	if len(b.Slashes) > 0 {
		if err := t.anchor(tagSlashedRoot, tagBondedRoot, tagQualifiedRoot); err != nil {
			return nil, err
		}
		t.writes = append(t.writes, stateRootSlashWriteSet(b, t.post[tagBondedRoot], t.Qualified())...)
		for i := range b.Slashes {
			culprit := b.Slashes[i].CulpritID()
			t.post[tagSlashedRoot][culprit] = struct{}{}
			delete(t.post[tagBondedRoot], culprit)
			delete(t.post[tagQualifiedRoot], culprit)
		}
	}

	// A boundary freezes the qualified set even when no class above touched it, so the freeze source
	// is anchored here rather than reconstructed a second time in class P.
	if boundary {
		if err := t.anchor(tagQualifiedRoot); err != nil {
			return nil, err
		}
	}
	return t, nil
}

// anchor proves each named keyspace's committed pre-state member set against prevStateRoot and seeds
// the running post-state from it. It is idempotent, and each class names the tags IN ITS OWN ORDER —
// so a block that carries one class and no witness for it stalls naming the digest THAT class reads
// first, and two classes starved of the same witness stay distinguishable by the name they land on.
func (t *idSetTransition) anchor(tags ...string) error {
	for _, tag := range tags {
		if _, done := t.pre[tag]; done {
			continue
		}
		set, err := anchoredPreSet(t.byTag, tag, t.prevStateRoot)
		if err != nil {
			return err
		}
		t.pre[tag], t.post[tag] = set, cloneIDSet(set)
	}
	return nil
}

// preSet is one keyspace's anchored committed pre-state membership, empty if no class read it.
func (t *idSetTransition) preSet(tag string) map[ports.NodeID]struct{} { return t.pre[tag] }

// Qualified is the POST-apply qualified id-set the class-P epoch freeze copies — the anchored
// pre-state with this block's registration, expiry and slash deltas applied in apply's order.
func (t *idSetTransition) Qualified() map[ports.NodeID]struct{} { return t.post[tagQualifiedRoot] }

// digestOps emits each touched whole-set digest ONCE, over the composed post-state set. A digest
// whose membership the block did not move is not emitted, which is the same "emit only changed
// leaves" rule every other class follows and what keeps the folded key set equal to the committed
// leaf diff.
func (t *idSetTransition) digestOps() []statehash.FoldOp {
	var ops []statehash.FoldOp
	for _, ks := range idKeyspaces {
		pre, read := t.pre[ks.digestTag]
		if !read || idSetsEqual(pre, t.post[ks.digestTag]) {
			continue
		}
		ops = append(ops, digestFoldOp(ks.digestTag, t.byTag, t.post[ks.digestTag]))
	}
	return ops
}

// bucketOps emits each affected dueBucket leaf ONCE, over the composed membership delta. A bucket
// that a registration vacates and a sweep drains in the same block is one leaf with one final value,
// not two deletes of one key.
//
// The pre-state members come from whichever witness carries that bucket: the per-due-height bucket
// witnesses a registration travels with, or — for the bucket that comes due — the sweep's own member
// list, which the scope gate has already proven against prevStateRoot. Both are proofs of the same
// leaf against the same root, and the fold verifies whichever is used.
func (t *idSetTransition) bucketOps(w StateRootWitness) ([]statehash.FoldOp, error) {
	if len(t.buckets) == 0 {
		return nil, nil
	}
	wits := make(map[uint64]StateRootBucketWitness, len(w.BondRegBuckets)+1)
	for _, bw := range w.BondRegBuckets {
		wits[bw.DueHeight] = bw
	}
	if w.TTLSweep != nil {
		if _, ok := wits[w.TTLSweep.Height]; !ok {
			wits[w.TTLSweep.Height] = StateRootBucketWitness{
				DueHeight:      w.TTLSweep.Height,
				PreMembers:     w.TTLSweep.Members,
				Proof:          w.TTLSweep.BucketProof,
				DeleteSiblings: w.TTLSweep.BucketDeleteSiblings,
			}
		}
	}
	dues := make([]uint64, 0, len(t.buckets))
	for d := range t.buckets {
		dues = append(dues, d)
	}
	sort.Slice(dues, func(i, j int) bool { return dues[i] < dues[j] })

	ops := make([]statehash.FoldOp, 0, len(dues))
	for _, d := range dues {
		bw, ok := wits[d]
		if !ok || bw.Proof.IsNil() {
			return nil, fmt.Errorf("%w: no dueBucket witness for affected due-height %d", ErrRecomputeStateRootDigest, d)
		}
		pre := make(map[ports.NodeID]struct{}, len(bw.PreMembers))
		for _, id := range bw.PreMembers {
			pre[id] = struct{}{}
		}
		post := cloneIDSet(pre)
		mv := t.buckets[d]
		for id := range mv.deletes {
			delete(post, id)
		}
		for id := range mv.inserts {
			post[id] = struct{}{}
		}
		if idSetsEqual(pre, post) {
			continue // the classes moved this bucket's membership out and back in again
		}
		var oldValue, newValue []byte
		if len(pre) > 0 {
			oldValue = dueBucketMTHFromSet(pre) // present pre-state (a CHANGE or a DELETE)
		}
		if len(post) > 0 {
			newValue = dueBucketMTHFromSet(post)
		}
		ops = append(ops, statehash.FoldOp{
			Key:            dueBucketKey(d),
			OldValue:       oldValue,
			NewValue:       newValue,
			Proof:          bw.Proof,
			DeleteSiblings: bw.DeleteSiblings,
		})
	}
	return ops, nil
}

// netWrites folds the composed write list to ONE write per key — the LAST value in apply order,
// which is the value the block leaves committed — and drops a write that nets back to the pre-state.
//
// THE NET-ZERO DELETE IS A REAL BLOCK SHAPE, not a defensive branch. apply screens a registration
// against the PRE-state slashed set, so an id that registers a bond and is slashed in the same block
// has its bonded leaf written by class B and deleted by class S, and the committed leaf ends where it
// started: absent. Emitting the delete would ask the fold to remove a leaf the pre-state never held,
// which it refuses. Pre-state membership is read from the anchored pre-state sets, never from a
// witness scalar.
func (t *idSetTransition) netWrites() []stateRootWrite {
	final := make(map[string]stateRootWrite, len(t.writes))
	order := make([]string, 0, len(t.writes))
	for _, wr := range t.writes {
		k := string(wr.key)
		if _, seen := final[k]; !seen {
			order = append(order, k)
		}
		final[k] = wr
	}
	out := make([]stateRootWrite, 0, len(order))
	for _, k := range order {
		wr := final[k]
		if wr.newValue == nil && !t.presentPreState(wr.key) {
			continue // written and removed inside one block: the committed leaf never moved
		}
		out = append(out, wr)
	}
	return out
}

// presentPreState reports whether an id-keyed leaf of one of the composed keyspaces was present in
// the committed pre-state. A key outside those keyspaces reports present, because only those three
// can be written and removed inside one block.
func (t *idSetTransition) presentPreState(key []byte) bool {
	for _, ks := range idKeyspaces {
		id, ok := idFromTaggedKey(key, ks.leafTag)
		if !ok {
			continue
		}
		_, in := t.pre[ks.digestTag][id]
		return in
	}
	return true
}

// expiredAfterRenewals is the sweep's real expired set: the committed members of the bucket that
// comes due, less the ids whose registration in this same block reset their TTL clock. apply decides
// this by reading bondRegHeight AFTER the registration loop has written it; the box reads the same
// fact from the registrations it derived.
func expiredAfterRenewals(due []ports.NodeID, registered map[ports.NodeID]struct{}) []ports.NodeID {
	out := make([]ports.NodeID, 0, len(due))
	for _, id := range due {
		if _, renewed := registered[id]; renewed {
			continue
		}
		out = append(out, id)
	}
	return out
}

// mergeBucketMoves folds one class's due-bucket deltas into the running set.
func mergeBucketMoves(into map[uint64]bucketMove, from map[uint64]bucketMove) {
	for d, mv := range from {
		dst := touchBucketMove(into, d)
		for id := range mv.deletes {
			dst.deletes[id] = struct{}{}
		}
		for id := range mv.inserts {
			dst.inserts[id] = struct{}{}
		}
	}
}

// touchBucketMove returns the delta record for one due-height, creating it on first touch.
func touchBucketMove(m map[uint64]bucketMove, due uint64) bucketMove {
	mv, ok := m[due]
	if !ok {
		mv = bucketMove{inserts: map[ports.NodeID]struct{}{}, deletes: map[ports.NodeID]struct{}{}}
		m[due] = mv
	}
	return mv
}

// digestWitnessesByTag indexes the supplied whole-set digest witnesses by their tag.
func digestWitnessesByTag(wits []StateRootDigestWitness) map[string]*StateRootDigestWitness {
	byTag := make(map[string]*StateRootDigestWitness, len(wits))
	for i := range wits {
		byTag[wits[i].Tag] = &wits[i]
	}
	return byTag
}
