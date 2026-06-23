package sstable

import (
	"testing"

	"github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/memtable"
)

func TestMergeReadersDedupesKeys(t *testing.T) {
	dir := t.TempDir()

	writeTable := func(name string, entries [][2]string) *Reader {
		t.Helper()
		path := dir + "/" + name
		mt := memtable.NewSkipList()
		for _, e := range entries {
			mt.Put([]byte(e[0]), []byte(e[1]))
		}
		w, err := NewWriter(path, 4096, uint(mt.Len()))
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
		r, err := OpenReader(path, nil)
		if err != nil {
			t.Fatal(err)
		}
		return r
	}

	old := writeTable("old.sst", [][2]string{{"a", "1"}, {"b", "old"}})
	newer := writeTable("new.sst", [][2]string{{"a", "2"}, {"c", "3"}})
	defer old.Close()
	defer newer.Close()

	outPath := dir + "/merged.sst"
	w, err := NewWriter(outPath, 4096, 4)
	if err != nil {
		t.Fatal(err)
	}
	if err := MergeReaders([]*Reader{old, newer}, w); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}

	merged, err := OpenReader(outPath, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer merged.Close()

	val, found, tomb, err := merged.Get([]byte("a"))
	if err != nil || !found || tomb || string(val) != "2" {
		t.Fatalf("a: val=%q found=%v tomb=%v err=%v", val, found, tomb, err)
	}
	val, found, tomb, err = merged.Get([]byte("b"))
	if err != nil || !found || tomb || string(val) != "old" {
		t.Fatalf("b: val=%q found=%v tomb=%v err=%v", val, found, tomb, err)
	}
}
