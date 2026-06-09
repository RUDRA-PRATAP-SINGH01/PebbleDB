package sstable

import (
	"os"
	"testing"

	"github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/memtable"
)

func TestSSTableWriter(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/test.sst"

	// Build a memtable with some data
	mt := memtable.NewSkipList()
	mt.Put([]byte("b"), []byte("2"))
	mt.Put([]byte("a"), []byte("1"))
	mt.Put([]byte("c"), []byte("3"))
	mt.Delete([]byte("d")) // tombstone

	w, err := NewWriter(path, 4096)
	if err != nil {
		t.Fatal(err)
	}
	it := mt.Iterator()
	defer it.Close()
	for it.Valid() {
		if it.IsTombstone() {
			// We'll write tombstone as empty value (or special marker)
			// For now, just write empty value – but tombstones need to be stored.
			// We'll store tombstone as a zero-length value? Actually we need a flag.
			// Let's keep it simple: store empty value and rely on tombstone flag later.
			// We'll add a tombstone flag in the future.
		}
		if err := w.Add(it.Key(), it.Value()); err != nil {
			t.Fatal(err)
		}
		it.Next()
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}

	// Check file exists and is non‑empty
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() == 0 {
		t.Error("SSTable file is empty")
	}
}
