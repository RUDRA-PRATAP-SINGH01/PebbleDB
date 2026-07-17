package db

import (
	"time"

	"github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/memtable"
)

// FlushPendingBatch exports WAL group-commit flush (batch → memtable) for tests.
func (db *DB) FlushPendingBatch() error {
	return db.flushPendingBatch()
}

// HasPendingFlush reports whether a memtable is queued for SST flush.
func (db *DB) HasPendingFlush() bool {
	db.mu.RLock()
	defer db.mu.RUnlock()
	return len(db.pendingFlush) > 0
}

// ForceMemtableFlush freezes the active memtable (if non-empty), enqueues SST flush,
// and waits until the flush queue drains or times out.
//
// Used by acceptance tests to hit flush crash points without going through Close().
func (db *DB) ForceMemtableFlush() error {
	if err := db.flushPendingBatch(); err != nil {
		return err
	}

	db.mu.Lock()
	if db.closed {
		db.mu.Unlock()
		return ErrClosed
	}
	needNotify := false
	if db.active.Size() > 0 {
		offset, err := db.wal.Size()
		if err != nil {
			db.mu.Unlock()
			return err
		}
		db.pendingFlush = append(db.pendingFlush, flushQueueEntry{
			mem:       db.active,
			walCutoff: offset,
		})
		db.active = memtable.NewSkipList()
		needNotify = true
	} else if db.hasPendingFlush() {
		needNotify = true
	}
	db.mu.Unlock()

	if !needNotify {
		return nil
	}
	db.notifyFlush()
	return db.waitForPendingFlushDrain(30 * time.Second)
}

// ForceCompaction synchronously runs a single compaction cycle (merge the oldest
// SSTables, commit the new file set to the manifest, swap the in-memory set, and
// discard the inputs). Acceptance tests use it to deterministically reach the
// compaction crash points (PEBBLEDB_CRASH_AT=compact_*) without depending on the
// background compactor's timing.
//
// It requires at least CompactionThreshold live SSTables; otherwise it is a no-op.
// It serializes against the background compactor via compactMu, so at most one
// compaction runs at a time.
func (db *DB) ForceCompaction() error {
	db.mu.RLock()
	closed := db.closed
	db.mu.RUnlock()
	if closed {
		return ErrClosed
	}
	db.compactMu.Lock()
	defer db.compactMu.Unlock()
	return db.doCompaction()
}
