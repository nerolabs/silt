package node

import (
	"crypto/rand"
	"crypto/rsa"
	"errors"
	"testing"

	"github.com/nerolabs/silt/adapters/guardstore"
	"github.com/nerolabs/silt/adapters/identity"
	"github.com/nerolabs/silt/core/blindtoken"
	"github.com/nerolabs/silt/ports"
)

// fixedClock holds the issuer at a chosen instant; nothing in these tests waits.
type fixedClock struct{ now ports.Time }

func (c *fixedClock) Now() ports.Time                                  { return c.now }
func (c *fixedClock) AfterFunc(ports.Duration, func()) (cancel func()) { return func() {} }

// atCreditEpoch puts iss's clock one day into credit epoch e.
func atCreditEpoch(iss *Node, e uint64) {
	iss.clock = &fixedClock{now: ports.Time(e)*ports.Time(creditEpochLength) + ports.Time(24*3600*ports.Second)}
}

// epochCredit mints a credit for epoch e under key, as a publisher would.
func epochCredit(t *testing.T, key *rsa.PrivateKey, e uint64) ports.PublishCredit {
	t.Helper()
	serial, err := blindtoken.NewSerial(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	blinded, secret, err := blindtoken.BlindCredit(rand.Reader, &key.PublicKey, e, serial)
	if err != nil {
		t.Fatal(err)
	}
	bs, err := blindtoken.SignBlinded(rand.Reader, key, blinded)
	if err != nil {
		t.Fatal(err)
	}
	sig, err := blindtoken.UnblindCredit(&key.PublicKey, e, serial, bs, secret)
	if err != nil {
		t.Fatal(err)
	}
	return ports.PublishCredit{Serial: serial, Sig: sig, Epoch: e}
}

// spendCredit runs the issuer's charge for cr, as a token request carrying it would.
func spendCredit(iss *Node, cr ports.PublishCredit) error {
	charge, err := iss.tokenChargeFor(identity.FromSeed(csEphSeed).NodeID(), &cr)
	if err != nil {
		return err
	}
	return charge()
}

// A credit is spendable in its own epoch and the next, and one epoch early for
// clock skew; outside that it is refused by name, however valid its signature.
func TestCreditIsSpendableOnlyWithinItsWindow(t *testing.T) {
	publishKey, committed := csKeys(t)
	iss, _ := csBootIssuer(t, csIssuerSeed, publishKey, committed, guardstore.NewMem())
	const now = 10
	atCreditEpoch(iss, now)
	for _, tc := range []struct {
		epoch uint64
		ok    bool
	}{{now - 2, false}, {now - 1, true}, {now, true}, {now + 1, true}, {now + 2, false}} {
		err := spendCredit(iss, epochCredit(t, publishKey, tc.epoch))
		if tc.ok && err != nil {
			t.Errorf("a credit of epoch %d at epoch %d was refused: %v", tc.epoch, now, err)
		}
		if !tc.ok && !errors.Is(err, errCreditExpired) {
			t.Errorf("a credit of epoch %d at epoch %d: got %v, want errCreditExpired", tc.epoch, now, err)
		}
	}
}

// When the epoch advances, the records of epochs no longer spendable leave memory
// and the log, the records still spendable stay, and an expired credit stays refused:
// the sweep never re-opens a double spend.
func TestCreditSweepDropsOnlyRecordsThatCanNoLongerBeSpent(t *testing.T) {
	publishKey, committed := csKeys(t)
	store := guardstore.NewMem()
	iss, _ := csBootIssuer(t, csIssuerSeed, publishKey, committed, store)
	atCreditEpoch(iss, 10)
	old, cur := epochCredit(t, publishKey, 9), epochCredit(t, publishKey, 10)
	for _, cr := range []ports.PublishCredit{old, cur} {
		if err := spendCredit(iss, cr); err != nil {
			t.Fatalf("spend at epoch 10: %v", err)
		}
	}

	atCreditEpoch(iss, 11)
	if err := spendCredit(iss, epochCredit(t, publishKey, 11)); err != nil {
		t.Fatalf("spend at epoch 11: %v", err)
	}
	if iss.creditFloor != 10 {
		t.Fatalf("floor %d after the epoch advanced to 11, want 10", iss.creditFloor)
	}
	if iss.creditSpent[creditSpentKey(9, old.Serial)] {
		t.Error("the epoch-9 record is still held after the sweep")
	}
	if !iss.creditSpent[creditSpentKey(10, cur.Serial)] {
		t.Error("the sweep dropped the epoch-10 record, whose credit is still spendable")
	}
	if _, has := csStoreHas(t, store, old.Serial); has {
		t.Error("the epoch-9 record is still in the log after the sweep")
	}
	if _, has := csStoreHas(t, store, cur.Serial); !has {
		t.Error("the epoch-10 record left the log")
	}
	if err := spendCredit(iss, old); !errors.Is(err, errCreditExpired) {
		t.Fatalf("the swept epoch-9 credit spent again: %v", err)
	}
	if err := spendCredit(iss, cur); !errors.Is(err, errCreditRefused) {
		t.Fatalf("the epoch-10 credit spent twice: %v", err)
	}
}

// The floor is what keeps a swept credit dead when the clock moves backwards. After
// a sweep and a restart with the clock set back an epoch, a credit whose record was
// swept is still refused, because the floor came back from the log.
func TestCreditFloorOutlivesARestartAndAClockSetBack(t *testing.T) {
	publishKey, committed := csKeys(t)
	store := guardstore.NewMem()
	iss, _ := csBootIssuer(t, csIssuerSeed, publishKey, committed, store)
	atCreditEpoch(iss, 10)
	old := epochCredit(t, publishKey, 9)
	if err := spendCredit(iss, old); err != nil {
		t.Fatal(err)
	}
	atCreditEpoch(iss, 11)
	if err := spendCredit(iss, epochCredit(t, publishKey, 11)); err != nil {
		t.Fatal(err)
	}

	again, _ := csBootIssuer(t, csIssuerSeed, publishKey, committed, store)
	if again.creditFloor != 10 {
		t.Fatalf("floor %d after the restart, want 10", again.creditFloor)
	}
	atCreditEpoch(again, 10) // the clock set back: epoch 9 is inside the window again
	if err := spendCredit(again, old); !errors.Is(err, errCreditExpired) {
		t.Fatalf("after a restart with the clock set back, the swept credit spent again: %v", err)
	}
}

// Only a credit-sized serial can be spent, so no credit can be read back at boot as
// the floor record and raise the floor past live epochs.
func TestCreditSerialMustBeCreditSized(t *testing.T) {
	publishKey, committed := csKeys(t)
	iss, _ := csBootIssuer(t, csIssuerSeed, publishKey, committed, guardstore.NewMem())
	atCreditEpoch(iss, 10)
	cr := epochCredit(t, publishKey, 10)
	blinded, secret, err := blindtoken.BlindCredit(rand.Reader, &publishKey.PublicKey, 11, creditFloorSerial)
	if err != nil {
		t.Fatal(err)
	}
	bs, err := blindtoken.SignBlinded(rand.Reader, publishKey, blinded)
	if err != nil {
		t.Fatal(err)
	}
	sig, err := blindtoken.UnblindCredit(&publishKey.PublicKey, 11, creditFloorSerial, bs, secret)
	if err != nil {
		t.Fatal(err)
	}
	if err := spendCredit(iss, ports.PublishCredit{Serial: creditFloorSerial, Sig: sig, Epoch: 11}); !errors.Is(err, errCreditRefused) {
		t.Fatalf("a credit whose serial is the floor marker was not refused: %v", err)
	}
	if err := spendCredit(iss, cr); err != nil {
		t.Fatalf("an ordinary credit was refused: %v", err)
	}
}
