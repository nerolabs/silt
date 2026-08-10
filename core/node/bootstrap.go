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
	// Only act when fully isolated. A non-empty table means the join succeeded
	// (or peers arrived on their own) — leave it alone.
	if n.table.Size() == 0 && len(n.bootstrapSeeds) > 0 {
		// The failed initial dial stamped each seed into the deadUntil negative
		// cache (HolderCooldown). Clear the seeds first so the retry actually
		// re-dials them now, instead of skipping them until the cooldown lapses.
		for _, s := range n.bootstrapSeeds {
			delete(n.deadUntil, s)
			n.table.Observe(s)
		}
		n.IterativeFindNode(n.id, func([]ports.NodeID) {
			if sz := n.table.Size(); sz > 0 && n.bootstrapRejoin != nil {
				n.bootstrapRejoin(sz) // empty → recovered: surface the self-heal
			}
		})
	}
	n.clock.AfterFunc(n.cfg.BootstrapRetryInterval, n.bootstrapRetryTick)
}
