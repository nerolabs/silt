package node

import (
	"crypto/rand"
	"testing"

	"github.com/nerolabs/silt/adapters/identity"
	"github.com/nerolabs/silt/adapters/memstore"
	"github.com/nerolabs/silt/ports"
)

// TestForwardedEntryReachesNoValidatorUnderTheValidatorsIdentity is the publish half of the
// privacy claim on a validator's own publish. The entry a validator publishes from its own UI
// is handed to the validators by the publish client's throwaway identity (ForwardEntry), so
// every validator — the publishing one included — sees that throwaway sender and nothing
// links the entry to the validator's durable identity; each still queues the entry for its
// next proposal. The contrast arm is the relay path, SubmitEntry, which a registry host uses
// for a third-party client's entry and which does carry the host's identity.
func TestForwardedEntryReachesNoValidatorUnderTheValidatorsIdentity(t *testing.T) {
	nodes, ids, net, _, _ := tier2AnchorNet(t, 4)
	publisher := ids[0].NodeID()
	var validators []ports.NodeID
	for _, id := range ids {
		validators = append(validators, id.NodeID())
	}

	clientID, err := identity.Generate(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	client := New(clientID.NodeID(), DefaultConfig(), nodes[0].clock, net.Endpoint(clientID.NodeID()), memstore.New())
	client.SetEphemeral(true)

	sendersOf := func(root ports.Hash) map[ports.NodeID]ports.NodeID {
		seen := map[ports.NodeID]ports.NodeID{}
		for _, m := range net.Pending() {
			if m.Kind == ports.MsgSubmitEntry {
				seen[m.To] = m.From
				net.Deliver(m.ID)
			}
		}
		return seen
	}

	e := mkEntry("validator-own-publish")
	client.ForwardEntry(e, validators)
	seen := sendersOf(e.Root)
	if len(seen) != len(validators) {
		t.Fatalf("the client reached %d of %d validators", len(seen), len(validators))
	}
	for to, from := range seen {
		if from == publisher {
			t.Fatalf("validator %s received the publisher's entry from the publisher's durable identity", to.String()[:8])
		}
		if from != clientID.NodeID() {
			t.Fatalf("validator %s received it from %s, not the throwaway client", to.String()[:8], from.String()[:8])
		}
	}
	for i, nd := range nodes {
		queued := false
		for _, pe := range nd.pendingEntries {
			if pe.E.Root == e.Root {
				queued = true
			}
		}
		if !queued {
			t.Fatalf("validator %d did not queue the forwarded entry for its next proposal", i)
		}
	}

	// Contrast: the relay path carries the relaying validator's identity, which is why a
	// validator's own publish must not take it.
	relayed := mkEntry("third-party-client-entry")
	nodes[0].SubmitEntry(relayed, validators)
	for to, from := range sendersOf(relayed.Root) {
		if from != publisher {
			t.Fatalf("control: SubmitEntry to %s came from %s, not the relaying validator", to.String()[:8], from.String()[:8])
		}
	}
}
