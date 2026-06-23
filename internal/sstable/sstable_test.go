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

	w, err := NewWriter(path, 4096, uint(mt.Len()))
	if err != nil {
		t.Fatal(err)
	}
	it := mt.Iterator()
	defer it.Close()
	for it.Valid() {
		if err := w.Add(it.Key(), it.Value(), it.IsTombstone()); err != nil {
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

func TestSSTableRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/test.sst"

	mt := memtable.NewSkipList()
	mt.Put([]byte("a"), []byte("1"))
	mt.Put([]byte("b"), []byte("2"))
	mt.Delete([]byte("c"))

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
	defer r.Close()

	val, found, tomb, err := r.Get([]byte("a"))
	if err != nil {
		t.Fatal(err)
	}
	if !found || tomb || string(val) != "1" {
		t.Errorf("a: got val=%q, found=%v, tomb=%v; want 1, true, false", val, found, tomb)
	}

	_, found, tomb, err = r.Get([]byte("c"))
	if err != nil {
		t.Fatal(err)
	}
	if !found || !tomb {
		t.Errorf("c: found=%v, tomb=%v; want true, true", found, tomb)
	}
}

func TestSSTableBloomFilterSkip(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/test.sst"

	mt := memtable.NewSkipList()
	mt.Put([]byte("a"), []byte("1"))

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
	defer r.Close()

	if r.MayContain([]byte("not-in-table")) {
		_, found, _, err := r.Get([]byte("not-in-table"))
		if err != nil {
			t.Fatal(err)
		}
		if found {
			t.Error("key not in SSTable should not be found")
		}
	}
}

func TestDiscardRespectsRefs(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/test.sst"

	mt := memtable.NewSkipList()
	mt.Put([]byte("key"), []byte("value"))

	w, err := NewWriter(path, 4096, 1)
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

	r.Ref()
	if err := r.Discard(); err != nil {
		t.Fatal(err)
	}

	val, found, _, err := r.Get([]byte("key"))
	if err != nil {
		t.Fatalf("Get during held ref after Discard: %v", err)
	}
	if !found || string(val) != "value" {
		t.Fatalf("key = %q found=%v, want value true", val, found)
	}

	r.Unref()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("discarded file should be removed after last Unref, stat err=%v", err)
	}
}
