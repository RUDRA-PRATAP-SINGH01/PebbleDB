package db

import (
	"testing"

	"github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/memtable"
	"github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/sstable"
)

func writeTestSST(t *testing.T, path string, entries [][2]string) {
	t.Helper()
	mt := memtable.NewSkipList()
	for _, e := range entries {
		mt.Put([]byte(e[0]), []byte(e[1]))
	}
	w, err := sstable.NewWriter(path, 4096, uint(mt.Len()))
	if err != nil {
		t.Fatal(err)
	}
	it := mt.Iterator()
	for it.Valid() {
		if err := w.Add(it.Key(), it.Value(), it.IsTombstone()); err != nil {
			t.Fatal(err)
		}
		it.Next()
	}
	it.Close()
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestLookupSSTReadersSkipsClosed(t *testing.T) {
	dir := t.TempDir()
	olderPath := dir + "/older.sst"
	newerPath := dir + "/newer.sst"

	writeTestSST(t, olderPath, [][2]string{{"k", "v1"}})
	writeTestSST(t, newerPath, [][2]string{{"k", "v2"}})

	older, err := sstable.OpenReader(olderPath, nil)
	if err != nil {
		t.Fatal(err)
	}
	newer, err := sstable.OpenReader(newerPath, nil)
	if err != nil {
		t.Fatal(err)
	}

	readers := []*sstable.Reader{older, newer}

	if err := newer.Discard(); err != nil {
		t.Fatal(err)
	}

	for _, r := range readers {
		r.Ref()
	}
	defer func() {
		for _, r := range readers {
			r.Unref()
			_ = r.Close()
		}
	}()

	val, found, err := lookupSSTReaders(readers, []byte("k"))
	if err != nil {
		t.Fatalf("lookupSSTReaders: %v", err)
	}
	if !found {
		t.Fatal("expected key in older SST")
	}
	if string(val) != "v1" {
		t.Fatalf("got %q, want v1", val)
	}
}
