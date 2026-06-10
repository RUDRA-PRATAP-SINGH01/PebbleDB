package db

import "github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/wal"

// Put writes a key-value pair into the database.
func (db *DB) Put(key, value []byte) error {
	db.mu.Lock()
	if db.closed {
		db.mu.Unlock()
		return ErrClosed
	}
	rec := wal.Record{Key: key, Value: value, Tombstone: false}
	if err := db.wal.Append(rec); err != nil {
		db.mu.Unlock()
		return err
	}
	if err := db.wal.Sync(); err != nil {
		db.mu.Unlock()
		return err
	}
	db.active.Put(key, value)
	shouldFlush := db.maybeFlushLocked()
	db.mu.Unlock()
	if shouldFlush {
		db.flushCh <- struct{}{}
	}
	return nil
}
