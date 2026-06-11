package db

import (
	"time"

	"github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/memtable"
)

// Close flushes pending data and closes the database.
func (db *DB) Close() error {
	db.mu.Lock()
	if db.closed {
		db.mu.Unlock()
		return nil
	}
	db.closed = true
	db.mu.Unlock()

	for {
		var needFlush bool
		db.mu.Lock()
		if !db.hasPendingFlush() && db.active.Size() <= 0 {
			db.mu.Unlock()
			break
		}
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
			needFlush = true
		}
		db.mu.Unlock()

		if needFlush {
			db.notifyFlush()
		}
		db.waitForPendingFlushDrain()
	}

	close(db.flushCh)
	<-db.flushDone

	db.compactMu.Lock()
	close(db.compactCh)
	db.compactMu.Unlock()
	<-db.compactDone

	db.mu.Lock()
	defer db.mu.Unlock()
	for _, r := range db.sstables {
		r.Close()
	}
	db.sstables = nil
	if db.wal != nil {
		db.wal.Sync()
		db.wal.Close()
		db.wal = nil
	}
	if db.manifest != nil {
		db.manifest.Close()
		db.manifest = nil
	}
	return nil
}

func (db *DB) waitForPendingFlushDrain() {
	for {
		db.mu.RLock()
		pending := db.hasPendingFlush()
		db.mu.RUnlock()
		if !pending {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
}
