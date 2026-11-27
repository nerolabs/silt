package node

import (
	"fmt"
	"io"
	"sort"

	"github.com/nerolabs/silt/core/link"
	"github.com/nerolabs/silt/core/manifest"
	"github.com/nerolabs/silt/core/pipeline"
	"github.com/nerolabs/silt/ports"
)

// manifestCacheSize bounds how many opened manifests a node keeps for ranged reads. A video
// player asks for a file a few seconds at a time, and re-pulling a large file's manifest
// (about 1.5 MB for a 2.4 GB file at the default chunk size) for every range would cost more
// than the range itself.
const manifestCacheSize = 16

type manifestCache struct {
	byKey map[string]*manifest.Manifest
	order []string // oldest first
}

func manifestCacheKey(h link.Handle) string {
	lk, ck := h.LayoutKey(), h.ContentKey()
	return string(h.Root[:]) + string(lk[:]) + string(ck[:])
}

func (c *manifestCache) get(h link.Handle) *manifest.Manifest {
	if c.byKey == nil {
		return nil
	}
	return c.byKey[manifestCacheKey(h)]
}

func (c *manifestCache) put(h link.Handle, m *manifest.Manifest) {
	if c.byKey == nil {
		c.byKey = map[string]*manifest.Manifest{}
	}
	k := manifestCacheKey(h)
	if _, ok := c.byKey[k]; ok {
		return
	}
	if len(c.order) >= manifestCacheSize {
		delete(c.byKey, c.order[0])
		c.order = c.order[1:]
	}
	c.byKey[k] = m
	c.order = append(c.order, k)
}

// NetGetRange writes bytes [off, off+length) of the file h names to w, length clamped to the
// file's end, pulling from the network only the chunks that range covers (pipeline.ReadRange
// judges and decrypts them). A covered chunk that cannot be fetched is rebuilt from its own
// stripe: the stripe's other data shards, then parity by the same deficit walk a whole-file
// fetch uses, so a range never costs more than the stripes it touches. Like NetGet it keeps
// nothing it pulled: what this call fetched is dropped when it settles, and what the node
// already held stays as it was.
func (n *Node) NetGetRange(reg ports.Registry, h link.Handle, off, length int64, w io.Writer, done func(error)) {
	n.lookupEntryAsync(reg, h.Root, func(entry ports.Entry, ok bool, err error) {
		if err != nil || !ok {
			done(fmt.Errorf("netget range %s: %w", h.Root, ports.ErrNoSuchEntry))
			return
		}
		n.rangeManifest(entry, h, func(m *manifest.Manifest, err error) {
			if err != nil {
				done(fmt.Errorf("netget range: %w", err))
				return
			}
			n.netGetRange(m, h, off, length, w, done)
		})
	})
}

// NetFileSize reports the size of the file h names, opening (and caching) its manifest, so
// a caller can resolve a range against the file's end before asking for it.
func (n *Node) NetFileSize(reg ports.Registry, h link.Handle, done func(int64, error)) {
	n.lookupEntryAsync(reg, h.Root, func(entry ports.Entry, ok bool, err error) {
		if err != nil || !ok {
			done(0, fmt.Errorf("netget range %s: %w", h.Root, ports.ErrNoSuchEntry))
			return
		}
		n.rangeManifest(entry, h, func(m *manifest.Manifest, err error) {
			if err != nil {
				done(0, fmt.Errorf("netget range: %w", err))
				return
			}
			done(m.FileSize, nil)
		})
	})
}

// rangeManifest returns h's opened manifest, from the cache or by pulling its chunks. The
// root check is made here, because a ranged read never runs pipeline.Get, which is where a
// whole-file fetch makes it.
func (n *Node) rangeManifest(entry ports.Entry, h link.Handle, done func(*manifest.Manifest, error)) {
	if m := n.manifests.get(h); m != nil {
		done(m, nil)
		return
	}
	held := map[ports.ChunkID]bool{}
	for _, id := range entry.ManifestChunks {
		if ok, _ := n.store.Has(bg(), id); ok {
			held[id] = true
		}
	}
	n.wantWorking(entry.ManifestChunks)
	n.fetchAll(entry.ManifestChunks, func(missing []ports.ChunkID) {
		var m *manifest.Manifest
		var err error
		if len(missing) > 0 {
			err = fmt.Errorf("%d of %d manifest chunks unreachable", len(missing), len(entry.ManifestChunks))
		} else if m, err = pipeline.LoadFull(bg(), workingStore{n}, entry, h); err == nil && m.Root() != h.Root {
			err = fmt.Errorf("manifest root %s does not match requested root %s", m.Root(), h.Root)
		}
		for _, id := range entry.ManifestChunks {
			if !held[id] {
				if ok, _ := n.store.Has(bg(), id); ok {
					n.dropHosted(id)
				}
			}
		}
		n.releaseWorking(entry.ManifestChunks)
		if err != nil {
			done(nil, err)
			return
		}
		n.manifests.put(h, m)
		done(m, nil)
	})
}

func (n *Node) netGetRange(m *manifest.Manifest, h link.Handle, off, length int64, w io.Writer, done func(error)) {
	first, last, ok, err := pipeline.RangeChunks(m, off, length)
	if err != nil {
		done(err)
		return
	}
	if !ok {
		done(nil)
		return
	}
	dataIDs := m.ChunkIDs()

	// Held-before snapshot, as in netGetEntry: only what this call pulls is dropped.
	held := map[ports.ChunkID]bool{}
	var wanted []ports.ChunkID
	want := func(ids []ports.ChunkID) {
		for _, id := range ids {
			if ok, _ := n.store.Has(bg(), id); ok {
				held[id] = true
			}
		}
		wanted = append(wanted, ids...)
		n.wantWorking(ids)
	}
	settle := func(err error) {
		for _, id := range wanted {
			if held[id] {
				continue
			}
			if ok, _ := n.store.Has(bg(), id); ok {
				n.dropHosted(id)
			}
		}
		n.releaseWorking(wanted)
		done(err)
	}
	finish := func() {
		settle(pipeline.ReadRange(bg(), workingStore{n}, m, off, length, w))
	}
	// Presence read through the working store, so a piece held in a full pledge's overflow
	// counts, and re-verified, as netGetEntry does (a disk Has is only a stat).
	present := func(id ports.ChunkID) bool {
		c, err := workingStore{n}.Get(bg(), id)
		return err == nil && c.Verify()
	}

	rangeIDs := dataIDs[first : last+1]
	want(rangeIDs)
	if m.K == 0 { // uncoded: no parity to fall back to
		n.fetchAll(rangeIDs, func([]ports.ChunkID) { finish() })
		return
	}

	// fetchByColumn pulls leaves grouped by their column — one provider lookup per column.
	fetchByColumn := func(leaves []int, after func()) {
		byCol := map[int][]ports.ChunkID{}
		leafIDs := m.Leaves()
		for _, li := range leaves {
			id := leafIDs[li]
			if present(id) {
				continue
			}
			c := columnOfLeaf(m, li)
			byCol[c] = append(byCol[c], id)
		}
		cols := make([]int, 0, len(byCol))
		for c := range byCol {
			cols = append(cols, c)
		}
		sort.Ints(cols)
		var next func(i int)
		next = func(i int) {
			if i == len(cols) {
				after()
				return
			}
			n.fetchColumn(h.Root, cols[i], byCol[cols[i]], func([]ports.ChunkID) { next(i + 1) })
		}
		next(0)
	}

	k, par := m.K, m.N-m.K
	s0, s1 := first/k, last/k
	rangeLeaves := make([]int, 0, last-first+1)
	for i := first; i <= last; i++ {
		rangeLeaves = append(rangeLeaves, i)
	}
	fetchByColumn(rangeLeaves, func() {
		// Stripes whose covered chunks did not all arrive need rebuilding: pull their other
		// data shards, then walk the parity columns for whatever is still short.
		var broken []int
		for s := s0; s <= s1; s++ {
			for i := max(s*k, first); i <= min(s*k+k-1, last); i++ {
				if !present(dataIDs[i]) {
					broken = append(broken, s)
					break
				}
			}
		}
		if len(broken) == 0 {
			finish()
			return
		}
		var rest []int
		for _, s := range broken {
			for i := s * k; i < min(s*k+k, len(dataIDs)); i++ {
				if i < first || i > last {
					rest = append(rest, i)
				}
			}
		}
		restIDs := make([]ports.ChunkID, len(rest))
		for x, i := range rest {
			restIDs[x] = dataIDs[i]
		}
		want(restIDs)
		fetchByColumn(rest, func() {
			deficit := map[int]int{}
			for _, s := range broken {
				for i := s * k; i < min(s*k+k, len(dataIDs)); i++ {
					if !present(dataIDs[i]) {
						deficit[s]++
					}
				}
			}
			parityIDs := m.ParityIDs()
			var walk func(q int)
			walk = func(q int) {
				short := 0
				for _, d := range deficit {
					short += d
				}
				if short == 0 || q >= par {
					finish()
					return
				}
				var leaves, asked []int
				var ids []ports.ChunkID
				for s, d := range deficit {
					if d <= 0 {
						continue
					}
					id := parityIDs[s*par+q]
					if present(id) {
						deficit[s]-- // already held: settles one shard without a fetch
						continue
					}
					leaves = append(leaves, len(dataIDs)+s*par+q)
					asked = append(asked, s)
					ids = append(ids, id)
				}
				if len(ids) == 0 {
					walk(q + 1)
					return
				}
				want(ids)
				n.Stats.ParityColumnLookups++
				fetchByColumn(leaves, func() {
					for _, s := range asked {
						if present(parityIDs[s*par+q]) {
							deficit[s]--
						}
					}
					walk(q + 1)
				})
			}
			walk(0)
		})
	})
}
