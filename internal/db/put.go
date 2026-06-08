package db

import "github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/wal"

// Put writes a key-value pair into the database.
func (db *DB) Put(key, value []byte) error {
	db.mu.Lock()
	defer db.mu.Unlock()
	if db.closed {
		return ErrClosed
	}
	rec := wal.Record{Key: key, Value: value, Tombstone: false}
	if err := db.wal.Append(rec); err != nil {
		return err
	}
	if err := db.wal.Sync(); err != nil {
		return err
	}
	db.active.Put(key, value)
	return nil
}