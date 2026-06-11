package db

import (
	"fmt"
	"testing"
)

const scanTestThreshold int64 = 8

func collectScan(t *testing.T, it *ScanIterator) map[string]string {
	t.Helper()
	out := make(map[string]string)
	for it.Valid() {
		out[string(it.Key())] = string(it.Value())
		if err := it.Next(); err != nil {
			t.Fatal(err)
		}
	}
	return out
}

func TestScanActiveOnly(t *testing.T) {
	dir := t.TempDir()
	db, err := Open(Options{Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	db.Put([]byte("a"), []byte("1"))
	db.Put([]byte("b"), []byte("2"))

	it, err := db.Scan(nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer it.Close()

	got := collectScan(t, it)
	if got["a"] != "1" || got["b"] != "2" {
		t.Fatalf("got %v", got)
	}
}

func TestScanNewestWinsAcrossLayers(t *testing.T) {
	dir := t.TempDir()
	db, err := Open(Options{Dir: dir, MemtableSize: scanTestThreshold})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	db.Put([]byte("shared"), []byte("sst"))
	for i := 0; i < 6; i++ {
		db.Put([]byte(fmt.Sprintf("f%02d", i)), []byte("x"))
	}
	waitForFlush(t, db)

	db.Put([]byte("shared"), []byte("active"))

	it, err := db.Scan(nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer it.Close()

	got := collectScan(t, it)
	if got["shared"] != "active" {
		t.Fatalf("shared = %q, want active", got["shared"])
	}
}

func TestScanSkipsTombstones(t *testing.T) {
	dir := t.TempDir()
	db, err := Open(Options{Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	db.Put([]byte("gone"), []byte("old"))
	db.Delete([]byte("gone"))
	db.Put([]byte("stay"), []byte("yes"))

	it, err := db.Scan(nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer it.Close()

	got := collectScan(t, it)
	if _, ok := got["gone"]; ok {
		t.Fatal("tombstoned key should not appear in scan")
	}
	if got["stay"] != "yes" {
		t.Fatalf("stay = %q", got["stay"])
	}
}

func TestScanRange(t *testing.T) {
	dir := t.TempDir()
	db, err := Open(Options{Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	for _, k := range []string{"a", "b", "c", "d"} {
		db.Put([]byte(k), []byte(k))
	}

	it, err := db.Scan([]byte("b"), []byte("d"))
	if err != nil {
		t.Fatal(err)
	}
	defer it.Close()

	got := collectScan(t, it)
	if len(got) != 2 || got["b"] != "b" || got["c"] != "c" {
		t.Fatalf("got %v, want b and c only", got)
	}
}

func TestScanEmptyRange(t *testing.T) {
	dir := t.TempDir()
	db, err := Open(Options{Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	db.Put([]byte("a"), []byte("1"))

	it, err := db.Scan([]byte("m"), []byte("z"))
	if err != nil {
		t.Fatal(err)
	}
	defer it.Close()

	if it.Valid() {
		t.Fatal("expected empty range")
	}
}

func TestScanAfterFlushAndCompact(t *testing.T) {
	dir := t.TempDir()
	db, err := Open(Options{
		Dir:                 dir,
		MemtableSize:        scanTestThreshold,
		CompactionThreshold: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	for round := 0; round < 3; round++ {
		db.Put([]byte("k"), []byte(fmt.Sprintf("v%d", round)))
		for i := 0; i < 4; i++ {
			db.Put([]byte(fmt.Sprintf("p%d-%d", round, i)), []byte("x"))
		}
		waitForFlush(t, db)
	}
	waitForCompaction(t, db, 1)

	it, err := db.Scan(nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer it.Close()

	got := collectScan(t, it)
	if got["k"] != "v2" {
		t.Fatalf("k = %q, want v2", got["k"])
	}
}
