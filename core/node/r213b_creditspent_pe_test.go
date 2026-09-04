package node

import (
	"errors"
	"testing"

	"github.com/nerolabs/silt/adapters/guardstore"
	"github.com/nerolabs/silt/ports"
)

// PE ruling RULING-R2.13b-creditspent-build-fa9f988 F3: refuse-to-start ABOVE the cap was
// claimed by the docs and unpinned (the PE's ablation "Load accepts > cap" left every gate
// green). This pins it: a store holding cap+1 records is refused at load, by name.
func TestCreditSpentLoadRefusesAboveTheCap(t *testing.T) {
	st := guardstore.NewMem()
	for i := 0; i < maxCreditSpent+1; i++ {
		s := make([]byte, 32)
		s[0], s[1], s[2] = byte(i), byte(i>>8), byte(i>>16)
		if err := st.Append(ports.PaidSerial{Serial: s}); err != nil {
			t.Fatal(err)
		}
	}
	n := &Node{creditSpent: map[string]bool{}}
	n.SetCreditSpentStore(st)
	if err := n.LoadCreditSpent(); err == nil {
		t.Fatalf("a persisted credit-spent guard with %d records (cap %d) loaded — the cap is a boot-time refusal, not advice", maxCreditSpent+1, maxCreditSpent)
	}
	if n.creditLoaded {
		t.Fatal("the guard must not be marked loaded after a refused load")
	}
}

// PE ruling F2: the file is BOUND to the publish key it was written under. A record
// carrying another key's fingerprint refuses the boot by name; unbound (zero Server)
// records are tolerated.
func TestCreditSpentFileBoundToThePublishKey(t *testing.T) {
	st := guardstore.NewMem()
	foreign := ports.NodeID{}
	foreign[0] = 0xF0
	if err := st.Append(ports.PaidSerial{Serial: []byte("credit-under-another-key-00000"), Server: foreign}); err != nil {
		t.Fatal(err)
	}
	n := &Node{creditSpent: map[string]bool{}}
	n.SetCreditSpentStore(st)
	err := n.LoadCreditSpent()
	if !errors.Is(err, errCreditStoreForeignKey) {
		t.Fatalf("a creditspent.log written under another publish key must refuse the boot with errCreditStoreForeignKey, got %v", err)
	}
	// Unbound records (zero Server) load: the test-padded / pre-binding shape.
	st2 := guardstore.NewMem()
	if err := st2.Append(ports.PaidSerial{Serial: []byte("unbound-record-000000000000000")}); err != nil {
		t.Fatal(err)
	}
	n2 := &Node{creditSpent: map[string]bool{}}
	n2.SetCreditSpentStore(st2)
	if err := n2.LoadCreditSpent(); err != nil {
		t.Fatalf("an unbound record must load, got %v", err)
	}
}
