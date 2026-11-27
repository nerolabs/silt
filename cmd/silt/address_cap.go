package main

// The daemon surface of observed-address keying: the -dht-address-cap mode
// parse (default SHADOW: count, never refuse, until the project enables `on`
// against the measured series A/B/E —) and the de-herd relay selection (a
// precondition of `on`: today every NATed pony adopted the lowest-ID relay).

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"

	"github.com/nerolabs/silt/adapters/tcpnet"
	"github.com/nerolabs/silt/core/dht"
	"github.com/nerolabs/silt/ports"
)

// defaultAddressCapMode is the -dht-address-cap default.
const defaultAddressCapMode = "shadow"

// parseAddressCapMode maps the flag value to the table mode; anything but
// off|shadow|on is refused.
func parseAddressCapMode(s string) (dht.AddressMode, error) {
	switch s {
	case "off":
		return dht.AddressCapOff, nil
	case "shadow":
		return dht.AddressCapShadow, nil
	case "on":
		return dht.AddressCapOn, nil
	}
	return dht.AddressCapOff, fmt.Errorf("-dht-address-cap=%q: want off, shadow or on", s)
}

// pickRelay is the de-herd: among the gossiped relays choose the one minimising
// H(self ‖ relayID) — deterministic per node, uniform across nodes, so no relay
// carries the herd — skipping relays that refused registration (fail-over).
// ok=false when no candidate remains.
func pickRelay(self ports.NodeID, relays []tcpnet.Peer, refused map[ports.NodeID]bool) (tcpnet.Peer, bool) {
	var best tcpnet.Peer
	var bestScore uint64
	found := false
	for _, r := range relays {
		if refused[r.ID] {
			continue
		}
		h := sha256.Sum256(append(append([]byte{}, self[:]...), r.ID[:]...))
		score := binary.BigEndian.Uint64(h[:8])
		if !found || score < bestScore {
			best, bestScore, found = r, score, true
		}
	}
	return best, found
}

// addressCapTallyEvery is how often a shadow-mode daemon logs its would-refuse tally.
const addressCapTallyEvery = 600 * ports.Second

// addressCapShadowTally renders the shadow run's evidence as one log line: the would-be refusals
// at the SHIPPED reserve and relay cap (the shadow also records other candidate settings), by peer
// class, beside how many entries the routing table holds.
func addressCapShadowTally(refusals map[dht.ShadowKey]int, reserve, capRelay, tableSize int) string {
	byClass := map[string]int{}
	total := 0
	for k, v := range refusals {
		if k.Reserve != reserve || k.CapRelay != capRelay {
			continue
		}
		byClass[className(k.Class)] += v
		total += v
	}
	return fmt.Sprintf("dht-address-cap: shadow tally — would refuse %d (direct %d, relayed %d, unverified %d) at reserve=%d cap_relay=%d; routing table holds %d",
		total, byClass["direct"], byClass["relayed"], byClass["unverified"], reserve, capRelay, tableSize)
}
