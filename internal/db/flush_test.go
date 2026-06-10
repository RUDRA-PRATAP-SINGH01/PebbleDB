package db

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/wal"
)

const flushTestThreshold int64 = 8

func waitForFlush(t *testing.T, db *DB) {
	t.Helper()
	db.mu.RLock()
	startSST := len(db.sstables)
	db.mu.RUnlock()

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		db.mu.RLock()
		imm := db.immutable
		sstCount := len(db.sstables)
		db.mu.RUnlock()

		if imm == nil && sstCount > startSST {
			time.Sleep(50 * time.Millisecond)
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("flush did not complete in time")
}

func TestFlushToSSTable(t *testing.T) {
	dir := t.TempDir()
	db, err := Open(Options{Dir: dir, MemtableSizeThreshold: flushTestThreshold})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	for i := 0; i < 20; i++ {
		key := fmt.Sprintf("key%02d", i)
		if err := db.Put([]byte(key), []byte("val")); err != nil {
			t.Fatal(err)
		}
	}

	waitForFlush(t, db)

	val, err := db.Get([]byte("key00"))
	if err != nil {
		t.Fatalf("key00 not found after flush: %v", err)
	}
	if string(val) != "val" {
		t.Errorf("key00 = %q, want val", val)
	}

	matches, err := filepath.Glob(filepath.Join(dir, "sst_*.sst"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) == 0 {
		t.Error("expected at least one SSTable file after flush")
	}
}

func TestFlushTombstone(t *testing.T) {
	dir := t.TempDir()
	db, err := Open(Options{Dir: dir, MemtableSizeThreshold: flushTestThreshold})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	for i := 0; i < 5; i++ {
		if err := db.Put([]byte(fmt.Sprintf("pre%02d", i)), []byte("v")); err != nil {
			t.Fatal(err)
		}
	}
	waitForFlush(t, db)

	if err := db.Delete([]byte("x")); err != nil {
		t.Fatal(err)
	}
	if err := db.Put([]byte("x"), []byte("100")); err != nil {
		t.Fatal(err)
	}
	if err := db.Delete([]byte("x")); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 10; i++ {
		key := fmt.Sprintf("fill%02d", i)
		if err := db.Put([]byte(key), []byte("v")); err != nil {
			t.Fatal(err)
		}
	}
	waitForFlush(t, db)

	_, err = db.Get([]byte("x"))
	if err != ErrNotFound {
		t.Errorf("deleted key x: got err=%v, want ErrNotFound", err)
	}
}

func TestDeleteShadowsSSTableBeforeFlush(t *testing.T) {
	dir := t.TempDir()
	db, err := Open(Options{Dir: dir, MemtableSizeThreshold: flushTestThreshold})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	for i := 0; i < 5; i++ {
		if err := db.Put([]byte(fmt.Sprintf("p%02d", i)), []byte("v")); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Put([]byte("x"), []byte("old")); err != nil {
		t.Fatal(err)
	}
	waitForFlush(t, db)

	if err := db.Delete([]byte("x")); err != nil {
		t.Fatal(err)
	}

	_, err = db.Get([]byte("x"))
	if err != ErrNotFound {
		t.Errorf("tombstone in active should hide SST value: err=%v", err)
	}
}

func TestWALPreservedAfterFlush(t *testing.T) {
	dir := t.TempDir()
	// Threshold 64: first batch triggers flush; single after-flush write stays in active.
	db, err := Open(Options{Dir: dir, MemtableSizeThreshold: 64})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	for i := 0; i < 5; i++ {
		if err := db.Put([]byte(fmt.Sprintf("k%02d", i)), []byte("val")); err != nil {
			t.Fatal(err)
		}
	}
	waitForFlush(t, db)

	if err := db.Put([]byte("after-flush"), []byte("survives")); err != nil {
		t.Fatal(err)
	}

	walPath := filepath.Join(dir, "wal.log")
	var keys []string
	err = wal.Replay(walPath, func(rec wal.Record) error {
		keys = append(keys, string(rec.Key))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(keys) != 1 || keys[0] != "after-flush" {
		t.Errorf("WAL after flush = %v, want only [after-flush]", keys)
	}

	val, err := db.Get([]byte("after-flush"))
	if err != nil {
		t.Fatalf("key not readable: %v", err)
	}
	if string(val) != "survives" {
		t.Errorf("got %q, want survives", val)
	}
}

func TestReopenLoadsSSTables(t *testing.T) {
	dir := t.TempDir()

	db1, err := Open(Options{Dir: dir, MemtableSizeThreshold: flushTestThreshold})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 5; i++ {
		if err := db1.Put([]byte(fmt.Sprintf("k%02d", i)), []byte("ok")); err != nil {
			t.Fatal(err)
		}
	}
	if err := db1.Put([]byte("persist"), []byte("ok")); err != nil {
		t.Fatal(err)
	}
	waitForFlush(t, db1)
	if err := db1.Close(); err != nil {
		t.Fatal(err)
	}

	db2, err := Open(Options{Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	defer db2.Close()

	val, err := db2.Get([]byte("persist"))
	if err != nil {
		t.Fatalf("key not found after reopen: %v", err)
	}
	if string(val) != "ok" {
		t.Errorf("got %q, want ok", val)
	}

	matches, _ := filepath.Glob(filepath.Join(dir, "sst_*.sst"))
	if len(matches) == 0 {
		t.Error("expected SSTable on disk")
	}
}
