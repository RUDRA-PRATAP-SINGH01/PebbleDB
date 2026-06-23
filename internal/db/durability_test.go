package db

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/memtable"
	"github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/wal"
)

func TestSyncPersistsPendingBatch(t *testing.T) {
	dir := t.TempDir()
	db, err := Open(Options{Dir: dir, MemtableSize: 1 << 30})
	if err != nil {
		t.Fatal(err)
	}

	if err := db.Put([]byte("dur"), []byte("able")); err != nil {
		t.Fatal(err)
	}
	if err := db.Sync(); err != nil {
		t.Fatal(err)
	}

	var replayed int
	if err := wal.Replay(filepath.Join(dir, "wal.log"), func(wal.Record) error {
		replayed++
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if replayed != 1 {
		t.Fatalf("replayed %d records, want 1 after Sync", replayed)
	}

	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestSyncWritesOptionWaitsForFsync(t *testing.T) {
	dir := t.TempDir()
	db, err := Open(Options{Dir: dir, SyncWrites: true})
	if err != nil {
		t.Fatal(err)
	}

	if err := db.Put([]byte("k"), []byte("v")); err != nil {
		t.Fatal(err)
	}

	db.mu.RLock()
	pending := len(db.pendingBatch)
	db.mu.RUnlock()
	if pending != 0 {
		t.Fatalf("pendingBatch len = %d after SyncWrites Put, want 0", pending)
	}

	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestOpenRejectsSecondProcessLock(t *testing.T) {
	dir := t.TempDir()
	db1, err := Open(Options{Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	defer db1.Close()

	_, err = Open(Options{Dir: dir})
	if !errors.Is(err, ErrDatabaseLocked) {
		t.Fatalf("second Open() = %v, want ErrDatabaseLocked", err)
	}
}

func TestOrphanSSTQuarantined(t *testing.T) {
	dir := t.TempDir()
	db1, err := Open(Options{Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	if err := db1.Put([]byte("k"), []byte("v")); err != nil {
		t.Fatal(err)
	}
	if err := db1.Sync(); err != nil {
		t.Fatal(err)
	}
	if err := db1.Close(); err != nil {
		t.Fatal(err)
	}

	orphan := filepath.Join(dir, "sst_00000099.sst")
	if err := os.WriteFile(orphan, []byte("orphan"), 0644); err != nil {
		t.Fatal(err)
	}

	db2, err := Open(Options{Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	defer db2.Close()

	if _, err := os.Stat(orphan); !os.IsNotExist(err) {
		t.Fatal("orphan SST should be moved out of data directory")
	}
	qpath := filepath.Join(quarantineDir(dir), "sst_00000099.sst")
	if _, err := os.Stat(qpath); err != nil {
		t.Fatalf("quarantined SST missing: %v", err)
	}
}

func TestOpenSkipsMalformedSSTFilename(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "sst_badname.sst"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	db, err := Open(Options{Dir: dir})
	if err != nil {
		t.Fatalf("Open with malformed SST name should succeed: %v", err)
	}
	defer db.Close()
}

func TestFlushRetryCapUnblocksQueue(t *testing.T) {
	oldMax := maxFlushRetries
	maxFlushRetries = 2
	t.Cleanup(func() { maxFlushRetries = oldMax })

	dir := t.TempDir()
	db, err := Open(Options{Dir: dir, CompactionThreshold: 1000})
	if err != nil {
		t.Fatal(err)
	}

	db.mu.Lock()
	db.pendingFlush = append(db.pendingFlush,
		flushQueueEntry{mem: memtable.NewSkipList(), walCutoff: 0},
		flushQueueEntry{mem: memtable.NewSkipList(), walCutoff: 0},
	)
	db.mu.Unlock()

	if err := db.manifest.Close(); err != nil {
		t.Fatal(err)
	}
	db.manifest = nil

	db.notifyFlushForce()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		db.mu.RLock()
		n := len(db.pendingFlush)
		db.mu.RUnlock()
		if n == 0 {
			_ = db.Close()
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	_ = db.Close()
	t.Fatal("pendingFlush queue did not drain after retry cap")
}
