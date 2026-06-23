package db

import (
	"errors"
	"fmt"
	"time"

	"github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/memtable"
)

var (
	closeFlushDrainTimeout = 30 * time.Second
	closeWorkerJoinTimeout = 5 * time.Second
)

// Close flushes pending data and closes the database.
func (db *DB) Close() error {
	db.mu.Lock()
	if db.closed {
		db.mu.Unlock()
		return nil
	}
	db.closed = true
	db.closedFlag.Store(true)
	if db.batchTimer != nil {
		db.batchTimer.Stop()
		db.batchTimer = nil
	}
	db.mu.Unlock()

	var closeErr error
	workersShutdown := false
	shutdownWorkers := func() {
		if workersShutdown {
			return
		}
		workersShutdown = true
		if err := db.shutdownBackgroundWorkers(); err != nil {
			closeErr = errors.Join(closeErr, err)
		}
	}
	defer shutdownWorkers()

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
				closeErr = errors.Join(closeErr, err)
				break
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

	// Stop flush/compaction before closing SST readers; otherwise doCompaction
	// can read blocks while discardAllReaders closes the same files (-race).
	shutdownWorkers()

	db.mu.Lock()
	defer db.mu.Unlock()
	db.sstables = nil
	db.sstablesSnap.Store(nil)
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

func (db *DB) shutdownBackgroundWorkers() error {
	var err error

	close(db.flushCh)
	if waitErr := waitForChannelClose(db.flushDone, closeWorkerJoinTimeout); waitErr != nil {
		err = errors.Join(err, waitErr)
	}

	db.compactMu.Lock()
	close(db.compactCh)
	db.compactMu.Unlock()
	if waitErr := waitForChannelClose(db.compactDone, closeWorkerJoinTimeout); waitErr != nil {
		err = errors.Join(err, waitErr)
	}
	return err
}

func waitForChannelClose(done <-chan struct{}, timeout time.Duration) error {
	select {
	case <-done:
		return nil
	case <-time.After(timeout):
		return ErrCloseWorkerTimeout
	}
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
