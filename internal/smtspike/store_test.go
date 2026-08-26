package smtspike

import (
	"github.com/pokt-network/smt/kvstore"
	"github.com/pokt-network/smt/kvstore/simplemap"
)

// countingStore wraps a MapStore and totals the bytes written through it. It
// exists to answer the residency question the heap number alone cannot: how
// much of the in-memory cost is Go map overhead, and how much is payload a
// disk-backed store would actually have to hold.
//
// It keeps no per-key bookkeeping of its own, so it does not inflate the heap
// measurement it sits inside. setCount vs Len() exposes rewrite churn: on a
// fresh build the two track each other, and a large gap would mean the trie is
// rewriting nodes it already wrote.
//
// It doubles as the worked size of the integration: kvstore.MapStore is five
// methods, so backing the trie with silt's own on-disk store is a small adapter,
// not a project.
type countingStore struct {
	inner    kvstore.MapStore
	setBytes int
	setCount int
}

func newCountingStore() *countingStore {
	return &countingStore{inner: simplemap.NewSimpleMap()}
}

func (c *countingStore) Get(key []byte) ([]byte, error) { return c.inner.Get(key) }

func (c *countingStore) Set(key, value []byte) error {
	if err := c.inner.Set(key, value); err != nil {
		return err
	}
	c.setBytes += len(key) + len(value)
	c.setCount++
	return nil
}

func (c *countingStore) Delete(key []byte) error { return c.inner.Delete(key) }

func (c *countingStore) Len() int { return c.inner.Len() }

func (c *countingStore) ClearAll() error {
	c.setBytes, c.setCount = 0, 0
	return c.inner.ClearAll()
}
