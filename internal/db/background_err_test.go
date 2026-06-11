package db

import (
	"errors"
	"os"
	"testing"
	"time"

	"github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/memtable"
)

func TestClearBackgroundErrOpIsScoped(t *testing.T) {
	db := &DB{}
	db.setBackgroundErr("compaction", os.ErrPermission)
	db.clearBackgroundErrOp("flush")
	if db.BackgroundError() == nil {
		t.Fatal("flush clear should not remove compaction error")
	}

	db.clearBackgroundErrOp("compaction")
	if db.BackgroundError() != nil {
		t.Fatal("compaction clear should remove compaction error")
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
