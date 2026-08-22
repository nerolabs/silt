package node

import "github.com/nerolabs/silt/ports"

// StartBootstrapRetry begins a periodic self-heal: while the routing table is
// EMPTY, re-run the Kademlia join against the original -bootstrap seeds.
//
// silt's join (Bootstrap) is otherwise ONE-SHOT. On a real multi-node cold start
// there is no ordering guarantee that a node's bootstrap target is already
// listening when the node comes up. If the initial FIND_NODE races the target's
// listener it fails, the node lands with an empty routing table, and — without
// this loop — never tries again: no peers, no consensus, no discovery, until a
// manual restart. This was found on the first 13-node cross-region cloud run,
// where three validators logged `bootstrapped (0 table entries)` and the network
// never formed (#281). The same loop also recovers a node that later loses every
// peer to churn.
//
// Idempotent and cheap: it only acts while Table().Size() == 0, and only if the
// node was given seeds. Once any peer is in the table it is a no-op, leaving
// normal DHT maintenance to the lookups themselves. Call once, after the initial
// Bootstrap, on the node's event loop. onRejoin (optional) fires when a retry
// recovers an empty table — so the daemon can log the self-heal.
func (n *Node) StartBootstrapRetry(onRejoin func(tableSize int)) {
	if n.cfg.BootstrapRetryInterval <= 0 || n.bootstrapRetryRunning {
		return
	}
	n.bootstrapRetryRunning = true
	n.bootstrapRejoin = onRejoin
	n.clock.AfterFunc(n.cfg.BootstrapRetryInterval, n.bootstrapRetryTick)
}

func (n *Node) bootstrapRetryTick() {
	sz := n.table.Size()
	switch {
	case sz == 0 && len(n.bootstrapSeeds) > 0:
		// Fully isolated WITH seeds (#281): re-seed and re-join. The failed initial
		// dial stamped each seed into the dead-peer negative cache (HolderCooldown);
		// clear the seeds first so the retry re-dials them now instead of skipping
		// them until the cooldown lapses.
		for _, s := range n.bootstrapSeeds {
			delete(n.dead, s)
			n.table.Observe(s)
		}
		n.IterativeFindNode(n.id, func([]ports.NodeID) {
			if s2 := n.table.Size(); s2 > 0 && n.bootstrapRejoin != nil {
				n.bootstrapRejoin(s2) // empty → recovered: surface the self-heal
			}
		})
	case sz > 0 && n.cfg.BootstrapWellConnected > 0 && sz < n.cfg.BootstrapWellConnected:
		// SPARSE mesh — a self-lookup discovers more peers (bucket refresh), so a
		// simultaneous cold start converges enough for consensus quorum to form.
		// Recovering an empty table (#281) is not enough: a node can re-bootstrap to
		// one peer and stall. This also reaches the seedless boot validator — it
		// queries the peers that dialed IN and thereby learns the rest of the set.
		n.IterativeFindNode(n.id, func([]ports.NodeID) {})
	}
	n.clock.AfterFunc(n.cfg.BootstrapRetryInterval, n.bootstrapRetryTick)
}
