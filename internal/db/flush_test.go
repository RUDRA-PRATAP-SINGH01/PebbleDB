package db

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"
)

func waitForFlush(t *testing.T, db *DB) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		db.mu.RLock()
		imm := db.immutable
		db.mu.RUnlock()
		if imm == nil {
			time.Sleep(50 * time.Millisecond)
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("flush did not complete in time")
}

func TestFlushToSSTable(t *testing.T) {
	dir := t.TempDir()
	db, err := Open(Options{Dir: dir, MemtableSizeThreshold: 64})
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
	db, err := Open(Options{Dir: dir, MemtableSizeThreshold: 64})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	if err := db.Put([]byte("x"), []byte("100")); err != nil {
		t.Fatal(err)
	}
	waitForFlush(t, db)

	if err := db.Delete([]byte("x")); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 20; i++ {
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

func TestReopenLoadsSSTables(t *testing.T) {
	dir := t.TempDir()

	db1, err := Open(Options{Dir: dir, MemtableSizeThreshold: 64})
	if err != nil {
		t.Fatal(err)
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
