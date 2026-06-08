package db

import "github.com/RUDRA-PRATAP-SINGH01/PebbleDB/internal/wal"

// Delete removes a key (tombstone).
func (db *DB) Delete(key []byte) error {
	db.mu.Lock()
	defer db.mu.Unlock()
	if db.closed {
		return ErrClosed
	}
	rec := wal.Record{Key: key, Value: nil, Tombstone: true}
	if err := db.wal.Append(rec); err != nil {
		return err
	}
	db.active.Delete(key)
	return nil
}