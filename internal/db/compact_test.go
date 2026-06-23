package db

import (
	"fmt"
	"testing"
	"time"

	"github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/sstable"
)

func waitForCompaction(t *testing.T, db *DB, maxSST int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		db.mu.RLock()
		count := len(db.sstables)
		pending := db.hasPendingFlush()
		db.mu.RUnlock()
		if !pending && count <= maxSST {
			time.Sleep(50 * time.Millisecond)
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("compaction did not finish; still have >%d SSTables", maxSST)
}

func TestCompactionMergesDuplicateKeys(t *testing.T) {
	dir := t.TempDir()
	db, err := Open(Options{
		Dir:                   dir,
		MemtableSize: 8,
		CompactionThreshold:   2,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	for round := 0; round < 3; round++ {
		if err := db.Put([]byte("shared"), []byte(fmt.Sprintf("v%d", round))); err != nil {
			t.Fatal(err)
		}
		for i := 0; i < 4; i++ {
			key := fmt.Sprintf("k%d-%d", round, i)
			if err := db.Put([]byte(key), []byte("x")); err != nil {
				t.Fatal(err)
			}
		}
		waitForFlush(t, db)
	}

	waitForCompaction(t, db, 1)

	val, err := db.Get([]byte("shared"))
	if err != nil {
		t.Fatalf("shared key missing: %v", err)
	}
	if string(val) != "v2" {
		t.Errorf("shared = %q, want v2", val)
	}

	live := db.manifest.LiveIDs()
	if len(live) != 1 {
		t.Fatalf("expected 1 live SSTable in manifest, got %v", live)
	}
	db.mu.RLock()
	if len(db.sstables) != 1 {
		t.Fatalf("expected 1 in-memory SSTable, got %d", len(db.sstables))
	}
	db.mu.RUnlock()
}

func TestCompactionDropsDeletedKeys(t *testing.T) {
	dir := t.TempDir()
	db, err := Open(Options{
		Dir:                   dir,
		MemtableSize: 8,
		CompactionThreshold:   2,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	if err := db.Put([]byte("gone"), []byte("old")); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 4; i++ {
		if err := db.Put([]byte(fmt.Sprintf("f%02d", i)), []byte("x")); err != nil {
			t.Fatal(err)
		}
	}
	waitForFlush(t, db)

	if err := db.Delete([]byte("gone")); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 4; i++ {
		if err := db.Put([]byte(fmt.Sprintf("g%02d", i)), []byte("y")); err != nil {
			t.Fatal(err)
		}
	}
	waitForFlush(t, db)
	waitForCompaction(t, db, 1)

	_, err = db.Get([]byte("gone"))
	if err != ErrNotFound {
		t.Errorf("deleted key gone: err=%v, want ErrNotFound", err)
	}
}

func TestPartialCompactionReducesCount(t *testing.T) {
	dir := t.TempDir()
	db, err := Open(Options{
		Dir:                 dir,
		MemtableSize:        8,
		CompactionThreshold: 4,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	for round := 0; round < 4; round++ {
		for i := 0; i < 4; i++ {
			key := fmt.Sprintf("r%d-k%d", round, i)
			if err := db.Put([]byte(key), []byte("v")); err != nil {
				t.Fatal(err)
			}
		}
		waitForFlush(t, db)
	}

	waitForCompaction(t, db, 3)

	db.mu.RLock()
	count := len(db.sstables)
	db.mu.RUnlock()
	if count != 3 {
		t.Errorf("after one partial compaction want 3 SSTables, got %d", count)
	}

	val, err := db.Get([]byte("r3-k0"))
	if err != nil {
		t.Fatalf("key missing after partial compaction: %v", err)
	}
	if string(val) != "v" {
		t.Errorf("got %q, want v", val)
	}
}

func TestGetSurvivesCompactionWithHeldRefs(t *testing.T) {
	dir := t.TempDir()
	db, err := Open(Options{
		Dir:                 dir,
		MemtableSize:        8,
		CompactionThreshold: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	for round := 0; round < 3; round++ {
		if err := db.Put([]byte("stable"), []byte(fmt.Sprintf("v%d", round))); err != nil {
			t.Fatal(err)
		}
		for i := 0; i < 4; i++ {
			key := fmt.Sprintf("k%d-%d", round, i)
			if err := db.Put([]byte(key), []byte("x")); err != nil {
				t.Fatal(err)
			}
		}
		waitForFlush(t, db)
	}

	db.mu.RLock()
	readers := append([]*sstable.Reader(nil), db.sstables...)
	for _, r := range readers {
		r.Ref()
	}
	db.mu.RUnlock()
	defer func() {
		for _, r := range readers {
			r.Unref()
		}
	}()

	waitForCompaction(t, db, 1)

	for _, r := range readers {
		val, found, _, err := r.Get([]byte("stable"))
		if err != nil {
			t.Fatalf("held reader Get failed after compaction: %v", err)
		}
		if !found {
			t.Fatal("held reader should still find key from compacted SST")
		}
		if string(val) != "v2" {
			t.Fatalf("held reader value = %q, want v2", val)
		}
	}

	val, err := db.Get([]byte("stable"))
	if err != nil {
		t.Fatalf("live Get after compaction: %v", err)
	}
	if string(val) != "v2" {
		t.Fatalf("live value = %q, want v2", val)
	}
}
