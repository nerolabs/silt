package statehash

import (
	"bytes"
	"encoding/binary"
	"math"
	"testing"

	"github.com/nerolabs/silt/ports"
)

// The encoder unit tests pin the canonical byte encoding. Widths and endianness
// are CONSENSUS PARAMETERS (research cert Q6 flag 2): a change here changes every
// root and is a hard fork. These tests are the tripwire that a width/endianness
// edit is a deliberate era bump, not a silent formatting change.

func TestEncodeInt64IsEightByteBigEndianTwosComplement(t *testing.T) {
	cases := []int64{0, 1, -1, math.MaxInt64, math.MinInt64, 1 << 21}
	for _, v := range cases {
		got := EncodeInt64(v)
		if len(got) != 8 {
			t.Fatalf("EncodeInt64(%d) width = %d, want 8", v, len(got))
		}
		var want [8]byte
		binary.BigEndian.PutUint64(want[:], uint64(v))
		if !bytes.Equal(got, want[:]) {
			t.Fatalf("EncodeInt64(%d) = %x, want %x", v, got, want)
		}
	}
	// The encoding is TOTAL and INJECTIVE over the int64 domain: distinct values
	// give distinct leaves, so the root faithfully images the summed weight.
	if bytes.Equal(EncodeInt64(1), EncodeInt64(-1)) {
		t.Fatal("EncodeInt64 is not injective: 1 and -1 collide")
	}
	if bytes.Equal(EncodeInt64(0), EncodeInt64(math.MinInt64)) {
		t.Fatal("EncodeInt64 is not injective: 0 and MinInt64 collide")
	}
}

func TestEncodeUint64IsEightByteBigEndian(t *testing.T) {
	for _, v := range []uint64{0, 1, math.MaxUint64, 42} {
		got := EncodeUint64(v)
		if len(got) != 8 {
			t.Fatalf("EncodeUint64(%d) width = %d, want 8", v, len(got))
		}
		var want [8]byte
		binary.BigEndian.PutUint64(want[:], v)
		if !bytes.Equal(got, want[:]) {
			t.Fatalf("EncodeUint64(%d) = %x, want %x", v, got, want)
		}
	}
}

func TestEncodeUint8IsOneByte(t *testing.T) {
	for _, v := range []uint8{0, 1, 3, 255} {
		got := EncodeUint8(v)
		if len(got) != 1 || got[0] != v {
			t.Fatalf("EncodeUint8(%d) = %x, want single byte %02x", v, got, v)
		}
	}
}

func TestEncodeBoolIsOneByte(t *testing.T) {
	if got := EncodeBool(false); len(got) != 1 || got[0] != 0 {
		t.Fatalf("EncodeBool(false) = %x, want 0x00", got)
	}
	if got := EncodeBool(true); len(got) != 1 || got[0] != 1 {
		t.Fatalf("EncodeBool(true) = %x, want 0x01", got)
	}
	// present-value-0 (encoded false) and the Present marker are DISTINCT, so a
	// Class-A membership marker can never be confused with a Class-B false value.
	if bytes.Equal(EncodeBool(false), Present) {
		t.Fatal("EncodeBool(false) collides with the Present membership marker")
	}
}

func TestEncodeIDIsRawThirtyTwoBytes(t *testing.T) {
	id := ports.NodeID{9, 8, 7}
	got := EncodeID(id)
	if len(got) != 32 {
		t.Fatalf("EncodeID width = %d, want 32", len(got))
	}
	if !bytes.Equal(got, id[:]) {
		t.Fatalf("EncodeID = %x, want raw %x", got, id[:])
	}
	// Independent copy: mutating the returned slice must not corrupt the id.
	got[0] = 0xFF
	if id[0] != 9 {
		t.Fatal("EncodeID returned an aliasing slice into the id")
	}
}

func TestKeyIsFieldTagPrefixInjective(t *testing.T) {
	raw := []byte("shared-raw-key")
	kA := Key("bonded\x00", raw)
	kB := Key("epochSet\x00", raw)
	if bytes.Equal(kA, kB) {
		t.Fatal("the same raw key under two tags produced the same leaf key — the " +
			"field-tag keyspace separation is broken")
	}
	// A scalar reserved key (empty raw) cannot equal a map key under the same tag:
	// map raw keys are non-empty.
	scalar := Key("gateHeight\x00", nil)
	if bytes.Equal(scalar, Key("gateHeight\x00", []byte{0})) {
		t.Fatal("scalar reserved key collided with a one-byte-keyed entry")
	}
}

// idBytes / hashBytes return a fresh []byte for a fixed-array key (the arrays are
// not addressable as composite literals, so [:] cannot be taken inline).
func idBytes(id ports.NodeID) []byte { return EncodeID(id) }
func hashBytes(h ports.Hash) []byte  { b := make([]byte, 32); copy(b, h[:]); return b }

// TestRootIsInsertionOrderIndependent is the core determinism property (cert R2):
// the root is a function of the leaf SET, not the order the leaves are supplied in.
func TestRootIsInsertionOrderIndependent(t *testing.T) {
	forward := []Leaf{
		{Key: Key("bonded\x00", idBytes(ports.NodeID{1})), Value: EncodeInt64(100)},
		{Key: Key("bonded\x00", idBytes(ports.NodeID{2})), Value: EncodeInt64(200)},
		{Key: Key("revoked\x00", hashBytes(ports.Hash{3})), Value: Present},
		{Key: Key("gateHeight\x00", nil), Value: EncodeUint64(7)},
	}
	reversed := make([]Leaf, len(forward))
	for i := range forward {
		reversed[i] = forward[len(forward)-1-i]
	}

	rf, err := Root(forward)
	if err != nil {
		t.Fatalf("Root(forward): %v", err)
	}
	rr, err := Root(reversed)
	if err != nil {
		t.Fatalf("Root(reversed): %v", err)
	}
	if rf != rr {
		t.Fatalf("root depends on insertion order: forward %x != reversed %x", rf, rr)
	}
}

// TestRootChangesOnWrongValue is the ablation half: a wrong VALUE at a present key
// produces a different root, so a value-encoding defect surfaces as a root mismatch.
// This is what makes the value (not just presence) load-bearing in the root.
func TestRootChangesOnWrongValue(t *testing.T) {
	base := []Leaf{
		{Key: Key("bonded\x00", idBytes(ports.NodeID{1})), Value: EncodeInt64(100)},
	}
	perturbed := []Leaf{
		{Key: Key("bonded\x00", idBytes(ports.NodeID{1})), Value: EncodeInt64(101)},
	}
	rb, _ := Root(base)
	rp, _ := Root(perturbed)
	if rb == rp {
		t.Fatal("perturbing a bonded weight from 100 to 101 did NOT change the root — " +
			"the value is not committed, so a wrong-value witness could forge the state")
	}
}

func TestRootRejectsDuplicateKey(t *testing.T) {
	dup := Key("bonded\x00", idBytes(ports.NodeID{1}))
	_, err := Root([]Leaf{
		{Key: dup, Value: EncodeInt64(1)},
		{Key: dup, Value: EncodeInt64(2)},
	})
	if err == nil {
		t.Fatal("Root accepted a duplicate leaf key — the last write would silently win")
	}
	if _, ok := err.(*DuplicateKeyError); !ok {
		t.Fatalf("want *DuplicateKeyError, got %T: %v", err, err)
	}
}

// TestEmptyRootIsAFixedConstant pins that the empty committedSet commits a
// DEFINITE, reproducible root (cert freeze condition 4: empty-vs-absent closed).
func TestEmptyRootIsAFixedConstant(t *testing.T) {
	r1, err := Root(nil)
	if err != nil {
		t.Fatalf("Root(nil): %v", err)
	}
	r2, _ := Root([]Leaf{})
	if r1 != r2 {
		t.Fatalf("empty root is not stable: %x != %x", r1, r2)
	}
	// A non-empty set must differ from the empty root.
	nonEmpty, _ := Root([]Leaf{{Key: Key("revoked\x00", hashBytes(ports.Hash{1})), Value: Present}})
	if nonEmpty == r1 {
		t.Fatal("a non-empty state produced the empty-tree root")
	}
}
