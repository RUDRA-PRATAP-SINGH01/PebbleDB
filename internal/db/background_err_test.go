package db

import (
	"errors"
	"os"
	"testing"
	"time"
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

func TestFlushErrorBlocksWrites(t *testing.T) {
	dir := t.TempDir()
	db, err := Open(Options{Dir: dir, CompactionThreshold: 1000})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	if err := db.Put([]byte("k"), []byte("v")); err != nil {
		t.Fatal(err)
	}
	if err := db.Sync(); err != nil {
		t.Fatal(err)
	}

	db.setBackgroundErr("flush", os.ErrPermission)

	if err := db.Put([]byte("k2"), []byte("v2")); err == nil {
		t.Fatal("Put should fail while flush background error is set")
	}

	val, err := db.Get([]byte("k"))
	if err != nil {
		t.Fatalf("Get should succeed while only flush is failing: %v", err)
	}
	if string(val) != "v" {
		t.Fatalf("got %q, want v", val)
	}
}

func TestWalBackgroundErrorBlocksWritesOnly(t *testing.T) {
	dir := t.TempDir()
	db, err := Open(Options{Dir: dir, CompactionThreshold: 1000})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	if err := db.Put([]byte("existing"), []byte("data")); err != nil {
		t.Fatal(err)
	}
	if err := db.Sync(); err != nil {
		t.Fatal(err)
	}

	db.setBackgroundErr("wal", os.ErrPermission)

	if err := db.Put([]byte("k"), []byte("v")); err == nil {
		t.Fatal("expected WAL background error on Put")
	}
	val, err := db.Get([]byte("existing"))
	if err != nil {
		t.Fatalf("Get should succeed during WAL background error: %v", err)
	}
	if string(val) != "data" {
		t.Fatalf("got %q, want data", val)
	}
	if _, err := db.Scan(nil, nil); err != nil {
		t.Fatalf("Scan should succeed during WAL background error: %v", err)
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

func TestCloseIncompleteWhenWalSizeFails(t *testing.T) {
	dir := t.TempDir()
	database, err := Open(Options{Dir: dir, CompactionThreshold: 1000})
	if err != nil {
		t.Fatal(err)
	}

	database.mu.Lock()
	database.active.Put([]byte("pending"), []byte("data"))
	database.mu.Unlock()

	if err := database.wal.Close(); err != nil {
		t.Fatal(err)
	}

	closeErr := database.Close()
	if !errors.Is(closeErr, ErrCloseIncomplete) {
		t.Fatalf("Close() = %v, want ErrCloseIncomplete", closeErr)
	}
	if database.manifest == nil {
		t.Fatal("manifest should remain open after incomplete close")
	}
	_ = database.manifest.Close()
}
