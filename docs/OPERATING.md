# Running a silt node

This guide is for anyone who runs a silt node where other people can reach it. Read it before you
expose a node to the internet. It is not legal advice.

## What a node holds

A storage node keeps encrypted pieces of other people's files and serves them to anyone who asks.

- **You cannot read what you hold.** Each file is encrypted on its publisher's machine, and the key
  lives only in the file's `silt:` link. A piece on your disk is ciphertext.
- **You do not choose what you hold.** Placement assigns pieces to nodes by hash. Any file
  published on the network can have pieces on your node.
- **Your node is visible.** Peers see your IP address, and you see theirs. silt does not hide
  addresses; it is not an anonymity network.

In many countries, storing and serving other people's data makes you a hosting provider, with
duties when you are told that something you host is unlawful. The rest of this guide covers the
tools silt gives you to meet those duties. Check what applies where you run the node.

## Starting a node

A storage node needs a store directory, a storage pledge and at least one peer to join through:

```sh
silt daemon -store /var/lib/silt -capacity 50G \
  -listen 0.0.0.0:4001 -bootstrap ID@HOST:PORT
```

- `-capacity` is how much disk you pledge. The node stores up to that much and no more.
- `-listen` must be reachable from outside for the node to serve. Behind NAT, the node hole-punches
  where it can and falls back to a relay where it can't.
- `silt id -store /var/lib/silt` prints the node's ID, which is what other operators use to name it.

To try silt without exposing anything, run `./examples/try-a-local-network.sh` instead. It runs a
whole network on loopback.

### The smallest validator: the floor box

A floor box takes part in consensus without keeping a copy of the chain. It checks each block
against proofs pulled from other nodes, so it runs on 1 CPU, 2 GB of RAM and 10 GB of disk:

```sh
silt daemon -floor-box -ws-checkpoint HEIGHT:HASH \
  -witness-from ID1,ID2,ID3 -bootstrap ID@HOST:PORT
```

The checkpoint is the block your box starts trusting from. Take it from a node you trust:
`silt chain-status` prints its head height and hash. A box refuses to start without one.

Name several witness providers. Your box's safety never depends on them: it checks every proof
against block roots it already trusts. Its liveness does, so a box with one unreachable provider
stalls until that provider returns. Providers keep the state of their last 8 blocks. A box that
falls further behind than that, or starts from an older checkpoint, must re-anchor on a newer one.

## Taking content down

silt removes content by its hash, one file at a time, and only on the nodes whose operators choose
to remove it. No one can remove a file from the whole network, and nothing acts on a person.

You have two tools. Use either or both.

### Your own denylist

A denylist is a file of root hashes, one per line, that your node refuses to store or serve:

```sh
silt daemon ... -denylist /etc/silt/denylist.txt
```

At startup the node deletes every held piece of a listed file, and it refuses those pieces from
then on. Use this for a notice you receive directly. A file's root hash is in its link; the
`examples/flow8-takedown.sh` script shows the conversion.

### Following curators

A curator is anyone who maintains a takedown list on the chain: a hotline, a rights body, a
community moderator, or you. Each takedown names its curator and carries the curator's signature.
Where publishing costs a token, so does a takedown. Every takedown is recorded in an append-only log that anyone
can audit.

Your node honours the takedowns of the curators you follow, and no one else's:

```sh
silt daemon ... -honor-curators CURATOR_ID,CURATOR_ID
```

When a followed curator lists a file, your node deletes its pieces at once and logs the removal
with a receipt:

```
takedown: honoured root=... curator=... purged=2 ... receipt=...
```

The receipt proves the removal was the network's committed record. Anyone holding a copy of the
chain can check it with `silt takedown-verify -receipt ...`.

A node that follows no curator honours no on-chain takedown. Following a curator means trusting
its judgment, so choose curators the way you would choose any blocklist.

### Becoming a curator

A validator lists a file under its own key with `-revoke ROOT`, or under a separate curator key
with `-revoke ROOT -curator-key FILE`. The key's ID is what operators put in `-honor-curators`.
Only the curator that listed a file can lift its listing.

## Reporting abuse

Report a file to the operators and curators who can act on it, with its link or root hash.

To report abuse of silt to the project, write to **abuse@silthq.com** with the link or root hash.
The project hosts no files, runs no network, maintains no denylist and has no takedown authority,
so it cannot remove a file from anyone's node. It will point you to the operators and curators who
can act.

Report child sexual abuse material to your national hotline as well, such as NCMEC in the US or the
IWF in the UK.

To report a security problem in the software itself, see [`SECURITY.md`](../SECURITY.md).

## Updates

Maintainers can publish a signed advisory that sets a minimum version. A node below it warns its
operator in its log. Update when you see one. At the advisory's enforce tier, after its grace
period, the network refuses consensus messages from nodes below the floor. An old node keeps
storing, serving and fetching; it stops counting in consensus.
