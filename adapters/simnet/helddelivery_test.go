package simnet_test

import (
	"testing"

	"github.com/nerolabs/silt/adapters/simnet"
	"github.com/nerolabs/silt/ports"
)

// TestHeldDeliveryParksAndFiresInChosenOrder is the conformance test for the
// model-check delivery control: in held mode Send parks messages (nothing fires on
// its own), the driver inspects Pending() and fires them in an order IT picks via
// Deliver(id), and DropPending() models loss. This is the substrate the tier-2
// consensus model-check drives the real node loop over.
func TestHeldDeliveryParksAndFiresInChosenOrder(t *testing.T) {
	_, n := setup(1, simnet.DefaultConfig())
	n.EnableHeldDelivery()

	a, b, c := n.Endpoint(id(1)), n.Endpoint(id(2)), n.Endpoint(id(3))
	var got []byte
	rec := func(from ports.NodeID, msg ports.Message) { got = append(got, msg.Data...) }
	b.SetHandler(rec)
	c.SetHandler(rec)
	a.SetHandler(func(ports.NodeID, ports.Message) {})

	// Three sends: to b ("A"), to c ("B"), to b ("C"). Nothing delivers yet.
	a.Send(id(2), ports.Message{Data: []byte("A")})
	a.Send(id(3), ports.Message{Data: []byte("B")})
	a.Send(id(2), ports.Message{Data: []byte("C")})

	if len(got) != 0 {
		t.Fatalf("held mode must park delivery: handler ran before Deliver (got %q)", got)
	}
	p := n.Pending()
	if len(p) != 3 {
		t.Fatalf("Pending must show all 3 parked messages, got %d", len(p))
	}
	if p[0].From != id(1) || p[0].To != id(2) {
		t.Fatalf("Pending metadata wrong: %+v", p[0])
	}

	// Deliver OUT of FIFO order: the 2nd ("B"), then the 3rd ("C"); DROP the 1st ("A").
	if !n.Deliver(p[1].ID) {
		t.Fatal("Deliver(second) should succeed")
	}
	if !n.Deliver(p[2].ID) {
		t.Fatal("Deliver(third) should succeed")
	}
	if !n.DropPending(p[0].ID) {
		t.Fatal("DropPending(first) should succeed")
	}
	if string(got) != "BC" {
		t.Fatalf("delivery must follow the driver's chosen order (want BC, the dropped A never arrives), got %q", got)
	}
	if len(n.Pending()) != 0 {
		t.Fatalf("queue must be empty after delivering/dropping all, got %d", len(n.Pending()))
	}
	if n.Deliver(999) { // no such id
		t.Fatal("Deliver of an unknown id must return false")
	}
}

// TestHeldDeliveryReChecksDeathAtDeliverTime confirms a parked message honors a Kill
// that happens AFTER it was sent but BEFORE it is delivered — the property the tier-2
// I2 restart drill relies on (kill mid-schedule, then a stale in-flight message must
// not reach the dead/handlerless node).
func TestHeldDeliveryReChecksDeathAtDeliverTime(t *testing.T) {
	_, n := setup(1, simnet.DefaultConfig())
	n.EnableHeldDelivery()
	a, b := n.Endpoint(id(1)), n.Endpoint(id(2))
	delivered := false
	b.SetHandler(func(ports.NodeID, ports.Message) { delivered = true })
	a.SetHandler(func(ports.NodeID, ports.Message) {})

	a.Send(id(2), ports.Message{Data: []byte("x")})
	n.Kill(id(2)) // b dies while the message is parked
	p := n.Pending()
	if len(p) != 1 {
		t.Fatalf("one message should be parked, got %d", len(p))
	}
	n.Deliver(p[0].ID)
	if delivered {
		t.Fatal("a parked message must be dropped at Deliver time if the destination has since died")
	}
}
