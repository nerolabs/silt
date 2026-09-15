package node

import (
	"errors"
	"fmt"

	"github.com/fxamacker/cbor/v2"
	"github.com/nerolabs/silt/core/chain"
	"github.com/nerolabs/silt/core/statehash"
	"github.com/nerolabs/silt/ports"
)

// The witness seam on the wire.
//
// A floor box holds no tree. To judge a block it needs committed leaves and their proofs,
// whole-set member lists, a bounded ancestor window, and transparency-log extension proofs —
// all of which a full node has and it does not. This file is the request/response shape that
// carries them, and nothing else: no policy, no trust, no state.
//
// WHY THE ASKER NEEDS NO TRUST IN THE ANSWERER. Every field below is checked by the box
// against a root it already holds — point leaves by statehash.Resolve against the committed
// StateRoot, whole sets by the RFC-6962 MTH over the claimed id-list against the committed
// digest root, ancestors by Prev-linkage, extensions by the consistency proof. A server that
// lies, omits, or injects produces a STALL, never an acceptance. That is what lets a box point
// at any reachable node, including a hostile one, and why this transport carries no
// authentication of its own beyond what the transport already provides.
//
// WHY IT IS CHATTY, DELIBERATELY. One request is one accessor call, not a precomputed bundle.
// A bundle would require the server to run the validation composition itself to discover which
// leaves the box will read — which is the very work the box is not delegating. Paying round
// trips keeps the read set the BOX's decision. The box's own byte budget bounds the total, and
// a client-side per-head cache (witnessClient) keeps a repeated key from crossing the wire
// twice.

// witnessCall discriminates the four accessors of the witness seam. Appended, never
// renumbered: an old box's ancestors request must never be read as a leaf request.
type witnessCall uint8

const (
	witnessCallLeaf         witnessCall = 1
	witnessCallMembers      witnessCall = 2
	witnessCallAncestors    witnessCall = 3
	witnessCallLogExtension witnessCall = 4
	// witnessCallBundle is the fifth, and it is not an accessor. The other four answer one
	// question the composition asked; this one carries the O(payload) pre-state bundle the
	// state-root recompute FOLDS, which cannot be pulled key by key because the fold needs every
	// changed leaf's proof in hand before it can compute a post-root at all. It is untrusted like
	// the rest: the box derives the changed-key set itself and verifies every proof against
	// prevStateRoot, so a short or forged bundle stalls.
	witnessCallBundle witnessCall = 5
)

// witnessReq is one accessor call against a named head.
//
// Head is load-bearing and not a courtesy. A provider is a SNAPSHOT of one committed state; a
// box validating the block after h needs the leaves as of h. If the server answered from
// whatever head it happened to hold, a box would read some leaves from one state and some from
// another, and the composition would stall on the seam with nothing to name. So the box says
// which head it is asking about and a server whose head has moved refuses rather than guesses.
type witnessReq struct {
	Call   witnessCall  `cbor:"1,keyasint"`
	Head   ports.Hash   `cbor:"2,keyasint"`
	Key    []byte       `cbor:"3,keyasint"` // leaf
	Tag    string       `cbor:"4,keyasint"` // members
	K      int          `cbor:"5,keyasint"` // ancestors
	M      int          `cbor:"6,keyasint"` // log extension: the parent log size to prove from
	Leaves []ports.Hash `cbor:"7,keyasint"` // log extension: the leaves the block appends
	// Block is the encoded candidate block a bundle is requested for. The server reads it for its
	// PAYLOAD only — which leaves it would change, and which ids its carrier names — and believes
	// nothing it claims: every value in the answer is the server's own committed pre-state.
	Block []byte `cbor:"8,keyasint"`
}

// witnessResp is the answer. OK=false is a REFUSAL TO ANSWER — "I have no witness for this" —
// and is not the same as a proven absence: absence is itself a proof (an exclusion proof) and
// arrives with OK=true and a witness. Conflating the two would let an unreachable or
// uncooperative server masquerade as proof that a leaf does not exist, which is precisely the
// forgery the box exists to refuse.
type witnessResp struct {
	OK     bool           `cbor:"1,keyasint"`
	Value  []byte         `cbor:"2,keyasint"` // leaf value
	Proof  []byte         `cbor:"3,keyasint"` // leaf proof, statehash.Witness.MarshalBinary
	IDs    []ports.NodeID `cbor:"4,keyasint"` // members
	Hashes []ports.Hash   `cbor:"5,keyasint"` // ancestors, or the log-extension consistency proof
	Incl   [][]ports.Hash `cbor:"6,keyasint"` // log extension: one inclusion proof per appended leaf
	// Head is the head the SERVER answered from. A box compares it to what it asked for and
	// discards a mismatch, so a server that advanced mid-conversation cannot silently mix two
	// states into one judgement.
	Head ports.Hash `cbor:"7,keyasint"`
	// Bundle is the encoded StateRootWitness (witnessCallBundle only).
	Bundle []byte `cbor:"8,keyasint"`
	// NextHeight is the height the server's chain would commit next, so a box that asked about
	// no head in particular learns not only WHICH head it was answered from but WHERE that head
	// sits — which is what it needs to ask a source for the block above it.
	NextHeight uint64 `cbor:"9,keyasint"`
}

// witnessReqMaxBytes bounds a decoded request. A request is attacker-chosen and decoding
// allocates, so the server refuses an oversized one before it reads it. The dominant field is
// the appended-leaf list on a log-extension call; a block that appended more leaves than this
// would exceed the box's own frame budget long before it got here.
const witnessReqMaxBytes = 1 << 20

// witnessBlockMaxBytes bounds a BUNDLE request, which carries a whole candidate block and is
// therefore bounded by what a block can be rather than by what an accessor call can be. A
// validator's bond registration alone runs to megabytes, so the accessor ceiling above would
// refuse the most ordinary block a live network produces. A block past this has already failed the
// proposer-side byte budgets that keep a block gatherable over a real WAN, so nothing honest is
// refused here.
const witnessBlockMaxBytes = 16 << 20

// witnessProofMaxBytes bounds ONE decoded leaf proof at the client. A sparse-Merkle proof over
// a tree of any realistic size is a few KiB; the ceiling is generous enough never to refuse an
// honest proof and small enough that a server cannot hand a 2 GiB floor box its memory ceiling
// one reply at a time.
const witnessProofMaxBytes = 1 << 20

// witnessBundleMaxBytes bounds ONE decoded state-root bundle at the client. A bundle is
// O(payload write-set x log N) plus the whole-set digest pre-images, so it is the largest single
// reply the seam carries; the ceiling is well above what an honest block needs and below what a
// hostile server could use to spend a 2 GiB floor box's memory in one frame. The box's own byte
// budget is the binding limit on the honest path — this is the belt that stops a decode the
// budget never gets to see.
const witnessBundleMaxBytes = 16 << 20

// errWitnessHeadMoved is a server's refusal to answer about a head it no longer holds. Named,
// because the box's correct response is to re-anchor on the new head and ask again — not to
// treat it as a lie and not to retry blindly.
var errWitnessHeadMoved = errors.New("node: witness request names a head this server no longer holds")

// serveWitness answers one witness request from this node's own committed chain. It is the
// serving side and it has no discretion: it either produces what the provider holds, or it
// answers OK=false. It never judges the asker and never varies its answer by who asked.
func (n *Node) serveWitness(msg ports.Message) ports.Message {
	deny := func() ports.Message {
		raw, _ := cbor.Marshal(witnessResp{OK: false})
		return ports.Message{Kind: ports.MsgWitnessReply, OK: false, Data: raw}
	}
	if len(msg.Data) > witnessBlockMaxBytes {
		return deny()
	}
	var req witnessReq
	if cbor.Unmarshal(msg.Data, &req) != nil {
		return deny()
	}
	// The accessor calls keep the tight ceiling; only the bundle call is allowed to be
	// block-sized, and only because it carries a block.
	if req.Call != witnessCallBundle && len(msg.Data) > witnessReqMaxBytes {
		return deny()
	}
	if n.chain == nil {
		return deny()
	}
	// The cache is asked for the head the REQUEST names, not for whatever head this node is on.
	// A box names one head for the whole of one validation, and this node keeps committing under
	// it; serving from the current head instead would mix two states into one judgement, and
	// refusing outright would make a box on a live chain re-anchor forever.
	p, ok := n.witnessProviders.For(n.chain, req.Head)
	if !ok {
		return deny()
	}
	head, next := p.Head()
	// Belt: a head this node has never held, or has held and dropped, is refused rather than
	// answered from another one. See witnessReq.Head.
	if req.Head != (ports.Hash{}) && req.Head != head {
		return deny()
	}

	resp := witnessResp{OK: true, Head: head, NextHeight: next}
	switch req.Call {
	case witnessCallLeaf:
		v, w, got := p.Leaf(req.Key)
		if !got {
			return deny()
		}
		pf, err := w.MarshalBinary()
		if err != nil {
			return deny()
		}
		resp.Value, resp.Proof = v, pf
	case witnessCallMembers:
		ids, got := p.Members(req.Tag)
		if !got {
			return deny()
		}
		resp.IDs = ids
	case witnessCallAncestors:
		hs, got := p.Ancestors(req.K)
		if !got {
			return deny()
		}
		resp.Hashes = hs
	case witnessCallLogExtension:
		cons, incl, got := p.LogExtension(req.M, req.Leaves)
		if !got {
			return deny()
		}
		resp.Hashes, resp.Incl = cons, incl
	case witnessCallBundle:
		blk, err := chain.Decode(req.Block)
		if err != nil {
			return deny()
		}
		w, bErr := p.Bundle(*blk)
		if bErr != nil {
			// A provider that cannot build this block's bundle says so. It is not a lie and not
			// a proof of anything; the asker walks on to the next provider.
			return deny()
		}
		raw, eErr := chain.EncodeStateRootWitness(w)
		if eErr != nil {
			return deny()
		}
		resp.Bundle = raw
	default:
		return deny()
	}
	raw, err := cbor.Marshal(resp)
	if err != nil {
		return deny()
	}
	return ports.Message{Kind: ports.MsgWitnessReply, OK: true, Data: raw}
}

// decodeWitnessResp turns a reply into the accessor's return shape, refusing an oversized
// proof before decoding it.
func decodeWitnessResp(data []byte, wantHead ports.Hash) (witnessResp, error) {
	var r witnessResp
	if err := cbor.Unmarshal(data, &r); err != nil {
		return r, fmt.Errorf("node: decode witness reply: %w", err)
	}
	if !r.OK {
		return r, nil
	}
	if wantHead != (ports.Hash{}) && r.Head != wantHead {
		return r, errWitnessHeadMoved
	}
	return r, nil
}

// witnessFromProof decodes a leaf proof under the client's ceiling.
func witnessFromProof(proof []byte) (statehash.Witness, error) {
	return statehash.UnmarshalWitness(proof, witnessProofMaxBytes)
}

// compile-time: the serving side answers exactly the seam the box reads through.
var _ chain.WitnessSource = (*chain.WitnessProvider)(nil)
