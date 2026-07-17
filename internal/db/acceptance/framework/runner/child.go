package runner

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/db"
	"github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/db/acceptance/framework/dataset"
)

// PebbleWriter adapts *db.DB to interfaces.LogicalWriter.
type PebbleWriter struct {
	instance *db.DB
}

// NewPebbleWriter wraps a DB handle.
func NewPebbleWriter(instance *db.DB) *PebbleWriter {
	return &PebbleWriter{instance: instance}
}

func (w *PebbleWriter) Put(key, value []byte) error { return w.instance.Put(key, value) }
func (w *PebbleWriter) Delete(key []byte) error     { return w.instance.Delete(key) }
func (w *PebbleWriter) Sync() error                 { return w.instance.Sync() }
func (w *PebbleWriter) Close() error                { return w.instance.Close() }

// Flush forces memtable → SST (acceptance crash points live on this path).
func (w *PebbleWriter) Flush() error {
	return w.instance.ForceMemtableFlush()
}

// replayExpectedState re-applies the oracle's logical state as a second write
// pass: live keys are Put with their oracle value and tombstoned keys are
// Deleted again. Because every write is idempotent with respect to the oracle
// (identical final value / tombstone status), the logical state is unchanged
// while a second in-memory generation is produced. Flushing after this pass
// yields a second SSTable, which is the precondition for exercising the
// compaction crash points.
func replayExpectedState(writer *PebbleWriter, expected *dataset.MapExpectedState) error {
	for _, key := range expected.Keys() {
		snap, ok := expected.Get(key)
		if !ok {
			continue
		}
		if snap.Tombstone {
			if err := writer.Delete(key); err != nil {
				return err
			}
			continue
		}
		if err := writer.Put(key, snap.Value); err != nil {
			return err
		}
	}
	return writer.Sync()
}

// RunChildProcessMain is the ATF child entrypoint. It must:
//  1. write dataset
//  2. persist expected_state.json (before any crash)
//  3. drive the engine to the requested PEBBLEDB_CRASH_AT crash point:
//     - flush_* points are reached by ForceMemtableFlush
//     - compact_* points are reached by producing two SSTables and then
//     ForceCompaction (see replayExpectedState)
//  4. clear crash env before Close so shutdown flush cannot re-trigger crash
func RunChildProcessMain() {
	scenarioID := os.Getenv("PEBBLEDB_SCENARIO_ID")
	fmt.Fprintf(os.Stderr, "atf-child: scenario=%s\n", scenarioID)

	testDir := os.Getenv("PEBBLEDB_TEST_DIR")
	if testDir == "" {
		fmt.Fprintf(os.Stderr, "atf-child: missing PEBBLEDB_TEST_DIR\n")
		os.Exit(1)
	}

	memtableSize := parseInt64Env("PEBBLEDB_MEMTABLE_SIZE", 4<<20)
	compactionThreshold := int(parseInt64Env("PEBBLEDB_COMPACTION_THRESHOLD", 4))
	syncWrites, _ := strconv.ParseBool(os.Getenv("PEBBLEDB_SYNC_WRITES"))
	seed := parseInt64Env("PEBBLEDB_SEED", 12345)
	keyCount := int(parseInt64Env("PEBBLEDB_KEY_COUNT", 100))
	overwriteCount := int(parseInt64Env("PEBBLEDB_OVERWRITE_COUNT", 10))
	tombstoneEvery := int(parseInt64Env("PEBBLEDB_TOMBSTONE_EVERY", 5))
	doFlush := os.Getenv("PEBBLEDB_FORCE_FLUSH") != "0"
	crashAt := os.Getenv("PEBBLEDB_CRASH_AT")

	instance, err := db.Open(db.Options{
		Dir:                 testDir,
		MemtableSize:        memtableSize,
		CompactionThreshold: compactionThreshold,
		SyncWrites:          syncWrites,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "atf-child: open failed: %v\n", err)
		os.Exit(1)
	}

	writer := NewPebbleWriter(instance)
	gen := dataset.NewSequentialGenerator(seed, keyCount, overwriteCount, tombstoneEvery)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	expected, err := gen.Generate(ctx, writer)
	if err != nil {
		_ = instance.Close()
		fmt.Fprintf(os.Stderr, "atf-child: generate failed: %v\n", err)
		os.Exit(1)
	}
	expected.ScenarioID = scenarioID
	expected.ExecutionID = os.Getenv("PEBBLEDB_EXECUTION_ID")

	// Persist oracle BEFORE crash so parent can verify after process death.
	if err := expected.Persist(testDir); err != nil {
		_ = os.Unsetenv("PEBBLEDB_CRASH_AT")
		_ = instance.Close()
		fmt.Fprintf(os.Stderr, "atf-child: persist expected state failed: %v\n", err)
		os.Exit(1)
	}

	isCompaction := strings.HasPrefix(crashAt, "compact_")

	if doFlush {
		// First flush turns the generated memtable into SST #1.
		// For flush_* crash points this call hits maybeCrash(crashAt) and never
		// returns (os.Exit(2)). For compact_* points the flush hooks do not match
		// crashAt, so it completes normally.
		if err := writer.Flush(); err != nil {
			_ = os.Unsetenv("PEBBLEDB_CRASH_AT")
			_ = instance.Close()
			fmt.Fprintf(os.Stderr, "atf-child: force flush failed: %v\n", err)
			os.Exit(1)
		}

		if isCompaction {
			// Produce a second SSTable with identical logical content, then run a
			// synchronous compaction so maybeCrash(compact_*) fires deterministically.
			if err := replayExpectedState(writer, expected); err != nil {
				_ = os.Unsetenv("PEBBLEDB_CRASH_AT")
				_ = instance.Close()
				fmt.Fprintf(os.Stderr, "atf-child: replay for compaction failed: %v\n", err)
				os.Exit(1)
			}
			if err := writer.Flush(); err != nil {
				_ = os.Unsetenv("PEBBLEDB_CRASH_AT")
				_ = instance.Close()
				fmt.Fprintf(os.Stderr, "atf-child: second flush failed: %v\n", err)
				os.Exit(1)
			}
			// On crash this never returns (os.Exit(2)).
			if err := instance.ForceCompaction(); err != nil {
				_ = os.Unsetenv("PEBBLEDB_CRASH_AT")
				_ = instance.Close()
				fmt.Fprintf(os.Stderr, "atf-child: force compaction failed: %v\n", err)
				os.Exit(1)
			}
		}
	}

	// Prevent Close()-induced flush from re-entering crash points.
	_ = os.Unsetenv("PEBBLEDB_CRASH_AT")
	if err := instance.Close(); err != nil {
		fmt.Fprintf(os.Stderr, "atf-child: close failed: %v\n", err)
		os.Exit(1)
	}

	if crashAt != "" && doFlush {
		// Crash point was set but process survived — treat as incomplete crash coverage.
		fmt.Fprintf(os.Stderr, "atf-child: crash point %q did not fire\n", crashAt)
		os.Exit(1)
	}
	os.Exit(0)
}

func parseInt64Env(key string, def int64) int64 {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return def
	}
	return n
}
