package db

import (
	"time"

	"github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/wal"
)

const (
	batchMaxRecords = 64
	batchMaxBytes   = 16 * 1024
	batchFlushDelay = 1 * time.Millisecond
)

func recordWireSize(rec wal.Record) int {
	return 4 + len(rec.Key) + 4 + len(rec.Value) + 1 + 4
}

func copyRecord(rec wal.Record) wal.Record {
	out := wal.Record{Tombstone: rec.Tombstone}
	if rec.Key != nil {
		out.Key = append([]byte(nil), rec.Key...)
	}
	if rec.Value != nil {
		out.Value = append([]byte(nil), rec.Value...)
	}
	return out
}

func applyRecordToMemtable(db *DB, rec wal.Record) {
	if rec.Tombstone {
		db.active.Delete(rec.Key)
	} else {
		db.active.Put(rec.Key, rec.Value)
	}
}

// takePendingBatchLocked moves the current WAL batch out of the queue.
// db.mu must be held.
func takePendingBatchLocked(db *DB) []wal.Record {
	if len(db.pendingBatch) == 0 {
		return nil
	}
	batch := append([]wal.Record(nil), db.pendingBatch...)
	db.pendingBatch = db.pendingBatch[:0]
	db.batchSizeBytes = 0
	return batch
}

// restorePendingBatchLocked prepends a failed WAL batch back onto the queue.
// db.mu must be held.
func restorePendingBatchLocked(db *DB, batch []wal.Record) {
	db.pendingBatch = append(batch, db.pendingBatch...)
	for _, rec := range batch {
		db.batchSizeBytes += recordWireSize(rec)
	}
}

func (db *DB) scheduleBatchFlushLocked() {
	if db.batchTimer == nil {
		db.batchTimer = time.AfterFunc(batchFlushDelay, func() {
			select {
			case db.batchFlushCh <- struct{}{}:
			default:
			}
		})
		return
	}
	db.batchTimer.Reset(batchFlushDelay)
}

func (db *DB) batchFlusher() {
	defer close(db.batchDone)

	ticker := time.NewTicker(batchFlushDelay)
	defer ticker.Stop()

	for {
		select {
		case <-db.batchStop:
			_ = db.flushPendingBatch()
			return
		case <-db.batchFlushCh:
			_ = db.flushPendingBatch()
		case <-ticker.C:
			db.mu.Lock()
			pending := len(db.pendingBatch) > 0
			db.mu.Unlock()
			if pending {
				_ = db.flushPendingBatch()
			}
		}
	}
}

// flushPendingBatch writes the in-memory batch to the WAL (one fsync) and applies
// records to the active memtable. Safe to call with an empty batch.
func (db *DB) flushPendingBatch() error {
	db.mu.Lock()
	if len(db.pendingBatch) == 0 {
		db.mu.Unlock()
		return nil
	}
	batch := takePendingBatchLocked(db)
	db.mu.Unlock()

	if err := db.wal.AppendBatch(batch); err != nil {
		db.mu.Lock()
		restorePendingBatchLocked(db, batch)
		db.mu.Unlock()
		db.setBackgroundErr("wal", err)
		return err
	}

	db.mu.Lock()
	for _, rec := range batch {
		applyRecordToMemtable(db, rec)
	}
	shouldFlush, err := db.maybeFlushLocked()
	db.mu.Unlock()
	if err != nil {
		return err
	}
	if shouldFlush {
		db.notifyFlush()
	}
	return nil
}

func (db *DB) stopBatchFlusher() {
	if db.batchStop == nil {
		return
	}
	close(db.batchStop)
	<-db.batchDone
	db.batchStop = nil
}
