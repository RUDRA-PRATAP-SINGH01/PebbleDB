package db

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

const crashTestDirEnv = "PEBBLEDB_TEST_DIR"

func runCrashSubprocess(t *testing.T, testName, crashAt, dir string) {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^"+testName+"$", "-test.count=1")
	cmd.Env = append(os.Environ(),
		"PEBBLEDB_CRASH_TEST=1",
		"PEBBLEDB_CRASH_AT="+crashAt,
		crashTestDirEnv+"="+dir,
	)
	err := cmd.Run()
	exitErr, ok := err.(*exec.ExitError)
	if !ok {
		t.Fatalf("child process: %v", err)
	}
	if exitErr.ExitCode() != 2 {
		t.Fatalf("child exit code = %d, want 2 (crash point)", exitErr.ExitCode())
	}
}

func triggerFlush(db *DB, t *testing.T) {
	t.Helper()
	if err := db.flushPendingBatch(); err != nil {
		t.Fatal(err)
	}
	if err := db.Put([]byte("anchor"), []byte("x")); err != nil {
		t.Fatal(err)
	}
	if err := db.flushPendingBatch(); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 12; i++ {
		key := fmt.Sprintf("flush%02d", i)
		if err := db.Put([]byte(key), []byte("val")); err != nil {
			t.Fatal(err)
		}
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		db.mu.RLock()
		pending := len(db.pendingFlush)
		db.mu.RUnlock()
		if pending > 0 {
			time.Sleep(20 * time.Millisecond)
			continue
		}
		return
	}
	t.Fatal("flush did not start")
}

func TestCrashRecoveryFlushAfterManifest(t *testing.T) {
	if os.Getenv("PEBBLEDB_CRASH_TEST") == "1" {
		dir := os.Getenv(crashTestDirEnv)
		db, err := Open(Options{Dir: dir, MemtableSize: 64, CompactionThreshold: 100})
		if err != nil {
			fmt.Fprintf(os.Stderr, "open: %v\n", err)
			os.Exit(1)
		}
		if err := db.Put([]byte("survive"), []byte("yes")); err != nil {
			fmt.Fprintf(os.Stderr, "put: %v\n", err)
			os.Exit(1)
		}
		waitForFlushNoT(db)
		db.Close()
		return
	}

	dir, err := os.MkdirTemp("", "pebble-crash-flush-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	runCrashSubprocess(t, t.Name(), CrashAfterManifestNewFile, dir)

	db2, err := Open(Options{Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	defer db2.Close()

	val, err := db2.Get([]byte("survive"))
	if err != nil {
		t.Fatalf("Get after crash recovery: %v", err)
	}
	if string(val) != "yes" {
		t.Fatalf("got %q, want yes", val)
	}
}

func TestCrashRecoveryCompactAfterManifest(t *testing.T) {
	if os.Getenv("PEBBLEDB_CRASH_TEST") == "1" {
		dir := os.Getenv(crashTestDirEnv)
		db, err := Open(Options{Dir: dir, MemtableSize: 48, CompactionThreshold: 2})
		if err != nil {
			os.Exit(1)
		}
		for round := 0; round < 2; round++ {
			if err := db.Put([]byte("key"), []byte(fmt.Sprintf("v%d", round))); err != nil {
				os.Exit(1)
			}
			for i := 0; i < 6; i++ {
				k := fmt.Sprintf("r%d-%02d", round, i)
				if err := db.Put([]byte(k), []byte("x")); err != nil {
					os.Exit(1)
				}
			}
			waitForFlushNoT(db)
		}
		db.maybeTriggerCompaction()
		time.Sleep(2 * time.Second)
		db.Close()
		return
	}

	dir, err := os.MkdirTemp("", "pebble-crash-compact-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	runCrashSubprocess(t, t.Name(), CrashAfterManifestSetFileSet, dir)

	db2, err := Open(Options{Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	defer db2.Close()

	val, err := db2.Get([]byte("key"))
	if err != nil {
		t.Fatalf("Get after compaction crash: %v", err)
	}
	if string(val) != "v1" {
		t.Fatalf("key = %q, want v1", val)
	}
}

func waitForFlushNoT(db *DB) {
	_ = db.flushPendingBatch()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		db.mu.RLock()
		pending := len(db.pendingFlush)
		batchPending := len(db.pendingBatch) > 0
		sst := len(db.sstables)
		db.mu.RUnlock()
		if pending == 0 && !batchPending && sst > 0 {
			time.Sleep(50 * time.Millisecond)
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestCrashRecoveryFlushBoundaries(t *testing.T) {
	points := []string{
		CrashAfterSSTClose,
		CrashAfterManifestNewFile,
		CrashAfterWalFlushState,
		CrashAfterWalTruncate,
	}
	for _, point := range points {
		t.Run(point, func(t *testing.T) {
			if os.Getenv("PEBBLEDB_CRASH_TEST") == "1" {
				dir := os.Getenv(crashTestDirEnv)
				db, err := Open(Options{Dir: dir, MemtableSize: 64, CompactionThreshold: 100})
				if err != nil {
					os.Exit(1)
				}
				_ = db.Put([]byte("k"), []byte("v"))
				triggerFlush(db, t)
				db.Close()
				return
			}
			dir, err := os.MkdirTemp("", "pebble-crash-*")
			if err != nil {
				t.Fatal(err)
			}
			defer os.RemoveAll(dir)
			runCrashSubprocess(t, t.Name(), point, dir)
			db2, err := Open(Options{Dir: dir})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := db2.Get([]byte("k")); err != nil {
				t.Fatalf("recovery after %s: %v", point, err)
			}
			db2.Close()
		})
	}
}

func TestCrashRecoveryCompactBoundaries(t *testing.T) {
	points := []string{
		CrashAfterMergeClose,
		CrashAfterManifestSetFileSet,
		CrashAfterSSTablesUpdate,
		CrashAfterDeleteOldSSTs,
	}
	for _, point := range points {
		t.Run(point, func(t *testing.T) {
			if os.Getenv("PEBBLEDB_CRASH_TEST") == "1" {
				dir := os.Getenv(crashTestDirEnv)
				db, err := Open(Options{Dir: dir, MemtableSize: 48, CompactionThreshold: 2})
				if err != nil {
					os.Exit(1)
				}
				for round := 0; round < 2; round++ {
					_ = db.Put([]byte("k"), []byte("v"))
					for i := 0; i < 6; i++ {
						_ = db.Put([]byte(fmt.Sprintf("p%d-%d", round, i)), []byte("x"))
					}
					waitForFlushNoT(db)
				}
				db.maybeTriggerCompaction()
				time.Sleep(3 * time.Second)
				db.Close()
				return
			}
			dir, err := os.MkdirTemp("", "pebble-crash-compact-*")
			if err != nil {
				t.Fatal(err)
			}
			defer os.RemoveAll(dir)
			runCrashSubprocess(t, t.Name(), point, dir)
			db2, err := Open(Options{Dir: dir})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := db2.Get([]byte("k")); err != nil {
				t.Fatalf("recovery after %s: %v", point, err)
			}
			db2.Close()
		})
	}
}

func TestCrashRecoveryOrphanSSTIgnored(t *testing.T) {
	dir := t.TempDir()
	db1, err := Open(Options{Dir: dir, MemtableSize: 48, CompactionThreshold: 2})
	if err != nil {
		t.Fatal(err)
	}
	_ = db1.Put([]byte("A"), []byte("1"))
	for i := 0; i < 6; i++ {
		_ = db1.Put([]byte(fmt.Sprintf("f%02d", i)), []byte("x"))
	}
	waitForFlush(t, db1)
	_ = db1.Delete([]byte("A"))
	for i := 0; i < 6; i++ {
		_ = db1.Put([]byte(fmt.Sprintf("g%02d", i)), []byte("y"))
	}
	waitForFlush(t, db1)
	waitForCompaction(t, db1, 1)
	_ = db1.Close()

	orphan := filepath.Join(dir, "sst_00000001.sst")
	w, err := os.Create(orphan)
	if err != nil {
		t.Fatal(err)
	}
	w.Close()

	db2, err := Open(Options{Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	defer db2.Close()
	if _, err := db2.Get([]byte("A")); err != ErrNotFound {
		t.Fatalf("orphan SST should be ignored: err=%v", err)
	}
}
