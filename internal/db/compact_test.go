package db

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func waitForCompaction(t *testing.T, db *DB, maxSST int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		db.mu.RLock()
		count := len(db.sstables)
		imm := db.immutable
		db.mu.RUnlock()
		if imm == nil && count <= maxSST {
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
		MemtableSizeThreshold: 8,
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

	matches, err := filepath.Glob(filepath.Join(dir, "sst_*.sst"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 1 {
		t.Errorf("expected 1 SSTable after compaction, got %d: %v", len(matches), matches)
	}
}

func TestCompactionDropsDeletedKeys(t *testing.T) {
	dir := t.TempDir()
	db, err := Open(Options{
		Dir:                   dir,
		MemtableSizeThreshold: 8,
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

func TestBackgroundErrorSurfacesToCaller(t *testing.T) {
	dir := t.TempDir()
	db, err := Open(Options{Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	db.setBackgroundErr("flush", os.ErrPermission)

	err = db.Put([]byte("k"), []byte("v"))
	if err == nil {
		t.Fatal("expected background error on Put")
	}
	bg, ok := err.(*BackgroundError)
	if !ok || bg.Op != "flush" {
		t.Fatalf("got %T %v, want *BackgroundError flush", err, err)
	}

	if db.BackgroundError() == nil {
		t.Error("BackgroundError() should report stored error")
	}
}
