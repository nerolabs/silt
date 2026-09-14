package chain

import (
	"github.com/nerolabs/silt/core/statehash"
	"github.com/nerolabs/silt/core/translog"
	"github.com/nerolabs/silt/ports"
)

// WitnessProvider is the SERVING half of the witness seam: a full node's answer to a box that
// holds no tree. It reads one chain's committed v5 leaf set and produces, for any key the box
// asks about, the value and the inclusion/exclusion proof that binds it to the committed
// StateRoot — plus the whole-set member lists, the bounded ancestor window, and the
// transparency-log extension proofs the composition needs.
//
// IT IS NOT A TRUSTED ROLE, and that is the design rather than a caveat on it. Everything a
// provider returns is checked by the reader against a root the reader already holds: point
// leaves by statehash.Resolve, whole sets by the RFC-6962 MTH over the claimed id-list
// against the committed digest root, ancestors by Prev-linkage, extensions by the consistency
// proof. A provider that lies produces a STALL, never an acceptance. So a floor box may point
// at any node at all — including a hostile one — without extending it trust, and needs no
// relationship with that node beyond reachability.
//
// It is the production counterpart of the prover-backed source the box's own gates already
// run against, over a live chain instead of a fixture. Serving honestly is the whole job; a
// provider has no policy and no discretion.
//
// SNAPSHOT, NOT A LIVE READ. A provider is built over the leaf set as it stood at one head and
// does not follow the chain forward. A box validating the block after h needs the leaves as of
// h, so a provider that silently advanced under it would answer some reads from one state and
// some from another, and the composition would stall on the seam with no way to say why.
// Rebuild per head; that is what Head reports and what a serving node keys its cache on.
//
// NOT SAFE FOR CONCURRENT CONSTRUCTION with chain mutation. A node runs on one scheduler
// thread with no locks (core/node), and this type inherits that: build it on the loop, from
// the same callback that would otherwise read the chain.
type WitnessProvider struct {
	head    ports.Hash
	next    uint64
	prover  *statehash.Prover
	leaves  map[string][]byte
	members map[string][]ports.NodeID
	chain   []ports.Hash
	// log is the chain's transparency log — the tree extension proofs start from. Extensions
	// are served off a CLONE, never this one: a provider answers questions, it does not
	// advance the node's own log to do so.
	log *translog.Log
}

// A provider IS the serving half of the witness seam, and the compiler is what keeps that
// true: if the seam gains a method, this assertion fails here rather than at the one call
// site that wires a real node to a real box.
var _ WitnessSource = (*WitnessProvider)(nil)

// ancestorWindow is the number of parent-linked block hashes a provider retains. The
// bond-registration nonce rule walks a bounded header window; retaining more than the rule can
// ask for is dead weight on the serving node, and retaining fewer stalls an honest box.
const ancestorWindow = 64

// NewWitnessProvider builds a provider over c's committed v5 leaf set at its current head. It
// returns false if the chain cannot produce a prover over its own leaves — a node with nothing
// to serve says so, rather than serving an empty tree that would answer every read with a
// confident absence.
func NewWitnessProvider(c *Chain) (*WitnessProvider, bool) {
	if c == nil {
		return nil, false
	}
	leafSet := c.stateRootLeavesV5()
	pr, err := statehash.NewProver(leafSet)
	if err != nil {
		return nil, false
	}
	head, next := c.Head()
	p := &WitnessProvider{
		head:    head,
		next:    next,
		prover:  pr,
		leaves:  make(map[string][]byte, len(leafSet)),
		members: make(map[string][]ports.NodeID, 5),
		log:     c.revLog,
	}
	for _, l := range leafSet {
		p.leaves[string(l.Key)] = l.Value
	}
	// The five whole-set keyspaces the composition folds. Each is served as the COMPLETE
	// claimed id-list; the reader checks completeness itself by re-deriving the MTH and
	// comparing it against the committed digest root, so one omitted or injected id stalls.
	p.members[tagEpochSetRoot] = sortIDs(weightedIDs(c.epochSet))
	p.members[tagQualifiedRoot] = sortIDs(weightedIDs(c.qualified))
	p.members[tagBondedRoot] = sortIDs(weightedIDs(c.bonded))
	p.members[tagValidatorsSeenRoot] = sortIDs(flaggedIDs(c.validatorsSeen))
	p.members[tagSlashedRoot] = sortIDs(flaggedIDs(c.slashed))

	cur := head
	for i := 0; i < ancestorWindow; i++ {
		p.chain = append(p.chain, cur)
		blk, ok := c.blockByHash(cur)
		if !ok || blk.Height == 0 {
			break
		}
		cur = blk.Prev
	}
	return p, true
}

// Head reports the head hash this provider was built over, and the height the chain would
// commit next. A serving node keys its provider cache on the hash, and a box that asked about
// a different head is told so rather than answered from the wrong state.
func (p *WitnessProvider) Head() (ports.Hash, uint64) { return p.head, p.next }

// Leaf serves one committed leaf and its proof. ok == false means "I have no witness for this
// key", which stalls the read that wanted it — it does NOT mean the leaf is absent. Absence is
// itself a proven fact (an exclusion proof), and conflating the two would let an unreachable
// provider masquerade as a proof of non-membership.
func (p *WitnessProvider) Leaf(key []byte) ([]byte, statehash.Witness, bool) {
	w, err := p.prover.Prove(key)
	if err != nil {
		return nil, statehash.Witness{}, false
	}
	return p.leaves[string(key)], w, true
}

// Members serves the complete claimed member id-list of a whole-set keyspace. A keyspace this
// provider has no entry for is an EMPTY complete list rather than a refusal: an empty keyspace
// genuinely has an empty member set, its committed digest root is the empty MTH, and the reader
// checks that exactly as it checks a populated one.
func (p *WitnessProvider) Members(digestTag string) ([]ports.NodeID, bool) {
	ids, ok := p.members[digestTag]
	if !ok {
		return nil, true
	}
	out := make([]ports.NodeID, len(ids))
	copy(out, ids)
	return out, true
}

// Ancestors serves up to k parent-linked block hashes ending at this provider's head, most
// recent first. Self-authenticating by Prev-linkage, so a short or forged window is caught by
// the reader walking the links rather than by trusting the count.
func (p *WitnessProvider) Ancestors(k int) ([]ports.Hash, bool) {
	if len(p.chain) == 0 || k <= 0 {
		return nil, false
	}
	if k > len(p.chain) {
		k = len(p.chain)
	}
	out := make([]ports.Hash, k)
	copy(out, p.chain[:k])
	return out, true
}

// LogExtension serves the RFC-6962 proofs that a block's committed log root is the parent's
// transparency log extended by exactly the supplied leaves: one consistency proof from m, plus
// one inclusion proof per appended leaf.
//
// The append happens on a CLONE, which is what keeps the provider side-effect free — two
// callers asking about two different candidate blocks must not see each other's leaves, and
// neither may advance the node's own log.
func (p *WitnessProvider) LogExtension(m int, leaves []ports.Hash) ([]ports.Hash, [][]ports.Hash, bool) {
	if p.log == nil || m < 0 {
		return nil, nil, false
	}
	ext := p.log.Clone()
	for _, lf := range leaves {
		ext.Append(lf)
	}
	n := ext.Size()
	cons, err := ext.ConsistencyProof(m, n)
	if err != nil {
		return nil, nil, false
	}
	incl := make([][]ports.Hash, 0, len(leaves))
	for j := range leaves {
		pf, iErr := ext.InclusionProof(m+j, n)
		if iErr != nil {
			return nil, nil, false
		}
		incl = append(incl, pf)
	}
	return cons, incl, true
}

// weightedIDs and flaggedIDs project the chain's two member-map shapes onto plain id lists.
// The bonded/qualified/epoch maps carry a weight per member and every key is a member; the
// seen/slashed maps carry a flag, and only the true entries are members.
func weightedIDs(m map[ports.NodeID]int64) []ports.NodeID {
	out := make([]ports.NodeID, 0, len(m))
	for id := range m {
		out = append(out, id)
	}
	return out
}

func flaggedIDs(m map[ports.NodeID]bool) []ports.NodeID {
	out := make([]ports.NodeID, 0, len(m))
	for id, v := range m {
		if v {
			out = append(out, id)
		}
	}
	return out
}

// ProviderCache holds ONE provider for a serving node, so a run of witness requests about the
// same head pays for the leaf set and the prover once. Single-entry deliberately: a box
// validates forward, so the useful working set is the current head, and a map keyed by a
// peer-supplied hash would let a stranger make the node build an unbounded number of provers.
type ProviderCache struct {
	p *WitnessProvider
}

// For returns a provider over c's current head, rebuilding only when the head has moved.
func (pc *ProviderCache) For(c *Chain) (*WitnessProvider, bool) {
	if c == nil {
		return nil, false
	}
	head, _ := c.Head()
	if pc.p != nil && pc.p.head == head {
		return pc.p, true
	}
	p, ok := NewWitnessProvider(c)
	if !ok {
		return nil, false
	}
	pc.p = p
	return p, true
}
