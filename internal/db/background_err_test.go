package db

import (
	"errors"
	"os"
	"testing"
	"time"

	"github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/memtable"
)

func TestClearBackgroundErrOpIsScoped(t *testing.T) {
	db := &DB{bgErrs: newBackgroundErrStore()}
	db.setBackgroundErr("compaction", os.ErrPermission)
	db.setBackgroundErr("flush", os.ErrClosed)
	db.clearBackgroundErrOp("flush")

	bg := db.BackgroundError()
	if bg == nil {
		t.Fatal("compaction error should remain after clearing flush")
	}
	comp, ok := bg.(*BackgroundError)
	if ok {
		if comp.Op != "compaction" {
			t.Fatalf("got op %q, want compaction", comp.Op)
		}
	} else if !errors.Is(bg, os.ErrPermission) {
		t.Fatalf("BackgroundError() = %v, want compaction permission error", bg)
	}

	db.clearBackgroundErrOp("compaction")
	if db.BackgroundError() != nil {
		t.Fatal("all background errors should be cleared")
	}
}

func TestMultipleBackgroundErrorsVisible(t *testing.T) {
	db := &DB{bgErrs: newBackgroundErrStore()}
	db.setBackgroundErr("flush", os.ErrClosed)
	db.setBackgroundErr("compaction", os.ErrPermission)

	bg := db.BackgroundError()
	if bg == nil {
		t.Fatal("expected joined background errors")
	}

	ops := backgroundErrorOps(bg)
	if !ops["flush"] || !ops["compaction"] {
		t.Fatalf("joined error missing ops: %v (%v)", ops, bg)
	}
}

func backgroundErrorOps(err error) map[string]bool {
	ops := make(map[string]bool)
	forEachBackgroundError(err, func(be *BackgroundError) {
		ops[be.Op] = true
	})
	return ops
}

func forEachBackgroundError(err error, fn func(*BackgroundError)) {
	if err == nil {
		return
	}
	if joined, ok := err.(interface{ Unwrap() []error }); ok {
		for _, e := range joined.Unwrap() {
			forEachBackgroundError(e, fn)
		}
		return
	}
	var be *BackgroundError
	if errors.As(err, &be) {
		fn(be)
	}
}

func TestFlushErrorDoesNotBlockWrites(t *testing.T) {
	dir := t.TempDir()
	db, err := Open(Options{Dir: dir, CompactionThreshold: 1000})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	db.setBackgroundErr("flush", os.ErrPermission)

	if err := db.Put([]byte("k"), []byte("v")); err != nil {
		t.Fatalf("Put should succeed while only flush is failing: %v", err)
	}

	val, err := db.Get([]byte("k"))
	if err != nil {
		t.Fatalf("Get should succeed while only flush is failing: %v", err)
	}
	if string(val) != "v" {
		t.Fatalf("got %q, want v", val)
	}
}

func TestWalBackgroundErrorBlocksForegroundOps(t *testing.T) {
	dir := t.TempDir()
	db, err := Open(Options{Dir: dir, CompactionThreshold: 1000})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	db.setBackgroundErr("wal", os.ErrPermission)

	if err := db.Put([]byte("k"), []byte("v")); err == nil {
		t.Fatal("expected WAL background error on Put")
	}
	if _, err := db.Get([]byte("k")); err == nil {
		t.Fatal("expected WAL background error on Get")
	}
	if _, err := db.Scan(nil, nil); err == nil {
		t.Fatal("expected WAL background error on Scan")
	}
}

func TestBackgroundErrorSurfacesWalToCaller(t *testing.T) {
	dir := t.TempDir()
	db, err := Open(Options{Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	db.setBackgroundErr("wal", os.ErrPermission)

	err = db.Put([]byte("k"), []byte("v"))
	if err == nil {
		t.Fatal("expected background error on Put")
	}
	var bg *BackgroundError
	if !errors.As(err, &bg) || bg.Op != "wal" {
		t.Fatalf("got %T %v, want *BackgroundError wal", err, err)
	}

	if db.BackgroundError() == nil {
		t.Error("BackgroundError() should report stored error")
	}
}

func TestCompactionDisabledWithNegativeThreshold(t *testing.T) {
	dir := t.TempDir()
	db, err := Open(Options{
		Dir:                 dir,
		MemtableSize:        8,
		CompactionThreshold: -1,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	for round := 0; round < 3; round++ {
		for i := 0; i < 6; i++ {
			key := []byte{byte('a' + round), byte('0' + i)}
			if err := db.Put(key, []byte("v")); err != nil {
				t.Fatal(err)
			}
		}
		waitForFlush(t, db)
	}

	db.mu.RLock()
	count := len(db.sstables)
	db.mu.RUnlock()
	if count < 3 {
		t.Fatalf("expected multiple SSTables without compaction, got %d", count)
	}

	time.Sleep(100 * time.Millisecond)

	db.mu.RLock()
	after := len(db.sstables)
	db.mu.RUnlock()
	if after != count {
		t.Fatalf("compaction should be disabled: count changed %d -> %d", count, after)
	}
}

func TestCloseReturnsTimeoutWhenFlushStuck(t *testing.T) {
	oldTimeout := closeFlushDrainTimeout
	closeFlushDrainTimeout = 100 * time.Millisecond
	t.Cleanup(func() { closeFlushDrainTimeout = oldTimeout })

	dir := t.TempDir()
	database, err := Open(Options{Dir: dir, CompactionThreshold: 1000})
	if err != nil {
		t.Fatal(err)
	}

	database.mu.Lock()
	database.pendingFlush = append(database.pendingFlush, flushQueueEntry{
		mem:       memtable.NewSkipList(),
		walCutoff: 0,
	})
	database.mu.Unlock()

	if err := database.manifest.Close(); err != nil {
		t.Fatal(err)
	}
	database.manifest = nil

	err = database.Close()
	if !errors.Is(err, ErrCloseFlushTimeout) {
		t.Fatalf("Close() = %v, want ErrCloseFlushTimeout", err)
	}
}
