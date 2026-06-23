package sstable

import (
	"sync"
	"sync/atomic"

	lru "github.com/hashicorp/golang-lru/v2"
)

// DefaultBlockCacheBytes is the default SSTable data-block cache size (32 MiB).
const DefaultBlockCacheBytes = 32 << 20

// BlockCache is a byte-bounded LRU of immutable SSTable data-block payloads.
// The underlying lru.Cache is safe for concurrent Get; Add serialises eviction.
type BlockCache struct {
	inner *lru.Cache[uint64, []byte]
	mu    sync.Mutex
	bytes int
	max   int
}

var nextReaderFileID atomic.Uint64

// NewBlockCache returns an LRU cache capped at maxBytes. maxBytes <= 0 uses
// DefaultBlockCacheBytes.
func NewBlockCache(maxBytes int) *BlockCache {
	if maxBytes <= 0 {
		maxBytes = DefaultBlockCacheBytes
	}
	// Entry cap is an upper bound; eviction is driven by max bytes.
	inner, _ := lru.New[uint64, []byte](maxBytes / 256)
	return &BlockCache{
		inner: inner,
		max:   maxBytes,
	}
}

func blockCacheKey(fileID, offset uint64) uint64 {
	return fileID<<32 | (offset & 0xffffffff)
}

func (c *BlockCache) get(key uint64) ([]byte, bool) {
	if c == nil {
		return nil, false
	}
	return c.inner.Get(key)
}

func (c *BlockCache) add(key uint64, block []byte) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	if existing, ok := c.inner.Get(key); ok {
		c.bytes -= len(existing)
	}

	stored := append([]byte(nil), block...)
	c.inner.Add(key, stored)
	c.bytes += len(stored)

	for c.bytes > c.max {
		_, v, ok := c.inner.RemoveOldest()
		if !ok {
			break
		}
		c.bytes -= len(v)
	}
}
