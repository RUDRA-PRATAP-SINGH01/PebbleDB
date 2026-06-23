package db

// Sync waits until all queued writes are persisted to the WAL (fsynced).
// After Sync returns nil, acknowledged writes survive process crash or power loss
// (assuming the underlying storage honors fsync).
//
// Sync does not flush memtables to SSTables; call Close for a full durable shutdown.
func (db *DB) Sync() error {
	if err := db.writeBlockingBackgroundErr(); err != nil {
		return err
	}

	db.mu.Lock()
	if db.closed {
		db.mu.Unlock()
		return ErrClosed
	}
	needsPersist := len(db.pendingBatch) > 0
	db.mu.Unlock()

	if !needsPersist {
		return nil
	}
	return db.awaitBatchPersist()
}
