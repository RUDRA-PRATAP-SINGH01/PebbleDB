package db

import (
	"errors"
	"fmt"
	"time"

	"github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/memtable"
)

var closeFlushDrainTimeout = 30 * time.Second

// Close flushes pending data and closes the database.
func (db *DB) Close() error {
	db.mu.Lock()
	if db.closed {
		db.mu.Unlock()
		return nil
	}
	db.closed = true
	if db.batchTimer != nil {
		db.batchTimer.Stop()
		db.batchTimer = nil
	}
	db.mu.Unlock()

	var closeErr error

	db.stopBatchFlusher()
	if err := db.flushPendingBatch(); err != nil {
		closeErr = errors.Join(closeErr, err)
	}

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
			db.notifyFlushForce()
		}
		if err := db.waitForPendingFlushDrain(closeFlushDrainTimeout); err != nil {
			closeErr = errors.Join(closeErr, err)
			break
		}
	}

	close(db.flushCh)
	<-db.flushDone

	db.compactMu.Lock()
	close(db.compactCh)
	db.compactMu.Unlock()
	<-db.compactDone

	db.mu.Lock()
	defer db.mu.Unlock()
	db.sstables = nil
	db.discardAllReaders()
	if db.wal != nil {
		db.wal.Sync()
		db.wal.Close()
		db.wal = nil
	}
	if db.manifest != nil {
		db.manifest.Close()
		db.manifest = nil
	}
	return closeErr
}

func (db *DB) waitForPendingFlushDrain(timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		db.mu.RLock()
		pending := db.hasPendingFlush()
		db.mu.RUnlock()
		if !pending {
			return nil
		}
		time.Sleep(5 * time.Millisecond)
	}

	if bg := db.BackgroundError(); bg != nil {
		return fmt.Errorf("%w: %v", ErrCloseFlushTimeout, bg)
	}
	return ErrCloseFlushTimeout
}
