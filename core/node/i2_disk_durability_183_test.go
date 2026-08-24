package node

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/nerolabs/silt/adapters/identity"
	"github.com/nerolabs/silt/adapters/markstore"
	"github.com/nerolabs/silt/core/chain"
	"github.com/nerolabs/silt/ports"
)

// #183 red-team coverage caveat C-2: I2's fsync-before-the-wire durability was
// verified by inspection, and every restart test used markstore.NewMem — so no
// test drove the never-sign-twice mark against the REAL Disk store the daemon
// uses. These close that at the NODE layer: the watermark refusal must survive
// a genuine process restart backed by disk, and recordSign must WITHHOLD the
// signature when the store cannot make the mark durable (the fail-safe that
// keeps an honest validator from a permanent self-slash).

// TestI2DiskDurability_RefuseAcrossDiskRestart: a validator signs (h1,r0,prepare)
// with a real Disk-backed mark, then a FRESH node opens a new Disk at the SAME
// path (the restart) and must REFUSE a same-slot competitor over a different
// hash — the #397 guarantee, proven against disk, not memory.
func TestI2DiskDurability_RefuseAcrossDiskRestart(t *testing.T) {
	id := identity.FromSeed(9610)
	g := &chain.Block{Version: 1, Height: 0, Entries: []ports.Entry{mkEntry("i2-disk-g")}}
	chain.Sign(g, id.Signer())
	cfg := chain.Config{Quorum: 1, MinBond: 1 << 20, ByzantineQuorum: true, MatureValidators: 99}

	path := filepath.Join(t.TempDir(), "signmark.json")
	signedHash := ports.HashBytes([]byte("signed-block"))
	otherHash := ports.HashBytes([]byte("competitor-block"))

	// Sign (h1, r0, prepare) with a real Disk store — the mark is fsync'd.
	signer := i2Reload(t, id, g, cfg, markstore.New(path))
	if !signer.recordSign(1, 0, chain.PhasePrepare, signedHash) {
		t.Fatal("recordSign against the real Disk store failed")
	}

	// RESTART: a fresh node, a NEW Disk at the same path — as a rebooted daemon
	// would reopen it. The mark must reload and refuse the same-slot competitor.
	restarted := i2Reload(t, id, g, cfg, markstore.New(path))
	if !restarted.signMarkSet {
		t.Fatal("I2 durability VIOLATION: the mark did not reload from disk after restart")
	}
	if restarted.signAllowedAt(1, 0, chain.PhasePrepare, otherHash) {
		t.Fatal("I2 durability VIOLATION: a disk-restarted validator allowed a competitor at the SAME (h,r,phase) slot it signed before the crash — the persisted mark did not survive the round trip")
	}
	// A strictly-higher slot is still signable (liveness escape survives too).
	if !restarted.signAllowedAt(1, 1, chain.PhasePrepare, otherHash) {
		t.Fatal("I2 durability VIOLATION: a disk-restarted validator refused a strictly-HIGHER round — a crash must not re-wedge the height (#432)")
	}
}

// failOnSave wraps a store so Save always errors — modeling a full/broken disk.
type failOnSave struct{ ports.SignMarkStore }

func (failOnSave) Save(ports.SignMark) error { return errors.New("disk full") }

// TestI2DiskDurability_WithholdsSignatureOnSaveFailure: the mark-then-sign
// fail-safe — if the store cannot persist the mark, recordSign returns false
// and the caller must NOT release the signature. A signature without a durable
// mark is exactly the honest-self-slash-after-crash this ordering prevents.
func TestI2DiskDurability_WithholdsSignatureOnSaveFailure(t *testing.T) {
	id := identity.FromSeed(9611)
	g := &chain.Block{Version: 1, Height: 0, Entries: []ports.Entry{mkEntry("i2-disk-fail-g")}}
	chain.Sign(g, id.Signer())
	cfg := chain.Config{Quorum: 1, MinBond: 1 << 20, ByzantineQuorum: true, MatureValidators: 99}

	nd := i2Reload(t, id, g, cfg, failOnSave{markstore.NewMem()})
	if nd.recordSign(1, 0, chain.PhasePrepare, ports.HashBytes([]byte("blk"))) {
		t.Fatal("I2 fail-safe VIOLATION: recordSign returned true despite a Save failure — the signature would be released with no durable mark (honest self-slash on restart)")
	}
	// And having refused to persist, the node's live mark must not have advanced
	// — otherwise it would sign the wire while disk stayed blank.
	if nd.signMarkSet {
		t.Fatal("I2 fail-safe VIOLATION: the live watermark advanced even though the mark could not be persisted")
	}
}
