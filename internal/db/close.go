package db

import (
	"errors"
	"fmt"
	"time"

	"github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/memtable"
)

var (
	closeFlushDrainTimeout = 30 * time.Second
	closeWorkerJoinTimeout = 30 * time.Second
)

// Close flushes pending data and closes the database.
// If shutdown cannot complete (flush drain or worker join timeout), Close returns
// an error and leaves the database closed without tearing down WAL/manifest handles
// so background workers cannot race with a nil manifest.
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

	defer func() {
		releaseDirLock(db.dirLock)
		db.dirLock = nil
	}()

	var closeErr error

	if err := db.stopBatchFlusher(); err != nil {
		closeErr = errors.Join(closeErr, err)
	}
	if err := db.flushPendingBatch(); err != nil {
		closeErr = errors.Join(closeErr, err)
	}

	for {
		var needFlush bool
		var hasPending bool
		db.mu.Lock()
		hasPending = db.hasPendingFlush()
		if !hasPending && db.active.Size() <= 0 {
			db.mu.Unlock()
			break
		}
		if db.active.Size() > 0 {
			offset, err := db.wal.Size()
			if err != nil {
				db.mu.Unlock()
				closeErr = errors.Join(closeErr, err)
				return db.abortClose(closeErr)
			}
			db.pendingFlush = append(db.pendingFlush, flushQueueEntry{
				mem:       db.active,
				walCutoff: offset,
			})
			db.active = memtable.NewSkipList()
			needFlush = true
			hasPending = true
		}
		db.mu.Unlock()

		if needFlush || hasPending {
			db.notifyFlushForce()
		}
		if err := db.waitForPendingFlushDrain(closeFlushDrainTimeout); err != nil {
			closeErr = errors.Join(closeErr, err)
			return db.abortClose(closeErr)
		}
	}

	if err := db.shutdownBackgroundWorkers(); err != nil {
		return db.abortClose(errors.Join(closeErr, err))
	}

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

// abortClose stops background workers but leaves WAL/manifest open so callers
// never tear down resources while flusher/compactor goroutines are still running.
func (db *DB) abortClose(err error) error {
	if shutdownErr := db.shutdownBackgroundWorkers(); shutdownErr != nil {
		err = errors.Join(err, shutdownErr)
	}
	return errors.Join(err, ErrCloseIncomplete)
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
