package node

import (
	"errors"
	"fmt"

	"github.com/fxamacker/cbor/v2"
	"github.com/nerolabs/silt/core/chain"
	"github.com/nerolabs/silt/core/statehash"
	"github.com/nerolabs/silt/ports"
)

// The ASKING half of the witness seam, and the one structural problem it has to solve.
//
// chain.WitnessSource is SYNCHRONOUS: the box calls Leaf and expects an answer before it
// continues. This node's transport is asynchronous callbacks on ONE scheduler thread with no
// goroutines and no locks (core/node). A source that blocked waiting for a reply would
// deadlock outright — the reply it is waiting for arrives on the very loop it is blocking.
//
// THE RESOLUTION IS DEMAND-DRIVEN REPLAY, and it falls out of a property the box already has.
// A source that has no answer returns ok=false, and the box STALLS — that is its designed
// behavior for an absent witness, not an error path invented here. So:
//
//  1. Run Validate against a local cache. Every read the cache cannot answer is RECORDED and
//     answered ok=false, so the pass stalls at the first miss but tells us what it wanted.
//  2. Fetch the recorded misses over the wire, into the cache.
//  3. Run Validate again. It reads further this time, and misses later.
//  4. Stop when a pass records no misses — that pass ran to a real verdict.
//
// Each pass is a fresh, deterministic run of the same composition over a strictly larger
// cache, so it terminates: a key fetched once never misses again, and a pass that adds nothing
// ends the loop. The cost is one round of round-trips per read-set LAYER, not per read.
//
// WHY NOT ASK THE SERVER TO BUNDLE IT. Because the server would then have to run the
// validation composition itself to discover which leaves the box is going to read — which is
// exactly the work the box is refusing to delegate. Replay keeps the read set the box's own
// decision, and leaves the server with no way to influence which questions get asked.

// witnessCacheKey identifies one accessor call for caching and for miss-recording.
type witnessCacheKey string

func leafKey(k []byte) witnessCacheKey    { return witnessCacheKey("L" + string(k)) }
func membersKey(t string) witnessCacheKey { return witnessCacheKey("M" + t) }
func ancestorsKey(k int) witnessCacheKey  { return witnessCacheKey(fmt.Sprintf("A%d", k)) }
func logExtKey(m int, leaves []ports.Hash) witnessCacheKey {
	b := make([]byte, 0, 8+len(leaves)*32)
	b = append(b, byte(m), byte(m>>8), byte(m>>16), byte(m>>24))
	for _, lf := range leaves {
		b = append(b, lf[:]...)
	}
	return witnessCacheKey("X" + string(b))
}

type leafAnswer struct {
	value []byte
	w     statehash.Witness
	ok    bool
}

type membersAnswer struct {
	ids []ports.NodeID
	ok  bool
}

type hashesAnswer struct {
	hashes []ports.Hash
	incl   [][]ports.Hash
	ok     bool
}

// witnessCache is the box's WitnessSource for one validation. It answers from what has been
// fetched so far and records what it could not answer, so the driver knows what to fetch next.
//
// It is per-validation and per-head. It is NOT a long-lived cache across heads: a provider is
// a snapshot, and answers from two heads mixed into one judgement is the exact seam the head
// field on the request exists to prevent.
type witnessCache struct {
	head    ports.Hash
	leaves  map[witnessCacheKey]leafAnswer
	members map[witnessCacheKey]membersAnswer
	hashes  map[witnessCacheKey]hashesAnswer
	// misses are the requests this pass wanted and could not answer, in the order the box
	// asked for them. Reset between passes.
	misses []witnessReq
	seen   map[witnessCacheKey]bool
}

func newWitnessCache(head ports.Hash) *witnessCache {
	return &witnessCache{
		head:    head,
		leaves:  map[witnessCacheKey]leafAnswer{},
		members: map[witnessCacheKey]membersAnswer{},
		hashes:  map[witnessCacheKey]hashesAnswer{},
		seen:    map[witnessCacheKey]bool{},
	}
}

// A cache IS a witness source. The compiler keeps that true if the seam gains a method.
var _ chain.WitnessSource = (*witnessCache)(nil)

// miss records one unanswerable read, once per pass per key.
func (c *witnessCache) miss(k witnessCacheKey, req witnessReq) {
	if c.seen[k] {
		return
	}
	c.seen[k] = true
	c.misses = append(c.misses, req)
}

// startPass clears the per-pass miss record. The fetched answers are kept — that is the
// progress that makes the replay terminate.
func (c *witnessCache) startPass() {
	c.misses = c.misses[:0]
	c.seen = map[witnessCacheKey]bool{}
}

func (c *witnessCache) Leaf(key []byte) ([]byte, statehash.Witness, bool) {
	k := leafKey(key)
	if a, hit := c.leaves[k]; hit {
		return a.value, a.w, a.ok
	}
	dup := make([]byte, len(key))
	copy(dup, key)
	c.miss(k, witnessReq{Call: witnessCallLeaf, Head: c.head, Key: dup})
	return nil, statehash.Witness{}, false
}

func (c *witnessCache) Members(digestTag string) ([]ports.NodeID, bool) {
	k := membersKey(digestTag)
	if a, hit := c.members[k]; hit {
		return a.ids, a.ok
	}
	c.miss(k, witnessReq{Call: witnessCallMembers, Head: c.head, Tag: digestTag})
	return nil, false
}

func (c *witnessCache) Ancestors(n int) ([]ports.Hash, bool) {
	k := ancestorsKey(n)
	if a, hit := c.hashes[k]; hit {
		return a.hashes, a.ok
	}
	c.miss(k, witnessReq{Call: witnessCallAncestors, Head: c.head, K: n})
	return nil, false
}

func (c *witnessCache) LogExtension(m int, leaves []ports.Hash) ([]ports.Hash, [][]ports.Hash, bool) {
	k := logExtKey(m, leaves)
	if a, hit := c.hashes[k]; hit {
		return a.hashes, a.incl, a.ok
	}
	dup := make([]ports.Hash, len(leaves))
	copy(dup, leaves)
	c.miss(k, witnessReq{Call: witnessCallLogExtension, Head: c.head, M: m, Leaves: dup})
	return nil, nil, false
}

// store files one fetched answer under the key the corresponding read will use.
func (c *witnessCache) store(req witnessReq, r witnessResp) error {
	switch req.Call {
	case witnessCallLeaf:
		w, err := witnessFromProof(r.Proof)
		if err != nil {
			return err
		}
		c.leaves[leafKey(req.Key)] = leafAnswer{value: r.Value, w: w, ok: r.OK}
	case witnessCallMembers:
		c.members[membersKey(req.Tag)] = membersAnswer{ids: r.IDs, ok: r.OK}
	case witnessCallAncestors:
		c.hashes[ancestorsKey(req.K)] = hashesAnswer{hashes: r.Hashes, ok: r.OK}
	case witnessCallLogExtension:
		c.hashes[logExtKey(req.M, req.Leaves)] = hashesAnswer{hashes: r.Hashes, incl: r.Incl, ok: r.OK}
	default:
		return fmt.Errorf("node: unknown witness call %d", req.Call)
	}
	return nil
}

// maxWitnessPasses bounds the replay. Each pass must add at least one cached answer or the
// loop ends on its own, so this is a belt against a pathological or hostile server that
// answers in a way that keeps producing fresh keys — not the normal stopping condition.
const maxWitnessPasses = 32

// ErrWitnessUnreachable is the floor box's verdict when it cannot obtain the witnesses a
// judgement needs. It is a STALL, and it is the correct outcome: a box that cannot see the
// evidence must not accept the block, and must not pretend the block is bad either. Named so
// an operator can tell it apart from a block that was actually judged and refused.
var ErrWitnessUnreachable = errors.New("node: floor box could not reach a witness provider — stall (no verdict, not a rejection)")

// ValidateWithWitnesses runs the floor box's door over witnesses pulled from `peer`, and calls
// done exactly once with the outcome.
//
// The verdict is the BOX's. This function moves bytes and nothing else: it never inspects the
// block, never overrides an outcome, and has no opinion about the peer. A peer that lies,
// stalls, or vanishes yields a stall, because everything it returns is checked by the box
// against roots the box already holds.
// mkBox is supplied by the caller rather than a constructed box being passed in, and the
// reason is load-bearing rather than stylistic: a Box reads through the source it was
// CONSTRUCTED with. Handing this function a finished box would leave it validating against
// whatever source that box already held, while the cache below filled up unread — the replay
// would never converge and the stall would look like a transport failure. The source has to
// reach the box through its constructor, so the constructor is the parameter.
func (n *Node) ValidateWithWitnesses(mkBox func(chain.WitnessSource) (*chain.Box, error),
	b chain.Block, w chain.StateRootWitness,
	peer ports.NodeID, head ports.Hash, done func(chain.FloorBoxOutcome, error)) {
	if mkBox == nil {
		done(chain.IndeterminateTrustlessly, errors.New("node: no floor box constructor"))
		return
	}
	cache := newWitnessCache(head)
	box, err := mkBox(cache)
	if err != nil {
		done(chain.IndeterminateTrustlessly, fmt.Errorf("node: floor box: %w", err))
		return
	}
	if box == nil {
		done(chain.IndeterminateTrustlessly, errors.New("node: no floor box"))
		return
	}
	n.witnessPass(box, b, w, peer, cache, 0, done)
}

// witnessPass runs one replay pass and either returns its verdict or fetches what the pass
// asked for and schedules the next one.
func (n *Node) witnessPass(box *chain.Box, b chain.Block, w chain.StateRootWitness,
	peer ports.NodeID, cache *witnessCache, pass int, done func(chain.FloorBoxOutcome, error)) {
	if pass >= maxWitnessPasses {
		done(chain.IndeterminateTrustlessly, fmt.Errorf("%w: the read set did not close in %d passes", ErrWitnessUnreachable, maxWitnessPasses))
		return
	}
	cache.startPass()
	out, err := box.Validate(b, w)
	if len(cache.misses) == 0 {
		// This pass answered every read from the cache, so it ran the whole composition to a
		// real verdict. That verdict is the box's, whatever it is.
		done(out, err)
		return
	}
	// The pass stalled on reads we can still go and get. Fetch them, then replay.
	pending := make([]witnessReq, len(cache.misses))
	copy(pending, cache.misses)
	n.fetchWitnesses(peer, cache, pending, func(fetchErr error) {
		if fetchErr != nil {
			done(chain.IndeterminateTrustlessly, fetchErr)
			return
		}
		n.witnessPass(box, b, w, peer, cache, pass+1, done)
	})
}

// fetchWitnesses pulls a batch of requests in sequence, filing each answer in the cache.
//
// SEQUENTIAL, not concurrent: this node has one loop, and a batch issued in parallel would
// interleave its callbacks with the rest of the node's work for no gain the box can use — the
// next pass cannot start until the whole batch has landed regardless.
//
// A refusal (OK=false) is FILED, not treated as a failure. "I have no witness for this" is a
// legitimate answer that stalls the read that wanted it; filing it stops the replay from
// asking again forever, and lets the box reach its own stall by its own route.
func (n *Node) fetchWitnesses(peer ports.NodeID, cache *witnessCache, reqs []witnessReq, done func(error)) {
	if len(reqs) == 0 {
		done(nil)
		return
	}
	req := reqs[0]
	rest := reqs[1:]
	raw, err := cbor.Marshal(req)
	if err != nil {
		done(fmt.Errorf("node: encode witness request: %w", err))
		return
	}
	n.request(peer, ports.Message{Kind: ports.MsgGetWitness, Data: raw}, func(resp ports.Message, rerr error) {
		if rerr != nil {
			done(fmt.Errorf("%w: %v", ErrWitnessUnreachable, rerr))
			return
		}
		r, derr := decodeWitnessResp(resp.Data, cache.head)
		if derr != nil {
			done(fmt.Errorf("%w: %v", ErrWitnessUnreachable, derr))
			return
		}
		if serr := cache.store(req, r); serr != nil {
			done(fmt.Errorf("%w: %v", ErrWitnessUnreachable, serr))
			return
		}
		n.fetchWitnesses(peer, cache, rest, done)
	})
}
