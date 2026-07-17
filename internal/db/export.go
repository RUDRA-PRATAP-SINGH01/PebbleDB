package db

// FlushPendingBatch exports the internal flushPendingBatch method for acceptance testing.
func (db *DB) FlushPendingBatch() error {
	return db.flushPendingBatch()
}

// HasPendingFlush returns true if there is a pending flush entry in the queue.
func (db *DB) HasPendingFlush() bool {
	db.mu.RLock()
	defer db.mu.RUnlock()
	return len(db.pendingFlush) > 0
}
