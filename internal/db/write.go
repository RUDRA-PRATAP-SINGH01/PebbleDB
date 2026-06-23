package db

import "github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/wal"

func (db *DB) writeRecord(rec wal.Record) error {
	if err := db.blockingBackgroundErr(); err != nil {
		return err
	}

	rec = copyRecord(rec)

	db.mu.Lock()
	if db.closed {
		db.mu.Unlock()
		return ErrClosed
	}

	db.pendingBatch = append(db.pendingBatch, rec)
	db.batchSizeBytes += recordWireSize(rec)
	db.scheduleBatchFlushLocked()

	flushNow := len(db.pendingBatch) >= batchMaxRecords ||
		db.batchSizeBytes >= batchMaxBytes ||
		db.active.Size()+int64(db.batchSizeBytes) > db.memtableSize
	if !flushNow {
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
	for _, r := range batch {
		applyRecordToMemtable(db, r)
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
