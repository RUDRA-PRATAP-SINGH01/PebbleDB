// Package runner orchestrates scenario execution.
package runner

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/db"
	"github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/db/acceptance/framework/dataset"
)

// PebbleWriter wraps a *db.DB instance to implement interfaces.LogicalWriter.
type PebbleWriter struct {
	instance *db.DB
}

// NewPebbleWriter allocates a PebbleWriter wrapper.
func NewPebbleWriter(instance *db.DB) *PebbleWriter {
	return &PebbleWriter{instance: instance}
}

// Put writes key and value.
func (w *PebbleWriter) Put(key, value []byte) error {
	return w.instance.Put(key, value)
}

// Delete writes tombstone.
func (w *PebbleWriter) Delete(key []byte) error {
	return w.instance.Delete(key)
}

// Flush triggers database flush.
func (w *PebbleWriter) Flush() error {
	return w.instance.FlushPendingBatch()
}

// Sync flushes write ahead logs.
func (w *PebbleWriter) Sync() error {
	return w.instance.Sync()
}

// Close closes database connection.
func (w *PebbleWriter) Close() error {
	return w.instance.Close()
}

// RunChildProcessMain executes inside the child process.
// It parses env variables, opens PebbleDB, runs dataset writing, and exits cleanly.
func RunChildProcessMain() {
	scenarioID := os.Getenv("PEBBLEDB_SCENARIO_ID")
	fmt.Fprintf(os.Stderr, "child process: starting scenario %s\n", scenarioID)
	testDir := os.Getenv("PEBBLEDB_TEST_DIR")
	if testDir == "" {
		fmt.Fprintf(os.Stderr, "child process: missing PEBBLEDB_TEST_DIR\n")
		os.Exit(1)
	}

	memtableSize, err := strconv.ParseInt(os.Getenv("PEBBLEDB_MEMTABLE_SIZE"), 10, 64)
	if err != nil {
		memtableSize = 4 << 20 // default 4MB
	}

	compactionThreshold, err := strconv.Atoi(os.Getenv("PEBBLEDB_COMPACTION_THRESHOLD"))
	if err != nil {
		compactionThreshold = 4
	}

	syncWrites, _ := strconv.ParseBool(os.Getenv("PEBBLEDB_SYNC_WRITES"))

	// Create and Open PebbleDB Options
	opts := db.Options{
		Dir:                 testDir,
		MemtableSize:        memtableSize,
		CompactionThreshold: compactionThreshold,
		SyncWrites:          syncWrites,
	}

	instance, err := db.Open(opts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "child process: open database failed: %v\n", err)
		os.Exit(1)
	}

	writer := NewPebbleWriter(instance)

	// In Milestone 2 we only support standard Sequential Generator
	seed := int64(12345)
	if sVal, err := strconv.ParseInt(os.Getenv("PEBBLEDB_SEED"), 10, 64); err == nil {
		seed = sVal
	}

	keyCount := 100
	if kVal, err := strconv.Atoi(os.Getenv("PEBBLEDB_KEY_COUNT")); err == nil {
		keyCount = kVal
	}

	gen := dataset.NewSequentialGenerator(seed, keyCount, 10, 5)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	_, err = gen.Generate(ctx, writer)
	if err != nil {
		_ = instance.Close()
		fmt.Fprintf(os.Stderr, "child process: dataset generation failed: %v\n", err)
		os.Exit(1)
	}

	if err := instance.Close(); err != nil {
		fmt.Fprintf(os.Stderr, "child process: close database failed: %v\n", err)
		os.Exit(1)
	}

	// Exit code 0 signals success to parent process
	os.Exit(0)
}
