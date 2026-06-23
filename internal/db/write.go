package db

import "github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/wal"

func (db *DB) writeRecord(rec wal.Record) error {
	if err := db.blockingBackgroundErr(); err != nil {
		return err
	}

	db.mu.Lock()
	if db.closed {
		db.mu.Unlock()
		return ErrClosed
	}

	db.pendingBatch = append(db.pendingBatch, ownedRecord(rec))
	db.batchSizeBytes += recordWireSize(rec)
	db.scheduleBatchFlushLocked()

	flushNow := len(db.pendingBatch) >= batchMaxRecords ||
		db.batchSizeBytes >= batchMaxBytes ||
		db.active.Size()+int64(db.batchSizeBytes) > db.memtableSize
	if !flushNow {
		db.mu.Unlock()
		return nil
	}
	db.mu.Unlock()

	return db.awaitBatchPersist()
}
