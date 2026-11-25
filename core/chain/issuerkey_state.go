package chain

import (
	"bytes"
	"encoding/binary"
	"sort"

	"github.com/nerolabs/silt/ports"
)

// The committed encodings of the issuer-key keyspace (see tagIssuerKey). Both are canonical —
// ascending, no duplicates — and their decoders refuse anything else, so one member set has
// exactly one encoding and a reordered or padded list can never stand in for the committed one.

const issuerKeyMemberBytes = 64 // issuer NodeID (32) || key fingerprint (32)

// encodeIssuerKeyBucket is one epoch bucket's committed value: its members sorted by issuer.
func encodeIssuerKeyBucket(byIssuer map[ports.NodeID]ports.Hash) []byte {
	ids := make([]ports.NodeID, 0, len(byIssuer))
	for id := range byIssuer {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return bytes.Compare(ids[i][:], ids[j][:]) < 0 })
	out := make([]byte, 0, len(ids)*issuerKeyMemberBytes)
	for _, id := range ids {
		fp := byIssuer[id]
		out = append(out, id[:]...)
		out = append(out, fp[:]...)
	}
	return out
}

// decodeIssuerKeyBucket parses a committed bucket value, refusing a non-canonical one.
func decodeIssuerKeyBucket(v []byte) (map[ports.NodeID]ports.Hash, bool) {
	if len(v) == 0 || len(v)%issuerKeyMemberBytes != 0 {
		return nil, false
	}
	out := make(map[ports.NodeID]ports.Hash, len(v)/issuerKeyMemberBytes)
	var prev []byte
	for off := 0; off < len(v); off += issuerKeyMemberBytes {
		idb := v[off : off+32]
		if prev != nil && bytes.Compare(prev, idb) >= 0 {
			return nil, false
		}
		prev = idb
		var id ports.NodeID
		var fp ports.Hash
		copy(id[:], idb)
		copy(fp[:], v[off+32:off+64])
		out[id] = fp
	}
	return out, true
}

// encodeIssuerKeyEpochs is the index scalar's value: the epochs present, ascending.
func encodeIssuerKeyEpochs(epochs []uint64) []byte {
	sorted := append([]uint64(nil), epochs...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	out := make([]byte, 0, 8*len(sorted))
	for _, e := range sorted {
		var b [8]byte
		binary.BigEndian.PutUint64(b[:], e)
		out = append(out, b[:]...)
	}
	return out
}

// decodeIssuerKeyEpochs parses the index scalar, refusing a non-canonical one. An absent index
// (nil) is the empty set.
func decodeIssuerKeyEpochs(v []byte) ([]uint64, bool) {
	if len(v)%8 != 0 {
		return nil, false
	}
	out := make([]uint64, 0, len(v)/8)
	for off := 0; off < len(v); off += 8 {
		e := binary.BigEndian.Uint64(v[off : off+8])
		if len(out) > 0 && out[len(out)-1] >= e {
			return nil, false
		}
		out = append(out, e)
	}
	return out, true
}

// issuerKeyEpochOutOfBand is pruneIssuerKeyCommit's retention rule, shared with the box so the
// two can never disagree about which buckets a registration-carrying block deletes.
func issuerKeyEpochOutOfBand(e, cur uint64) bool {
	return e > cur+issuerKeyPrePublish || (cur > issuerKeyPrePublish && e+issuerKeyPrePublish < cur)
}
