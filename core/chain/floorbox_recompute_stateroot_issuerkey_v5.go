package chain

import (
	"encoding/binary"
	"errors"
	"fmt"

	"github.com/nerolabs/silt/core/statehash"
	"github.com/nerolabs/silt/ports"
)

// era-4 (v5) trustless floor-box RECOMPUTE — CLASS I (demand-issuer key registrations).
//
// applyIssuerKeys adds each registration (epoch, issuer, fingerprint) to its epoch bucket unless
// that issuer already has one there (append-only), then prunes every epoch that has left the
// retention band — deleting whole buckets. The committed layout (see tagIssuerKey) makes every one
// of those writes a POINT the box can derive and prove: the index scalar commits which epochs
// exist, and each bucket leaf commits all of its members. So the box reads the index and the
// bucket of every epoch a registration names, re-runs the add and the prune over them, and writes
// exactly the leaves whose value moves: the touched buckets, the pruned buckets (deletes), and the
// index.
//
// THE READS ARE VERIFIED HERE, NOT LEFT TO THE FOLD. A re-registration leaves its bucket unchanged,
// so the bucket may never be folded, yet its value decides whether the registration adds anything.
// Each read is therefore resolved against prevStateRoot before it is used: a provider that lies
// about a bucket or the index produces a failed proof and a stall. A provider that omits one
// produces a stall too.

// ErrRecomputeIssuerKey is a class-I read the box could not establish against the pre-state root,
// or a committed issuer-key value that is not canonical. Never a disproof: a stall.
var ErrRecomputeIssuerKey = errors.New("chain: floor-box issuer-key recompute could not establish its pre-state")

func issuerKeyBucketKey(epoch uint64) []byte {
	var ek [8]byte
	binary.BigEndian.PutUint64(ek[:], epoch)
	return statehash.Key(tagIssuerKey, ek[:])
}

func issuerKeyIndexKey() []byte { return statehash.Key(tagIssuerKeyEpochs, nil) }

// issuerKeyReadKeys are the leaves class I reads for block b — the index and the bucket of every
// epoch a registration names. The witness provider serves exactly these whether or not they
// change, and the box asks for exactly these: one derivation, both sides.
func issuerKeyReadKeys(b Block) [][]byte {
	if len(b.IssuerKeys) == 0 {
		return nil
	}
	keys := [][]byte{issuerKeyIndexKey()}
	seen := map[uint64]bool{}
	for _, r := range b.IssuerKeys {
		if !seen[r.Epoch] {
			seen[r.Epoch] = true
			keys = append(keys, issuerKeyBucketKey(r.Epoch))
		}
	}
	return keys
}

// issuerKeyWrites derives class I's committed-leaf writes for block b over the witnessed pre-state.
func (c *Chain) issuerKeyWrites(prevStateRoot ports.Hash, b Block, witByKey map[string]*StateRootChangedLeafWitness) ([]stateRootWrite, error) {
	if len(b.IssuerKeys) == 0 {
		return nil, nil // apply writes nothing: no add, and the prune runs only beside one
	}
	read := func(key []byte) ([]byte, error) {
		wit, ok := witByKey[string(key)]
		if !ok {
			return nil, fmt.Errorf("%w: no witness for %x", ErrRecomputeIssuerKey, key)
		}
		res := statehash.Resolve(prevStateRoot, key, wit.OldValue, wit.Proof)
		switch {
		case res.IsProvenPresent():
			return wit.OldValue, nil
		case res.IsProvenAbsent():
			return nil, nil
		default:
			return nil, fmt.Errorf("%w: the witness for %x does not verify against the pre-state root", ErrRecomputeIssuerKey, key)
		}
	}

	idxRaw, err := read(issuerKeyIndexKey())
	if err != nil {
		return nil, err
	}
	preEpochs, ok := decodeIssuerKeyEpochs(idxRaw)
	if !ok {
		return nil, fmt.Errorf("%w: the committed index is not canonical", ErrRecomputeIssuerKey)
	}
	present := make(map[uint64]bool, len(preEpochs))
	for _, e := range preEpochs {
		present[e] = true
	}

	// The add: touched buckets, loaded from their verified pre-state, in registration order.
	buckets := map[uint64]map[ports.NodeID]ports.Hash{}
	preRaw := map[uint64][]byte{}
	for _, r := range b.IssuerKeys {
		bucket, loaded := buckets[r.Epoch]
		if !loaded {
			raw, rErr := read(issuerKeyBucketKey(r.Epoch))
			if rErr != nil {
				return nil, rErr
			}
			if present[r.Epoch] != (raw != nil) {
				return nil, fmt.Errorf("%w: the index and bucket %d disagree on whether it exists", ErrRecomputeIssuerKey, r.Epoch)
			}
			bucket = map[ports.NodeID]ports.Hash{}
			if raw != nil {
				if bucket, ok = decodeIssuerKeyBucket(raw); !ok {
					return nil, fmt.Errorf("%w: committed bucket %d is not canonical", ErrRecomputeIssuerKey, r.Epoch)
				}
			}
			buckets[r.Epoch], preRaw[r.Epoch] = bucket, raw
		}
		if _, exists := bucket[r.IssuerID()]; !exists {
			bucket[r.IssuerID()] = r.Fingerprint
		}
	}

	// The prune, over the post-add epoch set, by the same retention rule apply uses.
	// The block's epoch from the box's own config — blockEpoch's arithmetic, inlined so the
	// recompute reads nothing but own-cfg (the fold-file rule).
	cur := uint64(0)
	if c.cfg.EpochBlocks > 0 {
		cur = b.Height / c.cfg.EpochBlocks
	}
	post := map[uint64]bool{}
	for e := range present {
		post[e] = true
	}
	for e := range buckets {
		post[e] = true
	}
	var writes []stateRootWrite
	for e := range post {
		if !issuerKeyEpochOutOfBand(e, cur) {
			continue
		}
		delete(post, e)
		if _, touched := buckets[e]; touched {
			delete(buckets, e) // added and pruned in one block: in-band by validity, so unreachable
		}
		if present[e] {
			writes = append(writes, stateRootWrite{key: issuerKeyBucketKey(e), newValue: nil})
		}
	}
	for e, bucket := range buckets {
		if v := encodeIssuerKeyBucket(bucket); string(v) != string(preRaw[e]) {
			writes = append(writes, stateRootWrite{key: issuerKeyBucketKey(e), newValue: v})
		}
	}
	postEpochs := make([]uint64, 0, len(post))
	for e := range post {
		postEpochs = append(postEpochs, e)
	}
	var newIdx []byte
	if len(postEpochs) > 0 {
		newIdx = encodeIssuerKeyEpochs(postEpochs)
	}
	if string(newIdx) != string(idxRaw) {
		writes = append(writes, stateRootWrite{key: issuerKeyIndexKey(), newValue: newIdx})
	}
	return writes, nil
}
