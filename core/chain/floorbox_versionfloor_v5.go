package chain

import "github.com/nerolabs/silt/core/statehash"

// The floor box's half of the version-floor advisory leaf (versionfloor.go): the leaf it reads and
// the write it derives. The write needs no pre-state: the new leaf is the block's own advisory and
// height, so the box derives it from the payload, and the read exists only for the sequence check
// in v5ValidateVersionFloor (validate_v5_predicates.go).

func versionFloorKey() []byte { return statehash.Key(tagVersionFloor, nil) }

// versionFloorReadKeys is the leaf the floor box reads for an advisory-carrying block: the
// advisory in force, which validity compares sequences against.
func versionFloorReadKeys(b Block) [][]byte {
	if b.Advisory == nil {
		return nil
	}
	return [][]byte{versionFloorKey()}
}

// versionFloorWrites derives the advisory leaf's write for block b: the block's advisory and its
// height. It writes nothing for a block without one, which is what apply does.
func versionFloorWrites(b Block) []stateRootWrite {
	if b.Advisory == nil {
		return nil
	}
	return []stateRootWrite{{key: versionFloorKey(), newValue: encodeVersionFloor(versionFloorState{Advisory: b.Advisory.Advisory, CommittedAt: b.Height})}}
}
