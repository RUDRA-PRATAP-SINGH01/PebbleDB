package db

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
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

func TestFlushNeverAbandonsQueueEntry(t *testing.T) {
	if os.Getenv("PEBBLE_FLUSH_ABANDON_CHILD") == "1" {
		dir := os.Getenv(crashTestDirEnv)
		if dir == "" {
			fmt.Fprintln(os.Stderr, "missing test dir")
			os.Exit(1)
		}
		db, err := Open(Options{Dir: dir, CompactionThreshold: 1000})
		if err != nil {
			fmt.Fprintf(os.Stderr, "open: %v\n", err)
			os.Exit(1)
		}

		imm := memtable.NewSkipList()
		imm.Put([]byte("stuck"), []byte("key"))

		db.mu.Lock()
		db.pendingFlush = append(db.pendingFlush,
			flushQueueEntry{mem: imm, walCutoff: 0},
		)
		db.mu.Unlock()

		if err := db.manifest.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "manifest close: %v\n", err)
			os.Exit(1)
		}
		db.manifest = nil
		db.notifyFlushForce()

		deadline := time.Now().Add(500 * time.Millisecond)
		for time.Now().Before(deadline) {
			db.mu.RLock()
			n := len(db.pendingFlush)
			db.mu.RUnlock()
			if n != 1 {
				fmt.Fprintf(os.Stderr, "pendingFlush len = %d, want 1", n)
				os.Exit(1)
			}
			time.Sleep(20 * time.Millisecond)
		}
		if db.BackgroundError() == nil {
			fmt.Fprintln(os.Stderr, "expected flush background error")
			os.Exit(1)
		}
		os.Exit(0)
	}

	dir := t.TempDir()
	cmd := exec.Command(os.Args[0], "-test.run=^TestFlushNeverAbandonsQueueEntry$", "-test.count=1")
	cmd.Env = append(os.Environ(),
		"PEBBLE_FLUSH_ABANDON_CHILD=1",
		crashTestDirEnv+"="+dir,
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("child failed: %v\n%s", err, out)
	}
}

func TestSyncWaitsForInFlightBatch(t *testing.T) {
	dir := t.TempDir()
	db, err := Open(Options{
		Dir:             dir,
		MemtableSize:    1 << 30,
		BatchFlushDelay: 50 * time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	started := make(chan struct{})
	release := make(chan struct{})
	wal.BeforeBatchSync = func() {
		select {
		case <-started:
		default:
			close(started)
		}
		<-release
	}
	t.Cleanup(func() {
		wal.BeforeBatchSync = nil
		select {
		case <-release:
		default:
			close(release)
		}
	})

	if err := db.Put([]byte("inflight"), []byte("v")); err != nil {
		t.Fatal(err)
	}

	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for WAL batch to reach fsync")
	}

	syncDone := make(chan error, 1)
	go func() {
		syncDone <- db.Sync()
	}()

	select {
	case err := <-syncDone:
		t.Fatalf("Sync returned early: %v", err)
	case <-time.After(200 * time.Millisecond):
	}

	close(release)

	select {
	case err := <-syncDone:
		if err != nil {
			t.Fatalf("Sync: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Sync did not complete after releasing blocked fsync")
	}

	var replayed int
	if err := wal.Replay(filepath.Join(dir, "wal.log"), func(wal.Record) error {
		replayed++
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if replayed != 1 {
		t.Fatalf("replayed %d records after Sync, want 1", replayed)
	}
}

func TestWALAppendErrorBlocksWrites(t *testing.T) {
	dir := t.TempDir()
	db, err := Open(Options{Dir: dir, SyncWrites: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		db.mu.Lock()
		db.closed = true
		db.mu.Unlock()
		if db.manifest != nil {
			_ = db.manifest.Close()
		}
		if db.dirLock != nil {
			releaseDirLock(db.dirLock)
			db.dirLock = nil
		}
	})

	if err := db.Put([]byte("before"), []byte("ok")); err != nil {
		t.Fatal(err)
	}

	if err := db.wal.Close(); err != nil {
		t.Fatal(err)
	}

	err = db.Put([]byte("after"), []byte("fail"))
	if err == nil {
		t.Fatal("Put after WAL close should fail")
	}
	if db.BackgroundError() == nil {
		t.Fatalf("Put error = %v, want WAL background failure", err)
	}

	if val, getErr := db.Get([]byte("before")); getErr != nil || string(val) != "ok" {
		t.Fatalf("Get(before) = %q, %v — reads must continue", val, getErr)
	}
}
