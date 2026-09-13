# Development / test infrastructure — NOT operated by the Silt project

Everything in this directory is **throwaway dev scaffolding**: a way to
stand up one publicly reachable node to develop and test cross-network
reachability (#27) against, because localhost has no NAT to traverse.

Read this before using it, because the distinction matters:

- **Not privileged.** Nothing in the protocol treats this node
  specially. It is an ordinary `silt daemon` that happens to have a
  public IP and the `-relay` flag on.
- **Not default.** No address from here is baked into the binary.
  Peers use it only if an operator passes it to `-bootstrap`,
  `-dns-seed`, or `-relay-via` by hand.
- **Not "official."** SiltHQ publishes source; it does not run the
  network. A developer running a box to test against is an operator
  like any other, and this one is explicitly temporary.
- **Replaced by community peers.** The goal state is many operators
  running reachable `-relay` daemons and publishing `-dns-seed`
  domains; this box is torn down or becomes one unremarkable peer
  among them.

The reasoning is the no-permanent-center immutable: nothing the project runs may
become load-bearing.

## What's here

- `dev-relay.sh` — provision + run script for a small VPS (any
  $5 Debian/Ubuntu box with a public IP). Builds Silt from source and
  runs a daemon with the relay capability on.
- `silt-dev-relay.service` — optional systemd unit so the daemon
  survives reboots for the life of the experiment.

## Quick start (on the VPS)

```sh
# as a normal user with go >= 1.22 installed
git clone https://github.com/nerolabs/silt.git
cd silt/deploy
./dev-relay.sh
```

The script prints the two lines that matter, e.g.:

```
bootstrap: <ID>@203.0.113.7:4001     # for -bootstrap / a dns-seed TXT record
relay-via: <ID>@203.0.113.7:4002     # for NATed daemons' -relay-via
```

Open TCP 4001 (swarm) and 4002 (relay) in the provider's firewall.

Point two NATed machines on different networks at it and confirm one can
publish and the other can fetch. The automated form of the same proof is
`integration/nat/run.sh`, which stands up two genuinely-NATed daemons and a
relay in real container networks and drives publish, fetch, restart survival
and hole-punch (cone punches through, symmetric falls back to the relay). CI
gates it as `Integration — cross-NAT over a real relay (docker)` and
`Integration — NAT hole-punch (cone punches, symmetric falls back)`. A VPS
adds real routers, real NAT timeouts and a real WAN on top of that.

## Teardown

Delete the VPS. Nothing in the network depends on it: peers that
learned its address will simply time it out of their routing tables,
and any daemon that used it as `-relay-via` falls back to being
unreachable across networks until pointed at another relay — which is
exactly the dependency honesty the design wants.
