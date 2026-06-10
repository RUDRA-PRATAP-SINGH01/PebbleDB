package db

import "github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/wal"

// Delete removes a key (tombstone).
func (db *DB) Delete(key []byte) error {
	db.mu.Lock()
	if db.closed {
		db.mu.Unlock()
		return ErrClosed
	}
	rec := wal.Record{Key: key, Value: nil, Tombstone: true}
	if err := db.wal.Append(rec); err != nil {
		db.mu.Unlock()
		return err
	}
	if err := db.wal.Sync(); err != nil {
		db.mu.Unlock()
		return err
	}
	db.active.Delete(key)
	shouldFlush := db.maybeFlushLocked()
	db.mu.Unlock()
	if shouldFlush {
		db.flushCh <- struct{}{}
	}
	return nil
}
