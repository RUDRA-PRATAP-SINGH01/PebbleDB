package sstable

import (
	"path/filepath"
	"testing"
)

func TestBlockCacheHit(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cache.sst")

	w, err := NewWriter(path, 4096, 4)
	if err != nil {
		t.Fatal(err)
	}
	for _, kv := range []struct{ k, v string }{
		{"a", "1"}, {"b", "2"}, {"c", "3"},
	} {
		if err := w.Add([]byte(kv.k), []byte(kv.v), false); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}

	r, err := OpenReader(path, NewBlockCache(DefaultBlockCacheBytes))
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()

	entry := r.index[0]
	key := makeBlockCacheKey(r.fileID, entry.Offset)

	if _, ok := r.blockCache.get(key); ok {
		t.Fatal("cache should be cold before first read")
	}

	if _, err := r.readBlock(entry.Offset, entry.Length); err != nil {
		t.Fatal(err)
	}
	if _, ok := r.blockCache.get(key); !ok {
		t.Fatal("expected cache hit after readBlock")
	}

	if _, err := r.readBlock(entry.Offset, entry.Length); err != nil {
		t.Fatal(err)
	}
}
