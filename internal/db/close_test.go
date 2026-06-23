package db

import (
	"errors"
	"testing"
	"time"

	"github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/memtable"
	"github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/wal"
)

func TestCloseShutsDownWorkersOnWalSizeError(t *testing.T) {
	dir := t.TempDir()
	db, err := Open(Options{Dir: dir, CompactionThreshold: 1000})
	if err != nil {
		t.Fatal(err)
	}

	db.mu.Lock()
	db.active.Put([]byte("pending"), []byte("data"))
	db.mu.Unlock()

	if err := db.wal.Close(); err != nil {
		t.Fatal(err)
	}

	done := make(chan error, 1)
	go func() {
		done <- db.Close()
	}()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("Close() should report wal.Size error")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Close() blocked instead of shutting down background workers")
	}
}

func TestCloseBoundedWhenFlusherStuck(t *testing.T) {
	oldDrain := closeFlushDrainTimeout
	oldJoin := closeWorkerJoinTimeout
	closeFlushDrainTimeout = 50 * time.Millisecond
	closeWorkerJoinTimeout = 50 * time.Millisecond
	t.Cleanup(func() {
		closeFlushDrainTimeout = oldDrain
		closeWorkerJoinTimeout = oldJoin
	})

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

	done := make(chan error, 1)
	go func() {
		done <- database.Close()
	}()

	select {
	case err := <-done:
		if !errors.Is(err, ErrCloseFlushTimeout) {
			t.Fatalf("Close() = %v, want ErrCloseFlushTimeout", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Close() blocked past flush drain and worker join timeouts")
	}
}

func TestWalAppendFailurePreservesPendingBatch(t *testing.T) {
	dir := t.TempDir()
	db, err := Open(Options{Dir: dir})
	if err != nil {
		t.Fatal(err)
	}

	db.mu.Lock()
	db.pendingBatch = append(db.pendingBatch, wal.Record{
		Key:   []byte("k"),
		Value: []byte("v"),
	})
	db.batchSizeBytes = recordWireSize(db.pendingBatch[0])
	db.mu.Unlock()

	if err := db.wal.Close(); err != nil {
		t.Fatal(err)
	}

	if err := db.flushPendingBatch(); err == nil {
		t.Fatal("expected WAL append failure")
	}

	db.mu.Lock()
	pending := len(db.pendingBatch)
	db.mu.Unlock()
	if pending != 1 {
		t.Fatalf("pendingBatch len = %d, want 1 (no data loss)", pending)
	}

	db.mu.RLock()
	_, activeHas, _ := db.active.Get([]byte("k"))
	db.mu.RUnlock()
	if activeHas {
		t.Fatal("memtable should not contain record when WAL append failed")
	}

	_ = db.Close()
}
