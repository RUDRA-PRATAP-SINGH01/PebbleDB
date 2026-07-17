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
